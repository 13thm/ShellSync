import { defineStore } from 'pinia'
import { shallowRef, ref } from 'vue'
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
  /** Increments on every reconnect; views embed it in :key to force resubscribe. */
  const wsSession = ref(0)
  let onState: (connected: boolean) => void = () => {}

  function init(url: string, token: string, refresh?: () => Promise<{ url: string; token: string } | null>) {
    if (ws.value) return
    const client = new WSClient(url, token)
    client.onState = (c) => {
      if (c) wsSession.value++ // connection (re)established → views may resubscribe
      onState(c)
    }
    if (refresh) client.refresh = refresh

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

  return { ws, wsSession, init, set onState(fn: (c: boolean) => void) { onState = fn } }
})
