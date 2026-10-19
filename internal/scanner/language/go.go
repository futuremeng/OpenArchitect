package language

import (
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

func init() { Register(goAdapter{}) }

type goAdapter struct{}

func (goAdapter) Language() string { return "go" }

// packagesNeedAll is the LoadMode PoC-3 settled on (spec U2), plus NeedImports.
// tools/parseprobe keeps a copy of the same bit set because it must stay
// runnable as the measurement harness.
//
// NeedImports is not optional here: the containment walk in componentOwners
// reads pkg.Imports, and without this bit the v0.52.0 driver clears that field
// on the way out (its mode filter is guarded by requestedMode&NeedImports). It
// costs nothing — impliedLoadMode already adds NeedImports for any of
// NeedDeps/NeedTypes/NeedTypesInfo, because "all these things require knowing
// the import graph" — so the bit only stops a graph the driver has already
// built from being thrown away.
const packagesNeedAll = packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
	packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports | packages.NeedDeps |
	packages.NeedModule

// Detect reports whether root is a Go module. Merging "no go.mod" and "go.mod
// unreadable" into one (false, err) is deliberate: R2's degradation semantics
// are "this directory cannot be parsed as a Go module", and both causes have
// the same consequence for the user (Task 11 records one package error and the
// CLI stays read-only). Splitting them would force callers to run a second
// os.Stat whose result has no guarantee against the first.
func (goAdapter) Detect(root string) (bool, error) {
	info, err := os.Stat(filepath.Join(root, "go.mod"))
	if err != nil {
		return false, err // includes ErrNotExist: an absent go.mod is "unreadable" here
	}
	return !info.IsDir(), nil
}

func (a goAdapter) Parse(ctx context.Context, root string) (Output, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return Output{}, err
	}
	pkgs, modulePath, err := load(ctx, abs)
	if err != nil {
		return Output{}, err
	}
	repoID := model.RepoIDFor(abs)

	// spec §6.3: the scanned module itself is always exactly one System element.
	elements := []model.Element{{
		ID:          model.SystemID(modulePath),
		Type:        model.TypeSystem,
		Name:        filepath.Base(modulePath),
		Description: rootPackageDoc(pkgs, modulePath),
		RepoID:      repoID,
	}}
	var (
		packageErrors []string
		comps         = map[string]*component{} // element id -> component
	)
	containers := map[string]bool{} // pkgPath of packages named main

	for _, pkg := range pkgs {
		for _, e := range pkg.Errors {
			packageErrors = append(packageErrors, fmt.Sprintf("%s: %s", pkg.PkgPath, e))
		}
		if isTestVariant(pkg.ID) || !ownPackage(modulePath, pkg.PkgPath) {
			continue
		}
		if pkg.IllTyped {
			packageErrors = append(packageErrors, fmt.Sprintf("%s: package has type errors", pkg.PkgPath))
		}
		if pkg.Name == "main" {
			containers[pkg.PkgPath] = true
			elements = append(elements, model.Element{
				ID: model.ContainerID(pkg.PkgPath), Type: model.TypeContainer,
				Name: filepath.Base(pkg.PkgPath), RepoID: repoID,
			})
			continue
		}
		docs := typeDocs(pkg)
		for _, named := range exportedComponentTypes(pkg) {
			id := model.ComponentID(pkg.PkgPath, named.Obj().Name())
			c := &component{pkgPath: pkg.PkgPath, named: named}
			comps[id] = c
			elements = append(elements, model.Element{
				ID: id, Type: model.TypeComponent, Name: named.Obj().Name(),
				Description: docs[named.Obj()], RepoID: repoID,
				FilePath: relativeFile(abs, pkg, named.Obj().Pos()),
				Methods:  methodNames(named),
			})
		}
	}

	// packages.Load gives no order guarantee for its slice, and the global
	// constraint requires sorted elements before Task 11's golden comparison.
	slices.SortFunc(elements, func(a, b model.Element) int { return strings.Compare(a.ID, b.ID) })

	out := Output{ModulePath: modulePath, Elements: elements, PackageErrors: packageErrors}
	out.Relationships = relationships(modulePath, pkgs, comps, containers)
	return out, nil
}

// component is a named type the scanner treats as a C4 Component. It carries
// only what the edge rules need: where it lives, and the type itself.
type component struct {
	pkgPath string
	named   *types.Named
}

func exportedComponentTypes(pkg *packages.Package) []*types.Named {
	if pkg.Types == nil {
		return nil
	}
	var out []*types.Named
	scope := pkg.Types.Scope()
	for _, name := range scope.Names() {
		tn, ok := scope.Lookup(name).(*types.TypeName)
		if !ok || !tn.Exported() {
			continue
		}
		named, ok := tn.Type().(*types.Named)
		if !ok {
			continue
		}
		// spec §6.3's Component rule requires a named type: `Type() == Obj()`.
		// A type alias (`type Sink = Reader`, `type Handler = http.Handler`) is a
		// TypeName whose type resolves to the *aliased* object's *types.Named, so
		// without this guard the alias would emit a Component built from another
		// type's declaration — a duplicate/foreign element whose FilePath can even
		// be computed from a file outside the repo (§6.3 排除集). On go1.27 the
		// materialised *types.Alias already fails the assertion above; the guard
		// is the same rule written against the type object so the exclusion does
		// not depend on that toolchain detail.
		if named.Obj() != tn {
			continue
		}
		switch named.Underlying().(type) {
		case *types.Struct, *types.Interface:
			out = append(out, named)
		}
	}
	return out
}

func methodNames(named *types.Named) []string {
	set := types.NewMethodSet(types.NewPointer(named))
	out := make([]string, 0, set.Len())
	for i := 0; i < set.Len(); i++ {
		method, ok := set.At(i).Obj().(*types.Func)
		if !ok || !method.Exported() {
			continue
		}
		out = append(out, method.Name())
	}
	slices.Sort(out)
	return out
}

func ownPackage(modulePath, pkgPath string) bool {
	return pkgPath == modulePath || strings.HasPrefix(pkgPath, modulePath+"/")
}

// Test variants carry a bracketed id such as "main [example.com/x.test]".
func isTestVariant(id string) bool {
	return strings.Contains(id, ".test]") || strings.Contains(id, "-test]") || strings.Contains(id, ".coverpkg")
}

func relativeFile(abs string, pkg *packages.Package, pos token.Pos) string {
	if !pos.IsValid() || pkg.Fset == nil {
		return ""
	}
	full := pkg.Fset.Position(pos).Filename
	rel, err := filepath.Rel(abs, full)
	if err != nil {
		return full
	}
	return rel
}

// typeDocs maps each package-level named type to its doc comment. The comment
// has to come from the AST: go/types (go1.27.2) gives types.Object and
// *types.TypeName no Doc method at all, so there is nothing to read off the
// type-checker's object. pkg.Syntax is walked together with pkg.TypesInfo.Defs,
// which links each declaration's identifier back to that object.
func typeDocs(pkg *packages.Package) map[*types.TypeName]string {
	docs := map[*types.TypeName]string{}
	if pkg.TypesInfo == nil {
		return docs
	}
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				continue
			}
			for _, spec := range gen.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				typeName, ok := pkg.TypesInfo.Defs[typeSpec.Name].(*types.TypeName)
				if !ok {
					continue
				}
				if text := docText(typeSpec.Doc, gen.Doc); text != "" {
					docs[typeName] = text
				}
			}
		}
	}
	return docs
}

// rootPackageDoc is the module root package's doc comment, which is the sentence
// that describes the System (spec §6.1 takes Description from doc comments).
// Empty when the module has no root package or that package is undocumented.
func rootPackageDoc(pkgs []*packages.Package, modulePath string) string {
	for _, pkg := range pkgs {
		if pkg.PkgPath != modulePath {
			continue
		}
		for _, file := range pkg.Syntax {
			if text := docText(file.Doc, nil); text != "" {
				return text
			}
		}
	}
	return ""
}

// docText prefers the spec's own comment — grouped declarations give every spec
// its own — and falls back to the GenDecl comment, which is where go/parser puts
// the doc of an unparenthesised "type X struct { ... }". CommentGroup.Text has
// already stripped the "//" markers.
func docText(spec, decl *ast.CommentGroup) string {
	if spec != nil {
		return strings.TrimSpace(spec.Text())
	}
	if decl != nil {
		return strings.TrimSpace(decl.Text())
	}
	return ""
}

func load(ctx context.Context, root string) ([]*packages.Package, string, error) {
	cfg := &packages.Config{Mode: packagesNeedAll, Dir: root, Context: ctx}
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return nil, "", fmt.Errorf("language: load %s: %w", root, err)
	}
	modulePath := ""
	for _, pkg := range pkgs {
		if pkg.Module != nil {
			modulePath = pkg.Module.Path
			break
		}
	}
	if modulePath == "" {
		return nil, "", fmt.Errorf("language: no module under %s", root)
	}
	return pkgs, modulePath, nil
}

// relationships derives the edges. Task 10 adds depends_on and calls.
func relationships(modulePath string, pkgs []*packages.Package,
	comps map[string]*component, containers map[string]bool) []model.Relationship {
	var acc []model.Relationship
	// System -> every Container.
	for pkgPath := range containers {
		acc = append(acc, model.Relationship{
			FromID: model.SystemID(modulePath), ToID: model.ContainerID(pkgPath),
			Type: model.RelContains, Weight: 1,
		})
	}
	// Component -> exactly one owner: the sole Container that reaches it, else System.
	for _, owner := range componentOwners(modulePath, pkgs, comps, containers) {
		acc = append(acc, model.Relationship{FromID: owner.from, ToID: owner.toID, Type: model.RelContains, Weight: 1})
	}
	return dedup(acc)
}

type ownership struct{ from, toID string }

func componentOwners(modulePath string, pkgs []*packages.Package,
	comps map[string]*component, containers map[string]bool) []ownership {
	reaching := map[string]map[string]bool{} // component pkgPath -> container pkgPaths
	for container := range containers {
		seen := map[string]bool{}
		var walk func(pkgPath string)
		walk = func(pkgPath string) {
			if seen[pkgPath] {
				return
			}
			seen[pkgPath] = true
			for _, imported := range importsOf(pkgs, pkgPath) {
				walk(imported)
			}
		}
		walk(container)
		delete(seen, container)
		for pkgPath := range seen {
			if reaching[pkgPath] == nil {
				reaching[pkgPath] = map[string]bool{}
			}
			reaching[pkgPath][container] = true
		}
	}

	out := make([]ownership, 0, len(comps))
	for id, comp := range comps {
		owners := reaching[comp.pkgPath]
		if len(owners) == 1 {
			for container := range owners {
				out = append(out, ownership{
					from: model.ContainerID(container), toID: id})
			}
			continue
		}
		out = append(out, ownership{from: model.SystemID(modulePath), toID: id})
	}
	slices.SortFunc(out, func(a, b ownership) int { return strings.Compare(a.from+a.toID, b.from+b.toID) })
	return out
}

func importsOf(pkgs []*packages.Package, pkgPath string) []string {
	for _, pkg := range pkgs {
		if pkg.PkgPath != pkgPath || pkg.Imports == nil {
			continue
		}
		out := make([]string, 0, len(pkg.Imports))
		for _, imported := range pkg.Imports {
			if imported != nil {
				out = append(out, imported.PkgPath)
			}
		}
		slices.Sort(out)
		return out
	}
	return nil
}

// dedup merges the (from,to,type) triple into one edge and sums Weight, then
// sorts so golden comparisons are stable (§11).
func dedup(in []model.Relationship) []model.Relationship {
	type key struct {
		from, to string
		kind     model.RelType
	}
	index := map[key]int{}
	out := []model.Relationship{}
	for _, rel := range in {
		k := key{rel.FromID, rel.ToID, rel.Type}
		if i, ok := index[k]; ok {
			out[i].Weight += rel.Weight
			continue
		}
		index[k] = len(out)
		weight := rel.Weight
		if weight < 1 {
			weight = 1
		}
		out = append(out, model.Relationship{FromID: rel.FromID, ToID: rel.ToID, Type: rel.Type, Weight: weight})
	}
	slices.SortFunc(out, func(a, b model.Relationship) int {
		if c := strings.Compare(a.FromID, b.FromID); c != 0 {
			return c
		}
		if c := strings.Compare(a.ToID, b.ToID); c != 0 {
			return c
		}
		return strings.Compare(string(a.Type), string(b.Type))
	})
	return out
}
