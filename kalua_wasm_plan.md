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

## User Decisions
1. **wa-sqlite**: `github.com/ncruces/go-sqlite3` (WASM-compatible, pure Go)
2. **Relay protocol**: Plain JSON over WebSocket (matching run-mode protocol)
3. **Assets**: Embed all CSS/JS inline in `index.html` (self-contained, works `file://`)
4. **IndexedDB FS**: Subset matching `k.file_*` API
5. **Priority**: Sequential M0→M1→M2→M3→M4