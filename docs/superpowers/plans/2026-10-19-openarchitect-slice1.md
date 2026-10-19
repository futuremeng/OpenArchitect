# OpenArchitect 切片 1 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 扫描一个本地 Go 仓库，把架构元素与关系写进本地 SQLite，用 onevra `flow` 画出可交互的架构图，并支持单击查看详情、影响范围淡出与 headless CLI 复算。

**Architecture:** 三层。`internal/core`（model/store/graph）与 `internal/scanner`（语言探测 + Go adapter）不 import 任何渲染栈；`internal/ui` 用 mygo 原生 UI + onevra `sidebar`/`flow`/`command` 只做展示与事件；`cmd/openarchitect` 与 `cmd/oac` 只做装配。GUI 与 CLI 共用同一份 core，DoD-4 靠这条一致性验收。

**Tech Stack:** Go 1.27.2（darwin/arm64）、`github.com/egoist/mygo v0.5.5`、`github.com/egoist/mygo/plugins/sqlite`、`github.com/oneo-me/onevra/source v0.2026.12`（`shells/sidebar`、`components/flow`、`components/command`、`icons/lucide`、`theme`）、`golang.org/x/tools/go/packages`、`go/types`。

**Spec:** `docs/superpowers/specs/2026-10-19-openarchitect-slice1-design.md`

---

## 本计划对 spec 的三处定稿

spec §16 留了未决问题，计划必须给出答案才能写出无占位的代码。以下三条是**唯三**改动，其余全部照 spec 执行。

1. **U3 = 选项 (a)**：`arch_elements` 增列 `methods_json TEXT NOT NULL DEFAULT '[]'`，扫描时写入导出方法名；`model.Element` 因此有 7 个字段（spec §6.1 的 6 个 + `Methods []string`）。理由：选项 (b) 要在 UI 线程现读现解析，违反 §9.2「视图是纯函数」。（被 Task 5、6、7、9、19 依赖。）
2. **U1 已核实**：`sidebar.ShellOptions` **没有**页面注册 API，只有 `SetPage(page func(*ui.Context))`，而它的文档明确写着 "SetPage replaces the entire default page, **including sidebar controls**"。三页导航若走 `SetPage` 会连侧栏一起换掉，还会丢掉 `ContentTitle`/`LeftHeader`。所以页面切换用 `Content` 里对 `app.page` 的分支，`SetPage` 本切片不调用。（证据：`$GOMODCACHE/github.com/oneo-me/onevra/source@v0.2026.12/shells/sidebar/shell.go` 的 `SetPage` 文档注释与 `ShellOptions` 字段表。）
3. **`Graph.Merge` 签名补 error**：spec §8.1 写 `func (g *Graph) Merge(other *Graph)` 又要它返回 `ErrNotImplemented`，二者矛盾。定稿为 `func (g *Graph) Merge(other *Graph) error`。

另外一处澄清（不是改动）：偏差 11 说的 `Detect(root) (string, error)` 落在 `scanner.Detect`（返回语言名），`language.Adapter.Detect` 用 `(bool, error)`——适配器自己的 `Language()` 已经是那个 string，返回 string 是冗余。两个错误哨兵 `ErrUnsupportedLanguage`/`ErrUnreadable` 仍由 `scanner.Detect` 产出，§11 的测试意图不变。

---

## Global Constraints

每个任务的隐含需求都包含本节，数值逐字来自 spec。

- Go 版本下限 **1.27**（onevra 自己要求 ≥ 1.27.1）；开发机实测 `go1.27.2 darwin/arm64`。
- Go 不在 PATH 上：每条命令前都要 `export PATH="/usr/local/go/bin:$PATH"`。**本计划不修改用户的 `~/.zshrc`/`~/.zprofile`**；请用户自行追加。
- 模块代理固定 `export GOPROXY=https://goproxy.cn,direct`（`proxy.golang.org` 在此网络不可达，实测 i/o timeout）。
- 依赖精确 pin：`github.com/egoist/mygo v0.5.5`、`github.com/oneo-me/onevra/source v0.2026.12`。业务依赖**禁止 `@latest`**；只有脚手架 CLI 允许按版本 `@v0.5.5` 调用。
- 全程 `CGO_ENABLED=0`。禁止引入 `mattn/go-sqlite3`、`smacker/go-tree-sitter` 或任何需要 C 工具链的包（spec 偏差 6：其他语言一律 sidecar）。唯一例外：跑 `-race` 的那一遍要 `CGO_ENABLED=1`——竞态检测器本身要求 cgo，而依赖图里没有一行 cgo（`plugins/sqlite` 走 purego，`ffi_unix.go:8`），所以打开它只是让检测器能用，不会让 cgo 依赖混进产品构建。CI 里因此没有 race 步骤（Task 23），race 是本地闸门。
- **R1**：`internal/core/**` 与 `internal/scanner/**` 不得 import `github.com/egoist/mygo`、`github.com/egoist/mygo/ui`、`github.com/oneo-me/onevra/**`；唯一例外是 `github.com/egoist/mygo/plugins/sqlite`，且只允许出现在 `internal/core/store`。Task 23 用 CI 机械检查。
- 观测为主：不创建 Issue、不触发 CI、不调度 Agent、不写任何外部系统。
- 所有 SQL 取值只走 `?` 占位 + `Args`。唯一允许的字符串插值是 `PRAGMA user_version = %d`，其参数来自包内常量，不来自输入。
- 节点卡片尺寸恒定 **168 × 46 DIP**（spec §9.3，由 mygo `docs/ui/performance.md` 的 1000 label = 397 µs/帧推出来）。卡片内只放一个图标 + 两行文本，禁止按钮、进度条、富文本。
- ID 前缀只能用 `sys:` / `ctn:` / `cmp:`（仓库行用 `repo:`），且不含 snapshot id、不含自增值。
- 目录与文件名按 spec §5.2（含 `internal/config/config.go` 与 `internal/ui/state.go`）；本计划额外新增的只有：`internal/ui/flow_data.go`、`tools/parseprobe/`、`tools/flowprobe/`、`tools/ciguard/`、`docs/superpowers/poc/2026-10-19-slice1-findings.md`。`README.md` 只在 Task 24 的 Gatekeeper 分支出现。除 `cmd/openarchitect/main.go`（Task 22 从根目录搬过去）外，不新增 spec 没写的主包。
- 每个任务收尾必须：`go vet ./... && go test ./...` 全绿，然后 `git add` 明确列出的文件 + 一次提交，message 用 `feat:` / `test:` / `chore:` / `docs:` 前缀 + 中文正文。
- 与 golden 文件比对前**必须排序**（`go/packages` 的包遍历顺序不保证）。

### 一次性环境前置（在做任何任务之前执行一次）

```sh
export PATH="/usr/local/go/bin:$PATH"
export GOPROXY=https://goproxy.cn,direct
go version                      # 期望 go1.27.2 darwin/arm64
go env GOPROXY                  # 期望 https://goproxy.cn,direct
```

### PoC 结论记录

Phase 0 的四个任务就是 spec §12 的 PoC-1…PoC-5。**所有实测数字写进 `docs/superpowers/poc/2026-10-19-slice1-findings.md`**（Task 1 创建），每条固定三段：`结论` / `数字` / `影响`。

**任一门槛不通过就停下来向用户报告，不要绕过它继续做后面的任务**——spec §12 每条都配了失败分支，走哪条分支是用户的决定，不是执行者的。

判定门槛（逐字）：

| PoC | 门槛 | 不通过时 |
|---|---|---|
| 1 | `go tool mygo dev` 能起带侧栏的窗口 | 手搭骨架（`go mod init` + 照 onevra `source/mygo.json` 抄），报告后再继续 |
| 2 | native app 里 `sqlite.Open` 成功，且 `sqlite.LibraryPath()` 能找到库 | 换 `modernc.org/sqlite`；R1 的驱动例外随之作废。停下报告 |
| 3 | 真实仓库解析 ≤ 60 s 且峰值 ≤ 2 GB | 收窄 `LoadMode`/加包级预筛；把实测数字回填 spec §6.4 后再继续 |
| 4 | 600 节点 ≤ 16 ms/帧，且两次 `Advance` 后节点位置不漂移 | 走逃生通道切 WebView + React Flow（只换 `internal/ui`）。停下报告 |
| 5 | 单事务 2100 条 `Statement` 不失败且可接受地快 | 分块提交 + 自实现 Savepoint，并补「半成品快照不可见」规则。停下报告 |

---

## Phase 0 — 骨架与 PoC（Task 1–4）

spec §12："PoC 1/2 在任何实现工作之前跑完 —— 它们决定工程骨架形状。"

### Task 1: mygo 原生骨架 + 空侧栏窗口（PoC-1）

**Files:**
- Create: `go.mod`、`main.go`（暂放根目录，Task 22 移到 `cmd/openarchitect/`）
- Create: `mygo.json`
- Create: `docs/superpowers/poc/2026-10-19-slice1-findings.md`
- Modify: `go.sum`（由 go 生成）

**Interfaces:**
- Produces: 模块路径 `github.com/futuremeng/OpenArchitect`；`mygo.json` 里 `productName = "OpenArchitect"`（**必须是这个拼写**：GUI 的库目录是 `App.Path(PathUserData)`，macOS 下等于 `~/Library/Application Support/OpenArchitect`，Task 14 的 CLI 用 `os.UserConfigDir()/OpenArchitect` 对齐它）。

- [ ] **Step 1: 用官方脚手架建到临时目录**

```sh
cd /Users/futuremeng/github/futuremeng/OpenArchitect
go run github.com/egoist/mygo/cmd/mygo@v0.5.5 init -template native .tmp-scaffold
```

- [ ] **Step 2: 只搬配置，不要整目录覆盖仓库**

`go mod init` 要在仓库根建立我们自己的模块，脚手架的 `go.mod`（模块名是模板名）不能直接拿来用。

```sh
ls .tmp-scaffold                       # 先看清模板给了什么
cp .tmp-scaffold/mygo.json ./mygo.json
cat .tmp-scaffold/go.mod               # 若含 tool 指令 / tools.go，把同样内容搬进我们的 go.mod
```

若模板里有 `tools.go` 或 go.mod 里有 `tool ...` 行，记录其内容，Step 3 要复刻——这是 `go tool mygo` 能用的前提。

- [ ] **Step 3: 初始化模块并精确 pin 依赖**

```sh
go mod init github.com/futuremeng/OpenArchitect
go get github.com/egoist/mygo@v0.5.5
go get github.com/oneo-me/onevra/source@v0.2026.12
go get github.com/egoist/mygo/cmd/mygo@v0.5.5
go mod edit -tool=github.com/egoist/mygo/cmd/mygo
rm -rf .tmp-scaffold
gofmt -l . | grep -v '^docs/' || true
```

Expected: `gofmt` 无输出。`go build ./...` 这一步会因为没有 main 报错？不会——根目录还没有 `main.go`，`go build ./...` 在无 Go 文件时静默通过。

- [ ] **Step 4: 打开 mygo.json 核对产品名**

`productName`（或模板里等价的 `name`/`productName` 字段）必须是 `OpenArchitect`。库里没这个键就照着加，不要猜其他字段名——只改这一处。

- [ ] **Step 5: 写最小窗口**

```go
package main

import (
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/icons/lucide"
	"github.com/oneo-me/onevra/source/shells/sidebar"
	"github.com/oneo-me/onevra/source/theme"
)

func main() {
	// 名字决定 PathUserData = os.UserConfigDir() + App.Name()（mygo/paths.go）。
	// `go tool mygo dev` 下的可执行文件不叫 OpenArchitect，不叫这一句 GUI 会和
	// oac 各开一个库。
	mygo.App.SetName("OpenArchitect")
	mygo.App.WhenReady(func() {
		shell := sidebar.NewShell(sidebar.ShellOptions{
			Title:     "OpenArchitect",
			StateKey:  "main",
			LeftWidth: 220,
			Theme:     theme.Onevra,
			Left: func(c *ui.Context) {
				ui.Column(c).Fill().Gap(4).Padding(12).Children(func() {
					ui.Row(c).Gap(8).Children(func() {
						ui.Text(c, "架构图")
					})
					ui.Row(c).Gap(8).Children(func() {
						ui.Text(c, "仓库")
					})
					ui.Row(c).Gap(8).Children(func() {
						ui.Text(c, "设置")
					})
				})
			},
			Content: func(c *ui.Context) {
				ui.Column(c).Fill().Children(func() {
					ui.Spacer(c)
					ui.Text(c, "尚未扫描任何仓库").FontSize(14)
					ui.Spacer(c)
				})
			},
		})
		shell.Window().OnClosed(func() { mygo.App.Quit() })
		_ = lucide.Box // 保留图标依赖，Task 17 起真正使用
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
```

`sidebar.HeaderHeight(c)` 的内缩在 Task 16 才加（那时才有真正的 header）。这里先不管。

- [ ] **Step 6: 起窗口，肉眼验收**

```sh
go tool mygo doctor
go tool mygo dev
```

Expected: 出现一个窗口，左侧 220 DIP 侧栏列出「架构图 / 仓库 / 设置」，右侧居中一行「尚未扫描任何仓库」；关窗进程退出。

- [ ] **Step 7: 记录 PoC-1 结论**

创建 `docs/superpowers/poc/2026-10-19-slice1-findings.md`：

```markdown
# 切片 1 PoC 实测（2026-10-19）

## PoC-1 mygo 原生骨架 + onevra 侧栏窗口
- 结论：
- 数字：（go tool mygo dev 首次出帧耗时、窗口尺寸、是否报错）
- 影响：
```

**门槛不达标就停下报告**（见上表）。

- [ ] **Step 8: 提交**

```sh
go vet ./... && go test ./...
git add go.mod go.sum mygo.json main.go docs/superpowers/poc/2026-10-19-slice1-findings.md
git commit -m "chore: 用 mygo 原生模板搭起骨架并跑通 onevra 侧栏窗口（PoC-1）"
git status   # 期望干净；若模板还留了其他文件，逐条决定去留并写进提交，不要 git checkout 丢弃
```

---

### Task 2: store 打开与驱动可用性（PoC-2）

**Files:**
- Create: `internal/core/store/db.go`
- Test: `internal/core/store/db_test.go`

**Interfaces:**
- Consumes: `github.com/egoist/mygo/plugins/sqlite`（R1 的唯一例外，只准出现在本包）
- Produces:
  - `type DB struct { handle *sqlite.DB; path string }`
  - `func Open(ctx context.Context, path string) (*DB, error)` —— 开库 + `PRAGMA foreign_keys = ON`；**不**跑迁移（Task 6 才加）
  - `func (d *DB) Execute(ctx context.Context, sql string, args ...any) (sqlite.Result, error)`
  - `func (d *DB) Query(ctx context.Context, sql string, args ...any) (sqlite.Rows, error)`
  - `func (d *DB) Transact(ctx context.Context, stmts []sqlite.Statement) ([]sqlite.Result, error)`
  - `func (d *DB) Close() error`
  - `func (d *DB) UserVersion(ctx context.Context) (int, error)`
  - `func (d *DB) SetUserVersion(ctx context.Context, version int) error`

- [ ] **Step 1: 写失败测试**

`internal/core/store/db_test.go`：

```go
package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestOpenRoundTrip(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	// 表名故意带注入形状的字符与单引号，验证它们只会作为值进去，不参与 SQL 文本。
	if _, err := db.Execute(ctx, `CREATE TABLE needle (name TEXT)`); err != nil {
		t.Fatalf("CREATE: %v", err)
	}
	const tricky = `带'引号;和分号`
	if _, err := db.Execute(ctx, `INSERT INTO needle (name) VALUES (?)`, tricky); err != nil {
		t.Fatalf("INSERT: %v", err)
	}
	rows, err := db.Query(ctx, `SELECT name FROM needle`)
	if err != nil {
		t.Fatalf("SELECT: %v", err)
	}
	if len(rows.Values) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows.Values))
	}
	if got := rows.Values[0][0]; got != tricky {
		t.Fatalf("round trip = %v, want %v", got, tricky)
	}
}

func TestForeignKeysEnabled(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	rows, err := db.Query(context.Background(), `PRAGMA foreign_keys`)
	if err != nil {
		t.Fatalf("PRAGMA: %v", err)
	}
	if got := num(rows.Values[0][0]); got != 1 {
		t.Fatalf("foreign_keys = %v, want 1", rows.Values[0][0])
	}
}
```

`num` 在 Task 7 才出现——本任务先在测试文件底部放临时版，Task 7 把它移到 `rows.go` 时删掉这里的副本：

```go
func num(v any) float64 {
	switch n := v.(type) {
	case int64:
		return float64(n)
	case float64:
		return n
	default:
		t := &testing.T{}
		_ = t
		panic("unexpected column type")
	}
}
```

这个临时实现故意难看，就是为了别忘：Step 3 之后立刻由 Task 7 的 `rows.go` 取代。

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/core/store/ 2>&1 | head -20`
Expected: `internal/core/store/db.go: no Go files` 或 undefined 错误。

- [ ] **Step 3: 实现**

`internal/core/store/db.go`：

```go
// Package store keeps the local SQLite database. It is the only package in
// the module allowed to import mygo's sqlite driver (spec §5.1, R1 exception).
package store

import (
	"context"
	"fmt"
	"strconv"

	"github.com/egoist/mygo/plugins/sqlite"
)

type DB struct {
	handle *sqlite.DB
	path   string
}

func Open(ctx context.Context, path string) (*DB, error) {
	handle, err := sqlite.Open(ctx, path, sqlite.OpenOptions{})
	if err != nil {
		return nil, fmt.Errorf("store: open %s: %w", path, err)
	}
	db := &DB{handle: handle, path: path}
	if _, err := db.Execute(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		_ = handle.Close()
		return nil, fmt.Errorf("store: foreign_keys: %w", err)
	}
	return db, nil
}

func (d *DB) Path() string { return d.path }

func (d *DB) Execute(ctx context.Context, sql string, args ...any) (sqlite.Result, error) {
	return d.handle.Execute(ctx, sql, args...)
}

func (d *DB) Query(ctx context.Context, sql string, args ...any) (sqlite.Rows, error) {
	return d.handle.Query(ctx, sql, args...)
}

func (d *DB) Transact(ctx context.Context, stmts []sqlite.Statement) ([]sqlite.Result, error) {
	return d.handle.Transaction(ctx, stmts)
}

func (d *DB) Close() error { return d.handle.Close() }

func (d *DB) UserVersion(ctx context.Context) (int, error) {
	rows, err := d.Query(ctx, `PRAGMA user_version`)
	if err != nil {
		return 0, err
	}
	if len(rows.Values) == 0 || len(rows.Values[0]) == 0 {
		return 0, nil
	}
	raw, err := strconv.Atoi(fmt.Sprint(rows.Values[0][0]))
	if err != nil {
		return 0, fmt.Errorf("store: user_version: %w", err)
	}
	return raw, nil
}

// SetUserVersion interpolates its argument because SQLite takes no placeholder
// in a PRAGMA. The value only ever comes from the package's own constant.
func (d *DB) SetUserVersion(ctx context.Context, version int) error {
	_, err := d.Execute(ctx, `PRAGMA user_version = `+strconv.Itoa(version))
	return err
}
```

`newTestDB` helper（放 `internal/core/store/db_test.go`，后续任务的测试都用它）：

```go
func newTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return db
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/core/store/ -v -run 'TestOpenRoundTrip|TestForeignKeysEnabled'`
Expected: 两条 PASS。若 `sqlite.Open` 报加载库失败，这就是 **PoC-2 未过**：先按 spec §12 记录，停下报告，不要顺手换 driver。

- [ ] **Step 5: 在应用上下文里验一次库路径**

`LibraryPath` 需要在 app ready 后才有意义（它读 `mygo.App.Path(mygo.PathResources)`），所以放在 GUI 里验，而不是在单测里：

实际做法是临时在 `main.go` 的 `WhenReady` 回调末尾加两行，跑一次，记录输出，然后**用 Edit 删掉这两行**（不要用 `git checkout` 回滚，工作区里可能有别的未提交内容）：

```go
		if p, err := sqlite.LibraryPath(); err != nil {
			log.Printf("sqlite library: %v", err)
		} else {
			log.Printf("sqlite library: %s", p)
		}
		if db, err := store.Open(context.Background(), "probe.db"); err != nil {
			log.Printf("store.Open: %v", err)
		} else {
			_ = db.Close()
			log.Print("store.Open ok")
		}
```

Run: `go tool mygo dev`（`go run .` 也可，`go run` 下库缺失时插件会自己去缓存目录取）
Expected: 两行日志分别给出库文件路径与 `store.Open ok`。`probe.db` 落在工作目录，验完删掉。

- [ ] **Step 6: 记录 PoC-2 结论并提交**

findings 里补 `## PoC-2`（结论/数字/影响；数字 = `LibraryPath()` 返回的路径、`go run` 与 `go tool mygo dev` 两种跑法下是否都成功）。

```sh
go vet ./... && go test ./...
git add internal/core/store/db.go internal/core/store/db_test.go docs/superpowers/poc/2026-10-19-slice1-findings.md
git commit -m "feat: 接入 mygo SQLite 插件的 store 打开层（PoC-2）"
```

---

### Task 3: go/packages 规模探针（PoC-3，定 U2 的 LoadMode）

**Files:**
- Create: `tools/parseprobe/main.go`
- Create: `docs/superpowers/poc/2026-10-19-slice1-findings.md`（补 PoC-3 段）

**Interfaces:**
- Produces: 常量 `packagesNeedAll`（`packages.LoadMode` 的按位或），Task 9 的 `internal/scanner/language/go.go` 直接复用同一串位；探针本身是一次性工具，用完可删（本计划保留它，因为每次调整映射规则都要复测）。

- [ ] **Step 1: 写探针**

`tools/parseprobe/main.go`：

```go
// Command parseprobe measures go/packages cost on a real repository: exported
// named types, edges, wall time and peak heap. Throwaway harness for PoC-3.
package main

import (
	"context"
	"flag"
	"fmt"
	"go/types"
	"log"
	"os"
	"runtime"
	"strings"
	"time"

	"golang.org/x/tools/go/packages"
)

// packagesNeedAll is the LoadMode the scanner will use; PoC-3 picks the bits.
const packagesNeedAll = packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
	packages.NeedTypes | packages.NeedTypesInfo | packages.NeedDeps | packages.NeedModule

func main() {
	root := flag.String("root", ".", "repository root holding go.mod")
	flag.Parse()

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	start := time.Now()

	modPath, pkgs, err := load(*root)
	if err != nil {
		log.Fatal(err)
	}
	elapsed := time.Since(start)
	runtime.ReadMemStats(&after)

	var exported, ownPkgs, errs int
	for _, pkg := range pkgs {
		if len(pkg.Errors) > 0 {
			errs++
		}
		if pkg.Module == nil || pkg.Types == nil {
			continue
		}
		if !own(modPath, pkg.PkgPath) {
			continue
		}
		ownPkgs++
		scope := pkg.Types.Scope()
		for _, name := range scope.Names() {
			obj := scope.Lookup(name)
			if _, ok := componentOf(obj); !ok {
				continue
			}
			exported++
		}
	}

	fmt.Printf("module=%s pkgs=%d ownPkgs=%d exportedTypes=%d packageErrors=%d elapsed=%s heapAllocMB=%.1f sysMB=%.1f\n",
		modPath, len(pkgs), ownPkgs, exported, errs, elapsed.Round(time.Millisecond),
		float64(after HeapAlloc)/(1<<20), float64(after.Sys)/(1<<20))
}
```

上面 `after HeapAlloc` 是故意写错的占位？不——计划不允许占位，正确写法是 `float64(after.HeapAlloc)/(1<<20)`。照这个来。

```go
func load(root string) (string, []*packages.Package, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", nil, err
	}
	cfg := &packages.Config{Mode: packagesNeedAll, Dir: abs, Context: context.Background()}
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return "", nil, err
	}
	mod := ""
	for _, p := range pkgs {
		if p.Module != nil {
			mod = p.Module.Path
			break
		}
	}
	if mod == "" {
		return "", nil, fmt.Errorf("no module under %s", abs)
	}
	return mod, pkgs, nil
}

func own(modulePath, pkgPath string) bool {
	return pkgPath == modulePath || strings.HasPrefix(pkgPath, modulePath+"/")
}

func componentOf(obj types.Object) (*types.Named, bool) {
	tn, ok := obj.(*types.TypeName)
	if !ok || !tn.Exported() {
		return nil, false
	}
	named, ok := tn.Type().(*types.Named)
	if !ok {
		return nil, false
	}
	switch named.Underlying().(type) {
	case *types.Struct, *types.Interface:
		return named, true
	default:
		return nil, false
	}
}
```

`load` 需要 `path/filepath` 导入。

- [ ] **Step 2: 跑真实仓库**

用 onevra 自己的 `source` 模块当一个中等仓库（它有 components 树、纯 Go、能解析）：

```sh
go run ./tools/parseprobe -root /Users/futuremeng/go/pkg/mod/github.com/oneo-me/onevra/source@v0.2026.12
```

再跑一个你手上更大的仓库（有就加，没有就用 mygo 模块目录），命令同上换路径。

Expected: 一行 `module=... exportedTypes=N elapsed=... heapAllocMB=...`。

- [ ] **Step 3: 收窄对比**

把 `packagesNeedAll` 换成不带 `NeedDeps` 的串（临时用 sed 或 Edit 改一行），再跑一次，记下两份耗时/内存差。这个数字决定 Task 9 是否保留 `NeedDeps`（`NeedTypes` 在类型检查跨包时基本必须依赖它，所以先实测再定，不要凭猜）。改完把常量恢复成 `packagesNeedAll` 原值。

- [ ] **Step 4: 记录 PoC-3 结论（并回填 spec §6.4 的规模预期）**

findings 补 `## PoC-3`：两个仓库各自的 `exportedTypes/elapsed/heapAllocMB/sysMB`，以及 `NeedDeps` 开与关的差值。**门槛：≤ 60 s 且 ≤ 2 GB**；不达标停下报告。同时把「一个中等仓库的实际类型数」写进 spec §6.4 那句"200~600 个 Component"旁边，作为它是否成立的证据。

- [ ] **Step 5: 提交**

```sh
go vet ./... && go test ./...
git add tools/parseprobe/main.go docs/superpowers/poc/2026-10-19-slice1-findings.md
git commit -m "chore: 加入 go/packages 规模探针并实测解析成本（PoC-3）"
```

---

### Task 4: flow 在 600 节点下的帧时间与稳定性（PoC-4）

**Files:**
- Create: `tools/flowprobe/main.go`
- Create: `docs/superpowers/poc/2026-10-19-slice1-findings.md`（补 PoC-4 段）

**Interfaces:**
- Produces: 尺寸常量 `probeCardWidth = 168`、`probeCardHeight = 46`（与 Task 17 生产卡片同值，spec §9.3）；结论决定 Task 18 是否照 §9.3 直接用 `flow`。

- [ ] **Step 1: 写探针（自带一个与生产同尺寸的合成卡片，避免依赖尚未存在的 internal/ui）**

`tools/flowprobe/main.go`：

```go
// Command flowprobe renders synthetic flow diagrams at 50/300/600 nodes and
// measures per-frame cost. Throwaway harness for PoC-4.
package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components/flow"
	"github.com/oneo-me/onevra/source/icons/lucide"
	"github.com/oneo-me/onevra/source/theme"
)

// Same fixed size the production node card uses (spec §9.3).
const (
	probeCardWidth  = 168
	probeCardHeight = 46
)

func main() {
	nodes := flag.Int("nodes", 600, "node count to render")
	frames := flag.Int("frames", 120, "frames to time")
	flag.Parse()

	mygo.App.WhenReady(func() {
		win := mygo.NewWindow(mygo.WindowOptions{Title: "flowprobe", Width: 1400, Height: 900})
		var state flow.State
		data := syntheticData(*nodes)
		view := func(c *ui.Context) {
			start := time.Now()
			win.SetContent(flow.View(c, &state, data, flow.Options{
				Label:     fmt.Sprintf("%d nodes", len(data.Nodes)),
				Direction: flow.LeftToRight,
				NodeGap:   28,
				RankGap:   64,
			}))
			c.Invalidate()
			if c.Frame() > 1 {
				fmt.Printf("frame %d: %s\n", c.Frame(), time.Since(start))
			}
		}
		win.SetContent(nil)
		_ = lucide.Box
		win.Show()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

func syntheticData(n int) flow.Data {
	nodes := make([]flow.Node, 0, n)
	edges := make([]flow.Edge, 0, n*3)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("cmp:example.com/probe/p%d.T%d", i/10, i%10)
		nodes = append(nodes, flow.Node{
			ID:     id,
			Label:  fmt.Sprintf("Type%d", i),
			Width:  probeCardWidth,
			Height: probeCardHeight,
			Content: func(c *ui.Context) {
				ui.Row(c).Gap(6).Padding(8).Children(func() {
					ui.Text(c, fmt.Sprintf("Type%d", i)).FontSize(12)
				})
			},
		})
		if i > 9 {
			edges = append(edges, flow.Edge{Source: id, Target: fmt.Sprintf("cmp:example.com/probe/p%d.T%d", (i-7)/10, (i-7)%10)})
		}
	}
	return flow.Data{Nodes: nodes, Edges: edges}
}
```

这段是骨架，Step 2 会告诉你它哪里必须按实测调整：**`c.Frame()`、`win.SetContent` 的确切用法以 mygo 原生 UI 文档为准**（`views.md` 里内容是 `App.Run` 之前注册 window content 回调，不是每帧 `SetContent`）。正确写法：

```go
		win := mygo.NewWindow(mygo.WindowOptions{Title: "flowprobe", Width: 1400, Height: 900})
		var state flow.State
		data := syntheticData(*nodes)
		var timings []time.Duration
		state.Ready() // 触发一次零值检查，确认 State 可空跑
		_ = timings
		win.SetContent(func(c *ui.Context) {
			start := time.Now()
			flow.View(c, &state, data, flow.Options{
				Label: fmt.Sprintf("%d nodes", len(data.Nodes)), Direction: flow.LeftToRight,
				NodeGap: 28, RankGap: 64,
			})
			timings = append(timings, time.Since(start))
		})
		win.Show()
```

`Window.SetContent(func(*ui.Context))` 是 mygo 注册内容回调的方法（见 `$GOMODCACHE/github.com/egoist/mygo@v0.5.5/content.go`）；执行本步骤前先 `grep -n 'func (w \*Window) SetContent' content.go` 确认签名，若不同就用 `mygo.NewWindow(mygo.WindowOptions{Content: ...})` 那个字段。

- [ ] **Step 2: 三档实测**

```sh
for n in 50 300 600; do go run ./tools/flowprobe -nodes $n | tail -20; done
```

记录每档的：首帧耗时（含布局）、稳定后每帧耗时中位数、`state.Ready()` 变 true 的帧数、目视可读性。

- [ ] **Step 3: 抖动检查**

在探针里追加两次间隔 500 ms 的 `Advance` 等价操作（原生 UI 下用 `c.Invalidate()` + `mygo.App` 定时器重绘；纯函数视图在尺寸不变时不该重排），比较 `state.Position(id)` 前后是否一致：

```go
		if state.Ready() {
			x1, y1, ok1 := state.Position("cmp:example.com/probe/p0.T0")
			// 强制再走 30 帧后重取
			x2, y2, ok2 := state.Position("cmp:example.com/probe/p0.T0")
			fmt.Printf("stable=%v drift=%v\n", ok1 && ok2, (x2-x1)*(x2-x1)+(y2-y1)*(y2-y1))
		}
```

Expected: `drift` 为 0（浮点比较用 `== 0` 是允许的，因为布局结果完全相同时才是 0；若非 0 说明卡片自然测量在改尺寸——这正是 §9.3 定死尺寸要防的事）。

- [ ] **Step 4: 记录 PoC-4 结论**

findings 补 `## PoC-4`。**门槛：600 节点 ≤ 16 ms/帧且 drift = 0**。三种结果对应三条路（spec §12）：性能过、可读性差 → 默认只画 Container 层，Component 靠双击下钻（这会让 Task 17 多一个 `showComponents bool`；先报告再加）；性能不过 → 停下报告走逃生通道（只换 `internal/ui`，core 不动）。

- [ ] **Step 5: 提交**

```sh
go vet ./... && go test ./...
git add tools/flowprobe/main.go docs/superpowers/poc/2026-10-19-slice1-findings.md
git commit -m "chore: 实测 flow 在 50/300/600 节点下的帧时间与布局稳定性（PoC-4）"
```

---

## Phase 1 — core 与 scanner（Task 5–13）

这一相结束后，`oac` 之外的全部能力都已可测；Phase 2 只做装配。

### Task 5: model 与稳定 ID

**Files:**
- Create: `internal/core/model/element.go`、`relationship.go`、`snapshot.go`、`repo.go`、`ids.go`
- Test: `internal/core/model/ids_test.go`

**Interfaces:**
- Produces:
  - `model.ElementType` 常量 `TypeSystem="System"`、`TypeContainer="Container"`、`TypeComponent="Component"`
  - `model.Element{ID, Type ElementType, Name, Description, RepoID, FilePath string; Methods []string}`（`Methods` = 本计划定稿 #1）
  - `model.RelType` 常量 `RelCalls="calls"`、`RelDependsOn="depends_on"`、`RelContains="contains"`
  - `model.Relationship{FromID, ToID string; Type RelType; Weight int}`
  - `model.ArchitectureSnapshot{ID, RepoID, CommitHash, Branch string; PackageErrors []string; CreatedAt string}`
  - `model.Repo{ID, Name, LocalPath, Lang string}`
  - `model.SystemID(modulePath string) string`
  - `model.ContainerID(pkgPath string) string`
  - `model.ComponentID(pkgPath, typeName string) string`
  - `model.RepoIDFor(localPath string) string`
  - `model.PkgPathOf(elementID string) string`（从 cmp/ctn id 反解包路径，Task 10 的同包判定、Task 17 的卡片第二行都用它）

`ContainerID`/`ComponentID` 只收**完整包路径**（`example.com/cart/cmd/api`），不收 modulePath：ID 里带完整包路径是 spec §6.2 的字面要求，而 pkgPath 本来就包含 modulePath，多传一个参数只会给调用方一个把两者弄反的机会。pkgPath 由 Task 9 的 `ownPackage` 从 `go/packages` 直接给出，永远是完整形态。

- [ ] **Step 1: 写失败测试**

`internal/core/model/ids_test.go`：

```go
package model

import (
	"path/filepath"
	"testing"
)

func TestIDRules(t *testing.T) {
	const mod = "example.com/cart"
	cases := []struct{ got, want string }{
		{SystemID(mod), "sys:example.com/cart"},
		{ContainerID(mod + "/cmd/api"), "ctn:example.com/cart/cmd/api"},
		{ComponentID(mod+"/internal/basket", "Basket"), "cmp:example.com/cart/internal/basket.Basket"},
		// 泛型类型只用声明名，丢弃参数：一个类型一个节点（spec §6.2）。
		{ComponentID(mod+"/pkg", "Box"), "cmp:example.com/cart/pkg.Box"},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("got %q want %q", tc.got, tc.want)
		}
	}
}

func TestIDsAreStableAndNoCollision(t *testing.T) {
	const mod = "example.com/cart"
	if SystemID(mod) != SystemID(mod) {
		t.Fatal("SystemID must be pure")
	}
	// 不同包下的同名类型必须分开。
	a := ComponentID(mod+"/aaa", "Box")
	b := ComponentID(mod+"/bbb", "Box")
	if a == b {
		t.Fatalf("collision: %s == %s", a, b)
	}
	// 三种前缀互不重叠。
	prefixes := []string{SystemID(mod), ContainerID(mod), ComponentID(mod, "T")}
	seen := map[string]bool{}
	for _, id := range prefixes {
		switch id[:4] {
		case "sys:", "ctn:", "cmp:":
		default:
			t.Fatalf("bad prefix %q", id)
		}
		if seen[id] {
			t.Fatalf("collision among %v", prefixes)
		}
		seen[id] = true
	}
}

func TestRepoIDIsPathBased(t *testing.T) {
	abs, err := filepath.Abs("/tmp/whatever")
	if err != nil {
		t.Fatal(err)
	}
	if got := RepoIDFor("/tmp/whatever"); got != "repo:"+abs {
		t.Fatalf("got %q want %q", got, "repo:"+abs)
	}
	if RepoIDFor("/tmp/a") == RepoIDFor("/tmp/b") {
		t.Fatal("different paths must give different ids")
	}
}

func TestPkgPathOf(t *testing.T) {
	const mod = "example.com/cart"
	if got := PkgPathOf(ComponentID(mod+"/internal/x", "Y")); got != mod+"/internal/x" {
		t.Fatalf("cmp: got %q", got)
	}
	if got := PkgPathOf(ContainerID(mod + "/cmd/api")); got != mod+"/cmd/api" {
		t.Fatalf("ctn: got %q", got)
	}
	if got := PkgPathOf(SystemID(mod)); got != "" {
		t.Fatalf("sys must have no package: got %q", got)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/core/model/`
Expected: 编译失败，`undefined: SystemID`。

- [ ] **Step 3: 实现**

`internal/core/model/element.go`：

```go
// Package model holds the C4-shaped architecture vocabulary shared by the
// scanner, the store, the graph and the UI. It imports no UI stack (R1).
package model

type ElementType string

const (
	TypeSystem    ElementType = "System"
	TypeContainer ElementType = "Container"
	TypeComponent ElementType = "Component"
)

type Element struct {
	ID          string
	Type        ElementType
	Name        string
	Description string
	RepoID      string
	FilePath    string
	// Methods lists exported method names of a Component. Plan amendment 1.
	Methods []string
}
```

`relationship.go`：

```go
package model

type RelType string

const (
	RelCalls     RelType = "calls"
	RelDependsOn RelType = "depends_on"
	RelContains  RelType = "contains"
)

// Relationship carries Weight instead of an ID: the (from,to,type) triple is
// unique after de-duplication (spec §6.4), so a surrogate key is pure cost.
type Relationship struct {
	FromID string
	ToID   string
	Type   RelType
	Weight int
}
```

`snapshot.go`：

```go
package model

// ArchitectureSnapshot is one scan. CommitHash/Branch stay empty in this slice
// (no Git), and PackageErrors keeps the per-package failures collected during
// parsing (spec §10) for audit only.
type ArchitectureSnapshot struct {
	ID            string
	RepoID        string
	CommitHash    string
	Branch        string
	PackageErrors []string
	CreatedAt     string
}
```

`repo.go`：

```go
package model

// Repo keeps only fields this slice has a source for: URL and Branch were
// dropped deliberately (spec deviation 9).
type Repo struct {
	ID        string
	Name      string
	LocalPath string
	Lang      string
}
```

`ids.go`：

```go
package model

import (
	"path/filepath"
	"strings"
)

func SystemID(modulePath string) string { return "sys:" + modulePath }

func ContainerID(pkgPath string) string { return "ctn:" + pkgPath }

func ComponentID(pkgPath, typeName string) string {
	return "cmp:" + pkgPath + "." + typeName
}

// RepoIDFor keys on the absolute local path; the same checkout is the same row.
func RepoIDFor(localPath string) string { return "repo:" + filepath.Clean(localPath) }

// PkgPathOf reverses the package part of a Container or Component id.
func PkgPathOf(id string) string {
	switch {
	case strings.HasPrefix(id, "ctn:"):
		return strings.TrimPrefix(id, "ctn:")
	case strings.HasPrefix(id, "cmp:"):
		body := strings.TrimPrefix(id, "cmp:")
		if i := strings.LastIndex(body, "."); i >= 0 {
			return body[:i]
		}
	}
	return ""
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/core/model/ -v`
Expected: 4 个测试 PASS。

- [ ] **Step 5: 提交**

```sh
go vet ./... && go test ./...
git add internal/core/model/
git commit -m "feat: 架构模型与跨扫描稳定的元素 ID 规则"
```

---

### Task 6: schema 与迁移（含 methods_json）

**Files:**
- Create: `internal/core/store/schema.go`、`internal/core/store/migrate.go`
- Modify: `internal/core/store/db.go`（`Open` 末尾调用 `Migrate`）
- Test: `internal/core/store/migrate_test.go`

**Interfaces:**
- Consumes: Task 2 的 `DB.Query/Execute/UserVersion/SetUserVersion`
- Produces: `const schemaVersion = 1`；`func (d *DB) Migrate(ctx context.Context) error`；`var migrations = [][]string{ v1Statements }`（索引 i 对应版本 i+1）

- [ ] **Step 1: 写失败测试**

`internal/core/store/migrate_test.go`：

```go
package store

import (
	"context"
	"sort"
	"testing"
)

func TestMigrateCreatesFiveTables(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	rows, err := db.Query(context.Background(),
		`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	var got []string
	for _, v := range rows.Values {
		got = append(got, str(v[0]))
	}
	sort.Strings(got)
	want := []string{"app_config", "arch_elements", "arch_relationships", "architecture_snapshots", "repos"}
	if len(got) != len(want) {
		t.Fatalf("tables = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tables = %v, want %v", got, want)
		}
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	for i := 0; i < 3; i++ {
		if err := db.Migrate(context.Background()); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
	v, err := db.UserVersion(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if v != schemaVersion {
		t.Fatalf("user_version = %d, want %d", v, schemaVersion)
	}
}

func TestSnapshotCascadeDeletesChildren(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	ctx := context.Background()
	for _, q := range []string{
		`INSERT INTO repos (id, name, local_path, lang) VALUES ('repo:/x', 'x', '/x', 'go')`,
		`INSERT INTO architecture_snapshots (id, repo_id) VALUES ('s1', 'repo:/x')`,
		`INSERT INTO arch_elements (snapshot_id, element_id, element_type, name, repo_id) VALUES ('s1', 'cmp:a.B', 'Component', 'B', 'repo:/x')`,
		`INSERT INTO arch_relationships (snapshot_id, from_id, to_id, relationship_type) VALUES ('s1', 'cmp:a.B', 'cmp:c.D', 'calls')`,
	} {
		if _, err := db.Execute(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if _, err := db.Execute(ctx, `DELETE FROM repos WHERE id = 'repo:/x'`); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"architecture_snapshots", "arch_elements", "arch_relationships"} {
		rows, err := db.Query(ctx, `SELECT count(*) FROM `+table)
		if err != nil {
			t.Fatalf("%s: %v", table, err)
		}
		if got := num(rows.Values[0][0]); got != 0 {
			t.Fatalf("%s survived cascade: count = %v", table, got)
		}
	}
}

// 索引名要能被查到：idx_elements_repo 存在过就说明 CREATE INDEX 都跑了。
func TestIndexesExist(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	rows, err := db.Query(context.Background(),
		`SELECT name FROM sqlite_master WHERE type='index' AND name LIKE 'idx_%'`)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, v := range rows.Values {
		got[str(v[0])] = true
	}
	for _, want := range []string{"idx_snapshots_repo", "idx_elements_repo", "idx_elements_name",
		"idx_elements_element", "idx_rel_from", "idx_rel_to"} {
		if !got[want] {
			t.Fatalf("missing index %s (have %v)", want, got)
		}
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/core/store/`
Expected: FAIL，`no such table: repos`（`Open` 里还没迁移）与 `undefined: schemaVersion`。

- [ ] **Step 3: 实现 schema**

`internal/core/store/schema.go`。DDL 逐字取自 spec §7.1，加上本计划定稿 #1 的 `methods_json` 一列；并把 spec 里名字写错的 `idx_elements_type`（它建的其实是 `element_id`）改名 `idx_elements_element`：

```go
package store

const schemaVersion = 1

var migrations = [][]string{{
	`CREATE TABLE IF NOT EXISTS repos (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		local_path TEXT NOT NULL,
		lang TEXT NOT NULL,
		created_at TEXT DEFAULT (datetime('now')),
		updated_at TEXT DEFAULT (datetime('now'))
	)`,
	`CREATE TABLE IF NOT EXISTS architecture_snapshots (
		id TEXT PRIMARY KEY,
		repo_id TEXT NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
		commit_hash TEXT NOT NULL DEFAULT '',
		branch TEXT NOT NULL DEFAULT '',
		package_errors_json TEXT NOT NULL DEFAULT '[]',
		created_at TEXT DEFAULT (datetime('now'))
	)`,
	`CREATE INDEX IF NOT EXISTS idx_snapshots_repo ON architecture_snapshots(repo_id)`,
	`CREATE TABLE IF NOT EXISTS arch_elements (
		snapshot_id TEXT NOT NULL REFERENCES architecture_snapshots(id) ON DELETE CASCADE,
		element_id TEXT NOT NULL,
		element_type TEXT NOT NULL,
		name TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		repo_id TEXT NOT NULL,
		file_path TEXT NOT NULL DEFAULT '',
		methods_json TEXT NOT NULL DEFAULT '[]',
		PRIMARY KEY (snapshot_id, element_id)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_elements_repo ON arch_elements(repo_id)`,
	`CREATE INDEX IF NOT EXISTS idx_elements_name ON arch_elements(name)`,
	`CREATE INDEX IF NOT EXISTS idx_elements_element ON arch_elements(element_id)`,
	`CREATE TABLE IF NOT EXISTS arch_relationships (
		snapshot_id TEXT NOT NULL REFERENCES architecture_snapshots(id) ON DELETE CASCADE,
		from_id TEXT NOT NULL,
		to_id TEXT NOT NULL,
		relationship_type TEXT NOT NULL,
		weight INTEGER NOT NULL DEFAULT 1,
		PRIMARY KEY (snapshot_id, from_id, to_id, relationship_type)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_rel_from ON arch_relationships(from_id)`,
	`CREATE INDEX IF NOT EXISTS idx_rel_to ON arch_relationships(to_id)`,
	`CREATE TABLE IF NOT EXISTS app_config (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	)`,
}}
```

- [ ] **Step 4: 实现迁移**

`internal/core/store/migrate.go`：

```go
package store

import (
	"context"
	"fmt"
)

// Migrate applies every migration above the stored user_version, in order.
// Each version's statements are CREATE TABLE/INDEX IF NOT EXISTS, so a
// partially applied version is safe to replay.
func (d *DB) Migrate(ctx context.Context) error {
	current, err := d.UserVersion(ctx)
	if err != nil {
		return err
	}
	if current > schemaVersion {
		return fmt.Errorf("store: database schema %d is newer than code schema %d", current, schemaVersion)
	}
	for version := current + 1; version <= schemaVersion; version++ {
		for _, statement := range migrations[version-1] {
			if _, err := d.Execute(ctx, statement); err != nil {
				return fmt.Errorf("store: migrate to %d: %w", version, err)
			}
		}
		if err := d.SetUserVersion(ctx, version); err != nil {
			return fmt.Errorf("store: migrate version %d: %w", version, err)
		}
	}
	return nil
}
```

`db.go` 的 `Open` 在 `PRAGMA foreign_keys` 之后加：

```go
	if err := db.Migrate(ctx); err != nil {
		_ = handle.Close()
		return nil, err
	}
```

- [ ] **Step 5: 跑测试确认通过**

Run: `go test ./internal/core/store/ -v`
Expected: Task 2 的两条 + 本任务四条全 PASS。若 `TestSnapshotCascadeDeletesChildren` 失败，说明 `PRAGMA foreign_keys` 没生效——回去看 `Open` 的顺序，不要改成手写级联删除。

- [ ] **Step 6: 提交**

```sh
go vet ./... && go test ./...
git add internal/core/store/schema.go internal/core/store/migrate.go internal/core/store/db.go internal/core/store/migrate_test.go
git commit -m "feat: 五表 schema 与按 user_version 的幂等迁移"
```

---

### Task 7: 行映射与 CRUD，单事务写快照（PoC-5）

**Files:**
- Create: `internal/core/store/rows.go`、`repo.go`、`snapshot.go`、`element.go`、`relationship.go`、`config.go`
- Modify: `internal/core/store/db_test.go`（删掉临时 `num`，改用 `rows.go` 的版本）
- Test: `internal/core/store/rows_test.go`、`snapshot_test.go`、`bench_test.go`

**Interfaces:**
- Consumes: Task 5 的 model；Task 6 的 schema
- Produces:
  - `func rowMap(cols []string) map[string]int`
  - `func rowsToMaps(r sqlite.Rows) []map[string]any`
  - `func str(v any) string` / `func num(v any) float64` / `func jsonString(v any) string` / `func stringList(raw string) []string`
  - `func (d *DB) UpsertRepo(ctx context.Context, r model.Repo) error`
  - `func (d *DB) Repos(ctx context.Context) ([]model.Repo, error)`（按 name 排序，Task 21 的仓库列表页直接吃这个顺序）
  - `func (d *DB) SaveSnapshot(ctx context.Context, s model.ArchitectureSnapshot, e []model.Element, r []model.Relationship) error`
  - `func (d *DB) LatestSnapshotID(ctx context.Context, repoID string) (string, error)`（该仓库没有快照时返回 `("", nil)`：空态不是错误）
  - `func (d *DB) Snapshot(ctx context.Context, snapshotID string) (model.ArchitectureSnapshot, error)`（Task 16 的状态行取 PackageErrors）
  - `func (d *DB) SnapshotForElement(ctx context.Context, elementID string) (string, error)`
  - `func (d *DB) Elements(ctx context.Context, snapshotID string) ([]model.Element, error)`
  - `func (d *DB) Relationships(ctx context.Context, snapshotID string) ([]model.Relationship, error)`
  - `func (d *DB) GetElement(ctx context.Context, snapshotID, elementID string) (model.Element, error)`
  - `func (d *DB) SearchElements(ctx context.Context, snapshotID, query string) ([]model.Element, error)`
  - `func (d *DB) ConfigGet(ctx context.Context, key string) (string, error)` / `ConfigSet(ctx, key, value string) error`
  - `const maxSearchResults = 50`

- [ ] **Step 1: 写失败测试（列名映射 + LIKE 转义）**

`internal/core/store/rows_test.go`：

```go
package store

import (
	"encoding/json"
	"testing"

	"github.com/egoist/mygo/plugins/sqlite"
)

func TestRowsToMapsIgnoresColumnOrder(t *testing.T) {
	// 故意把列序打乱：按下标取列的写法在这里必然错位。
	r := sqlite.Rows{
		Columns: []string{"name", "element_id", "repo_id"},
		Values: [][]any{
			{"Basket", "cmp:x.Basket", "repo:/x"},
			{"Order", "cmp:x.Order", "repo:/x"},
		},
	}
	got := rowsToMaps(r)
	if len(got) != 2 {
		t.Fatalf("rows = %d", len(got))
	}
	if str(got[0]["element_id"]) != "cmp:x.Basket" || str(got[1]["name"]) != "Order" {
		t.Fatalf("mapping = %v", got)
	}
}

func TestStringListRoundTrip(t *testing.T) {
	raw := jsonString([]string{"A", "带\"引号"})
	var back []string
	if err := json.Unmarshal([]byte(raw), &back); err != nil {
		t.Fatal(err)
	}
	if len(stringList(raw)) != 2 {
		t.Fatalf("stringList(%s) = %v", raw, stringList(raw))
	}
	if stringList("not json") != nil {
		t.Fatal("corrupt json must yield no entries, not panic")
	}
	if got := stringList(""); len(got) != 0 {
		t.Fatalf("empty column = %v", got)
	}
}

func TestEscapeLike(t *testing.T) {
	cases := map[string]string{
		"plain": `%plain%`,
		"a%b":   `a\%b`,
		"a_b":   `a\_b`,
		`a\b`:   `a\\b`,
	}
	for in, want := range cases {
		if got := "%"+escapeLike(in)+"%"; got != want {
			t.Errorf("escapeLike(%q) -> %q want %q", in, got, want)
		}
	}
}
```

`sqlite.Rows` 是导出 struct（`database.go:50`：`Columns []string`、`Values [][]any`；值只可能是 nil / int64 / float64 / string / []byte），测试里直接字面量构造就行，不需要额外的助手；`rows_test.go` 顶部要 import `github.com/egoist/mygo/plugins/sqlite`。

- [ ] **Step 2: 写失败测试（快照读写 + 原子性 + 注入面）**

`internal/core/store/snapshot_test.go`：

```go
package store

import (
	"context"
	"testing"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

func repoFixture() model.Repo {
	return model.Repo{ID: model.RepoIDFor("/repo/带'引号;和分号"), Name: "x", LocalPath: "/repo/带'引号;和分号", Lang: "go"}
}

func snapshotFixture(repo model.Repo) model.ArchitectureSnapshot {
	return model.ArchitectureSnapshot{
		ID: "s0000000000000000001", RepoID: repo.ID,
		PackageErrors: []string{"pkga: undefined: Foo"},
	}
}

func elementsFixture(repo model.Repo) []model.Element {
	return []model.Element{
		{ID: model.SystemID("example.com/x"), Type: model.TypeSystem, Name: "x", RepoID: repo.ID},
		{ID: "cmp:example.com/x/internal/basket.Basket", Type: model.TypeComponent, Name: "Basket",
			Description: "Basket holds items.", RepoID: repo.ID, FilePath: "internal/basket/basket.go",
			Methods: []string{"Add", "Total"}},
	}
}

func relsFixture() []model.Relationship {
	return []model.Relationship{
		{FromID: model.SystemID("example.com/x"), ToID: "cmp:example.com/x/internal/basket.Basket", Type: model.RelContains, Weight: 1},
		{FromID: "cmp:example.com/x/internal/basket.Basket", ToID: model.SystemID("example.com/x"), Type: model.RelCalls, Weight: 3},
	}
}

func TestSaveSnapshotRoundTrip(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	ctx := context.Background()
	repo := repoFixture()
	if err := db.UpsertRepo(ctx, repo); err != nil {
		t.Fatal(err)
	}
	err := db.SaveSnapshot(ctx, snapshotFixture(repo), elementsFixture(repo), relsFixture())
	if err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	latest, err := db.LatestSnapshotID(ctx, repo.ID)
	if err != nil || latest != snapshotFixture(repo).ID {
		t.Fatalf("LatestSnapshotID = %q, %v", latest, err)
	}
	// A repo with no snapshot is an empty state, not an error (Task 16's Reload
	// loops over every repo and must not fail on the ones never scanned).
	empty, err := db.LatestSnapshotID(ctx, model.RepoIDFor("/tmp/never-scanned"))
	if err != nil || empty != "" {
		t.Fatalf("LatestSnapshotID(no repo) = %q, %v", empty, err)
	}

	elements, err := db.Elements(ctx, latest)
	if err != nil {
		t.Fatal(err)
	}
	if len(elements) != 2 {
		t.Fatalf("elements = %d", len(elements))
	}
	if elements[0].ID != "cmp:example.com/x/internal/basket.Basket" {
		t.Fatalf("Elements must come back sorted by element_id, got %q first", elements[0].ID)
	}
	if got := elements[0].Description; got != "Basket holds items." {
		t.Fatalf("description = %q", got)
	}
	if len(elements[0].Methods) != 2 || elements[0].Methods[1] != "Total" {
		t.Fatalf("methods = %v", elements[0].Methods)
	}

	rels, err := db.Relationships(ctx, latest)
	if err != nil {
		t.Fatal(err)
	}
	if len(rels) != 2 {
		t.Fatalf("relationships = %d", len(rels))
	}
	// `Relationships` 的 SQL 是 ORDER BY from_id, to_id, relationship_type：
	// calls 的 FromID 以 cmp: 开头排在前面，contains 的 FromID 是 sys: 排后面。
	if rels[0].Type != model.RelCalls || rels[0].Weight != 3 {
		t.Fatalf("rels[0] = %+v", rels[0])
	}
	if rels[1].Type != model.RelContains {
		t.Fatalf("rels[1] = %+v", rels[1])
	}

	one, err := db.GetElement(ctx, latest, "cmp:example.com/x/internal/basket.Basket")
	if err != nil {
		t.Fatalf("GetElement: %v", err)
	}
	if one.FilePath != "internal/basket/basket.go" {
		t.Fatalf("file path = %q", one.FilePath)
	}
}
```

继续，原子性与检索：

```go
// TestSnapshotRowRoundTrip covers the read path Task 16's status line uses:
// the package-error list must survive the JSON column.
func TestSnapshotRowRoundTrip(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	ctx := context.Background()
	repo := repoFixture()
	if err := db.UpsertRepo(ctx, repo); err != nil {
		t.Fatal(err)
	}
	want := snapshotFixture(repo)
	if err := db.SaveSnapshot(ctx, want, elementsFixture(repo), relsFixture()); err != nil {
		t.Fatal(err)
	}
	got, err := db.Snapshot(ctx, want.ID)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if got.RepoID != repo.ID || len(got.PackageErrors) != 1 ||
		got.PackageErrors[0] != "pkga: undefined: Foo" {
		t.Fatalf("got %+v", got)
	}
	if got.CreatedAt == "" {
		t.Fatal("created_at must come back from the column default")
	}
	if _, err := db.Snapshot(ctx, "s0000000000000009999"); err == nil {
		t.Fatal("Snapshot of a missing id must fail")
	}
}

func TestSaveSnapshotRollsBackCompletely(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	ctx := context.Background()
	repo := repoFixture()

	// 让快照行违反 `architecture_snapshots.repo_id` 的外键：`SaveSnapshot`
	// 是单事务写三类行，第 1 条语句失败就整批回滚，所以三张表都必须还是空的。
	// 不用测试专用钩子注入失败语句——那会把测试 seam 长进生产 API 里。
	bad := snapshotFixture(repo)
	bad.RepoID = "repo:/not-inserted"
	err := db.SaveSnapshot(ctx, bad, elementsFixture(repo), relsFixture())
	if err == nil {
		t.Fatal("expected foreign key failure")
	}
	for _, table := range []string{"architecture_snapshots", "arch_elements", "arch_relationships"} {
		rows, qerr := db.Query(ctx, `SELECT count(*) FROM `+table)
		if qerr != nil {
			t.Fatalf("%s: %v", table, qerr)
		}
		if got := num(rows.Values[0][0]); got != 0 {
			t.Fatalf("%s survived rollback: %d rows", table, got)
		}
	}
}

func TestUpsertRepoIsIdempotentAndUpdatesName(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	ctx := context.Background()
	repo := repoFixture()
	repo.Name = "first"
	if err := db.UpsertRepo(ctx, repo); err != nil {
		t.Fatal(err)
	}
	repo.Name = "second"
	if err := db.UpsertRepo(ctx, repo); err != nil {
		t.Fatal(err)
	}
	rows, err := db.Query(ctx, `SELECT name FROM repos`)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows.Values) != 1 || str(rows.Values[0][0]) != "second" {
		t.Fatalf("rows = %v", rows.Values)
	}
}

func TestSearchElementsEscapesWildcards(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	ctx := context.Background()
	repo := repoFixture()
	if err := db.UpsertRepo(ctx, repo); err != nil {
		t.Fatal(err)
	}
	elements := []model.Element{
		{ID: "cmp:a.Basket", Type: model.TypeComponent, Name: "Basket", RepoID: repo.ID},
		{ID: "cmp:a.Bask_x", Type: model.TypeComponent, Name: "Bask_x", RepoID: repo.ID},
		{ID: "cmp:a.Percent", Type: model.TypeComponent, Name: "100%", RepoID: repo.ID},
	}
	if err := db.SaveSnapshot(ctx, snapshotFixture(repo), elements, nil); err != nil {
		t.Fatal(err)
	}
	// "_" 是 LIKE 单字符通配：不转义的话 "Bask_x" 会连 Basket 一起命中。
	got, err := db.SearchElements(ctx, snapshotFixture(repo).ID, "Bask_x")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "Bask_x" {
		t.Fatalf("underscore leaked: %+v", got)
	}
	got, err = db.SearchElements(ctx, snapshotFixture(repo).ID, "100%")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("percent literal: %+v", got)
	}
}

func TestSnapshotForElementPicksLatest(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	ctx := context.Background()
	repo := repoFixture()
	if err := db.UpsertRepo(ctx, repo); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"s0000000000000000001", "s0000000000000000002"} {
		snap := snapshotFixture(repo)
		snap.ID = id
		if err := db.SaveSnapshot(ctx, snap, elementsFixture(repo), nil); err != nil {
			t.Fatalf("%s: %v", id, err)
		}
	}
	got, err := db.SnapshotForElement(ctx, "cmp:example.com/x/internal/basket.Basket")
	if err != nil {
		t.Fatal(err)
	}
	if got != "s0000000000000000002" {
		t.Fatalf("got %q", got)
	}
	if _, err := db.GetElement(ctx, got, "cmp:example.com/x/internal/basket.Basket"); err != nil {
		t.Fatalf("unknown element: %v", err)
	}
}

func TestConfigGetSet(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	ctx := context.Background()
	if got, err := db.ConfigGet(ctx, "missing"); got != "" || err != nil {
		t.Fatalf("missing key = %q, %v", got, err)
	}
	if err := db.ConfigSet(ctx, "impact.depth", "2"); err != nil {
		t.Fatal(err)
	}
	if err := db.ConfigSet(ctx, "impact.depth", "3"); err != nil {
		t.Fatal(err)
	}
	if got, err := db.ConfigGet(ctx, "impact.depth"); got != "3" || err != nil {
		t.Fatalf("got %q, %v", got, err)
	}
}
```

`GetElement` 在元素不存在时返回 `model.ErrUnknownElement`，不是零值 + nil：Task 13 的 `graph.Analyze` 复用同一个哨兵，Task 18 的抽屉要靠它区分「这个 id 不在快照里」和「读库失败」。哨兵声明加在 `internal/core/model/element.go`：

```go
var ErrUnknownElement = errors.New("model: unknown element")
```

- [ ] **Step 3: 跑测试确认失败**

Run: `go test ./internal/core/store/`
Expected: FAIL，`rowMap`/`str`/`SaveSnapshot` undefined。

- [ ] **Step 4: 实现 rows.go**

`internal/core/store/rows.go`：

```go
package store

import (
	"encoding/json"
	"strings"

	"github.com/egoist/mygo/plugins/sqlite"
)

// The driver materializes whole result sets, so mapping by column name costs
// nothing and survives adding a column (spec W1). Never index by position.
func rowMap(cols []string) map[string]int {
	m := make(map[string]int, len(cols))
	for i, name := range cols {
		m[name] = i
	}
	return m
}

func rowsToMaps(r sqlite.Rows) []map[string]any {
	index := rowMap(r.Columns)
	out := make([]map[string]any, 0, len(r.Values))
	for _, values := range r.Values {
		row := make(map[string]any, len(index))
		for name, i := range index {
			if i < len(values) {
				row[name] = values[i]
			}
		}
		out = append(out, row)
	}
	return out
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func num(v any) float64 {
	switch n := v.(type) {
	case int64:
		return float64(n)
	case float64:
		return n
	default:
		return 0
	}
}

func jsonString(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return string(raw)
}

func stringList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		// A corrupt column is worth a skipped row, not a crashed scan. Store
		// 没有 logger 可用，所以这里只降级不报错。
		return nil
	}
	return out
}

// escapeLike makes a user query literal inside a LIKE pattern.
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}
```

- [ ] **Step 5: 实现 CRUD**

`repo.go`：

```go
package store

import (
	"context"
	"fmt"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

const upsertRepoSQL = `INSERT INTO repos (id, name, local_path, lang) VALUES (?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET name = excluded.name, local_path = excluded.local_path,
	lang = excluded.lang, updated_at = datetime('now')`

func (d *DB) UpsertRepo(ctx context.Context, r model.Repo) error {
	if _, err := d.Execute(ctx, upsertRepoSQL, r.ID, r.Name, r.LocalPath, r.Lang); err != nil {
		return fmt.Errorf("store: upsert repo %s: %w", r.ID, err)
	}
	return nil
}

func (d *DB) Repos(ctx context.Context) ([]model.Repo, error) {
	rows, err := d.Query(ctx, `SELECT id, name, local_path, lang FROM repos ORDER BY name`)
	if err != nil {
		return nil, err
	}
	out := make([]model.Repo, 0, len(rows.Values))
	for _, row := range rowsToMaps(rows) {
		out = append(out, model.Repo{ID: str(row["id"]), Name: str(row["name"]),
			LocalPath: str(row["local_path"]), Lang: str(row["lang"])})
	}
	return out, nil
}
```

`snapshot.go`：

```go
package store

import (
	"context"
	"fmt"

	"github.com/egoist/mygo/plugins/sqlite"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

const (
	// W2: one scan is one transaction. Every statement is pre-computed because
	// the driver takes a static batch.
	insertSnapshotSQL = `INSERT INTO architecture_snapshots
		(id, repo_id, commit_hash, branch, package_errors_json) VALUES (?, ?, ?, ?, ?)`
	selectLatestSnapshotSQL = `SELECT id FROM architecture_snapshots WHERE repo_id = ? ORDER BY id DESC LIMIT 1`
	selectSnapshotByID      = `SELECT id, repo_id, commit_hash, branch, package_errors_json, created_at
		FROM architecture_snapshots WHERE id = ?`
	selectSnapshotByElement = `SELECT snapshot_id FROM arch_elements WHERE element_id = ? ORDER BY snapshot_id DESC LIMIT 1`
)

func (d *DB) SaveSnapshot(ctx context.Context, s model.ArchitectureSnapshot,
	elements []model.Element, relationships []model.Relationship) error {
	stmts := make([]sqlite.Statement, 0, 1+len(elements)+len(relationships))
	stmts = append(stmts, sqlite.Statement{SQL: insertSnapshotSQL,
		Args: []any{s.ID, s.RepoID, s.CommitHash, s.Branch, jsonString(defaultStrings(s.PackageErrors))}})
	for _, e := range elements {
		stmts = append(stmts, sqlite.Statement{SQL: insertElementSQL,
			Args: []any{s.ID, e.ID, string(e.Type), e.Name, e.Description, e.RepoID, e.FilePath,
				jsonString(defaultStrings(e.Methods))}})
	}
	for _, r := range relationships {
		weight := r.Weight
		if weight < 1 {
			weight = 1
		}
		stmts = append(stmts, sqlite.Statement{SQL: insertRelationshipSQL,
			Args: []any{s.ID, r.FromID, r.ToID, string(r.Type), weight}})
	}
	if _, err := d.Transact(ctx, stmts); err != nil {
		return fmt.Errorf("store: save snapshot %s (%d statements): %w", s.ID, len(stmts), err)
	}
	return nil
}

func defaultStrings(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

func (d *DB) LatestSnapshotID(ctx context.Context, repoID string) (string, error) {
	rows, err := d.Query(ctx, selectLatestSnapshotSQL, repoID)
	if err != nil {
		return "", err
	}
	// No snapshot yet is a normal state (a repo the user registered but never
	// scanned), not an error: callers branch on the empty string.
	if len(rows.Values) == 0 {
		return "", nil
	}
	return str(rows.Values[0][0]), nil
}

// Snapshot reads one snapshot's own row. The UI's status line needs
// PackageErrors (spec §10: "完成，N 个包有错误"), which no other read path
// carries.
func (d *DB) Snapshot(ctx context.Context, snapshotID string) (model.ArchitectureSnapshot, error) {
	rows, err := d.Query(ctx, selectSnapshotByID, snapshotID)
	if err != nil {
		return model.ArchitectureSnapshot{}, err
	}
	out := rowsToMaps(rows)
	if len(out) == 0 {
		return model.ArchitectureSnapshot{}, fmt.Errorf("store: unknown snapshot %q", snapshotID)
	}
	row := out[0]
	return model.ArchitectureSnapshot{
		ID:            str(row["id"]),
		RepoID:        str(row["repo_id"]),
		CommitHash:    str(row["commit_hash"]),
		Branch:        str(row["branch"]),
		PackageErrors: stringList(str(row["package_errors_json"])),
		CreatedAt:     str(row["created_at"]),
	}, nil
}

// SnapshotForElement is what the CLI uses: an element id alone, latest snapshot
// that still carries it.
func (d *DB) SnapshotForElement(ctx context.Context, elementID string) (string, error) {
	rows, err := d.Query(ctx, selectSnapshotByElement, elementID)
	if err != nil {
		return "", err
	}
	if len(rows.Values) == 0 {
		return "", model.ErrUnknownElement
	}
	return str(rows.Values[0][0]), nil
}
```

`element.go`：

```go
package store

import (
	"context"
	"fmt"

	"github.com/egoist/mygo/plugins/sqlite"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

const (
	insertElementSQL = `INSERT INTO arch_elements
		(snapshot_id, element_id, element_type, name, description, repo_id, file_path, methods_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	elementColumns = `element_id, element_type, name, description, repo_id, file_path, methods_json`
	// LIKE needs ESCAPE because we escape the pattern ourselves.
	selectElementsBySnapshot = `SELECT ` + elementColumns + ` FROM arch_elements
		WHERE snapshot_id = ? ORDER BY element_id`
	selectElementByID = `SELECT ` + elementColumns + ` FROM arch_elements
		WHERE snapshot_id = ? AND element_id = ?`
	selectElementSearch = `SELECT ` + elementColumns + ` FROM arch_elements
		WHERE snapshot_id = ? AND name LIKE ? ESCAPE '\' ORDER BY name LIMIT ?`
)

// maxSearchResults 是命令面板的取回上限（Task 20 的输入不做本地二次截断）。
// 走 `LIMIT ?` 而不是拼进 SQL：全局约束只允许 `PRAGMA user_version` 一处插值。
const maxSearchResults = 50

func elementsFromRows(rows sqlite.Rows) []model.Element {
	out := make([]model.Element, 0, len(rows.Values))
	for _, row := range rowsToMaps(rows) {
		out = append(out, model.Element{
			ID:          str(row["element_id"]),
			Type:        model.ElementType(str(row["element_type"])),
			Name:        str(row["name"]),
			Description: str(row["description"]),
			RepoID:      str(row["repo_id"]),
			FilePath:    str(row["file_path"]),
			Methods:     stringList(str(row["methods_json"])),
		})
	}
	return out
}

func (d *DB) Elements(ctx context.Context, snapshotID string) ([]model.Element, error) {
	rows, err := d.Query(ctx, selectElementsBySnapshot, snapshotID)
	if err != nil {
		return nil, fmt.Errorf("store: elements: %w", err)
	}
	return elementsFromRows(rows), nil
}

func (d *DB) GetElement(ctx context.Context, snapshotID, elementID string) (model.Element, error) {
	rows, err := d.Query(ctx, selectElementByID, snapshotID, elementID)
	if err != nil {
		return model.Element{}, err
	}
	out := elementsFromRows(rows)
	if len(out) == 0 {
		return model.Element{}, model.ErrUnknownElement
	}
	return out[0], nil
}

func (d *DB) SearchElements(ctx context.Context, snapshotID, query string) ([]model.Element, error) {
	rows, err := d.Query(ctx, selectElementSearch, snapshotID, "%"+escapeLike(query)+"%", maxSearchResults)
	if err != nil {
		return nil, fmt.Errorf("store: search %q: %w", query, err)
	}
	return elementsFromRows(rows), nil
}
```

`relationship.go`：

```go
package store

import (
	"context"
	"fmt"

	"github.com/egoist/mygo/plugins/sqlite"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

const (
	insertRelationshipSQL = `INSERT INTO arch_relationships
		(snapshot_id, from_id, to_id, relationship_type, weight) VALUES (?, ?, ?, ?, ?)`
	selectRelsBySnapshot = `SELECT from_id, to_id, relationship_type, weight FROM arch_relationships
		WHERE snapshot_id = ? ORDER BY from_id, to_id, relationship_type`
)

func (d *DB) Relationships(ctx context.Context, snapshotID string) ([]model.Relationship, error) {
	rows, err := d.Query(ctx, selectRelsBySnapshot, snapshotID)
	if err != nil {
		return nil, fmt.Errorf("store: relationships: %w", err)
	}
	out := make([]model.Relationship, 0, len(rows.Values))
	for _, row := range rowsToMaps(rows) {
		out = append(out, model.Relationship{
			FromID: str(row["from_id"]), ToID: str(row["to_id"]),
			Type: model.RelType(str(row["relationship_type"])), Weight: int(num(row["weight"])),
		})
	}
	return out, nil
}
```

排序键是 `from_id, to_id, relationship_type`。`TestSaveSnapshotRoundTrip` 里断言的 `rels[0]` 是 `calls`（from `cmp:...`，`c` < `s`，因为 SystemID 是 `sys:example.com/x`，`cmp:` 在前）—— 与实现一致。

`config.go`：

```go
package store

import (
	"context"
	"database/sql"
)
```

——不要 `database/sql`，本包不用它。`config.go` 全文：

```go
package store

import "context"

const (
	upsertConfigSQL = `INSERT INTO app_config (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`
	selectConfigSQL = `SELECT value FROM app_config WHERE key = ?`
)

func (d *DB) ConfigGet(ctx context.Context, key string) (string, error) {
	rows, err := d.Query(ctx, selectConfigSQL, key)
	if err != nil {
		return "", err
	}
	if len(rows.Values) == 0 {
		return "", nil
	}
	return str(rows.Values[0][0]), nil
}

func (d *DB) ConfigSet(ctx context.Context, key, value string) error {
	_, err := d.Execute(ctx, upsertConfigSQL, key, value)
	return err
}
```

- [ ] **Step 6: 跑测试确认通过**

Run: `go test ./internal/core/store/ -v`
Expected: 全 PASS。`TestSearchElementsEscapesWildcards` 若失败，是 `ESCAPE '\'` 里的反斜杠被 Go 字符串吃掉——常量里必须写 `ESCAPE '\'`（Go 源文本中是 `ESCAPE '\\'`），检查 SQL 常量。

- [ ] **Step 7: 写 PoC-5 基准并跑**

`internal/core/store/bench_test.go`：

```go
package store

import (
	"context"
	"fmt"
	"testing"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

// BenchmarkSaveSnapshot is PoC-5's re-runnable measurement: a spec §6.4 sized
// snapshot is 600 components + 1 System + containers ≈ 620 elements and about
// 1500 relationships, i.e. ~2100 statements in one transaction.
func BenchmarkSaveSnapshot(b *testing.B) {
	ctx := context.Background()
	db := newTestDB(b)
	defer db.Close()
	repo := model.Repo{ID: "repo:/bench", Name: "bench", LocalPath: "/bench", Lang: "go"}
	if err := db.UpsertRepo(ctx, repo); err != nil {
		b.Fatal(err)
	}
	const elements, relationships = 620, 1500
	els := make([]model.Element, 0, elements)
	for i := 0; i < elements; i++ {
		els = append(els, model.Element{
			ID: fmt.Sprintf("cmp:pkg%d.Type%d", i/10, i%10), Type: model.TypeComponent,
			Name: fmt.Sprintf("Type%d", i), RepoID: repo.ID, FilePath: "pkg/file.go",
			Methods: []string{"Run", "Stop"},
		})
	}
	rels := make([]model.Relationship, 0, relationships)
	for i := 0; i < relationships; i++ {
		rels = append(rels, model.Relationship{
			FromID: fmt.Sprintf("cmp:pkg%d.Type%d", i/10, i%10),
			ToID:   fmt.Sprintf("cmp:pkg%d.Type%d", (i+7)%elements/10, (i+7)%elements%10),
			Type:   model.RelCalls, Weight: 1 + i%9,
		})
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		snap := model.ArchitectureSnapshot{ID: fmt.Sprintf("s%019d", i + 1), RepoID: repo.ID}
		if err := db.SaveSnapshot(ctx, snap, els, rels); err != nil {
			b.Fatal(err)
		}
	}
}
```

`newTestDB(b)` 收的是 `*testing.T`。把它改成泛型或 `testing.TB`：`func newTestDB(t testing.TB) *DB`（Task 2 定义处一并改，`t.Helper()` 保留）。

Run: `go test ./internal/core/store/ -bench BenchmarkSaveSnapshot -benchtime 20x -run '^$'`
Expected: 一次 `SaveSnapshot` 的耗时打印。把它记进 findings 的 `## PoC-5`。**门槛：单事务 2100 条不失败**，且耗时（b.N=1 时）你能接受；失败或明显过慢就停下报告（分块提交的分支要用户拍板）。

- [ ] **Step 8: 提交**

```sh
go vet ./... && go test ./...
git add internal/core/store/ docs/superpowers/poc/2026-10-19-slice1-findings.md
git commit -m "feat: 存储层行映射与快照单事务写入，实测 2100 语句事务（PoC-5）"
```

---

### Task 8: LanguageAdapter 接口与注册表

**Files:**
- Create: `internal/scanner/language/adapter.go`
- Test: `internal/scanner/language/adapter_test.go`

**Interfaces:**
- Consumes: Task 5 的 model
- Produces:
  - `type language.Adapter interface { Language() string; Detect(root string) (bool, error); Parse(ctx context.Context, root string) (language.Output, error) }`
  - `type language.Output struct { ModulePath string; Elements []model.Element; Relationships []model.Relationship; PackageErrors []string }`
  - `func language.Register(a Adapter)`（重复注册 panic）
  - `func language.All() []Adapter`（按 `Language()` 升序）
  - `func language.Get(name string) (Adapter, bool)`

- [ ] **Step 1: 写失败测试**

`internal/scanner/language/adapter_test.go`：

```go
package language

import (
	"context"
	"errors"
	"testing"
)

type stub struct {
	name    string
	language string
	detect  bool
	err     error
}

func (s stub) Language() string { return s.language }
func (s stub) Detect(string) (bool, error) {
	return s.detect, s.err
}
func (s stub) Parse(context.Context, string) (Output, error) {
	return Output{ModulePath: s.name}, nil
}

func resetRegistry(t *testing.T) {
	t.Helper()
	previous := registry
	registry = map[string]Adapter{}
	t.Cleanup(func() { registry = previous })
}

func TestAllSortedAndGet(t *testing.T) {
	resetRegistry(t)
	Register(stub{language: "go"})
	Register(stub{language: "python"})
	Register(stub{language: "java"})
	got := All()
	if len(got) != 3 || got[0].Language() != "go" || got[1].Language() != "java" || got[2].Language() != "python" {
		t.Fatalf("All = %v", names(got))
	}
	if a, ok := Get("java"); !ok || a.Language() != "java" {
		t.Fatalf("Get(java) = %v %v", a, ok)
	}
	if _, ok := Get("rust"); ok {
		t.Fatal("rust must be absent")
	}
}

func TestRegisterRejectsDuplicates(t *testing.T) {
	resetRegistry(t)
	Register(stub{language: "go"})
	defer func() {
		if recover() == nil {
			t.Fatal("duplicate registration must panic")
		}
	}()
	Register(stub{language: "go"})
}

func TestDetectReportsUnreadableSeparatelyFromUnsupported(t *testing.T) {
	// 偏差 11 的目的：UI 要对这两种情况给不同文案。
	ok, err := stub{language: "go", detect: false}.Detect("/anywhere")
	if ok || err != nil {
		t.Fatalf("not-go = %v %v", ok, err)
	}
	if _, err := stub{language: "go", err: errors.New("permission denied")}.Detect("/nope"); err == nil {
		t.Fatal("unreadable must surface an error")
	}
}

func names(adapters []Adapter) []string {
	out := make([]string, 0, len(adapters))
	for _, a := range adapters {
		out = append(out, a.Language())
	}
	return out
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/scanner/language/`
Expected: `undefined: Adapter`。

- [ ] **Step 3: 实现**

`internal/scanner/language/adapter.go`：

```go
// Package language defines the pluggable project parser. Slice 1 registers
// only Go; later languages arrive as sidecar adapters, never cgo (spec §3.6).
package language

import (
	"context"
	"sort"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

type Output struct {
	ModulePath      string
	Elements        []model.Element
	Relationships   []model.Relationship
	PackageErrors   []string
}

type Adapter interface {
	// Language is the registry key, lowercase, e.g. "go".
	Language() string
	// Detect reports whether root is a project of this language. A non-nil
	// error means the directory could not be read at all, which the UI must
	// phrase differently from "not this language".
	Detect(root string) (bool, error)
	Parse(ctx context.Context, root string) (Output, error)
}

var registry = map[string]Adapter{}

func Register(a Adapter) {
	if _, taken := registry[a.Language()]; taken {
		panic("language: adapter registered twice: " + a.Language())
	}
	registry[a.Language()] = a
}

func All() []Adapter {
	out := make([]Adapter, 0, len(registry))
	for _, a := range registry {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Language() < out[j].Language() })
	return out
}

func Get(name string) (Adapter, bool) {
	a, ok := registry[name]
	return a, ok
}
```

`Register` 只在 `init` 里调用（Task 9），因此并发写不是问题；包注释里说明这一点即可，不要加锁。

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/scanner/language/ -v`
Expected: 4 个测试 PASS。

- [ ] **Step 5: 提交**

```sh
go vet ./... && go test ./...
git add internal/scanner/language/adapter.go internal/scanner/language/adapter_test.go
git commit -m "feat: 语言适配器接口与注册表"
```

---

### Task 9: Go adapter —— 元素抽取

**Files:**
- Create: `internal/scanner/language/go.go`
- Create: `testdata/fixtures/simple-multi-main/{go.mod,cmd/api/main.go,cmd/worker/main.go,internal/store/store.go,internal/store/store_test.go,doc.go}`
- Create: `testdata/fixtures/shared-lib/{go.mod,cmd/app/main.go,core/core.go,edge/edge.go}`
- Create: `testdata/fixtures/generic-and-iface/{go.mod,box/box.go,iface/iface.go,unexported/u.go}`
- Test: `internal/scanner/language/go_test.go`

**Interfaces:**
- Consumes: Task 8 的 `Adapter`/`Output`/`Register`；Task 3 的 `packagesNeedAll`（把该常量从探针搬到本文件，探针改成本地复制一份并注明）；Task 5 的 ID 规则
- Produces:
  - `type goAdapter struct{}` + `func init() { Register(goAdapter{}) }`
  - `func (goAdapter) Language() string { return "go" }`
  - `func (goAdapter) Detect(root string) (bool, error)` —— `go.mod` 存在
  - `func (goAdapter) Parse(ctx context.Context, root string) (Output, error)`
  - `func (goAdapter) ModulePath(root string) (string, error)`（Task 11 的 scanner 用它给 `Repo.Name`）
  - 内部：`load(ctx, root) ([]*packages.Package, string, error)`、`ownPackage(modulePath, pkgPath) bool`、`exportedComponents(pkg) []types.Named 信息`、`methodNames(*types.Named) []string`

- [ ] **Step 1: 建三个 fixture**

`testdata/fixtures/simple-multi-main/go.mod`：

```
module example.com/simple

go 1.27
```

`testdata/fixtures/simple-multi-main/doc.go`：

```go
// Package simple is the System root of this fixture.
package simple
```

`testdata/fixtures/simple-multi-main/internal/store/store.go`：

```go
// Package store keeps the fixture's shared data.
package store

// Cart is shared by both commands.
type Cart struct {
	Items []string
}

// Add appends an item.
func (c *Cart) Add(name string) { c.Items = append(c.Items, name) }

// Total reports how many items were added.
func (c *Cart) Total() int { return len(c.Items) }

// unexported must never become an element.
type hidden struct{}

func newHidden() hidden { return hidden{} }
```

`testdata/fixtures/simple-multi-main/cmd/api/main.go`：

```go
package main

import "example.com/simple/internal/store"

func main() {
	c := &store.Cart{}
	c.Add("a")
	_ = c.Total()
}
```

`testdata/fixtures/simple-multi-main/cmd/worker/main.go`：

```go
package main

import "example.com/simple/internal/store"

func main() {
	_ = (&store.Cart{}).Total()
}
```

`testdata/fixtures/simple-multi-main/internal/store/store_test.go`：

```go
package store

import "testing"

func TestCart(t *testing.T) {
	c := &Cart{}
	c.Add("x")
	if c.Total() != 1 {
		t.Fatal("bad")
	}
}
```

`testdata/fixtures/shared-lib/go.mod`：

```
module example.com/shared

go 1.27
```

`testdata/fixtures/shared-lib/core/core.go`：

```go
// Package core imports edge, and edge imports core: a real package cycle,
// which the containment walk must terminate on.
package core

import "example.com/shared/edge"

type Server struct{ Port int }

func (s *Server) Addr() string { return s.Port_string() }

func (s *Server) Port_string() string { return "core" + edge.Version() }
```

`Port_string` 是合法 Go 标识符但会被 go vet 当测试辅助命名？不会。**简化它**，避免无谓的细节：

```go
package core

import "example.com/shared/edge"

type Server struct{ Port int }

func (s *Server) Addr() string { return edge.Version() }
```

`testdata/fixtures/shared-lib/edge/edge.go`：

```go
package edge

import "example.com/shared/core"

type Client struct{ Target core.Server }

func Version() string { return "1" }
```

`testdata/fixtures/shared-lib/cmd/app/main.go`：

```go
package main

import (
	"example.com/shared/core"
	"example.com/shared/edge"
)

func main() {
	_ = core.Server{}
	_ = edge.Client{}
}
```

`shared-lib` 不放 `_test.go`：`_test.go` 的排除已在 `simple-multi-main/internal/store/store_test.go` 验过，一个 fixture 只测一件事，`TestSharedLibContainment` 的元素清单才不会混进测试文件里的类型。

`testdata/fixtures/generic-and-iface/go.mod`：

```
module example.com/generic

go 1.27
```

`testdata/fixtures/generic-and-iface/box/box.go`：

```go
// Package box holds a generic component: one node, parameters dropped (§6.2).
package box

type Box[T any] struct{ Value T }

func (b *Box[T]) Get() T { return b.Value }

type Pair[T, U any] struct {
	First  T
	Second U
}

func (p *Pair[T, U]) Swap() { p.First, p.Second = p.Second, p.First }
```

`testdata/fixtures/generic-and-iface/iface/iface.go`：

```go
package iface

type Reader interface {
	Read() string
}

// The conventional assertion is the only statically reliable "implements"
// signal, which is why implements edges were dropped (spec §3.2).
var _ Reader = String("")

type String string

func (s String) Read() string { return string(s) }
```

`testdata/fixtures/generic-and-iface/unexported/u.go`：

```go
package unexported

type notAComponent struct{}

func use() notAComponent { return notAComponent{} }
```

- [ ] **Step 2: 写失败测试**

`internal/scanner/language/go_test.go`：

```go
package language

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

func parseFixture(t *testing.T, name string) Output {
	t.Helper()
	out, err := goAdapter{}.Parse(context.Background(), fixturesDir(t)+"/"+name)
	if err != nil {
		t.Fatalf("Parse(%s): %v", name, err)
	}
	return out
}

func fixturesDir(t *testing.T) string {
	t.Helper()
	return "../../../testdata/fixtures"
}

func idsOf(elements []model.Element) []string {
	out := make([]string, 0, len(elements))
	for _, e := range elements {
		out = append(out, e.ID)
	}
	slices.Sort(out)
	return out
}

func TestSimpleMultiMainElements(t *testing.T) {
	out := parseFixture(t, "simple-multi-main")
	want := []string{
		// Cart 被两个 main 传递引用 → 挂 System（§6.3 的多归属规则）。
		"cmp:example.com/simple/internal/store.Cart",
		"ctn:example.com/simple/cmd/api",
		"ctn:example.com/simple/cmd/worker",
		"sys:example.com/simple",
	}
	if got := idsOf(out.Elements); !slices.Equal(got, want) {
		t.Fatalf("elements =\n%v\nwant\n%v", got, want)
	}
	for _, e := range out.Elements {
		if e.ID != "cmp:example.com/simple/internal/store.Cart" {
			continue
		}
		if e.Name != "Cart" || e.Description != "Cart is shared by both commands." {
			t.Fatalf("element = %+v", e)
		}
		if !slices.Equal(e.Methods, []string{"Add", "Total"}) {
			t.Fatalf("methods = %v", e.Methods)
		}
		if e.FilePath != "internal/store/store.go" {
			t.Fatalf("file path = %q", e.FilePath)
		}
	}
	if slices.Contains(strings.Join(idsOf(out.Elements), " "), "hidden") {
		t.Fatal("unexported type leaked into elements")
	}
}

func TestSharedLibContainment(t *testing.T) {
	out := parseFixture(t, "shared-lib")
	// 只有 cmd/app 一个 main：core 与 edge 都挂它，循环依赖不能引起死循环。
	want := []string{"cmp:example.com/shared/core.Server", "cmp:example.com/shared/edge.Client", "ctn:example.com/shared/cmd/app", "sys:example.com/shared"}
	if got := idsOf(out.Elements); !slices.Equal(got, want) {
		t.Fatalf("elements =\n%v\nwant\n%v", got, want)
	}
	var contains []string
	for _, r := range out.Relationships {
		if r.Type == model.RelContains {
			contains = append(contains, r.FromID+" -> "+r.ToID)
		}
	}
	slices.Sort(contains)
	wantContains := []string{
		"ctn:example.com/shared/cmd/app -> cmp:example.com/shared/core.Server",
		"ctn:example.com/shared/cmd/app -> cmp:example.com/shared/edge.Client",
		"sys:example.com/shared -> ctn:example.com/shared/cmd/app",
	}
	if !slices.Equal(contains, wantContains) {
		t.Fatalf("contains =\n%v\nwant\n%v", contains, wantContains)
	}
}

func TestGenericAndInterfaceElements(t *testing.T) {
	out := parseFixture(t, "generic-and-iface")
	want := []string{
		"cmp:example.com/generic/box.Box", "cmp:example.com/generic/box.Pair",
		"cmp:example.com/generic/iface.Reader", "cmp:example.com/generic/iface.String",
		"sys:example.com/generic",
	}
	if got := idsOf(out.Elements); !slices.Equal(got, want) {
		t.Fatalf("elements =\n%v\nwant\n%v", got, want)
	}
	for _, e := range out.Elements {
		if strings.HasPrefix(e.ID, "ctn:") {
			t.Fatalf("fixture has no main package, got container %q", e.ID)
		}
	}
	// 无 main 包：没有任何 Component 该挂到 Container 上。
	for _, r := range out.Relationships {
		if r.Type == model.RelContains && strings.HasPrefix(r.FromID, "ctn:") {
			t.Fatalf("unexpected container edge %+v", r)
		}
	}
}

func TestDetect(t *testing.T) {
	root := fixturesDir(t) + "/generic-and-iface"
	ok, err := goAdapter{}.Detect(root)
	if !ok || err != nil {
		t.Fatalf("Detect(go fixture) = %v %v", ok, err)
	}
	ok, err = goAdapter{}.Detect(fixturesDir(t))
	if ok || err != nil {
		t.Fatalf("Detect(fixtures dir, no go.mod) = %v %v", ok, err)
	}
	if _, err := goAdapter{}.Detect("/definitely/not/here"); err == nil {
		t.Fatal("unreadable root must return an error")
	}
}
```

`Detect("/definitely/not/here")` 是否返回 error 取决于实现——`os.Stat` 的 `ErrNotExist` 按本测试的意图应算「读不了」。这条与 §11 的 `ErrUnreadable` 一致，实现里把 `!os.IsNotExist(err)` 之类的花活省掉：**只要 `os.Stat` 非 nil 错误就往外传**，包括 NotExist。

- [ ] **Step 3: 跑测试确认失败**

Run: `go test ./internal/scanner/language/ -run 'Simple|Shared|Generic|Detect'`
Expected: `undefined: goAdapter`。

- [ ] **Step 4: 实现解析**

`internal/scanner/language/go.go`（依赖 `golang.org/x/tools`，Step 5 的 `go get` 拉它；版本由 mygo/onevra 的 `go.sum` 决定，不手工 pin）：

```go
package language

import (
	"bufio"
	"context"
	"fmt"
	"go/ast"
	"go/types"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

func init() { Register(goAdapter{}) }

type goAdapter struct{}

func (goAdapter) Language() string { return "go" }

// packagesNeedAll is the LoadMode PoC-3 settled on (spec U2). It matches
// tools/parseprobe, which keeps a copy because it must stay runnable as the
// measurement harness.
const packagesNeedAll = packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
	packages.NeedTypes | packages.NeedTypesInfo | packages.NeedDeps | packages.NeedModule

func (goAdapter) Detect(root string) (bool, error) {
	info, err := os.Stat(filepath.Join(root, "go.mod"))
	if err != nil {
		return false, err // includes ErrNotExist: an absent go.mod is "unreadable" here
	}
	return !info.IsDir(), nil
}

func (goAdapter) ModulePath(root string) (string, error) {
	file, err := os.Open(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 2 && fields[0] == "module" {
			return strings.Trim(fields[1], `"'`), nil
		}
	}
	return "", fmt.Errorf("language: no module line in %s/go.mod", root)
}

func (a goAdapter) Parse(ctx context.Context, root string) (Output, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return Output{}, err
	}
	pkgs, modulePath, err := load(ctx, abs)
	if err != nil {
		return Output{}, err
	}
	repoID := model.RepoIDFor(abs)

	var (
		elements      []model.Element
		packageErrors []string
		comps         = map[string]*component{} // element id -> component
	)
	containers := map[string]bool{} // pkgPath of packages named main

	for _, pkg := range pkgs {
		for _, e := range pkg.Errors {
			packageErrors = append(packageErrors, fmt.Sprintf("%s: %s", pkg.PkgPath, e))
		}
		if isTestVariant(pkg.ID) || !ownPackage(modulePath, pkg.PkgPath) {
			continue
		}
		if pkg.IllTyped {
			packageErrors = append(packageErrors, fmt.Sprintf("%s: package has type errors", pkg.PkgPath))
		}
		if pkg.Name == "main" {
			containers[pkg.PkgPath] = true
			elements = append(elements, model.Element{
				ID: model.ContainerID(pkg.PkgPath), Type: model.TypeContainer,
				Name: filepath.Base(pkg.PkgPath), RepoID: repoID,
			})
			continue
		}
		for _, named := range exportedComponentTypes(pkg) {
			id := model.ComponentID(pkg.PkgPath, named.Obj().Name())
			c := &component{pkgPath: pkg.PkgPath, named: named}
			comps[id] = c
			elements = append(elements, model.Element{
				ID: id, Type: model.TypeComponent, Name: named.Obj().Name(),
				Description: named.Obj().Doc(), RepoID: repoID,
				FilePath: relativeFile(abs, pkg, named.Obj().Pos()),
				Methods:  methodNames(named),
			})
		}
	}

	out := Output{ModulePath: modulePath, Elements: elements, PackageErrors: packageErrors}
	out.Relationships = relationships(modulePath, pkgs, comps, containers)
	return out, nil
}

// component is a named type the scanner treats as a C4 Component. It carries
// only what the edge rules need: where it lives, and the type itself.
type component struct {
	pkgPath string
	named   *types.Named
}

func exportedComponentTypes(pkg *packages.Package) []*types.Named {
	if pkg.Types == nil {
		return nil
	}
	var out []*types.Named
	scope := pkg.Types.Scope()
	for _, name := range scope.Names() {
		tn, ok := scope.Lookup(name).(*types.TypeName)
		if !ok || !tn.Exported() {
			continue
		}
		named, ok := tn.Type().(*types.Named)
		if !ok {
			continue
		}
		switch named.Underlying().(type) {
		case *types.Struct, *types.Interface:
			out = append(out, named)
		}
	}
	return out
}

func methodNames(named *types.Named) []string {
	set := types.NewMethodSet(types.NewPointer(named))
	out := make([]string, 0, set.Len())
	for i := 0; i < set.Len(); i++ {
		method, ok := set.At(i).Obj().(*types.Func)
		if !ok || !method.Exported() {
			continue
		}
		out = append(out, method.Name())
	}
	slices.Sort(out)
	return out
}

func ownPackage(modulePath, pkgPath string) bool {
	return pkgPath == modulePath || strings.HasPrefix(pkgPath, modulePath+"/")
}

// Test variants carry a bracketed id such as "main [example.com/x.test]".
func isTestVariant(id string) bool {
	return strings.Contains(id, ".test]") || strings.Contains(id, "-test]") || strings.Contains(id, ".coverpkg")
}

func relativeFile(abs string, pkg *packages.Package, pos token.Pos) string {
	if !pos.IsValid() || pkg.Fset == nil {
		return ""
	}
	full := pkg.Fset.Position(pos).Filename
	rel, err := filepath.Rel(abs, full)
	if err != nil {
		return full
	}
	return rel
}
```

`methodNames` 用 `slices`，`relativeFile` 与 `token.Pos` 签名用 `go/token`：import 补齐 `"go/token"`、`"slices"`（`Detect` 不碰 `errors` 包，别顺手加进来，否则这个包在 `go.go` 里是未使用 import）。

`Detect` 把「go.mod 不存在」和「go.mod 读不了（权限、IO 故障）」合成一个 `false, err` 是有意的：R2 的降级语义是**这个目录不能被当作 Go 模块解析**，两种原因对用户的后果一样（Task 11 一律记成一条 package error 并落到只读 CLI）。把两种情况拆成 `false, nil` / `false, err` 只会逼调用方为区分它们做第二遍 `os.Stat`，而第二次 `Stat` 的结果和第一次之间根本没有保证。

`load` 放同文件：

```go
func load(ctx context.Context, root string) ([]*packages.Package, string, error) {
	cfg := &packages.Config{Mode: packagesNeedAll, Dir: root, Context: ctx}
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return nil, "", fmt.Errorf("language: load %s: %w", root, err)
	}
	modulePath := ""
	for _, pkg := range pkgs {
		if pkg.Module != nil {
			modulePath = pkg.Module.Path
			break
		}
	}
	if modulePath == "" {
		return nil, "", fmt.Errorf("language: no module under %s", root)
	}
	return pkgs, modulePath, nil
}
```

`ctx` 取消时 `packages.Load` 会返回 `context.Canceled`（它把 cfg.Context 传给 go list），`Parse` 随之返回错误——这就是 §10 取消路径的实现基础。

`relationships(...)` 本任务先给一个**只产生 contains 边**的最小实现（Task 10 补 depends_on/calls，函数签名一次定下来）：

```go
// relationships derives the edges. Task 10 adds depends_on and calls.
func relationships(modulePath string, pkgs []*packages.Package,
	comps map[string]*component, containers map[string]bool) []model.Relationship {
	var acc []model.Relationship
	// System -> every Container.
	for pkgPath := range containers {
		acc = append(acc, model.Relationship{
			FromID: model.SystemID(modulePath), ToID: model.ContainerID(pkgPath),
			Type: model.RelContains, Weight: 1,
		})
	}
	// Component -> exactly one owner: the sole Container that reaches it, else System.
	for _, owner := range componentOwners(modulePath, pkgs, comps, containers) {
		acc = append(acc, model.Relationship{FromID: owner.from, ToID: owner.toID, Type: model.RelContains, Weight: 1})
	}
	return dedup(acc)
}

type ownership struct{ from, toID string }

func componentOwners(modulePath string, pkgs []*packages.Package,
	comps map[string]*component, containers map[string]bool) []ownership {
	reaching := map[string]map[string]bool{} // component pkgPath -> container pkgPaths
	for container := range containers {
		seen := map[string]bool{}
		var walk func(pkgPath string)
		walk = func(pkgPath string) {
			if seen[pkgPath] {
				return
			}
			seen[pkgPath] = true
			for _, imported := range importsOf(pkgs, pkgPath) {
				walk(imported)
			}
		}
		walk(container)
		delete(seen, container)
		for pkgPath := range seen {
			if reaching[pkgPath] == nil {
				reaching[pkgPath] = map[string]bool{}
			}
			reaching[pkgPath][container] = true
		}
	}

	out := make([]ownership, 0, len(comps))
	for id, comp := range comps {
		owners := reaching[comp.pkgPath]
		if len(owners) == 1 {
			for container := range owners {
				out = append(out, ownership{
					from: model.ContainerID(container), toID: id})
			}
			continue
		}
		out = append(out, ownership{from: model.SystemID(modulePath), toID: id})
	}
	slices.SortFunc(out, func(a, b ownership) int { return strings.Compare(a.from+a.toID, b.from+b.toID) })
	return out
}
```

owner 规则写在代码里：一个 Component 只有一条 contains 入边。能被**唯一**一个 Container 传递导入到就挂在那个 Container 下；被多个 Container 共用（或没人导入，比如只被测试引用的包）就挂到 System。这样图上不会有重复归属，也不会有孤儿节点。末尾的 `slices.SortFunc` 是必需的——`comps` 和 `owners` 都是 map，不排序则同一份代码每次扫描产出的边顺序都不同，Task 11 的 golden 比对会红（全局约束里"与 golden 文件比对前必须排序"那条），单测里也没法写死的期望值。

`importsOf`：

```go
func importsOf(pkgs []*packages.Package, pkgPath string) []string {
	for _, pkg := range pkgs {
		if pkg.PkgPath != pkgPath || pkg.Imports == nil {
			continue
		}
		out := make([]string, 0, len(pkg.Imports))
		for _, imported := range pkg.Imports {
			if imported != nil {
				out = append(out, imported.PkgPath)
			}
		}
		slices.Sort(out)
		return out
	}
	return nil
}
```

`dedup` 放同文件（Task 10 也用它）：

```go
// dedup merges the (from,to,type) triple into one edge and sums Weight, then
// sorts so golden comparisons are stable (§11).
func dedup(in []model.Relationship) []model.Relationship {
	type key struct{ from, to string; kind model.RelType }
	index := map[key]int{}
	out := []model.Relationship{}
	for _, rel := range in {
		k := key{rel.FromID, rel.ToID, rel.Type}
		if i, ok := index[k]; ok {
			out[i].Weight += rel.Weight
			continue
		}
		index[k] = len(out)
		weight := rel.Weight
		if weight < 1 {
			weight = 1
		}
		out = append(out, model.Relationship{FromID: rel.FromID, ToID: rel.ToID, Type: rel.Type, Weight: weight})
	}
	slices.SortFunc(out, func(a, b model.Relationship) int {
		if c := strings.Compare(a.FromID, b.FromID); c != 0 {
			return c
		}
		if c := strings.Compare(a.ToID, b.ToID); c != 0 {
			return c
		}
		return strings.Compare(string(a.Type), string(b.Type))
	})
	return out
}
```

- [ ] **Step 5: 拉依赖并跑测试**

```sh
go get golang.org/x/tools@v0.51.0
go test ./internal/scanner/language/ -v
```

`golang.org/x/tools` 的版本以 `go get` 解析到的、onevra/mygo 的 go.sum 里已有的那个为准（onevra 的依赖树里是 v0.51.0）；**若 go.mod 里已间接存在，就不要显式 pin，直接 import 让 `go mod tidy` 补上**：`go mod tidy && go test ./internal/scanner/language/ -v`。

Expected: `TestSimpleMultiMainElements`、`TestSharedLibContainment`、`TestGenericAndInterfaceElements`、`TestDetect` 全 PASS。`TestAllSortedAndGet` 等 Task 8 的测试仍 PASS（fixture 目录里没有 `go.mod` 之外的干扰）。

fixture 能被 `go/packages` 解析的前提：`go` 命令忽略路径里名为 `testdata` 的目录元素，因此 fixture 自带 go.mod 就自成模块。若 `packages.Load` 报 cannot find module，先用 `cd testdata/fixtures/simple-multi-main && go list ./...` 手工确认，问题多半是 GOFLAGS 或 GOPROXY 而非 fixture 结构。

- [ ] **Step 6: 提交**

```sh
go vet ./... && go test ./...
git add internal/scanner/language/go.go internal/scanner/language/go_test.go testdata/fixtures/ go.mod go.sum
git commit -m "feat: Go 语言适配器抽取 System/Container/Component 与 contains 边"
```

---

### Task 10: depends_on 与 calls 边

**Files:**
- Modify: `internal/scanner/language/go.go`（`component` 加 `id` 字段；新增归属与两类边；`relationships` 接线）
- Modify: `testdata/fixtures/shared-lib/core/core.go`、`testdata/fixtures/shared-lib/edge/edge.go`
- Modify: `internal/scanner/language/go_test.go`（`TestSharedLibContainment` 的期望元素表 + 新增三个测试）

**Interfaces:**
- Consumes: Task 9 的 `component`/`comps`/`dedup`/`ownPackage`
- Produces:
  - `func componentIDFor(modulePath string, named *types.Named) (string, bool)`
  - `func edgeOwnership(modulePath string, pkg *packages.Package, byPackage map[string][]*component, pos token.Pos) (string, bool)`
  - `func dependsOnEdges(modulePath string, pkg *packages.Package, comps map[string]*component, byPackage map[string][]*component) []model.Relationship`
  - `func callsEdges(modulePath string, pkg *packages.Package, comps map[string]*component, byPackage map[string][]*component) []model.Relationship`
  - `func receiverNamed(obj types.Object) (*types.Named, bool)` / `func baseNamed(t types.Type) (*types.Named, bool)`
  - `func astUnparen(e ast.Expr) ast.Expr` / `func owns(comps map[string]*component, id string) bool`

- [ ] **Step 1: 扩 fixture**

`testdata/fixtures/shared-lib/core/core.go` 全文替换：

```go
// Package core imports edge, and edge imports core: a real package cycle the
// containment walk has to terminate on (spec §6.3).
package core

import "example.com/shared/edge"

// Server is the shared service type.
type Server struct{ Port int }

// Addr calls a package function. A package function has no Component subject,
// so this produces no calls edge (spec §6.4).
func (s *Server) Addr() string { return edge.Version() }

// Ledger lives in the same package as Server, so the edges below it are
// intra-package: stored for impact analysis, not drawn.
type Ledger struct{ server Server }

// Bind touches its sibling.
func (l *Ledger) Bind() string { return l.server.Addr() }
```

`testdata/fixtures/shared-lib/edge/edge.go` 全文替换：

```go
package edge

import "example.com/shared/core"

// Client names core.Server in its field and calls its method.
type Client struct{ server core.Server }

// Version is a package function, not a component.
func Version() string { return "1" }

// Ping reaches across the package boundary twice: a type use and a call.
func (c *Client) Ping() string { return c.server.Addr() }
```

- [ ] **Step 2: 写失败测试**

先把 `TestSharedLibContainment` 的期望改成 5 个元素（fixture 多了 Ledger）：`want` 加 `"cmp:example.com/shared/core.Ledger"`（排在 `core.Server` 之前），`wantContains` 加 `"ctn:example.com/shared/cmd/app -> cmp:example.com/shared/core.Ledger"`（排在同前缀的 `core.Server` 那行之前）。然后追加：

`internal/scanner/language/go_test.go` 末尾：

```go
func TestSharedLibDependencyEdges(t *testing.T) {
	out := parseFixture(t, "shared-lib")
	var got []string
	for _, r := range out.Relationships {
		if r.Type == model.RelContains {
			continue
		}
		got = append(got, fmt.Sprintf("%s -%s(%d)-> %s", r.FromID, r.Type, r.Weight, r.ToID))
	}
	slices.Sort(got)
	want := []string{
		"cmp:example.com/shared/core.Ledger -calls(1)-> cmp:example.com/shared/core.Server",
		"cmp:example.com/shared/core.Ledger -depends_on(1)-> cmp:example.com/shared/core.Server",
		"cmp:example.com/shared/edge.Client -calls(1)-> cmp:example.com/shared/core.Server",
		"cmp:example.com/shared/edge.Client -depends_on(1)-> cmp:example.com/shared/core.Server",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("edges =\n%v\nwant\n%v", got, want)
	}
}

func TestNoEdgesFromMainPackageFiles(t *testing.T) {
	// cmd/api 的 main() 调用 store.Cart 的方法，但 main 包不产 Component，
	// 于是这些调用没有边的主语（spec §6.4）。
	out := parseFixture(t, "simple-multi-main")
	for _, r := range out.Relationships {
		if r.Type != model.RelContains {
			t.Fatalf("unexpected edge from a main package: %+v", r)
		}
	}
}

func TestPackageFunctionCallProducesNoEdge(t *testing.T) {
	// core.Server.Addr 调 edge.Version：包函数不是 Component，不该出现
	// 指向 "…edge.Version" 这类不存在的元素的边。
	out := parseFixture(t, "shared-lib")
	known := map[string]bool{}
	for _, e := range out.Elements {
		known[e.ID] = true
	}
	for _, r := range out.Relationships {
		if !known[r.FromID] || !known[r.ToID] {
			t.Fatalf("dangling edge %+v", r)
		}
		if strings.Contains(r.ToID, "Version") {
			t.Fatalf("package function became a call target: %+v", r)
		}
	}
}
```

测试文件 import 补 `"fmt"`、`"strings"`（`parseFixture`/`slices` 已在 Task 9 引入）。

- [ ] **Step 3: 跑测试确认失败**

Run: `go test ./internal/scanner/language/ -run 'DependencyEdges|NoEdgesFromMain|PackageFunction'`
Expected: FAIL —— 当前只产 contains 边，`TestSharedLibDependencyEdges` 的 `got` 为空。

- [ ] **Step 4: 归属规则**

归属规则要按元素 id 反查组件，所以给 `component` 加 `id` 字段。`go.go` 里整块替换 Task 9 的 struct 定义：

```go
// component is a named type the scanner treats as a C4 Component.
type component struct {
	id      string
	pkgPath string
	named   *types.Named
}
```

Task 9 的构造处跟着改成 `comps[id] = &component{id: id, pkgPath: pkg.PkgPath, named: named}`。

追加归属实现：

```go
// componentIDFor turns a named type into its element id, reporting false for
// anything that is not a component of this scan (dependency types, unexported,
// builtin types with no package).
func componentIDFor(modulePath string, named *types.Named) (string, bool) {
	obj := named.Obj()
	if obj == nil || !obj.Exported() || obj.Pkg() == nil {
		return "", false
	}
	pkgPath := obj.Pkg().Path()
	if !ownPackage(modulePath, pkgPath) {
		return "", false
	}
	return model.ComponentID(pkgPath, obj.Name()), true
}

// edgeOwnership answers "which Component owns this source position?" — the
// attribution rule behind spec §6.4. A position inside a method whose receiver
// is a Component belongs to that Component; otherwise the nearest preceding
// component declaration in the same file owns it; a file-level function with no
// such owner contributes no edge.
func edgeOwnership(modulePath string, pkg *packages.Package, byPackage map[string][]*component,
	pos token.Pos) (string, bool) {
	if pkg.Fset == nil || !pos.IsValid() {
		return "", false
	}
	target := pkg.Fset.Position(pos)
	for _, syntax := range pkg.Syntax {
		if pkg.Fset.Position(syntax.Pos()).Filename != target.Filename {
			continue
		}
		return ownershipInFile(modulePath, pkg, byPackage, syntax, target.Line)
	}
	return "", false
}

func ownershipInFile(modulePath string, pkg *packages.Package, byPackage map[string][]*component,
	file *ast.File, line int) (string, bool) {
	var owner string
	ast.Inspect(file, func(node ast.Node) bool {
		decl, ok := node.(*ast.FuncDecl)
		if !ok || decl.Body == nil || decl.Recv == nil {
			return true
		}
		start, end := pkg.Fset.Position(decl.Pos()).Line, pkg.Fset.Position(decl.End()).Line
		if line < start || line > end {
			return true
		}
		named, ok := receiverNamed(pkg.TypesInfo.Defs[decl.Name])
		if !ok {
			return true
		}
		owner, _ = componentIDFor(modulePath, named)
		return owner == ""
	})
	if owner != "" {
		return owner, true
	}
	return nearestPrecedingComponent(modulePath, pkg, byPackage, file, line)
}

func nearestPrecedingComponent(modulePath string, pkg *packages.Package,
	byPackage map[string][]*component, file *ast.File, line int) (string, bool) {
	filename := pkg.Fset.Position(file.Pos()).Filename
	best, bestLine := "", -1
	for _, comp := range byPackage[pkg.PkgPath] {
		obj := comp.named.Obj()
		if pkg.Fset.Position(obj.Pos()).Filename != filename {
			continue
		}
		if declared := pkg.Fset.Position(obj.Pos()).Line; declared <= line && declared > bestLine {
			best, bestLine = comp.id, declared
		}
	}
	return best, best != ""
}

func receiverNamed(obj types.Object) (*types.Named, bool) {
	fn, ok := obj.(*types.Func)
	if !ok {
		return nil, false
	}
	sig, ok := fn.Type().(*types.Signature)
	if !ok || sig.Recv() == nil {
		return nil, false
	}
	return baseNamed(sig.Recv().Type())
}

func baseNamed(t types.Type) (*types.Named, bool) {
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	named, ok := t.(*types.Named)
	return named, ok
}

// go/ast has no Unparen, so parens come off by hand.
func astUnparen(e ast.Expr) ast.Expr {
	for {
		p, ok := e.(*ast.ParenExpr)
		if !ok {
			return e
		}
		e = p.X
	}
}

func owns(comps map[string]*component, id string) bool {
	_, ok := comps[id]
	return ok
}
```

- [ ] **Step 5: 两类边**

```go
func dependsOnEdges(modulePath string, pkg *packages.Package, comps map[string]*component,
	byPackage map[string][]*component) []model.Relationship {
	if pkg.TypesInfo == nil {
		return nil
	}
	var out []model.Relationship
	for ident, used := range pkg.TypesInfo.Uses {
		tn, ok := used.(*types.TypeName)
		if !ok {
			continue
		}
		named, ok := tn.Type().(*types.Named)
		if !ok {
			continue
		}
		to, isComponent := componentIDFor(modulePath, named)
		if !isComponent || !owns(comps, to) {
			continue
		}
		from, ok := edgeOwnership(modulePath, pkg, byPackage, ident.Pos())
		if !ok || from == to {
			continue
		}
		out = append(out, model.Relationship{FromID: from, ToID: to, Type: model.RelDependsOn, Weight: 1})
	}
	return out
}

func callsEdges(modulePath string, pkg *packages.Package, comps map[string]*component,
	byPackage map[string][]*component) []model.Relationship {
	if pkg.TypesInfo == nil {
		return nil
	}
	var out []model.Relationship
	for _, syntax := range pkg.Syntax {
		ast.Inspect(syntax, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			target, ok := calledComponent(pkg, call)
			if !ok {
				return true
			}
			to, isComponent := componentIDFor(modulePath, target)
			if !isComponent || !owns(comps, to) {
				return true
			}
			if from, owned := edgeOwnership(modulePath, pkg, byPackage, call.Pos()); owned && from != to {
				out = append(out, model.Relationship{FromID: from, ToID: to, Type: model.RelCalls, Weight: 1})
			}
			return true
		})
	}
	return out
}

// calledComponent resolves a call's subject. A method value resolves to its
// receiver type; an interface method therefore lands on the interface itself,
// which is where static analysis stops (spec §6.4).
func calledComponent(pkg *packages.Package, call *ast.CallExpr) (*types.Named, bool) {
	switch fun := astUnparen(call.Fun).(type) {
	case *ast.SelectorExpr:
		if sel := pkg.TypesInfo.Selections[fun]; sel != nil && sel.Kind() != types.FieldVal {
			if named, ok := baseNamed(sel.Recv()); ok {
				return named, true
			}
		}
		if obj := pkg.TypesInfo.Uses[fun.Sel]; obj != nil {
			return receiverNamed(obj)
		}
	case *ast.Ident:
		if obj := pkg.TypesInfo.Uses[fun]; obj != nil {
			return receiverNamed(obj)
		}
	}
	return nil, false
}
```

- [ ] **Step 6: 接线**

`relationships` 全文替换：

```go
func relationships(modulePath string, pkgs []*packages.Package,
	comps map[string]*component, containers map[string]bool) []model.Relationship {
	byPackage := map[string][]*component{}
	for _, comp := range comps {
		byPackage[comp.pkgPath] = append(byPackage[comp.pkgPath], comp)
	}

	var acc []model.Relationship
	for pkgPath := range containers {
		acc = append(acc, model.Relationship{
			FromID: model.SystemID(modulePath), ToID: model.ContainerID(pkgPath),
			Type: model.RelContains, Weight: 1,
		})
	}
	for _, owner := range componentOwners(modulePath, pkgs, comps, containers) {
		acc = append(acc, model.Relationship{FromID: owner.from, ToID: owner.toID, Type: model.RelContains, Weight: 1})
	}
	for _, pkg := range pkgs {
		if isTestVariant(pkg.ID) || pkg.Name == "main" || !ownPackage(modulePath, pkg.PkgPath) {
			continue
		}
		acc = append(acc, dependsOnEdges(modulePath, pkg, comps, byPackage)...)
		acc = append(acc, callsEdges(modulePath, pkg, comps, byPackage)...)
	}
	return dedup(acc)
}
```

这是 Task 10 对 `relationships` 的**完整替换**：签名与 Task 9 一致，只是在 contains 边之后追加 depends_on 与 calls 两类边，并为它们先建 `byPackage` 索引（`comps` 的 key 是元素 id，按包取组件需要反过来索引一次）。

- [ ] **Step 7: 跑测试确认通过**

```sh
go test ./internal/scanner/language/ -v
```

Expected: Task 9 + Task 10 全部 PASS。若 `core.Ledger -calls-> core.Server` 缺失，说明 `l.server.Addr()` 的 `Selections` 没命中：`fun.Sel` 是 `Addr`，`Selections[fun].Kind()` 应为 `types.MethodVal`、`Recv()` 为 `core.Server`。若 `depends_on` 多出 `Ledger` 指向自身，检查 `from == to` 的跳过。

- [ ] **Step 8: 提交**

```sh
go vet ./... && go test ./...
git add internal/scanner/language/go.go internal/scanner/language/go_test.go testdata/fixtures/shared-lib/
git commit -m "feat: 按源码位置归属推导 depends_on 与 calls 边，同包边入库不渲染"
```

---

### Task 11: 语言探测与扫描编排

**Files:**
- Create: `internal/scanner/{detector.go,report.go,scanner.go}`
- Modify: `internal/core/model/snapshot.go`（加 `NewSnapshotID`）
- Test: `internal/scanner/{detector_test.go,scanner_test.go}`、`internal/core/model/ids_test.go`（追加一条）

**Interfaces:**
- Consumes: Task 8 的 `language.All/Get`、Task 7 的 `store`、Task 5 的 ID 规则
- Produces:
  - `var scanner.ErrUnsupportedLanguage`、`var scanner.ErrUnreadable`
  - `func scanner.Detect(root string) (string, error)`
  - `func model.NewSnapshotID() string`
  - `type scanner.Report struct { SnapshotID, RepoID, ModulePath, Language string; Elements, Relationships int; PackageErrors []string }`
  - `func scanner.Run(ctx context.Context, db *store.DB, root string) (Report, error)`

- [ ] **Step 1: 写失败测试**

`internal/scanner/detector_test.go`：

```go
package scanner

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func fixtureRoot() string { return filepath.Join("..", "..", "testdata", "fixtures") }

func TestDetectGoFixture(t *testing.T) {
	name, err := Detect(filepath.Join(fixtureRoot(), "generic-and-iface"))
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if name != "go" {
		t.Fatalf("language = %q, want go", name)
	}
}

func TestDetectUnsupportedVersusUnreadable(t *testing.T) {
	// 没有 go.mod 的目录：语言不支持，而不是读不了（偏差 11）。
	if _, err := Detect(fixtureRoot()); !errors.Is(err, ErrUnsupportedLanguage) {
		t.Fatalf("err = %v, want ErrUnsupportedLanguage", err)
	}
	if _, err := Detect(filepath.Join(t.TempDir(), "does-not-exist")); !errors.Is(err, ErrUnreadable) {
		t.Fatalf("err = %v, want ErrUnreadable", err)
	}
}

func TestDetectUnreadablePermission(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: chmod would not block access")
	}
	blocked := filepath.Join(t.TempDir(), "locked")
	if err := os.Mkdir(blocked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(blocked, 0o755) })
	if _, err := Detect(blocked); !errors.Is(err, ErrUnreadable) {
		t.Fatalf("err = %v, want ErrUnreadable", err)
	}
}
```

`internal/core/model/ids_test.go` 追加：

```go
func TestSnapshotIDsSortByTime(t *testing.T) {
	first := NewSnapshotID()
	time.Sleep(time.Millisecond)
	second := NewSnapshotID()
	if first >= second {
		t.Fatalf("ids must sort by time: %q then %q", first, second)
	}
	if len(first) != 20 || first[0] != 's' {
		t.Fatalf("id = %q, want s + 19 digits", first)
	}
}
```

import 补 `"time"`。

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/scanner/ ./internal/core/model/`
Expected: `undefined: Detect`、`undefined: NewSnapshotID`。

- [ ] **Step 3: 实现 detector**

`internal/scanner/detector.go`：

```go
// Package scanner turns a directory into stored architecture: it picks a
// language adapter, parses, and writes one snapshot in one transaction.
// It imports no UI stack (spec §5.1, R1).
package scanner

import (
	"errors"
	"fmt"
	"os"

	"github.com/futuremeng/OpenArchitect/internal/scanner/language"
)

var (
	ErrUnsupportedLanguage = errors.New("scanner: unsupported language")
	ErrUnreadable          = errors.New("scanner: cannot read directory")
)

// Detect returns the language name for root. It separates "no adapter
// recognised this project" from "the project could not be read at all",
// because the UI has to say different things about each (spec §10).
func Detect(root string) (string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrUnreadable, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%w: %s is not a directory", ErrUnsupportedLanguage, root)
	}
	var unreadable error
	for _, adapter := range language.All() {
		ok, err := adapter.Detect(root)
		if err != nil {
			unreadable = err
			continue
		}
		if ok {
			return adapter.Language(), nil
		}
	}
	if unreadable != nil {
		return "", fmt.Errorf("%w: %v", ErrUnreadable, unreadable)
	}
	return "", ErrUnsupportedLanguage
}
```

- [ ] **Step 4: 实现 Report 与 Run**

`internal/scanner/report.go`：

```go
package scanner

// Report is one scan's summary: what was written, and which packages failed.
type Report struct {
	SnapshotID    string
	RepoID        string
	ModulePath    string
	Language      string
	Elements      int
	Relationships int
	PackageErrors []string
}
```

`internal/scanner/scanner.go`：

```go
package scanner

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
	"github.com/futuremeng/OpenArchitect/internal/core/store"
	"github.com/futuremeng/OpenArchitect/internal/scanner/language"
)

// Run scans root and writes one snapshot. A cancelled ctx or a failed parse
// writes nothing, so the caller keeps showing the previous snapshot (spec §10).
func Run(ctx context.Context, db *store.DB, root string) (Report, error) {
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
	snapshot := model.ArchitectureSnapshot{
		ID: model.NewSnapshotID(), RepoID: repo.ID, PackageErrors: out.PackageErrors,
	}
	if err := db.SaveSnapshot(ctx, snapshot, out.Elements, out.Relationships); err != nil {
		return Report{}, err
	}
	return Report{
		SnapshotID: snapshot.ID, RepoID: repo.ID, ModulePath: out.ModulePath, Language: name,
		Elements: len(out.Elements), Relationships: len(out.Relationships),
		PackageErrors: out.PackageErrors,
	}, nil
}
```

`internal/core/model/snapshot.go` 追加（import 补 `"fmt"`、`"time"`）：

```go
// NewSnapshotID is a fixed-width nanosecond stamp, so "ORDER BY id DESC" is
// "newest first" without a timestamp column doing double duty.
func NewSnapshotID() string { return fmt.Sprintf("s%019d", time.Now().UnixNano()) }
```

- [ ] **Step 5: Run 的测试**

`internal/scanner/scanner_test.go`：

```go
package scanner

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
	"github.com/futuremeng/OpenArchitect/internal/core/store"
)

func newTestDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "scan.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestRunWritesReadableSnapshot(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	report, err := Run(ctx, db, filepath.Join(fixtureRoot(), "simple-multi-main"))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// 1 System + 2 Container + Cart = 4，Task 9 的期望一致。
	if report.Language != "go" || report.Elements != 4 || report.Relationships == 0 {
		t.Fatalf("report = %+v", report)
	}
	elements, err := db.Elements(ctx, report.SnapshotID)
	if err != nil || len(elements) != report.Elements {
		t.Fatalf("stored elements = %d, %v", len(elements), err)
	}
	rels, err := db.Relationships(ctx, report.SnapshotID)
	if err != nil || len(rels) != report.Relationships {
		t.Fatalf("stored relationships = %d, %v", len(rels), err)
	}
}

func TestRunRejectsUnsupportedLanguage(t *testing.T) {
	db := newTestDB(t)
	if _, err := Run(context.Background(), db, fixtureRoot()); !errors.Is(err, ErrUnsupportedLanguage) {
		t.Fatalf("err = %v", err)
	}
	assertEmpty(t, db)
}

func TestRunCancelWritesNothing(t *testing.T) {
	db := newTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Run(ctx, db, filepath.Join(fixtureRoot(), "shared-lib"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	assertEmpty(t, db)
}

func TestRunKeepsPreviousSnapshotOnFailure(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	first, err := Run(ctx, db, filepath.Join(fixtureRoot(), "generic-and-iface"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Run(ctx, db, filepath.Join(fixtureRoot(), "no-such-fixture")); err == nil {
		t.Fatal("expected failure")
	}
	latest, err := db.LatestSnapshotID(ctx, first.RepoID)
	if err != nil || latest != first.SnapshotID {
		t.Fatalf("latest = %q, %v; want %q", latest, err, first.SnapshotID)
	}
}

func assertEmpty(t *testing.T, db *store.DB) {
	t.Helper()
	for _, table := range []string{"repos", "architecture_snapshots", "arch_elements", "arch_relationships"} {
		rows, err := db.Query(context.Background(), `SELECT count(*) FROM `+table)
		if err != nil {
			t.Fatalf("%s: %v", table, err)
		}
		if got := rows.Values[0][0]; got != int64(0) && got != float64(0) {
			t.Fatalf("%s has %v rows after a scan that should not have written", table, got)
		}
	}
}
```

`model` 只在这条测试里没用到（`Report` 字段够用了），import 去掉 `model`。`TestRunCancelWritesNothing` 依赖 `packages.Load` 把 ctx 取消传出来；若它返回的是别的错误，`Run` 里 `adapter.Parse` 之后的 `ctx.Err()` 检查已经保证不写库，此时把断言放宽为「`err != nil` 且 `assertEmpty`」，并在 findings 里记下 `packages.Load` 的取消行为。

- [ ] **Step 6: 跑测试确认通过**

```sh
go test ./internal/scanner/ ./internal/core/model/ ./internal/core/store/ -v
```

Expected: 全 PASS。

- [ ] **Step 7: 提交**

```sh
go vet ./... && go test ./...
git add internal/scanner/ internal/core/model/snapshot.go internal/core/model/ids_test.go
git commit -m "feat: 语言探测与扫描编排，取消与失败路径都不写库"
```
---

### Task 12: 图构建与多跳邻居

**Files:**
- Create: `internal/core/graph/{graph.go,bfs.go}`
- Test: `internal/core/graph/{graph_test.go,bfs_test.go}`

**Interfaces:**
- Consumes: Task 5 的 model（含 `model.ErrUnknownElement`）
- Produces:
  - `func graph.Build(elements []model.Element, rels []model.Relationship) *Graph`
  - `func (g *Graph) Elements() []model.Element` / `Relationships() []model.Relationship` / `Element(id string) (model.Element, bool)`
  - `type graph.Direction uint8`，常量 `DirIn/DirOut/DirBoth`
  - `func (g *Graph) Neighbors(id string, dir Direction) []model.Element`
  - `type graph.Hit struct { Element model.Element; Depth int; Via model.RelType }`
  - `func (g *Graph) Upstream(id string, maxDepth int) []Hit` / `Downstream(id string, maxDepth int) []Hit`
  - `func (g *Graph) Merge(other *Graph) error`、`var graph.ErrNotImplemented`

- [ ] **Step 1: 写失败测试**

`internal/core/graph/graph_test.go`：

```go
package graph

import (
	"errors"
	"testing"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

// tinyGraph: sys:x -contains-> A -calls-> B -depends_on-> C, plus a self loop
// on C. Cycle and self loop are what the BFS termination tests run against.
func tinyGraph() *Graph {
	elements := []model.Element{
		{ID: "cmp:a.A", Name: "A", Type: model.TypeComponent},
		{ID: "cmp:b.B", Name: "B", Type: model.TypeComponent},
		{ID: "cmp:c.C", Name: "C", Type: model.TypeComponent},
		{ID: "sys:x", Name: "x", Type: model.TypeSystem},
	}
	rels := []model.Relationship{
		{FromID: "cmp:a.A", ToID: "cmp:b.B", Type: model.RelCalls, Weight: 2},
		{FromID: "cmp:b.B", ToID: "cmp:c.C", Type: model.RelDependsOn, Weight: 1},
		{FromID: "sys:x", ToID: "cmp:a.A", Type: model.RelContains, Weight: 1},
		{FromID: "cmp:c.C", ToID: "cmp:c.C", Type: model.RelCalls, Weight: 1},
	}
	return Build(elements, rels)
}

func TestBuildSortsAndKeepsEverything(t *testing.T) {
	g := tinyGraph()
	if got := len(g.Elements()); got != 4 {
		t.Fatalf("elements = %d", got)
	}
	rels := g.Relationships()
	if len(rels) != 4 {
		t.Fatalf("relationships = %d", len(rels))
	}
	if rels[0].FromID != "cmp:a.A" {
		t.Fatalf("relationships must sort by from_id, got %q first", rels[0].FromID)
	}
	first := g.Elements()[0]
	if first.ID != "cmp:a.A" {
		t.Fatalf("elements must sort by id, got %q", first.ID)
	}
	if _, ok := g.Element("cmp:a.A"); !ok {
		t.Fatal("Element lookup failed")
	}
}

func TestBuildDropsEdgesWithUnknownEndpoints(t *testing.T) {
	g := Build([]model.Element{{ID: "cmp:a.A", Name: "A"}}, []model.Relationship{
		{FromID: "cmp:a.A", ToID: "cmp:ghost.G", Type: model.RelCalls, Weight: 1},
	})
	if len(g.Relationships()) != 0 {
		t.Fatalf("dangling edge kept: %+v", g.Relationships())
	}
}

func TestNeighborsDirection(t *testing.T) {
	g := tinyGraph()
	if down := names(g.Neighbors("cmp:a.A", DirOut)); len(down) != 1 || down[0] != "B" {
		t.Fatalf("DirOut(A) = %v", down)
	}
	// contains 参与 Neighbors（详情面板要列所属 Container），只是不参与扩散。
	if up := names(g.Neighbors("cmp:a.A", DirIn)); len(up) != 1 || up[0] != "x" {
		t.Fatalf("DirIn(A) = %v", up)
	}
	if both := names(g.Neighbors("cmp:b.B", DirBoth)); len(both) != 2 {
		t.Fatalf("DirBoth(B) = %v", both)
	}
}

func TestMergeIsNotImplemented(t *testing.T) {
	if err := tinyGraph().Merge(tinyGraph()); !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("err = %v", err)
	}
}

func names(elements []model.Element) []string {
	out := make([]string, 0, len(elements))
	for _, e := range elements {
		out = append(out, e.Name)
	}
	return out
}
```

`internal/core/graph/bfs_test.go`：

```go
package graph

import (
	"fmt"
	"testing"
	"time"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

func describe(hits []Hit) []string {
	out := make([]string, 0, len(hits))
	for _, h := range hits {
		out = append(out, fmt.Sprintf("%s@%d/%s", h.Element.Name, h.Depth, h.Via))
	}
	return out
}

func TestUpstreamExcludesContainsFromDiffusion(t *testing.T) {
	g := tinyGraph()
	// B 的上游只有 A。sys:x 经 contains 连到 A，若 contains 参与扩散就会在
	// 2 跳处冒出 x，影响范围立刻失真（spec §8.2）。
	if got := describe(g.Upstream("cmp:b.B", 3)); fmt.Sprint(got) != fmt.Sprint([]string{"A@1/calls"}) {
		t.Fatalf("upstream = %v", got)
	}
}

func TestDownstreamDepthStop(t *testing.T) {
	g := tinyGraph()
	if got := describe(g.Downstream("cmp:a.A", 1)); fmt.Sprint(got) != fmt.Sprint([]string{"B@1/calls"}) {
		t.Fatalf("depth 1 = %v", got)
	}
	got := describe(g.Downstream("cmp:a.A", 2))
	want := []string{"B@1/calls", "C@2/depends_on"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("depth 2 = %v", got)
	}
}

func TestSelfLoopTerminates(t *testing.T) {
	g := tinyGraph()
	done := make(chan []Hit, 1)
	go func() { done <- g.Downstream("cmp:c.C", 5) }()
	select {
	case hits := <-done:
		if len(hits) != 0 {
			t.Fatalf("self loop escaped into hits: %+v", hits)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("BFS did not terminate on a self loop")
	}
}

func TestCycleTerminates(t *testing.T) {
	g := Build([]model.Element{
		{ID: "cmp:a.A", Name: "A"}, {ID: "cmp:b.B", Name: "B"},
	}, []model.Relationship{
		{FromID: "cmp:a.A", ToID: "cmp:b.B", Type: model.RelCalls, Weight: 1},
		{FromID: "cmp:b.B", ToID: "cmp:a.A", Type: model.RelCalls, Weight: 1},
	})
	done := make(chan []Hit, 1)
	go func() { done <- g.Downstream("cmp:a.A", 10) }()
	select {
	case hits := <-done:
		if len(hits) != 1 || hits[0].Element.Name != "B" {
			t.Fatalf("hits = %+v", hits)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("BFS did not terminate on a package cycle")
	}
}

func TestResultsSortByDepthThenName(t *testing.T) {
	g := Build([]model.Element{
		{ID: "cmp:z.Z", Name: "Zulu"}, {ID: "cmp:m.M", Name: "Mike"},
		{ID: "cmp:a.A", Name: "Alpha"}, {ID: "cmp:c.Center", Name: "Center"},
	}, []model.Relationship{
		{FromID: "cmp:c.Center", ToID: "cmp:z.Z", Type: model.RelCalls, Weight: 1},
		{FromID: "cmp:c.Center", ToID: "cmp:m.M", Type: model.RelDependsOn, Weight: 1},
		{FromID: "cmp:c.Center", ToID: "cmp:a.A", Type: model.RelCalls, Weight: 1},
	})
	got := describe(g.Downstream("cmp:c.Center", 1))
	want := []string{"Alpha@1/calls", "Mike@1/depends_on", "Zulu@1/calls"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("order = %v want %v", got, want)
	}
}

func TestNonPositiveDepthReturnsNothing(t *testing.T) {
	// clamp 只发生在 Analyze（Task 13），图这一层不加解释。
	g := tinyGraph()
	if got := g.Downstream("cmp:a.A", 0); len(got) != 0 {
		t.Fatalf("depth 0 = %+v", got)
	}
	if got := g.Upstream("cmp:a.A", -1); len(got) != 0 {
		t.Fatalf("negative depth = %+v", got)
	}
}

func TestUnknownStartReturnsNothing(t *testing.T) {
	if got := tinyGraph().Downstream("cmp:none.Nope", 3); len(got) != 0 {
		t.Fatalf("hits = %+v", got)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/core/graph/`
Expected: `undefined: Build`。

- [ ] **Step 3: 实现 graph.go**

```go
// Package graph holds the in-memory architecture graph: adjacency, neighbour
// queries and impact diffusion. It imports no UI stack (spec §5.1, R1).
package graph

import (
	"errors"
	"slices"
	"strings"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

// ErrNotImplemented marks the slice-2 entry point. Keeping the method here
// means callers do not change shape when multi-repo merge arrives.
var ErrNotImplemented = errors.New("graph: multi-repo merge lands in slice 2")

type Direction uint8

const (
	DirIn Direction = iota
	DirOut
	DirBoth
)

type link struct {
	other string
	rel   model.Relationship
}

type Graph struct {
	elements      map[string]model.Element
	order         []string
	out, in       map[string][]link
	relationships []model.Relationship
}

func Build(elements []model.Element, rels []model.Relationship) *Graph {
	sorted := slices.Clone(elements)
	slices.SortFunc(sorted, func(a, b model.Element) int { return strings.Compare(a.ID, b.ID) })
	g := &Graph{
		elements: make(map[string]model.Element, len(sorted)),
		order:    make([]string, 0, len(sorted)),
		out:      map[string][]link{},
		in:       map[string][]link{},
	}
	for _, e := range sorted {
		g.elements[e.ID] = e
		g.order = append(g.order, e.ID)
	}
	kept := make([]model.Relationship, 0, len(rels))
	for _, rel := range rels {
		if _, ok := g.elements[rel.FromID]; !ok {
			continue
		}
		if _, ok := g.elements[rel.ToID]; !ok {
			continue
		}
		kept = append(kept, rel)
		g.out[rel.FromID] = append(g.out[rel.FromID], link{other: rel.ToID, rel: rel})
		g.in[rel.ToID] = append(g.in[rel.ToID], link{other: rel.FromID, rel: rel})
	}
	slices.SortFunc(kept, compareRel)
	g.relationships = kept
	return g
}

func (g *Graph) Elements() []model.Element {
	out := make([]model.Element, 0, len(g.order))
	for _, id := range g.order {
		out = append(out, g.elements[id])
	}
	return out
}

func (g *Graph) Relationships() []model.Relationship { return slices.Clone(g.relationships) }

func (g *Graph) Element(id string) (model.Element, bool) {
	e, ok := g.elements[id]
	return e, ok
}

func (g *Graph) Neighbors(id string, dir Direction) []model.Element {
	var links []link
	switch dir {
	case DirIn:
		links = g.in[id]
	case DirOut:
		links = g.out[id]
	default:
		// A fresh slice: appending to g.in[id] would write into the
		// adjacency list's spare capacity.
		links = append(append([]link{}, g.in[id]...), g.out[id]...)
	}
	seen := map[string]bool{id: true}
	out := make([]model.Element, 0, len(links))
	for _, l := range links {
		if seen[l.other] {
			continue
		}
		seen[l.other] = true
		out = append(out, g.elements[l.other])
	}
	slices.SortFunc(out, func(a, b model.Element) int {
		if c := strings.Compare(a.Name, b.Name); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
	return out
}

func (g *Graph) Merge(other *Graph) error { return ErrNotImplemented }

func compareRel(a, b model.Relationship) int {
	if c := strings.Compare(a.FromID, b.FromID); c != 0 {
		return c
	}
	if c := strings.Compare(a.ToID, b.ToID); c != 0 {
		return c
	}
	return strings.Compare(string(a.Type), string(b.Type))
}
```

- [ ] **Step 4: 实现 bfs.go**

```go
package graph

import (
	"slices"
	"strings"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

// Hit is a reachable element with its hop count and the relation that carried
// us there.
type Hit struct {
	Element model.Element
	Depth   int
	Via     model.RelType
}

// Upstream walks incoming edges (who calls or depends on me); Downstream walks
// outgoing ones. Both stop at maxDepth, and both skip contains edges: a
// hierarchy edge would let one System swallow the whole tree inside a hop
// (spec §8.2).
func (g *Graph) Upstream(id string, maxDepth int) []Hit { return g.bfs(id, maxDepth, g.in) }

func (g *Graph) Downstream(id string, maxDepth int) []Hit { return g.bfs(id, maxDepth, g.out) }

func (g *Graph) bfs(start string, maxDepth int, adjacency map[string][]link) []Hit {
	if _, ok := g.elements[start]; !ok || maxDepth < 1 {
		return nil
	}
	visited := map[string]bool{start: true}
	hits := []Hit{}
	frontier := []string{start}
	for depth := 1; depth <= maxDepth && len(frontier) > 0; depth++ {
		var next []string
		for _, current := range frontier {
			for _, l := range adjacency[current] {
				if l.rel.Type == model.RelContains || visited[l.other] {
					continue
				}
				// One visit per node records it at its shallowest hop, which is
				// what makes cycles and self loops terminate.
				visited[l.other] = true
				hits = append(hits, Hit{Element: g.elements[l.other], Depth: depth, Via: l.rel.Type})
				next = append(next, l.other)
			}
		}
		frontier = next
	}
	slices.SortFunc(hits, func(a, b Hit) int {
		if a.Depth != b.Depth {
			return a.Depth - b.Depth
		}
		if c := strings.Compare(a.Element.Name, b.Element.Name); c != 0 {
			return c
		}
		return strings.Compare(a.Element.ID, b.Element.ID)
	})
	return hits
}
```

- [ ] **Step 5: 跑测试确认通过**

Run: `go test ./internal/core/graph/ -v`
Expected: 全 PASS。`TestNonPositiveDepthReturnsNothing` 与 `TestSelfLoopTerminates` 直接由 `bfs` 的两处守卫满足。

- [ ] **Step 6: 提交**

```sh
go vet ./... && go test ./...
git add internal/core/graph/
git commit -m "feat: 内存架构图与 BFS 多跳查询，contains 不参与扩散"
```

---

### Task 13: 影响分析输出

**Files:**
- Create: `internal/core/graph/impact.go`
- Test: `internal/core/graph/impact_test.go`

**Interfaces:**
- Consumes: Task 12 的 `Graph`/`Hit`
- Produces:
  - `type graph.ImpactReport struct { Center model.Element; Upstream, Downstream []Hit }`
  - `func graph.Analyze(g *graph.Graph, elementID string, maxDepth int) (graph.ImpactReport, error)`
  - `func (r graph.ImpactReport) AffectedIDs() []string`
  - `func graph.IntraPackageIDs(g *graph.Graph, elementID string) []string`

- [ ] **Step 1: 写失败测试**

`internal/core/graph/impact_test.go`：

```go
package graph

import (
	"errors"
	"fmt"
	"testing"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

func TestAnalyzeBothSides(t *testing.T) {
	report, err := Analyze(tinyGraph(), "cmp:b.B", 2)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if report.Center.ID != "cmp:b.B" {
		t.Fatalf("center = %+v", report.Center)
	}
	if len(report.Upstream) != 1 || report.Upstream[0].Element.ID != "cmp:a.A" {
		t.Fatalf("upstream = %+v", report.Upstream)
	}
	if len(report.Downstream) != 1 || report.Downstream[0].Element.ID != "cmp:c.C" {
		t.Fatalf("downstream = %+v", report.Downstream)
	}
	got := fmt.Sprint(report.AffectedIDs())
	if got != fmt.Sprint([]string{"cmp:a.A", "cmp:b.B", "cmp:c.C"}) {
		t.Fatalf("affected = %s", got)
	}
}

func TestAnalyzeUnknownElement(t *testing.T) {
	if _, err := Analyze(tinyGraph(), "cmp:none.Nope", 2); !errors.Is(err, model.ErrUnknownElement) {
		t.Fatalf("err = %v, want model.ErrUnknownElement", err)
	}
}

func TestAnalyzeClampsDepthToOne(t *testing.T) {
	g := tinyGraph()
	zero, err := Analyze(g, "cmp:a.A", 0)
	if err != nil {
		t.Fatal(err)
	}
	one, err := Analyze(g, "cmp:a.A", 1)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(zero.AffectedIDs()) != fmt.Sprint(one.AffectedIDs()) {
		t.Fatalf("maxDepth < 1 must read as 1: %v vs %v", zero.AffectedIDs(), one.AffectedIDs())
	}
}

func TestAffectedIDsSortedAndDeduped(t *testing.T) {
	g := Build([]model.Element{
		{ID: "cmp:a.A", Name: "A"}, {ID: "cmp:b.B", Name: "B"}, {ID: "cmp:c.Center", Name: "Center"},
	}, []model.Relationship{
		{FromID: "cmp:c.Center", ToID: "cmp:a.A", Type: model.RelCalls, Weight: 1},
		{FromID: "cmp:a.A", ToID: "cmp:b.B", Type: model.RelCalls, Weight: 1},
		{FromID: "cmp:b.B", ToID: "cmp:a.A", Type: model.RelCalls, Weight: 1},
	})
	report, err := Analyze(g, "cmp:a.A", 3)
	if err != nil {
		t.Fatal(err)
	}
	// B 既从 A 到（1 跳）又从 Center 到（2 跳），只出现一次。
	got := fmt.Sprint(report.AffectedIDs())
	want := fmt.Sprint([]string{"cmp:a.A", "cmp:b.B", "cmp:c.Center"})
	if got != want {
		t.Fatalf("affected = %s want %s", got, want)
	}
}

func TestIntraPackageIDs(t *testing.T) {
	g := Build([]model.Element{
		{ID: "cmp:p/a.A", Name: "A"}, {ID: "cmp:p/a.B", Name: "B"}, {ID: "cmp:p/b.C", Name: "C"},
	}, []model.Relationship{
		{FromID: "cmp:p/a.A", ToID: "cmp:p/a.B", Type: model.RelCalls, Weight: 1},
		{FromID: "cmp:p/a.A", ToID: "cmp:p/b.C", Type: model.RelDependsOn, Weight: 1},
		{FromID: "sys:p", ToID: "cmp:p/a.A", Type: model.RelContains, Weight: 1},
	})
	got := fmt.Sprint(IntraPackageIDs(g, "cmp:p/a.A"))
	if got != fmt.Sprint([]string{"cmp:p/a.B"}) {
		t.Fatalf("intra-package = %s, want only the same-package sibling", got)
	}
	if len(IntraPackageIDs(g, "sys:p")) != 0 {
		t.Fatal("a System has no package of its own")
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/core/graph/ -run 'Analyze|Affected|IntraPackage'`
Expected: `undefined: Analyze`。

- [ ] **Step 3: 实现**

`internal/core/graph/impact.go`：

```go
package graph

import (
	"slices"
	"strings"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

// ImpactReport is the whole of "change here, what breaks there" in this slice:
// two BFS passes from one center.
type ImpactReport struct {
	Center     model.Element
	Upstream   []Hit
	Downstream []Hit
}

// Analyze reads a maxDepth below 1 as 1, so a garbage config value cannot
// silently produce an empty report that looks like "nothing depends on this".
func Analyze(g *Graph, elementID string, maxDepth int) (ImpactReport, error) {
	center, ok := g.Element(elementID)
	if !ok {
		return ImpactReport{}, model.ErrUnknownElement
	}
	if maxDepth < 1 {
		maxDepth = 1
	}
	return ImpactReport{
		Center:     center,
		Upstream:   g.Upstream(elementID, maxDepth),
		Downstream: g.Downstream(elementID, maxDepth),
	}, nil
}

// AffectedIDs is what the UI turns into flow's Muted flags: the center plus
// everything within depth, sorted, so CLI and GUI output can be compared line
// by line (spec §4.3 DoD-4).
func (r ImpactReport) AffectedIDs() []string {
	set := map[string]bool{r.Center.ID: true}
	for _, hit := range r.Upstream {
		set[hit.Element.ID] = true
	}
	for _, hit := range r.Downstream {
		set[hit.Element.ID] = true
	}
	out := make([]string, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	slices.Sort(out)
	return out
}

// IntraPackageIDs lists same-package neighbours reached by non-contains edges.
// flow cannot draw those usefully (spec §6.4), so the detail panel counts them
// instead. Living here keeps the count out of the UI, where it would violate R3.
func IntraPackageIDs(g *Graph, elementID string) []string {
	packagePath := model.PkgPathOf(elementID)
	if packagePath == "" {
		return nil
	}
	out := []string{}
	for _, rel := range g.Relationships() {
		if rel.Type == model.RelContains {
			continue
		}
		if model.PkgPathOf(rel.FromID) != packagePath || model.PkgPathOf(rel.ToID) != packagePath {
			continue
		}
		for _, id := range []string{rel.FromID, rel.ToID} {
			if id != elementID && !slices.Contains(out, id) {
				out = append(out, id)
			}
		}
	}
	slices.Sort(out)
	return out
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/core/graph/ -v`
Expected: Task 12 + 13 全 PASS。

- [ ] **Step 5: 提交**

```sh
go vet ./... && go test ./...
git add internal/core/graph/impact.go internal/core/graph/impact_test.go
git commit -m "feat: 影响分析报告与受影响元素集合，CLI 与 UI 共用同一份计算"
```

---

## Phase 2 — CLI、UI 与验收（Task 14–23）

### Task 14: headless CLI `oac`

**Files:**
- Create: `cmd/oac/{main.go,scan.go,graph.go,impact.go}`
- Test: `cmd/oac/{impact_test.go,parity_test.go}`

**Interfaces:**
- Consumes: `scanner.Run`、`store.Open`、`graph.Build/Analyze`、`db.Repos/LatestSnapshotID/SnapshotForElement`
- Produces:
  - `func defaultDBPath() (string, error)` = `filepath.Join(os.UserConfigDir(), "OpenArchitect", "openarchitect.db")`
  - `func runScan(ctx context.Context, db *store.DB, root string, out io.Writer) error`
  - `func runGraph(ctx context.Context, db *store.DB, repoID string, out io.Writer) error`
  - `func runImpact(ctx context.Context, db *store.DB, elementID string, maxDepth int, out io.Writer) error`
  - `func loadGraph(ctx context.Context, db *store.DB, snapshotID string) (*graph.Graph, error)`

CLI 不 import mygo/onevra（R2；Task 23 给 `cmd/oac` 单独加一条 `go list` 守卫，并用 `tools/ciguard` 钉住那条 YAML）。`os.UserConfigDir()` 在 macOS 返回 `~/Library/Application Support`，再套一层 `OpenArchitect` 正好等于 GUI 的 `App.Path(PathUserData)`——前提是 Task 1 把 productName 定成 `OpenArchitect`。

- [ ] **Step 1: 写失败测试**

`cmd/oac/impact_test.go`：

```go
package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/futuremeng/OpenArchitect/internal/core/store"
	"github.com/futuremeng/OpenArchitect/internal/scanner"
)

func fixturesRoot() string { return filepath.Join("..", "..", "testdata", "fixtures") }

// scanFixture scans once and hands back the database plus the snapshot id, so
// every CLI test reads real rows rather than a stub.
func scanFixture(t *testing.T, name string) (*store.DB, string, string) {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "cli.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	root := filepath.Join(fixturesRoot(), name)
	report, err := scanner.Run(context.Background(), db, root)
	if err != nil {
		t.Fatalf("scan %s: %v", name, err)
	}
	return db, report.SnapshotID, report.RepoID
}

func TestRunImpactPrintsBothSides(t *testing.T) {
	db, _, _ := scanFixture(t, "shared-lib")
	var buf bytes.Buffer
	// core.Server 的直接上游：edge.Client（跨包）与 core.Ledger（同包，
	// 影响分析仍然要算）。
	err := runImpact(context.Background(), db, "cmp:example.com/shared/core.Server", 1, &buf)
	if err != nil {
		t.Fatalf("runImpact: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"Upstream", "Downstream", "edge.Client", "core.Ledger"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRunImpactUnknownElement(t *testing.T) {
	db, _, _ := scanFixture(t, "shared-lib")
	err := runImpact(context.Background(), db, "cmp:nope.Nope", 2, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "unknown element") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunScanThenGraphAndImpact(t *testing.T) {
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "cli.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	var buf bytes.Buffer
	root := filepath.Join(fixturesRoot(), "simple-multi-main")
	if err := runScan(ctx, db, root, &buf); err != nil {
		t.Fatalf("runScan: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"snapshot s", "language go", "module example.com/simple", "elements 4"} {
		if !strings.Contains(out, want) {
			t.Fatalf("scan output missing %q:\n%s", want, out)
		}
	}
	repoID := model.RepoIDFor(mustAbs(t, root))
	buf.Reset()
	if err := runGraph(ctx, db, repoID, &buf); err != nil {
		t.Fatalf("runGraph: %v", err)
	}
	out = buf.String()
	if !strings.Contains(out, "sys:example.com/simple") || !strings.Contains(out, "contains") {
		t.Fatalf("graph output:\n%s", out)
	}
	buf.Reset()
	if err := runImpact(ctx, db, "cmp:example.com/simple/internal/store.Cart", 2, &buf); err != nil {
		t.Fatalf("runImpact: %v", err)
	}
}

func mustAbs(t *testing.T, p string) string {
	t.Helper()
	abs, err := filepath.Abs(p)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}
```

`model` 需要 import `"github.com/futuremeng/OpenArchitect/internal/core/model"`（`TestRunScanThenGraphAndImpact` 用了 `model.RepoIDFor`）。

`cmd/oac/parity_test.go` 是 **DoD-4 的机械那一半**：

```go
package main

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/futuremeng/OpenArchitect/internal/core/graph"
)

// TestCLIAndLibraryAgree: the ids "oac impact" prints must equal
// graph.Analyze().AffectedIDs() for the same fixture. The GUI reads the same
// AffectedIDs, so this single assertion is what keeps CLI, library and GUI from
// drifting apart (spec §4.3 DoD-4).
func TestCLIAndLibraryAgree(t *testing.T) {
	ctx := context.Background()
	db, snapshotID, _ := scanFixture(t, "shared-lib")
	const center = "cmp:example.com/shared/core.Server"

	elements, err := db.Elements(ctx, snapshotID)
	if err != nil {
		t.Fatal(err)
	}
	rels, err := db.Relationships(ctx, snapshotID)
	if err != nil {
		t.Fatal(err)
	}
	report, err := graph.Analyze(graph.Build(elements, rels), center, 2)
	if err != nil {
		t.Fatal(err)
	}
	fromLibrary := report.AffectedIDs()

	var buf bytes.Buffer
	if err := runImpact(ctx, db, center, 2, &buf); err != nil {
		t.Fatal(err)
	}
	var fromCLI []string
	for _, field := range strings.Fields(buf.String()) {
		if hasIDPrefix(field) {
			fromCLI = append(fromCLI, field)
		}
	}
	slices.Sort(fromCLI)
	fromCLI = slices.Compact(fromCLI)
	if !slices.Equal(fromLibrary, fromCLI) {
		t.Fatalf("CLI = %v\nlibrary = %v", fromCLI, fromLibrary)
	}
}

func hasIDPrefix(field string) bool {
	for _, prefix := range []string{"sys:", "ctn:", "cmp:", "repo:"} {
		if strings.HasPrefix(field, prefix) {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./cmd/oac/`
Expected: `undefined: runImpact`。

- [ ] **Step 3: 实现 main.go**

`cmd/oac/main.go`：

```go
// Command oac is OpenArchitect's headless face: it drives the same core as the
// desktop app and imports no UI stack (spec §5.1, R2).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/futuremeng/OpenArchitect/internal/core/store"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "oac:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return usage()
	}
	defaultPath, err := defaultDBPath()
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet(args[0], flag.ExitOnError)
	dbPath := fs.String("db", defaultPath, "database file")
	depth := fs.Int("depth", 2, "impact diffusion depth")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	rest := fs.Args()

	db, err := openDB(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	switch args[0] {
	case "scan":
		if len(rest) != 1 {
			return fmt.Errorf("scan wants exactly one path, got %v", rest)
		}
		return runScan(ctx, db, rest[0], os.Stdout)
	case "graph":
		if len(rest) != 1 {
			return fmt.Errorf("graph wants exactly one repo id, got %v", rest)
		}
		return runGraph(ctx, db, rest[0], os.Stdout)
	case "impact":
		if len(rest) != 1 {
			return fmt.Errorf("impact wants exactly one element id, got %v", rest)
		}
		return runImpact(ctx, db, rest[0], *depth, os.Stdout)
	default:
		return usage()
	}
}

func usage() error {
	fmt.Fprint(os.Stderr, `usage:
  oac scan   <path>                   [--db PATH]
  oac graph  <repo-id>                [--db PATH]
  oac impact <element-id> [--depth N] [--db PATH]
`)
	return fmt.Errorf("no subcommand")
}

func defaultDBPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "OpenArchitect", "openarchitect.db"), nil
}

func openDB(ctx context.Context, path string) (*store.DB, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	return store.Open(ctx, path)
}
```

- [ ] **Step 4: 实现三个子命令**

`cmd/oac/scan.go`：

```go
package main

import (
	"context"
	"fmt"
	"io"

	"github.com/futuremeng/OpenArchitect/internal/core/store"
	"github.com/futuremeng/OpenArchitect/internal/scanner"
)

func runScan(ctx context.Context, db *store.DB, root string, out io.Writer) error {
	report, err := scanner.Run(ctx, db, root)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "repo %s\nsnapshot %s\nlanguage %s\nmodule %s\nelements %d relationships %d\n",
		report.RepoID, report.SnapshotID, report.Language, report.ModulePath,
		report.Elements, report.Relationships)
	if len(report.PackageErrors) > 0 {
		fmt.Fprintf(out, "package errors (%d):\n", len(report.PackageErrors))
		for _, item := range report.PackageErrors {
			fmt.Fprintf(out, "  %s\n", item)
		}
	}
	return nil
}
```

`cmd/oac/graph.go`：

```go
package main

import (
	"context"
	"fmt"
	"io"

	"github.com/futuremeng/OpenArchitect/internal/core/graph"
	"github.com/futuremeng/OpenArchitect/internal/core/store"
)

// loadGraph builds the in-memory graph from stored rows. Every caller goes
// through here so the CLI, the GUI and the tests share one path (DoD-4).
func loadGraph(ctx context.Context, db *store.DB, snapshotID string) (*graph.Graph, error) {
	elements, err := db.Elements(ctx, snapshotID)
	if err != nil {
		return nil, err
	}
	rels, err := db.Relationships(ctx, snapshotID)
	if err != nil {
		return nil, err
	}
	return graph.Build(elements, rels), nil
}

func runGraph(ctx context.Context, db *store.DB, repoID string, out io.Writer) error {
	snapshotID, err := db.LatestSnapshotID(ctx, repoID)
	if err != nil {
		return err
	}
	if snapshotID == "" {
		return fmt.Errorf("oac: repo %q has no snapshot; run `oac scan` first", repoID)
	}
	g, err := loadGraph(ctx, db, snapshotID)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "snapshot %s: %d elements, %d relationships\n",
		snapshotID, len(g.Elements()), len(g.Relationships()))
	for _, e := range g.Elements() {
		fmt.Fprintf(out, "%s\t%s\t%s\n", e.ID, e.Type, e.Name)
	}
	for _, rel := range g.Relationships() {
		fmt.Fprintf(out, "%s -> %s [%s] x%d\n", rel.FromID, rel.ToID, rel.Type, rel.Weight)
	}
	return nil
}
```

`cmd/oac/impact.go`：

```go
package main

import (
	"context"
	"fmt"
	"io"

	"github.com/futuremeng/OpenArchitect/internal/core/graph"
	"github.com/futuremeng/OpenArchitect/internal/core/store"
)

func runImpact(ctx context.Context, db *store.DB, elementID string, maxDepth int, out io.Writer) error {
	snapshotID, err := db.SnapshotForElement(ctx, elementID)
	if err != nil {
		return err
	}
	g, err := loadGraph(ctx, db, snapshotID)
	if err != nil {
		return err
	}
	report, err := graph.Analyze(g, elementID, maxDepth)
	if err != nil {
		return err
	}
	// Only the two tables carry ids. The Affected set is deliberately not
	// printed: the parity test derives it from these rows, and printing it
	// again would put the same ids on screen twice.
	fmt.Fprintf(out, "Center %s %s (%s)\n", report.Center.ID, report.Center.Name, report.Center.Type)
	writeHits(out, "Upstream", report.Upstream)
	writeHits(out, "Downstream", report.Downstream)
	return nil
}

func writeHits(out io.Writer, title string, hits []graph.Hit) {
	fmt.Fprintf(out, "%s (%d)\n", title, len(hits))
	if len(hits) == 0 {
		fmt.Fprintln(out, "  (none)")
		return
	}
	for _, hit := range hits {
		fmt.Fprintf(out, "  depth %d  %s  %s <%s>  %s\n",
			hit.Depth, hit.Element.ID, hit.Element.Name, hit.Element.Type, hit.Via)
	}
}
```

- [ ] **Step 5: 跑测试确认通过**

```sh
go test ./cmd/oac/ -v
```

Expected: 全 PASS，含 `TestCLIAndLibraryAgree`。若 parity 断言失败，先看 `fromCLI` 里是否混进了 `<Component>` 这类带尖括号的字段——`hasIDPrefix` 只认前缀，不会误收；真出现多收，说明打印格式里出现了第二个 id 字段，按 Step 4 的格式收敛。

- [ ] **Step 6: 真机跑一遍**

```sh
go build -o /tmp/oac ./cmd/oac
/tmp/oac scan testdata/fixtures/shared-lib --db /tmp/oac.db
/tmp/oac impact cmp:example.com/shared/core.Server --depth 2 --db /tmp/oac.db
```

Expected: 两条命令都有输出；`scan` 首行给出 `repo repo:/Users/.../testdata/fixtures/shared-lib`（绝对路径）。

- [ ] **Step 7: 提交**

```sh
go vet ./... && go test ./...
git add cmd/oac/
git commit -m "feat: headless CLI oac 的 scan/graph/impact 与库路径一致性验收"
```

---

### Task 15: app_config 的类型化设置

**Files:**
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Consumes: `store.ConfigGet/ConfigSet`
- Produces:
  - `const config.DefaultImpactDepth = 2`
  - `type config.Settings struct { ScanRoot string; ImpactDepth int }`
  - `func config.Load(ctx context.Context, db *store.DB) (Settings, error)`
  - `func config.Save(ctx context.Context, db *store.DB, s Settings) error`
  - key 常量 `scanRootKey = "scan.root"`、`impactDepthKey = "impact.depth"`（包内不导出）

非法值（非数字、深度 < 1）回落默认且 `error` 为 nil：`app_config` 是用户可以手改的东西，坏值不该让 app 起不来。

- [ ] **Step 1: 写失败测试**

`internal/config/config_test.go`：

```go
package config

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/futuremeng/OpenArchitect/internal/core/store"
)

func newDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "cfg.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestLoadDefaultsWhenEmpty(t *testing.T) {
	got, err := Load(context.Background(), newDB(t))
	if err != nil {
		t.Fatal(err)
	}
	if got.ImpactDepth != DefaultImpactDepth || got.ScanRoot != "" {
		t.Fatalf("settings = %+v", got)
	}
}

func TestSaveThenLoad(t *testing.T) {
	db := newDB(t)
	want := Settings{ScanRoot: "/Users/me/code/cart", ImpactDepth: 3}
	if err := Save(context.Background(), db, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestGarbageValuesFallBack(t *testing.T) {
	db := newDB(t)
	ctx := context.Background()
	for _, bad := range []string{"not-a-number", "0", "-4", ""} {
		if err := db.ConfigSet(ctx, impactDepthKey, bad); err != nil {
			t.Fatal(err)
		}
		got, err := Load(ctx, db)
		if err != nil {
			t.Fatalf("%q must not fail Load: %v", bad, err)
		}
		if got.ImpactDepth != DefaultImpactDepth {
			t.Fatalf("%q fell back to %d", bad, got.ImpactDepth)
		}
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/config/`
Expected: `undefined: Load`。

- [ ] **Step 3: 实现**

`internal/config/config.go`：

```go
// Package config types the app_config table. User settings only: the schema
// version lives in PRAGMA user_version, not here (spec §7.1).
package config

import (
	"context"
	"strconv"

	"github.com/futuremeng/OpenArchitect/internal/core/store"
)

const (
	DefaultImpactDepth = 2

	scanRootKey    = "scan.root"
	impactDepthKey = "impact.depth"
)

type Settings struct {
	ScanRoot    string
	ImpactDepth int
}

func Load(ctx context.Context, db *store.DB) (Settings, error) {
	root, err := db.ConfigGet(ctx, scanRootKey)
	if err != nil {
		return Settings{}, err
	}
	raw, err := db.ConfigGet(ctx, impactDepthKey)
	if err != nil {
		return Settings{}, err
	}
	out := Settings{ScanRoot: root, ImpactDepth: DefaultImpactDepth}
	if depth, convErr := strconv.Atoi(raw); convErr == nil && depth >= 1 {
		out.ImpactDepth = depth
	}
	return out, nil
}

func Save(ctx context.Context, db *store.DB, s Settings) error {
	if err := db.ConfigSet(ctx, scanRootKey, s.ScanRoot); err != nil {
		return err
	}
	return db.ConfigSet(ctx, impactDepthKey, strconv.Itoa(s.ImpactDepth))
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/config/ -v`
Expected: 三条 PASS。

- [ ] **Step 5: 提交**

```sh
go vet ./... && go test ./...
git add internal/config/
git commit -m "feat: app_config 的类型化设置读写与非法值回落"
```

---

### Task 16: App 状态机（扫描编排、从库重建图、选中与影响模式）

**Files:**
- Create: `internal/ui/state.go`、`internal/ui/app.go`
- Test: `internal/ui/app_test.go`

外壳装配（`internal/ui/shell.go` + `main.go` 搬到 `cmd/openarchitect/`）在 **Task 22**：它一 import `shells/sidebar`，`internal/ui` 就非有页面函数不可，提前做会让本任务编译不过。Task 17–21 全部只写 `App` 的方法和纯渲染函数，用 `ui.NewTester` 单测，不碰窗口。

**Interfaces:**
- Consumes: Task 7 的 `store.DB`（含刚补的 `Snapshot`）、Task 11 的 `scanner.Run/Detect`、Task 12/13 的 `graph.Build/Analyze`、Task 15 的 `config.Load/Save`
- Produces（Task 17–22 只认这些名字）：
  - `ui.Page`，常量 `PageGraph`、`PageRepos`、`PageSettings`
  - `ui.Status`，常量 `StatusIdle`、`StatusScanning`、`StatusReady`、`StatusFailed`
  - `type ui.App` —— 字段见 Step 3；构造 `func NewApp(db *store.DB) *App`
  - `func (a *App) SetPost(fn func(func()))`
  - `func (a *App) SetOpenPath(fn func(string) error)`、`func (a *App) OpenFile(path string) error`、`var ErrNoPathOpener`
  - `func (a *App) Reload(ctx context.Context) error`
  - `func (a *App) SetPage(p Page)`、`func (a *App) Page() Page`
  - `var ErrScanInFlight`、`func (a *App) BeginScan(root string) error`、`func (a *App) CancelScan()`
  - `func (a *App) Select(id string)`、`Open(id string)`、`SetImpactMode(on bool)`、`SetCommandOpen(open bool)`
  - `func (a *App) SaveSettings(ctx context.Context, s config.Settings) error`
  - `func (a *App) StatusLine() string`

**两条纪律先说清楚，后面所有 UI 任务都靠它们：**

1. `App` 的字段只在主线程写。跨线程交接只有 `a.post`（= `Window().Update`）一处，Step 3 的 `SetPost` 默认值是**同步执行**，所以测试里不需要窗口也能跑完整状态机。
2. mygo 的交互是**每帧轮询**，不是注册回调：`element.Clicked() bool`、`c.Shortcut(mods, key) bool`（`ui/api_gen.go:3057`、`:468`）。不要写 `Clicked(func(){…})`，编译不过。

另有一条看着像坑、实测合法：本包自己叫 `ui`，又 `import "github.com/egoist/mygo/ui"`。Go 允许（包声明名不进文件作用域，`ui.Row(...)` 一律指导入的那个 `ui`），Task 16–22 全程就这么写，不要改成别名。已用一个双 `ui` 包的最小模块验证过 `go build ./... && go vet ./...` 通过。

- [ ] **Step 1: 写失败测试**

`internal/ui/app_test.go`：

```go
package ui

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/futuremeng/OpenArchitect/internal/core/store"
	"github.com/futuremeng/OpenArchitect/internal/scanner"
)

// newTestApp 开一个真实的临时库：App 的全部意义在于把库里的行变成图，
// 用假存储等于什么都没测（spec §4.3 DoD-4）。
func newTestApp(t *testing.T) *App {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "openarchitect.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewApp(db)
}

func fixtureDir(name string) string {
	return filepath.Join("..", "..", "testdata", "fixtures", name)
}

func seedScan(t *testing.T, a *App, root string) scanner.Report {
	t.Helper()
	report, err := scanner.Run(context.Background(), a.db, root)
	if err != nil {
		t.Fatalf("scanner.Run: %v", err)
	}
	if report.Elements == 0 {
		t.Fatal("fixture scan produced no elements")
	}
	return report
}

func TestNewAppDefaults(t *testing.T) {
	a := newTestApp(t)
	if a.Page() != PageGraph || a.status != StatusIdle {
		t.Fatalf("page/status = %v/%v", a.Page(), a.status)
	}
	if a.graph == nil || len(a.graph.Elements()) != 0 {
		t.Fatal("NewApp must install an empty graph so later reads never nil-check")
	}
}

func TestReloadPicksNewestSnapshotAcrossRepos(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	seedScan(t, a, fixtureDir("simple-multi-main"))
	second := seedScan(t, a, fixtureDir("shared-lib"))
	if err := a.Reload(ctx); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if len(a.repos) != 2 {
		t.Fatalf("repos = %d", len(a.repos))
	}
	if a.snapshot.ID != second.SnapshotID {
		t.Fatalf("Reload must take the newest snapshot, got %q want %q", a.snapshot.ID, second.SnapshotID)
	}
	if a.repoID != second.RepoID || a.status != StatusReady {
		t.Fatalf("repoID/status = %q/%v", a.repoID, a.status)
	}
	if len(a.graph.Elements()) != second.Elements {
		t.Fatalf("graph elements = %d want %d", len(a.graph.Elements()), second.Elements)
	}
}

func TestReloadWithEmptyDatabase(t *testing.T) {
	a := newTestApp(t)
	if err := a.Reload(context.Background()); err != nil {
		t.Fatalf("Reload on an empty database must not fail: %v", err)
	}
	if a.snapshot.ID != "" || a.repoID != "" || len(a.graph.Elements()) != 0 {
		t.Fatalf("snapshot = %+v repoID = %q", a.snapshot, a.repoID)
	}
}

func TestBeginScanGatesOnScanning(t *testing.T) {
	a := newTestApp(t)
	a.status = StatusScanning
	if err := a.BeginScan(fixtureDir("shared-lib")); !errors.Is(err, ErrScanInFlight) {
		t.Fatalf("BeginScan while scanning = %v", err)
	}
}

func TestBeginScanRejectsNonGoPathWithoutGoroutine(t *testing.T) {
	a := newTestApp(t)
	err := a.BeginScan(fixtureDir("shared-lib/core"))
	if err == nil {
		t.Fatal("a directory with no go.mod must fail")
	}
	// spec §10: 这种情况根本不起 goroutine，所以状态立刻可判，没有竞态。
	if a.status != StatusFailed || a.err == nil {
		t.Fatalf("status/err = %v/%v", a.status, a.err)
	}
	if a.cancel != nil {
		t.Fatal("cancel must not be set when the scan never started")
	}
}

// TestFinishScanKeepsPreviousGraph covers spec §10's three failure branches in
// one place. post defaults to running inline, so this is a plain function call.
func TestFinishScanKeepsPreviousGraph(t *testing.T) {
	a := newTestApp(t)
	seedScan(t, a, fixtureDir("shared-lib"))
	if err := a.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	before := len(a.graph.Elements())

	a.finishScan(scanner.Report{}, errors.New("parse exploded"))
	if a.status != StatusFailed || a.err == nil || len(a.graph.Elements()) != before {
		t.Fatalf("status=%v err=%v elements=%d", a.status, a.err, len(a.graph.Elements()))
	}

	a.finishScan(scanner.Report{}, context.Canceled)
	if a.status != StatusIdle || a.err != nil || len(a.graph.Elements()) != before {
		t.Fatalf("cancel must return to Idle with the old graph: status=%v err=%v", a.status, a.err)
	}
}

func TestSelectClearsDrawerOnBlankClick(t *testing.T) {
	a := newTestApp(t)
	seedScan(t, a, fixtureDir("shared-lib"))
	if err := a.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	id := a.graph.Elements()[0].ID
	a.SetImpactMode(true)
	a.Select(id)
	if a.selected != id || a.impact == nil || a.impact.Center.ID != id {
		t.Fatalf("selected=%q impact=%+v", a.selected, a.impact)
	}
	a.Select("")
	if a.selected != "" || a.impact != nil || a.drillInto != "" {
		t.Fatalf("blank click must clear: %+v", a)
	}
}

func TestSetImpactModeOffDropsReport(t *testing.T) {
	a := newTestApp(t)
	seedScan(t, a, fixtureDir("shared-lib"))
	if err := a.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	id := a.graph.Elements()[0].ID
	a.Select(id)
	if a.impact != nil {
		t.Fatal("impact must stay nil while the mode is off")
	}
	a.SetImpactMode(true)
	if a.impact == nil {
		t.Fatal("turning the mode on with a selection must analyse at once")
	}
	a.SetImpactMode(false)
	if a.impact != nil {
		t.Fatal("turning it off must drop the report so the map un-fades")
	}
}

// TestOpenFileUntilWired: Task 22 is the only thing that knows how to reach
// the OS, so before it the drawer must be able to disable the row instead of
// calling into an uninitialised app runtime.
func TestOpenFileUntilWired(t *testing.T) {
	a := newTestApp(t)
	if err := a.OpenFile("/tmp/x.go"); !errors.Is(err, ErrNoPathOpener) {
		t.Fatalf("OpenFile(unwired) = %v", err)
	}
	if err := a.OpenFile(""); err == nil {
		t.Fatal("empty path must fail before the opener is consulted")
	}
	var got string
	a.SetOpenPath(func(path string) error { got = path; return nil })
	if err := a.OpenFile("/tmp/x.go"); err != nil || got != "/tmp/x.go" {
		t.Fatalf("OpenFile = %v, reached %q", err, got)
	}
}

func TestStatusLineCountsPackageErrors(t *testing.T) {
	a := newTestApp(t)
	seedScan(t, a, fixtureDir("shared-lib"))
	if err := a.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	a.status = StatusReady
	line := a.StatusLine()
	if line == "" || !strings.HasPrefix(line, "完成：") {
		t.Fatalf("StatusLine = %q", line)
	}
	a.status = StatusFailed
	a.err = errors.New("boom")
	if got := a.StatusLine(); !strings.Contains(got, "boom") {
		t.Fatalf("failed StatusLine = %q", got)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `export PATH="/usr/local/go/bin:$PATH" && go test ./internal/ui/`
Expected: 编译失败，`undefined: NewApp`、`undefined: ErrScanInFlight`。

- [ ] **Step 3: 实现 state.go**

`internal/ui/state.go`：

```go
package ui

import (
	"context"
	"errors"
	"fmt"

	"github.com/oneo-me/onevra/source/components/flow"

	"github.com/futuremeng/OpenArchitect/internal/config"
	"github.com/futuremeng/OpenArchitect/internal/core/graph"
	"github.com/futuremeng/OpenArchitect/internal/core/model"
	"github.com/futuremeng/OpenArchitect/internal/core/store"
)

// Page is which page the content region shows. sidebar.Shell has no page
// registry (spec §16 U1), so switching is a branch inside the Content builder
// and Shell.SetPage is never called.
type Page string

const (
	PageGraph    Page = "graph"
	PageRepos    Page = "repos"
	PageSettings Page = "settings"
)

// Status is the scan lifecycle shown in the sidebar's status line.
type Status uint8

const (
	StatusIdle Status = iota
	StatusScanning
	StatusReady
	StatusFailed
)

// App is the whole application state, and the only thing the pages read.
// spec §9.2 listed snapshot as a pointer; a value type with ID == "" is the
// empty state, which removes a nil check from every reader.
type App struct {
	db       *store.DB
	repoID   string
	scanRoot string

	page     Page
	status   Status
	err      error
	progress string

	repos    []model.Repo
	snapshot model.ArchitectureSnapshot
	graph    *graph.Graph
	settings config.Settings

	selected    string
	drillInto   string
	impactMode  bool
	impact      *graph.ImpactReport
	commandOpen bool

	flowState flow.State
	cancel    context.CancelFunc
	// post hands work to the main thread. It is Window().Update in production
	// and runs inline in tests, which is why no test needs a window.
	post func(fn func())
	// openPath reveals a file in the OS. Task 22 wires it to
	// mygo.Shell.OpenPath; nil until then, and detail_panel.go disables the
	// row instead of importing the mygo root package early.
	openPath func(string) error
}

func NewApp(db *store.DB) *App {
	return &App{
		db:    db,
		page:  PageGraph,
		graph: graph.Build(nil, nil),
		post:  func(fn func()) { fn() },
	}
}

// SetPost installs the cross-thread handoff. Call it once, right after
// NewShell; every later goroutine result goes through it (spec §5.3).
func (a *App) SetPost(fn func(func())) {
	if fn != nil {
		a.post = fn
	}
}

// ErrNoPathOpener is what OpenFile reports before Task 22 wires the real
// opener, so the drawer can grey the row out instead of panicking.
var ErrNoPathOpener = errors.New("ui: no path opener installed")

func (a *App) SetOpenPath(fn func(string) error) { a.openPath = fn }

// OpenFile reveals an element's source file (spec §9.4). It never panics:
// mygo.Shell.OpenPath needs a running app, and the only caller is a click
// handler, which by definition runs after App.Run.
func (a *App) OpenFile(path string) error {
	if path == "" {
		return fmt.Errorf("ui: empty path")
	}
	if a.openPath == nil {
		return ErrNoPathOpener
	}
	return a.openPath(path)
}

func (a *App) Page() Page { return a.page }

func (a *App) SetPage(p Page) { a.page = p }

func (a *App) SetCommandOpen(open bool) { a.commandOpen = open }

func (a *App) Open(id string) {
	a.drillInto = id
	if a.selected != id {
		a.Select(id)
	}
}

// Select is flow's single-click handler; "" comes from a click on empty
// space and closes the drawer (spec §9.3).
func (a *App) Select(id string) {
	a.selected = id
	if id == "" {
		a.impact = nil
		a.drillInto = ""
		return
	}
	if a.impactMode {
		a.analyse(id)
	}
}

func (a *App) SetImpactMode(on bool) {
	a.impactMode = on
	if !on {
		a.impact = nil
		return
	}
	if a.selected != "" {
		a.analyse(a.selected)
	}
}

func (a *App) analyse(id string) {
	report, err := graph.Analyze(a.graph, id, a.settings.ImpactDepth)
	if err != nil {
		a.impact = nil
		return
	}
	a.impact = &report
}

// StatusLine is the sidebar's one-line summary (spec §10's error table rows).
func (a *App) StatusLine() string {
	switch a.status {
	case StatusScanning:
		if a.progress != "" {
			return a.progress
		}
		return "正在扫描…"
	case StatusFailed:
		return "失败：" + a.err.Error()
	case StatusReady:
		count := len(a.graph.Elements())
		if errs := len(a.snapshot.PackageErrors); errs > 0 {
			return fmt.Sprintf("完成：%d 个元素，%d 个包有错误", count, errs)
		}
		return fmt.Sprintf("完成：%d 个元素", count)
	default:
		if len(a.graph.Elements()) == 0 {
			return "选择目录后点扫描"
		}
		return fmt.Sprintf("%d 个元素", len(a.graph.Elements()))
	}
}
```

`a.err` 在 `StatusFailed` 分支一定非空：只有 `failScan` 和 `finishScan` 两条路径会置这个状态，两处都同时写 `err`。所以这里不用防 nil。

- [ ] **Step 4: 实现 app.go（Reload 与扫描编排）**

`internal/ui/app.go`：

```go
package ui

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/futuremeng/OpenArchitect/internal/config"
	"github.com/futuremeng/OpenArchitect/internal/core/graph"
	"github.com/futuremeng/OpenArchitect/internal/core/model"
	"github.com/futuremeng/OpenArchitect/internal/scanner"
)

// ErrScanInFlight guards the button while a scan runs (spec §10: 期间重复点
// 扫描被 status == Scanning 挡住).
var ErrScanInFlight = errors.New("ui: a scan is already running")

// Reload re-reads everything from the database. Deliberately it does not
// reuse the scanner's in-memory output: the graph is built from stored rows,
// which is what makes GUI and CLI views identical by construction (DoD-4).
func (a *App) Reload(ctx context.Context) error {
	settings, err := config.Load(ctx, a.db)
	if err != nil {
		return err
	}
	a.settings = settings
	repos, err := a.db.Repos(ctx)
	if err != nil {
		return err
	}
	a.repos = repos

	newest, owner := "", ""
	for _, r := range repos {
		id, err := a.db.LatestSnapshotID(ctx, r.ID)
		if err != nil {
			return err
		}
		if id > newest {
			newest, owner = id, r.ID
		}
	}
	if newest == "" {
		a.snapshot = model.ArchitectureSnapshot{}
		a.repoID = ""
		a.graph = graph.Build(nil, nil)
		return nil
	}
	snapshot, err := a.db.Snapshot(ctx, newest)
	if err != nil {
		return err
	}
	elements, err := a.db.Elements(ctx, newest)
	if err != nil {
		return err
	}
	rels, err := a.db.Relationships(ctx, newest)
	if err != nil {
		return err
	}
	a.snapshot, a.repoID = snapshot, owner
	a.graph = graph.Build(elements, rels)
	a.selected, a.drillInto, a.impact = "", "", nil
	return nil
}

// BeginScan detects the language on the main thread first: spec §10's "根目录
// 无 go.mod → 不起 goroutine" row. scanner.Run detects again internally; the
// duplicate is the price of not starting a goroutine for a known-bad path.
func (a *App) BeginScan(root string) error {
	if a.status == StatusScanning {
		return ErrScanInFlight
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return a.failScan(err)
	}
	if _, err := scanner.Detect(abs); err != nil {
		return a.failScan(err)
	}
	a.repoID, a.scanRoot = model.RepoIDFor(abs), abs
	a.status, a.err, a.progress = StatusScanning, nil, "正在解析 "+filepath.Base(abs)

	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	go a.scan(ctx, abs)
	return nil
}

func (a *App) scan(ctx context.Context, root string) {
	report, err := scanner.Run(ctx, a.db, root)
	a.post(func() { a.finishScan(report, err) })
}

// finishScan runs only on the main thread, inside post. A failure leaves the
// previous snapshot and graph on screen (spec §10).
func (a *App) finishScan(report scanner.Report, err error) {
	a.cancel = nil
	if err != nil {
		if errors.Is(err, context.Canceled) {
			a.status, a.err, a.progress = StatusIdle, nil, ""
			return
		}
		a.status, a.err, a.progress = StatusFailed, err, ""
		return
	}
	a.progress = ""
	if reloadErr := a.Reload(context.Background()); reloadErr != nil {
		a.status, a.err = StatusFailed, reloadErr
		return
	}
	a.status = StatusReady
	// Remember the root that produced what is on screen. report has no file
	// path in it (ModulePath is an import path), so BeginScan's a.scanRoot is
	// the only correct source.
	if a.scanRoot != "" && a.scanRoot != a.settings.ScanRoot {
		a.settings.ScanRoot = a.scanRoot
		_ = a.SaveSettings(context.Background(), a.settings)
	}
}

func (a *App) failScan(err error) error {
	a.status, a.err, a.progress = StatusFailed, err, ""
	return err
}

// CancelScan is safe to call when nothing is running.
func (a *App) CancelScan() {
	if a.cancel != nil {
		a.cancel()
	}
}

func (a *App) SaveSettings(ctx context.Context, s config.Settings) error {
	a.settings = s
	return config.Save(ctx, a.db, s)
}
```

`scanner.Report` 里没有绝对路径，所以 `BeginScan` 要顺手记一份 `a.scanRoot = abs`（Step 3 的 `App` 字段表里就有它）。`finishScan` 只在成功路径把它写回 `settings.ScanRoot`，失败时不改设置——用户下次点扫描还得用原来的目录。

- [ ] **Step 5: 跑测试确认通过**

Run: `export PATH="/usr/local/go/bin:$PATH" && go test ./internal/ui/ -v && go vet ./internal/ui/`
Expected: 全部 PASS。`internal/ui` 到这里只有 `state.go` + `app.go`，没有任何 UI 元素构造，所以 `go test` 不需要窗口。Task 17–21 会开始 import `components`/`shells/sidebar`/`mygo/ui`，但只用 `ui.NewTester` 渲染，同样不开窗口；真窗口是 Task 22 的 `NewShell`，那是 `mygo` 根包第一次进这个包。

- [ ] **Step 6: 提交**

```sh
go vet ./... && go test ./...
git add internal/ui/state.go internal/ui/app.go internal/ui/app_test.go
git commit -m "feat: App 状态机，扫描编排与从库重建图（GUI/CLI 同源）"
```

---

### Task 17: 节点卡片与 flow 数据装配

**Files:**
- Create: `internal/ui/node_card.go`、`internal/ui/flow_data.go`
- Test: `internal/ui/node_card_test.go`、`internal/ui/flow_data_test.go`

**Interfaces:**
- Consumes: Task 5 的 `model`（含 `model.PkgPathOf`）、Task 12/13 的 `graph`、Task 16 的 `App`（`graph`、`impactMode`、`impact`、`selected`）
- Produces（Task 18/19 只认这些）：
  - `const cardWidth = float32(168)`、`const cardHeight = float32(46)`
  - `func iconFor(t model.ElementType) *ui.SVG`
  - `func nodeCard(c *ui.Context, el model.Element)`
  - `func truncateMiddle(s string, keep int) string`
  - `func (a *App) flowData() flow.Data`
  - `func (a *App) flowEdges() []flow.Edge`
  - `func (a *App) quietSet() map[string]bool`（nil = 这一帧谁都不淡出）

`168×46` 不是审美数字：PoC-4 就是用这个尺寸的合成卡片量出 600 节点的帧时间的（Task 4），生产卡片必须和探针同形，否则那个数字作废。

- [ ] **Step 1: 写失败测试**

`internal/ui/node_card_test.go`：

```go
package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

func TestTruncateMiddle(t *testing.T) {
	cases := []struct{ in string; keep int; want string }{
		{"Basket", 8, "Basket"},
		{"abcdefghij", 5, "ab…ij"},
		{"example.com/cart/internal/basket", 11, "examp…asket"},
		{"任意长的中文包路径abcdefgh", 5, "任意…gh"},
		{"short", 3, "…"},
	}
	for _, tc := range cases {
		if got := truncateMiddle(tc.in, tc.keep); got != tc.want {
			t.Errorf("truncateMiddle(%q, %d) = %q want %q", tc.in, tc.keep, got, tc.want)
		}
	}
}

func TestIconForEveryType(t *testing.T) {
	for _, tp := range []model.ElementType{model.TypeSystem, model.TypeContainer, model.TypeComponent} {
		if iconFor(tp) == nil {
			t.Errorf("iconFor(%v) = nil", tp)
		}
	}
}

// TestNodeCardIsTwoTextLines 是 spec §9.3 那条性能约束的守门测试：卡片每帧
// 重建，1000 个 label 已经要 397 µs，第三行文本或任何控件都是拿性能换可读性。
// ui.NewTester 在内存里构建视图，不开窗口。
func TestNodeCardIsTwoTextLines(t *testing.T) {
	el := model.Element{
		ID:   "cmp:example.com/cart/internal/basket.Basket",
		Type: model.TypeComponent,
		Name: "Basket",
	}
	tt := ui.NewTester(func(c *ui.Context) { nodeCard(c, el) }, 220, 60)
	if !tt.HasText("Basket") {
		t.Fatalf("card must show the type name, texts = %v", tt.Texts())
	}
	if got := len(tt.Texts()); got != 2 {
		t.Fatalf("a card must stay at two text lines, got %d: %v", got, tt.Texts())
	}
}
```

`internal/ui/flow_data_test.go`：

```go
package ui

import (
	"testing"

	"github.com/oneo-me/onevra/source/components/flow"

	"github.com/futuremeng/OpenArchitect/internal/core/graph"
	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

// newFlowApp 不碰数据库：flowData 只读内存里的图。
func newFlowApp(t *testing.T) *App {
	t.Helper()
	elements := []model.Element{
		{ID: model.SystemID("example.com/x"), Type: model.TypeSystem, Name: "x"},
		{ID: model.ContainerID("example.com/x/cmd/api"), Type: model.TypeContainer, Name: "api"},
		{ID: model.ComponentID("example.com/x/internal/basket", "Basket"), Type: model.TypeComponent, Name: "Basket"},
		{ID: model.ComponentID("example.com/x/internal/basket", "Item"), Type: model.TypeComponent, Name: "Item"},
		{ID: model.ComponentID("example.com/x/internal/orders", "Orders"), Type: model.TypeComponent, Name: "Orders"},
	}
	rels := []model.Relationship{
		{FromID: model.SystemID("example.com/x"), ToID: model.ContainerID("example.com/x/cmd/api"), Type: model.RelContains, Weight: 1},
		{FromID: model.ComponentID("example.com/x/internal/basket", "Basket"), ToID: model.ComponentID("example.com/x/internal/basket", "Item"), Type: model.RelCalls, Weight: 40},
		{FromID: model.ComponentID("example.com/x/internal/orders", "Orders"), ToID: model.ComponentID("example.com/x/internal/basket", "Basket"), Type: model.RelDependsOn, Weight: 2},
		{FromID: model.ContainerID("example.com/x/cmd/api"), ToID: model.ComponentID("example.com/x/internal/basket", "Basket"), Type: model.RelCalls, Weight: 9},
	}
	a := NewApp(nil)
	a.graph = graph.Build(elements, rels)
	return a
}

func TestFlowDataFixesEveryCardSize(t *testing.T) {
	data := newFlowApp(t).flowData()
	if len(data.Nodes) != 5 {
		t.Fatalf("nodes = %d", len(data.Nodes))
	}
	for _, n := range data.Nodes {
		if n.Width != cardWidth || n.Height != cardHeight {
			t.Fatalf("node %s sized %gx%g, must be fixed at %gx%g", n.ID, n.Width, n.Height, cardWidth, cardHeight)
		}
		if n.ID == "" || n.Label == "" || n.Content == nil {
			t.Fatalf("node %+v", n)
		}
	}
}

func TestQuietSetNilWithoutImpactMode(t *testing.T) {
	a := newFlowApp(t)
	if a.quietSet() != nil {
		t.Fatal("no impact mode means nothing fades")
	}
	a.impactMode = true
	if a.quietSet() != nil {
		t.Fatal("mode on but no centre yet: still nothing should fade")
	}
	data := a.flowData()
	for _, n := range data.Nodes {
		if n.Muted {
			t.Fatalf("node %s muted with no report", n.ID)
		}
	}
}

func TestFlowDataMutesOutsideImpact(t *testing.T) {
	a := newFlowApp(t)
	center := model.ComponentID("example.com/x/internal/basket", "Basket")
	a.impactMode = true
	a.impact = &graph.ImpactReport{
		Center:     model.Element{ID: center},
		Downstream: []graph.Hit{{Element: model.Element{ID: model.ComponentID("example.com/x/internal/basket", "Item")}, Depth: 1}},
	}
	muted := map[string]bool{}
	for _, n := range a.flowData().Nodes {
		muted[n.ID] = n.Muted
	}
	if muted[center] || muted[model.ComponentID("example.com/x/internal/basket", "Item")] {
		t.Fatalf("the impact set must stay bright: %v", muted)
	}
	if !muted[model.SystemID("example.com/x")] || !muted[model.ContainerID("example.com/x/cmd/api")] {
		t.Fatalf("everything outside the impact set must fade: %v", muted)
	}
}

func TestFlowEdgesStyleAndSkip(t *testing.T) {
	edges := newFlowApp(t).flowEdges()
	// 同包的那条 calls（Basket→Item，都在 internal/basket）不画（spec §6.4）。
	for _, e := range edges {
		if e.Source == model.ComponentID("example.com/x/internal/basket", "Basket") &&
			e.Target == model.ComponentID("example.com/x/internal/basket", "Item") {
			t.Fatalf("same-package edge must not be drawn: %+v", e)
		}
	}
	byPair := map[[2]string]flow.Edge{}
	for _, e := range edges {
		byPair[[2]string{e.Source, e.Target}] = e
	}
	dep := byPair[[2]string{
		model.ComponentID("example.com/x/internal/orders", "Orders"),
		model.ComponentID("example.com/x/internal/basket", "Basket"),
	}]
	if !dep.Dashed || dep.Label != string(model.RelDependsOn) {
		t.Fatalf("depends_on = %+v", dep)
	}
	calls := byPair[[2]string{
		model.ContainerID("example.com/x/cmd/api"),
		model.ComponentID("example.com/x/internal/basket", "Basket"),
	}]
	if calls.Dashed || calls.Width < 2 || calls.Width > 4 {
		t.Fatalf("calls with weight 9 = %+v", calls)
	}
	contains := byPair[[2]string{model.SystemID("example.com/x"), model.ContainerID("example.com/x/cmd/api")}]
	if contains.Label != string(model.RelContains) || contains.Width != 0 {
		t.Fatalf("contains = %+v", contains)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `export PATH="/usr/local/go/bin:$PATH" && go test ./internal/ui/ -run 'Truncate|Icon|NodeCard|FlowData|QuietSet|FlowEdges'`
Expected: 编译失败，`undefined: truncateMiddle`、`undefined: iconFor`、`undefined: (*App).flowData`。

- [ ] **Step 3: 实现 node_card.go**

`internal/ui/node_card.go`：

```go
package ui

import (
	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/components/flow"
	"github.com/oneo-me/onevra/source/icons/lucide"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

// cardWidth/cardHeight fix every node's size. flow measures Content when the
// size is zero and re-runs layout whenever a measured size changes, so
// content-sized cards make the map jitter frame after frame (spec §9.3).
const (
	cardWidth  = float32(168)
	cardHeight = float32(46)
)

// pkgBudget is a rune count, not DIPs: the card is a fixed size, so a
// character budget is the honest limit and needs no font measurement.
const pkgBudget = 20

func iconFor(t model.ElementType) *ui.SVG {
	switch t {
	case model.TypeSystem:
		return lucide.Globe()
	case model.TypeContainer:
		return lucide.SquareTerminal()
	default:
		return lucide.Box()
	}
}

// nodeCard is one icon and two text lines. flow rebuilds every card on every
// frame, and the measured baseline is 397 µs for 1000 plain labels: a button,
// a progress bar or a third line inside here trades the map's frame time for
// nothing (spec §2.1, spec §9.3).
func nodeCard(c *ui.Context, el model.Element) {
	t := c.Theme()
	ui.Row(c).Size(cardWidth, cardHeight).Gap(t.Space(1.5)).AlignItems(ui.Center).
		Padding(t.Space(1.5), t.Space(2)).Radius(flow.CardRadius).
		Background(t.Surface).Border(1, t.Border).Children(func() {
		ui.Icon(c, iconFor(el.Type)).Size(14, 14)
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			components.Text(c, el.Name).SingleLine().FontWeight(600).TextColor(t.Text)
			components.Text(c, truncateMiddle(model.PkgPathOf(el.ID), pkgBudget)).
				SingleLine().FontSize(t.Rem(0.8)).TextColor(t.TextMuted)
		})
	})
}

// truncateMiddle keeps both ends of a package path: the head says which module
// and the tail says which package, and either half alone is unreadable.
func truncateMiddle(s string, keep int) string {
	r := []rune(s)
	if len(r) <= keep {
		return s
	}
	if keep < 5 {
		return "…"
	}
	head := (keep - 1) / 2
	tail := keep - 1 - head
	return string(r[:head]) + "…" + string(r[len(r)-tail:])
}
```

圆角用 `flow.CardRadius`（`components/flow/card.go:9`，10 DIP）而不是自造常量：flow 画的选中环跟着它，两边一致才看得出「这一张被选中了」。

- [ ] **Step 4: 实现 flow_data.go**

`internal/ui/flow_data.go`：

```go
package ui

import (
	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components/flow"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

// flowData is the whole App-to-flow translation. Elements come out of
// graph.Elements() already sorted by id, so node order — and with it flow's
// keyboard order and PoC-4's position drift check — is stable across frames.
func (a *App) flowData() flow.Data {
	elements := a.graph.Elements()
	keep := a.quietSet()
	nodes := make([]flow.Node, 0, len(elements))
	for _, el := range elements {
		node := flow.Node{
			ID:      el.ID,
			Label:   el.Name + "，" + model.PkgPathOf(el.ID),
			Width:   cardWidth,
			Height:  cardHeight,
			Content: func(c *ui.Context) { nodeCard(c, el) },
		}
		if keep != nil {
			node.Muted = !keep[el.ID]
		}
		nodes = append(nodes, node)
	}
	return flow.Data{Nodes: nodes, Edges: a.flowEdges()}
}

// quietSet is the ids that stay bright this frame, or nil when nothing should
// fade. Computed once per flowData call: doing it per node would run one BFS
// per card.
func (a *App) quietSet() map[string]bool {
	if !a.impactMode || a.impact == nil {
		return nil
	}
	keep := make(map[string]bool, len(a.impact.Upstream)+len(a.impact.Downstream)+1)
	for _, id := range a.impact.AffectedIDs() {
		keep[id] = true
	}
	return keep
}

func (a *App) flowEdges() []flow.Edge {
	rels := a.graph.Relationships()
	edges := make([]flow.Edge, 0, len(rels))
	for _, rel := range rels {
		if rel.FromID == rel.ToID {
			continue
		}
		// spec §6.4: 包内耦合存着但不画 —— 分层布局画不出同排节点之间的边。
		if rel.Type != model.RelContains && samePackage(rel.FromID, rel.ToID) {
			continue
		}
		edge := flow.Edge{Source: rel.FromID, Target: rel.ToID, Label: string(rel.Type)}
		switch rel.Type {
		case model.RelDependsOn:
			edge.Dashed = true
		case model.RelCalls:
			// 强依赖更粗，封顶到 4 DIP（1 + min(3, weight/8)）。
			width := 1 + float32(rel.Weight)/8
			if width > 4 {
				width = 4
			}
			edge.Width = width
		}
		edges = append(edges, edge)
	}
	return edges
}

func samePackage(from, to string) bool {
	p := model.PkgPathOf(from)
	return p != "" && p == model.PkgPathOf(to)
}
```

边的顺序不用再排：`store.Relationships` 的 SQL 就是 `ORDER BY from_id, to_id, relationship_type`（Task 7），节点顺序来自 `graph.Elements()` 的按 id 排序，两边都稳定，flow 才可能过 PoC-4 的「两次 Advance 位置不漂移」。

- [ ] **Step 5: 跑测试确认通过**

Run: `export PATH="/usr/local/go/bin:$PATH" && go test ./internal/ui/ -v`
Expected: 全部 PASS，含 Task 16 的状态机测试。

- [ ] **Step 6: 提交**

```sh
go vet ./... && go test ./...
git add internal/ui/
git commit -m "feat: 168×46 固定尺寸节点卡片与 flow 数据装配（含淡出与边样式）"
```

---

### Task 18: 详情抽屉（右侧元素详情与方法下钻）

**Files:**
- Create: `internal/ui/detail_panel.go`
- Test: `internal/ui/detail_panel_test.go`

**Interfaces:**
- Consumes: Task 16 的 `App`（`selected`、`drillInto`、`impact`、`graph`、`settings`、`flowState`、`Select`、`Open`、`OpenFile`、`SetOpenPath`、`openPath`）、Task 12/13 的 `Graph.Element/Relationships/Upstream/Downstream` 与 `graph.IntraPackageIDs`、Task 17 的 `iconFor`、Task 5 的 `model.Element.Methods`（定稿 #1）
- Produces（Task 19 只认这些）：
  - `const detailWidth = float32(320)`、`const methodPreview = 3`
  - `func (a *App) detailPanel(c *ui.Context)`
  - `func (a *App) containersOf(id string) []model.Element`
  - `func previewMethods(el model.Element, expanded bool) (shown []string, hidden int)`
  - `func (a *App) jumpTo(id string)`

抽屉的全部价值在于把 `contains` 的多归属歧义摆出来（spec §6.3、§9.4）：共享库里的同一个类型可以同时属于两个 cmd 二进制，`containersOf` 返回的是**集合**，不是"第一个"。所以 Task 7 把 contains 从 BFS 剔除、Task 17 把 contains 不画成边之后，这一层是它唯一的可见处。

**点击语义有两条会从下面咬上来，都在 mygo 源码里核实过，Task 19–22 同样靠它们：**

1. 一次按下只交给命中链里**最内层的 interactive 节点**，父容器不会跟着收到（`ui/input.go:270-277` 取 `target`、`:413` 只给 `target.clicks++`）。而 `Clicked()` 就是那个标志：它给自己置 `flagClickable`（`input.go:979`）。所以「点整行」就只轮询行，「点文字」就只轮询文字，同一处不能两个都要 —— 下面的 chip 轮询行、文件路径行轮询整行，两颗按钮的命中区都按这个选。
2. 标志位是**上一帧**置的，`ui.NewTester` 之后要先 `tt.Frame()` 跑一帧；点击本身不需要再补 `Frame()`（`send` 会 `settle`，`headless.go:351-358`），但**点击之后又改了状态、要断言渲染结果**时得再跑一帧 —— `TestDetailMethodsHeaderClickExpands` 里那第二次 `tt.Frame()` 就是为这个，删掉它测试会变成假绿。

- [ ] **Step 1: 写失败测试**

`internal/ui/detail_panel_test.go`：

```go
package ui

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/futuremeng/OpenArchitect/internal/core/graph"
	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

const detailModule = "example.com/x"

var detailFile = filepath.Join("internal", "basket", "basket.go")

// newDetailApp 另造一份图，不用 Task 17 的 newFlowApp：抽屉要断言的字段
// （file_path、doc comment、方法列表、多归属）在 flow 数据里一个都用不到，
// 那份 fixture 不带它们。
func newDetailApp(t *testing.T) *App {
	t.Helper()
	pkg := detailModule + "/internal/basket"
	basket := model.Element{
		ID:          model.ComponentID(pkg, "Basket"),
		Type:        model.TypeComponent,
		Name:        "Basket",
		Description: "Basket 管理一次结账的全部条目。",
		FilePath:    detailFile,
		Methods:     []string{"Add", "Remove", "Total", "Clear"},
	}
	item := model.Element{
		ID:       model.ComponentID(pkg, "Item"),
		Type:     model.TypeComponent,
		Name:     "Item",
		FilePath: filepath.Join("internal", "basket", "item.go"),
	}
	orders := model.Element{
		ID:       model.ComponentID(detailModule+"/internal/orders", "Orders"),
		Type:     model.TypeComponent,
		Name:     "Orders",
		FilePath: filepath.Join("internal", "orders", "orders.go"),
	}
	api := model.Element{ID: model.ContainerID(detailModule + "/cmd/api"), Type: model.TypeContainer, Name: "api"}
	worker := model.Element{
		ID:   model.ContainerID(detailModule + "/cmd/worker"),
		Type: model.TypeContainer,
		Name: "worker",
	}
	system := model.Element{ID: model.SystemID(detailModule), Type: model.TypeSystem, Name: "x"}
	rels := []model.Relationship{
		{FromID: system.ID, ToID: api.ID, Type: model.RelContains, Weight: 1},
		{FromID: system.ID, ToID: worker.ID, Type: model.RelContains, Weight: 1},
		// 同一个 Basket 属于两个二进制 —— §6.3 的多归属，抽屉必须整个摆出来。
		{FromID: api.ID, ToID: basket.ID, Type: model.RelContains, Weight: 1},
		{FromID: worker.ID, ToID: basket.ID, Type: model.RelContains, Weight: 1},
		{FromID: basket.ID, ToID: item.ID, Type: model.RelCalls, Weight: 3},
		{FromID: orders.ID, ToID: basket.ID, Type: model.RelDependsOn, Weight: 2},
	}
	a := NewApp(nil)
	a.graph = graph.Build(
		[]model.Element{system, api, worker, basket, item, orders},
		rels,
	)
	a.selected = basket.ID
	return a
}

func elementNames(elements []model.Element) []string {
	out := make([]string, len(elements))
	for i, el := range elements {
		out[i] = el.Name
	}
	return out
}

func newDetailTester(t *testing.T, a *App) *ui.Tester {
	t.Helper()
	tt := ui.NewTester(func(c *ui.Context) { a.detailPanel(c) }, 320, 900)
	tt.Frame()
	return tt
}

func TestPreviewMethods(t *testing.T) {
	el := model.Element{Methods: []string{"Add", "Remove", "Total", "Clear"}}
	if shown, hidden := previewMethods(el, false); len(shown) != methodPreview || hidden != 1 {
		t.Fatalf("collapsed = %v, hidden %d", shown, hidden)
	}
	if shown, hidden := previewMethods(el, true); len(shown) != 4 || hidden != 0 {
		t.Fatalf("expanded = %v, hidden %d", shown, hidden)
	}
	short := model.Element{Methods: []string{"Add"}}
	if shown, hidden := previewMethods(short, false); len(shown) != 1 || hidden != 0 {
		t.Fatalf("a short list hides nothing: %v, hidden %d", shown, hidden)
	}
	if shown, hidden := previewMethods(model.Element{}, false); len(shown) != 0 || hidden != 0 {
		t.Fatalf("no methods: %v, hidden %d", shown, hidden)
	}
}

func TestContainersOfReturnsEveryOwner(t *testing.T) {
	a := newDetailApp(t)
	got := elementNames(a.containersOf(a.selected))
	if len(got) != 2 || got[0] != "api" || got[1] != "worker" {
		t.Fatalf("containers = %v, want both binaries", got)
	}
	if len(a.containersOf(model.SystemID(detailModule))) != 0 {
		t.Fatal("a System has no Container above it")
	}
	if len(a.containersOf("cmp:nowhere.Nope")) != 0 {
		t.Fatal("unknown id is empty, not an error")
	}
}

func TestDetailPanelRendersFacts(t *testing.T) {
	a := newDetailApp(t)
	tt := newDetailTester(t, a)
	for _, want := range []string{
		"Basket", "Component",
		detailModule + "/internal/basket",
		"api、worker",
		detailFile,
		"Basket 管理一次结账的全部条目。",
		"导出方法（4）", "Add", "Total",
		"直接上游", "直接下游", "Orders", "Item",
		"同包类型",
	} {
		if !tt.HasText(want) {
			t.Fatalf("%q missing, texts = %v", want, tt.Texts())
		}
	}
	// 折叠状态下第 4 个方法不该在帧里 —— 它在，就是展开逻辑没接上。
	if tt.HasText("Clear") {
		t.Fatalf("a collapsed list must not render every method, texts = %v", tt.Texts())
	}
}

func TestDetailCloseClickClearsSelection(t *testing.T) {
	a := newDetailApp(t)
	tt := newDetailTester(t, a)
	if err := tt.Click("收起详情"); err != nil {
		t.Fatal(err)
	}
	if a.selected != "" {
		t.Fatalf("close must clear the selection, got %q", a.selected)
	}
}

func TestDetailMethodsHeaderClickExpands(t *testing.T) {
	a := newDetailApp(t)
	tt := newDetailTester(t, a)
	if err := tt.Click("导出方法（4）"); err != nil {
		t.Fatal(err)
	}
	if a.drillInto != a.selected {
		t.Fatalf("drillInto = %q, selected = %q", a.drillInto, a.selected)
	}
	// 这一帧画的还是折叠列表（shown 在点击前算好），所以断言渲染要再跑一帧。
	tt.Frame()
	if !tt.HasText("Clear") {
		t.Fatalf("expanded list must show every method, texts = %v", tt.Texts())
	}
}

func TestNeighbourChipJumpsToThatElement(t *testing.T) {
	a := newDetailApp(t)
	tt := newDetailTester(t, a)
	if err := tt.Click("Orders"); err != nil {
		t.Fatal(err)
	}
	if want := model.ComponentID(detailModule+"/internal/orders", "Orders"); a.selected != want {
		t.Fatalf("selected = %q, want %q", a.selected, want)
	}
}

func TestFilePathRowReachesTheInjectedOpener(t *testing.T) {
	a := newDetailApp(t)
	var opened []string
	a.SetOpenPath(func(path string) error {
		opened = append(opened, path)
		return nil
	})
	tt := newDetailTester(t, a)
	if err := tt.Click(detailFile); err != nil {
		t.Fatal(err)
	}
	if len(opened) != 1 || opened[0] != detailFile {
		t.Fatalf("opened = %v, want [%s]", opened, detailFile)
	}
}

// 只有当前选中元素的路径进抽屉，邻居 chip 只带名字。守的是取数范围：别顺手
// 把整个 snapshot 的 file_path 都渲染出来，那会是几百行。
func TestDrawerShowsOnlyTheSelectedElement(t *testing.T) {
	a := newDetailApp(t)
	tt := newDetailTester(t, a)
	if _, ok := tt.Find("internal/basket/item.go"); ok {
		t.Fatal("a neighbour chip carries a name, not its file path")
	}
}
```

`Find` 是整串相等、`HasText` 是 `Contains`（`headless.go:441`、`:460`），所以"不该出现"的断言只能用 `Find`。

- [ ] **Step 2: 跑测试确认失败**

Run: `export PATH="/usr/local/go/bin:$PATH" && go test ./internal/ui/ -run 'Detail|Drawer|ContainersOf|PreviewMethods|NeighbourChip|FilePath'`
Expected: 编译失败，`undefined: (*App).detailPanel`、`undefined: previewMethods`、`undefined: (*App).containersOf`、`undefined: methodPreview`。

- [ ] **Step 3: 实现 detail_panel.go**

```go
package ui

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
	"github.com/oneo-me/onevra/source/shells/sidebar"

	"github.com/futuremeng/OpenArchitect/internal/core/graph"
	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

const (
	// detailWidth 是写死的：抽屉一宽，flow 那一侧的可用宽度就每帧不变。
	// 让它随内容伸缩，选中与取消选中会来回重排整张图（同 Task 17 定死
	// 卡片尺寸的理由）。
	detailWidth = float32(320)
	// methodPreview 是折叠时露出的方法数。
	methodPreview = 3
)

func (a *App) detailPanel(c *ui.Context) {
	el, ok := a.graph.Element(a.selected)
	if !ok {
		// 重新扫描换了一张图，selected 还指着上一版的节点。这一帧不画，
		// selected 由下一次点击重写。
		return
	}
	t := c.Theme()
	ui.Column(c).Width(detailWidth).MinWidth(detailWidth).FillHeight().
		Padding(t.Space(2)).Gap(t.Space(2)).
		Background(t.Surface).Border(1, t.Border).Radius(t.Radius).
		Children(func() {
			a.detailHeader(c, el)
			ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
				ui.Column(c).FillWidth().Gap(t.Space(2)).Children(func() {
					a.detailFacts(c, el)
					a.detailMethods(c, el)
					a.neighbourGroup(c, "直接上游", a.graph.Upstream(el.ID, 1), lucide.ArrowUpRight)
					a.neighbourGroup(c, "直接下游", a.graph.Downstream(el.ID, 1), lucide.ArrowDownLeft)
					a.detailImpact(c, el)
				})
			})
		})
}

func (a *App) detailHeader(c *ui.Context, el model.Element) {
	t := c.Theme()
	ui.Row(c).FillWidth().Gap(t.Space(1.5)).AlignItems(ui.Start).Children(func() {
		components.Icon(c, iconFor(el.Type)).Size(18, 18)
		ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space(0.5)).Children(func() {
			components.Text(c, el.Name).FontWeight(600).FontSize(t.Rem(1.05)).
				SingleLine().TextColor(t.Text)
			components.Badge(c, string(el.Type), components.BadgeOptions{Tone: components.BadgeAccent})
		})
		dismiss := sidebar.IconButton(c, lucide.X(), "收起详情")
		// 关抽屉走 Select("")：它顺手把 impact 和 drillInto 一起清掉
		// （Task 16），这里不能只写 selected。
		if dismiss.Clicked() {
			a.Select("")
		}
	})
}

func (a *App) detailFacts(c *ui.Context, el model.Element) {
	t := c.Theme()
	if pkg := model.PkgPathOf(el.ID); pkg != "" {
		detailField(c, t, "包路径", pkg)
	}
	if owners := a.containersOf(el.ID); len(owners) > 0 {
		names := make([]string, len(owners))
		for i, owner := range owners {
			names[i] = owner.Name
		}
		detailField(c, t, "所属 Container", strings.Join(names, "、"))
	}
	a.filePathRow(c, el)
	if el.Description != "" {
		detailField(c, t, "文档注释", el.Description)
	}
	if siblings := len(graph.IntraPackageIDs(a.graph, el.ID)); siblings > 0 {
		detailField(c, t, "同包类型", fmt.Sprintf("%d 个", siblings))
	}
}

func detailField(c *ui.Context, t *ui.Theme, label, value string) {
	ui.Column(c).FillWidth().Gap(t.Space(0.5)).Children(func() {
		components.Text(c, label).FontSize(t.Rem(.75)).TextColor(t.TextMuted)
		components.Text(c, value).FontSize(t.Rem(.9)).TextColor(t.Text)
	})
}

// containersOf 反查 contains 边，取出所有把 id 装在里面的 Container。返回集合
// 而不是第一个，是 §6.3 多归属的可见处（spec §9.4）。顺序来自
// Graph.Relationships()，那个切片按 (from,to,type) 排过序，所以每帧一致。
func (a *App) containersOf(id string) []model.Element {
	var out []model.Element
	for _, rel := range a.graph.Relationships() {
		if rel.ToID != id || rel.Type != model.RelContains {
			continue
		}
		owner, ok := a.graph.Element(rel.FromID)
		if ok && owner.Type == model.TypeContainer {
			out = append(out, owner)
		}
	}
	return out
}

// previewMethods 折叠时只露 methodPreview 个。expanded 由 drillInto 决定，
// 也就是图上双击（flow 的 Open）或抽屉标题那一下。
func previewMethods(el model.Element, expanded bool) (shown []string, hidden int) {
	if expanded || len(el.Methods) <= methodPreview {
		return el.Methods, 0
	}
	return el.Methods[:methodPreview], len(el.Methods) - methodPreview
}

func (a *App) detailMethods(c *ui.Context, el model.Element) {
	if el.Type != model.TypeComponent || len(el.Methods) == 0 {
		return
	}
	t := c.Theme()
	shown, hidden := previewMethods(el, a.drillInto == el.ID)
	ui.Column(c).FillWidth().Gap(t.Space(0.5)).Children(func() {
		head := components.Text(c, fmt.Sprintf("导出方法（%d）", len(el.Methods))).
			FontSize(t.Rem(.75)).TextColor(t.TextMuted).Cursor(ui.CursorPointer)
		// 点标题展开，再点收回。图上双击走的是同一条路（Task 19 的 Open）。
		if head.Clicked() {
			if a.drillInto == el.ID {
				a.drillInto = ""
			} else {
				a.Open(el.ID)
			}
		}
		for _, method := range shown {
			ui.Row(c).Gap(t.Space(1)).AlignItems(ui.Center).Children(func() {
				components.Icon(c, lucide.Hash).Size(12, 12)
				components.Text(c, method).FontSize(t.Rem(.9)).SingleLine().TextColor(t.Text)
			})
		}
		if hidden > 0 {
			components.Text(c, fmt.Sprintf("还有 %d 个，点标题展开", hidden)).
				FontSize(t.Rem(.8)).TextColor(t.TextMuted)
		}
	})
}

func (a *App) neighbourGroup(c *ui.Context, label string, hits []graph.Hit, icon func() *ui.SVG) {
	if len(hits) == 0 {
		return
	}
	t := c.Theme()
	ui.Column(c).FillWidth().Gap(t.Space(0.5)).Children(func() {
		components.Text(c, fmt.Sprintf("%s（%d）", label, len(hits))).
			FontSize(t.Rem(.75)).TextColor(t.TextMuted)
		ui.Row(c).FillWidth().Wrap().Gap(t.Space(1)).Children(func() {
			for _, hit := range hits {
				chip := ui.Row(c).Gap(t.Space(0.5)).AlignItems(ui.Center).
					Padding(t.Space(0.25), t.Space(0.75)).Radius(t.Radius).Border(1, t.Border)
				chip.Children(func() {
					components.Icon(c, icon).Size(12, 12)
					components.Text(c, hit.Element.Name).FontSize(t.Rem(.85)).SingleLine().TextColor(t.Text)
				})
				// 轮询落在 chip 上，整颗 chip 都是命中区：chip 里的图标和
				// 文本没有 interactive 标志，抢不走这一击（input.go:275）。
				if chip.Clicked() {
					a.jumpTo(hit.Element.ID)
				}
			}
		})
	})
}

// jumpTo 是 chip 的去向：选中它并把图平移过去。Reveal 在图里没有这个节点时
// 什么都不做，所以影响模式下点一颗被淡出的邻居也不会出错。
func (a *App) jumpTo(id string) {
	a.Select(id)
	a.flowState.Reveal(id)
}

// filePathRow 是抽屉里唯一会离开 app 的动作。Task 22 装上 openPath 之前它照画，
// 只是没有手型、点了不动。
func (a *App) filePathRow(c *ui.Context, el model.Element) {
	if el.FilePath == "" {
		return
	}
	t := c.Theme()
	openable := a.openPath != nil
	row := ui.Row(c).Gap(t.Space(1)).AlignItems(ui.Center)
	row.Children(func() {
		components.Icon(c, lucide.FileCode).Size(13, 13)
		path := components.Text(c, el.FilePath).FontSize(t.Rem(.85)).SingleLine()
		if openable {
			path.Cursor(ui.CursorPointer).TextColor(t.Accent)
		} else {
			path.TextColor(t.TextMuted)
		}
	})
	// OpenPath 只在桌面环境拒绝时返回错误，为它存一条状态行要付一个 App 字段，
	// 而本地单用户重试的成本是零，所以这里把错误丢掉。
	if row.Clicked() && openable {
		_ = a.OpenFile(el.FilePath)
	}
}

func (a *App) detailImpact(c *ui.Context, el model.Element) {
	if a.impact == nil || a.impact.Center.ID != el.ID {
		return
	}
	t := c.Theme()
	ui.Column(c).FillWidth().Gap(t.Space(0.5)).Children(func() {
		components.Text(c, "影响范围").FontSize(t.Rem(.75)).TextColor(t.TextMuted)
		components.Text(c, fmt.Sprintf("上游 %d 个，下游 %d 个，深度 %d 跳",
			len(a.impact.Upstream), len(a.impact.Downstream), a.settings.ImpactDepth)).
			FontSize(t.Rem(.9)).TextColor(t.Text)
	})
}
```

以下几处写法是刻意的，别在后续任务里"顺手统一"：

- 元素取数只走 `a.graph.Element(a.selected)`，不查库。抽屉和图共用一份内存态图，是 DoD-4（CLI 与 GUI 同源）在 UI 侧的那一半。
- 邻居取数用 `Upstream(id, 1)/Downstream(id, 1)`，不用 `Neighbors`：`Neighbors` 不挑关系类型，会把 `contains` 的 api、worker 也算成"直接上游"，而它们已经在上一个字段里了。BFS 天生剔掉 contains（Task 12），深度 1 就是"直接"。
- `detailField` 是包级函数、不挂 `App`：它不读状态，挂上就等于骗读者说抽屉的字段是应用的一部分。
- chip 组用 `Wrap()` 而不滚动：一个热点类型的直接邻居可能有十几个，横排会溢出，纵向堆叠又会把 `Reveal` 的目标推出可视区。
- 图标传参有两种，别混：`components.Icon` 是泛型的，`*ui.SVG` 和 `func() *ui.SVG` 都收（`components/icon.go:16`），所以 `components.Icon(c, lucide.Hash)` 合法；但 `sidebar.IconButton(c, icon *ui.SVG, label string)` 只要已解析的那个，必须写 `lucide.X()`。`command.Item.Icon` 同样是 `*ui.SVG`（Task 20）。

- [ ] **Step 4: 跑测试确认通过**

Run: `export PATH="/usr/local/go/bin:$PATH" && go test ./internal/ui/ -v`
Expected: 全部 PASS，含 Task 16/17 的测试。

- [ ] **Step 5: 提交**

```sh
go vet ./... && go test ./...
git add internal/ui/detail_panel.go internal/ui/detail_panel_test.go
git commit -m "feat: 元素详情抽屉，多归属 Container 集合、方法下钻与邻居跳转"
```

---

### Task 19: 架构图页（flow 主视图、状态行、缩放控件）

**Files:**
- Create: `internal/ui/page_graph.go`
- Test: `internal/ui/page_graph_test.go`

**Interfaces:**
- Consumes: Task 17 的 `flowData/flowEdges/quietSet/cardWidth`、Task 18 的 `detailPanel/detailWidth`、Task 16 的 `App`（`selected`、`drillInto`、`impactMode`、`flowState`、`StatusLine`）
- Produces（Task 19/20/21 依赖）：
  - `func (a *App) graphPage(c *ui.Context)`
  - `func (a *App) graphOptions(data flow.Data) flow.Options`
  - `func (a *App) mapHeader(c *ui.Context)`
  - `func (a *App) zoomControls(c *ui.Context)`
  - `func emptyGraph(c *ui.Context)`

`graphOptions` 单独成一个函数，是因为 `flow.View` 把 `Select`/`Open` 吞进它自己的事件处理里，`ui.NewTester` 点不到节点。把回调的装配点抽出来，才能真测「单击接到 `App.Select`」这条线 —— 这正是 DoD-2 的接线处。

- [ ] **Step 1: 写失败测试**

`internal/ui/page_graph_test.go`：

```go
package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components/flow"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

func TestGraphOptionsWireCallbacks(t *testing.T) {
	a := newFlowApp(t)
	data := a.flowData()
	opts := a.graphOptions(data)
	target := model.ComponentID("example.com/x/internal/basket", "Basket")

	if opts.Selected != "" || opts.Direction != flow.LeftToRight {
		t.Fatalf("opts = %+v", opts)
	}
	if opts.NodeGap != 28 || opts.RankGap != 64 {
		t.Fatalf("gaps = %v/%v", opts.NodeGap, opts.RankGap)
	}
	if opts.Select == nil || opts.Open == nil || opts.Overlay == nil || opts.Empty == nil {
		t.Fatal("callbacks must all be wired; the map is useless without them")
	}
	opts.Select(target)
	if a.selected != target {
		t.Fatalf("Select did not reach App: %q", a.selected)
	}
	opts.Open(target)
	if a.drillInto != target {
		t.Fatalf("Open did not reach App: %q", a.drillInto)
	}
	opts.Select("")
	if a.selected != "" {
		t.Fatalf("blank select must clear, got %q", a.selected)
	}
	if opts.Label == "" || opts.EmptyText == "" {
		t.Fatalf("label/emptyText = %q/%q", opts.Label, opts.EmptyText)
	}
	if len(opts.Selected) == 0 && a.selected != "" {
		t.Fatal("Selected must echo App.selected")
	}
}

func TestGraphOptionsSelectedEchoesApp(t *testing.T) {
	a := newFlowApp(t)
	target := model.ComponentID("example.com/x/internal/orders", "Orders")
	a.selected = target
	if got := a.graphOptions(a.flowData()).Selected; got != target {
		t.Fatalf("Selected = %q want %q", got, target)
	}
}

// TestGraphPageRendersEmptyState 用 headless tester 起一帧：不开窗口，但真的
// 走了一遍 flow.View，所以 HeaderHeight、Empty 回调、主题取色都被执行过。
func TestGraphPageRendersEmptyState(t *testing.T) {
	a := NewApp(nil)
	tt := ui.NewTester(func(c *ui.Context) { a.graphPage(c) }, 900, 600)
	if !tt.HasText("架构图") {
		t.Fatalf("header missing, texts = %v", tt.Texts())
	}
	if !tt.HasText("还没有扫描过任何仓库") {
		t.Fatalf("empty state missing, texts = %v", tt.Texts())
	}
}

func TestGraphPageRendersNodesAndImpactSwitch(t *testing.T) {
	a := newFlowApp(t)
	tt := ui.NewTester(func(c *ui.Context) { a.graphPage(c) }, 900, 600)
	for _, want := range []string{"Basket", "Orders", "影响范围"} {
		if !tt.HasText(want) {
			t.Fatalf("%q missing, texts = %v", want, tt.Texts())
		}
	}
}

func TestMapHeaderRendersStatusLine(t *testing.T) {
	a := newFlowApp(t)
	a.status = StatusReady
	tt := ui.NewTester(func(c *ui.Context) { a.mapHeader(c) }, 900, 80)
	if !tt.HasText("5 个元素") {
		t.Fatalf("status line missing, texts = %v", tt.Texts())
	}
}
```

`5 个元素` 来自 `newFlowApp` 造的 5 个元素，`StatusLine` 在 `StatusReady` 分支拼这个数字。

- [ ] **Step 2: 跑测试确认失败**

Run: `export PATH="/usr/local/go/bin:$PATH" && go test ./internal/ui/ -run 'GraphOptions|GraphPage|MapHeader'`
Expected: 编译失败，`undefined: (*App).graphPage`、`undefined: (*App).graphOptions`。

- [ ] **Step 3: 实现 page_graph.go**

```go
package ui

import (
	"fmt"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/components/flow"
	"github.com/oneo-me/onevra/source/icons/lucide"
	"github.com/oneo-me/onevra/source/shells/sidebar"
)

func (a *App) graphPage(c *ui.Context) {
	t := c.Theme()
	data := a.flowData()
	ui.Column(c).Fill().Padding(sidebar.HeaderHeight(c)+8, 12, 12, 12).
		Gap(t.Space(2)).Children(func() {
		a.mapHeader(c)
		ui.Row(c).Grow(1).MinHeight(0).Gap(t.Space(3)).Children(func() {
			flow.View(c, &a.flowState, data, a.graphOptions(data)).Grow(1).MinWidth(0)
			if a.selected != "" {
				a.detailPanel(c)
			}
		})
	})
}

// graphOptions is split out of graphPage so a test can take the callbacks:
// flow.View swallows Select and Open into its own hit-testing, which a headless
// tester cannot click through. This is where DoD-2's wiring lives.
func (a *App) graphOptions(data flow.Data) flow.Options {
	return flow.Options{
		Label:     fmt.Sprintf("%d 个元素，%d 条关系", len(data.Nodes), len(data.Edges)),
		EmptyText: "还没有扫描过任何仓库",
		Direction: flow.LeftToRight,
		Selected:  a.selected,
		Select:    a.Select,
		Open:      a.Open,
		NodeGap:   28,
		RankGap:   64,
		Empty:     emptyGraph,
		Overlay:   a.zoomControls,
	}
}

func (a *App) mapHeader(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).FillWidth().Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
		components.Text(c, "架构图").FontWeight(600).TextColor(t.Text)
		ui.Spacer(c)
		// The switch writes a.impactMode itself, so Changed only has to fix up
		// the report that goes with it.
		if ui.Switch(c, &a.impactMode).Changed() {
			a.SetImpactMode(a.impactMode)
		}
		components.Text(c, "影响范围").FontSize(t.Rem(.85)).TextColor(t.TextMuted)
		components.Text(c, a.StatusLine()).FontSize(t.Rem(.85)).TextColor(t.TextMuted)
	})
}

func (a *App) zoomControls(c *ui.Context) {
	t := c.Theme()
	row := ui.Row(c).Gap(t.Space(1)).Padding(t.Space(1)).Radius(t.Radius).Background(t.Surface)
	row.Children(func() {
		for _, control := range []struct {
			label string
			icon  func() *ui.SVG
			apply func()
		}{
			{"适应窗口", lucide.Maximize, a.flowState.Fit},
			{"放大", lucide.ZoomIn, a.flowState.ZoomIn},
			{"缩小", lucide.ZoomOut, a.flowState.ZoomOut},
		} {
			button := sidebar.IconButton(c, control.icon(), control.label)
			if button.Clicked() {
				control.apply()
			}
		}
	})
}

func emptyGraph(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Gap(t.Space(2)).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
		components.Icon(c, lucide.Network).Size(28, 28)
		components.Text(c, "还没有扫描过任何仓库").TextColor(t.TextMuted)
		components.Text(c, "在左栏点「扫描」，或按 ⌘K 搜索元素").
			FontSize(t.Rem(.85)).TextColor(t.TextMuted)
	})
}
```

`Overlay` 画在图的右上角（`flow.go` 里是 `Absolute().Top(inset).Right(inset)`），所以这一簇按钮不能 `FillWidth`；`sidebar.IconButton` 的面是透明的，靠行的 `Background(t.Surface)` 把三个托成一簇。

- [ ] **Step 4: 跑测试确认通过**

Run: `export PATH="/usr/local/go/bin:$PATH" && go test ./internal/ui/ -v`
Expected: PASS。`graphPage` 里的 `a.detailPanel(c)` 来自上一个任务，这是 18→19 顺序的唯一理由，别调换。

- [ ] **Step 5: 提交**

```sh
go vet ./... && go test ./...
git add internal/ui/page_graph.go internal/ui/page_graph_test.go
git commit -m "feat: 架构图页，flow 主视图与缩放、影响范围开关"
```

---

### Task 20: 命令面板（⌘K 搜索元素与跳转）

**Files:**
- Create: `internal/ui/command.go`
- Modify: `internal/ui/state.go`（给 `App` 加一个字段，见下）
- Test: `internal/ui/command_test.go`

**Interfaces:**
- Consumes: Task 7 的 `store.SearchElements`、Task 16 的 `App`（`commandOpen`、`SetCommandOpen`、`selected`、`snapshot`、`db`、`graph`、`flowState`、`Select`、`SetPage`、`SetImpactMode`、`BeginScan`、`scanRoot`）、Task 17/18 的图与抽屉
- Produces（Task 22 只认这些）：
  - `const commandScanID = "cmd:scan"`、`commandReposID = "cmd:repos"`、`commandSettingsID = "cmd:settings"`
  - `const elementItemPrefix = "element:"`、`impactItemPrefix = "impact:"`
  - `func (a *App) commandPalette(c *ui.Context)`
  - `func (a *App) commandItems(query string) ([]command.Item, error)`
  - `func (a *App) runCommand(id string)`
  - `func filterCommands(items []command.Item, query string) []command.Item`

`command.State` 是宿主必须持有的（`command.go:30` "State is retained by the host for one command view; do not copy it after use"）。所以 `App` 加一个字段，Task 20 顺手把它写进 `state.go`：

```go
	// commandState 只能在主线程读写：command.State 明确禁止拷贝使用。
	commandState command.State
```

`CommandDialog` 的契约是「每帧都调，用宿主保留的 open 和 state」（`dialog.go:9-14`），所以 `commandPalette` 在关闭状态也要跑，不能写成 `if a.commandOpen { ... }`。它由 Task 22 的 `content` 每帧调用。

- [ ] **Step 1: 写失败测试**

`internal/ui/command_test.go`：

```go
package ui

import (
	"context"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components/command"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

func commandIDs(items []command.Item) []string {
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = item.ID
	}
	return out
}

func TestCommandItemsWithoutSnapshotAreStaticOnly(t *testing.T) {
	a := NewApp(nil)
	items, err := a.commandItems("Basket")
	if err != nil {
		t.Fatal(err)
	}
	// 没扫过仓库就没有可搜索的快照，静态条目照常给——面板不是空图时才出现
	// 的东西，它恰恰是这时候的入口。
	if len(items) != 2 || items[0].ID != commandScanID || items[1].ID != commandSettingsID {
		t.Fatalf("items = %v", commandIDs(items))
	}
	for _, item := range items {
		if item.Title == "" || item.Group == "" {
			t.Fatalf("item %+v must be displayable", item)
		}
	}
}

func TestCommandItemsAddsSelectedElementToImpact(t *testing.T) {
	a := newFlowApp(t)
	target := model.ComponentID("example.com/x/internal/orders", "Orders")
	a.selected = target
	items, err := a.commandItems("")
	if err != nil {
		t.Fatal(err)
	}
	// 影响分析条目指向"当前选中的那个"，所以它只在有选中时出现。
	if len(items) != 3 || items[2].ID != impactItemPrefix+target {
		t.Fatalf("items = %v", commandIDs(items))
	}
	if items[2].Title == "" {
		t.Fatal("the impact item must name its center")
	}
}

func TestCommandItemsSearchesCurrentSnapshot(t *testing.T) {
	a := newTestApp(t)
	seedScan(t, a, fixtureDir("simple-multi-main"))
	if err := a.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	items, err := a.commandItems("Cart")
	if err != nil {
		t.Fatal(err)
	}
	// fixture 的 module 是 example.com/simple（Task 9），里面的导出类型只有 Cart。
	want := elementItemPrefix + model.ComponentID("example.com/simple/internal/store", "Cart")
	var found bool
	for _, item := range items {
		if item.ID == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("search missed Cart, items = %v", commandIDs(items))
	}
}

func TestFilterCommandsLeavesDatabaseMatchesAlone(t *testing.T) {
	items := []command.Item{
		{ID: commandScanID, Title: "扫描当前仓库", Keywords: []string{"scan"}},
		{ID: elementItemPrefix + "cmp:a.Basket", Title: "Basket"},
	}
	// 库里已经按 query 筛过元素条目；再套一遍 command.Filter 的分词，
	// "bas 2" 这类查询会把命中清空。
	got := filterCommands(items, "bas 2")
	if len(got) != 1 || got[0].ID != commandScanID {
		t.Fatalf("static filter broken: %v", commandIDs(got))
	}
	got = filterCommands(items, "scan")
	if len(got) != 1 || got[0].ID != elementItemPrefix+"cmp:a.Basket" {
		t.Fatalf("element matches must survive: %v", commandIDs(got))
	}
	if got := filterCommands(items, ""); len(got) != 2 {
		t.Fatalf("empty query keeps everything: %v", commandIDs(got))
	}
}

func TestRunCommandElementSelectsAndReveals(t *testing.T) {
	a := newFlowApp(t)
	target := model.ComponentID("example.com/x/internal/orders", "Orders")
	a.SetPage(PageRepos)
	a.commandState.Query = "Orders"
	a.runCommand(elementItemPrefix + target)
	if a.commandOpen {
		t.Fatal("activating must close the panel")
	}
	if a.page != PageGraph || a.selected != target {
		t.Fatalf("page=%q selected=%q", a.page, a.selected)
	}
	if a.commandState.Query != "" {
		t.Fatalf("query must reset after activation: %q", a.commandState.Query)
	}
}

func TestRunCommandImpactTurnsModeOnBeforeSelecting(t *testing.T) {
	a := newFlowApp(t)
	target := model.ComponentID("example.com/x/internal/basket", "Basket")
	a.SetImpactMode(false)
	a.runCommand(impactItemPrefix + target)
	// 顺序有意义：Select 只在 impactMode 已开时才计算报告（Task 16）。
	if !a.impactMode || a.selected != target || a.impact == nil {
		t.Fatalf("mode=%v selected=%q impact=%v", a.impactMode, a.selected, a.impact)
	}
	if a.impact.Center.ID != target {
		t.Fatalf("center = %q", a.impact.Center.ID)
	}
}

func TestRunCommandUnknownIDIsNoop(t *testing.T) {
	a := newFlowApp(t)
	a.SetPage(PageRepos)
	a.runCommand("cmd:whatever")
	if a.page != PageRepos || a.selected != "" {
		t.Fatalf("page=%q selected=%q", a.page, a.selected)
	}
}

func TestCommandShortcutOpensPanel(t *testing.T) {
	a := NewApp(nil)
	tt := ui.NewTester(func(c *ui.Context) { a.commandPalette(c) }, 900, 700)
	tt.Frame()
	if a.commandOpen {
		t.Fatal("the panel starts closed")
	}
	tt.Key(ui.Cmd, ui.KeyK)
	if !a.commandOpen {
		t.Fatal("⌘K must open the panel")
	}
}
```

`TestCommandItemsSearchesCurrentSnapshot` 是 `store.SearchElements` 在 core 之外唯一的消费者：Task 7 测的是它的 SQL，这条测的是「面板真的会去调它，并且用的是当前快照的 id」。少了这条，把 `a.snapshot.ID` 写成空串只会让搜索静默返回空，SQL 测试照样绿。

`TestCommandShortcutOpensPanel` 只断言 `a.commandOpen`，不断言弹窗画出来了。留一个已知的降级口：如果 `components.Dialog` 在 headless 下因为原生窗口拖拽条 panic（`mygo` 的 `needsApp`，`app.go:130`），就在 `components.DialogOptions` 里加 `DisableWindowDrag: true`；仍然不行，就把这一条测试改成断言 `commandItems` 与 `runCommand`，把弹窗渲染交给 Task 24 的 GUI 验收，并在 `docs/superpowers/poc/2026-10-19-slice1-findings.md` 记一行原因。不要为了跑通而删掉断言。

- [ ] **Step 2: 跑测试确认失败**

Run: `export PATH="/usr/local/go/bin:$PATH" && go test ./internal/ui/ -run 'Command|FilterCommands|RunCommand'`
Expected: 编译失败，`undefined: (*App).commandItems`、`undefined: commandScanID`。

- [ ] **Step 3: 实现 command.go**

```go
package ui

import (
	"context"
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/components/command"
	"github.com/oneo-me/onevra/source/icons/lucide"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

const (
	commandScanID     = "cmd:scan"
	commandReposID    = "cmd:repos"
	commandSettingsID = "cmd:settings"
	// 搜索结果和影响分析指向元素，前缀后面就是 model.Element.ID。
	elementItemPrefix = "element:"
	impactItemPrefix  = "impact:"
)

func (a *App) commandPalette(c *ui.Context) {
	if c.Shortcut(ui.Cmd, ui.KeyK) {
		a.SetCommandOpen(true)
	}
	items, err := a.commandItems(a.commandState.Query)
	options := command.Options{
		SearchLabel:  "命令",
		Placeholder:  "搜索元素，或跳到仓库与设置",
		ResultsLabel: "结果",
		Filter:       filterCommands,
		OnActivate:   a.runCommand,
	}
	if err != nil {
		// Options.Error 会替掉结果列表并压制激活（command.go:71-73），
		// 所以库里查不到就是面板里一行字，不是静默的空结果。
		options.Error = err.Error()
	}
	command.CommandDialog(c, &a.commandOpen, &a.commandState, items, options,
		components.DialogOptions{Label: "命令面板"})
}

func (a *App) commandItems(query string) ([]command.Item, error) {
	items := []command.Item{
		{ID: commandScanID, Title: "扫描当前仓库", Group: "命令",
			Shortcut: "⌘R", Icon: lucide.Radar(), Keywords: []string{"scan", "扫描", "rescan"}},
		{ID: commandSettingsID, Title: "打开设置", Group: "命令",
			Icon: lucide.Settings(), Keywords: []string{"settings", "设置", "depth"}},
	}
	if a.selected != "" {
		name := a.selectedName()
		items = append(items, command.Item{
			ID:   impactItemPrefix + a.selected,
			Title: fmt.Sprintf("以 %s 为中心做影响分析", name),
			Description: name,
			Group:  "分析",
			Icon:   lucide.ScanSearch(),
			Keywords: []string{"impact", "影响", "analysis"},
		})
	}
	if a.db == nil || a.snapshot.ID == "" || strings.TrimSpace(query) == "" {
		return items, nil
	}
	elements, err := a.db.SearchElements(context.Background(), a.snapshot.ID, query)
	if err != nil {
		return items, err
	}
	for _, el := range elements {
		items = append(items, command.Item{
			ID:          elementItemPrefix + el.ID,
			Title:       el.Name,
			Description: model.PkgPathOf(el.ID),
			Group:       "元素",
			Icon:        iconFor(el.Type),
			Keywords:    []string{string(el.Type), model.PkgPathOf(el.ID)},
		})
	}
	return items, nil
}

// selectedName 是 a.graph 里的显示名；selected 可能指向上一版快照的节点，
// 那种情况下退化成 id 本身，面板条目仍然可点（点下去 detailPanel 会自己跳过）。
func (a *App) selectedName() string {
	if el, ok := a.graph.Element(a.selected); ok {
		return el.Name
	}
	return a.selected
}

// filterCommands 分两条路：元素条目是 store.SearchElements 按 query 取回来的，
// 原样保留；静态条目交给包里的 Filter 做标题/关键词匹配。
func filterCommands(items []command.Item, query string) []command.Item {
	if strings.TrimSpace(query) == "" {
		return items
	}
	var static, matched []command.Item
	for _, item := range items {
		if strings.HasPrefix(item.ID, elementItemPrefix) {
			matched = append(matched, item)
			continue
		}
		static = append(static, item)
	}
	return append(command.Filter(static, query), matched...)
}

func (a *App) runCommand(id string) {
	// 关闭和清空是宿主的事（command.go:76-78 的 OnActivate 只给 id）。
	a.SetCommandOpen(false)
	a.commandState.Query = ""
	switch {
	case id == commandScanID:
		if a.scanRoot != "" {
			// ErrScanInFlight 在这里不上屏：面板开着就说明用户刚点过扫描，
			// 侧栏状态行已经是 Scanning 了。
			_ = a.BeginScan(a.scanRoot)
		}
	case id == commandReposID:
		a.SetPage(PageRepos)
	case id == commandSettingsID:
		a.SetPage(PageSettings)
	case strings.HasPrefix(id, impactItemPrefix):
		target := strings.TrimPrefix(id, impactItemPrefix)
		a.SetPage(PageGraph)
		// 先开模式再选中：Select 只在 impactMode 为真时才算报告。
		a.SetImpactMode(true)
		a.Select(target)
	case strings.HasPrefix(id, elementItemPrefix):
		target := strings.TrimPrefix(id, elementItemPrefix)
		a.SetPage(PageGraph)
		a.Select(target)
		a.flowState.Reveal(target)
	}
}
```

`commandReposID` 在 `commandItems` 里没有对应条目 —— 仓库页在侧栏导航上始终可见，面板再放一条就是三个地方做同一件事。它保留在 `runCommand` 里，是因为 Task 21 的仓库页会在空态放一个「去设置扫描目录」的按钮，那条走的是 `commandSettingsID`；如果后续要把仓库页加回面板，加一条 `command.Item` 即可，`runCommand` 已经认这个 id。

- [ ] **Step 4: 给 App 补 commandState 字段**

`internal/ui/state.go` 的 `App` 里，紧跟 `flowState flow.State` 那一行之后插入：

```go
	// commandState 只在主线程读写：command.State 明令禁止拷贝使用
	// （components/command/command.go:30）。
	commandState command.State
```

并在同一文件的 import 块加 `"github.com/oneo-me/onevra/source/components/command"`。`state_test` 不需要改：`NewApp` 不初始化它，零值即「ready to use」（同处注释）。

- [ ] **Step 5: 跑测试确认通过**

Run: `export PATH="/usr/local/go/bin:$PATH" && go test ./internal/ui/ -v`
Expected: 全部 PASS。若 `TestCommandShortcutOpensPanel` 落在 Step 1 说明的降级分支上，findings 文件里必须已经写下那行原因。

- [ ] **Step 6: 提交**

```sh
go vet ./... && go test ./...
git add internal/ui/command.go internal/ui/command_test.go internal/ui/state.go
git commit -m "feat: ⌘K 命令面板，元素搜索与影响分析入口"
```

---

### Task 21: 仓库页与设置页

**Files:**
- Create: `internal/ui/page_repos.go`、`internal/ui/page_settings.go`
- Modify: `internal/ui/state.go`（`App` 加两个字段，见 Step 4）
- Test: `internal/ui/page_repos_test.go`、`internal/ui/page_settings_test.go`

**Interfaces:**
- Consumes: Task 16 的 `App`（`repos`、`settings`、`status`、`BeginScan`、`SetPost`、`SaveSettings`、`StatusLine`）、Task 5 的 `model.Repo`、Task 15 的 `config.DefaultImpactDepth`
- Produces（Task 22 只认这些）：
  - `func (a *App) reposPage(c *ui.Context)`
  - `func (a *App) settingsPage(c *ui.Context)`
  - `const minImpactDepth = 1`、`maxImpactDepth = 8`

**mygo v0.5.5 没有原生目录选择器**：root 包里和文件系统有关的只有 `Shell.OpenPath`（`modules.go:38`），既没有 `PickFolder` 也没有 `OpenFileDialog`。所以设置页的扫描目录是**手填**，靠 `BeginScan` 里那次同步的 `scanner.Detect` 兜底（spec §10 的第二行：不是 go 模块就当场失败，不起 goroutine）。这是 spec §9.1 的一处收窄，不是漏做。

**深度用加减按钮，不用 `components.NumberInput`**：`NumberInput(c, value *float64, lo, hi, step)` 要一个可寻址的 `*float64`，而 `config.Settings.ImpactDepth` 是 `int`，为它加一个 float 草稿字段或 `ui.Local` 都只为了一个 1…8 的整数。两颗按钮 + 一次边界检查，测试也更好点。

- [ ] **Step 1: 写失败测试**

`internal/ui/page_repos_test.go`：

```go
package ui

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

func TestReposPageEmptyState(t *testing.T) {
	a := NewApp(nil)
	tt := ui.NewTester(func(c *ui.Context) { a.reposPage(c) }, 900, 600)
	tt.Frame()
	if !tt.HasText("还没有扫描过仓库") {
		t.Fatalf("empty state missing, texts = %v", tt.Texts())
	}
	// 空态必须给出路：一句"去设置里填扫描目录"，而不是只有标题。
	if !tt.HasText("设置") {
		t.Fatalf("the empty state must point at the settings page, texts = %v", tt.Texts())
	}
}

func TestReposPageListsEveryRepo(t *testing.T) {
	a := newTestApp(t)
	seedScan(t, a, fixtureDir("simple-multi-main"))
	seedScan(t, a, fixtureDir("shared-lib"))
	if err := a.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(func(c *ui.Context) { a.reposPage(c) }, 900, 600)
	tt.Frame()
	for _, repo := range a.repos {
		for _, want := range []string{repo.Name, repo.LocalPath, repo.Lang} {
			if !tt.HasText(want) {
				t.Fatalf("%q missing for %s, texts = %v", want, repo.Name, tt.Texts())
			}
		}
	}
	if len(a.repos) != 2 {
		t.Fatalf("repos = %d", len(a.repos))
	}
}

// TestRepoScanButtonRunsToReady 是点击→扫描→状态回写这一整条链唯一的端到端
// 测试。post 换成往 channel 里投递，扫描 goroutine 就只在 channel 上交接，
// finishScan 由测试线程执行 —— 没有竞态，-race 也过。
func TestRepoScanButtonRunsToReady(t *testing.T) {
	a := newTestApp(t)
	seedScan(t, a, fixtureDir("simple-multi-main"))
	if err := a.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	// BeginScan 把传入路径 filepath.Abs 后才算 repo 主键（Task 16），所以
	// 这里必须用绝对路径比对；直接塞 fixtureDir 的相对路径会让最后的断言假失败。
	root, err := filepath.Abs(fixtureDir("shared-lib"))
	if err != nil {
		t.Fatal(err)
	}
	repo := model.Repo{
		ID:        model.RepoIDFor(root),
		Name:      "example.com/shared",
		LocalPath: root,
		Lang:      "go",
	}
	a.repos = []model.Repo{repo}

	done := make(chan func(), 1)
	a.SetPost(func(fn func()) { done <- fn })
	tt := ui.NewTester(func(c *ui.Context) { a.reposPage(c) }, 900, 600)
	tt.Frame()
	if err := tt.Click("扫描 " + repo.Name); err != nil {
		t.Fatal(err)
	}
	if a.status != StatusScanning {
		t.Fatalf("status right after the click = %v", a.status)
	}
	select {
	case fn := <-done:
		fn()
	case <-time.After(60 * time.Second):
		t.Fatal("the scan never reported back")
	}
	if a.status != StatusReady {
		t.Fatalf("status = %v err = %v", a.status, a.err)
	}
	if a.repoID != repo.ID {
		t.Fatalf("repoID = %q want %q", a.repoID, repo.ID)
	}
}

func TestRepoScanButtonDisabledWhileScanning(t *testing.T) {
	a := newTestApp(t)
	a.repos = []model.Repo{{ID: "repo:/x", Name: "x", LocalPath: t.TempDir(), Lang: "go"}}
	a.status = StatusScanning
	tt := ui.NewTester(func(c *ui.Context) { a.reposPage(c) }, 900, 600)
	tt.Frame()
	if err := tt.Click("扫描 x"); err != nil {
		t.Fatal(err)
	}
	// 门在 BeginScan 里（ErrScanInFlight），按钮的 Disabled 只是把它画出来：
	// 两条都得在，只留一条等于把状态机的一半交给 UI。
	if a.cancel != nil {
		t.Fatal("a gated scan must never start")
	}
	if a.status != StatusScanning {
		t.Fatalf("status = %v", a.status)
	}
}
```

`Disabled` 的点击语义已经核实：`release` 只在节点没有 `flagDisabled` 时才 `s.clicks++`（`input.go:407-414`），而 `tt.Click` 按 label 定位、不看 disabled，所以 `TestRepoScanButtonDisabledWhileScanning` 里 `tt.Click` 返回 nil、`Clicked()` 返回 false —— 这正是"按钮关掉了但测试点得到"的组合。

`internal/ui/page_settings_test.go`：

```go
package ui

import (
	"context"
	"errors"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/futuremeng/OpenArchitect/internal/config"
)

func TestSettingsPageShowsCurrentValues(t *testing.T) {
	a := newTestApp(t)
	a.settings = config.Settings{ScanRoot: "/repos/cart", ImpactDepth: 5}
	tt := ui.NewTester(func(c *ui.Context) { a.settingsPage(c) }, 900, 600)
	tt.Frame()
	for _, want := range []string{"设置", "/repos/cart", "影响分析深度", "5"} {
		if !tt.HasText(want) {
			t.Fatalf("%q missing, texts = %v", want, tt.Texts())
		}
	}
}

func TestSaveSettingsButtonPersists(t *testing.T) {
	a := newTestApp(t)
	a.settings = config.Settings{ScanRoot: "/repos/cart", ImpactDepth: config.DefaultImpactDepth}
	tt := ui.NewTester(func(c *ui.Context) { a.settingsPage(c) }, 900, 600)
	tt.Frame()
	if err := tt.Click("保存设置"); err != nil {
		t.Fatal(err)
	}
	if a.settingsErr != nil {
		t.Fatalf("save failed: %v", a.settingsErr)
	}
	if !a.settingsSaved {
		t.Fatal("the page must confirm what it just wrote")
	}
	got, err := config.Load(context.Background(), a.db)
	if err != nil {
		t.Fatal(err)
	}
	if got.ScanRoot != "/repos/cart" || got.ImpactDepth != config.DefaultImpactDepth {
		t.Fatalf("stored = %+v", got)
	}
}

func TestImpactDepthStepperStaysInBounds(t *testing.T) {
	a := newTestApp(t)
	a.settings.ImpactDepth = maxImpactDepth
	tt := ui.NewTester(func(c *ui.Context) { a.settingsPage(c) }, 900, 600)
	tt.Frame()
	if err := tt.Click("加深影响分析"); err != nil {
		t.Fatal(err)
	}
	if a.settings.ImpactDepth != maxImpactDepth {
		t.Fatalf("depth must clamp at %d, got %d", maxImpactDepth, a.settings.ImpactDepth)
	}
	a.settings.ImpactDepth = minImpactDepth
	tt.Frame()
	if err := tt.Click("减小影响分析深度"); err != nil {
		t.Fatal(err)
	}
	if a.settings.ImpactDepth != minImpactDepth {
		t.Fatalf("depth must clamp at %d, got %d", minImpactDepth, a.settings.ImpactDepth)
	}
	a.settings.ImpactDepth = config.DefaultImpactDepth
	tt.Frame()
	if err := tt.Click("减小影响分析深度"); err != nil {
		t.Fatal(err)
	}
	if a.settings.ImpactDepth != config.DefaultImpactDepth-1 {
		t.Fatalf("depth = %d", a.settings.ImpactDepth)
	}
}

func TestSettingsPageRendersSaveError(t *testing.T) {
	a := NewApp(nil)
	a.settingsErr = errors.New("attempt to write a readonly database")
	tt := ui.NewTester(func(c *ui.Context) { a.settingsPage(c) }, 900, 600)
	tt.Frame()
	if !tt.HasText("readonly database") {
		t.Fatalf("error not surfaced, texts = %v", tt.Texts())
	}
}
```

深度的边界只写两处：`config.Load` 把库里的坏值回落成默认（Task 15），两颗按钮在 `min/max` 上 `Disabled`。**不在渲染时 clamp** —— 视图写 `App` 字段违反 spec §9.2 的纯函数约定，而且库里既然不可能有坏值，那就是不存在的情形。

最后一条用 `NewApp(nil)`：`settingsErr` 是纯渲染输入，不需要库，也不该碰保存按钮 —— `save.Disabled(a.db == nil)` 就是为这一条存在的。

- [ ] **Step 2: 跑测试确认失败**

Run: `export PATH="/usr/local/go/bin:$PATH" && go test ./internal/ui/ -run 'ReposPage|RepoScan|SettingsPage|SaveSettings|ImpactDepth'`
Expected: 编译失败，`undefined: (*App).reposPage`、`undefined: (*App).settingsPage`、`undefined: maxImpactDepth`。

- [ ] **Step 3: 实现两页**

`internal/ui/page_repos.go`：

```go
package ui

import (
	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
	"github.com/oneo-me/onevra/source/shells/sidebar"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

func (a *App) reposPage(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Padding(sidebar.HeaderHeight(c)+8, 12, 12, 12).
		Gap(t.Space(2)).Children(func() {
		ui.Row(c).FillWidth().Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
			components.Text(c, "仓库").FontWeight(600).FontSize(t.Rem(1.15)).TextColor(t.Text)
			ui.Spacer(c)
			components.Text(c, a.StatusLine()).FontSize(t.Rem(.85)).TextColor(t.TextMuted)
		})
		if len(a.repos) == 0 {
			emptyRepos(c)
			return
		}
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).FillWidth().Gap(t.Space(1)).Children(func() {
				for _, repo := range a.repos {
					a.repoRow(c, repo)
				}
			})
		})
	})
}

// repoRow 的三行事实来自 arch_repos 那四列，本切片没有 URL、分支和提交数，
// 所以这里不放"最后扫描于"这类取不到的字段（spec 偏离 9）。
func (a *App) repoRow(c *ui.Context, repo model.Repo) {
	t := c.Theme()
	ui.Row(c).FillWidth().Gap(t.Space(2)).AlignItems(ui.Center).
		Padding(t.Space(1.5)).Radius(t.Radius).Border(1, t.Border).Children(func() {
		components.Icon(c, lucide.Package).Size(18, 18)
		ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space(0.5)).Children(func() {
			components.Text(c, repo.Name).FontWeight(600).TextColor(t.Text).SingleLine()
			components.Text(c, repo.LocalPath).FontSize(t.Rem(.85)).
				TextColor(t.TextMuted).SingleLine()
		})
		scan := sidebar.IconButton(c, lucide.Radar(), "扫描 "+repo.Name)
		// 扫描中就把这一行的按钮关掉：BeginScan 也会返回 ErrScanInFlight，
		// 但按钮亮着等于邀请用户去撞那个错误。
		scan.Disabled(a.status == StatusScanning)
		if scan.Clicked() {
			_ = a.BeginScan(repo.LocalPath)
		}
	})
}

func emptyRepos(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Grow(1).Gap(t.Space(2)).AlignItems(ui.Center).Justify(ui.Center).
		Children(func() {
		components.Icon(c, lucide.Package).Size(28, 28)
		components.Text(c, "还没有扫描过仓库").TextColor(t.TextMuted)
		components.Text(c, "在「设置」里填扫描目录，然后按 ⌘K 选「扫描当前仓库」。").
			FontSize(t.Rem(.85)).TextColor(t.TextMuted)
	})
}
```

`internal/ui/page_settings.go`：

```go
package ui

import (
	"context"
	"strconv"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
	"github.com/oneo-me/onevra/source/shells/sidebar"

	"github.com/futuremeng/OpenArchitect/internal/config"
)

const (
	minImpactDepth = 1
	maxImpactDepth = 8
)

func (a *App) settingsPage(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Padding(sidebar.HeaderHeight(c)+8, 12, 12, 12).
		Gap(t.Space(3)).Children(func() {
		components.Text(c, "设置").FontWeight(600).FontSize(t.Rem(1.15)).TextColor(t.Text)
		a.scanRootField(c)
		a.depthField(c)
		a.saveRow(c)
	})
}

// scanRootField 绑的是 a.settings.ScanRoot 本身：components.Text 的 Editable
// 每把编辑立刻写进绑定（text.go:6-10），而编辑回调在主线程，正好落在
// spec §9.2 允许写状态的那两处之一。
func (a *App) scanRootField(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).FillWidth().Gap(t.Space(1)).Children(func() {
		components.Text(c, "扫描目录").FontSize(t.Rem(.75)).TextColor(t.TextMuted)
		root := components.Text(c, "", components.TextOptions{Editable: &a.settings.ScanRoot})
		root.Label("扫描目录").FontSize(t.Rem(.95)).TextColor(t.Text).
			Padding(t.Space(1)).Radius(t.Radius).Border(1, t.Border).FillWidth().MinWidth(0)
		if root.Changed() {
			a.settingsSaved = false
		}
	})
	ui.Row(c).FillWidth().Gap(t.Space(1)).Children(func() {
		components.Icon(c, lucide.FolderOpen).Size(13, 13)
		components.Text(c, "填仓库根目录的绝对路径；mygo 没有原生目录选择器，只能手填。").
			FontSize(t.Rem(.8)).TextColor(t.TextMuted)
	})
}

func (a *App) depthField(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).FillWidth().Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space(0.5)).Children(func() {
			components.Text(c, "影响分析深度").FontSize(t.Rem(.75)).TextColor(t.TextMuted)
			components.Text(c, strconv.Itoa(a.settings.ImpactDepth)).
				FontWeight(600).FontSize(t.Rem(1.05)).TextColor(t.Text)
			components.Text(c, "以选中的元素为中心，向上向下各走几跳（默认 "+
				strconv.Itoa(config.DefaultImpactDepth)+"）。").
				FontSize(t.Rem(.8)).TextColor(t.TextMuted)
		})
		less := sidebar.IconButton(c, lucide.Minus(), "减小影响分析深度")
		less.Disabled(a.settings.ImpactDepth <= minImpactDepth)
		if less.Clicked() {
			a.settings.ImpactDepth--
			a.settingsSaved = false
		}
		more := sidebar.IconButton(c, lucide.Plus(), "加深影响分析")
		more.Disabled(a.settings.ImpactDepth >= maxImpactDepth)
		if more.Clicked() {
			a.settings.ImpactDepth++
			a.settingsSaved = false
		}
	})
}

func (a *App) saveRow(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).FillWidth().Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
		save := components.Button(c, "保存设置", components.ButtonPrimary)
		save.Disabled(a.db == nil)
		if save.Clicked() {
			a.settingsErr = a.SaveSettings(context.Background(), a.settings)
			a.settingsSaved = a.settingsErr == nil
		}
		switch {
		case a.settingsErr != nil:
			components.Icon(c, lucide.CircleAlert).Size(14, 14)
			components.Text(c, a.settingsErr.Error()).
				FontSize(t.Rem(.85)).TextColor(t.Danger)
		case a.settingsSaved:
			components.Icon(c, lucide.Check).Size(14, 14)
			components.Text(c, "已保存").FontSize(t.Rem(.85)).TextColor(t.TextMuted)
		}
	})
}
```

`save.Disabled(a.db == nil)` 只护测试里那条 `NewApp(nil)` 路径；真窗口永远有库。写它是为了让 `TestSettingsPageRendersSaveError` 能不开库跑 —— 但注意它**不**护 `SaveSettings` 里的 `d.ConfigSet`：那条路径在 `db == nil` 时是 nil 指针方法调用，会 panic。所以按钮关掉是必要的，不是装饰。

- [ ] **Step 4: 给 App 补两个字段**

`internal/ui/state.go` 的 `App` 里，紧跟 `settings config.Settings` 之后：

```go
	// 设置页的保存回执。两个字段都只在主线程写（纪律 1），且 settingsErr
	// 非空时 settingsSaved 必为 false —— saveRow 里同一次赋值保证这一点。
	settingsSaved bool
	settingsErr   error
```

import 块加 `"github.com/futuremeng/OpenArchitect/internal/config"`（若 Task 16 已加过就跳过）。

- [ ] **Step 5: 跑测试确认通过**

Run: `export PATH="/usr/local/go/bin:$PATH" && CGO_ENABLED=1 go test ./internal/ui/ -race -v`
Expected: 全部 PASS。`-race` 是这一条必须的：`TestRepoScanButtonRunsToReady` 是计划里第一次让真 goroutine 参与点击测试。

- [ ] **Step 6: 提交**

```sh
go vet ./... && go test ./...
git add internal/ui/page_repos.go internal/ui/page_settings.go internal/ui/state.go \
        internal/ui/page_repos_test.go internal/ui/page_settings_test.go
git commit -m "feat: 仓库页与设置页，扫描入口、目录与影响深度"
```


---

### Task 22: 外壳装配（侧栏导航、页面路由、生命周期、main 搬家）

**Files:**
- Create: `internal/ui/shell.go`
- Create: `internal/ui/shell_test.go`
- Modify: `internal/ui/app_test.go`（补 `CancelScan`/`finishScan` 的两条测试，见 Step 1 最后一块）
- Move: `main.go` → `cmd/openarchitect/main.go`（`git mv`，然后按 Step 4 重写）
- Modify: `mygo.json`（加顶层 `"main": "./cmd/openarchitect"`）
- Modify: `docs/superpowers/poc/2026-10-19-slice1-findings.md`（追加一段实测）

**Interfaces:**
- Consumes: Task 16 的 `App`（`page`/`SetPage`/`Page`、`status`、`settings`、`BeginScan`、`CancelScan`、`SetPost`、`SetOpenPath`、`StatusLine`、`finishScan`）、Task 19 的 `graphPage`、Task 20 的 `commandPalette`、Task 21 的 `reposPage`/`settingsPage`
- Produces:
  - `type navItem struct { label string; icon *ui.SVG; page Page }`、`func navItems() []navItem`
  - `func (a *App) NewShell() *sidebar.Shell`
  - `func (a *App) sidebarNav(c *ui.Context)`、`func (a *App) scanToolbar(c *ui.Context)`、`func (a *App) content(c *ui.Context)`
  - `main` 包的入口在 `cmd/openarchitect`，`mygo.json` 的 `main` 键指向它

**为什么导航行用 `components.SidebarItem`：** 它是 onevra 现成的导航按钮（`components/sidebar.go:34`：`SidebarItem(c, label, icon *ui.SVG, selected bool)`，文档写明 "The caller supplies selection and handles Clicked"），带选中面、Tab 焦点和 Enter/Space 激活。`shells/sidebar` 自己没有导航行组件（整包只导出 `NewShell`/`ShellOptions`/`Shell` 方法/`IconButton`/`HeaderHeight`/`SetToasts`）。本任务因此只用 `sidebar.IconButton` 和 `sidebar.HeaderHeight`，导航行走 `components`。

**`SidebarSection` 不调用**：它会给分组加一个 muted 标题（`sidebar.go:16`），而 spec §9.1 的侧栏只有"三页 + 底部扫描按钮 + 状态行"。三行下面不需要一个和窗口标题重复的表头。

- [ ] **Step 1: 写失败测试**

`internal/ui/shell_test.go`：

```go
package ui

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

func TestSidebarNavClickSelectsThatPage(t *testing.T) {
	for _, item := range navItems() {
		a := newTestApp(t)
		tt := ui.NewTester(func(c *ui.Context) { a.sidebarNav(c) }, 260, 600)
		tt.Frame()
		if err := tt.Click(item.label); err != nil {
			t.Fatalf("%s: %v", item.label, err)
		}
		if a.page != item.page {
			t.Fatalf("click %q → page %v, want %v", item.label, a.page, item.page)
		}
	}
}

func TestContentRoutesEachPage(t *testing.T) {
	cases := []struct {
		page Page
		want string
	}{
		{PageGraph, "还没有扫描过任何仓库"},
		{PageRepos, "还没有扫描过仓库"},
		{PageSettings, "影响分析深度"},
	}
	for _, tc := range cases {
		a := newTestApp(t)
		a.SetPage(tc.page)
		tt := ui.NewTester(func(c *ui.Context) { a.content(c) }, 1180, 780)
		tt.Frame()
		if !tt.HasText(tc.want) {
			t.Fatalf("page %v must show %q, texts = %v", tc.page, tc.want, tt.Texts())
		}
	}
}

// TestCommandShortcutOpensPanelThroughContent 证明的是"位置"：Task 20 的单测
// 直接跑 commandPalette，这一条跑 content —— ⌘K 的轮询只有在真的每帧被调用时
// 才有效，接错地方（比如放进 graphPage）就只在架构图页能用。
func TestCommandShortcutOpensPanelThroughContent(t *testing.T) {
	a := newTestApp(t)
	tt := ui.NewTester(func(c *ui.Context) { a.content(c) }, 1180, 780)
	tt.Frame()
	if a.commandOpen {
		t.Fatal("the panel starts closed")
	}
	tt.Key(ui.Cmd, ui.KeyK)
	if !a.commandOpen {
		t.Fatal("⌘K polled through content must open the panel")
	}
}

func TestScanToolbarScansConfiguredRoot(t *testing.T) {
	a := newTestApp(t)
	root, err := filepath.Abs(fixtureDir("simple-multi-main"))
	if err != nil {
		t.Fatal(err)
	}
	a.settings.ScanRoot = root
	done := make(chan func(), 1)
	a.SetPost(func(fn func()) { done <- fn })
	tt := ui.NewTester(func(c *ui.Context) { a.scanToolbar(c) }, 220, 60)
	tt.Frame()
	if err := tt.Click("扫描当前仓库"); err != nil {
		t.Fatal(err)
	}
	if a.status != StatusScanning {
		t.Fatalf("status right after the click = %v", a.status)
	}
	tt.Frame()
	if !tt.HasText("正在解析") {
		t.Fatalf("the toolbar must show the progress line, texts = %v", tt.Texts())
	}
	select {
	case fn := <-done:
		fn()
	case <-time.After(60 * time.Second):
		t.Fatal("the scan never reported back")
	}
	if a.status != StatusReady {
		t.Fatalf("status = %v err = %v", a.status, a.err)
	}
	if len(a.repos) != 1 || a.repos[0].ID != model.RepoIDFor(root) {
		t.Fatalf("repos after reload = %+v, want the scanned root %q", a.repos, model.RepoIDFor(root))
	}
}

func TestScanToolbarWithoutRootOpensSettings(t *testing.T) {
	a := newTestApp(t)
	tt := ui.NewTester(func(c *ui.Context) { a.scanToolbar(c) }, 220, 60)
	tt.Frame()
	if err := tt.Click("扫描当前仓库"); err != nil {
		t.Fatal(err)
	}
	// 空目录不是错误：BeginScan 会在 scanner.Detect 上失败并把状态打成 Failed，
	// 所以这里根本不能起扫描，只能把用户送到能填目录的那一页。
	if a.status != StatusIdle {
		t.Fatalf("status = %v err = %v", a.status, a.err)
	}
	if a.page != PageSettings {
		t.Fatalf("page = %v", a.page)
	}
}
```

追加到 `internal/ui/app_test.go` 末尾（`context`、`scanner` 已在该文件的 import 块里）：

```go
// TestFinishScanMapsCancellationToIdle 钉住 spec §10 第二行：取消不是失败。
// 走 goroutine 再取消会是掷硬币（fixture 太小，扫描可能先跑完），所以直接喂
// finishScan 那个哨兵——它才是"cancel → Idle"这条映射的实现。
func TestFinishScanMapsCancellationToIdle(t *testing.T) {
	a := newTestApp(t)
	a.status, a.progress = StatusScanning, "正在解析 x"
	_, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.finishScan(scanner.Report{}, context.Canceled)
	if a.status != StatusIdle || a.err != nil || a.progress != "" {
		t.Fatalf("status = %v err = %v progress = %q", a.status, a.err, a.progress)
	}
	if a.cancel != nil {
		t.Fatal("finishScan must drop the cancel func; a stale one would kill the next scan")
	}
}

// main.go 的 OnClosed 无条件调用 CancelScan，包括"开窗就关窗"那次——那时
// a.cancel 还是 nil。这条测试就是那个 nil。
func TestCancelScanIsSafeWithNothingInFlight(t *testing.T) {
	newTestApp(t).CancelScan()
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `export PATH="/usr/local/go/bin:$PATH" && go test ./internal/ui/ -run 'SidebarNav|ContentRoutes|CommandShortcutOpensPanelThroughContent|ScanToolbar|FinishScanMaps|CancelScanIsSafe'`
Expected: 编译失败，`undefined: navItems`、`undefined: (*App).sidebarNav`、`undefined: (*App).scanToolbar`、`undefined: (*App).content`。

- [ ] **Step 3: 实现 shell.go**

```go
package ui

import (
	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
	"github.com/oneo-me/onevra/source/shells/sidebar"
	"github.com/oneo-me/onevra/source/theme"
)

// navItem 是侧栏的一行。spec §9.1 固定了三页，所以这张表就是路由表。
type navItem struct {
	label string
	icon  *ui.SVG
	page  Page
}

func navItems() []navItem {
	return []navItem{
		{"架构图", lucide.Network(), PageGraph},
		{"仓库", lucide.Package(), PageRepos},
		{"设置", lucide.Settings(), PageSettings},
	}
}

// NewShell 建原生窗口。调用方必须紧接着装好两条线程通道，再进 App.Run：
//
//	s := a.NewShell()
//	a.SetPost(s.Window().Update)
//	a.SetOpenPath(mygo.Shell.OpenPath)
//
// SetPost 晚一步，扫描 goroutine 的结果就落在 Task 16 那个同步执行的默认
// post 上，等于在主线程之外写 App 字段（纪律 1）。
func (a *App) NewShell() *sidebar.Shell {
	return sidebar.NewShell(sidebar.ShellOptions{
		Title:     "OpenArchitect",
		StateKey:  "main",
		Width:     1180,
		Height:    780,
		MinWidth:  900,
		MinHeight: 600,
		LeftWidth: 220,
		Theme:     theme.Onevra,
		// 三页都走 Content 分支，不用 SetPage：后者的文档写着它会连侧栏控件
		// 一起换掉（shell.go:165-169），而 spec §9.1 要侧栏常驻。
		Left:        a.sidebarNav,
		LeftToolbar: a.scanToolbar,
		Content:     a.content,
	})
}

func (a *App) sidebarNav(c *ui.Context) {
	// Shell 不给任何内缩（shell.go:207-212），侧栏自己躲不开悬浮 header。
	ui.Column(c).FillWidth().Padding(sidebar.HeaderHeight(c), 0, 0, 0).Children(func() {
		side := components.Sidebar(c)
		side.Children(func() {
			for _, item := range navItems() {
				row := components.SidebarItem(c, item.label, item.icon, a.page == item.page)
				if row.Clicked() {
					a.SetPage(item.page)
				}
			}
		})
	})
}

// scanToolbar 建在 Shell 的 ui.Toolbar 里（shell.go:295-298），所以这里只放
// 工具栏的子元素，不自己包行。
func (a *App) scanToolbar(c *ui.Context) {
	t := c.Theme()
	components.Text(c, a.StatusLine()).
		FontSize(t.Rem(.75)).TextColor(t.TextMuted).
		Grow(1).Shrink(1).MinWidth(0).Ellipsis("…")
	scan := sidebar.IconButton(c, lucide.Radar(), "扫描当前仓库")
	scan.Disabled(a.status == StatusScanning)
	if scan.Clicked() {
		if a.settings.ScanRoot == "" {
			a.SetPage(PageSettings)
			return
		}
		_ = a.BeginScan(a.settings.ScanRoot)
	}
}

// content 是页路由器。每一页自己带上 sidebar.HeaderHeight(c) 的顶部内缩，
// 这里不再包一层，否则 header 下面会多出 48 DIP 空白。
func (a *App) content(c *ui.Context) {
	switch a.page {
	case PageRepos:
		a.reposPage(c)
	case PageSettings:
		a.settingsPage(c)
	default:
		// Page 只有三个常量，default 就是 PageGraph。写成 case PageGraph
		// 再加一个走不到的 default 分支是噪音。
		a.graphPage(c)
	}
	// 放在页面之后：overlay 元素挂在窗口级的 overlay 根上（mygo 的
	// coreOverlay → c.overlayRoot()），层级由调用顺序决定，页面内容先建、
	// 面板后建，面板才压在内容之上。
	a.commandPalette(c)
}
```

- [ ] **Step 4: 搬 main.go 并重写**

```sh
mkdir -p cmd/openarchitect
git mv main.go cmd/openarchitect/main.go
```

`mygo.json` 加顶层键 `"main": "./cmd/openarchitect"`（对应 `cmd/mygo/config.go:35` 的 `Main` 字段，它的 json tag 是 `main`，缺省值 `"."`；`dev`/`build` 都用 `go list -f {{.Dir}} <Main>` 找包目录，见 `embed.go:101-108`）。只加这一个键，别的字段不动——键不存在就照着现有缩进加在 `"productName"` 那一行后面。

`cmd/openarchitect/main.go` 全文替换成：

```go
package main

import (
	"context"
	"log"
	"path/filepath"

	"github.com/egoist/mygo"

	"github.com/futuremeng/OpenArchitect/internal/core/store"
	appui "github.com/futuremeng/OpenArchitect/internal/ui"
)

// main 包和 internal/ui 都叫 ui，所以自己的那个必须起别名（Task 16 那条
// "本包自己叫 ui 又 import mygo/ui 合法"的规则只在包内成立，导入方两个都要）。
func main() {
	// 名字决定 PathUserData = os.UserConfigDir() + App.Name()（mygo/paths.go），
	// 也就决定库目录，必须和 cmd/oac 的 defaultDBPath 对齐（spec §5.2 之后那段）。
	mygo.App.SetName("OpenArchitect")
	mygo.App.WhenReady(func() {
		// Path 要 app ready 之后才有意义，所以开库只能待在这个回调里。
		dir, err := mygo.App.Path(mygo.PathUserData)
		if err != nil {
			log.Fatalf("拿不到用户数据目录: %v", err)
		}
		db, err := store.Open(context.Background(), filepath.Join(dir, "openarchitect.db"))
		if err != nil {
			log.Fatalf("打开数据库 %s: %v", filepath.Join(dir, "openarchitect.db"), err)
		}
		a := appui.NewApp(db)
		if err := a.Reload(context.Background()); err != nil {
			// 空库、旧库都不该拦住窗口：页面自己有"还没扫描过"的引导态。
			log.Printf("首次载入: %v", err)
		}
		s := a.NewShell()
		a.SetPost(s.Window().Update)
		a.SetOpenPath(mygo.Shell.OpenPath)
		s.Window().OnClosed(func() {
			// spec §10：关窗即取消在跑的扫描，然后收库、退出。
			a.CancelScan()
			if err := db.Close(); err != nil {
				log.Printf("关闭数据库: %v", err)
			}
			mygo.App.Quit()
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
```

`Window.Update` 的签名就是 `func(fn func())`（`content.go:172`），所以 `s.Window().Update` 直接当 `SetPost` 的参数用；方法值在 `NewShell` 返回后取，`s.window` 那时已经赋值（`shell.go:93`）。`OnClosed` 返回一个 `off func()`（`window.go:1478`），这里不接。

Task 1 的 `_ = lucide.Box` 随旧 main.go 一起消失：图标依赖现在由 `internal/ui` 真的在用，不需要占位。

- [ ] **Step 5: 跑测试确认通过**

Run: `export PATH="/usr/local/go/bin:$PATH" && CGO_ENABLED=0 go build ./... && CGO_ENABLED=1 go test ./internal/ui/ -race -v`
Expected: 全部 PASS，`-race` 干净（`TestScanToolbarScansConfiguredRoot` 是第二次让真 goroutine 参与点击测试）。两个 `CGO_ENABLED` 都写出来，不靠继承 shell 环境：`go build` 那遍证明产品构建在 `=0` 下成立，`-race` 那遍用 `=1`（全局约束里的唯一例外）。

- [ ] **Step 6: 起窗口，肉眼验收**

```sh
go tool mygo doctor
go tool mygo dev
```

逐项核对，任何一项不对就停下来修，别往下做 Task 23：

1. `mygo dev` 起的是 `cmd/openarchitect`（改任意 `internal/ui/*.go` 会热重载；如果它盯着别的包报 "no main package"，就是 `mygo.json` 的 `main` 键写错了）；
2. 窗口默认 1180×780，侧栏 220 DIP、可折叠可调宽，重开窗口记得上次大小（`StateKey: "main"`）；
3. 三行导航可点，当前页有高亮面，切页时侧栏和底部工具栏不动；
4. 底部状态行从"尚未扫描任何仓库"变成"正在解析 xxx"再到元素计数；扫描中雷达按钮是灰的；
5. 设置页填一个非法目录（比如 `/tmp`）→ 保存 → 点雷达 → 停在设置页或架构图页且不崩，状态行给出错误；
6. ⌘K 开面板，能搜到元素、能跳设置页；Esc 关；
7. 架构图页点节点出右栏抽屉，抽屉里点邻居 chip 会跳过去；
8. 关窗口 → 终端里进程退出（`pgrep -f openarchitect` 无输出），库文件没有 `-wal`/`-journal` 残留。

把 1–8 的实测结果（含首次出帧耗时、窗口实际尺寸）追加到 `docs/superpowers/poc/2026-10-19-slice1-findings.md`：

```markdown
## Task 22 外壳装配实测
- 结论：
- 数字：
- 影响：
```

- [ ] **Step 7: 提交**

```sh
go vet ./... && go test ./...
git add internal/ui/shell.go internal/ui/shell_test.go internal/ui/app_test.go \
        cmd/openarchitect/main.go mygo.json \
        docs/superpowers/poc/2026-10-19-slice1-findings.md
git commit -m "feat: 侧栏外壳装配与窗口生命周期，main 搬进 cmd/openarchitect"
git status   # 期望干净；根目录不该再有 main.go
```

---

### Task 23: CI 依赖守卫（R1/R2 的机械化）

**Files:**
- Create: `.github/workflows/ci.yaml`
- Create: `tools/ciguard/ci_guard_test.go`（test-only 包，目录里没有非测试文件）
- Modify: `docs/superpowers/poc/2026-10-19-slice1-findings.md`（记全量耗时和 `timeout-minutes` 的推导）

**Interfaces:**
- Consumes: Task 14 的 `cmd/oac`、Task 16–22 的 `internal/ui` 依赖面、spec §5.1 的 R1/R2 与 §4.3 DoD-6
- Produces: `tools/ciguard` 包（`go test ./...` 的一部分），`.github/workflows/ci.yaml` 的五个门禁步骤

**这一步只创建文件，不 `git push`、不开 PR、不碰任何外部系统**（Global Constraints 的"观测为主"）。工作流文件一旦被推到 GitHub 就会自动跑，那是用户的决定；执行者把守卫测试做绿就停。

**为什么守卫要写成 Go 测试而不是只写在 YAML 里**：YAML 是"没人改就没人看"的文件。删掉一条 `grep`、把 `timeout-minutes` 改回 5、给 `go test` 加一个 `continue-on-error`，CI 自己不会红——红的是很多个提交之后一次卡死的作业或一个被静默吞掉的竞态。守卫测试进 `go test ./...`，所以这些改动在下一次本地跑测试时就红。断言一律用**下限**（`>= 15`）而不是相等值：以后合法地调高不会假报警，而调回旧值或删行一定会红。

- [ ] **Step 1: 写失败测试**

`tools/ciguard/ci_guard_test.go`：

```go
// Package ciguard 守住 .github/workflows/ci.yaml 里的依赖守卫（spec §5.1
// R1/R2、§4.3 DoD-6）。它读 YAML 原文而不是重新执行命令：命令本身也在断言范围
// 里，一条被删掉的 grep 不会自己报错。
package ciguard

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const workflowPath = "../../.github/workflows/ci.yaml"

// minJobTimeout 是 YAML 里 timeout-minutes 的下限，不是它的值。推导：Task 3 的
// PoC-3 门槛是单仓解析 ≤ 60 s，CI 跑的是全模块单测 + 三个 fixture 的扫描，
// Step 4 会量出本地总耗时；15 分钟 ≈ 那个数字的三倍余量。YAML 写 20。
const minJobTimeout = 15

func readWorkflow(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("read %s: %v（Task 23 的 YAML 还没建，或路径变了）", workflowPath, err)
	}
	return string(raw)
}

// normalize 吃掉 shell 的续行（反斜杠 + 换行）并把空白压成单空格，这样比对
// 只关心命令内容，不关心 YAML 里的缩进和折行写法。
func normalize(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, "\\\n", " ")), " ")
}

const uiStackPattern = `'github.com/egoist/mygo|github.com/egoist/mygo/ui|github.com/oneo-me/onevra[^ ]*'`

func TestWorkflowKeepsTheThreeDependencyGuards(t *testing.T) {
	got := normalize(readWorkflow(t))
	wants := []string{
		// 正向：core 与 scanner 的依赖图里不许出现 UI 栈。-x 是锚定匹配——
		// 前缀匹配会把 plugins/sqlite 一起误判成违规（spec §5.1 特意点过）。
		`go list -deps ./internal/core/... ./internal/scanner/... | grep -x -E ` + uiStackPattern + ` && exit 1 || true`,
		// 反向：驱动是例外，但例外只在 store 那一层。
		`go list -deps ./internal/scanner/... ./internal/core/model/... ./internal/core/graph/... | grep -x 'github.com/egoist/mygo/plugins/sqlite' && exit 1 || true`,
		// DoD-4：oac 那份数据不是 GUI 顺手喂给它的。
		`go list -deps ./cmd/oac | grep -x -E ` + uiStackPattern + ` && exit 1 || true`,
	}
	for _, want := range wants {
		if !strings.Contains(got, normalize(want)) {
			t.Errorf("ci.yaml 丢了这条守卫：\nwant %s\ngot  %s", want, got)
		}
	}
}

func TestWorkflowRunsVetAndTest(t *testing.T) {
	got := normalize(readWorkflow(t))
	for _, want := range []string{"go vet ./...", "go test ./..."} {
		if !strings.Contains(got, want) {
			t.Errorf("ci.yaml 必须跑 %q", want)
		}
	}
}

// 三条守卫里的 `|| true` 是合法的：它吞的是 grep "没匹配到" 的退出码 1，不
// 是断言失败。vet/test 这两条 run 不一样——本仓库没有已知失败集，给它们加
// continue-on-error 或 || true 等于把后来的真红灯降级成日志。
func TestWorkflowHasNoToleratedGates(t *testing.T) {
	raw := readWorkflow(t)
	if strings.Contains(raw, "continue-on-error") {
		t.Fatal("ci.yaml 没有已知失败可吞，continue-on-error 只会掩盖真故障")
	}
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		gated := strings.HasPrefix(trimmed, "run: go vet") || strings.HasPrefix(trimmed, "run: go test")
		if gated && (strings.Contains(trimmed, "|| true") || strings.Contains(trimmed, "; true")) {
			t.Fatalf("门禁步骤被容忍了：%q", trimmed)
		}
	}
}

func TestWorkflowTimeoutKeepsItsFloor(t *testing.T) {
	m := regexp.MustCompile(`(?m)^\s*timeout-minutes:\s*(\d+)\s*$`).FindStringSubmatch(readWorkflow(t))
	if m == nil {
		t.Fatalf("ci.yaml 没有 job 级 timeout-minutes：卡死的作业会一直占着 runner")
	}
	got, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatal(err)
	}
	if got < minJobTimeout {
		t.Errorf("timeout-minutes = %d，下限 %d", got, minJobTimeout)
	}
}

func TestWorkflowPinsCgoAndGoVersion(t *testing.T) {
	raw := readWorkflow(t)
	// 两个值逐字来自 Global Constraints：CGO_ENABLED=0 与 go1.27.2（onevra 的下限
	// 是 1.27.1）。CI 里放开 CGO 等于让"没有 C 工具链也能构建"这条前提失真。
	for _, want := range []string{`CGO_ENABLED: "0"`, `go-version: "1.27.2"`} {
		if !strings.Contains(raw, want) {
			t.Errorf("ci.yaml 丢了 %q", want)
		}
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `export PATH="/usr/local/go/bin:$PATH" && go test ./tools/ciguard/`
Expected: 六条全部 FAIL，第一行是 `read ../../.github/workflows/ci.yaml: no such file or directory`。

- [ ] **Step 3: 写 ci.yaml**

`.github/workflows/ci.yaml`：

```yaml
name: ci

on:
  push:
    branches: [main]
  pull_request:

jobs:
  check:
    # runner 必须是 macOS：存储驱动走 mygo/plugins/sqlite 随包附带的预编译库，
    # PoC-2 只在 darwin/arm64 上验过（Task 2），换 ubuntu 会红在 store 那批测试上，
    # 而且和交付平台不一致。UI 测试用 mygo 的内存渲染器（ui.NewTester），不需要显示器。
    #
    # 没有 -race 步骤：竞态检测器要求 cgo，和本 job 的 CGO_ENABLED=0 冲突。
    # 这不是放弃 race 覆盖——Task 21/22/24 的收尾命令都带 -race，那一遍显式写
    # CGO_ENABLED=1（全局约束里的唯一例外），是本地必跑的一遍。
    runs-on: macos-latest
    timeout-minutes: 20
    env:
      CGO_ENABLED: "0"
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.27.2"
      - name: vet
        run: go vet ./...
      - name: test
        run: go test ./...
      - name: R1 core 与 scanner 不依赖 UI 栈
        run: |
          go list -deps ./internal/core/... ./internal/scanner/... \
            | grep -x -E 'github.com/egoist/mygo|github.com/egoist/mygo/ui|github.com/oneo-me/onevra[^ ]*' \
            && exit 1 || true
      - name: R1 反向 驱动只准出现在 store
        run: |
          go list -deps ./internal/scanner/... ./internal/core/model/... ./internal/core/graph/... \
            | grep -x 'github.com/egoist/mygo/plugins/sqlite' && exit 1 || true
      - name: DoD-4 oac 不含 UI 依赖
        run: |
          go list -deps ./cmd/oac \
            | grep -x -E 'github.com/egoist/mygo|github.com/egoist/mygo/ui|github.com/oneo-me/onevra[^ ]*' \
            && exit 1 || true
```

`env.CGO_ENABLED` 在 job 级，所以四个 `run` 步骤都吃到；`actions/setup-go@v5` 不设 `CGO_ENABLED`，不会覆盖它。

- [ ] **Step 4: 本地把七步都跑一遍**

CI 的每一步都要能在本机原样跑通，否则把它写进 YAML 就是空头承诺。

```sh
export PATH="/usr/local/go/bin:$PATH"
go vet ./... && go test ./...
CGO_ENABLED=0 go list -deps ./internal/core/... ./internal/scanner/... \
  | grep -x -E 'github.com/egoist/mygo|github.com/egoist/mygo/ui|github.com/oneo-me/onevra[^ ]*' \
  && echo "R1 VIOLATION" || echo "R1 ok"
CGO_ENABLED=0 go list -deps ./internal/scanner/... ./internal/core/model/... ./internal/core/graph/... \
  | grep -x 'github.com/egoist/mygo/plugins/sqlite' && echo "driver leaked" || echo "driver confined"
CGO_ENABLED=0 go list -deps ./cmd/oac \
  | grep -x -E 'github.com/egoist/mygo|github.com/egoist/mygo/ui|github.com/oneo-me/onevra[^ ]*' \
  && echo "oac has UI deps" || echo "oac headless"
```

Expected: `R1 ok`、`driver confined`、`oac headless`，且 `go test ./...` 里 `tools/ciguard` 全绿。

三条 `go list` 输出错了就是真违规，按 spec 的分支处理：UI 栈进了 core → 把那个 import 挪回 `internal/ui`；驱动出现在 `core/model`/`core/graph`/`scanner` → 说明有人在那三层里直接开库，改成收 `*store.DB` 参数。

- [ ] **Step 5: 量一遍全量耗时，复核 timeout-minutes**

```sh
export PATH="/usr/local/go/bin:$PATH"
export GOPROXY=https://goproxy.cn,direct
/usr/bin/time -p go test ./... > /tmp/ci-timing.log 2>&1; tail -3 /tmp/ci-timing.log
```

把 `real` 秒数写进 findings 的 `## CI 门禁` 段，格式：`结论/数字/影响`。`数字` 里必须给出：全量耗时、`timeout-minutes: 20` 的下限 15、以及"耗时 × 多少倍才是 20"。若实测已经超过 `15 × 60` 秒，**停下来报告**——那说明 `minJobTimeout` 的推导不成立，改常量要连同推导一起改，不是就地抬高 YAML 里的数字。

findings 追加：

```markdown
## CI 门禁（Task 23）
- 结论：
- 数字：全量 go test 耗时 = ；macOS runner 冷启动另计；
- 影响：
```

- [ ] **Step 6: 提交**

```sh
go vet ./... && go test ./...
git add .github/workflows/ci.yaml tools/ciguard/ci_guard_test.go \
        docs/superpowers/poc/2026-10-19-slice1-findings.md
git commit -m "ci: 加 R1/R2 依赖守卫工作流，并用 go 测试守住这份 YAML"
```

---

### Task 24: DoD 验收（spec §4.3 六条）+ PoC-6 / PoC-7 + PoC-2 剩下的一半

**Files:**
- Modify: `docs/superpowers/poc/2026-10-19-slice1-findings.md`（DoD 六条 + PoC-6/7 的实测表）
- Create: `README.md`（只在 PoC-7 的 Gatekeeper 分支触发；不触发就不建）

**Interfaces:**
- Consumes: 前面 23 个任务的全部产物
- Produces: 一份"每条 DoD 都有数字或截图说明"的验收记录，以及切片 1 是否算完成的判断

**这一步不写新功能代码。** 验收中发现的缺陷回到对应任务修，修完重跑对应 Step，不要在 Task 24 里就地补一个"顺手"的抽象。任何一条 DoD 不达标就停下来报告，给出实测数字和你判断的根因——是否降级、是否改范围，是用户的决定（和 PoC 门槛同一条规则）。

- [ ] **Step 1: DoD-5 / DoD-6——机器门禁先全绿**

```sh
export PATH="/usr/local/go/bin:$PATH"
CGO_ENABLED=0 go vet ./... && CGO_ENABLED=0 go test ./... && CGO_ENABLED=0 go test ./tools/ciguard/ -v
CGO_ENABLED=1 go test -race ./internal/ui/ ./internal/core/store/
```

Expected: 全 PASS；`tools/ciguard` 六条断言逐条列出；`-race` 干净（`TestScanToolbarScansConfiguredRoot` 与 `TestRepoScanButtonRunsToReady` 是仅有的两处真 goroutine 点击测试）。前两行用 `CGO_ENABLED=0` 是为了和 CI 跑的是同一棵树——Task 23 的守卫只保证 YAML 里写着 0，本地这一遍才证明代码在 0 下面真的过。

DoD-6 的"由 CI 检查强制"就是那六条断言 + `.github/workflows/ci.yaml` 里三条 `go list` 守卫。Task 23 已经把它们钉住，这一条只核"CI 里跑的命令在本地也跑绿"。

- [ ] **Step 2: DoD-1——60 秒内出图，两种冷启动都量**

用 PoC-3 同一个中等仓库（`$GOMODCACHE/github.com/oneo-me/onevra/source@v0.2026.12`），保证两次验收的可比性。

spec §4.3 的"起窗口后 60 秒内出图"有两种读法，都量，用数字选：

| 场景 | 量什么 | 门槛 |
|---|---|---|
| (a) 库里已有快照，`go tool mygo dev` 冷启动 | 窗口出现 → 首帧画出节点 | ≤ 2 s（这条路只走 `Reload`：读库 + 建图 + 布局，没有解析） |
| (b) 空库，从点下扫描按钮起 | 按钮 → 首帧画出节点 | ≤ 60 s（PoC-3 的解析门槛 + 写库 + Reload + `flow` 布局） |

```sh
go tool mygo dev
```

场景 (a) 先做（库里留着 Step 5 或 Task 22 验收时那次扫描的结果）。场景 (b) 要在空库上做：

```sh
db="$HOME/Library/Application Support/OpenArchitect/openarchitect.db"
cp "$db" "$db.bak"            # 验收会往真库里写，先备份
rm -f "$db" "$db"-wal "$db"-journal
```

窗口里：设置页填扫描目录（粘那一行 `$GOMODCACHE/...` 的全路径）→ 保存 → 侧栏雷达按钮 → 记时间。

Expected: 两个数字写进 findings。场景 (b) 超 60 s 就停下报告——DoD-1 的门槛是 spec 定的，不是本地实测出来的。

验收完把库恢复回去：`cp "$db.bak" "$db"`，删掉 `.bak`。

- [ ] **Step 3: DoD-2——抽屉五件套 + Reveal**

在架构图页单击一个 struct 节点（挑一个已知有上下游的，比如 onevra 里的 `components.Dialog`），右侧抽屉必须给出：

1. 名称与类型徽章；
2. 所属包（完整导入路径）；
3. 文件路径；
4. 导出方法列表（超过 3 个时折叠成"另有 N 个"，点表头展开）；
5. 直接上游、直接下游两组 chip。

再点一个上游 chip：图必须把镜头移到那个节点（`flow.State.Reveal`），且抽屉换成它的详情。方法折叠/展开、点邻居跳转，两条都有单测（`TestDetailMethodsHeaderClickExpands`、`TestNeighbourChipJumpsToThatElement`），这一步只验单测证明不了的那半：`Reveal` 之后目标节点确实落在视口里。

Expected: 五个字段都有真实值（不是 `-` 也不是空），文件路径点一下能在 Finder 里选中该文件（`mygo.Shell.OpenPath`）。

- [ ] **Step 4: DoD-3 + PoC-6——影响范围淡出够不够表达**

打开"影响范围"开关，选中一个热点类型：中心 + 2 跳内的节点保持正常，其余淡出（`flow.Node.Muted`）。

逐条判断，把答案写进 findings 的 `## PoC-6`：

- 淡出后还能不能一眼看出"这次改动会打到哪些包"？
- 淡出的边是否跟着淡出（只淡节点会留下一堆穿过灰区的亮线）？
- 2 跳的默认深度对这个仓库是太浅还是太深（用设置页的加减钮改一次再看）？

**够表达** → 记录结论，继续 Step 5。**不够表达** → 走 spec §12 PoC-6 的退化分支（影响分析只在 CLI/表格里看，图保持纯浏览）。那条分支要删的是已经做完的 UI，属于范围变更，**停下报告**，等用户拍。

- [ ] **Step 5: DoD-4——CLI 与 GUI 同源**

机器可验证的那一半是 Task 14 的 `cmd/oac/parity_test.go`：`runImpact` 的写出集合与 `graph.Analyze(...).AffectedIDs()` 逐行相等。先确认它是绿的：

```sh
go test ./cmd/oac/ -run Parity -v
```

GUI 这一半人工核对一次：同一个元素、同一个深度，`oac impact` 的上下游计数必须等于抽屉"影响范围"那行的两个数字。

```sh
go build -o /tmp/oac ./cmd/oac
# 不带 --db：默认路径必须就是 GUI 那个库（spec §5.2 之后那段 + Task 1 的 productName）
/tmp/oac impact cmp:github.com/oneo-me/onevra/source/components.Dialog --depth 2
```

Expected: 输出的 `Upstream (N)` / `Downstream (M)` 两个数，和 GUI 抽屉里"上游 N 个，下游 M 个，深度 2 跳"完全一致。ID 拼错了就先用 `oac graph <repo-id>` 拿真实 ID 列表。

不一致就是真 bug（两边读的是同一份 core），停下报告，别用"四舍五入/深度默认值不同"解释过去——`--depth 2` 和 `config.DefaultImpactDepth = 2` 是钉死的。

- [ ] **Step 6: PoC-7 + PoC-2 剩下的一半——打包后的 .app 能不能跑**

Task 2 只在 `go tool mygo dev` / `go run .` 下验过驱动，spec §12 PoC-2 的原文是"`go tool mygo build` 后 `sqlite.LibraryPath()` 在 **.app 内**能找到预编译库"。这一补上，顺带做 PoC-7。

```sh
go tool mygo build          # 输出里给出 .app 的落点，下面用 $APP 代指那个路径
open "$APP"                 # 从 Finder/LaunchServices 起，不是从终端起
```

三个问题依次回答，都写进 findings：

1. **PoC-2（打包态）**：窗口能起、能扫描、能出图 → 预编译 sqlite 库随 bundle 走通了。如果报"加载库失败"或 `LibraryPath` 指到 bundle 外，这就是 PoC-2 的失败分支：**停下报告**，换 driver 的代价（R1 例外条款作废）由用户定。
2. **PoC-7（Gatekeeper）**：`open` 之后是否直接被拦（"已损坏"或"无法验证开发者"）。拦了就试 `xattr -dr com.apple.quarantine "$APP"` 再开一次，记下这条命令是否解决。
3. **数据一致性**：打包版和 dev 版用的是同一个库目录吗？`ls -l "$HOME/Library/Application Support/OpenArchitect/"` 应该只有一份 `openarchitect.db`——两份就说明 `mygo.json` 的 `productName` 和 `App.SetName` 没对上（Task 1 与 Task 22 的那两处注释）。

只有第 2 条真的被拦时，才创建 `README.md`，加一节，把实测过的命令原样写进去（spec §12 PoC-7 授权的分支）：

```markdown
## 从源码构建与首次运行

前置：Go 1.27.1 以上（`/usr/local/go/bin/go version` 确认），`export PATH="/usr/local/go/bin:$PATH"`。

    go tool mygo doctor
    go tool mygo dev      # 开发，热重载
    go tool mygo build    # 产出未签名的 .app

未签名包在本地首次打开会被 Gatekeeper 拦下。去掉隔离属性后可以直接运行：

    xattr -dr com.apple.quarantine "<go tool mygo build 输出的 .app 路径>"

签名与公证属于后续切片（spec §15 第 6 项）。
```

- [ ] **Step 7: 写验收结论并提交**

findings 末尾加一张表，每行只允许三种值：`PASS` / `FAIL` / `未验（原因）`，并各带一个证据（命令、数字、或哪条测试）：

```markdown
## 切片 1 验收（DoD）

| DoD | 结果 | 证据 |
|---|---|---|
| 1 出图 ≤60 s |  | 场景 a/b 各 N s |
| 2 抽屉五件套 + Reveal |  |  |
| 3 影响范围淡出 |  | 含 PoC-6 三条判断 |
| 4 CLI 与 GUI 同源 |  | parity 测试 + 一次 N/M 计数比对 |
| 5 vet + test |  |  |
| 6 core 不 import UI（CI 强制） |  | ciguard 六条 + 三条 go list |
| PoC-2 打包态 |  |  |
| PoC-7 Gatekeeper |  |  |
```

表里的"结果"列有 `FAIL` 就先停下报告，别提交成"切片完成"。全 `PASS` 才提交：

```sh
git add docs/superpowers/poc/2026-10-19-slice1-findings.md
# 只在走了 Step 6 第 2 条分支时才加 README.md
git commit -m "docs: 切片 1 的 DoD 与 PoC-6/7 实测验收记录"
```
