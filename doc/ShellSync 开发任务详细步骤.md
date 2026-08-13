# ShellSync 开发任务详细步骤

> 配套《开发任务规划》，把每个任务拆成可照做执行的最小步骤。
> 标注约定：🔧 命令 ｜ 📁 文件 ｜ 📦 依赖 ｜ ✅ 完成标志 ｜ 🧪 自测

---

# M1. Daemon 基础

## M1-1 初始化 Go module + 目录骨架

**目标**：建立 daemon 工程骨架，`go build` 通过。**预估**：0.5d

**步骤**：
1. 🔧 进入项目根目录建 daemon 子目录
   ```bash
   mkdir -p daemon/cmd/shellsync-daemon
   cd daemon
   go mod init github.com/<you>/shellsync/daemon
   ```
2. 📁 按设计 §3.1 创建 internal 包目录
   ```
   daemon/internal/{config,lifecycle,pty,terminal,repository,logstore,service,auth,sync}
   daemon/internal/repository/migrations
   daemon/internal/transport/{http,ws}
   ```
3. 📁 `daemon/cmd/shellsync-daemon/main.go` 写最小 main：打印版本并 `select{}` 阻塞
4. 📦 引入基础库：`go get github.com/google/uuid`
5. 📁 每个包建一个 `doc.go` 写包注释占位，便于后续补全
6. 🔧 `go build ./...` 与 `go vet ./...` 通过
7. 📁 建 `daemon/.gitignore`（忽略二进制、`*.db`、`*.db-wal`）

✅ **完成标志**：目录结构齐全，编译无错，可生成 `shellsync-daemon` 二进制。

---

## M1-2 config 配置加载

**目标**：读取配置，缺省值合理。**预估**：0.5d **依赖**：M1-1

**步骤**：
1. 📦 `go get github.com/spf13/viper`（或用标准库 `encoding/json` 轻量实现）
2. 📁 `internal/config/config.go` 定义结构体：
   ```go
   type Config struct {
     DataDir      string // ~/.shellsync
     HTTPPort     int    // 0 = 动态分配
     WSPort       int
     LogLevel     string // info
     DBPath       string // DataDir/shellsync.db
     LogRetention int    // 天
   }
   ```
3. ✍️ 实现 `Load()`：优先 `~/.shellsync/config.json`，不存在则用默认值并写回
4. ✍️ 实现 `DataDir` 自动创建（`MkdirAll`）
5. 🧪 单测：缺省值正确、自定义覆盖、目录自动建

✅ **完成标志**：首次运行生成 `~/.shellsync/config.json` + 目录。

---

## M1-3 lifecycle：lock 文件 + 单实例 + 优雅退出

**目标**：daemon 单实例运行，可被复用与优雅关闭。**预估**：1d **依赖**：M1-2

**步骤**：
1. 📁 `internal/lifecycle/lock.go`：定义 lock 文件结构
   ```go
   type LockData struct { PID int; Port int; Token string; StartedAt int64 }
   ```
2. ✍️ `Acquire()`：若 lock 存在 → 探活 `GET /health`（用 token）→ 存活则退出提示"已在运行"，否则抢占
3. ✍️ 启动后写 lock（含动态端口 + 随机 token，用 `crypto/rand`）
4. ✍️ `Release()`：退出时删 lock（用 `os.Remove`，注意 PID 校验防误删）
5. 📁 `internal/lifecycle/signal.go`：`signal.Notify(SIGINT, SIGTERM)` → 触发 `context.Cancel` + 优雅关闭（停 HTTP、关 PTY、flush 日志）
6. 🧪 手测：启动两次，第二次提示已在运行；Ctrl+C 后 lock 被清理

✅ **完成标志**：单实例保证 + 信号优雅退出。

---

## M1-4 SQLite 初始化 + WAL + 自动迁移

**目标**：启动即建 8 张表。**预估**：1d **依赖**：M1-1

**步骤**：
1. 📦 `go get modernc.org/sqlite`（纯 Go，无 CGO，跨平台省心）
2. 📁 `internal/repository/db.go`：`Open(path)` 打开连接，设置
   ```sql
   PRAGMA journal_mode=WAL;
   PRAGMA synchronous=NORMAL;
   PRAGMA foreign_keys=ON;
   PRAGMA busy_timeout=5000;
   ```
3. 📁 `internal/repository/migrations/0001_init.sql`：把设计 §4.2 的 8 张表 DDL 全部写入（users/devices/tasks/terminals/terminal_logs/todos/settings/sync_cursors）
4. ✍️ 迁移执行器：用 `schema_version` 表记录已应用版本，按序跑 SQL 文件（embed `//go:embed migrations/*.sql`）
5. ✍️ 种子数据：无 user 时插入默认单用户 `local`
6. 🧪 单测：建表幂等、外键生效、WAL 文件生成

✅ **完成标志**：启动后 `shellsync.db` 含全部表 + 默认用户。

---

## M1-5 repository：各实体 Repo（CRUD）

**目标**：全套数据访问层 + 单测。**预估**：2d **依赖**：M1-4

**步骤**：
1. 📁 按实体建 Repo：`task_repo.go / terminal_repo.go / todo_repo.go / log_repo.go / device_repo.go / user_repo.go / settings_repo.go`
2. ✍️ 每个 Repo 实现接口（以 Task 为例）：
   ```go
   List(userID, filter) ([]Task, error)
   Get(id) (Task, error)
   Create(Task) (Task, error)      // 自动填 id/created_at/updated_at
   Update(id, patch) (Task, error) // 更新 updated_at
   Delete(id) error
   ListSince(userID, sinceTs) ([]Task, error) // 增量同步
   ```
3. ✍️ `log_repo`：`AppendChunk(terminalID, seq, dir, b64, ts)` + `ReadRange(terminalID, fromSeq, limit)` + `Tail(terminalID, limit)` + `MaxSeq(terminalID)`
4. ✍️ 时间戳/JSON 字段统一在 Repo 内转换（DB int64 ↔ DTO）
5. ✍️ 所有写操作用 `context` 传超时；`terminal_logs` 单条一事务
6. 🧪 用内存 SQLite（`:memory:`）写表驱动单测，覆盖 CRUD + 级联 + 增量

✅ **完成标志**：单测全绿，覆盖各实体增删改查与外键级联。

---

## M1-6 pty 跨平台抽象（Unix + Windows ConPTY）

**目标**：能 spawn 四种 shell 并读写。**预估**：2d **依赖**：M1-1

**步骤**：
1. 📦 `go get github.com/creack/pty`（Unix）；Windows 评估 `github.com/UserExistsError/conpty` 或 `github.com/microsoft/go-crypto-conpty`
2. 📁 `internal/pty/pty.go` 定义接口：
   ```go
   type PTY interface {
       Write(p []byte) (int, error)
       Read(p []byte) (int, error)
       Resize(cols, rows int) error
       Close() error
       PID() int
   }
   type SpawnOpts struct { Shell string; Cwd string; Cols, Rows int; Env map[string]string }
   func Spawn(opts SpawnOpts) (PTY, error)
   ```
3. 📁 `pty_unix.go`（`//go:build !windows`）：用 `creack/pty.Start` 启动 `cmd.exe/bash/zsh`，按 `opts.Shell` 选择可执行
4. 📁 `pty_windows.go`（`//go:build windows`）：用 ConPTY，启动 `cmd.exe`/`powershell.exe`/`pwsh.exe`
5. ✍️ shell 路径解析：先查 `opts.Shell` 是否绝对路径，否则按 `SHELL`/`ComSpec` 环境变量或默认值
6. ✍️ `Close()` 区分正常关闭与 kill（先关 stdin 触发 EOF，超时再 kill）
7. 🧪 三平台手测：spawn → 写 `echo hi\n` → 读到输出 → resize → close

✅ **完成标志**：Windows/macOS/Linux 都能跑通四类 shell 的读写与 resize。

---

## M1-7 logstore：chunk 聚合 + seq + 落盘

**目标**：高频输出按 chunk 入库，seq 单调。**预估**：1.5d **依赖**：M1-5

**步骤**：
1. 📁 `internal/logstore/store.go` 定义
   ```go
   type Chunk struct { Seq int; Direction string; ContentB64 string; CreatedAt int64 }
   type Store interface {
       Append(terminalID string, direction string, data []byte) (seq int, err error)
       ReadRange(terminalID string, fromSeq, limit int) ([]Chunk, bool, error)
       Tail(terminalID string, limit int) ([]Chunk, error)
   }
   ```
2. ✍️ 聚合器 `aggregator`：每个终端一个，内部 channel 收字节，按「16ms 定时器」或「累计 16KB」先到先 flush 成一个 chunk
3. ✍️ seq 分配：内存 `atomic.Int64`（每终端一个，启动时从 DB `MAX(seq)` 恢复）
4. ✍️ base64 编码后调 `log_repo.AppendChunk`
5. 📁 冷归档：`internal/logstore/archive.go` 定时把超过 `LogRetention` 天的行追加到 `~/.shellsync/logs/<terminal_id>.log` 并删除 DB 行
6. 🧪 单测：模拟高频小包，验证聚合后 chunk 数合理、seq 连续不重号

✅ **完成标志**：灌入 1MB 输出，DB 行数远小于字节数，seq 严格递增。

---

## M1-8 terminal.Manager：会话生命周期 + 读取协程

**目标**：终端创建/列表/关闭/restart，崩溃标记。**预估**：1.5d **依赖**：M1-6, M1-7

**步骤**：
1. 📁 `internal/terminal/session.go`：
   ```go
   type Session struct {
       ID string; PTY pty.PTY; LogStore *Aggregator
       cancel context.CancelFunc; wg sync.WaitGroup
       status string; exitCode int; mu sync.RWMutex
       outputHooks []func(seq int, dir, b64 string) // ws 订阅回调
   }
   ```
2. ✍️ 启动读取协程：`go s.readLoop()` 循环 `PTY.Read` → `LogStore.Append` → 触发 outputHooks（广播用，M2 接）
3. ✍️ `Write(data)`/`Resize(cols,rows)` 透传 PTY；`input` 也经 logstore 记录（direction=stdin）
4. 📁 `internal/terminal/manager.go`：`Manager` 用 `sync.Map` 管理活跃 Session
   ```go
   Create(opts) (*Session, error)   // spawn PTY + 建 terminal 行 + 启动读循环
   Get(id) (*Session, bool)
   List(filter) []TerminalDTO
   Close(id, keepLogs) error
   Restart(id) error
   ```
5. ✍️ 进程退出监听：`Wait()` 拿 exitCode → 更新 DB status=exited/crashed → 触发 status 钩子
6. ✍️ **启动恢复**：daemon 启动时把 DB 中 `status=running` 的全部改 `crashed`（进程已不在），日志保留
7. 🧪 手测：创建终端→执行命令→kill 子进程→DB 状态正确；重启 daemon→历史可查

✅ **完成标志**：单终端闭环可用，关闭 daemon 不丢元数据与日志。

---

# M2. Daemon 接口层

## M2-1 service 领域层（状态机 + 级联）

**目标**：业务校验集中在此层。**预估**：1.5d **依赖**：M1-5

**步骤**：
1. 📁 `internal/service/{task,todo,terminal}_service.go`
2. ✍️ TaskService 实现 `transition(id, action)`：合法流转表（见设计 §6.3）
   ```
   pending→running(start)  running→paused(pause)  paused→running(resume)
   *→done(complete)  done→running(reopen)
   ```
   非法返回 `ErrInvalidTransition`
3. ✍️ 创建终端时校验 task_id 存在；删除任务时其终端 task_id 置空（依赖 DB `ON DELETE SET NULL`，service 层做日志）
4. ✍️ TodoService 关联校验 + 排序维护（`sort_order` 重排）
5. ✍️ 所有 service 方法接收 `ctx` + `userID`，写后返回完整 DTO（含 updated_at）
6. 🧪 单测：状态机全分支 + 非法分支报错

✅ **完成标志**：业务规则在 service 层闭环，handler 只做 IO 转换。

---

## M2-2 HTTP/REST 全套端点

**目标**：设计 §6 全部端点可用。**预估**：2d **依赖**：M2-1, M1-8

**步骤**：
1. 📦 `go get github.com/go-chi/chi/v5 github.com/go-chi/cors`
2. 📁 `internal/transport/http/router.go`：装配 chi router + CORS + recover + requestLog + auth 中间件
3. 📁 按资源拆 handler：`task_handler.go / terminal_handler.go / todo_handler.go / log_handler.go / settings_handler.go / pair_handler.go / system_handler.go`
4. ✍️ 统一响应封装 `respond(w, data, err)` → `{code, data, message}`；错误码常量集中定义（设计 §6.8）
5. ✍️ 逐个实现端点（按设计 §6.2/6.4/6.5/6.6/6.7 表），DTO struct + `json tag` 对齐设计
6. ✍️ `/health` 不需鉴权（lock 探活用）
7. ✍️ 参数校验：用 `validator` 或手写，必填/枚举/范围
8. 🧪 用 `httptest` + 表驱动测试核心端点；用 curl/Postman 手测全流程

✅ **完成标志**：curl 能完成「建任务→建终端→查日志→改待办」全链路。

---

## M2-3 auth：本地 token + 会话 token

**目标**：双链路鉴权。**预估**：1d **依赖**：M1-5

**步骤**：
1. 📁 `internal/auth/auth.go`：
   - `IssueLocalToken()` → 写 lock 文件，Desktop 用
   - `IssueSessionToken(deviceID)` → 存 devices 表（bcrypt 散列），Mobile 用
2. ✍️ `Verify(token)` → 先查本地 token，再查 devices.session_token
3. 📁 `transport/http/middleware.go`：`AuthMiddleware` 从 `Authorization: Bearer` 取 token 校验，失败 40001；`/health` `/pair/*` 放行
4. ✍️ WS 连接也走同一 token（query 参数 `?token=`）
5. 🧪 单测：合法/非法/过期 token 各分支

✅ **完成标志**：无 token 请求被拦，配对端点除外。

---

## M2-4 配对流程（二维码）

**目标**：手机扫码完成配对。**预估**：1d **依赖**：M2-3

**步骤**：
1. 📦 `go get github.com/skip2/go-qrcode`
2. 📁 `internal/service/pair_service.go`：
   - `Init()`：生成 6 位配对码 + 随机 nonce，存内存（TTL 120s），探测本机 LAN IP（遍历网卡取非 loopback IPv4）
   - `qrPayload` = `shellsync://pair?ip=<lan>&port=<port>&code=<code>`
3. ✍️ `Verify(code, deviceName, platform)`：校验码存在且未过期 → 建 device 行 → 发 session_token → 作废配对码
4. ✍️ `/api/pair/init` 返回 `{pairingCode, qrPayload(base64 PNG), expiresAt}`（PNG 直接 base64 给前端显示）
5. ✍️ 同一配对码只能用一次；过期清理
6. 🧪 手测：init → 拿二维码 → verify → 拿 token → 用 token 访问 `/api/tasks` 成功

✅ **完成标志**：配对闭环可用，token 可访问受保护资源。

---

## M2-5 WebSocket Server

**目标**：终端实时流 + 历史回放。**预估**：2.5d **依赖**：M1-8, M2-2

**步骤**：
1. 📦 `go get github.com/coder/websocket`
2. 📁 `internal/transport/ws/hub.go`：`Hub` 管理所有 `*Conn`，按连接维护订阅集合 `map[terminalID]bool`
3. 📁 `internal/transport/ws/conn.go`：单连接读写循环，解析统一信封 `{type,id,payload}`
4. ✍️ 事件分发：
   - `terminal.subscribe` → 注册订阅 → 先推 `terminal.history`（tail N 条，分页）→ 之后转发该 session 的 output
   - `terminal.input` → base64 解码 → `session.Write`
   - `terminal.resize` → `session.Resize`
   - `terminal.history.fetch` → `logstore.ReadRange` → 推 `terminal.history`
   - `terminal.unsubscribe` → 取消订阅
5. ✍️ Session output 广播：`terminal.Manager` 在创建 session 时注册 hook → hook 投递到 hub → hub 分发给所有订阅该 terminal 的 conn
6. ✍️ 心跳：每 25s 发 ping，60s 无响应关闭
7. ✍️ 统一错误响应：`{type, ref, ok:false, error:{code,message}}`
8. 🧪 用 `wscat` 或脚本双连：一端订阅，另一端 POST input，验证 output 推送

✅ **完成标志**：双客户端订阅同一终端，实时同步输出。

---

## M2-6 sync 事件总线

**目标**：REST 写入 → WS 广播。**预估**：1d **依赖**：M2-5

**步骤**：
1. 📁 `internal/sync/bus.go`：`Bus` 用 channel + 订阅者列表，`Publish(Event)` 分发
   ```go
   type Event struct { Type string; Payload any } // task.created 等
   ```
2. ✍️ 在各 service 写操作末尾 `bus.Publish(...)`（创建/更新/删除三类事件）
3. ✍️ hub 订阅 bus，把事件转成 WS 帧广播给所有连接（终端事件除外，那是流）
4. ✍️ 客户端可幂等处理（按 id + updatedAt 去重）
5. 🧪 手测：Desktop 端建任务，Postman 模拟另一连接收到 `task.created`

✅ **完成标志**：任意写操作触发全量连接广播。

---

# M3. Desktop 客户端

## M3-1 Electron 工程 + 守护 Daemon

**目标**：App 启动拉起 daemon。**预估**：1.5d **依赖**：M1-3

**步骤**：
1. 🔧 用 `electron-vite` 脚手架或手动 `npm create vite` + 接 electron
   ```bash
   mkdir desktop && cd desktop && npm init -y
   npm i -D electron electron-vite vite
   npm i vue pinia vue-router
   ```
2. 📁 `electron/main/index.ts`：
   - `app.whenReady()` → 读 `~/.shellsync/daemon.lock` → ping `/health`
   - 不存活 → `child_process.spawn(daemonPath, [], {detached:true, stdio:'ignore'}).unref()`
   - 轮询等待 lock 就绪（最多 5s）
3. 📁 daemon 二进制路径：dev 用 `path.join(__dirname, '../../daemon/bin')`，打包时打到 `resources`
4. 📁 托盘 Tray：最小化到托盘，右键退出（退出时**不**关 daemon）
5. 📁 `preload/index.ts`：contextBridge 暴露 `window.api`（端口/token 读取）
6. 🧪 手测：启动 App → daemon 起来；关 App → daemon 仍在（任务管理器确认）

✅ **完成标志**：App 与 daemon 生命周期解耦。

---

## M3-2 Vue3 脚手架 + 四页骨架

**目标**：导航与空页面就绪。**预估**：1d **依赖**：M3-1

**步骤**：
1. 📁 `src/{api,stores,views/{Task,Terminal,Todo,Settings},components,router,assets}`
2. 📁 `router/index.ts`：四条路由 + 侧边栏 layout
3. 📦 UI 库选型：轻量用 `naive-ui` 或 `element-plus`（按喜好）
4. 📁 `App.vue` + 侧边栏导航（任务/终端/待办/设置）
5. 📁 四个 view 占位 + Pinia store 空壳（`taskStore/terminalStore/todoStore`）
6. ✍️ 全局错误/加载态组件
7. 🧪 `npm run dev` 四页可切换

✅ **完成标志**：UI 框架跑起来。

---

## M3-3 API 层（REST + WS 封装）

**目标**：统一调用入口。**预估**：1.5d **依赖**：M2-2, M2-5

**步骤**：
1. 📁 `src/api/client.ts`：axios 实例，baseURL 从 preload 读端口，拦截器注入 `Authorization`
2. 📁 `src/api/{tasks,terminals,todos,settings,pair}.ts`：按设计 §6 封装每个资源方法，返回 `Promise<DTO>`
3. 📁 `src/api/ws.ts`：WebSocket 封装
   - 自动重连（指数退避）
   - 请求-响应配对（用 `id`/`ref`）
   - 事件订阅 API：`on('terminal.output', cb)`
4. ✍️ 响应解包：`{code,data}` → data，code≠0 抛业务错误
5. ✍️ base64 工具：`src/utils/bytes.ts`（encode/decode for terminal input/output）
6. 🧪 单测/手测：登录后能拉到任务列表

✅ **完成标志**：API 层稳定，store 可直接调。

---

## M3-4 任务管理页

**目标**：任务全功能 CRUD + 状态流转。**预估**：2d **依赖**：M3-3

**步骤**：
1. 📁 `views/Task/TaskList.vue`：列表 + 筛选（状态/归档）+ 新建弹窗
2. 📁 `views/Task/TaskDetail.vue`：基本信息编辑 + 关联终端列表 + 关联待办列表
3. ✍️ 状态流转按钮组（按状态机动态显示可用 action）
4. ✍️ Pinia `taskStore`：list/detail/create/update/delete + WS 事件监听增量更新
5. ✍️ 订阅 WS `task.created/updated/deleted` → 本地 store 同步
6. ✍️ 删除二次确认（提示终端会解绑）
7. 🧪 手测：建/改/删/状态切换，多端实时同步

✅ **完成标志**：任务管理闭环。

---

## M3-5 待办管理页

**目标**：待办 CRUD + 排序。**预估**：1.5d **依赖**：M3-3

**步骤**：
1. 📁 `views/Todo/TodoList.vue`：分组（按任务/按状态）+ 勾选完成
2. ✍️ 新建/编辑表单：标题、备注、关联任务、关联终端、优先级
3. ✍️ 拖拽排序：用 `vuedraggable`，变更后批量 PATCH `sortOrder`
4. ✍️ `todoStore` + WS 同步
5. 🧪 手测：增删改 + 拖拽 + 同步

✅ **完成标志**：待办可用。

---

## M3-6 XTerminal.vue（xterm.js 封装）

**目标**：终端体验等同原生。**预估**：2.5d **依赖**：M3-3

**步骤**：
1. 📦 `npm i @xterm/xterm @xterm/addon-fit @xterm/addon-search @xterm/addon-web-links`
2. 📁 `components/XTerminal.vue`：props `{terminalId}`，生命周期挂载/卸载
3. ✍️ 挂载流程：
   - `GET /logs/tail?limit=2000` → 解码 base64 → `term.write()` 回填历史
   - `ws.send('terminal.subscribe')` → 监听 `terminal.output` → 解码 → `term.write()`
4. ✍️ 输入：`term.onData(str)` → base64 编码 → `ws.send('terminal.input')`
5. ✍️ 尺寸：`addon-fit` + ResizeObserver → `terminal.resize`
6. ✍️ 主题/字体/滚动条样式；链接可点（web-links addon）
7. ✍️ 卸载时 `terminal.unsubscribe`，避免泄漏
8. 🧪 **关键验收**：真实跑 Claude Code / Pi Agent，验证颜色、进度条、交互式确认正常

✅ **完成标志**：AI 工具完整可用，输出无乱码。

---

## M3-7 终端管理页

**目标**：多终端隔离、shell 切换。**预估**：1.5d **依赖**：M3-6

**步骤**：
1. 📁 `views/Terminal/TerminalList.vue`：终端卡片（名称/shell/状态/最后活跃）
2. ✍️ 新建终端弹窗：选 shell（GET `/api/shells`）、工作目录、归属任务、尺寸
3. 📁 `views/Terminal/TerminalTabs.vue`：多标签页，每页一个 XTerminal
4. ✍️ 操作：改名、改归属、restart、关闭（默认保留日志）
5. ✍️ 状态实时刷新（WS `terminal.status`）
6. 🧪 手测：开 3 个不同 shell 并行运行，互不干扰

✅ **完成标志**：多终端并行可用。

---

## M3-8 设置页

**目标**：配对二维码 + 配置。**预估**：1d **依赖**：M3-3, M2-4

**步骤**：
1. 📁 `views/Settings/Settings.vue`：分区（通用/同步/终端/关于）
2. ✍️ 配对区：点"生成二维码" → `POST /pair/init` → 显示 PNG + 配对码 + 倒计时
3. ✍️ 设备列表：`GET /api/devices` + 吊销按钮
4. ✍️ 默认 shell 选择、日志保留天数、主题、开机自启（写 settings）
5. 🧪 手测：二维码能用手机扫（待 M4 联调）

✅ **完成标志**：设置项可读写。

---

# M4. Mobile 客户端

## M4-1 Flutter 工程脚手架

**目标**：双端可运行空壳。**预估**：1d

**步骤**：
1. 🔧 `flutter create --org com.shellsync shellsync_mobile`（在 `mobile/`）
2. 📦 状态管理：`flutter_riverpod`；路由：`go_router`；网络：`dio` + `web_socket_channel`
3. 📁 `lib/{api,stores,pages/{pairing,task,terminal,todo},widgets,models,utils}`
4. 📁 `go_router` 路由：未配对 → pairing；已配对 → 首页（任务）
5. 🧪 `flutter run` iOS/Android 跑起来

✅ **完成标志**：App 可启动导航。

---

## M4-2 扫码配对页

**目标**：扫码完成配对。**预估**：1d **依赖**：M2-4

**步骤**：
1. 📦 `mobile_scanner`（扫码）
2. 📁 `pages/pairing/pairing_page.dart`：摄像头扫码 → 解析 `shellsync://pair?...`
3. ✍️ `POST /pair/verify` → 拿 sessionToken + endpoint
4. ✍️ 用 `flutter_secure_storage` 安全持久化 token + endpoint
5. ✍️ 成功后跳首页
6. 🧪 手测：扫 Desktop 二维码完成配对

✅ **完成标志**：配对 token 落地存储。

---

## M4-3 API 客户端（REST + WS + 重连）

**目标**：稳定通信层。**预估**：1.5d **依赖**：M4-2

**步骤**：
1. 📁 `lib/api/rest_client.dart`：dio + 拦截器注入 token
2. 📁 `lib/api/ws_client.dart`：web_socket_channel 封装
   - 自动重连（指数退避，封顶 30s）
   - 请求-响应 id 配对
   - 事件流 `Stream<WsEvent>`
3. 📁 `lib/api/services/{tasks,terminals,todos}.dart`
4. ✍️ 断线重连后：REST `?since=lastSyncTs` 拉增量；终端流订阅带 `fromSeq`
5. 🧪 手测：开关飞行模式验证重连与补齐

✅ **完成标志**：弱网下数据不丢。

---

## M4-4 任务/待办列表页

**目标**：同步展示 + 状态更新。**预估**：2d **依赖**：M4-3

**步骤**：
1. 📁 `pages/task/task_list_page.dart`：列表 + 状态筛选 + 下拉刷新
2. 📁 `pages/task/task_detail_page.dart`：详情 + 关联终端/待办 + 状态按钮
3. 📁 `pages/todo/todo_list_page.dart`：勾选完成 + 关联
4. ✍️ riverpod provider 监听 WS 事件增量更新
5. ✍️ 基础编辑：状态切换、待办勾选（创建/编辑后续迭代）
6. 🧪 手测：Desktop 改任务，手机实时变

✅ **完成标志**：只读为主 + 状态可改，满足"基础状态更新"需求。

---

## M4-5 终端列表 + 远程终端视图

**目标**：手机看历史 + 实时输出 + 输入干预。**预估**：3d **依赖**：M4-3

**步骤**：
1. 📁 `pages/terminal/terminal_list_page.dart`：终端卡片（名称/shell/状态）
2. 📁 `pages/terminal/terminal_view_page.dart`：进入即
   - `GET /logs/tail` 回填历史
   - `terminal.subscribe` 订阅实时
3. ✍️ ANSI 渲染：用 `xterm.dart` 或自研轻量 ANSI parser + monospace 文本组件
4. ✍️ 输入栏：底部文本框 + 发送（base64 编码 `terminal.input`）+ 常用键（Ctrl+C、Tab、↑↓）
5. ✍️ 滚动加载更早历史（`terminal.history.fetch`）
6. ✍**关键验收**：手机实时看 Claude Code 思考/输出，能输入指令
7. 🧪 手测：AI 跑任务时手机监控并干预

✅ **完成标志**：远程操控闭环。

---

## M4-6 设备管理 + 退出登录

**目标**：账号/设备管理。**预估**：0.5d **依赖**：M4-2

**步骤**：
1. 📁 `pages/settings/settings_page.dart`：已配对设备列表 + 吊销
2. ✍️ 退出登录：清 secure_storage → 回配对页
3. 🧪 手测：吊销后该 token 失效

✅ **完成标志**：设备可吊销。

---

# M5. 联调、稳定性与发布

## M5-1 终端长时压力测试

**目标**：8h+ 不卡死、不丢日志。**预估**：1.5d **依赖**：M3, M4

**步骤**：
1. ✍️ 写脚本：终端内 `yes` / `find /` / 编译大项目持续输出
2. ✍️ 定时采样：daemon 内存/RSS、DB 行数、日志文件大小、WS 延迟
3. ✍️ 验证：kill -9 子进程 → 状态 crashed、日志完整
4. ✍️ 调优：chunk 窗口、WAL checkpoint 策略、goroutine 泄漏排查
5. ✍️ 记录基线数据（内存增长曲线、吞吐）

✅ **完成标志**：8h 稳定，无崩溃无内存泄漏。

---

## M5-2 断线重连 & 增量同步验证

**预估**：1d **依赖**：M2-6

**步骤**：
1. ✍️ 模拟：拔网线/禁网卡 30s 后恢复
2. ✍ 验证：Mobile 自动重连 → `since` 拉增量 → 终端流按 seq 续传无丢无重
3. ✍️ 验证：Desktop 改任务期间 Mobile 离线 → 重连后补齐
4. ✍️ 边界：seq 跳跃（chunk 丢失）、token 过期重配对

✅ **完成标志**：弱网下数据最终一致。

---

## M5-3 三平台打包（Daemon + Desktop）

**预估**：1.5d **依赖**：M3

**步骤**：
1. 🔧 Daemon：`goreleaser` 或 Makefile 三平台交叉编译
   ```bash
   GOOS=windows GOARCH=amd64 go build -o bin/shellsync-daemon.exe
   GOOS=darwin  GOARCH=arm64 go build -o bin/shellsync-daemon
   GOOS=linux   GOARCH=amd64 go build -o bin/shellsync-daemon
   ```
2. 🔧 Desktop：`electron-builder` 配置，把 daemon 二进制打进 `resources`，三平台产物（nsis/dmg/AppImage）
3. ✍️ 启动逻辑读打包后的 daemon 路径
4. ✍️ macOS 代码签名与公证（后续）；Windows 走 ConPTY 验证
5. 🧪 三平台各装一台真机验收

✅ **完成标志**：三平台一键安装可用。

---

## M5-4 Flutter 双端打包

**预估**：1d **依赖**：M4

**步骤**：
1. 🔧 配置 `flutter build ios --release` / `flutter build apk --release`
2. ✍️ iOS：bundle id、签名、Info.plist 权限（相机/局域网）
3. ✍️ Android：网络权限、明文流量（局域网 IP）配置
4. 🧪 真机安装验收

✅ **完成标志**：双端可安装运行。

---

## M5-5 README + 使用文档 + 截图

**预估**：0.5d

**步骤**：
1. 📁 根 `README.md`：项目简介、架构图、截图、安装/使用、配对流程
2. 📁 各子工程 README（daemon/desktop/mobile 启动方式）
3. 📁 截图：四大页面 + 手机端
4. 📁 开发文档：本地起 daemon/Desktop/Mobile 的命令
5. ✍️ License + 贡献指南

✅ **完成标志**：开源可读性达标。

---

# 附录：执行检查清单（每完成一项勾选）

```
M1  [x] M1-1  [x] M1-2  [ ] M1-3  [x] M1-4  [x] M1-5  [x] M1-6  [x] M1-7  [x] M1-8  [ ] M1-6  [ ] M1-7  [ ] M1-8  [ ] M1-6  [ ] M1-7  [ ] M1-8
M2  [ ] M2-1  [ ] M2-2  [ ] M2-3  [ ] M2-4  [ ] M2-5  [ ] M2-6
M3  [ ] M3-1  [ ] M3-2  [ ] M3-3  [ ] M3-4  [ ] M3-5  [ ] M3-6  [ ] M3-7  [ ] M3-8
M4  [ ] M4-1  [ ] M4-2  [ ] M4-3  [ ] M4-4  [ ] M4-5  [ ] M4-6
M5  [ ] M5-1  [ ] M5-2  [ ] M5-3  [ ] M5-4  [ ] M5-5
```

> 建议每完成一个任务：① 勾选 ② 提交 git（`feat(m1-5): repository layer`）③ 更新《开发任务规划》看板进度。
