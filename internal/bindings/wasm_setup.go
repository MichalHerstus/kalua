//go:build js && wasm

// Package bindings provides the KALUA standard library WASM profile.
// This file registers only the bindings that work in the browser.
package bindings

import (
	"github.com/yuin/gopher-lua"

	"kalua/internal/common"
	"kalua/internal/vm"
)

// SetupWasm configures the Lua state for WASM mode (browser).
// This registers only the bindings that work in the browser environment.
func SetupWasm(L *lua.LState, app *vm.App, opts Options, sess common.SessionInterface, logger Logger) *Env {
	e := &Env{L: L, App: app, known: map[string]string{}, maxFileSize: opts.MaxFileSize, Sess: sess, Logger: logger, verbose: opts.Verbose}
	if e.maxFileSize <= 0 {
		e.maxFileSize = DefaultMaxFileSize
	}
	e.workdir = workdirOf(opts)
	e.allowFS = allowFSOf(opts)

	k := L.NewTable()
	L.SetGlobal("k", k)
	e.k = k

	// K.NULL sentinel
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

	// Register only WASM-compatible bindings
	registerFlowWasm(e)
	registerDebug(e)
	registerFormsWasm(e)
	registerControlsWasm(e)
	registerFilesWasm(e)
	registerParamWasm(e)
	registerHTTPWasm(e)
	registerClipboardWasm(e)
	registerPickFileWasm(e)
	registerSystemWasm(e)
	registerDBWasm(e)
	registerRelayClientWasm(e)
	registerExprFuncs(e)
	// Note: registerJSON, registerCrypto, registerXML, registerFormats, registerRows
	// are stubbed for WASM mode

	argsT := L.NewTable()
	for i, a := range opts.Args {
		argsT.RawSetInt(i+1, lua.LString(a))
	}
	L.SetGlobal("ARGS", argsT)

	// Seed Kalipso error globals (nil/"", until a binding fails).
	seedErrorGlobals(L, e)

	return e
}