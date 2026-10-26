package model

import (
	"fmt"
	"time"
)

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

// NewSnapshotID is a fixed-width nanosecond stamp, so "ORDER BY id DESC" is
// "newest first" without a timestamp column doing double duty.
func NewSnapshotID() string { return fmt.Sprintf("s%019d", time.Now().UnixNano()) }
