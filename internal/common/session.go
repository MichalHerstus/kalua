// Package common provides shared types and interfaces used across KALUA packages to avoid import cycles.
package common

import (
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
}

// DefaultConv converts an async result to a Lua value on the caller's state,
// falling back to GoValueToLua.
func DefaultConv(L *lua.LState, v interface{}) lua.LValue {
	return GoValueToLua(L, v)
}
