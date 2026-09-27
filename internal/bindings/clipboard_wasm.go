//go:build js && wasm

// Package bindings implements the WASM profile for clipboard operations.
package bindings

import (
	"syscall/js"

	"github.com/yuin/gopher-lua"
)

// registerClipboardWasm registers k.clipboard_* bindings for WASM mode.
func registerClipboardWasm(e *Env) {
	k := e.L.GetGlobal("k").(*lua.LTable)
	clipTbl := e.L.NewTable()

	// k.clipboard_set(text)
	e.registerInTable(clipTbl, "set", "flow", func(L *lua.LState) int {
		text := L.CheckString(1)
		js.Global().Get("navigator").Get("clipboard").Call("writeText", text)
		return 0
	})

	// k.clipboard_get() -> string (synchronous stub for M2)
	e.registerInTable(clipTbl, "get", "flow", func(L *lua.LState) int {
		L.Push(lua.LString(""))
		return 1
	})

	k.RawSetString("clipboard", clipTbl)
}