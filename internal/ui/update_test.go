package ui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/futuremeng/OpenArchitect/internal/update"
)

func updateCheckerFor(srv *httptest.Server) *update.Checker {
	return &update.Checker{BaseURL: srv.URL, Client: srv.Client()}
}

func releaseServer(tag string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"tag_name":%q,"assets":[{"name":"openarchitect-x-darwin-arm64.tar.gz",`+
			`"browser_download_url":"http://x/a.tar.gz"}]}`, tag)
	}))
}

func drainOne(t *testing.T, queue chan func()) {
	t.Helper()
	select {
	case fn := <-queue:
		fn()
	case <-time.After(30 * time.Second):
		t.Fatal("update never posted its completion")
	}
}

func TestBeginUpdateCheckFindsNewer(t *testing.T) {
	a := newTestApp(t)
	srv := releaseServer("v9.9.9")
	defer srv.Close()
	a.updateChecker = updateCheckerFor(srv)
	queue := make(chan func(), 8)
	a.post = func(fn func()) { queue <- fn }
	a.BeginUpdateCheck()
	if a.updateState != "checking" {
		t.Fatalf("state = %q want checking", a.updateState)
	}
	a.BeginUpdateCheck() // guarded: no second goroutine while one runs
	drainOne(t, queue)
	if a.updateState != "" || a.updateErr != nil {
		t.Fatalf("state=%q err=%v", a.updateState, a.updateErr)
	}
	if a.updateLatest != "9.9.9" || a.updateRelease == nil {
		t.Fatalf("latest=%q release=%v", a.updateLatest, a.updateRelease)
	}
}

func TestBeginUpdateCheckReportsLatest(t *testing.T) {
	a := newTestApp(t)
	srv := releaseServer("v0.0.1")
	defer srv.Close()
	a.updateChecker = updateCheckerFor(srv)
	queue := make(chan func(), 8)
	a.post = func(fn func()) { queue <- fn }
	a.BeginUpdateCheck()
	drainOne(t, queue)
	if !a.updateChecked || a.updateLatest != "" {
		t.Fatalf("checked=%v latest=%q, want checked with no update", a.updateChecked, a.updateLatest)
	}
}

func TestBeginUpdateCheckError(t *testing.T) {
	a := newTestApp(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	a.updateChecker = updateCheckerFor(srv)
	queue := make(chan func(), 8)
	a.post = func(fn func()) { queue <- fn }
	a.BeginUpdateCheck()
	drainOne(t, queue)
	if a.updateErr == nil || a.updateState != "" {
		t.Fatalf("err=%v state=%q want error with idle state", a.updateErr, a.updateState)
	}
}

func TestUpdateInstallGuardedWithoutRelease(t *testing.T) {
	a := newTestApp(t)
	a.BeginUpdateInstall() // no release known: must no-op, not panic
	if a.updateState != "" {
		t.Fatalf("state=%q want untouched", a.updateState)
	}
}
