import 'package:flutter_secure_storage/flutter_secure_storage.dart';

class Credentials {
  final String endpoint; // http://ip:port
  final String token; // session token
  Credentials(this.endpoint, this.token);
}

/// Persists the paired endpoint + session token securely.
class SecureStore {
  const SecureStore();
  static const _endpointKey = 'shellsync.endpoint';
  static const _tokenKey = 'shellsync.token';
  static const _storage = FlutterSecureStorage();

  Future<Credentials?> read() async {
    final ep = await _storage.read(key: _endpointKey);
    final tok = await _storage.read(key: _tokenKey);
    if (ep != null && ep.isNotEmpty && tok != null && tok.isNotEmpty) {
      return Credentials(ep, tok);
    }
    return null;
  }

  Future<void> write(Credentials c) async {
    await _storage.write(key: _endpointKey, value: c.endpoint);
    await _storage.write(key: _tokenKey, value: c.token);
  }

  Future<void> clear() async {
    await _storage.delete(key: _endpointKey);
    await _storage.delete(key: _tokenKey);
  }
}
