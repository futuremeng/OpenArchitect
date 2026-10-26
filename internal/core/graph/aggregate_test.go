package graph

import (
	"slices"
	"testing"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

func twoRepoGraph() *Graph {
	elements := []model.Element{
		{ID: "sys:example.com/app", Type: model.TypeSystem, Name: "app", RepoID: "repo:/app"},
		{ID: "ctn:example.com/app/cmd/api", Type: model.TypeContainer, Name: "api", RepoID: "repo:/app"},
		{ID: "cmp:example.com/app/store.Gateway", Type: model.TypeComponent, Name: "Gateway", RepoID: "repo:/app"},
		{ID: "sys:example.com/lib", Type: model.TypeSystem, Name: "lib", RepoID: "repo:/lib"},
		{ID: "cmp:example.com/lib/store.DB", Type: model.TypeComponent, Name: "DB", RepoID: "repo:/lib"},
	}
	rels := []model.Relationship{
		{FromID: "sys:example.com/app", ToID: "ctn:example.com/app/cmd/api", Type: model.RelContains, Weight: 1},
		{FromID: "ctn:example.com/app/cmd/api", ToID: "cmp:example.com/app/store.Gateway", Type: model.RelContains, Weight: 1},
		{FromID: "cmp:example.com/app/store.Gateway", ToID: "cmp:example.com/lib/store.DB", Type: model.RelDependsOn, Weight: 3},
		{FromID: "cmp:example.com/app/store.Gateway", ToID: "cmp:example.com/lib/store.DB", Type: model.RelCalls, Weight: 5},
	}
	return Build(elements, rels)
}

func nodeIDs(g *Graph) []string {
	var out []string
	for _, e := range g.Elements() {
		out = append(out, e.ID)
	}
	return out
}

func TestAggregatePackageLevel(t *testing.T) {
	g := Aggregate(twoRepoGraph(), LevelPackage)
	want := []string{
		"pkg:example.com/app/cmd/api",
		"pkg:example.com/app/store",
		"pkg:example.com/lib/store",
	}
	if got := nodeIDs(g); !slices.Equal(got, want) {
		t.Fatalf("package nodes =\n%v\nwant\n%v", got, want)
	}
	rels := g.Relationships()
	if len(rels) != 1 {
		t.Fatalf("edges = %+v, want exactly the merged cross-package edge", rels)
	}
	e := rels[0]
	if e.FromID != "pkg:example.com/app/store" || e.ToID != "pkg:example.com/lib/store" {
		t.Fatalf("edge endpoints = %s -> %s", e.FromID, e.ToID)
	}
	// calls 压过 depends_on（强依赖决定画法），权重求和。
	if e.Type != model.RelCalls || e.Weight != 8 {
		t.Fatalf("edge = %+v, want calls weight 8", e)
	}
	if el, _ := g.Element("pkg:example.com/app/store"); el.Description != "1 个元素" {
		t.Fatalf("group description = %q", el.Description)
	}
}

func TestAggregateRepoLevel(t *testing.T) {
	g := Aggregate(twoRepoGraph(), LevelRepo)
	want := []string{"sys:repo:/app", "sys:repo:/lib"}
	if got := nodeIDs(g); !slices.Equal(got, want) {
		t.Fatalf("repo nodes =\n%v\nwant\n%v", got, want)
	}
	rels := g.Relationships()
	if len(rels) != 1 || rels[0].Type != model.RelCalls || rels[0].Weight != 8 {
		t.Fatalf("edges = %+v, want one calls edge weight 8", rels)
	}
	// 组节点借 System 元素的 Name 当仓库名，抽屉里读到的是人话。
	if el, _ := g.Element("sys:repo:/lib"); el.Name != "lib" || el.Type != model.TypeSystem {
		t.Fatalf("repo group = %+v", el)
	}
}

func TestAggregateElementLevelIsIdentity(t *testing.T) {
	g := twoRepoGraph()
	if Aggregate(g, LevelElement) != g {
		t.Fatal("element level must return the graph unchanged")
	}
}

func TestGroupLabel(t *testing.T) {
	if got := GroupLabel("pkg:github.com/futuremeng/OpenLaw/store"); got != "OpenLaw/store" {
		t.Fatalf("GroupLabel = %q", got)
	}
}
