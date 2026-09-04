package bindings

import (
	"github.com/yuin/gopher-lua"
)

// registerDebug installs the k.debug.* API (Tier 1 debugging). These give a
// script read-only VM introspection without requiring a patched VM: stack,
// locals, and upvalues of the current call frames. Enabled in both run and
// serve modes.
func registerDebug(e *Env) {
	// k.debug.stack() — return a table of the current call frames.
	// Each frame: {level, name, source, line, what, locals={...}}.
	e.register("debug.stack", "debug", func(L *lua.LState) int {
		tbl := L.NewTable()
		level := 0
		for {
			dbg, ok := L.GetStack(level)
			if !ok {
				break
			}
			_, _ = L.GetInfo("nSlu", dbg, lua.LNil)

			frame := L.NewTable()
			frame.RawSetString("level", lua.LNumber(level+1))
			frame.RawSetString("name", lua.LString(dbg.Name))
			frame.RawSetString("source", lua.LString(dbg.Source))
			frame.RawSetString("line", lua.LNumber(dbg.CurrentLine))
			frame.RawSetString("what", lua.LString(dbg.What))

			locals := L.NewTable()
			li := 1
			for {
				name, val := L.GetLocal(dbg, li)
				if name == "" {
					break
				}
				localTbl := L.NewTable()
				localTbl.RawSetString("name", lua.LString(name))
				localTbl.RawSetString("value", lua.LString(val.String()))
				locals.RawSetInt(li, localTbl)
				li++
			}
			frame.RawSetString("locals", locals)
			tbl.RawSetInt(level+1, frame)
			level++
		}
		L.Push(tbl)
		return 1
	})

	// k.debug.locals([level]) — return a table of local name → value at the
	// given frame level (1 = caller of k.debug.locals).
	e.register("debug.locals", "debug", func(L *lua.LState) int {
		level := L.OptInt(1, 1)
		tbl := L.NewTable()
		if dbg, ok := L.GetStack(level); ok {
			li := 1
			for {
				name, val := L.GetLocal(dbg, li)
				if name == "" {
					break
				}
				if val == lua.LNil {
					val = lua.LString("(nil)")
				}
				tbl.RawSetString(name, val)
				li++
			}
		}
		L.Push(tbl)
		return 1
	})

	// k.debug.trace([msg]) — if verbose tracing is enabled, log a message
	// through the host logger (a script-side trace anchor). Returns nothing.
	e.register("debug.trace", "debug", func(L *lua.LState) int {
		if e.verbose && e.Logger != nil {
			e.Logger.Tracef("k.debug.trace(%s)", L.Get(1).String())
		}
		return 0
	})
}
