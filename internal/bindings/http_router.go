//go:build !wasm

package bindings

import (
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/textproto"
	"strings"

	"github.com/yuin/gopher-lua"
)

// registerHTTPHelpers installs the Phase 1 REST router helpers
// (serve-communication-plan.md §1): k.http_response_json/text/redirect,
// k.http_request_parse_multipart, k.http_auth_middleware, and k.http_stats.
// The declarative k.http_routes / k.http_middleware tables are read by the
// server (worker router), not bound here.
func registerHTTPHelpers(e *Env, opts Options) {
	e.register("http_response_json", "server", func(L *lua.LState) int {
		data := L.Get(1)
		status := L.OptInt(2, 200)
		body, err := stringifyJSON(e, data)
		if err != nil {
			return e.fail(L, KErrorInvalidParam, fmt.Sprintf("http_response_json: %v", err))
		}
		tbl := L.NewTable()
		tbl.RawSetString("status", lua.LNumber(status))
		hdr := L.NewTable()
		hdr.RawSetString("content-type", lua.LString("application/json"))
		tbl.RawSetString("headers", hdr)
		tbl.RawSetString("body", lua.LString(body))
		L.Push(tbl)
		return 1
	})

	e.register("http_response_text", "server", func(L *lua.LState) int {
		text := L.ToString(1)
		status := L.OptInt(2, 200)
		tbl := L.NewTable()
		tbl.RawSetString("status", lua.LNumber(status))
		hdr := L.NewTable()
		hdr.RawSetString("content-type", lua.LString("text/plain"))
		tbl.RawSetString("headers", hdr)
		tbl.RawSetString("body", lua.LString(text))
		L.Push(tbl)
		return 1
	})

	e.register("http_response_redirect", "server", func(L *lua.LState) int {
		url := L.ToString(1)
		status := L.OptInt(2, 302)
		tbl := L.NewTable()
		tbl.RawSetString("status", lua.LNumber(status))
		hdr := L.NewTable()
		hdr.RawSetString("location", lua.LString(url))
		tbl.RawSetString("headers", hdr)
		tbl.RawSetString("body", lua.LString(""))
		L.Push(tbl)
		return 1
	})

	// k.http_auth_middleware(mw) appends a middleware (a global function name
	// or a function value) to the chain. Prepend order: chain order preserved.
	e.register("http_auth_middleware", "server", func(L *lua.LState) int {
		mw := L.Get(1)
		if mw == lua.LNil {
			return e.fail(L, KErrorInvalidParam, "http_auth_middleware: expected a function or name")
		}
		k := e.L.GetGlobal("k").(*lua.LTable)
		chain := k.RawGetString("http_middleware")
		var tbl *lua.LTable
		if chain == lua.LNil {
			tbl = e.L.NewTable()
			k.RawSetString("http_middleware", tbl)
		} else if t, ok := chain.(*lua.LTable); ok {
			tbl = t
		} else {
			return e.fail(L, KErrorInvalidParam, "http_auth_middleware: k.http_middleware is not a table")
		}
		tbl.Append(mw)
		return 0
	})

	// k.http_request_parse_multipart(req) returns {fields={...}, files={...}}
	// parsed from a request table's multipart/form-data body.
	e.register("http_request_parse_multipart", "server", func(L *lua.LState) int {
		req := L.CheckTable(1)
		contentType := ""
		if h := req.RawGetString("headers"); h != lua.LNil {
			if ht, ok := h.(*lua.LTable); ok {
				for _, k := range []string{"content-type", "Content-Type", "CONTENT-TYPE"} {
					if v := ht.RawGetString(k); v != lua.LNil {
						contentType = v.String()
						break
					}
				}
			}
		}
		body := req.RawGetString("body")
		_, params, err := mime.ParseMediaType(contentType)
		if err != nil || !strings.HasPrefix(params["boundary"], "") || body == lua.LNil {
			return e.fail(L, KErrorInvalidParam, "http_request_parse_multipart: not a multipart request")
		}
		boundary := params["boundary"]
		if boundary == "" {
			return e.fail(L, KErrorInvalidParam, "http_request_parse_multipart: missing boundary")
		}
		mr := multipart.NewReader(strings.NewReader(body.String()), boundary)
		fields := L.NewTable()
		files := L.NewTable()
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				return e.fail(L, KErrorParse, fmt.Sprintf("http_request_parse_multipart: %v", err))
			}
			data, _ := io.ReadAll(part)
			name := part.FormName()
			if name == "" {
				continue
			}
			filename := part.FileName()
			if filename == "" {
				fields.RawSetString(name, lua.LString(string(data)))
				continue
			}
			ft := L.NewTable()
			ft.RawSetString("filename", lua.LString(filename))
			ft.RawSetString("content_type", lua.LString(part.Header.Get(textproto.CanonicalMIMEHeaderKey("Content-Type"))))
			ft.RawSetString("data", lua.LString(string(data)))
			files.RawSetString(name, ft)
		}
		out := L.NewTable()
		out.RawSetString("fields", fields)
		out.RawSetString("files", files)
		L.Push(out)
		return 1
	})

	// k.http_stats() -> {requests_total, errors_total, avg_latency_ms, active_connections}
	e.register("http_stats", "server", func(L *lua.LState) int {
		tbl := L.NewTable()
		if opts.Stats == nil {
			tbl.RawSetString("requests_total", lua.LNumber(0))
			tbl.RawSetString("errors_total", lua.LNumber(0))
			tbl.RawSetString("avg_latency_ms", lua.LNumber(0))
			tbl.RawSetString("active_connections", lua.LNumber(0))
			L.Push(tbl)
			return 1
		}
		tbl.RawSetString("requests_total", lua.LNumber(opts.Stats.RequestsTotal.Load()))
		tbl.RawSetString("errors_total", lua.LNumber(opts.Stats.ErrorsTotal.Load()))
		avg := 0.0
		if c := opts.Stats.LatencyCount.Load(); c > 0 {
			avg = float64(opts.Stats.LatencySumMs.Load()) / float64(c)
		}
		tbl.RawSetString("avg_latency_ms", lua.LNumber(avg))
		tbl.RawSetString("active_connections", lua.LNumber(opts.Stats.ActiveConns.Load()))
		L.Push(tbl)
		return 1
	})
}
