// Package store keeps the local SQLite database. It is the only package in
// the module allowed to import mygo's sqlite driver (spec §5.1, R1 exception).
package store

import (
	"context"
	"fmt"
	"strconv"

	"github.com/egoist/mygo/plugins/sqlite"
)

type DB struct {
	handle *sqlite.DB
	path   string
}

func Open(ctx context.Context, path string) (*DB, error) {
	handle, err := sqlite.Open(ctx, path, sqlite.OpenOptions{})
	if err != nil {
		return nil, fmt.Errorf("store: open %s: %w", path, err)
	}
	db := &DB{handle: handle, path: path}
	if _, err := db.Execute(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		_ = handle.Close()
		return nil, fmt.Errorf("store: foreign_keys: %w", err)
	}
	if err := db.Migrate(ctx); err != nil {
		_ = handle.Close()
		return nil, err
	}
	return db, nil
}

func (d *DB) Path() string { return d.path }

func (d *DB) Execute(ctx context.Context, sql string, args ...any) (sqlite.Result, error) {
	return d.handle.Execute(ctx, sql, args...)
}

func (d *DB) Query(ctx context.Context, sql string, args ...any) (sqlite.Rows, error) {
	return d.handle.Query(ctx, sql, args...)
}

func (d *DB) Transact(ctx context.Context, stmts []sqlite.Statement) ([]sqlite.Result, error) {
	return d.handle.Transaction(ctx, stmts)
}

func (d *DB) Close() error { return d.handle.Close() }

func (d *DB) UserVersion(ctx context.Context) (int, error) {
	rows, err := d.Query(ctx, `PRAGMA user_version`)
	if err != nil {
		return 0, err
	}
	if len(rows.Values) == 0 || len(rows.Values[0]) == 0 {
		return 0, nil
	}
	raw, err := strconv.Atoi(fmt.Sprint(rows.Values[0][0]))
	if err != nil {
		return 0, fmt.Errorf("store: user_version: %w", err)
	}
	return raw, nil
}

// SetUserVersion interpolates its argument because SQLite takes no placeholder
// in a PRAGMA. The value only ever comes from the package's own constant.
func (d *DB) SetUserVersion(ctx context.Context, version int) error {
	_, err := d.Execute(ctx, `PRAGMA user_version = `+strconv.Itoa(version))
	return err
}
