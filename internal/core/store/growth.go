package store

import (
	"context"
	"fmt"

	"github.com/egoist/mygo/plugins/sqlite"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

// The PK (snapshot_to, element_id, kind) makes the upsert below idempotent:
// re-diffing the same snapshot pair rewrites the same rows, nothing doubles.
const upsertGrowthEventSQL = `INSERT INTO growth_events
	(repo_id, snapshot_from, snapshot_to, element_id, kind, commit_hash)
	VALUES (?, ?, ?, ?, ?, ?)
	ON CONFLICT(snapshot_to, element_id, kind) DO UPDATE SET
		repo_id = excluded.repo_id, snapshot_from = excluded.snapshot_from,
		commit_hash = excluded.commit_hash`

// SaveGrowthEvents writes one diff round's events in a single transaction,
// mirroring the commits rule (W2): the growth story of a sync is
// all-or-nothing.
func (d *DB) SaveGrowthEvents(ctx context.Context, events []model.GrowthEvent) error {
	stmts := make([]sqlite.Statement, 0, len(events))
	for _, e := range events {
		stmts = append(stmts, sqlite.Statement{SQL: upsertGrowthEventSQL,
			Args: []any{e.RepoID, e.SnapshotFrom, e.SnapshotTo, e.ElementID, e.Kind, e.CommitHash}})
	}
	if len(stmts) == 0 {
		return nil
	}
	if _, err := d.Transact(ctx, stmts); err != nil {
		return fmt.Errorf("store: save %d growth events: %w", len(stmts), err)
	}
	return nil
}

const selectGrowthEventsSQL = `SELECT snapshot_from, snapshot_to, element_id, kind, commit_hash
	FROM growth_events WHERE repo_id = ? AND (? = '' OR element_id = ?)
	ORDER BY rowid DESC LIMIT ?`

// GrowthEvents reads a repo's growth story newest first; elementID "" asks
// for every element. limit follows the commits rule (cap so a long history
// cannot flood a view).
func (d *DB) GrowthEvents(ctx context.Context, repoID, elementID string, limit int) ([]model.GrowthEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	rows, err := d.Query(ctx, selectGrowthEventsSQL, repoID, elementID, elementID, limit)
	if err != nil {
		return nil, fmt.Errorf("store: growth events %s: %w", repoID, err)
	}
	out := make([]model.GrowthEvent, 0, len(rows.Values))
	for _, row := range rowsToMaps(rows) {
		out = append(out, model.GrowthEvent{RepoID: repoID,
			SnapshotFrom: str(row["snapshot_from"]), SnapshotTo: str(row["snapshot_to"]),
			ElementID: str(row["element_id"]), Kind: str(row["kind"]),
			CommitHash: str(row["commit_hash"])})
	}
	return out, nil
}