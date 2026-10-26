package scanner

// Report is one scan's summary: what was written, and which packages failed.
type Report struct {
	SnapshotID    string
	RepoID        string
	ModulePath    string
	Language      string
	Elements      int
	Relationships int
	PackageErrors []string
}
