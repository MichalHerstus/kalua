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

* **Config**: env `KALUA_AI_BASE_URL / KALUA_AI_API_KEY / KALUA_AI_MODEL`, flags `--provider lmstudio|openrouter|custom --model --base-url --api-key-env`. Defaults: lmstudio, `http://localhost:1234/v1`, model `local-model`. OpenRouter requires key; local does not. Keys never leave the Go server — browser talks to `/api/ai/*`, Go proxies to the LLM (avoids CORS + key leak).
* **Knowledge pack (run-mode forms subset)**: `k.form.*`, `k.ctrl.*` (+`k.table.*`, chart/image/grid/layout opts), `k.msgbox`, `k.popup`, `k.print/sleep/yield/quit/error`, `K.*` coercion, flat-global expr funcs relevant to forms (`tostr/tonum/upper/...`), globals `ARGS/CTRL/main`. Hard constraints baked in: `function main()` required; `k.form.new` + `k.form.show`; snake_case; expr funcs are flat globals not `k.*`; no `io/os.execute/require`; suspend semantics of `show/msgbox`. Few-shots pulled from `testdata/apps/` (`hello`, `crud_app`, `layout_demo`, `msgbox_demo`, `chart_demo`). Full `api.md` is too large for local-model context — ship a compacted static pack in Phase 1, keyword-filtered retrieval in Phase 2.
* **OpenUI usage (MVP-minimal)**: embed `@openuidev/browser-bundle` (script-tag, no build step) or `react-ui` chat layout into `internal/builder/assets/index.html` as a prompt/history panel beside the existing canvas. Define Kalua controls as OpenUI component descriptors only to generate prompt instructions via `lang-core`; **do not** render apps with the OpenUI renderer. Fallback if bundle proves heavy: vanilla chat panel with identical `/api/ai/*` contract.

## 3. Implementation phases

| Phase | Work | Files |
|---|---|---|
| **1. AI core + CLI** (~3d) | `internal/ai/` package: OpenAI-compat client (net/http stdlib, non-streaming first), knowledge-pack builder with drift test vs `api_doc.go`, `generate→check→run --test→auto-fix (≤3 retries)` loop; `KALUA ai "build a …" [-o app.lua] [--provider/--model]`, plus `ai fix <app.lua>` reusing the loop | `internal/ai/{provider,knowledge,generate}_*.go`, `internal/cli/ai.go`, CLI usage |
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
