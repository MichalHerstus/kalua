//go:build !wasm

package session

import (
	"context"
	"fmt"
	"strings"

	"github.com/yuin/gopher-lua"

	"kalua/internal/ai"
	"kalua/internal/bindings"
	"kalua/internal/common"
)

// aiChatDispatch returns true when a browser event targets an AI chat form,
// so the generic control-value update is skipped.
func (s *Session) aiChatDispatch(msg inboxMsg) bool {
	if msg.event != "ai_chat_send" && msg.event != "ai_chat_close" {
		return false
	}
	formTbl := s.L.GetGlobal(msg.form)
	if formTbl == lua.LNil {
		return false
	}
	ft, ok := formTbl.(*lua.LTable)
	if !ok {
		return false
	}
	return ft.RawGetString("ai_transcript") != lua.LNil
}

// handleAIChatSend processes an ai_chat_send browser message: appends the
// user's message to the transcript and starts the agent loop.
func (s *Session) handleAIChatSend(formName, text, apiKey string, logger Logger) {
	if text == "" {
		return
	}

	formTbl := s.L.GetGlobal(formName)
	if formTbl == lua.LNil {
		return
	}
	ft, ok := formTbl.(*lua.LTable)
	if !ok {
		return
	}

	// Read chat options. Per-call api_key overrides env/INI config.
	system := ""
	if v := ft.RawGetString("ai_system"); v != lua.LNil {
		system = v.String()
	}
	maxTurns := 8
	if v := ft.RawGetString("ai_max_turns"); v != lua.LNil {
		maxTurns = int(lua.LVAsNumber(v))
		if maxTurns < 1 {
			maxTurns = 1
		}
	}
	baseURL := ""
	if v := ft.RawGetString("ai_base_url"); v != lua.LNil {
		baseURL = v.String()
	}
	model := ""
	if v := ft.RawGetString("ai_model"); v != lua.LNil {
		model = v.String()
	}
	if model == "" {
		model = "local-model"
	}
	if baseURL == "" {
		baseURL = "http://localhost:1234/v1"
	}
	// Per-message api_key (from browser) overrides form-stored key.
	if apiKey == "" {
		if v := ft.RawGetString("ai_api_key"); v != lua.LNil {
			apiKey = v.String()
		}
	}

	// Append user message to transcript.
	bindings.AppendAITranscript(s.L, formName, "user", text, "")

	// Capture a snapshot of the transcript for the agent loop.
	t := ft.RawGetString("ai_transcript")
	transcriptTbl, _ := t.(*lua.LTable)
	transcript := bindings.LuaTableToTranscript(s.L, transcriptTbl)

	// Build tool schemas: explicit ai.tool overrides first, then auto-exposed
	// script globals.
	var schemas []common.ToolSchema
	if tools := ft.RawGetString("ai_tools"); tools != lua.LNil {
		if tt, ok := tools.(*lua.LTable); ok {
			tt.ForEach(func(_, v lua.LValue) {
				if entry, ok := v.(*lua.LTable); ok {
					schemas = append(schemas, bindings.AIToolSchemaFromLua(entry))
				}
			})
		}
	}
	autoSchemas := ai.CollectSchemas(s.L, bindings.SandboxLibNames())
	schemas = append(schemas, autoSchemas...)

	// Build the system prompt with tool schemas.
	sysPrompt := system
	if len(schemas) > 0 {
		sysPrompt += "\n\nAvailable tools:\n" + ai.ToolSchemasJSON(schemas)
	}
	sysPrompt += "\n\nTo call a tool, emit: function {\"name\":\"<tool>\",\"arguments\":{...}}"

	// Send status.
	s.SendOutbox(common.OutboxMsg{Type: "ai_chat_status", Form: formName, Data: "Thinking…"})

	// Run the agent loop in a worker goroutine. Tool execution goes through
	// Session.Query (actor-goroutine Lua access) so tools can call k.* APIs.
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		cfg := ai.ProviderConfig{
			BaseURL: baseURL,
			APIKey:  apiKey,
			Model:   model,
		}
		client := ai.NewClient(cfg)

		var assistantParts []string
		_, turnErr := ai.AgentRun(
			context.Background(),
			client,
			sysPrompt,
			transcript,
			maxTurns,
			func(name string, args map[string]interface{}) (interface{}, error) {
				return s.executeAITool(name, args, logger)
			},
			func(delta string) {
				assistantParts = append(assistantParts, delta)
				s.SendOutbox(common.OutboxMsg{
					Type: "ai_chat_delta",
					Form: formName,
					Data: delta,
				})
			},
			func(statusText string) {
				s.SendOutbox(common.OutboxMsg{
					Type: "ai_chat_status",
					Form: formName,
					Data: statusText,
				})
			},
		)
		_ = turnErr

		assistantText := strings.Join(assistantParts, "")
		if assistantText != "" {
			bindings.AppendAITranscript(s.L, formName, "assistant", assistantText, "")
		}

		if turnErr != nil {
			s.SendOutbox(common.OutboxMsg{
				Type: "ai_chat_status",
				Form: formName,
				Data: "Error: " + turnErr.Error(),
			})
		}

		s.SendOutbox(common.OutboxMsg{Type: "ai_chat_done", Form: formName})
	}()
}

// executeAITool runs a tool call on the actor goroutine via Session.Query.
// The tool implementation is a Lua global function looked up by name.
func (s *Session) executeAITool(name string, args map[string]interface{}, logger Logger) (interface{}, error) {
	// Look up the global function by name.
	fnVal := s.L.GetGlobal(name)
	fn, ok := fnVal.(*lua.LFunction)
	if !ok || fn == nil {
		return nil, fmt.Errorf("tool %q not found", name)
	}

	// Build a single args table.
	argsTbl := s.L.NewTable()
	for k, v := range args {
		argsTbl.RawSetString(k, common.GoValueToLua(s.L, v))
	}

	result := s.Query(func(L *lua.LState) lua.LValue {
		L.Push(fn)
		L.Push(argsTbl)
		if err := L.PCall(1, 1, nil); err != nil {
			return lua.LString(fmt.Sprintf("tool error: %v", err))
		}
		return L.Get(-1)
	})

	return luaValueToGo(result), nil
}

// luaValueToGo converts a Lua value to a Go value suitable for JSON marshaling.
func luaValueToGo(v lua.LValue) interface{} {
	switch v.Type() {
	case lua.LTNil:
		return nil
	case lua.LTBool:
		return bool(v.(lua.LBool))
	case lua.LTNumber:
		return float64(v.(lua.LNumber))
	case lua.LTString:
		return string(v.(lua.LString))
	case lua.LTTable:
		tbl := v.(*lua.LTable)
		// Detect array vs map.
		if tbl.Len() > 0 {
			arr := make([]interface{}, tbl.Len())
			for i := 1; i <= tbl.Len(); i++ {
				arr[i-1] = luaValueToGo(tbl.RawGetInt(i))
			}
			return arr
		}
		m := make(map[string]interface{})
		tbl.ForEach(func(k, v2 lua.LValue) {
			m[k.String()] = luaValueToGo(v2)
		})
		return m
	default:
		return v.String()
	}
}

// handleAIChatClose processes an ai_chat_close browser message: resumes the
// suspended coroutine with the full transcript table.
func (s *Session) handleAIChatClose(formName string) {
	formTbl := s.L.GetGlobal(formName)
	if formTbl == lua.LNil {
		return
	}
	ft, ok := formTbl.(*lua.LTable)
	if !ok {
		return
	}

	t := ft.RawGetString("ai_transcript")
	var transcript *lua.LTable
	if t == lua.LNil {
		transcript = s.L.NewTable()
	} else {
		transcript, _ = t.(*lua.LTable)
		if transcript == nil {
			transcript = s.L.NewTable()
		}
	}

	s.ResumeFormCoroWith(formName, transcript)
}
