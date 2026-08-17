import 'dart:convert';

import 'package:dio/dio.dart';

import 'relay_http.dart';

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

/// 隧道请求器:把一次 HTTP 请求送进云隧道(RelayHttpClient.request 的签名)。
typedef TunnelRequester = Future<TunnelHttpResponse> Function(
  String method,
  String pathAndQuery, {
  Map<String, String>? headers,
  List<int>? body,
});

/// REST client that auto-unwraps the envelope and injects the bearer token.
/// [tunnel] 为空走 dio(局域网直连);非空则走云隧道 —— 上层 API 完全一致。
class RestClient {
  RestClient(String baseUrl, String token, {TunnelRequester? tunnel})
      : _tunnel = tunnel,
        _authHeader = 'Bearer $token',
        _dio = Dio(BaseOptions(
          baseUrl: baseUrl,
          connectTimeout: const Duration(seconds: 8),
          receiveTimeout: const Duration(seconds: 15),
          headers: {'Authorization': 'Bearer $token'},
          responseType: ResponseType.json,
        )) {
    if (tunnel == null) {
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
  }

  final Dio _dio;
  final TunnelRequester? _tunnel;
  final String _authHeader;

  Future<dynamic> get(String path, {Map<String, dynamic>? query}) =>
      _request('GET', path, query: query);

  Future<dynamic> post(String path, [dynamic data]) =>
      _request('POST', path, data: data);

  Future<dynamic> patch(String path, [dynamic data]) =>
      _request('PATCH', path, data: data);

  Future<dynamic> delete(String path, {Map<String, dynamic>? query}) =>
      _request('DELETE', path, query: query);

  Future<dynamic> _request(String method, String path,
      {Map<String, dynamic>? query, dynamic data}) async {
    if (_tunnel == null) {
      final r = await _dio.request<dynamic>(path,
          data: data, queryParameters: query);
      return r.data;
    }

    // ---- cloud path: HTTP over tunnel, manual envelope unwrap ----
    var pq = path;
    if (query != null && query.isNotEmpty) {
      final qs = query.entries
          .map((e) =>
              '${Uri.encodeComponent(e.key)}=${Uri.encodeComponent('${e.value}')}')
          .join('&');
      pq = '$pq${pq.contains('?') ? '&' : '?'}$qs';
    }
    final resp = await _tunnel(method, pq,
        headers: {'Authorization': _authHeader},
        body: data == null ? null : utf8.encode(jsonEncode(data)));
    return _unwrapHttp(resp);
  }

  static dynamic _unwrapHttp(TunnelHttpResponse resp) {
    if (resp.status == 401) {
      throw ApiError(40001, 'unauthorized');
    }
    if (resp.status >= 400) {
      throw ApiError(resp.status, 'HTTP ${resp.status}');
    }
    final parsed = resp.json();
    if (parsed is Map<String, dynamic> && parsed.containsKey('code')) {
      final env = _Envelope.fromJson(parsed);
      if (env.code != 0) {
        throw ApiError(env.code, env.message);
      }
      return env.data;
    }
    return parsed;
  }

  // ---- pairing (public endpoints; optional cloud tunnel) ----

  static Future<Map<String, dynamic>> pairInit(String baseUrl,
      {TunnelRequester? tunnel, Duration timeout = const Duration(seconds: 8)}) async {
    return (await _pairCall(tunnel, baseUrl, 'POST', '/api/pair/init', timeout))
        as Map<String, dynamic>;
  }

  static Future<Map<String, dynamic>> pairVerify(
    String baseUrl,
    String code,
    String deviceName,
    String platform, {
    TunnelRequester? tunnel,
    Duration timeout = const Duration(seconds: 8),
  }) async {
    return (await _pairCall(
            tunnel,
            baseUrl,
            'POST',
            '/api/pair/verify',
            timeout,
            {'pairingCode': code, 'deviceName': deviceName, 'platform': platform}))
        as Map<String, dynamic>;
  }

  static Future<dynamic> _pairCall(TunnelRequester? tunnel, String baseUrl,
      String method, String path, Duration timeout,
      [Map<String, dynamic>? data]) async {
    if (tunnel != null) {
      final resp = await tunnel(method, path,
          body: data == null ? null : utf8.encode(jsonEncode(data)));
      return _unwrapHttp(resp);
    }
    final dio = Dio(BaseOptions(
      baseUrl: baseUrl,
      connectTimeout: timeout,
      receiveTimeout: timeout,
    ));
    final r = await dio.post(path, data: data);
    final raw = r.data;
    if (raw is Map<String, dynamic> && raw['code'] == 0) return raw['data'];
    throw ApiError(
        raw is Map<String, dynamic> ? (raw['code'] as int? ?? -1) : -1,
        raw is Map<String, dynamic> ? '${raw['message']}' : 'bad response');
  }
}
