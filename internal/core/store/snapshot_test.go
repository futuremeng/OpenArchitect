package store

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

func repoFixture() model.Repo {
	return model.Repo{ID: model.RepoIDFor("/repo/带'引号;和分号"), Name: "x", LocalPath: "/repo/带'引号;和分号", Lang: "go"}
}

func snapshotFixture(repo model.Repo) model.ArchitectureSnapshot {
	return model.ArchitectureSnapshot{
		ID: "s0000000000000000001", RepoID: repo.ID,
		PackageErrors: []string{"pkga: undefined: Foo"},
	}
}

func elementsFixture(repo model.Repo) []model.Element {
	return []model.Element{
		{ID: model.SystemID("example.com/x"), Type: model.TypeSystem, Name: "x", RepoID: repo.ID},
		{ID: "cmp:example.com/x/internal/basket.Basket", Type: model.TypeComponent, Name: "Basket",
			Description: "Basket holds items.", RepoID: repo.ID, FilePath: "internal/basket/basket.go",
			Methods: []string{"Add", "Total"}},
	}
}

func relsFixture() []model.Relationship {
	return []model.Relationship{
		{FromID: model.SystemID("example.com/x"), ToID: "cmp:example.com/x/internal/basket.Basket", Type: model.RelContains, Weight: 1},
		{FromID: "cmp:example.com/x/internal/basket.Basket", ToID: model.SystemID("example.com/x"), Type: model.RelCalls, Weight: 3},
	}
}

func TestSaveSnapshotRoundTrip(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	ctx := context.Background()
	repo := repoFixture()
	if err := db.UpsertRepo(ctx, repo); err != nil {
		t.Fatal(err)
	}
	err := db.SaveSnapshot(ctx, snapshotFixture(repo), elementsFixture(repo), relsFixture())
	if err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	latest, err := db.LatestSnapshotID(ctx, repo.ID)
	if err != nil || latest != snapshotFixture(repo).ID {
		t.Fatalf("LatestSnapshotID = %q, %v", latest, err)
	}
	// A repo with no snapshot is an empty state, not an error (Task 16's Reload
	// loops over every repo and must not fail on the ones never scanned).
	empty, err := db.LatestSnapshotID(ctx, model.RepoIDFor("/tmp/never-scanned"))
	if err != nil || empty != "" {
		t.Fatalf("LatestSnapshotID(no repo) = %q, %v", empty, err)
	}

	elements, err := db.Elements(ctx, latest)
	if err != nil {
		t.Fatal(err)
	}
	if len(elements) != 2 {
		t.Fatalf("elements = %d", len(elements))
	}
	if elements[0].ID != "cmp:example.com/x/internal/basket.Basket" {
		t.Fatalf("Elements must come back sorted by element_id, got %q first", elements[0].ID)
	}
	if got := elements[0].Description; got != "Basket holds items." {
		t.Fatalf("description = %q", got)
	}
	if len(elements[0].Methods) != 2 || elements[0].Methods[1] != "Total" {
		t.Fatalf("methods = %v", elements[0].Methods)
	}

	rels, err := db.Relationships(ctx, latest)
	if err != nil {
		t.Fatal(err)
	}
	if len(rels) != 2 {
		t.Fatalf("relationships = %d", len(rels))
	}
	// `Relationships` 的 SQL 是 ORDER BY from_id, to_id, relationship_type：
	// calls 的 FromID 以 cmp: 开头排在前面，contains 的 FromID 是 sys: 排后面。
	if rels[0].Type != model.RelCalls || rels[0].Weight != 3 {
		t.Fatalf("rels[0] = %+v", rels[0])
	}
	if rels[1].Type != model.RelContains {
		t.Fatalf("rels[1] = %+v", rels[1])
	}

	one, err := db.GetElement(ctx, latest, "cmp:example.com/x/internal/basket.Basket")
	if err != nil {
		t.Fatalf("GetElement: %v", err)
	}
	if one.FilePath != "internal/basket/basket.go" {
		t.Fatalf("file path = %q", one.FilePath)
	}
}

// TestSnapshotRowRoundTrip covers the read path Task 16's status line uses:
// the package-error list must survive the JSON column.
func TestSnapshotRowRoundTrip(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	ctx := context.Background()
	repo := repoFixture()
	if err := db.UpsertRepo(ctx, repo); err != nil {
		t.Fatal(err)
	}
	want := snapshotFixture(repo)
	if err := db.SaveSnapshot(ctx, want, elementsFixture(repo), relsFixture()); err != nil {
		t.Fatal(err)
	}
	got, err := db.Snapshot(ctx, want.ID)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if got.RepoID != repo.ID || len(got.PackageErrors) != 1 ||
		got.PackageErrors[0] != "pkga: undefined: Foo" {
		t.Fatalf("got %+v", got)
	}
	if got.CreatedAt == "" {
		t.Fatal("created_at must come back from the column default")
	}
	if _, err := db.Snapshot(ctx, "s0000000000000009999"); err == nil {
		t.Fatal("Snapshot of a missing id must fail")
	}
}

func TestSaveSnapshotRollsBackCompletely(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	ctx := context.Background()
	repo := repoFixture()

	// 让快照行违反 `architecture_snapshots.repo_id` 的外键：`SaveSnapshot`
	// 是单事务写三类行，第 1 条语句失败就整批回滚，所以三张表都必须还是空的。
	// 不用测试专用钩子注入失败语句——那会把测试 seam 长进生产 API 里。
	bad := snapshotFixture(repo)
	bad.RepoID = "repo:/not-inserted"
	err := db.SaveSnapshot(ctx, bad, elementsFixture(repo), relsFixture())
	if err == nil {
		t.Fatal("expected foreign key failure")
	}
	for _, table := range []string{"architecture_snapshots", "arch_elements", "arch_relationships"} {
		rows, qerr := db.Query(ctx, `SELECT count(*) FROM `+table)
		if qerr != nil {
			t.Fatalf("%s: %v", table, qerr)
		}
		if got := num(rows.Values[0][0]); got != 0 {
			t.Fatalf("%s survived rollback: %v rows", table, got)
		}
	}
}

// TestSaveSnapshotRollsBackAfterChildRows 才是 spec §7.2 W2「全成或全滚，
// 不留半成品快照」的正题：失败语句被安排在孩子行已写进去之后——两条
// (from_id, to_id, relationship_type) 完全相同的关系在 `arch_relationships`
// 的复合主键上冲突。语句序号：1 快照父行、2~3 元素子行、4~5 关系子行均
// 成功，第 6 条才失败。因此「遇到错误就停但不回滚」的实现会留下 5 行
// 半成品，本测试必红；而上面的外键测试第 1 条即失败，查不出这种实现。
func TestSaveSnapshotRollsBackAfterChildRows(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	ctx := context.Background()
	repo := repoFixture()
	if err := db.UpsertRepo(ctx, repo); err != nil {
		t.Fatal(err)
	}
	dup := model.Relationship{
		FromID: "cmp:example.com/x/internal/basket.Basket",
		ToID:   model.SystemID("example.com/x"), Type: model.RelCalls, Weight: 5}
	rels := append(relsFixture(), dup) // relsFixture 的第二条与 dup 主键相同
	err := db.SaveSnapshot(ctx, snapshotFixture(repo), elementsFixture(repo), rels)
	if err == nil {
		t.Fatal("expected composite primary key violation on the duplicate relationship")
	}
	for _, table := range []string{"architecture_snapshots", "arch_elements", "arch_relationships"} {
		rows, qerr := db.Query(ctx, `SELECT count(*) FROM `+table)
		if qerr != nil {
			t.Fatalf("%s: %v", table, qerr)
		}
		if got := num(rows.Values[0][0]); got != 0 {
			t.Fatalf("%s survived mid-transaction rollback: %v rows", table, got)
		}
	}
}

func TestUpsertRepoIsIdempotentAndUpdatesName(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	ctx := context.Background()
	repo := repoFixture()
	repo.Name = "first"
	if err := db.UpsertRepo(ctx, repo); err != nil {
		t.Fatal(err)
	}
	repo.Name = "second"
	if err := db.UpsertRepo(ctx, repo); err != nil {
		t.Fatal(err)
	}
	rows, err := db.Query(ctx, `SELECT name FROM repos`)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows.Values) != 1 || str(rows.Values[0][0]) != "second" {
		t.Fatalf("rows = %v", rows.Values)
	}
	repos, err := db.Repos(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 || repos[0].ID != repo.ID || repos[0].Name != "second" ||
		repos[0].LocalPath != repo.LocalPath || repos[0].Lang != "go" {
		t.Fatalf("Repos = %+v", repos)
	}
}

func TestSearchElementsEscapesWildcards(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	ctx := context.Background()
	repo := repoFixture()
	if err := db.UpsertRepo(ctx, repo); err != nil {
		t.Fatal(err)
	}
	// Bask9x 与 10000 是「变异检验锚点」：它们只有在不转义时才会被通配符吃掉。
	// `%Bask_x%`（`_` 当通配）命中 Bask9x，`%100%%`（`%` 当通配）命中 10000；
	// 转义后二者都落空。若 `escapeLike` 退化成恒等函数，下面两个断言必红。
	elements := []model.Element{
		{ID: "cmp:a.Basket", Type: model.TypeComponent, Name: "Basket", RepoID: repo.ID},
		{ID: "cmp:a.Bask_x", Type: model.TypeComponent, Name: "Bask_x", RepoID: repo.ID},
		{ID: "cmp:a.Bask9x", Type: model.TypeComponent, Name: "Bask9x", RepoID: repo.ID},
		{ID: "cmp:a.Percent", Type: model.TypeComponent, Name: "100%", RepoID: repo.ID},
		{ID: "cmp:a.Thousand", Type: model.TypeComponent, Name: "10000", RepoID: repo.ID},
	}
	if err := db.SaveSnapshot(ctx, snapshotFixture(repo), elements, nil); err != nil {
		t.Fatal(err)
	}
	// "_" 是 LIKE 单字符通配：不转义的话 "Bask_x" 会连 Bask9x 一起命中。
	got, err := db.SearchElements(ctx, snapshotFixture(repo).ID, "Bask_x")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "Bask_x" {
		t.Fatalf("underscore leaked: %+v", got)
	}
	// "%" 是 LIKE 多字符通配：不转义的话 "100%" 会连 10000 一起命中。
	got, err = db.SearchElements(ctx, snapshotFixture(repo).ID, "100%")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "100%" {
		t.Fatalf("percent literal: %+v", got)
	}
}

// maxSearchResults 走 `LIMIT ?`：60 个同名候选必须截到上限条数。
func TestSearchElementsHonoursLimit(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	ctx := context.Background()
	repo := repoFixture()
	if err := db.UpsertRepo(ctx, repo); err != nil {
		t.Fatal(err)
	}
	elements := make([]model.Element, 0, 60)
	for i := 0; i < 60; i++ {
		elements = append(elements, model.Element{
			ID: fmt.Sprintf("cmp:a.T%02d", i), Type: model.TypeComponent,
			Name: "Target", RepoID: repo.ID})
	}
	if err := db.SaveSnapshot(ctx, snapshotFixture(repo), elements, nil); err != nil {
		t.Fatal(err)
	}
	got, err := db.SearchElements(ctx, snapshotFixture(repo).ID, "Target")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != maxSearchResults {
		t.Fatalf("search results = %d, want cap %d", len(got), maxSearchResults)
	}
}

func TestSnapshotForElementPicksLatest(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	ctx := context.Background()
	repo := repoFixture()
	if err := db.UpsertRepo(ctx, repo); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"s0000000000000000001", "s0000000000000000002"} {
		snap := snapshotFixture(repo)
		snap.ID = id
		if err := db.SaveSnapshot(ctx, snap, elementsFixture(repo), nil); err != nil {
			t.Fatalf("%s: %v", id, err)
		}
	}
	got, err := db.SnapshotForElement(ctx, "cmp:example.com/x/internal/basket.Basket")
	if err != nil {
		t.Fatal(err)
	}
	if got != "s0000000000000000002" {
		t.Fatalf("got %q", got)
	}
	if _, err := db.GetElement(ctx, got, "cmp:example.com/x/internal/basket.Basket"); err != nil {
		t.Fatalf("unknown element: %v", err)
	}
	// Task 18 的抽屉靠哨兵区分「不在快照里」和「读库失败」。
	if _, err := db.SnapshotForElement(ctx, "cmp:nowhere.Nope"); !errors.Is(err, model.ErrUnknownElement) {
		t.Fatalf("SnapshotForElement(unknown) = %v, want model.ErrUnknownElement", err)
	}
	if _, err := db.GetElement(ctx, got, "cmp:nowhere.Nope"); !errors.Is(err, model.ErrUnknownElement) {
		t.Fatalf("GetElement(unknown) = %v, want model.ErrUnknownElement", err)
	}
}

func TestConfigGetSet(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	ctx := context.Background()
	if got, err := db.ConfigGet(ctx, "missing"); got != "" || err != nil {
		t.Fatalf("missing key = %q, %v", got, err)
	}
	if err := db.ConfigSet(ctx, "impact.depth", "2"); err != nil {
		t.Fatal(err)
	}
	if err := db.ConfigSet(ctx, "impact.depth", "3"); err != nil {
		t.Fatal(err)
	}
	if got, err := db.ConfigGet(ctx, "impact.depth"); got != "3" || err != nil {
		t.Fatalf("got %q, %v", got, err)
	}
}
