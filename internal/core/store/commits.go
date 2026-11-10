package store

import (
	"context"
	"fmt"

	"github.com/egoist/mygo/plugins/sqlite"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

const upsertCommitSQL = `INSERT INTO commits
	(repo_id, hash, author, author_email, date, message, agent_tag, files_json)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(repo_id, hash) DO UPDATE SET author = excluded.author,
		author_email = excluded.author_email, date = excluded.date,
		message = excluded.message, agent_tag = excluded.agent_tag,
		files_json = excluded.files_json`

const selectCommitsSQL = `SELECT hash, author, author_email, date, message, agent_tag, files_json
	FROM commits WHERE repo_id = ? ORDER BY date DESC, hash DESC LIMIT ?`

// SaveCommits writes a batch of commit records in one transaction, mirroring
// the snapshot rule (W2): bookkeeping is all-or-nothing per sync.
func (d *DB) SaveCommits(ctx context.Context, commits []model.Commit) error {
	stmts := make([]sqlite.Statement, 0, len(commits))
	for _, c := range commits {
		stmts = append(stmts, sqlite.Statement{SQL: upsertCommitSQL,
			Args: []any{c.RepoID, c.Hash, c.Author, c.AuthorEmail, c.Date, c.Message,
				c.AgentTag, jsonString(defaultStrings(c.Files))}})
	}
	if len(stmts) == 0 {
		return nil
	}
	if _, err := d.Transact(ctx, stmts); err != nil {
		return fmt.Errorf("store: save %d commits: %w", len(stmts), err)
	}
	return nil
}

// Commits reads a repo's recorded commits, newest first. limit <= 0 means
// "no opinion" and lands on a sane cap so a huge history cannot flood a view.
func (d *DB) Commits(ctx context.Context, repoID string, limit int) ([]model.Commit, error) {
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	rows, err := d.Query(ctx, selectCommitsSQL, repoID, limit)
	if err != nil {
		return nil, fmt.Errorf("store: commits %s: %w", repoID, err)
	}
	out := make([]model.Commit, 0, len(rows.Values))
	for _, row := range rowsToMaps(rows) {
		out = append(out, model.Commit{RepoID: repoID, Hash: str(row["hash"]),
			Author: str(row["author"]), AuthorEmail: str(row["author_email"]),
			Date: str(row["date"]), Message: str(row["message"]),
			AgentTag: str(row["agent_tag"]), Files: stringList(str(row["files_json"]))})
	}
	return out, nil
}

// Fruits is the 果实 query: commits attributed to an agent (or mixed),
// newest first. Attribution is a strong hint, not an adjudication
// (growth spec §6 risk 4) — mixed rides along and the UI marks it apart.
func (d *DB) Fruits(ctx context.Context, repoID string, limit int) ([]model.Commit, error) {
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	rows, err := d.Query(ctx, `SELECT hash, author, author_email, date, message, agent_tag, files_json
		FROM commits WHERE repo_id = ? AND agent_tag != 'human'
		ORDER BY date DESC, hash DESC LIMIT ?`, repoID, limit)
	if err != nil {
		return nil, fmt.Errorf("store: fruits %s: %w", repoID, err)
	}
	out := make([]model.Commit, 0, len(rows.Values))
	for _, row := range rowsToMaps(rows) {
		out = append(out, model.Commit{RepoID: repoID, Hash: str(row["hash"]),
			Author: str(row["author"]), AuthorEmail: str(row["author_email"]),
			Date: str(row["date"]), Message: str(row["message"]),
			AgentTag: str(row["agent_tag"]), Files: stringList(str(row["files_json"]))})
	}
	return out, nil
}