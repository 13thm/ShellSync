import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:xterm/xterm.dart' as xterm;
import '../api/ws_client.dart';

/// Terminal pane backed by the xterm engine, wired to the daemon WS.
///
/// 尺寸策略（tmux attach 模式）：PTY 网格跟随「当前观看/操作的客户端」。
/// - 手机进入终端页 → 上报手机尺寸，PTY/TTY 重绘适配手机（TUI 不错位）；
/// - daemon 广播 terminal.size → 桌面端同步对齐自己的网格，两端显示一致；
/// - 双指缩放字号，输入栏发送命令。
class TerminalPane extends StatefulWidget {
  const TerminalPane({required this.terminalId, required this.ws, super.key});
  final String terminalId;
  final WsClient ws;

  @override
  State<TerminalPane> createState() => _TerminalPaneState();
}

class _TerminalPaneState extends State<TerminalPane> {
  late final xterm.Terminal _terminal;
  final _subs = <void Function()>[];
  final _inputCtrl = TextEditingController();
  bool _showInputBar = true;
  double _fontSize = 13;
  double _scaleStartFont = 13;

  static const _theme = xterm.TerminalTheme(
    background: Color(0xFFFFFFFF),
    foreground: Color(0xFF1F2329),
    selection: Color(0xFFEBF5EF),
    black: Color(0xFF1F2329),
    red: Color(0xFFE45656),
    green: Color(0xFF3BA776),
    yellow: Color(0xFFE0A13C),
    blue: Color(0xFF4B8AF0),
    magenta: Color(0xFFA05EB5),
    cyan: Color(0xFF3BA776),
    white: Color(0xFFC0C4CC),
    brightBlack: Color(0xFF8A909A),
    brightRed: Color(0xFFE45656),
    brightGreen: Color(0xFF3BA776),
    brightYellow: Color(0xFFE0A13C),
    brightBlue: Color(0xFF4B8AF0),
    brightMagenta: Color(0xFFA05EB5),
    brightCyan: Color(0xFF3BA776),
    brightWhite: Color(0xFF1F2329),
    searchHitBackground: Color(0xFFEBF5EF),
    searchHitBackgroundCurrent: Color(0xFFEBF5EF),
    searchHitForeground: Color(0xFF1F2329),
    cursor: Color(0xFF3BA776),
  );

  void _sendInput(String data) {
    // shell 的回车是 CR；移动端键盘/输入法产生的是 LF
    widget.ws.send('terminal.input', {
      'terminalId': widget.terminalId,
      'dataB64': base64Encode(utf8.encode(data.replaceAll('\n', '\r'))),
    });
  }

  void _sendResize(int cols, int rows) {
    widget.ws.send('terminal.resize', {
      'terminalId': widget.terminalId,
      'cols': cols,
      'rows': rows,
    });
  }

  @override
  void initState() {
    super.initState();
    _terminal = xterm.Terminal(maxLines: 8000);

    // 手机端接管尺寸：视图尺寸变化即上报（PTY 跟随手机，TUI 按手机网格重绘）
    _terminal.onResize = (width, height, _, __) {
      if (width > 0 && height > 0) _sendResize(width, height);
    };
    _terminal.onOutput = (data) => _sendInput(data);

    final ws = widget.ws;
    _subs.add(ws.on('terminal.output', (m) {
      final p = (m['payload'] ?? {}) as Map;
      if (p['terminalId'] != widget.terminalId) return;
      if (p['direction'] != null && p['direction'] != 'stdout') return;
      _terminal.write(utf8.decode(base64Decode(p['contentB64'])));
    }));
    _subs.add(ws.on('terminal.history', (m) {
      final p = (m['payload'] ?? {}) as Map;
      if (p['terminalId'] != widget.terminalId) return;
      for (final c in (p['chunks'] as List? ?? [])) {
        final chunk = c as Map;
        if (chunk['direction'] != null && chunk['direction'] != 'stdout') continue;
        _terminal.write(utf8.decode(base64Decode(chunk['contentB64'])));
      }
    }));
    // 其他端（桌面）调整窗口时，daemon 广播 size：手机对齐相同网格，
    // 保证两端渲染坐标系一致（tmux attach 语义）。
    _subs.add(ws.on('terminal.size', (m) {
      final p = (m['payload'] ?? {}) as Map;
      if (p['terminalId'] != widget.terminalId) return;
      final cols = (p['cols'] as num?)?.toInt() ?? 0;
      final rows = (p['rows'] as num?)?.toInt() ?? 0;
      // 主动对齐：临时摘掉 onResize 避免回声循环
      if (cols > 0 && rows > 0 &&
          (cols != _terminal.viewWidth || rows != _terminal.viewHeight)) {
        final orig = _terminal.onResize;
        _terminal.onResize = null;
        _terminal.resize(cols, rows);
        _terminal.onResize = orig;
      }
    }));

    // subscribe (server replies with terminal.history + live output)
    ws.request('terminal.subscribe', {'terminalId': widget.terminalId}).catchError((_) {});

    // 首帧布局完成后上报手机实际尺寸（接管 PTY）
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (!mounted) return;
      final cols = _terminal.viewWidth;
      final rows = _terminal.viewHeight;
      if (cols > 0 && rows > 0) _sendResize(cols, rows);
    });
  }

  @override
  void dispose() {
    for (final s in _subs) {
      s();
    }
    widget.ws.send('terminal.unsubscribe', {'terminalId': widget.terminalId});
    _inputCtrl.dispose();
    super.dispose();
  }

  void _submitInput() {
    final text = _inputCtrl.text;
    if (text.isEmpty) {
      _sendInput('\r'); // 空输入直接回车（重复上一条命令等场景）
    } else {
      _sendInput('$text\r');
    }
    _inputCtrl.clear();
  }

  @override
  Widget build(BuildContext context) {
    return Column(
      children: [
        Expanded(
          child: GestureDetector(
            onDoubleTap: () => setState(() => _showInputBar = !_showInputBar),
            // 双指缩放字号（单元格随之变小，列数自动变多并上报 PTY）
            onScaleStart: (_) => _scaleStartFont = _fontSize,
            onScaleUpdate: (details) {
              final s = (_scaleStartFont * details.scale).clamp(8.0, 28.0);
              if ((s - _fontSize).abs() > 0.1) setState(() => _fontSize = s);
            },
            child: xterm.TerminalView(
              _terminal,
              autofocus: true,
              theme: _theme,
              textStyle: xterm.TerminalStyle(
                fontSize: _fontSize,
                fontFamilyFallback: const [
                  'JetBrains Mono', 'Consolas', 'Courier New'
                ],
              ),
            ),
          ),
        ),
        if (_showInputBar) _buildInputBar(),
      ],
    );
  }

  Widget _buildInputBar() {
    return SafeArea(
      top: false,
      child: Container(
        color: Colors.white,
        padding: const EdgeInsets.fromLTRB(8, 6, 8, 6),
        child: Row(
          children: [
            Expanded(
              child: TextField(
                controller: _inputCtrl,
                style: const TextStyle(fontSize: 14),
                decoration: InputDecoration(
                  hintText: '输入命令，回车发送…',
                  isDense: true,
                  contentPadding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
                  border: OutlineInputBorder(
                    borderRadius: BorderRadius.circular(6),
                    borderSide: const BorderSide(color: Color(0xFFE5E6EB)),
                  ),
                  enabledBorder: OutlineInputBorder(
                    borderRadius: BorderRadius.circular(6),
                    borderSide: const BorderSide(color: Color(0xFFE5E6EB)),
                  ),
                  focusedBorder: OutlineInputBorder(
                    borderRadius: BorderRadius.circular(6),
                    borderSide: const BorderSide(color: Color(0xFF3BA776)),
                  ),
                ),
                onSubmitted: (_) => _submitInput(),
              ),
            ),
            const SizedBox(width: 6),
            _ctrlButton('C', '\x03'),
            const SizedBox(width: 6),
            FilledButton(
              style: FilledButton.styleFrom(
                minimumSize: const Size(52, 42),
                padding: EdgeInsets.zero,
              ),
              onPressed: _submitInput,
              child: const Icon(Icons.keyboard_return, size: 20),
            ),
          ],
        ),
      ),
    );
  }

  Widget _ctrlButton(String label, String data) {
    return OutlinedButton(
      style: OutlinedButton.styleFrom(
        minimumSize: const Size(44, 42),
        padding: EdgeInsets.zero,
        foregroundColor: const Color(0xFF4E5969),
        side: const BorderSide(color: Color(0xFFE5E6EB)),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(6)),
      ),
      onPressed: () => _sendInput(data),
      child: Text(label, style: const TextStyle(fontSize: 13)),
    );
  }
}
