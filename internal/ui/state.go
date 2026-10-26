package ui

import (
	"context"
	"errors"
	"fmt"

	"github.com/oneo-me/onevra/source/components/command"
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
	// repoLatest 是 Reload 每次重算的 repo id → 最新快照 id 表（空串 = 没有
	// 快照）。仓库选择器与图页 chips 都只读它。
	repoLatest map[string]string

	// 设置页的保存回执。两个字段都只在主线程写（纪律 1），且 settingsErr
	// 非空时 settingsSaved 必为 false —— saveRow 里同一次赋值保证这一点。
	settingsSaved bool
	settingsErr   error

	selected    string
	drillInto   string
	impactMode  bool
	impact      *graph.ImpactReport
	commandOpen bool

	// showComponents is the type-level fold switch: the default is the
	// Container layer only, per spec §12 row 4's readability branch. PoC-4
	// measured the fit zoom (300 nodes → 0.1348), which scales a 168×46 card
	// down to ~23×6 px. The toggle opts components in; it is never an
	// auto-collapse on node count.
	showComponents bool

	// flowState is the flow camera/viewport state. spec §9.2 mandates App hold
	// it and Task 19 passes &a.flowState to flow.View, so it is deliberately a
	// value field even though nothing here reads it yet.
	flowState flow.State
	// commandState 只在主线程读写：command.State 明令禁止拷贝使用
	// （components/command/command.go:30）。
	commandState command.State
	cancel       context.CancelFunc
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
		db:             db,
		page:           PageGraph,
		graph:          graph.Build(nil, nil),
		showComponents: false,
		post:           func(fn func()) { fn() },
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

// SetShowComponents toggles the type-level fold. Only flowData consults it;
// flowEdges stays the unfiltered translation.
func (a *App) SetShowComponents(on bool) { a.showComponents = on }

// Open drills into id. Like flow's double-click it lands the selection too;
// the spec §9.3 line 515 form only wrote drillInto because there Select ran
// first, but command-palette callers skip the single click.
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

// SetGraphRepo 把图页钉到 repoID 的最新快照并持久化（空串 = 取消钉选，
// 回到"全局最新"）。它走 Reload 而不是就地换图：库是唯一的真相源，
// GUI 与 CLI 的一致性（DoD-4）不允许第二条装载路径。
func (a *App) SetGraphRepo(ctx context.Context, repoID string) error {
	a.settings.GraphRepo = repoID
	if err := config.Save(ctx, a.db, a.settings); err != nil {
		return err
	}
	return a.Reload(ctx)
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
		// No nil guard on a.err: only failScan and finishScan set StatusFailed,
		// and both write err in the same statement.
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
