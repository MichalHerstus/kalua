//go:build js && wasm

// Package bindings implements the WASM profile for system operations.
package bindings

import (
	"syscall/js"

	"github.com/yuin/gopher-lua"
)

// registerSystemWasm registers k.screen_size, k.net_ok, k.ping, k.locale for WASM mode (synchronous stubs for M2).
func registerSystemWasm(e *Env) {
	_ = e.L.GetGlobal("k").(*lua.LTable)

	// k.screen_size() -> {width, height}
	e.register("screen_size", "flow", func(L *lua.LState) int {
		w := js.Global().Get("window").Get("innerWidth").Int()
		h := js.Global().Get("window").Get("innerHeight").Int()
		result := L.NewTable()
		result.RawSetString("width", lua.LNumber(w))
		result.RawSetString("height", lua.LNumber(h))
		L.Push(result)
		return 1
	})

	// k.net_ok() -> bool (synchronous stub for M2)
	e.register("net_ok", "flow", func(L *lua.LState) int {
		L.Push(lua.LBool(true))
		return 1
	})

	// k.ping(host) -> latency_ms (synchronous stub for M2)
	e.register("ping", "flow", func(L *lua.LState) int {
		_ = L.CheckString(1)
		L.Push(lua.LNumber(0))
		return 1
	})

	// k.locale() -> string
	e.register("locale", "flow", func(L *lua.LState) int {
		locale := js.Global().Get("navigator").Get("language").String()
		L.Push(lua.LString(locale))
		return 1
	})
}