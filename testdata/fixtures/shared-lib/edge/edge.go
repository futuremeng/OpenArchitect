package edge

import "example.com/shared/core"

type Client struct{ Target core.Server }

func Version() string { return "1" }
