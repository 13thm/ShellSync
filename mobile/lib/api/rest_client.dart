import 'package:dio/dio.dart';

/// Daemon unified response envelope {code, data, message}.
class _Envelope {
  final int code;
  final dynamic data;
  final String message;
  _Envelope(this.code, this.data, this.message);

  factory _Envelope.fromJson(Map<String, dynamic> j) =>
      _Envelope(j['code'] ?? -1, j['data'], j['message'] ?? '');
}

/// Thrown when the daemon returns a non-zero code.
class ApiError implements Exception {
  final int code;
  final String message;
  ApiError(this.code, this.message);
  @override
  String toString() => 'ApiError($code): $message';
}

/// REST client that auto-unwraps the envelope and injects the bearer token.
class RestClient {
  RestClient(String baseUrl, String token)
      : _dio = Dio(BaseOptions(
          baseUrl: baseUrl,
          connectTimeout: const Duration(seconds: 8),
          receiveTimeout: const Duration(seconds: 15),
          headers: {'Authorization': 'Bearer $token'},
          responseType: ResponseType.json,
        )) {
    _dio.interceptors.add(
      InterceptorsWrapper(
        onResponse: (response, handler) {
          final raw = response.data;
          if (raw is Map<String, dynamic> && raw.containsKey('code')) {
            final env = _Envelope.fromJson(raw);
            if (env.code == 0) {
              response.data = env.data;
              return handler.next(response);
            }
            return handler.reject(
              DioException(
                requestOptions: response.requestOptions,
                message: env.message,
                error: ApiError(env.code, env.message),
              ),
              true,
            );
          }
          handler.next(response);
        },
      ),
    );
  }

  final Dio _dio;

  Future<dynamic> get(String path, {Map<String, dynamic>? query}) =>
      _dio.get(path, queryParameters: query).then((r) => r.data);

  Future<dynamic> post(String path, [dynamic data]) =>
      _dio.post(path, data: data).then((r) => r.data);

  Future<dynamic> patch(String path, [dynamic data]) =>
      _dio.patch(path, data: data).then((r) => r.data);

  Future<dynamic> delete(String path) => _dio.delete(path).then((r) => r.data);

  /// Pairing endpoints are public (no token), so a static helper is used.
  static Future<Map<String, dynamic>> pairInit(String baseUrl) async {
    final dio = Dio(BaseOptions(baseUrl: baseUrl));
    final r = await dio.post('/api/pair/init');
    final env = _Envelope.fromJson(r.data as Map<String, dynamic>);
    return env.data as Map<String, dynamic>;
  }

  static Future<Map<String, dynamic>> pairVerify(
    String baseUrl,
    String code,
    String deviceName,
    String platform,
  ) async {
    final dio = Dio(BaseOptions(baseUrl: baseUrl));
    final r = await dio.post('/api/pair/verify', data: {
      'pairingCode': code,
      'deviceName': deviceName,
      'platform': platform,
    });
    final env = _Envelope.fromJson(r.data as Map<String, dynamic>);
    return env.data as Map<String, dynamic>;
  }
}
