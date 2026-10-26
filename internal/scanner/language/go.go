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
			c := &component{id: id, pkgPath: pkg.PkgPath, named: named}
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
// only what the edge rules need: its element id, where it lives, and the type
// itself.
type component struct {
	id      string
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

// relationships derives the edges: contains first, then the Task 10
// depends_on/calls inferences. Same (from,to,type) triples merge into one edge
// with accumulated Weight (spec §6.4 去重与多重性); intra-package pairs are
// kept in storage and only skipped when drawing.
func relationships(modulePath string, pkgs []*packages.Package,
	comps map[string]*component, containers map[string]bool) []model.Relationship {
	// comps is keyed by element id; the ownership fallback needs the reverse
	// (package -> components declared in it), so index once here.
	byPackage := map[string][]*component{}
	for _, comp := range comps {
		byPackage[comp.pkgPath] = append(byPackage[comp.pkgPath], comp)
	}

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
	for _, pkg := range pkgs {
		if isTestVariant(pkg.ID) || pkg.Name == "main" || !ownPackage(modulePath, pkg.PkgPath) {
			continue
		}
		acc = append(acc, dependsOnEdges(modulePath, pkg, comps, byPackage)...)
		acc = append(acc, callsEdges(modulePath, pkg, comps, byPackage)...)
	}
	return dedup(acc)
}

// componentIDFor turns a named type into its element id. ownPackage types are
// the scan's own components (ok without further checks); foreign exported
// named types from a real module (slice 2: cross-module edges) come back
// flagged external, so the caller can store a dangling edge whose endpoint
// only materialises when that other module is scanned. Standard library and
// unexported targets are dropped: an edge to fmt.Println carries no
// architecture (spec §6.4's exclusion set).
func componentIDFor(modulePath string, named *types.Named) (id string, external, ok bool) {
	obj := named.Obj()
	if obj == nil || !obj.Exported() || obj.Pkg() == nil {
		return "", false, false
	}
	pkgPath := obj.Pkg().Path()
	if ownPackage(modulePath, pkgPath) {
		return model.ComponentID(pkgPath, obj.Name()), false, true
	}
	if isStdlibPackage(pkgPath) {
		return "", false, false
	}
	return model.ComponentID(pkgPath, obj.Name()), true, true
}

// isStdlibPackage is the two-segment test: a module path's first segment
// carries a domain (github.com, golang.org/x, gopkg.in), a standard library
// path's never does. This is the same heuristic go's own module resolution
// uses to tell std apart from the proxy.
func isStdlibPackage(pkgPath string) bool {
	first := pkgPath
	if i := strings.Index(pkgPath, "/"); i >= 0 {
		first = pkgPath[:i]
	}
	return !strings.Contains(first, ".")
}

// edgeOwnership answers "which Component owns this source position?" — the
// attribution rule behind spec §6.4. A position inside a method whose receiver
// is a Component belongs to that Component; otherwise the nearest preceding
// component declaration in the same file owns it; a file-level function with no
// such owner contributes no edge.
func edgeOwnership(modulePath string, pkg *packages.Package, byPackage map[string][]*component,
	pos token.Pos) (string, bool) {
	if pkg.Fset == nil || !pos.IsValid() {
		return "", false
	}
	target := pkg.Fset.Position(pos)
	for _, syntax := range pkg.Syntax {
		if pkg.Fset.Position(syntax.Pos()).Filename != target.Filename {
			continue
		}
		return ownershipInFile(modulePath, pkg, byPackage, syntax, target.Line)
	}
	return "", false
}

func ownershipInFile(modulePath string, pkg *packages.Package, byPackage map[string][]*component,
	file *ast.File, line int) (string, bool) {
	var owner string
	ast.Inspect(file, func(node ast.Node) bool {
		decl, ok := node.(*ast.FuncDecl)
		if !ok || decl.Body == nil || decl.Recv == nil {
			return true
		}
		start, end := pkg.Fset.Position(decl.Pos()).Line, pkg.Fset.Position(decl.End()).Line
		if line < start || line > end {
			return true
		}
		named, ok := receiverNamed(pkg.TypesInfo.Defs[decl.Name])
		if !ok {
			return true
		}
		// edgeOwnership only ever attributes to this scan's own components;
		// an external flag here would mean a method declared on a foreign
		// type, which ownPackage parsing cannot produce — guard it anyway.
		id, external, _ := componentIDFor(modulePath, named)
		if external {
			id = ""
		}
		owner = id
		return owner == ""
	})
	if owner != "" {
		return owner, true
	}
	return nearestPrecedingComponent(modulePath, pkg, byPackage, file, line)
}

func nearestPrecedingComponent(modulePath string, pkg *packages.Package,
	byPackage map[string][]*component, file *ast.File, line int) (string, bool) {
	filename := pkg.Fset.Position(file.Pos()).Filename
	best, bestLine := "", -1
	for _, comp := range byPackage[pkg.PkgPath] {
		obj := comp.named.Obj()
		if pkg.Fset.Position(obj.Pos()).Filename != filename {
			continue
		}
		if declared := pkg.Fset.Position(obj.Pos()).Line; declared <= line && declared > bestLine {
			best, bestLine = comp.id, declared
		}
	}
	return best, best != ""
}

func receiverNamed(obj types.Object) (*types.Named, bool) {
	fn, ok := obj.(*types.Func)
	if !ok {
		return nil, false
	}
	sig, ok := fn.Type().(*types.Signature)
	if !ok || sig.Recv() == nil {
		return nil, false
	}
	return baseNamed(sig.Recv().Type())
}

func baseNamed(t types.Type) (*types.Named, bool) {
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	named, ok := t.(*types.Named)
	return named, ok
}

// go/ast has no Unparen, so parens come off by hand.
func astUnparen(e ast.Expr) ast.Expr {
	for {
		p, ok := e.(*ast.ParenExpr)
		if !ok {
			return e
		}
		e = p.X
	}
}

func owns(comps map[string]*component, id string) bool {
	_, ok := comps[id]
	return ok
}

func dependsOnEdges(modulePath string, pkg *packages.Package, comps map[string]*component,
	byPackage map[string][]*component) []model.Relationship {
	if pkg.TypesInfo == nil {
		return nil
	}
	var out []model.Relationship
	for ident, used := range pkg.TypesInfo.Uses {
		tn, ok := used.(*types.TypeName)
		if !ok {
			continue
		}
		named, ok := tn.Type().(*types.Named)
		if !ok {
			continue
		}
		to, external, isComponent := componentIDFor(modulePath, named)
		if !isComponent || (!external && !owns(comps, to)) {
			continue
		}
		from, ok := edgeOwnership(modulePath, pkg, byPackage, ident.Pos())
		if !ok || from == to {
			continue
		}
		out = append(out, model.Relationship{FromID: from, ToID: to, Type: model.RelDependsOn, Weight: 1})
	}
	return out
}

func callsEdges(modulePath string, pkg *packages.Package, comps map[string]*component,
	byPackage map[string][]*component) []model.Relationship {
	if pkg.TypesInfo == nil {
		return nil
	}
	var out []model.Relationship
	for _, syntax := range pkg.Syntax {
		ast.Inspect(syntax, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			target, ok := calledComponent(pkg, call)
			if !ok {
				return true
			}
			to, external, isComponent := componentIDFor(modulePath, target)
			if !isComponent || (!external && !owns(comps, to)) {
				return true
			}
			if from, owned := edgeOwnership(modulePath, pkg, byPackage, call.Pos()); owned && from != to {
				out = append(out, model.Relationship{FromID: from, ToID: to, Type: model.RelCalls, Weight: 1})
			}
			return true
		})
	}
	return out
}

// calledComponent resolves a call's subject. A method value resolves to its
// receiver type; an interface method therefore lands on the interface itself,
// which is where static analysis stops (spec §6.4).
func calledComponent(pkg *packages.Package, call *ast.CallExpr) (*types.Named, bool) {
	switch fun := astUnparen(call.Fun).(type) {
	case *ast.SelectorExpr:
		if sel := pkg.TypesInfo.Selections[fun]; sel != nil && sel.Kind() != types.FieldVal {
			if named, ok := baseNamed(sel.Recv()); ok {
				return named, true
			}
		}
		if obj := pkg.TypesInfo.Uses[fun.Sel]; obj != nil {
			return receiverNamed(obj)
		}
	case *ast.Ident:
		if obj := pkg.TypesInfo.Uses[fun]; obj != nil {
			return receiverNamed(obj)
		}
	}
	return nil, false
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
