# KALUA Agentic Development — GitHub Copilot Entrypoint

This file tells GitHub Copilot how to work with KALUA apps. The canonical
authoring guide lives at `docs/agentic/development.md`.

## Commands

- **Check syntax**: `./KALUA check app.lua`
- **Headless run-mode test**: `./KALUA run app.lua --test --json`
- **Headless serve-mode test**: `./KALUA serve app.lua --test --json`
- **Scaffold new app**: `./KALUA new app --template serve-all`
- **AI generate**: `./KALUA ai generate "a form with name field" -o app.lua`
- **AI fix**: `./KALUA ai fix app.lua`
- **LSP server**: `./KALUA lsp`
- **MCP server**: `./KALUA mcp` (stdio, protocol 2025-06-18)
- **Form builder**: `./KALUA builder app.lua`

## Key Rules

1. Always validate with `check` + headless test (`--test --json`)
2. Prefer `k.*` / `K.*` / expression functions over generic Lua
3. Run-mode: `function main()` + `k.form.*`
4. Serve-mode: `handle_http/handle_ws/handle_tcp` — no UI bindings
5. Exit codes: 0=OK, 1=Error, 2=Usage, 3=IO

## References

- Full API: `_opencode/skills/kalua-api/api.md`
- Quick ref: `docs/agentic/quickref.md`
- Config: `KALUA.INI` (`[RUN]`, `[SERVE]`, `[BUILDER]`, `[CHECK]`, `[AI]`)