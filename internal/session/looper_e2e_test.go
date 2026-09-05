package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	glua "github.com/yuin/gopher-lua"
	"kalua/internal/bindings"
)

// TestRealLooperDBLinked runs a session whose Lua app connects a sqlite DB and
// links a k.ctrl.looper to it (db=, query=, links=). The Go pager serves each
// looper_scroll_request from the browser with a looper_db_batch reply whose rows
// carry a `data` map keyed by the linked template control names.
func TestRealLooperDBLinked(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "test.db")
	script := filepath.Join(tmp, "app.lua")
	src := `
function main()
  local db = k.connect_sqlite(%q)
  k.sql(db, "CREATE TABLE items (id INTEGER, name TEXT, qty INTEGER)")
  for i = 1, 30 do
    k.sql(db, "INSERT INTO items (id, name, qty) VALUES (?, ?, ?)", i, "item" .. i, (i * 10) %% 97)
  end

  k.form.new("f", {title="t"})
  k.ctrl.looper("f", "lp", {
    db = db,
    query = "SELECT id, name, qty FROM items",
    page_size = 10,
    links = {
      {column=1, field="id",   control="lb_id"},
      {column=2, field="name", control="lb_name"},
      {column=3, field="qty",  control="lb_qty"},
    },
  })
  k.form.show("f")
end
`
	if err := os.WriteFile(script, []byte(fmt.Sprintf(src, dbPath)), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := New("t1", script, bindings.Options{AllowFS: []string{tmp}}, tLogger{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	out := make(chan outboxWire, 32)
	go func() {
		for msg := range s.Outbox() {
			out <- outboxWire{Type: msg.Type, Selector: msg.Selector, Data: msg.Data, Form: msg.Form, Ctrl: msg.Ctrl}
		}
	}()

	time.Sleep(200 * time.Millisecond)

	// Browser has 20 rows rendered; asks for the next 10 starting at index 21.
	s.PostLooperScrollRequest("f", "lp", map[string]interface{}{
		"start_idx": 21, "count": 10,
	})

	deadline := time.After(4 * time.Second)
	var got string
	for {
		select {
		case w := <-out:
			if w.Type != "looper_db_batch" {
				continue
			}
			if w.Ctrl != "lp" {
				t.Fatalf("batch ctrl = %q, want lp", w.Ctrl)
			}
			got = w.Data
			goto done
		case <-deadline:
			t.Fatalf("timed out waiting for looper_db_batch")
		}
	}
done:
	var payload struct {
		Rows []struct {
			Index int                    `json:"index"`
			Data  map[string]interface{} `json:"data"`
		} `json:"rows"`
		HasMore  bool `json:"has_more"`
		LastPage int  `json:"last_page"`
	}
	if err := json.Unmarshal([]byte(got), &payload); err != nil {
		t.Fatalf("looper_db_batch JSON: %v", err)
	}
	if payload.LastPage != 3 {
		t.Errorf("last_page = %d, want 3", payload.LastPage)
	}
	if payload.HasMore {
		t.Errorf("has_more = true, want false for final batch")
	}
	if len(payload.Rows) != 10 {
		t.Fatalf("batch row count = %d, want 10", len(payload.Rows))
	}
	if payload.Rows[0].Index != 21 {
		t.Errorf("first row index = %d, want 21", payload.Rows[0].Index)
	}
	first := payload.Rows[0].Data
	if first["lb_id"] != float64(21) || first["lb_name"] != "item21" {
		t.Errorf("first row data = %+v", first)
	}
}

// TestRealLooperSortFilterDW exercises the server-side sort/filter path for a
// linked looper, mirroring the tabulator behavior: the browser sends sort/filter
// arrays and the Go pager applies them (not the Lua side).
func TestRealLooperDBLinkedFilter(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "test.db")
	script := filepath.Join(tmp, "app.lua")
	src := `
function main()
  local db = k.connect_sqlite(%q)
  k.sql(db, "CREATE TABLE items (id INTEGER, name TEXT, qty INTEGER)")
  for i = 1, 30 do
    k.sql(db, "INSERT INTO items (id, name, qty) VALUES (?, ?, ?)", i, "item" .. i, (i * 10) %% 97)
  end

  k.form.new("f", {title="t"})
  k.ctrl.looper("f", "lp", {
    db = db,
    query = "SELECT id, name, qty FROM items",
    page_size = 10,
    links = { {field="id", control="lb_id"}, {field="name", control="lb_name"} },
  })
  k.form.show("f")
end
`
	if err := os.WriteFile(script, []byte(fmt.Sprintf(src, dbPath)), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := New("t1", script, bindings.Options{AllowFS: []string{tmp}}, tLogger{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	out := make(chan outboxWire, 16)
	go func() {
		for msg := range s.Outbox() {
			out <- outboxWire{Type: msg.Type, Selector: msg.Selector, Data: msg.Data, Form: msg.Form, Ctrl: msg.Ctrl}
		}
	}()

	time.Sleep(200 * time.Millisecond)

	s.PostLooperScrollRequest("f", "lp", map[string]interface{}{
		"start_idx": 1, "count": 10,
		"sort": []interface{}{
			map[string]interface{}{"field": "id", "dir": "DESC"},
		},
		"filter": []interface{}{
			map[string]interface{}{"field": "name", "type": "=", "value": "item5"},
		},
	})

	deadline := time.After(4 * time.Second)
	var got string
	for {
		select {
		case w := <-out:
			if w.Type != "looper_db_batch" {
				continue
			}
			got = w.Data
			goto done
		case <-deadline:
			t.Fatalf("timed out waiting for looper_db_batch")
		}
	}
done:
	var payload struct {
		Rows []struct {
			Index int                    `json:"index"`
			Data  map[string]interface{} `json:"data"`
		} `json:"rows"`
		HasMore  bool `json:"has_more"`
		LastPage int  `json:"last_page"`
	}
	if err := json.Unmarshal([]byte(got), &payload); err != nil {
		t.Fatalf("looper_db_batch JSON: %v", err)
	}
	if len(payload.Rows) != 1 || payload.Rows[0].Data["lb_id"] != float64(5) {
		t.Fatalf("filtered rows = %+v", payload.Rows)
	}
	if payload.LastPage != 1 || payload.HasMore {
		t.Errorf("last_page=%d has_more=%v, want 1/false", payload.LastPage, payload.HasMore)
	}
}

// TestLooperRowSelection verifies the browser's row-click dispatch: selecting a
// looper row fires the host handlers with the Kalipso signatures
// onselect(line_idx) and onclick(ctrl_name, line_idx), and the value table is
// not written into the form's control definitions.
func TestLooperRowSelection(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "test.db")
	script := filepath.Join(tmp, "app.lua")
	src := `
function main()
  local db = k.connect_sqlite(%q)
  k.sql(db, "CREATE TABLE items (id INTEGER, name TEXT, qty INTEGER)")
  for i = 1, 30 do
    k.sql(db, "INSERT INTO items (id, name, qty) VALUES (?, ?, ?)", i, "item" .. i, (i * 10) %% 97)
  end

  k.form.new("f", {title="t"})
  k.ctrl.looper("f", "lp", {
    db = db,
    query = "SELECT id, name, qty FROM items",
    page_size = 10,
    links = { {field="id", control="lb_id"}, {field="name", control="lb_name"} },
  })
  selected_line = nil
  clicked_ctrl = nil
  clicked_line = nil
  k.form.on("f", "lp", "onselect", function(line_idx)
    selected_line = line_idx
  end)
  k.form.on("f", "lp", "onclick", function(ctrl_name, line_idx)
    clicked_ctrl = ctrl_name
    clicked_line = line_idx
  end)
  k.form.show("f")
end
`
	if err := os.WriteFile(script, []byte(fmt.Sprintf(src, dbPath)), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := New("t1", script, bindings.Options{AllowFS: []string{tmp}}, tLogger{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	time.Sleep(200 * time.Millisecond)

	s.PostEventAny("f", "lp", "onselect", map[string]interface{}{
		"line_idx":  float64(5),
		"ctrl_name": "lb_name",
	})
	s.PostEventAny("f", "lp", "onclick", map[string]interface{}{
		"line_idx":  float64(6),
		"ctrl_name": "lb_qty",
	})

	deadline := time.After(4 * time.Second)
	for {
		sl := glua.LVAsNumber(s.GetGlobal("selected_line"))
		cc := s.GetGlobal("clicked_ctrl")
		cl := glua.LVAsNumber(s.GetGlobal("clicked_line"))
		if sl == 5 && cc == glua.LString("lb_qty") && cl == 6 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out: selected_line=%v clicked_ctrl=%v clicked_line=%v", sl, cc, cl)
		case <-time.After(20 * time.Millisecond):
		}
	}

	// The selection value table must not leak into the form's control values.
	formTbl, ok := s.GetGlobal("f").(*glua.LTable)
	if !ok {
		t.Fatalf("form f is not a table")
	}
	controls, ok := formTbl.RawGetString("controls").(*glua.LTable)
	if !ok {
		t.Fatalf("form f has no controls")
	}
	lp, ok := controls.RawGetString("lp").(*glua.LTable)
	if !ok {
		t.Fatalf("looper lp not found")
	}
	if v := lp.RawGetString("value"); v != glua.LNil {
		t.Errorf("looper control value set to %v, want nil", v)
	}
}

var _ = bindings.Options{}
