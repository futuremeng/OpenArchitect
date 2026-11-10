package store

import (
	"context"
	"fmt"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

const upsertRepoSQL = `INSERT INTO repos (id, name, local_path, lang) VALUES (?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET name = excluded.name, local_path = excluded.local_path,
	lang = excluded.lang, updated_at = datetime('now')`

// repos LEFT JOIN repo_sync: sync fields ride along so every existing caller
// of Repos keeps compiling; unregistered-for-sync repos read as state "new".
const selectReposSQL = `SELECT r.id, r.name, r.local_path, r.lang,
	COALESCE(s.remote_url, '') AS remote_url, COALESCE(s.sync_state, 'new') AS sync_state
	FROM repos r LEFT JOIN repo_sync s ON s.repo_id = r.id ORDER BY r.name`

func (d *DB) UpsertRepo(ctx context.Context, r model.Repo) error {
	if _, err := d.Execute(ctx, upsertRepoSQL, r.ID, r.Name, r.LocalPath, r.Lang); err != nil {
		return fmt.Errorf("store: upsert repo %s: %w", r.ID, err)
	}
	return nil
}

func (d *DB) Repos(ctx context.Context) ([]model.Repo, error) {
	rows, err := d.Query(ctx, selectReposSQL)
	if err != nil {
		return nil, err
	}
	out := make([]model.Repo, 0, len(rows.Values))
	for _, row := range rowsToMaps(rows) {
		out = append(out, model.Repo{ID: str(row["id"]), Name: str(row["name"]),
			LocalPath: str(row["local_path"]), Lang: str(row["lang"]),
			RemoteURL: str(row["remote_url"]), SyncState: str(row["sync_state"])})
	}
	return out, nil
}

// UpsertRepoSync writes the forest bookkeeping row (growth spec 切片 A).
// Callers own the state machine; this is storage, not policy.
func (d *DB) UpsertRepoSync(ctx context.Context, repoID, remoteURL, syncState, lastHead string) error {
	_, err := d.Execute(ctx, `INSERT INTO repo_sync (repo_id, remote_url, sync_state, last_head)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(repo_id) DO UPDATE SET remote_url = excluded.remote_url,
			sync_state = excluded.sync_state, last_head = excluded.last_head,
			updated_at = datetime('now')`, repoID, remoteURL, syncState, lastHead)
	if err != nil {
		return fmt.Errorf("store: upsert repo_sync %s: %w", repoID, err)
	}
	return nil
}

// RepoSync reads one repo's sync row. A repo without one is not an error:
// it simply reports state "new" with empty fields.
func (d *DB) RepoSync(ctx context.Context, repoID string) (remoteURL, syncState, lastHead string, err error) {
	rows, err := d.Query(ctx, `SELECT remote_url, sync_state, last_head FROM repo_sync WHERE repo_id = ?`, repoID)
	if err != nil {
		return "", "", "", fmt.Errorf("store: repo_sync %s: %w", repoID, err)
	}
	if len(rows.Values) == 0 {
		return "", "new", "", nil
	}
	row := rowsToMaps(rows)[0]
	return str(row["remote_url"]), str(row["sync_state"]), str(row["last_head"]), nil
}
