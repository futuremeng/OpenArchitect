// Command oac is OpenArchitect's headless face: it drives the same core as the
// desktop app and imports no UI stack (spec §5.1, R1).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/futuremeng/OpenArchitect/internal/core/store"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "oac:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return usage()
	}
	defaultPath, err := defaultDBPath()
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet(args[0], flag.ExitOnError)
	dbPath := fs.String("db", defaultPath, "database file")
	depth := fs.Int("depth", 2, "impact diffusion depth")
	if err := fs.Parse(hoistFlags(args[1:])); err != nil {
		return err
	}
	rest := fs.Args()

	db, err := openDB(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	switch args[0] {
	case "scan":
		if len(rest) != 1 {
			return fmt.Errorf("scan wants exactly one path, got %v", rest)
		}
		return runScan(ctx, db, rest[0], os.Stdout)
	case "graph":
		if len(rest) != 1 {
			return fmt.Errorf("graph wants exactly one repo id, got %v", rest)
		}
		return runGraph(ctx, db, rest[0], os.Stdout)
	case "impact":
		if len(rest) != 1 {
			return fmt.Errorf("impact wants exactly one element id, got %v", rest)
		}
		return runImpact(ctx, db, rest[0], *depth, os.Stdout)
	default:
		return usage()
	}
}

func usage() error {
	fmt.Fprint(os.Stderr, `usage:
  oac scan   <path>                   [--db PATH]
  oac graph  <repo-id>                [--db PATH]
  oac impact <element-id> [--depth N] [--db PATH]
`)
	return fmt.Errorf("no subcommand")
}

// hoistFlags moves flags ahead of positional args: Go's flag package stops
// parsing at the first positional, while usage() documents the flags last
// ("oac scan <path> [--db PATH]"). Both orders are therefore accepted. The
// value-taking flags of this CLI are exactly -db and -depth, so their values
// travel with them.
func hoistFlags(args []string) []string {
	valueFlags := map[string]bool{"db": true, "depth": true}
	var flags, positionals []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "-" || a == "--" || !strings.HasPrefix(a, "-") {
			positionals = append(positionals, a)
			continue
		}
		if strings.Contains(a, "=") { // self-contained, e.g. --db=x
			flags = append(flags, a)
			continue
		}
		if valueFlags[strings.TrimLeft(a, "-")] && i+1 < len(args) {
			flags = append(flags, a, args[i+1])
			i++
			continue
		}
		flags = append(flags, a)
	}
	return append(flags, positionals...)
}

// defaultDBPath is the single hinge where the CLI and the GUI open the same
// database: mygo's PathUserData is os.UserConfigDir()+App.Name()
// (mygo@v0.5.5 paths.go:91-92), but App.Name() only resolves to "OpenArchitect"
// from the bundle name or an explicit SetName—it silently falls back to the
// executable basename when unpackaged (app.go:275-289), so a plain
// `go build ./cmd/openarchitect` binary would give the GUI its own database.
func defaultDBPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "OpenArchitect", "openarchitect.db"), nil
}

func openDB(ctx context.Context, path string) (*store.DB, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	return store.Open(ctx, path)
}
