package store

import (
	"context"
	"testing"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

func TestGrowthEventsRoundTrip(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	ctx := context.Background()
	if _, err := db.Execute(ctx,
		`INSERT INTO repos (id, name, local_path, lang) VALUES ('repo:/g', 'g', '/g', 'go')`); err != nil {
		t.Fatal(err)
	}
	events := []model.GrowthEvent{
		{RepoID: "repo:/g", SnapshotFrom: "s1", SnapshotTo: "s2", ElementID: "cmp:a.A", Kind: model.GrowthAdded, CommitHash: "h1"},
		{RepoID: "repo:/g", SnapshotFrom: "s1", SnapshotTo: "s2", ElementID: "cmp:a.B", Kind: model.GrowthEdgeChanged},
	}
	if err := db.SaveGrowthEvents(ctx, events); err != nil {
		t.Fatal(err)
	}
	// Idempotent replay: same diff round twice stores it once.
	if err := db.SaveGrowthEvents(ctx, events); err != nil {
		t.Fatal(err)
	}
	all, err := db.GrowthEvents(ctx, "repo:/g", "", 0)
	if err != nil || len(all) != 2 {
		t.Fatalf("all = %+v, %v", all, err)
	}
	if all[0].ElementID != "cmp:a.B" || all[0].CommitHash != "" { // rowid DESC: newest written first
		t.Fatalf("newest-first violated: %+v", all)
	}
	one, err := db.GrowthEvents(ctx, "repo:/g", "cmp:a.A", 0)
	if err != nil || len(one) != 1 || one[0].Kind != model.GrowthAdded || one[0].CommitHash != "h1" {
		t.Fatalf("filtered = %+v, %v", one, err)
	}
	// Another repo sees nothing.
	none, err := db.GrowthEvents(ctx, "repo:/other", "", 0)
	if err != nil || len(none) != 0 {
		t.Fatalf("other repo = %+v, %v", none, err)
	}
}

func TestGrowthEventsCascadeWithRepo(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	ctx := context.Background()
	if err := db.SaveGrowthEvents(ctx, []model.GrowthEvent{
		{RepoID: "repo:/c", SnapshotFrom: "s1", SnapshotTo: "s2", ElementID: "cmp:a.A", Kind: model.GrowthRemoved}}); err == nil {
		t.Fatal("events for an unknown repo must not land (FK)")
	}
	if _, err := db.Execute(ctx,
		`INSERT INTO repos (id, name, local_path, lang) VALUES ('repo:/c', 'c', '/c', 'go')`); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveGrowthEvents(ctx, []model.GrowthEvent{
		{RepoID: "repo:/c", SnapshotFrom: "s1", SnapshotTo: "s2", ElementID: "cmp:a.A", Kind: model.GrowthRemoved}}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Execute(ctx, `DELETE FROM repos WHERE id = 'repo:/c'`); err != nil {
		t.Fatal(err)
	}
	rows, err := db.Query(ctx, `SELECT count(*) FROM growth_events`)
	if err != nil {
		t.Fatal(err)
	}
	if got := num(rows.Values[0][0]); got != 0 {
		t.Fatalf("growth_events survived repo cascade: count = %v", got)
	}
}