package ui

// TestDoDProbeRealDB 是切片 1 验收的探针（与 tools/flowprobe、parseprobe 同类：
// 一次性尖峰，不是产品代码）。它对着真实用户库跑 fixture 单测证不了的三件事：
//
//   - DoD-1(a)：库里已有快照时，开库 + Reload + 首帧布局（flow.State.Ready，
//     即「每个节点都被测量、布局并显示」flow.go:92）的耗时门槛 ≤2 s；
//   - DoD-2：真实数据下抽屉五件套齐全，且 Reveal 后节点卡片真的落在视口里
//     （单测钉的是语义接线，这里钉的是几何结果）；
//   - DoD-4 GUI 半边：抽屉「影响范围」行必须与 `oac impact` 的计数一致
//     （onevra 快照下 DialogOptions 深度 2 = 上游 2 / 下游 0）。
//
// 默认 skip；验收时跑：
//
//	OAC_DOD_DB="$HOME/Library/Application Support/OpenArchitect/openarchitect.db" \
//	  go test ./internal/ui/ -run TestDoDProbeRealDB -v
//
// 探针只读库（Reload/Select/SetImpactMode 都不写），跑完不改任何状态。

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/futuremeng/OpenArchitect/internal/core/store"
)

const (
	// dodTarget 是 DoD-4 的比对中心：CLI 侧 2026-10-26 实测 Upstream 2 / Downstream 0。
	dodTarget = "cmp:github.com/oneo-me/onevra/source/components.DialogOptions"
	// dodRich 是抽屉与 Reveal 的靶子：名字在 147 个元素里无子串冲突（Find 是
	// 子串语义，DialogOptions 会被 AlertDialogOptions 污染），方法多到触发折叠，
	// 文件路径非空。
	dodRich = "cmp:github.com/oneo-me/onevra/source/components.BreadcrumbsControl"
)

func TestDoDProbeRealDB(t *testing.T) {
	dbPath := os.Getenv("OAC_DOD_DB")
	if dbPath == "" {
		t.Skip("OAC_DOD_DB 未设置：这条探针只在验收时对真库跑")
	}
	ctx := context.Background()

	t0 := time.Now()
	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("store.Open(%s): %v", dbPath, err)
	}
	t.Cleanup(func() { db.Close() })
	a := NewApp(db)
	if err := a.Reload(ctx); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if n := len(a.graph.Elements()); n == 0 {
		t.Fatal("真库里没有元素")
	} else {
		t.Logf("载入 %d 个元素，Reload 耗时 %.2f s", n, time.Since(t0).Seconds())
	}

	a.SetShowComponents(true)
	tt := ui.NewTester(func(c *ui.Context) { a.graphPage(c) }, 1180, 780)
	tt.Frame()
	first := time.Since(t0)
	if !a.flowState.Ready() {
		t.Fatalf("DoD-1(a) 失败：首帧后 flow 未 Ready（%.2f s）", first.Seconds())
	}
	t.Logf("DoD-1(a): 首帧 Ready，开库+Reload+布局合计 %.2f s（门槛 2 s）", first.Seconds())
	if first > 2*time.Second {
		t.Errorf("DoD-1(a) 超门槛：%.2f s > 2 s", first.Seconds())
	}

	// DoD-2 五件套：名称、类型徽章、包路径、文件路径、方法折叠、两组邻居 chip。
	a.Select(dodRich)
	tt.Frame()
	for _, want := range []string{
		"BreadcrumbsControl", "Component",
		"包路径", "github.com/oneo-me/onevra/source/components",
		"components/breadcrumbs.go",
		"导出方法（", "还有 ", "点标题展开",
		"直接上游", "直接下游",
	} {
		if !tt.HasText(want) {
			t.Errorf("DoD-2：抽屉缺 %q", want)
		}
	}

	// DoD-2b Reveal 落视口：先收抽屉，chip 文本不再污染 Find（子串语义）。
	// 镜头是 flow 私有状态，headless 下能动的两条路是键盘（View 文档：
	// "the camera follows the focused node"）与拖拽。流程：放大到卡片文字
	// 被绘制（LOD）→ Tab 走焦点把镜头挪开（卡片 rect 变化或消失即挪开）→
	// Reveal → 卡片回到视口中央带（reveal 把节点居中，flow.go:104）。
	a.Select("")
	tt.Frame()
	for i := 0; i < 4; i++ {
		if _, ok := tt.Find("BreadcrumbsControl"); ok {
			break
		}
		a.flowState.ZoomIn()
		tt.Frame()
	}
	r1, shown := tt.Find("BreadcrumbsControl")
	if !shown {
		t.Error("DoD-2b：放大 4 档仍找不到靶子卡片文字，Reveal 断言无法进行")
	} else {
		moved := false
		r2 := r1
		for i := 0; i < 15 && !moved; i++ {
			tt.Key(0, ui.KeyTab)
			tt.Advance(500 * time.Millisecond)
			tt.Frame()
			r, ok := tt.Find("BreadcrumbsControl")
			moved = !ok || r.X != r2.X || r.Y != r2.Y
			r2 = r
		}
		if !moved {
			t.Error("DoD-2b：15 次 Tab 都没挪动镜头，Reveal 断言钉不住")
		} else {
			a.flowState.Reveal(dodRich)
			// reveal 只挪 camera，渲染层的 shown 靠动画追赶（render.go:159：
			// 只有 reduceMotion 才同步）。headless 时钟不自己走，必须 Advance
			// 过动画时长，否则 Find 拍到的是中间帧——首跑 cy=196 就是这么来的。
			tt.Advance(2 * time.Second)
			tt.Frame()
			r3, ok := tt.Find("BreadcrumbsControl")
			cx, cy := r3.X+r3.W/2, r3.Y+r3.H/2
			switch {
			case !ok:
				t.Error("DoD-2b：Reveal 之后靶子卡片没有回到视口")
			case cx < 0.3*1180 || cx > 0.7*1180 || cy < 0.3*780 || cy > 0.7*780:
				t.Errorf("DoD-2b：Reveal 后卡片中心 (%.0f, %.0f) 不在视口中央带", cx, cy)
			default:
				t.Logf("DoD-2b: 镜头被 Tab 挪开后 Reveal 把卡片居中到 (%.0f, %.0f) ✓", cx, cy)
			}
		}
	}

	// DoD-4 GUI 半边：与 CLI 的 2/0 对上（深度默认 2，config.DefaultImpactDepth）。
	a.Select(dodTarget)
	a.SetImpactMode(true)
	tt.Frame()
	if !tt.HasText("上游 2 个，下游 0 个，深度 2 跳") {
		t.Errorf("DoD-4：抽屉影响范围行与 CLI（上游 2 / 下游 0）不一致")
	} else {
		t.Log("DoD-4: 抽屉「上游 2 个，下游 0 个，深度 2 跳」= oac impact 输出 ✓")
	}
}
