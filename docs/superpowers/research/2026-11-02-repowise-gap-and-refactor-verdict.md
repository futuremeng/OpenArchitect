# 调研（2026-11-02）：repowise 对照，与「是否需要重构」的裁决

- 对象：<https://github.com/repowise-dev/repowise>（AGPL-3.0，8.1k stars，3765 commits，Python monorepo + Next.js web）
- 来源：README、`docs/layers/INTELLIGENCE_LAYERS.md`、`docs/architecture/ARCHITECTURE.md`、
  `docs/architecture/code-health.md`、`docs/architecture/pluggable-storage.md`、
  `docs/reference/CLI_REFERENCE.md`、`ROADMAP.md`（2026-11-02 抓取）
- 我方现状证据全部来自 `main@f865f47` 的代码实测，不是 spec 转述
- 关联：`2026-10-26-ix-system-intelligence.md`（同类对照，结论互补）、切片 1/2 spec、
  `2026-10-27-graph-interaction-gap-review.md`

---

## 1. repowise 是什么

一句话：**一次索引（`init`）→ 五个智能层 → 三种交付面（CLI / MCP / dashboard）**，
卖点是「别让 agent 反复重读你的代码库」——它把同一份索引既给人看（dashboard），
也给 agent 用（MCP + hooks + 生成的 CLAUDE.md）。

它的自我表述是「one index, five layers」：

| 层 | 输入 | 产出 | 要不要 LLM |
|---|---|---|---|
| 1 Graph | 源码 AST（42 语言在支持阶梯上，28 个解析到全 AST） | 节点=file/symbol，17 类边，每条 `calls` 边带 39 种「解析来源」之一与可信度；PageRank/betweenness/Leiden 社区/SCC/入口执行流 | 零 |
| 2 Git history | 仓库自身历史 | churn/hotspot、co-change 对（含无 import 的隐式耦合）、owner、bus factor、bug-fix 提交、Change Records、AI attribution | 零 |
| 3 Docs | 图 + 结构（可选模型润色） | 每模块/每文件一页 wiki + 全文/语义/符号/路径四路检索 | 可零（结构渲染） |
| 4 Decisions | 代码/git/会话挖掘 | ADR 式决策记录 + 「哪个 hotspot 没有决策管辖」 | 零为主 |
| 5 Health | 图 + git + coverage | 53 个 detector（biomarker）投票 → 每文件分数/五档 band → `Fix first` 队列（impact/effort 排序） | 零 |

组合才是产品：`get_risk(changed_files)` = 图的 dependents **+** git 的 co-change
**+** 能到达这段改动的测试；`untested_hotspot` = 高 churn **+** 图中心度 ≥4
dependents **+** 无测试到达。任何单层都给不出这些答案。

交付面：57 个顶层 CLI 命令；18 个 MCP 工具（默认注册 10）；本地 Next.js dashboard；
agent hooks（编辑时把「这文件近 30 天修了 7 次」塞进上下文）；生成的 CLAUDE.md 管理段。
`init` 之后每次 commit 由 post-commit hook 跑增量 `update`，索引落后时 MCP 响应带
`stale_warning`。

**关键判断：它的哲学和我们同构**——「Graph/risk/health/tests/dead code 零模型调用」
就是「观测为主」的营销版。所以差距不是路线分歧，是**同一路线上走到了第几层**。

## 2. 裁决：不重构，补层 + 调排期

三条证据说明现有骨架是对的，推倒重来只会烧掉已经验证过的资产：

1. **分层同构且已被 CI 机械守住**。我们的 `cmd → ui → scanner → core{model,store,graph}`
   与它的 `cli/server/web → core{ingestion,analysis,generation,persistence,providers}`
   是同一形状。切片 1 的 R1/R2/R3（`internal/core/**` 与 `internal/scanner/**` 不得
   import 渲染栈，CI 用直接 import 检查，`tools/ciguard` + `.github/workflows`）
   正是它 `IndexStore`/`GraphStore` ABC 分离的 Go 版。它的
   `pluggable-storage.md` 结尾原话是「Defaults stay the defaults」——单机单用户下
   它的默认形态就是我们的形态，我们没有任何需要为多租户/服务端预留的结构。
2. **扩展是加表不是改表**。它的 health 层落地方式是「追加 4 张表 + 一个 alembic
   迁移」（`0019_code_health.py`）；我们的 `migrations [][]string` + `PRAGMA
   user_version`（`internal/core/store/schema.go:6`，现 v1，5 张表）是同一机制，
   切片 2 已预告 v2。**加一层分析 = 加一张表 + 一个新包，core 主干一字不改。**
3. **交付面扩展零结构成本**。它的 MCP 面就是「把 core 查询函数注册成工具」。
   我们 R1 已保证 core 不 import UI、`cmd/oac` 只做装配（`cmd/oac/main.go:34` 的
   `subcommands` 表），加 `oac mcp` 是加一个子命令，不是加一层。

**真正的差距是产品面宽度，不是代码质量**：repowise 回答 8 类问题
（这是什么 / 怎么工作 / 谁调用它 / 改了会坏什么 / 为什么这么设计 / 先修什么 /
能删什么 / 该跑哪些测试），我们现在回答 2 类，其中「改了会坏什么」只靠静态边。
按缺口性质分，需要动的只有两处窄改动（§5）和一处**唯一真正需要重新设计的
东西——元素粒度**（§6）。

## 3. 逐层对照（我方实测）

| repowise | 我方现状（`main@f865f47`） | 差距性质 |
|---|---|---|
| Graph：file/symbol 粒度，17 类边，边带解析来源与可信度 | 3 类元素 `System/Container/Component`（`model/element.go:14-16`）× 3 类边 `calls/depends_on/contains`（`model/relationship.go:6-8`）；Component = 导出的 named type；边无置信度标注 | **粒度**（§6，唯一需要重新设计的一条） |
| 图算法：PageRank / betweenness / Leiden 社区 / SCC / 执行流 | 只有 BFS 邻居 + N 跳影响集（`graph/bfs.go`、`graph/impact.go:19`）+ 包/仓聚合（`graph/aggregate.go:38`）；`Merge` 仍 `ErrNotImplemented`（`graph/graph.go:112`） | 补算法，一轮 BFS/邻接表级别的活 |
| Git 智能层：churn/hotspot/co-change/owner/bus factor/bug-fix | **完全没有**。`os/exec` / go-git 在 `internal/` 与 `cmd/` 里 grep 为空；`CommitHash` 恒空串（`model/snapshot.go:14`） | **最大的产品差距**（切片 3 已规划，但优先级被低估，§4-1） |
| Health：53 detector → 每文件分数 + band + Fix first 队列 | 无任何「结论」物化。schema 只有 elements/relationships 两张事实表，没有 findings/metrics 表 | **schema 缺口**（additive，§5-A） |
| Dead code / doc drift / smells / impacted-tests | 无。`in_degree == 0` 这类判定我们的图现在就能算，但没人算 | 补查询，成本最低收益最高 |
| Docs 层：LLM 生成 wiki + 四路检索 | 无。`oac context`（markdown 架构摘要）未实现，`cmd/oac/main.go:34` 的 `subcommands` 只有 `scan/graph/impact` | 有意排除（§7），但 `context` 该回来 |
| Decisions 层 | 无。切片 4「手工标注」是它的弱化版 | 有意排除，切片 4 范围内 |
| 交付面：CLI + MCP(18) + hooks + 生成的 CLAUDE.md + dashboard | GUI（mygo 原生，10 个非测试文件）+ CLI 3 命令；无 MCP、无 skill 目录、无 `--format llm` | **排期错位**（切片 7 才做，§4-3） |
| 增量：post-commit hook + `watch` + `stale_warning` | 切片 2 的 Merkle 指纹 + `oac verify`（**未落地**，schema 仍 v1） | 已在计划内 |
| 多仓 workspace：跨仓 contract 匹配 + breaking-change 检测 | 切片 2 的 `Graph.Merge` + 悬空跨仓边（`scanner/language/go.go:318-340` 已落） | 我们只到「边能并」，它到「契约能配」 |
| 42 语言 / 28 全 AST | 1 语言（Go adapter） | 切片 5，观察库实测 17/20 仓的真实需求 |
| 交互：dashboard 是 Next.js 网页，图是其中一页 | 原生桌面 + 分层下钻 + LOD 三档 + 深度锚点 + NavigationMap | **我们的差异化**，不降级（同 Ix §4 判定） |

一句话：**我们有第 1 层的约六成，第 2/5 层 0%，第 3/4 层是主动不做**；
而 repowise 的价值恰恰在第 1 层之上的组合。这就是「与预期差距很大」的算术来源。

## 4. 值得直接借鉴（按优先级）

1. **把 Git 智能层提到图交互之前**（P0，改切片顺序）。repowise 全部「有牙齿」的
   结论都是 graph × git 的积：hotspot、co-change 隐式耦合、untested hotspot、
   bug-magnet、change risk。我们的影响分析目前是纯静态可达集——`oac impact`
   给出的那 15 个上游里，哪些「每周一起改」、哪些「三年没人碰」，我们分不出来。
   落点：切片 3 的 Git 部分单独提前，`git log --numstat` 一次遍历 → 新表
   `file_metrics(repo_id, file_path, commit_count_90d, last_author, churn)` +
   `co_change(a_file, b_file, n)`；`graph.Analyze` 的结果按 hotspot 加权排序。
   参考它的诚实纪律：**co-change 只报事实与依据，不打分**（`CHANGE_RISK.md`：
   rows give a basis, never a score）。
2. **findings 成为一等公民**（P0，§5-A）。它的 health/dead-code/doc-drift 全部落库成
   findings（稳定 id + severity + confidence + basis + status），因此可排序、可跨快照
   对比、可进 Fix first 队列、可由 MCP 直接吐。我们现在所有结论都是请求时即时算完
   即丢，无法回答「上次扫描发现的问题修掉了几个」。保守置信度规则可整段照抄：
   `unreachable + 90 天无提交 + 最后提交 >1 年 → 1.0`，`最近被碰过 → 0.4`，
   `safe_to_delete` 只在 ≥0.7 才给。
3. **MCP 面提前到切片 3 末**（P1）。它的第一交付面是 agent，不是 GUI；18 个工具里
   默认开 10 个，全是 core 查询的薄包装。我们的分析 agent（切片 7 A0）本来就要把
   `impact/neighbors/search` 注册成工具——**同一份工具表同时出 MCP server 和进程内
   工具**，成本几乎不变，收益是 Claude Code/Codex 立刻能消费架构图。这条是纯排期
   收益，结构上零风险（R1 已保证 core 可无 GUI 使用）。
4. **边的可信度显式化**（P1）。它每条 `calls` 边带「解析来源」（39 种之一）与可信度，
   UI 与 CLI 据此说明「这条边该信几分」。我们的 `depends_on/calls` 来自 `go/types`，
   天然高可信，但**跨仓悬空端点**（切片 2 新落的那批）与「接口方法调用记到接口类型
   本身」（切片 1 spec §6.4，不做动态分派）这两类必须标注——否则 UI 上一样粗的线，
   用户无法知道哪条是猜的。落点：`Relationship` 加 `Origin`/`Confidence` 两列。
5. **rank/next 的排序口径**（P1，切片 2 的 `rank` 顺手升级）。它用
   `total_impact / effort_bucket`（只算 open findings）并显式排除噪声。我们已定
   「dependents 只算 `calls`+`depends_on`，`contains` 不计」——方向一致，补一个
   hotspot 权重即可从「谁被依赖多」升级成「先动谁划算」。
6. **新鲜度是一等状态**（P1）。它每次响应带 `stale_warning`，`doctor` 检查索引漂移。
   我们的 `oac verify`（未落地）就是同一件事，切片 2 已有表；补一条：GUI 顶栏常驻
   新鲜度徽标，MCP 响应带同一字段。
7. **文档即接口**（P2）。它把命令参考做成 skill 目录 + 一键装进各家 harness。与
   Ix §3.8 结论一致：oac 命令面稳定后随仓库带一份 skill。

## 5. 需要动的两处（是加，不是重构）

**A. `internal/core/findings` 新包 + 一张 `findings` 表（schema 追加一版）**

```
internal/core/findings/{findings.go,deadcode.go,hotspots.go,smells.go}
  输入：[]*graph.Graph + file_metrics      输出：[]Finding
  Finding{ID(稳定), Kind, Severity, Confidence, ElementID|FilePath, Basis, SnapshotID}
store：findings 表（按稳定 ID upsert），跨快照可查「新增 / 已消失」
```

它天然满足 R1（不 import UI），与 `graph` 包平级；GUI 加一个「发现」页，CLI 加
`oac findings [--kind dead-code] [--json]`，MCP 直接注册同一个函数。
**core 主干（model/store/graph/scanner）一字不改**——这就是「不用重构」的技术理由。

**B. 交付面从两面变三面**：`cmd/oac mcp`（stdio，只读工具集）+ 一份
`skills/openarchitect/`。工具表与切片 7 A0 的进程内工具表共用同一份定义。

## 6. 唯一需要重新设计的一条：元素粒度

这是本次对照里我唯一会说「值得改现有设计」的地方，必须诚实标出来。

repowise 的一切指标挂在 **file / symbol** 上：churn 是文件级的，coverage 是文件级的，
duplication 是 token 级的，`health_file_metrics` 主键是 `file_path`。我们的节点是
**C4 的 System/Container/Component**，Component = 一个导出的 named type
（切片 1 spec §6.3）。后果：

- **文件级事实无处可挂**。一个包 8 个文件里 1 个是 hotspot，我们的图上只有一个
  Component 节点，hotspot 只能挂在那一个 type 上，或者不挂。
- **dead code 判定粒度错位**。它的 `unreachable_file` / `unused_export` 两级，我们的
  Component 粒度只能出「type 入度为 0」，而 Go 里一个只被反射/JSON 用的 struct
  与一个真死代码在图上长得一样。
- **diff 与 element_history（切片 2）会漏掉「没有 type 的文件」**——纯函数文件、
  只有常量的文件、`main` 包（切片 1 §6.3 明确排除 main 包内类型）。这类文件在
  我们的视图里**根本不存在**，而它们恰恰是 diff 里最常见的。

三个选项：

| 方案 | 做法 | 代价 | 判断 |
|---|---|---|---|
| (a) 加一类 `File` 元素 | `ElementType` 追加 `File`，ID `fil:<module>/<path>`，`contains` 边 File→Component；type 级节点不动 | schema 零改（`element_type` 是 TEXT），flow 布局多一档 LOD，UI 默认不渲染 File 层 | **推荐**。additive，且立刻给 git/health/diff 一个挂载点 |
| (b) 全面下沉 file/symbol | 重写映射规则，Component 不再是 C4 语义 | 推翻切片 1 §6.3 与全部 golden fixture；「架构图」定位变成「代码地图」 | 不推荐——这是产品定位变更，不是重构 |
| (c) 不动 | git/health 指标只在详情面板按文件路径列表显示 | 图上看不到 hotspot，第 2 层的价值砍半 | 可接受的最小方案，切片 3 想少干活时选它 |

**方案 (a) 已在 2026-11-02 裁决采纳**——证据是 treemap 的 PoC-13：以元素数当面积时
OpenArchitect 41% 的包（21/51）与 OpenLaw 57%（17/30）面积为 0 被组件**静默省略**，
64%/47% 的文件不贡献任何元素（`../poc/2026-11-02-treemap-poc.md`）。`File` 元素同时是
矩形树图的面积单位、`file_metrics` 的外键落点、和快照 diff 不再漏文件的根据——
一个 scanner 改动 + 一类 ID，schema 零改。

## 7. 不该学的

| repowise 的做法 | 为什么不跟 |
|---|---|
| Python monorepo + `uv` + Next.js dashboard + FastMCP server | 破 local-first / 单二进制 / 无运行时。我们的原生桌面 + 单 SQLite 是差异化，不是欠账 |
| AGPL-3.0 + 商业授权层 | 我们是 MIT，且没有企业销售面 |
| 42 语言 / 53 detector 的广度 | 我们的观察库是 3 个 Go 仓在跑。先把 Go 这一层做深，语言广度是切片 5 的 tree-sitter sidecar |
| LLM 生成 wiki 页 + 置信度衰减（confidence decay） | 「观测为主」不允许把结论建立在模型生成的散文上；`oac context` 该是结构导出的 markdown，不是 prose |
| walker 不缓存 AST、每文件重解析（~1 ms/file） | 我们已有 Merkle 指纹路线（切片 2），比它更省 |
| 57 个顶层命令 | 命令面宽度是它的营销资产不是必需品。`oac` 保持 8±3 个命令 |

## 8. 建议落点（谁改哪份文档）

1. **切片 2 plan**（未落地部分）：照原样执行，**不加新任务**。§4 的 1/2/3 全部落在
   切片 2 之外。唯一例外：`oac rank` 的排序口径若采纳 §4-5，改在 rank 那一个任务里。
2. **新增切片 3 spec**：Git 智能层提前，范围 = `git log` 一次遍历 + `file_metrics` +
   `co_change` 两表 + 影响分析按 hotspot 加权 + 仓库页新鲜度。**前置**：§6 的粒度裁决。
3. **切片 3 或 4 的 schema**：`findings` 表 + `Relationship` 的 `Origin/Confidence`
   两列（§5-A、§4-4）。
4. **切片 7 spec**：把 A0 的工具表拆成「一份定义，两个宿主」（进程内 + `oac mcp`），
   并明确 MCP 面可提前到切片 3 末。
5. **切片 5 spec**：tree-sitter sidecar 路线再获一次实证（它 28 语言全 AST 走同一条
   路），照 Ix §3.7 的结论不变。
6. **README**（仓库还没有）：把 R1/R2/R3 分层契约与「我们有第 1 层」的公开定位写出来。
   repowise 每个数都有出处页，这种「敢给口径」是它的信任来源，我们至少要把自己的
   口径写清楚。
7. **可视化（2026-11-02 追加）**：矩形树图作为图页的第二个视图，与切片 3 的 git 层
   同期落地——组件已在 fork 的 `components/treemap/` 里，工作量只有一个数据映射文件。
   裁决与面积语义见 `2026-11-02-treemap-view-feasibility.md`。

## 9. 风险

1. **对照带来的范围膨胀**是本次最大风险。repowise 有 3765 commits 和一个团队；我们的
   杠杆是「一人公司 + Go + 原生桌面」的窄而深。**一次只补一层**，每层先立表再实现，
   不要同时开 findings + git + health 三条线。
2. **git 层的隐性复杂度被低估**：co-change 需要路径重命名跟踪（`--follow`）、merge
   提交去重、噪声路径过滤。它的文档里专门写了「测过但没发布」的 SZZ 工作——
   **先只报事实与依据，不打分**，与它的纪律一致。
3. **粒度方案 (a) 会让节点数近乎翻倍**（每文件一个 File 节点），PoC-11 的规模门槛会更
   紧。File 层默认不进 flow 布局、只在 LOD 与详情面板出现，可绕开。
4. **AGPL 素材污染**：只借鉴概念与口径，不逐字移植；任何阈值（0.7 置信度、
   ≥4 dependents）自定并写明来源。与切片 7 §12 风险 4 同一条纪律。

## 10. 一句话结论

**不需要重构。** 骨架（分层 + 稳定 ID + 单事务快照 + 迁移追加 + CI 依赖守卫）与
repowise 的 core 分离同构，且已被我们的 CI 机械守住；差距全部是**层数**：我们有它的
第 1 层（约六成），第 2 层（git）与第 5 层（health/findings）是 0，第 3/4 层主动不做。
所以正确的动作是三个：**(1) 把 Git 智能层从切片 3 提前并作为下一个主线**（它是我们
所有「有牙齿的结论」缺的那一半）；**(2) 新增 `internal/core/findings` + 一张 findings
表**，让结论可排序、可跨快照对比、可被 MCP 直接吐；**(3) 把 MCP 面从切片 7 提前**，
一份工具表两个宿主。唯一真正需要重新设计的是**元素粒度**（§6）：推荐加一类 `File`
元素作为 git/health/diff 的挂载点，其余一律不动（**已裁决**，见 §6 末与 PoC-13）。
切片 2 未落地的部分照原计划做完，不因为这份对照加任务。

