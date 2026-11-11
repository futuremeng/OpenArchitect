package ui

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components/flow"

	"github.com/futuremeng/OpenArchitect/internal/core/graph"
	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

// flowData is the whole App-to-flow translation. Elements come out of
// graph.Elements() already sorted by id, so node order — and with it flow's
// keyboard order and PoC-4's position drift check — is stable across frames.
// The per-frame clone/rebuild is accepted, not cached: PoC-4 measured 3.4–3.5
// ms of render work against the 16 ms budget, so memoisation and its
// invalidation points are not worth a slice-1 slot.
func (a *App) flowData() flow.Data {
	elements := a.view.Elements()
	keep := a.quietSet()
	nodes := make([]flow.Node, 0, len(elements))
	// visible is read only in collapsed mode (the default, spec §12 row 4):
	// allocating and filling it when components are opted in would be one
	// wasted map plus n wasted writes per frame. Filtering still happens in
	// exactly one place.
	var visible map[string]bool
	if !a.showComponents {
		visible = make(map[string]bool, len(elements))
	}
	for _, el := range elements {
		// The type-level fold (SetShowComponents): spec §12 row 4 prescribes
		// the Container layer as the default, and PoC-4 measured the fit
		// zoom that triggers that branch — 600 nodes fit at minZoom = .1 and
		// 300 at 0.1348, which shrinks a 168×46 card to about 23×6 px. The
		// geometry is measured; whether the eye can still read it was not
		// (no screenshot path). It is a manual opt-in only — never an
		// auto-collapse — because the readable/unreadable crossover was
		// never measured and any node-count threshold would be a guess.
		if !a.showComponents && el.Type == model.TypeComponent {
			continue
		}
		pkg := model.PkgPathOf(el.ID)
		label := el.Name
		if pkg != "" {
			label += "，" + pkg
		} else if strings.HasPrefix(el.ID, "pkg:") {
			// 包组节点：只画最后一段会把两个仓的 "store" 撞成同名，
			// GroupLabel 给出 "仓名/包名" 这一级可区分的短路径。
			label = graph.GroupLabel(el.ID)
		}
		badge := fruitBadge{
			commits:  a.fruits[el.ID],
			maxFiles: a.fruitMaxFiles,
			growth:   a.growthByEl[el.ID],
			// 分级渲染：选中/焦点卡展开成条，其余单点。600 卡全展开要多画
			// ~1200 个 Box（超 10% 闸门），展开数被选中集大小天然封顶。
			expand: el.ID == a.selected || el.ID == a.focusElement,
		}
		node := flow.Node{
			ID:     el.ID,
			Label:  label,
			Width:  cardWidth,
			Height: cardHeight,
			// LOD 读的是当前相机 scale（flow.State.Zoom 公开可得）：
			// 相机缓动期间 flow 每帧重建卡片，档位跟着换，尺寸不变。
			Content: func(c *ui.Context) { nodeCard(c, el, a.flowState.Zoom(), badge) },
		}
		if keep != nil {
			node.Muted = !keep[el.ID]
		}
		nodes = append(nodes, node)
		if visible != nil {
			visible[el.ID] = true
		}
	}
	edges := a.flowEdges()
	if !a.showComponents {
		kept := edges[:0]
		for _, e := range edges {
			if visible[e.Source] && visible[e.Target] {
				kept = append(kept, e)
			}
		}
		edges = kept
	}
	return flow.Data{Nodes: nodes, Edges: edges}
}

// quietSet is the ids that stay bright this frame, or nil when nothing should
// fade. Computed once per flowData call: doing it per node would run one BFS
// per card. The impact set wins over the neighborhood fade: both answer
// "what matters around here", and the impact report is the wider, explicit
// answer.
func (a *App) quietSet() map[string]bool {
	if a.impactMode {
		if a.impact == nil {
			return nil
		}
		keep := make(map[string]bool, len(a.impact.Upstream)+len(a.impact.Downstream)+1)
		for _, id := range a.impact.AffectedIDs() {
			keep[id] = true
		}
		return keep
	}
	// 邻域聚焦：选中一个节点，它和 1-hop 邻居保持亮，其余淡出——
	// 「改这里会坏哪里」的第一眼答案，不必先打开影响模式。
	if a.neighborFocus && a.selected != "" {
		keep := map[string]bool{a.selected: true}
		for _, hit := range a.view.Upstream(a.selected, 1) {
			keep[hit.Element.ID] = true
		}
		for _, hit := range a.view.Downstream(a.selected, 1) {
			keep[hit.Element.ID] = true
		}
		return keep
	}
	return nil
}

// flowEdges is the unfiltered App→edges translation; the visibility fold
// lives only in flowData (one place, ruling 1).
func (a *App) flowEdges() []flow.Edge {
	rels := a.view.Relationships()
	edges := make([]flow.Edge, 0, len(rels))
	for _, rel := range rels {
		if rel.FromID == rel.ToID {
			continue
		}
		// spec §6.4: 包内耦合存着但不画 —— 分层布局画不出同排节点之间的边。
		if rel.Type != model.RelContains && samePackage(rel.FromID, rel.ToID) {
			continue
		}
		edge := flow.Edge{Source: rel.FromID, Target: rel.ToID, Label: string(rel.Type)}
		switch rel.Type {
		case model.RelDependsOn:
			edge.Dashed = true
		case model.RelCalls:
			// 强依赖更粗，封顶到 4 DIP（1 + min(3, weight/8)）。
			width := 1 + float32(rel.Weight)/8
			if width > 4 {
				width = 4
			}
			edge.Width = width
		}
		edges = append(edges, edge)
	}
	return edges
}

func samePackage(from, to string) bool {
	p := model.PkgPathOf(from)
	return p != "" && p == model.PkgPathOf(to)
}
