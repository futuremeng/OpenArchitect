// Package model holds the C4-shaped architecture vocabulary shared by the
// scanner, the store, the graph and the UI. It imports no UI stack (R1).
package model

import "errors"

// ErrUnknownElement 是「这个 id 不在快照里」的哨兵：store 读路径（Task 7）
// 与 graph.Analyze（Task 13）共用，UI 抽屉用它区分查无与读库失败。
var ErrUnknownElement = errors.New("model: unknown element")

type ElementType string

const (
	TypeSystem    ElementType = "System"
	TypeContainer ElementType = "Container"
	TypeComponent ElementType = "Component"
	// TypeFile is the treemap's area unit (PoC-13: element-count area made
	// 41% of OpenArchitect's packages invisible). A File element is canopy
	// material and diff material — it never enters the C4 graph view.
	TypeFile ElementType = "File"
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
