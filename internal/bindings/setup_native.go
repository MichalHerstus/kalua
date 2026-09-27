//go:build !wasm

// Package bindings provides the native Setup function.
package bindings

import (
	"github.com/yuin/gopher-lua"

	"kalua/internal/common"
	"kalua/internal/vm"
)

// Setup wires the k.* namespace, the K.* helpers and ARGS into a sandboxed
// state, then installs every implemented binding. opts.Args seeds the ARGS
// global table (in order, starting at 1). It must be called once per LState.
// sess is the session this env belongs to (for msgbox, clipboard, etc.); can be nil.
// logger is used for error logging; can be nil.
func Setup(L *lua.LState, app *vm.App, opts Options, sess common.SessionInterface, logger Logger) *Env {
	e := &Env{L: L, App: app, known: map[string]string{}, maxFileSize: opts.MaxFileSize, Sess: sess, Logger: logger, verbose: opts.Verbose}
	if e.maxFileSize <= 0 {
		e.maxFileSize = DefaultMaxFileSize
	}
	e.workdir = workdirOf(opts)
	e.allowFS = allowFSOf(opts)

	k := L.NewTable()
	L.SetGlobal("k", k)
	e.k = k

	// K.NULL sentinel: the only value k.json_parse/k.json_load produce for a
	// JSON null, and k.is_null's identity check.
	e.kNULL = L.NewTable()

	K := L.NewTable()
	registerHelpers(e, K)
	K.RawSetString("NULL", e.kNULL)
	K.RawSetString("is_null", L.NewFunction(e.isNull))
	L.SetGlobal("K", K)

	// CTRL(name) - accessor function for controls
	L.SetGlobal("CTRL", L.NewFunction(func(L *lua.LState) int {
		formName := L.CheckString(1)
		ctrlName := L.CheckString(2)

		formTbl := L.GetGlobal(formName)
		if formTbl == lua.LNil {
			L.Push(lua.LNil)
			return 1
		}
		tbl, ok := formTbl.(*lua.LTable)
		if !ok {
			L.Push(lua.LNil)
			return 1
		}

		controls := tbl.RawGetString("controls")
		if controls == lua.LNil {
			L.Push(lua.LNil)
			return 1
		}
		controlsTbl, ok := controls.(*lua.LTable)
		if !ok {
			L.Push(lua.LNil)
			return 1
		}

		ctrl := controlsTbl.RawGetString(ctrlName)
		L.Push(ctrl)
		return 1
	}))

	registerFlow(e)
	registerDebug(e)
	registerForms(e)
	registerControls(e)
	registerNative(e)
	registerJSON(e)
	registerCrypto(e)
	registerXML(e)
	registerExprFuncs(e)
	registerFormats(e)
	registerRows(e)

	argsT := L.NewTable()
	for i, a := range opts.Args {
		argsT.RawSetInt(i+1, lua.LString(a))
	}
	L.SetGlobal("ARGS", argsT)

	// Seed Kalipso error globals (nil/"", until a binding fails).
	seedErrorGlobals(L, e)

	return e
}