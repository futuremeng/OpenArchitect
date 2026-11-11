package growth

import (
	"testing"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

func el(id string, methods ...string) model.Element {
	return model.Element{ID: id, Type: model.TypeComponent, Name: id, Methods: methods}
}

func rel(from, to string, typ model.RelType) model.Relationship {
	return model.Relationship{FromID: from, ToID: to, Type: typ}
}

func kinds(events []model.GrowthEvent) [][2]string {
	out := make([][2]string, len(events))
	for i, e := range events {
		out[i] = [2]string{e.ElementID, e.Kind}
	}
	return out
}

func TestDiffAddedRemoved(t *testing.T) {
	got := Diff([]model.Element{el("cmp:a.Old")}, nil,
		[]model.Element{el("cmp:a.New")}, nil)
	want := [][2]string{{"cmp:a.New", model.GrowthAdded}, {"cmp:a.Old", model.GrowthRemoved}}
	if len(got) != 2 || kinds(got)[0] != want[0] || kinds(got)[1] != want[1] {
		t.Fatalf("kinds = %v, want %v", kinds(got), want)
	}
}

func TestDiffSignature(t *testing.T) {
	prev := []model.Element{el("cmp:a.B", "M1", "M2")}
	// Reordered methods are not a signature change.
	if got := Diff(prev, nil, []model.Element{el("cmp:a.B", "M2", "M1")}, nil); len(got) != 0 {
		t.Fatalf("method reorder produced %v", kinds(got))
	}
	// A new method is.
	got := Diff(prev, nil, []model.Element{el("cmp:a.B", "M1", "M2", "M3")}, nil)
	want := [][2]string{{"cmp:a.B", model.GrowthSignatureChanged}}
	if len(got) != 1 || kinds(got)[0] != want[0] {
		t.Fatalf("kinds = %v, want %v", kinds(got), want)
	}
}

func TestDiffEdges(t *testing.T) {
	elems := []model.Element{el("cmp:a.A"), el("cmp:a.B")}
	// Weight drift alone is not structural.
	prev := []model.Relationship{{FromID: "cmp:a.A", ToID: "cmp:a.B", Type: model.RelCalls, Weight: 1}}
	cur := []model.Relationship{{FromID: "cmp:a.A", ToID: "cmp:a.B", Type: model.RelCalls, Weight: 9}}
	if got := Diff(elems, prev, elems, cur); len(got) != 0 {
		t.Fatalf("weight drift produced %v", kinds(got))
	}
	// A new edge marks both endpoints.
	got := Diff(elems, nil, elems, []model.Relationship{rel("cmp:a.A", "cmp:a.B", model.RelCalls)})
	want := [][2]string{{"cmp:a.A", model.GrowthEdgeChanged}, {"cmp:a.B", model.GrowthEdgeChanged}}
	if len(got) != 2 || kinds(got)[0] != want[0] || kinds(got)[1] != want[1] {
		t.Fatalf("kinds = %v, want %v", kinds(got), want)
	}
}

func TestDiffSignatureAndEdgeBothReported(t *testing.T) {
	prev := []model.Element{el("cmp:a.A"), el("cmp:a.B")}
	cur := []model.Element{el("cmp:a.A", "New"), el("cmp:a.B")}
	got := Diff(prev, nil, cur, []model.Relationship{rel("cmp:a.A", "cmp:a.B", model.RelCalls)})
	want := [][2]string{{"cmp:a.A", model.GrowthEdgeChanged}, {"cmp:a.A", model.GrowthSignatureChanged},
		{"cmp:a.B", model.GrowthEdgeChanged}}
	if len(got) != 3 || kinds(got)[0] != want[0] || kinds(got)[1] != want[1] || kinds(got)[2] != want[2] {
		t.Fatalf("kinds = %v, want %v", kinds(got), want)
	}
}

func TestDiffEmptyIsQuiet(t *testing.T) {
	if got := Diff(nil, nil, nil, nil); len(got) != 0 {
		t.Fatalf("empty snapshots produced %v", kinds(got))
	}
}