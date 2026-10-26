// api_doc.go is the single source of truth for tooling (LSP completion,
// hover, definition). Every entry here must correspond to one in
// registerKnown; TestApiDocSync enforces that so the two never drift.
package bindings

// Param documents a single call parameter of a k.* binding. For functions
// taking an options table, one Param is emitted per meaningful opts key
// (Name like "optsTable.label"). Documented params and examples are used by
// the generated guides; entries with no Params/Example render as before.
type Param struct {
	// Name is the parameter name, e.g. "form", "name" or "optsTable.label".
	Name string

	// Type is the Lua type, e.g. "string", "number", "table", "function".
	Type string

	// Desc is a short description of the parameter's meaning.
	Desc string
}

// Info documents a single k.* binding for editors and generated API
// reference stubs.
type Info struct {
	// Name is the canonical dotted name, e.g. "form.new".
	Name string

	// Group is the registry group, e.g. "forms", "controls" or "json".
	Group string

	// Signature is a Lua-flavoured call signature, e.g.
	// "k.form.new(name, optsTable)".
	Signature string

	// Docs is a short human description shown on hover.
	Docs string

	// Source is the bindings source file that implements the binding
	// (e.g. "forms.go"); empty for namespace entries. Used to locate a
	// definition when the module sources are available.
	Source string

	// Params lists the call parameters (positional args and options-table
	// keys) rendered as a table in the generated guides. Empty → no table.
	Params []Param

	// Example is a short Lua usage snippet rendered after the description.
	// Empty → no example block.
	Example string
}

// P is a compact constructor for a Param.
func P(name, typ, desc string) Param { return Param{Name: name, Type: typ, Desc: desc} }

// apiDocs carries the tooling documentation for every k.* binding. Keep in
// sync with registerKnown; ApiDocSyncTest verifies the correspondence.
var apiDocs = map[string]Info{
	// flow
	"print": {Name: "print", Group: "flow", Signature: "k.print(...)",
		Docs:    "Prints values to the app log (tab-separated, like Lua print).",
		Params:  []Param{P("...", "any", "Values to print; joined with tabs into the app log.")},
		Example: `k.print("avg =", 3.5)`},
	"sleep": {Name: "sleep", Group: "flow", Signature: "k.sleep(ms)",
		Docs:    "Suspends the script for ms milliseconds.",
		Params:  []Param{P("ms", "number", "Milliseconds to pause before resuming.")},
		Example: `k.sleep(2000)  -- wait 2 seconds`},
	"yield": {Name: "yield", Group: "flow", Signature: "k.yield()",
		Docs:    "Yields the current coroutine, allowing other coroutines to run.",
		Example: `k.yield()`},
	"quit": {Name: "quit", Group: "flow", Signature: "k.quit()",
		Docs:    "Requests a clean termination of the app.",
		Example: `k.quit()`},
	"error": {Name: "error", Group: "flow", Signature: "k.error(msg)",
		Docs:    "Raises a deliberate Lua error.",
		Params:  []Param{P("msg", "string", "Error message raised to the caller.")},
		Example: `k.error("download failed")`},
	"on_error": {Name: "on_error", Group: "flow", Signature: "k.on_error(fn)",
		Docs:    "Registers (or clears, with nil) the Kalipso error hook. When ERRORCODE is set — by a failing binding or a genuine Lua error — fn is called with (ERRORCODE, ERRORMSG) so the script can show the error and continue. Erroring bindings return nil; branch on ERRORCODE.",
		Params:  []Param{P("fn", "function", "Error handler invoked as fn(ERRORCODE, ERRORMSG); pass nil to clear.")},
		Example: "k.on_error(function(code, msg)\n  k.msgbox{title=\"Error\", message=msg, type=\"danger\"}\nend)"},
	"msgbox": {Name: "msgbox", Group: "flow", Signature: "k.msgbox(opts)",
		Docs: "Shows a message box and returns the clicked button's value. Legacy form: `k.msgbox(text[, kind])` where kind is `info`/`warn`/`error`/`ok-cancel`/`yes-no` (returns `\"ok\"`, `\"cancel\"`, `\"yes\"`, `\"no\"`). Rich form takes a single options table: `type` sets the left color strip (`info` blue, `warning` amber, `danger` red); `buttons` is a list of `{label, value}` pairs (values keep their type: number, boolean or string), `{label=…, value=…}` tables, or bare strings (label = value). When omitted, a single `OK` button returning `\"ok\"` is added.",
		Params: []Param{
			P("title", "string", "Optional dialog title; empty hides the header."),
			P("message", "string", "Body text; empty hides the text."),
			P("type", "string", "\"info\" (default), \"warning\", or \"danger\" — sets the left color strip."),
			P("buttons", "list", "{label, value} entries (or bare strings); default single OK."),
		},
		Example: `local choice = k.msgbox{
  title   = "Confirm delete",
  message = "Delete row 42?",
  type    = "warning",                    -- "info" | "warning" | "danger"
  buttons = { {"Delete", 1}, {"Keep", 0}, {"Cancel", false} },
}
-- choice == 1, 0, false, or nil`},
	"popup": {Name: "popup", Group: "flow", Signature: "k.popup(items[, opts])",
		Docs: "Shows a multilevel menu-style popup (centered modal, fly-out submenus) and returns the picked item's value with its type preserved, or nil when dismissed (Esc / click outside). Items are leaves (label, value) or branches ({label, items={...}} which only open a fly-out submenu, up to 8 levels). Leaves accept {label, value} pairs, {label=…, value=…} tables, or bare strings (label = value); values round-trip typed (number, boolean, string, table). Alternative list form (no title): k.popup{ {\"Open\", \"open\"}, {\"Quit\"} }.",
		Params: []Param{
			P("title", "string", "Optional menu header (options-table form)."),
			P("items", "list", "Menu items: leaves {label, value} / bare strings, or branches {label, items={...}}."),
		},
		Example: `local pick = k.popup{
  title = "Maintenance",
  items = {
    { label = "File", items = {                       -- branch -> hover/click to open
        { label = "Open",   value = "open" },
        { label = "Recent", items = {
            { label = "a.lua", value = "r1" },
            { label = "b.lua", value = "r2" },
        }},
    }},
    { label = "Refresh", value = 1 },                  -- leaf -> returns 1
    { "Quit" },                                        -- bare string -> returns "Quit"
  },
}
if pick == nil then ... end                            -- dismissed`},
	"clipboard_set": {Name: "clipboard_set", Group: "flow", Signature: "k.clipboard_set(text)", Docs: "Writes text to the browser clipboard."},
	"clipboard_get": {Name: "clipboard_get", Group: "flow", Signature: "k.clipboard_get()", Docs: "Reads text from the browser clipboard."},
	"pick_file": {Name: "pick_file", Group: "flow", Signature: "k.pick_file([opts])",
		Docs: "Opens a browser file picker dialog. mode=\"open\" (default): picks existing files and returns a table of {{name, size, type, data}, ...} with base64-encoded data. mode=\"save\": shows a save dialog with a filename and returns {path, name}. mode=\"download\": triggers a download of base64 data and returns {path, name}. Returns nil on cancel. Suspends the script until the dialog completes.",
		Params: []Param{
			P("opts.accept", "string", "Comma-separated accepted types, e.g. \"image/*,.pdf\"."),
			P("opts.multiple", "boolean", "Allow choosing several files at once."),
			P("opts.mode", "string", `"open"`+" (default), "+`"save"`+" or "+`"download"`+"."),
			P("opts.filename", "string", "Suggested name for save/download modes."),
			P("opts.data", "string", "Base64 content to save when mode = \"download\"."),
		},
		Example: `local files = k.pick_file{ accept = "image/*,.pdf", multiple = true }  -- nil on cancel`},
	"bell":        {Name: "bell", Group: "flow", Signature: "k.bell()", Docs: "Plays a system beep sound via WebAudio."},
	"screen_size": {Name: "screen_size", Group: "flow", Signature: "k.screen_size()", Docs: "Returns viewport dimensions as {width, height}."},
	"http_request": {Name: "http_request", Group: "flow", Signature: "k.http_request(optsTable)",
		Docs: "Makes an HTTP request asynchronously; returns {status, headers, body}. Suspends the script until the response arrives.",
		Params: []Param{
			P("optsTable.method", "string", "HTTP method; default \"GET\"."),
			P("optsTable.url", "string", "Full request URL."),
			P("optsTable.headers", "table", "Request headers as {name = value, ...}."),
			P("optsTable.body", "string", "Request body for POST/PUT."),
			P("optsTable.timeout", "number", "Timeout in milliseconds."),
		},
		Example: `local res = k.http_request{ method = "GET", url = "https://api.example.com/x" }
k.print(res.status, res.body)`},

	// action set trio (§5.2)
	"assign": {Name: "assign", Group: "flow", Signature: "k.assign(target, kind, value)", Docs: "Sets a global variable or control value with type coercion. target: string (global name) or table {form=, ctrl=}. kind: \"numeric\"|\"string\"|\"boolean\"|\"date\". Returns the coerced value."},
	"set":    {Name: "set", Group: "flow", Signature: "k.set(name, fn)", Docs: "Stores a function in the action registry for later execution via k.exec."},
	"exec":   {Name: "exec", Group: "flow", Signature: "k.exec(name, ...)", Docs: "Executes a previously stored function (via k.set) asynchronously with the given arguments. Returns the function's result(s)."},
	"ai.tool": {Name: "ai.tool", Group: "ai", Signature: "k.ai.tool(name, schemaTable, fn)",
		Docs: "Registers or overrides an AI tool for k.form.ai_agent. name: tool name the model calls. schemaTable: {desc=string, params={{name=,type=,required=,desc=},...}}. fn: optional implementation function; omitted defaults to the global function of the same name. §12",
		Params: []Param{
			P("name", "string", "Tool name."),
			P("schema", "table", "Tool schema: {desc=string, params=...}."),
			P("fn", "function", "Optional implementation; defaults to the global function of the same name."),
		},
		Example: `k.ai.tool("get_customer", { desc = "Look up a customer by id", params = { {name="id", type="number", required=true} } }, get_customer)`},

	// debug
	"debug":        {Name: "debug", Group: "debug", Signature: "k.debug", Docs: "Runtime introspection helpers: stack/locals/trace."},
	"debug.stack":  {Name: "debug.stack", Group: "debug", Signature: "k.debug.stack()", Docs: "Returns a table of the current call frames, each with level, name, source, line and locals."},
	"debug.locals": {Name: "debug.locals", Group: "debug", Signature: "k.debug.locals([level])", Docs: "Returns a table of local name → value for the given frame level (default 1)."},
	"debug.trace":  {Name: "debug.trace", Group: "debug", Signature: "k.debug.trace([msg])", Docs: "Logs a script-side trace anchor when verbose tracing is enabled."},

	// forms
	"form": {Name: "form", Group: "forms", Signature: "k.form", Docs: "Form declarations: k.form.new/show/close/..."},
	"form.new": {Name: "form.new", Group: "forms", Signature: "k.form.new(name, optsTable)",
		Docs: "Declares a form. opts: {title, layout=vertical|grid, align=left|center|right, gap=n px, cells}. grid cells: {id={width 1-12, bg, border={width,color}, align}} or ordered array of {id,...}; assign controls via control opt cell=\"id\" and override alignment via align (kforms_enhancements §6).",
		Params: []Param{
			P("name", "string", "Unique form name."),
			P("optsTable.title", "string", "Title shown in the form header."),
			P("optsTable.layout", "string", "\"vertical\" (default) or \"grid\"."),
			P("optsTable.align", "string", "\"left\" (default), \"center\" or \"right\" — control alignment in vertical layout."),
			P("optsTable.gap", "number", "Gap between controls/cells in px."),
			P("optsTable.cells", "table", "Grid cells: ordered {id, width, bg, border, align} array or {id = {width, bg, border, align}} map."),
		},
		Example: `k.form.new("main", {
  title  = "Hello",
  layout = "vertical",            -- or "grid" with a cells table
  align  = "center",
})`},
	"form.show": {Name: "form.show", Group: "forms", Signature: "k.form.show(name, [options])",
		Docs: "Shows a form and suspends the script until it closes. By default the form fills the stage (normal). With options.modal=true the form is shown as a centered modal overlay with configurable gap from screen edges.",
		Params: []Param{
			P("name", "string", "Form name declared with k.form.new."),
			P("options", "table", "Optional: {modal=true|false (default false), gap=number|{x=num,y=num} (default 5% desktop, 3% mobile)}"),
		},
		Example: `k.form.show("main")
k.form.show("dialog", {modal=true})
k.form.show("settings", {modal=true, gap=10})
k.form.show("custom", {modal=true, gap={x=15, y=5}})`},
	"form.close": {Name: "form.close", Group: "forms", Signature: "k.form.close([name])",
		Docs:    "Closes the top form, or the named form.",
		Params:  []Param{P("name", "string", "Optional form name; omitted closes the top form.")},
		Example: `k.form.close()`},
	"form.return_to": {Name: "form.return_to", Group: "forms", Signature: "k.form.return_to(name)",
		Docs:    "Closes all forms above name (returns to it).",
		Params:  []Param{P("name", "string", "Form name to return to.")},
		Example: `k.form.return_to("main")`},
	"form.clear": {Name: "form.clear", Group: "forms", Signature: "k.form.clear(name)",
		Docs:    "Clears a form's control values.",
		Params:  []Param{P("name", "string", "Form name to clear.")},
		Example: `k.form.clear("main")`},
	"form.refresh": {Name: "form.refresh", Group: "forms", Signature: "k.form.refresh(name)",
		Docs:    "Re-renders and pushes the form to the browser.",
		Params:  []Param{P("name", "string", "Form name to re-render.")},
		Example: `k.form.refresh("main")`},
	"form.ai_agent": {Name: "form.ai_agent", Group: "forms", Signature: "k.form.ai_agent([optsTable]) → transcriptTable",
		Docs: "Opens a modal AI chat form. The script blocks until the user closes the chat; the return value is the full transcript as an array of {role, content, name?} message tables. Every script global function is auto-exposed as a tool the agent can call (descriptions from comments above the function, param names from debug info). Use k.ai.tool(name, schema, fn) to override or register a tool explicitly. opts: {title, welcome, system, max_turns, tools, api_key, base_url, model}. Requires AI configuration ([AI] in KALUA.INI or KALUA_AI_* env vars), or pass api_key explicitly. §12",
		Params: []Param{
			P("opts", "table", "Optional options: {title='AI Assistant', welcome, system, max_turns=8, tools, api_key, base_url, model}."),
		},
		Example: `local transcript = k.form.ai_agent{ title = "Support assistant", welcome = "How can I help?", max_turns = 6, api_key = "sk-..." }
for _, m in ipairs(transcript) do print(m.role, m.content) end`},
	"form.on": {Name: "form.on", Group: "forms", Signature: "k.form.on(form, ctrl, event, fn) | k.form.on(name, event, fn) | k.form.on(name, \"on_idle\", ms, fn)",
		Docs: "Registers an event handler. 4-arg form: control handler (form, ctrl, event, fn) for control events (e.g. \"onclick\"). 3-arg form: form-level handler (name, event, fn) for events like open_form, after_open_form, close_form, key_pressed (used as fallback when no control handler exists). 4-arg form with a number: k.form.on(name, \"on_idle\", ms, fn) sets a periodic idle callback (ms interval, default 1000) — only the topmost form receives idle events.",
		Params: []Param{
			P("form", "string", "Form name (4-arg control handler)."),
			P("ctrl", "string", "Control name (4-arg control handler)."),
			P("name", "string", "Form name (3-arg form-level handler)."),
			P("event", "string", "Event name: \"onclick\", \"open_form\", \"after_open_form\", \"close_form\", \"key_pressed\", \"on_idle\", chart events, ..."),
			P("ms", "number", "Idle callback interval in ms (on_idle form)."),
			P("fn", "function", "Handler function invoked with the event payload."),
		},
		Example: `k.form.on("main", "btn_ok", "onclick", function()
  k.form.show("details")
end)
k.form.on("main", "on_idle", 500, function() ... end)`},

	"set_property": {Name: "set_property", Group: "forms", Signature: "k.set_property(form, prop, value)",
		Docs: "Sets a form-level property and re-renders the form. Supports title, align, gap and dynamic styling props: bg (background color), color (text color), font (CSS font-family or an h1–h6/p text preset), font_size (numeric px, overrides preset), style (h1–h6/p text preset). The reserved keys name, controls, handlers and order cannot be set.",
		Params: []Param{
			P("form", "string", "Form name declared with k.form.new."),
			P("prop", "string", "Property name: \"title\", \"align\", \"gap\", \"bg\", \"color\", \"font\", \"font_size\", \"style\", ..."),
			P("value", "any", "Property value (string or number)."),
		},
		Example: `k.set_property("main", "bg", "#1e3a5f")
k.set_property("main", "font", "h2")
k.set_property("main", "title", "Welcome")`},
	"get_property": {Name: "get_property", Group: "forms", Signature: "k.get_property(form, prop)",
		Docs:    "Reads a form-level property; returns nil when the property or form is not set.",
		Params:  []Param{P("form", "string", "Form name declared with k.form.new."), P("prop", "string", "Property name to read.")},
		Example: `local bg = k.get_property("main", "bg")`},

	// controls
	"ctrl": {Name: "ctrl", Group: "controls", Signature: "k.ctrl", Docs: "Control constructors: k.ctrl.label/textbox/button/.... All k.ctrl.* constructors accept the shared styling opts {bg, color, font, font_size, style} (kforms_enhancements.md §10) and layout opts {cell, align}. font/style take an h1–h6 or p preset to size the text, or font takes a raw font family; font_size (px) overrides the preset."},
	"ctrl.label": {Name: "ctrl.label", Group: "controls", Signature: "k.ctrl.label(form, name, optsTable)",
		Docs: "Adds a label control. opts: {text, multiline?:boolean, cell?, align?, bg?, color?, font?, font_size?, style?}. multiline renders a pre-wrap div preserving \\n (kforms_enhancements.md §4.2). cell/align: grid layout assignment + alignment (kforms_enhancements.md §6). bg/color/font/font_size/style: per-widget styling (kforms_enhancements.md §10); font h1–h6/p or a font family, style an alternative h1–h6/p preset, font_size px overrides the preset.",
		Params: []Param{
			P("form", "string", "Parent form name."),
			P("name", "string", "Unique control name within the form."),
			P("optsTable.text", "string", "Label text."),
			P("optsTable.multiline", "boolean", "Preserve newlines with white-space: pre-wrap."),
			P("optsTable.cell", "string", "Grid cell id to place the control in."),
			P("optsTable.align", "string", "Alignment override: \"left\", \"center\" or \"right\"."),
			P("optsTable.bg", "string", "Background color (any CSS color)."),
			P("optsTable.color", "string", "Text color (any CSS color)."),
			P("optsTable.font", "string", "Font: an h1–h6/p preset or a raw font family."),
			P("optsTable.font_size", "number", "Font size in px (overrides the preset)."),
			P("optsTable.style", "string", "Alternative to font: an h1–h6/p preset."),
		},
		Example: `k.ctrl.label("main", "lbl", { text = "Hello\nWorld", multiline = true, font = "h2", color = "#d32f2f" })`},
	"ctrl.textbox": {Name: "ctrl.textbox", Group: "controls", Signature: "k.ctrl.textbox(form, name, optsTable)",
		Docs: "Adds a textbox control. opts: {label, value, enabled, visible, multiline?:boolean, rows?:number, cols?:number, datetime?:boolean|table, cell?, align?}. multiline renders a <textarea>. datetime enables a flatpickr picker: mode=\"date\"|\"time\"|\"datetime\", format, min, max, step (kforms_enhancements.md §4.1). cell/align: grid layout assignment + alignment (kforms_enhancements.md §6).",
		Params: []Param{
			P("form", "string", "Parent form name."),
			P("name", "string", "Unique control name within the form."),
			P("optsTable.label", "string", "Label text above the input."),
			P("optsTable.value", "string", "Initial value."),
			P("optsTable.enabled", "boolean", "Whether the control is editable."),
			P("optsTable.visible", "boolean", "Whether the control is shown."),
			P("optsTable.multiline", "boolean", "Render a <textarea>."),
			P("optsTable.rows", "number", "Textarea rows (multiline); default 4."),
			P("optsTable.cols", "number", "Textarea columns (multiline); default 50."),
			P("optsTable.datetime", "table", "Enable a flatpickr picker: {mode, format, min, max, step}."),
			P("optsTable.cell", "string", "Grid cell id to place the control in."),
			P("optsTable.align", "string", "Alignment override: \"left\", \"center\" or \"right\"."),
		},
		Example: `k.ctrl.textbox("main", "age", { label = "Age", datetime = { mode = "date", format = "Y-m-d" } })`},
	"ctrl.button": {Name: "ctrl.button", Group: "controls", Signature: "k.ctrl.button(form, name, optsTable)",
		Docs:    "Adds a button control. opts may set label, class, onclick, enabled.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Unique control name."), P("optsTable.label", "string", "Button text."), P("optsTable.class", "string", "CSS class applied to the button."), P("optsTable.onclick", "string", "Event name fired on click."), P("optsTable.enabled", "boolean", "Whether the button is clickable.")},
		Example: `k.ctrl.button("main", "btn_ok", { label = "OK", onclick = "ok_clicked" })`},
	"ctrl.combo": {Name: "ctrl.combo", Group: "controls", Signature: "k.ctrl.combo(form, name, optsTable)",
		Docs:    "Adds a combo (dropdown) control. opts.items is a table of choices.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Unique control name."), P("optsTable.items", "list", "Drop-down choices (list of strings or {label, value} pairs)."), P("optsTable.label", "string", "Label text above the control."), P("optsTable.value", "any", "Initial selected value.")},
		Example: `k.ctrl.combo("main", "color", { items = {"red", "green", "blue"}, value = "green" })`},
	"ctrl.list": {Name: "ctrl.list", Group: "controls", Signature: "k.ctrl.list(form, name, optsTable)",
		Docs:    "Adds a multi-row select list. opts.items is a table of choices.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Unique control name."), P("optsTable.items", "list", "List choices."), P("optsTable.label", "string", "Label text above the control.")},
		Example: `k.ctrl.list("main", "sel", { items = {"a", "b", "c"} })`},
	"ctrl.table": {Name: "ctrl.table", Group: "controls", Signature: "k.ctrl.table(form, name, optsTable)",
		Docs:    "Adds a table control; rows manipulated via k.table.*. With opts {db, query, ...} the table is DB-linked (Tabulator mode). db is a k.connect_db handle or a --db NAME prereregistered at startup.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Unique control name."), P("optsTable.columns", "list", "Column definitions (Tabulator mode)."), P("optsTable.db", "string", "DB handle (k.connect_db result) or a --db NAME."), P("optsTable.query", "string", "SQL query for a DB-linked table.")},
		Example: `k.ctrl.table("main", "grid", { columns = { {field = "id", title = "ID"} } })`},
	"ctrl.checkbox": {Name: "ctrl.checkbox", Group: "controls", Signature: "k.ctrl.checkbox(form, name, optsTable)",
		Docs:    "Adds a checkbox control.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Unique control name."), P("optsTable.label", "string", "Label text next to the box."), P("optsTable.value", "boolean", "Initial checked state.")},
		Example: `k.ctrl.checkbox("main", "agreed", { label = "I agree", value = false })`},
	"ctrl.radio": {Name: "ctrl.radio", Group: "controls", Signature: "k.ctrl.radio(form, name, optsTable)",
		Docs:    "Adds a radio button control.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Unique control name."), P("optsTable.items", "list", "List of radio options."), P("optsTable.value", "any", "Initially selected option value.")},
		Example: `k.ctrl.radio("main", "plan", { items = {"basic", "pro"}, value = "pro" })`},
	"ctrl.set_value": {Name: "ctrl.set_value", Group: "controls", Signature: "k.ctrl.set_value(form, name, value)",
		Docs:    "Sets a control's value and re-renders it.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Control name."), P("value", "any", "New value; for images the src (kforms_enhancements.md §4.3).")},
		Example: `k.ctrl.set_value("main", "age", 31)`},
	"ctrl.get_value": {Name: "ctrl.get_value", Group: "controls", Signature: "k.ctrl.get_value(form, name)",
		Docs:    "Returns a control's current value.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Control name.")},
		Example: `local age = k.ctrl.get_value("main", "age")`},
	"ctrl.set_property": {Name: "ctrl.set_property", Group: "controls", Signature: "k.ctrl.set_property(form, name, prop, value)",
		Docs:    "Sets an arbitrary control property. Styling props (bg, color, font, font_size, style, kforms_enhancements.md §10) restyle the widget live via a re-render.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Control name."), P("prop", "string", "Property name, e.g. \"label\", \"enabled\", \"cell\", \"align\", \"bg\", \"color\", \"font\", \"font_size\", \"style\"."), P("value", "any", "New property value.")},
		Example: `k.ctrl.set_property("main", "btn", "enabled", false)\nk.ctrl.set_property("main", "lbl", "color", "#1976d2") -- live restyle`},
	"ctrl.get_property": {Name: "ctrl.get_property", Group: "controls", Signature: "k.ctrl.get_property(form, name, prop)",
		Docs:    "Gets an arbitrary control property.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Control name."), P("prop", "string", "Property name.")},
		Example: `local label = k.ctrl.get_property("main", "btn", "label")`},
	"ctrl.set_focus": {Name: "ctrl.set_focus", Group: "controls", Signature: "k.ctrl.set_focus(form, name)",
		Docs:    "Moves focus to a control in the browser.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Control name.")},
		Example: `k.ctrl.set_focus("main", "age")`},
	"ctrl.refresh": {Name: "ctrl.refresh", Group: "controls", Signature: "k.ctrl.refresh(form, name)",
		Docs:    "Re-renders a single control and pushes the update.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Control name.")},
		Example: `k.ctrl.refresh("main", "grid")`},
	"ctrl.looper": {Name: "ctrl.looper", Group: "controls", Signature: "k.ctrl.looper(form, name, optsTable)",
		Docs:    "Adds a looper control (repeating row layout). DB-linked when opts carry {db,query,links,page_size?,count_query?,where?,order_by?}; db is a k.connect_db handle or a --db NAME prereregistered at startup. With opts.row (array of {type,name,property?,field?|column?,opts?} row-template control defs) rows are rendered server-side as real controls (label/textbox/image/checkbox, read-only); links can be omitted — they are derived from row.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Unique control name."), P("optsTable.db", "string", "DB handle (k.connect_db result) or a --db NAME."), P("optsTable.query", "string", "SQL query for a DB-linked looper."), P("optsTable.links", "list", "Maps result columns to template controls (optional when row is set)."), P("optsTable.row", "list", "Row-template control defs {type, name, property, field or column, opts} — server-rendered read-only rows."), P("optsTable.page_size", "number", "Rows per page for pagination.")},
		Example: `k.ctrl.looper("main", "rows", { db = h, query = "SELECT * FROM items", row = { {type="label", name="lb_name", field="name"}, {type="textbox", name="tx_qty", field="qty"} } })`},
	"ctrl.grid": {Name: "ctrl.grid", Group: "controls", Signature: "k.ctrl.grid(form, name, optsTable)",
		Docs: "Adds a CRUD grid control (kforms_enhancements.md §7): a DB-linked Tabulator table with row/global actions, selection and an optional detail/edit form. opts: {db, query, count_query?, page_size?, where?, order_by?, pk_field?, columns, row_actions?, global_actions?, selection_mode? (default multi), row_click_action?, column_visibility?, form (\"name\" or inline {title, controls})}. Reads page/sort/filter through the Go host like tabulator=true tables; writes are wired via later phases.",
		Params: []Param{
			P("form", "string", "Parent form name."),
			P("name", "string", "Unique control name."),
			P("optsTable.db", "string", "DB handle (k.connect_db result) or a --db NAME."),
			P("optsTable.query", "string", "Base SELECT for the grid data."),
			P("optsTable.columns", "list", "Tabulator column definitions {field, title, sortable, headerFilter, visible}."),
			P("optsTable.pk_field", "string", "Primary-key column (used by row actions/CRUD)."),
			P("optsTable.page_size", "number", "Rows per page (default 25)."),
			P("optsTable.row_actions", "table", "Row action toggles: {view=true, edit=true, delete=true, ...}."),
			P("optsTable.global_actions", "table", "Toolbar actions: {new_record=true, batch_delete=true, ...}."),
			P("optsTable.selection_mode", "string", "\"none\", \"single\" or \"multi\" (default \"multi\")."),
			P("optsTable.row_click_action", "string", "Action on row click: \"view\" | \"edit\" | \"select\" | \"none\"."),
			P("optsTable.form", "any", "Detail/edit form: a referenced form name or an inline {title, controls} table."),
		},
		Example: `k.ctrl.grid("main", "users", {
  db = "main", query = "SELECT * FROM users", pk_field = "id",
  columns = { {field="id", title="ID"}, {field="name", title="Name"} },
  row_actions = { view = true, edit = true, delete = true },
  global_actions = { new_record = true, batch_delete = true },
})`},
	"ctrl.chart": {Name: "ctrl.chart", Group: "controls", Signature: "k.ctrl.chart(form, name, optsTable)",
		Docs: "Adds a Chart.js control. opts: {type=line|bar|hbar|pie|doughnut|scatter|radar|area, title, width=400, height=300, labels, datasets, options, responsive=true, maintainAspectRatio=false, legend=true, legendPosition=top, animation=true, stacked=false}. Events chart_click/chart_hover/chart_legend_click via k.form.on.",
		Params: []Param{
			P("form", "string", "Parent form name."),
			P("name", "string", "Unique control name."),
			P("optsTable.type", "string", "\"line\", \"bar\", \"hbar\", \"pie\", \"doughnut\", \"scatter\", \"radar\" or \"area\"."),
			P("optsTable.title", "string", "Chart title."),
			P("optsTable.width", "number", "Canvas width in px (default 400)."),
			P("optsTable.height", "number", "Canvas height in px (default 300)."),
			P("optsTable.labels", "list", "X-axis category labels."),
			P("optsTable.datasets", "list", "Dataset tables {label, data, ...}."),
			P("optsTable.options", "table", "Extra Chart.js options deep-merged over the defaults."),
			P("optsTable.responsive", "boolean", "Scale canvas to its container (default true)."),
			P("optsTable.legend", "boolean", "Show the legend (default true)."),
			P("optsTable.legendPosition", "string", "Legend position: \"top\" (default), \"bottom\", \"left\", \"right\"."),
			P("optsTable.animation", "boolean", "Animate updates (default true)."),
			P("optsTable.stacked", "boolean", "Stack datasets on the value axis (default false)."),
		},
		Example: `k.ctrl.chart("dash", "trend", {
  type     = "line",
  labels   = {"Mon", "Tue", "Wed"},
  datasets = { { label = "Sales", data = {10, 20, 15} } },
})`},
	"ctrl.image": {Name: "ctrl.image", Group: "controls", Signature: "k.ctrl.image(form, name, optsTable)",
		Docs: "Adds an image control (<img>). opts: {src (required), alt, width, height (px or %), fit=\"cover|contain|fill|scale-down|none\" (default contain), clickable?, onclick?}. k.ctrl.set_value(form, name, new_src) updates the image (kforms_enhancements.md §4.3).",
		Params: []Param{
			P("form", "string", "Parent form name."),
			P("name", "string", "Unique control name."),
			P("optsTable.src", "string", "Image URL or data URI (required)."),
			P("optsTable.alt", "string", "Accessibility/fallback text."),
			P("optsTable.width", "string", "Width (px or %)."),
			P("optsTable.height", "string", "Height (px or %)."),
			P("optsTable.fit", "string", "Object-fit: \"cover\", \"contain\" (default), \"fill\", \"scale-down\", \"none\"."),
			P("optsTable.clickable", "boolean", "Enable click events on the image."),
			P("optsTable.onclick", "string", "Event name fired when the image is clicked."),
		},
		Example: `k.ctrl.image("main", "logo", { src = "/img/logo.png", width = "200px", fit = "contain", clickable = true, onclick = "logo_clicked" })`},
	"ctrl.login": {Name: "ctrl.login", Group: "controls", Signature: "k.ctrl.login(form, name, optsTable)",
		Docs: "Adds a login control panel for user authentication. The control renders a login/password form with submit/cancel buttons and handles credential verification against a database. opts: {db (required), table=users, login_column=login, password_column=password, salt_column, hash=pbkdf2|sha256|plain, salt, iterations=100000, keylen=32, title, subtitle, login_label=Login, password_label=Password, login_placeholder, password_placeholder, submit_label=Sign IN, cancel_label=Cancel, error_message=Invalid login or password, attempts=3}. Events: on_login_submit(login, password) — runs before verification; on_login(user) — fired on success with user row map; on_login_error(message, attempts_left) — fired on failure; on_login_cancel() — fired when cancel clicked. k.ctrl.get_value returns the verified user map.",
		Params: []Param{
			P("form", "string", "Parent form name."),
			P("name", "string", "Unique control name within the form."),
			P("optsTable.db", "string", "Required. DB handle from k.connect_db/k.connect_sqlite or --db NAME."),
			P("optsTable.table", "string", "Users table name (default \"users\")."),
			P("optsTable.login_column", "string", "Column matched against login input (default \"login\")."),
			P("optsTable.password_column", "string", "Column holding stored hash/secret (default \"password\")."),
			P("optsTable.salt_column", "string", "Per-user salt column (required for hash=pbkdf2)."),
			P("optsTable.hash", "string", "Hash algorithm: \"pbkdf2\" (default), \"sha256\", or \"plain\"."),
			P("optsTable.salt", "string", "Static salt; overrides salt_column."),
			P("optsTable.iterations", "number", "pbkdf2 rounds (default 100000)."),
			P("optsTable.keylen", "number", "pbkdf2 key length in bytes (default 32)."),
			P("optsTable.title", "string", "Panel heading."),
			P("optsTable.subtitle", "string", "Secondary line under heading."),
			P("optsTable.login_label", "string", "Label above login input (default \"Login\")."),
			P("optsTable.password_label", "string", "Label above password input (default \"Password\")."),
			P("optsTable.submit_label", "string", "Submit button text (default \"Sign IN\")."),
			P("optsTable.cancel_label", "string", "Cancel button text (default \"Cancel\")."),
			P("optsTable.error_message", "string", "Inline error text (default \"Invalid login or password\")."),
			P("optsTable.attempts", "number", "Wrong-password retries before control stops (default 3)."),
		},
		Example: `k.form.new("loginform", {title="Sign in", layout="vertical", align="center"})
k.ctrl.login("loginform", "login", {db="main", table="users"})
k.form.on("loginform", "login", "on_login", function(user)
    k.msgbox("Welcome " .. user.name)
end)
k.form.show("loginform", {modal=true})`},
	"ctrl.tree": {Name: "ctrl.tree", Group: "controls", Signature: "k.ctrl.tree(form, name, optsTable)",
		Docs: "Adds a DB-linked expandable tree control (kforms_enhancements.md §9): displays a field from a table as an adjacency list. opts: {db (required), table (required), label_field (required), id_field=id, parent_field=parent_id, where?, order_by?, expand_all=false, selection=single, max_nodes=5000}. The host fetches all matching rows in one query and builds the tree server-side; the client expands/collapses locally. Events via k.form.on: on_click(node), on_select(node), on_expand(node_id, expanded), on_loaded(node_count), on_error(message). k.ctrl.get_value returns the currently selected node map (or nil); set_value is ignored.",
		Params: []Param{
			P("form", "string", "Parent form name."),
			P("name", "string", "Unique control name."),
			P("optsTable.db", "string", "Required. DB handle from k.connect_db/k.connect_sqlite or --db NAME."),
			P("optsTable.table", "string", "Required. Source table holding the adjacency list."),
			P("optsTable.label_field", "string", "Required. Column rendered as the node label."),
			P("optsTable.id_field", "string", "Unique-ID column (default \"id\")."),
			P("optsTable.parent_field", "string", "Parent-reference column (default \"parent_id\")."),
			P("optsTable.where", "string", "Extra WHERE clause (passed through verbatim)."),
			P("optsTable.order_by", "string", "Deterministic sibling order, e.g. \"name ASC\"."),
			P("optsTable.expand_all", "boolean", "Render roots expanded (default false)."),
			P("optsTable.selection", "string", "\"single\" (default) or \"none\" — enables on_select."),
			P("optsTable.max_nodes", "number", "Safety cap on the full load (default 5000)."),
		},
		Example: `k.form.new("main", {title="Categories"})
k.ctrl.tree("main", "tree", {
  db = "main", table = "categories", label_field = "name",
  order_by = "name ASC",
})
k.form.on("main", "tree", "on_click", function(node)
  k.print("clicked " .. node.name .. " (id=" .. tostr(node.id) .. ")")
end)`},
	"ctrl.tab": {Name: "ctrl.tab", Group: "controls", Signature: "k.ctrl.tab(form, name, optsTable)",
		Docs: "Adds a tab control (kforms_enhancements.md §10) that organizes content into multiple tabs showing only the active tab's panel at a time. Users switch tabs by clicking headers; k.tab.* switches programmatically. opts: {tabs (required array of {title, icon?}), tab_position=\"top\"|\"bottom\" (default \"top\"), swipable=?boolean (default true), cell?, align?}. Events via k.form.on(form, ctrl, \"onchange\", function(new_idx, old_idx) ... end). k.tab.get_tab returns the active tab index (1-based).",
		Params: []Param{
			P("form", "string", "Parent form name."),
			P("name", "string", "Unique control name."),
			P("optsTable.tabs", "list", "Required. Array of tab definitions: {title=..., icon=...}."),
			P("optsTable.tab_position", "string", "\"top\" (default) or \"bottom\"."),
			P("optsTable.swipable", "boolean", "Enable swipe navigation on content (default true)."),
		},
		Example: `k.form.new("main", {title="Tab Demo", layout="vertical"})
k.ctrl.tab("main", "tabs", {
  tabs = {
    {title = "Dashboard", icon = "📊"},
    {title = "Reports", icon = "📈"},
    {title = "Settings", icon = "⚙️"}
  },
  tab_position = "top",
  swipable = true,
})
k.form.on("main", "tabs", "onchange", function(new_idx, old_idx)
  k.print("Switched from tab " .. old_idx .. " to " .. new_idx)
end)
k.form.show("main")`},

	// tab operations (kforms_enhancements.md §10)
	"tab": {Name: "tab", Group: "controls", Signature: "k.tab", Docs: "Tab control operations: k.tab.set_tab/get_tab/add_tab/remove_tab/..."},
	"tab.set_tab": {Name: "tab.set_tab", Group: "controls", Signature: "k.tab.set_tab(form, name, index)",
		Docs: "Programmatically switch to the tab at 1-based index. Sends a tab_set_tab message so the browser activates the tab.",
		Params: []Param{
			P("form", "string", "Parent form name."),
			P("name", "string", "Tab control name."),
			P("index", "number", "1-based tab index to activate."),
		},
		Example: `k.tab.set_tab("main", "tabs", 2)`},
	"tab.get_tab": {Name: "tab.get_tab", Group: "controls", Signature: "k.tab.get_tab(form, name)",
		Docs:    "Returns the active tab index (1-based) or nil when the tab control is not found.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Tab control name.")},
		Example: `local idx = k.tab.get_tab("main", "tabs")`},
	"tab.get_tab_count": {Name: "tab.get_tab_count", Group: "controls", Signature: "k.tab.get_tab_count(form, name)",
		Docs:    "Returns the number of tabs or nil when the tab control is not found.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Tab control name.")},
		Example: `local n = k.tab.get_tab_count("main", "tabs")`},
	"tab.add_tab": {Name: "tab.add_tab", Group: "controls", Signature: "k.tab.add_tab(form, name, {title, icon?})",
		Docs: "Appends a new tab at the end and re-renders headers/panels.",
		Params: []Param{
			P("form", "string", "Parent form name."),
			P("name", "string", "Tab control name."),
			P("tab", "table", "Tab definition: {title=..., icon=...}."),
		},
		Example: `k.tab.add_tab("main", "tabs", { title = "Help", icon = "❓" })`},
	"tab.insert_tab": {Name: "tab.insert_tab", Group: "controls", Signature: "k.tab.insert_tab(form, name, index, {title, icon?})",
		Docs: "Inserts a new tab at the 1-based position and re-renders headers/panels.",
		Params: []Param{
			P("form", "string", "Parent form name."),
			P("name", "string", "Tab control name."),
			P("index", "number", "1-based insert position."),
			P("tab", "table", "Tab definition: {title=..., icon=...}."),
		},
		Example: `k.tab.insert_tab("main", "tabs", 1, { title = "First", icon = "🔝" })`},
	"tab.remove_tab": {Name: "tab.remove_tab", Group: "controls", Signature: "k.tab.remove_tab(form, name, index)",
		Docs: "Removes the tab at the 1-based position and re-renders headers/panels.",
		Params: []Param{
			P("form", "string", "Parent form name."),
			P("name", "string", "Tab control name."),
			P("index", "number", "1-based tab index to remove."),
		},
		Example: `k.tab.remove_tab("main", "tabs", 3)`},
	"tab.set_tab_title": {Name: "tab.set_tab_title", Group: "controls", Signature: "k.tab.set_tab_title(form, name, index, title)",
		Docs: "Updates the tab title at the 1-based position and re-renders headers.",
		Params: []Param{
			P("form", "string", "Parent form name."),
			P("name", "string", "Tab control name."),
			P("index", "number", "1-based tab index."),
			P("title", "string", "New tab title."),
		},
		Example: `k.tab.set_tab_title("main", "tabs", 1, "Overview")`},
	"tab.set_tab_icon": {Name: "tab.set_tab_icon", Group: "controls", Signature: "k.tab.set_tab_icon(form, name, index, icon)",
		Docs: "Updates the tab icon at the 1-based position and re-renders headers.",
		Params: []Param{
			P("form", "string", "Parent form name."),
			P("name", "string", "Tab control name."),
			P("index", "number", "1-based tab index."),
			P("icon", "string", "New tab icon (emoji or text)."),
		},
		Example: `k.tab.set_tab_icon("main", "tabs", 2, "📉")`},

	// layout controls (kforms_enhancements.md §11)
	"ctrl.topbar": {Name: "ctrl.topbar", Group: "controls", Signature: "k.ctrl.topbar(form, name, optsTable)",
		Docs: "Adds a fixed topbar (kforms_enhancements.md §11.1) spanning the viewport width with app branding, optional user info and a logout affordance. opts: {icon (image path), title, user={name, role, avatar}, logout (function called when the user block is clicked), visible}. Clicking the user block fires the \"logout\" event — the function passed to `logout` is registered as that handler, and k.form.on(form, ctrl, \"logout\", fn) overrides it.",
		Params: []Param{
			P("form", "string", "Parent form name."),
			P("name", "string", "Unique control name."),
			P("optsTable.icon", "string", "Image path (JPG/PNG/SVG) shown at the left of the bar."),
			P("optsTable.title", "string", "App name."),
			P("optsTable.user", "table", "User info: {name=..., role=..., avatar=<image path>}."),
			P("optsTable.logout", "function", "Called when the user clicks the avatar/name (direct logout call)."),
			P("optsTable.visible", "boolean", "Show/hide the topbar (default true)."),
		},
		Example: `k.form.new("main", {title="App", layout="vertical"})
k.ctrl.topbar("main", "bar", {
  icon = "assets/logo.png",
  title = "My App",
  user = { name = "John Doe", role = "admin", avatar = "assets/avatar.png" },
  logout = function() k.quit() end,
})
k.form.show("main")`},

	"ctrl.sidebar": {Name: "ctrl.sidebar", Group: "controls", Signature: "k.ctrl.sidebar(form, name, optsTable)",
		Docs: "Adds a fixed navigation sidebar (kforms_enhancements.md §11.2) anchored to the viewport's left or right edge (position), outside the form flow — like the topbar and footer. On the run page a form with a sidebar fills the whole remaining viewport area between the fixed bars (topbar/footer/sidebar) with an ~3px gap on every side; the sidebar is collapsible to an icon-only strip (session-only, resets on reload, the form frame follows the strip width). opts: {position=\"left\"|\"right\", width, collapsed_width, collapsible, collapsed, items=[{label, icon, action, badge} | {type=\"separator\"}], on_select (global handler called with the clicked item), visible, style={bg, border_right, ...}}. Clicking an item fires the \"sidebar_select\" event with (action, item, index) — payload {action, item, index}.",
		Params: []Param{
			P("form", "string", "Parent form name."),
			P("name", "string", "Unique control name."),
			P("optsTable.position", "string", "\"left\" (default) or \"right\" — which viewport edge the fixed sidebar anchors to."),
			P("optsTable.width", "number", "Expanded width in pixels (default 280)."),
			P("optsTable.collapsed_width", "number", "Collapsed width in pixels (default 64)."),
			P("optsTable.collapsible", "boolean", "Allow the user to collapse/expand (default true)."),
			P("optsTable.collapsed", "boolean", "Initial collapsed state (default false; session only)."),
			P("optsTable.items", "list", "Array of item definitions (see below)."),
			P("optsTable.on_select", "function", "Global selection handler invoked with the clicked item (optional)."),
			P("optsTable.visible", "boolean", "Show/hide the sidebar (default true)."),
			P("optsTable.style", "table", "CSS style overrides, e.g. {bg=..., border_right=...}."),
		},
		Example: `k.form.new("main", {title="App", layout="grid"})
k.ctrl.sidebar("main", "side", {
  position = "left",
  items = {
    { label = "Dashboard", icon = "assets/home.png", action = "nav_dashboard" },
    { type = "separator" },
    { label = "Settings", icon = "assets/settings.png", action = "nav_settings", badge = 5 },
  },
})
k.form.on("main", "side", "sidebar_select", function(action, item, index)
  k.print("selected", action)
end)
k.form.show("main")`},

	"ctrl.footer": {Name: "ctrl.footer", Group: "controls", Signature: "k.ctrl.footer(form, name, optsTable)",
		Docs: "Adds a fixed footer (kforms_enhancements.md §11.3) at the bottom of the viewport with copyright text, optional links, and auto-populated version + script name. opts: {text, version (auto-filled from the KALUA binary when omitted), script_name (auto-filled from the running script), links=[{label, url}], visible, style={bg, height, ...}}. No events.",
		Params: []Param{
			P("form", "string", "Parent form name."),
			P("name", "string", "Unique control name."),
			P("optsTable.text", "string", "Copyright/description text."),
			P("optsTable.version", "string", "App version (auto-filled from KALUA if omitted)."),
			P("optsTable.script_name", "string", "Running script name (auto-filled)."),
			P("optsTable.links", "list", "Array of {label, url} link definitions."),
			P("optsTable.visible", "boolean", "Show/hide the footer (default true)."),
			P("optsTable.style", "table", "CSS style overrides: {bg=..., height=...}."),
		},
		Example: `k.form.new("main", {title="App", layout="vertical"})
k.ctrl.footer("main", "foot", {
  text = "© 2024 My Company",
  version = "1.0.0",
  links = { { label = "Privacy", url = "/privacy" }, { label = "Terms", url = "/terms" } },
})
k.form.show("main")`},

	"ctrl.select_text": {Name: "ctrl.select_text", Group: "controls", Signature: "k.ctrl.select_text(form, name)",
		Docs:    "Selects all text in a textbox or textarea control.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Control name.")},
		Example: `k.ctrl.select_text("main", "notes")`},
	"ctrl.set_selection": {Name: "ctrl.set_selection", Group: "controls", Signature: "k.ctrl.set_selection(form, name, from, to)",
		Docs:    "Sets the selection range in a textbox or textarea (0-based character offsets).",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Control name."), P("from", "number", "0-based start offset."), P("to", "number", "0-based end offset.")},
		Example: `k.ctrl.set_selection("main", "notes", 0, 3)`},
	"ctrl.get_selection": {Name: "ctrl.get_selection", Group: "controls", Signature: "k.ctrl.get_selection(form, name)",
		Docs:    "Returns the current selection as {start, end, text} from a textbox or textarea.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Control name.")},
		Example: `local sel = k.ctrl.get_selection("main", "notes")  -- {start, end, text}`},
	"ctrl.get_item_count": {Name: "ctrl.get_item_count", Group: "controls", Signature: "k.ctrl.get_item_count(form, name)",
		Docs:    "Returns the number of items/rows in a combo, list, radio, or table control.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Control name.")},
		Example: `local n = k.ctrl.get_item_count("main", "sel")`},
	"ctrl.execute_event": {Name: "ctrl.execute_event", Group: "controls", Signature: "k.ctrl.execute_event(form, name, event)",
		Docs:    "Fires a control's event handler as if the user triggered it (e.g., \"onclick\"). Runs asynchronously via the session actor.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Control name."), P("event", "string", "Event name to fire, e.g. \"onclick\".")},
		Example: `k.ctrl.execute_event("main", "btn_ok", "onclick")`},

	// AI chat control (kforms_enhancements.md §12.2)
	"ctrl.ai_chat": {Name: "ctrl.ai_chat", Group: "controls", Signature: "k.ctrl.ai_chat(form, name, optsTable)",
		Docs: "Adds an embeddable AI chat control (kforms_enhancements.md §12.2) that renders inside any form and never blocks the script. It is the same assistant as k.form.ai_agent but event-driven: state lives on the control, so a form may host several chats. opts: {title=\"AI Assistant\", welcome, placeholder=\"Ask me anything…\", system, max_turns=8, tools=false|true|{names}|{schemas}, height=420, show_header=true, show_apikey=false, markdown=true, disabled=false, model, base_url, api_key} plus shared styling/layout keys (cell, align, bg, color, font, font_size, style, visible, class). AI configuration is resolved lazily on the first turn; a turn with no endpoint fires on_error. Events via k.form.on(form, ctrl, \"on_send|on_reply|on_tool|on_error|on_done\", fn). k.ctrl.get_value returns the last assistant text; set_value is a no-op.",
		Params: []Param{
			P("form", "string", "Parent form name."),
			P("name", "string", "Unique control name."),
			P("optsTable.title", "string", "Header text (default \"AI Assistant\")."),
			P("optsTable.welcome", "string", "Assistant message seeded into the transcript at construction."),
			P("optsTable.placeholder", "string", "Input placeholder (default \"Ask me anything…\")."),
			P("optsTable.system", "string", "Extra system prompt (persona, domain rules)."),
			P("optsTable.max_turns", "number", "Tool-loop bound, min 1 (default 8)."),
			P("optsTable.tools", "bool|table", "false (default): no tools. true: auto-expose eligible globals. {name,...}: allowlist. {{name,desc,params},...}: explicit schemas."),
			P("optsTable.height", "string|number", "Panel height; numbers become px, strings used verbatim (default 420)."),
			P("optsTable.show_header", "boolean", "Render the header (default true)."),
			P("optsTable.show_apikey", "boolean", "Render the in-panel API-key input (default false)."),
			P("optsTable.markdown", "boolean", "Render assistant bubbles as Markdown (default true)."),
			P("optsTable.disabled", "boolean", "Disable input/send (default false)."),
		},
		Example: `k.ctrl.ai_chat("main", "assistant", {
  title      = "Customer assistant",
  welcome    = "Ask me to load a customer, e.g. *load customer 7*.",
  system     = "You load customers into the form. Always call fill_customer.",
  tools      = { "find_customer", "fill_customer" },
  height     = "100%",
  placeholder = "load customer 7…",
})
k.form.on("main", "assistant", "on_tool", function(name, args, result)
  k.print("ai tool " .. name .. " -> " .. k.jsonencode(result))
end)`},

	// ai_chat operations (kforms_enhancements.md §12.2)
	"ai_chat.send": {Name: "ai_chat.send", Group: "controls", Signature: "k.ai_chat.send(form, name, text[, api_key])",
		Docs: "Starts a turn through the same path as a browser send, including the on_send veto. Asynchronous; returns nothing.",
		Params: []Param{
			P("form", "string", "Parent form name."),
			P("name", "string", "AI chat control name."),
			P("text", "string", "User message to send."),
			P("api_key", "string", "Optional per-send API key override."),
		},
		Example: `k.ai_chat.send("main", "assistant", "load customer 7")`},
	"ai_chat.stop": {Name: "ai_chat.stop", Group: "controls", Signature: "k.ai_chat.stop(form, name)",
		Docs:    "Cancels the in-flight turn; partial assistant text is kept.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "AI chat control name.")},
		Example: `k.ai_chat.stop("main", "assistant")`},
	"ai_chat.add_message": {Name: "ai_chat.add_message", Group: "controls", Signature: "k.ai_chat.add_message(form, name, role, content[, name])",
		Docs: "Appends a {role, content, name?} message to the transcript and pushes an ai_chat_message update.",
		Params: []Param{
			P("form", "string", "Parent form name."),
			P("name", "string", "AI chat control name."),
			P("role", "string", "\"system\", \"user\", \"assistant\" or \"tool\"."),
			P("content", "string", "Message text."),
			P("name", "string", "Optional tool name for role=\"tool\" messages."),
		},
		Example: `k.ai_chat.add_message("main", "assistant", "assistant", "Done.")`},
	"ai_chat.get_transcript": {Name: "ai_chat.get_transcript", Group: "controls", Signature: "k.ai_chat.get_transcript(form, name) → table",
		Docs:    "Returns the transcript array of {role, content, name?} messages.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "AI chat control name.")},
		Example: `local msgs = k.ai_chat.get_transcript("main", "assistant")`},
	"ai_chat.set_transcript": {Name: "ai_chat.set_transcript", Group: "controls", Signature: "k.ai_chat.set_transcript(form, name, messages)",
		Docs: "Replaces the transcript and pushes an ai_chat_transcript update to the browser.",
		Params: []Param{
			P("form", "string", "Parent form name."),
			P("name", "string", "AI chat control name."),
			P("messages", "list", "Array of {role, content, name?} message tables."),
		},
		Example: `k.ai_chat.set_transcript("main", "assistant", {{role="system", content="You are helpful."}})`},
	"ai_chat.clear": {Name: "ai_chat.clear", Group: "controls", Signature: "k.ai_chat.clear(form, name)",
		Docs:    "Clears the transcript (set_transcript({})).",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "AI chat control name.")},
		Example: `k.ai_chat.clear("main", "assistant")`},
	"ai_chat.set_system": {Name: "ai_chat.set_system", Group: "controls", Signature: "k.ai_chat.set_system(form, name, text)",
		Docs:    "Replaces the system prompt for subsequent turns.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "AI chat control name."), P("text", "string", "New system prompt text.")},
		Example: `k.ai_chat.set_system("main", "assistant", "You are a support agent.")`},
	"ai_chat.is_busy": {Name: "ai_chat.is_busy", Group: "controls", Signature: "k.ai_chat.is_busy(form, name) → boolean",
		Docs:    "Returns whether a turn is in flight for the control.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "AI chat control name.")},
		Example: `if k.ai_chat.is_busy("main", "assistant") then k.print("working…") end)`},

	// looper operations
	"looper": {Name: "looper", Group: "controls", Signature: "k.looper", Docs: "Looper control operations: k.looper.link_db/set_db_source/refresh/..."},
	"looper.link_db": {Name: "looper.link_db", Group: "controls", Signature: "k.looper.link_db(form, name, opts)",
		Docs: "Attaches a DB source to a looper: {db,query,links,page_size?,count_query?,where?,order_by?}. links list maps result columns to template controls: {column=N,control,property} by 1-based index or {field,col,control,property} by name.",
		Params: []Param{
			P("form", "string", "Parent form name."),
			P("name", "string", "Looper control name."),
			P("opts.db", "string", "DB handle."),
			P("opts.query", "string", "SQL query producing the rows."),
			P("opts.links", "list", "Column→control mappings: {field/column, control, property}."),
			P("opts.page_size", "number", "Rows per page for pagination."),
			P("opts.count_query", "string", "Optional query returning total row count."),
			P("opts.where", "string", "Optional WHERE clause appended to query."),
			P("opts.order_by", "string", "Optional ORDER BY clause."),
		},
		Example: `k.looper.link_db("main", "rows", {
  db = h, query = "SELECT * FROM items",
  links = { {field = "name", control = "tpl_name", property = "text"} },
})`},
	"looper.set_db_source": {Name: "looper.set_db_source", Group: "controls", Signature: "k.looper.set_db_source(form, name, opts)",
		Docs:    "Swaps a DB-linked looper's source {db,query,links?,page_size?,count_query?,where?,order_by?} and refreshes.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Looper control name."), P("opts", "table", "New source: {db, query, links, page_size, count_query, where, order_by}.")},
		Example: `k.looper.set_db_source("main", "rows", { db = h, query = "SELECT * FROM archived" })`},
	"looper.refresh": {Name: "looper.refresh", Group: "controls", Signature: "k.looper.refresh(form, name)",
		Docs:    "Re-runs a DB-linked looper's query and shows page 1.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Looper control name.")},
		Example: `k.looper.refresh("main", "rows")`},
	"looper.add_line": {Name: "looper.add_line", Group: "controls", Signature: "k.looper.add_line(form, name, valuesTable)",
		Docs:    "Raises a runtime error on DB-linked loopers (rows come from the linked query).",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Looper control name."), P("valuesTable", "table", "Row values (unused on DB-linked loopers).")},
		Example: `k.looper.add_line("main", "rows", { name = "new" })`},
	"looper.set_line": {Name: "looper.set_line", Group: "controls", Signature: "k.looper.set_line(form, name, index, valuesTable)",
		Docs:    "Raises a runtime error on DB-linked loopers (rows come from the linked query).",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Looper control name."), P("index", "number", "1-based row index."), P("valuesTable", "table", "Row values (unused on DB-linked loopers).")},
		Example: `k.looper.set_line("main", "rows", 1, { name = "x" })`},
	"looper.delete_line": {Name: "looper.delete_line", Group: "controls", Signature: "k.looper.delete_line(form, name, index)",
		Docs:    "Raises a runtime error on DB-linked loopers (rows come from the linked query).",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Looper control name."), P("index", "number", "1-based row index (ignored on DB-linked loopers).")},
		Example: `k.looper.delete_line("main", "rows", 2)`},

	// chart operations
	"chart": {Name: "chart", Group: "controls", Signature: "k.chart", Docs: "Chart control operations: k.chart.set_data/add_dataset/..."},
	"chart.set_data": {Name: "chart.set_data", Group: "controls", Signature: "k.chart.set_data(form, name, {labels, datasets})",
		Docs:    "Bulk replaces a chart's labels and datasets.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Chart control name."), P("opts.labels", "list", "New X-axis labels."), P("opts.datasets", "list", "New dataset tables.")},
		Example: `k.chart.set_data("dash", "trend", { labels = {"A","B"}, datasets = { {label = "S", data = {1,2}} } })`},
	"chart.add_dataset": {Name: "chart.add_dataset", Group: "controls", Signature: "k.chart.add_dataset(form, name, dataset)",
		Docs:    "Appends a dataset {label, data, backgroundColor?, borderColor?, fill?, tension?, ...} to a chart.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Chart control name."), P("dataset", "table", "Dataset: {label, data, backgroundColor, borderColor, fill, tension, ...}.")},
		Example: `k.chart.add_dataset("dash", "trend", { label = "Clicks", data = {5, 8, 12} })`},
	"chart.remove_dataset": {Name: "chart.remove_dataset", Group: "controls", Signature: "k.chart.remove_dataset(form, name, index)",
		Docs:    "Removes a dataset by 1-based index.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Chart control name."), P("index", "number", "1-based dataset index.")},
		Example: `k.chart.remove_dataset("dash", "trend", 1)`},
	"chart.update_dataset": {Name: "chart.update_dataset", Group: "controls", Signature: "k.chart.update_dataset(form, name, index, dataset)",
		Docs:    "Replaces the dataset at 1-based index.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Chart control name."), P("index", "number", "1-based dataset index."), P("dataset", "table", "New dataset table.")},
		Example: `k.chart.update_dataset("dash", "trend", 2, { label = "B", data = {3, 4, 1} })`},
	"chart.set_labels": {Name: "chart.set_labels", Group: "controls", Signature: "k.chart.set_labels(form, name, labels)",
		Docs:    "Replaces the chart's X-axis labels (array).",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Chart control name."), P("labels", "list", "New label array.")},
		Example: `k.chart.set_labels("dash", "trend", {"Jan", "Feb", "Mar"})`},
	"chart.set_options": {Name: "chart.set_options", Group: "controls", Signature: "k.chart.set_options(form, name, options)",
		Docs:    "Merges Chart.js options (scales, plugins, ...) into the chart.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Chart control name."), P("options", "table", "Chart.js options to deep-merge.")},
		Example: `k.chart.set_options("dash", "trend", { scales = { y = { beginAtZero = true } } })`},
	"chart.get_image": {Name: "chart.get_image", Group: "controls", Signature: "k.chart.get_image(form, name)",
		Docs:    "Renders the chart canvas to a base64 PNG data URL.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Chart control name.")},
		Example: `local png = k.chart.get_image("dash", "trend")  -- "data:image/png;base64,..."`},
	"chart.resize": {Name: "chart.resize", Group: "controls", Signature: "k.chart.resize(form, name, width, height)",
		Docs:    "Resizes the chart canvas to the given pixel dimensions.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Chart control name."), P("width", "number", "New width in px."), P("height", "number", "New height in px.")},
		Example: `k.chart.resize("dash", "trend", 800, 400)`},

	// grid operations (kforms_enhancements.md §7)
	"grid": {Name: "grid", Group: "controls", Signature: "k.grid", Docs: "Grid control operations: k.grid.refresh/set_db_source/..."},
	"grid.refresh": {Name: "grid.refresh", Group: "controls", Signature: "k.grid.refresh(form, name)",
		Docs:    "Re-runs a grid's data query and shows page 1.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Grid control name.")},
		Example: `k.grid.refresh("main", "users")`},
	"grid.set_db_source": {Name: "grid.set_db_source", Group: "controls", Signature: "k.grid.set_db_source(form, name, opts)",
		Docs:    "Swaps a grid's data source {db,query,columns?,page_size?,count_query?,where?,order_by?,pk_field?,selection_mode?} and refreshes.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Grid control name."), P("opts", "table", "New source: {db, query, columns, page_size, count_query, where, order_by, pk_field, selection_mode}.")},
		Example: `k.grid.set_db_source("main", "users", { db = "main", query = "SELECT * FROM users" })`},

	// CRUD Grid operations (kforms_enhancements.md §7)
	"grid.get_selected": {Name: "grid.get_selected", Group: "controls", Signature: "k.grid.get_selected(form, name)",
		Docs:    "Returns the selected rows (async). Yields until the browser responds with the selected row data. Returns a list of row objects (each a map of column→value) or nil if none selected.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Grid control name.")},
		Example: `local selected = k.grid.get_selected("main", "users"); if selected then for _, row in ipairs(selected) do print(row.id) end end`},
	"grid.get_row": {Name: "grid.get_row", Group: "controls", Signature: "k.grid.get_row(form, name, pk)",
		Docs:    "Fetches a single row by primary key (async). Yields until the DB query completes. Returns a row object (map of column→value) or nil if not found.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Grid control name."), P("pk", "any", "Primary key value.")},
		Example: `local row = k.grid.get_row("main", "users", 42); if row then print(row.name) end`},
	"grid.delete_row": {Name: "grid.delete_row", Group: "controls", Signature: "k.grid.delete_row(form, name, pk)",
		Docs:    "Deletes a single row by primary key (async). Triggers tabulator_refresh on success. Returns boolean success or nil + error message.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Grid control name."), P("pk", "any", "Primary key value.")},
		Example: `local ok = k.grid.delete_row("main", "users", 42); if ok then print("deleted") end`},
	"grid.batch_delete": {Name: "grid.batch_delete", Group: "controls", Signature: "k.grid.batch_delete(form, name, pksTable)",
		Docs:    "Deletes multiple rows by primary keys (async). Triggers tabulator_refresh on success. Returns boolean success or nil + error message.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Grid control name."), P("pksTable", "list", "Array of primary key values.")},
		Example: `local ok = k.grid.batch_delete("main", "users", {42, 43, 44})`},
	"grid.insert_row": {Name: "grid.insert_row", Group: "controls", Signature: "k.grid.insert_row(form, name, dataTable)",
		Docs:    "Inserts a new row (async). Returns the inserted row's primary key (if available) or true on success, or nil + error message.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Grid control name."), P("dataTable", "table", "Column→value map for the new row.")},
		Example: `local pk = k.grid.insert_row("main", "users", {name="Alice", email="alice@example.com"})`},
	"grid.update_row": {Name: "grid.update_row", Group: "controls", Signature: "k.grid.update_row(form, name, pk, dataTable)",
		Docs:    "Updates a row by primary key (async). Triggers tabulator_refresh on success. Returns boolean success or nil + error message.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Grid control name."), P("pk", "any", "Primary key value."), P("dataTable", "table", "Column→value map for the updated columns (PK column is ignored).")},
		Example: `local ok = k.grid.update_row("main", "users", 42, {email="new@example.com"})`},

	// table operations
	"table": {Name: "table", Group: "controls", Signature: "k.table", Docs: "Table control operations: k.table.add_line/delete_line/..."},
	"table.add_line": {Name: "table.add_line", Group: "controls", Signature: "k.table.add_line(form, name, valuesTable)",
		Docs:    "Appends a row to a table control.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Table control name."), P("valuesTable", "list", "Cell values in column order.")},
		Example: `k.table.add_line("main", "grid", {"Alice", 32})`},
	"table.delete_line": {Name: "table.delete_line", Group: "controls", Signature: "k.table.delete_line(form, name, index)",
		Docs:    "Removes the row at index.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Table control name."), P("index", "number", "1-based row index.")},
		Example: `k.table.delete_line("main", "grid", 1)`},
	"table.set_column_value": {Name: "table.set_column_value", Group: "controls", Signature: "k.table.set_column_value(form, name, row, column, value)",
		Docs:    "Sets a cell value in a table control.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Table control name."), P("row", "number", "1-based row index."), P("column", "number", "1-based column index."), P("value", "any", "New cell value.")},
		Example: `k.table.set_column_value("main", "grid", 1, 2, 33)`},
	"table.get_column_value": {Name: "table.get_column_value", Group: "controls", Signature: "k.table.get_column_value(form, name, row, column)",
		Docs:    "Gets a cell value from a table control.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Table control name."), P("row", "number", "1-based row index."), P("column", "number", "1-based column index.")},
		Example: `local v = k.table.get_column_value("main", "grid", 1, 2)`},
	"table.get_selected_column": {Name: "table.get_selected_column", Group: "controls", Signature: "k.table.get_selected_column(form, name)",
		Docs:    "Gets the currently selected column.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Table control name.")},
		Example: `local col = k.table.get_selected_column("main", "grid")`},
	"table.set_selected_column": {Name: "table.set_selected_column", Group: "controls", Signature: "k.table.set_selected_column(form, name, column)",
		Docs:    "Sets the selected column.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Table control name."), P("column", "number", "1-based column index.")},
		Example: `k.table.set_selected_column("main", "grid", 2)`},
	"table.set_data": {Name: "table.set_data", Group: "controls", Signature: "k.table.set_data(form, name, dataTable)",
		Docs:    "Bulk replaces all row data (Tabulator mode pushes tabulator_update).",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Table control name."), P("dataTable", "list", "Rows: list of value lists, or of row-maps (Tabulator mode).")},
		Example: `k.table.set_data("main", "grid", { {"Alice", 32}, {"Bob", 41} })`},
	"table.get_data": {Name: "table.get_data", Group: "controls", Signature: "k.table.get_data(form, name)",
		Docs:    "Returns all current data of a table control.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Table control name.")},
		Example: `local rows = k.table.get_data("main", "grid")`},
	"table.get_selected_rows": {Name: "table.get_selected_rows", Group: "controls", Signature: "k.table.get_selected_rows(form, name)",
		Docs:    "Returns the selected row indices (1-based).",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Table control name.")},
		Example: `local sel = k.table.get_selected_rows("main", "grid")  -- {1, 3}`},
	"table.set_remote_data": {Name: "table.set_remote_data", Group: "controls", Signature: "k.table.set_remote_data(form, name, {data,last_page,last_row})",
		Docs:    "Pushes server-side pagination data to a tabulator table =  {data=rows, last_page=n} or {data=rows, last_row=n}.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Table control name."), P("opts.data", "list", "Page rows."), P("opts.last_page", "number", "Total page count (page-based pagination)."), P("opts.last_row", "number", "Total row count (row-based pagination).")},
		Example: `k.table.set_remote_data("main", "grid", { data = rows, last_page = 5 })`},
	"table.refresh": {Name: "table.refresh", Group: "controls", Signature: "k.table.refresh(form, name)",
		Docs:    "Re-runs a DB-linked tabulator table's query and shows page 1.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Table control name.")},
		Example: `k.table.refresh("main", "grid")`},
	"table.set_db_source": {Name: "table.set_db_source", Group: "controls", Signature: "k.table.set_db_source(form, name, opts)",
		Docs:    "Swaps a DB-linked tabulator table's source {db,query,columns?,page_size?,count_query?,where?,order_by?} and refreshes.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Table control name."), P("opts", "table", "New source: {db, query, columns, page_size, count_query, where, order_by}.")},
		Example: `k.table.set_db_source("main", "grid", { db = h, query = "SELECT * FROM archived" })`},
	"table.find": {Name: "table.find", Group: "controls", Signature: "k.table.find(form, name, value)",
		Docs:    "Searches for a row where any cell equals value. Returns 1-based row index or nil. Works for both traditional tables and tabulator tables.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Table control name."), P("value", "any", "Value to search for.")},
		Example: `local idx = k.table.find("main", "grid", "Alice")`},

	// database
	"connect_db": {Name: "connect_db", Group: "database", Signature: "k.connect_db(dsn)",
		Docs:    "Opens a database connection (DSN scheme: sqlite://, mysql://, postgres://, sqlserver://) and returns a handle. Supported drivers: SQLite (built-in), MySQL (github.com/go-sql-driver/mysql), PostgreSQL (github.com/jackc/pgx/v5/stdlib), SQL Server (github.com/microsoft/go-mssqldb). SQLite paths resolve through the filesystem sandbox (workdir + --allow-fs); use a path inside it.",
		Params:  []Param{P("dsn", "string", "Connection string, e.g. \"sqlite://data.db\" or \"postgres://user:pw@host/db\".")},
		Example: `local h = k.connect_db("sqlite://app.db")`},
	"disconnect_db": {Name: "disconnect_db", Group: "database", Signature: "k.disconnect_db([handle])",
		Docs:    "Closes a database handle. With no argument closes all handles including --db prereregistered connections.",
		Params:  []Param{P("handle", "string", "Optional handle from k.connect_db; omitted closes all.")},
		Example: `k.disconnect_db(h)`},
	"sql": {Name: "sql", Group: "database", Signature: "k.sql(handle, query, ...params)",
		Docs:    "Executes arbitrary SQL; returns rows or {rows_affected}.",
		Params:  []Param{P("handle", "string", "Connection handle."), P("query", "string", "SQL statement (may contain ? placeholders)."), P("...params", "any", "Bind values for placeholders.")},
		Example: `local res = k.sql(h, "SELECT * FROM items WHERE stock < ?", 5)`},
	"db_select": {Name: "db_select", Group: "database", Signature: "k.db_select(handle, table, fieldsTable, whereTable, order)",
		Docs:    "Query builder returning {columns, rows}.",
		Params:  []Param{P("handle", "string", "Connection handle."), P("table", "string", "Table name."), P("fieldsTable", "list", "Columns to select (or \"*\" as a string)."), P("whereTable", "table", "Equality filters {column = value}."), P("order", "string", "Optional ORDER BY clause.")},
		Example: `local res = k.db_select(h, "items", {"name", "price"}, {category = "x"}, "price DESC")`},
	"db_insert": {Name: "db_insert", Group: "database", Signature: "k.db_insert(handle, table, keyvalsTable)",
		Docs:    "Inserts a row; returns {last_insert_id, rows_affected}.",
		Params:  []Param{P("handle", "string", "Connection handle."), P("table", "string", "Table name."), P("keyvalsTable", "table", "{column = value} map to insert.")},
		Example: `local r = k.db_insert(h, "items", { name = "widget", price = 9.9 })`},
	"db_update": {Name: "db_update", Group: "database", Signature: "k.db_update(handle, table, keyvalsTable, whereTable)",
		Docs:    "Updates rows matching the where table.",
		Params:  []Param{P("handle", "string", "Connection handle."), P("table", "string", "Table name."), P("keyvalsTable", "table", "{column = value} map to set."), P("whereTable", "table", "Equality filters {column = value}.")},
		Example: `k.db_update(h, "items", { price = 11.5 }, { name = "widget" })`},
	"db_delete": {Name: "db_delete", Group: "database", Signature: "k.db_delete(handle, table, whereTable)",
		Docs:    "Deletes rows matching the where table.",
		Params:  []Param{P("handle", "string", "Connection handle."), P("table", "string", "Table name."), P("whereTable", "table", "Equality filters {column = value}.")},
		Example: `k.db_delete(h, "items", { name = "widget" })`},
	"tx_begin": {Name: "tx_begin", Group: "database", Signature: "k.tx_begin(handle)",
		Docs:    "Starts a transaction on a connection.",
		Params:  []Param{P("handle", "string", "Connection handle.")},
		Example: `k.tx_begin(h)`},
	"tx_commit": {Name: "tx_commit", Group: "database", Signature: "k.tx_commit(handle)",
		Docs:    "Commits the active transaction.",
		Params:  []Param{P("handle", "string", "Connection handle.")},
		Example: `k.tx_commit(h)`},
	"tx_rollback": {Name: "tx_rollback", Group: "database", Signature: "k.tx_rollback(handle)",
		Docs:    "Rolls back the active transaction.",
		Params:  []Param{P("handle", "string", "Connection handle.")},
		Example: `k.tx_rollback(h)`},
	"rows": {Name: "rows", Group: "database", Signature: "k.rows(result)",
		Docs:    "Returns an iterator over a query result's rows.",
		Params:  []Param{P("result", "table", "Result set from k.sql/k.db_select.")},
		Example: `for row in k.rows(res) do k.print(row.name) end`},

	// files
	"file_open": {Name: "file_open", Group: "files", Signature: "k.file_open(path[, mode])",
		Docs:    "Opens a file; mode is r, r+, w, w+, a or a+ (default r). Returns a handle.",
		Params:  []Param{P("path", "string", "File path to open."), P("mode", "string", "Access mode: r, r+, w, w+, a, a+ (default r).")},
		Example: `local f = k.file_open("/tmp/data.txt", "w")`},
	"file_read": {Name: "file_read", Group: "files", Signature: "k.file_read(handle[, count])",
		Docs:    "Reads the whole file or count bytes; empty string at EOF.",
		Params:  []Param{P("handle", "string", "Open file handle."), P("count", "number", "Optional byte count to read.")},
		Example: `local all = k.file_read(f)`},
	"file_read_line": {Name: "file_read_line", Group: "files", Signature: "k.file_read_line(handle)",
		Docs:    "Reads one line (trailing newline trimmed); nil at EOF.",
		Params:  []Param{P("handle", "string", "Open file handle.")},
		Example: `local line = k.file_read_line(f)`},
	"file_write": {Name: "file_write", Group: "files", Signature: "k.file_write(handle, data)",
		Docs:    "Writes data to an open file.",
		Params:  []Param{P("handle", "string", "Open file handle."), P("data", "string", "Text to write.")},
		Example: `k.file_write(f, "hello\n")`},
	"file_close": {Name: "file_close", Group: "files", Signature: "k.file_close(handle)",
		Docs:    "Closes an open file handle.",
		Params:  []Param{P("handle", "string", "Open file handle.")},
		Example: `k.file_close(f)`},
	"file_load": {Name: "file_load", Group: "files", Signature: "k.file_load(path)",
		Docs:    "Reads an entire file as a string (async; max 16 MiB).",
		Params:  []Param{P("path", "string", "File path to read.")},
		Example: `local text = k.file_load("/tmp/data.txt")`},
	"file_save": {Name: "file_save", Group: "files", Signature: "k.file_save(path, data)",
		Docs:    "Writes a file atomically (async; temp file + rename).",
		Params:  []Param{P("path", "string", "Destination path."), P("data", "string", "Content to write.")},
		Example: `k.file_save("/tmp/out.txt", "content")`},
	"file_copy": {Name: "file_copy", Group: "files", Signature: "k.file_copy(src, dst)",
		Docs:    "Copies a file, preserving permissions.",
		Params:  []Param{P("src", "string", "Source path."), P("dst", "string", "Destination path.")},
		Example: `k.file_copy("/tmp/a.txt", "/tmp/b.txt")`},
	"file_move": {Name: "file_move", Group: "files", Signature: "k.file_move(src, dst)",
		Docs:    "Moves/renames a file.",
		Params:  []Param{P("src", "string", "Source path."), P("dst", "string", "Destination path.")},
		Example: `k.file_move("/tmp/a.txt", "/tmp/b.txt")`},
	"file_delete": {Name: "file_delete", Group: "files", Signature: "k.file_delete(path)",
		Docs:    "Deletes a file.",
		Params:  []Param{P("path", "string", "Path to delete.")},
		Example: `k.file_delete("/tmp/junk.txt")`},
	"file_exists": {Name: "file_exists", Group: "files", Signature: "k.file_exists(path)",
		Docs:    "Reports whether a path exists.",
		Params:  []Param{P("path", "string", "Path to test.")},
		Example: `if k.file_exists("/tmp/data.txt") then ... end`},
	"file_mkdir": {Name: "file_mkdir", Group: "files", Signature: "k.file_mkdir(path[, parents])",
		Docs:    "Creates a directory; parents=true creates intermediate dirs.",
		Params:  []Param{P("path", "string", "Directory to create."), P("parents", "boolean", "Create intermediate directories (default false).")},
		Example: `k.file_mkdir("/tmp/a/b/c", true)`},
	"file_list": {Name: "file_list", Group: "files", Signature: "k.file_list(dir)",
		Docs:    "Lists a directory as a 1-based, sorted table of names.",
		Params:  []Param{P("dir", "string", "Directory to list.")},
		Example: `for _, name in ipairs(k.file_list("/tmp")) do k.print(name) end`},
	"file_info": {Name: "file_info", Group: "files", Signature: "k.file_info(path)",
		Docs:    "Returns {name, size, is_dir, modified} for a path.",
		Params:  []Param{P("path", "string", "Path to inspect.")},
		Example: `local info = k.file_info("/tmp/data.txt")`},

	// json
	"json_parse": {Name: "json_parse", Group: "json", Signature: "k.json_parse(text)",
		Docs:    "Parses JSON text; null maps to K.NULL.",
		Params:  []Param{P("text", "string", "JSON document to parse.")},
		Example: `local root = k.json_parse('{"name": "Alice", "age": 32}')`},
	"json_string": {Name: "json_string", Group: "json", Signature: "k.json_string(value)",
		Docs:    "Encodes a value as compact JSON (sorted keys).",
		Params:  []Param{P("value", "any", "Value to serialize (tables, strings, numbers, booleans, nil).")},
		Example: `local text = k.json_string({ name = "Alice", age = 32 })`},
	"json_load": {Name: "json_load", Group: "json", Signature: "k.json_load(path)",
		Docs:    "Reads and parses a JSON file (async; max 16 MiB).",
		Params:  []Param{P("path", "string", "Path of the JSON file.")},
		Example: `local root = k.json_load("config.json")`},
	"json_save": {Name: "json_save", Group: "json", Signature: "k.json_save(path, value)",
		Docs:    "Encodes a value and writes it atomically (async).",
		Params:  []Param{P("path", "string", "Destination path."), P("value", "any", "Value to serialize.")},
		Example: `k.json_save("out.json", { ok = true })`},
	"json_get": {Name: "json_get", Group: "json", Signature: "k.json_get(root, path)",
		Docs:    "Walks a dot/bracket path over a parsed value, e.g. \"a.b[0].c\".",
		Params:  []Param{P("root", "any", "Parsed document (from k.json_parse/k.json_load)."), P("path", "string", "Dot/bracket path, e.g. \"a.b[0].c\".")},
		Example: `local v = k.json_get(root, "a.b[0].c")`},
	"json_array_item": {Name: "json_array_item", Group: "json", Signature: "k.json_array_item(root, path, index)",
		Docs:    "Returns the element at a 0-based array index.",
		Params:  []Param{P("root", "any", "Parsed document."), P("path", "string", "Path to an array (or \"\" for root)."), P("index", "number", "0-based element index.")},
		Example: `local first = k.json_array_item(root, "tags", 0)`},
	"json_count": {Name: "json_count", Group: "json", Signature: "k.json_count(root, path)",
		Docs:    "Returns the element count (array length or object size).",
		Params:  []Param{P("root", "any", "Parsed document."), P("path", "string", "Path to an array or object.")},
		Example: `local n = k.json_count(root, "items")`},
	"json_names": {Name: "json_names", Group: "json", Signature: "k.json_names(root, path)",
		Docs:    "Returns a 1-based table of keys or indices.",
		Params:  []Param{P("root", "any", "Parsed document."), P("path", "string", "Path to an array or object.")},
		Example: `for _, key in ipairs(k.json_names(root, "items")) do ... end`},
	"is_null": {Name: "is_null", Group: "json", Signature: "k.is_null(value)",
		Docs:    "Reports whether value is the K.NULL sentinel.",
		Params:  []Param{P("value", "any", "Value to test.")},
		Example: `if k.is_null(v) then ... end`},

	// crypto
	"checksum": {Name: "checksum", Group: "crypto", Signature: "k.checksum(alg, data[, key[, salt[, iterations[, keylen]]]])", Docs: "Hex hash for alg: crc32, md5, sha1, sha256, hmac-sha256 (requires key), pbkdf2 (requires salt)."},
	"encrypt":  {Name: "encrypt", Group: "crypto", Signature: "k.encrypt(plaintext, key)", Docs: "AES-GCM encryption; returns base64(nonce || ciphertext)."},
	"decrypt":  {Name: "decrypt", Group: "crypto", Signature: "k.decrypt(b64, key)", Docs: "Reverse of k.encrypt."},

	// xml
	"xml_parse": {Name: "xml_parse", Group: "xml", Signature: "k.xml_parse(text)",
		Docs:    "Parses XML text and returns a document handle.",
		Params:  []Param{P("text", "string", "XML document to parse.")},
		Example: `local doc = k.xml_parse("<book><author>A</author></book>")`},
	"xml_root": {Name: "xml_root", Group: "xml", Signature: "k.xml_root(doc)",
		Docs:    "Returns the root element name of a parsed document.",
		Params:  []Param{P("doc", "any", "Document handle from k.xml_parse.")},
		Example: `local name = k.xml_root(doc)  -- "book"`},
	"xml_child": {Name: "xml_child", Group: "xml", Signature: "k.xml_child(doc, path)",
		Docs:    "Returns child element at path (e.g., \"book/author\").",
		Params:  []Param{P("doc", "any", "Document handle."), P("path", "string", "Element path, e.g. \"book/author\".")},
		Example: `local el = k.xml_child(doc, "book/author")`},
	"xml_child_list": {Name: "xml_child_list", Group: "xml", Signature: "k.xml_child_list(doc, path)",
		Docs:    "Returns a table of child elements at path.",
		Params:  []Param{P("doc", "any", "Document handle."), P("path", "string", "Element path.")},
		Example: `local els = k.xml_child_list(doc, "catalog/cd")`},
	"xml_attr": {Name: "xml_attr", Group: "xml", Signature: "k.xml_attr(doc, path, name)",
		Docs:    "Returns attribute value at path.",
		Params:  []Param{P("doc", "any", "Document handle."), P("path", "string", "Element path."), P("name", "string", "Attribute name.")},
		Example: `local id = k.xml_attr(doc, "catalog/cd", "id")`},
	"xml_content": {Name: "xml_content", Group: "xml", Signature: "k.xml_content(doc, path)",
		Docs:    "Returns text content of element at path.",
		Params:  []Param{P("doc", "any", "Document handle."), P("path", "string", "Element path.")},
		Example: `local title = k.xml_content(doc, "catalog/cd/title")`},
	"xml_attrs": {Name: "xml_attrs", Group: "xml", Signature: "k.xml_attrs(doc, path)",
		Docs:    "Returns all attributes of element at path as a table.",
		Params:  []Param{P("doc", "any", "Document handle."), P("path", "string", "Element path.")},
		Example: `local attrs = k.xml_attrs(doc, "catalog/cd")`},
	"xml_name": {Name: "xml_name", Group: "xml", Signature: "k.xml_name(doc, path)",
		Docs:    "Returns the name of the element at path.",
		Params:  []Param{P("doc", "any", "Document handle."), P("path", "string", "Element path.")},
		Example: `local name = k.xml_name(doc, "catalog/cd")`},

	// server (serve mode)
	"shared":       {Name: "shared", Group: "server", Signature: "k.shared", Docs: "Shared state across workers: k.shared.set/get/del/keys/incr."},
	"shared.set":   {Name: "shared.set", Group: "server", Signature: "k.shared.set(key, value)", Docs: "Stores a string value in shared state."},
	"shared.get":   {Name: "shared.get", Group: "server", Signature: "k.shared.get(key)", Docs: "Retrieves a value from shared state; empty string if missing."},
	"shared.del":   {Name: "shared.del", Group: "server", Signature: "k.shared.del(key)", Docs: "Deletes a key from shared state."},
	"shared.keys":  {Name: "shared.keys", Group: "server", Signature: "k.shared.keys([pattern])", Docs: "Returns all keys matching pattern (prefix, * = all)."},
	"shared.incr":  {Name: "shared.incr", Group: "server", Signature: "k.shared.incr(key[, delta])", Docs: "Increments a numeric key by delta (default 1); returns new value."},
	"ws":           {Name: "ws", Group: "server", Signature: "k.ws", Docs: "WebSocket operations: k.ws.broadcast/send/close."},
	"ws.broadcast": {Name: "ws.broadcast", Group: "server", Signature: "k.ws.broadcast(message)", Docs: "Broadcasts a text message to all connected WebSocket clients."},
	"ws.send":      {Name: "ws.send", Group: "server", Signature: "k.ws.send(client_id, message)", Docs: "Sends a text message to a specific WebSocket client."},
	"ws.close":     {Name: "ws.close", Group: "server", Signature: "k.ws.close(client_id)", Docs: "Closes a WebSocket connection."},
	"tcp":          {Name: "tcp", Group: "server", Signature: "k.tcp", Docs: "TCP operations: k.tcp.send/close/accept."},
	"tcp.send":     {Name: "tcp.send", Group: "server", Signature: "k.tcp.send(client_id, data)", Docs: "Sends data to a specific TCP client."},
	"tcp.close":    {Name: "tcp.close", Group: "server", Signature: "k.tcp.close(client_id)", Docs: "Closes a TCP connection."},
	"tcp.accept":   {Name: "tcp.accept", Group: "server", Signature: "k.tcp.accept()", Docs: "Waits for an incoming TCP connection and returns {id}. The connection can then be used with k.tcp.send and k.tcp.close. (Serve mode only.)"},

	// Phase 1: declarative REST router + HTTP helpers (serve mode)
	"http_routes": {Name: "http_routes", Group: "server", Signature: "k.http_routes = {...}",
		Docs: "Declarative REST routes for serve mode. Each entry is {method, path, handler} where path uses :param segments (/api/users/:id) and handler names a global Lua function called with the request table (req.params exposes the matched segments). Declared at the top level of the script.",
		Example: `k.http_routes = {
  {method="GET",  path="/api/users/:id", handler="get_user"},
  {method="POST", path="/api/users",     handler="create_user"},
}
function get_user(req)
  local id = req.params.id
  return k.http_response_json({id=id, name="John"})
end`},
	"http_middleware": {Name: "http_middleware", Group: "server", Signature: "k.http_middleware = {...}",
		Docs: "Middleware chain for serve-mode HTTP: a list of global function names (or function values, registered via k.http_auth_middleware). Each runs with the request table before the route handler; returning nil continues the chain, returning a response (e.g. k.http_response_json) short-circuits it. Middleware may attach data to req (e.g. req.user) for the handler. Runs before k.http_routes handlers only.",
		Example: `k.http_middleware = {"auth_middleware"}
function auth_middleware(req)
  local token = req.headers["authorization"]
  if not token then return k.http_response_json({error="unauthorized"}, 401) end
  req.user = token  -- visible to the route handler
  return nil        -- continue
end`},
	"http_response_json": {Name: "http_response_json", Group: "server", Signature: "k.http_response_json(data[, status])",
		Docs: "Builds a JSON HTTP response: status defaults to 200, headers carry content-type application/json. Return it from a route handler or middleware.",
		Params: []Param{P("data", "any", "Value encoded as JSON body."), P("status", "number", "HTTP status (default 200).")},
		Example: `return k.http_response_json({ok=true}, 200)`},
	"http_response_text": {Name: "http_response_text", Group: "server", Signature: "k.http_response_text(text[, status])",
		Docs: "Builds a plain-text HTTP response (content-type text/plain).",
		Params: []Param{P("text", "string", "Response body."), P("status", "number", "HTTP status (default 200).")},
		Example: `return k.http_response_text("hello", 200)`},
	"http_response_redirect": {Name: "http_response_redirect", Group: "server", Signature: "k.http_response_redirect(url[, status])",
		Docs: "Builds an HTTP redirect response (Location header; status defaults to 302).",
		Params: []Param{P("url", "string", "Redirect target."), P("status", "number", "HTTP status (default 302).")},
		Example: `return k.http_response_redirect("/login")`},
	"http_request_parse_multipart": {Name: "http_request_parse_multipart", Group: "server", Signature: "k.http_request_parse_multipart(req)",
		Docs: "Parses a multipart/form-data request table and returns {fields={...}, files={name={filename, content_type, data}}}. The request table must carry headers (with content-type boundary) and body.",
		Params: []Param{P("req", "table", "Serve-mode request table (headers + body).")},
		Example: `local mp = k.http_request_parse_multipart(req)`},
	"http_auth_middleware": {Name: "http_auth_middleware", Group: "server", Signature: "k.http_auth_middleware(fn)",
		Docs: "Appends a middleware function (or global function name) to the k.http_middleware chain at runtime. Equivalent to setting the k.http_middleware table entry.",
		Params: []Param{P("fn", "function|string", "Middleware function or the name of a global function.")},
		Example: `k.http_auth_middleware(auth_middleware)`},
	"http_stats": {Name: "http_stats", Group: "server", Signature: "k.http_stats()",
		Docs: "Returns serve-mode HTTP metrics: {requests_total, errors_total, avg_latency_ms, active_connections}.",
		Example: `local s = k.http_stats()`},

	// Phase 8: structured logging (serve mode)
	"log": {Name: "log", Group: "server", Signature: "k.log", Docs: "Structured logging: k.log.info/warn/error/debug/trace(msg[, data]) and k.log.stats(). Records land in klog.db when serve runs with --log-sqlite."},
	"log.info":    {Name: "log.info", Group: "server", Signature: "k.log.info(msg[, data])", Docs: "Records an info-level structured log entry (category app). data is a table stored as JSON.", Params: []Param{P("msg", "string", "Log message."), P("data", "table", "Optional structured fields stored as JSON.")}},
	"log.warn":    {Name: "log.warn", Group: "server", Signature: "k.log.warn(msg[, data])", Docs: "Records a warn-level structured log entry.", Params: []Param{P("msg", "string", "Log message."), P("data", "table", "Optional structured fields stored as JSON.")}},
	"log.error":   {Name: "log.error", Group: "server", Signature: "k.log.error(msg[, data])", Docs: "Records an error-level structured log entry.", Params: []Param{P("msg", "string", "Log message."), P("data", "table", "Optional structured fields stored as JSON.")}},
	"log.debug":   {Name: "log.debug", Group: "server", Signature: "k.log.debug(msg[, data])", Docs: "Records a debug-level structured log entry (dropped unless --log-level debug).", Params: []Param{P("msg", "string", "Log message."), P("data", "table", "Optional structured fields stored as JSON.")}},
	"log.trace":   {Name: "log.trace", Group: "server", Signature: "k.log.trace(msg[, data])", Docs: "Records a trace-level structured log entry (dropped unless --log-level trace).", Params: []Param{P("msg", "string", "Log message."), P("data", "table", "Optional structured fields stored as JSON.")}},
	"log.stats":   {Name: "log.stats", Group: "server", Signature: "k.log.stats()", Docs: "Returns structured-log sink stats: {rows, dropped, oldest, newest, path}.", Example: `local st = k.log.stats()`},

	// tier-2 flow
	"timer_start": {Name: "timer_start", Group: "flow", Signature: "k.timer_start(id, ms[, repeats])",
		Docs: "Starts a session timer; fires a Lua function named id every ms (repeats times when given, else forever).",
		Params: []Param{
			P("id", "string", "Timer id; a Lua function with this global name is invoked on each tick."),
			P("ms", "number", "Interval in milliseconds."),
			P("repeats", "number", "Optional tick count; omitted runs forever."),
		},
		Example: `k.timer_start("refresh", 1000)  -- calls refresh() every second
function refresh() k.form.refresh("main") end`},
	"timer_stop": {Name: "timer_stop", Group: "flow", Signature: "k.timer_stop(id)",
		Docs:    "Stops a running session timer.",
		Params:  []Param{P("id", "string", "Timer id passed to k.timer_start.")},
		Example: `k.timer_stop("refresh")`},
	"status_show":  {Name: "status_show", Group: "flow", Signature: "k.status_show(text)", Docs: "Shows a busy/status bar with the given text."},
	"status_close": {Name: "status_close", Group: "flow", Signature: "k.status_close()", Docs: "Hides the status bar."},
	"param_set":    {Name: "param_set", Group: "flow", Signature: "k.param_set(key, value)", Docs: "Persists an app param (string) to an app-side file."},
	"param_get":    {Name: "param_get", Group: "flow", Signature: "k.param_get(key)", Docs: "Reads a persisted app param (string; \"\" if unset)."},
	"net_ok":       {Name: "net_ok", Group: "flow", Signature: "k.net_ok(timeout_ms)", Docs: "Reports internet reachability via a TCP dial."},
	"locale":       {Name: "locale", Group: "flow", Signature: "k.locale()", Docs: "Returns the session locale (\"en-US\" default)."},
	"ping":         {Name: "ping", Group: "flow", Signature: "k.ping(host, timeout_ms)", Docs: "TCP-based latency probe returning ms, or nil when unreachable."},

	// tier-2 database
	"connect_sqlite": {Name: "connect_sqlite", Group: "database", Signature: "k.connect_sqlite(path)",
		Docs:    "Opens a SQLite database file; returns a handle usable with k.sql/k.db_*. The path resolves through the filesystem sandbox (workdir + --allow-fs).",
		Params:  []Param{P("path", "string", "SQLite database file path, relative to the sandbox root or an --allow-fs root.")},
		Example: `local h = k.connect_sqlite("app.db")`},
	"disconnect_sqlite": {Name: "disconnect_sqlite", Group: "database", Signature: "k.disconnect_sqlite([handle])",
		Docs:    "Closes a SQLite connection (or all).",
		Params:  []Param{P("handle", "string", "Optional handle from k.connect_sqlite; omitted closes all.")},
		Example: `k.disconnect_sqlite(h)`},
	"db_kill_table": {Name: "db_kill_table", Group: "database", Signature: "k.db_kill_table(handle, table, where)",
		Docs:    "Deletes rows matching the where table.",
		Params:  []Param{P("handle", "string", "Connection handle."), P("table", "string", "Table name."), P("where", "table", "Equality filters {column = value}.")},
		Example: `k.db_kill_table(h, "items", { expired = 1 })`},
	"db_proc": {Name: "db_proc", Group: "database", Signature: "k.db_proc(handle, name, ...params)",
		Docs:    "Executes a stored procedure.",
		Params:  []Param{P("handle", "string", "Connection handle."), P("name", "string", "Stored procedure name."), P("...params", "any", "Procedure arguments.")},
		Example: `local res = k.db_proc(h, "sp_reprice", 1.1)`},

	// tier-2 comm
	"socket_open":      {Name: "socket_open", Group: "comm", Signature: "k.socket_open(host, port[, timeout_ms])", Docs: "Opens a TCP client connection; returns a handle."},
	"socket_write":     {Name: "socket_write", Group: "comm", Signature: "k.socket_write(handle, data)", Docs: "Writes data to an open socket; returns bytes written."},
	"socket_read":      {Name: "socket_read", Group: "comm", Signature: "k.socket_read(handle[, count])", Docs: "Reads count bytes (or all until close) from a socket."},
	"socket_read_line": {Name: "socket_read_line", Group: "comm", Signature: "k.socket_read_line(handle)", Docs: "Reads one line (trailing newline trimmed); nil at EOF."},
	"socket_close":     {Name: "socket_close", Group: "comm", Signature: "k.socket_close(handle)", Docs: "Closes an open socket."},

	// tier-2 FTP
	"ftp_connect":     {Name: "ftp_connect", Group: "comm", Signature: "k.ftp_connect(host[, port, user, pw])", Docs: "Connects to an FTP server; returns a handle."},
	"ftp_set_cwd":     {Name: "ftp_set_cwd", Group: "comm", Signature: "k.ftp_set_cwd(handle, path)", Docs: "Changes the remote working directory (CWD)."},
	"ftp_get_file":    {Name: "ftp_get_file", Group: "comm", Signature: "k.ftp_get_file(handle, remote, local)", Docs: "Downloads remote to a local path (RETR)."},
	"ftp_put_file":    {Name: "ftp_put_file", Group: "comm", Signature: "k.ftp_put_file(handle, local, remote)", Docs: "Uploads a local file to remote (STOR)."},
	"ftp_file_exists": {Name: "ftp_file_exists", Group: "comm", Signature: "k.ftp_file_exists(handle, path)", Docs: "Reports whether a remote file exists (SIZE)."},
	"ftp_create_dir":  {Name: "ftp_create_dir", Group: "comm", Signature: "k.ftp_create_dir(handle, path)", Docs: "Creates a remote directory (MKD)."},
	"ftp_delete":      {Name: "ftp_delete", Group: "comm", Signature: "k.ftp_delete(handle, path)", Docs: "Deletes a remote file (DELE)."},
	"ftp_rename":      {Name: "ftp_rename", Group: "comm", Signature: "k.ftp_rename(handle, from, to)", Docs: "Renames a remote file or folder (RNFR/RNTO)."},
	"ftp_list":        {Name: "ftp_list", Group: "comm", Signature: "k.ftp_list(handle[, path])", Docs: "Lists remote entry names (LIST)."},
	"ftp_disconnect":  {Name: "ftp_disconnect", Group: "comm", Signature: "k.ftp_disconnect(handle)", Docs: "Sends QUIT and closes the FTP connection."},

	// tier-2 email (SMTP)
	"smtp_connect":    {Name: "smtp_connect", Group: "email", Signature: "k.smtp_connect{host,port,user,pw,tls}", Docs: "Connects to an SMTP server; returns a handle."},
	"smtp_send":       {Name: "smtp_send", Group: "email", Signature: "k.smtp_send(handle, {from,to,subject,body,attachments})", Docs: "Sends an email through the connected SMTP server."},
	"smtp_disconnect": {Name: "smtp_disconnect", Group: "email", Signature: "k.smtp_disconnect([handle])", Docs: "Closes an SMTP connection (or all)."},

	// tier-2 email (POP3)
	"pop3_connect": {Name: "pop3_connect", Group: "email", Signature: "k.pop3_connect{host,port,user,pw,tls}", Docs: "Connects to a POP3 server; returns a handle."},
	"pop3_stat":    {Name: "pop3_stat", Group: "email", Signature: "k.pop3_stat(handle)", Docs: "Returns {count,size} of the mailbox."},
	"pop3_list":    {Name: "pop3_list", Group: "email", Signature: "k.pop3_list(handle)", Docs: "Returns a table of {id,size} message summaries."},
	"pop3_retr":    {Name: "pop3_retr", Group: "email", Signature: "k.pop3_retr(handle, index)", Docs: "Retrieves a message by index."},
	"pop3_dele":    {Name: "pop3_dele", Group: "email", Signature: "k.pop3_dele(handle, index)", Docs: "Marks a message for deletion."},
	"pop3_noop":    {Name: "pop3_noop", Group: "email", Signature: "k.pop3_noop(handle)", Docs: "Keeps the POP3 connection alive."},
	"pop3_quit":    {Name: "pop3_quit", Group: "email", Signature: "k.pop3_quit(handle)", Docs: "Sends QUIT and closes the POP3 connection."},

	// tier-2 web service (SOAP)
	"webservice_run": {Name: "webservice_run", Group: "comm", Signature: "k.webservice_run(profile, params)", Docs: "Calls a SOAP web service. profile: {url,action[,method,timeout_ms]}; params is the body table. Returns {status,headers,body}."},

	// Phase 2 MQTT client (serve mode; serve-communication-plan.md §2)
	"mqtt_connect": {Name: "mqtt_connect", Group: "comm", Signature: "k.mqtt_connect{broker, port, user?, password?, client_id?, tls?}",
		Docs: "Connects to an MQTT broker and returns a client handle for the other k.mqtt_* calls. broker is a host (with port) or a full mqtt:// / ssl:// URL; tls=true switches to ssl. Gated by the server's --allow-net flag; raises ERROR_PERMISSION when network egress is disabled.",
		Params: []Param{
			P("broker", "string", "Broker host (e.g. broker.local) or full URL (mqtt://host:port)."),
			P("port", "number", "Broker port (default 1883 when broker is a bare host)."),
			P("user", "string", "Optional username."),
			P("password", "string", "Optional password."),
			P("client_id", "string", "Optional MQTT client id (random when omitted)."),
			P("tls", "boolean", "Use TLS (ssl://) when broker is a bare host."),
		},
		Example: `local h = k.mqtt_connect{broker="broker.local", port=1883, user="app", password="secret"}`},
	"mqtt_subscribe": {Name: "mqtt_subscribe", Group: "comm", Signature: "k.mqtt_subscribe(handle, topic[, qos])",
		Docs: "Subscribes the client to a topic filter (MQTT wildcards + / #). Inbound messages matching a k.mqtt_on(pattern) handler are delivered to it.",
		Params: []Param{P("handle", "string", "Client handle from k.mqtt_connect."), P("topic", "string", "Topic filter (e.g. sensors/+/temp)."), P("qos", "number", "QoS 0 (default), 1 or 2.")},
		Example: `k.mqtt_subscribe(h, "sensors/#")`},
	"mqtt_publish": {Name: "mqtt_publish", Group: "comm", Signature: "k.mqtt_publish(handle, topic, payload[, qos[, retain]])",
		Docs: "Publishes payload to a topic.",
		Params: []Param{P("handle", "string", "Client handle."), P("topic", "string", "Topic to publish to."), P("payload", "string", "Message payload."), P("qos", "number", "QoS 0 (default), 1 or 2."), P("retain", "boolean", "Retain the message (default false).")},
		Example: `k.mqtt_publish(h, "sensors/1/temp", "21.5", 1)`},
	"mqtt_unsubscribe": {Name: "mqtt_unsubscribe", Group: "comm", Signature: "k.mqtt_unsubscribe(handle, topic)",
		Docs: "Unsubscribes from a topic filter.",
		Params: []Param{P("handle", "string", "Client handle."), P("topic", "string", "Topic filter to drop.")},
		Example: `k.mqtt_unsubscribe(h, "sensors/#")`},
	"mqtt_close": {Name: "mqtt_close", Group: "comm", Signature: "k.mqtt_close(handle)",
		Docs: "Disconnects the MQTT client and releases its handle.",
		Params: []Param{P("handle", "string", "Client handle.")},
		Example: `k.mqtt_close(h)`},
	"mqtt_on": {Name: "mqtt_on", Group: "comm", Signature: "k.mqtt_on(pattern, fn_name)",
		Docs: "Registers an inbound message handler: a global function named fn_name is called with (topic, payload) for every message whose topic matches pattern (MQTT + / # wildcards). Multiple handlers may match; all run in registration order. Serve mode only.",
		Params: []Param{P("pattern", "string", "Topic filter (e.g. sensors/+/temp or sensors/#)."), P("fn_name", "string", "Global function invoked as fn(topic, payload).")},
		Example: `k.mqtt_on("sensors/#", "on_sensor")
function on_sensor(topic, payload) k.log.info(topic, {data=payload}) end`},
	"mqtt_stats": {Name: "mqtt_stats", Group: "comm", Signature: "k.mqtt_stats(handle)",
		Docs: "Returns client connection state: {connected, subscriptions, messages_in, messages_out}.",
		Params: []Param{P("handle", "string", "Client handle.")},
		Example: `local st = k.mqtt_stats(h)`},

	// tier-2 crypto
	"crypt_symmetric":  {Name: "crypt_symmetric", Group: "crypto", Signature: "k.crypt_symmetric(alg, key, data[, iv])", Docs: "AES-CBC symmetric encrypt/decrypt (alg aes-encrypt/aes-decrypt). Returns base64."},
	"crypt_asymmetric": {Name: "crypt_asymmetric", Group: "crypto", Signature: "k.crypt_asymmetric(alg, key, data)", Docs: "RSA PKCS#1 v1.5 encrypt/decrypt with a PEM key."},
	"sign":             {Name: "sign", Group: "crypto", Signature: "k.sign(data, key[, alg])", Docs: "RSA signature (default SHA-256); returns base64."},
	"verify":           {Name: "verify", Group: "crypto", Signature: "k.verify(data, signature, key[, alg])", Docs: "Verifies an RSA signature; returns true/false."},

	// tier-2 files (zip)
	"zip_list": {Name: "zip_list", Group: "files", Signature: "k.zip_list(zipPath)",
		Docs:    "Lists the member names of a zip archive.",
		Params:  []Param{P("zipPath", "string", "Path of the zip archive.")},
		Example: `for _, name in ipairs(k.zip_list("bundle.zip")) do k.print(name) end`},
	"zip_add": {Name: "zip_add", Group: "files", Signature: "k.zip_add(zipPath, entries)",
		Docs:    "Writes a zip archive from {name=content} entries.",
		Params:  []Param{P("zipPath", "string", "Destination zip path."), P("entries", "table", "{memberName = content, ...} map.")},
		Example: `k.zip_add("bundle.zip", { "readme.txt" = "hello" })`},
	"zip_extract": {Name: "zip_extract", Group: "files", Signature: "k.zip_extract(zipPath, dir)",
		Docs:    "Extracts a zip archive into dir; returns file count.",
		Params:  []Param{P("zipPath", "string", "Path of the zip archive."), P("dir", "string", "Destination directory.")},
		Example: `local n = k.zip_extract("bundle.zip", "/tmp/out")`},

	// tier-2 data formats
	"csv_parse":   {Name: "csv_parse", Group: "formats", Signature: "k.csv_parse(text[, opts])", Docs: "Parses CSV (opts {header, sep, quote})."},
	"csv_string":  {Name: "csv_string", Group: "formats", Signature: "k.csv_string(data[, opts])", Docs: "Serializes CSV from a table."},
	"csv_load":    {Name: "csv_load", Group: "formats", Signature: "k.csv_load(path[, opts])", Docs: "Reads and parses a CSV file."},
	"csv_save":    {Name: "csv_save", Group: "formats", Signature: "k.csv_save(path, data[, opts])", Docs: "Writes a CSV file atomically."},
	"ini_parse":   {Name: "ini_parse", Group: "formats", Signature: "k.ini_parse(text)", Docs: "Parses INI into {section={key=value}, _root={...}}."},
	"ini_string":  {Name: "ini_string", Group: "formats", Signature: "k.ini_string(data)", Docs: "Serializes INI from a table."},
	"ini_load":    {Name: "ini_load", Group: "formats", Signature: "k.ini_load(path)", Docs: "Reads and parses an INI file."},
	"ini_save":    {Name: "ini_save", Group: "formats", Signature: "k.ini_save(path, data)", Docs: "Writes an INI file atomically."},
	"ini_read":    {Name: "ini_read", Group: "formats", Signature: "k.ini_read(path, section, key)", Docs: "Reads a single INI key (Kalipso parity)."},
	"ini_write":   {Name: "ini_write", Group: "formats", Signature: "k.ini_write(path, section, key, value)", Docs: "Writes a single INI key (Kalipso parity)."},
	"yaml_parse":  {Name: "yaml_parse", Group: "formats", Signature: "k.yaml_parse(text)", Docs: "Parses YAML; multi-document input yields a list of tables."},
	"yaml_string": {Name: "yaml_string", Group: "formats", Signature: "k.yaml_string(data)", Docs: "Serializes a value as YAML."},
	"yaml_load":   {Name: "yaml_load", Group: "formats", Signature: "k.yaml_load(path)", Docs: "Reads and parses a YAML file."},
	"yaml_save":   {Name: "yaml_save", Group: "formats", Signature: "k.yaml_save(path, data)", Docs: "Writes a YAML file atomically."},
	"xml_load":    {Name: "xml_load", Group: "formats", Signature: "k.xml_load(path)", Docs: "Reads an XML file into the element-table shape {_name,_attrs,_children,_text}."},
	"xml_save":    {Name: "xml_save", Group: "formats", Signature: "k.xml_save(path, table)", Docs: "Writes an element-table as XML."},

	// tier-2 rows conversions
	"json_to_rows": {Name: "json_to_rows", Group: "rows", Signature: "k.json_to_rows(value)", Docs: "Converts a JSON array of row-maps into {columns, rows}."},
	"rows_to_json": {Name: "rows_to_json", Group: "rows", Signature: "k.rows_to_json(result)", Docs: "Extracts the rows array from a result set."},
	"csv_to_rows":  {Name: "csv_to_rows", Group: "rows", Signature: "k.csv_to_rows(csvTable)", Docs: "Converts a parsed CSV table into {columns, rows}."},
	"rows_to_csv":  {Name: "rows_to_csv", Group: "rows", Signature: "k.rows_to_csv(result[, opts])", Docs: "Serializes a result set as CSV."},
	"xml_to_rows":  {Name: "xml_to_rows", Group: "rows", Signature: "k.xml_to_rows(document)", Docs: "Converts an XML element-table into {columns, rows}."},
	"rows_to_xml":  {Name: "rows_to_xml", Group: "rows", Signature: "k.rows_to_xml(result[, rootName[, rowName]])", Docs: "Serializes a result set as XML."},
}

// KSets documents the K.* helpers and constants. This is static tooling data;
// the corresponding globals are installed by registerHelpers and Setup.
var KSets = []Info{
	{Name: "K.EQ", Signature: "K.EQ", Docs: "Operator constant \"=\".", Group: "helpers"},
	{Name: "K.NEQ", Signature: "K.NEQ", Docs: "Operator constant \"<>\".", Group: "helpers"},
	{Name: "K.ADD", Signature: "K.ADD", Docs: "Operator constant \"+\".", Group: "helpers"},
	{Name: "K.eq", Signature: "K.eq(a, b)", Docs: "Kalipso equality (numeric when both coerce, else string compare).", Group: "helpers"},
	{Name: "K.ne", Signature: "K.ne(a, b)", Docs: "Negation of K.eq.", Group: "helpers"},
	{Name: "K.add", Signature: "K.add(a, b)", Docs: "Kalipso addition: numeric if both coerce, else concatenation.", Group: "helpers"},
	{Name: "K.tonum", Signature: "K.tonum(x)", Docs: "Coerces to a number, else 0.", Group: "helpers"},
	{Name: "K.tostr", Signature: "K.tostr(x)", Docs: "Coerces to the Kalipso string form.", Group: "helpers"},
	{Name: "K.truthy", Signature: "K.truthy(x)", Docs: "Kalipso condition test (0/\"0\"/\"\"/nil are false).", Group: "helpers"},
	{Name: "K.NULL", Signature: "K.NULL", Docs: "Sentinel representing a JSON null.", Group: "helpers"},
	{Name: "K.is_null", Signature: "K.is_null(value)", Docs: "Reports whether value is K.NULL.", Group: "helpers"},
}

// ExprFuncs documents the §5.9 expression functions installed as flat globals
// (not under k.*) so expressions read like Kalipso. This is static tooling
// data; the globals are installed by registerExprFuncs in Setup.
var ExprFuncs = []Info{
	// string
	{Name: "left", Group: "string", Signature: "left(s, n)", Docs: "Returns the first n characters of s.", Example: `left("hello", 2)  -- "he"`},
	{Name: "right", Group: "string", Signature: "right(s, n)", Docs: "Returns the last n characters of s.", Example: `right("hello", 2)  -- "lo"`},
	{Name: "middle", Group: "string", Signature: "middle(s, start, count)", Docs: "Returns count characters of s starting at 1-based start.", Example: `middle("hello", 2, 3)  -- "ell"`},
	{Name: "length", Group: "string", Signature: "length(s)", Docs: "Returns the byte length of s (Lua # semantics).", Example: `length("hello")  -- 5`},
	{Name: "replace", Group: "string", Signature: "replace(s, old, new)", Docs: "Replaces all occurrences of old in s with new.", Example: `replace("a-b-c", "-", ".")  -- "a.b.c"`},
	{Name: "trim", Group: "string", Signature: "trim(s)", Docs: "Removes leading/trailing whitespace from s.", Example: `trim("  hi  ")  -- "hi"`},
	{Name: "upper", Group: "string", Signature: "upper(s)", Docs: "Converts s to uppercase.", Example: `upper("hi")  -- "HI"`},
	{Name: "lower", Group: "string", Signature: "lower(s)", Docs: "Converts s to lowercase.", Example: `lower("HI")  -- "hi"`},
	{Name: "find", Group: "string", Signature: "find(s, needle[, start])", Docs: "Returns the 1-based position of needle in s (0 if absent).", Example: `find("hello", "ll")  -- 3`},
	{Name: "string_count", Group: "string", Signature: "string_count(s, needle)", Docs: "Counts non-overlapping occurrences of needle in s.", Example: `string_count("bobobo", "bo")  -- 2`},
	{Name: "complete", Group: "string", Signature: "complete(s, length[, pad])", Docs: "Pads s on the right to length with pad (default space).", Example: `complete("ab", 4, ".")  -- "ab.."`},
	{Name: "ascii", Group: "string", Signature: "ascii(ch)", Docs: "Returns the numeric code of the first byte of ch.", Example: `ascii("A")  -- 65`},
	{Name: "charact", Group: "string", Signature: "charact(code)", Docs: "Returns the byte corresponding to code.", Example: `charact(65)  -- "A"`},
	{Name: "base64_encode", Group: "string", Signature: "base64_encode(s)", Docs: "Encodes s as base64.", Example: `base64_encode("hi")  -- "aGk="`},
	{Name: "base64_decode", Group: "string", Signature: "base64_decode(s)", Docs: "Decodes base64 into a string.", Example: `base64_decode("aGk=")  -- "hi"`},
	{Name: "urlencode", Group: "string", Signature: "urlencode(s)", Docs: "URL-encodes s (query style, spaces become +).", Example: `urlencode("a b")  -- "a+b"`},
	{Name: "urldecode", Group: "string", Signature: "urldecode(s)", Docs: "Decodes a URL-encoded string.", Example: `urldecode("a+b")  -- "a b"`},
	{Name: "encode", Group: "string", Signature: "encode(s)", Docs: "Alias of urlencode.", Example: `encode("a b")  -- "a+b"`},
	{Name: "decode", Group: "string", Signature: "decode(s)", Docs: "Alias of urldecode.", Example: `decode("a+b")  -- "a b"`},
	{Name: "full_encode", Group: "string", Signature: "full_encode(s)", Docs: "Percent-encodes every non-unreserved byte.", Example: `full_encode("a b")  -- "a%20b"`},
	{Name: "jsonencode", Group: "string", Signature: "jsonencode(value)", Docs: "Encodes a value as compact JSON (same as k.json_string).", Example: `jsonencode({ a = 1 })  -- '{"a":1}'`},
	{Name: "jsondecode", Group: "string", Signature: "jsondecode(text)", Docs: "Parses JSON text (same as k.json_parse).", Example: `jsondecode('{"a":1}').a  -- 1`},
	{Name: "xmlencode", Group: "string", Signature: "xmlencode(s)", Docs: "Escapes XML special characters.", Example: `xmlencode("<a>")  -- "&lt;a&gt;"`},
	{Name: "xmldecode", Group: "string", Signature: "xmldecode(s)", Docs: "Unescapes XML entities.", Example: `xmldecode("&lt;a&gt;")  -- "<a>"`},
	{Name: "guid", Group: "string", Signature: "guid()", Docs: "Generates a random RFC 4122 version-4 UUID.", Example: `guid()  -- "4f3c..."`},
	{Name: "extract_string", Group: "string", Signature: "extract_string(s, start, end)", Docs: "Returns s from 1-based start to end inclusive.", Example: `extract_string("hello", 2, 4)  -- "ell"`},
	{Name: "set_string", Group: "string", Signature: "set_string(s, start, count, new)", Docs: "Replaces count characters of s at start with new.", Example: `set_string("hello", 2, 3, "i")  -- "hi"`},
	{Name: "file_extract_part", Group: "string", Signature: "file_extract_part(path, part)", Docs: "Extracts path/name/ext from a file path.", Example: `file_extract_part("/a/b.txt", "name")  -- "b"`},
	{Name: "mltext", Group: "string", Signature: "mltext(...)", Docs: "Joins arguments with newlines (multi-line text).", Example: `mltext("a", "b")  -- "a\nb"`},

	// numeric
	{Name: "abs", Group: "numeric", Signature: "abs(x)", Docs: "Absolute value of x.", Example: `abs(-3)  -- 3`},
	{Name: "round", Group: "numeric", Signature: "round(x[, decimals])", Docs: "Rounds x to decimals (default 0) with half-away-from-zero.", Example: `round(2.567, 2)  -- 2.57`},
	{Name: "floor", Group: "numeric", Signature: "floor(x)", Docs: "Largest integer <= x.", Example: `floor(2.7)  -- 2`},
	{Name: "ceiling", Group: "numeric", Signature: "ceiling(x)", Docs: "Smallest integer >= x.", Example: `ceiling(2.1)  -- 3`},
	{Name: "power", Group: "numeric", Signature: "power(x, y)", Docs: "x raised to the power y.", Example: `power(2, 3)  -- 8`},
	{Name: "nth_root", Group: "numeric", Signature: "nth_root(x, n)", Docs: "The n-th root of x.", Example: `nth_root(27, 3)  -- 3`},
	{Name: "sqrt", Group: "numeric", Signature: "sqrt(x)", Docs: "Square root of x.", Example: `sqrt(16)  -- 4`},
	{Name: "exp", Group: "numeric", Signature: "exp(x)", Docs: "e raised to x.", Example: `exp(1)  -- 2.718...`},
	{Name: "log", Group: "numeric", Signature: "log(x)", Docs: "Natural logarithm of x.", Example: `log(1)  -- 0`},
	{Name: "log10", Group: "numeric", Signature: "log10(x)", Docs: "Base-10 logarithm of x.", Example: `log10(100)  -- 2`},
	{Name: "sin", Group: "numeric", Signature: "sin(x)", Docs: "Sine of x (radians).", Example: `sin(0)  -- 0`},
	{Name: "cos", Group: "numeric", Signature: "cos(x)", Docs: "Cosine of x (radians).", Example: `cos(0)  -- 1`},
	{Name: "tan", Group: "numeric", Signature: "tan(x)", Docs: "Tangent of x (radians).", Example: `tan(0)  -- 0`},
	{Name: "asin", Group: "numeric", Signature: "asin(x)", Docs: "Arcsine in radians.", Example: `asin(1)  -- 1.570...`},
	{Name: "acos", Group: "numeric", Signature: "acos(x)", Docs: "Arccosine in radians.", Example: `acos(1)  -- 0`},
	{Name: "atan", Group: "numeric", Signature: "atan(x)", Docs: "Arctangent in radians.", Example: `atan(1)  -- 0.785...`},
	{Name: "deg2rad", Group: "numeric", Signature: "deg2rad(x)", Docs: "Converts degrees to radians.", Example: `deg2rad(180)  -- 3.141...`},
	{Name: "rad2deg", Group: "numeric", Signature: "rad2deg(x)", Docs: "Converts radians to degrees.", Example: `rad2deg(math.pi)  -- 180`},
	{Name: "bitwise_and", Group: "numeric", Signature: "bitwise_and(a, b)", Docs: "Bitwise AND of the integer parts of a and b.", Example: `bitwise_and(6, 3)  -- 2`},
	{Name: "bitwise_or", Group: "numeric", Signature: "bitwise_or(a, b)", Docs: "Bitwise OR of the integer parts of a and b.", Example: `bitwise_or(4, 1)  -- 5`},
	{Name: "bitwise_xor", Group: "numeric", Signature: "bitwise_xor(a, b)", Docs: "Bitwise XOR of the integer parts of a and b.", Example: `bitwise_xor(6, 3)  -- 5`},
	{Name: "random", Group: "numeric", Signature: "random() / random(max) / random(min, max)", Docs: "Random float in [0,1), integer in [1,max], or in [min,max).", Example: `random(1, 6)  -- die roll`},
	{Name: "int_part", Group: "numeric", Signature: "int_part(x)", Docs: "Integer part of x (truncated).", Example: `int_part(3.7)  -- 3`},
	{Name: "dec_part", Group: "numeric", Signature: "dec_part(x)", Docs: "Fractional part of x.", Example: `dec_part(3.7)  -- 0.7`},
	{Name: "mask_number", Group: "numeric", Signature: "mask_number(x, mask)", Docs: "Formats x using a mask (#, 0, comma grouping, decimal).", Example: `mask_number(1234.5, "#,##0.00")  -- "1,234.50"`},
	{Name: "val", Group: "numeric", Signature: "val(x)", Docs: "Numeric value of x; 0 when not numeric.", Example: `val("42")  -- 42`},
	{Name: "sum", Group: "numeric", Signature: "sum(table)", Docs: "Sum of the numeric values in a table.", Example: `sum({1, 2, 3})  -- 6`},
	{Name: "extractstringd", Group: "numeric", Signature: "extractstringd(s)", Docs: "Digits of s as a number.", Example: `extractstringd("ab12cd")  -- 12`},

	// conditional
	{Name: "lookup", Group: "conditional", Signature: "lookup(key, k1, v1, ...)", Docs: "Returns the value whose key equals key; \"\" when absent.", Example: `lookup("b", "a", 1, "b", 2)  -- 2`},
	{Name: "yesno", Group: "conditional", Signature: "yesno(cond, a, b)", Docs: "Returns a when cond is Kalipso-truthy, else b.", Example: `yesno(1, "y", "n")  -- "y"`},
	{Name: "iif", Group: "conditional", Signature: "iif(cond, a, b)", Docs: "Inline if; returns a when cond is truthy, else b.", Example: `iif(5 > 2, "yes", "no")  -- "yes"`},

	// datetime
	{Name: "sys_date", Group: "datetime", Signature: "sys_date()", Docs: "Today's date as \"YYYY-MM-DD\".", Example: `sys_date()  -- "2026-09-07"`},
	{Name: "sys_time", Group: "datetime", Signature: "sys_time()", Docs: "Current time as \"HH:MM:SS\".", Example: `sys_time()  -- "14:30:00"`},
	{Name: "day", Group: "datetime", Signature: "day(date)", Docs: "Day of month of a date string.", Example: `day("2026-09-07")  -- 7`},
	{Name: "month", Group: "datetime", Signature: "month(date)", Docs: "Month (1-12) of a date string.", Example: `month("2026-09-07")  -- 9`},
	{Name: "year", Group: "datetime", Signature: "year(date)", Docs: "Year of a date string.", Example: `year("2026-09-07")  -- 2026`},
	{Name: "hour", Group: "datetime", Signature: "hour(time)", Docs: "Hour of a date/time string.", Example: `hour("12:34:56")  -- 12`},
	{Name: "minute", Group: "datetime", Signature: "minute(time)", Docs: "Minute of a date/time string.", Example: `minute("12:34:56")  -- 34`},
	{Name: "second", Group: "datetime", Signature: "second(time)", Docs: "Second of a date/time string.", Example: `second("12:34:56")  -- 56`},
	{Name: "add_days", Group: "datetime", Signature: "add_days(date, n)", Docs: "Date n days later as \"YYYY-MM-DD\".", Example: `add_days("2026-09-07", 3)  -- "2026-09-10"`},
	{Name: "subtract_days", Group: "datetime", Signature: "subtract_days(date, n)", Docs: "Date n days earlier as \"YYYY-MM-DD\".", Example: `subtract_days("2026-09-07", 3)  -- "2026-09-04"`},
	{Name: "date_diff", Group: "datetime", Signature: "date_diff(d2, d1)", Docs: "Whole days between two date strings (d2 minus d1).", Example: `date_diff("2026-09-10", "2026-09-07")  -- 3`},
	{Name: "datetime_add", Group: "datetime", Signature: "datetime_add(dt, days[, hours[, minutes[, seconds]]])", Docs: "Adds a duration to a datetime string.", Example: `datetime_add("2026-09-07 10:00", 1, 2)  -- "2026-09-08 12:00"`},
	{Name: "datetime_sub", Group: "datetime", Signature: "datetime_sub(dt, days[, hours[, minutes[, seconds]]])", Docs: "Subtracts a duration from a datetime string.", Example: `datetime_sub("2026-09-07 10:00", 1, 2)  -- "2026-09-06 08:00"`},
	{Name: "datetime_diff", Group: "datetime", Signature: "datetime_diff(dt2, dt1)", Docs: "Seconds between two datetime strings.", Example: `datetime_diff("2026-09-07 11:00", "2026-09-07 10:30")  -- 1800`},
	{Name: "date_to_string", Group: "datetime", Signature: "date_to_string(date[, format])", Docs: "Formats a date using %Y %m %d etc.; default \"YYYY-MM-DD\".", Example: `date_to_string("2026-09-07", "%d/%m/%Y")  -- "07/09/2026"`},
	{Name: "time_to_string", Group: "datetime", Signature: "time_to_string(time[, format])", Docs: "Formats a time using %H %M %S etc.; default \"HH:MM:SS\".", Example: `time_to_string("14:05:00", "%H:%M")  -- "14:05"`},
	{Name: "week_day", Group: "datetime", Signature: "week_day(date)", Docs: "Day of week as 1-7 (Sunday=1).", Example: `week_day("2026-09-07")  -- 1`},
	{Name: "week_number", Group: "datetime", Signature: "week_number(date)", Docs: "ISO week number of a date.", Example: `week_number("2026-09-07")  -- 37`},
	{Name: "tick_count", Group: "datetime", Signature: "tick_count()", Docs: "Unix milliseconds since the epoch.", Example: `tick_count()`},
	{Name: "julian", Group: "datetime", Signature: "julian(date)", Docs: "Julian day number of a date.", Example: `julian("2026-09-07")`},
	{Name: "utc_to_local", Group: "datetime", Signature: "utc_to_local(dt)", Docs: "Converts a UTC datetime string to local wall-clock.", Example: `utc_to_local("2026-09-07 12:00:00")`},
	{Name: "local_to_utc", Group: "datetime", Signature: "local_to_utc(dt)", Docs: "Converts a local datetime string to UTC.", Example: `local_to_utc("2026-09-07 14:00:00")`},

	// conversion
	{Name: "tostr", Group: "conversion", Signature: "tostr(x)", Docs: "Kalipso string form of x.", Example: `tostr(42)  -- "42"`},
	{Name: "tonum", Group: "conversion", Signature: "tonum(x)", Docs: "Kalipso number of x; 0 when not numeric.", Example: `tonum("3.5")  -- 3.5`},
	{Name: "todate", Group: "conversion", Signature: "todate(s)", Docs: "Normalizes s to \"YYYY-MM-DD\".", Example: `todate("07/09/2026")  -- "2026-09-07"`},
	{Name: "strtodate", Group: "conversion", Signature: "strtodate(s[, format])", Docs: "Parses s (optionally with %-tokens) and emits \"YYYY-MM-DD\".", Example: `strtodate("07/09/2026", "%d/%m/%Y")  -- "2026-09-07"`},
	{Name: "boolstr", Group: "conversion", Signature: "boolstr(v)", Docs: "{\"true\",\"false\"} for the Kalipso truthiness of v.", Example: `boolstr(1)  -- "true"`},
}

// ExprInfo returns a copy of the expression-function documentation.
func ExprInfo() []Info {
	out := make([]Info, len(ExprFuncs))
	copy(out, ExprFuncs)
	return out
}

// Globals lists the script-visible globals beyond the k/K namespaces.
var Globals = []string{"ARGS", "CTRL", "ERRORCODE", "ERRORMSG", "main"}

// namespaceNames are registry entries that are pure namespaces with no
// implementation of their own; the sync test exempts them from Info.
var namespaceNames = map[string]bool{"form": true, "ctrl": true, "table": true, "looper": true, "chart": true, "grid": true, "tab": true, "shared": true, "ws": true, "tcp": true, "xml": true, "ai": true, "ai_chat": true}

// Docs returns a copy of the k.* documentation map (name → Info).
func Docs() map[string]Info {
	out := make(map[string]Info, len(apiDocs))
	for k, v := range apiDocs {
		out[k] = v
	}
	return out
}

// KInfo returns a copy of the K.* helper documentation slice.
func KInfo() []Info {
	out := make([]Info, len(KSets))
	copy(out, KSets)
	return out
}

// GlobalsList returns the script-visible globals copy.
func GlobalsList() []string {
	out := make([]string, len(Globals))
	copy(out, Globals)
	return out
}

// Namespace reports whether name is a pure namespace entry (form, ctrl, table).
func Namespace(name string) bool { return namespaceNames[name] }
