package scanner

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func fixtureRoot() string { return filepath.Join("..", "..", "testdata", "fixtures") }

func TestDetectGoFixture(t *testing.T) {
	name, err := Detect(filepath.Join(fixtureRoot(), "generic-and-iface"))
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if name != "go" {
		t.Fatalf("language = %q, want go", name)
	}
}

func TestDetectUnsupportedVersusUnreadable(t *testing.T) {
	// 没有 go.mod 的目录：语言不支持，而不是读不了（偏差 11）。
	if _, err := Detect(fixtureRoot()); !errors.Is(err, ErrUnsupportedLanguage) {
		t.Fatalf("err = %v, want ErrUnsupportedLanguage", err)
	}
	if _, err := Detect(filepath.Join(t.TempDir(), "does-not-exist")); !errors.Is(err, ErrUnreadable) {
		t.Fatalf("err = %v, want ErrUnreadable", err)
	}
}

func TestDetectUnreadablePermission(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: chmod would not block access")
	}
	blocked := filepath.Join(t.TempDir(), "locked")
	if err := os.Mkdir(blocked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(blocked, 0o755) })
	if _, err := Detect(blocked); !errors.Is(err, ErrUnreadable) {
		t.Fatalf("err = %v, want ErrUnreadable", err)
	}
}
