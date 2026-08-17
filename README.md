# ShellSync



**双端持久化终端任务管理系统** —— 一个由 Go 守护进程 + Electron 桌面端 + Flutter 手机端组成的局域网终端管理工具。

## 项目组成

| 目录 | 项目 | 技术栈 | 说明 |
|------|------|--------|------|
| `daemon/` | Go 守护进程 | Go 1.25 + chi + modernc.org/sqlite | 系统核心与唯一数据源：PTY 终端托管、SQLite 持久化、日志存储、HTTP + WebSocket 接口；含云中继出站客户端 |
| `desktop/` | 桌面客户端 | Electron 31 + Vue 3 + Pinia + xterm.js | 电脑端 UI，自动拉起并连接 daemon，本地 HTTP/WS 通信 |
| `mobile/` | 手机客户端 | Flutter + provider + xterm | 扫码配对电脑端，远程查看/操控任务、待办、终端；局域网优先→云中继回落 |
| `server/` | 云中继服务 | Go + chi + coder/websocket | 官方 relay-server：配对码路由 + 透明字节隧道（见 [server/README.md](./server/README.md)） |
| `deploy/` | 跨网络部署 | frp + Docker Compose / Caddy | 自托管 frp 中转 + 官方 relay 生产部署（见 [deploy/README.md](./deploy/README.md) 与 [deploy/relay/](./deploy/relay/)） |
| `doc/` | 文档 | Markdown + CSS/Vue 设计规范 | 需求分析、系统设计、开发任务规划、视觉设计规范、跨网络改造方案 |

```
                ┌────────────────┐  本地 HTTP/WS   ┌────────────────┐
                │  Desktop       │ ◄──(127.0.0.1)─►│  Go Daemon     │
                │  Electron+Vue3 │    Token        │  PTY + SQLite  │
                └────────────────┘                 └───┬────────┬───┘
                                              WebSocket(LAN) │  WSS 出站(多路复用)
                                                    ▼         ▼
                                          ┌────────────┐  ┌──────────────┐
                                          │  Mobile    │◄─┤ relay-server │
                                          │  Flutter   │  │  云中继(公网) │
                                          └────────────┘  └──────────────┘
```

## 快速启动

详见 **[start.md](./start.md)**。

各子项目的文件结构与框架说明见各自的 `README.md`：

- [daemon/README.md](./daemon/README.md)
- [desktop/README.md](./desktop/README.md)
- [mobile/README.md](./mobile/README.md)
- [doc/README.md](./doc/README.md)

## 根目录文件

| 文件 | 用途 |
|------|------|
| `README.md` | 本文件，项目总览 |
| `start.md` | 各项目的环境准备与启动步骤 |
| `setup-flutter-env.ps1` | Windows 下配置 Flutter 环境变量（PATH + 国内镜像 PUB_HOSTED_URL）的一次性脚本 |

