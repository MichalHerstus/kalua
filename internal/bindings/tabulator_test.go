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

// TestRenderTabulatorOptionsJSON pins that tabulatorOptions and explicit
// columns (Lua tables) are serialized to real JSON, not "table: 0x...".
func TestRenderTabulatorOptionsJSON(t *testing.T) {
	L := setupTestState(t)
	ctrl := L.NewTable()
	ctrl.RawSetString("tabulator", lua.LString("true"))
	opts := L.NewTable()
	opts.RawSetString("paginationMode", lua.LString("remote"))
	opts.RawSetString("paginationSize", lua.LNumber(10))
	ctrl.RawSetString("tabulatorOptions", opts)
	cols := L.NewTable()
	col := L.NewTable()
	col.RawSetString("field", lua.LString("name"))
	col.RawSetString("title", lua.LString("Name"))
	col.RawSetString("sorter", lua.LString("string"))
	cols.RawSetInt(1, col)
	ctrl.RawSetString("columns", cols)

	html := renderTable(ctrl, "frm", "tbl", "c:frm:tbl", "", "", "", "", "")
	if strings.Contains(html, "table: 0x") {
		t.Fatalf("renderTable emitted a raw Lua table string: %s", html)
	}
	if !strings.Contains(html, "paginationMode") || !strings.Contains(html, "remote") {
		t.Errorf("tabulator options JSON missing paginationMode: %s", html)
	}
	if !strings.Contains(html, "&#34;field&#34;:&#34;name&#34;") || !strings.Contains(html, "&#34;title&#34;:&#34;Name&#34;") {
		t.Errorf("columns JSON missing field/title: %s", html)
	}
}

// TestRemotePageFromLua pins the k.table.set_remote_data payload conversion.
func TestRemotePageFromLua(t *testing.T) {
	L := setupTestState(t)
	tbl := L.NewTable()
	data := L.NewTable()
	r := L.NewTable()
	r.RawSetString("id", lua.LNumber(1))
	data.RawSetInt(1, r)
	tbl.RawSetString("data", data)
	tbl.RawSetString("last_page", lua.LNumber(2))

	p := remotePageFromLua(tbl)
	if p.LastPage != 2 {
		t.Errorf("last_page = %d, want 2", p.LastPage)
	}
	out := remotePayloadJSON(p)
	if strings.Contains(out, "table: 0x") {
		t.Errorf("remotePayloadJSON emitted raw Lua string: %s", out)
	}
	if !strings.Contains(out, `"last_page":2`) || !strings.Contains(out, `"id":1`) {
		t.Errorf("remotePayloadJSON = %s, want data + last_page", out)
	}
}
