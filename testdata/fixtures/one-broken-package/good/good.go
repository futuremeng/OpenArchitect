// Package good type-checks cleanly. Its Widget must still become an element
// even though a sibling package of the same module does not type-check
// (spec §10: 包级类型错误不中断，成功解析的包照常入库).
package good

// Widget is the healthy package's component.
type Widget struct {
	Label string
}

// Rename sets the label.
func (w *Widget) Rename(label string) { w.Label = label }
