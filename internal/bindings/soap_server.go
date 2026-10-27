//go:build !wasm

package bindings

import (
	"sync/atomic"

	"github.com/yuin/gopher-lua"
)

// SOAPStats holds SOAP server statistics.
type SOAPStats struct {
	RequestsTotal atomic.Int64
	FaultsTotal   atomic.Int64
	LatencySumMs  atomic.Int64
	LatencyCount  atomic.Int64
}

// registerSOAPServer installs the Phase 4 SOAP server bindings in serve mode:
// k.soap_operations (declarative table), k.soap_fault(code, message) -> fault table,
// and k.soap_stats() -> {requests_total, faults_total, avg_latency_ms}.
// The actual SOAP endpoint is registered by the server when it starts.
// We just need to ensure the schema table and fault function are accessible.
func registerSOAPServer(e *Env, opts Options) {
	k := e.L.GetGlobal("k").(*lua.LTable)

	// k.soap_fault(code, message) -> {fault = {faultcode, faultstring}}
	k.RawSetString("soap_fault", e.L.NewFunction(func(L *lua.LState) int {
		code := L.CheckString(1)
		message := L.CheckString(2)
		fault := L.NewTable()
		fault.RawSetString("faultcode", lua.LString(code))
		fault.RawSetString("faultstring", lua.LString(message))
		L.Push(fault)
		return 1
	}))

	// k.soap_stats() -> {requests_total, faults_total, avg_latency_ms}
	// Reads from the server's SOAPStats (via Options) when available.
	k.RawSetString("soap_stats", e.L.NewFunction(func(L *lua.LState) int {
		tbl := L.NewTable()
		if opts.SOAPStats == nil {
			tbl.RawSetString("requests_total", lua.LNumber(0))
			tbl.RawSetString("faults_total", lua.LNumber(0))
			tbl.RawSetString("avg_latency_ms", lua.LNumber(0))
			L.Push(tbl)
			return 1
		}
		tbl.RawSetString("requests_total", lua.LNumber(opts.SOAPStats.RequestsTotal.Load()))
		tbl.RawSetString("faults_total", lua.LNumber(opts.SOAPStats.FaultsTotal.Load()))
		avg := 0.0
		if c := opts.SOAPStats.LatencyCount.Load(); c > 0 {
			avg = float64(opts.SOAPStats.LatencySumMs.Load()) / float64(c)
		}
		tbl.RawSetString("avg_latency_ms", lua.LNumber(avg))
		L.Push(tbl)
		return 1
	}))

	// The k.soap_operations table is declared by the script at the top level.
	// The server reads it during BuildOperations.
}