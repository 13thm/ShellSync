# dev 三件套:relay-server + daemon(cloud 指向本机 relay)
# 用法:  powershell -ExecutionPolicy Bypass -File scripts/dev-relay.ps1
# 结束:  Ctrl+C 退出 daemon;relay 需另行停止(taskkill /F /IM relay-server.exe 或关窗口)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot

# 1) relay-server(127.0.0.1:8788)
Write-Host "==> starting relay-server on 127.0.0.1:8788" -ForegroundColor Cyan
$relay = Start-Process -PassThru -NoNewWindow -FilePath "go" `
  -ArgumentList "run ./cmd/relay-server" -WorkingDirectory "$root/server"
Start-Sleep -Seconds 3

# 2) daemon(独立数据目录,cloud.url 默认 ws://127.0.0.1:8788/ws)
Write-Host "==> starting daemon (data: $env:TEMP\ssd-relay-dev)" -ForegroundColor Cyan
$env:SHELLSYNC_DATA_DIR = Join-Path $env:TEMP "ssd-relay-dev"
try {
  Push-Location "$root/daemon"
  go run ./cmd/shellsync-daemon
} finally {
  Pop-Location
  if (!$relay.HasExited) { Stop-Process -Id $relay.Id -Force -ErrorAction SilentlyContinue }
}
