package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kalua/internal/bindings"
	lua "github.com/yuin/gopher-lua"
)

// TestRealGridDBLinked runs a session whose Lua app links a k.ctrl.grid to a
// sqlite source. A grid control opts into the shared Tabulator read pager, so
// the browser's ajax page request is served server-side without any Lua handler.
// It also verifies the initial render carries the .kalua-grid CRUD config and
// that k.grid.refresh (from a button onclick) pushes tabulator_refresh.
func TestRealGridDBLinked(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "test.db")
	script := filepath.Join(tmp, "app.lua")
	src := `
function main()
  local db = k.connect_sqlite(%q)
  k.sql(db, "CREATE TABLE users (id INTEGER, name TEXT)")
  for i = 1, 30 do
    k.sql(db, "INSERT INTO users (id, name) VALUES (?, ?)", i, "user" .. i)
  end

  k.form.new("f", {title="t"})
  k.ctrl.grid("f", "g1", {
    db = db,
    query = "SELECT id, name FROM users",
    pk_field = "id",
    page_size = 10,
    selection_mode = "single",
    row_actions = {view = true, delete = true},
  })
  k.ctrl.button("f", "btn", {label="Refresh", onclick=function() k.grid.refresh("f", "g1") end})
  k.form.show("f")
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

	out := make(chan outboxWire, 32)
	go func() {
		for msg := range s.Outbox() {
			out <- outboxWire{Type: msg.Type, Selector: msg.Selector, Data: msg.Data, Form: msg.Form, Ctrl: msg.Ctrl, HTML: msg.HTML}
		}
	}()

	// Initial render: the grid wrapper carries the CRUD config.
	for {
		select {
		case w := <-out:
			if w.Type != "render_form" {
				continue
			}
			got := w.HTML
			for _, want := range []string{
				`class="kalua-grid"`,
				`data-k-grid="1"`,
				`data-k-grid-pk="id"`,
				`data-k-grid-selection="single"`,
				`data-k-grid-row-actions`,
			} {
				if !strings.Contains(got, want) {
					t.Errorf("grid render missing %q", want)
				}
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for initial render_form")
		}
		break
	}

	// Browser asks page 2 (rows 11..20).
	s.PostTabulatorAjaxRequest("f", "g1", map[string]interface{}{
		"page": 2, "size": 10,
		"sort":   []interface{}{},
		"filter": []interface{}{},
	})

	deadline := time.After(4 * time.Second)
	var got string
	var ctrl string
	for {
		select {
		case w := <-out:
			if w.Type != "tabulator_remote_data" {
				continue
			}
			got = w.Data
			ctrl = w.Ctrl
			goto paged
		case <-deadline:
			t.Fatalf("timed out waiting for tabulator_remote_data")
		}
	}
paged:
	if ctrl != "g1" {
		t.Errorf("remote data ctrl = %q, want g1", ctrl)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(got), &payload); err != nil {
		t.Fatalf("remote data JSON: %v", err)
	}
	rows, _ := payload["data"].([]interface{})
	lastPage, _ := payload["last_page"].(float64)
	if len(rows) != 10 {
		t.Fatalf("page 2 row count = %d, want 10", len(rows))
	}
	if int(lastPage) != 3 {
		t.Errorf("last_page = %v, want 3", lastPage)
	}
	first, _ := rows[0].(map[string]interface{})
	if first["id"].(float64) != 11 {
		t.Errorf("page 2 first id = %v, want 11", first["id"])
	}

	// k.grid.refresh from a button onclick → tabulator_refresh for the grid.
	s.PostEvent("f", "btn", "click", nil)

	deadline = time.After(4 * time.Second)
	for {
		select {
		case w := <-out:
			if w.Type != "tabulator_refresh" {
				continue
			}
			if w.Selector != "#c:f:g1" {
				t.Fatalf("refresh selector = %q, want #c:f:g1", w.Selector)
			}
			return
		case <-deadline:
			t.Fatalf("timed out waiting for tabulator_refresh")
		}
	}
}

var _ = bindings.Options{} // keep import used if the file is ever slimmed

// TestGridFormOpenEditSaveCancel tests the full CRUD modal cycle:
// open edit form, save (insert), open view form, cancel, open new form, save (insert).
func TestGridFormOpenEditSaveCancel(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "test.db")
	script := filepath.Join(tmp, "app.lua")
	src := `
function main()
  local db = k.connect_sqlite(%q)
  k.sql(db, "CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT, value INTEGER)")
  k.form.new("f", {title="CRUD Test"})
  k.ctrl.grid("f", "g1", {
    db = db,
    query = "SELECT id, name, value FROM items",
    pk_field = "id",
    page_size = 10,
    selection_mode = "multi",
    row_actions = {view = true, edit = true, delete = true},
    global_actions = {new_record = true, batch_delete = true},
  })
  k.form.show("f")
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

	out := make(chan outboxWire, 32)
	go func() {
		for msg := range s.Outbox() {
			out <- outboxWire{Type: msg.Type, Selector: msg.Selector, Data: msg.Data, Form: msg.Form, Ctrl: msg.Ctrl, HTML: msg.HTML, Modal: msg.Modal, GridMode: msg.GridMode, GridPK: msg.GridPK, GridRow: msg.GridRow}
		}
	}()

	// Wait for initial render
	waitForRenderForm(out, "f")

	// 1. Open NEW record form (global action "new_record")
	s.PostGridFormOpen("f", "g1", map[string]interface{}{
		"mode": "new",
		"pk":   nil,
		"row":  map[string]interface{}{},
	})
	modalName, html := waitForRenderFormModal(out)
	if modalName == "" {
		t.Fatal("no modal rendered for new record")
	}
	if !strings.Contains(html, `data-k-grid-mode="new"`) {
		t.Errorf("new form should have mode=new, got: %s", html)
	}
	if !strings.Contains(html, `data-k-grid-pk=""`) {
		t.Errorf("new form should have empty pk, got: %s", html)
	}

	// 2. Save new record
	s.PostGridFormSave("f", "g1", map[string]interface{}{
		"mode": "new",
		"pk":   nil,
		"values": map[string]interface{}{
			"name":  "new_item",
			"value": 42,
		},
	})
	// Expect close_form + tabulator_refresh
	waitForCloseFormModal(out, modalName)
	waitForTabulatorRefresh(out, "f", "g1")

	// 3. Open EDIT form for the inserted row (pk=1)
	s.PostGridFormOpen("f", "g1", map[string]interface{}{
		"mode": "edit",
		"pk":   1,
		"row": map[string]interface{}{
			"id":    1,
			"name":  "new_item",
			"value": 42,
		},
	})
	modalName, html = waitForRenderFormModal(out)
	if modalName == "" {
		t.Fatal("no modal rendered for edit")
	}
	if !strings.Contains(html, `data-k-grid-mode="edit"`) {
		t.Errorf("edit form should have mode=edit, got: %s", html)
	}
	if !strings.Contains(html, `data-k-grid-pk="1"`) {
		t.Errorf("edit form should have pk=1, got: %s", html)
	}

	// 4. Save edit
	s.PostGridFormSave("f", "g1", map[string]interface{}{
		"mode": "edit",
		"pk":   1,
		"values": map[string]interface{}{
			"name":  "updated_item",
			"value": 99,
		},
	})
	waitForCloseFormModal(out, modalName)
	waitForTabulatorRefresh(out, "f", "g1")

	// 5. Open VIEW form (read-only)
	s.PostGridFormOpen("f", "g1", map[string]interface{}{
		"mode": "view",
		"pk":   1,
		"row": map[string]interface{}{
			"id":    1,
			"name":  "updated_item",
			"value": 99,
		},
	})
	modalName, html = waitForRenderFormModal(out)
	if modalName == "" {
		t.Fatal("no modal rendered for view")
	}
	if !strings.Contains(html, `data-k-grid-mode="view"`) {
		t.Errorf("view form should have mode=view, got: %s", html)
	}
	// View mode should have mode=view; the Save button is removed client-side

	// 6. Cancel view form
	s.PostGridFormCancel("f", "g1")
	waitForCloseFormModal(out, modalName)

	// 7. Open NEW form again and cancel it
	s.PostGridFormOpen("f", "g1", map[string]interface{}{
		"mode": "new",
		"pk":   nil,
		"row":  map[string]interface{}{},
	})
	modalName, _ = waitForRenderFormModal(out)
	s.PostGridFormCancel("f", "g1")
	waitForCloseFormModal(out, modalName)
}

// TestGridRowDeleteBatchDelete tests row and batch delete operations.
func TestGridRowDeleteBatchDelete(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "test.db")
	script := filepath.Join(tmp, "app.lua")
	src := `
function main()
  local db = k.connect_sqlite(%q)
  k.sql(db, "CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT)")
  for i = 1, 5 do
    k.sql(db, "INSERT INTO items (id, name) VALUES (?, ?)", i, "item" .. i)
  end
  k.form.new("f", {title="Delete Test"})
  k.ctrl.grid("f", "g1", {
    db = db,
    query = "SELECT id, name FROM items",
    pk_field = "id",
    page_size = 10,
    row_actions = {view = true, edit = true, delete = true},
    global_actions = {new_record = true, batch_delete = true},
    selection_mode = "multi",
  })
  k.form.show("f")
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

	out := make(chan outboxWire, 32)
	go func() {
		for msg := range s.Outbox() {
			out <- outboxWire{Type: msg.Type, Selector: msg.Selector, Data: msg.Data, Form: msg.Form, Ctrl: msg.Ctrl, HTML: msg.HTML, Modal: msg.Modal, GridMode: msg.GridMode, GridPK: msg.GridPK, GridRow: msg.GridRow}
		}
	}()

	waitForRenderForm(out, "f")

	// Single row delete (pk=3)
	s.PostGridRowDelete("f", "g1", map[string]interface{}{"pk": 3})
	waitForTabulatorRefresh(out, "f", "g1")

	// Batch delete (pks=1,2)
	s.PostGridBatchDelete("f", "g1", map[string]interface{}{"pks": []interface{}{1, 2}})
	waitForTabulatorRefresh(out, "f", "g1")
}

// TestGridValidationRejection tests server-side validation hook rejection.
func TestGridValidationRejection(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "test.db")
	script := filepath.Join(tmp, "app.lua")
	src := `
function main()
  local db = k.connect_sqlite(%q)
  k.sql(db, "CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT, value INTEGER)")
  k.form.new("f", {title="Validation Test"})

  -- Define the grid's detail form explicitly so it exists for k.form.on
  k.form.new("item_form", {title = "Item Details"})
  k.ctrl.textbox("item_form", "id", {label = "ID", opts = {enabled = false}})
  k.ctrl.textbox("item_form", "name", {label = "Name"})
  k.ctrl.textbox("item_form", "value", {label = "Value"})

  k.ctrl.grid("f", "g1", {
    db = db,
    query = "SELECT id, name, value FROM items",
    pk_field = "id",
    page_size = 10,
    form = "item_form",  -- referenced form
  })

  -- Server-side validation: reject negative values
  -- Use the referenced form name directly
  k.form.on("item_form", "on_save", function(data)
    k.print("VALIDATION HOOK: value = " .. tostring(data.values.value) .. ", type = " .. type(data.values.value))
    if data.values.value and data.values.value < 0 then
      return {ok = false, error = "value must be non-negative"}
    end
    return {ok = true}
  end)

  k.form.show("f")
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

	out := make(chan outboxWire, 32)
	go func() {
		for msg := range s.Outbox() {
			t.Logf("OUTBOX: type=%s form=%s ctrl=%s modal=%v", msg.Type, msg.Form, msg.Ctrl, msg.Modal)
			out <- outboxWire{Type: msg.Type, Selector: msg.Selector, Data: msg.Data, Form: msg.Form, Ctrl: msg.Ctrl, HTML: msg.HTML, Modal: msg.Modal, GridMode: msg.GridMode, GridPK: msg.GridPK, GridRow: msg.GridRow}
		}
	}()

	waitForRenderForm(out, "f")

	// Debug: check if handler is registered on item_form
	formTbl := s.L.GetGlobal("item_form")
	if formTbl != lua.LNil {
		if tbl, ok := formTbl.(*lua.LTable); ok {
			t.Logf("DEBUG: item_form exists, type=%T", tbl)
			if handlers := tbl.RawGetString("handlers"); handlers != lua.LNil {
				t.Logf("DEBUG: item_form has handlers table")
				if hTbl, ok := handlers.(*lua.LTable); ok {
					if formHandlers := hTbl.RawGetString("@form"); formHandlers != lua.LNil {
						t.Logf("DEBUG: item_form has @form handlers")
						if fhTbl, ok := formHandlers.(*lua.LTable); ok {
							fhTbl.ForEach(func(k, v lua.LValue) {
								t.Logf("DEBUG: handler key=%v", k)
							})
						}
					} else {
						t.Logf("DEBUG: item_form has NO @form handlers")
					}
				} else {
					t.Logf("DEBUG: item_form has NO handlers table")
				}
			} else {
				t.Logf("DEBUG: item_form has NO handlers key")
			}
		}
	} else {
		t.Logf("DEBUG: item_form NOT found as global")
	}

	// Try to save with negative value - should be rejected
	s.PostGridFormSave("f", "g1", map[string]interface{}{
		"mode": "new",
		"pk":   nil,
		"values": map[string]interface{}{
			"name":  "bad_item",
			"value": -10,
		},
	})

	// Debug: check if handler is registered on item_form AFTER the save attempt
	formTbl = s.L.GetGlobal("item_form")
	if formTbl != lua.LNil {
		if tbl, ok := formTbl.(*lua.LTable); ok {
			if handlers := tbl.RawGetString("handlers"); handlers != lua.LNil {
				if hTbl, ok := handlers.(*lua.LTable); ok {
					if formHandlers := hTbl.RawGetString("@form"); formHandlers != lua.LNil {
						if fhTbl, ok := formHandlers.(*lua.LTable); ok {
							fhTbl.ForEach(func(k, v lua.LValue) {
								t.Logf("DEBUG AFTER: handler key=%v", k)
							})
						}
					}
				}
			}
		}
	}

	// Expect error outbox (modal should stay open)
	select {
	case w := <-out:
		t.Logf("RECEIVED: type=%s", w.Type)
		if w.Type != "error" {
			t.Errorf("expected error outbox, got %q", w.Type)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("timed out waiting for error")
	}

	// Debug: check if handler is registered on item_form AFTER the save attempt
	formTbl2 := s.L.GetGlobal("item_form")
	if formTbl2 != lua.LNil {
		if tbl, ok := formTbl2.(*lua.LTable); ok {
			if handlers := tbl.RawGetString("handlers"); handlers != lua.LNil {
				if hTbl, ok := handlers.(*lua.LTable); ok {
					if formHandlers := hTbl.RawGetString("@form"); formHandlers != lua.LNil {
						if fhTbl, ok := formHandlers.(*lua.LTable); ok {
							fhTbl.ForEach(func(k, v lua.LValue) {
								t.Logf("DEBUG AFTER: handler key=%v", k)
							})
						}
					}
				}
			}
		}
	}

	// Modal should still be open - no close_form
	select {
	case w := <-out:
		t.Logf("RECEIVED2: type=%s", w.Type)
		if w.Type == "close_form" && w.Modal {
			t.Errorf("modal should not close on validation failure")
		}
	case <-time.After(1 * time.Second):
		// Expected: no close_form
	}
}

// TestGridDefaultActions verifies defaults when row_actions/global_actions omitted.
func TestGridDefaultActions(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "test.db")
	script := filepath.Join(tmp, "app.lua")
	src := `
function main()
  local db = k.connect_sqlite(%q)
  k.sql(db, "CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT)")
  k.form.new("f", {title="Defaults Test"})
  k.ctrl.grid("f", "g1", {
    db = db,
    query = "SELECT id, name FROM items",
    pk_field = "id",
    page_size = 10,
    -- No row_actions or global_actions specified - should get defaults
  })
  k.form.show("f")
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

	out := make(chan outboxWire, 32)
	errChan := make(chan error, 1)
	go func() {
		for msg := range s.Outbox() {
			t.Logf("OUTBOX: type=%s form=%s ctrl=%s modal=%v", msg.Type, msg.Form, msg.Ctrl, msg.Modal)
			out <- outboxWire{Type: msg.Type, Selector: msg.Selector, Data: msg.Data, Form: msg.Form, Ctrl: msg.Ctrl, HTML: msg.HTML, Modal: msg.Modal, GridMode: msg.GridMode, GridPK: msg.GridPK, GridRow: msg.GridRow}
		}
		errChan <- nil
	}()

	renderHTML := waitForRenderForm(out, "f")

	if !strings.Contains(renderHTML, `data-k-grid-row-actions`) {
		t.Errorf("default row_actions should be rendered")
	}
	if !strings.Contains(renderHTML, `data-k-grid-global-actions`) {
		t.Errorf("default global_actions should be rendered")
	}
}

// --- Test Helpers ---

func waitForRenderForm(out <-chan outboxWire, form string) string {
	deadline := time.After(5 * time.Second)
	for {
		select {
		case w := <-out:
			fmt.Printf("DEBUG waitForRenderForm RECEIVED: type=%s form=%q modal=%v (want form=%q modal=false)\n", w.Type, w.Form, w.Modal, form)
			if w.Type == "render_form" && w.Form == form && !w.Modal {
				fmt.Printf("DEBUG: MATCHED!\n")
				return w.HTML
			}
		case <-deadline:
			panic("timed out waiting for render_form " + form)
		}
	}
}

func waitForRenderFormModal(out <-chan outboxWire) (modalName, html string) {
	deadline := time.After(5 * time.Second)
	for {
		select {
		case w := <-out:
			if w.Type == "render_form" && w.Modal {
				return w.Form, w.HTML
			}
		case <-deadline:
			panic("timed out waiting for modal render_form")
		}
	}
}

func waitForCloseFormModal(out <-chan outboxWire, modalName string) {
	deadline := time.After(5 * time.Second)
	for {
		select {
		case w := <-out:
			if w.Type == "close_form" && w.Form == modalName && w.Modal {
				return
			}
		case <-deadline:
			panic("timed out waiting for close_form modal " + modalName)
		}
	}
}

func waitForTabulatorRefresh(out <-chan outboxWire, form, ctrl string) {
	deadline := time.After(5 * time.Second)
	for {
		select {
		case w := <-out:
			if w.Type == "tabulator_refresh" && w.Form == form && w.Ctrl == ctrl {
				return
			}
		case <-deadline:
			panic("timed out waiting for tabulator_refresh " + form + "/" + ctrl)
		}
	}
}

func waitForGridBatchDeleteResp(out <-chan outboxWire) {
	deadline := time.After(5 * time.Second)
	for {
		select {
		case w := <-out:
			if w.Type == "grid_batch_delete_resp" {
				return
			}
		case <-deadline:
			panic("timed out waiting for grid_batch_delete_resp")
		}
	}
}