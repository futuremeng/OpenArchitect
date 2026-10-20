package ui

import (
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

// nodeCard is one icon and two text lines. flow rebuilds every card on every
// frame, and the measured baseline is 397 µs for 1000 plain labels: a button,
// a progress bar or a third line inside here trades the map's frame time for
// nothing (spec §2.1, spec §9.3). The second line stays drawn even when its
// text is empty (a System has no package path): uniform structure is what
// makes flow's measurement and layout predictable.
func nodeCard(c *ui.Context, el model.Element) {
	t := c.Theme()
	ui.Row(c).Size(cardWidth, cardHeight).Gap(t.Space(1.5)).AlignItems(ui.Center).
		Padding(t.Space(1.5), t.Space(2)).Radius(flow.CardRadius).
		Background(t.Surface).Border(1, t.Border).Children(func() {
		ui.Icon(c, iconFor(el.Type)).Size(14, 14)
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			components.Text(c, el.Name).SingleLine().FontWeight(600).TextColor(t.Text)
			components.Text(c, truncateMiddle(model.PkgPathOf(el.ID), pkgBudget)).
				SingleLine().FontSize(t.Rem(0.8)).TextColor(t.TextMuted)
		})
	})
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
