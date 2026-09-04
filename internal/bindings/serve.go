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
// Expression-function globals (§5.9) and K.* helpers (§2.3) are installed too,
// because serve and run modes share the coerce layer and expression surface.
func SetupServe(L *lua.LState, app *vm.App, opts Options, shared SharedStore, wsHub WSHub, tcpHub TCPHub, logger Logger) {
	// Create k table first
	L.SetGlobal("k", L.NewTable())

	k := L.GetGlobal("k").(*lua.LTable)

	// Register serve-mode WS/TCP modules (Env-independent)
	registerWS(L, wsHub)
	registerTCP(L, tcpHub)

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

	// Build the Env so expression functions and helpers share the same
	// kNULL/coerce surface as run mode.
	e := &Env{
		L:       L,
		App:     app,
		known:   map[string]string{},
		Logger:  logger,
		k:       k,
		workdir: workdirOf(opts),
		allowFS: allowFSOf(opts),
		verbose: opts.Verbose,
	}
	if e.maxFileSize <= 0 {
		e.maxFileSize = DefaultMaxFileSize
	}
	e.kNULL = L.NewTable()
	K := L.NewTable()
	registerHelpers(e, K)
	K.RawSetString("NULL", e.kNULL)
	K.RawSetString("is_null", L.NewFunction(e.isNull))
	L.SetGlobal("K", K)

	registerExprFuncs(e)
	registerDebug(e)

	// k.shared_* uses the Env for JSON value serialization.
	registerShared(e, shared)

	// Tier-2 self-contained bindings are shared between run and serve modes:
	// data formats, result-set conversions, comm, crypto, and files (zip).
	registerFormats(e)
	registerRows(e)
	registerComm(e)
	registerSMTP(e)
	registerPop3(e)
	registerFTP(e)
	registerSoap(e)
	registerCrypto(e)
	registerFiles(e)
	registerDB(e)

	// Session-dependent UI/timer bindings are removed; SetupUIError installs
	// the error-raising stubs for form/ctrl/msgbox/status. Timers and net/param
	// helpers stay available (net/param are pure file/net behavior).
}

// registerShared registers k.shared_* bindings. Values are stored as JSON so
// numbers, booleans, tables, arrays, and K.NULL round-trip across workers.
func registerShared(e *Env, shared SharedStore) {
	k := e.L.GetGlobal("k").(*lua.LTable)
	sharedTbl := e.L.NewTable()

	// k.shared.set(key, value) — any Lua value is stored as its JSON form
	sharedTbl.RawSetString("set", e.L.NewFunction(func(L *lua.LState) int {
		key := L.CheckString(1)
		text, err := stringifyJSON(e, L.Get(2))
		if err != nil {
			L.RaiseError("shared error: %v", err)
			return 0
		}
		shared.Set(key, text)
		return 0
	}))

	// k.shared.get(key) -> value (JSON-decoded when the stored value is valid
	// JSON; otherwise returned as a raw string, preserving legacy behavior)
	sharedTbl.RawSetString("get", e.L.NewFunction(func(L *lua.LState) int {
		key := L.CheckString(1)
		val := shared.Get(key)
		v, err := parseJSON(L, e, []byte(val))
		if err != nil {
			L.Push(lua.LString(val))
		} else {
			L.Push(v)
		}
		return 1
	}))

	// k.shared.del(key)
sharedTbl.RawSetString("del", e.L.NewFunction(func(L *lua.LState) int {
		key := L.CheckString(1)
		shared.Del(key)
		return 0
	}))

	// k.shared.keys([pattern]) -> table of keys
sharedTbl.RawSetString("keys", e.L.NewFunction(func(L *lua.LState) int {
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

	// k.shared.incr(key, delta) -> new_value
sharedTbl.RawSetString("incr", e.L.NewFunction(func(L *lua.LState) int {
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

	// Disable k.status_show / k.status_close (and legacy k.status_*).
	statusFuncs := []string{"show", "close", "set", "clear", "progress"}
	for _, fn := range statusFuncs {
		k.RawSetString("status_"+fn, L.NewFunction(errorFunc("k.status_"+fn)))
	}

	// Disable k.timer_start / k.timer_stop (need a session actor).
	k.RawSetString("timer_start", L.NewFunction(errorFunc("k.timer_start")))
	k.RawSetString("timer_stop", L.NewFunction(errorFunc("k.timer_stop")))
}
