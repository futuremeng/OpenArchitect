# OpenArchitect 切片 2 设计：多仓合并图 + 增量扫描 + 快照对比 + 元素历史

- 状态：草案（待用户逐段确认）
- 日期：2026-10-26
- 上游：切片 1 已合入 `main`（merge `9ae93f0`，CI 绿）。本切片消费它留下的三处钩子：
  `Graph.Merge` 签名（`graph.go:112`，现返回 `ErrNotImplemented`）、稳定元素 ID
  （`cmp:<pkgpath>.<Name>` / `ctn:<module>`，切片 1 spec §6.2）、`migrations
  [][]string` + `PRAGMA user_version` 迁移机制（`store/schema.go`，现 v1）。
- Ix 调研落点（`research/2026-10-26-ix-system-intelligence.md` §6）中归本切片的三条：
  每文件 source hash（§3.3）、element 级历史（§3.6）、`rank` 查询（§3.5）。

---

## 1. 一句话与动机

**把"一个仓库的图"升级为"一人公司的图"**：同时观测 3~10 个相互引用的 Go 仓库，
跨仓边自动对齐；改动后不必全量重扫；每次扫描自动留下"什么变了"的记录，
可查两个快照的 diff，也可查一个元素的一生。

切片 1 验证了单机单仓闭环；切片 2 是产品定位（"多项目统一架构视图"）第一次
真正成立，也是切片 3（Git/Feature）与切片 7（agent 观测闭环）的数据地基。

## 2. 已核实事实（2026-10-26，全部读源码或实测，非 README 转述）

| 事实 | 证据 |
|---|---|
| `flow.State` 公开面只有 `Ready/Zoom/Fit/ZoomIn/ZoomOut/Reveal/Position(id)`；**相机 x/y 与节点尺寸不可读** | `onevra/source@v0.2026.12/components/flow/flow.go:85-114`，`view` 是私有内嵌 struct（`render.go:44`） |
| `flow.Options.Overlay` 只是 `func(*ui.Context)` 插槽，不接收相机/布局信息 | `flow.go:81` |
| mygo v0.5.5 **没有原生目录选择器**（`ui/` 无 Folder/FileDialog/OpenPanel 类 API） | 对 `ui/`、`platform/` 的 grep 为空 |
| 切片 1 扫描器 LoadMode = `NeedAllSyntax|NeedTypes|NeedTypesInfo|NeedDeps|NeedModule|NeedImports`，`packages.Load(cfg, "./...")` | `internal/scanner/language/go.go:25-32,266` |
| onevra 全量扫描（解析+写库）实测 **1.95 s**（147 元素/335 关系）；首帧出图（读库+布局）实测 **0.07 s** | findings 验收表 + `TestDoDProbeRealDB` |
| 快照写入是单事务；`LatestSnapshotID(repoID)` 已有 | `store/snapshot.go:23,54` |
| 关系类型只有 `contains/depends_on/calls`；`uses/implements` 在切片 1 被裁（偏差 #2，启发式无消费方） | `model/relationship.go`；切片 1 spec §3 偏差表 |

**minimap 的直接推论**：切片 1 spec §9.5 设想"自绘点阵 + 视口矩形"。点阵可行
（遍历元素调 `Position(id)`）；**视口矩形在现 API 下不可得**。本切片交付点阵 +
点击导航 + 选中/影响集高亮，视口矩形列为对 onevra 的上游 API 请求
（建议 `State.Camera() (x, y float32)`），不阻塞本切片。

## 3. 范围

### 3.1 包含

1. **多仓管理**：`repos` 表已有多行能力，补 GUI 仓库页（列表/添加/删除/逐仓扫描/全部重扫）与 CLI（`oac repo list|remove`，添加=对路径 `oac scan`）。mygo 无目录选择器 → 添加走路径文本输入 + 存在性与 `go.mod` 校验。
2. **合并图**：实现 `Graph.Merge`——多仓最新快照的并集图；GUI 架构图页加仓库过滤器（多选，含"全部"），选择持久化到 `app_config`。
3. **source hash**：新表 `arch_files`，每快照记录每个被解析文件的 SHA-256。同时服务 `oac verify` 与增量。
4. **增量扫描**：Merkle 包指纹（§6），默认增量、`--full` 强制全量。正确性红线：增量结果与全量**逐字节一致**（§11 平价测试）。
5. **快照对比**：core 层 `DiffSnapshots(a, b)`（§8）+ CLI `oac diff` + GUI 对比着色模式。
6. **元素历史**：新表 `element_history`，扫描时对新旧快照做 diff 落事件；CLI `oac history <element-id>`；GUI 抽屉加"历史"段。
7. **`oac rank --by dependents`**：load-bearing 排序（直接入度，graph 层顺手实现）。
8. **minimap（降级版）**：点阵 + 点击 `Reveal` + 选中/影响集高亮（§2 推论）。
9. **`oac verify`**：存储 hash vs 磁盘 hash，输出 `clean / stale` 与 stale 文件清单（Ix §3.3 的两值简化版——`diverged` 三值裁决需要"图中节点 vs 重解析"，本切片不产，见 §6.4）。

### 3.2 排除（各需自己的 spec）

`uses`/`implements` 边（roadmap 原挂切片 2，本 spec 明确移出：启发式识别在"改这里会坏哪里"上无消费方，切片 1 偏差 #2 的判定继续有效）· Git 集成与 Feature（切片 3）· 手工标注（切片 4）· 非 Go adapter（切片 5）· 打包发布（切片 6）· agent 子系统与退出码 2/3/4、`--format llm`（切片 7，Ix §6 决定）· 跨机共享/多用户（永远）。

### 3.3 退出码与输出格式纪律（承接 Ix §6）

本切片所有新 CLI 命令沿用既有纪律：退出码只有 0/1；输出 `text` + `--json`。
不为"歧义/不存在"引入 2/3/4——那是切片 7 出现程序化消费者时一次性定的码表。

## 4. 完成定义（DoD）

给定两个真实互相引用的 Go 仓库（fixture：`repoB` 纯库 + `repoA` 依赖 B，`go.mod` 带 `replace`）：

1. 两仓都扫描后，合并图出现 **A→B 的 `depends_on`/`calls` 跨仓边**，端点 ID 与两仓各自扫描时逐字一致；
2. 仓库过滤器切到仅 A / 仅 B / 全部，三种视图的元素计数与各自快照一致；
3. 改动 B 的一个文件 → `oac scan B` 默认增量：日志显示复用的未变包 ≥ 50%，且**增量结果与 `--full` 结果逐字节一致**（平价测试）；
4. `oac diff <repoB>` 报告恰为脚本化改动产生的变更集（元素/边两个维度）；GUI 对比模式同一数据着色；
5. `oac history <element-id>` 返回该元素 appeared/changed/disappeared 全序列，与脚本化改动吻合；GUI 抽屉历史段同源；
6. `oac verify`：扫描后 `clean`；touch 任一文件后 `stale` 且列出该文件；
7. `oac rank --by dependents --limit 10` 与测试预算的排序一致；
8. minimap：点阵渲染、点击点阵某点 → 镜头 `Reveal` 到对应元素（headless 可测）；
9. 合并图 ≥600 节点时首帧 ≤2 s（探针扩到合并库）；CI 三条守卫（R1/driver/oac headless）继续绿；`vet`/`test`/`-race` 全绿。

## 5. 数据模型（schema v1 → v2，全部增量）

```sql
-- v2 迁移，追加 migrations[1]，无对既有表的 ALTER（避免 SQLite ALTER 方言面）
CREATE TABLE IF NOT EXISTS arch_files (
  snapshot_id TEXT NOT NULL REFERENCES architecture_snapshots(id) ON DELETE CASCADE,
  file_path   TEXT NOT NULL,          -- 仓库根相对路径，与 arch_elements.file_path 同规则
  sha256      TEXT NOT NULL,
  PRIMARY KEY (snapshot_id, file_path)
);
CREATE TABLE IF NOT EXISTS pkg_fingerprints (
  snapshot_id TEXT NOT NULL REFERENCES architecture_snapshots(id) ON DELETE CASCADE,
  pkg_path    TEXT NOT NULL,          -- 第一方包 import path
  fp          TEXT NOT NULL,          -- §6 的 Merkle 指纹
  PRIMARY KEY (snapshot_id, pkg_path)
);
CREATE INDEX IF NOT EXISTS idx_files_snapshot ON arch_files(snapshot_id);
CREATE TABLE IF NOT EXISTS element_history (
  repo_id     TEXT NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
  element_id  TEXT NOT NULL,
  snapshot_id TEXT NOT NULL REFERENCES architecture_snapshots(id) ON DELETE CASCADE,
  event       TEXT NOT NULL,          -- appeared | changed | disappeared
  detail_json TEXT NOT NULL DEFAULT '{}',
  created_at  TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_history_element ON element_history(element_id);
CREATE INDEX IF NOT EXISTS idx_history_repo    ON element_history(repo_id, snapshot_id);
```

- `schemaVersion = 2`；迁移机制不变（`user_version` 整版成功后才 bump，拒绝降级已有测试钉住）。
- 历史保留策略：每仓快照数封顶 `maxSnapshotsPerRepo = 50`（`app_config` 可覆盖），超出删最旧快照——CASCADE 自动收走其 `arch_files`/`pkg_fingerprints`/`element_history` 行。这是 Ix「实体级历史」与磁盘有界的折中。

## 6. 增量扫描：Merkle 包指纹

### 6.1 为什么文件级增量在 go/packages 上不成立

`calls`/`depends_on` 边来自包 P 的**类型化引用**（`TypesInfo.Uses`），P 的引用解析
结果不仅取决于 P 的文件，还取决于其（传递）依赖的声明。单文件粒度复用会产错边。

### 6.2 指纹定义

```
fp(go.mod+go.sum)  = H(文件内容)                       // 全局哨兵
fp(P)              = H( sort(P 内第一方文件的 (path, H(content)))
                     ‖ sort(直接 import 的第一方依赖 Q 的 fp(Q))
                     ‖ fp(go.mod+go.sum) )
```

- 第三方依赖被 `go.mod` 版本 pin 住，故哨兵进每个 `fp(P)`：`go.mod` 变 → 全体指纹变 → 退化为全量（正确且便宜）。
- 指纹计算成本 = 读文件 + SHA-256，相对类型检查可忽略。

### 6.3 复用规则

- 上一快照（`LatestSnapshotID`）存有 `pkg_fingerprints` 时：新算 fp 相等的包 P，其**全部产出**（P 的 elements、P 发起的边、contains 结构）从上一快照**原样搬运**；fp 不等的包进入解析集。
- 解析集仍用 `packages.Load("./...")` 全载（LoadMode 不变）——**go/packages 没有包级增量加载**，本设计复用的是"分析+序列化+写库"段，不假装省掉 load 段。收益实测门槛见 PoC-9；若收益 <20%，保留指纹与 hash（verify 需要），停用复用路径，默认回到全量（开关在 `app_config`）。
- 上一快照缺 `pkg_fingerprints`（旧库、v1 快照）→ 自动全量。

### 6.4 verify 的关系

`oac verify` 只比对 `arch_files` vs 磁盘 hash（不重解析）：`clean` = 全等；
`stale` = 列出 hash 不匹配/缺失/新增的文件。Ix 的第三值 `diverged`
（图与源码不一致但 hash 相同）需要重解析比对，本切片不产，verify 输出里
明确只有两值——诚实比三值好看更重要。

## 7. 合并图语义

- `Graph.Merge(graphs []*Graph) *Graph`（对切片 1 留的 `Merge(other)` 两两形态做
  有意修订：并集运算 N 路一次做完更直接，调用方只有 App 与 CLI，改形成本可控，
  记录为对"调用方不必改形"承诺的一处偏差）。
- 并集：元素按 `element_id` 去重（稳定 ID 是 pkgpath 制，跨仓对齐是设计根基，
  PoC-8 钉它）；边保留当且仅当两端都在并集内（单仓视图里被过滤的外部边，
  在合并视图里因对方仓库在场而复活——这是跨仓边的全部来源，扫描器不改）。
- 冲突规则：同一 `element_id` 出现在两仓快照（vendored/复制代码）→ 归属
  **模块路径最长前缀**的一方；平局取仓库选择顺序靠前者并记 warning。
- 快照选择：合并图 = 每个选中仓库的 `LatestSnapshotID`。跨快照混合（A 用今天、
  B 用昨天）是允许且真实的——一人公司的仓库本来就不同时提交。

## 8. diff 与历史的判定

`DiffSnapshots(a, b)`（core，GUI/CLI/历史三处共用一个实现）：

- 元素：`added` = b 有 a 无；`removed` = a 有 b 无；`changed` = 同在但
  `element_type/file_path/methods_json/description` 任一不同（detail 记字段名列表）。
- 边：三元组 `(from,to,type)` 集合差，`added/removed`。
- 历史事件在扫描提交事务内计算（旧快照 = 该仓上一最新快照）：added→`appeared`、
  removed→`disappeared`、changed→`changed`（detail_json = 字段名列表）。
  同事务保证：快照、hash、指纹、历史四者原子。
## 9. CLI 增量（`cmd/oac`）

| 命令 | 说明 |
|---|---|
| `oac scan <path> [--full]` | 默认增量；stdout 报告 `reused=<n> parsed=<m>` |
| `oac repo list / remove <repo-id>` | 删除级联清快照（CASCADE 已有） |
| `oac diff <repo-id> [--from s --to s] [--json]` | 默认该仓最新两个快照 |
| `oac history <element-id> [--limit N] [--json]` | 时间倒序 |
| `oac rank [--repo R / --all] [--by dependents] [--limit N] [--json]` | 默认 dependents |
| `oac verify [<repo-id>] [--json]` | 两值裁决 + stale 清单 |

`graph/impact/context` 增加 `--all`（合并图）与可重复 `--repo`，缺省行为不变
（单仓最新快照）——对切片 7 已定稿的 CLI 契约只做**加性**扩展。

## 10. GUI 增量

1. **仓库页**（切片 1 的占位页转正）：列表（名称/路径/最近扫描/新鲜度徽标：
   `clean/stale` 由 verify 的 hash 比对得出，扫描后缓存进 `app_config`）、
   路径文本输入添加、删除确认、逐仓扫描 + 全部重扫。
2. **架构图页**：仓库过滤器 chips（含"全部"），选择持久化到 `app_config`；
   对比模式开关（选两个快照 → 节点着色：added 绿描边 / changed 琥珀描边 /
   removed 虚线幽灵节点，边 added 高亮 removed 虚线；计数行进 mapHeader）；
   minimap（右下 Overlay：点阵来自 `Position(id)` 遍历，点击 → `Reveal`，
   选中与影响集高亮；**无**视口矩形，§2）。
3. **抽屉**：新增"历史"段（最近 5 条事件 + 计数），数据 = `element_history`。
4. 相机/层级记忆沿用切片 1 机制。

## 11. 测试策略（平价是核心）

- **平价测试**（本切片最重要的一条）：fixture 改动一个文件 → 增量扫描结果
  vs `--full` 结果，元素与边按 (id/type/sorted) 规范化后**逐字节相等**。CI 必跑。
- 指纹单测：fp 对文件内容/依赖/`go.mod` 变化的敏感性；对无关重排（文件顺序、
  import 顺序）的稳定性。
- Merge 单测：ID 对齐、边复活、冲突归属（最长前缀规则）、空并集、重复仓去重。
- Diff/历史单测：脚本化变更序列 → 期望变更集/事件序列；封顶裁剪保留最新 50。
- verify 单测：clean/stale 两态 + 文件清单 + 新增文件被识别。
- GUI：过滤器计数、对比着色映射（复用 `Muted`/描边通道）、minimap 点击→Reveal
  语义（headless Tester，断言前 `Advance` 过动画——切片 1 探针经验）、
  抽屉历史段渲染。
- 探针扩展：`TestDoDProbeRealDB` 增合并场景（onevra + OpenArchitect 自身两仓）：
  首帧 ≤2 s、并集计数 = 两仓计数和 − 重叠；跨仓边断言放 fixture（真实两仓无
  互相 import）。

## 12. PoC 门槛（失败分支先写好，执行在 plan 阶段）

| # | 验证 | 失败分支 |
|---|---|---|
| PoC-8 | 双仓 fixture（`replace` 指向本地）扫描后跨仓边存在且 ID 逐字对齐 | `replace` 不生效（proxy 副本对齐失败）→ 文档化 `go.work` 指引；ID 结构性错位才回炉 ID 方案 |
| PoC-9 | 增量在 onevra 上：平价测试绿 + 墙钟收益 ≥20% | 收益不足 → 停用复用路径（默认 full），hash/指纹/verify 保留；平价不绿 → 增量整体禁用，切片 2 其余照常 |
| PoC-10 | minimap：Overlay 内自绘点阵可点击命中并 `Reveal` | Overlay 命中被 flow 吞 → minimap 移出 `flow.View` 成独立小面板（`Position(id)` 跨帧可用，state 在手） |
| PoC-11 | 合并图 ≥600 节点（多仓）首帧与可读性 | 帧时间崩 → 默认仅 Container 层 + 过滤器收窄；仍崩 → 切片 1 逃生通道（WebView+React Flow）只换 `internal/ui` |

## 13. 风险

- **增量正确性是本切片最大风险**：Merkle 论证（§6.1）覆盖传递依赖，但
  `go/packages` 的包级结果是否只由这些输入决定，靠 PoC-9 平价测试实证，
  不靠论证自洽。
- **合并图规模**：3~10 仓 × 数百节点 = 数千节点，flow 的实测上限在 600 级
  （切片 1 PoC-4）；PoC-11 是硬门槛，过滤器/分层是缓冲。
- **历史封顶与 CASCADE**：删快照连带删其历史行——"元素的一生"在 50 快照
  窗口内完整，窗口外不完整；UI 文案要诚实（"近 50 次扫描内"）。
- **上游 API 缺口**（minimap 视口矩形）：向 onevra 提 issue，不私改依赖。

## 14. 未决问题（plan 第一步解决）

- **U1** `Container` 层在合并图里的形态：每仓一个 Container 节点自然成立，
  但"System 层"是否需要一个跨仓总根？倾向不加（flow LTR 布局下多根森林可行），
  PoC-11 看图定。
- **U2** 删除仓库时历史去留：`element_history.repo_id` 带 FK CASCADE → 删仓
  即删历史。定：级联删干净，孤儿历史没有查询入口。
- **U3** 对比模式的幽灵节点（removed）参与不参与 flow 布局（参与 = 布局稳定
  但图更大；不参与 = 只列清单）。plan 内做小 spike 定。
- **U4** `rank` 的 dependents 度量是直接入度还是 2 跳加权：先直接入度，
  切片 7 用数据说话再调。

