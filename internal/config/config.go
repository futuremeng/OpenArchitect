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
	graphRepoKey   = "graph.repo"
)

type Settings struct {
	ScanRoot    string
	ImpactDepth int
	// GraphRepo 是图页钉住的仓库 id（切片 1 的"仓库过滤器"最小版）：空 =
	// 跟随全局最新快照；指向的仓库没有快照时同样回落到全局最新。
	GraphRepo string
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
	pinned, err := db.ConfigGet(ctx, graphRepoKey)
	if err != nil {
		return Settings{}, err
	}
	out := Settings{ScanRoot: root, ImpactDepth: DefaultImpactDepth, GraphRepo: pinned}
	if depth, convErr := strconv.Atoi(raw); convErr == nil && depth >= 1 {
		out.ImpactDepth = depth
	}
	return out, nil
}

func Save(ctx context.Context, db *store.DB, s Settings) error {
	if err := db.ConfigSet(ctx, scanRootKey, s.ScanRoot); err != nil {
		return err
	}
	if err := db.ConfigSet(ctx, impactDepthKey, strconv.Itoa(s.ImpactDepth)); err != nil {
		return err
	}
	return db.ConfigSet(ctx, graphRepoKey, s.GraphRepo)
}
