// Package bindings implements the form and control bindings.
package bindings

import (
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

		// Store form definition in Lua global
		formTbl := L.NewTable()
		formTbl.RawSetString("name", lua.LString(name))
		formTbl.RawSetString("title", lua.LString(opts.RawGetString("title").String()))
		formTbl.RawSetString("layout", lua.LString(opts.RawGetString("layout").String()))
		formTbl.RawSetString("controls", L.NewTable())
		formTbl.RawSetString("handlers", L.NewTable())
		L.SetGlobal(name, formTbl)

		return 0
	})

	// k.form.show(name) - show form (modal, suspends caller)
	e.register("form.show", "forms", func(L *lua.LState) int {
		name := L.CheckString(1)

		// Push form onto session stack
		sess := e.App.Session()
		if sess != nil {
			sess.PushForm(name)
			// Store the suspended coroutine so it can be resumed when form closes
			sess.StoreFormCoro(name, L)
		}

		// Render form and send to browser
		html := renderForm(L, name)
		sendOutbox(e, common.OutboxMsg{
			Type: "render_form",
			Form: name,
			HTML: html,
		})

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

			// Resume the suspended coroutine for this form
			sess.ResumeFormCoro(name)
		}

		sendOutbox(e, common.OutboxMsg{
			Type: "close_form",
			Form: name,
		})

		return 0
	})

	// k.form.return_to(name) - close all forms above name
	e.register("form.return_to", "forms", func(L *lua.LState) int {
		name := L.CheckString(1)

		sess := e.App.Session()
		if sess != nil {
			for sess.TopForm() != name && sess.TopForm() != "" {
				closed := sess.PopForm()
				// Resume the suspended coroutine for each closed form
				sess.ResumeFormCoro(closed)
				sendOutbox(e, common.OutboxMsg{
					Type: "close_form",
					Form: closed,
				})
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

	// k.form.on(form, ctrl, event, fn) - register event handler
	e.register("form.on", "forms", func(L *lua.LState) int {
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

	// k.ctrl.set_value(form, name, value)
	e.register("ctrl.set_value", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)
		value := L.Get(3)

		ctrl := getControl(L, formName, name)
		if ctrl == nil {
			return 0
		}
		ctrl.RawSetString("value", value)

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
	if controls == lua.LNil {
		controls = L.NewTable()
		tbl.RawSetString("controls", controls)
	}
	controlsTbl, ok := controls.(*lua.LTable)
	if !ok {
		return
	}

	// Track control order
	order := tbl.RawGetString("order")
	if order == lua.LNil {
		order = L.NewTable()
		tbl.RawSetString("order", order)
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

	// Label control uses "text" option, others use "label"
	if ctrlType == "label" {
		ctrlTbl.RawSetString("label", lua.LString(opts.RawGetString("text").String()))
	} else {
		ctrlTbl.RawSetString("label", lua.LString(opts.RawGetString("label").String()))
	}

	ctrlTbl.RawSetString("value", opts.RawGetString("value"))
	ctrlTbl.RawSetString("enabled", opts.RawGetString("enabled"))
	ctrlTbl.RawSetString("visible", opts.RawGetString("visible"))
	ctrlTbl.RawSetString("class", opts.RawGetString("class"))
	ctrlTbl.RawSetString("items", opts.RawGetString("items"))
	ctrlTbl.RawSetString("hidden_value", opts.RawGetString("hidden_value"))

	// For button with onclick, register as click handler
	if ctrlType == "button" {
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

	controlsTbl.RawSetString(name, ctrlTbl)
}

// renderForm renders a form to HTML using templ-like logic (simplified for now).
func renderForm(L *lua.LState, formName string) string {
	formTbl := L.GetGlobal(formName)
	if formTbl == lua.LNil {
		return `<div class="error">Form not found: ` + formName + `</div>`
	}
	tbl, ok := formTbl.(*lua.LTable)
	if !ok {
		return `<div class="error">Invalid form</div>`
	}

	title := tbl.RawGetString("title").String()
	layout := tbl.RawGetString("layout").String()
	controls := tbl.RawGetString("controls")
	order := tbl.RawGetString("order")

	var html string
	html += `<div id="f:` + formName + `" class="kalua-form"`
	if layout != "" && layout != "vertical" {
		html += ` layout="` + layout + `"`
	}
	html += `>`

	if title != "" {
		html += `<div class="kalua-form-title">` + title + `</div>`
	}

	if controlsTbl, ok := controls.(*lua.LTable); ok {
		// Iterate in order if available
		if orderTbl, ok := order.(*lua.LTable); ok {
			orderTbl.ForEach(func(k, v lua.LValue) {
				name := v.String()
				ctrl := controlsTbl.RawGetString(name)
				if ctrl != lua.LNil {
					if ctrlTbl, ok := ctrl.(*lua.LTable); ok {
						html += renderControl(ctrlTbl)
					}
				}
			})
		} else {
			// Fallback to unordered iteration
			controlsTbl.ForEach(func(k, v lua.LValue) {
				if ctrl, ok := v.(*lua.LTable); ok {
					html += renderControl(ctrl)
				}
			})
		}
	}

	html += `</div>`
	return html
}

func renderControl(ctrl *lua.LTable) string {
	ctrlType := ctrl.RawGetString("type").String()
	name := ctrl.RawGetString("name").String()
	formName := ctrl.RawGetString("form").String()
	label := ctrl.RawGetString("label").String()
	value := ctrl.RawGetString("value").String()

	id := "c:" + formName + ":" + name

	switch ctrlType {
	case "label":
		return `<label class="kalua-label" id="` + id + `">` + label + `</label>`
	case "textbox":
		enabled := ""
		enabledVal := ctrl.RawGetString("enabled")
		if enabledVal != lua.LNil && enabledVal.String() == "false" {
			enabled = ` disabled`
		}
		visible := ""
		if ctrl.RawGetString("visible") != lua.LNil && ctrl.RawGetString("visible").String() == "false" {
			visible = ` style="display:none"`
		}
		return `<div class="kalua-control"` + visible + `>
			<label class="kalua-label" for="` + id + `">` + label + `</label>
			<input type="text" class="kalua-input" id="` + id + `" name="` + name + `" value="` + value + `" data-k-form="` + formName + `" data-k-ctrl="` + name + `"` + enabled + `>
		</div>`
	case "button":
		btnClass := "kalua-button kalua-button-primary"
		if v := ctrl.RawGetString("class"); v != lua.LNil {
			btnClass = v.String()
		}
		enabled := ""
		enabledVal := ctrl.RawGetString("enabled")
		if enabledVal != lua.LNil && enabledVal.String() == "false" {
			enabled = ` disabled`
		}
		visible := ""
		if ctrl.RawGetString("visible") != lua.LNil && ctrl.RawGetString("visible").String() == "false" {
			visible = ` style="display:none"`
		}
		return `<button type="button" class="` + btnClass + `" id="` + id + `" name="` + name + `" data-k-form="` + formName + `" data-k-ctrl="` + name + `" ` + enabled + visible + `>` + label + `</button>`
	case "combo", "list":
		items := ctrl.RawGetString("items")
		var options string
		if itemsTbl, ok := items.(*lua.LTable); ok {
			itemsTbl.ForEach(func(k, v lua.LValue) {
				options += `<option value="` + k.String() + `">` + v.String() + `</option>`
			})
		}
		size := ""
		if ctrlType == "list" {
			size = ` size="5"`
		}
		enabled := ""
		enabledVal := ctrl.RawGetString("enabled")
		if enabledVal != lua.LNil && enabledVal.String() == "false" {
			enabled = ` disabled`
		}
		visible := ""
		if ctrl.RawGetString("visible") != lua.LNil && ctrl.RawGetString("visible").String() == "false" {
			visible = ` style="display:none"`
		}
		return `<div class="kalua-control"` + visible + `>
			<label class="kalua-label" for="` + id + `">` + label + `</label>
			<select class="kalua-select" id="` + id + `" name="` + name + `" data-k-form="` + formName + `" data-k-ctrl="` + name + `"` + size + enabled + `>` + options + `</select>
		</div>`
	case "checkbox":
		checked := ""
		if value == "true" || value == "1" {
			checked = ` checked`
		}
		enabled := ""
		enabledVal := ctrl.RawGetString("enabled")
		if enabledVal != lua.LNil && enabledVal.String() == "false" {
			enabled = ` disabled`
		}
		visible := ""
		if ctrl.RawGetString("visible") != lua.LNil && ctrl.RawGetString("visible").String() == "false" {
			visible = ` style="display:none"`
		}
		hiddenValue := ctrl.RawGetString("hidden_value").String()
		hiddenInput := ""
		if hiddenValue != "" {
			hiddenInput = `<input type="hidden" name="` + name + `_hidden" value="` + hiddenValue + `">`
		}
		return `<div class="kalua-control kalua-checkbox-item"` + visible + `>
			<input type="checkbox" class="kalua-input" id="` + id + `" name="` + name + `" value="` + value + `" data-k-form="` + formName + `" data-k-ctrl="` + name + "`" + checked + enabled + `>
			<label class="kalua-label" for="` + id + `">` + label + `</label>
			` + hiddenInput + `
		</div>`
	case "radio":
		checked := ""
		if value == "true" || value == "1" {
			checked = ` checked`
		}
		enabled := ""
		enabledVal := ctrl.RawGetString("enabled")
		if enabledVal != lua.LNil && enabledVal.String() == "false" {
			enabled = ` disabled`
		}
		visible := ""
		if ctrl.RawGetString("visible") != lua.LNil && ctrl.RawGetString("visible").String() == "false" {
			visible = ` style="display:none"`
		}
		hiddenValue := ctrl.RawGetString("hidden_value").String()
		hiddenInput := ""
		if hiddenValue != "" {
			hiddenInput = `<input type="hidden" name="` + name + `_hidden" value="` + hiddenValue + `">`
		}
		return `<div class="kalua-control kalua-radio-item"` + visible + `>
			<input type="radio" class="kalua-input" id="` + id + `" name="` + name + `" value="` + value + `" data-k-form="` + formName + `" data-k-ctrl="` + name + "`" + checked + enabled + `>
			<label class="kalua-label" for="` + id + `">` + label + `</label>
			` + hiddenInput + `
		</div>`
	case "table":
		// Table rendering with columns and rows
		columns := ctrl.RawGetString("columns")
		rows := ctrl.RawGetString("rows")

		var thead string
		if columnsTbl, ok := columns.(*lua.LTable); ok {
			thead = "<thead><tr>"
			columnsTbl.ForEach(func(k, v lua.LValue) {
				thead += `<th data-k-col="` + k.String() + `">` + v.String() + `</th>`
			})
			thead += "</tr></thead>"
		} else {
			thead = `<thead><tr><th>` + label + `</th></tr></thead>`
		}

		var tbody string
		if rowsTbl, ok := rows.(*lua.LTable); ok {
			tbody = "<tbody>"
			rowsTbl.ForEach(func(k, v lua.LValue) {
				if rowTbl, ok := v.(*lua.LTable); ok {
					tbody += "<tr data-k-row=\"" + k.String() + "\">"
					rowTbl.ForEach(func(colK, colV lua.LValue) {
						tbody += `<td data-k-col="` + colK.String() + `">` + colV.String() + `</td>`
					})
					tbody += "</tr>"
				}
			})
			tbody += "</tbody>"
		} else {
			tbody = "<tbody></tbody>"
		}

		enabled := ""
		enabledVal := ctrl.RawGetString("enabled")
		if enabledVal != lua.LNil && enabledVal.String() == "false" {
			enabled = ` disabled`
		}
		visible := ""
		if ctrl.RawGetString("visible") != lua.LNil && ctrl.RawGetString("visible").String() == "false" {
			visible = ` style="display:none"`
		}
		return `<div class="kalua-control"` + visible + `>
			<table class="kalua-table" id="` + id + `" data-k-form="` + formName + `" data-k-ctrl="` + name + `" ` + enabled + `>` + thead + tbody + `</table>
		</div>`
	}
	return `<div class="kalua-control">Unknown control: ` + ctrlType + `</div>`
}

// sendOutbox sends a message to the session outbox.
func sendOutbox(e *Env, msg common.OutboxMsg) {
	sess := e.App.Session()
	if sess != nil {
		sess.SendOutbox(msg)
	}
}

// getControl retrieves a control table from a form.
func getControl(L *lua.LState, formName, name string) *lua.LTable {
	formTbl := L.GetGlobal(formName)
	if formTbl == lua.LNil {
		return nil
	}
	tbl, ok := formTbl.(*lua.LTable)
	if !ok {
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
