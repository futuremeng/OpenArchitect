package ui

import (
	"strings"
	"testing"

	"github.com/oneo-me/onevra/source/components/flow"

	"github.com/futuremeng/OpenArchitect/internal/core/graph"
	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

// flowFixtureApp 不碰数据库：flowData 只读内存里的图。它不调
// SetShowComponents，所以拿到的就是 spec §12 的默认折叠态。
func flowFixtureApp(t *testing.T) *App {
	t.Helper()
	elements := []model.Element{
		{ID: model.SystemID("example.com/x"), Type: model.TypeSystem, Name: "x"},
		{ID: model.ContainerID("example.com/x/cmd/api"), Type: model.TypeContainer, Name: "api"},
		{ID: model.ComponentID("example.com/x/internal/basket", "Basket"), Type: model.TypeComponent, Name: "Basket"},
		{ID: model.ComponentID("example.com/x/internal/basket", "Item"), Type: model.TypeComponent, Name: "Item"},
		{ID: model.ComponentID("example.com/x/internal/orders", "Orders"), Type: model.TypeComponent, Name: "Orders"},
	}
	rels := []model.Relationship{
		{FromID: model.SystemID("example.com/x"), ToID: model.ContainerID("example.com/x/cmd/api"), Type: model.RelContains, Weight: 1},
		{FromID: model.ComponentID("example.com/x/internal/basket", "Basket"), ToID: model.ComponentID("example.com/x/internal/basket", "Item"), Type: model.RelCalls, Weight: 40},
		{FromID: model.ComponentID("example.com/x/internal/orders", "Orders"), ToID: model.ComponentID("example.com/x/internal/basket", "Basket"), Type: model.RelDependsOn, Weight: 2},
		{FromID: model.ContainerID("example.com/x/cmd/api"), ToID: model.ComponentID("example.com/x/internal/basket", "Basket"), Type: model.RelCalls, Weight: 9},
	}
	a := NewApp(nil)
	a.graph = graph.Build(elements, rels)
	return a
}

// newFlowApp 是展开 fixture：这批测试写的时候都看着全部 5 个节点和
// component 级边，所以它显式 opt-in 组件层（默认已是 Container 层）。
func newFlowApp(t *testing.T) *App {
	t.Helper()
	a := flowFixtureApp(t)
	a.SetShowComponents(true)
	return a
}

func TestFlowDataFixesEveryCardSize(t *testing.T) {
	data := newFlowApp(t).flowData()
	if len(data.Nodes) != 5 {
		t.Fatalf("nodes = %d", len(data.Nodes))
	}
	for _, n := range data.Nodes {
		if n.Width != cardWidth || n.Height != cardHeight {
			t.Fatalf("node %s sized %gx%g, must be fixed at %gx%g", n.ID, n.Width, n.Height, cardWidth, cardHeight)
		}
		if n.ID == "" || n.Label == "" || n.Content == nil {
			t.Fatalf("node %+v", n)
		}
	}
}

// TestFlowDataSystemLabel 钉住 ruling 2：System 的 PkgPathOf 是 ""，label 不能
// 拖着一个悬空的「，」分隔符。
func TestFlowDataSystemLabel(t *testing.T) {
	for _, n := range newFlowApp(t).flowData().Nodes {
		if n.ID != model.SystemID("example.com/x") {
			continue
		}
		if strings.HasSuffix(n.Label, "，") {
			t.Fatalf("system label %q ends in a dangling separator", n.Label)
		}
		if n.Label != "x" {
			t.Fatalf("system label = %q, want %q", n.Label, "x")
		}
		return
	}
	t.Fatal("system node missing from flowData")
}

func TestQuietSetNilWithoutImpactMode(t *testing.T) {
	a := newFlowApp(t)
	if a.quietSet() != nil {
		t.Fatal("no impact mode means nothing fades")
	}
	a.impactMode = true
	if a.quietSet() != nil {
		t.Fatal("mode on but no centre yet: still nothing should fade")
	}
	data := a.flowData()
	for _, n := range data.Nodes {
		if n.Muted {
			t.Fatalf("node %s muted with no report", n.ID)
		}
	}
}

func TestFlowDataMutesOutsideImpact(t *testing.T) {
	a := newFlowApp(t)
	center := model.ComponentID("example.com/x/internal/basket", "Basket")
	a.impactMode = true
	a.impact = &graph.ImpactReport{
		Center:     model.Element{ID: center},
		Downstream: []graph.Hit{{Element: model.Element{ID: model.ComponentID("example.com/x/internal/basket", "Item")}, Depth: 1}},
	}
	muted := map[string]bool{}
	for _, n := range a.flowData().Nodes {
		muted[n.ID] = n.Muted
	}
	if muted[center] || muted[model.ComponentID("example.com/x/internal/basket", "Item")] {
		t.Fatalf("the impact set must stay bright: %v", muted)
	}
	if !muted[model.SystemID("example.com/x")] || !muted[model.ContainerID("example.com/x/cmd/api")] {
		t.Fatalf("everything outside the impact set must fade: %v", muted)
	}
}

func TestFlowEdgesStyleAndSkip(t *testing.T) {
	edges := newFlowApp(t).flowEdges()
	// 同包的那条 calls（Basket→Item，都在 internal/basket）不画（spec §6.4）。
	for _, e := range edges {
		if e.Source == model.ComponentID("example.com/x/internal/basket", "Basket") &&
			e.Target == model.ComponentID("example.com/x/internal/basket", "Item") {
			t.Fatalf("same-package edge must not be drawn: %+v", e)
		}
	}
	byPair := map[[2]string]flow.Edge{}
	for _, e := range edges {
		byPair[[2]string{e.Source, e.Target}] = e
	}
	dep := byPair[[2]string{
		model.ComponentID("example.com/x/internal/orders", "Orders"),
		model.ComponentID("example.com/x/internal/basket", "Basket"),
	}]
	if !dep.Dashed || dep.Label != string(model.RelDependsOn) {
		t.Fatalf("depends_on = %+v", dep)
	}
	calls := byPair[[2]string{
		model.ContainerID("example.com/x/cmd/api"),
		model.ComponentID("example.com/x/internal/basket", "Basket"),
	}]
	if calls.Dashed || calls.Width < 2 || calls.Width > 4 {
		t.Fatalf("calls with weight 9 = %+v", calls)
	}
	contains := byPair[[2]string{model.SystemID("example.com/x"), model.ContainerID("example.com/x/cmd/api")}]
	if contains.Label != string(model.RelContains) || contains.Width != 0 {
		t.Fatalf("contains = %+v", contains)
	}
}

// TestFlowEdgesWidthCapsAtFour 单独钉封顶：newFlowApp 里最粗的 calls 是
// weight 9（2.125 DIP），碰不到 4 的上限，删掉夹取那句没有测试会红。
func TestFlowEdgesWidthCapsAtFour(t *testing.T) {
	from := model.ComponentID("example.com/x/internal/orders", "Orders")
	to := model.ComponentID("example.com/x/internal/gift", "Gift")
	a := NewApp(nil)
	a.graph = graph.Build(
		[]model.Element{
			{ID: from, Type: model.TypeComponent, Name: "Orders"},
			{ID: to, Type: model.TypeComponent, Name: "Gift"},
		},
		[]model.Relationship{{FromID: from, ToID: to, Type: model.RelCalls, Weight: 100}},
	)
	edges := a.flowEdges()
	if len(edges) != 1 {
		t.Fatalf("edges = %d, want the single cross-package calls edge", len(edges))
	}
	// 1 + 100/8 = 13.5 是原值，spec §6.4 把它夹到 4 DIP。
	if edges[0].Width != 4 {
		t.Fatalf("calls weight 100 width = %g, want the 4 DIP cap", edges[0].Width)
	}
}

// TestFlowEdgesSkipsSelfLoop 钉住 spec §6.3/§6.4：自环不画，只进详情面板的
// 计数。自环刻意挂在 System 上：它的 PkgPathOf 是 ""，同包跳过（§6.4）罩不住
// 它——删掉 FromID == ToID 那句守卫只有这条测试会红。
func TestFlowEdgesSkipsSelfLoop(t *testing.T) {
	sys := model.SystemID("example.com/x")
	to := model.ContainerID("example.com/x/cmd/api")
	a := NewApp(nil)
	a.graph = graph.Build(
		[]model.Element{
			{ID: sys, Type: model.TypeSystem, Name: "x"},
			{ID: to, Type: model.TypeContainer, Name: "api"},
		},
		[]model.Relationship{
			{FromID: sys, ToID: sys, Type: model.RelCalls, Weight: 5},
			{FromID: sys, ToID: to, Type: model.RelCalls, Weight: 1},
		},
	)
	edges := a.flowEdges()
	for _, e := range edges {
		if e.Source == e.Target {
			t.Fatalf("self-loop must not be drawn: %+v", e)
		}
	}
	if len(edges) != 1 || edges[0].Source != sys || edges[0].Target != to {
		t.Fatalf("the normal edge must survive next to the dropped self-loop, edges = %+v", edges)
	}
}

// TestNewAppDefaultsToContainerLayer 存在的原因：spec §12 门槛表第 4 行在
// PoC-4 量出「可读性差但性能 OK」后规定的分支就是默认值 —— 新 App 不调
// SetShowComponents 时只画 System+Container 层，Component 靠开关或双击下钻
// 进来（300 节点缩放时 168×46 卡片只剩 ~23×6 px）。它刻意绕开 newFlowApp
// （那是展开 fixture），直接 NewApp，否则默认值翻了也没人守。
func TestNewAppDefaultsToContainerLayer(t *testing.T) {
	a := NewApp(nil)
	a.graph = graph.Build(
		[]model.Element{
			{ID: model.SystemID("example.com/x"), Type: model.TypeSystem, Name: "x"},
			{ID: model.ContainerID("example.com/x/cmd/api"), Type: model.TypeContainer, Name: "api"},
			{ID: model.ComponentID("example.com/x/internal/basket", "Basket"), Type: model.TypeComponent, Name: "Basket"},
		},
		[]model.Relationship{
			{FromID: model.SystemID("example.com/x"), ToID: model.ContainerID("example.com/x/cmd/api"), Type: model.RelContains, Weight: 1},
			{FromID: model.ContainerID("example.com/x/cmd/api"), ToID: model.ComponentID("example.com/x/internal/basket", "Basket"), Type: model.RelCalls, Weight: 9},
		},
	)
	data := a.flowData()
	if len(data.Nodes) != 2 {
		t.Fatalf("fresh app nodes = %d, want only System+Container (spec §12 row 4 default)", len(data.Nodes))
	}
	for _, n := range data.Nodes {
		if n.ID != model.SystemID("example.com/x") && n.ID != model.ContainerID("example.com/x/cmd/api") {
			t.Fatalf("Container-layer default emitted node %s", n.ID)
		}
	}
	if len(data.Edges) != 1 || data.Edges[0].Target != model.ContainerID("example.com/x/cmd/api") {
		t.Fatalf("edges touching the folded Component must be dropped by default, got %+v", data.Edges)
	}
}

// TestFlowDataShowComponentsFilter 钉住 ruling 1 的开关契约：折叠态只剩
// System+Container，碰到隐藏节点的边一并丢掉，展开后 5 个节点全部回来；
// flowEdges 本身不过滤。折叠就是默认态，所以它不调 SetShowComponents(false)，
// 由默认折叠 → SetShowComponents(true) 驱动开关。
func TestFlowDataShowComponentsFilter(t *testing.T) {
	a := flowFixtureApp(t)
	if a.showComponents {
		t.Fatal("a fresh app starts folded: the Container-layer default is what spec §12 row 4 mandates")
	}
	data := a.flowData()
	if len(data.Nodes) != 2 {
		t.Fatalf("type-level nodes = %d, want only System+Container", len(data.Nodes))
	}
	visible := map[string]bool{}
	for _, n := range data.Nodes {
		visible[n.ID] = true
	}
	if len(data.Edges) != 1 || !visible[data.Edges[0].Source] || !visible[data.Edges[0].Target] {
		t.Fatalf("type-level edges = %+v, want only the surviving sys→ctn edge", data.Edges)
	}
	if len(a.flowEdges()) != 3 {
		t.Fatalf("flowEdges stays the unfiltered translation, got %d", len(a.flowEdges()))
	}
	a.SetShowComponents(true)
	if got := len(a.flowData().Nodes); got != 5 {
		t.Fatalf("SetShowComponents(true) must restore every node, nodes = %d", got)
	}
}
