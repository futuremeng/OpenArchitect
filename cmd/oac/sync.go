package main

import (
	"context"
	"fmt"
	"io"

	"github.com/futuremeng/OpenArchitect/internal/core/store"
	"github.com/futuremeng/OpenArchitect/internal/forest"
)

func runSync(ctx context.Context, db *store.DB, root string, out io.Writer) error {
	report, err := forest.Sync(ctx, db, root)
	if err != nil {
		return err
	}
	for _, line := range report.Repos {
		fmt.Fprintf(out, "%s %s from=%s to=%s commits=%d scanned=%t",
			line.Repo.Name, line.Action, short(line.From), short(line.To),
			line.Commits, line.Scanned)
		if line.Note != "" {
			fmt.Fprintf(out, " note=%q", line.Note)
		}
		fmt.Fprintln(out)
	}
	return nil
}

func short(hash string) string {
	if len(hash) > 8 {
		return hash[:8]
	}
	if hash == "" {
		return "-"
	}
	return hash
}