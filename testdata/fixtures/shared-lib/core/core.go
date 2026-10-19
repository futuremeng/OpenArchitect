// Package core imports edge, and edge imports core: a real package cycle the
// containment walk has to terminate on (spec §6.3).
package core

import "example.com/shared/edge"

// Server is the shared service type.
type Server struct{ Port int }

// Addr calls a package function. A package function has no Component subject,
// so this produces no calls edge (spec §6.4).
func (s *Server) Addr() string { return edge.Version() }

// Ledger lives in the same package as Server, so the edges below it are
// intra-package: stored for impact analysis, not drawn.
type Ledger struct{ server Server }

// Bind touches its sibling.
func (l *Ledger) Bind() string { return l.server.Addr() }

// Snapshot is a package-level function: its owner is the nearest preceding
// component declaration in this file (Ledger), which pins edgeOwnership's
// fallback (spec §6.4 "A 所在文件").
func Snapshot(l Ledger) string { return l.server.Addr() }
