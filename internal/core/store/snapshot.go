package store

import (
	"context"
	"fmt"

	"github.com/egoist/mygo/plugins/sqlite"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

const (
	// W2: one scan is one transaction. Every statement is pre-computed because
	// the driver takes a static batch.
	insertSnapshotSQL = `INSERT INTO architecture_snapshots
		(id, repo_id, commit_hash, branch, package_errors_json) VALUES (?, ?, ?, ?, ?)`
	selectLatestSnapshotSQL = `SELECT id FROM architecture_snapshots WHERE repo_id = ? ORDER BY id DESC LIMIT 1`
	selectSnapshotByID      = `SELECT id, repo_id, commit_hash, branch, package_errors_json, created_at
		FROM architecture_snapshots WHERE id = ?`
	selectSnapshotByElement = `SELECT snapshot_id FROM arch_elements WHERE element_id = ? ORDER BY snapshot_id DESC LIMIT 1`
)

func (d *DB) SaveSnapshot(ctx context.Context, s model.ArchitectureSnapshot,
	elements []model.Element, relationships []model.Relationship) error {
	stmts := make([]sqlite.Statement, 0, 1+len(elements)+len(relationships))
	stmts = append(stmts, sqlite.Statement{SQL: insertSnapshotSQL,
		Args: []any{s.ID, s.RepoID, s.CommitHash, s.Branch, jsonString(defaultStrings(s.PackageErrors))}})
	for _, e := range elements {
		stmts = append(stmts, sqlite.Statement{SQL: insertElementSQL,
			Args: []any{s.ID, e.ID, string(e.Type), e.Name, e.Description, e.RepoID, e.FilePath,
				jsonString(defaultStrings(e.Methods))}})
	}
	for _, r := range relationships {
		weight := r.Weight
		if weight < 1 {
			weight = 1
		}
		stmts = append(stmts, sqlite.Statement{SQL: insertRelationshipSQL,
			Args: []any{s.ID, r.FromID, r.ToID, string(r.Type), weight}})
	}
	if _, err := d.Transact(ctx, stmts); err != nil {
		return fmt.Errorf("store: save snapshot %s (%d statements): %w", s.ID, len(stmts), err)
	}
	return nil
}

func defaultStrings(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

func (d *DB) LatestSnapshotID(ctx context.Context, repoID string) (string, error) {
	rows, err := d.Query(ctx, selectLatestSnapshotSQL, repoID)
	if err != nil {
		return "", err
	}
	// No snapshot yet is a normal state (a repo the user registered but never
	// scanned), not an error: callers branch on the empty string.
	if len(rows.Values) == 0 {
		return "", nil
	}
	return str(rows.Values[0][0]), nil
}

// Snapshot reads one snapshot's own row. The UI's status line needs
// PackageErrors (spec §10: "完成，N 个包有错误"), which no other read path
// carries.
func (d *DB) Snapshot(ctx context.Context, snapshotID string) (model.ArchitectureSnapshot, error) {
	rows, err := d.Query(ctx, selectSnapshotByID, snapshotID)
	if err != nil {
		return model.ArchitectureSnapshot{}, err
	}
	out := rowsToMaps(rows)
	if len(out) == 0 {
		return model.ArchitectureSnapshot{}, fmt.Errorf("store: unknown snapshot %q", snapshotID)
	}
	row := out[0]
	return model.ArchitectureSnapshot{
		ID:            str(row["id"]),
		RepoID:        str(row["repo_id"]),
		CommitHash:    str(row["commit_hash"]),
		Branch:        str(row["branch"]),
		PackageErrors: stringList(str(row["package_errors_json"])),
		CreatedAt:     str(row["created_at"]),
	}, nil
}

// SnapshotForElement is what the CLI uses: an element id alone, latest snapshot
// that still carries it.
func (d *DB) SnapshotForElement(ctx context.Context, elementID string) (string, error) {
	rows, err := d.Query(ctx, selectSnapshotByElement, elementID)
	if err != nil {
		return "", err
	}
	if len(rows.Values) == 0 {
		return "", model.ErrUnknownElement
	}
	return str(rows.Values[0][0]), nil
}
