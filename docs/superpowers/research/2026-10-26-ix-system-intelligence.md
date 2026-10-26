# 调研：ix-infrastructure/Ix 对 OpenArchitect 设计的启示

- 日期：2026-10-26
- 对象：https://github.com/ix-infrastructure/Ix （Apache-2.0，1.1k stars，1345 commits）
- 来源：README、docs/llm-format.md（43KB 输出格式规范）、docs/mcp.md、CONTRIBUTING.md 实测抓取
- 关联 spec：`../specs/2026-10-19-openarchitect-slice1-design.md`、`../specs/2026-10-23-openarchitect-agent-harness-design.md`

---

## 1. Ix 是什么

一句话：把仓库解析成**持久化的系统图**（symbols / calls / imports / 关系），让人和 coding agent 查询结构而不是 grep 猜。

模型：**Map → Structure → Retrieve → Remember**

| 组件 | 技术 | 角色 |
|---|---|---|
| `ix-cli` | TypeScript/Node CLI | 全部用户命令；`ix mcp` stdio server 也在这里 |
| `core-ingestion` | TypeScript parser library | tree-sitter 解析 **27 种语言**，抽 symbols/calls/imports |
| memory layer | **闭源 Scala**（私有仓库 Ix-memory），Docker 镜像 `ghcr.io/.../ix-memory-layer` 监听 `127.0.0.1:8090` | 图查询 HTTP API（`/v1/health`、`/v1/resolve-prefix` 等） |
| ArangoDB | Docker 容器（不暴露宿主端口） | 图存储 |
| `skills/ix` | Markdown | 命令参考作为 agent skill 安装进 harness |
| `bench/agent-eval` | — | agent 消费面的基准评测 |

命令面四组：Build（map/watch/reset/verify/doctor）、Understand（search/locate/explain/overview/impact/read）、Explore（trace/callers/callees/depends/imports/rank/inventory/history/diff）、Inspect（view/subsystems/stats/smells）。

Agent 集成三面：`--format llm`（token-minimal 记录流）、`ix mcp`（stdio MCP server + 7 家客户端注册）、`skills/ix`（文档即接口）。

---

## 2. 它验证了我们哪些方向

1. **持久图 + 快照**：Ix 的 "Remember"（图跨 session 持久）与我们 SQLite 快照主干同构。它的卖点表（"Architecture: Re-derived every session → Mapped once, kept"）就是观测主干的营销版。
2. **有界结构切片**："Bounded structural slices, not whole files" 正是 `oac context` / core 查询层的价值主张。
3. **可读稳定 ID**：llm-format 明确规定 opaque id 截到 8 字符，但 "a slug like `root`, a prefixed id like `c-8f31a2`" 保持完整——与我们 `sys:`/`ctn:`/`cmp:` 可读稳定 ID 的选择一致。
4. **只读图查询工具喂 agent**：A0 分析 agent 把 impact/neighbors/search 注册成只读工具，与 Ix 的 MCP 工具面完全同构。
5. **架构自动映射**：`ix subsystems` 保存的 "architectural map"（region/level 分层）与我们的 Container/Component 层级同题；他们也是启发式聚类，印证 §6.3 映射规则必然迭代的风险判断。
6. **实体级 history/diff**：`ix history <target>` / `ix diff <from> <to>` 证明快照之上的实体级历史有真实需求（对应切片 2）。
7. **agent 评测基准**：`bench/agent-eval` 目录的存在，佐证切片 7 的 TaskSource + 机械评分路线值得做。

---

## 3. 值得直接借鉴（按优先级）

### 3.1 `--format llm` 输出规范 → 进 oac CLI spec（切片 1 CLI 定稿前）
- 三档 `--format text|json|llm`；llm 档 newline-delimited `key=value` 记录，无嵌套无装饰，比 json 省 2–4× 字节。
- 空值/零值字段直接省略；含空格/`=`/引号的值才加引号转义。
- **统一错误记录** `error code=<slug> message="..."`，与数据行同格式，进程仍非零退出。
- **显式截断信号**：`truncated ...` 记录 + shown/total 计数——agent 必须知道有东西被切了。
- 默认值解析顺序：`IX_FORMAT` env → 配置文件 → `text`；**故意不按 tty 自动切换**（会让解析 text 的脚本下次突然收到记录流）；非法值忽略 + stderr 提示而非 fatal。
- 结论：`oac --json` 之外应加 `--format llm`（或等价档），切片 7 A0 的工具返回直接受益。

### 3.2 退出码纪律 + ambiguity 语义 → 进 oac CLI spec
- 图命令统一退出码：`0` 有结果 / `1` 错误 / `2` 目标歧义 / `3` 目标不存在 / `4` workspace 未 map。
- 血泪教训（CONTRIBUTING 原文）：**给已有命令加非零退出码是对所有 plugin/MCP 客户端的 breaking change**——调用方在失败分支丢弃 stdout，结构化 payload 反而永远看不到。改退出码前必须先 grep 全部程序化调用点，且下游改动先落地。
- 歧义处理：多候选 → exit 2 + 候选列表；8 字符前缀可经 `/v1/resolve-prefix` 回解析。我们切片 2 跨仓后同名元素必然歧义，这套语义可直接对齐。

### 3.3 图新鲜度校验（verify/doctor）→ 补强 I2 观测闭环
- `ix verify`：逐文件比对「图中节点 vs 重新解析」+「存储 source hash vs 磁盘 hash」，三值裁决 `clean / stale / diverged`，外加 `unverified` 及 reason 枚举；输出带 `fix="ix reset ..."` 的**修复建议字段**。
- `ix status` 缓存最近一次 verify 的记录（`verified_at`/`verified_files`/…），status 只读记录、绝不跑 verify。
- 对我们的意义：切片 7 I2「rollout 后自动重扫复查」需要一个「当前快照是否新鲜」的机械信号；GUI 状态行也应显示快照新鲜度。schema 上就是**每文件存 source hash**——这一列建议切片 2 就加，同时服务增量扫描。

### 3.4 MCP server 面 → 切片 7 A0 顺路交付
- covonaut 协议面自带 MCP client/server；A0 可低成本附带 `oac mcp`（stdio），把 impact/neighbors/search 暴露成只读工具，让 Claude Code/Codex 直接消费架构图。
- 借鉴其客户端注册策略：能走客户端自己的命令就走（`claude mcp add` 等），否则直接编辑配置文件且 **merge in place + 留 .bak**；名字被占**绝不覆盖**；`doctor` 检测 launcher 路径漂移并可修复。

### 3.5 命令词汇对齐 → oac 命令面设计成本归零
- `explain / impact / trace / callers / callees / depends / imports / rank / inventory / stats / smells` 的命名与语义已经打磨过，oac 直接对齐。
- `rank --by dependents`（load-bearing code）和 `smells`（结构问题）是 graph 层低成本高价值的查询，切片 1/2 的 graph 包可顺手实现。

### 3.6 实体级历史 → 切片 2 schema 预留
- 快照 diff 之外，物化 element 级跨快照变更记录，支持 `oac history <element>`（该元素何时出现/改边/消失）。

### 3.7 切片 5 adapter 策略修正
- Ix 用 tree-sitter（core-ingestion，独立 parser library）覆盖 27 语言的 symbols/calls/imports 抽取——证明「通用 parser sidecar」路线工程上成立。切片 5 的 TS/Python/Java/Rust adapter 应优先做**一个 tree-sitter sidecar**（Node 或 web-tree-sitter，进程隔离，不破 no-cgo），而不是每种语言手写 adapter。

### 3.8 文档即 agent 接口
- Ix 把完整命令参考放 `skills/ix/references/*.md` 并有一键安装进各家 harness 的脚本。oac 稳定后同样应随仓库带 skill 目录（成本极低，杠杆大）。

---

## 4. 不该学的

| Ix 的做法 | 为什么不跟 |
|---|---|
| Docker + ArangoDB + 本地 HTTP backend（:8090） | 破我们的 local-first / 单二进制 / 零外部运行时。Ix 的重量来自多仓共享后端与任意深度图遍历；OPC 3–10 仓规模下 SQLite + 内存图 + N 跳集合运算足够 |
| 核心 memory layer 闭源（私有仓库），OSS 壳 + Pro 边界 | 与 MIT 全开源承诺冲突；用户无法审计核心 |
| Node/TS 双 npm 包运行时 | 桌面分发体积与更新链复杂度；我们的 sidecar 只在切片 5 按需引入 |
| 可视化仅「打开 web visualizer」 | 我们 onevra flow 原生分层下钻 + 相机持久化是差异化，不降级 |

---

## 5. 建议落点（谁改哪份 spec）

1. **切片 1 plan/spec**（oac CLI 定稿前）：补三档输出格式与退出码表（§3.1、§3.2）。
2. **切片 2 spec**：schema 加每文件 source hash 列（增量 + verify 两用）；element 级变更记录表（history）。
3. **切片 7 spec A0**：工具输出采用 llm 记录格式 + 截断信号；可选 `oac mcp` 只读工具面；rollout 后复查引入 verify 式新鲜度裁决。
4. **切片 5 spec**：adapter 策略改为单一 tree-sitter sidecar 优先。
5. 新增小命令候选：`oac verify`（快照 vs 磁盘一致性，I2 闭环的机械检查面）。

---

## 6. 落点状态（2026-10-26，切片 1 合入 main 后回填）

切片 1 已合入 `main`（merge commit `9ae93f0`），`oac` CLI 定稿为
`text` + `--json` 两档、退出码只有 0/1。据此修正 §3.1 的「切片 1 CLI
定稿前」口径——以下条目**全部顺延，不在切片 1 补**：

| 条目 | 落点 | 状态与理由 |
|---|---|---|
| §3.1 `--format llm` | 切片 7 A0 | 切片 1 已定稿不动。llm 档的首个消费方就是 A0 的工具返回，届时一并设计省得空转 |
| §3.2 退出码 2/3/4（歧义/缺失/未 map） | 切片 7 A0 起引入 | 正是 Ix 自己那条血泪教训（加非零退出码对程序化调用方是 breaking change）：切片 1 的调用方是人，0/1 够用；等程序化消费者出现时一次性定码表 |
| §3.3 每文件 source hash | 切片 2 schema | 写切片 2 spec 时进 `arch_files`（或等价表）；同时服务增量扫描与 `oac verify` |
| §3.6 element 级历史表 | 切片 2 schema 预留 | 同上，spec 里先立表不实现也可 |
| §3.4 `oac mcp` + 注册策略 | 切片 7 A0 | 顺路交付 |
| §3.5 命令词汇对齐 + `rank`/`smells` | 切片 2 graph 层 | `internal/core/graph` 顺手加，成本一轮 BFS 级别 |
| §3.7 tree-sitter sidecar | 切片 5 spec | 写 spec 时按单 sidecar 路线 |
| §3.8 skill 目录 | 切片 7（oac 命令面稳定后） | 一条目录 + 一份 reference |

