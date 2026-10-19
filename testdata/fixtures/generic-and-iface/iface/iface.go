package iface

type Reader interface {
	Read() string
}

// The conventional assertion is the only statically reliable "implements"
// signal, which is why implements edges were dropped (spec §3.2).
var _ Reader = String("")

type String string

func (s String) Read() string { return string(s) }
