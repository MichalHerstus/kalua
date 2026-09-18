# AI Builder for Kalua — Plan (NL → `.lua`, run-mode forms)

User decisions locked: **LLM = LM Studio (local) + OpenRouter (external)** · **Surface = OpenUI-based** · **Scope MVP = run-mode forms only**.

## 1. Key insight from research

* Both LM Studio (`http://localhost:1234/v1`) and OpenRouter (`https://openrouter.ai/api/v1`) speak **OpenAI-compatible `/chat/completions`**. One Go HTTP client with `baseURL` switch covers both — no SDK needed.
* **OpenUI** (`thesysdev/openui`, MIT, 8.8k★) is a React generative-UI framework: component library → generated system prompt → LLM streams OpenUI Lang → React renderer. Kalua runtime is **Go server-rendered HTML + Lua, not React**, so adopting the OpenUI *renderer* for app output would duplicate `renderControl`. Correct use is **OpenUI chat/UI components as the builder's prompt surface only**; preview stays on the existing Go renderer (`internal/builder/preview.go`, `kalua.css`).
* Deep `k.*` understanding already exists as machine-readable source of truth: `internal/bindings/api_doc.go` (+ `make gen-api` → `_opencode/skills/kalua-api/api.md`), consumed by LSP/checker. The builder must reuse it, not fork it.
* Golden validation loop already exists (`_opencode/skills/kalua-authoring/SKILL.md`): `check` → `run --test` → `run`. AI output feeds through it, with errors fed back to the LLM for auto-fix.

## 2. Architecture

```
prompt ──► builder UI (OpenUI chat panel in internal/builder/assets)
              │  POST /api/ai/generate {prompt, context}
              ▼
Go: internal/ai/
  provider.go  OpenAI-compat client (baseURL/key/model, SSE streaming passthrough)
  knowledge.go system prompt built from api_doc.go (run-mode subset) + few-shots
  generate.go  NL → extract ```lua → checker.Check → host.Run(--test) → retry w/ error feedback
              ▼
.lua file ──► existing builder preview + KALUA run / check
```

* **Config**: env `KALUA_AI_BASE_URL / KALUA_AI_API_KEY / KALUA_AI_MODEL` (or `[AI]` in `KALUA.INI`), flags `--model --base-url --api-key-env`. Defaults: `http://localhost:1234/v1`, model `local-model`. OpenRouter requires key; local does not (OpenRouter endpoints fall back to `OPENAI_API_KEY`). Keys never leave the Go server — browser talks to `/api/ai/*`, Go proxies to the LLM (avoids CORS + key leak).
* **Knowledge pack (run-mode forms subset)**: `k.form.*`, `k.ctrl.*` (+`k.table.*`, chart/image/grid/layout opts), `k.msgbox`, `k.popup`, `k.print/sleep/yield/quit/error`, `K.*` coercion, flat-global expr funcs relevant to forms (`tostr/tonum/upper/...`), globals `ARGS/CTRL/main`. Hard constraints baked in: `function main()` required; `k.form.new` + `k.form.show`; snake_case; expr funcs are flat globals not `k.*`; no `io/os.execute/require`; suspend semantics of `show/msgbox`. Few-shots pulled from `testdata/apps/` (`hello`, `crud_app`, `layout_demo`, `msgbox_demo`, `chart_demo`). Full `api.md` is too large for local-model context — ship a compacted static pack in Phase 1, keyword-filtered retrieval in Phase 2.
* **OpenUI usage (MVP-minimal)**: embed `@openuidev/browser-bundle` (script-tag, no build step) or `react-ui` chat layout into `internal/builder/assets/index.html` as a prompt/history panel beside the existing canvas. Define Kalua controls as OpenUI component descriptors only to generate prompt instructions via `lang-core`; **do not** render apps with the OpenUI renderer. Fallback if bundle proves heavy: vanilla chat panel with identical `/api/ai/*` contract.

## 3. Implementation phases

| Phase | Work | Files |
|---|---|---|
| **1. AI core + CLI** (~3d) | `internal/ai/` package: OpenAI-compat client (net/http stdlib, non-streaming first), knowledge-pack builder with drift test vs `api_doc.go`, `generate→check→run --test→auto-fix (≤3 retries)` loop; `KALUA ai "build a …" [-o app.lua] [--model/--base-url]`, plus `ai fix <app.lua>` reusing the loop | `internal/ai/{provider,knowledge,generate}_*.go`, `internal/cli/ai.go`, CLI usage |
| **2. Builder chat endpoints** (~2d) | `POST /api/ai/generate`, `/api/ai/fix`, `GET /api/ai/status` (provider reachability + model) on existing `internal/builder` server; prompt panel in builder assets; write-back to open document + re-preview | `internal/builder/server.go`, `assets/{index.html,builder.js,builder.css}` |
| **3. OpenUI panel** (~3d) | Vendor `browser-bundle` (CDN pinned + local fallback), Kalua control descriptors → prompt instructions via `lang-core` equivalent in Go (static port, no npm build), streaming via SSE; preview still via Go `renderForm` | `internal/builder/assets/*`, `internal/ai/openui_prompt.go` |
| **4. Polish** (~2d) | Conversation history + "edit existing" (send current `.lua` as context), error→fix one-click, docs (`USER_GUIDE`, `kalua_spec.md` §, `kalua-authoring` skill update), e2e demo app | docs, skills, `testdata/apps/ai_demo.lua` |

## 4. Testing

* `internal/ai/*_test.go`: mock LLM via `httptest` (SSE + JSON), Lua-block extraction, knowledge-pack sync test (fails if `api_doc.go` run-mode subset drifts — mirrors `TestApiDocSync` / `make check-api`).
* E2E with fake LLM: `generate → checker.Check → run --test` green; auto-fix loop converges on injected `kbogus.lua`-style error.
* Builder endpoint tests (existing `builder_test.go` pattern) + `node --check` on touched assets; gate `go generate ./... + go vet + go test ./...` stays green.

## 5. Risks / open decisions

* **Context size on local models** — mitigated by run-mode-only compact pack + retrieval later; large models via OpenRouter for complex apps.
* **OpenUI bundle weight vs vanilla** — spike first: if `browser-bundle` > acceptable or React clashes with vanilla `builder.js`, ship vanilla chat with same API and keep OpenUI descriptors prompt-side only.
* **Streaming** — Phase 1 non-streaming; SSE passthrough in Phase 3.
* **Safety** — generated code always passes `check` + sandboxed `run --test` before preview; filesystem writes confined to the builder's open file (existing `--allow-fs` rules apply).

**Status**: Phases 1–4 complete + handler preservation (2026-09-11): the builder captures inline `onclick`/`k.form.on` function bodies as source at import (AST→Lua printer in `internal/builder/lua_printer.go`, stored in `Control.Inline`/`Form.HandlerBodies`) and re-injects them on Save/Export, so handlers survive the JSON round-trip; the foot-of-page import note was reworded from "function body cannot be serialized; skipped". Previous bugfix pass: CLI `ai generate/fix`
now inherits the `KALUA_AI_*` env vars (previously only `--base-url/--model`
flags were read; env was ignored → bare `Post "/chat/completions"`). Builder
gained `--model/--base-url/--api-key-env` flags, env values are trimmed, the
AI status ping timeout is 20s, and `/api/ai/status` reports the provider label
and surfaces the exact backend error in the chat panel. Verified against the
real OpenRouter endpoint (401 "User not found" with a fake key proves the
client sends the Bearer header and the request path is correct). Phase 3 shipped the **vanilla chat panel** (not the React OpenUI bundle) because the builder's CSP is `script-src 'self'` and the app is vanilla JS with `go:embed` assets — CDN loading of OpenUI's React renderer is blocked. The OpenUI concept that matters (component library → prompt instructions) was ported to Go as a static component descriptor. See the phase notes below.

## Phase 4 ✅ — Polish

- **Conversation history**: `GenerateRequest.History []ChatMessage` threaded through `buildMessages` (system → history → current user); builder `/api/ai/generate|fix|stream` accept `history`. Frontend keeps `AI.history` (user/assistant turns), appends each generation/fix, rolls back on failure, clears on Clear.
- **Edit existing**: "Edit current form" checkbox in the chat panel sends the open Lua source as `script` context so follow-ups modify the form instead of starting over; multi-turn follow-ups keep prior turns.
- **Docs**: `docs/USER_GUIDE.md` §7.6 (AI builder: CLI + panel + env config + pipeline) and §7.5 endpoint table; `kalua_spec.md` §6.1 `ai` command + decision-log entry D20; `_opencode/skills/kalua-authoring/SKILL.md` AI-assisted authoring loop.
- **Demo**: `testdata/apps/ai_demo.lua` (AI-builder-shaped run-mode form) passes `KALUA check`.

## Phase 1 ✅ — AI core + CLI

- `internal/ai/provider.go` — OpenAI-compatible LLM client (LM Studio / OpenRouter)
- `internal/ai/knowledge.go` — system prompt built from `api_doc.go` (run-mode subset)
- `internal/ai/generate.go` — NL → Lua with validation→fix loop (≤3 retries)
- `internal/cli/ai.go` — `KALUA ai {generate,fix,validate}` CLI
- `internal/ai/ai_test.go` — 8 tests (provider, generate, fix loop, prompt, lint)

## Phase 2 ✅ — Builder chat endpoints

- `POST /api/ai/generate` — `{request, script?}` → generates Lua, writes to file, validates
- `POST /api/ai/fix` — `{script}` → fixes via LLM, writes back, validates
- `GET /api/ai/status` — provider, model, reachable (live LLM ping)
- `internal/builder/ai_endpoint_test.go` — endpoint tests (status, generate valid/invalid, fix, methods)

## Phase 3 ✅ — AI chat panel + streaming (OpenUI concept ported to Go)

- `internal/ai/openui_prompt.go` — `KaluaComponentPrompt()`: the OpenUI "component library → prompt" idea as a static Go port. Lists every buildable UI element (form, label, textbox, button, combo, list, checkbox, radio, table, looper, chart, image, msgbox, popup, flow helpers, common opts) with its KALUA constructor and options. Appended to the system prompt so local models stay on a controlled surface.
- `internal/ai/generate.go` — `GenerateStream()`: streams the first-pass LLM tokens (`CompletionStream`), then runs the same validation→fix loop, emitting `token`/`status`/`done` events.
- `internal/ai/provider.go` — `CompletionStream` now parses real Server-Sent Events (`data: {...}` frames + `[DONE]` terminator) — LM Studio/OpenRouter format.
- `internal/builder/server.go` — `POST /api/ai/stream` (SSE): relays `GenerateStream` events over `text/event-stream`; bounded channel decouples the LLM goroutine from the HTTP writer; closes with a `done` event.
- Frontend (vanilla) in `internal/builder/assets/`:
  - `index.html` — "AI" topbar button + slide-over chat panel (Chat/Code tabs, prompt box, Generate/Fix/Apply-to-Builder/Clear buttons, provider/model status).
  - `builder.js` — `wireAI()`: `/api/ai/status` dot, SSE consumption, streaming code view, `Fix` (re-send script+errors), `Apply to Builder` (POST `/api/import` → `normalizeDoc` → re-render).
  - `builder.css` — panel, chat bubbles, status dot, code pane styles.
- Tests: `internal/ai/ai_test.go` (component prompt coverage, `GenerateStream` mock), `internal/builder/ai_endpoint_test.go` (`TestAIStreamE2E` — real HTTP + SSE frames), plus live smoke test vs a Python fake-LLM (status/generate/stream + file write all verified).
- **Decision (plan risk item)**: vendoring the React `@openuidev/browser-bundle` is infeasible here — the builder CSP blocks external scripts and the app is vanilla JS with strict `script-src 'self'`. Ships the plan's pre-authorized fallback: vanilla chat panel with identical `/api/ai/*` contract + the OpenUI prompt-generation concept ported to Go.

# Full agentic development plan

Goal: full agentic development of KALUA apps — **both** run mode (`kalua run`) and serve mode (`kalua serve`). All components must be agent-independent (work with OpenCode, GitHub Copilot, Claude Code, Codex, Cursor), with **OpenCode as the evaluation/testbed platform**.

**User decisions locked**: Full P0–P2 scope · **MCP deferred to P2** · OpenCode testbed = a standalone `dev/testkalua/` workspace with `.opencode/` (canonical agent content stays in the repo's `_opencode/`).

## 1. Evaluation — what exists today

Already in place (keep):
- **`AGENTS.md`** (root, 43 KB) — accurate command/docs/testing/exit-code reference; the emerging cross-tool standard (Claude/Codex/OpenCode read it).
- **Closed agent feedback loop for run mode:** `check` (static: syntax + unknown `k.*` + `main`), `check -w/-l/-d` (canonical gofmt-style formatter), `run --test` (headless smoke, exit 0/1), `run --verbose`, `--repl-on-error`.
- **Drift-gated API reference:** `api_doc.go` → `api.md` via `make gen-api`/`check-api` (CI-guarded); also feeds LSP completion/hover.
- **Two OpenCode skills:** `kalua-api` (70 KB generated reference) + `kalua-authoring` (golden loop, run/serve patterns, conventions, checklist).
- **LSP over stdio** (`KALUA lsp`) — works with any LSP editor + the VSCode extension.
- `ai generate/fix/validate`, `builder` (visual + AI chat panel), `new`, stable exit codes (0/1/2/3).

Gaps that block *full* agentic development:

| Agentic capability | Current support | Gap |
|---|---|---|
| Iterate on errors programmatically | `check` prints only `file: msg` (no line/col), no `--json` | Structured diagnostics missing |
| Verify serve-mode apps headlessly | none (agent must background server + curl) | **No `serve --test` smoke path** |
| Verify run-mode event-handler logic | `run --test` only runs up to `main()` suspension; handlers/timers never exercised | **No UI scenario/assertion testing** |
| `ai` bootstraps API apps | `ai generate` is explicitly run-mode-only (`knowledge.go`) | Serve-mode generation missing |
| Agent knows app structure | must read whole file | No `describe` / AST overview command |
| Cross-agent distribution | only root `AGENTS.md`; skills live in `_opencode/` which OpenCode deliberately does **not** auto-load | No `CLAUDE.md`/Copilot/Cursor entrypoints; OpenCode testbed config is **inert** |
| Authoring convention enforcement | conventions are prose | Rule **prefer `k.*` over generic Lua** not yet codified anywhere |

## 2. Required components — phased plan

### P0 — MVP for real agentic development

- **P0.1 Structured diagnostics (`--json`)** — `--json` flag on `check` (+ `-l`/`-d`): `{ok, issues:[{file,line,col,message}]}`. The checker already computes `Issue{Message,Line,Col}` (`internal/checker/checker.go`); the CLI drops it. Exit codes unchanged. Also add `--json` to `run --test` / `serve --test` output.
- **P0.2 line:col in default `check` output** — change `"file: message"` → `"file:line:col: message"`.
- **P0.3 `serve --test` headless smoke (headline gap)** — boots the real worker pool on an ephemeral port (pattern proven in `internal/server/server_e2e_test.go`), runs `init()`, probes `handle_http` (`--http "GET /path"`, `--expect-status N`, `--expect-body-json …`), opens WS/TCP probe connections (echo assert), runs `shutdown()`, prints PASS/FAIL summary (`--json`), exit 0/1.
- **P0.4 Codify "prefer `k.*`/`K.*`/expression functions over generic Lua"** as a first-class convention, present in: `kalua-authoring` skill, `AGENTS.md` conventions table, the `ai` system prompt (`internal/ai/knowledge.go`), and spec §5.2. Rationale: KALUA functions encode Kalipso coercion semantics (`K.tonum`, `K.truthy`, `iif`, `left`, `sys_date`), work identically in run+serve, and are what the static checker/LSP understand; generic Lua (`string.sub`, `tonumber`, `table.insert`, …) is allowed only when no equivalent exists.
- **P0.5 OpenCode testbed + cross-agent entrypoints** — standalone `dev/testkalua/` workspace with `.opencode/opencode.json` (LSP + permissions), `.opencode/skills/kalua-api|kalua-authoring`, `AGENTS.md`/`CLAUDE.md` (Claude Code, `@`-imports)/`.github/copilot-instructions.md` (Copilot)/`.cursor/rules/kalua.mdc` (Cursor) thin pointers to canonical content, plus `KALUA.INI`. Canonical content stays in the repo root. New `docs/agentic/development.md` — the "Agentic Development Guide" (golden loop, exit codes, `--json`, conventions, per-platform wiring matrix, serve-smoke usage).
- **P0.6 `new` fixes + serve templates** — detect `.lua` suffix (`new foo.lua` → `foo.lua`, not `foo.lua.lua`); `--template` flag (`run-form` default, `run-crud`, `serve-http`, `serve-ws`, `serve-tcp`, `serve-all`). Serve templates show `handle_http`/`handle_ws`/`handle_tcp` stubs + `k.shared.*` + `init`/`shutdown`.

### P1 — Make it smooth

- **P1.1 `test` aggregator** — `KALUA test app.lua [--json]`: auto-detects run vs serve (handler presence), runs check + format-check + the right headless test; one uniform exit code/summary. "The one command agents run."
- **P1.2 `describe app.lua --json`** — AST walk (reuse checker/builder import) → `{entry, forms:[{name,controls}], handlers, entry_functions, k_bindings}`. Agents understand an app before editing.
- **P1.3 `ai generate --mode serve`** — serve-oriented system prompt (handler lifecycle, §2.5 response forms, `k.shared.*`, WS/TCP event shapes, UI bindings forbidden); validate with serve-presence rule. `ai fix` becomes mode-aware.
- **P1.4 Compact quick-reference card** — `docs/agentic/quickref.md` (~2 KB, top ~40 most-used `k.*` + coercion idioms) for cheap in-context loading; generated alongside `api.md` via `make gen-api`.
- **P1.5 `make check-agents` CI gate** — verify `docs/agentic/development.md` exists, `@`-references in platform entrypoints resolve, `quickref.md` exists, `check-api` still passes.

### P2 — Depth (larger builds)

- **P2.1 Run-mode UI scenario testing** — `test runapp.lua --scenario scenarios/login.json`: `set_control → click → assert_value / assert_outbox / assert_msgbox`. Small harness driving `session.Session` inbox/outbox (same pattern as `internal/session/*_test.go`), exposed as a CLI command. Deepest new work — the real payoff for frontend logic.
- **P2.2 `KALUA mcp` (stdio)** — expose `check`, `format`, `run_test`, `serve_test`, `describe`, `query_db`, `lsp_complete`, `lsp_hover` as MCP tools. Strongest agent-independence win (Copilot, Claude, Codex, Cursor, OpenCode all speak MCP); cheap once P0–P1 land since the components exist.

## 3. Effort summary

| Phase | Items | Estimated effort | What it unlocks |
|---|---|---|---|
| **P0** | JSON diagnostics, serve-smoke, conventions, entrypoints, new fixes | ~2 weeks | Agents can iterate on run AND serve apps headlessly |
| **P1** | test aggregator, describe, ai serve, quickref, CI | ~1.5 weeks | Polished, one-command workflow for agents |
| **P2** | UI scenarios, MCP server | ~3+ weeks | Deep frontend logic testing; universal agent interop |

**Status**: planning only (2026-09-18) — nothing implemented yet. Next step when green-lit: P0.1–P0.4 (diagnostics + conventions, fast wins), then P0.3 (`serve --test`) and P0.5 (testbed wiring).

## 4. How OpenCode stays the testbed without coupling

- OpenCode = the **validation harness** for every component: skills load, `opencode.json` permission rules work, JSON diagnostics parse, serve-smoke passes/fails correctly.
- Everything is exposed as **plain CLI + JSON + MCP + standard markdown entrypoints**, so Claude Code, Copilot, Codex, Cursor consume the same artifacts. OpenCode-specific surface stays thin (`.opencode/` in `dev/testkalua/`), canonical content stays agent-neutral (repo root `_opencode/`, `docs/agentic/`, `AGENTS.md`).
