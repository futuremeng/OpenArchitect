package graph

import (
	"slices"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

// ImpactReport is the whole of "change here, what breaks there" in this slice:
// two BFS passes from one center.
type ImpactReport struct {
	Center     model.Element
	Upstream   []Hit
	Downstream []Hit
}

// Analyze reads a maxDepth below 1 as 1, so a garbage config value cannot
// silently produce an empty report that looks like "nothing depends on this".
func Analyze(g *Graph, elementID string, maxDepth int) (ImpactReport, error) {
	center, ok := g.Element(elementID)
	if !ok {
		return ImpactReport{}, model.ErrUnknownElement
	}
	if maxDepth < 1 {
		maxDepth = 1
	}
	return ImpactReport{
		Center:     center,
		Upstream:   g.Upstream(elementID, maxDepth),
		Downstream: g.Downstream(elementID, maxDepth),
	}, nil
}

// AffectedIDs is what the UI turns into flow's Muted flags: the center plus
// everything within depth, sorted, so CLI and GUI output can be compared line
// by line (spec §4.3 DoD-4).
func (r ImpactReport) AffectedIDs() []string {
	set := map[string]bool{r.Center.ID: true}
	for _, hit := range r.Upstream {
		set[hit.Element.ID] = true
	}
	for _, hit := range r.Downstream {
		set[hit.Element.ID] = true
	}
	out := make([]string, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	slices.Sort(out)
	return out
}

// IntraPackageIDs lists same-package neighbours that have an edge to or from
// elementID, excluding hierarchy edges: flow cannot draw those usefully (spec
// §6.4), so the detail panel counts them ("同包类型 N 个") instead of drawing
// them. Centre-incidence only—a package-wide count would show coupling on
// elements that have none. Living here keeps the count out of the UI, where
// it would violate R3.
func IntraPackageIDs(g *Graph, elementID string) []string {
	packagePath := model.PkgPathOf(elementID)
	if packagePath == "" {
		return nil
	}
	out := []string{}
	for _, rel := range g.Relationships() {
		// §6.4 counts coupling as "any drawable edge": a future non-contains
		// relation should be counted here even though bfs refuses to diffuse
		// it (§8.2 allowlist). The two guards disagree on purpose.
		if rel.Type == model.RelContains {
			continue
		}
		if rel.FromID != elementID && rel.ToID != elementID {
			continue
		}
		other := rel.ToID
		if rel.ToID == elementID {
			other = rel.FromID
		}
		if other == elementID || model.PkgPathOf(other) != packagePath {
			continue
		}
		if !slices.Contains(out, other) {
			out = append(out, other)
		}
	}
	slices.Sort(out)
	return out
}
