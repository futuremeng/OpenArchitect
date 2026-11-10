// Package forest is the observation trunk's discovery and check-in half
// (growth spec 切片 A/D): it finds git repos under a scan root, keeps the
// repo_sync bookkeeping fresh, and drives the fetch→gate→scan pipeline.
// Per I2′ its automation is allowed to call exactly two things: git fetch
// and scanner.Run. It imports no UI stack and no HTTP client (source
// implementations excepted, 切片 D).
package forest

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
	"github.com/futuremeng/OpenArchitect/internal/core/store"
	"github.com/futuremeng/OpenArchitect/internal/forest/gitcmd"
	"github.com/futuremeng/OpenArchitect/internal/scanner"
)

// ErrUnreadableRoot marks a scan root that cannot be walked at all.
var ErrUnreadableRoot = errors.New("forest: cannot read scan root")

// Discover walks root (max depth 3, hidden dirs skipped, no descent into a
// found repo) and registers every git repository it meets. Repos already in
// the database under root but missing this pass flip to "stale"; everything
// else keeps its state and cursor.
func Discover(ctx context.Context, db *store.DB, root string) ([]model.Repo, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if info, err := os.Stat(abs); err != nil || !info.IsDir() {
		return nil, ErrUnreadableRoot
	}
	var found []model.Repo
	foundPaths := map[string]bool{}
	walkErr := filepath.WalkDir(abs, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // one unreadable corner must not sink the forest
		}
		if !d.IsDir() {
			return nil
		}
		if path != abs && strings.HasPrefix(d.Name(), ".") {
			return fs.SkipDir
		}
		rel, rerr := filepath.Rel(abs, path)
		if rerr == nil && rel != "." && strings.Count(rel, string(filepath.Separator)) >= 3 {
			return fs.SkipDir
		}
		if !gitcmd.IsRepo(ctx, path) {
			return nil
		}
		if gitcmd.IsLinkedWorktree(path) {
			// Orca/paperclip 的 worktree 不是新树，是同一棵树的另一条枝：
			// 观察落在主检出上，worktree 不登记、不记账（定位文档第 3 条）。
			return fs.SkipDir
		}
		repo, rerr := register(ctx, db, path)
		if rerr != nil {
			return nil
		}
		found = append(found, repo)
		foundPaths[path] = true
		return fs.SkipDir // a repo's own subdirs are its business, not new trees
	})
	if walkErr != nil {
		return nil, walkErr
	}
	if err := markStale(ctx, db, abs, foundPaths); err != nil {
		return nil, err
	}
	return found, nil
}

// register upserts one repo and its sync row, preserving an existing
// state/cursor when the repo is already known.
func register(ctx context.Context, db *store.DB, path string) (model.Repo, error) {
	lang, _ := scanner.Detect(path) // unsupported language is still a tree: lang ""
	repo := model.Repo{ID: model.RepoIDFor(path), Name: filepath.Base(path),
		LocalPath: path, Lang: lang}
	if err := db.UpsertRepo(ctx, repo); err != nil {
		return model.Repo{}, err
	}
	_, state, lastHead, err := db.RepoSync(ctx, repo.ID)
	if err != nil {
		return model.Repo{}, err
	}
	if state == "" || state == "stale" {
		state = "new"
	}
	remote := gitcmd.RemoteURL(ctx, path)
	if err := db.UpsertRepoSync(ctx, repo.ID, remote, state, lastHead); err != nil {
		return model.Repo{}, err
	}
	repo.RemoteURL, repo.SyncState = remote, state
	return repo, nil
}

// markStale flips tracked repos under root that vanished this pass. Repos
// outside root (registered by hand earlier) are left alone.
func markStale(ctx context.Context, db *store.DB, root string, found map[string]bool) error {
	repos, err := db.Repos(ctx)
	if err != nil {
		return err
	}
	for _, r := range repos {
		if found[r.LocalPath] || r.SyncState == "stale" {
			continue
		}
		if rel, err := filepath.Rel(root, r.LocalPath); err != nil ||
			strings.HasPrefix(rel, "..") || rel == "." {
			continue // not under this root
		}
		_, state, lastHead, err := db.RepoSync(ctx, r.ID)
		if err != nil || state == "new" && lastHead == "" {
			continue // never synced: absence proves nothing yet
		}
		if err := db.UpsertRepoSync(ctx, r.ID, r.RemoteURL, "stale", lastHead); err != nil {
			return err
		}
	}
	return nil
}