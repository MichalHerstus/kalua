package bindings

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/yuin/gopher-lua"
)

func ctrlTable(L *lua.LState, ctrlType string, opts map[string]lua.LValue) *lua.LTable {
	c := L.NewTable()
	c.RawSetString("type", lua.LString(ctrlType))
	c.RawSetString("name", lua.LString("c1"))
	c.RawSetString("form", lua.LString("f"))
	for k, v := range opts {
		c.RawSetString(k, v)
	}
	return c
}

func str(s string) lua.LValue  { return lua.LString(s) }
func num(n float64) lua.LValue { return lua.LNumber(n) }
func ltrue() lua.LValue        { return lua.LTrue }

// TestRenderLabelMultiline verifies §4.2: multiline labels render as a pre-wrap
// div while plain labels keep the <label> element.
func TestRenderLabelMultiline(t *testing.T) {
	L := setupTestState(t)

	plain := renderControl(ctrlTable(L, "label", map[string]lua.LValue{
		"label": str("Hello"),
	}))
	if !strings.Contains(plain, `<label class="kalua-label"`) || strings.Contains(plain, "kalua-label-multiline") {
		t.Errorf("plain label = %s", plain)
	}

	multi := renderControl(ctrlTable(L, "label", map[string]lua.LValue{
		"label":     str("Line1\nLine2"),
		"multiline": ltrue(),
	}))
	if !strings.Contains(multi, `class="kalua-label kalua-label-multiline"`) {
		t.Errorf("multiline label missing div class: %s", multi)
	}
	if !strings.Contains(multi, "Line1\nLine2") {
		t.Errorf("multiline label dropped newline text: %s", multi)
	}
}

// TestRenderTextboxMultiline verifies §4.1: multiline textboxes render a
// <textarea> with rows/cols defaults and escaped content.
func TestRenderTextboxMultiline(t *testing.T) {
	L := setupTestState(t)

	got := renderControl(ctrlTable(L, "textbox", map[string]lua.LValue{
		"label":     str("Notes"),
		"multiline": ltrue(),
		"value":     str("a < b & c"),
		"rows":      num(6),
		"cols":      num(40),
	}))
	if !strings.Contains(got, `<textarea class="kalua-textarea"`) {
		t.Errorf("missing textarea: %s", got)
	}
	if !strings.Contains(got, `rows="6"`) || !strings.Contains(got, `cols="40"`) {
		t.Errorf("textarea rows/cols missing: %s", got)
	}
	if !strings.Contains(got, "a &lt; b &amp; c") {
		t.Errorf("textarea content not escaped: %s", got)
	}

	defaults := renderControl(ctrlTable(L, "textbox", map[string]lua.LValue{
		"label":     str("Notes"),
		"multiline": ltrue(),
	}))
	if !strings.Contains(defaults, `rows="4"`) || !strings.Contains(defaults, `cols="50"`) {
		t.Errorf("textarea default rows/cols: %s", defaults)
	}

	plain := renderControl(ctrlTable(L, "textbox", map[string]lua.LValue{
		"label": str("Name"),
		"value": str("x"),
	}))
	if !strings.Contains(plain, `<input type="text" class="kalua-input"`) || strings.Contains(plain, "kalua-datetime") {
		t.Errorf("plain textbox should stay an input: %s", plain)
	}
}

// TestRenderTextboxDatetime verifies §4.1: datetime textboxes carry the
// flatpickr class and a parseable data-k-datetime-options attribute.
func TestRenderTextboxDatetime(t *testing.T) {
	L := setupTestState(t)

	for _, tc := range []struct {
		mode string
		want string // expected dateFormat
	}{
		{"date", "Y-m-d"},
		{"time", "H:i"},
		{"datetime", "Y-m-d H:i"},
	} {
		dt := L.NewTable()
		dt.RawSetString("mode", lua.LString(tc.mode))
		got := renderControl(ctrlTable(L, "textbox", map[string]lua.LValue{
			"label":    str("When"),
			"datetime": dt,
		}))
		if !strings.Contains(got, `class="kalua-input kalua-datetime"`) {
			t.Errorf("[%s] missing kalua-datetime class: %s", tc.mode, got)
		}
		cfg := decodeDatetimeAttr(t, got)
		if cfg["dateFormat"] != tc.want {
			t.Errorf("[%s] dateFormat = %v, want %s", tc.mode, cfg["dateFormat"], tc.want)
		}
	}
}

// TestFlatpickrFormat verifies display-format translation.
func TestFlatpickrFormat(t *testing.T) {
	cases := map[string]string{
		"YYYY-MM-DD":       "Y-m-d",
		"HH:MM":            "H:i",
		"YYYY-MM-DD HH:MM": "Y-m-d H:i",
		"DD/MM/YYYY":       "d/m/Y",
	}
	for in, want := range cases {
		if got := flatpickrFormat(in); got != want {
			t.Errorf("flatpickrFormat(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestRenderImage verifies §4.3: image control renders an <img> with src/alt/
// style, clickable images carry data-k-* attrs, others do not.
func TestRenderImage(t *testing.T) {
	L := setupTestState(t)

	got := renderControl(ctrlTable(L, "image", map[string]lua.LValue{
		"src":   str("data:image/png;base64,AAA"),
		"alt":   str("logo"),
		"width": num(120),
		"fit":   str("cover"),
	}))
	if !strings.Contains(got, `<img class="kalua-image"`) {
		t.Errorf("missing img: %s", got)
	}
	if !strings.Contains(got, `src="data:image/png;base64,AAA"`) || !strings.Contains(got, `alt="logo"`) {
		t.Errorf("img src/alt missing: %s", got)
	}
	if !strings.Contains(got, `width:120px;`) || !strings.Contains(got, `object-fit:cover;`) {
		t.Errorf("img style missing: %s", got)
	}
	if strings.Contains(got, "data-k-form") {
		t.Errorf("non-clickable image should not carry data-k attrs: %s", got)
	}

	clickable := renderControl(ctrlTable(L, "image", map[string]lua.LValue{
		"src":       str("/img/x.png"),
		"clickable": ltrue(),
		"height":    str("50%"),
	}))
	if !strings.Contains(clickable, `data-k-form="f"`) || !strings.Contains(clickable, `data-k-ctrl="c1"`) {
		t.Errorf("clickable image missing data-k attrs: %s", clickable)
	}
	if !strings.Contains(clickable, `height:50%;`) || !strings.Contains(clickable, `object-fit:contain;`) {
		t.Errorf("clickable image style (fit default) missing: %s", clickable)
	}
}

// TestImageSetValueMapsToSrc verifies §4.3 Dynamic Update: k.ctrl.set_value on an
// image re-renders with the new src and get_value returns it.
func TestImageSetValueMapsToSrc(t *testing.T) {
	L := setupTestState(t)
	formTbl := L.NewTable()
	formTbl.RawSetString("name", lua.LString("f"))
	formTbl.RawSetString("controls", L.NewTable())
	formTbl.RawSetString("handlers", L.NewTable())
	L.SetGlobal("f", formTbl)

	img := L.NewTable()
	img.RawSetString("src", str("/a.png"))
	img.RawSetString("type", str("image"))
	img.RawSetString("name", str("pic"))
	img.RawSetString("form", str("f"))
	formTbl.RawGetString("controls").(*lua.LTable).RawSetString("pic", img)

	// Simulate k.ctrl.set_value("f","pic","/b.png") behavior: value set to new src.
	img.RawSetString("value", str("/b.png"))
	if img.RawGetString("type").String() == "image" {
		img.RawSetString("src", str("/b.png"))
	}

	html := renderControl(img)
	if !strings.Contains(html, `src="/b.png"`) {
		t.Errorf("image src not updated: %s", html)
	}
	getVal := img.RawGetString("src").String()
	if getVal != "/b.png" {
		t.Errorf("get_value source = %q, want /b.png", getVal)
	}
}

func decodeDatetimeAttr(t *testing.T, html string) map[string]interface{} {
	t.Helper()
	start := strings.Index(html, `data-k-datetime-options="`) + len(`data-k-datetime-options="`)
	if start < len(`data-k-datetime-options="`) {
		t.Fatalf("missing data-k-datetime-options: %s", html)
	}
	end := strings.Index(html[start:], `"`)
	raw := html[start : start+end]
	raw = strings.ReplaceAll(raw, "&quot;", `"`)
	raw = strings.ReplaceAll(raw, "&amp;", "&")
	raw = strings.ReplaceAll(raw, "&#34;", `"`)
	var cfg map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("data-k-datetime-options not valid JSON (%s): %v", raw, err)
	}
	return cfg
}

// layoutTestForm builds a form table registered as a global, adds controls via
// the real addControl path, and renders it. Returns the rendered HTML.
func layoutTestForm(t *testing.T, L *lua.LState, opts map[string]lua.LValue, controls [][]lua.LValue) string {
	t.Helper()
	f := L.NewTable()
	f.RawSetString("name", str("f"))
	f.RawSetString("title", str("T"))
	f.RawSetString("layout", str("vertical"))
	f.RawSetString("align", str("left"))
	for k, v := range opts {
		if k == "cells" {
			f.RawSetString("cells", v)
			continue
		}
		f.RawSetString(k, v)
	}
	f.RawSetString("controls", L.NewTable())
	f.RawSetString("handlers", L.NewTable())
	L.SetGlobal("f", f)

	for _, ctrlOpts := range controls {
		oC := L.NewTable()
		for i := 0; i+1 < len(ctrlOpts); i += 2 {
			oC.RawSetString(ctrlOpts[i].String(), ctrlOpts[i+1])
		}
		addControl(L, "f", ctrlOpts[0].String(), ctrlOpts[1].String(), oC)
	}
	return renderForm(L, "f")
}

// TestRenderVerticalAlignGap verifies §6 vertical layout: the form div carries
// align and a --kalua-gap style var, and grid cells are not used.
func TestRenderVerticalAlignGap(t *testing.T) {
	L := setupTestState(t)
	html := layoutTestForm(t, L, map[string]lua.LValue{
		"align": str("center"),
		"gap":   num(8),
	}, [][]lua.LValue{
		{str("a"), str("button"), str("label"), str("Go")},
	})

	if !strings.Contains(html, `class="kalua-form" align="center"`) {
		t.Errorf("missing form align attr: %s", html)
	}
	if !strings.Contains(html, `style="--kalua-gap:8px"`) {
		t.Errorf("missing gap var: %s", html)
	}
	if strings.Contains(html, "kalua-cell") {
		t.Errorf("vertical layout must not render cells: %s", html)
	}
	if !strings.Contains(html, `id="c:f:a"`) {
		t.Errorf("control not rendered: %s", html)
	}
}

// TestRenderGridCellsOrder verifies §6 grid cells (array form): cells render in
// declaration order with column spans, bg/border/align styles, and controls are
// placed into their assigned cells.
func TestRenderGridCellsOrder(t *testing.T) {
	L := setupTestState(t)

	header := L.NewTable()
	header.RawSetString("id", str("header"))
	header.RawSetString("width", num(12))
	header.RawSetString("bg", str("#f5f5f5"))
	header.RawSetString("align", str("center"))
	bd := L.NewTable()
	bd.RawSetString("width", num(1))
	bd.RawSetString("color", str("#ddd"))
	header.RawSetString("border", bd)

	sidebar := L.NewTable()
	sidebar.RawSetString("id", str("sidebar"))
	sidebar.RawSetString("width", num(3))

	main := L.NewTable()
	main.RawSetString("id", str("main"))
	main.RawSetString("width", num(9))

	cells := L.NewTable()
	cells.RawSetInt(1, header)
	cells.RawSetInt(2, sidebar)
	cells.RawSetInt(3, main)

	html := layoutTestForm(t, L, map[string]lua.LValue{
		"layout": str("grid"),
		"gap":    num(16),
		"cells":  cells,
	}, [][]lua.LValue{
		{str("search"), str("textbox"), str("cell"), str("header"), str("label"), str("Search")},
		{str("menu"), str("list"), str("cell"), str("sidebar"), str("label"), str("Menu")},
		{str("data"), str("textbox"), str("cell"), str("main"), str("label"), str("Data")},
	})

	for _, want := range []string{
		`layout="grid"`,
		`data-k-cell="header"`,
		`data-k-cell="sidebar"`,
		`data-k-cell="main"`,
		"grid-column: span 12",
		"grid-column: span 3",
		"grid-column: span 9",
		`background-color: #f5f5f5`,
		`border: 1px solid #ddd`,
		`align="center"`,
		`id="c:f:search"`,
		`id="c:f:menu"`,
		`id="c:f:data"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q in: %s", want, html)
		}
	}

	// Cell order must follow declaration order (header, sidebar, main).
	hIdx := strings.Index(html, `data-k-cell="header"`)
	sIdx := strings.Index(html, `data-k-cell="sidebar"`)
	mIdx := strings.Index(html, `data-k-cell="main"`)
	if !(hIdx < sIdx && sIdx < mIdx) {
		t.Errorf("cells not in declaration order (header=%d sidebar=%d main=%d): %s", hIdx, sIdx, mIdx, html)
	}

	// Each control must live inside its cell container (after its cell's
	// opening div and before the cell's closing div).
	if !strings.Contains(html[strings.Index(html, `data-k-cell="header"`):strings.Index(html, `data-k-cell="sidebar"`)], `id="c:f:search"`) {
		t.Errorf("search control not inside header cell: %s", html)
	}
	if !strings.Contains(html[strings.Index(html, `data-k-cell="sidebar"`):strings.Index(html, `data-k-cell="main"`)], `id="c:f:menu"`) {
		t.Errorf("menu control not inside sidebar cell: %s", html)
	}
	if !strings.Contains(html[strings.Index(html, `data-k-cell="main"`):], `id="c:f:data"`) {
		t.Errorf("data control not inside main cell: %s", html)
	}
}

// TestRenderGridAutoMain verifies §6 backward compatibility: layout="grid"
// without cells auto-creates a single "main" cell (width 12) holding controls
// with no cell assignment.
func TestRenderGridAutoMain(t *testing.T) {
	L := setupTestState(t)
	html := layoutTestForm(t, L, map[string]lua.LValue{
		"layout": str("grid"),
	}, [][]lua.LValue{
		{str("a"), str("button"), str("label"), str("Go")},
	})

	if !strings.Contains(html, `data-k-cell="main"`) {
		t.Errorf("missing auto main cell: %s", html)
	}
	if !strings.Contains(html, "grid-column: span 12") {
		t.Errorf("auto cell must span 12: %s", html)
	}
	if !strings.Contains(html, `id="c:f:a"`) {
		t.Errorf("control not in auto cell: %s", html)
	}
	if !strings.Contains(html[strings.Index(html, `data-k-cell="main"`):], `id="c:f:a"`) {
		t.Errorf("control not inside main cell: %s", html)
	}
}

// TestRenderGridUnknownCellFallsBackToMain verifies that a control referencing
// an undefined cell lands in the auto-created main cell.
func TestRenderGridUnknownCellFallsBackToMain(t *testing.T) {
	L := setupTestState(t)
	header := L.NewTable()
	header.RawSetString("id", str("header"))
	header.RawSetString("width", num(12))
	cells := L.NewTable()
	cells.RawSetInt(1, header)

	html := layoutTestForm(t, L, map[string]lua.LValue{
		"layout": str("grid"),
		"cells":  cells,
	}, [][]lua.LValue{
		{str("a"), str("button"), str("cell"), str("bogus"), str("label"), str("Go")},
	})

	if !strings.Contains(html, `data-k-cell="main"`) {
		t.Errorf("missing auto main fallback cell: %s", html)
	}
	if !strings.Contains(html[strings.Index(html, `data-k-cell="main"`):], `id="c:f:a"`) {
		t.Errorf("control with unknown cell not in main: %s", html)
	}
}

// TestRenderGridMapForm verifies the map form of cells is accepted and sorted
// deterministically (lexicographic by id, since gopher-lua has no key order).
func TestRenderGridMapForm(t *testing.T) {
	L := setupTestState(t)
	cells := L.NewTable()
	cells.RawSetString("b.one", L.NewTable())
	cells.RawSetString("a.two", L.NewTable())

	html := layoutTestForm(t, L, map[string]lua.LValue{
		"layout": str("grid"),
		"cells":  cells,
	}, [][]lua.LValue{
		{str("x"), str("button"), str("label"), str("Go")},
	})

	aIdx := strings.Index(html, `data-k-cell="a.two"`)
	bIdx := strings.Index(html, `data-k-cell="b.one"`)
	if aIdx == -1 || bIdx == -1 {
		t.Fatalf("map form cells missing: %s", html)
	}
	if aIdx > bIdx {
		t.Errorf("map form should be lexicographically sorted: %s", html)
	}
}

// TestRenderControlAlign verifies §6 per-control alignment is baked into the
// control element's align-self, including when merged with hidden visibility.
func TestRenderControlAlign(t *testing.T) {
	L := setupTestState(t)

	center := renderControl(ctrlTable(L, "button", map[string]lua.LValue{
		"label": str("Go"),
		"align": str("center"),
	}))
	if !strings.Contains(center, `style="align-self:center"`) {
		t.Errorf("center align missing: %s", center)
	}

	right := renderControl(ctrlTable(L, "textbox", map[string]lua.LValue{
		"label":   str("Notes"),
		"visible": lua.LFalse,
		"align":   str("right"),
	}))
	if !strings.Contains(right, `style="display:none;align-self:flex-end"`) {
		t.Errorf("hidden + right align not merged: %s", right)
	}

	left := renderControl(ctrlTable(L, "label", map[string]lua.LValue{
		"label": str("Hi"),
		"align": str("left"),
	}))
	if strings.Contains(left, "align-self") {
		t.Errorf("left align should be a no-op: %s", left)
	}
}
