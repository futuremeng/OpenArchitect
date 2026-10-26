package ui

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
	"github.com/oneo-me/onevra/source/shells/sidebar"

	"github.com/futuremeng/OpenArchitect/internal/core/graph"
	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

const (
	// detailWidth 是写死的：抽屉一宽，flow 那一侧的可用宽度就每帧不变。
	// 让它随内容伸缩，选中与取消选中会来回重排整张图（同 Task 17 定死
	// 卡片尺寸的理由）。
	detailWidth = float32(320)
	// methodPreview 是折叠时露出的方法数。
	methodPreview = 3
)

func (a *App) detailPanel(c *ui.Context) {
	el, ok := a.view.Element(a.selected)
	if !ok {
		// 重新扫描换了一张图，selected 还指着上一版的节点。这一帧不画，
		// selected 由下一次点击重写。
		return
	}
	t := c.Theme()
	ui.Column(c).Width(detailWidth).MinWidth(detailWidth).FillHeight().
		Padding(t.Space(2)).Gap(t.Space(2)).
		Background(t.Surface).Border(1, t.Border).Radius(t.Radius).
		Children(func() {
			a.detailHeader(c, el)
			ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
				ui.Column(c).FillWidth().Gap(t.Space(2)).Children(func() {
					a.detailFacts(c, el)
					a.detailMethods(c, el)
					a.neighbourGroup(c, "直接上游", a.view.Upstream(el.ID, 1), lucide.ArrowUpRight)
					a.neighbourGroup(c, "直接下游", a.view.Downstream(el.ID, 1), lucide.ArrowDownLeft)
					a.detailImpact(c, el)
				})
			})
		})
}

func (a *App) detailHeader(c *ui.Context, el model.Element) {
	t := c.Theme()
	ui.Row(c).FillWidth().Gap(t.Space(1.5)).AlignItems(ui.Start).Children(func() {
		components.Icon(c, iconFor(el.Type)).Size(18, 18)
		ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space(0.5)).Children(func() {
			components.Text(c, el.Name).FontWeight(600).FontSize(t.Rem(1.05)).
				SingleLine().TextColor(t.Text)
			components.Badge(c, string(el.Type), components.BadgeOptions{Tone: components.BadgeAccent})
		})
		dismiss := sidebar.IconButton(c, lucide.X(), "收起详情")
		// 关抽屉走 Select("")：它顺手把 impact 和 drillInto 一起清掉
		// （Task 16），这里不能只写 selected。
		if dismiss.Clicked() {
			a.Select("")
		}
	})
}

func (a *App) detailFacts(c *ui.Context, el model.Element) {
	t := c.Theme()
	if pkg := model.PkgPathOf(el.ID); pkg != "" {
		detailField(c, t, "包路径", pkg)
	}
	if owners := a.containersOf(el.ID); len(owners) > 0 {
		names := make([]string, len(owners))
		for i, owner := range owners {
			names[i] = owner.Name
		}
		detailField(c, t, "所属 Container", strings.Join(names, "、"))
	}
	a.filePathRow(c, el)
	if el.Description != "" {
		detailField(c, t, "文档注释", el.Description)
	}
	if siblings := len(graph.IntraPackageIDs(a.graph, el.ID)); siblings > 0 {
		detailField(c, t, "同包类型", fmt.Sprintf("%d 个", siblings))
	}
}

func detailField(c *ui.Context, t *ui.Theme, label, value string) {
	ui.Column(c).FillWidth().Gap(t.Space(0.5)).Children(func() {
		components.Text(c, label).FontSize(t.Rem(.75)).TextColor(t.TextMuted)
		components.Text(c, value).FontSize(t.Rem(.9)).TextColor(t.Text)
	})
}

// containersOf 反查 contains 边，取出所有把 id 装在里面的 Container。返回集合
// 而不是第一个，是 §6.3 多归属的可见处（spec §9.4）。顺序来自
// Graph.Relationships()，那个切片按 (from,to,type) 排过序，所以每帧一致。
func (a *App) containersOf(id string) []model.Element {
	var out []model.Element
	for _, rel := range a.view.Relationships() {
		if rel.ToID != id || rel.Type != model.RelContains {
			continue
		}
		owner, ok := a.view.Element(rel.FromID)
		if ok && owner.Type == model.TypeContainer {
			out = append(out, owner)
		}
	}
	return out
}

// previewMethods 折叠时只露 methodPreview 个。expanded 由 drillInto 决定，
// 也就是图上双击（flow 的 Open）或抽屉标题那一下。
func previewMethods(el model.Element, expanded bool) (shown []string, hidden int) {
	if expanded || len(el.Methods) <= methodPreview {
		return el.Methods, 0
	}
	return el.Methods[:methodPreview], len(el.Methods) - methodPreview
}

func (a *App) detailMethods(c *ui.Context, el model.Element) {
	if el.Type != model.TypeComponent || len(el.Methods) == 0 {
		return
	}
	t := c.Theme()
	shown, hidden := previewMethods(el, a.drillInto == el.ID)
	ui.Column(c).FillWidth().Gap(t.Space(0.5)).Children(func() {
		head := components.Text(c, fmt.Sprintf("导出方法（%d）", len(el.Methods))).
			FontSize(t.Rem(.75)).TextColor(t.TextMuted).Cursor(ui.CursorPointer)
		// 点标题展开，再点收回。图上双击走的是同一条路（Task 19 的 Open）。
		if head.Clicked() {
			if a.drillInto == el.ID {
				a.drillInto = ""
			} else {
				a.Open(el.ID)
			}
		}
		for _, method := range shown {
			ui.Row(c).Gap(t.Space(1)).AlignItems(ui.Center).Children(func() {
				components.Icon(c, lucide.Hash).Size(12, 12)
				components.Text(c, method).FontSize(t.Rem(.9)).SingleLine().TextColor(t.Text)
			})
		}
		if hidden > 0 {
			components.Text(c, fmt.Sprintf("还有 %d 个，点标题展开", hidden)).
				FontSize(t.Rem(.8)).TextColor(t.TextMuted)
		}
	})
}

func (a *App) neighbourGroup(c *ui.Context, label string, hits []graph.Hit, icon func() *ui.SVG) {
	if len(hits) == 0 {
		return
	}
	t := c.Theme()
	ui.Column(c).FillWidth().Gap(t.Space(0.5)).Children(func() {
		components.Text(c, fmt.Sprintf("%s（%d）", label, len(hits))).
			FontSize(t.Rem(.75)).TextColor(t.TextMuted)
		ui.Row(c).FillWidth().Wrap().Gap(t.Space(1)).Children(func() {
			for _, hit := range hits {
				chip := ui.Row(c).Gap(t.Space(0.5)).AlignItems(ui.Center).
					Padding(t.Space(0.25), t.Space(0.75)).Radius(t.Radius).Border(1, t.Border)
				chip.Children(func() {
					components.Icon(c, icon).Size(12, 12)
					components.Text(c, hit.Element.Name).FontSize(t.Rem(.85)).SingleLine().TextColor(t.Text)
				})
				// 轮询落在 chip 上，整颗 chip 都是命中区：chip 里的图标和
				// 文本没有 interactive 标志，抢不走这一击（input.go:275）。
				if chip.Clicked() {
					a.jumpTo(hit.Element.ID)
				}
			}
		})
	})
}

// jumpTo 是 chip 的去向：选中它并把图平移过去。Reveal 在图里没有这个节点时
// 什么都不做，所以影响模式下点一颗被淡出的邻居也不会出错。
func (a *App) jumpTo(id string) {
	a.Select(id)
	a.flowState.Reveal(id)
}

// filePathRow 是抽屉里唯一会离开 app 的动作。Task 22 装上 openPath 之前它照画，
// 只是没有手型、点了不动。
func (a *App) filePathRow(c *ui.Context, el model.Element) {
	if el.FilePath == "" {
		return
	}
	t := c.Theme()
	openable := a.openPath != nil
	row := ui.Row(c).Gap(t.Space(1)).AlignItems(ui.Center)
	row.Children(func() {
		components.Icon(c, lucide.FileCode).Size(13, 13)
		path := components.Text(c, el.FilePath).FontSize(t.Rem(.85)).SingleLine()
		if openable {
			path.Cursor(ui.CursorPointer).TextColor(t.Accent)
		} else {
			path.TextColor(t.TextMuted)
		}
	})
	// OpenPath 只在桌面环境拒绝时返回错误，为它存一条状态行要付一个 App 字段，
	// 而本地单用户重试的成本是零，所以这里把错误丢掉。
	if row.Clicked() && openable {
		_ = a.OpenFile(el.FilePath)
	}
}

func (a *App) detailImpact(c *ui.Context, el model.Element) {
	if a.impact == nil || a.impact.Center.ID != el.ID {
		return
	}
	t := c.Theme()
	ui.Column(c).FillWidth().Gap(t.Space(0.5)).Children(func() {
		components.Text(c, "影响范围").FontSize(t.Rem(.75)).TextColor(t.TextMuted)
		components.Text(c, fmt.Sprintf("上游 %d 个，下游 %d 个，深度 %d 跳",
			len(a.impact.Upstream), len(a.impact.Downstream), a.settings.ImpactDepth)).
			FontSize(t.Rem(.9)).TextColor(t.Text)
	})
}
