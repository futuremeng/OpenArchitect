package model

// ArchitectureSnapshot is one scan. CommitHash/Branch stay empty in this slice
// (no Git), and PackageErrors keeps the per-package failures collected during
// parsing (spec §10) for audit only.
type ArchitectureSnapshot struct {
	ID            string
	RepoID        string
	CommitHash    string
	Branch        string
	PackageErrors []string
	CreatedAt     string
}
