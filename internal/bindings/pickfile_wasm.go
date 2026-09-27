//go:build js && wasm

// Package bindings implements the WASM profile for file picker operations.
package bindings

import (
	"github.com/yuin/gopher-lua"
)

// registerPickFileWasm registers k.pick_file for WASM mode.
func registerPickFileWasm(e *Env) {
	_ = e.L.GetGlobal("k").(*lua.LTable)

	// k.pick_file(opts) -> files table (synchronous stub for M2)
	// opts: {mode="open"|"save"|"download", accept="...", multiple=bool, filename="...", data="base64"}
	e.register("pick_file", "flow", func(L *lua.LState) int {
		opts := L.OptTable(1, L.NewTable())

		mode := "open"
		if v := opts.RawGetString("mode"); v != lua.LNil {
			mode = v.String()
		}
		_ = opts.RawGetString("accept")
		_ = opts.RawGetString("multiple")
		_ = opts.RawGetString("filename")
		_ = opts.RawGetString("data")

		switch mode {
		case "open":
			L.Push(lua.LNil)
			return 1
		case "save", "download":
			L.Push(lua.LNil)
			return 1
		}

		return 0
	})
}