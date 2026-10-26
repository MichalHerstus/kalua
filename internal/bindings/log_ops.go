//go:build !wasm

package bindings

import (
	"github.com/yuin/gopher-lua"

	"kalua/internal/klog"
)

// registerLogOps installs the Phase 8 structured-log API in serve mode:
// k.log.info/warn/error/debug/trace(msg[, data]) and k.log.stats().
// Records are captured into the configured klog.DB sink when --log-sqlite is
// enabled (opts.LogDB); otherwise the calls are cheap no-ops (matching
// k.print which stays stdout-only). data must be a table; it is stored as JSON.
func registerLogOps(e *Env, opts Options) {
	db := opts.LogDB

	logTbl := e.L.NewTable()

	emitFn := func(level klog.Level) lua.LGFunction {
		return func(L *lua.LState) int {
			if db == nil {
				return 0
			}
			msg := L.OptString(1, "")
			r := klog.Record{
				Level:    level,
				Category: "app",
			}
			if t := L.Get(2); t != lua.LNil {
				if tbl, ok := t.(*lua.LTable); ok {
					if data, err := stringifyJSON(e, tbl); err == nil {
						r.Data = data
					} else {
						r.Data = `{"_error":"data not JSON-serializable"}`
					}
				} else {
					r.Data = `{"_value":` + luaToString(L, 2) + `}`
				}
			}
			r.Message = msg
			db.Emit(r)
			return 0
		}
	}

	logTbl.RawSetString("info", e.L.NewFunction(emitFn(klog.LevelInfo)))
	logTbl.RawSetString("warn", e.L.NewFunction(emitFn(klog.LevelWarn)))
	logTbl.RawSetString("error", e.L.NewFunction(emitFn(klog.LevelError)))
	logTbl.RawSetString("debug", e.L.NewFunction(emitFn(klog.LevelDebug)))
	logTbl.RawSetString("trace", e.L.NewFunction(emitFn(klog.LevelTrace)))

	// k.log.stats() -> {rows, dropped, oldest, newest, path}
	logTbl.RawSetString("stats", e.L.NewFunction(func(L *lua.LState) int {
		if db == nil {
			t := L.NewTable()
			t.RawSetString("rows", lua.LNumber(0))
			t.RawSetString("dropped", lua.LNumber(0))
			t.RawSetString("path", lua.LString(""))
			L.Push(t)
			return 1
		}
		st := db.Stats()
		t := L.NewTable()
		t.RawSetString("rows", lua.LNumber(st.Rows))
		t.RawSetString("dropped", lua.LNumber(st.Dropped))
		if !st.Oldest.IsZero() {
			t.RawSetString("oldest", lua.LString(st.Oldest.Format("2006-01-02T15:04:05Z07:00")))
		}
		if !st.Newest.IsZero() {
			t.RawSetString("newest", lua.LString(st.Newest.Format("2006-01-02T15:04:05Z07:00")))
		}
		t.RawSetString("path", lua.LString(st.Path))
		L.Push(t)
		return 1
	}))

	e.k.RawSetString("log", logTbl)
}
