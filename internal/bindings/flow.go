// Package bindings implements the K.* literal-value helpers (expression
// functions) and the Phase 1 flow bindings k.print/sleep/quit/error.
package bindings

import (
	"net/http"
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
	// k.print(...) → stdout / log sink (joins with \t like Lua print)
	e.register("print", "flow", func(L *lua.LState) int {
		n := L.GetTop()
		if n == 0 {
			return 0
		}
		var parts []string
		for i := 1; i <= n; i++ {
			parts = append(parts, L.Get(i).String())
		}
		// TODO: hook into structured logger; for now write to stdout
		println(join(parts, "\t"))
		return 0
	})

	// k.sleep(ms) — yields coroutine; resumes after ms milliseconds
	e.register("sleep", "flow", func(L *lua.LState) int {
		ms := L.ToInt64(1)
		if ms <= 0 {
			return 0
		}
		return e.App.ScheduleSleep(L, time.Duration(ms)*time.Millisecond)
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

		// Get the current coroutine
		co, cancel := L.NewThread()
		defer cancel()

		// Get the session from the env
		if e.Sess == nil {
			L.RaiseError("msgbox: no session available")
			return 0
		}

		// Send msgbox to browser and suspend coroutine
		e.Sess.ShowMsgbox(co, cancel, text, kind)

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
	// Returns the clipboard text or empty string
	e.register("clipboard_get", "flow", func(L *lua.LState) int {
		if e.Sess == nil {
			L.RaiseError("clipboard_get: no session available")
			return 0
		}
		// For now, send request and return empty (async would need coroutine suspension)
		e.Sess.SendOutbox(common.OutboxMsg{
			Type: "clipboard_get",
		})
		L.Push(lua.LString(""))
		return 1
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
		// For now return default size; actual size comes from client_info
		tbl := L.NewTable()
		tbl.RawSetString("width", lua.LNumber(1024))
		tbl.RawSetString("height", lua.LNumber(768))
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

		co, cancel := L.NewThread()
		defer cancel()

		e.Sess.RunAsync(co, cancel, func() (interface{}, error) {
			client := &http.Client{Timeout: timeout}
			req, err := http.NewRequest(method.String(), url.String(), nil)
			if err != nil {
				return nil, err
			}
			if body.String() != "" {
				req.Body = http.NoBody
				// For simplicity, we don't set body here
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
}

// join is strings.Join without the import for the single call site.
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
