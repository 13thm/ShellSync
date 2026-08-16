import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:xterm/xterm.dart' as xterm;
import '../api/ws_client.dart';

/// Terminal pane backed by the xterm engine, wired to the daemon WS.
///
/// 尺寸策略（服务端网格优先）：手机【不再】上报自己的尺寸接管共享 PTY。
/// 之前 phone attach/detach 会把共享终端在 93↔47 列之间来回拽，
/// ConPTY 每次都按新宽度整屏重排重画，把屏幕上未滚走的内容原地覆盖，
/// 桌面实时观看与后续回放都出现「内容被吃」。
/// 现在手机始终按 daemon 下发的 terminal.size 对齐本地网格渲染，
/// 双指缩放字号适应 93 列宽度；手机键盘输入照常工作。
///
/// 输入栏（微信风格）：顶部拖拽条可上下拉动调节高度；展开按钮把输入栏
/// 放大成大面板进行多行编辑；多行内容发送时走 bracketed paste 通道，
/// pi / claude code 等会把整段识别为一次粘贴，不会被空格/换行拆开执行。
class TerminalPane extends StatefulWidget {
  const TerminalPane({required this.terminalId, required this.ws, super.key});
  final String terminalId;
  final WsClient ws;

  @override
  State<TerminalPane> createState() => TerminalPaneState();
}

class TerminalPaneState extends State<TerminalPane> {
  late xterm.Terminal _terminal;
  final _subs = <void Function()>[];
  final _inputCtrl = TextEditingController();
  bool _showInputBar = true;
  double _fontSize = 13;
  double _scaleStartFont = 13;

  // 输入栏高度（收起状态），可拖拽调节。最小高度容纳：拖拽条+单行输入+按钮行
  double _inputHeight = 110;
  // 是否展开成大面板
  bool _expanded = false;

  static const _minInputHeight = 110.0;

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

  /// 手机不再上报尺寸（见类注释）：共享 PTY 网格由桌面端决定，
  /// 手机按 daemon 广播的 terminal.size 对齐本地渲染网格。
  ///
  /// void _sendResize(int cols, int rows) —— 已移除

  /// 创建终端实例并接上输入/尺寸回调。
  void _wireTerminal() {
    _terminal = xterm.Terminal(maxLines: 10000);
    // 手机不上报尺寸（见类注释）。onOutput 只负责把按键发给 PTY。
    _terminal.onOutput = (data) => _sendInput(data);
  }

  /// 刷新会话：等同「返回终端列表再点进来」——重建本地终端并重新订阅，
  /// 服务端会重发全部历史。订阅/回调保持不变，无 unsubscribe 竞态。
  void refresh() {
    setState(() => _wireTerminal());
    widget.ws
        .request('terminal.subscribe', {'terminalId': widget.terminalId})
        .catchError((_) {});
    // 布局完成后 TerminalView 会触发 onResize，重新上报手机尺寸接管 PTY
  }

  @override
  void initState() {
    super.initState();
    _wireTerminal();

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
        // resize 标记：按当时的网格重设终端，否则 Windows ConPTY 的绝对
        // 光标定位会在错误尺寸下原地覆盖，滚动缓冲里大部分历史丢失。
        if (chunk['direction'] == 'resize') {
          try {
            final size =
                jsonDecode(utf8.decode(base64Decode(chunk['contentB64'])));
            final cols = (size['cols'] as num?)?.toInt() ?? 0;
            final rows = (size['rows'] as num?)?.toInt() ?? 0;
            if (cols > 0 && rows > 0) {
              // 临时摘掉 onResize 避免回声循环
              final orig = _terminal.onResize;
              _terminal.onResize = null;
              _terminal.resize(cols, rows);
              _terminal.onResize = orig;
            }
          } catch (_) {
            // 忽略非法 resize 标记
          }
          continue;
        }
        if (chunk['direction'] != null && chunk['direction'] != 'stdout') {
          continue;
        }
        _terminal.write(utf8.decode(base64Decode(chunk['contentB64'])));
      }
    }));
    // 桌面端调整窗口时 daemon 广播 terminal.size —— 手机【不】对齐服务端网格：
    // TerminalView 的 autoResize 会把本地网格锁定在手机视口，强行 resize
    // 只会与它打架（resize→下一帧布局又被打回，来回重排两次导致闪跳）。
    // 手机本地网格纯粹用于显示：auto-fit 手机宽度 + 软换行，排版自然。

    // subscribe (server replies with terminal.history + live output)
    ws.request('terminal.subscribe', {'terminalId': widget.terminalId}).catchError((_) {});
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
    } else if (text.contains('\n')) {
      // 多行内容（通常来自粘贴）：走 bracketed paste 通道。远端程序若开启
      // bracketed paste mode（pi / claude code 等都会开启），整段内容会被
      // 识别为一次粘贴，空格与换行不会被拆开逐条执行；末尾不附加回车，
      // 先进入输入框供确认。若远端不支持，则按普通文本逐行输入。
      _terminal.paste(text);
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

  /// 微信风格输入栏：
  /// - 顶部拖拽条：上下拉动调节高度（56 → 约半屏）；
  /// - 展开按钮：放大成大面板（约 85% 屏高）多行编辑，再点收起；
  /// - 多行内容发送走 bracketed paste（见 [_submitInput]）。
  Widget _buildInputBar() {
    final media = MediaQuery.of(context).size;
    final maxDragHeight = media.height * 0.5;
    final panelHeight =
        _expanded ? media.height * 0.85 : _inputHeight.clamp(_minInputHeight, maxDragHeight);

    return SafeArea(
      top: false,
      child: AnimatedContainer(
        duration: const Duration(milliseconds: 180),
        curve: Curves.easeOutCubic,
        height: panelHeight,
        decoration: const BoxDecoration(
          color: Colors.white,
          border: Border(top: BorderSide(color: Color(0xFFE5E6EB), width: 0.5)),
        ),
        child: Column(
          children: [
            // 顶部拖拽条 + 展开/收起按钮
            GestureDetector(
              behavior: HitTestBehavior.opaque,
              onVerticalDragUpdate: _expanded
                  ? null
                  : (details) {
                      setState(() {
                        _inputHeight = (_inputHeight - details.delta.dy)
                            .clamp(_minInputHeight, maxDragHeight);
                      });
                    },
              onDoubleTap: () => setState(() => _expanded = !_expanded),
              child: SizedBox(
                height: 20,
                child: Row(
                  children: [
                    const Spacer(),
                    Container(
                      width: 36,
                      height: 4,
                      decoration: BoxDecoration(
                        color: const Color(0xFFC9CDD4),
                        borderRadius: BorderRadius.circular(2),
                      ),
                    ),
                    const Spacer(),
                    Padding(
                      padding: const EdgeInsets.only(right: 8),
                      child: GestureDetector(
                        onTap: () => setState(() => _expanded = !_expanded),
                        child: Icon(
                          _expanded ? Icons.close_fullscreen : Icons.open_in_full,
                          size: 15,
                          color: const Color(0xFF86909C),
                        ),
                      ),
                    ),
                  ],
                ),
              ),
            ),
            // 输入区：输入框占据剩余全部高度（随拖拽/展开伸缩），按钮固定在底部
            Expanded(
              child: Padding(
                padding: const EdgeInsets.fromLTRB(8, 0, 8, 6),
                child: Column(
                  children: [
                    Expanded(
                      child: TextField(
                        controller: _inputCtrl,
                        style: const TextStyle(fontSize: 14),
                        maxLines: null,
                        expands: true,
                        textAlignVertical: TextAlignVertical.top,
                        keyboardType: TextInputType.multiline,
                        textInputAction: TextInputAction.newline,
                        decoration: InputDecoration(
                          hintText: _expanded ? '输入命令，可多行编辑…' : '输入命令，回车发送…',
                          isDense: true,
                          contentPadding:
                              const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
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
                      ),
                    ),
                    const SizedBox(height: 6),
                    Row(
                      mainAxisAlignment: MainAxisAlignment.end,
                      children: [
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
                  ],
                ),
              ),
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
