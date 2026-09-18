# KALUA — Agentic Development Testbed

Standalone OpenCode workspace for evaluating and testing agentic development
of KALUA `.lua` apps. **pwd = this directory.** All tools are local.

- **Binary**: `./KALUA` (self-contained copy; no repo-root traversal needed).
- **Skills**: `.opencode/skills/kalua-api` and `.opencode/skills/kalua-authoring`
  are full copies (identical to canonical at `_opencode/skills/` — keep in sync).
- **Authoring guide**: `./docs/agentic/development.md` (full copy of the canonical
  guide at repo root `docs/agentic/development.md`).
- **Config**: `./KALUA.INI` drives CLI defaults.

## Golden Loop (short form)

```bash
./KALUA check app.lua                                      # static validation
./KALUA run app.lua --test --json                          # headless run + machine-readable result
./KALUA serve app.lua --test --json                        # headless API smoke
./KALUA serve app.lua --test --json --http "GET /healthz"  # headless API smoke with HTTP request
./KALUA new app --template serve-all                       # scaffold
```
