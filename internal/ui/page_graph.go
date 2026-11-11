package ui

import (
	"context"
	"fmt"
	"slices"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/components/flow"
	"github.com/oneo-me/onevra/source/icons/lucide"
	"github.com/oneo-me/onevra/source/shells/sidebar"
)

func (a *App) graphPage(c *ui.Context) {
	t := c.Theme()
	// 快捷键先于 flowData：Esc 清掉的选中要在这一帧就反映到抽屉与淡出。
	a.graphShortcuts(c)
	data := a.flowData()
	ui.Column(c).Fill().Padding(sidebar.HeaderHeight(c)+8, 12, 12, 12).
		Gap(t.Space(2)).Children(func() {
		a.mapHeader(c)
		a.repoPicker(c)
		ui.Row(c).Grow(1).MinHeight(0).Gap(t.Space(3)).Children(func() {
			flow.View(c, &a.flowState, data, a.graphOptions(data)).Grow(1).MinWidth(0)
			if a.selected != "" {
				a.detailPanel(c)
			}
		})
		a.timeScrubber(c)
	})
}

// timeScrubber 是底部时间轴（第二参考图第 5 条语言）：拖动=只看"游标之前"
// 结出的果实与生长——一条轴回放 commit 序，两个故事共用（growth spec 切片
// E）。两条日志都空整条不画；满格=全部（游标归零）。拖动的代价是
// reaggregateFruits 与 regrow 各一次线性扫——不查库、不重扫。
func (a *App) timeScrubber(c *ui.Context) {
	if len(a.fruitLog) == 0 && len(a.growthLog) == 0 {
		return
	}
	lo, hi := a.cursorRange()
	if hi.IsZero() {
		return // 没有任何读得出时间的条目：无跨度可拖
	}
	t := c.Theme()
	frac := 1.0
	if !a.fruitCursor.IsZero() {
		span := hi.Sub(lo)
		if span > 0 {
			frac = float64(a.fruitCursor.Sub(lo)) / float64(span)
		}
	}
	label := "全部"
	if !a.fruitCursor.IsZero() {
		label = a.fruitCursor.Format("2006-01-02")
	}
	ui.Row(c).FillWidth().Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
		components.Text(c, "时间轴").FontSize(t.Rem(.8)).TextColor(t.TextMuted)
		v := frac
		if ui.Slider(c, &v, 0, 1).Label("时间游标").Changed() {
			a.setCursorFrac(v)
		}
		components.Text(c, label).FontSize(t.Rem(.8)).TextColor(t.TextMuted)
	})
}

// graphOptions is split out of graphPage so a test can take the callbacks:
// flow.View swallows Select and Open into its own hit-testing, which a headless
// tester cannot click through. This is where DoD-2's wiring lives.
// OnZoom is the map contract's other half: the wheel, the trackpad scroll
// and the empty-space double-click all change *depth* — the camera's scale
// stays put, so the type size in view never changes. OnZoomAt (the fork's
// pointer-aware variant) wins over it: the depth change anchors on whatever
// node the gesture happened over. A pinch or a ⌘-scroll stays a camera
// zoom: that is the deliberate scale gesture, and it is what feeds the LOD
// stairs in nodeCard.
func (a *App) graphOptions(data flow.Data) flow.Options {
	return flow.Options{
		Label:      fmt.Sprintf("%d 个元素，%d 条关系", len(data.Nodes), len(data.Edges)),
		EmptyText:  "还没有扫描过任何仓库",
		Direction:  flow.LeftToRight,
		Navigation: flow.NavigationMap,
		OnZoom:     a.ZoomDepth,
		OnZoomAt:   a.ZoomDepthAt,
		Selected:   a.selected,
		Select:     a.Select,
		Open:       a.Open,
		NodeGap:    28,
		RankGap:    64,
		Empty:      emptyGraph,
		Overlay:    a.zoomControls,
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
		// 邻域聚焦：选中节点的 1-hop 保持亮、其余淡出，默认开（切片 2
		// 交互复盘 G4）。开关写法照抄影响模式：字段直绑 + Changed 走 setter。
		if ui.Switch(c, &a.neighborFocus).Label("邻域聚焦模式").Changed() {
			a.SetNeighborFocus(a.neighborFocus)
		}
		components.Text(c, "邻域聚焦").FontSize(t.Rem(.85)).TextColor(t.TextMuted)
		// 果实图例（参考图设计语言第 6 条 + 第二图逐 agent 修订）：没有果实不画。
		if len(a.fruitLog) > 0 {
			a.fruitLegend(c)
		}
		// 深度角标：滚轮/双击改的是层级不是尺寸，这里得有一处把当前层
		// 说出来，否则"缩放没放大"会被当成坏了。
		components.Text(c, a.LevelName()).FontSize(t.Rem(.85)).TextColor(t.TextMuted)
		components.Text(c, a.StatusLine()).FontSize(t.Rem(.85)).TextColor(t.TextMuted)
	})
}

// repoPicker 是图页的仓库多选过滤器：默认全选（settings 空），点 chip 在
// 勾选/取消之间切换，"全选"一键回到全选。只有一个仓时不画——没有可选项
// 的选择器只是噪音。chips 用主题边框着色标记勾选态，而不是背景色：accent
// 背景上的文字颜色不在主题契约里。
func (a *App) repoPicker(c *ui.Context) {
	if len(a.repos) < 2 {
		return
	}
	t := c.Theme()
	allSelected := true
	for _, repo := range a.repos {
		id := string(repo.ID)
		if a.repoLatest[id] != "" && !a.selectedRepos[id] {
			allSelected = false
		}
	}
	ui.Row(c).FillWidth().Gap(t.Space(1)).Children(func() {
		all := ui.Button(c, "全选")
		all.Border(1, pick(allSelected, t.Accent, t.Border))
		if all.Clicked() {
			_ = a.SetGraphRepos(context.Background(), nil)
		}
		for _, repo := range a.repos {
			id := string(repo.ID)
			chip := ui.Button(c, repo.Name)
			chip.Border(1, pick(a.selectedRepos[id], t.Accent, t.Border))
			// 没有快照的仓不能选：选了也只是回落，按钮亮着等于邀请用户
			// 去撞一次无效点击。
			if a.repoLatest[id] == "" {
				chip.Disabled(true)
			}
			if chip.Clicked() {
				_ = a.ToggleGraphRepo(context.Background(), id)
			}
		}
	})
}

func pick(cond bool, ifTrue, ifFalse ui.Color) ui.Color {
	if cond {
		return ifTrue
	}
	return ifFalse
}

// fruitLegend 是顶栏的逐 agent 图例（第二参考图第 2 条语言）：每个 agent
// 一颗身份色点 + 名字 + 计数，mixed 橙点收尾。计数来自 reaggregateFruits，
// 时间滑杆拖动后这里跟着变——图例就是游标左侧世界的 census。
func (a *App) fruitLegend(c *ui.Context) {
	t := c.Theme()
	names := make([]string, 0, len(a.fruitByAgent))
	for name := range a.fruitByAgent {
		names = append(names, name)
	}
	slices.Sort(names)
	ui.Row(c).Gap(t.Space(1)).AlignItems(ui.Center).Children(func() {
		for _, name := range names {
			ui.Box(c).Size(7, 7).Radius(4).Background(agentColor(name))
			components.Text(c, name+" "+fmt.Sprint(a.fruitByAgent[name])).
				FontSize(t.Rem(.8)).TextColor(t.TextMuted)
		}
		if a.fruitMixed > 0 {
			ui.Box(c).Size(7, 7).Radius(4).Background(t.Warning)
			components.Text(c, fmt.Sprintf("协作 %d", a.fruitMixed)).
				FontSize(t.Rem(.8)).TextColor(t.TextMuted)
		}
	})
}

// graphShortcuts 是图页交互的键盘形态：Esc 清选中、F 适应窗口、
// ⌘=/⌘− 调缩放（与 overlay 的放大/缩小按钮同一入口）。命令面板打开时
// 不注册——Escape 归对话框，⌘K 由 commandPalette 自己处理。
func (a *App) graphShortcuts(c *ui.Context) {
	if a.commandOpen {
		return
	}
	if c.Shortcut(0, ui.KeyEscape) && a.selected != "" {
		a.Select("")
	}
	if c.Shortcut(0, ui.KeyF) {
		a.flowState.Fit()
	}
	if c.Shortcut(ui.Cmd, ui.KeyEqual) {
		a.flowState.ZoomIn()
	}
	if c.Shortcut(ui.Cmd, ui.KeyMinus) {
		a.flowState.ZoomOut()
	}
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
			// 缩放（scale）与深度是两条通道：滚轮/双击走深度
			// （flow.Options.OnZoom 的落点），这对按钮和 ⌘=/⌘− 走
			// 相机缩放——低倍读不出字时的救生圈（flow 的 ZoomIn/
			// ZoomOut 是公开 API，绕过了 OnZoom 的劫持）。
			{"放大", lucide.ZoomIn, a.flowState.ZoomIn},
			{"缩小", lucide.ZoomOut, a.flowState.ZoomOut},
			{"深入一层", lucide.LayersPlus, func() { a.ZoomDepth(true) }},
			{"上一层", lucide.LayersMinus, func() { a.ZoomDepth(false) }},
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
