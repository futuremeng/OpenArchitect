package ui

import (
	"context"
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
	// repoID、err。a.repos 的内容不是这条测试的断言对象。
	if a.repoID != repo.ID {
		t.Fatalf("repoID = %q want %q", a.repoID, repo.ID)
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
	// 这条钉的是 BeginScan 里的门（ErrScanInFlight，app.go:79-81），不是那一行
	// scan.Disabled：门在，删掉 Disabled 本测试照样绿（headless 里 disabled 的
	// 按钮仍进 rt.labels、tt.Click 仍返回 nil，只是 release 不 ++clicks，
	// input.go:406-414）。那句 Disabled 唯一的可观察后果是这个按钮不画
	// 按压/悬停底色（shells/sidebar/icon_button.go:24-26），像素级的事留给
	// Task 24 的真窗口手工验收。反向证据：删掉门，BeginScan 会走到
	// scanner.Detect(t.TempDir())（无 go.mod）→ failScan → StatusFailed，
	// 最后那条 status 断言就红。
	if a.cancel != nil {
		t.Fatal("a gated scan must never start")
	}
	if a.status != StatusScanning {
		t.Fatalf("status = %v", a.status)
	}
}
