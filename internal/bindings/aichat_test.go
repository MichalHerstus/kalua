//go:build !wasm

package bindings

import (
	"strings"
	"testing"

	"github.com/yuin/gopher-lua"
)

// TestRenderAIChatControl verifies §12.2: the ai_chat control renders the
// inline shell with the standard id/data attributes, title/placeholder,
// height CSS var, header toggle, API-key row, disabled state and shared
// styling attributes.
func TestRenderAIChatControl(t *testing.T) {
	L := setupTestState(t)

	welcome := L.NewTable()
	wm := L.NewTable()
	wm.RawSetString("role", lua.LString("assistant"))
	wm.RawSetString("content", lua.LString("Ask me to load a customer…"))
	welcome.RawSetInt(1, wm)

	base := map[string]lua.LValue{
		"form":           lua.LString("main"),
		"name":           lua.LString("assistant"),
		"type":           lua.LString("ai_chat"),
		"ai_title":       lua.LString("Customer assistant"),
		"ai_welcome":     lua.LString("Ask me to load a customer…"),
		"ai_placeholder": lua.LString("load customer 7…"),
		"ai_max_turns":   lua.LNumber(8),
		"ai_markdown":    lua.LTrue,
		"ai_show_header": lua.LTrue,
		"ai_busy":        lua.LFalse,
		"ai_transcript":  welcome,
		"ai_height":      lua.LNumber(420),
	}

	got := renderControl(ctrlTable(L, "ai_chat", base))
	if !strings.Contains(got, `id="c:main:assistant"`) {
		t.Errorf("missing control id: %s", got)
	}
	if !strings.Contains(got, `data-k-ai-chat="1"`) {
		t.Errorf("missing data-k-ai-chat: %s", got)
	}
	if !strings.Contains(got, `data-k-form="main"`) || !strings.Contains(got, `data-k-ctrl="assistant"`) {
		t.Errorf("missing form/ctrl attrs: %s", got)
	}
	if !strings.Contains(got, `class="kalua-ai-chat-inline"`) {
		t.Errorf("missing inline class: %s", got)
	}
	if !strings.Contains(got, `data-k-md="1"`) {
		t.Errorf("missing markdown attr: %s", got)
	}
	if !strings.Contains(got, `--kalua-ai-height:420`) {
		t.Errorf("missing height var: %s", got)
	}
	if !strings.Contains(got, `Customer assistant`) {
		t.Errorf("missing title: %s", got)
	}
	if !strings.Contains(got, `data-k-ai-transcript="1"`) {
		t.Errorf("missing transcript: %s", got)
	}
	if !strings.Contains(got, `Ask me to load a customer…`) {
		t.Errorf("missing welcome bubble: %s", got)
	}
	if !strings.Contains(got, `data-k-ai-input="1"`) {
		t.Errorf("missing input: %s", got)
	}
	if !strings.Contains(got, `data-k-ai-send="1"`) {
		t.Errorf("missing send: %s", got)
	}
	if !strings.Contains(got, `data-k-ai-stop="1"`) {
		t.Errorf("missing stop: %s", got)
	}
	if !strings.Contains(got, `data-k-ai-status="1"`) {
		t.Errorf("missing status: %s", got)
	}

	// show_header = false omits the header.
	noHeader := map[string]lua.LValue{}
	for k, v := range base {
		noHeader[k] = v
	}
	noHeader["ai_show_header"] = lua.LFalse
	gotNH := renderControl(ctrlTable(L, "ai_chat", noHeader))
	if strings.Contains(gotNH, `kalua-ai-header`) {
		t.Errorf("header should be omitted: %s", gotNH)
	}

	// show_apikey = true adds the API key row.
	withKey := map[string]lua.LValue{}
	for k, v := range base {
		withKey[k] = v
	}
	withKey["ai_show_apikey"] = lua.LTrue
	gotKey := renderControl(ctrlTable(L, "ai_chat", withKey))
	if !strings.Contains(gotKey, `data-k-ai-apikey="1"`) {
		t.Errorf("missing apikey row: %s", gotKey)
	}

	// disabled sets disabled on input/send/stop.
	dis := map[string]lua.LValue{}
	for k, v := range base {
		dis[k] = v
	}
	dis["enabled"] = lua.LString("false")
	gotDis := renderControl(ctrlTable(L, "ai_chat", dis))
	if !strings.Contains(gotDis, ` disabled`) {
		t.Errorf("missing disabled attr: %s", gotDis)
	}

	// height sets the CSS var.
	hi := map[string]lua.LValue{}
	for k, v := range base {
		hi[k] = v
	}
	hi["ai_height"] = lua.LString("60vh")
	gotHi := renderControl(ctrlTable(L, "ai_chat", hi))
	if !strings.Contains(gotHi, `--kalua-ai-height:60vh`) {
		t.Errorf("missing height var value: %s", gotHi)
	}
}

// TestAIChatControlValue verifies the §12.2 control value semantics: get_value
// returns the last assistant text (the control's value prop, written by the
// actor on assistant turns), and set_value is a no-op.
func TestAIChatControlValue(t *testing.T) {
	L := setupFullState(t)

	src := `
k.form.new("main", { title = "t" })
k.ctrl.table("main", "tbl", {})
-- Simulate an ai_chat control on the form (the constructor needs a session,
-- so we build the control table directly to exercise the value plumbing).
local ct = CTRL("main", "tbl")
ct.type = "ai_chat"
ct.value = "last assistant reply"

local function assert(c, m) if not c then error(m) end end

local v = k.ctrl.get_value("main", "tbl")
assert(v == "last assistant reply", "get_value == last assistant text")

k.ctrl.set_value("main", "tbl", "nope")
assert(k.ctrl.get_value("main", "tbl") == "last assistant reply", "set_value is a no-op on ai_chat")

_ok = true
`
	if err := L.DoString(src); err != nil {
		t.Fatalf("run: %v", err)
	}
	if L.GetGlobal("_ok") != lua.LTrue {
		t.Fatal("value semantics assertions failed")
	}
}
