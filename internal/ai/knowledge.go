package ai

import (
	"fmt"
	"strings"

	"kalua/internal/bindings"
)

// ModeRun and ModeServe name the two app shapes KALUA can host. The mode
// selects the entry-point contract and the compact API subset the AI prompt
// teaches. An empty mode resolves to ModeRun.
const (
	ModeRun   = "run"
	ModeServe = "serve"
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

// serveModeBindings is the set of k.* names (from api_docs) that are relevant
// to serve-mode apps (headless API workers). Used as the compact subset when
// generating serve apps; run-mode form bindings are intentionally excluded.
var serveModeBindings = map[string]bool{
	// flow
	"print": true, "sleep": true, "yield": true, "error": true, "on_error": true,
	// shared state (across workers)
	"shared.set": true, "shared.get": true, "shared.del": true,
	"shared.keys": true, "shared.incr": true,
	// websocket server
	"ws.broadcast": true, "ws.send": true, "ws.close": true,
	// tcp server
	"tcp.accept": true, "tcp.send": true, "tcp.close": true,
	// http client
	"http_request": true,
	// database
	"connect_db": true, "disconnect_db": true, "db_select": true,
	"db_insert": true, "db_update": true, "db_delete": true,
	"sql": true, "tx_begin": true, "tx_commit": true, "tx_rollback": true,
	"connect_sqlite": true, "disconnect_sqlite": true,
	"db_kill_table": true, "db_proc": true,
	// result-set conversions
	"json_to_rows": true, "rows_to_json": true,
	"csv_to_rows": true, "rows_to_csv": true,
	"xml_to_rows": true, "rows_to_xml": true,
	// data formats
	"json_load": true, "json_save": true, "json_parse": true, "json_string": true,
	"csv_parse": true, "csv_string": true, "csv_load": true, "csv_save": true,
	"ini_parse": true, "ini_string": true, "ini_load": true, "ini_save": true,
	"ini_read": true, "ini_write": true,
	"yaml_parse": true, "yaml_string": true, "yaml_load": true, "yaml_save": true,
	"xml_load": true, "xml_save": true,
	// files & zip
	"file_load": true, "file_save": true, "file_open": true, "file_read": true,
	"file_write": true, "file_close": true, "file_copy": true, "file_move": true,
	"file_delete": true, "file_exists": true, "file_mkdir": true, "file_list": true,
	"file_info": true, "file_read_line": true, "checksum": true,
	"zip_add": true, "zip_extract": true, "zip_list": true,
	// crypto
	"crypt_symmetric": true, "crypt_asymmetric": true,
	"sign": true, "verify": true, "encrypt": true, "decrypt": true,
	// comm
	"socket_open": true, "socket_write": true, "socket_read": true,
	"socket_read_line": true, "socket_close": true,
	"ftp_connect": true, "ftp_set_cwd": true, "ftp_get_file": true,
	"ftp_put_file": true, "ftp_file_exists": true, "ftp_create_dir": true,
	"ftp_delete": true, "ftp_rename": true, "ftp_list": true, "ftp_disconnect": true,
	"smtp_connect": true, "smtp_send": true, "smtp_disconnect": true,
	"pop3_connect": true, "pop3_stat": true, "pop3_list": true,
	"pop3_retr": true, "pop3_dele": true, "pop3_noop": true, "pop3_quit": true,
	"webservice_run": true,
	// param & net
	"param_get": true, "param_set": true, "net_ok": true, "ping": true,
	// debug
	"debug.stack": true, "debug.locals": true, "debug.trace": true,
	// expression funcs
	"tostr": true, "tonum": true, "boolstr": true,
	"upper": true, "lower": true, "trim": true, "left": true, "right": true,
	"middle": true, "find": true, "length": true, "replace": true,
	"abs": true, "round": true, "floor": true, "ceiling": true,
	"lookup": true, "yesno": true, "iif": true,
	"sys_date": true, "sys_time": true, "day": true, "month": true, "year": true,
	"hour": true, "minute": true, "second": true,
	"add_days": true, "subtract_days": true, "date_diff": true,
	"datetime_add": true, "datetime_sub": true, "datetime_diff": true,
	"week_day": true, "week_number": true,
}

// BuildSystemPrompt constructs the run-mode system prompt for the AI builder.
// It is built from api_doc.go so it stays in sync with the runtime.
// If includeFull is false, only run-mode-relevant bindings are included
// (smaller context for local models).
func BuildSystemPrompt(includeFull bool) string {
	return BuildSystemPromptFor(ModeRun, includeFull)
}

// BuildSystemPromptFor constructs the system prompt for a given app mode
// ("run" or "serve"). It is built from api_doc.go so it stays in sync with
// the runtime. If includeFull is false, only mode-relevant bindings are
// included (smaller context for local models).
func BuildSystemPromptFor(mode string, includeFull bool) string {
	serve := mode == ModeServe
	subset := runModeBindings
	entryRule := "Every app MUST define `function main()` as the entry point."
	if serve {
		subset = serveModeBindings
		entryRule = "Every app MUST define one or more serve entry points: `function handle_http(req)`, `function handle_ws(msg)`, `function handle_tcp(msg)`, optionally plus `function init(config)` and `function shutdown()`."
	}

	var sb strings.Builder
	sb.WriteString("You are a KALUA Lua code generator. You generate single-file `.lua` apps that run on the KALUA runtime (embedded gopher-lua + net/http server).\n\n")
	sb.WriteString("## Hard Rules\n")
	sb.WriteString("- " + entryRule + "\n")
	if !serve {
		sb.WriteString("- Use `k.form.new(name, opts)` then `k.form.show(name, [options])` to display a form. `k.form.show` suspends until the form closes.\n")
		sb.WriteString("- `k.form.show` options: `{modal=true|false (default false), gap=number|{x=num,y=num} (default 5% desktop, 3% mobile)}`. When modal=true, form is centered overlay.\n")
		sb.WriteString("- Use `k.ctrl.*` to add controls to a form. Controls are placed after `k.form.new` and before `k.form.show`.\n")
	} else {
		sb.WriteString("- UI bindings (k.form.*, k.ctrl.*, k.msgbox, k.popup, k.status_*) raise runtime errors in serve mode — do NOT use them.\n")
		sb.WriteString("- `handle_http(req)` returns one of: nil (200 empty), a plain string (200 text/plain), `{json = ...}` (200 application/json), `{status = n}` (n empty), or `{status = n, headers = {...}, body = ...}`. `req` has `.path`, `.method`, `.headers`, `.query`, `.body`.\n")
		sb.WriteString("- `handle_ws(msg)` / `handle_tcp(msg)` receive `{type = \"open\"|\"text\"|\"binary\"|\"close\", data=, client_id=}`; a string return value is echoed back to the connection. Use `k.ws.broadcast/send/close` and `k.tcp.send/close`.\n")
		sb.WriteString("- Multiple workers run the script concurrently; use `k.shared.*` for cross-worker state (it is the only shared memory). `main()` is never called in serve mode and its absence is fine.\n")
	}
	sb.WriteString("- Function names use snake_case (e.g. `k.db_select`, `k.form.new`).\n")
	sb.WriteString("- Expression functions (string/numeric/etc.) are FLAT GLOBALS, NOT under `k.*`: use `upper(s)`, `round(x)`, `sys_date()`, NOT `k.upper(s)`, `k.round(x)`.\n")
	sb.WriteString("- Do NOT use `io`, `os.execute`, `require`, or any library not exposed via `k.*`.\n")
	sb.WriteString("- **Prefer `k.*` / `K.*` / expression functions over generic Lua whenever a KALUA API exists for the task** — e.g. use `k.file_extract_part()` not `io.open`, `k.db_connect()` not a raw C binding, `left(s,n)` not `string.sub(s,1,n)`, `round(x)` not manual `x+0.5`. Fall back to plain Lua only when no KALUA equivalent exists.\n")
	sb.WriteString("- `k.print` writes to the app log. Use it for debugging output.\n")
	sb.WriteString("- `K.tonum(v)`, `K.tostr(v)`, `K.eq(a,b)`, `K.truthy(v)` for Kalipso-coercion semantics.\n")

	if includeFull {
		sb.WriteString("\n## Available k.* API\n")
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
		if serve {
			sb.WriteString("\n## Available k.* API (serve-mode subset)\n")
		} else {
			sb.WriteString("\n## Available k.* API (run-mode form subset)\n")
		}
		for name, info := range bindings.Docs() {
			if !subset[name] {
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
			if !subset[info.Name] {
				continue
			}
			sb.WriteString(fmt.Sprintf("- `%s` — %s\n", info.Name, info.Docs))
		}
	}

	if serve {
		sb.WriteString("\n## Examples\n")
		sb.WriteString("```lua\n")
		sb.WriteString("function handle_http(req)\n")
		sb.WriteString("  if req.path == \"/api/hello\" then\n")
		sb.WriteString("    k.shared.set(\"visits\", tonum(k.shared.get(\"visits\") or \"0\") + 1)\n")
		sb.WriteString("    return {status = 200, headers = {[\"content-type\"] = \"application/json\"},\n")
		sb.WriteString("            body = k.json_string({message = \"hello\", visits = k.shared.get(\"visits\")})}\n")
		sb.WriteString("  end\n")
		sb.WriteString("  return {status = 404, body = \"not found\"}\n")
		sb.WriteString("end\n")
		sb.WriteString("```\n")
	} else {
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
	}

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
