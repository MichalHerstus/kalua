// DB-linked Tabulator tables (Kalipso "connect to database" parity).
//
// A k.ctrl.table created with tabulator=true plus a db handle and SELECT query
// is paged server-side by the Go host. Every tabulator_ajax_request page/sort/
// filter is turned into a safe SELECT with bound parameters; sort/filter fields
// are whitelisted against the table's mapped columns so injected identifiers
// can never reach the SQL text.
package bindings

import (
	"fmt"
	"strings"

	"github.com/yuin/gopher-lua"
)

// TableLink is the stored DB configuration of a tabulator table control.
type TableLink struct {
	HandleID   string
	Query      string
	Columns    []string // mapped column fields (whitelist); empty = auto from result
	PageSize   int
	CountQuery string
	Where      map[string]interface{} // base filter {col=value}
	OrderBy    string                 // base ordering, e.g. "id DESC, name ASC"
}

// SortSpec is one ORDER BY clause from the browser.
type SortSpec struct {
	Field, Dir string
}

// FilterSpec is one WHERE condition from the browser.
type FilterSpec struct {
	Field, Op, Value string
}

// TablePageReq is the parsed remote-pagination request.
type TablePageReq struct {
	Page   int
	Size   int
	Sort   []SortSpec
	Filter []FilterSpec
	// Raw holds the unfiltered filter list (for callers that want it).
	Raw []byte
}

// TablePageResult is a paged slice plus the derived page count.
type TablePageResult struct {
	Columns  []string
	Rows     []map[string]interface{}
	LastPage int
}

// TableLinkFromControl exports tableLinkFromControl for cross-package use
// (the session actor needs to detect + build a DB-linked table's pager).
func TableLinkFromControl(L *lua.LState, ctrl *lua.LTable) (*TableLink, bool) {
	return tableLinkFromControl(L, ctrl)
}

// tableLinkFromControl reads the DB-link configuration stored on a table
// control (set by addControl). Returns nil, false when the control is not a
// DB-linked tabulator table.
func tableLinkFromControl(L *lua.LState, ctrl *lua.LTable) (*TableLink, bool) {
	if !isTabulator(ctrl) {
		return nil, false
	}
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

	link := &TableLink{
		HandleID:   handleID,
		Query:      query,
		PageSize:   25,
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
	if v := ctrl.RawGetString("db_columns"); v != lua.LNil {
		if colsTbl, ok := v.(*lua.LTable); ok {
			colsTbl.ForEach(func(_ lua.LValue, cv lua.LValue) {
				if s := cv.String(); s != "" {
					link.Columns = append(link.Columns, s)
				}
			})
		}
	}
	if v := ctrl.RawGetString("db_where"); v != lua.LNil {
		if whereTbl, ok := v.(*lua.LTable); ok {
			whereTbl.ForEach(func(k, wv lua.LValue) {
				link.Where[k.String()] = luaValueToGo(wv)
			})
		}
	}

	return link, true
}

// FetchTablePage executes a paged SELECT against the control's stored DB link.
// req is the browser's remote-pagination ask. The returned rows are plain map
// rows (column → value); the caller serializes them for the WebSocket message.
func FetchTablePage(L *lua.LState, link *TableLink, req TablePageReq) (*TablePageResult, error) {
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

	// Count query for last_page.
	// - Explicit count_query: author-trusted, executed as-is.
	// - Derived: COUNT(*) over the base query WITH the same base+filter WHERE,
	//   so last_page tracks the filtered page count (not the unfiltered total).
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
		size = 25
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

	return &TablePageResult{Columns: columns, Rows: rows, LastPage: lastPage}, nil
}

// factoryColumns discovers the result columns of a base query via a 0-row probe.
func factoryColumns(L *lua.LState, handle *DBHandle, query string) ([]string, error) {
	probe := "SELECT * FROM (" + query + ") ENV_PAGE LIMIT 0"
	res, err := executeDBQuery(handle, probe, nil, false, true, false)
	if err != nil {
		// Some drivers reject LIMIT 0 in a derived table; fall back to a
		// 1-row probe and ignore the row itself.
		res, err = executeDBQuery(handle, "SELECT * FROM ("+query+") ENV_PAGE LIMIT 1", nil, false, true, false)
		if err != nil {
			return nil, err
		}
	}
	resMap, ok := res.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("column probe failed")
	}
	columns, _ := resMap["columns"].([]string)
	if len(columns) == 0 {
		return nil, fmt.Errorf("no columns discovered from query")
	}
	return columns, nil
}

// runCount executes a COUNT query with no bound parameters.
func runCount(L *lua.LState, handle *DBHandle, countSQL string) (int, error) {
	return runCountParams(L, handle, countSQL, nil)
}

// runCountParams executes a COUNT query with bound parameters and returns the
// total row count (handles int64/float64/[]byte scan results).
func runCountParams(L *lua.LState, handle *DBHandle, countSQL string, params []interface{}) (int, error) {
	res, err := executeDBQuery(handle, countSQL, params, false, true, false)
	if err != nil {
		return 0, err
	}
	resMap, ok := res.(map[string]interface{})
	if !ok || resMap["rows"] == nil {
		return 0, nil
	}
	rows, ok := resMap["rows"].([]map[string]interface{})
	if !ok || len(rows) == 0 {
		return 0, nil
	}
	for _, v := range rows[0] {
		switch n := v.(type) {
		case int64:
			return int(n), nil
		case float64:
			return int(n), nil
		case []byte:
			var out int
			fmt.Sscanf(string(n), "%d", &out)
			return out, nil
		}
	}
	return 0, nil
}

// buildFilterClause maps a browser filter to a safe SQL fragment. Returns
// ok=false when the field isn't whitelisted or the operator is unsupported.
func buildFilterClause(handle *DBHandle, whitelist []string, f FilterSpec, idx *int) (string, []interface{}, bool) {
	if !inList(whitelist, f.Field) || !isValidIdentifier(f.Field) {
		return "", nil, false
	}
	op := strings.ToLower(strings.TrimSpace(f.Op))

	switch op {
	case "=", "equals":
		return fmt.Sprintf("%s = %s", f.Field, handle.Placeholder(*idx)), []interface{}{f.Value}, consume(idx)
	case "!=", "<>", "notequals":
		return fmt.Sprintf("%s <> %s", f.Field, handle.Placeholder(*idx)), []interface{}{f.Value}, consume(idx)
	case "<", "less":
		return fmt.Sprintf("%s < %s", f.Field, handle.Placeholder(*idx)), []interface{}{f.Value}, consume(idx)
	case "<=", "lessequal", "lte":
		return fmt.Sprintf("%s <= %s", f.Field, handle.Placeholder(*idx)), []interface{}{f.Value}, consume(idx)
	case ">", "greater":
		return fmt.Sprintf("%s > %s", f.Field, handle.Placeholder(*idx)), []interface{}{f.Value}, consume(idx)
	case ">=", "greaterequal", "gte":
		return fmt.Sprintf("%s >= %s", f.Field, handle.Placeholder(*idx)), []interface{}{f.Value}, consume(idx)
	case "like", "contains":
		return fmt.Sprintf("%s LIKE %s", f.Field, handle.Placeholder(*idx)), []interface{}{"%" + f.Value + "%"}, consume(idx)
	case "starts":
		return fmt.Sprintf("%s LIKE %s", f.Field, handle.Placeholder(*idx)), []interface{}{f.Value + "%"}, consume(idx)
	case "ends":
		return fmt.Sprintf("%s LIKE %s", f.Field, handle.Placeholder(*idx)), []interface{}{"%" + f.Value}, consume(idx)
	case "in":
		parts := strings.Split(f.Value, ",")
		var cleanParts []string
		for _, p := range parts {
			if s := strings.TrimSpace(p); s != "" {
				cleanParts = append(cleanParts, s)
			}
		}
		if len(cleanParts) == 0 {
			return "1 = 0", nil, true // empty IN → no rows
		}
		var ph []string
		var vals []interface{}
		for _, item := range cleanParts {
			ph = append(ph, handle.Placeholder(*idx))
			*idx++
			vals = append(vals, item)
		}
		return fmt.Sprintf("%s IN (%s)", f.Field, strings.Join(ph, ", ")), vals, true
	default:
		return "", nil, false
	}
}

func consume(idx *int) bool {
	*idx++
	return true
}

func inList(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// LimitClause renders the driver-specific paging fragment.
func (h *DBHandle) LimitClause(size, offset int) string {
	switch h.driver {
	case "sqlserver":
		return fmt.Sprintf(" OFFSET %d ROWS FETCH NEXT %d ROWS ONLY", offset, size)
	default:
		return fmt.Sprintf(" LIMIT %d OFFSET %d", size, offset)
	}
}
