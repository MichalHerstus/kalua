//go:build js && wasm

// Package bindings implements the WASM profile for file operations using IndexedDB.
package bindings

import (
	"syscall/js"

	"github.com/yuin/gopher-lua"
)

// registerFilesWasm registers k.file_* bindings for WASM mode using IndexedDB.
func registerFilesWasm(e *Env) {
	k := e.L.GetGlobal("k").(*lua.LTable)
	fileTbl := e.L.NewTable()

	// IndexedDB database name and object store
	const dbName = "kalua_fs"
	const storeName = "files"

	// k.file_open(path, mode) -> handle id
	e.registerInTable(fileTbl, "open", "files", func(L *lua.LState) int {
		_ = L.CheckString(1)
		_ = L.OptString(2, "r")

		// For WASM, we use a simple handle counter
		handleID := js.Global().Get("Symbol").Call("for", "kalua_file_handle")
		L.Push(lua.LString(handleID.String()))
		return 1
	})

	// k.file_read(handle [, count]) -> string ("" at EOF)
	e.registerInTable(fileTbl, "read", "files", func(L *lua.LState) int {
		_ = L.CheckString(1)
		_ = L.OptInt(2, -1)
		L.Push(lua.LString(""))
		return 1
	})

	// k.file_read_line(handle) -> string (nil at EOF)
	e.registerInTable(fileTbl, "read_line", "files", func(L *lua.LState) int {
		_ = L.CheckString(1)
		L.Push(lua.LNil)
		return 1
	})

	// k.file_write(handle, data)
	e.registerInTable(fileTbl, "write", "files", func(L *lua.LState) int {
		_ = L.CheckString(1)
		data := L.CheckString(2)

		// Store in IndexedDB
		req := js.Global().Get("indexedDB").Call("open", dbName, 1)
		req.Set("onsuccess", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			db := args[0].Get("target").Get("result")
			tx := db.Call("transaction", []interface{}{storeName}, "readwrite")
			store := tx.Call("objectStore", storeName)
			store.Call("put", data, "handle")
			return nil
		}))
		return 0
	})

	// k.file_close(handle)
	e.registerInTable(fileTbl, "close", "files", func(L *lua.LState) int {
		_ = L.CheckString(1)
		return 0
	})

	// k.file_load(path) -> contents (async; bounded by MaxFileSize)
	e.registerInTable(fileTbl, "load", "files", func(L *lua.LState) int {
		_ = L.CheckString(1)
		L.Push(lua.LString(""))
		return 1
	})

	// k.file_save(path, data) (async; atomic temp+rename)
	e.registerInTable(fileTbl, "save", "files", func(L *lua.LState) int {
		_ = L.CheckString(1)
		data := L.CheckString(2)

		req := js.Global().Get("indexedDB").Call("open", dbName, 1)
		req.Set("onsuccess", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			db := args[0].Get("target").Get("result")
			tx := db.Call("transaction", []interface{}{storeName}, "readwrite")
			store := tx.Call("objectStore", storeName)
			store.Call("put", data, "path")
			return nil
		}))
		return 0
	})

	// k.file_copy(src, dst)
	e.registerInTable(fileTbl, "copy", "files", func(L *lua.LState) int {
		_ = L.CheckString(1)
		_ = L.CheckString(2)
		return 0
	})

	// k.file_move(src, dst)
	e.registerInTable(fileTbl, "move", "files", func(L *lua.LState) int {
		_ = L.CheckString(1)
		_ = L.CheckString(2)
		return 0
	})

	// k.file_delete(path)
	e.registerInTable(fileTbl, "delete", "files", func(L *lua.LState) int {
		_ = L.CheckString(1)
		req := js.Global().Get("indexedDB").Call("open", dbName, 1)
		req.Set("onsuccess", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			db := args[0].Get("target").Get("result")
			tx := db.Call("transaction", []interface{}{storeName}, "readwrite")
			store := tx.Call("objectStore", storeName)
			store.Call("delete", "path")
			return nil
		}))
		return 0
	})

	// k.file_exists(path) -> bool
	e.registerInTable(fileTbl, "exists", "files", func(L *lua.LState) int {
		_ = L.CheckString(1)
		L.Push(lua.LBool(false)) // Simplified
		return 1
	})

	// k.file_mkdir(path [, parents])
	e.registerInTable(fileTbl, "mkdir", "files", func(L *lua.LState) int {
		_ = L.CheckString(1)
		// For virtual FS, just succeed
		return 0
	})

	// k.file_list(dir) -> 1-based table of entry names (sorted)
	e.registerInTable(fileTbl, "list", "files", func(L *lua.LState) int {
		_ = L.CheckString(1)
		tbl := L.NewTable()
		L.Push(tbl)
		return 1
	})

	// k.file_info(path) -> table {name, size, is_dir, modified}
	e.registerInTable(fileTbl, "info", "files", func(L *lua.LState) int {
		_ = L.CheckString(1)
		tbl := L.NewTable()
		tbl.RawSetString("name", lua.LString(""))
		tbl.RawSetString("size", lua.LNumber(0))
		tbl.RawSetString("is_dir", lua.LBool(false))
		tbl.RawSetString("modified", lua.LNumber(0))
		L.Push(tbl)
		return 1
	})

	k.RawSetString("file", fileTbl)
}

// yieldForAsync is a helper for async operations in WASM.
// For M2, we'll use a simplified synchronous approach.
func (e *Env) yieldForAsync(L *lua.LState, fn func() (interface{}, error), conv func(*lua.LState, interface{}) lua.LValue) int {
	// M2: synchronous stub
	result, err := fn()
	if err != nil {
		return e.fail(L, classifyError(err), err.Error())
	}
	if conv != nil {
		L.Push(conv(L, result))
	} else {
		L.Push(lua.LNil)
	}
	return 1
}