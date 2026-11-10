package ui

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

func TestFruitColorMapping(t *testing.T) {
	ui.NewTester(func(c *ui.Context) {
		tt := c.Theme()
		if got, want := fruitColor("agent:claude", tt), agentColor("claude"); got != want {
			t.Errorf("agent:claude = %v, want identity color %v", got, want)
		}
		if fruitColor("agent:codex", tt) == fruitColor("agent:claude", tt) {
			t.Error("distinct agents must not collide on the first palette slot")
		}
		if got := fruitColor("mixed", tt); got != tt.Warning {
			t.Errorf("mixed = %v, want Warning", got)
		}
		if got := fruitColor("", tt); got != tt.TextMuted {
			t.Errorf("no fruit = %v, want TextMuted", got)
		}
	}, 10, 10)
}

func TestNodeCardRendersWithFruit(t *testing.T) {
	el := model.Element{
		ID:   model.ComponentID("example.com/x/internal/basket", "Basket"),
		Type: model.TypeComponent,
		Name: "Basket",
	}
	badge := fruitBadge{
		commits:  []model.Commit{{AgentTag: "agent:claude", Files: []string{"a.go", "b.go"}}, {AgentTag: "mixed"}},
		maxFiles: 4,
	}
	tt := ui.NewTester(func(c *ui.Context) { nodeCard(c, el, 1, badge) }, 220, 60)
	tt.Frame()
	if !tt.HasText("Basket") {
		t.Fatal("fruit badge must not push the name off the card")
	}
	// 展开档只在选中卡上出现条；不展开时一切照旧。
	expanded := badge
	expanded.expand = true
	tt = ui.NewTester(func(c *ui.Context) { nodeCard(c, el, 1, expanded) }, 220, 60)
	tt.Frame()
	if !tt.HasText("Basket") {
		t.Fatal("expanded bars must not push the name off the card")
	}
}

func TestDetailPanelShowsFruit(t *testing.T) {
	a := newDetailApp(t)
	a.fruits = map[string][]model.Commit{
		a.selected: {{
			Hash:     strings.Repeat("a", 40),
			Date:     "2026-11-10T09:00:00Z",
			Message:  "feat: 结算支持多币种",
			AgentTag: "agent:claude",
		}},
	}
	tt := newDetailTester(t, a)
	if !tt.HasText("果实（1）") {
		t.Fatal("fruit group missing")
	}
	if !tt.HasText("feat: 结算支持多币种") {
		t.Fatal("commit subject missing")
	}
	if !tt.HasText(strings.Repeat("a", 8) + " · 2026-11-10") {
		t.Fatal("short hash and date missing")
	}
}

// TestBuildFruitsIndexesAgentCommits 走真库真扫描：果实按目录落到元素上，
// 计数按归因分类。这是 Reload 里唯一一次果实计算。
func TestBuildFruitsIndexesAgentCommits(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	seedScan(t, a, fixtureDir("shared-lib"))
	if err := a.Reload(ctx); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	var target model.Element
	for _, el := range a.graph.Elements() {
		if el.FilePath != "" {
			target = el
			break
		}
	}
	if target.ID == "" {
		t.Fatal("fixture produced no element with a file path")
	}
	touched := filepath.ToSlash(filepath.Join(filepath.Dir(target.FilePath), "z.go"))
	err := a.db.SaveCommits(ctx, []model.Commit{
		{RepoID: a.repoID, Hash: strings.Repeat("a", 40), Date: "2026-11-10T09:00:00Z",
			Message: "feat: x", Files: []string{touched}, AgentTag: "agent:claude"},
		{RepoID: a.repoID, Hash: strings.Repeat("b", 40), Date: "2026-11-09T09:00:00Z",
			Message: "fix: y", Files: []string{touched}, AgentTag: "mixed"},
		{RepoID: a.repoID, Hash: strings.Repeat("c", 40), Date: "2026-11-08T09:00:00Z",
			Message: "chore: z", Files: []string{touched}, AgentTag: "human"},
	})
	if err != nil {
		t.Fatalf("SaveCommits: %v", err)
	}
	if err := a.Reload(ctx); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if got := len(a.fruits[target.ID]); got != 2 {
		t.Fatalf("fruits = %d, want the two agent/mixed commits", got)
	}
	if a.fruitByAgent["claude"] != 1 || a.fruitMixed != 1 {
		t.Fatalf("counts claude=%d mixed=%d, want 1/1", a.fruitByAgent["claude"], a.fruitMixed)
	}
	if len(a.fruitLog) != 2 {
		t.Fatalf("fruitLog = %d, want human commits excluded", len(a.fruitLog))
	}
}

// TestCursorFiltersFruits 钉时间滑杆的重聚合语义：游标之前可见、之后隐藏、
// 满格回到全部——全程不查库。
func TestCursorFiltersFruits(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	seedScan(t, a, fixtureDir("shared-lib"))
	if err := a.Reload(ctx); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	var target model.Element
	for _, el := range a.graph.Elements() {
		if el.FilePath != "" {
			target = el
			break
		}
	}
	file := filepath.ToSlash(filepath.Join(filepath.Dir(target.FilePath), "z.go"))
	err := a.db.SaveCommits(ctx, []model.Commit{
		{RepoID: a.repoID, Hash: strings.Repeat("a", 40), Date: "2026-11-10T09:00:00+08:00",
			Message: "feat: new", Files: []string{file}, AgentTag: "agent:claude"},
		{RepoID: a.repoID, Hash: strings.Repeat("b", 40), Date: "2026-01-02T09:00:00+08:00",
			Message: "fix: old", Files: []string{file}, AgentTag: "mixed"},
	})
	if err != nil {
		t.Fatalf("SaveCommits: %v", err)
	}
	if err := a.Reload(ctx); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if len(a.fruits[target.ID]) != 2 {
		t.Fatalf("all fruits = %d, want 2", len(a.fruits[target.ID]))
	}
	a.setCursorFrac(0.5) // 时间轴中点：只剩 1 月那条
	if cs := a.fruits[target.ID]; len(cs) != 1 || cs[0].Hash[0] != 'b' {
		t.Fatalf("at mid-cursor fruits = %v, want only the January commit", cs)
	}
	if len(a.fruitByAgent) != 0 || a.fruitMixed != 1 {
		t.Fatalf("legend at mid-cursor: agents=%v mixed=%d", a.fruitByAgent, a.fruitMixed)
	}
	a.setCursorFrac(1) // 满格 = 全部
	if len(a.fruits[target.ID]) != 2 {
		t.Fatal("full cursor must show everything")
	}
}

// TestBeginSyncGuardsAndFinishes 用排队 post 把 finishSync 留在主线程：
// 生产里 post 就是主线程派发，测试内联执行会和断言抢 status（-race）。
func TestBeginSyncGuardsAndFinishes(t *testing.T) {
	a := newTestApp(t)
	if err := a.BeginSync(); err == nil {
		t.Fatal("an empty ScanRoot must refuse to sync")
	}
	a.settings.ScanRoot = t.TempDir()
	queue := make(chan func(), 8)
	a.post = func(fn func()) { queue <- fn }
	if err := a.BeginSync(); err != nil {
		t.Fatalf("BeginSync: %v", err)
	}
	if err := a.BeginSync(); !errors.Is(err, ErrScanInFlight) {
		t.Fatalf("second BeginSync = %v, want ErrScanInFlight", err)
	}
	select {
	case fn := <-queue:
		fn()
	case <-time.After(30 * time.Second):
		t.Fatal("sync never posted its completion")
	}
	if a.status != StatusReady {
		t.Fatalf("status = %v, err = %v, want Ready", a.status, a.err)
	}
}
