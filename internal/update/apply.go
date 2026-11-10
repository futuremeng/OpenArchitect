package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// ChecksumAsset is the release asset holding "sha256  filename" lines.
const ChecksumAsset = "checksums.txt"

// ApplyOptions makes Apply testable: ExePath stands in for the running
// executable, Fetch for the HTTP download. Zero values use the real thing.
type ApplyOptions struct {
	ExePath string
	Fetch   func(ctx context.Context, url string) ([]byte, error)
}

// Apply replaces the running executable with the release build. The archive
// must pass its checksum before anything on disk changes; the swap is a
// rename pair so a failure mid-way leaves the old binary intact.
func (ch Checker) Apply(ctx context.Context, rel *Release, opt ApplyOptions) error {
	exe := opt.ExePath
	if exe == "" {
		var err error
		if exe, err = os.Executable(); err != nil {
			return err
		}
		exe, _ = filepath.EvalSymlinks(exe)
	}
	name := filepath.Base(exe)
	if name != "oac" && name != "openarchitect" {
		return fmt.Errorf("update: running from %q; only installed oac/openarchitect binaries can self-replace", exe)
	}
	asset, err := rel.Asset()
	if err != nil {
		return err
	}
	fetch := opt.Fetch
	if fetch == nil {
		fetch = ch.fetch
	}
	archive, err := fetch(ctx, asset.DownloadURL)
	if err != nil {
		return fmt.Errorf("download %s: %w", asset.Name, err)
	}
	sums, err := fetch(ctx, checksumURL(rel, asset))
	if err != nil {
		return fmt.Errorf("download %s: %w", ChecksumAsset, err)
	}
	want, err := checksumFor(sums, asset.Name)
	if err != nil {
		return err
	}
	got := sha256.Sum256(archive)
	if hex.EncodeToString(got[:]) != want {
		return fmt.Errorf("update: checksum mismatch for %s, refusing to install", asset.Name)
	}
	payload, err := extract(archive, name)
	if err != nil {
		return err
	}
	return swap(exe, payload)
}

// checksumURL points at the checksums asset of the same release. When the
// asset list carries no checksums entry we fail closed rather than install
// an unverified binary.
func checksumURL(rel *Release, forAsset Asset) string {
	for _, a := range rel.Assets {
		if a.Name == ChecksumAsset {
			return a.DownloadURL
		}
	}
	return forAsset.DownloadURL + ".sha256" // GitHub serves this for every asset
}

func checksumFor(sums []byte, assetName string) (string, error) {
	for _, line := range strings.Split(string(sums), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == assetName {
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", fmt.Errorf("update: %s lists no checksum for %s", ChecksumAsset, assetName)
}

// extract pulls one named file out of a tar.gz payload.
func extract(archive []byte, name string) ([]byte, error) {
	zr, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("update: bad archive: %w", err)
	}
	defer zr.Close()
	tr := tar.NewReader(zr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("update: archive has no %q", name)
		}
		if err != nil {
			return nil, err
		}
		if hdr.Typeflag != tar.TypeReg || filepath.Base(hdr.Name) != name {
			continue
		}
		return io.ReadAll(tr)
	}
}

// swap writes payload over exe atomically enough for a running process:
// rename aside, move in the new file, drop the backup; any step that fails
// renames the backup back.
func swap(exe string, payload []byte) error {
	backup := exe + ".old"
	if err := os.Rename(exe, backup); err != nil {
		return fmt.Errorf("update: move aside: %w", err)
	}
	if err := os.WriteFile(exe, payload, 0o755); err != nil {
		os.Rename(backup, exe)
		return fmt.Errorf("update: install: %w", err)
	}
	if err := os.Chmod(exe, 0o755); err != nil {
		os.Remove(exe)
		os.Rename(backup, exe)
		return fmt.Errorf("update: chmod: %w", err)
	}
	os.Remove(backup)
	return nil
}

func (ch Checker) fetch(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if tok := os.Getenv(TokenEnv); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := ch.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: HTTP %d", url, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1<<30))
}
