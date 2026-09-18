package builder

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"kalua/internal/bindings"
)

// sqlite driver (modernc.org/sqlite) is registered by internal/bindings, so an
// explicit blank import here is unnecessary.

func buildTableDoc(db string) *Document {
	return &Document{Version: DocVersion, Forms: []*Form{{
		Name: "main", Title: "DB table", Layout: "vertical",
		Controls: []*Control{{
			Name: "grid1", Type: "table",
			Opts: map[string]any{
				"db":      db,
				"query":   "SELECT 1",
				"columns": map[string]any{"id": "ID"},
			},
		}},
	}}}
}

// TestExportNamedDBLiteral: a static --db name exports as db = "NAME" without
// the runtime-handle hint comment; an opaque runtime id keeps the old behavior.
func TestExportNamedDBLiteral(t *testing.T) {
	// Static name.
	doc := buildTableDoc("main")
	out := ExportLua(doc)
	if !strings.Contains(out, `db = "main"`) {
		t.Errorf("export should contain db = \"main\":\n%s", out)
	}
	if strings.Contains(out, "assigned at runtime") {
		t.Errorf("export should not hint a runtime handle for a named DB:\n%s", out)
	}
	// Round-trip: re-import keeps the literal db.
	d2, err := Import(out, "sample.lua")
	if err != nil {
		t.Fatalf("re-import named db: %v", err)
	}
	if got := findCtrl(firstForm(d2), "grid1").Opts["db"]; got != "main" {
		t.Errorf("round-trip db = %v, want main", got)
	}

	// Opaque runtime id: skipped + hint comment, no literal.
	doc = buildTableDoc("db_0xc0000b0880")
	out = ExportLua(doc)
	if strings.Contains(out, `db = "db_0xc0000b0880"`) {
		t.Errorf("export must not emit a runtime handle id:\n%s", out)
	}
	if !strings.Contains(out, "assigned at runtime") {
		t.Errorf("export should keep the runtime-handle hint:\n%s", out)
	}
}

func TestBuilderDBEndpoints(t *testing.T) {
	dbName := "e2edb"
	if err := bindings.RegisterNamedDB(dbName, "sqlite://"+filepath.Join(t.TempDir(), "e2e.db")); err != nil {
		t.Fatal(err)
	}
	defer bindings.CloseNamedDBs()
	found := false
	for _, n := range bindings.NamedDBs() {
		if n == dbName {
			found = true
		}
	}
	if !found {
		t.Fatal("named db not registered")
	}

	srv, _ := startTestServer(t, "sample.lua", sampleLua)
	base := "http://" + srv.Addr()

	// GET /api/db lists the preregistered named handles.
	code, body := httpJSON(t, "GET", base+"/api/db", nil)
	if code != http.StatusOK {
		t.Fatalf("GET /api/db status=%d body=%s", code, body)
	}
	var list struct {
		Dbs []string `json:"dbs"`
	}
	_ = json.Unmarshal(body, &list)
	if len(list.Dbs) != 1 || list.Dbs[0] != dbName {
		t.Errorf("GET /api/db dbs = %v, want [%s]", list.Dbs, dbName)
	}

	// POST /api/db/query runs a read-only query on the named handle.
	code, body = httpJSON(t, "POST", base+"/api/db/query", map[string]any{"db": dbName, "query": "SELECT 1 AS one, 'x' AS two", "limit": 5})
	if code != http.StatusOK {
		t.Fatalf("POST /api/db/query status=%d body=%s", code, body)
	}
	var res struct {
		Columns []string `json:"columns"`
		Rows    [][]any  `json:"rows"`
	}
	_ = json.Unmarshal(body, &res)
	if len(res.Columns) != 2 || res.Columns[0] != "one" || res.Columns[1] != "two" {
		t.Errorf("query columns = %v, want [one two]", res.Columns)
	}
	if len(res.Rows) != 1 {
		t.Errorf("query rows = %d, want 1", len(res.Rows))
	}

	// Unknown DB → 400.
	code, _ = httpJSON(t, "POST", base+"/api/db/query", map[string]any{"db": "nope", "query": "SELECT 1"})
	if code != http.StatusBadRequest {
		t.Errorf("unknown db status = %d, want 400", code)
	}

	// Write statements → 400 even with a valid handle.
	code, body = httpJSON(t, "POST", base+"/api/db/query", map[string]any{"db": dbName, "query": "DROP TABLE t"})
	if code != http.StatusBadRequest {
		t.Errorf("write query status = %d body=%s, want 400", code, body)
	}
}

// TestBuilderTabulatorProxy verifies /static/tabulator proxy reaches the
// embedded runtime bundle.
func TestBuilderTabulatorProxy(t *testing.T) {
	srv, _ := startTestServer(t, "sample.lua", sampleLua)
	base := "http://" + srv.Addr()
	code, body := httpJSON(t, "GET", base+"/static/tabulator/tabulator.min.js", nil)
	if code != http.StatusOK {
		t.Fatalf("tabulator proxy status=%d", code)
	}
	if len(body) < 1000 || !strings.Contains(string(body[:len(body)]), "Tabulator") {
		t.Errorf("tabulator.min.js body looks wrong (%d bytes)", len(body))
	}
}

// TestBuilderLooperRowsEndpoint: with opts.row defs the endpoint answers
// server-rendered HTML sample rows; without them it falls back to the raw
// mini-grid, mirroring the runtime's legacy value-cell model.
func TestBuilderLooperRowsEndpoint(t *testing.T) {
	dbName := "loopdb"
	dbPath := filepath.Join(t.TempDir(), "looper.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("CREATE TABLE items (id INTEGER, name TEXT)"); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO items VALUES (1, '<b>one</b>'), (2, 'two')"); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := bindings.RegisterNamedDB(dbName, "sqlite://"+dbPath); err != nil {
		t.Fatal(err)
	}
	defer bindings.CloseNamedDBs()

	srv, _ := startTestServer(t, "sample.lua", sampleLua)
	base := "http://" + srv.Addr()

	row := []any{
		map[string]any{"type": "label", "name": "lb_id", "property": "text", "field": "id"},
		map[string]any{"type": "textbox", "name": "tx_name", "property": "value", "field": "name"},
	}
	code, body := httpJSON(t, "POST", base+"/api/looper/rows",
		map[string]any{"db": dbName, "query": "SELECT id, name FROM items ORDER BY id", "row": row, "limit": 10})
	if code != http.StatusOK {
		t.Fatalf("POST /api/looper/rows status=%d body=%s", code, body)
	}
	var res struct {
		Columns  []string `json:"columns"`
		HTMLRows []struct {
			Index int    `json:"index"`
			HTML  string `json:"html"`
		} `json:"html_rows"`
	}
	_ = json.Unmarshal(body, &res)
	if len(res.Columns) != 2 || res.Columns[0] != "id" {
		t.Errorf("columns = %v", res.Columns)
	}
	if len(res.HTMLRows) != 2 {
		t.Fatalf("html_rows = %d, want 2", len(res.HTMLRows))
	}
	if res.HTMLRows[0].Index != 1 {
		t.Errorf("html row 0 index = %d, want 1", res.HTMLRows[0].Index)
	}
	html := res.HTMLRows[0].HTML
	for _, want := range []string{
		`data-k-looper-index="1"`,
		`id="c:looper:lb_id:1"`,
		`id="c:looper:tx_name:1"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("html row missing %q:\n%s", want, html)
		}
	}
	if !strings.Contains(html, "&lt;b&gt;one&lt;/b&gt;") {
		t.Errorf("bound value not escaped in server-rendered row:\n%s", html)
	}

	// Without row defs → raw mini-grid.
	code, body = httpJSON(t, "POST", base+"/api/looper/rows",
		map[string]any{"db": dbName, "query": "SELECT id, name FROM items ORDER BY id LIMIT 2"})
	if code != http.StatusOK {
		t.Fatalf("legacy looper rows status=%d body=%s", code, body)
	}
	var legacy struct {
		Rows [][]any `json:"rows"`
	}
	_ = json.Unmarshal(body, &legacy)
	if len(legacy.Rows) != 2 {
		t.Errorf("legacy rows = %d, want 2", len(legacy.Rows))
	}
}

// TestExportLooperRow: a looper with opts.row (the new row-template model)
// exports both the row defs and the derived links, and re-import preserves both.
func TestExportLooperRow(t *testing.T) {
	doc := &Document{Version: DocVersion, Forms: []*Form{{
		Name: "main", Title: "Looper", Layout: "vertical",
		Controls: []*Control{{
			Name: "rows1", Type: "looper",
			Opts: map[string]any{
				"db":    "main",
				"query": "SELECT id, name FROM items",
				"row": []any{
					map[string]any{"type": "label", "name": "lb_id", "property": "text", "field": "id"},
					map[string]any{"type": "textbox", "name": "tx_name", "property": "value", "field": "name", "opts": map[string]any{"multiline": true}},
				},
				"links": []any{
					map[string]any{"field": "id", "control": "lb_id", "property": "text"},
					map[string]any{"field": "name", "control": "tx_name", "property": "value", "opts": map[string]any{}},
				},
			},
		}},
	}}}

	out := ExportLua(doc)
	if !strings.Contains(out, `row = {{field = "id", name = "lb_id", property = "text", type = "label"}`) {
		t.Errorf("export missing row defs:\n%s", out)
	}
	if !strings.Contains(out, `multiline = true`) {
		t.Errorf("export missing row cell opts:\n%s", out)
	}
	if !strings.Contains(out, `links = {{control = "lb_id", field = "id", property = "text"}`) {
		t.Errorf("export missing derived links:\n%s", out)
	}

	// Round-trip: re-import keeps row + links.
	d2, err := Import(out, "sample.lua")
	if err != nil {
		t.Fatalf("re-import looper row: %v", err)
	}
	o2 := findCtrl(firstForm(d2), "rows1").Opts
	if rowArr, ok := o2["row"].([]any); !ok || len(rowArr) != 2 {
		t.Errorf("round-trip row = %v", o2["row"])
	}
	if linksArr, ok := o2["links"].([]any); !ok || len(linksArr) != 2 {
		t.Errorf("round-trip links = %v", o2["links"])
	}
}

// TestExportLooperLegacyLinks: a looper with only legacy links (no row) keeps
// the old export shape — no row key is emitted.
func TestExportLooperLegacyLinks(t *testing.T) {
	doc := &Document{Version: DocVersion, Forms: []*Form{{
		Name: "main", Title: "Looper", Layout: "vertical",
		Controls: []*Control{{
			Name: "rows1", Type: "looper",
			Opts: map[string]any{
				"db":    "main",
				"query": "SELECT id, name FROM items",
				"links": []any{
					map[string]any{"field": "id", "control": "id", "property": "value"},
				},
			},
		}},
	}}}

	out := ExportLua(doc)
	if strings.Contains(out, "row =") {
		t.Errorf("legacy links-only looper must not export row:\n%s", out)
	}
	if !strings.Contains(out, `links = {{control = "id", field = "id", property = "value"}`) {
		t.Errorf("legacy links lost:\n%s", out)
	}
}
