//go:build !wasm

// DB-linked Tree control (Kalipso "tree" parity, kforms_enhancements.md §9).
//
// A k.ctrl.tree displays a specified field from a database table as an
// expandable tree. The hierarchy is an adjacency list: each row has a unique-ID
// column and a parent-reference column (parent_id). The Go host fetches all
// matching rows in one query, builds the tree server-side, and the client
// expands/collapses locally (full load — no per-node round-trips, good to
// ~5k nodes).
package bindings

import (
	"fmt"

	"github.com/yuin/gopher-lua"
)

// TreeLink is the stored DB configuration of a tree control.
type TreeLink struct {
	HandleID    string
	Table       string
	Where       string
	OrderBy     string
	IDField     string
	ParentField string
	LabelField  string
	MaxNodes    int
}

// TreePage is the full-load result of a tree fetch: columns plus all rows.
type TreePage struct {
	Columns []string
	Rows    []map[string]interface{}
}

// TreeLinkFromControl reads the DB-link configuration stored on a tree control
// (set by addControl). Returns nil, false when the control is not a valid
// DB-linked tree: db + table + label_field are required, and the identifiers
// that reach SQL text (table, id_field, parent_field, label_field) must pass
// isValidIdentifier — the same SQL-injection posture as buildWhereClause.
func TreeLinkFromControl(ctrl *lua.LTable) (*TreeLink, bool) {
	handleID := ""
	if v := ctrl.RawGetString("db"); v != lua.LNil {
		handleID = v.String()
	}
	if handleID == "" {
		return nil, false
	}
	table := ""
	if v := ctrl.RawGetString("table"); v != lua.LNil {
		table = v.String()
	}
	if table == "" {
		return nil, false
	}
	labelField := ""
	if v := ctrl.RawGetString("label_field"); v != lua.LNil {
		labelField = v.String()
	}
	if labelField == "" {
		return nil, false
	}

	link := &TreeLink{
		HandleID:    handleID,
		Table:       table,
		IDField:     "id",
		ParentField: "parent_id",
		LabelField:  labelField,
		MaxNodes:    5000,
	}

	if v := ctrl.RawGetString("id_field"); v != lua.LNil && v.String() != "" {
		link.IDField = v.String()
	}
	if v := ctrl.RawGetString("parent_field"); v != lua.LNil && v.String() != "" {
		link.ParentField = v.String()
	}
	if v := ctrl.RawGetString("where"); v != lua.LNil {
		link.Where = v.String()
	}
	if v := ctrl.RawGetString("order_by"); v != lua.LNil {
		link.OrderBy = v.String()
	}
	if v := ctrl.RawGetString("max_nodes"); v != lua.LNil {
		if n := int(lua.LVAsNumber(v)); n > 0 {
			link.MaxNodes = n
		}
	}

	for _, ident := range []string{link.Table, link.IDField, link.ParentField, link.LabelField} {
		if !isValidIdentifier(ident) {
			return nil, false
		}
	}

	return link, true
}

// FetchTreeRows executes the full-load SELECT against the control's stored DB
// link: SELECT * FROM <table> [WHERE <where>] [ORDER BY <order_by>]. The WHERE
// clause is passed through verbatim (same contract as looper/table where); the
// table and field names are identifier-guarded by TreeLinkFromControl. Fails
// fast when the result exceeds max_nodes so a pathological table cannot cause
// unbounded memory use.
func FetchTreeRows(L *lua.LState, link *TreeLink) (*TreePage, error) {
	handle := getDBHandle(L, link.HandleID)
	if handle == nil {
		return nil, fmt.Errorf("database handle not found: %s", link.HandleID)
	}

	sqlStr := "SELECT * FROM " + link.Table
	if link.Where != "" {
		sqlStr += " WHERE " + link.Where
	}
	if link.OrderBy != "" {
		sqlStr += " ORDER BY " + link.OrderBy
	}

	res, err := executeDBQuery(handle, sqlStr, nil, false, true, false)
	if err != nil {
		return nil, err
	}
	resMap, ok := res.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected query result")
	}
	columns, _ := resMap["columns"].([]string)
	rows, _ := resMap["rows"].([]map[string]interface{})

	if len(rows) > link.MaxNodes {
		return nil, fmt.Errorf("tree exceeds max_nodes (%d rows > %d cap); narrow the WHERE clause or raise max_nodes", len(rows), link.MaxNodes)
	}

	return &TreePage{Columns: columns, Rows: rows}, nil
}

// BuildTree assembles the adjacency tree from flat rows into a Lua table of
// root nodes. Each node carries the full row map plus normalized hierarchy
// keys: id (from id_field), parent_id (from parent_field) and label (from
// label_field), with _children holding the array of child nodes. Orphan
// children (a parent reference that is not a loaded row) are surfaced as
// root-level nodes so no row is lost. Returns the root-nodes table and the
// total node count.
func BuildTree(L *lua.LState, link *TreeLink, rows []map[string]interface{}) (*lua.LTable, int, error) {
	// Index rows by id key (string form so mixed numeric/string id types work).
	byID := make(map[string]int, len(rows))
	for i, row := range rows {
		byID[idKey(row[link.IDField])] = i
	}

	// parent key → child row indices, preserving row order.
	children := make(map[string][]int)
	for i, row := range rows {
		if pk := idKey(row[link.ParentField]); pk != "" {
			children[pk] = append(children[pk], i)
		}
	}

	// Roots: rows whose parent is missing/NULL or not in the loaded id set
	// (orphans are promoted to roots so no row is dropped).
	var roots []int
	for i, row := range rows {
		pk := idKey(row[link.ParentField])
		if pk == "" {
			roots = append(roots, i)
			continue
		}
		if _, ok := byID[pk]; !ok {
			roots = append(roots, i)
		}
	}

	var build func(idx int) *lua.LTable
	build = func(idx int) *lua.LTable {
		row := rows[idx]
		node := L.NewTable()
		for k, v := range row {
			node.RawSetString(k, luaValueFromGo(L, v))
		}
		// Normalized hierarchy keys are always present regardless of the actual
		// column names, so the client and event handlers can rely on
		// node.id / node.parent_id / node.label.
		node.RawSetString("id", luaValueFromGo(L, row[link.IDField]))
		node.RawSetString("parent_id", luaValueFromGo(L, row[link.ParentField]))
		node.RawSetString("label", luaValueFromGo(L, row[link.LabelField]))

		if kids := children[idKey(row[link.IDField])]; len(kids) > 0 {
			kidsTbl := L.NewTable()
			for j, kid := range kids {
				kidsTbl.RawSetInt(j+1, build(kid))
			}
			node.RawSetString("_children", kidsTbl)
		}
		return node
	}

	rootsTbl := L.NewTable()
	for i, idx := range roots {
		rootsTbl.RawSetInt(i+1, build(idx))
	}
	return rootsTbl, len(rows), nil
}

// idKey renders a row value as a stable map key ("" for NULL/missing).
func idKey(v interface{}) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}
