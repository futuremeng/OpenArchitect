package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

// 1.0 T9 性能闸门：果实角标必须留在 3.4 ms/帧基线的 ±10% 内。
// 这里量的是每帧 flowData 真的会重做的部分——600 张 nodeCard 的构建；
// 布局/绘制那一半 fruit 与无 fruit 完全同构（多的是同一型 Box）。
// 对照跑：go test ./internal/ui -run NONE -bench NodeCard600 -benchtime 50x

func benchNodeCards(b *testing.B, badge fruitBadge) {
	b.Helper()
	el := model.Element{
		ID:   model.ComponentID("example.com/x/internal/basket", "Basket"),
		Type: model.TypeComponent,
		Name: "Basket",
	}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Children(func() {
			for i := 0; i < 600; i++ {
				nodeCard(c, el, 1, badge)
			}
		})
	}, 1400, 900)
	tt.Frame()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tt.Frame()
	}
}

func benchBadge() fruitBadge {
	return fruitBadge{
		commits: []model.Commit{
			{AgentTag: "agent:claude", Files: []string{"a.go", "b.go"}},
			{AgentTag: "mixed", Files: []string{"c.go"}},
			{AgentTag: "agent:codex", Files: []string{"d.go", "e.go", "f.go"}},
		},
		maxFiles: 3,
	}
}

func BenchmarkNodeCard600(b *testing.B) { benchNodeCards(b, fruitBadge{}) }

func BenchmarkNodeCard600Fruit(b *testing.B) { benchNodeCards(b, benchBadge()) }

// 最坏情况上界：600 卡全部展开成条。真实场景 expand 只落在选中/焦点卡上，
// 这条 bench 是天花板，不是日常。
func BenchmarkNodeCard600Bars(b *testing.B) {
	badge := benchBadge()
	badge.expand = true
	benchNodeCards(b, badge)
}
