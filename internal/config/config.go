// Package config types the app_config table. User settings only: the schema
// version lives in PRAGMA user_version, not here (spec §7.1).
package config

import (
	"context"
	"strconv"
	"strings"

	"github.com/futuremeng/OpenArchitect/internal/core/store"
)

const (
	DefaultImpactDepth = 2

	scanRootKey    = "scan.root"
	impactDepthKey = "impact.depth"
	graphRepoKey   = "graph.repo"
	graphReposKey  = "graph.repos"
)

type Settings struct {
	ScanRoot    string
	ImpactDepth int
	// GraphRepos 是图页勾选的仓库 id 集合（多选过滤器）。空 = 全选：
	// 所有带快照的仓都进合并视图，新扫描的仓也自动进集合。旧版单仓钉选
	// （graph.repo）在 Load 里迁移成单元素集合。
	GraphRepos []string
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
	repos, err := db.ConfigGet(ctx, graphReposKey)
	if err != nil {
		return Settings{}, err
	}
	out := Settings{ScanRoot: root, ImpactDepth: DefaultImpactDepth, GraphRepos: splitRepos(repos)}
	if depth, convErr := strconv.Atoi(raw); convErr == nil && depth >= 1 {
		out.ImpactDepth = depth
	}
	if len(out.GraphRepos) == 0 {
		// 一次性迁移：旧版只有一个钉选位。读到就搬进新键，读不到保持空
		// （= 全选）。迁移只发生在 Load，Save 从此只写 graph.repos。
		legacy, err := db.ConfigGet(ctx, graphRepoKey)
		if err != nil {
			return Settings{}, err
		}
		out.GraphRepos = splitRepos(legacy)
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
	return db.ConfigSet(ctx, graphReposKey, strings.Join(s.GraphRepos, ","))
}

// splitRepos parses the comma-joined list, dropping empty fields so a stray
// trailing comma cannot fake a selected repo.
func splitRepos(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
