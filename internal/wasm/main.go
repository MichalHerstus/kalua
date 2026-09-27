//go:build js && wasm

// Package wasm provides the KALUA WASM entry point for running apps in the browser.
package main

import (
	"encoding/json"
	"syscall/js"

	"github.com/yuin/gopher-lua"

	"kalua/internal/bindings"
	"kalua/internal/common"
	"kalua/internal/session"
	"kalua/internal/vm"
)

// Global state for the WASM instance
var (
	app            *vm.App
	wasmBridge     *WasmBridge
	currentSession *session.Session
)

// wasmLogger is a no-op logger for WASM (logging goes to browser console via JS).
type wasmLogger struct{}

func (wasmLogger) Printf(format string, args ...interface{})  {}
func (wasmLogger) Errorf(format string, args ...interface{})  {}
func (wasmLogger) Warnf(format string, args ...interface{})   {}
func (wasmLogger) Tracef(format string, args ...interface{})  {}

// WasmBridge implements the common.Transport interface for WASM.
type WasmBridge struct {
	onMessageFunc js.Func
	sendChan      chan common.OutboxMsg
	recvChan      chan common.InboxMsg
	done          chan struct{}
}

func NewWasmBridge() *WasmBridge {
	return &WasmBridge{
		sendChan: make(chan common.OutboxMsg, 64),
		recvChan: make(chan common.InboxMsg, 64),
		done:     make(chan struct{}),
	}
}

// Send implements the Transport interface - sends outbox message to JS.
func (b *WasmBridge) Send(msg common.OutboxMsg) error {
	select {
	case b.sendChan <- msg:
		return nil
	case <-b.done:
		return nil
	}
}

// Recv implements the Transport interface - receives inbox message from JS.
func (b *WasmBridge) Recv() <-chan common.InboxMsg {
	return b.recvChan
}

func (b *WasmBridge) Close() error {
	close(b.done)
	if !b.onMessageFunc.IsUndefined() && b.onMessageFunc.Truthy() {
		b.onMessageFunc.Release()
	}
	return nil
}

// startApp is the main entry point called from JavaScript.
// It initializes the Lua VM, bindings, and session with the given script source.
func startApp(source string) js.Value {
	// Create Lua VM
	L := vm.New()
	app = vm.NewApp(L)

	// Setup bindings with WASM profile (no session yet)
	env := bindings.SetupWasm(L, app, bindings.Options{
		Args: []string{},
	}, nil, wasmLogger{})

	// Create WASM bridge
	wasmBridge = NewWasmBridge()

	// Execute the script to define main()
	chunkFn, err := vm.LoadSource(L, "app.lua", source)
	if err != nil {
		return js.ValueOf(map[string]interface{}{
			"error": err.Error(),
		})
	}

	if err := L.CallByParam(lua.P{
		Fn:      chunkFn,
		NRet:    0,
		Protect: true,
	}); err != nil {
		return js.ValueOf(map[string]interface{}{
			"error": err.Error(),
		})
	}

	// Get main function
	mainFn := L.GetGlobal("main")
	if mainFn == nil || mainFn.String() == "<nil>" {
		return js.ValueOf(map[string]interface{}{
			"error": "main function not found",
		})
	}

	mainLFn, ok := mainFn.(*lua.LFunction)
	if !ok {
		return js.ValueOf(map[string]interface{}{
			"error": "main is not a function",
		})
	}

	// Create session with WASM bridge
	sess, err := session.NewWithTransport("wasm-session", L, app, env, wasmBridge, wasmLogger{})
	if err != nil {
		return js.ValueOf(map[string]interface{}{
			"error": err.Error(),
		})
	}

	currentSession = sess
	env.Sess = sess
	app.SetSession(sess)

	// Start main in a coroutine
	go func() {
		err := app.Run(mainLFn)
		if err != nil && err != vm.ErrSuspended {
			// Send error to JS
			wasmBridge.Send(common.OutboxMsg{
				Type: "error",
				Msg:  err.Error(),
			})
		}
	}()

	// Start outbox pump
	go wasmBridge.pumpOutbox()

	return js.ValueOf(map[string]interface{}{
		"ok": true,
	})
}

// pumpOutbox sends outbox messages to JavaScript via the registered callback.
func (b *WasmBridge) pumpOutbox() {
	for {
		select {
		case msg := <-b.sendChan:
			if !b.onMessageFunc.IsUndefined() && b.onMessageFunc.Truthy() {
				// Convert OutboxMsg to JSON and call JS callback
				data, err := json.Marshal(msg)
				if err != nil {
					continue
				}
				jsVal := js.Global().Get("JSON").Call("parse", string(data))
				b.onMessageFunc.Invoke(jsVal)
			}
		case <-b.done:
			return
		}
	}
}

// PostInboxMessage is called from JavaScript to deliver an inbox message to the session.
func PostInboxMessage(msgJSON string) {
	if wasmBridge != nil {
		var msg common.InboxMsg
		if err := json.Unmarshal([]byte(msgJSON), &msg); err != nil {
			return
		}
		select {
		case wasmBridge.recvChan <- msg:
		case <-wasmBridge.done:
		}
	}
}

// RegisterCallbacks registers the JS callbacks for the bridge.
// Called from JavaScript after the WASM module loads.
func RegisterCallbacks(onMessage js.Value) {
	if wasmBridge != nil {
		wasmBridge.onMessageFunc = js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			// Convert []js.Value to []any for Invoke
			argsAny := make([]any, len(args))
			for i, v := range args {
				argsAny[i] = v
			}
			onMessage.Invoke(argsAny...)
			return nil
		})
	}
}

// Export functions to JavaScript
func main() {
	js.Global().Set("kaluaStartApp", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 {
			return map[string]interface{}{"error": "startApp requires source argument"}
		}
		source := args[0].String()
		return startApp(source)
	}))

	js.Global().Set("kaluaPostMessage", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 {
			return nil
		}
		msgJSON := args[0].String()
		PostInboxMessage(msgJSON)
		return nil
	}))

	js.Global().Set("kaluaRegisterCallbacks", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 {
			return nil
		}
		if args[0].Type() == js.TypeFunction {
			RegisterCallbacks(args[0])
		}
		return nil
	}))

	// Keep the WASM module alive
	<-make(chan struct{})
}