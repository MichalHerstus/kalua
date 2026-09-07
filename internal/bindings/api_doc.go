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
	"form.show": {Name: "form.show", Group: "forms", Signature: "k.form.show(name)",
		Docs:    "Shows a form (modal) and suspends the script until it closes.",
		Params:  []Param{P("name", "string", "Form name declared with k.form.new.")},
		Example: `k.form.show("main")`},
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

	// controls
	"ctrl": {Name: "ctrl", Group: "controls", Signature: "k.ctrl", Docs: "Control constructors: k.ctrl.label/textbox/button/..."},
	"ctrl.label": {Name: "ctrl.label", Group: "controls", Signature: "k.ctrl.label(form, name, optsTable)",
		Docs: "Adds a label control. opts: {text, multiline?:boolean, cell?, align?}. multiline renders a pre-wrap div preserving \\n (kforms_enhancements.md §4.2). cell/align: grid layout assignment + alignment (kforms_enhancements.md §6).",
		Params: []Param{
			P("form", "string", "Parent form name."),
			P("name", "string", "Unique control name within the form."),
			P("optsTable.text", "string", "Label text."),
			P("optsTable.multiline", "boolean", "Preserve newlines with white-space: pre-wrap."),
			P("optsTable.cell", "string", "Grid cell id to place the control in."),
			P("optsTable.align", "string", "Alignment override: \"left\", \"center\" or \"right\"."),
		},
		Example: `k.ctrl.label("main", "lbl", { text = "Hello\nWorld", multiline = true })`},
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
		Docs:    "Adds a table control; rows manipulated via k.table.*. With opts {db, query, ...} the table is DB-linked (Tabulator mode).",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Unique control name."), P("optsTable.columns", "list", "Column definitions (Tabulator mode)."), P("optsTable.db", "string", "DB handle for a DB-linked table."), P("optsTable.query", "string", "SQL query for a DB-linked table.")},
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
		Docs:    "Sets an arbitrary control property.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Control name."), P("prop", "string", "Property name, e.g. \"label\", \"enabled\", \"cell\", \"align\"."), P("value", "any", "New property value.")},
		Example: `k.ctrl.set_property("main", "btn", "enabled", false)`},
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
		Docs:    "Adds a looper control (repeating row layout). DB-linked when opts carry {db,query,links,page_size?,count_query?,where?,order_by?}.",
		Params:  []Param{P("form", "string", "Parent form name."), P("name", "string", "Unique control name."), P("optsTable.db", "string", "DB handle for a DB-linked looper."), P("optsTable.query", "string", "SQL query for a DB-linked looper."), P("optsTable.links", "list", "Maps result columns to template controls."), P("optsTable.page_size", "number", "Rows per page for pagination.")},
		Example: `k.ctrl.looper("main", "rows", { db = h, query = "SELECT * FROM items", links = { {field = "name", control = "tpl_name", property = "text"} } })`},
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
		Docs:    "Opens a database connection (DSN scheme: sqlite://, mysql://, postgres://, sqlserver://) and returns a handle. Supported drivers: SQLite (built-in), MySQL (github.com/go-sql-driver/mysql), PostgreSQL (github.com/jackc/pgx/v5/stdlib), SQL Server (github.com/microsoft/go-mssqldb).",
		Params:  []Param{P("dsn", "string", "Connection string, e.g. \"sqlite://data.db\" or \"postgres://user:pw@host/db\".")},
		Example: `local h = k.connect_db("sqlite:///tmp/app.db")`},
	"disconnect_db": {Name: "disconnect_db", Group: "database", Signature: "k.disconnect_db([handle])",
		Docs:    "Closes a connection, or all connections when no handle is given.",
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
		Docs:    "Opens a SQLite database file; returns a handle usable with k.sql/k.db_*.",
		Params:  []Param{P("path", "string", "SQLite database file path.")},
		Example: `local h = k.connect_sqlite("/tmp/app.db")`},
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
var Globals = []string{"ARGS", "CTRL", "main"}

// namespaceNames are registry entries that are pure namespaces with no
// implementation of their own; the sync test exempts them from Info.
var namespaceNames = map[string]bool{"form": true, "ctrl": true, "table": true, "looper": true, "chart": true, "shared": true, "ws": true, "tcp": true, "xml": true}

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
