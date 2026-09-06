# KALUA User Guide

**KALUA** — "KAlipso in LUA" — a sandboxed Lua runtime for Kalipso-style web apps. One binary, any `.lua` app.

## Contents

1. [What is KALUA](#1-what-is-kalua)
2. [Quick Start](#2-quick-start)
3. [KALUA CLI](#3-kalua-cli)
4. [Lua Basics](#4-lua-basics)
5. [KALUA Functions — `k.*` Syntax & Usage](#5-kalua-functions--k-syntax--usage)
6. [Examples](#6-examples)
7. [KALUA Builder](#7-kalua-builder)

---

# 1. What is KALUA

KALUA is a Go runtime that embeds a sandboxed **gopher-lua** virtual machine to run Kalipso-style `.lua` apps as **web applications**. A single generic binary interprets any Lua script — there is no compilation step and no per-app runtime.

The `k.*` API is inspired by the **Sysdev Mobile Kalipso** low-code platform (`k.form`, `k.ctrl`, data formats, DB access, file & communication functions), so developers familiar with Kalipso can mostly port their logic directly. KALUA adds Kalipso-compatible *expression functions* (`left`, `round`, `sys_date`, `lookup`, …) and value semantics (`K.eq`, `K.add`, `K.truthy`) on top of plain Lua.

## 1.1 Two run modes

| Mode | Command | What it does |
|------|---------|--------------|
| **Interactive web app** | `KALUA run app.lua` | Serves the app at `http://127.0.0.1:9000` and opens the browser. The Lua script builds **forms**, controls and event handlers; each browser tab gets its own Lua session. |
| **Headless API server** | `KALUA serve app.lua` | Exposes `handle_http`, `handle_ws` and/or `handle_tcp` Lua callbacks as an HTTP/WebSocket/TCP server with a worker pool and shared state (`k.shared.*`). No UI bindings. |

## 1.2 Architecture

```
myapp.lua ──► KALUA run myapp.lua
              │
        ┌─────┴─────────────────────┐
        │ Go host (net/http + WebSocket) │
        │   per tab → Session actor (1 LState) │
        │   inbox = typed events (WS, timers)   │
        │   outbox = UI commands → templ → WS  │
        └──────────────────────────────────────┘
```

- The Lua script must define a `function main()` — the entry point.
- In `run` mode every browser tab owns a single Lua state. UI events arrive on the session inbox; UI commands go out over WebSocket.
- In `serve` mode a pool of Lua workers shares thread-safe state through `k.shared.*`.

## 1.3 Sandbox

Scripts run in a locked-down VM: only a whitelist of standard Lua libraries is opened, and heavy globals (`require`, `loadfile`, `dofile`, `os.execute`, `io`) do not exist. Everything a script can do goes through `k.*` (files, DB, network, forms) or the `K.*` helpers. See [Lua Basics](#4-lua-basics).

## 1.4 What ships in the box

- **Run mode**: form system, 11 control types, charts (Chart.js), msgbox, clipboard, XML/JSON, async HTTP.
- **Serve mode**: HTTP/WS/TCP servers, worker pool, hot reload (SIGHUP), lifecycle hooks (`init`/`shutdown`).
- **Expression functions**: ~100 Kalipso-style globals (string, numeric, date/time, conditional).
- **Data & integration**: JSON, XML, CSV, INI, YAML, result-set conversions, SQLite/MySQL/Postgres/SQL Server, FTP, SMTP, POP3, SOAP, sockets, AES/RSA crypto, ZIP.
- **Tooling**: `KALUA check` static validation, `KALUA builder` visual form editor, `KALUA lsp` Language Server (+ VSCode extension).

---

# 2. Quick Start

Prerequisites: **Go 1.26.3+**. No runtime install; the KALUA binary embeds everything.

## 2.1 Build

```bash
go build -o KALUA ./cmd/KALUA
```

## 2.2 Scaffold your first app

```bash
./KALUA new myapp
```

This writes `myapp.lua`:

```lua
-- myapp.lua
function main()
  k.print("hello from myapp")
end
```

## 2.3 Run it

```bash
./KALUA run myapp.lua
```

The default browser opens at `http://127.0.0.1:9000`. `k.print` output appears in the terminal where KALUA runs. `main()` returns when the app closes — `k.sleep(ms)` and `k.quit()` control the flow:

```lua
function main()
  k.print("hello")
  k.sleep(2000)   -- keep the app alive 2 s
  k.quit()        -- then terminate cleanly
end
```

## 2.4 Validate

```bash
./KALUA check myapp.lua      # syntax + unknown k.* + missing main()
```

## 2.5 Serve as an API

```bash
./KALUA serve myapp.lua --port 8080 --workers 4 --mode http,ws
```

## 2.6 Visual form editor

```bash
./KALUA builder myapp.lua
```

Opens the [KALUA Builder](#7-kalua-builder) at `http://127.0.0.1:9001`.

## 2.7 Editor support (LSP / VSCode)

```bash
./KALUA lsp                    # Language Server over stdio
```

The bundled VSCode extension (`extensions/vscode-kalua`) wires completion, hover and diagnostics into the editor.

---

# 3. KALUA CLI

```
Usage: KALUA <command> [args...]

Commands:
  run     <app.lua> [flags]   Run app as web app (opens browser)
  serve   <app.lua> [flags]   Run app as headless API server
  check   <app.lua>           Validate script (syntax, unknown k.*, main)
  builder <app.lua|form.json> Visual form builder (opens browser)
  new     <name>              Scaffold a minimal app.lua
  lsp                         Language server over stdio
  version                     Print version
```

Flags may be placed before or after the script argument.

## 3.1 `run` — interactive web app

```
KALUA run <app.lua> [--port 9000] [--no-browser] [--session-limit 8] [--test]
            [--verbose] [--repl-on-error] [--debug] [--db NAME=DSN]...
            [--arg K=V]... [--allow-fs PATH]...
```

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--port N` | `-p` | `9000` | HTTP port; `0` picks a free ephemeral port |
| `--no-browser` | `-n` | off | Do not auto-open the browser |
| `--session-limit N` | `-l` | `8` | Max concurrent browser tabs (sessions) |
| `--test` | | off | Headless test mode: no HTTP server, run `main()` once |
| `--verbose` | `-v` | off | Trace all `k.*` calls (args + returns) + full stack on error |
| `--repl-on-error` | | off | Drop into an interactive Lua REPL at the crash site (`--test`) |
| `--debug` | | off | EmmyLua debugger (Tier 2 stub, not yet implemented) |
| `--db NAME=DSN` | `-d` | | Pre-register a named connection, usable as `k.connect_db("#NAME")` |
| `--arg K=V` | `-a` | | Seed the `ARGS` global table |
| `--allow-fs PATH` | `-f` | | Allow script file access outside the working directory (repeatable) |

## 3.2 `serve` — headless API

```
KALUA serve <app.lua> [--host 127.0.0.1] [--port 8080] [--workers 4]
             [--mode http|ws|tcp] [--verbose] [--debug] [--debug-worker]
             [--db NAME=DSN]... [--arg K=V]... [--allow-fs PATH]...
```

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--host H` | | `127.0.0.1` | Bind address |
| `--port N` | `-p` | `8080` | HTTP port |
| `--workers N` | `-w` | `4` | Number of Lua worker VMs |
| `--mode m` | `-m` | `http` | Comma-separated: `http`, `ws`, `tcp`, or any combination |
| `--verbose` | `-v` | off | Verbose k.* tracing |
| `--debug` / `--debug-worker` | | off | EmmyLua debugger stubs (not yet implemented) |
| `--db`, `--arg`, `--allow-fs` | `-d -a -f` | | Same as `run` |

Signals:
- `SIGTERM` / `SIGINT` — graceful shutdown (runs optional `shutdown()` hook once).
- `SIGHUP` — **hot reload**: recompiles the script and atomically swaps the worker pool; in-flight requests and open connections finish on the old workers.

## 3.3 `check` — static validation

```bash
KALUA check <app.lua> [-v]
```

Reports syntax errors, unknown `k.*` references and a missing `main()`. Does not execute anything.

## 3.4 `new`, `lsp`, `version`

```bash
KALUA new <name>     # write a minimal runnable <name>.lua
KALUA lsp            # Language Server over stdio (LSP frames), UTF-8 positions
KALUA version
```

## 3.5 Exit codes

| Code | Meaning |
|------|---------|
| 0 | OK |
| 1 | Script / runtime error |
| 2 | CLI usage error |
| 3 | File I/O error (not found, permission) |

## 3.6 Realistic invocations

```bash
./KALUA run myapp.lua --port 8080
./KALUA run app.lua -n -v --arg env=dev --db main=sqlite://app.db
./KALUA serve api.lua --mode http,ws,tcp --workers 8 -p 9090
./KALUA check myapp.lua
./KALUA builder forms/app.lua --no-browser
```

---

# 4. Lua Basics

KALUA scripts are plain **Lua 5.1**-style programs (gopher-lua). If you know Lua you already know KALUA's syntax; the new vocabulary is the `k.*` API.

## 4.1 The sandboxed standard library

Only the following standard globals/libraries are available:

| Library / globals | Notes |
|-------------------|-------|
| `string`, `table`, `math` | Full libraries (`string.sub`, `table.insert`, `math.floor`, …) |
| `os` | Read-only subset: `os.clock()`, `os.difftime()`, `os.date()`, `os.time()` |
| `debug` | Introspection (`debug.getinfo`, hooks); see `k.debug.*` for script-friendly helpers |
| Base: `pairs`, `ipairs`, `assert`, `error`, `pcall`, `xpcall`, `select`, `tonumber`, `tostring`, `type`, `unpack`, `setmetatable`, `getmetatable`, `next`, `rawequal`, `_G`, `_VERSION` | The safe subset |
| `string.format`, `table.concat`, …, `string.gsub` | Everyday workhorses |

**Removed** (attempting to use them is a runtime error): `require`, `print`, `dofile`, `loadfile`, `load`, `loadstring`, `collectgarbage`, `rawget`, `rawset`, `setfenv`, `getfenv`, `module`, `newproxy`, `io`, `os.execute`, `os.exit`, `os.remove`, `os.rename`, `os.getenv`, `os.setenv`, `os.tmpname`, `os.setlocale`.

> Use `k.print(...)` instead of Lua's `print` — output goes to the KALUA host log.

## 4.2 Values and types

```lua
nil      -- absent value
false    -- boolean
true
42       -- number
3.14
"hello"  -- string (bytes, no separate char type)
{1, 2, 3}              -- array table (1-based!)
{name = "Ada", x = 1}  -- map table
function(x) return x * 2 end  -- first-class function
```

Lua tables are **1-based**: `t[1]` is the first element of `{"a", "b"}`.

## 4.3 Kalipso value semantics

KALUA installs `K.*` helpers so code reads like Kalipso and plain-Lua traps (e.g. `"0"` being truthy) disappear:

| Helper | Meaning |
|--------|---------|
| `K.eq(a, b)` / `K.ne(a, b)` | Equality: numeric when both sides coerce, else string compare |
| `K.add(a, b)` | Addition: numeric if both coerce, else concatenation |
| `K.tonum(x)` | Coerce to number (else 0) |
| `K.tostr(x)` | Coerce to the Kalipso string form |
| `K.truthy(x)` | Kalipso condition test — `0`, `"0"`, `""` and `nil` are **false** |
| `K.NULL`, `K.is_null(v)` | JSON `null` sentinel (e.g. from `k.json_parse`) |
| `K.EQ`, `K.NEQ`, `K.ADD` | Operator constants `"="`, `"<>"`, `"+"` |

So the idiomatic condition test is `if K.truthy(x) then` — not `if x then`.

## 4.4 Functions as event handlers

Controls take handler functions directly, or you register them with `k.form.on`:

```lua
k.ctrl.button("main", "btn", { label = "Go",
  onclick = function()
    -- ...
  end })

k.form.on("main", "btn", "onclick", function()
  -- ...
end)
```

## 4.5 Async operations (coroutine suspension)

Some functions suspend the script until the browser/host answers: `k.msgbox`, `k.http_request`, `k.pick_file`, `k.clipboard_get`, `k.ctrl.get_value` round-trips, `k.chart.get_image`, `k.table.get_data`. They look like normal function calls:

```lua
local choice = k.msgbox("Delete row?")     -- suspends until the user clicks
local res    = k.http_request({url = "..."}) -- suspends until HTTP responds
```

While a script is suspended the session keeps servicing events, so timers and other tabs keep working.

## 4.6 Error handling

Raise and catch errors the normal way:

```lua
k.error("download failed")          -- deliberate error
local ok, err = pcall(k.http_request, {url = "..."})
xpcall(function() k.sql(db, "bad sql") end, debug.traceback)
```

Script globals: **`ARGS`** (table from `--arg K=V`), **`CTRL(name)`** (control handle accessor), and **`main`** (required entry point).

---

# 5. KALUA Functions — `k.*` Syntax & Usage

Everything beyond bare Lua lives on the `k` table, the `K` helpers, and the flat expression-function globals. All of it is generated into the reference at the end of this chapter from `internal/bindings/api_doc.go`.

## 5.1 Group overview

| Group | Namespace prefix | Purpose |
|-------|------------------|---------|
| Flow | `k.print`, `k.sleep`, `k.quit`, `k.msgbox`, `k.http_request`, `k.timer_start`, `k.param_set`, `k.net_ok`, `k.locale`, `k.ping`, … | App lifecycle, UI dialogs, time, persistence |
| Forms | `k.form.*` | Declare/show/close forms, register events |
| Controls | `k.ctrl.*` | Add & manipulate the 11 control types |
| Tables / Loopers | `k.table.*`, `k.looper.*` | Row data ops, DB-linked grids and repeaters |
| Charts | `k.chart.*` | Chart.js data updates, PNG export |
| Debug | `k.debug.*` | Stack, locals, trace anchors |
| Database | `k.connect_db`, `k.sql`, `k.db_select/insert/update/delete`, `k.rows`, `k.tx_*` | Any SQL DB via DSN; sqlite helpers |
| Files | `k.file_*`, `k.zip_*` | Read/write files, ZIP archives |
| JSON / XML | `k.json_*`, `k.xml_*` | Parse, navigate, serialize |
| Formats | `k.csv_*`, `k.ini_*`, `k.yaml_*` | Data interchange formats |
| Rows | `k.json_to_rows`, `k.csv_to_rows`, `k.rows_to_json`, … | Result-set ↔ format conversions |
| Comm | `k.socket_*`, `k.ftp_*`, `k.smtp_*`, `k.pop3_*`, `k.webservice_run` | Networking & integrations |
| Crypto | `k.checksum`, `k.encrypt`/`k.decrypt`, `k.crypt_*`, `k.sign`/`k.verify` | Hashing, AES/RSA, signatures |
| Server (serve mode) | `k.shared.*`, `k.ws.*`, `k.tcp.*` | Cross-worker state, WS/TCP push |

## 5.2 Conventions

- **`k.<namespace>.<function>(...)`** — arguments are positional; the example signature in the reference shows the exact shape.
- **Options as tables.** Constructors take an options table: `k.ctrl.textbox("main", "name", {label = "Name", value = ""})`.
- **Return values.** Functions return Lua tables/numbers/strings; on failure most raise a Lua error you can `pcall`. `nil` is returned when "nothing found" (e.g. `k.pick_file` cancel, `k.ping` unreachable).
- **Result sets** come back as `{columns = {...}, rows = {{...}, ...}}` from `k.sql` / `k.db_*`. Iterate with `k.rows(result)`.
- **JSON null** round-trips through the `K.NULL` sentinel; test with `K.is_null(v)`.
- **DSN schemes** for `k.connect_db`: `sqlite://`, `mysql://`, `postgres://`, `sqlserver://`. `--db NAME=DSN` pre-registers a handle you open with `k.connect_db("#NAME")`.
- **Async ops** suspend the coroutine (see [Lua Basics §4.5](#45-async-operations-coroutine-suspension)).

## 5.3 Forms & controls in practice

```lua
function main()
  k.form.new("main", { title = "Hello", layout = "vertical", align = "center" })  -- or layout = "grid", gap, cells

  k.ctrl.label("main", "greeting", { text = "KALUA Forms" })
  k.ctrl.textbox("main", "name", { label = "Your name", value = "World" })

  k.ctrl.button("main", "btn", { label = "Greet",
    onclick = function()
      local who = k.ctrl.get_value("main", "name")
      k.msgbox("Hello " .. who .. "!")
    end })

  k.form.show("main")
end
```

Grid layouts — form-level cells, controls placed into them:

```lua
k.form.new("dash", {
  title = "Dashboard",
  layout = "grid",
  gap = 16,
  cells = {
    { id = "header",  width = 12 },
    { id = "main",    width = 9 },
    { id = "sidebar", width = 3 },
  },
})
k.ctrl.list("dash", "nav", { cell = "sidebar", items = {"Dash", "Reports"} })
k.ctrl.chart("dash", "trend", { cell = "main", type = "line",
  labels = {"Jan", "Feb"}, datasets = {{ label = "Rev", data = {10, 20} }} })
```

## 5.4 Reference

> Auto-generated from `internal/bindings/api_doc.go`. Regenerate with `go run ./cmd/kalua-userguide`.

<!-- KALUA:gen-api -->

### k.* Bindings

#### Flow

**`k.bell()`**  
Plays a system beep sound via WebAudio.

**`k.clipboard_get()`**  
Reads text from the browser clipboard.

**`k.clipboard_set(text)`**  
Writes text to the browser clipboard.

**`k.error(msg)`**  
Raises a deliberate Lua error.

**`k.http_request(optsTable)`**  
Makes an HTTP request. opts: {method, url, headers, body, timeout}. Returns {status, headers, body}.

**`k.locale()`**  
Returns the session locale ("en-US" default).

**`k.msgbox(text[, kind])`**  
Shows a message box; kind defaults to "info". Returns user's choice.

**`k.net_ok(timeout_ms)`**  
Reports internet reachability via a TCP dial.

**`k.param_get(key)`**  
Reads a persisted app param (string; "" if unset).

**`k.param_set(key, value)`**  
Persists an app param (string) to an app-side file.

**`k.pick_file([opts])`**  
Opens a browser file picker dialog. opts (optional table): {accept="image/*,.pdf", multiple=true}. Returns a table of files: {{name, size, type, data}, ...} where data is base64-encoded. Returns nil on cancel.

**`k.ping(host, timeout_ms)`**  
TCP-based latency probe returning ms, or nil when unreachable.

**`k.print(...)`**  
Prints values to the app log (tab-separated, like Lua print).

**`k.quit()`**  
Requests a clean termination of the app.

**`k.screen_size()`**  
Returns viewport dimensions as {width, height}.

**`k.sleep(ms)`**  
Suspends the script for ms milliseconds.

**`k.status_close()`**  
Hides the status bar.

**`k.status_show(text)`**  
Shows a busy/status bar with the given text.

**`k.timer_start(id, ms[, repeats])`**  
Starts a session timer; fires a Lua function named id (repeats optional).

**`k.timer_stop(id)`**  
Stops a running session timer.

#### Debug

**`k.debug`**  
Runtime introspection helpers: stack/locals/trace.

**`k.debug.locals([level])`**  
Returns a table of local name → value for the given frame level (default 1).

**`k.debug.stack()`**  
Returns a table of the current call frames, each with level, name, source, line and locals.

**`k.debug.trace([msg])`**  
Logs a script-side trace anchor when verbose tracing is enabled.

#### Forms

**`k.form.clear(name)`**  
Clears a form's control values.

**`k.form.close([name])`**  
Closes the top form, or the named form.

**`k.form.new(name, optsTable)`**  
Declares a form. opts: {title, layout=vertical|grid, align=left|center|right, gap=n px, cells}. grid cells: {id={width 1-12, bg, border={width,color}, align}} or ordered array of {id,...}; assign controls via control opt cell="id" and override alignment via align (kforms_enhancements §6).

**`k.form.on(form, ctrl, event, fn)`**  
Registers an event handler (e.g. event "onclick") for a control.

**`k.form.refresh(name)`**  
Re-renders and pushes the form to the browser.

**`k.form.return_to(name)`**  
Closes all forms above name.

**`k.form.show(name)`**  
Shows a form (modal) and suspends the script until it closes.

#### Controls

**`k.chart`**  
Chart control operations: k.chart.set_data/add_dataset/...

**`k.chart.add_dataset(form, name, dataset)`**  
Appends a dataset {label, data, backgroundColor?, borderColor?, fill?, tension?, ...} to a chart.

**`k.chart.get_image(form, name)`**  
Renders the chart canvas to a base64 PNG data URL.

**`k.chart.remove_dataset(form, name, index)`**  
Removes a dataset by 1-based index.

**`k.chart.resize(form, name, width, height)`**  
Resizes the chart canvas to the given pixel dimensions.

**`k.chart.set_data(form, name, {labels, datasets})`**  
Bulk replaces a chart's labels and datasets.

**`k.chart.set_labels(form, name, labels)`**  
Replaces the chart's X-axis labels (array).

**`k.chart.set_options(form, name, options)`**  
Merges Chart.js options (scales, plugins, ...) into the chart.

**`k.chart.update_dataset(form, name, index, dataset)`**  
Replaces the dataset at 1-based index.

**`k.ctrl.button(form, name, optsTable)`**  
Adds a button control. opts may set label, class, onclick, enabled.

**`k.ctrl.chart(form, name, optsTable)`**  
Adds a Chart.js control. opts: {type=line|bar|hbar|pie|doughnut|scatter|radar|area, title, width=400, height=300, labels, datasets, options, responsive=true, maintainAspectRatio=false, legend=true, legendPosition=top, animation=true, stacked=false}. Events chart_click/chart_hover/chart_legend_click via k.form.on.

**`k.ctrl.checkbox(form, name, optsTable)`**  
Adds a checkbox control.

**`k.ctrl.combo(form, name, optsTable)`**  
Adds a combo (dropdown) control. opts.items is a table of choices.

**`k.ctrl.get_property(form, name, prop)`**  
Gets an arbitrary control property.

**`k.ctrl.get_value(form, name)`**  
Returns a control's current value.

**`k.ctrl.image(form, name, optsTable)`**  
Adds an image control (<img>). opts: {src (required), alt, width, height (px or %), fit="cover|contain|fill|scale-down|none" (default contain), clickable?, onclick?}. k.ctrl.set_value(form, name, new_src) updates the image (kforms_enhancements.md §4.3).

**`k.ctrl.label(form, name, optsTable)`**  
Adds a label control. opts: {text, multiline?:boolean, cell?, align?}. multiline renders a pre-wrap div preserving \n (kforms_enhancements.md §4.2). cell/align: grid layout assignment + alignment (kforms_enhancements.md §6).

**`k.ctrl.list(form, name, optsTable)`**  
Adds a multi-row select list. opts.items is a table of choices.

**`k.ctrl.looper(form, name, optsTable)`**  
Adds a looper control (repeating row layout). DB-linked when opts carry {db,query,links,page_size?,count_query?,where?,order_by?}.

**`k.ctrl.radio(form, name, optsTable)`**  
Adds a radio button control.

**`k.ctrl.refresh(form, name)`**  
Re-renders a single control and pushes the update.

**`k.ctrl.set_focus(form, name)`**  
Moves focus to a control in the browser.

**`k.ctrl.set_property(form, name, prop, value)`**  
Sets an arbitrary control property.

**`k.ctrl.set_value(form, name, value)`**  
Sets a control's value and re-renders it.

**`k.ctrl.table(form, name, optsTable)`**  
Adds a table control; rows manipulated via k.table.*.

**`k.ctrl.textbox(form, name, optsTable)`**  
Adds a textbox control. opts: {label, value, enabled, visible, multiline?:boolean, rows?:number, cols?:number, datetime?:boolean|table, cell?, align?}. multiline renders a <textarea>. datetime enables a flatpickr picker: mode="date"|"time"|"datetime", format, min, max, step (kforms_enhancements.md §4.1). cell/align: grid layout assignment + alignment (kforms_enhancements.md §6).

**`k.looper`**  
Looper control operations: k.looper.link_db/set_db_source/refresh/...

**`k.looper.add_line(form, name, valuesTable)`**  
Raises a runtime error on DB-linked loopers (rows come from the linked query).

**`k.looper.delete_line(form, name, index)`**  
Raises a runtime error on DB-linked loopers (rows come from the linked query).

**`k.looper.link_db(form, name, opts)`**  
Attaches a DB source to a looper: {db,query,links,page_size?,count_query?,where?,order_by?}. links list maps result columns to template controls: {column=N,control,property} by 1-based index or {field,col,control,property} by name.

**`k.looper.refresh(form, name)`**  
Re-runs a DB-linked looper's query and shows page 1.

**`k.looper.set_db_source(form, name, opts)`**  
Swaps a DB-linked looper's source {db,query,links?,page_size?,count_query?,where?,order_by?} and refreshes.

**`k.looper.set_line(form, name, index, valuesTable)`**  
Raises a runtime error on DB-linked loopers (rows come from the linked query).

**`k.table.add_line(form, name, valuesTable)`**  
Appends a row to a table control.

**`k.table.delete_line(form, name, index)`**  
Removes the row at index.

**`k.table.get_column_value(form, name, row, column)`**  
Gets a cell value from a table control.

**`k.table.get_data(form, name)`**  
Returns all current data of a table control.

**`k.table.get_selected_column(form, name)`**  
Gets the currently selected column.

**`k.table.get_selected_rows(form, name)`**  
Returns the selected row indices (1-based).

**`k.table.refresh(form, name)`**  
Re-runs a DB-linked tabulator table's query and shows page 1.

**`k.table.set_column_value(form, name, row, column, value)`**  
Sets a cell value in a table control.

**`k.table.set_data(form, name, dataTable)`**  
Bulk replaces all row data (Tabulator mode pushes tabulator_update).

**`k.table.set_db_source(form, name, opts)`**  
Swaps a DB-linked tabulator table's source {db,query,columns?,page_size?,count_query?,where?,order_by?} and refreshes.

**`k.table.set_remote_data(form, name, {data,last_page,last_row})`**  
Pushes server-side pagination data to a tabulator table =  {data=rows, last_page=n} or {data=rows, last_row=n}.

**`k.table.set_selected_column(form, name, column)`**  
Sets the selected column.

#### Database

**`k.connect_db(dsn)`**  
Opens a database connection (DSN scheme: sqlite://, mysql://, postgres://, sqlserver://) and returns a handle.

**`k.connect_sqlite(path)`**  
Opens a SQLite database file; returns a handle usable with k.sql/k.db_*.

**`k.db_delete(handle, table, whereTable)`**  
Deletes rows matching the where table.

**`k.db_insert(handle, table, keyvalsTable)`**  
Inserts a row; returns {last_insert_id, rows_affected}.

**`k.db_kill_table(handle, table, where)`**  
Deletes rows matching the where table.

**`k.db_proc(handle, name, ...params)`**  
Executes a stored procedure.

**`k.db_select(handle, table, fieldsTable, whereTable, order)`**  
Query builder returning {columns, rows}.

**`k.db_update(handle, table, keyvalsTable, whereTable)`**  
Updates rows matching the where table.

**`k.disconnect_db([handle])`**  
Closes a connection, or all connections when no handle is given.

**`k.disconnect_sqlite([handle])`**  
Closes a SQLite connection (or all).

**`k.rows(result)`**  
Returns an iterator over a query result's rows.

**`k.sql(handle, query, ...params)`**  
Executes arbitrary SQL; returns rows or {rows_affected}.

**`k.tx_begin(handle)`**  
Starts a transaction on a connection.

**`k.tx_commit(handle)`**  
Commits the active transaction.

**`k.tx_rollback(handle)`**  
Rolls back the active transaction.

#### Files

**`k.file_close(handle)`**  
Closes an open file handle.

**`k.file_copy(src, dst)`**  
Copies a file, preserving permissions.

**`k.file_delete(path)`**  
Deletes a file.

**`k.file_exists(path)`**  
Reports whether a path exists.

**`k.file_info(path)`**  
Returns {name, size, is_dir, modified} for a path.

**`k.file_list(dir)`**  
Lists a directory as a 1-based, sorted table of names.

**`k.file_load(path)`**  
Reads an entire file as a string (async; max 16 MiB).

**`k.file_mkdir(path[, parents])`**  
Creates a directory; parents=true creates intermediate dirs.

**`k.file_move(src, dst)`**  
Moves/renames a file.

**`k.file_open(path[, mode])`**  
Opens a file; mode is r, r+, w, w+, a or a+ (default r). Returns a handle.

**`k.file_read(handle[, count])`**  
Reads the whole file or count bytes; empty string at EOF.

**`k.file_read_line(handle)`**  
Reads one line (trailing newline trimmed); nil at EOF.

**`k.file_save(path, data)`**  
Writes a file atomically (async; temp file + rename).

**`k.file_write(handle, data)`**  
Writes data to an open file.

**`k.zip_add(zipPath, entries)`**  
Writes a zip archive from {name=content} entries.

**`k.zip_extract(zipPath, dir)`**  
Extracts a zip archive into dir; returns file count.

**`k.zip_list(zipPath)`**  
Lists the member names of a zip archive.

#### Json

**`k.is_null(value)`**  
Reports whether value is the K.NULL sentinel.

**`k.json_array_item(root, path, index)`**  
Returns the element at a 0-based array index.

**`k.json_count(root, path)`**  
Returns the element count (array length or object size).

**`k.json_get(root, path)`**  
Walks a dot/bracket path over a parsed value, e.g. "a.b[0].c".

**`k.json_load(path)`**  
Reads and parses a JSON file (async; max 16 MiB).

**`k.json_names(root, path)`**  
Returns a 1-based table of keys or indices.

**`k.json_parse(text)`**  
Parses JSON text; null maps to K.NULL.

**`k.json_save(path, value)`**  
Encodes a value and writes it atomically (async).

**`k.json_string(value)`**  
Encodes a value as compact JSON (sorted keys).

#### Crypto

**`k.checksum(alg, data[, key[, salt[, iterations[, keylen]]]])`**  
Hex hash for alg: crc32, md5, sha1, sha256, hmac-sha256 (requires key), pbkdf2 (requires salt).

**`k.crypt_asymmetric(alg, key, data)`**  
RSA PKCS#1 v1.5 encrypt/decrypt with a PEM key.

**`k.crypt_symmetric(alg, key, data[, iv])`**  
AES-CBC symmetric encrypt/decrypt (alg aes-encrypt/aes-decrypt). Returns base64.

**`k.decrypt(b64, key)`**  
Reverse of k.encrypt.

**`k.encrypt(plaintext, key)`**  
AES-GCM encryption; returns base64(nonce || ciphertext).

**`k.sign(data, key[, alg])`**  
RSA signature (default SHA-256); returns base64.

**`k.verify(data, signature, key[, alg])`**  
Verifies an RSA signature; returns true/false.

#### Xml

**`k.xml_attr(doc, path, name)`**  
Returns attribute value at path.

**`k.xml_attrs(doc, path)`**  
Returns all attributes of element at path as a table.

**`k.xml_child(doc, path)`**  
Returns child element at path (e.g., "book/author").

**`k.xml_child_list(doc, path)`**  
Returns a table of child elements at path.

**`k.xml_content(doc, path)`**  
Returns text content of element at path.

**`k.xml_name(doc, path)`**  
Returns the name of the element at path.

**`k.xml_parse(text)`**  
Parses XML text and returns a document handle.

**`k.xml_root(doc)`**  
Returns the root element name of a parsed document.

#### Server

**`k.shared.del(key)`**  
Deletes a key from shared state.

**`k.shared.get(key)`**  
Retrieves a value from shared state; empty string if missing.

**`k.shared.incr(key[, delta])`**  
Increments a numeric key by delta (default 1); returns new value.

**`k.shared.keys([pattern])`**  
Returns all keys matching pattern (prefix, * = all).

**`k.shared.set(key, value)`**  
Stores a string value in shared state.

**`k.tcp.close(client_id)`**  
Closes a TCP connection.

**`k.tcp.send(client_id, data)`**  
Sends data to a specific TCP client.

**`k.ws.broadcast(message)`**  
Broadcasts a text message to all connected WebSocket clients.

**`k.ws.close(client_id)`**  
Closes a WebSocket connection.

**`k.ws.send(client_id, message)`**  
Sends a text message to a specific WebSocket client.

#### Comm

**`k.ftp_connect(host[, port, user, pw])`**  
Connects to an FTP server; returns a handle.

**`k.ftp_create_dir(handle, path)`**  
Creates a remote directory (MKD).

**`k.ftp_delete(handle, path)`**  
Deletes a remote file (DELE).

**`k.ftp_disconnect(handle)`**  
Sends QUIT and closes the FTP connection.

**`k.ftp_file_exists(handle, path)`**  
Reports whether a remote file exists (SIZE).

**`k.ftp_get_file(handle, remote, local)`**  
Downloads remote to a local path (RETR).

**`k.ftp_list(handle[, path])`**  
Lists remote entry names (LIST).

**`k.ftp_put_file(handle, local, remote)`**  
Uploads a local file to remote (STOR).

**`k.ftp_rename(handle, from, to)`**  
Renames a remote file or folder (RNFR/RNTO).

**`k.ftp_set_cwd(handle, path)`**  
Changes the remote working directory (CWD).

**`k.socket_close(handle)`**  
Closes an open socket.

**`k.socket_open(host, port[, timeout_ms])`**  
Opens a TCP client connection; returns a handle.

**`k.socket_read(handle[, count])`**  
Reads count bytes (or all until close) from a socket.

**`k.socket_read_line(handle)`**  
Reads one line (trailing newline trimmed); nil at EOF.

**`k.socket_write(handle, data)`**  
Writes data to an open socket; returns bytes written.

**`k.webservice_run(profile, params)`**  
Calls a SOAP web service. profile: {url,action[,method,timeout_ms]}; params is the body table. Returns {status,headers,body}.

#### Email

**`k.pop3_connect{host,port,user,pw,tls}`**  
Connects to a POP3 server; returns a handle.

**`k.pop3_dele(handle, index)`**  
Marks a message for deletion.

**`k.pop3_list(handle)`**  
Returns a table of {id,size} message summaries.

**`k.pop3_noop(handle)`**  
Keeps the POP3 connection alive.

**`k.pop3_quit(handle)`**  
Sends QUIT and closes the POP3 connection.

**`k.pop3_retr(handle, index)`**  
Retrieves a message by index.

**`k.pop3_stat(handle)`**  
Returns {count,size} of the mailbox.

**`k.smtp_connect{host,port,user,pw,tls}`**  
Connects to an SMTP server; returns a handle.

**`k.smtp_disconnect([handle])`**  
Closes an SMTP connection (or all).

**`k.smtp_send(handle, {from,to,subject,body,attachments})`**  
Sends an email through the connected SMTP server.

#### Formats

**`k.csv_load(path[, opts])`**  
Reads and parses a CSV file.

**`k.csv_parse(text[, opts])`**  
Parses CSV (opts {header, sep, quote}).

**`k.csv_save(path, data[, opts])`**  
Writes a CSV file atomically.

**`k.csv_string(data[, opts])`**  
Serializes CSV from a table.

**`k.ini_load(path)`**  
Reads and parses an INI file.

**`k.ini_parse(text)`**  
Parses INI into {section={key=value}, _root={...}}.

**`k.ini_read(path, section, key)`**  
Reads a single INI key (Kalipso parity).

**`k.ini_save(path, data)`**  
Writes an INI file atomically.

**`k.ini_string(data)`**  
Serializes INI from a table.

**`k.ini_write(path, section, key, value)`**  
Writes a single INI key (Kalipso parity).

**`k.xml_load(path)`**  
Reads an XML file into the element-table shape {_name,_attrs,_children,_text}.

**`k.xml_save(path, table)`**  
Writes an element-table as XML.

**`k.yaml_load(path)`**  
Reads and parses a YAML file.

**`k.yaml_parse(text)`**  
Parses YAML; multi-document input yields a list of tables.

**`k.yaml_save(path, data)`**  
Writes a YAML file atomically.

**`k.yaml_string(data)`**  
Serializes a value as YAML.

#### Rows

**`k.csv_to_rows(csvTable)`**  
Converts a parsed CSV table into {columns, rows}.

**`k.json_to_rows(value)`**  
Converts a JSON array of row-maps into {columns, rows}.

**`k.rows_to_csv(result[, opts])`**  
Serializes a result set as CSV.

**`k.rows_to_json(result)`**  
Extracts the rows array from a result set.

**`k.rows_to_xml(result[, rootName[, rowName]])`**  
Serializes a result set as XML.

**`k.xml_to_rows(document)`**  
Converts an XML element-table into {columns, rows}.

### K.* Helpers & Constants

**`K.EQ`**  
Operator constant "=".

**`K.NEQ`**  
Operator constant "<>".

**`K.ADD`**  
Operator constant "+".

**`K.eq(a, b)`**  
Kalipso equality (numeric when both coerce, else string compare).

**`K.ne(a, b)`**  
Negation of K.eq.

**`K.add(a, b)`**  
Kalipso addition: numeric if both coerce, else concatenation.

**`K.tonum(x)`**  
Coerces to a number, else 0.

**`K.tostr(x)`**  
Coerces to the Kalipso string form.

**`K.truthy(x)`**  
Kalipso condition test (0/"0"/""/nil are false).

**`K.NULL`**  
Sentinel representing a JSON null.

**`K.is_null(value)`**  
Reports whether value is K.NULL.

### Expression Functions (§5.9)

> Flat globals (not under `k.*`) — Kalipso-style expressions.

#### String

**`ascii(ch)`**  
Returns the numeric code of the first byte of ch.

**`base64_decode(s)`**  
Decodes base64 into a string.

**`base64_encode(s)`**  
Encodes s as base64.

**`charact(code)`**  
Returns the byte corresponding to code.

**`complete(s, length[, pad])`**  
Pads s on the right to length with pad (default space).

**`decode(s)`**  
Alias of urldecode.

**`encode(s)`**  
Alias of urlencode.

**`extract_string(s, start, end)`**  
Returns s from 1-based start to end inclusive.

**`file_extract_part(path, part)`**  
Extracts path/name/ext from a file path.

**`find(s, needle[, start])`**  
Returns the 1-based position of needle in s (0 if absent).

**`full_encode(s)`**  
Percent-encodes every non-unreserved byte.

**`guid()`**  
Generates a random RFC 4122 version-4 UUID.

**`jsondecode(text)`**  
Parses JSON text (same as k.json_parse).

**`jsonencode(value)`**  
Encodes a value as compact JSON (same as k.json_string).

**`left(s, n)`**  
Returns the first n characters of s.

**`length(s)`**  
Returns the byte length of s (Lua # semantics).

**`lower(s)`**  
Converts s to lowercase.

**`middle(s, start, count)`**  
Returns count characters of s starting at 1-based start.

**`mltext(...)`**  
Joins arguments with newlines (multi-line text).

**`replace(s, old, new)`**  
Replaces all occurrences of old in s with new.

**`right(s, n)`**  
Returns the last n characters of s.

**`set_string(s, start, count, new)`**  
Replaces count characters of s at start with new.

**`string_count(s, needle)`**  
Counts non-overlapping occurrences of needle in s.

**`trim(s)`**  
Removes leading/trailing whitespace from s.

**`upper(s)`**  
Converts s to uppercase.

**`urldecode(s)`**  
Decodes a URL-encoded string.

**`urlencode(s)`**  
URL-encodes s (query style, spaces become +).

**`xmldecode(s)`**  
Unescapes XML entities.

**`xmlencode(s)`**  
Escapes XML special characters.

#### Numeric

**`abs(x)`**  
Absolute value of x.

**`acos(x)`**  
Arccosine in radians.

**`asin(x)`**  
Arcsine in radians.

**`atan(x)`**  
Arctangent in radians.

**`bitwise_and(a, b)`**  
Bitwise AND of the integer parts of a and b.

**`bitwise_or(a, b)`**  
Bitwise OR of the integer parts of a and b.

**`bitwise_xor(a, b)`**  
Bitwise XOR of the integer parts of a and b.

**`ceiling(x)`**  
Smallest integer >= x.

**`cos(x)`**  
Cosine of x (radians).

**`dec_part(x)`**  
Fractional part of x.

**`deg2rad(x)`**  
Converts degrees to radians.

**`exp(x)`**  
e raised to x.

**`extractstringd(s)`**  
Digits of s as a number.

**`floor(x)`**  
Largest integer <= x.

**`int_part(x)`**  
Integer part of x (truncated).

**`log(x)`**  
Natural logarithm of x.

**`log10(x)`**  
Base-10 logarithm of x.

**`mask_number(x, mask)`**  
Formats x using a mask (#, 0, comma grouping, decimal).

**`nth_root(x, n)`**  
The n-th root of x.

**`power(x, y)`**  
x raised to the power y.

**`rad2deg(x)`**  
Converts radians to degrees.

**`random() / random(max) / random(min, max)`**  
Random float in [0,1), integer in [1,max], or in [min,max).

**`round(x[, decimals])`**  
Rounds x to decimals (default 0) with half-away-from-zero.

**`sin(x)`**  
Sine of x (radians).

**`sqrt(x)`**  
Square root of x.

**`sum(table)`**  
Sum of the numeric values in a table.

**`tan(x)`**  
Tangent of x (radians).

**`val(x)`**  
Numeric value of x; 0 when not numeric.

#### Conditional

**`iif(cond, a, b)`**  
Inline if; returns a when cond is truthy, else b.

**`lookup(key, k1, v1, ...)`**  
Returns the value whose key equals key; "" when absent.

**`yesno(cond, a, b)`**  
Returns a when cond is Kalipso-truthy, else b.

#### Datetime

**`add_days(date, n)`**  
Date n days later as "YYYY-MM-DD".

**`date_diff(d2, d1)`**  
Whole days between two date strings (d2 minus d1).

**`date_to_string(date[, format])`**  
Formats a date using %Y %m %d etc.; default "YYYY-MM-DD".

**`datetime_add(dt, days[, hours[, minutes[, seconds]]])`**  
Adds a duration to a datetime string.

**`datetime_diff(dt2, dt1)`**  
Seconds between two datetime strings.

**`datetime_sub(dt, days[, hours[, minutes[, seconds]]])`**  
Subtracts a duration from a datetime string.

**`day(date)`**  
Day of month of a date string.

**`hour(time)`**  
Hour of a date/time string.

**`julian(date)`**  
Julian day number of a date.

**`local_to_utc(dt)`**  
Converts a local datetime string to UTC.

**`minute(time)`**  
Minute of a date/time string.

**`month(date)`**  
Month (1-12) of a date string.

**`second(time)`**  
Second of a date/time string.

**`subtract_days(date, n)`**  
Date n days earlier as "YYYY-MM-DD".

**`sys_date()`**  
Today's date as "YYYY-MM-DD".

**`sys_time()`**  
Current time as "HH:MM:SS".

**`tick_count()`**  
Unix milliseconds since the epoch.

**`time_to_string(time[, format])`**  
Formats a time using %H %M %S etc.; default "HH:MM:SS".

**`utc_to_local(dt)`**  
Converts a UTC datetime string to local wall-clock.

**`week_day(date)`**  
Day of week as 1-7 (Sunday=1).

**`week_number(date)`**  
ISO week number of a date.

**`year(date)`**  
Year of a date string.

#### Conversion

**`boolstr(v)`**  
{"true","false"} for the Kalipso truthiness of v.

**`strtodate(s[, format])`**  
Parses s (optionally with %-tokens) and emits "YYYY-MM-DD".

**`todate(s)`**  
Normalizes s to "YYYY-MM-DD".

**`tonum(x)`**  
Kalipso number of x; 0 when not numeric.

**`tostr(x)`**  
Kalipso string form of x.

### Script Globals

- **`ARGS`** — Table seeded from `--arg K=V` flags (string keys).
- **`CTRL`** — Accessor: `CTRL(name)` returns a control handle for `k.ctrl.*` operations.
- **`main`** — Entry point function (required in run mode).


<!-- KALUA:gen-api-end -->

---

# 6. Examples

All examples below ship in `testdata/apps/` and are runnable.

## 6.1 Minimal app — `hello.lua`

```lua
function main()
  k.print("hello")
  k.sleep(50)
  k.quit()
end
```

```bash
./KALUA run testdata/apps/hello.lua
```

## 6.2 Form + events + msgbox — `test_form.lua`

```lua
function main()
  k.form.new("main", {title="Test Form", layout="vertical"})
  k.ctrl.label("main", "lbl1", {text="Hello KALUA!"})
  k.ctrl.textbox("main", "txt1", {label="Name", value="World"})
  k.ctrl.button("main", "btn1", {label="Click Me",
    onclick=function()
      local name = k.ctrl.get_value("main", "txt1")
      k.msgbox("Hello " .. name .. "!")
      k.quit()
    end})
  k.form.show("main")
end
```

Notice `onclick` is a Lua closure; `k.msgbox` suspends and resumes when the user chooses. Browser state round-trips through the session, so `k.ctrl.get_value` returns what the user typed.

## 6.3 Charts — `chart_demo.lua`

Chart types `line`, `bar`, `hbar`, `pie`, `doughnut`, `scatter`, `radar`, `area` with timers pushing live data:

```lua
k.form.new("dashboard", {title = "Sales Dashboard", layout = "grid"})
k.ctrl.chart("dashboard", "sales_trend", { type = "line", title = "Monthly Sales",
  labels = {"Jan", "Feb", "Mar"},
  datasets = {{ label = "Revenue", data = {12000, 19000, 15000} }},
  options = { scales = { y = { beginAtZero = true } } } })

k.timer_start("tick", 3000, true)            -- fire global `tick()` every 3 s
function tick()
  k.chart.set_data("dashboard", "sales_trend", { labels = ..., datasets = ... })
end
```

`chart_click`/`chart_hover`/`chart_legend_click` events arrive via `k.form.on`, and `k.chart.get_image` returns a PNG data URL.

## 6.4 DB-linked table with server-side paging — `tabulator_demo.lua`

```lua
function main()
  local db = k.connect_sqlite(".kalua_tabdemo.db")
  -- ... create & seed table ...

  k.form.new("main", { title="Customers", layout="vertical" })
  k.ctrl.table("main", "customers", {
    db = db,
    query = "SELECT id, name, city, balance FROM customers",
    page_size = 25,
    count_query = "SELECT count(*) AS n FROM customers",
    columns = { { field = "id" }, { field = "name" }, { field = "city" } },
    allow_edit = true,
  })
  k.form.show("main")
end
```

`k.table.refresh` runs the query again; `k.table.set_remote_data` pushes paged rows; row selects fire `onselect`/`onclick`. The same pattern with `k.ctrl.looper` + `links` repeats a template per row (`looper_demo.lua`).

## 6.5 Grid layout builder demo — `layout_demo.lua`

A header/main/sidebar/footer grid where a button moves a control between cells at runtime:

```lua
k.form.new("grid_demo", { title="Dashboard (layout=grid)", layout="grid", gap=16,
  cells = {
    { id="header",  width=12, bg="#f5f5f5", border={width=1, color="#ddd"} },
    { id="main",    width=9 },
    { id="sidebar", width=3 },
    { id="footer",  width=12 },
  }})
k.ctrl.button("grid_demo", "btn_move", { cell="main", label="Move to Sidebar",
  onclick = function()
    k.ctrl.set_property("grid_demo", "btn_move", "cell", "sidebar")
  end })
```

At < 600 px the layout collapses to a single column automatically.

## 6.6 Serve-mode API — `api_demo.lua`

```bash
./KALUA serve testdata/apps/api_demo.lua --port 8080 --workers 2 --mode http,ws
```

```lua
function init(config)                      -- runs once at startup
  k.shared.set("demo:counter", "0")
end

function handle_http(req)                  -- every HTTP request
  if req.path == "/health" then
    return {json = {status = "ok", visits = k.shared.incr("demo:visits")}}
  elseif req.path == "/expr" then
    return {json = {upper = upper(req.query.input or "hi")}}
  end
  return {status = 404, json = {error = "not found"}}
end

function handle_ws(msg)                    -- type: open|text|binary|close
  if msg.type == "text" then
    return jsonencode({ echo = jsondecode(msg.data) })  -- echoed back
  end
end

function handle_tcp(msg) return "ECHO " .. msg.data end

function shutdown() k.print("bye") end     -- on SIGTERM/SIGINT
```

`handle_http` supports the response forms: `nil` (empty 200), a plain string (text/plain), `{json=...}`, `{status=n}`, or `{status, headers, body}`. `req` carries `method`, `path`, `query`, `query_raw`, `remote_addr`, `tls`. `SIGHUP` hot-reloads the script without dropping in-flight work.

## 6.7 Data round-trips in one breath

```lua
local rows = k.sql(db, "SELECT * FROM orders")
local json = k.json_string(k.rows_to_json(rows))
local csv  = k.rows_to_csv(k.csv_to_rows(k.csv_parse(csv_text)))
local yml  = k.yaml_string({orders = rows})
```

---

# 7. KALUA Builder

The **KALUA Builder** is a design-time visual editor for forms. It never executes the app; it renders the **real Go renderer** in-process, so the preview is pixel-accurate. One form per builder file.

## 7.1 Start

```bash
./KALUA builder <app.lua|form.json> [--host 127.0.0.1] [--port 9001] [--no-browser] [-p N] [-n]
```

The file need not exist: a new empty form is served, and **Save** creates it. Lua files are written as generated source; `.kalua-form.json` files as documents.

## 7.2 Workflow

1. Select the form layout (**Vertical** or **Grid** with cells) in the form editor.
2. Drag controls from the palette onto the canvas (11 types: label, textbox, button, combo, list, table, looper, chart, image, checkbox, radio).
3. Edit properties in the property panel (label, value, items, chart type/data, cell assignment, alignment…).
4. The preview updates via a debounced server round-trip (the Go renderer is the source of truth).
5. **Save:** for a `.lua` target, KALUA exports the generated `k.form.new` / `k.ctrl.*` / `k.form.on` source; for `.json`, it writes the document.

## 7.3 Importing existing Lua

- Structure is extracted from the AST: the **first** `k.form.new` wins.
- Controls are imported in creation order (vertical rendering and grid bucketing depend on it).
- Non-form code is noted as "will be lost on export".
- Event handlers are **preserved as read-only metadata** (the builder shows how many handlers are wired); export emits `k.form.on` / `onclick` **TODO stubs** for you to fill in.
- Additional forms are skipped with a note.

## 7.4 Document format (`.kalua-form.json`)

```jsonc
{
  "version": 1,
  "form": {
    "name": "main",
    "title": "Test Form",
    "layout": "vertical",
    "align": "left",
    "gap": 16,
    "cells": {},                    // grid: { "id": { "width": 1-12, "bg": "...", "border": {"width":1,"color":"#ddd"}, "align": "..." } }
    "controls": [
      { "name": "lbl1", "type": "label", "text": "Hello" },
      { "name": "txt1", "type": "textbox", "label": "Name", "value": "World" }
    ],
    "handlers": { "btn1": ["click"] }   // read-only metadata
  }
}
```

Common control properties: `name`, `type`, `label` (label control uses `text`), `value`, `enabled`, `visible`, `class`, `cell`, `align`. Type-specific ones:

| Control | Notable properties |
|---------|--------------------|
| label | `text`, `multiline` |
| textbox | `value`, `multiline`, `rows`, `cols`, `datetime` (bool or `{mode,format,min,max,step}`) |
| button | `label`, `class` |
| combo/list | `items`, `value` |
| table | `columns`, `rows`, `data`; DB: `db`,`query`,`page_size`,`count_query`,`where`,`order_by` |
| looper | `columns`; DB: `db`,`query`,`links`,`page_size`,`count_query`,`where`,`order_by` |
| chart | `chart_type`, `title`, `labels`, `datasets`, `options`, `width`, `height`, `legend`, `stacked` |
| image | `src` (required), `alt`, `width`, `height`, `fit`, `clickable` |
| checkbox/radio | `value`, `hidden_value` |

- **items** (combo/list): JSON stores an ordered array `[{key, display}]`; Lua export emits the runtime map literal `items={k1="Display 1"}`.
- **DB handles are runtime values** — `db` is not serializable. The builder stores the structural options (`query`, `where`, `…`) and exports a `-- TODO: assign a DB handle` comment in place of `db`.

## 7.5 HTTP API

The builder is a small local HTTP server (embedded UI under `/`):

| Endpoint | Purpose |
|----------|---------|
| `GET /api/form` | Current document (imported from Lua or read from JSON) |
| `PUT /api/form` | Save the document (writes Lua source export or the JSON file) |
| `GET /api/source` | Raw file contents |
| `POST /api/export` | Generated Lua source for a document |
| `POST /api/import` | Document extracted from Lua source |
| `POST /api/validate` | Static validation of Lua (`KALUA check` logic) |
| `POST /api/preview` | HTML preview rendered by the real Go renderer |
| `GET /healthz` | Liveness |

---

*Generated sections of this guide (chapter 5) come from `internal/bindings/api_doc.go`. Keep them fresh with `go run ./cmd/kalua-userguide`.*