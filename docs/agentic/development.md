# KALUA Agentic Development Guide

Canonical guide for developing and validating KALUA `.lua` apps with AI
agents (OpenCode, Claude Code, Cursor, GitHub Copilot, or any CLI-driven
agent). **This is the single source of truth**; all per-platform entrypoints
(see §7) read this document.

## 1. Golden Loop

Every change to a `.lua` app is validated headlessly with the KALUA binary —
shell out to `KALUA`, never guess from a spec doc.

```bash
KALUA check app.lua                  # 1. static validation (syntax, unknown k.*, main)
KALUA run app.lua --test --json      # 2. headless run mode test (no HTTP server)
KALUA serve app.lua --test --json    # 3. headless serve-mode smoke (HTTP API worker pool)
KALUA new app --template serve-all   # 4. scaffold a new app (appends .lua, refuses overwrite)
```

Use `../KALUA` (repo root) or `./KALUA` (testbed workspace, self-contained
binary copy) depending on where the agent runs — never a path that mixes
both.

## 2. Exit Codes

| Code | Constant   | Meaning                     |
|------|------------|-----------------------------|
| 0    | `ExitOK`   | Success                     |
| 1    | `ExitError`| Script/runtime error        |
| 2    | `ExitUsage`| CLI usage error             |
| 3    | `ExitIOError`| File I/O error (not found, permission) |

## 3. Machine-Readable Output (`--json`)

Append `--json` to `check`, `run --test`, `serve --test`, and `new` for
structured diagnostics: line/col-precise issues for `check`, and smoke-result
fields for the two test modes. Use it to decide pass/fail in a script without
parsing prose.

## 4. Code Style Rules

- **Prefer `k.*` / `K.*` / expression functions over generic Lua** whenever
  an equivalent exists: `left`, `upper`, `round`, `sys_date`, `k.http_request`,
  `k.csv_parse`, `k.file_*`, … Plain Lua is a fallback only.
- Every app must define `function main()` (`check` enforces it).
- Serve-mode apps must not use UI bindings (`k.form.*`, `k.ctrl.*`,
  `k.msgbox`, `k.status_*`).

## 5. KALUA.INI

Agents may rely on `KALUA.INI` in the workspace for CLI defaults
(`[RUN]`, `[SERVE]`, `[BUILDER]`, `[CHECK]`, `[AI]` sections; precedence:
CLI flags > INI > env > defaults). Keep keys matching long flag names.

## 6. API Reference

The generated, drift-gated API reference is `_opencode/skills/kalua-api/api.md`
(repo root) or `.opencode/skills/kalua-api/api.md` (testbed full copy). Author
behavior rules live in `_opencode/skills/kalua-authoring/SKILL.md`.

## 7. Per-Platform Wiring Matrix

| Platform        | Entrypoint                                                  | Reads this guide |
|-----------------|-------------------------------------------------------------|------------------|
| OpenCode        | `opencode.json` (LSP `lsp.lua.command = ["KALUA","lsp"]`) + `AGENTS.md` | `@`-import / direct read |
| Claude Code     | `CLAUDE.md` impossible -> use `CLAUDE.local.md` or the repo's global rules | `@`-import |
| Cursor          | `.cursor/rules/kalua.mdc` (globs `**/*.lua`)                | relative path     |
| GitHub Copilot  | `.github/copilot-instructions.md`                           | relative path     |
| VSCode Ext      | `extensions/vscode-kalua` (LSP server over stdio)           | bundled docs      |

All entrypoints are thin pointers to this guide; this repo root copy is the
authority.