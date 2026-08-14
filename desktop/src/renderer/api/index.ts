import { http } from './client'
import type {
  Device,
  LogsResponse,
  PairInit,
  Shell,
  Task,
  Terminal,
  Todo,
} from '../types'

export interface CreateTaskInput {
  name: string
  description?: string
  color?: string
  status?: string
}
export interface UpdateTaskInput {
  name?: string
  description?: string
  status?: string
  color?: string
  archived?: boolean
}

export const tasksApi = {
  list: (params: { status?: string; archived?: boolean; since?: number } = {}) =>
    http().get<Task[]>('/api/tasks', { params }).then((r) => r.data),
  get: (id: string) => http().get<Task>(`/api/tasks/${id}`).then((r) => r.data),
  create: (input: CreateTaskInput) =>
    http().post<Task>('/api/tasks', input).then((r) => r.data),
  update: (id: string, input: UpdateTaskInput) =>
    http().patch<Task>(`/api/tasks/${id}`, input).then((r) => r.data),
  remove: (id: string) => http().delete(`/api/tasks/${id}`).then((r) => r.data),
}

export interface CreateTerminalInput {
  taskId?: string
  name?: string
  shellType: string
  cwd?: string
  cols?: number
  rows?: number
  env?: Record<string, string>
}

export const terminalsApi = {
  list: (params: { taskId?: string; status?: string } = {}) =>
    http().get<Terminal[]>('/api/terminals', { params }).then((r) => r.data),
  get: (id: string) => http().get<Terminal>(`/api/terminals/${id}`).then((r) => r.data),
  create: (input: CreateTerminalInput) =>
    http().post<Terminal>('/api/terminals', input).then((r) => r.data),
  update: (id: string, input: { name?: string; taskId?: string }) =>
    http().patch<Terminal>(`/api/terminals/${id}`, input).then((r) => r.data),
  resize: (id: string, cols: number, rows: number) =>
    http().post(`/api/terminals/${id}/resize`, { cols, rows }).then((r) => r.data),
  restart: (id: string) =>
    http().post<Terminal>(`/api/terminals/${id}/restart`).then((r) => r.data),
  remove: (id: string) => http().delete(`/api/terminals/${id}`).then((r) => r.data),
  logs: (id: string, fromSeq = 1, limit = 500) =>
    http()
      .get<LogsResponse>(`/api/terminals/${id}/logs`, { params: { fromSeq, limit } })
      .then((r) => r.data),
  logsTail: (id: string, limit = 500) =>
    http()
      .get<LogsResponse>(`/api/terminals/${id}/logs/tail`, { params: { limit } })
      .then((r) => r.data),
}

export interface CreateTodoInput {
  title: string
  content?: string
  taskId?: string
  terminalID?: string
  priority?: number
}
export interface UpdateTodoInput {
  title?: string
  content?: string
  status?: string
  priority?: number
  taskId?: string
  terminalID?: string
  sortOrder?: number
}

export const todosApi = {
  list: (params: { taskId?: string; status?: string } = {}) =>
    http().get<Todo[]>('/api/todos', { params }).then((r) => r.data),
  get: (id: string) => http().get<Todo>(`/api/todos/${id}`).then((r) => r.data),
  create: (input: CreateTodoInput) =>
    http().post<Todo>('/api/todos', input).then((r) => r.data),
  update: (id: string, input: UpdateTodoInput) =>
    http().patch<Todo>(`/api/todos/${id}`, input).then((r) => r.data),
  remove: (id: string) => http().delete(`/api/todos/${id}`).then((r) => r.data),
}

export const devicesApi = {
  list: () => http().get<Device[]>('/api/devices').then((r) => r.data),
  revoke: (id: string) => http().delete(`/api/devices/${id}`).then((r) => r.data),
  delete: (id: string) =>
    http().delete(`/api/devices/${id}`, { params: { mode: 'delete' } }).then((r) => r.data),
}

export const settingsApi = {
  getAll: () => http().get<Record<string, string>>('/api/settings').then((r) => r.data),
  patch: (kv: Record<string, string>) =>
    http().patch('/api/settings', kv).then((r) => r.data),
}

export const systemApi = {
  shells: () => http().get<Shell[]>('/api/shells').then((r) => r.data),
  pairInit: () => http().post<PairInit>('/api/pair/init').then((r) => r.data),
  pairVerify: (pairingCode: string, deviceName: string, platform: string) =>
    http()
      .post<{ sessionToken: string; device: Device }>('/api/pair/verify', {
        pairingCode,
        deviceName,
        platform,
      })
      .then((r) => r.data),
}
