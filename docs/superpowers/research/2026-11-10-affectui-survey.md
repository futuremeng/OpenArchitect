# 调研（2026-11-10）：affectUI——agent UI 的词汇表，不是依赖

- 素材：<https://github.com/flohoeller/affectui>（661★，MIT，119 commits，活跃）。
  React/shadcn 单文件组件库，专做「agent 在做什么」的 UI：runs、tool calls、
  traces、handoffs、approvals、context。
- 用户提示「这个资源可以用」——本文把"怎么用"定死，避免误当依赖。

## 1. 硬结论：不进依赖树

OpenArchitect 是 Go + mygo 立即模式渲染；affectUI 是 React/TSX + shadcn
registry。没有共同的运行地，抄代码也不划算（样式内嵌在 TSX 里）。
**它的正确角色是设计词汇表 + 信息架构参照**，与前两张参考图同类，
但成体系、有名字、有生态。

## 2. 可搬的东西（按价值排）

| affectUI | 语义 | 我们的对应/落点 |
|---|---|---|
| 命名体系：runs / tool calls / traces / handoffs / approvals / context | agent 活动观察的标准词汇 | 1.x「agent 活动」视图的英文术语照抄生态，与 paperclip/Orca 的话术对齐 |
| `change-summary` | 一次变更的摘要卡（谁、动了什么、diff 摘要） | 详情面板果实行的升级目标：从"一行 commit"到一张摘要卡 |
| `tool-call-card` | 一次工具调用的可折叠卡 | 未来「worktree 里 agent 正在动哪个仓」的展示单元 |
| `thinking-loader` / shimmer | 「正在思考/进行中」态 | 扫描与 sync 的 progress 行（现在只是文本） |
| `decision-inbox` | 审批队列 | **边界警示**：审批是 paperclip 的地盘——I2′ 下我们只观察不审批，这个组件提醒我们别把功能越界做过去 |

Pro 组件（Agent Tree、Trace Waterfall、Agent Handoff、Context Map）源码
不在仓库——只作为信息架构参考，不承诺复刻。

## 3. 一个真实的未来入口

若哪天做**只读 Web 观察视图**（把架构图/果实暴露成页面），affectUI 的
shadcn registry + MIT 是现成的组件层——那是它唯一可能变成真依赖的路径。
已挂进定位文档 backlog。

## 4. 决定

- ✅ 采纳词汇与三个组件的**设计语义**（change-summary → 果实行升级、
  tool-call-card → agent 活动卡、loader 态 → sync 进度）
- ❌ 不引入依赖、不抄代码
- ⏸ Web 导出视图（若立项）以 affectUI 为组件层候选