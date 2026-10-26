// Command oac is OpenArchitect's headless face: it drives the same core as the
// desktop app and imports no UI stack (spec §5.1, R1).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
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

// subcommand is one verb of the CLI. subcommands is the single source of the
// list: run() validates args[0] against it *before* opening the database
// (a typo must not create a real DB, F4) and dispatches through the same
// entry, so validation and dispatch can never drift apart.
type subcommand struct {
	arg string // what the command takes exactly one of, for the arity error
	run func(ctx context.Context, db *store.DB, id string, depth int, out io.Writer) error
}

var subcommands = map[string]subcommand{
	"scan": {arg: "path", run: func(ctx context.Context, db *store.DB, id string, _ int, out io.Writer) error {
		return runScan(ctx, db, id, out)
	}},
	"graph": {arg: "repo id", run: func(ctx context.Context, db *store.DB, id string, _ int, out io.Writer) error {
		return runGraph(ctx, db, id, out)
	}},
	"impact": {arg: "element id", run: func(ctx context.Context, db *store.DB, id string, depth int, out io.Writer) error {
		return runImpact(ctx, db, id, depth, out)
	}},
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return usage("no subcommand")
	}
	sub, ok := subcommands[args[0]]
	if !ok {
		return usage(fmt.Sprintf("unknown subcommand %q", args[0]))
	}
	defaultPath, err := defaultDBPath()
	if err != nil {
		return err
	}
	// ContinueOnError (F3): parse errors must come back as errors from run(),
	// not os.Exit(2) from ExitOnError killing the process. The flagset's own
	// output is discarded so a failed parse does not interleave an
	// auto-generated flag list into stderr; the error returned here already
	// names the offending flag, and -h prints the usage() block below, which
	// is the one that documents the commands.
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dbPath := fs.String("db", defaultPath, "database file")
	depth := fs.Int("depth", 2, "impact diffusion depth")
	if err := fs.Parse(hoistFlags(fs, args[1:])); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printUsage()
			return nil
		}
		return err
	}
	rest := fs.Args()
	if len(rest) != 1 {
		return fmt.Errorf("%s wants exactly one %s, got %v", args[0], sub.arg, rest)
	}

	db, err := openDB(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	return sub.run(ctx, db, rest[0], *depth, os.Stdout)
}

func printUsage() {
	fmt.Fprint(os.Stderr, `usage:
  oac scan   <path>                   [--db PATH]
  oac graph  <repo-id>                [--db PATH]
  oac impact <element-id> [--depth N] [--db PATH]
`)
}

func usage(reason string) error {
	printUsage()
	return errors.New(reason)
}

// hoistFlags moves flags ahead of positional args: Go's flag package stops
// parsing at the first positional, while usage() documents the flags last
// ("oac scan <path> [--db PATH]"). Both orders are therefore accepted.
// Whether a flag takes a value is read from the FlagSet registration itself
// (hoistValue), not from a hand-maintained list, so registering a new value
// flag on fs needs no change here (F2). After a literal "--" every remaining
// token is a positional, dash-leading ones included; the "--" itself stays in
// the positional group so flag.Parse still finds and strips it (F1).
func hoistFlags(fs *flag.FlagSet, args []string) []string {
	var flags, positionals []string
	terminated := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case terminated:
			positionals = append(positionals, a)
		case a == "--":
			terminated = true
			positionals = append(positionals, a)
		case a == "-" || !strings.HasPrefix(a, "-"):
			positionals = append(positionals, a)
		case strings.Contains(a, "="): // self-contained, e.g. --db=x
			flags = append(flags, a)
		case hoistValue(fs, strings.TrimLeft(a, "-")):
			// The flag travels together with its value. With no token
			// left to take it stays among the positionals: hoisting it
			// bare would let it eat a positional as its value, while
			// plain flag.Parse would stop at the first positional and
			// leave it unconsumed in Args (F3).
			if i+1 < len(args) {
				flags = append(flags, a, args[i+1])
				i++
			} else {
				positionals = append(positionals, a)
			}
		default:
			flags = append(flags, a)
		}
	}
	return append(flags, positionals...)
}

// hoistValue reports whether the registered flag named flagName consumes the
// next token: it must be registered on fs, and per the flag package's own
// rule a flag only skips its argument when its Value satisfies IsBoolFlag.
func hoistValue(fs *flag.FlagSet, flagName string) bool {
	f := fs.Lookup(flagName)
	if f == nil {
		return false
	}
	bf, ok := f.Value.(interface{ IsBoolFlag() bool })
	return !ok || !bf.IsBoolFlag()
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
