package ui

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/futuremeng/OpenArchitect/internal/config"
	"github.com/futuremeng/OpenArchitect/internal/core/model"
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
	// A direct Reload re-reads the database but is not a completed scan, so the
	// scan lifecycle state stays Idle (finishScan is what sets Ready). The
	// empty-database test above pins the same branch.
	if a.repoID != second.RepoID || a.status != StatusIdle {
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

// TestBeginScanCompletesScanThroughPost covers the branch the tests above
// cannot reach: the a.scan goroutine plus finishScan's SUCCESS path
// (Reload → StatusReady → scan.root write-back). post is captured, so the
// goroutine's closure is run manually on the test goroutine — the only writer
// of App fields stays the main thread. No sleeps, no polling: the channel
// receive is the handoff, not a retry loop.
func TestBeginScanCompletesScanThroughPost(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	first := seedScan(t, a, fixtureDir("shared-lib"))
	if err := a.Reload(ctx); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	queued := make(chan func(), 1)
	a.SetPost(func(fn func()) { queued <- fn })

	root := fixtureDir("simple-multi-main")
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.BeginScan(root); err != nil {
		t.Fatalf("BeginScan: %v", err)
	}

	// In-flight state, asserted on the main goroutine before the posted
	// closure runs. repoID must still be repo A's: BeginScan no longer
	// assigns it, so a failed/cancelled scan of repo B can never leave the
	// repo page highlighting B while A's graph is on screen.
	if a.status != StatusScanning || a.progress == "" || a.cancel == nil {
		t.Fatalf("in-flight status/progress/cancel = %v/%q/%v", a.status, a.progress, a.cancel != nil)
	}
	if a.repoID != first.RepoID {
		t.Fatalf("repoID during flight = %q want %q (BeginScan must not reassign it)", a.repoID, first.RepoID)
	}

	(<-queued)() // run the posted finishScan on the main thread

	if a.status != StatusReady {
		t.Fatalf("status after scan = %v, err = %v", a.status, a.err)
	}
	stored, err := a.db.Elements(ctx, a.snapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if a.repoID != model.RepoIDFor(abs) || len(stored) == 0 || len(a.graph.Elements()) != len(stored) {
		t.Fatalf("graph must be rebuilt from stored rows: repoID=%q graph=%d stored=%d",
			a.repoID, len(a.graph.Elements()), len(stored))
	}
	settings, err := config.Load(ctx, a.db)
	if err != nil {
		t.Fatal(err)
	}
	if settings.ScanRoot != abs {
		t.Fatalf("scan.root after a successful scan = %q want %q", settings.ScanRoot, abs)
	}
}

// TestCancelScanFiresInjectedCancel pins a Produces symbol Tasks 19/22 call
// by name: the scan button's cancel path.
func TestCancelScanFiresInjectedCancel(t *testing.T) {
	a := newTestApp(t)
	fired := false
	a.cancel = func() { fired = true }
	a.CancelScan()
	if !fired {
		t.Fatal("CancelScan must call the injected cancel func")
	}
	a.cancel = nil
	a.CancelScan() // safe to call when nothing is running
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

// TestOpenSetsDrillIntoAndSelection pins Open for Task 18's drawer. This is a
// deliberate superset of spec §9.3 line 515, where Open only writes drillInto:
// flow fires Select before Open on a real double-click, so Open landing both
// fields only matters for callers (command palette) that skip the single click.
func TestOpenSetsDrillIntoAndSelection(t *testing.T) {
	a := newTestApp(t)
	seedScan(t, a, fixtureDir("shared-lib"))
	if err := a.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	id := a.graph.Elements()[0].ID
	a.Open(id)
	if a.drillInto != id || a.selected != id {
		t.Fatalf("after Open: drillInto=%q selected=%q want %q", a.drillInto, a.selected, id)
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

func TestSetPageAndCommandOpen(t *testing.T) {
	a := newTestApp(t)
	for _, p := range []Page{PageRepos, PageSettings, PageGraph} {
		a.SetPage(p)
		if a.Page() != p {
			t.Fatalf("Page = %v want %v", a.Page(), p)
		}
	}
	a.SetCommandOpen(true)
	if !a.commandOpen {
		t.Fatal("SetCommandOpen(true) must open the palette (Task 20 reads it)")
	}
	a.SetCommandOpen(false)
	if a.commandOpen {
		t.Fatal("SetCommandOpen(false) must close the palette")
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
