package edge

import "example.com/shared/core"

// Client names core.Server in its field and calls its method.
type Client struct{ server core.Server }

// Version is a package function, not a component.
func Version() string { return "1" }

// Ping reaches across the package boundary twice: a type use and a call.
func (c *Client) Ping() string { return c.server.Addr() }
