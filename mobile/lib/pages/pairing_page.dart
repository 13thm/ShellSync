import 'package:flutter/material.dart';
import 'package:mobile_scanner/mobile_scanner.dart';
import 'package:provider/provider.dart';
import '../stores/app_state.dart';

/// M4-2 配对页：首屏让用户选择「扫码」或「手动输入」，
/// 扫描 Desktop 生成的 shellsync://pair?ip=&port=&code= 二维码，
/// 解析后调用 /api/pair/verify 换取 session token 并持久化。
enum _PairMode { chooser, scan, manual }

class PairingPage extends StatefulWidget {
  const PairingPage({super.key});

  @override
  State<PairingPage> createState() => _PairingPageState();
}

class _PairingPageState extends State<PairingPage> {
  _PairMode _mode = _PairMode.chooser;
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

  void _backToChooser() => setState(() {
        _mode = _PairMode.chooser;
        _error = null;
      });

  @override
  Widget build(BuildContext context) {
    final Widget body;
    switch (_mode) {
      case _PairMode.chooser:
        body = _chooser();
      case _PairMode.scan:
        body = _scannerView();
      case _PairMode.manual:
        body = _manualForm();
    }
    return Scaffold(
      appBar: AppBar(
        title: const Text('配对 ShellSync'),
        leading: _mode == _PairMode.chooser
            ? null
            : IconButton(icon: const Icon(Icons.arrow_back), onPressed: _backToChooser),
      ),
      body: _busy ? const Center(child: CircularProgressIndicator()) : body,
    );
  }

  /// 首屏：让用户自己选择扫码还是手动输入。
  Widget _chooser() {
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 28),
      child: Column(
        mainAxisAlignment: MainAxisAlignment.center,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          const Icon(Icons.devices, size: 56, color: Color(0xFF3BA776)),
          const SizedBox(height: 12),
          const Text(
            '连接到电脑端',
            textAlign: TextAlign.center,
            style: TextStyle(fontSize: 20, fontWeight: FontWeight.w600, color: Color(0xFF1F2329)),
          ),
          const SizedBox(height: 6),
          const Text(
            '在电脑端 ShellSync「设置 → 生成配对码」后，\n扫描二维码或手动输入配对信息',
            textAlign: TextAlign.center,
            style: TextStyle(fontSize: 13, color: Color(0xFF86909C), height: 1.6),
          ),
          const SizedBox(height: 36),
          FilledButton.icon(
            style: FilledButton.styleFrom(
              padding: const EdgeInsets.symmetric(vertical: 14),
              shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(8)),
            ),
            onPressed: () => setState(() => _mode = _PairMode.scan),
            icon: const Icon(Icons.qr_code_scanner, size: 22),
            label: const Text('扫码配对', style: TextStyle(fontSize: 16)),
          ),
          const SizedBox(height: 12),
          OutlinedButton.icon(
            style: OutlinedButton.styleFrom(
              padding: const EdgeInsets.symmetric(vertical: 14),
              foregroundColor: const Color(0xFF1F2329),
              side: const BorderSide(color: Color(0xFFE5E6EB)),
              shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(8)),
            ),
            onPressed: () => setState(() => _mode = _PairMode.manual),
            icon: const Icon(Icons.keyboard, size: 22),
            label: const Text('手动输入 IP / 端口 / 配对码', style: TextStyle(fontSize: 16)),
          ),
          if (_error != null)
            Padding(
              padding: const EdgeInsets.only(top: 16),
              child: Text(
                _error!,
                textAlign: TextAlign.center,
                style: const TextStyle(color: Color(0xFFE45656), fontSize: 13),
              ),
            ),
        ],
      ),
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
                onPressed: () => setState(() => _mode = _PairMode.manual),
                child: const Text('改用手动输入'),
              ),
            ],
          ),
        ),
      ],
    );
  }

  Widget _manualForm() {
    return SingleChildScrollView(
      padding: const EdgeInsets.all(20),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          const Text('输入电脑端「设置 → 配对」显示的信息', style: TextStyle(color: Color(0xFF4E5969))),
          const SizedBox(height: 16),
          TextField(
            controller: _ipCtrl,
            keyboardType: TextInputType.url,
            decoration: const InputDecoration(labelText: '电脑 IP', border: OutlineInputBorder()),
          ),
          const SizedBox(height: 12),
          TextField(
            controller: _portCtrl,
            keyboardType: TextInputType.number,
            decoration: const InputDecoration(labelText: '端口', border: OutlineInputBorder()),
          ),
          const SizedBox(height: 12),
          TextField(
            controller: _codeCtrl,
            decoration: const InputDecoration(labelText: '配对码', border: OutlineInputBorder()),
          ),
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
