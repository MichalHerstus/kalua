# 1. New Table Widget Features

## Overview
Enhance KALUA's `k.ctrl.table` control with [Tabulator](https://github.com/tabulator-tables/tabulator) v6.x for advanced data grid capabilities (sorting, filtering, pagination, selection) while maintaining backward compatibility via progressive enhancement.

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        Go Host (Session)                        │
│  ┌─────────────────┐    ┌──────────────────┐                  │
│  │ k.ctrl.table    │    │ k.table.set_data │                  │
│  │ (opts.tabulator)│───▶│ (JSON data)      │                  │
│  └────────┬────────┘    └────────┬─────────┘                  │
│           │                      │                             │
│           ▼                      ▼                             │
│  ┌─────────────────────────────────────────┐                  │
│  │ renderControl: minimal <div> + data     │                  │
│  │ attributes (data-k-tabulator, data)     │                  │
│  └─────────────────┬───────────────────────┘                  │
└────────────────────┼──────────────────────────────────────────┘
                     │ WebSocket update_control
                     ▼
┌─────────────────────────────────────────────────────────────────┐
│                     Browser (app.js)                            │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │ Tabulator.init(selector, options, data)                   │   │
│  │ • Destroys old instance on update                         │   │
│  │ • Stores instance in Map<selector, Tabulator>             │   │
│  └──────────────────────────┬────────────────────────────────┘   │
│                             │                                     │
│              ┌──────────────┼──────────────┐                     │
│              ▼              ▼              ▼                     │
│       rowSelectionChanged  dataFiltered  pageChanged             │
│              │              │              │                     │
│              └──────────────┼──────────────┘                     │
│                             ▼                                     │
│              PostEvent: tabulator_selection_change,              │
│                         tabulator_filter_change,                 │
│                         tabulator_page_change                    │
└─────────────────────────────────────────────────────────────────┘
```

## API Surface

### New Options for `k.ctrl.table(form, name, opts)`

| Option | Type | Description |
|--------|------|-------------|
| `tabulator` | `boolean` | Enable Tabulator enhancement (default: false) |
| `tabulatorOptions` | `table` | Pass-through Tabulator options object |
| `columns` | `table[]` | Explicit column definitions (array of tables) |
| `data` | `table[]` | Initial row data (array of tables, 1-based) |

### New Functions

| Function | Signature | Description |
|----------|-----------|-------------|
| `k.table.set_data` | `(form, name, dataTable)` | Bulk replace all row data |
| `k.table.get_data` | `(form, name)` → table | Get all current data from browser |
| `k.table.get_selected_rows` | `(form, name)` → table | Get selected row indices (1-based) |
| `k.table.set_remote_data` | `(form, name, {data, last_page?, last_row?})` | Provide server-side pagination data |

### Events (via `k.form.on(form, ctrl, event, fn)`)

| Event | Payload |
|-------|---------|
| `tabulator_selection_change` | `{rows: number[], data: object[]}` |
| `tabulator_filter_change` | `{filters: object[], rowCount: number}` |
| `tabulator_page_change` | `{page: number}` |
| `tabulator_ajax_request` | `{page, size, sort: [{field, dir}], filter: [...]}` |

## Key Behavior Decisions

| Decision | Implementation |
|----------|----------------|
| **Initial data** | In `opts.data` passed to `k.ctrl.table` |
| **Auto-columns** | If `opts.columns` absent, infer from first row of `opts.data` |
| **Type inference** | `number`→sorter/editor number, `boolean`→tickCross, `string`→input |
| **Selection** | `selectable: true` (single row, click to select) |
| **Cleanup** | Destroy Tabulator instance on `close_form` / form stack pop |

## Column Type Inference Rules

| Lua Type | Tabulator Column Config |
|----------|-------------------------|
| `number` | `{sorter: "number", editor: "number", hozAlign: "right"}` |
| `boolean` | `{formatter: "tickCross", editor: "tickCross", hozAlign: "center"}` |
| `string` | `{sorter: "string", editor: "input"}` |
| `nil` / missing | `{sorter: "string", editor: "input"}` (default) |

```go
func inferColumn(field string, value lua.LValue) map[string]interface{} {
    switch value.Type() {
    case lua.LTNumber:
        return map[string]interface{}{"field": field, "title": field, 
            "sorter": "number", "editor": "number", "hozAlign": "right"}
    case lua.LTBool:
        return map[string]interface{}{"field": field, "title": field, 
            "formatter": "tickCross", "editor": "tickCross", "hozAlign": "center"}
    default:
        return map[string]interface{}{"field": field, "title": field, 
            "sorter": "string", "editor": "input"}
    }
}
```

## Implementation Phases

### Phase 1: Assets & Go Rendering (2 days)
- Embed Tabulator v6.2+ assets: `tabulator.min.js`, `tabulator.min.css`, `themes/simple/simple.min.css`
- Update `shell.html` to include Tabulator CSS/JS
- Modify `ctrl.table` registration to parse `tabulator`, `tabulatorOptions`, `columns`, `data`
- Implement `renderTabulatorTable` with auto-column inference + type inference
- Add `k.table.set_data` (sends `tabulator_update` message)
- Add form close cleanup: send `tabulator_destroy` on `PopForm`

### Phase 2: Client-Side Integration (3 days)
- `app.js`: Tabulator lifecycle management (init/destroy/update)
- Instance Map keyed by selector
- Event bridging: `rowSelectionChanged` → `tabulator_selection_change`
- Default options: `layout: "fitColumns"`, `selectable: true`, `selectableRangeMode: "click"`
- Handle `tabulator_update` (replaceData), `tabulator_destroy`, `close_form`

### Phase 3: Remote Pagination (2 days)
- New event: `tabulator_ajax_request` (browser → Go with page, size, sort, filter)
- Lua handler returns `{data: [...], last_page: N}` or `{data: [...], last_row: N}`
- Go: `k.table.set_remote_data` sends `tabulator_remote_data`
- Client: handle `tabulator_remote_data` with setData/setMaxPage/setRowCount

### Phase 4: Selection & Data Query API (1 day)
- `k.table.get_selected_rows`: sends `tabulator_get_selection` → returns 1-based indices
- `k.table.get_data`: sends `tabulator_get_data` → returns all row data

### Phase 5: Static Checker & LSP (1 day)
- Checker: validate `tabulator`, `tabulatorOptions`, `columns`, `data` in `k.ctrl.table`
- LSP: completions for Tabulator options, column properties, formatters, editors

### Phase 6: CSS & Polish (1 day)
- `kalua.css`: Tabulator simple theme overrides matching KALUA look
- Edge cases: empty data, column type inference, memory leak prevention

## New WebSocket Message Types

| Direction | Type | Payload |
|-----------|------|---------|
| Go → Browser | `tabulator_update` | `{selector, data}` |
| Go → Browser | `tabulator_remote_data` | `{selector, data, last_page?, last_row?}` |
| Go → Browser | `tabulator_destroy` | `{form}` |
| Browser → Go | `tabulator_selection_change` | `{form, ctrl, event, value: {rows, data}}` |
| Browser → Go | `tabulator_filter_change` | `{form, ctrl, event, value: {filters, rowCount}}` |
| Browser → Go | `tabulator_page_change` | `{form, ctrl, event, value: {page}}` |
| Browser → Go | `tabulator_ajax_request` | `{form, ctrl, event, value: {page, size, sort, filter}}` |
| Go → Browser | `tabulator_get_selection` | `{id, form, ctrl}` |
| Browser → Go | `tabulator_selection_resp` | `{id, rows: number[]}` |
| Go → Browser | `tabulator_get_data` | `{id, form, ctrl}` |
| Browser → Go | `tabulator_data_resp` | `{id, data: object[]}` |

## File Changes

| File | Changes |
|------|---------|
| `internal/web/assets/tabulator/*` | **New** - embedded assets |
| `internal/web/templates/shell.html` | Add Tabulator CSS/JS links |
| `internal/bindings/forms.go` | Parse tabulator opts, renderTabulatorTable, new APIs |
| `internal/session/session.go` | Handle tabulator_destroy on close_form, new inbox types |
| `internal/web/assets/app.js` | Tabulator init/destroy/update, event bridging |
| `internal/web/assets/kalua.css` | Tabulator simple theme overrides |
| `internal/checker/checker.go` | Validate tabulator options |
| `internal/lsp/server.go` | Completions for new API |

## Dependencies
- Tabulator v6.2+ (embedded, no new Go dependencies)
- Embedded via `//go:embed` in `internal/web/server.go`

## Estimated Timeline: 10 days

| Phase | Days |
|-------|------|
| 1: Assets & Go rendering | 2 |
| 2: Client integration | 3 |
| 3: Remote pagination | 2 |
| 4: Selection/Query API | 1 |
| 5: Checker & LSP | 1 |
| 6: CSS & Polish | 1 |

## DB-Linked Tables (Kalipso "Connect to Database" parity)

Kalipso lets a table widget be bound directly to a DB table/view via a SELECT
statement with fields assigned to columns. KALUA implements the same with the
Tabulator widget backed by a **built-in Go pager** (no per-table Lua handler
needed). Server-side sort/filter (Decision A) so page count stays correct.

### New Options for `k.ctrl.table(form, name, opts)` (all optional → zero regression)

| Option | Type | Description |
|--------|------|-------------|
| `db` | `handle` | Connection handle from `k.connect_db` / `k.connect_sqlite` |
| `query` | `string` | Base `SELECT` statement (table/view/raw SQL) |
| `columns` | `table[]` | **Optional** field→column override; else auto-mapped from the query result columns |
| `page_size` | `number` | Rows per page (Tabulator `paginationSize`; default from opts) |
| `count_query` | `string` | **Optional** `SELECT COUNT(*) ...`; derived from `query` (`SELECT COUNT(*) FROM (query)`) when omitted |
| `where` | `table` | **Optional** base filter `{col=val}` ANDed into every page (reuses `db_select` builder) |
| `order_by` | `string` | **Optional** base ordering `"col [ASC|DESC]"` prepended to user sort |

### New Functions

| Function | Signature | Description |
|----------|-----------|-------------|
| `k.table.refresh` | `(form, name)` | Re-run the linked query, show page 1 |
| `k.table.set_db_source` | `(form, name, {db, query, columns?, page_size?, count_query?, where?, order_by?})` | Swap the DB source at runtime; next page request uses it |

### Behavior

- **Dispatching**: on `tabulator_ajax_request`, if the control carries a `db`
  handle the session services the page in-process via the Go pager; otherwise
  the existing Lua `tabulator_ajax_request` handler path runs (unchanged).
- **Auto-columns**: when `columns` is absent the pager maps the query result's
  column names to Tabulator fields; type inference stays client-side.
- **Sorting**: header sort → `ORDER BY <whitelisted field> [ASC|DESC]`,
  prepended by base `order_by` when present.
- **Filtering**: header filter `{field, type, value}` → safe operator map
  (`= != < <= > >= like in`); values are bound parameters. Non-whitelisted
  fields are dropped.
- **Whitelist (security)**: only mapped/result columns may appear in
  `ORDER BY`/`WHERE` — same discipline as the Phase-B2/B6 SQL-identifier fix.
  `query` is author-supplied (trusted, like `k.sql`); the appended paging/
  sort/filter is generated exclusively from whitelisted fields + bound params.
- **Count**: `last_page` from `COUNT(*)` (`count_query`, or derived subquery).
- **Refresh**: `tabulator_refresh` → client re-triggers the remote loader for
  page 1.

### WebSocket Message Types

| Direction | Type | Payload |
|-----------|------|---------|
| Go → Browser | `tabulator_refresh` | `{selector}` |

### Implementation Phases

| # | Work | Files |
|---|------|-------|
| B1 | `addControl` stores `db/query/db_columns/page_size/count_query/db_where/db_order_by` | `internal/bindings/forms.go` |
| B2 | `FetchTablePage(e, ctrl, req)` pager (COUNT + page + safe sort/filter) | `internal/bindings/db.go` |
| B3 | Dispatch DB-linked pages in `handleTabulatorAjaxRequest`; error → banner (no crash) | `internal/session/session.go` |
| B4 | `k.table.refresh` + `k.table.set_db_source` (+ `registerKnown`/`api_doc.go`/`api.md`) | `internal/bindings/tabulator.go` |
| B5 | Client: handle `tabulator_refresh` (page-1 reload) | `internal/web/assets/app.js` |
| B6 | Tests: pager unit (paging/sort/whitelist/filter/count) + session e2e (sqlite) | `tabulator_test.go`, session e2e |
| B7 | Demo: sqlite table + seed + DB-linked tabulator + refresh | `testdata/apps/tabulator_demo.lua` |

---

# 2. Looper Control Evaluation & Plan

## Overview
Add a **Looper control** to KALUA — a repeater where each row is a custom form template (not just cells), matching Kalipso's looper concept. Each "cell" contains arbitrary controls (label, textbox, button, etc.) with free positioning.

## Kalipso Looper Concept
- Template of controls repeated per record
- Layout: vertical/horizontal, cell dimensions, columns
- Data binding: link control properties to DB columns
- Population: manual (`Add Line`) or database-linked (`Refresh Control`)

## Proposed KALUA Looper API

```lua
-- Create looper (vertical only, per decision)
k.ctrl.looper(form, "mylooper", {
    cell_width = 300,
    cell_height = 150,
    columns = 1,
    virtual_scrolling = true,  -- for large datasets
})

-- Add controls TO THE LOOPER TEMPLATE (separate namespace)
k.looper.add_control("mylooper", "lbl_name", "label", {label = "Name:", x = 10, y = 10})
k.looper.add_control("mylooper", "txt_name", "textbox", {x = 80, y = 10, width = 200})
k.looper.add_control("mylooper", "btn_view", "button", {label = "View", x = 10, y = 50})

-- Data operations (manual population)
k.looper.add_line(form, "mylooper", {txt_name = "John"})
k.looper.delete_line(form, "mylooper", index)
k.looper.set_line(form, "mylooper", index, {txt_name = "Jane"})
k.looper.get_line(form, "mylooper", index)  -- returns control values table
k.looper.clear(form, "mylooper")

-- DB linking (simplified: SQL query + column mapping)
k.looper.link_db(form, "mylooper", {
    db = db_handle,
    query = "SELECT name, email FROM customers WHERE active=1 ORDER BY name",
    links = {
        {column = 1, control = "txt_name", property = "value"},   -- 1-based column index
        {column = 2, control = "txt_email", property = "value"},
    }
})
k.looper.refresh(form, "mylooper")  -- re-execute query, repopulate

-- Events
k.form.on(form, "mylooper", "onclick", function(ctrl_name, line_idx) ... end)
k.form.on(form, "mylooper", "onselect", function(line_idx) ... end)
```

## Key Design Decisions (Per User)

| Decision | Choice | Rationale |
|----------|--------|-----------|
| **Template controls** | `k.looper.add_control(looper_name, ctrl_name, type, opts)` | Separate namespace, clear ownership |
| **DB linking** | SQL query + column index mapping | Simpler than Kalipso's table/filter/order_by |
| **Orientation** | Vertical only (horizontal deferred) | Covers 90% use cases, simpler layout |
| **Nested loopers** | No | Complexity not justified yet |
| **Virtual scrolling** | Yes (required) | Handle 1000+ rows efficiently |
| **Priority** | After Tabulator table enhancement | Sequential phases |

## Technical Architecture

### Go Side (`forms.go`)

```go
// LooperControl struct
type LooperControl struct {
    Type             string            // "looper"
    Template         *lua.LTable       // controls: name -> {type, opts}
    CellWidth        int
    CellHeight       int
    Columns          int               // always 1 for vertical
    VirtualScrolling bool
    Rows             []map[string]lua.LValue  // per-row control values
    DBLink           *LooperDBLink
}

// LooperDBLink
type LooperDBLink struct {
    DBHandle   int
    Query      string
    Links      []LooperDBColumnLink  // {Column int, Control string, Property string}
}

// Render: 
// 1. Generate template HTML once (hidden)
// 2. For each row: clone template, apply row values, assign data-k-line-index
// 3. Wrap in scroll container with virtual scrolling sentinel elements
```

### Client Side (`app.js`)

```javascript
// Virtual scrolling with IntersectionObserver
// - Render only visible rows + buffer
// - Sentinel elements trigger load more
// - Template cloned via cloneNode(true)

// Event delegation:
// - Template controls get prefixed IDs: "looper:mylooper:1:txt_name"
// - Click/change events parsed for line index
// - Forward to Go as looper_click, looper_change with line_idx
```

### WebSocket Messages

| Direction | Type | Payload |
|-----------|------|---------|
| Go → Browser | `looper_render` | `{selector, template_html, rows_data, virtual: true}` |
| Go → Browser | `looper_add_line` | `{selector, index, row_data}` |
| Go → Browser | `looper_delete_line` | `{selector, index}` |
| Go → Browser | `looper_update_line` | `{selector, index, row_data}` |
| Go → Browser | `looper_clear` | `{selector}` |
| Go → Browser | `looper_destroy` | `{selector}` |
| Browser → Go | `looper_click` | `{form, ctrl, event, value: {line_idx, ctrl_name}}` |
| Browser → Go | `looper_change` | `{form, ctrl, event, value: {line_idx, ctrl_name, value}}` |
| Browser → Go | `looper_scroll_request` | `{selector, start_idx, count}` |

## Implementation Phases

### Phase 1: Core Looper Structure (2 days)
- `k.ctrl.looper` registration with options parsing
- `k.looper.add_control` - register template controls
- LooperControl struct, template storage in Lua
- Basic render: generate template HTML + clone per row (no virtual yet)

### Phase 2: Data Operations (2 days)
- `k.looper.add_line/delete_line/set_line/get_line/clear`
- Row data storage in control
- WebSocket messages for incremental updates
- Client: apply row data to cloned template controls

- WebSocket messages for incremental updates
- Client: apply row data to cloned template controls

### Phase 3: DB Linking (2 days)
- `k.looper.link_db` - store query + column mappings
- `k.looper.refresh` - execute query, populate rows
- Reuse existing `k.db_select` / `k.sql` infrastructure

### DB-Linked Loopers (mirror of the Tabulator DB-Linked Tables design)

Same pattern as §1's DB-linked Tabulator: a `k.ctrl.looper` linked to a DB
handle + SELECT is populated server-side by a Go helper (no per-app render
loop), reusing the identical whitelist / bound-param / driver-paging
discipline from `tabledb.go`.

#### New Options for `k.ctrl.looper(form, name, opts)` (all optional → zero regression)

| Option | Type | Description |
|--------|------|-------------|
| `db` | `handle` | Connection handle from `k.connect_db` / `k.connect_sqlite` |
| `query` | `string` | Base `SELECT` statement (table/view/raw SQL) |
| `links` | `table[]` | Result→template map: `{column=N, control="txt_name", property="value"}` (1-based column index) **or** `{field="col_name", control="...", property=...}` |
| `page_size` | `number` | Rows per page (virtual-scroll batch size; default from opts) |
| `count_query` | `string` | Optional `SELECT COUNT(*) ...`; derived from `query` if omitted |
| `where` | `table` | Optional base filter `{col=val}` ANDed into every page (reuses `db_select` builder) |
| `order_by` | `string` | Optional base ordering `"col [ASC|DESC]"` prepended to user sort |

### New Functions

| Function | Signature | Description |
|----------|-----------|-------------|
| `k.looper.link_db` | `(form, name, {db, query, links, page_size?, count_query?, where?, order_by?})` | Attach DB source to a looper |
| `k.looper.set_db_source` | `(form, name, {db, query, links?, page_size?, count_query?, where?, order_by?})` | Swap DB source at runtime; next fetch uses it |
| `k.looper.refresh` | `(form, name)` | Re-run the linked query, show page 1 |

### Behavior

- **Dispatching**: on `looper_scroll_request`, if the control carries a `db`
  handle the session services the page in-process via the Go pager; otherwise
  the existing Lua `looper_scroll_request` handler path runs (unchanged).
- **Auto-columns**: when `columns` is absent the pager maps the query result's
  column names to Tabulator fields; type inference stays client-side.
- **Sorting**: header sort → `ORDER BY <whitelisted field> [ASC|DESC]`,
  prepended by base `order_by` when present.
- **Filtering**: header filter `{field, type, value}` → safe operator map
  (`= != < <= > >= like in`); values are bound params. Non-whitelisted
  fields are dropped.
- **Whitelist (security)**: only mapped/result columns may appear in
  `ORDER BY`/`WHERE` — same discipline as the Phase-B2/B6 SQL-identifier fix.
  `query` is author-supplied (trusted, like `k.sql`); the appended paging/
  sort/filter is generated exclusively from whitelisted fields + bound params.
- **Count**: `has_more` from `COUNT(*)` (`count_query`, or derived subquery).
- **Refresh**: `looper_refresh` → client re-triggers the remote loader for
  page 1.

### WebSocket Message Types

| Direction | Type | Payload |
|-----------|------|---------|
| Go → Browser | `looper_db_batch` | `{selector, rows: [{index, data:{ctrl:value}}], has_more, last_page}` |
| Browser → Go | `looper_scroll_request` | `{form, ctrl, start_idx, count, sort?, filter?}` |
| Browser → Go | `looper_refresh_request` | `{form, ctrl}` |

### Implementation Phases

| # | Work | Files |
|---|------|-------|
| L1 | `addControl` stores `db/query/links/page_size/count_query/db_where/db_order_by` | `internal/bindings/forms.go` |
| L2 | `FetchLooperRows(e, link, req)` pager (COUNT + page + safe sort/filter) | `internal/bindings/tabledb.go` |
| L3 | Dispatch DB-linked pages in `handleLooperScrollRequest`; error → banner (no crash) | `internal/session/session.go` |
| L4 | `k.looper.refresh` + `k.looper.set_db_source` (+ `registerKnown`/`api_doc.go`/`api.md`) | `internal/bindings/tabulator.go` |
| L5 | Client: handle `looper_db_batch`, `looper_refresh`, virtual scroll + `has_more` | `internal/web/assets/app.js` |
| L5 | Tests: pager unit (paging/sort/whitelist/filter/count) + session e2e (sqlite) | `tabulator_test.go`, session e2e |
| L6 | Demo: sqlite DB-linked looper + refresh + runtime source swap | `testdata/apps/tabulator_demo.lua` |

### Phase 4: Virtual Scrolling (2 days)
- Go: render with sentinel elements, `looper_scroll_request` handler
- Client: IntersectionObserver, dynamic row loading/unloading
- Buffer management (render ±50 rows around viewport)

### Phase 5: Events & Polish (1 day)
- `onclick` / `onchange` / `onselect` event bridging
- `k.form.on` integration for looper controls
- CSS: looper container, cell spacing, scroll styling
- Destroy on `close_form`

### Phase 6: Checker & LSP (1 day)
- Validate looper API, template controls, DB link structure
- LSP completions for `k.looper.*` namespace

## File Changes

| File | Changes |
|------|---------|
| `internal/bindings/forms.go` | LooperControl struct, add_control, data ops, DB link, render |
| `internal/session/session.go` | Handle looper messages, virtual scroll requests |
| `internal/web/assets/app.js` | Virtual scrolling, template cloning, event delegation |
| `internal/web/assets/kalua.css` | Looper container, cell, virtual scroll styles |
| `internal/checker/checker.go` | Validate looper API |
| `internal/lsp/server.go` | Looper completions |

## Dependencies
- No new external dependencies
- Uses existing `database/sql`, `github.com/yuin/gopher-lua`
- Virtual scrolling: pure JS (IntersectionObserver)

## Estimated Timeline: 10 days (after Tabulator)

| Phase | Days |
|-------|------|
| 1: Core structure | 2 |
| 2: Data operations | 2 |
| 3: DB linking | 2 |
| 4: Virtual scrolling | 2 |
| 5: Events & polish | 1 |
| 6: Checker & LSP | 1 |

## Open Questions for Implementation

1. **Template control events**: Should template controls support all events (`onclick`, `onchange`, `onfocus`, etc.) or just `onclick` initially?
-> just “onlick"
2. **Line index base**: 1-based (Kalipso) or 0-based (JS)? Recommend 1-based for consistency.
-> 1-based (Kalipso)
3. **Row data storage**: Store full control values per row, or just linked DB columns? Full values for manual population flexibility.
-> full values
4. **Virtual scroll buffer size**: Fixed (e.g., 50) or configurable via option?
-> configurable
5. **Template control IDs**: Prefix format `"looper:{looper}:{line}:{ctrl}"` — any conflicts with existing ID scheme?
-> YES
6. **Refresh behavior**: `k.looper.refresh` replaces all rows (like Kalipso) or merges? Replace for simplicity.
-> replace
---

# 3. Chart Control (Chart.js)

## Overview

Add a **Chart control** to KALUA using **Chart.js v4.x** for data visualization. Supports line, bar, pie, doughnut, scatter, radar, and area charts with full Chart.js options pass-through, dynamic data updates, and interactive events.

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        Go Host (Session)                        │
│  ┌─────────────────┐    ┌──────────────────┐                  │
│  │ k.ctrl.chart    │    │ k.chart.set_data │                  │
│  │ (type, options) │───▶│ (datasets, labels)                 │
│  └────────┬────────┘    └────────┬─────────┘                  │
│           │                      │                             │
│           ▼                      ▼                             │
│  ┌─────────────────────────────────────────┐                  │
│  │ renderControl: <canvas> + data attrs    │                  │
│  │ data-k-chart-config (JSON)              │                  │
│  └─────────────────┬───────────────────────┘                  │
└────────────────────┼──────────────────────────────────────────┘
                     │ WebSocket update_control
                     ▼
┌─────────────────────────────────────────────────────────────────┐
│                     Browser (app.js + Chart.js)                 │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │ Chart Instance Map keyed by selector                      │   │
│  │ On render: new Chart(ctx, config)                         │   │
│  │ On update: chart.data = newData; chart.update()           │   │
│  │ On destroy: chart.destroy() on close_form                 │   │
│  │ Events: click, hover → tabulator_* style events           │   │
│  └──────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

## Supported Chart Types (Chart.js v4.x)

| Chart Type | Chart.js Type | Use Case |
|------------|---------------|----------|
| Line | `'line'` | Trends over time |
| Bar | `'bar'` | Categorical comparison |
| Horizontal Bar | `'bar'` with `indexAxis: 'y'` | Long labels |
| Pie | `'pie'` | Proportions |
| Doughnut | `'doughnut'` | Proportions with center |
| Scatter | `'scatter'` | Correlations |
| Radar | `'radar'` | Multi-dimensional |
| Area | `'line'` with `fill: true` | Cumulative |

## API Surface

### New Constructor: `k.ctrl.chart(form, name, opts)`

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `type` | string | `"line"` | Chart type: `line`, `bar`, `hbar`, `pie`, `doughnut`, `scatter`, `radar`, `area` |
| `title` | string | `""` | Chart title |
| `width` | number | `400` | Canvas width (px) |
| `height` | number | `300` | Canvas height (px) |
| `labels` | table | `{}` | X-axis labels (array) |
| `datasets` | table | `{}` | Data series (array of dataset tables) |
| `options` | table | `{}` | Chart.js options pass-through |
| `responsive` | boolean | `true` | Responsive resize |
| `maintainAspectRatio` | boolean | `false` | Aspect ratio handling |
| `legend` | boolean | `true` | Show legend |
| `legendPosition` | string | `"top"` | `top`, `bottom`, `left`, `right` |
| `animation` | boolean | `true` | Enable animations |
| `stacked` | boolean | `false` | Stacked bars/areas |

### Dataset Structure (each dataset in `datasets` array)

| Property | Type | Description |
|----------|------|-------------|
| `label` | string | Series name (for legend) |
| `data` | table | Array of numbers |
| `backgroundColor` | string/table | Fill color(s) |
| `borderColor` | string/table | Border color(s) |
| `borderWidth` | number | Border width (default: 2) |
| `fill` | boolean | Area fill (line/area) |
| `tension` | number | Line curve (0-1, default: 0.2) |
| `pointRadius` | number | Point size (line/scatter) |
| `type` | string | Override chart type for this dataset (combo charts) |

### New Functions

| Function | Signature | Description |
|----------|-----------|-------------|
| `k.chart.set_data` | `(form, name, {labels, datasets})` | Bulk replace all data |
| `k.chart.add_dataset` | `(form, name, dataset)` | Append a new dataset |
| `k.chart.remove_dataset` | `(form, name, index)` | Remove dataset by index |
| `k.chart.update_dataset` | `(form, name, index, dataset)` | Update specific dataset |
| `k.chart.set_labels` | `(form, name, labels)` | Replace X-axis labels |
| `k.chart.set_options` | `(form, name, options)` | Update Chart.js options |
| `k.chart.get_image` | `(form, name)` → string | Base64 PNG data URL |
| `k.chart.resize` | `(form, name, width, height)` | Resize canvas |

### Events (via `k.form.on(form, ctrl, event, fn)`)

| Event | Payload |
|-------|---------|
| `chart_click` | `{element: dataset_index, index: point_index, value: number}` |
| `chart_hover` | `{element: dataset_index, index: point_index, value: number}` |
| `chart_legend_click` | `{dataset_index: number}` |

### WebSocket Message Types

| Direction | Type | Payload |
|-----------|------|---------|
| Go → Browser | `chart_update` | `{selector, data: {labels, datasets}, options?}` |
| Go → Browser | `chart_options` | `{selector, options}` |
| Go → Browser | `chart_destroy` | `{selector}` |
| Browser → Go | `chart_click` | `{form, ctrl, event, value: {dataset_index, index, value}}` |
| Browser → Go | `chart_hover` | `{form, ctrl, event, value: {dataset_index, index, value}}` |
| Browser → Go | `chart_legend_click` | `{form, ctrl, event, value: {dataset_index}}` |

### Example Usage

```lua
function main()
    k.form.new("dashboard", {title = "Sales Dashboard", layout = "grid"})
    
    -- Line chart
    k.ctrl.chart("dashboard", "sales_trend", {
        type = "line",
        title = "Monthly Sales",
        width = 600,
        height = 300,
        labels = {"Jan", "Feb", "Mar", "Apr", "May", "Jun"},
        datasets = {
            {
                label = "Revenue",
                data = {12000, 19000, 15000, 25000, 22000, 30000},
                borderColor = "#1976d2",
                backgroundColor = "rgba(25, 118, 210, 0.1)",
                fill = true,
                tension = 0.3
            },
            {
                label = "Orders",
                data = {120, 180, 150, 250, 220, 300},
                borderColor = "#d32f2f",
                backgroundColor = "rgba(211, 47, 47, 0.1)",
                fill = false,
                tension = 0.3
            }
        },
        options = {
            scales = {
                y = {beginAtZero = true, title = {display = true, text = "Amount"}}
            }
        }
    })
    
    -- Bar chart
    k.ctrl.chart("dashboard", "category_sales", {
        type = "bar",
        title = "Sales by Category",
        labels = {"Electronics", "Clothing", "Home", "Sports"},
        datasets = {{
            label = "Q1 Sales",
            data = {45000, 32000, 28000, 19000},
            backgroundColor = {"#1976d2", "#388e3c", "#f57c00", "#7b1fa2"}
        }}
    })
    
    k.form.show("dashboard")
    
    -- Dynamic update after 5 seconds
    k.timer_start("update_chart", 5000, function()
        k.chart.set_data("dashboard", "sales_trend", {
            labels = {"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul"},
            datasets = {{
                label = "Revenue",
                data = {12000, 19000, 15000, 25000, 22000, 30000, 35000},
                borderColor = "#1976d2",
                fill = true
            }}
        })
    end)
end
```

## Key Behavior Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| **Chart.js version** | v4.x (latest stable) | Modern, maintained, TypeScript support |
| **Data format** | Chart.js native config | Direct pass-through, no custom mapping needed |
| **Canvas sizing** | Responsive container | Fits form layout (grid/flex) |
| **Cleanup** | Destroy on `close_form` | Prevent memory leaks |
| **Updates** | `chart.data = newData; chart.update('none')` | Smooth animations optional |
| **Combo charts** | Per-dataset `type` override | Chart.js native support |
| **Events** | Click, hover, legend | Most common interactive needs |
| **Export** | `toDataURL('image/png')` | Base64 for reports/printing |

## Implementation Phases

| Phase | Description | Days |
|-------|-------------|------|
| 1 | Assets & Go Rendering | 2 |
| 2 | Client-Side Chart.js Integration | 3 |
| 3 | Data Management API (set_data, add_dataset, etc.) | 2 |
| 4 | Events & Interaction (click, hover, legend) | 1 |
| 5 | Advanced: Image Export, Combo Charts | 1 |
| 6 | Checker, LSP, Tests, CSS | 1 |
| **Total** | | **10** |

## File Changes

| File | Changes |
|------|---------|
| `internal/web/assets/chartjs/*` | **New** - embedded Chart.js v4.x assets |
| `internal/web/templates/shell.html` | Add Chart.js script tag |
| `internal/bindings/forms.go` | Register `k.ctrl.chart`, `k.chart.*` APIs, `renderChart` function |
| `internal/session/session.go` | Handle `chart_destroy` on `close_form`, new inbox types |
| `internal/web/assets/app.js` | Chart instance Map, init/update/destroy, event bridging |
| `internal/web/assets/kalua.css` | Chart container styles, responsive canvas |
| `internal/checker/checker.go` | Validate chart options, datasets structure |
| `internal/lsp/server.go` | Completions for `k.ctrl.chart`, `k.chart.*`, dataset properties |

## CSS Additions (kalua.css)

```css
/* Chart control */
.kalua-chart-container {
    position: relative;
    width: 100%;
    height: 100%;
}
.kalua-chart-container canvas {
    display: block;
    width: 100% !important;
    height: 100% !important;
}

/* Responsive: chart fills control width */
.kalua-control > .kalua-chart-container {
    flex: 1;
    min-height: 200px;
}
```

## Dependencies

- **Chart.js v4.4+** (embedded, ~180KB minified)
- No new Go dependencies
- Chart.js is pure JS, no WASM needed

---

# 4. Extended Form Controls (Textbox, Label, Image)

## Overview
Enhance existing controls and add a new image control:
1. **Textbox**: Add `multiline` (textarea) and `datetime` (calendar/time picker) modes
2. **Label**: Add `multiline` option for multi-line text
3. **Image**: New control mapping to `<img>` tag

## Current Control System
- Controls registered in `registerControls()` → `addControl()` stores options in Lua table
- Rendering in `renderControl()` switch on `ctrlType` (forms.go:688-839)
- Client updates via WebSocket `update_control` with full HTML replacement

---

## 4.1 Textbox Enhancements

### New Options for `k.ctrl.textbox(form, name, opts)`

| Option | Type | Description |
|--------|------|-------------|
| `multiline` | `boolean` | Render as `<textarea>` instead of `<input>` |
| `rows` | `number` | Textarea rows (default: 4) |
| `cols` | `number` | Textarea cols (default: 50) |
| `datetime` | `boolean` / `table` | Enable date/time picker |
| `datetime.mode` | `string` | `"date"`, `"time"`, `"datetime"` (default: `"datetime"`) |
| `datetime.format` | `string` | Display format (default: `"YYYY-MM-DD HH:MM"` for datetime) |
| `datetime.min` | `string` | Minimum selectable date/time |
| `datetime.max` | `string` | Maximum selectable date/time |
| `datetime.step` | `number` | Time step in minutes (default: 15) |

### Behavior
- **multiline=true**: Renders `<textarea class="kalua-textarea">` with `rows`/`cols`
- **datetime=true**: Renders `<input type="text" class="kalua-input kalua-datetime">` + flatpickr (embedded)
- **Both false** (default): Current `<input type="text">` behavior

### Date/Time Picker Library
- **flatpickr** (lightweight, no dependencies, ~20KB gzipped)
- Embedded via `//go:embed` in `internal/web/assets/flatpickr/`
- Theme: match KALUA CSS

### Value Format
- **date**: `"YYYY-MM-DD"`
- **time**: `"HH:MM"` (24h)
- **datetime**: `"YYYY-MM-DD HH:MM"`
- Lua side: store as string, parse via `k.date_parse()` if needed

---

## 4.2 Label Enhancement

### New Option for `k.ctrl.label(form, name, opts)`

| Option | Type | Description |
|--------|------|-------------|
| `multiline` | `boolean` | Allow line breaks in label text |
| `text` | `string` | Label text (existing, renamed from `label` for label control) |

### Behavior
- **multiline=false** (default): Current `<label>` rendering, single line
- **multiline=true**: Renders `<div class="kalua-label kalua-label-multiline">` with `white-space: pre-wrap` to preserve `\n`

### CSS Addition
```css
.kalua-label-multiline {
    white-space: pre-wrap;
    word-wrap: break-word;
}
```

---

## 4.3 Image Control (New)

### New Constructor: `k.ctrl.image(form, name, opts)`

| Option | Type | Description |
|--------|------|-------------|
| `src` | `string` | Image URL or data URI (required) |
| `alt` | `string` | Alt text (default: "") |
| `width` | `number/string` | Width in px or % (default: auto) |
| `height` | `number/string` | Height in px or % (default: auto) |
| `fit` | `string` | `"cover"`, `"contain"`, `"fill"`, `"scale-down"`, `"none"` (default: `"contain"`) |
| `clickable` | `boolean` | If true, wraps in `<a>` or adds click handler (default: false) |
| `onclick` | `function` | Click handler (requires `clickable=true`) |
| `enabled` | `boolean` | Show/hide (default: true) |
| `visible` | `boolean` | Show/hide (default: true) |

### Rendering
```html
<div class="kalua-control kalua-image-container" ...>
    <img class="kalua-image" id="c:form:name" 
         src="..." alt="..." 
         style="width:...; height:...; object-fit:..." 
         data-k-form="form" data-k-ctrl="name">
</div>
```

### Click Handling

The image control supports click events when `clickable=true` is set. The click can be handled in two ways:

**1. Inline onclick handler (constructor option):**
```lua
k.ctrl.image("main", "logo", {
    src = "assets/logo.png",
    clickable = true,
    onclick = function()
        k.msgbox("Logo clicked!")
    end,
})
```

**2. Event-based handler via `k.form.on` (recommended for consistency):**
```lua
k.ctrl.image("main", "logo", {
    src = "assets/logo.png",
    clickable = true,
})

k.form.on("main", "logo", "click", function()
    k.msgbox("Logo clicked!")
end)
```

The image control supports custom click data via the `onclick_data` option, which allows passing custom data with the click event:

```lua
k.ctrl.image("main", "logo", {
    src = "assets/logo.png",
    clickable = true,
    onclick_data = { action = "open_modal", modal = "about" },
})
```

When clicked, the event value passed to the handler will contain the custom data.

### Click Event Value

By default, clicking an image control sends an empty value `{}`. If `onclick_data` is provided, the custom data is passed as the event value.

### Dynamic Update
- `k.ctrl.set_value(form, name, new_src)` → updates `src` attribute
- `k.ctrl.set_property(form, name, "src", new_src)` → same
- `k.ctrl.set_property(form, name, "alt", ...)` etc.

### Visual Feedback

When `clickable=true`, the image gets visual feedback:
- **Hover**: Slight scale up (1.02x) with subtle shadow
- **Active/Click**: Slight scale down (0.98x)
- **Cursor**: Pointer cursor indicates clickability

---

## Implementation Plan

### Phase 1: Textbox - Multiline (1 day)
- [ ] Modify `addControl` to store `multiline`, `rows`, `cols` options
- [ ] Update `renderControl` case `"textbox"` to render `<textarea>` when `multiline=true`
- [ ] Add `kalua-textarea` CSS (extends existing)
- [ ] Update checker/LSP for new options

### Phase 2: Textbox - DateTime Picker (2 days)
- [ ] Download & embed flatpickr (JS + CSS + themes)
- [ ] Update `shell.html` to include flatpickr assets
- [ ] Modify `addControl` to store `datetime` options
- [ ] Update `renderControl` to add `kalua-datetime` class + `data-k-datetime-options` attribute
- [ ] In `app.js`: initialize flatpickr on `.kalua-datetime` elements after render
- [ ] Handle dynamic updates (re-init on `update_control`)
- [ ] Add flatpickr theme CSS matching KALUA

### Phase 3: Label - Multiline (0.5 day)
- [ ] Modify `addControl` to store `multiline` for label type
- [ ] Update `renderControl` case `"label"` for multiline rendering
- [ ] Add CSS for `kalua-label-multiline`

### Phase 4: Image Control (1 day)
- [ ] Register `k.ctrl.image` in `registerControls`
- [ ] Add `"image"` case in `renderControl`
- [ ] Support `set_value`/`set_property` for dynamic src/alt
- [ ] Add CSS for `kalua-image-container`, `kalua-image`
- [ ] Update checker/LSP for new control

### Phase 5: Checker, LSP, Tests (1 day)
- [ ] Checker: validate new options for textbox, label, image
- [ ] LSP: completions for new options
- [ ] Unit tests for rendering

---

## File Changes

| File | Changes |
|------|---------|
| `internal/bindings/forms.go` | `addControl` option storage, `renderControl` cases for textbox/label/image |
| `internal/bindings/api_doc.go` | Document new options and `k.ctrl.image` |
| `internal/web/assets/app.js` | flatpickr initialization, image handling |
| `internal/web/assets/kalua.css` | textarea, datetime, label-multiline, image styles |
| `internal/web/templates/shell.html` | Include flatpickr CSS/JS |
| `internal/web/assets/flatpickr/*` | **New** - embedded flatpickr assets |
| `internal/checker/checker.go` | Validate new options |
| `internal/lsp/server.go` | Completions for new API |

---

## New WebSocket Message Types (No new types needed)
- Uses existing `update_control` with full HTML replacement
- flatpickr auto-initialized on new elements

---

## Dependencies
- **flatpickr** v4.6+ (embedded, ~20KB gzipped)
- No new Go dependencies

---

## Estimated Timeline: 5.5 days (after Looper)

| Phase | Days |
|-------|------|
| 1: Textbox multiline | 1 |
| 2: Textbox datetime | 2 |
| 3: Label multiline | 0.5 |
| 4: Image control | 1 |
| 5: Checker, LSP, tests | 1 |

---

## Open Questions

1. **DateTime format**: Use ISO 8601 (`YYYY-MM-DDTHH:MM:SS`) internally, or keep display format? Display format in UI, ISO in value for parsing.
-> YES
2. **flatpickr locale**: Support multiple locales? Embed all or just en-US? Start with en-US, add locale option later.
-> Start with en-US, add locale option later.
3. **Image src**: Support data URIs for embedded images? Yes, just pass through to `src`.
-> YES
4. **Image click**: `clickable=true` + `onclick` handler — should it use `k.form.on` or inline `onclick`? Use `k.form.on` for consistency.
-> k.form.on
5. **Textarea resize**: Allow user resize? Default `resize: vertical` in CSS.
-> NO, set the number of visible lines in control configuration
6. **Datetime value binding**: When user picks date, fire `whenever_modified` event? Yes, consistent with textbox.
-> YES
7. **Initial value for datetime**: If `value` provided, pre-fill flatpickr? Yes, set `defaultDate` in flatpickr config.
-> YES
---

# 5. Form Builder (Visual Designer)

## Executive Summary

A visual form builder for KALUA that allows drag-and-drop form design with live preview, property editing, Lua import/export, and validation. This is a **design-time tool** — the builder never executes the app; it renders controls through the **real Go renderer** in-process for a pixel-accurate preview.

---

## Decisions (locked)

| Decision | Choice |
|----------|--------|
| **Hosting** | **Option B — standalone web app** served by a new `KALUA builder` subcommand (VS Code webview dropped) |
| **Domain** | `127.0.0.1`-bound developer tool (like `run`), `--port`, `--no-browser` flags |
| **Layouts (MVP)** | **Vertical + Grid with cells**, full runtime parity (`align`, `gap`, `cells`, per-control `cell`/`align`) |
| **Preview** | **Hybrid** — JS sim only for palette thumbnails / drag ghosts; the **Go renderer** endpoint is the canvas source of truth (debounced) |
| **Controls (MVP)** | **All 11 runtime types**, incl. `chart` and `looper` |
| **Event handlers** | Not edited / exported; preserved as read-only metadata; export emits TODO comments (`k.form.on` / `onclick`) |
| **Multi-form** | One form per workspace file (per spec) |
| **Save model** | CLI path + Go HTTP **GET/PUT** of the workspace file |
| **File format** | `.kalua-form.json` primary; Lua is export-only (and import-only) |
| **Cells (2026-09-07)** | **Ordered array (v2)** — cell order survives import/export/preview; array form is the runtime's canonical repr. v1 object-form docs auto-migrate on load |
| **Layout edit (2026-09-07)** | Rename / reorder / delete cells in the inspector; border width+color; grid cells are clickable dashed regions in the canvas; delete-cell reassigns its controls to `main` |
| **Deletion (2026-09-07)** | Controls: list `×`, editor-header `×`, and **Delete/Backspace** (when no input is focused) |
| **Undo/Redo (2026-09-07)** | One-step, client-side snapshot history (coalesced typing bursts), toolbar buttons + Ctrl/Cmd+Z / Ctrl/Cmd+Shift+Z / Ctrl/Cmd+Y; reset on New/Load |

---

## Current Form Structure Analysis

### Live runtime model

Forms and controls live entirely in the gopher-lua state. A form is a Lua global table:

```lua
form := {
    name, title, layout, align, gap, cells,
    controls = { name = ctrlTable, ... },
    handlers = { name = { event = fn, ... } },   -- k.form.on / constructor onclick
    order    = { "lbl1", "txt1", ... }            -- creation order (rendering)
}
```

`renderForm` (`internal/bindings/forms.go`) walks `order` (vertical) or buckets controls into grid `cells`; `renderControl` switches on the control `type`. The builder reuses both verbatim for preview.

### Lua → JSON mapping

```lua
-- Lua script
k.form.new("main", {title="Test Form", layout="vertical"})
k.ctrl.label("main", "lbl1", {text="Hello KALUA!"})
k.ctrl.textbox("main", "txt1", {label="Name", value="World"})
k.ctrl.button("main", "btn1", {label="Click Me", onclick=function() ... end})
```

```jsonc
// Equivalent JSON representation (v2 — cells are an ordered array)
{
  "version": 2,
  "form": {
    "name": "main",
    "title": "Test Form",
    "layout": "vertical",
    "align": "left",
    "gap": 16,
    "cells": [
      { "id": "header", "width": 12, "bg": "#f5f5f5",
        "border": { "width": 1, "color": "#ddd" }, "align": "center" },
      { "id": "sidebar", "width": 3 }
    ],
    "controls": [
      { "name": "lbl1", "type": "label",    "text": "Hello KALUA!" },
      { "name": "txt1", "type": "textbox",  "label": "Name", "value": "World" },
      { "name": "btn1", "type": "button",   "label": "Click Me", "cell": "header" }
      // onclick: not serializable — held in "handlers" metadata
    ],
    "handlers": { "btn1": ["click"] }
  }
}
```

**`controls` is an array in creation order** — both vertical rendering and grid bucketing depend on it. `handlers` is read-only metadata (event names per control) so the UI can show "1 event handler wired in Lua".

### Form Properties

| Property | Type | Default | Description |
|----------|------|---------|-------------|
| `name` | string | required | Form identifier (`^[a-zA-Z_][a-zA-Z0-9_]*$`) |
| `title` | string | `""` | Form title, rendered as `.kalua-form-title` |
| `layout` | `"vertical" \| "grid"` | `"vertical"` | Layout mode |
| `align` | `"left" \| "center" \| "right"` | `"left"` | Form-level alignment (vertical) / cell default |
| `gap` | number | `16` | Spacing between controls/cells (px, CSS `--kalua-gap`) |
| `cells` | `{cell_id → cell}` | `{}` | Grid cell definitions; absent ⇒ auto `main` cell (width 12) |

Cell definition: `{ width (1-12), bg|background, border {width, color}, align }`.
Runtime accepts array (`{id=..., ...}`) or map form; the builder writes **map form** in JSON and exports **array form** for deterministic order.

### Control Properties by Type

Every control shares: `name`, `type`, `label` (label control uses `text`), `value`, `enabled`, `visible`, `class`, `cell`, `align`.

| Control | Optional Props | Runtime notes |
|---------|----------------|---------------|
| **label** | `text` (required), `multiline` | `text` stored as `label`; multiline renders pre-wrap div |
| **textbox** | `label`, `value`, `multiline`, `rows`, `cols`, `datetime` (bool or `{mode,format,min,max,step}`) | datetime modes date/time/datetime |
| **button** | `label`, `class` | `onclick` ⇒ handler; `value` unused at render |
| **combo / list** | `label`, `items`, `value` | `items` = value→display map |
| **table** | `label`, `columns`, `rows`, `data`; advanced: `tabulator`, `tabulatorOptions`, `db`, `query`, `page_size`, `count_query`, `where`, `order_by` | Tabulator + DB-linking (§1) |
| **checkbox / radio** | `label`, `value`, `hidden_value` | `value` boolean drives `checked` |
| **image** | `src` (required), `alt`, `width`, `height`, `fit` (`cover\|contain\|fill\|scale-down\|none`), `clickable` | `onclick` ⇒ handler (requires `clickable=true`) |
| **chart** | `chart_type` (from `type`), `title`, `labels`, `datasets`, `options`, `width`, `height`, `responsive`, `maintainAspectRatio`, `legend`, `legendPosition`, `animation`, `stacked` | §3 Chart.js control |
| **looper** | `columns`, `db`, `query`, `links` (`[{column\|field, control, property}]`), `page_size`, `count_query`, `where`, `order_by` | §2 looper control |

### Items Format (combo/list)

Lua stores a **value→display map**; combo/list rendering iterates it in hash order (a gopher-lua table with string keys has no insertion order).

```json
{ "items": [ { "key": "key1", "display": "Display 1" }, ... ] }
```

**Decision:** JSON stores `items` as an ordered array; export emits a Lua map literal
(`items={key1="Display 1", key2="Display 2"}`). Display order in a live app follows
gopher-lua hash order — runtime behavior, out of builder scope.

### DB handles are runtime values

`db` is a handle from `k.connect_db` / `k.connect_sqlite` — not serializable. The
builder stores the *structure* opts (`query`, `where`, `order_by`, `page_size`,
`count_query`, `links`) and exports a `-- TODO: assign a DB handle (k.connect_db(...) / k.connect_sqlite(...))` comment in place of `db`.

---

## File Format (`.kalua-form.json`)

```json
{
  "$schema": "./kalua-form.schema.json",
  "version": 1,
  "form": {
    "name": "main",
    "title": "Test Form",
    "layout": "vertical",
    "align": "left",
    "gap": 16,
    "cells": {
      "header": { "width": 12, "bg": "#f5f5f5", "border": { "width": 1, "color": "#ddd" }, "align": "left" }
    },
    "controls": [
      { "name": "txt1", "type": "textbox", "label": "Name", "value": "World", "cell": "header" }
    ],
    "handlers": { "btn1": ["click"] }
  }
}
```

* `.kalua-form.json` is the builder-native (primary) format.
* Lua is an **export-only** (and import-only) format.
* `handlers` is informational; it round-trips but is never re-emitted as code.

---

## Architecture — Option B: Standalone Web App

Served by the KALUA binary, so the **real Go renderer** (`renderForm`/`renderControl` in `internal/bindings/forms.go`) is callable in-process — no HTML drift possible.

```
KALUA builder <file.lua|file.json> [--host 127.0.0.1] [--port 9001] [--no-browser]
```

```
┌─────────────────────────────────────────────────────────────────┐
│                KALUA builder <file>  (internal/builder)          │
│                                                                  │
│  ┌──────────────┐   ┌───────────────────┐   ┌─────────────────┐  │
│  │ /static/*    │   │ /api/form GET/PUT │   │ /api/preview    │  │
│  │ builder UI   │   │ workspace file    │   │ real renderer   │  │
│  └──────────────┘   └─────────┬─────────┘   │ renderForm()    │  │
│                               │             └────────┬────────┘  │
│  ┌──────────────┐   ┌─────────┴─────────┐             │           │
│  │ /api/import  │   │ /api/export       │   /api/validate        │
│  │ Lua → JSON   │   │ JSON → Lua        │   checker.Check()      │
│  └──────────────┘   └───────────────────┘   └─────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

### Package layout

```
internal/builder/
  builder.go       // Form JSON <-> LTable serialization (mirrors addControl storage)
  lua_export.go    // Form JSON -> Lua source
  lua_import.go    // Lua source -> Form JSON (gopher-lua AST walk)
  server.go        // HTTP routes + preview/validate handlers
  assets/          // //go:embed: index.html, builder.js, builder.css
```

### HTTP API

| Method & Path | Purpose |
|---------------|---------|
| `GET /` | builder shell (`index.html`) |
| `GET /static/*` | builder assets for the app and the preview iframe (incl. KALUA's `kalua.css`) |
| `GET /api/form` | workspace parsed to Form JSON (`.json` read; `.lua` imported on demand) |
| `PUT /api/form` | save Form JSON — writes `.kalua-form.json`; for a `.lua` workspace defaults to a sidecar `.kalua-form.json` |
| `POST /api/export` | `{form}` → `{lua}` |
| `POST /api/import` | `{lua}` → `{form, warnings[]}` |
| `POST /api/validate` | `{lua}` → `checker.Result` (syntax, unknown `k.*`, missing `main`) |
| `POST /api/preview` | `{form}` → form HTML from the **real Go renderer** (private `LState` built from JSON → `renderForm`) |

CSP mirrors run mode: `style-src 'self' 'unsafe-inline'` (controls carry inline
`style=""`), `script-src 'self'` (no inline scripts). Do **not** reuse serve-mode's
CSP, which blocks inline styles.

---

## Preview Rendering Strategy — Hybrid

* **Canvas = accurate.** Every model change debounces (~150 ms) a `POST /api/preview`; the returned form HTML (go-rendered) is injected into a sandboxed `<iframe>` with `/static/kalua.css`. Selection / click hit-tests use the real DOM ids `c:{form}:{ctrl}`.
* **JS sim = feedback only.** A TS port of `renderControl` powers palette thumbnails and drag ghosts. Because the canvas never depends on it, drift risk is contained.
* **Parity guards (tests):** Go test asserts canonical go-rendered markup per control type from a Form JSON fixture; a Node test mirrors the TS sim against the same fixtures when the builder UI is built.

---

## Builder UI Layout

```
+-----------------------------------------------------------------+
| Toolbar: [Save JSON] [Export Lua] [Validate] [Undo] [Redo] [Dev]|
+----------+--------------------------------------+----------------+
|          |                                      |                |
| Controls |        Preview Canvas (iframe)       | Property Editor|
| Palette  |  Go-rendered via /api/preview        |                |
|          |  +--------------------------------+  | +------------+ |
|  +------+ | |  Form Title                  |  | | Control:   | |
|  |label | | +--------------------------------+  | | | btn1      | |
|  +------+ | |  [lbl1] Hello KALUA!         |  | +------------+ |
|  |textbox| | |  [txt1] Name: [World______] |  | | Type:      | |
|  +------+ | |  [btn1] [Click Me]           |  | | button     | |
|  |button | | |                              |  | | Label:     | |
|  +------+ | |  Drag from palette → canvas.  |  | | Click Me   | |
|  |combo  | | |  Click to select.            |  | | Class:     | |
|  +------+ | +--------------------------------+  | | Enabled:  | |
|  |list   | |                                      | | [x]        | |
|  +------+ | +--------------------------------+  | | Visible:  | |
|  |table  | | |  Grid-mode: cell drop zones,  |  | | [x]        | |
|  +------+ | |  per-control cell assignment   |  | | Cell:     | |
|  |checkbox| | +--------------------------------+  | | [ header ]| |
|  +------+ |                                      | +------------+ |
|  |radio  | |                                      |                |
|  +------+ |                                      |                |
|  |image  | |                                      |                |
|  +------+ |                                      |                |
|  |chart  | |                                      |                |
|  +------+ |                                      |                |
|  |looper | |                                      |                |
|  +------+ |                                      |                |
|          |                                      |                |
+----------+--------------------------------------+----------------+
```

### Components

1. **Controls Palette** (left) — draggable control types (all 11)
2. **Preview Canvas** (center) — Go-rendered live form using the actual KALUA rendering + CSS
3. **Property Editor** (right) — dynamic form per selected control type, incl. special editors (items, cells, datetime, chart datasets/options, table columns/rows, looper links)

---

## Lua Import (Go AST)

Reuses the gopher-lua `parse`/`ast` packages (the same path `internal/checker` uses). Walk top-level statements of the provided Lua source:

* `k.form.new(name, opts)` → form props / cells
* `k.ctrl.<type>(form, name, opts)` → controls, **in order encountered**
* `k.form.on(form, ctrl, event, fn)` → recorded in `handlers` metadata (read-only; not exported back)
* anything else → warning `"non-form code will be lost on export"`

Import is structure-extraction only: logic, timers, and handlers are not editable and are dropped on export (with a confirmation warning when the export would overwrite the source file).

---

## Lua Export Generation

Template-based, matches runtime semantics exactly.

```lua
function main()
    k.form.new("main", {title="Test Form", layout="grid", align="left", gap=16,
        cells={
            {id="header", width=12, bg="#f5f5f5", border={width=1, color="#ddd"}, align="left"},
        }})
    k.ctrl.label("main", "lbl1", {text="Hello KALUA!"})
    k.ctrl.textbox("main", "txt1", {label="Name", value="World", cell="header"})
    -- TODO: add onclick handler: k.form.on("main", "btn1", "click", function() ... end)
    k.ctrl.button("main", "btn1", {label="Click Me"})
    k.form.show("main")
end
```

### Rules

* Label control emits `text=...`; all others emit `label=...`.
* `enabled=false` / `visible=false` emitted only when false; `class`, `cell`, `align` only when set; `value`/`rows`/`cols`/`multiline`/`datetime`/`hidden_value` when set.
* Button & clickable-image `onclick` ⇒ TODO comment (`k.form.on(...)`); option not emitted.
* Chart emits `labels`, `datasets`, `options` as Lua table literals; dataset maps recurse.
* Table: `columns` array + `rows` array; Tabulator opts (`tabulator=true`, `tabulatorOptions={...}`, `data={...}`) when present; DB opts (`query`, `page_size`, `count_query`, `where`, `order_by`) when present, with the DB-handle TODO.
* Looper: `columns`, `links` array (`{column=N, control="...", property="..."}`), DB opts as above.
* Grid cells exported as **array form** `cells={{id=..., ...}, ...}` for deterministic order.
* Proper Lua string escaping (quotes, backslashes, newlines), number/bool literals.
* Export is **whole-file**: importing `.lua` → edit → export overwrites the file and drops non-form code after a confirmation.

---

## Implementation Plan

### Phase 1: Foundation (3 days)
- [ ] `internal/builder` package: Form JSON ⇄ LTable serialization (mirrors `addControl` storage keys), TS types
- [ ] New `KALUA builder <file>` subcommand in `internal/cli` (+ usage, `--port`, `--host`, `--no-browser`)
- [ ] HTTP routes: `/`, `/static/*`, `/api/form` GET/PUT (workspace file), `/api/export`, `/api/import`, `/api/validate`, `/api/preview`
- [ ] Embedded builder shell (`index.html` + `builder.js` + `builder.css` via `//go:embed`)

### Phase 2: Preview Engine (3 days)
- [ ] `/api/preview` reuses `renderForm`/`renderControl` in-process (private `LState` built from Form JSON)
- [ ] Canvas iframe + `kalua.css`; debounced re-render on model change
- [ ] Selection/click hit-tests on real `c:{form}:{ctrl}` DOM ids
- [ ] JS sim ports of `renderControl` for palette thumbnails + drag ghosts

### Phase 3: Canvas & Palette (3 days)
- [ ] Drag-and-drop from palette to canvas; insert before/after/into
- [ ] Visual drop zones; reorder controls; delete; duplicate; copy/paste
- [ ] Vertical + grid rendering parity in the canvas

### Phase 4: Property Editor (4 days)
- [ ] Dynamic per-control-type editor for all 11 types
- [ ] Special editors: items (ordered key/display grid), cells (grid editor), datetime config, chart datasets/labels/options, table columns/rows, looper links, table Tabulator/DB advanced section
- [ ] Validation (required fields, unique names, name pattern)

### Phase 5: Grid Layout Editing (2 days)
- [ ] Cells editor (add/remove/rename, width, bg, border, align)
- [ ] Per-control `cell` assignment (dropdown from cells), `align`, form `gap`
- [ ] Mobile/desktop preview toggle

### Phase 6: Import / Export / Validate (3 days)
- [ ] Lua export generator (`lua_export.go`)
- [ ] Lua import via gopher-lua AST (`lua_import.go`)
- [ ] `/api/validate` preflight (syntax + unknown `k.*` + `main`) on save/export
- [ ] Round-trip tests: `.json` → `.lua` → `.json` equality; go-rendered markup parity fixtures

### Phase 7: Polish & Integration (2 days)
- [ ] Undo/redo stack, keyboard shortcuts
- [ ] Error handling & validation feedback UX
- [ ] Docs, parity tests, `make gen-api && make check-api` if `api_doc.go` touched

---

## Dependencies

- No heavy frameworks — vanilla TypeScript/JS + CSS
- Optional: `sortablejs` for drag-and-drop reordering (~20KB)
- Reuses: `kalua.css`, Go renderer (`forms.go`), `internal/checker` (validate), gopher-lua `ast`/`parse` (import)
- No new Go dependencies; new `//go:embed` block in `internal/builder`

---

## Risk Assessment

| Risk | Impact | Mitigation |
|------|--------|------------|
| Export/import drops non-form code | High | Explicit confirmation warning; `handlers` metadata read-only; structure-only extraction |
| JS-sim vs Go HTML drift | Low | Sim only feeds thumbnails/ghosts; canvas is Go-rendered; parity fixtures |
| Complex control property editors (chart, looper, tabulator) | Medium | Dedicated special editors in Phase 4 |
| Lua import correctness on exotic syntax | Medium | gopher-lua AST (same as checker); unknown calls → warnings, not failures |
| Builder preview matching runtime | None | Preview IS the Go renderer |

---

## Estimated Timeline: 20 days

| Phase | Days | Deliverable |
|-------|------|-------------|
| 1: Foundation | 3 | `KALUA builder` + API + shell |
| 2: Preview Engine | 3 | Live Go-rendered canvas |
| 3: Canvas & Palette | 3 | Drag-drop, reorder, delete |
| 4: Property Editor | 4 | Per-type editors incl. special editors |
| 5: Grid Layout Editing | 2 | Cells, cell assignment, alignment |
| 6: Import/Export/Validate | 3 | Lua round-trip + validation |
| 7: Polish | 2 | UX, undo/redo, docs |
| **Total** | **20** | **MVP Form Builder** |

---

## Future Enhancements (Post-MVP)

1. **Absolute / free positioning layout** — new runtime layout mode + builder support
2. **Looper template editor** — visual template for looper rows
3. **Tabulator column wizard** — visual column/format configuration
4. **Theme / dark mode preview**, **collaboration**, **full app skeleton export**

---

## Open Questions & Decisions (resolved)

| Question | Decision |
|----------|----------|
| Builder hosting | **Option B standalone** (`KALUA builder`) |
| Layout system | **Vertical + Grid with cells** (full runtime parity) |
| Event handlers | Not edited/exported; TODO comments; `handlers` metadata read-only |
| Table/Chart/Looper support | **All 11 types in MVP** (chart & looper included); Tabulator/DB-linking shown as advanced sections |
| Multi-form support | Single form per file |
| Live sync | Manual save/export; JSON auto-saved to workspace via `PUT /api/form` |
| CSS framework | Plain CSS (match KALUA style) |
| Preview accuracy | Hybrid (Go-rendered canvas + JS sim ghosts) |

---

# 6. Enhanced Form Layout System (Vertical + Grid with Cells)

## Overview

Enhance KALUA's form layout system with two powerful modes:

1. **Vertical** (enhanced): Form-level + per-control alignment (`left`/`center`/`right`)
2. **Grid**: Cell-based 12-column Bootstrap-like layout with configurable cells

Key features:
- Cells are containers with `width` (1-12), `bg`, `border`, `align` attributes
- Controls assigned to cells via `cell` property
- Mobile-responsive collapse (< 600px → single column)
- Backward compatible: `layout="grid"` without cells creates auto "main" cell (width=12)

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        Go Host (Session)                        │
│  ┌─────────────────┐    ┌──────────────────┐                  │
│  │ k.form.new      │    │ k.ctrl.*         │                  │
│  │ (layout, cells) │───▶│ (cell="...")     │                  │
│  └────────┬────────┘    └────────┬─────────┘                  │
│           │                      │                             │
│           ▼                      ▼                             │
│  ┌─────────────────────────────────────────┐                  │
│  │ renderForm: iterates cells, assigns     │                  │
│  │ controls to cell containers, renders    │                  │
│  │ HTML with data-k-cell, grid-column      │                  │
│  └─────────────────┬───────────────────────┘                  │
└────────────────────┼──────────────────────────────────────────┘
                     │ WebSocket update_control / render_form
                     ▼
┌─────────────────────────────────────────────────────────────────┐
│                     Browser (app.js + CSS)                      │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │ .kalua-form[layout="grid"] { grid-template-columns:      │   │
│  │   repeat(12, 1fr); gap: var(--kalua-gap); }              │   │
│  │ .kalua-cell { display: flex; flex-direction: column;     │   │
│  │   grid-column: span N; }                                 │   │
│  │ @media (max-width: 600px) { grid-template-columns: 1fr } │   │
│  └──────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

## API Surface

### Extended Options for `k.form.new(form, name, opts)`

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `layout` | string | `"vertical"` | `"vertical"` or `"grid"` |
| `align` | string | `"left"` | Default alignment: `left`, `center`, `right` |
| `gap` | number | `16` | Spacing between controls/cells (px) |
| `cells` | table | `{}` | Cell definitions keyed by `cell_id` |

### Cell Definition (value in `cells` table)

| Attribute | Type | Default | Description |
|-----------|------|---------|-------------|
| `width` | number | `12` | Column span 1-12 |
| `bg` / `background` | string | transparent | CSS background color |
| `border` | table | nil | Shorthand `{width=N, color="..."}` |
| `align` | string | `"left"` | Widget alignment in cell: `left`, `center`, `right` |

### Control Property (via constructor opts or `k.ctrl.set_property`)

| Property | Type | Description |
|----------|------|-------------|
| `cell` | string | Assign control to grid cell (`cell_id`) |
| `align` | string | Per-control alignment override: `left`, `center`, `right` |

### Example Usage

```lua
-- Vertical with center alignment
k.form.new("login", {title = "Login", layout = "vertical", align = "center"})

-- Grid with cells
k.form.new("dashboard", {
    title = "Dashboard",
    layout = "grid",
    gap = 16,
    cells = {
        header  = {width = 12, bg = "#f5f5f5", border = {width=1, color="#ddd"}, align="center"},
        sidebar = {width = 3,  bg = "#fff", align="left"},
        main    = {width = 9,  bg = "#fff", align="center"},
        footer  = {width = 12, bg = "#fafafa", border = {width=2, color="#ccc"}}
    }
})

-- Assign controls to cells
k.ctrl.textbox("dashboard", "search", {label = "Search", cell = "header"})
k.ctrl.list("dashboard", "menu", {items = {...}, cell = "sidebar"})
k.ctrl.table("dashboard", "data", {columns = {...}, cell = "main"})

-- Or via set_property
k.ctrl.set_property("dashboard", "search", "cell", "header")
k.ctrl.set_property("dashboard", "search", "align", "right")  -- override cell default
```

## Key Behavior Decisions

| Decision | Implementation |
|----------|----------------|
| Cell ordering | As defined in `cells` table (insertion order) |
| Mobile collapse | `< 600px` → single column, all cells span 12 |
| Nested cells | Not supported |
| Gap | Global form `gap` (CSS var `--kalua-gap`) |
| Backward compat | `layout="grid"` without cells → auto single cell `"main"` width=12 |
| Alignment | Per-control `align` overrides cell/form default |
| Control property | `cell` (not `cell_id`) |
| Border syntax | Shorthand `border = {width=N, color="..."}` |

## Implementation Phases

| Phase | Description | Days |
|-------|-------------|------|
| 1 | Vertical alignment (form + per-control `align`) | 1 |
| 2 | Grid cells data model (parse `cells`, store in form table) | 1 |
| 3 | `renderForm` cell rendering logic | 2 |
| 4 | `renderControl` per-control alignment + cell assignment | 1 |
| 5 | CSS for cells, mobile collapse, vertical alignment | 1 |
| 6 | `ctrl.set_property` support for `cell`/`align` | 1 |
| 7 | API docs + backward compat testing | 0.5 |
| **Total** | | **7.5** |

## File Changes

| File | Changes |
|------|---------|
| `internal/bindings/forms.go` | Parse `cells`/`align`/`gap` in `registerForms`; `addControl` stores `cell`; `renderForm` iterates cells, assigns controls; `renderControl` handles `align`; `ctrl.set_property` handles `cell`/`align` |
| `internal/web/assets/kalua.css` | Grid cell styles, `--kalua-gap` CSS var, mobile media query, vertical align attributes |
| `internal/bindings/api_doc.go` | Document new form options, cell schema, control properties |

## CSS Additions (kalua.css)

```css
/* Vertical form alignment */
.kalua-form[layout="vertical"][align="center"] { align-items: center; }
.kalua-form[layout="vertical"][align="right"] { align-items: flex-end; }

/* Grid form with cells */
.kalua-form[layout="grid"] {
    display: grid;
    grid-template-columns: repeat(12, 1fr);
    gap: var(--kalua-gap, 16px);
}

/* Cell container */
.kalua-cell {
    display: flex;
    flex-direction: column;
    gap: 8px;
    padding: 12px;
}
.kalua-cell[align="center"] { align-items: center; }
.kalua-cell[align="right"] { align-items: flex-end; }

/* Mobile collapse */
@media (max-width: 600px) {
    .kalua-form[layout="grid"] {
        grid-template-columns: 1fr;
    }
    .kalua-cell {
        grid-column: span 12 !important;
    }
}
```

---

## Execution Order Summary

| Phase | Feature | Timeline |
|-------|---------|----------|
| 1 | Table Widget (Tabulator) | 10 days |
| 2 | Looper Control | 10 days |
| 3 | Chart Control (Chart.js) | 10 days |
| 4 | Extended Controls (Textbox/Label/Image) | 5.5 days |
| 5 | Enhanced Form Layout System | 7.5 days |
| 6 | Form Builder | 20 days |
| 7 | CRUD Grid Control (Tabulator + Forms) | 30 days |
| **Total** | | **~93 days** |

---

# 7. CRUD Grid Control (Integrated Table + Form)

## Overview

Introduce a new `k.ctrl.grid(form, name, opts)` control that combines:
- **Tabular view** over a database table (Tabulator-powered, DB-linked)
- **Row actions**: View / Edit / Delete per row
- **Global actions**: New Record, Batch Delete
- **Column management**: visibility, filtering, sorting, custom renderers
- **Detail/Edit form**: modal form for view/edit operations (reuses `k.form.show` modal)

This is a higher-level "CRUD Grid" widget that subsumes the DB-linked table pattern (`k.ctrl.table` with `tabulator=true` + `db` + `query`) and adds integrated form-based editing.

## API Surface

### Control Creation

```lua
k.ctrl.grid("main", "users", {
    -- Data source (DB-linked, like table/looper)
    db = "main",                    -- named DB handle from --db
    query = "SELECT * FROM users",  -- base SELECT
    count_query = "SELECT COUNT(*) FROM users",
    page_size = 25,
    where = "",                     -- additional WHERE clause
    order_by = "id DESC",
    pk_field = "id",                -- explicit PK (or inferred from query)

    -- Columns (Tabulator column definitions)
    columns = {
        {field="id", title="ID", sortable=true, headerFilter="number", visible=true},
        {field="name", title="Name", sortable=true, headerFilter="text", visible=true},
        {field="email", title="Email", sortable=true, headerFilter="text", visible=true},
        {field="status", title="Status", sortable=true, headerFilter="text", visible=true},
        {field="created_at", title="Created", sortable=true, headerFilter="none", visible=true},
    },

    -- Row actions (displayed as action buttons in a dedicated column)
    row_actions = {
        view = true,      -- show "View" button → opens modal with read-only form
        edit = true,      -- show "Edit" button → opens modal with editable form
        delete = true,    -- show "Delete" button → confirms then deletes row
        -- Custom actions: {label="Archive", icon="archive", onclick=function(row) ... end}
    },

    -- Global actions (toolbar above table)
    global_actions = {
        new_record = true,           -- "New" button → opens empty edit form
        batch_delete = true,         -- "Delete Selected" button (enabled when rows selected)
        -- Custom actions: {label="Export CSV", onclick=function(selected_rows) ... end}
    },

    -- Detail/Edit form definition (inline or referenced)
    form = "user_form",              -- option A: reference existing form by name
    -- OR inline form definition:
    form = {
        title = "User Details",
        controls = {
            {type="textbox", name="id", label="ID", opts={enabled=false}},
            {type="textbox", name="name", label="Name"},
            {type="textbox", name="email", label="Email", opts={datetime={mode="email"}}},
            {type="combo", name="status", label="Status", opts={items={{"active","Active"},{"inactive","Inactive"}}}},
        }
    },

    -- Form behavior
    form_modal = true,               -- show form as modal (default true)
    form_gap = 10,                   -- modal gap percentage
    form_width = "80%",              -- modal width (optional)

    -- Column visibility control
    column_visibility = true,        -- show column visibility toggle in toolbar
    default_visible = {"id","name","email","status"}, -- default visible columns

    -- Advanced
    selection_mode = "multi",        -- "none" | "single" | "multi"
    row_click_action = "view",       -- action when clicking row: "view" | "edit" | "select" | "none"
})
```

### Grid Operations (`k.grid.*`)

| Function | Description |
|----------|-------------|
| `k.grid.set_db_source(form, name, {db, query, ...})` | Change data source |
| `k.grid.refresh(form, name)` | Reload page 1 |
| `k.grid.get_selected(form, name)` | Get selected rows (async) |
| `k.grid.get_row(form, name, pk)` | Get single row by PK (async) |
| `k.grid.delete_row(form, name, pk)` | Delete row by PK |
| `k.grid.batch_delete(form, name, {pks})` | Delete multiple rows |
| `k.grid.insert_row(form, name, data)` | Insert new row, returns PK |
| `k.grid.update_row(form, name, pk, data)` | Update row by PK |

### Form Events (via `k.form.on`)

```lua
-- Grid form events (fired on the grid's internal form)
k.form.on("users_grid", "__grid_form", "on_save", function(data)
    -- data = {mode="new|edit", pk=..., values={...}}
end)
k.form.on("users_grid", "__grid_form", "on_cancel", function(data)
    -- data = {mode="new|edit", pk=...}
end)

-- Row actions can also be handled via grid-level events
k.form.on("main", "users_grid", "on_row_view", function(row) ... end)
k.form.on("main", "users_grid", "on_row_edit", function(row) ... end)
k.form.on("main", "users_grid", "on_row_delete", function(pk) ... end)
k.form.on("main", "users_grid", "on_batch_delete", function(pks) ... end)
k.form.on("main", "users_grid", "on_new_record", function() ... end)
```

## Architecture

### 1. Go Runtime (`internal/bindings/grid.go` - new file)

- Control registration (`ctrl.grid`)
- Rendering (`renderGrid` in forms.go switch)
- Grid operations (`k.grid.*`) via `registerGridOps`
- PK inference from query / explicit `pk_field`

### 2. Client-Side (`internal/web/assets/app.js`)

- Grid instance management (`initGrids`, `createGrid`)
- Action column rendering (view/edit/delete buttons)
- Global action toolbar (new/batch delete/custom)
- Column visibility dropdown
- Form modal integration via `k.form.show` with `modal=true`
- Selection mode handling

### 3. Session Event Handling (`internal/session/session.go`)

New inbox message types:
- `grid_delete_row` — single row delete by PK
- `grid_batch_delete` — multiple row delete by PK array
- `grid_form_save` — form submission (mode: new/edit, pk, data)
- `grid_form_cancel` — form cancelled

Dispatcher routes to CRUD execution using existing DB bindings.

### 4. Form Definition for Grid

Two approaches supported:

**A. Reference Existing Form**
```lua
k.form.new("user_form", {title = "User"})
k.ctrl.textbox("user_form", "id", {label="ID", enabled=false})
k.ctrl.textbox("user_form", "name", {label="Name"})
k.ctrl.combo("user_form", "status", {label="Status", items={active="Active", inactive="Inactive"}})

k.ctrl.grid("main", "users", {
    db = "main",
    query = "SELECT * FROM users",
    form = "user_form",
    row_actions = {view=true, edit=true, delete=true},
    global_actions = {new_record=true, batch_delete=true},
})
```

**B. Inline Form Definition**
```lua
k.ctrl.grid("main", "users", {
    db = "main",
    query = "SELECT * FROM users",
    form = {
        title = "User Details",
        controls = {
            {type="textbox", name="id", label="ID", opts={enabled=false}},
            {type="textbox", name="name", label="Name"},
            {type="combo", name="status", label="Status", opts={items={{"active","Active"},{"inactive","Inactive"}}}},
        }
    },
    row_actions = {view=true, edit=true, delete=true},
    global_actions = {new_record=true, batch_delete=true},
})
```

**Auto-Generation**: If `form` omitted but row actions enabled, auto-generate form from query columns.

### 5. Builder Integration

- New "CRUD" tab in control-modal (extends table/looper editor)
- Form reference dropdown + inline form editor
- Live preview with sample data via `/api/grid/preview`
- Export/import grid config preserving inline form

### 6. CSS (`internal/web/assets/kalua.css`)

- Grid container, toolbar, action column styling
- Column visibility dropdown
- Form modal integration (reuses `.form-modal`)

## Implementation Status

| Phase | Description | Status |
|-------|-------------|--------|
| 1 | Core Grid Control (`grid.go`, rendering, basic Tabulator output) | ✅ Done |
| 2 | Client Interactions (action handlers, column visibility, toolbar, form modal) | ✅ Done |
| 3 | Server CRUD (DELETE/INSERT/UPDATE execution, PK handling) | ✅ Done |
| 4 | Form Integration (validation hooks, grid-level events, `k.grid.*` ops, form options) | ✅ Done |
| 5 | Builder Integration (CRUD editor tab, live preview, export/import) | ✅ Done |
| 6 | Documentation & Polish (API docs, USER_GUIDE, example app, tests) | ✅ Done |
| **Total** | | **30** |

## Key Design Decisions

| Decision | Rationale |
|----------|-----------|
| **New control type** (not extending table) | Clean separation; different semantics (CRUD vs read-only) |
| **Both inline + referenced forms** | Flexibility for simple vs complex cases |
| **Reuses `k.form.show` modal** | Consistent UX, leverages existing modal stack |
| **Explicit PK with inference** | Auto-detects single PK from query; composite PKs not supported |
| **Dedicated action column + row click** | Explicit + power-user shortcut; both coexist |
| **Validation hook returns `{ok,error}` table** | Non-throwing, explicit contract for `on_save`/`on_cancel` |
| **Default row/global actions** | `row_actions={view,edit,delete}`, `global_actions={new,batch_delete}` |
| **Row-level security (WHERE injection)** | **Cancelled** — not implementing |

## Migration Path

Existing `k.ctrl.table` with `tabulator=true` + DB link:
```lua
-- Before
k.ctrl.table("main", "users", {
    tabulator=true,
    db="main",
    query="SELECT * FROM users",
    columns={{field="id",title="ID"},{field="name",title="Name"}}
})

-- After (minimal)
k.ctrl.grid("main", "users", {
    db="main",
    query="SELECT * FROM users",
    columns={{field="id",title="ID"},{field="name",title="Name"}}
})
```
- `tabulator=true` implicit for grid
- `row_actions` defaults to `{view=true, edit=true, delete=true}`
- `global_actions` defaults to `{new_record=true, batch_delete=true}`

## Resolved Design Questions

| Question | Decision |
|----------|----------|
| **Composite PKs** | No — single-column PK only (auto-inferred or explicit `pk_field`) |
| **Soft deletes** | No — hard deletes only |
| **Custom row actions** | No — fixed View/Edit/Delete buttons only |
| **Inline cell editing** | No — modal form only |
| **Server-side validation** | Yes — `on_save`/`on_cancel` hooks on internal form; grid-level `on_row_*` events |
| **Row-level security (WHERE injection)** | Cancelled — not implementing |

## Completed Work Summary

### Phase 1: Core Grid Control (✅ Done)
- New `k.ctrl.grid` control with Tabulator integration
- DB-linked data source with server-side paging
- Column definitions, PK inference, selection modes

### Phase 2: Client Interactions (✅ Done)
- Action column (View/Edit/Delete buttons per row)
- Global toolbar (New Record, Batch Delete, Column Visibility)
- Row click actions (select/view/edit)
- Selection modes (multi/single/none)
- Modal form integration for view/edit/new

### Phase 3: Server CRUD (✅ Done)
- `GridInsert`, `GridUpdate`, `GridDeleteMany` in `internal/bindings/grid.go`
- PK inference from query or explicit `pk_field`
- Query parsing for table name extraction
- Async operations via session actor

### Phase 4: Form Integration (✅ Done)
- **Validation hooks**: `on_save`/`on_cancel` on internal grid form (`__kgrid_<form>_<ctrl>`)
- **Grid-level events**: `on_row_view`, `on_row_edit`, `on_row_delete`, `on_batch_delete`, `on_new_record` on main form
- **`k.grid.*` Lua bindings**: `get_selected`, `get_row`, `delete_row`, `batch_delete`, `insert_row`, `update_row`
- **Form options**: `form_width`, `default_visible`, default `row_actions`/`global_actions`
- Fixed `int` type handling in `toLuaValue` for proper numeric values

### Phase 5: Builder Integration (✅ Done)
- **CRUD tab** in builder modal (alongside Datasource/Table Setup/Row Template)
- Fields: DB selector, query, PK field, columns editor, row/global actions checkboxes, column visibility, form picker, selection mode, row click action, form width/gap, inline form editor
- Export/import of grid config in multi-form JSON

### Phase 6: Documentation & Polish (✅ Done)
- **API Documentation**: Added `CtrlFunc` entries for all `k.grid.*` ops in `api_doc.go`
- **USER_GUIDE.md**: Complete CRUD Grid Control section with examples, migration guide, options reference
- **E2E Tests** (`internal/session/grid_e2e_test.go`):
  - `TestGridFormOpenEditSaveCancel`: Full modal cycle (new/edit/view/cancel)
  - `TestGridRowDeleteBatchDelete`: Row and batch delete with refresh
  - `TestGridValidationRejection`: Server-side validation rejection
  - `TestGridDefaultActions`: Default actions when not specified
- **Validation**: All checks pass (`go test`, `go vet`, `node --check`, `make check-assets`, `KALUA check`)

## Example App
- `testdata/apps/grid_crud_demo.lua`: Complete CRUD grid demo with SQLite backend

### Phase 6: Documentation & Polish (Pending)

**6.1 API Documentation** (`internal/bindings/api_doc.go`)
- Add `CtrlFunc` entries for `grid`, `grid.set_db_source`, `grid.refresh`, `grid.get_selected`, `grid.get_row`, `grid.delete_row`, `grid.batch_delete`, `grid.insert_row`, `grid.update_row`
- Run `make gen-api && make check-api`

**6.2 User Guide** (`USER_GUIDE.md`)
- New section "CRUD Grid Control" with migration guide, complete examples, event hooks reference, `k.grid.*` operations

**6.3 Tests** (`internal/session/grid_e2e_test.go`)
- Full CRUD cycle: form open → save → `tabulator_refresh`
- View/edit/delete/batch delete flows
- Server-side validation rejection
- `k.grid.*` operations from Lua
- Default actions when not specified

**6.4 Code Quality**
- `go test ./...`, `go vet ./...`, `node --check`, `make check-assets` all pass

---

# 8. Login Control (`k.ctrl.login`)

## Overview

Introduce a new `k.ctrl.login(form, name, opts)` control: a **declarative login dialog** rendered as a single panel inside its form. The script declares it, shows the form modally, and receives the verified `users` record as a Lua map through a `k.form.on` event.

The control is a peer of `k.ctrl.grid` (section 7) — same shape (`addControl`, inline in the form, modal presentation via `k.form.show(name, {modal = true})`, results delivered by form events rather than a return value).

It is deliberately **not** a blocking `k.form.login(opts)` call: a control composes with the rest of the form system, needs no new coroutine suspension, and returns its result through the existing event-dispatch path.

```
┌──────────────────────────────────────────────────────────────────┐
│  .form-modal-overlay          (k.form.show(f, {modal = true}))   │
│  ┌────────────────────────────────────────────────────────────┐  │
│  │  Sign in                          ← control title          │  │
│  │  Enter your credentials           ← control subtitle       │  │
│  │                                                            │  │
│  │  Login     [ ann            ]                             │  │
│  │  Password  [ ••••••••       ]                             │  │
│  │  Invalid login or password         ← error line (retry)   │  │
│  │                                                            │  │
│  │                    [Cancel]  [Sign IN]                    │  │
│  └────────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────────┘
```

## API Surface

### Control Creation

```lua
k.form.new("loginform", { title = "Sign in", layout = "vertical", align = "center" })

k.ctrl.login("loginform", "login", {
    -- Verification source (mirrors grid's db/query, but a single table)
    db             = "main",   -- named handle (--db main=…) or k.connect_sqlite/k.connect_db id. REQUIRED
    table          = "users",  -- users table
    login_column   = "login",  -- column matched against the login input
    password_column = "password",
    salt_column    = "salt",   -- required for hash="pbkdf2" unless salt= is given

    -- Password hashing
    hash       = "pbkdf2",     -- "pbkdf2" | "sha256" | "plain"
    salt       = nil,          -- static salt; overrides salt_column
    iterations = 100000,       -- pbkdf2 rounds (k.checksum defaults to 10000; stricter here)
    keylen     = 32,           -- pbkdf2 key length in bytes

    -- Presentation
    title      = "Sign in",
    subtitle   = "Enter your credentials",
    login_label = "Login",
    password_label = "Password",
    login_placeholder = nil,
    password_placeholder = nil,
    submit_label = "Sign IN",
    cancel_label = "Cancel",

    -- Retry policy
    error_message = "Invalid login or password",
    attempts      = 3,         -- wrong-password retries before the control stops retrying
})
```

### Events (via `k.form.on(form, ctrl, event, fn)`)

| Event | Signature | Fires when |
|-------|-----------|------------|
| `on_login_submit` | `on_login_submit(login, password)` | "Sign IN" clicked or Enter pressed. Runs **before** verification; the control always verifies afterwards regardless of the handler's return value. |
| `on_login` | `on_login(user)` | Credentials verified. `user` is the matched `users` row as a Lua map (`{id=1, login="ann", …}`), built by the same `toLuaValue` conversion `k.grid.get_row` uses. |
| `on_login_error` | `on_login_error(message, attempts_left)` | Wrong login/password. `attempts_left` reaches `0` once the retry budget is exhausted. |
| `on_login_cancel` | `on_login_cancel()` | "Cancel" clicked. No record is delivered. |

Registering only `on_login` is the common case:

```lua
k.form.on("loginform", "login", "on_login", function(user)
    k.print("welcome " .. user.login)
    k.msgbox("Hello " .. user.name)
end)
k.form.on("loginform", "login", "on_login_cancel", function()
    k.quit()
end)
```

### Options Reference

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `db` | string | — | **Required.** Named DB handle (`--db main=sqlite://…`) or a runtime id from `k.connect_sqlite` / `k.connect_db`. |
| `table` | string | `"users"` | Users table. Validated as a SQL identifier. |
| `login_column` | string | `"login"` | Column compared to the login input. Validated. |
| `password_column` | string | `"password"` | Column holding the stored hash/secret. Validated. |
| `salt_column` | string | `""` | Per-user salt column (`hash="pbkdf2"`). Validated. |
| `hash` | string | `"pbkdf2"` | `pbkdf2` \| `sha256` \| `plain`. |
| `salt` | string | `""` | Static salt; overrides `salt_column`. |
| `iterations` | number | `100000` | pbkdf2 rounds. |
| `keylen` | number | `32` | pbkdf2 derived key length. |
| `title` | string | `""` | Panel heading. Independent of the form's own `title`. |
| `subtitle` | string | `""` | Secondary line under the heading. |
| `login_label` | string | `"Login"` | Label above the login input. |
| `password_label` | string | `"Password"` | Label above the password input. |
| `login_placeholder` | string | `""` | Placeholder for the login input. |
| `password_placeholder` | string | `""` | Placeholder for the password input. |
| `submit_label` | string | `"Sign IN"` | Primary button text. |
| `cancel_label` | string | `"Cancel"` | Secondary button text. |
| `error_message` | string | `"Invalid login or password"` | Inline error shown on a failed attempt. |
| `attempts` | number | `3` | Wrong-password retries before the control stops retrying. |
| `label`, `enabled`, `visible`, `cell`, `align` | — | — | Standard control options from section 6 — handled by `renderVisibility` and the grid-cell layout for free. |

### Reading the verified user

`k.ctrl.get_value(form, "login")` returns the last verified user map (set on success, `nil` otherwise), mirroring the image control's `set_value`→`src` mapping from section 4.3. `k.ctrl.set_value` on a login control is ignored.

## Architecture

### 1. Go Runtime — `internal/bindings/login.go` (new file)

The control is **registered** alongside `k.ctrl.grid` in `registerForms` (`forms.go:443`):

```go
e.register("ctrl.login", "controls", func(L *lua.LState) int {
    formName := L.CheckString(1)
    name     := L.CheckString(2)
    opts     := L.OptTable(3, L.NewTable())
    addControl(L, formName, name, "login", opts)
    return 0
})
```

`addControl` (`forms.go:966`) stores the opts verbatim — the login control keeps its DB/crypto config on the control table, so no extra Go-side registry is needed.

The file hosts the verification logic, which must live in `bindings` rather than `session` because it needs the package-private `isValidIdentifier` (`db.go:671`) and `(*DBHandle).Placeholder` (`db.go:877`) — the same reason `session.go` calls `bindings.GridTableFromQuery`:

| Function | Signature | Responsibility |
|----------|-----------|----------------|
| `LoginOptsFromTable` | `(L *lua.LState, ctrl *lua.LTable) (LoginOptions, error)` | Apply defaults; validate `table`/`login_column`/`password_column`/`salt_column` through `isValidIdentifier`; require `db`. |
| `LookupLoginUser` | `(L *lua.LState, ctrl *lua.LTable, login string) (map[string]interface{}, error)` | `getDBHandle(L, opts.DB)` (`db.go:686`) → `SELECT * FROM <table> WHERE <login_column> = <Placeholder(1)>` → `h.Query(...)` (`db.go:921`); first row or `nil`. |
| `VerifyLoginPassword` | `(L *lua.LState, ctrl *lua.LTable, row map[string]interface{}, password string) bool` | Dispatch on `hash`; `crypto/subtle.ConstantTimeCompare` in every mode. |
| `loginUserFromRow` | `(L *lua.LState, row map[string]interface{}) *lua.LTable` | `common.GoValueToLua` conversion of the row into a Lua map. |

### 2. Rendering — `internal/bindings/forms.go`

`renderControl` (`forms.go:1613`) gains `case "login"` delegating to `renderLoginPanel(ctrl)`:

```html
<div class="kalua-control kalua-login" id="c:loginform:login" data-k-login="1"
     data-k-form="loginform" data-k-ctrl="login">
  <div class="kalua-login-title">Sign in</div>
  <div class="kalua-login-subtitle">Enter your credentials</div>
  <label class="kalua-label" for="c:loginform:login:login">Login</label>
  <input class="kalua-input kalua-login-input" type="text" id="c:loginform:login:login"
         name="login" data-k-form="loginform" data-k-ctrl="login" value="">
  <label class="kalua-label" for="c:loginform:login:password">Password</label>
  <input class="kalua-input kalua-login-input" type="password" id="c:loginform:login:password"
         name="password" data-k-form="loginform" data-k-ctrl="password" value="">
  <div class="kalua-login-error">Invalid login or password</div>   <!-- only when ctrl.error -->
  <div class="kalua-login-actions">
    <button type="button" class="kalua-btn" data-k-login-cancel="1"
            data-k-form="loginform" data-k-ctrl="login">Cancel</button>
    <button type="button" class="kalua-btn kalua-login-submit" data-k-login-submit="1"
            data-k-form="loginform" data-k-ctrl="login">Sign IN</button>
  </div>
</div>
```

The two action buttons are modelled on `gridModalFooter` (`session.go:1333`): they carry `data-k-form`/`data-k-ctrl` for identification but use `data-k-login-*` hooks, so the generic button branch at the end of `handleClick` (`app.js:1864`) never sees them and no spurious `click` event is fired. The two inputs *do* carry `data-k-form`/`data-k-ctrl` so the existing `gridFormValues(overlay)` collector (`app.js:842`) picks them up.

Server-side state lives on the control table, so nothing extra is needed in the session:
- `ctrl.attempt` — failed attempts so far (reset on success and on exhaustion)
- `ctrl.error` — current inline error text
- `ctrl.login_value` / `ctrl.password_value` — re-render values (login preserved, password always cleared on retry)
- `ctrl.user` — last verified row

**New: `k.ctrl.textbox { password = true }`.** No password input type exists today — `renderControl`'s default textbox branch hardcodes `type="text"` (`forms.go:1678`). Add the option so both the login control and ordinary forms can use it:

```go
if ctrl.RawGetString("password").String() == "true" {
    return `<div class="kalua-control"…><label …>` + label + `</label>` +
        `<input type="password" class="kalua-input" id="…" name="…" value="…"` + attrs + enabled + `>
        </div>`
}
```

`getControlValue` (`app.js:1950`) already returns `el.value` for `type="password"`, so no client change is needed to read it.

### 3. Client-Side — `internal/web/assets/app.js`

No new outbox message type. Two branches in `handleClick`, placed **before** the generic `[data-k-form][data-k-ctrl]` branch (siblings of the grid-footer branches at `app.js:1827-1862`):

```js
const loginSubmit = e.target.closest('[data-k-login-submit]');
if (loginSubmit) {
    e.preventDefault();
    const overlay = loginSubmit.closest('.form-modal-overlay');
    const v = overlay ? gridFormValues(overlay) : {};
    send({ type: 'event', form: loginSubmit.dataset.kForm,
           ctrl: loginSubmit.dataset.kCtrl, event: 'on_login_submit',
           value: { login: v.login || '', password: v.password || '' } });
    return;
}

const loginCancel = e.target.closest('[data-k-login-cancel]');
if (loginCancel) {
    e.preventDefault();
    send({ type: 'event', form: loginCancel.dataset.kForm,
           ctrl: loginCancel.dataset.kCtrl, event: 'on_login_cancel', value: null });
    return;   // the script decides what to do with the modal
}
```

Plus `setupLoginModal()` attached from the `render_form` branch of `handleMessage` (`app.js:1047`): focus the login input, and bind a `keydown` listener on the overlay so **Enter** in either input submits (mirrors the focus trap already installed by `renderModalForm`).

### 4. Session Event Handling — `internal/session/session.go`

`handleWSEvent` (`session.go:463`) grows a `loginDispatch` case, structurally identical to the existing `looperDispatch` / `chartDispatch` handling:

```go
loginDispatch := false
if msg.event == "on_login_submit" && s.isLoginControl(msg.form, msg.ctrl) {
    if vt, ok := value.(*lua.LTable); ok {
        loginDispatch = vt.RawGetString("password") != lua.LNil
    }
}
```

Two consequences, both required:

1. `loginDispatch` joins `looperDispatch`/`chartDispatch` in the guard at `session.go:495`, so **`updateControlValue` is skipped** — the plaintext password is never written into the Lua form definition.
2. `resumeArgs` unpacks to `[]lua.LValue{login, password}` so the script handler receives the two scalars.

After `on_login_submit` returns, the session runs the same flow `fireGridEvent` uses (`session.go:1271`):

| Outcome | Action |
|---------|--------|
| Row found **and** password verified | `ctrl.user = row`; clear `ctrl.error`/`ctrl.attempt`; `runFormHandler(form, ctrl, "on_login", [rowTable])` |
| Not found / mismatch, `attempt < attempts` | `ctrl.attempt++`; `ctrl.error = error_message`; `ctrl.login_value = login`; `ctrl.password_value = ""`; push the standard `update_control` outbox (`forms.go:521`) to re-render just the control; `runFormHandler(..., "on_login_error", [msg, left])` |
| Not found / mismatch, budget exhausted | same, but `ctrl.attempt = 0` (fresh budget for the next dialog) and `attempts_left = 0` |
| `on_login_cancel` | `runFormHandler(form, ctrl, "on_login_cancel", [])` |

`updateControl` (`app.js:1483`) replaces the element with `outerHTML`, which is why the submitted values are echoed through `ctrl.login_value` / `ctrl.password_value` before re-rendering.

`isLoginControl` is a copy of `isChartControl` (`session.go:3222`) testing `type == "login"`.

## Password Verification

| `hash` | Computation | Stored value |
|--------|-------------|--------------|
| `pbkdf2` (default) | `hex(pbkdf2.Key([]byte(pw), []byte(salt), iterations, keylen, sha256.New))` | hex digest in `password_column` |
| `sha256` | `hex(sha256(pw))`, or `hex(sha256(pw+salt))` when a salt is present | hex digest in `password_column` |
| `plain` | raw string | raw string in `password_column` |

- `golang.org/x/crypto/pbkdf2` is already imported by `crypto.go:27`; `golang.org/x/crypto v0.57.0` is in `go.mod:11`. **No new dependency.**
- A non-empty salt is **mandatory** for `pbkdf2` — the same rule `k.checksum` enforces (`crypto.go:63`). A missing/empty salt is a configuration error, surfaced via `on_login_error`, never silently downgraded to `plain`.
- Every mode compares with `crypto/subtle.ConstantTimeCompare` so a wrong password leaks no timing information.
- `iterations` defaults to 100 000 here, deliberately stricter than `k.checksum`'s 10 000 default.

Hash creation is the script's job (there is no `k.hash_password`); the demo derives it with `k.checksum`:

```lua
k.sql(db, "INSERT INTO users (login, password, salt) VALUES (?, ?, ?)",
      "ann", k.checksum("pbkdf2", "secret", "pepper", 100000, 32), "pepper")
```

## WebSocket Message Types

**No new types.** The control reuses `event` (submit/cancel) and the existing `render_form` / `update_control` outbox messages. `common.OutboxMsg` and `common.SessionInterface` are untouched.

## Key Behavior Decisions

| Decision | Rationale |
|----------|-----------|
| **Control, not a blocking `k.form.login(opts)` call** | Composes with the form system, needs no coroutine suspension, and delivers its result through the same event path as every other control |
| **Inline panel, shown via `k.form.show(..., {modal = true})`** | Reuses `renderModalForm` verbatim — zero new outbox types, one modal code path |
| **Result via `k.form.on` events, not a return value** | Consistent with `k.ctrl.grid` (`on_row_view(row)`) and `k.ctrl.chart` (`chart_click(...)`) |
| **Row map, not a filtered map** | `k.grid.get_row` already returns the whole row; hiding columns would surprise callers. Mask the secret in the handler if it matters |
| **`pbkdf2` default, configurable hash** | No KDF policy baked in; reuses the existing `k.checksum` primitive; no new dependency |
| **Retry inside the dialog, surfaced as `on_login_error(msg, left)`** | A failed attempt re-renders in place, keeping the login value and clearing the password, instead of returning to the script and re-showing the form |
| **Plaintext password skipped in `updateControlValue`** | `loginDispatch` joins the existing guard so the secret never lands in the Lua form definition |
| **Identifier validation on all four column/table names** | Same `isValidIdentifier` rule `buildWhereClause` (`db.go:647`) already applies; prevents SQL injection through option strings |
| **Verification always runs, even without `on_login_submit`** | The submit event is an observation hook, not a gate; a script cannot bypass authentication by omitting it |
| **Attacks:** no lockout, no rate limiting | Out of scope. `attempts` only bounds retries within one dialog; a script that re-shows the form gets a fresh budget |

## Implementation Phases

### Phase 1: `k.ctrl.textbox { password = true }` (0.5 day)
`renderControl` textbox branch + `api_doc.go` + `forms_test.go` render assertion. Self-contained; unblocks Phase 3.

### Phase 2: Verification core (2 days)
New `internal/bindings/login.go`: `LoginOptsFromTable`, `LookupLoginUser`, `VerifyLoginPassword`. Fully testable headlessly against a real SQLite `users` table in `t.TempDir()` — this is the security-critical part and needs no UI.

### Phase 3: Rendering (1 day)
`case "login"` in `renderControl`, `renderLoginPanel`, CSS, `make sync-assets` (the builder preview stylesheet is a copy guarded by `make check-assets`), render test.

### Phase 4: Registration & plumbing (0.5 day)
`e.register("ctrl.login", …)`, `registerKnown` entry (`bindings.go:110`) — without it `KALUA check` reports `unknown k.ctrl.login` (`checker.go:337`) — `api_doc.go` entry (required by `TestApiDocSync`, `api_doc_test.go:9`), `serve.go:317` `ctrlFuncs`, `internal/ai/knowledge.go` `runModeBindings`.

### Phase 5: Client (1 day)
`handleClick` submit/cancel branches, `setupLoginModal` focus + Enter-to-submit, `node --check`.

### Phase 6: Session (1.5 days)
`loginDispatch` + `resumeArgs` unpack, `isLoginControl`, submit/cancel handlers, retry re-render via `update_control`, `get_value` for the verified user.

### Phase 7: Tests, docs, demo (1.5 days)
E2E session test, `make gen-api`, `USER_GUIDE.md`, `kalua_spec.md`, `AGENTS.md`, `testdata/apps/login_demo.lua`.

**Total: ~8 days**

## File Changes

| File | Change |
|------|--------|
| `internal/bindings/login.go` | **New** — options parsing, `LookupLoginUser`, `VerifyLoginPassword`, row→Lua conversion |
| `internal/bindings/forms.go` | `ctrl.login` registration; `case "login"` in `renderControl`; `renderLoginPanel`; textbox `password` option |
| `internal/session/session.go` | `loginDispatch` + `resumeArgs` in `handleWSEvent`; `isLoginControl`; submit/cancel handling; retry re-render |
| `internal/web/assets/app.js` | `handleClick` login branches; `setupLoginModal`; Enter-to-submit |
| `internal/web/assets/kalua.css` | `.kalua-login*` rules |
| `internal/bindings/bindings.go` | `"ctrl.login": "controls"` in `registerKnown` |
| `internal/bindings/api_doc.go` | `ctrl.login` `Info`; extend `ctrl.textbox` doc with `password` |
| `internal/bindings/serve.go` | add `"login"` to `ctrlFuncs` so serve mode raises instead of silently nil-ing |
| `internal/ai/knowledge.go` | `"ctrl.login": true` in `runModeBindings` (otherwise invisible to the AI builder) |
| `_opencode/skills/kalua-api/api.md`, `docs/agentic/quickref.md` | regenerated by `make gen-api` |
| `docs/USER_GUIDE.md`, `kalua_spec.md`, `AGENTS.md` | prose updates |
| `testdata/apps/login_demo.lua` | **New** — seeds a `users` table, shows the form modally, greets the user |

## CSS Additions (kalua.css)

```css
/* Login control (k.ctrl.login) */
.kalua-login { display: flex; flex-direction: column; gap: 6px; max-width: 360px; }
.kalua-login-title { font-size: 18px; font-weight: 600; color: #212121; }
.kalua-login-subtitle { font-size: 13px; color: #757575; margin-bottom: 8px; }
.kalua-login-input { width: 100%; box-sizing: border-box; }
.kalua-login-error { display: none; font-size: 13px; color: #c62828; min-height: 18px; }
.kalua-login-error:not(:empty) { display: block; }
.kalua-login-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 10px; }
.kalua-login-submit { background: #1976d2; color: #fff; border-color: #1976d2; }
```

## Dependencies

None. `golang.org/x/crypto/pbkdf2` is already a dependency (`go.mod:11`, used by `crypto.go:27`).

## Tests

**`internal/bindings/login_test.go`** (new)
- `TestLoginPanelRender` — title, subtitle, `<input type="password"`, both `data-k-login-*` buttons; labels/placeholders escaped
- `TestVerifyLoginUser` — real SQLite `users` table: correct pbkdf2 password returns the row map with the expected columns; wrong password fails; unknown login fails; missing salt with `hash="pbkdf2"` errors; `sha256` and `plain` modes
- `TestLoginOptsValidation` — non-identifier characters in `table`/`login_column`/`password_column`/`salt_column` rejected; missing `db` raises
- `TestTextboxPassword` — `password=true` → `type="password"`, absent → `type="text"`

**`internal/session/login_e2e_test.go`** (new, modelled on `grid_e2e_test.go`)
- Real session + real SQLite `users` table; drain `s.Outbox()`; assert the `render_form` message carries `Modal:true` and the login HTML
- `PostEvent(form, ctrl, "on_login_submit", …)` with a wrong password → no `on_login`, an `update_control` carrying the error text arrived, `ctrl.attempt == 1`
- Correct password → the `on_login` handler sets a global equal to the row map
- `on_login_cancel` → handler fires, no record
- Regression guard: the password is never written into the form definition

## Resolved Design Questions

| Question | Decision |
|----------|----------|
| **Control vs. blocking form call** | Control — `k.ctrl.login` (see Overview) |
| **Modal presentation** | Inline control; the script calls `k.form.show(name, {modal = true})` |
| **Result channel** | `k.form.on` events; `k.ctrl.get_value` also returns the verified user |
| **Password input markup** | New `k.ctrl.textbox { password = true }` (reusable, not login-private) |
| **Hash algorithm** | `pbkdf2` default; `sha256`/`plain` configurable |
| **Wrong credentials** | Retry in-dialog up to `attempts`, then `on_login_error(msg, 0)` |
| **Cancel** | `on_login_cancel()`, no record |
| **Exhausted retries vs. cancel** | Distinguishable only to a handler registering both `on_login_error` and `on_login_cancel` |
| **Row contents** | Whole row returned; masking is the handler's job |
| **User storage / sessions** | Out of scope — the control verifies, the app stores `user` in a global |
| **Brute-force protection** | Out of scope — no lockout, no rate limiting |

## Open Decisions

- Should retry exhaustion raise a Lua error (`KErrorUserCanceled`, `bindings.go:541`) instead of only firing `on_login_error(msg, 0)`? Current design stays non-throwing, consistent with `on_save` returning `{ok, error}`.
- Should `k.ctrl.get_value(form, "login")` exist at all, or should the record be reachable only through the event? Cheap to add, cheap to drop.
- Should the control expose `on_login_locked` for an in-DB `locked_until` column, so lockout policy can live in the schema?

## Example App

`testdata/apps/login_demo.lua` — seeds a `users` table in SQLite, declares the control, shows the form modally, and greets the verified user. Doubles as the runnable counterpart to the `k.form.new("login", …)` layout example in section 6.

## Implementation Status

Not started — plan only.

---

# 9. Tree Control (`k.ctrl.tree`)

## Overview

Introduce a new `k.ctrl.tree(form, name, opts)` control that displays a **specified field from a database table as an expandable tree**. The hierarchy is an adjacency list: each row has a unique-ID column and a parent-reference column (`parent_id`). The control is a peer of `k.ctrl.looper` / `k.ctrl.grid` — DB-linked config stored on the control, results delivered through `k.form.on` events.

```
categories                                (table rows)
┌──────────┬──────────────┐
│ id       │ parent_id    │
├──────────┼──────────────┤
│ 1        │ NULL         │  → Electronics
│ 2        │ 1            │     ├─ Laptops
│ 3        │ 1            │     └─ Phones
│ 4        │ 2            │        └─ ThinkPad
│ 5        │ NULL         │  → Books
└──────────┴──────────────┘

Rendered:  ▸ Electronics     (click ▸ expands)
             ▸ Laptops
               └─ ThinkPad
             ▸ Phones
           ▸ Books
```

### Design decisions (locked)

| Decision | Rationale |
|----------|-----------|
| **Full load — all nodes upfront** | Server fetches all matching rows in one query; hierarchy built server-side; client expands/collapses locally. No per-node round-trips; good to ~5k nodes |
| **Flat rows `{id, parent_id, label, …extra}`** | `label_field` picks the displayed column; the script receives the full row map in events |
| **`on_click` / `on_expand` / `on_select` events** | Mirrors `k.ctrl.looper`'s `onselect`/`onclick`; node objects delivered as Lua maps |
| **Full builder support** | Palette entry, property editor, import/export, preview — same effort as any new control type |

## API Surface

### Control Creation

```lua
k.form.new("main", { title = "Categories" })

k.ctrl.tree("main", "tree", {
    -- Data source (DB-linked, like looper/table)
    db      = "main",                 -- named handle from --db, or k.connect_sqlite/k.connect_db id. REQUIRED
    table   = "categories",           -- source table. REQUIRED
    where   = "",                     -- extra WHERE clause (identifier/params guarded)
    order_by = "name ASC",            -- deterministic sibling order (recommended)

    -- Hierarchy definition (adjacency list)
    id_field     = "id",              -- DEFAULT "id"
    parent_field = "parent_id",       -- DEFAULT "parent_id"
    label_field  = "name",            -- REQUIRED: column rendered as the node label

    -- Presentation
    expand_all = false,               -- DEFAULT false: render roots collapsed
    max_nodes  = 5000,                -- DEFAULT 5000: safety cap on full-load
})
```

### Events (via `k.form.on(form, ctrl, event, fn)`)

| Event | Signature | Description |
|-------|-----------|-------------|
| `on_click` | `on_click(node)` | Node clicked. `node` = full row map (`{id=…, parent_id=…, [label_field]=…, …extra columns}`) |
| `on_select` | `on_select(node)` | Node selected (click while `selection=...` active). Same payload as `on_click` |
| `on_expand` | `on_expand(node_id, expanded)` | A node was expanded (`true`) or collapsed (`false`) by the user |
| `on_loaded` | `on_loaded(node_count)` | Full tree fetched and rendered on the client. `node_count` = total nodes (0 on error) |
| `on_error` | `on_error(message)` | Fetch/SQL/max_nodes failure. Control renders an empty state; never crashes the session |

```lua
k.form.on("main", "tree", "on_click", function(node)
    k.print("clicked " .. node.name .. " (id=" .. tostr(node.id) .. ")")
end)
k.form.on("main", "tree", "on_select", function(node)
    k.ctrl.set_value("main", "detail", node.name)   -- feed a sibling control
end)
```

### Reading values

- `k.ctrl.get_value(form, "tree")` returns the **currently selected node map** (or `nil`). Mirror of the login control's verified-user read-back (section 8) and the image control's `src` mapping (section 4.3).
- `k.ctrl.set_value(form, "tree", node)` is ignored (nodes come from the DB).

## Architecture

### 1. Go Runtime — `internal/bindings/tree.go` (new file, `!wasm`)

Registered in `registerForms` alongside `k.ctrl.grid` (`forms.go:443`):

```go
e.register("ctrl.tree", "controls", func(L *lua.LState) int {
    formName := L.CheckString(1)
    name     := L.CheckString(2)
    opts     := L.OptTable(3, L.NewTable())
    addControl(L, formName, name, "tree", opts)
    return 0
})
```

| Function | Signature | Responsibility |
|----------|-----------|----------------|
| `TreeLinkFromControl` | `(L *lua.LState, ctrl *lua.LTable) (*TreeLink, bool)` | Read `db`/`table`/`where`/`order_by`/`id_field`/`parent_field`/`label_field`/`max_nodes`. Requires `db`+`table`+`label_field`. Validates identifiers via `isValidIdentifier` (`db.go:671`) like `buildWhereClause` |
| `FetchTreeRows` | `(L *lua.LState, link *TreeLink) (*TreePage, error)` | `getDBHandle` (`db.go:686`) → `SELECT * FROM <table>` + optional `WHERE` + `ORDER BY` → `h.Query` (`db.go:921`). Returns columns + all rows |
| `BuildTree` | `(L *lua.LState, link *TreeLink, rows []map[string]interface{}) (*lua.LTable, int, error)` | Build the adjacency tree in Go: map `parent_id` → children (strings as keys for mixed id types); roots = parents missing/`NULL`/not in the set; attach full row data to each node as `{id, parent_id, label, _children=[...], ...extra}`. Returns node count |

**Safety:**
- `max_nodes` (default 5000): `FetchTreeRows` fails fast (no unbounded memory).
- Orphan children (parent reference that is not a loaded row) are surfaced as **root-level** nodes so no row is lost.
- `WHERE` is passed through verbatim (same contract as looper/table `where`); `id_field`/`parent_field`/`label_field`/`table` are identifier-guarded.

### 2. Rendering — `internal/bindings/forms.go`

`renderControl` (`forms.go:1613`) gains `case "tree"` → `renderTree(ctrl, formName, name)`:

```html
<div class="kalua-control kalua-tree-control">
  <div class="kalua-tree" id="c:main:tree"
       data-k-form="main" data-k-ctrl="tree"
       data-k-tree-expand-all="false"
       data-k-tree-selection="single"
       data-k-tree-error="">
    <div class="kalua-tree-empty">Loading…</div>
  </div>
</div>
```

The container carries the config the client needs to issue the single `tree_data_request`; the Go side does **not** render nodes (the client renders from the nested JSON payload for cheap local expand/collapse).

The control participates in the switch at `forms.go:1747` (`case "grid"` region) and gets visibility/enabled/layout (cell/align §6) for free via `renderVisibility`/`renderAttrs`.

### 3. Client — `internal/web/assets/app.js`

- **`initTrees(scope)`** — like `initLoopers` (`app.js:1272`): scan `.kalua-tree:not([data-k-tree-ready])`, mark ready, send `{type:"tree_data_request", form, ctrl}` once.
- **`handleTreeData(msg)`** — the `tree_data` outbox carries nested JSON; client renders recursively:
  ```js
  function renderTreeNodes(list, depth) {
      return list.map(n => `<li class="kalua-tree-node" data-k-node-id="${n.id}"
              data-k-node='${escapeAttr(JSON.stringify(n))}' style="padding-left:${depth*14}px">
          ${n._children ? `<button class="kalua-tree-toggle" data-k-tree-toggle="1">▸</button>` : ''}
          <span class="kalua-tree-label" data-k-tree-label="1">${escapeHtml(n.label)}</span></li>`).join('');
  }
  ```
  The node's full row map is embedded as `data-k-node` JSON → the click event echoes the whole record back to the host so `on_click` receives the row map.
- **Toggle** — `data-k-tree-toggle` toggles the nested `<ul>` (`display:none/none`), rotates ▸/▾, and sends `{type:"event", form, ctrl, event:"on_expand", value:{node_id, expanded}}`.
- **Node click** — `data-k-tree-label` sends `{type:"event", form, ctrl, event:"on_click", value:{node:<parsed data-k-node>}}`; with selection enabled also `on_select` with the same payload.

### 4. Session Event Handling — `internal/session/session.go`

| Message | Handler |
|---------|---------|
| `tree_data_request` (inbox) | `handleTreeDataRequest` — resolve control → `TreeLinkFromControl` → `FetchTreeRows` → `BuildTree` → send `tree_data` outbox; errors → `on_error` |
| `tree_data` (outbox) | `{Type:"tree_data", Form, Ctrl, Selector:"#c:form:ctrl", Data:<nested JSON>}` — mirrors `looper_db_batch` |
| `on_click`/`on_select` | In `handleWSEvent` (`session.go:463`), mirror the chart/looper dispatch: unpack the `value` table's `node` into `[]lua.LValue{nodeTable}`; skip `updateControlValue` (a node map is not a control value) |
| `on_expand` | unpack `{node_id, expanded}` → `[]lua.LValue{nodeID, expandedBool}` |

A `treeDispatch` guard (like `looperDispatch`/`chartDispatch`, `session.go:476-497`) prevents the node table from being written into the form definition.

### 5. WebSocket Message Types

| Type | Direction | Payload |
|------|-----------|---------|
| `tree_data_request` | browser → host | `{type, form, ctrl}` |
| `tree_data` | host → browser | `Data` = `{nodes:[{id,parent_id,label,_children:[...],…extra}], count:n}` |
| `event` (`on_click`/`on_select`/`on_expand`) | browser → host | existing event mechanism |

No new `OutboxMsg`/`InboxMsg` fields needed — `Data`/`Value` suffice (same as looper).

## Key Behavior Decisions

| Decision | Rationale |
|----------|-----------|
| **Full load, not lazy** | ≤ ~5k nodes; one query; client-side expand/collapse with zero latency. Lazy per-child fetch is future work (`page_size` in the link) |
| **Server builds tree, client renders** | Go produces the adjacency JSON once; JS renders it to HTML — no server round-trip on expand |
| **Whole row map per node** | `on_click`/`on_select` receive the full record (like `k.grid.get_row`), so the app can surface extra columns without a second lookup |
| **`label_field` required** | The whole point of the control; missing → `on_error` |
| **Identifier-guarded fields** | `id_field`/`parent_field`/`label_field`/`table` pass `isValidIdentifier` — same SQL-injection posture as `buildWhereClause` |
| **Orphans as roots** | No row is dropped when its parent isn't in the fetch window |
| **`max_nodes` cap** | Fast-fail on pathological tables instead of unbounded memory |

## Implementation Phases

| # | Phase | Est. |
|---|-------|------|
| 1 | `internal/bindings/tree.go`: `TreeLinkFromControl` + `FetchTreeRows` + `BuildTree` | 2 days |
| 2 | `renderTree` in `forms.go` + support `tree` in the serve/registry/api-doc surfaces | 1 day |
| 3 | Builder palette + property editor + export/import + preview | 1.5 days |
| 4 | Client (`app.js`): `initTrees`/`handleTreeData`/toggle/click dispatch | 1.5 days |
| 5 | Session handlers (`tree_data_request`, `treeDispatch`, events) | 1 day |
| 6 | Tests (bindings unit + session e2e) | 1 day |
| 7 | Docs (`api_doc.go`, USER_GUIDE, spec, AGENTS.md) + demo app + `make gen-api` | 1 day |
| **Total** | | **~8 days** |

## File Changes

| File | Change |
|------|--------|
| `internal/bindings/tree.go` | **New** — link parsing, fetch, tree build |
| `internal/bindings/forms.go` | `ctrl.tree` registration; `case "tree"` in `renderControl`; `renderTree` |
| `internal/bindings/bindings.go` | `"ctrl.tree": "controls"` in `registerKnown` (required by checker `checker.go:337`) |
| `internal/bindings/api_doc.go` | `ctrl.tree` Info (Group `controls`) — required by `TestApiDocSync` |
| `internal/bindings/serve.go:317` | add `"tree"` to `ctrlFuncs` so serve mode raises instead of nil-ing |
| `internal/ai/knowledge.go` | `"ctrl.tree": true` in `runModeBindings` |
| `internal/session/session.go` | `tree_data_request` handler, `treeDispatch`, event unpacking |
| `internal/web/assets/app.js` | `initTrees`, `handleTreeData`, toggle/click handlers |
| `internal/web/assets/kalua.css` | `.kalua-tree`, `.kalua-tree-node`, `.kalua-tree-toggle`, `.kalua-tree-label`, `::marker` reset |
| `internal/builder/model.go` | add `"tree"` to `Types` |
| `internal/builder/assets/builder.js` | `PALETTE` + `TYPE_OPTS.tree` (db, table, where, order_by, id/parent/label field, expand_all, max_nodes) |
| `internal/builder/lua_export.go` | DB-handle export already handled generically; verify `table`-style handling |
| `_opencode/skills/kalua-api/api.md`, `docs/agentic/quickref.md` | regenerate via `make gen-api` |
| `docs/USER_GUIDE.md`, `kalua_spec.md`, `AGENTS.md` | prose updates |
| `testdata/apps/tree_demo.lua` | **New** — org-chart/menu-tree demo with expand/click/select handlers |

## CSS Additions (kalua.css)

```css
/* Tree control (k.ctrl.tree §9) */
.kalua-tree { display: block; max-height: 360px; overflow-y: auto;
              font-size: 14px; border: 1px solid #e0e0e0; border-radius: 4px; }
.kalua-tree ul { list-style: none; margin: 0; padding: 0 0 0 14px; }
.kalua-tree-node { line-height: 1.7; }
Run `make sync-assets` to mirror into the builder preview stylesheet (guarded by `make check-assets`).

## Dependencies

None. Reuses `getDBHandle`/`h.Query`/`isValidIdentifier` from `db.go`.

## Tests

**`internal/bindings/tree_test.go`** (new)
- `TestTreeLinkDefaults` — `id`/`parent_id` defaults; `db`+`table`+`label_field` required; non-identifier fields rejected
- `TestFetchTreeRows` — real SQLite `t.TempDir()`; `SELECT *` + `where` + `order_by` honored
- `TestBuildTree` — nesting, multi-root forests, orphan rows promoted to roots, `max_nodes` cap error
- `TestRenderTree` — render carries `data-k-tree-*` attrs and empty state

**`internal/session/tree_e2e_test.go`** (new, modelled on `looper_e2e_test.go`)
- Real SQLite categories table + session; drain `s.Outbox()`; `PostTreeDataRequest` → `tree_data` payload with nested children and count
- `PostEvent(form, ctrl, "on_click", {node=…})` → `on_click` handler global set to the row map
- `PostEvent(form, ctrl, "on_expand", {node_id=…, expanded=true})` → handler args correct
- Regression guard: node table never written into the form's control values (mirror `TestLooperRowSelection`)

## Implementation Status

Not started — plan only.

---

## 10. `kforms_enhancements.md` — §10 Tab Control

### Overview

Add a **Tab control** (`k.ctrl.tab`) to KALUA matching Kalipso's tab control behavior. The control organizes content into multiple tabs, showing only one tab's content at a time. Users switch tabs by clicking headers or swiping left/right on the content area.

### Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        Go Host (Session)                        │
│  ┌─────────────────┐    ┌──────────────────┐                  │
│  │ k.ctrl.tab      │    │ k.tab.set_tab    │                  │
│  │ (tabs array,    │───▶│ (programmatic    │                  │
│  │  position,      │    │  tab switching)  │                  │
│  │  swipable)      │    └────────┬─────────┘                  │
│  └────────┬────────┘             │                             │
│           │                      ▼                             │
│           ▼              ┌───────────────────────┐             │
│  ┌─────────────────────────────────────────┐                  │
│  │ renderControl: <div.kalua-tab> + data   │                  │
│  │ attributes (tabs, position, swipable)   │                  │
│  └─────────────────────────────────────────┘                  │
└────────────────────┼──────────────────────────────────────────┘
                     │ WebSocket update_control
                     ▼
┌─────────────────────────────────────────────────────────────────┐
│                     Browser (app.js)                            │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │ Tab initialization: click handlers, swipe detection       │   │
│  │ Instance Map keyed by selector (#c:form:ctrl)             │   │
│  │ On tab click: send 'tab_change' event to host             │   │
│  │ On swipe: detect delta, send 'tab_change' event           │   │
│  │ On 'tab_set_tab' from host: activate tab programmatically │   │
│  └──────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

### API Surface

#### New Constructor: `k.ctrl.tab(form, name, opts)`

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `tabs` | `table` | required | Array of tab definitions: `{title="Tab 1", icon="📊"}` |
| `tab_position` | `string` | `"top"` | `"top"` or `"bottom"` |
| `swipable` | `boolean` | `true` | Enable swipe navigation on content |
| `visible` | `boolean` | `true` | Control visibility |
| `enabled` | `boolean` | `true` | Control enabled state |
| `cell` | `string` | — | Grid cell assignment (§6) |
| `align` | `string` | — | Alignment override (§6) |

**Tab definition:**
```lua
{
  tabs = {
    {title = "Dashboard", icon = "📊"},
    {title = "Reports", icon = "📈"},
    {title = "Settings", icon = "⚙️"}
  },
  tab_position = "top",
  swipable = true,
  onchange = function(new_idx, old_idx)
    print("Switched from tab " .. old_idx .. " to " .. new_idx)
  end
}
```

#### Tab Operations (`k.tab.*`)

| Function | Signature | Description |
|----------|-----------|-------------|
| `k.tab.set_tab` | `(form, name, index)` | Programmatically switch to tab (1-based) |
| `k.tab.get_tab` | `(form, name)` → number | Returns current tab index (1-based) |
| `k.tab.get_tab_count` | `(form, name)` → number | Returns number of tabs |
| `k.tab.add_tab` | `(form, name, {title, icon?})` | Add tab at end |
| `k.tab.insert_tab` | `(form, name, index, {title, icon?})` | Insert tab at position |
| `k.tab.remove_tab` | `(form, name, index)` | Remove tab by index |
| `k.tab.set_tab_title` | `(form, name, index, title)` | Update tab title |
| `k.tab.set_tab_icon` | `(form, name, index, icon)` | Update tab icon |

#### Events (via `k.form.on(form, ctrl, event, fn)`)

| Event | Payload |
|-------|---------|
| `onchange` | `{new_index, old_index}` — fired when user switches tabs |

### Example Usage

```lua
function main()
    k.form.new("main", {title = "Tab Demo", layout = "vertical"})
    
    k.ctrl.tab("main", "tabs", {
        tabs = {
            {title = "Dashboard", icon = "📊"},
            {title = "Reports", icon = "📈"},
            {title = "Settings", icon = "⚙️"}
        },
        tab_position = "top",
        swipable = true,
        onchange = function(new_idx, old_idx)
            k.ctrl.set_value("main", "status", "Switched from tab " .. old_idx .. " to " .. new_idx)
        end
    })
    
    k.ctrl.textbox("main", "status", {label = "Status", value = "Tab 1 active"})
    k.ctrl.button("main", "btn_goto_3", {label = "Go to Settings", onclick = function()
        k.tab.set_tab("main", "tabs", 3)
    end})
    
    k.form.show("main")
end
```

### Key Behavior Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| **Tab content** | Each tab panel is a nested form-like container | Controls can be added to each tab's panel |
| **Swipe support** | Client-side only (touch events on panel container) | No server round-trip for swipe |
| **State sync** | `current_tab` stored in Lua control table | Client and server in sync via `tab_change` / `tab_set_tab` |
| **Accessibility** | Proper ARIA: `role="tablist"`, `role="tab"`, `role="tabpanel"` | WCAG compliance |
| **Icons** | Emoji or text string prepended to title | Simple, no icon font dependency |
| **Tab index** | 1-based (Kalipso convention) | Consistency with other controls |

### WebSocket Message Types

| Direction | Type | Payload |
|-----------|------|---------|
| Browser → Go | `tab_change` | `{form, ctrl, event, value: {index}}` |
| Go → Browser | `tab_set_tab` | `{selector, index}` |

### Implementation Phases

| Phase | Description | Days |
|-------|-------------|------|
| 1 | `forms.go`: Register `ctrl.tab`, parse options, store tabs array | 1 |
| 2 | `render.go`: Implement `renderTab()` with nested panel structure | 2 |
| 3 | `tab.go` (new): `k.tab.*` operations (set_tab, get_tab, add/remove, etc.) | 1 |
| 4 | `api_doc.go`: Document `ctrl.tab` and all `k.tab.*` ops | 0.5 |
| 5 | `app.js`: Client-side init, click/swipe handlers, `tab_set_tab` handling | 2 |
| 6 | `session.go`: Handle `tab_change` inbox, run `onchange` handler | 0.5 |
| 7 | `kalua.css`: Tab styling (headers, panels, active state, swipe hints) | 1 |
| 8 | Builder: Palette entry, property editor, tab management UI, preview | 2 |
| 9 | Tests: Unit (bindings) + e2e (session) | 1 |
| 10 | Demo app: `testdata/apps/tab_demo.lua` | 0.5 |
| **Total** | | **~11.5** |

### File Changes

| File | Changes |
|------|---------|
| `internal/bindings/forms.go` | Register `ctrl.tab` in `registerControls()`, store options |
| `internal/bindings/render.go` | Add `case "tab":` → `renderTab()`, nested panel rendering |
| `internal/bindings/tab.go` | **New** — `registerTabOps()`, all `k.tab.*` functions |
| `internal/bindings/api_doc.go` | Document `ctrl.tab` and `k.tab.*` |
| `internal/session/session.go` | Add `inboxTabChange`, `handleTabChange()`, form close cleanup |
| `internal/web/assets/app.js` | `tabInstances` Map, `initTabs()`, click/swipe handlers, `tabUpdate()` |
| `internal/web/assets/kalua.css` | Tab styles (`.kalua-tab`, `.kalua-tab-btn`, `.kalua-tab-panel`) |
| `internal/builder/server.go` | Add tab control to palette, preview, export |
| `internal/builder/assets/builder.js` | Tab editor modal (add/remove/reorder tabs) |
| `internal/checker/checker.go` | Validate tab options |
| `internal/lsp/server.go` | Completions for tab API |

### CSS Additions (kalua.css)

```css
/* Tab control */
.kalua-tab { display: flex; flex-direction: column; }
.kalua-tab[data-k-tab-position="bottom"] { flex-direction: column-reverse; }
.kalua-tab-headers { display: flex; border-bottom: 1px solid #ddd; }
.kalua-tab[data-k-tab-position="bottom"] .kalua-tab-headers { 
  border-top: 1px solid #ddd; border-bottom: none; 
}
.kalua-tab-btn { 
  flex: 1; padding: 12px 16px; background: transparent; border: none; 
  cursor: pointer; white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  font: inherit; color: #666; border-bottom: 3px solid transparent; margin-bottom: -1px;
}
.kalua-tab-btn[aria-selected="true"] { 
  color: #007bff; border-bottom-color: #007bff; font-weight: 600; 
}
.kalua-tab-panels { flex: 1; overflow: hidden; position: relative; }
.kalua-tab-panel { display: none; height: 100%; overflow: auto; }
.kalua-tab-panel[aria-hidden="false"] { display: block; }
.kalua-tab-panel > .kalua-form { height: 100%; }
/* Swipe support */
.kalua-tab[data-k-swipable="true"] .kalua-tab-panels { touch-action: pan-y; }
```

### Dependencies

- No new external dependencies
- No new Go dependencies
- Pure Go/JS implementation

### Tests

**`internal/bindings/tab_test.go`** (new)
- `TestTabRender` — renders headers + panels, correct `aria-selected`, `tab_position` bottom
- `TestTabSetGet` — `set_tab`/`get_tab`/`get_tab_count` work correctly
- `TestTabAddRemoveInsert` — add/remove/insert tab updates tab count and panels
- `TestTabTitleIcon` — `set_tab_title`/`set_tab_icon` update header

**`internal/session/tab_e2e_test.go`** (new)
- Real session: create tab, switch tabs programmatically, verify `onchange` fires
- Test programmatic `k.tab.set_tab` updates `current_tab` and sends `tab_set_tab`
- Test swipe not applicable in headless; test `onchange` payload correctness

### Implementation Status

Not started — plan only.

---

## 11. `kforms_enhancements.md` — §11 Layout Controls: Topbar, Sidebar, Footer

### Overview

Three new layout controls provide a complete application shell:

- **Topbar** (`k.ctrl.topbar`) — Fixed header with app branding, user info, and logout
- **Sidebar** (`k.ctrl.sidebar`) — Collapsible navigation (grid cell or horizontal tabs)
- **Footer** (`k.ctrl.footer`) — Fixed bottom bar with version, copyright, links

All three are **layout controls** — they render outside the normal form control flow and
provide the application shell. The Topbar and Footer are fixed-position elements that
span the full viewport width. The Sidebar behaves differently depending on the form's
layout mode.

### 11.1 Topbar Control (`k.ctrl.topbar`)

#### Overview

The Topbar is a fixed-position header bar at the top of the viewport. It displays
application branding, the current user's information, and an optional logout action.

#### API Surface

```lua
k.ctrl.topbar(form, name, {
    icon = "assets/logo.png",     -- image path relative to assets folder
    title = "My App",             -- app name
    user = {                      -- optional user info
        name = "John Doe",
        role = "admin",
        avatar = "assets/avatar.png",
    },
    logout = function() k.quit() end,  -- direct logout call
    visible = true,
})
```

#### Options

| Option | Type | Description |
|--------|------|-------------|
| `icon` | `string` | Image path relative to assets folder (JPG/PNG/SVG) |
| `title` | `string` | App name |
| `user` | `table` | User info table with `name`, `role`, `avatar` (image path) |
| `logout` | `function` | Called when user clicks avatar/name — direct logout call |
| `visible` | `boolean` | Show/hide the topbar (default: `true`) |

#### Behavior

- **Fixed position** at top of viewport, outside form layout flow
- **User click** → invokes the `logout` function directly (no dropdown menu)
- **Assets** loaded from `assets/` folder beside the KALUA binary
- **Fixed height** (56px default), spans full viewport width

#### Events

| Event | Payload | Description |
|-------|---------|-------------|
| `logout` | `{}` | Fired when user clicks avatar/name; triggers `logout` function |

### 11.2 Sidebar Control (`k.ctrl.sidebar`)

#### Overview

The Sidebar is a navigation panel that adapts to the form's layout mode:

- **Grid layout**: Renders as a grid cell (assigned via `cell = "sidebar"`), collapsible to an icon-only strip
- **Vertical layout**: Renders as a horizontal tab bar below the topbar

#### API Surface

```lua
k.ctrl.sidebar(form, name, {
    position = "left",              -- "left" | "right"
    width = 280,                    -- expanded width in pixels
    collapsed_width = 64,           -- collapsed width (icon strip only)
    collapsible = true,             -- allow user to collapse/expand
    collapsed = false,              -- initial state (session only, resets on reload)
    items = {
        { label = "Dashboard", icon = "assets/home.png", action = "nav_dashboard" },
        { type = "separator" },
        { label = "Users", icon = "assets/users.png", action = "nav_users", badge = 5 },
        { label = "Settings", icon = "assets/settings.png", action = "nav_settings" },
    },
    on_select = function(item) end,  -- optional global handler
    visible = true,
    style = { bg = "#f8f9fa", border_right = "1px solid #e0e0e0" }
})
```

#### Options

| Option | Type | Description |
|--------|------|-------------|
| `position` | `string` | `"left"` or `"right"` (default: `"left"`) |
| `width` | `number` | Expanded width in pixels (default: `280`) |
| `collapsed_width` | `number` | Collapsed width in pixels (default: `64`) |
| `collapsible` | `boolean` | Allow user to collapse/expand (default: `true`) |
| `collapsed` | `boolean` | Initial collapsed state (default: `false`, session only) |
| `items` | `table[]` | Array of item definitions (see below) |
| `on_select` | `function` | Global selection handler (optional) |
| `visible` | `boolean` | Show/hide sidebar (default: `true`) |
| `style` | `table` | CSS style overrides |

#### Item Definition

Each item in the `items` array is a table with:

| Field | Type | Description |
|-------|------|-------------|
| `label` | `string` | Display text |
| `icon` | `string` | Image path relative to assets folder (JPG/PNG/SVG) |
| `action` | `string` | Action identifier sent on click |
| `badge` | `number\|string` | Optional badge text/number |
| `type` | `string` | `"separator"` for visual divider (no label/action needed) |

#### Behavior

- **Grid layout**: Renders as a grid cell (`cell = "sidebar"`), occupies assigned columns, collapsible to icon strip
- **Vertical layout**: Renders as horizontal tab bar below the topbar (full width)
- **Collapse state**: Session-only (resets on page reload), controlled via collapse button
- **Icons**: JPG/PNG/SVG images from `assets/` folder beside KALUA binary
- **Click behavior**: Fires `k.form.on` event with `sidebar_select` event and item data

#### Events

| Event | Payload | Description |
|-------|---------|-------------|
| `sidebar_select` | `{action: string, item: table, index: number}` | Fired when user clicks a sidebar item |

```lua
k.form.on("main", "side", "select", function(item)
    k.ctrl.set_value("main", "content", item.action .. "_content")
end)
```

### 11.3 Footer Control (`k.ctrl.footer`)

#### Overview

The Footer is a fixed-position bar at the bottom of the viewport displaying
version info, copyright, and optional links.

#### API Surface

```lua
k.ctrl.footer(form, name, {
    text = "© 2024 My Company",      -- copyright text
    version = "1.0.0",               -- app version (auto-filled from KALUA if omitted)
    script_name = "myapp.lua",       -- running script name
    links = {                        -- optional links
        { label = "Privacy", url = "/privacy" },
        { label = "Terms", url = "/terms" }
    },
    visible = true,
    style = { bg = "#f5f5f5", height = 40 }
})
```

#### Options

| Option | Type | Description |
|--------|------|-------------|
| `text` | `string` | Copyright/description text |
| `version` | `string` | App version (auto-filled from KALUA if omitted) |
| `script_name` | `string` | Running script name (auto-filled) |
| `links` | `table[]` | Array of `{label, url}` for footer links |
| `visible` | `boolean` | Show/hide footer (default: `true`) |
| `style` | `table` | CSS style overrides (`bg`, `height`, etc.) |

#### Behavior

- **Fixed position** at bottom of viewport
- **Auto-populated**: `version` from KALUA version, `script_name` from running script
- **Links** render as inline links
- **Fixed height** (40px default), spans full viewport width

### 11.4 Layout Integration

| Layout | Topbar | Sidebar | Footer |
|--------|--------|---------|--------|
| **Grid** | Fixed top (spans 12 cols) | Grid cell (`cell="sidebar"`), collapsible | Fixed bottom (12 cols) |
| **Vertical** | Fixed top | **Horizontal tab bar** (below topbar) | Fixed bottom |

#### Grid Layout

In grid layout, the sidebar is assigned to a cell via `cell = "sidebar"` in the
control options. The form's `cells` definition should include a `sidebar` cell:

```lua
k.form.new("main", {
    layout = "grid",
    cells = {
        sidebar  = {width = 3, bg = "#f8f9fa", border = {width=1, color="#e0e0e0"}},
        content  = {width = 9},
    }
})

k.ctrl.sidebar("main", "side", { cell = "sidebar", ... })
```

The sidebar cell width determines the expanded width; collapsed width is fixed at 64px.

### 11.5 Events Summary

| Control | Event | Payload |
|---------|-------|---------|
| Topbar | `logout` | `{}` — user clicked avatar/name |
| Sidebar | `sidebar_select` | `{action, item, index}` — user clicked an item |
| Footer | — | No events |

### 11.6 Implementation Notes

- **Renderer**: Shared renderer in `internal/bindings/render.go` (same as native)
- **Assets**: Icons/images loaded from `assets/` folder beside KALUA binary
- **Sidebar state**: Collapsed/expanded state is session-only (resets on reload)
- **Layout switching**: Grid ↔ Vertical transition preserves sidebar content, changes rendering mode
- **Assets**: Icons/images served from `assets/` folder beside KALUA binary (served via static file handler)

---

## 12. `kforms_enhancements.md` — §12 AI Chat Form (`k.form.ai_agent`)

### Overview

A **built-in, modal AI chat form** for `KALUA run`. Every function the script defines
becomes a tool the agent can call, so the model can act on the app's own data
(`k.db_select`, file reads, computed helpers) rather than only answering in prose.

`k.form.ai_agent(opts)` **blocks the script** (like `k.form.show` / `k.msgbox`) and
returns the full transcript when the user closes the chat.

```lua
function get_customer(id)
    -- Look up a customer by id        ← comment above becomes the tool description
    return k.db_select("select * from customers where id = ?", {id})
end

function main()
    local transcript = k.form.ai_agent{ title = "Support assistant" }
    for _, m in ipairs(transcript) do print(m.role, m.content) end
end
```

Three design constraints drive the architecture:

1. **The agent loop blocks on network I/O**, so it must not run on the session actor
   goroutine → it runs via `RunAsync`.
2. **Tools touch `lua.LState`**, which is actor-goroutine-only → tool calls are routed
   through the existing `Session.Query` primitive.
3. **A form's suspend resumes with `LNil`** (`ResumeFormCoro` hardcodes it) → the chat
   needs a new pending kind that resumes with a table.

#### Prior art (already shipped, reused rather than reinvented)

| Existing asset | Reuse |
|----------------|-------|
| `internal/builder/assets/builder.js` (`aiAddMsg`, `mdRender`, `aiStop`, history) | Chat UI shape and streaming UX |
| `internal/builder/assets/markdown.js` | Copied to `internal/web/assets/` (escape-first, zero-dep) |
| `internal/ai/` (`Client`, `Completion`, `CompletionStream`) | Provider transport — reused unchanged |
| `internal/cli/ai.go` (`resolveAI`) | Config resolution — already handles `[AI]` INI + `KALUA_AI_*` env |
| `materializeGridForm` (`grid.go:431`) | Runtime-form materialization pattern |
| `Query` (`session.go:3065`) | Cross-goroutine tool execution |

### Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        Go Host (Session)                        │
│  k.form.ai_agent{…}  ──► materialize __kai_chat form table      │
│         │                        │                              │
│         │                  App.Block(PendingFormShowValue)      │
│         │                        ║  (script suspends)          │
│         ▼                        ▼                              │
│  ┌──────────────┐   RunAsync   ┌──────────────────────────────┐ │
│  │ ai.AgentRun  │◄─────────────│  worker goroutine            │ │
│  │  (internal/ai│   ◄── Query ──┤   • CompletionStream loop   │ │
│  │   VM-free)   │  (actor)     │   • ExtractToolCalls         │ │
│  └──────┬───────┘              │   • exec tools via Query     │ │
│         │ onDelta              └──────────────────────────────┘ │
│         ▼                                                       │
│  outbox: ai_chat_open / _delta / _status / _done                │
└─────────────────────┼───────────────────────────────────────────┘
                      │ WebSocket
                      ▼
┌─────────────────────────────────────────────────────────────────┐
│                     Browser (app.js)                            │
│  • Markdown rendering (markdown.js, escape-first)              │
│  • Token deltas append to the streaming assistant bubble        │
│  • "Calling get_customer(7)…" status line during tool exec      │
│  • Enter / Send → ai_chat_send ; close → ai_chat_close          │
└─────────────────────────────────────────────────────────────────┘
```

### Tool-Calling Protocol

**Text protocol, model-agnostic.** `internal/ai` has no native tool support today
(`ChatRequest` has no `tools`; `ChatMessage` has only `Role`/`Content`), and the
default target is a local OpenAI-compatible endpoint where many models do not
implement `tool_calls` at all. So the model is instructed to emit a tagged JSON block:

```
<tool_call>{"name":"get_customer","arguments":{"id":7}}</tool_call>
```

Loop: stream assistant reply → `ExtractToolCalls(text)` → if any, execute each,
append the results as `role:"tool"`, re-prompt → repeat until the model answers in
prose or the turn bound is hit.

**No `lua` import in `internal/ai`.** The loop takes a `ToolExecutor` callback, so the
package stays VM-free and testable against an `httptest` fake.

### Auto-Exposed Tools

Auto-exposure is viable because gopher-lua retains enough debug info to build **real
JSON schemas**, not just a bare name list:

| Schema field | Source | Verified |
|--------------|--------|----------|
| Parameter names | `LFunction.Proto.DbgLocals[i].Name`, count from `Proto.NumParameters` | `get_customer(id, limit)` → `NumParameters=2`, `DbgLocals=[id, limit]` |
| Vararg | `Proto.IsVarArg` | `withvar(a, ...)` → `IsVarArg=7` |
| Description | Comment block above `Proto.LineDefined`, read from the script source | — |
| Types | `@param <name> <type>` in the comment, else `string` | — |

**Exclusions:** `main`; members of `vm.SandboxGlobals.Libs`; `k`, `K`, `CTRL`, `ARGS`;
any name starting with `_`; Go functions and library tables (kept only when
`Proto != nil`).

`k.ai.tool(name, {desc=, params=}, fn)` exists as an **override** — auto-exposure
cannot know intent, so a script can supply a better description or document a
parameter the comment missed.

### API Surface

#### New Function: `k.form.ai_agent([opts])` → transcript table

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `title` | `string` | `"AI Assistant"` | Form header text |
| `welcome` | `string` | — | Assistant message seeded into the transcript |
| `system` | `string` | — | Extra system prompt (persona, domain rules) |
| `max_turns` | `number` | `8` | Tool-loop bound; prevents a model looping forever |
| `tools` | `table` | auto | Explicit tool-name allowlist (default: every script global) |

**Return value** — array of message tables, in order:

```lua
{ {role = "user",      content = "customer 7?"},
  {role = "assistant", content = "Let me look that up."},
  {role = "tool",      name = "get_customer", content = '{"name":"Ada"}'},
  {role = "assistant", content = "Ada, 42."} }
```

#### New Function: `k.ai.tool(name, schema, fn)`

| Parameter | Type | Description |
|-----------|------|-------------|
| `name` | `string` | Tool name the model calls |
| `schema` | `table` | `{desc = string, params = {{name=, type=, required=}}}` |
| `fn` | `function` | Implementation (defaults to the global `name`) |

Overrides auto-exposure for `name`. Calling it with no `fn` registers the existing
global `name`.

### Example Usage

```lua
-- Tools are ordinary script functions. The comment above each one becomes
-- the description shown to the model.
function get_customer(id)
    -- Look up a customer by id
    return k.db_select("select id, name, city from customers where id = ?", {id})
end

function search_orders(email, since)
    -- Find a customer's orders since a date (YYYY-MM-DD)
    return k.db_select(
        "select o.id, o.total, o.created from orders o join customers c on c.id=o.customer_id " ..
        "where c.email = ? and o.created >= ? order by o.created desc limit 20",
        {email, since})
end

function money(n)
    -- Format a number as a currency amount
    return string.format("%.2f", n)
end

function main()
    k.connect_sqlite("app.db")

    local transcript = k.form.ai_agent{
        title     = "Sales assistant",
        welcome   = "Ask me about customers and orders.",
        system    = "You are a sales assistant. Always cite the customer id.",
        max_turns = 6,
    }

    -- Runs after the user closes the chat.
    for _, m in ipairs(transcript) do
        print(m.role .. ": " .. m.content)
    end
end
```

### Key Behavior Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| **Tool protocol** | Text (`<tool_call>{json}</tool_call>`) | `internal/ai` has no native tool support; many local models lack `tool_calls` entirely. Model-agnostic, fully unit-testable, no provider variance |
| **Tool exposure** | Auto-expose all script globals | Zero declaration. Made viable by `Proto` introspection (real param names) + comment-derived descriptions, so schemas are meaningful rather than noise |
| **Override hook** | `k.ai.tool` | Auto-exposure cannot know intent; a script can improve or hide a tool |
| **Semantics** | Blocks until closed, returns transcript | Matches the established `k.form.show` / `k.msgbox` idiom; the script gets the result naturally |
| **Config** | Reuse `[AI]` INI + `KALUA_AI_*` env, **no new flags** | `resolveAI` already exists and is shared with `builderCmd`; config already lives in one place |
| **Streaming** | Yes — token deltas | Matches the builder chat UX; a slow local model otherwise looks frozen (the builder needed a "still working…" nudge for exactly this) |
| **Tool execution** | `Session.Query`, **synchronous only** (phase 1) | `Query` runs the closure on the actor goroutine and cannot suspend |
| **Loop location** | `RunAsync` worker goroutine | The loop blocks on network I/O; the actor goroutine must stay responsive |
| **Turn bound** | `max_turns` (default 8) | A model that keeps calling tools terminates instead of hanging the session |
| **Markdown** | Reuse `markdown.js` (escape-first) | LLM output is untrusted; the renderer escapes everything before transforming |

### Suspend Plumbing

`k.form.show` resumes with `LNil` (`ResumeFormCoro`, `session.go:3294`). The chat must
resume with a table, so:

```go
// internal/vm/app.go
PendingFormShowValue   // resumes the blocked coroutine with a Lua table
```

```go
// internal/session/session.go
func (s *Session) ResumeFormCoroWith(name string, val lua.LValue) bool
```

`ResumeFormCoro` delegates to it with `LNil`, so no existing caller changes.

### WebSocket Message Types

All payload fields already exist on `common.OutboxMsg` / `common.InboxMsg` — no
protocol struct change required.

| Direction | Type | Payload |
|-----------|------|---------|
| Browser → Go | `ai_chat_send` | `{form, text}` |
| Browser → Go | `ai_chat_close` | `{form}` |
| Go → Browser | `ai_chat_open` | `{form, html}` — modal shell + transcript |
| Go → Browser | `ai_chat_delta` | `{form, text}` — one streamed token |
| Go → Browser | `ai_chat_status` | `{form, text}` — "Calling get_customer(7)…" / "Thinking…" |
| Go → Browser | `ai_chat_done` | `{form}` — turn finished (also emitted on error) |

### Implementation Phases

| Phase | Description | Days |
|-------|-------------|------|
| 1 | `internal/ai/tools.go` — `ToolCall`, `ToolSchema`, `ExtractToolCalls` | 1 |
| 2 | `internal/ai/agent.go` — `AgentRun` loop, `ToolExecutor`, turn bound | 1 |
| 3 | `internal/ai/collect.go` — `_G` walk, `Proto` introspection, comment extraction | 1 |
| 4 | `common/ai.go` `AIConfig` + `Options.AIConfig` + `runCmd` `resolveAI` wiring | 0.5 |
| 5 | `vm/app.go` `PendingFormShowValue` + `ResumeFormCoroWith` | 0.5 |
| 6 | `internal/bindings/aichat.go` — `k.form.ai_agent`, `k.ai.tool`, form materialization | 1.5 |
| 7 | `internal/session/aichat.go` — agent loop, inbox/outbox cases, `Query` tool exec | 2 |
| 8 | `markdown.js` → `web/assets/`; `app.js` handlers; `kalua.css`; `shell.html` | 1.5 |
| 9 | WASM parity — `common/brain.go` `RouteOutbox` cases + `session_wasm.go` stubs | 0.5 |
| 10 | `registerKnown` + `api_doc.go` + `USER_GUIDE.md` + `make gen-api` | 0.5 |
| 11 | Tests (below) | 1.5 |
| 12 | Demo app `testdata/apps/ai_agent_demo.lua` | 0.5 |
| **Total** | | **~12.5** |

### File Changes

| File | Changes |
|------|---------|
| `internal/ai/tools.go` | **New** — protocol types + `ExtractToolCalls` parser |
| `internal/ai/agent.go` | **New** — `AgentRun`, `ToolExecutor`, turn bound (no `lua` import) |
| `internal/ai/collect.go` | **New** — `_G` walk, `Proto` introspection, comment extraction |
| `internal/common/ai.go` | **New** — `AIConfig` (plain struct; `ai` imports `host` → `bindings`, so `bindings` cannot import `ai`) |
| `internal/bindings/aichat.go` | **New** — `k.form.ai_agent`, `k.ai.tool`, `__kai_chat` materialization |
| `internal/session/aichat.go` | **New** — agent loop, `ai_chat_*` inbox/outbox, `Query` tool exec |
| `internal/session/session.go` | `ResumeFormCoroWith`, new inbox types, ws event cases |
| `internal/vm/app.go` | `PendingFormShowValue` kind |
| `internal/bindings/bindings.go` | `Options.AIConfig`; `registerKnown["form.ai_agent"]`, `["ai.tool"]` |
| `internal/cli/cli.go` | `runCmd` calls `resolveAI` (interactive **and** headless `--test`) |
| `internal/common/brain.go` | `RouteOutbox` — 4 `ai_chat_*` cases (WASM parity) |
| `internal/session/session_wasm.go` | 4 stubs |
| `internal/web/assets/markdown.js` | **New** — copy of `builder/assets/markdown.js` |
| `internal/web/assets/app.js` | 4 `handleMessage` cases; `showAIChat`/`aiAppend`/`aiSend` |
| `internal/web/assets/kalua.css` | `.kalua-ai-*` styles |
| `internal/web/templates/shell.html` | One `<script src="/static/markdown.js">` tag |
| `internal/bindings/api_doc.go` | `k.form.ai_agent` + `k.ai.tool` entries (with params + example) |
| `testdata/apps/ai_agent_demo.lua` | **New** — demo |

### Dependencies

- No new Go dependencies — reuses `internal/ai`, `gopher-lua`, existing `net/http`
- No new JS dependencies — `markdown.js` is already zero-dep and escape-first

### Tests

**`internal/ai/tools_test.go`** (new)
- `TestExtractToolCalls` — single, multiple, whitespace/newline inside the block,
  fenced ```` ```json ````, prose passthrough, name-only (no `arguments`)
- `TestExtractToolCallsMalformed` — bad JSON yields a tool error result, not an abort

**`internal/ai/agent_test.go`** (new)
- `TestAgentRunToolLoop` — `httptest` fake OpenAI endpoint: tool call → executor →
  `role:"tool"` message → final prose; delta order preserved
- `TestAgentRunTurnBound` — a model that always calls tools stops at `max_turns`
- `TestAgentRunNoTools` — plain prose reply performs exactly one LLM round-trip

**`internal/ai/collect_test.go`** (new)
- `TestCollectSchemas` — param names from `Proto.DbgLocals`, vararg detection,
  comment-derived description, `@param` type hint
- `TestCollectExcludes` — `main`, `k`/`K`/`CTRL`/`ARGS`, lib members, `_`-prefixed

**`internal/session/aichat_test.go`** (new, modelled on `looper_e2e_test.go`)
- Real session: `ai_chat_send` → tool invoked with decoded args → result fed back →
  `ai_chat_delta` sequence → `ai_chat_close` → transcript table returned to the
  suspended coroutine
- Tool error surfaces as a `role:"tool"` error result, not a session crash

**`internal/cli/ai_test.go`** (extend)
- `run` resolves `[AI]` INI / `KALUA_AI_*` env into `Options.AIConfig`

Plus: `KALUA check testdata/apps/ai_agent_demo.lua`, `go test ./...`,
`go vet ./...`, `node --check internal/web/assets/app.js`.

### Implementation Status

Not started — plan only.

### Known Limitation (phase 1)

**Tools are synchronous.** A tool that itself calls a blocking KALUA API
(`k.form.show`, `k.http_request`, `k.msgbox`) will hang, because `Session.Query`
runs the closure on the actor goroutine and cannot suspend.

*Phase 2 route:* run tools in a coroutine registered in `asyncOps` with a completion
callback, so a suspending tool parks until its own resume lands. Note this is the same
limitation the existing `k.exec` has (`handleExec` collects return values even when
`Resume` yields).
