// Package vm owns the sandboxed gopher-lua LState and the Lua app runner.
//
// Sandboxing stays in this package; the public surface for scripts is the
// k.* bindings registered afterwards (internal/bindings).
package vm

import (
	"github.com/yuin/gopher-lua"
)

// removedBase are globals that OpenBase registers which are too powerful for a
// KALUA script: anything that reads/writes files, compiles/runs code, loads
// modules, or exposes the raw runtime/registry.
var removedBase = []string{
	"collectgarbage",
	"dofile",
	"getfenv",
	"load",
	"loadfile",
	"loadstring",
	"module",
	"newproxy",
	"print",
	"rawget",
	"rawset",
	"require",
	"setfenv",
}

// removedOS are os.* functions beyond the whitelisted read-only subset
// (os.clock, os.difftime, os.date, os.time).
var removedOS = []string{
	"execute",
	"exit",
	"getenv",
	"remove",
	"rename",
	"setenv",
	"setlocale",
	"tmpname",
}

// whitelisted libs opened by New in dependency-free order.
var whitelisted = []struct {
	name string
	fn   lua.LGFunction
}{
	{lua.BaseLibName, lua.OpenBase},
	{lua.TabLibName, lua.OpenTable},
	{lua.StringLibName, lua.OpenString},
	{lua.MathLibName, lua.OpenMath},
	{lua.OsLibName, lua.OpenOs},
}

// New returns a sandboxed LState. No io, no os.execute, no require, no debug
// hooks: everything the script can do is exposed later via k.* and the K
// helpers by internal/bindings.
func New() *lua.LState {
	L := lua.NewState(lua.Options{SkipOpenLibs: true})
	for _, lib := range whitelisted {
		_ = L.CallByParam(lua.P{Fn: L.NewFunction(lib.fn), NRet: 0, Protect: true}, lua.LString(lib.name))
	}
	for _, name := range removedBase {
		L.SetGlobal(name, lua.LNil)
	}
	if osmod, ok := L.GetGlobal("os").(*lua.LTable); ok {
		for _, name := range removedOS {
			osmod.RawSetString(name, lua.LNil)
		}
	}
	return L
}

// SandboxGlobals is the ordered whitelist of library globals a script may rely
// on: everything not listed here does not exist. Kept in one place so tests
// and docs stay in sync with New.
var SandboxGlobals = struct {
	Libs []string
}{
	Libs: []string{"_G", "_VERSION", "ipairs", "pairs", "assert", "error", "getmetatable", "next", "pcall", "rawequal", "select", "setmetatable", "tonumber", "tostring", "type", "unpack", "xpcall", "string", "table", "math", "os"},
}
