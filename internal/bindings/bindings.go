// Package bindings exposes the KALUA standard library to scripts. Nothing the
// host implements lives outside this package: any k.* call your script makes
// must exist in the registry below or the checker flags it.
//
// A binding is a plain gopher-lua LGFunction. Whenever it needs to block
// (k.sleep) or take control flow decisions (k.quit), it talks to the *vm.App
// in Env so the app scheduler keeps pumping the single coroutine.
package bindings

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/yuin/gopher-lua"

	"kalua/internal/coerce"
	"kalua/internal/common"
	"kalua/internal/klog"
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

	// DenyFS lists absolute paths (or directories) of host-owned artefacts
	// scripts may never read or write, even when they fall inside AllowFS or
	// the working directory (see kalua_security_plan.md L16/P11). Every entry
	// is matched against the fully-resolved absolute path, so it also covers
	// k.zip_extract member targets and k.connect_db("sqlite://…") DSNs, both
	// of which resolve through the sandbox.
	DenyFS []string

	// MaxFileSize bounds how many bytes k.file_load / k.json_load may read.
	// Zero means the default of 16 MiB.
	MaxFileSize int64

	// MaxWSMessageSize caps WebSocket messages from the browser (run mode).
	// Zero means the default of 1 MiB (H5/P6: prevents large-message OOM).
	MaxWSMessageSize int64

	// Verbose enables enhanced tracing of k.* API calls (Tier 1 debugging).
	Verbose bool

	// ScriptName is the base name of the script being run (e.g. "myapp.lua").
	// Used to auto-fill the footer control's `script_name` (§11.3) when the
	// script omits it. Empty in headless/unknown contexts.
	ScriptName string

	// AIConfig configures the LLM for k.form.ai_agent. Nil means the chat form
	// will fail with a clear error.
	AIConfig *common.AIConfig

	// Quit stops the whole serve-mode server (wired from the serve command's
	// runner). Nil keeps the pre-setup behavior where k.quit only flags the
	// single worker.
	Quit func()

	// Stats receives serve-mode HTTP router/metrics state (Phase 1:
	// k.http_stats). Nil disables k.http_stats (run mode).
	Stats *HTTPStats

	// LogDB is the structured-log sink for serve mode (Phase 8: k.log.*).
	// Nil disables k.log.*.
	LogDB *klog.DB

	// MQTT is the serve-mode MQTT client hub (Phase 2: k.mqtt_*). Nil
	// disables the MQTT bindings.
	MQTT MQTTHub

	// AllowNet gates outbound MQTT client connections (serve-communication
	// plan Phase 2 security). k.mqtt_connect refuses when false.
	AllowNet bool

	// SOAPStats accumulates serve-mode SOAP metrics readable from k.soap_stats().
	// The server increments it per request; bindings only read it.
	SOAPStats *SOAPStats

	// AllowOrigin lists additional Origin patterns to accept for WebSocket
	// connections in run mode (e.g., "http://192.168.1.*"). Patterns support
	// glob-style * in host and port. When binding to a specific IP (not
	// 0.0.0.0/::), that host:port is automatically allowed. When binding to
	// 0.0.0.0/::, only localhost is allowed by default; use this to add LAN origins.
	AllowOrigin []string

	// AuthToken is an optional shared secret for WebSocket authentication in
	// run mode. When non-empty, the client must provide the token via the
	// "token" query parameter or "X-Kalua-Token" header. Use AuthTokenFile
	// to read the token from a file (supports rotation).
	AuthToken string
}

// DefaultMaxFileSize bounds k.file_load/k.json_load reads unless Options
// overrides it.
const DefaultMaxFileSize int64 = 16 << 20

// HTTPStats accumulates serve-mode HTTP metrics readable from k.http_stats().
// The server increments it per request; bindings only read it.
type HTTPStats struct {
	RequestsTotal  atomic.Int64
	ErrorsTotal    atomic.Int64
	LatencySumMs   atomic.Int64
	LatencyCount   atomic.Int64
	ActiveConns    atomic.Int64
}

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
	// denyFS holds the absolute paths of host-owned artefacts scripts may
	// never touch (evaluated before allowFS in resolvePath).
	workdir     string
	allowFS     []string
	denyFS      []string
	maxFileSize int64

	// kNULL is the K.NULL sentinel table representing JSON null.
	kNULL *lua.LTable

	// Sess is the session this env belongs to (for msgbox, clipboard, etc.)
	Sess common.SessionInterface

	// Logger for error logging
	Logger Logger

	// verbose eust whether k.* API tracing is enabled (Options.Verbose).
	verbose bool

	// scriptName is the running script's base name (Options.ScriptName); the
	// footer control auto-fills its script_name from it (§11.3).
	scriptName string

	// aiConfig configures k.form.ai_agent.
	aiConfig *common.AIConfig

	// onErr is the script-registered k.on_error(fn) hook (or nil). When a
	// binding fails it is invoked with (ERRORCODE, ERRORMSG) so the script can
	// show the error and continue. See e.fail / fireError.
	onErr lua.LValue

	// mqttHandlers is the k.mqtt_on registration list (Phase 2); inbound
	// messages are dispatched in registration order.
	mqttHandlers []mqttHandler

	// allowNet gates outbound network clients (k.mqtt_connect) — Options.
	allowNet bool

	// onErrBusy guards against re-entrancy: while a hook runs, further errors
	// (from the hook itself) set ERRORCODE/ERRORMSG but skip re-invoking it.
	onErrBusy bool

	// errCache remembers the last ERRORCODE/ERRORMSG written to THIS state so
	// identical repeated failures skip touching the globals. Per-Env (per
	// LState) so fresh sessions always observe freshly-seeded values.
	errCache struct {
		code lua.LValue
		msg  lua.LValue
	}
}

// Logger interface for logging.
type Logger interface {
	Printf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
	Warnf(format string, args ...interface{})
	Tracef(format string, args ...interface{})
}

// Verbose returns whether verbose tracing is enabled.
func (e *Env) Verbose() bool {
	return e.verbose
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
	"popup":                     "flow",
	"clipboard_set":             "flow",
	"clipboard_get":             "flow",
	"pick_file":                 "flow",
	"bell":                      "flow",
	"screen_size":               "flow",
	"http_request":              "flow",
	"yield":                     "flow",
	"on_error":                  "flow",
	"assign":                    "flow",
	"set":                       "flow",
	"exec":                      "flow",
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
	"set_property":              "forms",
	"get_property":              "forms",
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
	"ctrl.looper":               "controls",
	"looper":                    "controls", // namespace
	"looper.link_db":            "controls",
	"looper.set_db_source":      "controls",
	"looper.refresh":            "controls",
	"looper.add_line":           "controls",
	"looper.set_line":           "controls",
	"looper.delete_line":        "controls",
	"ctrl.chart":                "controls",
	"chart":                     "controls", // namespace
	"ctrl.grid":                 "controls",
	"grid":                      "controls", // namespace
	"grid.refresh":              "controls",
	"grid.set_db_source":        "controls",
	"grid.get_selected":         "controls",
	"grid.get_row":              "controls",
	"grid.delete_row":           "controls",
	"grid.batch_delete":         "controls",
	"grid.insert_row":           "controls",
	"grid.update_row":           "controls",
	"ctrl.image":                "controls",
	"ctrl.login":                "controls",
	"ctrl.tree":                 "controls",
	"ctrl.tab":                  "controls",
	"tab":                       "controls", // namespace
	"tab.set_tab":               "controls",
	"tab.get_tab":               "controls",
	"tab.get_tab_count":         "controls",
	"tab.add_tab":               "controls",
	"tab.insert_tab":            "controls",
	"tab.remove_tab":            "controls",
	"tab.set_tab_title":         "controls",
	"tab.set_tab_icon":          "controls",
	"ctrl.topbar":               "controls",
	"ctrl.sidebar":              "controls",
	"ctrl.footer":               "controls",
	"ctrl.ai_chat":              "controls",
	"ctrl.select_text":          "controls",
	"ctrl.set_selection":        "controls",
	"ctrl.get_selection":        "controls",
	"ctrl.get_item_count":       "controls",
	"ctrl.execute_event":        "controls",
	"form.ai_agent":             "forms",
	"ai":                        "ai", // namespace
	"ai.tool":                   "ai",
	"ai_chat":                   "controls", // namespace
	"ai_chat.send":              "controls",
	"ai_chat.stop":              "controls",
	"ai_chat.add_message":       "controls",
	"ai_chat.get_transcript":    "controls",
	"ai_chat.set_transcript":    "controls",
	"ai_chat.clear":             "controls",
	"ai_chat.set_system":        "controls",
	"ai_chat.is_busy":           "controls",
	"chart.set_data":            "controls",
	"chart.add_dataset":         "controls",
	"chart.remove_dataset":      "controls",
	"chart.update_dataset":      "controls",
	"chart.set_labels":          "controls",
	"chart.set_options":         "controls",
	"chart.get_image":           "controls",
	"chart.resize":              "controls",
	"table":                     "controls", // namespace
	"table.add_line":            "controls",
	"table.delete_line":         "controls",
	"table.set_column_value":    "controls",
	"table.get_column_value":    "controls",
	"table.get_selected_column": "controls",
	"table.set_selected_column": "controls",
	"table.set_data":            "controls",
	"table.get_data":            "controls",
	"table.get_selected_rows":   "controls",
	"table.set_remote_data":     "controls",
	"table.refresh":             "controls",
	"table.set_db_source":       "controls",
	"table.find":                "controls",
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
	"tcp.accept":   "server",
	// tier-2 flow
	"timer_start":  "flow",
	"timer_stop":   "flow",
	"status_show":  "flow",
	"status_close": "flow",
	"param_set":    "flow",
	"param_get":    "flow",
	"net_ok":       "flow",
	"locale":       "flow",
	"ping":         "flow",
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
	"crypt_symmetric":  "crypto",
	"crypt_asymmetric": "crypto",
	"sign":             "crypto",
	"verify":           "crypto",
	// tier-2 files
	"zip_list":    "files",
	"zip_add":     "files",
	"zip_extract": "files",
	// tier-2 data formats
	"csv_parse":   "formats",
	"csv_string":  "formats",
	"csv_load":    "formats",
	"csv_save":    "formats",
	"ini_parse":   "formats",
	"ini_string":  "formats",
	"ini_load":    "formats",
	"ini_save":    "formats",
	"ini_read":    "formats",
	"ini_write":   "formats",
	"yaml_parse":  "formats",
	"yaml_string": "formats",
	"yaml_load":   "formats",
	"yaml_save":   "formats",
	"xml_load":    "formats",
	"xml_save":    "formats",
	// tier-2 rows conversions
	"json_to_rows": "rows",
	"rows_to_json": "rows",
	"csv_to_rows":  "rows",
	"rows_to_csv":  "rows",
	"xml_to_rows":  "rows",
	"rows_to_xml":  "rows",
	// Phase 1 REST router + HTTP helpers (serve-communication-plan.md §1)
	"http_routes":               "server", // script-set declarative route table
	"http_middleware":           "server", // script-set middleware chain
	"http_response_json":        "server",
	"http_response_text":        "server",
	"http_response_redirect":     "server",
	"http_request_parse_multipart": "server",
	"http_auth_middleware":       "server",
	"http_stats":                 "server",
	// Phase 8 structured logging (serve-communication-plan.md §8)
	"log":         "server", // namespace
	"log.info":    "server",
	"log.warn":    "server",
	"log.error":   "server",
	"log.debug":   "server",
	"log.trace":   "server",
	"log.stats":   "server",
	// Phase 2 MQTT client (serve-communication-plan.md §2)
	"mqtt_connect":    "comm",
	"mqtt_subscribe":  "comm",
	"mqtt_publish":    "comm",
	"mqtt_unsubscribe": "comm",
	"mqtt_close":      "comm",
	"mqtt_on":         "comm",
	"mqtt_stats":      "comm",
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

// registerInTable registers a function in a specific sub-table of k.
// e.g., registerInTable(k.file, "open", "files", fn) creates k.file.open
func (e *Env) registerInTable(parent *lua.LTable, name, group string, fn lua.LGFunction) {
	e.known[name] = group
	registerKnown[name] = group

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

	parent.RawSetString(name, e.L.NewFunction(callFn))
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

// seedErrorGlobals writes the initial ERRORCODE/ERRORMSG (nil/"") and resets
// the per-Env cache so the first failure in this LState always re-writes them.
func seedErrorGlobals(L *lua.LState, e *Env) {
	L.SetGlobal("ERRORCODE", lua.LNil)
	L.SetGlobal("ERRORMSG", lua.LString(""))
	e.errCache.code = lua.LNil
	e.errCache.msg = lua.LString("")
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

// defaultDenyFS returns the list of host-owned artefacts living in the sandbox
// home that scripts must never read or write, even through AllowFS. Today these
// are the k.param_set/get persistence file and the serve-mode structured-log
// sink (klog.db, serve-communication-plan.md Phase 8). Entries are
// symlink-canonicalized so they compare equal to resolvePath output (which
// resolves the same way on macOS /var -> /private/var).
func defaultDenyFS(workdir string) []string {
	params := filepath.Join(workdir, ".kalua.params.json")
	if resolved, err := evalSymlinksBestEffort(params); err == nil {
		params = resolved
	}
	klogPath := filepath.Join(workdir, klog.DefaultPath)
	if resolved, err := evalSymlinksBestEffort(klogPath); err == nil {
		klogPath = resolved
	}
	return []string{params, klogPath}
}

// denyFSOf resolves the operator-supplied DenyFS entries to absolute,
// symlink-resolved paths.
func denyFSOf(opts Options) []string {
	var out []string
	for _, p := range opts.DenyFS {
		abs, err := filepathAbs(p)
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

// denyFSFor combines the operator-supplied DenyFS entries with the host
// artefact defaults for the given sandbox home.
func denyFSFor(opts Options, workdir string) []string {
	return append(denyFSOf(opts), defaultDenyFS(workdir)...)
}

// deniedPath reports whether the absolute path p equals or lies inside a
// DenyFS entry. Entries are exact files or directories; a sibling of a file
// entry (e.g. "klog.db.bak" vs "klog.db") is not denied.
func (e *Env) deniedPath(p string) bool {
	for _, d := range e.denyFS {
		rel, err := filepath.Rel(d, p)
		if err != nil {
			continue
		}
		if rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..") {
			return true
		}
	}
	return false
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

// Kalipso K_ERROR_* negative error codes (§5.4 "error codes"). A binding that
// fails via e.fail picks the closest code, defaulting to K_ERROR_GENERIC (-1).
const (
	KErrorGeneric      = -1
	KErrorInvalidParam = -6
	KErrorInvalidPK    = -7
	KErrorUserCanceled = -8
	KErrorConnected    = -12 // already connected
	KErrorNotConnected = -13 // not connected
	KErrorLoadFile     = -14
	KErrorSaveFile     = -15
	KErrorParse        = -16
	KErrorNotFound     = -17
	KErrorComm         = -18 // connection/communication failure
	KErrorCommRemote   = -19
	KErrorPermission   = -20
)

// setErrorGlobals writes ERRORCODE / ERRORMSG into the state, caching the last
// written values on the Env so repeated identical writes are cheap.
func (e *Env) setErrorGlobals(L *lua.LState, code int, msg string) {
	c := lua.LNumber(code)
	m := lua.LString(msg)
	if e.errCache.code != c {
		L.SetGlobal("ERRORCODE", c)
		e.errCache.code = c
	}
	if e.errCache.msg != m {
		L.SetGlobal("ERRORMSG", m)
		e.errCache.msg = m
	}
}

// fireError sets the ERRORCODE/ERRORMSG globals and, when a k.on_error hook is
// registered, invokes it with (code, msg) protected by pcall so a misbehaving
// hook can never abort the binding or recurse. Genuinely fatal Lua errors call
// this too (via fireErrorFromHeart / session / worker frames).
func (e *Env) fireError(L *lua.LState, code int, msg string) {
	e.setErrorGlobals(L, code, msg)
	if e == nil || e.onErr == nil || e.onErr == lua.LNil {
		return
	}
	fn, ok := e.onErr.(*lua.LFunction)
	if !ok || e.onErrBusy {
		return
	}
	e.onErrBusy = true
	defer func() { e.onErrBusy = false }()
	L.SetTop(0)
	L.Push(fn)
	L.Push(lua.LNumber(code))
	L.Push(lua.LString(msg))
	_ = L.PCall(2, 0, nil)
}

// fail is the catch+continue error path for a binding: it records the Kalipso
// error (globals + optional k.on_error hook) and pushes nil so the script
// continues and can branch on ERRORCODE. Callers `return e.fail(...)`.
func (e *Env) fail(L *lua.LState, code int, msg string) int {
	e.fireError(L, code, msg)
	L.Push(lua.LNil)
	return 1
}

// ClassifyError maps a host-side error to the closest Kalipso error code.
// Exported for host frames (session/worker) that fire the hook on genuine
// errors. See classifyError for details.
func ClassifyError(err error) int { return classifyError(err) }

// FireError records a Kalipso error (ERRORCODE/ERRORMSG globals + optional
// k.on_error hook) without returning a value to Lua. Host frames call this
// when a genuine Lua error aborts a script frame so the hook still fires.
func (e *Env) FireError(L *lua.LState, code int, msg string) {
	e.fireError(L, code, msg)
}

// classifyError maps a host-side error to the closest Kalipso error code.
// Used by bindings that aggregate many failure modes (file ops, runBlocking).
func classifyError(err error) int {
	if err == nil {
		return KErrorGeneric
	}
	if errors.Is(err, os.ErrNotExist) {
		return KErrorNotFound
	}
	if errors.Is(err, os.ErrPermission) {
		return KErrorPermission
	}
	lower := strings.ToLower(err.Error())
	switch {
	case strings.Contains(lower, "denied"), strings.Contains(lower, "permission"):
		return KErrorPermission
	case strings.Contains(lower, "no such file"), strings.Contains(lower, "not found"):
		return KErrorNotFound
	case strings.Contains(lower, "unsupported"), strings.Contains(lower, "invalid"):
		return KErrorInvalidParam
	}
	return KErrorGeneric
}
