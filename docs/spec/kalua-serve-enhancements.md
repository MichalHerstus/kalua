# KALUA Serve Enhancements — Flow Primitives

**Status**: Plan only — not implemented.

**Scope decision**: Adopt the *useful primitives* of the Node-RED node model.
No `k.node.*` routing registry, no visual editor, no embedded broker. Plus an MQTT
**client**. **Phases 0–2 cover the core server/sensor/sync use cases** (API server,
sensor ingestion, REST↔SQL sync); **Phases 3–4 are optional hardware/observability
add-ons**.

**Phase 3 (`k.rfid_*`)** adds hardware-specific input functions for Zebra FX/ATR
RFID readers over the ZIOTC protocol, built on the transports above. See §6.

**Phase 4 (`k.log.*`)** adds structured logging to a self-maintained `klog.db`
SQLite file in the working directory — off by default, enabled and tuned by flag
(`--log-sqlite`, `--log-level`, `--log-retain`, `--log-max-mb`, `--log-max-row`),
all also settable in `KALUA.INI [SERVE]`. It reuses the already-present
`modernc.org/sqlite` driver, so it adds no dependency. See §7.

**Threat model** (unchanged from `kalua_security_plan.md`): intranet deployment;
any public exposure sits behind a hardened reverse proxy.

---

## 1. Why this exists

`kalua serve` has **transports but no flow layer**. HTTP in, WS in/out, TCP in/out
and an in-memory key-value store all work. What is missing is everything a
Node-RED-style script needs to actually do work: a scheduler, a real sleep, an
outbound HTTP client, JSON/XML parsing, and error/durability semantics.

Separately, serve mode's execution model is close to the inverse of Node-RED's:

| | Node-RED | KALUA serve (today) |
|---|---|---|
| Concurrency | async, many in-flight | one mutex per worker |
| Dispatch | per-node | `CallByParam`, blocking |
| Backpressure | queues | 256-slot buffer, **silent drop** |
| Admission | unbounded | instant `503 worker busy` |

This must be understood before adding primitives: every new blocking primitive
occupies a worker for its full duration.

---

## 2. Audit findings that shape the plan

### 2.1 `registerFlow`, `registerJSON`, `registerXML` are never called in serve

`SetupServe` (`internal/bindings/serve.go:39-134`) omits all three. The comment at
`serve.go:126-128` claims *"Timers and net/param helpers stay available"* — this is
**false**. Missing in serve mode today:

- `k.http_request` — a serve script cannot make an outbound HTTP call at all
- `k.json_parse` / `k.json_save` / … — no JSON parsing
- `k.xml_parse` / `k.xml_root` / … — no XML parsing
- `k.timer_start` / `k.timer_stop` — error stubs
- `k.param_get` / `k.param_set` — absent
- `k.net_ok`, `k.locale`, `k.ping` — absent
- `k.yield`, `k.error` — absent

Run mode has all of these. Serve mode — the mode you would use for a flow — has none.

### 2.2 `k.sleep` is a no-op

`serve.go:61-65` checks its argument and returns without sleeping or yielding. Any
polling loop written today spins a CPU core at 100%.

### 2.3 `k.exec` and `k.assign(table)` are broken

Both raise `"no session available"` (`flow.go:582`, `:663`) for no good reason in
serve mode. String targets of `k.assign` work.

### 2.4 Admission control fails by round-robin luck

`Worker.CallHTTP` (`internal/server/worker.go:141-202`) takes `w.mu` and, if already
held, returns `503 worker busy` immediately. Workers are leased by a round-robin
cursor (`server.go:502-512`) — *not* a free-list — so a request 503s whenever the
cursor happens to land on a busy worker. There is no queue, retry, or `Retry-After`.

Worse: a WS/TCP connection leases a worker **for its entire lifetime**
(`server.go:348`). With `--workers 1` and one open WS connection, *all* HTTP
requests get 503.

### 2.5 `k.quit` does not stop the server

`serve.go:68-72` sets `app.quitting`, but `Server.Run` only watches `ctx.Done()`.

### 2.6 `coroutine` is not in the sandbox whitelist

`internal/vm/vm.go:44-54` opens only Base, Table, String, Math, OS, Debug. A Lua
script **cannot spawn its own thread**. Background work must be created from Go via
`L.NewThread` — which is exactly what run mode's `RunAsync` does, and what the
timer registry in Phase 1 will do.

---

## 3. Phase 0 — serve-mode correctness (no new API surface)

Prerequisites. Phase 1 primitives are not usable until these land.

| # | Change | Location |
|---|--------|----------|
| 0.1 | Call `registerJSON` + `registerXML` from `SetupServe` — pure parsers, mode-independent | `serve.go:103-105` |
| 0.2 | Correct the false comment about timers/net/param helpers | `serve.go:126-128` |
| 0.3 | Make `k.sleep` really sleep (`time.Sleep`), matching `runBlocking`'s existing serve behaviour | `serve.go:61-65` |
| 0.4 | Fix `k.exec` and `k.assign(table)` | `flow.go:582`, `:663` |
| 0.5 | Replace instant `503 worker busy` with a bounded wait (~2s) + `Retry-After` header | `worker.go:146-149` |
| 0.6 | Make `k.quit` actually stop the server (poll `Quitting()` in `Run`, or trigger shutdown) | `serve.go:68-72` |
| 0.7 | Enable headless-safe `registerFlow` subset: `param_get/set`, `net_ok`, `locale`, `ping`, `yield`, `error` | `serve.go:103-105` |

**Why 0.5 matters more than its size suggests**: with blocking primitives, four
simultaneous slow calls turn every subsequent request into a 503. ~30 lines, and it
decides whether Phase 1 is usable in practice.

### Design note: keep `CallHTTP` on `CallByParam`

Converting `CallHTTP` to `NewThread` + `Resume` would be the "proper" fix and would
enable a *yielding* `k.sleep`. It is deliberately **not** in this plan:

- A yielding sleep needs a completion channel resumed from a timer goroutine that
  must re-acquire `w.mu` — real re-entrancy risk against a `closeOnce`-guarded worker.
- Serve already runs every DB/file call synchronously inline under the lock
  (`internal/bindings/files.go:459-476`), so blocking is the established convention.
- The benefit is latency, not correctness, and Phase 0.5 removes the failure mode.

**Consequence**: `k.yield` only works in WS/TCP handlers (which *are* coroutines).
Calling it from an HTTP handler will error. Documented, not silently broken.

---

## 4. Phase 1 — flow primitives

| Binding | Node-RED analogue | Notes |
|---------|-------------------|-------|
| `k.timer_start(id, ms[, repeats])` | **inject** (interval) | Go-side timer registry in `internal/server`, mirroring the `WSHub` lifecycle. Fires global `id` on a **leased** worker under `w.mu`. Must pin to a single worker or it fires `--workers` times per tick |
| `k.timer_stop(id)` | inject stop | Drop from the registry |
| `k.http_request(opts)` | **http request** | Already implemented at `flow.go:318-409` but unreachable in serve. Needs a no-session path; lower the 30s default timeout |
| `k.template(str, tbl)` | **template** | `{{key}}` and `{{#list}}…{{/list}}`. No dependency, ~60 lines. KALUA has no string interpolation at all |
| `k.ws.list()` / `k.ws.count()` | (hubs) | Scripts cannot enumerate connections today; needed for connection-count branching |
| `k.shared.set(k, v, ttl_ms)` | context store | Optional 3rd argument, in-memory lazy expiry. Stays RAM-only (no disk) per scope decision |

### Node-RED nodes deliberately **not** implemented

| Node | Reason |
|------|--------|
| `switch`, `change`, `range`, `filter`, `rbe`, `sort` | Low value in a text-first runtime — plain Lua is a better fit than a declarative config node. Per `AGENTS.md`: prefer `k.*`/expression functions over generic Lua, but a declarative node is worse than code |
| `split`, `join`, `batch` | Real cases already covered by `k.csv_*` and `k.json_to_rows` |
| `debug` | `k.print` + `--verbose` already covers it |
| `function` | The entire Lua script *is* the function node — already maximal |
| `exec` | Correctly absent; `os.execute` is removed from the sandbox by design |
| `link in`/`link out`, `subflow` | Would need multi-file `require`, which is removed from the sandbox (`vm.go:14-28`) |
| `udp in`/`udp out` | Low value for business apps |
| JSONata expressions | New query language to specify, sandbox, and document |

---

## 5. Phase 2 — MQTT client

**Dependency**: `github.com/eclipse/paho.mqtt.golang` — de facto standard, QoS 0/1/2,
auto-reconnect, callback handler that maps cleanly onto the existing `handle_ws`
dispatch path. Adds `golang.org/x/sync` and `google.golang.org/protobuf`; acceptable
given pgx/mssql/modernc-sqlite are already in the tree.

```lua
k.mqtt_connect{broker, port, user, password, client_id, tls} -> handle
k.mqtt_subscribe(handle, topic[, qos])                    -- 'mqtt in'
k.mqtt_publish(handle, topic, payload[, qos, retain])     -- 'mqtt out'
k.mqtt_unsubscribe(handle, topic)
k.mqtt_close(handle)
k.mqtt_on(pattern, fn_name)                               -- inbound dispatch
```

**Implementation**: an `MQTTHub` in `internal/server` alongside `WSHub` — inbound
message → lease worker → `L.NewThread` + `Resume` the registered handler, the same
path as `Worker.CallWS`. Credentials live in Lua source.

**Sequencing**: ship `--allow-net` (P0 in `kalua_security_plan.md`) *before* this
phase, so the new outbound channel arrives already constrained rather than after.

---

## 6. Phase 3 — Zebra RFID (ZIOTC) reader input

Hardware-specific input functions for Zebra **FX Series** (FX7500, FX9600, FXR90)
and **ATR Series** (ATR7000) fixed readers, speaking the **Zebra IoT Connector**
(ZIOTC) protocol. Serve mode only.

### 6.1 Scope

Reader input only — tag data ingestion, inventory control, and health. Tag
*writing*, encoding, and any Zebra-specific workflow logic are out of scope.
Everything is exposed under `k.rfid_*` and is transport-blind: the hub sits above a
`ReaderTransport` interface, so the WS and MQTT paths share one buffering and
binding surface.

### 6.2 The transport question — WS is a real ZIOTC endpoint

ZIOTC exposes four endpoint types: **MQTT**, **REST** (control), **HTTP-POST**
(data), and **WebSocket**. The reader's WS and TCP/IP endpoints are *listeners* —
the reader listens, the application dials in. WebSocket requires reader firmware
**≥ 3.24.X**.

This is the key architectural consequence, and it is easy to get wrong: the
existing `internal/server/ws.go` is a **server**, so it is the wrong thing to reuse.
KALUA must act as an outbound **client**, and no `k.*` binding dials out over
WebSocket today.

| Path | Direction | New dependency | Notes |
|------|-----------|----------------|-------|
| **WS** (no broker) | KALUA dials `wss://<reader>:<port>` | **none** — `coder/websocket` `Dial` (already vendored) | Tags inbound, control JSON back on the same socket |
| **MQTT** (broker) | KALUA subscribes/publishes | paho (Phase 2) | `zebra/<r>/data`, `/control`, `/control-resp` |

Both are KALUA-as-client, so both need new outbound code behind one interface:

```go
type ReaderTransport interface { // internal/server/rfid.go
    Connect(ctx context.Context, url string) error
    Publish(topic string, payload []byte) error // control
    Events() <-chan []byte                     // inbound batches
    Close() error
}
```

`WSReader` (zero new deps) and `MQTTReader` (paho) implement it. `control-resp`
correlation on `command_id` is the main thing MQTT adds over WS.

### 6.3 Verified tag-data payload

The ZIOTC tag-data event is a **JSON array of events**, one per tag read, with
fields nested under `data`:

```json
[{"data":{"CRC":"518e","PC":"3000","antenna":1,"channel":927.25,"eventNum":194,
          "format":"epc","idHex":"ad99160040aac99525000025","peakRssi":-54,
          "phase":0.0,"reads":9},
  "timestamp":"2023-02-24T10:14:42.851+0000","type":"INVENTORY"}]
```

Notes for the parser:

- The envelope is an **array**, not `{readerName, tags:[...]}`. A single message
  carries a **batch** of tags, so one frame ≠ one tag.
- Field names differ from most online examples: `data.peakRssi` (not `rssi`),
  `data.antenna` (not `antennaId`), `data.idHex` (nested, not top-level).
- `data.reads` is a seen-count, useful for duplicate suppression. `data.format`
  distinguishes EPC from other memory banks. `data.CRC` / `data.PC` carry protocol
  control state. `data.phase` is present on phased-array (ATR) events.
- `timestamp` is a **string with a `+0000` offset**, not RFC 3339 `Z` and not
  epoch millis.
- Operating modes may add a `userDefined` object; the parser must tolerate it.

**Parser must be tolerant**: accept both array and single-object frames, unwrap
`data.*` with a top-level `idHex` fallback, and skip unknown keys rather than
erroring. Fixtures should be captured from real hardware rather than transcribed
from vendor docs — see §6.9.

### 6.4 Filter at the reader, not in KALUA

Realistic throughput is roughly **120 reads/sec**; readers are tuned to stay well
below the rate at which KALUA could absorb every tag. Reader-side filtering is
therefore the primary control, and the reason a per-tag Lua handler is untenable:

| Lever | Purpose |
|-------|---------|
| `filter: {value, match:"prefix"\|"regex", operation:"include"\|"exclude"}` | only matching EPCs leave the reader |
| `rssiFilter: {threshold: -86}` | drop weak reads |
| `tagMetaData: ["EPC","ANTENNA","RSSI","SEEN_COUNT"]` | shrink payload (`SIMPLE` reports metadata only) |
| `reportFilter: {duration, type:"RADIO_WIDE"\|"PER_ANTENNA"}` | throttle reports |
| `antennaStopCondition: {type:"DURATION", value}` | per-antenna dwell — the main throughput knob |
| `query: {session, tagPopulation, sel, target}` | singulation state; `AB` reads much faster than `A` |

All settable through `k.rfid_start` / `k.rfid_set_mode`.

### 6.5 Hub: bounded buffer, never per-tag dispatch

`CallWS` / `CallTCP` take `w.mu.Lock()` and run a full `L.Resume` **per event**
(`internal/server/worker.go:311-360`). At 120 tags/sec that pins one worker and
overflows the 256-slot inbox, which drops **silently**. A per-tag dispatch model
cannot work here.

```
reader-side filter → RFIDHub ring buffer (Go, bounded) → one Lua call per batch
```

Backpressure (blocking or retrying the reader) and *detailed* drop accounting are
deferred (see §11). The buffer is still **hard-capped** — a misconfigured reader
filter should cost a bounded drop, never unbounded memory. A minimal counter is not
deferred: `k.rfid_count` reports `{buffered, dropped_since}` from R0 so a bad filter
is diagnosable rather than silent. Retrofitting that later would mean guessing
whether earlier drops were data loss.

### 6.6 API

Serve only, registered as dotted `e.register("rfid.connect", …)` names so the
checker validates them (same contract as §6.6).

```lua
k.rfid_connect{ url = "wss://10.0.0.5:8000/rfid" }      -- WS transport
k.rfid_disconnect{reader}                              -- implicit on close
k.rfid_readers()                                       -- name → {connected, mode, buffered, dropped_since}

k.rfid_status{reader}                                  -- health, GET /api/management/status
k.rfid_set_mode{reader, mode, opts}                    -- SIMPLE|INVENTORY|PORTAL|CONVEYOR|CUSTOM
k.rfid_start{reader, opts}                             -- {type, antennas, transmitPower, filter, rssiFilter, …}
k.rfid_stop{reader}                                    -- stops the inventory session

k.rfid_count{reader}                                   -- {buffered, dropped_since}
k.rfid_drain{reader, max}                              -- {n, tags={...}, dropped=n}
k.rfid_flush{reader}                                   -- drop the buffer
k.rfid_on(reader, "read", fn_name, {batch=25, every_ms=500})
```

Normalized tag record: `{epc, format, antenna, rssi, reads, crc, pc, phase, ts,
type, raw}`.

`k.rfid_on` fires **with a batch**, never per tag — it triggers when `batch` tags
are buffered or `every_ms` elapses, and hands the named function `{tags={…}, n=…}`.
This is the "script decides what to do with tags" surface, expressed in a way the
worker model can survive.

### 6.7 Phasing

| Step | Work | Blocked by |
|------|------|-------------|
| **R0** | `RFIDHub` + capped ring buffer, tolerant parser, `count`/`drain`/`flush` | nothing — device-free |
| **R1** | `WSReader` transport (`coder/websocket` `Dial`), `start`/`stop`/`set_mode`/`status` over REST, `k.rfid_on` batch dispatch | serve Phase 0+1 |
| **R2** | `MQTTReader` transport + `control-resp` correlation on `command_id` | serve Phase 2 (paho) |
| **R3** | WS egress to dashboards | nothing |

R0 is fully testable with no hardware and should land first regardless of the
transport choice.

### 6.8 Security

- **Gate every `k.rfid_*` behind `--allow-net`**, with reader addresses on the
  allowlist. This raises the priority of `--allow-net` (P0 in
  `kalua_security_plan.md`) to before R1, consistent with the MQTT phase.
- The WS and MQTT paths both dial **out** to LAN hardware and carry credentials;
  reader/broker credentials live in Lua source, the same exposure as Phase 2.
- **Do not** ship an inbound `POST /rfid/*` ingest endpoint. The WS and MQTT paths
  need no inbound exposure on KALUA, so that surface is avoidable; if HTTP-POST
  ingest is ever added it must land *after* the P0 auth middleware.
- Treat `epc` and the rest of the tag record as **untrusted input** — they
  originate from a device and commonly flow into `k.db_*` writes.

### 6.9 Open items

- **Tag captures from real hardware** (one frame each from an FX and an ATR7000)
  are needed to write the parser fixtures. Firmware ≥ 3.24.X has format variants
  that vendor docs do not enumerate.
- **ATR7000 spatial position** (the `DIRECTIONALITY`/RTLS X,Y output) is
  firmware-specific and unverified. The parser carries `phase` and preserves `raw`,
  so the field can be surfaced once a capture exists, but no spatial model is
  specified here.
- **Control-plane default** — REST (`k.http_request` → `POST /api/control`) versus
  control over the same WebSocket. REST is more debuggable; the socket is fewer
  moving parts and avoids a second auth path. Undecided.
- **WebSocket URL and port** are reader-configured per endpoint and vary by
  firmware; discovery is out of scope and readers must be declared in Lua.

---

## 7. Phase 4 — Structured logging to SQLite (`klog.db`)

Serve emits a queryable structured log into a SQLite file the process creates and
maintains itself. Off by default; enabled and tuned entirely by flag. This phase
adds an *observability* surface — it changes no request handling.

### 7.1 Why SQLite and not a file

A flat text log cannot answer "which requests 5xx'd last hour" or "how long did
p99 take". SQLite gives indexed time-range queries over a file that needs no
server, no rotation script, and no agent. Critically, **no new dependency**:
`modernc.org/sqlite v1.57.0` is already a direct dependency (used by `k.connect_*`),
SQLite is 3.53.3 with json1 and WAL available, and it is pure Go — so `CGO_ENABLED=0`
release builds stay static.

### 7.2 The single-writer constraint

SQLite permits **one writer**. With `--workers N` there are N concurrent goroutines,
so a naive per-worker handle produces `SQLITE_BUSY` storms. The design funnels
everything through one writer goroutine:

```
N workers ──► Emit() ──► truncate(16 KiB) ──► chan Record (cap 1024)
                                                      │
                                            single writer goroutine
                                             batch ≤64 or ≤200 ms
                                                      ▼
                                    tx: multi-row INSERT
                                    hourly: age prune + size cap
```

- One `*sql.DB`, owned **exclusively** by the writer goroutine, with
  `SetMaxOpenConns(1)` as a guard against accidental future concurrency.
- Channel full ⇒ **drop the record and increment `klog_meta.dropped_total`**. Logging
  must never add latency to a request; a bounded drop is the correct failure.
- Channel is 1024 rather than 4096 so the pathological worst case
  (1024 × 16 KiB ≈ 16 MB) stays bounded; typical records are ~200 B (~200 KB).
- Schema is created **eagerly** at startup when the flag is on, so a bad path fails
  fast rather than at the first log line. `klog_meta.schema_version` allows future
  migrations.

### 7.3 Schema

```sql
PRAGMA journal_mode=WAL;        -- concurrent readers while the writer commits
PRAGMA busy_timeout=5000;
PRAGMA synchronous=NORMAL;

CREATE TABLE IF NOT EXISTS klog (
  id          INTEGER PRIMARY KEY,   -- rowid; append-only, so no AUTOINCREMENT
  ts          TEXT    NOT NULL,      -- RFC3339Nano, UTC
  ts_unix_ms  INTEGER NOT NULL,      -- indexed; cheap range predicates
  level       INTEGER NOT NULL,      -- 0..4
  category    TEXT    NOT NULL,      -- http|ws|tcp|worker|app|db|rfid|lifecycle
  worker      INTEGER,               -- worker id; NULL = process-level
  session     TEXT,                  -- WS/TCP client id
  remote_addr TEXT,
  message     TEXT    NOT NULL,      -- the existing printf text
  data        TEXT                   -- valid JSON object of structured fields
);
CREATE INDEX IF NOT EXISTS klog_ts        ON klog(ts_unix_ms);
CREATE INDEX IF NOT EXISTS klog_level_cat ON klog(level, category);
CREATE TABLE IF NOT EXISTS klog_meta (
  key   TEXT PRIMARY KEY,
  value TEXT                              -- schema_version, created_at, dropped_total
);
```

Fixed columns carry the hot query paths; the JSON `data` blob carries per-category
extras so `json_extract(data,'$.status') >= 500` works **without a schema migration
per category**. `id` is a plain rowid deliberately — the table is append-only and
never needs a gap-free monotonic sequence.

### 7.4 Per-row cap — 16 KiB

A script can log an arbitrarily large string (`k.log.info(string.rep("x", 1e7))`).
Two consequences, so the cap is enforced in **`Emit`, before enqueueing** — one
choke point covering both host instrumentation and `k.log.*`, and the bounded
channel can then never hold an oversized record:

- **Memory** — an uncapped record in the channel multiplies by the channel depth.
- **Storage** — a single huge row inflates `klog.db` and later page reads.

The budget is `len(message) + len(data) <= log_max_row`:

| Field | On overflow |
|---|---|
`message` | Plain text, so truncate at a rune boundary and append `…[truncated]`. |
`data` | **Must stay valid JSON.** Truncating mid-token makes `json_extract` *error* for that row. Replace with a valid envelope: `{"_truncated":true,"_orig_bytes":N,"_preview":"…"}` |

If `message` leaves under 64 bytes of budget, `data` is dropped entirely and the
marker folds into `message`.

### 7.5 Retention — age plus a size backstop

Both run on the same hourly tick, size first:

```
--log-retain 7d    age prune:   DELETE FROM klog WHERE ts_unix_ms < cutoff
--log-max-mb 50    size cap:    page_count * page_size (includes WAL, no VACUUM needed)
```

When over the cap, delete oldest in **one** statement down to a **low-water mark at
80 % of cap**. Without the mark, every tick would trim a handful of rows and cause
write amplification; the mark means the next prune is at least 20 % of the file away.
`0` disables either bound. The size cap is the backstop for a runaway
`category='app'` writer that the age window alone would not catch.

### 7.6 API

Script-facing, for domain events that should be queryable rather than grepped:

```lua
k.log.info("order placed", {order_id = 42, total = 99.5})
k.log.error("payment failed", {reason = "timeout"})
k.log.stats()   -- {rows, dropped, oldest, newest, path}
```

`k.log.info|warn|error|debug|trace` + `k.log.stats`. The optional second argument is
a table of structured fields that becomes the `data` JSON. Registered in
`registerKnown` **and** `api_doc.go` — `internal/bindings/api_doc_test.go` enforces
bidirectional sync, and `KALUA check` rejects any `k.*` access missing from the
registry.

### 7.7 The sink is a wrapper, not a refactor

`bindings.Logger` (`bindings.go:95`) is an **interface**, so `SQLLogger` satisfies it
and **every existing call site keeps working unchanged**:

```go
type SQLLogger struct {
    inner bindings.Logger   // host.Logger — stdout/stderr behaviour unchanged
    sink  *klog.Sink
    cat   string
}
func (l *SQLLogger) Printf(f string, a ...any) { l.emit(klog.LevelInfo, f, a, nil) }
func (l *SQLLogger) Emit(lv klog.Level, cat string, fields map[string]any)
```

The database **complements** stdout; it never replaces it. `k.print`
(`serve.go:56`) is deliberately **excluded** — it stays stdout-only, so a chatty
script cannot dominate the table.

### 7.8 Package layout — the import-cycle constraint

The sink **cannot** live in `internal/server`: `server.go:18` already imports
`internal/bindings`, so a sink there would make `k.log.*` in `bindings` cyclic.
`internal/common` is the existing cycle-avoidance precedent, so the sink gets its
own **leaf** package importing only stdlib + `modernc.org/sqlite`:

| File | Build tag | Role |
|---|---|---|
`internal/klog/klog.go` | — | `Record`, `Sink`, `Level`, `truncate()` (pure, no sqlite) |
`internal/klog/size.go` | — | `ParseSize`, `ParseRetention` |
`internal/klog/sqlite.go` | `!wasm` | `Open`, `Emit`, `Flush`, `Prune`, `Stats`, `Close` |
`internal/klog/sqlite_wasm.go` | `js && wasm` | no-op sink |
`internal/bindings/sqllogger.go` | `!wasm` | `SQLLogger` + `Emit` |
`internal/bindings/log_ops.go` | `!wasm` | `k.log.*` (serve mode) |
`internal/cli/logflags.go` | — | `sizeValue` / `durValue` `flag.Value` adapters |

The `!wasm` tag is required and follows the `db.go` precedent: WASM uses
`ncruces/go-sqlite3` (wa-sqlite), not `modernc`. The pure files compile everywhere.
The `host.Level` widening is plain Go and is safe in both builds.

### 7.9 Level widening

`host.Level` is currently `Error=0, Info=1, Trace=2` and is confined to `log.go:13-15`
plus one `SetLevel` call (`smoketest.go:79`), so widening is low-risk:

```go
const (
    LevelError Level = iota // 0
    LevelWarn               // 1
    LevelInfo               // 2  (default; was 1)
    LevelDebug              // 3
    LevelTrace              // 4  (was 2)
)
```

`-v` still maps to `LevelTrace`. **Regression to fix:** `Warnf` currently gates on
`l.level >= LevelInfo` (`log.go:70`); with `LevelWarn` inserted that gate is wrong
and must become `>= LevelWarn`, or warnings silently disappear at debug verbosity.

### 7.10 Instrumented call sites

Serve currently has only four logger call sites, so the structured surface is small
and explicit:

| Location | Category | `data` fields |
|---|---|---|
`server.go:316` `handle_http` | `http` | `method, path, status, duration_ms, bytes, query, remote_addr` |
`server.go:288` read body | `http` | `path, bytes, error` |
`worker.go:331` WS handler | `ws` | `session, event, error` |
`worker.go:366` TCP handler | `tcp` | `session, event, error` |
`worker.go:427` shutdown | `lifecycle` | `error` |
`server.go:128` listening | `lifecycle` | `host, port, workers, mode` |
worker lease / `Reload` / pool swap | `lifecycle` | `worker, event, pid, generation` |
named DB connect/query | `db` | `handle, op, rows, duration_ms` |

### 7.11 Security — the log DB must be script-denied

`workdirOf` (`bindings.go:470`) is the **process CWD**, which is also the sandbox
root, so `klog.db` in CWD would otherwise be readable by the very script it logs.
A `DenyFS` list is enforced in **three** places, because `resolvePath` alone is
insufficient — two vectors bypass it:

| Vector | Current state | Fix |
|---|---|---|
Files (`file_load`/`save`/`copy`/`delete`/`list`…), formats (`csv`/`ini`/`yaml`/`xml` via `loadVia`/`saveVia`), `connect_sqlite` | all funnel through `resolvePath` (`files.go:536`) | **deny-list in `resolvePath`** |
**`k.connect_db("sqlite://klog.db")`** | **no `resolvePath` at all** — `db.go:46-64` hands the DSN straight to `sql.Open` | **route sqlite DSNs through `resolvePath`** |
**`k.zip_extract(zip, ".")` with a member named `klog.db`** | member target written via `os.Create` after only a dir check (`files.go:357-360`) | **check each target against the deny-list** |

`k.connect_db` is a **pre-existing sandbox escape** — a script can open any SQLite
file on disk and `SELECT` from it. Tracked as **C7** in `kalua_security_plan.md`; the
fix ships here but the finding is not logging's to own.

Mechanics: `Options.DenyFS` + `denyFSOf()` mirroring `allowFSOf()`; `Env.denyFS` set
at all three construction sites (`serve.go:94`, `setup_native.go:21`,
`wasm_setup.go:19`); in `resolvePath` the deny test runs **before** the allow-roots
loop. Comparison is on the symlink-resolved absolute path, so `./klog.db`,
`klog.db` and `sub/../klog.db` are all denied while `klog.db.bak` and `mylog.db`
stay allowed — exact-file denial, no wildcard, no directory denial, no over-blocking.

### 7.12 CLI and KALUA.INI

```
kalua serve app.lua --log-sqlite --log-level debug --log-retain 7d \
                    --log-max-mb 50 --log-max-row 16k
```

| Flag | Type | Default | Accepted forms | `0` means |
|---|---|---|---|---|
`--log-sqlite` | bool | off | `1`/`0` in INI | — |
`--log-level` | enum | `info` | `error\|warn\|info\|debug\|trace` | — |
`--log-retain` | `durValue` | `7d` | `7d` `48h` `90m` `1h30m` | keep forever |
`--log-max-mb` | `sizeValue` | `50` | `50` `512m` `2g` | unlimited |
`--log-max-row` | `sizeValue` | `16k` | `16k` `16kb` `65536` | unlimited |

`--log-retain 7d` would be the **first duration flag in the codebase**, and
`time.ParseDuration` rejects `d` (it stops at `h`), so `ParseRetention` extends it
by rewriting the day component to `24h` before delegating — which also makes
`1d12h` work while all native forms pass through. Bare numbers are bytes; `0`/empty
means unlimited.

`ApplyFlags` (`config.go:142`) drives INI values through `fs.Set`, so **any custom
`flag.Value` gets INI support for free** — only `boolFlag` is special-cased, and
`Norm` (`config.go:31`) already folds `-`/`_` and case. All five keys ride `[SERVE]`
via `config.ApplyFlags` (`internal/cli/cli.go:504`), CLI beats INI, matching the
existing `v`↔`verbose` alias pattern.

```ini
[SERVE]
; Structured logging to klog.db in the current working directory.
; log-sqlite  = 1            ; enable the klog.db structured log
; log-level   = info          ; error|warn|info|debug|trace
; log-retain  = 7d            ; delete rows older than this (0 = keep forever)
; log-max-mb  = 50            ; size cap backstop (0 = unlimited)
; log-max-row = 16k           ; per-row cap on message+data (0 = unlimited)
```

Values are parsed and validated at startup **whether or not `--log-sqlite` is set**,
so a typo fails loudly rather than silently doing nothing — this matches the tested
behaviour in `internal/cli/cli_test.go` (`TestRun_Ini_*`), where a bad INI value
errors even with the feature off. Invalid value → stderr + `ExitUsage` (2); via INI
the message is wrapped as `KALUA.INI [SERVE] log-max-row="bogus": …`.

**Path** is fixed: `filepath.Join(pwd, "klog.db")` — always that name, always the
directory `kalua` was started from, ignoring `--ini` and the script's location.

### 7.13 Shutdown

`Shutdown(ctx)` drains the channel and closes the handle on `SIGTERM`, `SIGINT`, and
`Server.Reload()`, so a hot-reload does not leave a half-written batch behind.

### 7.14 Phasing

| Step | Work | Depends on |
|---|---|---|
L0 | `internal/klog` package: types, `truncate`, `ParseSize`/`ParseRetention`, `!wasm` sink, wasm no-op | — |
L1 | `host.Level` widening + `Warnf` gate fix | — |
L2 | Hardening: `DenyFS` + `resolvePath` + `zip_extract` + `connect_db` (closes **C7**) | — |
L3 | `internal/bindings/sqllogger.go` | L0, L1 |
L4 | Instrument the 8 call sites | L3 |
L5 | `k.log.*` + `registerKnown` + `api_doc.go` | L3 |
L6 | CLI flags + adapters + validation + INI + `KALUA.ini.sample` | L0 |
L7 | Tests | L0–L6 |
L8 | Docs | L7 |

L2 is sequenced early and independently: it is a standalone security fix, and
deferring it would leave `klog.db` readable by every app that turns logging on.

### 7.15 Tests

- `klog/sqlite_test.go` — schema creation, batched flush, drop counter, age prune,
  size cap + low-water (assert no re-trim per tick), 16 KiB cap (message truncation,
  `data` still valid JSON, `_truncated` envelope, combined budget), drain-on-`Close`,
  WAL, `Stats`.
- `klog/size_test.go` — `ParseSize` (`16k` `16kb` `65536` `1m` `1g` `0` `""` invalid);
  `ParseRetention` (`7d` `1d12h` `48h` `90m` `0` invalid).
- `internal/bindings/deny_test.go` — `file_load`/`csv_load` denied; `zip_extract`
  member denied; **`connect_db("sqlite://klog.db")` denied (C7 regression)**;
  in-workdir `app.db` still allowed; `klog.db.bak` allowed.
- `internal/bindings/sqllogger_test.go` — interface conformance, level gating incl.
  the `Warnf` regression, stdout still receives output.
- `internal/server/log_e2e_test.go` — real HTTP request → row with correct
  `status`/`duration_ms`; `k.log.info` from Lua → `category='app'`; `k.print` →
  stdout and **no** DB row.
- `internal/cli/logflags_test.go` — `--log-max-row 1k` overrides INI `16k`; INI-only
  `log_max_row = 4k` applies (underscore form); bad CLI value → exit 2; bad INI
  value → exit 2 with `KALUA.INI [SERVE]` in stderr.
- `go test -race ./internal/server/... ./internal/klog/...` with concurrent workers;
  `CGO_ENABLED=0 go build ./cmd/KALUA` still static; WASM still builds.

---

## 9. Out of scope

Node-RED `k.node.*` routing registry · link nodes · subflows · visual editor ·
embedded MQTT broker · JSONata · cron scheduling (interval only) · RFID tag writing/encoding · ATR7000 spatial
modelling · RFID reader auto-discovery · log shipping/aggregation · log rotation to
sibling files · a log viewer UI · indexing `data` JSON columns per category.

*RFID reader input* is Phase 3 (§6), and *structured logging* is Phase 4 (§7); what remains out of scope is the
routing/wiring paradigm Node-RED uses rather than the state-machine model LangGraph
uses, and the log-file-management tooling an ops team would normally bolt on.

---

## 9. Defaults assumed

| Decision | Default | Alternative |
|----------|---------|-------------|
| MQTT library | `paho.mqtt.golang` | `mochi-co/mqtt` — much lighter, less battle-tested |
| Timers | interval only | cron needs `robfig/cron`; a Lua-side evaluator is possible but ugly |
| Optional Phase 1 trio | include `k.template`, `k.ws.list`, `k.shared` TTL | drop any independently — all three are small and self-contained |
| RFID transport (R1) | WebSocket, zero new deps | MQTT needs paho and a broker; do R2 only once a broker is confirmed |
| RFID tag dispatch | buffered batch, one Lua call per batch | per-tag dispatch would pin a worker at 120 tags/sec |
| Log DB path | fixed `./klog.db` in the process CWD | `--ini` and the script's directory are ignored; a configurable path risks the DB landing inside an `--allow-fs` root |
| Log driver | reuse `modernc.org/sqlite` | a second driver (e.g. `ncruces`) is WASM-only and would break `CGO_ENABLED=0` releases |
| Log buffer | drop-and-count on a full channel | blocking the request path would make an observability feature into an availability one |
| Log retention | age **and** size, both bounded | age alone misses a runaway `category='app'` writer; size alone loses the time window |
| Log per-row cap | 16 KiB on `message`+`data` combined | no cap lets one script call inflate memory via the channel and storage at once |
| Log stdout | database complements stdout | replacing it would break existing `--test`/log-scraping workflows for no gain |

---

## 10. Risks

- **Timer × workers** is the one genuinely subtle piece. Firing on a leased worker
  prevents races, but it must pin to a single worker or the callback runs
  `--workers` times per tick.
- **0.5 changes observable behaviour** — requests that previously got an instant 503
  now queue up to ~2s. Load characteristics shift; call it out in the commit.
- **0.1 exposes `k.json_*` / `k.xml_*` in serve for the first time**, so
  `make gen-api && make check-api` needs to reflect serve availability.
- **0.7 widens the serve surface** — `k.ping` and `k.net_ok` are new outbound probes.
- **MQTT credentials in Lua source** — raises the priority of `--allow-net`.
- **Registry drift is silent until `KALUA check` fails** on a *valid* script —
  the `api_doc.go` + `registerKnown` edits must move together in the same commit
  for every new `k.*` namespace.
- **RFID tag rate is a memory-safety risk, not just a performance one.** A
  misconfigured or omitted reader-side `filter` sends every tag to KALUA; the
  capped buffer bounds memory, but the drops are otherwise invisible. `k.rfid_count`
  must expose `dropped_since` from R0 — retrofitting that later means guessing
  whether earlier drops were data loss.
- **RFID firmware drift** — ZIOTC payload variants across firmware ≥ 3.24.X are
  not fully enumerated by vendor docs. The tolerant parser mitigates this, but
  fixtures must come from real captures (§6.9) or the first field mismatch will be
  found in production.
- **The log DB is in the sandbox root.** Putting `klog.db` in PWD means it is
  reachable by `k.file_load` unless the `DenyFS` list lands. Because `k.connect_db`
  and `k.zip_extract` both bypass `resolvePath` (§7.11), a deny-list in
  `resolvePath` alone would look correct while leaving the file readable. Enforce
  all three points or the feature is a net information-disclosure risk.
- **A full channel is silent data loss.** Dropping is the right call for latency, but
  it is invisible unless `dropped_total` is actually surfaced — the same lesson as
  the RFID buffer. Instrument it from L0, not later.
- **Logging must never become an availability dependency.** Any future change that
  lets a slow or blocked `klog.db` write delay a request reintroduces a much worse
  failure than the one being debugged. The drop path is the pressure valve that makes
  this safe; do not remove it in the name of completeness.
- **`WAL`/`-shm` siblings appear next to `klog.db`.** Age- and size-based retention
  trim rows, never the file, so all three files persist across restarts. Expected, but
  worth documenting before someone reports them as leftovers.

---

## 11. `k.*` available in `kalua serve` after implementation

**161 functions** — 110 present today, **51 new names**, **7 newly functional**.
(Group totals sum: 18+9+8+16+6+15+17+6+10+10+7+5+5+3+6+3+11+6 = 161.)

> Phase 3 adds 11 `k.rfid_*` names (144 → 155); Phase 4 adds 6 `k.log.*` names (155 → 161). The original sum expression
> carried a stray extra `+3` (17 terms for 16 groups); the corrected base is used above.

`✓` present today · `★` new · `▲` exists but broken/stubbed, fixed by this plan

## Flow / core — 18

`k.print`✓ `k.sleep`▲ `k.quit`▲ `k.on_error`✓ `k.assign`▲ `k.set`✓ `k.exec`▲
`k.error`★ `k.yield`★ `k.http_request`★ `k.template`★ `k.timer_start`▲
`k.timer_stop`▲ `k.param_get`★ `k.param_set`★ `k.net_ok`★ `k.locale`★ `k.ping`★

## JSON — 9 ★ (all new)

`k.json_parse` `k.json_string` `k.json_load` `k.json_save` `k.json_get`
`k.json_array_item` `k.json_count` `k.json_names` `k.is_null`

## XML — 8 ★ (all new)

`k.xml_parse` `k.xml_root` `k.xml_child` `k.xml_child_list` `k.xml_attr`
`k.xml_content` `k.xml_attrs` `k.xml_name`

## Data formats — 16 ✓

`k.csv_parse` `k.csv_string` `k.csv_load` `k.csv_save` ·
`k.ini_parse` `k.ini_string` `k.ini_load` `k.ini_save` `k.ini_read` `k.ini_write` ·
`k.yaml_parse` `k.yaml_string` `k.yaml_load` `k.yaml_save` ·
`k.xml_load` `k.xml_save`

## Result-set conversion — 6 ✓

`k.json_to_rows` `k.rows_to_json` `k.csv_to_rows` `k.rows_to_csv` `k.xml_to_rows` `k.rows_to_xml`

## Database — 15 ✓

`k.connect_db` `k.disconnect_db` `k.sql` `k.db_select` `k.db_insert` `k.db_update`
`k.db_delete` `k.db_proc` `k.db_kill_table` `k.rows` `k.tx_begin` `k.tx_commit`
`k.tx_rollback` `k.connect_sqlite` `k.disconnect_sqlite`

## Files & zip — 17 ✓

`k.file_open` `k.file_read` `k.file_read_line` `k.file_write` `k.file_close`
`k.file_load` `k.file_save` `k.file_copy` `k.file_move` `k.file_delete`
`k.file_exists` `k.file_mkdir` `k.file_list` `k.file_info` ·
`k.zip_list` `k.zip_add` `k.zip_extract`

## Sockets & SOAP — 6 ✓

`k.socket_open` `k.socket_write` `k.socket_read` `k.socket_read_line` `k.socket_close` `k.webservice_run`

## FTP — 10 ✓

`k.ftp_connect` `k.ftp_set_cwd` `k.ftp_get_file` `k.ftp_put_file` `k.ftp_file_exists`
`k.ftp_create_dir` `k.ftp_delete` `k.ftp_rename` `k.ftp_list` `k.ftp_disconnect`

## Email — 10 ✓

`k.smtp_connect` `k.smtp_send` `k.smtp_disconnect` ·
`k.pop3_connect` `k.pop3_stat` `k.pop3_list` `k.pop3_retr` `k.pop3_dele` `k.pop3_noop` `k.pop3_quit`

## Crypto — 7 ✓

`k.checksum` `k.encrypt` `k.decrypt` `k.crypt_symmetric` `k.crypt_asymmetric` `k.sign` `k.verify`

## Shared state — 5 (4 ✓, 1 ▲)

`k.shared.set`▲ *(now `set(k, v, ttl_ms)`)* `k.shared.get`✓ `k.shared.del`✓
`k.shared.keys`✓ `k.shared.incr`✓

## WebSocket — 5 (3 ✓, 2 ★)

`k.ws.broadcast`✓ `k.ws.send`✓ `k.ws.close`✓ `k.ws.list`★ `k.ws.count`★

## TCP — 3 ✓

`k.tcp.send` `k.tcp.close` `k.tcp.accept`

## MQTT — 6 ★ (all new)

`k.mqtt_connect` `k.mqtt_subscribe` `k.mqtt_publish` `k.mqtt_unsubscribe`
`k.mqtt_close` `k.mqtt_on`

## RFID — 11 ★ (all new)

`k.rfid_connect` `k.rfid_disconnect` `k.rfid_readers` `k.rfid_status`
`k.rfid_set_mode` `k.rfid_start` `k.rfid_stop` `k.rfid_count` `k.rfid_drain`
`k.rfid_flush` `k.rfid_on`

Phase 3, Zebra FX/ATR readers over ZIOTC. All gated behind `--allow-net`; see §6.

## Log — 6 ★ (all new)

`k.log.info` `k.log.warn` `k.log.error` `k.log.debug` `k.log.trace` `k.log.stats`

Phase 4. `k.print` is deliberately **not** in this group — it stays stdout-only
(§7.7). Writes to the script-denied `klog.db`; see §7.

## Debug — 3 ✓

`k.debug.stack` `k.debug.locals` `k.debug.trace`

---

## Present in serve but raise `"not available in serve mode"` (38)

Unchanged by this plan:

- `k.form.{new, show, close, on, clear, refresh, return_to}` (7)
- `k.ctrl.{set_value, get_value, set_property, get_property, textbox, button, label, combo, list, table, checkbox, radio, select_text, set_selection, get_selection, get_item_count, execute_event, image, chart, looper, grid}` (22)
- `k.set_property` `k.get_property`
- `k.msgbox` `k.popup`
- `k.status_{show, close, set, clear, progress}` (5)

## Never available in serve (browser-only, no serve meaning)

`k.clipboard_set` `k.clipboard_get` `k.bell` `k.screen_size` `k.pick_file`

---

## Non-`k` globals also available in serve

**`K.*` helpers (11)**: `K.EQ` `K.NEQ` `K.ADD` `K.eq` `K.ne` `K.add` `K.tonum`
`K.tostr` `K.truthy` `K.NULL` `K.is_null`

**87 expression globals** (flat, not under `k.*`):

- *String* — `left right middle length replace trim upper lower find string_count complete ascii charact base64_encode base64_decode urlencode urldecode encode decode full_encode jsonencode jsondecode xmlencode xmldecode guid extract_string set_string file_extract_part mltext`
- *Numeric* — `abs round floor ceiling power nth_root sqrt exp log log10 sin cos tan asin acos atan deg2rad rad2deg bitwise_and bitwise_or bitwise_xor random int_part dec_part mask_number val sum extractstringd`
- *Conditional* — `lookup yesno iif`
- *Date/time* — `sys_date sys_time day month year hour minute second add_days subtract_days date_diff datetime_add datetime_sub datetime_diff date_to_string time_to_string week_day week_number tick_count julian utc_to_local local_to_utc`
- *Conversion* — `tostr tonum todate strtodate boolstr`

**Other globals**: `ARGS` (array of `--arg` values) · `ERRORCODE` · `ERRORMSG`

**Lua stdlib**: base, table, string, math, os (read-only subset: `clock difftime date
time` only), debug. **`coroutine` is NOT opened** — scripts cannot spawn threads.

---

## Correction to `kalua_security_plan.md`

Finding **L5** in that document describes "`pbkdf2` iterations default 10,000" as
though `pbkdf2` were a standalone binding. It is not: `pbkdf2` is an **algorithm
option of `k.checksum`** (`internal/bindings/crypto.go:27,56-68`), selectable via
`alg = "pbkdf2"`. The low-default-iterations observation still holds, but the framing
should be corrected to "`k.checksum` with `alg="pbkdf2"`".
