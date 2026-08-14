import { contextBridge, ipcRenderer } from 'electron'

/**
 * Renderer-facing bridge. The renderer asks the main process for the daemon
 * connection (port + token + URLs). Every call re-resolves in the main
 * process, so a daemon restart (new port/token) is always picked up.
 */
export interface DaemonConnection {
  port: number
  token: string
  baseUrl: string
  wsUrl: string
}

contextBridge.exposeInMainWorld('daemon', {
  async connect(): Promise<DaemonConnection | null> {
    return (await ipcRenderer.invoke('daemon:connect')) as DaemonConnection | null
  },
})
