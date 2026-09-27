//go:build js && wasm

// Package bindings implements the WASM profile for param operations using localStorage.
package bindings

import (
	"syscall/js"

	"github.com/yuin/gopher-lua"
)

// registerParamWasm registers k.param_* bindings for WASM mode using localStorage.
func registerParamWasm(e *Env) {
	k := e.L.GetGlobal("k").(*lua.LTable)
	paramTbl := e.L.NewTable()

	// k.param_set(key, value)
	e.registerInTable(paramTbl, "set", "flow", func(L *lua.LState) int {
		key := L.CheckString(1)
		value := L.CheckString(2)
		js.Global().Get("localStorage").Call("setItem", "kalua_param_"+key, value)
		return 0
	})

	// k.param_get(key) -> value
	e.registerInTable(paramTbl, "get", "flow", func(L *lua.LState) int {
		key := L.CheckString(1)
		val := js.Global().Get("localStorage").Call("getItem", "kalua_param_"+key)
		if val.IsNull() || val.IsUndefined() {
			L.Push(lua.LString(""))
		} else {
			L.Push(lua.LString(val.String()))
		}
		return 1
	})

	k.RawSetString("param", paramTbl)
}