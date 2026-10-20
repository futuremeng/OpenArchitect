package main

import (
	"context"
	"fmt"
	"io"

	"github.com/futuremeng/OpenArchitect/internal/core/graph"
	"github.com/futuremeng/OpenArchitect/internal/core/store"
)

// loadGraph builds the in-memory graph from stored rows. What the CLI, the GUI
// and the tests share is this sequence—db.Elements + db.Relationships +
// graph.Build over the same snapshot rows—not this function: loadGraph lives
// in package main and the GUI must not import it (spec §5.1 layering).
func loadGraph(ctx context.Context, db *store.DB, snapshotID string) (*graph.Graph, error) {
	elements, err := db.Elements(ctx, snapshotID)
	if err != nil {
		return nil, err
	}
	rels, err := db.Relationships(ctx, snapshotID)
	if err != nil {
		return nil, err
	}
	return graph.Build(elements, rels), nil
}

func runGraph(ctx context.Context, db *store.DB, repoID string, out io.Writer) error {
	snapshotID, err := db.LatestSnapshotID(ctx, repoID)
	if err != nil {
		return err
	}
	if snapshotID == "" {
		// No inner "oac:" prefix: main already prints "oac: %v".
		return fmt.Errorf("repo %q has no snapshot; run `oac scan` first", repoID)
	}
	g, err := loadGraph(ctx, db, snapshotID)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "snapshot %s: %d elements, %d relationships\n",
		snapshotID, len(g.Elements()), len(g.Relationships()))
	for _, e := range g.Elements() {
		fmt.Fprintf(out, "%s\t%s\t%s\n", e.ID, e.Type, e.Name)
	}
	for _, rel := range g.Relationships() {
		fmt.Fprintf(out, "%s -> %s [%s] x%d\n", rel.FromID, rel.ToID, rel.Type, rel.Weight)
	}
	return nil
}
