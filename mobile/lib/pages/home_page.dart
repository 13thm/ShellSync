import 'package:flutter/material.dart';
import 'tasks_page.dart';
import 'todos_page.dart';
import 'terminals_page.dart';
import 'settings_page.dart';

/// M4-4 主页：底部三 Tab（任务 / 待办 / 终端）+ 设置入口。
class HomePage extends StatefulWidget {
  const HomePage({super.key});

  @override
  State<HomePage> createState() => _HomePageState();
}

class _HomePageState extends State<HomePage> {
  int _index = 0;

  static const _pages = [
    TasksPage(),
    TodosPage(),
    TerminalsPage(),
  ];

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: Text(_titles[_index]),
        actions: [
          IconButton(
            icon: const Icon(Icons.settings_outlined),
            onPressed: () => Navigator.of(context).push(
              MaterialPageRoute(builder: (_) => const SettingsPage()),
            ),
          ),
        ],
      ),
      body: IndexedStack(index: _index, children: _pages),
      bottomNavigationBar: NavigationBar(
        selectedIndex: _index,
        onDestinationSelected: (i) => setState(() => _index = i),
        destinations: const [
          NavigationDestination(icon: Icon(Icons.checklist), label: '任务'),
          NavigationDestination(icon: Icon(Icons.check_box_outlined), label: '待办'),
          NavigationDestination(icon: Icon(Icons.terminal), label: '终端'),
        ],
      ),
    );
  }
}

const _titles = ['任务', '待办', '终端'];
