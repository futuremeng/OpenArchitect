package ui

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/components/flow"
	"github.com/oneo-me/onevra/source/icons/lucide"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

// cardWidth/cardHeight fix every node's size. flow measures Content when the
// size is zero and re-runs layout whenever a measured size changes, so
// content-sized cards make the map jitter frame after frame (spec §9.3).
// PoC-4 measured the 600-node frame time with cards of exactly this size
// (Task 4); a different production card would invalidate that number.
const (
	cardWidth  = float32(168)
	cardHeight = float32(46)
)

// pkgBudget is a rune count, not DIPs: the card is a fixed size, so a
// character budget is the honest limit and needs no font measurement.
const pkgBudget = 20

// LOD thresholds (spec §9.5's 分级绘制): the camera's scale decides how much
// of a card is worth drawing. PoC-4 measured the fit zoom of 300/600 nodes at
// 0.1348/0.1 — at that scale two text lines are noise, and drawing them per
// frame per node buys nothing. Below lodDot the card is a dot; between lodDot
// and lodFull it is an icon and a name; at lodFull and above the full card.
const (
	lodDot  = float32(0.5)
	lodFull = float32(0.9)
)

func iconFor(t model.ElementType) *ui.SVG {
	switch t {
	case model.TypeSystem:
		return lucide.Globe()
	case model.TypeContainer:
		return lucide.SquareTerminal()
	default:
		return lucide.Box()
	}
}

// nodeCard is one icon and two text lines — or fewer, by LOD (see the
// thresholds above). flow rebuilds every card on every frame, and the
// measured baseline is 397 µs for 1000 plain labels: a button, a progress bar
// or a third line inside here trades the map's frame time for nothing (spec
// §2.1, spec §9.3). The full-LOD second line stays drawn even when its text
// is empty (a System has no package path): uniform structure is what makes
// flow's measurement and layout predictable. The box is 168×46 at every LOD
// — the layout never moves because the camera moved.
// fruitBadge 是 nodeCard 看到的果实视图：commit 列表（新在前）、条长标尺，
// 以及这一张卡要不要展开成条（分级渲染：只有选中/焦点卡展开，其余单点——
// 600 卡全展开要多画 ~1200 个 Box，超帧时闸门）。
type fruitBadge struct {
	commits  []model.Commit
	maxFiles int
	expand   bool
	// growth 是该元素最近的生长事件（新在前，至多 growthCardLimit 条）。
	growth []model.GrowthEvent
}

func (b fruitBadge) tag() string {
	if len(b.commits) == 0 {
		return ""
	}
	return b.commits[0].AgentTag
}

// agentPalette 是智能体身份色：中间调，深/浅主题都可读；不含橙（mixed 专用）
// 与红（Danger 归保留色，agent 走身份色后红色让给 stale 等状态语义）。
var agentPalette = []string{
	"#3b82f6", // blue
	"#8b5cf6", // violet
	"#14b8a6", // teal
	"#ec4899", // pink
	"#06b6d4", // cyan
	"#84cc16", // lime
}

// agentColor 把 agent 名哈希成稳定身份色——同一个 agent 在任何一帧、任何
// 一台机器上都是同一个颜色，图例才认得出。
func agentColor(name string) ui.Color {
	sum := 0
	for _, r := range name {
		sum += int(r)
	}
	return ui.Hex(agentPalette[sum%len(agentPalette)])
}

// fruitColor 把归因映射成颜色语义（research 2026-11-10 §5 修订）：
// agent:<name>→身份色，mixed→橙；human 或无果实走中性色。
func fruitColor(fruit string, t *ui.Theme) ui.Color {
	switch name, ok := strings.CutPrefix(fruit, "agent:"); {
	case ok:
		return agentColor(name)
	case fruit == "mixed":
		return t.Warning
	default:
		return t.TextMuted
	}
}

// fruitBars 是展开卡的果实清单：≤3 条横条，颜色=归属，长度≈变更规模
// （len(Files) 对全图最大值归一，参考图第 1 条语言）。
func fruitBars(c *ui.Context, badge fruitBadge, t *ui.Theme) {
	ui.Column(c).Gap(2).Children(func() {
		for i, cm := range badge.commits {
			if i == 3 {
				break
			}
			w := float32(16)
			if badge.maxFiles > 0 {
				ratio := float32(len(cm.Files)) / float32(badge.maxFiles)
				if ratio > 1 {
					ratio = 1
				}
				w += 44 * ratio
			}
			ui.Box(c).Size(w, 4).Radius(2).Background(fruitColor(cm.AgentTag, t))
		}
	})
}

// growthColor 把事件类型映射成颜色语义：added 绿（多出来的）、removed 红
// （没了的）、signature-changed 橙（还在但变了）、edge-changed 中性（结构
// 挪了位，不值得点名）。红在这里是"消失"，与 Danger 的失败语义同形不同事。
func growthColor(kind string, t *ui.Theme) ui.Color {
	switch kind {
	case model.GrowthAdded:
		return t.Success
	case model.GrowthRemoved:
		return t.Danger
	case model.GrowthSignatureChanged:
		return t.Warning
	default:
		return t.TextMuted
	}
}

// growthMarks 是节点卡的「生长」区（growth spec 切片 E）：展开卡画最近 ≤3
// 条小方块，折叠态只留最近一条的一颗点——和果实同一套分级渲染，600 卡全
// 展开同样要超帧时闸门。它不新增任何文本行（§9.3 的守门测试盯着）。
func growthMarks(c *ui.Context, events []model.GrowthEvent, expand bool, t *ui.Theme) {
	if len(events) == 0 {
		return
	}
	if !expand {
		ui.Box(c).Size(6, 6).Radius(3).Background(growthColor(events[0].Kind, t))
		return
	}
	ui.Row(c).Gap(2).AlignItems(ui.Center).Children(func() {
		for i, e := range events {
			if i == growthCardLimit {
				break
			}
			ui.Box(c).Size(6, 6).Radius(2).Background(growthColor(e.Kind, t))
		}
	})
}

func nodeCard(c *ui.Context, el model.Element, zoom float32, badge fruitBadge) {
	t := c.Theme()
	box := ui.Row(c).Size(cardWidth, cardHeight).Radius(flow.CardRadius).
		Background(t.Surface).Border(1, t.Border)
	if zoom < lodDot {
		// 点阵档：低倍全景下读不出任何字，画一颗居中的圆点——果实色是
		// 这一档唯一能传达的信息（参考图设计语言第 4 条）。
		dot := fruitColor(badge.tag(), t)
		box.AlignItems(ui.Center).Justify(ui.Center).Children(func() {
			ui.Box(c).Size(10, 10).Radius(5).Background(dot)
		})
		return
	}
	box.Gap(t.Space(1.5)).AlignItems(ui.Center).
		Padding(t.Space(1.5), t.Space(2)).Children(func() {
		ui.Icon(c, iconFor(el.Type)).Size(14, 14)
		text := ui.Column(c).Grow(1).MinWidth(0).Gap(1)
		if zoom < lodFull {
			// 图标档：名字是唯一还能读的东西，包路径这一帧不值得画；
			// 条也读不出长短，展开与否都只画优先点。
			text.Children(func() {
				components.Text(c, el.Name).SingleLine().FontWeight(600).TextColor(t.Text)
			})
			growthMarks(c, badge.growth, false, t)
			if badge.tag() != "" {
				fruitDot(c, badge.tag(), 8)
			}
			return
		}
		text.Children(func() {
			components.Text(c, el.Name).SingleLine().FontWeight(600).TextColor(t.Text)
			components.Text(c, truncateMiddle(model.PkgPathOf(el.ID), pkgBudget)).
				SingleLine().FontSize(t.Rem(0.8)).TextColor(t.TextMuted)
		})
		growthMarks(c, badge.growth, badge.expand, t)
		switch {
		case badge.expand && len(badge.commits) > 0:
			fruitBars(c, badge, t)
		case badge.tag() != "":
			fruitDot(c, badge.tag(), 8)
		}
	})
}

// fruitDot 是卡片右侧的归因圆点。mygo 没有 Stack，角标落在行尾垂直居中——
// 参考图的语义（色点挂卡上）保住，代价是没有真角位的四角选择。
func fruitDot(c *ui.Context, fruit string, size float32) {
	t := c.Theme()
	ui.Box(c).Size(size, size).Radius(size / 2).Background(fruitColor(fruit, t))
}

// truncateMiddle keeps both ends of a package path: the head says which module
// and the tail says which package, and either half alone is unreadable.
func truncateMiddle(s string, keep int) string {
	r := []rune(s)
	if len(r) <= keep {
		return s
	}
	if keep < 5 {
		return "…"
	}
	head := (keep - 1) / 2
	tail := keep - 1 - head
	return string(r[:head]) + "…" + string(r[len(r)-tail:])
}
