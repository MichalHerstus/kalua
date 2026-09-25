// CRUD Grid control support (see kforms_enhancements.md §7).
//
// k.ctrl.grid is a higher-level DB-linked tabular widget that subsumes the
// tabulator=true table pattern: it renders the same Tabulator container (so the
// Go pager and the browser's Tabulator lifecycle serve it unchanged) and adds
// CRUD-oriented config — pk_field, row_actions, global_actions, selection_mode,
// referenced/inline form — as data-k-grid-* attributes for the client to build
// the toolbar and action column on top.
//
// Reads (page/sort/filter) go through the shared bindings.FetchTablePage pager.
// Write operations (insert/update/delete) are wired server-side by later
// phases; Phase 1 exposes k.grid.refresh and k.grid.set_db_source.
package bindings

import (
	"fmt"
	"sort"
	"strings"

	"github.com/yuin/gopher-lua"

	"kalua/internal/common"
)

// registerGridOps installs the k.grid.* CRUD-grid operations. Called from
// registerControls so the operations share the controls API namespace.
func registerGridOps(e *Env) {
	// k.grid.set_db_source(form, name, opts) - swap a grid's data source
	e.register("grid.set_db_source", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)
		opts := L.OptTable(3, L.NewTable())

		ctrl := getControl(L, formName, name)
		if ctrl == nil {
			return 0
		}
		if ctrl.RawGetString("type").String() != "grid" {
			L.RaiseError("control %s is not a grid", name)
			return 0
		}

		if v := opts.RawGetString("db"); v != lua.LNil {
			ctrl.RawSetString("db", v)
		}
		if v := opts.RawGetString("query"); v != lua.LNil {
			ctrl.RawSetString("query", v)
		}
		if v := opts.RawGetString("columns"); v != lua.LNil {
			ctrl.RawSetString("columns", v)
		}
		if v := opts.RawGetString("page_size"); v != lua.LNil {
			ctrl.RawSetString("page_size", v)
		}
		if v := opts.RawGetString("count_query"); v != lua.LNil {
			ctrl.RawSetString("count_query", v)
		}
		if v := opts.RawGetString("where"); v != lua.LNil {
			ctrl.RawSetString("db_where", v)
		}
		if v := opts.RawGetString("order_by"); v != lua.LNil {
			ctrl.RawSetString("db_order_by", v)
		}
		if v := opts.RawGetString("pk_field"); v != lua.LNil {
			ctrl.RawSetString("pk_field", v)
		}
		if v := opts.RawGetString("selection_mode"); v != lua.LNil {
			ctrl.RawSetString("selection_mode", v)
		}

		// Refresh immediately so the new source is visible.
		gridRefresh(e, formName, name)
		return 0
	})

	// k.grid.refresh(form, name) - reload page 1
	e.register("grid.refresh", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)

		ctrl := getControl(L, formName, name)
		if ctrl == nil {
			return 0
		}
		if ctrl.RawGetString("type").String() != "grid" {
			L.RaiseError("control %s is not a grid", name)
			return 0
		}

		gridRefresh(e, formName, name)
		return 0
	})

	// k.grid.get_selected(form, name) - async, returns selected rows via coroutine
	e.register("grid.get_selected", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)

		ctrl := getControl(L, formName, name)
		if ctrl == nil {
			L.Push(lua.LNil)
			return 1
		}
		if ctrl.RawGetString("type").String() != "grid" {
			L.RaiseError("control %s is not a grid", name)
			return 0
		}

		co, cancel := L.NewThread()
		reqID := e.Sess.RequestGridGetSelected(co, cancel, formName, name)

		// Store the request ID to match the response
		ctrl.RawSetString("_grid_sel_req", lua.LString(reqID))

		// Suspend and wait for response
		L.Push(co)
		return L.Yield(lua.LNil)
	})

	// k.grid.get_row(form, name, pk) - async, returns single row by PK
	e.register("grid.get_row", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)
		pk := L.Get(3)

		ctrl := getControl(L, formName, name)
		if ctrl == nil {
			L.Push(lua.LNil)
			return 1
		}
		if ctrl.RawGetString("type").String() != "grid" {
			L.RaiseError("control %s is not a grid", name)
			return 0
		}

		co, cancel := L.NewThread()
		_ = e.Sess.RequestGridGetRow(co, cancel, formName, name, pk)

		L.Push(co)
		return L.Yield(lua.LNil)
	})

	// k.grid.delete_row(form, name, pk) - async, returns success
	e.register("grid.delete_row", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)
		pk := L.Get(3)

		ctrl := getControl(L, formName, name)
		if ctrl == nil {
			L.Push(lua.LFalse)
			return 1
		}
		if ctrl.RawGetString("type").String() != "grid" {
			L.RaiseError("control %s is not a grid", name)
			return 0
		}

		co, cancel := L.NewThread()
		_ = e.Sess.RequestGridDeleteRow(co, cancel, formName, name, pk)

		L.Push(co)
		return L.Yield(lua.LNil)
	})

	// k.grid.batch_delete(form, name, {pks}) - async, returns success
	e.register("grid.batch_delete", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)
		pksTbl := L.CheckTable(3)

		ctrl := getControl(L, formName, name)
		if ctrl == nil {
			L.Push(lua.LFalse)
			return 1
		}
		if ctrl.RawGetString("type").String() != "grid" {
			L.RaiseError("control %s is not a grid", name)
			return 0
		}

		var pks []interface{}
		pksTbl.ForEach(func(_, v lua.LValue) {
			pks = append(pks, v)
		})

		co, cancel := L.NewThread()
		_ = e.Sess.RequestGridBatchDelete(co, cancel, formName, name, pks)

		L.Push(co)
		return L.Yield(lua.LNil)
	})

	// k.grid.insert_row(form, name, data) - async, returns PK
	e.register("grid.insert_row", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)
		data := L.CheckTable(3)

		ctrl := getControl(L, formName, name)
		if ctrl == nil {
			L.Push(lua.LNil)
			return 1
		}
		if ctrl.RawGetString("type").String() != "grid" {
			L.RaiseError("control %s is not a grid", name)
			return 0
		}

		vals := make(map[string]interface{})
		data.ForEach(func(k, v lua.LValue) {
			vals[k.String()] = v
		})

		co, cancel := L.NewThread()
		_ = e.Sess.RequestGridInsertRow(co, cancel, formName, name, vals)

		L.Push(co)
		return L.Yield(lua.LNil)
	})

	// k.grid.update_row(form, name, pk, data) - async, returns success
	e.register("grid.update_row", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)
		pk := L.Get(3)
		data := L.CheckTable(4)

		ctrl := getControl(L, formName, name)
		if ctrl == nil {
			L.Push(lua.LFalse)
			return 1
		}
		if ctrl.RawGetString("type").String() != "grid" {
			L.RaiseError("control %s is not a grid", name)
			return 0
		}

		vals := make(map[string]interface{})
		data.ForEach(func(k, v lua.LValue) {
			vals[k.String()] = v
		})

		co, cancel := L.NewThread()
		_ = e.Sess.RequestGridUpdateRow(co, cancel, formName, name, pk, vals)

		L.Push(co)
		return L.Yield(lua.LNil)
	})
}

// gridRefresh reloads a grid's first page in the browser.
func gridRefresh(e *Env, formName, name string) {
	sendOutbox(e, common.OutboxMsg{
		Type:     "tabulator_refresh",
		Form:     formName,
		Ctrl:     name,
		Selector: "#c:" + formName + ":" + name,
	})
}

// renderGrid renders a grid control. The inner element is the standard
// `.kalua-tabulator-table` container the browser's initTabulators manages; the
// outer `.kalua-grid` wrapper carries the data-k-grid-* CRUD configuration the
// client uses to build the toolbar, action column and form modal.
func renderGrid(ctrl *lua.LTable, formName, name, id, visible string) string {
	optionsJSON, columnsJSON, dataJSON := tabulatorWidgetJSON(ctrl)

	// Apply default row_actions if not specified but grid has any row action capability
	rowActionsJSON := ""
	if v := ctrl.RawGetString("row_actions"); v != lua.LNil {
		if aTbl, ok := v.(*lua.LTable); ok {
			if j := gridConfigJSON(aTbl); j != "" {
				rowActionsJSON = j
			}
		}
	} else {
		// Default: view, edit, delete
		rowActionsJSON = `{"view":true,"edit":true,"delete":true}`
	}

	// Apply default global_actions if not specified
	globalActionsJSON := ""
	if v := ctrl.RawGetString("global_actions"); v != lua.LNil {
		if aTbl, ok := v.(*lua.LTable); ok {
			if j := gridConfigJSON(aTbl); j != "" {
				globalActionsJSON = j
			}
		}
	} else {
		// Default: new_record, batch_delete
		globalActionsJSON = `{"new_record":true,"batch_delete":true}`
	}

	gridAttrs := ` data-k-grid="1" data-k-grid-selection="multi"`
	if v := ctrl.RawGetString("selection_mode"); v != lua.LNil && v.String() != "" {
		gridAttrs = ` data-k-grid="1" data-k-grid-selection="` + escAttr(v.String()) + `"`
	}
	if v := ctrl.RawGetString("pk_field"); v != lua.LNil && v.String() != "" {
		gridAttrs += ` data-k-grid-pk="` + escAttr(v.String()) + `"`
	}
	if rowActionsJSON != "" {
		gridAttrs += ` data-k-grid-row-actions="` + escAttr(rowActionsJSON) + `"`
	}
	if globalActionsJSON != "" {
		gridAttrs += ` data-k-grid-global-actions="` + escAttr(globalActionsJSON) + `"`
	}
	if v := ctrl.RawGetString("row_click_action"); v != lua.LNil && v.String() != "" {
		gridAttrs += ` data-k-grid-row-click="` + escAttr(v.String()) + `"`
	}
	if v := ctrl.RawGetString("column_visibility"); v != lua.LNil && v.String() == "true" {
		gridAttrs += ` data-k-grid-column-visibility="true"`
	}
	if v := ctrl.RawGetString("form"); v != lua.LNil {
		if fTbl, ok := v.(*lua.LTable); ok {
			gridAttrs += ` data-k-grid-form="` + escAttr(luaTableToJSON(fTbl)) + `"`
		} else if v.String() != "" {
			gridAttrs += ` data-k-grid-formref="` + escAttr(v.String()) + `"`
		}
	}

	return `<div class="kalua-control"` + visible + `>
		<div class="kalua-grid" data-k-form="` + escAttr(formName) + `" data-k-ctrl="` + escAttr(name) + `"` + gridAttrs + `>
			<div class="kalua-tabulator-wrapper">
				<div id="` + escAttr(id) + `" class="kalua-tabulator-table"
				     data-k-form="` + escAttr(formName) + `" data-k-ctrl="` + escAttr(name) + `"
				     data-k-tabulator-options="` + escAttr(optionsJSON) + `"
				     data-k-tabulator-columns="` + escAttr(columnsJSON) + `"
				     data-k-tabulator-data="` + escAttr(dataJSON) + `"></div>
			</div>
		</div>
	</div>`
}

// gridConfigJSON serializes a row_actions/global_actions config table for the
// data-k-grid-* attribute. Function values (custom onclick handlers) cannot
// cross the WebSocket boundary and are dropped; the client reads those through
// k.form.on handlers instead.
func gridConfigJSON(tbl *lua.LTable) string {
	var parts []string
	isArray := true
	expected := 1
	tbl.ForEach(func(k, v lua.LValue) {
		if v.Type() == lua.LTFunction || v == lua.LNil {
			isArray = false
			return
		}
		if n, ok := k.(lua.LNumber); ok && int(n) == expected {
			expected++
		} else {
			isArray = false
		}
		parts = append(parts, luaValueJSON(v))
	})
	if len(parts) == 0 {
		return ""
	}
	if isArray {
		return "[" + strings.Join(parts, ",") + "]"
	}
	var keys []string
	keyMap := map[string]lua.LValue{}
	tbl.ForEach(func(k, v lua.LValue) {
		if v.Type() == lua.LTFunction || v == lua.LNil {
			return
		}
		keyMap[k.String()] = v
		keys = append(keys, k.String())
	})
	sort.Strings(keys)
	var objParts []string
	for _, k := range keys {
		objParts = append(objParts, `"`+jsonEscape(k)+`":`+luaValueJSON(keyMap[k]))
	}
	if len(objParts) == 0 {
		return ""
	}
	return "{" + strings.Join(objParts, ",") + "}"
}

// GridTableFromQuery extracts a single table name from a SELECT query for the
// grid's write operations. Only plain SELECT ... FROM tbl forms are accepted;
// joins, subqueries, WITH and non-SELECT statements cannot be mapped to a
// single write target.
func GridTableFromQuery(query string) (string, error) {
	q := strings.TrimSpace(query)
	q = strings.TrimSpace(strings.TrimSuffix(q, ";"))
	lower := strings.ToLower(q)
	prefix := ""
	if strings.HasPrefix(lower, "select") {
		prefix = "select"
	} else if strings.HasPrefix(lower, "with") {
		prefix = "with"
	} else {
		return "", fmt.Errorf("grid query is not a plain SELECT")
	}
	rest := strings.TrimSpace(q[len(prefix):])
	loc := keywordBounds(rest, "from")
	if loc < 0 {
		return "", fmt.Errorf("grid query has no FROM clause")
	}
	target := strings.TrimSpace(rest[loc+4:]) // skip "from"
	fields := strings.Fields(target)
	if len(fields) == 0 {
		return "", fmt.Errorf("grid query has empty FROM clause")
	}
	name := strings.Trim(fields[0], "\"`'[]()")
	if !isValidIdentifier(name) {
		return "", fmt.Errorf("cannot derive a write table from %q", fields[0])
	}
	return name, nil
}

// keywordBounds finds the byte offset of a whole-word keyword (case-insensitive).
func keywordBounds(s, kw string) int {
	for i := 0; i+len(kw) <= len(s); i++ {
		if s[i] == kw[0] || s[i] == kw[0]-32 || s[i] == kw[0]+32 {
			if strings.EqualFold(s[i:i+len(kw)], kw) {
				before := i == 0 || !isIdentByte(s[i-1])
				after := i+len(kw) == len(s) || !isIdentByte(s[i+len(kw)])
				if before && after {
					return i
				}
			}
		}
	}
	return -1
}

func isIdentByte(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
}

// GridPKFromControl resolves the grid's primary key column: an explicit
// pk_field, else a column literally named "id" in db_columns/columns, else the
// first data column, else "id" as a safe fallback.
func GridPKFromControl(ctrl *lua.LTable) string {
	if v := ctrl.RawGetString("pk_field"); v != lua.LNil && v.String() != "" {
		if isValidIdentifier(v.String()) {
			return v.String()
		}
	}

	var fields []string
	if v := ctrl.RawGetString("db_columns"); v != lua.LNil {
		if t, ok := v.(*lua.LTable); ok {
			t.ForEach(func(_, cv lua.LValue) {
				if s := cv.String(); s != "" && isValidIdentifier(s) {
					fields = append(fields, s)
				}
			})
		}
	}
	if len(fields) == 0 {
		if v := ctrl.RawGetString("columns"); v != lua.LNil {
			if t, ok := v.(*lua.LTable); ok {
				t.ForEach(func(_, cv lua.LValue) {
					if cd, ok := cv.(*lua.LTable); ok {
						if f := cd.RawGetString("field"); f != lua.LNil && isValidIdentifier(f.String()) {
							fields = append(fields, f.String())
						}
					}
				})
			}
		}
	}
	if len(fields) > 0 {
		for _, f := range fields {
			if f == "id" {
				return f
			}
		}
		return fields[0]
	}
	return "id"
}

// GridFormModalName returns the LState global form name a grid's detail modal
// uses: the referenced form name when ctrl.form is a string, else a synthetic
// name derived from the grid's form/ctrl identity.
func GridFormModalName(ctrl *lua.LTable, formName, ctrlName string) string {
	if v := ctrl.RawGetString("form"); v != lua.LNil {
		if _, isTbl := v.(*lua.LTable); !isTbl && v.String() != "" {
			return v.String()
		}
	}
	return "__kgrid_" + formName + "_" + ctrlName
}

// BuildGridForm materializes and renders the detail form a grid edit/view/new
// modal should show, returning the form's global name and its rendered HTML.
// Three sources are supported in order of precedence:
//
//   - ctrl.form = "FormName"      renders the referenced (already defined) form
//   - ctrl.form = {controls=...}  materializes the inline definition into the
//     synthetic global form and renders it
//   - neither                     auto-generates a textbox-per-column form from
//     the grid's db_columns/columns
func BuildGridForm(L *lua.LState, formName, ctrlName string, ctrl *lua.LTable) (string, string, error) {
	modalName := GridFormModalName(ctrl, formName, ctrlName)
	if v := ctrl.RawGetString("form"); v != lua.LNil {
		if def, ok := v.(*lua.LTable); ok {
			materializeGridForm(L, modalName, def)
			return modalName, renderForm(L, modalName), nil
		}
		if ref := v.String(); ref != "" {
			if getForm(L, ref) == nil {
				return "", "", fmt.Errorf("referenced form %q is not defined", ref)
			}
			return ref, renderForm(L, ref), nil
		}
	}
	materializeGridAutoForm(L, modalName, ctrl)
	if getForm(L, modalName) == nil {
		return "", "", fmt.Errorf("grid %s/%s has no formable columns", formName, ctrlName)
	}
	return modalName, renderForm(L, modalName), nil
}

// GridFormGap returns the modal gap for ctrl.form_gap (number or {x,y}).
func GridFormGap(ctrl *lua.LTable) (float64, float64) {
	v := ctrl.RawGetString("form_gap")
	if v == lua.LNil {
		return 10, 10
	}
	if n := lua.LVAsNumber(v); n > 0 {
		f := float64(n)
		return f, f
	}
	if t, ok := v.(*lua.LTable); ok {
		x := float64(lua.LVAsNumber(t.RawGetString("x")))
		y := float64(lua.LVAsNumber(t.RawGetString("y")))
		if x <= 0 {
			x = 10
		}
		if y <= 0 {
			y = x
		}
		return x, y
	}
	return 10, 10
}

// materializeGridForm rebuilds the synthetic global form modalName from an
// inline {title=, gap=, controls={...}} definition. Each control entry is a
// {type=, name=, label=, opts={...}} table; opts are shallow-cloned so the
// inline definition is never mutated by later rendering.
func materializeGridForm(L *lua.LState, name string, def *lua.LTable) {
	tbl := ensureGridForm(L, name)
	if v := def.RawGetString("title"); v != lua.LNil && v.String() != "" {
		tbl.RawSetString("title", v)
	}
	if v := def.RawGetString("gap"); v != lua.LNil {
		tbl.RawSetString("gap", v)
	}
	controls := def.RawGetString("controls")
	ct, ok := controls.(*lua.LTable)
	if !ok {
		return
	}
	ct.ForEach(func(_, v lua.LValue) {
		cd, ok := v.(*lua.LTable)
		if !ok {
			return
		}
		typ := cd.RawGetString("type").String()
		cname := cd.RawGetString("name").String()
		if typ == "" || cname == "" {
			return
		}
		opts := L.NewTable()
		if o := cd.RawGetString("opts"); o != nil && o != lua.LNil {
			if ot, ok := o.(*lua.LTable); ok {
				cloneTableInto(L, ot, opts)
			}
		}
		if lbl := cd.RawGetString("label"); lbl != lua.LNil {
			opts.RawSetString("label", lbl)
		}
		addControl(L, name, cname, typ, opts)
	})
}

// materializeGridAutoForm builds a textbox-per-column detail form from the
// grid's db_columns (field names) or its columns (field/title pairs) when the
// grid carries no explicit form definition.
func materializeGridAutoForm(L *lua.LState, name string, ctrl *lua.LTable) {
	tbl := ensureGridForm(L, name)
	tbl.RawSetString("title", lua.LString("Record Details"))

	var fields []string
	titleOf := map[string]string{}
	if v := ctrl.RawGetString("db_columns"); v != lua.LNil {
		if t, ok := v.(*lua.LTable); ok {
			t.ForEach(func(_, cv lua.LValue) {
				if s := cv.String(); s != "" && isValidIdentifier(s) {
					fields = append(fields, s)
					titleOf[s] = s
				}
			})
		}
	}
	if len(fields) == 0 {
		if v := ctrl.RawGetString("columns"); v != lua.LNil {
			if t, ok := v.(*lua.LTable); ok {
				t.ForEach(func(_, cv lua.LValue) {
					if cd, ok := cv.(*lua.LTable); ok {
						f := cd.RawGetString("field").String()
						if f == "" || !isValidIdentifier(f) {
							return
						}
						fields = append(fields, f)
						if td := cd.RawGetString("title"); td != lua.LNil && td.String() != "" {
							titleOf[f] = td.String()
						} else {
							titleOf[f] = f
						}
					}
				})
			}
		}
	}
	for _, f := range fields {
		opts := L.NewTable()
		opts.RawSetString("label", lua.LString(titleOf[f]))
		addControl(L, name, f, "textbox", opts)
	}
}

// ensureGridForm returns a fresh empty global form table, resetting any prior
// control lists so re-materialization never accumulates stale controls.
func ensureGridForm(L *lua.LState, name string) *lua.LTable {
	tbl := getForm(L, name)
	if tbl == nil {
		tbl = L.NewTable()
		L.SetGlobal(name, tbl)
	}
	tbl.RawSetString("controls", nil)
	tbl.RawSetString("order", nil)
	tbl.RawSetString("title", nil)
	tbl.RawSetString("gap", nil)
	return tbl
}

// cloneTableInto shallow-copies a Lua table into a fresh target table.
func cloneTableInto(L *lua.LState, src, dst *lua.LTable) {
	src.ForEach(func(k, v lua.LValue) {
		dst.RawSet(k, v)
	})
}

// placeholders returns count placeholder tokens starting at 1-based index n.
func placeholders(h *DBHandle, n, count int) string {
	parts := make([]string, count)
	for i := 0; i < count; i++ {
		parts[i] = h.Placeholder(n + i)
	}
	return strings.Join(parts, ", ")
}

// GridWrite executes a write statement against a grid's DB handle. The handle
// id is resolved through the shared handle registry (opaque runtime id or
// --db name). Intended for grid save/delete flows driven by the session actor.
func GridWrite(handleID, query string, params ...interface{}) error {
	h := getDBHandle(nil, handleID)
	if h == nil {
		return fmt.Errorf("unknown database handle %q", handleID)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := h.db.Exec(query, params...)
	return err
}

// GridInsert inserts one row (values keyed by column). An empty-string primary
// key value is skipped so auto-increment tables can be written with the client's
// blank pk field. Invalid column identifiers are dropped.
func GridInsert(handleID, table, pkCol string, values map[string]interface{}) error {
	cols := sortedKeys(values)
	var setCols []string
	var params []interface{}
	var n = 1
	for _, c := range cols {
		if c == pkCol {
			if s, ok := values[c].(string); ok && s == "" {
				continue
			}
		}
		if !isValidIdentifier(c) {
			continue
		}
		setCols = append(setCols, c)
		params = append(params, values[c])
		n++
	}
	if len(setCols) == 0 {
		return fmt.Errorf("no valid columns to insert")
	}
	h := getDBHandle(nil, handleID)
	if h == nil {
		return fmt.Errorf("unknown database handle %q", handleID)
	}
	q := "INSERT INTO " + table + " (" + strings.Join(setCols, ", ") + ") VALUES (" + placeholders(h, 1, len(setCols)) + ")"

	h.mu.Lock()
	defer h.mu.Unlock()
	if _, err := h.db.Exec(q, params...); err != nil {
		return fmt.Errorf("grid insert: %w", err)
	}
	return nil
}

// GridUpdate updates a single row keyed by pkCol with pk as the key's scalar.
// The primary key column is excluded from the SET list; invalid identifiers are
// dropped.
func GridUpdate(handleID, table, pkCol string, pk interface{}, values map[string]interface{}) error {
	h := getDBHandle(nil, handleID)
	if h == nil {
		return fmt.Errorf("unknown database handle %q", handleID)
	}

	var setSQL []string
	var params []interface{}
	n := 1
	cols := sortedKeys(values)
	for _, c := range cols {
		if c == pkCol || !isValidIdentifier(c) {
			continue
		}
		setSQL = append(setSQL, c+" = "+h.Placeholder(n))
		params = append(params, values[c])
		n++
	}
	if len(setSQL) == 0 {
		return fmt.Errorf("no valid columns to update")
	}
	q := "UPDATE " + table + " SET " + join(setSQL, ", ") + " WHERE " + pkCol + " = " + h.Placeholder(n)
	params = append(params, pk)

	h.mu.Lock()
	defer h.mu.Unlock()
	if _, err := h.db.Exec(q, params...); err != nil {
		return fmt.Errorf("grid update: %w", err)
	}
	return nil
}

// GridDeleteMany deletes rows whose pkCol matches any of pks (single-value or
// IN list). Nil entries are skipped.
func GridDeleteMany(handleID, table, pkCol string, pks []interface{}) error {
	var clean []interface{}
	for _, p := range pks {
		if p == nil {
			continue
		}
		clean = append(clean, p)
	}
	if len(clean) == 0 {
		return fmt.Errorf("no primary key values to delete")
	}

	h := getDBHandle(nil, handleID)
	if h == nil {
		return fmt.Errorf("unknown database handle %q", handleID)
	}
	var q string
	var params = clean
	if len(clean) == 1 {
		q = "DELETE FROM " + table + " WHERE " + pkCol + " = " + h.Placeholder(1)
	} else {
		q = "DELETE FROM " + table + " WHERE " + pkCol + " IN (" + placeholders(h, 1, len(clean)) + ")"
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if _, err := h.db.Exec(q, params...); err != nil {
		return fmt.Errorf("grid delete: %w", err)
	}
	return nil
}

func sortedKeys(values map[string]interface{}) []string {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}