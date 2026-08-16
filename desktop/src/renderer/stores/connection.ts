import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
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
  // 直接派生自 realtime store（自持 wsConnected）；曾经用 accessor 回调同步，
  // 但 Pinia setup store 的 accessor 属性会被拷贝丢失，回调永远不触发，
  // 状态灯永远卡在「重连中」。
  const wsConnected = computed(() => useRealtimeStore().wsConnected)

  async function init(): Promise<boolean> {
    status.value = 'connecting'
    error.value = ''
    try {
      const conn = await window.daemon.connect()
      if (!conn) throw new Error('daemon 未运行，且无法启动')
      configureHttp(conn.baseUrl, conn.token)

      const realtime = useRealtimeStore()
      // WS 断线重连前重新解析 daemon 连接（daemon 重启后端口/token 会变）
      realtime.init(conn.wsUrl, conn.token, async () => {
        try {
          const fresh = await window.daemon.connect()
          if (!fresh) return null
          configureHttp(fresh.baseUrl, fresh.token)
          return { url: fresh.wsUrl, token: fresh.token }
        } catch {
          return null // 保畵上次已知地址继续重试
        }
      })

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
