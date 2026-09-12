package ai

import "strings"

// KaluaComponentPrompt returns the OpenUI-style "component library" for KALUA
// run-mode forms. OpenUI generates its system prompt from a component library;
// this is the same idea ported to Go (a static port — no npm/build needed).
// Each entry names a buildable UI element, its KALUA constructor, and the
// options the model may use. Appending this to the system prompt teaches the
// model exactly what it is allowed to generate, which keeps local models on a
// smaller, controlled surface.
func KaluaComponentPrompt() string {
	var sb strings.Builder
	sb.WriteString("\n## Component Library (buildable UI elements)\n")
	sb.WriteString("You may only build the following KALUA UI elements. Every element lives inside a form created with `k.form.new` and shown with `k.form.show`.\n\n")
	sb.WriteString(`### form
- k.form.new(name, {title=, layout="vertical"|"grid", align="left"|"center"|"right", gap=px, cells={...}})
- k.form.show(name)  — suspends until the form closes
- k.form.on(form, event, fn) / k.form.on(form, ctrl, event, fn) — events: open_form, after_open_form, close_form, key_pressed

### label
- k.ctrl.label(form, name, {text=, multiline=true|false})
  A static text line; use for headings and captions.

### textbox
- k.ctrl.textbox(form, name, {label=, value=, multiline=true|false, rows=, cols=, datetime=bool|{mode="date"|"time"|"datetime",format=}})
  Single- or multi-line text input; datetime renders a date/time picker.

### button
- k.ctrl.button(form, name, {label=, class="kalua-button-primary"|"", onclick=function() ... end})
  Clickable action button. onclick receives no args; read control values inside with k.ctrl.get_value.

### combo
- k.ctrl.combo(form, name, {label=, items={ {key=,display=}, ... }, size=})
  Dropdown single-select.

### list
- k.ctrl.list(form, name, {label=, items={ {key=,display=}, ... }, size=N})
  Multi-row selectable list (size = visible rows).

### checkbox
- k.ctrl.checkbox(form, name, {label=, value=bool, hidden_value=})
  Boolean toggle; checked state read via k.ctrl.get_value.

### radio
- k.ctrl.radio(form, name, {label=, value=bool, hidden_value=})
  Radio button in a group.

### table
- k.ctrl.table(form, name, {columns={"col1",...}, data={ {...}, ... }, query=, page_size=, count_query=})
  Data grid. Either supply static data or a query (fetched server-side).

### looper
- k.ctrl.looper(form, name, {db=, query=, page_size=, count_query=})
  Record-driven repeater bound to a query.

### chart
- k.ctrl.chart(form, name, {type="line"|"bar"|"hbar"|"pie"|"doughnut"|"scatter"|"radar"|"area", title=, labels={...}, datasets={{label=,data={...}},...}, options={...}})
  Interactive chart. Supply labels and datasets inline.

### image
- k.ctrl.image(form, name, {src=, alt=, width=, height=, fit="contain"|"cover"|"fill"|"scale-down"|"none", clickable=bool})
  Shows an image; src may be a URL or base64 data.

### msgbox
- k.msgbox{title=, message=, type="info"|"warning"|"danger", buttons={{label,value},...}} → returns clicked value
- k.msgbox(text) — simple OK box

### popup
- k.popup({ {label=, value=} | {label=, items={...}}, ... }) → returns picked value or nil

### flow / helpers
- k.print(...) — log output · k.sleep(ms) · k.yield() · k.quit() · k.error(msg)
- k.ctrl.set_value(form, ctrl, v) / k.ctrl.get_value(form, ctrl) / k.ctrl.set_property(form, ctrl, prop, v)
- K.tonum(v), K.tostr(v), K.eq(a,b), K.truthy(v) — Kalipso coercion
- Expression functions are FLAT globals: tostr, tonum, upper, lower, trim, left, right, middle, length, replace, find, round, abs, floor, ceiling, lookup, iif, yesno, sys_date, sys_time, add_days, subtract_days, date_diff, week_day, day, month, year, hour, minute, second

### common control options (all controls)
- label=, value=, enabled=bool, visible=bool, class=, cell= (grid cell id), align="left"|"center"|"right"
- Events via k.form.on(form, ctrl, "event", fn): click (button), whenever_modified/get_focus/lose_focus (textbox), selection_change (combo/list/radio), check/uncheck (checkbox), chart_click/chart_hover/chart_legend_click (chart)
`)
sb.WriteString("\nWhen generating a form app, structure the script as:\n")
	sb.WriteString("1. `function main()`\n2. `k.form.new(\"<name>\", {...})` with the form title and layout\n3. one `k.ctrl.<type>(...)` call per control (with sensible default values)\n4. `k.form.on(...)` handlers for interactivity\n5. `k.form.show(\"<name>\")\n\nWhen editing an existing script that is provided in the request, keep every control and handler from it and change only what the request asks for — never drop or rename controls the request does not mention.\n")
	return sb.String()
}

// buildMessages assembles the system+user messages for a generation request,
// always including the component library so the model stays on-surface.
// req.History (if any) is interleaved between the system message and the
// current user request (oldest first). The system prompt honors req.FullDoc:
// the compact run-mode subset is the default (it fits local-model context);
// the full API reference is opt-in via --full-doc / the FullDoc flag.
func buildMessages(req GenerateRequest) []ChatMessage {
	full := req.FullDoc
	messages := []ChatMessage{
		{Role: "system", Content: BuildSystemPrompt(full) + KaluaComponentPrompt()},
	}
	for _, h := range req.History {
		messages = append(messages, h)
	}
	messages = append(messages, ChatMessage{Role: "user", Content: buildPrompt(req)})
	return messages
}
