package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	glua "github.com/yuin/gopher-lua"
	"kalua/internal/bindings"
	"kalua/internal/common"
)

// TestMsgboxElementRender pins the rich msgbox HTML renderer: header with
// title + icon, escaped message, and buttons carrying the JSON-encoded typed
// return value in data-k-value.
func TestMsgboxElementRender(t *testing.T) {
	html := renderMsgboxHTML("mb1", common.MsgboxOptions{
		Title:   "A <title> & more",
		Message: "Hello <world> & \"friends\"",
		Kind:    "warning",
		Buttons: []common.MsgboxButton{
			{Label: "Overwrite", Value: "1"},
			{Label: "Quit", Value: "\"bye\""},
		},
	})

	if !strings.Contains(html, `<div class="msgbox-header">`) {
		t.Fatalf("expected header; got %s", html)
	}
	if !strings.Contains(html, "A &lt;title&gt; &amp; more") {
		t.Fatalf("title not escaped; got %s", html)
	}
	if !strings.Contains(html, `<span class="msgbox-icon">`) {
		t.Fatalf("expected icon; got %s", html)
	}
	if !strings.Contains(html, "Hello &lt;world&gt; &amp; &#34;friends&#34;") {
		t.Fatalf("message not escaped; got %s", html)
	}
	if !strings.Contains(html, `data-k-value="1"`) || !strings.Contains(html, `>Overwrite</button>`) {
		t.Fatalf("numeric button missing; got %s", html)
	}
	if !strings.Contains(html, `data-k-value="&#34;bye&#34;"`) || !strings.Contains(html, `>Quit</button>`) {
		t.Fatalf("string button missing; got %s", html)
	}
}

// TestMsgboxElementRenderNoTitle pins that an empty title omits the header.
func TestMsgboxElementRenderNoTitle(t *testing.T) {
	html := renderMsgboxHTML("mb1", common.MsgboxOptions{Message: "hi", Kind: "info"})
	if strings.Contains(html, "msgbox-header") {
		t.Fatalf("expected no header; got %s", html)
	}
	if !strings.Contains(html, `data-k-value="&#34;ok&#34;"`) {
		t.Fatalf("expected default OK button; got %s", html)
	}
}

// TestMsgboxKindMapping pins legacy kind → CSS class mapping.
func TestMsgboxKindMapping(t *testing.T) {
	cases := map[string]string{
		"info":      "info",
		"warning":   "warning",
		"danger":    "danger",
		"warn":      "warning",
		"error":     "danger",
		"ok-cancel": "ok-cancel",
		"yes-no":    "yes-no",
		"":          "",
	}
	for in, want := range cases {
		if got := msgboxKind(in); got != want {
			t.Fatalf("msgboxKind(%q) = %q, want %q", in, got, want)
		}
	}
}

// runMsgboxSession runs an app whose button handler invokes the given Lua
// expression and answers the resulting msgbox with value, returning the value
// the handler received after resume.
func runMsgboxSession(t *testing.T, expr string, value interface{}, choice string) glua.LValue {
	t.Helper()
	tmp := t.TempDir()
	script := filepath.Join(tmp, "app.lua")
	src := `
function main()
  local form = k.form.new("f", {title="t"})
  k.form.on("f", "btn", "click", function(ctx)
    local choice = ` + expr + `
    _result = choice
    _done = true
  end)
  k.form.show("f")
end
`
	if err := os.WriteFile(script, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := New("t1", script, bindings.Options{}, tLogger{t})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	go func() {
		for {
			select {
			case out, ok := <-s.Outbox():
				if !ok {
					return
				}
				if out.Type == "msgbox" {
					s.HandleMsgboxChoice(out.ID, value, choice)
				}
			case <-time.After(3 * time.Second):
				return
			}
		}
	}()

	time.Sleep(100 * time.Millisecond)
	s.PostEvent("f", "btn", "click", glua.LString(""))

	deadline := time.Now().Add(2 * time.Second)
	for {
		flag := s.GetGlobal("_done")
		if flag == glua.LBool(true) {
			return s.GetGlobal("_result")
		}
		if time.Now().After(deadline) {
			t.Fatalf("handler did not complete within deadline; _done=%v", flag)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestMsgboxRichNumericValue verifies the rich table form returns the clicked
// button's numeric value with its type intact.
func TestMsgboxRichNumericValue(t *testing.T) {
	script := `k.msgbox{title="Delete", message="Sure?", type="danger", buttons={{"Delete", 1}, {"Keep", 0}, {"Help", "help"}}}`
	res := runMsgboxSession(t, script, float64(1), "")
	if res != glua.LNumber(1) {
		t.Fatalf("expected LNumber(1); got %T %v", res, res)
	}
}

// TestMsgboxRichStringValue verifies a string button value round-trips.
func TestMsgboxRichStringValue(t *testing.T) {
	script := `k.msgbox{message="Pick", buttons={{"Yes", "yep"}, {"No", "nope"}}}`
	res := runMsgboxSession(t, script, "yep", "")
	if res != glua.LString("yep") {
		t.Fatalf("expected LString(\"yep\"); got %T %v", res, res)
	}
}

// TestMsgboxRichBoolValue verifies a boolean button value round-trips.
func TestMsgboxRichBoolValue(t *testing.T) {
	script := `k.msgbox{type="warning", buttons={{"Yes", true}, {"No", false}}}`
	res := runMsgboxSession(t, script, false, "")
	if res != glua.LFalse {
		t.Fatalf("expected LFalse; got %T %v", res, res)
	}
}

// TestMsgboxDefaultOK verifies the rich form with no buttons adds a single OK
// button returning "ok".
func TestMsgboxDefaultOK(t *testing.T) {
	script := `k.msgbox{title="Hello", message="Welcome"}`
	res := runMsgboxSession(t, script, "ok", "")
	if res != glua.LString("ok") {
		t.Fatalf("expected LString(\"ok\"); got %T %v", res, res)
	}
}

// TestMsgboxLegacyChoiceFallback verifies that a client answering with only the
// legacy choice string still resumes with that string.
func TestMsgboxLegacyChoiceFallback(t *testing.T) {
	script := `k.msgbox("are you sure?")`
	res := runMsgboxSession(t, script, nil, "ok")
	if res != glua.LString("ok") {
		t.Fatalf("expected LString(\"ok\"); got %T %v", res, res)
	}
}
