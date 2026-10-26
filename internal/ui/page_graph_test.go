package ui

import (
	"context"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components/flow"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
	"github.com/futuremeng/OpenArchitect/internal/scanner"
)

func TestGraphOptionsWireCallbacks(t *testing.T) {
	a := newFlowApp(t)
	data := a.flowData()
	opts := a.graphOptions(data)
	target := model.ComponentID("example.com/x/internal/basket", "Basket")

	if opts.Selected != "" || opts.Direction != flow.LeftToRight {
		t.Fatalf("opts = %+v", opts)
	}
	if opts.NodeGap != 28 || opts.RankGap != 64 {
		t.Fatalf("gaps = %v/%v", opts.NodeGap, opts.RankGap)
	}
	if opts.Select == nil || opts.Open == nil || opts.Overlay == nil || opts.Empty == nil {
		t.Fatal("callbacks must all be wired; the map is useless without them")
	}
	opts.Select(target)
	if a.selected != target {
		t.Fatalf("Select did not reach App: %q", a.selected)
	}
	opts.Open(target)
	if a.drillInto != target {
		t.Fatalf("Open did not reach App: %q", a.drillInto)
	}
	opts.Select("")
	if a.selected != "" {
		t.Fatalf("blank select must clear, got %q", a.selected)
	}
	if opts.Label == "" || opts.EmptyText == "" {
		t.Fatalf("label/emptyText = %q/%q", opts.Label, opts.EmptyText)
	}
	// The brief's own tail line here was `len(opts.Selected) == 0 && a.selected != ""`,
	// which is false on every path (a.selected was just cleared above) — an
	// assertion that cannot fail. Rebuilt opts is what the page actually uses.
	if got := a.graphOptions(a.flowData()).Selected; got != "" {
		t.Fatalf("graphOptions must echo a cleared selection, got %q", got)
	}
}

func TestGraphOptionsSelectedEchoesApp(t *testing.T) {
	a := newFlowApp(t)
	target := model.ComponentID("example.com/x/internal/orders", "Orders")
	a.selected = target
	if got := a.graphOptions(a.flowData()).Selected; got != target {
		t.Fatalf("Selected = %q want %q", got, target)
	}
}

// TestGraphPageRendersEmptyState 用 headless tester 起一帧：不开窗口，但真的
// 走了一遍 flow.View，所以 HeaderHeight、Empty 回调、主题取色都被执行过。
func TestGraphPageRendersEmptyState(t *testing.T) {
	a := NewApp(nil)
	tt := ui.NewTester(func(c *ui.Context) { a.graphPage(c) }, 900, 600)
	if !tt.HasText("架构图") {
		t.Fatalf("header missing, texts = %v", tt.Texts())
	}
	if !tt.HasText("还没有扫描过任何仓库") {
		t.Fatalf("empty state missing, texts = %v", tt.Texts())
	}
}

func TestGraphPageRendersNodesAndImpactSwitch(t *testing.T) {
	a := newFlowApp(t)
	tt := ui.NewTester(func(c *ui.Context) { a.graphPage(c) }, 900, 600)
	for _, want := range []string{"Basket", "Orders", "影响范围"} {
		if !tt.HasText(want) {
			t.Fatalf("%q missing, texts = %v", want, tt.Texts())
		}
	}
}

func TestMapHeaderRendersStatusLine(t *testing.T) {
	a := newFlowApp(t)
	a.status = StatusReady
	tt := ui.NewTester(func(c *ui.Context) { a.mapHeader(c) }, 900, 80)
	if !tt.HasText("5 个元素") {
		t.Fatalf("status line missing, texts = %v", tt.Texts())
	}
}

// TestMapHeaderFoldSwitch 钉住 Ruling 1：showComponents 默认 false（spec §12
// 第 4 行），折叠开关是看到 Component 层的入口，所以它必须真的可点、真的走
// SetShowComponents 写回、真的两个方向都改节点集。开关的 accessible label
// （展开组件层）与可见 caption（组件层）是两个不同字符串（Ruling 1(c)）：
// Find/Click 认「showing text s, or labeled s」（headless.go:441），caption
// 那颗 Text 非 interactive、点了不驱动任何东西（input.go:275 只往交互式祖先
// 走）。变异证据：把 label 改成别的串，本测试以「no element shows」红。
func TestMapHeaderFoldSwitch(t *testing.T) {
	a := flowFixtureApp(t) // 默认折叠态
	tt := ui.NewTester(func(c *ui.Context) { a.graphPage(c) }, 900, 600)
	// Find is exact-equality (headless.go:443), so this is the caption and not
	// the switch's own 「展开组件层」 label — HasText("组件层") would be Contains and
	// would stay green with the caption deleted, matched on the label instead.
	if _, ok := tt.Find("组件层"); !ok {
		t.Fatalf("fold caption missing, texts = %v", tt.Texts())
	}
	if tt.HasText("Basket") {
		t.Fatalf("the Container-layer default must not draw components, texts = %v", tt.Texts())
	}
	if err := tt.Click("展开组件层"); err != nil {
		t.Fatal(err)
	}
	if !a.showComponents {
		t.Fatal("clicking the fold switch must reach SetShowComponents")
	}
	tt.Advance(time.Second) // 把第二次点击推出双击窗口
	if err := tt.Click("展开组件层"); err != nil {
		t.Fatal(err)
	}
	if a.showComponents {
		t.Fatal("the fold switch must work in both directions")
	}
}

// TestFoldSwitchRefitsCameraAfterUserZoom 钉住 Ruling 1(b)：userMoved=true
// 之后 flow 永不再自动 fit（flow.go:398），所以展开/折叠必须自己 Fit()，
// 否则刚展开的节点在视野外。900×600 下 5 节点与 2 节点的 fit zoom 都被
// clamp(…, minZoom, 1) 的上界取到 1（render.go:124，layout 宽 < 画布宽，
// 见 fit 公式），而空白处双击把 camera.zoom 乘 2（render.go:235-240），
// 于是「有没有 Fit()」在 Zoom() 上可分辨：有则回到 1，无则停在 2。
func TestFoldSwitchRefitsCameraAfterUserZoom(t *testing.T) {
	a := newFlowApp(t)
	tt := ui.NewTester(func(c *ui.Context) { a.graphPage(c) }, 900, 600)
	tt.Advance(500 * time.Millisecond)
	if !a.flowState.Ready() {
		t.Fatal("flow must be ready inside the tester's settled frames (PoC-4: frame 3)")
	}
	if z := a.flowState.Zoom(); z != 1 {
		t.Fatalf("fit zoom for the expanded fixture at 900x600 clamps to the upper bound 1, got %g", z)
	}
	// 空白处双击才会 zoom*=2：PointerDown 在 `v.hit(x,y) >= 0` 时直接 return false
	// （render.go:208），所以只有点不中节点的路径能走到 render.go:235-240 那臂。
	// 下面 Zoom()==2 那条断言本身就是「这两下没落在任何节点上」的证据。
	tt.ClickAt(850, 550)
	tt.ClickAt(850, 550)
	if z := a.flowState.Zoom(); z != 2 {
		t.Fatalf("the empty-space double click must have doubled the camera zoom, got %g", z)
	}
	tt.Advance(time.Second)
	if err := tt.Click("展开组件层"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if z := a.flowState.Zoom(); z != 1 {
		t.Fatalf("the fold switch must call Fit() (userMoved suppresses flow's auto-fit), zoom = %g", z)
	}
}

// TestMapHeaderImpactSwitch 钉住 Ruling 3：影响范围开关的 Changed() →
// SetImpactMode 这条臂。有 accessible label 才点得到；建帧期间的写入下一帧
// 才可见，所以断言渲染输出前要再跑一帧（Task 18 的同款时序）。
func TestMapHeaderImpactSwitch(t *testing.T) {
	a := newFlowApp(t)
	basket := model.ComponentID("example.com/x/internal/basket", "Basket")
	a.selected = basket
	tt := ui.NewTester(func(c *ui.Context) { a.graphPage(c) }, 900, 600)
	if err := tt.Click("影响范围模式"); err != nil {
		t.Fatal(err)
	}
	if !a.impactMode {
		t.Fatal("clicking the impact switch must reach SetImpactMode")
	}
	if a.impact == nil || a.impact.Center.ID != basket {
		t.Fatalf("mode on with a selection must analyse it: %+v", a.impact)
	}
	tt.Frame()
	if !tt.HasText("上游 2 个，下游 1 个") {
		t.Fatalf("the drawer must render the report one frame later, texts = %v", tt.Texts())
	}
	// 反向：关掉模式要清掉报告。
	tt.Advance(time.Second)
	if err := tt.Click("影响范围模式"); err != nil {
		t.Fatal(err)
	}
	if a.impactMode || a.impact != nil {
		t.Fatalf("off must clear mode and report: %v %+v", a.impactMode, a.impact)
	}
}

// TestZoomControlsDriveTheCamera 逐钮钉住 zoomControls 的 apply 映射：
// 放大 = ×1.25（zoomStep, render.go:22），缩小 = ÷1.25，适应窗口 = Fit()
// 回到被上界 1 夹住的 fit zoom。Zoom() 读的是 camera（目标值），在点击引发
// 的那次 build 里 Changed 臂执行后即刻可断言。
func TestZoomControlsDriveTheCamera(t *testing.T) {
	a := newFlowApp(t)
	tt := ui.NewTester(func(c *ui.Context) { a.graphPage(c) }, 900, 600)
	tt.Advance(500 * time.Millisecond)
	if !a.flowState.Ready() {
		t.Fatal("zoom controls sit over a diagram that must be ready")
	}
	steps := []struct {
		label string
		want  float32
	}{
		{"放大", 1.25},
		{"放大", 1.5625},
		{"缩小", 1.25},
		{"适应窗口", 1},
	}
	for _, step := range steps {
		tt.Advance(time.Second) // 每次点击都从「单击」重新开始计数
		if err := tt.Click(step.label); err != nil {
			t.Fatal(err)
		}
		if z := a.flowState.Zoom(); z != step.want {
			t.Fatalf("after %q zoom = %g, want %g", step.label, z, step.want)
		}
	}
}

// TestGraphNodePositionsDoNotDriftAcrossAdvances 是 spec §11「ui」行的第 4
// 条：节点卡片尺寸恒定（两次 tt.Advance 后 Position(id) 不漂移）。这是它
// 唯一能落地的位置——需要一次真实的 flow.View 布局。Position 返回的是
// 布局坐标（flow.go:109-115）；AnimateTo 只挪元素的屏上位置、从不写
// result（flow.go:204），所以这条断言与相机和动画无关。PoC-4 在 600 节点
// 上实测漂移为 0，这里把同一不变量钉进 UI 套件：layout 的输入（节点 id
// 与顺序、尺寸、间距、边）每帧必须一致。
func TestGraphNodePositionsDoNotDriftAcrossAdvances(t *testing.T) {
	a := newFlowApp(t)
	tt := ui.NewTester(func(c *ui.Context) { a.graphPage(c) }, 900, 600)
	ids := make([]string, 0, 5)
	for _, n := range a.flowData().Nodes {
		ids = append(ids, n.ID)
	}
	if len(ids) != 5 {
		t.Fatalf("expanded fixture must lay out 5 nodes, got %d", len(ids))
	}
	capture := func() [][2]float32 {
		out := make([][2]float32, len(ids))
		for i, id := range ids {
			x, y, ok := a.flowState.Position(id)
			if !ok {
				t.Fatalf("node %s not laid out", id)
			}
			out[i] = [2]float32{x, y}
		}
		return out
	}
	tt.Advance(500 * time.Millisecond)
	first := capture()
	// 就绪且静止后，Advance 本身不产帧：flow 不再 AnimationFrame（build 只在未就绪/
	// 移动/resize 时续帧），所以真正的「再建一次数据」由 Frame 强制。
	// 没有这一步，两次 Position 读的是同一份静止 result，测试恒真。
	tt.Frame()
	tt.Advance(500 * time.Millisecond)
	second := capture()
	for i, id := range ids {
		if first[i] != second[i] {
			t.Fatalf("node %s drifted: %v then %v", id, first[i], second[i])
		}
	}
}

// TestGraphPageMountsDrawerOnlyForSelection 钉住 Ruling 4 的条件挂载：
// 没有选中就没有抽屉，Select 回调写入选中后下一帧挂载（走 graphOptions
// 的 Select，顺带把 DoD-2 的接线在渲染路径上再证一次）。
func TestGraphPageMountsDrawerOnlyForSelection(t *testing.T) {
	a := newFlowApp(t)
	tt := ui.NewTester(func(c *ui.Context) { a.graphPage(c) }, 900, 600)
	if tt.HasText("收起详情") {
		t.Fatalf("no selection means no drawer, texts = %v", tt.Texts())
	}
	a.graphOptions(a.flowData()).Select(model.ComponentID("example.com/x/internal/basket", "Basket"))
	tt.Frame()
	// 钉「包路径」这个抽屉独有的字段标签：newFlowApp 的 fixture 没有指向
	// Basket 的 contains 边，所以 detailFacts 摆出来的是 包路径/同包类型，
	// 而不是 所属 Container。
	for _, want := range []string{"收起详情", "包路径"} {
		if !tt.HasText(want) {
			t.Fatalf("%q missing after Select, texts = %v", want, tt.Texts())
		}
	}
	a.Select("")
	tt.Frame()
	if tt.HasText("收起详情") {
		t.Fatalf("clearing the selection must unmount the drawer, texts = %v", tt.Texts())
	}
}

// TestGraphPageRepoPickerSwitchesGraph 用 headless tester 真点一遍 chips：
// 点仓库名 → 图切到那个仓的最新快照。
func TestGraphPageRepoPickerSwitchesGraph(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	first := seedScan(t, a, fixtureDir("simple-multi-main"))
	second := seedScan(t, a, fixtureDir("shared-lib"))
	if err := a.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(func(c *ui.Context) { a.graphPage(c) }, 900, 600)
	tt.Frame()
	// 仓名来自 go.mod 模块路径，不是目录名——从库里读，别硬编码。
	nameOf := func(report scanner.Report) string {
		for _, r := range a.repos {
			if string(r.ID) == string(report.RepoID) {
				return r.Name
			}
		}
		t.Fatalf("repo %q missing from a.repos", string(report.RepoID))
		return ""
	}
	firstName, secondName := nameOf(first), nameOf(second)
	for _, want := range []string{"最新", firstName, secondName} {
		if !tt.HasText(want) {
			t.Fatalf("picker chip %q missing, texts = %v", want, tt.Texts())
		}
	}
	if err := tt.Click(firstName); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.snapshot.ID != first.SnapshotID {
		t.Fatalf("chip click must switch snapshot: got %q want %q", a.snapshot.ID, first.SnapshotID)
	}
	if len(a.graph.Elements()) != first.Elements {
		t.Fatalf("graph elements = %d want %d", len(a.graph.Elements()), first.Elements)
	}
	if err := tt.Click(secondName); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.snapshot.ID != second.SnapshotID {
		t.Fatalf("second chip click must switch back: got %q want %q", a.snapshot.ID, second.SnapshotID)
	}
}

// TestGraphPageHidesPickerForSingleRepo: 只有一个仓时没有可选项，chips 不画。
func TestGraphPageHidesPickerForSingleRepo(t *testing.T) {
	a := newTestApp(t)
	seedScan(t, a, fixtureDir("simple-multi-main"))
	if err := a.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(func(c *ui.Context) { a.graphPage(c) }, 900, 600)
	tt.Frame()
	if _, ok := tt.Find("最新"); ok {
		t.Fatal("the picker must not render with a single repo")
	}
}

