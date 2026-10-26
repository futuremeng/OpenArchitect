package scanner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
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

// TestRunParseFailureWritesNothing covers spec §10 row 3's other half: a
// directory that IS recognised as a Go project (go.mod is there, so Detect says
// "go") but whose module cannot be loaded at all, so the whole parse fails.
// The brief's failure fixture is a missing path, which stops inside Detect and
// never reaches the parse-error guard, so this is the test that gives that guard
// its bite: were Run to write anyway, the repos row would land here.
func TestRunParseFailureWritesNothing(t *testing.T) {
	db := newTestDB(t)
	root := filepath.Join(t.TempDir(), "broken-module")
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"go.mod":     "this is not a go.mod file\n",
		"sub/sub.go": "package sub\n\ntype Thing struct{ N int }\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Run(context.Background(), db, root); err == nil {
		t.Fatal("expected the module load to fail outright")
	}
	assertEmpty(t, db)
}

// TestRunAccumulatesPackageErrors covers spec §10 row 1 and §11's "包级错误累积
// 测试": a package that does not type-check must be reported, not fatal, and the
// packages that did resolve have to reach the database anyway.
func TestRunAccumulatesPackageErrors(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	report, err := Run(ctx, db, filepath.Join(fixtureRoot(), "one-broken-package"))
	if err != nil {
		t.Fatalf("Run: %v (a per-package error must not abort the scan)", err)
	}
	if len(report.PackageErrors) == 0 {
		t.Fatal("package errors must accumulate")
	}
	broken := "example.com/broken/bad"
	var namesBroken bool
	for _, e := range report.PackageErrors {
		if strings.Contains(e, broken) {
			namesBroken = true
		}
	}
	if !namesBroken {
		t.Fatalf("PackageErrors = %v, want one naming %q", report.PackageErrors, broken)
	}
	// 成功解析的包照常入库——this is what §10 promises, so it is what is asserted.
	// The broken package's own element count is deliberately not pinned: partial
	// type info for an IllTyped package is toolchain-dependent.
	elements, err := db.Elements(ctx, report.SnapshotID)
	if err != nil {
		t.Fatalf("Elements: %v", err)
	}
	const healthy = "cmp:example.com/broken/good.Widget"
	if !slices.Contains(ids(elements), healthy) {
		t.Fatalf("healthy package element %q missing from %v", healthy, ids(elements))
	}
	// The accumulated errors are stored with the snapshot (spec §10: 随快照存
	// package_errors_json), which is the only way Task 16's status line can say
	// "完成，N 个包有错误".
	stored, err := db.Snapshot(ctx, report.SnapshotID)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(stored.PackageErrors) != len(report.PackageErrors) {
		t.Fatalf("stored package errors = %v, want %v", stored.PackageErrors, report.PackageErrors)
	}
	if !slices.Contains(stored.PackageErrors, report.PackageErrors[0]) {
		t.Fatalf("stored package errors = %v, want to contain %q",
			stored.PackageErrors, report.PackageErrors[0])
	}
}

// ids lists element ids sorted, so a failure prints a stable comparison.
func ids(elements []model.Element) []string {
	out := make([]string, 0, len(elements))
	for _, e := range elements {
		out = append(out, e.ID)
	}
	slices.Sort(out)
	return out
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
