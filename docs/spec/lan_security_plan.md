# KALUA LAN Security — Minimal Enhancement Plan

**Context**: `kalua run` now works over local network (binds to `0.0.0.0`). Phase A of the security plan is implemented (sandbox escapes closed, crypto/rand IDs, WS message caps, DenyFS). This plan covers the **minimal** additions to make LAN deployment reasonably secure.

---

## Threat Model for LAN

- **Trusted network**: Home lab, office LAN, isolated VLAN — no public internet exposure
- **Attacker**: Compromised device on same LAN, or malicious insider with network access
- **Assets**: App data, script logic, host filesystem (via `k.file_*`), DB connections, outbound network calls
- **Assumption**: Reverse proxy (Caddy/nginx) with TLS + auth sits at network edge for internet exposure

---

## Current State (Run Mode on LAN)

| Control | Status | Gap |
|---------|--------|-----|
| **WebSocket Origin** | `*` allowed when host ≠ loopback | **Critical** — any LAN origin can connect |
| **Authentication** | None (H1) | **Critical** — no session validation |
| **Session IDs** | `crypto/rand` 16 bytes | ✅ Fixed (P4) |
| **WS Message Cap** | 1 MiB default, configurable | ✅ Fixed (P6/H5) |
| **File Sandbox** | `resolvePath` + `AllowFS`/`DenyFS` | ✅ Fixed (C2/P11/L16) |
| **SQLite Sandbox** | `connect_db` routes through `resolvePath` | ✅ Fixed (C7) |
| **Outbound Net Calls** | Unrestricted (`k.http_request`, `k.socket_open`, FTP, SMTP, POP3, SOAP) | **High** — SSRF to internal services (P0) |
| **Rate Limiting** | None on outbound (L6) | **Medium** — DoS risk |
| **TLS** | Not supported in run mode (H9) | **Medium** — plaintext on LAN |

---

## Minimal Plan (3 Items)

### 1. Fix WebSocket Origin Check — **Required**

**Problem**: `internal/web/server.go:299-302` allows `OriginPatterns: ["*"]` when binding to non-loopback. Any LAN browser/tab can connect.

**Fix**: Restrict to the actual bound host/port + common LAN variants.

```go
// internal/web/server.go handleWS()
originPatterns := []string{"http://127.0.0.1:*", "http://localhost:*", "http://[::1]:*"}
if !isLoopbackHost(s.host) {
    // Build patterns from the bound address
    host := s.host
    if host == "0.0.0.0" || host == "::" {
        // Can't know all interfaces; allow localhost + explicit --allow-origin
        originPatterns = []string{"http://127.0.0.1:*", "http://localhost:*", "http://[::1]:*"}
    } else {
        originPatterns = []string{
            fmt.Sprintf("http://%s:*", host),
            fmt.Sprintf("http://%s:%d", host, s.port),
        }
    }
}
// Add any explicit --allow-origin entries
for _, o := range s.opts.AllowOrigin {
    originPatterns = append(originPatterns, o)
}
```

**New flag**: `--allow-origin` (repeatable) — extra Origin patterns to accept (e.g., `http://192.168.1.*`, `https://app.local`). Accepts glob-style `*` in host/port.

**Default behavior**: When binding to specific IP (e.g., `192.168.1.50`), only that host:port is allowed. When binding to `0.0.0.0`, defaults to localhost-only (secure by default) — user must explicitly add LAN origins via `--allow-origin`.

---

### 2. Optional Token Authentication — **Required for Multi-user LAN**

**Problem**: No auth on WebSocket upgrade (H1). Any user on LAN who knows the URL can open the app.

**Fix**: Add a simple shared-token mechanism. Two modes:

| Mode | Description |
|------|-------------|
| `--auth-token=<token>` | Single shared token (query param `?token=X` or header `X-Kalua-Token`) |
| `--auth-token-file=<path>` | Read token from file (supports rotation, secrets managers) |

**Implementation** (`internal/web/server.go`):

```go
// In Server struct
authToken string

// In handleWS(), before upgrade:
if s.authToken != "" {
    token := r.URL.Query().Get("token")
    if token == "" {
        token = r.Header.Get("X-Kalua-Token")
    }
    if token != s.authToken {
        w.Header().Set("Content-Type", "text/html; charset=utf-8")
        w.WriteHeader(http.StatusUnauthorized)
        fmt.Fprint(w, `<!DOCTYPE html><html><body><h1>401 Unauthorized</h1><p>Invalid or missing token.</p></body></html>`)
        return
    }
}
```

**Client side** (`internal/web/assets/app.js`): Read token from `localStorage` or URL param on load, send as `X-Kalua-Token` header on WS connect. First visit without token → prompt user to enter it (stored in `localStorage`).

**CLI flags**:
- `--auth-token` (string, empty = disabled)
- `--auth-token-file` (path, alternative to `--auth-token`)
- Both readable from `[RUN]` section in `KALUA.INI`

---

### 3. Document Secure LAN Deployment — **Required**

**Add to USER_GUIDE.md** a "Secure LAN Deployment" section with:

1. **Reverse proxy example (Caddy)** — TLS termination + basic auth:
   ```caddy
   kalua.local {
       reverse_proxy 192.168.1.50:9000
       basicauth {
           alice $2a$14$...
       }
   }
   ```

2. **Direct LAN without proxy** — Use `--auth-token` + `--allow-origin`:
   ```bash
   KALUA run app.lua --host 0.0.0.0 --port 9000 \
       --auth-token "$(openssl rand -hex 16)" \
       --allow-origin "http://192.168.1.*"
   ```

3. **INI file permissions** — `chmod 600 KALUA.INI` (may contain tokens/DSNs)

4. **Firewall** — Bind to specific LAN IP (`--host 192.168.1.50`), not `0.0.0.0`, when possible

---

## Out of Scope for "Minimal" (Defer to Phase B/C)

| Item | Reason |
|------|--------|
| `--allow-net` allowlist (P0) | Breaking change; needs compat decision (warn vs deny) |
| Rate limiting on outbound (P7/L6) | Lower priority; DoS requires script compromise first |
| TLS in run mode (P5/H9) | Reverse proxy handles this; adds cert management complexity |
| Builder auth (P9/L12) | Builder binds to `127.0.0.1` by default; not LAN-facing |
| CSP nonce (L1/Quick win 5) | Nice-to-have; inline styles required for dynamic layout |

---

## Implementation Checklist

- [ ] **Origin Fix** (`internal/web/server.go`)
  - [ ] Add `AllowOrigin []string` to `bindings.Options`
  - [ ] Add `--allow-origin` flag to `runCmd` (repeatable)
  - [ ] Apply from `[RUN]` INI section
  - [ ] Build origin patterns from bound host + explicit allow list
  - [ ] Default to localhost-only when binding to `0.0.0.0`
  - [ ] Tests: `internal/web/server_test.go` (origin accept/deny)

- [ ] **Token Auth** (`internal/web/server.go`, `internal/web/assets/app.js`)
  - [ ] Add `AuthToken string` to `bindings.Options` (or `Server` struct)
  - [ ] Add `--auth-token` / `--auth-token-file` flags to `runCmd`
  - [ ] Validate token on WS upgrade (query param + header)
  - [ ] Client: prompt for token, store in `localStorage`, send header
  - [ ] Tests: auth success/fail, token rotation via file

- [ ] **Documentation** (`docs/USER_GUIDE.md`)
  - [ ] "Secure LAN Deployment" section
  - [ ] Caddy + basic auth example
  - [ ] Direct LAN with token example
  - [ ] INI file permissions reminder

---

## Acceptance Criteria

1. `KALUA run app.lua --host 0.0.0.0 --port 9000` → **rejects** WS connections from LAN origins (only localhost allowed)
2. `KALUA run app.lua --host 192.168.1.50 --port 9000` → **accepts** only `http://192.168.1.50:9000` origin
3. `KALUA run app.lua --host 0.0.0.0 --allow-origin "http://192.168.1.*"` → **accepts** LAN origins matching pattern
4. `KALUA run app.lua --auth-token "secret123"` → **rejects** WS without `?token=secret123` or `X-Kalua-Token: secret123`
5. Token from `--auth-token-file` works and can be rotated without restart
6. All existing tests pass; new tests cover origin/auth logic

---

## References

- `kalua_security_plan.md` — Full threat model and Phase A/B/C breakdown
- `internal/web/server.go:255-382` — WebSocket upgrade handler
- `internal/web/assets/app.js` — Client WS connection logic
- `internal/cli/cli.go:107-279` — `runCmd` flag definitions
- `internal/config/config.go` — INI parsing and `ApplyFlags`