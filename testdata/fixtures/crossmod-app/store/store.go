package store

import (
	"sync"

	depstore "crossmod.example/dep/store"
)

// Gateway talks to a foreign store across the module boundary.
type Gateway struct {
	mu sync.Mutex
	db depstore.DB
}

// Write reaches into the foreign module twice: a call and a type use.
func (g *Gateway) Write() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.db.Save()
}
