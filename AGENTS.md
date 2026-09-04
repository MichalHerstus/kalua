# KALUA — Agent Instructions

## Project Overview
**KALUA** is a Go runtime that embeds a sandboxed gopher-lua VM to run Kalipso-style `.lua` apps as web apps. Single generic binary interprets any Lua script.

## Key Commands

```bash
# Build
go build -o KALUA ./cmd/KALUA

# Run tests
go test ./...

# Run single package tests
go test ./internal/host

# CLI usage
./KALUA run <app.lua> [--port 0] [--no-browser] [--db NAME=DSN] [--arg K=V] [-v|--verbose] [--repl-on-error] [--debug] [--test]
./KALUA serve <app.lua> [--port 8080] [--host 127.0.0.1] [--workers 4] [--mode http|ws|tcp] [--db NAME=DSN] [--arg K=V] [-v|--verbose] [--debug] [--debug-worker]
./KALUA check <app.lua>     # static validation (syntax, unknown k.*, main)
./KALUA new <name>          # scaffold minimal app.lua
./KALUA lsp                 # language server over stdio (LSP, Content-Length frames)
./KALUA version

# VSCode extension (extensions/vscode-kalua)
# npm install && npm run compile && npm run package   -> .vsix
# F5 launch uses the KALUA binary at the repo root.
```

## Architecture (from kalua_spec.md)

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
- **Run modes**: `run` = interactive web app (UI bindings live), `serve` = headless API (HTTP/WS/TCP with worker pool)
- **Sandbox**: gopher-lua with `SkipOpenLibs`, custom `k.*` API only

## Package Layout

```
cmd/KALUA/           # CLI entry point (main.go → cli.Run)
internal/cli/        # Command parsing, flags, usage
internal/host/       # App lifecycle, RunConfig, exit codes, logging
internal/vm/         # LState setup, sandbox whitelist, script loader, App runner
internal/bindings/   # k.* API registration (flow, forms, controls, db, files, server)
internal/coerce/     # K.eq/ne/add value semantics
internal/checker/    # Static analysis (syntax, unknown k.*, main presence)
internal/lsp/        # LSP server (stdio): completion, hover, diagnostics, definition
internal/session/    # Per-tab actor: inbox/outbox, form stack, timers
internal/web/        # HTTP server, WebSocket bridge, embedded assets
internal/server/     # Serve mode: worker pool, HTTP/WS/TCP servers, shared state
internal/common/     # Shared types (OutboxMsg, SessionInterface) to avoid import cycles
extensions/vscode-kalua/  # VSCode extension (TS client, Lua grammar, language-config)
```

## Testing Conventions

- Standard `go test ./...` runs all tests
- Test files use `*_test.go` naming
- Temp directories via `t.TempDir()` for isolation
- Logger with `Verbose: false` for quiet tests
- Exit codes: `ExitOK=0`, `ExitError=1`, `ExitUsage=2`, `ExitIOError=3`
- Use `--test` flag with `run` command for headless test mode

## Development Notes

- Go 1.26.3 (matches go.mod)
- Dependencies: `github.com/coder/websocket v1.8+`, `go.lsp.dev/protocol v1.0.1`, `go.lsp.dev/jsonrpc2 v1.0.1`
- **gopher-lua**: vendored fork at `third_party/gopher-lua` (based on v1.1.2) with `debug.hook()` support; referenced via `replace github.com/yuin/gopher-lua => ./third_party/gopher-lua` in go.mod
- LSP: `internal/lsp` serves `KALUA lsp` over stdio; position encoding is UTF-8 (character = byte offset in line); server is the source of truth for completion/hover/definition via `internal/bindings` api_doc; diagnostics use `internal/checker`. LSP union types are sealed interfaces (`Boolean`, `TextDocumentSync`, `InlayHintTooltip`); `TextDocumentContentChangeEvent` is a union of WholeDocument/Partial. The connection is wired manually (union-aware codec via `protocol.Marshal/Unmarshal`) so messages dispatch serially in arrival order — do NOT reintroduce `AsyncHandler`/`CancelHandler` (breaks LSP ordering; `CancelHandler`'s context propagation races the pooled request).
- No linting/formatting config found — uses `go fmt` defaults
- No CI/CD config found
- Lua scripts are plain `.lua` files with `k.form.new()` for form declarations
- API naming: `snake_case` matching Kalipso (`k.form.new`, `k.ctrl.textbox`)
- Templates: Go `html/template` for shell, controls rendered via Go code
- Assets: embedded via `//go:embed` (CSS, JS)

## Exit Codes (host.ExitCode)

| Code | Constant | Meaning |
|------|----------|---------|
| 0 | ExitOK | Success |
| 1 | ExitError | Script/runtime error |
| 2 | ExitUsage | CLI usage error |
| 3 | ExitIOError | File I/O error (not found, permission) |

## Implemented Features (Phase 2 - Web Core)

- HTTP server with WebSocket support (`github.com/coder/websocket`)
- Session actor per browser tab with inbox/outbox pattern
- Form system: `k.form.new`, `k.form.show`, `k.form.close`, `k.form.on`
- Controls: label, textbox, button, combo, list, table, checkbox, radio
- Control properties: `k.ctrl.set_value`, `k.ctrl.get_value`, `k.ctrl.set_property`, `k.ctrl.get_property`
- Static checker validates `k.*` references including nested (`k.form.new`, `k.ctrl.textbox`)
- Embedded assets: `kalua.css`, `app.js` (vanilla JS with event delegation)
- Test mode: `--test` flag runs headless without HTTP server

## Implemented Features (Phase 5 - LSP & Editor)

- `KALUA lsp` subcommand: LSP over stdio (`internal/lsp`), UTF-8 position encoding
- Full document sync + debounced diagnostics from `internal/checker` (syntax errors, unknown `k.*`)
- Completion for `k.*` (namespaces form/ctrl/table, member functions), `K.*` helpers, globals
- Hover markdown from `internal/bindings` api_doc; go-to-definition via generated API reference stub
- `extensions/vscode-kalua`: language id `kalua` registered for `*.lua`, vendored Lua TextMate grammar, TS client via `vscode-languageclient` (`TransportKind.stdio`), commands `KALUA: Check file / Run app / New app`, setting `kalua.binaryPath` (default auto-detect repo binary then PATH); package with `vsce`

## Implemented Features (Phase 6 - Serve Mode)

- `KALUA serve` subcommand: headless API server with worker pool
- Worker pool: multiple Lua VMs sharing state via `k.shared.*`
- HTTP server with `handle_http(req)` callback returning (status, headers, body)
- WebSocket server with `handle_ws(conn)` callback; `k.ws.broadcast/send/close`
- TCP server with `handle_tcp(conn)` callback; `k.tcp.send/close`
- Shared state: `k.shared.set/get/del/keys/incr` (thread-safe across workers)
- Mode flag: `--mode http,ws,tcp` (comma-separated)
- UI bindings (`k.form.*`, `k.ctrl.*`, `k.msgbox`, `k.status_*`) raise runtime error in serve mode
- `--workers N` flag for worker count; `--host`, `--port` for binding
- ARGS table seeded from `--arg` flags

## Implemented Features (Phase 7 - Complete T1 Run Mode)

- `k.msgbox(text[, kind])` — modal message box with user choice returned to script
- `k.clipboard_set(text)` / `k.clipboard_get()` — browser clipboard access
- `k.bell()` — play system beep via WebAudio
- `k.screen_size()` — returns viewport `{width, height}` from client_info
- `k.http_request(opts)` — async HTTP client: `{method,url,headers,body,timeout}` → `{status,headers,body}`
- `k.xml_parse(text)` + `xml_*` — XML parsing: `xml_root`, `xml_child`, `xml_child_list`, `xml_attr`, `xml_content`, `xml_attrs`, `xml_name`
- Session-based coroutine suspension for async operations (msgbox, http_request)
- Web server handles `msgbox_choice`, `client_info`, `clipboard_get` messages

## Implemented Features (Phase 8 - Expression-Function Library)

- §5.9 expression functions as flat globals (not under `k.*`) in `internal/bindings/funcs.go`, installed by `Setup` (run mode) and `SetupServe` (serve mode):
  - String: `left right middle length replace trim upper lower find string_count complete ascii charact base64_encode/decode urlencode/urldecode encode/decode full_encode jsonencode/jsondecode xmlencode/xmldecode guid extract_string set_string file_extract_part mltext`
  - Numeric: `abs round floor ceiling power nth_root sqrt exp log log10 sin cos tan asin acos atan deg2rad rad2deg bitwise_and/or/xor random int_part dec_part mask_number val sum extractstringd`
  - Conditional: `lookup(key, k1, v1, ...)`, `yesno(cond, a, b)`, `iif(cond, a, b)` (Kalipso truthiness via `coerce`)
  - Date/Time: `sys_date sys_time day month year hour minute second add_days subtract_days date_diff datetime_add/sub datetime_diff date_to_string time_to_string week_day week_number tick_count julian utc_to_local local_to_utc` (dates as `YYYY-MM-DD[ HH:MM[:SS]]`)
  - Conversion: `tostr tonum todate strtodate boolstr`
- Expression funcs documented in `api_doc.go` (`ExprFuncs`/`ExprInfo`); LSP completion (bare-identifier path), hover, and go-to-definition support them
- Serve mode (`SetupServe`) now also installs the `K.*` helpers (§2.3) and expression functions, matching run mode's coerce/expression surface
- Coercion-based semantics (half-away-from-zero round, Sunday=1 weekday) pinned by `internal/bindings/funcs_test.go` and `internal/host/exprfuncs_test.go`

## Implemented Features (Phase 9 - Tier 2 wave)

- Data formats (§5.10): `k.csv_parse/string/load/save`, `k.ini_parse/string/load/save` + `k.ini_read/write`, `k.yaml_parse/string/load/save`, `k.xml_load/xml_save` (`internal/bindings/formats.go`; YAML via `gopkg.in/yaml.v3`)
- Result-set conversions: `k.json_to_rows/rows_to_json`, `k.csv_to_rows/rows_to_csv`, `k.xml_to_rows/rows_to_xml` (`internal/bindings/rows.go`)
- Database tier-2: `k.connect_sqlite/disconnect_sqlite`, `k.db_kill_table`, `k.db_proc` (`internal/bindings/db.go`)
- Crypto tier-2: `k.crypt_symmetric` (AES-CBC), `k.crypt_asymmetric`/`k.sign`/`k.verify` (RSA PKCS#1 v1.5) (`internal/bindings/crypto.go`)
- Files tier-2: `k.zip_add/extract/list` (`internal/bindings/files.go`)
- Flow tier-2: `k.timer_start/stop` (fires Lua global named by the timer id), `k.status_show/close`, `k.param_get/set` (persisted `.kalua.params.json`), `k.net_ok`, `k.locale`, `k.ping` (TCP latency probe) (`internal/bindings/flow.go`, `net.go`)
- Comm tier-2: `k.socket_open/write/read/read_line/close` (`internal/bindings/comm.go`)
- Comm tier-2 (phase 9 remainder): FTP `k.ftp_connect/set_cwd/get_file/put_file/file_exists/create_dir/delete/rename/list/disconnect` (minimal client, `internal/bindings/ftp.go`); SMTP `k.smtp_connect/send/disconnect` (`smtp.go`, `net/smtp`); POP3 `k.pop3_connect/stat/list/retr/dele/noop/quit` (`pop3.go`); SOAP `k.webservice_run(profile, params)` (`soap.go`)
- Fixed a latent gopher-lua v1.1.2 bug: `cancel()` on a *finished* coroutine nil-panics; all session coroutine resumes now only cancel suspended/errored threads (`internal/session/session.go`); also wired `app.SetSession` so `sendOutbox` reaches the session outbox
- New dependency: `gopkg.in/yaml.v3`

## Implemented Features (Phase 9 remainder — serve/run correctness)

- Serve `handle_http(req)` now supports the §2.5 response forms: nil→200 empty, plain string→200 text/plain, `{json=...}`→200 application/json, `{status=n}`→n empty, full `{status,headers,body}`; `req` gains `query` (single→string, repeated→list), `query_raw`, `remote_addr`, `tls` (`internal/server/worker.go`; new exported `bindings.JSONStringifyLua` in `json.go`)
- Serve WS/TCP inbound dispatch: `handle_ws(msg)`/`handle_tcp(msg)` called in a fresh coroutine per event with `{type="open|text|binary|close", data, client_id}`; string return value echoed back to the connection; per-worker mutex serializes all handler entry so an `LState` is never touched concurrently (WS outbound pump + TCP pump drain the send channels, `internal/server/server.go`)
- Serve lifecycle: optional `init(config)` runs once on the first worker at startup (error aborts), optional `shutdown()` runs once on a worker on `SIGTERM`/`SIGINT` (wired via `signal.NotifyContext` in `cmd`/`cli.go`); gopher-lua `Resume` values read from its 3rd return (not the main stack)
- Serve SIGHUP hot reload: `Server.Reload()` recompiles the script, builds a fresh worker pool, and swaps it atomically under `workerMu`; workers are leased per HTTP request / WS/TCP connection (`Worker.refs`/`retired`/`closeOnce`), so superseded workers finish in-flight work and open connections before their Lua state is released — a reload error keeps the old pool serving. `Internal/cli` `serveCmd` consumes SIGHUP (INT/TERM still stop via `signal.NotifyContext`)
- Serve `k.shared.*` JSON round-trip: `set` stores JSON, `get` decodes with legacy raw-string fallback (`registerShared(e, store)` in `serve.go`); `k.print` sinks through the host logger in run mode too (`internal/bindings/flow.go`)
- Tests: `internal/server/worker_test.go` (HTTP response forms, WS/TCP cancel-panic + error logging), `internal/server/server_e2e_test.go` (real HTTP/WS/TCP sockets + init/shutdown lifecycle + SIGHUP reload swap / error-keeps-old-pool / open-connection-survives-reload)

## Implemented Features (Phase 11 - Debugging Tier 1)

- **Vendored patched gopher-lua** (`third_party/gopher-lua`): fork of v1.1.2 with `debug.hook()` support (port from edolphin-ydf/gopher-lua). Added `hook.go` with LHook/CHook/RHook/CTHook, hook call-sites in `vm.go` (line, count, call, return), `SetHook` + `debug.sethook` in `debuglib.go`. Enabled `debug` library in sandbox. Uses `replace` directive in go.mod.
- `--verbose` (short `-v`): enhanced tracing of all `k.*` API calls — logs function entry with args and exit with return values (reads from top-of-stack). Works in both `run` and `serve` modes.
- `k.debug.*` API (Tier 1 introspection):
  - `k.debug.stack()` — returns table of call frames with `{level, name, source, line, what, locals}`
  - `k.debug.locals([level])` — returns table of local name→value at given frame (default 1)
  - `k.debug.trace(msg)` — script-side trace anchor; logs via host logger when verbose
- Post-mortem dump: on runtime error with `--verbose`, prints full stack trace with locals using `xpcall` + `debug.traceback`
- `--repl-on-error`: headless interactive Lua REPL at crash site (works with `--test` mode). Wraps `main()` in `xpcall(debug.traceback)`, captures full traceback, drops into REPL with access to `k.*`, `K.*`, `debug.*`, and script globals. Supports expressions (`1+1` or `= 1+1`), statements, and REPL commands (`exit`, `quit`, Ctrl+D).
- CLI flags: `run --repl-on-error` (functional), `run --debug` / `serve --debug --debug-worker` (stub warnings, Tier 2 EmmyLua debugger not yet implemented)