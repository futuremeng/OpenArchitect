package ui

import (
	"context"
	"errors"
	"fmt"
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

// TestReloadUnionsAllSnapshottedRepos: 默认全选 —— 两个带快照的仓都进合并
// 视图；snapshot 行取按名排序的第一个（状态行的元数据），repoID 为空（合并
// 视图没有单一归属仓）。
func TestReloadUnionsAllSnapshottedRepos(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	first := seedScan(t, a, fixtureDir("simple-multi-main"))
	second := seedScan(t, a, fixtureDir("shared-lib"))
	if err := a.Reload(ctx); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if len(a.repos) != 2 {
		t.Fatalf("repos = %d", len(a.repos))
	}
	if len(a.selectedRepos) != 2 {
		t.Fatalf("both repos must be selected by default: %v", a.selectedRepos)
	}
	// A direct Reload re-reads the database but is not a completed scan, so the
	// scan lifecycle state stays Idle (finishScan is what sets Ready). The
	// empty-database test below pins the ruling's other half with the same
	// status assertion.
	if a.repoID != "" || a.status != StatusIdle {
		t.Fatalf("repoID/status = %q/%v", a.repoID, a.status)
	}
	if got, want := len(a.graph.Elements()), first.Elements+second.Elements; got != want {
		t.Fatalf("graph elements = %d want %d", got, want)
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
	// Ruling 6's other half: Reload never writes the scan lifecycle state, in
	// the empty branch any more than in the non-empty one.
	if a.status != StatusIdle {
		t.Fatalf("Reload must not touch the scan lifecycle state: status = %v", a.status)
	}
}

// TestReloadEmptyBranchClearsSelection pins the hoisted clear: every Reload
// that completes the read phase, empty-database branch included, ends with a
// cleared selection — one writer, no asymmetry between the two branches.
// Impact mode is off, so Select never runs analyse and the stale selection is
// purely field state.
func TestReloadEmptyBranchClearsSelection(t *testing.T) {
	a := newTestApp(t)
	if err := a.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	a.Select("cmp:stale")
	if a.selected != "cmp:stale" {
		t.Fatalf("setup: selected = %q", a.selected)
	}
	if err := a.Reload(context.Background()); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if a.selected != "" || a.drillInto != "" || a.impact != nil {
		t.Fatalf("empty-database Reload must clear selection: selected=%q drillInto=%q impact=%+v",
			a.selected, a.drillInto, a.impact)
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

// TestFinishScanReloadFailureKeepsGraph pins finishScan's fourth failure
// branch (spec §10): the scan goroutine succeeded, but the Reload that
// re-reads stored rows failed, so the screen must keep the previous graph —
// StatusReady over a graph that no longer matches the database would lie.
// The database is closed out from under the app so a.Reload's config.Load
// errors for real; newTestApp's t.Cleanup discards Close's error, so the
// second close there is harmless.
func TestFinishScanReloadFailureKeepsGraph(t *testing.T) {
	a := newTestApp(t)
	seedScan(t, a, fixtureDir("shared-lib"))
	if err := a.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	before := len(a.graph.Elements())
	if before == 0 {
		t.Fatal("setup: graph is empty")
	}
	if err := a.db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	// A report that looks successful: err == nil, so only the internal
	// Reload can fail.
	a.finishScan(scanner.Report{Elements: before}, nil)
	if a.status != StatusFailed || a.err == nil {
		t.Fatalf("status/err = %v/%v, want Failed/non-nil", a.status, a.err)
	}
	if len(a.graph.Elements()) != before {
		t.Fatalf("reload failure must keep the previous graph on screen: %d want %d",
			len(a.graph.Elements()), before)
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
	// 图必须是从库里读出来的行拼的：默认全选下，两个仓的最新快照并集就是
	// 图的全部内容。
	latestA, err := a.db.LatestSnapshotID(ctx, string(first.RepoID))
	if err != nil {
		t.Fatal(err)
	}
	latestB, err := a.db.LatestSnapshotID(ctx, model.RepoIDFor(abs))
	if err != nil {
		t.Fatal(err)
	}
	storedA, err := a.db.Elements(ctx, latestA)
	if err != nil {
		t.Fatal(err)
	}
	storedB, err := a.db.Elements(ctx, latestB)
	if err != nil {
		t.Fatal(err)
	}
	if len(storedA) == 0 || len(storedB) == 0 ||
		len(a.graph.Elements()) != len(storedA)+len(storedB) ||
		!a.selectedRepos[string(first.RepoID)] || !a.selectedRepos[model.RepoIDFor(abs)] {
		t.Fatalf("graph must be the union of stored rows: graph=%d stored=%d+%d selected=%v",
			len(a.graph.Elements()), len(storedA), len(storedB), a.selectedRepos)
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
	// Spec §10 row 1's copy: the errs > 0 arm of StatusReady. Count comes from
	// the live graph so the expected string cannot drift from the fixture.
	a.snapshot.PackageErrors = []string{"p1", "p2"}
	want := fmt.Sprintf("完成：%d 个元素，%d 个包有错误", len(a.graph.Elements()), len(a.snapshot.PackageErrors))
	if got := a.StatusLine(); got != want {
		t.Fatalf("StatusLine with package errors = %q want %q", got, want)
	}
	a.status = StatusFailed
	a.err = errors.New("boom")
	if got := a.StatusLine(); !strings.Contains(got, "boom") {
		t.Fatalf("failed StatusLine = %q", got)
	}
}

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

// TestGraphReposSelectUnionAndPersists 钉住图页多选过滤器的整条链：默认
// 全选（两仓并集）→ 收成单仓 → 新 App 读同一个库继承选择 → 回到全选。
func TestGraphReposSelectUnionAndPersists(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	first := seedScan(t, a, fixtureDir("simple-multi-main"))
	second := seedScan(t, a, fixtureDir("shared-lib"))
	if err := a.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	if len(a.selectedRepos) != 2 {
		t.Fatalf("default must select every snapshotted repo: %v", a.selectedRepos)
	}
	if got, want := len(a.graph.Elements()), first.Elements+second.Elements; got != want {
		t.Fatalf("union elements = %d want %d", got, want)
	}
	if err := a.SetGraphRepos(ctx, []string{string(first.RepoID)}); err != nil {
		t.Fatalf("SetGraphRepos: %v", err)
	}
	if a.snapshot.ID != first.SnapshotID || a.repoID != string(first.RepoID) {
		t.Fatalf("single pick must switch the graph: snapshot=%q repo=%q", a.snapshot.ID, a.repoID)
	}
	if len(a.graph.Elements()) != first.Elements {
		t.Fatalf("picked graph elements = %d want %d", len(a.graph.Elements()), first.Elements)
	}
	b := NewApp(a.db)
	if err := b.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	if b.snapshot.ID != first.SnapshotID || b.repoID != string(first.RepoID) {
		t.Fatalf("pick must persist through app_config: snapshot=%q repo=%q", b.snapshot.ID, b.repoID)
	}
	if err := b.SetGraphRepos(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if len(b.selectedRepos) != 2 {
		t.Fatalf("empty pick must mean select-all: %v", b.selectedRepos)
	}
}

// TestGraphReposFallbackWithoutSnapshot: 选一个没有快照的仓不改变画面，
// 但设置保留 —— 那个仓下次扫描后自动生效。
func TestGraphReposFallbackWithoutSnapshot(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	only := seedScan(t, a, fixtureDir("simple-multi-main"))
	if err := a.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	if err := a.SetGraphRepos(ctx, []string{"repo:/never/scanned"}); err != nil {
		t.Fatal(err)
	}
	if a.snapshot.ID != only.SnapshotID || a.repoID != string(only.RepoID) {
		t.Fatalf("a snapshotless pick must fall back: snapshot=%q repo=%q", a.snapshot.ID, a.repoID)
	}
	if len(a.settings.GraphRepos) != 1 || a.settings.GraphRepos[0] != "repo:/never/scanned" {
		t.Fatalf("the pick itself must survive the fallback: %v", a.settings.GraphRepos)
	}
}

// TestMergedGraphShowsCrossRepoEdges 是把整条链钉死的最重要一条：扫描器
// 把跨模块依赖/调用存成悬挂边（切片 2 修正），两个仓都进合并视图后，这些
// 边必须在包层聚合图里接上——不是被 graph.Build 的端点存在性过滤吃掉。
func TestMergedGraphShowsCrossRepoEdges(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	seedScan(t, a, fixtureDir("crossmod-dep"))
	seedScan(t, a, fixtureDir("crossmod-app"))
	if err := a.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	if len(a.selectedRepos) != 2 {
		t.Fatalf("both repos must be in the merged view: %v", a.selectedRepos)
	}
	// 元素层：悬挂边两端都在并集里 → 原始关系里就有跨仓 calls。
	var cross bool
	for _, rel := range a.graph.Relationships() {
		if strings.HasPrefix(rel.ToID, "cmp:crossmod.example/dep/") &&
			strings.HasPrefix(rel.FromID, "cmp:crossmod.example/app/") && rel.Type == model.RelCalls {
			cross = true
		}
	}
	if !cross {
		t.Fatal("cross-repo calls edge missing from the merged union")
	}
	// 包层（多仓默认深度）：跨仓边聚合成包组之间的 calls。
	if a.Depth() != 1 {
		t.Fatalf("multi-repo default depth = %d want 1", a.Depth())
	}
	found := false
	for _, rel := range a.view.Relationships() {
		if rel.FromID == "pkg:crossmod.example/app/store" && rel.ToID == "pkg:crossmod.example/dep/store" && rel.Type == model.RelCalls {
			found = true
		}
	}
	if !found {
		t.Fatalf("package-level merged graph lost the cross-repo edge: %+v", a.view.Relationships())
	}
	// 仓库层：两个仓之间一条 calls 聚合边。
	if !a.setDepth(0) {
		t.Fatal("setDepth(0) must change the depth")
	}
	a.refreshView()
	edges := a.view.Relationships()
	if len(a.view.Elements()) != 2 || len(edges) != 1 || edges[0].Type != model.RelCalls {
		t.Fatalf("repo level = %d nodes %v", len(a.view.Elements()), edges)
	}
}

// TestZoomDepthWalksLevels: 缩放改深度不改尺寸 —— ZoomDepth 在仓库层/包层/
// 元素层之间走，节点数随深度单调不减，越界被钳住。
func TestZoomDepthWalksLevels(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	seedScan(t, a, fixtureDir("crossmod-dep"))
	seedScan(t, a, fixtureDir("crossmod-app"))
	if err := a.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	if a.LevelName() != "包层" {
		t.Fatalf("multi-repo default = %q want 包层", a.LevelName())
	}
	a.ZoomDepth(false) // → 仓库层
	if a.LevelName() != "仓库层" || len(a.view.Elements()) != 2 {
		t.Fatalf("repo level: %q %d nodes", a.LevelName(), len(a.view.Elements()))
	}
	a.ZoomDepth(false) // 钳在 0
	if a.Depth() != 0 {
		t.Fatalf("depth must clamp at 0: %d", a.Depth())
	}
	a.ZoomDepth(true)
	a.ZoomDepth(true) // → 元素层
	if a.LevelName() != "元素层" {
		t.Fatalf("depth = %q want 元素层", a.LevelName())
	}
	a.ZoomDepth(true) // 钳在 2
	if a.Depth() != 2 {
		t.Fatalf("depth must clamp at 2: %d", a.Depth())
	}
}

