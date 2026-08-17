import 'dart:convert';
import 'dart:typed_data';

/// Relay wire protocol (mirror of server/relay/frame.go).
///
/// 控制帧 = WS TEXT(JSON);数据帧 = WS BINARY [streamId:4B][len:4B][payload]。
class RelayControl {
  RelayControl(this.t, [Map<String, dynamic>? fields])
      : fields = fields ?? const {};

  /// 帧类型:hello / reg / code / claim / open / accept / close / error。
  final String t;
  final Map<String, dynamic> fields;

  String? str(String k) => fields[k] as String?;
  int? intOf(String k) => fields[k] is int ? fields[k] as int : null;

  factory RelayControl.decode(String raw) {
    final m = jsonDecode(raw);
    if (m is! Map<String, dynamic>) {
      throw const FormatException('relay control frame is not an object');
    }
    final t = m['t'];
    if (t is! String || t.isEmpty) {
      throw const FormatException('relay control frame missing t');
    }
    m.remove('t');
    return RelayControl(t, m);
  }

  String encode() => jsonEncode({'t': t, ...fields});

  @override
  String toString() => 'RelayControl($t, $fields)';
}

/// 单个数据分帧上限(与服务端/daemon 一致)。
const kRelayMaxChunk = 32 * 1024;

Uint8List encodeDataFrame(int streamId, List<int> payload) {
  final out = Uint8List(8 + payload.length);
  final bd = ByteData.sublistView(out);
  bd.setUint32(0, streamId, Endian.big);
  bd.setUint32(4, payload.length, Endian.big);
  out.setAll(8, payload);
  return out;
}

class DecodedDataFrame {
  const DecodedDataFrame(this.streamId, this.payload);
  final int streamId;
  final Uint8List payload;
}

DecodedDataFrame decodeDataFrame(List<int> raw) {
  if (raw.length < 8) {
    throw const FormatException('relay data frame too short');
  }
  final bd = ByteData.sublistView(Uint8List.fromList(raw));
  final id = bd.getUint32(0, Endian.big);
  final len = bd.getUint32(4, Endian.big);
  if (len != raw.length - 8) {
    throw const FormatException('relay data frame length mismatch');
  }
  return DecodedDataFrame(id, Uint8List.fromList(raw.sublist(8)));
}
