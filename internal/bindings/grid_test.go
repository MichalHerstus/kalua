package bindings

import (
	"strings"
	"testing"

	"github.com/yuin/gopher-lua"
)

// TestRenderGrid verifies a grid control renders the outer .kalua-grid CRUD
// wrapper (data-k-grid-* config) plus the standard inner Tabulator container
// whose id the client keys its tabulatorInstances map on.
func TestRenderGrid(t *testing.T) {
	L := setupTestState(t)
	ctrl := L.NewTable()
	ctrl.RawSetString("type", lua.LString("grid"))
	ctrl.RawSetString("tabulator", lua.LTrue)
	ctrl.RawSetString("db", lua.LString("main"))
	ctrl.RawSetString("query", lua.LString("SELECT * FROM users"))
	ctrl.RawSetString("pk_field", lua.LString("id"))
	ctrl.RawSetString("selection_mode", lua.LString("single"))
	ctrl.RawSetString("row_click_action", lua.LString("edit"))

	cols := L.NewTable()
	col1 := L.NewTable()
	col1.RawSetString("field", lua.LString("id"))
	col1.RawSetString("title", lua.LString("ID"))
	col2 := L.NewTable()
	col2.RawSetString("field", lua.LString("name"))
	col2.RawSetString("title", lua.LString("Name"))
	cols.RawSetInt(1, col1)
	cols.RawSetInt(2, col2)
	ctrl.RawSetString("columns", cols)

	rowActions := L.NewTable()
	rowActions.RawSetString("view", lua.LTrue)
	rowActions.RawSetString("edit", lua.LTrue)
	rowActions.RawSetString("delete", lua.LTrue)
	ctrl.RawSetString("row_actions", rowActions)

	globalActions := L.NewTable()
	globalActions.RawSetString("new_record", lua.LTrue)
	globalActions.RawSetString("batch_delete", lua.LTrue)
	ctrl.RawSetString("global_actions", globalActions)

	html := renderGrid(ctrl, "main", "users", "c:main:users", "")

	for _, want := range []string{
		`class="kalua-grid"`,
		`data-k-grid="1"`,
		`data-k-grid-selection="single"`,
		`data-k-grid-pk="id"`,
		`data-k-grid-row-click="edit"`,
		`data-k-grid-row-actions="{&#34;delete&#34;:true,&#34;edit&#34;:true,&#34;view&#34;:true}"`,
		`data-k-grid-global-actions="{&#34;batch_delete&#34;:true,&#34;new_record&#34;:true}"`,
		`class="kalua-tabulator-table"`,
		`id="c:main:users"`,
		`data-k-tabulator-options="{&#34;layout&#34;:&#34;fitColumns&#34;`,
		`data-k-tabulator-columns="[`,
		`data-k-tabulator-columns="[`,
		`data-k-tabulator-data="[]"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("renderGrid missing %q in:\n%s", want, html)
		}
	}
	// The inner instance id must appear exactly once so the client's
	// tabulatorInstances map (keyed by '#'+id) stays unambiguous and the outer
	// grid wrapper never duplicates it.
	if strings.Count(html, "c:main:users") != 1 {
		t.Errorf("renderGrid id count = %d, want 1", strings.Count(html, "c:main:users"))
	}
}

// TestRenderGridRemotePaging verifies DB-linked grids get remote pagination
// forced like tabulator tables, so the browser installs the Go-backed loader.
func TestRenderGridRemotePaging(t *testing.T) {
	L := setupTestState(t)
	ctrl := L.NewTable()
	ctrl.RawSetString("type", lua.LString("grid"))
	ctrl.RawSetString("tabulator", lua.LTrue)
	ctrl.RawSetString("db", lua.LString("main"))
	ctrl.RawSetString("query", lua.LString("SELECT * FROM items"))
	ctrl.RawSetString("page_size", lua.LNumber(50))

	html := renderGrid(ctrl, "main", "items", "c:main:items", "")
	if !strings.Contains(html, `"paginationMode":"remote"`) && !strings.Contains(html, `&#34;paginationMode&#34;:&#34;remote&#34;`) {
		t.Errorf("renderGrid missing remote pagination: %s", html)
	}
	if !strings.Contains(html, `&#34;paginationSize&#34;:50`) {
		t.Errorf("renderGrid missing forced page size: %s", html)
	}
}

// TestRenderGridFormAttrs covers the inline vs referenced detail/edit form
// configuration.
func TestRenderGridFormAttrs(t *testing.T) {
	L := setupTestState(t)

	form := L.NewTable()
	form.RawSetString("title", lua.LString("Edit user"))
	fields := L.NewTable()
	field1 := L.NewTable()
	field1.RawSetString("type", lua.LString("textbox"))
	field1.RawSetString("name", lua.LString("name"))
	fields.RawSetInt(1, field1)
	form.RawSetString("controls", fields)

	ctrl := L.NewTable()
	ctrl.RawSetString("type", lua.LString("grid"))
	ctrl.RawSetString("tabulator", lua.LTrue)
	ctrl.RawSetString("form", form)

	html := renderGrid(ctrl, "main", "users", "c:main:users", "")
	for _, want := range []string{
		`data-k-grid-form="{`,
		`&#34;title&#34;:&#34;Edit user&#34;`,
		`&#34;type&#34;:&#34;textbox&#34;`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("inline grid form missing %q in:\n%s", want, html)
		}
	}

	ctrl2 := L.NewTable()
	ctrl2.RawSetString("type", lua.LString("grid"))
	ctrl2.RawSetString("tabulator", lua.LTrue)
	ctrl2.RawSetString("form", lua.LString("user_edit"))
	h2 := renderGrid(ctrl2, "main", "users", "c:main:users", "")
	if !strings.Contains(h2, `data-k-grid-formref="user_edit"`) {
		t.Errorf("grid form ref missing: %s", h2)
	}
}

// TestRenderGridDefaultSelection verifies the default selection_mode is "multi".
func TestRenderGridDefaultSelection(t *testing.T) {
	L := setupTestState(t)
	ctrl := L.NewTable()
	ctrl.RawSetString("type", lua.LString("grid"))
	ctrl.RawSetString("tabulator", lua.LTrue)

	html := renderGrid(ctrl, "main", "users", "c:main:users", "")
	if !strings.Contains(html, `data-k-grid-selection="multi"`) {
		t.Errorf("default selection mismatch: %s", html)
	}
}

// TestGridConfigJSON verifies row/global action config serialization: functions
// are dropped (they cannot cross the WebSocket boundary), array forms render as
// JSON arrays, and empty configs yield "".`
func TestGridConfigJSON(t *testing.T) {
	L := setupTestState(t)

	tbl := L.NewTable()
	tbl.RawSetString("edit", lua.LTrue)
	tbl.RawSetString("onclick", L.NewFunction(func(s *lua.LState) int { return 0 }))
	got := gridConfigJSON(tbl)
	if !strings.Contains(got, `"edit":true`) {
		t.Errorf("gridConfigJSON object missing key: %q", got)
	}
	if strings.Contains(got, "onclick") {
		t.Errorf("gridConfigJSON must drop function values: %q", got)
	}

	arr := L.NewTable()
	arr.RawSetInt(1, lua.LString("a"))
	arr.RawSetInt(2, lua.LString("b"))
	gotArr := gridConfigJSON(arr)
	if gotArr != `["a","b"]` {
		t.Errorf("gridConfigJSON array = %q, want [\"a\",\"b\"]", gotArr)
	}

	if empty := gridConfigJSON(L.NewTable()); empty != "" {
		t.Errorf("gridConfigJSON empty = %q, want \"\"", empty)
	}
}

// buildGridForm registers a form global holding a grid control, the way
// addControl does, so the k.grid.* ops can resolve it.
func buildGridForm(L *lua.LState, name string, opts map[string]lua.LString) *lua.LTable {
	form := L.NewTable()
	controls := L.NewTable()
	ctrl := L.NewTable()
	ctrl.RawSetString("type", lua.LString("grid"))
	ctrl.RawSetString("tabulator", lua.LTrue)
	for k, v := range opts {
		ctrl.RawSetString(k, v)
	}
	controls.RawSetString("grid_1", ctrl)
	form.RawSetString("controls", controls)
	L.SetGlobal(name, form)
	return ctrl
}

// TestGridOps verifies k.grid.set_db_source mutates the control's source fields
// and both ops reject non-grid controls.
func TestGridOps(t *testing.T) {
	L := setupFullState(t)

	src := `
k.form.new("main", {})
k.ctrl.grid("main", "grid_1", { db = "main", query = "SELECT * FROM users", pk_field = "id" })
k.ctrl.textbox("main", "not_a_grid", { text = "x" })

local c = main.controls["grid_1"]

k.grid.set_db_source("main", "grid_1", { db = "main", query = "SELECT * FROM archived", page_size = 10 })
assert(c.db == "main", "db unchanged")
assert(c.query == "SELECT * FROM archived", "query not swapped")
assert(c.page_size == 10, "page_size not set")
assert(c.pk_field == "id", "pk_field lost")

k.grid.refresh("main", "grid_1")

local ok, err = pcall(k.grid.refresh, "main", "not_a_grid")
assert(not ok and tostring(err):find("not a grid"), "refresh must reject non-grid: " .. tostring(err))
local ok2, err2 = pcall(k.grid.set_db_source, "main", "not_a_grid", { db = "main" })
assert(not ok2 and tostring(err2):find("not a grid"), "set_db_source must reject non-grid: " .. tostring(err2))
`
	if err := L.DoString(src); err != nil {
		t.Fatalf("run: %v", err)
	}
}