package bindings

import (
	"strings"
	"testing"

	"github.com/yuin/gopher-lua"
)

// setupTestState builds a minimal Env + LState for render tests.
func setupTestState(t *testing.T) *lua.LState {
	t.Helper()
	L := lua.NewState()
	t.Cleanup(L.Close)
	return L
}

func TestLuaTableToJSONArray(t *testing.T) {
	L := setupTestState(t)
	tbl := L.NewTable()
	row1 := L.NewTable()
	row1.RawSetString("name", lua.LString("Alice"))
	row1.RawSetString("age", lua.LNumber(30))
	row2 := L.NewTable()
	row2.RawSetString("name", lua.LString(`Bob"quoted`))
	row2.RawSetString("age", lua.LNumber(25))
	tbl.RawSetInt(1, row1)
	tbl.RawSetInt(2, row2)

	got := luaTableToJSON(tbl)
	if !strings.Contains(got, `"name":"Alice"`) || !strings.Contains(got, `"age":30`) ||
		!strings.Contains(got, `"name":"Bob\"quoted"`) || !strings.Contains(got, `"age":25`) {
		t.Errorf("luaTableToJSON = %s, want objects with name/age fields", got)
	}
	if !strings.HasPrefix(got, "[") || !strings.HasSuffix(got, "]") {
		t.Errorf("luaTableToJSON = %s, want array", got)
	}
}

func TestLuaTableToJSONObject(t *testing.T) {
	L := setupTestState(t)
	tbl := L.NewTable()
	tbl.RawSetString("format", lua.LString("json"))
	tbl.RawSetString("pretty", lua.LTrue)

	got := luaTableToJSON(tbl)
	if !strings.Contains(got, `"format":"json"`) || !strings.Contains(got, `"pretty":true`) {
		t.Errorf("luaTableToJSON object = %s", got)
	}
}

func TestRenderTabulatorTable(t *testing.T) {
	L := setupTestState(t)
	ctrl := L.NewTable()
	ctrl.RawSetString("tabulator", lua.LString("true"))
	data := L.NewTable()
	row := L.NewTable()
	row.RawSetString("name", lua.LString("<script>"))
	row.RawSetString("qty", lua.LNumber(5))
	data.RawSetInt(1, row)
	ctrl.RawSetString("data", data)

	html := renderTable(ctrl, "frm", "tbl", "c:frm:tbl", "", "", "", "", "")
	if !strings.Contains(html, `class="kalua-tabulator-table"`) {
		t.Errorf("renderTable missing tabulator container: %s", html)
	}
	// User data must be HTML-escaped inside the data attribute.
	if strings.Contains(html, "<script>") {
		t.Errorf("renderTable emitted raw user data: %s", html)
	}
	if strings.Contains(html, `&lt;script&gt;`) && !strings.Contains(html, `data-k-tabulator-data`) {
		t.Errorf("renderTable data attribute missing: %s", html)
	}
}

func TestRenderTraditionalTableStillWorks(t *testing.T) {
	L := setupTestState(t)
	ctrl := L.NewTable()
	cols := L.NewTable()
	cols.RawSetInt(1, lua.LString("Name"))
	cols.RawSetInt(2, lua.LString("Age"))
	ctrl.RawSetString("columns", cols)
	rows := L.NewTable()
	r := L.NewTable()
	r.RawSetString("name", lua.LString("Alice"))
	r.RawSetString("age", lua.LString("30"))
	rows.RawSetInt(1, r)
	ctrl.RawSetString("rows", rows)

	html := renderTable(ctrl, "frm", "tbl", "c:frm:tbl", "Table", "", "", "", "")
	if !strings.Contains(html, `<table class="kalua-table"`) {
		t.Errorf("traditional table missing: %s", html)
	}
	if !strings.Contains(html, "Alice") {
		t.Errorf("traditional table missing row data: %s", html)
	}
}
