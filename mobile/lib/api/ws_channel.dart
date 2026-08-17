import 'dart:async';
import 'dart:convert';
import 'dart:math';
import 'dart:typed_data';

import 'package:web_socket_channel/web_socket_channel.dart';

import 'package:meta/meta.dart';

import 'relay_transport.dart';

/// WsClient 的可注入底层通道:真实 WebSocket 或隧道流上的手工 WS 帧。
abstract class WsChannel {
  /// 已解码的消息(String 文本帧;本项目全部为 JSON 文本)。
  Stream get stream;

  /// 握手完成(101 之后)。
  Future<void> get ready;

  /// 发送一条文本消息。
  void add(Object data);

  /// 发起/确认关闭。
  Future<void> close();
}

/// 真实网络 WebSocket 实现。
class NetWsChannel implements WsChannel {
  NetWsChannel(String url) : _inner = WebSocketChannel.connect(Uri.parse(url));

  final WebSocketChannel _inner;

  @override
  Stream get stream => _inner.stream;

  @override
  Future<void> get ready => _inner.ready;

  @override
  void add(Object data) => _inner.sink.add(data);

  @override
  Future<void> close() => _inner.sink.close();
}

/// 隧道流上的 WebSocket:先发 HTTP Upgrade 握手,再手工编解码 WS 帧
/// (RFC 6455 客户端子集:文本/关闭/ping/pong,发送帧必须掩码)。
class TunnelWsChannel implements WsChannel {
  TunnelWsChannel(this._stream, this.pathAndQuery) {
    _ready = _handshake();
  }

  final RelayStream _stream;
  final String pathAndQuery; // 例如 /ws?token=xxx

  late final Future<void> _ready;
  final _incoming = StreamController<String>.broadcast();
  final _closed = Completer<void>();
  StreamSubscription? _sub;
  bool _sentClose = false;

  @override
  Stream get stream => _incoming.stream;

  @override
  Future<void> get ready => _ready;

  @override
  void add(Object data) {
    if (_closed.isCompleted) return;
    final payload = data is String ? utf8.encode(data) : (data as List<int>);
    _stream.send(encodeClientFrame(payload, 0x1));
  }

  @override
  Future<void> close() async {
    if (_sentClose) return;
    _sentClose = true;
    try {
      if (!_stream.isClosed) {
        _stream.send(encodeClientFrame(const [], 0x8)); // close frame
      }
    } catch (_) {}
    await _stream.close(why: 'ws closed');
    _finish();
  }

  void _finish() {
    if (!_closed.isCompleted) {
      _closed.complete();
      _incoming.close();
      _sub?.cancel();
      _sub = null;
    }
  }

  Future<void> _handshake() async {
    final key = _wsKey();
    final req = 'GET $pathAndQuery HTTP/1.1\r\n'
        'Host: shellsync\r\n'
        'Upgrade: websocket\r\n'
        'Connection: Upgrade\r\n'
        'Sec-WebSocket-Key: $key\r\n'
        'Sec-WebSocket-Version: 13\r\n\r\n';
    _stream.send(utf8.encode(req));

    final headersDone = Completer<void>();
    String? statusLine;
    var headerBytes = 0;
    final buf = <int>[];
    late final StreamSubscription sub;
    sub = _stream.incoming.listen((chunk) {
      if (headersDone.isCompleted) return;
      buf.addAll(chunk);
      final s = ascii.decode(buf, allowInvalid: true);
      final idx = s.indexOf('\r\n\r\n');
      if (idx >= 0) {
        statusLine = s.substring(0, s.indexOf('\r\n'));
        headerBytes = idx + 4;
        headersDone.complete();
      }
    });
    try {
      await headersDone.future.timeout(const Duration(seconds: 8));
    } finally {
      await sub.cancel();
    }
    if (statusLine == null || !statusLine!.contains('101')) {
      throw Exception('ws over tunnel: upgrade failed ($statusLine)');
    }

    // 剩余字节(101 响应后紧跟的首个数据帧)交给帧解码器
    final leftover = Uint8List.fromList(buf.sublist(headerBytes));
    _parser = WsFrameParser();
    _sub = _stream.incoming.listen(_onBytes);
    if (leftover.isNotEmpty) _onBytes(leftover);

    unawaited(_stream.done.whenComplete(() => _finish()));
  }

  late final WsFrameParser _parser;

  void _onBytes(Uint8List chunk) {
    for (final msg in _parser.feed(chunk)) {
      switch (msg.opcode) {
        case 0x1: // text
          _incoming.add(utf8.decode(msg.payload));
          break;
        case 0x2: // binary — 本项目未用,忽略
          break;
        case 0x8: // close → 回 close,结束
          if (!_sentClose && !_stream.isClosed) {
            try {
              _stream.send(encodeClientFrame(const [], 0x8));
            } catch (_) {}
          }
          _finish();
          break;
        case 0x9: // ping → pong
          if (!_stream.isClosed) {
            try {
              _stream.send(encodeClientFrame(msg.payload, 0xA));
            } catch (_) {}
          }
          break;
        default: // pong 等
          break;
      }
    }
  }

  static String _wsKey() {
    final rnd = Random.secure();
    final bytes = List<int>.generate(16, (_) => rnd.nextInt(256));
    return base64Encode(bytes);
  }
}

class WsFrameMessage {
  WsFrameMessage(this.opcode, this.payload);
  final int opcode;
  final List<int> payload;
}

/// 增量 WS 帧解码器(服务端不掩码;支持分片/扩展长度)。
@visibleForTesting
class WsFrameParser {
  final _buf = <int>[];
  final _fragPayload = <int>[];
  int _fragOpcode = 0;

  Iterable<WsFrameMessage> feed(List<int> chunk) {
    _buf.addAll(chunk);
    final out = <WsFrameMessage>[];
    while (true) {
      final m = _tryParseOne();
      if (m == null) break;
      out.add(m);
    }
    return out;
  }

  WsFrameMessage? _tryParseOne() {
    if (_buf.length < 2) return null;
    final b0 = _buf[0];
    final b1 = _buf[1];
    final fin = (b0 & 0x80) != 0;
    final opcode = b0 & 0x0F;
    final masked = (b1 & 0x80) != 0;
    var len = b1 & 0x7F;
    var off = 2;

    if (len == 126) {
      if (_buf.length < off + 2) return null;
      len = (_buf[off] << 8) | _buf[off + 1];
      off += 2;
    } else if (len == 127) {
      if (_buf.length < off + 8) return null;
      len = 0;
      for (var i = 0; i < 8; i++) {
        len = len * 256 + _buf[off + i];
      }
      off += 8;
    }

    Uint8List? mask;
    if (masked) {
      if (_buf.length < off + 4) return null;
      mask = Uint8List.fromList(_buf.sublist(off, off + 4));
      off += 4;
    }

    if (_buf.length < off + len) return null;
    var payload = _buf.sublist(off, off + len);
    if (mask != null) {
      for (var i = 0; i < payload.length; i++) {
        payload[i] ^= mask[i % 4];
      }
    }
    _buf.removeRange(0, off + len);

    if (opcode == 0x0) {
      // continuation
      _fragPayload.addAll(payload);
      if (fin) {
        final msg = WsFrameMessage(_fragOpcode, List<int>.from(_fragPayload));
        _fragPayload.clear();
        return msg;
      }
      return null;
    }
    if (!fin) {
      _fragPayload.clear();
      _fragPayload.addAll(payload);
      _fragOpcode = opcode;
      return null;
    }
    return WsFrameMessage(opcode, payload);
  }
}

/// 构造客户端→服务端帧(必须掩码)。opcode:0x1 文本 0x8 close 0x9 ping 0xA pong。
@visibleForTesting
Uint8List encodeClientFrame(List<int> payload, int opcode, {bool fin = true}) {
  final rnd = Random.secure();
  final mask = List<int>.generate(4, (_) => rnd.nextInt(256));
  final n = payload.length;

  final head = <int>[(fin ? 0x80 : 0x00) | opcode];
  if (n < 126) {
    head.add(0x80 | n);
  } else if (n <= 0xFFFF) {
    head.add(0x80 | 126);
    head..add((n >> 8) & 0xFF)..add(n & 0xFF);
  } else {
    head.add(0x80 | 127);
    for (var i = 7; i >= 0; i--) {
      head.add((n >> (8 * i)) & 0xFF);
    }
  }
  head.addAll(mask);

  final out = Uint8List(head.length + n);
  out.setAll(0, head);
  for (var i = 0; i < n; i++) {
    out[head.length + i] = payload[i] ^ mask[i % 4];
  }
  return out;
}
