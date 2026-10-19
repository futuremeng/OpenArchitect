package store

import (
	"context"
	"fmt"

	"github.com/egoist/mygo/plugins/sqlite"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

const (
	insertElementSQL = `INSERT INTO arch_elements
		(snapshot_id, element_id, element_type, name, description, repo_id, file_path, methods_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	elementColumns = `element_id, element_type, name, description, repo_id, file_path, methods_json`
	// LIKE needs ESCAPE because we escape the pattern ourselves.
	selectElementsBySnapshot = `SELECT ` + elementColumns + ` FROM arch_elements
		WHERE snapshot_id = ? ORDER BY element_id`
	selectElementByID = `SELECT ` + elementColumns + ` FROM arch_elements
		WHERE snapshot_id = ? AND element_id = ?`
	// `, element_id` 是并列名的 tie-breaker：620 组件下同名可撞 LIMIT 边界，
	// 没有它「哪 50 行返回」不确定，Task 20 的命令面板与 golden 比对会 flake。
	selectElementSearch = `SELECT ` + elementColumns + ` FROM arch_elements
		WHERE snapshot_id = ? AND name LIKE ? ESCAPE '\' ORDER BY name, element_id LIMIT ?`
)

// maxSearchResults 是命令面板的取回上限（Task 20 的输入不做本地二次截断）。
// 走 `LIMIT ?` 而不是拼进 SQL：全局约束只允许 `PRAGMA user_version` 一处插值。
const maxSearchResults = 50

func elementsFromRows(rows sqlite.Rows) []model.Element {
	out := make([]model.Element, 0, len(rows.Values))
	for _, row := range rowsToMaps(rows) {
		out = append(out, model.Element{
			ID:          str(row["element_id"]),
			Type:        model.ElementType(str(row["element_type"])),
			Name:        str(row["name"]),
			Description: str(row["description"]),
			RepoID:      str(row["repo_id"]),
			FilePath:    str(row["file_path"]),
			Methods:     stringList(str(row["methods_json"])),
		})
	}
	return out
}

func (d *DB) Elements(ctx context.Context, snapshotID string) ([]model.Element, error) {
	rows, err := d.Query(ctx, selectElementsBySnapshot, snapshotID)
	if err != nil {
		return nil, fmt.Errorf("store: elements: %w", err)
	}
	return elementsFromRows(rows), nil
}

func (d *DB) GetElement(ctx context.Context, snapshotID, elementID string) (model.Element, error) {
	rows, err := d.Query(ctx, selectElementByID, snapshotID, elementID)
	if err != nil {
		return model.Element{}, err
	}
	out := elementsFromRows(rows)
	if len(out) == 0 {
		return model.Element{}, model.ErrUnknownElement
	}
	return out[0], nil
}

func (d *DB) SearchElements(ctx context.Context, snapshotID, query string) ([]model.Element, error) {
	rows, err := d.Query(ctx, selectElementSearch, snapshotID, "%"+escapeLike(query)+"%", maxSearchResults)
	if err != nil {
		return nil, fmt.Errorf("store: search %q: %w", query, err)
	}
	return elementsFromRows(rows), nil
}
