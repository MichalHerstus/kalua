//go:build !wasm

package bindings

import (
	"strings"

	"github.com/yuin/gopher-lua"

	"kalua/internal/common"
	"kalua/internal/vm"
)

// ResolveAIConfig resolves AI connection settings: explicit opt > env/INI >
// defaults. Used by k.ctrl.ai_chat, which never fails construction: an
// unresolved endpoint simply makes the first turn fire on_error.
func ResolveAIConfig(e *Env, baseURL, apiKey, model string) (string, string, string) {
	if e != nil && e.aiConfig != nil {
		if baseURL == "" {
			baseURL = e.aiConfig.BaseURL
		}
		if apiKey == "" {
			apiKey = e.aiConfig.APIKey
		}
		if model == "" {
			model = e.aiConfig.Model
		}
	}
	if model == "" {
		model = "local-model"
	}
	if baseURL == "" {
		baseURL = "http://localhost:1234/v1"
	}
	return baseURL, apiKey, model
}

// registerAIChat installs k.form.ai_agent and k.ai.tool bindings.
func registerAIChat(e *Env) {
	e.register("form.ai_agent", "forms", func(L *lua.LState) int {
		if e.Sess == nil {
			return e.fail(L, KErrorInvalidParam, "form.ai_agent: no session available")
		}

		opts := L.OptTable(1, L.NewTable())

		title := "AI Assistant"
		if v := opts.RawGetString("title"); v != lua.LNil {
			title = v.String()
		}
		welcome := ""
		if v := opts.RawGetString("welcome"); v != lua.LNil {
			welcome = v.String()
		}
		system := ""
		if v := opts.RawGetString("system"); v != lua.LNil {
			system = v.String()
		}
		maxTurns := 8
		if v := opts.RawGetString("max_turns"); v != lua.LNil {
			maxTurns = int(lua.LVAsNumber(v))
			if maxTurns < 1 {
				maxTurns = 1
			}
		}
		apiKey := ""
		if v := opts.RawGetString("api_key"); v != lua.LNil {
			apiKey = v.String()
		}
		baseURL := ""
		if v := opts.RawGetString("base_url"); v != lua.LNil {
			baseURL = v.String()
		}
		model := ""
		if v := opts.RawGetString("model"); v != lua.LNil {
			model = v.String()
		}

		// Require AI configuration from env/INI, or an explicit api_key opt.
		if e.aiConfig == nil && apiKey == "" {
			return e.fail(L, KErrorInvalidParam, "form.ai_agent: AI not configured (set [AI] in KALUA.INI or KALUA_AI_* env, or pass api_key)")
		}

		// Fall back to env/INI config for any opt not explicitly provided.
		if e.aiConfig != nil {
			if baseURL == "" {
				baseURL = e.aiConfig.BaseURL
			}
			if apiKey == "" {
				apiKey = e.aiConfig.APIKey
			}
			if model == "" {
				model = e.aiConfig.Model
			}
		}

		// Default model (mirrors ai.EnvConfig).
		if model == "" {
			model = "local-model"
		}
		if baseURL == "" {
			baseURL = "http://localhost:1234/v1"
		}

		// Materialize the __kai_chat form table (hidden form that carries
		// the chat state and transcript). The script never sees this name.
		chatForm := "__kai_chat"
		chatTbl := L.NewTable()
		chatTbl.RawSetString("name", lua.LString(chatForm))
		chatTbl.RawSetString("title", lua.LString(title))
		chatTbl.RawSetString("layout", lua.LString("vertical"))
		chatTbl.RawSetString("controls", L.NewTable())
		chatTbl.RawSetString("handlers", L.NewTable())
		L.SetGlobal(chatForm, chatTbl)

		// Store options on the form so session can read them back.
		chatTbl.RawSetString("ai_title", lua.LString(title))
		chatTbl.RawSetString("ai_welcome", lua.LString(welcome))
		chatTbl.RawSetString("ai_system", lua.LString(system))
		chatTbl.RawSetString("ai_max_turns", lua.LNumber(maxTurns))
		chatTbl.RawSetString("ai_api_key", lua.LString(apiKey))
		chatTbl.RawSetString("ai_base_url", lua.LString(baseURL))
		chatTbl.RawSetString("ai_model", lua.LString(model))

		// Collect tool schemas from script globals (auto-exposure).
		schemas := collectAISchemas(L)
		if len(schemas) > 0 {
			schemaTbl := L.NewTable()
			for i, s := range schemas {
				st := L.NewTable()
				st.RawSetString("name", lua.LString(s.Name))
				st.RawSetString("desc", lua.LString(s.Desc))
				pt := L.NewTable()
				for j, p := range s.Params {
					pt.RawSetInt(j+1, luaValueToParamTable(L, p))
				}
				st.RawSetString("params", pt)
				schemaTbl.RawSetInt(i+1, st)
			}
			chatTbl.RawSetString("ai_tools", schemaTbl)
		}

		// Push form onto session stack and store suspended coroutine.
		sess := e.Sess
		sess.PushForm(chatForm)
		sess.StoreFormCoro(chatForm, L)

		// Build initial assistant message if welcome provided.
		transcript := L.NewTable()
		if welcome != "" {
			m := L.NewTable()
			m.RawSetString("role", lua.LString("assistant"))
			m.RawSetString("content", lua.LString(welcome))
			transcript.RawSetInt(1, m)
		}
		chatTbl.RawSetString("ai_transcript", transcript)

		// Render the chat form HTML and send as a modal.
		html := renderAIChatForm(chatForm, title, welcome, apiKey)
		sendOutbox(e, common.OutboxMsg{
			Type:  "ai_chat_open",
			Form:  chatForm,
			HTML:  html,
			Modal: true,
		})

		// Suspend with PendingFormShowValue so the coroutine resumes with
		// the transcript table (not LNil like form.show).
		return e.App.Block(L, &vm.PendingOp{Kind: vm.PendingFormShowValue, Form: chatForm, Resume: transcript})
	})

	// k.ai.tool(name, schema, fn) — register/override a tool for the agent.
	e.register("ai.tool", "ai", func(L *lua.LState) int {
		name := L.CheckString(1)
		schema := L.OptTable(2, L.NewTable())
		fn := L.OptFunction(3, nil)

		// Store the tool override on the k.ai.tools registry global.
		toolsReg := L.GetGlobal("k.ai.tools")
		if toolsReg == lua.LNil {
			toolsReg = L.NewTable()
			L.SetGlobal("k.ai.tools", toolsReg)
		}
		toolsTbl, _ := toolsReg.(*lua.LTable)

		entry := L.NewTable()
		entry.RawSetString("name", lua.LString(name))
		if desc := schema.RawGetString("desc"); desc != lua.LNil {
			entry.RawSetString("desc", desc)
		}
		if params := schema.RawGetString("params"); params != lua.LNil {
			entry.RawSetString("params", params)
		}
		if fn != nil {
			entry.RawSetString("fn", fn)
		}

		toolsTbl.RawSetString(name, entry)
		return 0
	})

	// k.ctrl.ai_chat(form, name, opts) — embeddable AI chat control (§12.2).
	// Non-blocking and event-driven: renders inside any form and never blocks
	// the script. AI configuration is resolved lazily on the first turn.
	e.register("ctrl.ai_chat", "controls", func(L *lua.LState) int {
		if e.Sess == nil {
			return e.fail(L, KErrorInvalidParam, "ctrl.ai_chat: no session available")
		}
		formName := L.CheckString(1)
		name := L.CheckString(2)
		opts := L.OptTable(3, L.NewTable())

		optStr := func(key string) string {
			if v := opts.RawGetString(key); v != lua.LNil {
				return v.String()
			}
			return ""
		}
		optBool := func(key string, def bool) bool {
			if v := opts.RawGetString(key); v != lua.LNil {
				return lua.LVAsBool(v)
			}
			return def
		}

		addControl(L, formName, name, "ai_chat", opts)

		ft := L.GetGlobal(formName).(*lua.LTable)
		controls := ft.RawGetString("controls").(*lua.LTable)
		ctrl := controls.RawGetString(name).(*lua.LTable)

		title := optStr("title")
		if title == "" {
			title = "AI Assistant"
		}
		welcome := optStr("welcome")
		placeholder := optStr("placeholder")
		if placeholder == "" {
			placeholder = "Ask me anything…"
		}
		system := optStr("system")
		maxTurns := 8
		if v := opts.RawGetString("max_turns"); v != lua.LNil {
			maxTurns = int(lua.LVAsNumber(v))
			if maxTurns < 1 {
				maxTurns = 1
			}
		}
		rawTools := opts.RawGetString("tools")
		height := optStr("height")
		showHeader := optBool("show_header", true)
		showAPIKey := optBool("show_apikey", false)
		markdown := optBool("markdown", true)
		disabled := optBool("disabled", false)
		baseURL, apiKey, model := ResolveAIConfig(e, optStr("base_url"), optStr("api_key"), optStr("model"))

		ctrl.RawSetString("ai_title", lua.LString(title))
		ctrl.RawSetString("ai_welcome", lua.LString(welcome))
		ctrl.RawSetString("ai_placeholder", lua.LString(placeholder))
		ctrl.RawSetString("ai_system", lua.LString(system))
		ctrl.RawSetString("ai_max_turns", lua.LNumber(maxTurns))
		ctrl.RawSetString("ai_tools_opt", rawTools)
		ctrl.RawSetString("ai_base_url", lua.LString(baseURL))
		ctrl.RawSetString("ai_api_key", lua.LString(apiKey))
		ctrl.RawSetString("ai_model", lua.LString(model))
		ctrl.RawSetString("ai_markdown", lua.LBool(markdown))
		if height != "" {
			ctrl.RawSetString("ai_height", lua.LString(height))
		}
		ctrl.RawSetString("ai_show_header", lua.LBool(showHeader))
		ctrl.RawSetString("ai_show_apikey", lua.LBool(showAPIKey))
		if disabled {
			ctrl.RawSetString("enabled", lua.LString("false"))
		}
		ctrl.RawSetString("ai_busy", lua.LFalse)

		// Transcript seeded with the welcome message.
		transcript := L.NewTable()
		if welcome != "" {
			m := L.NewTable()
			m.RawSetString("role", lua.LString("assistant"))
			m.RawSetString("content", lua.LString(welcome))
			transcript.RawSetInt(1, m)
		}
		ctrl.RawSetString("ai_transcript", transcript)

		// Register the on_send handler if provided.
		if fn := opts.RawGetString("on_send"); fn != lua.LNil {
			if lfn, ok := fn.(*lua.LFunction); ok {
				registerControlHandler(L, ft, name, "on_send", lfn)
			}
		}

		return 0
	})

	registerAIChatOps(e)
}

// luaValueToParamTable converts a common.Param to a Lua table.
func luaValueToParamTable(L *lua.LState, p common.Param) *lua.LTable {
	t := L.NewTable()
	t.RawSetString("name", lua.LString(p.Name))
	t.RawSetString("type", lua.LString(p.Type))
	t.RawSetString("required", lua.LBool(p.Required))
	if p.Desc != "" {
		t.RawSetString("desc", lua.LString(p.Desc))
	}
	return t
}

// collectAISchemas collects tool schemas from script globals (auto-exposure),
// using only common types so bindings does not import the ai package.
func collectAISchemas(L *lua.LState) []common.ToolSchema {
	schemas := make([]common.ToolSchema, 0)

	libNames := SandboxLibNames()

	// Iterate globals via the _G table.
	globals := L.GetGlobal("_G")
	gt, ok := globals.(*lua.LTable)
	if !ok {
		return schemas
	}
	gt.ForEach(func(k lua.LValue, v lua.LValue) {
		name := k.String()
		if name == "" {
			return
		}
		if name == "main" {
			return
		}
		if strings.HasPrefix(name, "_") {
			return
		}
		if libNames[name] {
			return
		}

		fn, ok := v.(*lua.LFunction)
		if !ok || fn == nil {
			return
		}
		if fn.Proto == nil {
			return
		}

		schema := common.ToolSchema{Name: name}

		proto := fn.Proto
		numParams := int(proto.NumParameters)
		paramNames := make([]string, 0, numParams)
		for i := 0; i < numParams; i++ {
			if i < len(proto.DbgLocals) {
				paramNames = append(paramNames, proto.DbgLocals[i].Name)
			} else {
				paramNames = append(paramNames, "arg")
			}
		}
		if (proto.IsVarArg & lua.VarArgIsVarArg) != 0 {
			paramNames = append(paramNames, "...")
		}

		schema.Desc = "" // comment extraction not available without source access
		for _, pname := range paramNames {
			schema.Params = append(schema.Params, common.Param{Name: pname, Type: "string", Required: pname != "..."})
		}

		schemas = append(schemas, schema)
		return
	})

	return schemas
}

// SandboxLibNames returns the set of global names that are NOT auto-exposed
// as tools.
func SandboxLibNames() map[string]bool {
	return map[string]bool{
		"k":              true,
		"K":              true,
		"CTRL":           true,
		"ARGS":           true,
		"main":           true,
		"string":         true,
		"table":          true,
		"math":           true,
		"os":             true,
		"io":             true,
		"debug":          true,
		"coroutine":      true,
		"_G":             true,
		"_VERSION":       true,
		"assert":         true,
		"collectgarbage": true,
		"dofile":         true,
		"error":          true,
		"getmetatable":   true,
		"ipairs":         true,
		"load":           true,
		"loadfile":       true,
		"loadstring":     true,
		"next":           true,
		"pairs":          true,
		"pcall":          true,
		"print":          true,
		"rawequal":       true,
		"rawget":         true,
		"rawset":         true,
		"require":        true,
		"select":         true,
		"setmetatable":   true,
		"tonumber":       true,
		"tostring":       true,
		"type":           true,
		"xpcall":         true,
	}
}

// renderAIChatForm renders the HTML for the AI chat form shell.
// If apiKey is non-empty, it is pre-filled in the API key input.
func renderAIChatForm(formName, title, welcome, apiKey string) string {
	apiKeyInput := ""
	if apiKey != "" {
		apiKeyInput = `
			<div class="kalua-ai-apikey-row">
				<label class="kalua-ai-apikey-label">API Key:</label>
				<input type="password" class="kalua-ai-apikey-input" id="` + escAttr(formName) + `-ai-apikey"
				       value="` + escAttr(apiKey) + `"
				       placeholder="sk-…" data-k-ai-apikey="1">
			</div>`
	}
	return `<div class="kalua-ai-chat" id="` + escAttr(formName) + `-ai-chat"
	     data-k-ai-chat="1" data-k-form="` + escAttr(formName) + `">
		<div class="kalua-ai-header">
			<div class="kalua-ai-title">` + escText(title) + `</div>
			<button class="kalua-ai-close" data-k-ai-close="1" data-k-form="` + escAttr(formName) + `">✕</button>
		</div>
		<div class="kalua-ai-transcript" id="` + escAttr(formName) + `-ai-transcript"></div>
		` + apiKeyInput + `
		<div class="kalua-ai-input-bar">
			<input type="text" class="kalua-ai-input" id="` + escAttr(formName) + `-ai-input"
			       placeholder="Ask me anything…" data-k-ai-input="1">
			<button class="kalua-ai-send" data-k-ai-send="1" data-k-form="` + escAttr(formName) + `">Send</button>
		</div>
		<div class="kalua-ai-status" id="` + escAttr(formName) + `-ai-status" style="display:none;"></div>
	</div>`
}

// AppendAITranscript appends a message to the chat form's transcript.
func AppendAITranscript(L *lua.LState, formName, role, content, name string) {
	formTbl := L.GetGlobal(formName)
	if formTbl == lua.LNil {
		return
	}
	ft, ok := formTbl.(*lua.LTable)
	if !ok {
		return
	}
	t := ft.RawGetString("ai_transcript")
	if t == lua.LNil {
		t = L.NewTable()
		ft.RawSetString("ai_transcript", t)
	}
	tt, _ := t.(*lua.LTable)
	if tt == nil {
		tt = L.NewTable()
		ft.RawSetString("ai_transcript", tt)
	}
	m := L.NewTable()
	m.RawSetString("role", lua.LString(role))
	m.RawSetString("content", lua.LString(content))
	if name != "" {
		m.RawSetString("name", lua.LString(name))
	}
	tt.RawSetInt(tt.Len()+1, m)
}

// LuaTableToTranscript converts a Lua transcript table (array of {role,
// content, name?}) into []common.ChatMessage.
func LuaTableToTranscript(L *lua.LState, tbl *lua.LTable) []common.ChatMessage {
	var msgs []common.ChatMessage
	tbl.ForEach(func(k, v lua.LValue) {
		if _, ok := k.(lua.LNumber); !ok {
			return
		}
		mt, ok := v.(*lua.LTable)
		if !ok {
			return
		}
		role := ""
		if rv := mt.RawGetString("role"); rv != lua.LNil {
			role = rv.String()
		}
		content := ""
		if cv := mt.RawGetString("content"); cv != lua.LNil {
			content = cv.String()
		}
		name := ""
		if nv := mt.RawGetString("name"); nv != lua.LNil {
			name = nv.String()
		}
		msgs = append(msgs, common.ChatMessage{Role: role, Content: content, Name: name})
	})
	return msgs
}

// AIToolSchemaFromLua builds a common.ToolSchema from a Lua tool table.
func AIToolSchemaFromLua(entry *lua.LTable) common.ToolSchema {
	s := common.ToolSchema{}
	if nv := entry.RawGetString("name"); nv != lua.LNil {
		s.Name = nv.String()
	}
	if dv := entry.RawGetString("desc"); dv != lua.LNil {
		s.Desc = dv.String()
	}
	if pv := entry.RawGetString("params"); pv != lua.LNil {
		if pt, ok := pv.(*lua.LTable); ok {
			pt.ForEach(func(_, pv2 lua.LValue) {
				pt2, ok := pv2.(*lua.LTable)
				if !ok {
					return
				}
				p := common.Param{}
				if nv := pt2.RawGetString("name"); nv != lua.LNil {
					p.Name = nv.String()
				}
				if tv := pt2.RawGetString("type"); tv != lua.LNil {
					p.Type = tv.String()
				}
				rv := pt2.RawGetString("required")
				p.Required = rv != lua.LNil && lua.LVAsBool(rv)
				if dv := pt2.RawGetString("desc"); dv != lua.LNil {
					p.Desc = dv.String()
				}
				s.Params = append(s.Params, p)
			})
		}
	}
	return s
}

// AppendAIChatMessage appends a {role, content, name?} message to a control's
// ai_transcript (control-scoped transcript, used by k.ctrl.ai_chat).
func AppendAIChatMessage(L *lua.LState, ct *lua.LTable, role, content, name string) {
	t := ct.RawGetString("ai_transcript")
	if t == lua.LNil {
		t = L.NewTable()
		ct.RawSetString("ai_transcript", t)
	}
	tt, _ := t.(*lua.LTable)
	if tt == nil {
		tt = L.NewTable()
		ct.RawSetString("ai_transcript", tt)
	}
	m := L.NewTable()
	m.RawSetString("role", lua.LString(role))
	m.RawSetString("content", lua.LString(content))
	if name != "" {
		m.RawSetString("name", lua.LString(name))
	}
	tt.RawSetInt(tt.Len()+1, m)
}

// sanitizeName ensures a tool name is safe for use in a tagged block.
func sanitizeName(name string) string {
	if name == "" {
		return ""
	}
	out := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			return r
		}
		return '_'
	}, name)
	return out
}
