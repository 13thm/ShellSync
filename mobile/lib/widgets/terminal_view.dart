import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:xterm/xterm.dart' as xterm;
import '../api/ws_client.dart';

/// Terminal pane backed by the xterm dart engine, wired to the daemon WS.
/// Loads full history on subscribe, streams live output, sends input + resize.
///
/// 移动端两个特殊处理：
/// 1. 软键盘/输入法（尤其中文）经 xterm 引擎不可靠 → 提供底部输入栏直接发送；
/// 2. 回车键产生 "\n"，而 shell 只认 "\r" → 发送前统一转换。
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

  // light theme to match the design system (§二 浅色，不搞暗黑科技感)
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

  @override
  void initState() {
    super.initState();
    _terminal = xterm.Terminal(maxLines: 8000);

    _terminal.onOutput = (data) => _sendInput(data);
    _terminal.onResize = (width, height, pixelWidth, pixelHeight) {
      widget.ws.send('terminal.resize', {
        'terminalId': widget.terminalId,
        'cols': width,
        'rows': height,
      });
    };

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

    // subscribe (server replies with terminal.history + live output)
    ws.request('terminal.subscribe', {'terminalId': widget.terminalId}).catchError((_) {});

    // 首帧布局完成后上报实际尺寸，避免 TUI（pi/claude 等）按错误的 80x24 渲染
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (!mounted) return;
      final cols = _terminal.viewWidth;
      final rows = _terminal.viewHeight;
      if (cols > 0 && rows > 0) {
        ws.send('terminal.resize', {
          'terminalId': widget.terminalId,
          'cols': cols,
          'rows': rows,
        });
      }
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
            // 点按终端区域收起输入栏，获得更大视野
            onDoubleTap: () => setState(() => _showInputBar = !_showInputBar),
            child: xterm.TerminalView(
              _terminal,
              autofocus: true,
              theme: _theme,
              textStyle: const xterm.TerminalStyle(fontSize: 12),
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
            // 常用控制键：Ctrl+C（中断）
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
