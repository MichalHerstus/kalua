package bindings

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yuin/gopher-lua"

	_ "modernc.org/sqlite"
)

// newTreeControl builds a DB-linked tree control table as k.ctrl.tree would
// store it (via addControl).
func newTreeControl(L *lua.LState, handleID string, opts map[string]string) *lua.LTable {
	ctrl := L.NewTable()
	ctrl.RawSetString("type", lua.LString("tree"))
	ctrl.RawSetString("form", lua.LString("main"))
	ctrl.RawSetString("name", lua.LString("tree"))
	ctrl.RawSetString("db", lua.LString(handleID))
	for k, v := range opts {
		ctrl.RawSetString(k, lua.LString(v))
	}
	return ctrl
}

// newCategoriesHandle creates a fresh sqlite DBHandle registered under "x" with
// an adjacency-list categories table (id, parent_id, name).
func newCategoriesHandle(t *testing.T) *DBHandle {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "tree.db"))
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE categories (
		id INTEGER PRIMARY KEY, parent_id INTEGER, name TEXT, sort INTEGER)`); err != nil {
		t.Fatalf("create: %v", err)
	}
	rows := [][4]interface{}{
		{1, nil, "Electronics", 1},
		{2, 1, "Laptops", 1},
		{3, 1, "Phones", 2},
		{4, 2, "ThinkPad", 1},
		{5, nil, "Books", 2},
		{6, 99, "Orphan", 1}, // parent 99 not loaded → promoted to root
	}
	for _, r := range rows {
		if _, err := db.Exec("INSERT INTO categories (id, parent_id, name, sort) VALUES (?, ?, ?, ?)",
			r[0], r[1], r[2], r[3]); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	h := &DBHandle{db: db, driver: "sqlite"}
	dbHandlesMu.Lock()
	dbHandles["x"] = h
	dbHandlesMu.Unlock()
	t.Cleanup(func() {
		dbHandlesMu.Lock()
		delete(dbHandles, "x")
		dbHandlesMu.Unlock()
		db.Close()
	})
	return h
}

func TestTreeLinkDefaults(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	// db + table + label_field required.
	if _, ok := TreeLinkFromControl(newTreeControl(L, "x", map[string]string{})); ok {
		t.Error("control without table/label_field must not link")
	}
	if _, ok := TreeLinkFromControl(newTreeControl(L, "x", map[string]string{"table": "categories"})); ok {
		t.Error("control without label_field must not link")
	}
	if _, ok := TreeLinkFromControl(newTreeControl(L, "x", map[string]string{"table": "categories", "label_field": "name"})); !ok {
		t.Error("valid control must link")
	}
	if _, ok := TreeLinkFromControl(newTreeControl(L, "", map[string]string{"table": "categories", "label_field": "name"})); ok {
		t.Error("control without db must not link")
	}

	// Defaults: id/parent_id, max_nodes 5000.
	link, ok := TreeLinkFromControl(newTreeControl(L, "x", map[string]string{"table": "categories", "label_field": "name"}))
	if !ok {
		t.Fatal("valid control must link")
	}
	if link.IDField != "id" || link.ParentField != "parent_id" {
		t.Errorf("defaults = id=%q parent=%q, want id/parent_id", link.IDField, link.ParentField)
	}
	if link.MaxNodes != 5000 {
		t.Errorf("default max_nodes = %d, want 5000", link.MaxNodes)
	}

	// Overrides.
	link, ok = TreeLinkFromControl(newTreeControl(L, "x", map[string]string{
		"table": "categories", "label_field": "name",
		"id_field": "cat_id", "parent_field": "parent", "max_nodes": "100",
		"where": "sort > 0", "order_by": "name ASC",
	}))
	if !ok {
		t.Fatal("overridden control must link")
	}
	if link.IDField != "cat_id" || link.ParentField != "parent" || link.MaxNodes != 100 {
		t.Errorf("overrides = %+v", link)
	}
	if link.Where != "sort > 0" || link.OrderBy != "name ASC" {
		t.Errorf("where/order_by = %q / %q", link.Where, link.OrderBy)
	}

	// Non-identifier fields are rejected (SQL-injection posture).
	for _, bad := range []map[string]string{
		{"table": "categories; DROP TABLE x", "label_field": "name"},
		{"table": "categories", "label_field": "name; DROP TABLE x"},
		{"table": "categories", "label_field": "name", "id_field": "id; DROP TABLE x"},
		{"table": "categories", "label_field": "name", "parent_field": "parent_id; DROP TABLE x"},
	} {
		if _, ok := TreeLinkFromControl(newTreeControl(L, "x", bad)); ok {
			t.Errorf("control with non-identifier field must not link: %+v", bad)
		}
	}
}

func TestFetchTreeRows(t *testing.T) {
	L := lua.NewState()
	defer L.Close()
	_ = newCategoriesHandle(t)

	link, _ := TreeLinkFromControl(newTreeControl(L, "x", map[string]string{
		"table": "categories", "label_field": "name",
	}))
	page, err := FetchTreeRows(L, link)
	if err != nil {
		t.Fatalf("FetchTreeRows: %v", err)
	}
	if len(page.Rows) != 6 {
		t.Fatalf("rows = %d, want 6", len(page.Rows))
	}
	if len(page.Columns) != 4 {
		t.Fatalf("columns = %v, want 4", page.Columns)
	}

	// WHERE + ORDER BY honored.
	link, _ = TreeLinkFromControl(newTreeControl(L, "x", map[string]string{
		"table": "categories", "label_field": "name",
		"where": "sort = 1", "order_by": "name ASC",
	}))
	page, err = FetchTreeRows(L, link)
	if err != nil {
		t.Fatalf("FetchTreeRows(where/order): %v", err)
	}
	if len(page.Rows) != 4 {
		t.Fatalf("filtered rows = %d, want 4", len(page.Rows))
	}
	if page.Rows[0]["name"] != "Electronics" {
		t.Errorf("first ordered row = %v, want Electronics", page.Rows[0]["name"])
	}

	// Unknown handle errors.
	link.HandleID = "nope"
	if _, err := FetchTreeRows(L, link); err == nil {
		t.Error("FetchTreeRows(unknown handle) succeeded, want error")
	}
}

func TestFetchTreeRowsMaxNodes(t *testing.T) {
	L := lua.NewState()
	defer L.Close()
	_ = newCategoriesHandle(t)

	link, _ := TreeLinkFromControl(newTreeControl(L, "x", map[string]string{
		"table": "categories", "label_field": "name", "max_nodes": "3",
	}))
	if _, err := FetchTreeRows(L, link); err == nil || !strings.Contains(err.Error(), "max_nodes") {
		t.Errorf("FetchTreeRows over cap err = %v, want max_nodes error", err)
	}
}

func TestBuildTree(t *testing.T) {
	L := lua.NewState()
	defer L.Close()
	_ = newCategoriesHandle(t)

	link, _ := TreeLinkFromControl(newTreeControl(L, "x", map[string]string{
		"table": "categories", "label_field": "name",
	}))
	page, err := FetchTreeRows(L, link)
	if err != nil {
		t.Fatalf("FetchTreeRows: %v", err)
	}
	roots, count, err := BuildTree(L, link, page.Rows)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}
	if count != 6 {
		t.Errorf("count = %d, want 6", count)
	}

	// Roots: Electronics, Books, and the orphan (parent 99 not loaded).
	if roots.Len() != 3 {
		t.Fatalf("root count = %d, want 3 (Electronics, Books, Orphan)", roots.Len())
	}
	root1, _ := roots.RawGetInt(1).(*lua.LTable)
	if root1 == nil || root1.RawGetString("label").String() != "Electronics" {
		t.Fatalf("root[1] = %+v, want Electronics", root1)
	}
	// Electronics has two children: Laptops, Phones.
	kids, _ := root1.RawGetString("_children").(*lua.LTable)
	if kids == nil || kids.Len() != 2 {
		t.Fatalf("Electronics children = %v, want 2", kids)
	}
	kid1, _ := kids.RawGetInt(1).(*lua.LTable)
	if kid1 == nil || kid1.RawGetString("label").String() != "Laptops" {
		t.Fatalf("child[1] = %+v, want Laptops", kid1)
	}
	// Laptops has one child: ThinkPad.
	grandkids, _ := kid1.RawGetString("_children").(*lua.LTable)
	if grandkids == nil || grandkids.Len() != 1 {
		t.Fatalf("Laptops children = %v, want 1", grandkids)
	}
	gk, _ := grandkids.RawGetInt(1).(*lua.LTable)
	if gk == nil || gk.RawGetString("label").String() != "ThinkPad" {
		t.Fatalf("grandchild = %+v, want ThinkPad", gk)
	}

	// Normalized keys: id/parent_id/label always present; extra columns kept.
	if root1.RawGetString("id").String() != "1" || root1.RawGetString("parent_id") != lua.LNil {
		t.Errorf("root id/parent_id = %v / %v", root1.RawGetString("id"), root1.RawGetString("parent_id"))
	}
	if root1.RawGetString("sort").String() != "1" {
		t.Errorf("extra column sort = %v, want 1", root1.RawGetString("sort"))
	}

	// Orphan (id 6, parent 99) is a root with no children.
	root3, _ := roots.RawGetInt(3).(*lua.LTable)
	if root3 == nil || root3.RawGetString("label").String() != "Orphan" {
		t.Fatalf("root[3] = %+v, want Orphan", root3)
	}
	if root3.RawGetString("_children") != lua.LNil {
		t.Errorf("orphan must have no children")
	}
}

func TestRenderTree(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	ctrl := newTreeControl(L, "x", map[string]string{
		"table": "categories", "label_field": "name",
	})
	html := renderControl(ctrl)
	for _, want := range []string{
		`class="kalua-control kalua-tree-control"`,
		`class="kalua-tree"`,
		`id="c:main:tree"`,
		`data-k-form="main"`,
		`data-k-ctrl="tree"`,
		`data-k-tree-expand-all="false"`,
		`data-k-tree-selection="single"`,
		`data-k-tree-error=""`,
		`class="kalua-tree-empty"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("render missing %q:\n%s", want, html)
		}
	}

	// expand_all + selection overrides.
	ctrl2 := newTreeControl(L, "x", map[string]string{
		"table": "categories", "label_field": "name",
		"expand_all": "true", "selection": "none",
	})
	html2 := renderControl(ctrl2)
	if !strings.Contains(html2, `data-k-tree-expand-all="true"`) {
		t.Errorf("expand_all not rendered:\n%s", html2)
	}
	if !strings.Contains(html2, `data-k-tree-selection="none"`) {
		t.Errorf("selection not rendered:\n%s", html2)
	}
}
