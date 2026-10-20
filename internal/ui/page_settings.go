package ui

import (
	"context"
	"strconv"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
	"github.com/oneo-me/onevra/source/shells/sidebar"

	"github.com/futuremeng/OpenArchitect/internal/config"
)

// 深度区间是本切片自定的（计划给的 1…8；spec 没规定上下限）。边界只在下面
// 两颗按钮的 Disabled 上落地：handler 里不再夹一次，config.Load 也不夹
// —— 库里真存了个 999999 也只是「+」灰着、「-」还能用，图遍历那侧有 visited
// 集合、frontier 空了就停（graph/bfs.go:38-41），跑不出第二个节点来。
const (
	minImpactDepth = 1
	maxImpactDepth = 8
)

func (a *App) settingsPage(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Padding(sidebar.HeaderHeight(c)+8, 12, 12, 12).
		Gap(t.Space(3)).Children(func() {
		components.Text(c, "设置").FontWeight(600).FontSize(t.Rem(1.15)).TextColor(t.Text)
		a.scanRootField(c)
		a.depthField(c)
		a.saveRow(c)
	})
}

// scanRootField 绑的是 a.settings.ScanRoot 本身：components.Text 的 Editable
// 每把编辑立刻写进绑定（components/text.go:7-9），而编辑回调在主线程，正好
// 落在 spec §9.2 允许写状态的那两处之一。
//
// 扫描目录是手填的，不是缺功能：mygo v0.5.5 确有原生目录选择器
// （mygo.Dialog.Open，OpenDialogOptions{Directory: true}，dialog.go:24-44/:118），
// 但它阻塞到对话框关闭、且入口先跑 needsApp("Dialog.Open")（dialog.go:119），
// `App.Run` 之前在主线程调用直接 panic（app.go:128-131），headless 测试一条
// 也穿不过去；spec §9.1 只要「扫描」这个动作，没点名选择器。所以这一切片用
// 输入框 + BeginScan 里那次同步的 scanner.Detect 兜底（不是 go 模块就当场
// 失败，不起 goroutine）。选择器本身记在 §13 的后续候选里。
func (a *App) scanRootField(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).FillWidth().Gap(t.Space(1)).Children(func() {
		components.Text(c, "扫描目录").FontSize(t.Rem(.75)).TextColor(t.TextMuted)
		root := components.Text(c, "", components.TextOptions{Editable: &a.settings.ScanRoot})
		root.Label("扫描目录").FontSize(t.Rem(.95)).TextColor(t.Text).
			Padding(t.Space(1)).Radius(t.Radius).Border(1, t.Border).FillWidth().MinWidth(0)
		if root.Changed() {
			a.settingsSaved = false
		}
	})
	ui.Row(c).FillWidth().Gap(t.Space(1)).Children(func() {
		components.Icon(c, lucide.FolderOpen).Size(13, 13)
		components.Text(c, "填仓库根目录的绝对路径（该目录要有 go.mod）。").
			FontSize(t.Rem(.8)).TextColor(t.TextMuted)
	})
}

func (a *App) depthField(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).FillWidth().Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space(0.5)).Children(func() {
			components.Text(c, "影响分析深度").FontSize(t.Rem(.75)).TextColor(t.TextMuted)
			components.Text(c, strconv.Itoa(a.settings.ImpactDepth)).
				FontWeight(600).FontSize(t.Rem(1.05)).TextColor(t.Text)
			components.Text(c, "以选中的元素为中心，向上向下各走几跳（默认 "+
				strconv.Itoa(config.DefaultImpactDepth)+"）。").
				FontSize(t.Rem(.8)).TextColor(t.TextMuted)
		})
		less := sidebar.IconButton(c, lucide.Minus(), "减小影响分析深度")
		less.Disabled(a.settings.ImpactDepth <= minImpactDepth)
		if less.Clicked() {
			a.settings.ImpactDepth--
			a.settingsSaved = false
		}
		more := sidebar.IconButton(c, lucide.Plus(), "加深影响分析")
		more.Disabled(a.settings.ImpactDepth >= maxImpactDepth)
		if more.Clicked() {
			a.settings.ImpactDepth++
			a.settingsSaved = false
		}
	})
}

func (a *App) saveRow(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).FillWidth().Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
		save := components.Button(c, "保存设置", components.ButtonPrimary)
		// 这一句是承重的，不是装饰：SaveSettings → config.Save → d.ConfigSet 在
		// db == nil 时是 nil 指针的方法调用（store/db.go:37-39 里解引用
		// d.handle），会 panic。真窗口永远有库，关掉的只有 NewApp(nil) 那条测试
		// 路径。
		save.Disabled(a.db == nil)
		if save.Clicked() {
			a.settingsErr = a.SaveSettings(context.Background(), a.settings)
			a.settingsSaved = a.settingsErr == nil
		}
		switch {
		case a.settingsErr != nil:
			components.Icon(c, lucide.CircleAlert).Size(14, 14)
			components.Text(c, a.settingsErr.Error()).
				FontSize(t.Rem(.85)).TextColor(t.Danger)
		case a.settingsSaved:
			components.Icon(c, lucide.Check).Size(14, 14)
			components.Text(c, "已保存").FontSize(t.Rem(.85)).TextColor(t.TextMuted)
		}
	})
}
