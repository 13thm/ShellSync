#!/usr/bin/env node
/**
 * 编译 Go daemon 到 daemon/bin/，供 electron-builder 的 extraResources 打包。
 *
 * 用法：
 *   npm run build:daemon                      # 按当前平台编译（Windows -> ssd.exe，其他 -> ssd）
 *   DAEMON_GOOS=linux npm run build:daemon    # 交叉编译（daemon 全部纯 Go，无 CGO 依赖）
 *   DAEMON_GOOS=darwin DAEMON_GOARCH=arm64 npm run build:daemon
 *
 * 产物命名与 desktop/src/main/daemon.ts 的查找约定保持一致：
 *   windows -> bin/ssd.exe   其他平台 -> bin/ssd
 */
import { execSync, spawnSync } from 'node:child_process'
import { mkdirSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const daemonDir = join(here, '..', '..', 'daemon')

const goos = process.env.DAEMON_GOOS || (process.platform === 'win32' ? 'windows' : process.platform)
const goarch = process.env.DAEMON_GOARCH || (process.arch === 'arm64' ? 'arm64' : 'amd64')
const exeName = goos === 'windows' ? 'ssd.exe' : 'ssd'

/** 在 PATH 里探测 go / go.exe（兼容 WSL interop 下只存在 go.exe 的情形）。 */
function findGo() {
  for (const cmd of ['go', 'go.exe']) {
    const r = spawnSync(cmd, ['version'], { stdio: 'ignore' })
    if (r.status === 0) return cmd
  }
  return 'go'
}

mkdirSync(join(daemonDir, 'bin'), { recursive: true })

const env = { ...process.env, CGO_ENABLED: '0', GOOS: goos, GOARCH: goarch }
console.log(`[build-daemon] GOOS=${goos} GOARCH=${goarch} -> daemon/bin/${exeName}`)
execSync(`"${findGo()}" build -trimpath -ldflags "-s -w" -o bin/${exeName} ./cmd/shellsync-daemon`, {
  cwd: daemonDir,
  stdio: 'inherit',
  env,
})
console.log(`[build-daemon] 完成：daemon/bin/${exeName}`)
