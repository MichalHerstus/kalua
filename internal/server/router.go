//go:build !wasm

package server

import (
	"net/http"
	"strings"

	"github.com/julienschmidt/httprouter"
	"github.com/yuin/gopher-lua"
)

// httpRoute is one declarative REST route from k.http_routes.
// handler is the name of a global Lua function to call for matches.
type httpRoute struct {
	Method  string
	Path    string
	Handler string
}

// httpMiddleware is one entry of the k.http_middleware chain. It is either a
// global function name (Name != "") or a direct function value (Fn != nil).
type httpMiddleware struct {
	Name string
	Fn   *lua.LFunction
}

// httpRouter wraps httprouter for declarative k.http_routes routing in serve
// mode (serve-communication-plan.md Phase 1). Routes are matched with
// httprouter's radix tree (:param wildcards), then dispatched to Lua functions.
type httpRouter struct {
	tree   *httprouter.Router
	routes []httpRoute
	mw     []httpMiddleware
	// matchIdx is written by the stub handles registered in the tree: Lookup
	// returns the exact handle for the matched node, and invoking it records
	// which route index matched so we can dispatch to the Lua handler.
	matchIdx int
	// L is the owning worker's LState (handlers/middleware resolve at call
	// time so the router works with hot reloads and late-defined functions).
	L *lua.LState
}

// newHTTPRouter builds a router from the k.http_routes / k.http_middleware
// tables of the given LState. Returns nil when no routes are declared.
func newHTTPRouter(L *lua.LState) *httpRouter {
	k := L.GetGlobal("k").(*lua.LTable)
	routesVal := k.RawGetString("http_routes")
	if routesVal == lua.LNil {
		return nil
	}
	routesTbl, ok := routesVal.(*lua.LTable)
	if !ok {
		return nil
	}

	rt := &httpRouter{
		tree:     httprouter.New(),
		matchIdx: -1,
		L:        L,
	}
	routesTbl.ForEach(func(_, v lua.LValue) {
		row, ok := v.(*lua.LTable)
		if !ok {
			return
		}
		method := strings.ToUpper(lua.LVAsString(row.RawGetString("method")))
		path := lua.LVAsString(row.RawGetString("path"))
		handler := lua.LVAsString(row.RawGetString("handler"))
		if method == "" || path == "" || handler == "" {
			return
		}
		idx := len(rt.routes)
		rt.routes = append(rt.routes, httpRoute{Method: method, Path: path, Handler: handler})
		// Stub handle records the matched index when Lookup resolves this node.
		// Recover from httprouter panics on conflicting duplicate routes so a
		// single bad declaration does not abort startup; the router keeps the
		// routes registered before the conflict.
		func(m, p string, i int) {
			defer func() { _ = recover() }()
			rt.tree.Handle(m, p, func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
				rt.matchIdx = i
			})
		}(method, path, idx)
	})
	if len(rt.routes) == 0 {
		return nil
	}

	// Middleware chain: k.http_middleware = {"auth_middleware", ...} or
	// function values appended via k.http_auth_middleware(fn).
	if mwVal := k.RawGetString("http_middleware"); mwVal != lua.LNil {
		if mwTbl, ok := mwVal.(*lua.LTable); ok {
			mwTbl.ForEach(func(_, v lua.LValue) {
				switch m := v.(type) {
				case lua.LString:
					rt.mw = append(rt.mw, httpMiddleware{Name: string(m)})
				case *lua.LFunction:
					rt.mw = append(rt.mw, httpMiddleware{Fn: m})
				}
			})
		}
	}
	return rt
}

// Match looks up a route by method+path. On success it returns the route index
// and the matched :param values (as (name, value) pairs).
func (rt *httpRouter) Match(method, path string) (int, httprouter.Params, bool) {
	if rt == nil {
		return 0, nil, false
	}
	rt.matchIdx = -1
	// httprouter.Lookup returns (Handle, Params, TSR) — the third value is a
	// trailing-slash-redirect recommendation, not "matched". The handle is nil
	// when nothing matched; invoking the (non-nil) stub records the route.
	h, params, _ := rt.tree.Lookup(strings.ToUpper(method), path)
	if h == nil {
		return 0, nil, false
	}
	h(nil, nil, params)
	if rt.matchIdx < 0 || rt.matchIdx >= len(rt.routes) {
		return 0, nil, false
	}
	return rt.matchIdx, params, true
}

// Handler returns the resolved Lua function for a route index.
func (rt *httpRouter) RouteHandler(idx int) *lua.LFunction {
	if idx < 0 || idx >= len(rt.routes) {
		return nil
	}
	fn := rt.L.GetGlobal(rt.routes[idx].Handler)
	if fn == lua.LNil {
		return nil
	}
	if lf, ok := fn.(*lua.LFunction); ok {
		return lf
	}
	return nil
}

// MiddlewareFn resolves the i-th middleware entry to a callable or nil.
func (rt *httpRouter) MiddlewareFn(i int) *lua.LFunction {
	if i < 0 || i >= len(rt.mw) {
		return nil
	}
	m := rt.mw[i]
	if m.Fn != nil {
		return m.Fn
	}
	fn := rt.L.GetGlobal(m.Name)
	if lf, ok := fn.(*lua.LFunction); ok {
		return lf
	}
	return nil
}

// MiddlewareCount returns the number of middleware entries.
func (rt *httpRouter) MiddlewareCount() int { return len(rt.mw) }
