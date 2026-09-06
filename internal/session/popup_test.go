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

// TestPopupElementRender pins the popup HTML renderer: optional header with
// title, a nested menu tree where branches carry data-k-submenu and a fly-out
// submenu, and leaves carry the JSON-encoded typed value in data-k-value.
func TestPopupElementRender(t *testing.T) {
	html := renderPopupHTML("pp1", common.PopupOptions{
		Title: "A <menu> & more",
		Items: []common.PopupItem{
			{Label: "File", Items: []common.PopupItem{{Label: "Open", Value: "1"}}},
			{Label: "Quit", Value: "\"bye\""},
			{Label: "Go", Value: "false"},
		},
	})

	if !strings.Contains(html, `<div class="popup-header">`) {
		t.Fatalf("expected header; got %s", html)
	}
	if !strings.Contains(html, "A &lt;menu&gt; &amp; more") {
		t.Fatalf("title not escaped; got %s", html)
	}
	if !strings.Contains(html, `class="popup-item popup-branch" data-k-popup-id="pp1" data-k-submenu="true"`) {
		t.Fatalf("branch item missing; got %s", html)
	}
	if !strings.Contains(html, `<ul class="kalua-popup-submenu">`) {
		t.Fatalf("nested submenu missing; got %s", html)
	}
	if !strings.Contains(html, `<span class="popup-caret"`) {
		t.Fatalf("branch caret missing; got %s", html)
	}
	if !strings.Contains(html, `>Open</span>`) || !strings.Contains(html, `data-k-value="1"`) {
		t.Fatalf("nested leaf (number) missing; got %s", html)
	}
	if !strings.Contains(html, `data-k-value="&#34;bye&#34;"`) {
		t.Fatalf("string leaf missing; got %s", html)
	}
	if !strings.Contains(html, `data-k-value="false"`) {
		t.Fatalf("bool leaf missing; got %s", html)
	}
}

// TestPopupElementRenderNoTitle pins that an empty title omits the header.
func TestPopupElementRenderNoTitle(t *testing.T) {
	html := renderPopupHTML("pp1", common.PopupOptions{Items: []common.PopupItem{{Label: "Quit", Value: "\"quit\""}}})
	if strings.Contains(html, "popup-header") {
		t.Fatalf("expected no header; got %s", html)
	}
	if !strings.Contains(html, `<ul class="kalua-popup-menu">`) {
		t.Fatalf("expected menu list; got %s", html)
	}
}

// TestPopupFallbackChoice pins the data-k-choice fallback: string values decode
// to themselves, non-strings fall back to the label.
func TestPopupFallbackChoice(t *testing.T) {
	if got := popupFallbackChoice(common.PopupItem{Label: "Quit", Value: "\"quit\""}); got != "quit" {
		t.Fatalf("string fallback = %q, want quit", got)
	}
	if got := popupFallbackChoice(common.PopupItem{Label: "Delete", Value: "1"}); got != "Delete" {
		t.Fatalf("numeric fallback = %q, want Delete", got)
	}
}

// runPopupSession runs an app whose button handler invokes the given Lua
// expression and answers the resulting popup via answer. It returns the value
// the handler received after resume (nil for a dismissal).
func runPopupSession(t *testing.T, expr string, answer func(s *Session, id string)) glua.LValue {
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
				if out.Type == "popup" {
					answer(s, out.ID)
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

// TestPopupNumericValue verifies a picked leaf returns its value with the
// original type (number).
func TestPopupNumericValue(t *testing.T) {
	script := `k.popup{{"Open", 1}, {"Quit", 0}}`
	res := runPopupSession(t, script, func(s *Session, id string) {
		s.HandlePopupChoice(id, float64(1))
	})
	if res != glua.LNumber(1) {
		t.Fatalf("expected LNumber(1); got %T %v", res, res)
	}
}

// TestPopupStringValue verifies a string leaf value round-trips.
func TestPopupStringValue(t *testing.T) {
	script := `k.popup{{label="Open", value="open"}, {"Quit"}}`
	res := runPopupSession(t, script, func(s *Session, id string) {
		s.HandlePopupChoice(id, "open")
	})
	if res != glua.LString("open") {
		t.Fatalf("expected LString(\"open\"); got %T %v", res, res)
	}
}

// TestPopupBoolValue verifies a boolean leaf value round-trips.
func TestPopupBoolValue(t *testing.T) {
	script := `k.popup{{"Yes", true}, {"No", false}}`
	res := runPopupSession(t, script, func(s *Session, id string) {
		s.HandlePopupChoice(id, false)
	})
	if res != glua.LFalse {
		t.Fatalf("expected LFalse; got %T %v", res, res)
	}
}

// TestPopupNestedPick verifies picking a leaf deep inside a submenu returns its
// typed value.
func TestPopupNestedPick(t *testing.T) {
	script := `k.popup{title="m", items={{label="File", items={{"Recent", items={{"a.lua", 7}}}}}, "Top"}}`
	res := runPopupSession(t, script, func(s *Session, id string) {
		s.HandlePopupChoice(id, float64(7))
	})
	if res != glua.LNumber(7) {
		t.Fatalf("expected LNumber(7); got %T %v", res, res)
	}
}

// TestPopupDismissNil verifies dismissing the popup resumes the coroutine with
// nil.
func TestPopupDismissNil(t *testing.T) {
	script := `k.popup{{"Open", 1}}`
	res := runPopupSession(t, script, func(s *Session, id string) {
		s.DismissPopup(id)
	})
	if res != glua.LNil {
		t.Fatalf("expected LNil on dismiss; got %T %v", res, res)
	}
}
