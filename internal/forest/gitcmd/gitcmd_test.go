package gitcmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// newRepo makes a throwaway git repo with one commit by Alice.
func newRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	git(t, dir, "config", "user.name", "Alice")
	git(t, dir, "config", "user.email", "alice@example.com")
	write(t, dir, "a.go", "package a\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "first")
	return dir
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_COMMITTER_DATE=2026-01-01T00:00:00+08:00",
		"GIT_AUTHOR_DATE=2026-01-01T00:00:00+08:00")
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("git %v: %v\n%s", args, err, ee.Stderr)
		}
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}

func commitFile(t *testing.T, dir, name, content, msg string) {
	t.Helper()
	write(t, dir, name, content)
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", msg)
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

func TestIsRepoAndHead(t *testing.T) {
	dir := newRepo(t)
	if !IsRepo(context.Background(), dir) {
		t.Fatal("IsRepo false on a fresh repo")
	}
	if IsRepo(context.Background(), t.TempDir()) {
		t.Fatal("IsRepo true on a plain dir")
	}
	hash, branch, err := Head(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(hash) != 40 || branch != "main" {
		t.Fatalf("Head = %q/%q", hash, branch)
	}
}

// A directory merely *inside* a repo is not one: without the top-level
// compare, a scan root living in someone else's tree registered itself as a
// phantom repo carrying the outer repo's identity.
func TestIsRepoFalseInsideOuterRepo(t *testing.T) {
	ctx := context.Background()
	outer := newRepo(t)
	sub := filepath.Join(outer, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if IsRepo(ctx, sub) {
		t.Fatal("IsRepo true on a subdir of an outer repo")
	}
	inner := filepath.Join(sub, "inner")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, inner, "init", "-q", "-b", "main")
	if !IsRepo(ctx, inner) {
		t.Fatal("IsRepo false on a nested repo")
	}
}

// A bare repo has no work tree; the flag is the only way to know.
func TestIsRepoTrueForBareRepo(t *testing.T) {
	dir := t.TempDir()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	git(t, dir, "init", "-q", "--bare")
	if !IsRepo(context.Background(), dir) {
		t.Fatal("IsRepo false on a bare repo")
	}
}

func TestRemoteURLEmptyWithoutOrigin(t *testing.T) {
	if got := RemoteURL(context.Background(), newRepo(t)); got != "" {
		t.Fatalf("RemoteURL = %q, want empty", got)
	}
}

func TestLogWithRangeAndFiles(t *testing.T) {
	ctx := context.Background()
	dir := newRepo(t)
	first, _, _ := Head(ctx, dir)
	commitFile(t, dir, "b.go", "package b\n", "add b")
	commitFile(t, dir, "c.go", "package c\n", "add c")
	head, _, _ := Head(ctx, dir)

	recs, err := Log(ctx, dir, first, head, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 {
		t.Fatalf("Log range = %d records, want 2", len(recs))
	}
	if recs[0].Message != "add c" || len(recs[0].Files) != 1 || recs[0].Files[0] != "c.go" {
		t.Fatalf("newest record = %+v", recs[0])
	}
	if recs[0].Author != "Alice" || recs[0].AuthorEmail != "alice@example.com" {
		t.Fatalf("author fields = %+v", recs[0])
	}
}

func TestLogMessageCannotForgeBoundaries(t *testing.T) {
	ctx := context.Background()
	dir := newRepo(t)
	// git itself refuses NUL in commit messages ("a NUL byte ... not
	// allowed"), so fieldSep is unforgeable at the source. The remaining
	// risk is a recordSep (\x1e) smuggled into a body: the forged chunk
	// carries no valid hash and the hash check drops it.
	msg := filepath.Join(t.TempDir(), "msg")
	if err := os.WriteFile(msg, []byte("evil\n\nCo-authored-by: X\n\x1efake record\x01fake.go\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "commit", "-q", "--allow-empty", "-F", msg)
	recs, err := Log(ctx, dir, "", "HEAD", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 {
		t.Fatalf("Log = %d records, want 2 (boundary in message must not split)", len(recs))
	}
}

func TestDiffFiles(t *testing.T) {
	ctx := context.Background()
	dir := newRepo(t)
	first, _, _ := Head(ctx, dir)
	commitFile(t, dir, "README.md", "hi\n", "docs only")
	head, _, _ := Head(ctx, dir)
	files := DiffFiles(ctx, dir, first, head)
	if len(files) != 1 || files[0] != "README.md" {
		t.Fatalf("DiffFiles = %v, want [README.md]", files)
	}
	if got := DiffFiles(ctx, dir, "", head); got != nil {
		t.Fatalf("DiffFiles without cursor = %v, want nil", got)
	}
	if got := DiffFiles(ctx, dir, "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef", head); len(got) != 1 || got[0] != "?" {
		t.Fatalf("DiffFiles with bad cursor = %v, want [?] (rescan is the safe answer)", got)
	}
}