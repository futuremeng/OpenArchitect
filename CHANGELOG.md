# Changelog

## Unreleased

- 生长切片 E（树冠）：新页「树冠」= treemap 视图——面积=文件当前字节数，
  颜色=距最近一次生长事件的新旧（30 天内全绿，一年褪到灰，无事件=灰），
  角标=按文件精确重合数出的果实 commit 数；点开文件跳到架构图并选中其中
  组件。前置的 `File` 元素落地（PoC-13 裁决）：scanner 为每个非测试 .go
  产 `fil:` 元素（ghost 文件也有面积），schema 零改；File 元素只走树冠与
  diff，不进 C4 图。重扫后首帧关动画（treemap 调研 §9-4）
- 生长切片 E（时间轴）：底部时间轴从「果实游标」升级为「commit 序回放」——
  一条轴同时驱动果实与生长两个故事；生长事件时间经 commit 解析（事件自带
  hash 优先，退到 snapshot_to 快照的 commit，都落空按最老算）；拖动即回放
  生长标记，重算全程不查库
- 生长切片 E（节点卡部分）：flow 节点卡新增「生长」区——展开卡画最近 ≤3 条
  事件小方块（added 绿 / removed 红 / signature-changed 橙 / edge-changed
  中性），折叠态一颗点，点阵档不画；数据在 Reload 一次装填 `growthByEl`，
  帧内零查询，卡片文本行数不变（§9.3 守门测试钉住）
- 修复：`gitcmd.IsRepo` 认「是仓」不再认「在仓里」——scan root 恰在外层
  git 仓内部时，不再把自己登记成带外层身份的幽灵仓（bare 仓仍认）
- 生长切片 B：跨快照 element 级 diff 引擎——`growth_events` 表（schema v3）、
  `growth.Diff` 纯函数（added/removed/signature-changed/edge-changed）、
  sync 重扫后自动记账并按文件重合归因 commit、`oac growth <repo-id>` 查询面；
  首扫是基线不是事件，edge 事件故意不归因（重合即猜测）
- 发布包形态升级：tar.gz 内含 `oac`（裸二进制）+ `OpenArchitect.app`
  （bundle，Info.plist 版本取自标签，图标由 resources/icon.png 生成，
  ad-hoc 签名）；自更新按 bundle 内小写 `openarchitect` 认身，闭环不变

## 1.1.0 — 2026-11-10

发布闭环：tag → CI Release → 应用内自更新。

- **GitHub Releases 自动化**（`release.yml`）：推 `v*` 标签构建 darwin
  arm64/amd64 双包 + sha256 checksums；发布说明直接取 CHANGELOG 小节；
  git tag 与版本常量不一致直接断在 CI
- **`oac update [--check]`**：查询 GitHub Releases，下载验 checksum 后
  原子替换自身二进制（rename 换身、失败回滚）；go-run 出来的临时二进制
  会被拒绝，不会误伤
- **设置页「版本与更新」**：显示版本、检查更新按钮、发现新版给安装按钮，
  装完明说重启后生效；检查/安装走后台，不碰扫描闸门与数据库
- 版本号单一来源 `internal/version`；网络只在用户明确触发时出现（I2′ 不变）

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
