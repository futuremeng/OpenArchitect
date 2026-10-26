package ui

import (
	"context"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
	"github.com/oneo-me/onevra/source/shells/sidebar"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

func (a *App) reposPage(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Padding(sidebar.HeaderHeight(c)+8, 12, 12, 12).
		Gap(t.Space(2)).Children(func() {
		ui.Row(c).FillWidth().Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
			components.Text(c, "仓库").FontWeight(600).FontSize(t.Rem(1.15)).TextColor(t.Text)
			ui.Spacer(c)
			components.Text(c, a.StatusLine()).FontSize(t.Rem(.85)).TextColor(t.TextMuted)
		})
		if len(a.repos) == 0 {
			emptyRepos(c)
			return
		}
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).FillWidth().Gap(t.Space(1)).Children(func() {
				for _, repo := range a.repos {
					a.repoRow(c, repo)
				}
			})
		})
	})
}

// repoRow 画的是 store.Repos 读回的四列里的三列（name、local_path、lang）：
// id 是主键、不是给人读的信息，所以不显示。URL 与分支在 spec §3 偏差表第 9
// 行就被去掉了（本切片不碰 Git），所以这里也不放「最后扫描于」「提交数」这
// 类库里根本没有的字段。
func (a *App) repoRow(c *ui.Context, repo model.Repo) {
	t := c.Theme()
	ui.Row(c).FillWidth().Gap(t.Space(2)).AlignItems(ui.Center).
		Padding(t.Space(1.5)).Radius(t.Radius).Border(1, t.Border).Children(func() {
		components.Icon(c, lucide.Package).Size(18, 18)
		ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space(0.5)).Children(func() {
			components.Text(c, repo.Name).FontWeight(600).TextColor(t.Text).SingleLine()
			components.Text(c, repo.LocalPath).FontSize(t.Rem(.85)).
				TextColor(t.TextMuted).SingleLine()
		})
		components.Text(c, repo.Lang).FontSize(t.Rem(.85)).TextColor(t.TextMuted).SingleLine()
		// 「查看」把图页过滤器收成这一个仓再跳过去——仓库页列出的仓必须
		// 在任何一屏里都点得到，否则列表只是清单。
		if a.repoLatest[string(repo.ID)] != "" {
			view := ui.Button(c, "查看")
			if view.Clicked() {
				if err := a.SetGraphRepos(context.Background(), []string{string(repo.ID)}); err == nil {
					a.SetPage(PageGraph)
				}
			}
		}
		scan := sidebar.IconButton(c, lucide.Radar(), "扫描 "+repo.Name)
		// 扫描中就把这一行的按钮关掉：BeginScan 也会返回 ErrScanInFlight，
		// 但按钮亮着等于邀请用户去撞那个错误。
		scan.Disabled(a.status == StatusScanning)
		if scan.Clicked() {
			_ = a.BeginScan(repo.LocalPath)
		}
	})
}

func emptyRepos(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Grow(1).Gap(t.Space(2)).AlignItems(ui.Center).Justify(ui.Center).
		Children(func() {
			components.Icon(c, lucide.Package).Size(28, 28)
			components.Text(c, "还没有扫描过仓库").TextColor(t.TextMuted)
			// 「扫描当前仓库」这五个字必须和 command.go:62 的条目 Title 同串，
			// 否则这句话把用户带到一个面板里并不存在的位置。
			components.Text(c, "在「设置」里填扫描目录，然后按 ⌘K 选「扫描当前仓库」。").
				FontSize(t.Rem(.85)).TextColor(t.TextMuted)
		})
}
