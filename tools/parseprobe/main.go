// Command parseprobe measures go/packages cost on a real repository: exported
// named types, edges, wall time and peak heap. Throwaway harness for PoC-3.
package main

import (
	"context"
	"flag"
	"fmt"
	"go/types"
	"log"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/tools/go/packages"
)

// packagesNeedAll is the LoadMode the scanner will use; PoC-3 picks the bits.
const packagesNeedAll = packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
	packages.NeedTypes | packages.NeedTypesInfo | packages.NeedDeps | packages.NeedModule

func main() {
	root := flag.String("root", ".", "repository root holding go.mod")
	flag.Parse()

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	start := time.Now()

	modPath, pkgs, err := load(*root)
	if err != nil {
		log.Fatal(err)
	}
	elapsed := time.Since(start)
	runtime.ReadMemStats(&after)

	var exported, ownPkgs, errs int
	for _, pkg := range pkgs {
		if len(pkg.Errors) > 0 {
			errs++
		}
		if pkg.Module == nil || pkg.Types == nil {
			continue
		}
		if !own(modPath, pkg.PkgPath) {
			continue
		}
		ownPkgs++
		scope := pkg.Types.Scope()
		for _, name := range scope.Names() {
			obj := scope.Lookup(name)
			if _, ok := componentOf(obj); !ok {
				continue
			}
			exported++
		}
	}

	fmt.Printf("module=%s pkgs=%d ownPkgs=%d exportedTypes=%d packageErrors=%d elapsed=%s heapAllocMB=%.1f sysMB=%.1f\n",
		modPath, len(pkgs), ownPkgs, exported, errs, elapsed.Round(time.Millisecond),
		float64(after.HeapAlloc)/(1<<20), float64(after.Sys)/(1<<20))
}

func load(root string) (string, []*packages.Package, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", nil, err
	}
	cfg := &packages.Config{Mode: packagesNeedAll, Dir: abs, Context: context.Background()}
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return "", nil, err
	}
	mod := ""
	for _, p := range pkgs {
		if p.Module != nil {
			mod = p.Module.Path
			break
		}
	}
	if mod == "" {
		return "", nil, fmt.Errorf("no module under %s", abs)
	}
	return mod, pkgs, nil
}

func own(modulePath, pkgPath string) bool {
	return pkgPath == modulePath || strings.HasPrefix(pkgPath, modulePath+"/")
}

func componentOf(obj types.Object) (*types.Named, bool) {
	tn, ok := obj.(*types.TypeName)
	if !ok || !tn.Exported() {
		return nil, false
	}
	named, ok := tn.Type().(*types.Named)
	if !ok {
		return nil, false
	}
	switch named.Underlying().(type) {
	case *types.Struct, *types.Interface:
		return named, true
	default:
		return nil, false
	}
}
