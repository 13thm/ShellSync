import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:shellsync_mobile/api/relay_protocol.dart';

void main() {
  group('RelayControl', () {
    test('roundtrip with fields', () {
      const raw = '{"t":"claim","code":"482913","devId":"a3f9"}';
      final f = RelayControl.decode(raw);
      expect(f.t, 'claim');
      expect(f.str('code'), '482913');
      expect(f.str('devId'), 'a3f9');
      expect(f.encode(), raw);
    });

    test('re-encode of constructed frame', () {
      final f = RelayControl('open', {'devId': 'x1', 'streamId': 7});
      final back = RelayControl.decode(f.encode());
      expect(back.t, 'open');
      expect(back.str('devId'), 'x1');
      expect(back.intOf('streamId'), 7);
    });

    test('rejects missing t / non-object', () {
      expect(() => RelayControl.decode('{"role":"client"}'),
          throwsA(isA<FormatException>()));
      expect(() => RelayControl.decode('[1,2]'), throwsA(isA<FormatException>()));
      expect(() => RelayControl.decode('nope'), throwsA(isA<FormatException>()));
    });
  });

  group('data frames', () {
    test('roundtrip empty and small payload', () {
      for (final payload in <List<int>>[[], [1, 2, 3], List.filled(1024, 65)]) {
        final b = encodeDataFrame(42, payload);
        final f = decodeDataFrame(b);
        expect(f.streamId, 42);
        expect(f.payload, payload);
      }
    });

    test('big streamId', () {
      final b = encodeDataFrame(0xFFFFFFFF, [9]);
      expect(decodeDataFrame(b).streamId, 0xFFFFFFFF);
    });

    test('rejects short frame', () {
      expect(() => decodeDataFrame(Uint8List(4)), throwsA(isA<FormatException>()));
    });

    test('rejects length mismatch', () {
      final b = encodeDataFrame(1, [1, 2, 3]);
      expect(() => decodeDataFrame(b.sublist(0, b.length - 1)),
          throwsA(isA<FormatException>()));
    });
  });
}
