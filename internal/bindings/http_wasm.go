//go:build js && wasm

// Package bindings implements the WASM profile for HTTP requests using fetch.
package bindings

import (
	"github.com/yuin/gopher-lua"
)

// registerHTTPWasm registers k.http_request for WASM mode using fetch (synchronous stub for M2).
func registerHTTPWasm(e *Env) {
	_ = e.L.GetGlobal("k").(*lua.LTable)

	// k.http_request(opts) -> {status, headers, body} (synchronous stub for M2)
	e.register("http_request", "flow", func(L *lua.LState) int {
		opts := L.CheckTable(1)

		// Extract options
		_ = opts.RawGetString("method")
		_ = opts.RawGetString("url")
		_ = opts.RawGetString("headers")
		_ = opts.RawGetString("body")
		_ = opts.RawGetString("timeout")

		// Return empty result for M2
		result := L.NewTable()
		result.RawSetString("status", lua.LNumber(0))
		result.RawSetString("headers", L.NewTable())
		result.RawSetString("body", lua.LString(""))
		L.Push(result)
		return 1
	})
}