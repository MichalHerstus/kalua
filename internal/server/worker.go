package server

import (
	"context"
	"fmt"
	"sync"

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
}

// WorkerHandlers holds the Lua callback functions for serve mode.
type WorkerHandlers struct {
	HandleHTTP *lua.LFunction
	HandleWS   *lua.LFunction
	HandleTCP  *lua.LFunction
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

	var httpHandler, wsHandler, tcpHandler *lua.LFunction
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
		},
		logger: logger,
	}, nil
}

// CallHTTP calls the handle_http callback with request data.
// Returns response status, headers, body, and error.
func (w *Worker) CallHTTP(ctx context.Context, req HTTPRequest) (HTTPResponse, error) {
	w.mu.Lock()
	if w.busy {
		w.mu.Unlock()
		return HTTPResponse{Status: 503, Body: "worker busy"}, fmt.Errorf("worker busy")
	}
	w.busy = true
	w.mu.Unlock()

	defer func() {
		w.mu.Lock()
		w.busy = false
		w.mu.Unlock()
	}()

	if w.handlers.HandleHTTP == nil {
		return HTTPResponse{Status: 404, Body: "no handle_http callback"}, nil
	}

	// Push request as table
	reqTable := w.L.NewTable()
	reqTable.RawSetString("method", lua.LString(req.Method))
	reqTable.RawSetString("path", lua.LString(req.Path))
	reqTable.RawSetString("query", lua.LString(req.Query))
	reqTable.RawSetString("body", lua.LString(req.Body))

	headersTable := w.L.NewTable()
	for k, v := range req.Headers {
		headersTable.RawSetString(k, lua.LString(v))
	}
	reqTable.RawSetString("headers", headersTable)

	// Call handle_http(req) -> (status, headers, body)
	fn := w.handlers.HandleHTTP
	err := w.L.CallByParam(lua.P{
		Fn:      fn,
		NRet:    3,
		Protect: true,
	}, reqTable)
	if err != nil {
		return HTTPResponse{Status: 500, Body: err.Error()}, err
	}

	// Get return values from main state
	var resp HTTPResponse
	top := w.L.GetTop()
	if top >= 1 {
		v := w.L.Get(1)
		if v != lua.LNil {
			if num, ok := v.(lua.LNumber); ok {
				resp.Status = int(num)
			}
		}
	}
	if top >= 2 {
		v := w.L.Get(2)
		if v != lua.LNil {
			if tbl, ok := v.(*lua.LTable); ok {
				resp.Headers = make(map[string]string)
				tbl.ForEach(func(k, v lua.LValue) {
					resp.Headers[k.String()] = v.String()
				})
			}
		}
	}
	if top >= 3 {
		v := w.L.Get(3)
		if v != lua.LNil {
			resp.Body = v.String()
		}
	}

	// Clear the stack for next call
	w.L.SetTop(0)

	if resp.Status == 0 {
		resp.Status = 200
	}
	return resp, nil
}

// CallWS calls the handle_ws callback for a new WebSocket connection.
func (w *Worker) CallWS(connID string, ws *WSConn) {
	if w.handlers.HandleWS == nil {
		return
	}

	co, cancel := w.L.NewThread()
	defer cancel()

	connTable := w.L.NewTable()
	connTable.RawSetString("id", lua.LString(connID))

	fn := w.handlers.HandleWS
	w.L.Resume(co, fn, connTable)
}

// CallTCP calls the handle_tcp callback for a new TCP connection.
func (w *Worker) CallTCP(connID string, tcp *TCPConn) {
	if w.handlers.HandleTCP == nil {
		return
	}

	co, cancel := w.L.NewThread()
	defer cancel()

	connTable := w.L.NewTable()
	connTable.RawSetString("id", lua.LString(connID))

	fn := w.handlers.HandleTCP
	w.L.Resume(co, fn, connTable)
}

// Close closes the worker's Lua state.
func (w *Worker) Close() {
	w.L.Close()
}

// Logger interface for worker logging.
type Logger interface {
	Errorf(format string, args ...interface{})
	Printf(format string, args ...interface{})
}

// HTTPRequest represents an incoming HTTP request.
type HTTPRequest struct {
	Method  string
	Path    string
	Query   string
	Headers map[string]string
	Body    string
}

// HTTPResponse represents an HTTP response.
type HTTPResponse struct {
	Status  int
	Headers map[string]string
	Body    string
}
