// Package model holds the C4-shaped architecture vocabulary shared by the
// scanner, the store, the graph and the UI. It imports no UI stack (R1).
package model

type ElementType string

const (
	TypeSystem    ElementType = "System"
	TypeContainer ElementType = "Container"
	TypeComponent ElementType = "Component"
)

type Element struct {
	ID          string
	Type        ElementType
	Name        string
	Description string
	RepoID      string
	FilePath    string
	// Methods lists exported method names of a Component. Plan amendment 1.
	Methods []string
}
