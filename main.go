package main

import (
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/icons/lucide"
	"github.com/oneo-me/onevra/source/shells/sidebar"
	"github.com/oneo-me/onevra/source/theme"
)

func main() {
	// 名字决定 PathUserData = os.UserConfigDir() + App.Name()（mygo/paths.go）。
	// `go tool mygo dev` 下的可执行文件不叫 OpenArchitect，不叫这一句 GUI 会和
	// oac 各开一个库。
	mygo.App.SetName("OpenArchitect")
	mygo.App.WhenReady(func() {
		shell := sidebar.NewShell(sidebar.ShellOptions{
			Title:     "OpenArchitect",
			StateKey:  "main",
			LeftWidth: 220,
			Theme:     theme.Onevra,
			Left: func(c *ui.Context) {
				ui.Column(c).Fill().Gap(4).Padding(12).Children(func() {
					ui.Row(c).Gap(8).Children(func() {
						ui.Text(c, "架构图")
					})
					ui.Row(c).Gap(8).Children(func() {
						ui.Text(c, "仓库")
					})
					ui.Row(c).Gap(8).Children(func() {
						ui.Text(c, "设置")
					})
				})
			},
			Content: func(c *ui.Context) {
				ui.Column(c).Fill().Children(func() {
					ui.Spacer(c)
					ui.Text(c, "尚未扫描任何仓库").FontSize(14)
					ui.Spacer(c)
				})
			},
		})
		shell.Window().OnClosed(func() { mygo.App.Quit() })
		_ = lucide.Box // 保留图标依赖，Task 17 起真正使用
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
