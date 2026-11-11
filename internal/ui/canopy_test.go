package ui

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components/treemap"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

// leafOf 在树冠里按文件元素 id 找叶子。
func leafOf(nodes []treemap.Node, id string) *treemap.Node {
	for _, repo := range nodes {
		for _, dir := range repo.Children {
			for i := range dir.Children {
				if dir.Children[i].ID == id {
					return &dir.Children[i]
				}
			}
		}
	}
	return nil
}

func TestBuildCanopyTree(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	seedScan(t, a, fixtureDir("generic-and-iface"))
	if err := a.Reload(ctx); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if !a.canopyFresh {
		t.Fatal("Reload must arm the no-animation frame")
	}
	// u.go 不导出任何类型（ghost file），但必须有叶子——PoC-13 的整条理由。
	ghost := leafOf(a.canopyNodes, "fil:example.com/generic/unexported/u.go")
	if ghost == nil {
		t.Fatalf("ghost file missing from canopy: %+v", a.canopyNodes)
	}
	root := fixtureDir("generic-and-iface")
	st, err := os.Stat(filepath.Join(root, "unexported", "u.go"))
	if err != nil {
		t.Fatal(err)
	}
	if ghost.Value != float64(st.Size()) || ghost.Value <= 0 {
		t.Fatalf("area = %v, want current size %d", ghost.Value, st.Size())
	}
	// 没有生长事件 → 彻底的旧灰。
	if ghost.Color != canopyOldColor {
		t.Fatalf("color without events = %v want %v", ghost.Color, canopyOldColor)
	}
	if ghost.Caption != "" {
		t.Fatalf("caption without fruits = %q, want empty", ghost.Caption)
	}
}

func TestCanopyColorAndFruitBadge(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	seedScan(t, a, fixtureDir("shared-lib"))
	if err := a.Reload(ctx); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	repoID := a.repos[0].ID
	// Server 住在 core/core.go——组件的生长事件必须染到它住的叶子上。
	a.growthLog = []growthEntry{{
		ev:   model.GrowthEvent{RepoID: repoID, ElementID: "cmp:example.com/shared/core.Server", Kind: model.GrowthAdded},
		when: time.Now(),
	}}
	a.fruitLog = []fruitEntry{{
		cm:   model.Commit{RepoID: repoID, Files: []string{"core/core.go", "core/core.go"}},
		when: time.Now(),
	}}
	a.buildCanopy()
	core := leafOf(a.canopyNodes, "fil:example.com/shared/core/core.go")
	if core == nil {
		t.Fatal("core.go missing")
	}
	if core.Color != canopyFreshColor {
		t.Fatalf("fresh color = %v want %v", core.Color, canopyFreshColor)
	}
	if core.Caption != "1 果" {
		t.Fatalf("caption = %q, want one fruit per commit (dupes collapse)", core.Caption)
	}
	edge := leafOf(a.canopyNodes, "fil:example.com/shared/edge/edge.go")
	if edge.Color != canopyOldColor || edge.Caption != "" {
		t.Fatalf("untouched file = %+v", edge)
	}
}

func TestCanopyOpenJumpsToComponent(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	seedScan(t, a, fixtureDir("shared-lib"))
	if err := a.Reload(ctx); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	a.canopyOpen("fil:example.com/shared/core/core.go")
	if a.page != PageGraph {
		t.Fatalf("page = %v, want the graph", a.page)
	}
	el, ok := a.graph.Element(a.selected)
	if !ok || el.Type != model.TypeComponent || el.FilePath != "core/core.go" {
		t.Fatalf("selected = %q (%+v), want a component of core.go", a.selected, el)
	}
	// ghost 文件没有组件（main 包不导出类型）：选文件自己，详情面板读得到。
	a.canopyOpen("fil:example.com/shared/cmd/app/main.go")
	if a.selected != "fil:example.com/shared/cmd/app/main.go" {
		t.Fatalf("ghost selection = %q", a.selected)
	}
}

func TestCanopyPageFrameAndFreshFlag(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	seedScan(t, a, fixtureDir("simple-multi-main"))
	if err := a.Reload(ctx); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	tt := ui.NewTester(func(c *ui.Context) { a.canopyPage(c) }, 900, 600)
	tt.Frame()
	if a.canopyFresh {
		t.Fatal("first frame after Reload must consume the fresh flag")
	}
	tt.Advance(16 * time.Millisecond)
	if !tt.HasText("树冠") {
		t.Fatalf("canopy header missing, texts = %v", tt.Texts())
	}
}

func TestCanopyColorFade(t *testing.T) {
	if canopyColor(time.Now().Add(-10*24*time.Hour)) != canopyFreshColor {
		t.Fatal("10 days must be full green")
	}
	if canopyColor(time.Now().Add(-400*24*time.Hour)) != canopyOldColor {
		t.Fatal("400 days must be fully faded")
	}
	mid := canopyColor(time.Now().Add(-180 * 24 * time.Hour))
	if mid == canopyFreshColor || mid == canopyOldColor {
		t.Fatalf("mid-age color must fade between the ends: %v", mid)
	}
}
