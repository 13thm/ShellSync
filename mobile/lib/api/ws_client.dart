import 'dart:async';
import 'dart:convert';
import 'package:web_socket_channel/web_socket_channel.dart';

typedef WsHandler = void Function(Map<String, dynamic> msg);

/// WebSocket client with auto-reconnect, request/response correlation and
/// typed event subscription.
class WsClient {
  WsClient(this.url, this.token);

  final String url;
  final String token;

  WebSocketChannel? _ch;
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
    _open();
  }

  void _open() {
    final ch = WebSocketChannel.connect(Uri.parse('$url?token=$token'));
    _ch = ch;
    _sub = ch.stream.listen(
      (data) => _handle(data),
      onError: (_) => _scheduleReconnect(),
      onDone: () {
        onState?.call(false);
        _scheduleReconnect();
      },
    );
    // consider it connected once the sink is ready; fire optimistically
    onState?.call(true);
    _backoff = const Duration(seconds: 1);
  }

  void _scheduleReconnect() {
    if (_stopped) return;
    _reconnectTimer?.cancel();
    _reconnectTimer = Timer(_backoff, () {
      _backoff = Duration(
          milliseconds: (_backoff.inMilliseconds * 1.6).round().clamp(1000, 15000));
      _open();
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
    _ch?.sink.add(jsonEncode({'type': type, 'payload': payload ?? {}}));
  }

  Future<dynamic> request(String type, [Map<String, dynamic>? payload]) {
    final id = 'r${++_idCounter}';
    final completer = Completer<dynamic>();
    _reqs[id] = completer;
    _ch?.sink.add(jsonEncode({'type': type, 'id': id, 'payload': payload ?? {}}));
    return completer.future.timeout(const Duration(seconds: 8), onTimeout: () {
      _reqs.remove(id);
      throw TimeoutException('$type timed out');
    });
  }

  void close() {
    _stopped = true;
    _reconnectTimer?.cancel();
    _sub?.cancel();
    _ch?.sink.close();
    _ch = null;
  }
}

typedef VoidFn = void Function();
