//go:build js && wasm

// Package bindings provides WASM stub for native registration.
package bindings

// registerNative is a no-op in WASM mode.
func registerNative(e *Env) {}