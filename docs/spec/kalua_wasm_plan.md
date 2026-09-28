# KALUA WASM in Browser — Implementation Plan

## Overview
Ship `index.html` + `KALUA.wasm` (bundled via new `KALUA wasm-bundle <app.lua>` subcommand) that runs any `run`-mode KALUA app 100% client-side. The existing browser client (`app.js`, `shell.html`) already speaks the session's outbox/inbox JSON protocol — this phase replaces WS/HTTP transport with an in-page bridge.

---

## Milestone M0: Toolchain Spike (Week 1-2)
**Deliverable:** `hello.wasm` renders a form in Chrome with no server.

### Tasks:
1. Add `//go:build !wasm` to all native-only files:
   - `internal/bindings/{db,files,net,comm,ftp,smtp,pop3,soap,serve}.go`
   - `internal/server/*.go` (serve mode)
2. Create `internal/wasm/main.go`:
   - `func startApp(source string)` — entry point called from JS
   - Embed script or fetch via `syscall/js` (`fetch()`)
   - Initialize `vm.New()` → `bindings.Setup` → `session.New` with WASM transport
3. Create `internal/wasm/bridge.go`:
   - `Transport` impl using `syscall/js` callbacks
   - Outbox → JS `bridge.onMessage(msg)`; Inbox ← JS `bridge.send(msg)`
4. Minimal `index.html` + `wasm_exec.js` (from Go SDK)
5. Build: `GOOS=js GOARCH=wasm go build -o KALUA.wasm ./internal/wasm`
6. Verify: Open `index.html` in Chrome → form renders, buttons click

---

## Milestone M1: Transport Refactor (Week 2-3)
**Deliverable:** Shared bridge package; `app.js` works with both WS and WASM transports.

### Tasks:
1. Extract `internal/web/bridge.go`:
   ```go
   type Transport interface {
       Send(msg OutboxMsg) error
       Recv() <-chan InboxMsg  // or callback-based
       Close() error
   }
   ```
2. Refactor `internal/web/server.go` to use `bridge.Transport` (WS implementation)
3. Refactor `internal/session/session.go` to accept `Transport` instead of WS connection
4. Update `internal/wasm/bridge.go` to implement `Transport`
5. Refactor `app.js`:
   - `const transport = { send: ..., onMessage: ... }`
   - All DOM handlers call `transport.send(msg)`
   - WASM mode: `transport.send = bridge.send` (syscall/js)
   - WS mode: `transport.send = ws.send` (current behavior)
6. Generate `index.html` with embedded assets (CSS, Tabulator, Chart.js, flatpickr, app.js inline)

---

## Milestone M2: Browser Binding Profile (Week 3-4)
**Deliverable:** `k.file_*`, `k.param_*`, `k.http_request`, `k.clipboard_*`, `k.pick_file`, `k.screen_size`, `k.net_ok`, `k.ping`, `k.locale` work in browser.

### Files to create:
- `internal/wasm/bindings_wasm.go` — browser-native bindings
- `internal/wasm/fs_wasm.go` — IndexedDB virtual filesystem

### Binding implementations:

| Binding | Implementation |
|---------|----------------|
| `k.file_open/read/write/close` | IndexedDB object store (`kalua_fs`), handle = numeric ID |
| `k.file_load/save` | IndexedDB get/put (bounded by MaxFileSize) |
| `k.file_list/mkdir/delete/exists/info` | IndexedDB cursor/iteration |
| `k.param_get/set` | `localStorage.getItem/setItem('kalua_params', JSON)` |
| `k.http_request` | `fetch()` with timeout; CORS failure → relay fallback |
| `k.clipboard_set/get` | `navigator.clipboard.writeText/readText` |
| `k.pick_file` | File System Access API (`showOpenFilePicker`) + `<input type=file>` fallback |
| `k.pick_file save/download` | `showSaveFilePicker` / `<a download>` |
| `k.screen_size` | `window.innerWidth/innerHeight` |
| `k.locale` | `navigator.language` |
| `k.net_ok` | `fetch('https://1.1.1.1', {mode:'no-cors'})` |
| `k.ping` | `fetch(host, {method:'HEAD'})` timing |

**Build tag:** `//go:build wasm` on new files; `bindings.Setup` detects `GOOS=js` and registers WASM profile.

---

## Milestone M3: DB + Network Relay (Week 4-5)
**Deliverable:** SQLite via wa-sqlite; MySQL/PG/MSSQL/FTP/SMTP/POP3/TCP via relay.

### Protocol: Plain JSON over WebSocket (matching existing run-mode protocol)

```json
// Run-mode message types reused directly:
{"type": "db_query", "id": "req-1", "handle": "main", "sql": "SELECT * FROM t"}
{"type": "db_query_resp", "id": "req-1", "rows": [...], "last_page": true}
{"type": "ftp_list", "id": "req-2", "handle": "ftp-1", "path": "/"}
{"type": "ftp_list_resp", "id": "req-2", "files": [...]}
{"type": "smtp_send", "id": "req-3", "handle": "smtp-1", "from": "...", "to": [...], "body": "..."}
{"type": "smtp_send_resp", "id": "req-3", "ok": true}
```

### Tasks:
1. **wa-sqlite integration:**
   - Add `github.com/ncruces/go-sqlite3` (supports `GOOS=js GOARCH=wasm`)
   - In `bindings_wasm.go`: `k.connect_sqlite("file:app.db")` → opens wa-sqlite
   - `k.db_*` row paths work against wa-sqlite (same API)
   - Persist DB file to IndexedDB (`kalua_sqlite` store) for durability

2. **Relay binary (`KALUA relay`):**
   - New CLI command: `internal/cli/relay.go`
   - Reuses existing drivers: `db.go` (MySQL/PG/MSSQL), `comm.go`, `ftp.go`, `smtp.go`, `pop3.go`
   - WebSocket server on `ws://127.0.0.1:9090/relay` with CORS
   - Plain JSON protocol matching run-mode message types
   - Uses `github.com/coder/websocket` (already in `go.mod`)

3. **JS relay client (`internal/wasm/relay_client.go`):**
   - WebSocket connection to relay
   - Maps `k.db_*` (non-sqlite), `k.ftp_*`, `k.smtp_*`, `k.pop3_*`, `k.socket_*`, `k.webservice_run` to relay calls
   - Uses existing coroutine suspension (`RequestAsync/PostAsyncResp`)

---

## Milestone M4: Packaging & Verification (Week 5-6)
**Deliverable:** `KALUA wasm-bundle app.lua` → self-contained `dist/`.

### Tasks:
1. **`wasm-bundle` command** (`internal/cli/wasm_bundle.go`):
   - Build WASM: `GOOS=js GOARCH=wasm go build -o KALUA.wasm ./internal/wasm`
   - Generate `index.html` with:
     - Inlined `kalua.css`, `tabulator.min.css/js`, `chart.umd.js`, `flatpickr.min.css/js`, `app.js`
     - Inlined `wasm_exec.js` (from `$(go env GOROOT)/misc/wasm/wasm_exec.js`)
     - Embedded app.lua source (or `fetch('app.lua')` for large scripts)
   - Copy to `dist/`
   - Optional: `--relay` flag to include relay binary in `dist/`

2. **E2E tests** (`internal/wasm/e2e_test.go` with `chromedp`):
   - Pure offline: hello form → click → msgbox → file load/save → chart → table
   - Relay path: spin up relay → MySQL query → FTP list → SMTP send

3. **Documentation:**
   - Update `AGENTS.md` with WASM build/test commands
   - Add `testdata/apps/wasm_demo.lua` (showcases all WASM features)

4. **CI integration:**
   - `make test-wasm` target (requires headless Chrome)
   - `make dist-wasm` for release artifacts

---

## File Tree Summary

```
internal/
├── wasm/                          # NEW
│   ├── main.go                    # startApp() entry point
│   ├── bridge.go                  # Transport impl (syscall/js)
│   ├── bindings_wasm.go           # Browser-native k.* bindings
│   ├── fs_wasm.go                 # IndexedDB virtual FS
│   ├── relay_client.go            # JSON-over-WS relay client
│   └── e2e_test.go                # chromedp tests
├── web/
│   ├── bridge.go                  # NEW: Transport interface + WS impl
│   ├── server.go                  # Uses bridge.Transport
│   └── templates/
│       └── shell.html             # Modified: standalone + embedded assets
├── bindings/
│   ├── bindings.go                # Detect GOOS=js → register WASM profile
│   ├── db.go                      # //go:build !wasm
│   ├── files.go                   # //go:build !wasm
│   ├── net.go                     # //go:build !wasm
│   ├── comm.go                    # //go:build !wasm
│   ├── ftp.go                     # //go:build !wasm
│   ├── smtp.go                    # //go:build !wasm
│   ├── pop3.go                    # //go:build !wasm
│   ├── soap.go                    # //go:build !wasm
│   └── serve.go                   # //go:build !wasm
├── cli/
│   ├── wasm_bundle.go             # NEW: wasm-bundle command
│   └── relay.go                   # NEW: relay subcommand
└── session/
    └── session.go                 # Accept Transport interface
```

---

## Build Commands (for AGENTS.md)

```bash
# M0: Toolchain spike
GOOS=js GOARCH=wasm go build -o KALUA.wasm ./internal/wasm

# M1-M3: Dev iteration (rebuild wasm + open index.html)
GOOS=js GOARCH=wasm go build -o internal/wasm/KALUA.wasm ./internal/wasm

# M4: Production bundle
./KALUA wasm-bundle app.lua -o dist/
# Output: dist/index.html, dist/KALUA.wasm (self-contained)

# With relay
./KALUA wasm-bundle app.lua -o dist/ --relay
# Output: dist/index.html, dist/KALUA.wasm, dist/relay (binary)

# E2E tests
go test -tags=wasm ./internal/wasm/...  # requires chromedp + headless Chrome
```

---

## Dependencies to Add

```go
// go.mod additions
github.com/ncruces/go-sqlite3 v1.x.x   // wa-sqlite for WASM
github.com/chromedp/chromedp v0.x.x    // E2E tests (test only)
```

---

## Out of Scope (Per Spec §14.4)
- ❌ Serve mode worker pool / `k.shared.*` / multi-client
- ❌ Session limits, HTTP server lifecycle
- ❌ Monaco REPL, LSP in browser

---

---
 
## Milestone M5: JS/HTML Simplification via WASM Logic Migration (Week 7-9)
**Goal:** Reduce `app.js` from ~2050 to ~300 lines; eliminate inlined Tabulator/Chart.js/flatpickr from HTML.

**Status: Phase 1–5 implemented (2026-09-28).** The WASM page now ships
`app.minimal.js` (~1370 lines) instead of the full 2050-line client; the rest
moved into the WASM Go binary:

- **Phase 1 — Message router in Go**: `common.RouteOutbox` (pure, natively
  unit-tested in `internal/common/brain_test.go`) maps every session outbox
  message onto a compact "hands" command vocabulary (`stage`, `modal_open`,
  `update_control`, `component`, `component_scan`, `msgbox`, `popup`, `status`,
  `clipboard_*`, `pick_file*`, ...). `internal/wasm/brain.go` (`WasmBrain`)
  owns the JS sink + browser-side component/control-value inventory. Unknown
  message types are dropped instead of reaching the page.
- **Phase 2 — Form/control HTML generation in WASM**: the pure renderer
  (`renderForm`/`renderControl`/`renderTable`/`renderGrid`/`renderChart`/
  looper/image + helpers) was extracted out of the `//go:build !wasm` files
  into `internal/bindings/render.go`, which compiles for both targets; the
  WASM forms profile (`forms_wasm.go`) now emits **real rendered HTML** in
  `render_form`/`update_control` (previously empty), so forms render in-browser
  via the same markup the native client consumes.
- **Phase 3 — Control value & event logic**: the JS hands report raw DOM events
  through a two-argument bridge (`kaluaOnDOMEvent(form, ctrl, event, value)`);
  the brain builds the session `InboxMsg` and tracks reported control values so
  click payloads can be assembled without DOM re-reads.
- **Phase 4 — Component lifecycle**: after every render/update the brain emits a
  `component_scan {scope}` command; the hands execute it against a component
  registry (Tabulator/Grid/Chart/looper/flatpickr) and answer round-trip
  requests (`tabulator_get_data`/`get_selection`, `chart_get_image`) via compact
  `component {kind, op, selector}` commands.
- **Phase 5 — Simplified JS bundle**: `internal/cli/wasm_assets/app.minimal.js`
  is the "hands" — DOM ops, event delegation, browser APIs, and third-party
  component init only. No WebSocket, no transport detection, no ~30-case
  message switch, no ping/reconnect. `wasm-bundle` embeds it in `index.html`.
  The removed `wasm_assets/app.js` copy is gone; the native WS client
  (`internal/web/assets/app.js`) is untouched.

### Current State
| Component | Lines | Responsibility |
|-----------|-------|----------------|
| `app.js` | ~2050 | Transport, event delegation, all UI logic, message routing, form rendering, Tabulator/Chart/Looper/Grid, msgbox/popup/status, form value handling |
| `index.html` template | ~20 | Inlines 6 CSS + 5 JS files + app.js |

### Target Architecture
- **WASM (Go) — "Brain"**: Message routing, form/control HTML generation, protocol logic, state management, component lifecycle
- **JavaScript — "Hands"**: DOM manipulation, event attachment, browser APIs, third-party lib init, actual DOM updates

### Phases

#### Phase 1: Message Router & Protocol Logic (Week 7, ~2 days)
Move `handleMessage` switch (30+ cases) and `sendEvent`/`send` to WASM.

```go
// internal/wasm/bridge.go — new message router
func (b *WasmBridge) handleInbox(msg common.InboxMsg) {
    switch msg.Type {
    case "render_form":     b.renderForm(msg)
    case "update_control":  b.updateControl(msg)
    case "close_form":      b.closeForm(msg)
    case "msgbox":          b.showMsgbox(msg)
    // ... all 30+ cases
    }
}
```

**JS becomes:** Single `onmessage` handler calling `wasmBridge.handleMessage(msg)`

#### Phase 2: Form/Control HTML Generation (Week 7-8, ~3 days)
Move form/control rendering to WASM (reuse existing Go logic in `internal/bindings/forms.go`).

```go
// internal/wasm/forms.go — expose existing form rendering
func (b *WasmBridge) renderForm(msg common.InboxMsg) {
    html := forms.RenderFormHTML(b.env, msg.Form, msg.HTML) // reuse existing Go logic
    b.sendToJS(map[string]any{"type": "dom_update", "selector": "#stage", "html": html})
}
```

**JS becomes:** Single `dom_update` handler doing `el.innerHTML = html`

#### Phase 3: Control Value & Event Logic (Week 8, ~3 days)
Move `getControlValue`, `collectFormValues`, `sendEvent` logic to WASM.

```go
// internal/wasm/controls.go
func (b *WasmBridge) extractFormValues(formName string) map[string]any { ... }

func (b *WasmBridge) handleDOMEvent(form, ctrl, event string, value any) {
    // Map DOM event → protocol event → dispatch to Lua via session inbox
}
```

**JS becomes:** Generic event delegator calling `wasmBridge.onDOMEvent(form, ctrl, event, value)`

#### Phase 4: Component Lifecycle Management (Week 8-9, ~2 days)
Move Tabulator/Chart/Looper/flatpickr init/destroy logic to WASM.

```go
// internal/wasm/components.go
func (b *WasmBridge) initComponents(scopeSelector string) []ComponentCmd {
    // Returns commands for JS to execute
    // e.g., {type: "init_tabulator", selector: "#c:form:ctrl", config: {...}}
}
```

**JS becomes:** Generic component initializer executing WASM-returned commands

#### Phase 5: Simplified JS Bundle & Cleanup (Week 9, ~1 day)
New `app.js` (~300 lines):
```javascript
const wasmBridge = {
    onMessage: (msg) => wasmModule.handleMessage(msg),
    onDOMEvent: (form, ctrl, event, value) => wasmModule.onDOMEvent(form, ctrl, event, value),
    domUpdate: (selector, html) => { document.querySelector(selector).innerHTML = html; },
    initComponent: (cmd) => initComponent(cmd), // Tabulator/Chart/flatpickr
    // Browser APIs only
    clipboard: {...}, filePicker: {...}, fetch: {...}, ...
};
```

**index.html** — loads only `wasm_exec.js`, `KALUA.wasm`, minimal `app.js` (~300 lines). Tabulator/Chart/flatpickr loaded on-demand via WASM commands.

### Effort & Risk

| Phase | Effort | Risk | Notes |
|-------|--------|------|-------|
| 1: Message Router | 2 days | Low | Pure Go logic |
| 2: Form HTML Gen | 3 days | Low | Reuse existing Go code |
| 3: Control/Event | 3 days | Medium | Value coercion edge cases |
| 4: Component Lifecycle | 2 days | Medium | Third-party lib quirks |
| 5: JS Cleanup | 1 day | Low | Deletion |

**Total: ~11 days** to reduce `app.js` from 2050→~300 lines.

### Open Questions
1. **Incremental or big bang?** Phases 1-3 incremental; 4-5 need 1-3.
2. **Tabulator/Chart/flatpickr loading:** On-demand via WASM commands vs keep inlined?
3. **WASM size budget:** Moving HTML gen to WASM adds ~200KB. Acceptable?
4. **Backward compat:** Keep current JS as fallback for native mode?
5. **HTML generation reuse:** Use existing `internal/bindings/forms.go` `renderForm`/`renderControl` functions?

---

## File Tree Additions for M5

```
internal/
├── wasm/
│   ├── bridge.go           # EXTENDED: handleInbox message router
│   ├── forms.go            # NEW: form/control HTML generation
│   ├── controls.go         # NEW: control value extraction, event mapping
│   ├── components.go       # NEW: Tabulator/Chart/Looper init/destroy commands
│   └── app_minimal.go      # NEW: minimal app entry for M5
├── cli/
│   └── wasm_bundle.go      # UPDATED: new app_minimal.js template
```