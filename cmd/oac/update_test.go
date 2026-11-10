package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/futuremeng/OpenArchitect/internal/update"
)

func fakeGitHub(t *testing.T, tag string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"tag_name":%q,"assets":[{"name":"openarchitect-x-darwin-arm64.tar.gz",`+
			`"browser_download_url":"http://x/a.tar.gz"}]}`, tag)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRunUpdateCheckOnly(t *testing.T) {
	old := updateChecker
	defer func() { updateChecker = old }()
	updateChecker = &update.Checker{BaseURL: fakeGitHub(t, "v9.9.9").URL, Client: http.DefaultClient}
	var out bytes.Buffer
	if err := runUpdate(context.Background(), []string{"--check"}, &out); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if !strings.Contains(s, "9.9.9") || !strings.Contains(s, "oac update") {
		t.Fatalf("output: %s", s)
	}
}

func TestRunUpdateAlreadyLatest(t *testing.T) {
	old := updateChecker
	defer func() { updateChecker = old }()
	updateChecker = &update.Checker{BaseURL: fakeGitHub(t, "v"+Version).URL, Client: http.DefaultClient}
	var out bytes.Buffer
	if err := runUpdate(context.Background(), nil, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "已是最新") {
		t.Fatalf("output: %s", out.String())
	}
}

func TestUpdateDispatchSkipsDatabase(t *testing.T) {
	old := updateChecker
	defer func() { updateChecker = old }()
	updateChecker = &update.Checker{BaseURL: fakeGitHub(t, "v"+Version).URL, Client: http.DefaultClient}
	var out bytes.Buffer
	// runUpdate through the real dispatch: run() must route update without
	// ever opening (or creating) the database (F4 rule, like version).
	if err := run(context.Background(), []string{"update", "--check"}); err != nil {
		t.Fatal(err)
	}
	_ = out
}

func TestRunUpdateBadFlag(t *testing.T) {
	var out bytes.Buffer
	if err := runUpdate(context.Background(), []string{"--nope"}, &out); err == nil {
		t.Fatal("want flag error")
	}
}
