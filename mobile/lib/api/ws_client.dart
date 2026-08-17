import 'dart:async';
import 'dart:convert';

import 'ws_channel.dart';

typedef WsHandler = void Function(Map<String, dynamic> msg);

/// 底层通道工厂:默认连真实 WS;隧道模式下由调用方注入
/// (每次(重)连都会重新调用,拿到一条新隧道流)。
typedef WsChannelFactory = Future<WsChannel> Function();

/// WebSocket client with auto-reconnect, request/response correlation and
/// typed event subscription.
class WsClient {
  WsClient(this.url, this.token, {WsChannelFactory? channelFactory})
      : _channelFactory = channelFactory;

  final String url;
  final String token;
  final WsChannelFactory? _channelFactory;

  WsChannel? _ch;
  StreamSubscription? _sub;
  bool _stopped = false;
  Timer? _reconnectTimer;
  Duration _backoff = const Duration(seconds: 1);

  final Map<String, Set<WsHandler>> _handlers = {};
  final Map<String, Completer<dynamic>> _reqs = {};
  int _idCounter = 0;

  void Function(bool connected)? onState;

  void connect() {
    _stopped = false;
    unawaited(_open());
  }

  Future<void> _open() async {
    try {
      final ch = _channelFactory != null
          ? await _channelFactory()
          : NetWsChannel('$url?token=$token');
      if (_stopped) {
        unawaited(ch.close());
        return;
      }
      await ch.ready;
      _ch = ch;
      _sub = ch.stream.listen(
        (data) => _handle(data),
        onError: (_) => _scheduleReconnect(),
        onDone: () => _scheduleReconnect(),
        cancelOnError: true,
      );
      _backoff = const Duration(seconds: 1);
      onState?.call(true);
    } catch (_) {
      _scheduleReconnect();
    }
  }

  void _scheduleReconnect() {
    if (_stopped) return;
    onState?.call(false);
    _reconnectTimer?.cancel();
    _reconnectTimer = Timer(_backoff, () {
      _backoff = Duration(
          milliseconds: (_backoff.inMilliseconds * 1.6).round().clamp(1000, 15000));
      unawaited(_open());
    });
  }

  void _handle(dynamic raw) {
    Map<String, dynamic> msg;
    try {
      msg = jsonDecode(raw as String) as Map<String, dynamic>;
    } catch (_) {
      return;
    }
    // request/response correlation by ref
    final ref = msg['ref'];
    if (ref is String && _reqs.containsKey(ref)) {
      final completer = _reqs.remove(ref)!;
      if (msg['ok'] == false) {
        completer.completeError(Exception(msg['error']?['message'] ?? 'failed'));
      } else {
        completer.complete(msg['payload']);
      }
      return;
    }
    final type = msg['type'] as String?;
    if (type == null) return;
    final set = _handlers[type];
    if (set != null) {
      for (final h in Set.of(set)) {
        h(msg);
      }
    }
    final wild = _handlers['*'];
    if (wild != null) {
      for (final h in Set.of(wild)) {
        h(msg);
      }
    }
  }

  VoidFn on(String type, WsHandler cb) {
    _handlers.putIfAbsent(type, () => {}).add(cb);
    return () => _handlers[type]?.remove(cb);
  }

  void send(String type, [Map<String, dynamic>? payload]) {
    _ch?.add(jsonEncode({'type': type, 'payload': payload ?? {}}));
  }

  Future<dynamic> request(String type, [Map<String, dynamic>? payload]) {
    final id = 'r${++_idCounter}';
    final completer = Completer<dynamic>();
    _reqs[id] = completer;
    _ch?.add(jsonEncode({'type': type, 'id': id, 'payload': payload ?? {}}));
    return completer.future.timeout(const Duration(seconds: 8), onTimeout: () {
      _reqs.remove(id);
      throw TimeoutException('$type timed out');
    });
  }

  void close() {
    _stopped = true;
    _reconnectTimer?.cancel();
    _sub?.cancel();
    final ch = _ch;
    _ch = null;
    if (ch != null) unawaited(ch.close());
  }
}

typedef VoidFn = void Function();
