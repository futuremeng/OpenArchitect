package store

import (
	"context"
	"fmt"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

const upsertRepoSQL = `INSERT INTO repos (id, name, local_path, lang) VALUES (?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET name = excluded.name, local_path = excluded.local_path,
	lang = excluded.lang, updated_at = datetime('now')`

func (d *DB) UpsertRepo(ctx context.Context, r model.Repo) error {
	if _, err := d.Execute(ctx, upsertRepoSQL, r.ID, r.Name, r.LocalPath, r.Lang); err != nil {
		return fmt.Errorf("store: upsert repo %s: %w", r.ID, err)
	}
	return nil
}

func (d *DB) Repos(ctx context.Context) ([]model.Repo, error) {
	rows, err := d.Query(ctx, `SELECT id, name, local_path, lang FROM repos ORDER BY name`)
	if err != nil {
		return nil, err
	}
	out := make([]model.Repo, 0, len(rows.Values))
	for _, row := range rowsToMaps(rows) {
		out = append(out, model.Repo{ID: str(row["id"]), Name: str(row["name"]),
			LocalPath: str(row["local_path"]), Lang: str(row["lang"])})
	}
	return out, nil
}
