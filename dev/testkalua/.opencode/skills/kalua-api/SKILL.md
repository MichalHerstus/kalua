# KALUA API Reference Skill

**Keywords:** kalua, kalipso, api, reference, k.*, K.*, expression functions, globals

---

## Purpose

This skill provides the **complete, drift-free API reference** for the KALUA runtime. The reference is auto-generated from `internal/bindings/api_doc.go` (the single source of truth for LSP completion, hover, and definition) and committed as `api.md` in this skill's directory.

**Regenerate with:** `make gen-api`  
**Verify drift:** `make check-api` (fails CI if out of sync)

---

## Conventions

- **`k.*`** — Host bindings (forms, controls, DB, files, HTTP, crypto, server, etc.)
- **`K.*`** — Coercion helpers & constants (`K.eq`, `K.tonum`, `K.NULL`, …)
- **Expression functions** — Flat globals (§5.9): `left`, `round`, `sys_date`, `lookup`, `tostr`, …
- **Script globals** — `ARGS`, `CTRL`, `main`

All `k.*` functions use `snake_case` matching Kalipso action names.

---

## Reference

See **[`api.md`](api.md)** for the full generated reference including:

- `k.*` bindings grouped by registry group (flow, debug, forms, controls, database, files, json, crypto, xml, server, comm, email, formats, rows)
- `K.*` helpers & constants
- Expression functions by category (string, numeric, conditional, datetime, conversion)
- Script globals

---

## Usage in Agent Workflow

1. **Authoring:** Use `kalua-authoring` skill for the golden loop (`check` → `run --test` → `run`/`serve`)
2. **Lookup:** When the agent needs a signature, parameter, or behavior detail, read `api.md`
3. **Completion/Hover/Definition:** The LSP (`./KALUA lsp`) serves this same data live in the editor
4. **Drift guard:** `make check-api` ensures the reference never silently diverges from the runtime