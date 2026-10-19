package store

import (
	"context"
	"fmt"
	"testing"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

// BenchmarkSaveSnapshot is PoC-5's re-runnable measurement: a spec §6.4 sized
// snapshot is 600 components + 1 System + containers ≈ 620 elements and about
// 1500 relationships, i.e. ~2100 statements in one transaction.
func BenchmarkSaveSnapshot(b *testing.B) {
	ctx := context.Background()
	db := newTestDB(b)
	defer db.Close()
	repo := model.Repo{ID: "repo:/bench", Name: "bench", LocalPath: "/bench", Lang: "go"}
	if err := db.UpsertRepo(ctx, repo); err != nil {
		b.Fatal(err)
	}
	const elements, relationships = 620, 1500
	els := make([]model.Element, 0, elements)
	for i := 0; i < elements; i++ {
		els = append(els, model.Element{
			ID: fmt.Sprintf("cmp:pkg%d.Type%d", i/10, i%10), Type: model.TypeComponent,
			Name: fmt.Sprintf("Type%d", i), RepoID: repo.ID, FilePath: "pkg/file.go",
			Methods: []string{"Run", "Stop"},
		})
	}
	rels := make([]model.Relationship, 0, relationships)
	for i := 0; i < relationships; i++ {
		rels = append(rels, model.Relationship{
			FromID: fmt.Sprintf("cmp:pkg%d.Type%d", i/10, i%10),
			ToID:   fmt.Sprintf("cmp:pkg%d.Type%d", (i+7)%elements/10, (i+7)%elements%10),
			Type:   model.RelCalls, Weight: 1 + i%9,
		})
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		snap := model.ArchitectureSnapshot{ID: fmt.Sprintf("s%019d", i+1), RepoID: repo.ID}
		if err := db.SaveSnapshot(ctx, snap, els, rels); err != nil {
			b.Fatal(err)
		}
	}
}
