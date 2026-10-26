// Package graph holds the in-memory architecture graph: adjacency, neighbour
// queries and impact diffusion. It imports no UI stack (spec §5.1, R1).
package graph

import (
	"errors"
	"slices"
	"strings"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

// ErrNotImplemented marks the slice-2 entry point. Keeping the method here
// means callers do not change shape when multi-repo merge arrives.
var ErrNotImplemented = errors.New("graph: multi-repo merge lands in slice 2")

type Direction uint8

const (
	DirIn Direction = iota
	DirOut
	DirBoth
)

type link struct {
	other string
	rel   model.Relationship
}

type Graph struct {
	elements      map[string]model.Element
	order         []string
	out, in       map[string][]link
	relationships []model.Relationship
}

func Build(elements []model.Element, rels []model.Relationship) *Graph {
	sorted := slices.Clone(elements)
	slices.SortFunc(sorted, func(a, b model.Element) int { return strings.Compare(a.ID, b.ID) })
	g := &Graph{
		elements: make(map[string]model.Element, len(sorted)),
		order:    make([]string, 0, len(sorted)),
		out:      map[string][]link{},
		in:       map[string][]link{},
	}
	for _, e := range sorted {
		g.elements[e.ID] = e
		g.order = append(g.order, e.ID)
	}
	kept := make([]model.Relationship, 0, len(rels))
	for _, rel := range rels {
		if _, ok := g.elements[rel.FromID]; !ok {
			continue
		}
		if _, ok := g.elements[rel.ToID]; !ok {
			continue
		}
		kept = append(kept, rel)
		g.out[rel.FromID] = append(g.out[rel.FromID], link{other: rel.ToID, rel: rel})
		g.in[rel.ToID] = append(g.in[rel.ToID], link{other: rel.FromID, rel: rel})
	}
	slices.SortFunc(kept, compareRel)
	g.relationships = kept
	return g
}

func (g *Graph) Elements() []model.Element {
	out := make([]model.Element, 0, len(g.order))
	for _, id := range g.order {
		out = append(out, g.elements[id])
	}
	return out
}

func (g *Graph) Relationships() []model.Relationship { return slices.Clone(g.relationships) }

func (g *Graph) Element(id string) (model.Element, bool) {
	e, ok := g.elements[id]
	return e, ok
}

func (g *Graph) Neighbors(id string, dir Direction) []model.Element {
	var links []link
	switch dir {
	case DirIn:
		links = g.in[id]
	case DirOut:
		links = g.out[id]
	default:
		// A fresh slice: appending to g.in[id] would write into the
		// adjacency list's spare capacity.
		links = append(append([]link{}, g.in[id]...), g.out[id]...)
	}
	seen := map[string]bool{id: true}
	out := make([]model.Element, 0, len(links))
	for _, l := range links {
		if seen[l.other] {
			continue
		}
		seen[l.other] = true
		out = append(out, g.elements[l.other])
	}
	slices.SortFunc(out, func(a, b model.Element) int {
		if c := strings.Compare(a.Name, b.Name); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
	return out
}

func (g *Graph) Merge(other *Graph) error { return ErrNotImplemented }

func compareRel(a, b model.Relationship) int {
	if c := strings.Compare(a.FromID, b.FromID); c != 0 {
		return c
	}
	if c := strings.Compare(a.ToID, b.ToID); c != 0 {
		return c
	}
	return strings.Compare(string(a.Type), string(b.Type))
}
