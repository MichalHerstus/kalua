package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	glua "github.com/yuin/gopher-lua"
	"kalua/internal/bindings"
)

// TestImageClickDispatch exercises the §4.3 image control in a real session: a
// clickable image's onclick handler is dispatched when the browser reports a
// click, with the image src as the event value.
func TestImageClickDispatch(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "app.lua")
	src := `
function main()
  k.form.new("f", {title = "t"})
  k.ctrl.image("f", "pic", {
    src = "/img/a.png",
    alt = "A",
    width = 120,
    clickable = true,
    onclick = function(val)
      _clicked = val
    end
  })
  k.form.show("f")
end
`
	if err := os.WriteFile(script, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := New("t1", script, bindings.Options{}, tLogger{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	// Drain outbox so rendering messages don't block.
	go func() {
		for range s.Outbox() {
		}
	}()

	time.Sleep(150 * time.Millisecond)

	// Browser click on the clickable image reports the src as the value.
	s.PostEventAny("f", "pic", "click", "/img/a.png")

	deadline := time.After(4 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for image click handler")
		default:
		}
		if v := s.GetGlobal("_clicked"); v == glua.LString("/img/a.png") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestImageNoClickWhenNotClickable verifies a non-clickable image is not
// registered as a click handler target.
func TestImageNoClickWhenNotClickable(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "app.lua")
	src := `
function main()
  k.form.new("f", {title = "t"})
  k.ctrl.image("f", "pic", {src = "/img/static.png"})
  k.form.show("f")
end
`
	if err := os.WriteFile(script, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := New("t1", script, bindings.Options{}, tLogger{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	go func() {
		for range s.Outbox() {
		}
	}()

	time.Sleep(150 * time.Millisecond)

	// The control table must not carry a click handler for a non-clickable image.
	// Inspect on the actor goroutine (Query) and return a plain Go report: the
	// LState and its tables are not safe to read from the test goroutine, and
	// dereferencing them after Query returns would race the actor.
	report := s.Query(func(L *glua.LState) glua.LValue {
		formTbl, ok := L.GetGlobal("f").(*glua.LTable)
		if !ok {
			return glua.LString("form f not found")
		}
		controls, ok := formTbl.RawGetString("controls").(*glua.LTable)
		if !ok {
			return glua.LString("no controls table")
		}
		pic, ok := controls.RawGetString("pic").(*glua.LTable)
		if !ok {
			return glua.LString("no pic control")
		}
		if handlers, ok := formTbl.RawGetString("handlers").(*glua.LTable); ok {
			if h := handlers.RawGetString("pic"); h != glua.LNil {
				return glua.LString("non-clickable image registered a click handler: " + h.String())
			}
		}
		if v := pic.RawGetString("clickable"); v != glua.LNil && v != glua.LFalse {
			return glua.LString("clickable should be unset/false, got " + v.String())
		}
		if v := pic.RawGetString("src"); v.String() != "/img/static.png" {
			return glua.LString("src = " + v.String() + ", want /img/static.png")
		}
		return glua.LString("")
	})
	if msg := report.String(); msg != "" {
		t.Fatal(msg)
	}
}
