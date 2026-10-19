package scanner

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
	"github.com/futuremeng/OpenArchitect/internal/core/store"
	"github.com/futuremeng/OpenArchitect/internal/scanner/language"
)

// Run scans root and writes one snapshot. A cancelled ctx or a failed parse
// writes nothing, so the caller keeps showing the previous snapshot (spec §10).
func Run(ctx context.Context, db *store.DB, root string) (Report, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return Report{}, err
	}
	name, err := Detect(abs)
	if err != nil {
		return Report{}, err
	}
	adapter, ok := language.Get(name)
	if !ok {
		return Report{}, ErrUnsupportedLanguage
	}
	out, err := adapter.Parse(ctx, abs)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Report{}, ctxErr
		}
		return Report{}, fmt.Errorf("scanner: parse %s: %w", abs, err)
	}
	if err := ctx.Err(); err != nil {
		return Report{}, err
	}

	repo := model.Repo{ID: model.RepoIDFor(abs), Name: out.ModulePath, LocalPath: abs, Lang: name}
	if err := db.UpsertRepo(ctx, repo); err != nil {
		return Report{}, err
	}
	snapshot := model.ArchitectureSnapshot{
		ID: model.NewSnapshotID(), RepoID: repo.ID, PackageErrors: out.PackageErrors,
	}
	if err := db.SaveSnapshot(ctx, snapshot, out.Elements, out.Relationships); err != nil {
		return Report{}, err
	}
	return Report{
		SnapshotID: snapshot.ID, RepoID: repo.ID, ModulePath: out.ModulePath, Language: name,
		Elements: len(out.Elements), Relationships: len(out.Relationships),
		PackageErrors: out.PackageErrors,
	}, nil
}
