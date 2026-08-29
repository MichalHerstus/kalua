// Package session implements the per-tab actor that owns an LState and serializes
// all Lua execution for that browser tab. It communicates with the browser via
// an inbox (WS events, timers, async completions) and an outbox (UI commands).
package session

import (
	"context"
	"fmt"
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
	inboxNone      inboxMsgType = iota
	inboxWSEvent                // event from browser (click, input, etc.)
	inboxTimer                  // timer fired
	inboxAsyncDone              // blocking operation completed (DB, HTTP, etc.)
	inboxClose                  // session teardown
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
	timer string      // timer ID
	data  interface{} // generic payload for async completions
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
	wg       sync.WaitGroup
	quitting bool

	// Form stack for modal Show Form semantics (D52).
	// Top of slice is the visible form.
	formStack []string

	// Timers managed by this session
	timers map[string]*time.Timer

	// Async operations - suspended coroutines waiting for completion
	asyncOps map[string]*asyncOp
	asyncMu  sync.Mutex
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
		timers: make(map[string]*time.Timer),
	}

	// Set session on env for msgbox, clipboard, etc.
	env.Sess = s

	// Start the actor goroutine
	s.wg.Add(1)
	go s.run(ctx, mainLFn, logger)

	return s, nil
}

// run is the actor's main loop. It starts main() and then drains the inbox.
func (s *Session) run(ctx context.Context, mainFn *lua.LFunction, logger Logger) {
	defer s.wg.Done()

	// Start main() in a coroutine
	if err := s.app.Run(mainFn); err != nil {
		// Error during startup
		logger.Errorf("main error: %v", err)
		s.outbox <- common.OutboxMsg{Type: "error", Msg: err.Error()}
		s.outbox <- common.OutboxMsg{Type: "quit"}
		return
	}

	// Main completed normally
	s.outbox <- common.OutboxMsg{Type: "quit"}

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
	case inboxClose:
		s.teardown(logger)
	}
}

// handleWSEvent dispatches a browser event to the appropriate Lua handler.
func (s *Session) handleWSEvent(msg inboxMsg, logger Logger) {
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
	defer cancel()

	st, err, _ := s.L.Resume(co, fn, msg.value)
	if st == lua.ResumeError {
		logger.Errorf("handler error: %v", err)
		s.outbox <- common.OutboxMsg{Type: "error", Msg: err.Error(), Stack: getStack(s.L)}
		return
	}

	// Flush outbox after handler
	s.flushOutbox()
}

// handleTimer processes a timer event.
func (s *Session) handleTimer(timerID string, logger Logger) {
	// Timer fires a form event: form "timer", event "timer(id)"
	// This is handled by the form's timer handler
	s.handleWSEvent(inboxMsg{
		typ:   inboxWSEvent,
		form:  "timer",
		ctrl:  "",
		event: "timer(" + timerID + ")",
		value: lua.LString(timerID),
	}, logger)
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

	// Clean up the stored op now that the coroutine is resumed.
	if op.cancel != nil {
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
		Text: text,
		Kind: kind,
	})

	// The coroutine will be resumed when HandleMsgboxChoice is called
	// We need to yield here - the actual resume happens via inbox
	return ""
}

// HandleMsgboxChoice resumes the coroutine waiting for a msgbox response.
func (s *Session) HandleMsgboxChoice(msgboxID, choice string) {
	s.asyncMu.Lock()
	op, exists := s.asyncOps[msgboxID]
	if exists {
		delete(s.asyncOps, msgboxID)
	}
	s.asyncMu.Unlock()

	if !exists {
		return
	}

	// Resume the coroutine with the choice
	st, err, _ := s.L.Resume(op.co, nil, lua.LString(choice))
	if st == lua.ResumeError {
		if s.env != nil && s.env.Logger != nil {
			s.env.Logger.Errorf("msgbox resume error: %v", err)
		}
		return
	}

	// Clean up the stored op now that the coroutine is resumed.
	if op.cancel != nil {
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

// PostTimer posts a timer event to the session inbox.
func (s *Session) PostTimer(timerID string) {
	s.inbox <- inboxMsg{typ: inboxTimer, timer: timerID}
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

// Close closes the session and releases resources.
func (s *Session) Close() error {
	s.cancel()
	close(s.inbox)
	s.wg.Wait()

	// Stop all timers
	for _, t := range s.timers {
		t.Stop()
	}

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
