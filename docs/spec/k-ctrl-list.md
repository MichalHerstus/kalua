# KALUA `k.ctrl.*` Controls — Complete Inventory

### ✅ **Implemented** (registered in `internal/bindings/forms.go` + `forms_wasm.go`)

| Control | Constructor | Key Features |
|---------|-------------|--------------|
| **label** | `k.ctrl.label(form, name, opts)` | `text`, `multiline` (pre-wrap div) |
| **textbox** | `k.ctrl.textbox(form, name, opts)` | `label`, `value`, `multiline` (textarea), `rows`/`cols`, `datetime` (flatpickr: date/time/datetime modes) |
| **button** | `k.ctrl.button(form, name, opts)` | `label`, `class`, `onclick` handler |
| **combo** | `k.ctrl.combo(form, name, opts)` | `label`, `items` (value→display map), `value` |
| **list** | `k.ctrl.list(form, name, opts)` | `label`, `items`, `size` (rows visible) |
| **table** | `k.ctrl.table(form, name, opts)` | Traditional `<table>` or Tabulator mode (`tabulator=true`), DB-linked paging, `columns`, `rows`, `data` |
| **checkbox** | `k.ctrl.checkbox(form, name, opts)` | `label`, `value` (bool), `hidden_value` |
| **radio** | `k.ctrl.radio(form, name, opts)` | `label`, `items`, `value` |
| **image** | `k.ctrl.image(form, name, opts)` | `src`, `alt`, `width`/`height`, `fit` (cover/contain/fill/scale-down/none), `clickable`, `onclick` |
| **chart** | `k.ctrl.chart(form, name, opts)` | Chart.js v4: `type` (line/bar/hbar/pie/doughnut/scatter/radar/area), `labels`, `datasets`, `options`, events (`chart_click`, `chart_hover`, `chart_legend_click`) |
| **looper** | `k.ctrl.looper(form, name, opts)` | Repeater with template controls (`opts.row`), DB-linked virtual scrolling, `links` or derived from `row` |
| **grid** | `k.ctrl.grid(form, name, opts)` | CRUD Grid: DB-linked Tabulator + row actions (view/edit/delete), global actions (new/batch delete), modal detail/edit form, column visibility, selection modes |

---

### ⏳ **Planned** (documented in `kforms_enhancements.md`)

| Control | Section | Description |
|---------|---------|-------------|
| **tab** | §10 | Tab container: `tabs[]` array with `title`/`icon`, `tab_position` (top/bottom), `swipable`, `onchange` event. Operations: `k.tab.set_tab/get_tab/add_tab/remove_tab/insert_tab/set_tab_title/set_tab_icon` |
| **login** | §8 | Declarative login panel: DB-backed (`db`, `table`, `login_column`, `password_column`), hash modes (pbkdf2/sha256/plain), retry policy, events (`on_login`, `on_login_error`, `on_login_cancel`) |
| **tree** | §9 | DB-backed expandable tree (adjacency list: `id_field`, `parent_field`, `label_field`), full load + client-side expand/collapse, events (`on_click`, `on_select`, `on_expand`, `on_loaded`) |
| **topbar** | §11.1 | Fixed header: `icon`, `title`, `user` (name/role/avatar), `logout` fn, fixed position |
| **sidebar** | §11.2 | Collapsible nav: grid cell (with `cell="sidebar"`) or horizontal tabs (vertical layout), `items[]` with `label`/`icon`/`action`/`badge`, `on_select` event |
| **footer** | §11.3 | Fixed bottom bar: `text` (copyright), `version` (auto-filled), `script_name`, `links[]` |

---

### 📋 Summary

| Status | Count | Controls |
|--------|-------|----------|
| **Implemented** | 12 | label, textbox, button, combo, list, table, checkbox, radio, image, chart, looper, grid |
| **Planned** | 6 | tab, login, tree, topbar, sidebar, footer |

**Total: 18 controls** (12 done + 6 planned)

---

### Notes

- **Tabulator mode** is an enhancement of `k.ctrl.table` (not a separate control)
- **CRUD Grid** (`k.ctrl.grid`) is a higher-level widget that subsumes DB-linked table + form editing
- **Layout controls** (topbar/sidebar/footer) render outside normal form flow (fixed position)
- All planned controls have detailed specs in `docs/spec/kforms_enhancements.md` sections 8–11