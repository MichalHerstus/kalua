package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kalua/internal/bindings"
)

// TestRealTabulatorRemotePage runs a session whose Lua app registers a
// tabulator_ajax_request handler, posts the browser's page ask, and asserts a
// tabulator_remote_data outbox response carrying the sliced page + last_page.
func TestRealTabulatorRemotePage(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "app.lua")
	src := `
function main()
  k.form.new("f", {title="t"})
  k.ctrl.table("f", "big", {tabulator=true, tabulatorOptions={paginationMode="remote"}})

  local ALL = {}
  for i = 1, 25 do ALL[i] = {ord=i, name="User"..i} end

  k.form.on("f", "big", "tabulator_ajax_request", function(req)
    local page = tonum(req.page) or 1
    local size = tonum(req.size) or 10
    local start = (page - 1) * size + 1
    local stop = math.min(start + size - 1, #ALL)
    local slice = {}
    local n = 1
    for i = start, stop do slice[n] = ALL[i] n = n + 1 end
    return {data = slice, last_page = math.ceil(#ALL / size)}
  end)

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

	out := make(chan outboxWire, 16)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for msg := range s.Outbox() {
			out <- outboxWire{Type: msg.Type, Selector: msg.Selector, Data: msg.Data, Form: msg.Form, Ctrl: msg.Ctrl}
		}
	}()

	// Give the actor a moment to define main() and build the form.
	time.Sleep(150 * time.Millisecond)

	// Simulate the browser asking for page 3 of table "big".
	value := map[string]interface{}{
		"page": 3, "size": 10,
		"sort":   []interface{}{},
		"filter": []interface{}{},
	}
	s.PostTabulatorAjaxRequest("f", "big", value)

	deadline := time.After(3 * time.Second)
	for {
		select {
		case w := <-out:
			if w.Type != "tabulator_remote_data" {
				continue
			}
			if w.Ctrl != "big" {
				t.Fatalf("remote data ctrl = %q, want big", w.Ctrl)
			}
			var payload map[string]interface{}
			if err := json.Unmarshal([]byte(w.Data), &payload); err != nil {
				t.Fatalf("remote data JSON: %v", err)
			}
			rows, _ := payload["data"].([]interface{})
			lastPage, _ := payload["last_page"].(float64)
			if len(rows) != 5 {
				t.Fatalf("page 3 row count = %d, want 5 (last row 25)", len(rows))
			}
			if int(lastPage) != 3 {
				t.Fatalf("last_page = %v, want 3", lastPage)
			}
			if !strings.Contains(strings.TrimSpace(w.Data), `"ord":21`) {
				t.Errorf("page 3 should start at ord 21, got %s", w.Data)
			}
			return
		case <-deadline:
			t.Fatalf("timed out waiting for tabulator_remote_data")
		}
	}
}

// outboxWire is a serializable mirror of the outbox fields we assert on.
type outboxWire struct {
	Type     string
	Selector string
	Data     string
	Form     string
	Ctrl     string
}

var _ = bindings.Options{} // keep import used if the file is ever slimmed
