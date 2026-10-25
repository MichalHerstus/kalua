//go:build !wasm

package bindings

import (
	"encoding/json"
	"fmt"

	"github.com/yuin/gopher-lua"

	"kalua/internal/common"
)

// aiChatControl looks up the ai_chat control definition table on a form.
func aiChatControl(L *lua.LState, form, name string) *lua.LTable {
	ft := L.GetGlobal(form)
	if ft == lua.LNil {
		return nil
	}
	ftbl, ok := ft.(*lua.LTable)
	if !ok {
		return nil
	}
	controls := ftbl.RawGetString("controls")
	if controls == lua.LNil {
		return nil
	}
	ctbl, ok := controls.(*lua.LTable)
	if !ok {
		return nil
	}
	ctrl := ctbl.RawGetString(name)
	if ctrl == lua.LNil {
		return nil
	}
	ct, ok := ctrl.(*lua.LTable)
	if !ok {
		return nil
	}
	return ct
}

// aiChatIsBusy reads the control's ai_busy flag.
func aiChatIsBusy(ct *lua.LTable) bool {
	if v := ct.RawGetString("ai_busy"); v != lua.LNil {
		return lua.LVAsBool(v)
	}
	return false
}

// registerAIChatOps installs the k.ai_chat.* operations (§12.2). Run mode only.
func registerAIChatOps(e *Env) {
	registerOp := func(name string, fn lua.LGFunction) {
		e.register("ai_chat."+name, "controls", fn)
	}

	// k.ai_chat.send(form, name, text) — starts a turn through the same path
	// as a browser send, including on_send veto.
	registerOp("send", func(L *lua.LState) int {
		if e.Sess == nil {
			return e.fail(L, KErrorInvalidParam, "ai_chat.send: no session available")
		}
		form := L.CheckString(1)
		name := L.CheckString(2)
		text := L.CheckString(3)
		if aiChatControl(L, form, name) == nil {
			return e.fail(L, KErrorInvalidParam, fmt.Sprintf("ai_chat.send: control not found: %s.%s", form, name))
		}
		tbl := L.NewTable()
		tbl.RawSetString("text", lua.LString(text))
		if apiKey := L.OptString(4, ""); apiKey != "" {
			tbl.RawSetString("api_key", lua.LString(apiKey))
		}
		e.Sess.PostEvent(form, name, "ai_chat_send", tbl)
		return 0
	})

	// k.ai_chat.stop(form, name) — cancels the in-flight turn.
	registerOp("stop", func(L *lua.LState) int {
		if e.Sess == nil {
			return e.fail(L, KErrorInvalidParam, "ai_chat.stop: no session available")
		}
		form := L.CheckString(1)
		name := L.CheckString(2)
		if aiChatControl(L, form, name) == nil {
			return e.fail(L, KErrorInvalidParam, fmt.Sprintf("ai_chat.stop: control not found: %s.%s", form, name))
		}
		e.Sess.PostEvent(form, name, "ai_chat_stop", lua.LNil)
		return 0
	})

	// k.ai_chat.add_message(form, name, role, content[, name]) — appends a
	// message and pushes an ai_chat_message update.
	registerOp("add_message", func(L *lua.LState) int {
		form := L.CheckString(1)
		name := L.CheckString(2)
		role := L.CheckString(3)
		content := L.CheckString(4)
		msgName := L.OptString(5, "")
		ct := aiChatControl(L, form, name)
		if ct == nil {
			return e.fail(L, KErrorInvalidParam, fmt.Sprintf("ai_chat.add_message: control not found: %s.%s", form, name))
		}
		AppendAIChatMessage(L, ct, role, content, msgName)
		sendOutbox(e, common.OutboxMsg{
			Type: "ai_chat_message",
			Form: form,
			Ctrl: name,
			Data: messageToJSON(L, role, content, msgName),
		})
		return 0
	})

	// k.ai_chat.get_transcript(form, name) → table
	registerOp("get_transcript", func(L *lua.LState) int {
		form := L.CheckString(1)
		name := L.CheckString(2)
		ct := aiChatControl(L, form, name)
		if ct == nil {
			return e.fail(L, KErrorInvalidParam, fmt.Sprintf("ai_chat.get_transcript: control not found: %s.%s", form, name))
		}
		if t := ct.RawGetString("ai_transcript"); t != lua.LNil {
			if tt, ok := t.(*lua.LTable); ok {
				L.Push(tt)
				return 1
			}
		}
		L.Push(L.NewTable())
		return 1
	})

	// k.ai_chat.set_transcript(form, name, messages) — replaces the transcript
	// and pushes ai_chat_transcript.
	registerOp("set_transcript", func(L *lua.LState) int {
		form := L.CheckString(1)
		name := L.CheckString(2)
		msgs := L.CheckTable(3)
		ct := aiChatControl(L, form, name)
		if ct == nil {
			return e.fail(L, KErrorInvalidParam, fmt.Sprintf("ai_chat.set_transcript: control not found: %s.%s", form, name))
		}
		ct.RawSetString("ai_transcript", msgs)
		data, err := JSONStringifyLua(L, msgs, lua.LNil)
		if err != nil {
			data = "[]"
		}
		sendOutbox(e, common.OutboxMsg{
			Type: "ai_chat_transcript",
			Form: form,
			Ctrl: name,
			Data: data,
		})
		return 0
	})

	// k.ai_chat.clear(form, name) — set_transcript({}).
	registerOp("clear", func(L *lua.LState) int {
		form := L.CheckString(1)
		name := L.CheckString(2)
		ct := aiChatControl(L, form, name)
		if ct == nil {
			return e.fail(L, KErrorInvalidParam, fmt.Sprintf("ai_chat.clear: control not found: %s.%s", form, name))
		}
		ct.RawSetString("ai_transcript", L.NewTable())
		sendOutbox(e, common.OutboxMsg{
			Type: "ai_chat_transcript",
			Form: form,
			Ctrl: name,
			Data: "[]",
		})
		return 0
	})

	// k.ai_chat.set_system(form, name, text) — replaces the system prompt.
	registerOp("set_system", func(L *lua.LState) int {
		form := L.CheckString(1)
		name := L.CheckString(2)
		text := L.CheckString(3)
		ct := aiChatControl(L, form, name)
		if ct == nil {
			return e.fail(L, KErrorInvalidParam, fmt.Sprintf("ai_chat.set_system: control not found: %s.%s", form, name))
		}
		ct.RawSetString("ai_system", lua.LString(text))
		return 0
	})

	// k.ai_chat.is_busy(form, name) → boolean
	registerOp("is_busy", func(L *lua.LState) int {
		form := L.CheckString(1)
		name := L.CheckString(2)
		ct := aiChatControl(L, form, name)
		if ct == nil {
			L.Push(lua.LFalse)
			return 1
		}
		L.Push(lua.LBool(aiChatIsBusy(ct)))
		return 1
	})
}

// messageToJSON renders one chat message as JSON for the ai_chat_message outbox.
func messageToJSON(L *lua.LState, role, content, name string) string {
	obj := map[string]interface{}{"role": role, "content": content}
	if name != "" {
		obj["name"] = name
	}
	b, err := json.Marshal(obj)
	if err != nil {
		return "{}"
	}
	return string(b)
}

var _ = fmt.Sprintf // keep fmt import used if needed below
