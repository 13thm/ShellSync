import { defineStore } from 'pinia'
import { shallowRef } from 'vue'
import { WSClient } from '../api/ws'
import { useTasksStore } from './tasks'
import { useTerminalsStore } from './terminals'
import { useTodosStore } from './todos'

/**
 * Owns the single WebSocket connection and routes daemon events into the
 * entity stores so the UI stays live across Desktop/Mobile.
 */
export const useRealtimeStore = defineStore('realtime', () => {
  const ws = shallowRef<WSClient | null>(null)
  let onState: (connected: boolean) => void = () => {}

  function init(url: string, token: string) {
    if (ws.value) return
    const client = new WSClient(url, token)
    client.onState = (c) => onState(c)

    const tasks = useTasksStore()
    const terminals = useTerminalsStore()
    const todos = useTodosStore()

    client.on('task.created', (p) => tasks.upsert(p))
    client.on('task.updated', (p) => tasks.upsert(p))
    client.on('task.deleted', (p) => tasks.removeLocal(p.id))

    client.on('terminal.created', (p) => terminals.upsert(p))
    client.on('terminal.updated', (p) => terminals.upsert(p))
    client.on('terminal.deleted', (p) => terminals.removeLocal(p.id))

    client.on('todo.created', (p) => todos.upsert(p))
    client.on('todo.updated', (p) => todos.upsert(p))
    client.on('todo.deleted', (p) => todos.removeLocal(p.id))

    client.connect()
    ws.value = client
  }

  return { ws, init, set onState(fn: (c: boolean) => void) { onState = fn } }
})
