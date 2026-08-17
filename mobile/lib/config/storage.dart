import 'package:flutter_secure_storage/flutter_secure_storage.dart';

/// 配对凭证(v2):局域网地址 + 云中继 + 设备 ID + token。
/// 云字段为空 = 纯局域网配对(v1 兼容)。
class Credentials {
  Credentials({
    this.lanEndpoint, // http://192.168.1.5:8787
    this.cloudHost, // relay.example.com 或 192.168.1.5:8788
    this.devId,
    required this.token,
  });

  final String? lanEndpoint;
  final String? cloudHost;
  final String? devId;
  final String token;

  bool get hasCloud => cloudHost != null && cloudHost!.isNotEmpty;
  bool get hasLan => lanEndpoint != null && lanEndpoint!.isNotEmpty;
}

/// cloud host(二维码 cloud 字段)→ relay WS 地址。
/// 带端口 → 明文 ws(开发/staging);无端口 → wss(生产,443 由 Caddy 终结)。
String relayWsUrl(String cloudHost) {
  if (cloudHost.contains(':')) return 'ws://$cloudHost/ws';
  return 'wss://$cloudHost/ws';
}

/// Persists paired credentials + debug overrides securely.
class SecureStore {
  const SecureStore();
  static const _endpointKey = 'shellsync.endpoint'; // v1 legacy (lan)
  static const _lanKey = 'shellsync.lan';
  static const _cloudKey = 'shellsync.cloud';
  static const _devIdKey = 'shellsync.devId';
  static const _tokenKey = 'shellsync.token';
  static const _devRelayKey = 'shellsync.dev.relayUrl';
  static const _storage = FlutterSecureStorage();

  Future<Credentials?> read() async {
    final tok = await _storage.read(key: _tokenKey);
    if (tok == null || tok.isEmpty) return null;
    final lan = await _storage.read(key: _lanKey);
    final cloud = await _storage.read(key: _cloudKey);
    final devId = await _storage.read(key: _devIdKey);

    var lanEndpoint = (lan != null && lan.isNotEmpty) ? lan : null;
    if (lanEndpoint == null) {
      // v1 → v2 迁移:旧 endpoint 即局域网地址
      final legacy = await _storage.read(key: _endpointKey);
      if (legacy != null && legacy.isNotEmpty) {
        lanEndpoint = legacy;
        await _storage.write(key: _lanKey, value: legacy);
      }
    }
    if (lanEndpoint == null && (cloud == null || cloud.isEmpty)) return null;
    return Credentials(
      lanEndpoint: lanEndpoint,
      cloudHost: (cloud != null && cloud.isNotEmpty) ? cloud : null,
      devId: (devId != null && devId.isNotEmpty) ? devId : null,
      token: tok,
    );
  }

  Future<void> write(Credentials c) async {
    await _storage.write(key: _lanKey, value: c.lanEndpoint ?? '');
    await _storage.write(key: _cloudKey, value: c.cloudHost ?? '');
    await _storage.write(key: _devIdKey, value: c.devId ?? '');
    await _storage.write(key: _tokenKey, value: c.token);
    // 旧键清理(值留在 lan 里)
    await _storage.delete(key: _endpointKey);
  }

  Future<void> clear() async {
    for (final k in [
      _endpointKey,
      _lanKey,
      _cloudKey,
      _devIdKey,
      _tokenKey,
    ]) {
      await _storage.delete(key: k);
    }
  }

  /// 开发者 relay 地址覆盖(debug 构建的设置页入口)。
  Future<String?> devRelayOverride() => _storage.read(key: _devRelayKey);

  Future<void> setDevRelayOverride(String? value) async {
    if (value == null || value.isEmpty) {
      await _storage.delete(key: _devRelayKey);
    } else {
      await _storage.write(key: _devRelayKey, value: value);
    }
  }
}
