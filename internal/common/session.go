// Package common provides shared types and interfaces used across KALUA packages to avoid import cycles.
package common

import (
	"time"

	"github.com/yuin/gopher-lua"
)

// SessionInterface defines the methods needed by the VM app to interact with the session.
type SessionInterface interface {
	PushForm(name string)
	PopForm() string
	TopForm() string
	SendOutbox(msg OutboxMsg)
	RunAsync(co *lua.LState, cancel func(), fn func() (interface{}, error), conv func(*lua.LState, interface{}) lua.LValue)
	ShowMsgbox(co *lua.LState, cancel func(), text, kind string) string
	HandleMsgboxChoice(msgboxID, choice string)
	StartTimer(id string, ms int, repeats bool)
	StopTimer(id string)
	ClientInfo() (w, h int, locale string)
	RequestClipboardGet(co *lua.LState, cancel func())
	PostClipboardResp(clipID, value string)
	RequestFilePicker(co *lua.LState, cancel func(), accept string, multiple bool)
	PostFilePickerResp(pickerID, value string)
	StoreFormCoro(name string, co *lua.LState)
	ResumeFormCoro(name string) bool
	ScheduleSleep(co *lua.LState, delay time.Duration)
	RequestTabulatorGetData(co *lua.LState, cancel func(), form, ctrl string)
	RequestTabulatorGetSelection(co *lua.LState, cancel func(), form, ctrl string)
	PostTabulatorDataResp(reqID, value string)
	PostTabulatorSelectionResp(reqID string, rows []int)
}

// DefaultConv converts an async result to a Lua value on the caller's state,
// falling back to GoValueToLua.
func DefaultConv(L *lua.LState, v interface{}) lua.LValue {
	return GoValueToLua(L, v)
}
