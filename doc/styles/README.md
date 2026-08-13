# ShellSync 桌面端 · 设计系统包

> 配套《[设计规范·视觉与组件样式定稿](../设计规范·视觉与组件样式定稿.md)》。本目录是可直接拷进 M3 Desktop 工程的「设计系统包」。

## 目录结构

```
doc/styles/
├── tokens.css              # 设计令牌（颜色/字体/间距/圆角/阴影/动效）—— 必须最先引入
├── base.css                # 全局基础样式（reset + 页面底 + 原生滚动条）
├── README.md               # 本文件
└── components/             # 4 个原子组件（标准件）
    ├── AppButton.vue       # 三档按钮：主 / 次要 / 文字
    ├── AppCard.vue         # 信息分组卡片（边框，不加阴影）
    ├── AppListItem.vue     # 通用列表项
    └── AppInput.vue        # 标准输入框
```

## 引入方式（M3 工程初始化后）

1. 把本目录整体拷到 `desktop/src/renderer/styles/` 与 `desktop/src/renderer/components/ui/`。
2. 在渲染进程入口 `main.ts` 按顺序引入：

```ts
import './styles/tokens.css' // ① 必须最先：定义所有 var(--xxx)
import './styles/base.css'   // ② 全局基础
import { createApp } from 'vue'
import App from './App.vue'
createApp(App).mount('#app')
```

3. 图标统一用 `lucide-vue-next`（线性、1.5–2px 描边、`currentColor`）。

## 使用铁律

- **禁止内联写死**颜色/圆角/字号——一律用 `var(--xxx)`。
- 全局**只有一个主色** `--color-primary`（柔和绿），面积 ≤5%，不做渐变/大块背景。
- **静态元素用边框**（`--color-border-base`），只有浮层（菜单/弹窗/Toast/模态）才用阴影。
- 圆角统一 6–10px，**禁止 16px+ 大圆角**。
- 文字非纯黑（用 `--color-text-primary` `#1F2329`），底色非纯黑。
- 危险操作用**红色文字按钮**，不用红色实心。
- **严禁用 emoji 当图标**。

## 自检清单（出图对照，全 ✅ 才合格）

- [ ] 没有渐变（背景/文字/边框）
- [ ] 没有玻璃模糊、没有发光光晕
- [ ] 没有 emoji 当图标
- [ ] 圆角在 6–10px，没有超大圆角
- [ ] 静态元素用边框而非阴影
- [ ] 全局只出现一个主色，且面积小
- [ ] 文字非纯黑、底色非纯黑
- [ ] 字号主要 13–16px
- [ ] 动画快而轻，没有弹跳瀑布
- [ ] 整体像系统设置/微信设置，不像 AI 模板

## ShellSync 四大模块映射

ShellSync Desktop 的四个模块全部用本设计系统搭建：

| 模块 | 主要组件 | 主色用法 |
|------|----------|----------|
| **任务管理** | `AppListItem`（任务行）+ 状态点 + `AppButton`（新建） | 状态点用功能色；选中态 `--color-bg-selected` |
| **终端管理** | 多标签 + xterm.js（等宽 `--font-family-mono`）+ `AppCard` | 运行/退出/崩溃状态点（绿/灰/红） |
| **待办管理** | `AppListItem`（勾选 + 关联标签）+ `AppButton` | 勾选用主色 |
| **设置 / 同步** | 左导航 + 表单（`AppInput` + Switch）+ 配对二维码卡片 | 配对码用主色强调 |

### ShellSync 专属补充令牌（已含在 tokens.css）

规范面向通用桌面工具，ShellSync 作为**终端任务管理**工具，额外约定：

- **任务状态点**：`--color-status-running`(绿) / `--color-status-paused`(橙) / `--color-status-done`(灰) / `--color-status-error`(红)。
  - 注意「已完成」用**灰**而非绿——已结束的事不应抢眼，符合「克制」。
- **终端/代码**一律 `--font-family-mono` + 浅灰底 `--color-bg-page`。
- **日志区**用分隔线分行，不用卡片堆叠（列表流为主）。
