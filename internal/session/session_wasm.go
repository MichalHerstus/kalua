//go:build js && wasm

// Package session implements the per-tab actor for WASM mode (M0: basic forms only).
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

// Session is the per-tab actor for WASM mode.
type Session struct {
	id       string
	L        *lua.LState
	app      *vm.App
	env      *bindings.Env
	verbose  bool
	inbox    chan inboxMsg
	outbox   chan common.OutboxMsg
	cancel   context.CancelFunc
	done     chan struct{}
	wg       sync.WaitGroup
	quitting bool

	// Form stack for modal Show Form semantics (D52).
	formStack []string

	// Form show coroutines - suspended coroutines waiting for form close
	formCoros  map[string]*lua.LState
	formCoroMu sync.Mutex

	// Timers managed by this session
	timers map[string]*time.Timer

	// Idle timers for form on_idle events (one per form name)
	idleTimers map[string]*time.Timer

	// Async operations - suspended coroutines waiting for completion
	asyncOps     map[string]*asyncOp
	asyncMu      sync.Mutex
	gridAsyncOps map[string]*gridAsyncOp
	gridAsyncMu  sync.Mutex

	// Sleep operations - suspended coroutines waiting for k.sleep
	sleepOps map[string]*lua.LState
	sleepMu  sync.Mutex

	// Client info from the browser's client_info message (screen size/locale).
	clientMu     sync.RWMutex
	clientW      int
	clientH      int
	clientLocale string
}

// NewWithTransport creates and starts a new session actor with a custom transport.
// This is used for WASM where the transport is an in-page bridge instead of WebSocket.
func NewWithTransport(id string, L *lua.LState, app *vm.App, env *bindings.Env, transport common.Transport, logger Logger) (*Session, error) {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Session{
		id:         id,
		L:          L,
		app:        app,
		env:        env,
		verbose:    env.Verbose(),
		inbox:      make(chan inboxMsg, 64),
		outbox:     make(chan common.OutboxMsg, 64),
		cancel:     cancel,
		done:       make(chan struct{}),
		timers:     make(map[string]*time.Timer),
		idleTimers: make(map[string]*time.Timer),
		formCoros:  make(map[string]*lua.LState),
		asyncOps:   make(map[string]*asyncOp),
		gridAsyncOps: make(map[string]*gridAsyncOp),
		sleepOps:   make(map[string]*lua.LState),
	}

	env.Sess = s
	app.SetSession(s)

	s.wg.Add(1)
	go s.runWithTransport(ctx, transport, logger)

	return s, nil
}

// runWithTransport is the actor's main loop with a custom transport.
func (s *Session) runWithTransport(ctx context.Context, transport common.Transport, logger Logger) {
	defer s.wg.Done()
	defer transport.Close()

	go func() {
		for {
			select {
			case msg, ok := <-s.outbox:
				if !ok {
					return
				}
				if err := transport.Send(msg); err != nil {
					logger.Errorf("transport send error: %v", err)
					return
				}
			case <-s.done:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	go func() {
		for {
			select {
			case msg, ok := <-transport.Recv():
				if !ok {
					return
				}
				s.handleTransportInbox(msg, logger)
			case <-s.done:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	<-ctx.Done()
}

// handleTransportInbox converts a common.InboxMsg to internal inboxMsg and processes it.
func (s *Session) handleTransportInbox(msg common.InboxMsg, logger Logger) {
	imsg := inboxMsg{
		form:       msg.Form,
		ctrl:       msg.Ctrl,
		event:      msg.Event,
		timer:      msg.Timer,
		respID:     msg.ID,
		resp:       msg.Choice,
		selectRows: msg.SelectRows,
	}

	switch msg.Type {
	case "event":
		imsg.typ = inboxWSEvent
		imsg.raw = msg.Value
	case "msgbox_choice":
		imsg.typ = inboxMsgboxChoice
		imsg.raw = msg.Value
	case "popup_choice":
		imsg.typ = inboxPopupChoice
		imsg.raw = msg.Value
	case "popup_dismiss":
		imsg.typ = inboxPopupDismiss
	case "clipboard_resp":
		imsg.typ = inboxClipboardResp
		imsg.resp = msg.Choice
	case "file_picker_resp":
		imsg.typ = inboxFilePickerResp
		imsg.resp = msg.Choice
	case "file_picker_save_resp":
		imsg.typ = inboxFilePickerSaveResp
		imsg.resp = msg.Choice
	case "tabulator_data_resp":
		imsg.typ = inboxTabulatorDataResp
		imsg.resp = msg.Choice
	case "tabulator_selection_resp":
		imsg.typ = inboxTabulatorSelectionResp
		imsg.selectRows = msg.SelectRows
	case "tabulator_ajax_request":
		imsg.typ = inboxTabulatorAjaxRequest
		imsg.raw = msg.Value
	case "looper_scroll_request":
		imsg.typ = inboxLooperScrollRequest
		imsg.raw = msg.Value
	case "looper_refresh_request":
		imsg.typ = inboxLooperRefreshRequest
		imsg.raw = msg.Value
	case "chart_image_resp":
		imsg.typ = inboxChartImageResp
		imsg.resp = msg.Choice
	case "client_info":
		if data, ok := msg.Value.(map[string]interface{}); ok {
			w := 0
			h := 0
			locale := ""
			if v, ok := data["w"].(float64); ok {
				w = int(v)
			}
			if v, ok := data["h"].(float64); ok {
				h = int(v)
			}
			if v, ok := data["locale"].(string); ok {
				locale = v
			}
			s.SetClientInfo(w, h, locale)
		}
		return
	case "selection_resp":
		imsg.typ = inboxSelectionResp
		imsg.raw = msg.Value
	default:
		logger.Warnf("unknown inbox message type: %s", msg.Type)
		return
	}

	s.handleInbox(imsg, logger)
}

// SetClientInfo sets the client info (screen size, locale).
func (s *Session) SetClientInfo(w, h int, locale string) {
	s.clientMu.Lock()
	defer s.clientMu.Unlock()
	s.clientW = w
	s.clientH = h
	s.clientLocale = locale
}

// handleInbox processes a single inbox message (minimal WASM version).
func (s *Session) handleInbox(msg inboxMsg, logger Logger) {
	switch msg.typ {
	case inboxWSEvent:
		s.handleWSEvent(msg, logger)
	case inboxFormEvent:
		s.handleFormEvent(msg.form, msg.event, logger)
	}
}

// handleWSEvent dispatches a browser event to the appropriate Lua handler.
func (s *Session) handleWSEvent(msg inboxMsg, logger Logger) {
	value := msg.value
	if msg.raw != nil {
		value = s.toLuaValue(msg.raw)
	}

	ran := s.runFormHandler(msg.form, msg.ctrl, msg.event, []lua.LValue{value}, logger)

	if !ran {
		s.runFormHandler(msg.form, "@form", msg.event, []lua.LValue{value}, logger)
	}
}

// handleFormEvent processes form lifecycle events.
func (s *Session) handleFormEvent(formName, event string, logger Logger) {
	s.runFormHandler(formName, "@form", event, []lua.LValue{lua.LString(event)}, logger)
}

// runFormHandler looks up and runs a form event handler in a fresh coroutine.
func (s *Session) runFormHandler(formName, ctrlKey, event string, resumeArgs []lua.LValue, logger Logger) bool {
	formTbl := s.L.GetGlobal(formName)
	if formTbl == lua.LNil {
		return false
	}
	tbl, ok := formTbl.(*lua.LTable)
	if !ok {
		return false
	}

	handlers := tbl.RawGetString("handlers")
	if handlers == lua.LNil {
		return false
	}
	handlersTbl, ok := handlers.(*lua.LTable)
	if !ok {
		return false
	}

	ctrlHandlers := handlersTbl.RawGetString(ctrlKey)
	if ctrlHandlers == lua.LNil {
		return false
	}
	ctrlTbl, ok := ctrlHandlers.(*lua.LTable)
	if !ok {
		return false
	}

	handler := ctrlTbl.RawGetString(event)
	if handler == lua.LNil {
		return false
	}
	fn, ok := handler.(*lua.LFunction)
	if !ok {
		return false
	}

	co, cancel := s.L.NewThread()

	st, err, _ := s.L.Resume(co, fn, resumeArgs...)
	if st == lua.ResumeError {
		if cancel != nil {
			cancel()
		}
		logger.Errorf("handler error: %v", err)
		s.notifyError(err)
		s.outbox <- common.OutboxMsg{Type: "error", Msg: err.Error(), Stack: getStack(s.L)}
		return true
	}

	s.flushOutbox()
	return true
}

// flushOutbox sends all pending outbox messages.
func (s *Session) flushOutbox() {
	for {
		select {
		case <-s.outbox:
			// Messages are sent via transport in runWithTransport
		default:
			return
		}
	}
}

// ClientInfo returns the client info (screen size, locale).
func (s *Session) ClientInfo() (w, h int, locale string) {
	s.clientMu.RLock()
	defer s.clientMu.RUnlock()
	return s.clientW, s.clientH, s.clientLocale
}

// notifyError sends an error to the error handler.
func (s *Session) notifyError(err error) {
	if s.env != nil {
		s.env.FireError(s.L, bindings.ClassifyError(err), err.Error())
	}
}

// Stub implementations for SessionInterface methods not needed in M0
func (s *Session) PushForm(name string)          { s.formStack = append(s.formStack, name) }
func (s *Session) PopForm() string              { n := len(s.formStack) - 1; if n >= 0 { name := s.formStack[n]; s.formStack = s.formStack[:n]; return name }; return "" }
func (s *Session) TopForm() string              { n := len(s.formStack) - 1; if n >= 0 { return s.formStack[n] }; return "" }
func (s *Session) SendOutbox(msg common.OutboxMsg) { select { case s.outbox <- msg: default: } }
func (s *Session) RunAsync(co *lua.LState, cancel func(), fn func() (interface{}, error), conv func(*lua.LState, interface{}) lua.LValue) {}
func (s *Session) ShowMsgbox(co *lua.LState, cancel func(), opts common.MsgboxOptions) string { return "" }
func (s *Session) HandleMsgboxChoice(msgboxID string, value interface{}, choice string) {}
func (s *Session) ShowPopup(co *lua.LState, cancel func(), opts common.PopupOptions) string { return "" }
func (s *Session) HandlePopupChoice(popupID string, value interface{}) {}
func (s *Session) DismissPopup(popupID string) {}
func (s *Session) StartTimer(id string, ms int, repeats bool) {}
func (s *Session) StopTimer(id string) {}
func (s *Session) RequestClipboardGet(co *lua.LState, cancel func()) {}
func (s *Session) PostClipboardResp(clipID, value string) {}
func (s *Session) RequestFilePicker(co *lua.LState, cancel func(), accept string, multiple bool) {}
func (s *Session) PostFilePickerResp(pickerID, value string) {}
func (s *Session) RequestFilePickerSave(co *lua.LState, cancel func(), mode, filename, data string) string { return "" }
func (s *Session) PostFilePickerSaveResp(pickerID, value string) {}
func (s *Session) StoreFormCoro(name string, co *lua.LState) {}
func (s *Session) ResumeFormCoro(name string) bool { return false }
func (s *Session) ScheduleSleep(co *lua.LState, delay time.Duration) {}
func (s *Session) RequestTabulatorGetData(co *lua.LState, cancel func(), form, ctrl string) {}
func (s *Session) RequestTabulatorGetSelection(co *lua.LState, cancel func(), form, ctrl string) {}
func (s *Session) PostTabulatorDataResp(reqID, value string) {}
func (s *Session) PostTabulatorSelectionResp(reqID string, rows []int) {}
func (s *Session) RequestChartGetImage(co *lua.LState, cancel func(), form, ctrl string) {}
func (s *Session) PostChartImageResp(reqID, value string) {}
func (s *Session) PostEvent(form, ctrl, event string, value lua.LValue) {}
func (s *Session) PostFormEvent(form, event string) {}
func (s *Session) RequestExec(co *lua.LState, cancel func(), fn *lua.LFunction, args []lua.LValue) string { return "" }
func (s *Session) RequestSelection(co *lua.LState, cancel func(), form, ctrl string) string { return "" }
func (s *Session) PostSelectionResp(reqID string, value map[string]interface{}) {}

// Grid methods (stubs for M0)
func (s *Session) RequestGridGetSelected(co *lua.LState, cancel func(), form, ctrl string) string { return "" }
func (s *Session) PostGridGetSelectedResp(reqID string, rows []map[string]interface{}) {}
func (s *Session) RequestGridGetRow(co *lua.LState, cancel func(), form, ctrl string, pk interface{}) string { return "" }
func (s *Session) PostGridGetRowResp(reqID string, row map[string]interface{}) {}
func (s *Session) RequestGridDeleteRow(co *lua.LState, cancel func(), form, ctrl string, pk interface{}) string { return "" }
func (s *Session) PostGridDeleteRowResp(reqID string, ok bool, err string) {}
func (s *Session) RequestGridBatchDelete(co *lua.LState, cancel func(), form, ctrl string, pks []interface{}) string { return "" }
func (s *Session) PostGridBatchDeleteResp(reqID string, ok bool, err string) {}
func (s *Session) RequestGridInsertRow(co *lua.LState, cancel func(), form, ctrl string, data map[string]interface{}) string { return "" }
func (s *Session) PostGridInsertRowResp(reqID string, pk interface{}, err string) {}
func (s *Session) RequestGridUpdateRow(co *lua.LState, cancel func(), form, ctrl string, pk interface{}, data map[string]interface{}) string { return "" }
func (s *Session) PostGridUpdateRowResp(reqID string, ok bool, err string) {}

// toLuaValue converts a Go value to a Lua value.
func (s *Session) toLuaValue(v interface{}) lua.LValue {
	switch t := v.(type) {
	case nil:
		return lua.LNil
	case bool:
		return lua.LBool(t)
	case string:
		return lua.LString(t)
	case float64:
		return lua.LNumber(t)
	case int:
		return lua.LNumber(t)
	case int64:
		return lua.LNumber(t)
	case map[string]interface{}:
		tbl := s.L.NewTable()
		for k, val := range t {
			tbl.RawSetString(k, s.toLuaValue(val))
		}
		return tbl
	case []interface{}:
		tbl := s.L.NewTable()
		for i, val := range t {
			tbl.RawSetInt(i+1, s.toLuaValue(val))
		}
		return tbl
	default:
		return lua.LString(fmt.Sprintf("%v", v))
	}
}

// getStack returns a string representation of the Lua stack.
func getStack(L *lua.LState) string {
	var result string
	level := 0
	for {
		dbg, ok := L.GetStack(level)
		if !ok {
			break
		}
		_, _ = L.GetInfo("nSlu", dbg, lua.LNil)
		result += fmt.Sprintf("  #%d %s in %q (line %d)\n",
			level, dbg.Source, dbg.Name, dbg.CurrentLine)
		level++
	}
	return result
}