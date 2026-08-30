// Package bindings implements the §5.1/§5.4/§5.5 result-set conversions:
// JSON / CSV / XML ↔ a result set {columns={...}, rows={{col=value,...}}}. The
// result-set shape matches k.db_select / k.rows() so authors can shuttle data
// between formats and the database with one convention.
package bindings

import (
	"fmt"
	"sort"
	"strings"

	"github.com/yuin/gopher-lua"
)

// resultColumns returns the ordered superset of string keys appearing across
// the row tables, sorted for determinism.
func resultColumns(rows []*lua.LTable) []string {
	seen := map[string]bool{}
	for _, r := range rows {
		r.ForEach(func(k, _ lua.LValue) {
			if s, ok := k.(lua.LString); ok {
				seen[string(s)] = true
			}
		})
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// luaRows extracts the array portion of a resultset's "rows" field.
func luaRows(rs *lua.LTable) []*lua.LTable {
	rows, _ := rs.RawGetString("rows").(*lua.LTable)
	if rows == nil {
		return nil
	}
	n := rows.Len()
	out := make([]*lua.LTable, 0, n)
	for i := 1; i <= n; i++ {
		if r, ok := rows.RawGetInt(i).(*lua.LTable); ok {
			out = append(out, r)
		}
	}
	return out
}

// makeResultSet wraps columns + rows into a {columns, rows} table.
func makeResultSet(L *lua.LState, columns []string, rows []*lua.LTable) *lua.LTable {
	rs := L.NewTable()
	colTbl := L.NewTable()
	for i, c := range columns {
		colTbl.RawSetInt(i+1, lua.LString(c))
	}
	rs.RawSetString("columns", colTbl)
	rowTbl := L.NewTable()
	for i, r := range rows {
		rowTbl.RawSetInt(i+1, r)
	}
	rs.RawSetString("rows", rowTbl)
	return rs
}

// registerRows installs the JSON/CSV/XML result-set conversions.
func registerRows(e *Env) {
	// k.json_to_rows(value) -> {columns, rows} from a JSON array of row-maps.
	e.register("json_to_rows", "rows", func(L *lua.LState) int {
		v := L.Get(1)
		arr, ok := v.(*lua.LTable)
		if !ok {
			L.RaiseError("json_to_rows: expected an array table")
			return 0
		}
		rows := make([]*lua.LTable, 0, arr.Len())
		for i := 1; i <= arr.Len(); i++ {
			if r, ok := arr.RawGetInt(i).(*lua.LTable); ok {
				rows = append(rows, r)
			}
		}
		cols := resultColumns(rows)
		L.Push(makeResultSet(L, cols, rows))
		return 1
	})

	// k.rows_to_json(result) -> array of row-map tables (compact via k.json_string).
	e.register("rows_to_json", "rows", func(L *lua.LState) int {
		rs := L.CheckTable(1)
		rows := luaRows(rs)
		out := L.NewTable()
		for i, r := range rows {
			out.RawSetInt(i+1, r)
		}
		L.Push(out)
		return 1
	})

	// k.csv_to_rows(csvTable) -> {columns, rows}. csvTable is the value k.csv_parse
	// returns (list of arrays) and, when the first row is a header, the columns
	// are taken from it; otherwise columns are "col1..colN".
	e.register("csv_to_rows", "rows", func(L *lua.LState) int {
		csvTbl := L.CheckTable(1)
		rows := make([]*lua.LTable, 0, csvTbl.Len())
		for i := 1; i <= csvTbl.Len(); i++ {
			if r, ok := csvTbl.RawGetInt(i).(*lua.LTable); ok {
				rows = append(rows, r)
			}
		}
		var cols []string
		if len(rows) > 0 && firstRowIsHeader(rows[0]) {
			// header row: every cell is a string and no cell has only numeric keys
			cols = headerFromRow(rows[0])
			rows = rows[1:]
		}
		if len(cols) == 0 {
			nc := maxRowWidth(rows)
			for i := 1; i <= nc; i++ {
				cols = append(cols, fmt.Sprintf("col%d", i))
			}
		}
		// rebuild each row slice as a {column=value} map
		mapped := make([]*lua.LTable, 0, len(rows))
		for _, row := range rows {
			rowTbl := L.NewTable()
			width := maxRowWidth([]*lua.LTable{row})
			for i := 0; i < width && i < len(cols); i++ {
				cell := row.RawGetInt(i + 1)
				if cell == lua.LNil {
					cell = lua.LString("")
				}
				rowTbl.RawSetString(cols[i], cell)
			}
			mapped = append(mapped, rowTbl)
		}
		L.Push(makeResultSet(L, cols, mapped))
		return 1
	})

	// k.rows_to_csv(result[, opts]) -> CSV text (same opts as k.csv_string).
	e.register("rows_to_csv", "rows", func(L *lua.LState) int {
		rs := L.CheckTable(1)
		opts := L.OptTable(2, L.NewTable())
		rows := luaRows(rs)
		cols := columnNames(rs)
		if len(cols) == 0 {
			cols = resultColumns(rows)
		}
		text, err := csvString(e, rowsToCSVTable(L, cols, rows), opts)
		if err != nil {
			L.RaiseError("rows_to_csv error: %v", err)
			return 0
		}
		L.Push(lua.LString(text))
		return 1
	})

	// k.xml_to_rows(doc) -> {columns, rows}. doc is a document element table
	// ({_name,_children,_text}); rows are its repeated child elements.
	e.register("xml_to_rows", "rows", func(L *lua.LState) int {
		doc := L.CheckTable(1)
		rows := make([]*lua.LTable, 0)
		children, _ := doc.RawGetString("_children").(*lua.LTable)
		if children != nil {
			for i := 1; i <= children.Len(); i++ {
				if child, ok := children.RawGetInt(i).(*lua.LTable); ok {
					row := L.NewTable()
					childChildren, _ := child.RawGetString("_children").(*lua.LTable)
					if childChildren != nil {
						for j := 1; j <= childChildren.Len(); j++ {
							if cell, ok := childChildren.RawGetInt(j).(*lua.LTable); ok {
								row.RawSetString(cell.RawGetString("_name").String(), lua.LString(cell.RawGetString("_text").String()))
							}
						}
					}
					rows = append(rows, row)
				}
			}
		}
		cols := resultColumns(rows)
		L.Push(makeResultSet(L, cols, rows))
		return 1
	})

	// k.rows_to_xml(result[, rootName]) -> XML text with repeated row elements.
	e.register("rows_to_xml", "rows", func(L *lua.LState) int {
		rs := L.CheckTable(1)
		rows := luaRows(rs)
		cols := columnNames(rs)
		if len(cols) == 0 {
			cols = resultColumns(rows)
		}
		rootName := L.OptString(2, "rows")
		rowName := L.OptString(3, "row")
		var sb strings.Builder
		sb.WriteString("<")
		sb.WriteString(xmlEscape(rootName))
		sb.WriteString(">\n")
		for _, row := range rows {
			sb.WriteString("  <")
			sb.WriteString(xmlEscape(rowName))
			sb.WriteString(">\n")
			for _, c := range cols {
				val := row.RawGetString(c).String()
				sb.WriteString("    <")
				sb.WriteString(xmlEscape(c))
				sb.WriteString(">")
				sb.WriteString(xmlEscape(val))
				sb.WriteString("</")
				sb.WriteString(xmlEscape(c))
				sb.WriteString(">\n")
			}
			sb.WriteString("  </")
			sb.WriteString(xmlEscape(rowName))
			sb.WriteString(">\n")
		}
		sb.WriteString("</")
		sb.WriteString(xmlEscape(rootName))
		sb.WriteString(">\n")
		L.Push(lua.LString(sb.String()))
		return 1
	})
}

// firstRowIsHeader reports whether a parsed-CSV first row should be treated as
// a header (all-string cells, no numeric keys, at least one entry).
func firstRowIsHeader(row *lua.LTable) bool {
	if row.Len() == 0 {
		return false
	}
	hasString := false
	ok := true
	row.ForEach(func(k, v lua.LValue) {
		if _, isNum := k.(lua.LNumber); isNum {
			hasString = true
		} else {
			ok = false
		}
		if v.Type() != lua.LTString {
			ok = false
		}
	})
	return ok && hasString
}

// headerFromRow returns the string cells of the header row.
func headerFromRow(row *lua.LTable) []string {
	n := row.Len()
	out := make([]string, 0, n)
	for i := 1; i <= n; i++ {
		out = append(out, row.RawGetInt(i).String())
	}
	return out
}

// maxRowWidth returns the maximum array length across rows.
func maxRowWidth(rows []*lua.LTable) int {
	w := 0
	for _, r := range rows {
		if r.Len() > w {
			w = r.Len()
		}
	}
	return w
}

// columnNames extracts the "columns" array from a resultset if present.
func columnNames(rs *lua.LTable) []string {
	cols, _ := rs.RawGetString("columns").(*lua.LTable)
	if cols == nil {
		return nil
	}
	n := cols.Len()
	out := make([]string, 0, n)
	for i := 1; i <= n; i++ {
		out = append(out, cols.RawGetInt(i).String())
	}
	return out
}

// rowsToCSVTable reshapes a resultset into the data table k.csv_string expects:
// an array whose first element is the header row (list of column names) and
// the rest are row maps keyed by column name.
func rowsToCSVTable(L *lua.LState, cols []string, rows []*lua.LTable) *lua.LTable {
	out := L.NewTable()
	header := L.NewTable()
	for i, c := range cols {
		header.RawSetInt(i+1, lua.LString(c))
	}
	out.RawSetInt(1, header)
	for i, r := range rows {
		out.RawSetInt(i+2, r)
	}
	return out
}