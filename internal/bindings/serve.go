package bindings

import (
	"github.com/yuin/gopher-lua"

	"kalua/internal/vm"
)

// SharedStore defines the interface for shared key-value storage.
type SharedStore interface {
	Set(key, value string)
	Get(key string) string
	Del(key string)
	Keys(pattern string) []string
	Incr(key string, delta int64) int64
}

// WSHub defines the interface for WebSocket hub operations.
type WSHub interface {
	Broadcast(msg []byte)
	Send(id string, msg []byte) bool
	Close(id string)
}

// TCPHub defines the interface for TCP hub operations.
type TCPHub interface {
	Send(id string, msg []byte) bool
	Close(id string)
}

// SetupServe configures the Lua state for serve mode (headless API).
// This registers k.shared_*, k.ws_*, k.tcp_* bindings and disables UI bindings.
func SetupServe(L *lua.LState, app *vm.App, opts Options, shared SharedStore, wsHub WSHub, tcpHub TCPHub, logger Logger) {
	// Create k table first
	L.SetGlobal("k", L.NewTable())

	// Register serve-mode modules
	registerShared(L, shared)
	registerWS(L, wsHub)
	registerTCP(L, tcpHub)

	k := L.GetGlobal("k").(*lua.LTable)

	// k.print for logging
	k.RawSetString("print", L.NewFunction(func(L *lua.LState) int {
		n := L.GetTop()
		args := make([]string, n)
		for i := 1; i <= n; i++ {
			args[i-1] = L.Get(i).String()
		}
		logger.Printf("%s", joinArgs(args))
		return 0
	}))

	// k.sleep for delays
	k.RawSetString("sleep", L.NewFunction(func(L *lua.LState) int {
		L.CheckInt(1) // ms
		// In serve mode, sleep yields the coroutine
		return 0
	}))

	// k.quit to stop the worker
	k.RawSetString("quit", L.NewFunction(func(L *lua.LState) int {
		// Signal main to exit
		app.RequestQuit()
		return 0
	}))

	// ARGS global
	argsTable := L.NewTable()
	for i, arg := range opts.Args {
		argsTable.RawSetInt(i+1, lua.LString(arg))
	}
	L.SetGlobal("ARGS", argsTable)
}

// registerShared registers k.shared_* bindings.
func registerShared(L *lua.LState, shared SharedStore) {
	k := L.GetGlobal("k").(*lua.LTable)
	sharedTbl := L.NewTable()

	// k.shared_set(key, value)
	sharedTbl.RawSetString("set", L.NewFunction(func(L *lua.LState) int {
		key := L.CheckString(1)
		value := L.CheckString(2)
		shared.Set(key, value)
		return 0
	}))

	// k.shared_get(key) -> value
	sharedTbl.RawSetString("get", L.NewFunction(func(L *lua.LState) int {
		key := L.CheckString(1)
		val := shared.Get(key)
		L.Push(lua.LString(val))
		return 1
	}))

	// k.shared_del(key)
	sharedTbl.RawSetString("del", L.NewFunction(func(L *lua.LState) int {
		key := L.CheckString(1)
		shared.Del(key)
		return 0
	}))

	// k.shared_keys([pattern]) -> table of keys
	sharedTbl.RawSetString("keys", L.NewFunction(func(L *lua.LState) int {
		pattern := "*"
		if L.GetTop() >= 1 {
			pattern = L.CheckString(1)
		}
		keys := shared.Keys(pattern)
		tbl := L.NewTable()
		for i, k := range keys {
			tbl.RawSetInt(i+1, lua.LString(k))
		}
		L.Push(tbl)
		return 1
	}))

	// k.shared_incr(key, delta) -> new_value
	sharedTbl.RawSetString("incr", L.NewFunction(func(L *lua.LState) int {
		key := L.CheckString(1)
		delta := int64(1)
		if L.GetTop() >= 2 {
			delta = L.CheckInt64(2)
		}
		val := shared.Incr(key, delta)
		L.Push(lua.LNumber(val))
		return 1
	}))

	k.RawSetString("shared", sharedTbl)
}

// registerWS registers k.ws_* bindings.
func registerWS(L *lua.LState, hub WSHub) {
	k := L.GetGlobal("k").(*lua.LTable)
	wsTbl := L.NewTable()

	// k.ws_broadcast(message)
	wsTbl.RawSetString("broadcast", L.NewFunction(func(L *lua.LState) int {
		msg := L.CheckString(1)
		hub.Broadcast([]byte(msg))
		return 0
	}))

	// k.ws_send(client_id, message)
	wsTbl.RawSetString("send", L.NewFunction(func(L *lua.LState) int {
		id := L.CheckString(1)
		msg := L.CheckString(2)
		ok := hub.Send(id, []byte(msg))
		L.Push(lua.LBool(ok))
		return 1
	}))

	// k.ws_close(client_id)
	wsTbl.RawSetString("close", L.NewFunction(func(L *lua.LState) int {
		id := L.CheckString(1)
		hub.Close(id)
		return 0
	}))

	k.RawSetString("ws", wsTbl)
}

// registerTCP registers k.tcp_* bindings.
func registerTCP(L *lua.LState, hub TCPHub) {
	k := L.GetGlobal("k").(*lua.LTable)
	tcpTbl := L.NewTable()

	// k.tcp_send(client_id, data)
	tcpTbl.RawSetString("send", L.NewFunction(func(L *lua.LState) int {
		id := L.CheckString(1)
		data := L.CheckString(2)
		ok := hub.Send(id, []byte(data))
		L.Push(lua.LBool(ok))
		return 1
	}))

	// k.tcp_close(client_id)
	tcpTbl.RawSetString("close", L.NewFunction(func(L *lua.LState) int {
		id := L.CheckString(1)
		hub.Close(id)
		return 0
	}))

	k.RawSetString("tcp", tcpTbl)
}

// joinArgs joins arguments with spaces.
func joinArgs(args []string) string {
	result := ""
	for i, a := range args {
		if i > 0 {
			result += " "
		}
		result += a
	}
	return result
}

// SetupUIError registers UI bindings that error in serve mode.
func SetupUIError(L *lua.LState) {
	k := L.GetGlobal("k").(*lua.LTable)
	if k == nil {
		k = L.NewTable()
		L.SetGlobal("k", k)
	}

	errorFunc := func(feature string) lua.LGFunction {
		return func(L *lua.LState) int {
			L.RaiseError("%s not available in serve mode", feature)
			return 0
		}
	}

	// Disable k.form.*
	formTbl := L.NewTable()
	formFuncs := []string{"new", "show", "close", "on", "clear", "refresh", "return_to"}
	for _, fn := range formFuncs {
		formTbl.RawSetString(fn, L.NewFunction(errorFunc("k.form."+fn)))
	}
	k.RawSetString("form", formTbl)

	// Disable k.ctrl.*
	ctrlTbl := L.NewTable()
	ctrlFuncs := []string{"set_value", "get_value", "set_property", "get_property", "textbox", "button", "label", "combo", "list", "table", "checkbox", "radio"}
	for _, fn := range ctrlFuncs {
		ctrlTbl.RawSetString(fn, L.NewFunction(errorFunc("k.ctrl."+fn)))
	}
	k.RawSetString("ctrl", ctrlTbl)

	// Disable k.msgbox
	k.RawSetString("msgbox", L.NewFunction(errorFunc("k.msgbox")))

	// Disable k.status_*
	statusTbl := L.NewTable()
	statusFuncs := []string{"set", "clear", "progress"}
	for _, fn := range statusFuncs {
		statusTbl.RawSetString(fn, L.NewFunction(errorFunc("k.status_"+fn)))
	}
	k.RawSetString("status", statusTbl)
}
