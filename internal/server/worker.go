//go:build !wasm

package server

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yuin/gopher-lua"

	"kalua/internal/bindings"
	"kalua/internal/vm"
)

// Worker represents a single Lua VM worker in the serve pool.
type Worker struct {
	id       int
	L        *lua.LState
	app      *vm.App
	env      *bindings.Env
	shared   *SharedState
	wsHub    *WSHub
	tcpHub   *TCPHub
	handlers WorkerHandlers
	router   *httpRouter
	// slot is a one-token semaphore serializing LState access: exactly one
	// handler (HTTP/WS/TCP/init/shutdown) runs at a time per worker. HTTP
	// waits a bounded time (busyTimeout) then answers 503 + Retry-After;
	// WS/TCP block until free (their connections lease the worker already).
	slot chan struct{}
	logger Logger

	refs      atomic.Int32 // active leases (HTTP handles, WS/TCP connections)
	retired   atomic.Bool  // true once superseded by a hot reload
	closeOnce sync.Once
}

// busyTimeout bounds how long CallHTTP waits for a busy worker before
// answering 503 + Retry-After (serve-communication-plan.md Phase 0.4).
const busyTimeout = 500 * time.Millisecond

// WorkerHandlers holds the Lua callback functions for serve mode.
type WorkerHandlers struct {
	HandleHTTP *lua.LFunction
	HandleWS   *lua.LFunction
	HandleTCP  *lua.LFunction
	Init       *lua.LFunction
	Shutdown   *lua.LFunction
}

// NewWorker creates a new worker with the given script.
func NewWorker(id int, scriptPath string, opts bindings.Options, shared *SharedState, wsHub *WSHub, tcpHub *TCPHub, logger Logger) (*Worker, error) {
	L := vm.New()
	app := vm.NewApp(L)

	// Load and compile the script
	chunkFn, err := vm.LoadFile(L, scriptPath)
	if err != nil {
		L.Close()
		return nil, err
	}

	// Setup serve-mode bindings (no UI bindings) BEFORE the chunk runs, so
	// top-level k.* calls and on_error registration work like run mode.
	env := bindings.SetupServe(L, app, opts, shared, wsHub, tcpHub, logger)
	bindings.SetupUIError(L)

	// Execute chunk to define main() and callbacks
	if err := L.CallByParam(lua.P{Fn: chunkFn, NRet: 0, Protect: true}); err != nil {
		L.Close()
		return nil, err
	}

	// Get handler functions from globals
	httpFn := L.GetGlobal("handle_http")
	wsFn := L.GetGlobal("handle_ws")
	tcpFn := L.GetGlobal("handle_tcp")
	initFn := L.GetGlobal("init")
	shutdownFn := L.GetGlobal("shutdown")

	var httpHandler, wsHandler, tcpHandler, initHandler, shutdownHandler *lua.LFunction
	if httpFn != lua.LNil {
		if fn, ok := httpFn.(*lua.LFunction); ok {
			httpHandler = fn
		}
	}
	if wsFn != lua.LNil {
		if fn, ok := wsFn.(*lua.LFunction); ok {
			wsHandler = fn
		}
	}
	if tcpFn != lua.LNil {
		if fn, ok := tcpFn.(*lua.LFunction); ok {
			tcpHandler = fn
		}
	}
	if initFn != lua.LNil {
		if fn, ok := initFn.(*lua.LFunction); ok {
			initHandler = fn
		}
	}
	if shutdownFn != lua.LNil {
		if fn, ok := shutdownFn.(*lua.LFunction); ok {
			shutdownHandler = fn
		}
	}

	// Get main function
	mainFn := L.GetGlobal("main")
	if mainFn == lua.LNil {
		L.Close()
		return nil, fmt.Errorf("main function not found")
	}
	mainLFn, ok := mainFn.(*lua.LFunction)
	if !ok {
		L.Close()
		return nil, fmt.Errorf("main is not a function")
	}

	// Run main() to initialize
	if err := app.Run(mainLFn); err != nil {
		L.Close()
		return nil, err
	}

	w := &Worker{
		id:     id,
		L:      L,
		app:    app,
		env:    env,
		shared: shared,
		wsHub:  wsHub,
		tcpHub: tcpHub,
		handlers: WorkerHandlers{
			HandleHTTP: httpHandler,
			HandleWS:   wsHandler,
			HandleTCP:  tcpHandler,
			Init:       initHandler,
			Shutdown:   shutdownHandler,
		},
		slot:   make(chan struct{}, 1),
		logger: logger,
	}
	// Seed the slot token: the worker starts idle.
	w.freeSlot()
	// Build the Phase 1 REST router from k.http_routes / k.http_middleware
	// (declared at the top level of the script).
	w.router = newHTTPRouter(L)
	return w, nil
}

// acquireSlot blocks until the worker's single Lua slot is free (WS/TCP/init/
// shutdown: they lease the worker already, so they may wait indefinitely,
// preserving the pre-Phase-0.4 behavior where w.mu serialized them).
func (w *Worker) acquireSlot() { <-w.slot }

// tryAcquireSlot claims the slot, waiting at most busyTimeout. On timeout (or
// when ctx is cancelled) it returns false and the caller answers 503 +
// Retry-After.
func (w *Worker) tryAcquireSlot(ctx context.Context) bool {
	timer := time.NewTimer(busyTimeout)
	defer timer.Stop()
	select {
	case <-w.slot:
		return true
	case <-timer.C:
		return false
	case <-ctx.Done():
		return false
	}
}

// freeSlot returns the slot token.
func (w *Worker) freeSlot() { w.slot <- struct{}{} }

// CallHTTP calls the handle_http callback with request data, or dispatches to
// a declarative REST route when one matches (Phase 1). Returns response status,
// headers, body, and error.
func (w *Worker) CallHTTP(ctx context.Context, req HTTPRequest) (HTTPResponse, error) {
	// Phase 0.4: instead of failing a busy worker instantly with 503, wait a
	// bounded time for it to free up (Retry-After advertises the wait). The
	// worker pool is round-robin so a short spin usually lands on a free one.
	if !w.tryAcquireSlot(ctx) {
		return HTTPResponse{
			Status:  503,
			Headers: map[string]string{"Retry-After": "1"},
			Body:    "worker busy, retry after 1s",
		}, fmt.Errorf("worker busy")
	}
	defer w.freeSlot()

	// Router first: a matched k.http_routes entry wins over handle_http.
	if w.router != nil {
		if resp, err, matched := w.callRoutedHTTP(req); matched {
			if err != nil {
				w.notifyError(err)
				return resp, err
			}
			return resp, nil
		}
	}

	if w.handlers.HandleHTTP == nil {
		return HTTPResponse{Status: 404, Body: "no handle_http callback"}, nil
	}

	// Push request as table (spec §2.5):
	// { method, path, query={...}, query_raw, headers={...}, body, remote_addr, tls }
	reqTable := w.L.NewTable()
	reqTable.RawSetString("method", lua.LString(req.Method))
	reqTable.RawSetString("path", lua.LString(req.Path))
	reqTable.RawSetString("query_raw", lua.LString(req.Query))
	reqTable.RawSetString("query", queryValuesTable(w.L, req.QueryValues))
	reqTable.RawSetString("body", lua.LString(req.Body))
	reqTable.RawSetString("remote_addr", lua.LString(req.RemoteAddr))
	reqTable.RawSetString("tls", lua.LBool(req.TLS))

	headersTable := w.L.NewTable()
	for k, v := range req.Headers {
		headersTable.RawSetString(k, lua.LString(v))
	}
	reqTable.RawSetString("headers", headersTable)

	// Call handle_http(req); the handler returns per the §2.5 response forms.
	fn := w.handlers.HandleHTTP
	err := w.L.CallByParam(lua.P{
		Fn:      fn,
		NRet:    1,
		Protect: true,
	}, reqTable)
	if err != nil {
		w.L.SetTop(0)
		w.notifyError(err)
		return HTTPResponse{Status: 500, Body: err.Error()}, err
	}

	// Decode the single returned value into the response.
	ret := w.L.Get(1)
	w.L.SetTop(0)

	resp := decodeHTTPResponse(w.L, ret)
	if resp.Status == 0 {
		resp.Status = 200
	}
	if resp.Headers == nil {
		resp.Headers = map[string]string{}
	}
	return resp, nil
}

// callRoutedHTTP dispatches a request to the declarative REST router: runs the
// k.http_middleware chain (each may short-circuit with a response), then calls
// the matched route's handler with a req table that includes params. Returns
// (resp, err, matched) — matched=false when no route matches so CallHTTP falls
// back to handle_http; a Lua error yields a 500 response plus the error.
func (w *Worker) callRoutedHTTP(req HTTPRequest) (HTTPResponse, error, bool) {
	idx, params, ok := w.router.Match(req.Method, req.Path)
	if !ok {
		return HTTPResponse{}, nil, false
	}

	// Build the req table (with params) once; middleware may extend it
	// (e.g. req.user = ...) and the handler sees those mutations.
	reqTable := w.L.NewTable()
	reqTable.RawSetString("method", lua.LString(req.Method))
	reqTable.RawSetString("path", lua.LString(req.Path))
	reqTable.RawSetString("query_raw", lua.LString(req.Query))
	reqTable.RawSetString("query", queryValuesTable(w.L, req.QueryValues))
	reqTable.RawSetString("body", lua.LString(req.Body))
	reqTable.RawSetString("remote_addr", lua.LString(req.RemoteAddr))
	reqTable.RawSetString("tls", lua.LBool(req.TLS))
	if params != nil {
		pTbl := w.L.NewTable()
		for _, p := range params {
			pTbl.RawSetString(p.Key, lua.LString(p.Value))
		}
		reqTable.RawSetString("params", pTbl)
	}
	headersTable := w.L.NewTable()
	for k, v := range req.Headers {
		headersTable.RawSetString(k, lua.LString(v))
	}
	reqTable.RawSetString("headers", headersTable)

	// Middleware chain: each middleware may short-circuit by returning a
	// response value (per k.http_response_* helpers); returning nil continues.
	for i := 0; i < w.router.MiddlewareCount(); i++ {
		mw := w.router.MiddlewareFn(i)
		if mw == nil {
			continue
		}
		err := w.L.CallByParam(lua.P{Fn: mw, NRet: 1, Protect: true}, reqTable)
		if err != nil {
			w.L.SetTop(0)
			return HTTPResponse{Status: 500, Body: err.Error()}, err, true
		}
		ret := w.L.Get(1)
		w.L.SetTop(0)
		if ret != nil && ret != lua.LNil {
			return normalizeHTTPResponse(decodeHTTPResponse(w.L, ret)), nil, true
		}
	}

	// Route handler.
	handler := w.router.RouteHandler(idx)
	if handler == nil {
		return HTTPResponse{
			Status:  500,
			Headers: map[string]string{},
			Body:    "route handler not found",
		}, fmt.Errorf("route handler not found"), true
	}
	err := w.L.CallByParam(lua.P{Fn: handler, NRet: 1, Protect: true}, reqTable)
	if err != nil {
		w.L.SetTop(0)
		return HTTPResponse{Status: 500, Body: err.Error()}, err, true
	}
	ret := w.L.Get(1)
	w.L.SetTop(0)
	resp := normalizeHTTPResponse(decodeHTTPResponse(w.L, ret))
	if resp.Status == 0 {
		resp.Status = 200
	}
	if resp.Headers == nil {
		resp.Headers = map[string]string{}
	}
	return resp, nil, true
}

// normalizeHTTPResponse fills default headers for a decoded response.
func normalizeHTTPResponse(resp HTTPResponse) HTTPResponse {
	if resp.Headers == nil {
		resp.Headers = map[string]string{}
	}
	return resp
}

// queryValuesTable converts parsed URL query values to a Lua table. Keys with a
// single value map to a plain string; repeated keys map to a list of strings.
func queryValuesTable(L *lua.LState, values map[string][]string) *lua.LTable {
	tbl := L.NewTable()
	for k, vals := range values {
		switch len(vals) {
		case 0:
			tbl.RawSetString(k, lua.LString(""))
		case 1:
			tbl.RawSetString(k, lua.LString(vals[0]))
		default:
			list := L.NewTable()
			for i, v := range vals {
				list.RawSetInt(i+1, lua.LString(v))
			}
			tbl.RawSetString(k, list)
		}
	}
	return tbl
}

// decodeHTTPResponse converts a handle_http return value to an HTTPResponse
// using the §2.5 response forms:
//
//	nil              → 200, empty body
//	"plain text"     → 200, text/plain
//	{json={...}}     → 200, JSON-encoded body, application/json
//	{status=n}       → n, empty body
//	{headers=…, body=…} → full form (body wins over json)
func decodeHTTPResponse(L *lua.LState, ret lua.LValue) HTTPResponse {
	var resp HTTPResponse
	if ret == nil || ret == lua.LNil {
		return HTTPResponse{Status: 200}
	}
	switch v := ret.(type) {
	case lua.LString:
		return HTTPResponse{
			Status:  200,
			Headers: map[string]string{"content-type": "text/plain"},
			Body:    string(v),
		}
	case lua.LNumber:
		return HTTPResponse{
			Status: int(v),
			Body:   "",
		}
	case *lua.LTable:
		if st := v.RawGetString("status"); st != lua.LNil {
			if num, ok := st.(lua.LNumber); ok {
				resp.Status = int(num)
			}
		}
		if h := v.RawGetString("headers"); h != lua.LNil {
			if ht, ok := h.(*lua.LTable); ok {
				resp.Headers = map[string]string{}
				ht.ForEach(func(k, vv lua.LValue) {
					resp.Headers[k.String()] = vv.String()
				})
			}
		}
		if b := v.RawGetString("body"); b != lua.LNil {
			resp.Body = b.String()
		} else if j := v.RawGetString("json"); j != lua.LNil {
			jsonBody, err := bindings.JSONStringifyLua(L, j, kNULLValue(L))
			if err != nil {
				resp.Status = 500
				resp.Body = err.Error()
				return resp
			}
			resp.Body = jsonBody
			if resp.Headers == nil {
				resp.Headers = map[string]string{}
			}
			if _, ok := resp.Headers["content-type"]; !ok {
				resp.Headers["content-type"] = "application/json"
			}
		}
	}
	return resp
}

// kNULLValue returns the K.NULL sentinel table for an LState (lua.LNil when the
// K globals are not installed).
func kNULLValue(L *lua.LState) lua.LValue {
	if k, ok := L.GetGlobal("K").(*lua.LTable); ok {
		return k.RawGetString("NULL")
	}
	return lua.LNil
}

// WSMessage describes a WebSocket event delivered to handle_ws(msg).
type WSMessage struct {
	Type     string // "open" | "text" | "binary" | "ping" | "pong" | "close"
	Data     string
	ClientID string
}

// TCPMessage describes a TCP event delivered to handle_tcp(msg).
type TCPMessage struct {
	Type     string // "open" | "text" | "close"
	Data     string
	ClientID string
}

// CallWS delivers a WebSocket message to handle_ws(msg). The whole invocation
// runs under the worker's slot so concurrent WS/TCP/HTTP dispatch never shares
// the LState; a string return value is echoed back to the same client.
func (w *Worker) CallWS(msg WSMessage, ws *WSConn) {
	if w.handlers.HandleWS == nil {
		return
	}
	w.acquireSlot()
	defer w.freeSlot()

	msgTable := msgLuaTable(w.L, msg.Type, msg.Data, msg.ClientID)

	co, cancel := w.L.NewThread()
	st, err, ret := w.L.Resume(co, w.handlers.HandleWS, msgTable)

	// cancel() may be nil (NewThread returns nil when the state has no
	// context) and may panic when called on a finished coroutine (gopher-lua
	// v1.1.2 bug); only cancel suspended/errored threads, and guard the nil.
	if cancel != nil && st != lua.ResumeOK {
		cancel()
	}
	if st == lua.ResumeError {
		if w.logger != nil {
			w.logger.Errorf("handle_ws error: %v", err)
		}
		w.notifyError(err)
		return
	}

	// Return value → send back to this client (spec §2.4 "serialize return
	// value → WS message"). Non-blocking: w.mu is held.
	if st == lua.ResumeOK && ws != nil && len(ret) > 0 && ret[0] != lua.LNil {
		select {
		case ws.sendCh <- []byte(ret[0].String()):
		default:
		}
	}
}

// CallTCP delivers a TCP event to handle_tcp(msg). Runs under the worker's
// slot; a string return value is echoed back to the same connection.
func (w *Worker) CallTCP(msg TCPMessage, tcp *TCPConn) {
	if w.handlers.HandleTCP == nil {
		return
	}
	w.acquireSlot()
	defer w.freeSlot()

	msgTable := msgLuaTable(w.L, msg.Type, msg.Data, msg.ClientID)

	co, cancel := w.L.NewThread()
	st, err, ret := w.L.Resume(co, w.handlers.HandleTCP, msgTable)

	if cancel != nil && st != lua.ResumeOK {
		cancel()
	}
	if st == lua.ResumeError {
		if w.logger != nil {
			w.logger.Errorf("handle_tcp error: %v", err)
		}
		w.notifyError(err)
		return
	}

	if st == lua.ResumeOK && tcp != nil && len(ret) > 0 && ret[0] != lua.LNil {
		select {
		case tcp.sendCh <- []byte(ret[0].String()):
		default:
		}
	}
}

// CallMQTT delivers an inbound MQTT message to the worker's registered
// k.mqtt_on handlers (Phase 2). Runs under the slot; a handler error is
// reported via the error hook without crashing the worker.
func (w *Worker) CallMQTT(msg bindings.MQTTMessage) {
	if w.env == nil {
		return
	}
	w.acquireSlot()
	defer w.freeSlot()
	w.env.DispatchMQTT(w.L, msg)
}

// CallInit runs the optional init(config) callback once at startup. Runs under
// the worker's slot; a handler error is returned so startup can abort.
func (w *Worker) CallInit(cfg Config) error {
	if w.handlers.Init == nil {
		return nil
	}

	w.acquireSlot()
	defer w.freeSlot()

	configTable := w.L.NewTable()
	configTable.RawSetString("host", lua.LString(cfg.Host))
	configTable.RawSetString("port", lua.LNumber(cfg.Port))
	configTable.RawSetString("workers", lua.LNumber(cfg.Workers))
	configTable.RawSetString("mode", lua.LString(cfg.Mode))

	co, cancel := w.L.NewThread()
	st, err, _ := w.L.Resume(co, w.handlers.Init, configTable)

	if cancel != nil && st != lua.ResumeOK {
		cancel()
	}
	if st == lua.ResumeError {
		w.notifyError(err)
		return fmt.Errorf("init error: %w", err)
	}
	return nil
}

// CallShutdown runs the optional shutdown() callback once before exit. Runs
// under the worker's slot; errors are logged, not returned.
func (w *Worker) CallShutdown() {
	if w.handlers.Shutdown == nil {
		return
	}

	w.acquireSlot()
	defer w.freeSlot()

	co, cancel := w.L.NewThread()
	st, err, _ := w.L.Resume(co, w.handlers.Shutdown)

	if cancel != nil && st != lua.ResumeOK {
		cancel()
	}
	if st == lua.ResumeError {
		if w.logger != nil {
			w.logger.Errorf("shutdown error: %v", err)
		}
		w.notifyError(err)
	}
}

// notifyError fires the Kalipso k.on_error hook for a genuine Lua error that
// aborted a serve-mode handler (http/ws/tcp/init/shutdown). Cleanup still
// proceeds (these frames run under w.mu); this lets the hook observe the error.
func (w *Worker) notifyError(err error) {
	if w.env != nil {
		w.env.FireError(w.L, bindings.KErrorGeneric, err.Error())
	}
}

// msgLuaTable builds the message table passed to handle_ws/handle_tcp.
func msgLuaTable(L *lua.LState, typ, data, clientID string) *lua.LTable {
	tbl := L.NewTable()
	tbl.RawSetString("type", lua.LString(typ))
	tbl.RawSetString("data", lua.LString(data))
	tbl.RawSetString("client_id", lua.LString(clientID))
	return tbl
}

// Close closes the worker's Lua state. Safe to call multiple times.
func (w *Worker) Close() {
	w.closeOnce.Do(func() { w.L.Close() })
}

// retire marks the worker as superseded by a hot reload; its Lua state is
// released once its last active lease ends.
func (w *Worker) retire() {
	w.retired.Store(true)
	if w.refs.Load() == 0 {
		w.Close()
	}
}

// release drops one lease. A retired worker's Lua state is closed when the
// final lease ends.
func (w *Worker) release() {
	if w.refs.Add(-1) == 0 && w.retired.Load() {
		w.Close()
	}
}

// Logger interface for worker logging.
type Logger interface {
	Errorf(format string, args ...interface{})
	Printf(format string, args ...interface{})
	Warnf(format string, args ...interface{})
	Tracef(format string, args ...interface{})
}

// HTTPRequest represents an incoming HTTP request.
type HTTPRequest struct {
	Method      string
	Path        string
	Query       string // raw query string (goes to req.query_raw)
	QueryValues map[string][]string
	Headers     map[string]string
	Body        string
	RemoteAddr  string
	TLS         bool
}

// HTTPResponse represents an HTTP response.
type HTTPResponse struct {
	Status  int
	Headers map[string]string
	Body    string
}
