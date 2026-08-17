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
  int _paneRefresh = 0;

  /// 刷新：重建本地终端并重新订阅，等同「返回列表再点进来」。
  void _refresh() => setState(() => _paneRefresh++);

  @override
  Widget build(BuildContext context) {
    final app = context.watch<AppState>(); // 切换局域网/云后 ws 重建 → 重新挂载终端面板
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
          IconButton(
            tooltip: '刷新（重新加载终端）',
            icon: const Icon(Icons.refresh),
            onPressed: _refresh,
          ),
        ],
      ),
      body: SafeArea(
        child: app.ws == null
            ? const Center(child: CircularProgressIndicator())
            : TerminalPane(
                key: ValueKey('${t.id}:${app.wsGen}:$_paneRefresh'),
                terminalId: t.id,
                ws: app.ws!),
      ),
    );
  }
}
