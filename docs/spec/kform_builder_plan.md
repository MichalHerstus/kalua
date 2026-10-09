# KALUA Form Builder - Evaluation & Implementation Plan

## Executive Summary

A visual form builder for KALUA that allows drag-and-drop form design with live preview, property editing, and export to Lua script. This is a **design-time tool** — no Lua execution, purely form structure editing.

---

## Current Form Structure Analysis

### Lua → JSON Mapping

```lua
-- Lua script
k.form.new("main", {title="Test Form", layout="vertical"})
k.ctrl.label("main", "lbl1", {text="Hello KALUA!"})
k.ctrl.textbox("main", "txt1", {label="Name", value="World"})
k.ctrl.button("main", "btn1", {label="Click Me", onclick=function() ... end})
```

```json
// Equivalent JSON representation
{
  "name": "main",
  "title": "Test Form",
  "layout": "vertical",
  "controls": [
    {
      "name": "lbl1",
      "type": "label",
      "text": "Hello KALUA!",
      "enabled": true,
      "visible": true
    },
    {
      "name": "txt1",
      "type": "textbox",
      "label": "Name",
      "value": "World",
      "enabled": true,
      "visible": true
    },
    {
      "name": "btn1",
      "type": "button",
      "label": "Click Me",
      "class": "kalua-button-primary",
      "enabled": true,
      "visible": true
      // onclick: not serializable to JSON - handled separately
    }
  ]
}
```

### Form Properties

| Property | Type | Default | Description |
|----------|------|---------|-------------|
| `name` | string | required | Form identifier |
| `title` | string | "" | Form title display |
| `layout` | "vertical" \| "grid" | "vertical" | Layout mode |

### Control Properties by Type

| Control | Required Props | Optional Props |
|---------|----------------|----------------|
| **label** | `name`, `type`, `text` | `enabled`, `visible` |
| **textbox** | `name`, `type`, `label` | `value`, `enabled`, `visible`, `multiline`, `rows`, `cols`, `datetime` |
| **button** | `name`, `type`, `label` | `class`, `enabled`, `visible`, `onclick` (handler ref) |
| **combo** | `name`, `type`, `label`, `items` | `enabled`, `visible` |
| **list** | `name`, `type`, `label`, `items` | `enabled`, `visible` |
| **table** | `name`, `type` | `label`, `columns`, `rows`, `enabled`, `visible` |
| **checkbox** | `name`, `type`, `label` | `value`, `enabled`, `visible`, `hidden_value` |
| **radio** | `name`, `type`, `label` | `value`, `enabled`, `visible`, `hidden_value` |

### Items Format (combo/list)
```json
{
  "items": {
    "key1": "Display 1",
    "key2": "Display 2"
  }
}
```

---

## Architecture Options

### Option A: VS Code Webview (Recommended)
- **Pros**: Integrated into existing extension, access to workspace, file system, LSP
- **Cons**: Webview API limitations, TypeScript only
- **Implementation**: Add "Open Form Builder" command → opens webview panel

### Option B: Standalone Web App (Served by KALUA)
- **Pros**: Full browser capabilities, can be used independently
- **Cons**: Separate deployment, needs auth/access control
- **Implementation**: New `KALUA builder` command serves builder UI

### Option C: Electron/Tauri Desktop App
- **Pros**: Native feel, file system access
- **Cons**: Additional maintenance, separate distribution
- **Not recommended** for Phase 1

### Recommendation: **Option A (VS Code Webview)**
- Leverages existing extension infrastructure
- Natural fit for "edit .lua file → open builder → save back"
- Can use VS Code's file watcher for live sync
- Access to LSP for validation

---

## JSON Schema Definition

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "title": "KALUA Form Definition",
  "type": "object",
  "required": ["name", "controls"],
  "properties": {
    "name": { "type": "string", "pattern": "^[a-zA-Z_][a-zA-Z0-9_]*$" },
    "title": { "type": "string" },
    "layout": { "type": "string", "enum": ["vertical", "grid"] },
    "controls": {
      "type": "array",
      "items": { "$ref": "#/definitions/control" }
    }
  },
  "definitions": {
    "control": {
      "type": "object",
      "required": ["name", "type"],
      "properties": {
        "name": { "type": "string", "pattern": "^[a-zA-Z_][a-zA-Z0-9_]*$" },
        "type": { "type": "string", "enum": ["label", "textbox", "button", "combo", "list", "table", "checkbox", "radio"] },
        "label": { "type": "string" },
        "text": { "type": "string" },
        "value": { "type": ["string", "number", "boolean"] },
        "enabled": { "type": "boolean", "default": true },
        "visible": { "type": "boolean", "default": true },
        "class": { "type": "string" },
        "items": { "type": "object", "additionalProperties": { "type": "string" } },
        "columns": { "type": "array", "items": { "type": "string" } },
        "rows": { "type": "array", "items": { "type": "object" } },
        "hidden_value": { "type": "string" },
        "multiline": { "type": "boolean" },
        "rows": { "type": "number" },
        "cols": { "type": "number" },
        "datetime": { "type": "object" }
      }
    }
  }
}
```

---

## Builder UI Layout

```
+-----------------------------------------------------------------+
| Toolbar: [Save] [Export Lua] [Import Lua] [Undo] [Redo] [Zoom]  |
+----------+--------------------------------------+----------------+
|          |                                      |                |
| Controls |         Preview Canvas             | Property Editor|
| Palette  |                                      |                |
|          |  +--------------------------------+  | +------------+ |
|  +------+ | |  Form Title                  |  | | Control:   | |
|  |label | | +--------------------------------+  | | | btn1      | |
|  +------+ | |  [lbl1] Hello KALUA!         |  | +------------+ |
|  |textbox| | |  [txt1] Name: [World______] |  | | Type:      | |
|  +------+ | |  [btn1] [Click Me]           |  | | button     | |
|  |button | | |                              |  | | Label:     | |
|  +------+ | |  Drag controls from palette  |  | | Click Me   | |
|  |combo  | | |  to canvas. Click to select. |  | | Class:     | |
|  +------+ | +--------------------------------+  | | primary    | |
|  |list   | |                                      | | Enabled:  | |
|  +------+ |                                      | | [x]        | |
|  |table  | |                                      | | Visible:  | |
|  +------+ |                                      | | [x]        | |
|  |checkbox| |                                      | | OnClick:  | |
|  +------+ |                                      | | [  fx  ]   | |
|  |radio  | |                                      | +------------+ |
|  +------+ |                                      |                |
|  |image  | |                                      |                |
|  +------+ |                                      |                |
|          |                                      |                |
+----------+--------------------------------------+----------------+
```

### Components

1. **Controls Palette** (left) - Draggable control types
2. **Preview Canvas** (center) - Live form rendering using actual KALUA CSS
3. **Property Editor** (right) - Dynamic form based on selected control type

---

## Preview Rendering Strategy

### Approach: Reuse KALUA Rendering Engine

**Option 1: Embed KALUA CSS + Simulated Rendering**
- Include `kalua.css` in webview
- JavaScript renders controls using same HTML structure as Go `renderControl()`
- Fast, no Go dependency in builder

**Option 2: Call KALUA Binary for Preview**
- Send JSON to `KALUA check` or custom endpoint
- Get back HTML
- Slower, but 100% accurate

**Recommendation: Option 1** with periodic sync validation via Option 2

### Preview HTML Generation (TypeScript)
```typescript
function renderControl(control: Control): string {
  switch (control.type) {
    case 'label':
      return '<label class="kalua-label" id="c:' + formName + ':' + control.name + '">' + escapeHtml(control.text) + '</label>';
    case 'textbox':
      return '<div class="kalua-control">\n' +
        '  <label class="kalua-label" for="c:' + formName + ':' + control.name + '">' + escapeHtml(control.label) + '</label>\n' +
        '  <input type="text" class="kalua-input" id="c:' + formName + ':' + control.name + '" value="' + escapeHtml(control.value) + '"' + (control.enabled ? '' : ' disabled') + '>\n' +
        '</div>';
    // ... etc
  }
}
```

---

## Lua Export Generation

### Strategy: Template-based Code Generation

```typescript
function exportToLua(form: FormDefinition): string {
  const lines: string[] = [];
  
  lines.push('function main()');
  lines.push('  k.form.new("' + form.name + '", {title="' + escapeLua(form.title) + '", layout="' + form.layout + '"})');
  
  for (const ctrl of form.controls) {
    const opts = buildControlOptions(ctrl);
    lines.push('  k.ctrl.' + ctrl.type + '("' + form.name + '", "' + ctrl.name + '", ' + opts + ')');
  }
  
  lines.push('  k.form.show("' + form.name + '")');
  lines.push('end');
  
  return lines.join('\n');
}

function buildControlOptions(ctrl: Control): string {
  const opts: string[] = [];
  
  if (ctrl.type === 'label') {
    if (ctrl.text) opts.push('text="' + escapeLua(ctrl.text) + '"');
  } else {
    if (ctrl.label) opts.push('label="' + escapeLua(ctrl.label) + '"');
  }
  
  if (ctrl.value !== undefined) opts.push('value="' + escapeLua(String(ctrl.value)) + '"');
  if (ctrl.enabled === false) opts.push('enabled=false');
  if (ctrl.visible === false) opts.push('visible=false');
  if (ctrl.class) opts.push('class="' + escapeLua(ctrl.class) + '"');
  if (ctrl.items) opts.push('items=' + jsonToLuaTable(ctrl.items));
  if (ctrl.columns) opts.push('columns=' + jsonToLuaArray(ctrl.columns));
  // ... etc
  
  return '{' + opts.join(', ') + '}';
}
```

### Event Handlers (onclick, etc.)
- **Exported as generated trace stubs** — builder emits `k.form.on` registrations
  for the form lifecycle and each control's documented events, so an exported
  app starts with visible event wiring. Stub bodies print the event path:
  ```lua
  k.ctrl.button("main", "btn1", {label="Click Me"})
  k.form.on("main", "btn1", "click", function()
    k.print("main.btn1.click")
  end)
  ```
  Form-level stubs use the 3-arg form: `k.form.on("main", "open_form", function() k.print("main.open_form") end)`.
- **Existing handlers are left as-is** — an imported inline `onclick` option or
  a captured `k.form.on` body suppresses the stub for that (control, event)
  pair, so real logic is never overwritten. Stubs use the runtime **wire event
  names** (`click`, `selection_change`, `chart_click`, `chart_legend_click`,
  looper `onclick`/`onselect`), not the inline-opt alias names.
- The user replaces a stub body with real logic directly in the Lua; re-export
  preserves it verbatim (`lua_import.go`/`lua_print.go` round-trip).

### Multi-Form Files & Source Preservation
- A builder document (v3) holds **many forms** (`Document.Forms`) plus an
  `activeForm` name. A `.lua` file may mix any number of `k.form.new` blocks
  with arbitrary non-form code (globals, helpers, DB setup) and a JSON
  document stores the forms natively.
- **Import** extracts every form (in declaration order), attributing
  `k.ctrl.*` / `k.form.on` / `k.form.show` calls by their literal form-name
  argument. Each owned statement's source **line span** is recorded
  (`Form.Lines`) and `k.form.on` handler statements are captured **verbatim**
  (exact source text in `Form.HandlerBodies`).
- **Save is a surgical splice** (`rebuild.go`): the server re-imports the
  current file, and only the statement lines belonging to *changed*,
  *deleted*, or *new* forms are touched. Everything else — non-form code,
  comments, blank lines, untouched forms — survives byte-for-byte. A form is
  "changed" when its document form differs from a fresh import (ignoring
  transient spans); an unchanged document is saved back byte-for-byte
  (idempotent, stub echo skipped by a line-signature check).
- **New forms are appended** after the last form block; a script with no form
  at all gets `function main()` appended with the document's forms.
- **Handlers are never rewritten**: when a form's block is regenerated its
  `k.form.on` statements are re-emitted verbatim. On a **control rename** the
  stale handler statement (old name) is preserved and a stub added under the
  new name; on a **form rename** the block is replaced in place with the stale
  handler text kept. Deleting a control or a form removes its `k.form.on`
  statements.
- **Auto-generated stubs** are added only to changed/new forms; opening a
  10-form file and editing one form never rewrites the other nine.
- The builder UI shows a **Form dropdown** (+ New / − Del) next to the title;
  the Code view shows the whole assembled file (what Save writes), and Apply /
  AI-import operates on the whole file, preserving the active selection by
  name.

---

## Implementation Plan

### Phase 1: Foundation (3 days)
- [ ] Create JSON schema for form definition
- [ ] Build TypeScript types matching schema
- [ ] Set up VS Code webview infrastructure in extension
- [ ] Add "KALUA: Open Form Builder" command

### Phase 2: Preview Engine (3 days)
- [ ] Port `renderControl` logic to TypeScript
- [ ] Include `kalua.css` in webview
- [ ] Implement live preview canvas
- [ ] Handle form/control selection highlighting

### Phase 3: Controls Palette (2 days)
- [ ] Draggable control list
- [ ] Drag-and-drop to canvas
- [ ] Insert at position (before/after/into)
- [ ] Visual drop zones

### Phase 4: Property Editor (3 days)
- [ ] Dynamic form generation per control type
- [ ] Real-time preview updates
- [ ] Validation (required fields, unique names)
- [ ] Special editors: items (key-value grid), columns, datetime config

### Phase 5: Layout & Ordering (2 days)
- [ ] Reorder controls (drag handles in preview)
- [ ] Delete controls
- [ ] Form-level properties (title, layout)
- [ ] Copy/paste/duplicate controls

### Phase 6: Import/Export (2 days)
- [ ] Export to Lua script (as described above)
- [ ] Import from existing Lua file (parse `k.form.new` + `k.ctrl.*` calls)
- [ ] Save/load `.kalua-form.json` files

### Phase 7: Polish & Integration (2 days)
- [ ] Undo/redo stack
- [ ] Keyboard shortcuts
- [ ] Responsive preview (mobile/desktop toggle)
- [ ] Error handling & validation feedback
- [ ] Documentation

---

## Technical Details

### Webview Communication

```typescript
// Extension side (extension.ts)
const panel = vscode.window.createWebviewPanel(
  'kaluaFormBuilder',
  'KALUA Form Builder',
  vscode.ViewColumn.One,
  { enableScripts: true, retainContextWhenHidden: true }
);

panel.webview.html = getWebviewContent();

// Message handling
panel.webview.onDidReceiveMessage(msg => {
  switch (msg.type) {
    case 'exportLua':
      // Write to .lua file
      break;
    case 'saveJson':
      // Write to .kalua-form.json
      break;
    case 'loadFile':
      // Read .lua or .json file, send back
      break;
  }
});
```

### File Association

- `.kalua-form.json` — Builder native format
- Double-click → opens in builder
- Right-click `.lua` → "Open in Form Builder" (parses and loads)

---

## Open Questions & Decisions Needed

| Question | Options | Recommendation |
|----------|---------|----------------|
| **Builder hosting** | VS Code webview vs standalone web app | VS Code webview (Option A) |
| **Layout system** | Vertical only vs Grid vs Absolute | Start vertical only; grid later |
| **Event handlers** | Include in JSON? | No — design-time only, export as TODO comments |
| **Table/Looper support** | Include complex controls? | Phase 2 — start with basic controls |
| **Multi-form support** | Single form vs multiple | **Multi-form** — edit one form at a time |
| **Live sync** | Auto-save to Lua on change? | Manual save/export; auto-save JSON |
| **CSS framework** | Plain CSS vs Tailwind vs other | Plain CSS (match KALUA style) |

---

## Dependencies

### New Dependencies (Webview)
- No heavy frameworks — vanilla TypeScript + CSS
- Optional: `sortablejs` for drag-and-drop reordering (~20KB)
- Optional: `uuid` for control IDs

### Existing Assets Reused
- `kalua.css` — embedded in webview
- Control rendering logic — ported to TypeScript

---

## Estimated Timeline: 17 days

| Phase | Days | Deliverable |
|-------|------|-------------|
| 1: Foundation | 3 | Webview + schema + types |
| 2: Preview Engine | 3 | Live rendering matching KALUA |
| 3: Controls Palette | 2 | Drag-drop from palette |
| 4: Property Editor | 3 | Dynamic per-control-type editor |
| 5: Layout & Ordering | 2 | Reorder, delete, form props |
| 6: Import/Export | 2 | Lua <-> JSON round-trip |
| 7: Polish | 2 | UX, validation, docs |
| **Total** | **17** | **MVP Form Builder** |

---

## Future Enhancements (Post-MVP)

1. **Grid Layout** — CSS Grid-based positioning
2. **Looper Support** — Template editor for looper controls
3. **Tabulator Table Config** — Visual column editor
4. **Theme Preview** — Light/dark mode toggle
5. **Responsive Preview** — Device toolbar (mobile/tablet/desktop)
6. **Collaboration** — Real-time co-editing via VS Code Live Share
7. **Code Generation** — Full app skeleton with handlers

---

## Risk Assessment

| Risk | Impact | Mitigation |
|------|--------|------------|
| Preview mismatch vs runtime | High | Periodic validation via `KALUA check` |
| Complex control properties (table, looper) | Medium | Defer to Phase 2 |
| Lua parsing for import | Medium | Use regex + simple AST for known patterns |
| Webview performance with many controls | Low | Virtualize palette, memoize preview |

---

## Next Steps

1. **Confirm architecture**: VS Code webview vs standalone?
2. **Prioritize controls**: All 8 basic controls + image, or subset first?
3. **Layout system**: Vertical-only MVP sufficient?
4. **Event handler strategy**: Placeholder comments acceptable?
5. **File format**: `.kalua-form.json` as primary, Lua as export-only?

Please confirm decisions on open questions, and I'll create detailed technical specs for Phase 1.

---

## Advanced Table & Looper Editor (Next Phase)

Visual, tabbed configuration for the **table** and **looper** widgets replaces the raw
JSON/text fields currently shown in the Properties panel. Both controls get an
**Edit** button in the Properties header (right of the delete button) that opens a
modal with three tabs. The two editors share one backend and one modal shell.

### Shared Backend

1. **Named DB handles** (`--db` becomes real; today the flag is parsed but never consumed):
   - `internal/bindings/db.go`: `RegisterNamedDB(name, dsn) error` — `parseDSN`, `sql.Open`
     + `Ping`, store into the existing `dbHandles` map under `name`, so `db="NAME"` resolves
     everywhere `getDBHandle` is used. Empty/non-identifier names rejected.
   - `QueryPreview(db, query string, limit int) (cols []string, rows [][]any, err error)` —
     read-only guard (`select|with|pragma|explain` only), hard cap ~200 rows.
   - Wire the dead `--db` flags in run (`internal/host/run.go`), serve (`internal/server`),
     and builder (`builderCmd` → `builder.New`). A bad DSN fails startup fast. `k.disconnect_db()`
     with no args closes named handles too (documented).
   - Runtime tables/loopers set `db="NAME"` work in `KALUA run app.lua --db main=sqlite://x.db`
     without a Lua-side `k.connect_db()`.
2. **Static `db` export**: `lua_export.go` emits `db = "NAME"` as a literal for readable names;
   only opaque runtime ids (`db_0x…`) keep the current skip + comment.
3. **Builder endpoints** (`internal/builder/server.go`):
   - `GET /api/db` → `{dbs: ["main", …]}` (empty ⇒ client hints `--db NAME=DSN`).
   - `POST /api/db/query` `{db, query, limit}` → `QueryPreview` result; 400 when no DB configured.
   - `POST /api/looper/rows` `{db, query, row, links, limit}` → server-rendered looper sample rows
     (Phase 4 renderer reuse; pixel-faithful preview, no client widget code).
   - Route `/static/tabulator/` proxying `internal/web/assets/tabulator` via a new exported
     `web.StaticFS()` (no 437 KB copy).

### Table Editor Modal (mode `table`)

- Modal shell `#control-modal` (patterned on `#ai-panel`), tabs **Datasource / Table Setup / Preview**,
  Apply/Cancel. Apply snapshots, writes into `ctrl.opts`, `dirty=true`, `schedulePreview()`.
- **Datasource**: `tabulator` toggle (basic table vs Tabulator); DB dropdown (`/api/db`);
  `query` textarea + `page_size`/`count_query`/`where`/`order_by`. **Run query** → mini results
  grid via `/api/db/query`; **Generate columns** seeds the Setup columns list (`{field,title}`).
  Static path: paste-JSON `data` rows when no DB (hybrid).
- **Table Setup**: row-grid column editor — `field`, `title`, `sortable`, `headerFilter`
  (none/text/number), `editor`, `width`, `align`, `frozen`; add/remove/reorder; writes `columns`
  (array for Tabulator, `{key: title}` map for basic). Global pagination size + layout fold into
  `tabulatorOptions` (deep-merge preserved).
- **Preview**: builds the config from working state; DB active → debounced `/api/db/query` refill;
  `new Tabulator(...)` in the modal (Tabulator JS/CSS loaded in the builder shell). Main canvas
  stays inert so the library never swallows click-to-select. Basic mode → plain `<table>`.

### Looper Editor Modal (mode `looper`)

Same shell, tabs **Datasource / Row Template / Preview**; Edit shown for both `table` and `looper`.

- **Datasource**: DB dropdown; `query`, `page_size`, `count_query`, `where`, `order_by`
  (`where`/`order_by` are currently missing from `TYPE_OPTS.looper`). **Run query** → mini grid
  + **Generate cells** seeds the template from result columns. No `tabulator` toggle (looper is
  always the custom virtual-scroll widget).
- **Row Template**: `links` row editor — `field` (select from query columns else free text),
  `control` (cell key), `property` (default `"value"`); plus `columns` int. In Phase 4 this tab
  becomes a mini form-designer (see below) while staying backward compatible with the legacy
  value-cell model.
- **Preview**: `.kalua-looper`-class markup from `/api/db/query` sample rows (no Tabulator, no WS);
  no DB → config-only + hint.

### Looper Row-Template Controls (runtime + builder)

Looper rows render as real controls (Kalipso-style form-template rows) instead of plain value spans.

- **Model**: new `opts.row` = array of control defs `{type, name, property, field, opts}`. Export
  derives the existing `links` contract (`{{field, control, property}}`) from `row`; both are stored.
  No `row` ⇒ legacy value-cell behavior (backward compatible).
- **Renderer** (`internal/bindings`): `BuildLooperRowHTML(L, rowDefs, rowData) string` — per template
  control set `form`/`name` synthetically, inject `value` from `rowData[field]`, call the existing
  `renderControl` (ids `c:<looper>:<ctrl>:<idx>`). v1 cell types: `label`, `textbox` (display),
  `image`, `checkbox` (display), rendered read-only.
- **Session pager** (`internal/session` `dispatchDBLooperPage`/`looperBatchRows`): ctrl with `row`
  produces `{index, html}` batch rows via the renderer; `renderLooper` adds `data-k-looper-html="1"`.
- **Client** (`app.js` `handleLooperDBBatch`): insert server-rendered `html` rows directly (skip the
  template-clone/text path); event delegation excludes `.kalua-looper-row` scopes so row inputs never
  fire normal control events.

### Tests & Docs

- Go: named-DB registration + `QueryPreview` against `t.TempDir()` SQLite; export literals (`db="NAME"`,
  `links` derived from `row`); builder e2e `/api/db`, `/api/db/query`, `/api/looper/rows`;
  `BuildLooperRowHTML` (bound values, escaping, read-only); session e2e batch with `row` (html rows);
  run-mode `--db` smoke test.
- JS: `node --check` on `builder.js` / `app.js`.
- Docs: AGENTS.md feature note, `api_doc.go` (named DBs + looper `row`/`links`), `KALUA.ini.sample`
  `[BUILDER] db=`.

### Sequencing

Phase 1 (shared backend) → Phase 4 runtime renderer → Phase 2 (table modal) → Phase 3 + Phase 4 UI
(modal shell built once, `table`/`looper` modes). `node --check` + `go test ./...` green throughout.

### Out of Scope (v1)

Interactive looper-row controls firing host events (editable inputs/combos per row), per-row click
handlers, grid/cell layout inside looper rows, table DB write-back / edit-in-grid, function-valued
Tabulator column props (formatters), looper reuse of the table's drag UI.

---

## Builder Palette: Add Missing Controls (Grid + Layout Controls)

The palette currently exposes 11 of the 12 implemented runtime controls — **`grid` is missing** —
and none of the planned layout controls (`topbar`, `sidebar`, `footer`, plus `tab`/`login`/`tree`)
exist in the runtime yet. This section is the plan to close that gap in two phases.

**Current state (research findings):**

| Control | Runtime | Builder palette | Builder Edit modal |
|---------|---------|-----------------|--------------------|
| `grid` | ✅ `k.ctrl.grid` (`grid.go`, `render.go` `renderGrid`) | ❌ | Edit button refuses it (`builder.js` `renderEditor` guard); **CRUD tab is built but dead** — `cmRenderCrud`/`wireCMCrud` fully wired to `CM` state, yet `cmApply` never saves `pk_field`/`selection_mode`/`row_actions`/… and `openControlModal` never loads them |
| `topbar` / `sidebar` / `footer` | ❌ spec only (`kforms_enhancements.md` §11) | ❌ | ❌ |
| `tab` / `login` / `tree` | ❌ spec only (§10/§8/§9) | ❌ | out of scope here |

**What already works**: builder import/export is type-generic (`k.ctrl.<type>` round-trips any
control, `lua_import.go` `importControl` has no type whitelist); `Preview()` calls
`bindings.RenderForm` so server-side `renderGrid` HTML renders in the canvas; the grid's inner
`.kalua-tabulator-table` is already converted by the existing `renderPreviewTabulators` fallback
(figures: import/export `k.ctrl.grid` today already survive a round trip); `KALUA check` already
accepts `k.ctrl.grid`; function-valued opts (`logout = function() … end`, `on_select = …`) already
round-trip through `optsToJSON` → `Inline` → export re-emission (`lua_export.go`).

### Phase 1 — Grid into the palette + wire the CRUD tab (client-only)

All changes in `internal/builder/assets/builder.js` + `internal/builder/builder_test.go`.

1. **Palette** (`PALETTE`): append `['grid', 'CRUD Grid']`. Palette render/click/dblclick wiring is
   generic (`data-palette`), so no other palette code changes.
2. **`TYPE_OPTS.grid`** (drives the generic ⚙ option editor):
   - shared datasource keys already labelled: `db`, `query`, `count_query`, `page_size`, `where`, `order_by`;
   - grid-specific: `columns` (json), `pk_field` (string), `selection_mode`
     (select `none|single|multi`), `row_click_action` (select `view|edit|select|none`),
     `column_visibility` (bool), `default_visible` (json), `row_actions` (json),
     `global_actions` (json), `form` (string — referenced detail/edit form).
   - add `LABELS` entries for all new keys.
3. **`addControl`**: `case 'grid'` → defaults `{query: 'SELECT * FROM t', pk_field: 'id',
   selection_mode: 'multi'}`.
4. **Open the Edit modal for grid**:
   - `renderEditor`: allow `grid` in the Edit-button guard (and tooltip).
   - `openControlModal`: accept `grid`; treat it like the `table` mode for the shared
     Datasource/Setup/Preview tabs (grid is always Tabulator-style columns → reuse `CM.columns`);
     add `grid` to the `CM.mode === 'table'` branch sites (`cmApply`, `wireCMSetup`,
     `cmRenderSetup`); hide the `tabulator` toggle row for grid; `cmModeLabel()` → `'CRUD Grid'`.
   - Show the **CRUD tab only for `grid`** (`renderControlModal` toggles `#cm-tab-crud`
     display); it is meaningless for `table`/`looper`.
5. **Wire the dead CRUD tab (load ↔ save)**:
   - Declare the missing `CM` fields (`pkField, selectionMode, columnVisibility, rowClickAction,
     defaultVisible, rowActions, globalActions, formRef, formWidth, inlineForm, inlineTitle,
     inlineGap, inlineControls`).
   - `openControlModal` (grid): load from `c.opts` using runtime keys (`pk_field`,
     `selection_mode`, `column_visibility`, `row_click_action`, `default_visible`, `row_actions`,
     `global_actions`; `form` → string ref **or** inline `{title, controls}` → `formRef` +
     inline-form editor path).
   - `cmApply` (grid branch): persist each with `setOpt(c, 'pk_field', …)` … (`column_visibility`
     as bool, `form` written back as string or inline object); save `columns` via the
     tabulator-table array path.
6. **Preview polish (optional)**: when the modal has `CM.lastResult`, render those sample rows for
   a DB-linked grid instead of the bare "No data" placeholder.
7. **Tests** (`internal/builder/builder_test.go`):
   - `TestExportImportGridRoundTrip` — grid with `pk_field`/`selection_mode`/`row_actions`/`form`
     survives export → import.
   - Extend `TestPreview` — doc with a grid control → HTML contains `data-k-grid="1"` and
     `.kalua-tabulator-table`.
   - Existing handler-preservation tests keep passing (grid `k.form.on` statements such as
     `grid_form_save` are already verbatim-preserved).
8. **Verify**: `make js-check`, `go test ./...`, `KALUA check testdata/apps/grid_crud_demo.lua`,
   headless-Chrome smoke (palette lists Grid; add; Edit modal opens with CRUD tab; Apply persists;
   export round-trips).

### Phase 2 — Layout controls `topbar` / `sidebar` / `footer` (runtime + builder)

Implemented per `kforms_enhancements.md` §11 (renderer shared with the runtime so the builder
preview is pixel-faithful). Background the runtime work, then add the palette entries.

#### 2a. Runtime implementation

1. **Constructors** — `internal/bindings/forms.go`: register `ctrl.topbar`, `ctrl.sidebar`,
   `ctrl.footer` (3-line `addControl(...)` pattern each); mirror in `forms_wasm.go` (M5 parity).
2. **Checker registry** — `internal/bindings/bindings.go`: add the three to the known-API map so
   `KALUA check` accepts them.
3. **Renderer** — `internal/bindings/render.go`: `renderControl` cases + `renderTopbar` /
   `renderSidebar` / `renderFooter`:
   - topbar: `.kalua-topbar` fixed bar — icon, title, user block (name/role/avatar), logout hook;
   - sidebar: `.kalua-sidebar` + `data-k-sidebar` attrs (position/width/collapsed/items JSON);
     vertical layout → horizontal tab-bar variant; grid layout → normal cell flow (`cell="sidebar"`
     per §11.4, reuses the existing `cell`/`align` common opts);
   - footer: `.kalua-footer` — text / auto-filled `version` (`internal/version`) / `script_name` /
     links.
4. **CSS** — `internal/web/assets/kalua.css`, then `make sync-assets` (copies to
   `internal/builder/assets/kalua.css`; `make check-assets` guards it). Run mode anchors bars with
   `position: fixed`; builder canvas needs them scoped to `#preview` (`position: absolute` inside
   the relative-positioned preview box).
5. **Client behavior** — `internal/web/assets/app.js`: topbar logout click → event; sidebar item
   click → `{action, item, index}` payload; sidebar collapse toggle (session-only, resets on
   reload, §11.6). Builder canvas is server-rendered HTML (no app.js) → static preview suffices,
   same as charts/tables.
6. **Session events** — `internal/session/session.go`: dispatch `logout` (topbar) and
   `sidebar_select` (sidebar) via the `fireGridEvent` pattern. Spec §11.2 example uses event name
   `"select"` while §11.5 says `sidebar_select` — implement `sidebar_select` as canonical, accept
   `"select"` as an alias.
7. **API docs** — `api_doc.go` entries with `Params` per §11 option tables; `make gen-api &&
   make check-api` (regenerates the AI knowledge base).
8. **Docs/status** — `docs/spec/k-ctrl-list.md` (Implemented 12 → 15).
9. **Tests** — render tests (attrs/HTML, both layout modes), session tests (`logout` /
   `sidebar_select` dispatch), checker test (no more unknown-`k.ctrl.topbar`).

#### 2b. Builder integration

1. `PALETTE` += `['topbar', 'Topbar']`, `['sidebar', 'Sidebar']`, `['footer', 'Footer']`.
2. `TYPE_OPTS` per §11 (tables → `t: 'json'`):
   - topbar: `icon`, `title`, `user` (json), `visible`;
   - sidebar: `position` (select left/right), `width` (num), `collapsed_width` (num),
     `collapsible`/`collapsed` (bool), `items` (json), `style` (json);
   - footer: `text`, `version`, `script_name`, `links` (json), `style` (json);
   - `LABELS` per new key.
3. `addControl` defaults (topbar `title="My App"`; sidebar one sample item; footer `text="© …"`).
4. **No import/export work** — function opts and `k.form.on` statements already round-trip; verify
   preview positioning (bars anchored to `#preview`, not the viewport).

### Verify (both phases)

```bash
make js-check                        # now in ci
go test ./... && go vet ./...
make gen-api && make check-api       # Phase 2 (api_doc changes)
make check-assets                    # Phase 2 (kalua.css sync)
KALUA check <demo>.lua
```
Plus headless-Chrome DOM checks (palette 11 → 12 after Phase 1; 15 after Phase 2) and a
`KALUA run` visual pass of a shell app.

### Sequencing & out of scope

Phase 1 and Phase 2 are independent commits (grid first — client-only). Phase 2 is the large piece
(runtime renderer + CSS + client events + session dispatch). `tab`/`login`/`tree` stay out of scope;
the palette/`TYPE_OPTS` pattern established here makes them cheap to add if/when the runtime lands.
