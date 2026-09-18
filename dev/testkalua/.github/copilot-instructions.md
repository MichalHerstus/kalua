# KALUA agentic development

Build and iterate on KALUA `.lua` apps (run mode and serve mode) by editing
`.lua` files and validating headlessly with the KALUA CLI.

Read `./docs/agentic/development.md` (guide) and, for API details,
`./.opencode/skills/kalua-api/api.md`.

## Quick start

1. Static-check: `./KALUA check app.lua`
2. Headless run: `./KALUA run app.lua --test`
3. Headless API smoke: `./KALUA serve app.lua --test --json`
4. Machine-readable output: append `--json`

## Rules

- Prefer `k.*` / `K.*` / expression functions over generic Lua whenever an equivalent exists.
- Always verify generated Lua with `./KALUA check` (and `run --test` / `serve --test`) before presenting it.
- Never generate UI bindings (`k.form.*`, `k.ctrl.*`, `k.msgbox`, `k.status_*`) for serve-mode apps.
