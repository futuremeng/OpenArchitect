package graph

import (
	"slices"
	"strings"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

// Hit is a reachable element with its hop count and the relation that carried
// us there.
type Hit struct {
	Element model.Element
	Depth   int
	Via     model.RelType
}

// Upstream walks incoming edges (who calls or depends on me); Downstream walks
// outgoing ones. Both stop at maxDepth, and both diffuse only calls and
// depends_on (spec §8.2 allowlist): a contains edge would let one System
// swallow the whole tree inside a hop, and a relation type added in slice 2
// must not start diffusing just because nobody edited bfs.
// IntraPackageIDs (§6.4) keeps the older non-contains rule on purpose—see
// the comment there.
func (g *Graph) Upstream(id string, maxDepth int) []Hit { return g.bfs(id, maxDepth, g.in) }

func (g *Graph) Downstream(id string, maxDepth int) []Hit { return g.bfs(id, maxDepth, g.out) }

// diffuses is the §8.2 allowlist: only these two relation types carry impact.
func diffuses(t model.RelType) bool {
	return t == model.RelCalls || t == model.RelDependsOn
}

func (g *Graph) bfs(start string, maxDepth int, adjacency map[string][]link) []Hit {
	if _, ok := g.elements[start]; !ok || maxDepth < 1 {
		return nil
	}
	visited := map[string]bool{start: true}
	hits := []Hit{}
	frontier := []string{start}
	for depth := 1; depth <= maxDepth && len(frontier) > 0; depth++ {
		var next []string
		for _, current := range frontier {
			for _, l := range adjacency[current] {
				if !diffuses(l.rel.Type) || visited[l.other] {
					continue
				}
				// One visit per node records it at its shallowest hop, which is
				// what makes cycles and self loops terminate.
				visited[l.other] = true
				hits = append(hits, Hit{Element: g.elements[l.other], Depth: depth, Via: l.rel.Type})
				next = append(next, l.other)
			}
		}
		frontier = next
	}
	slices.SortFunc(hits, func(a, b Hit) int {
		if a.Depth != b.Depth {
			return a.Depth - b.Depth
		}
		if c := strings.Compare(a.Element.Name, b.Element.Name); c != 0 {
			return c
		}
		return strings.Compare(a.Element.ID, b.Element.ID)
	})
	return hits
}
