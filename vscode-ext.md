# VS Code Extension Development — KALUA

## Quick Start

```bash
cd extensions/vscode-kalua
npm install --no-audit --no-fund --cache /tmp/kalua-npm-cache
npm run compile        # tsc → client/out/extension.js
npm run package        # vsce package → kalua.vsix
```

## Run Extension (F5)

1. Open the **repo root** (`/Users/michalherstus/dev/kalua`) in VS Code.
2. Press `F5` → launches "Run Extension" host (uses `.vscode/launch.json`, `preLaunchTask: npm: compile`).
3. In the new window, open any `.lua` file (or create one). The `KALUA` language server starts automatically (spawns `./KALUA lsp` from the repo root).

## Install Packaged Extension

```bash
# From repo root
cd extensions/vscode-kalua && npm run package
# → produces kalua.vsix in the same directory

# Install
code --install-extension extensions/vscode-kalua/kalua.vsix --force
```

## Configuration

| Setting | Default | Description |
|---------|---------|-------------|
| `kalua.binaryPath` | `""` (auto) | Path to `KALUA` binary. Empty = auto-detect: 1) `<extensionPath>/../../KALUA` (repo dev layout), 2) `KALUA` on `PATH`. |

## Commands

| Command | Action |
|---------|--------|
| `KALUA: Check file` | Runs `KALUA check <file>` in integrated terminal |
| `KALUA: Run app` | Runs `KALUA run <file>` in integrated terminal |
| `KALUA: New app` | Runs `KALUA new <name>` in integrated terminal |

## Development Tips

- **Rebuild Go binary after changes**: `go build -o KALUA ./cmd/KALUA` (from repo root). The F5 host picks it up immediately.
- **Watch TS**: `npm run watch` in `extensions/vscode-kalua` (incremental `tsc -w`).
- **Logs**: "Output" panel → "KALUA" shows LSP client logs.
- **Debug LSP**: Add `"trace.server": "verbose"` to `launch.json` args for full JSON-RPC trace.

## Troubleshooting

| Symptom | Fix |
|---------|-----|
| Extension not activating | Ensure file has `.lua` extension (language id `kalua`). |
| "KALUA binary not found" | Set `kalua.binaryPath` in settings, or run `go build -o KALUA ./cmd/KALUA` from repo root. |
| Completion/hover not working | Check LSP log (Output → KALUA). Verify `KALUA lsp` works standalone: `echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}' | ./KALUA lsp` |
| npm EACCES on install | Use `--cache /tmp/kalua-npm-cache` (global cache permission issue). |

## Project Structure

```
extensions/vscode-kalua/
├── package.json          # manifest, commands, config, deps
├── client/
│   ├── src/extension.ts  # LanguageClient setup (stdio, KALUA lsp)
│   └── tsconfig.json
├── syntaxes/lua.tmLanguage.json   # TextMate grammar
├── language-configuration.json    # brackets, comments, pairs
├── .vscodeignore       # excludes node_modules, client/out, *.vsix
├── .gitignore
├── README.md
└── LICENSE
```

## Dependencies

- Runtime: `vscode-languageclient ^9.0.1`
- Dev: `typescript ^5.4.0`, `@types/node ^20.11.0`, `@types/vscode ^1.80.0`, `@vscode/vsce ^3.1.0`

## LSP Capabilities (from `KALUA lsp`)

- `textDocumentSync: 1` (full)
- `completionProvider`: trigger chars `.`, `'`, `"`
- `hoverProvider: true`
- `definitionProvider: true`
- `positionEncoding: utf-8` (character = byte offset)