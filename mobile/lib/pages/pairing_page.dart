import 'package:flutter/material.dart';
import 'package:mobile_scanner/mobile_scanner.dart';
import 'package:provider/provider.dart';
import '../stores/app_state.dart';

/// M4-2 扫码配对页：扫描 Desktop 生成的 shellsync://pair?ip=&port=&code= 二维码，
/// 解析后调用 /api/pair/verify 换取 session token 并持久化。
/// 提供手动输入兜底（开发机无摄像头时）。
class PairingPage extends StatefulWidget {
  const PairingPage({super.key});

  @override
  State<PairingPage> createState() => _PairingPageState();
}

class _PairingPageState extends State<PairingPage> {
  bool _manual = false;
  bool _busy = false;
  String? _error;

  final _ipCtrl = TextEditingController();
  final _portCtrl = TextEditingController();
  final _codeCtrl = TextEditingController();

  @override
  void dispose() {
    _ipCtrl.dispose();
    _portCtrl.dispose();
    _codeCtrl.dispose();
    super.dispose();
  }

  Future<void> _doPair({required String ip, required int port, required String code}) async {
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      await context.read<AppState>().pair(ip: ip, port: port, code: code);
      // AppState.paired flips -> app routes to HomePage automatically
    } catch (e) {
      setState(() => _error = e.toString());
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  void _onDetect(BarcodeCapture capture) {
    if (_busy) return;
    for (final b in capture.barcodes) {
      final raw = b.rawValue;
      if (raw == null) continue;
      final parsed = _parsePairUri(raw);
      if (parsed != null) {
        _doPair(ip: parsed.$1, port: parsed.$2, code: parsed.$3);
        return;
      }
    }
  }

  static (String, int, String)? _parsePairUri(String raw) {
    final uri = Uri.tryParse(raw);
    if (uri == null || uri.scheme != 'shellsync' || uri.host != 'pair') return null;
    final ip = uri.queryParameters['ip'];
    final portStr = uri.queryParameters['port'];
    final code = uri.queryParameters['code'];
    final port = portStr == null ? null : int.tryParse(portStr);
    if (ip == null || port == null || code == null) return null;
    return (ip, port, code);
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('配对 ShellSync')),
      body: _busy
          ? const Center(child: CircularProgressIndicator())
          : _manual
              ? _manualForm()
              : _scannerView(),
    );
  }

  Widget _scannerView() {
    return Stack(
      children: [
        MobileScanner(onDetect: _onDetect),
        Positioned(
          left: 0,
          right: 0,
          bottom: 24,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              if (_error != null)
                Padding(
                  padding: const EdgeInsets.symmetric(horizontal: 24),
                  child: Text(_error!, style: const TextStyle(color: Color(0xFFE45656))),
                ),
              TextButton(
                onPressed: () => setState(() => _manual = true),
                child: const Text('手动输入 IP / 端口 / 配对码'),
              ),
            ],
          ),
        ),
      ],
    );
  }

  Widget _manualForm() {
    return Padding(
      padding: const EdgeInsets.all(20),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          const Text('输入电脑端「设置 → 配对」显示的信息', style: TextStyle(color: Color(0xFF4E5969))),
          const SizedBox(height: 16),
          TextField(
              controller: _ipCtrl,
              decoration: const InputDecoration(labelText: '电脑 IP', border: OutlineInputBorder())),
          const SizedBox(height: 12),
          TextField(
              controller: _portCtrl,
              keyboardType: TextInputType.number,
              decoration: const InputDecoration(labelText: '端口', border: OutlineInputBorder())),
          const SizedBox(height: 12),
          TextField(
              controller: _codeCtrl,
              decoration: const InputDecoration(labelText: '配对码', border: OutlineInputBorder())),
          const SizedBox(height: 20),
          FilledButton(
            onPressed: () {
              final ip = _ipCtrl.text.trim();
              final port = int.tryParse(_portCtrl.text.trim());
              final code = _codeCtrl.text.trim();
              if (ip.isEmpty || port == null || code.isEmpty) return;
              _doPair(ip: ip, port: port, code: code);
            },
            child: const Text('配对'),
          ),
          if (_error != null)
            Padding(
              padding: const EdgeInsets.only(top: 12),
              child: Text(_error!, style: const TextStyle(color: Color(0xFFE45656))),
            ),
        ],
      ),
    );
  }
}
