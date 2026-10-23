# OpenArchitect 智能体子系统设计（切片 7 草案）：Go 移植的递归 harness

- 状态：**草案**——依赖切片 1/2/3 的落地事实，届时按实现结果定稿；本文件先锁定架构、边界与概念映射
- 日期：2026-10-23
- 借鉴来源：harness2（Google，arXiv:2610.14224，Apache-2.0）。**只借鉴架构与概念，不移植 Python 代码**；逐字引用其正则/语法的部分保留 Apache 出处标注

---

## 1. 定位与原则修订

切片 1 的第一原则是「观测为主，不执行：不调度 Agent」。本切片**修订而非推翻**它：

> 观测是主干，主干不变（scan / store / graph 的语义与依赖规则一字不改）。agent 执行是一个**独立边界子系统**：全部由用户显式发起、全程记账、跑完自动被观测主干复查。子系统自身仍不写任何外部系统——不调 Issue、不触发 CI、不碰云 API；模型访问只经本地 CLI 通道。

三条不变量（CI 机械检查 + 代码评审双重守住）：

- **I1 单向依赖**：`internal/agent/**` 可以 import `internal/core/**`；`internal/core/**` 与 `internal/scanner/**` **永不** import `internal/agent/**`。观测主干对 agent 一无所知。
- **I2 一切执行皆用户发起、皆记账**：没有定时器、没有自动触发。每次 rollout 写 ledger（§9），跑完触发一次普通扫描（§8），agent 的产出与人类提交在快照层一视同仁。
- **I3 模型访问只有两条通道**：**分析 agent**（§1.5，进程内）连用户配置的 OpenAI-compatible 端点（BaseURL 可指本地网关 / Ollama，local-first 不破）；**变更 rollout** 只经本地 CLI 子进程（`claude -p` / `codex exec` / `opencode run`，与 harness2 的 `CHANNELS` 做法一致）。两条都不捆绑厂商 SDK、不破 no-cgo 交叉编译。

## 1.5 角色划分（2026-10-23 修订）：分析 agent 为主

用户澄清：这个智能体的主要用途是**架构分析**，不是执行代码变更。子系统因此分两个角色，harness2 移植的全部内容（substrate / 树 / 递归）属于变更侧：

| 角色 | 用途 | 执行面 | 前置 |
|---|---|---|---|
| **分析 agent（主）** | 回答架构问题：「改这里会坏哪里」的解释与推理、跨仓依赖分析、图与预期偏差检测、快照 diff 解读 | **进程内**：covonaut `agentcore` 工具循环，工具 = core/store/graph 的直接函数调用 | 仅切片 1 的 core（scan + store + graph），**可最先交付** |
| **变更 rollout（次）** | 安全变更任务执行 + harness 递归优化 | 外部 CLI substrate（Claude Code / Codex / OpenCode） | §10 的 A/B/C |

**分析 agent 选 covonaut 的依据**（`covoyage/covonaut`，MIT，2026-06 创建，67 stars，pushed 2026-10-21，Go 1.25+）：

- 架构分析要查的数据本来就在自家 SQLite 与内存图里——CLI coding agent 为「改代码」付全套代价（子进程、shell 工具、格式漂移）却只读；进程内工具循环把 `impact/neighbors/search` 直接注册成 Go 函数，零子进程、结构化返回、`ctx` 取消、单二进制交付。
- MIT 同许可证族零污染；依赖链纯 Go（chromedp / creack-pty / mvdan-sh / OTel 均无 cgo），交叉编译矩阵不破。
- provider = OpenAI-compatible `chatcompat`（可配 BaseURL → 本地 Ollama/网关），薄 HTTP 客户端而非厂商 SDK。
- 内建能力与分析场景对表：MaxTurns + 参数校验的工具循环、Sessions/Checkpoints、**Human Review（HITL）**、流式事件 + OTel traces——agent 自身可观测，与本项目的观测哲学同构。
- 协议自带 **MCP（client 与 server）**、A2A、ACP、HTTP API：将来「OpenArchitect 出 MCP server 供外部 agent 消费架构数据」与「分析 agent 挂外部工具」同一条路。
- 成熟度风险（4 个月、pre-1.0）与 mygo/onevra 同类，缓解同三件套：**pin 版本 + 单包隔离 + 窄接口**（§4.1）。

## 2. 从 harness2 借什么（概念映射表）

| harness2 概念 | Go 移植 | 改动与理由 |
|---|---|---|
| Substrate Protocol（opencode/codex） | `internal/agent/substrate`：接口 + `os/exec` 驱动本地 CLI agent | 原样。它本来就不实现 agent，只 materialise 文件树 + 拉起 CLI |
| harness 树 c1–c8（AGENTS.md、`skills/*/SKILL.md`、`sub_agents/*/AGENT.md`、`scripts/*.py`、`plugins/*.ts`） | 文件格式**原样复用**（substrate 原生格式就是文件格式）；Go 只做生成/编辑/校验 | 原样。树即数据，`Tree map[string]string` + 路径正则校验（正则移植自 `core/tree.py`，保留出处注释） |
| `BenchSpec`「a value, not a code path」 | `TaskSource` 配置 struct，TOML 定义任务集 | 原样思想。任务定义是数据 |
| `BenchmarkAdapter` 协议（tasks/execute/delivered/trajectory/score…） | `TaskSource` 接口（§6） | 换成 OpenArchitect 数据驱动：任务来自 Feature/快照，不来自外部 benchmark |
| Propose–Probe–Compose 递归（parallel/sequential，k 预算，无效 proposal 重试一次 `STAGE_ATTEMPTS=2`） | `internal/agent/harness`：proposer（CLI 通道）→ probe（并发 rollout）→ compose（行为对比合成） | 原样。selection 用行为对比、无 ground-truth，照搬 |
| 经验库 Library（`<domain>/harnesses/<task>/` + `EXPERIENCE.md`，整树保留不摘要） | 目录树落 userData 目录，格式原样 | 原样 |
| ledger / runmark / resume（run_id 幂等、重跑即续跑） | ledger JSONL + 目录状态；`run_id` 幂等 | 原样 |
| preflight（烧钱前健康检查） | `agent preflight`：CLI 存在性与版本、认证、仓库干净度 | 原样 |
| judge（LLM 评分） | **替换**：图不变量评分为主（§6.3），LLM judge 可选后置 | 差异化核心：我们有可执行的架构事实，不需要 rubric |
| 三轴分离（method / substrate / benchmark） | 三轴分离（harness 方法 / substrate / tasksource） | 原样，这是它最干净的资产 |

## 3. 架构与目录

```
cmd/oac-agent/{main.go,ask.go,run.go,preflight.go,harness.go}   # headless 优先；GUI 触发是切片 C 的事
internal/agent/
  analysis/{agent.go,tools.go}                                   # 分析 agent：唯一允许 import covonaut 的包（§4.1）
  substrate/{substrate.go,claude.go,codex.go,opencode.go}        # 变更侧：CLI 执行与 trajectory 解析
  tree/{tree.go,surface.go}                                      # harness 文件树 + 校验
  harness/{propose.go,probe.go,compose.go,selection.go}          # 递归方法（不含任何 substrate 知识）
  tasksource/{source.go,safechange.go,feature.go}                # 任务生成与评分
  library/  ledger/  runner/  preflight/
```

CI 守门在切片 1 的两条之外新增第三条：

```sh
# I1：观测主干不知道 agent 存在
go list -deps ./internal/core/... ./internal/scanner/... \
  | grep -x 'github.com/futuremeng/OpenArchitect/internal/agent/.*' && exit 1 || true
```

## 4. 两个执行面：分析 agent（进程内）与 Substrate（变更侧）

### 4.1 分析 agent（covonaut，只读）

```go
// analysis/agent.go —— 唯一允许 import covonaut 的包；其余代码只面对这个窄接口
type AnalysisAgent interface {
    Ask(ctx context.Context, q string, opts AskOpts) (Report, error) // 只读工具循环，MaxTurns 封顶
}
```

工具 = core 查询的直接函数调用（无子进程、无文件写）：`impact(id, depth)`、`neighbors(id, dir)`、`search_elements(q)`、`snapshot_diff(a, b)`、`context_export(repo)`。**只读是结构性保证而非纪律**：工具集里不存在任何写工具，分析 agent 也不 import substrate/tree/harness 任何代码。协程→`win.Update` 的交接纪律（切片 1 §10）同样适用于 agent 结果回 UI。

### 4.2 Substrate（变更侧，Go 接口）

```go
type Substrate interface {
    Name() string
    Surface() tree.SurfaceSpec                       // 该 CLI agent 的可编辑面 schema（路径规则、上限）
    Materialize(treeDir Path, ctx map[string]string) (Install, error)  // 文件树 → substrate 原生配置
    Execute(ctx context.Context, req RunRequest) ([]RunDir, error)     // 拉起 CLI，一个 run_id 一个目录
    ReadTrajectory(runDir RunDir) (string, error)    // → 共享语法 [ACTION]/[OBSERVATION]/[ERROR]
    Preflight() error                                // 存在性/版本/认证，失败给修复指令
}
```

实现 = `exec.Command` + stdin/stdout 管道 + 超时/取消（复用切片 1 的 `context` 纪律）。trajectory 共享语法原样借用——proposer 读的就是这份文本，语法稳定比解析原生 JSONL 更重要。三家 CLI 的输出格式漂移是头号工程风险（§12-R1），按 substrate 版本 pin + preflight 版本断言缓解。

## 5. Base harness 生成器（OpenArchitect 的第一产出）

从当前快照生成该仓库的 base harness（即 harness2 语义里的 `base_harness()`）：

- `AGENTS.md`：架构摘要（Container 清单 + 关键 Component + 依赖方向），由 `oac context <repo>` 的 markdown 产出（切片 1 已含该命令）；
- `scripts/oac_impact.py`（薄壳，实际调用 `oac` 二进制）：教 agent 在工具循环里查影响面；
- `skills/oac-arch/SKILL.md`：跨仓导航。

这份 base harness 同时是**独立可分发物**：没有 GUI 的用户也能 `oac harness <repo> -o ./` 直接拿走。递归优化（§7）以它为起点。

## 6. TaskSource 与评分（差异化核心）

### 6.1 任务定义（TOML，数据不是代码路径）

```toml
[[task]]
id = "repoA/refactor-export-cache"
domain = "repoA/internal/cache"          # 经验库分组单位
requirement = "把 Export 的缓存层换成 LRU，保持对外行为"
target = "cmp:repoA/internal/cache.Export"   # 架构元素稳定 ID（切片 1 §6.2 的 ID 规则）
```

任务可由用户手写，也可由切片 3 的 Feature 数据一键生成（`feature → task`）。

### 6.2 「安全变更」任务族

对 `target` 跑影响分析，产出上游集合（改动前必须保住的调用方）与下游集合（改动会波及的依赖），自动注入任务 prompt——这是 agent 自己 grep 不出来的全局事实。

### 6.3 评分 = 图不变量，不用 LLM judge

rollout 结束后 OpenArchitect 重扫仓库 → 新快照，机械判定三件事：

1. 测试命令通过（默认 `go test ./...`，任务 TOML 可覆盖）；
2. **影响不变量**：新快照中从 `target` 出发 N 跳内的依赖集合 ⊆ 变更前集合 ∪ 任务声明的允许新增；集合外的模块不得出现新边；
3. 导出 API 面（`methods_json`）未破坏任务声明的保留清单。

评分失败 ≠ 0 分——区分「评分器故障」与「做得差」，这条语义纪律照搬 harness2（`score() -> None` 永不缓存为零）。

## 7. 递归：Propose–Probe–Compose

- **Propose**：proposer 经 CLI 通道读 base harness + 最近 rollout 的 trajectory，提出 invariant/domain 两组树编辑；`tree.Validate` 拒绝 → 重试一次 → 继承现树（对齐 `STAGE_ATTEMPTS = 2`）。
- **Probe**：候选树 × 任务并发 rollout。并发宽度默认 2——桌面场景本机资源受限，这是与服务器场景的最大差别。
- **Compose**：从多次执行的行为对比合成精化树，无 ground-truth 反馈。
- 模式：`--k 1`（单步）/ `--mode parallel|sequential`，语义与 harness2 对齐；默认 k=1；预算护栏（`budget` 上限 + preflight 成本估算）先于一切。

## 8. 观测闭环（切片 2 的下游红利）

每次 rollout 完成 → 自动触发一次普通扫描 → 新快照 → 切片 2 的快照对比视图把「agent 变更」着色为独立类别（ledger 的 run_id 与 git author 对齐）。审计不是 agent 子系统的功能，而是观测主干对它的**被动复查**——I1 保证这条数据流单向。

## 9. 执行、恢复与记账

- `run_id = hash(task_id, harness_fingerprint, substrate, seed)`：同 run_id 重跑即续跑（幂等纪律照搬 harness2 的 `execute()` 语义）。
- ledger：`runs/<task>/<run_id>/ledger.jsonl`（阶段、成本、耗时、错误），GUI 只读展示。
- 取消：`ctx` cancel → SIGTERM CLI 子进程 → ledger 记 `cancelled`；与切片 1 §10 的取消语义同构。

## 10. 分阶段

| 阶段 | 内容 | 前置 |
|---|---|---|
| **A0 · 分析 agent** | covonaut 接线（pin + 单包隔离）+ 只读工具集 + `oac-agent ask` + HITL 审批 + ledger 记账 | 仅切片 1 的 core。**独立成立，最先交付** |
| **A · Runner** | substrate（首批 2 家 CLI）+ tree + base harness 生成 + TaskSource + 图不变量评分 + ledger/resume。**无递归** | 切片 1（`oac --json` / `oac context`） |
| **B · 递归** | propose/probe/compose + selection + library | A；切片 3（Feature → 任务） |
| **C · UI** | 「任务」页（列表 + run 时间线 + 审计着色复用切片 2 diff）；分析 agent 的问答面板进架构图页 | B；切片 2 |

阶段 A 单独就有产品价值（架构接地的 agent 执行 + 机械审计）；B 是研究性投入，允许永远停在 A。

## 11. PoC 验证清单

| # | 验证 | 失败分支 |
|---|---|---|
| 1 | `claude -p` / `codex exec`（或 `opencode run`）子进程长任务稳定性、超时与取消 | 换另一家 CLI；接口面已抽象，只换 substrate 实现 |
| 2 | 三家 CLI 的 harness 文件格式（AGENTS.md 注入、skills 目录、hooks）当前版本是否稳定可读 | 降级：只用 AGENTS.md 单文件面（对应 harness2 四 install_mode 里最保守的 `workspace_agents_md`） |
| 3 | 图不变量评分在真实仓库上的重扫耗时与误报率 | 误报高 → 不变量收窄到「导出 API 面 + 测试」，图约束降为警告 |
| 4 | proposer 经 CLI 通道产出的树编辑 `tree.Validate` 通过率 | <50% → 递归降级为「人工挑选编辑」，B 阶段缓做 |
| 5 | 并发 2 个 rollout + 本机扫描的资源峰值 | 并发宽度=1，sequential 为默认 |
| 6 | covonaut `chatcompat` 指向本地 Ollama/网关的工具循环可靠性（流式、多轮 tool call、参数校验） | 换 provider 实现；`AnalysisAgent` 窄接口使 provider 层可替换 |
| 7 | covonaut 的 MCP client/server 与文档所述能力面（HITL、Sessions）在当前 pin 版本实际可用 | 只用 agentcore 最小面（工具循环 + MaxTurns）；MCP server 推后 |
| 8 | 引入 covonaut 后 `GOOS=darwin/windows/linux` 交叉编译矩阵仍无 cgo | 理论上不可能出 cgo（其依赖全纯 Go）；真出问题 → 分析层 ~200 行自写工具循环（工具定义 + 手写循环是全部用量面） |

## 12. 风险登记

1. **CLI agent 格式漂移**（opencode/codex 月度级更新）：substrate 按版本 pin + preflight 版本断言；接口面窄（6 个方法）使适配成本可控。
2. **递归优化成本高、收益不确定**：预算护栏 + 默认 k=1 + 阶段 A 独立成立，B 可无限期推迟。
3. **范围蔓延污染观测主干**：I1 + CI 机械检查；GUI 对 agent 只读。
4. **许可证**：概念借鉴不构成衍生作品；`tree` 路径正则与 trajectory 语法若逐字移植，文件头保留 Apache-2.0 出处注释。prompts 一律自写（harness2 的 `prompts.py` 1163 行不移植）。
5. **covonaut 年轻**（67 stars、2026-06 创建、pre-1.0、open issues 异常少说明真实使用面还窄）：pin 精确版本、只在 `internal/agent/analysis` 单包 import、对外只暴露 `AnalysisAgent` 窄接口；升级破坏面 = 一个包。逃生通道是「自写 ~200 行工具循环」，不是换另一个框架。

## 13. 未决问题

- **U1** 首批支持哪两家 CLI？倾向 Claude Code + Codex，以用户环境实测定。
- **U2** 阶段 A 是否需要 GUI？倾向不需要，`oac-agent` CLI 先行，GUI 留给阶段 C。
- **U3** 测试命令默认值与覆盖语义：进 `app_config` 还是任务 TOML？倾向任务 TOML。
- **U4** 经验库分组单位是 repo 还是包簇（harness2 用 domain）？倾向包簇，与切片 2 多仓合并图对齐后定。
- **U5** 跨仓任务是否在 B 阶段解锁？依赖切片 2 的 `Graph.Merge` 落地质量。
- **U6** 分析 agent 的模型端点配置放哪：设置页（BaseURL/model/key）还是 env？倾向设置页存 `app_config`（`config.agent.*`），env 作 CI/无头覆盖。
- **U7** 分析 agent 是否默认开启？倾向默认关闭、设置页显式开启并配置端点——「观测为主」的产品气质不因引入 agent 而变。