// Package store keeps the fixture's shared data.
package store

// Cart is shared by both commands.
type Cart struct {
	Items []string
}

// Add appends an item.
func (c *Cart) Add(name string) { c.Items = append(c.Items, name) }

// Total reports how many items were added.
func (c *Cart) Total() int { return len(c.Items) }

// unexported must never become an element.
type hidden struct{}

func newHidden() hidden { return hidden{} }
