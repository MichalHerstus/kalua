//go:build js && wasm

// Package main (internal/wasm) — M5 "brain".
//
// WasmBrain is the thin WASM host for the client-protocol brain. The routing
// itself lives in common.RouteOutbox (pure, natively unit-tested); this wrapper
// owns the JS sink, the client-side component inventory (which Tabulator/Chart/
// Looper selectors are live, so lifecycle commands can target them) and the
// control/value model the hands consult.
//
// The JavaScript "hands" (wasm_assets/app.minimal.js) keep only DOM
// manipulation, event attachment, third-party component init and browser APIs.
package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"kalua/internal/common"
)

// WasmBrain routes session outbox messages into the compact hands vocabulary.
type WasmBrain struct {
	// sendCommand invokes the registered JS callback with a JSON object.
	sendCommand func(json string)
	// Live component inventory: selector -> component kind.
	componentMu sync.Mutex
	components  map[string]string
	// Control values reported by the browser (form -> ctrl -> value), used to
	// assemble click payloads without re-reading the DOM.
	valuesMu      sync.Mutex
	controlValues map[string]map[string]interface{}
}

// NewWasmBrain creates an empty brain.
func NewWasmBrain() *WasmBrain {
	return &WasmBrain{
		sendCommand:   func(_ string) {},
		components:    make(map[string]string),
		controlValues: make(map[string]map[string]interface{}),
	}
}

// SetSendCommand installs the JS command sink.
func (b *WasmBrain) SetSendCommand(fn func(json string)) {
	b.sendCommand = fn
}

// emit serializes a command map and routes it to the JS hands.
func (b *WasmBrain) emit(cmd common.WasmCmd) {
	data, err := json.Marshal(cmd)
	if err != nil {
		return
	}
	b.sendCommand(string(data))
}

// HandleOutbox routes one session outbox message into hand commands and keeps
// the component inventory in sync.
func (b *WasmBrain) HandleOutbox(msg common.OutboxMsg) {
	cmds := common.RouteOutbox(msg)
	for i := 0; i < len(cmds); i++ {
		b.emit(cmds[i])
	}
}

// RetainComponent records a live component so destroy-on-close can address it.
func (b *WasmBrain) RetainComponent(kind, selector string) {
	b.componentMu.Lock()
	defer b.componentMu.Unlock()
	b.components[selector] = kind
}

// ForgetComponentsOnForm drops all component selectors belonging to a form.
func (b *WasmBrain) ForgetComponentsOnForm(formName string) {
	b.componentMu.Lock()
	defer b.componentMu.Unlock()
	var kept map[string]string
	for sel, kind := range b.components {
		if !hasFormPrefix(sel, formName) {
			kept[sel] = kind
		}
	}
	b.components = kept
}

// ComponentKinds returns the live selectors per kind (for debugging/tests).
func (b *WasmBrain) ComponentKinds() map[string][]string {
	b.componentMu.Lock()
	defer b.componentMu.Unlock()
	out := make(map[string][]string)
	for sel, kind := range b.components {
		out[kind] = append(out[kind], sel)
	}
	return out
}

// hasFormPrefix reports whether a selector addresses controls of formName
// (selectors are "#c:<form>:<ctrl>" or "#f:<form>").
func hasFormPrefix(selector, formName string) bool {
	needle := "#c:" + formName + ":"
	if strings.HasPrefix(selector, needle) {
		return true
	}
	return strings.HasPrefix(selector, "#f:"+formName)
}

// SetControlValue records a control value reported by the browser.
func (b *WasmBrain) SetControlValue(form, ctrl string, value interface{}) {
	b.valuesMu.Lock()
	defer b.valuesMu.Unlock()
	formVals := b.controlValues[form]
	if formVals == nil {
		formVals = make(map[string]interface{})
		b.controlValues[form] = formVals
	}
	formVals[ctrl] = value
}

// ClearFormValues drops recorded values for a closed form.
func (b *WasmBrain) ClearFormValues(form string) {
	b.valuesMu.Lock()
	defer b.valuesMu.Unlock()
	delete(b.controlValues, form)
}

// ControlValues returns a copy of the recorded values for a form.
func (b *WasmBrain) ControlValues(form string) map[string]interface{} {
	b.valuesMu.Lock()
	defer b.valuesMu.Unlock()
	formVals := b.controlValues[form]
	out := make(map[string]interface{}, len(formVals))
	for k, v := range formVals {
		out[k] = v
	}
	return out
}

// String summarizes the brain for debugging.
func (b *WasmBrain) String() string {
	return fmt.Sprintf("WasmBrain{components=%v}", b.ComponentKinds())
}
