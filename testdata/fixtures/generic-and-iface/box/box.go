// Package box holds a generic component: one node, parameters dropped (§6.2).
package box

type Box[T any] struct{ Value T }

func (b *Box[T]) Get() T { return b.Value }

type Pair[T, U any] struct {
	First  T
	Second U
}

func (p *Pair[T, U]) Swap() { p.First, p.Second = p.Second, p.First }
