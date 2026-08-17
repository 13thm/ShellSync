# doc — 文档目录

ShellSync 的全部设计与规划文档，以及设计规范（视觉/组件）。

## 文件结构（每个文件的用途）

```
doc/
├── 双端持久化终端任务管理系统 （ShellSync）— 完整需求分析说明书.md
│       # 需求源头：产品背景、用户故事、功能需求、非功能需求
├── ShellSync 系统设计说明书.md
│       # 技术蓝图：总体架构（Daemon/Desktop/Mobile 三方拓扑）、模块划分、
│       # 数据库设计、REST/WS 接口定义、协议约定（时间戳/base64/UUID 等），
│       # 是三个子项目编码时共同遵守的指导文档
├── ShellSync 开发任务规划.md
│       # 里程碑划分（M1 daemon 基础 / M2 daemon 完整功能 / M3 Desktop / M4 Mobile）
│       # 及各阶段的任务拆分与优先级
├── ShellSync 开发任务详细步骤.md
│       # 按里程碑逐任务的详细实施步骤（M1-1、M1-2 … 编号对应各子项目 README 中的勾选项）
├── 跨网络配对方案.md
│       # 跨网络（任意网络）一键式配对与连接方案：官方云中继（relay-server）架构、
│       # L4 透明隧道、PAKE 配对与端到端加密设计、分期实施计划（R1~R4）
├── 跨网络配对方案的修改计划.md
│       # 实施阶段的配置驱动策略：一套代码跑所有环境（dev/staging/prod 只改配置）、
│       # P0~P4 阶段计划、测试检查清单、回滚预案、工时估算
├── 开发步骤程序.md
│       # R1（云中继 MVP）的逐任务可执行步骤（R1-1 ~ R1-11）：依赖顺序、
│       # 文件级操作清单、自测方法、生产部署物与上线切换步骤
├── 设计规范·视觉与组件样式定稿.md
│       # UI 定稿：颜色、字体、间距、组件形态的设计决策
└── styles/                       # 设计规范的可执行落地（CSS + Vue 参考实现）
    ├── README.md                 # styles 目录说明
    ├── tokens.css                # 设计令牌：颜色/间距/圆角/阴影/字体的 CSS 变量
    │                             #   （desktop/src/renderer/styles/tokens.css 即源自此）
    ├── base.css                  # 全局基础样式：reset、排版、通用类
    └── components/               # 基础组件的参考实现（desktop 的 components/ui 即源自此）
        ├── AppButton.vue         # 按钮（主要/次要/危险等变体）
        ├── AppCard.vue           # 卡片容器
        ├── AppInput.vue          # 输入框
        └── AppListItem.vue       # 列表项（含操作区插槽）
```

## 阅读顺序建议

1. 需求分析说明书 → 2. 系统设计说明书 → 3. 设计规范 → 4. 开发任务规划/详细步骤（对照代码）。
