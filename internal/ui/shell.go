package ui

import (
	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
	"github.com/oneo-me/onevra/source/shells/sidebar"
	"github.com/oneo-me/onevra/source/theme"
)

// navItem 是侧栏的一行。spec §9.1 固定了三页，所以这张表就是路由表。
type navItem struct {
	label string
	icon  *ui.SVG
	page  Page
}

func navItems() []navItem {
	return []navItem{
		{"架构图", lucide.Network(), PageGraph},
		{"仓库", lucide.Package(), PageRepos},
		{"设置", lucide.Settings(), PageSettings},
	}
}

// NewShell 建原生窗口。调用方必须紧接着装好两条线程通道，再进 App.Run：
//
//	s := a.NewShell()
//	a.SetPost(s.Window().Update)
//	a.SetOpenPath(mygo.Shell.OpenPath)
//
// SetPost 晚一步，扫描 goroutine 的结果就落在 Task 16 那个同步执行的默认
// post 上，等于在主线程之外写 App 字段（纪律 1）。
func (a *App) NewShell() *sidebar.Shell {
	return sidebar.NewShell(sidebar.ShellOptions{
		Title:     "OpenArchitect",
		StateKey:  "main",
		Width:     1180,
		Height:    780,
		MinWidth:  900,
		MinHeight: 600,
		LeftWidth: 220,
		Theme:     theme.Onevra,
		// 三页都走 Content 分支，不用 SetPage：后者的文档写着它会连侧栏控件
		// 一起换掉（shell.go:165-169），而 spec §9.1 要侧栏常驻。
		Left:        a.sidebarNav,
		LeftToolbar: a.scanToolbar,
		Content:     a.content,
	})
}

func (a *App) sidebarNav(c *ui.Context) {
	// Shell 不给任何内缩（shell.go:207-212），侧栏自己躲不开悬浮 header。
	ui.Column(c).FillWidth().Padding(sidebar.HeaderHeight(c), 0, 0, 0).Children(func() {
		side := components.Sidebar(c)
		side.Children(func() {
			for _, item := range navItems() {
				row := components.SidebarItem(c, item.label, item.icon, a.page == item.page)
				if row.Clicked() {
					a.SetPage(item.page)
				}
			}
		})
	})
}

// scanToolbar 建在 Shell 的 ui.Toolbar 里（shell.go:295-298），所以这里只放
// 工具栏的子元素，不自己包行。
func (a *App) scanToolbar(c *ui.Context) {
	t := c.Theme()
	components.Text(c, a.StatusLine()).
		FontSize(t.Rem(.75)).TextColor(t.TextMuted).
		Grow(1).Shrink(1).MinWidth(0).Ellipsis("…")
	scan := sidebar.IconButton(c, lucide.Radar(), "扫描当前仓库")
	scan.Disabled(a.status == StatusScanning)
	if scan.Clicked() {
		if a.settings.ScanRoot == "" {
			a.SetPage(PageSettings)
			return
		}
		_ = a.BeginScan(a.settings.ScanRoot)
	}
}

// content 是页路由器。每一页自己带上 sidebar.HeaderHeight(c) 的顶部内缩，
// 这里不再包一层，否则 header 下面会多出 48 DIP 空白。
func (a *App) content(c *ui.Context) {
	switch a.page {
	case PageRepos:
		a.reposPage(c)
	case PageSettings:
		a.settingsPage(c)
	default:
		// Page 只有三个常量，default 就是 PageGraph。写成 case PageGraph
		// 再加一个走不到的 default 分支是噪音。
		a.graphPage(c)
	}
	// 放在页面之后：overlay 元素挂在窗口级的 overlay 根上（mygo 的
	// coreOverlay → c.overlayRoot()），层级由调用顺序决定，页面内容先建、
	// 面板后建，面板才压在内容之上。
	a.commandPalette(c)
}
