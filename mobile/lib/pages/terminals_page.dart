import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import '../stores/app_state.dart';
import '../widgets/status.dart';
import 'terminal_session_page.dart';

class TerminalsPage extends StatelessWidget {
  const TerminalsPage({super.key});

  @override
  Widget build(BuildContext context) {
    final app = context.watch<AppState>();
    final terms = app.terminals;

    if (terms.isEmpty) {
      return const Center(child: Text('还没有终端\n在电脑端创建后这里会同步', style: TextStyle(color: Color(0xFF8A909A)), textAlign: TextAlign.center));
    }

    return ListView.separated(
      itemCount: terms.length,
      separatorBuilder: (_, __) => const Divider(height: 1, indent: 56),
      itemBuilder: (_, i) {
        final t = terms[i];
        return ListTile(
          leading: StatusDot(terminalStatusColor(t.status)),
          title: Text(t.name),
          subtitle: Text(
            '${t.shellType} · ${terminalStatusLabel(t.status)}',
            style: const TextStyle(fontSize: 13, color: Color(0xFF8A909A)),
          ),
          onTap: () => Navigator.of(context).push(
            MaterialPageRoute(builder: (_) => TerminalSessionPage(terminal: t)),
          ),
        );
      },
    );
  }
}
