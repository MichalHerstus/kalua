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
	inboxNone                   inboxMsgType = iota
	inboxWSEvent                             // event from browser (click, input, etc.)
	inboxTimer                               // timer fired
	inboxAsyncDone                           // blocking operation completed (DB, HTTP, etc.)
	inboxMsgboxChoice                        // user answered a k.msgbox
	inboxClipboardResp                       // browser clipboard_get value
	inboxFilePickerResp                      // browser file picker result (JSON-encoded files)
	inboxQuery                               // external read of Lua state (tests)
	inboxSleepDone                           // k.sleep completed
	inboxTabulatorDataResp                   // browser answered k.table.get_data
	inboxTabulatorSelectionResp              // browser answered k.table.get_selected_rows
	inboxTabulatorAjaxRequest                // browser asked for a remote page of rows
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
	respID     string
	resp       string
	selectRows []int

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
	verbose  bool
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

	// Sleep operations - suspended coroutines waiting for k.sleep
	sleepOps map[string]*lua.LState
	sleepMu  sync.Mutex

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
		id:      id,
		L:       L,
		app:     app,
		env:     env,
		verbose: opts.Verbose,
		inbox:   make(chan inboxMsg, 64),
		outbox:  make(chan common.OutboxMsg, 64),
		cancel:  cancel,
		done:    make(chan struct{}),
		timers:  make(map[string]*time.Timer),
		// Form show coroutines
		formCoros: make(map[string]*lua.LState),
		// Async operations - suspended coroutines waiting for completion
		asyncOps: make(map[string]*asyncOp),
		// Sleep operations - suspended coroutines waiting for k.sleep
		sleepOps: make(map[string]*lua.LState),
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
			if s.verbose {
				logger.Errorf("%s", postMortemDump(s.L))
			}
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
	case inboxFilePickerResp:
		s.resumeFilePickerResp(msg.respID, msg.resp, logger)
	case inboxQuery:
		if msg.query != nil && msg.reply != nil {
			msg.reply <- msg.query(s.L)
		}
	case inboxSleepDone:
		s.resumeSleep(msg.respID, logger)
	case inboxTabulatorDataResp:
		s.resumeTabulatorDataResp(msg.respID, msg.resp, logger)
	case inboxTabulatorSelectionResp:
		s.resumeTabulatorSelectionResp(msg.respID, msg.selectRows, logger)
	case inboxTabulatorAjaxRequest:
		s.handleTabulatorAjaxRequest(msg, logger)
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
		logger.Errorf("handler error: %v", err)
		if s.verbose {
			logger.Errorf("%s", postMortemDump(s.L))
		} else {
			logger.Errorf("%s", getStack(s.L))
		}
		s.outbox <- common.OutboxMsg{Type: "error", Msg: err.Error(), Stack: getStack(s.L)}
		return
	}

	// Flush outbox after handler
	s.flushOutbox()
}

// handleTabulatorAjaxRequest services the browser's remote-pagination page ask.
// It runs the tabulator_ajax_request handler in a fresh coroutine, captures the
// handler's Lua return value ({data, last_page} or {data, last_row}), and sends
// a tabulator_remote_data message back so the browser can render the page.
func (s *Session) handleTabulatorAjaxRequest(msg inboxMsg, logger Logger) {
	// Convert the JSON-decoded request {page,size,sort,filter} to a Lua value.
	val := lua.LNil
	if msg.raw != nil {
		val = s.toLuaValue(msg.raw)
	}

	// Look up the handler: form.handlers[ctrl]["tabulator_ajax_request"].
	formTbl := s.L.GetGlobal(msg.form)
	if formTbl == lua.LNil {
		return
	}
	tbl, ok := formTbl.(*lua.LTable)
	if !ok {
		return
	}
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
	handler := ctrlTbl.RawGetString("tabulator_ajax_request")
	if handler == lua.LNil {
		return
	}
	fn, ok := handler.(*lua.LFunction)
	if !ok {
		return
	}

	co, cancel := s.L.NewThread()
	st, err, rets := s.L.Resume(co, fn, val)
	if st == lua.ResumeError {
		if cancel != nil {
			cancel()
		}
		logger.Errorf("tabulator_ajax_request handler error: %v", err)
		if s.verbose {
			logger.Errorf("%s", postMortemDump(s.L))
		} else {
			logger.Errorf("%s", getStack(s.L))
		}
		s.outbox <- common.OutboxMsg{Type: "error", Msg: err.Error(), Stack: getStack(s.L)}
		return
	}
	if st != lua.ResumeOK && cancel != nil {
		cancel()
	}

	// Collect the handler return value and send the remote page to the browser.
	remote := remotePagePayload{Data: json.RawMessage("[]")}
	if len(rets) > 0 && rets[0] != lua.LNil {
		if retTbl, ok := rets[0].(*lua.LTable); ok {
			remote = pagePayloadFromTable(retTbl)
		}
	}

	s.SendOutbox(common.OutboxMsg{
		Type:     "tabulator_remote_data",
		Form:     msg.form,
		Ctrl:     msg.ctrl,
		Selector: "#c:" + msg.form + ":" + msg.ctrl,
		Data:     remote.toJSON(),
	})
	s.flushOutbox()
}

// remotePagePayload is the tabulator_remote_data response the Lua
// tabulator_ajax_request handler returns: {data=..., last_page=...} or
// {data=..., last_row=...}. It is serialized to JSON for the browser.
type remotePagePayload struct {
	Data     interface{} `json:"data"`
	LastPage int         `json:"last_page,omitempty"`
	LastRow  int         `json:"last_row,omitempty"`
}

// pagePayloadFromTable converts a Lua table return value into the fields of a
// remotePagePayload (data, last_page, last_row).
func pagePayloadFromTable(tbl *lua.LTable) remotePagePayload {
	var p remotePagePayload
	if d := tbl.RawGetString("data"); d != lua.LNil {
		// Re-encode the Lua table of rows as JSON for the browser.
		if dTbl, ok := d.(*lua.LTable); ok {
			p.Data = json.RawMessage(bindings.TableToJSON(dTbl))
		}
	}
	if lp := tbl.RawGetString("last_page"); lp != lua.LNil {
		p.LastPage = int(lua.LVAsNumber(lp))
	}
	if lr := tbl.RawGetString("last_row"); lr != lua.LNil {
		p.LastRow = int(lua.LVAsNumber(lr))
	}
	return p
}

// toJSON serializes the payload. If Data is nil, emit an empty array so the
// client always receives valid JSON and can clear the table.
func (p remotePagePayload) toJSON() string {
	data := p.Data
	if data == nil {
		data = json.RawMessage("[]")
	}
	type wire struct {
		Data     json.RawMessage `json:"data"`
		LastPage int             `json:"last_page,omitempty"`
		LastRow  int             `json:"last_row,omitempty"`
	}
	if p.Data == nil {
		out, _ := json.Marshal(wire{Data: json.RawMessage("[]")})
		return string(out)
	}
	out, _ := json.Marshal(wire{Data: data.(json.RawMessage), LastPage: p.LastPage, LastRow: p.LastRow})
	return string(out)
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
		if s.verbose {
			logger.Errorf("%s", postMortemDump(s.L))
		} else {
			logger.Errorf("%s", getStack(s.L))
		}
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
		if s.verbose {
			logger.Errorf("%s", postMortemDump(s.L))
		} else {
			logger.Errorf("%s", getStack(s.L))
		}
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
		select {
		case s.inbox <- inboxMsg{
			typ: inboxAsyncDone,
			data: map[string]interface{}{
				"op_id":  opID,
				"result": result,
				"error":  errStr,
			},
		}:
		case <-s.done:
			// session closed; drop
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

// RequestTabulatorGetData asks the browser for all current table data and
// suspends the coroutine until the browser delivers it (tabulator_data_resp).
// The resolved Lua value is a table of row tables (1-based).
func (s *Session) RequestTabulatorGetData(co *lua.LState, cancel func(), form, ctrl string) {
	reqID := fmt.Sprintf("getdata_%d", time.Now().UnixNano())

	s.asyncMu.Lock()
	s.asyncOps[reqID] = &asyncOp{co: co, cancel: cancel}
	s.asyncMu.Unlock()

	s.SendOutbox(common.OutboxMsg{
		Type: "tabulator_get_data",
		Form: form,
		Ctrl: ctrl,
		ID:   reqID,
	})
}

// RequestTabulatorGetSelection asks the browser for the selected row indices
// and suspends the coroutine until it is delivered (tabulator_selection_resp).
// The resolved Lua value is a 1-based table of row numbers.
func (s *Session) RequestTabulatorGetSelection(co *lua.LState, cancel func(), form, ctrl string) {
	reqID := fmt.Sprintf("getsel_%d", time.Now().UnixNano())

	s.asyncMu.Lock()
	s.asyncOps[reqID] = &asyncOp{co: co, cancel: cancel}
	s.asyncMu.Unlock()

	s.SendOutbox(common.OutboxMsg{
		Type: "tabulator_get_selection",
		Form: form,
		Ctrl: ctrl,
		ID:   reqID,
	})
}

// PostTabulatorDataResp forwards the browser's tabulator_data_resp value to the
// actor inbox so the suspended coroutine can be resumed on the actor goroutine.
func (s *Session) PostTabulatorDataResp(reqID, value string) {
	select {
	case s.inbox <- inboxMsg{typ: inboxTabulatorDataResp, respID: reqID, resp: value}:
	case <-s.done:
	}
}

// PostTabulatorSelectionResp forwards the browser's tabulator_selection_resp
// to the actor inbox so the suspended coroutine can be resumed.
func (s *Session) PostTabulatorSelectionResp(reqID string, rows []int) {
	select {
	case s.inbox <- inboxMsg{typ: inboxTabulatorSelectionResp, respID: reqID, selectRows: rows}:
	case <-s.done:
	}
}

// PostTabulatorAjaxRequest forwards the browser's tabulator_ajax_request (a
// remote-pagination page ask) to the actor inbox. The value is a JSON-decoded
// object (page/size/sort/filter) passed as-is to the Lua handler.
func (s *Session) PostTabulatorAjaxRequest(form, ctrl string, value interface{}) {
	select {
	case s.inbox <- inboxMsg{typ: inboxTabulatorAjaxRequest, form: form, ctrl: ctrl, raw: value}:
	case <-s.done:
	}
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

// RequestFilePicker asks the browser to open a file picker dialog and
// suspends the current coroutine until files are selected or cancelled.
// accept is a MIME filter (e.g. "image/*,.pdf"), multiple allows selecting
// more than one file.
func (s *Session) RequestFilePicker(co *lua.LState, cancel func(), accept string, multiple bool) {
	pickerID := fmt.Sprintf("filepicker_%d", time.Now().UnixNano())

	s.asyncMu.Lock()
	s.asyncOps[pickerID] = &asyncOp{co: co, cancel: cancel}
	s.asyncMu.Unlock()

	s.SendOutbox(common.OutboxMsg{
		Type:     "pick_file",
		ID:       pickerID,
		Accept:   accept,
		Multiple: multiple,
	})
}

// PostFilePickerResp is called from the web bridge goroutine when the browser
// delivers file picker results. The value is a JSON array of file objects.
func (s *Session) PostFilePickerResp(pickerID, value string) {
	select {
	case s.inbox <- inboxMsg{typ: inboxFilePickerResp, respID: pickerID, resp: value}:
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

// resumeFilePickerResp resumes a coroutine suspended by RequestFilePicker
// with a Lua table of file objects parsed from the JSON response.
func (s *Session) resumeFilePickerResp(pickerID string, jsonResp string, logger Logger) {
	s.asyncMu.Lock()
	op, exists := s.asyncOps[pickerID]
	if exists {
		delete(s.asyncOps, pickerID)
	}
	s.asyncMu.Unlock()

	if !exists {
		logger.Warnf("file_picker response for unknown op: %s", pickerID)
		return
	}

	val := s.parseFilePickerJSON(jsonResp)

	st, err, _ := s.L.Resume(op.co, nil, val)
	if st == lua.ResumeError {
		if s.env != nil && s.env.Logger != nil {
			s.env.Logger.Errorf("file_picker resume error: %v", err)
		}
		return
	}

	if st != lua.ResumeOK && op.cancel != nil {
		op.cancel()
	}

	s.flushOutbox()
}

// resumeSleep resumes a coroutine suspended by k.sleep.
func (s *Session) resumeSleep(sleepID string, logger Logger) {
	s.sleepMu.Lock()
	co, exists := s.sleepOps[sleepID]
	if exists {
		delete(s.sleepOps, sleepID)
	}
	s.sleepMu.Unlock()

	if !exists {
		logger.Warnf("sleep response for unknown op: %s", sleepID)
		return
	}

	st, err, _ := s.L.Resume(co, nil, lua.LNil)
	if st == lua.ResumeError {
		if s.env != nil && s.env.Logger != nil {
			s.env.Logger.Errorf("sleep resume error: %v", err)
		}
		return
	}

	s.flushOutbox()
}

// resumeTabulatorDataResp resumes the coroutine suspended by k.table.get_data
// with the browser-supplied row data (a JSON array converted to a Lua table).
func (s *Session) resumeTabulatorDataResp(reqID, jsonStr string, logger Logger) {
	s.asyncMu.Lock()
	op, exists := s.asyncOps[reqID]
	if exists {
		delete(s.asyncOps, reqID)
	}
	s.asyncMu.Unlock()

	if !exists {
		logger.Warnf("tabulator data response for unknown op: %s", reqID)
		return
	}

	val := lua.LNil
	if jsonStr != "" {
		var decoded interface{}
		if err := json.Unmarshal([]byte(jsonStr), &decoded); err == nil {
			val = s.toLuaValue(decoded)
		}
	}

	st, err, _ := s.L.Resume(op.co, nil, val)
	if st == lua.ResumeError {
		if s.env != nil && s.env.Logger != nil {
			s.env.Logger.Errorf("tabulator data resume error: %v", err)
		}
		return
	}
	if st != lua.ResumeOK && op.cancel != nil {
		op.cancel()
	}
	s.flushOutbox()
}

// resumeTabulatorSelectionResp resumes the coroutine suspended by
// k.table.get_selected_rows with a 1-based table of row numbers.
func (s *Session) resumeTabulatorSelectionResp(reqID string, rows []int, logger Logger) {
	s.asyncMu.Lock()
	op, exists := s.asyncOps[reqID]
	if exists {
		delete(s.asyncOps, reqID)
	}
	s.asyncMu.Unlock()

	if !exists {
		logger.Warnf("tabulator selection response for unknown op: %s", reqID)
		return
	}

	tbl := s.L.NewTable()
	for i, r := range rows {
		tbl.RawSetInt(i+1, lua.LNumber(r))
	}

	st, err, _ := s.L.Resume(op.co, nil, tbl)
	if st == lua.ResumeError {
		if s.env != nil && s.env.Logger != nil {
			s.env.Logger.Errorf("tabulator selection resume error: %v", err)
		}
		return
	}
	if st != lua.ResumeOK && op.cancel != nil {
		op.cancel()
	}
	s.flushOutbox()
}

// parseFilePickerJSON converts a JSON array of file objects to a Lua table.
// Each file object has: {name, size, type, data} where data is base64-encoded.
func (s *Session) parseFilePickerJSON(jsonStr string) lua.LValue {
	if jsonStr == "" || jsonStr == "null" {
		return lua.LNil
	}

	var files []map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &files); err != nil {
		// If parsing fails, return empty table
		return s.L.NewTable()
	}

	tbl := s.L.NewTable()
	for i, f := range files {
		fileTbl := s.L.NewTable()
		fileTbl.RawSetString("name", lua.LString(getStringFromMap(f, "name")))
		fileTbl.RawSetString("size", lua.LNumber(getFloatFromMap(f, "size")))
		fileTbl.RawSetString("type", lua.LString(getStringFromMap(f, "type")))
		fileTbl.RawSetString("data", lua.LString(getStringFromMap(f, "data")))
		tbl.RawSetInt(i+1, fileTbl)
	}
	return tbl
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

// Done returns a channel that is closed when the session is closing.
func (s *Session) Done() <-chan struct{} {
	return s.done
}

// PostEvent posts a browser event to the session inbox.
func (s *Session) PostEvent(form, ctrl, event string, value lua.LValue) {
	select {
	case s.inbox <- inboxMsg{typ: inboxWSEvent, form: form, ctrl: ctrl, event: event, value: value}:
	case <-s.done:
		// session closed; drop
	}
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
	var t *time.Timer
	t = time.AfterFunc(time.Duration(ms)*time.Millisecond, func() {
		s.PostTimer(id)
		if repeats {
			// Reschedule for repeats
			t.Reset(time.Duration(ms) * time.Millisecond)
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

// ScheduleSleep schedules a k.sleep completion.
// It stores the suspended coroutine and uses time.AfterFunc to post
// an inboxSleepDone message when the delay elapses.
func (s *Session) ScheduleSleep(co *lua.LState, delay time.Duration) {
	sleepID := fmt.Sprintf("sleep_%d", time.Now().UnixNano())

	s.sleepMu.Lock()
	s.sleepOps[sleepID] = co
	s.sleepMu.Unlock()

	s.wg.Add(1)
	go func(id string) {
		defer s.wg.Done()
		time.Sleep(delay)
		select {
		case s.inbox <- inboxMsg{typ: inboxSleepDone, respID: id}:
		case <-s.done:
			// session closed; drop
		}
	}(sleepID)
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
	select {
	case s.outbox <- common.OutboxMsg{Type: "quit"}:
	case <-s.done:
		// session already closing; drop
	}
}

// Logger interface for session logging.
type Logger interface {
	Printf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
	Warnf(format string, args ...interface{})
	Tracef(format string, args ...interface{})
}

func getStack(L *lua.LState) string {
	dbg, ok := L.GetStack(1) // skip getStack frame
	if !ok {
		return ""
	}
	return fmt.Sprintf("%s:%d", dbg.Source, dbg.CurrentLine)
}

// postMortemDump builds a backtrace for an error: every frame with its
// source, line, function name and local variables, plus upvalues. Used for
// post-mortem inspection (Tier 1 debugging) and attached to verbose error logs.
// NOTE: gopher-lua v1.1.2 unwinds frames before a Go Resume returns the error,
// so after a ResumeError the per-frame locals are usually no longer reachable;
// this returns the location as a fallback in that case.
func postMortemDump(L *lua.LState) string {
	var sb strings.Builder
	sb.WriteString("Lua stack trace:")
	found := false
	level := 0
	for {
		dbg, ok := L.GetStack(level)
		if !ok {
			break
		}
		found = true
		_, _ = L.GetInfo("nSlu", dbg, lua.LNil)
		fmt.Fprintf(&sb, "\n  #%d %s in %q (line %d)",
			level, dbg.Source, dbg.Name, dbg.CurrentLine)
		// Locals
		li := 1
		for {
			name, val := L.GetLocal(dbg, li)
			if name == "" {
				break
			}
			// Skip (*temporary) compiler temporaries in the dump for brevity
			if !strings.HasPrefix(name, "(*temporary)") {
				fmt.Fprintf(&sb, "\n      local %s = %s", name, val.String())
			}
			li++
		}
		level++
	}
	if !found {
		sb.WriteString("\n  (frames unwound; no backtrace available)")
	}
	return sb.String()
}

func getStringFromMap(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getFloatFromMap(m map[string]interface{}, key string) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}
	return 0
}
