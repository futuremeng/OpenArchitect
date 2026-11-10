package ui

import (
	"context"

	"github.com/futuremeng/OpenArchitect/internal/update"
	"github.com/futuremeng/OpenArchitect/internal/version"
)

// BeginUpdateCheck asks GitHub Releases for the newest version in the
// background. Deliberately not tied to the scan guard: it touches no
// database. Only the main thread writes state (discipline 1); the goroutine
// hands results back through post, same choreography as sync.
func (a *App) BeginUpdateCheck() {
	if a.updateState != "" {
		return
	}
	a.updateState, a.updateErr, a.updateChecked = "checking", nil, false
	go func() {
		rel, err := a.checker().Latest(context.Background())
		a.post(func() { a.finishUpdateCheck(rel, err) })
	}()
}

func (a *App) finishUpdateCheck(rel *update.Release, err error) {
	a.updateState = ""
	if err != nil {
		a.updateErr = err
		return
	}
	a.updateChecked = true
	if update.Newer(rel.Version(), version.Version) {
		a.updateLatest, a.updateRelease = rel.Version(), rel
	} else {
		a.updateLatest, a.updateRelease = "", nil
	}
}

// BeginUpdateInstall downloads and installs the release found by the last
// check. The swap replaces the binary on disk; the running window keeps
// showing the old version until the user relaunches.
func (a *App) BeginUpdateInstall() {
	if a.updateState != "" || a.updateRelease == nil {
		return
	}
	a.updateState, a.updateErr = "installing", nil
	rel := a.updateRelease
	go func() {
		err := a.checker().Apply(context.Background(), rel, update.ApplyOptions{})
		a.post(func() { a.finishUpdateInstall(err) })
	}()
}

func (a *App) finishUpdateInstall(err error) {
	a.updateState = ""
	if err != nil {
		a.updateErr = err
		return
	}
	a.updateInstalled = true
}

func (a *App) checker() *update.Checker {
	if a.updateChecker != nil {
		return a.updateChecker
	}
	return &update.Default
}
