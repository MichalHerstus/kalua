// Package bindings implements the K.* literal-value helpers (expression
// functions) and the Phase 1 flow bindings k.print/sleep/quit/error.
package bindings

import (
	"time"

	"github.com/yuin/gopher-lua"

	"kalua/internal/coerce"
)

// registerHelpers installs the K.* helpers per §2.3 and §5.9:
//
//	K.EQ, K.NEQ, K.ADD — operator name constants
//	K.eq, K.ne, K.add  — binary operators with Kalipso coercion
//	K.tonum, K.tostr   — coercion helpers
//	K.truthy           — condition test for If(...)
func registerHelpers(e *Env, K *lua.LTable) {
	K.RawSetString("EQ", lua.LString("="))
	K.RawSetString("NEQ", lua.LString("<>"))
	K.RawSetString("ADD", lua.LString("+"))

	// eq(a,b) — numeric when both coerce (with ""→0), else string compare
	K.RawSetString("eq", e.L.NewFunction(func(L *lua.LState) int {
		a := v(L.Get(1))
		b := v(L.Get(2))
		L.Push(lvalue(coerce.Eq(a, b)))
		return 1
	}))

	// ne(a,b) — negation of eq
	K.RawSetString("ne", e.L.NewFunction(func(L *lua.LState) int {
		a := v(L.Get(1))
		b := v(L.Get(2))
		L.Push(lvalue(coerce.Ne(a, b)))
		return 1
	}))

	// add(a,b) — Kalipso +: numeric if both coerce, else concat
	K.RawSetString("add", e.L.NewFunction(func(L *lua.LState) int {
		a := v(L.Get(1))
		b := v(L.Get(2))
		L.Push(lvalue(coerce.Add(a, b)))
		return 1
	}))

	// tonum(x) → number, or 0 when not numeric
	K.RawSetString("tonum", e.L.NewFunction(func(L *lua.LState) int {
		a := v(L.Get(1))
		if n, ok := coerce.ToNum(a); ok {
			L.Push(lua.LNumber(n))
		} else {
			L.Push(lua.LNumber(0))
		}
		return 1
	}))

	// tostr(x) → Kalipso string form
	K.RawSetString("tostr", e.L.NewFunction(func(L *lua.LState) int {
		a := v(L.Get(1))
		L.Push(lua.LString(coerce.Stringify(a)))
		return 1
	}))

	// truthy(x) → Kalipso condition test
	K.RawSetString("truthy", e.L.NewFunction(func(L *lua.LState) int {
		a := v(L.Get(1))
		L.Push(lua.LBool(coerce.Truthy(a)))
		return 1
	}))
}

// registerFlow installs the Phase 1 flow bindings: k.print, k.sleep, k.quit,
// k.error. All use the Env's App to interact with the scheduler.
func registerFlow(e *Env) {
	// k.print(...) → stdout / log sink (joins with \t like Lua print)
	e.register("print", "flow", func(L *lua.LState) int {
		n := L.GetTop()
		if n == 0 {
			return 0
		}
		var parts []string
		for i := 1; i <= n; i++ {
			parts = append(parts, L.Get(i).String())
		}
		// TODO: hook into structured logger; for now write to stdout
		println(join(parts, "\t"))
		return 0
	})

	// k.sleep(ms) — yields coroutine; resumes after ms milliseconds
	e.register("sleep", "flow", func(L *lua.LState) int {
		ms := L.ToInt64(1)
		if ms <= 0 {
			return 0
		}
		return e.App.ScheduleSleep(L, time.Duration(ms)*time.Millisecond)
	})

	// k.quit() — requests clean app termination at next scheduler tick
	e.register("quit", "flow", func(L *lua.LState) int {
		e.App.RequestQuit()
		return 0
	})

	// k.error(msg) — raises a deliberate Lua error
	e.register("error", "flow", func(L *lua.LState) int {
		msg := L.ToString(1)
		L.RaiseError("%s", msg)
		return 0
	})

	// k.msgbox(text[, kind]) — show message box (stub for now)
	e.register("msgbox", "flow", func(L *lua.LState) int {
		text := L.ToString(1)
		kind := L.OptString(2, "info")
		// TODO: actually show msgbox via session
		println("[MSGBOX][" + kind + "] " + text)
		return 0
	})
}

// join is strings.Join without the import for the single call site.
func join(elems []string, sep string) string {
	switch len(elems) {
	case 0:
		return ""
	case 1:
		return elems[0]
	}
	n := len(sep) * (len(elems) - 1)
	for _, s := range elems {
		n += len(s)
	}
	b := make([]byte, n)
	bp := copy(b, elems[0])
	for _, s := range elems[1:] {
		bp += copy(b[bp:], sep)
		bp += copy(b[bp:], s)
	}
	return string(b[:bp])
}
