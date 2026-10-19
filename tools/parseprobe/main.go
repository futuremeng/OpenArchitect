// Command parseprobe measures go/packages cost on a real repository: exported
// named types, wall time and return-time heap snapshot. Throwaway harness for
// PoC-3.
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

	"github.com/futuremeng/OpenArchitect/internal/core/model"
	"github.com/futuremeng/OpenArchitect/internal/scanner/language"
)

// packagesNeedAll is a local copy of the LoadMode the scanner uses. The
// canonical copy lives in internal/scanner/language/go.go and the two must stay
// in sync; this file keeps its own because it must stay runnable as the
// measurement harness. PoC-3 picked the bits; Task 9 added NeedImports, because
// the containment walk reads pkg.Imports, which the driver otherwise clears even
// though impliedLoadMode already builds the graph for
// NeedDeps/NeedTypes/NeedTypesInfo.
const packagesNeedAll = packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
	packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports | packages.NeedDeps |
	packages.NeedModule

func main() {
	root := flag.String("root", ".", "repository root holding go.mod")
	parseMode := flag.Bool("parse", false,
		"time language.Parse end-to-end (load + elements + the Task 10 edge pass) and print element/edge counts")
	flag.Parse()

	if *parseMode {
		adapter, ok := language.Get("go")
		if !ok {
			log.Fatal("parseprobe: go adapter not registered")
		}
		start := time.Now()
		out, err := adapter.Parse(context.Background(), *root)
		elapsed := time.Since(start)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("parse: root=%s module=%s elements=%d relationships=%d packageErrors=%d elapsed=%s\n",
			*root, out.ModulePath, len(out.Elements), len(out.Relationships),
			len(out.PackageErrors), elapsed.Round(time.Millisecond))
		var systems, containers, components int
		for _, e := range out.Elements {
			switch e.Type {
			case model.TypeSystem:
				systems++
			case model.TypeContainer:
				containers++
			case model.TypeComponent:
				components++
			}
		}
		fmt.Printf("parse: byType System=%d Container=%d Component=%d\n", systems, containers, components)
		// The PoC-3 headline number (519 for mygo) counts every exported named
		// struct/interface in the module's own packages. On this toolchain that
		// predicate already refuses a materialised *types.Alias, so aliases sit
		// outside both the headline and the adapter's Component set; the one
		// thing the Component set drops from the headline is the main-package
		// declarations (§6.3). Hence Component == exported - mainPkgTypes, and
		// elements == System + Container + Component. This is a second
		// packages.Load inside the same command, not a reuse of Parse's.
		exported, mainTypes, aliases, err := countPoC3Scope(*root)
		if err != nil {
			log.Fatal(err)
		}
		expected := systems + containers + (exported - mainTypes)
		fmt.Printf("parse: reconcile System=%d + Container=%d + (exported=%d - mainPkgTypes=%d)"+
			" = %d vs elements=%d match=%v (aliases=%d excluded from both counts)\n",
			systems, containers, exported, mainTypes, expected, len(out.Elements),
			expected == len(out.Elements), aliases)
		return
	}

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

// countPoC3Scope re-measures the exportedTypes headline of PoC-3 with the same
// predicate as the load-only path above, and splits it by the two things the
// adapter's Component rule is documented to drop: declarations in main packages
// (§6.3) and type aliases. Aliases are counted separately with types.Unalias
// precisely because the load-only predicate cannot cast a materialised
// *types.Alias to *types.Named: the headline already excludes them, so the
// split shows which of the two exclusions is additive against the headline and
// which is already netted out.
func countPoC3Scope(root string) (exported, mainPkg, aliases int, err error) {
	modPath, pkgs, err := load(root)
	if err != nil {
		return 0, 0, 0, err
	}
	for _, pkg := range pkgs {
		if pkg.Module == nil || pkg.Types == nil || !own(modPath, pkg.PkgPath) {
			continue
		}
		scope := pkg.Types.Scope()
		for _, name := range scope.Names() {
			tn, ok := scope.Lookup(name).(*types.TypeName)
			if !ok || !tn.Exported() {
				continue
			}
			if tn.IsAlias() {
				named, ok := types.Unalias(tn.Type()).(*types.Named)
				if !ok {
					continue
				}
				switch named.Underlying().(type) {
				case *types.Struct, *types.Interface:
					aliases++
				}
				continue
			}
			if _, ok := componentOf(tn); !ok {
				continue
			}
			exported++
			if pkg.Name == "main" {
				mainPkg++
			}
		}
	}
	return exported, mainPkg, aliases, nil
}
