//go:build !wasm

// Package bindings implements the form and control bindings.
package bindings

import (
	"encoding/json"

	"github.com/yuin/gopher-lua"

	"kalua/internal/common"
	"kalua/internal/vm"
)

// registerForms installs k.form.* bindings.
func registerForms(e *Env) {
	// k.form.new(name, options) - create a new form
	e.register("form.new", "forms", func(L *lua.LState) int {
		name := L.CheckString(1)
		opts := L.OptTable(2, L.NewTable())

		// Store form definition in Lua global. Layout opts (kforms_enhancements
		// §6): layout=vertical|grid, align=left|center|right, gap px, cells.
		layoutVal := opts.RawGetString("layout")
		layout := ""
		if layoutVal != lua.LNil {
			layout = layoutVal.String()
		}
		if layout == "" {
			layout = "vertical"
		}
		alignVal := opts.RawGetString("align")
		align := ""
		if alignVal != lua.LNil {
			align = alignVal.String()
		}
		if align == "" {
			align = "left"
		}
		formTbl := L.NewTable()
		formTbl.RawSetString("name", lua.LString(name))
		if v := opts.RawGetString("title"); v != lua.LNil {
			formTbl.RawSetString("title", lua.LString(v.String()))
		}
		formTbl.RawSetString("layout", lua.LString(layout))
		formTbl.RawSetString("align", lua.LString(align))
		gapVal := opts.RawGetString("gap")
		if gapVal != lua.LNil {
			formTbl.RawSetString("gap", gapVal)
		}
		if cells := opts.RawGetString("cells"); cells != lua.LNil {
			formTbl.RawSetString("cells", cells)
		}
		formTbl.RawSetString("controls", L.NewTable())
		formTbl.RawSetString("handlers", L.NewTable())
		L.SetGlobal(name, formTbl)

		return 0
	})

	// k.set_property(form, prop, value) - set a form-level property (title,
	// align, gap, or a dynamic styling prop: bg, color, font, font_size, style)
	// and re-render the whole form.
	e.register("set_property", "forms", func(L *lua.LState) int {
		formName := L.CheckString(1)
		prop := L.CheckString(2)
		value := L.Get(3)

		// Structural keys hold the form's control/handler registry and must
		// not be clobbered through the generic setter.
		switch prop {
		case "name", "controls", "handlers", "order":
			return e.fail(L, KErrorInvalidParam, "set_property: cannot set reserved form key: "+prop)
		}

		tbl := getForm(L, formName)
		if tbl == nil {
			return e.fail(L, KErrorInvalidParam, "set_property: form not found: "+formName)
		}
		tbl.RawSetString(prop, value)

		html := renderForm(L, formName)
		sendOutbox(e, common.OutboxMsg{
			Type: "render_form",
			Form: formName,
			HTML: html,
		})
		return 0
	})

	// k.get_property(form, prop) - read a form-level property; nil when unset.
	e.register("get_property", "forms", func(L *lua.LState) int {
		formName := L.CheckString(1)
		prop := L.CheckString(2)

		tbl := getForm(L, formName)
		if tbl == nil {
			L.Push(lua.LNil)
			return 1
		}
		L.Push(tbl.RawGetString(prop))
		return 1
	})

	// k.form.show(name, [options]) - show form (modal with gap, or normal; suspends caller)
// options: {modal=true|false, gap=number|{x=num,y=num}}
	e.register("form.show", "forms", func(L *lua.LState) int {
		name := L.CheckString(1)
		opts := L.OptTable(2, L.NewTable())

		// Parse modal option
		modalVal := opts.RawGetString("modal")
		modal := modalVal != lua.LNil && lua.LVAsBool(modalVal)

		// Parse gap option: number (applies to both) or table {x, y}
		gapVal := opts.RawGetString("gap")
		gapX, gapY := parseGap(gapVal)

		// Store modal/gap on form table for later reference
		formTbl := L.GetGlobal(name)
		if formTbl != lua.LNil {
			if ft, ok := formTbl.(*lua.LTable); ok {
				ft.RawSetString("modal", lua.LBool(modal))
				ft.RawSetString("gap_x", lua.LNumber(gapX))
				ft.RawSetString("gap_y", lua.LNumber(gapY))
			}
		}

		// Push form onto session stack
		sess := e.App.Session()
		if sess != nil {
			sess.PushForm(name)
			// Store the suspended coroutine so it can be resumed when form closes
			sess.StoreFormCoro(name, L)
		}

		// Fire open_form event before showing
		if sess != nil {
			sess.PostFormEvent(name, "open_form")
		}

		// Render form and send to browser
		html := renderForm(L, name)
		sendOutbox(e, common.OutboxMsg{
			Type:  "render_form",
			Form:  name,
			HTML:  html,
			Modal: modal,
			GapX:  gapX,
			GapY:  gapY,
		})

		// Fire after_open_form event after rendering
		if sess != nil {
			sess.PostFormEvent(name, "after_open_form")
		}

		// Suspend the coroutine until form is closed
		return e.App.Block(L, &vm.PendingOp{Kind: vm.PendingFormShow, Form: name})
	})

	// k.form.close([name]) - close form
	e.register("form.close", "forms", func(L *lua.LState) int {
		name := L.OptString(1, "")

		sess := e.App.Session()
		if sess != nil {
			if name == "" {
				name = sess.TopForm()
			}
			sess.PopForm()

			// Fire close_form event
			sess.PostFormEvent(name, "close_form")

			// Resume the suspended coroutine for this form
			sess.ResumeFormCoro(name)
		}

		sendOutbox(e, common.OutboxMsg{
			Type: "close_form",
			Form: name,
		})
		emitChartDestroys(e, L, name)

		return 0
	})

	// k.form.return_to(name) - close all forms above name
	e.register("form.return_to", "forms", func(L *lua.LState) int {
		name := L.CheckString(1)

		sess := e.App.Session()
		if sess != nil {
			for sess.TopForm() != name && sess.TopForm() != "" {
				closed := sess.PopForm()
				// Fire close_form event for each closed form
				sess.PostFormEvent(closed, "close_form")
				// Resume the suspended coroutine for each closed form
				sess.ResumeFormCoro(closed)
				sendOutbox(e, common.OutboxMsg{
					Type: "close_form",
					Form: closed,
				})
				emitChartDestroys(e, L, closed)
			}
		}

		return 0
	})

	// k.form.clear(name) - clear form values
	e.register("form.clear", "forms", func(L *lua.LState) int {
		_ = L.CheckString(1)
		// TODO: clear form control values
		return 0
	})

	// k.form.refresh(name) - refresh form
	e.register("form.refresh", "forms", func(L *lua.LState) int {
		name := L.CheckString(1)
		html := renderForm(L, name)
		sendOutbox(e, common.OutboxMsg{
			Type: "render_form",
			Form: name,
			HTML: html,
		})
		return 0
	})

	// k.form.on(form, ctrl, event, fn) - register control event handler
	// k.form.on(name, event, fn) - register form-level event handler (3 args)
	// k.form.on(name, "on_idle", ms, fn) - register form-level on_idle with interval (4 args, 3rd is number)
	e.register("form.on", "forms", func(L *lua.LState) int {
		top := L.GetTop()

		// 3-arg form: form.on(name, event, fn) - form-level handler
		if top == 3 {
			formName := L.CheckString(1)
			eventName := L.CheckString(2)
			fn := L.CheckFunction(3)

			formTbl := L.GetGlobal(formName)
			if formTbl == lua.LNil {
				L.RaiseError("form %s not found", formName)
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

			// Store under @form key for form-level events
			formHandlers := handlersTbl.RawGetString("@form")
			if formHandlers == lua.LNil {
				formHandlers = L.NewTable()
				handlersTbl.RawSetString("@form", formHandlers)
			}
			formHandlersTbl, ok := formHandlers.(*lua.LTable)
			if !ok {
				return 0
			}

			formHandlersTbl.RawSetString(eventName, fn)

			// If this is on_idle, store the interval
			if eventName == "on_idle" {
				tbl.RawSetString("idle_ms", lua.LNumber(1000))
			}
			return 0
		}

		// 4-arg form: check if 3rd arg is number (on_idle with interval) or string (control event)
		if top == 4 {
			formName := L.CheckString(1)
			arg3 := L.Get(3)
			fn := L.CheckFunction(4)

			// If 3rd arg is a number, it's: form.on(name, "on_idle", ms, fn)
			if _, ok := arg3.(lua.LNumber); ok {
				eventName := L.CheckString(2)
				ms := L.CheckInt(3)

				formTbl := L.GetGlobal(formName)
				if formTbl == lua.LNil {
					L.RaiseError("form %s not found", formName)
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

				formHandlers := handlersTbl.RawGetString("@form")
				if formHandlers == lua.LNil {
					formHandlers = L.NewTable()
					handlersTbl.RawSetString("@form", formHandlers)
				}
				formHandlersTbl, ok := formHandlers.(*lua.LTable)
				if !ok {
					return 0
				}

				formHandlersTbl.RawSetString(eventName, fn)
				tbl.RawSetString("idle_ms", lua.LNumber(ms))
				return 0
			}
		}

		// 4-arg form: form.on(form, ctrl, event, fn) - control handler (original)
		formName := L.CheckString(1)
		ctrlName := L.CheckString(2)
		eventName := L.CheckString(3)
		fn := L.CheckFunction(4)

		formTbl := L.GetGlobal(formName)
		if formTbl == lua.LNil {
			L.RaiseError("form %s not found", formName)
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
		ctrlTbl, ok := ctrlHandlers.(*lua.LTable)
		if !ok {
			return 0
		}

		ctrlTbl.RawSetString(eventName, fn)
		return 0
	})
}

// registerControls installs k.ctrl.* bindings.
func registerControls(e *Env) {
	// k.ctrl.label(form, name, options)
	e.register("ctrl.label", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)
		opts := L.OptTable(3, L.NewTable())
		addControl(L, formName, name, "label", opts)
		return 0
	})

	// k.ctrl.textbox(form, name, options)
	e.register("ctrl.textbox", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)
		opts := L.OptTable(3, L.NewTable())
		addControl(L, formName, name, "textbox", opts)
		return 0
	})

	// k.ctrl.button(form, name, options)
	e.register("ctrl.button", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)
		opts := L.OptTable(3, L.NewTable())
		addControl(L, formName, name, "button", opts)
		return 0
	})

	// k.ctrl.combo(form, name, options)
	e.register("ctrl.combo", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)
		opts := L.OptTable(3, L.NewTable())
		addControl(L, formName, name, "combo", opts)
		return 0
	})

	// k.ctrl.list(form, name, options)
	e.register("ctrl.list", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)
		opts := L.OptTable(3, L.NewTable())
		addControl(L, formName, name, "list", opts)
		return 0
	})

	// k.ctrl.table(form, name, options)
	e.register("ctrl.table", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)
		opts := L.OptTable(3, L.NewTable())
		addControl(L, formName, name, "table", opts)
		return 0
	})

	// k.ctrl.looper(form, name, options)
	e.register("ctrl.looper", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)
		opts := L.OptTable(3, L.NewTable())
		addControl(L, formName, name, "looper", opts)
		return 0
	})

	// k.ctrl.grid(form, name, options)
	// CRUD grid widget (kforms_enhancements.md §7): DB-linked Tabulator table
	// with row/global actions, selection, and an optional detail/edit form.
	e.register("ctrl.grid", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)
		opts := L.OptTable(3, L.NewTable())
		addControl(L, formName, name, "grid", opts)
		return 0
	})

	// k.ctrl.chart(form, name, options)
	e.register("ctrl.chart", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)
		opts := L.OptTable(3, L.NewTable())
		addControl(L, formName, name, "chart", opts)
		return 0
	})

	// k.ctrl.image(form, name, options)
	e.register("ctrl.image", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)
		opts := L.OptTable(3, L.NewTable())
		addControl(L, formName, name, "image", opts)
		return 0
	})

	// k.ctrl.checkbox(form, name, options)
	e.register("ctrl.checkbox", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)
		opts := L.OptTable(3, L.NewTable())
		addControl(L, formName, name, "checkbox", opts)
		return 0
	})

	// k.ctrl.radio(form, name, options)
	e.register("ctrl.radio", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)
		opts := L.OptTable(3, L.NewTable())
		addControl(L, formName, name, "radio", opts)
		return 0
	})

	// k.table.add_line(form, name, values)
	e.register("table.add_line", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)
		values := L.CheckTable(3)

		ctrl := getControl(L, formName, name)
		if ctrl == nil {
			return 0
		}
		if ctrl.RawGetString("type").String() != "table" {
			L.RaiseError("control %s is not a table", name)
			return 0
		}

		rows := ctrl.RawGetString("rows")
		if rows == lua.LNil {
			rows = L.NewTable()
			ctrl.RawSetString("rows", rows)
		}
		rowsTbl, ok := rows.(*lua.LTable)
		if !ok {
			return 0
		}

		rowIdx := rowsTbl.Len() + 1
		rowTbl := L.NewTable()
		values.ForEach(func(k, v lua.LValue) {
			rowTbl.RawSet(k, v)
		})
		rowsTbl.RawSetInt(rowIdx, rowTbl)

		// Re-render and send update
		html := renderControl(ctrl)
		sendOutbox(e, common.OutboxMsg{
			Type:     "update_control",
			Form:     formName,
			Ctrl:     name,
			Selector: "#c:" + formName + ":" + name,
			HTML:     html,
		})
		return 0
	})

	// k.table.delete_line(form, name, index)
	e.register("table.delete_line", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)
		index := L.CheckInt(3)

		ctrl := getControl(L, formName, name)
		if ctrl == nil {
			return 0
		}
		if ctrl.RawGetString("type").String() != "table" {
			return 0
		}

		rows := ctrl.RawGetString("rows")
		if rows == lua.LNil {
			return 0
		}
		rowsTbl, ok := rows.(*lua.LTable)
		if !ok {
			return 0
		}

		rowsTbl.RawSetInt(index, lua.LNil)

		// Re-render and send update
		html := renderControl(ctrl)
		sendOutbox(e, common.OutboxMsg{
			Type:     "update_control",
			Form:     formName,
			Ctrl:     name,
			Selector: "#c:" + formName + ":" + name,
			HTML:     html,
		})
		return 0
	})

	// k.table.set_column_value(form, name, row, column, value)
	e.register("table.set_column_value", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)
		row := L.CheckInt(3)
		column := L.CheckString(4)
		value := L.Get(5)

		ctrl := getControl(L, formName, name)
		if ctrl == nil {
			return 0
		}
		if ctrl.RawGetString("type").String() != "table" {
			return 0
		}

		rows := ctrl.RawGetString("rows")
		if rows == lua.LNil {
			return 0
		}
		rowsTbl, ok := rows.(*lua.LTable)
		if !ok {
			return 0
		}

		rowTbl := rowsTbl.RawGetInt(row)
		if rowTbl == lua.LNil {
			return 0
		}
		rowTbl2, ok := rowTbl.(*lua.LTable)
		if !ok {
			return 0
		}
		rowTbl2.RawSetString(column, value)

		// Re-render and send update
		html := renderControl(ctrl)
		sendOutbox(e, common.OutboxMsg{
			Type:     "update_control",
			Form:     formName,
			Ctrl:     name,
			Selector: "#c:" + formName + ":" + name,
			HTML:     html,
		})
		return 0
	})

	// k.table.get_column_value(form, name, row, column)
	e.register("table.get_column_value", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)
		row := L.CheckInt(3)
		column := L.CheckString(4)

		ctrl := getControl(L, formName, name)
		if ctrl == nil {
			L.Push(lua.LNil)
			return 1
		}
		if ctrl.RawGetString("type").String() != "table" {
			L.Push(lua.LNil)
			return 1
		}

		rows := ctrl.RawGetString("rows")
		if rows == lua.LNil {
			L.Push(lua.LNil)
			return 1
		}
		rowsTbl, ok := rows.(*lua.LTable)
		if !ok {
			L.Push(lua.LNil)
			return 1
		}

		rowTbl := rowsTbl.RawGetInt(row)
		if rowTbl == lua.LNil {
			L.Push(lua.LNil)
			return 1
		}
		rowTbl2, ok := rowTbl.(*lua.LTable)
		if !ok {
			L.Push(lua.LNil)
			return 1
		}
		L.Push(rowTbl2.RawGetString(column))
		return 1
	})

	// k.table.get_selected_column(form, name)
	e.register("table.get_selected_column", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)

		ctrl := getControl(L, formName, name)
		if ctrl == nil {
			L.Push(lua.LNil)
			return 1
		}
		L.Push(ctrl.RawGetString("selected_column"))
		return 1
	})

	// k.table.set_selected_column(form, name, column)
	e.register("table.set_selected_column", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)
		column := L.CheckString(3)

		ctrl := getControl(L, formName, name)
		if ctrl == nil {
			return 0
		}
		ctrl.RawSetString("selected_column", lua.LString(column))
		return 0
	})

	// k.ctrl.set_value(form, name, value). For image controls the value maps to
	// the src attribute (spec §4.3 Dynamic Update).
	e.register("ctrl.set_value", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)
		value := L.Get(3)

		ctrl := getControl(L, formName, name)
		if ctrl == nil {
			return 0
		}
		ctrl.RawSetString("value", value)
		if ctrl.RawGetString("type").String() == "image" {
			ctrl.RawSetString("src", value)
		}

		// Re-render and send update
		html := renderControl(ctrl)
		sendOutbox(e, common.OutboxMsg{
			Type:     "update_control",
			Form:     formName,
			Ctrl:     name,
			Selector: "#c:" + formName + ":" + name,
			HTML:     html,
		})
		return 0
	})

	// k.ctrl.get_value(form, name)
	e.register("ctrl.get_value", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)

		ctrl := getControl(L, formName, name)
		if ctrl == nil {
			L.Push(lua.LNil)
			return 1
		}
		if ctrl.RawGetString("type").String() == "image" {
			if v := ctrl.RawGetString("src"); v != lua.LNil {
				L.Push(v)
				return 1
			}
		}
		L.Push(ctrl.RawGetString("value"))
		return 1
	})

	// k.ctrl.set_property(form, name, prop, value)
	e.register("ctrl.set_property", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)
		prop := L.CheckString(3)
		value := L.Get(4)

		ctrl := getControl(L, formName, name)
		if ctrl == nil {
			return 0
		}

		// Moving a control to a different grid cell requires a full form
		// re-render so it is placed inside the target cell container.
		if prop == "cell" {
			ctrl.RawSetString(prop, value)
			html := renderForm(L, formName)
			sendOutbox(e, common.OutboxMsg{
				Type: "render_form",
				Form: formName,
				HTML: html,
			})
			return 0
		}
		ctrl.RawSetString(prop, value)

		// Re-render and send update
		html := renderControl(ctrl)
		sendOutbox(e, common.OutboxMsg{
			Type:     "update_control",
			Form:     formName,
			Ctrl:     name,
			Selector: "#c:" + formName + ":" + name,
			HTML:     html,
		})
		return 0
	})

	// k.ctrl.get_property(form, name, prop)
	e.register("ctrl.get_property", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)
		prop := L.CheckString(3)

		ctrl := getControl(L, formName, name)
		if ctrl == nil {
			L.Push(lua.LNil)
			return 1
		}
		L.Push(ctrl.RawGetString(prop))
		return 1
	})

	// k.ctrl.set_focus(form, name)
	e.register("ctrl.set_focus", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)

		sendOutbox(e, common.OutboxMsg{
			Type: "focus",
			Form: formName,
			Ctrl: name,
		})
		return 0
	})

	// k.ctrl.refresh(form, name)
	e.register("ctrl.refresh", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)

		ctrl := getControl(L, formName, name)
		if ctrl == nil {
			return 0
		}

		html := renderControl(ctrl)
		sendOutbox(e, common.OutboxMsg{
			Type:     "update_control",
			Form:     formName,
			Ctrl:     name,
			Selector: "#c:" + formName + ":" + name,
			HTML:     html,
		})
		return 0
	})

	// k.ctrl.select_text(form, name) - select all text in a textbox/textarea
	e.register("ctrl.select_text", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)

		ctrl := getControl(L, formName, name)
		if ctrl == nil {
			return 0
		}

		sendOutbox(e, common.OutboxMsg{
			Type: "select_text",
			Form: formName,
			Ctrl: name,
		})
		return 0
	})

	// k.ctrl.set_selection(form, name, from, to) - set selection range in a textbox/textarea
	e.register("ctrl.set_selection", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)
		from := L.CheckInt(3)
		to := L.CheckInt(4)

		ctrl := getControl(L, formName, name)
		if ctrl == nil {
			return 0
		}

		// Send selection range as JSON in Data field (OutboxMsg has no int fields)
		data := map[string]interface{}{
			"from": from,
			"to":   to,
		}
		jsonData, _ := json.Marshal(data)
		sendOutbox(e, common.OutboxMsg{
			Type: "select_range",
			Form: formName,
			Ctrl: name,
			Data: string(jsonData),
		})
		return 0
	})

	// k.ctrl.get_selection(form, name) -> {start, end, text} - get current selection
	e.register("ctrl.get_selection", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)

		ctrl := getControl(L, formName, name)
		if ctrl == nil {
			L.Push(lua.LNil)
			return 1
		}

		sess := e.App.Session()
		if sess == nil {
			L.Push(lua.LNil)
			return 1
		}

		// Request selection from browser and suspend
		respID := sess.RequestSelection(L, nil, formName, name)
		_ = respID
		return L.Yield(lua.LNil)
	})

	// k.ctrl.get_item_count(form, name) -> number - get item count for combo/list/radio/table
	e.register("ctrl.get_item_count", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)

		ctrl := getControl(L, formName, name)
		if ctrl == nil {
			L.Push(lua.LNumber(0))
			return 1
		}

		ctrlType := ctrl.RawGetString("type").String()
		count := 0

		switch ctrlType {
		case "combo", "list", "radio":
			items := ctrl.RawGetString("items")
			if itemsTbl, ok := items.(*lua.LTable); ok {
				count = itemsTbl.Len()
			}
		case "table":
			// Traditional table: rows; Tabulator: data
			rows := ctrl.RawGetString("rows")
			if rowsTbl, ok := rows.(*lua.LTable); ok {
				count = rowsTbl.Len()
			} else {
				data := ctrl.RawGetString("data")
				if dataTbl, ok := data.(*lua.LTable); ok {
					count = dataTbl.Len()
				}
			}
		}

		L.Push(lua.LNumber(count))
		return 1
	})

	// k.ctrl.execute_event(form, name, event) - fire a control's event handler as if user triggered it
	e.register("ctrl.execute_event", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)
		event := L.CheckString(3)

		ctrl := getControl(L, formName, name)
		if ctrl == nil {
			return 0
		}

		// Get current control value to pass to handler
		value := ctrl.RawGetString("value")
		if ctrl.RawGetString("type").String() == "image" {
			if v := ctrl.RawGetString("src"); v != lua.LNil {
				value = v
			}
		}

		sess := e.App.Session()
		if sess != nil {
			// Post event to session inbox - handler runs asynchronously
			sess.PostEvent(formName, name, event, value)
		}
		return 0
	})

	// Tabulator table data operations
	registerTableOps(e)

	// Looper data operations (DB-linked loopers)
	registerLooperOps(e)

	// Chart data operations (Chart.js)
	registerChartOps(e)

	// CRUD grid data operations (Tabulator + forms)
	registerGridOps(e)
}

// addControl adds a control to a form definition.
func addControl(L *lua.LState, formName, name, ctrlType string, opts *lua.LTable) {
	formTbl := L.GetGlobal(formName)
	if formTbl == lua.LNil {
		L.RaiseError("form %s not found", formName)
		return
	}
	tbl, ok := formTbl.(*lua.LTable)
	if !ok {
		return
	}

	controls := tbl.RawGetString("controls")
	if controls == lua.LNil || controls == nil {
		controls = L.NewTable()
		tbl.RawSetString("controls", controls)
		controls = tbl.RawGetString("controls")
	}
	controlsTbl, ok := controls.(*lua.LTable)
	if !ok {
		return
	}

	// Track control order
	order := tbl.RawGetString("order")
	if order == lua.LNil || order == nil {
		order = L.NewTable()
		tbl.RawSetString("order", order)
		order = tbl.RawGetString("order")
	}
	orderTbl, ok := order.(*lua.LTable)
	if !ok {
		return
	}
	orderTbl.RawSetInt(orderTbl.Len()+1, lua.LString(name))

	ctrlTbl := L.NewTable()
	ctrlTbl.RawSetString("name", lua.LString(name))
	ctrlTbl.RawSetString("type", lua.LString(ctrlType))
	ctrlTbl.RawSetString("form", lua.LString(formName))

	// Label control uses "text" option, others use "label". "label" is also
	// accepted as a fallback so builder-authored forms that set label="{...}"
	// render their text. Only set when the source option exists so an absent
	// label stays nil (renders empty, and k.ctrl.get_property returns nil
	// rather than the string "nil").
	if ctrlType == "label" {
		if v := opts.RawGetString("text"); v != lua.LNil {
			ctrlTbl.RawSetString("label", lua.LString(v.String()))
		} else if v := opts.RawGetString("label"); v != lua.LNil {
			ctrlTbl.RawSetString("label", lua.LString(v.String()))
		}
	} else {
		if v := opts.RawGetString("label"); v != lua.LNil {
			ctrlTbl.RawSetString("label", lua.LString(v.String()))
		}
	}

	ctrlTbl.RawSetString("value", opts.RawGetString("value"))
	ctrlTbl.RawSetString("enabled", opts.RawGetString("enabled"))
	ctrlTbl.RawSetString("visible", opts.RawGetString("visible"))
	ctrlTbl.RawSetString("class", opts.RawGetString("class"))
	ctrlTbl.RawSetString("items", opts.RawGetString("items"))
	ctrlTbl.RawSetString("hidden_value", opts.RawGetString("hidden_value"))

	// Layout properties (kforms_enhancements §6): assign the control to a grid
	// cell (cell = cell_id) and/or override alignment left|center|right.
	ctrlTbl.RawSetString("cell", opts.RawGetString("cell"))
	ctrlTbl.RawSetString("align", opts.RawGetString("align"))

	// For button with onclick, register as click handler. Image controls are
	// clickable too (clickable=true option) and share the same handler table.
	if ctrlType == "button" || ctrlType == "image" {
		onclick := opts.RawGetString("onclick")
		if onclick != lua.LNil {
			if lfn, ok := onclick.(*lua.LFunction); ok {
				// Register handler for click event
				handlers := tbl.RawGetString("handlers")
				if handlers == lua.LNil {
					handlers = L.NewTable()
					tbl.RawSetString("handlers", handlers)
				}
				handlersTbl, ok := handlers.(*lua.LTable)
				if ok {
					ctrlHandlers := handlersTbl.RawGetString(name)
					if ctrlHandlers == lua.LNil {
						ctrlHandlers = L.NewTable()
						handlersTbl.RawSetString(name, ctrlHandlers)
					}
					ctrlHandlersTbl, ok := ctrlHandlers.(*lua.LTable)
					if ok {
						ctrlHandlersTbl.RawSetString("click", lfn)
					}
				}
			}
		}
	}

	// Tabulator options for table control
	if ctrlType == "table" {
		ctrlTbl.RawSetString("tabulator", opts.RawGetString("tabulator"))
		ctrlTbl.RawSetString("tabulatorOptions", opts.RawGetString("tabulatorOptions"))
		ctrlTbl.RawSetString("columns", opts.RawGetString("columns"))
		ctrlTbl.RawSetString("data", opts.RawGetString("data"))

		// DB-linked table options (Kalipso "connect to DB" parity)
		ctrlTbl.RawSetString("db", opts.RawGetString("db"))
		ctrlTbl.RawSetString("query", opts.RawGetString("query"))
		ctrlTbl.RawSetString("db_columns", opts.RawGetString("db_columns"))
		ctrlTbl.RawSetString("page_size", opts.RawGetString("page_size"))
		ctrlTbl.RawSetString("count_query", opts.RawGetString("count_query"))
		ctrlTbl.RawSetString("db_where", opts.RawGetString("where"))
		ctrlTbl.RawSetString("db_order_by", opts.RawGetString("order_by"))
	}

	// DB-linked looper options (Kalipso "connect to DB" parity). `row` holds the
	// row-template control defs (Phase 4 looper row-template controls); `columns`
	// is the grid column count for the row layout.
	if ctrlType == "looper" {
		ctrlTbl.RawSetString("db", opts.RawGetString("db"))
		ctrlTbl.RawSetString("query", opts.RawGetString("query"))
		ctrlTbl.RawSetString("links", opts.RawGetString("links"))
		ctrlTbl.RawSetString("page_size", opts.RawGetString("page_size"))
		ctrlTbl.RawSetString("count_query", opts.RawGetString("count_query"))
		ctrlTbl.RawSetString("db_where", opts.RawGetString("where"))
		ctrlTbl.RawSetString("db_order_by", opts.RawGetString("order_by"))
		ctrlTbl.RawSetString("columns", opts.RawGetString("columns"))
		ctrlTbl.RawSetString("row", opts.RawGetString("row"))
	}

	// CRUD grid options (kforms_enhancements.md §7). Tabulator output is implicit;
	// the control adds pk_field, action config, selection mode and the optional
	// detail/edit form (referenced by name or declared inline).
	if ctrlType == "grid" {
		ctrlTbl.RawSetString("db", opts.RawGetString("db"))
		ctrlTbl.RawSetString("query", opts.RawGetString("query"))
		ctrlTbl.RawSetString("columns", opts.RawGetString("columns"))
		ctrlTbl.RawSetString("data", opts.RawGetString("data"))
		ctrlTbl.RawSetString("tabulatorOptions", opts.RawGetString("tabulatorOptions"))
		ctrlTbl.RawSetString("page_size", opts.RawGetString("page_size"))
		ctrlTbl.RawSetString("count_query", opts.RawGetString("count_query"))
		ctrlTbl.RawSetString("db_where", opts.RawGetString("where"))
		ctrlTbl.RawSetString("db_order_by", opts.RawGetString("order_by"))
		ctrlTbl.RawSetString("pk_field", opts.RawGetString("pk_field"))
		ctrlTbl.RawSetString("row_actions", opts.RawGetString("row_actions"))
		ctrlTbl.RawSetString("global_actions", opts.RawGetString("global_actions"))
		ctrlTbl.RawSetString("selection_mode", opts.RawGetString("selection_mode"))
		ctrlTbl.RawSetString("row_click_action", opts.RawGetString("row_click_action"))
		ctrlTbl.RawSetString("column_visibility", opts.RawGetString("column_visibility"))
		ctrlTbl.RawSetString("form", opts.RawGetString("form"))
	}

	// Chart options (Chart.js)
	if ctrlType == "chart" {
		ctrlTbl.RawSetString("chart_type", opts.RawGetString("type"))
		ctrlTbl.RawSetString("labels", opts.RawGetString("labels"))
		ctrlTbl.RawSetString("datasets", opts.RawGetString("datasets"))
		ctrlTbl.RawSetString("options", opts.RawGetString("options"))
		ctrlTbl.RawSetString("width", opts.RawGetString("width"))
		ctrlTbl.RawSetString("height", opts.RawGetString("height"))
		ctrlTbl.RawSetString("responsive", opts.RawGetString("responsive"))
		ctrlTbl.RawSetString("maintainAspectRatio", opts.RawGetString("maintainAspectRatio"))
		ctrlTbl.RawSetString("legend", opts.RawGetString("legend"))
		ctrlTbl.RawSetString("legendPosition", opts.RawGetString("legendPosition"))
		ctrlTbl.RawSetString("animation", opts.RawGetString("animation"))
		ctrlTbl.RawSetString("stacked", opts.RawGetString("stacked"))
	}

	// Textbox extended options (kforms_enhancements.md §4.1): multiline
	// (textarea) and datetime (flatpickr) modes.
	if ctrlType == "textbox" {
		ctrlTbl.RawSetString("multiline", opts.RawGetString("multiline"))
		ctrlTbl.RawSetString("rows", opts.RawGetString("rows"))
		ctrlTbl.RawSetString("cols", opts.RawGetString("cols"))
		ctrlTbl.RawSetString("datetime", opts.RawGetString("datetime"))
		dt := opts.RawGetString("datetime")
		if dtTbl, ok := dt.(*lua.LTable); ok {
			for _, k := range []string{"mode", "format", "min", "max", "step"} {
				ctrlTbl.RawSetString("datetime_"+k, dtTbl.RawGetString(k))
			}
		} else if dt != lua.LNil && dt != lua.LFalse {
			ctrlTbl.RawSetString("datetime_mode", lua.LString("datetime"))
		}
	}

	// Label multiline option (kforms_enhancements.md §4.2).
	if ctrlType == "label" {
		ctrlTbl.RawSetString("multiline", opts.RawGetString("multiline"))
	}

	// Image control options (kforms_enhancements.md §4.3).
	if ctrlType == "image" {
		ctrlTbl.RawSetString("src", opts.RawGetString("src"))
		ctrlTbl.RawSetString("alt", opts.RawGetString("alt"))
		ctrlTbl.RawSetString("width", opts.RawGetString("width"))
		ctrlTbl.RawSetString("height", opts.RawGetString("height"))
		ctrlTbl.RawSetString("fit", opts.RawGetString("fit"))
		ctrlTbl.RawSetString("clickable", opts.RawGetString("clickable"))
	}

	controlsTbl.RawSetString(name, ctrlTbl)
}

// AddControl is the exported registration entry point used by the visual
// builder (internal/builder) to construct the LState form model from
// source-level options. Identical semantics to k.ctrl.<type>.
func AddControl(L *lua.LState, formName, name, ctrlType string, opts *lua.LTable) {
	addControl(L, formName, name, ctrlType, opts)
}

// RenderForm is the exported preview entry point used by the visual builder:
// it renders the form stored in the LState global `formName` to HTML.
func RenderForm(L *lua.LState, formName string) string {
	return renderForm(L, formName)
}

// sendOutbox sends a message to the session outbox.
func sendOutbox(e *Env, msg common.OutboxMsg) {
	if e == nil || e.App == nil {
		return
	}
	sess := e.App.Session()
	if sess != nil {
		sess.SendOutbox(msg)
	}
}

// getForm retrieves a form definition table, or nil when no such form exists.
func getForm(L *lua.LState, formName string) *lua.LTable {
	formTbl := L.GetGlobal(formName)
	if formTbl == lua.LNil {
		return nil
	}
	tbl, ok := formTbl.(*lua.LTable)
	if !ok {
		return nil
	}
	return tbl
}

// getControl retrieves a control table from a form.
func getControl(L *lua.LState, formName, name string) *lua.LTable {
	tbl := getForm(L, formName)
	if tbl == nil {
		return nil
	}

	controls := tbl.RawGetString("controls")
	if controls == lua.LNil {
		return nil
	}
	controlsTbl, ok := controls.(*lua.LTable)
	if !ok {
		return nil
	}

	ctrl := controlsTbl.RawGetString(name)
	if ctrl == lua.LNil {
		return nil
	}
	ctrlTbl, ok := ctrl.(*lua.LTable)
	if !ok {
		return nil
	}
	return ctrlTbl
}

// parseGap parses the gap option: number (applies to both x/y) or table {x, y}.
// Returns gapX, gapY as float64. Default: 5% desktop, 3% mobile (CSS handles mobile).
func parseGap(v lua.LValue) (float64, float64) {
	const defaultGap = 5.0
	if v == lua.LNil {
		return defaultGap, defaultGap
	}
	if n, ok := v.(lua.LNumber); ok {
		f := float64(n)
		if f < 0 {
			f = 0
		}
		if f > 50 {
			f = 50
		}
		return f, f
	}
	if tbl, ok := v.(*lua.LTable); ok {
		gapX := defaultGap
		gapY := defaultGap
		if xVal := tbl.RawGetString("x"); xVal != lua.LNil {
			if n, ok := xVal.(lua.LNumber); ok {
				f := float64(n)
				if f >= 0 && f <= 50 {
					gapX = f
				}
			}
		}
		if yVal := tbl.RawGetString("y"); yVal != lua.LNil {
			if n, ok := yVal.(lua.LNumber); ok {
				f := float64(n)
				if f >= 0 && f <= 50 {
					gapY = f
				}
			}
		}
		return gapX, gapY
	}
	return defaultGap, defaultGap
}
