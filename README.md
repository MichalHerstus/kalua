# KALUA

A Go runtime that embeds a sandboxed gopher-lua VM to run Kalipso-style `.lua` apps as web apps. Single generic binary interprets any Lua script. Kalua (aka "KAlipso in LUA") include wide set of functions inspired by Sysdev Mobile **Kalipso lowcode platform**, like k.form, k.ctrl, etc.

## Quick Start

```bash
# Build
go build -o KALUA ./cmd/KALUA

# Scaffold a new app
./KALUA new myapp

# Run interactive web app (opens browser)
./KALUA run myapp.lua

# Run headless API server
./KALUA serve myapp.lua --port 8080 --workers 4 --mode http,ws

# Static validation
./KALUA check myapp.lua

# Language server (for editors)
./KALUA lsp

# Build WASM bundle for browser (runs 100% client-side, no server)
./KALUA wasm-bundle myapp.lua -o dist/
# Open dist/index.html in browser
```

## Architecture

```
myapp.lua ──► KALUA run myapp.lua
              │
        ┌─────┴─────────────────────┐
        │ Go host (net/http + WebSocket) │
        │   per tab → Session actor (1 LState) │
        │   inbox = typed events (WS, timers)   │
        │   outbox = UI commands → templ → WS  │
        └──────────────────────────────────────┘
```

- **Entry point**: Lua script must define `function main()`
- **Run modes**: `run` = interactive web app (UI bindings live), `serve` = headless API (HTTP/WS/TCP with worker pool), `wasm` = 100% client-side in browser (no server)
- **Sandbox**: gopher-lua with `SkipOpenLibs`, custom `k.*` API only

## WASM Mode (Run Mode Client-Side)

Ship `index.html` + `KALUA.wasm` (bundled via `KALUA wasm-bundle <app.lua>`) that runs any `run`-mode KALUA app 100% client-side. The existing browser client (`app.js`, `shell.html`) already speaks the session's outbox/inbox JSON protocol — this phase replaces one transport (WS/HTTP in `internal/web/server.go`) with an in-page bridge.

- **In scope**: run-mode semantics per wasm instance (gopher-lua VM, session actor, forms/controls, timers, coroutine suspension); pure bindings unchanged; browser host-IO profile; wa-sqlite; optional localhost relay for non-browser protocols.
- **Non-goals**: serve mode worker pool, `k.shared.*`, multi-session — native only. Tab isolation = one separate wasm instance per tab. No HTTP server in the page.

**Implemented bindings (synchronous stubs for M2, async in M3+):**
- Files: `k.file_*` via IndexedDB, `k.param_*` via localStorage
- Network: `k.http_request` via fetch, `k.net_ok`, `k.ping`, `k.locale`, `k.screen_size`
- Clipboard: `k.clipboard_set/get` via navigator.clipboard
- File picker: `k.pick_file` via File System Access API
- SQLite: `k.connect_sqlite` + `k.db_*` via wa-sqlite
- Relay client: `k.relay.connect/call/close` for MySQL/PG/MSSQL/FTP/SMTP/POP3/TCP

## Features

### Run Mode (Interactive Web Apps)
- Form system: `k.form.new`, `k.form.show`, `k.form.close`, `k.form.on`
- Controls: label, textbox, button, combo, list, table, checkbox, radio
- Control props: `k.ctrl.set_value`, `k.ctrl.get_value`, `k.ctrl.set_property`, `k.ctrl.get_property`
- Modal dialogs: `k.msgbox`, clipboard, bell, screen size
- Async HTTP client: `k.http_request`
- XML parsing: `k.xml_parse` + `xml_*` helpers
- Session-based coroutine suspension for async operations

### Serve Mode (Headless API)
- Worker pool with shared state via `k.shared.*` (thread-safe)
- HTTP server: `handle_http(req)` callback
- WebSocket server: `handle_ws(conn)` callback + `k.ws.broadcast/send/close`
- TCP server: `handle_tcp(conn)` callback + `k.tcp.send/close`
- Lifecycle hooks: `init(config)` at startup, `shutdown()` on signal
- SIGHUP hot reload with zero-downtime worker swap

### Expression Functions (Global)
String: `left`, `right`, `middle`, `length`, `replace`, `trim`, `upper`, `lower`, `find`, `base64_encode/decode`, `urlencode/urldecode`, `jsonencode/jsondecode`, `guid`, `mltext`…
Numeric: `abs`, `round`, `floor`, `ceiling`, `power`, `sqrt`, `sin`, `cos`, `tan`, `random`, `bitwise_and/or/xor`…
Conditional: `lookup`, `yesno`, `iif` (Kalipso truthiness)
Date/Time: `sys_date`, `sys_time`, `add_days`, `date_diff`, `datetime_add/sub`, `week_day`, `julian`, `utc_to_local`…
Conversion: `tostr`, `tonum`, `todate`, `strtodate`, `boolstr`

### Data Formats & DB (Tier 2)
- CSV, INI, YAML: `k.csv_*`, `k.ini_*`, `k.yaml_*` parse/string/load/save
- Result-set conversions: `k.json_to_rows`, `k.rows_to_json`, `k.csv_to_rows`, `k.xml_to_rows`
- SQLite: `k.connect_sqlite`, `k.db_kill_table`, `k.db_proc`
- Crypto: `k.crypt_symmetric` (AES-CBC), `k.crypt_asymmetric`, `k.sign`, `k.verify` (RSA PKCS#1 v1.5)
- Files: `k.zip_add/extract/list`
- Flow: `k.timer_start/stop`, `k.status_show/close`, `k.param_get/set`, `k.net_ok`, `k.locale`, `k.ping`
- Comm: `k.socket_*`, FTP, SMTP, POP3, SOAP

### Debugging (Tier 1)
- `--verbose` (`-v`): trace all `k.*` API calls with args/returns
- `k.debug.stack()`, `k.debug.locals([level])`, `k.debug.trace(msg)`
- `--repl-on-error`: interactive Lua REPL at crash site (with `--test`)

### Language Server (LSP)
- `KALUA lsp` over stdio (UTF-8 position encoding)
- Completion, hover, go-to-definition for `k.*`/`K.*`/globals
- Diagnostics: syntax errors + unknown `k.*` references
- VSCode extension in `extensions/vscode-kalua`

## Project Layout

```
cmd/KALUA/           # CLI entry point
internal/cli/        # Command parsing, flags (run, serve, check, test, lsp, builder, ai, scenario, wasm-bundle, relay, version)
internal/host/       # App lifecycle, RunConfig, exit codes, logging
internal/vm/         # LState setup, sandbox, script loader
internal/bindings/   # k.* API registration (flow, forms, controls, db, files, server, crypto, etc.)
internal/coerce/     # K.eq/ne/add value semantics
internal/checker/    # Static analysis (syntax, unknown k.*, main presence)
internal/lsp/        # LSP server: completion, hover, diagnostics, definition
internal/session/    # Per-tab actor: inbox/outbox, form stack, timers
internal/web/        # HTTP server, WebSocket bridge, embedded assets
internal/server/     # Serve mode: worker pool, HTTP/WS/TCP servers, shared state
internal/common/     # Shared types to avoid import cycles
internal/wasm/       # WASM entry point, bridge, browser bindings
third_party/gopher-lua/   # Vendored fork with debug.hook() support
extensions/vscode-kalua/  # VSCode extension
```

## Dependencies

- Go 1.26.3
- `github.com/coder/websocket v1.8+`
- `go.lsp.dev/protocol v1.0.1`, `go.lsp.dev/jsonrpc2 v1.0.1`
- `gopkg.in/yaml.v3`
- `github.com/ncruces/go-sqlite3` (wa-sqlite for WASM)
- gopher-lua: vendored fork at `third_party/gopher-lua` (based on v1.1.2 with `debug.hook()`)

## Testing

```bash
go test ./...                    # All tests
go test ./internal/host          # Single package
go test -v ./internal/bindings   # Verbose
```

Exit codes: `0=OK`, `1=Error`, `2=Usage`, `3=IOError`

## License

MIT
