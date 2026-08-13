import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:xterm/xterm.dart' as xterm;
import '../api/ws_client.dart';

/// Terminal pane backed by the xterm dart engine, wired to the daemon WS.
/// Loads full history on subscribe, streams live output, sends input + resize.
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
  final _focus = FocusNode();

  // light theme to match the design system (§二 浅色，不搞暗黑科技感)
  static const _theme = xterm.TerminalTheme(
    backgroundColor: Color(0xFFFFFFFF),
    foregroundColor: Color(0xFF1F2329),
    cursor: Color(0xFF3BA776),
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
  );

  @override
  void initState() {
    super.initState();
    _terminal = xterm.Terminal(maxLines: 8000);

    _terminal.onOutput = (data) {
      widget.ws.send('terminal.input', {
        'terminalId': widget.terminalId,
        'dataB64': base64Encode(utf8.encode(data)),
      });
    };
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
      _terminal.write(utf8.decode(base64Decode(p['contentB64'])));
    }));
    _subs.add(ws.on('terminal.history', (m) {
      final p = (m['payload'] ?? {}) as Map;
      if (p['terminalId'] != widget.terminalId) return;
      for (final c in (p['chunks'] as List? ?? [])) {
        _terminal.write(utf8.decode(base64Decode((c as Map)['contentB64'])));
      }
    }));
    _subs.add(ws.on('terminal.status', (m) {
      // status handled at list level; nothing to do in-pane
    }));

    // subscribe (server replies with terminal.history + live output)
    ws.request('terminal.subscribe', {'terminalId': widget.terminalId}).catchError((_) {});
  }

  @override
  void dispose() {
    for (final s in _subs) {
      s();
    }
    widget.ws.send('terminal.unsubscribe', {'terminalId': widget.terminalId});
    _focus.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return xterm.TerminalView(
      _terminal,
      autofocus: true,
      focusNode: _focus,
      theme: _theme,
      textStyle: const xterm.TerminalStyle(fontSize: 13),
    );
  }
}
