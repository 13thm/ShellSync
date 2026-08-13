import { contextBridge, ipcRenderer } from 'electron'

/**
 * Renderer-facing bridge. The renderer asks the main process for the daemon
 * connection (port + token + URLs) once on startup.
 */
export interface DaemonConnection {
  port: number
  token: string
  baseUrl: string
  wsUrl: string
}

let cached: DaemonConnection | null = null

contextBridge.exposeInMainWorld('daemon', {
  async connect(): Promise<DaemonConnection | null> {
    if (cached) return cached
    const conn = (await ipcRenderer.invoke('daemon:connect')) as DaemonConnection | null
    if (conn) cached = conn
    return conn
  },
})
