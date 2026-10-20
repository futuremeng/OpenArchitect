package ui

import (
	"fmt"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/components/flow"
	"github.com/oneo-me/onevra/source/icons/lucide"
	"github.com/oneo-me/onevra/source/shells/sidebar"
)

func (a *App) graphPage(c *ui.Context) {
	t := c.Theme()
	data := a.flowData()
	ui.Column(c).Fill().Padding(sidebar.HeaderHeight(c)+8, 12, 12, 12).
		Gap(t.Space(2)).Children(func() {
		a.mapHeader(c)
		ui.Row(c).Grow(1).MinHeight(0).Gap(t.Space(3)).Children(func() {
			flow.View(c, &a.flowState, data, a.graphOptions(data)).Grow(1).MinWidth(0)
			if a.selected != "" {
				a.detailPanel(c)
			}
		})
	})
}

// graphOptions is split out of graphPage so a test can take the callbacks:
// flow.View swallows Select and Open into its own hit-testing, which a headless
// tester cannot click through. This is where DoD-2's wiring lives.
func (a *App) graphOptions(data flow.Data) flow.Options {
	return flow.Options{
		Label:     fmt.Sprintf("%d 个元素，%d 条关系", len(data.Nodes), len(data.Edges)),
		EmptyText: "还没有扫描过任何仓库",
		Direction: flow.LeftToRight,
		Selected:  a.selected,
		Select:    a.Select,
		Open:      a.Open,
		NodeGap:   28,
		RankGap:   64,
		Empty:     emptyGraph,
		Overlay:   a.zoomControls,
	}
}

func (a *App) mapHeader(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).FillWidth().Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
		components.Text(c, "架构图").FontWeight(600).TextColor(t.Text)
		ui.Spacer(c)
		// The fold switch (Ruling 1): showComponents defaults to false, so
		// this is the entry point for the Component layer. It binds a
		// frame-local copy so SetShowComponents stays the only write path to
		// the App field (spec §9.2: writes happen in input callbacks), and
		// Fit() is load-bearing: after the user pans, userMoved is true and
		// flow never auto-fits again (flow.go:398), so the unfolded nodes
		// would sit off-screen. The switch needs its own accessible label to
		// be clickable at all: Tester.Find/Click resolve to an element
		// "showing text s, or labeled s" (headless.go:441) and the hit chain
		// only walks up to interactive ancestors (input.go:275), so a
		// caption Text alone is not a target. Label and caption are kept as
		// two different strings per Ruling 1(c), so neither name shadows the
		// other wherever in the build order it lands.
		expanded := a.showComponents
		sw := ui.Switch(c, &expanded).Label("展开组件层")
		if sw.Changed() {
			a.SetShowComponents(expanded)
			a.flowState.Fit()
		}
		components.Text(c, "组件层").FontSize(t.Rem(.85)).TextColor(t.TextMuted)
		// The switch writes a.impactMode itself, so Changed only has to fix up
		// the report that goes with it. Same two-string rule as above.
		if ui.Switch(c, &a.impactMode).Label("影响范围模式").Changed() {
			a.SetImpactMode(a.impactMode)
		}
		components.Text(c, "影响范围").FontSize(t.Rem(.85)).TextColor(t.TextMuted)
		components.Text(c, a.StatusLine()).FontSize(t.Rem(.85)).TextColor(t.TextMuted)
	})
}

func (a *App) zoomControls(c *ui.Context) {
	t := c.Theme()
	row := ui.Row(c).Gap(t.Space(1)).Padding(t.Space(1)).Radius(t.Radius).Background(t.Surface)
	row.Children(func() {
		for _, control := range []struct {
			label string
			icon  func() *ui.SVG
			apply func()
		}{
			{"适应窗口", lucide.Maximize, a.flowState.Fit},
			{"放大", lucide.ZoomIn, a.flowState.ZoomIn},
			{"缩小", lucide.ZoomOut, a.flowState.ZoomOut},
		} {
			button := sidebar.IconButton(c, control.icon(), control.label)
			if button.Clicked() {
				control.apply()
			}
		}
	})
}

func emptyGraph(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Gap(t.Space(2)).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
		components.Icon(c, lucide.Network).Size(28, 28)
		components.Text(c, "还没有扫描过任何仓库").TextColor(t.TextMuted)
		components.Text(c, "在左栏点「扫描」，或按 ⌘K 搜索元素").
			FontSize(t.Rem(.85)).TextColor(t.TextMuted)
	})
}
