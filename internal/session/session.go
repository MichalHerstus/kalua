// Package session implements the per-tab actor that owns an LState and serializes
// all Lua execution for that browser tab. It communicates with the browser via
// an inbox (WS events, timers, async completions) and an outbox (UI commands).
package session

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"strings"
	"sync"
	"time"

	"github.com/yuin/gopher-lua"

	"kalua/internal/bindings"
	"kalua/internal/common"
	"kalua/internal/vm"
)

// Inbox message types — all events that can reach the actor goroutine.
type inboxMsgType int

const (
	inboxNone          inboxMsgType = iota
	inboxWSEvent                    // event from browser (click, input, etc.)
	inboxTimer                      // timer fired
	inboxAsyncDone                  // blocking operation completed (DB, HTTP, etc.)
	inboxMsgboxChoice               // user answered a k.msgbox
	inboxClipboardResp              // browser clipboard_get value
	inboxQuery                      // external read of Lua state (tests)
	inboxClose                      // session teardown
)

// asyncOp represents a suspended coroutine waiting for an async operation
type asyncOp struct {
	co     *lua.LState                               // coroutine to resume
	cancel func()                                    // cleanup function
	conv   func(*lua.LState, interface{}) lua.LValue // result converter (nil = default)
}

// inboxMsg is a typed message delivered to the session actor's inbox.
type inboxMsg struct {
	typ   inboxMsgType
	form  string      // form name (for form events)
	ctrl  string      // control name
	event string      // event name (click, input, etc.)
	value lua.LValue  // event value
	raw   interface{} // JSON-decoded event value (converted on the actor goroutine)
	timer string      // timer ID
	data  interface{} // generic payload for async completions

	// respID and resp identify a browser round-trip response (msgbox choice,
	// clipboard_get value) so the actor can resume the suspended coroutine.
	respID string
	resp   string

	// query is a unit of work to run on the actor goroutine (inboxQuery).
	query func(*lua.LState) lua.LValue
	reply chan lua.LValue
}

// Session is the per-tab actor. It owns its LState and processes events
// sequentially from its inbox.
type Session struct {
	id       string
	L        *lua.LState
	app      *vm.App
	env      *bindings.Env
	inbox    chan inboxMsg
	outbox   chan common.OutboxMsg
	cancel   context.CancelFunc
	done     chan struct{} // closed on Close; stop PostTimer from blocking
	wg       sync.WaitGroup
	quitting bool

	// Form stack for modal Show Form semantics (D52).
	// Top of slice is the visible form.
	formStack []string

	// Form show coroutines - suspended coroutines waiting for form close
	formCoros  map[string]*lua.LState
	formCoroMu sync.Mutex

	// Timers managed by this session
	timers map[string]*time.Timer

	// Async operations - suspended coroutines waiting for completion
	asyncOps map[string]*asyncOp
	asyncMu  sync.Mutex

	// Client info from the browser's client_info message (screen size/locale).
	clientMu     sync.RWMutex
	clientW      int
	clientH      int
	clientLocale string
}

// New creates and starts a new session actor.
func New(id string, scriptPath string, opts bindings.Options, logger Logger) (*Session, error) {
	L := vm.New()
	app := vm.NewApp(L)
	env := bindings.Setup(L, app, opts, nil, logger) // session set after creation

	// Load and compile the script
	chunkFn, err := vm.LoadFile(L, scriptPath)
	if err != nil {
		L.Close()
		return nil, err
	}

	// Execute chunk to define main()
	if err := L.CallByParam(lua.P{Fn: chunkFn, NRet: 0, Protect: true}); err != nil {
		L.Close()
		return nil, err
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

	ctx, cancel := context.WithCancel(context.Background())
	s := &Session{
		id:     id,
		L:      L,
		app:    app,
		env:    env,
		inbox:  make(chan inboxMsg, 64),
		outbox: make(chan common.OutboxMsg, 64),
		cancel: cancel,
		done:   make(chan struct{}),
		timers: make(map[string]*time.Timer),
		// Form show coroutines
		formCoros: make(map[string]*lua.LState),
		// Async operations - suspended coroutines waiting for completion
		asyncOps: make(map[string]*asyncOp),
	}

	// Set session on env for msgbox, clipboard, etc.
	env.Sess = s
	// Share the session with the App so bindings' sendOutbox (which routes
	// through e.App.Session()) reach the session outbox.
	app.SetSession(s)

	// Start the actor goroutine
	s.wg.Add(1)
	go s.run(ctx, mainLFn, logger)

	return s, nil
}

// run is the actor's main loop. It starts main() and then drains the inbox.
func (s *Session) run(ctx context.Context, mainFn *lua.LFunction, logger Logger) {
	defer s.wg.Done()

	// Start main() in a coroutine
	err := s.app.Run(mainFn)
	if err != nil {
		if err == vm.ErrSuspended {
			// Main suspended on form.show - enter actor loop to process events
			// The main coroutine will be resumed when the form is closed
		} else {
			// Error during startup
			logger.Errorf("main error: %v", err)
			s.outbox <- common.OutboxMsg{Type: "error", Msg: err.Error()}
			s.outbox <- common.OutboxMsg{Type: "quit"}
			return
		}
	} else {
		// Main completed normally - send quit but continue actor loop for timers/async
		s.outbox <- common.OutboxMsg{Type: "quit"}
	}

	// Actor loop: drain inbox until closed
	for {
		select {
		case msg, ok := <-s.inbox:
			if !ok {
				return // inbox closed, session ending
			}
			s.handleInbox(msg, logger)
		case <-ctx.Done():
			return
		}
	}
}

// handleInbox processes a single inbox message.
func (s *Session) handleInbox(msg inboxMsg, logger Logger) {
	switch msg.typ {
	case inboxWSEvent:
		s.handleWSEvent(msg, logger)
	case inboxTimer:
		s.handleTimer(msg.timer, logger)
	case inboxAsyncDone:
		s.handleAsyncDone(msg.data, logger)
	case inboxMsgboxChoice:
		s.resumeAsyncResp(msg.respID, lua.LString(msg.resp), "msgbox", logger)
	case inboxClipboardResp:
		s.resumeAsyncResp(msg.respID, lua.LString(msg.resp), "clipboard", logger)
	case inboxQuery:
		if msg.query != nil && msg.reply != nil {
			msg.reply <- msg.query(s.L)
		}
	case inboxClose:
		s.teardown(logger)
		s.teardown(logger)
	}
}

// handleWSEvent dispatches a browser event to the appropriate Lua handler.
func (s *Session) handleWSEvent(msg inboxMsg, logger Logger) {
	// Convert a raw JSON-decoded value to a Lua value on the actor goroutine.
	// This keeps all LState access serialized (s.L is not goroutine-safe).
	value := msg.value
	if msg.raw != nil {
		value = s.toLuaValue(msg.raw)
	}

	// Update control value in form definition from browser event
	if value != lua.LNil {
		s.updateControlValue(msg.form, msg.ctrl, value)
	}

	// Look up the handler in the form's event table
	formTbl := s.L.GetGlobal(msg.form)
	if formTbl == lua.LNil {
		logger.Warnf("form %s not found for event %s", msg.form, msg.event)
		return
	}
	tbl, ok := formTbl.(*lua.LTable)
	if !ok {
		return
	}

	// Get the handler: form.handlers[ctrl][event]
	handlers := tbl.RawGetString("handlers")
	if handlers == lua.LNil {
		return
	}
	handlersTbl, ok := handlers.(*lua.LTable)
	if !ok {
		return
	}

	ctrlHandlers := handlersTbl.RawGetString(msg.ctrl)
	if ctrlHandlers == lua.LNil {
		return
	}
	ctrlTbl, ok := ctrlHandlers.(*lua.LTable)
	if !ok {
		return
	}

	handler := ctrlTbl.RawGetString(msg.event)
	if handler == lua.LNil {
		return
	}
	fn, ok := handler.(*lua.LFunction)
	if !ok {
		return
	}

	// Run the handler in a coroutine
	co, cancel := s.L.NewThread()

	st, err, _ := s.L.Resume(co, fn, value)
	if st == lua.ResumeError {
		// cancel() may be nil (NewThread returns nil when the state has no
		// context) and may panic if called on a finished coroutine; guard both.
		if cancel != nil {
			cancel()
		}
		logger.Errorf("handler error: %v\n%s", err, getStack(s.L))
		s.outbox <- common.OutboxMsg{Type: "error", Msg: err.Error(), Stack: getStack(s.L)}
		return
	}

	// Flush outbox after handler
	s.flushOutbox()
}

// handleTimer processes a timer event. It looks up a Lua global function named
// after the timer id (e.g. `function mytimer()`), or — when no such global
// exists — a form handler registered under the special "timer" form
// (k.form.on("timer", id, fn)). Running inside the actor keeps every timer
// handler serialized with the rest of the session's events.
func (s *Session) handleTimer(timerID string, logger Logger) {
	fn := s.L.GetGlobal(timerID)
	if fn != lua.LNil {
		if lfn, ok := fn.(*lua.LFunction); ok {
			s.runTimerHandler(lfn, lua.LString(timerID), logger)
			return
		}
	}
	// Fall back to the form handler table path.
	s.handleWSEvent(inboxMsg{
		typ:   inboxWSEvent,
		form:  "timer",
		ctrl:  "",
		event: "timer(" + timerID + ")",
		value: lua.LString(timerID),
	}, logger)
}

// runTimerHandler calls a timer handler function in a fresh coroutine.
func (s *Session) runTimerHandler(fn *lua.LFunction, val lua.LValue, logger Logger) {
	co, cancel := s.L.NewThread()
	defer func() {
		if r := recover(); r != nil {
			// defensive: never let one bad timer handler kill the session
			logger.Errorf("timer handler panic: %v", r)
			s.SendOutbox(common.OutboxMsg{Type: "error", Msg: fmt.Sprintf("timer panic: %v", r)})
		}
	}()
	st, err, _ := s.L.Resume(co, fn, val)
	if st == lua.ResumeError {
		// The coroutine is still suspended/errored; cancel it to release
		// resources. On a normal (ResumeOK) completion, cancel() may panic in
		// gopher-lua v1.1.2, so it is only called on this path. It may also be
		// nil when the state has no context.
		if cancel != nil {
			cancel()
		}
		logger.Errorf("timer handler error: %v", err)
		s.SendOutbox(common.OutboxMsg{Type: "error", Msg: err.Error(), Stack: getStack(s.L)})
		return
	}
	s.flushOutbox()
}

// handleAsyncDone resumes a suspended coroutine with async results.
func (s *Session) handleAsyncDone(data interface{}, logger Logger) {
	opData, ok := data.(map[string]interface{})
	if !ok {
		logger.Warnf("invalid async done data: %v", data)
		return
	}

	opID, _ := opData["op_id"].(string)
	result := opData["result"]
	err := opData["error"]

	s.asyncMu.Lock()
	op, exists := s.asyncOps[opID]
	if exists {
		delete(s.asyncOps, opID)
	}
	s.asyncMu.Unlock()

	if !exists {
		logger.Warnf("async operation not found: %s", opID)
		return
	}

	// Resume the coroutine with the result
	conv := common.DefaultConv
	if op.conv != nil {
		conv = op.conv
	}
	var resumeVal lua.LValue
	if err != nil {
		resumeVal = lua.LString(err.(string))
	} else if result != nil {
		resumeVal = conv(s.L, result)
	} else {
		resumeVal = lua.LNil
	}

	st, errResume, _ := s.L.Resume(op.co, nil, resumeVal)
	if st == lua.ResumeError {
		logger.Errorf("async resume error: %v", errResume)
		s.outbox <- common.OutboxMsg{Type: "error", Msg: errResume.Error(), Stack: getStack(s.L)}
		return
	}

	// Clean up the stored op now that the coroutine is resumed. On a normal
	// completion (ResumeOK) the coroutine is finished; calling cancel() on a
	// finished coroutine may panic in gopher-lua v1.1.2, so only cancel when it
	// is still suspended (ResumeYield, waiting for the next async completion).
	if st != lua.ResumeOK && op.cancel != nil {
		op.cancel()
	}

	// Flush outbox after handler
	s.flushOutbox()
}

// RunAsync executes a function in a worker goroutine and resumes the given coroutine when done.
// The coroutine should be in a yielded state (waiting for the result). conv converts the
// result to a Lua value on the session's main thread; nil means common.DefaultConv.
func (s *Session) RunAsync(co *lua.LState, cancel func(), fn func() (interface{}, error), conv func(*lua.LState, interface{}) lua.LValue) {
	opID := fmt.Sprintf("async_%d", time.Now().UnixNano())

	// Store the suspended coroutine
	s.asyncMu.Lock()
	s.asyncOps[opID] = &asyncOp{co: co, cancel: cancel, conv: conv}
	s.asyncMu.Unlock()

	// Run in worker goroutine
	s.wg.Add(1)
	go func(opID string) {
		defer s.wg.Done()

		result, err := fn()

		var errStr string
		if err != nil {
			errStr = err.Error()
		}

		// Post completion to inbox
		s.inbox <- inboxMsg{
			typ: inboxAsyncDone,
			data: map[string]interface{}{
				"op_id":  opID,
				"result": result,
				"error":  errStr,
			},
		}
	}(opID)
}

// ShowMsgbox shows a message box in the browser and suspends the coroutine until user responds.
// Returns the user's choice ("ok", "yes", "no", "cancel", etc.) or empty string on error.
func (s *Session) ShowMsgbox(co *lua.LState, cancel func(), text, kind string) string {
	msgboxID := fmt.Sprintf("msgbox_%d", time.Now().UnixNano())

	// Store the suspended coroutine
	s.asyncMu.Lock()
	s.asyncOps[msgboxID] = &asyncOp{co: co, cancel: cancel}
	s.asyncMu.Unlock()

	// Send msgbox to browser
	s.SendOutbox(common.OutboxMsg{
		Type: "msgbox",
		ID:   msgboxID,
		Kind: kind,
		HTML: renderMsgboxHTML(msgboxID, text, kind),
	})

	// The coroutine will be resumed when HandleMsgboxChoice is called
	// We need to yield here - the actual resume happens via inbox
	return ""
}

// renderMsgboxHTML builds the msgbox body: escaped text plus the buttons
// appropriate for the kind. Each button carries data-k-msgbox-id and
// data-k-choice so the JS client can answer with msgbox_choice.
func renderMsgboxHTML(id, text, kind string) string {
	choices := []string{"ok"}
	switch kind {
	case "ok-cancel":
		choices = []string{"ok", "cancel"}
	case "yes-no":
		choices = []string{"yes", "no"}
	case "info", "warn", "error", "":
		choices = []string{"ok"}
	}

	var sb strings.Builder
	sb.WriteString(`<p class="msgbox-text">`)
	sb.WriteString(html.EscapeString(text))
	sb.WriteString(`</p>`)
	sb.WriteString(`<div class="msgbox-buttons">`)
	for _, choice := range choices {
		label := strings.ToUpper(choice)
		sb.WriteString(`<button type="button" class="kalua-button" data-k-msgbox-id="`)
		sb.WriteString(html.EscapeString(id))
		sb.WriteString(`" data-k-choice="`)
		sb.WriteString(html.EscapeString(choice))
		sb.WriteString(`">`)
		sb.WriteString(html.EscapeString(label))
		sb.WriteString(`</button>`)
	}
	sb.WriteString(`</div>`)
	return sb.String()
}

// HandleMsgboxChoice is called from the web bridge goroutine when the browser
// answers a k.msgbox modal. It forwards the answer to the actor inbox so the
// resume happens on the actor goroutine (s.L is not thread-safe).
func (s *Session) HandleMsgboxChoice(msgboxID, choice string) {
	select {
	case s.inbox <- inboxMsg{typ: inboxMsgboxChoice, respID: msgboxID, resp: choice}:
	case <-s.done:
		// session closed; drop
	}
}

// RequestClipboardGet asks the browser for clipboard text and suspends the
// current coroutine until the value is delivered via PostClipboardResp.
func (s *Session) RequestClipboardGet(co *lua.LState, cancel func()) {
	clipID := fmt.Sprintf("clipboard_%d", time.Now().UnixNano())

	// Store the suspended coroutine
	s.asyncMu.Lock()
	s.asyncOps[clipID] = &asyncOp{co: co, cancel: cancel}
	s.asyncMu.Unlock()

	// Send clipboard read request to browser
	s.SendOutbox(common.OutboxMsg{
		Type: "clipboard_get",
		ID:   clipID,
	})
}

// PostClipboardResp is called from the web bridge goroutine when the browser
// delivers clipboard text. It forwards the value to the actor inbox so the
// resume happens on the actor goroutine (s.L is not thread-safe).
func (s *Session) PostClipboardResp(clipID, value string) {
	select {
	case s.inbox <- inboxMsg{typ: inboxClipboardResp, respID: clipID, resp: value}:
	case <-s.done:
		// session closed; drop
	}
}

// resumeAsyncResp resumes a coroutine suspended by ShowMsgbox/RequestClipboardGet
// with the browser's answer. Must run on the actor goroutine.
func (s *Session) resumeAsyncResp(respID string, val lua.LValue, kind string, logger Logger) {
	s.asyncMu.Lock()
	op, exists := s.asyncOps[respID]
	if exists {
		delete(s.asyncOps, respID)
	}
	s.asyncMu.Unlock()

	if !exists {
		logger.Warnf("%s response for unknown op: %s", kind, respID)
		return
	}

	// Resume the coroutine with the value.
	st, err, _ := s.L.Resume(op.co, nil, val)
	if st == lua.ResumeError {
		if s.env != nil && s.env.Logger != nil {
			s.env.Logger.Errorf("%s resume error: %v", kind, err)
		}
		return
	}

	// Clean up the stored op now that the coroutine is resumed. Only cancel a
	// still-suspended coroutine; calling cancel() on a finished one may panic
	// in gopher-lua v1.1.2.
	if st != lua.ResumeOK && op.cancel != nil {
		op.cancel()
	}

	// Flush outbox after handler
	s.flushOutbox()
}

// flushOutbox drains the outbox and sends to the browser (handled by caller).
func (s *Session) flushOutbox() {
	// The caller (WS handler) reads from s.Outbox()
}

// Outbox returns the outbox channel for the WS bridge to read.
func (s *Session) Outbox() <-chan common.OutboxMsg {
	return s.outbox
}

// SendOutbox sends a message to the session outbox.
func (s *Session) SendOutbox(msg common.OutboxMsg) {
	select {
	case s.outbox <- msg:
	default:
		// Channel full, drop message (should not happen with buffered channel)
	}
}

// Inbox returns the inbox channel for the WS bridge to write.
func (s *Session) Inbox() chan<- inboxMsg {
	return s.inbox
}

// PostEvent posts a browser event to the session inbox.
func (s *Session) PostEvent(form, ctrl, event string, value lua.LValue) {
	s.inbox <- inboxMsg{typ: inboxWSEvent, form: form, ctrl: ctrl, event: event, value: value}
}

// PostEventAny posts a browser event carrying an arbitrary JSON-decoded value
// (map[string]interface{}, []interface{}, string, bool, float64, nil). The
// value is converted to a Lua value inside the actor goroutine to keep all
// LState access serialized.
func (s *Session) PostEventAny(form, ctrl, event string, value interface{}) {
	select {
	case s.inbox <- inboxMsg{typ: inboxWSEvent, form: form, ctrl: ctrl, event: event, raw: value}:
	case <-s.done:
		// session closed; drop
	}
}

// GetGlobal reads a Lua global on the actor goroutine and returns its value.
// Safe to call from any goroutine (tests, web bridge); the read is serialized
// through the inbox so it cannot race the actor's Lua state access.
func (s *Session) GetGlobal(name string) lua.LValue {
	reply := make(chan lua.LValue, 1)
	select {
	case s.inbox <- inboxMsg{typ: inboxQuery, query: func(L *lua.LState) lua.LValue {
		return L.GetGlobal(name)
	}, reply: reply}:
	case <-s.done:
		return lua.LNil
	}
	return <-reply
}

// PostTimer posts a timer event to the session inbox.
func (s *Session) PostTimer(timerID string) {
	select {
	case s.inbox <- inboxMsg{typ: inboxTimer, timer: timerID}:
	case <-s.done:
		// session closed; drop
	}
}

// StartTimer starts a session-scoped timer.
func (s *Session) StartTimer(id string, ms int, repeats bool) {
	if existing, ok := s.timers[id]; ok {
		existing.Stop()
	}
	t := time.AfterFunc(time.Duration(ms)*time.Millisecond, func() {
		s.PostTimer(id)
		if repeats {
			// Reschedule for repeats (simplified)
		}
	})
	s.timers[id] = t
}

// StopTimer stops a session-scoped timer.
func (s *Session) StopTimer(id string) {
	if t, ok := s.timers[id]; ok {
		t.Stop()
		delete(s.timers, id)
	}
}

// PushForm pushes a form onto the stack (for k.form.show).
func (s *Session) PushForm(name string) {
	s.formStack = append(s.formStack, name)
}

// PopForm pops the top form from the stack.
func (s *Session) PopForm() string {
	if len(s.formStack) == 0 {
		return ""
	}
	name := s.formStack[len(s.formStack)-1]
	s.formStack = s.formStack[:len(s.formStack)-1]
	return name
}

// TopForm returns the current top form.
func (s *Session) TopForm() string {
	if len(s.formStack) == 0 {
		return ""
	}
	return s.formStack[len(s.formStack)-1]
}

// toLuaValue converts a JSON-decoded Go value (from a browser WS message) into
// a Lua value, creating tables recursively. Must run on the actor goroutine so
// s.L is only touched serially.
func (s *Session) toLuaValue(v interface{}) lua.LValue {
	switch val := v.(type) {
	case string:
		return lua.LString(val)
	case float64:
		return lua.LNumber(val)
	case bool:
		return lua.LBool(val)
	case nil:
		return lua.LNil
	case map[string]interface{}:
		tbl := s.L.NewTable()
		for k, item := range val {
			tbl.RawSetString(k, s.toLuaValue(item))
		}
		return tbl
	case []interface{}:
		tbl := s.L.NewTable()
		for i, item := range val {
			tbl.RawSetInt(i+1, s.toLuaValue(item))
		}
		return tbl
	case json.Number:
		if n, err := val.Int64(); err == nil {
			return lua.LNumber(n)
		}
		if f, err := val.Float64(); err == nil {
			return lua.LNumber(f)
		}
		return lua.LString(val.String())
	default:
		return lua.LString(fmt.Sprintf("%v", v))
	}
}

// updateControlValue updates a control's value in the form definition.
func (s *Session) updateControlValue(formName, ctrlName string, value lua.LValue) {
	// If value is a table with multiple control values, update all of them
	if valueTbl, ok := value.(*lua.LTable); ok {
		valueTbl.ForEach(func(k, v lua.LValue) {
			name := k.String()
			s.updateSingleControlValue(formName, name, v)
		})
		return
	}

	s.updateSingleControlValue(formName, ctrlName, value)
}

func (s *Session) updateSingleControlValue(formName, ctrlName string, value lua.LValue) {
	formTbl := s.L.GetGlobal(formName)
	if formTbl == lua.LNil {
		return
	}
	tbl, ok := formTbl.(*lua.LTable)
	if !ok {
		return
	}

	controls := tbl.RawGetString("controls")
	if controls == lua.LNil {
		return
	}
	controlsTbl, ok := controls.(*lua.LTable)
	if !ok {
		return
	}

	ctrl := controlsTbl.RawGetString(ctrlName)
	if ctrl == lua.LNil {
		return
	}
	ctrlTbl, ok := ctrl.(*lua.LTable)
	if !ok {
		return
	}

	ctrlTbl.RawSetString("value", value)
}

// StoreFormCoro stores the suspended coroutine for a form show operation.
func (s *Session) StoreFormCoro(name string, co *lua.LState) {
	s.formCoroMu.Lock()
	s.formCoros[name] = co
	s.formCoroMu.Unlock()
}

// ResumeFormCoro resumes the suspended coroutine for a form show operation.
func (s *Session) ResumeFormCoro(name string) bool {
	s.formCoroMu.Lock()
	co, exists := s.formCoros[name]
	if exists {
		delete(s.formCoros, name)
	}
	s.formCoroMu.Unlock()

	if !exists {
		return false
	}

	// Resume the coroutine with nil (form.show returns nil)
	st, err, _ := s.L.Resume(co, nil, lua.LNil)
	if st == lua.ResumeError {
		if s.env != nil && s.env.Logger != nil {
			s.env.Logger.Errorf("form show resume error: %v", err)
		}
		return false
	}

	// Clear the suspended form state in the App
	s.app.ResumeMain()

	return true
}

// SetClientInfo stores the browser viewport size and locale reported via the
// client_info WebSocket message. Called from the WS bridge goroutine, so it is
// guarded by clientMu; the actor reads it back via ClientInfo.
func (s *Session) SetClientInfo(w, h int, locale string) {
	s.clientMu.Lock()
	defer s.clientMu.Unlock()
	s.clientW = w
	s.clientH = h
	if locale != "" {
		s.clientLocale = locale
	}
}

// ClientInfo returns the stored viewport size and locale (0x0 / "" before the
// browser's first client_info message).
func (s *Session) ClientInfo() (w, h int, locale string) {
	s.clientMu.RLock()
	defer s.clientMu.RUnlock()
	return s.clientW, s.clientH, s.clientLocale
}

// Close closes the session and releases resources.
func (s *Session) Close() error {
	// Stop timers before tearing down so no timer callback touches s.L or the
	// inbox after we close them.
	for _, t := range s.timers {
		t.Stop()
	}
	s.cancel()
	close(s.done)
	close(s.inbox)
	s.wg.Wait()
	s.L.Close()
	return nil
}

// teardown performs session cleanup.
func (s *Session) teardown(logger Logger) {
	s.quitting = true
	// close_form cleanup for all forms on stack
	for len(s.formStack) > 0 {
		name := s.PopForm()
		// TODO: fire close_form events
		_ = name
	}
	s.outbox <- common.OutboxMsg{Type: "quit"}
}

// Logger interface for session logging.
type Logger interface {
	Printf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
	Warnf(format string, args ...interface{})
}

func getStack(L *lua.LState) string {
	dbg, ok := L.GetStack(1) // skip getStack frame
	if !ok {
		return ""
	}
	return fmt.Sprintf("%s:%d", dbg.Source, dbg.CurrentLine)
}
