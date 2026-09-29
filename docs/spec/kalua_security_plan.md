# KALUA Security Plan

**Threat Model**: Intranet apps only. Internet exposure only via reverse proxy (Caddy/nginx) with TLS termination, auth, rate limiting.

**Status**: Assessment only — not yet implemented. **C7** and **L16** are scoped and
scheduled in `kalua-serve-enhancements.md` §8.11 (Phase 5, logging), which also
lands the **P11** deny-list mitigation. See that section for the C7/L16/P11 details.

---

## 🔴 CRITICAL (RCE / Data Exfiltration / Auth Bypass)

| # | Issue | Location | Impact |
|---|-------|----------|--------|
| **C1** | Arbitrary SQL execution via `k.sql()` | `db.go:111-127` | Scripts can execute any SQL including `DROP TABLE`, `COPY ... TO PROGRAM` (Postgres), `LOAD DATA INFILE` (MySQL), `xp_cmdshell` (MSSQL). No query allowlist. If script is compromised (XSS, supply chain), full DB takeover. |
| **C2** | Arbitrary file system access via `k.file_*` | `files.go:35-198` | `file_open("r+")`, `file_write`, `file_save`, `file_delete`, `file_mkdir` with `resolvePath` sandbox. **But**: symlink traversal via `evalSymlinksBestEffort` only resolves *existing* prefix — a TOCTOU race between stat and open allows escape if attacker can plant symlinks. Also `AllowFS` roots can be set to `/` via CLI/INI. |
| **C3** | Arbitrary outbound HTTP via `k.http_request()` | `flow.go:318-409` | SSRF to internal services (metadata APIs, cloud IMDS, internal admin panels). No URL allowlist, no private IP blocking. In intranet, can hit other services. |
| **C4** | Arbitrary TCP sockets via `k.socket_open()` + `k.tcp_accept()` | `comm.go:34-50`, `serve.go:260-271` | Scripts can open raw TCP connections to any host:port (internal DBs, Redis, etcd, SSH). `k.tcp_accept()` in serve mode binds a listener — if exposed via reverse proxy misconfig, external RCE. |
| **C5** | FTP/SMTP/POP3/SOAP client bindings | `ftp.go`, `smtp.go`, `pop3.go`, `soap.go` | Same SSRF surface as `http_request` but for other protocols. Credentials passed in script (cleartext in Lua source). |
| **C6** | `k.db_proc()` executes stored procedures | `db.go:425-450` | Can invoke arbitrary stored procs including admin ones. No allowlist. |
| **C7** | `k.connect_db()` bypasses the filesystem sandbox | `db.go:46-64` | Unlike `k.connect_sqlite` (`db.go:374`), `connect_db` hands the DSN straight to `sql.Open` with **no `resolvePath` call** (verified: 0 occurrences in lines 46-100). `k.connect_db("sqlite://../../other-app/secret.db")` opens any SQLite file on disk, and `k.sql()` then reads it — a full sandbox escape for file-backed DBs. Found while scoping Phase 5 logging; the fix ships with that phase, but the finding is not logging's to own. |

---

## 🟠 HIGH (Privilege Escalation / DoS / Info Leak)

| # | Issue | Location | Impact |
|---|-------|----------|--------|
| **H1** | No authentication / authorization in run mode | `server.go:232-346` | WebSocket upgrade accepts any `Origin` matching `localhost` patterns. No auth token, no session cookie validation. Any local user/browser tab can connect and run the app. |
| **H2** | Serve mode: no auth on HTTP/WS/TCP listeners | `server.go:214-496` | `handle_http`, `handle_ws`, `handle_tcp` have zero authentication. If reverse proxy misconfigured (missing auth), full API exposure. |
| **H3** | Worker pool shares single Lua state per worker | `server/worker.go` | No isolation between requests. One request's `k.shared.set` affects others. Lua globals mutable across requests. |
| **H4** | `k.shared.*` is global mutable state | `serve.go:138-203` | Any worker can read/write/delete any key. No namespacing, no ACL. Data leakage between tenants/apps. |
| **H5** | No request size limits on WebSocket messages | `session.go`, `server.go` | Large messages can OOM the session. `ReadBufferSize` not configured on `websocket.Accept`. |
| **H6** | WebSocket `OriginPatterns` only allows localhost | `server.go:276`, `server.go:331` | Good for intranet, but if deployed behind proxy without `X-Forwarded-Host` handling, `Origin` check may fail or be bypassed via `Host` header confusion. |
| **H7** | Script hot-reload (SIGHUP/--watch) re-executes `init()` | `server.go:518-538`, `web/server.go:146-183` | Malicious script change during runtime re-triggers `init(config)` with attacker-controlled logic. No signature verification of script. |
| **H8** | `k.print` / logger outputs unsanitized user data | `flow.go:85-101`, `serve.go:50-58` | Log injection (CRLF) if script controls log messages. Could forge log entries. |
| **H9** | No TLS enforcement in serve mode (optional `WithTLS`) | `server.go:608-626` | If not configured, all traffic plaintext. Reverse proxy terminates TLS but internal traffic unencrypted. |

---

## 🟡 LOW (Hardening / Defense in Depth)

| # | Issue | Location | Impact |
|---|-------|----------|--------|
| **L1** | CSP allows `'unsafe-inline'` for styles | `web/server.go:207` | Required for inline `style=""` on controls. Mitigation: nonce-based CSP (complex with dynamic content). |
| **L2** | `k.checksum` supports weak algos (MD5, SHA1, CRC32) | `crypto.go:40-47` | Not for security use (checksums only), but scripts might misuse for password hashing. |
| **L3** | `k.crypt_symmetric` uses AES-CBC (not AEAD) | `crypto.go:107-156` | CBC without integrity check (no HMAC) = padding oracle risk if decrypt error distinguishable. `k.encrypt/decrypt` use AES-GCM (good). |
| **L4** | RSA uses PKCS#1 v1.5 (not OAEP/PSS) | `crypto.go:158-233` | Bleichenbacher-style attacks possible if decrypt oracle exists. Signatures use PKCS#1 v1.5 (acceptable but PSS preferred). |
| **L5** | `pbkdf2` iterations default 10,000 (low by 2026 standards) | `crypto.go:58` | Login control uses 100,000 (better). Should unify. |
| **L6** | No rate limiting on `k.http_request` / socket / FTP / SMTP | Various | Script can DoS external/internal services. |
| **L7** | `resolvePath` uses `evalSymlinksBestEffort` (TOCTOU) | `files.go:565-582` | Symlink race window. Use `os.OpenFile` with `O_NOFOLLOW` + `fstat` after open. |
| **L8** | `AllowFS` defaults to working dir; can be set to `/` | `bindings.go:24`, `config.go:142` | CLI flag `--allow-fs /` disables sandbox. No warning. |
| **L9** | Session ID generation uses `time.Now().UnixNano()` | `server.go:285` | Predictable if attacker knows startup time. Use `crypto/rand`. |
| **L10** | No security headers on `/static/` assets | `web/server.go:115` | Assets served without CSP/HSTS. Low risk (static). |
| **L11** | Debug library exposed (`debug.sethook`, `k.debug.*`) | `vm.go:53`, `debug.go` | `--verbose` / `--repl-on-error` enable introspection. Info leak if enabled in prod. |
| **L12** | Builder (`KALUA builder`) has no auth | `builder/server.go` | Dev tool only, but if exposed on LAN, allows form editing, DB query preview (`/api/db/query`), Lua export. |
| **L13** | INI config file readable by any local user | `config.go:80-86` | May contain DB DSNs with passwords. File perms not enforced (0644). |
| **L14** | `k.param_set/get` uses `.kalua.params.json` in CWD | `net.go:24-30` | World-readable if CWD perms loose. No encryption. |
| **L15** | Worker `init(config)` runs with full script privileges | `server.go:204-210` | `init()` can open sockets, write files, modify `k.shared`. No sandboxing during init. |
| **L16** | Host-written artefacts in the working directory are script-readable | `net.go:24-30`, planned `klog.db` | `.kalua.params.json` and the planned `klog.db` live in the sandbox root (`workdirOf` = CWD, `bindings.go:470`), so `k.file_load` can read them. `klog.db` holds request metadata (method, path, status, remote_addr) and `k.log.*` app rows may hold domain data. Mitigation: a `DenyFS` list enforced in `resolvePath` **and** in the two paths that bypass it — `k.connect_db` (C7) and `k.zip_extract` member targets. |

---

## ✅ What's Done Well

| Area | Notes |
|------|-------|
| **Lua sandbox** | `vm.go`: `SkipOpenLibs`, removed `os.execute`, `require`, `loadfile`, `debug` (except hook), `rawget/set`. Only `k.*` API exposed. |
| **SQL injection prevention** | `tabledb.go`: whitelist columns for sort/filter; `isValidIdentifier` on table/column names; bound parameters everywhere. |
| **Path traversal** | `files.go:536-563`: `resolvePath` restricts to `workdir` + `AllowFS` roots; symlink resolution. **But `k.connect_db` does not use it — see C7.** |
| **WebSocket origin check** | `server.go:276`, `331`: Restricts to localhost patterns. |
| **Security headers** | `web/server.go:207`, `server.go:269`: CSP, X-Frame-Options, X-Content-Type-Options, Referrer-Policy. |
| **Atomic file writes** | `files.go:503-532`: temp file + rename. |
| **ZIP slip prevention** | `files.go:357-360`: `filepath.Rel` + `..` check on extract. Prevents traversal *outside* `dest`, but does not block a member named after a denied host artefact such as `klog.db` (L16). |
| **Email header injection prevention** | `smtp.go:283-286`: `sanitizeHeader` strips CRLF. |
| **Hot reload safety** | `web/server.go:171-178`: static check before reload; failed reload keeps old app. |
| **Serve mode UI disable** | `serve.go:289-338`: UI bindings raise error in serve mode. |

---

## Recommended Mitigations (Priority Order)

| Priority | Action |
|----------|--------|
| **P0** | Add `--allow-net` CLI flag (default: empty = deny all). Require explicit allowlist for `k.http_request`, `k.socket_open`, FTP, SMTP, POP3, SOAP destinations (host:port or CIDR). |
| **P1** | Add `--allow-sql` flag: allowlist of permitted SQL patterns (regex or prepared statement names). Default: only `k.db_select/insert/update/delete` + parameterized `k.sql`. |
| **P2** | Add authentication middleware hook for serve mode: `auth_hook(req) -> (user, ok)` called before `handle_http/ws/tcp`. |
| **P3** | Harden `resolvePath`: use `os.OpenFile(path, O_NOFOLLOW|O_RDONLY)` then `fstat` to verify inode still under allowed root. |
| **P4** | Generate session IDs with `crypto/rand` (16+ bytes). |
| **P5** | Enforce TLS in serve mode when not behind trusted proxy (add `--require-tls` flag). |
| **P6** | Add request/response size limits on WebSocket (configurable, default 1MB). |
| **P7** | Add rate limiting on outbound network calls (per-script budget). |
| **P8** | Add `--readonly-fs` / `--no-net` / `--no-sql` preset flags for locked-down deployment. |
| **P9** | Builder: bind to `127.0.0.1` only (already default), add optional `--builder-token` for auth. |
| **P10** | Document secure deployment: reverse proxy config (Caddy example), INI file perms (0600), `--allow-fs` scoping. |
| **P11** | Route `sqlite://` DSNs in `k.connect_db` through `resolvePath` (closes **C7**), and add a `DenyFS` deny-list enforced in `resolvePath`, in `k.zip_extract` member targets, and in `k.connect_db`, so host-owned artefacts (`klog.db`) are neither readable nor writable by scripts. Shipped with `kalua-serve-enhancements.md` §8.11. |

---

## Quick Wins (Can implement in < 1 day each)

1. **Session ID**: `crypto/rand` instead of timestamp
2. **WebSocket max message size**: `websocket.AcceptOptions{MaxMessageSize: 1<<20}`
3. **INI file warning**: Log warning if `KALUA.INI` world-readable
4. **PBKDF2 default**: Bump to 100,000 everywhere
5. **CSP nonce**: Generate per-request nonce for inline styles (requires render.go changes)
6. **Route `connect_db` sqlite DSNs through `resolvePath`**: a small diff that closes **C7** entirely (sheds with `kalua-serve-enhancements.md` §8.11)

---

## Architecture Note

The **single binary + sandboxed Lua** model is strong: no plugin system, no dynamic code loading, no `require`. Attack surface is the **exposed `k.*` API surface**. The mitigations above focus on *constraining that surface* via allowlists, which fits the intranet threat model perfectly.