package graph

import (
	"errors"
	"fmt"
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

// tinyGraph 的输入恰好已按序，pin 不住 Build 的两次排序；下游一切行级比对
// （golden 等价的 want-list、CLI/GUI、DoD-4）都以这两个顺序为锚点，必须用
// 故意乱序的输入把它钉死（R4）。
func TestBuildSortsDeliberatelyUnsortedInput(t *testing.T) {
	g := Build([]model.Element{
		{ID: "cmp:z.Z", Name: "Z"}, {ID: "cmp:a.A", Name: "A"}, {ID: "cmp:m.M", Name: "M"},
	}, []model.Relationship{
		{FromID: "cmp:z.Z", ToID: "cmp:a.A", Type: model.RelCalls, Weight: 1},
		{FromID: "cmp:a.A", ToID: "cmp:m.M", Type: model.RelDependsOn, Weight: 1},
		{FromID: "cmp:a.A", ToID: "cmp:m.M", Type: model.RelCalls, Weight: 1},
		{FromID: "cmp:m.M", ToID: "cmp:a.A", Type: model.RelCalls, Weight: 1},
		{FromID: "cmp:a.A", ToID: "cmp:z.Z", Type: model.RelDependsOn, Weight: 1},
	})
	var ids []string
	for _, e := range g.Elements() {
		ids = append(ids, e.ID)
	}
	if fmt.Sprint(ids) != fmt.Sprint([]string{"cmp:a.A", "cmp:m.M", "cmp:z.Z"}) {
		t.Fatalf("Elements() must sort by id, got %v", ids)
	}
	var keys []string
	for _, r := range g.Relationships() {
		keys = append(keys, fmt.Sprintf("%s>%s:%s", r.FromID, r.ToID, r.Type))
	}
	want := []string{
		"cmp:a.A>cmp:m.M:calls",
		"cmp:a.A>cmp:m.M:depends_on",
		"cmp:a.A>cmp:z.Z:depends_on",
		"cmp:m.M>cmp:a.A:calls",
		"cmp:z.Z>cmp:a.A:calls",
	}
	if fmt.Sprint(keys) != fmt.Sprint(want) {
		t.Fatalf("Relationships() must sort by (from,to,type), got %v", keys)
	}
}

// R3：spec §8.2 的扩散白名单——「参与扩散的边类型只有 calls 与 depends_on」。
// 黑名单版（== contains 才跳过）会放行 implements，这个测试就是白名单的证据。
func TestBfsDiffusesOnlyCallsAndDependsOn(t *testing.T) {
	g := Build([]model.Element{
		{ID: "cmp:a.A", Name: "A"}, {ID: "cmp:b.B", Name: "B"},
	}, []model.Relationship{
		{FromID: "cmp:a.A", ToID: "cmp:b.B", Type: model.RelType("implements"), Weight: 1},
	})
	if hits := g.Downstream("cmp:a.A", 3); len(hits) != 0 {
		t.Fatalf("implements edge must not diffuse downstream: %+v", hits)
	}
	if hits := g.Upstream("cmp:b.B", 3); len(hits) != 0 {
		t.Fatalf("implements edge must not diffuse upstream: %+v", hits)
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
