package ui

import (
	"context"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components/command"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

func commandIDs(items []command.Item) []string {
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = item.ID
	}
	return out
}

func TestCommandItemsWithoutSnapshotAreStaticOnly(t *testing.T) {
	a := NewApp(nil)
	items, err := a.commandItems("Basket")
	if err != nil {
		t.Fatal(err)
	}
	// 没扫过仓库就没有可搜索的快照，静态条目照常给——面板不是空图时才出现
	// 的东西，它恰恰是这时候的入口。spec §9.1 的「跳到仓库/设置」是三条：
	// 扫描、仓库页、设置页，顺序就是面板里的显示顺序。
	if len(items) != 3 ||
		items[0].ID != commandScanID || items[1].ID != commandReposID || items[2].ID != commandSettingsID {
		t.Fatalf("items = %v", commandIDs(items))
	}
	for _, item := range items {
		if item.Title == "" || item.Group == "" {
			t.Fatalf("item %+v must be displayable", item)
		}
	}
}

func TestCommandItemsAddsSelectedElementToImpact(t *testing.T) {
	a := newFlowApp(t)
	target := model.ComponentID("example.com/x/internal/orders", "Orders")
	a.selected = target
	items, err := a.commandItems("")
	if err != nil {
		t.Fatal(err)
	}
	// 影响分析条目指向"当前选中的那个"，所以它只在有选中时出现，并且排在
	// 三条静态条目之后（静态在前是 commandItems 的固定顺序）。
	if len(items) != 4 || items[3].ID != impactItemPrefix+target {
		t.Fatalf("items = %v", commandIDs(items))
	}
	if items[3].Title == "" {
		t.Fatal("the impact item must name its center")
	}
}

func TestCommandItemsSearchesCurrentSnapshot(t *testing.T) {
	a := newTestApp(t)
	seedScan(t, a, fixtureDir("simple-multi-main"))
	if err := a.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	items, err := a.commandItems("Cart")
	if err != nil {
		t.Fatal(err)
	}
	// fixture 的 module 是 example.com/simple（Task 9），里面的导出类型只有 Cart。
	want := elementItemPrefix + model.ComponentID("example.com/simple/internal/store", "Cart")
	var found bool
	for _, item := range items {
		if item.ID == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("search missed Cart, items = %v", commandIDs(items))
	}
}

func TestFilterCommandsLeavesDatabaseMatchesAlone(t *testing.T) {
	items := []command.Item{
		{ID: commandScanID, Title: "扫描当前仓库", Keywords: []string{"scan"}},
		{ID: commandSettingsID, Title: "打开设置", Keywords: []string{"settings"}},
		{ID: elementItemPrefix + "cmp:a.Basket", Title: "Basket"},
	}
	// 库里已经按 query 筛过元素条目；再套一遍 command.Filter 的分词，
	// "bas 2" 这类查询会把命中清空。
	got := filterCommands(items, "bas 2")
	if len(got) != 1 || got[0].ID != elementItemPrefix+"cmp:a.Basket" {
		t.Fatalf("element matches must survive: %v", commandIDs(got))
	}
	// 这条是「静态条目真的过 Filter，元素条目不再过一遍」的钉子：
	// "scan" 命中扫描条目的关键词，加上 Basket 共 2 条，顺序是静态在前。
	// 若实现把 command.Filter 套在全部条目上，Basket 没有 "scan" 关键词会被滤掉，
	// 这里就只剩 1 条。
	got = filterCommands(items, "scan")
	if len(got) != 2 || got[0].ID != commandScanID || got[1].ID != elementItemPrefix+"cmp:a.Basket" {
		t.Fatalf("statics filtered, elements kept: %v", commandIDs(got))
	}
	if got := filterCommands(items, ""); len(got) != 3 {
		t.Fatalf("empty query keeps everything: %v", commandIDs(got))
	}
}

func TestRunCommandElementSelectsAndReveals(t *testing.T) {
	a := newFlowApp(t)
	target := model.ComponentID("example.com/x/internal/orders", "Orders")
	a.SetPage(PageRepos)
	a.commandState.Query = "Orders"
	a.runCommand(elementItemPrefix + target)
	if a.commandOpen {
		t.Fatal("activating must close the panel")
	}
	if a.page != PageGraph || a.selected != target {
		t.Fatalf("page=%q selected=%q", a.page, a.selected)
	}
	if a.commandState.Query != "" {
		t.Fatalf("query must reset after activation: %q", a.commandState.Query)
	}
}

func TestRunCommandImpactTurnsModeOnBeforeSelecting(t *testing.T) {
	a := newFlowApp(t)
	target := model.ComponentID("example.com/x/internal/basket", "Basket")
	a.SetImpactMode(false)
	a.runCommand(impactItemPrefix + target)
	// 顺序有意义：Select 只在 impactMode 已开时才计算报告（Task 16）。
	if !a.impactMode || a.selected != target || a.impact == nil {
		t.Fatalf("mode=%v selected=%q impact=%v", a.impactMode, a.selected, a.impact)
	}
	if a.impact.Center.ID != target {
		t.Fatalf("center = %q", a.impact.Center.ID)
	}
}

// TestCommandRunCommandStaticIDsRoutePages 钉住两个静态 id 的落点：
// commandReposID 只有这一条覆盖（仓库页在侧栏导航上始终可见，面板条目本身
// 在 GUI 之外没法点），commandSettingsID 同理。
func TestCommandRunCommandStaticIDsRoutePages(t *testing.T) {
	a := newFlowApp(t)
	a.SetPage(PageGraph)
	a.runCommand(commandReposID)
	if a.page != PageRepos {
		t.Fatalf("runCommand(%q) page = %q, want %q", commandReposID, a.page, PageRepos)
	}
	if a.commandOpen {
		t.Fatal("activating must close the panel")
	}
	a.runCommand(commandSettingsID)
	if a.page != PageSettings {
		t.Fatalf("runCommand(%q) page = %q, want %q", commandSettingsID, a.page, PageSettings)
	}
}

func TestRunCommandUnknownIDIsNoop(t *testing.T) {
	a := newFlowApp(t)
	a.SetPage(PageRepos)
	a.runCommand("cmd:whatever")
	if a.page != PageRepos || a.selected != "" {
		t.Fatalf("page=%q selected=%q", a.page, a.selected)
	}
}

func TestCommandShortcutOpensPanel(t *testing.T) {
	a := NewApp(nil)
	tt := ui.NewTester(func(c *ui.Context) { a.commandPalette(c) }, 900, 700)
	// 这一帧是简报指定的：窗口快捷键在构建时登记（input.go:856-857 把 Cmd+K
	// 推进 rt.nextRegs），先跑一帧才让「登记先于按键」这条顺序显式成立。
	// 实测记一笔：删掉它本测试仍然 PASS（连跑 3 次）——没投递出去的键会走
	// settleKeys 重试（input.go:845），下一帧补投递。所以这行钉的是时序，不是
	// 本测试的生死线。
	tt.Frame()
	if a.commandOpen {
		t.Fatal("the panel starts closed")
	}
	tt.Key(ui.Cmd, ui.KeyK)
	if !a.commandOpen {
		t.Fatal("⌘K must open the panel")
	}
}
