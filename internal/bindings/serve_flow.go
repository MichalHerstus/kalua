//go:build !wasm

package bindings

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/yuin/gopher-lua"
)
func registerServeFlow(e *Env) {
	e.register("yield", "flow", func(L *lua.LState) int {
		return L.Yield(lua.LNil)
	})

	e.register("error", "flow", func(L *lua.LState) int {
		msg := L.ToString(1)
		L.RaiseError("%s", msg)
		return 0
	})

	e.register("param_set", "flow", func(L *lua.LState) int {
		key := L.CheckString(1)
		value := luaToString(L, 2)
		e.setParam(key, value)
		return 0
	})

	e.register("param_get", "flow", func(L *lua.LState) int {
		key := L.CheckString(1)
		L.Push(lua.LString(e.getParam(key)))
		return 1
	})

	e.register("net_ok", "flow", func(L *lua.LState) int {
		timeout := time.Duration(L.OptInt(1, 3000)) * time.Millisecond
		L.Push(lua.LBool(netOK(timeout)))
		return 1
	})

	e.register("locale", "flow", func(L *lua.LState) int {
		L.Push(lua.LString(e.locale()))
		return 1
	})

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

// registerServeHTTP installs k.http_request for serve mode (Phase 0.1): the
// headless version blocks the worker until the HTTP exchange completes and
// returns the {status, headers, body} table directly (no session coroutine to
// suspend).
func registerServeHTTP(e *Env) {
	e.register("http_request", "flow", func(L *lua.LState) int {
		return serveHTTPRequest(L, e, L.CheckTable(1))
	})
}

// serveHTTPRequest performs a blocking HTTP request from a k.http_request
// options table: {method, url, headers, body, timeout}. Returns the Lua result
// table ({status, headers, body}) or nil on failure (ERRORCODE/ERRORMSG set).
func serveHTTPRequest(L *lua.LState, e *Env, opts *lua.LTable) int {
	method := "GET"
	if m := opts.RawGetString("method"); m != lua.LNil {
		method = m.String()
	}
	url := opts.RawGetString("url")
	if url == lua.LNil {
		return e.fail(L, KErrorInvalidParam, "http_request: url is required")
	}
	body := opts.RawGetString("body")
	if body == lua.LNil {
		body = lua.LString("")
	}
	timeout := 30 * time.Second
	if t := opts.RawGetString("timeout"); t != lua.LNil {
		if n, ok := t.(lua.LNumber); ok {
			timeout = time.Duration(n) * time.Millisecond
		}
	}

	headers := make(http.Header)
	if ht := opts.RawGetString("headers"); ht != lua.LNil {
		if tbl, ok := ht.(*lua.LTable); ok {
			tbl.ForEach(func(k, v lua.LValue) {
				headers.Add(k.String(), v.String())
			})
		}
	}

	var bodyReader io.Reader
	if body.String() != "" {
		bodyReader = strings.NewReader(body.String())
	}
	req, err := http.NewRequest(method, url.String(), bodyReader)
	if err != nil {
		return e.fail(L, KErrorInvalidParam, fmt.Sprintf("http_request: %v", err))
	}
	req.Header = headers

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return e.fail(L, KErrorComm, fmt.Sprintf("http_request: %v", err))
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	respHeaders := make(map[string]string)
	for k, v := range resp.Header {
		if len(v) > 0 {
			respHeaders[k] = v[0]
		}
	}

	tbl := L.NewTable()
	tbl.RawSetString("status", lua.LNumber(resp.StatusCode))
	ht := L.NewTable()
	for k, v := range respHeaders {
		ht.RawSetString(k, lua.LString(v))
	}
	tbl.RawSetString("headers", ht)
	tbl.RawSetString("body", lua.LString(string(respBody)))
	L.Push(tbl)
	return 1
}
