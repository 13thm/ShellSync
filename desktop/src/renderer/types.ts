/** DTOs — 必须与 daemon transport/http/dto.go 保持一致。 */

export type TaskStatus = 'pending' | 'running' | 'paused' | 'done'
export type TerminalStatus = 'running' | 'exited' | 'crashed'
export type TodoStatus = 'pending' | 'done'

export interface Task {
  id: string
  name: string
  description: string
  status: TaskStatus
  color: string | null
  archived: boolean
  createdAt: number
  updatedAt: number
}

export interface Terminal {
  id: string
  taskId: string
  name: string
  shellType: string
  cwd: string
  cols: number
  rows: number
  status: TerminalStatus
  exitCode: number | null
  lastSeq: number
  createdAt: number
  lastActiveAt: number
  updatedAt: number
}

export interface Todo {
  id: string
  taskId: string
  terminalID: string
  title: string
  content: string
  status: TodoStatus
  priority: number
  sortOrder: number
  createdAt: number
  updatedAt: number
}

export interface LogChunk {
  seq: number
  direction: string
  contentB64: string
  createdAt: number
}

export interface Device {
  id: string
  name: string
  platform: string
  lastSeenAt: number
  createdAt: number
  revoked: boolean
}

export interface Shell {
  type: string
  path: string
  available: boolean
}

export interface PairInit {
  pairingCode: string
  qrPayload: string
  expiresAt: number
}

export interface LogsResponse {
  terminalId: string
  chunks: LogChunk[]
  hasMore: boolean
}
