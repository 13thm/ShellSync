import 'package:flutter/material.dart';

Color taskStatusColor(String s) {
  switch (s) {
    case 'running':
      return const Color(0xFF3BA776);
    case 'paused':
      return const Color(0xFFE0A13C);
    case 'done':
      return const Color(0xFF8A909A);
    default:
      return const Color(0xFF8A909A);
  }
}

String taskStatusLabel(String s) {
  switch (s) {
    case 'pending':
      return '未开始';
    case 'running':
      return '进行中';
    case 'paused':
      return '已暂停';
    case 'done':
      return '已完成';
    default:
      return s;
  }
}

Color terminalStatusColor(String s) {
  switch (s) {
    case 'running':
      return const Color(0xFF3BA776);
    case 'crashed':
      return const Color(0xFFE45656);
    default:
      return const Color(0xFF8A909A);
  }
}

String terminalStatusLabel(String s) {
  switch (s) {
    case 'running':
      return '运行中';
    case 'exited':
      return '已退出';
    case 'crashed':
      return '已崩溃';
    default:
      return s;
  }
}

class StatusDot extends StatelessWidget {
  const StatusDot(this.color, {this.size = 8, super.key});
  final Color color;
  final double size;
  @override
  Widget build(BuildContext context) =>
      Container(width: size, height: size, decoration: BoxDecoration(color: color, shape: BoxShape.circle));
}
