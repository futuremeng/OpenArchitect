# Changelog

## 1.0.0 — 2026-11-10

多软件项目的架构观测 + 智能体工作动态观测的第一个完整版本。

### 观测主干（既有）
- Go 仓库扫描 → SQLite → 可交互架构图（flow），单击详情、影响范围分析
- 多仓合并图：仓库/包/元素三层聚合、仓库多选过滤、GIS 式导航与 LOD 缩放
- headless CLI：`oac scan / graph / impact / context`（`--json` 对外契约）

### 生长模型（本版新增）
- **自动发现**：`oac sync <root>` 遍历扫描根，git 仓库自动登记（新/跟踪/失联三态）
- **自动检入**：HEAD 与游标比对，漂移才 fetch + 重扫；快照从此绑定 commit 与分支
- **算力闸门**：只动文档/配置的 commit 记账不重扫
- **果实归因**：commit 按 author 与 `Co-authored-by` trailer 判
  human / agent:&lt;name&gt; / mixed，`agent_tag` 入 `commits` 表；
  human 不进果实数据（是背景，不是数据）
- 新子命令：`oac sync <root>`、`oac version`
- schema v2：`repo_sync` 与 `commits` 旁表（v1 库自动无损升级）

### 果实 UI（本版新增）
- 仓库页同步徽标（new 灰 / tracked 绿 / stale 红 + remote 标记）与「同步全部」按钮
- 图页顶栏**逐 agent 图例**：每个智能体一颗稳定身份色点（六色哈希调色板，
  跨帧跨机器一致），mixed 橙点收尾；红色让给状态语义
- 节点卡片果实标记分级渲染：常态单点（点阵档直接染果实色），
  选中/焦点卡展开为 ≤3 条横条，颜色=归属、长度≈变更文件规模。
  600 卡天花板基准（全展开）3.81 ms、日常 ≈2 ms，均在 16 ms 预算内；
  数据在 Reload 预聚合，每帧零查询
- **时间轴滑杆**：图页底部拖动时间游标，只看"游标之前"结出的果实；
  重聚合走内存日志线性扫，不查库不重扫；满格=全部
- 详情面板果实行：agent 名、commit 短 hash、日期、subject

### 工具链配合（本版新增，见定位文档）
- 发现层跳过 linked worktree 与 submodule（`.git` 是文件的目录）——
  Orca 式兄弟 worktree 不再被登记成第二棵树、commit 不再双计
- 归因经 trailer 认下 paperclip/Orca 所 spawn 的 claude/codex/opencode；
  观察保持单向（I2′），不驱动 agent、不写两个工具的状态

### 不变量
- I1 单向依赖、I2′「自动的只有观测（fetch + scan）」、I3 两条模型通道——CI 机械检查；
  R3 新增 forest 只走 git CLI、不碰 agent 与网络的守卫。
