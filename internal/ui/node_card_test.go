package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/icons/lucide"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

func TestTruncateMiddle(t *testing.T) {
	cases := []struct{ in string; keep int; want string }{
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
	tt := ui.NewTester(func(c *ui.Context) { nodeCard(c, el) }, 220, 60)
	if !tt.HasText("Basket") {
		t.Fatalf("card must show the type name, texts = %v", tt.Texts())
	}
	if got := len(tt.Texts()); got != 2 {
		t.Fatalf("a card must stay at two text lines, got %d: %v", got, tt.Texts())
	}
}
