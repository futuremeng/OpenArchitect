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
