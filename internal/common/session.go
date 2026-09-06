// Package common provides shared types and interfaces used across KALUA packages to avoid import cycles.
package common

import (
	"time"

	"github.com/yuin/gopher-lua"
)

// MsgboxButton is a single button in a message box. Value is the button's
// return value JSON-encoded so numbers, booleans and strings round-trip to the
// browser and back with their original type preserved.
type MsgboxButton struct {
	Label string // button text
	Value string // JSON-encoded return value
}

// MsgboxOptions describes a message box. Kind is the modal style used for the
// CSS class: info|warning|danger for the rich form, plus the legacy
// ok-cancel/yes-no/warn/error kinds from k.msgbox(text, kind).
type MsgboxOptions struct {
	ID      string
	Title   string
	Message string
	Kind    string
	Buttons []MsgboxButton
}

// PopupItem is a single entry in a k.popup menu. Label is the shown text. A
// leaf item carries Value (JSON-encoded return value); a branch item carries
// Items (the nested submenu). Items with a non-empty Items slice never return
// a value — they only open the submenu.
type PopupItem struct {
	Label string      // item text
	Value string      // JSON-encoded leaf return value ("" for branches)
	Items []PopupItem // nested submenu entries (branch)
}

// PopupOptions describes a k.popup menu. Title is optional and renders a
// header. Items is the top-level menu list.
type PopupOptions struct {
	ID    string
	Title string
	Items []PopupItem
}

// SessionInterface defines the methods needed by the VM app to interact with the session.
type SessionInterface interface {
	PushForm(name string)
	PopForm() string
	TopForm() string
	SendOutbox(msg OutboxMsg)
	RunAsync(co *lua.LState, cancel func(), fn func() (interface{}, error), conv func(*lua.LState, interface{}) lua.LValue)
	ShowMsgbox(co *lua.LState, cancel func(), opts MsgboxOptions) string
	HandleMsgboxChoice(msgboxID string, value interface{}, choice string)
	ShowPopup(co *lua.LState, cancel func(), opts PopupOptions) string
	HandlePopupChoice(popupID string, value interface{})
	DismissPopup(popupID string)
	StartTimer(id string, ms int, repeats bool)
	StopTimer(id string)
	ClientInfo() (w, h int, locale string)
	RequestClipboardGet(co *lua.LState, cancel func())
	PostClipboardResp(clipID, value string)
	RequestFilePicker(co *lua.LState, cancel func(), accept string, multiple bool)
	PostFilePickerResp(pickerID, value string)
	RequestFilePickerSave(co *lua.LState, cancel func(), mode, filename, data string) string
	PostFilePickerSaveResp(pickerID, value string)
	StoreFormCoro(name string, co *lua.LState)
	ResumeFormCoro(name string) bool
	ScheduleSleep(co *lua.LState, delay time.Duration)
	RequestTabulatorGetData(co *lua.LState, cancel func(), form, ctrl string)
	RequestTabulatorGetSelection(co *lua.LState, cancel func(), form, ctrl string)
	PostTabulatorDataResp(reqID, value string)
	PostTabulatorSelectionResp(reqID string, rows []int)
	RequestChartGetImage(co *lua.LState, cancel func(), form, ctrl string)
	PostChartImageResp(reqID, value string)

	// Post a browser event to the session's inbox (for ctrl.execute_event)
	PostEvent(form, ctrl, event string, value lua.LValue)

	// Form lifecycle events (k.form.on 3-arg overload)
	PostFormEvent(form, event string)

	// k.exec - run a stored function asynchronously
	RequestExec(co *lua.LState, cancel func(), fn *lua.LFunction, args []lua.LValue) string

	// k.ctrl.get_selection - browser round-trip for selection
	RequestSelection(co *lua.LState, cancel func(), form, ctrl string) string
	PostSelectionResp(reqID string, value map[string]interface{})
}

// DefaultConv converts an async result to a Lua value on the caller's state,
// falling back to GoValueToLua.
func DefaultConv(L *lua.LState, v interface{}) lua.LValue {
	return GoValueToLua(L, v)
}
