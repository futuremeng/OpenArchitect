// Command treemapprobe runs the two treemap gates: PoC-12 times headless
// frames over synthetic trees of 50/600/6000 tiles, PoC-13 measures how the
// two area semantics (exported struct/interface count vs .go file count)
// differ on real repositories. Throwaway harness, spec
// docs/superpowers/research/2026-11-02-treemap-view-feasibility.md §10.
package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components/treemap"
	"github.com/oneo-me/onevra/source/theme"
)

// probeSize is the viewport the layout is timed in — the width the desktop app
// opens at, so the component's text fitting sees real widths.
const probeWidth = 1400

func main() {
	tiles := flag.String("tiles", "50,600,6000", "comma-separated tile counts to time")
	frames := flag.Int("frames", 60, "frames to time per tree")
	repos := flag.String("repos", "", "comma-separated repo roots for the area-semantics pass")
	flag.Parse()

	if *repos != "" {
		for _, root := range strings.Split(*repos, ",") {
			if root = strings.TrimSpace(root); root != "" {
				reportSemantics(root)
			}
		}
	}
	for _, n := range parseIntList(*tiles) {
		reportLayout(n, *frames)
	}
}

func parseIntList(s string) []int {
	var out []int
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		var n int
		if _, err := fmt.Sscanf(part, "%d", &n); err == nil {
			out = append(out, n)
		}
	}
	return out
}

// reportLayout times `frames` headless builds of a repo→package→element tree
// with `n` leaves. Animation is off so the numbers are layout, not easing.
func reportLayout(n, frames int) {
	tree := syntheticTree(n)
	var timings []time.Duration
	var state treemap.State
	tt := ui.NewTester(func(c *ui.Context) {
		theme.Apply(c)
		start := time.Now()
		treemap.View(c, &state, tree, treemap.Options{
			Label:             fmt.Sprintf("%d tiles", n),
			AnimationDuration: -time.Second,
			MeasureLabel:      "dependents",
			MeasureDomain:     [2]float64{0, 20},
		})
		timings = append(timings, time.Since(start))
		c.Invalidate() // force a rebuild every Advance: this times layout, not idle frames
	}, probeWidth, 900)
	tt.Frame()
	for i := 0; i < frames; i++ {
		tt.Advance(16 * time.Millisecond)
	}
	fmt.Printf("PoC-12 tiles=%d frames=%d first=%v median=%v p95=%v max=%v\n",
		n, len(timings), timings[0].Round(time.Microsecond),
		percentile(timings, 0.5), percentile(timings, 0.95), maxDuration(timings))
}

// syntheticTree spreads n leaves over 10 repos x 10 packages so the layout
// sees the depth and the label lengths production produces.
func syntheticTree(n int) []treemap.Node {
	const repos, pkgs = 10, 10
	leaves := make([][]treemap.Node, repos*pkgs)
	for i := 0; i < n; i++ {
		bucket := i % (repos * pkgs)
		leaves[bucket] = append(leaves[bucket], treemap.Node{
			ID:      fmt.Sprintf("cmp:example.com/probe/p%d.T%d", bucket, i),
			Label:   fmt.Sprintf("Type%05d", i),
			Value:   1,
			Measure: float64(i % 21),
		})
	}
	out := make([]treemap.Node, 0, repos)
	for r := 0; r < repos; r++ {
		repo := treemap.Node{ID: fmt.Sprintf("sys:repo%d", r), Label: fmt.Sprintf("repo%d", r)}
		for p := 0; p < pkgs; p++ {
			group := treemap.Node{ID: fmt.Sprintf("pkg:%d/%d", r, p), Label: fmt.Sprintf("internal/pkg%02d", p)}
			group.Children = leaves[r*pkgs+p]
			repo.Children = append(repo.Children, group)
		}
		out = append(out, repo)
	}
	return out
}

// pkgStat is one package directory under the two area semantics.
type pkgStat struct {
	dir    string
	files  int
	elems  int // exported struct/interface declarations
	ghosts int // files in this dir contributing zero elements
}

// reportSemantics walks a repository and reports how the two area choices
// disagree: packages that would be invisible at element-count area, and the
// share of files carrying no exported struct/interface.
func reportSemantics(root string) {
	stats, err := walkGo(root)
	if err != nil {
		fmt.Printf("PoC-13 repo=%s ERROR %v\n", root, err)
		return
	}
	var files, elems, emptyPkgs, ghostFiles int
	for _, s := range stats {
		files += s.files
		elems += s.elems
		ghostFiles += s.ghosts
		if s.elems == 0 {
			emptyPkgs++
		}
	}
	share := 0.0
	if files > 0 {
		share = float64(ghostFiles) / float64(files)
	}
	fmt.Printf("PoC-13 repo=%s packages=%d files=%d elements=%d invisiblePackages=%d ghostFiles=%d ghostFileShare=%.2f\n",
		root, len(stats), files, elems, emptyPkgs, ghostFiles, share)
}

func walkGo(root string) ([]pkgStat, error) {
	byDir := map[string]*pkgStat{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable corners of a real working tree are not a failure here
		}
		if d.IsDir() {
			switch filepath.Base(path) {
			case "vendor", "node_modules", ".git", "dist", "build":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		dir := filepath.Dir(path)
		st, ok := byDir[dir]
		if !ok {
			st = &pkgStat{dir: strings.TrimPrefix(dir, root)}
			byDir[dir] = st
		}
		st.files++
		n := countElements(path)
		st.elems += n
		if n == 0 {
			st.ghosts++
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := make([]pkgStat, 0, len(byDir))
	for _, s := range byDir {
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].dir < out[j].dir })
	return out, nil
}

// countElements counts exported struct and interface declarations — the
// production mapping (slice 1 spec §6.3) — without type information.
func countElements(path string) int {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil || file == nil {
		return 0
	}
	count := 0
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok || !ts.Name.IsExported() {
				continue
			}
			switch ts.Type.(type) {
			case *ast.StructType, *ast.InterfaceType:
				count++
			}
		}
	}
	return count
}

func percentile(d []time.Duration, p float64) time.Duration {
	if len(d) == 0 {
		return 0
	}
	s := make([]time.Duration, len(d))
	copy(s, d)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	return s[int(p*float64(len(s)-1))].Round(time.Microsecond)
}

func maxDuration(d []time.Duration) time.Duration {
	var m time.Duration
	for _, v := range d {
		if v > m {
			m = v
		}
	}
	return m.Round(time.Microsecond)
}
