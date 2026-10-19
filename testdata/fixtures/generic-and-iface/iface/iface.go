package iface

type Reader interface {
	Read() string
}

// The conventional assertion is the only statically reliable "implements"
// signal, which is why implements edges were dropped (spec §3.2).
var _ Reader = String("")

type String string

func (s String) Read() string { return string(s) }

// An alias is not a type declared here: spec §6.3's named-type check
// (`Type() == Obj()`) keeps it out of the Component set, so `Sink` must
// never become its own Component (§6.3 排除集).
type Sink = Reader
