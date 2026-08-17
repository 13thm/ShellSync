import 'package:flutter/material.dart';
import 'package:mobile_scanner/mobile_scanner.dart';
import 'package:provider/provider.dart';
import '../stores/app_state.dart';
import '../stores/pair_target.dart';

/// M4-2 配对页：首屏让用户选择「扫码」或「手动输入」，
/// 扫描 Desktop 生成的 shellsync://pair 二维码（v2 含 lan+cloud+dev，
/// v1 只有 ip/port），解析后先试局域网、失败走云隧道完成配对。
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
  final int _scanAttempt = 0; // 相机重建计数（key 用，保持稳定）
  MobileScannerController? _scannerCtrl; // 显式 controller：进入扫码时创建，退出时销毁

  final _ipCtrl = TextEditingController();
  final _portCtrl = TextEditingController();
  final _codeCtrl = TextEditingController();

  @override
  void dispose() {
    _scannerCtrl?.dispose();
    _scannerCtrl = null;
    _ipCtrl.dispose();
    _portCtrl.dispose();
    _codeCtrl.dispose();
    super.dispose();
  }

  Future<void> _doPair(PairTarget target) async {
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      await context.read<AppState>().pairWith(target);
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
      final parsed = PairTarget.parse(raw);
      if (parsed != null) {
        _doPair(parsed);
        return;
      }
    }
  }

  /// 进入扫码模式：创建 controller（禁用自动启动）并显式 start。
  Future<void> _enterScan() async {
    final ctrl = MobileScannerController(autoStart: false);
    _scannerCtrl = ctrl;
    setState(() => _mode = _PairMode.scan);
    try {
      await ctrl.start(); // 显式启动相机（含权限请求）
    } catch (e) {
      if (mounted) {
        setState(() => _error = '相机启动失败：$e');
      }
    }
  }

  void _backToChooser() {
    _scannerCtrl?.dispose();
    _scannerCtrl = null;
    setState(() {
      _mode = _PairMode.chooser;
      _error = null;
    });
  }

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
            onPressed: _enterScan,
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
        MobileScanner(
          key: ValueKey(_scanAttempt),
          controller: _scannerCtrl,
          onDetect: _onDetect,
          errorBuilder: (context, error) {
            return Center(
              child: Padding(
                padding: const EdgeInsets.all(24),
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    const Icon(Icons.no_photography, size: 48, color: Color(0xFF86909C)),
                    const SizedBox(height: 12),
                    Text(
                      '相机无法启动：${error.errorCode.name}\n${error.errorDetails?.message ?? "未知原因"}\n请在系统设置中授予相机权限，\n或改用手动输入配对信息',
                      textAlign: TextAlign.center,
                      style: const TextStyle(color: Color(0xFF86909C), fontSize: 13, height: 1.6),
                    ),
                    const SizedBox(height: 16),
                    FilledButton(
                      onPressed: () {
                        _scannerCtrl?.dispose();
                        _enterScan(); // 完全重建相机
                      },
                      child: const Text('重试相机'),
                    ),
                    const SizedBox(height: 8),
                    TextButton(
                      onPressed: () => setState(() => _mode = _PairMode.manual),
                      child: const Text('改用手动输入'),
                    ),
                  ],
                ),
              ),
            );
          },
        ),
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
              _doPair(PairTarget(lan: '$ip:$port', code: code));
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
