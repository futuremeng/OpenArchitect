# OpenArchitect 切片 1 设计：Go 单仓扫描 → SQLite → flow 架构图

- 状态：已与用户逐段确认（A–C 段、D–H 段两轮通过）
- 日期：2026-10-19
- 来源需求：附件文档《OpenArchitect 项目方案》（6 章，含技术栈、架构模型、模块设计、数据库设计、UI 设计）
- 本文件只覆盖**第一个可运行闭环**；其余子系统各需自己的 spec → plan → 实现周期（见 §15）

---

## 1. 项目定位（摘自源文档，保留原文约束）

OpenArchitect 是面向**一人公司（OPC）**的开源桌面工具：自动扫描多个关联项目的代码仓库，维护跨项目的统一架构视图；从 Git/PR/Issue 提取特性并挂载到架构元素上追踪进度；沿关系图做影响分析，回答"改这里会坏哪里"。

六条核心原则在本切片全部成立，其中三条直接变成设计约束：

| 原则 | 本切片的落地约束 |
|---|---|
| 观测为主，不执行 | 不创建 Issue、不触发 CI、不调度 Agent。代码里不出现任何写外部系统的调用 |
| 本地优先 | 数据只在本地 SQLite；无服务端；离线可用 |
| 单用户优先 | 无用户体系、无租户、无权限。`repos` 表不做 owner 语义 |
| 多语言覆盖 | 本切片只实现 Go adapter；`LanguageAdapter` 接口按源文档 4.2 保留 |
| 松耦合协作 | Webhook 推迟到后续切片 |
| 开源 | MIT（仓库已有 LICENSE）；无厂商锁定 |

目标用户：一人同时维护 3~10 个有上下游关系的 Go 项目，没有专职架构师，痛点是脑中的"系统全景图"无法外化。

非目标（源文档 1.5，仍然有效）：不是协作文档平台、不是任务执行引擎、不是企业架构治理工具、不是通用代码搜索。

---

## 2. 依赖核实结论

以下是设计的前提，全部于 2026-10-19 用命令实测，不是来自 README 转述。核实方式：`gh api` 读仓库元数据、下载模块后读 `$GOMODCACHE` 里的实际 `.go` 源文件、`go doc -all`。

### 2.1 egoist/mygo v0.5.5

| 事实 | 证据 |
|---|---|
| 真实存在，MIT，2283 stars，2026-09-26 创建，v0.5.5 发布于 2026-10-16 | `gh api repos/egoist/mygo` |
| 双模式：纯 Go 原生 UI（GPU 自绘，无 HTML/JS/WebView）或系统 WebView（macOS WKWebView / Linux WebKitGTK / Windows WebView2） | README、`docs/README.md` |
| **不使用 cgo**，任意机器可交叉编译全平台；macOS ≥ 12 的原生 UI **不需要任何系统依赖**（用 Metal 绘制） | `docs/README.md` Requirements 段 |
| 开发要求：Go ≥ 1.27；原生 UI 项目**只需要 Go**（Web 前端才需要 Bun） | 同上 |
| 官方脚手架：`go run github.com/egoist/mygo/cmd/mygo@latest init -template native my-app`，然后 `go tool mygo dev` / `go tool mygo build` | README "Getting started" |
| 官方 SQLite 插件（无 cgo 的真 SQLite）；官方 GitHub Actions 构建签名公证模板；`mygo doctor` 检查开发机 | `docs/README.md` Official plugins / github-actions |

关键 API（`go doc -all github.com/egoist/mygo`）：

- `func RunOnMain(fn func())`
- `func (w *Window) Update(fn func())` —— **异步结果回主线程的唯一正解**：`docs/ui/views.md` 原文 "Change it from other goroutines with `Window.Update`, which runs a function on the main thread and then draws a new frame"；`Window.Invalidate()` 只重绘
- `func (c *Context) Invalidate()`（视图内请求下一帧）、`c.After(d)`
- `mygo.App.Path(mygo.PathUserData) (string, error)` —— `~/Library/Application Support/<AppName>`，本库文件位置
- `mygo.App.WhenReady`、`Theme.SetSource`、`NewWindow(WindowOptions)`
- 原生 UI 模型：视图是 app state 的纯函数，主线程每帧调用；state 就是自己的 struct，widget 收 `*T`（如 `EditableText(c, value *string)`）；`ui.Local[T](e, key, init)` 存元素局部状态。`views.md` 明确要求 "Model data and widget state such as `ListState` belong in your app"

性能基线（`docs/ui/performance.md`，M5 / macOS arm64 / Go 1.27.1 / `CGO_ENABLED=0 GOMAXPROCS=1`，2026-10-08 实测）：

| 场景 | 每帧耗时 |
|---|---|
| 1,000 个静态 label | 397 µs，0 分配 |
| 百万行列表的可见行 | 14.5 µs |
| 1,000 label 的 retained 堆 | ≈ 2.50 MB |

这组数字是本设计"节点卡片必须极薄"约束的来源（§9）。

### 2.2 oneo-me/onevra `source@v0.2026.12`

| 事实 | 证据 |
|---|---|
| 真实存在，MIT，69 stars，2026-10-06 创建，基于 MyGo；`source/v0.2026.12` 于 2026-10-18 发布 | `gh api repos/oneo-me/onevra` |
| 模块路径 `github.com/oneo-me/onevra/source`，要求 go 1.27.1，依赖 `ebitengine/purego`（无 cgo）与 `egoist/mygo v0.5.5` | 包内 `go.mod` |
| 源文档 2.2 承诺的复杂组件**全部存在**：`chart` `graph` `tree` `flow` `gesture` `command` `chat`，另有 `treemap` | `$GOMODCACHE/.../components/` 目录实测 |
| 窗口外壳两种：`shells/plain`、`shells/sidebar`（可折叠可调宽侧栏 + 内容区 + header actions + 可替换页面） | README "Shells" |
| 安装：`go get github.com/oneo-me/onevra/source@v0.2026.12`；示例可跑：`cd source && go run ./examples/gallery`、`./examples/desktop`、`./examples/launcher` | README |
| `Sidebar` 内容需避开浮动 header 时用 `sidebar.HeaderHeight(c)` | README |

`flow` 组件公开 API（`components/flow/flow.go`，切片 1 的**主渲染组件**）：

```go
type Direction uint8          // TopToBottom | LeftToRight
type Node struct {
    ID, Label string
    Width, Height float32
    Muted bool                              // "fades the node and its edges"
    Content func(c *ui.Context)             // 卡片由应用自绘
}
type Edge struct {
    Source, Target string
    Label string                            // 边标签 → 关系类型
    Dashed bool                             // 虚线 → 区分 depends_on / calls
    Color ui.Color
    Width float32
}
type Data struct { Nodes []Node; Edges []Edge }
type Options struct {
    Label, EmptyText string
    Height float32
    Direction Direction
    NodeGap, RankGap float32
    Selected string                          // 应用持有的选中节点
    Select, Open func(id string)             // 单击 / 双击；点空白处 Select("")
    Empty, Overlay func(*ui.Context)         // Overlay 放缩放控件
}
func View(c *ui.Context, state *State, data Data, opts Options) ui.Element
func (s *State) Ready() bool
func (s *State) Zoom() float32
func (s *State) Fit() / ZoomIn() / ZoomOut() / Reveal(id string)
func (s *State) Position(id string) (x, y float32, ok bool)
```

包注释原文："draws a **directed diagram**, such as a workflow, **a dependency graph** or a plan of tasks, and lays it out automatically: nodes fall into ranks along the edges' direction, ranks are ordered to avoid crossings and edges run between them as orthogonal lines."

`graph` 组件公开 API（切片 1 **不用**，理由见 §3 偏差 1）：`Node{ID,Label,Group,Weight,Size,Color,LabelColor,Openable}`、`Edge{Source,Target,Style{Distance,Strength,Width,Opacity,HoverOpacity,Color}}`、`Options{Label,EmptyText,Height,Palette,GroupColor,Forces,HoverDepth,Open,Empty,Overlay}`、`State{Hovered,Settled,Zoom,Fit}`、`View(...)`。布局是力导向（Barnes-Hut 四叉树，`barnesHutTheta2 = 0.9²`，256 并行分块）。在 `graph.go`/`layout.go`/`render.go` 三个文件里 grep `arrow|Arrow|dashed|Dashed|Select|OnClick` → **零命中**。

### 2.3 mygo 官方 SQLite 插件（决定 §7 的写法）

`github.com/egoist/mygo/plugins/sqlite`。包注释原文："SQLite's C amalgamation and a small C shim are compiled with Zig and loaded through purego: no cgo, Zig, Bun or compiler is needed at application run time."，以及 "Go and native UI apps call `Open` directly and close their own connections; no `mygo.Use` is needed."

**它不是 `database/sql`。** 从 `plugins/sqlite/database.go` 实测，`*DB` 只有四个方法：

```go
func Open(ctx context.Context, path string, opts OpenOptions) (*DB, error)
func (d *DB) Execute(ctx context.Context, sql string, args ...any) (Result, error)
func (d *DB) Query(ctx context.Context, sql string, args ...any) (Rows, error)
func (d *DB) Transaction(ctx context.Context, statements []Statement) ([]Result, error)
func (d *DB) Close() error

type OpenOptions struct { ReadOnly bool; BusyTimeout time.Duration } // 零值等锁 5s，负值不等待
type Statement  struct { SQL string; Args []any }
type Result     struct { Changes int64; LastInsertID int64 }
type Rows       struct { Columns []string; Values [][]any }          // 全量物化，无游标
```

三个后果：迁移签名改 `Migrate(ctx, *DB)`；不做 `database/sql` 兼容壳；`Transaction` 接收**静态语句批**，所以写库路径必须能预先算完全部语句（扫描落库正好满足）。

### 2.4 环境实测

| 项 | 状态 |
|---|---|
| Go | `/usr/local/go/bin/go` → `go1.27.2 darwin/arm64` ✓ 满足 ≥1.27 |
| Go 在 PATH | **否**：`~/.zshrc` 与 `~/.zprofile` 均无 `/usr/local/go/bin`，需用户自行追加（本设计不修改用户 shell 配置） |
| `proxy.golang.org` | **不可达**（`dial tcp ... i/o timeout`） |
| `GOPROXY=https://goproxy.cn,direct` | **可用**：`onevra/source@v0.2026.12` 与 `mygo@v0.5.5` 均已成功下载 |

---

## 3. 与源文档的偏差（决策记录）

每条都是对源文档既定写法的**主动修改**，需要你确认；有异议的改回去只影响本节引用的段落。

| # | 源文档写法 | 本设计 | 理由 |
|---|---|---|---|
| 1 | 2.2/6.2："架构图可视化优先使用 `graph` 原生组件" | 主视图用 **`flow`**；`graph` 本切片不用 | `graph` 无边方向（无箭头）、无边标签、无单击选中回调（只有双击 `Open`），而 3.1 的 `calls/depends_on/implements` 全是有向语义、6.1 又要"点击节点→右侧滑出详情面板"。`flow` 包注释直接点名 dependency graph，且 `Select`/`Selected`/`Edge.Label`/`Dashed`/`Muted`/`Reveal` 齐备 |
| 2 | 3.1：五种关系 `calls/depends_on/implements/contains/uses` | 只产 **`contains` / `depends_on` / `calls`** | `implements` 需遍历 method set 做接口匹配，静态可靠判定基本只剩 `var _ Iface = (*T)(nil)` 惯用断言；`uses`（识别 DB/MQ/第三方 API）是启发式活。两者都不是"改这里会坏哪里"的必要条件，进 MVP 只拖住切片 |
| 3 | 5.3：影响分析用 SQLite 递归 CTE | **内存 BFS**（`internal/core/graph`） | 节点量在几百级；两套图语义（SQL 递归 + 内存遍历）必然漂移。真 SQLite 支持递归 CTE，多仓合并后需要下推时再启用，接口不变 |
| 4 | 5.2：`func Migrate(db *sql.DB)` | `func Migrate(ctx context.Context, db *sqlite.DB)` | §2.3：插件不是 `database/sql` |
| 5 | 5.1：`architecture_snapshots` 同时存 `elements_json`/`relationships_json` **又**建结构化表 | 删两个 JSON 列，结构化表是唯一真相 | 双写同一数据只会制造不一致，且没有消费方 |
| 6 | 2.4：Java 用 `JavaParser`（"独立进程或 CGO"）、Rust 用 `syn`+`proc-macro2` | 后续语言 adapter 一律 **sidecar 子进程**，禁止 cgo | `syn`/`proc-macro2` 是 Rust crate，Go 无法链接；"CGO"那条写法不成立。mygo 的核心优势是 no-cgo 交叉编译全平台，不能被一个 tree-sitter 绑定破掉。`LanguageAdapter` 接口（源文档 4.2）本身是对的，保留 |
| 7 | 5.1：7 张表 | 本切片只建 **5 张** | `features`/`commits` 在切片 3 才有写入方；预建空表就是死表 |
| 8 | 3.2 `Element`：9 个字段 | 留 **6 个**：`ID/Type/Name/Description/RepoID/FilePath` | `Owner`、`TechStack`、`Metadata` 三个在本切片**没有任何数据来源** —— 没有用户体系（Owner 恒为一人）、没有配置解析（TechStack）、没有写入方（Metadata）。按 YAGNI 移除，等"手工标注架构元素"那一版再加回来，那时字段语义才定得准 |
| 9 | 3.2 `Repo`：`URL`/`Branch`/`Lang` | 保留 `Lang`，去掉 `URL`/`Branch` | 本切片不碰 Git，这两列无数据来源 |
| 10 | 3.2 `Relationship`：有 `ID`、`Description` | 去掉两个，加 `Weight` | `(from,to,type)` 三元组去重后天然唯一（§6.4），再造 surrogate ID 只是同步成本；`Description` 无来源，其"出现次数"语义改由 `Weight` 精确承载 |
| 11 | 4.2 `LanguageAdapter`：`Parse(root string)`、`Detect(root string) bool` | `Parse(ctx, root)`、`Detect(root) (string, error)` | 没有 `ctx` 就实现不了 §10 的取消；`Detect` 返回 bool 无法区分"不是 Go"与"目录读不了"，而这两条在 UI 上要给出不同提示 |
| 12 | 6.3：PoC 验证清单是空的 `- [ ]` | 填成 7 条，含每条失败时的分支（§12） | — |

---

## 4. 切片范围

### 4.1 包含

1. 扫描**单个本地 Go 仓库**（一个 `go.mod`）→ `[]Element` + `[]Relationship`
2. `LanguageAdapter` 接口 + 注册表（沿用源文档 4.2 的形状，签名按偏差 11 调整），本切片只有 `go` 一个实现
3. 语言探测 `detector.go`：MVP 只判 `go.mod` 存在
4. SQLite 落库：一次扫描 = 一个 snapshot，单事务写入
5. 图构建 + 邻居/多跳查询 + 只读影响分析
6. 桌面 UI：`sidebar` 外壳三页（架构图 / 仓库 / 设置）、`flow` 主视图、单击选中→右侧详情、双击→方法下钻、影响范围模式、`command` 命令面板（Cmd+K）、扫描进度、相机与层级记忆（zoom/pan/layer 持久化到 `app_config`，§9.5）
7. headless CLI `cmd/oac`：`scan <path>` / `graph <repo-id>` / `impact <element-id> [--depth N]` / `context <repo>`（markdown 架构摘要，供 agent 上下文注入），与 GUI 共用全部 core。`graph`/`impact`/`context` 支持 `--json`，输出 schema 按对外契约对待（切片 7 的 agent 子系统与外部工具都消费它，比 GUI 更怕漂移）

### 4.2 排除（各需自己的 spec）

其他语言 adapter · 特性抽取（Git log / PR / Issue → Feature）· `features`/`commits` 表 · GitHub/GitLab API · 多仓库图合并（`Graph.Merge` 留签名不实现）· 快照对比视图 · Webhook 服务 · OpenTelemetry · `ui/web/architecture-map`（React Flow 备用路线）· 架构元素手工标注 · Windows/Linux 打包验证 · 智能体子系统（切片 7，见 `2026-10-23-openarchitect-agent-harness-design.md`；本切片的 CLI 只读输出是它的全部接口面）

### 4.3 完成定义（DoD）

给定一个真实的多 package Go 仓库：

1. `go tool mygo dev` 起窗口后 **60 秒内**出图；
2. 单击一个 struct 节点 → 右侧抽屉显示其包、文件路径、导出方法列表、直接上游与直接下游，点上游条目能用 `State.Reveal(id)` 把镜头移到该节点；
3. 打开"影响范围"开关 → 选中节点 2 跳之外的节点全部淡出（`Muted`）；
4. `oac scan <path>` 无 GUI 产出同一份数据，`oac impact <id> --depth 2` 的节点集合与 GUI 影响范围模式**完全一致**（这是 core/UI 解耦的验收手段）；
5. `go vet ./...` 与 `go test ./...` 全绿；
6. core 包不 import 任何 UI 包，由 CI 检查强制（§5.1）；
7. 出图后再次扫描，图视图相机保持原位；若 PoC-4 证明 `flow.State` 不保持跨数据更新的 zoom/pan，退化为 `Reveal` 到记忆锚点节点（§9.5）。

---

## 5. 架构

### 5.1 模块边界与依赖规则

```
cmd/openarchitect ──▶ internal/ui ──┐
                                    ├──▶ internal/scanner ──▶ internal/core/model
cmd/oac ────────────────────────────┤                        internal/core/store
                                     └───────────────────────▶ internal/core/graph
```

两条硬规则（外加一条明确的例外）：

- **R1**：`internal/core/**` 与 `internal/scanner/**` 不得 import **UI 栈**：`github.com/egoist/mygo`（根包，负责窗口与生命周期）、`github.com/egoist/mygo/ui`、`github.com/oneo-me/onevra/**`。
  - **唯一例外**：`github.com/egoist/mygo/plugins/sqlite` 允许，且只允许出现在 `internal/core/store`。它是数据库驱动，不是 UI；§7 的整个存储层就建在它上面。逃生通道针对的是渲染栈，不是驱动。
- **R2**：`cmd/oac` 与 `cmd/openarchitect` 不含业务逻辑，只做装配。
- **R3**：UI 只读 core 的输出类型，core 不知道 UI 存在。

R1 由 CI 机械检查（`.github/workflows/ci.yaml`）。`go list -deps` 输出的是精确导入路径，所以用锚定匹配 —— 不能用前缀匹配，否则 `mygo/plugins/sqlite` 会被误判违规：

```sh
go list -deps ./internal/core/... ./internal/scanner/... \
  | grep -x -E 'github.com/egoist/mygo|github.com/egoist/mygo/ui|github.com/oneo-me/onevra[^ ]*' \
  && exit 1 || true
# 反向守门：驱动只准出现在 store
go list -deps ./internal/scanner/... ./internal/core/model/... ./internal/core/graph/... \
  | grep -x 'github.com/egoist/mygo/plugins/sqlite' && exit 1 || true
```

这条规则是**逃生通道**：若 §12 PoC-4 证明 `flow` 扛不住真实节点量，B 路线（WebView + React Flow）只替换 `internal/ui`，core 与其测试不动。附带收益：core 无需 GUI 即可测试。

依赖方向图与 R1–R3 同时写进 README，作为对贡献者（含 agent）的公开契约：先对齐分层再动手，违规由 CI 拒绝而不是靠 code review 记忆。

### 5.2 目录结构

```
cmd/openarchitect/main.go            # mygo App.Run + WhenReady + NewWindow
cmd/oac/{main.go,scan.go,graph.go,impact.go}
internal/core/model/{element.go,relationship.go,snapshot.go,repo.go,ids.go}
internal/core/store/{db.go,migrate.go,schema.go,repo.go,snapshot.go,element.go,relationship.go,rows.go}
internal/core/graph/{graph.go,bfs.go,impact.go}
internal/scanner/{scanner.go,detector.go,report.go}
internal/scanner/language/{adapter.go,go.go}
internal/config/config.go            # app_config 读写：扫描目录、深度默认值
internal/ui/{app.go,shell.go,page_graph.go,page_repos.go,page_settings.go,detail_panel.go,command.go,node_card.go,state.go}
testdata/fixtures/{simple-multi-main,shared-lib,generic-and-iface}/
docs/superpowers/specs/
go.mod  mygo.json
```

与源文档 4.1 的差异：删去 `internal/feature`、`internal/impact`、`internal/integration`（后续切片）、`ui/native/` 与 `ui/web/`（改用 `internal/ui`，因为没有 WebView 路线）、`assets/icons`（onevra 自带 Lucide 图标访问器）。影响分析归入 `internal/core/graph`，因为它就是图遍历，不该单列一层。

### 5.3 数据流（扫描一次）

```
UI: 用户点"扫描" / CLI: oac scan <path>
  → app.status = scanning；goroutine 启动（不阻塞主线程）
  → detector.Detect(root) → "go"
  → language.Get("go").Parse(ctx, root)
        go/packages.Load(NeedSyntax|NeedTypes|NeedTypesInfo|NeedDeps|NeedModule, root module)
        → 遍历模块自身包：导出 named type → Element
        → 遍历 types.Info 与 CallExpr → Relationship
        → 包级错误累积进 ScanReport，不中断
  → store.SaveSnapshot(ctx, snap, elements, rels)   # 单事务，全成或全滚
  → win.Update(func(){ app.snapshot = snap; app.graph = graph.Build(...) })   # 回主线程 + 触发新帧
  失败路径 → win.Update(func(){ app.status = err })   # 旧图保留
```

`Window.Update` 是 goroutine 与主线程之间唯一的交接点；core 全程不碰 UI。

---

## 6. 架构模型与映射规则

### 6.1 Go 结构（源文档 3.2 按偏差 8/9 收缩后）

```go
package model

type ElementType string
const (
    TypeSystem    ElementType = "System"
    TypeContainer ElementType = "Container"
    TypeComponent ElementType = "Component"
)

type Element struct {
    ID          string      // 稳定全局唯一，见 6.2
    Type        ElementType
    Name        string
    Description string      // 取自 go/types 的 Doc comment
    RepoID      string
    FilePath    string      // Component 的声明位置，可空
}

type RelType string
const (
    RelCalls     RelType = "calls"
    RelDependsOn RelType = "depends_on"
    RelContains  RelType = "contains"
)

type Relationship struct {
    FromID string
    ToID   string
    Type   RelType
    Weight int      // 同一 (from,to,type) 的出现次数，用于边宽
}
```

`Relationship.ID` 去掉：`(from,to,type)` 三元组在本模型里天然唯一（§6.4 去重后），再造一个 ID 只是同步成本。`ArchitectureSnapshot` 保留源文档字段，`CommitHash`/`Branch` 本切片写空串。

### 6.2 ID 规则（必须跨扫描稳定）

`flow.Node.ID` 的契约是 "nonempty, unique and stable across frames"，快照对比也依赖这一点，因此 ID 不含 snapshot id、不含自增值：

| 元素 | 规则 | 例 |
|---|---|---|
| System | `sys:<modulePath>` | `sys:example.com/cart` |
| Container | `ctn:<modulePath>/<pkgPath>` | `ctn:example.com/cart/cmd/api` |
| Component | `cmp:<modulePath>/<pkgPath>.<TypeName>` | `cmp:example.com/cart/internal/basket.Basket` |

泛型类型（`type Box[T any] struct`）用声明名 `Box`，丢弃参数 —— 一个类型只出一个节点。同名冲突（同一 pkgPath 下 `Box` 不可能重复，Go 已禁止）无需处理；不同 pkgPath 因路径入 ID 而天然区分。

### 6.3 C4 映射

- **System**：被扫仓库本身，恒 1 个。
- **Container**：每个含 `func main` 的 package。Go 里没有比"可执行文件"更接近 C4 "可独立运行单元"的东西。
- **Component**：模块**自身包**内每个导出的 named type（`*types.TypeName` 且 `Obj().IsExported()`，底层为 `struct`/`interface`，`Type() == Obj()` 即命名类型）。
- **排除集**：`_test.go`、`testing` 包、`vendor/**`、依赖模块的类型（`pkg.Path` 不以 `modulePath` 开头）、未导出类型、`main` 包内的类型（main 包只产 Container；其内部类型属于该可执行文件的实现细节，不进架构图）。

**Container 归属规则**（本表最容易需要迭代的一条）：包 P 被**恰好一个** main 包传递引用 → P 的类型作为 Component 挂该 Container（`contains` 边）。被**两个以上** main 传递引用、或不被任何 main 引用 → 挂 System。理由：复制节点会让同一类型在图上出现多次并各自长出边，语义比归属歧义更糟；`arch_elements` 里该 Component 仍只有一行，其 Container 集合由详情面板从 `contains` 边反查列出。

### 6.4 关系产生规则

| 类型 | 判定 |
|---|---|
| `contains` | System→Container（每个 main 包）、Container 或 System→Component（按 6.3 归属）。不参与影响分析扩散（§8.2） |
| `depends_on` | A 所在文件中，`types.Info.Uses[ident]` 解析到 `*types.TypeName` 且其对象是另一个 Component B 的类型 → `A depends_on B`。方向 = 使用方指向被使用方 |
| `calls` | A 所在包的方法体内 `*ast.CallExpr` 的被调函数解析到 `*types.Func`，其 receiver 基类型是另一 Component B → `A calls B`。接口方法调用记录到接口类型本身（静态解析终点），不做动态分派猜测 |

**位置归属的补集**（实现期补，出处见下）：源码位置落在**包级函数**（非任何组件的方法）体内时，`depends_on` 与 `calls` 都退给「同文件内最近的前置组件声明」作主语；该文件里没有前置组件，则这段代码不产边。这条不是 §6.4 原表覆盖的情形，是实现里必须有的一条兜底——把它写在这里，是为了让 golden 冻结的是明确规则而不是巧合。出处：计划 Task 10 的 controller 裁决（`testdata/fixtures/shared-lib/core/core.go` 的 `Snapshot` 就是钉这条兜底的 fixture）。

**去重与多重性**：同一 `(from, to, type)` 合并为一条边，`Weight` 累加出现次数。自环（同包内两个类型之间）保留但 UI 不渲染为边，只在详情面板的"包内耦合"计数里显示 —— 力导向/分层布局对自环都处理得不好。

**规模预期**（决定 §9 的性能约束）：200~600 个 Component、1000~2000 条边是一个中等 Go 仓库的量级。这个预期在 §12 PoC-3 变成实测数字，实测超 800 节点则加包级预筛（把非 handler/service/client/store 命名后缀的类型折叠进"包"节点）。PoC-3 实测（2026-10-19，`go/packages` 全串 LoadMode）：mygo 模块（108 包）自身包导出 named struct/interface 类型 519 个、onevra `source`（29 包）138 个（均含 main 包类型，是 §6.3 口径下 Component 数的上界）——"200~600"对百包级仓库成立，更小仓库按包数线性走低，未触发 800 节点预筛分支。

---

## 7. 存储层

### 7.1 DDL（5 张表，`schema.go` 里的常量）

```sql
CREATE TABLE IF NOT EXISTS repos (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    local_path TEXT NOT NULL,
    lang TEXT NOT NULL,
    created_at TEXT DEFAULT (datetime('now')),
    updated_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS architecture_snapshots (
    id TEXT PRIMARY KEY,                       -- 's<unixnano>'，字典序即时间序
    repo_id TEXT NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
    commit_hash TEXT NOT NULL DEFAULT '',
    branch TEXT NOT NULL DEFAULT '',
    package_errors_json TEXT NOT NULL DEFAULT '[]',   -- ScanReport 原文，仅审计用
    created_at TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_snapshots_repo ON architecture_snapshots(repo_id);

CREATE TABLE IF NOT EXISTS arch_elements (
    snapshot_id TEXT NOT NULL REFERENCES architecture_snapshots(id) ON DELETE CASCADE,
    element_id TEXT NOT NULL,
    element_type TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    repo_id TEXT NOT NULL,
    file_path TEXT NOT NULL DEFAULT '',
    methods_json TEXT NOT NULL DEFAULT '[]',
    PRIMARY KEY (snapshot_id, element_id)
);
CREATE INDEX IF NOT EXISTS idx_elements_repo ON arch_elements(repo_id);
CREATE INDEX IF NOT EXISTS idx_elements_name ON arch_elements(name);
CREATE INDEX IF NOT EXISTS idx_elements_element ON arch_elements(element_id);

CREATE TABLE IF NOT EXISTS arch_relationships (
    snapshot_id TEXT NOT NULL REFERENCES architecture_snapshots(id) ON DELETE CASCADE,
    from_id TEXT NOT NULL,
    to_id TEXT NOT NULL,
    relationship_type TEXT NOT NULL,
    weight INTEGER NOT NULL DEFAULT 1,
    PRIMARY KEY (snapshot_id, from_id, to_id, relationship_type)
);
CREATE INDEX IF NOT EXISTS idx_rel_from ON arch_relationships(from_id);
CREATE INDEX IF NOT EXISTS idx_rel_to ON arch_relationships(to_id);

CREATE TABLE IF NOT EXISTS app_config (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
```

本 DDL 与已提交的 `internal/core/store/schema.go` 逐字对齐，两处修正是有意为之而非笔误：`arch_elements.methods_json` 一列来自计划修正案 1（§16 U3 选定选项 (a)，扫描时写入导出方法名）；索引名由原文 `idx_elements_type` 更正为 `idx_elements_element`（该索引建在 `element_id` 上，原名与自身目标列矛盾）。

相对源文档 5.1 的改动：删 `elements_json`/`relationships_json`（偏差 5）、删 `features`/`commits`（偏差 7）、删 `arch_elements.id` 自增主键与 `metadata_json`/`tech_stack`/`owner`（偏差 8）、`repos` 去 `url`/`branch`（偏差 9）、`package_errors_json` 新增（承载 §10 的部分失败）、`weight` 新增（承载 §6.4 的多重性）。

主键用复合键而非 `id TEXT PRIMARY KEY` + 业务列：同一 `element_id` 会出现在多个 snapshot 里，跨 snapshot 唯一性由 `(snapshot_id, element_id)` 表达最直白，也免掉自造 surrogate。

### 7.2 读写约定

```go
package store

func Open(ctx context.Context, path string) (*DB, error)      // 包一层 sqlite.Open + Migrate
type DB struct{ handle *sqlite.DB; path string }

func (d *DB) Migrate(ctx context.Context) error
func (d *DB) UpsertRepo(ctx context.Context, r model.Repo) error
func (d *DB) SaveSnapshot(ctx context.Context, s model.ArchitectureSnapshot,
                          e []model.Element, r []model.Relationship) error
func (d *DB) LatestSnapshotID(ctx context.Context, repoID string) (string, error)
func (d *DB) Elements(ctx context.Context, snapshotID string) ([]model.Element, error)
func (d *DB) Relationships(ctx context.Context, snapshotID string) ([]model.Relationship, error)
func (d *DB) GetElement(ctx context.Context, snapshotID, elementID string) (model.Element, error)
func (d *DB) SearchElements(ctx context.Context, snapshotID, query string) ([]model.Element, error)  // name LIKE，命令面板用
func (d *DB) ConfigGet/ConfigSet(ctx context.Context, key string) (string, error)
```

三条约定：

- **W1 列名映射**：所有 `Query` 结果按 `Rows.Columns` 建 `map[string]int` 再取值，**不按下标**。`Rows` 是全量物化，几百行无所谓；`rows.go` 提供 `scanRow(values [][]any, i int, cols []string) map[string]any`。理由见 §2.3 —— 按下标取列会在未来加一列时静默错位。
- **W2 单事务写**：`SaveSnapshot` 预先生成全部 `Statement`（1 条 snapshot + N 条 element + M 条 relationship，实测约 2100 条），一次 `Transaction` 提交。全成或全滚，不留半成品快照。批量大小在 PoC-5 实测。
- **W3 无 ORM**：SQL 以常量写在 `schema.go` 与各文件顶部，参数一律 `?` 占位（`Execute`/`Query` 支持 `args ...any`）。任何用户输入的 path/name 只进 Args，绝不拼进 SQL 串。

库文件路径：GUI 用 `mygo.App.Path(mygo.PathUserData)` + `openarchitect.db`；`cmd/oac` 默认同一路径，`--db` 覆盖（测试用 `t.TempDir()`）。

schema 版本：用 SQLite 内建 `PRAGMA user_version`，从 1 起递增，`Migrate` 按版本号分支。`app_config` 只存用户配置与视图状态，不承担版本语义；key 用两个前缀区分：`config.*`（用户配置，如影响分析 `maxDepth`）与 `view.*`（视图状态，如图相机，§9.5），value 一律 JSON 字符串。

---

## 8. 图查询与影响分析

### 8.1 API（源文档 4.4 收缩）

```go
package graph

type Graph struct{ /* 邻接表：out map[string][]edge, in map[string][]edge, elements map[string]model.Element */ }

func Build(elements []model.Element, rels []model.Relationship) *Graph
func (g *Graph) Elements() []model.Element
func (g *Graph) Relationships() []model.Relationship
func (g *Graph) Neighbors(id string, dir Direction) []model.Element
func (g *Graph) Upstream(id string, maxDepth int) []Hit     // 谁依赖/调用我
func (g *Graph) Downstream(id string, maxDepth int) []Hit   // 我依赖/调用谁
func (g *Graph) Merge(other *Graph)                         // 本切片返回 ErrNotImplemented

type Hit struct { Element model.Element; Depth int; Via RelType }
type Direction uint8
const (DirIn Direction = iota; DirOut; DirBoth)
```

`Graph` 用 struct 而非源文档的 interface：只有一个实现，接口在此只是把一次指针传递换成两次间接。`Merge` 保留签名和 `ErrNotImplemented`，因为多仓合并是切片 2 的入口，届时 `Build` 的调用方不必改形。

### 8.2 扩散语义

- BFS，`visited` 集合守环（真实代码里包级循环依赖常见），`depth <= maxDepth` 截断。
- **参与扩散的边类型只有 `calls` 与 `depends_on`**；`contains` 排除。理由：`contains` 是层级边，纳入会让一个 System 节点在 1 跳内吞掉整棵树，影响范围立刻失真。
- `Upstream` 沿 `in` 边、`Downstream` 沿 `out` 边，`DirBoth` 留给命令面板的"邻居"快捷动作。
- 结果按 `(Depth, Name)` 稳定排序，保证 §4.3 DoD-4 里 CLI 与 GUI 可逐行比对。

### 8.3 影响分析输出

```go
type ImpactReport struct {
    Center     model.Element
    Upstream   []Hit
    Downstream []Hit
}
func Analyze(g *Graph, elementID string, maxDepth int) (ImpactReport, error)
```

CLI 打印成两张表；GUI 把它转成 `Muted` 集合喂给 `flow`。UI 上"影响范围"的默认 `maxDepth = 2`，可在设置页改，存 `app_config`。

---

## 9. UI

### 9.0 交互范式约束（负面清单）

主视图是**可矢量缩放的全景画布**（相机模型：pan / zoom / `Reveal`），不是列表产品。C4 三层（§6.3 的 System → Container → Component）天然就是三级语义缩放——缩放级别与抽象级别一一对应，这是让架构模型可读的唯一自然方式。以下形态明确排除，防止后续切片滑回列表语义：

- 主视图不做列表 / board / table 形态；「仓库」页是管理页，不是浏览页。
- 过滤与分析一律用**聚光语义**（`Muted` 淡出、边变细、层级显隐），不做「筛出剩余集合」——空间记忆不可破碎。
- 导航是**相机移动**（`Reveal`、缩放、层级切换 + 下钻面包屑），不是翻页路由；详情面板里的上下游条目是「跳到图上」的动作，不是列表跳列表。
- 详情面板是图的从属注解（`Split` 右侧、固定宽度），不是换屏抽屉。

### 9.1 外壳与页面

`shells/sidebar` 单窗口，页面：`架构图`（默认）、`仓库`、`设置`。`command` 组件实现 Cmd+K 面板，条目 = 「扫描当前仓库」「跳到仓库/设置」「搜索元素（`store.SearchElements`）」「以 X 为中心的影响分析」。侧栏底部一个扫描按钮 + 状态行。

### 9.2 app state

```go
type App struct {
    status     Status              // Idle | Scanning | Ready | Failed(err)
    progress   string              // 扫描阶段文案
    repos      []model.Repo
    snapshot   *model.ArchitectureSnapshot
    graph      *graph.Graph
    selected   string              // flow.Options.Selected
    impactMode bool                // 影响范围开关
    impact     *graph.ImpactReport
    flowState  flow.State          // 每图一份，按 views.md 归应用持有
    layer      string              // "container" | "component"，语义缩放层级（§9.5）
    camera     Camera              // 全景记忆：zoom + 锚点 + 层级，持久化到 app_config（§9.5）
    cancel     context.CancelFunc
}

// state.go
type Camera struct {
    Zoom   float32
    Anchor string  // 相机中心对准的 element id，'' = Fit
}
```

视图函数是 `App` 的纯函数，只读字段、不写库、不起 goroutine；写状态只发生在两处 —— 输入回调（主线程）与 `win.Update`（跨线程）。

状态树按持久化语义分四类：`status/progress` 瞬态，不落盘；`repos/snapshot/graph` 是数据，可从 SQLite 重建；`selected/impact/layer` 是会话态；`camera` 是视图状态，防抖写入 `app_config` 的 `view.*`（§9.5）。瞬态与视图状态永不混进同一字段。

### 9.3 flow 用法

```go
flow.View(c, &app.flowState, data, flow.Options{
    Direction: flow.LeftToRight,
    Selected:  app.selected,
    Select:    func(id string) { app.selected = id },        // 单击；点空白传 ""，抽屉自动收起
    Open:      func(id string) { app.drillInto = id },       // 双击 → 方法列表
    Overlay:   zoomControls,                                 // Fit / ZoomIn / ZoomOut
    Empty:     emptyHint,
    NodeGap:   28, RankGap: 64,
})
```

- `Node.Content` = `nodeCard(c, el)`：**一个 Lucide 图标 + 类型名一行 + 包名一行**。图标按 `ElementType` 三选其一。这是 §2.1 性能基线推出来的硬约束 —— flow 每帧重建每张卡片，1000 静态 label 已花 397 µs，卡片里塞按钮、进度条或多行富文本会把帧时间推到几十毫秒。`Content` 内部按 `flowState.Zoom()` 做 LOD 分级绘制（§9.5）。
- **卡片尺寸必须恒定**：`Width/Height` 显式给定，不让 `Content` 自然测量。flow 在测量尺寸变化时会重排（"laying the diagram out again when that size changes"），否则文本长度差异会让图逐帧抖动。超长包名中间截断。
- 边：`Edge.Label` = 关系类型；`depends_on` 用 `Dashed: true`，`calls` 用实线 + `Width: 1 + min(3, weight/8)` 让强依赖更粗。
- `Group`：`flow` 没有 `Group` 字段（那是 `graph` 的），聚类改由图标 + 卡片第二行的包名承担。
- 影响范围模式：`app.impactMode` 为真时，把 `Upstream ∪ Downstream ∪ {center}` 之外的所有节点 `Muted: true`；中心节点通过 `Selected` 自带描边。

### 9.4 详情面板

`Split` 右侧抽屉，宽度固定；数据源 = `Elements`/`Relationships` + `Neighbors`。展示：类型名、`ElementType`、包路径、**所属 Container 集合**（从 `contains` 边反查，承载 §6.3 的多归属歧义）、`file_path`（点击用 `mygo.Shell` 打开所在文件）、doc comment、导出方法列表（来源见 §16 U3）、直接上游、直接下游。后两类的条目可点击 → `app.selected = id` + `flowState.Reveal(id)`。注意这是「跳到图上」，不是列表跳列表（§9.0）。

### 9.5 缩放层级、LOD 与相机

**两种缩放，机制不同，不可混为一谈：**

- **连续矢量缩放**（`flow.State` 的 `Zoom/ZoomIn/ZoomOut/Fit` 区间内）：节点几何绝不变（§9.3 定死尺寸的约束——flow 在尺寸变化时重排，否则逐帧抖动），只变**绘制密度**。`Node.Content` 读 `flowState.Zoom()` 分级：远端只画色块 + 类型图标（一次 fill，比三行文本便宜得多，直接回应 §2.1 的 397 µs/1000 label 基线），中距画类型名 + 包名，近端可加第三行辅助信息；边 `Label` 低于阈值隐藏，只留颜色与虚线样式。
- **语义缩放（层级切换）**：Container 视图 ↔ Component 视图是**显式模式切换**——重排一次布局，随后相机对准切换前的焦点节点。这是一等导航机制，不只是 PoC-4 可读性不利时的兜底；`App.layer` 记录当前层级，下钻面包屑（System › repo › pkg）就是它的呈现。

**相机是一等状态**：`App.camera`（zoom + 锚点）与 `App.layer` 合并成一个 JSON 持久化到 `app_config` 的 `view.graph.<repoID>`，防抖写入（约 150 ms，不逐帧写库）。扫描完成触发重布局后相机不得跳回原点——`flow.State` 归应用持有正是为此。`flow.State` 的 zoom/pan 能否外部设定、能否跨数据更新保持，由 PoC-4 实测（§12）；若不可设定，退化为「重扫后 `Reveal` 到记忆锚点」。

**动效是一次性标志**：扫描完成的新节点淡入、层级切换的过渡，用「置标志 → 下一次渲染读取打标 → 立即清空」的一次性标志驱动，只播一次。mygo 每帧重绘的模型下，「渲染即动效」会逐帧重放，必须杜绝。

**Minimap（stretch）**：600 节点放大后必然迷失。`flow` 无内建 minimap，但 `State.Position(id)` 足够在 `Overlay` 里自绘点阵 + 视口矩形。切片 1 可不做，切片 2（多仓合并图）前必须有。

**扫描加载态**：切片 1 用 `Progress` + 状态行（§10）；画布式加载（节点团块逐个点亮）是后续改进。

---

## 10. 错误处理与取消

| 场景 | 行为 |
|---|---|
| `go/packages` 包级类型错误 | **不中断**。累积进 `ScanReport.PackageErrors`，随快照存 `package_errors_json`；成功解析的包照常入库。UI 状态行显示"完成，N 个包有错误"，详情见报告。理由：残缺视图比没有视图有用，且真实仓库里测试数据/构建约束包常年报错 |
| 根目录无 `go.mod` | `detector` 返回"不支持的语言"，UI 直接提示，不起 goroutine |
| 解析整体失败（模块加载不起来） | 不写库；`status = Failed`；保留上一版快照与旧图 |
| `SaveSnapshot` 事务失败 | W2 保证全滚；提示错误，旧快照仍是 `LatestSnapshot` |
| 扫描中用户取消 / 关窗 | `ctx` cancel → goroutine 退出（不写库）→ `win.Update` 只把 `status` 置回 `Idle` |
| goroutine 与主线程竞态 | 只有 `win.Update` 写 `App`；`App` 字段不加锁，靠这一条纪律约束。`context.CancelFunc` 存 `App.cancel`，只在主线程读写 |
| UI 等待扫描 | 进度用 `components.Progress` + `app.progress` 文案；期间重复点扫描被 `status == Scanning` 挡住 |

取消只在扫描这一条长路径上实现；`store` 的每个方法收 `ctx` 并透传给 `sqlite`，但不主动加超时（本地单用户，加超时只会制造假故障）。

---

## 11. 测试

core 不 import UI（R1），所以是真实单元测试，不是快照测试。

| 层 | 内容 |
|---|---|
| fixture | `testdata/fixtures/` 三个手工构造的小 Go 仓库：`simple-multi-main`（两个 cmd/main 共享一个 lib 包 → 验证 6.3 归属歧义规则）、`shared-lib`（多 main + 循环依赖包 → 验证环与多归属）、`generic-and-iface`（泛型类型 + interface + `var _ Iface = ...` 断言 + 未导出类型 + `_test.go` → 验证 6.3 排除集与泛型去参） |
| scanner | 每个 fixture 的 elements/relationships 与 golden 文件比对；**排序后比**（`go/packages` 的包遍历顺序不保证，不排序就是随机失败）；ID 规则单测（6.2 三种前缀）；包级错误累积测试 |
| store | `t.TempDir()` 建库；迁移幂等（跑两次不报错、`user_version` 正确）；`SaveSnapshot` 原子性（故意让第 N 条语句失败，断言库里无任何新行）；2100 条语句事务的耗时基准（`Benchmark`，PoC-5 的复测点）；`scanRow` 列名映射测试（故意乱序 `Columns`）；SQL 注入面测试（path 带 `'` 与 `;`） |
| graph | BFS：环、深度截断、`contains` 不扩散、方向正确性、`(Depth,Name)` 排序稳定性；`Merge` 返回 `ErrNotImplemented` |
| ui | `ui.NewTester(view, w, h)` + `tt.Advance(d)`（用法见 onevra `graph/example_test.go`）：断言 flow 收到的 Node 数 == elements 数、`Select` 回调写入 `app.selected`、影响范围模式下 `Muted` 数量正确、节点卡片尺寸恒定（两次 `tt.Advance` 后 `Position(id)` 不漂移） |
| 端到端 | fixture → scan → store → graph → flow 构造 `Data`，全程不 panic；以及 DoD-4 的一致性检查：`oac impact` 输出与同一 fixture 的 `graph.Analyze` 集合逐行相等 |
| CI | `go vet ./...` + `go test ./...` + R1 的 `go list -deps` 检查。三平台构建（官方 Actions 模板）留给发布切片 |

不为覆盖率设指标；每个 fixture 都要能对应到一条会被它抓到的真实回归。

---

## 12. PoC 验证清单（替换源文档 6.3 的空清单）

顺序 = 最可能失败且最贵的前置。每条一天内出结论。

| # | 验证 | 失败时的分支 |
|---|---|---|
| 1 | `mygo init -template native` 骨架里 `go get` onevra，`go tool mygo dev` 能否起窗口 | 骨架手搭（`go mod init` + `mygo.json` 照 onevra `source/mygo.json` 抄），或降级 `go run .` 不做打包 |
| 2 | native app 中 `sqlite.Open` 成功，且 `go tool mygo build` 后 `sqlite.LibraryPath()` 在 .app 内能找到预编译库 | 换 `modernc.org/sqlite`（纯 Go 转译，慢但零二进制）。替换面被 §7.2 的 `*DB` 包装层与 `rows.go` 完全吸收：`store` 的公开方法签名不变，上层无感。此时 R1 的驱动例外条款作废，改为允许 `modernc.org/sqlite` |
| 3 | 真实仓库（300~600 导出类型）上 `go/packages` 的耗时与峰值内存 | >60s 或 >2GB → 加包级预筛、或只加载模块自身包（`NeedDeps` 收窄）；这条数字同时回填 §6.4 的规模预期 |
| 4 | flow 在 300 / 600 节点的帧时间与可读性；顺带实测 `flow.State` 的 zoom/pan 是否跨数据更新保持、能否外部设定，`Position(id)` 是否够自绘 minimap（§9.5） | 不可用 → 走逃生通道切 WebView + React Flow（只换 `internal/ui`）；可读性差但性能 OK → 默认只画 Container 层，Component 靠双击下钻；相机不可保持 → 相机恢复退化为 `Reveal` 记忆锚点 |
| 5 | 单 `Transaction` 塞 2100 条 `Statement` | 过慢/失败 → 分块提交（每 200 条一批）+ `Savepoint` 语义自己实现，牺牲 W2 的原子性；此时必须补一条"半成品快照不可见"的规则 |
| 6 | `Select` → 抽屉 + `Muted` 淡出是否足以表达影响范围 | 不够 → 退化为"影响分析只在 CLI/表格里看"，图保持纯浏览 |
| 7 | 无签名 .app 本地能否运行（Gatekeeper） | 只影响分发，不阻塞切片；必要时 `xattr -d com.apple.quarantine` 写进 README |

PoC 1/2 在任何实现工作之前跑完 —— 它们决定工程骨架形状。

---

## 13. 风险登记

1. **onevra 只有 13 天大，pre-1.0**（`source/v0.2026.12` 昨天才发）。API 会变，`flow`/`graph` 是新组件、边界情况未验证。**缓解**：`go.mod` 精确 pin 版本、不追 `@latest`；UI 层集中所有 onevra 调用（R1 保证升级破坏面只在 `internal/ui`）；每次升级跑 §11 的 ui 测试。
2. **mygo 本身 3 周大**（v0.5.5，2026-09-26 创建）。桌面框架的地基处于早期。**缓解**：同上，core 不依赖它；`mygo doctor` 进开发环境检查步骤。
3. **`flow` 的分层布局对 600 节点 × 1800 边没有实测记录**。风险最集中的地方，由 PoC-4 定生死，逃生通道已就位。
4. **映射规则是启发式，一定会需要迭代**（尤其 §6.3 的 Container 归属和"main 包内类型不进图"）。**缓解**：规则全部集中在 `language/go.go` 一小段，用 fixture 锁住；迭代只改 golden 文件不改接口。
5. **`SQLite` 插件的打包依赖预编译 C 库随 bundle 分发**，这是 §7 全部工作的前提，PoC-2 未过则整层换 driver。

---

## 14. 环境与初始化（命令，不含实现代码）

```sh
# 1) 用户手动：把 Go 加进 PATH（本设计不改动 shell 配置）
export PATH="/usr/local/go/bin:$PATH"          # 追加进 ~/.zshrc

# 2) 模块代理（proxy.golang.org 在此网络不可达）
go env -w GOPROXY=https://goproxy.cn,direct

# 3) 脚手架（原生 UI 只需要 Go）
go run github.com/egoist/mygo/cmd/mygo@latest init -template native openarchitect

# 4) 依赖，精确 pin
go get github.com/oneo-me/onevra/source@v0.2026.12
go get github.com/egoist/mygo@v0.5.5

# 5) 自检与开发循环
go run github.com/egoist/mygo/cmd/mygo@latest doctor
go tool mygo dev
go tool mygo build
```

已核实可用的版本对：`mygo v0.5.5` + `onevra source@v0.2026.12` + `go1.27.2 darwin/arm64`（onevra 自身要求 go ≥ 1.27.1，且它依赖的正是 mygo v0.5.5）。

---

## 15. 后续切片路线（每个一份 spec）

| 切片 | 内容 | 依赖本切片处 |
|---|---|---|
| 2 | 多仓库图合并（实现 `Graph.Merge`）+ 快照对比视图 + `uses` 边 | 6.2 的稳定 ID 是为跨仓对齐准备的 |
| 3 | Git 集成 + 特性抽取（commit/PR/Issue → Feature）+ `features`/`commits` 表 + 进度追踪 + `implements` 边 | `snapshot.commit_hash` 列已留空位 |
| 4 | 手工标注架构元素（把偏差 8 砍掉的 `Owner`/`TechStack`/`Metadata` 加回来，此时字段语义有数据来源了）+ Webhook（Orca/Paperclip 协作） | `app_config` 与 store 写路径 |
| 5 | 非 Go 语言 adapter（TS/JS、Python、Java、Rust），全部 sidecar 子进程 | `LanguageAdapter` 接口 |
| 6 | 三平台打包、签名公证、GitHub Actions 发布、自动更新 | `mygo.json`、PoC-7 |
| 7 | 智能体子系统：**分析 agent 为主**（进程内 covonaut，只读工具 = core 查询）+ 变更 rollout 为辅（CLI substrate，harness2 式递归）。草案已立：`2026-10-23-openarchitect-agent-harness-design.md` | 切片 1 的 core 与 `oac --json`/`context`；切片 2 的快照 diff；切片 3 的 Feature |

---

## 16. 未决问题

写实施计划前需要答案，或作为计划第一步的调研项：

- **U1** `shells/sidebar` 的页面注册 API 未逐字核对（只确认了 `sidebar.HeaderHeight(c)` 与"可替换页面"的描述）。计划里 PoC-1 会读到 `source/shells/sidebar/shell.go` 全文，届时回填 §9.1。
- **U2** `go/packages` 的 `LoadMode` 取哪几位（§5.3 写的 `NeedSyntax|NeedTypes|NeedTypesInfo|NeedDeps|NeedModule` 是合理起点，未经 `go/packages` 文档核对，PoC-3 实测确定）。
- **U3** 详情面板要显示"导出方法列表"，但 §6.1 的 `Element` 没有方法字段。两个选项：(a) `arch_elements` 加 `methods_json` 列，扫描时写；(b) 选中时按 `file_path` 现读现解析。倾向 (a)——避免 UI 线程做解析。需要在 §7.1 定稿前决定，它会多一列。
- **U4** 单 main 仓库是否值得显示 Container 层（此时 System→Container 是一条无信息量的链，图多一层）。取决于 PoC-4 的可读性结论。
- **U5** 一个真实仓库里"包内自环耦合"的信息价值有多大，§6.4 已决定不渲染自环；若 §4.3 DoD-2 演示时发现有用，改为渲染。
