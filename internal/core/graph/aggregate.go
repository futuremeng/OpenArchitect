package graph

import (
	"fmt"
	"path"
	"strings"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

// Level is the display granularity of the map: like map zoom, going up a
// level trades breadth for detail instead of scaling the same picture.
type Level uint8

const (
	// LevelRepo folds every element into its repository's node.
	LevelRepo Level = iota
	// LevelPackage folds elements into their package's node.
	LevelPackage
	// LevelElement shows the stored elements unchanged.
	LevelElement
)

// PackageGroupID is the pseudo-element id a package node carries at
// LevelPackage. It round-trips through PkgPathOf's sibling helpers here, and
// no scanner ever emits it, so it cannot collide with stored ids.
func PackageGroupID(pkgPath string) string { return "pkg:" + pkgPath }

// Aggregate folds g down to level. Coarser levels keep only the
// depends_on/calls edges (contains is structure *inside* one repo and has
// nothing to say between packages or repos), merge parallel edges between
// the same group pair into one, and let calls outrank depends_on so the
// drawn style reflects the stronger relation.
func Aggregate(g *Graph, level Level) *Graph {
	if level == LevelElement {
		return g
	}
	group := groupByPackage
	if level == LevelRepo {
		group = groupByRepo
	}

	// groupOf maps an element id to its group id; members counts group
	// sizes; keep records the first element that names the group (its Name
	// and Type become the group node's, which is how the System element
	// lends a repo node its repo name).
	groupOf := map[string]string{}
	members := map[string]int{}
	named := map[string]model.Element{}
	for _, el := range g.Elements() {
		gid, ok := group(el)
		if !ok {
			continue
		}
		groupOf[el.ID] = gid
		members[gid]++
		if prev, ok := named[gid]; !ok || (prev.Type != model.TypeSystem && el.Type == model.TypeSystem) {
			named[gid] = el
		}
	}

	var elements []model.Element
	for gid, count := range members {
		name := gid
		if i := strings.LastIndex(gid, "/"); i >= 0 {
			name = gid[i+1:]
		}
		if base, ok := named[gid]; ok && base.Name != "" {
			name = base.Name
		}
		elements = append(elements, model.Element{
			ID:   gid,
			Type: namedType(named[gid]),
			Name: name,
			Description: fmt.Sprintf("%d 个元素", count),
			RepoID: named[gid].RepoID,
		})
	}

	type key struct{ from, to string }
	agg := map[key]*model.Relationship{}
	var order []key
	for _, rel := range g.Relationships() {
		if rel.Type == model.RelContains || rel.FromID == rel.ToID {
			continue
		}
		from, ok1 := groupOf[rel.FromID]
		to, ok2 := groupOf[rel.ToID]
		if !ok1 || !ok2 || from == to {
			continue
		}
		k := key{from, to}
		if cur, ok := agg[k]; ok {
			cur.Weight += max(rel.Weight, 1)
			if rel.Type == model.RelCalls && cur.Type != model.RelCalls {
				cur.Type = model.RelCalls
			}
			continue
		}
		relType := rel.Type
		if relType != model.RelDependsOn && relType != model.RelCalls {
			continue
		}
		weight := max(rel.Weight, 1)
		agg[k] = &model.Relationship{FromID: from, ToID: to, Type: relType, Weight: weight}
		order = append(order, k)
	}
	rels := make([]model.Relationship, 0, len(order))
	for _, k := range order {
		rels = append(rels, *agg[k])
	}
	return Build(elements, rels)
}

// namedType keeps the fold's layer honest: a repo group shows as System, a
// package group as Container.
func namedType(base model.Element) model.ElementType {
	if base.Type == model.TypeSystem {
		return model.TypeSystem
	}
	return model.TypeContainer
}

// groupByPackage keys on the declaring package; the System element has no
// package of its own and drops out — at package level the packages *are*
// the repo.
func groupByPackage(el model.Element) (string, bool) {
	pkg := model.PkgPathOf(el.ID)
	if pkg == "" {
		return "", false
	}
	return PackageGroupID(pkg), true
}

// groupByRepo keys on the owning repo with one canonical group id per repo
// ("sys:"+RepoID); the repo's System element still *names* the group (see
// the named[] pick in Aggregate) so the node carries the repo's human name.
func groupByRepo(el model.Element) (string, bool) {
	if el.RepoID == "" {
		return "", false
	}
	return "sys:" + el.RepoID, true
}

// groupLabel shortens a package group id for display: the last two path
// segments, which is where Go package names carry their meaning.
func GroupLabel(id string) string {
	pkg := strings.TrimPrefix(id, PackageGroupID(""))
	if pkg == "" {
		return id
	}
	return path.Base(path.Dir(pkg)) + "/" + path.Base(pkg)
}
