# 把 frpc 注册为 Windows 计划任务（登录自启、后台隐藏运行），让中转隧道常驻。
#
# 用法（管理员 PowerShell，按你的实际路径调整）：
#   powershell -ExecutionPolicy Bypass -File install-frpc-task.ps1 `
#       -FrpcExe  "C:\Tools\frp\frpc.exe" `
#       -ConfigPath "E:\code\app\ShellSync\deploy\frpc.toml"
#
# 管理：
#   手动启动：  schtasks /run /tn "ShellSync-frpc"
#   停止：      schtasks /end /tn "ShellSync-frpc"
#   卸载：      schtasks /delete /tn "ShellSync-frpc" /f
param(
    [Parameter(Mandatory = $true)][string]$FrpcExe,
    [Parameter(Mandatory = $true)][string]$ConfigPath
)

$ErrorActionPreference = 'Stop'
$taskName = 'ShellSync-frpc'

if (-not (Test-Path $FrpcExe))     { throw "frpc.exe 不存在：$FrpcExe（先从 https://github.com/fatedier/frp/releases 下载 windows_amd64 版）" }
if (-not (Test-Path $ConfigPath))  { throw "配置文件不存在：$ConfigPath" }

# 已存在同名任务则先删除
schtasks /query /tn $taskName *> $null
if ($LASTEXITCODE -eq 0) {
    schtasks /delete /tn $taskName /f | Out-Null
}

$frpc = $FrpcExe -replace '"', '""'
$cfg  = $ConfigPath -replace '"', '""'

schtasks /create `
    /tn $taskName `
    /tr "`"$frpc`" -c `"$cfg`"" `
    /sc onlogon `
    /rl limited `
    /f | Out-Null

Write-Host "已创建计划任务 [$taskName]（当前用户登录时自启）"
schtasks /run /tn $taskName
Write-Host "已启动。验证：tasklist | findstr frpc"
