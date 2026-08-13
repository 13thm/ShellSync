/**
 * WebSocket client for the daemon. Handles reconnection and typed event
 * subscription. Terminal stream events (terminal.output/history/status) and
 * data-sync events (task.created etc.) all flow through here.
 */

type Handler = (payload: any) => void

interface RawMessage {
  type: string
  id?: string
  ref?: string
  ok?: boolean
  payload?: any
  error?: { code: string; message: string }
}

export class WSClient {
  private ws: WebSocket | null = null
  private url: string
  private token: string
  private handlers = new Map<string, Set<Handler>>()
  private reqHandlers = new Map<string, (ok: boolean, payload: any, err?: any) => void>()
  private idCounter = 0
  private reconnectTimer: number | null = null
  private backoff = 1000
  private stopped = false

  /** Fires whenever the connection state changes. */
  public onState?: (connected: boolean) => void

  constructor(url: string, token: string) {
    this.url = url
    this.token = token
  }

  connect() {
    this.stopped = false
    this.open()
  }

  private open() {
    const ws = new WebSocket(`${this.url}?token=${encodeURIComponent(this.token)}`)
    this.ws = ws
    ws.onopen = () => {
      this.backoff = 1000
      this.onState?.(true)
    }
    ws.onmessage = (ev) => this.handleMessage(ev.data)
    ws.onclose = () => {
      this.onState?.(false)
      this.scheduleReconnect()
    }
    ws.onerror = () => {
      // onclose will follow
    }
  }

  private scheduleReconnect() {
    if (this.stopped) return
    if (this.reconnectTimer) return
    this.reconnectTimer = window.setTimeout(() => {
      this.reconnectTimer = null
      this.backoff = Math.min(this.backoff * 1.6, 15000)
      this.open()
    }, this.backoff)
  }

  private handleMessage(raw: string) {
    let msg: RawMessage
    try {
      msg = JSON.parse(raw)
    } catch {
      return
    }
    // request/response correlation by ref
    if (msg.ref && this.reqHandlers.has(msg.ref)) {
      const cb = this.reqHandlers.get(msg.ref)!
      this.reqHandlers.delete(msg.ref)
      cb(msg.ok !== false, msg.payload, msg.error)
      return
    }
    // typed event dispatch
    const set = this.handlers.get(msg.type)
    if (set) for (const h of set) h(msg.payload)
    // wildcard listeners
    const wild = this.handlers.get('*')
    if (wild) for (const h of wild) h(msg)
  }

  /** Subscribe to an event type. '*' matches every message. Returns unsubscribe. */
  on(type: string, cb: Handler): () => void {
    if (!this.handlers.has(type)) this.handlers.set(type, new Set())
    this.handlers.get(type)!.add(cb)
    return () => this.handlers.get(type)?.delete(cb)
  }

  /** Send a fire-and-forget event. */
  send(type: string, payload?: any) {
    this.ws?.send(JSON.stringify({ type, payload }))
  }

  /** Send an event and await the matching ack (by ref). */
  request(type: string, payload?: any, timeoutMs = 8000): Promise<any> {
    const id = `r${++this.idCounter}`
    return new Promise((resolve, reject) => {
      const timer = window.setTimeout(() => {
        this.reqHandlers.delete(id)
        reject(new Error(`${type} timed out`))
      }, timeoutMs)
      this.reqHandlers.set(id, (ok, p, err) => {
        window.clearTimeout(timer)
        ok ? resolve(p) : reject(new Error(err?.message || `${type} failed`))
      })
      this.ws?.send(JSON.stringify({ type, id, payload }))
    })
  }

  close() {
    this.stopped = true
    if (this.reconnectTimer) window.clearTimeout(this.reconnectTimer)
    this.ws?.close()
    this.ws = null
  }
}
