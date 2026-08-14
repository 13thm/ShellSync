# ShellSync 启动指南

> 前置：三端在同一局域网（手机连电脑的 WiFi/热点）时才能配对使用。
> 开发调试顺序建议：**先 daemon → 再 desktop → 最后 mobile**。

---

## 0. 环境要求

| 项目 | 要求 |
|------|------|
| Go | ≥ 1.25（daemon） |
| Node.js | ≥ 18（desktop，需要 npm） |
| Flutter | ≥ 3.3 / Dart ≥ 3.3（mobile） |
| 平台 | Windows（daemon 已编译 ConPTY 版本）；macOS/Linux 亦可编译（PTY 用 creack/pty） |

---

## 1. daemon（Go 守护进程）—— 系统核心，最先启动

```bash
cd daemon

# 方式 A：直接运行（开发）
go run ./cmd/shellsync-daemon

# 方式 B：编译后运行（Desktop 也会自动找这个位置拉起它）
go build -o bin/ssd ./cmd/shellsync-daemon        # Windows 下即 bin/ssd.exe
./bin/ssd
```

- 按 `Ctrl+C` 优雅退出。
- 启动后会写锁文件 `~/.shellsync/daemon.lock`（含 pid/port/token），保证单实例，并供 Desktop 复用连接。
- 快速自检：`curl http://127.0.0.1:<port>/healthz`（端口见启动日志/配置）。
- 仓库里已有编译好的 `daemon/bin/ssd.exe`（Windows），**只跑 Desktop 的话本步可跳过**，Desktop 会自动拉起。

---

## 2. desktop（Electron 桌面端）

```bash
cd desktop
npm install        # 首次
npm run dev        # 开发模式（热更新）
```

启动逻辑（无需手动开 daemon）：

1. 主进程先读 `~/.shellsync/daemon.lock`，若 daemon 已在运行 → 直接复用；
2. 否则自动查找并拉起 daemon 子进程，查找顺序：
   - 环境变量 `SHELLSYNC_DAEMON=<可执行文件路径>`（可选，自定义位置时用）
   - `../daemon/bin/ssd.exe` / `shellsync-daemon.exe`（开发布局）
   - `resources/daemon/`（打包布局）
3. 连接信息（port/token）经 preload 的 `window.daemon.connect()` 注入渲染进程。

其他命令：

```bash
npm run build      # 产出 out/（main + preload + renderer）
npm run preview    # 预览构建产物
npm run typecheck  # 类型检查（主进程 + 渲染进程）
```

---

## 3. mobile（Flutter 手机端）

### 3.1 环境准备（首次）

```powershell
# Windows：一键配置 Flutter PATH + 国内镜像（根目录脚本）
powershell -ExecutionPolicy Bypass -File ./setup-flutter-env.ps1
# 重开终端后验证
flutter doctor
```

### 3.2 生成平台目录（首次必做，android/ 目录不入库、不能手写）

```bash
cd mobile
flutter create . --org com.shellsync --project-name shellsync_mobile --platforms=android,ios
```

然后加权限（详见 `mobile/README.md`）：

- `android/app/src/main/AndroidManifest.xml`：加 `INTERNET`、`CAMERA` 权限，`<application>` 加 `android:usesCleartextTraffic="true"`；
- `ios/Runner/Info.plist`：加相机与本地网络用途描述。

### 3.3 安装依赖并运行

```bash
flutter pub get
flutter analyze          # 应输出 No issues found
flutter devices          # 确认手机/模拟器已连接
flutter run              # 连接的设备上启动（-d <deviceId> 指定设备）
```

### 3.4 与电脑端配对

1. 电脑端启动 Desktop → 「设置 → 生成配对码」显示二维码（`shellsync://pair?ip=&port=&code=`）；
2. 手机 App 打开 → 扫码（或手动输入）；
3. App 自动调 `POST /api/pair/verify` 换取 token 并存入安全存储，进入主页；
4. 之后每次启动 App 自动用已存凭证连接，无需重复扫码。

---

## 4. 常见问题

| 问题 | 处理 |
|------|------|
| Desktop 提示找不到 daemon | 先按 §1 编译出 `daemon/bin/ssd(.exe)`，或设 `SHELLSYNC_DAEMON` 环境变量 |
| 手机连不上电脑 | 确认同一局域网、电脑防火墙放行 daemon 端口、Android 已允许明文流量 |
| 想彻底重置 daemon | 删除 `~/.shellsync/`（lock + 数据库）后重启 |
| 端口被占用 | 见 daemon 启动日志中的端口配置说明（config 包支持环境变量覆盖） |
