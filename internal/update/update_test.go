package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		cand, cur string
		want      bool
	}{
		{"v1.2.0", "1.1.0", true},
		{"1.0.1", "1.1.0", false},
		{"v2.0.0", "1.9.9", true},
		{"1.1.0", "1.1.0", false},
		{"v1.2", "1.1.0", false},    // unparseable is never newer
		{"nightly", "1.1.0", false}, // ditto
		{"1.10.0", "1.9.0", true},   // numeric, not lexicographic
	}
	for _, tc := range cases {
		if got := Newer(tc.cand, tc.cur); got != tc.want {
			t.Errorf("Newer(%q,%q)=%v want %v", tc.cand, tc.cur, got, tc.want)
		}
	}
}

func TestLatestParsesPayloadAndSendsToken(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		fmt.Fprint(w, `{"tag_name":"v9.9.9","name":"nine","assets":[
			{"name":"openarchitect-9.9.9-darwin-arm64.tar.gz","browser_download_url":"http://x/a.tar.gz"},
			{"name":"checksums.txt","browser_download_url":"http://x/checksums.txt"}]}`)
	}))
	defer srv.Close()
	t.Setenv(TokenEnv, "tok123")
	ch := Checker{BaseURL: srv.URL, Client: srv.Client()}
	rel, err := ch.Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rel.Version() != "9.9.9" || len(rel.Assets) != 2 {
		t.Fatalf("got %+v", rel)
	}
	if auth != "Bearer tok123" {
		t.Errorf("token not forwarded: %q", auth)
	}
}

func TestLatestHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	ch := Checker{BaseURL: srv.URL, Client: srv.Client()}
	if _, err := ch.Latest(context.Background()); err == nil {
		t.Fatal("want error on HTTP 403")
	}
}

func tarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755,
			Typeflag: tar.TypeReg, Size: int64(len(body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func testRelease(archive []byte) *Release {
	return &Release{TagName: "v9.9.9", Assets: []Asset{
		{Name: "openarchitect-9.9.9-darwin-arm64.tar.gz", DownloadURL: "mem://a"},
		{Name: ChecksumAsset, DownloadURL: "mem://sums"},
	}}
}

func memFetch(archive []byte, sums string) func(context.Context, string) ([]byte, error) {
	return func(_ context.Context, url string) ([]byte, error) {
		switch url {
		case "mem://a":
			return archive, nil
		case "mem://sums":
			return []byte(sums), nil
		}
		return nil, fmt.Errorf("unexpected url %q", url)
	}
}

func goodSums(archive []byte) string {
	sum := sha256.Sum256(archive)
	return hex.EncodeToString(sum[:]) + "  openarchitect-9.9.9-darwin-arm64.tar.gz\n"
}

func TestApplySwapsExecutable(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "oac")
	if err := os.WriteFile(exe, []byte("OLD"), 0o755); err != nil {
		t.Fatal(err)
	}
	archive := tarGz(t, map[string]string{"oac": "NEW", "openarchitect": "GUI-NEW"})
	if err := (Checker{}).Apply(context.Background(), testRelease(archive), ApplyOptions{
		ExePath: exe, Fetch: memFetch(archive, goodSums(archive))}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(exe)
	if string(got) != "NEW" {
		t.Fatalf("exe content %q want NEW", got)
	}
	if _, err := os.Stat(exe + ".old"); !os.IsNotExist(err) {
		t.Error("backup not cleaned up")
	}
	fi, _ := os.Stat(exe)
	if fi.Mode().Perm()&0o100 == 0 {
		t.Error("new binary not executable")
	}
}

func TestApplyChecksumMismatchKeepsBinary(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "openarchitect")
	if err := os.WriteFile(exe, []byte("OLD"), 0o755); err != nil {
		t.Fatal(err)
	}
	archive := tarGz(t, map[string]string{"openarchitect": "EVIL"})
	err := (Checker{}).Apply(context.Background(), testRelease(archive), ApplyOptions{
		ExePath: exe, Fetch: memFetch(archive,
			"bogus  openarchitect-9.9.9-darwin-arm64.tar.gz\n")})
	if err == nil {
		t.Fatal("want checksum error")
	}
	got, _ := os.ReadFile(exe)
	if string(got) != "OLD" {
		t.Fatalf("binary changed despite bad checksum: %q", got)
	}
}

func TestApplyRefusesUnknownExecutable(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "go-build1234")
	if err := os.WriteFile(exe, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	archive := tarGz(t, map[string]string{"oac": "NEW"})
	err := (Checker{}).Apply(context.Background(), testRelease(archive),
		ApplyOptions{ExePath: exe, Fetch: memFetch(archive, goodSums(archive))})
	if err == nil {
		t.Fatal("want refusal for go-run-style binary")
	}
}

func TestApplyAssetSelection(t *testing.T) {
	rel := &Release{TagName: "v1.0.0", Assets: []Asset{{Name: "x-windows-amd64.zip"}}}
	if _, err := rel.Asset(); err == nil {
		t.Fatal("want no-asset error on wrong platform")
	}
}
