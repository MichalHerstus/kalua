// Package bindings implements the K.* literal-value helpers (expression
// functions) and the Phase 1 flow bindings k.print/sleep/quit/error.
package bindings

import (
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

// k.msgbox(text[, kind]) — show message box and wait for user choice
	// Returns the user's choice ("ok", "yes", "no", "cancel", etc.)
	e.register("msgbox", "flow", func(L *lua.LState) int {
		text := L.ToString(1)
		kind := L.OptString(2, "info")

		// Get the session from the env
		if e.Sess == nil {
			L.RaiseError("msgbox: no session available")
			return 0
		}

		// Send msgbox to browser and suspend the current coroutine. The
		// coroutine L is already started (we are running inside it), so the
		// actor can resume it later with the user's choice — creating a fresh
		// thread here would make Resume panic on the never-started thread.
		e.Sess.ShowMsgbox(L, func() {}, text, kind)

		// Yield the coroutine - it will be resumed when user responds
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
	// opts (optional table): {accept="image/*,.pdf", multiple=true}
	// Returns a table of files: {{name, size, type, data}, ...} where data is
	// base64-encoded content. Returns nil on cancel.
	// Suspends the current coroutine until files are selected or cancelled.
	e.register("pick_file", "flow", func(L *lua.LState) int {
		if e.Sess == nil {
			L.RaiseError("pick_file: no session available")
			return 0
		}

		accept := ""
		multiple := false

		opts := L.OptTable(1, nil)
		if opts != nil {
			if v := opts.RawGetString("accept"); v != lua.LNil {
				accept = v.String()
			}
			if v := opts.RawGetString("multiple"); v != lua.LNil {
				multiple = v == lua.LTrue || v.String() == "true"
			}
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
