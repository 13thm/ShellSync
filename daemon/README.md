# ShellSync Daemon

Go 守护进程，是整个系统的**核心与唯一数据源（Single Source of Truth）**。
负责：跨平台 PTY 终端托管、SQLite 持久化、终端日志存储、HTTP + WebSocket 接口。

详见 `doc/ShellSync 系统设计说明书.md` §3.1。

## 框架与技术栈

| 依赖 | 用途 |
|------|------|
| `go-chi/chi/v5` | HTTP 路由与中间件（REST API 服务器） |
| `go-chi/cors` | 跨域支持（桌面/手机客户端访问） |
| `coder/websocket` | WebSocket 服务器（终端实时流 + 事件推送） |
| `modernc.org/sqlite` | 纯 Go SQLite 驱动（无需 CGO，跨平台编译） |
| `creack/pty` | Unix PTY 终端创建 |
| `UserExistsError/conpty` | Windows ConPTY 终端创建 |
| `google/uuid` | 实体 UUID 主键生成 |
| `log/slog` | 结构化日志 |

**分层架构**（自底向上）：

```
cmd/main（装配与生命周期）
  └─ transport（HTTP/WS 接口层，DTO 转换）
       └─ service（领域层：状态机、事件、配对、鉴权）
            ├─ terminal + pty（终端会话管理，跨平台 PTY 抽象）
            ├─ logstore（终端日志落盘/聚合/归档）
            ├─ repository（SQLite 数据访问层，含迁移）
            └─ eventbus（实体变更事件广播，驱动 WS 推送）
```

## 文件结构（每个文件的用途）

```
daemon/
├── go.mod / go.sum                 # Go module 定义（github.com/shellsync/daemon）与依赖锁定
├── bin/                            # 已编译好的 Windows 可执行文件
│   ├── shellsync-daemon.exe        #   完整名二进制
│   └── ssd.exe                     #   短名二进制（Desktop 自动拉起时优先找它）
├── cmd/
│   └── shellsync-daemon/
│       └── main.go                 # 程序入口：装配所有模块（config → repository → logstore
│                                    #   → terminal → service → auth → HTTP/WS → lifecycle），监听信号优雅退出
└── internal/                       # 私有包（外部无法 import）
    ├── config/
    │   ├── config.go               # 配置加载（端口、数据目录、token 等默认值/环境变量覆盖）
    │   ├── config_test.go          # 配置加载测试
    │   └── doc.go                  # 包说明文档
    ├── lifecycle/
    │   ├── lock.go                 # 单实例锁：写 ~/.shellsync/daemon.lock（pid/port/token），供 Desktop 读取
    │   ├── lock_test.go            # 锁测试
    │   ├── alive_unix.go           # 判断进程是否存活（Unix 实现）
    │   ├── alive_windows.go        # 判断进程是否存活（Windows 实现）
    │   └── doc.go                  # 包说明文档
    ├── repository/                 # SQLite 数据访问层（每张表一个 repo）
    │   ├── db.go                   # 打开数据库连接、公共选项（WAL 等）
    │   ├── migrate.go              # 迁移执行器（按序执行 migrations/*.sql 并记录版本）
    │   ├── migrations/0001_init.sql# 初始建表 SQL（tasks/todos/terminals/devices/logs/settings…）
    │   ├── models.go               # 数据库实体结构体定义
    │   ├── task_repo.go            # 任务 CRUD
    │   ├── todo_repo.go            # 待办 CRUD
    │   ├── terminal_repo.go        # 终端会话持久化
    │   ├── log_repo.go             # 终端日志读写（增量拉取 since 游标）
    │   ├── device_repo.go          # 已配对设备管理（token、吊销）
    │   ├── settings_repo.go        # 键值设置存储
    │   ├── user_repo.go            # 本地用户/owner
    │   ├── *_test.go               # 各 repo 及迁移的单元测试
    │   └── doc.go                  # 包说明文档
    ├── logstore/
    │   ├── store.go                # 终端输出日志存储（原始字节 + 时间戳，base64 传输）
    │   ├── aggregator.go           # 日志聚合（任务视角汇总输出）
    │   ├── archive.go              # 历史日志归档/清理
    │   ├── store_test.go           # 测试
    │   └── doc.go                  # 包说明文档
    ├── pty/
    │   ├── pty.go                  # PTY 接口定义（跨平台抽象：Start/Read/Write/Resize/Wait）
    │   ├── pty_unix.go             # Unix 实现（creack/pty，spawn bash/zsh）
    │   ├── pty_windows.go          # Windows 实现（ConPTY，spawn cmd/pwsh）
    │   ├── pty_test.go             # 测试
    │   └── doc.go                  # 包说明文档
    ├── terminal/
    │   ├── manager.go              # 终端会话管理器：创建/列表/销毁会话，多路复用 PTY 读输出
    │   ├── session.go              # 单个终端会话：绑定 PTY + 任务 + logstore 写入
    │   ├── manager_test.go         # 测试
    │   └── doc.go                  # 包说明文档
    ├── eventbus/
    │   ├── bus.go                  # 实体变更事件总线（发布/订阅），驱动 WS 广播与双端同步
    │   ├── bus_test.go             # 测试
    │   └── doc.go                  # 包说明文档
    ├── auth/
    │   ├── auth.go                 # 鉴权：本地 token（Desktop）+ 设备 token（Mobile），WS/HTTP 中间件校验
    │   ├── auth_test.go            # 测试
    │   └── doc.go                  # 包说明文档
    ├── service/                    # 领域层（组合 repository/terminal/eventbus）
    │   ├── service.go              # Services 聚合根：统一依赖注入容器
    │   ├── task.go                 # 任务领域逻辑（状态机：任务关联终端启停、变更发事件）
    │   ├── todo.go                 # 待办领域逻辑
    │   ├── terminal.go             # 终端领域逻辑（创建/attach/输入/resize）
    │   ├── device.go               # 设备管理（列表/吊销）
    │   ├── pair.go                 # 配对流程（生成二维码 payload shellsync://pair?...、验证 code 换 token）
    │   ├── settings.go             # 设置读写
    │   └── doc.go                  # 包说明文档
    └── transport/
        ├── http/
        │   ├── router.go           # chi 路由注册（全部 /api/* 端点 + health + shutdown）
        │   ├── handlers.go         # 各端点的处理器函数
        │   ├── dto.go              # 请求/响应 DTO 定义与实体转换
        │   ├── middleware.go       # 中间件（鉴权、日志、CORS）
        │   ├── response.go         # 统一响应信封 {code, data, message}
        │   └── doc.go              # 包说明文档
        └── ws/
            ├── hub.go              # WS 连接管理（按设备/会话订阅、广播实体变更事件）
            ├── conn.go             # 单条 WS 连接封装（终端输入输出流、历史回放、请求-响应帧）
            ├── hub_test.go         # 测试
            └── doc.go              # 包说明文档
```

## 构建

```bash
cd daemon
go build ./...
```

## 运行

```bash
go run ./cmd/shellsync-daemon
# 或编译后运行
go build -o bin/shellsync-daemon ./cmd/shellsync-daemon
./bin/shellsync-daemon
```

按 `Ctrl+C` 优雅退出。

## 当前状态（M1 ✅ + M2 ✅ 完成）

- [x] 目录结构 / go module
- [x] main 入口（打印 banner + 等待信号）
- [x] M1-2 配置加载
- [x] M1-3 生命周期 / lock（✅ M1 完成）
- [x] M1-4 SQLite + 迁移
- [x] M1-5 repository
- [x] M1-6 pty 抽象
- [x] M1-7 logstore
- [x] M1-8 terminal manager
- [x] M2-1 service 领域层（状态机 + 事件）
- [x] M2-2 HTTP/REST（全套端点 + health + shutdown）
- [x] M2-3 auth（本地 token + 设备 token）
- [x] M2-4 配对（二维码 + /pair/*）
- [x] M2-5 WebSocket（终端实时流 + 历史回放）
- [x] M2-6 sync 事件总线（实体变更广播）
- [x] M3 Desktop 客户端（见 `../desktop/`）
