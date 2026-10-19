package main

import "example.com/simple/internal/store"

func main() {
	_ = (&store.Cart{}).Total()
}
