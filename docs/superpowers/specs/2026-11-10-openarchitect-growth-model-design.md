# OpenArchitect 生长模型设计（切片 3′/4′/5′ + 远端源）：自动发现 · 架构成长 · 智能体果实

- 状态：**草案**（待用户逐段确认）
- 日期：2026-11-10
- 上游：切片 1 已合入 `main`；切片 2 计划已立（`plans/2026-10-30-openarchitect-slice2.md`）。
  本设计消费三处预留钩子：
  ① `model.ArchitectureSnapshot.CommitHash/Branch`——`snapshot.go:8` 注释明写
  "stay empty in this slice (no Git)"，即为本设计留的接口；
  ② 切片 7 草案（`2026-10-23-openarchitect-agent-harness-design.md`）§1 不变量 I2
  的末句「agent 的产出与人类提交在快照层一视同仁」——果实归因即兑现此句；
  ③ treemap PoC（`poc/2026-11-02-treemap-poc.md`）PoC-12/13 已过规模与面积两道门槛，
  面积口径裁决落 `File` 元素。

---

## 1. 一句话与隐喻

**把"手动扫一个快照"升级为"持续观测一片森林的生长"**：仓库自动发现、git 自动检入，
架构随 commit 自动成长；智能体迭代软件时，树在哪里动，就在哪里结果子。

| 隐喻 | 工程含义 | 落点 |
|---|---|---|
| 森林 | 多仓并存 + 合并视图（已有）+ 自动发现与跟踪 | `internal/forest/` |
| 树的生长 | 快照绑定 commit → 跨快照 element 级 diff → 生长日志 | `internal/growth/` |
| 果实 | agent 归因的变更事件，挂在被改的 element 节点上 | 归因器 + UI 角标 |
| 树冠 | treemap 面积=规模、色相=变化新旧、角标=果实 | UI 生长视图 |

第一原则「观测为主，不执行」不变。本设计只扩大观测的**触发面**（自动），不触碰**执行面**。

## 2. 不变量修订

### 2.1 I2 修订（同步改切片 7 spec §1 措辞）

原文：「一切执行皆用户发起、皆记账：没有定时器、没有自动触发。」
修订为：

> **I2′ 自动的只有观测，一切执行皆用户发起、皆记账**：定时器与自动触发仅允许
> 调用两个纯观测动作——`git fetch` 与 `scanner.Run`；两者皆记账进 forest 的
> sync ledger。agent rollout、模型调用、任何写外部系统的动作仍必须用户显式发起。

主干（scan / store / graph）语义与依赖规则一字不改；I1（单向依赖）原样保留。

### 2.2 新守卫（CI 第三条之后追加）

```sh
# forest 只做观测：不 import agent，不 import HTTP 客户端
# （giteasource/githubsource 两个 source 实现包除外，见 §5.3 隔离先例）
go list -deps ./internal/forest/... | grep -v 'internal/forest/\(gitea\|github\)source' \
  | grep -x 'github.com/futuremeng/OpenArchitect/internal/agent/.*\|net/http' && exit 1 || true
```

## 3. 已核实事实（2026-11-10，读源码，非转述）

| 事实 | 证据 |
|---|---|
| `CommitHash/Branch` 字段与 `commit_hash/branch` 列已存在且恒空 | `model/snapshot.go:13-14`；`store/schema.go` v1 DDL；`store/snapshot.go:27,83` |
| `model.Repo` 曾**有意裁掉** URL/Branch（切片 1 偏差 9）——本设计以 `remote_url + sync_state` 恢复，登记为偏差 D2 | `model/repo.go:3-4` 注释 |
| `UpsertRepo` 按 `RepoIDFor(abs)` 幂等，forest 反复发现零成本 | `store/repo.go:10-19` |
| 多仓合并视图已成立：`Settings.GraphRepos` 空=全选，新仓自动进图 | `config/config.go:22-26` |
| schema 现为 v1，`migrations[i]` 机制就绪，v2 归本设计 | `store/schema.go:5` |
| 扫描是单仓全量 `packages.Load`，onevra 实测 1.95 s——大仓多仓定时重扫必须先做「值不值得扫」判定（§6.3） | 切片 2 spec §2 事实表 |
| Gitea REST API 在 `/api/v1`（自带 GitHub 兼容子集），token 用户自持；clone 走纯 git URL，无需 SDK | 用户自述在用 Gitea；Gitea API 文档 |
| claude/codex/opencode 的 commit 带 `Co-authored-by:` trailer，归因数据源在 git 历史里，**不需要 agent 子系统配合** | 切片 7 spec §1 通道约定 |

## 4. 切片与顺序

```
A forest-local ──→ B growth ──→ C fruit(归因) ──→ E 生长视图
      └→ D 远端源（gitea 先行，github 次之；只依赖 A 的 Source 接口）
```

### 切片 A：forest-local（自动发现 + git 自动检入）

- 新包 `internal/forest/`：
  - `discover.go`：遍历 `Settings.ScanRoot`，凡含 `.git` 的目录 `UpsertRepo` 登记；
    三态 `new / tracked / stale`（失联=登记路径上 `.git` 消失）。
  - `sync.go`：对每个 tracked 仓 `git rev-parse HEAD`（`os/exec`，与 I3 无 SDK 风格一致）；
    与最近快照 `CommitHash` 比对，**变了才** `git fetch` + `scanner.Run`，并把
    HEAD/branch 写进新快照。
  - `ledger.go`：每次 sync 动作一行 JSONL（repo、动作、前后 hash、耗时、错误），
    落 userData 目录。GUI「最近同步」活动流读它。
- 触发：App 启动一次 + 手动「同步全部」+ 可选 ticker（默认关，`forest.autoscan` 配置键）。
- 远端 URL 恢复：`repos` 表 v2 迁移加 `remote_url TEXT NOT NULL DEFAULT ''` 与
  `sync_state TEXT NOT NULL DEFAULT 'new'`；`git remote get-url origin` 采集。

### 切片 B：growth（架构自动成长）

- 新包 `internal/growth/`：同仓相邻快照 element 级 diff（按稳定 ID
  `cmp:/ctn:/fil:` 对齐），产出 `GrowthLog`：added / removed / signature-changed /
  edge-changed，粒度到 element。
- v2 迁移加两表：`commits(repo_id, hash, author, author_email, date, message, agent_tag)`
  与 `growth_events(snapshot_from, snapshot_to, element_id, kind, commit_hash)`。
- commits 采集用 `git log <old>..<new> --format=...`，一次 sync 一批入库。

### 切片 C：fruit（智能体动态归因）

- `internal/growth/attribute.go`：解析 commit author + `Co-authored-by:` trailer →
  `agent_tag ∈ {human, agent:<name>, mixed}`。规则表是数据不是代码（TOML），
  新 agent 工具零改码接入。
- 查询面：`Fruits(elementID)` 返回该节点全部 agent 归因事件（agent、commit、date、
  growth_events 摘要）。归因纯读 git 历史，与切片 7 agent 子系统**零耦合**。

### 切片 D：远端源（Gitea 先行）

- `internal/forest/source.go`：`Source` 接口 = `ListRepos(ctx) ([]RemoteRepo, error)`
  （name、clone URL、default branch、pushed_at）。
- `internal/forest/giteasource/`：**第一个实现**。配置 = base URL + token（用户自持，
  local-first 不破）；调 `/api/v1/user/repos` 或 org 列表；`pushed_at` 增量过滤。
- `internal/forest/githubsource/`：第二个实现，同一接口。
- 落仓：缺失仓 `git clone` 到 `ScanRoot/<owner>/<name>`，此后与切片 A 的 tracked 仓
  走完全相同的 sync 管线——远端只是发现源，不是第二套观测。
- 隔离先例（仿 analysis 包「唯一允许 import X」）：HTTP 客户端只准出现在两个
  source 实现包内。

### 切片 E：生长视图（UI）

- treemap=树冠：面积=`File` 元素规模（PoC-13 裁决）；色相/饱和度=距最近一次
  growth_event 的新旧；果实角标=agent 归因事件数，点开节点卡展开果实列表。
- flow 页节点卡加「生长」区：最近 N 条 growth_events + 果实。
- 生长时间轴：拖动回放 `growth_events` 的 commit 序。重扫后首帧关动画
  （treemap 调研 §9-4 已入清单）。

## 5. 目录与依赖

```
internal/forest/{discover.go,sync.go,ledger.go,source.go,gitcmd/}
internal/forest/giteasource/    # 唯一允许 net/http 的远端包之一
internal/forest/githubsource/   # 同上
internal/growth/{diff.go,attribute.go,log.go}
```

依赖规则：`forest → core/store + core/model + scanner + os/exec`；`growth → core/*`；
`core/scanner` 永不 import `forest/growth`（I1 的自然延伸，CI 守门 §2.2）。

## 6. 偏差与风险

1. **D1**：本设计使切片 7 spec 的 I2 措辞修订（§2.1），需同步改该文件一行。
2. **D2**：恢复切片 1 裁掉的 Repo URL 字段（偏差 9 反转一半：只恢复 remote_url，
   Branch 仍不进 Repo——branch 属于快照，不属于仓）。
3. **算力**：多仓 × 全量扫描会疼。缓解内建于切片 A：sync 先 `git diff --name-only
   <old>..<new>`，不触及受支持语言源文件则**跳过重扫只记 commit**（growth_events
   相应为空，commits 照记）。这条不许后补。
4. **归因误判**：trailer 可被伪造/缺失，`agent_tag` 定位为「强提示」而非事实裁决；
   UI 上 human/mixed 与 agent 用不同角标形状区分置信度。
5. **Gitea 版本面**：API 面按 ≥1.20 验证；更老实例降级为「手动 clone + 切片 A 跟踪」。
