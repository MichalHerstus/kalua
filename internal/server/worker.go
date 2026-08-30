package server

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/yuin/gopher-lua"

	"kalua/internal/bindings"
	"kalua/internal/vm"
)

// Worker represents a single Lua VM worker in the serve pool.
type Worker struct {
	id       int
	L        *lua.LState
	app      *vm.App
	shared   *SharedState
	wsHub    *WSHub
	tcpHub   *TCPHub
	handlers WorkerHandlers
	mu       sync.Mutex
	busy     bool
	logger   Logger

	refs     atomic.Int32 // active leases (HTTP handles, WS/TCP connections)
	retired  atomic.Bool  // true once superseded by a hot reload
	closeOnce sync.Once
}

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

	// Setup serve-mode bindings (no UI bindings)
	bindings.SetupServe(L, app, opts, shared, wsHub, tcpHub, logger)
	bindings.SetupUIError(L)

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

	return &Worker{
		id:     id,
		L:      L,
		app:    app,
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
		logger: logger,
	}, nil
}

// CallHTTP calls the handle_http callback with request data.
// Returns response status, headers, body, and error.
func (w *Worker) CallHTTP(ctx context.Context, req HTTPRequest) (HTTPResponse, error) {
	// Hold w.mu for the whole invocation so concurrent WS/TCP dispatch can
	// never touch the LState while a handler is running. Concurrent HTTP
	// requests are rejected with 503 rather than queued.
	w.mu.Lock()
	if w.busy {
		w.mu.Unlock()
		return HTTPResponse{Status: 503, Body: "worker busy"}, fmt.Errorf("worker busy")
	}
	w.busy = true
	defer func() {
		w.busy = false
		w.mu.Unlock()
	}()

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
// runs under w.mu so concurrent WS/TCP/HTTP dispatch never shares the LState;
// a string return value is echoed back to the same client.
func (w *Worker) CallWS(msg WSMessage, ws *WSConn) {
	if w.handlers.HandleWS == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()

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

// CallTCP delivers a TCP event to handle_tcp(msg). Runs under w.mu; a string
// return value is echoed back to the same connection.
func (w *Worker) CallTCP(msg TCPMessage, tcp *TCPConn) {
	if w.handlers.HandleTCP == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()

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
		return
	}

	if st == lua.ResumeOK && tcp != nil && len(ret) > 0 && ret[0] != lua.LNil {
		select {
		case tcp.sendCh <- []byte(ret[0].String()):
		default:
		}
	}
}

// CallInit runs the optional init(config) callback once at startup. Runs under
// w.mu; a handler error is returned so startup can abort.
func (w *Worker) CallInit(cfg Config) error {
	if w.handlers.Init == nil {
		return nil
	}

	w.mu.Lock()
	defer w.mu.Unlock()

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
		return fmt.Errorf("init error: %w", err)
	}
	return nil
}

// CallShutdown runs the optional shutdown() callback once before exit. Runs
// under w.mu; errors are logged, not returned.
func (w *Worker) CallShutdown() {
	if w.handlers.Shutdown == nil {
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	co, cancel := w.L.NewThread()
	st, err, _ := w.L.Resume(co, w.handlers.Shutdown)

	if cancel != nil && st != lua.ResumeOK {
		cancel()
	}
	if st == lua.ResumeError && w.logger != nil {
		w.logger.Errorf("shutdown error: %v", err)
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
