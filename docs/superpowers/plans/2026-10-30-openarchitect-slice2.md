# OpenArchitect 切片 2 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把「一个仓库的图」升级成「一人公司的图」：每文件 SHA-256 与 Merkle 包指纹让扫描可增量，每次扫描在同一事务里留下元素级历史，两个快照能 diff、能在图上着色，并有 `oac repo|diff|history|rank|verify` 五个新命令。

**Architecture:** 仍是三层。`internal/core`（model/store/graph）与 `internal/scanner`（adapter/fingerprint/verify/reuse）不 import 渲染栈；`internal/ui` 只做展示与事件；`cmd/oac` 与 `cmd/openarchitect` 只做装配。切片 2 新增的第四块是**扫描提交协议**：`store.CommitScan` 一次事务写「快照 + 元素 + 关系 + 文件 hash + 包指纹 + 历史事件 + 封顶裁剪」，CLI 与 GUI 共用同一个 `scanner.Run`，因此共用同一份提交语义。

**Tech Stack:** Go 1.27.2（darwin/arm64）、`github.com/egoist/mygo v0.5.5`（含 `plugins/sqlite`，purego 驱动）、`github.com/oneo-me/onevra/source`（经 `replace` 指向 `github.com/futuremeng/onevra/source v0.2026.13-0.20261028081121-a3bbb6f230ab`，即本地 fork 的 `gis-navigation` 分支）、`golang.org/x/tools v0.52.0/go/packages`、`go/types`、`crypto/sha256`、`encoding/json`。

**Spec:** `docs/superpowers/specs/2026-10-26-openarchitect-slice2-design.md`

---

## 前置：main 上已落地的切片 2 内容（2026-10-30 盘点，逐条给证据）

设计稿写于 2026-10-26，main 在其后续了 14 个 commit（`48ce2b4`…`f865f47`）。**本计划不是 §3.1 的十项清单**，而是「十项 − 已落地 + 实测倒逼的修订」。已核实落地的部分：

| spec 条目 | 状态 | 证据（main，2026-10-30） |
|---|---|---|
| §3.1-1 多仓管理（GUI 过滤器 + 逐仓扫描） | **部分落地**：逐仓扫描按钮与「查看」有，**添加/删除/全部重扫/新鲜度徽标无** | `internal/ui/page_repos.go:41-70` |
| §3.1-2 合并图 | **落地在 App 层**：`Reload` 把选中仓的最新快照并成 `a.graph`；`Graph.Merge` 仍返回 `ErrNotImplemented` | `internal/ui/app.go:74-115`、`internal/core/graph/graph.go:112` |
| §3.1-3 扫描器跨仓边持久化 | **已落地**：`componentIDFor` 返回 `(id, external, ok)`，非本模块的导出具名类型作为悬空端点存库；fixture `testdata/fixtures/crossmod-app`+`crossmod-dep` 已带 `replace` | `internal/scanner/language/go.go:318-340`、fixture `go.mod` |
| §3.1-9 minimap | **已落地且超过设计**：fork 的 `flow.NavigationMap` 提供点阵 + 视口矩形 + 点击导航 | `internal/ui/page_graph.go` 的 `Navigation: flow.NavigationMap` |
| §9 `graph/impact` 的仓库选择 | **GUI 侧落地**（chips + `graph.repos` 持久化），**CLI 侧 `--all/--repo` 无** | `internal/ui/page_graph.go:95-127`、`cmd/oac/main.go:34-44` |
| §10.4 相机/层级记忆 | **落地**：深度锚点、LOD 三档、scale 通道、键盘（Esc/F/⌘=/⌘−） | `internal/ui/state.go`、`docs/superpowers/research/2026-10-27-graph-interaction-gap-review.md` §6-7 |

**完全未落地**（本计划的全部工作量）：schema v2、`arch_files`、`pkg_fingerprints`、`element_history`、增量扫描与平价测试、快照封顶、`DiffSnapshots`、`rank`、`oac repo|diff|history|rank|verify`、`--json`、`--all/--repo`、GUI 对比着色、抽屉历史段、仓库页四项升级、PoC-8/9/11 的正式执行记录。

---

## 本计划对 spec 的定稿与修订

spec 是权威，本节是「spec 与 2026-10-30 的 main 冲突时怎么裁」。每条都给证据与代价，执行者不要再自行发明。

1. **`Graph.Merge` 定稿为 `func Merge(graphs []*Graph) (*Graph, []string)`**（N 路并集 + 归属冲突 warning 列表），并删除切片 1 留下的 `func (g *Graph) Merge(other *Graph) error` 与 `ErrNotImplemented`。理由：(a) CLI 的 `--all` 与 GUI 的合并视图必须走同一条并集路径，否则 DoD-4（GUI/CLI 一致）没有机制保证；(b) warnings 得有出口，挂在 `Graph` 上会随 `Aggregate` 丢失。这是 spec §7 自己预告的「有意修订」的落地形态。代价：两处调用方（`App.Reload`、`cmd/oac`）改形。
2. **spec §2 的两条「已核实事实」已过时，按实测修订**：`flow.State` 现在暴露 `Camera() (x, y, zoom float32)`、`SetCamera`、`Hit(x, y)`、`Hovered()`；`Options` 暴露 `Navigation NavigationMode`、`OnZoom`、`OnZoomAt`。证据：`$GOMODCACHE/github.com/futuremeng/onevra/source@v0.2026.13-0.20261028081121-a3bbb6f230ab/components/flow/flow.go`（`go.mod:22` 的 `replace` 就指向它）。因此 spec §13 的「不私改依赖」在本切片**已被用户的 fork 决策取代**（2026-10-28 起 `go.mod` 带 replace），本计划不再向 onevra 提 issue，也不引入新的上游改动。
3. **PoC-10 判定为已完成，不再排任务**。spec §3.1-9 设想的「自绘点阵 + 点击 Reveal」被 fork 的 `NavigationMap` 取代，且视口矩形（spec 明言「现 API 下不可得」）已经在里面。§12 表里 PoC-10 那一行在 findings 里记「discharged by fork NavigationMap」即可。
4. **U1 = 不加跨仓总根**。仓库层的根是 `RepoGroupID(repoID)` = `sys:repo:<abs path>`，与元素层的 `model.SystemID(modulePath)` = `sys:<modulePath>` 同前缀不同串（`internal/core/graph/aggregate.go`、`internal/core/model/ids.go:8`），合并图是森林，LTR 布局切片 1 已实测能画（PoC-4）。
5. **U2 = 删仓级联删历史**，`store.DeleteRepo` 只发一条 `DELETE FROM repos WHERE id = ?`，`architecture_snapshots`→`arch_elements`/`arch_relationships`/`arch_files`/`pkg_fingerprints`/`element_history` 全由 v1/v2 的 `ON DELETE CASCADE` 收走（`PRAGMA foreign_keys = ON` 已在 `store.Open` 打开）。
6. **U3 = 幽灵节点参与布局**。对比模式限定「单仓两快照」，节点数等于单仓规模（切片 1 实测 300 节点 Fit≈0.13x 仍可交互），缓冲是 §10 的仓库过滤器与 LOD。代价：若 PoC-11 显示帧时间崩，降级分支是「幽灵退出布局、只进对比摘要行」，在 T25 里作为一条明文分支执行，不是临时决定。
7. **U4 = rank 用直接入度，且只算 `calls` + `depends_on`**。`contains` 不计入——否则每个 Component 都从所属 Container 白拿 1 分，排序退化成元素类型分组。观察库基线（`poc/2026-10-26-observation-corpus.md`：OpenLaw `store.DB` 入度 20 全库最高）就是按这个口径算的，测试预算按它写。
8. **增量复用的归属键 = `from_id` 所属包**，用现成的 `model.PkgPathOf(id)`；返回空串的行（唯一的 `sys:<modulePath>` 元素与它的 `contains` 边）**永远重发**。理由：System 元素只有一行、它的 `description` 来自根包文档，复用与重发结果相同，为它开一个桶不值得。这条决定了 T13 的实现形状。
9. **`oac verify` 输出三清单、两值裁决**：`changed` / `missing` / `untracked`，`stale` ⟺ 任一非空。`untracked` 会把「因 build tag 没进任何已加载包的文件」也报出来——那不是误报，图里确实没有它；文案必须这样写，别把它藏起来（spec §6.4 的「诚实比三值好看更重要」在此延伸）。
10. **CLI 形状重写**：`subcommand{arg string, flags func(*flag.FlagSet), run func(ctx, db, options, out) error}`，`run()` 里那句全局「恰好一个位置参数」下沉为各命令自己的 `wantOne`。三条切片 1 纪律必须原样存活：F4（先验表后开库）、F3（`ContinueOnError` + `fs.SetOutput(io.Discard)`）、F2（`hoistFlags` 从 `FlagSet` 自己判断值 flag，新增 `--repo` 这类值 flag 无需改 `hoistFlags`）。
11. **`oac context` 不在本切片**。spec §9 表尾「`graph/impact/context` 增加 `--all`」里的 `context` 是切片 7 的命令（Ix §6 与退出码表一起决定），本切片只给已存在的 `graph`/`impact` 加 `--all`/`--repo`。
12. **历史读取在 `Select` 里同步做**（5 行查询）。先例是 `Select` 已经同步调 `analyse()`（`internal/ui/state.go`）；为 5 行查询开一条异步通道是多余的状态机。查询失败留空，不加状态字段（先例：`filePathRow` 丢弃 `OpenPath` 错误）。
13. **对比着色走 `nodeCard` 的 mark 参数**：fork 的 `flow.Node` 只有 `Muted` 与 `Content` 两个样式通道（`components/flow/flow.go:37-42`），没有描边字段，所以「added 绿描边 / changed 琥珀描边 / removed 虚线」只能在自绘卡片里画。
14. **`store.SaveSnapshot` 被 `store.CommitScan` 取代**。spec §8 要求「快照、hash、指纹、历史四者原子」，切片 1 的 `SaveSnapshot(ctx, s, elements, rels)` 签名装不下这三段。调用点实测 9 处：`internal/scanner/scanner.go:46` 一处产品代码，`internal/core/store/snapshot_test.go` 七处（`:47,116,146,179,242,278,301`）加 `bench_test.go:42` 一处基准。

---

## Global Constraints

每个任务的隐含需求都包含本节，数值逐字来自 spec。

- Go 版本下限 **1.27**；开发机实测 `go1.27.2 darwin/arm64`。Go 不在 PATH 上：每条命令前 `export PATH="/usr/local/go/bin:$PATH"`。**本计划不修改用户的 `~/.zshrc`/`~/.zprofile`**。
- 模块代理固定 `export GOPROXY=https://goproxy.cn,direct`。
- 依赖精确 pin：`github.com/egoist/mygo v0.5.5`、`golang.org/x/tools v0.52.0`、`onevra/source`（经 `replace` 到 fork 伪版本 `v0.2026.13-0.20261028081121-a3bbb6f230ab`）。业务依赖**禁止 `@latest`**；本切片**不新增任何第三方依赖**（`crypto/sha256`、`encoding/json`、`path/filepath` 都是标准库）。
- 全程 `CGO_ENABLED=0`，禁止 cgo / tree-sitter 绑定。唯一例外：跑 `-race` 那一遍要 `CGO_ENABLED=1`（检测器本身要求 cgo，依赖图里没有一行 cgo）。CI 里没有 race 步骤，race 是本地闸门。
- **R1**：`internal/core/**` 与 `internal/scanner/**` 不得 import `github.com/egoist/mygo`（唯一例外 `github.com/egoist/mygo/plugins/sqlite`，且只允许出现在 `internal/core/store`）、`github.com/oneo-me/onevra/**`。`cmd/oac` 不得 import 任何 UI。CI 用**直接 import** 检查（`go list -f '{{join .Imports "\n"}}'`，绝不 `-deps`），机械化在 `tools/ciguard`。
- 观测为主：**不创建 Issue、不触发 CI、不调度 Agent、不写任何外部系统**。`git push` 不在本计划授权范围内。
- 所有 SQL 取值只走 `?` 占位 + `Args`。唯一允许的字符串插值是 `PRAGMA user_version = %d`。
- ID 前缀只用 `sys:` / `ctn:` / `cmp:`（仓库行 `repo:`），不含 snapshot id、不含自增值。稳定 ID 是跨仓对齐的根基（spec §7 前提），任何改动元素 ID 生成的任务都会让 PoC-8 立刻变红。
- schema 变更只能**追加** `migrations[k]` 并 bump `schemaVersion`；禁止对既有表 `ALTER`（SQLite 方言面）。迁移拒绝降级，整版成功后才 bump `user_version`。
- 快照封顶默认 **50**/仓（`app_config` 键 `history.max_snapshots` 可覆盖），增量默认 **开**（`scan.incremental`）。
- 退出码只有 **0/1**；输出 `text` + `--json`。不为「歧义/不存在」引入 2/3/4（那是切片 7 的码表）。
- 节点卡片尺寸恒定 **168 × 46 DIP**；LOD 阈值 `lodDot=0.5` / `lodFull=0.9`；对比着色只改描边/虚线，**不改盒尺寸**（布局稳定性约束，切片 1 定死）。
- 每个任务收尾必须：`go vet ./... && go test ./...` 全绿（涉及并发或扫描路径的再加 `CGO_ENABLED=1 go test -race ./...`），然后 `git add` 明确列出的文件 + 一次提交，message 用 `feat:` / `test:` / `chore:` / `docs:` 前缀 + 中文正文。
- 与 golden/平价比对前**必须排序**（`go/packages` 的包遍历顺序不保证）。
- **任一 PoC 门槛不通过就停下来向用户报告，不要绕过它继续做后面的任务**。走哪条失败分支是用户的决定。

### 一次性环境前置（做任何任务之前执行一次）

```sh
export PATH="/usr/local/go/bin:$PATH"
export GOPROXY=https://goproxy.cn,direct
go version          # 期望 go1.27.2 darwin/arm64
go vet ./... && go test ./...   # 基线必须全绿（2026-10-30 实测 10 个包全绿）
```

### 工作区

用 `superpowers:using-git-worktrees` 建隔离工作区（分支名 `slice2-incremental`），worktree 目录沿用仓库既有的 `.worktrees/`（`.gitignore` 已覆盖）。本计划的 DoD 验证最终要回到 `main` 之前跑一次全量，合并动作本身（merge/push）需用户明确批准。

---

## 文件结构（新建 = N，修改 = M）

```
internal/core/model/
  scan.go            N  FileHash / HistoryEvent / 事件名常量
internal/core/store/
  schema.go          M  schemaVersion 2 + migrations[1]
  scan_commit.go     N  ScanCommit / CommitScan / 封顶裁剪语句（取代 SaveSnapshot）
  files.go           N  arch_files 读写
  fingerprint.go     N  pkg_fingerprints 读写
  history.go         N  element_history 读写
  snapshot.go        M  删 SaveSnapshot；加 Snapshots(repoID, limit)
  repo.go            M  加 DeleteRepo
internal/core/graph/
  merge.go           N  Merge(graphs) (*Graph, []string)
  graph.go           M  删 Merge 方法与 ErrNotImplemented
  diff.go            N  Diff(a, b) Diff / ChangedFields
  rank.go            N  RankDependents
internal/scanner/
  language/adapter.go M  Output.Packages []PackageSource
  language/go.go      M  填充 Packages（文件相对路径 + 第一方直接 import）
  fingerprint/fingerprint.go N  Compute（纯函数 Merkle，无 IO）
  hash.go             N  FileSHA256 / Sentinel / HashFiles
  verify.go           N  Verify（两值裁决三清单）
  reuse.go            N  splice（未变包整段搬运）
  scanner.go          M  Run/RunWithOptions 编排 + Report 新字段
  report.go           M  Report{+Reused,+Parsed,+Incremental}
internal/config/
  config.go           M  Incremental / MaxSnapshots 两键
cmd/oac/
  main.go             M  subcommand 新形状 + options
  options.go          N  options 结构 / stringListFlag / wantOne / writeJSON
  repo.go             N  oac repo list|remove
  diff.go             N  oac diff
  history.go          N  oac history
  rank.go             N  oac rank
  verify.go           N  oac verify
  scan.go             M  --full、reused/parsed 行、--json
  graph.go            M  --all/--repo、--json
  impact.go           M  --all/--repo、--json
internal/ui/
  node_card.go        M  nodeCard(c, el, zoom, mark)
  flow_data.go        M  mark 通道 + 幽灵节点
  state.go            M  对比状态、历史状态、Compare setter
  app.go              M  Reload 用 graph.Merge
  page_compare.go     N  mapHeader 的对比段（快照选择 + 计数行）
  page_graph.go       M  对比摘要行接进 mapHeader
  page_repos.go       M  添加/删除/全部重扫/新鲜度徽标/对比入口
  page_settings.go    M  增量开关与封顶输入行
  detail_panel.go     M  历史段
docs/superpowers/poc/
  2026-10-30-slice2-findings.md  N  PoC-8/9/11 实测数字（结论/数字/影响 三段）
```

依赖方向不变：`model` ← `store`/`graph` ← `scanner` ← `cmd`/`ui`；`scanner/fingerprint` 只准 `language` + `model` + 标准库。

---

## Phase A — 数据地基

### Task 1: schema v2（三张新表 + 两条索引）

**Files:**
- Modify: `internal/core/store/schema.go`
- Modify: `internal/core/store/migrate_test.go`（加 v2 的迁移断言）
- Test: `internal/core/store/schema_v2_test.go`（新建）

**Interfaces:**
- Consumes: 既有的 `migrations [][]string` + `PRAGMA user_version` 机制（`store/migrate.go`、`store/db.go:51-68`）。
- Produces: `schemaVersion = 2`；表 `arch_files(snapshot_id, file_path, sha256)`、`pkg_fingerprints(snapshot_id, pkg_path, fp)`、`element_history(repo_id, element_id, snapshot_id, event, detail_json, created_at)`；索引 `idx_files_snapshot`、`idx_history_element`、`idx_history_repo`。后续任务只写 SQL 常量，不再碰 DDL。

- [ ] **Step 1: 写失败的迁移测试**

新建 `internal/core/store/schema_v2_test.go`：

```go
package store

import (
	"context"
	"testing"
)

// v2 带来三张表。这里只钉「表在、列在、CASCADE 在」——列语义由各自的
// CRUD 测试负责，别在这儿重复一遍。
func TestV2TablesExist(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	ctx := context.Background()

	for _, table := range []string{"arch_files", "pkg_fingerprints", "element_history"} {
		rows, err := db.Query(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table)
		if err != nil || len(rows.Values) == 0 {
			t.Errorf("table %s missing after Migrate (%v)", table, err)
		}
	}
	v, err := db.UserVersion(ctx)
	if err != nil || v != 2 {
		t.Errorf("user_version = %d, %v; want 2", v, err)
	}
}

// 删快照必须连带收走它的 hash 行、指纹行和历史行——封顶策略（Task 3）完全
// 依赖这条 CASCADE，所以它是产品行为，不是数据库细节。
func TestV2CascadeReclaimsSlice2Rows(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	ctx := context.Background()
	repo := repoFixture()
	if err := db.UpsertRepo(ctx, repo); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Execute(ctx, insertSnapshotSQL, "s1", repo.ID, "", "", "[]"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Execute(ctx, `INSERT INTO arch_files (snapshot_id, file_path, sha256) VALUES (?, ?, ?)`,
		"s1", "a.go", "aa"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Execute(ctx, `INSERT INTO pkg_fingerprints (snapshot_id, pkg_path, fp) VALUES (?, ?, ?)`,
		"s1", "example.com/x", "bb"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Execute(ctx, `INSERT INTO element_history (repo_id, element_id, snapshot_id, event, detail_json)
		VALUES (?, ?, ?, ?, ?)`, repo.ID, "cmp:example.com/x.A", "s1", "appeared", "{}"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Execute(ctx, `DELETE FROM architecture_snapshots WHERE id = ?`, "s1"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		table string
		query string
	}{
		// 三条 SQL 逐字写死，不拼表名：全局约束里唯一允许的插值是
		// `PRAGMA user_version = %d`，测试代码也不是例外（拼一次就有人在生产代码里拼第二次）。
		{"arch_files", `SELECT COUNT(*) FROM arch_files`},
		{"pkg_fingerprints", `SELECT COUNT(*) FROM pkg_fingerprints`},
		{"element_history", `SELECT COUNT(*) FROM element_history`},
	} {
		rows, err := db.Query(ctx, tc.query)
		if err != nil {
			t.Fatal(err)
		}
		if n := num(rows.Values[0][0]); n != 0 {
			t.Errorf("%s kept %d orphan rows; CASCADE missing", tc.table, int(n))
		}
	}
}
```

> `insertSnapshotSQL` 是包内私有常量，同包测试直接用。它的列序是
> `(id, repo_id, commit_hash, branch, package_errors_json)`（`internal/core/store/snapshot.go:15-16`
> 与 `:26-27` 的 Args 顺序），所以 `?` 的实参顺序是 **快照 id 在前、repo id 在后**——
> 两个都是字符串，写反了不会报错，只会让 `arch_snapshots.repo_id` 装进一个快照 id、
> 外键当场失败或（在没有 FK 的列上）静默存错。


- [ ] **Step 2: 跑测试确认失败**

```sh
export PATH="/usr/local/go/bin:$PATH" CGO_ENABLED=0
go test ./internal/core/store/ -run 'TestV2' -v
```

期望：FAIL，`table arch_files missing after Migrate`（不是编译错误——DDL 缺失才是这条测试该抓的东西）。

- [ ] **Step 3: 写 migrations[1]**

改 `internal/core/store/schema.go`：`const schemaVersion = 2`，并在 `migrations` 外层切片里追加第二个元素（逐字取自 spec §5，不增删列）：

```go
var migrations = [][]string{{
	// …… v1 原文不动 ……
}, {
	`CREATE TABLE IF NOT EXISTS arch_files (
		snapshot_id TEXT NOT NULL REFERENCES architecture_snapshots(id) ON DELETE CASCADE,
		file_path   TEXT NOT NULL,
		sha256      TEXT NOT NULL,
		PRIMARY KEY (snapshot_id, file_path)
	)`,
	`CREATE TABLE IF NOT EXISTS pkg_fingerprints (
		snapshot_id TEXT NOT NULL REFERENCES architecture_snapshots(id) ON DELETE CASCADE,
		pkg_path    TEXT NOT NULL,
		fp          TEXT NOT NULL,
		PRIMARY KEY (snapshot_id, pkg_path)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_files_snapshot ON arch_files(snapshot_id)`,
	`CREATE TABLE IF NOT EXISTS element_history (
		repo_id     TEXT NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
		element_id  TEXT NOT NULL,
		snapshot_id TEXT NOT NULL REFERENCES architecture_snapshots(id) ON DELETE CASCADE,
		event       TEXT NOT NULL,
		detail_json TEXT NOT NULL DEFAULT '{}',
		created_at  TEXT DEFAULT (datetime('now'))
	)`,
	`CREATE INDEX IF NOT EXISTS idx_history_element ON element_history(element_id)`,
	`CREATE INDEX IF NOT EXISTS idx_history_repo ON element_history(repo_id, snapshot_id)`,
}}
```

- [ ] **Step 4: 跑测试确认通过**

```sh
go test ./internal/core/store/ -run 'TestV2|TestMigrate' -v
```

期望：全 PASS，包括切片 1 钉住的「拒绝降级」测试（`schemaVersion` 现在是 2，它对 v3 的断言仍成立）。若 `migrate_test.go` 里有硬写的 `want 1`，把它改成 `schemaVersion` 而不是 `2`——钉的是「升到顶」，不是「顶在哪」。

- [ ] **Step 5: 全量 + 提交**

```sh
go vet ./... && go test ./...
git add internal/core/store/schema.go internal/core/store/schema_v2_test.go internal/core/store/migrate_test.go
git commit -m "feat: schema v2（arch_files / pkg_fingerprints / element_history）"
```

---

### Task 2: model 类型 + arch_files / pkg_fingerprints 读写

**Files:**
- Create: `internal/core/model/scan.go`
- Create: `internal/core/store/files.go`
- Create: `internal/core/store/fingerprint.go`
- Test: `internal/core/store/files_test.go`、`internal/core/store/fingerprint_test.go`

**Interfaces:**
- Consumes: Task 1 的两张表；`DB.Query/Execute`、`rowsToMaps/str/num/jsonString/stringList`（`store/rows.go`）。
- Produces:
  - `model.FileHash{Path, SHA256 string}`
  - `model.HistoryEvent{RepoID, ElementID, SnapshotID string; Event string; Fields []string}`
  - `const model.EventAppeared / EventChanged / EventDisappeared = "appeared" / "changed" / "disappeared"`
  - `(d *DB) Files(ctx, snapshotID string) ([]model.FileHash, error)` — 按 `file_path` 升序
  - `(d *DB) Fingerprints(ctx, snapshotID string) (map[string]string, error)` — pkgPath → fp；无行返回空 map（不是 nil 检查陷阱，调用方只读）

- [ ] **Step 1: 写 model 类型**

新建 `internal/core/model/scan.go`（R1：不 import UI，不 import store）：

```go
package model

// FileHash 是一个快照里某个被解析文件的 SHA-256。Path 与
// Element.FilePath 同规则（仓库根相对），所以 verify 能直接拼回磁盘路径。
type FileHash struct {
	Path   string
	SHA256 string
}

// HistoryEvent 是元素在某个快照上发生的事。Fields 只在 changed 时非空，
// 存的是变化字段的列名（element_type / file_path / methods_json / description）。
type HistoryEvent struct {
	RepoID     string
	ElementID  string
	SnapshotID string
	Event      string
	Fields     []string
	CreatedAt  string
}

const (
	EventAppeared    = "appeared"
	EventChanged     = "changed"
	EventDisappeared = "disappeared"
)
```

- [ ] **Step 2: 写失败的读写测试**

新建 `internal/core/store/files_test.go`：

```go
package store

import (
	"context"
	"reflect"
	"testing"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

func TestFilesRoundTrip(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	ctx := context.Background()
	repo := repoFixture()
	if err := db.UpsertRepo(ctx, repo); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Execute(ctx, insertSnapshotSQL, "s1", repo.ID, "", "", "[]"); err != nil {
		t.Fatal(err)
	}
	want := []model.FileHash{
		{Path: "b.go", SHA256: "bb"},
		{Path: "a.go", SHA256: "aa"},
	}
	for _, f := range want { // 故意乱序写入：读必须有序
		if _, err := db.Execute(ctx, insertFileSQL, "s1", f.Path, f.SHA256); err != nil {
			t.Fatal(err)
		}
	}
	got, err := db.Files(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []model.FileHash{{Path: "a.go", SHA256: "aa"}, {Path: "b.go", SHA256: "bb"}}) {
		t.Errorf("Files = %+v", got)
	}
}

func TestFingerprintsRoundTrip(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	ctx := context.Background()
	repo := repoFixture()
	if err := db.UpsertRepo(ctx, repo); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Execute(ctx, insertSnapshotSQL, "s1", repo.ID, "", "", "[]"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Execute(ctx, insertFingerprintSQL, "s1", "example.com/x/store", "fp1"); err != nil {
		t.Fatal(err)
	}
	got, err := db.Fingerprints(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if got["example.com/x/store"] != "fp1" {
		t.Errorf("Fingerprints = %v", got)
	}
	// 切片 1 的 v1 库没有任何指纹行——空 map 是「没有基线」，调用方据此退回全量。
	none, err := db.Fingerprints(ctx, "s0000000000000000000")
	if err != nil || len(none) != 0 {
		t.Errorf("Fingerprints(unknown) = %v, %v", none, err)
	}
}
```

- [ ] **Step 3: 跑测试确认失败**

```sh
go test ./internal/core/store/ -run 'TestFilesRoundTrip|TestFingerprintsRoundTrip' -v
```

期望：编译失败 `undefined: insertFileSQL`（这就是「失败」，缺的是实现）。

- [ ] **Step 4: 写两个读文件**

`internal/core/store/files.go`：

```go
package store

import (
	"context"
	"fmt"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

const (
	insertFileSQL = `INSERT INTO arch_files (snapshot_id, file_path, sha256) VALUES (?, ?, ?)`
	// 有序：平价测试直接比切片，不需要每处调用方再排一遍。
	selectFilesSQL = `SELECT file_path, sha256 FROM arch_files WHERE snapshot_id = ? ORDER BY file_path`
)

func (d *DB) Files(ctx context.Context, snapshotID string) ([]model.FileHash, error) {
	rows, err := d.Query(ctx, selectFilesSQL, snapshotID)
	if err != nil {
		return nil, fmt.Errorf("store: files: %w", err)
	}
	out := make([]model.FileHash, 0, len(rows.Values))
	for _, row := range rowsToMaps(rows) {
		out = append(out, model.FileHash{Path: str(row["file_path"]), SHA256: str(row["sha256"])})
	}
	return out, nil
}
```

`internal/core/store/fingerprint.go`：

```go
package store

import (
	"context"
	"fmt"
)

const (
	insertFingerprintSQL = `INSERT INTO pkg_fingerprints (snapshot_id, pkg_path, fp) VALUES (?, ?, ?)`
	selectFingerprintsSQL = `SELECT pkg_path, fp FROM pkg_fingerprints WHERE snapshot_id = ?`
)

func (d *DB) Fingerprints(ctx context.Context, snapshotID string) (map[string]string, error) {
	out := map[string]string{}
	if snapshotID == "" {
		return out, nil
	}
	rows, err := d.Query(ctx, selectFingerprintsSQL, snapshotID)
	if err != nil {
		return nil, fmt.Errorf("store: fingerprints: %w", err)
	}
	for _, row := range rowsToMaps(rows) {
		out[str(row["pkg_path"])] = str(row["fp"])
	}
	return out, nil
}
```

- [ ] **Step 5: 跑测试确认通过 + 全量 + 提交**

```sh
go test ./internal/core/store/ -v -run 'TestFiles|TestFingerprints'
go vet ./... && go test ./...
git add internal/core/model/scan.go internal/core/store/files.go internal/core/store/fingerprint.go internal/core/store/files_test.go internal/core/store/fingerprint_test.go
git commit -m "feat: model 的 FileHash/HistoryEvent 与 arch_files/pkg_fingerprints 读写"
```

---

### Task 3: `CommitScan` —— 一次事务写全部，并做快照封顶

**Files:**
- Create: `internal/core/store/scan_commit.go`
- Modify: `internal/core/store/snapshot.go`（删 `SaveSnapshot`）
- Modify: `internal/core/store/snapshot_test.go`（7 处调用：`:47,116,146,179,242,278,301`）、`internal/core/store/bench_test.go`（`:42`）——实测共 8 处测试调用改形
- Modify: `internal/scanner/scanner.go:46`（唯一的产品调用方）
- Test: `internal/core/store/scan_commit_test.go`

**Interfaces:**
- Consumes: Task 2 的 `insertFileSQL`/`insertFingerprintSQL`、既有的 `insertSnapshotSQL`/`insertElementSQL`/`insertRelationshipSQL`、`DB.Transact([]sqlite.Statement)`。
- Produces:

```go
type ScanCommit struct {
	Snapshot      model.ArchitectureSnapshot
	Elements      []model.Element
	Relationships []model.Relationship
	Files         []model.FileHash
	Fingerprints  map[string]string
	History       []model.HistoryEvent
	// KeepSnapshots > 0 时把该仓快照数裁到这个值（删最旧，CASCADE 收走子表）。
	// 0 或负数 = 不裁剪（测试与手工修复用）。
	KeepSnapshots int
}
func (d *DB) CommitScan(ctx context.Context, c ScanCommit) error
```

- [ ] **Step 1: 写失败测试（原子性 + 封顶）**

新建 `internal/core/store/scan_commit_test.go`：

```go
package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

func commitFixture(repo model.Repo, id string) ScanCommit {
	return ScanCommit{
		Snapshot:      model.ArchitectureSnapshot{ID: id, RepoID: repo.ID},
		Elements:      elementsFixture(repo),
		Relationships: relsFixture(),
		Files:         []model.FileHash{{Path: "x.go", SHA256: "aa"}},
		Fingerprints:  map[string]string{"example.com/x": "fp"},
		History: []model.HistoryEvent{{
			RepoID: repo.ID, ElementID: elementsFixture(repo)[0].ID,
			SnapshotID: id, Event: model.EventAppeared,
		}},
		KeepSnapshots: 50,
	}
}

func TestCommitScanWritesEverySection(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	ctx := context.Background()
	repo := repoFixture()
	if err := db.UpsertRepo(ctx, repo); err != nil {
		t.Fatal(err)
	}
	if err := db.CommitScan(ctx, commitFixture(repo, "s0000000000000000001")); err != nil {
		t.Fatal(err)
	}
	files, err := db.Files(ctx, "s0000000000000000001")
	if err != nil || len(files) != 1 {
		t.Errorf("files = %+v, %v", files, err)
	}
	fps, err := db.Fingerprints(ctx, "s0000000000000000001")
	if err != nil || fps["example.com/x"] != "fp" {
		t.Errorf("fingerprints = %v, %v", fps, err)
	}
	evs, err := db.History(ctx, elementsFixture(repo)[0].ID, 5)
	if err != nil || len(evs) != 1 || evs[0].Event != model.EventAppeared {
		t.Errorf("history = %+v, %v", evs, err)
	}
}

// 一条元素语句失败 = 整笔回滚：快照行、hash 行、指纹行、历史行都不许留下。
// 这是 spec §8「四者原子」的机械化，也是增量复用敢写「库里即真相」的前提。
func TestCommitScanIsAtomic(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	ctx := context.Background()
	repo := repoFixture()
	if err := db.UpsertRepo(ctx, repo); err != nil {
		t.Fatal(err)
	}
	c := commitFixture(repo, "s0000000000000000001")
	c.Elements = append(c.Elements, model.Element{ID: "dup", Type: model.TypeComponent, Name: "dup", RepoID: repo.ID})
	c.Elements = append(c.Elements, c.Elements[0]) // 同 id 两次 → 主键冲突
	if err := db.CommitScan(ctx, c); err == nil {
		t.Fatal("duplicate element must fail")
	}
	if id, err := db.LatestSnapshotID(ctx, repo.ID); err != nil || id != "" {
		t.Errorf("snapshot survived a failed commit: %q, %v", id, err)
	}
	if fps, err := db.Fingerprints(ctx, "s0000000000000000001"); err != nil || len(fps) != 0 {
		t.Errorf("fingerprints survived a failed commit: %v", fps)
	}
}

func TestCommitScanTrimsToKeep(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	ctx := context.Background()
	repo := repoFixture()
	if err := db.UpsertRepo(ctx, repo); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 5; i++ {
		c := commitFixture(repo, snapshotIDFor(i))
		c.KeepSnapshots = 3
		if err := db.CommitScan(ctx, c); err != nil {
			t.Fatalf("commit %d: %v", i, err)
		}
	}
	// 5 个快照、封顶 3 → 留 s5/s4/s3；s5 的历史行必须还在，s1 的必须没了。
	latest, err := db.LatestSnapshotID(ctx, repo.ID)
	if err != nil || latest != snapshotIDFor(5) {
		t.Fatalf("latest = %q, %v", latest, err)
	}
	rows, err := db.Query(ctx, `SELECT COUNT(*) FROM architecture_snapshots WHERE repo_id = ?`, repo.ID)
	if err != nil {
		t.Fatal(err)
	}
	if n := int(num(rows.Values[0][0])); n != 3 {
		t.Errorf("snapshot count = %d, want 3", n)
	}
	fps, err := db.Fingerprints(ctx, snapshotIDFor(2))
	if err != nil || len(fps) != 0 {
		t.Errorf("trimmed snapshot kept fingerprints: %v", fps)
	}
}

// snapshotIDFor 复刻 model.NewSnapshotID 的定宽格式：小序号必须零填充，
// 否则 "ORDER BY id DESC" 的字典序不再是时间序（切片 1 的钉）。
func snapshotIDFor(n int) string {
	return fmt.Sprintf("s%019d", n)
}
```

`fmt` 与 `strings` 里只用 `fmt`——写完后删未用 import（`strings` 是给你看板的，不需要就删）。测试里引用了 `db.History`，它由 Task 4 提供：**本任务的 Step 4 里先把 `History` 的读函数一起写进来**（见下），否则 Step 1 的测试穿不过编译。

- [ ] **Step 2: 跑测试确认失败**

```sh
go test ./internal/core/store/ -run 'TestCommitScan' -v
```

期望：编译失败 `undefined: ScanCommit` / `db.CommitScan` / `db.History`。

- [ ] **Step 3: 写 `scan_commit.go`**

```go
package store

import (
	"context"
	"fmt"
	"sort"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
	"github.com/egoist/mygo/plugins/sqlite"
)

// W2 的延续：一次扫描 = 一笔事务。driver 收的是静态批次，所以每条语句都要
// 预先算好（含裁剪语句）。
const (
	insertHistorySQL = `INSERT INTO element_history (repo_id, element_id, snapshot_id, event, detail_json)
		VALUES (?, ?, ?, ?, ?)`
	// 封顶：删掉「不属于最新 keep 个」的快照行。子表靠 v1/v2 的
	// ON DELETE CASCADE 收走，所以这里只有这一条 DELETE。
	// keep 走 `?`：全局约束只允许 PRAGMA 插值。
	deleteOldSnapshotsSQL = `DELETE FROM architecture_snapshots
		WHERE repo_id = ? AND id NOT IN (
			SELECT id FROM architecture_snapshots WHERE repo_id = ? ORDER BY id DESC LIMIT ?)`
)

func (d *DB) CommitScan(ctx context.Context, c ScanCommit) error {
	stmts := make([]sqlite.Statement, 0, 1+len(c.Elements)+len(c.Relationships)+
		len(c.Files)+len(c.Fingerprints)+len(c.History)+1)
	stmts = append(stmts, sqlite.Statement{SQL: insertSnapshotSQL,
		Args: []any{c.Snapshot.ID, c.Snapshot.RepoID, c.Snapshot.CommitHash, c.Snapshot.Branch,
			jsonString(defaultStrings(c.Snapshot.PackageErrors))}})
	for _, e := range c.Elements {
		stmts = append(stmts, sqlite.Statement{SQL: insertElementSQL,
			Args: []any{c.Snapshot.ID, e.ID, string(e.Type), e.Name, e.Description, e.RepoID, e.FilePath,
				jsonString(defaultStrings(e.Methods))}})
	}
	for _, r := range c.Relationships {
		weight := r.Weight
		if weight < 1 {
			weight = 1
		}
		stmts = append(stmts, sqlite.Statement{SQL: insertRelationshipSQL,
			Args: []any{c.Snapshot.ID, r.FromID, r.ToID, string(r.Type), weight}})
	}
	for _, f := range c.Files {
		stmts = append(stmts, sqlite.Statement{SQL: insertFileSQL,
			Args: []any{c.Snapshot.ID, f.Path, f.SHA256}})
	}
	for _, pkg := range sortedKeys(c.Fingerprints) {
		stmts = append(stmts, sqlite.Statement{SQL: insertFingerprintSQL,
			Args: []any{c.Snapshot.ID, pkg, c.Fingerprints[pkg]}})
	}
	for _, h := range c.History {
		stmts = append(stmts, sqlite.Statement{SQL: insertHistorySQL,
			Args: []any{h.RepoID, h.ElementID, h.SnapshotID, h.Event, historyDetail(h.Fields)}})
	}
	if c.KeepSnapshots > 0 {
		stmts = append(stmts, sqlite.Statement{SQL: deleteOldSnapshotsSQL,
			Args: []any{c.Snapshot.RepoID, c.Snapshot.RepoID, c.KeepSnapshots}})
	}
	if _, err := d.Transact(ctx, stmts); err != nil {
		return fmt.Errorf("store: commit scan %s (%d statements): %w", c.Snapshot.ID, len(stmts), err)
	}
	return nil
}

// sortedKeys 让事务批次可复现：map 的迭代序随机，不排序的话同一份扫描两次
// 生成的语句序列不同，平价测试（T13）会抓到假的不一致。
func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// historyDetail 存的是变化字段名列表。空列表存 "{}" 而不是 {"fields":[]}：
// appeared/disappeared 事件本来就没有字段维度，读出后 Fields 为 nil。
func historyDetail(fields []string) string {
	if len(fields) == 0 {
		return "{}"
	}
	return jsonString(map[string]any{"fields": fields})
}
```

并在 `scan_commit.go` 顶部同一文件内声明结构（只声明 `ScanCommit`；`defaultStrings` 已经定义在同包的 `snapshot.go` 里，**不要搬、不要在 `scan_commit.go` 再写一遍**——Go 的同包跨文件引用不需要声明顺序，重复定义是编译错误）：

```go
// ScanCommit 是一次扫描提交的全部内容。见 spec §8：快照、hash、指纹、历史
// 四者必须原子，所以只有一个写入口。
type ScanCommit struct {
	Snapshot      model.ArchitectureSnapshot
	Elements      []model.Element
	Relationships []model.Relationship
	Files         []model.FileHash
	Fingerprints  map[string]string
	History       []model.HistoryEvent
	KeepSnapshots int
}
```

- [ ] **Step 4: 写 `History` 读（Task 4 会补它的事件测试）**

新建 `internal/core/store/history.go`：

```go
package store

import (
	"context"
	"fmt"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

const (
	// 时间倒序：id 是定宽纳秒戳，所以 ORDER BY snapshot_id DESC 就是 newest first
	// （同 selectLatestSnapshotSQL 的口径）。并列元素用 id 打散，保证可复现。
	selectHistorySQL = `SELECT repo_id, element_id, snapshot_id, event, detail_json, created_at
		FROM element_history WHERE element_id = ?
		ORDER BY snapshot_id DESC, element_id LIMIT ?`
	countHistorySQL = `SELECT COUNT(*) FROM element_history WHERE element_id = ?`
)

func (d *DB) History(ctx context.Context, elementID string, limit int) ([]model.HistoryEvent, error) {
	rows, err := d.Query(ctx, selectHistorySQL, elementID, limit)
	if err != nil {
		return nil, fmt.Errorf("store: history: %w", err)
	}
	out := make([]model.HistoryEvent, 0, len(rows.Values))
	for _, row := range rowsToMaps(rows) {
		out = append(out, model.HistoryEvent{
			RepoID: str(row["repo_id"]), ElementID: str(row["element_id"]),
			SnapshotID: str(row["snapshot_id"]), Event: str(row["event"]),
			Fields: historyFields(str(row["detail_json"])), CreatedAt: str(row["created_at"]),
		})
	}
	return out, nil
}

func (d *DB) HistoryCount(ctx context.Context, elementID string) (int, error) {
	rows, err := d.Query(ctx, countHistorySQL, elementID)
	if err != nil {
		return 0, err
	}
	return int(num(rows.Values[0][0])), nil
}

// historyFields 读 historyDetail 写出的 {"fields":[...]}。
func historyFields(raw string) []string {
	m := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil
	}
	list, _ := m["fields"].([]any)
	out := make([]string, 0, len(list))
	for _, v := range list {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
```

`history.go` 的 import 里必须有 `"encoding/json"`（实测 `store` 包里没有 `jsonUnmarshal` 这种包装，`rows.go` 是直接用 `encoding/json` 的 `jsonString`/`stringList`）。写的一侧不用另找包装：`historyDetail` 走既有的 `jsonString`。

- [ ] **Step 5: 删 `SaveSnapshot` 并改所有调用方**

`internal/core/store/snapshot.go`：删掉 `SaveSnapshot` 函数本体（`insertSnapshotSQL`、`defaultStrings`、`LatestSnapshotID`、`Snapshot`、`SnapshotForElement` 全部保留）。`defaultStrings` 就留在 `snapshot.go:47`，Step 3 直接同包引用它。

改这 8 处测试调用为 `CommitScan`（另有 `scanner.go:46` 一处产品调用，Step 6 里改），机械替换形态：

```go
// old
err := db.SaveSnapshot(ctx, snapshotFixture(repo), elementsFixture(repo), relsFixture())
// new
err := db.CommitScan(ctx, ScanCommit{Snapshot: snapshotFixture(repo),
	Elements: elementsFixture(repo), Relationships: relsFixture(), KeepSnapshots: 50})
```

`internal/scanner/scanner.go:46` 同步改成：

```go
if err := db.CommitScan(ctx, store.ScanCommit{Snapshot: snapshot,
	Elements: out.Elements, Relationships: out.Relationships,
	KeepSnapshots: defaultMaxSnapshots}); err != nil {
	return Report{}, err
}
```

`defaultMaxSnapshots` 本任务先给包内常量（T10 换成 config 里的值）：

```go
// internal/scanner/scanner.go
const defaultMaxSnapshots = 50
```

> 封顶 50 从这一步起**真的生效**。切片 1 的历史是「每次扫描留一个快照」，现在超过 50 会删最旧——`bench_test.go` 与 `snapshot_test.go` 里若有依赖第 N 个旧快照的断言，改用 ≤50 的数量（现有测试最多写 3 个快照，实测安全）。

- [ ] **Step 6: 跑测试确认通过 + 全量 + 提交**

```sh
go test ./internal/core/store/ ./internal/scanner/ -v -run 'TestCommitScan|TestSave|TestScan'
go vet ./... && go test ./...
git add internal/core/store/scan_commit.go internal/core/store/history.go internal/core/store/snapshot.go internal/core/store/snapshot_test.go internal/core/store/bench_test.go internal/core/store/scan_commit_test.go internal/scanner/scanner.go
git commit -m "feat: CommitScan 单事务写快照+hash+指纹+历史，并把快照数封顶 50"
```

---

### Task 4: `Snapshots` 列表与 `DeleteRepo`

**Files:**
- Modify: `internal/core/store/snapshot.go`
- Modify: `internal/core/store/repo.go`
- Modify: `internal/core/store/model` 无
- Test: `internal/core/store/repo_test.go`、`internal/core/store/history_test.go`

**Interfaces:**
- Consumes: Task 3 的 `insertHistorySQL`/`db.History`。
- Produces:
  - `(d *DB) Snapshots(ctx, repoID string, limit int) ([]model.ArchitectureSnapshot, error)` — 新→旧
  - `(d *DB) DeleteRepo(ctx, repoID string) error`
  - `(d *DB) Repo(ctx, repoID string) (model.Repo, error)`（仓库页删除确认要显示名字；`Repos` 已按 name 排序，但没有单条读）

- [ ] **Step 1: 写失败测试**

新建 `internal/core/store/repo_test.go`：

```go
package store

import (
	"context"
	"testing"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

// 删仓是级联删干净（计划定稿 U2）：孤儿历史没有查询入口，留着只是占盘。
func TestDeleteRepoCascadesEverything(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	ctx := context.Background()
	repo := repoFixture()
	if err := db.UpsertRepo(ctx, repo); err != nil {
		t.Fatal(err)
	}
	if err := db.CommitScan(ctx, commitFixture(repo, "s0000000000000000001")); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteRepo(ctx, repo.ID); err != nil {
		t.Fatal(err)
	}
	repos, err := db.Repos(ctx)
	if err != nil || len(repos) != 0 {
		t.Errorf("repos = %+v, %v", repos, err)
	}
	for _, q := range []string{"architecture_snapshots", "arch_elements", "arch_relationships",
		"arch_files", "pkg_fingerprints", "element_history"} {
		rows, err := db.Query(ctx, `SELECT COUNT(*) FROM `+q)
		if err != nil {
			t.Fatal(err)
		}
		if n := int(num(rows.Values[0][0])); n != 0 {
			t.Errorf("%s kept %d rows after DeleteRepo", q, n)
		}
	}
}

func TestSnapshotsListNewestFirst(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	ctx := context.Background()
	repo := repoFixture()
	if err := db.UpsertRepo(ctx, repo); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"s0000000000000000001", "s0000000000000000002", "s0000000000000000003"} {
		if err := db.CommitScan(ctx, commitFixture(repo, id)); err != nil {
			t.Fatal(err)
		}
	}
	got, err := db.Snapshots(ctx, repo.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "s0000000000000000003" || got[1].ID != "s0000000000000000002" {
		t.Errorf("Snapshots = %+v", got)
	}
}

func TestRepoSingleRead(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	ctx := context.Background()
	repo := repoFixture()
	if err := db.UpsertRepo(ctx, repo); err != nil {
		t.Fatal(err)
	}
	got, err := db.Repo(ctx, repo.ID)
	if err != nil || got.Name != repo.Name {
		t.Errorf("Repo = %+v, %v", got, err)
	}
	if _, err := db.Repo(ctx, model.RepoIDFor("/nope")); err == nil {
		t.Error("unknown repo must error")
	}
}
```

新建 `internal/core/store/history_test.go`（钉 T3 里 `History` 的字段语义）：

```go
package store

import (
	"context"
	"reflect"
	"testing"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

func TestHistoryCarriesChangedFields(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	ctx := context.Background()
	repo := repoFixture()
	if err := db.UpsertRepo(ctx, repo); err != nil {
		t.Fatal(err)
	}
	if err := db.CommitScan(ctx, commitFixture(repo, "s0000000000000000001")); err != nil {
		t.Fatal(err)
	}
	el := elementsFixture(repo)[1].ID
	if _, err := db.Execute(ctx, insertHistorySQL, repo.ID, el, "s0000000000000000002",
		model.EventChanged, historyDetail([]string{"methods_json", "description"})); err != nil {
		t.Fatal(err)
	}
	evs, err := db.History(ctx, el, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 || evs[0].Event != model.EventChanged {
		t.Fatalf("history = %+v", evs)
	}
	if !reflect.DeepEqual(evs[0].Fields, []string{"methods_json", "description"}) {
		t.Errorf("Fields = %v", evs[0].Fields)
	}
	if n, err := db.HistoryCount(ctx, el); err != nil || n != 1 {
		t.Errorf("HistoryCount = %d, %v", n, err)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

```sh
go test ./internal/core/store/ -run 'TestDeleteRepo|TestSnapshotsList|TestRepoSingleRead|TestHistoryCarries' -v
```

期望：编译失败 `db.DeleteRepo undefined` 等。

- [ ] **Step 3: 实现**

`internal/core/store/snapshot.go` 追加：

```go
const selectSnapshotsSQL = `SELECT id, repo_id, commit_hash, branch, package_errors_json, created_at
	FROM architecture_snapshots WHERE repo_id = ? ORDER BY id DESC LIMIT ?`

// Snapshots 是「这一仓扫过哪几次」，新→旧。CLI 的 `oac diff` 默认对取两个、
// GUI 的快照选择器都读它。limit <= 0 时不给上限——用 -1 表达「无 LIMIT」是
// SQLite 的 LIMIT 语义，走 `?` 而不是拼字符串。
func (d *DB) Snapshots(ctx context.Context, repoID string, limit int) ([]model.ArchitectureSnapshot, error) {
	if limit <= 0 {
		limit = -1
	}
	rows, err := d.Query(ctx, selectSnapshotsSQL, repoID, limit)
	if err != nil {
		return nil, fmt.Errorf("store: snapshots: %w", err)
	}
	out := make([]model.ArchitectureSnapshot, 0, len(rows.Values))
	for _, row := range rowsToMaps(rows) {
		out = append(out, model.ArchitectureSnapshot{
			ID: str(row["id"]), RepoID: str(row["repo_id"]),
			CommitHash: str(row["commit_hash"]), Branch: str(row["branch"]),
			PackageErrors: stringList(str(row["package_errors_json"])),
			CreatedAt:     str(row["created_at"]),
		})
	}
	return out, nil
}
```

`internal/core/store/repo.go` 追加：

```go
const (
	deleteRepoSQL = `DELETE FROM repos WHERE id = ?`
	selectRepoSQL = `SELECT id, name, local_path, lang FROM repos WHERE id = ?`
)

// DeleteRepo 只发一条 DELETE：快照/元素/关系/hash/指纹/历史全靠 schema 里的
// ON DELETE CASCADE 收走（T1 的 TestV2CascadeReclaimsSlice2Rows 钉住了它）。
func (d *DB) DeleteRepo(ctx context.Context, repoID string) error {
	if _, err := d.Execute(ctx, deleteRepoSQL, repoID); err != nil {
		return fmt.Errorf("store: delete repo %s: %w", repoID, err)
	}
	return nil
}

func (d *DB) Repo(ctx context.Context, repoID string) (model.Repo, error) {
	rows, err := d.Query(ctx, selectRepoSQL, repoID)
	if err != nil {
		return model.Repo{}, err
	}
	if len(rows.Values) == 0 {
		return model.Repo{}, fmt.Errorf("store: unknown repo %q", repoID)
	}
	row := rowsToMaps(rows)[0]
	return model.Repo{ID: str(row["id"]), Name: str(row["name"]),
		LocalPath: str(row["local_path"]), Lang: str(row["lang"])}, nil
}
```

- [ ] **Step 4: 跑测试确认通过 + 全量 + 提交**

```sh
go test ./internal/core/store/ -run 'TestDeleteRepo|TestSnapshotsList|TestRepoSingleRead|TestHistoryCarries' -v
go vet ./... && go test ./...
git add internal/core/store/snapshot.go internal/core/store/repo.go internal/core/store/repo_test.go internal/core/store/history_test.go
git commit -m "feat: store 的 Snapshots 列表、Repo 单读与 DeleteRepo 级联删除"
```

---

## Phase B — core 算法

### Task 5: `graph.Merge` —— N 路并集 + 归属冲突归最长模块前缀

**Files:**
- Create: `internal/core/graph/merge.go`
- Modify: `internal/core/graph/graph.go:13-15,112`（删 `ErrNotImplemented` 与旧方法）
- Modify: `internal/core/graph/graph_test.go:125-129`（删 `TestMergeIsNotImplemented`；`errors` 在这个文件里只被它用（实测 grep），连 import 一起删）
- Modify: `internal/ui/app.go:74-115`（`Reload` 改走 Merge）
- Test: `internal/core/graph/merge_test.go`、`internal/ui/app_test.go`

**Interfaces:**
- Consumes: `Build`（已按 id 排序、已丢悬空边）、`model.PkgPathOf`、`model.SystemID`。
- Produces: `func Merge(graphs []*Graph) (*Graph, []string)`

- [ ] **Step 1: 写失败测试**

新建 `internal/core/graph/merge_test.go`：

```go
package graph

import (
	"strings"
	"testing"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

func appGraph() *Graph {
	return Build([]model.Element{
		{ID: model.SystemID("app.example/m"), Type: model.TypeSystem, Name: "m"},
		{ID: "ctn:app.example/m/cmd", Type: model.TypeContainer, Name: "cmd"},
		{ID: "cmp:app.example/m/store.DB", Type: model.TypeComponent, Name: "DB"},
	}, []model.Relationship{
		{FromID: "ctn:app.example/m/cmd", ToID: "cmp:app.example/m/store.DB", Type: model.RelCalls, Weight: 2},
		// 跨仓悬空边：端点在另一张图里，Merge 之后必须活过来（DoD-1）。
		{FromID: "cmp:app.example/m/store.DB", ToID: "cmp:dep.example/lib.Cache", Type: model.RelCalls, Weight: 5},
	})
}

func depGraph() *Graph {
	return Build([]model.Element{
		{ID: model.SystemID("dep.example/lib"), Type: model.TypeSystem, Name: "lib"},
		{ID: "cmp:dep.example/lib.Cache", Type: model.TypeComponent, Name: "Cache"},
	}, []model.Relationship{
		{FromID: model.SystemID("dep.example/lib"), ToID: "cmp:dep.example/lib.Cache", Type: model.RelContains, Weight: 1},
	})
}

func TestMergeBringsCrossRepoEdgesBack(t *testing.T) {
	g, warnings := Merge([]*Graph{appGraph(), depGraph()})
	if len(warnings) != 0 {
		t.Errorf("warnings = %v", warnings)
	}
	if len(g.Elements()) != 5 {
		t.Errorf("elements = %d, want 5", len(g.Elements()))
	}
	for _, rel := range g.Relationships() {
		if rel.FromID == "cmp:app.example/m/store.DB" && rel.ToID == "cmp:dep.example/lib.Cache" {
			return
		}
	}
	t.Error("cross-repo edge lost in merge: single-repo Build dropped it as dangling, Merge must restore it")
}

// 同一 element_id 出现在两张图（复制代码）：归属模块路径最长前缀的一方。
func TestMergeAttributesLongestModulePrefix(t *testing.T) {
	vendor := Build([]model.Element{
		{ID: model.SystemID("app.example/m"), Type: model.TypeSystem, Name: "m"},
		{ID: "cmp:app.example/m/store.DB", Type: model.TypeComponent, Name: "DB", Description: "vendored copy"},
	}, nil)
	home := Build([]model.Element{
		{ID: model.SystemID("app.example/m/store"), Type: model.TypeSystem, Name: "store"},
		{ID: "cmp:app.example/m/store.DB", Type: model.TypeComponent, Name: "DB", Description: "home copy"},
	}, nil)
	g, warnings := Merge([]*Graph{vendor, home})
	el, ok := g.Element("cmp:app.example/m/store.DB")
	if !ok {
		t.Fatal("element lost")
	}
	if el.Description != "home copy" {
		t.Errorf("description = %q, want the longer-prefix owner", el.Description)
	}
	if len(warnings) != 0 {
		t.Errorf("no warning expected for a clear prefix win: %v", warnings)
	}
}

func TestMergeTieWarnsAndKeepsFirst(t *testing.T) {
	a := Build([]model.Element{{ID: "cmp:x.A", Type: model.TypeComponent, Name: "A", Description: "first"}}, nil)
	b := Build([]model.Element{{ID: "cmp:x.A", Type: model.TypeComponent, Name: "A", Description: "second"}}, nil)
	g, warnings := Merge([]*Graph{a, b})
	if len(warnings) != 1 || !strings.Contains(warnings[0], "cmp:x.A") {
		t.Fatalf("warnings = %v", warnings)
	}
	if el, _ := g.Element("cmp:x.A"); el.Description != "first" {
		t.Errorf("tie must keep the earlier graph, got %q", el.Description)
	}
}

// 同一条 (from,to,type) 在两仓各存一次（跨仓重复扫描的必然结果）：并集里只能
// 有一条，权重取大——Build 会把重复邻接算成两次入度，rank 就被污染。
func TestMergeDedupesEdgesByMaxWeight(t *testing.T) {
	e1 := Build([]model.Element{{ID: "cmp:p.A", Type: model.TypeComponent}, {ID: "cmp:p.B", Type: model.TypeComponent}},
		[]model.Relationship{{FromID: "cmp:p.A", ToID: "cmp:p.B", Type: model.RelCalls, Weight: 3}})
	e2 := Build([]model.Element{{ID: "cmp:p.A", Type: model.TypeComponent}, {ID: "cmp:p.B", Type: model.TypeComponent}},
		[]model.Relationship{{FromID: "cmp:p.A", ToID: "cmp:p.B", Type: model.RelCalls, Weight: 7}})
	g, _ := Merge([]*Graph{e1, e2})
	rels := g.Relationships()
	if len(rels) != 1 {
		t.Fatalf("relationships = %+v, want one deduped", rels)
	}
	if rels[0].Weight != 7 {
		t.Errorf("weight = %d, want max 7", rels[0].Weight)
	}
}

func TestMergeNilAndEmptyGraphsAreSafe(t *testing.T) {
	g, warnings := Merge([]*Graph{nil, Build(nil, nil)})
	if len(g.Elements()) != 0 || len(g.Relationships()) != 0 || len(warnings) != 0 {
		t.Errorf("empty merge = %+v / %v", g.Elements(), warnings)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

```sh
go test ./internal/core/graph/ -run TestMerge -v
```

期望：编译失败 `undefined: Merge`。

- [ ] **Step 3: 实现 `merge.go`**

```go
package graph

import (
	"fmt"
	"sort"
	"strings"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

// Merge 是 N 路并集：元素按 element_id 去重、边按 (from,to,type) 去重取最大
// 权重、端点齐了的边才留（Build 免费给）。跨仓边在单仓 Build 里因端点缺失被
// 丢掉，并集里端点在场，于是活过来——spec §7 与 DoD-1 的机制。
//
// 归属规则（spec §7）：同一 id 被多张图声称时，模块路径是元素路径最长前缀的
// 一方胜出；平局取输入顺序靠前者并产一条 warning。图的模块路径来自它唯一的
// System 元素（`sys:<modulePath>`，切片 1 spec §6.3）。
func Merge(graphs []*Graph) (*Graph, []string) {
	type claim struct {
		el     model.Element
		module string
	}
	byID := map[string]claim{}
	// 每个模块路径贡献几条元素，留给归属判断用（一张图 = 一个模块路径）。
	modules := make([]string, 0, len(graphs))
	for _, g := range graphs {
		if g == nil {
			continue
		}
		m := g.modulePath()
		modules = append(modules, m)
		for _, el := range g.Elements() {
			score := prefixLen(m, elementPath(el.ID))
			prev, ok := byID[el.ID]
			if !ok {
				byID[el.ID] = claim{el, m}
				continue
			}
			switch {
			case score > prefixLen(prev.module, elementPath(el.ID)):
				byID[el.ID] = claim{el, m}
			case score == prefixLen(prev.module, elementPath(el.ID)) && score > 0 && prev.module != m:
				byID[el.ID] = claim{prev.el, prev.module}
			}
			if score == prefixLen(prev.module, elementPath(el.ID)) && prev.module != m {
				byID[el.ID] = claim{prev.el, prev.module}
			}
		}
	}

	elements := make([]model.Element, 0, len(byID))
	for _, c := range byID {
		elements = append(elements, c.el)
	}
	sort.Slice(elements, func(i, j int) bool { return elements[i].ID < elements[j].ID })

	type edgeKey struct{ from, to, typ string }
	best := map[edgeKey]int{}
	order := make([]edgeKey, 0)
	for _, g := range graphs {
		if g == nil {
			continue
		}
		for _, rel := range g.Relationships() {
			k := edgeKey{rel.FromID, rel.ToID, string(rel.Type)}
			if _, seen := best[k]; !seen {
				order = append(order, k)
			}
			if rel.Weight > best[k] {
				best[k] = rel.Weight
			}
		}
	}
	rels := make([]model.Relationship, 0, len(order))
	for _, k := range order {
		rels = append(rels, model.Relationship{FromID: k.from, ToID: k.to,
			Type: model.RelType(k.typ), Weight: best[k]})
	}
	warnings := claimWarnings(graphs)
	return Build(elements, rels), warnings
}

// elementPath 是元素 id 里参与最长前缀比较的那段路径：ctn/cmp 用包路径，
// sys 用模块路径本身。
func elementPath(id string) string {
	if p := model.PkgPathOf(id); p != "" {
		return p
	}
	return strings.TrimPrefix(id, "sys:")
}

// prefixLen 返回 module 作为 path 前缀的字节长度，不是前缀则 0。
func prefixLen(module, path string) int {
	if module == "" {
		return 0
	}
	if path == module || strings.HasPrefix(path, module+"/") {
		return len(module)
	}
	if id := strings.TrimPrefix(path, "sys:"); id == module {
		return len(module)
	}
	return 0
}

// modulePath 从图里唯一的 System 元素反推模块路径；没有元素时 ""。
func (g *Graph) modulePath() string {
	for _, el := range g.Elements() {
		if el.Type == model.TypeSystem {
			return strings.TrimPrefix(el.ID, "sys:")
		}
	}
	return ""
}

// claimWarnings 单独一遍：只报真正冲突的 id，平局才警告。
func claimWarnings(graphs []*Graph) []string {
	owners := map[string][]string{}
	for _, g := range graphs {
		if g == nil {
			continue
		}
		m := g.modulePath()
		for _, el := range g.Elements() {
			owners[el.ID] = append(owners[el.ID], m)
		}
	}
	var out []string
	for id, mods := range owners {
		if len(mods) < 2 {
			continue
		}
		score := prefixLen(mods[0], elementPath(id))
		tied := true
		for _, m := range mods[1:] {
			if prefixLen(m, elementPath(id)) != score {
				tied = false
				break
			}
		}
		if tied && score > 0 {
			out = append(out, fmt.Sprintf("graph: element %s claimed by %s and %s; kept %s",
				id, strings.Join(mods, ", "), mods[0], mods[0]))
		}
	}
	sort.Strings(out)
	return out
}
```

> 上面 `Merge` 的归属分支写得啰嗦是有意的：**先比较得分、再单独收集 warnings**，两遍分开比一遍里既判归属又判警告更容易写错。执行者若把它简化成单遍，必须仍然让 `TestMergeTieWarnsAndKeepsFirst` 与 `TestMergeAttributesLongestModulePrefix` 同时绿。

- [ ] **Step 4: 删旧钩子**

`internal/core/graph/graph.go`：删 `ErrNotImplemented`（:13-15）与 `func (g *Graph) Merge(other *Graph) error`（:112）。全仓 `grep -rn ErrNotImplemented` 必须只剩零结果。实测引用只有两处，都在 graph 包内：定义处与 `graph_test.go:125-129` 的 `TestMergeIsNotImplemented`——**产品代码没有调用方**，所以这次删除不改任何行为；测试函数连同它的 `"errors"` import 一起删（那条测试钉的是「切片 2 会来实现」这个占位承诺，现在承诺兑现了）。

- [ ] **Step 5: `App.Reload` 改走 Merge**

`internal/ui/app.go`：把 `:74-115` 的「手动 append 元素/关系再 Build」换成逐仓建图 + `graph.Merge`，`a.snapshot`/`pkgErrs` 的聚合语义保持不变：

```go
	} else {
		var (
			graphs  []*graph.Graph
			pkgErrs []string
			first   model.ArchitectureSnapshot
		)
		for _, r := range repos {
			id := string(r.ID)
			if !selected[id] {
				continue
			}
			snapshot, err := a.db.Snapshot(ctx, latest[id])
			if err != nil {
				return err
			}
			el, err := a.db.Elements(ctx, latest[id])
			if err != nil {
				return err
			}
			rl, err := a.db.Relationships(ctx, latest[id])
			if err != nil {
				return err
			}
			graphs = append(graphs, graph.Build(el, rl))
			pkgErrs = append(pkgErrs, snapshot.PackageErrors...)
			if first.ID == "" {
				first = snapshot
			}
		}
		merged, warnings := graph.Merge(graphs)
		a.mergeWarnings = warnings
		first.PackageErrors = pkgErrs
		a.snapshot = first
		if len(selected) == 1 {
			for id := range selected {
				a.repoID = id
			}
		} else {
			a.repoID = ""
		}
		a.graph = merged
	}
```

`internal/ui/state.go` 的 `App` 加一个字段（图页摘要行 T25 会显示它；本任务先只存不用，但状态必须存在，否则 Reload 算出来的警告无处落）：

```go
	// mergeWarnings 是本次 Reload 里 Merge 报出的归属冲突（同 id 元素被两仓
	// 平局声称）。非空时图页状态行加一句「N 个元素归属存疑」——复制代码/vendored
	// 的警告必须让用户看见，否则最长前缀规则只是在暗处改图。
	mergeWarnings []string
```

- [ ] **Step 6: GUI 计数回归测试**

`internal/ui/app_test.go` 追加一条（装配用该文件里已有的三个 helper：`newTestApp(t)`（`app_test.go:19`，开临时库 + `t.Cleanup(Close)`，返回 `*App`，**测试里不要再 `defer db.Close()`**）、`fixtureDir(name)`（`:29`，相对路径 `../../testdata/fixtures/<name>`）、`seedScan(t, a, root)`（`:33`，往 `a.db` 扫一次并断言非空，返回 `scanner.Report`））：

```go
// 合并图计数 = 两仓之和 − 重叠：Merge 接管并集后，这条从「构造上成立」变成
// 「被测出来的」。切片 1 的 Reload 手写并集时它是巧合。
func TestReloadMergesTwoRepos(t *testing.T) {
	ctx := context.Background()
	app := newTestApp(t)
	dep := seedScan(t, app, fixtureDir("crossmod-dep"))
	appReport := seedScan(t, app, fixtureDir("crossmod-app"))
	if err := app.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	appIDs := snapshotElementCount(ctx, t, app.db, appReport.SnapshotID)
	depIDs := snapshotElementCount(ctx, t, app.db, dep.SnapshotID)
	got := len(app.graph.Elements())
	if got > appIDs+depIDs || got < max(appIDs, depIDs) {
		t.Errorf("merged elements = %d, want in [%d, %d]", got, max(appIDs, depIDs), appIDs+depIDs)
	}
	if len(app.mergeWarnings) != 0 {
		t.Errorf("两仓模块路径不同，不该有归属冲突: %v", app.mergeWarnings)
	}
	// DoD-1 的 GUI 半边：跨仓边在合并视图里必须在场。
	for _, rel := range app.graph.Relationships() {
		if strings.HasPrefix(rel.FromID, "cmp:crossmod.example/app") &&
			strings.HasPrefix(rel.ToID, "cmp:crossmod.example/dep") {
			return
		}
	}
	t.Error("merged view lost the cross-repo edge")
}

// snapshotElementCount 读一个快照的元素数——两仓各自扫描时的口径，
// 用来给并集画上下界（并集 ≤ 之和，且 ≥ 较大的一边）。
func snapshotElementCount(ctx context.Context, t *testing.T, db *store.DB,
	snapshotID string) int {
	t.Helper()
	elts, err := db.Elements(ctx, snapshotID)
	if err != nil {
		t.Fatal(err)
	}
	return len(elts)
}
```

> 助手就这三个名字（实测 `internal/ui` 包里没有 `newTestDB`/`scanFixtureRepo`/`fixtureElementIDs`，那几个是草稿想当然写的）；`store` 已在 `app_test.go` 的 import 块里，`context/strings/testing` 同样已存在，新增的 import 只有可能需要 `context`——它已在。双仓装配是这条测试新带的，之前 `app_test.go` 只有单仓。

- [ ] **Step 7: 全绿 + 提交**

```sh
go test ./internal/core/graph/ ./internal/ui/ -run 'TestMerge|TestReload' -v
go vet ./... && go test ./...
git add internal/core/graph/merge.go internal/core/graph/merge_test.go internal/core/graph/graph.go internal/ui/app.go internal/ui/state.go internal/ui/app_test.go
git commit -m "feat: graph.Merge 做 N 路并集与跨仓边复活，Reload 改走它"
```

---

### Task 6: `graph.Diff` —— 两个快照的元素与边差异

**Files:**
- Create: `internal/core/graph/diff.go`
- Test: `internal/core/graph/diff_test.go`

**Interfaces:**
- Consumes: `Graph`（`Elements()` 已按 id 排序、`Relationships()` 已按 (from,to,type) 排序）。
- Produces:

```go
type Diff struct {
	Added        []model.Element
	Removed      []model.Element
	Changed      []ChangedElement
	EdgesAdded   []model.Relationship
	EdgesRemoved []model.Relationship
}
type ChangedElement struct {
	From, To model.Element
	Fields   []string
}
func DiffSnapshots(a, b *Graph) Diff
func ChangedFields(a, b model.Element) []string
```

（spec §8 点名 `DiffSnapshots`，本计划保留这个名字——它同时被 CLI、GUI 和历史事件三处消费，名字比签名更值得稳定。）

- [ ] **Step 1: 写失败测试**

```go
package graph

import (
	"reflect"
	"testing"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

func snapshotGraphA() *Graph {
	return Build([]model.Element{
		{ID: "cmp:p.A", Type: model.TypeComponent, Name: "A", Description: "old doc", Methods: []string{"X"}},
		{ID: "cmp:p.Gone", Type: model.TypeComponent, Name: "Gone"},
	}, []model.Relationship{
		{FromID: "cmp:p.A", ToID: "cmp:p.Gone", Type: model.RelCalls, Weight: 1},
	})
}

func snapshotGraphB() *Graph {
	return Build([]model.Element{
		{ID: "cmp:p.A", Type: model.TypeComponent, Name: "A", Description: "new doc", Methods: []string{"X", "Y"}},
		{ID: "cmp:p.New", Type: model.TypeComponent, Name: "New"},
	}, []model.Relationship{
		{FromID: "cmp:p.A", ToID: "cmp:p.New", Type: model.RelCalls, Weight: 2},
	})
}

func TestDiffSnapshotsElements(t *testing.T) {
	d := DiffSnapshots(snapshotGraphA(), snapshotGraphB())
	if len(d.Added) != 1 || d.Added[0].ID != "cmp:p.New" {
		t.Errorf("Added = %+v", d.Added)
	}
	if len(d.Removed) != 1 || d.Removed[0].ID != "cmp:p.Gone" {
		t.Errorf("Removed = %+v", d.Removed)
	}
	if len(d.Changed) != 1 {
		t.Fatalf("Changed = %+v, want exactly cmp:p.A", d.Changed)
	}
	if !reflect.DeepEqual(d.Changed[0].Fields, []string{"description", "methods_json"}) {
		t.Errorf("Changed fields = %v, want [description methods_json] in column order", d.Changed[0].Fields)
	}
}

func TestDiffSnapshotsEdges(t *testing.T) {
	d := DiffSnapshots(snapshotGraphA(), snapshotGraphB())
	if len(d.EdgesRemoved) != 1 || d.EdgesRemoved[0].ToID != "cmp:p.Gone" {
		t.Errorf("EdgesRemoved = %+v", d.EdgesRemoved)
	}
	if len(d.EdgesAdded) != 1 || d.EdgesAdded[0].ToID != "cmp:p.New" {
		t.Errorf("EdgesAdded = %+v", d.EdgesAdded)
	}
}

// 三元组是边的身份，权重不是：A→B calls 从 1 次变 2 次不算「一条边被删了再加」。
// 排序/集合语义在这里钉死，历史事件（T12）和 GUI 着色（T20）都吃这份判定。
func TestDiffSnapshotsIgnoresWeight(t *testing.T) {
	a := Build([]model.Element{{ID: "cmp:p.A", Type: model.TypeComponent}, {ID: "cmp:p.B", Type: model.TypeComponent}},
		[]model.Relationship{{FromID: "cmp:p.A", ToID: "cmp:p.B", Type: model.RelCalls, Weight: 1}})
	b := Build([]model.Element{{ID: "cmp:p.A", Type: model.TypeComponent}, {ID: "cmp:p.B", Type: model.TypeComponent}},
		[]model.Relationship{{FromID: "cmp:p.A", ToID: "cmp:p.B", Type: model.RelCalls, Weight: 9}})
	if d := DiffSnapshots(a, b); len(d.EdgesAdded)+len(d.EdgesRemoved) != 0 {
		t.Errorf("weight change must not add/remove edges: %+v", d)
	}
}

func TestChangedFieldsStableOrder(t *testing.T) {
	base := model.Element{ID: "cmp:p.A", Type: model.TypeComponent, Name: "A",
		Description: "d", FilePath: "a.go", Methods: []string{"X"}}
	moved := base
	moved.Type = model.TypeContainer
	moved.FilePath = "b.go"
	moved.Description = "d2"
	moved.Methods = []string{"X", "Y"}
	want := []string{"element_type", "description", "file_path", "methods_json"}
	if got := ChangedFields(base, moved); !reflect.DeepEqual(got, want) {
		t.Errorf("ChangedFields = %v, want %v", got, want)
	}
	if ChangedFields(base, base) != nil {
		t.Error("identical elements must report no fields")
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

```sh
go test ./internal/core/graph/ -run 'TestDiff|TestChangedFields' -v
```

期望：编译失败 `undefined: DiffSnapshots`。

- [ ] **Step 3: 实现 `diff.go`**

```go
package graph

import (
	"slices"
	"strings"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

// Diff 是「b 相对 a 变了什么」。GUI 着色、`oac diff`、历史事件三处共用这一个
// 实现（spec §8：一个判定只写一次）。
type Diff struct {
	Added        []model.Element
	Removed      []model.Element
	Changed      []ChangedElement
	EdgesAdded   []model.Relationship
	EdgesRemoved []model.Relationship
}

type ChangedElement struct {
	From, To model.Element
	Fields   []string
}

// Empty 是摘要行的省事判据。
func (d Diff) Empty() bool {
	return len(d.Added)+len(d.Removed)+len(d.Changed)+len(d.EdgesAdded)+len(d.EdgesRemoved) == 0
}

type relKey struct{ from, to, typ string }

func key(r model.Relationship) relKey { return relKey{r.FromID, r.ToID, string(r.Type)} }

func DiffSnapshots(a, b *Graph) Diff {
	var d Diff
	inA := map[string]model.Element{}
	for _, el := range a.Elements() {
		inA[el.ID] = el
	}
	inB := map[string]model.Element{}
	for _, el := range b.Elements() {
		inB[el.ID] = el
	}
	for _, el := range b.Elements() {
		prev, ok := inA[el.ID]
		switch {
		case !ok:
			d.Added = append(d.Added, el)
		default:
			if fields := ChangedFields(prev, el); len(fields) > 0 {
				d.Changed = append(d.Changed, ChangedElement{From: prev, To: el, Fields: fields})
			}
		}
	}
	for _, el := range a.Elements() {
		if _, ok := inB[el.ID]; !ok {
			d.Removed = append(d.Removed, el)
		}
	}
	seenB := map[relKey]bool{}
	for _, rel := range b.Relationships() {
		seenB[key(rel)] = true
	}
	seenA := map[relKey]bool{}
	for _, rel := range a.Relationships() {
		seenA[key(rel)] = true
		if !seenB[key(rel)] {
			d.EdgesRemoved = append(d.EdgesRemoved, rel)
		}
	}
	for _, rel := range b.Relationships() {
		if !seenA[key(rel)] {
			d.EdgesAdded = append(d.EdgesAdded, rel)
		}
	}
	return d
}

// changedColumnOrder 是 detail_json 里字段名的输出顺序，逐字对应
// arch_elements 的列名（spec §8）。定序是为了让历史行可字符串比对——
// map 迭代序会把平价测试变成 flake。
var changedColumnOrder = []string{"element_type", "description", "file_path", "methods_json"}

func ChangedFields(a, b model.Element) []string {
	var out []string
	for _, col := range changedColumnOrder {
		switch col {
		case "element_type":
			if a.Type != b.Type {
				out = append(out, col)
			}
		case "description":
			if a.Description != b.Description {
				out = append(out, col)
			}
		case "file_path":
			if a.FilePath != b.FilePath {
				out = append(out, col)
			}
		case "methods_json":
			if !slices.Equal(a.Methods, b.Methods) {
				out = append(out, col)
			}
		}
	}
	return out
}

// Name 字段故意不参与判定：改名意味着换了类型，那在稳定 ID 里已经是
// 消失 + 出现（`cmp:<pkg>.<Name>` 以名字为键），报成 changed 会掩盖真实影响。
```

`diff.go` 不 import `strings`（上面这份代码没有用到它，加了就编译失败）。

- [ ] **Step 4: 全绿 + 提交**

```sh
go test ./internal/core/graph/ -run 'TestDiff|TestChangedFields' -v
go vet ./... && go test ./...
git add internal/core/graph/diff.go internal/core/graph/diff_test.go
git commit -m "feat: graph.DiffSnapshots —— 元素与边的快照差异，历史与对比着色的唯一判定源"
```

---

### Task 7: `graph.RankDependents` —— load-bearing 排序

**Files:**
- Create: `internal/core/graph/rank.go`
- Test: `internal/core/graph/rank_test.go`

**Interfaces:**
- Consumes: `Graph.Relationships()`、`model.RelCalls`/`model.RelDependsOn`。
- Produces: `type RankHit struct { Element model.Element; Dependents int }` + `func RankDependents(g *Graph) []RankHit`（分数降序、并列按 id 升序）

- [ ] **Step 1: 写失败测试**

```go
package graph

import (
	"testing"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

// 计划定稿 U4：只算 calls + depends_on 的直接入度，contains 不算。否则每个
// Component 都从所属 Container 白拿 1 分，排序退化成元素类型分组。
func TestRankDependentsCountsOnlyDiffusingEdges(t *testing.T) {
	g := Build([]model.Element{
		{ID: model.SystemID("m"), Type: model.TypeSystem, Name: "m"},
		{ID: "ctn:m/store", Type: model.TypeContainer, Name: "store"},
		{ID: "cmp:m/store.DB", Type: model.TypeComponent, Name: "DB"},
		{ID: "cmp:m/a.A", Type: model.TypeComponent, Name: "A"},
		{ID: "cmp:m/b.B", Type: model.TypeComponent, Name: "B"},
	}, []model.Relationship{
		{FromID: model.SystemID("m"), ToID: "ctn:m/store", Type: model.RelContains},
		{FromID: "ctn:m/store", ToID: "cmp:m/store.DB", Type: model.RelContains},
		{FromID: "cmp:m/a.A", ToID: "cmp:m/store.DB", Type: model.RelCalls},
		{FromID: "cmp:m/b.B", ToID: "cmp:m/store.DB", Type: model.RelDependsOn},
		{FromID: "cmp:m/a.A", ToID: "cmp:m/b.B", Type: model.RelCalls},
	})
	hits := RankDependents(g)
	if hits[0].Element.ID != "cmp:m/store.DB" || hits[0].Dependents != 2 {
		t.Fatalf("top = %+v, want cmp:m/store.DB with 2", hits[0])
	}
	if hits[1].Element.ID != "cmp:m/b.B" || hits[1].Dependents != 1 {
		t.Errorf("second = %+v", hits[1])
	}
	for _, h := range hits {
		if h.Element.Type == model.TypeContainer {
			t.Errorf("contains 边不该给 Container 记分: %+v", h)
		}
	}
}

// 同一个上游算一次：A 里 5 处调 B，B 的 dependents 是 1，不是 5。
// 观察库基线（OpenLaw store.DB 入度 20）用的就是这个口径。
func TestRankDependentsDistinctSources(t *testing.T) {
	g := Build([]model.Element{
		{ID: "cmp:m/a.A", Type: model.TypeComponent, Name: "A"},
		{ID: "cmp:m/b.B", Type: model.TypeComponent, Name: "B"},
	}, []model.Relationship{
		{FromID: "cmp:m/a.A", ToID: "cmp:m/b.B", Type: model.RelCalls, Weight: 5},
	})
	hits := RankDependents(g)
	if len(hits) != 1 || hits[0].Dependents != 1 {
		t.Errorf("hits = %+v, want one with Dependents 1", hits)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

```sh
go test ./internal/core/graph/ -run TestRank -v
```

期望：编译失败 `undefined: RankDependents`。

- [ ] **Step 3: 实现**

```go
package graph

import (
	"sort"
	"strings"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

// RankHit 是一个元素的 load-bearing 分数。
type RankHit struct {
	Element    model.Element
	Dependents int
}

// RankDependents 直接入度（distinct 上游元素数），只算 calls 与 depends_on——
// 与 bfs.go 的 diffuses() 白名单同一套语义，所以「rank 高」和「影响面大」
// 在定义上是一致的。零分的元素不出现在结果里（`oac rank --limit 10` 的
// 10 行应该全是有效信息）。
func RankDependents(g *Graph) []RankHit {
	scores := map[string]map[string]bool{}
	for _, rel := range g.Relationships() {
		if rel.Type != model.RelCalls && rel.Type != model.RelDependsOn {
			continue
		}
		if _, ok := scores[rel.ToID]; !ok {
			scores[rel.ToID] = map[string]bool{}
		}
		scores[rel.ToID][rel.FromID] = true
	}
	out := make([]RankHit, 0, len(scores))
	for id, deps := range scores {
		el, ok := g.Element(id)
		if !ok {
			continue
		}
		out = append(out, RankHit{Element: el, Dependents: len(deps)})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Dependents != out[j].Dependents {
			return out[i].Dependents > out[j].Dependents
		}
		return strings.Compare(out[i].Element.ID, out[j].Element.ID) < 0
	})
	return out
}
```

- [ ] **Step 4: 全绿 + 提交**

```sh
go test ./internal/core/graph/ -run TestRank -v
go vet ./... && go test ./...
git add internal/core/graph/rank.go internal/core/graph/rank_test.go
git commit -m "feat: graph.RankDependents —— calls+depends_on 的直接入度排序"
```

---

### Task 8: 适配器上报「包 → 文件 + 第一方直接 import」

**Files:**
- Modify: `internal/scanner/language/adapter.go`
- Modify: `internal/scanner/language/go.go`
- Test: `internal/scanner/language/go_packages_test.go`

**Interfaces:**
- Consumes: `packages.Package` 的 `PkgPath`/`GoFiles`/`Imports`、既有的 `ownPackage`（:178）与 `isTestVariant`、`relativeFile` 的相对路径口径（:187）。
- Produces:

```go
type PackageSource struct {
	PkgPath string   // 第一方 import path
	Files   []string // 仓库根相对的 .go 文件，升序
	Imports []string // 第一方的直接 import，升序
}
// Output 增字段（不改现有三个字段语义）：
//   Packages []PackageSource  // 升序按 PkgPath；main 包也在里面
```

- [ ] **Step 1: 写失败测试**

```go
package language

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func fixtureRoot(t *testing.T, name string) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "..", "testdata", "fixtures", name))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("fixture %s: %v", name, err)
	}
	return root
}

// arch_files 与 Merkle 指纹都建立在这份数据上，所以它的三条口径必须钉死：
// 路径是仓库根相对的（与 Element.FilePath 同规则）、只含第一方非测试包、
// import 只留第一方直接依赖。
func TestGoAdapterReportsPackageSources(t *testing.T) {
	out, err := goAdapter{}.Parse(context.Background(), fixtureRoot(t, "shared-lib"))
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Packages) == 0 {
		t.Fatal("Packages empty: 指纹没有输入")
	}
	seen := map[string]PackageSource{}
	for _, p := range out.Packages {
		seen[p.PkgPath] = p
	}
	core, ok := seen["example.com/shared/core"]
	if !ok {
		t.Fatalf("pkg paths = %v", keys(seen))
	}
	// shared-lib 的 core 包实测只有一个文件（testdata/fixtures/shared-lib/core/core.go），
	// 所以这里钉精确值而不是「非空」。写成 `!DeepEqual(...) && len(...)==0` 是
	// 一个只会「非空即通过」的假断言：文件名错了、路径没转相对，它都不响。
	if !reflect.DeepEqual(core.Files, []string{"core/core.go"}) {
		t.Errorf("Files = %v, want [core/core.go]", core.Files)
	}
	for _, f := range core.Files {
		if filepath.IsAbs(f) {
			t.Errorf("absolute path in arch_files source: %q", f)
		}
	}
	for _, imp := range core.Imports {
		if _, own := seen[imp]; !own {
			t.Errorf("Imports must be first-party only, got %q", imp)
		}
	}
	if !sort.SliceIsSorted(out.Packages, func(i, j int) bool {
		return out.Packages[i].PkgPath < out.Packages[j].PkgPath
	}) {
		t.Errorf("Packages must be sorted by PkgPath (golden 比对纪律): %v", keys(seen))
	}
}

func TestGoAdapterCrossModuleImportsAreExcluded(t *testing.T) {
	out, err := goAdapter{}.Parse(context.Background(), fixtureRoot(t, "crossmod-app"))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range out.Packages {
		for _, imp := range p.Imports {
			if imp == "crossmod.example/dep/store" {
				t.Errorf("%s leaked a foreign import into PackageSource", p.PkgPath)
			}
		}
	}
}
```

`keys` 是测试内唯一的辅助（写在同一文件，别放进产品包）：

```go
func keys(m map[string]PackageSource) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

```

> 有序性用 `sort.SliceIsSorted` 直接读被测切片，而不是「排一遍再 DeepEqual」：后者要两个几乎同名的辅助函数，且当两个函数退化成同一个实现时断言自动为真——它测的是自己。比较用 `<` 而不是 `<=`：`PkgPath` 由 `Merge`/`Build` 的去重保证唯一，写 `<=` 会让重复项蒙过去。

- [ ] **Step 2: 跑测试确认失败**

```sh
go test ./internal/scanner/language/ -run 'TestGoAdapterReports|TestGoAdapterCrossModule' -v
```

期望：编译失败 `out.Packages undefined`。

- [ ] **Step 3: 扩展 `language.Output`**

`internal/scanner/language/adapter.go`：

```go
// PackageSource 是适配器为一个第一方包读到的输入清单——指纹的原料，不是图数据。
// Files 与 Element.FilePath 同规则（仓库根相对），Imports 只含第一方的直接
// import（第三方被 go.mod 版本 pin 住，由哨兵表达）。
type PackageSource struct {
	PkgPath string
	Files   []string
	Imports []string
}

type Output struct {
	ModulePath    string
	Elements      []model.Element
	Relationships []model.Relationship
	PackageErrors []string
	// Packages 按 PkgPath 升序；go/packages 不保证切片顺序，适配器必须自己排。
	Packages []PackageSource
}
```

- [ ] **Step 4: go 适配器填充**

`internal/scanner/language/go.go`：在 `Parse` 的主循环里（`for _, pkg := range pkgs`，:79）把 `continue` 之前的过滤复用一遍，收集到循环外的切片；循环后排序并挂到 `out`：

```go
	var sources []PackageSource
	for _, pkg := range pkgs {
		for _, e := range pkg.Errors {
			packageErrors = append(packageErrors, fmt.Sprintf("%s: %s", pkg.PkgPath, e))
		}
		if isTestVariant(pkg.ID) || !ownPackage(modulePath, pkg.PkgPath) {
			continue
		}
		sources = append(sources, PackageSource{
			PkgPath: pkg.PkgPath,
			Files:   relativeFilePaths(abs, pkg.GoFiles),
			Imports: firstPartyImports(modulePath, pkg.Imports),
		})
		// …… 原有 main/component 分支不动 ……
	}
```

`out` 组装处（:115）加一行：

```go
	out := Output{ModulePath: modulePath, Elements: elements, PackageErrors: packageErrors,
		Packages: sources}
	slices.SortFunc(out.Packages, func(a, b PackageSource) int { return strings.Compare(a.PkgPath, b.PkgPath) })
```

两个新辅助函数放 `go.go` 尾部（与 `relativeFile` 相邻，口径同源）：

```go
// relativeFilePaths 把 pkg.GoFiles（绝对路径）换成仓库根相对路径并升序。
// 与 relativeFile 同一条规则，这样 arch_files.file_path 和
// arch_elements.file_path 能用同一个字符串拼回磁盘。
func relativeFilePaths(root string, files []string) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		rel, err := filepath.Rel(root, f)
		if err != nil {
			continue // 不在仓库里的生成文件不进 hash 清单
		}
		out = append(out, filepath.ToSlash(rel))
	}
	slices.Sort(out)
	return out
}

// firstPartyImports 只留本模块的直接依赖：第三方的变更由 fp(go.mod) 哨兵
// 表达，不必进 Merkle 递归。
func firstPartyImports(modulePath string, imports []*packages.Package) []string {
	out := make([]string, 0, len(imports))
	for _, imp := range imports {
		if imp != nil && ownPackage(modulePath, imp.PkgPath) {
			out = append(out, imp.PkgPath)
		}
	}
	slices.Sort(out)
	return out
}
```

- [ ] **Step 5: 跑测试确认通过 + 全量 + 提交**

```sh
go test ./internal/scanner/... -v -run 'TestGoAdapter'
go vet ./... && go test ./...
git add internal/scanner/language/adapter.go internal/scanner/language/go.go internal/scanner/language/go_packages_test.go
git commit -m "feat: Go 适配器上报每包的文件清单与第一方直接 import"
```

---

### Task 9: Merkle 包指纹（纯函数）

**Files:**
- Create: `internal/scanner/fingerprint/fingerprint.go`
- Test: `internal/scanner/fingerprint/fingerprint_test.go`

**Interfaces:**
- Consumes: `language.PackageSource`（T8）。
- Produces: `func Compute(files map[string]string, sentinel string, pkgs []language.PackageSource) map[string]string`（pkgPath → hex SHA-256；无 error——纯哈希，无 IO）

**指纹定义（spec §6.2，逐字实现）**：

```
fp(P) = H( "f <path> <sha>\n"…P.Files 升序
        ‖ "d <path> <fp(Q)>\n"…P.Imports ∩ 第一方，按 path 升序
        ‖ "m <sentinel>\n" )
sentinel = H(go.mod 内容 ‖ go.sum 内容（无则空）)
```

- [ ] **Step 1: 写失败测试**

```go
package fingerprint

import (
	"strings"
	"testing"

	"github.com/futuremeng/OpenArchitect/internal/scanner/language"
)

func TestComputeIsContentSensitive(t *testing.T) {
	pkgs := []language.PackageSource{
		{PkgPath: "m/a", Files: []string{"a.go"}, Imports: []string{"m/b"}},
		{PkgPath: "m/b", Files: []string{"b.go"}},
	}
	files := map[string]string{"a.go": "h1", "b.go": "h2"}
	base := Compute(files, "sent", pkgs)

	// 自身内容变 → 自身变。
	b1 := Compute(map[string]string{"a.go": "h1!", "b.go": "h2"}, "sent", pkgs)
	if base["m/a"] == b1["m/a"] {
		t.Error("own file change must change fp")
	}
	// 依赖内容变 → 依赖方变（这就是文件级增量不成立、包级 Merkle 成立的点）。
	b2 := Compute(map[string]string{"a.go": "h1", "b.go": "h2!"}, "sent", pkgs)
	if base["m/a"] == b2["m/a"] {
		t.Error("dependency change must change dependent fp")
	}
	if base["m/b"] == b2["m/b"] {
		t.Error("dependency change must not change an unrelated package")
	}
	// go.mod 变 → 全体变（哨兵进每个 fp）。
	b3 := Compute(files, "sent!", pkgs)
	for p := range base {
		if base[p] == b3[p] {
			t.Errorf("sentinel change must change every fp (%s unchanged)", p)
		}
	}
}

func TestComputeIsOrderStable(t *testing.T) {
	// 输入切片的顺序、以及已排序 Files/Imports 的等价表示都不能改变结果：
	// 平价测试（T13）比的是字符串，指纹不能是「同一份代码两个哈希」。
	pkgs := []language.PackageSource{
		{PkgPath: "m/a", Files: []string{"a.go", "b.go"}, Imports: []string{"m/b"}},
		{PkgPath: "m/b", Files: []string{"c.go"}},
	}
	files := map[string]string{"a.go": "1", "b.go": "2", "c.go": "3"}
	got := Compute(files, "s", pkgs)
	rev := Compute(files, "s", []language.PackageSource{pkgs[1], pkgs[0]})
	for p, fp := range got {
		if rev[p] != fp {
			t.Errorf("fp(%s) depends on slice order", p)
		}
	}
}

func TestComputeIgnoresForeignImports(t *testing.T) {
	own := []language.PackageSource{{PkgPath: "m/a", Files: []string{"a.go"}, Imports: []string{"m/absent"}}}
	files := map[string]string{"a.go": "1"}
	// 第一方清单里没有 m/absent：它不参与递归，结果必须与「没有这条 import」一致。
	noImp := []language.PackageSource{{PkgPath: "m/a", Files: []string{"a.go"}}}
	if Compute(files, "s", own)["m/a"] != Compute(files, "s", noImp)["m/a"] {
		t.Error("a foreign/unscanned import must not perturb the fp of a first-party package")
	}
}

func TestComputeFramingIsUnambiguous(t *testing.T) {
	// 反例形状：只用空格/换行分帧时，「删掉 b.go，同时把 `1\nf b.go 2` 写进
	// a.go」会和「a.go=1、b.go=2」喂进哈希同一串字节。长度前缀是为了让这种
	// 歧义不可能出现——平价红线（T13）靠的就是「内容变了指纹必变」。
	two := []language.PackageSource{{PkgPath: "m/a", Files: []string{"a.go", "b.go"}}}
	one := []language.PackageSource{{PkgPath: "m/a", Files: []string{"a.go"}}}
	gotTwo := Compute(map[string]string{"a.go": "1", "b.go": "2"}, "s", two)["m/a"]
	crafted := map[string]string{"a.go": "1\nf b.go 2"}
	if gotOne := Compute(crafted, "s", one)["m/a"]; gotOne == gotTwo {
		t.Errorf("framing ambiguous: %s == %s（长度前缀没写进哈希）", gotOne, gotTwo)
	}
}

func TestComputeHexShape(t *testing.T) {
	got := Compute(map[string]string{"a.go": "1"}, "s",
		[]language.PackageSource{{PkgPath: "m/a", Files: []string{"a.go"}}})
	if fp := got["m/a"]; len(fp) != 64 || strings.ToLower(fp) != fp {
		t.Errorf("fp = %q, want 64 lowercase hex", fp)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

```sh
go test ./internal/scanner/fingerprint/ -v
```

期望：包不存在 / `undefined: Compute`。

- [ ] **Step 3: 实现**

```go
// Package fingerprint 算 spec §6.2 的 Merkle 包指纹。纯函数：文件内容与
// 哨兵由调用方（scanner）读好交进来，本包不碰磁盘、不碰库，所以 §11 的
// 敏感性/稳定性测试是普通单测，不需要 fixture 仓库。
package fingerprint

import (
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"sort"
	"strconv"

	"github.com/futuremeng/OpenArchitect/internal/scanner/language"
)

// Compute 返回 pkgPath → 指纹。只有 pkgs 里出现的第一方包进结果；Imports
// 里指向未扫描/第三方的路径被忽略（哨兵已经代表 go.mod 的全部变化）。
func Compute(files map[string]string, sentinel string, pkgs []language.PackageSource) map[string]string {
	byPath := make(map[string]language.PackageSource, len(pkgs))
	paths := make([]string, 0, len(pkgs))
	for _, p := range pkgs {
		if _, dup := byPath[p.PkgPath]; dup {
			continue
		}
		byPath[p.PkgPath] = p
		paths = append(paths, p.PkgPath)
	}
	sort.Strings(paths)

	memo := make(map[string]string, len(paths))
	var walk func(path string, stack map[string]bool) string
	walk = func(path string, stack map[string]bool) string {
		if fp, ok := memo[path]; ok {
			return fp
		}
		p, ok := byPath[path]
		if !ok || stack[path] {
			// 不在第一方清单里（第三方），或理论不可能的自环：给一个稳定
			// 的「未知」值而不是 panic。Go 不允许 import cycle，自环分支
			// 只是把「不查栈」这一类 bug 变成可观察的常量而非死循环。
			return ""
		}
		stack[path] = true
		h := sha256.New()
		for _, f := range p.Files {
			writeLine(h, "f", f, files[f])
		}
		deps := make([]string, 0, len(p.Imports))
		for _, imp := range p.Imports {
			if _, own := byPath[imp]; own && imp != path {
				deps = append(deps, imp)
			}
		}
		sort.Strings(deps)
		for _, d := range deps {
			writeLine(h, "d", d, walk(d, stack))
		}
		writeLine(h, "m", sentinel)
		delete(stack, path)
		memo[path] = hex.EncodeToString(h.Sum(nil))
		return memo[path]
	}
	for _, p := range paths {
		walk(p, map[string]bool{})
	}
	return memo
}

// writeLine 把一行喂进哈希：kind（f=文件、d=依赖、m=go.mod 哨兵）之后每个
// part 都带长度前缀（`<len>:<bytes>`）。长度前缀不是讲究：只用空格/换行分帧时，
// 「删掉 b.go，同时把 `1\nf b.go 2` 写进 a.go」与「a.go=1、b.go=2」会喂进哈希
// 同一串字节，指纹相等，增量就漏掉这次改动。T13 的平价红线靠的正是
// 「内容变了指纹必变」，帧歧义是它的已知反例形状。
func writeLine(h hash.Hash, kind string, parts ...string) {
	_, _ = h.Write([]byte(kind))
	for _, part := range parts {
		_, _ = h.Write([]byte(strconv.Itoa(len(part)) + ":" + part))
	}
}
```

> `writeLine` 的参数类型定稿为 `hash.Hash`（实现文件的 import 块因此要有 `"hash"` 与 `"strconv"`，上面那份已带）。早期草稿写过匿名接口 `interface{ Write([]byte) (int, error) }`，理由是「不绑死 hash.Hash」——但调用方只有 `sha256.New()` 一处，它本来就返回 `hash.Hash`，匿名接口只是让读代码的人多解一层类型，没有换来任何东西。`kind` 与长度前缀两者都是格式契约的一部分，缺一就有语义丢失或帧歧义，理由写在了函数注释里。

- [ ] **Step 4: 全绿 + 提交**

```sh
go test ./internal/scanner/fingerprint/ -v
go vet ./... && go test ./...
git add internal/scanner/fingerprint/fingerprint.go internal/scanner/fingerprint/fingerprint_test.go
git commit -m "feat: Merkle 包指纹纯函数（内容/依赖/go.mod 敏感性 + 顺序稳定性）"
```

---

## Phase C — 扫描编排

### Task 10: 两个新配置键 + 文件 hash + 全量路径落 `arch_files`/`pkg_fingerprints`

**Files:**
- Modify: `internal/config/config.go`
- Create: `internal/scanner/hash.go`
- Modify: `internal/scanner/scanner.go`
- Modify: `internal/config/config_test.go`（实测存在，8 条测试，追加即可）
- Test: `internal/scanner/hash_test.go`

**Interfaces:**
- Consumes: `store.ConfigGet/ConfigSet`、`DB.CommitScan`（T3）、`language.Output.Packages`（T8）、`fingerprint.Compute`（T9）。
- Produces:
  - `config.Settings{... Incremental bool; MaxSnapshots int}`，键 `scan.incremental`（缺省 **true**）、`history.max_snapshots`（缺省 **50**）
  - `scanner.FileSHA256(path string) (string, error)`
  - `scanner.Sentinel(root string) (string, error)` — `H(go.mod ‖ go.sum)`
  - `scanner.HashPackageFiles(root string, pkgs []language.PackageSource) ([]model.FileHash, map[string]string, error)`
  - `scanner.RunWithOptions(ctx, db, root, Options{Full bool}) (Report, error)`；`Run` 保留为 `RunWithOptions(ctx, db, root, Options{})` 的薄壳（GUI 与既有测试的调用点不动）

- [ ] **Step 1: 写失败测试（config）**

`internal/config/config_test.go` 追加。该文件里已有的临时库助手实测叫 **`newDB(t)`**（`config_test.go:12`，`store.Open` 到 `t.TempDir()/cfg.db`，并且已经 `t.Cleanup(db.Close)`）——所以**不要再写 `defer db.Close()`**：全文件没有一个 `defer`，加了就是双重 Close，而 `store.DB.Close` 的二次调用行为本切片没测过。测试体里直接用 `newDB(t)`：

```go
func TestIncrementalAndRetentionDefaults(t *testing.T) {
	db := newDB(t)
	ctx := context.Background()
	// 库里什么都没有：增量开、封顶 50（spec §5 的历史保留策略）。
	s, err := Load(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Incremental || s.MaxSnapshots != 50 {
		t.Errorf("defaults = %+v, want Incremental true / MaxSnapshots 50", s)
	}
	s.Incremental = false
	s.MaxSnapshots = 10
	if err := Save(ctx, db, s); err != nil {
		t.Fatal(err)
	}
	got, err := Load(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if got.Incremental || got.MaxSnapshots != 10 {
		t.Errorf("round trip = %+v", got)
	}
	// 脏值不静默降档：max_snapshots 存了 0 或负数就当没存（删快照是破坏性动作）。
	if err := db.ConfigSet(ctx, "history.max_snapshots", "-1"); err != nil {
		t.Fatal(err)
	}
	back, err := Load(ctx, db)
	if err != nil || back.MaxSnapshots != 50 {
		t.Errorf("bad retention = %d, %v; want the default back", back.MaxSnapshots, err)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

```sh
go test ./internal/config/ -run TestIncrementalAndRetention -v
```

期望：编译失败 `unknown field Incremental`。

- [ ] **Step 3: 实现 config**

`internal/config/config.go`：常量区加 `incrementalKey = "scan.incremental"`、`retentionKey = "history.max_snapshots"`；`DefaultMaxSnapshots = 50` 进常量块。

```go
type Settings struct {
	ScanRoot    string
	ImpactDepth int
	GraphRepos  []string
	// Incremental 是「未变包整段搬运」的总开关。PoC-9 的失败分支要靠它把
	// 复用关掉而保留 hash/指纹（verify 需要），所以它是配置，不是编译期决定。
	Incremental bool
	// MaxSnapshots 是每仓保留的快照数；超出删最旧，CASCADE 连带删掉它的
	// hash/指纹/历史行。抽屉的历史文案必须写「近 N 次扫描内」。
	MaxSnapshots int
}
```

`Load` 里加两次读取（在 `repos` 读取之后、legacy 迁移之前）：

```go
	incRaw, err := db.ConfigGet(ctx, incrementalKey)
	if err != nil {
		return Settings{}, err
	}
	retRaw, err := db.ConfigGet(ctx, retentionKey)
	if err != nil {
		return Settings{}, err
	}
	out := Settings{ScanRoot: root, ImpactDepth: DefaultImpactDepth, GraphRepos: splitRepos(repos),
		Incremental: incRaw != "false", MaxSnapshots: parseRetention(retRaw, DefaultMaxSnapshots)}
```

（注意：`out` 那行是**替换**现有的 `out := Settings{...}`，不是新增第二句赋值。）

`Save` 里加两次写：

```go
	if err := db.ConfigSet(ctx, incrementalKey, strconv.FormatBool(s.Incremental)); err != nil {
		return err
	}
	if err := db.ConfigSet(ctx, retentionKey, strconv.Itoa(s.MaxSnapshots)); err != nil {
		return err
	}
```

新辅助函数：

```go
// parseRetention 只接受 ≥1：0/负数/脏字符串一律回落默认。删旧快照是唯一不可
// 恢复的动作，一个写坏的配置值不该把历史清空。
func parseRetention(raw string, def int) int {
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return def
	}
	return n
}
```

- [ ] **Step 4: 写失败测试（hash）**

新建 `internal/scanner/hash_test.go`：

```go
package scanner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/futuremeng/OpenArchitect/internal/scanner/language"
)

func TestFileSHA256IsContentOnly(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.go")
	if err := os.WriteFile(p, []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fp1, err := FileSHA256(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(fp1) != 64 {
		t.Errorf("hash = %q, want sha256 hex", fp1)
	}
	if err := os.WriteFile(p, []byte("package b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fp2, _ := FileSHA256(p)
	if fp1 == fp2 {
		t.Error("content change must change the hash")
	}
}

// 哨兵是「go.mod/go.sum 一动，全体指纹都动」的载体。go.sum 不存在时不能报错：
// 小模块（和本仓库的全部 fixture）就是没有 go.sum。
func TestSentinelCoversModAndSum(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module m\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	base, err := Sentinel(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(base) != 64 {
		t.Errorf("sentinel = %q", base)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module m\ngo 1.27\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	after, _ := Sentinel(dir)
	if base == after {
		t.Error("go.mod change must change the sentinel")
	}
	if err := os.WriteFile(filepath.Join(dir, "go.sum"), []byte("x h1:abc=\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	withSum, err := Sentinel(dir)
	if err != nil || withSum == after {
		t.Errorf("go.sum must join the sentinel: %q vs %q (%v)", withSum, after, err)
	}
}

func TestHashPackageFilesDedupesAndSorts(t *testing.T) {
	dir := t.TempDir()
	for _, rel := range []string{"a/a.go", "b/b.go"} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, rel)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, rel), []byte("package x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	pkgs := []language.PackageSource{
		{PkgPath: "m/b", Files: []string{"b/b.go"}},
		{PkgPath: "m/a", Files: []string{"a/a.go", "b/b.go"}}, // 同一个文件不该出现两次
	}
	list, byPath, err := HashPackageFiles(dir, pkgs)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Path != "a/a.go" || list[1].Path != "b/b.go" {
		t.Errorf("list = %+v, want two sorted unique rows", list)
	}
	if len(byPath) != 2 || byPath["b/b.go"] != list[1].SHA256 {
		t.Errorf("byPath = %v", byPath)
	}
	// 磁盘上没了的文件是错误，不是「跳过」：少一条 hash 就等于给 verify 造假干净。
	if _, _, err := HashPackageFiles(dir, []language.PackageSource{{PkgPath: "m/z", Files: []string{"z.go"}}}); err == nil {
		t.Error("missing file must error")
	}
}
```

- [ ] **Step 5: 实现 `hash.go`**

```go
package scanner

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
	"github.com/futuremeng/OpenArchitect/internal/scanner/language"
)

// FileSHA256 是单个文件的 SHA-256（hex）。verify 与增量共用这一条读法，
// 「存储 hash vs 磁盘 hash」两边必须是同一个函数算出来的。
func FileSHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("scanner: hash %s: %w", path, err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// Sentinel 是 fp(go.mod + go.sum)：spec §6.2 的全局哨兵。go.sum 可以不存在
// （小模块与全部 fixture 都没有），那时它只由 go.mod 决定。
func Sentinel(root string) (string, error) {
	h := sha256.New()
	for _, name := range []string{"go.mod", "go.sum"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", fmt.Errorf("scanner: sentinel %s: %w", name, err)
		}
		h.Write(data)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// HashPackageFiles 读一遍适配器报上来的文件，返回
// (去重升序的 arch_files 行, path→hash 给指纹用)。
// 文件在扫描中途消失就报错：宁可这一次扫描失败，也不留下一份「看起来干净」的
// hash 清单让 verify 永远说 clean。
func HashPackageFiles(root string, pkgs []language.PackageSource) ([]model.FileHash, map[string]string, error) {
	byPath := map[string]string{}
	for _, p := range pkgs {
		for _, rel := range p.Files {
			if _, done := byPath[rel]; done {
				continue
			}
			sum, err := FileSHA256(filepath.Join(root, rel))
			if err != nil {
				return nil, nil, err
			}
			byPath[rel] = sum
		}
	}
	paths := make([]string, 0, len(byPath))
	for p := range byPath {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	out := make([]model.FileHash, 0, len(paths))
	for _, p := range paths {
		out = append(out, model.FileHash{Path: p, SHA256: byPath[p]})
	}
	return out, byPath, nil
}
```

- [ ] **Step 6: 接进 `scanner.Run`（仍是全量）**

`internal/scanner/scanner.go` 重写编排。`Report` 先不动（T13 再加字段）：

```go
// Options 是一次扫描的开关。Full = 强制全量（`oac scan --full`）。
type Options struct {
	Full bool
}

func Run(ctx context.Context, db *store.DB, root string) (Report, error) {
	return RunWithOptions(ctx, db, root, Options{})
}

func RunWithOptions(ctx context.Context, db *store.DB, root string, opts Options) (Report, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return Report{}, err
	}
	name, err := Detect(abs)
	if err != nil {
		return Report{}, err
	}
	adapter, ok := language.Get(name)
	if !ok {
		return Report{}, ErrUnsupportedLanguage
	}
	out, err := adapter.Parse(ctx, abs)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Report{}, ctxErr
		}
		return Report{}, fmt.Errorf("scanner: parse %s: %w", abs, err)
	}
	if err := ctx.Err(); err != nil {
		return Report{}, err
	}

	repo := model.Repo{ID: model.RepoIDFor(abs), Name: out.ModulePath, LocalPath: abs, Lang: name}
	if err := db.UpsertRepo(ctx, repo); err != nil {
		return Report{}, err
	}
	settings, err := config.Load(ctx, db)
	if err != nil {
		return Report{}, err
	}
	files, hashes, err := HashPackageFiles(abs, out.Packages)
	if err != nil {
		return Report{}, err
	}
	sentinel, err := Sentinel(abs)
	if err != nil {
		return Report{}, err
	}
	fps := fingerprint.Compute(hashes, sentinel, out.Packages)

	snapshot := model.ArchitectureSnapshot{
		ID: model.NewSnapshotID(), RepoID: repo.ID, PackageErrors: out.PackageErrors,
	}
	if err := db.CommitScan(ctx, store.ScanCommit{
		Snapshot: snapshot, Elements: out.Elements, Relationships: out.Relationships,
		Files: files, Fingerprints: fps, KeepSnapshots: settings.MaxSnapshots,
	}); err != nil {
		return Report{}, err
	}
	return Report{
		SnapshotID: snapshot.ID, RepoID: repo.ID, ModulePath: out.ModulePath, Language: name,
		Elements: len(out.Elements), Relationships: len(out.Relationships),
		PackageErrors: out.PackageErrors,
	}, nil
}
```

`defaultMaxSnapshots`（T3 里加的临时常量）现在删掉——它被 `settings.MaxSnapshots` 取代。`internal/config` 的新 import 不违反 R1（`internal/scanner` 允许 import `internal/config`，二者都不碰渲染栈；ciguard 检查的是 UI 栈）。

- [ ] **Step 7: 加一条「真扫一次，hash 落库」的集成测试**

`internal/scanner/scanner_test.go` 追加（沿用该文件已有的 fixture/tempdb 装配）：

```go
func TestScanWritesFileHashesAndFingerprints(t *testing.T) {
	db := newScanTestDB(t)
	defer db.Close()
	ctx := context.Background()
	report, err := Run(ctx, db, fixturePath(t, "shared-lib"))
	if err != nil {
		t.Fatal(err)
	}
	files, err := db.Files(ctx, report.SnapshotID)
	if err != nil || len(files) == 0 {
		t.Fatalf("arch_files empty after scan: %d rows, %v", len(files), err)
	}
	for _, f := range files {
		if len(f.SHA256) != 64 || !strings.Contains(f.Path, "/") && f.Path != filepath.Base(f.Path) {
			t.Errorf("bad arch_files row %+v", f)
		}
		if filepath.IsAbs(f.Path) {
			t.Errorf("arch_files must store repo-root-relative paths: %q", f.Path)
		}
	}
	fps, err := db.Fingerprints(ctx, report.SnapshotID)
	if err != nil || len(fps) == 0 {
		t.Fatalf("pkg_fingerprints empty after scan: %v", err)
	}
}
```

- [ ] **Step 8: 全绿 + 提交**

```sh
go test ./internal/config/ ./internal/scanner/... -v -run 'TestIncremental|TestFileSHA|TestSentinel|TestHashPackage|TestScanWrites'
go vet ./... && go test ./...
git add internal/config/config.go internal/config/config_test.go internal/scanner/hash.go internal/scanner/hash_test.go internal/scanner/scanner.go internal/scanner/scanner_test.go
git commit -m "feat: 扫描时落每文件 SHA-256 与 Merkle 包指纹（含增量/保留数配置）"
```

---

### Task 11: `scanner.Verify` —— 两值裁决、三清单

**Files:**
- Create: `internal/scanner/verify.go`
- Create: `internal/scanner/testutil_test.go`（`fixtureCopy`/`copyTree`/`fixturePath`/`newScanTestDB`——切片 2 的后续任务全部共用这一份，Step 1 里给出实现）
- Test: `internal/scanner/verify_test.go`

**Interfaces:**
- Consumes: `DB.LatestSnapshotID`、`DB.Files`、`FileSHA256`、`model.FileHash`。
- Produces:

```go
type VerifyResult struct {
	RepoID, SnapshotID string
	Clean              bool
	Changed            []string // hash 不符
	Missing            []string // 库里有、磁盘没了
	Untracked          []string // 磁盘上有 .go，库里没有
}
func Verify(ctx context.Context, db *store.DB, repoID string) (VerifyResult, error)
```

裁决（计划定稿 #9）：`Clean = len(Changed)+len(Missing)+len(Untracked) == 0`。**只有两值**，不产 Ix 的第三值 `diverged`（spec §6.4）。

- [ ] **Step 1: 写失败测试**

```go
package scanner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 改一个文件 → stale，并且列出的正是那个文件。DoD-6 的机械化。
func TestVerifyReportsTouchedFile(t *testing.T) {
	db := newScanTestDB(t)
	defer db.Close()
	ctx := context.Background()
	root := fixtureCopy(t, "shared-lib", filepath.Join(t.TempDir(), "lib"))
	report, err := Run(ctx, db, root)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Verify(ctx, db, report.RepoID)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Clean {
		t.Fatalf("fresh scan must verify clean: %+v", res)
	}
	target := filepath.Join(root, "core", "core.go")
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, append(data, []byte("\n// touched\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err = Verify(ctx, db, report.RepoID)
	if err != nil {
		t.Fatal(err)
	}
	if res.Clean {
		t.Fatal("touched file must make the snapshot stale")
	}
	if len(res.Changed) != 1 || res.Changed[0] != "core/core.go" {
		t.Errorf("Changed = %v, want exactly [core/core.go]", res.Changed)
	}
}

func TestVerifyListsMissingAndUntracked(t *testing.T) {
	db := newScanTestDB(t)
	defer db.Close()
	ctx := context.Background()
	root := fixtureCopy(t, "shared-lib", filepath.Join(t.TempDir(), "lib"))
	report, err := Run(ctx, db, root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "core", "core.go")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "core", "extra.go"),
		[]byte("package core\n\n// extra\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Verify(ctx, db, report.RepoID)
	if err != nil {
		t.Fatal(err)
	}
	if res.Clean {
		t.Fatal("want stale")
	}
	joined := strings.Join(res.Missing, ",") + "|" + strings.Join(res.Untracked, ",")
	if !strings.Contains(joined, "core/core.go") {
		t.Errorf("Missing must name the deleted file: %+v", res)
	}
	if !strings.Contains(joined, "core/extra.go") {
		t.Errorf("Untracked must name the new file: %+v", res)
	}
}

// 没扫过的仓不是错误，但也无法裁决。CLI 把它报成一句人话。
func TestVerifyUnknownRepo(t *testing.T) {
	db := newScanTestDB(t)
	defer db.Close()
	res, err := Verify(context.Background(), db, "repo:/nope")
	if err == nil && res.SnapshotID == "" {
		t.Fatal("repo with no snapshot must say so")
	}
}
```

`fixtureCopy` 是新的测试辅助（把只读 fixture 复制到临时目录再改——**绝不允许改 `testdata/fixtures` 本体**，那是切片 1 全部测试的共同基线）。放 `internal/scanner/testutil_test.go`：

```go
package scanner

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/futuremeng/OpenArchitect/internal/core/store"
)

// fixturePath 转发到仓库里既有的 fixtureRoot()（internal/scanner/detector_test.go:10，
// 无参）。切片 2 的每个测试都按名字取 fixture，所以这里加一个带 name 的壳。
func fixturePath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(fixtureRoot(), name)
}

// newScanTestDB 刻意**不**复用既有的 newTestDB（internal/scanner/scanner_test.go:16）：
// 它注册了 t.Cleanup(db.Close)，而切片 2 的每条测试都写 `defer db.Close()`，
// 混用就是第二次 Close —— mygo 的 sqlite 句柄二次关闭行为没有测试钉过，
// 不赌。这里自己开库、把 Close 交给调用点。
func newScanTestDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "scan2.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	return db
}

// fixtureCopy 把 testdata/fixtures/<name> 整棵复制进临时目录，返回副本路径。
// verify 与平价测试都要改文件，改原件会让所有 fixture 测试互相污染。
func fixtureCopy(t *testing.T, name, dest string) string {
	t.Helper()
	src := fixturePath(t, name)
	if err := copyTree(src, dest); err != nil {
		t.Fatalf("copy %s → %s: %v", src, dest, err)
	}
	return dest
}

func copyTree(src, dest string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, in)
		return err
	})
}
```

> 切片 1 已有的等价物是 `fixtureRoot()`（`detector_test.go:10`，无参）与 `newTestDB(t)`（`scanner_test.go:16`，自带 `t.Cleanup`）。上面这份**不**复用 `newTestDB`，理由写在它的注释里（双重 Close）；`fixturePath` 只做一次转发。切片 1 的旧测试一律不动。

- [ ] **Step 2: 跑测试确认失败**

```sh
go test ./internal/scanner/ -run TestVerify -v
```

期望：编译失败 `undefined: Verify`。

- [ ] **Step 3: 实现 `verify.go`**

```go
package scanner

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/futuremeng/OpenArchitect/internal/core/store"
)

// VerifyResult 是「这份图还新不新」。两值裁决（Clean / 不 Clean）、三清单。
// Untracked 会把「因为 build tag 没进任何已加载包的文件」也列出来——那不是
// 误报，图里确实没有它；文案要照实说（计划定稿 #9）。
type VerifyResult struct {
	RepoID     string
	SnapshotID string
	Clean      bool
	Changed    []string
	Missing    []string
	Untracked  []string
}

func Verify(ctx context.Context, db *store.DB, repoID string) (VerifyResult, error) {
	out := VerifyResult{RepoID: repoID}
	repo, err := db.Repo(ctx, repoID)
	if err != nil {
		return out, err
	}
	snapshotID, err := db.LatestSnapshotID(ctx, repoID)
	if err != nil {
		return out, err
	}
	if snapshotID == "" {
		return out, fmt.Errorf("scanner: repo %q has no snapshot; scan it first", repoID)
	}
	out.SnapshotID = snapshotID
	stored, err := db.Files(ctx, snapshotID)
	if err != nil {
		return out, err
	}
	byStored := make(map[string]string, len(stored))
	for _, f := range stored {
		byStored[f.Path] = f.SHA256
	}
	for _, f := range stored {
		disk, err := FileSHA256(filepath.Join(repo.LocalPath, f.Path))
		if err != nil {
			if os.IsNotExist(err) {
				out.Missing = append(out.Missing, f.Path)
				continue
			}
			return out, err
		}
		if disk != f.SHA256 {
			out.Changed = append(out.Changed, f.Path)
		}
	}
	untracked, err := untrackedGoFiles(repo.LocalPath, byStored)
	if err != nil {
		return out, err
	}
	out.Untracked = untracked
	out.Clean = len(out.Changed)+len(out.Missing)+len(out.Untracked) == 0
	sort.Strings(out.Changed)
	sort.Strings(out.Missing)
	return out, nil
}

// untrackedGoFiles 走目录而不重解析（spec §6.4：verify 的代价必须是一次 stat
// 遍历，不是第二次类型检查）。排除测试变体与 vendor/testdata：它们按定义不在
// 图里。
func untrackedGoFiles(root string, stored map[string]string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == "vendor" || name == "testdata" || strings.HasPrefix(name, ".") && path != root {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if _, ok := stored[rel]; !ok {
			out = append(out, rel)
		}
		return nil
	})
	sort.Strings(out)
	return out, err
}
```

- [ ] **Step 4: 全绿 + 提交**

```sh
go test ./internal/scanner/ -run TestVerify -v
go vet ./... && go test ./...
git add internal/scanner/verify.go internal/scanner/verify_test.go internal/scanner/testutil_test.go
git commit -m "feat: scanner.Verify —— 存储 hash vs 磁盘的两值裁决与三清单"
```

---

### Task 12: 元素历史进扫描事务

**Files:**
- Modify: `internal/scanner/scanner.go`
- Test: `internal/scanner/history_test.go`

**Interfaces:**
- Consumes: `DB.Elements`/`DB.Relationships`（上一快照）、`graph.Build`、`graph.DiffSnapshots`/`ChangedFields`（T6）、`ScanCommit.History`（T3）。
- Produces: `func historyEvents(repoID, snapshotID string, prev, next *graph.Graph) []model.HistoryEvent`（`internal/scanner/history.go`，包内私有编排）

- [ ] **Step 1: 写失败测试**

```go
package scanner

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
	"github.com/futuremeng/OpenArchitect/internal/core/store"
)

// 脚本化变更 → 期望事件序列。DoD-5 的 core 半边：改一个方法列表 → changed；
// 加一个类型 → appeared；删掉它 → disappeared。
func TestScanRecordsElementHistory(t *testing.T) {
	db := newScanTestDB(t)
	defer db.Close()
	ctx := context.Background()
	root := fixtureCopy(t, "shared-lib", filepath.Join(t.TempDir(), "lib"))
	first, err := Run(ctx, db, root)
	if err != nil {
		t.Fatal(err)
	}
	const target = "cmp:example.com/shared/core.Server"
	if evs, err := db.History(ctx, target, 5); err != nil || len(evs) != 1 ||
		evs[0].Event != model.EventAppeared {
		t.Fatalf("first scan must record appeared for every element: %+v, %v", evs, err)
	}

	// changed：加一个导出方法。fixture 里 `Server` 只有 `Addr`（实测
	// testdata/fixtures/shared-lib/core/core.go:12），追加一个方法只会动
	// methods_json 一列，正好是 changed 的最小形态。
	written := filepath.Join(root, "core", "core.go")
	data, err := os.ReadFile(written)
	if err != nil {
		t.Fatal(err)
	}
	withMethod := string(data) + "\n// Extra widens the method list.\nfunc (s *Server) Extra() {}\n"
	if err := os.WriteFile(written, []byte(withMethod), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := Run(ctx, db, root)
	if err != nil {
		t.Fatal(err)
	}
	if second.SnapshotID == first.SnapshotID {
		t.Fatal("scan must produce a new snapshot")
	}
	evs, err := db.History(ctx, target, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 2 || evs[0].Event != model.EventChanged {
		t.Fatalf("events = %+v, want newest changed", evs)
	}
	if strings.Join(evs[0].Fields, ",") != "methods_json" {
		t.Errorf("changed fields = %v, want [methods_json]", evs[0].Fields)
	}

	// disappeared：把 `Server` 改名成 `Relic`。必须全树改，不是只改 core.go——
	// `core.Server` 还被 edge/edge.go:6（`server core.Server`）与
	// cmd/app/main.go:9（`_ = core.Server{}`）引用，core.go:16 的 Ledger 字段
	// 也用它。只改声明处会让这两个包编译失败，包错误会整包从图里掉出去，
	// 断言就测不到「恰好一条 disappeared」而是测到「扫描退化了」。
	if err := renameInGoFiles(t, root, "Server", "Relic"); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(ctx, db, root); err != nil {
		t.Fatal(err)
	}
	got, err := db.History(ctx, target, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Event != model.EventDisappeared {
		t.Errorf("events = %+v, want newest disappeared", got)
	}
}

// renameInGoFiles 是测试助手：全树 .go 文件里的 from → to，一次改写完，
// 保证改名后的 fixture 仍然编译通过（否则扫描会退化成包错误，历史事件就不再
// 由「元素变化」驱动）。跳过 vendor/testdata，与 scanner 的可见文件规则一致。
func renameInGoFiles(t *testing.T, root, from, to string) error {
	t.Helper()
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "vendor" || d.Name() == "testdata" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(path, []byte(strings.ReplaceAll(string(b), from, to)), 0o644)
	})
}

// 历史必须与快照同事务：一次 CommitScan 里历史写失败，快照行也不许留下。
// T3 的原子性测试钉的是元素语句，这里钉历史语句。
func TestHistoryFailureRollsBackSnapshot(t *testing.T) {
	db := newScanTestDB(t)
	defer db.Close()
	ctx := context.Background()
	repo := model.Repo{ID: "repo:/x", Name: "x", LocalPath: "/x", Lang: "go"}
	if err := db.UpsertRepo(ctx, repo); err != nil {
		t.Fatal(err)
	}
	c := store.ScanCommit{
		Snapshot:      model.ArchitectureSnapshot{ID: "s0000000000000000001", RepoID: repo.ID},
		Elements:      []model.Element{{ID: "cmp:x.A", Type: model.TypeComponent, Name: "A", RepoID: repo.ID}},
		KeepSnapshots: 50,
		History: []model.HistoryEvent{{ // 没有这个仓 → FK 违约
			RepoID: "repo:/ghost", ElementID: "cmp:x.A",
			SnapshotID: "s0000000000000000001", Event: model.EventAppeared,
		}},
	}
	if err := db.CommitScan(ctx, c); err == nil {
		t.Fatal("unknown repo in history must fail the commit")
	}
	if id, err := db.LatestSnapshotID(ctx, repo.ID); err != nil || id != "" {
		t.Errorf("snapshot survived a failed history write: %q, %v", id, err)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

```sh
go test ./internal/scanner/ -run 'TestScanRecords|TestHistoryFailure' -v
```

期望：第一条 FAIL（`History` 返回 0 行——本任务前没人写历史），第二条已 PASS（T3 的事务已覆盖）。**若第二条本来就绿，在 findings 里记一句「T3 的 TestCommitScanIsAtomic 已覆盖同一机制」，不许删测试。**

- [ ] **Step 3: 实现 `history.go`**

```go
package scanner

import (
	"github.com/futuremeng/OpenArchitect/internal/core/graph"
	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

// historyEvents 把 spec §8 的判定翻成事件行：added→appeared、removed→
// disappeared、changed→changed（detail = 字段名列表）。顺序是 appeared →
// changed → disappeared，每段内部按元素 id 升序——历史行的顺序会进 golden，
// 不能靠 map 迭代序。
func historyEvents(repoID, snapshotID string, prev, next *graph.Graph) []model.HistoryEvent {
	d := graph.DiffSnapshots(prev, next)
	out := make([]model.HistoryEvent, 0, len(d.Added)+len(d.Changed)+len(d.Removed))
	for _, el := range d.Added {
		out = append(out, model.HistoryEvent{RepoID: repoID, ElementID: el.ID,
			SnapshotID: snapshotID, Event: model.EventAppeared})
	}
	for _, c := range d.Changed {
		out = append(out, model.HistoryEvent{RepoID: repoID, ElementID: c.To.ID,
			SnapshotID: snapshotID, Event: model.EventChanged, Fields: c.Fields})
	}
	for _, el := range d.Removed {
		out = append(out, model.HistoryEvent{RepoID: repoID, ElementID: el.ID,
			SnapshotID: snapshotID, Event: model.EventDisappeared})
	}
	return out
}
```

- [ ] **Step 4: 在 `RunWithOptions` 里读旧快照并算历史**

`internal/scanner/scanner.go`，在 `CommitScan` 之前插入（`prevSnapshotID`/旧图同时供 T13 的复用使用，本任务先只为历史读）：

```go
	prevSnapshotID, err := db.LatestSnapshotID(ctx, repo.ID)
	if err != nil {
		return Report{}, err
	}
	prevElements, prevRels, err := readSnapshot(ctx, db, prevSnapshotID)
	if err != nil {
		return Report{}, err
	}
	history := historyEvents(repo.ID, snapshot.ID,
		graph.Build(prevElements, prevRels), graph.Build(out.Elements, out.Relationships))
```

并把 `History: history` 与 `Elements/Relationships` 的来源换成 `out`（T13 之后 `out` 是混合结果，本任务先等于原值）。新辅助（放 `history.go` 尾部，别在 scanner.go 里再写一遍）：

```go
// readSnapshot 把某个快照的行读成两张切片。snapshotID 为空 = 这仓第一次扫，
// 空切片是正常状态，不是错误。
func readSnapshot(ctx context.Context, db *store.DB, snapshotID string) ([]model.Element, []model.Relationship, error) {
	if snapshotID == "" {
		return nil, nil, nil
	}
	el, err := db.Elements(ctx, snapshotID)
	if err != nil {
		return nil, nil, err
	}
	rl, err := db.Relationships(ctx, snapshotID)
	if err != nil {
		return nil, nil, err
	}
	return el, rl, nil
}
```

`internal/scanner/scanner.go` 的 import 加 `"github.com/futuremeng/OpenArchitect/internal/core/graph"`。

> 顺序纪律：`snapshot := model.ArchitectureSnapshot{ID: model.NewSnapshotID(), …}` 必须在 `historyEvents` 之前（事件的 `snapshot_id` 指向新快照），而 `prevSnapshotID` 必须在 `CommitScan` 之前读——写完之后旧快照可能已经被封顶裁掉了。

- [ ] **Step 5: 全绿 + 提交**

```sh
go test ./internal/scanner/ -run 'TestScanRecords|TestHistoryFailure|TestScan' -v
go vet ./... && go test ./...
git add internal/scanner/history.go internal/scanner/scanner.go internal/scanner/history_test.go
git commit -m "feat: 扫描事务内写元素历史（appeared/changed/disappeared + 变化字段）"
```

---

### Task 13: 增量复用 + **平价门槛（PoC-9）**

**Files:**
- Create: `internal/scanner/reuse.go`、`cmd/oac/copy_test.go`（`copyTree` 的抄本，见 Step 5 下面那段）
- Modify: `internal/scanner/scanner.go`、`internal/scanner/report.go`、`cmd/oac/scan.go`（只加 `mode/reused/parsed` 一行，Step 7）
- Modify: `internal/config/config.go`（无需，键已在 T10）
- Test: `internal/scanner/reuse_test.go`、`cmd/oac/incremental_parity_test.go`、`docs/superpowers/poc/2026-10-30-slice2-findings.md`

**Interfaces:**
- Consumes: `fingerprint.Compute`、`DB.Fingerprints`、`readSnapshot`、`model.PkgPathOf`、`config.Settings`。
- Produces:
  - `func splice(out language.Output, prevElements []model.Element, prevRels []model.Relationship, reused map[string]bool) language.Output`
  - `Report.Reused int` / `Report.Parsed int` / `Report.Incremental bool`
  - `Options{Full bool}`（已在 T10）

**DoD-3 是本切片最大的风险项，也是唯一可能推翻产品决定的任务。门槛与失败分支见 Step 7。**

- [ ] **Step 1: 写失败测试（splice 语义）**

```go
package scanner

import (
	"reflect"
	"testing"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
	"github.com/futuremeng/OpenArchitect/internal/scanner/language"
)

func TestSpliceCarriesWholePackages(t *testing.T) {
	fresh := language.Output{
		Elements: []model.Element{
			{ID: model.SystemID("m"), Type: model.TypeSystem},
			{ID: "ctn:m/a", Type: model.TypeContainer, Name: "a-fresh"},
			{ID: "cmp:m/a.A", Type: model.TypeComponent, Name: "A-fresh"},
			{ID: "cmp:m/b.B", Type: model.TypeComponent, Name: "B-fresh"},
		},
		Relationships: []model.Relationship{
			{FromID: model.SystemID("m"), ToID: "ctn:m/a", Type: model.RelContains},
			{FromID: "ctn:m/a", ToID: "cmp:m/a.A", Type: model.RelContains},
			{FromID: "cmp:m/a.A", ToID: "cmp:m/b.B", Type: model.RelCalls, Weight: 3},
			{FromID: "cmp:m/b.B", ToID: "cmp:m/a.A", Type: model.RelCalls, Weight: 1},
		},
	}
	prev := []model.Element{
		{ID: model.SystemID("m"), Type: model.TypeSystem},
		{ID: "ctn:m/a", Type: model.TypeContainer, Name: "a"},
		{ID: "cmp:m/a.A", Type: model.TypeComponent, Name: "A"},
		{ID: "cmp:m/b.B", Type: model.TypeComponent, Name: "B-prev"},
	}
	prevRels := []model.Relationship{
		{FromID: model.SystemID("m"), ToID: "ctn:m/a", Type: model.RelContains},
		{FromID: "ctn:m/a", ToID: "cmp:m/a.A", Type: model.RelContains},
		{FromID: "cmp:m/a.A", ToID: "cmp:m/b.B", Type: model.RelCalls, Weight: 3},
		{FromID: "cmp:m/b.B", ToID: "cmp:m/a.A", Type: model.RelCalls, Weight: 9},
	}
	got := splice(fresh, prev, prevRels, map[string]bool{"m/a": true})

	// m/a 整段来自 prev：容器名、组件名、它的出边权重。
	if name := elementNames(got.Elements); !reflect.DeepEqual(name,
		[]string{"", "a", "A", "B-fresh"}) {
		t.Errorf("elements = %v", name)
	}
	for _, rel := range got.Relationships {
		if rel.FromID == "cmp:m/b.B" && rel.Weight != 1 {
			t.Errorf("未复用包 m/b 的出边被 prev 覆盖了: %+v", rel)
		}
		if rel.FromID == "cmp:m/a.A" && rel.Weight != 3 {
			t.Errorf("复用包 m/a 的出边必须原样搬运: %+v", rel)
		}
	}
}

// System 元素（PkgPathOf == ""）永远来自本次 fresh——它是唯一一行、
// doc 来自根包，复用与否结果相同，不值得为它开桶（计划定稿 #8）。
func TestSpliceNeverReusesSystemRows(t *testing.T) {
	fresh := language.Output{
		Elements:      []model.Element{{ID: model.SystemID("m"), Type: model.TypeSystem, Description: "fresh doc"}},
		Relationships: []model.Relationship{{FromID: model.SystemID("m"), ToID: "ctn:m/a", Type: model.RelContains}},
	}
	prev := []model.Element{{ID: model.SystemID("m"), Type: model.TypeSystem, Description: "stale doc"}}
	got := splice(fresh, prev, nil, map[string]bool{"m": true})
	if got.Elements[0].Description != "fresh doc" {
		t.Errorf("system element was replaced: %+v", got.Elements[0])
	}
}

func elementNames(els []model.Element) []string {
	out := make([]string, len(els))
	for i, e := range els {
		out[i] = e.Name
	}
	return out
}
```

- [ ] **Step 2: 跑测试确认失败**

```sh
go test ./internal/scanner/ -run TestSplice -v
```

期望：编译失败 `undefined: splice`。

- [ ] **Step 3: 实现 `reuse.go`**

```go
package scanner

import (
	"github.com/futuremeng/OpenArchitect/internal/core/model"
	"github.com/futuremeng/OpenArchitect/internal/scanner/language"
)

// splice 把「指纹未变」的包整段换回上一快照的行。归属键是 from_id 所属包
// （计划定稿 #8）：`model.PkgPathOf` 为空的行（唯一的 System 元素和它的
// contains 边）永远来自本次解析。
//
// 为什么整段搬运是对的：fp(P) 含 P 的直接第一方依赖的 fp（递归到传递闭包），
// 所以 fp 未变 ⇒ 类型化引用的输入全部未变 ⇒ P 的元素与出边与上次逐字相同
// （spec §6.1）。这条论证不在这里自证，由 Step 6 的平价测试实证。
func splice(out language.Output, prevElements []model.Element,
	prevRels []model.Relationship, reused map[string]bool) language.Output {
	keep := make([]model.Element, 0, len(out.Elements))
	for _, el := range out.Elements {
		if !inAny(el.ID, reused) {
			keep = append(keep, el)
		}
	}
	for _, el := range prevElements {
		if inAny(el.ID, reused) {
			keep = append(keep, el)
		}
	}
	rels := make([]model.Relationship, 0, len(out.Relationships))
	for _, rel := range out.Relationships {
		if !inAny(rel.FromID, reused) {
			rels = append(rels, rel)
		}
	}
	for _, rel := range prevRels {
		if inAny(rel.FromID, reused) {
			rels = append(rels, rel)
		}
	}
	out.Elements, out.Relationships = keep, rels
	return out
}

// inAny 只认「属于某个被复用的包」。PkgPathOf 为空（System 元素/边）永远
// 返回 false，于是永远走 fresh 分支。
func inAny(id string, reused map[string]bool) bool {
	pkg := model.PkgPathOf(id)
	if pkg == "" {
		return false
	}
	return reused[pkg]
}
```

- [ ] **Step 4: 编排进 `RunWithOptions`**

`internal/scanner/scanner.go`，在 `fps := fingerprint.Compute(...)` 之后、`CommitScan` 之前：

```go
	incremental := settings.Incremental && !opts.Full && prevSnapshotID != ""
	reused := map[string]bool{}
	if incremental {
		prevFPS, err := db.Fingerprints(ctx, prevSnapshotID)
		if err != nil {
			return Report{}, err
		}
		// 旧库/旧快照没有指纹行 → 空 map，下面的循环自然一次都不命中，
		// 退化为全量。spec §6.3 的这条回落不写分支，靠数据形状成立。
		for pkg, fp := range fps {
			if prev, ok := prevFPS[pkg]; ok && prev == fp {
				reused[pkg] = true
			}
		}
		if len(reused) > 0 {
			out = splice(out, prevElements, prevRels, reused)
		} else {
			incremental = false
		}
	}
```

`Report` 组装处加三行（`report.go` 先加字段）：

```go
type Report struct {
	SnapshotID    string
	RepoID        string
	ModulePath    string
	Language      string
	Elements      int
	Relationships int
	PackageErrors []string
	// Reused / Parsed 是增量收益的两半（spec §9：`oac scan` 要报
	// reused=<n> parsed=<m>）。全量时 Reused = 0。
	Reused      int
	Parsed      int
	Incremental bool
}
```

```go
	return Report{
		…existing fields…,
		Reused:      len(reused),
		Parsed:      len(out.Packages) - len(reused),
		Incremental: incremental,
	}, nil
```

`historyEvents` 的 `next` 参数保持 `graph.Build(out.Elements, out.Relationships)`（在 splice **之后**取，历史描述的正是最终落库的那份图）。

- [ ] **Step 5: 平价测试（DoD-3，本切片最重要的测试）**

新建 `cmd/oac/incremental_parity_test.go`：

```go
package main

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/futuremeng/OpenArchitect/internal/config"
	"github.com/futuremeng/OpenArchitect/internal/core/model"
	"github.com/futuremeng/OpenArchitect/internal/core/store"
	"github.com/futuremeng/OpenArchitect/internal/scanner"
)

// 增量结果必须与 --full 逐字节一致。这是增量能不能默认开的唯一判据，
// 也是 spec §11 点名「本切片最重要的一条测试」。
//
// 做法：复制 fixture → 两个库各自全量扫一次（同一份内容，两份相同基线）→
// 改一个文件（只改一次）→ 一个库走增量、另一个走 --full → 规范化成
// 「id|type|name\ndescription\nfile\nmethods\t…\n」的文本行，两个字符串必须
// 完全相等（元素、关系、hash、指纹四段）。
//
// 两个库必须扫**同一份最终内容**：早期草稿让第二个库在跑之前又改了一次文件，
// 于是两条出口比的是两个不同版本的仓库，`arch_files` 的哈希段必然不同——那是
// 假红，不是平价失败。
//
// 改的文件是 `cmd/app/main.go`，不是 `core/core.go`。实测 `shared-lib` 的三方
// 依赖是 core ← edge ← cmd/app（`edge/edge.go:3`、`cmd/app/main.go:4-5`），
// Merkle 会沿 import 图传播：改 core 会让三个包的指纹全变，`reused` 恒为 0，
// 增量路径一次都不走，平价测试退化成立；改没有任何包依赖的 `cmd/app` 才是
// 2 复用 / 1 重解析，既真的走了 splice，又满足 DoD-3 的 reused/(reused+parsed)
// ≥ 0.5。`main` 包在指纹侧不缺席——T8 的 `sources` 收集发生在 `go.go` 的
// `pkg.Name == "main"` 分支**之前**，`cmd/app` 有 PackageSource；元素侧它产
// Container（`go.go:89-95`），`model.PkgPathOf` 认 `ctn:` 前缀（`ids.go:27`），
// 所以 splice 会把它一起搬过来。
func TestIncrementalParityWithFullScan(t *testing.T) {
	ctx := context.Background()
	root := copyFixture(t, "shared-lib", filepath.Join(t.TempDir(), "lib"))

	dbInc := openTempDB(t)
	defer dbInc.Close()
	dbFull := openTempDB(t)
	defer dbFull.Close()
	for _, db := range []*store.DB{dbInc, dbFull} {
		if _, err := scanner.Run(ctx, db, root); err != nil { // 两份相同基线
			t.Fatal(err)
		}
	}

	// 显式开启增量，不依赖产品缺省：PoC-9 的三条出口里缺省可能是 false，
	// 而这条测试在三种走向下都必须绿——它钉的是「增量路径与全量逐字节一致」，
	// 不是「默认开增量」。
	if err := config.Save(ctx, dbInc, config.Settings{Incremental: true, MaxSnapshots: 50}); err != nil {
		t.Fatal(err)
	}

	touchMainPackage(t, root) // 唯一一次内容变更

	inc, err := scanner.RunWithOptions(ctx, dbInc, root, scanner.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !inc.Incremental || inc.Reused < 2 || inc.Parsed < 1 {
		t.Fatalf("want reuse of core+edge and a re-parsed cmd/app, got %+v", inc)
	}
	t.Logf("reused=%d parsed=%d", inc.Reused, inc.Parsed) // DoD-3 的比值从这里读

	full, err := scanner.RunWithOptions(ctx, dbFull, root, scanner.Options{Full: true})
	if err != nil {
		t.Fatal(err)
	}
	if full.Reused != 0 || full.Incremental {
		t.Fatalf("--full must reuse nothing: %+v", full)
	}

	for _, section := range []struct {
		name string
		read func(*store.DB, string) string
	}{
		{"elements", dumpElements},
		{"relationships", dumpRelationships},
		{"files", dumpFiles},
		{"fingerprints", dumpFingerprints},
	} {
		a, b := section.read(dbInc, inc.SnapshotID), section.read(dbFull, full.SnapshotID)
		if a != b {
			t.Errorf("%s differs:\n--- incremental ---\n%s\n--- full ---\n%s", section.name, a, b)
		}
	}
}

// 规范化转储：排序 + 固定分隔符，snapshot id 不进文本（它按定义不同）。
func dumpElements(db *store.DB, snapshotID string) string {
	ctx := context.Background()
	els, err := db.Elements(ctx, snapshotID)
	if err != nil {
		panic(err)
	}
	sort.Slice(els, func(i, j int) bool { return els[i].ID < els[j].ID })
	var b strings.Builder
	for _, e := range els {
		b.WriteString(strings.Join([]string{e.ID, string(e.Type), e.Name, e.Description,
			e.FilePath, strings.Join(e.Methods, ",")}, "\x1f"))
		b.WriteByte('\n')
	}
	return b.String()
}

func dumpRelationships(db *store.DB, snapshotID string) string {
	ctx := context.Background()
	rels, err := db.Relationships(ctx, snapshotID)
	if err != nil {
		panic(err)
	}
	lines := make([]string, 0, len(rels))
	for _, r := range rels {
		lines = append(lines, r.FromID+"\x1f"+r.ToID+"\x1f"+string(r.Type)+"\x1f"+strconv.Itoa(r.Weight))
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

func dumpFiles(db *store.DB, snapshotID string) string {
	ctx := context.Background()
	fs, err := db.Files(ctx, snapshotID)
	if err != nil {
		panic(err)
	}
	lines := make([]string, 0, len(fs))
	for _, f := range fs {
		lines = append(lines, f.Path+"\x1f"+f.SHA256)
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

func dumpFingerprints(db *store.DB, snapshotID string) string {
	ctx := context.Background()
	m, err := db.Fingerprints(ctx, snapshotID)
	if err != nil {
		panic(err)
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k + "\x1f" + m[k] + "\n")
	}
	return b.String()
}

func openTempDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "parity.db"))
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func touchMainPackage(t *testing.T, root string) {
	t.Helper()
	// 只有一处追加，且追加的是固定串：重复调用会改变最终内容，所以本任务
	// 的测试序列里它恰好调一次。
	writeAppend(t, filepath.Join(root, "cmd", "app", "main.go"), "\n// touch\n")
}

func writeAppend(t *testing.T, path, extra string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, []byte(extra)...), 0o644); err != nil {
		t.Fatal(err)
	}
}

func copyFixture(t *testing.T, name, dest string) string {
	t.Helper()
	src := filepath.Join(fixturesRoot(), name)
	if err := copyTree(src, dest); err != nil {
		t.Fatal(err)
	}
	return dest
}
```

`copyTree` 在 `cmd/oac` 包里不存在（实测全仓库只有 T11 的 `internal/scanner/testutil_test.go` 定义它）：把 T11 Step 1 那份 `copyTree` 原样抄进 `cmd/oac/copy_test.go`（连同它需要的 `io/os/path/filepath` import），`copyFixture` 调它——`fixturesRoot` 仓库里已有（`cmd/oac/impact_test.go:20`），**不要重复定义**。

**定稿：平价测试放 `cmd/oac/incremental_parity_test.go`，不放 `internal/scanner/parity_test.go`**——放哪不是风格问题：DoD-3 的验收命令写的就是 `go test ./cmd/oac/ -run TestIncrementalParity`，T23 的 `TestIncrementalParityTestIsNotSkippable` 读的文件路径也是 `../../cmd/oac/incremental_parity_test.go`（`tools/ciguard`）。代价是要在 `cmd/oac` 里带一份复制工具（跨包不能共享测试助手，`internal/scanner` 的 `copyTree` 引不过来）。它钉的是 **scanner 层**的 `Options{Full:true}` ↔ `Options{}` 两条出口，不是 CLI 的 `--full` 旗标——旗标行为由 T15 Step 4 钉（那时旗标才存在，T14 才注册）。决定记在 findings 里。

- [ ] **Step 6: 跑平价测试确认通过**

```sh
go test ./cmd/oac/ ./internal/scanner/ -run 'TestIncrementalParity|TestSplice' -v
```

期望：PASS。红了就是**增量正确性问题**——按本任务 Step 7 里「平价不绿」那条分支处理（默认关 + 删显式开关键，数据地基与纯函数测试保留），其余任务照常。**不要靠调指纹让它绿。**

- [ ] **Step 7: PoC-9 墙钟门槛（STOP 点）**

**先补一行 CLI 输出**（本步骤的前置，不是 T14 的活）：`--full` 旗标要到 T14 才注册，而 PoC-9 只需要「空库首跑 = 全量」和「有基线复跑 = 增量」这两种自然形态，所以这里直接用现有 CLI；但 `reused=`/`parsed=` 得先打得出来。`cmd/oac/scan.go` 的 `runScan` 在 `elements … relationships …` 那行之后加：

```go
	fmt.Fprintf(out, "mode %s reused=%d parsed=%d\n",
		map[bool]string{true: "incremental", false: "full"}[report.Incremental],
		report.Reused, report.Parsed)
```

T14 重写 `runScan` 时这一行**原样保留**（计划里 T14 的 `runScan` 已经带它），所以加早了不会有第二次改动。

测量对象（实测存在，且就是 `go.mod` 的 `replace` 目标；模块缓存目录 0555、文件 0444，**必须复制出来才能改文件**）：

```sh
export PATH="/usr/local/go/bin:$PATH" CGO_ENABLED=0 GOPROXY=https://goproxy.cn,direct
go build -o /tmp/oac ./cmd/oac                 # 不入库的一次性二进制
SRC="/Users/futuremeng/go/pkg/mod/github.com/futuremeng/onevra/source@v0.2026.13-0.20261028081121-a3bbb6f230ab"
TOP="$(mktemp -d)"; cp -R "$SRC" "$TOP/source" && chmod -R u+w "$TOP/source"
WORK="$TOP/source"; DB="$TOP/poc9.db"          # DB 放在仓库外，免得进扫描视野
TARGET="$(find "$WORK" -name '*.go' ! -name '*_test.go' | sort | head -1)"

# 全量：每次都是空库首跑（prevSnapshotID 为空 → 自然全量），第一跑是预热，弃掉
rm -f "$DB"; /tmp/oac scan "$WORK" --db "$DB" >/dev/null
for i in 1 2 3; do rm -f "$DB"; /usr/bin/time -p /tmp/oac scan "$WORK" --db "$DB" >/dev/null; done

# 增量：先建一次带快照的基线（同样弃掉预热计时），之后每轮追加一行注释再计时
rm -f "$DB"; /tmp/oac scan "$WORK" --db "$DB" >/dev/null
for i in 1 2 3; do
  echo "// poc9 $i" >> "$TARGET"
  /usr/bin/time -p /tmp/oac scan "$WORK" --db "$DB" 2>&1 | tail -4
done
```

每轮的 `tail -4` 里同时有 `mode/reused/parsed` 和 `real`，两者要一起记。

**为什么必须跑 3 次取中位数**：切片 1 的 PoC-3 数字段（`docs/superpowers/poc/2026-10-19-slice1-findings.md`）记录过 onevra 同一模式的热跑波动是 **0.89 s ↔ 8.24 s**，而全量基线本身只有 ≈1.95 s——单次读数的噪声比 20% 的门槛信号大一到两个数量级。所以判定规则定死：

- 每个模式取 3 次跑（弃预热）的**中位数**，收益 = `1 - median_inc/median_full`。
- 若某一模式的**组内极差 > 两模式中位数之差**，这组读数无效：再各跑 3 次取 6 个样本的中位数。
- 仍然分辨不出来 → findings 里如实写「噪声压过信号，门槛未证明」，并走下面的**收益 < 20%** 分支。门禁的原文是「收益 ≥20%」，未被证明的收益不是收益。

把数字（全量中位墙钟、增量中位墙钟、组内极差、`reused=<n>`/`parsed=<m>`）按「结论 / 数字 / 影响」写进 `docs/superpowers/poc/2026-10-30-slice2-findings.md`。

三条分支的**代码改动只有一处**：`config.Load` 里 `scan.incremental` 的缺省值。除此之外 `hash.go`/`fingerprint`/`arch_files`/`verify`/`oac verify`/T21 的增量开关与它的持久化测试全部原样保留——verify 与历史都不依赖增量路径，键也留着，用户仍可在设置页显式打开。

- **收益 ≥ 20%** → 缺省保持 true，本任务收尾。
- **收益 < 20%**（含「噪声压过信号」） → 缺省改 false。findings 写「复用收益未达门槛，增量默认关闭，可在设置页显式开启；hash/指纹/verify 地基保留」。
- **平价不绿** → 缺省同样改 false，但性质不同：**正确性**门槛没过。除了改缺省，必须同时在 findings 与 T24 的 DoD-3 行里记「逐字节平价未成立，增量路径已知不安全，默认关闭且不建议开启」，README 里那句增量默认也要跟着写反。设置页的开关**不删**（删它会连带改 T21 的三处断言，而一个诚实的开关比一个消失的开关更难被误用——用户在设置页看到的就是当前产品默认）。

两条失败分支都只动 `config.Load` 的缺省，所以 T10 的 `TestIncrementalAndRetentionDefaults` 里 **`if !s.Incremental || s.MaxSnapshots != 50`** 这一条断言（全计划里唯一依赖 `Incremental` 缺省值的断言，实测：该测试其余两条断言 round-trip 与脏值回落，与缺省无关）随之改成 `if s.Incremental || s.MaxSnapshots != 50`。`TestIncrementalParityWithFullScan` 在三条走向下都绿——它用 `config.Save` 在自己的临时库里显式开增量，不读产品缺省。

三种走向都要继续做完 T14-T26，但**必须先向用户报告数字与选择的分支**（全局约束：任一 PoC 门槛不通过就停下来报告）。

- [ ] **Step 8: 提交**

```sh
go vet ./... && go test ./...
git add internal/scanner/reuse.go internal/scanner/reuse_test.go internal/scanner/scanner.go internal/scanner/report.go cmd/oac/scan.go cmd/oac/copy_test.go cmd/oac/incremental_parity_test.go docs/superpowers/poc/2026-10-30-slice2-findings.md
git commit -m "feat: 增量扫描（未变包整段搬运）+ 平价测试与 PoC-9 门槛实测"
```

---

## Phase D — CLI

### Task 14: `subcommand` 形状改造 + `--json` 基建（零行为变更）

**Files:**
- Modify: `cmd/oac/main.go`
- Create: `cmd/oac/options.go`
- Test: `cmd/oac/main_test.go`

**Interfaces:**
- Consumes: 既有 `hoistFlags`/`hoistValue`/`openDB`/`defaultDBPath`/`printUsage`（`main.go:89-177`）。
- Produces:

```go
type options struct {
	args     []string
	depth    int
	full     bool
	json     bool
	all      bool
	repos    []string
	from     string
	to       string
	limit    int
	by       string
	dbPath   string
}
type subcommand struct {
	arg   string
	flags func(*flag.FlagSet, *options)
	run   func(ctx context.Context, db *store.DB, o options, out io.Writer) error
}
func run(ctx context.Context, args []string, out io.Writer) error   // 新增 out 形参
func writeJSON(out io.Writer, v any) error
func wantOne(o options, what string) (string, error)
func registerRepoFlag(fs *flag.FlagSet, o *options)
```

F3/F4 三条纪律逐字保留（见全局约束与计划定稿 #10）。

- [ ] **Step 1: 写失败测试**

新建 `cmd/oac/main_test.go`：

```go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// F4 的回归：typo 必须在开库之前被拒，否则一条 `oac graphg` 就会在用户
// 的 App Support 里凭空造出一个 0 字节的库。
func TestUnknownVerbNeverCreatesDB(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "nope.db")
	err := run(context.Background(), []string{"graphg", "--db", path}, io.Discard)
	if err == nil {
		t.Fatal("unknown verb must error")
	}
	if _, statErr := os.Stat(path); statErr == nil {
		t.Error("database file was created by a rejected verb")
	}
	if _, statErr := os.Stat(filepath.Dir(path)); statErr == nil {
		t.Error("MkdirAll ran before the verb was validated")
	}
}

// 新增值 flag 不需要改 hoistFlags（F2）：--repo 在位置参数前后都要能认。
func TestValueFlagsHoistFromBothPositions(t *testing.T) {
	fs := flag.NewFlagSet("graph", flag.ContinueOnError)
	var o options
	registerRepoFlag(fs, &o)
	fs.BoolVar(&o.all, "all", false, "")
	for _, order := range [][]string{
		{"--repo", "repo:/a", "repo:/b"},
		{"repo:/x", "--repo", "repo:/a"},
		{"--all", "repo:/x", "--repo", "repo:/a"},
	} {
		o = options{}
		fs = flag.NewFlagSet("graph", flag.ContinueOnError)
		registerRepoFlag(fs, &o)
		fs.BoolVar(&o.all, "all", false, "")
		if err := fs.Parse(hoistFlags(fs, order)); err != nil {
			t.Fatalf("%v: %v", order, err)
		}
		if fs.NArg() > 1 || len(o.repos) == 0 {
			t.Errorf("%v hoisted wrong: repos=%v args=%v", order, o.repos, fs.Args())
		}
	}
}

// -h 走 flag.ErrHelp → 打印 usage 并返回 nil（不是错误退出码）。
func TestHelpExitsZero(t *testing.T) {
	var buf bytes.Buffer
	if err := run(context.Background(), []string{"scan", "-h"}, &buf); err != nil {
		t.Fatalf("-h must be a clean exit: %v", err)
	}
}

func TestWriteJSONIsIndentedSingleObject(t *testing.T) {
	var buf bytes.Buffer
	if err := writeJSON(&buf, map[string]int{"elements": 2}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(buf.String(), "}\n") {
		t.Errorf("json must end with a newline, got %q", buf.String())
	}
	var back map[string]int
	if err := json.Unmarshal(buf.Bytes(), &back); err != nil || back["elements"] != 2 {
		t.Errorf("round trip = %v, %v", back, err)
	}
}

func TestWantOneArity(t *testing.T) {
	if _, err := wantOne(options{args: []string{"a", "b"}}, "repo id"); err == nil {
		t.Error("two positionals must be rejected")
	}
	if got, err := wantOne(options{args: []string{"repo:/x"}}, "repo id"); err != nil || got != "repo:/x" {
		t.Errorf("wantOne = %q, %v", got, err)
	}
}
```

补 `"io"` import（`io.Discard` 用到了）。

- [ ] **Step 2: 跑测试确认失败**

```sh
go test ./cmd/oac/ -run 'TestUnknownVerb|TestValueFlags|TestWriteJSON|TestWantOne|TestHelp' -v
```

期望：编译失败（`run` 签名不匹配 + `options` 未定义）。

- [ ] **Step 3: 写 `options.go`**

```go
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
)

// options 是所有 verb 的旗标并集。每个 verb 只在自己的 flags() 里注册用得上的
// 那几颗，于是 hoistFlags 的「从 FlagSet 自己判断是否吃值」继续免维护（F2）。
type options struct {
	args   []string // verb + 旗标之后剩下的位置参数
	dbPath string
	depth  int
	full   bool
	json   bool
	all    bool
	repos  []string
	from   string
	to     string
	limit  int
	by     string // 只有 rank 读它；放在这里是因为 Task 14 一次性定完结构
}

func wantOne(o options, what string) (string, error) {
	if len(o.args) != 1 {
		return "", fmt.Errorf("want exactly one %s, got %v", what, o.args)
	}
	return o.args[0], nil
}

// stringListFlag 让 --repo 可重复。它不实现 IsBoolFlag，所以 hoistValue 会
// 把它和它的值一起提前——这正是 F2 想要的免维护。
type stringListFlag struct{ target *[]string }

func (f stringListFlag) String() string {
	if f.target == nil {
		return ""
	}
	return strings.Join(*f.target, ",")
}

func (f stringListFlag) Set(v string) error {
	if *f.target == nil {
		*f.target = []string{}
	}
	*f.target = append(*f.target, v)
	return nil
}

func registerRepoFlag(fs *flag.FlagSet, o *options) {
	fs.Var(stringListFlag{&o.repos}, "repo", "repository id (repeatable)")
}

func registerJSONFlag(fs *flag.FlagSet, o *options) {
	fs.BoolVar(&o.json, "json", false, "machine-readable output")
}

// writeJSON 是 --json 的唯一出口：缩进两空格、结尾换行，失败只返回 error
// （退出码由 main 统一给 1，见全局约束）。
func writeJSON(out io.Writer, v any) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("oac: encode json: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: 重写 `main.go` 的 dispatch**

```go
func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "oac:", err)
		os.Exit(1)
	}
}

var subcommands = map[string]subcommand{
	"scan": {arg: "path", flags: func(fs *flag.FlagSet, o *options) {
		fs.BoolVar(&o.full, "full", false, "force a full scan, ignoring package fingerprints")
		registerJSONFlag(fs, o)
	}, run: runScan},
	"graph": {arg: "repo id", flags: func(fs *flag.FlagSet, o *options) {
		fs.BoolVar(&o.all, "all", false, "merge every repository")
		registerRepoFlag(fs, o)
		registerJSONFlag(fs, o)
	}, run: runGraph},
	"impact": {arg: "element id", flags: func(fs *flag.FlagSet, o *options) {
		fs.IntVar(&o.depth, "depth", 2, "impact diffusion depth")
		fs.BoolVar(&o.all, "all", false, "merge every repository")
		registerRepoFlag(fs, o)
		registerJSONFlag(fs, o)
	}, run: runImpact},
}

type subcommand struct {
	arg   string
	flags func(*flag.FlagSet, *options)
	run   func(ctx context.Context, db *store.DB, o options, out io.Writer) error
}

func run(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 {
		return usage("no subcommand")
	}
	sub, ok := subcommands[args[0]]
	if !ok {
		return usage(fmt.Sprintf("unknown subcommand %q", args[0]))
	}
	defaultPath, err := defaultDBPath()
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var o options
	fs.StringVar(&o.dbPath, "db", defaultPath, "database file")
	sub.flags(fs, &o)
	if err := fs.Parse(hoistFlags(fs, args[1:])); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printUsage()
			return nil
		}
		return err
	}
	o.args = fs.Args()
	// "--" 之后的终止符自己留在 Args 里（F1），所以这里要再摘一次。
	for o.args != nil && o.args[0] == "--" {
		o.args = o.args[1:]
	}

	db, err := openDB(ctx, o.dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	return sub.run(ctx, db, o, out)
}
```

`printUsage()` 换成动态版（新 verb 由 T15-T17 注册进来，usage 不能再手写三行）：

```go
func printUsage() {
	names := make([]string, 0, len(subcommands))
	for name := range subcommands {
		names = append(names, name)
	}
	sort.Strings(names)
	fmt.Fprintf(os.Stderr, "usage: oac <%s> …  [-db PATH]\n", strings.Join(names, "|"))
	for _, name := range names {
		// 措辞刻意不说「exactly one」：`arg` 只描述位置参数是什么，arity 由
		// 各 verb 自己判（wantOne 报 "want exactly one …"，T18 的 runGraph
		// 允许 0 或 1 个）。这一行同时走 -h 与错误路径，写死了就会撒谎。
		fmt.Fprintf(os.Stderr, "  oac %-8s positional: %s\n", name, subcommands[name].arg)
	}
}
```

`scan`/`graph`/`impact` 三个 run 函数的签名换成 `(ctx, db, o options, out io.Writer)`，函数体内第一行取位置参数：

```go
func runScan(ctx context.Context, db *store.DB, o options, out io.Writer) error {
	root, err := wantOne(o, "path")
	if err != nil {
		return err
	}
	report, err := scanner.RunWithOptions(ctx, db, root, scanner.Options{Full: o.full})
	if err != nil {
		return err
	}
	if o.json {
		return writeJSON(out, report)
	}
	fmt.Fprintf(out, "repo %s\nsnapshot %s\nlanguage %s\nmodule %s\nelements %d relationships %d\n",
		report.RepoID, report.SnapshotID, report.Language, report.ModulePath,
		report.Elements, report.Relationships)
	fmt.Fprintf(out, "mode %s reused=%d parsed=%d\n", map[bool]string{true: "incremental", false: "full"}[report.Incremental],
		report.Reused, report.Parsed)
	// …… package errors 段原样保留 ……
	return nil
}
```

`runGraph`/`runImpact` 本任务只改签名与 `wantOne`，`--all/--repo` 的装载逻辑留给 T18（中间状态：多给的 `--repo` 被忽略，findings 里记一句「T18 补」）。

签名一改，切片 1 留下的 **7 个既有调用点全部编译失败**（`go vet ./...` 会一次报出来）。逐字迁移，不要顺手改断言：

`cmd/oac/parity_test.go:37`
```go
	if err := runImpact(ctx, db, options{args: []string{center}, depth: 2}, &buf); err != nil {
```

`cmd/oac/impact_test.go` 六处：
```go
	// :44
	err := runImpact(context.Background(), db,
		options{args: []string{"cmp:example.com/shared/core.Server"}, depth: 1}, &buf)
	// :58
	err := runImpact(context.Background(), db,
		options{args: []string{"cmp:nope.Nope"}, depth: 2}, &bytes.Buffer{})
	// :73
	if err := runScan(ctx, db, options{args: []string{root}}, &buf); err != nil {
	// :84
	if err := runGraph(ctx, db, options{args: []string{repoID}}, &buf); err != nil {
	// :92
	if err := runImpact(ctx, db,
		options{args: []string{"cmp:example.com/simple/internal/store.Cart"}, depth: 2}, &buf); err != nil {
	// :106
	err := runGraph(context.Background(), db, options{args: []string{repoID}}, &buf)
```

`:106` 那条（`TestRunGraphNoSnapshot`）额外注意：错误文案的断言（`repo:/never/scanned`、`no snapshot`、不带内层 `oac:` 前缀）由 T18 的 `loadScopedGraph` 继续满足——本任务把 `runGraph` 的取参换成 `wantOne` 之后，这条路径仍然要返回同一个 error 文本，**别把它改成 `t.Skip`**。

上面数的是 `runScan/runGraph/runImpact` 的 7 处。`run` 自己也从 `(ctx, args)` 变成 `(ctx, args, out)`，**实测还有 5 处 `run(` 调用点**（都在 `cmd/oac/impact_test.go`），`go vet` 会一起报出来：

```go
// :218 / :230 / :245 —— 只看不返回的 error，输出丢弃即可（"io" 已在 import 块里）
err := run(context.Background(), []string{"scan", "--db"}, io.Discard)
err = run(context.Background(), []string{"scan", root, "--db"}, io.Discard)
err := run(context.Background(), []string{"nosuch", "--db", dbPath}, io.Discard)
```

`:148` 与 `:155` 不能这么处理：它们在 `captureStdout` 里断言 stdout 内容，而新的 `run` 写的是 `out` 参数、不再碰 `os.Stdout`，套着 `captureStdout` 会读到空串、把两条测试变成假绿。改成显式传缓冲：

```go
	var buf bytes.Buffer
	if err := run(ctx, []string{"scan", root, "--db", dbPath}, &buf); err != nil {
		t.Fatalf("scan through run(): %v", err)
	}
	if !strings.Contains(buf.String(), "snapshot s") {
		t.Fatalf("scan through run():\n%s", buf.String())
	}

	buf.Reset()
	if err := run(ctx, []string{"impact", "cmp:example.com/shared/core.Server",
		"--depth", "1", "--db", dbPath}, &buf); err != nil {
		t.Fatalf("impact through run(): %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Upstream") || !strings.Contains(out, "depth 1") {
		t.Fatalf("impact through run():\n%s", out)
	}
```

断言的字符串（`snapshot s`、`Upstream`、`depth 1`）一个字不改。改完这两处后 `captureStdout`（`impact_test.go:257-276`）与它唯一的依赖 `stdoutToPipe`（`:279-287`）在包里再无调用者，一起删——`os.Stdout` 的重定向在新签名下没有存在理由。`"bytes"` 已在 import 块内。

- [ ] **Step 5: 全绿 + 提交**

```sh
go test ./cmd/oac/ -v
go vet ./... && go test ./...
git add cmd/oac/main.go cmd/oac/options.go cmd/oac/scan.go cmd/oac/graph.go cmd/oac/impact.go cmd/oac/main_test.go
git commit -m "refactor: oac 的 subcommand 形状改造（每 verb 自注册旗标 + --json 基建）"
```

---

### Task 15: `oac repo list|remove` 与 `oac scan --full` 的收尾

**Files:**
- Create: `cmd/oac/repo.go`
- Modify: `cmd/oac/main.go`（注册 `repo` verb）
- Test: `cmd/oac/repo_test.go`

**Interfaces:**
- Consumes: `DB.Repos`、`DB.Repo`、`DB.DeleteRepo`、`DB.LatestSnapshotID`、`DB.Elements`（T4）。
- Produces: `func runRepo(ctx, db, o, out) error`，位置参数 `list` | `remove <repo-id>`；`o.json` 时 list 输出数组。

- [ ] **Step 1: 写失败测试**

```go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestRepoListsScannedFixture(t *testing.T) {
	db, _, repoID := scanFixture(t, "shared-lib")
	var buf bytes.Buffer
	if err := runRepo(context.Background(), db, options{args: []string{"list"}}, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), repoID) || !strings.Contains(buf.String(), "example.com/shared") {
		t.Errorf("list output = %q", buf.String())
	}
}

func TestRepoListJSONCarriesSnapshot(t *testing.T) {
	db, snapshotID, repoID := scanFixture(t, "shared-lib")
	var buf bytes.Buffer
	if err := runRepo(context.Background(), db, options{args: []string{"list"}, json: true}, &buf); err != nil {
		t.Fatal(err)
	}
	var rows []repoRow
	if err := json.Unmarshal(buf.Bytes(), &rows); err != nil {
		t.Fatalf("json = %q: %v", buf.String(), err)
	}
	if len(rows) != 1 || rows[0].SnapshotID != snapshotID || rows[0].Elements == 0 {
		t.Errorf("rows = %+v, want the scanned repo with its snapshot id", rows)
	}
	if rows[0].RepoID != repoID {
		t.Errorf("repo id = %q", rows[0].RepoID)
	}
}

// 删除是真的级联：删完再 list 必须是空，且那个仓的快照查不到。
func TestRepoRemoveCascades(t *testing.T) {
	db, _, repoID := scanFixture(t, "shared-lib")
	var buf bytes.Buffer
	if err := runRepo(context.Background(), db,
		options{args: []string{"remove", repoID}}, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "removed") {
		t.Errorf("remove output = %q", buf.String())
	}
	repos, err := db.Repos(context.Background())
	if err != nil || len(repos) != 0 {
		t.Errorf("repos after remove = %+v, %v", repos, err)
	}
	if id, err := db.LatestSnapshotID(context.Background(), repoID); err != nil || id != "" {
		t.Errorf("snapshot survived repo deletion: %q, %v", id, err)
	}
}

func TestRepoRejectsBadArity(t *testing.T) {
	db, _, _ := scanFixture(t, "shared-lib")
	if err := runRepo(context.Background(), db, options{args: []string{"remove"}}, &bytes.Buffer{}); err == nil {
		t.Error("remove without an id must error")
	}
	if err := runRepo(context.Background(), db, options{args: []string{"rename"}}, &bytes.Buffer{}); err == nil {
		t.Error("unknown repo action must error")
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

```sh
go test ./cmd/oac/ -run TestRepo -v
```

期望：编译失败 `undefined: runRepo`。

- [ ] **Step 3: 实现**

```go
package main

import (
	"context"
	"fmt"
	"io"

	"github.com/futuremeng/OpenArchitect/internal/core/store"
)

// repoRow 是 `oac repo list --json` 的一行。Elements 计数读的是最新快照，
// 没有快照的仓记 0——「注册过但没扫」是真实状态（GUI 的 chips 也据此禁用）。
type repoRow struct {
	RepoID     string `json:"repo_id"`
	Name       string `json:"name"`
	LocalPath  string `json:"local_path"`
	Lang       string `json:"lang"`
	SnapshotID string `json:"snapshot_id,omitempty"`
	Elements   int    `json:"elements"`
}

func runRepo(ctx context.Context, db *store.DB, o options, out io.Writer) error {
	action, err := wantOne(o, "action (list | remove)")
	if err != nil {
		return err
	}
	switch action {
	case "list":
		return repoList(ctx, db, o, out)
	case "remove":
		if len(o.args) != 2 {
			return fmt.Errorf("repo remove wants exactly one repo id, got %v", o.args[1:])
		}
		repoID := o.args[1]
		if _, err := db.Repo(ctx, repoID); err != nil {
			return err
		}
		if err := db.DeleteRepo(ctx, repoID); err != nil {
			return err
		}
		fmt.Fprintf(out, "removed %s\n", repoID)
		return nil
	default:
		return fmt.Errorf("unknown repo action %q (want list or remove)", action)
	}
}

func repoList(ctx context.Context, db *store.DB, o options, out io.Writer) error {
	repos, err := db.Repos(ctx)
	if err != nil {
		return err
	}
	rows := make([]repoRow, 0, len(repos))
	for _, r := range repos {
		row := repoRow{RepoID: string(r.ID), Name: r.Name, LocalPath: r.LocalPath, Lang: r.Lang}
		id, err := db.LatestSnapshotID(ctx, r.ID)
		if err != nil {
			return err
		}
		row.SnapshotID = id
		if id != "" {
			els, err := db.Elements(ctx, id)
			if err != nil {
				return err
			}
			row.Elements = len(els)
		}
		rows = append(rows, row)
	}
	if o.json {
		return writeJSON(out, rows)
	}
	if len(rows) == 0 {
		fmt.Fprintln(out, "no repositories: run `oac scan <path>` first")
		return nil
	}
	for _, r := range rows {
		snapshot := "—"
		if r.SnapshotID != "" {
			snapshot = r.SnapshotID
		}
		fmt.Fprintf(out, "%s\t%s\t%s\t%s\t%d elements\tsnapshot %s\n",
			r.RepoID, r.Name, r.LocalPath, r.Lang, r.Elements, snapshot)
	}
	return nil
}
```

`main.go` 的 `subcommands` 加：

```go
	"repo": {arg: "action", flags: func(fs *flag.FlagSet, o *options) { registerJSONFlag(fs, o) }, run: runRepo},
```

- [ ] **Step 4: 确认 `--full` 已生效（T14 注册了旗标，这里钉行为）**

`cmd/oac/scan_test.go`（新文件）：

```go
package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/futuremeng/OpenArchitect/internal/scanner"
)

// --full 必须报 mode full / reused=0；默认增量必须报 reused>0。
// DoD-3 的 stdout 半边（平价测试在 T13 钉的是数据）。
func TestScanReportsReuseCounts(t *testing.T) {
	db, _, _ := scanFixture(t, "shared-lib") // scanFixture 已在 impact_test.go
	var first bytes.Buffer
	if err := runScan(ctxBackground(), db, options{args: []string{fixturePathByName(t, "crossmod-dep")}}, &first); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(first.String(), "mode full reused=0") {
		t.Errorf("first scan of a repo has no baseline: %q", first.String())
	}
	var again bytes.Buffer
	if err := runScan(ctxBackground(), db, options{args: []string{fixturePathByName(t, "crossmod-dep")}}, &again); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(again.String(), "mode incremental reused=") ||
		strings.Contains(again.String(), "reused=0 ") {
		t.Errorf("second scan must reuse: %q", again.String())
	}
	var forced bytes.Buffer
	err := runScan(ctxBackground(), db, options{args: []string{fixturePathByName(t, "crossmod-dep")}, full: true}, &forced)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(forced.String(), "mode full reused=0") {
		t.Errorf("--full must not reuse: %q", forced.String())
	}
}
```

`ctxBackground()`/`fixturePathByName()` 直接写成 `context.Background()` 与 `filepath.Join(fixturesRoot(), name)`（别造 wrapper）。断言里的 `reused=0 ` 空格是刻意的：`reused=0 parsed=` 才算「一次都没复用」。跑之前先 `scanner.RunWithOptions` 的 `Report.Incremental` 语义确认——`mode` 取的是 `report.Incremental`。

- [ ] **Step 5: 全绿 + 提交**

```sh
go test ./cmd/oac/ -run 'TestRepo|TestScanReports' -v
go vet ./... && go test ./...
git add cmd/oac/repo.go cmd/oac/repo_test.go cmd/oac/scan.go cmd/oac/scan_test.go cmd/oac/main.go
git commit -m "feat: oac repo list|remove 与 scan 的 reused/parsed 计数输出"
```

---

### Task 16: `oac diff` 与 `oac history`

**Files:**
- Create: `cmd/oac/diff.go`、`cmd/oac/history.go`
- Modify: `cmd/oac/main.go`
- Test: `cmd/oac/diff_test.go`、`cmd/oac/history_test.go`

**Interfaces:**
- Consumes: `DB.Snapshots`、`DB.Elements`/`DB.Relationships`、`graph.DiffSnapshots`（T6）、`DB.History`/`HistoryCount`（T3）、`loadGraph`（`cmd/oac/graph.go:16`）。
- Produces: `runDiff`、`runHistory` 与各自的 JSON 结构。

- [ ] **Step 1: 写失败测试**

```go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/futuremeng/OpenArchitect/internal/scanner"
)

// 助手复用约定（不要再造同义函数）：`openTempDB` / `copyFixture` / `writeAppend`
// 已经在 T13 的 `cmd/oac/incremental_parity_test.go` 里定义（同包，直接可用）。
// 本任务只新增 `createFile` 一个——appendFile/write 这类名字仓库里不存在。
func createFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDiffDefaultsToLatestTwo(t *testing.T) {
	db := openTempDB(t)
	defer db.Close()
	root := copyFixture(t, "shared-lib", filepath.Join(t.TempDir(), "lib"))
	first, err := scanner.Run(context.Background(), db, root)
	if err != nil {
		t.Fatal(err)
	}
	writeAppend(t, filepath.Join(root, "core", "core.go"), "\n// second scan\n")
	second, err := scanner.Run(context.Background(), db, root)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	err = runDiff(context.Background(), db, options{args: []string{second.RepoID}}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{first.SnapshotID, second.SnapshotID, "elements", "edges"} {
		if !strings.Contains(out, want) {
			t.Errorf("diff output missing %q:\n%s", want, out)
		}
	}
}

// DoD-4：脚本化改动产生的变更集必须恰好是它。加一个类型 → 恰 1 个 +元素；
// 不多报（权重变化不是边变更，T6 已钉）。
func TestDiffReportsExactlyTheScriptedChange(t *testing.T) {
	db := openTempDB(t)
	defer db.Close()
	root := copyFixture(t, "shared-lib", filepath.Join(t.TempDir(), "lib"))
	if _, err := scanner.Run(context.Background(), db, root); err != nil {
		t.Fatal(err)
	}
	createFile(t, filepath.Join(root, "core", "added.go"), "package core\n\ntype Extra struct{}\n")
	second, err := scanner.Run(context.Background(), db, root)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := runDiff(context.Background(), db,
		options{args: []string{second.RepoID}, json: true}, &buf); err != nil {
		t.Fatal(err)
	}
	var payload diffPayload
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("json: %v\n%s", err, buf.String())
	}
	if len(payload.Elements.Added) != 1 ||
		!strings.HasSuffix(payload.Elements.Added[0].ID, "core.Extra") {
		t.Errorf("added = %+v, want exactly cmp:…core.Extra", payload.Elements.Added)
	}
	if len(payload.Elements.Changed) != 0 || len(payload.Elements.Removed) != 0 {
		t.Errorf("unrelated elements reported: %+v", payload.Elements)
	}
}

func TestDiffRejectsUnknownRepoAndSingleSnapshot(t *testing.T) {
	// scanFixture 只扫一次，并且自己注册了 db.Close（没有 openTempDB + defer）：
	// 这个库里每个仓恰好 1 个快照，正好是「无从对比」的形态。
	db, _, repoID := scanFixture(t, "shared-lib")
	if err := runDiff(context.Background(), db, options{args: []string{"repo:/nope"}}, &bytes.Buffer{}); err == nil {
		t.Error("unknown repo must error")
	}
	if err := runDiff(context.Background(), db, options{args: []string{repoID}}, &bytes.Buffer{}); err == nil {
		t.Error("one snapshot cannot be diffed against itself")
	}
}
```

```go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/futuremeng/OpenArchitect/internal/scanner"
)

// DoD-5：history 必须给出该元素完整的 appeared/changed/disappeared 序列。
// 变更脚本与 T12 的 TestScanRecordsElementHistory 同形（追加一个方法 → changed；
// 全树改名 → disappeared），但这里通过 CLI 读回，钉的是 `oac history` 而不是
// core 的行。fixture 的 .go 文件恰好三个（实测 find
// testdata/fixtures/shared-lib -type f）：core/core.go、edge/edge.go、
// cmd/app/main.go，后两个都引用 core.Server。
const historyTarget = "cmp:example.com/shared/core.Server"

// seedHistorySequence 跑完 appeared → changed → disappeared 三次扫描，返回打开的
// 库（Close 交给 t.Cleanup）。两条测试共用同一段文件操作，差别只在读回方式。
func seedHistorySequence(t *testing.T) *store.DB {
	t.Helper()
	db := openTempDB(t)
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	root := copyFixture(t, "shared-lib", filepath.Join(t.TempDir(), "lib"))
	if _, err := scanner.Run(ctx, db, root); err != nil {
		t.Fatal(err)
	}
	// changed：Server 原本只有 Addr（fixture 实测 core/core.go:12），追加一个
	// 导出方法只动 methods_json 一列。
	written := filepath.Join(root, "core", "core.go")
	data, err := os.ReadFile(written)
	if err != nil {
		t.Fatal(err)
	}
	withMethod := string(data) + "\n// Extra widens the method list.\nfunc (s *Server) Extra() {}\n"
	if err := os.WriteFile(written, []byte(withMethod), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := scanner.Run(ctx, db, root); err != nil {
		t.Fatal(err)
	}
	// disappeared：全树改名 Server → Relic。只改声明处会让 edge 与 cmd/app 编译
	// 失败（两处引用逐字：edge/edge.go:6 的 `server core.Server`、
	// cmd/app/main.go:9 的 `_ = core.Server{}`），包错误会让整包掉出图，事件序列
	// 就不再是「恰好一条 disappeared」。
	for _, p := range []string{
		filepath.Join(root, "core", "core.go"),
		filepath.Join(root, "edge", "edge.go"),
		filepath.Join(root, "cmd", "app", "main.go"),
	} {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(strings.ReplaceAll(string(b), "Server", "Relic")), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := scanner.Run(ctx, db, root); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestHistoryReturnsFullSequenceNewestFirst(t *testing.T) {
	db := seedHistorySequence(t)
	var buf bytes.Buffer
	if err := runHistory(context.Background(), db,
		options{args: []string{historyTarget}, limit: 10}, &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"disappeared", "changed", "methods_json", "appeared"} {
		if !strings.Contains(out, want) {
			t.Errorf("history output missing %q:\n%s", want, out)
		}
	}
	if strings.Index(out, "disappeared") > strings.Index(out, "appeared") {
		t.Error("history must print newest first")
	}
}

// DoD-5 的机器可读半边。--json 的键名是切片 7 消费者的契约（spec §8），所以
// Step 4 必须把它钉在 historyPayload 的 tag 上，而不是用匿名 map ——
// 键名散落在字面量里，改错了没有测试会红。
func TestHistoryJSONPinsThePayloadShape(t *testing.T) {
	db := seedHistorySequence(t)
	var buf bytes.Buffer
	if err := runHistory(context.Background(), db,
		options{args: []string{historyTarget}, limit: 10, json: true}, &buf); err != nil {
		t.Fatal(err)
	}
	var payload historyPayload
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("json: %v\n%s", err, buf.String())
	}
	if payload.ElementID != historyTarget || len(payload.Events) != 3 {
		t.Fatalf("payload = %+v, want 3 events for %s", payload, historyTarget)
	}
	if payload.Total != 3 {
		t.Errorf("total = %d, want 3", payload.Total)
	}
	if payload.Events[0].Event != "disappeared" || payload.Events[2].Event != "appeared" {
		t.Errorf("events = %+v, want newest first", payload.Events)
	}
	if strings.Join(payload.Events[1].Fields, ",") != "methods_json" {
		t.Errorf("changed fields = %v, want [methods_json]", payload.Events[1].Fields)
	}
}
```

`history_test.go` 的 import 块就是上面那一份（`bytes/context/encoding/json/os/path/filepath/strings/testing` + `internal/core/store` + `internal/scanner`）。**别留省略号，也别再造 `appendFile`/`write` 这类仓库里不存在的助手**：追加写成 `os.WriteFile(data + extra)`，整文件新建用本任务的 `createFile`；`openTempDB`/`copyFixture` 复用 T13 已在同包定义的版本（`store.DB` 的 Close 由 `t.Cleanup` 管，`openTempDB` 不注册清理）。

- [ ] **Step 2: 跑测试确认失败**

```sh
go test ./cmd/oac/ -run 'TestDiff|TestHistory' -v
```

期望：编译失败 `undefined: runDiff`。

- [ ] **Step 3: 实现 `diff.go`**

```go
package main

import (
	"context"
	"fmt"
	"io"

	"github.com/futuremeng/OpenArchitect/internal/core/graph"
	"github.com/futuremeng/OpenArchitect/internal/core/model"
	"github.com/futuremeng/OpenArchitect/internal/core/store"
)

// diffPayload 是 --json 的形状。字段名与 spec §8 的三段一致，切片 7 的
// 程序化消费者会直接吃它，所以这里的命名是契约。
type diffPayload struct {
	RepoID   string `json:"repo_id"`
	From     string `json:"from"`
	To       string `json:"to"`
	Elements struct {
		Added   []model.Element      `json:"added"`
		Removed []model.Element      `json:"removed"`
		Changed []changedElementJSON `json:"changed"`
	} `json:"elements"`
	Edges struct {
		Added   []model.Relationship `json:"added"`
		Removed []model.Relationship `json:"removed"`
	} `json:"edges"`
}

// changedElementJSON 单独成类型：payload 里声明一次、装配时再写一遍匿名结构体
// 是同一个形状的复制粘贴，改字段名时必漏一处。
type changedElementJSON struct {
	ID     string   `json:"id"`
	Fields []string `json:"fields"`
}

func runDiff(ctx context.Context, db *store.DB, o options, out io.Writer) error {
	repoID, err := wantOne(o, "repo id")
	if err != nil {
		return err
	}
	from, to := o.from, o.to
	if from == "" || to == "" {
		// 默认取该仓最新的两个快照（spec §9）。少于两个就没有「变更」可言。
		ids, err := db.Snapshots(ctx, repoID, 2)
		if err != nil {
			return err
		}
		if len(ids) < 2 {
			return fmt.Errorf("repo %q has %d snapshot(s); diff needs two (or pass --from/--to)", repoID, len(ids))
		}
		to, from = ids[0].ID, ids[1].ID
	}
	a, err := loadGraph(ctx, db, from)
	if err != nil {
		return err
	}
	b, err := loadGraph(ctx, db, to)
	if err != nil {
		return err
	}
	d := graph.DiffSnapshots(a, b)
	payload := diffPayload{RepoID: repoID, From: from, To: to}
	payload.Elements.Added = d.Added
	payload.Elements.Removed = d.Removed
	payload.Edges.Added = d.EdgesAdded
	payload.Edges.Removed = d.EdgesRemoved
	for _, c := range d.Changed {
		payload.Elements.Changed = append(payload.Elements.Changed,
			changedElementJSON{ID: c.To.ID, Fields: c.Fields})
	}
	if o.json {
		return writeJSON(out, payload)
	}
	fmt.Fprintf(out, "repo %s  %s → %s\n", repoID, from, to)
	fmt.Fprintf(out, "elements: +%d ~%d -%d\tnodes edges: +%d -%d\n",
		len(d.Added), len(d.Changed), len(d.Removed), len(d.EdgesAdded), len(d.EdgesRemoved))
	for _, el := range d.Added {
		fmt.Fprintf(out, "  + %s\t%s\t%s\n", el.ID, el.Type, el.Name)
	}
	for _, c := range d.Changed {
		fmt.Fprintf(out, "  ~ %s\t%s\t[%s]\n", c.To.ID, c.To.Name, joinFields(c.Fields))
	}
	for _, el := range d.Removed {
		fmt.Fprintf(out, "  - %s\t%s\t%s\n", el.ID, el.Type, el.Name)
	}
	for _, rel := range d.EdgesAdded {
		fmt.Fprintf(out, "  + %s -> %s [%s]\n", rel.FromID, rel.ToID, rel.Type)
	}
	for _, rel := range d.EdgesRemoved {
		fmt.Fprintf(out, "  - %s -> %s [%s]\n", rel.FromID, rel.ToID, rel.Type)
	}
	return nil
}

func joinFields(fields []string) string {
	out := ""
	for i, f := range fields {
		if i > 0 {
			out += ", "
		}
		out += f
	}
	return out
}
```

> `joinFields` 定义在这一份里（`diff.go`），同包的 `history.go`（Step 4）直接复用——**不要再抄一份**。空变更集走 Go 的 nil slice，`--json` 里是 `null` 而不是 `[]`：消费方用 `len()` 读，两种都安全，spec §8 也没要求区分，所以不加 `[]` 兜底代码。

- [ ] **Step 4: 实现 `history.go`**

```go
package main

import (
	"context"
	"fmt"
	"io"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
	"github.com/futuremeng/OpenArchitect/internal/core/store"
)

const defaultHistoryLimit = 5

// historyPayload 是 --json 的形状，与 TestHistoryJSONPinsThePayloadShape 一对
// 契约。model.HistoryEvent 本身不带 tag（T1 定稿），所以这里显式转一层，
// 事件行的字段名由这个结构体负责。
type historyPayload struct {
	ElementID string             `json:"element_id"`
	Total     int                `json:"total"`
	Events    []historyEventJSON `json:"events"`
}

type historyEventJSON struct {
	SnapshotID string   `json:"snapshot_id"`
	CreatedAt  string   `json:"created_at"`
	Event      string   `json:"event"`
	Fields     []string `json:"fields,omitempty"`
}

func runHistory(ctx context.Context, db *store.DB, o options, out io.Writer) error {
	elementID, err := wantOne(o, "element id")
	if err != nil {
		return err
	}
	limit := o.limit
	if limit <= 0 {
		limit = defaultHistoryLimit
	}
	events, err := db.History(ctx, elementID, limit)
	if err != nil {
		return err
	}
	total, err := db.HistoryCount(ctx, elementID)
	if err != nil {
		return err
	}
	if o.json {
		payload := historyPayload{ElementID: elementID, Total: total,
			Events: make([]historyEventJSON, 0, len(events))}
		for _, ev := range events {
			payload.Events = append(payload.Events, historyEventJSON{
				SnapshotID: ev.SnapshotID, CreatedAt: ev.CreatedAt,
				Event: ev.Event, Fields: ev.Fields,
			})
		}
		return writeJSON(out, payload)
	}
	if len(events) == 0 {
		fmt.Fprintf(out, "%s 没有历史记录（元素不存在，或它自始未变）\n", elementID)
		return nil
	}
	// 封顶是产品事实（spec §5 / §13）：只保留最近 N 个快照，所以「一生」只在
	// 窗口内完整。文案必须诚实，别让用户以为看到了全部。
	fmt.Fprintf(out, "%s  %d 条记录（最近 %d 次扫描内，显示 %d 条）\n",
		elementID, total, limit, min(len(events), limit))
	for _, ev := range events {
		detail := ""
		if len(ev.Fields) > 0 {
			detail = "\t[" + joinFields(ev.Fields) + "]"
		}
		fmt.Fprintf(out, "  %s  %s  %s%s\n", ev.SnapshotID, ev.CreatedAt, ev.Event, detail)
	}
	return nil
}
```

`history.go` 不用 `model`（事件行的类型由 `db.History` 的返回值推断），Step 4 代码块开头那份 import 里**删掉 `internal/core/model` 这一行**，否则 `imported and not used` 编译失败。注册进 `main.go`：

```go
	"diff": {arg: "repo id", flags: func(fs *flag.FlagSet, o *options) {
		fs.StringVar(&o.from, "from", "", "older snapshot id")
		fs.StringVar(&o.to, "to", "", "newer snapshot id")
		registerJSONFlag(fs, o)
	}, run: runDiff},
	"history": {arg: "element id", flags: func(fs *flag.FlagSet, o *options) {
		fs.IntVar(&o.limit, "limit", defaultHistoryLimit, "events to show, newest first")
		registerJSONFlag(fs, o)
	}, run: runHistory},
```

- [ ] **Step 5: 全绿 + 提交**

```sh
go test ./cmd/oac/ -run 'TestDiff|TestHistory' -v
go vet ./... && go test ./...
git add cmd/oac/diff.go cmd/oac/history.go cmd/oac/diff_test.go cmd/oac/history_test.go cmd/oac/main.go
git commit -m "feat: oac diff 与 oac history（默认对取最新两个快照 / 最近 5 条事件）"
```

---

### Task 17: `oac rank` 与 `oac verify`

**Files:**
- Create: `cmd/oac/rank.go`、`cmd/oac/verify.go`
- Modify: `cmd/oac/main.go`
- Test: `cmd/oac/rank_test.go`、`cmd/oac/verify_test.go`

**Interfaces:**
- Consumes: `graph.RankDependents`（T7）、`scanner.Verify`（T11）、T18 的 `loadScopedGraph` 与它的 `scopedDB` 双仓装配。**执行顺序硬约束：T18 早于本任务**——`runRank` 直接调 `loadScopedGraph`，`TestRankDefaultsToMergedScope` 直接调 `scopedDB`，两者都在 T18 落地（见文末速查）。
- Produces: `runRank`、`runVerify`。

- [ ] **Step 1: 写失败测试**

```go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// DoD-7：--limit 生效、排序正确、--by 的非 dependents 取值被拒（本切片只实现
// 一种度量，静默忽略未知值等于给用户一个假旗标）。
// rank 不接受位置参数：作用域只由 --repo（可重复）与 --all 决定，二者都不给
// = 全库合并——「谁最关键」这个问题默认只看一个仓会把跨仓 load-bearing 切掉。
func TestRankOrdersByDependentsAndLimits(t *testing.T) {
	db, _, _ := scanFixture(t, "shared-lib")
	var buf bytes.Buffer
	if err := runRank(context.Background(), db,
		options{by: "dependents", limit: 3}, &buf); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) > 3 {
		t.Errorf("--limit 3 gave %d lines:\n%s", len(lines), buf.String())
	}
	if !strings.Contains(buf.String(), "core.Server") {
		t.Logf("rank output (fixture 拓扑决定具体内容):\n%s", buf.String())
	}
}

// 位置参数一律拒：`oac rank repo:/x` 是 `oac graph` 的肌肉记忆串了台，
// 静默接受会给出一个用户没想要的排序。
func TestRankRejectsPositionalArg(t *testing.T) {
	db, _, repoID := scanFixture(t, "shared-lib")
	err := runRank(context.Background(), db, options{args: []string{repoID}}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "--repo") {
		t.Errorf("positional arg must be rejected with a scope hint, got %v", err)
	}
}

func TestRankRejectsUnknownMetric(t *testing.T) {
	db, _, _ := scanFixture(t, "shared-lib")
	err := runRank(context.Background(), db, options{by: "hype"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "dependents") {
		t.Errorf("unknown --by must name the supported metric, got %v", err)
	}
}

func TestRankJSONCarriesScores(t *testing.T) {
	db, _, _ := scanFixture(t, "shared-lib")
	var buf bytes.Buffer
	if err := runRank(context.Background(), db,
		options{by: "dependents", json: true, limit: 10}, &buf); err != nil {
		t.Fatal(err)
	}
	var hits []rankRow
	if err := json.Unmarshal(buf.Bytes(), &hits); err != nil {
		t.Fatalf("json %q: %v", buf.String(), err)
	}
	if len(hits) == 0 || hits[0].Dependents < hits[len(hits)-1].Dependents {
		t.Errorf("rows not descending: %+v", hits)
	}
}

// 默认作用域 = 全库合并，这是 rank 与切片 1 单仓视角的真正区别。
// scopedDB 是 T18 的 cmd/oac/scope_test.go 里的双仓装配（dep 先 app 后），
// 所以执行顺序上 T18 早于本任务（见文末速查）。两仓的组件 id 都出现在同一份
// 排名里，才证明 rank 读的是合并图。
func TestRankDefaultsToMergedScope(t *testing.T) {
	db, _, _ := scopedDB(t)
	defer db.Close()
	var buf bytes.Buffer
	if err := runRank(context.Background(), db,
		options{by: "dependents", json: true, limit: 200}, &buf); err != nil {
		t.Fatal(err)
	}
	var hits []rankRow
	if err := json.Unmarshal(buf.Bytes(), &hits); err != nil {
		t.Fatalf("json %q: %v", buf.String(), err)
	}
	var app, dep int
	for _, h := range hits {
		switch {
		case strings.HasPrefix(h.ID, "cmp:crossmod.example/app"):
			app++
		case strings.HasPrefix(h.ID, "cmp:crossmod.example/dep"):
			dep++
		}
	}
	if app == 0 || dep == 0 {
		t.Errorf("默认作用域漏了一仓：app=%d dep=%d，输出 %d 行", app, dep, len(hits))
	}
}
```

`oac rank` 的**位置参数一律拒**（定稿见 `TestRankRejectsPositionalArg`）：作用域只由 `--repo`（可重复）与 `--all` 决定，二者都不给 = 全库合并。草稿里这里写过 `repoPlaceholder()` 与一个 `repo:/…` 位置参数，那是把 `oac graph` 的形状抄错了，已删——`rank` 与 `graph` 在这一点上故意不同：`graph` 的位置参数是「看哪个仓」，`rank` 的分数只有在合并图上才有意义。

```go
package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/futuremeng/OpenArchitect/internal/scanner"
)

// DoD-6：扫描后 clean；touch 任一文件后 stale 且列出该文件。
func TestVerifyCleanThenStale(t *testing.T) {
	db := openTempDB(t)
	defer db.Close()
	root := copyFixture(t, "shared-lib", filepath.Join(t.TempDir(), "lib"))
	report, err := scanner.Run(context.Background(), db, root)
	if err != nil {
		t.Fatal(err)
	}
	var clean bytes.Buffer
	if err := runVerify(context.Background(), db, options{args: []string{report.RepoID}}, &clean); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(clean.String(), "clean") {
		t.Errorf("fresh scan must verify clean: %q", clean.String())
	}
	appendFile(t, filepath.Join(root, "core", "core.go"), "\n// stale\n")
	var stale bytes.Buffer
	if err := runVerify(context.Background(), db, options{args: []string{report.RepoID}}, &stale); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stale.String(), "stale") || !strings.Contains(stale.String(), "core/core.go") {
		t.Errorf("verify must name the touched file: %q", stale.String())
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

```sh
go test ./cmd/oac/ -run 'TestRank|TestVerify' -v
```

期望：编译失败 `undefined: runRank` / `runVerify`。

- [ ] **Step 3: 实现 `rank.go`**

```go
package main

import (
	"context"
	"fmt"
	"io"

	"github.com/futuremeng/OpenArchitect/internal/core/graph"
	"github.com/futuremeng/OpenArchitect/internal/core/model"
	"github.com/futuremeng/OpenArchitect/internal/core/store"
)

// rankRow 是 --json 的一行。Element 摊平成四个可读字段：切片 7 的程序化消费者
// 不该去猜 model.Element 的 JSON 形状会不会变。
type rankRow struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	RepoID     string `json:"repo_id"`
	Dependents int    `json:"dependents"`
}

const rankMetric = "dependents"

func runRank(ctx context.Context, db *store.DB, o options, out io.Writer) error {
	if len(o.args) > 0 {
		return fmt.Errorf("rank takes no positional argument; scope it with --repo/--all")
	}
	if o.by != "" && o.by != rankMetric {
		return fmt.Errorf("unknown --by %q: this slice implements %q", o.by, rankMetric)
	}
	g, _, err := loadScopedGraph(ctx, db, o)
	if err != nil {
		return err
	}
	hits := graph.RankDependents(g)
	if o.limit > 0 && len(hits) > o.limit {
		hits = hits[:o.limit]
	}
	if o.json {
		rows := make([]rankRow, 0, len(hits))
		for _, h := range hits {
			rows = append(rows, rankRow{ID: h.Element.ID, Name: h.Element.Name,
				Type: string(h.Element.Type), RepoID: h.Element.RepoID, Dependents: h.Dependents})
		}
		return writeJSON(out, rows)
	}
	if len(hits) == 0 {
		fmt.Fprintln(out, "nothing has inbound calls or depends_on edges")
		return nil
	}
	for i, h := range hits {
		fmt.Fprintf(out, "%2d  %3d  %s\t%s\t%s\n", i+1, h.Dependents,
			h.Element.ID, h.Element.Type, h.Element.Name)
	}
	return nil
}
```

`model` 若没直接用到就删 import。

- [ ] **Step 4: 实现 `verify.go`**

```go
package main

import (
	"context"
	"fmt"
	"io"

	"github.com/futuremeng/OpenArchitect/internal/core/store"
	"github.com/futuremeng/OpenArchitect/internal/scanner"
)

// runVerify 不做缓存：verify 的代价是遍历仓库文件 stat+hash，GUI 那边把它
// 放在后台 goroutine（T23），CLI 直接现算——用户敲这条命令就是要一个此刻的答案。
func runVerify(ctx context.Context, db *store.DB, o options, out io.Writer) error {
	repoID, err := wantOne(o, "repo id")
	if err != nil {
		return err
	}
	res, err := scanner.Verify(ctx, db, repoID)
	if err != nil {
		return err
	}
	if o.json {
		return writeJSON(out, res)
	}
	verdict := "stale"
	if res.Clean {
		verdict = "clean"
	}
	fmt.Fprintf(out, "repo %s  snapshot %s  %s\n", res.RepoID, res.SnapshotID, verdict)
	writeList(out, "changed", res.Changed)
	writeList(out, "missing", res.Missing)
	if len(res.Untracked) > 0 {
		fmt.Fprintln(out, "untracked (in the repo but not in the graph; build-tag-excluded files land here):")
		writeList(out, "", res.Untracked)
	}
	return nil
}

func writeList(out io.Writer, label string, items []string) {
	if len(items) == 0 {
		return
	}
	if label != "" {
		fmt.Fprintf(out, "%s (%d):\n", label, len(items))
	}
	for _, item := range items {
		fmt.Fprintf(out, "  %s\n", item)
	}
}
```

注册：

```go
	"rank": {arg: "nothing (--repo/--all)", flags: func(fs *flag.FlagSet, o *options) {
		fs.StringVar(&o.by, "by", rankMetric, "ranking metric (dependents)")
		fs.IntVar(&o.limit, "limit", 10, "rows to print")
		fs.BoolVar(&o.all, "all", false, "merge every repository")
		registerRepoFlag(fs, o)
		registerJSONFlag(fs, o)
	}, run: runRank},
	"verify": {arg: "repo id", flags: func(fs *flag.FlagSet, o *options) { registerJSONFlag(fs, o) }, run: runVerify},
```

> `--all` 的默认必须是 **false**，即使 rank 的默认作用域就是全库。原因：T18 的 `loadScopedGraph` 里 `o.all == true` 会把 `wanted` 清空（= 不过滤），所以一个 true 默认会让 `oac rank --repo R` 悄悄把 R 丢掉、返回全库排名——旗标默认与用户意图相反。而「谁都没指 = 全库」这件事本来就由同一条规则免费给出（`wanted` 为空 = 不过滤），不需要 flag 默认替它说话。测试断言的是行为不是默认值：`TestRankDefaultsToMergedScope`（不带旗标 → 两仓的组件出现在同一份排名里）与 `TestScopeNamedReposOnly`（T18，`--repo` 只并那一仓）。

- [ ] **Step 5: 全绿 + 提交**

```sh
go test ./cmd/oac/ -run 'TestRank|TestVerify' -v
go vet ./... && go test ./...
git add cmd/oac/rank.go cmd/oac/verify.go cmd/oac/rank_test.go cmd/oac/verify_test.go cmd/oac/main.go
git commit -m "feat: oac rank（load-bearing 排序）与 oac verify（两值裁决三清单）"
```

---

### Task 18: `graph` / `impact` 的 `--all` 与 `--repo`（CLI 侧合并视图）

**Files:**
- Create: `cmd/oac/scope.go`
- Modify: `cmd/oac/graph.go`、`cmd/oac/impact.go`
- Test: `cmd/oac/scope_test.go`

**Interfaces:**
- Consumes: `DB.Repos`、`DB.LatestSnapshotID`、`DB.Elements/Relationships`、`graph.Build`、`graph.Merge`（T5）。
- Produces:

```go
// loadScopedGraph 是 CLI 侧唯一的「按旗标选仓 → 并集」路径。
// o.all → 所有带快照的仓；o.repos 非空 → 指名的那些；两者都不给 → 不过滤，
// 全部带快照的仓一起并（与 GUI 的 GraphRepos 为空默认对齐）。
// 它不读 o.args：位置参数的含义按子命令不同（graph 是 repo id，impact/rank
// 是 element id），翻译那一步放在 runGraph 里做一次。
// 返回的 warnings 由调用方打进 stderr，不污染 --json 的输出。
func loadScopedGraph(ctx context.Context, db *store.DB, o options) (*graph.Graph, []string, error)
```

测试侧同时产出 `scopedDB(t) (*store.DB, []string, map[string]int)`（`cmd/oac/scope_test.go`，dep 先 app 后的双仓装配）：`cmd/oac` 包里只有这一份双仓装配，T17 的 `TestRankDefaultsToMergedScope` 复用它，不要再造第二个。（T24 的 PoC-8 在 `internal/scanner` 包里，跨包不能共享测试助手，它用自己的 `newScanTestDB` + `fixturePath`。）

DoD-2：仓库过滤器切到仅 A / 仅 B / 全部，三种视图计数与各自快照一致——GUI 半边 T5 已钉，这里钉 CLI 半边。

- [ ] **Step 1: 写失败测试**

```go
package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
	"github.com/futuremeng/OpenArchitect/internal/core/store"
	"github.com/futuremeng/OpenArchitect/internal/scanner"
)

// scopedDB 把两个互相引用的 fixture 扫进同一个临时库，返回库、按扫描顺序的
// repo id 切片、每个仓的元素数。
//
// 返回切片而不是只返回 map：--all 那条测试要的是「前一个 + 后一个」的和，
// map 的键序随机（虽然断言彼此独立不会 flaky，但求和这件事需要一个明确的顺序才写得出来）。
// openTempDB / fixturesRoot 来自 T13 的 cmd/oac/incremental_parity_test.go。
func scopedDB(t *testing.T) (*store.DB, []string, map[string]int) {
	t.Helper()
	db := openTempDB(t)
	counts := map[string]int{}
	ids := make([]string, 0, 2)
	for _, name := range []string{"crossmod-dep", "crossmod-app"} {
		report, err := scanner.Run(context.Background(), db, filepath.Join(fixturesRoot(), name))
		if err != nil {
			t.Fatal(err)
		}
		els, err := db.Elements(context.Background(), report.SnapshotID)
		if err != nil {
			t.Fatal(err)
		}
		counts[report.RepoID] = len(els)
		ids = append(ids, report.RepoID)
	}
	return db, ids, counts
}

// TestScopeAllMergesBothFixtures: DoD-2 的 CLI 半边。--all 的元素数必须正好是
// 两仓之和——两仓模块路径不同（crossmod.example/app 与 …/dep），没有同名 id，
// 并集不该去重掉任何东西。少了就是 Merge 吞了节点，多了不可能。
func TestScopeAllMergesBothFixtures(t *testing.T) {
	db, ids, counts := scopedDB(t)
	defer db.Close()
	want := "merged " + strconv.Itoa(counts[ids[0]]+counts[ids[1]]) + " elements"
	var buf bytes.Buffer
	if err := runGraph(context.Background(), db, options{all: true}, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), want) {
		t.Errorf("--all 输出缺 %q，实际：\n%s", want, buf.String())
	}
	// 两仓的元素都必须在输出里各占一行（计数对了但只画了一个仓，是 Merge 的
	// 并集写成了覆盖）。
	for _, prefix := range []string{"cmp:crossmod.example/app", "cmp:crossmod.example/dep"} {
		if !strings.Contains(buf.String(), prefix) {
			t.Errorf("--all 输出里没有 %s 的元素：\n%s", prefix, buf.String())
		}
	}
}

// TestScopeNamedReposOnly: --repo 指名单仓时，计数与那一仓自己的快照一致。
// 循环里的断言只依赖当前键，与 map 迭代顺序无关。
func TestScopeNamedReposOnly(t *testing.T) {
	db, _, counts := scopedDB(t)
	defer db.Close()
	for repoID, n := range counts {
		var buf bytes.Buffer
		err := runGraph(context.Background(), db, options{repos: []string{repoID}}, &buf)
		if err != nil {
			t.Fatalf("scope %s: %v", repoID, err)
		}
		want := "merged " + strconv.Itoa(n) + " elements"
		if !strings.Contains(buf.String(), want) {
			t.Errorf("scope %s 缺 %q，实际：\n%s", repoID, want, buf.String())
		}
	}
}

// TestScopeLegacyPositionalArg: 切片 1 的 `oac graph <repo-id>` 形态继续有效，
// 等价于 --repo <repo-id>。README 与用户的肌肉记忆都压在它身上。
func TestScopeLegacyPositionalArg(t *testing.T) {
	db, ids, counts := scopedDB(t)
	defer db.Close()
	var buf bytes.Buffer
	if err := runGraph(context.Background(), db, options{args: []string{ids[0]}}, &buf); err != nil {
		t.Fatal(err)
	}
	want := "merged " + strconv.Itoa(counts[ids[0]]) + " elements"
	if !strings.Contains(buf.String(), want) {
		t.Errorf("位置参数作用域缺 %q：\n%s", want, buf.String())
	}
}

// TestScopeAllSkipsRepoWithoutSnapshot: --all 撞上没扫过的仓要跳过而不是报错。
// 用户点 --all 的意思是「把有的都并上」，一个空仓不构成失败。
func TestScopeAllSkipsRepoWithoutSnapshot(t *testing.T) {
	db, ids, counts := scopedDB(t)
	defer db.Close()
	const ghost = "/tmp/never-scanned"
	if err := db.UpsertRepo(context.Background(), model.Repo{
		ID: model.RepoIDFor(ghost), Name: "ghost", LocalPath: ghost, Lang: "go"}); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := runGraph(context.Background(), db, options{all: true}, &buf); err != nil {
		t.Fatalf("--all 被没快照的仓挡住了：%v", err)
	}
	want := "merged " + strconv.Itoa(counts[ids[0]]+counts[ids[1]]) + " elements"
	if !strings.Contains(buf.String(), want) {
		t.Errorf("跳过空仓后计数变了：\n%s", buf.String())
	}
}

// TestScopeRejectsUnknownRepo: 指名（--repo 或位置参数）的仓没有快照必须报错——
// 与上一条不矛盾：--all 是「有的都并上」，指名是「我就要看这个」。
func TestScopeRejectsUnknownRepo(t *testing.T) {
	db, _, _ := scopedDB(t)
	defer db.Close()
	err := runGraph(context.Background(), db, options{repos: []string{"repo:/ghost"}}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("未知 --repo 必须报错，不能静默给一张空图")
	}
}
```

`scopedDB` 的三返回值形状同时被本任务 Step 5 与 T17 的 rank 作用域测试消费；`runGraph` 的首行输出 `merged <n> elements, <m> relationships` 由 Step 4 定义，两处必须逐字一致。


- [ ] **Step 2: 跑测试确认失败**

```sh
go test ./cmd/oac/ -run TestScope -v
```

期望：编译失败 `undefined: loadScopedGraph`。

- [ ] **Step 3: 实现 `scope.go`**

```go
package main

import (
	"context"
	"fmt"

	"github.com/futuremeng/OpenArchitect/internal/core/graph"
	"github.com/futuremeng/OpenArchitect/internal/core/store"
)

func loadScopedGraph(ctx context.Context, db *store.DB, o options) (*graph.Graph, []string, error) {
	repos, err := db.Repos(ctx)
	if err != nil {
		return nil, nil, err
	}
	known := make(map[string]bool, len(repos))
	for _, r := range repos {
		known[r.ID] = true // model.Repo.ID 就是 string（model/repo.go:6），没有 RepoID 类型
	}
	wanted := map[string]bool{}
	for _, id := range o.repos {
		if !known[id] {
			// 打错一个 id 也要报错，但不能用「unknown repo」这种不含
			// "no snapshot" 的措辞：既有测试 TestRunGraphNoSnapshot
			// （cmd/oac/impact_test.go:102-119，repo id 是根本没进过 repos 表的
			// repo:/never/scanned）钉的是「错误里点名 repo id + 说 no snapshot +
			// 不带内层 oac: 前缀」三条，本切片只加词、不改这三条。
			return nil, nil, fmt.Errorf("unknown repo %q: no snapshot; run `oac scan <path>` first", id)
		}
		wanted[id] = true
	}
	if o.all {
		wanted = nil
	}
	// 这里**不读 o.args**：args 的位置含义按子命令不同（graph 是 repo id，
	// impact/rank/history 是 element id）。把「位置参数当 repo id」这件事放在
	// runGraph 里做一次（Step 4），作用域解析器就只有一种输入。
	// wanted 为空 = 没有指名 = 全部有快照的仓都并进来，与 GUI 的
	// `app.go:57`「GraphRepos 为空时选中全部」是同一条默认。

	var (
		graphs   []*graph.Graph
		warnings []string
	)
	for _, r := range repos { // db.Repos 按 name 排序，作用域装载顺序因此确定
		if len(wanted) > 0 && !wanted[r.ID] {
			continue
		}
		snapshotID, err := db.LatestSnapshotID(ctx, r.ID)
		if err != nil {
			return nil, nil, err
		}
		if snapshotID == "" {
			if len(wanted) > 0 {
				return nil, nil, fmt.Errorf("repo %q has no snapshot; run `oac scan` first", r.ID)
			}
			continue // --all 里没有快照的仓直接跳过：它没有可并的东西
		}
		els, err := db.Elements(ctx, snapshotID)
		if err != nil {
			return nil, nil, err
		}
		rels, err := db.Relationships(ctx, snapshotID)
		if err != nil {
			return nil, nil, err
		}
		graphs = append(graphs, graph.Build(els, rels))
	}
	if len(graphs) == 0 {
		return nil, nil, fmt.Errorf("no repository in scope has a snapshot")
	}
	merged, warnings := graph.Merge(graphs)
	return merged, warnings, nil
}
```

上面的 import 块只有 `graph` 与 `store` 两个内部包——`loadScopedGraph` 不直接写 `model.X`（`db.Repos` 的返回类型由推断带入 range），所以也不需要 `model`；多加一个用不上的 import 会撞 `imported and not used`。**别把路径写成 `github.com/futuremeng/github.com/...`**：那是本文档早期版本的一个真实笔误，已改正。留这一句是因为这条模块路径很长，抄错时编译器报的 `package ... is not in std` 指向 Go，不指向这一行。

- [ ] **Step 4: 改 `runGraph` / `runImpact` 走作用域**

`graph.go`：

```go
func runGraph(ctx context.Context, db *store.DB, o options, out io.Writer) error {
	scope := o
	// 切片 1 的 `oac graph <repo-id>` 形态继续有效：等价于 --repo <repo-id>。
	// README 与用户的肌肉记忆都压在它身上，所以在这一个入口把它翻译掉。
	if len(o.args) > 1 {
		// 多给的 positional 不吞：F4 的教训是「位置参数只能正好一个」，
		// 少一个（0 个）现在有了新含义=全库，多一个一定是用户打错了。
		return fmt.Errorf("want exactly one repo id, got %v", o.args)
	}
	if !o.all && len(o.repos) == 0 && len(o.args) == 1 {
		scope.repos = []string{o.args[0]}
		scope.args = nil
	}
	g, warnings, err := loadScopedGraph(ctx, db, scope)
	if err != nil {
		return err
	}
	for _, w := range warnings {
		fmt.Fprintln(os.Stderr, "oac:", w)
	}
	if o.json {
		return writeJSON(out, graphPayload{
			Elements:      g.Elements(),
			Relationships: g.Relationships(),
		})
	}
	fmt.Fprintf(out, "merged %d elements, %d relationships\n",
		len(g.Elements()), len(g.Relationships()))
	for _, e := range g.Elements() {
		fmt.Fprintf(out, "%s\t%s\t%s\n", e.ID, e.Type, e.Name)
	}
	for _, rel := range g.Relationships() {
		fmt.Fprintf(out, "%s -> %s [%s] x%d\n", rel.FromID, rel.ToID, rel.Type, rel.Weight)
	}
	return nil
}

type graphPayload struct {
	Elements      []model.Element      `json:"elements"`
	Relationships []model.Relationship `json:"relationships"`
}
```

`cmd/oac/main.go` **本任务不改**（定稿）：`subcommands` 表里 graph 那条的 `arg: "repo id"` 继续成立——位置参数仍然是 repo id，只是从「必须一个」变成「最多一个」。T14 的 `printUsage()` 那行措辞是 `positional: %s`，不声明 arity（arity 由 verb 自己报），所以 main.go 这里没有必须跟着改的文案；`runGraph` 自己在 `len(o.args) > 1` 时返回更准确的 `want exactly one repo id, got …`。旗标注册也在 T14 就落了地，这里没有 main.go 的改动可做。原计划在 Files 里列了 main.go，是把它与 T14/T17 的表改动记混了，删。

`loadGraph(ctx, db, snapshotID)` 保留（`oac diff` 与平价测试按快照 id 读，那是「指定快照」语义，与作用域无关）。

import 变更：`cmd/oac/graph.go` 现在只 import `context/fmt/io/graph/store`（实测文件头），本任务要加 `"os"`（warnings 打 stderr）和 `"github.com/futuremeng/OpenArchitect/internal/core/model"`（`graphPayload` 的两个切片字段）；`cmd/oac/impact.go` 的 import 是 `context/fmt/io/graph/store`，只加 `"os"`。两处 `fmt.Fprintln(os.Stderr, "oac:", w)` 的内层 `oac:` 前缀是**故意的**——它不是错误字符串（错误路径才由 main 统一加前缀），是普通输出，所以 `TestRunGraphNoSnapshot` 那条「错误不得带内层 oac:」的断言不受影响。

`impact.go` 的改动：`runImpact` 现在从 `db.SnapshotForElement(center)` 推快照，再 `loadGraph`。有了作用域之后：

```go
func runImpact(ctx context.Context, db *store.DB, o options, out io.Writer) error {
	center, err := wantOne(o, "element id")
	if err != nil {
		return err
	}
	g, warnings, err := loadScopedGraph(ctx, db, o)
	if err != nil {
		return err
	}
	for _, w := range warnings {
		fmt.Fprintln(os.Stderr, "oac:", w)
	}
	report, err := graph.Analyze(g, center, o.depth)
	if err != nil {
		return err
	}
	if o.json {
		return writeJSON(out, report)
	}
	// 下面三行与 writeHits 整个函数逐字保留 cmd/oac/impact.go:25-44 的现状
	// （含那段「只打印两张表」的注释）——本任务只换取参与取图那两段。
	fmt.Fprintf(out, "Center %s %s (%s)\n", report.Center.ID, report.Center.Name, report.Center.Type)
	writeHits(out, "Upstream", report.Upstream)
	writeHits(out, "Downstream", report.Downstream)
	return nil
}
```

**行为变更（必须写进 findings 与 README）**：

1. `impact` 不再先查 `SnapshotForElement`。中心元素不在作用域图里时由 `graph.Analyze` 返回 `model.ErrUnknownElement`（`graph/impact.go` 的既有语义），退出码 1 不变。收益是跨仓影响分析终于可算（切片 2 的存在理由之一）。
2. 不给 `--all`/`--repo`/位置参数时，`graph` 与 `impact` 的作用域都是**全部有快照的仓**，与 GUI 的默认（`internal/ui/app.go:57`：`GraphRepos` 为空时选中全部）对齐。切片 1 的 `graph` 缺参数会报「want exactly one repo id」，现在它给的是合并视图——这不是放宽校验，是把「没指名 = 看全部」这条 GUI 已有的默认搬到 CLI。
3. `oac graph` 的首行文案从 `snapshot <id>: N elements, M relationships` 改成 `merged N elements, M relationships`（单仓作用域也说 merged，因为走的确实是同一条并集路径）。实测**没有任何既有测试断言这一行**（`cmd/oac/impact_test.go:102` 只断言错误路径，`graph.go:41` 那行没有匹配者），所以改文案安全；README 的示例输出与 `docs/superpowers/poc/2026-10-26-observation-corpus.md:64` 那句要跟着改。

- [ ] **Step 5: 加一条跨仓影响的验收测试**

```go
// DoD-1 的 CLI 半边：crossmod-app 的 Gateway 跨模块调用 crossmod-dep 的 DB.Save，
// 在 --all 作用域下必须让 dep 仓的元素出现在 app 仓元素的下游里。
// 两个 id 逐字取自既有断言：internal/scanner/language/go_test.go:282 的
// foreign = "cmp:crossmod.example/dep/store.DB"，以及该 fixture 的
// type Gateway（testdata/fixtures/crossmod-app/store/store.go:11）。
func TestImpactCrossesReposUnderAll(t *testing.T) {
	db, _, _ := scopedDB(t)
	defer db.Close()
	const center = "cmp:crossmod.example/app/store.Gateway"
	var buf bytes.Buffer
	err := runImpact(context.Background(), db, options{
		args: []string{center}, all: true, depth: 1}, &buf)
	if err != nil {
		t.Fatalf("--all impact %s: %v", center, err)
	}
	if !strings.Contains(buf.String(), "cmp:crossmod.example/dep/store.DB") {
		t.Errorf("--all 作用域下没有跨仓下游，输出：\n%s", buf.String())
	}
}

// 反面一条：同一中心在单仓作用域下必须看不到跨仓下游（悬空端点在单仓视图里
// 没有归属图）。这条与上一条合起来才是 spec §7「单仓视图看不到、合并视图复活」
// 的完整形状。
func TestImpactHidesCrossRepoUnderSingleRepoScope(t *testing.T) {
	db, ids, _ := scopedDB(t)
	defer db.Close()
	// scopedDB 先扫 dep 再扫 app，ids[1] 是 app。
	var buf bytes.Buffer
	err := runImpact(context.Background(), db, options{
		args: []string{"cmp:crossmod.example/app/store.Gateway"}, repos: []string{ids[1]},
		depth: 1}, &buf)
	if err != nil {
		t.Fatalf("单仓 impact: %v", err)
	}
	if strings.Contains(buf.String(), "crossmod.example/dep") {
		t.Errorf("单仓作用域里出现了跨仓下游（作用域过滤漏了）：\n%s", buf.String())
	}
}
```

> 第二条测试依赖一个事实：单仓图里 `cmp:crossmod.example/dep/store.DB` **不是节点**（它是悬空端点，只存在于边的 `to_id` 上），`graph.Build` 会丢掉端点不全的边。写代码前先用 `oac graph <app-repo-id>` 复核这一点；若切片 1 的 `go.go` 已经把外部类型也存成了元素，这条断言的期望就要反过来（并作为一条 spec 偏差记进 findings）。


- [ ] **Step 6: 全绿 + 提交**

```sh
go test ./cmd/oac/ -run 'TestScope|TestImpact|TestGraph' -v
go vet ./... && go test ./...
git add cmd/oac/scope.go cmd/oac/scope_test.go cmd/oac/graph.go cmd/oac/impact.go
git commit -m "feat: oac graph/impact/rank 的 --all 与可重复 --repo 作用域"
```

---

## Phase E — GUI

对比着色、幽灵节点、抽屉历史、仓库页转正。四件事共用一条前提：**GUI 只读库，颜色与历史的数据一律来自 T5/T6/T3 的图与存储 API**，绝不在绘制层重新实现判定逻辑（DoD-4：同一份数据，GUI 与 CLI 必须一致）。

### Task 19: `nodeCard` 的对比标记通道

**Files:**
- Modify: `internal/ui/node_card.go`（`nodeCard` 加第四个参数，`internal/ui/node_card.go:55`）
- Modify: `internal/ui/flow_data.go:59`（唯一的生产调用点）
- Modify: `internal/ui/node_card_test.go:69,98,103`（三处测试调用点补参数）
- Test: `internal/ui/node_card_test.go`

**Interfaces:**
- Consumes: 无（本任务不依赖前序任务的新 API）。
- Produces:

```go
type cardMark uint8
const (
	markNone cardMark = iota
	markAdded
	markChanged
	markRemoved
)
// markColor 是纯映射，返回（边框色, 是否虚线）。
func markColor(t *ui.Theme, m cardMark) (ui.Color, bool)
func nodeCard(c *ui.Context, el model.Element, zoom float32, mark cardMark)
```

`flow.Node` 只有 `{ID, Label, Width, Height, Muted, Content}`（fork `components/flow/flow.go` 的 `Node` 定义），没有颜色通道，所以描边只能画在 `nodeCard` 内部。卡片尺寸仍是 168×46：标记是**颜色与线型**的差异，不是几何差异，PoC-4 的帧时间数字继续有效。

**定稿（着色方案）**：`added → t.Success`、`changed → t.Warning`、`removed → t.TextMuted + BorderDashed`。spec §10 原文是「added 绿描边 / changed 琥珀描边 / removed 虚线幽灵节点」，没给 removed 上色；用 `TextMuted` 而不是 `Danger`，是因为「消失」不是错误，而 Danger 在切片 1 的语义是错误文本（`page_settings.go:108`）。代价：removed 节点在深色底上偏弱，靠虚线区分。验收时人眼确认（Task 26 的 DoD runbook 第 8 条）。

- [ ] **Step 1: 写失败测试**

追加到 `internal/ui/node_card_test.go`：

```go
// TestMarkColorMapping 钉的是映射本身。断言用真主题对象（ui.LightTheme()）而不是
// 十六进制字面量：换主题色时这条测试必须继续绿，只有「标记 ↔ 语义」的对应关系
// 才是它守的东西。
func TestMarkColorMapping(t *testing.T) {
	theme := ui.LightTheme()
	for _, tc := range []struct {
		mark   cardMark
		want   ui.Color
		dashed bool
	}{
		{markNone, theme.Border, false},
		{markAdded, theme.Success, false},
		{markChanged, theme.Warning, false},
		{markRemoved, theme.TextMuted, true},
	} {
		got, dashed := markColor(theme, tc.mark)
		if got != tc.want || dashed != tc.dashed {
			t.Errorf("markColor(%v) = (%v, %v), want (%v, %v)", tc.mark, got, dashed, tc.want, tc.dashed)
		}
	}
}

// TestMarksAddNoWidgets 是 spec §9.3 的性能守门：卡片每帧重建，标记不许给它添
// 第三个文本行或任何控件——那才是要付帧时间的东西。三个 LOD 档 × 四种标记都
// 必须维持既有的文本行数。
func TestMarksAddNoWidgets(t *testing.T) {
	el := model.Element{
		ID:   "cmp:example.com/cart/internal/basket.Basket",
		Type: model.TypeComponent,
		Name: "Basket",
	}
	for _, zoom := range []float32{0.13, lodDot, lodFull} {
		for _, mark := range []cardMark{markNone, markAdded, markChanged, markRemoved} {
			tt := ui.NewTester(func(c *ui.Context) { nodeCard(c, el, zoom, mark) }, 220, 60)
			if got, want := len(tt.Texts()), textLinesAt(zoom); got != want {
				t.Errorf("zoom %g mark %v: text lines = %d, want %d", zoom, mark, got, want)
			}
		}
	}
}

func textLinesAt(zoom float32) int {
	switch {
	case zoom < lodDot:
		return 0
	case zoom < lodFull:
		return 1
	default:
		return 2
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

```sh
go test ./internal/ui/ -run 'TestMark' -v
```

期望：编译失败 `undefined: cardMark`（改签名之前的旧调用点也一起红，这是预期的）。

- [ ] **Step 3: 实现映射与参数**

`internal/ui/node_card.go` 里 `nodeCard` 换成：

```go
// cardMark 是快照对比给一张卡片打的标记。它不进 flow.Node（那个结构没有样式
// 通道），只作为 nodeCard 的参数存在，所以绘制层的颜色判定集中在 markColor
// 这一处。
type cardMark uint8

const (
	markNone cardMark = iota
	markAdded
	markChanged
	markRemoved
)

// markColor 单独成纯函数：headless Tester 只暴露文本与像素，颜色断言走像素要
// 撞抗锯齿与 DPR，钉不住。所以这里钉映射，「卡片上确实画了这一色」由 DoD
// runbook（Task 26 第 8 条）的人眼项保证。
func markColor(t *ui.Theme, m cardMark) (ui.Color, bool) {
	switch m {
	case markAdded:
		return t.Success, false
	case markChanged:
		return t.Warning, false
	case markRemoved:
		return t.TextMuted, true
	default:
		return t.Border, false
	}
}

func nodeCard(c *ui.Context, el model.Element, zoom float32, mark cardMark) {
	t := c.Theme()
	color, dashed := markColor(t, mark)
	box := ui.Row(c).Size(cardWidth, cardHeight).Radius(flow.CardRadius).
		Background(t.Surface).Border(1, color)
	if dashed {
		box.BorderStyle(ui.BorderDashed)
	}
	dot := t.TextMuted
	if mark != markNone {
		// 点阵档只剩一颗圆点，颜色是它唯一的标记通道——低倍全景下的
		// "这一片都变了"必须还看得见。
		dot = color
	}
	if zoom < lodDot {
		box.AlignItems(ui.Center).Justify(ui.Center).Children(func() {
			ui.Box(c).Size(10, 10).Radius(5).Background(dot)
		})
		return
	}
	// 以下两个分支与切片 1 逐字相同，只是 dot 变量不进文本区。
	box.Gap(t.Space(1.5)).AlignItems(ui.Center).
		Padding(t.Space(1.5), t.Space(2)).Children(func() {
		ui.Icon(c, iconFor(el.Type)).Size(14, 14)
		text := ui.Column(c).Grow(1).MinWidth(0).Gap(1)
		if zoom < lodFull {
			text.Children(func() {
				components.Text(c, el.Name).SingleLine().FontWeight(600).TextColor(t.Text)
			})
			return
		}
		text.Children(func() {
			components.Text(c, el.Name).SingleLine().FontWeight(600).TextColor(t.Text)
			components.Text(c, truncateMiddle(model.PkgPathOf(el.ID), pkgBudget)).
				SingleLine().FontSize(t.Rem(0.8)).TextColor(t.TextMuted)
		})
	})
}
```

- [ ] **Step 4: 改四个调用点**

`internal/ui/flow_data.go:59`（本任务先传 `markNone`，T20 换成 `a.markFor(el.ID)`——分两步是为了每步都独立可绿）：

```go
Content: func(c *ui.Context) { nodeCard(c, el, a.flowState.Zoom(), markNone) },
```

`internal/ui/node_card_test.go` 的三处：`:69` 与 `:98` 补 `, markNone`，`:103` 同理。

- [ ] **Step 5: 全绿 + 提交**

```sh
go test ./internal/ui/ -run 'TestMark|TestNodeCard|TestCardSize|TestTruncate|TestIconFor' -v
go vet ./... && go test ./...
git add internal/ui/node_card.go internal/ui/node_card_test.go internal/ui/flow_data.go
git commit -m "feat: nodeCard 的快照对比标记通道（描边色 + 虚线）"
```

---

### Task 20: 对比状态机、幽灵节点与边着色

**Files:**
- Create: `internal/ui/compare.go`
- Modify: `internal/ui/state.go`（`App` 加 `compare` 字段；`refreshView` 改数据源）
- Modify: `internal/ui/app.go`（`Reload` 清空 `compare` 与 `mergeWarnings`）
- Modify: `internal/ui/flow_data.go`（`markNone` → `a.markFor(el.ID)`；`flowEdges` 读 `a.compare.edgeMark`）
- Test: `internal/ui/compare_test.go`

**Interfaces:**
- Consumes: T5 `graph.Merge`、T6 `graph.Diff`/`DiffSnapshots`/`ChangedElement`、T2 `DB.Elements`/`DB.Relationships`/`DB.LatestSnapshotID`/`DB.Snapshot`、T19 `cardMark`。
- Produces:

```go
type compareState struct {
	repoID    string
	fromID    string            // 较旧快照
	toID      string            // 永远是该仓最新快照（定稿）
	diff      graph.Diff
	merged    *graph.Graph      // 最新 + 幽灵节点/边
	marks     map[string]cardMark   // 节点 id → 标记（含组节点的汇总标记）
	edgeMarks map[string]cardMark   // relKey(from,to,type) → 标记
}
func (a *App) SetCompare(ctx context.Context, repoID, fromID string) error
func (a *App) ClearCompare()
func (a *App) Comparing() bool
func (a *App) CompareAllowed() bool
func (a *App) CompareSummary() string
func (a *App) markFor(id string) cardMark
func ghostGraph(cur *graph.Graph, d graph.Diff) *graph.Graph
func relKey(from, to string, typ model.RelationshipType) string
```

**三条定稿**（都写进 findings）：

1. **对比是单仓两快照**，`to` 恒为该仓最新快照。spec §10 说「选两个快照」，但 `DiffSnapshots` 的两个输入必须同仓才有意义（跨仓 diff 比的是两个不相干的模块树，`ChangedFields` 会给出满屏假变更）。多仓选中时 `CompareAllowed()` 为 false，开关画成禁用态并给出原因文案。代价：用户想看「A 仓上一版 vs B 仓当前版」没有入口——那不是对比，那是两张图。
2. **幽灵节点参与布局**。`merged` 把 `diff.Removed` 并进最新快照的图再一起 `graph.Build`，所以被删元素照旧占位、照旧被 `Aggregate` 归组，用户看得见「这里少了个东西」。若把幽灵做成浮在布局上的覆盖层，`pkg:`/`sys:` 组节点就无法汇总它们的标记。代价：对比时节点数比真实快照多，`flow` 的布局会挪——这是 PoC-11（T25）用带幽灵的合并图量首帧的原因。
3. **组节点的标记：成员一致取该标记，不一致取 `markChanged`。**「包层里一颗 added 一颗 removed」画成虚线会读成「整包消失了」，琥珀（"这组里变了"）才是诚实的答案。汇总在 `SetCompare` 里一次算完（两个深度、全部组 id 都预先算进 `marks`），因为 `flowData` 每帧跑，逐帧算成员等于给每颗卡片加一次遍历。

- [ ] **Step 1: 写失败测试**

`internal/ui/compare_test.go`：

```go
package ui

import (
	"context"
	"strings"
	"testing"

	"github.com/futuremeng/OpenArchitect/internal/core/graph"
	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

// commitCompare 往同一个仓写两个快照：from 有 Gone + Keep(v1)，to 有 Keep(v2) +
// Fresh。元素 ID 用真实的 cmp: 形态，因为 model.PkgPathOf 要能解析出包路径
// （组节点汇总那条测试依赖它）。
func commitCompare(t *testing.T, a *App) (repoID, fromID, toID string) {
	t.Helper()
	ctx := context.Background()
	repoID = "repo:/compare"
	if err := a.db.UpsertRepo(ctx, model.Repo{ID: repoID, Name: "compare",
		LocalPath: "/compare", Lang: "go"}); err != nil {
		t.Fatal(err)
	}
	keep := func(desc string) model.Element {
		return model.Element{ID: "cmp:example.com/c/internal/keep.Keep",
			Type: model.TypeComponent, Name: "Keep", Description: desc, RepoID: repoID}
	}
	from := model.Element{ID: "cmp:example.com/c/internal/gone.Gone",
		Type: model.TypeComponent, Name: "Gone", RepoID: repoID}
	fresh := model.Element{ID: "cmp:example.com/c/internal/fresh.Fresh",
		Type: model.TypeComponent, Name: "Fresh", RepoID: repoID}
	for _, step := range []struct {
		id   string
		elts []model.Element
		rels []model.Relationship
	} {
		{id: "s0000000000000000001", elts: []model.Element{from, keep("v1")},
			rels: []model.Relationship{{FromID: from.ID, ToID: keep("v1").ID,
				Type: model.RelCalls, Weight: 1}}},
		{id: "s0000000000000000002", elts: []model.Element{keep("v2"), fresh}},
	} {
		err := a.db.CommitScan(ctx, storeScanCommit(step.id, repoID, step.elts, step.rels))
		if err != nil {
			t.Fatalf("commit %s: %v", step.id, err)
		}
	}
	return repoID, "s0000000000000000001", "s0000000000000000002"
}

func TestSetCompareMapsNodeMarks(t *testing.T) {
	a := newTestApp(t)
	repoID, fromID, _ := commitCompare(t, a)
	if err := a.SetCompare(context.Background(), repoID, fromID); err != nil {
		t.Fatalf("SetCompare: %v", err)
	}
	if !a.Comparing() {
		t.Fatal("Comparing() must be true after SetCompare")
	}
	for _, tc := range []struct {
		id   string
		want cardMark
	}{
		{"cmp:example.com/c/internal/fresh.Fresh", markAdded},
		{"cmp:example.com/c/internal/gone.Gone", markRemoved},
		{"cmp:example.com/c/internal/keep.Keep", markChanged},
	} {
		if got := a.markFor(tc.id); got != tc.want {
			t.Errorf("markFor(%s) = %v, want %v", tc.id, got, tc.want)
		}
	}
	if got := a.markFor("cmp:example.com/c/internal/keep.Keep"); got == markNone {
		t.Fatal("changed mapping collapsed to none")
	}
}

// TestGhostNodeIsInDiagram: 幽灵必须真的进了视图——「在 marks 里」不算，那只是
// 一张表；它必须出现在 a.view 里，否则 flowData 根本不会为它画卡片。
func TestGhostNodeIsInDiagram(t *testing.T) {
	a := newTestApp(t)
	repoID, fromID, toID := commitCompare(t, a)
	if err := a.SetCompare(context.Background(), repoID, fromID); err != nil {
		t.Fatal(err)
	}
	if _, ok := a.view.Element("cmp:example.com/c/internal/gone.Gone"); !ok {
		t.Fatalf("ghost node missing from view: %s", a.CompareSummary())
	}
	// 最新快照自己的元素一个都不能少。
	latest, err := a.db.Elements(context.Background(), toID)
	if err != nil || len(latest) != 2 {
		t.Fatalf("latest snapshot elements = %d, %v", len(latest), err)
	}
}

// TestGhostEdgeKeepsSurvivingEndpoint: graph.Build 会丢掉端点不全的边。被删的边
// 一端连着幽灵、一端连着存活元素，所以 Build 的输入必须同时含两者——这条测试
// 钉的就是 ghostGraph 的拼装顺序，反过来写（先 Build 幽灵再拼存活）会静默丢边。
func TestGhostEdgeKeepsSurvivingEndpoint(t *testing.T) {
	a := newTestApp(t)
	repoID, fromID, _ := commitCompare(t, a)
	if err := a.SetCompare(context.Background(), repoID, fromID); err != nil {
		t.Fatal(err)
	}
	key := relKey("cmp:example.com/c/internal/gone.Gone",
		"cmp:example.com/c/internal/keep.Keep", model.RelCalls)
	if got := a.compare.edgeMarks[key]; got != markRemoved {
		t.Errorf("edgeMark(%s) = %v, want markRemoved", key, got)
	}
	var found bool
	for _, rel := range a.view.Relationships() {
		if relKey(rel.FromID, rel.ToID, rel.Type) == key {
			found = true
		}
	}
	if !found {
		t.Error("removed edge dropped from the view graph")
	}
}

func TestGroupMarkRollup(t *testing.T) {
	a := newTestApp(t)
	repoID, fromID, _ := commitCompare(t, a)
	if err := a.SetCompare(context.Background(), repoID, fromID); err != nil {
		t.Fatal(err)
	}
	// 同一包里有 added + removed + changed 三种成员 → 汇总必须是 changed。
	pkg := graph.PackageGroupID("example.com/c/internal/fresh")
	if got := a.markFor(pkg); got != markNone {
		t.Errorf("single-member package %s = %v, want markNone", pkg, got)
	}
	mixed := graph.PackageGroupID("example.com/c/internal")
	if got := a.markFor(mixed); got != markNone {
		t.Logf("注：internal/ 不是任何元素的包路径（cmp: 的包段是 …/internal/keep 等），%v", got)
	}
	// 真正的汇总靶子：keep 包只有 Keep 一个成员，它是 changed。
	keepPkg := graph.PackageGroupID("example.com/c/internal/keep")
	if got := a.markFor(keepPkg); got != markChanged {
		t.Errorf("rollup(%s) = %v, want markChanged", keepPkg, got)
	}
	repoGroup := graph.RepoGroupID(repoID)
	if got := a.markFor(repoGroup); got != markChanged {
		t.Errorf("repo group rollup = %v, want markChanged (mixed members)", got)
	}
}

func TestCompareDisabledForMultiRepo(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	first := seedScan(t, a, fixtureDir("shared-lib"))
	second := seedScan(t, a, fixtureDir("simple-multi-main"))
	if err := a.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	if a.CompareAllowed() {
		t.Fatal("两个仓同时选中时不允许开对比")
	}
	if err := a.SetCompare(ctx, string(first.RepoID), "s1"); err == nil {
		t.Error("SetCompare must refuse when CompareAllowed is false")
	}
	if err := a.SetGraphRepos(ctx, []string{string(second.RepoID)}); err != nil {
		t.Fatal(err)
	}
	if !a.CompareAllowed() {
		t.Fatal("单仓选中后必须允许对比")
	}
}

// TestReloadClearsCompare: Reload 会重选快照，留着旧 compare 等于让屏幕上的
// 着色指着已经不存在的快照对。
func TestReloadClearsCompare(t *testing.T) {
	a := newTestApp(t)
	repoID, fromID, _ := commitCompare(t, a)
	if err := a.SetCompare(context.Background(), repoID, fromID); err != nil {
		t.Fatal(err)
	}
	if err := a.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	if a.Comparing() {
		t.Error("Reload must clear the compare state")
	}
	if a.markFor("cmp:example.com/c/internal/gone.Gone") != markNone {
		t.Error("marks survive a cleared compare")
	}
}

func TestCompareSummaryCounts(t *testing.T) {
	a := newTestApp(t)
	repoID, fromID, _ := commitCompare(t, a)
	if err := a.SetCompare(context.Background(), repoID, fromID); err != nil {
		t.Fatal(err)
	}
	got := a.CompareSummary()
	// 新增 1（Fresh）、移除 1（Gone）、变更 1（Keep 的 description）。
	for _, want := range []string{"新增 1", "移除 1", "变更 1", "边 +0/−1"} {
		if !strings.Contains(got, want) {
			t.Errorf("summary %q missing %q", got, want)
		}
	}
}
```

`storeScanCommit` 是本测试文件的胶水（T3 的 `ScanCommit` 字段多，测试里不需要 Files/Fingerprints/History）：

```go
func storeScanCommit(id, repoID string, elts []model.Element,
	rels []model.Relationship) store.ScanCommit {
	return store.ScanCommit{
		Snapshot:      model.ArchitectureSnapshot{ID: id, RepoID: repoID},
		Elements:      elts,
		Relationships: rels,
		KeepSnapshots: 50,
	}
}
```

（`store` 已在 import 块里；`newTestApp`/`seedScan`/`fixtureDir` 来自 `app_test.go:19-43`。）

- [ ] **Step 2: 跑测试确认失败**

```sh
go test ./internal/ui/ -run 'TestSetCompare|TestGhost|TestGroupMark|TestCompare|TestReloadClears|TestCompareSummary' -v
```

期望：编译失败 `undefined: compareState` / `a.SetCompare`。

- [ ] **Step 3: App 字段与 refreshView**

`internal/ui/state.go` 的 `App` 结构里，紧跟 `view *graph.Graph` 之后加：

```go
	// compare 非 nil 时，view 的数据源从 a.graph 换成 compare.merged，卡片与边
	// 带上标记色。Reload 一律清空它（T20 定稿 1：对比绑定的是「一个仓的两个快照」，
	// Reload 之后选中的快照集合可能已经变了）。
	compare *compareState
	// mergeWarnings 是 graph.Merge 报的归属冲突/重复仓提示，条数进状态行，
	// 全文只在 CLI 的 oac graph --all 里读（GUI 不复制一个日志面板）。
	mergeWarnings []string
```

`refreshView` 换成（`state.go:406-417`）：

```go
func (a *App) refreshView() {
	src := a.graph
	if a.compare != nil {
		src = a.compare.merged
	}
	var level graph.Level
	switch a.Depth() {
	case 0:
		level = graph.LevelRepo
	case 1:
		level = graph.LevelPackage
	default:
		level = graph.LevelElement
	}
	a.view = graph.Aggregate(src, level)
}
```

`internal/ui/app.go` 的 `Reload`，在 `a.refreshView()` 之前加一行（与既有的「新图配旧锚点没有意义」同一处纪律）：

```go
	a.compare, a.mergeWarnings = nil, nil
```

T5 已经在 `Reload` 里写好了 `merged, warnings := graph.Merge(graphs)` → `a.mergeWarnings = warnings` / `a.graph = merged`（本任务不改它，`Merge` 的入参是 `[]*graph.Graph` 切片，不是可变参数——别写成 `graph.Merge(perRepo...)`）。本任务只加 `compare` 字段与 `refreshView` 的数据源切换。

- [ ] **Step 4: compare.go**

```go
package ui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/futuremeng/OpenArchitect/internal/core/graph"
	"github.com/futuremeng/OpenArchitect/internal/core/model"
	"github.com/futuremeng/OpenArchitect/internal/core/store"
)

// compareState 是一次「同仓两快照」对比的全部状态。它持有两张图的并集
// （merged，含幽灵）而不是只持有 diff：视图、抽屉、影响分析全都读 a.view，
// 让幽灵节点进 view 是唯一能让它们在同一张图上可点可钻的做法。
type compareState struct {
	repoID    string
	fromID    string
	toID      string
	diff      graph.Diff
	merged    *graph.Graph
	marks     map[string]cardMark
	edgeMarks map[string]cardMark
}

func relKey(from, to string, typ model.RelationshipType) string {
	return from + "\x00" + to + "\x00" + string(typ)
}

// CompareAllowed 钉的是定稿 1：对比只在恰好选中一个仓时可开。
func (a *App) CompareAllowed() bool {
	return a.db != nil && len(a.selectedRepos) == 1
}

func (a *App) Comparing() bool { return a.compare != nil }

// SetCompare 装载 repoID 的 fromID → 最新快照 的差异，重建视图。fromID 必须属于
// repoID（否则是把两个仓的东西混在一张对比图上），repoID 必须是当前唯一选中的仓。
func (a *App) SetCompare(ctx context.Context, repoID, fromID string) error {
	if !a.CompareAllowed() {
		return fmt.Errorf("ui: 对比需要对单个仓库开启")
	}
	snap, err := a.db.Snapshot(ctx, fromID)
	if err != nil {
		return err
	}
	if snap.RepoID != repoID {
		return fmt.Errorf("ui: 快照 %s 不属于仓库 %s", fromID, repoID)
	}
	toID := a.repoLatest[repoID]
	if toID == "" {
		return fmt.Errorf("ui: 仓库 %s 没有快照", repoID)
	}
	oldGraph, err := a.snapshotGraph(ctx, fromID)
	if err != nil {
		return err
	}
	newGraph, err := a.snapshotGraph(ctx, toID)
	if err != nil {
		return err
	}
	d := graph.DiffSnapshots(oldGraph, newGraph)
	c := &compareState{repoID: repoID, fromID: fromID, toID: toID, diff: d,
		merged: ghostGraph(newGraph, d)}
	c.marks = marksFor(d)
	c.edgeMarks = edgeMarksFor(d)
	rollUpGroupMarks(c)
	a.compare = c
	a.refreshView()
	// 视图换了一张，selected 指向的可能是幽灵也可能已经不存在的节点；影响报告
	// 同理。与 Reload 同一处置：清。
	a.selected, a.drillInto, a.impact = "", "", nil
	return nil
}

func (a *App) ClearCompare() {
	if a.compare == nil {
		return
	}
	a.compare = nil
	a.refreshView()
	a.selected, a.drillInto, a.impact = "", "", nil
}

// snapshotGraph 是「一个快照 → 一张图」的唯一写法，CLI 的 loadScopedGraph
// （cmd/oac/scope.go:1816 那一族）与这里逐字同序：读行、Build。形状一致是
// DoD-4 的内容，不是巧合。
func (a *App) snapshotGraph(ctx context.Context, snapshotID string) (*graph.Graph, error) {
	elements, err := a.db.Elements(ctx, snapshotID)
	if err != nil {
		return nil, err
	}
	rels, err := a.db.Relationships(ctx, snapshotID)
	if err != nil {
		return nil, err
	}
	return graph.Build(elements, rels), nil
}

// ghostGraph: 定稿 2。最新快照的全部 + 消失的元素 + 被删的边，一次 Build。
// 两边的 id 天然不相交（Removed 按定义不在最新快照里，EdgesRemoved 不在最新
// 的边集里），所以不需要去重；Build 会丢掉端点不全的边，因此节点必须先于边
// 进同一个调用——分开 Build 再 Merge 会把幽灵边全丢。
func ghostGraph(cur *graph.Graph, d graph.Diff) *graph.Graph {
	elements := append([]model.Element{}, cur.Elements()...)
	elements = append(elements, d.Removed...)
	rels := append([]model.Relationship{}, cur.Relationships()...)
	rels = append(rels, d.EdgesRemoved...)
	return graph.Build(elements, rels)
}

func marksFor(d graph.Diff) map[string]cardMark {
	marks := make(map[string]cardMark, len(d.Added)+len(d.Removed)+len(d.Changed))
	for _, el := range d.Added {
		marks[el.ID] = markAdded
	}
	for _, el := range d.Removed {
		marks[el.ID] = markRemoved
	}
	for _, ch := range d.Changed {
		marks[ch.To.ID] = markChanged
	}
	return marks
}

func edgeMarksFor(d graph.Diff) map[string]cardMark {
	edgeMarks := make(map[string]cardMark, len(d.EdgesAdded)+len(d.EdgesRemoved))
	for _, rel := range d.EdgesAdded {
		edgeMarks[relKey(rel.FromID, rel.ToID, rel.Type)] = markAdded
	}
	for _, rel := range d.EdgesRemoved {
		edgeMarks[relKey(rel.FromID, rel.ToID, rel.Type)] = markRemoved
	}
	return edgeMarks
}

// rollUpGroupMarks: 定稿 3。成员标记不一致 → markChanged（琥珀 = "这组里变了"），
// 一致 → 那个标记。两个深度的组 id 全都预先算好，因为 flowData 每帧跑一次。
func rollUpGroupMarks(c *compareState) {
	pkg := map[string]cardMark{}
	repo := map[string]cardMark{}
	for _, el := range c.merged.Elements() {
		m := c.marks[el.ID]
		if m == markNone {
			continue
		}
		if p := model.PkgPathOf(el.ID); p != "" {
			pkg[p] = mergeCardMark(pkg[p], m)
		}
		if el.RepoID != "" {
			repo[el.RepoID] = mergeCardMark(repo[el.RepoID], m)
		}
	}
	for p, m := range pkg {
		c.marks[graph.PackageGroupID(p)] = m
	}
	for r, m := range repo {
		c.marks[graph.RepoGroupID(r)] = m
	}
}

func mergeCardMark(cur, add cardMark) cardMark {
	if cur == markNone || cur == add {
		return add
	}
	return markChanged
}

func (a *App) markFor(id string) cardMark {
	if a.compare == nil {
		return markNone
	}
	return a.compare.marks[id]
}

func (a *App) edgeMark(rel model.Relationship) cardMark {
	if a.compare == nil {
		return markNone
	}
	return a.compare.edgeMarks[relKey(rel.FromID, rel.ToID, rel.Type)]
}

// CompareSummary 是 mapHeader 的计数行（spec §10「计数行进 mapHeader」）。id 只留
// 尾部 6 位：整串 s<19 位纳秒> 会把这一行撑成两屏。
func (a *App) CompareSummary() string {
	if a.compare == nil {
		return ""
	}
	d := a.compare.diff
	return fmt.Sprintf("对比 %s → %s：新增 %d，移除 %d，变更 %d，边 +%d/−%d",
		shortSnapshot(a.compare.fromID), shortSnapshot(a.compare.toID),
		len(d.Added), len(d.Removed), len(d.Changed),
		len(d.EdgesAdded), len(d.EdgesRemoved))
}

// id 本身就是 s+19 位数字，没有前缀可剥，所以只留尾部 6 位。
func shortSnapshot(id string) string {
	if len(id) <= 6 {
		return id
	}
	return "…" + id[len(id)-6:]
}
```

> `CompareSummary` 里的 `fromID`/`toID` 从 `a.compare` 取而不是 `d`：`graph.Diff` 结构体里没有快照 id 字段（T6 定稿的字段是 `Added/Removed/Changed/EdgesAdded/EdgesRemoved`），摘要行的两个 id 是 UI 状态，不是差集内容。

- [ ] **Step 5: 接线 flowData / flowEdges**

`internal/ui/flow_data.go`：`:59` 换成

```go
			Content: func(c *ui.Context) { nodeCard(c, el, a.flowState.Zoom(), a.markFor(el.ID)) },
```

`flowEdges` 在 `switch rel.Type` 之后、`edges = append` 之前加：

```go
		// 边上的对比标记：added 加粗到 3（与 calls 的权重通道同一根轴，
		// 封顶也是它的 4 以下），removed 走虚线。changed 的边 spec 没定义，
		// 不发明。
		switch a.edgeMark(rel) {
		case markAdded:
			if edge.Width < 3 {
				edge.Width = 3
			}
		case markRemoved:
			edge.Dashed = true
		}
```

- [ ] **Step 6: 全绿 + 提交**

```sh
go test ./internal/ui/ -run 'TestSetCompare|TestGhost|TestGroupMark|TestCompare|TestReloadClears|TestFlow' -v
go vet ./... && go test ./...
git add internal/ui/compare.go internal/ui/compare_test.go internal/ui/state.go internal/ui/app.go internal/ui/flow_data.go
git commit -m "feat: GUI 对比状态机（单仓两快照 + 幽灵节点 + 节点/边着色）"
```

---

### Task 21: 对比开关、快照 chips 与状态行合并提示

**Files:**
- Create: `internal/ui/page_compare.go`
- Modify: `internal/ui/page_graph.go`（`mapHeader` 调 `a.compareRow(c)`）
- Modify: `internal/ui/state.go`（`StatusLine` 追加合并提示；`App` 加 `compareOptions`）
- Modify: `internal/ui/app.go`（`Reload` 填 `compareOptions`）
- Test: `internal/ui/page_compare_test.go`

**Interfaces:**
- Consumes: T20 的 `SetCompare`/`ClearCompare`/`Comparing`/`CompareAllowed`/`CompareSummary`、T4 的 `DB.Snapshots(ctx, repoID, limit)`（limit=5）、T5 的 merge warnings（经 `App.mergeWarnings`）。
- Produces:

```go
func (a *App) compareRow(c *ui.Context)
func snapshotChipLabel(snap model.ArchitectureSnapshot, rank int) string
func (a *App) CompareOptions() []model.ArchitectureSnapshot
const compareSnapshotChoices = 5
```

chips 的数据在 `Reload` 里读，不在绘制里读：整个 app 的纪律是「渲染函数不做 IO」，`compareOptions` 就是这条纪律的落地形态（`repoLatest`/`selectedRepos` 是同一个先例）。

- [ ] **Step 1: 写失败测试**

`internal/ui/page_compare_test.go`：

```go
package ui

import (
	"context"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

func TestSnapshotChipLabel(t *testing.T) {
	// 序号在前是必须的：created_at 是 SQLite datetime('now')，秒级精度，
	// 同一秒内的两次扫描会撞成同一个标签，用户分不开、测试也没法断言。
	first := snapshotChipLabel(model.ArchitectureSnapshot{ID: "s0000000000000000001",
		CreatedAt: "2026-10-30 09:00:00"}, 1)
	if !strings.HasPrefix(first, "#1 ") {
		t.Errorf("rank 1 label = %q", first)
	}
	if !strings.Contains(first, "当前") {
		t.Errorf("the newest snapshot must be labelled as the base: %q", first)
	}
	if third := snapshotChipLabel(model.ArchitectureSnapshot{ID: "s3", CreatedAt: "x"}, 3); !strings.HasPrefix(third, "#3 ") {
		t.Errorf("rank 3 label = %q", third)
	}
}

func TestCompareRowSwitchRendersGhosts(t *testing.T) {
	a := newTestApp(t)
	repoID, fromID, _ := commitCompare(t, a)
	if err := a.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	if a.repoID != repoID || len(a.compareOptions) != 2 {
		t.Fatalf("repoID/compareOptions = %q/%d, want the compare repo and 2 snapshots",
			a.repoID, len(a.compareOptions))
	}
	tt := ui.NewTester(func(c *ui.Context) { a.graphPage(c) }, 1180, 780)
	tt.Frame()
	if tt.HasText("Gone") {
		t.Fatal("幽灵节点在开关打开之前就画出来了")
	}
	if err := a.SetCompare(context.Background(), repoID, fromID); err != nil {
		t.Fatalf("SetCompare: %v", err)
	}
	tt.Frame()
	if !tt.HasText("Gone") {
		t.Errorf("对比开启后幽灵节点没进图：%s", a.CompareSummary())
	}
	if !tt.HasText("新增 1") {
		t.Error("mapHeader 缺计数行")
	}
}

// TestCompareRowSwitchGoesThroughApp: 开关点击必须走 setter，不能直接写
// a.compare——切回关闭时 view 要重算，selected 要清，这两件事都在 ClearCompare 里。
func TestCompareRowSwitchGoesThroughApp(t *testing.T) {
	a := newTestApp(t)
	repoID, fromID, _ := commitCompare(t, a)
	if err := a.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	a.compareRepoForTest = repoID
	a.compareFromForTest = fromID
	tt := ui.NewTester(func(c *ui.Context) { a.compareRow(c) }, 900, 80)
	tt.Frame()
	if err := tt.Click("对比"); err != nil {
		t.Fatalf("click 对比: %v (texts=%v)", err, tt.Texts())
	}
	if !a.Comparing() {
		t.Error("开关没有走 SetCompare")
	}
	tt.Frame()
	if err := tt.Click("对比"); err != nil {
		t.Fatalf("second click: %v", err)
	}
	if a.Comparing() {
		t.Error("关开关没有走 ClearCompare")
	}
}

func TestCompareRowDisabledForMultiRepo(t *testing.T) {
	a := newTestApp(t)
	seedScan(t, a, fixtureDir("shared-lib"))
	seedScan(t, a, fixtureDir("simple-multi-main"))
	if err := a.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(func(c *ui.Context) { a.compareRow(c) }, 900, 80)
	tt.Frame()
	if !tt.HasText("对比需要对单个仓库开启") {
		t.Errorf("多仓选中时缺禁用原因文案，texts=%v", tt.Texts())
	}
}

func TestStatusLineCarriesMergeWarnings(t *testing.T) {
	a := newTestApp(t)
	a.status = StatusReady
	a.mergeWarnings = []string{"repo:/a 与 repo:/b 争用 cmp:x.Y", "再来一条"}
	got := a.StatusLine()
	if !strings.Contains(got, "合并提示 2 条") {
		t.Errorf("StatusLine = %q，缺合并提示计数", got)
	}
	a.mergeWarnings = nil
	if strings.Contains(a.StatusLine(), "合并提示") {
		t.Errorf("没有警告时不该出现合并提示：%q", a.StatusLine())
	}
}
```

两个测试专用字段（`compareRepoForTest`/`compareFromForTest`）的存在理由：`compareRow` 的开关在测试里要能开起来，而 `SetCompare` 需要的 `fromID` 是库里的行，UI 上靠点 chips 选。测试直接指定比造五颗 chips 再点击更省；实现里 `toggleCompare()` 读它们。**这两个字段是测试接缝，不进产品语义**：实现时把它们命名成 `a.compareSeed struct{ repoID, fromID string }`，并在 `compareRow` 里当 `a.compareSeed.repoID` 非空时优先用它——真窗口里它永远是零值，走 `a.repoID` + 最新之外的第一条 chips。写进 findings：这是 headless 无法可靠点击 chips 列表（按钮标签是动态 id 尾串）的取舍。

- [ ] **Step 2: 跑测试确认失败**

```sh
go test ./internal/ui/ -run 'TestSnapshotChipLabel|TestCompareRow|TestStatusLine' -v
```

期望：编译失败 `a.compareRow` 未定义。

- [ ] **Step 3: 状态字段 + Reload**

`state.go` 的 `App` 里，`compare` 字段之后：

```go
	// compareOptions 是最新 5 个快照（Reload 时读，渲染不做 IO）。第一条永远是
	// 当前基线，所以它是 chips 里不可点的那一颗。
	compareOptions []model.ArchitectureSnapshot
	// compareSeed 是测试接缝，见 page_compare_test.go 的说明。真窗口不写它。
	compareSeed struct{ repoID, fromID string }
```

`compareSnapshotChoices` 与 `CompareOptions`：

```go
// compareSnapshotChoices 是 spec §10「最近 5 次扫描」的窗口，也是抽屉历史段
// 的窗口（T22）。两处同值不是巧合，是同一个"给人看的最近"的定义。
const compareSnapshotChoices = 5

func (a *App) CompareOptions() []model.ArchitectureSnapshot { return a.compareOptions }
```

`Reload`（`app.go:108-116` 那段 `if len(selected) == 1` 里）在 `a.graph = ...` 之后补：

```go
		// 单仓时把最近 5 个快照读出来给对比 chips。多仓时清空——对比只在
		// CompareAllowed 的场合有意义，留着旧列表只是让 chips 画出一屏点不动的按钮。
		if a.repoID != "" {
			opts, err := a.db.Snapshots(ctx, a.repoID, compareSnapshotChoices)
			if err != nil {
				return err
			}
			a.compareOptions = opts
		} else {
			a.compareOptions = nil
		}
```

（`a.repoID` 的赋值在上面几行，已就绪。`Snapshots` 按 id 倒序返回，`#1` 即基线。）

`StatusLine` 的两个终态分支改成先算 base、再统一追加提示——计数行与提示同一条，状态行的宽度才有界：

```go
func (a *App) StatusLine() string {
	base := a.statusLineBase()
	if n := len(a.mergeWarnings); n > 0 {
		return base + fmt.Sprintf("，合并提示 %d 条", n)
	}
	return base
}

// statusLineBase 是切片 1 的原 StatusLine 函数体，一字未改地搬进这里。
func (a *App) statusLineBase() string { /* 原 StatusLine 的 switch 全部内容 */ }
```

- [ ] **Step 4: page_compare.go**

```go
package ui

import (
	"context"
	"fmt"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

// compareRow 是图页的对比区：开关 + 基线 chips + 计数行。它单独成文件是因为
// 这块的状态机（T20）与它的绘制一起是切片 2 新增的，跟切片 1 的 mapHeader
// 挤在同一个文件里会把两边的阅读顺序都搅乱。
func (a *App) compareRow(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).FillWidth().Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
		components.Icon(c, lucide.GitCompare).Size(14, 14)
		if !a.CompareAllowed() {
			// 禁用必须带原因：只有一个灰开关，用户读到的是"坏了"。
			components.Text(c, "对比模式").FontSize(t.Rem(.85)).TextColor(t.TextMuted)
			components.Text(c, "对比需要对单个仓库开启").FontSize(t.Rem(.8)).TextColor(t.TextMuted)
			return
		}
		on := a.Comparing()
		sw := ui.Switch(c, &on).Label("对比模式")
		if sw.Changed() {
			a.toggleCompare()
		}
		components.Text(c, "对比").FontSize(t.Rem(.85)).TextColor(t.TextMuted)
		if a.Comparing() {
			a.compareChips(c)
			components.Text(c, a.CompareSummary()).
				FontSize(t.Rem(.8)).TextColor(t.TextMuted)
		}
	})
}

// toggleCompare 开时按 compareSeed 或"最近 5 条里除了基线的第一条"落到
// SetCompare，关时走 ClearCompare。错误不吞：状态行的失败分支已经有位置。
func (a *App) toggleCompare() {
	ctx := context.Background()
	if a.Comparing() {
		a.ClearCompare()
		return
	}
	repoID, fromID := a.compareSeed.repoID, a.compareSeed.fromID
	if repoID == "" {
		repoID = a.repoID
	}
	if fromID == "" {
		// 默认比「上一版」：compareOptions 倒序，[0] 是基线，[1] 是上一版。
		if len(a.compareOptions) < 2 {
			return // 只有一次扫描，没有可比的过去
		}
		fromID = a.compareOptions[1].ID
	}
	// 这里的错误只影响这一行的显示：图继续是最新快照的图。
	_ = a.SetCompare(ctx, repoID, fromID)
}

func (a *App) compareChips(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Gap(t.Space(1)).Wrap().Children(func() {
		for i, snap := range a.compareOptions {
			label := snapshotChipLabel(snap, i+1)
			if i == 0 {
				// 基线不可点：它是 to，不是候选。
				components.Text(c, label).FontSize(t.Rem(.8)).TextColor(t.TextMuted)
				continue
			}
			chip := ui.Button(c, label)
			chip.Border(1, pick(a.compare != nil && a.compare.fromID == snap.ID,
				t.Accent, t.Border))
			id := snap.ID
			if chip.Clicked() {
				a.compareSeed.fromID = id
				if err := a.SetCompare(context.Background(), a.compareRepo(), id); err != nil {
					a.err = err
					a.status = StatusFailed
				}
			}
		}
	})
}

func (a *App) compareRepo() string {
	if a.compareSeed.repoID != "" {
		return a.compareSeed.repoID
	}
	return a.repoID
}

// snapshotChipLabel：序号在前，理由见测试。#1 标「当前」，因为它就是对比的基线。
func snapshotChipLabel(snap model.ArchitectureSnapshot, rank int) string {
	base := snap.CreatedAt
	if base == "" {
		base = snap.ID
	}
	if rank == 1 {
		return fmt.Sprintf("#%d %s（当前）", rank, base)
	}
	return fmt.Sprintf("#%d %s", rank, base)
}
```

- [ ] **Step 5: mapHeader 接线**

`page_graph.go` 的 `graphPage`，在 `a.mapHeader(c)` 之后加一行 `a.compareRow(c)`（它自带整行与图标，挤进 mapHeader 会让那一行有 9 个控件）。

- [ ] **Step 6: 全绿 + 提交**

```sh
go test ./internal/ui/ -run 'TestSnapshotChipLabel|TestCompare|TestStatusLine|TestReload|TestGraph' -v
go vet ./... && go test ./...
git add internal/ui/page_compare.go internal/ui/page_compare_test.go internal/ui/page_graph.go internal/ui/state.go internal/ui/app.go
git commit -m "feat: 图页对比开关、最近 5 快照 chips 与合并提示状态行"
```

---

### Task 22: 抽屉历史段

**Files:**
- Modify: `internal/ui/detail_panel.go`（新增 `detailHistory`）
- Modify: `internal/ui/state.go`（`Select` 装载历史；`App` 加两个字段）
- Test: `internal/ui/detail_panel_test.go`

**Interfaces:**
- Consumes: T3 的 `DB.History(ctx, elementID, limit)` / `DB.HistoryCount(ctx, elementID)`、T10 的 `Settings.MaxSnapshots`、`compareSnapshotChoices`（T21）。
- Produces:

```go
// App 字段
history      []model.HistoryEvent
historyTotal int
// 方法
func (a *App) loadHistory(ctx context.Context, id string)
func (a *App) detailHistory(c *ui.Context, el model.Element)
func (a *App) HistoryWindow() int
func historyEventLabel(event string) string
```

**定稿（读取时机）**：`Select` 里**同步**读库。spec §5.3 的线程纪律是「重活进 goroutine」，历史读的是 `element_history` 的一条 `WHERE element_id = ?` 加 `LIMIT 5`，与抽屉本来就在读的 `a.view` 邻居遍历同量级；异步化要付一个「先画空段、post 回来再重画」的状态机，而这一段的收益只是省一次索引查询。代价：`Select` 现在会碰库，`NewApp(nil)` 的测试路径必须走 `a.db == nil` 的守卫（这条守卫 `saveRow` 早就有了，`page_settings.go:99` 记着同一条 panic 链）。

**定稿（文案）**：`历史（近 50 次扫描内，共 3 条）`。窗口数字来自 `a.settings.MaxSnapshots`，不是硬编码——T3 的封顶裁剪正是按这个值删快照的，快照删了它的历史行也跟着 CASCADE 掉。spec §13 点名的「UI 文案要诚实」就落在这五个字上。

- [ ] **Step 1: 写失败测试**

追加到 `internal/ui/detail_panel_test.go`：

```go
// seedHistory 写两次扫描：第一次让 Basket 出现，第二次把它改掉。历史行由
// CommitScan 一并写（T3 的 ScanCommit.History），不是测试手插的——手插会绕开
// 扫描器真正生成的事件形状。
func seedHistory(t *testing.T, a *App) (repoID string) {
	t.Helper()
	ctx := context.Background()
	repoID = "repo:/history"
	if err := a.db.UpsertRepo(ctx, model.Repo{ID: repoID, Name: "history",
		LocalPath: "/history", Lang: "go"}); err != nil {
		t.Fatal(err)
	}
	id := "cmp:example.com/h/internal/basket.Basket"
	for _, step := range []struct {
		snap, event, desc string
	}{{"s0000000000000000001", model.EventAppeared, "Basket holds items."},
		{"s0000000000000000002", model.EventChanged, "Basket holds items and total."}} {
		elts := []model.Element{{ID: id, Type: model.TypeComponent, Name: "Basket",
			Description: step.desc, RepoID: repoID, FilePath: "internal/basket/basket.go"}}
		rels := []model.Relationship{{FromID: model.SystemID("example.com/h"),
			ToID: id, Type: model.RelContains, Weight: 1}}
		history := []model.HistoryEvent{{RepoID: repoID, ElementID: id,
			SnapshotID: step.snap, Event: step.event}}
		if step.event == model.EventChanged {
			history[0].Fields = []string{"description"}
		}
		err := a.db.CommitScan(ctx, storeScanCommit(step.snap, repoID, elts, rels, history))
		if err != nil {
			t.Fatalf("commit %s: %v", step.snap, err)
		}
	}
	return repoID
}

func TestSelectLoadsHistory(t *testing.T) {
	a := newTestApp(t)
	repoID := seedHistory(t, a)
	if err := a.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	if a.repoID != repoID {
		t.Fatalf("reload repo = %q", a.repoID)
	}
	a.SetShowComponents(true)
	const id = "cmp:example.com/h/internal/basket.Basket"
	a.Select(id)
	if len(a.history) != 2 || a.historyTotal != 2 {
		t.Fatalf("history = %d/%d, want 2/2", len(a.history), a.historyTotal)
	}
	// 倒序：最新在前。抽屉只有五行的地盘，先看最近发生了什么。
	if a.history[0].Event != model.EventChanged {
		t.Errorf("newest event = %q, want changed", a.history[0].Event)
	}
	tt := newDetailTester(t, a) // detail_panel_test.go:77 的既有助手
	tt.Frame()
	for _, want := range []string{"历史", "变更", "新增", "description", "近 50 次扫描内"} {
		if !tt.HasText(want) {
			t.Errorf("抽屉历史段缺 %q，texts=%v", want, tt.Texts())
		}
	}
}

func TestSelectEmptyClearsHistory(t *testing.T) {
	a := newTestApp(t)
	seedHistory(t, a)
	if err := a.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	a.Select("cmp:example.com/h/internal/basket.Basket")
	a.Select("")
	if a.history != nil || a.historyTotal != 0 {
		t.Errorf("Select(\"\") 之后仍留着历史：%+v / %d", a.history, a.historyTotal)
	}
}

// TestHistoryReadIsNilDBSafe: NewApp(nil) 是既有的测试路径（saveRow 为它写了
// Disabled(a.db == nil)，page_settings.go:99）。Select 现在会读库，同一条守卫
// 必须在这儿，否则点一下就 panic。
func TestHistoryReadIsNilDBSafe(t *testing.T) {
	a := NewApp(nil)
	a.Select("cmp:example.com/x/y.Y") // 不许 panic
	if len(a.history) != 0 {
		t.Error("nil db 下不该有历史")
	}
}

func TestHistoryEventLabels(t *testing.T) {
	for _, tc := range []struct{ event, want string }{
		{model.EventAppeared, "新增"},
		{model.EventChanged, "变更"},
		{model.EventDisappeared, "消失"},
		{"nonsense", "未知"},
	} {
		if got := historyEventLabel(tc.event); got != tc.want {
			t.Errorf("historyEventLabel(%q) = %q, want %q", tc.event, got, tc.want)
		}
	}
}
```

`storeScanCommit`（`compare_test.go` 里那个）本任务的版本多一个 `history` 参数——**改成一个变参**，别复制第二个函数：

```go
func storeScanCommit(id, repoID string, elts []model.Element,
	rels []model.Relationship, history ...[]model.HistoryEvent) store.ScanCommit {
	c := store.ScanCommit{
		Snapshot:      model.ArchitectureSnapshot{ID: id, RepoID: repoID},
		Elements:      elts,
		Relationships: rels,
		KeepSnapshots: 50,
	}
	if len(history) > 0 {
		c.History = history[0]
	}
	return c
}
```

- [ ] **Step 2: 跑测试确认失败**

```sh
go test ./internal/ui/ -run 'TestSelectLoadsHistory|TestSelectEmptyClears|TestHistoryRead|TestHistoryEventLabels' -v
```

期望：编译失败 `a.history` 未定义。

- [ ] **Step 3: App 字段与 Select 接线**

`state.go` 的 `App`：

```go
	// history 是当前选中元素最近 compareSnapshotChoices 条事件（最新在前），
	// historyTotal 是该事件在保留窗口内的总数。两者由 Select 同步装载，
	// 抽屉只读不查（见 T22 的读取时机定稿）。
	history      []model.HistoryEvent
	historyTotal int
```

`Select`（`state.go:190-201`）：

```go
func (a *App) Select(id string) {
	a.selected = id
	if id == "" {
		a.impact = nil
		a.drillInto = ""
		a.history, a.historyTotal = nil, 0
		return
	}
	a.remember(id)
	a.loadHistory(context.Background(), id)
	if a.impactMode {
		a.analyse(id)
	}
}

// loadHistory 读 element_history。守卫 a.db == nil 是 NewApp(nil) 那条测试路径
// 的既有先例（page_settings.go:99）：grouped 查询在 nil 句柄上会 panic。
// 读失败留空段而不是报错：历史是抽屉的补充信息，不该让一次查询失败把详情面板
// 变成错误页。
func (a *App) loadHistory(ctx context.Context, id string) {
	a.history, a.historyTotal = nil, 0
	if a.db == nil {
		return
	}
	events, err := a.db.History(ctx, id, compareSnapshotChoices)
	if err != nil {
		return
	}
	total, err := a.db.HistoryCount(ctx, id)
	if err != nil {
		total = len(events)
	}
	a.history, a.historyTotal = events, total
}

// HistoryWindow 是文案里那个 N：保留窗口按 settings.MaxSnapshots 走，零值
// （没读过配置的测试）退回默认 50。
func (a *App) HistoryWindow() int {
	if a.settings.MaxSnapshots > 0 {
		return a.settings.MaxSnapshots
	}
	return config.DefaultMaxSnapshots
}
```

（`state.go` 已 import `config`；`Select` 需要 `context`，已在 import 块里。）

- [ ] **Step 4: detailHistory**

`detail_panel.go` 的 `detailPanel` 滚动列里，`a.detailImpact(c, el)` 之后加 `a.detailHistory(c, el)`；函数体：

```go
// historyEventLabel 是 model 的事件常量的人话名字。未知值给「未知」而不是丢掉：
// 库里存了个没见过的 event 是 bug，画出来才看得见。
func historyEventLabel(event string) string {
	switch event {
	case model.EventAppeared:
		return "新增"
	case model.EventChanged:
		return "变更"
	case model.EventDisappeared:
		return "消失"
	default:
		return "未知"
	}
}

// detailHistory 是 spec §10 第 3 条：最近 5 条 + 计数。段本身对任何选中元素都画
// （包括组节点 pkg:/sys:——它们没有历史，此时只有标题行），因为「这条没有历史」
// 和「这一段没画」必须在视觉上可区分，否则用户以为功能坏了。
func (a *App) detailHistory(c *ui.Context, el model.Element) {
	t := c.Theme()
	head := fmt.Sprintf("历史（近 %d 次扫描内，共 %d 条）", a.HistoryWindow(), a.historyTotal)
	ui.Column(c).FillWidth().Gap(t.Space(0.5)).Children(func() {
		components.Text(c, head).FontSize(t.Rem(.75)).TextColor(t.TextMuted)
		if len(a.history) == 0 {
			components.Text(c, "这一次扫描窗口内没有它的事件").
				FontSize(t.Rem(.85)).TextColor(t.TextMuted)
			return
		}
		for _, ev := range a.history {
			ui.Row(c).FillWidth().Gap(t.Space(1)).AlignItems(ui.Center).Children(func() {
				components.Text(c, historyEventLabel(ev.Event)).
					FontSize(t.Rem(.85)).FontWeight(600).TextColor(t.Text)
				line := ev.CreatedAt
				if len(ev.Fields) > 0 {
					if line != "" {
						line += " · "
					}
					line += strings.Join(ev.Fields, "、")
				}
				components.Text(c, line).FontSize(t.Rem(.8)).
					SingleLine().TextColor(t.TextMuted)
			})
		}
		if a.historyTotal > len(a.history) {
			components.Text(c, fmt.Sprintf("仅显示最近 %d 条", len(a.history))).
				FontSize(t.Rem(.75)).TextColor(t.TextMuted)
		}
	})
}
```

`model.HistoryEvent` 的 `CreatedAt` 由 T2 定义、T3 的写入路径从库里带回来。若某条事件的 `CreatedAt` 为空（测试里手造的 `CommitScan` 不带时间），那一行的文本就是字段列表——不会画出空行。

- [ ] **Step 5: 全绿 + 提交**

```sh
go test ./internal/ui/ -run 'TestSelect|TestHistory|TestDetail' -v
go vet ./... && go test ./...
git add internal/ui/detail_panel.go internal/ui/detail_panel_test.go internal/ui/state.go
git commit -m "feat: 抽屉历史段（element_history 最近 5 条 + 保留窗口计数）"
```

---

### Task 23: 仓库页转正、新鲜度后台检查与设置两行

**Files:**
- Modify: `internal/ui/page_repos.go`
- Modify: `internal/ui/page_settings.go`
- Modify: `internal/ui/state.go`（字段 + `SetPage`）
- Modify: `internal/ui/app.go`（`finishScan` 出队下一条）
- Test: `internal/ui/page_repos_test.go`、`internal/ui/page_settings_test.go`

**Interfaces:**
- Consumes: T4 的 `DB.DeleteRepo`/`DB.Repo`、T11 的 `scanner.Verify`、T10 的 `Settings.Incremental`/`MaxSnapshots`、既有的 `DB.UpsertRepo`、`scanner.Detect(root) (string, error)`（`internal/scanner/detector.go:23`）、`model.RepoIDFor`。
- Produces:

```go
type freshness uint8
const (
	freshUnknown freshness = iota
	freshClean
	freshStale
)
func (a *App) AddRepo(ctx context.Context, path string) error
func (a *App) RemoveRepo(ctx context.Context, repoID string) error
func (a *App) QueueAllScans(ctx context.Context) error
func (a *App) ConfirmDelete(repoID string)
func (a *App) CancelDelete()
func (a *App) SetNewRepoPath(p string)
func (a *App) startFreshnessCheck()
func (f freshness) Label() string
// App 字段：newRepoPath, reposErr, confirmDelete string,
//          freshness map[string]freshness, freshnessPending bool, scanQueue []string
```

**定稿（新鲜度不写 app_config）**：spec §10 说新鲜度徽标「扫描后缓存进 `app_config`」。本计划不进库，改成**进仓库页时后台算一次、缓存在内存里**。理由三条：(1) `app_config` 是设置页唯一的写路径，让一次页面浏览写库会把「保存设置」的回执语义搅浑；(2) verify 是纯哈希比对（T11），一次全仓 ≈ 文件读 + sha256，切片 1 实测 OpenCourt 全量解析才 2.7 s，哈希段比它小两个数量级；(3) 缓存进库就要处理「另一进程扫完」的失效，而本地单用户重启后重算的代价是零。代价：重启后进第一遍仓库页要等一次后台检查（有 pending 态就看得见）。

**定稿（删除确认用两段式，不用原生对话框）**：`mygo.Dialog` 阻塞到关闭、且在 `App.Run` 之前调用直接 panic（切片 1 在 `page_settings.go:39-46` 已经把这条链记清楚了，目录选择器就因为这两条退成了输入框）。删除是行内两段式：点「删除」→ 该行变成「确认删除 / 取消」。

- [ ] **Step 1: 写失败测试**

`internal/ui/page_repos_test.go` 追加：

```go
func TestAddRepoValidatesPath(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	if err := a.AddRepo(ctx, fixtureDir("shared-lib")); err != nil {
		t.Fatalf("AddRepo: %v", err)
	}
	repos, err := a.db.Repos(ctx)
	if err != nil || len(repos) != 1 {
		t.Fatalf("repos = %d, %v", len(repos), err)
	}
	if repos[0].Lang != "go" || filepath.Base(repos[0].LocalPath) != "shared-lib" {
		t.Errorf("stored repo = %+v", repos[0])
	}
	// 没有 go.mod 的目录必须被拒（同 BeginScan 的门，app.go:139）。
	if err := a.AddRepo(ctx, fixtureDir("shared-lib/core")); err == nil {
		t.Error("非 Go 目录必须被拒")
	}
	if err := a.AddRepo(ctx, "/definitely/not/here"); err == nil {
		t.Error("不存在的路径必须被拒")
	}
}

func TestAddRepoRelativePathIsAbsolutized(t *testing.T) {
	a := newTestApp(t)
	if err := a.AddRepo(context.Background(), fixtureDir("shared-lib")); err != nil {
		t.Fatal(err)
	}
	repos, err := a.db.Repos(context.Background())
	if err != nil || len(repos) != 1 {
		t.Fatal(err)
	}
	if !filepath.IsAbs(repos[0].LocalPath) {
		t.Errorf("local_path = %q, must be absolute", repos[0].LocalPath)
	}
	// 幂等：同一个目录再加一次不产生第二行（RepoIDFor 是主键）。
	if err := a.AddRepo(context.Background(), repos[0].LocalPath); err != nil {
		t.Fatal(err)
	}
	repos, _ = a.db.Repos(context.Background())
	if len(repos) != 1 {
		t.Errorf("重复添加产生了两行：%+v", repos)
	}
}

func TestRemoveRepoNeedsConfirmation(t *testing.T) {
	a := newTestApp(t)
	if err := a.AddRepo(context.Background(), fixtureDir("shared-lib")); err != nil {
		t.Fatal(err)
	}
	if err := a.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(func(c *ui.Context) { a.reposPage(c) }, 900, 500)
	tt.Frame()
	if err := tt.Click("删除"); err != nil {
		t.Fatalf("click 删除: %v (texts=%v)", err, tt.Texts())
	}
	tt.Frame()
	if !tt.HasText("确认删除") || !tt.HasText("取消") {
		t.Errorf("第一段点击后没进确认态：%v", tt.Texts())
	}
	repos, _ := a.db.Repos(context.Background())
	if len(repos) != 1 {
		t.Fatal("第一段点击就把仓删了——确认还没发生")
	}
	a.ConfirmDelete()
	repos, _ = a.db.Repos(context.Background())
	if len(repos) != 0 {
		t.Errorf("ConfirmDelete 没删掉：%+v", repos)
	}
}

func TestConfirmDeleteCancel(t *testing.T) {
	a := newTestApp(t)
	_ = a.AddRepo(context.Background(), fixtureDir("shared-lib"))
	if err := a.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	id := a.repos[0].ID
	a.ConfirmDelete(id)
	if a.confirmDelete != id {
		t.Fatalf("第一段只记录待确认，confirmDelete = %q", a.confirmDelete)
	}
	a.CancelDelete()
	if a.confirmDelete != "" {
		t.Error("取消没清空")
	}
	a.ConfirmDelete() // 无待确认项时必须报错而不 panic
	if _, err := a.db.Repo(context.Background(), id); err != nil {
		t.Error("无待确认时第二段执行了删除")
	}
}

func TestQueueAllScansEnqueuesSnapshotlessRepos(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	// shared-lib 扫过一次（有快照），crossmod-dep 只添加没扫。
	seedScan(t, a, fixtureDir("shared-lib"))
	if err := a.AddRepo(ctx, fixtureDir("crossmod-dep")); err != nil {
		t.Fatal(err)
	}
	if err := a.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	if err := a.QueueAllScans(ctx); err != nil {
		t.Fatalf("QueueAllScans: %v", err)
	}
	// 只有没有快照的仓进队：全量重扫是「扫描全部」的反面。
	if len(a.scanQueue) != 1 || a.scanQueue[0] != abs(t, fixtureDir("crossmod-dep")) {
		t.Errorf("scanQueue = %v", a.scanQueue)
	}
	drained := make(chan func(), 8)
	a.SetPost(func(fn func()) { drained <- fn })
	for len(a.scanQueue) > 0 || a.status == StatusScanning {
		if a.status != StatusScanning {
			t.Fatalf("队列还有 %d 条但没在扫：%v", len(a.scanQueue), a.scanQueue)
		}
		(<-drained)() // 主线程跑 finishScan → 出队下一条
	}
	if err := a.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	for _, r := range a.repos {
		if a.repoLatest[string(r.ID)] == "" {
			t.Errorf("仓 %s 扫完仍无快照", r.Name)
		}
	}
}

func TestFreshnessCheckPublishesThroughPost(t *testing.T) {
	a := newTestApp(t)
	seedScan(t, a, fixtureDir("shared-lib"))
	if err := a.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	queued := make(chan func(), 8)
	a.SetPost(func(fn func()) { queued <- fn })
	a.SetPage(PageRepos) // 触发后台检查
	if !a.freshnessPending {
		t.Fatal("SetPage(PageRepos) 没有挂起检查")
	}
	for len(queued) > 0 {
		(<-queued)()
	}
	if a.freshnessPending {
		t.Error("检查结果没有通过 post 落回主线程")
	}
	if got := a.freshness[abs(t, fixtureDir("shared-lib"))]; got != freshClean {
		// key 是 repo id，不是路径——见实现。这里用 repo id 重取一次。
		if got := a.freshness[a.repos[0].ID]; got != freshClean {
			t.Errorf("freshness = %v, want freshClean for an untouched scan", map[string]freshness(a.freshness))
		}
	}
}

func TestFreshnessLabel(t *testing.T) {
	for _, tc := range []struct {
		f    freshness
		want string
	}{{freshUnknown, "未检查"}, {freshClean, "新鲜"}, {freshStale, "可能已过期"}} {
		if got := tc.f.Label(); got != tc.want {
			t.Errorf("Label(%v) = %q", tc.f, got)
		}
	}
}
```

`abs(t, path)` 是新助手（`fixtureDir` 返回的是相对路径，`BeginScan`/`AddRepo` 一律绝对化）：

```go
func abs(t *testing.T, path string) string {
	t.Helper()
	p, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
```

`internal/ui/page_settings_test.go` 追加两行设置：

```go
func TestIncrementalSwitchPersists(t *testing.T) {
	a := newTestApp(t)
	if !a.settings.Incremental {
		t.Fatal("scan.incremental 的默认必须是开（T10）")
	}
	tt := ui.NewTester(func(c *ui.Context) { a.settingsPage(c) }, 700, 600)
	tt.Frame()
	if err := tt.Click("增量扫描"); err != nil {
		t.Fatalf("click: %v (texts=%v)", err, tt.Texts())
	}
	if a.settings.Incremental {
		t.Error("开关没写进 settings")
	}
	// 保存走既有的 saveRow；重开一次 Load 才是真验收。
	if err := tt.Click("保存设置"); err != nil {
		t.Fatal(err)
	}
	got, err := config.Load(context.Background(), a.db)
	if err != nil || got.Incremental {
		t.Errorf("persisted incremental = %v, %v", got.Incremental, err)
	}
}

func TestMaxSnapshotsFieldKeepsItsBounds(t *testing.T) {
	a := newTestApp(t)
	a.settings.MaxSnapshots = config.DefaultMaxSnapshots
	for i := 0; i < 100; i++ {
		_ = a.BumpMaxSnapshots(-1)
	}
	if a.settings.MaxSnapshots != minKeepSnapshots {
		t.Errorf("下界没夹住：%d", a.settings.MaxSnapshots)
	}
	for i := 0; i < 600; i++ {
		_ = a.BumpMaxSnapshots(1)
	}
	if a.settings.MaxSnapshots != maxKeepSnapshots {
		t.Errorf("上界没夹住：%d", a.settings.MaxSnapshots)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

```sh
go test ./internal/ui/ -run 'TestAddRepo|TestRemoveRepo|TestConfirmDelete|TestQueueAll|TestFreshness|TestIncrementalSwitch|TestMaxSnapshots' -v
```

期望：编译失败 `a.AddRepo` 未定义。

- [ ] **Step 3: App 字段与 SetPage 钩子**

`state.go`：

```go
	// 仓库页的三条输入态。newRepoPath 是添加框；reposErr 是它最近一次的失败
	// 原因（画在框下面，不是弹窗）；confirmDelete 是两段式删除的第一段记下的
	// 待确认 repo id，空串表示没有待确认项。
	newRepoPath   string
	reposErr      error
	confirmDelete string
	// freshness 是 repo id → 新鲜度，只由后台检查通过 post 写。pending 标志既是
	// 重复触发的闸门，也是"未检查"文案的来源。
	freshness        map[string]freshness
	freshnessPending bool
	// scanQueue 是「扫描全部」排进来的待扫根目录（绝对路径），由 finishScan
	// 在主线程逐条出队。
	scanQueue []string
```

`SetPage`（`state.go:158`）：

```go
func (a *App) SetPage(p Page) {
	a.page = p
	if p == PageRepos {
		a.startFreshnessCheck()
	}
}
```

```go
type freshness uint8

const (
	freshUnknown freshness = iota
	freshClean
	freshStale
)

func (f freshness) Label() string {
	switch f {
	case freshClean:
		return "新鲜"
	case freshStale:
		return "可能已过期"
	default:
		return "未检查"
	}
}

// startFreshnessCheck 对每个有快照的仓跑一次 scanner.Verify。整段在 goroutine
// 里，结果一律经 post 落回主线程（spec §5.3）；pending 闸门让反复切页不会堆出
// 十个并发检查。Verify 只读库、只读源文件，不写任何东西。
func (a *App) startFreshnessCheck() {
	if a.db == nil || a.freshnessPending || len(a.repos) == 0 {
		return
	}
	if a.freshness == nil {
		a.freshness = make(map[string]freshness, len(a.repos))
	}
	targets := make([]model.Repo, 0, len(a.repos))
	for _, r := range a.repos {
		if a.repoLatest[string(r.ID)] != "" {
			targets = append(targets, r)
		}
	}
	if len(targets) == 0 {
		return
	}
	a.freshnessPending = true
	go func() {
		result := make(map[string]freshness, len(targets))
		for _, r := range targets {
			res, err := scanner.Verify(context.Background(), a.db, string(r.ID))
			switch {
			case err != nil:
				result[string(r.ID)] = freshUnknown
			case res.Clean:
				result[string(r.ID)] = freshClean
			default:
				result[string(r.ID)] = freshStale
			}
		}
		a.post(func() {
			a.freshness = result
			a.freshnessPending = false
		})
	}()
}
```

- [ ] **Step 4: AddRepo / RemoveRepo / QueueAllScans**

```go
// AddRepo 把路径登记进 repos 表但不扫描——仓库页的职责是「列得出、点得动」，
// 扫描仍是那一行的雷达按钮。校验与 BeginScan 同一条门（scanner.Detect），
// 于是"添加得进、扫不动"这种半死状态不存在。
func (a *App) AddRepo(ctx context.Context, path string) error {
	if a.db == nil {
		return fmt.Errorf("ui: 没有可用的库")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		a.reposErr = err
		return err
	}
	lang, err := scanner.Detect(abs)
	if err != nil {
		a.reposErr = err
		return err
	}
	a.reposErr = nil
	r := model.Repo{ID: model.RepoIDFor(abs), Name: filepath.Base(abs), LocalPath: abs, Lang: lang}
	if err := a.db.UpsertRepo(ctx, r); err != nil {
		a.reposErr = err
		return err
	}
	a.newRepoPath = ""
	return a.Reload(ctx)
}

func (a *App) SetNewRepoPath(p string) { a.newRepoPath = p }

// ConfirmDelete 有两个形态：带 repoID 是「第一段：记下待确认」；不带参数是
// 「第二段：执行」。这个重载形状和 ToggleGraphRepo 一样是点击回调的直接形态，
// 因为行内按钮的闭包只能拿到自己那一行的 id。
func (a *App) ConfirmDelete(repoID ...string) {
	if len(repoID) > 0 {
		a.confirmDelete = repoID[0]
		return
	}
	if a.confirmDelete == "" || a.db == nil {
		return
	}
	id := a.confirmDelete
	a.confirmDelete = ""
	if err := a.db.DeleteRepo(context.Background(), id); err != nil {
		a.reposErr = err
		return
	}
	a.reposErr = nil
	_ = a.Reload(context.Background())
}

func (a *App) CancelDelete() { a.confirmDelete = "" }

// QueueAllScans 只把「没有快照的仓」排进队：spec §10 的「全部重扫」在观察库里
// 面对的是三个仓 6 秒的解析，每次切页都重扫是把 PoC-3 的数字当装饰。
// 需要真正的强制重扫时，用 CLI 的 `oac scan --full`（T16）。
func (a *App) QueueAllScans(ctx context.Context) error {
	if a.status == StatusScanning {
		return ErrScanInFlight
	}
	for _, r := range a.repos {
		if a.repoLatest[string(r.ID)] == "" {
			a.scanQueue = append(a.scanQueue, r.LocalPath)
		}
	}
	if len(a.scanQueue) == 0 {
		return nil
	}
	root := a.scanQueue[0]
	a.scanQueue = a.scanQueue[1:]
	return a.BeginScan(root)
}
```

`app.go` 的 `finishScan` 末尾（`StatusReady` 之后）补出队——这一段必须在主线程，因为它改 `status` 与 `scanQueue`：

```go
	// 「扫描全部」的队列由上一条扫描的收尾驱动。这里忽略错误：BeginScan 已经把
	// 它自己那条失败写进 a.err 与 StatusFailed，一条接一条地覆盖没有意义。
	if len(a.scanQueue) > 0 {
		root := a.scanQueue[0]
		a.scanQueue = a.scanQueue[1:]
		_ = a.BeginScan(root)
	}
```

**注意顺序**：`BeginScan` 第一件事是 `if a.status == StatusScanning { return ErrScanInFlight }`，而 `finishScan` 在上面几行刚把 status 写成 `StatusReady`，所以这里不会被自己挡住。若把这段插到 `a.status = StatusReady` 之前，第一条排队就永远失败——这是这条队列唯一的隐形前提，写在这里而不是藏在注释里。

- [ ] **Step 5: 仓库页绘制**

`reposPage` 的头部那一行之后（`ui.Scroll(...)` 之前）插入添加行与「扫描全部」，`repoRow` 加徽标与删除：

```go
// addRepoRow：输入框 + 添加按钮 + 错误行。输入框绑 a.newRepoPath 本身
// （components.Text 的 Editable 立刻写回绑定，page_settings.go:51 的同一条机制）。
func (a *App) addRepoRow(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).FillWidth().Gap(t.Space(1)).Children(func() {
		ui.Row(c).FillWidth().Gap(t.Space(1)).Children(func() {
			field := components.Text(c, "", components.TextOptions{Editable: &a.newRepoPath})
			field.Label("仓库绝对路径").FontSize(t.Rem(.9)).TextColor(t.Text).
				Padding(t.Space(1)).Radius(t.Radius).Border(1, t.Border).Grow(1).MinWidth(0)
			add := components.Button(c, "添加仓库", components.ButtonPrimary)
			add.Disabled(a.db == nil || a.newRepoPath == "")
			if add.Clicked() {
				_ = a.AddRepo(context.Background(), a.newRepoPath)
			}
			all := sidebar.IconButton(c, lucide.RefreshCw(), "扫描全部未扫描的仓库")
			all.Disabled(a.status == StatusScanning)
			if all.Clicked() {
				_ = a.QueueAllScans(context.Background())
			}
		})
		components.Text(c, "填仓库根目录的绝对路径（该目录要有 go.mod）。").
			FontSize(t.Rem(.8)).TextColor(t.TextMuted)
		if a.reposErr != nil {
			components.Text(c, a.reposErr.Error()).FontSize(t.Rem(.85)).TextColor(t.Danger)
		}
	})
}

func (a *App) repoRow(c *ui.Context, repo model.Repo) {
	t := c.Theme()
	id := string(repo.ID)
	deleting := a.confirmDelete == id
	ui.Row(c).FillWidth().Gap(t.Space(2)).AlignItems(ui.Center).
		Padding(t.Space(1.5)).Radius(t.Radius).Border(1, t.Border).Children(func() {
		components.Icon(c, lucide.Package).Size(18, 18)
		ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space(0.5)).Children(func() {
			components.Text(c, repo.Name).FontWeight(600).TextColor(t.Text).SingleLine()
			components.Text(c, repo.LocalPath).FontSize(t.Rem(.85)).
				TextColor(t.TextMuted).SingleLine()
		})
		components.Text(c, repo.Lang).FontSize(t.Rem(.85)).TextColor(t.TextMuted).SingleLine()
		// 新鲜度徽标只在有快照时画：没扫过的仓谈不上过期。pending 期间保持
		// "未检查"，用户看得见那一格还没填。
		if a.repoLatest[id] != "" {
			state := a.freshness[id]
			badge := ui.Row(c).Gap(t.Space(0.5)).AlignItems(ui.Center)
			badge.Children(func() {
				if state == freshClean {
					components.Icon(c, lucide.CircleCheck).Size(13, 13)
				} else {
					components.Icon(c, lucide.Clock).Size(13, 13)
				}
				components.Text(c, state.Label()).FontSize(t.Rem(.8)).TextColor(t.TextMuted)
			})
		}
		if a.repoLatest[id] != "" {
			view := ui.Button(c, "查看")
			if view.Clicked() {
				if err := a.SetGraphRepos(context.Background(), []string{id}); err == nil {
					a.SetPage(PageGraph)
				}
			}
		}
		scan := sidebar.IconButton(c, lucide.Radar(), "扫描 "+repo.Name)
		scan.Disabled(a.status == StatusScanning)
		if scan.Clicked() {
			_ = a.BeginScan(repo.LocalPath)
		}
		// 两段式删除。第一段只记下 id，第二段才动库——删除会连带 CASCADE 掉
		// 这个仓的全部快照、元素、边与历史（T4）。
		if deleting {
			yes := ui.Button(c, "确认删除")
			yes.Border(1, t.Danger)
			if yes.Clicked() {
				a.ConfirmDelete()
			}
			no := ui.Button(c, "取消")
			if no.Clicked() {
				a.CancelDelete()
			}
			return
		}
		trash := sidebar.IconButton(c, lucide.Trash(), "删除 "+repo.Name)
		if trash.Clicked() {
			a.ConfirmDelete(id)
		}
	})
}
```

（`emptyRepos` 的文案不动：它已经指向设置页与 ⌘K，而仓库页现在自己就有添加框，所以 `reposPage` 里 `len(a.repos) == 0` 那支要在画完空态后**仍然**画添加行——把 `addRepoRow` 提到那个 `if` 之前，否则「一仓都没有」的新用户看不到入口。）

- [ ] **Step 6: 设置页两行**

```go
const (
	minKeepSnapshots = 1
	maxKeepSnapshots = 500
)

// incrementalField 是 scan.incremental 的开关。绑定的是局部拷贝，写入走 setter
// ——与 page_graph.go:78 的组件层开关同一写法（spec §9.2：写状态只发生在输入回调）。
func (a *App) incrementalField(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).FillWidth().Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space(0.5)).Children(func() {
			components.Text(c, "增量扫描").FontSize(t.Rem(.75)).TextColor(t.TextMuted)
			components.Text(c, "未变的包整段搬上一快照的结果，只重新解析改动过的包。"+
				"关掉等于每次 --full。").FontSize(t.Rem(.8)).TextColor(t.TextMuted)
		})
		on := a.settings.Incremental
		if ui.Switch(c, &on).Label("增量扫描").Changed() {
			a.settings.Incremental = on
			a.settingsSaved = false
		}
	})
}

// maxSnapshotsField 是 history.max_snapshots。上限给到 500 而不是 50：默认 50
// 是 spec 定的，但抽屉历史段的文案要如实报出窗口，窗口太小用户会先调大它
// 再来问「为什么只有 50 条」。
func (a *App) maxSnapshotsField(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).FillWidth().Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space(0.5)).Children(func() {
			components.Text(c, "每仓保留快照数").FontSize(t.Rem(.75)).TextColor(t.TextMuted)
			components.Text(c, strconv.Itoa(a.settings.MaxSnapshots)).
				FontWeight(600).FontSize(t.Rem(1.05)).TextColor(t.Text)
			components.Text(c, "超出的旧快照连同元素、边与历史一起删除（默认 "+
				strconv.Itoa(config.DefaultMaxSnapshots)+"）。").
				FontSize(t.Rem(.8)).TextColor(t.TextMuted)
		})
		less := sidebar.IconButton(c, lucide.Minus(), "减少保留快照数")
		less.Disabled(a.settings.MaxSnapshots <= minKeepSnapshots)
		if less.Clicked() {
			_ = a.BumpMaxSnapshots(-1)
		}
		more := sidebar.IconButton(c, lucide.Plus(), "增加保留快照数")
		more.Disabled(a.settings.MaxSnapshots >= maxKeepSnapshots)
		if more.Clicked() {
			_ = a.BumpMaxSnapshots(1)
		}
	})
}
```

```go
// BumpMaxSnapshots 钳制在 [minKeepSnapshots, maxKeepSnapshots] 并返回是否真变了。
// 钳制放在这里而不是 Disabled 上：Disabled 只管眼睛，键盘与未来的调用路径绕过
// 按钮时，边界还得有人守。
func (a *App) BumpMaxSnapshots(delta int) bool {
	next := a.settings.MaxSnapshots + delta
	if next < minKeepSnapshots {
		next = minKeepSnapshots
	}
	if next > maxKeepSnapshots {
		next = maxKeepSnapshots
	}
	if next == a.settings.MaxSnapshots {
		return false
	}
	a.settings.MaxSnapshots = next
	a.settingsSaved = false
	return true
}
```

`settingsPage` 的 `Children` 里在 `a.depthField(c)` 之后插入这两行，并把 `maxSnapshotsField` 的 −/+ 按钮 `Disabled` 改成读 `a.settings.MaxSnapshots`（上面已经这么写）。

- [ ] **Step 7: 全绿 + 提交**

```sh
go test ./internal/ui/ -run 'TestAddRepo|TestRemoveRepo|TestConfirmDelete|TestQueueAll|TestFreshness|TestIncremental|TestMaxSnapshots|TestSettings' -v
go vet ./... && go test ./...
git add internal/ui/page_repos.go internal/ui/page_repos_test.go internal/ui/page_settings.go internal/ui/page_settings_test.go internal/ui/state.go internal/ui/app.go
git commit -m "feat: 仓库页增删/全部扫描/新鲜度后台检查 + 增量与保留窗口设置行"
```

---

## Phase F — PoC 门槛与收尾

PoC-9（增量收益）与 PoC-10（minimap）已经在 T13 与「已落地清单」里结清；这一相只剩三个门槛：PoC-8（真仓跨边）、PoC-11（合并图首帧）、以及把 CI/`-race`/DoD 一次跑完的收尾。三件事都要**先把数字写进 findings，再决定继续与否**（全局约束：任一 PoC 门槛不通过就停下来向用户报告）。

### Task 24: PoC-8 —— 跨仓边的 ID 逐字对齐（fixture 半边 + OpenCourt/OpenLaw 真仓半边）

**Files:**
- Create: `internal/scanner/poc8_test.go`
- Create: `docs/superpowers/poc/2026-10-30-poc8-cross-repo-real.md`
- Test: 同 Create 第一条（fixture 半边就是测试，进 `go test ./...`）

**Interfaces:**
- Consumes: T5 的 `graph.Merge`（用它做「合并后跨仓边两端都在图里」的断言）、既有 `scanner.Run`、T18 的 `loadScopedGraph`（CLI 半边已经在 T18 验过，这里不重复）。
- Produces: 无新产品 API。产出是**一份带实测数字的 findings**，以及 `TestPoC8FixtureCrossEdgeIDsAlign` / `TestPoC8RealPair`（后者 env 门、默认 skip）。

spec §12 的失败分支逐字照抄进任务，不要执行时才现编：

> `replace` 不生效（proxy 副本对齐失败）→ 文档化 `go.work` 指引；ID 结构性错位才回炉 ID 方案。

- [ ] **Step 1: 写 fixture 半边的失败测试**

`internal/scanner/poc8_test.go`：

```go
package scanner

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/futuremeng/OpenArchitect/internal/core/graph"
	"github.com/futuremeng/OpenArchitect/internal/core/model"
	"github.com/futuremeng/OpenArchitect/internal/core/store"
)

// 助手一律复用 T11 已落在同包 testutil_test.go 里的版本：
// newScanTestDB(t)（不注册 Close，调用点自己 defer db.Close()）、
// fixturePath(t, name)。这两个是本包唯一的开库/取 fixture 入口，
// 不要再造 openTempStore / scanFixtureInto 之类的同义函数。

// TestPoC8FixtureCrossEdgeIDsAlign 是 DoD-1 的机械化：两仓先后扫进同一个库，
// 合并图里必须出现 app→dep 的跨仓边，而且这条边的两个端点 id 必须逐字出现在
// 各自单仓快照里（"逐字一致"是 spec §7 的原文，也是合并能不能成立的前提）。
func TestPoC8FixtureCrossEdgeIDsAlign(t *testing.T) {
	ctx := context.Background()
	db := newScanTestDB(t)
	defer db.Close()

	depReport, err := Run(ctx, db, fixturePath(t, "crossmod-dep"))
	if err != nil {
		t.Fatalf("scan dep: %v", err)
	}
	appReport, err := Run(ctx, db, fixturePath(t, "crossmod-app"))
	if err != nil {
		t.Fatalf("scan app: %v", err)
	}

	// 各自单仓的 id 集合（合并之后再来取，就证不了"逐字"了）。
	depIDs := elementIDs(ctx, t, db, depReport.SnapshotID)
	appIDs := elementIDs(ctx, t, db, appReport.SnapshotID)

	perRepo := []*graph.Graph{
		graphRepo(ctx, t, db, depReport.SnapshotID),
		graphRepo(ctx, t, db, appReport.SnapshotID),
	}
	merged, warnings := graph.Merge(perRepo)
	if len(warnings) != 0 {
		t.Errorf("两个不同模块的仓不该产生归属警告：%v", warnings)
	}

	var cross int
	for _, rel := range merged.Relationships() {
		fromApp := strings.HasPrefix(rel.FromID, "cmp:crossmod.example/app")
		toDep := strings.HasPrefix(rel.ToID, "cmp:crossmod.example/dep")
		if !fromApp || !toDep {
			continue
		}
		cross++
		// 逐字对齐：两个端点都在各自的单仓集合里，且合并图里都能查到。
		if _, ok := appIDs[rel.FromID]; !ok {
			t.Errorf("跨仓边起点 %q 不在 app 快照里", rel.FromID)
		}
		if _, ok := depIDs[rel.ToID]; !ok {
			t.Errorf("跨仓边终点 %q 不在 dep 快照里", rel.ToID)
		}
		if _, ok := merged.Element(rel.ToID); !ok {
			t.Errorf("终点 %q 在并集里缺席（Merge 会丢端点不全的边）", rel.ToID)
		}
	}
	if cross == 0 {
		t.Fatal("PoC-8 失败：合并图里没有一条 app→dep 的跨仓边")
	}
	t.Logf("PoC-8 fixture: %d 条跨仓边，端点 id 与单仓快照逐字一致", cross)
}

func elementIDs(ctx context.Context, t *testing.T, db *store.DB,
	snapshotID string) map[string]bool {
	t.Helper()
	elts, err := db.Elements(ctx, snapshotID)
	if err != nil {
		t.Fatal(err)
	}
	out := make(map[string]bool, len(elts))
	for _, e := range elts {
		out[e.ID] = true
	}
	return out
}

func graphRepo(ctx context.Context, t *testing.T, db *store.DB,
	snapshotID string) *graph.Graph {
	t.Helper()
	elts, err := db.Elements(ctx, snapshotID)
	if err != nil {
		t.Fatal(err)
	}
	rels, err := db.Relationships(ctx, snapshotID)
	if err != nil {
		t.Fatal(err)
	}
	return graph.Build(elts, rels)
}
```

助手全部来自 T11 落在同包的 `internal/scanner/testutil_test.go`：`newScanTestDB(t)`（不注册 Close，所以每条测试自己 `defer db.Close()`）、`fixturePath(t, name)`（转发到既有的 `fixtureRoot()`，`detector_test.go:10`，返回相对路径 `../../testdata/fixtures`——`go test ./internal/scanner/` 的 cwd 就是包目录，相对路径成立）、`fixtureCopy(t, name, dest)`（要改文件的测试用它，本任务的 PoC-8 只读 fixture，用 `fixturePath` 即可）。这三个名字就是最终名字，`openTempStore`/`scanFixtureInto` 这类同义写法一律不要出现。

- [ ] **Step 2: 跑测试确认失败/通过**

```sh
go test ./internal/scanner/ -run TestPoC8 -v
```

期望：如果 T5/T10 已把跨仓边与并集打通，这条测试**第一次就绿**。这时不要怀疑测试没测到东西——它的价值在下一步：把它当成真仓半边的判据模板。如果这里红，**先停下来修到绿**，别用真仓去 debug fixture（真仓噪声大得多）。

- [ ] **Step 3: 真仓半边的 env 门测试**

追加到同一个文件：

```go
// TestPoC8RealPair 是 spec §12 里「fixture 之外再用真仓对钉一遍」那半条门槛。
// 观察库记录过：OpenCourt/go.mod 带 require github.com/futuremeng/OpenLaw +
// replace … => ../OpenLaw（docs/superpowers/poc/2026-10-26-observation-corpus.md）。
// 真仓不进 CI（依赖用户机器上的目录），所以默认 skip。
//
// 跑法：
//
//	OAC_POC8_APP="$HOME/github/futuremeng/OpenCourt" \
//	OAC_POC8_DEP="$HOME/github/futuremeng/OpenLaw" \
//	  go test ./internal/scanner/ -run TestPoC8RealPair -v
func TestPoC8RealPair(t *testing.T) {
	appRoot, depRoot := os.Getenv("OAC_POC8_APP"), os.Getenv("OAC_POC8_DEP")
	if appRoot == "" || depRoot == "" {
		t.Skip("OAC_POC8_APP/OAC_POC8_DEP 未设置：这条门槛在验收时对真仓跑")
	}
	ctx := context.Background()
	db := newScanTestDB(t)
	defer db.Close()
	// 顺序有讲究：先扫被依赖方，让 app 那一次扫描时 dep 的指纹已经在库里
	// （增量路径会读它）。全量路径不依赖顺序，所以两种顺序都该得到同样的
	// 跨仓边——这一步顺手验了这条不变量。
	dep, err := Run(ctx, db, depRoot)
	if err != nil {
		t.Fatalf("scan dep: %v", err)
	}
	app, err := Run(ctx, db, appRoot)
	if err != nil {
		t.Fatalf("scan app: %v", err)
	}
	merged, _ := graph.Merge([]*graph.Graph{
		graphRepo(ctx, t, db, dep.SnapshotID),
		graphRepo(ctx, t, db, app.SnapshotID),
	})
	var cross int
	for _, rel := range merged.Relationships() {
		from, to := model.PkgPathOf(rel.FromID), model.PkgPathOf(rel.ToID)
		if from == "" || to == "" {
			continue
		}
		// 跨仓 = 两端点落在不同的 module path 下。model.Relationship 只有
		// {FromID, ToID, Type, Weight}，没有「按仓归类」的助手，所以这里用
		// 前缀比较——不要为了一条测试给 model 加字段。
		fromTag, toTag := repoTag(from, app.ModulePath, dep.ModulePath), repoTag(to, app.ModulePath, dep.ModulePath)
		if fromTag != toTag && fromTag != "third-party" && toTag != "third-party" {
			cross++
		}
	}
	t.Logf("真仓对：%s %d 元素 / %s %d 元素，合并 %d 元素 %d 关系，跨仓候选边 %d 条",
		dep.ModulePath, dep.Elements, app.ModulePath, app.Elements,
		len(merged.Elements()), len(merged.Relationships()), cross)
	if cross == 0 {
		t.Error("PoC-8 真仓半边：合并图里没有任何跨仓边，按失败分支走 go.work 指引")
	}
}
```

同一个文件末尾加这条助手（`model.Relationship` 只有 `{FromID, ToID, Type, Weight}`，没有「按仓归类」的字段，所以按 module path 前缀归类；两个 module path 不相交，先匹配到的算）：

```go
func repoTag(pkgPath, appModule, depModule string) string {
	switch {
	case strings.HasPrefix(pkgPath, appModule):
		return "app"
	case strings.HasPrefix(pkgPath, depModule):
		return "dep"
	default:
		return "third-party"
	}
}
```

> 第三方与 stdlib 端点归到 `"third-party"`：两端都是它时相等不计入 `cross`，一端是它时也不算跨仓边（那是「本仓依赖外部库」，切片 5 的统计口径），所以判定里那两条 `!= "third-party"` 是必需的。两个 module path 实测不相交（`github.com/futuremeng/OpenCourt` 与 `…/OpenLaw`），先匹配到的算，不需要最长前缀。

- [ ] **Step 4: 真仓门槛执行与失败分支**

```sh
export PATH="/usr/local/go/bin:$PATH" GOPROXY=https://goproxy.cn,direct CGO_ENABLED=0
OAC_POC8_APP="$HOME/github/futuremeng/OpenCourt" \
OAC_POC8_DEP="$HOME/github/futuremeng/OpenLaw" \
  go test ./internal/scanner/ -run TestPoC8RealPair -v
```

判据与分支（按顺序走，每步记下实测数字）：

1. **跨仓边 ≥1 条，且端点 id 在两侧快照里逐字出现** → PoC-8 通过。
2. **0 条** → 先分清是 `replace` 没生效还是 ID 方案错了。在 OpenCourt 目录下跑：

   ```sh
   go list -m -f '{{.Path}} => {{.Replace}}' github.com/futuremeng/OpenLaw
   ```

   - 输出的 `Replace` 不是 `github.com/futuremeng/OpenLaw ../OpenLaw` → **`replace` 不生效**，走 spec 的失败分支：在 findings 里写清 `go.work` 指引（`go work init OpenCourt OpenLaw && go work use . ../OpenLaw`），并记录这条路径下一次扫描是否产生跨仓边。**这不是回炉 ID 方案的条件。**
   - `replace` 生效但跨仓边仍为 0 → 检查 `arch_relationships` 里 `to_id` 前缀是 `github.com/futuremeng/OpenLaw` 的行数（切片 1 的实测是 **0**，根因 `ownPackage` 过滤，见观察库文档）。若仍为 0，说明 T9 的 `PackageSource`/悬空端点路径没落地 → 这是**实现缺陷**，回到 T10/T13 修，不算 PoC-8 失败。
   - 悬空边有行数但 id 与 OpenLaw 自己快照里的 id **结构性错位**（比如一方带 `/v2` 后缀、一方大小写不同）→ 这才是 spec 说的「回炉 ID 方案」，**停下来向用户报告**，带上两个 id 的逐字对比。

- [ ] **Step 5: 写 findings**

`docs/superpowers/poc/2026-10-30-poc8-cross-repo-real.md`，三段结构（结论 / 数字 / 影响），**实测与推导分开写**：

```markdown
# PoC-8：跨仓边与 ID 逐字对齐（2026-10-30）

## 结论
（通过 / 走 go.work 指引 / 回炉 ID 方案——三选一并给出判据行）

## 数字（实测）
- fixture crossmod-app → crossmod-dep：跨仓边 N 条，端点逐字一致 ✓
- OpenCourt：元素 X，关系 Y；OpenLaw：元素 Z，关系 W（观察库 2026-10-26 记的是
  68/146 与 52/118，本轮数字变了也要如实写下）
- 合并图：元素 = X+Z−重叠，跨仓边 N 条
- `go list -m` 的 replace 输出原文

## 影响
（对 T5 Merge 的归属规则、对 T13 复用键、对 README 的 go.work 提示各写了什么）
```

- [ ] **Step 6: 全绿 + 提交**

```sh
go vet ./... && go test ./...
git add internal/scanner/poc8_test.go docs/superpowers/poc/2026-10-30-poc8-cross-repo-real.md
git commit -m "test: PoC-8 跨仓边门槛（fixture 断言 + OpenCourt/OpenLaw 真仓实测）"
```

---

### Task 25: PoC-11 —— 合并图 ≥600 节点首帧 ≤2 s（探针 + 真库合并场景）

**Files:**
- Create: `internal/ui/mergedprobe_test.go`（`//go:build probe`，一次性尖峰，不进 `go test ./...`）
- Modify: `internal/ui/dodprobe_test.go`（新增 `TestDoDProbeMergedRealDB`）
- Create: `docs/superpowers/poc/2026-10-30-poc11-merged-firstframe.md`

**Interfaces:**
- Consumes: T3 的 `DB.CommitScan`（往临时库灌合成的多仓快照）、T5 的 `graph.Merge`、既有的 `App.Reload`/`graphPage`/`flow.State.Ready`、`ui.NewTester`、`internal/ui/app_test.go` 的 `newTestApp`。
- Produces: 无产品 API。产出是 `TestMergedProbe`（env 门 + build tag，默认不跑）+ findings 里的四个规模台阶数字。

**为什么是包内测试而不是 `tools/mergedprobe` 命令**：`internal/ui` 是 `package ui`，没有 `internal/ui/app` 子包；`App.graphPage`、`App.flowState`、`newTestApp` 全是包内标识符（实测 `page_graph.go:14`、`app_test.go:19`）。外部 main 要驱动它，只能给产品包加一层 `GraphPageForProbe`/`FlowReadyForProbe` 的导出面——为一次性探针永久扩产品 API，是本末倒置。放包内测试则 `newTestApp`、`graphPage`、`flowState` 直接可用。PoC-4 的 `tools/flowprobe` 保持原样，它量的输入是内存里的 `flow.Data`；PoC-11 要量的是**切片 2 新加的那一段**——`store` 读 N 个快照 → `Merge` → `Aggregate` → `flowData` → 首帧 Ready。绕开存储与合并的读数，等于没测新东西。

**ghost 也在门槛里**：对比模式会把节点数往上抬（T20 定稿 2：幽灵参与布局），所以探针吃 `OAC_PROBE_GHOSTS=1` 把每仓元素数按 +30% 再灌一遍，量同一扇门。

- [ ] **Step 1: 写探针测试**

`internal/ui/mergedprobe_test.go`：

```go
//go:build probe

// PoC-11 的一次性探针：往临时库灌 N 个仓、总计 ≥600 个元素，然后走产品路径
// （store.Open → NewApp → Reload → graphPage 首帧）量开库 / 读图 / 布局各段
// 耗时。跑法：
//
//	OAC_PROBE_REPOS=4 OAC_PROBE_PER=160 \
//	  go test ./internal/ui/ -tags probe -run TestMergedProbe -v
//
// 不是产品代码，和 tools/flowprobe 同类；写侧必须走 DB.CommitScan（探针要测的
// 就是真实读图路径，写侧绕过去等于给读侧编了一条不存在的行形状）。
package ui

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
	"github.com/futuremeng/OpenArchitect/internal/core/store"
)

func probeInt(t *testing.T, key string, def int) int {
	t.Helper()
	raw, ok := os.LookupEnv(key)
	if !ok {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		t.Fatalf("%s = %q: %v", key, raw, err)
	}
	return v
}

func TestMergedProbe(t *testing.T) {
	repos := probeInt(t, "OAC_PROBE_REPOS", 4)
	per := probeInt(t, "OAC_PROBE_PER", 160)
	ghosts := os.Getenv("OAC_PROBE_GHOSTS") == "1"
	frames := probeInt(t, "OAC_PROBE_FRAMES", 3)

	ctx := context.Background()
	t0 := time.Now()
	db := newTestApp(t).db // 临时库 + t.Cleanup(Close)，与 dodprobe_test.go:48 同形
	opened := time.Since(t0)

	elts, rels := seedMerged(ctx, db, repos, per, ghosts)

	a := NewApp(db)
	t1 := time.Now()
	if err := a.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	reloaded := time.Since(t1)

	t2 := time.Now()
	tt := ui.NewTester(func(c *ui.Context) { a.graphPage(c) }, 1180, 780)
	tt.Frame()
	first := time.Since(t2)
	if !a.flowState.Ready() {
		t.Fatalf("首帧未 Ready（%.2f s）、元素 %d", first.Seconds(), elts)
	}
	var avg time.Duration
	for i := 0; i < frames; i++ {
		s := time.Now()
		tt.Frame()
		avg += time.Since(s)
	}
	avg /= time.Duration(frames)

	t.Logf("repos=%d per=%d ghosts=%v → 元素 %d 关系 %d", repos, per, ghosts, elts, rels)
	t.Logf("开库 %.0f ms / Reload %.0f ms / 首帧 %.0f ms / 后续帧均 %.2f ms",
		opened.Milliseconds(), reloaded.Milliseconds(), first.Milliseconds(),
		float64(avg.Microseconds())/1000)
	// 门槛：DoD-9 的「首帧 ≤2 s」含 Reload（spec §4 的原文是「打开合并库到
	// 看见图」，开库那一段由 opened 单独记录，不进门槛——它是切片 1 已量过的
	// PoC-2 范围）。
	if elapsed := reloaded + first; elapsed > 2*time.Second {
		t.Errorf("DoD-9 超门槛：Reload+首帧 %.0f ms > 2000 ms（元素 %d）",
			elapsed.Milliseconds(), elts)
	}
}
```

`newTestApp(t).db` 直接复用 `app_test.go:19` 的助手（它已经 `t.Cleanup(db.Close)`，所以这里不写 `defer`）；探针和单测共用同一条开库路径，量出来的数字才代表用户那一次。

`seedMerged`（生成 N 仓、每仓 `per` 个元素 + 每元素一条 `contains` + 每三个元素一条 `calls`，跨仓 `calls` 每仓 5 条），追加在同一个文件里：

```go
// seedMerged 灌库并返回 (元素数, 关系数)。元素 id 形如
// cmp:example.com/r<k>/pkg<i>.Type<j>，包路径可被 model.PkgPathOf 解析，
// 因为 Aggregate 依赖它。
func seedMerged(ctx context.Context, db *store.DB, repos, per int, ghosts bool) (int, int) {
	var elts, rels int
	for k := 0; k < repos; k++ {
		repoID := fmt.Sprintf("repo:/probe/r%d", k)
		if err := db.UpsertRepo(ctx, model.Repo{ID: repoID,
			Name: fmt.Sprintf("r%d", k), LocalPath: "/probe/r" + strconv.Itoa(k), Lang: "go"}); err != nil {
			panic(err)
		}
		n := per
		if ghosts {
			n = per * 13 / 10
		}
		e := make([]model.Element, 0, n+1)
		r := make([]model.Relationship, 0, n*2)
		sys := model.SystemID(fmt.Sprintf("example.com/r%d", k))
		e = append(e, model.Element{ID: sys, Type: model.TypeSystem,
			Name: fmt.Sprintf("r%d", k), RepoID: repoID})
		for i := 0; i < n; i++ {
			id := fmt.Sprintf("cmp:example.com/r%d/pkg%d.Type%d", k, i/8, i)
			e = append(e, model.Element{ID: id, Type: model.TypeComponent,
				Name: fmt.Sprintf("Type%d", i), RepoID: repoID,
				FilePath: fmt.Sprintf("pkg%d/x.go", i/8), Methods: []string{"Do"}})
			r = append(r, model.Relationship{FromID: sys, ToID: id,
				Type: model.RelContains, Weight: 1})
			if i%3 == 0 {
				r = append(r, model.Relationship{FromID: id,
					ToID: fmt.Sprintf("cmp:example.com/r%d/pkg%d.Type%d", k, i/8, i-1),
					Type: model.RelCalls, Weight: 2})
			}
		}
		// 每仓 5 条指向下一仓的跨仓 calls：合并视图的真实形状。
		for j := 0; j < 5 && k+1 < repos; j++ {
			r = append(r, model.Relationship{
				FromID: fmt.Sprintf("cmp:example.com/r%d/pkg0.Type%d", k, j),
				ToID:   fmt.Sprintf("cmp:example.com/r%d/pkg0.Type%d", k+1, j),
				Type:   model.RelCalls, Weight: 1})
		}
		// KeepSnapshots 给 1：每仓只写一版快照。给 0 的话裁剪子查询是
		// `LIMIT 0`（空集），连刚写的这行一起删掉，探针下一步就读空库。
		err := db.CommitScan(ctx, store.ScanCommit{Snapshot: model.ArchitectureSnapshot{
			ID: fmt.Sprintf("s%019d", k), RepoID: repoID},
			Elements: e, Relationships: r, KeepSnapshots: 1})
		if err != nil {
			panic(err)
		}
		elts += len(e)
		rels += len(r)
	}
	return elts, rels
}
```

- [ ] **Step 2: 跑探针，默认形状（4 仓 × 160，644 个元素）**

```sh
go test ./internal/ui/ -tags probe -run TestMergedProbe -v
```

期望按 `seedMerged` 的算术核对：每仓 `1 个 sys + 160 个 cmp = 161` → **元素 644**；每仓 `160 条 contains + 54 条仓内 calls`（`i%3==0`，i=0..159）`+ 5 条跨仓 calls`（最后一仓没有）→ **关系 871**。打印行必须逐字含这两个数（对不上就是 seed 变了，先修 seed 再读数）。`Reload + 首帧 ≤ 2000 ms`，把这一行的**原文**贴进 findings。

一个已知的数差：每仓 `i==0` 那条 calls 指向不存在的 `Type-1`，`graph.Build` 会在读侧丢掉它（`Build` 只收端点齐的边），所以 `a.graph.Relationships()` 是 `871 − 4 = 867`。**这不是 bug，正是 PoC-11 要的那条真实路径**——别为了让两个数相等去改 seed。

- [ ] **Step 3: 扫一遍规模台阶**

```sh
for per in 50 160 400; do
  OAC_PROBE_PER=$per go test ./internal/ui/ -tags probe -run TestMergedProbe -v
done
OAC_PROBE_PER=400 OAC_PROBE_GHOSTS=1 go test ./internal/ui/ -tags probe -run TestMergedProbe -v
```

记录四组数字（`per=50` → 204 元素、`160` → 644、`400` → 1604，再加 `400 + ghosts` → 4×(520+1) = **2084**）。这是 PoC-11 的观测曲线，也是 README 里「合并视图建议的仓数」那条的唯一依据。DoD-9 的门槛只钉在 644 那一档（spec §4 的原文是 ≥600）；其余三档是曲线，不是门禁。

- [ ] **Step 4: 真库合并场景测试**

`internal/ui/dodprobe_test.go` 追加（沿用同一条 `OAC_DOD_DB` env 门，`TestDoDProbeRealDB` 已经解释过默认 skip 的理由）：

```go
// TestDoDProbeMergedRealDB 是 DoD-9 的另一半：真库里多仓同时选中时的首帧与
// 计数守恒。跨仓边不在这里断言——观察库里的三个 Go 仓（onevra / OpenCourt /
// OpenLaw）与 OpenArchitect 之间没有互相 import（观察库文档明确记过这点），
// 跨仓边的真仓断言归 PoC-8。
func TestDoDProbeMergedRealDB(t *testing.T) {
	dbPath := os.Getenv("OAC_DOD_DB")
	if dbPath == "" {
		t.Skip("OAC_DOD_DB 未设置：这条探针只在验收时对真库跑")
	}
	ctx := context.Background()
	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	a := NewApp(db)
	if err := a.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	if len(a.selectedRepos) < 2 {
		t.Skipf("真库里只有 %d 个带快照的仓，凑不出合并场景", len(a.selectedRepos))
	}
	// 计数守恒：并集 = Σ 各仓 − 重叠 id。重叠在这里应为 0（不同模块路径），
	// 但必须量出来写进 findings，因为它是 Merge 归属规则的第一手证据。
	var sum int
	seen := map[string]bool{}
	var overlap int
	for repoID, snapshotID := range a.repoLatest {
		if snapshotID == "" || !a.selectedRepos[repoID] {
			continue
		}
		elts, err := db.Elements(ctx, snapshotID)
		if err != nil {
			t.Fatal(err)
		}
		sum += len(elts)
		for _, e := range elts {
			if seen[e.ID] {
				overlap++
			}
			seen[e.ID] = true
		}
	}
	if got := len(a.graph.Elements()); got != sum-overlap {
		t.Errorf("并集计数 %d ≠ Σ各仓 %d − 重叠 %d", got, sum, overlap)
	}
	t.Logf("合并：%d 个仓，Σ %d，重叠 %d，并集 %d，关系 %d",
		len(a.selectedRepos), sum, overlap, len(a.graph.Elements()),
		len(a.graph.Relationships()))

	t0 := time.Now()
	a.SetShowComponents(true)
	tt := ui.NewTester(func(c *ui.Context) { a.graphPage(c) }, 1180, 780)
	tt.Frame()
	if !a.flowState.Ready() {
		t.Fatalf("DoD-9 首帧未 Ready（%.2f s）", time.Since(t0).Seconds())
	}
	first := time.Since(t0)
	if first > 2*time.Second {
		t.Errorf("DoD-9 超门槛：合并首帧 %.2f s > 2 s", first.Seconds())
	}
	t.Logf("DoD-9: 合并 %d 节点首帧 %.2f s（门槛 2 s）", len(a.view.Elements()), first.Seconds())
}
```

> `got0(...)` 是笔误，删掉——`t.Logf` 里直接写 `len(a.graph.Elements())`。留着这种套壳只会让下一个读者去找那个不存在的函数。

- [ ] **Step 5: 门槛判定与失败分支**

spec §12 的 PoC-11 分支，按顺序执行、每一步都记录实测：

1. **首帧 ≤2 s** → 通过，写 findings 收尾。
2. **超门槛** → 第一档降级（spec 原文）：**默认仅 Container 层 + 过滤器收窄**。代码落点已经存在，不需要新写：`showComponents` 本来就是 false（`state.go:123`、`flowData` 的折叠分支 `flow_data.go:40-42`），`repoPicker` 的多选过滤器也是既有的（`page_graph.go:108`）。所以第一档的动作是**把 `--tags probe` 用 `SetShowComponents(false)` 重跑一遍**，量折叠后的首帧。若折叠后达标，findings 写「600 节点元素层不达标，Container 层达标，默认层保持折叠」，并把元素层的实测数字留在文档里不要抹掉。
3. **折叠后仍崩** → spec 的逃生通道：切片 1 预留的 WebView + React Flow 替换，**只换 `internal/ui`**（`core`/`scanner`/`store` 与 CLI 全部原样，这是 §5.1 分层换来的退路）。这是一次架构级重做，**停下来向用户报告并等决定**，不要在本任务里启动它。

- [ ] **Step 6: findings + 提交**

`docs/superpowers/poc/2026-10-30-poc11-merged-firstframe.md`：规模台阶四组数字（原文）、真库合并的 Σ/重叠/并集三个数、判定走了第几档、以及一句诚实的边界——**headless `ui.NewTester` 量的是布局+逻辑帧，不含真实窗口的 GPU 光栅**，这条与切片 1 findings 里「是否读得清没有截图证据」是同一条限制的两个面。

```sh
go vet ./... && go test ./...
git add internal/ui/mergedprobe_test.go internal/ui/dodprobe_test.go docs/superpowers/poc/2026-10-30-poc11-merged-firstframe.md
git commit -m "test: PoC-11 合并图首帧探针（-tags probe）与真库合并计数守恒"
```

---

### Task 26: CI 守卫复核、`-race` 一遍与 DoD 收尾

**Files:**
- Modify: `tools/ciguard/ci_guard_test.go`
- Modify: `internal/ui/shell.go`（只在 runbook 要求时改）
- Create: `docs/superpowers/poc/2026-10-30-slice2-findings.md` 的「DoD 验收」章节（该文件由 T13 创建，这里补最后一节）
- Modify: `README.md`（若前面各任务没补；见 Step 6）

**Interfaces:**
- Consumes: 全部前序任务。
- Produces: 无新 API。产出是一份**每条 DoD 都有命令与实测输出**的验收记录，外加 ciguard 的一条新断言。

- [ ] **Step 1: ciguard 补一条「平价测试不许被跳过」的断言**

DoD-3 的平价测试是切片 2 的地基，而它是一条 `t.Skip` 就能静默消失的普通测试。ciguard 已经在做「断言 YAML 文本本身」这一类事（`ci_guard_test.go:46` 起），同一条思路用在这里：

```go
// TestIncrementalParityTestIsNotSkippable: DoD-3 的平价测试是 CI 必跑项，
// 而它在 `go test ./...` 里跑——所以它不许含 t.Skip / testing.Short 的退路口。
// 这里读测试文件原文而不是执行它：执行只能证明"今天跑了"，读文本才钉得住
// "明天有人加了一句 skip 也会红"。
func TestIncrementalParityTestIsNotSkippable(t *testing.T) {
	raw, err := os.ReadFile("../../cmd/oac/incremental_parity_test.go")
	if err != nil {
		t.Fatalf("read parity test: %v（T13 还没落地，或文件改名了）", err)
	}
	text := string(raw)
	for _, banned := range []string{"t.Skip(", "testing.Short(", "t.Setenv(\"OAC_"} {
		if strings.Contains(text, banned) {
			t.Errorf("平价测试里有退路口 %q", banned)
		}
	}
	if !strings.Contains(text, "func TestIncrementalParityWithFullScan") {
		t.Error("平价测试改名了：DoD-3 的 CI 必跑项必须保持这个名，findings 与 spec 都引用它")
	}
}
```

- [ ] **Step 2: 跑守卫确认失败**

```sh
go test ./tools/ciguard/ -run TestIncrementalParityTestIsNotSkippable -v
```

期望：PASS（T13 落地时它就该是绿的）。**如果这条测试一写出来就 PASS，它仍是有效的**——它守的是未来的改动，不是今天的 bug。这是它与本计划其它「先红后绿」测试的唯一区别，写在这里免得执行者以为写错了而去「制造」一个红。

- [ ] **Step 3: CI 三条守卫复核**

切片 2 动了包结构吗？答案是没动：`internal/core/{model,store,graph}`、`internal/scanner{,/language}`、`internal/config`、`internal/ui`、`cmd/oac` 都在既有 glob 覆盖里（`./internal/core/...`、`./internal/scanner/...`）。但 **T5 让 `internal/core/graph` 变成 App 与 CLI 共用的并集入口**，`internal/ui` 也新增了对 `graph.DiffSnapshots` 的依赖，这两件事都不触碰守卫的三条断言。要复核的是新依赖有没有把 UI 栈漏进 core/scanner：

```sh
export PATH="/usr/local/go/bin:$PATH" CGO_ENABLED=0
go list -f '{{join .Imports "\n"}}' ./internal/core/... ./internal/scanner/... \
  | grep -x -E 'github.com/egoist/mygo|github.com/egoist/mygo/ui|github.com/oneo-me/onevra[^ ]*' \
  && echo "R1 违规（上面的行就是罪魁）" || echo "R1 绿"
go list -f '{{join .Imports "\n"}}' ./internal/scanner/... ./internal/core/model/... ./internal/core/graph/... \
  | grep -x 'github.com/egoist/mygo/plugins/sqlite' && echo "驱动泄漏" || echo "反向守卫 绿"
go list -f '{{join .Imports "\n"}}' ./cmd/oac \
  | grep -x -E 'github.com/egoist/mygo|github.com/egoist/mygo/ui|github.com/oneo-me/onevra[^ ]*' \
  && echo "oac 混进 UI" || echo "DoD-4 绿"
```

三条都必须打印「绿」。任何一条红：**先停**，把违规 import 的来源包与被引的 UI 栈包写进 findings，再决定是移代码还是修守卫——**改守卫的匹配列表是最后选项，且必须写清理由**（切片 1 已经有一次「-deps → 直接 import」的定稿修正，那是有证据的例外，不是惯例）。

- [ ] **Step 4: `-race` 一遍（CGO_ENABLED=1，全局约束的唯一例外）**

切片 2 新增了三条后台路径值得竞态检测：T23 的新鲜度检查（goroutine + post）、T23 的扫描队列出队（`finishScan` 主线程写 `scanQueue`/`status`）、T13 的复用（无 goroutine，但 `scanner.Run` 与 `store` 的边界变了）。

```sh
export PATH="/usr/local/go/bin:$PATH" GOPROXY=https://goproxy.cn,direct CGO_ENABLED=1
go test -race ./internal/ui/ ./internal/scanner/... ./internal/core/... ./cmd/oac/
```

期望：0 个 `DATA RACE`。红了不要 `t.Setenv` 关掉、不要加 mutex 遮羞：**先把两个栈贴进 findings 再判断**——按切片 1 的纪律 1，凡是 goroutine 写 App 字段的路径都必须经 `a.post`；`-race` 报出这条，说明有个新写点绕过了 post，那是接线错误，修接线。

- [ ] **Step 5: DoD 九条的验收 runbook**

逐条跑、逐条把命令与输出原文写进 findings 的「DoD 验收」章节。命令一律带绝对 PATH 前缀：

| DoD | 命令 | 判据 |
|---|---|---|
| 1 跨仓边 + id 逐字 | `go test ./internal/scanner/ -run TestPoC8Fixture -v`；再跑 T24 Step 4 的真仓 env 门 | 两条都绿，findings 有真仓数字 |
| 2 过滤器三种切法计数一致 | `go test ./internal/ui/ -run TestReload -v` + `oac graph --repo <A> --db $DB` 与 GUI 选中 A 的状态行逐字对比 | 元素数等于各自快照；GUI/CLI 同源由 DoD-4 的读库路径保证 |
| 3 平价 + 复用 ≥50% | `go test ./cmd/oac/ -run TestIncrementalParity -v`；日志里 `reused=`/`parsed=` | 逐字节相等 **且** reused/(reused+parsed) ≥ 0.5 |
| 4 `oac diff` 与 GUI 同数据 | `oac diff <repoB> --from s1 --json` 与 `TestSetCompareMapsNodeMarks` 的三份计数 | 数字相同，着色由同一 `marks` 表出 |
| 5 `oac history` 全序列 | `oac history <element-id> --json` + `TestSelectLoadsHistory` | 事件序列与脚本化改动吻合，段文案含「近 50 次扫描内」 |
| 6 `oac verify` 两态 | `oac verify --db $DB <repo>` → 改一个文件 → 再跑 | 第一次 `clean`，第二次 `stale` 且清单里有那个文件 |
| 7 `oac rank` | `oac rank --by dependents --limit 10 --db $DB --all` | 与测试预算排序一致 |
| 8 minimap | `go test ./internal/ui/ -run 'TestPageGraph\|TestFlow' -v`。点击→镜头的行为在 fork 里已被测（`components/flow/flow_test.go:199,264,306,338,439` 四段 NavigationMap 用例），本仓库这一侧的可测断言只有**接线**：`page_graph_test.go:23` 钉住 `opts.Navigation == flow.NavigationMap` | 接线未回归；「点阵渲染 + 点击 Reveal」的观感进 Step 6 的人眼清单（DoD-8 原文的 headless 可测部分由上游测试结清，这里不重复实现，也不假装测了） |
| 9 ≥600 首帧 ≤2 s + CI 三条守卫 + vet/test/-race | T25 Step 2-3 + 本任务 Step 3-4 + `go vet ./... && go test ./...` | 全绿，findings 有四个规模台阶 |

- [ ] **Step 6: 人眼项与 README**

findings 里单列一节「需要人眼确认（本会话无截图能力）」，三条，每条写明「代码依据」与「待看什么」：
1. 对比着色（T19 定稿：`removed` 用 `TextMuted` + 虚线，spec 未指定其色）——开窗口切到对比模式，看三种描边是否可分辨、幽灵节点是否读得出「已消失」。
2. 合并视图 600 节点的可读性（切片 1 同样只量了帧时间没量可读性）。
3. 仓库页新鲜度徽标从「未检查」变「新鲜」的那一次刷新是否看得见。

README 若缺切片 2 的四条新命令（`oac repo` / `diff` / `history` / `rank` / `verify`、`--all` / `--repo` / `--json`、增量默认与 `--full`、`go.work` 提示），补上——出处分别是 T15-T18、T24。检查 README 里切片 1 那句「已知限制」是否还被切片 2 的落地事实支撑。

- [ ] **Step 7: 提交**

```sh
go vet ./... && go test ./...
git add tools/ciguard/ci_guard_test.go docs/superpowers/poc/2026-10-30-slice2-findings.md README.md
git commit -m "ci: 平价测试防跳过守卫 + 切片 2 DoD 验收记录"
```

**收尾不在本计划授权范围内**：合并到 `main`、`git push`、删除 `.worktrees/slice1-flow` 与它的 SDD 工作目录，都需要用户点头。切片 1 的人眼清单（`docs/superpowers/poc/2026-10-19-slice1-findings.md` 的最后一节）至今未跑，与上面 Step 6 三条并成一次窗口验收最省时间——提出来，由用户决定何时。

---

## 不在本计划里（延后，别当成漏了）

评审时会被问到，先写在这里：

| 项 | 出处 | 为什么不在切片 2 |
|---|---|---|
| 相机与层级的跨会话记忆（`view.graph.<repoID>` 持久化） | 切片 1 复盘 G5 | spec §10 第 4 条说「沿用切片 1 机制」，切片 1 的机制就是**不持久化**（`flow.State` 只在内存）。要它得先定 key 的命名空间与失效规则，那是新决策 |
| 边的交互（点边看关系详情、边上工具提示） | 切片 1 复盘 G6 | `flow.Edge` 只有 `Source/Target/Label/Dashed/Width`，没有点击通道；要改的是 fork，不是本仓库 |
| 组合分组框（把同包成员画进一个可视盒子） | 切片 1 复盘 G8 | 与 `Aggregate` 的组节点是两种互斥的呈现思路，同时做会让包层既被折叠又被框住；先要一个呈现决定 |
| 打包、签名、发布 | 切片 6 | spec §3.2 明确排除 |
| 非 Go 语言适配器（Python/Rust/TS/PHP/C++/C#） | 切片 5 | 观察库的语言分布（17/20 非 Go）是切片 5 的动机数据，与本切片无关；`LanguageAdapter` 的注册表形状切片 1 已经留好 |
| Agent 子系统（自然语言问架构） | 切片 7 | spec §3.2 排除 |
| 跨仓对比（两个仓的快照互 diff） | 无 | T20 定稿 1 论证过它语义不成立；若将来要「A 仓 vs B 仓的同名类型」，那是新 spec |
| minimap 的视口矩形 | spec §2/§13 | fork 已提供 `Camera()/SetCamera()`，PoC-10 结清；上游 issue 不在本计划授权范围（「观测为主，不写任何外部系统」） |

---

## 执行顺序与门槛速查

```
Phase A 地基      T1 schema v2 → T2 model/读 → T3 CommitScan+历史+封顶 → T4 Snapshots/DeleteRepo/Repo
Phase B 图语义    T5 Merge → T6 DiffSnapshots → T7 RankDependents → T8 PackageSource → T9 指纹
Phase C 扫描器    T10 配置+哈希+Options → T11 verify → T12 历史事件 → T13 复用+平价+【PoC-9 门槛】
Phase D CLI      T14 subcommand/--json → T15 repo list|remove + scan --full → T16 oac diff + oac history
                  → T18 graph/impact 的 --all/--repo（scope.go）→ T17 oac rank + oac verify【编号 17 在 18 之后跑】
Phase E GUI      T19 卡片标记 → T20 对比状态机+幽灵 → T21 对比区 → T22 抽屉历史 → T23 仓库页+设置
Phase F 收尾      T24【PoC-8】 → T25【PoC-11】 → T26 CI/-race/DoD
```

三个 STOP 点：**PoC-9（T13 Step 7）**、**PoC-8（T24 Step 4 第三分支）**、**PoC-11（T25 Step 5 第三档）**。共同规则：门槛不通过 → 先按各自的预设分支走；分支落到「架构级重做」或「与 spec 结论相反」时，停下来向用户报告数字与选项，不许绕过门槛继续。

**T18 必须早于 T17**（编号只是文档排布，不是执行序）：`runRank` 调 T18 的 `loadScopedGraph`，`TestRankDefaultsToMergedScope` 调 T18 的 `scopedDB`。倒过来跑会在 T17 撞 `undefined`，届时才发现「rank 的作用域来自哪里」这件事不值一次返工。

依赖上的两个软顺序可以并行，其余硬顺序不能倒：T3 的 `CommitScan` 是 T12/T13/T20/T25 四处共同的写入口，必须早于它们；T6 的 `DiffSnapshots` 同时被 CLI（T16）、GUI（T20）和历史事件（T12）消费，是 T12 的前置；T19 早于 T20（后者调用前者的 `cardMark`），T21 早于 T22 只是文件相邻的便利，两者无依赖。



