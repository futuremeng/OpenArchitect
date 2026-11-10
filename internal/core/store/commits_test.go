package store

import (
	"context"
	"testing"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

func TestCommitsAndFruitsRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	defer db.Close()
	if _, err := db.Execute(ctx,
		`INSERT INTO repos (id, name, local_path, lang) VALUES ('repo:/c', 'c', '/c', 'go')`); err != nil {
		t.Fatal(err)
	}
	err := db.SaveCommits(ctx, []model.Commit{
		{RepoID: "repo:/c", Hash: "a1", Author: "Alice", Date: "2026-01-02T00:00:00+08:00",
			Message: "docs", AgentTag: "mixed", Files: []string{"README.md"}},
		{RepoID: "repo:/c", Hash: "a2", Author: "Alice", Date: "2026-01-01T00:00:00+08:00",
			Message: "human only", AgentTag: "human", Files: []string{"a.go"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	all, err := db.Commits(ctx, "repo:/c", 0)
	if err != nil || len(all) != 2 {
		t.Fatalf("Commits = %d, %v", len(all), err)
	}
	if all[0].Hash != "a1" || all[0].Files[0] != "README.md" { // date DESC
		t.Fatalf("newest first violated: %+v", all[0])
	}
	fruits, err := db.Fruits(ctx, "repo:/c", 0)
	if err != nil || len(fruits) != 1 || fruits[0].Hash != "a1" || fruits[0].AgentTag != "mixed" {
		t.Fatalf("Fruits = %+v, %v", fruits, err)
	}
	// Upsert on the same hash updates instead of duplicating.
	if err := db.SaveCommits(ctx, []model.Commit{
		{RepoID: "repo:/c", Hash: "a1", AgentTag: "agent:claude"}}); err != nil {
		t.Fatal(err)
	}
	fruits, _ = db.Fruits(ctx, "repo:/c", 0)
	if len(fruits) != 1 || fruits[0].AgentTag != "agent:claude" {
		t.Fatalf("upsert did not update: %+v", fruits)
	}
}

func TestRepoSyncAndSnapshotCommit(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	defer db.Close()
	repo := model.Repo{ID: "repo:/s", Name: "s", LocalPath: "/s", Lang: "go"}
	if err := db.UpsertRepo(ctx, repo); err != nil {
		t.Fatal(err)
	}
	// Unregistered sync row reads as "new", not an error.
	if _, state, head, err := db.RepoSync(ctx, repo.ID); err != nil || state != "new" || head != "" {
		t.Fatalf("fresh RepoSync = %q/%q, %v", state, head, err)
	}
	if err := db.UpsertRepoSync(ctx, repo.ID, "git@example.com:s.git", "tracked", "abc"); err != nil {
		t.Fatal(err)
	}
	repos, err := db.Repos(ctx)
	if err != nil || len(repos) != 1 {
		t.Fatalf("Repos = %+v, %v", repos, err)
	}
	if repos[0].RemoteURL != "git@example.com:s.git" || repos[0].SyncState != "tracked" {
		t.Fatalf("join fields = %+v", repos[0])
	}
	// UpsertRepo (e.g. a rescan) must not clobber the sync side table.
	if err := db.UpsertRepo(ctx, repo); err != nil {
		t.Fatal(err)
	}
	if _, _, head, err := db.RepoSync(ctx, repo.ID); err != nil || head != "abc" {
		t.Fatalf("UpsertRepo clobbered cursor: %q, %v", head, err)
	}
	// SetSnapshotCommit backfills the reserved columns.
	s := model.ArchitectureSnapshot{ID: "s1", RepoID: repo.ID}
	if err := db.SaveSnapshot(ctx, s, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := db.SetSnapshotCommit(ctx, "s1", "deadbeef", "main"); err != nil {
		t.Fatal(err)
	}
	got, err := db.Snapshot(ctx, "s1")
	if err != nil || got.CommitHash != "deadbeef" || got.Branch != "main" {
		t.Fatalf("snapshot = %+v, %v", got, err)
	}
}

func TestCommitsCascadeWithRepo(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	defer db.Close()
	if _, err := db.Execute(ctx,
		`INSERT INTO repos (id, name, local_path, lang) VALUES ('repo:/k', 'k', '/k', 'go')`); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveCommits(ctx, []model.Commit{{RepoID: "repo:/k", Hash: "h1"}}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertRepoSync(ctx, "repo:/k", "", "tracked", "h1"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Execute(ctx, `DELETE FROM repos WHERE id = 'repo:/k'`); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"commits", "repo_sync"} {
		rows, err := db.Query(ctx, `SELECT count(*) FROM `+table)
		if err != nil {
			t.Fatalf("%s: %v", table, err)
		}
		if got := num(rows.Values[0][0]); got != 0 {
			t.Fatalf("%s survived cascade: count = %v", table, got)
		}
	}
}
