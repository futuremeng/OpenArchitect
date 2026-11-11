package ui

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components/treemap"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

// 树冠的色带：生长新近度从「新绿」褪到「旧灰」。30 天内是全绿（一次同步
// 周期内），一年后彻底灰——中间的线性不是精度，是让"多久没动过"用眼睛
// 就能排序。growth spec 切片 E「色相/饱和度=距最近一次 growth_event 的新旧」。
var (
	canopyFreshColor = ui.Hex("#16a34a")
	canopyOldColor   = ui.Hex("#71717a")
)

const (
	canopyFullAge  = 30 * 24 * time.Hour
	canopyFadeAge  = 365 * 24 * time.Hour
	canopyFreshDur = -time.Second // 负值关闭 treemap 过渡（PoC-12 的探针用法）
)

// buildCanopy 在 Reload 末尾把 File 元素铺成 treemap 树：仓 → 目录 → 文件。
// 面积=当前字节数（stat 一次；文件已删则 0，组件自然省略——诚实的树），
// 颜色=该文件上（含其上组件的）最近生长事件的新旧，caption=果实 commit 数。
func (a *App) buildCanopy() {
	a.canopyNodes = nil
	if len(a.canopyFiles) == 0 {
		return
	}
	// fileKey → 最近生长事件时间：事件挂在 element id 上，经 element→file
	// 索引落到树冠的叶子上（组件的事件也算到它住的文件头上去）。
	elFile := map[string]string{}
	fileKeys := map[string]bool{}
	last := map[string]time.Time{}
	for _, el := range a.canopyFiles {
		key := canopyKey(el.RepoID, el.FilePath)
		elFile[el.ID] = key
		fileKeys[key] = true
	}
	for _, el := range a.view.Elements() {
		if el.FilePath != "" {
			elFile[el.ID] = canopyKey(el.RepoID, el.FilePath)
		}
	}
	for _, e := range a.growthLog {
		key, ok := elFile[e.ev.ElementID]
		if !ok || !e.when.After(last[key]) {
			continue
		}
		last[key] = e.when
	}
	// fileKey → 果实 commit 数（按 commit.Files 的精确文件重合，不是目录级）。
	fruits := map[string]int{}
	for _, e := range a.fruitLog {
		seen := map[string]bool{}
		for _, f := range e.cm.Files {
			key := canopyKey(e.cm.RepoID, path.Clean(f))
			if fileKeys[key] && !seen[key] {
				seen[key] = true
				fruits[key]++
			}
		}
	}
	byRepo := map[string][]model.Element{}
	for _, f := range a.canopyFiles {
		byRepo[f.RepoID] = append(byRepo[f.RepoID], f)
	}
	for _, r := range a.repos {
		files := byRepo[r.ID]
		if len(files) == 0 {
			continue
		}
		repo := treemap.Node{ID: r.ID, Label: r.Name}
		byDir := map[string][]model.Element{}
		for _, f := range files {
			byDir[path.Dir(f.FilePath)] = append(byDir[path.Dir(f.FilePath)], f)
		}
		dirs := make([]string, 0, len(byDir))
		for d := range byDir {
			dirs = append(dirs, d)
		}
		slices.Sort(dirs)
		for _, d := range dirs {
			dir := treemap.Node{ID: "dir:" + r.ID + "|" + d, Label: d}
			for _, f := range byDir[d] {
				key := canopyKey(f.RepoID, f.FilePath)
				size := float64(0)
				if st, err := os.Stat(filepath.Join(r.LocalPath, filepath.FromSlash(f.FilePath))); err == nil {
					size = float64(st.Size())
				}
				node := treemap.Node{
					ID: f.ID, Label: f.Name, Value: size,
					Color: canopyColor(last[key]),
				}
				if n := fruits[key]; n > 0 {
					node.Caption = fmt.Sprintf("%d 果", n)
				}
				dir.Children = append(dir.Children, node)
			}
			repo.Children = append(repo.Children, dir)
		}
		a.canopyNodes = append(a.canopyNodes, repo)
	}
}

func canopyKey(repoID, relFile string) string { return repoID + "|" + relFile }

// canopyColor 把「距最近生长事件」褪成颜色：无事件=彻底的旧灰。
func canopyColor(when time.Time) ui.Color {
	if when.IsZero() {
		return canopyOldColor
	}
	age := time.Since(when)
	switch {
	case age <= canopyFullAge:
		return canopyFreshColor
	case age >= canopyFadeAge:
		return canopyOldColor
	default:
		t := float32(float64(age-canopyFullAge) / float64(canopyFadeAge-canopyFullAge))
		return canopyFreshColor.Mix(canopyOldColor, t)
	}
}

// canopyPage 是树冠页。canopyFresh 时那一帧关动画（§9-4），建完即清旗标：
// 关的是"数据变化后的首帧"，不是永关——下钻的 400 ms 过渡是留给交互的。
func (a *App) canopyPage(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).FillWidth().FillHeight().Gap(t.Space(2)).Children(func() {
		anim := time.Duration(0)
		if a.canopyFresh {
			anim = canopyFreshDur
			a.canopyFresh = false
		}
		treemap.View(c, &a.canopyState, a.canopyNodes, treemap.Options{
			Label:             "树冠：面积=文件规模，颜色=生长新近度",
			EmptyText:         "还没有带文件的快照——重新扫描一次即可",
			Open:              a.canopyOpen,
			FormatValue:       formatBytes,
			AnimationDuration: anim,
		})
	})
}

// canopyOpen 是叶子（文件）的点开：跳到架构图页并选中这个文件里的第一个
// 组件——节点卡的果实展开在那边。纯文件（没有组件）选中文件本身，详情
// 面板照样读得到。全程内存：canopyFiles 和视图就是全部数据源，不查库。
func (a *App) canopyOpen(id string) {
	target := id
	for _, f := range a.canopyFiles {
		if f.ID != id {
			continue
		}
		for _, v := range a.view.Elements() {
			if v.RepoID == f.RepoID && v.FilePath == f.FilePath && v.Type == model.TypeComponent {
				target = v.ID
				break
			}
		}
		break
	}
	a.SetPage(PageGraph)
	a.Select(target)
}

func formatBytes(v float64) string {
	switch {
	case v >= 1<<20:
		return fmt.Sprintf("%.1f MB", v/(1<<20))
	case v >= 1<<10:
		return fmt.Sprintf("%.0f KB", v/(1<<10))
	default:
		return fmt.Sprintf("%.0f B", v)
	}
}
