package main

import "example.com/simple/internal/store"

func main() {
	c := &store.Cart{}
	c.Add("a")
	_ = c.Total()
}
