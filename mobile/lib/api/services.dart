import '../models.dart';
import 'rest_client.dart';

/// Resource service: wraps all REST endpoints with typed returns.
class ApiService {
  ApiService(this.http);
  final RestClient http;

  // tasks
  Future<List<Task>> listTasks({String? status}) async {
    final data = await http.get('/api/tasks', query: {if (status != null) 'status': status});
    return (data as List).map((e) => Task.fromJson(e as Map<String, dynamic>)).toList();
  }

  Future<Task> updateTask(String id, Map<String, dynamic> patch) async {
    final data = await http.patch('/api/tasks/$id', patch);
    return Task.fromJson(data as Map<String, dynamic>);
  }

  Future<void> deleteTask(String id) => http.delete('/api/tasks/$id');

  // todos
  Future<List<Todo>> listTodos({String? status}) async {
    final data = await http.get('/api/todos', query: {if (status != null) 'status': status});
    return (data as List).map((e) => Todo.fromJson(e as Map<String, dynamic>)).toList();
  }

  Future<Todo> createTodo(String title, {String? taskId}) async {
    final data = await http.post('/api/todos', {'title': title, if (taskId != null) 'taskId': taskId});
    return Todo.fromJson(data as Map<String, dynamic>);
  }

  Future<Todo> updateTodo(String id, Map<String, dynamic> patch) async {
    final data = await http.patch('/api/todos/$id', patch);
    return Todo.fromJson(data as Map<String, dynamic>);
  }

  Future<void> deleteTodo(String id) => http.delete('/api/todos/$id');

  // terminals
  Future<List<Terminal>> listTerminals({String? status}) async {
    final data = await http.get('/api/terminals', query: {if (status != null) 'status': status});
    return (data as List).map((e) => Terminal.fromJson(e as Map<String, dynamic>)).toList();
  }

  // devices
  Future<List<Device>> listDevices() async {
    final data = await http.get('/api/devices');
    return (data as List).map((e) => Device.fromJson(e as Map<String, dynamic>)).toList();
  }

  Future<void> revokeDevice(String id) => http.delete('/api/devices/$id');

  Future<void> deleteDevice(String id) => http.delete('/api/devices/$id?mode=delete');
}
