import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:shellsync_mobile/api/relay_http.dart';
import 'package:shellsync_mobile/api/ws_channel.dart';

void main() {
  group('TunnelHttpParser', () {
    test('completes on Content-Length', () {
      final c = TunnelHttpParser();
      c.add(_bytes(
          'HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: 7\r\n\r\n'));
      expect(c.isComplete, isFalse);
      c.add(_bytes('{"a"'));
      expect(c.isComplete, isFalse);
      c.add(_bytes(':1}EXTRA'));
      expect(c.isComplete, isTrue);
      final r = c.finish();
      expect(r.status, 200);
      expect(r.reason, 'OK');
      expect(r.headers['content-type'], 'application/json');
      expect(r.body, '{"a":1}'.codeUnits);
      expect(r.json(), {'a': 1});
    });

    test('no Content-Length: finish at EOF', () {
      final c = TunnelHttpParser();
      c.add(_bytes('HTTP/1.1 404 Not Found\r\n\r\nmissing'));
      expect(c.isComplete, isFalse);
      final r = c.finish();
      expect(r.status, 404);
      expect(r.ok, isFalse);
      expect(utf8Of(r.body), 'missing');
    });

    test('split across single bytes', () {
      final c = TunnelHttpParser();
      const raw = 'HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nhi';
      for (final b in _bytes(raw)) {
        c.add(Uint8List.fromList([b]));
      }
      expect(c.isComplete, isTrue);
      expect(utf8Of(c.finish().body), 'hi');
    });

    test('headers only arrive in pieces', () {
      final c = TunnelHttpParser();
      c.add(_bytes('HTTP/1.1 200 OK\r\n'));
      c.add(_bytes('Content-Length: 0\r\n'));
      expect(c.isComplete, isFalse);
      c.add(_bytes('\r\n'));
      expect(c.isComplete, isTrue);
      expect(c.finish().body, isEmpty);
    });
  });

  group('WS client frame codec', () {
    test('text frame roundtrip', () {
      final frame = encodeClientFrame('hello ws'.codeUnits, 0x1);
      final parser = WsFrameParser();
      final msgs = parser.feed(frame).toList();
      expect(msgs.length, 1);
      expect(msgs.first.opcode, 0x1);
      expect(msgs.first.payload, 'hello ws'.codeUnits);
    });

    test('fragmented frames reassemble', () {
      final parser = WsFrameParser();
      final f1 = encodeClientFrame('foo'.codeUnits, 0x1, fin: false);
      final f2 = encodeClientFrame('bar'.codeUnits, 0x0, fin: true);
      var msgs = parser.feed(f1).toList();
      expect(msgs, isEmpty);
      msgs = parser.feed(f2).toList();
      expect(msgs.length, 1);
      expect(msgs.first.payload, 'foobar'.codeUnits);
      expect(msgs.first.opcode, 0x1);
    });

    test('extended length (126)', () {
      final payload = List<int>.generate(300, (i) => i % 251);
      final frame = encodeClientFrame(payload, 0x2);
      final msgs = WsFrameParser().feed(frame).toList();
      expect(msgs.single.payload, payload);
    });

    test('multiple frames in one chunk', () {
      final a = encodeClientFrame('a'.codeUnits, 0x1);
      final b = encodeClientFrame('b'.codeUnits, 0x1);
      final chunk = Uint8List.fromList([...a, ...b]);
      final msgs = WsFrameParser().feed(chunk).toList();
      expect(msgs.length, 2);
      expect(msgs[0].payload, 'a'.codeUnits);
      expect(msgs[1].payload, 'b'.codeUnits);
    });

    test('close and ping opcodes pass through', () {
      final close = encodeClientFrame(const [], 0x8);
      final ping = encodeClientFrame('x'.codeUnits, 0x9);
      final msgs = WsFrameParser()
          .feed(Uint8List.fromList([...close, ...ping]))
          .toList();
      expect(msgs[0].opcode, 0x8);
      expect(msgs[1].opcode, 0x9);
    });
  });
}

String utf8Of(Uint8List b) => String.fromCharCodes(b);

Uint8List _bytes(String s) => Uint8List.fromList(s.codeUnits);


