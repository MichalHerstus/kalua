package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	glua "github.com/yuin/gopher-lua"
	"kalua/internal/bindings"
)

// TestRealSidebarSelect verifies a sidebar item click is unpacked into the
// sidebar_select event: the optional on_select (global handler) receives the
// clicked item, and the k.form.on("sidebar_select") handler receives
// (action, item, index). The generic control-value update is skipped.
func TestRealSidebarSelect(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "app.lua")
	src := `
function main()
  k.form.new("main", {title="t", layout="grid"})
  k.ctrl.sidebar("main", "side", {
    items = {
      { label = "Dashboard", action = "nav_dashboard" },
      { type = "separator" },
      { label = "Settings", action = "nav_settings", badge = 5 },
    },
    on_select = function(item)
      _on_select_action = item.action
      _on_select_ran = true
    end,
  })
  _ev_action = nil
  _ev_item = nil
  _ev_index = nil
  _ev_ran = false
  k.form.on("main", "side", "sidebar_select", function(action, item, index)
    _ev_action = action
    _ev_item = item
    _ev_index = index
    _ev_ran = true
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

	time.Sleep(300 * time.Millisecond)

	item := map[string]interface{}{"label": "Settings", "action": "nav_settings", "badge": float64(5)}
	s.PostEventAny("main", "side", "sidebar_select", map[string]interface{}{
		"action": "nav_settings",
		"item":   item,
		"index":  float64(2),
	})

	deadline := time.After(4 * time.Second)
	for {
		if s.GetGlobal("_ev_ran") == glua.LBool(true) && s.GetGlobal("_on_select_ran") == glua.LBool(true) {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for sidebar_select handlers (ev=%v sel=%v)",
				s.GetGlobal("_ev_ran"), s.GetGlobal("_on_select_ran"))
		case <-time.After(20 * time.Millisecond):
		}
	}

	// on_select got the item table.
	if a := s.GetGlobal("_on_select_action"); a.String() != "nav_settings" {
		t.Errorf("on_select action = %v, want nav_settings", a)
	}
	// k.form.on handler got (action, item, index).
	if a := s.GetGlobal("_ev_action"); a.String() != "nav_settings" {
		t.Errorf("ev action = %v, want nav_settings", a)
	}
	if i := int(glua.LVAsNumber(s.GetGlobal("_ev_index"))); i != 2 {
		t.Errorf("ev index = %d, want 2", i)
	}
	res := s.Query(func(L *glua.LState) glua.LValue {
		itemTbl, ok := L.GetGlobal("_ev_item").(*glua.LTable)
		if !ok {
			return glua.LString("no table")
		}
		return itemTbl.RawGetString("action")
	})
	if res.String() != "nav_settings" {
		t.Errorf("ev item.action = %v, want nav_settings", res)
	}
}

// TestRealTopbarLogoutOpt verifies clicking the topbar user block runs the
// registered logout function (from the `logout` opt), via the "logout" event.
func TestRealTopbarLogoutOpt(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "app.lua")
	src := `
function main()
  k.form.new("main", {title="t"})
  _logged_out = false
  k.ctrl.topbar("main", "bar", {
    title = "My App",
    user = { name = "John", role = "admin" },
    logout = function() _logged_out = true end,
  })
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

	time.Sleep(300 * time.Millisecond)
	s.PostEventAny("main", "bar", "logout", map[string]interface{}{})

	deadline := time.After(4 * time.Second)
	for {
		if s.GetGlobal("_logged_out") == glua.LBool(true) {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for logout handler")
		case <-time.After(20 * time.Millisecond):
		}
	}
}

// TestRealFooterAutoFill verifies the footer auto-fills version (non-empty)
// and script_name from the running script's base name (§11.3).
func TestRealFooterAutoFill(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "myapp.lua")
	src := `
function main()
  k.form.new("main", {title="t"})
  k.ctrl.footer("main", "foot", { text = "© me" })
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

	time.Sleep(300 * time.Millisecond)

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
		ctrl, _ := controls.RawGetString("foot").(*glua.LTable)
		if ctrl == nil {
			return glua.LString("ctrl nil")
		}
		ver := ctrl.RawGetString("version").String()
		name := ctrl.RawGetString("script_name").String()
		return glua.LString(ver + "|" + name)
	})
	parts := strings.SplitN(res.String(), "|", 2)
	if len(parts) != 2 || parts[0] == "" {
		t.Errorf("footer version not auto-filled: %q", res)
	}
	if parts[1] != "myapp.lua" {
		t.Errorf("footer script_name = %q, want myapp.lua", parts[1])
	}
}
