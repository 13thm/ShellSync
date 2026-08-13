import { spawn } from 'node:child_process'
import { existsSync, readFileSync } from 'node:fs'
import { homedir } from 'node:os'
import { join } from 'node:path'
import { get } from 'node:http'

export interface DaemonConnection {
  port: number
  token: string
  baseUrl: string
  wsUrl: string
}

interface LockData {
  pid: number
  port: number
  token: string
  startedAt: number
}

const LOCK_PATH = join(homedir(), '.shellsync', 'daemon.lock')

/** Locate the daemon binary across dev / packaged layouts. */
function resolveDaemonPath(): string | null {
  const candidates = [
    process.env.SHELLSYNC_DAEMON,
    // dev: <root>/daemon/bin/*  (this file runs from desktop/out/main)
    join(__dirname, '..', '..', '..', 'daemon', 'bin', 'ssd.exe'),
    join(__dirname, '..', '..', '..', 'daemon', 'bin', 'shellsync-daemon.exe'),
    join(__dirname, '..', '..', '..', 'daemon', 'bin', 'ssd'),
    join(__dirname, '..', '..', '..', 'daemon', 'bin', 'shellsync-daemon'),
    // packaged
    join(process.resourcesPath ?? '', 'daemon', 'ssd.exe'),
    join(process.resourcesPath ?? '', 'daemon', 'shellsync-daemon.exe'),
  ].filter(Boolean) as string[]
  return candidates.find((p) => existsSync(p)) ?? null
}

function readLock(): LockData | null {
  if (!existsSync(LOCK_PATH)) return null
  try {
    return JSON.parse(readFileSync(LOCK_PATH, 'utf-8'))
  } catch {
    return null
  }
}

/** HTTP GET /health with a short timeout; resolves true if the daemon answers. */
function pingHealth(port: number): Promise<boolean> {
  return new Promise((resolve) => {
    const req = get(`http://127.0.0.1:${port}/health`, (res) => {
      res.resume()
      resolve(res.statusCode === 200)
    })
    req.on('error', () => resolve(false))
    req.setTimeout(800, () => {
      req.destroy()
      resolve(false)
    })
  })
}

async function waitForLock(timeoutMs = 12000): Promise<LockData | null> {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    const lock = readLock()
    if (lock && lock.port > 0) {
      if (await pingHealth(lock.port)) return lock
    }
    await new Promise((r) => setTimeout(r, 250))
  }
  return null
}

/**
 * ensureDaemon guarantees a running daemon and returns how to reach it.
 * Reuses an already-running instance; otherwise spawns one detached.
 */
export async function ensureDaemon(): Promise<DaemonConnection> {
  // 1. reuse a live instance if present
  const existing = readLock()
  if (existing && existing.port > 0 && (await pingHealth(existing.port))) {
    return toConnection(existing)
  }

  // 2. spawn a fresh detached daemon
  const exe = resolveDaemonPath()
  if (!exe) {
    throw new Error(
      '未找到 ShellSync daemon 二进制。请先在 daemon/ 下 `go build -o bin/ssd`，' +
        '或设置环境变量 SHELLSYNC_DAEMON 指向它。',
    )
  }
  const child = spawn(exe, [], {
    detached: true,
    stdio: 'ignore',
    windowsHide: true,
  })
  child.unref()

  // 3. wait for it to write its lock and serve health
  const lock = await waitForLock()
  if (!lock) {
    throw new Error('daemon 启动超时：未能在 12s 内就绪。')
  }
  return toConnection(lock)
}

function toConnection(lock: LockData): DaemonConnection {
  return {
    port: lock.port,
    token: lock.token,
    baseUrl: `http://127.0.0.1:${lock.port}`,
    wsUrl: `ws://127.0.0.1:${lock.port}/ws`,
  }
}
