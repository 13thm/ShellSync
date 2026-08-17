import 'dart:async';
import 'dart:math';

import 'package:dio/dio.dart';
import 'package:flutter/foundation.dart';
import '../api/relay_http.dart';
import '../api/relay_transport.dart';
import '../api/rest_client.dart';
import '../api/services.dart';
import '../api/ws_channel.dart';
import '../api/ws_client.dart';
import '../config/storage.dart';
import '../models.dart';
import 'pair_target.dart';

/// 当前连接路径。
enum ConnPath { lan, cloud }

/// Central app state: connection strategy (LAN first → cloud fallback),
/// entity lists + WS event routing.
class AppState extends ChangeNotifier {
  bool initialized = false;
  bool paired = false;
  bool wsConnected = false;

  /// 连接路径(null = 未连接/离线)。
  ConnPath? path;
  String? endpoint; // LAN 直连时的 http://ip:port
  String? token;
  String? relayHost; // 云路径时实际使用的 relay(可能含开发者覆盖)

  /// ws 重建计数:终端页用它做 key,云/局域网切换后重新抓取 ws。
  int wsGen = 0;

  RestClient? _rest;
  ApiService? api;
  WsClient? ws;
  RelayConnection? relay;

  List<Task> tasks = [];
  List<Todo> todos = [];
  List<Terminal> terminals = [];

  final _storage = const SecureStore();
  final List<VoidCallback> _subs = [];
  Timer? _recoverTimer;
  int _recoverFailures = 0;
  bool _switching = false;
  Credentials? _creds;

  ApiService? get service => api;

  Future<void> init() async {
    final creds = await _storage.read();
    if (creds != null) {
      _creds = creds;
      final ok = await _connectAuto(creds);
      paired = ok;
      if (!ok) {
        // 无法连上(离线/休眠的电脑):保持"已配对"并周期重试,
        // 而不是把用户踢回配对页。凭证失效的判定在连接成功后做。
        paired = true;
        _scheduleRecover();
      }
    }
    initialized = true;
    notifyListeners();
  }

  // ---- connection strategy (R1-8) ----

  /// ① 局域网探测(300ms)→ ② 云隧道握手 → 都失败返回 false。
  Future<bool> _connectAuto(Credentials creds) async {
    if (creds.hasLan && await _probeLan(creds.lanEndpoint!)) {
      await _connectLan(creds);
      return true;
    }
    if (creds.hasCloud) {
      final host = await _devRelayHost(creds.cloudHost!);
      final r = RelayConnection(relayWsUrl(host));
      try {
        await r.connect(timeout: const Duration(seconds: 6));
      } catch (_) {
        await r.close();
        return false;
      }
      if (creds.devId == null) {
        await r.close();
        return false; // 云凭证不完整
      }
      await _connectCloud(creds, r, host);
      return true;
    }
    return false;
  }

  Future<String> _devRelayHost(String cloudHost) async {
    if (!kDebugMode) return cloudHost;
    final override = await _storage.devRelayOverride();
    return (override != null && override.isNotEmpty) ? override : cloudHost;
  }

  /// GET /health 快探测(短超时,失败即回落云)。
  Future<bool> _probeLan(String ep) async {
    try {
      final dio = Dio(BaseOptions(
        baseUrl: ep,
        connectTimeout: const Duration(milliseconds: 600),
        receiveTimeout: const Duration(milliseconds: 600),
      ));
      final r = await dio.get<dynamic>('/health');
      return r.statusCode == 200;
    } catch (_) {
      return false;
    }
  }

  Future<void> _connectLan(Credentials creds) async {
    await _teardownClients();
    path = ConnPath.lan;
    endpoint = creds.lanEndpoint;
    relayHost = null;
    token = creds.token;
    _rest = RestClient(creds.lanEndpoint!, creds.token);
    api = ApiService(_rest!);
    _setupWs(WsClient(_wsUrlOf(creds.lanEndpoint!), creds.token));
    await _afterConnect();
  }

  Future<void> _connectCloud(Credentials creds, RelayConnection r, String host) async {
    await _teardownClients();
    path = ConnPath.cloud;
    endpoint = null;
    relayHost = host;
    token = creds.token;
    relay = r;
    final devId = creds.devId!;
    final http = RelayHttpClient(r, devId);
    _rest = RestClient('http://tunnel', creds.token,
        tunnel: http.request);
    api = ApiService(_rest!);
    // 每次连接新开一条隧道流承载 WS
    _setupWs(WsClient('ws://tunnel/ws', creds.token, channelFactory: () async {
      final stream = await r.open(devId);
      return TunnelWsChannel(stream, '/ws?token=${creds.token}');
    }));
    await _afterConnect();
  }

  void _setupWs(WsClient w) {
    ws = w;
    w.onState = _onWsState;
    _wireEvents();
    w.connect();
  }

  Future<void> _afterConnect() async {
    await _loadAll();
    paired = true;
    wsGen++;
    notifyListeners();
  }

  String _wsUrlOf(String ep) => '${ep.replaceFirst(RegExp(r'^http'), 'ws')}/ws';

  Future<void> _teardownClients() async {
    for (final s in _subs) {
      s();
    }
    _subs.clear();
    ws?.close();
    ws = null;
    await relay?.close();
    relay = null;
    _rest = null;
    api = null;
  }

  // ---- 断线切换 ----

  void _onWsState(bool connected) {
    wsConnected = connected;
    notifyListeners();
    if (!connected && paired) {
      // 给 WsClient 自身重连留 3 秒;仍不行则整套重评(可能要换路径)
      _recoverTimer?.cancel();
      _recoverTimer = Timer(const Duration(seconds: 3), () => _recover());
    } else if (connected) {
      _recoverTimer?.cancel();
      _recoverFailures = 0;
    }
  }

  Future<void> _recover() async {
    if (_switching || _creds == null) return;
    _switching = true;
    try {
      final ok = await _connectAuto(_creds!);
      if (ok) {
        _recoverFailures = 0;
        return; // _afterConnect 已 notify
      }
      _scheduleRecover();
    } catch (_) {
      _scheduleRecover();
    } finally {
      _switching = false;
    }
  }

  void _scheduleRecover() {
    _recoverFailures = min(_recoverFailures + 1, 5);
    final delay = Duration(seconds: 1 << _recoverFailures); // 2s … 32s
    _recoverTimer?.cancel();
    _recoverTimer = Timer(delay, () => _recover());
  }

  /// 手动重连(设置页"立即重连"按钮)。
  Future<void> reconnectNow() async {
    _recoverTimer?.cancel();
    await _recover();
  }

  /// 开发者 relay 覆盖(代理到 SecureStore)。
  Future<String?> devRelayOverride() => _storage.devRelayOverride();
  Future<void> setDevRelayOverride(String? v) => _storage.setDevRelayOverride(v);

  void _wireEvents() {
    final w = ws!;
    _subs
      ..add(w.on('task.created', (m) => _upsertTask(m['payload'])))
      ..add(w.on('task.updated', (m) => _upsertTask(m['payload'])))
      ..add(w.on('task.deleted', (m) => _removeTask(m['payload']['id'])))
      ..add(w.on('terminal.created', (m) => _upsertTerminal(m['payload'])))
      ..add(w.on('terminal.updated', (m) => _upsertTerminal(m['payload'])))
      ..add(w.on('terminal.deleted', (m) => _removeTerminal(m['payload']['id'])))
      ..add(w.on('todo.created', (m) => _upsertTodo(m['payload'])))
      ..add(w.on('todo.updated', (m) => _upsertTodo(m['payload'])))
      ..add(w.on('todo.deleted', (m) => _removeTodo(m['payload']['id'])));
  }

  Future<void> _loadAll() async {
    final results = await Future.wait([
      api!.listTasks(),
      api!.listTodos(),
      api!.listTerminals(),
    ]);
    tasks = results[0] as List<Task>;
    todos = results[1] as List<Todo>;
    terminals = results[2] as List<Terminal>;
  }

  // ---- pairing (R1-8: v2 QR, LAN first → cloud) ----

  /// 扫码/手动输入后调用:先试局域网,失败走云隧道。
  Future<void> pairWith(PairTarget t) async {
    String? sessionToken;
    final lanEp = t.lan == null ? null : 'http://${t.lan}';
    final devId = t.devId;

    // ① 局域网直连(短超时)
    if (lanEp != null) {
      try {
        final res = await RestClient.pairVerify(lanEp, t.code, '我的手机',
            defaultTargetPlatform.name,
            timeout: const Duration(seconds: 3));
        sessionToken = res['sessionToken'] as String;
      } catch (_) {
        sessionToken = null; // 落到云
      }
    }

    // ② 云中继(扫码跨网的主路径)
    var cloudHost = t.cloud;
    if (sessionToken == null && cloudHost != null && cloudHost.isNotEmpty) {
      if (!kDebugMode) {
        // release 用二维码地址
      } else {
        final override = await _storage.devRelayOverride();
        if (override != null && override.isNotEmpty) cloudHost = override;
      }
      final r = RelayConnection(relayWsUrl(cloudHost));
      try {
        await r.connect(timeout: const Duration(seconds: 6));
        final claimedDev = await r.claim(t.code);
        final effectiveDev = (devId != null && devId.isNotEmpty) ? devId : claimedDev;
        final http = RelayHttpClient(r, effectiveDev);
        final res = await RestClient.pairVerify(
            '', t.code, '我的手机', defaultTargetPlatform.name,
            tunnel: http.request);
        sessionToken = res['sessionToken'] as String;
        await r.close();
      } catch (e) {
        await r.close();
        rethrow;
      }
    }

    if (sessionToken == null) {
      throw Exception('配对失败:局域网与云端均不可达或配对码无效');
    }

    final creds = Credentials(
      lanEndpoint: lanEp,
      cloudHost: (cloudHost != null && cloudHost.isNotEmpty) ? cloudHost : null,
      devId: devId,
      token: sessionToken,
    );
    await _storage.write(creds);
    _creds = creds;
    final ok = await _connectAuto(creds);
    if (!ok) {
      // 配对成功但立即失联(罕见):保持离线重试
      paired = true;
      _scheduleRecover();
      notifyListeners();
    }
  }

  /// 手动输入(v1 语义):纯局域网配对。
  Future<void> pair({required String ip, required int port, required String code}) {
    return pairWith(PairTarget(
      lan: '$ip:$port',
      code: code,
    ));
  }

  Future<void> logout() async {
    _recoverTimer?.cancel();
    for (final s in _subs) {
      s();
    }
    _subs.clear();
    ws?.close();
    ws = null;
    await relay?.close();
    relay = null;
    api = null;
    _rest = null;
    await _storage.clear();
    _creds = null;
    paired = false;
    path = null;
    wsConnected = false;
    tasks = [];
    todos = [];
    terminals = [];
    notifyListeners();
  }

  // ---- entity mutators ----

  void _upsertTask(dynamic json) {
    if (json is! Map<String, dynamic>) return;
    final t = Task.fromJson(json);
    final i = tasks.indexWhere((e) => e.id == t.id);
    if (i >= 0) {
      tasks[i] = t;
    } else {
      tasks.insert(0, t);
    }
    notifyListeners();
  }

  void _removeTask(String id) {
    tasks.removeWhere((e) => e.id == id);
    notifyListeners();
  }

  Future<void> updateTask(String id, Map<String, dynamic> patch) async {
    await api!.updateTask(id, patch); // server broadcasts update via WS
  }

  void _upsertTodo(dynamic json) {
    if (json is! Map<String, dynamic>) return;
    final t = Todo.fromJson(json);
    final i = todos.indexWhere((e) => e.id == t.id);
    if (i >= 0) {
      todos[i] = t;
    } else {
      todos.insert(0, t);
    }
    notifyListeners();
  }

  void _removeTodo(String id) {
    todos.removeWhere((e) => e.id == id);
    notifyListeners();
  }

  Future<void> addTodo(String title, {String? taskId}) async {
    await api!.createTodo(title, taskId: taskId);
  }

  Future<void> toggleTodo(Todo t) async {
    await api!.updateTodo(t.id, {'status': t.status == 'done' ? 'pending' : 'done'});
  }

  Future<void> deleteTodo(String id) async {
    await api!.deleteTodo(id);
  }

  void _upsertTerminal(dynamic json) {
    if (json is! Map<String, dynamic>) return;
    final t = Terminal.fromJson(json);
    final i = terminals.indexWhere((e) => e.id == t.id);
    if (i >= 0) {
      terminals[i] = t;
    } else {
      terminals.insert(0, t);
    }
    notifyListeners();
  }

  void _removeTerminal(String id) {
    terminals.removeWhere((e) => e.id == id);
    notifyListeners();
  }
}
