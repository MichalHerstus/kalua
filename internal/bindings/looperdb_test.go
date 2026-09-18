package bindings

import (
	"strings"
	"testing"

	"github.com/yuin/gopher-lua"
)

// newLooperControl builds a DB-linked looper control table as k.ctrl.looper +
// k.looper.link_db would store it, with links mapping result fields onto
// template controls.
func newLooperControl(L *lua.LState, handleID, query string, columnLinks [][2]string) *lua.LTable {
	ctrl := L.NewTable()
	ctrl.RawSetString("type", lua.LString("looper"))
	ctrl.RawSetString("db", lua.LString(handleID))
	ctrl.RawSetString("query", lua.LString(query))
	ctrl.RawSetString("page_size", lua.LNumber(10))
	links := L.NewTable()
	for i, l := range columnLinks {
		lt := L.NewTable()
		lt.RawSetString("column", lua.LNumber(i+1))
		lt.RawSetString("field", lua.LString(l[0]))
		lt.RawSetString("control", lua.LString(l[1]))
		lt.RawSetString("property", lua.LString("value"))
		links.RawSetInt(i+1, lt)
	}
	ctrl.RawSetString("links", links)
	return ctrl
}

func TestLooperDBLinkFromControl(t *testing.T) {
	L := lua.NewState()
	defer L.Close()
	ctrl := newLooperControl(L, "x", "SELECT id, name, qty, active FROM items", [][2]string{
		{"id", "lb_id"},
		{"name", "lb_name"},
		{"qty", "lb_qty"},
	})
	link, ok := LooperDBLinkFromControl(ctrl)
	if !ok {
		t.Fatal("expected DB link on linked looper control")
	}
	if link.HandleID != "x" {
		t.Fatalf("handle = %q", link.HandleID)
	}
	if link.PageSize != 10 {
		t.Fatalf("page_size = %d", link.PageSize)
	}
	if len(link.Links) != 3 {
		t.Fatalf("len(links) = %d", len(link.Links))
	}
	if link.Links[1].Field != "name" || link.Links[1].Control != "lb_name" {
		t.Fatalf("link[1] = %+v", link.Links[1])
	}

	// A control without a query is not DB-linked.
	plain := L.NewTable()
	plain.RawSetString("type", lua.LString("looper"))
	plain.RawSetString("db", lua.LString("x"))
	if _, ok := LooperDBLinkFromControl(plain); ok {
		t.Fatal("control without query must not be DB-linked")
	}
}

func TestFetchLooperRowsPaging(t *testing.T) {
	L := lua.NewState()
	defer L.Close()
	_ = newSQLiteHandle(t, 30)
	link, _ := LooperDBLinkFromControl(newLooperControl(L, "x", "SELECT id, name, qty, active FROM items", [][2]string{
		{"id", "lb_id"},
		{"name", "lb_name"},
	}))

	res, err := FetchLooperRows(L, link, LooperPageReq{Page: 2, Size: 10})
	if err != nil {
		t.Fatalf("FetchLooperRows: %v", err)
	}
	if res.LastPage != 3 {
		t.Fatalf("last_page = %d, want 3", res.LastPage)
	}
	if len(res.Rows) != 10 {
		t.Fatalf("page 2 rows = %d, want 10", len(res.Rows))
	}
	if got, _ := res.Rows[0]["id"].(int64); got != 11 {
		t.Fatalf("first row id = %v, want 11", res.Rows[0]["id"])
	}
	if got := res.Columns; len(got) != 4 || got[0] != "id" {
		t.Fatalf("columns = %v", got)
	}
}

func TestFetchLooperRowsSortFilter(t *testing.T) {
	L := lua.NewState()
	defer L.Close()
	_ = newSQLiteHandle(t, 30)
	link, _ := LooperDBLinkFromControl(newLooperControl(L, "x", "SELECT id, name, qty, active FROM items", [][2]string{
		{"id", "lb_id"},
		{"name", "lb_name"},
	}))

	// Filter name='item5', whitelisted sort id DESC, paged so the empty tail page
	// proves last_page accounts for filtering.
	res, err := FetchLooperRows(L, link, LooperPageReq{
		Page:   1,
		Size:   10,
		Filter: []FilterSpec{{Field: "name", Op: "=", Value: "item5"}},
		Sort:   []SortSpec{{Field: "id", Dir: "DESC"}},
	})
	if err != nil {
		t.Fatalf("FetchLooperRows: %v", err)
	}
	if res.LastPage != 1 {
		t.Fatalf("filtered last_page = %d, want 1", res.LastPage)
	}
	if len(res.Rows) != 1 {
		t.Fatalf("filtered rows = %+v", res.Rows)
	}
	if got, _ := res.Rows[0]["id"].(int64); got != 5 {
		t.Fatalf("filtered id = %v, want 5", res.Rows[0]["id"])
	}
}

func TestFetchLooperRowsInjectionDropped(t *testing.T) {
	L := lua.NewState()
	defer L.Close()
	_ = newSQLiteHandle(t, 5)
	link, _ := LooperDBLinkFromControl(newLooperControl(L, "x", "SELECT id, name, qty, active FROM items", [][2]string{
		{"id", "lb_id"},
	}))

	// A malicious sort field never reaches SQL; the query still succeeds.
	res, err := FetchLooperRows(L, link, LooperPageReq{
		Page:   1,
		Size:   10,
		Sort:   []SortSpec{{Field: "id; DROP TABLE items", Dir: "ASC"}},
		Filter: []FilterSpec{{Field: "id; DROP TABLE items", Op: "=", Value: "1"}},
	})
	if err != nil {
		t.Fatalf("FetchLooperRows: %v", err)
	}
	if len(res.Rows) == 0 {
		t.Fatal("expected rows after injection attempt dropped")
	}
}

// newRowLooperControl builds a looper control carrying opts.row (row-template
// control defs) and no explicit links table.
func newRowLooperControl(L *lua.LState, handleID, query string) *lua.LTable {
	ctrl := L.NewTable()
	ctrl.RawSetString("type", lua.LString("looper"))
	ctrl.RawSetString("db", lua.LString(handleID))
	ctrl.RawSetString("query", lua.LString(query))
	ctrl.RawSetString("page_size", lua.LNumber(10))
	row := L.NewTable()
	defs := []struct {
		typ  string
		name string
		prop string
		col  string
	}{
		{"label", "lb_id", "text", "id"},
		{"textbox", "tx_name", "value", "name"},
		{"checkbox", "ck_ok", "value", "active"},
		{"image", "img_pic", "src", "pic"},
	}
	for i, d := range defs {
		def := L.NewTable()
		def.RawSetString("type", lua.LString(d.typ))
		def.RawSetString("name", lua.LString(d.name))
		if d.prop != "" {
			def.RawSetString("property", lua.LString(d.prop))
		}
		if d.col != "" {
			def.RawSetString("field", lua.LString(d.col))
		}
		row.RawSetInt(i+1, def)
	}
	ctrl.RawSetString("row", row)
	return ctrl
}

// TestLooperDBLinkDerivesLinksFromRow: a row-template looper with no links table
// derives its DB links from opts.row so the pager maps result columns.
func TestLooperDBLinkDerivesLinksFromRow(t *testing.T) {
	L := lua.NewState()
	defer L.Close()
	link, ok := LooperDBLinkFromControl(newRowLooperControl(L, "x", "SELECT id, name, active, pic FROM items"))
	if !ok {
		t.Fatal("expected DB link on row-template looper")
	}
	if len(link.Links) != 4 {
		t.Fatalf("derived links = %d, want 4", len(link.Links))
	}
	if link.Links[0].Control != "lb_id" || link.Links[0].Field != "id" {
		t.Errorf("link[0] = %+v", link.Links[0])
	}
	if link.Links[1].Control != "tx_name" || link.Links[1].Field != "name" || link.Links[1].Property != "value" {
		t.Errorf("link[1] = %+v", link.Links[1])
	}
	if link.Links[2].Property != "value" {
		t.Errorf("default property = %q, want value", link.Links[2].Property)
	}
}

// TestBuildLooperRowHTML renders a row via reference renderControl: bound values
// are escaped, ids are c:<looper>:<def>:<idx>, and editable controls come back
// as read-only display spans (no inputs).
func TestBuildLooperRowHTML(t *testing.T) {
	L := lua.NewState()
	defer L.Close()
	ctrl := newRowLooperControl(L, "x", "SELECT id, name, active, pic FROM items")
	html := BuildLooperRowHTML(L, "lp", 7, ctrl.RawGetString("row"), map[string]interface{}{
		"id":     int64(7),
		"name":   "<b>&co</b>",
		"active": true,
		"pic":    "/img/a.png",
	}, []string{"id", "name", "active", "pic"})

	for _, want := range []string{
		`class="kalua-looper-row" data-k-looper-index="7"`,
		`id="c:lp:lb_id:7"`,
		`id="c:lp:tx_name:7"`,
		`id="c:lp:ck_ok:7"`,
		`id="c:lp:img_pic:7"`,
		`data-k-looper-control="tx_name"`,
		`<img`,
		`src="/img/a.png"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("row html missing %q:\n%s", want, html)
		}
	}
	// Escaping: the hostile value must appear escaped, never raw.
	if strings.Contains(html, "<b>&co</b>") {
		t.Errorf("value not escaped:\n%s", html)
	}
	if !strings.Contains(html, "&lt;b&gt;&amp;co&lt;/b&gt;") {
		t.Errorf("expected escaped value in html:\n%s", html)
	}
	// Read-only: textbox/checkbox are display spans, no <input.
	if strings.Contains(html, "<input") || strings.Contains(html, "<textarea") {
		t.Errorf("row controls must be display-only, got inputs:\n%s", html)
	}
	if !strings.Contains(html, ">✓<") {
		t.Errorf("checked checkbox should render ✓:\n%s", html)
	}
}

// TestBuildLooperRowHTMLColumnIndex: field-less defs bind by 1-based column.
func TestBuildLooperRowHTMLColumnIndex(t *testing.T) {
	L := lua.NewState()
	defer L.Close()
	row := L.NewTable()
	def := L.NewTable()
	def.RawSetString("type", lua.LString("label"))
	def.RawSetString("name", lua.LString("lb_second"))
	def.RawSetString("column", lua.LNumber(2))
	row.RawSetInt(1, def)
	html := BuildLooperRowHTML(L, "lp", 1, row, map[string]interface{}{
		"id":   int64(1),
		"name": "second",
	}, []string{"id", "name"})
	if !strings.Contains(html, `id="c:lp:lb_second:1"`) || !strings.Contains(html, "second") {
		t.Errorf("column-index binding failed:\n%s", html)
	}
}
