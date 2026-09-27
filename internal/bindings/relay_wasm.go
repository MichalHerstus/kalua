//go:build js && wasm

// Package bindings implements the WASM relay client for connecting to the localhost relay.
package bindings

import (
	"encoding/json"
	"sync"
	"syscall/js"

	"github.com/yuin/gopher-lua"
)

// registerRelayClientWasm registers the JS relay client for WASM mode.
func registerRelayClientWasm(e *Env) {
	k := e.L.GetGlobal("k").(*lua.LTable)
	relayTbl := e.L.NewTable()

	// relay.connect(url) -> handle
	e.registerInTable(relayTbl, "connect", "relay", func(L *lua.LState) int {
		url := L.CheckString(1)

		// Create WebSocket connection to relay
		ws := js.Global().Get("WebSocket").New(url)
		handleID := js.Global().Get("Symbol").Call("for", "kalua_relay_handle").String()

		// Store the WebSocket in a map
		relayHandlesMu.Lock()
		relayHandles[handleID] = ws
		relayHandlesMu.Unlock()

		L.Push(lua.LString(handleID))
		return 1
	})

	// relay.call(handle, method, params) -> result
	e.registerInTable(relayTbl, "call", "relay", func(L *lua.LState) int {
		handleID := L.CheckString(1)
		method := L.CheckString(2)
		params := L.CheckTable(3)

		relayHandlesMu.Lock()
		ws, ok := relayHandles[handleID]
		relayHandlesMu.Unlock()

		if !ok {
			return e.fail(L, KErrorNotConnected, "relay handle not found: "+handleID)
		}

		// Convert params to map
		paramMap := map[string]interface{}{}
		params.ForEach(func(k, v lua.LValue) {
			paramMap[k.String()] = v.String()
		})

		// Create request
		requestID := js.Global().Get("Symbol").Call("for", "kalua_relay_request").String()
		request := map[string]interface{}{
			"id":     requestID,
			"type":   method,
			"params": paramMap,
		}

		requestJSON, _ := json.Marshal(request)
		ws.Call("send", string(requestJSON))

		// For M3, we'll use a simplified synchronous approach
		// In a full implementation, we'd yield and wait for response
		L.Push(lua.LNil)
		return 1
	})

	// relay.close(handle)
	e.registerInTable(relayTbl, "close", "relay", func(L *lua.LState) int {
		handleID := L.CheckString(1)

		relayHandlesMu.Lock()
		ws, ok := relayHandles[handleID]
		if ok {
			ws.Call("close")
			delete(relayHandles, handleID)
		}
		relayHandlesMu.Unlock()

		return 0
	})

	k.RawSetString("relay", relayTbl)
}

// relayHandles stores WebSocket connections to the relay
var relayHandles = make(map[string]js.Value)
var relayHandlesMu sync.Mutex