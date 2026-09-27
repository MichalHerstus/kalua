// Package session implements the per-tab actor that owns an LState and serializes
// all Lua execution for that browser tab. It communicates with the browser via
// an inbox (WS events, timers, async completions) and an outbox (UI commands).
package session

import (
	"github.com/yuin/gopher-lua"
)

// Logger interface for logging.
type Logger interface {
	Printf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
	Warnf(format string, args ...interface{})
	Tracef(format string, args ...interface{})
}

// Inbox message types — all events that can reach the actor goroutine.
type inboxMsgType int

const (
	inboxNone                   inboxMsgType = iota
	inboxWSEvent                             // event from browser (click, input, etc.)
	inboxTimer                               // timer fired
	inboxAsyncDone                           // blocking operation completed (DB, HTTP, etc.)
	inboxMsgboxChoice                        // user answered a k.msgbox
	inboxPopupChoice                         // user picked a k.popup item
	inboxPopupDismiss                        // user dismissed a k.popup (Esc / outside)
	inboxClipboardResp                       // browser clipboard_get value
	inboxFilePickerResp                      // browser file picker result (JSON-encoded files)
	inboxQuery                               // external read of Lua state (tests)
	inboxSleepDone                           // k.sleep completed
	inboxTabulatorDataResp                   // browser answered k.table.get_data
	inboxTabulatorSelectionResp              // browser answered k.table.get_selected_rows
	inboxTabulatorAjaxRequest                // browser asked for a remote page of rows
	inboxLooperScrollRequest                 // browser asked for the next looper batch of rows
	inboxLooperRefreshRequest                // browser re-triggered a looper fetch
	inboxChartImageResp                      // browser answered k.chart.get_image
	inboxFormEvent                           // form lifecycle event (open_form, after_open_form, close_form, on_idle)
	inboxExec                                // k.exec async function execution
	inboxSelectionResp                       // browser answered k.ctrl.get_selection
	inboxFilePickerSaveResp                  // browser answered k.pick_file save/download
	inboxGridFormOpen                        // grid detail/edit/new modal requested
	inboxGridFormSave                        // grid modal Save clicked
	inboxGridFormCancel                      // grid modal Cancel/Close clicked
	inboxGridRowDelete                       // grid row delete requested
	inboxGridBatchDelete                     // grid selected-rows delete requested
	inboxGridGetSelected                     // k.grid.get_selected request
	inboxGridGetSelectedResp                 // k.grid.get_selected response
	inboxGridGetRow                          // k.grid.get_row request
	inboxGridGetRowResp                      // k.grid.get_row response
	inboxGridDeleteRow                       // k.grid.delete_row request
	inboxGridDeleteRowResp                   // k.grid.delete_row response
	inboxGridBatchDeleteReq                  // k.grid.batch_delete request
	inboxGridBatchDeleteResp                 // k.grid.batch_delete response
	inboxGridInsertRow                       // k.grid.insert_row request
	inboxGridInsertRowResp                   // k.grid.insert_row response
	inboxGridUpdateRow                       // k.grid.update_row request
	inboxGridUpdateRowResp                   // k.grid.update_row response
)

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

// asyncOp represents a suspended coroutine waiting for an async operation
type asyncOp struct {
	co     *lua.LState                               // coroutine to resume
	cancel func()                                    // cleanup function
	conv   func(*lua.LState, interface{}) lua.LValue // result converter (nil = default)
	// For file picker: "open" | "save" | "download"
	pickMode string
}

// gridAsyncOp represents a suspended coroutine waiting for a grid async operation
type gridAsyncOp struct {
	co     *lua.LState
	cancel func()
}