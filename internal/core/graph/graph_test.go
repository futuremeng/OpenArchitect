package graph

import (
	"errors"
	"testing"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

// tinyGraph: sys:x -contains-> A -calls-> B -depends_on-> C, plus a self loop
// on C. Cycle and self loop are what the BFS termination tests run against.
func tinyGraph() *Graph {
	elements := []model.Element{
		{ID: "cmp:a.A", Name: "A", Type: model.TypeComponent},
		{ID: "cmp:b.B", Name: "B", Type: model.TypeComponent},
		{ID: "cmp:c.C", Name: "C", Type: model.TypeComponent},
		{ID: "sys:x", Name: "x", Type: model.TypeSystem},
	}
	rels := []model.Relationship{
		{FromID: "cmp:a.A", ToID: "cmp:b.B", Type: model.RelCalls, Weight: 2},
		{FromID: "cmp:b.B", ToID: "cmp:c.C", Type: model.RelDependsOn, Weight: 1},
		{FromID: "sys:x", ToID: "cmp:a.A", Type: model.RelContains, Weight: 1},
		{FromID: "cmp:c.C", ToID: "cmp:c.C", Type: model.RelCalls, Weight: 1},
	}
	return Build(elements, rels)
}

func TestBuildSortsAndKeepsEverything(t *testing.T) {
	g := tinyGraph()
	if got := len(g.Elements()); got != 4 {
		t.Fatalf("elements = %d", got)
	}
	rels := g.Relationships()
	if len(rels) != 4 {
		t.Fatalf("relationships = %d", len(rels))
	}
	if rels[0].FromID != "cmp:a.A" {
		t.Fatalf("relationships must sort by from_id, got %q first", rels[0].FromID)
	}
	first := g.Elements()[0]
	if first.ID != "cmp:a.A" {
		t.Fatalf("elements must sort by id, got %q", first.ID)
	}
	if _, ok := g.Element("cmp:a.A"); !ok {
		t.Fatal("Element lookup failed")
	}
}

func TestBuildDropsEdgesWithUnknownEndpoints(t *testing.T) {
	g := Build([]model.Element{{ID: "cmp:a.A", Name: "A"}}, []model.Relationship{
		{FromID: "cmp:a.A", ToID: "cmp:ghost.G", Type: model.RelCalls, Weight: 1},
	})
	if len(g.Relationships()) != 0 {
		t.Fatalf("dangling edge kept: %+v", g.Relationships())
	}
}

func TestNeighborsDirection(t *testing.T) {
	g := tinyGraph()
	if down := names(g.Neighbors("cmp:a.A", DirOut)); len(down) != 1 || down[0] != "B" {
		t.Fatalf("DirOut(A) = %v", down)
	}
	// contains 参与 Neighbors（详情面板要列所属 Container），只是不参与扩散。
	if up := names(g.Neighbors("cmp:a.A", DirIn)); len(up) != 1 || up[0] != "x" {
		t.Fatalf("DirIn(A) = %v", up)
	}
	if both := names(g.Neighbors("cmp:b.B", DirBoth)); len(both) != 2 {
		t.Fatalf("DirBoth(B) = %v", both)
	}
}

func TestMergeIsNotImplemented(t *testing.T) {
	if err := tinyGraph().Merge(tinyGraph()); !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("err = %v", err)
	}
}

func names(elements []model.Element) []string {
	out := make([]string, 0, len(elements))
	for _, e := range elements {
		out = append(out, e.Name)
	}
	return out
}
