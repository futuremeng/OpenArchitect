package store

import "testing"

func TestCart(t *testing.T) {
	c := &Cart{}
	c.Add("x")
	if c.Total() != 1 {
		t.Fatal("bad")
	}
}
