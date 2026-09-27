//go:build js && wasm

// Package bindings implements the WASM profile for flow bindings:
// k.param_set/get, k.net_ok, k.ping, k.locale, k.yield, k.on_error
package bindings

import (
	"syscall/js"

	"github.com/yuin/gopher-lua"
)

// registerFlowWasm registers flow bindings for WASM mode.
func registerFlowWasm(e *Env) {
	k := e.L.GetGlobal("k").(*lua.LTable)

	// k.print for console.log
	k.RawSetString("print", e.L.NewFunction(func(L *lua.LState) int {
		n := L.GetTop()
		args := make([]interface{}, n)
		for i := 1; i <= n; i++ {
			args[i-1] = L.Get(i).String()
		}
		js.Global().Get("console").Call("log", args...)
		return 0
	}))

	// k.sleep(ms) - yields via setTimeout
	k.RawSetString("sleep", e.L.NewFunction(func(L *lua.LState) int {
		ms := L.CheckInt(1)
		// In WASM, we yield the coroutine and resume via setTimeout
		return e.yieldForSleep(L, ms)
	}))

	// k.quit - stops the app
	k.RawSetString("quit", e.L.NewFunction(func(L *lua.LState) int {
		e.App.RequestQuit()
		return 0
	}))

	// k.error(msg) - sets ERRORCODE/ERRORMSG
	k.RawSetString("error", e.L.NewFunction(func(L *lua.LState) int {
		msg := L.CheckString(1)
		e.setErrorGlobals(L, KErrorGeneric, msg)
		return 0
	}))

	// k.on_error(fn) - registers error handler
	k.RawSetString("on_error", e.L.NewFunction(func(L *lua.LState) int {
		fn := L.CheckFunction(1)
		e.onErr = fn
		return 0
	}))

	// k.yield - cooperative yield
	k.RawSetString("yield", e.L.NewFunction(func(L *lua.LState) int {
		return L.Yield(lua.LNil)
	}))

	// k.param_set(key, value)
	k.RawSetString("param_set", e.L.NewFunction(func(L *lua.LState) int {
		key := L.CheckString(1)
		value := L.CheckString(2)
		js.Global().Get("localStorage").Call("setItem", "kalua_param_"+key, value)
		return 0
	}))

	// k.param_get(key) -> value
	k.RawSetString("param_get", e.L.NewFunction(func(L *lua.LState) int {
		key := L.CheckString(1)
		val := js.Global().Get("localStorage").Call("getItem", "kalua_param_"+key)
		if val.IsNull() || val.IsUndefined() {
			L.Push(lua.LString(""))
		} else {
			L.Push(lua.LString(val.String()))
		}
		return 1
	}))

	// k.net_ok() -> bool
	k.RawSetString("net_ok", e.L.NewFunction(func(L *lua.LState) int {
		// Use fetch with no-cors to check connectivity
		_ = js.Global().Call("fetch", "https://1.1.1.1", map[string]interface{}{
			"mode": "no-cors",
		})
		// For simplicity, return true (async would need coroutine yield)
		L.Push(lua.LBool(true))
		return 1
	}))

	// k.ping(host) -> latency_ms
	k.RawSetString("ping", e.L.NewFunction(func(L *lua.LState) int {
		_ = L.CheckString(1)
		// Simplified: return 0 (would need async fetch timing)
		L.Push(lua.LNumber(0))
		return 1
	}))

	// k.locale() -> string
	k.RawSetString("locale", e.L.NewFunction(func(L *lua.LState) int {
		locale := js.Global().Get("navigator").Get("language").String()
		L.Push(lua.LString(locale))
		return 1
	}))
}

// yieldForSleep implements k.sleep - for M0, just return immediately.
// Full async sleep will be implemented in M1/M2.
func (e *Env) yieldForSleep(L *lua.LState, ms int) int {
	// M0: no-op sleep
	return 0
}