import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import '../stores/app_state.dart';
import '../models.dart';
import '../widgets/status.dart';

class TasksPage extends StatelessWidget {
  const TasksPage({super.key});

  /// Allowed next statuses (mirrors the daemon task state machine).
  List<({String to, String label})> _transitions(String status) {
    switch (status) {
      case 'pending':
        return const [(to: 'running', label: '开始')];
      case 'running':
        return const [
          (to: 'paused', label: '暂停'),
          (to: 'done', label: '完成'),
        ];
      case 'paused':
        return const [
          (to: 'running', label: '继续'),
          (to: 'done', label: '完成'),
        ];
      case 'done':
        return const [(to: 'running', label: '重新开始')];
      default:
        return const [];
    }
  }

  @override
  Widget build(BuildContext context) {
    final app = context.watch<AppState>();
    final active = app.tasks.where((t) => !t.archived).toList();

    if (active.isEmpty) {
      return const Center(child: Text('还没有任务', style: TextStyle(color: Color(0xFF8A909A))));
    }

    return ListView.separated(
      itemCount: active.length,
      separatorBuilder: (_, __) => const Divider(height: 1, indent: 56),
      itemBuilder: (_, i) {
        final t = active[i];
        final trs = _transitions(t.status);
        return ListTile(
          leading: StatusDot(taskStatusColor(t.status)),
          title: Text(t.name),
          subtitle: Text(taskStatusLabel(t.status), style: const TextStyle(fontSize: 13, color: Color(0xFF8A909A))),
          trailing: trs.isEmpty
              ? null
              : PopupMenuButton<String>(
                  icon: const Icon(Icons.more_horiz),
                  onSelected: (to) => app.updateTask(t.id, {'status': to}),
                  itemBuilder: (_) => [for (final tr in trs) PopupMenuItem(value: tr.to, child: Text(tr.label))],
                ),
        );
      },
    );
  }
}
