package session

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	glua "github.com/yuin/gopher-lua"
	"kalua/internal/bindings"
)

// TestRealTabOnChange verifies the browser's tab_change event is unpacked into
// onchange(new_index, old_index), current_tab is updated on the control (so
// k.tab.get_tab reflects the active tab), and the generic control-value update
// is skipped (the payload is an index, not a form value). It also verifies
// k.tab.set_tab sends a tab_set_tab outbox message.
func TestRealTabOnChange(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "app.lua")
	src := `
function main()
  k.form.new("main", {title="t"})
  k.ctrl.tab("main", "tabs", {
    tabs = {
      { title = "Dashboard" },
      { title = "Reports" },
      { title = "Settings" },
    },
  })
  _changed = nil
  _old = nil
  _new = nil
  k.form.on("main", "tabs", "onchange", function(new_idx, old_idx)
    _new = new_idx
    _old = old_idx
    _changed = true
  end)
  k.form.show("main")
end
`
	if err := os.WriteFile(script, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := New("t1", script, bindings.Options{}, tLogger{t: t})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	// Capture outbox messages (tab_set_tab etc.).
	var outboxMsgs []string
	go func() {
		for msg := range s.Outbox() {
			outboxMsgs = append(outboxMsgs, fmt.Sprintf("%s ctrl=%q data=%q", msg.Type, msg.Ctrl, msg.Data))
		}
	}()

	time.Sleep(300 * time.Millisecond)

	// Browser reports a switch to tab 3.
	s.PostEventAny("main", "tabs", "onchange", map[string]interface{}{"index": float64(3)})

	deadline := time.After(4 * time.Second)
	for {
		if s.GetGlobal("_changed") == glua.LBool(true) {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for onchange handler")
		case <-time.After(20 * time.Millisecond):
		}
	}

	// handler received (new=3, old=1).
	live := func(name string) int {
		return int(glua.LVAsNumber(s.GetGlobal(name)))
	}
	if live("_new") != 3 {
		t.Errorf("onchange new = %d, want 3", live("_new"))
	}
	if live("_old") != 1 {
		t.Errorf("onchange old = %d, want 1", live("_old"))
	}

	// control current_tab updated so k.tab.get_tab reads 3.
	res := s.Query(func(L *glua.LState) glua.LValue {
		main := L.GetGlobal("main")
		tbl, _ := main.(*glua.LTable)
		if tbl == nil {
			return glua.LString("main nil")
		}
		controls, _ := tbl.RawGetString("controls").(*glua.LTable)
		if controls == nil {
			return glua.LString("controls nil")
		}
		ctrl, _ := controls.RawGetString("tabs").(*glua.LTable)
		if ctrl == nil {
			return glua.LString("ctrl nil")
		}
		return ctrl.RawGetString("current_tab")
	})
	if n := glua.LVAsNumber(res); n != 3 {
		t.Errorf("control current_tab = %v, want 3", res)
	}
}

// TestRealTabSetTabOutbox verifies k.tab.set_tab updates current_tab and pushes
// a tab_set_tab outbox message carrying {index, old_index}.
func TestRealTabSetTabOutbox(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "app.lua")
	src := `
function main()
  k.form.new("main", {title="t"})
  k.ctrl.tab("main", "tabs", {
    tabs = {
      { title = "Dashboard" },
      { title = "Reports" },
      { title = "Settings" },
    },
  })
  -- switch programmatically before show (also verifies ops work before show)
  k.tab.set_tab("main", "tabs", 2)
  k.form.show("main")
end
`
	if err := os.WriteFile(script, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := New("t1", script, bindings.Options{}, tLogger{t: t})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	var got string
	done := make(chan struct{})
	go func() {
		for msg := range s.Outbox() {
			if msg.Type == "tab_set_tab" {
				got = msg.Data
				close(done)
				return
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(4 * time.Second):
		t.Fatalf("timed out waiting for tab_set_tab message")
	}

	if got != `{"index":2,"old_index":1}` {
		t.Errorf("tab_set_tab data = %s, want {\"index\":2,\"old_index\":1}", got)
	}

	// get_tab reads back 2 from the control table.
	res := s.Query(func(L *glua.LState) glua.LValue {
		main := L.GetGlobal("main")
		tbl, _ := main.(*glua.LTable)
		if tbl == nil {
			return glua.LNumber(-1)
		}
		controls, _ := tbl.RawGetString("controls").(*glua.LTable)
		if controls == nil {
			return glua.LNumber(-1)
		}
		ctrl, _ := controls.RawGetString("tabs").(*glua.LTable)
		if ctrl == nil {
			return glua.LNumber(-1)
		}
		return ctrl.RawGetString("current_tab")
	})
	if n := glua.LVAsNumber(res); n != 2 {
		t.Errorf("current_tab after set_tab = %v, want 2", res)
	}
}
