// Package config types the app_config table. User settings only: the schema
// version lives in PRAGMA user_version, not here (spec §7.1).
package config

import (
	"context"
	"strconv"

	"github.com/futuremeng/OpenArchitect/internal/core/store"
)

const (
	DefaultImpactDepth = 2

	scanRootKey    = "scan.root"
	impactDepthKey = "impact.depth"
)

type Settings struct {
	ScanRoot    string
	ImpactDepth int
}

func Load(ctx context.Context, db *store.DB) (Settings, error) {
	root, err := db.ConfigGet(ctx, scanRootKey)
	if err != nil {
		return Settings{}, err
	}
	raw, err := db.ConfigGet(ctx, impactDepthKey)
	if err != nil {
		return Settings{}, err
	}
	out := Settings{ScanRoot: root, ImpactDepth: DefaultImpactDepth}
	if depth, convErr := strconv.Atoi(raw); convErr == nil && depth >= 1 {
		out.ImpactDepth = depth
	}
	return out, nil
}

func Save(ctx context.Context, db *store.DB, s Settings) error {
	if err := db.ConfigSet(ctx, scanRootKey, s.ScanRoot); err != nil {
		return err
	}
	return db.ConfigSet(ctx, impactDepthKey, strconv.Itoa(s.ImpactDepth))
}
