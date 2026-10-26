// Package language defines the pluggable project parser. Slice 1 registers
// only Go; later languages arrive as sidecar adapters, never cgo (spec §3.6).
//
// Register is only called from package init funcs (Task 9 onward), so the
// registry map needs no locking.
package language

import (
	"context"
	"sort"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
)

type Output struct {
	ModulePath    string
	Elements      []model.Element
	Relationships []model.Relationship
	PackageErrors []string
}

type Adapter interface {
	// Language is the registry key, lowercase, e.g. "go".
	Language() string
	// Detect reports whether root is a project of this language. A non-nil
	// error means the directory could not be read at all, which the UI must
	// phrase differently from "not this language".
	Detect(root string) (bool, error)
	Parse(ctx context.Context, root string) (Output, error)
}

var registry = map[string]Adapter{}

func Register(a Adapter) {
	if _, taken := registry[a.Language()]; taken {
		panic("language: adapter registered twice: " + a.Language())
	}
	registry[a.Language()] = a
}

func All() []Adapter {
	out := make([]Adapter, 0, len(registry))
	for _, a := range registry {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Language() < out[j].Language() })
	return out
}

func Get(name string) (Adapter, bool) {
	a, ok := registry[name]
	return a, ok
}
