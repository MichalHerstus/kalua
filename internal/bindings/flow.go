// Package bindings implements the K.* literal-value helpers (expression
// functions) and the Phase 1 flow bindings k.print/sleep/quit/error.
package bindings

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/yuin/gopher-lua"

	"kalua/internal/coerce"
	"kalua/internal/common"
)

// registerHelpers installs the K.* helpers per §2.3 and §5.9:
//
//	K.EQ, K.NEQ, K.ADD — operator name constants
//	K.eq, K.ne, K.add  — binary operators with Kalipso coercion
//	K.tonum, K.tostr   — coercion helpers
//	K.truthy           — condition test for If(...)
func registerHelpers(e *Env, K *lua.LTable) {
	K.RawSetString("EQ", lua.LString("="))
	K.RawSetString("NEQ", lua.LString("<>"))
	K.RawSetString("ADD", lua.LString("+"))

	// eq(a,b) — numeric when both coerce (with ""→0), else string compare
	K.RawSetString("eq", e.L.NewFunction(func(L *lua.LState) int {
		a := v(L.Get(1))
		b := v(L.Get(2))
		L.Push(lvalue(coerce.Eq(a, b)))
		return 1
	}))

	// ne(a,b) — negation of eq
	K.RawSetString("ne", e.L.NewFunction(func(L *lua.LState) int {
		a := v(L.Get(1))
		b := v(L.Get(2))
		L.Push(lvalue(coerce.Ne(a, b)))
		return 1
	}))

	// add(a,b) — Kalipso +: numeric if both coerce, else concat
	K.RawSetString("add", e.L.NewFunction(func(L *lua.LState) int {
		a := v(L.Get(1))
		b := v(L.Get(2))
		L.Push(lvalue(coerce.Add(a, b)))
		return 1
	}))

	// tonum(x) → number, or 0 when not numeric
	K.RawSetString("tonum", e.L.NewFunction(func(L *lua.LState) int {
		a := v(L.Get(1))
		if n, ok := coerce.ToNum(a); ok {
			L.Push(lua.LNumber(n))
		} else {
			L.Push(lua.LNumber(0))
		}
		return 1
	}))

	// tostr(x) → Kalipso string form
	K.RawSetString("tostr", e.L.NewFunction(func(L *lua.LState) int {
		a := v(L.Get(1))
		L.Push(lua.LString(coerce.Stringify(a)))
		return 1
	}))

	// truthy(x) → Kalipso condition test
	K.RawSetString("truthy", e.L.NewFunction(func(L *lua.LState) int {
		a := v(L.Get(1))
		L.Push(lua.LBool(coerce.Truthy(a)))
		return 1
	}))
}

// registerFlow installs the Phase 1 flow bindings: k.print, k.sleep, k.quit,
// k.error. All use the Env's App to interact with the scheduler.
func registerFlow(e *Env) {
	// k.print(...) → log sink (joins with \t like Lua print)
	e.register("print", "flow", func(L *lua.LState) int {
		n := L.GetTop()
		if n == 0 {
			return 0
		}
		var parts []string
		for i := 1; i <= n; i++ {
			parts = append(parts, L.Get(i).String())
		}
		if e.Logger != nil {
			e.Logger.Printf("%s", join(parts, "\t"))
		} else {
			// Fallback if no logger was configured
			println(join(parts, "\t"))
		}
		return 0
	})

	// k.sleep(ms) — yields coroutine; resumes after ms milliseconds
	e.register("sleep", "flow", func(L *lua.LState) int {
		ms := L.ToInt64(1)
		if ms <= 0 {
			return 0
		}
		if e.Sess == nil {
			// Headless (test / serve) mode: block synchronously.
			time.Sleep(time.Duration(ms) * time.Millisecond)
			return 0
		}
		e.Sess.ScheduleSleep(L, time.Duration(ms)*time.Millisecond)
		return L.Yield(lua.LNil)
	})

	// k.yield() — yields the current coroutine, allowing other coroutines to run.
	// Returns any values passed to the next resume.
	e.register("yield", "flow", func(L *lua.LState) int {
		return L.Yield(lua.LNil)
	})

	// k.quit() — requests clean app termination at next scheduler tick
	e.register("quit", "flow", func(L *lua.LState) int {
		e.App.RequestQuit()
		return 0
	})

	// k.error(msg) — raises a deliberate Lua error
	e.register("error", "flow", func(L *lua.LState) int {
		msg := L.ToString(1)
		L.RaiseError("%s", msg)
		return 0
	})

	// k.msgbox([opts]) — show a message box and wait for user choice.
	//
	// Two call forms:
	//
	//	k.msgbox(text[, kind])              // legacy (kind: info/warn/error/ok-cancel/yes-no)
	//	k.msgbox{title=, message=, type=, buttons={...}}
	//
	// The rich form takes an options table: title, message, type ("info"/
	// "warning"/"danger" → left color strip), and buttons = list of {label, value}
	// pairs (or {label=.., value=..}, or bare strings). The clicked button's value
	// is returned with its original type; buttons default to a single "OK".
	// Returns the user's choice.
	e.register("msgbox", "flow", func(L *lua.LState) int {
		var opts common.MsgboxOptions
		if t, ok := L.Get(1).(*lua.LTable); ok {
			opts = msgboxFromTable(L, e, t)
		} else {
			text := L.ToString(1)
			kind := L.OptString(2, "info")
			opts = common.MsgboxOptions{Message: text, Kind: kind, Buttons: msgboxPresetButtons(kind)}
		}

		// Get the session from the env
		if e.Sess == nil {
			L.RaiseError("msgbox: no session available")
			return 0
		}

		// Send msgbox to browser and suspend the current coroutine. The
		// coroutine L is already started (we are running inside it), so the
		// actor can resume it later with the user's choice — creating a fresh
		// thread here would make Resume panic on the never-started thread.
		e.Sess.ShowMsgbox(L, func() {}, opts)

		// Yield the coroutine - it will be resumed when user responds
		return L.Yield(lua.LNil)
	})

	// k.popup(items) — show a multilevel menu-style popup and wait for a pick.
	//
	// Two call forms:
	//
	//	k.popup{{label=, value=}, {label=, items={...}}, "bare"}  // list form
	//	k.popup{title=, items={{label=, value=}, {label=, items={...}}}} // options form
	//
	// Each item is a leaf ({label, value} — the clicked value is returned with
	// its original type) or a branch ({label, items={...}} — opens a fly-out
	// submenu). Positional {label, value} pairs and bare strings are accepted
	// for leaves (bare string → label == value). Returns the picked value, or
	// nil when the popup is dismissed (Esc / click outside).
	e.register("popup", "flow", func(L *lua.LState) int {
		opts, err := popupFromTable(L, e, L.Get(1))
		if err != nil {
			L.RaiseError("popup: %s", err)
			return 0
		}

		// Get the session from the env
		if e.Sess == nil {
			L.RaiseError("popup: no session available")
			return 0
		}

		// Send the popup to the browser and suspend. Resumed via the actor
		// inbox with the picked value, or with no values when dismissed.
		e.Sess.ShowPopup(L, func() {}, opts)

		// Yield the coroutine - it will be resumed when the user picks an item
		return L.Yield(lua.LNil)
	})

	// k.clipboard_set(text) — write text to browser clipboard
	e.register("clipboard_set", "flow", func(L *lua.LState) int {
		text := L.ToString(1)
		if e.Sess == nil {
			L.RaiseError("clipboard_set: no session available")
			return 0
		}
		e.Sess.SendOutbox(common.OutboxMsg{
			Type: "clipboard_set",
			Text: text,
		})
		return 0
	})

	// k.clipboard_get() — read text from browser clipboard
	// Returns the clipboard text. Suspends the current coroutine until the
	// browser answers via the clipboard_resp round-trip.
	e.register("clipboard_get", "flow", func(L *lua.LState) int {
		if e.Sess == nil {
			L.RaiseError("clipboard_get: no session available")
			return 0
		}
		e.Sess.RequestClipboardGet(L, func() {})
		return L.Yield(lua.LNil)
	})

// k.pick_file([opts]) — open a browser file picker dialog.
// opts (optional table): {accept="image/*,.pdf", multiple=true, mode="open|save|download", filename="default.txt", data="base64 content"}
// mode: "open" (default) - pick existing files
//       "save" - pick location to save a file, returns {path, name}
//       "download" - download provided data to a file, returns {path, name}
// Returns a table of files for open mode: {{name, size, type, data}, ...}
// Returns {path, name} for save/download modes.
// Returns nil on cancel.
// Suspends the current coroutine until files are selected or cancelled.
	e.register("pick_file", "flow", func(L *lua.LState) int {
		if e.Sess == nil {
			L.RaiseError("pick_file: no session available")
			return 0
		}

		accept := ""
		multiple := false
		mode := "open"
		filename := ""
		fileData := ""

		opts := L.OptTable(1, nil)
		if opts != nil {
			if v := opts.RawGetString("accept"); v != lua.LNil {
				accept = v.String()
			}
			if v := opts.RawGetString("multiple"); v != lua.LNil {
				multiple = v == lua.LTrue || v.String() == "true"
			}
			if v := opts.RawGetString("mode"); v != lua.LNil {
				mode = v.String()
			}
			if v := opts.RawGetString("filename"); v != lua.LNil {
				filename = v.String()
			}
			if v := opts.RawGetString("data"); v != lua.LNil {
				fileData = v.String()
			}
		}

		// For save/download modes, use RequestFilePickerSave
		if mode == "save" || mode == "download" {
			e.Sess.RequestFilePickerSave(L, func() {}, mode, filename, fileData)
			return L.Yield(lua.LNil)
		}

		e.Sess.RequestFilePicker(L, func() {}, accept, multiple)
		return L.Yield(lua.LNil)
	})

	// k.bell() — play a system beep sound
	e.register("bell", "flow", func(L *lua.LState) int {
		if e.Sess == nil {
			L.RaiseError("bell: no session available")
			return 0
		}
		e.Sess.SendOutbox(common.OutboxMsg{
			Type: "bell",
		})
		return 0
	})

	// k.screen_size() — get viewport dimensions
	// Returns table with width and height
	e.register("screen_size", "flow", func(L *lua.LState) int {
		if e.Sess == nil {
			L.RaiseError("screen_size: no session available")
			return 0
		}
		w, h, _ := e.Sess.ClientInfo()
		// Fall back to defaults before the browser reports client_info.
		if w <= 0 {
			w = 1024
		}
		if h <= 0 {
			h = 768
		}
		tbl := L.NewTable()
		tbl.RawSetString("width", lua.LNumber(w))
		tbl.RawSetString("height", lua.LNumber(h))
		L.Push(tbl)
		return 1
	})

	// k.http_request(opts) — make an HTTP request
	// opts: {method="GET", url="...", headers={...}, body="...", timeout=30000}
	// Returns {status=200, headers={...}, body="..."}
	e.register("http_request", "flow", func(L *lua.LState) int {
		if e.Sess == nil {
			L.RaiseError("http_request: no session available")
			return 0
		}

		opts := L.CheckTable(1)
		method := opts.RawGetString("method")
		if method == lua.LNil {
			method = lua.LString("GET")
		}
		url := opts.RawGetString("url")
		if url == lua.LNil {
			L.RaiseError("http_request: url is required")
			return 0
		}
		body := opts.RawGetString("body")
		if body == lua.LNil {
			body = lua.LString("")
		}
		timeoutMs := opts.RawGetString("timeout")
		timeout := 30 * time.Second
		if timeoutMs != lua.LNil {
			timeout = time.Duration(timeoutMs.(lua.LNumber)) * time.Millisecond
		}

		// Build headers
		headers := make(http.Header)
		headersTbl := opts.RawGetString("headers")
		if headersTbl != lua.LNil {
			if ht, ok := headersTbl.(*lua.LTable); ok {
				ht.ForEach(func(k, v lua.LValue) {
					headers.Add(k.String(), v.String())
				})
			}
		}

		// Run the HTTP work in a worker goroutine and suspend the current
		// coroutine L (already running), which the actor resumes with the
		// result. A fresh NewThread here would panic on resume (gopher-lua).
		e.Sess.RunAsync(L, func() {}, func() (interface{}, error) {
			client := &http.Client{Timeout: timeout}

			var bodyReader io.Reader
			if body.String() != "" {
				bodyReader = strings.NewReader(body.String())
			}
			req, err := http.NewRequest(method.String(), url.String(), bodyReader)
			if err != nil {
				return nil, err
			}
			req.Header = headers

			resp, err := client.Do(req)
			if err != nil {
				return nil, err
			}
			defer resp.Body.Close()

			// Read response body
			respBody := make([]byte, 0, 4096)
			buf := make([]byte, 4096)
			for {
				n, err := resp.Body.Read(buf)
				if n > 0 {
					respBody = append(respBody, buf[:n]...)
				}
				if err != nil {
					break
				}
			}

			// Build response headers
			respHeaders := make(map[string]string)
			for k, v := range resp.Header {
				if len(v) > 0 {
					respHeaders[k] = v[0]
				}
			}

			return map[string]interface{}{
				"status":  resp.StatusCode,
				"headers": respHeaders,
				"body":    string(respBody),
			}, nil
		}, func(L *lua.LState, v interface{}) lua.LValue {
			m, ok := v.(map[string]interface{})
			if !ok {
				return lua.LNil
			}
			tbl := L.NewTable()
			if status, ok := m["status"].(int); ok {
				tbl.RawSetString("status", lua.LNumber(status))
			}
			if headers, ok := m["headers"].(map[string]string); ok {
				htbl := L.NewTable()
				for k, v := range headers {
					htbl.RawSetString(k, lua.LString(v))
				}
				tbl.RawSetString("headers", htbl)
			}
			if body, ok := m["body"].(string); ok {
				tbl.RawSetString("body", lua.LString(body))
			}
			return tbl
		})

		return L.Yield(lua.LNil)
	})

	// k.timer_start(id, ms[, repeats]) — starts a session-scoped timer. On fire
	// the actor calls a Lua function named id (or a "timer" form handler).
	e.register("timer_start", "flow", func(L *lua.LState) int {
		id := L.CheckString(1)
		ms := L.CheckInt(2)
		repeats := L.OptBool(3, false)
		if e.Sess == nil {
			L.RaiseError("timer_start: no session available (run mode only)")
			return 0
		}
		e.Sess.StartTimer(id, ms, repeats)
		return 0
	})

	// k.timer_stop(id) — stops a running session timer.
	e.register("timer_stop", "flow", func(L *lua.LState) int {
		id := L.CheckString(1)
		if e.Sess == nil {
			L.RaiseError("timer_stop: no session available (run mode only)")
			return 0
		}
		e.Sess.StopTimer(id)
		return 0
	})

	// k.status_show(text) — show a busy/status bar (spec §5.8).
	e.register("status_show", "flow", func(L *lua.LState) int {
		text := L.ToString(1)
		sendOutbox(e, common.OutboxMsg{Type: "status", Text: text})
		return 0
	})

	// k.status_close() — hides the status bar.
	e.register("status_close", "flow", func(L *lua.LState) int {
		sendOutbox(e, common.OutboxMsg{Type: "status_close"})
		return 0
	})

	// k.param_set(key, value) — persists an app param to a file next to the
	// sandbox home (spec §5.2). Values are strings.
	e.register("param_set", "flow", func(L *lua.LState) int {
		key := L.CheckString(1)
		value := luaToString(L, 2)
		e.setParam(key, value)
		return 0
	})

	// k.param_get(key) — reads an app param (string; "" if unset).
	e.register("param_get", "flow", func(L *lua.LState) int {
		key := L.CheckString(1)
		L.Push(lua.LString(e.getParam(key)))
		return 1
	})

	// k.net_ok(timeout_ms) — reachability check (TCP dial to a public host).
	e.register("net_ok", "flow", func(L *lua.LState) int {
		timeout := time.Duration(L.OptInt(1, 3000)) * time.Millisecond
		ok := netOK(timeout)
		L.Push(lua.LBool(ok))
		return 1
	})

	// k.locale() — browser/locale string (best-effort; "en-US" default).
	e.register("locale", "flow", func(L *lua.LState) int {
		L.Push(lua.LString(e.locale()))
		return 1
	})

	// k.ping(host, timeout_ms) — TCP-based latency in ms; nil when unreachable.
	e.register("ping", "flow", func(L *lua.LState) int {
		host := L.CheckString(1)
		timeout := time.Duration(L.OptInt(2, 2000)) * time.Millisecond
		ms, ok := pingHost(host, timeout)
		if !ok {
			L.Push(lua.LNil)
			return 1
		}
		L.Push(lua.LNumber(ms))
		return 1
	})

	// k.assign / k.set / k.exec (action set trio, §5.2)
	registerAssignSetExec(e)
}

// registerAssignSetExec registers k.assign, k.set, k.exec (action set trio, §5.2).
// Called from registerFlow (run mode) and SetupServe (serve mode).
func registerAssignSetExec(e *Env) {
	// k.assign(target, kind, value) — set a global or control value with coercion.
	// target: string (global name) or table {form=, ctrl=} (control target)
	// kind: "numeric"|"string"|"boolean"|"date"|"number"|"bool"
	// Returns the coerced value.
	e.register("assign", "flow", func(L *lua.LState) int {
		target := L.Get(1)
		kind := L.CheckString(2)
		val := L.Get(3)

		// Coerce value according to kind
		var coerced lua.LValue
		switch strings.ToLower(kind) {
		case "numeric", "number":
			if n, ok := coerce.ToNum(val); ok {
				coerced = lua.LNumber(n)
			} else {
				coerced = lua.LNumber(0)
			}
		case "string":
			coerced = lua.LString(coerce.Stringify(val))
		case "boolean", "bool":
			coerced = lua.LBool(coerce.Truthy(val))
		case "date":
			coerced = lua.LString(coerce.Stringify(val))
		default:
			coerced = val
		}

		// String target -> set global
		if lv, ok := target.(lua.LString); ok {
			L.SetGlobal(string(lv), coerced)
			L.Push(coerced)
			return 1
		}

		// Table target {form=, ctrl=} -> set control value
		if tbl, ok := target.(*lua.LTable); ok {
			formName := tbl.RawGetString("form")
			ctrlName := tbl.RawGetString("ctrl")
			if formName == lua.LNil || ctrlName == lua.LNil {
				L.RaiseError("assign: control target requires form and ctrl fields")
				return 0
			}
			if e.Sess == nil {
				L.RaiseError("assign: no session available for control target")
				return 0
			}
			// Use set_value logic via control
			ctrl := getControl(L, formName.String(), ctrlName.String())
			if ctrl == nil {
				L.RaiseError("assign: control not found: %s.%s", formName, ctrlName)
				return 0
			}
			ctrl.RawSetString("value", coerced)
			if ctrl.RawGetString("type").String() == "image" {
				ctrl.RawSetString("src", coerced)
			}
			html := renderControl(ctrl)
			sendOutbox(e, common.OutboxMsg{
				Type:     "update_control",
				Form:     formName.String(),
				Ctrl:     ctrlName.String(),
				Selector: "#c:" + formName.String() + ":" + ctrlName.String(),
				HTML:     html,
			})
			L.Push(coerced)
			return 1
		}

		L.RaiseError("assign: target must be string (global) or table {form, ctrl}")
		return 0
	})

	// k.set(name, fn) — store a function in the action registry.
	e.register("set", "flow", func(L *lua.LState) int {
		name := L.CheckString(1)
		fn := L.CheckFunction(2)

		// Store in _kalua_actions global table
		actions := L.GetGlobal("_kalua_actions")
		if actions == lua.LNil {
			actions = L.NewTable()
			L.SetGlobal("_kalua_actions", actions)
		}
		actionsTbl, ok := actions.(*lua.LTable)
		if !ok {
			L.RaiseError("set: _kalua_actions corrupted")
			return 0
		}
		actionsTbl.RawSetString(name, fn)
		return 0
	})

	// k.exec(name, ...) — execute a stored function asynchronously.
	// Returns the function's result(s) when it completes.
	e.register("exec", "flow", func(L *lua.LState) int {
		name := L.CheckString(1)
		fn := L.GetGlobal("_kalua_actions")
		if fn == lua.LNil {
			L.RaiseError("exec: no actions registered")
			return 0
		}
		actionsTbl, ok := fn.(*lua.LTable)
		if !ok {
			L.RaiseError("exec: _kalua_actions corrupted")
			return 0
		}
		actionFn := actionsTbl.RawGetString(name)
		if actionFn == lua.LNil {
			L.RaiseError("exec: action not found: %s", name)
			return 0
		}
		lfn, ok := actionFn.(*lua.LFunction)
		if !ok {
			L.RaiseError("exec: %s is not a function", name)
			return 0
		}

		// Collect arguments (2..top)
		var args []lua.LValue
		for i := 2; i <= L.GetTop(); i++ {
			args = append(args, L.Get(i))
		}

		if e.Sess == nil {
			L.RaiseError("exec: no session available")
			return 0
		}

		// Request async execution and yield
		e.Sess.RequestExec(L, func() {}, lfn, args)
		return L.Yield(lua.LNil)
	})
}

// msgboxFromTable parses the rich options-table form of k.msgbox.
func msgboxFromTable(L *lua.LState, e *Env, t *lua.LTable) common.MsgboxOptions {
	opts := common.MsgboxOptions{Kind: "info"}
	if v := t.RawGetString("title"); v != lua.LNil {
		opts.Title = lua.LVAsString(v)
	}
	if v := t.RawGetString("message"); v != lua.LNil {
		opts.Message = lua.LVAsString(v)
	}
	if v := t.RawGetString("type"); v != lua.LNil {
		if k := lua.LVAsString(v); k != "" && k != "info" {
			opts.Kind = k
		}
	}

	btns := t.RawGetString("buttons")
	if bt, ok := btns.(*lua.LTable); ok {
		bt.ForEach(func(_, bv lua.LValue) {
			label := ""
			value := lua.LNil
			switch b := bv.(type) {
			case *lua.LTable:
				label = lua.LVAsString(b.RawGetString("label"))
				if label == "" {
					label = lua.LVAsString(b.RawGetInt(1))
				}
				value = b.RawGetString("value")
				if value == lua.LNil {
					value = b.RawGetInt(2)
				}
			default:
				label = lua.LVAsString(bv)
				value = bv
			}
			if value == lua.LNil {
				value = lua.LString(label)
			}
			opts.Buttons = append(opts.Buttons, common.MsgboxButton{Label: label, Value: jsonValue(e, value)})
		})
	}

	if len(opts.Buttons) == 0 {
		opts.Buttons = msgboxPresetButtons("info")
	}
	return opts
}

// msgboxPresetButtons returns the button set for a legacy k.msgbox kind.
// Each button's return value is its lowercase choice string, matching the
// historical k.msgbox contract.
func msgboxPresetButtons(kind string) []common.MsgboxButton {
	switch kind {
	case "ok-cancel":
		return []common.MsgboxButton{{Label: "OK", Value: "\"ok\""}, {Label: "CANCEL", Value: "\"cancel\""}}
	case "yes-no":
		return []common.MsgboxButton{{Label: "YES", Value: "\"yes\""}, {Label: "NO", Value: "\"no\""}}
	default:
		return []common.MsgboxButton{{Label: "OK", Value: "\"ok\""}}
	}
}

// popupFromTable parses the arguments of k.popup. The first argument is either
// an options table ({title=, items={...}}) or the menu item list itself. The
// options form is detected by a non-nil "items" string key.
func popupFromTable(L *lua.LState, e *Env, arg lua.LValue) (common.PopupOptions, error) {
	t, ok := arg.(*lua.LTable)
	if !ok {
		return common.PopupOptions{}, errors.New("expected a menu table")
	}

	opts := common.PopupOptions{}
	// The branch-max-depth guard lives here; children are appended by
	// popupParseItems, which re-enters for the item list only.
	if items := t.RawGetString("items"); items != lua.LNil {
		if v := t.RawGetString("title"); v != lua.LNil {
			opts.Title = lua.LVAsString(v)
		}
		itemsTbl, ok := items.(*lua.LTable)
		if !ok {
			return opts, errors.New("items must be a list of menu items")
		}
		opts.Items, _ = popupParseItems(L, e, itemsTbl, 0)
		return opts, nil
	}

	opts.Items, _ = popupParseItems(L, e, t, 0)
	return opts, nil
}

// popupParseItems converts a Lua list of menu items into PopupItem records.
// depth caps nested submenus. Each item is a leaf ({label,value} / {label=,value=}
// / bare string) or a branch ({label=, items={...}}).
func popupParseItems(L *lua.LState, e *Env, t *lua.LTable, depth int) ([]common.PopupItem, error) {
	if depth > 8 {
		return nil, errors.New("menu nesting too deep (max 8 levels)")
	}
	var items []common.PopupItem
	var err error
	t.ForEach(func(_, iv lua.LValue) {
		if err != nil {
			return
		}
		label := ""
		value := lua.LNil
		var sub *lua.LTable
		switch it := iv.(type) {
		case *lua.LTable:
			label = lua.LVAsString(it.RawGetString("label"))
			if label == "" {
				label = lua.LVAsString(it.RawGetInt(1))
			}
			if v := it.RawGetString("value"); v != lua.LNil {
				value = v
			} else if v := it.RawGetInt(2); v != lua.LNil {
				value = v
			}
			if s := it.RawGetString("items"); s != lua.LNil {
				if st, ok := s.(*lua.LTable); ok {
					sub = st
				}
			}
		default:
			label = lua.LVAsString(iv)
			value = iv
		}
		if label == "" {
			label = "?"
		}

		if sub != nil {
			children, err2 := popupParseItems(L, e, sub, depth+1)
			if err2 != nil {
				err = err2
				return
			}
			items = append(items, common.PopupItem{Label: label, Items: children})
			return
		}
		if value == lua.LNil {
			value = lua.LString(label)
		}
		items = append(items, common.PopupItem{Label: label, Value: jsonValue(e, value)})
	})
	return items, err
}

// jsonValue JSON-encodes a Lua value for transport in a msgbox button's
// data-k-value attribute.
func jsonValue(e *Env, v lua.LValue) string {
	s, err := stringifyJSON(e, v)
	if err != nil {
		return "\"" + strings.ReplaceAll(lua.LVAsString(v), "\"", "\\\"") + "\""
	}
	return s
}

func join(elems []string, sep string) string {
	switch len(elems) {
	case 0:
		return ""
	case 1:
		return elems[0]
	}
	n := len(sep) * (len(elems) - 1)
	for _, s := range elems {
		n += len(s)
	}
	b := make([]byte, n)
	bp := copy(b, elems[0])
	for _, s := range elems[1:] {
		bp += copy(b[bp:], sep)
		bp += copy(b[bp:], s)
	}
	return string(b[:bp])
}
