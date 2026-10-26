package store

import "context"

const (
	upsertConfigSQL = `INSERT INTO app_config (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`
	selectConfigSQL = `SELECT value FROM app_config WHERE key = ?`
)

func (d *DB) ConfigGet(ctx context.Context, key string) (string, error) {
	rows, err := d.Query(ctx, selectConfigSQL, key)
	if err != nil {
		return "", err
	}
	if len(rows.Values) == 0 {
		return "", nil
	}
	return str(rows.Values[0][0]), nil
}

func (d *DB) ConfigSet(ctx context.Context, key, value string) error {
	_, err := d.Execute(ctx, upsertConfigSQL, key, value)
	return err
}
