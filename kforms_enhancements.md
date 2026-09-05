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

## 3.1 Textbox Enhancements

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

## 3.2 Label Enhancement

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

## 3.3 Image Control (New)

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

### Dynamic Update
- `k.ctrl.set_value(form, name, new_src)` → updates `src` attribute
- `k.ctrl.set_property(form, name, "src", new_src)` → same
- `k.ctrl.set_property(form, name, "alt", ...)` etc.

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
| **image** | `name`, `type`, `src` | `alt`, `width`, `height`, `fit`, `clickable`, `onclick`, `enabled`, `visible` |

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

## Architecture

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
__Decision: Option B, standalone web app__

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
        "type": { "type": "string", "enum": ["label", "textbox", "button", "combo", "list", "table", "checkbox", "radio", "image"] },
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
        "datetime": { "type": "object" },
        "src": { "type": "string" },
        "alt": { "type": "string" },
        "width": { "type": ["number", "string"] },
        "height": { "type": ["number", "string"] },
        "fit": { "type": "string", "enum": ["cover", "contain", "fill", "scale-down", "none"] },
        "clickable": { "type": "boolean" }
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
__Decision: Option 1__

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
- **Not exported** — builder only handles form structure
- User adds logic manually in Lua after export
- Builder can insert placeholder comments:
  ```lua
  k.ctrl.button("main", "btn1", {label="Click Me"})
  -- TODO: Add onclick handler: k.form.on("main", "btn1", "click", function() ... end)
  ```

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
| **Multi-form support** | Single form vs multiple | Single form per file (per spec) |
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
| -------------------------------------- | ----- | ------------------------------------ |

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
| 6 | Form Builder | 17 days |
| **Total** | | **~60 days** |

---

## Next Steps

1. **Confirm architecture**: VS Code webview vs standalone? -> __Standalone__
2. **Prioritize controls**: All 8 basic controls + image, or subset first? -> __all__
3. **Layout system**: Vertical-only MVP sufficient? -> __no, even in MVP I want more options of layout__
4. **Event handler strategy**: Placeholder comments acceptable? -> __Yes__
5. **File format**: `.kalua-form.json` as primary, Lua as export-only? -> __yes__

Please confirm decisions on open questions, and I'll create detailed technical specs for Phase 1.
