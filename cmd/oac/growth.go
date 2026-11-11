package main

import (
	"context"
	"fmt"
	"io"

	"github.com/futuremeng/OpenArchitect/internal/core/store"
)

// runGrowth prints a repo's growth story newest first: what happened to
// which element, and the commit proven to own it when there is one.
func runGrowth(ctx context.Context, db *store.DB, repoID string, out io.Writer) error {
	events, err := db.GrowthEvents(ctx, repoID, "", 200)
	if err != nil {
		return err
	}
	if len(events) == 0 {
		fmt.Fprintln(out, "no growth recorded yet (first scan is a baseline)")
		return nil
	}
	for _, e := range events {
		fmt.Fprintf(out, "%-18s %s %s→%s", e.Kind, e.ElementID,
			tail(e.SnapshotFrom), tail(e.SnapshotTo))
		if e.CommitHash != "" {
			fmt.Fprintf(out, " commit=%s", short(e.CommitHash))
		}
		fmt.Fprintln(out)
	}
	return nil
}

// tail is short() for snapshot ids: NewSnapshotID is a nanosecond stamp, so
// the digits that differ live at the end — the head would render every pair
// identical.
func tail(id string) string {
	if len(id) > 8 {
		return "…" + id[len(id)-8:]
	}
	if id == "" {
		return "-"
	}
	return id
}