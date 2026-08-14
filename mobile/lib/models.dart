/// DTOs — 必须与 daemon transport/http/dto.go 对齐（camelCase）。
library models;

class Task {
  final String id;
  final String name;
  final String description;
  final String status;
  final String? color;
  final bool archived;
  final int createdAt;
  final int updatedAt;

  Task({
    required this.id,
    required this.name,
    required this.description,
    required this.status,
    this.color,
    required this.archived,
    required this.createdAt,
    required this.updatedAt,
  });

  factory Task.fromJson(Map<String, dynamic> j) => Task(
        id: j['id'],
        name: j['name'],
        description: j['description'] ?? '',
        status: j['status'] ?? 'pending',
        color: j['color'],
        archived: j['archived'] ?? false,
        createdAt: j['createdAt'] ?? 0,
        updatedAt: j['updatedAt'] ?? 0,
      );
}

class Terminal {
  final String id;
  final String taskId;
  final String name;
  final String shellType;
  final String cwd;
  final int cols;
  final int rows;
  final String status;
  final int? exitCode;
  final int lastSeq;
  final int createdAt;
  final int lastActiveAt;
  final int updatedAt;

  Terminal({
    required this.id,
    required this.taskId,
    required this.name,
    required this.shellType,
    required this.cwd,
    required this.cols,
    required this.rows,
    required this.status,
    this.exitCode,
    required this.lastSeq,
    required this.createdAt,
    required this.lastActiveAt,
    required this.updatedAt,
  });

  factory Terminal.fromJson(Map<String, dynamic> j) => Terminal(
        id: j['id'],
        taskId: j['taskId'] ?? '',
        name: j['name'],
        shellType: j['shellType'],
        cwd: j['cwd'] ?? '',
        cols: j['cols'] ?? 80,
        rows: j['rows'] ?? 24,
        status: j['status'] ?? 'running',
        exitCode: j['exitCode'],
        lastSeq: j['lastSeq'] ?? 0,
        createdAt: j['createdAt'] ?? 0,
        lastActiveAt: j['lastActiveAt'] ?? 0,
        updatedAt: j['updatedAt'] ?? 0,
      );
}

class Todo {
  final String id;
  final String taskId;
  final String terminalID;
  final String title;
  final String content;
  final String status;
  final int priority;
  final int sortOrder;
  final int createdAt;
  final int updatedAt;

  Todo({
    required this.id,
    required this.taskId,
    required this.terminalID,
    required this.title,
    required this.content,
    required this.status,
    required this.priority,
    required this.sortOrder,
    required this.createdAt,
    required this.updatedAt,
  });

  factory Todo.fromJson(Map<String, dynamic> j) => Todo(
        id: j['id'],
        taskId: j['taskId'] ?? '',
        terminalID: j['terminalID'] ?? '',
        title: j['title'],
        content: j['content'] ?? '',
        status: j['status'] ?? 'pending',
        priority: j['priority'] ?? 0,
        sortOrder: j['sortOrder'] ?? 0,
        createdAt: j['createdAt'] ?? 0,
        updatedAt: j['updatedAt'] ?? 0,
      );
}

class Device {
  final String id;
  final String name;
  final String platform;
  final int lastSeenAt;
  final int createdAt;
  final bool revoked;

  Device({
    required this.id,
    required this.name,
    required this.platform,
    required this.lastSeenAt,
    required this.createdAt,
    required this.revoked,
  });

  factory Device.fromJson(Map<String, dynamic> j) => Device(
        id: j['id'],
        name: j['name'],
        platform: j['platform'],
        lastSeenAt: j['lastSeenAt'] ?? 0,
        createdAt: j['createdAt'] ?? 0,
        revoked: j['revoked'] ?? false,
      );
}
