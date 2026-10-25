//go:build !wasm

package session

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yuin/gopher-lua"

	"kalua/internal/ai"
	"kalua/internal/bindings"
	"kalua/internal/common"
)

// errSessionClosed is returned when a tool call cannot complete because the
// session is shutting down.
var errSessionClosed = fmt.Errorf("session closed")

// handleAIChatControlSend processes a control-scoped ai_chat_send: it runs the
// on_send veto synchronously, appends the user message to the control's
// transcript, marks the control busy, and spawns the agent turn worker.
func (s *Session) handleAIChatControlSend(form, ctrl, text, apiKey string, logger Logger) {
	if text == "" {
		return
	}

	ctrlTbl := s.controlTable(form, ctrl)
	if ctrlTbl == nil {
		return
	}
	if ctrlTbl.RawGetString("type").String() != "ai_chat" {
		return
	}

	// One turn per control: ignore sends while busy.
	if v := ctrlTbl.RawGetString("ai_busy"); v != lua.LNil && lua.LVAsBool(v) {
		return
	}

	// on_send veto — runs synchronously so its return value can be inspected.
	// A suspending API inside on_send is unsupported; the turn proceeds if it
	// yields.
	if rets, ok := s.runFormHandlerSync(logger, form, ctrl, "on_send", []lua.LValue{lua.LString(text)}); ok && len(rets) > 0 {
		if v := rets[0]; v != lua.LNil && lua.LVAsBool(v) == false {
			return // vetoed
		}
	}

	// Append the user message to the control's transcript (actor-owned).
	bindings.AppendAIChatMessage(s.L, ctrlTbl, "user", text, "")
	ctrlTbl.RawSetString("ai_busy", lua.LTrue)

	// Snapshot the transcript for the agent loop.
	t := ctrlTbl.RawGetString("ai_transcript")
	transcript := bindings.LuaTableToTranscript(s.L, t.(*lua.LTable))

	// Read options from the control table.
	system := ""
	if v := ctrlTbl.RawGetString("ai_system"); v != lua.LNil {
		system = v.String()
	}
	maxTurns := 8
	if v := ctrlTbl.RawGetString("ai_max_turns"); v != lua.LNil {
		maxTurns = int(lua.LVAsNumber(v))
		if maxTurns < 1 {
			maxTurns = 1
		}
	}
	baseURL := ""
	if v := ctrlTbl.RawGetString("ai_base_url"); v != lua.LNil {
		baseURL = v.String()
	}
	model := ""
	if v := ctrlTbl.RawGetString("ai_model"); v != lua.LNil {
		model = v.String()
	}
	if model == "" {
		model = "local-model"
	}
	if baseURL == "" {
		baseURL = "http://localhost:1234/v1"
	}
	if apiKey == "" {
		if v := ctrlTbl.RawGetString("ai_api_key"); v != lua.LNil {
			apiKey = v.String()
		}
	}

	// Build schemas once, here (deduped vs. the old modal path).
	schemas := collectAIChatSchemas(s.L, ctrlTbl)

	// Build the system prompt.
	sysPrompt := system
	if len(schemas) > 0 {
		sysPrompt += "\n\nAvailable tools:\n" + ai.ToolSchemasJSON(schemas)
	}
	sysPrompt += "\n\nTo call a tool, emit: function {\"name\":\"<tool>\",\"arguments\":{...}}"

	// Status.
	s.SendOutbox(common.OutboxMsg{Type: "ai_chat_status", Form: form, Ctrl: ctrl, Data: "Thinking…"})

	key := form + "\x00" + ctrl
	ctx, cancel := context.WithCancel(context.Background())
	s.aiTurnMu.Lock()
	s.aiTurns[key] = &aiTurn{cancel: cancel}
	s.aiTurnMu.Unlock()

	// Capture form/ctrl for the worker goroutine.
	formCap, ctrlCap := form, ctrl

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		cfg := ai.ProviderConfig{BaseURL: baseURL, APIKey: apiKey, Model: model}
		client := ai.NewClient(cfg)

		var assistantParts []string
		_, turnErr := ai.AgentRun(
			ctx,
			client,
			sysPrompt,
			transcript,
			maxTurns,
			func(name string, args map[string]interface{}) (interface{}, error) {
				result, err := s.executeAIToolControl(formCap, ctrlCap, name, args, logger)
				// Fire on_tool on the actor with JSON-encoded args/result; the
				// actor parses them into Lua tables (postAIChatEvent) so no
				// LState is touched from this worker goroutine.
				argsJSON, _ := json.Marshal(args)
				resultJSON, _ := json.Marshal(result)
				if resultJSON == nil {
					resultJSON = []byte("null")
				}
				select {
				case s.inbox <- inboxMsg{typ: inboxAIChatEvent, form: formCap, ctrl: ctrlCap,
					event: "on_tool",
					args: []lua.LValue{lua.LString(name),
						lua.LString(string(argsJSON)), lua.LString(string(resultJSON))}}:
				case <-s.done:
				}
				return result, err
			},
			func(delta string) {
				assistantParts = append(assistantParts, delta)
				s.SendOutbox(common.OutboxMsg{
					Type: "ai_chat_delta",
					Form: formCap,
					Ctrl: ctrlCap,
					Data: delta,
				})
			},
			func(statusText string) {
				s.SendOutbox(common.OutboxMsg{
					Type: "ai_chat_status",
					Form: formCap,
					Ctrl: ctrlCap,
					Data: statusText,
				})
			},
		)
		_ = turnErr

		assistantText := strings.Join(assistantParts, "")

		// Write transcript + state on the actor (never from the worker).
		s.Query(func(L *lua.LState) lua.LValue {
			ct := s.controlTable(formCap, ctrlCap)
			if ct == nil {
				return lua.LNil
			}
			if assistantText != "" {
				bindings.AppendAIChatMessage(L, ct, "assistant", assistantText, "")
			}
			ct.RawSetString("ai_busy", lua.LFalse)
			ct.RawSetString("value", lua.LString(assistantText))
			return lua.LNil
		})

		// Remove the cancel entry.
		s.aiTurnMu.Lock()
		delete(s.aiTurns, key)
		s.aiTurnMu.Unlock()

		// Turn finished.
		s.SendOutbox(common.OutboxMsg{Type: "ai_chat_done", Form: formCap, Ctrl: ctrlCap})
		s.flushOutbox()

		// Post Lua handler events on the actor: on_reply/on_error, then on_done.
		// on_reply carries the assistant text; the transcript table is rebuilt
		// on the actor by postAIChatEvent. on_error carries the error message.
		post := func(ev string, args []lua.LValue) {
			select {
			case s.inbox <- inboxMsg{typ: inboxAIChatEvent, form: formCap, ctrl: ctrlCap,
				event: ev, args: args}:
			case <-s.done:
				return // session closing; the actor is gone
			}
		}
		if turnErr != nil {
			post("on_error", []lua.LValue{lua.LString(turnErr.Error())})
		} else if assistantText != "" {
			post("on_reply", []lua.LValue{lua.LString(assistantText)})
		}
		post("on_done", nil)
	}()
}

// handleAIChatControlStop cancels the in-flight turn for the control. Partial
// assistant text is kept.
func (s *Session) handleAIChatControlStop(form, ctrl string, logger Logger) {
	key := form + "\x00" + ctrl
	s.aiTurnMu.Lock()
	turn, exists := s.aiTurns[key]
	if exists {
		delete(s.aiTurns, key)
	}
	s.aiTurnMu.Unlock()
	if exists && turn.cancel != nil {
		turn.cancel()
	}
	// Mark not busy so the control can be used again (we are on the actor
	// goroutine here, so touch s.L directly — Query would deadlock).
	if ct := s.controlTable(form, ctrl); ct != nil {
		ct.RawSetString("ai_busy", lua.LFalse)
	}
	s.SendOutbox(common.OutboxMsg{Type: "ai_chat_done", Form: form, Ctrl: ctrl})
	s.flushOutbox()
}

// executeAIToolControl asks the actor to run one tool call to completion,
// allowing the tool to suspend (e.g. k.db_select). Blocks until the actor
// delivers the result or the session ends.
func (s *Session) executeAIToolControl(form, ctrl, name string, args map[string]interface{}, logger Logger) (interface{}, error) {
	_ = form
	_ = ctrl
	reply := make(chan toolResult, 1)
	select {
	case s.inbox <- inboxMsg{
		typ:       inboxAIToolCall,
		form:      form,
		ctrl:      ctrl,
		data:      toolCall{Name: name, Args: args},
		replyChan: reply,
	}:
	case <-s.done:
		return nil, errSessionClosed
	}
	select {
	case r := <-reply:
		return r.Value, r.Err
	case <-s.done:
		return nil, errSessionClosed
	}
}

// handleAIToolCall runs one tool call on the actor goroutine. The tool is an
// explicit k.ai.tool override or an eligible Lua global. If the tool yields
// (e.g. calls k.db_select), its coroutine is parked and resumed later via
// handleAsyncDone, which forwards the result through pendingToolCalls.
func (s *Session) handleAIToolCall(msg inboxMsg, logger Logger) {
	tc, ok := msg.data.(toolCall)
	if !ok {
		msg.replyChan <- toolResult{Err: fmt.Errorf("invalid tool call")}
		return
	}

	fn := s.resolveAITool(tc.Name)
	if fn == nil {
		msg.replyChan <- toolResult{Err: fmt.Errorf("tool %q not found", tc.Name)}
		return
	}

	argsTbl := s.L.NewTable()
	for k, v := range tc.Args {
		argsTbl.RawSetString(k, common.GoValueToLua(s.L, v))
	}

	co, cancel := s.L.NewThread()
	s.aiToolMu.Lock()
	s.pendingToolCalls[co] = msg.replyChan
	s.aiToolMu.Unlock()

	st, err, rets := s.L.Resume(co, fn, argsTbl)
	switch st {
	case lua.ResumeOK:
		var ret lua.LValue = lua.LNil
		if len(rets) > 0 {
			ret = rets[0]
		}
		s.aiToolMu.Lock()
		delete(s.pendingToolCalls, co)
		s.aiToolMu.Unlock()
		msg.replyChan <- toolResult{Value: luaValueToGo(ret)}
	case lua.ResumeYield:
		// Suspended (e.g. k.db_select). pendingToolCalls[co] holds the reply
		// channel; handleAsyncDone forwards the result when the tool resumes.
	case lua.ResumeError:
		s.aiToolMu.Lock()
		delete(s.pendingToolCalls, co)
		s.aiToolMu.Unlock()
		if cancel != nil {
			cancel()
		}
		msg.replyChan <- toolResult{Err: err}
	}
}

// resolveAITool returns the tool implementation: an explicit k.ai.tool override
// (any Lua function) or an eligible auto-exposed Lua global (Proto != nil).
func (s *Session) resolveAITool(name string) *lua.LFunction {
	if toolsReg := s.L.GetGlobal("k.ai.tools"); toolsReg != lua.LNil {
		if toolsTbl, ok := toolsReg.(*lua.LTable); ok {
			if entry := toolsTbl.RawGetString(name); entry != lua.LNil {
				if et, ok := entry.(*lua.LTable); ok {
					if fn := et.RawGetString("fn"); fn != lua.LNil {
						if lfn, ok := fn.(*lua.LFunction); ok {
							return lfn
						}
					}
				}
			}
		}
	}
	fnVal := s.L.GetGlobal(name)
	if lfn, ok := fnVal.(*lua.LFunction); ok && lfn != nil && lfn.Proto != nil {
		return lfn
	}
	return nil
}

// postAIChatEvent runs a control-scoped Lua AI chat event handler on the
// actor. Events: on_reply, on_tool, on_error, on_done.
func (s *Session) postAIChatEvent(msg inboxMsg, logger Logger) {
	args := msg.args
	if msg.event == "on_reply" {
		// on_reply carries the assistant text followed by the full transcript
		// (rebuilt here on the actor so no LState escapes the worker goroutine).
		if len(args) > 0 {
			if ct := s.controlTable(msg.form, msg.ctrl); ct != nil {
				if tt := ct.RawGetString("ai_transcript"); tt != lua.LNil {
					if ttbl, ok := tt.(*lua.LTable); ok {
						msgs := bindings.LuaTableToTranscript(s.L, ttbl)
						args = append(args, TranscriptToLuaTable(s.L, msgs))
					}
				}
			}
		}
	} else if msg.event == "on_tool" && len(args) >= 3 {
		// on_tool(name, argsJSON, resultJSON) — parse the JSON payloads into
		// Lua tables on the actor.
		name := ""
		if v, ok := args[0].(lua.LString); ok {
			name = string(v)
		}
		argsTbl, resultTbl := s.L.NewTable(), s.L.NewTable()
		if v, ok := args[1].(lua.LString); ok && string(v) != "" {
			var m map[string]interface{}
			if json.Unmarshal([]byte(string(v)), &m) == nil {
				if t := s.toLuaValue(m); t != lua.LNil {
					if tt, ok := t.(*lua.LTable); ok {
						argsTbl = tt
					}
				}
			}
		}
		if v, ok := args[2].(lua.LString); ok && string(v) != "" {
			var m map[string]interface{}
			if json.Unmarshal([]byte(string(v)), &m) == nil {
				if t := s.toLuaValue(m); t != lua.LNil {
					if tt, ok := t.(*lua.LTable); ok {
						resultTbl = tt
					}
				}
			}
		}
		args = []lua.LValue{lua.LString(name), argsTbl, resultTbl}
	}
	s.runFormHandler(msg.form, msg.ctrl, msg.event, args, logger)
	s.flushOutbox()
}

// cancelAIChatTurns cancels all in-flight turns whose key starts with the given
// form prefix (used on close_form / teardown).
func (s *Session) cancelAIChatTurns(form string) {
	s.aiTurnMu.Lock()
	for key, turn := range s.aiTurns {
		if strings.HasPrefix(key, form+"\x00") {
			delete(s.aiTurns, key)
			if turn.cancel != nil {
				turn.cancel()
			}
		}
	}
	s.aiTurnMu.Unlock()
}

// cancelAllAITurns cancels every in-flight AI chat turn (used on session
// Close / teardown so worker goroutines release their reply channels and exit).
func (s *Session) cancelAllAITurns() {
	s.aiTurnMu.Lock()
	for key, turn := range s.aiTurns {
		delete(s.aiTurns, key)
		if turn.cancel != nil {
			turn.cancel()
		}
	}
	s.aiTurnMu.Unlock()
}

// collectAIChatSchemas builds the tool schemas for a control-scoped turn based
// on the control's ai_tools_opt (false | true | allowlist | explicit schemas)
// merged with k.ai.tools overrides.
func collectAIChatSchemas(L *lua.LState, ctrlTbl *lua.LTable) []common.ToolSchema {
	toolsOpt := ctrlTbl.RawGetString("ai_tools_opt")

	// Explicit schema table: [{name, desc, params}, ...].
	if tt, ok := toolsOpt.(*lua.LTable); ok && tt.Len() > 0 {
		var first *lua.LTable
		tt.ForEach(func(_, v lua.LValue) {
			if first == nil {
				if vt, ok := v.(*lua.LTable); ok {
					first = vt
				}
			}
		})
		if first != nil && first.RawGetString("name") != lua.LNil {
			// Explicit schemas.
			schemas := make([]common.ToolSchema, 0, tt.Len())
			tt.ForEach(func(_, v lua.LValue) {
				schemas = append(schemas, bindings.AIToolSchemaFromLua(v.(*lua.LTable)))
			})
			return schemas
		}
		// Allowlist of names.
		names := make([]string, 0, tt.Len())
		tt.ForEach(func(_, v lua.LValue) {
			names = append(names, v.String())
		})
		return allowlistedSchemas(L, names)
	}

	enabled := false
	if b := lua.LVAsBool(toolsOpt); b {
		enabled = true
	}
	if !enabled {
		return nil
	}

	// Auto-expose eligible globals.
	schemas := ai.CollectSchemas(L, bindings.SandboxLibNames())
	// Layer k.ai.tools overrides on top (by name).
	if toolsReg := L.GetGlobal("k.ai.tools"); toolsReg != lua.LNil {
		if toolsTbl, ok := toolsReg.(*lua.LTable); ok {
			byName := map[string]common.ToolSchema{}
			for _, s := range schemas {
				byName[s.Name] = s
			}
			toolsTbl.ForEach(func(_, v lua.LValue) {
				if entry, ok := v.(*lua.LTable); ok {
					appendOrOverride(byName, &schemas, bindings.AIToolSchemaFromLua(entry))
				}
			})
		}
	}
	return schemas
}

// appendOrOverride adds or replaces a schema by name.
func appendOrOverride(byName map[string]common.ToolSchema, schemas *[]common.ToolSchema, s common.ToolSchema) {
	if _, exists := byName[s.Name]; exists {
		for i := range *schemas {
			if (*schemas)[i].Name == s.Name {
				(*schemas)[i] = s
				return
			}
		}
	} else {
		byName[s.Name] = s
		*schemas = append(*schemas, s)
	}
}

// allowlistedSchemas returns schemas for the named globals/overrides.
func allowlistedSchemas(L *lua.LState, names []string) []common.ToolSchema {
	schemas := make([]common.ToolSchema, 0, len(names))
	all := ai.CollectSchemas(L, bindings.SandboxLibNames())
	byName := map[string]common.ToolSchema{}
	for _, s := range all {
		byName[s.Name] = s
	}
	// k.ai.tools overrides.
	if toolsReg := L.GetGlobal("k.ai.tools"); toolsReg != lua.LNil {
		if toolsTbl, ok := toolsReg.(*lua.LTable); ok {
			toolsTbl.ForEach(func(_, v lua.LValue) {
				if entry, ok := v.(*lua.LTable); ok {
					byName[bindings.AIToolSchemaFromLua(entry).Name] = bindings.AIToolSchemaFromLua(entry)
				}
			})
		}
	}
	for _, n := range names {
		if s, ok := byName[n]; ok {
			schemas = append(schemas, s)
		}
	}
	return schemas
}

// TranscriptToLuaTable converts []common.ChatMessage into a Lua array table.
func TranscriptToLuaTable(L *lua.LState, msgs []common.ChatMessage) *lua.LTable {
	tbl := L.NewTable()
	for i, m := range msgs {
		mt := L.NewTable()
		mt.RawSetString("role", lua.LString(m.Role))
		mt.RawSetString("content", lua.LString(m.Content))
		if m.Name != "" {
			mt.RawSetString("name", lua.LString(m.Name))
		}
		tbl.RawSetInt(i+1, mt)
	}
	return tbl
}
