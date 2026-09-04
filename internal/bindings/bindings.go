// Package bindings exposes the KALUA standard library to scripts. Nothing the
// host implements lives outside this package: any k.* call your script makes
// must exist in the registry below or the checker flags it.
//
// A binding is a plain gopher-lua LGFunction. Whenever it needs to block
// (k.sleep) or take control flow decisions (k.quit), it talks to the *vm.App
// in Env so the app scheduler keeps pumping the single coroutine.
package bindings

import (
	"os"
	"path/filepath"

	"github.com/yuin/gopher-lua"

	"kalua/internal/coerce"
	"kalua/internal/common"
	"kalua/internal/vm"
)

// Options carries host configuration a binding may depend on. It is captured
// by Setup and read-only afterwards.
type Options struct {
	// Args seeds the ARGS global table.
	Args []string

	// AllowFS lists extra filesystem roots (absolute paths) scripts may touch
	// besides the working directory. Paths are resolved at Setup time.
	AllowFS []string

	// MaxFileSize bounds how many bytes k.file_load / k.json_load may read.
	// Zero means the default of 16 MiB.
	MaxFileSize int64

	// Verbose enables enhanced tracing of k.* API calls (Tier 1 debugging).
	Verbose bool
}

// DefaultMaxFileSize bounds k.file_load/k.json_load reads unless Options
// overrides it.
const DefaultMaxFileSize int64 = 16 << 20

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

	// workdir is the sandbox's home directory; relative file paths resolve
	// against it. allowFS holds the absolute roots scripts may write to.
	workdir     string
	allowFS     []string
	maxFileSize int64

	// kNULL is the K.NULL sentinel table representing JSON null.
	kNULL *lua.LTable

	// Sess is the session this env belongs to (for msgbox, clipboard, etc.)
	Sess common.SessionInterface

	// Logger for error logging
	Logger Logger

	// verbose eust whether k.* API tracing is enabled (Options.Verbose).
	verbose bool
}

// Logger interface for logging.
type Logger interface {
	Printf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
	Tracef(format string, args ...interface{})
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
	"clipboard_set":             "flow",
	"clipboard_get":             "flow",
	"pick_file":                 "flow",
	"bell":                      "flow",
	"screen_size":               "flow",
	"http_request":              "flow",
	"debug":                     "debug", // namespace
	"debug.stack":               "debug",
	"debug.locals":              "debug",
	"debug.trace":               "debug",
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
	"file_open":                 "files",
	"file_read":                 "files",
	"file_read_line":            "files",
	"file_write":                "files",
	"file_close":                "files",
	"file_load":                 "files",
	"file_save":                 "files",
	"file_copy":                 "files",
	"file_move":                 "files",
	"file_delete":               "files",
	"file_exists":               "files",
	"file_mkdir":                "files",
	"file_list":                 "files",
	"file_info":                 "files",
	"json_parse":                "json",
	"json_string":               "json",
	"json_load":                 "json",
	"json_save":                 "json",
	"json_get":                  "json",
	"json_array_item":           "json",
	"json_count":                "json",
	"json_names":                "json",
	"is_null":                   "json",
	"checksum":                  "crypto",
	"encrypt":                   "crypto",
	"decrypt":                   "crypto",
	// xml bindings
	"xml_parse":      "xml",
	"xml_root":       "xml",
	"xml_child":      "xml",
	"xml_child_list": "xml",
	"xml_attr":       "xml",
	"xml_content":    "xml",
	"xml_attrs":      "xml",
	"xml_name":       "xml",
	// serve mode bindings
	"shared":       "server", // namespace
	"shared.set":   "server",
	"shared.get":   "server",
	"shared.del":   "server",
	"shared.keys":  "server",
	"shared.incr":  "server",
	"ws":           "server", // namespace
	"ws.broadcast": "server",
	"ws.send":      "server",
	"ws.close":     "server",
	"tcp":          "server", // namespace
	"tcp.send":     "server",
	"tcp.close":    "server",
	// tier-2 flow
	"timer_start":   "flow",
	"timer_stop":    "flow",
	"status_show":   "flow",
	"status_close":  "flow",
	"param_set":     "flow",
	"param_get":     "flow",
	"net_ok":        "flow",
	"locale":        "flow",
	"ping":          "flow",
	// tier-2 database
	"connect_sqlite":    "database",
	"disconnect_sqlite": "database",
	"db_kill_table":     "database",
	"db_proc":           "database",
	// tier-2 comm
	"socket_open":      "comm",
	"socket_write":     "comm",
	"socket_read":      "comm",
	"socket_read_line": "comm",
	"socket_close":     "comm",
	// tier-2 FTP
	"ftp_connect":     "comm",
	"ftp_set_cwd":     "comm",
	"ftp_get_file":    "comm",
	"ftp_put_file":    "comm",
	"ftp_file_exists": "comm",
	"ftp_create_dir":  "comm",
	"ftp_delete":      "comm",
	"ftp_rename":      "comm",
	"ftp_list":        "comm",
	"ftp_disconnect":  "comm",
	// tier-2 email (SMTP)
	"smtp_connect":    "email",
	"smtp_send":       "email",
	"smtp_disconnect": "email",
	// tier-2 email (POP3)
	"pop3_connect": "email",
	"pop3_stat":    "email",
	"pop3_list":    "email",
	"pop3_retr":    "email",
	"pop3_dele":    "email",
	"pop3_noop":    "email",
	"pop3_quit":    "email",
	// tier-2 web service (SOAP)
	"webservice_run": "comm",
	// tier-2 crypto
	"crypt_symmetric":   "crypto",
	"crypt_asymmetric":  "crypto",
	"sign":              "crypto",
	"verify":            "crypto",
	// tier-2 files
	"zip_list":    "files",
	"zip_add":     "files",
	"zip_extract": "files",
	// tier-2 data formats
	"csv_parse":  "formats",
	"csv_string": "formats",
	"csv_load":   "formats",
	"csv_save":   "formats",
	"ini_parse":  "formats",
	"ini_string": "formats",
	"ini_load":   "formats",
	"ini_save":   "formats",
	"ini_read":   "formats",
	"ini_write":  "formats",
	"yaml_parse": "formats",
	"yaml_string": "formats",
	"yaml_load":  "formats",
	"yaml_save":  "formats",
	"xml_load":   "formats",
	"xml_save":   "formats",
	// tier-2 rows conversions
	"json_to_rows": "rows",
	"rows_to_json": "rows",
	"csv_to_rows":  "rows",
	"rows_to_csv":  "rows",
	"xml_to_rows":  "rows",
	"rows_to_xml":  "rows",
}

// register puts a k.* binding into the env's k namespace.
// Supports dotted names like "form.new" to create nested tables.
func (e *Env) register(name, group string, fn lua.LGFunction) {
	e.known[name] = group
	registerKnown[name] = group

	// When verbose tracing is enabled, wrap every k.* binding so function
	// entry/exit is logged. Keeps the trace cheap (one interface assert) when
	// tracing is off.
	var callFn lua.LGFunction = fn
	if e.verbose && e.Logger != nil {
		callFn = func(L *lua.LState) int {
			n := L.GetTop()
			args := make([]string, 0, n)
			for i := 1; i <= n; i++ {
				args = append(args, L.Get(i).String())
			}
			e.Logger.Tracef("k.%s(%s)", name, joinArgs(args))
			ret := fn(L)
			if ret > 0 {
				top := L.GetTop()
				outs := make([]string, 0, ret)
				for i := top - ret + 1; i <= top; i++ {
					outs = append(outs, L.Get(i).String())
				}
				e.Logger.Tracef("k.%s => %s", name, joinArgs(outs))
			}
			return ret
		}
	}

	// Handle dotted names (e.g., "form.new" -> k.form.new)
	parts := split(name, ".")
	tbl := e.k
	for i, part := range parts {
		if i == len(parts)-1 {
			// Last part: set the function
			tbl.RawSetString(part, e.L.NewFunction(callFn))
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
// state, then installs every implemented binding. opts.Args seeds the ARGS
// global table (in order, starting at 1). It must be called once per LState.
// sess is the session this env belongs to (for msgbox, clipboard, etc.); can be nil.
// logger is used for error logging; can be nil.
func Setup(L *lua.LState, app *vm.App, opts Options, sess common.SessionInterface, logger Logger) *Env {
	e := &Env{L: L, App: app, known: map[string]string{}, maxFileSize: opts.MaxFileSize, Sess: sess, Logger: logger, verbose: opts.Verbose}
	if e.maxFileSize <= 0 {
		e.maxFileSize = DefaultMaxFileSize
	}
	e.workdir = workdirOf(opts)
	e.allowFS = allowFSOf(opts)

	k := L.NewTable()
	L.SetGlobal("k", k)
	e.k = k

	// K.NULL sentinel: the only value k.json_parse/k.json_load produce for a
	// JSON null, and k.is_null's identity check.
	e.kNULL = L.NewTable()

	K := L.NewTable()
	registerHelpers(e, K)
	K.RawSetString("NULL", e.kNULL)
	K.RawSetString("is_null", L.NewFunction(e.isNull))
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
	registerDebug(e)
	registerForms(e)
	registerControls(e)
	registerDB(e)
	registerFiles(e)
	registerJSON(e)
	registerCrypto(e)
	registerXML(e)
	registerExprFuncs(e)
	registerFormats(e)
	registerRows(e)
	registerComm(e)
	registerSMTP(e)
	registerPop3(e)
	registerFTP(e)
	registerSoap(e)
	registerSMTP(e)
	registerPop3(e)
	registerFTP(e)
	registerSoap(e)

	argsT := L.NewTable()
	for i, a := range opts.Args {
		argsT.RawSetInt(i+1, lua.LString(a))
	}
	L.SetGlobal("ARGS", argsT)

	return e
}

func filepathAbs(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

// workdirOf resolves the sandbox home directory from the process working dir.
func workdirOf(opts Options) string {
	wd, _ := os.Getwd()
	return wd
}

// allowFSOf resolves the AllowFS roots to absolute, symlink-resolved paths.
func allowFSOf(opts Options) []string {
	var out []string
	for _, root := range opts.AllowFS {
		abs, err := filepathAbs(root)
		if err != nil {
			continue
		}
		if resolved, err := evalSymlinksBestEffort(abs); err == nil {
			out = append(out, resolved)
		} else {
			out = append(out, abs)
		}
	}
	return out
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

// isNull reports whether the argument is the K.NULL sentinel: the value a JSON
// null parses to. Returns false for plain nil.
func (e *Env) isNull(L *lua.LState) int {
	L.Push(lua.LBool(L.Get(1) == e.kNULL))
	return 1
}
