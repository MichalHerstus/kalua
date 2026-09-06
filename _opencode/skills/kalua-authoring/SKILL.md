# KALUA Authoring Skill

**Keywords:** kalua, kalipso, k.form, k.ctrl, handle_http, serve, lua, web app

---

## Overview

This skill guides AI agents through authoring, reviewing, and fixing **KALUA** `.lua` apps — single-file web apps or headless APIs that run on the KALUA Go runtime (embedded gopher-lua VM + `net/http` server + WebSocket bridge).

**Source of truth:** `kalua_spec.md` (§1–§12), `internal/bindings/api_doc.go`, generated `api.md`.

---

## App Model

### Run Mode (Interactive Web App)
```lua
function main()
  -- 1. Optional DB connection
  k.connect_db("sqlite://app.db")

  -- 2. Declare forms
  k.form.new("main", {title = "My App"})
  k.ctrl.textbox("main", "name", {label = "Name"})
  k.ctrl.button("main", "ok", {label = "OK", onclick = function()
    k.msgbox("Hello, " .. k.ctrl.get_value("main", "name"))
  end})

  -- 3. Show form (suspends until closed)
  k.form.show("main")
end
```

- Entry point: **`function main()`** (required)
- Each browser tab = one session = one Lua VM
- `k.form.show()` suspends the coroutine until the form closes (modal)
- Events (`onclick`, `whenever_modified`, etc.) run inside the session actor

### Serve Mode (Headless API)
```lua
function init(config)          -- optional: runs once at startup
  k.connect_db("sqlite://app.db")
  k.shared.set("version", "1.0")
end

function handle_http(req)      -- required for --mode http
  return {status = 200, json = {ok = true, path = req.path}}
end

function handle_ws(msg)        -- required for --mode ws
  if msg.type == "text" then
    k.ws.broadcast({type = "text", data = "echo: " .. msg.data})
  end
end

function handle_tcp(msg)       -- required for --mode tcp
  k.tcp.send(msg.client_id, "echo: " .. msg.data)
end

function shutdown()            -- optional: runs on SIGTERM/SIGINT
  k.disconnect_db()
end
```

- Worker pool with shared `k.shared.*` state (thread-safe, JSON-backed)
- UI bindings (`k.form.*`, `k.ctrl.*`, `k.msgbox`, `k.status_*`) **raise runtime error**

---

## Golden Authoring Loop

```bash
# 1. Write / edit the .lua file
# 2. Static validation (syntax + unknown k.* refs)
./KALUA check myapp.lua

# 3. Headless test run (no browser, no HTTP server)
./KALUA run myapp.lua --test

# 4. Interactive run (opens browser at printed URL)
./KALUA run myapp.lua

# 5. Headless API server
./KALUA serve myapp.lua --port 8080 --workers 4 --mode http,ws
```

**Iterate on the CLI output.** The LSP (via `./KALUA lsp`) gives live diagnostics, completion, and hover while editing.

---

## Conventions & Pitfalls

| Topic | Rule |
|-------|------|
| **Naming** | `snake_case` everywhere (`k.form.new`, `k.ctrl.textbox`, `k.http_request`) |
| **Expression functions** | **Flat globals**, NOT under `k.*` — use `left(s, 3)`, `round(x)`, `sys_date()`, `lookup(k, k1, v1, ...)` |
| **Serve mode** | Any `k.form.*`, `k.ctrl.*`, `k.msgbox`, `k.status_*` → runtime error |
| **Coercion** | Kalipso weak typing: `0 == ""` is **true**; use `K.tonum()`, `K.tostr()`, `K.truthy()` for byte-exact semantics |
| **Dates** | Strings `"YYYY-MM-DD[ HH:MM[:SS]]"`; `week_day()` returns **Sunday = 1** |
| **Sandbox** | No `io`, `os.execute`, `require`; only `k.*`, `K.*`, expression globals, `debug.*` (under `--debug`) |
| **File cap** | 16 MiB default (`--max-file-size`) for load/save operations |
| **Globals** | `ARGS` (from `--arg`), `CTRL(name)` accessor, `main` (run mode) |

---

## Key Reference Pointers

- **Full API reference:** `kalua-api/api.md` (generated, drift-gated)
- **Spec inventory:** `kalua_spec.md` §5 (all action/function groups with tiers)
- **LSP commands:** `KALUA lsp` — completion, hover, definition, diagnostics for `k.*`/`K.*`/globals
- **Checker:** `KALUA check` — syntax + unknown `k.*` detection

---

## Common Patterns

### Form with Event Handlers
```lua
k.form.new("order", {title = "Order Entry"})
k.ctrl.textbox("order", "qty", {label = "Qty"})
k.ctrl.button("order", "calc", {label = "Calc", onclick = function()
  local n = K.tonum(k.ctrl.get_value("order", "qty"))
  k.ctrl.set_value("order", "total", n * 10)
end})
k.form.show("order")
```

### Async HTTP Request (run mode)
```lua
local resp = k.http_request{
  method = "GET",
  url = "https://api.example.com/data",
  timeout = 5000
}
k.print("Status:", resp.status, "Body:", resp.body)
```

### Shared State (serve mode)
```lua
function handle_http(req)
  local visits = k.shared.incr("visits")
  return {json = {visits = visits}}
end
```

### Data Format Round-trip
```lua
local data = k.json_load("config.json")
data.version = data.version + 1
k.json_save("config.json", data)
```

---

## Debugging

```bash
# Enhanced tracing (function calls, args, returns, k.* API)
./KALUA run myapp.lua --verbose

# Drop into REPL at crash site
./KALUA run myapp.lua --repl-on-error

# Headless test with REPL on error
./KALUA run myapp.lua --test --repl-on-error
```

---

## Checklist for Agent Review

- [ ] `main()` exists (run) or `handle_http`/`handle_ws`/`handle_tcp` exist (serve)
- [ ] `./KALUA check` passes (no unknown `k.*`)
- [ ] `./KALUA run --test` exits 0
- [ ] No UI bindings in serve mode
- [ ] Expression functions called as flat globals (`upper(s)` not `k.upper(s)`)
- [ ] Coercion helpers used where Kalipso semantics matter (`K.eq`, `K.tonum`, `K.truthy`)