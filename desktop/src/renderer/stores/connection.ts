import { defineStore } from 'pinia'
import { ref } from 'vue'
import { configureHttp } from '../api/client'
import { useRealtimeStore } from './realtime'
import { useTasksStore } from './tasks'
import { useTerminalsStore } from './terminals'
import { useTodosStore } from './todos'
import { useSettingsStore } from './settings'

export type ConnStatus = 'idle' | 'connecting' | 'ready' | 'error'

/** Bootstraps the daemon connection, HTTP client, WS client and initial data. */
export const useConnectionStore = defineStore('connection', () => {
  const status = ref<ConnStatus>('idle')
  const error = ref('')
  const wsConnected = ref(false)

  async function init(): Promise<boolean> {
    status.value = 'connecting'
    error.value = ''
    try {
      const conn = await window.daemon.connect()
      if (!conn) throw new Error('daemon 未运行，且无法启动')
      configureHttp(conn.baseUrl, conn.token)

      const realtime = useRealtimeStore()
      realtime.init(conn.wsUrl, conn.token)
      realtime.onState = (c) => (wsConnected.value = c)

      await Promise.all([
        useTasksStore().load(),
        useTerminalsStore().load(),
        useTodosStore().load(),
        useSettingsStore().load(),
        useTerminalsStore().loadShells(),
      ])
      status.value = 'ready'
      return true
    } catch (e: any) {
      status.value = 'error'
      error.value = e?.message ?? String(e)
      return false
    }
  }

  return { status, error, wsConnected, init }
})
