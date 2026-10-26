module github.com/futuremeng/OpenArchitect

go 1.27.2

require (
	github.com/egoist/mygo v0.5.5
	github.com/oneo-me/onevra/source v0.2026.12
	golang.org/x/tools v0.52.0
)

require (
	github.com/ebitengine/purego v0.11.1 // indirect
	github.com/go-text/typesetting v0.3.5 // indirect
	golang.org/x/image v0.47.0 // indirect
	golang.org/x/mod v0.42.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/text v0.43.0 // indirect
)

tool github.com/egoist/mygo/cmd/mygo

replace github.com/oneo-me/onevra/source => github.com/futuremeng/onevra/source v0.2026.13-0.20261026124152-70c837f0421e
