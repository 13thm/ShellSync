import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import '../models.dart';
import '../stores/app_state.dart';
import '../widgets/status.dart';
import '../widgets/terminal_view.dart';

/// M4-5 远程终端会话页：加载全部历史 + 实时输出 + 可输入干预。
class TerminalSessionPage extends StatefulWidget {
  const TerminalSessionPage({required this.terminal, super.key});
  final Terminal terminal;

  @override
  State<TerminalSessionPage> createState() => _TerminalSessionPageState();
}

class _TerminalSessionPageState extends State<TerminalSessionPage> {
  @override
  Widget build(BuildContext context) {
    final app = context.read<AppState>();
    final t = widget.terminal;
    return Scaffold(
      appBar: AppBar(
        title: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            StatusDot(terminalStatusColor(t.status)),
            const SizedBox(width: 8),
            Flexible(child: Text(t.name, overflow: TextOverflow.ellipsis)),
          ],
        ),
        actions: [
          if (!app.wsConnected)
            const Padding(
              padding: EdgeInsets.only(right: 12),
              child: Center(
                child: Text('重连中…', style: TextStyle(color: Color(0xFFE0A13C), fontSize: 13)),
              ),
            ),
        ],
      ),
      body: SafeArea(
        child: app.ws == null
            ? const Center(child: CircularProgressIndicator())
            : TerminalPane(terminalId: t.id, ws: app.ws!),
      ),
    );
  }
}
