package store

import (
	"context"
	"fmt"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

const (
	insertRelationshipSQL = `INSERT INTO arch_relationships
		(snapshot_id, from_id, to_id, relationship_type, weight) VALUES (?, ?, ?, ?, ?)`
	selectRelsBySnapshot = `SELECT from_id, to_id, relationship_type, weight FROM arch_relationships
		WHERE snapshot_id = ? ORDER BY from_id, to_id, relationship_type`
)

func (d *DB) Relationships(ctx context.Context, snapshotID string) ([]model.Relationship, error) {
	rows, err := d.Query(ctx, selectRelsBySnapshot, snapshotID)
	if err != nil {
		return nil, fmt.Errorf("store: relationships: %w", err)
	}
	out := make([]model.Relationship, 0, len(rows.Values))
	for _, row := range rowsToMaps(rows) {
		out = append(out, model.Relationship{
			FromID: str(row["from_id"]), ToID: str(row["to_id"]),
			Type: model.RelType(str(row["relationship_type"])), Weight: int(num(row["weight"])),
		})
	}
	return out, nil
}
