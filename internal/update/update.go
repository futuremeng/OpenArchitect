// Package update closes the release loop: it asks GitHub Releases for the
// newest version and can replace the running executable with the official
// build. Network lives here on purpose — but only ever on an explicit user
// action (`oac update`, the settings-page button). Automated observation
// stays offline (I2′); the R3 guard keeps this package out of forest.
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// DefaultBaseURL is the GitHub API endpoint for this repository's latest
// release. Checker.BaseURL overrides it in tests.
const DefaultBaseURL = "https://api.github.com/repos/futuremeng/OpenArchitect/releases/latest"

// TokenEnv optionally carries a token for private-repo checks.
const TokenEnv = "OPENARCHITECT_GITHUB_TOKEN"

// Checker resolves and downloads releases. The zero value is not usable;
// use Default or set BaseURL.
type Checker struct {
	BaseURL string
	Client  *http.Client
}

// Default checks the real GitHub releases endpoint.
var Default = Checker{BaseURL: DefaultBaseURL, Client: &http.Client{Timeout: 30 * time.Second}}

// Release is the slice of the GitHub release payload the loop needs.
type Release struct {
	TagName string  `json:"tag_name"`
	Name    string  `json:"name"`
	Assets  []Asset `json:"assets"`
}

// Asset is one downloadable file of a release.
type Asset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"browser_download_url"`
}

// Latest fetches the newest published release.
func (ch Checker) Latest(ctx context.Context) (*Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ch.base(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if tok := os.Getenv(TokenEnv); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := ch.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("check for updates: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("check for updates: no published release yet (repo private? set %s)", TokenEnv)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("check for updates: got HTTP %d", resp.StatusCode)
	}
	var rel Release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("check for updates: bad payload: %w", err)
	}
	if rel.TagName == "" {
		return nil, fmt.Errorf("check for updates: payload has no tag_name")
	}
	return &rel, nil
}

// Version strips the leading "v" from the tag: "v1.2.3" -> "1.2.3".
func (r *Release) Version() string { return strings.TrimPrefix(r.TagName, "v") }

// Asset returns the release asset for the running platform, e.g.
// "openarchitect-1.2.3-darwin-arm64.tar.gz".
func (r *Release) Asset() (Asset, error) {
	suffix := "-" + runtime.GOOS + "-" + runtime.GOARCH + ".tar.gz"
	for _, a := range r.Assets {
		if strings.HasSuffix(a.Name, suffix) {
			return a, nil
		}
	}
	return Asset{}, fmt.Errorf("release %s has no asset for %s", r.TagName, suffix)
}

// Newer reports whether candidate (with or without a "v") is a strictly
// higher dotted-triple than current. Unparseable candidates are never newer.
func Newer(candidate, current string) bool {
	c, ok := parseVersion(candidate)
	if !ok {
		return false
	}
	cur, ok := parseVersion(current)
	if !ok {
		return false
	}
	for i := 0; i < 3; i++ {
		if c[i] != cur[i] {
			return c[i] > cur[i]
		}
	}
	return false
}

func parseVersion(s string) ([3]int, bool) {
	var out [3]int
	parts := strings.Split(strings.TrimPrefix(strings.TrimSpace(s), "v"), ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

func (ch Checker) base() string {
	if ch.BaseURL == "" {
		return DefaultBaseURL
	}
	return ch.BaseURL
}

func (ch Checker) client() *http.Client {
	if ch.Client != nil {
		return ch.Client
	}
	return Default.Client
}
