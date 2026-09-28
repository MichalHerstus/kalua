# KALUA Development Overview — Implementation Plans & Status

Roll-up of every implementation plan in `docs/spec/`. Each spec document's
milestones are listed as tables with their current status, cross-checked
against the codebase, `AGENTS.md`, and `git log`.

**Status legend:** ✅ implemented/complete · 🔶 partial · ⏳ pending (plan only) · 🔁 superseded

Generated: 2026-09-28

---

## 1. `kalua_spec.md` — Core spec (§8 Build-out phases)

| # | Phase | Status |
|---|-------|--------|
| 1 | Host skeleton — CLI, sandboxed VM, loader, error handling, exit codes | ✅ |
| 2 | Web core — `net/http` server, WS session bridge, form renderer, session actor | ✅ |
| 3 | Controls full set — 8 controls, events, properties, `CTRL()`, vanilla JS client | ✅ |
| 4 | Database group (T1) — connect/select/sql/insert/update/delete/tx, sqlite + DSN drivers | ✅ |
| 5 | Data & comms (T1) — files, JSON/XML, HTTP request, checksum/encrypt, XML getters | ✅ |
| 6 | Expression-function library — §5.9 globals, coercion tests, LSP support | ✅ |
| 7 | CLI polish — `new`, `check`, `--db/--arg/--allow-fs`, `lsp`, extended testing | ✅ |
| 8 | Server mode (T1) — worker pool, HTTP listener, `k.shared.*`, lifecycle, hot reload | ✅ |
| 9 | Tier 2 wave — CSV/INI/YAML/XML, rows, crypto, ZIP, sockets, timers, status, params, FTP, SMTP/POP3, SOAP; file picker | ✅ |
| 10 | Server mode (T2) — WS listener (`handle_ws`), TCP listener (`handle_tcp`) | ✅ |
| 11 | REPL mode — `KALUA repl` with Monaco Editor, persistent actor, split-view frontend | ⏳ |
| 12 | WASM in browser — static page + `KALUA.wasm`, wa-sqlite, relay; M0–M4 shipped, M5 pending (see §2) | 🔶 |
| 13 | Dynamic control/form styling — `bg/color/font/font_size/style/align`, `k.form.set_property`; Tabulator title colors deferred | ✅ |
| 14 | Kalipso error handling — `k.on_error` + `ERRORCODE`/`ERRORMSG`, `Env.fail` | ✅ |

> Implementation order differed from the list (LSP/server shipped before DB/data groups) — see spec §10.

---

## 2. `kalua_spec.md` §14 / `kalua_wasm_plan.md` — WASM in-browser run mode

| Milestone | Deliverable | Status |
|-----------|-------------|--------|
| M0 — Toolchain spike | `hello.wasm` renders a form in Chrome; `//go:build !wasm` tags; `internal/wasm` entry + bridge | ✅ |
| M1 — Transport refactor | `common.Transport` interface, WS impl, `app.js` transport abstraction, embedded assets | ✅ |
| M2 — Browser binding profile | `k.file_*` (IndexedDB), `k.param_*` (localStorage), `http_request` (fetch), clipboard, pick_file, screen_size, locale, net_ok, ping | ✅ |
| M3 — DB + network relay | wa-sqlite (`k.connect_sqlite`/`k.db_*`); `KALUA n` relay for MySQL/PG/MSSQL/FTP/SMTP/POP3/TCP; JS relay client | ✅ |
| M4 — Packaging & verification | `KALUA wasm-bundle <app.lua>` → self-contained `dist/` (+`--relay`); CLI in `internal/cli/wasm_bundle.go` | ✅ |
| M5 — JS/HTML simplification | Move message router, form HTML gen, control/event logic, component lifecycle into WASM; `app.js` ~2050 → ~300 lines | ⏳ |

M5 phases: 1 Message Router (~2 d) · 2 Form HTML Gen (~3 d) · 3 Control/Event (~3 d) ·
4 Component Lifecycle (~2 d) · 5 JS Cleanup (~1 d). Total ~11 days, not started.

---

## 3. `kalua_spec.md` §11 — Debugging capabilities

| Phase | Scope | Status |
|-------|-------|--------|
| A — Core infrastructure (Tier 1) | Vendored patched gopher-lua (`debug.hook`) ✅ · post-mortem dump ✅ · `--repl-on-error` ✅ · `k.debug.*` ✅ · enhanced `--verbose` 🔶 (traces `k.*` calls only, not general Lua tracing) · CLI flags 🔶 (`--debug`/`--debug-worker` stubs, no `--debug-port` yet) | 🔶 |
| B — DAP integration (Tier 2, decided 2026-09-07) | JSON-RPC TCP adapter on `--debug-port` 9966, hook-wired breakpoints/stepping, variable watch, breakpoint persistence | ⏳ |
| C — Domain features (Tier 3) | Custom formatters, session/actor awareness, `--debug-worker`, hot-reload breakpoint preserve, server request tracing | ⏳ |

Decisions: protocol = **DAP** (not EmmyLua) · sandbox `debug` lib **gated behind `--debug`** ·
`k.debug.*` / `--verbose` / post-mortem always-on · single debug worker in serve mode.

---

## 4. `kalua_spec.md` §13 — Codebase optimization (Phases C & D)

| Phase | Scope | Status |
|-------|-------|--------|
| A+B — Crash/correctness + security hardening | Shipped independently 2026-09-05 | ✅ |
| C — Dead code sweep (C1–C13) | Unreachable functions, legacy async-DB design, stub flags, duplicate registrations | ⏳ |
| D — Deduplication (D1–D15) | Single source of truth: k.* registry, namespaces, coercion, converters, date parsers, JSON, coroutine-resume blocks, stack-walker, handle registries, XML shapes, CLI flags, print/join, HTML helpers, getString | ⏳ (D14 resolved during B1) |

Sequencing: C → D6/D5/D3/D4/D14 → D1/D2 → D7–D13/D15. Phase E (perf) = separate follow-up, unplanned.

---

## 5. `AI_builder.md` — AI builder (NL → `.lua`)

| Phase | Work | Status |
|-------|------|--------|
| 1 — AI core + CLI | `internal/ai/` provider/knowledge/generate, `generate→check→run --test→auto-fix` (≤3 retries), `KALUA ai {generate,fix,validate}` | ✅ |
| 2 — Builder chat endpoints | `POST /api/ai/generate|fix`, `GET /api/ai/status` | ✅ |
| 3 — AI chat panel + streaming | Vanilla chat panel in builder (OpenUI concept ported to Go), SSE `/api/ai/stream`, component prompt | ✅ |
| 4 — Polish | Conversation history, "Edit current form", docs, `ai_demo.lua` | ✅ |

## 6. `AI_builder.md` — Full agentic development plan

| Phase | Items | Status |
|-------|-------|--------|
| P0 — MVP | P0.1 `--json` diagnostics · P0.2 `file:line:col` · P0.3 `serve --test` · P0.4 prefer-`k.*` convention · P0.5 testbed + cross-agent entrypoints · P0.6 `new` fixes + serve templates | ✅ |
| P1 — Smooth | P1.1 `test` aggregator · P1.2 `describe --json` · P1.3 `ai generate --mode serve` · P1.4 quickref card · P1.5 `make check-agents` | ✅ |
| P2 — Depth | P2.1 run-mode UI scenario testing (`--scenario`) · P2.2 `KALUA mcp` (stdio) | ✅ |

P2.2 shipped with 9 MCP tools: check, format, run_test, serve_test, describe, query_db,
lsp_complete, lsp_hover, run_scenario.

---

## 7. `kforms_enhancements.md` — §1 Tabulator table (+ DB-linked)

| Phase | Work | Status |
|-------|------|--------|
| 1 — Assets & Go rendering | Embed Tabulator v6+, `renderTabulatorTable`, `k.table.set_data`, destroy on close | ✅ |
| 2 — Client-Side integration | Instance map, lifecycle, selection bridging, default options | ✅ |
| 3 — Remote pagination | `tabulator_ajax_request` ↔ `tabulator_remote_data` | ✅ |
| 4 — Selection & query API | `k.table.get_selected_rows` / `get_data` | ✅ |
| 5 — Checker & LSP | validate/lint+complete new options | ✅ |
| 6 — CSS & polish | Simple-theme overrides, edge cases | ✅ |
| B1–B7 — DB-linked tables | `db/query/...` opts, Go pager (safe sort/filter), `k.table.refresh`/`set_db_source`, client refresh, tests, demo | ✅ |

## 8. `kforms_enhancements.md` — §2 Looper control (+ DB-linked, + row templates)

| Phase | Work | Status |
|-------|------|--------|
| 1 — Core structure | `k.ctrl.looper`, template controls, basic render | ✅ |
| 2 — Data operations | `k.looper.add_line/delete_line/set_line/get_line/clear` | ✅ |
| 3 — DB linking | `k.looper.link_db`/`refresh`, Go pager (L1–L6) | ✅ |
| 4 — Virtual scrolling | Sentinel/IntersectionObserver batching | ✅ |
| 5 — Events & polish | `onclick`/`onchange`/`onselect`, CSS, destroy on close | ✅ |
| 6 — Checker & LSP | looper API validation + completions | ✅ |
| 4-r — Row-template controls (Phase 4 runtime) | `opts.row` = control-def array, `BuildLooperRowHTML`, `{index,html}` batch rows, client insert | ✅ |

## 9. `kforms_enhancements.md` — §3 Chart control (Chart.js)

| Phase | Work | Status |
|-------|------|--------|
| 1 | Assets & Go rendering | ✅ |
| 2 | Client-Side Chart.js integration | ✅ |
| 3 | Data management API (`k.chart.set_data`, datasets ops, labels, options, resize) | ✅ |
| 4 | Events & interaction (`chart_click`/`chart_hover`/`chart_legend_click`) | ✅ |
| 5 | Image export (`k.chart.get_image`), combo charts | ✅ |
| 6 | Checker, LSP, tests, CSS | ✅ |

## 10. `kforms_enhancements.md` — §4 Extended form controls

| Phase | Work | Status |
|-------|------|--------|
| 1 | Textbox `multiline` (textarea, rows/cols) | ✅ |
| 2 | Textbox `datetime` + flatpickr (modes date/time/datetime, format tokens) | ✅ |
| 3 | Label `multiline` (pre-wrap) | ✅ |
| 4 | Image control (`k.ctrl.image`, src/alt/size/fit/clickable, set_value→src) | ✅ |
| 5 | Checker, LSP, tests | ✅ |

## 11. `kforms_enhancements.md` — §5 Form builder (Option B standalone)

| Phase | Work | Status |
|-------|------|--------|
| 1 — Foundation | `internal/builder`, `KALUA builder`, HTTP API, embedded shell | ✅ |
| 2 — Preview engine | Go-rendered canvas (in-process `renderForm`), selection hit-test | ✅ |
| 3 — Canvas & palette | Drag-drop, reorder, delete, duplicate | ✅ |
| 4 — Property editor | Per-type editors incl. special editors (items/cells/datetime/chart/table/looper/DB) | ✅ |
| 5 — Grid layout editing | Cells editor, `cell` assignment, align, gap | ✅ |
| 6 — Import / export / validate | `lua_import`/`lua_export` (AST), round-trip, preflight | ✅ |
| 7 — Polish | Undo/redo, shortcuts, docs | ✅ |
| 8 — Multi-form (schema v3) + source-preserving save | Multi-form docs, AST import with line spans, surgical `RebuildLua` splice, verbatim handlers, idempotent save | ✅ |
| 9 — Table/Looper editor modal | Tabbed Datasource / Table Setup or Row Template / Preview; named DB handles; `/api/db`, `/api/db/query`, `/api/looper/rows`, `/static/tabulator` proxy | ✅ |

## 12. `kforms_enhancements.md` — §6 Enhanced form layout

| Phase | Work | Status |
|-------|------|--------|
| 1 | Vertical alignment (`align` form/control) + `gap` | ✅ |
| 2 | Grid layout with cells (width/bg/border/align), ordered or map cells, auto-"main" fallback | ✅ |
| 3 | Mobile responsive (< 600px single column) | ✅ |
| 4 | Dynamic cell re-assignment (`set_property("cell")` → full re-render), per-control `align-self` | ✅ |

## 13. `kforms_enhancements.md` — §7 CRUD Grid control

| Phase | Work | Status |
|-------|------|--------|
| 1 — Core grid control | `k.ctrl.grid`, Tabulator integration, DB-linked paging, PK inference, selection | ✅ |
| 2 — Client interactions | Action column, global toolbar, row click, modal form | ✅ |
| 3 — Server CRUD | `GridInsert/Update/DeleteMany`, PK handling, async ops | ✅ |
| 4 — Form integration | `on_save`/`on_cancel` validation hooks, grid-level events, `k.grid.*` ops | ✅ |
| 5 — Builder integration | CRUD tab, live preview, export/import | ✅ |
| 6 — Documentation & polish | api_doc, USER_GUIDE, grid e2e tests, demo app | ✅ |

## 10. `kforms_enhancements.md` — §10 Layout Controls: Topbar, Sidebar, Footer

| Phase | Work | Status |
|-------|------|--------|
| 1 | Topbar — `k.ctrl.topbar` with app icon, user info, logout | ⏳ |
| 2 | Sidebar — `k.ctrl.sidebar` (grid cell + horizontal tabs), collapsible | ⏳ |
| 3 | Footer — `k.ctrl.footer` with version, copyright, links | ⏳ |
| 4 | Layout integration — Grid (cell) + Vertical (horizontal tabs) | ⏳ |
| 5 | Go renderer (`render.go`) + JS hands (`app.minimal.js`) | ⏳ |

## 14. `kforms_enhancements.md` — §8 Login control

| Phase | Work | Status |
|-------|------|--------|
| 1 | Textbox `{password=true}` | ⏳ |
| 2 | Verification core (`login.go`: `LookupLoginUser`, `VerifyLoginPassword`, pbkdf2) | ⏳ |
| 3 | Rendering (`renderLoginPanel`) | ⏳ |
| 4 | Registration & plumbing (registry, checker, api_doc, serve, AI knowledge) | ⏳ |
| 5 | Client (submit/cancel, Enter-to-submit) | ⏳ |
| 6 | Session (`loginDispatch`, retry re-render) | ⏳ |
| 7 | Tests, docs, demo (~8 days total) | ⏳ |

## 15. `kforms_enhancements.md` — §9 Tree control

| Phase | Work | Status |
|-------|------|--------|
| 1 | `tree.go` — `TreeLinkFromControl` + `FetchTreeRows` + `BuildTree` (2 d) | ⏳ |
| 2 | `renderTree` in `forms.go` + registry/api-doc surfaces (1 d) | ⏳ |
| 3 | Builder palette + property editor + export/import + preview (1.5 d) | ⏳ |
| 4 | Client `app.js` — `initTrees`/`handleTreeData`/toggle/click (1.5 d) | ⏳ |
| 5 | Session handlers — `tree_data_request`, `treeDispatch`, events (1 d) | ⏳ |
| 6 | Tests — bindings unit + session e2e (1 d) | ⏳ |
| 7 | Docs + demo app + `make gen-api` (1 d) | ⏳ |

---

## 16. `kform_builder_plan.md` — Original builder plan (webview)

| Phase | Work | Status |
|-------|------|--------|
| 1 — Foundation | VS Code webview + schema + types | 🔁 Option B (standalone `KALUA builder`) chosen |
| 2 — Preview engine | TS render control + kalua.css in webview | 🔁 Go-rendered canvas |
| 3 — Controls palette | Drag-and-drop | 🔁 Implemented in standalone builder |
| 4 — Property editor | Per-control-type dynamic form | 🔁 Implemented in standalone builder |
| 5 — Layout & ordering | Reorder/delete/copy, form props | 🔁 Implemented in standalone builder |
| 6 — Import/export | Lua ↔ JSON `.kalua-form.json` | 🔁 `lua_import`/`lua_export` (AST) |
| 7 — Polish | Undo/redo, shortcuts, responsive, docs | 🔁 Implemented in standalone builder |
| 8 — Advanced Table & Looper editor + named DB handles | `/api/db`, `/api/db/query`, `/api/looper/rows`, row-template controls | ✅ (landed in the standalone builder) |

> Superseded by `kforms_enhancements.md` §5 (Option B). The only part still tracked
> here — named DB handles + table/looper editor + looper row templates — is ✅ done.

---

## 17. `vscode-ext.md` — VS Code extension

| Area | Status |
|------|--------|
| Extension runtime — `kalua` language id, `KALUA lsp` stdio client, commands (Check/Run/New app), `kalua.binaryPath` | ✅ |
| LSP capabilities — full sync, completion (`.`/`'`/`"`), hover, definition, UTF-8 positions | ✅ |
| Package/build guide — `npm install/compile/package`, F5 launch, troubleshooting | ✅ (reference doc) |

No milestone plan in this document — it is a development guide for the shipped
extension (Phase 5 LSP & editor).

---

## Cross-document summary

| Document | Plan | Overall status |
|----------|------|----------------|
| `kalua_spec.md` | §8 build-out phases 1–14, §11 debug, §13 cleanup, §14 WASM | 🔶 REPL (11) pending; debug Tier 2/3 pending; cleanup C/D pending; WASM M5 pending |
| `kalua_wasm_plan.md` | M0–M5 | 🔶 M0–M4 ✅, M5 ⏳ |
| `AI_builder.md` | AI phases 1–4, agentic P0–P2 | ✅ complete |
| `kform_builder_plan.md` | Webview builder phases 1–7, table/looper editor | 🔁 superseded; table/looper + named DBs ✅ |
| `kforms_enhancements.md` | §§1–11 | 🔶 §§1–7 ✅, §8 login ⏳, §9 tree ⏳, §10 layout controls ⏳ |
| `vscode-ext.md` | Extension guide | ✅ shipped |

**Top pending plan items (no code yet):** REPL mode (§8 #11) · DAP debugger (§11 B/C) ·
codebase cleanup C/D (§13) · WASM M5 JS simplification · `k.ctrl.login` (§8) ·
`k.ctrl.tree` (§9) · **Layout Controls: Topbar, Sidebar, Footer (§11) · Image onclick handling (§4.3)**.

---

*Generated from specs in `docs/spec/`. Run `make gen-api && make check-api` to verify API docs sync.*