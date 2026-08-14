import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import '../models.dart';
import '../stores/app_state.dart';

/// M4-6 设置页：已配对设备管理 + 退出登录。
class SettingsPage extends StatefulWidget {
  const SettingsPage({super.key});

  @override
  State<SettingsPage> createState() => _SettingsPageState();
}

class _SettingsPageState extends State<SettingsPage> {
  List<Device> _devices = [];
  bool _loading = true;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    final api = context.read<AppState>().service;
    if (api == null) return;
    try {
      _devices = await api.listDevices();
    } catch (_) {}
    if (mounted) setState(() => _loading = false);
  }

  @override
  Widget build(BuildContext context) {
    final app = context.watch<AppState>();
    return Scaffold(
      appBar: AppBar(title: const Text('设置')),
      body: ListView(
        children: [
          ListTile(
            title: const Text('连接状态'),
            subtitle: Text(app.wsConnected ? '已同步' : '重连中…'),
            leading: Icon(Icons.cloud_done,
                color: app.wsConnected ? const Color(0xFF3BA776) : const Color(0xFFE0A13C)),
          ),
          const Divider(height: 1),
          const _SectionHeader('已配对设备'),
          if (_loading)
            const Padding(padding: EdgeInsets.all(24), child: Center(child: CircularProgressIndicator()))
          else if (_devices.isEmpty)
            const ListTile(title: Text('暂无设备', style: TextStyle(color: Color(0xFF8A909A))))
          else
            for (final d in _devices)
              ListTile(
                title: Text('${d.name} · ${d.platform}'),
                subtitle: Text(d.revoked ? '已吊销' : '设备 ID: ${d.id.substring(0, 8)}'),
                enabled: !d.revoked,
                trailing: Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    if (!d.revoked)
                      TextButton(
                        onPressed: () => _revoke(d),
                        child: const Text('吊销', style: TextStyle(color: Color(0xFFE45656))),
                      ),
                    TextButton(
                      onPressed: () => _delete(d),
                      child: const Text('删除', style: TextStyle(color: Color(0xFF86909C))),
                    ),
                  ],
                ),
              ),
          const Divider(height: 32),
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 16),
            child: FilledButton(
              style: FilledButton.styleFrom(
                backgroundColor: const Color(0xFFFCEDED),
                foregroundColor: const Color(0xFFE45656),
              ),
              onPressed: () => _confirmLogout(),
              child: const Text('退出登录'),
            ),
          ),
          const SizedBox(height: 8),
          const Padding(
            padding: EdgeInsets.symmetric(horizontal: 16),
            child: Text(
              '退出后将清除本机的配对信息，需要重新扫码或手动输入配对码才能重新连接电脑。',
              textAlign: TextAlign.center,
              style: TextStyle(color: Color(0xFF86909C), fontSize: 12, height: 1.5),
            ),
          ),
        ],
      ),
    );
  }

  Future<void> _revoke(Device d) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('吊销设备'),
        content: Text('吊销「${d.name}」？\n该设备将立即断开，无法再访问电脑端（可重新扫码配对）。'),
      ),
    );
    if (ok != true || !mounted) return;
    await context.read<AppState>().service!.revokeDevice(d.id);
    _load();
  }

  Future<void> _delete(Device d) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('删除设备'),
        content: Text('删除「${d.name}」？\n记录将从列表中移除。'),
      ),
    );
    if (ok != true || !mounted) return;
    await context.read<AppState>().service!.deleteDevice(d.id);
    _load();
  }

  Future<void> _confirmLogout() async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('退出登录'),
        content: const Text('确定要退出吗？\n\n退出会断开与电脑端的连接，并清除本机保存的配对信息。下次使用需要重新配对。'),
        actions: [
          TextButton(onPressed: () => Navigator.pop(ctx, false), child: const Text('取消')),
          FilledButton(
            style: FilledButton.styleFrom(
              backgroundColor: const Color(0xFFE45656),
            ),
            onPressed: () => Navigator.pop(ctx, true),
            child: const Text('退出'),
          ),
        ],
      ),
    );
    if (ok != true || !mounted) return;
    context.read<AppState>().logout();
  }
}

class _SectionHeader extends StatelessWidget {
  const _SectionHeader(this.text);
  final String text;
  @override
  Widget build(BuildContext context) => Padding(
        padding: const EdgeInsets.fromLTRB(16, 16, 16, 8),
        child: Text(text, style: const TextStyle(color: Color(0xFF8A909A), fontSize: 13)),
      );
}
