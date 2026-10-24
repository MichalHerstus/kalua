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

// TestTreeSetValueSingleUpdatePerClick reproduces the tree_demo scenario: a tree
// with an on_select handler that calls k.ctrl.set_value on a detail textbox.
//
// Each click must produce exactly one update_control outbox message for the
// detail control. The client (app.js updateControl) replaces the outer
// .kalua-control wrapper, so repeated k.ctrl.set_value calls never nest the
// control inside itself and never duplicate its label/field.
func TestTreeSetValueSingleUpdatePerClick(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "test.db")
	script := filepath.Join(tmp, "app.lua")
	src := `
function main()
  local db = k.connect_sqlite(%q)
  k.sql(db, "CREATE TABLE categories (id INTEGER PRIMARY KEY, parent_id INTEGER, name TEXT, note TEXT)")
  k.sql(db, "INSERT INTO categories VALUES (1, NULL, 'Electronics', 'Gadgets')")
  k.sql(db, "INSERT INTO categories VALUES (2, 1, 'Laptops', 'Portable')")

  k.form.new("main", { title = "Categories", layout = "vertical" })
  k.ctrl.tree("main", "tree", {
    db = db, table = "categories", label_field = "name", expand_all = true,
  })
  k.ctrl.textbox("main", "detail", { label = "Selected node", value = "(click a node)" })

  k.form.on("main", "tree", "on_click", function(node)
    k.print("clicked " .. node.name .. " (id=" .. tostr(node.id) .. ")")
  end)
  k.form.on("main", "tree", "on_select", function(node)
    k.ctrl.set_value("main", "detail", node.name .. " — " .. (node.note or ""))
  end)

  -- Handlers must be registered before form.show() (main suspends at show).
  k.form.show("main")
end
`
	if err := os.WriteFile(script, []byte(fmt.Sprintf(src, dbPath)), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := New("t1", script, bindings.Options{AllowFS: []string{tmp}}, tLogger{t: t})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	var detailUpdates []string
	go func() {
		for msg := range s.Outbox() {
			if msg.Type == "update_control" && msg.Ctrl == "detail" {
				detailUpdates = append(detailUpdates, msg.HTML)
			}
		}
	}()

	time.Sleep(300 * time.Millisecond)

	node := map[string]interface{}{
		"id": float64(2), "parent_id": float64(1), "name": "Laptops", "note": "Portable",
	}
	// Two clicks → two on_select events → exactly two update_control messages.
	s.PostEventAny("main", "tree", "on_select", map[string]interface{}{"node": node})
	s.PostEventAny("main", "tree", "on_select", map[string]interface{}{"node": node})

	time.Sleep(1 * time.Second)

	t.Logf("detail update_control count = %d", len(detailUpdates))
	for i, u := range detailUpdates {
		t.Logf("  detail update %d HTML: %s", i, u)
	}
	if len(detailUpdates) != 2 {
		t.Fatalf("expected exactly 2 update_control messages (one per click), got %d", len(detailUpdates))
	}

	// The tree control value stores the selected node (runFormHandler on_select branch).
	formTbl, _ := s.GetGlobal("main").(*glua.LTable)
	controls, _ := formTbl.RawGetString("controls").(*glua.LTable)
	treeCtrl, _ := controls.RawGetString("tree").(*glua.LTable)
	val := treeCtrl.RawGetString("value")
	if vt, ok := val.(*glua.LTable); ok {
		t.Logf("tree control value name = %q", vt.RawGetString("name").String())
	} else {
		t.Errorf("tree control value is not a node table: %v", val)
	}
}
