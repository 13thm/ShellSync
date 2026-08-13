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
                trailing: d.revoked
                    ? null
                    : TextButton(
                        onPressed: () async {
                          await app.service!.revokeDevice(d.id);
                          _load();
                        },
                        child: const Text('吊销', style: TextStyle(color: Color(0xFFE45656))),
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
              onPressed: () => app.logout(),
              child: const Text('退出登录'),
            ),
          ),
        ],
      ),
    );
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
