package ui

import (
	"context"
	"errors"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/futuremeng/OpenArchitect/internal/config"
	"github.com/futuremeng/OpenArchitect/internal/core/graph"
	"github.com/futuremeng/OpenArchitect/internal/core/model"
	"github.com/futuremeng/OpenArchitect/internal/forest"
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

	// repoLatest 记下每个仓的最新快照：选择器用它把"没有快照的仓"的按钮
	// 关掉，也让下面的多选回落有据可查。
	latest := make(map[string]string, len(repos))
	for _, r := range repos {
		id, err := a.db.LatestSnapshotID(ctx, r.ID)
		if err != nil {
			return err
		}
		latest[string(r.ID)] = id
	}
	// 多选过滤器：空 settings = 全选（所有带快照的仓）；勾选的仓全都失去
	// 快照时回落到全选，设置本身保留——它下次扫描出快照后自动生效。
	// repos 按 name 排序，并集装载顺序因此确定。
	selected := make(map[string]bool)
	for _, r := range repos {
		id := string(r.ID)
		if latest[id] == "" {
			continue
		}
		if len(settings.GraphRepos) == 0 || slices.Contains(settings.GraphRepos, id) {
			selected[id] = true
		}
	}
	if len(selected) == 0 && len(settings.GraphRepos) > 0 {
		for _, r := range repos {
			if latest[string(r.ID)] != "" {
				selected[string(r.ID)] = true
			}
		}
	}
	a.repoLatest = latest
	a.selectedRepos = selected
	// The selection clear is hoisted to the single point below, so every
	// Reload that completes the read phase ends with a cleared selection —
	// one writer, no asymmetry between the branches. The error returns
	// deliberately leave it unset: a failed read keeps the old screen.
	if len(selected) == 0 {
		a.snapshot = model.ArchitectureSnapshot{}
		a.repoID = ""
		a.graph = graph.Build(nil, nil)
	} else {
		var (
			elements []model.Element
			rels     []model.Relationship
			pkgErrs  []string
			first    model.ArchitectureSnapshot
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
			elements = append(elements, el...)
			rels = append(rels, rl...)
			pkgErrs = append(pkgErrs, snapshot.PackageErrors...)
			if first.ID == "" {
				first = snapshot
			}
		}
		// 状态行的"N 个包有错误"统计的是整个视图，不是其中一个仓。
		first.PackageErrors = pkgErrs
		a.snapshot = first
		if len(selected) == 1 {
			for id := range selected {
				a.repoID = id
			}
		} else {
			a.repoID = ""
		}
		a.graph = graph.Build(elements, rels)
	}
	if err := a.buildFruits(ctx, selected); err != nil {
		return err
	}
	a.refreshView()
	a.selected, a.drillInto, a.impact = "", "", nil
	// 新图配旧锚点没有意义：焦点记忆和选中一起清。
	a.focusElement, a.focusPackage, a.focusRepo = "", "", ""
	return nil
}

// BeginScan detects the language on the main thread first: spec §10's "根目录
// 无 go.mod → 不起 goroutine" row. scanner.Run detects again internally; the
// duplicate is the price of not starting a goroutine for a known-bad path.
// It deliberately does not assign a.repoID: what is on screen is still the
// previously loaded repo until Reload says otherwise, so a failed or
// cancelled scan of repo B cannot make the repo page highlight B while A's
// graph is showing.
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
	a.scanRoot = abs
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
		// Best-effort by ruling, and deliberate per spec §10's silence: a failed
		// app_config write must not invalidate a scan that succeeded. Accepted
		// consequence: memory (a.settings.ScanRoot) and the database then diverge
		// until the next successful save — the settings page shows what the user
		// just scanned, the next launch shows what was persisted.
		_ = a.SaveSettings(context.Background(), a.settings)
	}
}

func (a *App) failScan(err error) error {
	a.status, a.err, a.progress = StatusFailed, err, ""
	return err
}

// fruitEntry 是 fruitLog 的一条：commit 加解析好的时间戳。git 的 %aI 是带
// 偏移的严格 ISO，字典序在不同时区之间不成立，所以时间比较一律走解析后的 time。
type fruitEntry struct {
	cm   model.Commit
	when time.Time
}

// buildFruits indexes the selected repos' agent-attributed commits onto the
// elements that live in the touched directories. One pass at Reload, so the
// per-frame nodeCard only reads a map (frame-time rule: no queries in a card).
// 1.1 批次起它只填原始日志 fruitLog 与目录索引；帧内视图由 reaggregateFruits
// 产出——Reload 与时间滑杆共用那一条路。
func (a *App) buildFruits(ctx context.Context, selected map[string]bool) error {
	a.fruitLog, a.fruitMaxFiles = nil, 0
	a.fruitMin, a.fruitMax, a.fruitCursor = time.Time{}, time.Time{}, time.Time{}
	// 两侧都是 slash 路径（git 输出与 scanner 的 relativeFile），用 path
	// 不用 filepath：Windows 上 filepath.Dir 会把比较打散。
	a.fruitDir = map[string][]string{}
	for _, el := range a.graph.Elements() {
		if el.FilePath == "" {
			continue
		}
		dir := path.Dir(el.FilePath)
		a.fruitDir[dir] = append(a.fruitDir[dir], el.ID)
	}
	for _, r := range a.repos {
		if !selected[r.ID] {
			continue
		}
		commits, err := a.db.Fruits(ctx, r.ID, 200)
		if err != nil {
			return err
		}
		for _, cm := range commits {
			if cm.AgentTag == "" || cm.AgentTag == "human" {
				continue // 果实只收 agent 与 mixed；human 是背景，不是数据
			}
			when, err := time.Parse(time.RFC3339, cm.Date)
			if err != nil {
				when = time.Time{} // 读不懂的时间戳当作最老，永远落在游标之内
			}
			if !when.IsZero() {
				if a.fruitMin.IsZero() || when.Before(a.fruitMin) {
					a.fruitMin = when
				}
				if when.After(a.fruitMax) {
					a.fruitMax = when
				}
			}
			if n := len(cm.Files); n > a.fruitMaxFiles {
				a.fruitMaxFiles = n
			}
			a.fruitLog = append(a.fruitLog, fruitEntry{cm: cm, when: when})
		}
	}
	slices.SortFunc(a.fruitLog, func(x, y fruitEntry) int { return y.when.Compare(x.when) })
	a.reaggregateFruits()
	return nil
}

// reaggregateFruits 从 fruitLog 重算帧内视图（a.fruits 与图例计数），按游标
// 截断。scrub 一次重算一次，全程不查库。
func (a *App) reaggregateFruits() {
	a.fruits, a.fruitByAgent, a.fruitMixed = map[string][]model.Commit{}, map[string]int{}, 0
	for _, e := range a.fruitLog {
		if !a.fruitCursor.IsZero() && !e.when.IsZero() && e.when.After(a.fruitCursor) {
			continue
		}
		if name, ok := strings.CutPrefix(e.cm.AgentTag, "agent:"); ok {
			a.fruitByAgent[name]++
		} else {
			a.fruitMixed++
		}
		seen := map[string]bool{}
		for _, file := range e.cm.Files {
			for _, id := range a.fruitDir[path.Dir(file)] {
				if seen[id] {
					continue
				}
				seen[id] = true
				a.fruits[id] = append(a.fruits[id], e.cm)
			}
		}
	}
}

// setCursorFrac 把滑杆的 0–1 位置换算成时间游标并重聚合。满格是"全部"：
// 游标归零（无过滤），而不是 fruitMax——差一个 commit 的边界不该藏住最新果实。
func (a *App) setCursorFrac(frac float64) {
	if frac >= 1 || a.fruitMax.IsZero() {
		a.fruitCursor = time.Time{}
	} else {
		span := a.fruitMax.Sub(a.fruitMin)
		a.fruitCursor = a.fruitMin.Add(time.Duration(frac * float64(span)))
	}
	a.reaggregateFruits()
}

// BeginSync starts one forest round over the configured scan root. It shares
// the scan guard and the cancel slot deliberately: sync runs scans inside,
// and two concurrent background jobs would race the same database.
func (a *App) BeginSync() error {
	if a.status == StatusScanning {
		return ErrScanInFlight
	}
	if a.settings.ScanRoot == "" {
		return errors.New("ui: 还没有扫描目录，先在设置里填一个")
	}
	a.status, a.err, a.progress = StatusScanning, nil, "同步森林…"
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	go a.sync(ctx)
	return nil
}

func (a *App) sync(ctx context.Context) {
	_, err := forest.Sync(ctx, a.db, a.settings.ScanRoot)
	a.post(func() { a.finishSync(err) })
}

func (a *App) finishSync(err error) {
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
