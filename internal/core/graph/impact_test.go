package graph

import (
	"errors"
	"fmt"
	"testing"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

func TestAnalyzeBothSides(t *testing.T) {
	report, err := Analyze(tinyGraph(), "cmp:b.B", 2)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if report.Center.ID != "cmp:b.B" {
		t.Fatalf("center = %+v", report.Center)
	}
	if len(report.Upstream) != 1 || report.Upstream[0].Element.ID != "cmp:a.A" {
		t.Fatalf("upstream = %+v", report.Upstream)
	}
	if len(report.Downstream) != 1 || report.Downstream[0].Element.ID != "cmp:c.C" {
		t.Fatalf("downstream = %+v", report.Downstream)
	}
	got := fmt.Sprint(report.AffectedIDs())
	if got != fmt.Sprint([]string{"cmp:a.A", "cmp:b.B", "cmp:c.C"}) {
		t.Fatalf("affected = %s", got)
	}
}

func TestAnalyzeUnknownElement(t *testing.T) {
	if _, err := Analyze(tinyGraph(), "cmp:none.Nope", 2); !errors.Is(err, model.ErrUnknownElement) {
		t.Fatalf("err = %v, want model.ErrUnknownElement", err)
	}
}

func TestAnalyzeClampsDepthToOne(t *testing.T) {
	g := tinyGraph()
	zero, err := Analyze(g, "cmp:a.A", 0)
	if err != nil {
		t.Fatal(err)
	}
	one, err := Analyze(g, "cmp:a.A", 1)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(zero.AffectedIDs()) != fmt.Sprint(one.AffectedIDs()) {
		t.Fatalf("maxDepth < 1 must read as 1: %v vs %v", zero.AffectedIDs(), one.AffectedIDs())
	}
}

func TestAffectedIDsSortedAndDeduped(t *testing.T) {
	g := Build([]model.Element{
		{ID: "cmp:a.A", Name: "A"}, {ID: "cmp:b.B", Name: "B"}, {ID: "cmp:c.Center", Name: "Center"},
	}, []model.Relationship{
		{FromID: "cmp:c.Center", ToID: "cmp:a.A", Type: model.RelCalls, Weight: 1},
		{FromID: "cmp:a.A", ToID: "cmp:b.B", Type: model.RelCalls, Weight: 1},
		{FromID: "cmp:b.B", ToID: "cmp:a.A", Type: model.RelCalls, Weight: 1},
	})
	report, err := Analyze(g, "cmp:a.A", 3)
	if err != nil {
		t.Fatal(err)
	}
	// B 既从 A 到（1 跳）又从 Center 到（2 跳），只出现一次。
	got := fmt.Sprint(report.AffectedIDs())
	want := fmt.Sprint([]string{"cmp:a.A", "cmp:b.B", "cmp:c.Center"})
	if got != want {
		t.Fatalf("affected = %s want %s", got, want)
	}
}

// fixture：A 的同包耦合只有 B。D-E 是同包但与中心无关的边（R1：只数
// 与 elementID 相连的边）；ctn:p/a 的包路径与 cmp:p/a.* 相同，靠 contains
// 守卫才能排除（R2：没有这对元素/边，contains 守卫在套件里零覆盖）。
func intraPackageGraph() *Graph {
	return Build([]model.Element{
		{ID: "cmp:p/a.A", Name: "A"}, {ID: "cmp:p/a.B", Name: "B"}, {ID: "cmp:p/b.C", Name: "C"},
		{ID: "cmp:p/a.D", Name: "D"}, {ID: "cmp:p/a.E", Name: "E"},
		{ID: "ctn:p/a", Name: "api", Type: model.TypeContainer},
		{ID: "sys:p", Name: "p", Type: model.TypeSystem},
	}, []model.Relationship{
		{FromID: "cmp:p/a.A", ToID: "cmp:p/a.B", Type: model.RelCalls, Weight: 1},
		{FromID: "cmp:p/a.A", ToID: "cmp:p/b.C", Type: model.RelDependsOn, Weight: 1},
		{FromID: "cmp:p/a.D", ToID: "cmp:p/a.E", Type: model.RelCalls, Weight: 1},
		{FromID: "sys:p", ToID: "cmp:p/a.A", Type: model.RelContains, Weight: 1},
		{FromID: "ctn:p/a", ToID: "cmp:p/a.A", Type: model.RelContains, Weight: 1},
	})
}

func TestIntraPackageIDs(t *testing.T) {
	g := intraPackageGraph()
	got := fmt.Sprint(IntraPackageIDs(g, "cmp:p/a.A"))
	if got != fmt.Sprint([]string{"cmp:p/a.B"}) {
		t.Fatalf("intra-package = %s, want only the same-package sibling touching the centre", got)
	}
	if len(IntraPackageIDs(g, "sys:p")) != 0 {
		t.Fatal("a System has no package of its own")
	}
}
