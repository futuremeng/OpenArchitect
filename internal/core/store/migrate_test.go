package store

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"testing"
)

func TestMigrateCreatesFiveTables(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	rows, err := db.Query(context.Background(),
		`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	var got []string
	for _, v := range rows.Values {
		got = append(got, str(v[0]))
	}
	sort.Strings(got)
	want := []string{"app_config", "arch_elements", "arch_relationships", "architecture_snapshots", "repos"}
	if len(got) != len(want) {
		t.Fatalf("tables = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tables = %v, want %v", got, want)
		}
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	for i := 0; i < 3; i++ {
		if err := db.Migrate(context.Background()); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
	v, err := db.UserVersion(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if v != schemaVersion {
		t.Fatalf("user_version = %d, want %d", v, schemaVersion)
	}
}

// 同一个文件第二次 Open：已迁移的库上 Migrate 必须是 no-op，不报错、版本不回退。
func TestReopenMigratedDBIsNoOp(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "reopen.db")
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	db2, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer db2.Close()
	v, err := db2.UserVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if v != schemaVersion {
		t.Fatalf("user_version after reopen = %d, want %d", v, schemaVersion)
	}
}

func TestSnapshotCascadeDeletesChildren(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	ctx := context.Background()
	for _, q := range []string{
		`INSERT INTO repos (id, name, local_path, lang) VALUES ('repo:/x', 'x', '/x', 'go')`,
		`INSERT INTO architecture_snapshots (id, repo_id) VALUES ('s1', 'repo:/x')`,
		`INSERT INTO arch_elements (snapshot_id, element_id, element_type, name, repo_id) VALUES ('s1', 'cmp:a.B', 'Component', 'B', 'repo:/x')`,
		`INSERT INTO arch_relationships (snapshot_id, from_id, to_id, relationship_type) VALUES ('s1', 'cmp:a.B', 'cmp:c.D', 'calls')`,
	} {
		if _, err := db.Execute(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if _, err := db.Execute(ctx, `DELETE FROM repos WHERE id = 'repo:/x'`); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"architecture_snapshots", "arch_elements", "arch_relationships"} {
		rows, err := db.Query(ctx, `SELECT count(*) FROM `+table)
		if err != nil {
			t.Fatalf("%s: %v", table, err)
		}
		if got := num(rows.Values[0][0]); got != 0 {
			t.Fatalf("%s survived cascade: count = %v", table, got)
		}
	}
}

// 索引名要能被查到：idx_elements_repo 存在过就说明 CREATE INDEX 都跑了。
func TestIndexesExist(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	rows, err := db.Query(context.Background(),
		`SELECT name FROM sqlite_master WHERE type='index' AND name LIKE 'idx_%'`)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, v := range rows.Values {
		got[str(v[0])] = true
	}
	for _, want := range []string{"idx_snapshots_repo", "idx_elements_repo", "idx_elements_name",
		"idx_elements_element", "idx_rel_from", "idx_rel_to"} {
		if !got[want] {
			t.Fatalf("missing index %s (have %v)", want, got)
		}
	}
}

// Task 2 审查要求：PRAGMA user_version 的写读往返，且高于 schemaVersion 时 Migrate 拒绝降级。
func TestUserVersionRoundTrip(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	ctx := context.Background()
	if err := db.SetUserVersion(ctx, 7); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, err := db.UserVersion(ctx)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != 7 {
		t.Fatalf("user_version = %d, want 7", got)
	}
	if err := db.Migrate(ctx); err == nil {
		t.Fatal("Migrate on newer schema: want error, got nil")
	}
}

// str 与 db_test.go 的 num 同理是测试期取值助手，Task 7 之后归 rows.go 侧的正式映射取代。
func str(v any) string {
	s, ok := v.(string)
	if !ok {
		panic(fmt.Sprintf("store test: unexpected column type %T (%v)", v, v))
	}
	return s
}
