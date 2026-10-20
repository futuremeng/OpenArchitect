package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
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

// TestRunGraphNoSnapshot pins runGraph's live empty-snapshot branch (controller
// ruling R1): LatestSnapshotID returns ("", nil) for a repo that was never
// scanned, so the error must name the repo id, say "no snapshot", and carry no
// inner "oac:" prefix—main already prints "oac: %v", so an inner one would show
// up doubled to the user.
func TestRunGraphNoSnapshot(t *testing.T) {
	db, _, _ := scanFixture(t, "shared-lib")
	const repoID = "repo:/never/scanned"
	var buf bytes.Buffer
	err := runGraph(context.Background(), db, repoID, &buf)
	if err == nil {
		t.Fatalf("want no-snapshot error, got output:\n%s", buf.String())
	}
	if !strings.Contains(err.Error(), repoID) {
		t.Fatalf("error must name the repo id: %v", err)
	}
	if !strings.Contains(err.Error(), "no snapshot") {
		t.Fatalf("error must say no snapshot: %v", err)
	}
	if strings.Contains(err.Error(), "oac:") {
		t.Fatalf("error carries an inner oac: prefix, main adds one too: %v", err)
	}
}

// TestDefaultDBPathShape pins the hinge between the CLI and the GUI (controller
// ruling R2): mygo's PathUserData is os.UserConfigDir()+App.Name()
// (paths.go:91-92), and App.Name() is the bundle name only when packaged.
// The exact directory and file names are what both faces must agree on.
func TestDefaultDBPathShape(t *testing.T) {
	path, err := defaultDBPath()
	if err != nil {
		t.Fatal(err)
	}
	if base := filepath.Base(path); base != "openarchitect.db" {
		t.Fatalf("file = %q, want openarchitect.db", base)
	}
	if dir := filepath.Base(filepath.Dir(path)); dir != "OpenArchitect" {
		t.Fatalf("directory = %q, want OpenArchitect", dir)
	}
}

// TestRunAcceptsFlagsAfterPositional pins the usage strings in usage()
// ("oac scan <path> [--db PATH]", "oac impact <id> [--depth N] [--db PATH]"):
// Go's flag package stops at the first positional argument, so run() hoists
// flags ahead of positionals instead of rejecting the documented order.
func TestRunAcceptsFlagsAfterPositional(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "run.db")
	root := filepath.Join(fixturesRoot(), "shared-lib")

	out := captureStdout(t, func() error {
		return run(ctx, []string{"scan", root, "--db", dbPath})
	})
	if !strings.Contains(out, "snapshot s") {
		t.Fatalf("scan through run():\n%s", out)
	}

	out = captureStdout(t, func() error {
		return run(ctx, []string{"impact", "cmp:example.com/shared/core.Server",
			"--depth", "1", "--db", dbPath})
	})
	if !strings.Contains(out, "Upstream") || !strings.Contains(out, "depth 1") {
		t.Fatalf("impact through run():\n%s", out)
	}
}

func captureStdout(t *testing.T, fn func() error) string {
	t.Helper()
	old, r, w, err := stdoutToPipe()
	if err != nil {
		t.Fatal(err)
	}
	runErr := fn()
	_ = w.Close()
	os.Stdout = old
	out, readErr := io.ReadAll(r)
	if runErr != nil {
		t.Fatalf("run: %v (stdout:\n%s)", runErr, out)
	}
	if readErr != nil {
		t.Fatal(readErr)
	}
	return string(out)
}

func stdoutToPipe() (*os.File, *os.File, *os.File, error) {
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		return old, nil, nil, err
	}
	os.Stdout = w
	return old, r, w, nil
}

func mustAbs(t *testing.T, p string) string {
	t.Helper()
	abs, err := filepath.Abs(p)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}
