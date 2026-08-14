# ShellSync Mobile (Flutter)

手机端：扫码配对电脑端 ShellSync daemon，远程查看任务/待办/终端，并可输入指令干预。

> 与桌面端共用同一套 daemon REST + WebSocket 协议（见 `../daemon/` 与设计说明书 §5/§6）。

## 框架与技术栈

| 依赖 | 用途 |
|------|------|
| `dio` | REST 客户端（自动解包 `{code,data,message}` 信封） |
| `web_socket_channel` | WebSocket（自动重连 + 请求-响应帧 + 事件订阅） |
| `provider` + `ChangeNotifier` | 状态管理（单 `AppState`） |
| `xterm` 4.x | Flutter 终端渲染引擎（浅色主题，贴合设计系统） |
| `mobile_scanner` | 摄像头扫码（配对二维码） |
| `flutter_secure_storage` | 安全存储配对凭证（endpoint + token） |
| `intl` | 本地化/格式化 |

**架构**：

```
main.dart → app.dart（MaterialApp + provider 注入 AppState）
  ├─ AppState（唯一 ChangeNotifier）
  │    ├─ 连接态（paired / wsConnected / endpoint / token）
  │    ├─ 三个实体列表（tasks / todos / terminals）
  │    └─ WS 事件路由 → 收到实体变更事件后刷新对应列表
  ├─ api/   RestClient(dio) + ApiService(各实体端点) + WsClient
  ├─ pages/ 未配对 → PairingPage；已配对 → HomePage（底部导航）
  └─ widgets/ 终端视图、状态组件
```

## 文件结构（每个文件的用途）

```
mobile/
├── pubspec.yaml                    # Flutter 项目定义与依赖
├── pubspec.lock                    # 依赖锁定
├── analysis_options.yaml           # Dart 静态分析规则（flutter_lints）
├── .flutter-plugins-dependencies   # 自动生成的插件依赖清单
├── android/ ios/                   # 平台目录（需用 SDK 生成，见下文「平台脚手架」）
└── lib/
    ├── main.dart                   # 入口：初始化 → runApp
    ├── app.dart                    # MaterialApp 根组件：路由（配对页/主页）、provider 注入
    ├── models.dart                 # 实体模型（Task/Todo/Terminal/Device/配对结果…，含 fromJson）
    ├── api/
    │   ├── rest_client.dart        # dio 封装：baseUrl + token 头、响应信封自动解包与错误处理
    │   ├── services.dart           # ApiService：tasks/todos/terminals/devices/logs/pair 各端点方法
    │   └── ws_client.dart          # WebSocket 客户端：连接/自动重连、请求-响应帧、实体变更事件流
    ├── config/
    │   └── storage.dart            # SecureStore：flutter_secure_storage 封装（保存/读取/清除配对凭证）
    ├── stores/
    │   └── app_state.dart          # AppState（ChangeNotifier）：初始化、配对、登出、实体列表、WS 事件路由
    ├── pages/
    │   ├── pairing_page.dart       # 扫码配对页：mobile_scanner 扫二维码 + 手动输入兜底
    │   ├── home_page.dart          # 主页：底部导航（任务/待办/终端/设置）
    │   ├── tasks_page.dart         # 任务列表页：状态筛选、跳转详情
    │   ├── todos_page.dart         # 待办页：清单勾选管理
    │   ├── terminals_page.dart     # 终端会话列表页：查看在线终端、点击进入
    │   ├── terminal_session_page.dart # 终端会话页：xterm 渲染 + 输入/resize 经 WS 转发 + 历史回放
    │   └── settings_page.dart      # 设置页：连接状态、设备管理、退出登录（清除凭证）
    └── widgets/
        ├── terminal_view.dart      # xterm 终端组件（浅色主题、attach 到 WS 会话）
        └── status.dart             # 状态组件（状态点/徽标，映射终端运行状态）
```

## 当前状态（M4 ✅）

`lib/` 下 Dart 业务代码全部完成（6 个子任务）：

| 模块 | 文件 |
|------|------|
| M4-1 脚手架 | `pubspec.yaml`, `main.dart`, `app.dart` |
| M4-2 扫码配对 | `pages/pairing_page.dart`（mobile_scanner + 手动兜底） |
| M4-3 API 层 | `api/{rest_client,ws_client,services}.dart`, `config/storage.dart` |
| M4-4 任务/待办 | `pages/{home,tasks,todos}_page.dart`, `stores/app_state.dart` |
| M4-5 远程终端 | `pages/terminals_page.dart`, `pages/terminal_session_page.dart`, `widgets/terminal_view.dart`（xterm.dart） |
| M4-6 设备/登出 | `pages/settings_page.dart` |

## ✅ 已验证

本机已安装 Flutter 3.44.x（Dart 3.12），运行 `flutter pub get` + `flutter analyze` 通过：

```
Analyzing mobile...
No issues found! (ran in 5.5s)
```

依赖 70 个全部解析成功（xterm 4.0.0 / mobile_scanner 5.2.3 / dio / provider / web_socket_channel ...）。

> 仍未做真机/模拟器 GUI 验证（需 Android 设备），但代码静态检查全过。

## 平台脚手架（首次必做）

`lib/` 之外的平台目录（android/ios）需用 SDK 生成（不能手写，避免与 SDK 模板冲突）：

```bash
cd mobile
flutter create . --org com.shellsync --project-name shellsync_mobile --platforms=android,ios
```

然后改两处权限：

### Android — `android/app/src/main/AndroidManifest.xml`

在 `<manifest>` 内加：

```xml
<uses-permission android:name="android.permission.INTERNET"/>
<uses-permission android:name="android.permission.CAMERA"/>
```

并在 `<application>` 标签上加（局域网走明文 http）：

```xml
android:usesCleartextTraffic="true"
```

### iOS — `ios/Runner/Info.plist`

```xml
<key>NSCameraUsageDescription</key>
<string>用于扫描电脑端的配对二维码</string>
<key>NSLocalNetworkUsageDescription</key>
<string>用于连接同局域网的 ShellSync 电脑端</string>
```

## 配对流程

1. 电脑端 Desktop「设置 → 生成配对码」得到二维码（`shellsync://pair?ip=&port=&code=`）。
2. 手机端打开 App → 扫码（或手动输入）。
3. App 调 `POST /api/pair/verify` 换 sessionToken，存入安全存储，进入主页。
4. 之后启动 App 自动用已存凭证连接；「退出登录」清除凭证。
