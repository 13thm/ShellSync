import 'package:flutter_test/flutter_test.dart';
import 'package:shellsync_mobile/config/storage.dart';
import 'package:shellsync_mobile/stores/pair_target.dart';

void main() {
  group('PairTarget.parse', () {
    test('v2 payload (lan+cloud+dev)', () {
      final t = PairTarget.parse(
          'shellsync://pair?v=2&code=482913&lan=192.168.1.5:8787&cloud=relay.example.com&dev=a3f9c2');
      expect(t, isNotNull);
      expect(t!.lan, '192.168.1.5:8787');
      expect(t.cloud, 'relay.example.com');
      expect(t.devId, 'a3f9c2');
      expect(t.code, '482913');
      expect(t.hasCloud, isTrue);
    });

    test('v2 without cloud (daemon cloud offline)', () {
      final t = PairTarget.parse(
          'shellsync://pair?v=2&code=111222&lan=192.168.1.5:8787&dev=x');
      expect(t, isNotNull);
      expect(t!.cloud, isNull);
      expect(t.hasCloud, isFalse);
      expect(t.lan, '192.168.1.5:8787');
    });

    test('v1 payload (ip/port/code)', () {
      final t = PairTarget.parse(
          'shellsync://pair?ip=192.168.1.5&port=8787&code=111222');
      expect(t, isNotNull);
      expect(t!.lan, '192.168.1.5:8787');
      expect(t.cloud, isNull);
      expect(t.devId, isNull);
      expect(t.code, '111222');
    });

    test('non-pair QR / junk', () {
      expect(PairTarget.parse('https://example.com/?code=1'), isNull);
      expect(PairTarget.parse('shellsync://other?code=1'), isNull);
      expect(PairTarget.parse('shellsync://pair?v=2'), isNull); // no code
      expect(PairTarget.parse('shellsync://pair?ip=1.2.3.4'), isNull); // no port
    });
  });

  group('relayWsUrl', () {
    test('host with port → plain ws (dev/staging)', () {
      expect(relayWsUrl('192.168.1.5:8788'), 'ws://192.168.1.5:8788/ws');
    });

    test('bare host → wss (prod)', () {
      expect(relayWsUrl('relay.example.com'), 'wss://relay.example.com/ws');
    });
  });
}
