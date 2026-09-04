module kalua

go 1.26.3

require (
	github.com/coder/websocket v1.8.15
	github.com/yuin/gopher-lua v1.1.2
	go.lsp.dev/jsonrpc2 v1.0.1
	go.lsp.dev/protocol v1.0.1
	go.lsp.dev/uri v1.0.1
	golang.org/x/crypto v0.55.0
	gopkg.in/yaml.v3 v3.0.1
	modernc.org/sqlite v1.57.0
)

replace github.com/yuin/gopher-lua => ./third_party/gopher-lua

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/go-json-experiment/json v0.0.0-20260623181947-01eb4420fa68 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	golang.org/x/sys v0.47.0 // indirect
	modernc.org/libc v1.74.4 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)
