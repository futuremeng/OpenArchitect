package store

import (
	"context"
	"fmt"
)

// Migrate applies every migration above the stored user_version, in order.
// Migrations run outside a transaction and user_version is bumped only after
// every statement of a version succeeds, so each statement must be replay-safe.
func (d *DB) Migrate(ctx context.Context) error {
	current, err := d.UserVersion(ctx)
	if err != nil {
		return err
	}
	if current > schemaVersion {
		return fmt.Errorf("store: database schema %d is newer than code schema %d", current, schemaVersion)
	}
	for version := current + 1; version <= schemaVersion; version++ {
		for _, statement := range migrations[version-1] {
			if _, err := d.Execute(ctx, statement); err != nil {
				return fmt.Errorf("store: migrate to %d: %w", version, err)
			}
		}
		if err := d.SetUserVersion(ctx, version); err != nil {
			return fmt.Errorf("store: migrate version %d: %w", version, err)
		}
	}
	return nil
}
