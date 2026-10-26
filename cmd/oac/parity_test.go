package main

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/futuremeng/OpenArchitect/internal/core/graph"
)

// TestCLIAndLibraryAgree: the ids "oac impact" prints must equal
// graph.Analyze().AffectedIDs() for the same fixture. The GUI reads the same
// AffectedIDs, so this single assertion is what keeps CLI, library and GUI from
// drifting apart (spec §4.3 DoD-4).
func TestCLIAndLibraryAgree(t *testing.T) {
	ctx := context.Background()
	db, snapshotID, _ := scanFixture(t, "shared-lib")
	const center = "cmp:example.com/shared/core.Server"

	elements, err := db.Elements(ctx, snapshotID)
	if err != nil {
		t.Fatal(err)
	}
	rels, err := db.Relationships(ctx, snapshotID)
	if err != nil {
		t.Fatal(err)
	}
	report, err := graph.Analyze(graph.Build(elements, rels), center, 2)
	if err != nil {
		t.Fatal(err)
	}
	fromLibrary := report.AffectedIDs()

	var buf bytes.Buffer
	if err := runImpact(ctx, db, center, 2, &buf); err != nil {
		t.Fatal(err)
	}
	var fromCLI []string
	for _, field := range strings.Fields(buf.String()) {
		if hasIDPrefix(field) {
			fromCLI = append(fromCLI, field)
		}
	}
	slices.Sort(fromCLI)
	fromCLI = slices.Compact(fromCLI)
	if !slices.Equal(fromLibrary, fromCLI) {
		t.Fatalf("CLI = %v\nlibrary = %v", fromCLI, fromLibrary)
	}
}

func hasIDPrefix(field string) bool {
	for _, prefix := range []string{"sys:", "ctn:", "cmp:", "repo:"} {
		if strings.HasPrefix(field, prefix) {
			return true
		}
	}
	return false
}
