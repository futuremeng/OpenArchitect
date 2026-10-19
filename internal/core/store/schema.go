package store

// schemaVersion is the value written to PRAGMA user_version by Migrate.
// Every future schema change appends a new []string to migrations and bumps
// this constant; migrations[i] brings a database from version i to i+1.
const schemaVersion = 1

// migrations[0] is v1: the five spec §7.1 tables plus their indexes. DDL text
// is verbatim from the spec with two settled amendments — plan finalized
// decision #1 adds arch_elements.methods_json, and idx_elements_type (which
// actually indexed element_id) is renamed idx_elements_element.
//
// Everything is CREATE ... IF NOT EXISTS so replaying a version over a
// partially applied database is safe; user_version is bumped only after a
// whole version's statements succeed. No statement here interpolates input:
// they are constants executed as-is (W3 / the ?-placeholder rule).
var migrations = [][]string{{
	`CREATE TABLE IF NOT EXISTS repos (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		local_path TEXT NOT NULL,
		lang TEXT NOT NULL,
		created_at TEXT DEFAULT (datetime('now')),
		updated_at TEXT DEFAULT (datetime('now'))
	)`,
	`CREATE TABLE IF NOT EXISTS architecture_snapshots (
		id TEXT PRIMARY KEY,
		repo_id TEXT NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
		commit_hash TEXT NOT NULL DEFAULT '',
		branch TEXT NOT NULL DEFAULT '',
		package_errors_json TEXT NOT NULL DEFAULT '[]',
		created_at TEXT DEFAULT (datetime('now'))
	)`,
	`CREATE INDEX IF NOT EXISTS idx_snapshots_repo ON architecture_snapshots(repo_id)`,
	`CREATE TABLE IF NOT EXISTS arch_elements (
		snapshot_id TEXT NOT NULL REFERENCES architecture_snapshots(id) ON DELETE CASCADE,
		element_id TEXT NOT NULL,
		element_type TEXT NOT NULL,
		name TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		repo_id TEXT NOT NULL,
		file_path TEXT NOT NULL DEFAULT '',
		methods_json TEXT NOT NULL DEFAULT '[]',
		PRIMARY KEY (snapshot_id, element_id)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_elements_repo ON arch_elements(repo_id)`,
	`CREATE INDEX IF NOT EXISTS idx_elements_name ON arch_elements(name)`,
	`CREATE INDEX IF NOT EXISTS idx_elements_element ON arch_elements(element_id)`,
	`CREATE TABLE IF NOT EXISTS arch_relationships (
		snapshot_id TEXT NOT NULL REFERENCES architecture_snapshots(id) ON DELETE CASCADE,
		from_id TEXT NOT NULL,
		to_id TEXT NOT NULL,
		relationship_type TEXT NOT NULL,
		weight INTEGER NOT NULL DEFAULT 1,
		PRIMARY KEY (snapshot_id, from_id, to_id, relationship_type)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_rel_from ON arch_relationships(from_id)`,
	`CREATE INDEX IF NOT EXISTS idx_rel_to ON arch_relationships(to_id)`,
	`CREATE TABLE IF NOT EXISTS app_config (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	)`,
}}
