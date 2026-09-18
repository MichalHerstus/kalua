// DB-linked Looper tables (Kalipso "connect to database" parity).
//
// A k.ctrl.looper created with link_db is paged server-side by the Go host.
// Every looper_scroll_request page/sort/filter is turned into a safe SELECT
// with bound parameters; sort/filter fields are whitelisted against the query
// result columns so injected identifiers can never reach the SQL text.
package bindings

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/yuin/gopher-lua"
)

// LooperDBLink is the stored DB configuration of a looper control.
type LooperDBLink struct {
	HandleID   string
	Query      string
	Columns    []string // whitelist for sort/filter; empty = auto from query result
	Links      []LooperDBColumnLink
	PageSize   int
	CountQuery string
	Where      map[string]interface{} // base filter {col=value}
	OrderBy    string                 // base ordering, e.g. "id DESC, name ASC"
}

// LooperDBColumnLink maps a result column (by 1-based column index or by field
// name) to a template control and control property.
type LooperDBColumnLink struct {
	Column   int    // 1-based column index (mutually exclusive with Field)
	Field    string // result column name (mutually exclusive with Column)
	Control  string // looper template control name, e.g. "txt_name"
	Property string // control property to set, e.g. "value"
}

// LooperRowDef is one control template inside a looper's opts.row (Kalipso
// row-template controls, kform_builder_plan.md "Advanced Table & Looper
// Editor"). The type/name/opts produce a real control per row; field (or
// 1-based column) binds a result value into the control property.
type LooperRowDef struct {
	Type     string
	Name     string
	Property string // control property receiving the bound value (default "value")
	Field    string // result column name (mutually exclusive with Column)
	Column   int    // 1-based result column index (mutually exclusive with Field)
	Opts     *lua.LTable
}

// LooperPageReq is the parsed remote-pagination request for a looper. It is
// shaped like the table pager request so the whitelist/sort/filter machinery is
// shared.
type LooperPageReq struct {
	Page   int
	Size   int
	Sort   []SortSpec
	Filter []FilterSpec
}

// LooperPageResult is a paged slice of plain rows plus the derived page count.
type LooperPageResult struct {
	Columns  []string
	Rows     []map[string]interface{}
	LastPage int
}

// LooperDBLinkFromControl reads the DB-link configuration stored on a looper
// control (set by addControl or k.looper.set_db_source). Returns nil, false
// when the control is not DB-linked.
func LooperDBLinkFromControl(ctrl *lua.LTable) (*LooperDBLink, bool) {
	handleID := ""
	if v := ctrl.RawGetString("db"); v != lua.LNil {
		handleID = v.String()
	}
	if handleID == "" {
		return nil, false
	}
	query := ""
	if v := ctrl.RawGetString("query"); v != lua.LNil {
		query = v.String()
	}
	if query == "" {
		return nil, false
	}

	link := &LooperDBLink{
		HandleID:   handleID,
		Query:      query,
		PageSize:   50,
		CountQuery: "",
		Where:      map[string]interface{}{},
		OrderBy:    "",
	}

	if v := ctrl.RawGetString("count_query"); v != lua.LNil && v.String() != "" {
		link.CountQuery = v.String()
	}
	if v := ctrl.RawGetString("page_size"); v != lua.LNil {
		if n := int(lua.LVAsNumber(v)); n > 0 {
			link.PageSize = n
		}
	}
	if v := ctrl.RawGetString("db_order_by"); v != lua.LNil {
		link.OrderBy = v.String()
	}
	if v := ctrl.RawGetString("db_where"); v != lua.LNil {
		if whereTbl, ok := v.(*lua.LTable); ok {
			whereTbl.ForEach(func(k, wv lua.LValue) {
				link.Where[k.String()] = luaValueToGo(wv)
			})
		}
	}
	if v := ctrl.RawGetString("links"); v != lua.LNil {
		if linksTbl, ok := v.(*lua.LTable); ok {
			linksTbl.ForEach(func(_, lv lua.LValue) {
				linkTbl, ok := lv.(*lua.LTable)
				if !ok {
					return
				}
				var colIdx int
				var field string
				colControl := ""
				prop := "value"
				linkTbl.ForEach(func(k, cv lua.LValue) {
					switch k.String() {
					case "column", "col":
						colIdx = int(lua.LVAsNumber(cv))
					case "field", "name":
						field = cv.String()
					case "control", "ctrl":
						colControl = cv.String()
					case "property", "prop":
						prop = cv.String()
					}
				})
				if colControl == "" || (colIdx <= 0 && field == "") {
					return
				}
				link.Links = append(link.Links, LooperDBColumnLink{
					Column:   colIdx,
					Field:    field,
					Control:  colControl,
					Property: prop,
				})
			})
		}
	}

	// Row-template loopers (opts.row) carry links derived from their control
	// defs, so the pager maps result columns even when the script never wrote a
	// links table (the builder exports both; this keeps hand-written apps with a
	// row + no links working).
	if len(link.Links) == 0 {
		if rowVal := ctrl.RawGetString("row"); rowVal != lua.LNil {
			for _, d := range ParseLooperRowDefs(rowVal) {
				if d.Field == "" && d.Column < 1 {
					continue // control not bound to a result column
				}
				link.Links = append(link.Links, LooperDBColumnLink{
					Column:   d.Column,
					Field:    d.Field,
					Control:  d.Name,
					Property: d.Property,
				})
			}
		}
	}

	return link, true
}

// FetchLooperRows executes a paged SELECT against the control's stored DB link.
// req is the browser's remote-pagination ask. The returned rows are plain map
// rows (column → value); the caller maps them onto template controls before
// sending looper_db_batch to the browser.
func FetchLooperRows(L *lua.LState, link *LooperDBLink, req LooperPageReq) (*LooperPageResult, error) {
	handle := getDBHandle(L, link.HandleID)
	if handle == nil {
		return nil, fmt.Errorf("database handle not found: %s", link.HandleID)
	}

	// Whitelist for sort/filter fields: the mapped columns, or discovered from
	// a 0-row probe of the base query.
	whitelist := link.Columns
	if len(whitelist) == 0 {
		cols, err := factoryColumns(L, handle, link.Query)
		if err != nil {
			return nil, err
		}
		whitelist = cols
	}

	// Build WHERE: base filter ANDed with browser filters.
	var conds []string
	var params []interface{}
	idx := 1

	for col, val := range link.Where {
		if !inList(whitelist, col) || !isValidIdentifier(col) {
			continue // never let un-whitelisted identifiers into SQL
		}
		conds = append(conds, fmt.Sprintf("%s = %s", col, handle.Placeholder(idx)))
		params = append(params, val)
		idx++
	}
	for _, f := range req.Filter {
		clause, vals, ok := buildFilterClause(handle, whitelist, f, &idx)
		if !ok {
			continue // unknown op or non-whitelisted field: silently drop
		}
		conds = append(conds, clause)
		params = append(params, vals...)
	}

	whereSQL := ""
	if len(conds) > 0 {
		whereSQL = " WHERE " + strings.Join(conds, " AND ")
	}

	// ORDER BY: base order_by first, then browser sort (whitelisted fields only).
	var orders []string
	if link.OrderBy != "" {
		for _, part := range strings.Split(link.OrderBy, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			fields := strings.Fields(part)
			if len(fields) == 0 || !isValidIdentifier(fields[0]) {
				continue
			}
			if len(fields) > 1 {
				dir := strings.ToUpper(fields[1])
				if dir != "ASC" && dir != "DESC" {
					dir = "ASC"
				}
				orders = append(orders, fields[0]+" "+dir)
			} else {
				orders = append(orders, fields[0])
			}
		}
	}
	for _, s := range req.Sort {
		if !inList(whitelist, s.Field) || !isValidIdentifier(s.Field) {
			continue
		}
		dir := strings.ToUpper(s.Dir)
		if dir != "ASC" && dir != "DESC" {
			dir = "ASC"
		}
		orders = append(orders, s.Field+" "+dir)
	}
	orderSQL := ""
	if len(orders) > 0 {
		orderSQL = " ORDER BY " + strings.Join(orders, ", ")
	}

	// Count for last_page: explicit count_query is author-trusted; the derived
	// count re-applies the base+filter WHERE so last_page matches the filtered
	// page count rather than the unfiltered total.
	var total int
	var err error
	if link.CountQuery != "" {
		total, err = runCount(L, handle, link.CountQuery)
	} else {
		countSQL := "SELECT COUNT(*) FROM (" + link.Query + ") ENV_PAGE" + whereSQL
		total, err = runCountParams(L, handle, countSQL, params)
	}
	if err != nil {
		return nil, err
	}

	size := req.Size
	if size <= 0 {
		size = link.PageSize
	}
	if size <= 0 {
		size = 50
	}
	page := req.Page
	if page < 1 {
		page = 1
	}
	lastPage := 1
	if size > 0 {
		lastPage = (total + size - 1) / size
	}
	if lastPage < 1 {
		lastPage = 1
	}

	// Paged query.
	proj := "SELECT * FROM (" + link.Query + ") ENV_PAGE"
	offset := (page - 1) * size
	pageSQL := proj + whereSQL + orderSQL + handle.LimitClause(size, offset)

	sqlResult, err := executeDBQuery(handle, pageSQL, params, false, true, false)
	if err != nil {
		return nil, err
	}
	resMap, ok := sqlResult.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected query result")
	}
	columns, _ := resMap["columns"].([]string)
	rows, _ := resMap["rows"].([]map[string]interface{})

	return &LooperPageResult{Columns: columns, Rows: rows, LastPage: lastPage}, nil
}

// ParseLooperRowDefs reads a looper's opts.row (a Lua array of
// {type,name,property,field,column,opts} tables) into Go structs. Non-table
// entries and defs without a type/name are skipped.
func ParseLooperRowDefs(rowVal lua.LValue) []LooperRowDef {
	var defs []LooperRowDef
	if rowVal == lua.LNil {
		return defs
	}
	rowTbl, ok := rowVal.(*lua.LTable)
	if !ok {
		return defs
	}
	rowTbl.ForEach(func(_, v lua.LValue) {
		defTbl, ok := v.(*lua.LTable)
		if !ok {
			return
		}
		d := LooperRowDef{}
		defTbl.ForEach(func(k, cv lua.LValue) {
			switch k.String() {
			case "type", "ctrl_type":
				d.Type = cv.String()
			case "name", "control":
				d.Name = cv.String()
			case "property", "prop":
				d.Property = cv.String()
			case "field", "column_name":
				d.Field = cv.String()
			case "column", "col", "column_index":
				d.Column = int(lua.LVAsNumber(cv))
			case "opts", "options":
				if optTbl, ok := cv.(*lua.LTable); ok {
					d.Opts = optTbl
				}
			}
		})
		if d.Type != "" && d.Name != "" {
			defs = append(defs, d)
		}
	})
	return defs
}

// BuildLooperRowHTML renders one DB-linked looper row from opts.row template
// defs (Kalipso row-template controls). Each def becomes a real control with a
// synthetic form/name (ids c:<looper>:<def>:<idx>), its bound property set from
// the fetched row, and rendered read-only via renderControl's looper_display
// path. The row is wrapped in a .kalua-looper-row with one .kalua-looper-cell
// per control so it lays out like the legacy value-cell rows.
func BuildLooperRowHTML(L *lua.LState, looperName string, idx int, rowVal lua.LValue, rowData map[string]interface{}, columns []string) string {
	defs := ParseLooperRowDefs(rowVal)
	if len(defs) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<div class="kalua-looper-row" data-k-looper-index="` + strconv.Itoa(idx) + `">`)
	for _, def := range defs {
		ctrl := L.NewTable()
		ctrl.RawSetString("type", lua.LString(def.Type))
		ctrl.RawSetString("form", lua.LString(looperName))
		ctrl.RawSetString("name", lua.LString(def.Name+":"+strconv.Itoa(idx)))
		ctrl.RawSetString("looper_display", lua.LString("true"))
		if def.Opts != nil {
			def.Opts.ForEach(func(k, v lua.LValue) {
				ctrl.RawSet(k, v)
			})
		}

		var val interface{}
		if def.Field != "" {
			val = rowData[def.Field]
		} else if def.Column >= 1 && def.Column <= len(columns) {
			val = rowData[columns[def.Column-1]]
		}
		prop := def.Property
		if prop == "" {
			prop = "value"
		}
		ctrl.RawSetString(prop, luaValueFromGo(L, val))
		if def.Type == "label" && prop != "label" {
			ctrl.RawSetString("label", luaValueFromGo(L, val))
		}

		b.WriteString(`<div class="kalua-looper-cell" data-k-looper-control="` + escAttr(def.Name) + `">`)
		b.WriteString(renderControl(ctrl))
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

// luaValueFromGo converts a database row value into a Lua value for rendering.
func luaValueFromGo(L *lua.LState, v interface{}) lua.LValue {
	switch t := v.(type) {
	case nil:
		return lua.LNil
	case string:
		return lua.LString(t)
	case bool:
		return lua.LBool(t)
	case float64:
		return lua.LNumber(t)
	case float32:
		return lua.LNumber(float64(t))
	case int:
		return lua.LNumber(float64(t))
	case int64:
		return lua.LNumber(float64(t))
	case []byte:
		return lua.LString(string(t))
	default:
		return lua.LString(fmt.Sprintf("%v", t))
	}
}
