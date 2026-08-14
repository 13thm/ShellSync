# ShellSync Desktop

电脑端桌面客户端：**Electron + Vue 3** 单页应用。
启动时自动拉起（或复用已有的）Go daemon 子进程，通过本地 HTTP/WS（127.0.0.1 + Token）通信，提供任务/待办/终端管理界面。

> 与手机端共用同一套 daemon REST + WebSocket 协议（见 `../daemon/` 与设计说明书 §5/§6）。

## 框架与技术栈

| 层 | 技术 | 说明 |
|----|------|------|
| 构建 | `electron-vite` + Vite 5 | 三段式构建：main / preload / renderer |
| 主进程 | Electron 31 + TypeScript | 窗口、托盘、daemon 子进程管理 |
| 渲染进程 | Vue 3 + `vue-router` + `pinia` | SPA，视图/路由/状态管理 |
| 终端 | `@xterm/xterm` + fit/web-links 插件 | 浏览器内终端渲染 |
| 网络 | `axios`（REST）+ 原生 WebSocket | 与 daemon 通信 |
| 其他 | `lucide-vue-next`（图标）、`qrcode`（生成配对二维码） | |

**进程架构**：

```
Electron main（src/main）                       ── 窗口/托盘/生命周期
  ├─ ensureDaemon()：读 ~/.shellsync/daemon.lock → 已运行则复用，否则 spawn daemon 子进程
  ├─ IPC  'daemon:connect' → 把 {port, token, baseUrl, wsUrl} 经 preload 交给渲染进程
  └─ preload（contextBridge）→ 渲染进程拿到的唯一入口 window.daemon.connect()

Renderer（src/renderer，Vue SPA）
  ├─ api/    axios 客户端（自动带 token）+ WS 封装（重连/订阅）
  ├─ stores/ Pinia 状态（连接/任务/待办/终端/设置/实时事件）
  └─ views/  五个页面（终端/任务列表/任务详情/待办/设置）
```

## 文件结构（每个文件的用途）

```
desktop/
├── package.json                    # 依赖与脚本（dev / build / preview / typecheck）
├── package-lock.json               # 依赖锁定
├── electron.vite.config.ts         # electron-vite 配置（main/preload/renderer 三段构建，@ 别名指向 renderer）
├── tsconfig.json                   # TS 配置入口（引用下面两个）
├── tsconfig.node.json              # 主进程/preload 的 TS 配置（Node 环境）
├── tsconfig.web.json               # 渲染进程（Vue）的 TS 配置
├── .gitignore
└── src/
    ├── main/                       # ── Electron 主进程 ──
    │   ├── index.ts                # 应用入口：创建主窗口/托盘/菜单、IPC 注册、bootstrap 拉起 daemon
    │   └── daemon.ts               # daemon 子进程管理：定位二进制（dev / 打包路径 / SHELLSYNC_DAEMON 环境变量）、
    │                                #   读 daemon.lock 复用已有实例、spawn 并等健康检查通过、返回连接信息
    ├── preload/
    │   └── index.ts                # contextBridge：向渲染进程暴露 window.daemon.connect()（缓存连接信息）
    └── renderer/                   # ── Vue 3 渲染进程 ──
        ├── index.html              # SPA HTML 模板
        ├── main.ts                 # Vue 应用引导：装 pinia/router、挂载根组件
        ├── App.vue                 # 根组件：整体布局（侧边导航 + 路由出口 + 连接状态）
        ├── env.d.ts                # 环境类型声明（window.daemon 等）
        ├── types.ts                # 实体类型定义（Task/Todo/Terminal/Device/Shell/日志响应…）
        ├── router/
        │   └── index.ts            # 路由表：终端 / 任务列表 / 任务详情 / 待办 / 设置
        ├── api/
        │   ├── client.ts           # axios 实例工厂：注入 baseUrl + token（从 window.daemon 获取）
        │   ├── index.ts            # REST API 封装：tasksApi / todosApi / terminalsApi / devicesApi /
        │   │                       #   logsApi / pairApi / settingsApi（对应 daemon 的 /api/*）
        │   └── ws.ts               # WebSocket 封装：连接/自动重连、终端流收发、实体变更事件订阅
        ├── stores/                 # Pinia 状态管理
        │   ├── connection.ts       # daemon 连接状态（port/token/是否在线）
        │   ├── tasks.ts            # 任务列表/筛选/增删改
        │   ├── todos.ts            # 待办列表/勾选切换
        │   ├── terminals.ts        # 终端会话列表/创建/销毁
        │   ├── settings.ts         # 用户设置（配对码生成等入口）
        │   └── realtime.ts         # WS 实时事件路由：收到变更事件后分发刷新各 store
        ├── views/                  # 页面组件（路由级）
        │   ├── TerminalView.vue    # 终端页：会话列表 + 新建终端 + XTerminal 实时交互
        │   ├── TaskList.vue        # 任务列表页：状态筛选/归档、新建任务
        │   ├── TaskDetail.vue      # 任务详情页：信息编辑、关联终端、日志查看
        │   ├── TodoView.vue        # 待办页：清单勾选管理
        │   └── SettingsView.vue    # 设置页：连接信息、生成配对二维码（供手机扫码）、设备管理
        ├── components/
        │   ├── XTerminal.vue       # xterm.js 终端组件：attach 到 WS 会话、fit 自适应、输入转发、历史回放
        │   └── ui/                 # 基础 UI 组件库（对应 doc/styles 设计规范）
        │       ├── AppButton.vue   # 按钮
        │       ├── AppCard.vue     # 卡片容器
        │       ├── AppInput.vue    # 输入框
        │       ├── AppListItem.vue # 列表项
        │       ├── EmptyState.vue  # 空状态占位
        │       └── StatusDot.vue   # 状态圆点（终端运行状态等）
        ├── utils/
        │   └── status.ts           # 状态工具（任务/终端状态 → 颜色/文案映射）
        └── styles/
            ├── tokens.css          # 设计令牌（颜色/间距/圆角/字体 CSS 变量）
            └── base.css            # 全局基础样式（reset、通用排版）
```

## 启动

```bash
cd desktop
npm install        # 首次
npm run dev        # 开发模式（electron-vite dev，热更新）
npm run build      # 产出 out/ 目录
npm run typecheck  # 主进程 + 渲染进程类型检查
```

> 开发模式下会自动查找 `../daemon/bin/ssd.exe`（或 `shellsync-daemon[.exe]`）并拉起；
> 也可用环境变量 `SHELLSYNC_DAEMON=<路径>` 指定自定义位置。
> 若 daemon 已在运行（存在 `~/.shellsync/daemon.lock`），则直接复用，不会重复启动。
