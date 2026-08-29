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
./KALUA run <app.lua> [--port 0] [--no-browser] [--db NAME=DSN] [--arg K=V]
./KALUA check <app.lua>     # static validation (syntax, unknown k.*, main)
./KALUA new <name>          # scaffold minimal app.lua
./KALUA version
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
- **Run modes**: `run` = interactive web app (UI bindings live), `serve` = headless API (planned)
- **Sandbox**: gopher-lua with `SkipOpenLibs`, custom `k.*` API only

## Package Layout

```
cmd/KALUA/           # CLI entry point (main.go → cli.Run)
internal/cli/        # Command parsing, flags, usage
internal/host/       # App lifecycle, RunConfig, exit codes, logging
internal/vm/         # LState setup, sandbox whitelist, script loader, App runner
internal/bindings/   # k.* API registration (flow, forms, controls, db, files)
internal/coerce/     # K.eq/ne/add value semantics
internal/checker/    # Static analysis (syntax, unknown k.*, main presence)
internal/session/    # Per-tab actor: inbox/outbox, form stack, timers
internal/web/        # HTTP server, WebSocket bridge, embedded assets
internal/common/     # Shared types (OutboxMsg, SessionInterface) to avoid import cycles
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
- Dependencies: `github.com/yuin/gopher-lua v1.1.2`, `github.com/coder/websocket v1.8+`
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