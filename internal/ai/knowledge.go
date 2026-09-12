package ai

import (
	"fmt"
	"strings"

	"kalua/internal/bindings"
)

// runModeBindings is the set of k.* names (from api_docs) that are relevant
// to run-mode form apps. Used to keep the knowledge pack compact for local
// models; serve-mode bindings are intentionally excluded.
var runModeBindings = map[string]bool{
	// flow
	"print": true, "sleep": true, "yield": true, "quit": true, "error": true,
	"msgbox": true, "popup": true, "clipboard_set": true, "clipboard_get": true,
	"pick_file": true, "bell": true, "screen_size": true, "on_error": true,
	// forms
	"form.new": true, "form.show": true, "form.close": true, "form.return_to": true,
	"form.clear": true, "form.refresh": true, "form.on": true,
	// controls
	"ctrl.label": true, "ctrl.textbox": true, "ctrl.button": true,
	"ctrl.combo": true, "ctrl.list": true, "ctrl.table": true,
	"ctrl.checkbox": true, "ctrl.radio": true, "ctrl.image": true,
	"ctrl.set_value": true, "ctrl.get_value": true, "ctrl.set_property": true,
	"ctrl.get_property": true, "ctrl.set_focus": true, "ctrl.select_text": true,
	"ctrl.get_selection": true, "ctrl.set_selection": true, "ctrl.get_item_count": true,
	"ctrl.execute_event": true, "ctrl.chart": true,
	// table & looper
	"table.find": true, "table.add_line": true, "table.delete_line": true,
	"table.set_column_value": true, "table.get_column_value": true,
	"table.set_selected_column": true, "table.get_selected_column": true,
	// shared (basic)
	"shared.set": true, "shared.get": true, "shared.del": true, "shared.keys": true,
	// comm (basic)
	"http_request": true,
	// data formats (basic)
	"json_load": true, "json_save": true, "json_parse": true, "json_string": true,
	// DB (basic)
	"connect_db": true, "disconnect_db": true, "db_select": true,
	"db_insert": true, "db_update": true, "db_delete": true,
	"sql": true, "tx_begin": true, "tx_commit": true, "tx_rollback": true,
	// file (basic)
	"file_load": true, "file_save": true, "file_open": true, "file_read": true,
	"file_write": true, "file_close": true, "file_copy": true, "file_move": true,
	"file_delete": true, "file_exists": true, "file_mkdir": true, "file_list": true,
	"file_info": true,
	// expression funcs relevant to forms
	"tostr": true, "tonum": true, "boolstr": true,
	"upper": true, "lower": true, "trim": true, "left": true, "right": true,
	"middle": true, "find": true, "length": true, "replace": true,
	"abs": true, "round": true, "floor": true, "ceiling": true,
	"lookup": true, "yesno": true, "iif": true,
	// date/time
	"sys_date": true, "sys_time": true, "day": true, "month": true, "year": true,
	"hour": true, "minute": true, "second": true,
	"add_days": true, "subtract_days": true, "date_diff": true,
	"datetime_add": true, "datetime_sub": true, "datetime_diff": true,
	"week_day": true, "week_number": true,
	// server
	"status_show": true, "status_close": true,
	// param
	"param_get": true, "param_set": true,
	// net
	"net_ok": true, "ping": true,
	// timer
	"timer_start": true, "timer_stop": true,
}

// BuildSystemPrompt constructs the system prompt for the AI builder.
// It is built from api_doc.go so it stays in sync with the runtime.
// If includeFull is false, only run-mode-relevant bindings are included
// (smaller context for local models).
func BuildSystemPrompt(includeFull bool) string {
	var sb strings.Builder
	sb.WriteString("You are a KALUA Lua code generator. You generate single-file `.lua` apps that run on the KALUA runtime (embedded gopher-lua + net/http server).\n\n")
	sb.WriteString("## Hard Rules\n")
	sb.WriteString("- Every app MUST define `function main()` as the entry point.\n")
	sb.WriteString("- Use `k.form.new(name, opts)` then `k.form.show(name)` to display a form. `k.form.show` suspends until the form closes.\n")
	sb.WriteString("- Use `k.ctrl.*` to add controls to a form. Controls are placed after `k.form.new` and before `k.form.show`.\n")
	sb.WriteString("- Function names use snake_case (e.g. `k.ctrl.textbox`, `k.form.new`).\n")
	sb.WriteString("- Expression functions (string/numeric/etc.) are FLAT GLOBALS, NOT under `k.*`: use `upper(s)`, `round(x)`, `sys_date()`, NOT `k.upper(s)`, `k.round(x)`.\n")
	sb.WriteString("- Do NOT use `io`, `os.execute`, `require`, or any library not exposed via `k.*`.\n")
	sb.WriteString("- `k.print` writes to the app log. Use it for debugging output.\n")
	sb.WriteString("- `K.tonum(v)`, `K.tostr(v)`, `K.eq(a,b)`, `K.truthy(v)` for Kalipso-coercion semantics.\n")
	sb.WriteString("- In serve mode UI bindings (k.form.*, k.ctrl.*, k.msgbox, k.status_*) raise runtime errors. This builder generates run-mode apps only.\n\n")

	if includeFull {
		sb.WriteString("## Available k.* API\n")
		for _, info := range bindings.Docs() {
			sb.WriteString(fmt.Sprintf("- `%s` — %s\n", info.Name, info.Docs))
		}
		sb.WriteString("\n## K.* Helpers\n")
		for _, info := range bindings.KInfo() {
			sb.WriteString(fmt.Sprintf("- `%s` — %s\n", info.Name, info.Docs))
		}
		sb.WriteString("\n## Expression Functions\n")
		for _, info := range bindings.ExprInfo() {
			sb.WriteString(fmt.Sprintf("- `%s` — %s\n", info.Name, info.Docs))
		}
	} else {
		sb.WriteString("## Available k.* API (run-mode form subset)\n")
		for name, info := range bindings.Docs() {
			if !runModeBindings[name] {
				continue
			}
			sb.WriteString(fmt.Sprintf("- `%s` — %s\n", name, info.Docs))
		}
		sb.WriteString("\n## K.* Helpers (subset)\n")
		for _, info := range bindings.KInfo() {
			sb.WriteString(fmt.Sprintf("- `%s` — %s\n", info.Name, info.Docs))
		}
		sb.WriteString("\n## Expression Functions (subset)\n")
		for _, info := range bindings.ExprInfo() {
			if !runModeBindings[info.Name] {
				continue
			}
			sb.WriteString(fmt.Sprintf("- `%s` — %s\n", info.Name, info.Docs))
		}
	}

	sb.WriteString("\n## Examples\n")
	sb.WriteString("```lua\n")
	sb.WriteString("function main()\n")
	sb.WriteString("  k.form.new(\"main\", {title = \"Hello\"})\n")
	sb.WriteString("  k.ctrl.label(\"main\", \"lbl1\", {text = \"Enter your name:\"})\n")
	sb.WriteString("  k.ctrl.textbox(\"main\", \"name\", {label = \"Name\"})\n")
	sb.WriteString("  k.ctrl.button(\"main\", \"ok\", {label = \"OK\", onclick = function()\n")
	sb.WriteString("    k.msgbox(\"Hello, \" .. k.ctrl.get_value(\"main\", \"name\"))\n")
	sb.WriteString("    k.quit()\n")
	sb.WriteString("  end})\n")
	sb.WriteString("  k.form.show(\"main\")\n")
	sb.WriteString("end\n")
	sb.WriteString("```\n")

	return sb.String()
}

// BuildUserPrompt constructs the user prompt from a natural language request
// and optional existing script content (for editing).
func BuildUserPrompt(request string, existingScript string) string {
	var sb strings.Builder
	sb.WriteString("Generate a KALUA Lua app: ")
	sb.WriteString(request)
	sb.WriteString("\n\n")
	if existingScript != "" {
		sb.WriteString("Modify the following existing script according to the request above:\n\n")
		sb.WriteString("```lua\n")
		sb.WriteString(existingScript)
		sb.WriteString("\n```\n\n")
		sb.WriteString("IMPORTANT: Keep EVERY control, handler, function body and option from the existing script. Only add, remove, or change what the request explicitly demands, and preserve all control names and the form name exactly. Preserve the form's layout, align, gap, cells and per-control cell assignments unless the request explicitly asks to change the layout. Changing or dropping anything the request does not mention will be treated as an error.\n\n")
	}
	sb.WriteString("Return the complete code inside a single ```lua ... ``` code block.")
	return sb.String()
}
