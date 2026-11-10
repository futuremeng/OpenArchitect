package model

// Repo keeps only fields this slice has a source for: URL and Branch were
// dropped deliberately (spec deviation 9). v2 (1.0 plan) restores RemoteURL
// and adds SyncState, but they live in the repo_sync side table, not in the
// repos row — so the deviation-9 decision on the repos table itself stands.
type Repo struct {
	ID        string
	Name      string
	LocalPath string
	Lang      string
	RemoteURL string
	SyncState string // new / tracked / stale (growth spec §4 切片 A)
}
