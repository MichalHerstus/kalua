# KALUA Serve Communication Capabilities — Implementation Plan

**Status**: Plan only — not implemented.

**Scope**: Add REST, GraphQL, SOAP servers; SFTP server; MQTT client; WebSocket enhancements; UDP server to `kalua serve` mode.

---

## Decisions

| # | Decision | Choice |
|---|----------|--------|
| 1 | GraphQL Library | `github.com/graph-gophers/graphql-go` (runtime-only, no codegen) |
| 2 | SOAP Parser | Custom lightweight (~200 lines) |
| 3 | SFTP Host Key | Both — generate on first run + load from file |
| 4 | Auth Middleware | Add `k.http_auth_middleware(fn)` |
| 5 | Metrics | Add `k.http_stats()`, `k.ws_stats()`, `k.sftp_stats()` |

---

## Phase 0: Serve Mode Correctness (Prerequisite)

*From `kalua-serve-enhancements.md` — must complete first*

| Task | Files |
|------|-------|
| 0.1 Register `k.http_request`, `k.json_*`, `k.xml_*` in `SetupServe` | `internal/bindings/serve.go` |
| 0.2 Fix `k.sleep` → `time.Sleep` | `internal/bindings/serve.go` |
| 0.3 Fix `k.exec` / `k.assign(table)` | `internal/bindings/flow.go` |
| 0.4 Replace instant 503 with bounded wait + `Retry-After` | `internal/server/worker.go` |
| 0.5 `k.quit` stops server | `internal/bindings/serve.go`, `internal/server/server.go` |
| 0.6 Register flow subset: `k.param_get/set`, `k.net_ok`, `k.locale`, `k.ping`, `k.yield`, `k.error` | `internal/bindings/serve.go` |

**Validation**: `KALUA serve app.lua --test` passes; `k.http_request` works in serve mode.

---

## Phase 1: REST Router + HTTP Enhancements

### API Design

```lua
-- Declarative routes table (Option B)
k.http_routes = {
  {method="GET", path="/api/users/:id", handler="get_user"},
  {method="POST", path="/api/users", handler="create_user"},
}

-- Middleware chain
k.http_middleware = {
  "auth_middleware",  -- runs first
  "logging_middleware",
  "cors_middleware",
}

-- Handler
function get_user(req)
  local id = req.params.id
  return k.http_response_json({id=id, name="John"})
end

-- Auth middleware
function auth_middleware(req)
  local token = req.headers["authorization"]
  if not valid(token) then
    return k.http_response_json({error="unauthorized"}, 401)
  end
  req.user = decode(token)  -- pass data to next handler
  return nil  -- continue chain
end
```

### New Helpers

- `k.http_response_json(data[, status])`
- `k.http_response_text(text[, status])`
- `k.http_response_redirect(url[, status])`
- `k.http_request_parse_multipart(req)`
- `k.http_auth_middleware(fn)` — registers middleware
- `k.http_stats()` — `{requests_total, errors_total, avg_latency_ms, active_connections}`

### Files

| File | Change |
|------|--------|
| `internal/bindings/serve.go` | Add `registerHTTPRouter`, `registerHTTPHelpers` |
| `internal/server/worker.go` | Router lookup in `CallHTTP`, middleware chain execution |
| `internal/bindings/api_doc.go` | Document `k.http_routes`, `k.http_middleware`, helpers |
| `internal/checker/checker.go` | Allow `k.http_routes`, `k.http_middleware` globals |

**Dependency**: `github.com/julienschmidt/httprouter`

---

## Phase 2: MQTT Client

*Per `kalua-serve-enhancements.md` §5*

### API

```lua
k.mqtt_connect{broker, port, user, password, client_id, tls} -> handle
k.mqtt_subscribe(handle, topic[, qos])
k.mqtt_publish(handle, topic, payload[, qos, retain])
k.mqtt_unsubscribe(handle, topic)
k.mqtt_close(handle)
k.mqtt_on(pattern, fn_name)
k.mqtt_stats(handle) -> {connected, subscriptions, messages_in, messages_out}
```

### Files

| File | Purpose |
|------|---------|
| `internal/server/mqtt.go` | `MQTTHub` — connection pool, subscription management |
| `internal/bindings/mqtt.go` | `k.mqtt_*` bindings |
| `internal/bindings/serve.go` | Register MQTT in `SetupServe` |
| `internal/bindings/api_doc.go` | Document all functions |

**Dependency**: `github.com/eclipse/paho.mqtt.golang`
**Security**: Gated behind `--allow-net`

---

## Phase 3: GraphQL Server

*Schema-first, Lua table, `graphql-go/graphql`*

### API

```lua
-- Schema as Lua table
k.graphql_schema = {
  types = {
    User = {fields={id={type="ID"}, name="String", email="String", posts="[Post]"}},
    Post = {fields={id={type="ID"}, title="String", author="User"}},
    Query = {fields={
      user = {type="User", args={id="ID"}, resolve="resolve_user"},
      users = {type="[User]", resolve="resolve_users"},
    }},
    Mutation = {fields={
      createUser = {type="User", args={name="String", email="String"}, resolve="create_user"},
    }},
  }
}

-- Resolvers receive (args, context, info)
function resolve_user(args, ctx, info)
  return k.db_select("users", "*", "id=?", {args.id})[1]
end

-- Optional: context initializer (runs per request)
k.graphql_context = function(req)
  return {user = get_user_from_token(req.headers["authorization"])}
end

k.graphql_stats() -> {queries_total, mutations_total, errors_total, avg_latency_ms}
```

### Files

| File | Purpose |
|------|---------|
| `internal/server/graphql.go` | Schema parser (Lua table → gqlparser AST), executor |
| `internal/bindings/graphql.go` | Reads `k.graphql_schema`, registers `/graphql` endpoint |
| `internal/bindings/serve.go` | Register GraphQL in `SetupServe` |
| `internal/bindings/api_doc.go` | Document `k.graphql_schema`, `k.graphql_context` |

**Dependencies**: `github.com/graph-gophers/graphql-go`, `github.com/vektah/gqlparser/v2`

---

## Phase 4: SOAP Server

*Raw envelope, custom lightweight parser*

### API

```lua
k.soap_operations = {
  ["GetUser"] = "soap_get_user",
  ["CreateUser"] = "soap_create_user",
}

function soap_get_user(body, headers)
  -- body is parsed XML table: {GetUser = {userId = "123"}}
  local user = k.db_select("users", "*", "id=?", {body.GetUser.userId})
  return {GetUserResponse = {user = user[1]}}
end

-- Optional: custom fault
function soap_fault(code, message)
  return {fault = {faultcode = code, faultstring = message}}
end

k.soap_stats() -> {requests_total, faults_total, avg_latency_ms}
```

### Files

| File | Purpose |
|------|---------|
| `internal/server/soap.go` | SOAP 1.1/1.2 parser, envelope dispatcher |
| `internal/bindings/soap_server.go` | Reads `k.soap_operations` |
| `internal/bindings/serve.go` | Register SOAP in `SetupServe` |
| `internal/bindings/api_doc.go` | Document `k.soap_operations`, `k.soap_fault` |

---

## Phase 5: SFTP Server

*SSH-based, single port, both key modes*

### API

```lua
k.sftp_server_start{
  port = 2222,
  host_key = "path/to/host_key",      -- optional: load existing
  host_key_type = "ed25519",          -- or "rsa", used for generation
  auth = "both",                      -- "password" | "publickey" | "both"
  users = {
    alice = {password="secret", home="/data/alice", permissions="rw"},
    bob = {public_key="ssh-ed25519 AAAA...", home="/data/bob", permissions="r"},
  },
  on_upload = "handle_upload",
  on_delete = "handle_delete",
  on_connect = "handle_connect",
  on_disconnect = "handle_disconnect",
} -> handle

k.sftp_server_stop(handle)
k.sftp_stats(handle) -> {connections, uploads, downloads, bytes_in, bytes_out}
```

### Event Handlers

```lua
function handle_upload(session, filepath, size)
  -- session = {user="alice", remote_addr="1.2.3.4:5678", auth_method="password"}
  k.log.info("SFTP upload", {user=session.user, file=filepath, size=size})
end
```

### Files

| File | Purpose |
|------|---------|
| `internal/server/sftp.go` | SFTP server using `gliderlabs/ssh` + `pkg/sftp` |
| `internal/bindings/sftp.go` | `k.sftp_server_start/stop`, stats |
| `internal/bindings/serve.go` | Register SFTP in `SetupServe` |
| `internal/bindings/api_doc.go` | Document SFTP functions |

**Dependencies**: `github.com/gliderlabs/ssh`, `github.com/pkg/sftp`, `golang.org/x/crypto`
**Key Generation**: On first run if `host_key` not provided, generate Ed25519 key, save to `host_key` path, log fingerprint.

---

## Phase 6: WebSocket Enhancements

### API

```lua
-- Router (pattern matching)
k.ws_routes = {
  {pattern="/api/:resource", handler="ws_api_handler"},
  {pattern="/chat/:room", handler="ws_chat_handler"},
}

-- Per-connection state
k.ws_session_set(client_id, "user_id", 123)
k.ws_session_get(client_id, "user_id") -> 123

-- Rooms / PubSub
k.ws_room_join(client_id, "room1")
k.ws_room_leave(client_id, "room1")
k.ws_room_broadcast("room1", {type="message", text="hello"})
k.ws_room_clients("room1") -> {"client_id_1", "client_id_2"}

k.ws_stats() -> {connections, rooms, messages_total, broadcast_total}
```

### Files

| File | Purpose |
|------|---------|
| `internal/server/ws.go` | Extend `WSHub` with rooms, session store |
| `internal/bindings/ws_server.go` | `k.ws_routes`, session, rooms bindings |
| `internal/bindings/serve.go` | Register WS enhancements |

---

## Phase 7: UDP Server (Optional)

### API

```lua
k.udp_server_start{port=9999, handler="handle_udp", buffer_size=65536} -> handle
k.udp_server_stop(handle)
k.udp_stats(handle) -> {packets_in, packets_out, bytes_in, bytes_out, errors}
```

---

## Phase 8: Structured Logging to SQLite (`klog.db`)

*From `kalua-serve-enhancements.md` Phase 4 — self-maintained structured log, off by default, no new dependency*

### Why SQLite
- Indexed time-range queries over a file that needs no server, no rotation script, no agent
- **No new dependency**: `modernc.org/sqlite` already a direct dependency (used by `k.connect_*`)
- Pure Go — `CGO_ENABLED=0` release builds stay static
- Replaces `k.print` for queryable logs; `k.print` stays stdout-only

### Architecture
```
N workers ──► Emit() ──► truncate(16 KiB) ──► chan Record (cap 1024)
                                              │
                                    single writer goroutine
                                     batch ≤64 or ≤200 ms
                                              ▼
                                  tx: multi-row INSERT
                                  hourly: age prune + size cap
```

### Schema
```sql
PRAGMA journal_mode=WAL;
PRAGMA busy_timeout=5000;
PRAGMA synchronous=NORMAL;

CREATE TABLE IF NOT EXISTS klog (
  id          INTEGER PRIMARY KEY,
  ts          TEXT    NOT NULL,      -- RFC3339Nano, UTC
  ts_unix_ms  INTEGER NOT NULL,      -- indexed; cheap range predicates
  level       INTEGER NOT NULL,      -- 0..4 (error/warn/info/debug/trace)
  category    TEXT    NOT NULL,      -- http|ws|tcp|worker|app|db|rfid|lifecycle
  worker      INTEGER,
  session     TEXT,
  remote_addr TEXT,
  message     TEXT    NOT NULL,
  data        TEXT                   -- valid JSON object of structured fields
);
CREATE INDEX IF NOT EXISTS klog_ts        ON klog(ts_unix_ms);
CREATE INDEX IF NOT EXISTS klog_level_cat ON klog(level, category);
CREATE TABLE IF NOT EXISTS klog_meta (
  key   TEXT PRIMARY KEY,
  value TEXT                              -- schema_version, created_at, dropped_total
);
```

### Per-Row Cap — 16 KiB
- `message` + `data` combined ≤ `log_max_row` (default 16k)
- `message` truncation: append `…[truncated]` at rune boundary
- `data` truncation: replace with valid envelope `{"_truncated":true,"_orig_bytes":N,"_preview":"…"}`

### Retention
- `--log-retain` (default `7d`) — age prune
- `--log-max-mb` (default `50`) — size cap with 80% low-water mark

### API

```lua
k.log.info("order placed", {order_id = 42, total = 99.5})
k.log.warn("retry attempt", {attempt = 3})
k.log.error("payment failed", {reason = "timeout"})
k.log.debug("cache miss", {key = "user:123"})
k.log.trace("entering function", {fn = "processOrder"})
k.log.stats()   -- {rows, dropped, oldest, newest, path}
```

### CLI Flags
```
kalua serve app.lua --log-sqlite --log-level debug --log-retain 7d \
                     --log-max-mb 50 --log-max-row 16k
```

| Flag | Type | Default | `0` means |
|------|------|---------|-----------|
| `--log-sqlite` | bool | off | — |
| `--log-level` | enum | `info` | — |
| `--log-retain` | duration | `7d` | keep forever |
| `--log-max-mb` | size | `50` | unlimited |
| `--log-max-row` | size | `16k` | unlimited |

### KALUA.INI
```ini
[SERVE]
log-sqlite  = 1
log-level   = info
log-retain  = 7d
log-max-mb  = 50
log-max-row = 16k
```

### Files

| File | Build Tag | Role |
|------|-----------|------|
| `internal/klog/klog.go` | — | `Record`, `Sink`, `Level`, `truncate()` (pure) |
| `internal/klog/size.go` | — | `ParseSize`, `ParseRetention` |
| `internal/klog/sqlite.go` | `!wasm` | `Open`, `Emit`, `Flush`, `Prune`, `Stats`, `Close` |
| `internal/klog/sqlite_wasm.go` | `js && wasm` | no-op sink |
| `internal/bindings/sqllogger.go` | `!wasm` | `SQLLogger` + `Emit` |
| `internal/bindings/log_ops.go` | `!wasm` | `k.log.*` (serve mode) |
| `internal/cli/logflags.go` | — | `sizeValue` / `durValue` flag.Value adapters |

### Security — Script-Denied `klog.db`
- `DenyFS` enforced in `resolvePath`, `k.connect_db`, `k.zip_extract`
- `k.connect_db("sqlite://klog.db")` denied (closes C7 sandbox escape)
- `k.zip_extract` member targets checked against deny-list
- Path fixed: `filepath.Join(pwd, "klog.db")` — ignores `--ini` and script location

### Dependencies
- **None new** — reuses `modernc.org/sqlite` (already in tree)

### Testing
- `klog/sqlite_test.go` — schema, batched flush, drop counter, age prune, size cap, 16 KiB cap, WAL, `Stats`
- `klog/size_test.go` — `ParseSize`/`ParseRetention` variants
- `internal/bindings/deny_test.go` — `file_load`/`csv_load` denied; `zip_extract` member denied; `connect_db("sqlite://klog.db")` denied; in-workdir `app.db` allowed; `klog.db.bak` allowed
- `internal/bindings/sqllogger_test.go` — interface conformance, level gating, stdout still receives output
- `internal/server/log_e2e_test.go` — real HTTP request → row with correct `status`/`duration_ms`; `k.log.info` from Lua → `category='app'`; `k.print` → stdout, no DB row
- `internal/cli/logflags_test.go` — CLI overrides INI; bad CLI/INI value → exit 2 with `KALUA.INI [SERVE]` in stderr
- `go test -race ./internal/server/... ./internal/klog/...` with concurrent workers

---

## Complete Dependency List

```go
// Phase 0: None (stdlib only)

// Phase 1: REST Router
github.com/julienschmidt/httprouter v1.3.0

// Phase 2: MQTT
github.com/eclipse/paho.mqtt.golang v1.4.3

// Phase 3: GraphQL
github.com/graph-gophers/graphql-go v1.0.0
github.com/vektah/gqlparser/v2 v2.5.0

// Phase 4: SOAP (custom, no new deps)

// Phase 5: SFTP
github.com/gliderlabs/ssh v0.3.5
github.com/pkg/sftp v1.13.5
golang.org/x/crypto v0.20.0

// Phase 6: WS Enhancements (stdlib only)

// Phase 7: UDP (stdlib only)

// Phase 8: Structured Logging (no new deps — reuses modernc.org/sqlite)
```

---

## Testing Matrix

| Phase | Unit Tests | Integration Tests |
|-------|------------|-------------------|
| 0 | `serve_test.go` — bindings work | `server_e2e_test.go` — full server |
| 1 | `rest_router_test.go` — routing, params, middleware | `rest_e2e_test.go` — real HTTP calls |
| 2 | `mqtt_test.go` — mock broker | `mqtt_e2e_test.go` — testcontainers mosquitto |
| 3 | `graphql_test.go` — schema, resolvers, variables | `graphql_e2e_test.go` — real queries |
| 4 | `soap_server_test.go` — envelopes, faults | `soap_e2e_test.go` — real SOAP client |
| 5 | `sftp_test.go` — auth, events | `sftp_e2e_test.go` — real SFTP client |
| 6 | `ws_enhancements_test.go` — rooms, sessions | `ws_e2e_test.go` — real WS clients |
| 7 | `udp_test.go` | `udp_e2e_test.go` |
| 8 | `klog/sqlite_test.go`, `size_test.go`, `deny_test.go`, `sqllogger_test.go` | `log_e2e_test.go`, `logflags_test.go` |

---

## File Summary

### New Files (17)
```
internal/server/mqtt.go
internal/bindings/mqtt.go
internal/server/graphql.go
internal/bindings/graphql.go
internal/server/soap.go
internal/bindings/soap_server.go
internal/server/sftp.go
internal/bindings/sftp.go
internal/bindings/ws_server.go
internal/server/udp.go (optional)
internal/klog/klog.go
internal/klog/size.go
internal/klog/sqlite.go
internal/klog/sqlite_wasm.go
internal/bindings/sqllogger.go
internal/bindings/log_ops.go
internal/cli/logflags.go
```

### Modified Files (8)
```
internal/bindings/serve.go      (phases 0-6, 8)
internal/server/worker.go       (phases 0-1)
internal/server/server.go       (phase 0)
internal/bindings/flow.go       (phase 0)
internal/bindings/api_doc.go    (phases 1-6, 8)
internal/checker/checker.go     (phase 1)
internal/bindings/serve.go      (phase 8 - register log_ops)
internal/cli/cli.go             (phase 8 - log flags)
```

---

## Timeline

```
Week 1: Phase 0 (serve correctness)
Week 2: Phase 1 (REST router + HTTP helpers + middleware + metrics)
Week 3: Phase 2 (MQTT client)
Week 4: Phase 3 (GraphQL) + Phase 4 (SOAP) parallel
Week 5: Phase 5 (SFTP) + Phase 6 (WS enhancements) parallel
Week 6: Phase 7 (UDP, optional) + Phase 8 (structured logging) parallel
Week 7: Integration testing + docs
```

**Total: 6-7 weeks**

---

## Implementation Order Notes

1. **Phase 0** is the blocker — everything depends on `k.http_request` and data formats working in serve mode
2. After Phase 0, Phases 1-2 can proceed in parallel (different files)
3. Phases 3-4 can proceed in parallel after Phase 1
4. Phases 5-6 can proceed in parallel after Phase 1
5. **Phase 8** (structured logging) can proceed in parallel with Phase 7 after Phase 1 (independent files, reuses existing sqlite driver)