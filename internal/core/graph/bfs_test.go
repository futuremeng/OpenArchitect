package graph

import (
	"fmt"
	"testing"
	"time"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

func describe(hits []Hit) []string {
	out := make([]string, 0, len(hits))
	for _, h := range hits {
		out = append(out, fmt.Sprintf("%s@%d/%s", h.Element.Name, h.Depth, h.Via))
	}
	return out
}

func TestUpstreamExcludesContainsFromDiffusion(t *testing.T) {
	g := tinyGraph()
	// B 的上游只有 A。sys:x 经 contains 连到 A，若 contains 参与扩散就会在
	// 2 跳处冒出 x，影响范围立刻失真（spec §8.2）。
	if got := describe(g.Upstream("cmp:b.B", 3)); fmt.Sprint(got) != fmt.Sprint([]string{"A@1/calls"}) {
		t.Fatalf("upstream = %v", got)
	}
}

func TestDownstreamDepthStop(t *testing.T) {
	g := tinyGraph()
	if got := describe(g.Downstream("cmp:a.A", 1)); fmt.Sprint(got) != fmt.Sprint([]string{"B@1/calls"}) {
		t.Fatalf("depth 1 = %v", got)
	}
	got := describe(g.Downstream("cmp:a.A", 2))
	want := []string{"B@1/calls", "C@2/depends_on"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("depth 2 = %v", got)
	}
}

func TestSelfLoopTerminates(t *testing.T) {
	g := tinyGraph()
	done := make(chan []Hit, 1)
	go func() { done <- g.Downstream("cmp:c.C", 5) }()
	select {
	case hits := <-done:
		if len(hits) != 0 {
			t.Fatalf("self loop escaped into hits: %+v", hits)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("BFS did not terminate on a self loop")
	}
}

func TestCycleTerminates(t *testing.T) {
	g := Build([]model.Element{
		{ID: "cmp:a.A", Name: "A"}, {ID: "cmp:b.B", Name: "B"},
	}, []model.Relationship{
		{FromID: "cmp:a.A", ToID: "cmp:b.B", Type: model.RelCalls, Weight: 1},
		{FromID: "cmp:b.B", ToID: "cmp:a.A", Type: model.RelCalls, Weight: 1},
	})
	done := make(chan []Hit, 1)
	go func() { done <- g.Downstream("cmp:a.A", 10) }()
	select {
	case hits := <-done:
		if len(hits) != 1 || hits[0].Element.Name != "B" {
			t.Fatalf("hits = %+v", hits)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("BFS did not terminate on a package cycle")
	}
}

func TestResultsSortByDepthThenName(t *testing.T) {
	g := Build([]model.Element{
		{ID: "cmp:z.Z", Name: "Zulu"}, {ID: "cmp:m.M", Name: "Mike"},
		{ID: "cmp:a.A", Name: "Alpha"}, {ID: "cmp:c.Center", Name: "Center"},
	}, []model.Relationship{
		{FromID: "cmp:c.Center", ToID: "cmp:z.Z", Type: model.RelCalls, Weight: 1},
		{FromID: "cmp:c.Center", ToID: "cmp:m.M", Type: model.RelDependsOn, Weight: 1},
		{FromID: "cmp:c.Center", ToID: "cmp:a.A", Type: model.RelCalls, Weight: 1},
	})
	got := describe(g.Downstream("cmp:c.Center", 1))
	want := []string{"Alpha@1/calls", "Mike@1/depends_on", "Zulu@1/calls"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("order = %v want %v", got, want)
	}
}

func TestNonPositiveDepthReturnsNothing(t *testing.T) {
	// clamp 只发生在 Analyze（Task 13），图这一层不加解释。
	g := tinyGraph()
	if got := g.Downstream("cmp:a.A", 0); len(got) != 0 {
		t.Fatalf("depth 0 = %+v", got)
	}
	if got := g.Upstream("cmp:a.A", -1); len(got) != 0 {
		t.Fatalf("negative depth = %+v", got)
	}
}

func TestUnknownStartReturnsNothing(t *testing.T) {
	if got := tinyGraph().Downstream("cmp:none.Nope", 3); len(got) != 0 {
		t.Fatalf("hits = %+v", got)
	}
}
