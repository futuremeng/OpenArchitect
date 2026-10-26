package config

import (
	"context"
	"path/filepath"
	"slices"
	"testing"

	"github.com/futuremeng/OpenArchitect/internal/core/store"
)

func newDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "cfg.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestLoadDefaultsWhenEmpty(t *testing.T) {
	got, err := Load(context.Background(), newDB(t))
	if err != nil {
		t.Fatal(err)
	}
	if got.ImpactDepth != DefaultImpactDepth || got.ScanRoot != "" || len(got.GraphRepos) != 0 {
		t.Fatalf("settings = %+v", got)
	}
}

func TestSaveThenLoad(t *testing.T) {
	db := newDB(t)
	want := Settings{ScanRoot: "/Users/me/code/cart", ImpactDepth: 3,
		GraphRepos: []string{"repo:/x/y", "repo:/a/b"}}
	if err := Save(context.Background(), db, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if got.ScanRoot != want.ScanRoot || got.ImpactDepth != want.ImpactDepth ||
		!slices.Equal(got.GraphRepos, want.GraphRepos) {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

// 旧版单仓钉选（graph.repo）必须迁进多选集合，否则升级当天用户就会看到
// 自己丢了的过滤器选择。
func TestLegacyGraphRepoMigrates(t *testing.T) {
	db := newDB(t)
	ctx := context.Background()
	if err := db.ConfigSet(ctx, graphRepoKey, "repo:/old/pin"); err != nil {
		t.Fatal(err)
	}
	got, err := Load(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.GraphRepos, []string{"repo:/old/pin"}) {
		t.Fatalf("legacy pin not migrated: %+v", got)
	}
	// 新键一旦有值，旧键就此失声。
	if err := Save(ctx, db, Settings{GraphRepos: []string{"repo:/new"}}); err != nil {
		t.Fatal(err)
	}
	got, err = Load(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.GraphRepos, []string{"repo:/new"}) {
		t.Fatalf("stale legacy key still winning: %+v", got)
	}
}

// 坏值口径：`Atoi` 报错的（not-a-number、空串）、数学上非法的（0、-4），还
// 有一条 `strconv` 会**同时返回夹到 MaxInt64 的值和 ErrRange** 的
// "99999999999999999999"。最后这条是承重的：它是全套件里唯一能钉住
// config.go:34 里 `convErr == nil` 那半边的输入 —— 删掉那个子句，前四条照样
// 回落（它们被 `depth >= 1` 拦下：Atoi 给它们返回 0 或负数），只有这条会被
// 当成 MaxInt64 收下。
func TestGarbageValuesFallBack(t *testing.T) {
	db := newDB(t)
	ctx := context.Background()
	for _, bad := range []string{"not-a-number", "0", "-4", "", "99999999999999999999"} {
		if err := db.ConfigSet(ctx, impactDepthKey, bad); err != nil {
			t.Fatal(err)
		}
		got, err := Load(ctx, db)
		if err != nil {
			t.Fatalf("%q must not fail Load: %v", bad, err)
		}
		if got.ImpactDepth != DefaultImpactDepth {
			t.Fatalf("%q fell back to %d", bad, got.ImpactDepth)
		}
	}
}

// R1: Save writes exactly what it is handed; the repair lives only in Load.
// Task 21's settings page writes user-typed values through this path, so the
// asymmetry is part of the contract: a change that makes Save clamp (raw no
// longer "0") or makes Load trust the stored value (returned depth no longer
// the default) must fail here.
func TestSaveDoesNotValidateButLoadRepairs(t *testing.T) {
	db := newDB(t)
	ctx := context.Background()
	if err := Save(ctx, db, Settings{ScanRoot: "", ImpactDepth: 0}); err != nil {
		t.Fatal(err)
	}
	raw, err := db.ConfigGet(ctx, impactDepthKey)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "0" {
		t.Fatalf("Save must store the value verbatim; raw = %q, want %q", raw, "0")
	}
	got, err := Load(ctx, db)
	if err != nil {
		t.Fatalf("bad stored depth must not fail Load: %v", err)
	}
	if got.ImpactDepth != DefaultImpactDepth {
		t.Fatalf("Load did not repair: got %d, want DefaultImpactDepth %d", got.ImpactDepth, DefaultImpactDepth)
	}
}

// R2: when the read itself fails, Load propagates the error and returns the
// zero-value Settings — whose ImpactDepth is 0, not DefaultImpactDepth.
// Contract: caller must not read Settings when err != nil. Verified against
// the committed driver: closing the DB makes ConfigGet return
// "sqlite: database is closed".
func TestLoadPropagatesReadErrorWithZeroValue(t *testing.T) {
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "cfg.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() }) // body closes first; second close is a no-op error, ignored
	if err := Save(context.Background(), db, Settings{ScanRoot: "/keep/me", ImpactDepth: 5}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := Load(context.Background(), db)
	if err == nil {
		t.Fatal("Load on a closed DB must return a non-nil error")
	}
	if got.ScanRoot != "" || got.ImpactDepth != 0 || got.GraphRepos != nil {
		t.Fatalf("on error Load must return the zero Settings; got %+v (ImpactDepth 0, not DefaultImpactDepth %d) — caller must not read it when err != nil", got, DefaultImpactDepth)
	}
}
