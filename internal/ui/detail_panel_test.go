package ui

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/futuremeng/OpenArchitect/internal/core/graph"
	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

const detailModule = "example.com/x"

var detailFile = filepath.Join("internal", "basket", "basket.go")

// newDetailApp 另造一份图，不用 Task 17 的 newFlowApp：抽屉要断言的字段
// （file_path、doc comment、方法列表、多归属）在 flow 数据里一个都用不到，
// 那份 fixture 不带它们。
func newDetailApp(t *testing.T) *App {
	t.Helper()
	pkg := detailModule + "/internal/basket"
	basket := model.Element{
		ID:          model.ComponentID(pkg, "Basket"),
		Type:        model.TypeComponent,
		Name:        "Basket",
		Description: "Basket 管理一次结账的全部条目。",
		FilePath:    detailFile,
		Methods:     []string{"Add", "Remove", "Total", "Clear"},
	}
	item := model.Element{
		ID:       model.ComponentID(pkg, "Item"),
		Type:     model.TypeComponent,
		Name:     "Item",
		FilePath: filepath.Join("internal", "basket", "item.go"),
	}
	orders := model.Element{
		ID:       model.ComponentID(detailModule+"/internal/orders", "Orders"),
		Type:     model.TypeComponent,
		Name:     "Orders",
		FilePath: filepath.Join("internal", "orders", "orders.go"),
	}
	api := model.Element{ID: model.ContainerID(detailModule + "/cmd/api"), Type: model.TypeContainer, Name: "api"}
	worker := model.Element{
		ID:   model.ContainerID(detailModule + "/cmd/worker"),
		Type: model.TypeContainer,
		Name: "worker",
	}
	system := model.Element{ID: model.SystemID(detailModule), Type: model.TypeSystem, Name: "x"}
	rels := []model.Relationship{
		{FromID: system.ID, ToID: api.ID, Type: model.RelContains, Weight: 1},
		{FromID: system.ID, ToID: worker.ID, Type: model.RelContains, Weight: 1},
		// 同一个 Basket 属于两个二进制 —— §6.3 的多归属，抽屉必须整个摆出来。
		{FromID: api.ID, ToID: basket.ID, Type: model.RelContains, Weight: 1},
		{FromID: worker.ID, ToID: basket.ID, Type: model.RelContains, Weight: 1},
		{FromID: basket.ID, ToID: item.ID, Type: model.RelCalls, Weight: 3},
		{FromID: orders.ID, ToID: basket.ID, Type: model.RelDependsOn, Weight: 2},
	}
	a := NewApp(nil)
	a.graph = graph.Build(
		[]model.Element{system, api, worker, basket, item, orders},
		rels,
	)
	a.selected = basket.ID
	return a
}

func elementNames(elements []model.Element) []string {
	out := make([]string, len(elements))
	for i, el := range elements {
		out[i] = el.Name
	}
	return out
}

func newDetailTester(t *testing.T, a *App) *ui.Tester {
	t.Helper()
	tt := ui.NewTester(func(c *ui.Context) { a.detailPanel(c) }, 320, 900)
	tt.Frame()
	return tt
}

func TestPreviewMethods(t *testing.T) {
	el := model.Element{Methods: []string{"Add", "Remove", "Total", "Clear"}}
	if shown, hidden := previewMethods(el, false); len(shown) != methodPreview || hidden != 1 {
		t.Fatalf("collapsed = %v, hidden %d", shown, hidden)
	}
	if shown, hidden := previewMethods(el, true); len(shown) != 4 || hidden != 0 {
		t.Fatalf("expanded = %v, hidden %d", shown, hidden)
	}
	short := model.Element{Methods: []string{"Add"}}
	if shown, hidden := previewMethods(short, false); len(shown) != 1 || hidden != 0 {
		t.Fatalf("a short list hides nothing: %v, hidden %d", shown, hidden)
	}
	if shown, hidden := previewMethods(model.Element{}, false); len(shown) != 0 || hidden != 0 {
		t.Fatalf("no methods: %v, hidden %d", shown, hidden)
	}
}

func TestContainersOfReturnsEveryOwner(t *testing.T) {
	a := newDetailApp(t)
	got := elementNames(a.containersOf(a.selected))
	if len(got) != 2 || got[0] != "api" || got[1] != "worker" {
		t.Fatalf("containers = %v, want both binaries", got)
	}
	if len(a.containersOf(model.SystemID(detailModule))) != 0 {
		t.Fatal("a System has no Container above it")
	}
	if len(a.containersOf("cmp:nowhere.Nope")) != 0 {
		t.Fatal("unknown id is empty, not an error")
	}
}

func TestDetailPanelRendersFacts(t *testing.T) {
	a := newDetailApp(t)
	tt := newDetailTester(t, a)
	for _, want := range []string{
		"Basket", "Component",
		detailModule + "/internal/basket",
		"api、worker",
		detailFile,
		"Basket 管理一次结账的全部条目。",
		"导出方法（4）", "Add", "Total",
		"直接上游", "直接下游", "Orders", "Item",
		"同包类型",
	} {
		if !tt.HasText(want) {
			t.Fatalf("%q missing, texts = %v", want, tt.Texts())
		}
	}
	// 折叠状态下第 4 个方法不该在帧里 —— 它在，就是展开逻辑没接上。
	if tt.HasText("Clear") {
		t.Fatalf("a collapsed list must not render every method, texts = %v", tt.Texts())
	}
}

func TestDetailCloseClickClearsSelection(t *testing.T) {
	a := newDetailApp(t)
	tt := newDetailTester(t, a)
	if err := tt.Click("收起详情"); err != nil {
		t.Fatal(err)
	}
	if a.selected != "" {
		t.Fatalf("close must clear the selection, got %q", a.selected)
	}
}

func TestDetailMethodsHeaderClickExpands(t *testing.T) {
	a := newDetailApp(t)
	tt := newDetailTester(t, a)
	if err := tt.Click("导出方法（4）"); err != nil {
		t.Fatal(err)
	}
	if a.drillInto != a.selected {
		t.Fatalf("drillInto = %q, selected = %q", a.drillInto, a.selected)
	}
	// 这一帧画的还是折叠列表（shown 在点击前算好），所以断言渲染要再跑一帧。
	tt.Frame()
	if !tt.HasText("Clear") {
		t.Fatalf("expanded list must show every method, texts = %v", tt.Texts())
	}
}

func TestNeighbourChipJumpsToThatElement(t *testing.T) {
	a := newDetailApp(t)
	tt := newDetailTester(t, a)
	if err := tt.Click("Orders"); err != nil {
		t.Fatal(err)
	}
	if want := model.ComponentID(detailModule+"/internal/orders", "Orders"); a.selected != want {
		t.Fatalf("selected = %q, want %q", a.selected, want)
	}
}

func TestFilePathRowReachesTheInjectedOpener(t *testing.T) {
	a := newDetailApp(t)
	var opened []string
	a.SetOpenPath(func(path string) error {
		opened = append(opened, path)
		return nil
	})
	tt := newDetailTester(t, a)
	if err := tt.Click(detailFile); err != nil {
		t.Fatal(err)
	}
	if len(opened) != 1 || opened[0] != detailFile {
		t.Fatalf("opened = %v, want [%s]", opened, detailFile)
	}
}

// 只有当前选中元素的路径进抽屉，邻居 chip 只带名字。守的是取数范围：别顺手
// 把整个 snapshot 的 file_path 都渲染出来，那会是几百行。
func TestDrawerShowsOnlyTheSelectedElement(t *testing.T) {
	a := newDetailApp(t)
	tt := newDetailTester(t, a)
	if _, ok := tt.Find("internal/basket/item.go"); ok {
		t.Fatal("a neighbour chip carries a name, not its file path")
	}
}

// TestDetailImpactGroup 钉住 detailImpact 的两条接线：计数只来自
// len(a.impact.Upstream)/len(a.impact.Downstream)，深度数字是
// a.settings.ImpactDepth。ImpactDepth 显式设成非零，否则实现里写死 0 也能被
// 零值蒙混过关。
func TestDetailImpactGroup(t *testing.T) {
	a := newDetailApp(t)
	a.settings.ImpactDepth = 3
	center, ok := a.graph.Element(a.selected)
	if !ok {
		t.Fatal("fixture lost the selected element")
	}
	item, _ := a.graph.Element(model.ComponentID(detailModule+"/internal/basket", "Item"))
	orders, _ := a.graph.Element(model.ComponentID(detailModule+"/internal/orders", "Orders"))
	api, _ := a.graph.Element(model.ContainerID(detailModule + "/cmd/api"))
	a.impact = &graph.ImpactReport{
		Center:     center,
		Upstream:   []graph.Hit{{Element: orders}, {Element: api}},
		Downstream: []graph.Hit{{Element: item}},
	}
	tt := newDetailTester(t, a)
	if !tt.HasText("影响范围") {
		t.Fatalf("a report centred on the selected element must render the impact group, texts = %v", tt.Texts())
	}
	if !tt.HasText("上游 2 个，下游 1 个，深度 3 跳") {
		t.Fatalf("counts must come from the report's hit lists and depth from settings, texts = %v", tt.Texts())
	}
	// 负半：报告中心是别的元素时这一组不画。
	a.impact = &graph.ImpactReport{Center: orders}
	other := newDetailTester(t, a)
	if _, ok := other.Find("影响范围"); ok {
		t.Fatalf("a report centred on another element must not render the group, texts = %v", other.Texts())
	}
}

// TestDetailStaleSelectionRendersNothing 钉住 detailPanel 的选中态失效守卫：
// 重新扫描换图后 selected 可能指着上一版的节点，这一帧抽屉必须整个不画。
func TestDetailStaleSelectionRendersNothing(t *testing.T) {
	a := newDetailApp(t)
	a.selected = model.ComponentID(detailModule+"/internal/gone", "Gone")
	tt := newDetailTester(t, a)
	if texts := tt.Texts(); len(texts) != 0 {
		t.Fatalf("a stale selection must render an empty drawer, texts = %v", texts)
	}
	if _, ok := tt.Find("收起详情"); ok {
		t.Fatal("the drawer header must not exist for a stale selection")
	}
}
