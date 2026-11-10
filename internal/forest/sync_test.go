package forest

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
	"github.com/futuremeng/OpenArchitect/internal/core/store"
)

// newForest makes root/proj: a committed one-package Go module.
func newForest(t *testing.T) (root, repo string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root = t.TempDir()
	repo = filepath.Join(root, "proj")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, repo, "go.mod", "module example.com/proj\n\ngo 1.22\n")
	write(t, repo, "a.go", "package proj\n\nfunc A() int { return 1 }\n")
	git(t, repo, "init", "-q", "-b", "main")
	git(t, repo, "config", "user.name", "Alice")
	git(t, repo, "config", "user.email", "alice@example.com")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "init")
	return root, repo
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = os.Environ()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func commit(t *testing.T, dir, msg string) {
	t.Helper()
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", msg)
}

func TestSyncScanGateAndCursor(t *testing.T) {
	ctx := context.Background()
	root, repo := newForest(t)
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Round 1: discovery + first scan (no cursor), snapshot bound to commit.
	rep, err := Sync(ctx, db, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Repos) != 1 || rep.Repos[0].Action != "synced" || !rep.Repos[0].Scanned {
		t.Fatalf("round 1 = %+v", rep.Repos)
	}
	repoID := model.RepoIDFor(repo)
	head1 := rep.Repos[0].To
	sid, err := db.LatestSnapshotID(ctx, repoID)
	if err != nil || sid == "" {
		t.Fatalf("no snapshot after first sync: %q %v", sid, err)
	}
	snap, err := db.Snapshot(ctx, sid)
	if err != nil || snap.CommitHash != head1 || snap.Branch != "main" {
		t.Fatalf("snapshot commit binding = %+v (want %s/main)", snap, head1[:8])
	}

	// Round 2: README-only commit — bookkeeping without rescan (DoD 3).
	write(t, repo, "README.md", "grown\n")
	commit(t, repo, "docs: readme\n\nCo-Authored-By: Claude <noreply@anthropic.com>")
	rep, err = Sync(ctx, db, root)
	if err != nil {
		t.Fatal(err)
	}
	line := rep.Repos[0]
	if line.Action != "synced" || line.Scanned || line.Commits != 1 {
		t.Fatalf("round 2 = %+v, want synced/no-scan/1 commit", line)
	}
	fruits, err := db.Fruits(ctx, repoID, 10)
	if err != nil || len(fruits) != 1 || fruits[0].AgentTag != "mixed" {
		t.Fatalf("fruits = %+v, %v", fruits, err)
	}

	// Round 3: source change — gate opens, rescan, cursor and commit move.
	write(t, repo, "a.go", "package proj\n\nfunc A() int { return 2 }\n")
	commit(t, repo, "feat: A returns 2")
	rep, err = Sync(ctx, db, root)
	if err != nil {
		t.Fatal(err)
	}
	line = rep.Repos[0]
	if line.Action != "synced" || !line.Scanned || line.Commits != 1 {
		t.Fatalf("round 3 = %+v", line)
	}
	sid, _ = db.LatestSnapshotID(ctx, repoID)
	snap, _ = db.Snapshot(ctx, sid)
	if snap.CommitHash != line.To {
		t.Fatalf("round 3 snapshot commit = %q, want %q", snap.CommitHash, line.To)
	}

	// Round 4: nothing moved — clean, no commits, no scan.
	rep, err = Sync(ctx, db, root)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Repos[0].Action != "clean" || rep.Repos[0].Scanned {
		t.Fatalf("round 4 = %+v, want clean", rep.Repos[0])
	}

	// Repos() carries the sync state through the join.
	repos, err := db.Repos(ctx)
	if err != nil || len(repos) != 1 || repos[0].SyncState != "tracked" {
		t.Fatalf("repos = %+v, %v", repos, err)
	}
	if !strings.HasPrefix(repos[0].ID, "repo:") {
		t.Fatalf("repo id = %q", repos[0].ID)
	}
}

func TestSyncStaleWhenRepoVanishes(t *testing.T) {
	ctx := context.Background()
	root, repo := newForest(t)
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := Sync(ctx, db, root); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(repo); err != nil {
		t.Fatal(err)
	}
	if _, err := Sync(ctx, db, root); err != nil {
		t.Fatal(err)
	}
	repos, err := db.Repos(ctx)
	if err != nil || len(repos) != 1 || repos[0].SyncState != "stale" {
		t.Fatalf("repos = %+v, %v, want stale", repos, err)
	}
}