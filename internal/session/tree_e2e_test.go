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

// TestRealTreeDataRequest runs a session whose Lua app connects a sqlite DB
// and declares a k.ctrl.tree over an adjacency-list categories table. The Go
// host answers the browser's tree_data_request with a tree_data payload whose
// nodes carry nested _children and the total count.
func TestRealTreeDataRequest(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "test.db")
	script := filepath.Join(tmp, "app.lua")
	src := `
function main()
  local db = k.connect_sqlite(%q)
  k.sql(db, "CREATE TABLE categories (id INTEGER PRIMARY KEY, parent_id INTEGER, name TEXT)")
  k.sql(db, "INSERT INTO categories VALUES (1, NULL, 'Electronics')")
  k.sql(db, "INSERT INTO categories VALUES (2, 1, 'Laptops')")
  k.sql(db, "INSERT INTO categories VALUES (3, 1, 'Phones')")
  k.sql(db, "INSERT INTO categories VALUES (4, 2, 'ThinkPad')")
  k.sql(db, "INSERT INTO categories VALUES (5, NULL, 'Books')")

  k.form.new("f", {title="t"})
  k.ctrl.tree("f", "tree", {
    db = db,
    table = "categories",
    label_field = "name",
    order_by = "name ASC",
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

	s.PostTreeDataRequest("f", "tree")

	deadline := time.After(4 * time.Second)
	var got string
	for {
		select {
		case w := <-out:
			if w.Type != "tree_data" {
				continue
			}
			if w.Ctrl != "tree" {
				t.Fatalf("tree_data ctrl = %q, want tree", w.Ctrl)
			}
			if w.Selector != "#c:f:tree" {
				t.Fatalf("tree_data selector = %q, want #c:f:tree", w.Selector)
			}
			got = w.Data
			goto done
		case <-deadline:
			t.Fatalf("timed out waiting for tree_data")
		}
	}
done:
	var payload struct {
		Nodes []struct {
			ID       float64 `json:"id"`
			Label    string  `json:"label"`
			Children []struct {
				ID    float64 `json:"id"`
				Label string  `json:"label"`
			} `json:"_children"`
		} `json:"nodes"`
		Count int `json:"count"`
	}
	if err := json.Unmarshal([]byte(got), &payload); err != nil {
		t.Fatalf("tree_data JSON: %v", err)
	}
	if payload.Count != 5 {
		t.Errorf("count = %d, want 5", payload.Count)
	}
	if len(payload.Nodes) != 2 {
		t.Fatalf("root nodes = %d, want 2 (Electronics, Books)", len(payload.Nodes))
	}
	// order_by = "name ASC" sorts roots: Books, Electronics.
	if payload.Nodes[0].Label != "Books" {
		t.Errorf("root[0] = %q, want Books (name ASC)", payload.Nodes[0].Label)
	}
	electronics := payload.Nodes[1]
	if electronics.Label != "Electronics" {
		t.Fatalf("root[1] = %q, want Electronics", electronics.Label)
	}
	if len(electronics.Children) != 2 {
		t.Fatalf("Electronics children = %d, want 2", len(electronics.Children))
	}
	if electronics.Children[0].Label != "Laptops" {
		t.Errorf("child[0] = %q, want Laptops", electronics.Children[0].Label)
	}
}

// TestTreeEvents verifies the browser event dispatch: on_click/on_select
// receive the node row map, on_expand receives (node_id, expanded), and the
// node table is never written into the form's control values by the generic
// updateControlValue path (on_select stores it explicitly so get_value works).
func TestTreeEvents(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "test.db")
	script := filepath.Join(tmp, "app.lua")
	src := `
function main()
  local db = k.connect_sqlite(%q)
  k.sql(db, "CREATE TABLE categories (id INTEGER PRIMARY KEY, parent_id INTEGER, name TEXT)")
  k.sql(db, "INSERT INTO categories VALUES (1, NULL, 'Electronics')")
  k.sql(db, "INSERT INTO categories VALUES (2, 1, 'Laptops')")

  k.form.new("f", {title="t"})
  k.ctrl.tree("f", "tree", {
    db = db,
    table = "categories",
    label_field = "name",
  })
  clicked = nil
  selected = nil
  expanded_id = nil
  expanded_flag = nil
  k.form.on("f", "tree", "on_click", function(node)
    clicked = node
  end)
  k.form.on("f", "tree", "on_select", function(node)
    selected = node
  end)
  k.form.on("f", "tree", "on_expand", function(node_id, expanded)
    expanded_id = node_id
    expanded_flag = expanded
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

	// on_click with the node row map.
	s.PostEventAny("f", "tree", "on_click", map[string]interface{}{
		"node": map[string]interface{}{
			"id": float64(2), "parent_id": float64(1), "name": "Laptops",
		},
	})
	// on_select with the same payload.
	s.PostEventAny("f", "tree", "on_select", map[string]interface{}{
		"node": map[string]interface{}{
			"id": float64(2), "parent_id": float64(1), "name": "Laptops",
		},
	})
	// on_expand with {node_id, expanded}.
	s.PostEventAny("f", "tree", "on_expand", map[string]interface{}{
		"node_id": float64(1), "expanded": true,
	})

	deadline := time.After(4 * time.Second)
	for {
		clicked := s.GetGlobal("clicked")
		selected := s.GetGlobal("selected")
		expID := glua.LVAsNumber(s.GetGlobal("expanded_id"))
		expFlag := s.GetGlobal("expanded_flag")
		if clicked != glua.LNil && selected != glua.LNil && expID == 1 && expFlag == glua.LTrue {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out: clicked=%v selected=%v expanded_id=%v expanded_flag=%v",
				clicked, selected, expID, expFlag)
		case <-time.After(20 * time.Millisecond):
		}
	}

	// on_click handler received the full row map.
	clickedTbl, ok := s.GetGlobal("clicked").(*glua.LTable)
	if !ok {
		t.Fatalf("clicked is not a table")
	}
	if clickedTbl.RawGetString("name").String() != "Laptops" {
		t.Errorf("clicked node name = %v, want Laptops", clickedTbl.RawGetString("name"))
	}
	if glua.LVAsNumber(clickedTbl.RawGetString("id")) != 2 {
		t.Errorf("clicked node id = %v, want 2", clickedTbl.RawGetString("id"))
	}

	// on_select stored the node as the control's value so get_value works.
	formTbl, ok := s.GetGlobal("f").(*glua.LTable)
	if !ok {
		t.Fatalf("form f is not a table")
	}
	controls, ok := formTbl.RawGetString("controls").(*glua.LTable)
	if !ok {
		t.Fatalf("form f has no controls")
	}
	treeCtrl, ok := controls.RawGetString("tree").(*glua.LTable)
	if !ok {
		t.Fatalf("tree control not found")
	}
	if v := treeCtrl.RawGetString("value"); v == glua.LNil {
		t.Error("on_select must store the selected node as the control value")
	} else if vt, ok := v.(*glua.LTable); !ok || vt.RawGetString("name").String() != "Laptops" {
		t.Errorf("control value = %v, want Laptops node map", v)
	}
}

// TestTreeErrorNoCrash verifies that a broken tree link (missing label_field)
// surfaces as an on_error event plus an empty-state tree_data payload, and the
// session keeps running.
func TestTreeErrorNoCrash(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "test.db")
	script := filepath.Join(tmp, "app.lua")
	src := `
function main()
  local db = k.connect_sqlite(%q)
  k.sql(db, "CREATE TABLE categories (id INTEGER PRIMARY KEY, parent_id INTEGER, name TEXT)")
  k.sql(db, "INSERT INTO categories VALUES (1, NULL, 'Electronics')")

  k.form.new("f", {title="t"})
  k.ctrl.tree("f", "tree", {
    db = db,
    table = "categories",
  })
  err_msg = nil
  k.form.on("f", "tree", "on_error", function(message)
    err_msg = message
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

	out := make(chan outboxWire, 32)
	go func() {
		for msg := range s.Outbox() {
			out <- outboxWire{Type: msg.Type, Selector: msg.Selector, Data: msg.Data, Form: msg.Form, Ctrl: msg.Ctrl}
		}
	}()

	time.Sleep(200 * time.Millisecond)

	s.PostTreeDataRequest("f", "tree")

	deadline := time.After(4 * time.Second)
	var got string
	for {
		select {
		case w := <-out:
			if w.Type != "tree_data" {
				continue
			}
			got = w.Data
			goto done
		case <-deadline:
			t.Fatalf("timed out waiting for tree_data error payload")
		}
	}
done:
	var payload struct {
		Error string `json:"error"`
		Count int    `json:"count"`
	}
	if err := json.Unmarshal([]byte(got), &payload); err != nil {
		t.Fatalf("tree_data error JSON: %v", err)
	}
	if payload.Error == "" {
		t.Errorf("error payload missing error message: %s", got)
	}
	if payload.Count != 0 {
		t.Errorf("error payload count = %d, want 0", payload.Count)
	}

	// The on_error handler fired with the message.
	deadline2 := time.After(4 * time.Second)
	for {
		if em := s.GetGlobal("err_msg"); em != glua.LNil {
			break
		}
		select {
		case <-deadline2:
			t.Fatalf("on_error handler never fired")
		case <-time.After(20 * time.Millisecond):
		}
	}
}
