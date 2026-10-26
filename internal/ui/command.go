package ui

import (
	"context"
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/components/command"
	"github.com/oneo-me/onevra/source/icons/lucide"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

const (
	commandScanID     = "cmd:scan"
	commandReposID    = "cmd:repos"
	commandSettingsID = "cmd:settings"
	// 搜索结果和影响分析指向元素，前缀后面就是 model.Element.ID。
	elementItemPrefix = "element:"
	impactItemPrefix  = "impact:"
)

// commandPalette 由 Task 22 的外壳每帧调用，关闭状态也要跑——
// CommandDialog 的契约是「每帧都调，用宿主保留的 open 和 state」
// （components/command/dialog.go:9-14），所以这里不能写成 if a.commandOpen。
// 关闭态的帧因此不产 items 消费之外的任何开销：commandItems 的门控
// （a.commandOpen）让那时根本不发 store.SearchElements 读取，Escape 保留的
// query（command.go:34）也查不了。打开期间的这次读取是只读查询，代价 bounded
// 在 SQL 的 LIMIT 50 与 (snapshot_id, element_id) 主键前缀扫描里
// （internal/core/store/element.go:24-30）；帧是 demand-driven 的，无人交互
// 时根本没有帧。spec §9.2 禁的是视图写库，这里没有写。
func (a *App) commandPalette(c *ui.Context) {
	if c.Shortcut(ui.Cmd, ui.KeyK) {
		a.SetCommandOpen(true)
	}
	items, err := a.commandItems(a.commandState.Query)
	options := command.Options{
		SearchLabel:  "命令",
		Placeholder:  "搜索元素，或跳到仓库与设置",
		ResultsLabel: "结果",
		Filter:       filterCommands,
		OnActivate:   a.runCommand,
	}
	if err != nil {
		// Options.Error 会替掉结果列表并压制激活（command.go:71-73），
		// 所以库里查不到就是面板里一行字，不是静默的空结果。
		options.Error = err.Error()
	}
	// &a.commandState：command.State 内嵌 components.ListState，明令禁止拷贝
	// （command.go:30），所以它只能是 App 的值字段、只能按地址传。
	command.CommandDialog(c, &a.commandOpen, &a.commandState, items, options,
		components.DialogOptions{Label: "命令面板"})
}

func (a *App) commandItems(query string) ([]command.Item, error) {
	// 三条静态条目对应 spec §9.1 的「扫描、跳到仓库/设置」。没有 ⌘R 之类的
	// Shortcut 文案：command.Item 的 doc 写着「Shortcut is display text; the
	// host registers actual keys」，而本仓库只登记了 ⌘K。
	items := []command.Item{
		{ID: commandScanID, Title: "扫描当前仓库", Group: "命令",
			Icon: lucide.Radar(), Keywords: []string{"scan", "扫描", "rescan"}},
		{ID: commandReposID, Title: "跳到仓库页", Group: "命令",
			Icon: lucide.FolderOpen(), Keywords: []string{"repos", "仓库", "repo"}},
		{ID: commandSettingsID, Title: "打开设置", Group: "命令",
			Icon: lucide.Settings(), Keywords: []string{"settings", "设置", "depth"}},
	}
	if a.selected != "" {
		name := a.selectedName()
		items = append(items, command.Item{
			ID:          impactItemPrefix + a.selected,
			Title:       fmt.Sprintf("以 %s 为中心做影响分析", name),
			Description: name,
			Group:       "分析",
			Icon:        lucide.ScanSearch(),
			Keywords:    []string{"impact", "影响", "analysis"},
		})
	}
	if !a.commandOpen || a.db == nil || a.snapshot.ID == "" || strings.TrimSpace(query) == "" {
		// 关闭态不查库：Escape 关闭会保留 query（command.go:34），而弹窗的构建
		// 回调只在打开时运行（dialog.go:27）。门控写在这里而不是 commandPalette，
		// 是因为关闭时什么都不渲染——只有 commandItems 的返回值能证明这次读取
		// 被跳过了（调用点侧的门控测不出来）。
		return items, nil
	}
	elements, err := a.db.SearchElements(context.Background(), a.snapshot.ID, query)
	if err != nil {
		return items, err
	}
	for _, el := range elements {
		items = append(items, command.Item{
			ID:          elementItemPrefix + el.ID,
			Title:       el.Name,
			Description: model.PkgPathOf(el.ID),
			Group:       "元素",
			Icon:        iconFor(el.Type),
			Keywords:    []string{string(el.Type), model.PkgPathOf(el.ID)},
		})
	}
	return items, nil
}

// selectedName 是 a.graph 里的显示名；selected 可能指向上一版快照的节点，
// 那种情况下退化成 id 本身，面板条目仍然可点（点下去 detailPanel 会自己跳过）。
func (a *App) selectedName() string {
	if el, ok := a.graph.Element(a.selected); ok {
		return el.Name
	}
	return a.selected
}

// filterCommands 分两条路：元素条目是 store.SearchElements 按 query 取回来的，
// 原样保留；静态条目交给包里的 Filter 做标题/关键词匹配。
func filterCommands(items []command.Item, query string) []command.Item {
	if strings.TrimSpace(query) == "" {
		return items
	}
	var static, matched []command.Item
	for _, item := range items {
		if strings.HasPrefix(item.ID, elementItemPrefix) {
			matched = append(matched, item)
			continue
		}
		static = append(static, item)
	}
	return append(command.Filter(static, query), matched...)
}

func (a *App) runCommand(id string) {
	// 关闭和清空是宿主的事（command.go:76-78 的 OnActivate 只给 id）。
	a.SetCommandOpen(false)
	a.commandState.Query = ""
	switch {
	case id == commandScanID:
		if a.scanRoot != "" {
			// ErrScanInFlight 在这里不上屏：面板开着就说明用户刚点过扫描，
			// 侧栏状态行已经是 Scanning 了。
			_ = a.BeginScan(a.scanRoot)
		} else {
			// scanRoot 只在 BeginScan 成功分支里赋值（app.go:89）：一个没配好的
			// 根目录没有可扫的地方，诚实的出口是把用户带到设置 scan.root 的页面
			// （Task 15/21），而不是吞掉激活什么都不做——spec §9.1 只点名了
			// 「扫描当前仓库」这一条。
			a.SetPage(PageSettings)
		}
	case id == commandReposID:
		a.SetPage(PageRepos)
	case id == commandSettingsID:
		a.SetPage(PageSettings)
	case strings.HasPrefix(id, impactItemPrefix):
		target := strings.TrimPrefix(id, impactItemPrefix)
		a.SetPage(PageGraph)
		// 先开模式再选中：Select 只在 impactMode 为真时才算报告。
		a.SetImpactMode(true)
		a.Select(target)
	case strings.HasPrefix(id, elementItemPrefix):
		target := strings.TrimPrefix(id, elementItemPrefix)
		a.SetPage(PageGraph)
		a.Select(target)
		a.flowState.Reveal(target)
	}
}
