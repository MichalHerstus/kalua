//go:build !wasm

package session

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yuin/gopher-lua"

	"kalua/internal/bindings"
	"kalua/internal/common"
)

// sseFrame wraps text in an OpenAI-compatible streaming chunk.
func sseFrame(text string) string {
	b, _ := json.Marshal(map[string]any{"choices": []any{
		map[string]any{"delta": map[string]any{"content": text}},
	}})
	return "data: " + string(b) + "\n\n"
}

// mockLLM streams each entry of responses (one per completion call) as SSE.
func mockLLM(t *testing.T, responses ...string) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	calls := 0
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		idx := calls
		calls++
		mu.Unlock()
		resp := "ok"
		if idx < len(responses) {
			resp = responses[idx]
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		for _, tok := range strings.Split(resp, " ") {
			if tok == "" {
				continue
			}
			_, _ = w.Write([]byte(sseFrame(tok + " ")))
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
}

// aiChatScript builds a script with an ai_chat control and handlers that record
// events into globals so the test can inspect them after the turn.
func aiChatScript(toolsOpt string, handlers string, extra string) string {
	return `
function fill_customer(id)
  local customers = {
    { id = 7,  name = "Ada Lovelace",  city = "London" },
    { id = 12, name = "Grace Hopper",  city = "New York" },
  }
  for _, c in ipairs(customers) do
    if c.id == id then return c end
  end
  return { error = "no customer " .. tostring(id) }
end

function main()
  k.form.new("main", {title="t", layout="vertical"})
  k.ctrl.ai_chat("main", "chat", {` + toolsOpt + `})
  k.ctrl.textbox("main", "status", {label="Status"})
  _turn_done = false
  _reply_text = ""
  _reply_msgs = 0
  _tool_calls = 0
  _last_tool = ""
  _err = ""

  ` + handlers + `

  ` + extra + `
  k.form.show("main", {modal=false})
end
`
}

func outboxChan(s *Session) chan outboxWire {
	out := make(chan outboxWire, 64)
	go func() {
		for msg := range s.Outbox() {
			out <- outboxWire{Type: msg.Type, Selector: msg.Selector, Data: msg.Data, Form: msg.Form, Ctrl: msg.Ctrl}
		}
	}()
	return out
}

// waitGlobal polls a string global until it equals want or timeout.
func waitGlobal(t *testing.T, s *Session, name, want string, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		got := s.Query(func(L *lua.LState) lua.LValue {
			return L.GetGlobal(name)
		})
		if got.String() == want {
			return true
		}
		time.Sleep(15 * time.Millisecond)
	}
	t.Logf("waitGlobal(%s) timed out; last value = %q", name,
		s.Query(func(L *lua.LState) lua.LValue { return L.GetGlobal(name) }).String())
	return false
}

// waitInt polls a number global until it equals want.
func waitInt(t *testing.T, s *Session, name string, want int, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		got := s.Query(func(L *lua.LState) lua.LValue {
			return L.GetGlobal(name)
		})
		if int(lua.LVAsNumber(got)) == want {
			return true
		}
		time.Sleep(15 * time.Millisecond)
	}
	return false
}

// TestAIChatToolsOption exercises the tools normalization (unit, no session).
func TestAIChatToolsOption(t *testing.T) {
	L := lua.NewState()
	defer L.Close()
	bindings.Setup(L, nil, bindings.Options{}, nil, tLogger{t: t})

	makeCtrl := func(opt lua.LValue) *lua.LTable {
		ct := L.NewTable()
		ct.RawSetString("type", lua.LString("ai_chat"))
		ct.RawSetString("form", lua.LString("f"))
		ct.RawSetString("name", lua.LString("c"))
		if opt != lua.LNil {
			ct.RawSetString("ai_tools_opt", opt)
		}
		return ct
	}

	// nil/false → no tools.
	if got := collectAIChatSchemas(L, makeCtrl(lua.LNil)); len(got) != 0 {
		t.Errorf("nil tools = %d schemas, want 0", len(got))
	}
	ct := makeCtrl(lua.LFalse)
	if got := collectAIChatSchemas(L, ct); len(got) != 0 {
		t.Errorf("false tools = %d schemas, want 0", len(got))
	}

	// true → auto-exposes eligible Lua globals (main excluded).
	if err := L.DoString(`function helper() end`); err != nil {
		t.Fatal(err)
	}
	L.SetGlobal("main", L.NewFunction(func(l *lua.LState) int { return 0 }))
	ct = makeCtrl(lua.LTrue)
	schemas := collectAIChatSchemas(L, ct)
	found := false
	for _, s := range schemas {
		if s.Name == "helper" {
			found = true
		}
	}
	if !found {
		t.Errorf("true tools should auto-expose helper, got %v", schemas)
	}

	// Allowlist: only helper.
	toolsTbl := L.NewTable()
	toolsTbl.RawSetInt(1, lua.LString("helper"))
	ct = makeCtrl(toolsTbl)
	schemas = collectAIChatSchemas(L, ct)
	if len(schemas) != 1 || schemas[0].Name != "helper" {
		t.Errorf("allowlist = %v, want [helper]", schemas)
	}

	// Explicit schema table.
	toolsTbl = L.NewTable()
	et := L.NewTable()
	et.RawSetString("name", lua.LString("custom"))
	et.RawSetString("desc", lua.LString("a custom tool"))
	toolsTbl.RawSetInt(1, et)
	ct = makeCtrl(toolsTbl)
	schemas = collectAIChatSchemas(L, ct)
	if len(schemas) != 1 || schemas[0].Name != "custom" || schemas[0].Desc != "a custom tool" {
		t.Errorf("explicit = %v, want [custom]", schemas)
	}
}

// TestAIChatControlTurn runs a full turn against a fake LLM: status, ordered
// deltas, done, transcript, value and on_reply.
func TestAIChatControlTurn(t *testing.T) {
	srv := mockLLM(t, "Hello world")
	defer srv.Close()

	script := aiChatScript(`title="C", ai_base_url="`+srv.URL+`", ai_model="m"`,
		`k.form.on("main", "chat", "on_reply", function(text, messages)
  _reply_text = text
  _reply_msgs = #messages
end)
k.form.on("main", "chat", "on_done", function()
  _turn_done = true
end)`,
		``)

	tmp := t.TempDir()
	scriptPath := writeScript(t, tmp, script)
	s, err := New("t1", scriptPath, bindings.Options{
		AllowFS:  []string{tmp},
		AIConfig: &common.AIConfig{BaseURL: srv.URL, Model: "m"},
	}, tLogger{t: t})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	out := outboxChan(s)
	time.Sleep(200 * time.Millisecond)

	// Send a user message.
	s.PostEventAny("main", "chat", "ai_chat_send", map[string]interface{}{
		"text":    "hi",
		"api_key": "",
	})

	// Expect status, then deltas, then done.
	seenStatus := false
	deltas := 0
	seenDone := false
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case w := <-out:
			switch w.Type {
			case "ai_chat_status":
				seenStatus = true
			case "ai_chat_delta":
				deltas++
			case "ai_chat_done":
				seenDone = true
				goto done
			}
		case <-time.After(100 * time.Millisecond):
			// keep polling
		}
		if seenDone {
			break
		}
	}
done:
	if !seenStatus {
		t.Error("no ai_chat_status")
	}
	if deltas == 0 {
		t.Errorf("no deltas, got %d", deltas)
	}
	if !seenDone {
		t.Error("no ai_chat_done")
	}

	// Turn completed on the actor.
	if !waitGlobal(t, s, "_turn_done", "true", 5*time.Second) {
		t.Error("turn_done not set")
	}
	if got := s.Query(func(L *lua.LState) lua.LValue { return L.GetGlobal("_reply_text") }); got.String() != "Hello world " {
		t.Errorf("reply_text = %q, want 'Hello world '", got.String())
	}
	if got := s.Query(func(L *lua.LState) lua.LValue { return L.GetGlobal("_reply_msgs") }); lua.LVAsNumber(got) < 2 {
		t.Errorf("reply_msgs = %v, want >= 2", got)
	}
	// value = last assistant text.
	if got := s.Query(func(L *lua.LState) lua.LValue {
		ct := s.controlTable("main", "chat")
		return ct.RawGetString("value")
	}); got.String() != "Hello world " {
		t.Errorf("value = %q, want 'Hello world '", got.String())
	}
	// Not busy after turn.
	if v := s.Query(func(L *lua.LState) lua.LValue {
		ct := s.controlTable("main", "chat")
		return ct.RawGetString("ai_busy")
	}); lua.LVAsBool(v) {
		t.Error("still busy after turn")
	}
	_ = srv
}

// TestAIChatControlVeto: on_send returning false performs no HTTP request and
// appends nothing.
func TestAIChatControlVeto(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(sseFrame("nope ")))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	script := aiChatScript(`title="C", ai_base_url="`+srv.URL+`"`,
		`k.form.on("main", "chat", "on_send", function(text)
  return false  -- veto
end)`,
		``)

	tmp := t.TempDir()
	scriptPath := writeScript(t, tmp, script)
	s, err := New("t1", scriptPath, bindings.Options{
		AllowFS:  []string{tmp},
		AIConfig: &common.AIConfig{BaseURL: srv.URL, Model: "m"},
	}, tLogger{t: t})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	time.Sleep(200 * time.Millisecond)
	s.PostEventAny("main", "chat", "ai_chat_send", map[string]interface{}{"text": "hi"})

	time.Sleep(800 * time.Millisecond)
	if hits != 0 {
		t.Errorf("vetoed turn still hit the LLM %d times", hits)
	}
	if got := s.Query(func(L *lua.LState) lua.LValue {
		ct := s.controlTable("main", "chat")
		return ct.RawGetString("ai_transcript")
	}); got.(*lua.LTable).Len() != 0 {
		t.Errorf("vetoed turn appended %d transcript messages, want 0", got.(*lua.LTable).Len())
	}
}

// TestAIChatControlToolsDisabled: default (tools off) sends no tool block; a
// tool-shaped reply is treated as plain text.
func TestAIChatControlToolsDisabled(t *testing.T) {
	srv := mockLLM(t, `function {"name":"db_lookup","arguments":{"id":7}} plain answer`)
	defer srv.Close()

	script := aiChatScript(`title="C", ai_base_url="`+srv.URL+`"`,
		`k.form.on("main", "chat", "on_reply", function(text, messages)
  _reply_text = text
end)
k.form.on("main", "chat", "on_done", function()
  _turn_done = true
end)`,
		``)

	tmp := t.TempDir()
	scriptPath := writeScript(t, tmp, script)
	s, err := New("t1", scriptPath, bindings.Options{
		AllowFS:  []string{tmp},
		AIConfig: &common.AIConfig{BaseURL: srv.URL, Model: "m"},
	}, tLogger{t: t})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	time.Sleep(200 * time.Millisecond)
	s.PostEventAny("main", "chat", "ai_chat_send", map[string]interface{}{"text": "hi"})
	if !waitGlobal(t, s, "_turn_done", "true", 5*time.Second) {
		t.Fatal("turn_done not set")
	}
	if got := s.Query(func(L *lua.LState) lua.LValue { return L.GetGlobal("_reply_text") }); !strings.Contains(got.String(), `function {"name"`) {
		t.Errorf("reply should be plain text (tools off), got %q", got.String())
	}
}

// TestAIChatControlToolLoop: an allowlisted tool is invoked, on_tool fires, and
// the result reaches the model.
func TestAIChatControlToolLoop(t *testing.T) {
	srv := mockLLM(t,
		`function {"name":"fill_customer","arguments":{"id":7}} loading`,
		`Customer loaded.`,
	)
	defer srv.Close()

	script := aiChatScript(`title="C", tools={ "fill_customer" }, ai_base_url="`+srv.URL+`"`,
		`k.form.on("main", "chat", "on_tool", function(name, args, result)
  _tool_calls = _tool_calls + 1
  _last_tool = name
end)
k.form.on("main", "chat", "on_done", function()
  _turn_done = true
end)`,
		``)

	tmp := t.TempDir()
	scriptPath := writeScript(t, tmp, script)
	s, err := New("t1", scriptPath, bindings.Options{
		AllowFS:  []string{tmp},
		AIConfig: &common.AIConfig{BaseURL: srv.URL, Model: "m"},
	}, tLogger{t: t})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	time.Sleep(200 * time.Millisecond)
	s.PostEventAny("main", "chat", "ai_chat_send", map[string]interface{}{"text": "load customer 7"})
	if !waitInt(t, s, "_tool_calls", 1, 8*time.Second) {
		t.Fatalf("on_tool not fired (_tool_calls=%d)", queryInt(t, s, "_tool_calls"))
	}
	if !waitGlobal(t, s, "_turn_done", "true", 5*time.Second) {
		t.Error("turn_done not set")
	}
	if got := s.Query(func(L *lua.LState) lua.LValue { return L.GetGlobal("_last_tool") }); got.String() != "fill_customer" {
		t.Errorf("last_tool = %q, want fill_customer", got.String())
	}
}

// TestAIChatControlToolSuspends: a tool that calls k.db_select (which suspends)
// returns its result to the model via the suspend-aware runner.
func TestAIChatControlToolSuspends(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "test.db")
	srv := mockLLM(t,
		`function {"name":"db_lookup","arguments":{"id":7}} looking up`,
		`Found Ada in London.`,
	)
	defer srv.Close()

	script := fmt.Sprintf(`
function db_lookup(id)
  local db = k.connect_sqlite(%q)
  local rows = k.sql(db, "select id, name, city from customers where id = ?", id)
  k.disconnect_sqlite(db)
  if #rows > 0 then return rows[1] end
  return { error = "not found" }
end

function main()
  local db = k.connect_sqlite(%q)
  k.sql(db, "create table if not exists customers (id integer primary key, name text, city text)")
  k.sql(db, "insert or ignore into customers values (7, 'Ada Lovelace', 'London')")
  k.disconnect_sqlite(db)
  k.form.new("main", {title="t", layout="vertical"})
  k.ctrl.ai_chat("main", "chat", {tools={ "db_lookup" }, ai_base_url="%s"})
  _turn_done = false
  _tool_calls = 0
  k.form.on("main", "chat", "on_tool", function(name, args, result)
    _tool_calls = _tool_calls + 1
  end)
  k.form.on("main", "chat", "on_done", function()
    _turn_done = true
  end)
  k.form.show("main", {modal=false})
end
`, dbPath, dbPath, srv.URL)

	scriptPath := writeScript(t, tmp, script)
	s, err := New("t1", scriptPath, bindings.Options{
		AllowFS:  []string{tmp},
		AIConfig: &common.AIConfig{BaseURL: srv.URL, Model: "m"},
	}, tLogger{t: t})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	time.Sleep(200 * time.Millisecond)
	s.PostEventAny("main", "chat", "ai_chat_send", map[string]interface{}{"text": "find customer 7"})
	if !waitInt(t, s, "_tool_calls", 1, 10*time.Second) {
		t.Fatalf("on_tool not fired (_tool_calls=%d)", queryInt(t, s, "_tool_calls"))
	}
	if !waitGlobal(t, s, "_turn_done", "true", 5*time.Second) {
		t.Error("turn_done not set")
	}
}

// TestAIChatControlStop: cancelling mid-stream keeps partial text and ends busy.
func TestAIChatControlStop(t *testing.T) {
	// A slow LLM: streams many tokens with delay.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		for i := 0; i < 50; i++ {
			_, _ = w.Write([]byte(sseFrame("token ")))
			if flusher != nil {
				flusher.Flush()
			}
			time.Sleep(20 * time.Millisecond)
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer srv.Close()

	script := aiChatScript(`title="C", ai_base_url="`+srv.URL+`"`,
		`k.form.on("main", "chat", "on_done", function()
  _turn_done = true
end)`,
		``)

	tmp := t.TempDir()
	scriptPath := writeScript(t, tmp, script)
	s, err := New("t1", scriptPath, bindings.Options{
		AllowFS:  []string{tmp},
		AIConfig: &common.AIConfig{BaseURL: srv.URL, Model: "m"},
	}, tLogger{t: t})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	time.Sleep(200 * time.Millisecond)
	s.PostEventAny("main", "chat", "ai_chat_send", map[string]interface{}{"text": "hi"})
	time.Sleep(300 * time.Millisecond)

	// Stop mid-stream.
	s.PostEventAny("main", "chat", "ai_chat_stop", nil)

	if !waitGlobal(t, s, "_turn_done", "true", 5*time.Second) {
		t.Error("turn_done not set after stop")
	}
	// Not busy after stop.
	if v := s.Query(func(L *lua.LState) lua.LValue {
		ct := s.controlTable("main", "chat")
		return ct.RawGetString("ai_busy")
	}); lua.LVAsBool(v) {
		t.Error("still busy after stop")
	}
}

// TestAIChatControlCloseCancels: closing the form cancels the in-flight turn.
func TestAIChatControlCloseCancels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		for i := 0; i < 100; i++ {
			_, _ = w.Write([]byte(sseFrame("slow ")))
			time.Sleep(10 * time.Millisecond)
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	script := aiChatScript(`title="C", ai_base_url="`+srv.URL+`"`,
		`k.form.on("main", "chat", "on_done", function()
  _turn_done = true
end)`,
		``)

	tmp := t.TempDir()
	scriptPath := writeScript(t, tmp, script)
	s, err := New("t1", scriptPath, bindings.Options{
		AllowFS:  []string{tmp},
		AIConfig: &common.AIConfig{BaseURL: srv.URL, Model: "m"},
	}, tLogger{t: t})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	time.Sleep(200 * time.Millisecond)
	s.PostEventAny("main", "chat", "ai_chat_send", map[string]interface{}{"text": "hi"})
	time.Sleep(200 * time.Millisecond)

	// Close the form while the turn is in flight (the k.form.close path).
	s.PostFormEvent("main", "close_form")

	// Turn should end without busy stuck.
	time.Sleep(500 * time.Millisecond)
	if v := s.Query(func(L *lua.LState) lua.LValue {
		ct := s.controlTable("main", "chat")
		return ct.RawGetString("ai_busy")
	}); lua.LVAsBool(v) {
		t.Error("still busy after close_form")
	}
}

// TestAIChatControlIsolation: two controls in one form keep separate transcripts.
func TestAIChatControlIsolation(t *testing.T) {
	srv := mockLLM(t, "from alpha", "from beta")
	defer srv.Close()

	script := aiChatScript(`title="A", ai_base_url="`+srv.URL+`"`,
		``,
		`k.ctrl.ai_chat("main", "beta", {title="B", ai_base_url="`+srv.URL+`"})`)

	tmp := t.TempDir()
	scriptPath := writeScript(t, tmp, script)
	s, err := New("t1", scriptPath, bindings.Options{
		AllowFS:  []string{tmp},
		AIConfig: &common.AIConfig{BaseURL: srv.URL, Model: "m"},
	}, tLogger{t: t})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	time.Sleep(200 * time.Millisecond)
	// Send to the default "chat" control (first mock response).
	s.PostEventAny("main", "chat", "ai_chat_send", map[string]interface{}{"text": "a"})
	// Send to "beta" (second mock response).
	s.PostEventAny("main", "beta", "ai_chat_send", map[string]interface{}{"text": "b"})

	// Wait until both transcripts hold a user + assistant message.
	getLen := func(name string) int {
		return int(lua.LVAsNumber(s.Query(func(L *lua.LState) lua.LValue {
			ct := s.controlTable("main", name)
			if ct == nil {
				return lua.LNumber(0)
			}
			tt := ct.RawGetString("ai_transcript")
			if tbl, ok := tt.(*lua.LTable); ok {
				return lua.LNumber(tbl.Len())
			}
			return lua.LNumber(0)
		})))
	}
	deadline := time.Now().Add(8 * time.Second)
	ok := false
	for time.Now().Before(deadline) {
		if getLen("chat") >= 2 && getLen("beta") >= 2 {
			ok = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !ok {
		t.Fatalf("transcripts not complete: chat=%d beta=%d", getLen("chat"), getLen("beta"))
	}
	if got := getLen("chat"); got != 2 {
		t.Errorf("chat transcript = %d messages, want 2 (user+assistant)", got)
	}
	if got := getLen("beta"); got != 2 {
		t.Errorf("beta transcript = %d messages, want 2 (user+assistant)", got)
	}
}

func queryInt(t *testing.T, s *Session, name string) int {
	t.Helper()
	got := s.Query(func(L *lua.LState) lua.LValue { return L.GetGlobal(name) })
	return int(lua.LVAsNumber(got))
}

// TestAIChatControlRenders runs a real session whose script creates an ai_chat
// control in a grid cell and confirms the rendered form carries the inline AI
// chat shell markup. Guards the constructor + renderer path end-to-end.
func TestAIChatControlRenders(t *testing.T) {
	script := `
function main()
  k.form.new("main", {title="t", layout="grid", cells={{id="a", width=6},{id="b", width=6}}})
  k.ctrl.ai_chat("main", "assistant", {
    cell = "b", title = "Customer assistant",
    welcome = "Ask me…", placeholder = "load customer 7…",
    tools = {}, ai_base_url = "http://localhost:1234/v1",
  })
  k.ctrl.textbox("main", "status", {cell="a", label="Status"})
  k.form.show("main", {modal=false})
end
`
	tmp := t.TempDir()
	scriptPath := writeScript(t, tmp, script)
	s, err := New("t1", scriptPath, bindings.Options{}, tLogger{t: t})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	var html string
	deadline := time.After(4 * time.Second)
loop:
	for {
		select {
		case msg, ok := <-s.Outbox():
			if !ok {
				t.Fatal("outbox closed")
			}
			if msg.Type == "render_form" {
				html = msg.HTML
				break loop
			}
		case <-deadline:
			t.Fatal("no render_form received")
		}
	}
	if !strings.Contains(html, `data-k-ai-chat="1"`) {
		t.Errorf("missing data-k-ai-chat:\n%s", html)
	}
	if !strings.Contains(html, "kalua-ai-chat-inline") {
		t.Errorf("missing inline class:\n%s", html)
	}
	if !strings.Contains(html, "Customer assistant") {
		t.Errorf("missing title:\n%s", html)
	}
	if !strings.Contains(html, "Ask me…") {
		t.Errorf("missing welcome bubble:\n%s", html)
	}
	if !strings.Contains(html, "load customer 7…") {
		t.Errorf("missing placeholder:\n%s", html)
	}
	if !strings.Contains(html, `data-k-ai-input="1"`) {
		t.Errorf("missing input:\n%s", html)
	}
}

// TestAIChatControlOps exercises the k.ai_chat.* script operations against a
// real session: add_message, get_transcript, set_transcript, clear, set_system,
// is_busy, and the ai_chat_message/ai_chat_transcript outbox payloads.
func TestAIChatControlOps(t *testing.T) {
	script := aiChatScript(`title="C"`,
		``,
		`-- Exercise operations synchronously after construction.
local ok, err = pcall(function()
k.ai_chat.add_message("main", "chat", "system", "You are helpful.")
k.ai_chat.add_message("main", "chat", "user", "hello")
local t1 = k.ai_chat.get_transcript("main", "chat")
if #t1 ~= 2 then error("transcript len " .. tostring(#t1) .. " after 1 system+1 user") end
if tostring(k.ai_chat.is_busy("main", "chat")) ~= "false" then error("is_busy should be false") end
k.ai_chat.set_system("main", "chat", "New persona")
k.ai_chat.set_transcript("main", "chat", {
  { role = "user", content = "only" },
})
local t2 = k.ai_chat.get_transcript("main", "chat")
if #t2 ~= 1 then error("transcript len " .. tostring(#t2) .. " after set_transcript") end
k.ai_chat.clear("main", "chat")
local t3 = k.ai_chat.get_transcript("main", "chat")
if #t3 ~= 0 then error("transcript len " .. tostring(#t3) .. " after clear") end
end)
if not ok then _results_ok = err else _results_ok = true end`)

	tmp := t.TempDir()
	scriptPath := writeScript(t, tmp, script)
	s, err := New("t1", scriptPath, bindings.Options{}, tLogger{t: t})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	out := outboxChan(s)
	time.Sleep(300 * time.Millisecond)

	if !waitGlobal(t, s, "_results_ok", "true", 3*time.Second) {
		t.Fatal("script operations did not complete cleanly")
	}

	// The add_message calls pushed ai_chat_message outbox messages.
	msgs := 0
	for {
		select {
		case w := <-out:
			if w.Type == "ai_chat_message" {
				msgs++
			}
		case <-time.After(500 * time.Millisecond):
			goto done
		}
		if msgs >= 2 {
			break
		}
	}
done:
	if msgs < 2 {
		t.Errorf("expected >=2 ai_chat_message outbox, got %d", msgs)
	}
}

// TestAIChatControlSendScript: k.ai_chat.send starts a turn against the fake
// LLM (the browser-send path, including on_send veto handling).
func TestAIChatControlSendScript(t *testing.T) {
	srv := mockLLM(t, "script initiated turn")
	defer srv.Close()

	script := aiChatScript(`title="C", ai_base_url="`+srv.URL+`"`,
		`k.form.on("main", "chat", "on_done", function()
  _turn_done = true
end)`,
		`k.ai_chat.send("main", "chat", "from script")`)

	tmp := t.TempDir()
	scriptPath := writeScript(t, tmp, script)
	s, err := New("t1", scriptPath, bindings.Options{
		AIConfig: &common.AIConfig{BaseURL: srv.URL, Model: "m"},
	}, tLogger{t: t})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	if !waitGlobal(t, s, "_turn_done", "true", 8*time.Second) {
		t.Error("k.ai_chat.send turn did not complete")
	}
}

func writeScript(t *testing.T, dir, src string) string {
	t.Helper()
	path := filepath.Join(dir, "app.lua")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
