package model

import (
	"path/filepath"
	"testing"
	"time"
)

func TestIDRules(t *testing.T) {
	const mod = "example.com/cart"
	cases := []struct{ got, want string }{
		{SystemID(mod), "sys:example.com/cart"},
		{ContainerID(mod + "/cmd/api"), "ctn:example.com/cart/cmd/api"},
		{ComponentID(mod+"/internal/basket", "Basket"), "cmp:example.com/cart/internal/basket.Basket"},
		// 泛型类型按调用方传入的裸声明名建 id：一个类型一个节点（spec §6.2）。
		{ComponentID(mod+"/pkg", "Box"), "cmp:example.com/cart/pkg.Box"},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("got %q want %q", tc.got, tc.want)
		}
	}
}

func TestIDsAreStableAndNoCollision(t *testing.T) {
	const mod = "example.com/cart"
	if SystemID(mod) != SystemID(mod) {
		t.Fatal("SystemID must be pure")
	}
	// 不同包下的同名类型必须分开。
	a := ComponentID(mod+"/aaa", "Box")
	b := ComponentID(mod+"/bbb", "Box")
	if a == b {
		t.Fatalf("collision: %s == %s", a, b)
	}
	// 三种前缀互不重叠。
	prefixes := []string{SystemID(mod), ContainerID(mod), ComponentID(mod, "T")}
	seen := map[string]bool{}
	for _, id := range prefixes {
		switch id[:4] {
		case "sys:", "ctn:", "cmp:":
		default:
			t.Fatalf("bad prefix %q", id)
		}
		if seen[id] {
			t.Fatalf("collision among %v", prefixes)
		}
		seen[id] = true
	}
}

func TestRepoIDIsPathBased(t *testing.T) {
	abs, err := filepath.Abs("/tmp/whatever")
	if err != nil {
		t.Fatal(err)
	}
	if got := RepoIDFor("/tmp/whatever"); got != "repo:"+abs {
		t.Fatalf("got %q want %q", got, "repo:"+abs)
	}
	if RepoIDFor("/tmp/a") == RepoIDFor("/tmp/b") {
		t.Fatal("different paths must give different ids")
	}
}

func TestPkgPathOf(t *testing.T) {
	const mod = "example.com/cart"
	if got := PkgPathOf(ComponentID(mod+"/internal/x", "Y")); got != mod+"/internal/x" {
		t.Fatalf("cmp: got %q", got)
	}
	if got := PkgPathOf(ContainerID(mod + "/cmd/api")); got != mod+"/cmd/api" {
		t.Fatalf("ctn: got %q", got)
	}
	if got := PkgPathOf(SystemID(mod)); got != "" {
		t.Fatalf("sys must have no package: got %q", got)
	}
}

func TestSnapshotIDsSortByTime(t *testing.T) {
	first := NewSnapshotID()
	time.Sleep(time.Millisecond)
	second := NewSnapshotID()
	if first >= second {
		t.Fatalf("ids must sort by time: %q then %q", first, second)
	}
	if len(first) != 20 || first[0] != 's' {
		t.Fatalf("id = %q, want s + 19 digits", first)
	}
}
