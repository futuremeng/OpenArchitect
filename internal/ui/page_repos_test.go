package ui

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

func TestReposPageEmptyState(t *testing.T) {
	a := NewApp(nil)
	tt := ui.NewTester(func(c *ui.Context) { a.reposPage(c) }, 900, 600)
	tt.Frame()
	if !tt.HasText("还没有扫描过仓库") {
		t.Fatalf("empty state missing, texts = %v", tt.Texts())
	}
	// 空态必须给出路：一句"去设置里填扫描目录"，而不是只有标题。
	if !tt.HasText("设置") {
		t.Fatalf("the empty state must point at the settings page, texts = %v", tt.Texts())
	}
}

func TestReposPageListsEveryRepo(t *testing.T) {
	a := newTestApp(t)
	seedScan(t, a, fixtureDir("simple-multi-main"))
	seedScan(t, a, fixtureDir("shared-lib"))
	if err := a.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(func(c *ui.Context) { a.reposPage(c) }, 900, 600)
	tt.Frame()
	for _, repo := range a.repos {
		for _, want := range []string{repo.Name, repo.LocalPath, repo.Lang} {
			if !tt.HasText(want) {
				t.Fatalf("%q missing for %s, texts = %v", want, repo.Name, tt.Texts())
			}
		}
	}
	if len(a.repos) != 2 {
		t.Fatalf("repos = %d", len(a.repos))
	}
}

// TestRepoScanButtonRunsToReady 是点击→扫描→状态回写这一整条链唯一的端到端
// 测试。post 换成往 channel 里投递，扫描 goroutine 就只在 channel 上交接，
// finishScan 由测试线程执行 —— 没有竞态，-race 也过。
func TestRepoScanButtonRunsToReady(t *testing.T) {
	a := newTestApp(t)
	seedScan(t, a, fixtureDir("simple-multi-main"))
	if err := a.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	// BeginScan 把传入路径 filepath.Abs 后才算 repo 主键（Task 16），所以
	// 这里必须用绝对路径比对；直接塞 fixtureDir 的相对路径会让最后的断言假失败。
	root, err := filepath.Abs(fixtureDir("shared-lib"))
	if err != nil {
		t.Fatal(err)
	}
	repo := model.Repo{
		ID:        model.RepoIDFor(root),
		Name:      "example.com/shared",
		LocalPath: root,
		Lang:      "go",
	}
	a.repos = []model.Repo{repo}
	// 先留一条旧的失败错误：BeginScan 在把 status 写成 Scanning 的同一条语句里
	// 清 a.err（app.go:90），而 finishScan 的成功臂从不碰 a.err。所以删掉那句
	// 清理，下面「跑完的扫描必须清掉旧错误」就红 —— 不这么 setup，a.err 本来就是
	// nil，那条断言钉不住任何东西。
	a.err = errors.New("stale failure from the last scan")

	done := make(chan func(), 1)
	a.SetPost(func(fn func()) { done <- fn })
	tt := ui.NewTester(func(c *ui.Context) { a.reposPage(c) }, 900, 600)
	tt.Frame()
	if err := tt.Click("扫描 " + repo.Name); err != nil {
		t.Fatal(err)
	}
	if a.status != StatusScanning {
		t.Fatalf("status right after the click = %v", a.status)
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
	// Reload 会用库里读出的行覆盖 a.repos，所以这里只钉状态机：status、
	// 选择集、err。a.repos 的内容不是这条测试的断言对象。两个仓都带快照
	// 且默认全选，repoID 因此是空（合并视图没有单一归属仓）。
	if !a.selectedRepos[string(repo.ID)] || len(a.selectedRepos) != 2 {
		t.Fatalf("the scanned repo must join the select-all union: %v", a.selectedRepos)
	}
	if a.err != nil {
		t.Fatalf("a finished scan must clear err, got %v", a.err)
	}
}

func TestRepoScanButtonDisabledWhileScanning(t *testing.T) {
	a := newTestApp(t)
	a.repos = []model.Repo{{ID: "repo:/x", Name: "x", LocalPath: t.TempDir(), Lang: "go"}}
	a.status = StatusScanning
	tt := ui.NewTester(func(c *ui.Context) { a.reposPage(c) }, 900, 600)
	tt.Frame()
	if err := tt.Click("扫描 x"); err != nil {
		t.Fatal(err)
	}
	// 这条钉的是 BeginScan 里的门（ErrScanInFlight，app.go:79-81）与那一行
	// scan.Disabled 这**一对**守卫：只删门、留着 Disabled 时点击根本进不到
	// BeginScan，测试仍绿；只删 Disabled、留着门时门照挡，也仍绿。变异实测：
	// 两个同时删掉，点击一路走到 scanner.Detect(t.TempDir())（无 go.mod）→
	// failScan → StatusFailed，最后那条 status 断言红。
	// Disabled 在这一条 headless 点击路径上的可观察后果只有「按钮不画按压/
	// 悬停底色」——sidebar.IconButton 直接委派 components.IconButton（icon_button.go:12-14），
	// Plain 臂的 Draw 在 case b.IsDisabled() 上取空分支（components/icon_button.go:43），
	// 像素级的事留给 Task 24 的真窗口手工验收。disabled 还摘掉 Tab 可达性与键盘
	// 激活（同文件 :16-18「MyGo owns its pointer, keyboard and focus behavior」），
	// 但那是 mygo 的输入层，本测试无从区分。
	// （disabled 的按钮仍进 rt.labels、tt.Click 仍返回 nil，只是 mygo 的 ui/input.go:406-414
	// 在 release 时不 ++clicks。）
	if a.cancel != nil {
		t.Fatal("a gated scan must never start")
	}
	if a.status != StatusScanning {
		t.Fatalf("status = %v", a.status)
	}
}

// TestRepoViewButtonPinsAndJumpsToGraph: 仓库页的「查看」把图页过滤器收成
// 这一个仓，并把页面切过去 —— 列表里每一行都必须点得到一张图。
func TestRepoViewButtonPinsAndJumpsToGraph(t *testing.T) {
	a := newTestApp(t)
	only := seedScan(t, a, fixtureDir("simple-multi-main"))
	if err := a.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(func(c *ui.Context) { a.reposPage(c) }, 900, 600)
	tt.Frame()
	if err := tt.Click("查看"); err != nil {
		t.Fatal(err)
	}
	if a.Page() != PageGraph {
		t.Fatalf("view button must jump to the graph page: page = %v", a.Page())
	}
	if a.repoID != string(only.RepoID) {
		t.Fatalf("repoID = %q want %q", a.repoID, string(only.RepoID))
	}
	if len(a.settings.GraphRepos) != 1 || a.settings.GraphRepos[0] != string(only.RepoID) {
		t.Fatalf("the jump must pin the pick: GraphRepos = %v", a.settings.GraphRepos)
	}
}

