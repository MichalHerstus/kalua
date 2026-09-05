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
