# KALUA — Kalipso-style web apps in embedded Lua

**KALUA** is a runtime (not a compiler): one generic Go binary, `KALUA`, embeds a
sandboxed **gopher-lua** VM, a **Go `net/http` server**, and a **templ**-rendered web UI
with a vanilla-JavaScript client. Applications are plain `.lua` files written against a
`k.*` API that mirrors Kalipso 5.0 actions and functions. Kalipso forms are displayed as
web pages in a browser tab.

> Former "Approach A" (transpile Kalipso Copy-as-Text → Go source → `go build`) was dropped
> on 2026-08-23 in favor of this runtime approach. Rationale: no compiler pipeline, dynamic
> Lua absorbs Kalipso weak typing, hot edit/run cycle, single runner binary matches the
> "app = one file" vision. Ground-truth grammar work from Approach A is no longer needed.
>
> UI switched from a tview TUI to a web app on 2026-08-28 (decisions D3, D15–D18): forms
> render in the browser; a browser tab is a KALUA session with its own Lua VM.

## Evaluation basis (2026-08-23, UI re-evaluated 2026-08-28)

| Package | Status when evaluated | Notes |
|---------|----------------------|-------|
| `github.com/yuin/gopher-lua` | v1.1.2 (Apr 2026), slow-but-alive, ~90% coverage | De-facto standard pure-Go Lua 5.1 VM; coroutines + Go channels; `SkipOpenLibs` sandboxing |
| `github.com/a-h/templ` | v0.3x, active | Typed server-side templates compiling to Go; auto-escaped output; `go:generate templ generate` |
| `github.com/coder/websocket` | v1.8+ (formerly nhooyr.io/websocket), active | Minimal, context-aware WebSocket for `net/http`; the only networking dependency beyond stdlib |
| Browser client | n/a | Vanilla JS (WebSocket/WebAudio/clipboard/DOM APIs) + one embedded CSS file. No JS modules or frameworks |

Ground truth for the action/function inventory: official Kalipso 5.0 docs
(doc.sysdevmobile.com/kalipso5) — 13 action groups and the expression-functions taxonomy
(String/Numeric/Conditional/Date/Time/Conversion + Operators + Keywords).

## Decision log

| # | Topic | Decision | Date |
|---|-------|----------|------|
| D1 | Approach | Runtime in embedded Lua; transpiler dropped | 2026-08-23 |
| D2 | App format | Plain `.lua` files; declarations via `k.declare()` | 2026-08-23 |
| D3 | UI | Web app: Go `net/http` server + templ pages + vanilla JS (replaces tview) | 2026-08-28 |
| D4 | Distribution | One generic `KALUA` runner binary interpreting any `{app}.lua` | 2026-08-23 |
| D5 | API naming | `snake_case`, recognizable from Kalipso names (`k.set_value`, `k.show_form`) | 2026-08-23 |
| D6 | Entry point | Script must define `function main()` | 2026-08-23 |
| D7 | Form planes | Single-plane forms (Kalipso plane actions out of scope) | 2026-08-23 |
| D8 | Visual-heavy controls excluded | Draw, Image, Shape, Chart, HTML Viewer, Media Player, Looper | 2026-08-23 |
| D9 | Hardware/remote-device groups out of scope | Barcode, RFID, GPS, Voice, GPIO, Camera/Sensors, Phone/SMS/Push, Bluetooth/BLE/Beacons, GPRS, Printing, DLL/JAR, In-App Purchase, MIS Communicator, MSS, Synchronization | 2026-08-19, reaffirmed 2026-08-23 |
| D10 | Databases | MariaDB/MySQL, Postgres, MSSQL via `k.connect_db({DSN})`; SQLite Tier 2 | 2026-08-19 |
| D11 | CLI shape | `KALUA {command} {file} --flags` | 2026-08-19 |
| D12 | Fidelity goal | Technology project; documented-behavior fidelity, not commercial migration | 2026-08-19 |
| D13 | Data-format IO | Unified `k.{fmt}_load/_save/_parse/_string` actions for JSON, XML, YAML, CSV, INI (KALUA-specific, §5.10) | 2026-08-23 |
| D14 | Server mode | Per-request LState worker pool; Go-side shared state via `k.shared_*`; HTTP/WebSocket/TCP listeners | 2026-08-24 |
| D15 | Run-mode concurrency | Session actor: one LState per browser tab, owned by one goroutine; events serialized through an inbox channel | 2026-08-28 |
| D16 | Render model | Server-side templ renders HTML fragments for every UI command; vanilla JS injects at a target selector | 2026-08-28 |
| D17 | UI transport | WebSocket (`github.com/coder/websocket`) between browser and server | 2026-08-28 |
| D18 | Run vs serve | `run` = interactive web app (UI bindings live); `serve` = headless API (D14), UI bindings are errors | 2026-08-28 |

---

# Specification

## 1. Architecture

```
myapp.lua ──► KALUA run myapp.lua
                  │
         ┌────────┴──────────────────────────────────────────┐
         │ host (Go)                                          │
         │  net/http server (listen 127.0.0.1:PORT)           │
         │  routing: GET / (page shell) · /ws/ui (WebSocket)  │
         │  templ: shell/form/control/msgbox renderers        │
         │  assets: app.js (vanilla) + kalua.css (embedded)   │
         │                                                    │
         │  per browser tab → Session (actor goroutine):      │
         │    owns a sandboxed gopher-lua LState              │
         │    inbox  = typed events (WS msgs, timers, async   │
         │              completions) — serialized by channel  │
         │    outbox = UI commands → templ fragments → WS     │
         │    bindings k.* (flow/forms/controls/db/files/…)   │
         │  worker goroutines for blocking ops (db/http,      │
         │  sleep) → results posted into the session inbox    │
         └────────────────────────────────────────────────────┘

server.lua ──► KALUA serve server.lua --port 8080 --workers 4
                  │
         ┌────────┴──────────────────────────────┐
         │ host (Go)                             │
         │  worker pool (N goroutines)           │
         │    each: pre-created LState           │
         │    bindings: k.* + k.shared_*         │
         │  HTTP / WebSocket / TCP listener      │
         │  Go-side shared state (sync.Map)      │
         │  graceful shutdown (SIGTERM/SIGHUP)   │
         └───────────────────────────────────────┘
```

Repo layout:

```
cmd/KALUA/                 # CLI: run/check/new/version
internal/host/            # app lifecycle, flags, exit codes, logging
internal/vm/              # LState setup, sandbox whitelist, script loader
internal/coerce/          # K.eq/ne/add value semantics shared by all bindings
internal/bindings/        # k.* registration, one file per group:
                          #   flow.go forms.go controls.go db.go
                          #   files.go formats.go jsonxml.go http.go crypto.go
                          #   datetime.go comm.go email.go shared.go
                          #   funcs_str.go funcs_num.go ...
internal/session/         # run-mode actor: inbox, outbox, coroutine resume,
                          # timers, async completions, teardown
internal/web/             # net/http server, routing, WS bridge (coder/websocket),
                          # embedded assets
internal/web/templates/   # templ: shell.templ, form.templ, ctrl_*.templ,
                          # msgbox.templ, status.templ
internal/web/assets/      # app.js (vanilla), kalua.css
internal/server/          # serve mode: worker pool, listeners, lifecycle
testdata/apps/            # fixture .lua apps + expected outputs (incl. HTML goldens)
```

templ codegen is wired into the build (`//go:generate templ generate`, `go generate ./...`
run in CI and before `go build`).

## 2. Runtime model

### 2.1 VM setup & sandboxing

- `lua.NewState(lua.Options{SkipOpenLibs: true})`, then open a whitelist only:
  `base` (minus `load*`, `rawset`… reviewed), `string`, `table`, `math`, `os.time/clock/date`
  subset. No `io`, no `os.execute`, no `require`, no debug hooks.
- All host functionality reaches the script exclusively through `k.*` bindings and the
  expression-function globals (§5). Anything not exposed does not exist.
- Script size/time guard: VM context with generous-but-finite instruction budget between
  yields; runaway pure-Lua loops are killed with a diagnostic.

### 2.2 Threading & concurrency core (foundational, D15)

The gopher-lua `LState` is **not goroutine-safe**. KALUA serializes all Lua execution per
session with one actor goroutine per browser tab:

1. A `Session` owns its `LState` and executes **all** Lua for that tab. Handlers
   (form events, button clicks, timers, async completions) run there — serialized by
   construction, zero locks.
2. The actor drains a typed **inbox** channel: WS events from the browser (`event`,
   `key`, `msgbox_choice`, `client_info`), timer fires, async completions, app teardown.
   One inbox message is processed at a time.
3. After every handler returns, the actor drains the pending UI **outbox** and flushes it
   to the browser as WebSocket messages (rendered templ fragments). Batching guarantees a
   handler that triggers several UI changes produces coherent screen updates.
4. A blocking operation (DB query, HTTP, FTP, sleep, `form.show`, `msgbox`) must not
   freeze the session. Binding pattern:
   - spawn a worker goroutine for the Go-side work (or for `sleep`, a timer);
   - suspend the calling Lua coroutine (every handler runs inside a coroutine; the
     binding calls `coroutine.yield` on an internal control channel);
   - when the work finishes, post a completion message into the session inbox;
   - the actor resumes the coroutine with the results, then flushes the outbox.
5. `k.form.show(name)` suspends the caller until the form closes (matches Kalipso's modal
   Show Form). The form stack mirrors nested Show Form calls. `k.msgbox(...)` likewise
   suspends until the browser posts a button choice (D16 protocol, §3.4).
6. Timers (`k.timer_start`) are session-scoped: the actor owns the timer additions; fires
   post a message into the inbox. Timer handlers run inside the actor like everything else.
7. Teardown: on WS close / reload / `k.quit`, the actor cancels the session context
   (stopping timers and in-flight worker goroutines), runs `close_form` cleanup and ends.
   Active LStates are recycled safely; async completions for a dead session are a no-op.
8. Guidance: any handler that computes >~50 ms in pure Lua should call `k.yield()`
   (returns to the actor loop so pending events/worker completions can interleave).

### 2.3 Value model & coercion

Lua values are used directly (string / number / boolean / nil / table). Kalipso's two data
types — **Numeric** and **String** — plus its weak-typing quirks are reproduced by helpers
in `internal/coerce`, also exported to scripts:

| Helper | Semantics (from documented Kalipso behavior) |
|--------|----------------------------------------------|
| `K.eq(a,b)` | numeric-vs-string compares numerically when both coerce to numbers (`0 = ""` → true); falls back to string compare |
| `K.ne(a,b)` | negation of `K.eq` |
| `K.add(a,b)` | Kalipso `+`: numeric if both coerce, else concatenation quirks per docs |
| `K.tonum(v)`, `K.tostr(v)` | Set Value-style coercion (Numeric/String types of the original action) |
| `K.truthy(v)` | Kalipso condition truthiness for `If(...)` |

Direct Lua operators remain available; authors who need byte-exact Kalipso semantics in
mixed-type expressions use the helpers. Bindings themselves always normalize through
`coerce` so DB params/results behave identically everywhere.

### 2.4 Server mode runtime model (D14, unchanged; UI bindings unavailable)

**Concurrency model:**
- **Worker pool**: `--workers N` goroutines (default: `GOMAXPROCS`), each with a pre-created `LState`
- **Per-request isolation**: Each HTTP/WebSocket/TCP request runs in a fresh coroutine on a worker's `LState`
- **No shared Lua state**: Workers do not share Lua globals; communication via Go-side shared store

**Shared state (`k.shared_*`):**
- Backed by Go `sync.Map` — lock-free reads, sharded writes
- Values serialized as JSON (Lua primitives, tables → JSON; functions/threads → error)
- Persists across requests and worker restarts (until process exit)

**Request lifecycle:**
1. Listener accepts connection → dispatches to next available worker (round-robin)
2. Worker: `pcall(handle_http, req)` (or `handle_ws`, `handle_tcp`)
3. On success: serialize return value → HTTP response / WS message / TCP write
4. On error: log + return 500 / close connection
5. Worker returns to pool; `LState` reused (instruction budget reset)

**Entry points (script-defined):**
```lua
-- server.lua
function init(config)        -- optional: runs once at startup in dedicated LState
  k.connect_db("...")
  k.shared_set("version", "1.0")
end

function handle_http(req)    -- required for HTTP mode
  return {status=200, body="OK"}
end

function handle_ws(msg)      -- required for WebSocket mode
  k.ws_broadcast({type="text", data=msg.data})
end

function handle_tcp(data)    -- required for TCP mode
  k.tcp_send(msg.client_id, "echo: " .. data)
end

function shutdown()          -- optional: runs on SIGTERM before exit
  k.disconnect_db()
end
```

**HTTP `req` table:**
```lua
{ method="GET", path="/api", query={...}, headers={...}, body="", remote_addr="1.2.3.4:5678", tls=false }
```

**HTTP response (return value):**
```lua
-- Full form
{ status=200, headers={["content-type"]="application/json"}, body="..." }
-- Shortcuts
"plain text"           → 200, text/plain
{json={...}}            → 200, application/json
{status=404}            → 404, empty body
```

**WebSocket `msg` table:**
```lua
{ type="text|binary|ping|pong|close", data="...", client_id="uuid" }
```

**Lifecycle & signals:**
- `SIGTERM` / `SIGINT`: stop accepting, drain workers, call `shutdown()`, exit
- `SIGHUP`: hot reload — recompile script, swap worker `LStates` atomically

**Interactions with the run-mode UI:** `serve` is headless (D18). UI/forms bindings
(`k.form.*`, `k.ctrl.*`, `k.msgbox`, `k.status_*`) raise a runtime error in serve mode.
`k.print` writes to the log sink. The two modes share command parsing, the sandbox, the
bindings registry, and the coerce layer; they differ only in lifecycle + event delivery.

## 3. Forms & controls

### 3.1 Form model

- `k.form.new(name, {title=…})`, `k.form.add_*` builders, `k.form.show(name)` (pushes onto
  the form stack and suspends caller), `k.form.close([name])`, `k.form.return_to(name)`
  (closes everything above `name`), `k.form.clear/refresh(name)`.
- Single plane per form (D7). Layout: vertical stack of controls by default +
  `layout="grid"` option with row/col hints; rendered as CSS flex column / CSS grid.
- Form events: `open_form`, `after_open_form`, `close_form`, `key_pressed`, `on_idle(ms)`,
  `timer(id)`. Handlers are plain Lua functions set via options or `k.form.on(name, event, fn)`.
- The visible form is the top of the stack; content is (re)rendered through templ each time
  the stack or a control changes (§3.4).

### 3.2 Control mapping (web)

Elements are generated by templ and carry data attributes `data-k-form`, `data-k-ctrl`,
`data-k-event` so the single delegated vanilla-JS handler can bind every control without
re-attaching listeners on each inject.

| Kalipso control | KALUA constructor | HTML element | Events supported |
|---|---|---|---|
| Label | `k.ctrl.label(form,name,{text})` | `<label>` | — |
| Text Box | `k.ctrl.textbox(...)` | `<input type="text">` | `whenever_modified` (input), `get_focus`, `lose_focus`, `key_pressed` |
| Button | `k.ctrl.button(...)` | `<button type="button">` | `click` |
| Combo Box | `k.ctrl.combo(...)` | `<select>` | `selection_change` |
| List | `k.ctrl.list(...)` | `<select size=N>` | `selection_change`, `item_added`, `click` |
| Table (Grid) | `k.ctrl.table(...)` | `<table>` | `cell_value_modified`, `column_header_click`, `selection_change` |
| Check Box | `k.ctrl.checkbox(...)` | `<input type="checkbox">` | `check`, `uncheck` |
| Radio Button | `k.ctrl.radio(...)` | `<input type="radio" name="...">` group | `selection_change` |

Excluded (D8): Draw, Image, Shape, Chart, HTML Viewer, Media Player, Looper, Scroll Area, Tab.

### 3.3 Control API (Group Controls actions)

`k.ctrl.set_value/get_value`, `k.ctrl.set_property/get_property` (property subset:
`text`, `value`, `enabled`, `visible`, `title`, `color`, `items`, `selected_index`),
`k.ctrl.set_focus`, `k.ctrl.set_selection/get_selection`, `k.ctrl.get_item_count`,
`k.ctrl.select_text`, `k.ctrl.execute_event(ctrl,event)` (invokes handler directly),
`k.ctrl.refresh(ctrl)`. Table-specific: `k.table.add_line/delete_line/set_column_value/
get_column_value/find/set_selected_column/get_selected_column` (MVP);
column width/order/color (Tier 2). Hidden value per control: `hidden_value` property.

Property → HTML mapping: `text`→label/textContent · `value`→input value · `enabled`→
`disabled` attr · `visible`→`hidden` · `title`→label · `color`→style color ·
`items`→`<option>`s · `selected_index`→`selectedIndex`.

### 3.4 Client protocol (WebSocket, D16/D17)

One WS connection per session at `/ws/ui`. Messages are JSON.

**Server → client** (payloads are server-rendered templ fragments):

| type | shape | meaning |
|------|-------|---------|
| `init` | `{session_id}` | session established |
| `render_form` | `{html}` | inject whole form at `#stage` (top of stack) |
| `update_control` | `{selector, html}` | replace one control (e.g. after `set_value`/`refresh`) |
| `close_form` | `{name, top?}` | pop current / all-above on `return_to` |
| `msgbox` | `{id, kind, html}` | show templ-rendered modal (info/warn/error/ok-cancel/yes-no) |
| `close_msgbox` | `{id}` | dismiss modal |
| `status` | `{text}` | busy/status bar update |
| `error` | `{msg, stack?}` | runtime error banner (app keeps running for event errors) |
| `quit` | — | app ended; page shows terminal state |

**Client → server** (vanilla JS in `assets/app.js`):

| type | shape | meaning |
|------|-------|---------|
| `event` | `{form, ctrl, event_name, value}` | control event (`click`, `input`, `change`, `focus`, `blur`, checkbox/radio value) |
| `key` | `{form, ctrl, key, code}` | keydown on focused control → `key_pressed` |
| `msgbox_choice` | `{id, choice}` | button pressed in modal → resumes suspended coroutine |
| `client_info` | `{w, h, locale}` | on connect: `screen_size` source, locale |
| `ping` | — | keep-alive / detect dead session |

Rendering rules: controls get `id="c:<form>:<name>"`, forms `id="f:<name>"`. The client
injects via `document.getElementById(...)`; it never parses or executes markup beyond
`innerHTML` of trusted server fragments. User strings are data, never markup — templ
auto-escapes and `templ.Raw` is banned for user-controlled content.

Browser-side capabilities used from Lua (§5.2): clipboard (`navigator.clipboard`), bell
(WebAudio), screen size (`client_info`), file picker (`<input type="file">`), keyboard
(`key` messages). No external JS modules or frameworks are loaded.

## 4. App format & error handling

```lua
-- orders.lua
function main()
  k.connect_db("mysql://user:pw@host/erp")

  k.form.new("main", {title="Orders"})
  k.ctrl.textbox("main", "qty", {label="Qty"})
  k.ctrl.button("main", "ok", {label="OK",
      onclick=function()
        local n = K.tonum(k.ctrl.get_value("qty"))
        k.msgbox(string.format("Qty x2 = %d", n * 2))
        k.print("done")                       -- stdout / log sink
        k.quit()
      end})
  k.form.show("main")                         -- suspends until closed
end
```

- `main()` is required (D6). Runner loads+compiles the script (syntax errors abort), sets up
  sandbox/bindings, then runs `main()` inside the session actor when the first tab connects.
- Errors: every event dispatch wraps the handler in `pcall`. Uncaught Lua error or Go
  binding error → red banner in the page (§3.4 `error`) + structured log entry; the session
  keeps running unless the error occurred during startup. `k.error(msg)` raises deliberately.
- Exit codes: `0` ok · `1` script/runtime error · `2` usage error · `3` I/O error.

## 5. List of supported actions/functions from Kalipso

Legend: **T1** = MVP · **T2** = second milestone · **—** = out of scope (reason).
"Native" means the construct becomes plain Lua when authoring directly.

### 5.1 Group Code (flow, JSON, encryption)

| Kalipso action/subgroup | KALUA | Tier |
|---|---|---|
| If / Else / Else If / End If | native `if/elseif/else` | Native |
| While … End While | native `while` | Native |
| For Each … End For Each | native `for … in` (+ `k.rows(res)` DB iterator) | Native |
| Break / Continue | native | Native |
| Cancel | `return` from handler | Native |
| Cancel All | `k.quit()` | T1 |
| Sleep | `k.sleep(ms)` (yields coroutine) | T1 |
| On Error … End On Error | `pcall` / `xpcall` conventions (§4) | Native |
| Comment / Separator | `--` comments | Native |
| Debug / Breakpoint / Set Trace State | `--verbose` trace flag + log | T2 |
| Threads: Critical Section ×3, Thread Priority, Wait For Threads | — (single-threaded host) | — |
| JSON Get Value / Array Item / Array Item Count / Name List | `k.json_get`, `k.json_array_item`, `k.json_count`, `k.json_names` (whole-document file IO in §5.10) | T1 |
| JSON Import/Export to/from Table | `k.json_to_rows`, `k.rows_to_json` (result-set based) | T2 |
| Encrypt / Decrypt (key-based) | `k.encrypt`, `k.decrypt` (AES-GCM) | T1 |
| CheckSum (CRC32, MD5, SHA256, HMAC-SHA256, PBKDF2) | `k.checksum(alg, data)` | T1 |
| Encrypt/Decrypt Symmetric | `k.crypt_symmetric(alg,key,data,[iv])` | T2 |
| Encrypt/Decrypt Asymmetric, Sign/Verify Message | RSA helpers `k.crypt_asymmetric`, `k.sign`, `k.verify` | T2 |
| KeyStore Encrypt/Decrypt | — (device keystore) | — |
| AppCenter ×4 | — (SaaS telemetry) | — |

### 5.2 Group Others

| Kalipso action | KALUA | Tier |
|---|---|---|
| Set Value | plain assignment; typed variant `k.assign(target,"Numeric",v)` | T1 |
| Message Box | `k.msgbox(text[, kind])` — kind: info/warn/error/ok-cancel/yes-no (returns choice via browser modal, §3.4) | T1 |
| Print *(new, KALUA-specific)* | `k.print(...)` → stdout/log sink | T1 |
| Exec Global Action Set / Invoke From Local Action Set / Exec Local Action Set | functions + `k.set(name,fn)`, `k.exec(name,...)` | T1 |
| Return Values | native `return` | Native |
| Close Project | `k.quit()` | T1 |
| Open Project / Make Backup / Restore Backup / Restart Kalipso | — (single-file apps) | — |
| Get/Set Project Param | `k.param_get(key)`, `k.param_set(key,v)` (persisted to app-side file) | T2 |
| Copy to Clipboard / Get Clipboard | `k.clipboard_set/get` (browser `navigator.clipboard`) | T1 |
| Play Sound | `k.bell()` (WebAudio beep) | T1 |
| Notification Message / Vibrate / Set Blinking / Post App Notification | — (mobile OS features) | — |
| Show Popup / Exec JScript / Process/Create EAN128 | — (no equivalent) | — |
| Get Screen Dimensions | `k.screen_size()` (browser viewport from `client_info`) | T1 |
| Check Internet Connection | `k.net_ok(timeout_ms)` | T2 |
| Get Locale Information / Language get/set | `k.locale()` stub returning browser locale | T2 |
| Run/Kill Process, Shell Execute, Registry, Serial Number, Battery/Memory, Terminal ID, Wireless, Camera/OCR/Barcode Recognize, Permissions, Power, Wake Lock, SIP, Display Orientation, Network Adapters, Wifi, Certificate Fingerprint, Rooted check, Send Keys/Mouse, Capture Keys, Scroll Position, Cursor Pos, Keyboard, DLL ×3, JAR/APK ×3, GPIO ×7, In-App Purchase ×7, MSS ×4, Printing ×5, Services ×3 | — (hardware/desktop/mobile-only, D9) | — |

### 5.3 Group Database

| Kalipso action | KALUA | Tier |
|---|---|---|
| *(new — no Kalipso equivalent)* | `k.connect_db(dsn)`, `k.disconnect_db()` — drivers: MariaDB/MySQL, Postgres, MSSQL | T1 |
| Select | `k.db_select(table, fields, where, order)` → result set `{columns, rows}`; iterate with `k.rows()` | T1 |
| SQL Advanced | `k.sql(sql, params...)` → result set or affected-count | T1 |
| Insert / Update / Delete | `k.db_insert(table, keyvals)`, `k.db_update(table, keyvals, where)`, `k.db_delete(table, where)` | T1 |
| Kill Table | `k.db_kill_table(table, where)` | T2 |
| Exec. Procedure | `k.db_proc(name, params...)` | T2 |
| Begin/Commit/Rollback Transaction | `k.tx_begin()`, `k.tx_commit()`, `k.tx_rollback()` | T1 |
| Connect/Disconnect SQLite | `k.connect_sqlite(path)`, `k.disconnect_sqlite()` | T2 |
| Data Link ×9, Set Sync Status, DB Profile params, Close All ODBC | — (control-linked records / sync infra) | — |

All DB calls run through the async worker pattern (§2.2) and accept bound parameters
(`?` placeholders) — string interpolation into SQL is discouraged but not forbidden.

### 5.4 Group Communications

| Kalipso action | KALUA | Tier |
|---|---|---|
| HTTP Request | `k.http_request{method,url,headers,body,timeout}` → `{status,headers,body}` | T1 |
| Web Service Run | `k.webservice_run(profile, params)` — SOAP client | T2 |
| XML Get Root Element / Child Element / Child Element List / Element Attribute / Element Content / Attribute List / Element's Name | `k.xml_parse(s)` → doc handle; `k.xml_root/h child/attr/content/attrs/name` | T1 |
| XML Import/Export to/from Table | `k.xml_to_rows`, `k.rows_to_xml` (document file IO in §5.10) | T2 |
| FTP Connect/Set Current Dir/Get File/Put File/File Exists/Disconnect/Create Dir/Delete File-Folder/Rename File/List Files | `k.ftp_*` (10 functions, same verbs) | T2 |
| Socket Connect/Write/Read/Close/Accept | `k.socket_*` | T2 |
| Ping | `k.ping(host, timeout_ms)` | T2 |
| *(KALUA server mode — §2.4)* | `k.shared_set(key, val)`, `k.shared_get(key)`, `k.shared_del(key)`, `k.shared_keys([pattern])`, `k.shared_incr(key, delta)` | T1 |
| *(KALUA server mode — §2.4)* | `k.ws_broadcast(msg)`, `k.ws_send(client_id, msg)` | T1 |
| *(KALUA server mode — §2.4)* | `k.tcp_send(client_id, data)`, `k.tcp_close(client_id)` | T1 |
| Monitor ×2, Synchronization ×6, MIS Communicator ×12, Push ×5, GSM ×4, GPRS ×3, Serial ×5, Bluetooth ×7, BLE ×11, Beacons ×6 | — (D9 hardware/remote infra) | — |

### 5.5 Group Files

| Kalipso action | KALUA | Tier |
|---|---|---|
| File Open/Read/Write/Close | `k.file_open(path,mode)` → handle; `k.file_read/write(handle,…)`; `k.file_close(h)` | T1 |
| File Load Content / Save Content | `k.file_load(path)`, `k.file_save(path,data)` | T1 |
| File/Folder Copy / Move / Delete / Exists / Create Dir / List Files / Get File Information | `k.file_copy/move/delete/exists/mkdir/list/info` | T1 |
| INI Read / Write | `k.ini_read(path,section,key)`, `k.ini_write(...)` — whole-file API in §5.10 | T2 |
| ZIP Add/Extract/List | `k.zip_add`, `k.zip_extract`, `k.zip_list` | T2 |
| File Import/Export to/from Table | CSV ↔ result-set: `k.csv_to_rows`, `k.rows_to_csv` (file-level API in §5.10) | T2 |
| Select File / Share File | `k.pick_file(open|save, path_hint)` (browser file input for open; save = download) / — | T2 / — |
| Image ×5 | — (D8 visual) | — |

Filesystem access is confined to the working directory unless `--allow-fs PATH` is given.

### 5.6 Group Date/Time

| Kalipso action | KALUA | Tier |
|---|---|---|
| Start Timer / Stop Timer | `k.timer_start(id, ms [,repeats])`, `k.timer_stop(id)` → form `timer(id)` event | T1 |
| Set System Date / Time, Get Server Date/Time | — (host OS / MIS infra) | — |

### 5.7 Group Email

| Kalipso action | KALUA | Tier |
|---|---|---|
| SMTP Connect / Send E-Mail / Disconnect | `k.smtp_connect{host,port,user,pw,tls}`, `k.smtp_send{from,to,subject,body,attachments}`, `k.smtp_disconnect()` | T2 |
| POP3 ×7 + Load E-Mail From File | `k.pop3_*` (same verbs) | T2 |
| Send E-mail (OS client), Pocket Outlook ×14 | — (OS integration) | — |

### 5.8 Group Forms

| Kalipso action | KALUA | Tier |
|---|---|---|
| Show Form | `k.form.show(name)` (stacked/modal semantics, §2.2) | T1 |
| Close Form | `k.form.close([name])` | T1 |
| Return to Form | `k.form.return_to(name)` | T2 |
| Clear Form / Refresh | `k.form.clear(name)`, `k.form.refresh(name)` | T1 |
| First/Last/Next/Previous/Go To/Clear/Refresh Plane | — (single-plane, D7) | — |
| Present Form, Set Foreground Window, Force Redraw | — (mobile/windowing) | — |
| Show/Close Status Window | `k.status_show(text)`, `k.status_close()` (busy/status bar, §3.4) | T2 |
| Set/Refresh Form Table Filter | — (Data Link dependent) | — |

### 5.9 Expression functions (globals)

Exposed as flat globals (not under `k.`) so expressions read like Kalipso. Types: strings
are `YYYY-MM-DD[ HH:MM[:SS]]`.

**String:** `left right middle length replace trim upper lower find string_count complete
ascii charact base64_encode base64_decode urlencode urldecode jsonencode jsondecode
xmlencode xmldecode guid extract_string file_extract_part full_encode decode encode set_string mltext`

**Numeric:** `abs round floor ceiling power nth_root sqrt exp log log10 sin cos tan
asin acos atan deg2rad rad2deg bitwise_and bitwise_or bitwise_xor random int_part dec_part
mask_number val sum extractstringd`

**Conditional:** `lookup(key, pairs…)`, `yesno(cond, a, b)`, `iif(cond,a,b)`

**Date/Time:** `sys_date sys_time day month year hour minute second add_days subtract_days
date_diff datetime_add datetime_sub datetime_diff date_to_string time_to_string week_day
week_number tick_count julian utc_to_local local_to_utc`

**Conversion:** `tostr tonum todate strtodate boolstr`

**Operators / keywords:** `=`→`==` · `<>`→`~=` · `AND/OR/NOT`→`and/or/not` · string concat `&`→`..` ·
`NULL`→`nil` · `CTRL(name)`→accessor function.
Mixed-type comparisons route through `K.eq/ne/add` (§2.3).

### 5.10 Data formats — read/parse & write (KALUA-specific, D14)

No direct Kalipso equivalent; designed for KALUA. Every format follows one convention:

- `k.<fmt>_load(path)` — read file, parse → Lua table
- `k.<fmt>_save(path, data)` — serialize Lua table → write file
- `k.<fmt>_parse(str)` / `k.<fmt>_string(data)` — in-memory variants

| Format | Functions | Value-mapping notes | Tier |
|---|---|---|---|
| JSON | `json_load`, `json_save`, `json_parse`, `json_string` | object↔table (string keys), array↔sequence, `null`↔`K.NULL` sentinel (`K.is_null(v)`) | T1 |
| CSV | `csv_load`, `csv_save` with opts `{header=bool, sep, quote}` | `header=true` → list of row-maps; else list of arrays; all values strings unless coerced | T2 |
| INI | `ini_load`, `ini_save`; single-key `ini_read/ini_write` (§5.5, Kalipso parity) | `{section={key=value}}`, no-section keys under `_root`; values strings | T2 |
| YAML | `yaml_load`, `yaml_save`, `yaml_parse`, `yaml_string` | same mapping as JSON; multi-document files → list of tables | T2 |
| XML | `xml_load`, `xml_save` (alongside §5.4 `xml_parse` + getter family) | element→table: `{_name, _attrs={…}, _children={…}, _text}`; one convention shared by all `xml_*` bindings, documented once in `formats.go` | T2 |

Rules for all format actions:

- Loads/saves run through the async worker pattern (§2.2); a size cap (default 16 MB,
  `--max-file-size`) guards against runaway reads.
- UTF-8 in/out; BOM tolerated on load; save never emits BOM.
- Save is atomic: write to temp file + rename.
- Round-trip guarantee applies only within one format (JSON→JSON, YAML→YAML …); cross-format
  conversion is the author composing `load` + `save`.

### 5.11 Out-of-scope groups (whole)

RFID · Voice · GPS · Barcode (scanning) — D9. Their events (Barcode Scanned, Scanner Trigger,
NFC, Sensors, RFID Tag Found…) have no KALUA counterpart.

## 6. CLI specification

```
KALUA run    <app.lua> [--port 8080|0] [--no-browser] [--session-limit N] [--verbose] [--db NAME=DSN]... [--arg K=V]...
KALUA serve  <app.lua> [--port 8080] [--workers N] [--mode http|ws|tcp] [--verbose] [--db NAME=DSN]... [--arg K=V]...
KALUA check  <app.lua>                  # reports syntax/global misuse
KALUA version
```

- `run` serves the app as a web app and opens the default browser. `--port 0` (default)
  picks a free ephemeral port; `--port N` binds a fixed port; `--no-browser` suppresses the
  auto-open (also used by tests). `--session-limit` caps concurrent tabs (default 8);
  extra tabs get a friendly "app already running in this browser" page.
- `serve` starts a headless HTTP/WebSocket/TCP API server (`handle_http`/`handle_ws`/
  `handle_tcp` entry points); `SIGTERM`/`SIGINT` graceful shutdown; `SIGHUP` hot reload.
  Forms/UI bindings are errors here (D18).
- `check` catches syntax errors and unknown `k.*` references at load time.
- if —verbose, full log to terminal (functions, arguments, variables values etc.). Otherwise just error messages.
- `--db` pre-registers named connections usable by `k.connect_db("#NAME")`. Alternatively connections can be stored in .ENV file in KALUA folder.
- `--arg` seeds `ARGS` table for headless/scriptable use.
- all flags will have also short variant (-r, -c, -d, -a; run: -p for port, -n for no-browser, -l for session-limit)

## 7. Testing strategy

- **Binding unit tests**: drive the VM headlessly (no browser); bindings that touch UI push
  commands onto a session outbox that assertions inspect instead of an HTTP client.
- **Web/integration tests**: `net/http/httptest` server + the WS client of choice
  (`websocket.NetConn`/dial helper): scripted fixture apps asserting the *sequence* of
  outbox messages (e.g. two-form demo → `render_form` then `close_form` then
  `render_form`, proving suspend/resume over the wire).
- **HTML goldens**: rendered control/form fragments pinned in `testdata/` (golden files);
  templ output is stable and deterministic.
- **Coercion tests**: every documented Kalipso example (`0=""` true etc.) as table-driven
  tests against `internal/coerce`.
- **Async tests**: fake slow DB driver verifying UI stays responsive during queries —
  timers and msgbox still delivered while a query is pending.
- **JS client**: exercised only through the integration tests (no framework);
  a lint pass over `assets/app.js` in CI (syntax only).
- Gate: `go generate ./...` (templ) + `go vet` + `go test ./...` green on every change.

## 8. Build-out phases

1. **Host skeleton** — CLI, sandboxed VM, loader, `k.print/sleep/quit/error`, error handling, exit codes.
2. **Web core** — `net/http` server, WS session bridge, templ shell + form renderer,
   textbox + button, two-form demo proving suspend/resume over WS; session actor + inbox/outbox (§2.2).
3. **Controls full set** — remaining 8 controls, events, properties, focus/key tracking,
   `CTRL()` accessor; vanilla JS client (`app.js`) event delegation, modal, status bar, clipboard/bell/screen-size.
4. **Database group (T1)** — connect_db + select/sql/insert/update/delete/transactions over
   the async pattern; testcontainers-style DB tests.
5. **Data & comms groups (T1)** — files, JSON/XML getters + `k.json_load/save` (§5.10),
   HTTP request, checksum/encrypt, date/time functions, timers.
6. **Expression-function library** — §5.9 globals + operator docs + coercion golden tests.
7. **CLI polish** — `new` templates, `check` diagnostics, `--db/--arg/--allow-fs`,
   `--no-browser/--session-limit`, packaging (single static binary, embedded assets, cgo-free drivers where possible).
8. **Server mode (T1)** — `serve` command, worker pool, HTTP listener, `k.shared_*`,
   `init`/`handle_http`/`shutdown` lifecycle, graceful shutdown, hot reload (SIGHUP).
9. **Tier 2 wave** — FTP/sockets/ping/web-service, SMTP/POP3, SQLite,
   asymmetric crypto, clipboard/status-window/file-picker, remaining §5.10 formats
   (XML save, YAML, CSV, INI).
10. **Server mode (T2)** — WebSocket listener (`handle_ws`, `k.ws_*`), TCP listener (`handle_tcp`, `k.tcp_*`).

## 9. Risks / mitigations

| Risk | Mitigation |
|------|------------|
| LState not goroutine-safe | Session actor owns the LState (phase 2, foundational); all bindings audited against it |
| templ codegen step in build | `//go:generate templ generate` + CI gate; generated files committed |
| Two non-stdlib Go deps (gopher-lua, coder/websocket) | Isolated behind `internal/vm` + `internal/web`; thin seams to swap |
| Slow/closed browser stalls outbox flush | Bounded write buffer + context; WS close cancels session (timers/workers) and recycles LState |
| Tab reload leaves zombie session | Session ctx tied to WS lifecycle; async completions to dead sessions are no-ops; `--session-limit` |
| XSS / markup injection via user strings | templ auto-escapes; control values are data; `templ.Raw` banned for user content |
| Multiple concurrent sessions contend for DB/files | Per-session budgets; shared single DB pool; sessions isolated logically |
| Runtime-only errors (no compiler) | `KALUA check` + strict-mode unknown-global detection at load; optional lint later |
| gopher-lua slow-moving maintenance | Interop isolated behind `bindings/` + `coerce/`; VM swap possible |
| `SetContext` timeout quirk (long single ops stall VM) | Blocking work always in worker goroutines; instruction-budget watchdog |
| Lua 5.1 gaps (integers, DST) | Irrelevant to domain; documented |
| MSSQL driver may require cgo | Prefer go-mssqldb (pure Go); verify in phase 4 spike |
| Scope creep toward mobile features | D7–D9 exclusions are permanent; new requests require decision-log entry |
| Server mode: LState memory leak | Periodic full GC; max requests per LState before recycle |
| Server mode: shared state contention | `sync.Map` for reads; sharded locks for writes; document limits |
| Server mode: blocking handler stalls worker | Instruction budget watchdog; document async patterns (`k.sleep`, `k.http_request`) |
| Server mode: script panic crashes worker | `pcall` wrapper; auto-restart worker on panic; structured error logging |

## 10. Status

Spec complete (this document, web-UI revision 2026-08-28). Nothing implemented yet.
Next step: Phase 1 skeleton.

---

## 11. Debugging Capabilities — Design Plan

*(Unchanged from the TUI version — the debugger attaches to the Lua VM server-side and is
UI-transport agnostic. Notable interactions with the web runtime:*

- *`--verbose` tracing and `--repl-on-error` operate inside the session actor, before
  outbox flush, so frame inspection sees the exact handler state.*
- *Post-mortem dumps on event-handler errors are sent as part of the `error` WS message
  when `--verbose` is on.*
- *Breakpoints (Tier 2, EmmyLua) pause the session actor; the page shows a "paused"
  indicator via a new WS `pause`/`resume` message pair.*
- *Server-mode worker debugging is unchanged (§11.4 Phase C).*)

### 11.1 What gopher-lua Provides (Built-in)

| Capability | API | Notes |
|------------|-----|-------|
| Stack inspection | `LState.GetStack(level)` | Get call frame at level |
| Frame info | `LState.GetInfo(what, dbg, fn)` | Source, line, function name, upvalues, locals count |
| Local variables | `LState.GetLocal(dbg, n)` / `SetLocal(dbg, n, val)` | Read/write locals in a frame |
| Upvalues | `LState.GetUpvalue(fn, n)` / `SetUpvalue(fn, n, val)` | Closure upvalues |
| Local names | `LFunction.LocalName(regno, pc)` | Variable names at instruction pointer |
| Debug library | `OpenDebug(L)` | Exposes `debug.*` to Lua (traceback, getinfo, getlocal, setupvalue, etc.) |

**Critical gap**: gopher-lua **does not implement `debug.sethook()` / `debug.hook()`** — no line/Call/Return hooks for breakpoints or stepping.

### 11.2 Existing Solution: gopherlua-debugger (EmmyLua Protocol)

- Implements debugger via **patched gopher-lua fork** that adds `debug.hook()`
- Uses **EmmyLua protocol** (IDE as server, Lua as client via TCP)
- Works with VS Code / IntelliJ via EmmyLua plugin
- Requires `replace github.com/yuin/gopher-lua => github.com/edolphin-ydf/gopher-lua` in go.mod

### 11.3 Recommended Debugging Architecture for KALUA

#### Tier 1: Built-in (No External Deps) — "Always Available"

| Feature | Implementation | CLI Flag |
|---------|----------------|----------|
| Execution tracing | `--verbose`: logs function calls, args, variable assignments | `--verbose` |
| Post-mortem inspection | On error: dump call stack, locals, upvalues via `GetStack`/`GetLocal`/`GetUpvalue` | Automatic on error |
| REPL at crash | Drop into interactive Lua prompt in error handler with full frame access | `--repl-on-error` |
| Scriptable watchpoints | Lua-side `debug.getinfo`/`getlocal` wrappers exposed via `k.debug.*` | `k.debug.trace()` |

#### Tier 2: Breakpoints & Stepping — Requires Patched VM

| Feature | Approach |
|---------|----------|
| Line breakpoints | Use gopherlua-debugger's patched VM + EmmyLua protocol |
| Conditional breakpoints | Same; evaluate condition in hook callback |
| Step over/into/out | Hook returns control to debugger on line/call/return events |
| Variable watch | Evaluate expressions in current frame context |

**Integration**: Add `--debug` flag that:
1. Uses patched gopher-lua (vendored fork)
2. Starts EmmyLua TCP server on port (default 9966)
3. Auto-injects `require('emmy_core').tcpConnect('localhost', 9966)` at script start
4. Works with VS Code (EmmyLua extension) / GoLand / IntelliJ

#### Tier 3: KALUA-Specific Enhancements

| Feature | Description |
|---------|-------------|
| Form/Control inspection | `k.debug.get_form_state(name)` → widget values, focus, visibility |
| DB query tracing | Log SQL + params + timing via `k.sql`/`k.db_*` wrappers |
| Coroutine visualization | Track suspended/resumed coroutines, show await points |
| Server-mode request tracing | Per-request call trees, shared state diffs, worker assignment |
| Hot-reload debugging | Preserve breakpoints across `SIGHUP` script reload (serve mode) |

### 11.4 Implementation Phases — UPDATED PER DECISIONS

#### Phase A: Core Infrastructure (Week 1-2) — TIER 1 FOCUS
1. **Vendored patched gopher-lua** — fork from `github.com/edolphin-ydf/gopher-lua` with `debug.hook()` + bug fixes
2. **Enhanced tracing (Tier 1)** — extend `--verbose`: function entry/exit with args/returns, variable assignments, control flow, k.* API calls
3. **Post-mortem dump** — on error/panic: full call stack, locals, upvalues, globals
4. **REPL on error** — `--repl-on-error` drops into interactive prompt at crash site
5. **k.debug API** — `k.debug.trace()`, `k.debug.locals()`, `k.debug.stack()`
6. **CLI flags** — `--debug`, `--debug-port`, `--repl-on-error`, `--verbose` (enhanced)

#### Phase B: EmmyLua Integration (Week 2-3) — TIER 2
1. **EmmyLua transport** — TCP server, protocol handler (adapt gopherlua-debugger)
2. **Auto-inject connection** — `require('emmy_core').tcpConnect('localhost', port)` at script start
3. **Breakpoint support** — line/conditional breakpoints via hook; and the actor `pause`/`resume` WS messages
4. **Step controls** — step over/into/out, continue, pause
5. **Variable watch** — evaluate expressions in current frame
6. **Breakpoint persistence** — `.kalua/breakpoints.json` per project

#### Phase C: KALUA Domain Features (Week 3-4) — TIER 3
1. **Custom formatters** — pretty-print `k.form`, `k.ctrl`, `k.db` handles in debugger UI
2. **Session/actor awareness** — show "waiting on form.show()", "timer #3 pending", "DB query in flight"
3. **Server-mode debug worker** — `--debug-worker=1` designates single worker for debugging
4. **Hot-reload breakpoint preserve** — survive `SIGHUP` script reload (serve mode)
5. **Server request tracing** — per-request call trees, shared state diffs

### 11.5 Key Tradeoffs — DECIDED

| Decision | Options | **Decision** |
|----------|---------|--------------|
| Protocol | EmmyLua (existing) vs DAP (standard) | **EmmyLua** — working code exists; migrate to DAP later |
| Patched VM | Vendor fork vs upstream PR | **Vendor fork** for v1; upstream `debug.hook` if maintainable |
| Sandboxing | Allow `debug` library in sandbox? | **Only under `--debug`** — keep production sandbox clean |
| Server mode | Debug all workers or single? | **Single "debug worker"** + `--debug-worker=1` flag |
| Priority | Interactive breakpoints vs enhanced tracing | **Enhanced tracing (Tier 1+)** first; breakpoints as follow-up |

### 11.6 Minimal MVP (Week 1) — TIER 1 ONLY

```go
// internal/vm/debug.go
func NewDebugState(opts DebugOptions) (*lua.LState, *DebugSession) {
    // Use patched gopher-lua (vendored fork with debug.hook)
    L := lua.NewState(lua.Options{
        SkipOpenLibs: true,
        // ... KALUA sandbox options
    })
    lua.OpenDebug(L)  // Enable debug.* library (only in --debug mode)

    if opts.Port > 0 {
        // Start EmmyLua TCP listener (Phase B)
        session := StartEmmyLuaServer(L, opts.Port)
        L.DoString(`require('emmy_core').tcpConnect('localhost', ` + port + `)`)
        return L, session
    }

    // Phase A: Enhanced tracing only
    return L, nil
}
```

CLI: `KALUA run app.lua --verbose --repl-on-error` (Phase A)
       `KALUA run app.lua --debug --debug-port=9966` (Phase B)
       `KALUA serve app.lua --debug --debug-worker=1 --debug-port=9966` (Phase B + Server)

### 11.7 Updated CLI Spec (Section 6 Addition)

```
KALUA run    <app.lua> [--port 8080|0] [--no-browser] [--session-limit N] [--verbose] [--repl-on-error] [--debug] [--debug-port 9966] [--db NAME=DSN]... [--arg K=V]...
KALUA serve  <app.lua> [--port 8080] [--workers N] [--mode http|ws|tcp] [--debug] [--debug-worker 1] [--debug-port 9966] [--verbose] [--db NAME=DSN]... [--arg K=V]...
KALUA check  <app.lua>                  # reports syntax/global misuse
KALUA version
```

- `--verbose`: enhanced tracing (function calls, args, returns, variable assignments, control flow, k.* API calls)
- `--repl-on-error`: drop into interactive Lua REPL at crash site with full frame access
- `--debug`: enable EmmyLua debugger (requires patched VM)
- `--debug-port`: TCP port for debugger (default 9966)
- `--debug-worker`: which worker to debug in server mode (1-based, default 1)

(End of file)