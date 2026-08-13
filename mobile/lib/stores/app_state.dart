import 'package:flutter/foundation.dart';
import '../api/rest_client.dart';
import '../api/services.dart';
import '../api/ws_client.dart';
import '../config/storage.dart';
import '../models.dart';

/// Central app state: connection + entity lists + WS event routing.
class AppState extends ChangeNotifier {
  bool initialized = false;
  bool paired = false;
  bool wsConnected = false;

  String? endpoint;
  String? token;

  RestClient? _rest;
  ApiService? api;
  WsClient? ws;

  List<Task> tasks = [];
  List<Todo> todos = [];
  List<Terminal> terminals = [];

  final _storage = const SecureStore();
  final List<VoidCallback> _subs = [];

  ApiService? get service => api;

  Future<void> init() async {
    final creds = await _storage.read();
    if (creds != null) {
      try {
        await _connect(creds.endpoint, creds.token);
      } catch (_) {
        // stored creds invalid; user must re-pair
      }
    }
    initialized = true;
    notifyListeners();
  }

  Future<void> _connect(String ep, String tok) async {
    endpoint = ep;
    token = tok;
    _rest = RestClient(ep, tok);
    api = ApiService(_rest!);

    final wsUrl = ep.replaceFirst(RegExp(r'^http'), 'ws') + '/ws';
    ws = WsClient(wsUrl, tok);
    ws!.onState = (c) {
      wsConnected = c;
      notifyListeners();
    };
    _wireEvents();
    ws!.connect();

    await _loadAll();
    paired = true;
    notifyListeners();
  }

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
    notifyListeners();
  }

  /// Pair via QR payload (shellsync://pair?ip=&port=&code=) or raw fields.
  Future<void> pair({required String ip, required int port, required String code}) async {
    final baseUrl = 'http://$ip:$port';
    final res = await RestClient.pairVerify(baseUrl, code, '我的手机', defaultTargetPlatform.name);
    final tok = res['sessionToken'] as String;
    await _storage.write(Credentials(baseUrl, tok));
    await _connect(baseUrl, tok);
  }

  Future<void> logout() async {
    for (final s in _subs) {
      s();
    }
    _subs.clear();
    ws?.close();
    ws = null;
    api = null;
    _rest = null;
    await _storage.clear();
    paired = false;
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
    if (i >= 0) tasks[i] = t; else tasks.insert(0, t);
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
    if (i >= 0) todos[i] = t; else todos.insert(0, t);
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
    if (i >= 0) terminals[i] = t; else terminals.insert(0, t);
    notifyListeners();
  }

  void _removeTerminal(String id) {
    terminals.removeWhere((e) => e.id == id);
    notifyListeners();
  }
}
