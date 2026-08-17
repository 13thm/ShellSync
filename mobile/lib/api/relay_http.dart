import 'dart:async';
import 'package:meta/meta.dart';
import 'dart:convert';
import 'dart:typed_data';

import 'relay_transport.dart';

/// 隧道内极简 HTTP/1.1 客户端:把请求写成字节发进虚拟流,按
/// Content-Length / 读到流结束收响应。只覆盖项目用到的子集
/// (JSON GET/POST/PATCH/DELETE,无文件上传、无重定向)。
class TunnelHttpResponse {
  TunnelHttpResponse({
    required this.status,
    required this.reason,
    required this.headers,
    required this.body,
  });

  final int status;
  final String reason;
  final Map<String, String> headers; // 小写键
  final Uint8List body;

  bool get ok => status >= 200 && status < 300;

  dynamic json() {
    final text = utf8.decode(body, allowMalformed: true);
    if (text.isEmpty) return null;
    return jsonDecode(text);
  }

  @override
  String toString() => 'TunnelHttpResponse($status $reason, ${body.length}B)';
}

class RelayHttpException implements Exception {
  RelayHttpException(this.message);
  final String message;
  @override
  String toString() => 'RelayHttpException: $message';
}

class RelayHttpClient {
  RelayHttpClient(this.conn, this.devId);

  final RelayConnection conn;
  final String? devId;

  /// 发起一次 HTTP 请求:每请求一条隧道流,完成即关流。
  Future<TunnelHttpResponse> request(
    String method,
    String pathAndQuery, {
    Map<String, String>? headers,
    List<int>? body,
    Duration timeout = const Duration(seconds: 15),
  }) async {
    final stream = await conn.open(devId);

    final h = <String, String>{
      'Host': 'shellsync',
      'Connection': 'close',
      ...?headers,
    };
    if (body != null && body.isNotEmpty) {
      h['Content-Length'] = '${body.length}';
      h.putIfAbsent('Content-Type', () => 'application/json');
    }
    final req = StringBuffer()
      ..write(method.toUpperCase())
      ..write(' ')
      ..write(pathAndQuery)
      ..write(' HTTP/1.1\r\n');
    h.forEach((k, v) => req..write(k)..write(': ')..write(v)..write('\r\n'));
    req.write('\r\n');

    final bytes = <int>[...utf8.encode(req.toString())];
    if (body != null) bytes.addAll(body);
    stream.send(bytes);

    final collector = TunnelHttpParser();
    final complete = Completer<void>();
    final sub = stream.incoming.listen((chunk) {
      collector.add(chunk);
      if (collector.isComplete && !complete.isCompleted) complete.complete();
    });
    unawaited(stream.done.whenComplete(() {
      if (!complete.isCompleted) complete.complete();
    }));

    try {
      await complete.future.timeout(timeout,
          onTimeout: () => throw TimeoutException('tunnel http timeout'));
      return collector.finish();
    } finally {
      await sub.cancel();
      if (!stream.isClosed) {
        unawaited(stream.close(why: 'http done'));
      }
    }
  }
}

/// 增量解析 HTTP/1.1 响应:头齐后按 Content-Length 截体;无长度则读到 EOF。
/// 公开仅为单元测试(@visibleForTesting)。
@visibleForTesting
class TunnelHttpParser {
  final _buf = <int>[];
  TunnelHttpResponse? _done;
  bool _headersParsed = false;
  int _status = 0;
  String _reason = '';
  Map<String, String> _headers = {};
  int _bodyStart = 0;

  bool get isComplete => _done != null;

  void add(List<int> chunk) {
    if (isComplete) return;
    _buf.addAll(chunk);
    if (!_headersParsed) {
      _tryParseHeaders();
    }
    if (_headersParsed) {
      final cl = int.tryParse(_headers['content-length'] ?? '');
      if (cl != null && _buf.length - _bodyStart >= cl) {
        _done = TunnelHttpResponse(
          status: _status,
          reason: _reason,
          headers: _headers,
          body: Uint8List.fromList(
              _buf.sublist(_bodyStart, _bodyStart + cl)),
        );
      }
    }
  }

  void _tryParseHeaders() {
    // find \r\n\r\n in byte buffer
    int sep = -1;
    for (var i = 0; i + 3 < _buf.length; i++) {
      if (_buf[i] == 13 && _buf[i + 1] == 10 && _buf[i + 2] == 13 && _buf[i + 3] == 10) {
        sep = i;
        break;
      }
    }
    if (sep < 0) return;
    final head = ascii.decode(_buf.sublist(0, sep), allowInvalid: true);
    final lines = const LineSplitter().convert(head);
    if (lines.isEmpty) return;
    final statusLine = lines.first.split(' ');
    if (statusLine.length < 2) return;
    final status = int.tryParse(statusLine[1]);
    if (status == null) return;

    final headers = <String, String>{};
    for (final line in lines.skip(1)) {
      final i = line.indexOf(':');
      if (i > 0) {
        headers[line.substring(0, i).trim().toLowerCase()] =
            line.substring(i + 1).trim();
      }
    }
    _status = status;
    _reason = statusLine.length > 2 ? statusLine.sublist(2).join(' ') : '';
    _headers = headers;
    _bodyStart = sep + 4;
    _headersParsed = true;
  }

  TunnelHttpResponse finish() {
    if (_done != null) return _done!;
    if (!_headersParsed) {
      throw RelayHttpException('incomplete HTTP response');
    }
    return TunnelHttpResponse(
      status: _status,
      reason: _reason,
      headers: _headers,
      body: Uint8List.fromList(_buf.sublist(_bodyStart)),
    );
  }
}
