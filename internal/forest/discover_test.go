package forest

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/futuremeng/OpenArchitect/internal/core/store"
)

// Orca 的标准布局是主检出旁边的兄弟 worktree。worktree 与主检出共享对象库：
// 登记成第二棵树会把同一批 commit 记两遍、果实数翻倍。发现层必须认得
// 「.git 是文件」这一族（linked worktree / submodule），整树跳过。
func TestDiscoverSkipsLinkedWorktrees(t *testing.T) {
	root, repo := newForest(t)
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	git(t, repo, "worktree", "add", "-q", filepath.Join(root, "proj-wt"), "-b", "wt-branch")

	repos, err := Discover(ctx, db, root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(repos) != 1 || repos[0].LocalPath != repo {
		t.Fatalf("repos = %v, want only the main checkout", repos)
	}
	stored, err := db.Repos(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 {
		t.Fatalf("stored repos = %d, want 1", len(stored))
	}
}