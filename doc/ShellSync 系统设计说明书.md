# ShellSync 系统设计说明书

> 基于《双端持久化终端任务管理系统（ShellSync）— 完整需求分析说明书》进行系统架构设计、模块划分、数据库设计与接口定义，作为后续编码的指导蓝图。

---

## 0. 文档约定

| 项 | 约定 |
|---|---|
| 时间格式 | 全部使用 Unix 毫秒时间戳（`int64`，字段后缀 `_at`） |
| 主键 | 业务实体使用 `TEXT`（UUID v4），日志流水使用自增 `INTEGER` |
| 编码 | 终端字符流为 **UTF-8 原始字节（含 ANSI）**，JSON 接口字段为字符串 |
| 字节量 | 终端输出 `data`/`contentB64` 字段统一 **base64 编码** 传输，保证 ANSI 控制序列不损坏 |
| 名词 | Daemon（Go 守护进程）、Desktop（Electron 客户端）、Mobile（Flutter 客户端） |

---

## 1. 系统总体架构

### 1.1 架构总览

```
┌─────────────────────────── 本机（电脑） ───────────────────────────┐
│                                                                    │
│   ┌──────────────────────┐    本地 HTTP/WS     ┌────────────────┐  │
│   │  Desktop UI          │ ◄────(127.0.0.1)──► │  Go Daemon     │  │
│   │  Electron+Vue3       │    IPC + Token      │  (核心/单一     │  │
│   │  + xterm.js          │                     │   数据源)       │  │
│   └──────────────────────┘                     │                │  │
│                                                │ ┌────────────┐ │  │
│                                                │ │ PTY/ConPTY │ │  │
│                                                │ │ Manager    │ │  │
│                                                │ ├────────────┤ │  │
│                                                │ │ SQLite DB  │ │  │
│                                                │ ├────────────┤ │  │
│                                                │ │ Log Store  │ │  │
│                                                │ ├────────────┤ │  │
│                                                │ │ WS Server  │ │  │
│                                                │ └────────────┘ │  │
│   ┌──────────────────────┐                     └───────┬────────┘  │
│   │  OS Shell (cmd/pwsh/ │                            │            │
│   │  bash/zsh) 子进程     │◄──── PTY spawn ────────────┘            │
│   │  含 Claude Code/Pi   │                                         │
│   └──────────────────────┘                                         │
│                                                                    │
└────────────────────────────────┬───────────────────────────────────┘
                                 │  WebSocket（局域网 LAN / 同 WiFi）
                                 │  纯字符流，Token 鉴权
                                 ▼
                        ┌──────────────────┐
                        │  Mobile (Flutter) │
                        │  任务/终端/待办    │
                        │  远程查看与操控    │
                        └──────────────────┘
```

### 1.2 核心设计原则

1. **Daemon 为唯一数据源（Single Source of Truth）**：所有任务、终端、日志、待办数据均以 Daemon 内 SQLite 为权威，Desktop 与 Mobile 均为客户端。
2. **终端托管在 Daemon，UI 可脱离**：PTY 子进程由 Daemon 持有，Desktop UI 退出不影响终端任务；Daemon 重启后依据 DB 恢复元数据（运行中的进程因 OS 已回收需标记为 `crashed`，但日志完整保留）。
3. **纯字符流同步**：绝不投屏、不传图像。仅同步终端 stdin/stdout 字节流 + 元数据变更事件。
4. **Daemon 独立进程**：Electron 启动时拉起（detached）一个独立的 `shellsync-daemon` 可执行文件，进程脱离父进程生命周期；首启后可注册为系统服务（后续迭代），MVP 用 detached + lock 文件 + 本地端口发现。

### 1.3 进程与生命周期模型

```
        ┌───────────── 启动 ─────────────┐
        ▼                                 │
  Electron Main                         │
   ├─ 检测 Daemon 是否存活               │
   │   (读 lock 文件 → ping /health)     │
   │   ├─ 存活：复用                      │
   │   └─ 不存活：spawn detached daemon ─┘
   └─ 连接 Daemon REST/WS，加载 UI

  Go Daemon（独立进程）
   ├─ 监听 127.0.0.1:固定/动态端口（写 lock 文件）
   ├─ 启动 WS Server（同时监听 0.0.0.0:port 供 LAN）
   ├─ 初始化 SQLite
   ├─ 恢复终端元数据（status=crashed 的标记，日志可读）
   └─ 事件循环：管理 PTY、落盘日志、广播 WS
```

- **lock 文件**（`~/.shellsync/daemon.lock`）：记录 `pid`、`port`、`token`，用于 Desktop 发现与鉴权。
- **优雅退出**：Desktop 退出只断开一条本地 WS 连接，Daemon 与所有 PTY 继续；只有系统关机或显式 `POST /api/daemon/shutdown` 才结束 Daemon。

### 1.4 通信架构总表

| 链路 | 协议 | 传输 | 端口 | 鉴权 | 说明 |
|---|---|---|---|---|---|
| Desktop ↔ Daemon | HTTP/REST | JSON | 127.0.0.1:动态 | 本地 token（lock 文件） | 元数据 CRUD、历史日志拉取 |
| Desktop ↔ Daemon | WebSocket | JSON 帧 | 127.0.0.1:动态 | 本地 token | 终端实时输入输出、状态推送 |
| Mobile ↔ Daemon | WebSocket | JSON 帧 | 0.0.0.0:动态（LAN） | 会话 token | 终端实时流 + 数据同步事件 |
| Mobile ↔ Daemon | HTTP/REST | JSON | 同上 | 会话 token | 配对、历史拉取、离线补齐 |

> 说明：MVP 不做外网中继，Mobile 与电脑需处于同一局域网；配对通过 Desktop 展示二维码（含 LAN IP + 端口 + 一次性配对码）完成。

---

## 2. 技术栈确认

| 层 | 技术 | 版本建议 | 选型理由 |
|---|---|---|---|
| 守护进程 | **Go** | 1.22+ | 跨平台、单二进制、GC 稳定、PTY 库成熟 |
| 终端抽象 | `creack/pty` + Windows ConPTY 封装 | latest | 跨平台 PTY；Windows 走 ConPTY |
| 数据库 | **SQLite** | 3.40+（用 `modernc.org/sqlite` 纯 Go 驱动） | 单文件、零运维、并发读 |
| 日志存储 | 文件 + DB 索引 | — | 见 §4.5 存储策略 |
| HTTP 路由 | `go-chi/chi` v5 | latest | 轻量、中间件友好 |
| WebSocket | `coder/websocket`（原 `nhooyr.io/websocket`） | latest | 上下文友好、性能好 |
| Desktop 框架 | **Electron** | 30+ | 跨平台桌面壳，内嵌 Vue3 |
| Desktop UI | **Vue3 + TypeScript** | 3.4+ | 组件化、响应式 |
| 终端前端 | **xterm.js** | 5.x + `xterm-addon-fit` `xterm-addon-search` | ANSI/颜色/光标完整支持 |
| Desktop 状态 | Pinia | latest | — |
| Mobile | **Flutter** | 3.22+（Dart 3.4+） | 跨端（iOS/Android） |
| Mobile 终端渲染 | `xterm.dart` 或自研 ANSI 渲染 | — | 轻量字符终端组件 |
| 配对发现 | 二维码（`go-qrcode`）+ 局域网 IP | — | MVP 不引入 mDNS |

---

## 3. 模块划分

### 3.1 Go Daemon 模块

```
daemon/
├── cmd/shellsync-daemon/      # main 入口
└── internal/
    ├── config/                # 配置加载（端口、数据目录、token）
    ├── pty/                   # ★ 终端抽象层（跨平台 ConPTY/PTY）
    │   ├── pty.go             #   接口定义
    │   ├── pty_unix.go        #   creack/pty 实现
    │   └── pty_windows.go     #   ConPTY 实现
    ├── terminal/              # ★ 终端会话管理（生命周期、读写循环、resize）
    ├── repository/            # ★ SQLite 数据访问层（task/terminal/todo/log）
    │   └── migrations/        #   建表 SQL / 迁移
    ├── logstore/              # ★ 日志落盘与索引（文件分片 + seq 管理）
    ├── service/               # 业务编排（task/todo/terminal 的领域逻辑）
    ├── transport/
    │   ├── http/              # REST API（chi 路由、中间件、handler）
    │   └── ws/                # ★ WebSocket Server（hub + 连接 + 事件广播）
    ├── auth/                  # 本地 token / 配对会话 token
    ├── sync/                  # 数据变更事件总线 & 增量同步
    └── lifecycle/             # 进程守护、lock 文件、信号处理、优雅退出
```

#### 关键模块职责

| 模块 | 职责 | 关键接口（见 §5、§6 详述） |
|---|---|---|
| `pty` | 跨平台启动 shell 子进程，提供 stdin 写入、stdout/stderr 读取、resize、关闭 | `Spawn`, `Read`, `Write`, `Resize`, `Close` |
| `terminal` | 终端会话对象：持有 pty 实例、独立 seq 计数器、读取协程→(落盘 logstore + 广播 ws) | `Session` 结构体、`Manager` 注册表 |
| `logstore` | 按 terminalId 分文件追加写入原始字节；维护 `seq` 单调递增；支持按 seq 区间读取 | `Append`, `ReadRange`, `Tail` |
| `repository` | SQLite CRUD；事务封装；自动迁移 | 各 `Repo` 结构 |
| `service` | 领域逻辑：创建任务/终端/待办、状态流转校验、级联规则 | `TaskService` 等 |
| `transport/ws` | 维护客户端连接池；订阅/取消订阅终端；将终端输出与数据变更事件分发到订阅者 | `Hub`, `Conn` |
| `sync` | 领域事件总线：实体变更产生事件 → WS 广播 → 客户端增量更新 | `Bus`, `Event` |
| `auth` | 生成/校验本地 token（Desktop）与配对会话 token（Mobile） | `Issue`, `Verify` |

### 3.2 Desktop（Electron + Vue3）模块

```
desktop/
├── electron/
│   ├── main/              # 主进程：拉起/守护 Daemon、窗口管理、托盘
│   └── preload/           # 预加载：暴露 IPC 给渲染进程
└── src/                   # Vue3 渲染进程
    ├── api/               # 封装 Daemon REST + WS 客户端
    ├── stores/            # Pinia：tasks/terminals/todos/settings
    ├── views/
    │   ├── Task/          # 任务管理（列表/详情）
    │   ├── Terminal/      # 终端管理（列表 + xterm 容器 + 多标签）
    │   ├── Todo/          # 待办管理
    │   └── Settings/      # 同步设置、配对二维码、Shell 切换
    ├── components/
    │   ├── XTerminal.vue  # xterm.js 封装（attach WS 流、resize、历史回填）
    │   └── ...
    └── router/
```

职责要点：
- **Electron 主进程**负责守护 Daemon（detached spawn）、读 lock 文件、托盘常驻。
- **渲染进程**不直连 PTY，全部经 Daemon REST/WS。
- **XTerminal.vue**：进入终端时先 `GET /logs/tail` 拉历史回填 xterm buffer，再 `subscribe` 接收增量；键盘输入转发 `terminal.input`；窗口 resize 转 `terminal.resize`。

### 3.3 Mobile（Flutter）模块

```
mobile/lib/
├── main.dart
├── api/                   # REST + WS 客户端
├── stores/                # 状态管理（Riverpod / Provider）
├── pages/
│   ├── pairing/           # 扫码配对
│   ├── task/              # 任务列表/详情
│   ├── terminal/          # 终端列表 + 远程终端视图
│   └── todo/              # 待办列表
├── widgets/
│   └── terminal_view.dart # ANSI 渲染 + 输入键盘 + 历史
└── models/                # DTO
```

职责要点：纯远程客户端，无本地终端、无本地数据库（缓存可选）。进入终端同样先拉历史再订阅增量。

---

## 4. 数据库设计

### 4.1 ER 模型

```
┌──────────┐ 1     N ┌────────────┐ 1     N ┌────────────────┐
│  tasks   │─────────│ terminals  │─────────│ terminal_logs  │
│  任务    │         │  终端       │         │  日志(分块)     │
└────┬─────┘         └─────┬──────┘         └────────────────┘
     │ 1                   │ N
     │                     │
     │ N                   └─────────────┐
┌────┴─────┐                      (可选) │
│  todos   │◄─────────────────────────────┘
│  待办    │
└──────────┘

┌──────────┐ 1   N ┌──────────┐         ┌──────────┐
│  users   │───────│ devices  │         │ settings │
└──────────┘       └──────────┘         └──────────┘
```

关系：
- `tasks 1:N terminals`：一个任务可绑多个终端，终端归属唯一任务（`task_id` 可空，表示游离终端）。
- `terminals 1:N terminal_logs`：终端所有 stdin/stdout/system 输出。
- `tasks 1:N todos`：待办可关联任务（`task_id` 可空），可额外关联终端（`terminal_id` 可空）。
- `users 1:N devices`：账号与配对设备（MVP 单 user，预留多账号）。

### 4.2 表结构定义（SQLite DDL）

#### 4.2.1 `users` — 账号

```sql
CREATE TABLE users (
  id           TEXT PRIMARY KEY,          -- UUID
  username     TEXT NOT NULL UNIQUE,
  display_name TEXT,
  created_at   INTEGER NOT NULL,
  updated_at   INTEGER NOT NULL
);
```

#### 4.2.2 `devices` — 已配对设备（Desktop & Mobile）

```sql
CREATE TABLE devices (
  id            TEXT PRIMARY KEY,        -- UUID
  user_id       TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name          TEXT NOT NULL,           -- 设备名，如 "iPhone 15"
  platform      TEXT NOT NULL,           -- desktop | ios | android
  session_token TEXT NOT NULL UNIQUE,    -- 访问 token（散列存储）
  last_seen_at  INTEGER,
  created_at    INTEGER NOT NULL,
  revoked       INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_devices_user ON devices(user_id);
```

#### 4.2.3 `tasks` — 任务

```sql
CREATE TABLE tasks (
  id           TEXT PRIMARY KEY,         -- UUID
  user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name         TEXT NOT NULL,
  description  TEXT,
  status       TEXT NOT NULL DEFAULT 'pending',
               -- pending(未开始) | running(进行中) | paused(已暂停) | done(已完成)
  color        TEXT,                     -- UI 标记色（可选）
  archived     INTEGER NOT NULL DEFAULT 0,
  created_at   INTEGER NOT NULL,
  updated_at   INTEGER NOT NULL
);
CREATE INDEX idx_tasks_user_status ON tasks(user_id, archived, status);
CREATE INDEX idx_tasks_updated     ON tasks(updated_at);   -- 增量同步用
```

#### 4.2.4 `terminals` — 终端实例

```sql
CREATE TABLE terminals (
  id             TEXT PRIMARY KEY,       -- UUID
  user_id        TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  task_id        TEXT REFERENCES tasks(id) ON DELETE SET NULL,  -- 可空
  name           TEXT NOT NULL,
  shell_type     TEXT NOT NULL,          -- cmd | powershell | bash | zsh
  cwd            TEXT,                   -- 启动工作目录
  cols           INTEGER NOT NULL DEFAULT 80,
  rows           INTEGER NOT NULL DEFAULT 24,
  env            TEXT,                   -- JSON 串：额外环境变量
  status         TEXT NOT NULL DEFAULT 'running',
                 -- running | exited | crashed
  exit_code      INTEGER,
  os_pid         INTEGER,                -- 子进程 PID（仅运行时有效）
  last_seq       INTEGER NOT NULL DEFAULT 0,  -- 该终端已写入的最大日志 seq
  created_at     INTEGER NOT NULL,
  last_active_at INTEGER NOT NULL,
  updated_at     INTEGER NOT NULL
);
CREATE INDEX idx_terminals_task    ON terminals(task_id);
CREATE INDEX idx_terminals_status  ON terminals(status);
CREATE INDEX idx_terminals_updated ON terminals(updated_at);
```

#### 4.2.5 `terminal_logs` — 终端日志（核心，高频写入）

```sql
CREATE TABLE terminal_logs (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  terminal_id  TEXT NOT NULL REFERENCES terminals(id) ON DELETE CASCADE,
  seq          INTEGER NOT NULL,         -- 单终端内单调递增（从1开始）
  direction    TEXT NOT NULL,            -- stdout | stderr | stdin | system
  content_b64  TEXT NOT NULL,            -- base64(原始字节)，单条建议 ≤ 64KB
  created_at   INTEGER NOT NULL,
  UNIQUE(terminal_id, seq)
);
CREATE INDEX idx_logs_term_seq ON terminal_logs(terminal_id, seq);
```

> 写入策略见 §4.5。终端输出在 Daemon 内存中按 16ms / 16KB 聚合为一个 chunk，分配一个 `seq` 后批量落盘，避免逐字节写库。

#### 4.2.6 `todos` — 待办

```sql
CREATE TABLE todos (
  id           TEXT PRIMARY KEY,
  user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  task_id      TEXT REFERENCES tasks(id) ON DELETE SET NULL,
  terminal_id  TEXT REFERENCES terminals(id) ON DELETE SET NULL,
  title        TEXT NOT NULL,
  content      TEXT,                     -- 备注
  status       TEXT NOT NULL DEFAULT 'pending',  -- pending | done
  priority     INTEGER NOT NULL DEFAULT 0,        -- 0 普通 / 1 重要 / 2 紧急
  sort_order   INTEGER NOT NULL DEFAULT 0,
  created_at   INTEGER NOT NULL,
  updated_at   INTEGER NOT NULL
);
CREATE INDEX idx_todos_task    ON todos(task_id);
CREATE INDEX idx_todos_status  ON todos(status, sort_order);
CREATE INDEX idx_todos_updated ON todos(updated_at);
```

#### 4.2.7 `settings` — 全局配置（KV）

```sql
CREATE TABLE settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL                    -- JSON 字符串
);
-- 示例 key: default_shell, log_retention_days, theme, auto_start_daemon
```

#### 4.2.8 `sync_cursors` — 各客户端同步游标（可选，离线补齐用）

```sql
CREATE TABLE sync_cursors (
  device_id  TEXT PRIMARY KEY REFERENCES devices(id) ON DELETE CASCADE,
  entity     TEXT NOT NULL,              -- task | terminal | todo
  last_ts    INTEGER NOT NULL            -- 已同步到的 updated_at
);
```

### 4.3 关键约束与级联

| 规则 | 实现 |
|---|---|
| 删除任务时，其下终端不被删（设 `task_id=NULL`） | `ON DELETE SET NULL` |
| 删除终端时，日志级联删除 | `ON DELETE CASCADE` |
| `terminal_logs` 内 `(terminal_id, seq)` 唯一 | `UNIQUE` 约束 + 应用层原子自增 |
| 任务状态流转合法性 | Service 层校验（见 §6.3 状态机） |

### 4.4 并发与事务

- SQLite 开启 WAL 模式：`PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL;` 允许并发读 + 单写。
- 终端日志写入走**单独的批量写事务**（每 chunk 一事务），与元数据写分开，降低锁竞争。
- `last_seq` 自增：写日志前用内存计数器原子 `+1`，配合 `UNIQUE` 约束兜底。

### 4.5 终端日志存储策略（重点）

终端数据具有「**写多、读少、顺序、可分段**」特征，采用 **DB 分块索引 + 原始文件归档** 双层：

1. **热数据（DB）**：近 N 天（默认 7 天，可配置）的 `terminal_logs` 留在 SQLite，支持 Mobile/桌面按 seq 区间快速回放。
2. **冷归档（文件）**：超过阈值的日志按 `terminal_id` 合并追加到 `~/.shellsync/logs/<terminal_id>.log`（原始字节），并从 DB 删除对应行；读取冷数据时流式读文件。
3. **chunk 聚合**：每个终端一个读取 goroutine，用「时间窗口（16ms）或体积窗口（16KB）先到先触发」聚合成一条 `terminal_logs`，兼顾日志可读性与写性能。
4. **永久保留**：默认不删除归档文件；提供 `log_retention_days` 配置项可做清理。

> 该策略满足「所有终端输入输出永久落盘」+「重启可回溯完整过程」+「Mobile 进入即加载全部历史」。

---

## 5. 接口定义 — WebSocket 协议（核心）

> 所有 WS 消息均为 JSON 文本帧，统一信封：

```jsonc
// 客户端 → 服务端
{ "type": "<event>", "id": "<可选-请求Id>", "payload": { ... } }
// 服务端 → 客户端（成功）
{ "type": "<event>", "ref": "<可选-对应请求Id>", "ok": true, "payload": { ... } }
// 服务端 → 客户端（错误）
{ "type": "<event>", "ref": "<id>", "ok": false, "error": { "code": "...", "message": "..." } }
```

### 5.1 连接与鉴权

- 端点：
  - Desktop：`ws://127.0.0.1:{port}/ws?token={local_token}`
  - Mobile：`ws://{lan_ip}:{port}/ws?token={session_token}`
- 鉴权失败立即关闭连接（close code 4001）。

### 5.2 终端实时流事件

| 方向 | type | payload | 说明 |
|---|---|---|---|
| C→S | `terminal.subscribe` | `{terminalId}` | 订阅某终端输出；服务端先推送历史快照事件 |
| S→C | `terminal.history` | `{terminalId, fromSeq, toSeq, chunks:[{seq,direction,contentB64,createdAt}]}` | 历史日志（可分多条，按 seq 升序；大历史分页） |
| C→S | `terminal.history.fetch` | `{terminalId, fromSeq, limit}` | 主动拉取更早历史（向上滚动加载） |
| C→S | `terminal.input` | `{terminalId, dataB64}` | 向终端 stdin 写入字节 |
| C→S | `terminal.resize` | `{terminalId, cols, rows}` | 调整 PTY 尺寸 |
| S→C | `terminal.output` | `{terminalId, seq, direction, contentB64, createdAt}` | 实时输出（每条对应一个 chunk/seq） |
| S→C | `terminal.status` | `{terminalId, status, exitCode?}` | 运行/退出/崩溃 |
| C→S | `terminal.unsubscribe` | `{terminalId}` | 取消订阅 |

### 5.3 数据同步事件（实体变更广播）

> 任意端通过 REST 修改实体后，Daemon 向所有已连接客户端广播对应事件，便于其他端实时更新 UI。

| 方向 | type | payload |
|---|---|---|
| S→C | `task.created` | `{task: TaskDTO}` |
| S→C | `task.updated` | `{task: TaskDTO}` |
| S→C | `task.deleted` | `{id}` |
| S→C | `terminal.created` / `updated` / `deleted` | 同上 |
| S→C | `todo.created` / `updated` / `deleted` | 同上 |

### 5.4 心跳

- C→S `ping`（每 25s）→ S→C `pong`；60s 无心跳断开。

---

## 6. 接口定义 — REST API

> Base URL：Desktop 用 `http://127.0.0.1:{port}`，Mobile 用 `http://{lan_ip}:{port}`。请求头 `Authorization: Bearer {token}`。响应统一信封：

```jsonc
{ "code": 0, "data": { ... }, "message": "ok" }    // 成功
{ "code": 40001, "data": null, "message": "..." }  // 失败
```

### 6.1 认证与配对

| Method | Path | 入参 | 出参 `data` | 说明 |
|---|---|---|---|---|
| POST | `/api/pair/init` | — | `{pairingCode, qrPayload, expiresAt}` | Desktop 生成一次性配对码与二维码内容 |
| POST | `/api/pair/verify` | `{pairingCode, deviceName, platform}` | `{sessionId, sessionToken, user}` | Mobile 扫码后换取会话 token |
| POST | `/api/auth/refresh` | — | `{sessionToken}` | 刷新 token（可选） |
| GET | `/api/devices` | — | `[DeviceDTO]` | 列出已配对设备 |
| DELETE | `/api/devices/:id` | — | `{}` | 吊销设备 |

### 6.2 任务 Task

| Method | Path | 入参 | 出参 | 说明 |
|---|---|---|---|---|
| GET | `/api/tasks` | query: `?status=&archived=&since=` | `[TaskDTO]` | 列表，`since` 为 updated_at 增量同步 |
| POST | `/api/tasks` | `{name, description?, color?}` | `TaskDTO` | 创建 |
| GET | `/api/tasks/:id` | — | `TaskDTO` | 详情（含 terminals/todos 概要） |
| PATCH | `/api/tasks/:id` | `{name?, description?, status?, color?, archived?}` | `TaskDTO` | 更新 |
| DELETE | `/api/tasks/:id` | — | `{}` | 删除（其终端 task_id 置空） |

**TaskDTO**
```jsonc
{
  "id": "uuid", "name": "string", "description": "string|null",
  "status": "pending|running|paused|done", "color": "string|null",
  "archived": false, "terminalCount": 2, "todoCount": 5,
  "createdAt": 0, "updatedAt": 0
}
```

### 6.3 任务状态机

```
pending ──start──► running ──pause──► paused ──resume──► running
   │                   │                                   │
   └───────────────────┴──────────── complete ───────────► done
   （任意态可 → done；done 可重新置 running）
```
非法流转返回 `code=40909 CONFLICT`。

### 6.4 终端 Terminal

| Method | Path | 入参 | 出参 | 说明 |
|---|---|---|---|---|
| GET | `/api/terminals` | query: `?taskId=&status=&since=` | `[TerminalDTO]` | 列表 |
| POST | `/api/terminals` | `{taskId?, name?, shellType, cwd?, cols?, rows?, env?}` | `TerminalDTO` | 创建并 spawn PTY |
| GET | `/api/terminals/:id` | — | `TerminalDTO` | 详情 |
| PATCH | `/api/terminals/:id` | `{name?, taskId?}` | `TerminalDTO` | 改名/改归属 |
| POST | `/api/terminals/:id/resize` | `{cols, rows}` | `{}` | resize（也可走 WS） |
| POST | `/api/terminals/:id/restart` | — | `TerminalDTO` | 用相同配置重启 PTY |
| DELETE | `/api/terminals/:id` | query: `?keepLogs=true` | `{}` | 关闭终端（默认保留日志） |

**TerminalDTO**
```jsonc
{
  "id": "uuid", "taskId": "uuid|null", "name": "string",
  "shellType": "powershell", "cwd": "E:\\code",
  "cols": 120, "rows": 32,
  "status": "running|exited|crashed", "exitCode": null,
  "lastSeq": 12345,
  "createdAt": 0, "lastActiveAt": 0, "updatedAt": 0
}
```

### 6.5 终端历史日志

| Method | Path | 入参 | 出参 | 说明 |
|---|---|---|---|---|
| GET | `/api/terminals/:id/logs` | query: `?fromSeq=1&limit=500&direction=` | `{terminalId, chunks:[LogChunk], hasMore}` | 按 seq 升序分页 |
| GET | `/api/terminals/:id/logs/tail` | query: `?limit=500` | 同上 | 取最新 N 条（进入终端首屏） |

**LogChunk**
```jsonc
{
  "seq": 1,
  "direction": "stdout|stderr|stdin|system",
  "contentB64": "base64...",
  "createdAt": 0
}
```

> 大量历史优先走 REST 分页；进入终端首屏用 `tail`；WS 订阅后增量推送。

### 6.6 待办 Todo

| Method | Path | 入参 | 出参 |
|---|---|---|---|
| GET | `/api/todos` | query: `?taskId=&status=&since=` | `[TodoDTO]` |
| POST | `/api/todos` | `{title, content?, taskId?, terminalId?, priority?}` | `TodoDTO` |
| PATCH | `/api/todos/:id` | `{title?, content?, status?, priority?, taskId?, terminalId?, sortOrder?}` | `TodoDTO` |
| DELETE | `/api/todos/:id` | — | `{}` |

**TodoDTO**
```jsonc
{
  "id": "uuid", "taskId": "uuid|null", "terminalId": "uuid|null",
  "title": "string", "content": "string|null",
  "status": "pending|done", "priority": 0,
  "sortOrder": 0, "createdAt": 0, "updatedAt": 0
}
```

### 6.7 系统与设置

| Method | Path | 入参 | 出参 | 说明 |
|---|---|---|---|---|
| GET | `/api/health` | — | `{ok, version, uptime}` | 探活（Desktop 启动检测用） |
| GET | `/api/settings` | — | `{...kv}` | 读取配置 |
| PATCH | `/api/settings` | `{...kv}` | `{...kv}` | 更新配置 |
| GET | `/api/shells` | — | `[{type, path, available}]` | 列出可用 shell |
| POST | `/api/daemon/shutdown` | — | `{}` | 优雅关闭 Daemon（需确认） |

### 6.8 错误码

| code | 含义 |
|---|---|
| 0 | 成功 |
| 40001 | 未授权 / token 无效 |
| 40003 | 禁止访问（设备已吊销） |
| 40404 | 资源不存在 |
| 40901 | 参数校验失败 |
| 40909 | 状态冲突（非法状态流转） |
| 40910 | 配对码无效/已过期 |
| 50000 | 服务器内部错误 |
| 50001 | PTY 启动失败 |

---

## 7. 关键业务流程

### 7.1 创建终端并交互（Desktop）

```
Desktop                  Daemon                   OS Shell
  │  POST /api/terminals   │                          │
  │ ─────────────────────► │ spawn PTY (ConPTY/pty)   │
  │  {id, status=running}  │ ───────────────────────► │
  │ ◄───────────────────── │                          │
  │  ws connect + subscribe│                          │
  │ ─────────────────────► │ 推送 history(tail)        │
  │ ◄───────────────────── │ 读取 goroutine 持续读 stdout
  │  terminal.output (xN)  │  → 聚合 chunk → 落盘 DB   │
  │ ◄───────────────────── │  → 广播 terminal.output  │
  │  terminal.input        │                          │
  │ ─────────────────────► │ 写 stdin ──────────────► │
  关闭 Desktop 窗口：仅断开本地 WS，PTY 继续运行、日志继续落盘
```

### 7.2 关闭窗口与重启恢复

1. 关闭 Desktop UI → Daemon 与所有 PTY 不受影响。
2. 重启电脑 / 重启 Daemon → 读取 `terminals` 表，将 `status=running` 的置为 `crashed`，保留全部日志；用户可在 UI 点 `restart` 重新 spawn（命令历史可从日志回看）。

### 7.3 手机远程操控终端

```
Mobile 扫码配对 → POST /api/pair/verify → 拿到 sessionToken
  │
  │ GET /api/terminals           （列表）
  │ ws connect + terminal.subscribe(id)
  │ ◄── terminal.history（历史回放，分页）
  │ ◄── terminal.output（实时增量）
  │  terminal.input（手机键盘输入干预 AI）──►
```

### 7.4 双端数据同步

- **实时**：任一端 REST 写入 → Daemon `sync` 总线 → WS 广播 `*.created/updated/deleted` → 其他端 UI 增量更新。
- **断线重连补齐**：客户端记录本地 `lastSyncTs`，重连后 `GET /api/{entity}?since={lastSyncTs}` 拉取增量；终端流则按 `seq` 续传（订阅时带 `fromSeq`）。

---

## 8. 工程目录结构（Monorepo 建议）

```
ShellSync/
├── doc/                       # 需求/设计文档
├── daemon/                    # Go 守护进程
│   ├── cmd/shellsync-daemon/
│   ├── internal/...           # 见 §3.1
│   ├── go.mod
│   └── README.md
├── desktop/                   # Electron + Vue3
│   ├── electron/
│   ├── src/
│   ├── package.json
│   └── vite.config.ts
├── mobile/                    # Flutter
│   ├── lib/
│   └── pubspec.yaml
├── proto/                     # 接口 DTO / WS 事件定义（可选，JSON Schema 或 TS 类型）
├── scripts/                   # 构建/打包脚本
└── README.md
```

- 建议在 `proto/` 用 TypeScript 类型（`types.ts`）+ Go 结构体（`dto.go`）双份定义，保持字段一致；后续可引入 `quicktype` 或 `buf` 统一生成。

---

## 9. 非功能性设计落地

| 需求 | 设计支撑 |
|---|---|
| 终端后台运行数小时不卡死 | Go goroutine 读循环 + chunk 聚合 + WAL；PTY 独立于 UI |
| 全平台 + Windows ConPTY | `pty` 抽象层，Windows 走 ConPTY，Unix 走 `creack/pty` |
| ANSI 颜色/光标/进度条 | xterm.js 前端 + 原始字节透传，不做任何字符改写 |
| 终端日志永久保存 | DB 热数据 + 文件冷归档（§4.5） |
| 双端同步延迟 0~2s | WebSocket 实时推送；chunk 聚合窗口 16ms |
| 重启不丢元数据 | 全部元数据落 SQLite |
| 手机进入即看全部历史 | `tail` + `history.fetch` 分页回放，按 seq 严格有序 |

---

## 10. 迭代边界（与需求「非需求」对齐）

- **MVP 范围**：单账号、单电脑、局域网 Mobile、任务/终端/待办/日志全功能、双端实时同步。
- **明确不做（MVP）**：外网中继、多用户/权限分享、接管系统外部终端窗口、远程桌面投屏、手机本地终端。
- **后续迭代预留**：`users` 多账号表、`sync_cursors`、Daemon 注册为系统服务、外网中继服务器（独立 Relay 项目）、端到端加密。

---

## 11. 后续工作分解（编码蓝图）

1. **Daemon 骨架**：`lifecycle` + `config` + `http/health` + lock 文件机制。
2. **数据层**：SQLite 迁移 + `repository` 各 Repo + 种子 user。
3. **终端核心**：`pty` 抽象 → `terminal.Manager` → `logstore` chunk 落盘。
4. **REST API**：task/terminal/todo/logs 完整 CRUD + 鉴权中间件。
5. **WS Server**：hub + subscribe/input/output + 历史回放 + 数据变更广播。
6. **Desktop**：Electron 守护 Daemon + Vue3 四大模块 + XTerminal.vue。
7. **Mobile**：扫码配对 + 四大页面 + 远程终端视图。
8. **联调与稳定性**：长时压力测试、断线重连、日志归档验证。

> 每一步均可独立交付与验证，建议按此顺序推进编码。
