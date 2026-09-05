// DB-linked Looper tables (Kalipso "connect to database" parity).
//
// A k.ctrl.looper created with link_db is paged server-side by the Go host.
// Every looper_scroll_request page/sort/filter is turned into a safe SELECT
// with bound parameters; sort/filter fields are whitelisted against the query
// result columns so injected identifiers can never reach the SQL text.
package bindings

import (
	"fmt"
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
