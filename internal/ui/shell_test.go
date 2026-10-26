package ui

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

func TestSidebarNavClickSelectsThatPage(t *testing.T) {
	for _, item := range navItems() {
		a := newTestApp(t)
		tt := ui.NewTester(func(c *ui.Context) { a.sidebarNav(c) }, 260, 600)
		tt.Frame()
		if err := tt.Click(item.label); err != nil {
			t.Fatalf("%s: %v", item.label, err)
		}
		if a.page != item.page {
			t.Fatalf("click %q → page %v, want %v", item.label, a.page, item.page)
		}
	}
}

func TestContentRoutesEachPage(t *testing.T) {
	cases := []struct {
		page Page
		want string
	}{
		{PageGraph, "还没有扫描过任何仓库"},
		{PageRepos, "还没有扫描过仓库"},
		{PageSettings, "影响分析深度"},
	}
	for _, tc := range cases {
		a := newTestApp(t)
		a.SetPage(tc.page)
		tt := ui.NewTester(func(c *ui.Context) { a.content(c) }, 1180, 780)
		tt.Frame()
		if !tt.HasText(tc.want) {
			t.Fatalf("page %v must show %q, texts = %v", tc.page, tc.want, tt.Texts())
		}
	}
}

// TestCommandShortcutOpensPanelThroughContent 证明的是"位置"：Task 20 的单测
// 直接跑 commandPalette，这一条跑 content —— ⌘K 的轮询只有在真的每帧被调用时
// 才有效，接错地方（比如放进 graphPage）就只在架构图页能用。
func TestCommandShortcutOpensPanelThroughContent(t *testing.T) {
	a := newTestApp(t)
	tt := ui.NewTester(func(c *ui.Context) { a.content(c) }, 1180, 780)
	tt.Frame()
	if a.commandOpen {
		t.Fatal("the panel starts closed")
	}
	tt.Key(ui.Cmd, ui.KeyK)
	if !a.commandOpen {
		t.Fatal("⌘K polled through content must open the panel")
	}
}

func TestScanToolbarScansConfiguredRoot(t *testing.T) {
	a := newTestApp(t)
	root, err := filepath.Abs(fixtureDir("simple-multi-main"))
	if err != nil {
		t.Fatal(err)
	}
	a.settings.ScanRoot = root
	done := make(chan func(), 1)
	a.SetPost(func(fn func()) { done <- fn })
	tt := ui.NewTester(func(c *ui.Context) { a.scanToolbar(c) }, 220, 60)
	tt.Frame()
	if err := tt.Click("扫描当前仓库"); err != nil {
		t.Fatal(err)
	}
	if a.status != StatusScanning {
		t.Fatalf("status right after the click = %v", a.status)
	}
	tt.Frame()
	if !tt.HasText("正在解析") {
		t.Fatalf("the toolbar must show the progress line, texts = %v", tt.Texts())
	}
	select {
	case fn := <-done:
		fn()
	case <-time.After(60 * time.Second):
		t.Fatal("the scan never reported back")
	}
	if a.status != StatusReady {
		t.Fatalf("status = %v err = %v", a.status, a.err)
	}
	if len(a.repos) != 1 || a.repos[0].ID != model.RepoIDFor(root) {
		t.Fatalf("repos after reload = %+v, want the scanned root %q", a.repos, model.RepoIDFor(root))
	}
}

func TestScanToolbarWithoutRootOpensSettings(t *testing.T) {
	a := newTestApp(t)
	tt := ui.NewTester(func(c *ui.Context) { a.scanToolbar(c) }, 220, 60)
	tt.Frame()
	if err := tt.Click("扫描当前仓库"); err != nil {
		t.Fatal(err)
	}
	// 空目录不是错误：BeginScan 会在 scanner.Detect 上失败并把状态打成 Failed，
	// 所以这里根本不能起扫描，只能把用户送到能填目录的那一页。
	if a.status != StatusIdle {
		t.Fatalf("status = %v err = %v", a.status, a.err)
	}
	if a.page != PageSettings {
		t.Fatalf("page = %v", a.page)
	}
}
