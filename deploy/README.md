# ShellSync 跨网络部署（frp 中转）

> 解决的问题：手机不在电脑同一 WiFi（如在外面用流量）时无法连接。
> 原理：加一台有公网 IP 的 VPS 做中转，**不需要改动任何现有代码**。

```
  手机（任意网络）                你的 VPS（公网）                你的电脑
  ┌──────────┐   公网:18787   ┌──────────┐   frp隧道(7000)  ┌──────────────┐
  │  Mobile  │ ◄────────────► │   frps   │ ◄──────────────► │ frpc → daemon│
  └──────────┘   HTTP/WS      │ (Docker) │   出站连接       │ 127.0.0.1:8787│
                              └──────────┘   (无需公网入站) └──────────────┘
```

手机访问 `http://<VPS IP>:18787` = 直接访问你电脑上的 daemon。
电脑上的 frpc 是**出站连接**，所以家里路由器无需公网 IP、无需端口转发（CGNAT 也能穿透）。

---

## 步骤一：VPS 上部署 frps（服务端）

前置：一台有公网 IP 的 Linux VPS（阿里云/腾讯云轻量即可，1 核 512M 足够），已装 Docker。

```bash
# 1. 把本目录（deploy/）上传到 VPS，例如 /opt/shellsync-relay/
#    （scp -r deploy/ root@你的VPS:/opt/shellsync-relay）

# 2. 改配置
vim frps.toml        # 必改 auth.token（openssl rand -hex 16 生成）

# 3. 启动（开机自启由 restart: unless-stopped + Docker 服务保证）
docker compose up -d
docker compose logs -f frps    # 看到 "frps started successfully" 即可
```

**放行安全组/防火墙端口：**

| 端口 | 用途 |
|------|------|
| 7000 | frp 控制隧道（仅 frpc 连） |
| 18787 | ShellSync daemon 中转端口（手机访问） |

## 步骤二：电脑上运行 frpc（客户端）

### 2.1 固定 daemon 端口

frp 转发需要固定端口，而 daemon 默认随机端口。编辑 `~/.shellsync/config.json`
（Windows 即 `C:\Users\<你>\.shellsync\config.json`）：

```json
{ "httpPort": 8787 }
```

然后重启 daemon（托盘退出 ShellSync → 结束 ssd.exe → 重新打开 ShellSync；
或直接 `taskkill /im ssd.exe /f` 后重启应用）。

### 2.2 安装并运行 frpc

1. 从 [frp releases](https://github.com/fatedier/frp/releases) 下载 `frp_*_windows_amd64.zip`，
   解压出 `frpc.exe`（例如放到 `C:\Tools\frp\frpc.exe`）；
2. 编辑本目录 `frpc.toml`：填 `serverAddr`（VPS 公网 IP）和与 frps 相同的 `auth.token`；
3. 测试运行（保持窗口开着）：

   ```powershell
   C:\Tools\frp\frpc.exe -c E:\code\app\ShellSync\deploy\frpc.toml
   # 看到 "login to server success" 即成功
   ```

4. 注册为开机自启（登录后后台常驻）：

   ```powershell
   powershell -ExecutionPolicy Bypass -File deploy\install-frpc-task.ps1 `
       -FrpcExe "C:\Tools\frp\frpc.exe" `
       -ConfigPath "E:\code\app\ShellSync\deploy\frpc.toml"
   ```

### 2.3 验证中转

在 VPS 上（或任何外网机器）：

```bash
curl http://<VPS IP>:18787/health
# 返回 200 / OK 即通
```

## 步骤三：手机端连接（跨网络配对）

电脑端 ShellSync「设置 → 生成配对码」后，手机端：

1. 如果之前配对过（存的是局域网地址），先在手机 App 里清除/重新配对；
2. 选「**手动输入**」，填：
   - IP：`<VPS 公网 IP>`（不是电脑的 192.168.x.x！）
   - 端口：`18787`
   - 配对码：电脑上显示的 6 位码
3. 配对成功后，手机就**永远走 VPS**——在家、在公司、用流量，都是同一个地址，无需切换。

> 桌面端不受影响：它始终连 `127.0.0.1`，不经过 VPS。

---

## 常见问题

| 问题 | 处理 |
|------|------|
| frpc 报 `login to server error` | 两边 `auth.token` 不一致，或 VPS 安全组没放行 7000 |
| frpc 连上但手机连不通 | 检查 VPS 18787 放行；确认 daemon 已固定 `httpPort: 8787` 且重启过 |
| 手机一直转圈 | 电脑休眠会断隧道；电源设置里允许网络唤醒或保持唤醒状态 |
| 想换端口 | 同步改 `frps.toml` 的 allowPorts、`frpc.toml` 的 remotePort、VPS 安全组、手机端输入 |
| 电脑重启后隧道没恢复 | 检查计划任务是否创建（`schtasks /query /tn ShellSync-frpc`）；frpc 自带断线重连 |

## ⚠️ 安全须知（必读）

把 daemon 暴露到公网后，攻击面变化如下：

1. **数据接口是安全的**：`/api/*`（除 pair）和 `/ws` 都要 token（32 字节随机数），无法伪造。
2. **配对接口存在暴力枚举窗口**：`/api/pair/verify` 无限速，配对码仅 6 位数字、2 分钟有效。
   公网暴露后理论上可在有效期内穷举。缓解措施：
   - remotePort 用不常见值（别用 8080/8888 这类常被扫的端口）；
   - 不用时停掉 frpc（`schtasks /end /tn ShellSync-frpc`）；
   - VPS 安全组可把 18787 限制为手机常用的出口网段（运营商 CGNAT 段，如需我可以帮你查）。
   - **后续开发项**：给 daemon 加 pair verify 限速 + 失败锁定（本地局域网无所谓，公网必须加）。
3. **frp 自身**：`auth.token` 用强随机值；frps 管理面板默认不开，要开必须改弱口令。
4. **传输加密**：daemon 是明文 HTTP/WS。frp 的 `transport.tls` 只加密 frpc↔frps 控制连接，
   手机↔VPS 这段仍是明文。终端内容和 token 会经过 VPS（frps 只转发 TCP，不解析内容）。
   高安全需求请等 HTTPS/WSS 支持，或用 Tailscale 方案。

## 备选方案

| 方案 | 优缺点 |
|------|--------|
| **Tailscale/ZeroTier**（不自己买 VPS） | 电脑和手机都装 App 组虚拟局域网，加密好、零配置；缺点：依赖第三方、手机常驻 VPN |
| **自研 outbound relay**（长期正解） | daemon 主动连云服务器，手机也连服务器，服务器按 deviceId 转发 WS 帧。无需公网暴露端口、天然穿 CGNAT、可加端到端加密；需要写代码（可作为 daemon 的新 transport 模块），有需要我可以设计实现 |
