// Package bad does not type-check: `Total` returns a string where an int is
// declared, which is the deliberate per-package error spec §10 row 1 says must
// not abort the scan.
package bad

// Ledger would be a Component if this package type-checked.
type Ledger struct {
	Entries int
}

// Total does not compile: an untyped string where an int is required.
func (l *Ledger) Total() int { return "not an int" }
