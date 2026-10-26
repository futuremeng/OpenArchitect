package main

import (
	"context"
	"fmt"
	"io"

	"github.com/futuremeng/OpenArchitect/internal/core/store"
	"github.com/futuremeng/OpenArchitect/internal/scanner"
)

func runScan(ctx context.Context, db *store.DB, root string, out io.Writer) error {
	report, err := scanner.Run(ctx, db, root)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "repo %s\nsnapshot %s\nlanguage %s\nmodule %s\nelements %d relationships %d\n",
		report.RepoID, report.SnapshotID, report.Language, report.ModulePath,
		report.Elements, report.Relationships)
	if len(report.PackageErrors) > 0 {
		fmt.Fprintf(out, "package errors (%d):\n", len(report.PackageErrors))
		for _, item := range report.PackageErrors {
			fmt.Fprintf(out, "  %s\n", item)
		}
	}
	return nil
}
