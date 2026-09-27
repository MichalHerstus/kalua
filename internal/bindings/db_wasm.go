//go:build js && wasm

// Package bindings implements the WASM profile for database operations using wa-sqlite.
package bindings

import (
	"database/sql"
	"sync"
	"syscall/js"

	"github.com/yuin/gopher-lua"
)

// registerDBWasm registers k.connect_sqlite, k.disconnect_sqlite, and k.db_* bindings for WASM mode.
func registerDBWasm(e *Env) {
	k := e.L.GetGlobal("k").(*lua.LTable)
	dbTbl := e.L.NewTable()

	// k.connect_sqlite(dsn) -> handle
	// dsn format: "file:app.db" or "file:app.db?mode=rwc"
	e.registerInTable(dbTbl, "connect_sqlite", "database", func(L *lua.LState) int {
		dsn := L.CheckString(1)

		// Open wa-sqlite database
		db, err := sql.Open("sqlite3", dsn)
		if err != nil {
			return e.fail(L, KErrorComm, "failed to connect to SQLite: "+err.Error())
		}

		// Test connection
		if err := db.Ping(); err != nil {
			db.Close()
			return e.fail(L, KErrorComm, "failed to ping SQLite: "+err.Error())
		}

		// Store handle in Go map (using a simple ID)
		handleID := "sqlite_" + js.Global().Get("Symbol").Call("for", "kalua_db_handle").String()
		dbHandlesMu.Lock()
		dbHandles[handleID] = &DBHandle{db: db, driver: "sqlite3"}
		dbHandlesMu.Unlock()

		L.Push(lua.LString(handleID))
		return 1
	})

	// k.disconnect_sqlite([handle])
	e.registerInTable(dbTbl, "disconnect_sqlite", "database", func(L *lua.LState) int {
		handleID := L.OptString(1, "")
		if handleID == "" {
			// Disconnect all SQLite handles
			dbHandlesMu.Lock()
			for id, h := range dbHandles {
				if id[:6] == "sqlite" {
					h.Close()
					delete(dbHandles, id)
				}
			}
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
			return e.fail(L, KErrorNotConnected, "database handle not found: "+handleID)
		}
		h.Close()
		return 0
	})

	// k.sql(handle, query) -> rows
	e.register("sql", "database", func(L *lua.LState) int {
		handleID := L.CheckString(1)
		query := L.CheckString(2)

		dbHandlesMu.Lock()
		h, ok := dbHandles[handleID]
		dbHandlesMu.Unlock()

		if !ok {
			return e.fail(L, KErrorNotConnected, "database handle not found: "+handleID)
		}

		h.mu.Lock()
		defer h.mu.Unlock()

		rows, err := h.db.Query(query)
		if err != nil {
			return e.fail(L, KErrorComm, "sql error: "+err.Error())
		}
		defer rows.Close()

		cols, err := rows.Columns()
		if err != nil {
			return e.fail(L, KErrorComm, "sql error: "+err.Error())
		}

		result := L.NewTable()
		for rows.Next() {
			vals := make([]interface{}, len(cols))
			valPtrs := make([]interface{}, len(cols))
			for i := range cols {
				valPtrs[i] = &vals[i]
			}
			if err := rows.Scan(valPtrs...); err != nil {
				return e.fail(L, KErrorComm, "sql error: "+err.Error())
			}

			row := L.NewTable()
			for i, col := range cols {
				var v lua.LValue
				switch val := vals[i].(type) {
				case nil:
					v = e.kNULL
				case int64:
					v = lua.LNumber(val)
				case float64:
					v = lua.LNumber(val)
				case string:
					v = lua.LString(val)
				case []byte:
					v = lua.LString(string(val))
				case bool:
					v = lua.LBool(val)
				default:
					v = lua.LString("")
				}
				row.RawSetString(col, v)
			}
			result.RawSetInt(result.Len()+1, row)
		}

		L.Push(result)
		return 1
	})

	// k.db_select(handle, query) -> rows
	e.register("db_select", "database", func(L *lua.LState) int {
		handleID := L.CheckString(1)
		query := L.CheckString(2)

		dbHandlesMu.Lock()
		h, ok := dbHandles[handleID]
		dbHandlesMu.Unlock()

		if !ok {
			return e.fail(L, KErrorNotConnected, "database handle not found: "+handleID)
		}

		h.mu.Lock()
		defer h.mu.Unlock()

		rows, err := h.db.Query(query)
		if err != nil {
			return e.fail(L, KErrorComm, "db_select error: "+err.Error())
		}
		defer rows.Close()

		cols, err := rows.Columns()
		if err != nil {
			return e.fail(L, KErrorComm, "db_select error: "+err.Error())
		}

		result := L.NewTable()
		for rows.Next() {
			vals := make([]interface{}, len(cols))
			valPtrs := make([]interface{}, len(cols))
			for i := range cols {
				valPtrs[i] = &vals[i]
			}
			if err := rows.Scan(valPtrs...); err != nil {
				return e.fail(L, KErrorComm, "db_select error: "+err.Error())
			}

			row := L.NewTable()
			for i, col := range cols {
				var v lua.LValue
				switch val := vals[i].(type) {
				case nil:
					v = e.kNULL
				case int64:
					v = lua.LNumber(val)
				case float64:
					v = lua.LNumber(val)
				case string:
					v = lua.LString(val)
				case []byte:
					v = lua.LString(string(val))
				case bool:
					v = lua.LBool(val)
				default:
					v = lua.LString("")
				}
				row.RawSetString(col, v)
			}
			result.RawSetInt(result.Len()+1, row)
		}

		L.Push(result)
		return 1
	})

	// k.db_insert(handle, table, data) -> last_id
	e.registerInTable(dbTbl, "db_insert", "database", func(L *lua.LState) int {
		handleID := L.CheckString(1)
		table := L.CheckString(2)
		data := L.CheckTable(3)

		dbHandlesMu.Lock()
		h, ok := dbHandles[handleID]
		dbHandlesMu.Unlock()

		if !ok {
			return e.fail(L, KErrorNotConnected, "database handle not found: "+handleID)
		}

		cols := []string{}
		vals := []string{}
		data.ForEach(func(k, v lua.LValue) {
			cols = append(cols, k.String())
			vals = append(vals, v.String())
		})

		colStr := joinArgs(cols)
		valStr := joinArgs(vals)
		query := "INSERT INTO " + table + " (" + colStr + ") VALUES (" + valStr + ")"

		h.mu.Lock()
		defer h.mu.Unlock()

		result, err := h.db.Exec(query)
		if err != nil {
			return e.fail(L, KErrorComm, "db_insert error: "+err.Error())
		}

		id, _ := result.LastInsertId()
		L.Push(lua.LNumber(id))
		return 1
	})

	// k.db_update(handle, table, where, data) -> affected
	e.registerInTable(dbTbl, "db_update", "database", func(L *lua.LState) int {
		handleID := L.CheckString(1)
		table := L.CheckString(2)
		where := L.CheckString(3)
		data := L.CheckTable(4)

		dbHandlesMu.Lock()
		h, ok := dbHandles[handleID]
		dbHandlesMu.Unlock()

		if !ok {
			return e.fail(L, KErrorNotConnected, "database handle not found: "+handleID)
		}

		sets := []string{}
		data.ForEach(func(k, v lua.LValue) {
			sets = append(sets, k.String()+" = "+v.String())
		})
		query := "UPDATE " + table + " SET " + joinArgs(sets) + " WHERE " + where

		h.mu.Lock()
		defer h.mu.Unlock()

		result, err := h.db.Exec(query)
		if err != nil {
			return e.fail(L, KErrorComm, "db_update error: "+err.Error())
		}

		affected, _ := result.RowsAffected()
		L.Push(lua.LNumber(affected))
		return 1
	})

	// k.db_delete(handle, table, where) -> affected
	e.registerInTable(dbTbl, "db_delete", "database", func(L *lua.LState) int {
		handleID := L.CheckString(1)
		table := L.CheckString(2)
		where := L.CheckString(3)

		dbHandlesMu.Lock()
		h, ok := dbHandles[handleID]
		dbHandlesMu.Unlock()

		if !ok {
			return e.fail(L, KErrorNotConnected, "database handle not found: "+handleID)
		}

		query := "DELETE FROM " + table + " WHERE " + where

		h.mu.Lock()
		defer h.mu.Unlock()

		result, err := h.db.Exec(query)
		if err != nil {
			return e.fail(L, KErrorComm, "db_delete error: "+err.Error())
		}

		affected, _ := result.RowsAffected()
		L.Push(lua.LNumber(affected))
		return 1
	})

	// k.tx_begin(handle)
	e.registerInTable(dbTbl, "tx_begin", "database", func(L *lua.LState) int {
		handleID := L.CheckString(1)

		dbHandlesMu.Lock()
		h, ok := dbHandles[handleID]
		dbHandlesMu.Unlock()

		if !ok {
			return e.fail(L, KErrorNotConnected, "database handle not found: "+handleID)
		}

		h.mu.Lock()
		defer h.mu.Unlock()

		tx, err := h.db.Begin()
		if err != nil {
			return e.fail(L, KErrorComm, "tx_begin error: "+err.Error())
		}
		h.tx = tx
		h.inTx = true
		return 0
	})

	// k.tx_commit(handle)
	e.registerInTable(dbTbl, "tx_commit", "database", func(L *lua.LState) int {
		handleID := L.CheckString(1)

		dbHandlesMu.Lock()
		h, ok := dbHandles[handleID]
		dbHandlesMu.Unlock()

		if !ok || !h.inTx {
			return e.fail(L, KErrorNotConnected, "no active transaction")
		}

		h.mu.Lock()
		defer h.mu.Unlock()

		if err := h.tx.Commit(); err != nil {
			return e.fail(L, KErrorComm, "tx_commit error: "+err.Error())
		}
		h.tx = nil
		h.inTx = false
		return 0
	})

	// k.tx_rollback(handle)
	e.registerInTable(dbTbl, "tx_rollback", "database", func(L *lua.LState) int {
		handleID := L.CheckString(1)

		dbHandlesMu.Lock()
		h, ok := dbHandles[handleID]
		dbHandlesMu.Unlock()

		if !ok || !h.inTx {
			return e.fail(L, KErrorNotConnected, "no active transaction")
		}

		h.mu.Lock()
		defer h.mu.Unlock()

		if err := h.tx.Rollback(); err != nil {
			return e.fail(L, KErrorComm, "tx_rollback error: "+err.Error())
		}
		h.tx = nil
		h.inTx = false
		return 0
	})

	k.RawSetString("db", dbTbl)
}

// DBHandle wraps a database connection
type DBHandle struct {
	db     *sql.DB
	mu     sync.Mutex
	tx     *sql.Tx
	inTx   bool
	driver string
}

// Close closes the database connection
func (h *DBHandle) Close() {
	if h.db != nil {
		h.db.Close()
		h.db = nil
	}
}

// dbHandles stores database handles by ID
var dbHandles = make(map[string]*DBHandle)
var dbHandlesMu sync.Mutex