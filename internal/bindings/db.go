// Package bindings implements the database bindings.
package bindings

import (
	"database/sql"
	"fmt"
	"sync"

	"github.com/yuin/gopher-lua"

	_ "modernc.org/sqlite"

	"kalua/internal/common"
)

// DBHandle wraps a database connection for use in Lua
type DBHandle struct {
	db     *sql.DB
	mu     sync.Mutex
	tx     *sql.Tx
	inTx   bool
	driver string
}

// dbHandles stores database handles by ID (Go-side only)
var dbHandles = make(map[string]*DBHandle)
var dbHandlesMu sync.Mutex

// registerDB installs k.db.* bindings
func registerDB(e *Env) {
	// k.connect_db(dsn) - connect to database
	e.register("connect_db", "database", func(L *lua.LState) int {
		dsn := L.CheckString(1)

		// Parse driver from DSN
		driver, cleanDSN := parseDSN(dsn)
		if driver == "" {
			L.RaiseError("unsupported database driver in DSN: %s", dsn)
			return 0
		}

		db, err := sql.Open(driver, cleanDSN)
		if err != nil {
			L.RaiseError("failed to connect to database: %v", err)
			return 0
		}

		// Test connection
		if err := db.Ping(); err != nil {
			db.Close()
			L.RaiseError("failed to ping database: %v", err)
			return 0
		}

		handle := &DBHandle{
			db:     db,
			driver: driver,
		}

		// Store handle in Go map
		id := fmt.Sprintf("db_%p", handle)
		dbHandlesMu.Lock()
		dbHandles[id] = handle
		dbHandlesMu.Unlock()

		L.Push(lua.LString(id))
		return 1
	})

	// k.disconnect_db([handle]) - disconnect from database
	e.register("disconnect_db", "database", func(L *lua.LState) int {
		handleID := L.OptString(1, "")
		if handleID == "" {
			// Disconnect all
			dbHandlesMu.Lock()
			for _, h := range dbHandles {
				h.Close()
			}
			dbHandles = make(map[string]*DBHandle)
			dbHandlesMu.Unlock()
			return 0
		}

		dbHandlesMu.Lock()
		h, ok := dbHandles[handleID]
		if ok {
			delete(dbHandles, handleID)
		}
		dbHandlesMu.Unlock()

		if !ok {
			L.RaiseError("database handle not found: %s", handleID)
			return 0
		}
		h.Close()
		return 0
	})

	// k.sql(handle, sql, params...) - execute arbitrary SQL
	e.register("sql", "database", func(L *lua.LState) int {
		handleID := L.CheckString(1)
		sqlStr := L.CheckString(2)

		handle := getDBHandle(L, handleID)
		if handle == nil {
			L.RaiseError("database handle not found: %s", handleID)
			return 0
		}

		// Collect parameters
		var params []interface{}
		for i := 3; i <= L.GetTop(); i++ {
			params = append(params, luaValueToGo(L.Get(i)))
		}

		return executeDBAsync(e, L, handle, sqlStr, params, true, false, false)
	})

	// k.db_select(table, fields, where, order) - select query builder
	e.register("db_select", "database", func(L *lua.LState) int {
		handleID := L.CheckString(1)
		table := L.CheckString(2)
		fields := L.OptTable(3, L.NewTable())
		where := L.OptTable(4, L.NewTable())
		order := L.OptString(5, "")

		handle := getDBHandle(L, handleID)
		if handle == nil {
			L.RaiseError("database handle not found: %s", handleID)
			return 0
		}

		// Build field list
		var fieldList []string
		fields.ForEach(func(k, v lua.LValue) {
			fieldList = append(fieldList, v.String())
		})
		if len(fieldList) == 0 {
			fieldList = []string{"*"}
		}

		// Build WHERE clause
		whereClause, whereParams := buildWhereClause(L, where)

		// Build ORDER BY
		orderClause := ""
		if order != "" {
			orderClause = " ORDER BY " + order
		}

		sqlStr := fmt.Sprintf("SELECT %s FROM %s%s%s",
			join(fieldList, ", "), table, whereClause, orderClause)

		return executeDBAsync(e, L, handle, sqlStr, whereParams, false, true, false)
	})

	// k.db_insert(table, keyvals) - insert row
	e.register("db_insert", "database", func(L *lua.LState) int {
		handleID := L.CheckString(1)
		table := L.CheckString(2)
		keyvals := L.CheckTable(3)

		handle := getDBHandle(L, handleID)
		if handle == nil {
			L.RaiseError("database handle not found: %s", handleID)
			return 0
		}

		var columns []string
		var placeholders []string
		var params []interface{}
		idx := 1

		keyvals.ForEach(func(k, v lua.LValue) {
			col := k.String()
			columns = append(columns, col)
			placeholders = append(placeholders, handle.Placeholder(idx))
			params = append(params, luaValueToGo(v))
			idx++
		})

		sqlStr := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
			table, join(columns, ", "), join(placeholders, ", "))

		return executeDBAsync(e, L, handle, sqlStr, params, true, false, true)
	})

	// k.db_update(table, keyvals, where) - update rows
	e.register("db_update", "database", func(L *lua.LState) int {
		handleID := L.CheckString(1)
		table := L.CheckString(2)
		keyvals := L.CheckTable(3)
		where := L.OptTable(4, L.NewTable())

		handle := getDBHandle(L, handleID)
		if handle == nil {
			L.RaiseError("database handle not found: %s", handleID)
			return 0
		}

		var setClauses []string
		var params []interface{}
		idx := 1

		keyvals.ForEach(func(k, v lua.LValue) {
			col := k.String()
			setClauses = append(setClauses, fmt.Sprintf("%s = %s", col, handle.Placeholder(idx)))
			params = append(params, luaValueToGo(v))
			idx++
		})

		whereClause, whereParams := buildWhereClause(L, where)
		params = append(params, whereParams...)

		sqlStr := fmt.Sprintf("UPDATE %s SET %s%s", table, join(setClauses, ", "), whereClause)

		return executeDBAsync(e, L, handle, sqlStr, params, true, false, false)
	})

	// k.db_delete(table, where) - delete rows
	e.register("db_delete", "database", func(L *lua.LState) int {
		handleID := L.CheckString(1)
		table := L.CheckString(2)
		where := L.OptTable(3, L.NewTable())

		handle := getDBHandle(L, handleID)
		if handle == nil {
			L.RaiseError("database handle not found: %s", handleID)
			return 0
		}

		whereClause, whereParams := buildWhereClause(L, where)
		sqlStr := fmt.Sprintf("DELETE FROM %s%s", table, whereClause)

		return executeDBAsync(e, L, handle, sqlStr, whereParams, true, false, false)
	})

	// k.tx_begin(handle) - begin transaction
	e.register("tx_begin", "database", func(L *lua.LState) int {
		handleID := L.CheckString(1)
		handle := getDBHandle(L, handleID)
		if handle == nil {
			L.RaiseError("database handle not found: %s", handleID)
			return 0
		}

		handle.mu.Lock()
		defer handle.mu.Unlock()

		if handle.inTx {
			L.RaiseError("transaction already in progress")
			return 0
		}

		tx, err := handle.db.Begin()
		if err != nil {
			L.RaiseError("failed to begin transaction: %v", err)
			return 0
		}
		handle.tx = tx
		handle.inTx = true
		return 0
	})

	// k.tx_commit(handle) - commit transaction
	e.register("tx_commit", "database", func(L *lua.LState) int {
		handleID := L.CheckString(1)
		handle := getDBHandle(L, handleID)
		if handle == nil {
			L.RaiseError("database handle not found: %s", handleID)
			return 0
		}

		handle.mu.Lock()
		defer handle.mu.Unlock()

		if !handle.inTx || handle.tx == nil {
			L.RaiseError("no transaction in progress")
			return 0
		}

		if err := handle.tx.Commit(); err != nil {
			L.RaiseError("failed to commit transaction: %v", err)
			return 0
		}
		handle.tx = nil
		handle.inTx = false
		return 0
	})

	// k.tx_rollback(handle) - rollback transaction
	e.register("tx_rollback", "database", func(L *lua.LState) int {
		handleID := L.CheckString(1)
		handle := getDBHandle(L, handleID)
		if handle == nil {
			L.RaiseError("database handle not found: %s", handleID)
			return 0
		}

		handle.mu.Lock()
		defer handle.mu.Unlock()

		if !handle.inTx || handle.tx == nil {
			L.RaiseError("no transaction in progress")
			return 0
		}

		if err := handle.tx.Rollback(); err != nil {
			L.RaiseError("failed to rollback transaction: %v", err)
			return 0
		}
		handle.tx = nil
		handle.inTx = false
		return 0
	})

	// k.rows(result) - iterator for result set
	e.register("rows", "database", func(L *lua.LState) int {
		result := L.CheckTable(1)

		rows := result.RawGetString("rows")
		if rows == lua.LNil {
			L.Push(lua.LNil)
			return 1
		}
		rowsTbl, ok := rows.(*lua.LTable)
		if !ok {
			L.Push(lua.LNil)
			return 1
		}

		// Create an iterator closure
		idx := 0
		iterFn := L.NewFunction(func(L *lua.LState) int {
			idx++
			row := rowsTbl.RawGetInt(idx)
			if row == lua.LNil {
				L.Push(lua.LNil)
				return 1
			}
			L.Push(row)
			return 1
		})
		L.Push(iterFn)
		return 1
	})
}

// executeDBAsync executes a database operation asynchronously using the session's worker pool
func executeDBAsync(e *Env, L *lua.LState, handle *DBHandle, sqlStr string, params []interface{}, isExec, isQuery, isInsert bool) int {
	sess := e.App.Session()
	if sess != nil {
		// Web mode: run DB work in the session's worker goroutine and yield
		// the handler coroutine; the session resumes us with the result.
		sess.RunAsync(L, func() {}, func() (interface{}, error) {
			return executeDBQuery(handle, sqlStr, params, isExec, isQuery, isInsert)
		})

		// Yield the current coroutine - it will be resumed by the session when done
		return L.Yield(lua.LNil)
	}

	// Test mode (--test): execute synchronously
	result, err := executeDBQuery(handle, sqlStr, params, isExec, isQuery, isInsert)
	if err != nil {
		L.RaiseError("database error: %v", err)
		return 0
	}
	pushDBResult(L, result, isExec, isInsert)
	return 1
}

// executeDBQuery executes a database query synchronously (for test mode)
func executeDBQuery(handle *DBHandle, sqlStr string, params []interface{}, isExec, isQuery, isInsert bool) (interface{}, error) {
	handle.mu.Lock()
	defer handle.mu.Unlock()

	var executor interface {
		Exec(query string, args ...interface{}) (sql.Result, error)
		Query(query string, args ...interface{}) (*sql.Rows, error)
		QueryRow(query string, args ...interface{}) *sql.Row
	}

	if handle.inTx && handle.tx != nil {
		executor = handle.tx
	} else {
		executor = handle.db
	}

	if isExec {
		result, err := executor.Exec(sqlStr, params...)
		if err != nil {
			return nil, err
		}
		if isInsert {
			lastID, _ := result.LastInsertId()
			rowsAffected, _ := result.RowsAffected()
			return map[string]interface{}{
				"last_insert_id": lastID,
				"rows_affected":  rowsAffected,
			}, nil
		}
		rowsAffected, _ := result.RowsAffected()
		return map[string]interface{}{
			"rows_affected": rowsAffected,
		}, nil
	}

	if isQuery {
		rows, err := executor.Query(sqlStr, params...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		columns, err := rows.Columns()
		if err != nil {
			return nil, err
		}

		var results []map[string]interface{}
		for rows.Next() {
			values := make([]interface{}, len(columns))
			valuePtrs := make([]interface{}, len(columns))
			for i := range values {
				valuePtrs[i] = &values[i]
			}

			if err := rows.Scan(valuePtrs...); err != nil {
				return nil, err
			}

			row := make(map[string]interface{})
			for i, col := range columns {
				val := values[i]
				if b, ok := val.([]byte); ok {
					row[col] = string(b)
				} else {
					row[col] = val
				}
			}
			results = append(results, row)
		}

		if err := rows.Err(); err != nil {
			return nil, err
		}

		return map[string]interface{}{
			"columns": columns,
			"rows":    results,
		}, nil
	}

	return nil, fmt.Errorf("unknown operation type")
}

// pushDBResult pushes a database result to the Lua stack
func pushDBResult(L *lua.LState, result interface{}, isExec, isInsert bool) {
	resMap, ok := result.(map[string]interface{})
	if !ok {
		L.Push(lua.LNil)
		return
	}

	tbl := L.NewTable()
	for k, v := range resMap {
		tbl.RawSetString(k, common.GoValueToLua(L, v))
	}
	L.Push(tbl)
}

// parseDSN extracts driver name and cleans DSN
func parseDSN(dsn string) (driver, cleanDSN string) {
	// mysql://user:pass@host/db
	// postgres://user:pass@host/db
	// sqlserver://user:pass@host/db
	// sqlite:///path/to/db
	if len(dsn) > 8 && dsn[:8] == "mysql://" {
		return "mysql", dsn[8:]
	}
	if len(dsn) > 11 && dsn[:11] == "postgres://" {
		return "postgres", dsn[11:]
	}
	if len(dsn) > 13 && dsn[:13] == "sqlserver://" {
		return "sqlserver", dsn[13:]
	}
	if len(dsn) > 8 && dsn[:8] == "sqlite://" {
		return "sqlite", dsn[8:]
	}
	// Try to detect from prefix (driver:path format)
	if len(dsn) > 6 && dsn[:6] == "mysql:" {
		return "mysql", dsn[6:]
	}
	if len(dsn) > 9 && dsn[:9] == "postgres:" {
		return "postgres", dsn[9:]
	}
	if len(dsn) > 11 && dsn[:11] == "sqlserver:" {
		return "sqlserver", dsn[11:]
	}
	if len(dsn) > 7 && dsn[:7] == "sqlite:" {
		return "sqlite", dsn[7:]
	}
	return "", ""
}

// buildWhereClause builds WHERE clause from table
func buildWhereClause(L *lua.LState, where *lua.LTable) (string, []interface{}) {
	var clauses []string
	var params []interface{}
	idx := 1

	where.ForEach(func(k, v lua.LValue) {
		col := k.String()
		clauses = append(clauses, fmt.Sprintf("%s = ?", col)) // Use ? as generic placeholder
		params = append(params, luaValueToGo(v))
		idx++
	})

	if len(clauses) == 0 {
		return "", params
	}
	return " WHERE " + join(clauses, " AND "), params
}

// getDBHandle retrieves a database handle by ID
func getDBHandle(L *lua.LState, id string) *DBHandle {
	dbHandlesMu.Lock()
	defer dbHandlesMu.Unlock()
	return dbHandles[id]
}

// Placeholder returns the parameter placeholder for the driver
func (h *DBHandle) Placeholder(n int) string {
	switch h.driver {
	case "postgres":
		return fmt.Sprintf("$%d", n)
	case "sqlserver":
		return fmt.Sprintf("@p%d", n)
	default: // mysql, sqlite3
		return "?"
	}
}

// luaValueToGo converts a Lua value to a Go value for SQL parameters
func luaValueToGo(v lua.LValue) interface{} {
	switch v.Type() {
	case lua.LTNil:
		return nil
	case lua.LTBool:
		return bool(v.(lua.LBool))
	case lua.LTNumber:
		return float64(v.(lua.LNumber))
	case lua.LTString:
		return string(v.(lua.LString))
	default:
		return v.String()
	}
}

// Close closes the database handle
func (h *DBHandle) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.inTx && h.tx != nil {
		h.tx.Rollback()
		h.tx = nil
		h.inTx = false
	}
	if h.db != nil {
		return h.db.Close()
	}
	return nil
}
