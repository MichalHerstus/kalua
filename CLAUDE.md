# KALUA Agentic Development — Claude Code Entrypoint

This file tells Claude Code how to work with KALUA apps. The canonical
authoring guide lives at `docs/agentic/development.md` — this file just
points to it and ensures the right commands are available.

## Quick Reference

- **Check syntax**: `./KALUA check app.lua`
- **Headless run-mode test**: `./KALUA run app.lua --test --json`
- **Headless serve-mode test**: `./KALUA serve app.lua --test --json`
- **Scaffold new app**: `./KALUA new app --template serve-all` (or run-form, run-crud, serve-http, serve-ws, serve-tcp)
- **AI generate**: `./KALUA ai generate "a form with name field" -o app.lua --mode run`
- **AI fix**: `./KALUA ai fix app.lua --mode serve`
- **LSP server**: `./KALUA lsp` (stdio)
- **MCP server**: `./KALUA mcp` (stdio, protocol 2025-06-18)
- **Form builder**: `./KALUA builder app.lua`

## Key Rules (from `docs/agentic/development.md`)

1. **Always validate with `check` + headless test** — never guess from specs
2. **Prefer `k.*` / `K.*` / expression functions over generic Lua** when equivalent exists
3. **Run-mode apps**: `function main()` + `k.form.new/show` + handlers
4. **Serve-mode apps**: `handle_http(req)` / `handle_ws(msg)` / `handle_tcp(msg)` — no UI bindings
5. **Exit codes**: 0=OK, 1=Error, 2=Usage, 3=IO
6. **Machine-readable output**: append `--json` to `check`, `run --test`, `serve --test`, `new`

## API Reference

- Full reference: `_opencode/skills/kalua-api/api.md`
- Quick card: `docs/agentic/quickref.md` (top ~40 bindings + idioms)

## Config

`KALUA.INI` in workspace (sections `[RUN]`, `[SERVE]`, `[BUILDER]`, `[CHECK]`, `[AI]`; precedence: CLI > INI > env > defaults).