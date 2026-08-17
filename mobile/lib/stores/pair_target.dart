/// 二维码/手动输入解析出的配对目标(v1 与 v2 兼容)。
///
/// v2: shellsync://pair?v=2&code=482913&lan=192.168.1.5:8787&cloud=relay.example.com&dev=a3f9c2
/// v1: shellsync://pair?ip=192.168.1.5&port=8787&code=482913
class PairTarget {
  const PairTarget({this.lan, this.cloud, this.devId, required this.code});

  /// 局域网地址 "ip:port"(v1 的 ip+port 或 v2 的 lan)。
  final String? lan;

  /// 云中继 host[:port]。
  final String? cloud;

  /// daemon 设备 ID(日常连接直接 open 用)。
  final String? devId;

  final String code;

  bool get hasCloud => cloud != null && cloud!.isNotEmpty;

  /// 解析 shellsync://pair 二维码;非配对二维码返回 null。
  static PairTarget? parse(String raw) {
    final uri = Uri.tryParse(raw);
    if (uri == null || uri.scheme != 'shellsync' || uri.host != 'pair') {
      return null;
    }
    final q = uri.queryParameters;
    final code = q['code'];
    if (code == null || code.isEmpty) return null;

    final v = q['v'];
    if (v == '2') {
      return PairTarget(
        lan: _nonEmpty(q['lan']),
        cloud: _nonEmpty(q['cloud']),
        devId: _nonEmpty(q['dev']),
        code: code,
      );
    }
    // v1(以及缺 v 的旧码):ip/port
    final ip = q['ip'];
    final port = q['port'] == null ? null : int.tryParse(q['port']!);
    if (ip == null || port == null) return null;
    return PairTarget(lan: '$ip:$port', code: code);
  }

  static String? _nonEmpty(String? s) => (s == null || s.isEmpty) ? null : s;
}
