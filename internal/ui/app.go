package ui

import (
	"context"
	"errors"
	"path/filepath"
	"slices"

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
	a.refreshView()
	a.selected, a.drillInto, a.impact = "", "", nil
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
