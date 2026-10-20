package ui

import (
	"context"
	"strings"
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

// TestCommandItemsImpactItemSurvivesStaleSelection 钉住 selectedName 的退化路径：
// selected 可能指向上一版快照的节点、不在当前 graph 里（command.go:100-101），
// 那时影响分析条目的标题必须用 id 本身，条目照常可点、照常命名它的中心。
func TestCommandItemsImpactItemSurvivesStaleSelection(t *testing.T) {
	a := newFlowApp(t)
	stale := model.ComponentID("example.com/x/gone", "Gone")
	a.selected = stale
	items, err := a.commandItems("")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 4 || items[3].ID != impactItemPrefix+stale {
		t.Fatalf("items = %v", commandIDs(items))
	}
	if !strings.Contains(items[3].Title, stale) {
		t.Fatalf("stale-id fallback broken: title = %q, want it to name %q", items[3].Title, stale)
	}
}

func TestCommandItemsSearchesCurrentSnapshot(t *testing.T) {
	a := newTestApp(t)
	seedScan(t, a, fixtureDir("simple-multi-main"))
	if err := a.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Ruling 7 之后查库只发生在面板打开期间（commandItems 的 a.commandOpen 门控），
	// 所以先开门——这一条仍是「面板真的会去调 SearchElements，并且用的是当前快照
	// 的 id」的唯一钉子（core 之外没有别的消费者）。
	a.SetCommandOpen(true)
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
	// Escape 关闭会保留 query（State doc："Closing a dialog preserves the
	// query"，command.go:34），而弹窗的构建回调只在打开时运行（dialog.go:27），
	// 关闭后的帧没有任何东西消费 items。所以同一个 query 在关闭态必须只回到
	// 三条静态条目——这次查库要被子时掐掉，不能留着。
	a.SetCommandOpen(false)
	closed, err := a.commandItems("Cart")
	if err != nil {
		t.Fatal(err)
	}
	if len(closed) != 3 ||
		closed[0].ID != commandScanID || closed[1].ID != commandReposID || closed[2].ID != commandSettingsID {
		t.Fatalf("closed panel must serve static items only, no element read: %v", commandIDs(closed))
	}
}

func TestFilterCommandsLeavesDatabaseMatchesAlone(t *testing.T) {
	// 三条静态条目逐字对齐 commandItems 的实际产出（id/标题/关键词），fixture
	// 因此漂不出生产形状：下面的命中集合是拿真文案核对过的。
	items := []command.Item{
		{ID: commandScanID, Title: "扫描当前仓库", Keywords: []string{"scan", "扫描", "rescan"}},
		{ID: commandReposID, Title: "跳到仓库页", Keywords: []string{"repos", "仓库", "repo"}},
		{ID: commandSettingsID, Title: "打开设置", Keywords: []string{"settings", "设置", "depth"}},
		{ID: elementItemPrefix + "cmp:a.Basket", Title: "Basket"},
	}
	// 库里已经按 query 筛过元素条目；command.Filter 是「标题或关键词、逐词
	// AND」，"bas 2" 里的 "2" 谁都命中不了 ⇒ 静态全灭、元素条目原样留下。
	got := filterCommands(items, "bas 2")
	if len(got) != 1 || got[0].ID != elementItemPrefix+"cmp:a.Basket" {
		t.Fatalf("element matches must survive: %v", commandIDs(got))
	}
	// 这条是「静态条目真的过 Filter，元素条目不再过一遍」的钉子：
	// "scan" 只命中扫描条目的关键词——仓库/设置两条必须被排除（这正是断言的
	// 牙口），加上 Basket 共 2 条，静态在前。若实现把 command.Filter 套在全部
	// 条目上，Basket 没有 "scan" 关键词会被滤掉，这里就只剩 1 条。
	got = filterCommands(items, "scan")
	if len(got) != 2 || got[0].ID != commandScanID || got[1].ID != elementItemPrefix+"cmp:a.Basket" {
		t.Fatalf("statics filtered, elements kept: %v", commandIDs(got))
	}
	if got := filterCommands(items, ""); len(got) != 4 ||
		got[0].ID != commandScanID || got[1].ID != commandReposID ||
		got[2].ID != commandSettingsID || got[3].ID != elementItemPrefix+"cmp:a.Basket" {
		t.Fatalf("empty query keeps everything in order: %v", commandIDs(got))
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

// TestRunCommandScanWithoutRootGoesToSettings 钉住 Ruling 6：scanRoot 只在
// BeginScan 成功分支里赋值（app.go:89），没配根目录时「扫描当前仓库」没有可扫
// 的地方——诚实的出口是把它导航到 scan.root 所在的设置页（Task 15/21），spec
// §9.1 只点名了这条目，吞掉激活、静默什么都不做不是出口。面板这时恰恰是
// 空状态用户的入口（TestCommandItemsWithoutSnapshotAreStaticOnly 的注释）。
func TestRunCommandScanWithoutRootGoesToSettings(t *testing.T) {
	a := NewApp(nil)
	a.SetPage(PageGraph)
	a.runCommand(commandScanID)
	if a.Page() != PageSettings {
		t.Fatalf("empty scanRoot must route to settings, page = %q", a.Page())
	}
	if a.commandOpen {
		t.Fatal("activating must close the panel")
	}
}

// TestRunCommandScanInFlightStaysNoOp 钉住另一臂：scanRoot 已设、扫描进行中时，
// runCommand 的 `_ =` 吞掉 ErrScanInFlight 是有意的无操作（简报里那句「侧栏状态
// 行已经是 Scanning 了」的注释所指）。复用 app_test.go
// TestBeginScanCompletesScanThroughPost 的 post 捕获模式：无 sleep、无轮询，
// 通道收信就是交接点。
func TestRunCommandScanInFlightStaysNoOp(t *testing.T) {
	a := newTestApp(t)
	queued := make(chan func(), 1)
	a.SetPost(func(fn func()) { queued <- fn })
	if err := a.BeginScan(fixtureDir("shared-lib")); err != nil {
		t.Fatalf("BeginScan: %v", err)
	}
	// 在测试 goroutine 上断言 in-flight 状态（此时扫描 goroutine 只碰 db，
	// 不写 App 字段）。
	if a.scanRoot == "" || a.status != StatusScanning || a.cancel == nil {
		t.Fatalf("setup in-flight: scanRoot=%q status=%v cancel==nil:%v", a.scanRoot, a.status, a.cancel == nil)
	}
	a.runCommand(commandScanID)
	// 第二次 BeginScan 返回 ErrScanInFlight 被吞：状态无操作——仍是 Scanning，
	// 页面不动（NewApp 默认 PageGraph）。
	if a.status != StatusScanning || a.Page() != PageGraph {
		t.Fatalf("in-flight activation must leave state untouched: status=%v page=%q", a.status, a.Page())
	}
	// 先 CancelScan 再排空投递的闭包：finishScan 因此走 context.Canceled →
	// StatusIdle 分支（app.go:105-115），测试返回时不留任何还在写 App 的 goroutine。
	a.CancelScan()
	(<-queued)()
	if a.status != StatusIdle {
		t.Fatalf("cancelled scan must land on Idle, got status=%v err=%v", a.status, a.err)
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
