import 'dart:async';
import 'dart:typed_data';
import 'package:web_socket_channel/web_socket_channel.dart';

import 'relay_protocol.dart';

/// 云中继连接状态。
enum RelayState { connecting, online, offline }

/// 一条隧道流(虚拟 socket):`send` 发原始字节,`incoming` 收原始字节。
class RelayStream {
  RelayStream._(this.id, this._conn);

  final int id;
  final RelayConnection _conn;

  final _incoming = StreamController<Uint8List>.broadcast();
  final _done = Completer<void>();
  bool _locallyClosed = false;
  String closeReason = '';

  /// 收到的字节(流式,broadcast:可多订阅;不自动重放)。
  Stream<Uint8List> get incoming => _incoming.stream;

  /// 流结束(本端关闭或对端关闭)。
  Future<void> get done => _done.future;

  bool get isClosed => _done.isCompleted;

  void _onData(Uint8List payload) {
    if (!isClosed) _incoming.add(payload);
  }

  void _onPeerClosed(String why) {
    closeReason = why;
    _finish();
  }

  void _finish() {
    if (!_done.isCompleted) {
      _done.complete();
      _incoming.close();
    }
  }

  /// 发送原始字节(自动按 streamId 分帧)。
  void send(List<int> bytes) {
    if (isClosed) {
      throw StateError('relay stream #$id already closed');
    }
    _conn._sendRaw(encodeDataFrame(id, bytes));
  }

  /// 主动关闭(发送 close 控制帧)。
  Future<void> close({String why = 'client closed'}) async {
    if (_locallyClosed) return;
    _locallyClosed = true;
    _conn._sendControl(RelayControl('close', {'streamId': id, 'why': why}));
    _finish();
    _conn._streams.remove(id);
  }
}

/// 与云中继的一条 WSS 长连接:hello(client) 握手、claim/open 控制流程、
/// 按 streamId 多路复用数据帧、断线指数退避重连。
///
/// 用法:
/// ```dart
/// final conn = RelayConnection('ws://127.0.0.1:8788/ws');
/// await conn.connect();              // 首连;失败抛出
/// final devId = await conn.claim(code); // 配对时
/// final stream = await conn.open(devId); // 每个会话一条流
/// ```
class RelayConnection {
  RelayConnection(this.url);

  /// relay WSS/WS 地址(ws://host:port/ws 或 wss://host/ws)。
  final String url;

  RelayState _state = RelayState.offline;
  final _states = StreamController<RelayState>.broadcast();

  WebSocketChannel? _ws;
  StreamSubscription? _sub;
  bool _manuallyClosed = false;
  Timer? _reconnectTimer;
  Duration _backoff = const Duration(seconds: 1);

  final _streams = <int, RelayStream>{};
  final _pendingOpens = <_PendingOpen>[];

  RelayState get state => _state;
  Stream<RelayState> get states => _states.stream;
  bool get isOnline => _state == RelayState.online;

  void _setState(RelayState s) {
    if (_state == s) return;
    _state = s;
    if (!_states.isClosed) _states.add(s);
  }

  /// 首次连接;失败抛出(之后由内部循环自动重连)。
  Future<void> connect({Duration timeout = const Duration(seconds: 8)}) async {
    _manuallyClosed = false;
    try {
      await _dial(timeout: timeout);
    } catch (_) {
      _setState(RelayState.offline);
      _scheduleReconnect();
      rethrow;
    }
  }

  Future<void> _dial({Duration timeout = const Duration(seconds: 8)}) async {
    _setState(RelayState.connecting);
    final ws = WebSocketChannel.connect(Uri.parse(url));
    _ws = ws;
    try {
      await ws.ready.timeout(timeout);
    } catch (e) {
      await _teardownSocket();
      throw Exception('relay connect failed: $e');
    }

    _sub = ws.stream.listen(
      _onSocketData,
      onError: (_) => _onSocketDown(),
      onDone: _onSocketDown,
      cancelOnError: true,
    );

    // hello(client) 握手
    final helloDone = Completer<void>();
    final sub = states.listen((s) {
      if (s == RelayState.online && !helloDone.isCompleted) helloDone.complete();
    });
    _sendControl(RelayControl('hello', {'role': 'client', 'ver': 1}));
    _backoff = const Duration(seconds: 1);
    try {
      await helloDone.future.timeout(timeout);
    } catch (e) {
      await sub.cancel();
      await _teardownSocket();
      throw Exception('relay handshake timeout');
    }
    await sub.cancel();
  }

  Future<void> _teardownSocket() async {
    await _sub?.cancel();
    _sub = null;
    try {
      await _ws?.sink.close();
    } catch (_) {}
    _ws = null;
  }

  void _onSocketDown() {
    if (_manuallyClosed) return;
    // 断线:杀掉所有流、失败所有 pending open,安排重连
    for (final p in _pendingOpens) {
      p.completer.completeError(Exception('relay disconnected'));
    }
    _pendingOpens.clear();
    final dead = List<RelayStream>.from(_streams.values);
    _streams.clear();
    for (final s in dead) {
      s._onPeerClosed('relay disconnected');
    }
    _setState(RelayState.offline);
    unawaited(_teardownSocket());
    _scheduleReconnect();
  }

  void _scheduleReconnect() {
    if (_manuallyClosed) return;
    _reconnectTimer?.cancel();
    final wait = _backoff;
    _backoff = Duration(
        milliseconds: (_backoff.inMilliseconds * 1.8).round().clamp(1000, 30000));
    _reconnectTimer = Timer(wait, () async {
      if (_manuallyClosed) return;
      try {
        await _dial();
      } catch (_) {
        _scheduleReconnect();
      }
    });
  }

  /// 配对:领取配对码,返回绑定的 daemon devId。
  Future<String> claim(String code,
      {Duration timeout = const Duration(seconds: 8)}) async {
    final completer = Completer<String>();
    _claimCompleters.add(completer);
    _sendControl(RelayControl('claim', {'code': code}));
    return completer.future.timeout(timeout, onTimeout: () {
      _claimCompleters.remove(completer);
      throw TimeoutException('claim timeout');
    });
  }

  final _claimCompleters = <Completer<String>>[];

  /// 打开一条到 daemon 的隧道流(等待 daemon accept)。
  Future<RelayStream> open(String? devId,
      {Duration timeout = const Duration(seconds: 10)}) async {
    if (!isOnline) {
      throw StateError('relay connection is not online');
    }
    final pending = _PendingOpen();
    _pendingOpens.add(pending);
    _sendControl(RelayControl('open', {if (devId != null) 'devId': devId}));
    return pending.completer.future.timeout(timeout, onTimeout: () {
      _pendingOpens.remove(pending);
      throw TimeoutException('relay open timeout');
    });
  }

  /// 关闭连接(不再自动重连)。
  Future<void> close() async {
    _manuallyClosed = true;
    _reconnectTimer?.cancel();
    for (final p in _pendingOpens) {
      p.completer.completeError(Exception('relay closed'));
    }
    _pendingOpens.clear();
    final dead = List<RelayStream>.from(_streams.values);
    _streams.clear();
    for (final s in dead) {
      s._onPeerClosed('relay closed');
    }
    await _teardownSocket();
    _setState(RelayState.offline);
    if (!_states.isClosed) {
      await _states.close();
    }
  }

  // ---- socket I/O ----

  void _sendControl(RelayControl f) {
    _ws?.sink.add(f.encode());
  }

  void _sendRaw(Uint8List bytes) {
    _ws?.sink.add(bytes);
  }

  void _onSocketData(dynamic msg) {
    if (msg is String) {
      _onControl(msg);
    } else if (msg is List<int>) {
      try {
        final f = decodeDataFrame(msg);
        _streams[f.streamId]?._onData(f.payload);
      } on FormatException {
        // bad frame — ignore (server closes offenders anyway)
      }
    }
  }

  void _onControl(String raw) {
    final RelayControl f;
    try {
      f = RelayControl.decode(raw);
    } on FormatException {
      return;
    }
    switch (f.t) {
      case 'hello':
        _setState(RelayState.online);
        break;
      case 'claim':
        final c = _claimCompleters.isNotEmpty ? _claimCompleters.removeAt(0) : null;
        final devId = f.str('devId');
        if (c != null && devId != null) {
          c.complete(devId);
        } else if (c != null) {
          c.completeError(Exception('claim failed'));
        }
        break;
      case 'open':
        // stream id ack:daemon accept 之后才完成 completer
        final id = f.intOf('streamId');
        if (id != null && _pendingOpens.isNotEmpty) {
          _pendingOpens.first.ackedStreamId = id;
        }
        break;
      case 'accept':
        final id = f.intOf('streamId');
        if (id != null && _pendingOpens.isNotEmpty) {
          final p = _pendingOpens.removeAt(0);
          final s = RelayStream._(id, this);
          _streams[id] = s;
          p.completer.complete(s);
        }
        break;
      case 'close':
        final id = f.intOf('streamId');
        if (id != null) {
          final s = _streams.remove(id);
          s?._onPeerClosed(f.str('why') ?? 'closed');
        } else if (_pendingOpens.isNotEmpty) {
          // 无 streamId 的 close:打到最早的 pending open
          _pendingOpens.removeAt(0).completer.completeError(
              Exception(f.str('why') ?? 'stream closed'));
        }
        break;
      case 'error':
        final code = f.str('code') ?? 'error';
        final why = f.str('why') ?? '';
        // 优先失败 pending open(daemon_offline / too_many_streams 等)
        if (_pendingOpens.isNotEmpty) {
          _pendingOpens.removeAt(0).completer.completeError(Exception('$code: $why'));
        } else if (_claimCompleters.isNotEmpty) {
          _claimCompleters.removeAt(0).completeError(Exception('$code: $why'));
        }
        break;
      default:
        break;
    }
  }
}

class _PendingOpen {
  final completer = Completer<RelayStream>();
  int? ackedStreamId;
}
