# OpenArchitect 项目设定（PM × 架构师双视角基线）

- 状态：基线草案（2026-11-10 生长模型愿景合入后的第一次全项目收拢）
- 性质：**设定文档**——不新开设计，只把散在 4 份 spec、3 份 PoC、4 份调研里的
  既定事实收拢成一份可引用的项目基线。与任何 spec 冲突时，以 spec 为准并回改本文。

---

# 第一部分：产品经理视角

## 1. 一句话定位（两代演进）

- **v1 定位**（切片 1 spec §1，源自立项文档）：面向**一人公司（OPC）**的开源桌面
  工具——自动扫描多个关联项目的代码仓库，维护跨项目统一架构视图；沿关系图做
  影响分析，回答"改这里会坏哪里"。
- **v2 升级**（2026-11-10 生长模型）：v1 回答的是**空间**问题（哪里连着哪里）；
  v2 加上**时间**维度——仓库自动发现、git 自动检入、架构随 commit 自动成长，
  并观测**智能体工作动态**：智能体迭代软件时，树在哪里动，就在哪里结果子。

一句话 v2：**多软件项目的架构观测 + 智能体工作动态观测的终极方案——
一片持续生长的森林，人看到的是树冠，果实标出智能体动过的地方。**

## 2. 目标用户与场景

- 画像：一人同时维护 3~10 个有上下游关系的仓库（现在：Go；预留多语言），
  代码托管在自建 Gitea（兼 GitHub），日常有 coding agent（claude/codex/opencode）
  代为迭代代码。没有专职架构师，也没有团队看板。
- 核心痛点三个，按锐度排序：
  1. **全景图无法外化**——脑中的系统地图随多仓演化失真，"改这里会坏哪里"靠回忆；
  2. **智能体成了改码主力，但它的活动是暗物质**——commit 混在历史里，
     哪个模块被 agent 反复动、动了什么，没有任何工具能回答；
  3. **架构是静态快照，而软件是生长过程**——手动扫描永远滞后于演化，
     看不出"哪里在动、哪里停摆"。

## 3. 产品支柱

| # | 支柱 | 回答的问题 | 现状 |
|---|---|---|---|
| P1 | **森林观测** | 我有哪些仓、漂没漂？ | 多仓合并视图已成立；自动发现/检入=生长切片 A/D |
| P2 | **生长观测** | 架构哪里在动、哪里停摆？ | 快照时序有、diff 引擎无=生长切片 B |
| P3 | **果实观测** | 哪个 agent 在哪个节点动了什么？ | 全缺，但数据源（trailer）现成=生长切片 C |
| P4 | **影响分析** | 改这里会坏哪里？ | ✅ 已交付（BFS/impact，GUI 与 CLI 同一集合） |

辅助面：headless CLI（`oac scan/graph/impact/context --json`）是对 agent 世界的
**契约面**——将来出 MCP server 供外部 agent 消费架构数据，走同一条路。

## 4. 明确不做（非目标，仍然有效）

不是协作文档平台 · 不是任务执行引擎 · 不是企业架构治理工具 · 不是通用代码搜索。
边界补充：本产品的智能体**只观测、不集成**——归因纯读 git 历史，不要求任何
agent 框架配合；切片 7 的 agent 执行子系统是独立边界，主干对它一无所知。

## 5. 差异化（对照 repowise，调研 2026-11-02 裁决）

repowise 是「一次索引 → 五个智能层 → CLI/MCP/dashboard」，本质**为 agent 建索引**，
Python monorepo + Next.js，偏服务端部署。我们的分野：

| 维度 | repowise | OpenArchitect |
|---|---|---|
| 交付形态 | 服务端 + web dashboard | 单二进制桌面 app，local-first，离线可用 |
| 主用户 | agent（索引消费者） | **人**（森林的园丁），agent 是观测对象而非客户 |
| 时间维度 | churn/hotspot 统计 | **生长模型**：commit 级架构 diff + 归因果实 + 时间轴回放 |
| 评分依据 | 53 detector 投票 | **图不变量**（我们有可执行的架构事实，不需要 rubric） |
| 许可 | AGPL | MIT，无厂商锁定 |

它有的（co-change、AI attribution）我们作为生长模型的子集吸收；它没有的
（跨仓合并图的交互、生长视图）正是我们的主场。

## 6. 成功信号

- 北极星：**用户不再在脑中维护系统全景图**（反馈语言从"我记得"变成"图上说"）。
- 可测信号：多仓常开率（≥3 仓且每周打开）；果实点击率（agent 归因事件被查看）；
  `oac context` 被外部 agent 注入上下文的次数。

## 7. 路线图

| 切片 | 内容 | 状态 |
|---|---|---|
| 1 | 单仓扫描→SQLite→flow 图→影响分析→headless CLI | ✅ 已合入 main |
| 2 | 多仓合并图 + 增量扫描 + 快照对比 + 元素历史 + File 元素 | 计划已立 |
| 生长 A | forest-local：本地自动发现 + git 检入 + 快照绑 commit | 设定完成 |
| 生长 B | growth：跨快照 element diff → GrowthLog | 设定完成 |
| 生长 C | fruit：commit 归因（human/agent/mixed） | 设定完成 |
| 生长 D | 远端源：Gitea 先行、GitHub 次之（Source 接口） | 设定完成 |
| 生长 E | 生长视图：treemap 树冠 + 果实角标 + 时间轴 | 设定完成 |
| 7 | agent 子系统（分析 agent 为主，变更 rollout 为次） | 草案，独立边界 |

---

# 第二部分：架构师视角

## 8. 原则与不变量（全部有 CI 机械检查）

六条核心原则（切片 1 spec §1）：观测为主不执行 · 本地优先 · 单用户优先 ·
多语言覆盖（现仅 Go adapter）· 松耦合协作 · 开源 MIT。

三条不变量：
- **I1 单向依赖**：`internal/agent/**` 可 import `internal/core/**`；反向永不。
- **I2′ 自动的只有观测**（2026-11-10 修订）：定时器/自动触发仅准调
  `git fetch` 与 `scanner.Run`，皆记账进 sync ledger；一切执行皆用户发起。
- **I3 模型访问两条通道**：进程内 OpenAI-compatible / 本地 CLI 子进程；
  不捆绑厂商 SDK，不破 no-cgo 交叉编译。

CI 守门：core 不 import UI；驱动只准出现在 store；agent 不入主干依赖；
forest 不 import agent/net/http（source 实现包除外）。

## 9. 分层与依赖规则

```
cmd/oac, cmd/oac-agent          # headless 契约面（--json 按对外契约对待）
internal/ui/                    # mygo 原生 UI：sidebar 壳 + flow + treemap
internal/forest/  ──┐           # 发现·检入·ledger（生长 A/D）
internal/growth/  ──┼──→ core   # diff·归因·GrowthLog（生长 B/C）
internal/scanner/ ──┤           # LanguageAdapter → elements/relationships
internal/agent/    ─┘           # 独立边界子系统（切片 7）
internal/core/{model,store,graph}  # 主干：数据结构·SQLite·图算法，零 UI 依赖
```

## 10. 数据模型（v1 现状 + v2 生长扩展）

- v1（已落库）：`repos`、`architecture_snapshots`（commit_hash/branch 列**预留恒空**
  ——生长模型的接口）、`arch_elements`（稳定 ID `cmp:/ctn:/fil:`）、
  `arch_relationships`（contains/depends_on/calls）、`app_config`。
- v2（生长模型）：`repos + remote_url/sync_state`；`commits`（含 agent_tag）；
  `growth_events`（snapshot 对 × element × kind × commit）。
- 面积口径已裁决：treemap 面积单位=**File 元素**（PoC-13：元素数口径 41~64%
  零面积不可接受）。

## 11. 关键技术选择（全部经实测核实）

| 选择 | 理由与证据 |
|---|---|
| egoist/mygo 原生 UI（Metal 自绘，无 cgo） | 单二进制全平台交叉编译；1000 label 397µs/帧 |
| onevra flow + treemap（fork pin 版本） | flow 600 节点 3.4ms/帧；treemap 6000 tile 1.19ms/帧（PoC-12） |
| purego SQLite（mygo 官方插件） | 无 cgo 的真 SQLite，单事务快照写入 |
| `go/packages` + `go/types` | 类型级事实而非启发式；单仓全量实测 1.95s |
| git 一律 `os/exec` 调 CLI | 与 I3 同风格：无 SDK、无 go-git 重依赖 |
| 归因=trailer 解析，规则表 TOML | 不依赖 agent 子系统；新工具零改码接入 |

## 12. 演进钩子（已埋的接口）

1. `Snapshot.CommitHash/Branch`——生长模型 A 的落点；
2. `Graph.Merge` 签名——多仓合并（切片 2）；
3. `LanguageAdapter` 注册表——多语言；
4. `Source` 接口——Gitea/GitHub 只是发现源，观测管线唯一；
5. CLI `--json` 输出——将来 MCP server 同一契约。

## 13. 性能预算与风险登记

- 预算：出图 60s 内（DoD）；treemap 重扫首帧关动画（调研 §9-4）。
- 风险：①多仓×全量扫描算力——sync 先 `git diff --name-only` 判触及，
  不触及只记 commit（内建于切片 A，不许后补）；②归因误判——trailer 可伪造，
  定位为强提示非裁决，UI 用形状区分置信度；③上游 pre-1.0 依赖
  （mygo/onevra/covonaut）——pin 版本+单包隔离+窄接口三件套；
  ④Gitea API 面按 ≥1.20 验证，更老实例降级手动 clone。

