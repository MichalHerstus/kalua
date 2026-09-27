//go:build js && wasm

// Package bindings implements the WASM profile for forms and controls.
package bindings

import (
	"kalua/internal/common"

	"github.com/yuin/gopher-lua"
)

// registerFormsWasm registers k.form.* bindings for WASM mode.
func registerFormsWasm(e *Env) {
	// k.form.new(name, opts)
	e.register("form.new", "forms", func(L *lua.LState) int {
		name := L.CheckString(1)
		_ = L.OptTable(2, L.NewTable())

		// Create form table
		form := L.NewTable()
		form.RawSetString("name", lua.LString(name))
		form.RawSetString("controls", L.NewTable())
		form.RawSetString("handlers", L.NewTable())
		L.SetGlobal(name, form)

		L.Push(lua.LString(name))
		return 1
	})

	// k.form.show(name)
	e.register("form.show", "forms", func(L *lua.LState) int {
		name := L.CheckString(1)
		// In WASM, we send a render_form message via the session
		if e.Sess != nil {
			e.Sess.SendOutbox(common.OutboxMsg{
				Type: "render_form",
				Form: name,
			})
		}
		return 0
	})

	// k.form.close(name)
	e.register("form.close", "forms", func(L *lua.LState) int {
		name := L.CheckString(1)
		if e.Sess != nil {
			e.Sess.SendOutbox(common.OutboxMsg{
				Type: "close_form",
				Form: name,
			})
		}
		return 0
	})

	// k.form.on(form, ctrl, event, fn)
	e.register("form.on", "forms", func(L *lua.LState) int {
		formName := L.CheckString(1)
		ctrlName := L.CheckString(2)
		event := L.CheckString(3)
		fn := L.CheckFunction(4)

		formTbl := L.GetGlobal(formName)
		if formTbl == lua.LNil {
			return 0
		}
		tbl, ok := formTbl.(*lua.LTable)
		if !ok {
			return 0
		}

		handlers := tbl.RawGetString("handlers")
		if handlers == lua.LNil {
			handlers = L.NewTable()
			tbl.RawSetString("handlers", handlers)
		}
		handlersTbl, ok := handlers.(*lua.LTable)
		if !ok {
			return 0
		}

		ctrlHandlers := handlersTbl.RawGetString(ctrlName)
		if ctrlHandlers == lua.LNil {
			ctrlHandlers = L.NewTable()
			handlersTbl.RawSetString(ctrlName, ctrlHandlers)
		}
		ctrlHandlersTbl, ok := ctrlHandlers.(*lua.LTable)
		if !ok {
			return 0
		}

		ctrlHandlersTbl.RawSetString(event, fn)
		return 0
	})

	// k.form.clear(name)
	e.register("form.clear", "forms", func(L *lua.LState) int {
		name := L.CheckString(1)
		formTbl := L.GetGlobal(name)
		if formTbl == lua.LNil {
			return 0
		}
		tbl, ok := formTbl.(*lua.LTable)
		if !ok {
			return 0
		}
		tbl.RawSetString("controls", L.NewTable())
		tbl.RawSetString("handlers", L.NewTable())
		return 0
	})

	// k.form.refresh(name)
	e.register("form.refresh", "forms", func(L *lua.LState) int {
		name := L.CheckString(1)
		if e.Sess != nil {
			e.Sess.SendOutbox(common.OutboxMsg{
				Type: "render_form",
				Form: name,
			})
		}
		return 0
	})
}

// registerControlsWasm registers k.ctrl.* bindings for WASM mode.
func registerControlsWasm(e *Env) {
	// k.ctrl.label(form, name, opts)
	e.register("ctrl.label", "controls", func(L *lua.LState) int {
		return addControlWasm(L, e, "label")
	})

	// k.ctrl.textbox(form, name, opts)
	e.register("ctrl.textbox", "controls", func(L *lua.LState) int {
		return addControlWasm(L, e, "textbox")
	})

	// k.ctrl.button(form, name, opts)
	e.register("ctrl.button", "controls", func(L *lua.LState) int {
		return addControlWasm(L, e, "button")
	})

	// k.ctrl.combo(form, name, opts)
	e.register("ctrl.combo", "controls", func(L *lua.LState) int {
		return addControlWasm(L, e, "combo")
	})

	// k.ctrl.list(form, name, opts)
	e.register("ctrl.list", "controls", func(L *lua.LState) int {
		return addControlWasm(L, e, "list")
	})

	// k.ctrl.table(form, name, opts)
	e.register("ctrl.table", "controls", func(L *lua.LState) int {
		return addControlWasm(L, e, "table")
	})

	// k.ctrl.checkbox(form, name, opts)
	e.register("ctrl.checkbox", "controls", func(L *lua.LState) int {
		return addControlWasm(L, e, "checkbox")
	})

	// k.ctrl.radio(form, name, opts)
	e.register("ctrl.radio", "controls", func(L *lua.LState) int {
		return addControlWasm(L, e, "radio")
	})

	// k.ctrl.set_value(form, name, value)
	e.register("ctrl.set_value", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		ctrlName := L.CheckString(2)
		value := L.Get(3)

		formTbl := L.GetGlobal(formName)
		if formTbl == lua.LNil {
			return 0
		}
		tbl, ok := formTbl.(*lua.LTable)
		if !ok {
			return 0
		}
		controls := tbl.RawGetString("controls")
		if controls == lua.LNil {
			return 0
		}
		controlsTbl, ok := controls.(*lua.LTable)
		if !ok {
			return 0
		}
		ctrl := controlsTbl.RawGetString(ctrlName)
		if ctrl == lua.LNil {
			return 0
		}
		ctrlTbl, ok := ctrl.(*lua.LTable)
		if !ok {
			return 0
		}
		ctrlTbl.RawSetString("value", value)

		if e.Sess != nil {
			e.Sess.SendOutbox(common.OutboxMsg{
				Type:     "update_control",
				Form:     formName,
				Ctrl:     ctrlName,
				Selector: "#c:" + formName + ":" + ctrlName,
				HTML:     value.String(),
			})
		}
		return 0
	})

	// k.ctrl.get_value(form, name) -> value
	e.register("ctrl.get_value", "controls", func(L *lua.LState) int {
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
		if ctrl == lua.LNil {
			L.Push(lua.LNil)
			return 1
		}
		ctrlTbl, ok := ctrl.(*lua.LTable)
		if !ok {
			L.Push(lua.LNil)
			return 1
		}
		value := ctrlTbl.RawGetString("value")
		L.Push(value)
		return 1
	})
}

// addControlWasm adds a control to a form in WASM mode.
func addControlWasm(L *lua.LState, e *Env, ctrlType string) int {
	formName := L.CheckString(1)
	name := L.CheckString(2)
	opts := L.OptTable(3, L.NewTable())

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
		controls = L.NewTable()
		tbl.RawSetString("controls", controls)
	}
	controlsTbl, ok := controls.(*lua.LTable)
	if !ok {
		L.Push(lua.LNil)
		return 1
	}

	ctrl := L.NewTable()
	ctrl.RawSetString("type", lua.LString(ctrlType))
	ctrl.RawSetString("name", lua.LString(name))
	ctrl.RawSetString("value", lua.LNil)
	// Copy opts
	opts.ForEach(func(k, v lua.LValue) {
		ctrl.RawSet(k, v)
	})

	controlsTbl.RawSetString(name, ctrl)

	if e.Sess != nil {
		e.Sess.SendOutbox(common.OutboxMsg{
			Type: "render_form",
			Form: formName,
		})
	}

	L.Push(ctrl)
	return 1
}