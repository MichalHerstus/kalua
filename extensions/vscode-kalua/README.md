# KALUA for Visual Studio Code

Language support for the [KALUA](https://github.com/MichalHerstus/kalua) runtime —
a Go runtime that embeds a sandboxed gopher-lua VM to run Kalipso-style `.lua`
apps as web apps.

Implemented through the `KALUA lsp` subcommand (language server over stdio).

## Features

- **Completion** — after `k.`/`K.` (top-level functions, `form`/`ctrl`/`table`
  namespaces and members, `K.*` convenience helpers).
- **Hover** — signature and documentation for every `k.*` function.
- **Go to definition** — jumps to the generated API reference for a `k.*` name.
- **Inline diagnostics** — syntax errors and unknown `k.*` references,
  reported live as you type.
- **Commands**
  - `KALUA: Check file` — run static validation on the active Lua file.
  - `KALUA: Run app` — start the interactive web app for the active file.
  - `KALUA: New app...` — scaffold a minimal `app.lua`.

## Requirements

- [KALUA](https://github.com/MichalHerstus/kalua) binary on `PATH`, or built next
  to the extension repo (`go build -o KALUA ./cmd/KALUA`).
- VSCode 1.80+.

## Configuration

| Setting            | Description                                                        |
| ------------------ | ------------------------------------------------------------------ |
| `kalua.binaryPath` | Path to the KALUA binary. Empty = auto-detect, then `KALUA` on PATH. |

## Development

```bash
cd extensions/vscode-kalua
npm install
npm run compile      # tsc
npm run package      # build + .vsix via vsce
```

Launch the extension host with F5 (uses `KALUA` at the repo root by default).