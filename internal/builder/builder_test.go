package builder

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleLua = `-- Sample grid form
function main()
  k.form.new("dashboard", {
    title = "Dashboard",
    layout = "grid",
    align = "center",
    gap = 16,
    cells = {
      {id = "header", width = 12, bg = "#f5f5f5", align = "center",
       border = {width = 1, color = "#ddd"}},
      {id = "sidebar", width = 3},
      {id = "main", width = 9, bg = "#fff"}
    }
  })

  k.ctrl.label("dashboard", "title_lbl", {text = "KPI Overview", cell = "header", multiline = true})
  k.ctrl.textbox("dashboard", "search", {label = "Search", cell = "header", value = ""})
  k.ctrl.button("dashboard", "btn_refresh", {label = "Refresh", cell = "header", onclick = function()
    k.ctrl.set_value("dashboard", "search", "go")
  end})
  k.ctrl.list("dashboard", "menu", {cell = "sidebar", items = {["1"] = "Dashboard", ["2"] = "Reports"}, size = 6})
  k.ctrl.chart("dashboard", "kpi", {type = "line", labels = {"Jan", "Feb"}, datasets = {{label = "Rev", data = {10, 20}}}})
  k.ctrl.table("dashboard", "tbl", {query = "SELECT * FROM x", page_size = 25, count_query = "SELECT count(*) FROM x"})
  k.form.on("dashboard", "btn_refresh", "click", function()
    k.print("refreshed")
  end)

  k.form.show("dashboard")
end
`

func TestImportLua(t *testing.T) {
	d, err := Import(sampleLua, "sample.lua")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	f := d.Form
	if f.Name != "dashboard" {
		t.Errorf("name=%q want dashboard", f.Name)
	}
	if f.Layout != "grid" || f.Align != "center" || f.Title != "Dashboard" {
		t.Errorf("form props wrong: %+v", f)
	}
	if f.Gap == nil || *f.Gap != 16 {
		t.Errorf("gap=%v want 16", f.Gap)
	}
	if len(f.Cells) != 3 {
		t.Fatalf("cells=%d want 3", len(f.Cells))
	}
	h := f.Cells["header"]
	if h.Width != 12 || h.Bg != "#f5f5f5" || h.Align != "center" || h.Border == nil || h.Border.Color != "#ddd" {
		t.Errorf("header cell wrong: %+v", h)
	}
	if len(f.Controls) != 6 {
		t.Fatalf("controls=%d want 6", len(f.Controls))
	}
	menu := findCtrl(f, "menu")
	if menu == nil {
		t.Fatal("menu control missing")
	}
	items, ok := menu.Opts["items"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("menu items wrong: %#v", menu.Opts["items"])
	}
	first := items[0].(map[string]any)
	if first["key"] != "1" || first["display"] != "Dashboard" {
		t.Errorf("item0=%#v", first)
	}
	tbl := findCtrl(f, "tbl")
	if tbl == nil || tbl.Opts["query"] != "SELECT * FROM x" || tbl.Opts["page_size"] != 25.0 {
		t.Errorf("table opts wrong: %+v", tbl)
	}
	evs := f.Handlers["btn_refresh"]
	if len(evs) != 1 || evs[0] != "click" {
		t.Errorf("handlers=%v", evs)
	}
	if len(f.Notes) == 0 {
		t.Error("expected import notes for onclick function body")
	}
}

func TestExportImportRoundTrip(t *testing.T) {
	d, err := Import(sampleLua, "sample.lua")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	lua := ExportLua(d)
	if !strings.Contains(lua, `k.form.new("dashboard", {`) {
		t.Error("export missing k.form.new")
	}
	if !strings.Contains(lua, `k.ctrl.list("dashboard", "menu"`) {
		t.Error("export missing list control")
	}
	// items must be exported as the runtime map literal
	if !strings.Contains(lua, `["1"] = "Dashboard"`) && !strings.Contains(lua, `[1] = "Dashboard"`) {
		t.Error("export items should be a map literal, got:\n" + lua)
	}

	d2, err := Import(lua, "roundtrip.lua")
	if err != nil {
		t.Fatalf("re-import: %v", err)
	}
	if len(d2.Form.Controls) != len(d.Form.Controls) {
		t.Errorf("control count mismatch: %d vs %d", len(d2.Form.Controls), len(d.Form.Controls))
	}
	for _, c := range d.Form.Controls {
		got := findCtrl(d2.Form, c.Name)
		if got == nil {
			t.Errorf("control %s lost in round trip", c.Name)
			continue
		}
		if got.Type != c.Type {
			t.Errorf("ctrl %s type %s != %s", c.Name, got.Type, c.Type)
		}
	}
}

func TestValidateDocument(t *testing.T) {
	d := &Document{Version: 1, Form: &Form{Name: "main", Layout: "grid", Controls: []*Control{
		{Name: "a", Type: "label", Opts: map[string]any{}},
		{Name: "a", Type: "textbox", Opts: map[string]any{}},
	}}}
	msgs := d.Validate()
	if len(msgs) != 1 || !strings.Contains(msgs[0], "duplicate") {
		t.Errorf("expected duplicate error, got %v", msgs)
	}
	d.Form.Layout = "broken"
	d.Form.Controls = d.Form.Controls[:1]
	msgs = d.Validate()
	if len(msgs) != 1 || !strings.Contains(msgs[0], "vertical") {
		t.Errorf("expected layout error, got %v", msgs)
	}
}

func TestPreview(t *testing.T) {
	d, err := Import(sampleLua, "sample.lua")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	html, err := Preview(d)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	for _, want := range []string{`id="f:dashboard"`, `data-k-ctrl="menu"`, `data-k-ctrl="btn_refresh"`, `data-k-cell="header"`, `data-k-cell="sidebar"`} {
		if !strings.Contains(html, want) {
			t.Errorf("preview missing %s", want)
		}
	}
	// grid cells must render with the span
	if !strings.Contains(html, `style="grid-column: span 3`) && !strings.Contains(html, `grid-column: span 3`) {
		t.Error("preview missing sidebar 3-column span")
	}
}

func findCtrl(f *Form, name string) *Control {
	for _, c := range f.Controls {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func startTestServer(t *testing.T, name, content string) (*Server, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if content != "" {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	srv, err := New(path, "127.0.0.1", 0)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go srv.Run(ctx)
	t.Cleanup(func() { cancel(); srv.Close() })
	return srv, path
}

func httpJSON(t *testing.T, method, url string, body any) (int, []byte) {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, url, rd)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(res.Body)
	return res.StatusCode, data
}

func TestServerLuaEndpoints(t *testing.T) {
	srv, path := startTestServer(t, "sample.lua", sampleLua)
	base := "http://" + srv.Addr()

	// GET /api/form — imports the Lua source
	code, body := httpJSON(t, "GET", base+"/api/form", nil)
	if code != 200 {
		t.Fatalf("GET /api/form status=%d body=%s", code, body)
	}
	var got struct {
		Path   string   `json:"path"`
		Format string   `json:"format"`
		Doc    Document `json:"doc"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.Doc.Form.Name != "dashboard" || got.Doc.Form.Layout != "grid" {
		t.Errorf("unexpected doc: %+v", got.Doc.Form)
	}

	// POST /api/preview
	code, body = httpJSON(t, "POST", base+"/api/preview", map[string]any{"doc": got.Doc})
	if code != 200 {
		t.Fatalf("preview status=%d body=%s", code, body)
	}
	var prev struct {
		HTML string `json:"html"`
	}
	if err := json.Unmarshal(body, &prev); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`data-k-ctrl="menu"`, `data-k-ctrl="btn_refresh"`, `data-k-cell="header"`, `data-k-cell="sidebar"`} {
		if !bytes.Contains([]byte(prev.HTML), []byte(want)) {
			t.Errorf("preview missing %s", want)
		}
	}

	// POST /api/validate — export then check
	code, body = httpJSON(t, "POST", base+"/api/export", map[string]any{"doc": got.Doc})
	if code != 200 {
		t.Fatalf("export failed: %s", body)
	}
	var out struct {
		Lua string `json:"lua"`
	}
	_ = json.Unmarshal(body, &out)
	code, body = httpJSON(t, "POST", base+"/api/validate", map[string]any{"lua": out.Lua})
	res := struct {
		OK     bool     `json:"ok"`
		Errors []string `json:"errors"`
	}{}
	if err := json.Unmarshal(body, &res); err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Errorf("validate not ok: %v", res.Errors)
	}

	// PUT /api/form — save as generated Lua, then re-load it
	code, body = httpJSON(t, "PUT", base+"/api/form", map[string]any{"doc": got.Doc})
	if code != 200 {
		t.Fatalf("PUT failed: %s", body)
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(saved), `k.form.new("dashboard"`) {
		t.Error("saved file is not generated Lua")
	}
}

func TestServerJSONDocument(t *testing.T) {
	doc := Document{Version: 1, Form: &Form{Name: "main", Title: "JSON form", Layout: "vertical", Align: "left", Controls: []*Control{
		{Name: "lbl1", Type: "label", Opts: map[string]any{"text": "Hello"}},
	}}}
	srv, path := startTestServer(t, "form.json", "")
	base := "http://" + srv.Addr()

	// PUT writes the document; GET reads it back.
	code, body := httpJSON(t, "PUT", base+"/api/form", map[string]any{"doc": doc})
	if code != 200 {
		t.Fatalf("PUT status=%d body=%s", code, body)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("json file not written")
	}
	code, body = httpJSON(t, "GET", base+"/api/form", nil)
	if code != 200 {
		t.Fatalf("GET status=%d body=%s", code, body)
	}
	var got struct {
		Format string   `json:"format"`
		Doc    Document `json:"doc"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.Format != "json" || got.Doc.Form.Controls[0].Opts["text"] != "Hello" {
		t.Errorf("round trip broken: %+v", got)
	}
}

func TestServerEmptyLuaFile(t *testing.T) {
	// A missing file should yield an empty form (start-from-empty).
	srv, _ := startTestServer(t, "app.lua", "")
	code, body := httpJSON(t, "GET", "http://"+srv.Addr()+"/api/form", nil)
	if code != 200 {
		t.Fatalf("status=%d body=%s", code, body)
	}
	var got struct {
		Message string   `json:"message"`
		Doc     Document `json:"doc"`
	}
	_ = json.Unmarshal(body, &got)
	if got.Doc.Form == nil || got.Doc.Form.Name == "" {
		t.Errorf("expected empty form scaffold, got %+v", got)
	}
	if got.Message == "" {
		t.Error("expected explanatory message")
	}
}

func TestChartConfigRoundTrip(t *testing.T) {
	src := `function main()
  k.form.new("c", {title = "C"})
  k.ctrl.chart("c", "ch", {type = "bar", labels = {"A","B"}, datasets = {{label = "S1", data = {1, 2, 3}}, {label = "S2", data = {4, 5}}}, options = {title = {display = true}}})
  k.form.show("c")
end`
	d, err := Import(src, "c.lua")
	if err != nil {
		t.Fatal(err)
	}
	ch := findCtrl(d.Form, "ch")
	ds, ok := ch.Opts["datasets"].([]any)
	if !ok || len(ds) != 2 {
		t.Fatalf("datasets not array: %#v", ch.Opts["datasets"])
	}
	ds0 := ds[0].(map[string]any)
	if ds0["label"] != "S1" {
		t.Errorf("ds0 label=%v", ds0["label"])
	}
	data := ds0["data"].([]any)
	if data[0] != 1.0 || data[1] != 2.0 {
		t.Errorf("data=%v", data)
	}
	lua := ExportLua(d)
	if !strings.Contains(lua, `datasets = {{`) || !strings.Contains(lua, `label = "S1"`) || !strings.Contains(lua, `data = {1, 2, 3}`) {
		t.Errorf("nested tables export wrong:\n%s", lua)
	}
	html, err := Preview(d)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if !strings.Contains(html, "kalua-chart-container") {
		t.Error("preview should render chart container")
	}
}