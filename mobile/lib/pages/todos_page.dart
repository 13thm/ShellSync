import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import '../stores/app_state.dart';
import '../models.dart';

class TodosPage extends StatelessWidget {
  const TodosPage({super.key});

  @override
  Widget build(BuildContext context) {
    final app = context.watch<AppState>();
    final pending = app.todos.where((t) => t.status == 'pending').toList();
    final done = app.todos.where((t) => t.status == 'done').toList();

    return Column(
      children: [
        _AddBar(),
        Expanded(
          child: ListView(
            children: [
              for (final t in pending) _TodoTile(t: t, done: false),
              if (done.isNotEmpty) ...[
                const Padding(
                  padding: EdgeInsets.fromLTRB(16, 16, 16, 4),
                  child: Text('已完成', style: TextStyle(color: Color(0xFF8A909A), fontSize: 13)),
                ),
                for (final t in done) _TodoTile(t: t, done: true),
              ],
            ],
          ),
        ),
      ],
    );
  }
}

class _AddBar extends StatefulWidget {
  @override
  State<_AddBar> createState() => _AddBarState();
}

class _AddBarState extends State<_AddBar> {
  final _ctrl = TextEditingController();

  @override
  void dispose() {
    _ctrl.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Container(
      color: Colors.white,
      padding: const EdgeInsets.fromLTRB(16, 8, 8, 8),
      child: Row(
        children: [
          Expanded(
            child: TextField(
              controller: _ctrl,
              decoration: const InputDecoration(
                isDense: true,
                hintText: '新建待办…',
                border: InputBorder.none,
              ),
              onSubmitted: (_) => _add(),
            ),
          ),
          IconButton(
            icon: const Icon(Icons.add),
            onPressed: _add,
          ),
        ],
      ),
    );
  }

  Future<void> _add() async {
    final title = _ctrl.text.trim();
    if (title.isEmpty) return;
    await context.read<AppState>().addTodo(title);
    _ctrl.clear();
  }
}

class _TodoTile extends StatelessWidget {
  const _TodoTile({required this.t, required this.done});
  final Todo t;
  final bool done;

  @override
  Widget build(BuildContext context) {
    final app = context.read<AppState>();
    return ListTile(
      leading: GestureDetector(
        onTap: () => app.toggleTodo(t),
        child: Icon(
          done ? Icons.check_circle : Icons.radio_button_unchecked,
          color: done ? const Color(0xFF3BA776) : const Color(0xFFCDD1D8),
        ),
      ),
      title: Text(
        t.title,
        style: TextStyle(
          color: done ? const Color(0xFF8A909A) : const Color(0xFF1F2329),
          decoration: done ? TextDecoration.lineThrough : null,
        ),
      ),
      trailing: IconButton(
        icon: const Icon(Icons.close, size: 18, color: Color(0xFF8A909A)),
        onPressed: () => app.deleteTodo(t.id),
      ),
    );
  }
}
