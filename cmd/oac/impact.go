package main

import (
	"context"
	"fmt"
	"io"

	"github.com/futuremeng/OpenArchitect/internal/core/graph"
	"github.com/futuremeng/OpenArchitect/internal/core/store"
)

func runImpact(ctx context.Context, db *store.DB, elementID string, maxDepth int, out io.Writer) error {
	snapshotID, err := db.SnapshotForElement(ctx, elementID)
	if err != nil {
		return err
	}
	g, err := loadGraph(ctx, db, snapshotID)
	if err != nil {
		return err
	}
	report, err := graph.Analyze(g, elementID, maxDepth)
	if err != nil {
		return err
	}
	// Only the two tables carry ids. The Affected set is deliberately not
	// printed: the parity test derives it from these rows, and printing it
	// again would put the same ids on screen twice.
	fmt.Fprintf(out, "Center %s %s (%s)\n", report.Center.ID, report.Center.Name, report.Center.Type)
	writeHits(out, "Upstream", report.Upstream)
	writeHits(out, "Downstream", report.Downstream)
	return nil
}

func writeHits(out io.Writer, title string, hits []graph.Hit) {
	fmt.Fprintf(out, "%s (%d)\n", title, len(hits))
	if len(hits) == 0 {
		fmt.Fprintln(out, "  (none)")
		return
	}
	for _, hit := range hits {
		fmt.Fprintf(out, "  depth %d  %s  %s <%s>  %s\n",
			hit.Depth, hit.Element.ID, hit.Element.Name, hit.Element.Type, hit.Via)
	}
}
