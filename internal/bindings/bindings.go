// Package bindings exposes the KALUA standard library to scripts. Nothing the
// host implements lives outside this package: any k.* call your script makes
// must exist in the registry below or the checker flags it.
//
// A binding is a plain gopher-lua LGFunction. Whenever it needs to block
// (k.sleep) or take control flow decisions (k.quit), it talks to the *vm.App
// in Env so the app scheduler keeps pumping the single coroutine.
package bindings

import (
	"github.com/yuin/gopher-lua"

	"kalua/internal/coerce"
	"kalua/internal/vm"
)

// Env carries the host context every binding acts on. Bindings receive the
// coroutine LState as their LGFunction first argument; Env surfaces the app
// scheduler reachable from helpers.
type Env struct {
	L   *lua.LState
	App *vm.App

	// k is the global table backing the k.* namespace (created in Setup).
	k *lua.LTable

	// known is this env's name→group registry of implemented k.* bindings.
	known map[string]string
}

// registerKnown tracks name→group across all envs. register() writes here so
// the checker's Known() always describes what is truly installed.
// Pre-populated with Phase 1 bindings so checker works before Setup runs.
var registerKnown = map[string]string{
	"print":                     "flow",
	"sleep":                     "flow",
	"quit":                      "flow",
	"error":                     "flow",
	"msgbox":                    "flow",
	"form":                      "forms", // namespace
	"form.new":                  "forms",
	"form.show":                 "forms",
	"form.close":                "forms",
	"form.return_to":            "forms",
	"form.clear":                "forms",
	"form.refresh":              "forms",
	"form.on":                   "forms",
	"ctrl":                      "controls", // namespace
	"ctrl.label":                "controls",
	"ctrl.textbox":              "controls",
	"ctrl.button":               "controls",
	"ctrl.combo":                "controls",
	"ctrl.list":                 "controls",
	"ctrl.table":                "controls",
	"ctrl.checkbox":             "controls",
	"ctrl.radio":                "controls",
	"ctrl.set_value":            "controls",
	"ctrl.get_value":            "controls",
	"ctrl.set_property":         "controls",
	"ctrl.get_property":         "controls",
	"ctrl.set_focus":            "controls",
	"ctrl.refresh":              "controls",
	"table":                     "controls", // namespace
	"table.add_line":            "controls",
	"table.delete_line":         "controls",
	"table.set_column_value":    "controls",
	"table.get_column_value":    "controls",
	"table.get_selected_column": "controls",
	"table.set_selected_column": "controls",
	"connect_db":                "database",
	"disconnect_db":             "database",
	"sql":                       "database",
	"db_select":                 "database",
	"db_insert":                 "database",
	"db_update":                 "database",
	"db_delete":                 "database",
	"tx_begin":                  "database",
	"tx_commit":                 "database",
	"tx_rollback":               "database",
	"rows":                      "database",
}

// register puts a k.* binding into the env's k namespace.
// Supports dotted names like "form.new" to create nested tables.
func (e *Env) register(name, group string, fn lua.LGFunction) {
	e.known[name] = group
	registerKnown[name] = group

	// Handle dotted names (e.g., "form.new" -> k.form.new)
	parts := split(name, ".")
	tbl := e.k
	for i, part := range parts {
		if i == len(parts)-1 {
			// Last part: set the function
			tbl.RawSetString(part, e.L.NewFunction(fn))
		} else {
			// Intermediate part: get or create sub-table
			next := tbl.RawGetString(part)
			if next == lua.LNil {
				next = e.L.NewTable()
				tbl.RawSetString(part, next)
			}
			tbl, _ = next.(*lua.LTable)
		}
	}
}

func split(s, sep string) []string {
	var result []string
	start := 0
	for i := 0; i <= len(s)-len(sep); i++ {
		if s[i:i+len(sep)] == sep {
			result = append(result, s[start:i])
			start = i + len(sep)
			i += len(sep) - 1
		}
	}
	result = append(result, s[start:])
	return result
}

// Known returns a copy of the k.* name registry (shared across envs) for the
// checker.
func Known() map[string]bool {
	m := make(map[string]bool, len(registerKnown))
	for name := range registerKnown {
		m[name] = true
	}
	return m
}

// Setup wires the k.* namespace, the K.* helpers and ARGS into a sandboxed
// state, then installs every implemented binding. args seeds the ARGS global
// table (in order, starting at 1). It must be called once per LState.
func Setup(L *lua.LState, app *vm.App, args []string) *Env {
	e := &Env{L: L, App: app, known: map[string]string{}}

	k := L.NewTable()
	L.SetGlobal("k", k)
	e.k = k

	K := L.NewTable()
	registerHelpers(e, K)
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
	registerForms(e)
	registerControls(e)
	registerDB(e)

	argsT := L.NewTable()
	for i, a := range args {
		argsT.RawSetInt(i+1, lua.LString(a))
	}
	L.SetGlobal("ARGS", argsT)

	return e
}

// lvalue converts a Go value (as produced by coerce helpers) into a Lua value.
func lvalue(v any) lua.LValue {
	switch t := v.(type) {
	case nil:
		return lua.LNil
	case bool:
		return lua.LBool(t)
	case string:
		return lua.LString(t)
	case float64:
		return lua.LNumber(t)
	case int:
		return lua.LNumber(t)
	case int64:
		return lua.LNumber(t)
	default:
		return lua.LString(coerce.Stringify(t))
	}
}

// v extracts a Go value from a Lua value for coerce processing.
func v(lv lua.LValue) any {
	switch lv.Type() {
	case lua.LTNil:
		return nil
	case lua.LTBool:
		return bool(lv.(lua.LBool))
	case lua.LTNumber:
		return float64(lv.(lua.LNumber))
	case lua.LTString:
		return string(lv.(lua.LString))
	default:
		return lv.String()
	}
}
