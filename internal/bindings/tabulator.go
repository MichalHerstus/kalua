// Tabulator table control support (see kforms_enhancements.md §1).
//
// When k.ctrl.table is created with tabulator=true, the control renders as a
// <div> container with data-* attributes carrying the Tabulator options,
// columns and row data. The browser (app.js) instantiates and manages the
// Tabulator instance. The k.table.* operations in this file push update /
// query messages through the session outbox.
package bindings

import (
	"encoding/json"
	"strings"

	"github.com/yuin/gopher-lua"

	"kalua/internal/common"
)

// registerTableOps installs the k.table.* Tabulator data operations. Called
// from registerControls so the operations share the same API namespace.
func registerTableOps(e *Env) {
	// k.table.set_data(form, name, dataTable) - bulk replace all row data
	e.register("table.set_data", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)
		data := L.CheckTable(3)

		ctrl := getControl(L, formName, name)
		if ctrl == nil {
			return 0
		}
		if ctrl.RawGetString("type").String() != "table" {
			L.RaiseError("control %s is not a table", name)
			return 0
		}

		if isTabulator(ctrl) {
			sendOutbox(e, common.OutboxMsg{
				Type:     "tabulator_update",
				Form:     formName,
				Ctrl:     name,
				Selector: "#c:" + formName + ":" + name,
				Data:     luaTableToJSON(data),
			})
		} else {
			// Traditional table: store rows and re-render.
			rows := L.NewTable()
			data.ForEach(func(_ lua.LValue, v lua.LValue) {
				if rowTbl, ok := v.(*lua.LTable); ok {
					rows.RawSetInt(rows.Len()+1, rowTbl)
				}
			})
			ctrl.RawSetString("rows", rows)
			html := renderControl(ctrl)
			sendOutbox(e, common.OutboxMsg{
				Type:     "update_control",
				Form:     formName,
				Ctrl:     name,
				Selector: "#c:" + formName + ":" + name,
				HTML:     html,
			})
		}
		return 0
	})

	// k.table.get_data(form, name) - get all current data from table
	e.register("table.get_data", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)

		ctrl := getControl(L, formName, name)
		if ctrl == nil || ctrl.RawGetString("type").String() != "table" {
			L.Push(lua.LNil)
			return 1
		}

		if isTabulator(ctrl) {
			if e.Sess == nil {
				L.RaiseError("table.get_data: no session available")
				return 0
			}
			// Suspend the coroutine; the browser answers via tabulator_data_resp
			// which the session resumes with a Lua table of rows.
			e.Sess.RequestTabulatorGetData(L, func() {}, formName, name)
			return L.Yield(lua.LNil)
		}
		L.Push(ctrl.RawGetString("rows"))
		return 1
	})

	// k.table.get_selected_rows(form, name) - get selected row indices (1-based)
	e.register("table.get_selected_rows", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)

		ctrl := getControl(L, formName, name)
		if ctrl == nil || ctrl.RawGetString("type").String() != "table" {
			L.Push(lua.LNil)
			return 1
		}

		if isTabulator(ctrl) {
			if e.Sess == nil {
				L.RaiseError("table.get_selected_rows: no session available")
				return 0
			}
			e.Sess.RequestTabulatorGetSelection(L, func() {}, formName, name)
			return L.Yield(lua.LNil)
		}
		L.Push(ctrl.RawGetString("selected_column"))
		return 1
	})

	// k.table.set_remote_data(form, name, {data=..., last_page=..., last_row=...})
	// pushes server-side pagination data to the browser. The table typically
	// stays in sync via tabulator_ajax_request round-trips; this is the
	// app-initiating variant used to seed or replace the current page.
	e.register("table.set_remote_data", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)
		opts := L.OptTable(3, L.NewTable())

		ctrl := getControl(L, formName, name)
		if ctrl == nil {
			return 0
		}
		if ctrl.RawGetString("type").String() != "table" {
			L.RaiseError("control %s is not a table", name)
			return 0
		}
		if !isTabulator(ctrl) {
			L.RaiseError("control %s is not a tabulator table", name)
			return 0
		}

		remote := remotePageFromLua(opts)
		sendOutbox(e, common.OutboxMsg{
			Type:     "tabulator_remote_data",
			Form:     formName,
			Ctrl:     name,
			Selector: "#c:" + formName + ":" + name,
			Data:     remotePayloadJSON(remote),
		})
		return 0
	})
}

// remotePage is the tabulator_remote_data payload.
type remotePage struct {
	Data     string
	LastPage int
	LastRow  int
}

// remotePageFromLua extracts {data, last_page, last_row} from a Lua table.
func remotePageFromLua(tbl *lua.LTable) remotePage {
	var r remotePage
	if d := tbl.RawGetString("data"); d != lua.LNil {
		if dTbl, ok := d.(*lua.LTable); ok {
			r.Data = luaTableToJSON(dTbl)
		} else {
			r.Data = luaValueJSON(d)
		}
	}
	if v := tbl.RawGetString("last_page"); v != lua.LNil {
		r.LastPage = int(lua.LVAsNumber(v))
	}
	if v := tbl.RawGetString("last_row"); v != lua.LNil {
		r.LastRow = int(lua.LVAsNumber(v))
	}
	return r
}

// remotePayloadJSON serializes a remotePage for the tabulator_remote_data
// WebSocket message.
func remotePayloadJSON(r remotePage) string {
	data := r.Data
	if data == "" {
		data = "[]"
	}
	type wire struct {
		Data     json.RawMessage `json:"data"`
		LastPage int             `json:"last_page,omitempty"`
		LastRow  int             `json:"last_row,omitempty"`
	}
	out, _ := json.Marshal(wire{Data: json.RawMessage(data), LastPage: r.LastPage, LastRow: r.LastRow})
	return string(out)
}

// isTabulator reports whether a table control has tabulator=true enabled.
func isTabulator(ctrl *lua.LTable) bool {
	t := ctrl.RawGetString("tabulator")
	return t != lua.LNil && t.String() == "true"
}

// renderTable renders a table control, dispatching to the traditional or the
// Tabulator renderer. Accessed via renderControl's "table" case.
func renderTable(ctrl *lua.LTable, formName, name, id, label, value, visible, enabled, attrs string) string {
	if isTabulator(ctrl) {
		return renderTabulatorTable(ctrl, formName, name, id, label, visible, enabled)
	}
	return renderTraditionalTable(ctrl, formName, name, id, label, value, visible, enabled, attrs)
}

// renderTabulatorTable renders the container div. The client reads the
// data-k-tabulator-* attributes and initializes a Tabulator instance.
func renderTabulatorTable(ctrl *lua.LTable, formName, name, id, label, visible, enabled string) string {
	optionsJSON := `{"layout":"fitColumns","selectable":true,"selectableRangeMode":"click"}`
	if to := ctrl.RawGetString("tabulatorOptions"); to != lua.LNil {
		if toTbl, ok := to.(*lua.LTable); ok {
			optionsJSON = luaTableToJSON(toTbl)
		} else if to.String() != "" {
			optionsJSON = to.String()
		}
	}

	columnsJSON := "[]"
	if cols := ctrl.RawGetString("columns"); cols != lua.LNil {
		if colsTbl, ok := cols.(*lua.LTable); ok {
			var colStrs []string
			colsTbl.ForEach(func(_ lua.LValue, v lua.LValue) {
				if colTbl, ok := v.(*lua.LTable); ok {
					colStrs = append(colStrs, luaTableToJSON(colTbl))
				} else if v.String() != "" {
					colStrs = append(colStrs, `"`+jsonEscape(v.String())+`"`)
				}
			})
			columnsJSON = "[" + strings.Join(colStrs, ",") + "]"
		}
	}

	dataJSON := "[]"
	if data := ctrl.RawGetString("data"); data != lua.LNil {
		if dataTbl, ok := data.(*lua.LTable); ok {
			dataJSON = luaTableToJSON(dataTbl)
		}
	}

	return `<div class="kalua-control"` + visible + `>
		<div class="kalua-tabulator-wrapper"` + enabled + `>
			<div id="` + escAttr(id) + `" class="kalua-tabulator-table"
			     data-k-form="` + escAttr(formName) + `" data-k-ctrl="` + escAttr(name) + `"
			     data-k-tabulator-options="` + escAttr(optionsJSON) + `"
			     data-k-tabulator-columns="` + escAttr(columnsJSON) + `"
			     data-k-tabulator-data="` + escAttr(dataJSON) + `"></div>
		</div>
	</div>`
}

// renderTraditionalTable renders the classic <table> control.
func renderTraditionalTable(ctrl *lua.LTable, formName, name, id, label, value, visible, enabled, attrs string) string {
	columns := ctrl.RawGetString("columns")
	rows := ctrl.RawGetString("rows")

	var thead string
	if columnsTbl, ok := columns.(*lua.LTable); ok {
		thead = "<thead><tr>"
		columnsTbl.ForEach(func(k, v lua.LValue) {
			thead += `<th data-k-col="` + escAttr(k.String()) + `">` + escText(v.String()) + `</th>`
		})
		thead += "</tr></thead>"
	} else {
		thead = `<thead><tr><th>` + label + `</th></tr></thead>`
	}

	var tbody string
	if rowsTbl, ok := rows.(*lua.LTable); ok {
		tbody = "<tbody>"
		rowsTbl.ForEach(func(k, v lua.LValue) {
			if rowTbl, ok := v.(*lua.LTable); ok {
				tbody += "<tr data-k-row=\"" + escAttr(k.String()) + "\">"
				rowTbl.ForEach(func(colK, colV lua.LValue) {
					tbody += `<td data-k-col="` + escAttr(colK.String()) + `">` + escText(colV.String()) + `</td>`
				})
				tbody += "</tr>"
			}
		})
		tbody += "</tbody>"
	} else {
		tbody = "<tbody></tbody>"
	}

	return `<div class="kalua-control"` + visible + `>
		<table class="kalua-table" id="` + escAttr(id) + `"` + attrs + enabled + `>` + thead + tbody + `</table>
	</div>`
}

// TableToJSON exports luaTableToJSON for cross-package use (e.g. the session
// actor serializing a tabulator_ajax_request handler's Lua return value).
func TableToJSON(tbl *lua.LTable) string {
	return luaTableToJSON(tbl)
}

// luaTableToJSON converts a Lua table to a JSON string. Sequential 1..N
// numeric keys produce an array; otherwise an object is emitted. Strings and
// numbers are emitted literally; nested tables are recursed.
func luaTableToJSON(tbl *lua.LTable) string {
	var parts []string
	isArray := true
	expected := 1
	tbl.ForEach(func(k, v lua.LValue) {
		if n, ok := k.(lua.LNumber); ok && int(n) == expected {
			expected++
		} else {
			isArray = false
		}
		parts = append(parts, luaValueJSON(v))
	})

	if isArray {
		return "[" + strings.Join(parts, ",") + "]"
	}

	var objParts []string
	tbl.ForEach(func(k, v lua.LValue) {
		objParts = append(objParts, `"`+jsonEscape(k.String())+`":`+luaValueJSON(v))
	})
	return "{" + strings.Join(objParts, ",") + "}"
}

// luaValueJSON renders a single Lua value as JSON.
func luaValueJSON(v lua.LValue) string {
	switch v.Type() {
	case lua.LTString:
		return `"` + jsonEscape(v.String()) + `"`
	case lua.LTNumber:
		return v.String()
	case lua.LTBool:
		return v.String()
	case lua.LTNil:
		return "null"
	case lua.LTTable:
		return luaTableToJSON(v.(*lua.LTable))
	default:
		return `"` + jsonEscape(v.String()) + `"`
	}
}

// jsonEscape escapes a string for embedding in a JSON double-quoted value.
func jsonEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	s = strings.ReplaceAll(s, "\t", `\t`)
	return s
}
