# ShellSync Mobile (Flutter)

手机端：扫码配对电脑端 ShellSync daemon，远程查看任务/待办/终端，并可输入指令干预。

> 与桌面端共用同一套 daemon REST + WebSocket 协议（见 `daemon/` 与设计说明书 §5/§6）。

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

## ⚠️ 未在本机验证

本仓库所在机器**未安装 Flutter SDK**（~1GB，未在自动化环境里安装）。
代码已按 Flutter / Dart 3 规范编写并自审，但**未跑 `flutter analyze` / `flutter build`**。
请在本机执行：

```bash
cd mobile
flutter pub get
flutter analyze        # 期望 0 error（少量 info 级 lint 可忽略）
flutter run            # 连接真机/模拟器
```

如 `flutter analyze` 报依赖 API 不匹配（最可能的是 `package:xterm` 的 `TerminalView`/`textStyle`/`TerminalTheme` 字段随版本变化），按提示微调 `lib/widgets/terminal_view.dart` 即可。

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

## 架构

- 状态：单 `AppState`（ChangeNotifier + provider）持有连接 + 三个实体列表 + WS 事件路由。
- 网络：`dio`（REST，自动解包 `{code,data,message}` 信封）+ `web_socket_channel`（WS，自动重连 + 请求响应 + 事件订阅）。
- 终端：`xterm` dart 引擎，浅色主题（贴合设计系统），输入/resize 经 WS 转发。
