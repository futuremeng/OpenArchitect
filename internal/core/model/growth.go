package model

// GrowthEvent is one element-level change between two adjacent snapshots of
// the same repo (growth spec 切片 B). Kind is one of the Growth* constants.
// CommitHash is attribution by file overlap — a strong hint like AgentTag,
// empty when no commit in the range provably touched the element (growth
// spec §6 risk 4 applies to attribution too).
type GrowthEvent struct {
	RepoID       string
	SnapshotFrom string
	SnapshotTo   string
	ElementID    string
	Kind         string
	CommitHash   string
}

// Event kinds, verbatim from the growth spec 切片 B.
const (
	GrowthAdded             = "added"
	GrowthRemoved           = "removed"
	GrowthSignatureChanged  = "signature-changed"
	GrowthEdgeChanged       = "edge-changed"
)