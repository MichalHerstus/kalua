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

	"kalua/internal/checker"
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
	f := firstForm(d)
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
	h := findCell(f, "header")
	if h == nil || h.Width != 12 || h.Bg != "#f5f5f5" || h.Align != "center" || h.Border == nil || h.Border.Color != "#ddd" {
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

func TestButtonsOnePerRow(t *testing.T) {
	// Vertical layout: each button renders on its own full-width row — no
	// horizontal grouping wrapper, regardless of newRow/hGap opts (which are
	// still parsed and round-tripped for backward compatibility).
	btnLua := `function main()
  k.form.new("main", { title="T" })
  k.ctrl.button("main", "b1", { label = "Save", hGap = 12 })
  k.ctrl.button("main", "b2", { label = "Cancel" })
  k.ctrl.button("main", "b3", { label = "Delete", newRow = true })
  k.form.show("main")
end
`
	d, err := Import(btnLua, "btns.lua")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if b1 := findCtrl(firstForm(d), "b1"); b1 == nil || b1.Opts["hGap"] != float64(12) {
		t.Fatalf("b1 opts=%#v, want hGap=12", b1.Opts)
	}
	if b3 := findCtrl(firstForm(d), "b3"); b3 == nil || b3.Opts["newRow"] != true {
		t.Fatalf("b3 opts=%#v, want newRow=true", b3.Opts)
	}
	html, err := Preview(d)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if strings.Contains(html, "kalua-button-row") || strings.Contains(html, "kalua-button-newrow") {
		t.Errorf("unexpected button-row/newrow markup: %s", html)
	}
	for _, need := range []string{`id="c:main:b1"`, `id="c:main:b2"`, `id="c:main:b3"`} {
		if !strings.Contains(html, need) {
			t.Errorf("preview missing %q: %s", need, html)
		}
	}
	out := ExportLua(d)
	if !strings.Contains(out, `hGap = 12`) || !strings.Contains(out, `newRow = true`) {
		t.Errorf("export dropped newRow/hGap:\n%s", out)
	}
}

func TestLabelControlTextCanonicalization(t *testing.T) {
	// label controls: text is the canonical option; "label" is an accepted
	// alias that Import rewrites to "text" so export + preview agree.
	labelLua := `function main()
  k.form.new("main", { title="T" })
  k.ctrl.label("main", "lbl", { label = "Hello" })
  k.ctrl.label("main", "lbl2", { text = "Already" })
  k.form.show("main")
end
`
	d, err := Import(labelLua, "label.lua")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	lbl := findCtrl(firstForm(d), "lbl")
	if lbl == nil || lbl.Opts["text"] != "Hello" {
		t.Fatalf("lbl opts=%#v, want text=Hello", lbl.Opts)
	}
	if _, hasLabel := lbl.Opts["label"]; hasLabel {
		t.Errorf("lbl still carries 'label' opt after canonicalization: %#v", lbl.Opts)
	}
	lbl2 := findCtrl(firstForm(d), "lbl2")
	if lbl2 == nil || lbl2.Opts["text"] != "Already" {
		t.Errorf("lbl2 opts=%#v, want text=Already", lbl2.Opts)
	}
	html, err := Preview(d)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	for _, want := range []string{`>Hello</label>`, `>Already</label>`} {
		if !strings.Contains(html, want) {
			t.Errorf("preview missing label text %q", want)
		}
	}
	out := ExportLua(d)
	if !strings.Contains(out, `{text = "Hello"}`) {
		t.Errorf("export does not emit text= for label ctrl:\n%s", out)
	}
	if strings.Contains(out, "label = ") {
		t.Errorf("export still emits label= for label ctrl:\n%s", out)
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
	if len(firstForm(d2).Controls) != len(firstForm(d).Controls) {
		t.Errorf("control count mismatch: %d vs %d", len(firstForm(d2).Controls), len(firstForm(d).Controls))
	}
	for _, c := range firstForm(d).Controls {
		got := findCtrl(firstForm(d2), c.Name)
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
	d := &Document{Version: DocVersion, Forms: []*Form{&Form{Name: "main", Layout: "grid", Controls: []*Control{
		{Name: "a", Type: "label", Opts: map[string]any{}},
		{Name: "a", Type: "textbox", Opts: map[string]any{}},
	}}}}
	f := firstForm(d)
	msgs := d.Validate()
	if len(msgs) != 1 || !strings.Contains(msgs[0], "duplicate") {
		t.Errorf("expected duplicate error, got %v", msgs)
	}
	f.Layout = "broken"
	f.Controls = f.Controls[:1]
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

func TestCellsOrderPreserved(t *testing.T) {
	src := `function main()
  k.form.new("dash", {layout = "grid", cells = {
    {id = "footer", width = 12},
    {id = "header", width = 12},
    {id = "sidebar", width = 3},
  }})
  k.ctrl.label("dash", "title_lbl", {cell = "header"})
  k.ctrl.button("dash", "b1", {label = "Go", cell = "footer"})
  k.form.show("dash")
end`
	d, err := Import(src, "sample.lua")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	ids := make([]string, 0, len(firstForm(d).Cells))
	for _, c := range firstForm(d).Cells {
		ids = append(ids, c.Id)
	}
	if len(ids) != 3 || ids[0] != "footer" || ids[1] != "header" || ids[2] != "sidebar" {
		t.Errorf("import must preserve declared cell order, got %v", ids)
	}

	// Export round trip keeps the order.
	lua := ExportLua(d)
	d2, err := Import(lua, "sample.lua")
	if err != nil {
		t.Fatalf("re-Import: %v", err)
	}
	ids2 := make([]string, 0, len(firstForm(d2).Cells))
	for _, c := range firstForm(d2).Cells {
		ids2 = append(ids2, c.Id)
	}
	if strings.Join(ids2, ",") != "footer,header,sidebar" {
		t.Errorf("cell order lost on round trip, got %v", ids2)
	}

	// Preview must render the cells in declared order.
	html, err := Preview(d)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	iFooter := strings.Index(html, `data-k-cell="footer"`)
	iHeader := strings.Index(html, `data-k-cell="header"`)
	iSidebar := strings.Index(html, `data-k-cell="sidebar"`)
	if iFooter < 0 || iHeader < 0 || iSidebar < 0 {
		t.Error("preview missing a cell")
	} else if !(iFooter < iHeader && iHeader < iSidebar) {
		t.Errorf("preview cell order wrong: footer@%d header@%d sidebar@%d", iFooter, iHeader, iSidebar)
	}
}

func TestLegacyCellsMigration(t *testing.T) {
	// A v1 document with object-form cells must upgrade to an ordered v2 array.
	legacy := map[string]any{
		"version": float64(1),
		"form": map[string]any{
			"name":   "old",
			"layout": "grid",
			"cells": map[string]any{
				"main": map[string]any{"width": float64(9)},
				"side": map[string]any{"width": float64(3)},
			},
		},
	}
	b, _ := json.MarshalIndent(legacy, "", "  ")
	srv, _ := startTestServer(t, "form.json", string(b))
	base := "http://" + srv.Addr()
	code, body := httpJSON(t, "GET", base+"/api/form", nil)
	if code != 200 {
		t.Fatalf("GET status=%d body=%s", code, body)
	}
	var got struct {
		Doc Document `json:"doc"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.Doc.Version != DocVersion {
		t.Errorf("expected version %d, got %d", DocVersion, got.Doc.Version)
	}
	if len(firstForm(&got.Doc).Cells) != 2 {
		t.Errorf("expected 2 cells, got %d", len(firstForm(&got.Doc).Cells))
	}
	ids := make([]string, 0, len(firstForm(&got.Doc).Cells))
	for _, c := range firstForm(&got.Doc).Cells {
		ids = append(ids, c.Id)
	}
	if strings.Join(ids, ",") != "main,side" {
		t.Errorf("migration must preserve sorted cell ids, got %v", ids)
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

func findCell(f *Form, id string) *CellDef {
	for _, c := range f.Cells {
		if c != nil && c.Id == id {
			return c
		}
	}
	return nil
}

func formNames(d *Document) string {
	var names []string
	for _, f := range d.Forms {
		if f != nil {
			names = append(names, f.Name)
		}
	}
	return "[" + strings.Join(names, ", ") + "]"
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
	if firstForm(&got.Doc).Name != "dashboard" || firstForm(&got.Doc).Layout != "grid" {
		t.Errorf("unexpected doc: %+v", firstForm(&got.Doc))
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
	doc := Document{Version: DocVersion, Forms: []*Form{&Form{Name: "main", Title: "JSON form", Layout: "vertical", Align: "left", Controls: []*Control{
		{Name: "lbl1", Type: "label", Opts: map[string]any{"text": "Hello"}},
	}}}}
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
	if got.Format != "json" || firstForm(&got.Doc).Controls[0].Opts["text"] != "Hello" {
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
	if firstForm(&got.Doc) == nil || firstForm(&got.Doc).Name == "" {
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
	ch := findCtrl(firstForm(d), "ch")
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

const handlerLua = `-- handler preservation fixture
function main()
  k.form.new("main", {title = "Login"})
  k.ctrl.label("main", "lbl", {text = "Hi"})
  k.ctrl.button("main", "save", {
    label = "Save",
    onclick = function()
      local n = k.ctrl.get_value("main", "name")
      if n == "" then
        k.msgbox("empty")
      else
        k.msgbox("hello " .. n)
      end
    end,
  })
  k.ctrl.button("main", "cancel", {label = "Cancel", onclick = function() k.form.close("main") end})
  k.form.on("main", "save", "key_pressed", function()
    for i = 1, 3 do
      k.print("key", i)
    end
    k.yield()
  end)
  k.form.show("main")
end
`

func TestImportCapturesHandlerBodies(t *testing.T) {
	d, err := Import(handlerLua, "handlers.lua")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	save := findCtrl(firstForm(d), "save")
	if save == nil {
		t.Fatal("save control missing")
	}
	if !strings.Contains(save.Inline["onclick"], "k.ctrl.get_value") {
		t.Errorf("onclick body not captured: %+v", save.Inline)
	}
	cancel := findCtrl(firstForm(d), "cancel")
	if cancel == nil || !strings.Contains(cancel.Inline["onclick"], "k.form.close") {
		t.Errorf("cancel onclick not captured: %+v", dsInline(cancel))
	}
	body := firstForm(d).HandlerBodies["save.key_pressed"]
	if body == "" || !strings.Contains(body, "k.print(\"key\", i)") {
		t.Errorf("form.on body not captured: %q", body)
	}
	// Notes should now be informative, not the old jargon.
	found := false
	for _, n := range firstForm(d).Notes {
		if strings.Contains(n, "event handler") && strings.Contains(n, "preserved") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected reworded handler note, got %v", firstForm(d).Notes)
	}
	if strings.Contains(strings.Join(firstForm(d).Notes, ","), "cannot be serialized") {
		t.Error("old jargon still present")
	}
}

func TestExportPreservesHandlerBodies(t *testing.T) {
	d, err := Import(handlerLua, "handlers.lua")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	lua := ExportLua(d)
	if !strings.Contains(lua, `onclick = function()`) {
		t.Error("export dropped onclick=function:")
		t.Error(lua)
	}
	if !strings.Contains(lua, "k.ctrl.get_value") {
		t.Error("export lost onclick body")
	}
	if !strings.Contains(lua, `k.form.on("main", "save", "key_pressed", function()`) {
		t.Error("export dropped k.form.on body")
	}
	if !strings.Contains(lua, `k.print("key", i)`) {
		t.Error("export lost form.on body")
	}
	// The captured body must replace the lost TODO scaffold.
	if strings.Contains(lua, "TODO: handle") {
		t.Error("export still emits TODO scaffold despite captured body")
	}

	d2, err := Import(lua, "roundtrip2.lua")
	if err != nil {
		t.Fatalf("re-import: %v", err)
	}
	save2 := findCtrl(firstForm(d2), "save")
	if save2 == nil || !strings.Contains(save2.Inline["onclick"], "if n == \"\" then") {
		t.Errorf("handlers lost across re-import: %+v", dsInline(save2))
	}
	if !strings.Contains(firstForm(d2).HandlerBodies["save.key_pressed"], "for i = 1, 3 do") {
		t.Errorf("form.on body lost across re-import: %q", firstForm(d2).HandlerBodies["save.key_pressed"])
	}
}

func TestExportedHandlersAreValidLua(t *testing.T) {
	d, err := Import(handlerLua, "handlers.lua")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	lua := ExportLua(d)
	res := checker.Check(lua, "generated.lua")
	if len(res.Errors) > 0 {
		t.Errorf("generated (with re-printed handlers) does not parse:\n%v\n---\n%s", res.Errors, lua)
	}
}

func TestImportCapturesFormLevelHandlers(t *testing.T) {
	src := `function main()
  k.form.new("main", {title = "T"})
  k.ctrl.button("main", "go", {label = "Go"})
  k.form.on("main", "open_form", function()
    k.print("welcome")
  end)
  k.form.show("main")
end`
	d, err := Import(src, "formlevel.lua")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	evs := firstForm(d).Handlers["@form"]
	if len(evs) != 1 || evs[0] != "open_form" {
		t.Errorf("form-level handlers=%v want [open_form]", evs)
	}
	if !strings.Contains(firstForm(d).HandlerBodies["@form.open_form"], "welcome") {
		t.Errorf("form-level body not captured: %q", firstForm(d).HandlerBodies["@form.open_form"])
	}
	// Export re-emits the 3-arg form and must NOT add a stub for open_form
	// (the handler already exists), while close_form still gets a stub.
	lua := ExportLua(d)
	if !strings.Contains(lua, `k.form.on("main", "open_form", function()`) {
		t.Errorf("3-arg form.on not re-emitted:\n%s", lua)
	}
	if strings.Contains(lua, `k.print("main.open_form")`) {
		t.Errorf("stub generated despite existing open_form handler:\n%s", lua)
	}
	if !strings.Contains(lua, `k.form.on("main", "close_form", function()`) {
		t.Errorf("close_form stub missing:\n%s", lua)
	}
}

func TestExportEmitsDefaultHandlerStubs(t *testing.T) {
	// A document with no imported handlers exports trace stubs for the form
	// lifecycle and each control's documented events.
	d := &Document{Version: DocVersion, Forms: []*Form{&Form{
		Name:   "main",
		Layout: "vertical",
		Controls: []*Control{
			{Name: "lbl",   Type: "label",     Opts: map[string]any{"text": "Hi"}},
			{Name: "name",  Type: "textbox",   Opts: map[string]any{}},
			{Name: "go",    Type: "button",    Opts: map[string]any{}},
			{Name: "pic",   Type: "image",     Opts: map[string]any{"src": "/x.png"}},
			{Name: "pic2",  Type: "image",     Opts: map[string]any{"src": "/y.png", "clickable": true}},
			{Name: "chart1", Type: "chart",    Opts: map[string]any{"type": "line"}},
			{Name: "list1", Type: "list",      Opts: map[string]any{}},
			{Name: "loop1", Type: "looper",    Opts: map[string]any{}},
		},
		Handlers:      map[string][]string{},
		HandlerBodies: map[string]string{},
	}}}
	lua := ExportLua(d)
	for _, want := range []string{
		`k.form.on("main", "open_form", function()`,
		`k.print("main.open_form")`,
		`k.form.on("main", "after_open_form", function()`,
		`k.form.on("main", "close_form", function()`,
		`k.print("main.close_form")`,
		`k.form.on("main", "name", "selection_change", function()`,
		`k.print("main.name.selection_change")`,
		`k.form.on("main", "go", "click", function()`,
		`k.print("main.go.click")`,
		`k.form.on("main", "pic2", "click", function()`,
		`k.print("main.pic2.click")`,
		`k.form.on("main", "chart1", "chart_click", function()`,
		`k.form.on("main", "chart1", "chart_legend_click", function()`,
		`k.form.on("main", "list1", "selection_change", function()`,
		`k.form.on("main", "loop1", "onselect", function()`,
		`k.form.on("main", "loop1", "onclick", function()`,
	} {
		if !strings.Contains(lua, want) {
			t.Errorf("export missing stub %q:\n%s", want, lua)
		}
	}
	// Non-clickable images get no click stub; labels get no stubs at all.
	if strings.Contains(lua, `k.form.on("main", "pic", "click"`) {
		t.Errorf("non-clickable image should have no click stub:\n%s", lua)
	}
	if strings.Contains(lua, `k.form.on("main", "lbl"`) {
		t.Errorf("label control should have no handler stubs:\n%s", lua)
	}
	// The generated file must be valid Lua.
	res := checker.Check(lua, "generated.lua")
	if len(res.Errors) > 0 {
		t.Errorf("stubbed export does not parse:\n%v\n---\n%s", res.Errors, lua)
	}
}

func TestExportSkipsExistingHandlers(t *testing.T) {
	// Imported handlers (inline onclick and k.form.on) are preserved as-is and
	// suppress the generated stubs for the same (ctrl, event) pairs.
	d, err := Import(handlerLua, "handlers.lua")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	lua := ExportLua(d)
	// save/cancel have inline onclick → no click stubs, no clobbering.
	if strings.Contains(lua, `k.print("main.save.click")`) || strings.Contains(lua, `k.print("main.cancel.click")`) {
		t.Errorf("inline onclick should suppress the click stub:\n%s", lua)
	}
	// key_pressed was captured → preserved verbatim, nothing falls to TODO.
	if !strings.Contains(lua, `k.print("key", i)`) || strings.Contains(lua, "TODO: handle") {
		t.Errorf("captured key_pressed body mishandled:\n%s", lua)
	}
	// Form lifecycle has no handlers → stubs are still emitted.
	if !strings.Contains(lua, `k.form.on("main", "open_form", function()`) {
		t.Errorf("form-level stub missing:\n%s", lua)
	}
	// Round trip: re-imported stubs are preserved and not duplicated.
	d2, err := Import(lua, "roundtrip.lua")
	if err != nil {
		t.Fatalf("re-import: %v", err)
	}
	lua2 := ExportLua(d2)
	if strings.Count(lua2, `k.form.on("main", "open_form", function()`) > 1 {
		t.Errorf("form stub duplicated on round trip:\n%s", lua2)
	}
	if strings.Count(lua2, `k.print("main.open_form")`) > 1 {
		t.Errorf("form stub body duplicated on round trip:\n%s", lua2)
	}
}

func dsInline(c *Control) map[string]string {
	if c == nil {
		return nil
	}
	return c.Inline
}

func TestMergeDocumentKeepsBaseOnlyControls(t *testing.T) {
	base, _ := Import(sampleLua, "base.lua")
	gen, _ := Import(handlerLua, "gen.lua") // different name → separate form
	merged := MergeDocument(base, gen)

	// Both forms survive: base "dashboard" (untouched) and gen "main" appended.
	if len(merged.Forms) != 2 {
		t.Fatalf("forms=%d want 2 (base + gen)", len(merged.Forms))
	}
	dash := findForm(merged, "dashboard")
	main_ := findForm(merged, "main")
	if dash == nil || main_ == nil {
		t.Fatal("both forms must survive the merge")
	}
	// base form keeps its own scaffold (gen has a different name)
	if dash.Title != "Dashboard" || dash.Layout != "grid" {
		t.Errorf("base form scaffold changed: %+v", dash)
	}
	// base controls kept: title_lbl, search, btn_refresh, menu, kpi, tbl
	if findCtrl(dash, "title_lbl") == nil {
		t.Error("title_lbl (base-only) should be preserved")
	}
	if findCtrl(dash, "tbl") == nil {
		t.Error("tbl (base-only) should be preserved")
	}
	// base cells preserved (gen form has none, different name)
	hdr := findCell(dash, "header")
	if hdr == nil || hdr.Width != 12 {
		t.Errorf("header cell missing or replaced: %+v", hdr)
	}
	// gen form appended with its controls: lbl, save, cancel
	if findCtrl(main_, "lbl") == nil {
		t.Error("lbl (gen) should be in the appended form")
	}
	if findCtrl(main_, "save") == nil {
		t.Error("save (gen) should be in the appended form")
	}
	// gen form order: after the base form
	if merged.Forms[0].Name != "dashboard" || merged.Forms[1].Name != "main" {
		t.Errorf("form order wrong: %v", formNames(merged))
	}
}

func TestMergeDocumentOverlayOpts(t *testing.T) {
	srcA := `function main()
  k.form.new("f", {title = "T", layout = "grid", cells = {{id="main", width = 12}}})
  k.ctrl.button("f", "btn", {label = "Go", enabled = true, cell = "main"})
  k.form.show("f")
end`
	srcB := `function main()
  k.form.new("f", {title = "T2"})
  k.ctrl.button("f", "btn", {label = "Go now", class = "special"})
  k.form.show("f")
end`
	base, _ := Import(srcA, "a.lua")
	gen, _ := Import(srcB, "b.lua")
	merged := MergeDocument(base, gen)

	btn := findCtrl(firstForm(merged), "btn")
	if btn == nil {
		t.Fatal("btn missing")
	}
	if btn.Opts["label"] != "Go now" {
		t.Errorf("gen label should win: %v", btn.Opts["label"])
	}
	if btn.Opts["enabled"] != true {
		t.Errorf("base-only opt 'enabled' should be preserved: %v", btn.Opts["enabled"])
	}
	if btn.Opts["cell"] != "main" {
		t.Errorf("base-only opt 'cell' should be preserved: %v", btn.Opts["cell"])
	}
	if btn.Opts["class"] != "special" {
		t.Errorf("gen-only opt 'class' should be added: %v", btn.Opts["class"])
	}
	// cells merged: gen had no cells for dashboard name but different form.
	// Both are "f"; gen had no cells → base cells kept.
	main := findCell(firstForm(merged), "main")
	if main == nil {
		t.Error("base cell 'main' should be preserved when gen has none")
	}
}

func TestMergeDocumentHandlersMerged(t *testing.T) {
	base, _ := Import(handlerLua, "base.lua")
	gen, _ := Import(sampleLua, "gen.lua")
	merged := MergeDocument(base, gen)

	// handlerLua's form is "main", sampleLua's is "dashboard" → two forms; the
	// gen form (dashboard) is appended with its own handlers.
	if len(merged.Forms) != 2 {
		t.Fatalf("forms=%d want 2", len(merged.Forms))
	}
	dash := findForm(merged, "dashboard")
	main_ := findForm(merged, "main")
	if dash == nil || main_ == nil {
		t.Fatal("both forms must survive the merge")
	}
	if dash.Handlers == nil || main_.Handlers == nil {
		t.Fatal("handlers map missing")
	}
	// base form "main" keeps its own handlers: save → key_pressed
	kpEvents := main_.Handlers["save"]
	kpFound := false
	for _, ev := range kpEvents {
		if ev == "key_pressed" { kpFound = true }
	}
	if !kpFound {
		t.Errorf("save.key_pressed from base handler bodies should be present: %v", kpEvents)
	}
	// gen form "dashboard" keeps btn_refresh → click
	events := dash.Handlers["btn_refresh"]
	found := false
	for _, ev := range events {
		if ev == "click" { found = true }
	}
	if !found {
		t.Errorf("btn_refresh.click missing after merge: %v", events)
	}
}

func TestMergeDocumentSameNameOverlaysHandlers(t *testing.T) {
	// When base and gen name a form identically, the handler maps overlay.
	srcA := `function main()
  k.form.new("f", {})
  k.ctrl.label("f", "a", {text = "a"})
  k.form.on("f", "a", "selection_change", function()
    k.print("A changed")
  end)
  k.form.show("f")
end`
	srcB := `function main()
  k.form.new("f", {})
  k.ctrl.button("f", "b", {label = "B"})
  k.form.show("f")
end`
	base, _ := Import(srcA, "a.lua")
	gen, _ := Import(srcB, "b.lua")
	merged := MergeDocument(base, gen)
	f := findForm(merged, "f")
	if f == nil {
		t.Fatal("form f missing")
	}
	if findCtrl(f, "a") == nil {
		t.Error("base control a should be preserved")
	}
	if findCtrl(f, "b") == nil {
		t.Error("gen control b should be appended")
	}
	if contains(f.Handlers["a"], "selection_change") == false {
		t.Errorf("base handler a.selection_change lost on merge: %v", f.Handlers)
	}
}

func TestMergeDocumentCellsAdoptedFromGen(t *testing.T) {
	srcA := `function main()
  k.form.new("f", {layout = "grid", cells = {
    {id = "header", width = 12, bg = "#fff"},
    {id = "sidebar", width = 3}
  }})
  k.ctrl.label("f", "l", {cell = "header"})
  k.form.show("f")
end`
	srcB := `function main()
  k.form.new("f", {layout = "grid", cells = {
    {id = "header", width = 6},
    {id = "footer", width = 12}
  }})
  k.ctrl.label("f", "l2", {cell = "footer"})
  k.form.show("f")
end`
	base, _ := Import(srcA, "a.lua")
	gen, _ := Import(srcB, "b.lua")
	merged := MergeDocument(base, gen)

	hdr := findCell(firstForm(merged), "header")
	if hdr == nil { t.Fatal("header cell missing") }
	if hdr.Width != 6 { t.Errorf("header.width=%d want 6 (gen wins)", hdr.Width) }
	if findCell(firstForm(merged), "sidebar") == nil {
		t.Error("sidebar (base-only cell) should be preserved")
	}
	if findCell(firstForm(merged), "footer") == nil {
		t.Error("footer (gen-only cell) should be appended")
	}
}

func TestMergeNoteAdded(t *testing.T) {
	srcA := `function main()
  k.form.new("f", {})
  k.ctrl.label("f", "a", {text = "a"})
  k.form.show("f")
end`
	srcB := `function main()
  k.form.new("f", {})
  k.ctrl.label("f", "b", {text = "b"})
  k.form.new("g", {})
  k.form.show("g")
end`
	base, _ := Import(srcA, "a.lua")
	gen, _ := Import(srcB, "b.lua")
	merged := MergeDocument(base, gen)
	if len(firstForm(merged).Notes) == 0 {
		t.Error("expected merge note in Notes slice")
	}
	if len(merged.Notes) == 0 {
		t.Error("expected added-form note on the document")
	}
}


func TestMergeDocumentNilSafety(t *testing.T) {
	d, _ := Import(sampleLua, "d.lua")
	if MergeDocument(nil, d) != d { t.Error("nil base") }
	if MergeDocument(d, nil) != d { t.Error("nil gen") }
}

func TestImportMergeEndpoint(t *testing.T) {
	srv, path := startTestServer(t, "app.lua", "")
	baseURL := "http://" + srv.Addr()

	// Write an initial form to the file so there's something to merge against.
	initialLua := `function main()
  k.form.new("dash", {title = "Dash", layout = "grid"})
  k.ctrl.label("dash", "lbl", {text = "hi", cell = "header"})
  k.form.show("dash")
end`
	os.WriteFile(path, []byte(initialLua), 0o644)

	// Reload to get the doc.
	_, body := httpJSON(t, "GET", baseURL+"/api/form", nil)
	var loaded struct { Doc Document `json:"doc"` }
	json.Unmarshal(body, &loaded)

	// Import a new control via merge.
	genLua := `function main()
  k.form.new("dash", {title = "Dash"})
  k.ctrl.button("dash", "newbtn", {label = "New"})
  k.form.show("dash")
end`
	code, respBody := httpJSON(t, "POST", baseURL+"/api/import", map[string]any{
		"lua":  genLua,
		"mode": "merge",
		"base": loaded.Doc,
	})
	if code != 200 {
		t.Fatalf("merge import status=%d body=%s", code, respBody)
	}
	var merged struct {
		Ok     bool     `json:"ok"`
		Merged bool     `json:"merged"`
		Doc    Document `json:"doc"`
	}
	json.Unmarshal(respBody, &merged)
	if !merged.Ok || !merged.Merged {
		t.Errorf("expected ok+merged: %+v", merged)
	}
	if findCtrl(firstForm(&merged.Doc), "lbl") == nil {
		t.Error("base control lbl missing after merge")
	}
	if findCtrl(firstForm(&merged.Doc), "newbtn") == nil {
		t.Error("gen control newbtn missing after merge")
	}
	if firstForm(&merged.Doc).Name != "dash" {
		t.Errorf("name=%q want dash", firstForm(&merged.Doc).Name)
	}

	// Merge without base should error.
	code, body = httpJSON(t, "POST", baseURL+"/api/import", map[string]any{
		"lua":  genLua,
		"mode": "merge",
	})
	if code != http.StatusBadRequest {
		t.Errorf("merge without base: status=%d want %d, body=%s", code, http.StatusBadRequest, body)
	}
}

func TestImportReplaceEndpoint(t *testing.T) {
	srv, _ := startTestServer(t, "app.lua", "")
	baseURL := "http://" + srv.Addr()
	genLua := `function main()
  k.form.new("newform", {})
  k.ctrl.label("newform", "l", {text = "x"})
  k.form.show("newform")
end`
	code, body := httpJSON(t, "POST", baseURL+"/api/import", map[string]any{
		"lua":  genLua,
		"mode": "replace",
	})
	if code != 200 {
		t.Fatalf("replace import status=%d body=%s", code, body)
	}
	var r struct { Doc Document `json:"doc"` }
	json.Unmarshal(body, &r)
	if firstForm(&r.Doc).Name != "newform" {
		t.Errorf("expected new form, got %+v", firstForm(&r.Doc))
	}
}

/* ---------- multi-form + source-preserving splice ---------- */

const multiLua = `-- multi form fixture
local cfg = {cols = 2}

function main()
  k.form.new("login", {title = "Login", layout = "vertical"})
  k.ctrl.textbox("login", "user", {label = "User"})
  k.form.on("login", "user", "selection_change", function()
    k.print("login.user.change")
  end)
  k.form.show("login")

  local x = 1
  k.form.new("admin", {title = "Admin"})
  k.ctrl.list("admin", "list1", {items = {["1"] = "A", ["2"] = "B"}})
  k.ctrl.button("admin", "go", {label = "Go"})
  k.form.on("admin", "go", "click", function()
    k.msgbox("go")
  end)
  k.form.show("admin")
end
`

func TestImportMultiForm(t *testing.T) {
	d, err := Import(multiLua, "multi.lua")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(d.Forms) != 2 {
		t.Fatalf("forms=%d want 2", len(d.Forms))
	}
	if d.Forms[0].Name != "login" || d.Forms[1].Name != "admin" {
		t.Errorf("form order wrong: %v", formNames(d))
	}
	if len(d.Forms[1].Controls) != 2 {
		t.Errorf("admin controls=%d want 2", len(d.Forms[1].Controls))
	}
	if len(d.Forms[0].Lines) != 4 {
		t.Errorf("login owned spans=%d want 4 (new, ctrl, on, show): %+v", len(d.Forms[0].Lines), d.Forms[0].Lines)
	}
	if len(d.Forms[1].Lines) != 5 {
		t.Errorf("admin owned spans=%d want 5 (new, list, button, on, show): %+v", len(d.Forms[1].Lines), d.Forms[1].Lines)
	}
	if d.Forms[0].Indent != "  " {
		t.Errorf("indent=%q want two spaces", d.Forms[0].Indent)
	}
	// handler statements captured verbatim (exact source text)
	if !strings.Contains(d.Forms[0].HandlerBodies["user.selection_change"], `k.print("login.user.change")`) {
		t.Error("form.on body not captured verbatim")
	}
	// non-form code did not leak into a form's structure
	for _, f := range d.Forms {
		if f.Title == "cfg" || findCtrl(f, "cfg") != nil {
			t.Error("non-form code leaked into a form")
		}
	}
}

func TestRebuildLuaIdempotent(t *testing.T) {
	// Opening a file and saving without edits must rewrite nothing.
	d, _ := Import(multiLua, "multi.lua")
	out, err := RebuildLua(multiLua, d)
	if err != nil {
		t.Fatalf("RebuildLua: %v", err)
	}
	if out != multiLua {
		a := strings.Split(multiLua, "\n")
		b := strings.Split(out, "\n")
		for i := 0; i < len(a) || i < len(b); i++ {
			av, bv := "⟂", "⟂"
			if i < len(a) { av = a[i] }
			if i < len(b) { bv = b[i] }
			if av != bv { t.Logf("line %d:|%s| vs |%s|", i+1, av, bv) }
		}
		t.Errorf("byte-identical expected when nothing changed")
	}
}

func TestRebuildLuaPreservesNonForm(t *testing.T) {
	d, err := Import(multiLua, "multi.lua")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	b := findForm(d, "admin")
	goBtn := findCtrl(b, "go")
	goBtn.Opts["label"] = "Go now"
	out, err := RebuildLua(multiLua, d)
	if err != nil {
		t.Fatalf("RebuildLua: %v", err)
	}
	if strings.Contains(out, "label = \"Go\"") {
		t.Error("old admin label still present")
	}
	if !strings.Contains(out, "label = \"Go now\"") {
		t.Error("edited label missing from output")
	}
	// the k.form.on line (reference @admin,go,click) survives verbatim
	if !strings.Contains(out, `k.form.on("admin", "go", "click", function()`) {
		t.Error("admin handler not preserved:")
		t.Error(out)
	}
	if !strings.Contains(out, `k.msgbox("go")`) {
		t.Error("admin handler body dropped")
	}
	// login form untouched, single copy; non-form code in order
	if strings.Count(out, `k.form.new("login"`) != 1 {
		t.Error("login form must stay as a single untouched block")
	}
	if !strings.Contains(out, `k.form.new("admin", {title="Admin", layout="vertical"`) {
		t.Error("regenerated admin block should carry its edited form:")
		t.Error(out)
	}
	retainOrdered(t, multiLua, out, b.Lines)
	// result is still valid and imports back to two forms
	d2, err := Import(out, "out.lua")
	if err != nil {
		t.Fatalf("re-import of spliced output: %v", err)
	}
	if len(d2.Forms) != 2 || findForm(d2, "admin") == nil {
		t.Errorf("spliced output lost a form: %v", formNames(d2))
	}
}

func TestRebuildLuaNewAndDeleteForms(t *testing.T) {
	d, _ := Import(multiLua, "multi.lua")
	// add form "report"
	c := &Form{Name: "report", Layout: "vertical", Align: "left",
		Handlers: map[string][]string{}, HandlerBodies: map[string]string{},
		Controls: []*Control{{Name: "go", Type: "button", Opts: map[string]any{"label": "Run"}}}}
	d.Forms = append(d.Forms, c)
	out, err := RebuildLua(multiLua, d)
	if err != nil {
		t.Fatalf("RebuildLua add: %v", err)
	}
	if !strings.Contains(out, `k.form.new("report"`) {
		t.Error("new form missing from output:")
		t.Error(out)
	}
	// form order kept: login, admin, report
	if !strings.Contains(out, `k.form.show("admin")`) {
		t.Error("admin block lost")
	}
	if !(strings.Index(out, `k.form.new("admin"`) < strings.Index(out, `k.form.new("report"`)) {
		t.Error("new form should be appended after existing forms")
	}
	retainOrdered(t, multiLua, out, nil)

	// delete form "admin"
	d2, _ := Import(multiLua, "multi.lua")
	var kept []*Form
	for _, f := range d2.Forms {
		if f.Name != "admin" {
			kept = append(kept, f)
		}
	}
	d2.Forms = kept
	del, derr := RebuildLua(multiLua, d2)
	if derr != nil {
		t.Fatalf("RebuildLua delete: %v", derr)
	}
	if strings.Contains(del, `k.form.new("admin"`) || strings.Contains(del, "k.msgbox(\"go\")") {
		t.Error("deleted form block (with its k.form.on) still present")
	}
	if !strings.Contains(del, `k.form.new("login"`) {
		t.Error("login form lost after deleting admin")
	}
	d3, err := Import(del, "del.lua")
	if err != nil {
		t.Fatalf("re-import after delete: %v", err)
	}
	if len(d3.Forms) != 1 || d3.Forms[0].Name != "login" {
		t.Errorf("expected only login, got %v", formNames(d3))
	}
}

func TestRebuildLuaRenameKeepsHandlers(t *testing.T) {
	// Renaming a control must NOT rewrite its k.form.on line — the stale
	// handler statement (referencing the old name) is preserved verbatim.
	src := `function main()
  k.form.new("f", {})
  k.ctrl.button("f", "old", {label = "Old"})
  k.form.on("f", "old", "click", function()
    k.print("handling")
  end)
  k.form.show("f")
end
`
	d, err := Import(src, "r.lua")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	f := findForm(d, "f")
	f.Controls[0].Name = "newname"
	f.Controls[0].Opts["label"] = "New"
	out, err := RebuildLua(src, d)
	if err != nil {
		t.Fatalf("RebuildLua: %v", err)
	}
	if !strings.Contains(out, `k.ctrl.button("f", "newname"`) {
		t.Error("renamed control missing from output")
	}
	if !strings.Contains(out, `k.form.on("f", "old", "click", function()`) || !strings.Contains(out, `k.print("handling")`) {
		t.Error("orphaned handler must be preserved verbatim after control rename:")
		t.Error(out)
	}
	if _, rerr := Import(out, "rout.lua"); rerr != nil {
		t.Errorf("output after rename does not parse: %v", rerr)
	}
}

func TestRebuildLuaRenameForm(t *testing.T) {
	d, _ := Import(multiLua, "multi.lua")
	findForm(d, "admin").Name = "dashboard"
	out, err := RebuildLua(multiLua, d)
	if err != nil {
		t.Fatalf("RebuildLua: %v", err)
	}
	if strings.Contains(out, `k.form.new("admin"`) {
		t.Error("old form name still present")
	}
	if !strings.Contains(out, `k.form.new("dashboard"`) {
		t.Error("renamed form missing")
	}
	if !strings.Contains(out, `k.form.on("admin", "go", "click", function()`) || !strings.Contains(out, `k.msgbox("go")`) {
		t.Error("renamed form must keep its stale handler verbatim (never rewritten):")
		t.Error(out)
	}
	if _, rerr := Import(out, "dash.lua"); rerr != nil {
		t.Errorf("output after form rename does not parse: %v", rerr)
	}
}

func TestRebuildLuaNoFormScript(t *testing.T) {
	// A pure non-form script gets function main() appended with the doc forms.
	src := `require("utils")
print("booted")
`
	out, err := RebuildLua(src, NewEmptyDoc())
	if err != nil {
		t.Fatalf("RebuildLua: %v", err)
	}
	if !strings.Contains(out, `require("utils")`) || !strings.Contains(out, `print("booted")`) {
		t.Error("non-form script not preserved:")
		t.Error(out)
	}
	if !strings.Contains(out, "function main()") || !strings.Contains(out, `k.form.new("main"`) {
		t.Error("main() with the document's form not appended:")
		t.Error(out)
	}
	// appended block must keep the two-space body indent (no double indentation)
	if strings.Contains(out, "    k.form.new(\"main\"") {
		t.Error("appended main() body double-indented:")
		t.Error(out)
	}
	if _, rerr := Import(out, "nf.lua"); rerr != nil {
		t.Errorf("appended script does not parse: %v", rerr)
	}
}

func TestServerMultiFormEndToEnd(t *testing.T) {
	srv, path := startTestServer(t, "multi.lua", multiLua)
	base := "http://" + srv.Addr()

	_, body := httpJSON(t, "GET", base+"/api/form", nil)
	var got struct {
		Doc Document `json:"doc"`
	}
	json.Unmarshal(body, &got)
	if len(got.Doc.Forms) != 2 {
		t.Fatalf("forms=%d want 2", len(got.Doc.Forms))
	}

	// Edit admin's button label through the document and save.
	findCtrl(findForm(&got.Doc, "admin"), "go").Opts["label"] = "Go now"
	code, body := httpJSON(t, "PUT", base+"/api/form", map[string]any{"doc": got.Doc})
	if code != 200 {
		t.Fatalf("PUT status=%d body=%s", code, body)
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(saved)
	if !strings.Contains(text, "-- multi form fixture") || !strings.Contains(text, "local cfg = {cols = 2}") {
		t.Error("non-form code lost on save")
	}
	if !strings.Contains(text, `k.form.new("login"`) {
		t.Error("login form lost on save")
	}
	if strings.Count(text, `k.form.on("login", "user"`) != 1 {
		t.Error("login handler must survive byte-for-byte")
	}
	if !strings.Contains(text, `label = "Go now"`) {
		t.Error("edited label not saved")
	}
	if !strings.Contains(text, `k.msgbox("go")`) {
		t.Error("admin handler not preserved")
	}

	// The assembled /api/export reflects the edit without saving.
	code, body = httpJSON(t, "POST", base+"/api/export", map[string]any{"doc": got.Doc})
	if code != 200 {
		t.Fatalf("export status=%d body=%s", code, body)
	}
	var out struct { Lua string `json:"lua"` }
	json.Unmarshal(body, &out)
	if strings.Count(out.Lua, `k.form.new("login"`) != 1 {
		t.Error("assembled export must not duplicate login")
	}
}

// retainOrdered asserts that the source lines OUTSIDE spans (1-based) appear
// in out, in the same relative order. spans == nil → all source lines.
func retainOrdered(t *testing.T, src, out string, spans [][]int) {
	expect := make([]string, 0, 16)
	for i, ln := range strings.Split(src, "\n") {
		if spans != nil && len(spans) > 0 && inRanges(i+1, spans) {
			continue
		}
		expect = append(expect, ln)
	}
	next := 0
	for _, oln := range strings.Split(out, "\n") {
		if next < len(expect) && expect[next] == oln {
			next++
		}
	}
	if next != len(expect) {
		t.Errorf("preserved lines not in order (consumed %d/%d):\nOUT:\n%s\n\nEXPECT:\n%s",
			next, len(expect), out, strings.Join(expect, "\n"))
	}
}

func inRanges(ln int, spans [][]int) bool {
	for _, s := range spans {
		if ln >= s[0] && ln <= s[1] {
			return true
		}
	}
	return false
}

func TestJSONv2MigrationToForms(t *testing.T) {
	// A v2 single-form document ({version:2, form}) loads as v3 with forms[].
	legacy := map[string]any{
		"version": float64(2),
		"form": map[string]any{
			"name":  "old",
			"title": "Legacy",
			"cells": map[string]any{
				"main": map[string]any{"width": float64(9)},
				"side": map[string]any{"width": float64(3)},
			},
		},
	}
	b, _ := json.MarshalIndent(legacy, "", "  ")
	srv, _ := startTestServer(t, "legacy.json", string(b))
	code, body := httpJSON(t, "GET", "http://"+srv.Addr()+"/api/form", nil)
	if code != 200 {
		t.Fatalf("GET status=%d body=%s", code, body)
	}
	var got struct {
		Doc Document `json:"doc"`
	}
	json.Unmarshal(body, &got)
	if got.Doc.Version != DocVersion {
		t.Errorf("version=%d want %d", got.Doc.Version, DocVersion)
	}
	if len(got.Doc.Forms) != 1 || got.Doc.Forms[0].Name != "old" {
		t.Errorf("expected one migrated form 'old': %+v", got.Doc.Forms)
	}
	if len(got.Doc.Forms[0].Cells) != 2 {
		t.Errorf("cells not migrated: %+v", got.Doc.Forms[0].Cells)
	}
}
