package growth

import (
	"sort"
	"strings"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

// Diff aligns two adjacent snapshots of the same repo by stable element ID
// (growth spec 切片 B: cmp:/ctn:/fil: IDs are the alignment key) and reports
// element-level change. The returned events carry ElementID and Kind only;
// snapshot bounds and commit attribution are the caller's bookkeeping.
//
// An element whose signature AND edges both changed yields two events —
// losing one of the truths to a kind priority would be the worse lie.
// Output order is deterministic: by element ID, then kind.
func Diff(prev []model.Element, prevRels []model.Relationship,
	cur []model.Element, curRels []model.Relationship) []model.GrowthEvent {
	prevByID := make(map[string]model.Element, len(prev))
	for _, e := range prev {
		prevByID[e.ID] = e
	}
	curByID := make(map[string]model.Element, len(cur))
	for _, e := range cur {
		curByID[e.ID] = e
	}
	prevEdges := edgeSet(prevRels)
	curEdges := edgeSet(curRels)

	var out []model.GrowthEvent
	for id, e := range curByID {
		p, ok := prevByID[id]
		if !ok {
			out = append(out, event(id, model.GrowthAdded))
			continue
		}
		if signature(p) != signature(e) {
			out = append(out, event(id, model.GrowthSignatureChanged))
		}
		if !sameEdges(prevEdges[id], curEdges[id]) {
			out = append(out, event(id, model.GrowthEdgeChanged))
		}
	}
	for id := range prevByID {
		if _, ok := curByID[id]; !ok {
			out = append(out, event(id, model.GrowthRemoved))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ElementID != out[j].ElementID {
			return out[i].ElementID < out[j].ElementID
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}

// signature is what the element *is*: name, description, exported methods.
// Methods compare order-insensitively — a reordered source file is not a
// changed architecture.
func signature(e model.Element) string {
	methods := append([]string(nil), e.Methods...)
	sort.Strings(methods)
	return e.Name + "\x00" + e.Description + "\x00" + strings.Join(methods, ",")
}

// edgeSet indexes every relationship under both endpoint IDs, so "did this
// element's world change" is one set compare per element. Weight is not in
// the key: call-count drift alone is not a structural change.
func edgeSet(rels []model.Relationship) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	add := func(id, key string) {
		if out[id] == nil {
			out[id] = map[string]bool{}
		}
		out[id][key] = true
	}
	for _, r := range rels {
		key := string(r.Type) + ":" + r.FromID + "->" + r.ToID
		add(r.FromID, key)
		if r.ToID != r.FromID {
			add(r.ToID, key)
		}
	}
	return out
}

func sameEdges(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

func event(id, kind string) model.GrowthEvent {
	return model.GrowthEvent{ElementID: id, Kind: kind}
}