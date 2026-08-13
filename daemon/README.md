# ShellSync Daemon

Go 守护进程，是整个系统的核心与唯一数据源（Single Source of Truth）。
负责：跨平台 PTY 终端托管、SQLite 持久化、终端日志存储、HTTP + WebSocket 接口。

详见 `doc/ShellSync 系统设计说明书.md` §3.1。

## 构建

```bash
cd daemon
go build ./...
```

## 运行

```bash
go run ./cmd/shellsync-daemon
# 或
go build -o bin/shellsync-daemon ./cmd/shellsync-daemon
./bin/shellsync-daemon
```

按 `Ctrl+C` 优雅退出。

## 当前状态（M1-1 骨架）

- [x] 目录结构 / go module
- [x] main 入口（打印 banner + 等待信号）
- [x] M1-2 配置加载
- [ ] M1-3 生命周期 / lock
- [x] M1-4 SQLite + 迁移
- [x] M1-5 repository
- [x] M1-6 pty 抽象
- [x] M1-7 logstore
- [x] M1-8 terminal manager
- [ ] M2 接口层（REST + WS）

## 目录结构

```
daemon/
├── cmd/shellsync-daemon/   # main 入口
└── internal/
    ├── config/             # 配置
    ├── lifecycle/          # 进程生命周期 / lock / 信号
    ├── pty/                # 跨平台 PTY 抽象
    ├── terminal/           # 终端会话管理
    ├── repository/         # SQLite 数据访问层
    │   └── migrations/     # 建表 SQL
    ├── logstore/           # 终端日志存储
    ├── service/            # 领域逻辑
    ├── auth/               # 鉴权
    ├── sync/               # 事件总线
    └── transport/
        ├── http/           # REST API
        └── ws/             # WebSocket Server
```
