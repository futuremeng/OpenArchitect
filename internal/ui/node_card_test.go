package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/icons/lucide"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

func TestTruncateMiddle(t *testing.T) {
	cases := []struct {
		in   string
		keep int
		want string
	}{
		{"Basket", 8, "Basket"},
		{"abcdefghij", 5, "ab…ij"},
		{"example.com/cart/internal/basket", 11, "examp…asket"},
		{"任意长的中文包路径abcdefgh", 5, "任意…gh"},
		{"short", 3, "…"},
	}
	for _, tc := range cases {
		if got := truncateMiddle(tc.in, tc.keep); got != tc.want {
			t.Errorf("truncateMiddle(%q, %d) = %q want %q", tc.in, tc.keep, got, tc.want)
		}
	}
}

// TestIconForEveryType compares pointers, not nil-ness: lucide returns cached
// singletons, so collapsing the switch to one icon leaves a nil check green but
// fails this one.
func TestIconForEveryType(t *testing.T) {
	for _, tc := range []struct {
		tp   model.ElementType
		want *ui.SVG
	}{
		{model.TypeSystem, lucide.Globe()},
		{model.TypeContainer, lucide.SquareTerminal()},
		{model.TypeComponent, lucide.Box()},
	} {
		if got := iconFor(tc.tp); got != tc.want {
			t.Errorf("iconFor(%v) = %p, want the %p singleton", tc.tp, got, tc.want)
		}
	}
}

// TestCardSizeIsPoC4ProbeShape 钉的是 node_card.go 里两个字面量本身，不是
// 常量与 flow.Node 的一致性（那由 TestFlowDataFixesEveryCardSize 把关，改成
// 200 它照样绿）：PoC-4 的 600 节点帧时间就是用 168×46 的合成卡片量出来的
// （Task 4），生产卡片一旦换了形状那个数字就作废。
func TestCardSizeIsPoC4ProbeShape(t *testing.T) {
	w, h := cardWidth, cardHeight
	if w != float32(168) || h != float32(46) {
		t.Fatalf("card size = %gx%g, PoC-4 measured frame time on 168x46 cards", w, h)
	}
}

// TestNodeCardIsTwoTextLines 是 spec §9.3 那条性能约束的守门测试：卡片每帧
// 重建，1000 个 label 已经要 397 µs，第三行文本或任何控件都是拿性能换可读性。
// ui.NewTester 在内存里构建视图，不开窗口。
func TestNodeCardIsTwoTextLines(t *testing.T) {
	el := model.Element{
		ID:   "cmp:example.com/cart/internal/basket.Basket",
		Type: model.TypeComponent,
		Name: "Basket",
	}
	tt := ui.NewTester(func(c *ui.Context) { nodeCard(c, el, 1, fruitBadge{}) }, 220, 60)
	if !tt.HasText("Basket") {
		t.Fatalf("card must show the type name, texts = %v", tt.Texts())
	}
	if got := len(tt.Texts()); got != 2 {
		t.Fatalf("a card must stay at two text lines, got %d: %v", got, tt.Texts())
	}
}

// TestNodeCardLODStairs 钉住 spec §9.5 分级绘制的三个档位：低倍只剩一颗
// 点（0 条文本），中倍只剩名字（1 条），高倍才是名字+包路径（2 条）。
// 阈值两侧各取一点，把 lodDot/lodFull 的边界也钉住。
func TestNodeCardLODStairs(t *testing.T) {
	el := model.Element{
		ID:   "cmp:example.com/cart/internal/basket.Basket",
		Type: model.TypeComponent,
		Name: "Basket",
	}
	for _, tc := range []struct {
		zoom float32
		want int
	}{
		{0.13, 0}, // PoC-4 的 300 节点 fit zoom：只剩点
		{lodDot - 0.01, 0},
		{lodDot, 1},
		{lodFull - 0.01, 1},
		{lodFull, 2},
		{1.5, 2},
	} {
		tt := ui.NewTester(func(c *ui.Context) { nodeCard(c, el, tc.zoom, fruitBadge{}) }, 220, 60)
		if got := len(tt.Texts()); got != tc.want {
			t.Errorf("zoom %g: text lines = %d, want %d: %v", tc.zoom, got, tc.want, tt.Texts())
		}
	}
	if tt := ui.NewTester(func(c *ui.Context) { nodeCard(c, el, lodDot, fruitBadge{}) }, 220, 60); !tt.HasText("Basket") {
		t.Fatalf("the icon level must still name the node, texts = %v", tt.Texts())
	}
}

// TestGrowthMarksAddNoText 钉住生长区的性能契约：事件再多也只画 Box，不
// 新增文本行（§9.3 守门测试的同族）；展开/折叠/图标/点阵四档都构建得出来。
func TestGrowthMarksAddNoText(t *testing.T) {
	el := model.Element{
		ID:   "cmp:example.com/cart/internal/basket.Basket",
		Type: model.TypeComponent,
		Name: "Basket",
	}
	events := []model.GrowthEvent{
		{ElementID: el.ID, Kind: model.GrowthEdgeChanged},
		{ElementID: el.ID, Kind: model.GrowthSignatureChanged},
		{ElementID: el.ID, Kind: model.GrowthAdded},
	}
	for _, tc := range []struct {
		name   string
		zoom   float32
		expand bool
		want   int
	}{
		{"full expanded", lodFull, true, 2},
		{"full collapsed", lodFull, false, 2},
		{"icon", lodDot, false, 1},
		{"dot", 0.13, false, 0},
	} {
		badge := fruitBadge{growth: events, expand: tc.expand}
		tt := ui.NewTester(func(c *ui.Context) { nodeCard(c, el, tc.zoom, badge) }, 220, 60)
		if got := len(tt.Texts()); got != tc.want {
			t.Errorf("%s: text lines = %d, want %d: %v", tc.name, got, tc.want, tt.Texts())
		}
	}
}

// TestGrowthColorKindsDistinct 钉四个 kind 的语义色：added/removed/
// signature-changed 互不相同，edge-changed 与未知 kind 共用中性是设计。
func TestGrowthColorKindsDistinct(t *testing.T) {
	theme := ui.DarkTheme()
	added := growthColor(model.GrowthAdded, theme)
	removed := growthColor(model.GrowthRemoved, theme)
	sig := growthColor(model.GrowthSignatureChanged, theme)
	edge := growthColor(model.GrowthEdgeChanged, theme)
	unknown := growthColor("?", theme)
	if edge != unknown {
		t.Error("edge-changed and unknown kinds must share the neutral color by design")
	}
	if added == removed || added == sig || removed == sig {
		t.Error("added/removed/signature-changed must carry distinct colors")
	}
	if edge == added || edge == removed || edge == sig {
		t.Error("edge-changed must not collide with the named kinds")
	}
}
