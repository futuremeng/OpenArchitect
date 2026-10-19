package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestOpenRoundTrip(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	// 绑进去的「值」故意带注入形状的字符与单引号，验证它们只会作为 ? 参数
	// 进去，不参与 SQL 文本。
	if _, err := db.Execute(ctx, `CREATE TABLE needle (name TEXT)`); err != nil {
		t.Fatalf("CREATE: %v", err)
	}
	const tricky = `带'引号;和分号`
	if _, err := db.Execute(ctx, `INSERT INTO needle (name) VALUES (?)`, tricky); err != nil {
		t.Fatalf("INSERT: %v", err)
	}
	rows, err := db.Query(ctx, `SELECT name FROM needle`)
	if err != nil {
		t.Fatalf("SELECT: %v", err)
	}
	if len(rows.Values) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows.Values))
	}
	if got := rows.Values[0][0]; got != tricky {
		t.Fatalf("round trip = %v, want %v", got, tricky)
	}
}

func TestForeignKeysEnabled(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	rows, err := db.Query(context.Background(), `PRAGMA foreign_keys`)
	if err != nil {
		t.Fatalf("PRAGMA: %v", err)
	}
	if got := num(rows.Values[0][0]); got != 1 {
		t.Fatalf("foreign_keys = %v, want 1", rows.Values[0][0])
	}
}

// newTestDB 收 testing.TB：测试与基准（bench_test.go 的 PoC-5）共用。
func newTestDB(t testing.TB) *DB {
	t.Helper()
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return db
}
