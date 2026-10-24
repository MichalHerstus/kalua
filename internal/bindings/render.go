// Render-only control HTML generation, shared by the native and WASM
// builds (M5 Phase 2). Pure string building over the Lua control/
// form tables — no Env/session/DB use — so it compiles for both the
// native and the js/wasm targets. The JS hands (app.minimal.js) read
// the same data-k-* attributes and kalua-* classes this emits.
package bindings

import (
	"encoding/json"
	"html"
	"sort"
	"strconv"
	"strings"

	"github.com/yuin/gopher-lua"

	"kalua/internal/version"
)

// renderForm renders a form to HTML using templ-like logic (simplified for now).
func renderForm(L *lua.LState, formName string) string {
	formNameEsc := escAttr(formName)
	formTbl := L.GetGlobal(formName)
	if formTbl == lua.LNil {
		return `<div class="error">Form not found: ` + escText(formName) + `</div>`
	}
	tbl, ok := formTbl.(*lua.LTable)
	if !ok {
		return `<div class="error">Invalid form</div>`
	}

	title := ""
	if tv := tbl.RawGetString("title"); tv != lua.LNil {
		title = escText(tv.String())
	}
	layout := tbl.RawGetString("layout").String()
	if layout == "" {
		layout = "vertical"
	}
	align := tbl.RawGetString("align").String()
	gap := 16
	if v := tbl.RawGetString("gap"); v != lua.LNil {
		if n := int(lua.LVAsNumber(v)); n >= 0 {
			gap = n
		}
	}
	controls := tbl.RawGetString("controls")
	controlsTbl, _ := controls.(*lua.LTable)
	shell := scanShellLayout(controlsTbl)

	var html string
	html += `<div id="f:` + formNameEsc + `" class="kalua-form`
	// Layout (shell) controls (kforms_enhancements.md §11) are position:fixed,
	// so the form body must leave room for them: add a class per present shell
	// control and kalua.css pads the form accordingly. The sidebar is anchored
	// to its configured side (data-k-sidebar-position) so padding applies on
	// that side only.
	for _, shellType := range []string{"topbar", "sidebar", "footer"} {
		if formHasControlType(controls, shellType) {
			html += " kalua-has-" + shellType
		}
	}
	if shell.hasSidebar {
		if shell.sidePosition == "right" {
			html += " kalua-has-sidebar-right"
		} else {
			html += " kalua-has-sidebar-left"
		}
	}
	html += `"`
	if layout != "vertical" {
		html += ` layout="` + escAttr(layout) + `"`
	}
	if align != "" && align != "left" {
		html += ` align="` + escAttr(align) + `"`
	}
	style := "--kalua-gap:" + strconv.Itoa(gap) + "px"
	// Shell metrics as CSS variables. Only a fixed sidebar needs them: it
	// ranks between the topbar (56px) and the footer (40px) when those bars
	// are present, and spans the full viewport height otherwise. Forms without
	// a sidebar carry no shell vars (their inline style stays unchanged).
	if shell.hasSidebar {
		top := "56px"
		if !shell.hasTopbar {
			top = "0px"
		}
		bottom := "40px"
		if !shell.hasFooter {
			bottom = "0px"
		}
		style += ";--kalua-topbar-height:" + top
		style += ";--kalua-footer-height:" + bottom
		style += ";--kalua-sidebar-width:" + strconv.Itoa(shell.sideWidth) + "px"
		style += ";--kalua-sidebar-collapsed-width:" + strconv.Itoa(shell.sideCollapse) + "px"
	}
	if props := styleFromProps(tbl); props != "" {
		style += ";" + props
	}
	html += ` style="` + escAttr(style) + `"`
	html += `>`

	if title != "" {
		html += `<div class="kalua-form-title">` + title + `</div>`
	}

	if layout == "grid" {
		html += renderGridForm(tbl)
	} else if controlsTbl != nil {
		html += renderVerticalControls(controlsTbl, tbl)
	}

	// §11 layout (shell) controls render outside the normal control flow as
	// position:fixed elements. They stay inside the form element (so closing
	// the form removes them) but are skipped by renderVerticalControls/
	// renderGridForm, which would otherwise place/row them.
	if controlsTbl != nil {
		if orderTbl, ok := tbl.RawGetString("order").(*lua.LTable); ok {
			orderTbl.ForEach(func(_, v lua.LValue) {
				if ctrl, ok := controlsTbl.RawGetString(v.String()).(*lua.LTable); ok && shellControl(ctrl.RawGetString("type").String()) {
					html += renderControl(ctrl)
				}
			})
		}
	}

	html += `</div>`
	return html
}

// shellControl reports whether a control type is a §11 layout (shell) control
// rendered outside the normal form flow (position:fixed).
func shellControl(ctrlType string) bool {
	return ctrlType == "topbar" || ctrlType == "sidebar" || ctrlType == "footer"
}

// shellLayout summarizes a form's §11 shell controls for renderForm: whether a
// fixed topbar/footer is present (their heights offset the form) and the
// sidebar's position/widths (used for the form's side padding + CSS vars).
type shellLayout struct {
	hasTopbar    bool
	hasFooter    bool
	hasSidebar   bool
	sidePosition string // "left" | "right" (default "left")
	sideWidth    int    // expanded px (default 280)
	sideCollapse int    // collapsed px (default 64)
}

// scanShellLayout inspects a form's controls for §11 shell controls.
func scanShellLayout(controlsTbl *lua.LTable) shellLayout {
	var sl shellLayout
	if controlsTbl == nil {
		return sl
	}
	sl.sideWidth = 280
	sl.sideCollapse = 64
	controlsTbl.ForEach(func(_, v lua.LValue) {
		ctrl, ok := v.(*lua.LTable)
		if !ok {
			return
		}
		switch ctrl.RawGetString("type").String() {
		case "topbar":
			sl.hasTopbar = true
		case "footer":
			sl.hasFooter = true
		case "sidebar":
			sl.hasSidebar = true
			if p := ctrl.RawGetString("position"); p != lua.LNil && p.String() == "right" {
				sl.sidePosition = "right"
			}
			if w := ctrl.RawGetString("width"); w != lua.LNil {
				if n := int(lua.LVAsNumber(w)); n > 0 {
					sl.sideWidth = n
				}
			}
			if w := ctrl.RawGetString("collapsed_width"); w != lua.LNil {
				if n := int(lua.LVAsNumber(w)); n > 0 {
					sl.sideCollapse = n
				}
			}
		}
	})
	return sl
}

// renderVerticalControls renders a vertical-layout form's controls in creation
// order (the form's "order" table), falling back to unordered iteration. Each
// control (including buttons) renders on its own full-width row — matching
// `kalua run` exactly (the preview loads the same kalua.css). §11 shell
// controls (topbar/sidebar/footer) are rendered separately by renderForm.
func renderVerticalControls(controlsTbl, tbl *lua.LTable) string {
	order := tbl.RawGetString("order")
	var html string
	if orderTbl, ok := order.(*lua.LTable); ok {
		orderTbl.ForEach(func(k, v lua.LValue) {
			if ctrl, ok := controlsTbl.RawGetString(v.String()).(*lua.LTable); ok {
				if !shellControl(ctrl.RawGetString("type").String()) {
					html += renderControl(ctrl)
				}
			}
		})
	} else {
		// Fallback to unordered iteration
		controlsTbl.ForEach(func(k, v lua.LValue) {
			if ctrl, ok := v.(*lua.LTable); ok {
				if !shellControl(ctrl.RawGetString("type").String()) {
					html += renderControl(ctrl)
				}
			}
		})
	}
	return html
}

// cellDef describes one grid cell (kforms_enhancements §6).
type cellDef struct {
	id     string
	width  int
	bg     string
	border string
	align  string
}

// renderGridForm renders a grid-layout form: it iterates the form's cells and
// renders each control inside its assigned cell container. Backward compatible:
// layout="grid" without cells auto-creates a single "main" cell (width 12), and
// controls without a cell (or with an unknown cell) fall back to "main".
func renderGridForm(tbl *lua.LTable) string {
	controlsTbl, _ := tbl.RawGetString("controls").(*lua.LTable)
	orderTbl, _ := tbl.RawGetString("order").(*lua.LTable)

	cells := parseCells(tbl)
	cellIDs := map[string]bool{}
	for _, c := range cells {
		cellIDs[c.id] = true
	}

	// buckets maps cell id → ordered control names.
	buckets := map[string][]string{}
	bucket := func(cellID, name string) {
		if !cellIDs[cellID] {
			cellID = "main"
		}
		buckets[cellID] = append(buckets[cellID], name)
	}

	if orderTbl != nil {
		orderTbl.ForEach(func(_, v lua.LValue) {
			name := v.String()
			if ctrl := controlsTbl.RawGetString(name); ctrl != lua.LNil {
				if ct, ok := ctrl.(*lua.LTable); ok && shellControl(ct.RawGetString("type").String()) {
					return // §11 shell controls render fixed, outside cells
				}
			}
			cellID := "main"
			if ctrl := controlsTbl.RawGetString(name); ctrl != lua.LNil {
				if ct, ok := ctrl.(*lua.LTable); ok {
					if c := ct.RawGetString("cell"); c != lua.LNil && c.String() != "" {
						cellID = c.String()
					}
				}
			}
			bucket(cellID, name)
		})
	} else {
		controlsTbl.ForEach(func(_, v lua.LValue) {
			ct, ok := v.(*lua.LTable)
			if !ok {
				return
			}
			if shellControl(ct.RawGetString("type").String()) {
				return
			}
			cellID := ct.RawGetString("cell").String()
			if cellID == "" {
				cellID = "main"
			}
			bucket(cellID, ct.RawGetString("name").String())
		})
	}

	// Backward compat: no cells defined → single auto "main" cell.
	if len(cells) == 0 {
		cells = []cellDef{{id: "main", width: 12}}
	}
	// Auto-create "main" when controls reference it but it was not defined.
	if len(buckets["main"]) > 0 && !cellIDs["main"] {
		cells = append(cells, cellDef{id: "main", width: 12})
	}

	var html string
	for _, c := range cells {
		html += renderCell(c, buckets[c.id], controlsTbl)
	}
	return html
}

// styleFromProps translates dynamic styling properties (set at runtime via
// k.set_property / k.ctrl.set_property) into a "name:value;name:value" CSS
// string for a form or control. Supported props:
//
//	bg, color        — raw CSS color values
//	font             — CSS font-family string, or an h1–h6/p text preset
//	font_size        — numeric px; overrides the preset/em font-size
//	style            — h1–h6/p text preset (font-size + font-weight)
//
// An empty string is returned when no styling props are set.
func styleFromProps(tbl *lua.LTable) string {
	var sb strings.Builder
	if v := tbl.RawGetString("bg"); v != lua.LNil && v.String() != "" {
		sb.WriteString("background:" + v.String() + ";")
	}
	if v := tbl.RawGetString("color"); v != lua.LNil && v.String() != "" {
		sb.WriteString("color:" + v.String() + ";")
	}

	fontFamily := ""
	preset := ""
	if v := tbl.RawGetString("font"); v != lua.LNil && v.String() != "" {
		if fontPreset(v.String()) != "" {
			preset = strings.ToLower(v.String())
		} else {
			fontFamily = v.String()
		}
	}
	if v := tbl.RawGetString("style"); v != lua.LNil && v.String() != "" {
		if fontPreset(v.String()) != "" {
			preset = strings.ToLower(v.String())
		}
	}

	if v := tbl.RawGetString("font_size"); v != lua.LNil {
		if n := int(lua.LVAsNumber(v)); n > 0 {
			preset = "" // explicit px wins over the em preset
			sb.WriteString("font-size:" + strconv.Itoa(n) + "px;")
		}
	}
	if fontFamily != "" {
		sb.WriteString("font-family:" + fontFamily + ";")
	}
	if preset != "" {
		sb.WriteString("font-size:" + fontPresetSize(preset) + ";")
		sb.WriteString("font-weight:" + fontPresetWeight(preset) + ";")
	}
	return strings.TrimSuffix(sb.String(), ";")
}

// fontPreset reports whether s is a supported text preset (h1–h6 or p).
func fontPreset(s string) string {
	switch strings.ToLower(s) {
	case "h1", "h2", "h3", "h4", "h5", "h6", "p":
		return strings.ToLower(s)
	}
	return ""
}

// fontPresetSize returns the CSS font-size for a text preset.
func fontPresetSize(preset string) string {
	switch preset {
	case "h1":
		return "2em"
	case "h2":
		return "1.5em"
	case "h3":
		return "1.17em"
	case "h4":
		return "1em"
	case "h5":
		return "0.83em"
	case "h6":
		return "0.67em"
	case "p":
		return "1em"
	}
	return ""
}

// fontPresetWeight returns the CSS font-weight for a text preset.
func fontPresetWeight(preset string) string {
	if preset == "p" {
		return "400"
	}
	return "700"
}

// parseCells reads the form's "cells" table into ordered defs. gopher-lua does
// not preserve insertion order for string keys, so the array form (each element
// a table with an id) is the canonical ordered representation; the map form is
// still supported and falls back to lexicographic order by cell id.
func parseCells(tbl *lua.LTable) []cellDef {
	cellsV := tbl.RawGetString("cells")
	cellsTbl, ok := cellsV.(*lua.LTable)
	if !ok {
		return nil
	}

	var defs []cellDef
	if cellsTbl.Len() > 0 {
		for i := 1; i <= cellsTbl.Len(); i++ {
			ct, ok := cellsTbl.RawGetInt(i).(*lua.LTable)
			if !ok {
				continue
			}
			id := ct.RawGetString("id").String()
			if id == "" {
				continue
			}
			defs = append(defs, cellFromTable(id, ct))
		}
		if len(defs) > 0 {
			return defs
		}
	}

	var ids []string
	pairs := map[string]*lua.LTable{}
	cellsTbl.ForEach(func(k, v lua.LValue) {
		id := k.String()
		if id == "" {
			return
		}
		ids = append(ids, id)
		if ct, ok := v.(*lua.LTable); ok {
			pairs[id] = ct
		}
	})
	sort.Strings(ids)
	for _, id := range ids {
		d := cellDef{id: id, width: 12}
		if ct := pairs[id]; ct != nil {
			d = cellFromTable(id, ct)
		}
		defs = append(defs, d)
	}
	return defs
}

// cellFromTable converts one cell definition table into a cellDef.
func cellFromTable(id string, ct *lua.LTable) cellDef {
	width := 12
	if v := ct.RawGetString("width"); v != lua.LNil {
		if n := int(lua.LVAsNumber(v)); n >= 1 && n <= 12 {
			width = n
		}
	}
	bg := ""
	if v := ct.RawGetString("bg"); v != lua.LNil && v.String() != "" {
		bg = v.String()
	} else if v := ct.RawGetString("background"); v != lua.LNil && v.String() != "" {
		bg = v.String()
	}
	border := ""
	if bt, ok := ct.RawGetString("border").(*lua.LTable); ok {
		w := 1
		if v := bt.RawGetString("width"); v != lua.LNil {
			if n := int(lua.LVAsNumber(v)); n > 0 {
				w = n
			}
		}
		color := "#ccc"
		if v := bt.RawGetString("color"); v != lua.LNil && v.String() != "" {
			color = v.String()
		}
		border = strconv.Itoa(w) + "px solid " + color
	}
	align := ""
	if v := ct.RawGetString("align"); v != lua.LNil {
		align = v.String()
	}
	return cellDef{id: id, width: width, bg: bg, border: border, align: align}
}

// renderCell renders a cell container and the controls assigned to it.
func renderCell(c cellDef, names []string, controlsTbl *lua.LTable) string {
	style := "grid-column: span " + strconv.Itoa(c.width) + ";"
	if c.bg != "" {
		style += "background-color: " + c.bg + ";"
	}
	if c.border != "" {
		style += "border: " + c.border + ";"
	}
	alignAttr := ""
	if c.align != "" && c.align != "left" {
		alignAttr = ` align="` + escAttr(c.align) + `"`
	}
	html := `<div class="kalua-cell" data-k-cell="` + escAttr(c.id) + `" style="` + escAttr(style) + `"` + alignAttr + `>`
	for _, name := range names {
		if ctrl := controlsTbl.RawGetString(name); ctrl != lua.LNil {
			if ct, ok := ctrl.(*lua.LTable); ok {
				html += renderControl(ct)
			}
		}
	}
	html += `</div>`
	return html
}

// escText escapes a string for safe use as HTML text content.
func escText(s string) string {
	return html.EscapeString(s)
}

// escAttr escapes a string for safe use as an HTML attribute value.
func escAttr(s string) string {
	return html.EscapeString(s)
}

// renderAttrs builds the standard data-k-* attributes for a control.
func renderAttrs(formName, name string) string {
	return ` data-k-form="` + escAttr(formName) + `" data-k-ctrl="` + escAttr(name) + `"`
}

// renderEnabledVisible builds the enabled/disabled and visible/hidden attributes.
func renderEnabledVisible(ctrl *lua.LTable) (enabled, visible string) {
	enabledVal := ctrl.RawGetString("enabled")
	if enabledVal != lua.LNil && enabledVal.String() == "false" {
		enabled = ` disabled`
	}
	visibleVal := ctrl.RawGetString("visible")
	if visibleVal != lua.LNil && visibleVal.String() == "false" {
		visible = ` style="display:none"`
	}
	return
}

// renderVisibility builds the enabled/visible attributes plus the per-control
// alignment (kforms_enhancements §6) via align-self on the control element;
// the align-self merges into the visibility style when both apply.
func renderVisibility(ctrl *lua.LTable) (enabled, visible string) {
	enabled, visible = renderEnabledVisible(ctrl)
	if a := ctrl.RawGetString("align"); a != lua.LNil && a.String() != "" && a.String() != "left" {
		alignSelf := "center"
		if a.String() == "right" {
			alignSelf = "flex-end"
		}
		if visible == "" {
			visible = ` style="align-self:` + alignSelf + `"`
		} else {
			inner := strings.TrimSuffix(strings.TrimPrefix(visible, ` style="`), `"`)
			visible = ` style="` + inner + `;align-self:` + alignSelf + `"`
		}
	}
	return
}

// mergeControlStyle folds per-widget styling options (bg, color, font, font_size,
// style — the same keys styleFromProps reads at the form level, kforms_enhancements.md
// §10) into the style attribute produced by renderVisibility. The styling is appended
// after the visibility/alignment rules so display/align-self keep priority. Returns the
// combined ` style="..."` attribute, or the visibility attribute unchanged when no
// styling option is set.
func mergeControlStyle(ctrl *lua.LTable, visible string) string {
	props := styleFromProps(ctrl)
	if props == "" {
		return visible
	}
	if visible == "" {
		return ` style="` + escAttr(props) + `"`
	}
	inner := strings.TrimSuffix(strings.TrimPrefix(visible, ` style="`), `"`)
	return ` style="` + escAttr(inner) + ";" + escAttr(props) + `"`
}

// renderButton renders a single button control. marginRight, when non-empty,
// is merged into the button's style attribute.
func renderButton(ctrl *lua.LTable, marginRight string) string {
	btnClass := "kalua-button kalua-button-primary"
	if v := ctrl.RawGetString("class"); v != lua.LNil {
		btnClass = escAttr(v.String())
	}
	formName := escAttr(ctrl.RawGetString("form").String())
	name := escAttr(ctrl.RawGetString("name").String())
	id := "c:" + formName + ":" + name
	label := ""
	if lv := ctrl.RawGetString("label"); lv != lua.LNil {
		label = escText(lv.String())
	}
	enabled, visible := renderVisibility(ctrl)
	// Per-widget styling (kforms_enhancements.md §10) merges into the button's
	// own style attribute; renderControl already merged its copy, but renderButton
	// recomputes visibility so the styling must fold in here too.
	visible = mergeControlStyle(ctrl, visible)
	if marginRight != "" {
		if visible == "" {
			visible = ` style="margin-right:` + marginRight + `"`
		} else {
			inner := strings.TrimSuffix(strings.TrimPrefix(visible, ` style="`), `"`)
			visible = ` style="` + inner + `;margin-right:` + marginRight + `"`
		}
	}
	attrs := renderAttrs(formName, name)
	return `<button type="button" class="` + btnClass + `" id="` + escAttr(id) + `" name="` + name + `"` + attrs + ` ` + enabled + visible + `>` + label + `</button>`
}

func renderControl(ctrl *lua.LTable) string {
	ctrlType := ctrl.RawGetString("type").String()
	name := escAttr(ctrl.RawGetString("name").String())
	formName := escAttr(ctrl.RawGetString("form").String())
	labelVal := ctrl.RawGetString("label")
	label := ""
	if labelVal != lua.LNil {
		label = escText(labelVal.String())
	}
	v := ctrl.RawGetString("value")
	if v == nil {
		v = lua.LNil
	}
	value := ""
	if v != lua.LNil {
		value = escAttr(v.String())
	}

	id := "c:" + formName + ":" + name

	enabled, visible := renderVisibility(ctrl)
	// Per-widget styling merges into the visibility/alignment style attribute;
	// every branch below (including the renderTable/renderGrid/renderChart/
	// renderLooper/renderImage/renderLogin sinks and renderButton) applies the
	// merged attribute to the control's outermost element.
	visible = mergeControlStyle(ctrl, visible)
	attrs := renderAttrs(formName, name)

	switch ctrlType {
	case "label":
		// Multiline labels render as a pre-wrap div so \n is preserved.
		if ctrl.RawGetString("multiline").String() == "true" {
			return `<div class="kalua-label kalua-label-multiline" id="` + escAttr(id) + `"` + visible + `>` + label + `</div>`
		}
		return `<label class="kalua-label" id="` + escAttr(id) + `"` + visible + `>` + label + `</label>`
	case "textbox":
		if looperDisplay(ctrl) {
			// Raw value (escaped exactly once below); the shared `value` var is
			// already attribute-escaped and must not be re-escaped as text.
			raw := ctrl.RawGetString("value")
			rawStr := ""
			if raw != lua.LNil {
				rawStr = raw.String()
			}
			return `<span class="kalua-looper-cell-value" id="` + escAttr(id) + `">` + escText(rawStr) + `</span>`
		}
		// Support password=true option for password input type
		inputType := "text"
		if ctrl.RawGetString("password").String() == "true" {
			inputType = "password"
		}
		if ctrl.RawGetString("multiline").String() == "true" {
			rows := 4
			if v := ctrl.RawGetString("rows"); v != lua.LNil {
				if n := int(lua.LVAsNumber(v)); n > 0 {
					rows = n
				}
			}
			cols := 50
			if v := ctrl.RawGetString("cols"); v != lua.LNil {
				if n := int(lua.LVAsNumber(v)); n > 0 {
					cols = n
				}
			}
			return `<div class="kalua-control"` + visible + `>
				<label class="kalua-label" for="` + escAttr(id) + `">` + label + `</label>
				<textarea class="kalua-textarea" id="` + escAttr(id) + `" name="` + name + `" rows="` + strconv.Itoa(rows) + `" cols="` + strconv.Itoa(cols) + `"` + attrs + enabled + `>` + value + `</textarea>
			</div>`
		}
		if datetime := ctrl.RawGetString("datetime"); datetime != lua.LNil && datetime != lua.LFalse {
			return `<div class="kalua-control"` + visible + `>
				<label class="kalua-label" for="` + escAttr(id) + `">` + label + `</label>
				<input type="text" class="kalua-input kalua-datetime" id="` + escAttr(id) + `" name="` + name + `" value="` + value + `" data-k-datetime-options="` + datetimeOptionsAttr(ctrl) + `"` + attrs + enabled + `>
			</div>`
		}
		return `<div class="kalua-control"` + visible + `>
			<label class="kalua-label" for="` + escAttr(id) + `">` + label + `</label>
			<input type="` + inputType + `" class="kalua-input" id="` + escAttr(id) + `" name="` + name + `" value="` + value + `"` + attrs + enabled + `>
		</div>`
	case "button":
		return renderButton(ctrl, "")
	case "combo", "list":
		items := ctrl.RawGetString("items")
		var options string
		if itemsTbl, ok := items.(*lua.LTable); ok {
			itemsTbl.ForEach(func(k, v lua.LValue) {
				options += `<option value="` + escAttr(k.String()) + `">` + escText(v.String()) + `</option>`
			})
		}
		size := ""
		if ctrlType == "list" {
			size = ` size="5"`
		}
		return `<div class="kalua-control"` + visible + `>
			<label class="kalua-label" for="` + escAttr(id) + `">` + label + `</label>
			<select class="kalua-select" id="` + escAttr(id) + `" name="` + name + `"` + attrs + size + enabled + `>` + options + `</select>
		</div>`
	case "checkbox":
		if looperDisplay(ctrl) {
			mark := ""
			if value == "true" || value == "1" {
				mark = "\u2713"
			}
			return `<span class="kalua-looper-cell-value" id="` + escAttr(id) + `">` + mark + `</span>`
		}
		checked := ""
		if value == "true" || value == "1" {
			checked = ` checked`
		}
		hiddenValue := ""
		if hv := ctrl.RawGetString("hidden_value"); hv != lua.LNil {
			hiddenValue = escAttr(hv.String())
		}
		hiddenInput := ""
		if hiddenValue != "" {
			hiddenInput = `<input type="hidden" name="` + name + `_hidden" value="` + hiddenValue + `">`
		}
		return `<div class="kalua-control kalua-checkbox-item"` + visible + `>
			<input type="checkbox" class="kalua-input" id="` + escAttr(id) + `" name="` + name + `" value="` + value + `"` + attrs + checked + enabled + `>
			<label class="kalua-label" for="` + escAttr(id) + `">` + label + `</label>
			` + hiddenInput + `
		</div>`
	case "radio":
		checked := ""
		if value == "true" || value == "1" {
			checked = ` checked`
		}
		hiddenValue := ""
		if hv := ctrl.RawGetString("hidden_value"); hv != lua.LNil {
			hiddenValue = escAttr(hv.String())
		}
		hiddenInput := ""
		if hiddenValue != "" {
			hiddenInput = `<input type="hidden" name="` + name + `_hidden" value="` + hiddenValue + `">`
		}
		return `<div class="kalua-control kalua-radio-item"` + visible + `>
			<input type="radio" class="kalua-input" id="` + escAttr(id) + `" name="` + name + `" value="` + value + `"` + attrs + checked + enabled + `>
			<label class="kalua-label" for="` + escAttr(id) + `">` + label + `</label>
			` + hiddenInput + `
		</div>`
	case "table":
		return renderTable(ctrl, formName, name, id, label, value, visible, enabled, attrs)
	case "looper":
		return renderLooper(ctrl, formName, name, id, visible)
	case "grid":
		return renderGrid(ctrl, formName, name, id, visible)
	case "chart":
		return renderChart(ctrl, formName, name, id, visible)
	case "image":
		return renderImage(ctrl, formName, name, id, visible)
	case "login":
		return renderLogin(ctrl, formName, name, id, visible)
	case "tree":
		return renderTree(ctrl, formName, name, id, visible)
	case "tab":
		return renderTab(ctrl, formName, name, id, visible)
	case "topbar":
		return renderTopbar(ctrl, formName, name, id, visible)
	case "sidebar":
		return renderSidebar(ctrl, formName, name, id, visible)
	case "footer":
		return renderFooter(ctrl, formName, name, id, visible)
	}
	return `<div class="kalua-control">Unknown control: ` + escText(ctrlType) + `</div>`
}

// RenderControl renders a control to HTML. Exported for use by session package.
func RenderControl(ctrl *lua.LTable) string {
	return renderControl(ctrl)
}

// renderLooper renders a looper container. When the looper is DB-linked, the
// container carries the DB paging contract as data-k-looper-* attributes and a
// template row derived from the row→template links; the client populates rows
// from looper_db_batch messages as the user scrolls.
func renderLooper(ctrl *lua.LTable, formName, name, id, visible string) string {
	columns := 1
	if v := ctrl.RawGetString("columns"); v != lua.LNil {
		if n := int(lua.LVAsNumber(v)); n > 0 {
			columns = n
		}
	}
	pageSize := 50
	if v := ctrl.RawGetString("page_size"); v != lua.LNil {
		if n := int(lua.LVAsNumber(v)); n > 0 {
			pageSize = n
		}
	}

	templateCells := looperTemplateHTML(ctrl, formName, name)
	dbLinked := ""
	if ctrl.RawGetString("db") != lua.LNil {
		dbLinked = ` data-k-looper-links="` + escAttr(looperLinksAttr(ctrl)) + `"`
	}
	// Row-template loopers (opts.row) get server-rendered rows ({index,html}
	// batches) instead of the value-cell model, signalled to the client here.
	htmlAttr := ""
	if ctrl.RawGetString("row") != lua.LNil {
		htmlAttr = ` data-k-looper-html="1"`
	}

	return `<div class="kalua-control"` + visible + `>
		<div class="kalua-looper" id="` + escAttr(id) + `"
		     data-k-form="` + escAttr(formName) + `" data-k-ctrl="` + escAttr(name) + `"
		     data-k-looper-columns="` + strconv.Itoa(columns) + `"
		     data-k-looper-page-size="` + strconv.Itoa(pageSize) + `"` + dbLinked + htmlAttr + `>
			<div class="kalua-looper-rows">` + templateCells + `</div>
			<div class="kalua-looper-sentinel"></div>
		</div>
	</div>`
}

// looperTemplateHTML emits the template row that defines one row's cell
// structure. Cells are keyed by the link's control name so the host can map
// looper_db_batch data onto them. A non-DB looper renders a single empty cell
// (no rows until a data source is attached).
func looperTemplateHTML(ctrl *lua.LTable, formName, name string) string {
	// Row-template loopers render rows server-side; no value-cell template.
	if ctrl.RawGetString("row") != lua.LNil {
		return ""
	}
	links := ctrl.RawGetString("links")
	if links == lua.LNil {
		return `<div class="kalua-looper-row" data-k-looper-template="1">
				<div class="kalua-looper-cell" data-k-looper-control="">
					<span class="kalua-looper-cell-value"></span>
				</div>
			</div>`
	}
	linksTbl, ok := links.(*lua.LTable)
	if !ok {
		return ""
	}
	var cells []string
	linksTbl.ForEach(func(_, v lua.LValue) {
		linkTbl, ok := v.(*lua.LTable)
		if !ok {
			return
		}
		control := linkTbl.RawGetString("control").String()
		if control == "" {
			control = linkTbl.RawGetString("ctrl").String()
		}
		prop := looperLinkProp(linkTbl, "property")
		if prop == "" {
			prop = looperLinkProp(linkTbl, "prop")
		}
		display := control
		if prop != "" && prop != "value" {
			display = control + "." + prop
		}
		if display == "" {
			display = "cell"
		}
		cells = append(cells, `<div class="kalua-looper-cell" data-k-looper-control="`+escAttr(display)+`">
					<span class="kalua-looper-cell-value"></span>
				</div>`)
	})
	// Hide the template row from the user; the client uses it only to learn the
	// per-row cell layout before replacing it with real (batched) rows.
	return `<div class="kalua-looper-row" data-k-looper-template="1" style="display:none">` + strings.Join(cells, "\n") + `</div>`
}

// looperLinksAttr renders the links table as a compact JSON attribute so the
// client knows the control order for map keys without the template row.
func looperLinksAttr(ctrl *lua.LTable) string {
	links := ctrl.RawGetString("links")
	linksTbl, ok := links.(*lua.LTable)
	if !ok {
		return "[]"
	}
	var parts []string
	linksTbl.ForEach(func(_, v lua.LValue) {
		linkTbl, ok := v.(*lua.LTable)
		if !ok {
			return
		}
		key := linkTbl.RawGetString("control").String()
		if key == "" {
			key = linkTbl.RawGetString("ctrl").String()
		}
		prop := looperLinkProp(linkTbl, "property")
		if prop == "" {
			prop = looperLinkProp(linkTbl, "prop")
		}
		if key == "" {
			return
		}
		if prop != "" && prop != "value" {
			key += "." + prop
		}
		parts = append(parts, `"`+jsonEscape(key)+`"`)
	})
	return "[" + strings.Join(parts, ",") + "]"
}

// looperLinkProp reads a looper link key guarding against LNil, whose .String()
// would come back as "nil" and corrupt data-k-looper-* attrs.
func looperLinkProp(linkTbl *lua.LTable, key string) string {
	v := linkTbl.RawGetString(key)
	if v == lua.LNil {
		return ""
	}
	return v.String()
}

// looperDisplay reports whether a control is a read-only value display inside a
// server-rendered looper row (Phase 4 row-template controls).
func looperDisplay(ctrl *lua.LTable) bool {
	v := ctrl.RawGetString("looper_display")
	return v == lua.LTrue || (v != lua.LNil && v.String() == "true")
}

// renderImage renders the §4.3 image control. When clickable, the <img> carries
// the data-k-form/data-k-ctrl attrs so the client reports clicks (value = src).
func renderImage(ctrl *lua.LTable, formName, name, id, visible string) string {
	src := ctrl.RawGetString("src")
	srcTxt := ""
	if src != lua.LNil {
		srcTxt = src.String()
	}
	alt := ctrl.RawGetString("alt")
	altTxt := ""
	if alt != lua.LNil {
		altTxt = alt.String()
	}
	fit := ctrl.RawGetString("fit")
	fitTxt := "contain"
	if fit != lua.LNil && fit.String() != "" {
		fitTxt = fit.String()
	}
	var style string
	if w := ctrl.RawGetString("width"); w != lua.LNil && w.String() != "" {
		style += "width:" + cssLength(w.String()) + ";"
	}
	if h := ctrl.RawGetString("height"); h != lua.LNil && h.String() != "" {
		style += "height:" + cssLength(h.String()) + ";"
	}
	style += "object-fit:" + cssLength(fitTxt) + ";"

	data := ""
	if ctrl.RawGetString("clickable").String() == "true" {
		data = ` data-k-form="` + escAttr(formName) + `" data-k-ctrl="` + escAttr(name) + `"`
	}

	return `<div class="kalua-control kalua-image-container"` + visible + `>
		<img class="kalua-image" id="` + escAttr(id) + `" src="` + escAttr(srcTxt) + `" alt="` + escAttr(altTxt) + `" style="` + style + `"` + data + `>
	</div>`
}

// renderLogin renders the §8 login control panel.
// The control is shown as a modal form panel with login/password inputs,
// error display, and submit/cancel buttons.
func renderLogin(ctrl *lua.LTable, formName, name, id, visible string) string {
	// Get options with defaults
	title := ctrl.RawGetString("title")
	titleTxt := "Sign in"
	if title != lua.LNil && title.String() != "" {
		titleTxt = escText(title.String())
	}

	subtitle := ctrl.RawGetString("subtitle")
	subtitleTxt := "Enter your credentials"
	if subtitle != lua.LNil && subtitle.String() != "" {
		subtitleTxt = escText(subtitle.String())
	}

	loginLabel := ctrl.RawGetString("login_label")
	loginLabelTxt := "Login"
	if loginLabel != lua.LNil && loginLabel.String() != "" {
		loginLabelTxt = escText(loginLabel.String())
	}

	passwordLabel := ctrl.RawGetString("password_label")
	passwordLabelTxt := "Password"
	if passwordLabel != lua.LNil && passwordLabel.String() != "" {
		passwordLabelTxt = escText(passwordLabel.String())
	}

	loginPlaceholder := ctrl.RawGetString("login_placeholder")
	loginPlaceholderTxt := ""
	if loginPlaceholder != lua.LNil && loginPlaceholder.String() != "" {
		loginPlaceholderTxt = ` placeholder="` + escAttr(loginPlaceholder.String()) + `"`
	}

	passwordPlaceholder := ctrl.RawGetString("password_placeholder")
	passwordPlaceholderTxt := ""
	if passwordPlaceholder != lua.LNil && passwordPlaceholder.String() != "" {
		passwordPlaceholderTxt = ` placeholder="` + escAttr(passwordPlaceholder.String()) + `"`
	}

	submitLabel := ctrl.RawGetString("submit_label")
	submitLabelTxt := "Sign IN"
	if submitLabel != lua.LNil && submitLabel.String() != "" {
		submitLabelTxt = escText(submitLabel.String())
	}

	cancelLabel := ctrl.RawGetString("cancel_label")
	cancelLabelTxt := "Cancel"
	if cancelLabel != lua.LNil && cancelLabel.String() != "" {
		cancelLabelTxt = escText(cancelLabel.String())
	}

	errorMessage := ctrl.RawGetString("error_message")
	errorMessageTxt := "Invalid login or password"
	if errorMessage != lua.LNil && errorMessage.String() != "" {
		errorMessageTxt = escText(errorMessage.String())
	}

	// Current error state and preserved login value
	errorVal := ctrl.RawGetString("error")
	errorTxt := ""
	if errorVal != lua.LNil && errorVal.String() != "" {
		errorTxt = escText(errorVal.String())
	} else {
		errorTxt = errorMessageTxt
	}

	loginValueVal := ctrl.RawGetString("login_value")
	loginValueTxt := ""
	if loginValueVal != lua.LNil && loginValueVal.String() != "" {
		loginValueTxt = escAttr(loginValueVal.String())
	}

	// The control element itself carries data-k-login="1" so the client can identify it
	// The two action buttons use data-k-login-submit and data-k-login-cancel
	// The inputs carry data-k-form/data-k-ctrl so gridFormValues picks them up

	return `<div class="kalua-control kalua-login" id="` + escAttr(id) + `" data-k-login="1" data-k-form="` + escAttr(formName) + `" data-k-ctrl="` + escAttr(name) + `"` + visible + `>
		<div class="kalua-login-title">` + titleTxt + `</div>
		<div class="kalua-login-subtitle">` + subtitleTxt + `</div>
		<label class="kalua-label" for="` + escAttr(id) + `:login">` + loginLabelTxt + `</label>
		<input class="kalua-input kalua-login-input" type="text" id="` + escAttr(id) + `:login" name="login" value="` + loginValueTxt + `"` + loginPlaceholderTxt + ` data-k-form="` + escAttr(formName) + `" data-k-ctrl="` + escAttr(name) + `">
		<label class="kalua-label" for="` + escAttr(id) + `:password">` + passwordLabelTxt + `</label>
		<input class="kalua-input kalua-login-input" type="password" id="` + escAttr(id) + `:password" name="password" value=""` + passwordPlaceholderTxt + ` data-k-form="` + escAttr(formName) + `" data-k-ctrl="` + escAttr(name) + `">
		<div class="kalua-login-error">` + errorTxt + `</div>
		<div class="kalua-login-actions">
			<button type="button" class="kalua-btn" data-k-login-cancel="1" data-k-form="` + escAttr(formName) + `" data-k-ctrl="` + escAttr(name) + `">` + cancelLabelTxt + `</button>
			<button type="button" class="kalua-btn kalua-login-submit" data-k-login-submit="1" data-k-form="` + escAttr(formName) + `" data-k-ctrl="` + escAttr(name) + `">` + submitLabelTxt + `</button>
		</div>
	</div>`
}

// renderTree renders a tree control container (kforms_enhancements.md §9). The
// Go side does not render nodes — the container carries the DB-link config as
// data-k-tree-* attributes and the client issues a single tree_data_request,
// then renders the nested JSON payload locally for cheap expand/collapse.
func renderTree(ctrl *lua.LTable, formName, name, id, visible string) string {
	expandAll := "false"
	if v := ctrl.RawGetString("expand_all"); v != lua.LNil && v.String() == "true" {
		expandAll = "true"
	}
	selection := "single"
	if v := ctrl.RawGetString("selection"); v != lua.LNil && v.String() != "" {
		selection = escAttr(v.String())
	}
	errorTxt := ""
	if v := ctrl.RawGetString("error"); v != lua.LNil {
		errorTxt = escAttr(v.String())
	}

	return `<div class="kalua-control kalua-tree-control"` + visible + `>
		<div class="kalua-tree" id="` + escAttr(id) + `"
		     data-k-form="` + escAttr(formName) + `" data-k-ctrl="` + escAttr(name) + `"
		     data-k-tree-expand-all="` + expandAll + `"
		     data-k-tree-selection="` + selection + `"
		     data-k-tree-error="` + errorTxt + `">
			<div class="kalua-tree-empty">Loading…</div>
		</div>
	</div>`
}

// renderTab renders a tab control (kforms_enhancements.md §10). The control
// organizes content into tabs showing only the active tab's panel at a time.
// The container carries data-k-tab-* attributes; the client (app.js) handles
// click/swipe switching and sends tab_change events to the host.
func renderTab(ctrl *lua.LTable, formName, name, id, visible string) string {
	position := "top"
	if v := ctrl.RawGetString("tab_position"); v != lua.LNil && v.String() != "" {
		position = escAttr(v.String())
	}
	swipable := "true"
	if v := ctrl.RawGetString("swipable"); v != lua.LNil && v.String() == "false" {
		swipable = "false"
	}
	currentTab := 1
	if v := ctrl.RawGetString("current_tab"); v != lua.LNil {
		if n := int(lua.LVAsNumber(v)); n > 0 {
			currentTab = n
		}
	}

	tabs := ctrl.RawGetString("tabs")
	var tabsHtml, panelsHtml string
	if tabsTbl, ok := tabs.(*lua.LTable); ok {
		tabsTbl.ForEach(func(k, v lua.LValue) {
			tab, ok := v.(*lua.LTable)
			if !ok {
				return
			}
			idx := k.String()
			title := ""
			if tv := tab.RawGetString("title"); tv != lua.LNil {
				title = escText(tv.String())
			}
			icon := ""
			if iv := tab.RawGetString("icon"); iv != lua.LNil {
				icon = escText(iv.String()) + " "
			}
			selected := idx == strconv.Itoa(currentTab)
			ariaSelected := "false"
			if selected {
				ariaSelected = "true"
			}
			tabsHtml += `<button type="button" class="kalua-tab-btn" data-k-tab-btn="` + idx + `"
				aria-selected="` + ariaSelected + `">` + icon + title + `</button>`
			ariaHidden := "true"
			if selected {
				ariaHidden = "false"
			}
			panelsHtml += `<div class="kalua-tab-panel" data-k-tab-panel="` + idx + `"
				aria-hidden="` + ariaHidden + `"></div>`
		})
	}

	return `<div class="kalua-tab"` + visible + `>
		<div class="kalua-tab-headers" id="` + escAttr(id) + `-headers"
		     data-k-form="` + escAttr(formName) + `" data-k-ctrl="` + escAttr(name) + `"
		     data-k-tab-position="` + position + `"
		     data-k-tab-swipable="` + swipable + `"
		     data-k-tab-current-tab="` + strconv.Itoa(currentTab) + `">
			` + tabsHtml + `
		</div>
		<div class="kalua-tab-panels" id="` + escAttr(id) + `-panels">
			` + panelsHtml + `
		</div>
	</div>`
}

// formHasControlType reports whether a form's controls table contains a control
// of the given type. Used by renderForm to tag shell-layout classes.
func formHasControlType(controls lua.LValue, ctrlType string) bool {
	if controls == lua.LNil {
		return false
	}
	controlsTbl, ok := controls.(*lua.LTable)
	if !ok {
		return false
	}
	found := false
	controlsTbl.ForEach(func(_, v lua.LValue) {
		if found {
			return
		}
		if ct, ok := v.(*lua.LTable); ok && ct.RawGetString("type").String() == ctrlType {
			found = true
		}
	})
	return found
}

// renderTopbar renders the §11.1 fixed topbar (k.ctrl.topbar): app branding,
// optional user info (avatar/name/role) and a logout affordance. The user
// block carries data-k-topbar-user so the client reports a "logout" event on
// click. position:fixed styling comes from kalua.css.
func renderTopbar(ctrl *lua.LTable, formName, name, id, visible string) string {
	// The shared per-widget styling (bg/color/...) merges into `visible`.
	var inner string
	if v := ctrl.RawGetString("icon"); v != lua.LNil && v.String() != "" {
		inner += `<img class="kalua-topbar-icon" src="` + escAttr(v.String()) + `" alt="">`
	}
	if v := ctrl.RawGetString("title"); v != lua.LNil && v.String() != "" {
		inner += `<span class="kalua-topbar-title">` + escText(v.String()) + `</span>`
	}
	inner += `<div class="kalua-topbar-spacer"></div>`
	if u := ctrl.RawGetString("user"); u != lua.LNil {
		if userTbl, ok := u.(*lua.LTable); ok {
			user := ""
			if av := userTbl.RawGetString("avatar"); av != lua.LNil && av.String() != "" {
				user += `<img class="kalua-topbar-avatar" src="` + escAttr(av.String()) + `" alt="">`
			}
			if nv := userTbl.RawGetString("name"); nv != lua.LNil && nv.String() != "" {
				user += `<span class="kalua-topbar-user-name">` + escText(nv.String()) + `</span>`
			}
			if rv := userTbl.RawGetString("role"); rv != lua.LNil && rv.String() != "" {
				user += `<span class="kalua-topbar-user-role">` + escText(rv.String()) + `</span>`
			}
			if user != "" {
				inner += `<div class="kalua-topbar-user" data-k-topbar-user="1" role="button" tabindex="0"` +
					` title="` + escAttr(name) + ` logout">` + user + `</div>`
			}
		}
	}
	return `<div class="kalua-topbar" id="` + escAttr(id) + `"` + attrsFor(formName, name) + ` data-k-topbar="1"` + visible + `>` + inner + `</div>`
}

// attrsFor builds the shared data-k-form/data-k-ctrl attribute pair.
func attrsFor(formName, name string) string {
	return ` data-k-form="` + escAttr(formName) + `" data-k-ctrl="` + escAttr(name) + `"`
}

// renderSidebar renders the §11.2 collapsible sidebar (k.ctrl.sidebar). Items
// are nav buttons carrying data-k-sidebar-item plus the item JSON (data-k-value)
// so the client reports sidebar_select {action, item, index}. The layout mode
// (grid cell vs vertical tab bar) is set by addControl from the owning form and
// rendered as data-k-sidebar-layout; collapse state is session-only and handled
// client-side (data-k-sidebar-collapsed + .kalua-sidebar-collapsed).
func renderSidebar(ctrl *lua.LTable, formName, name, id, visible string) string {
	position := "left"
	if v := ctrl.RawGetString("position"); v != lua.LNil && v.String() != "" {
		position = escAttr(v.String())
	}
	width := 280
	if v := ctrl.RawGetString("width"); v != lua.LNil {
		if n := int(lua.LVAsNumber(v)); n > 0 {
			width = n
		}
	}
	collapsedWidth := 64
	if v := ctrl.RawGetString("collapsed_width"); v != lua.LNil {
		if n := int(lua.LVAsNumber(v)); n > 0 {
			collapsedWidth = n
		}
	}
	collapsible := "true"
	if v := ctrl.RawGetString("collapsible"); v != lua.LNil && v.String() == "false" {
		collapsible = "false"
	}
	collapsed := "false"
	if v := ctrl.RawGetString("collapsed"); v != lua.LNil && v.String() == "true" {
		collapsed = "true"
	}
	layout := "vertical"
	if v := ctrl.RawGetString("layout"); v != lua.LNil && v.String() != "" {
		layout = escAttr(v.String())
	}
	// Per-widget styling: the control's own bg/color/font props plus the §11.2
	// `style` table ({bg=..., border_right=...}) fold into the element style.
	// Widths ride as CSS variables so the collapsed class can swap them without
	// inline style juggling.
	visible = mergeCSS(visible, styleTableCSS(ctrl))
	cssVars := "--kalua-sidebar-width:" + strconv.Itoa(width) + "px;" +
		"--kalua-sidebar-collapsed-width:" + strconv.Itoa(collapsedWidth) + "px"
	visible = mergeCSS(visible, cssVars)

	var toggle string
	if collapsible == "true" {
		toggle = `<button type="button" class="kalua-sidebar-toggle" data-k-sidebar-toggle="1" aria-label="Toggle sidebar" title="Toggle sidebar">⇤</button>`
	}

	var items string
	if itemsTbl, ok := ctrl.RawGetString("items").(*lua.LTable); ok {
		idx := 0
		itemsTbl.ForEach(func(_, v lua.LValue) {
			itemTbl, ok := v.(*lua.LTable)
			if !ok {
				return
			}
			if itemTbl.RawGetString("type").String() == "separator" {
				items += `<div class="kalua-sidebar-separator" role="separator"></div>`
				return
			}
			idx++
			label := ""
			if lv := itemTbl.RawGetString("label"); lv != lua.LNil {
				label = escText(lv.String())
			}
			action := ""
			if av := itemTbl.RawGetString("action"); av != lua.LNil {
				action = escAttr(av.String())
			}
			icon := ""
			if iv := itemTbl.RawGetString("icon"); iv != lua.LNil && iv.String() != "" {
				icon = `<img class="kalua-sidebar-icon" src="` + escAttr(iv.String()) + `" alt="">`
			}
			badge := ""
			if bv := itemTbl.RawGetString("badge"); bv != lua.LNil && bv.String() != "" {
				badge = `<span class="kalua-sidebar-badge">` + escText(bv.String()) + `</span>`
			}
			labelWrap := ""
			if label != "" {
				labelWrap = `<span class="kalua-sidebar-label">` + label + `</span>`
			}
			items += `<button type="button" class="kalua-sidebar-item" data-k-sidebar-item="1"` +
				` data-k-sidebar-action="` + action + `" data-k-sidebar-index="` + strconv.Itoa(idx) + `"` +
				` data-k-value="` + escAttr(luaTableToJSON(itemTbl)) + `">` +
				icon + labelWrap + badge + `</button>`
		})
	}

	return `<div class="kalua-sidebar" id="` + escAttr(id) + `"` + attrsFor(formName, name) + visible + `
		data-k-sidebar="1"
		data-k-sidebar-position="` + position + `"
		data-k-sidebar-width="` + strconv.Itoa(width) + `"
		data-k-sidebar-collapsed-width="` + strconv.Itoa(collapsedWidth) + `"
		data-k-sidebar-collapsible="` + collapsible + `"
		data-k-sidebar-collapsed="` + collapsed + `"
		data-k-sidebar-layout="` + layout + `">
		` + toggle + `
		<div class="kalua-sidebar-items">` + items + `</div>
	</div>`
}

// renderFooter renders the §11.3 fixed footer (k.ctrl.footer): copyright text,
// optional links, and the auto-filled KALUA version + script name.
func renderFooter(ctrl *lua.LTable, formName, name, id, visible string) string {
	visible = mergeCSS(visible, styleTableCSS(ctrl))

	var inner string
	if v := ctrl.RawGetString("text"); v != lua.LNil && v.String() != "" {
		inner += `<span class="kalua-footer-text">` + escText(v.String()) + `</span>`
	}
	if linksTbl, ok := ctrl.RawGetString("links").(*lua.LTable); ok {
		linksTbl.ForEach(func(_, v lua.LValue) {
			linkTbl, ok := v.(*lua.LTable)
			if !ok {
				return
			}
			label := ""
			if lv := linkTbl.RawGetString("label"); lv != lua.LNil {
				label = escText(lv.String())
			}
			url := ""
			if uv := linkTbl.RawGetString("url"); uv != lua.LNil {
				url = escAttr(uv.String())
			}
			if label != "" && url != "" {
				inner += `<a class="kalua-footer-link" href="` + url + `" target="_blank" rel="noopener">` + label + `</a>`
			}
		})
	}
	// Version/script_name are auto-filled (§11.3): version falls back to the
	// running KALUA binary version when omitted; script_name is filled by the
	// host (bindings.Options.ScriptName) at control creation.
	versionTxt := ""
	if v := ctrl.RawGetString("version"); v != lua.LNil && v.String() != "" {
		versionTxt = escText(v.String())
	} else {
		versionTxt = escText(version.Get().Version)
	}
	if versionTxt != "" {
		inner += `<span class="kalua-footer-version">` + versionTxt + `</span>`
	}
	if v := ctrl.RawGetString("script_name"); v != lua.LNil && v.String() != "" {
		inner += `<span class="kalua-footer-script">` + escText(v.String()) + `</span>`
	}

	return `<footer class="kalua-footer" id="` + escAttr(id) + `"` + attrsFor(formName, name) + ` data-k-footer="1"` + visible + `>` + inner + `</footer>`
}

// styleTableCSS converts a control's `style` option table
// ({bg=..., border_right=...}) into a "property:value;property:value;" CSS
// string. Key normalization: `bg` maps to `background`, snake_case (_) and
// camelCase map to the kebab-case CSS property; values are emitted verbatim.
func styleTableCSS(ctrl *lua.LTable) string {
	st, ok := ctrl.RawGetString("style").(*lua.LTable)
	if !ok {
		return ""
	}
	var parts []string
	st.ForEach(func(k, v lua.LValue) {
		if v == lua.LNil || v.String() == "" {
			return
		}
		prop := stylePropName(k.String())
		if prop == "" {
			return
		}
		parts = append(parts, prop+":"+v.String()+";")
	})
	return strings.Join(parts, "")
}

// stylePropName maps a style-table key to a CSS property name: `bg` becomes
// `background`, `_` becomes `-`, and camelCase becomes kebab-case (borderRight
// → border-right). Unmappable names are dropped.
func stylePropName(k string) string {
	switch k {
	case "bg":
		return "background"
	case "bg_color":
		return "background-color"
	case "text_color":
		return "color"
	}
	k = strings.ReplaceAll(k, "_", "-")
	var sb strings.Builder
	for i := 0; i < len(k); i++ {
		c := k[i]
		if c >= 'A' && c <= 'Z' {
			if i > 0 {
				sb.WriteByte('-')
			}
			sb.WriteByte(c - 'A' + 'a')
			continue
		}
		sb.WriteByte(c)
	}
	return sb.String()
}

// mergeCSS merges extra CSS properties into the visibility/alignment style
// attribute produced by renderVisibility/renderControl, returning the combined
// ` style="..."` attribute.
func mergeCSS(visible, props string) string {
	if props == "" {
		return visible
	}
	if visible == "" {
		return ` style="` + escAttr(props) + `"`
	}
	inner := strings.TrimSuffix(strings.TrimPrefix(visible, ` style="`), `"`)
	return ` style="` + escAttr(inner) + ";" + escAttr(props) + `"`
}

// cssLength maps a numeric value to a px length and passes %, auto, keywords
// through unchanged.
func cssLength(s string) string {
	if s == "" || s == "auto" {
		return s
	}
	if _, err := strconv.ParseFloat(s, 64); err == nil {
		return s + "px"
	}
	return s
}

// datetimeOptionsAttr builds the flatpickr config JSON embedded in the
// data-k-datetime-options attribute for a §4.1 datetime textbox. It reads the
// flattened datetime_* keys (populated by addControl) and falls back to reading
// the datetime table directly for robustness.
func datetimeOptionsAttr(ctrl *lua.LTable) string {
	flattened := ctrl.RawGetString("datetime_mode") != lua.LNil
	dt := func(key string) string {
		if flattened {
			if v := ctrl.RawGetString("datetime_" + key); v != lua.LNil {
				return v.String()
			}
			return ""
		}
		dtTbl, ok := ctrl.RawGetString("datetime").(*lua.LTable)
		if !ok {
			return ""
		}
		if v := dtTbl.RawGetString(key); v != lua.LNil {
			return v.String()
		}
		return ""
	}
	dtNum := func(key string) int {
		if flattened {
			if v := ctrl.RawGetString("datetime_" + key); v != lua.LNil {
				return int(lua.LVAsNumber(v))
			}
			return 0
		}
		dtTbl, ok := ctrl.RawGetString("datetime").(*lua.LTable)
		if !ok {
			return 0
		}
		if v := dtTbl.RawGetString(key); v != lua.LNil {
			return int(lua.LVAsNumber(v))
		}
		return 0
	}

	mode := dt("mode")
	if mode == "" {
		mode = "datetime"
	}
	format := dt("format")
	min := dt("min")
	max := dt("max")
	step := dtNum("step")

	cfg := map[string]interface{}{}
	switch mode {
	case "date":
		cfg["enableTime"] = false
		cfg["noCalendar"] = false
		cfg["dateFormat"] = "Y-m-d"
	case "time":
		cfg["enableTime"] = true
		cfg["noCalendar"] = true
		cfg["dateFormat"] = "H:i"
	default:
		cfg["enableTime"] = true
		cfg["noCalendar"] = false
		cfg["dateFormat"] = "Y-m-d H:i"
	}
	if format != "" {
		cfg["dateFormat"] = flatpickrFormat(format)
	}
	if min != "" {
		cfg["minDate"] = min
	}
	if max != "" {
		cfg["maxDate"] = max
	}
	if step > 0 {
		cfg["minuteIncrement"] = step
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		return ""
	}
	return escAttr(string(b))
}

// flatpickrFormat translates a Kalipso display format ("YYYY-MM-DD HH:MM") to
// the flatpickr token dialect (Y-m-d H:i). A space splits date and time parts;
// a ":" marks a time-only format.
func flatpickrFormat(s string) string {
	if date, time, ok := strings.Cut(s, " "); ok {
		return flatpickrDate(date) + " " + flatpickrTime(time)
	}
	if strings.Contains(s, ":") {
		return flatpickrTime(s)
	}
	return flatpickrDate(s)
}

func flatpickrDate(s string) string {
	s = strings.ReplaceAll(s, "YYYY", "Y")
	s = strings.ReplaceAll(s, "MM", "m")
	s = strings.ReplaceAll(s, "DD", "d")
	return s
}

func flatpickrTime(s string) string {
	s = strings.ReplaceAll(s, "HH", "H")
	s = strings.ReplaceAll(s, "MM", "i")
	return s
}

// isTabulator reports whether a control renders as a Tabulator instance.
// Tables opt in via tabulator=true; grid controls are always Tabulator-backed
// (kforms_enhancements.md §7), so the shared read pager and client lifecycle
// treat both identically.
func isTabulator(ctrl *lua.LTable) bool {
	if ctrl.RawGetString("type").String() == "grid" {
		return true
	}
	t := ctrl.RawGetString("tabulator")
	return t != lua.LNil && t.String() == "true"
}

// isDBLinked reports whether a table control carries a DB link (db handle +
// base SELECT), i.e. the Go pager should serve its pages.
func isDBLinked(ctrl *lua.LTable) bool {
	db := ctrl.RawGetString("db")
	q := ctrl.RawGetString("query")
	return db != lua.LNil && db.String() != "" && q != lua.LNil && q.String() != ""
}

// forceRemotePaging merges remote pagination into a Tabulator options JSON
// string. DB-linked tables always page server-side, so paginationMode is forced
// to "remote" and paginationSize defaults to the control's page_size. Existing
// explicit values win.
func forceRemotePaging(optionsJSON string, ctrl *lua.LTable) string {
	var opts map[string]interface{}
	if err := json.Unmarshal([]byte(optionsJSON), &opts); err != nil || opts == nil {
		opts = map[string]interface{}{}
	}
	opts["paginationMode"] = "remote"
	if _, has := opts["paginationSize"]; !has {
		if v := ctrl.RawGetString("page_size"); v != lua.LNil {
			if n := int(lua.LVAsNumber(v)); n > 0 {
				opts["paginationSize"] = n
			}
		}
	}
	out, err := json.Marshal(opts)
	if err != nil {
		return optionsJSON
	}
	return string(out)
}

// renderTable renders a table control, dispatching to the traditional or the
// Tabulator renderer. Accessed via renderControl's "table" case.
func renderTable(ctrl *lua.LTable, formName, name, id, label, value, visible, enabled, attrs string) string {
	if isTabulator(ctrl) {
		return renderTabulatorTable(ctrl, formName, name, id, label, visible, enabled)
	}
	return renderTraditionalTable(ctrl, formName, name, id, label, value, visible, enabled, attrs)
}

// renderTabulatorTable renders the container div. The client reads the
// data-k-tabulator-* attributes and initializes a Tabulator instance.
func renderTabulatorTable(ctrl *lua.LTable, formName, name, id, label, visible, enabled string) string {
	optionsJSON, columnsJSON, dataJSON := tabulatorWidgetJSON(ctrl)

	return `<div class="kalua-control"` + visible + `>
		<div class="kalua-tabulator-wrapper"` + enabled + `>
			<div id="` + escAttr(id) + `" class="kalua-tabulator-table"
			     data-k-form="` + escAttr(formName) + `" data-k-ctrl="` + escAttr(name) + `"
			     data-k-tabulator-options="` + escAttr(optionsJSON) + `"
			     data-k-tabulator-columns="` + escAttr(columnsJSON) + `"
			     data-k-tabulator-data="` + escAttr(dataJSON) + `"></div>
		</div>
	</div>`
}

// tabulatorWidgetJSON computes the JSON for the data-k-tabulator-* attributes
// shared by the table and grid renderers: options (with remote pagination
// forced for DB-linked sources), columns and row data.
func tabulatorWidgetJSON(ctrl *lua.LTable) (optionsJSON, columnsJSON, dataJSON string) {
	optionsJSON = `{"layout":"fitColumns","selectable":true,"selectableRangeMode":"click"}`
	if to := ctrl.RawGetString("tabulatorOptions"); to != lua.LNil {
		if toTbl, ok := to.(*lua.LTable); ok {
			optionsJSON = luaTableToJSON(toTbl)
		} else if to.String() != "" {
			optionsJSON = to.String()
		}
	}
	// DB-linked sources must page through the Go host; force remote pagination
	// (and size) so the client installs the dataLoader that drives it.
	if isDBLinked(ctrl) {
		optionsJSON = forceRemotePaging(optionsJSON, ctrl)
	}

	columnsJSON = "[]"
	defaultVisible := make(map[string]bool)
	if v := ctrl.RawGetString("default_visible"); v != lua.LNil {
		if vTbl, ok := v.(*lua.LTable); ok {
			vTbl.ForEach(func(_, val lua.LValue) {
				if s := val.String(); s != "" {
					defaultVisible[s] = true
				}
			})
		}
	}
	if cols := ctrl.RawGetString("columns"); cols != lua.LNil {
		if colsTbl, ok := cols.(*lua.LTable); ok {
			var colStrs []string
			colsTbl.ForEach(func(_ lua.LValue, v lua.LValue) {
				if colTbl, ok := v.(*lua.LTable); ok {
					// Apply default_visible if specified
					if field := colTbl.RawGetString("field"); field != lua.LNil {
						if defaultVisible[field.String()] {
							// Create a copy to avoid mutating original
							colCopy := colTbl
							colCopy.RawSetString("visible", lua.LTrue)
						}
					}
				}
				if colTbl, ok := v.(*lua.LTable); ok {
					colStrs = append(colStrs, luaTableToJSON(colTbl))
				} else if v.String() != "" {
					colStrs = append(colStrs, `"`+jsonEscape(v.String())+`"`)
				}
			})
			columnsJSON = "[" + strings.Join(colStrs, ",") + "]"
		}
	}

	dataJSON = "[]"
	if data := ctrl.RawGetString("data"); data != lua.LNil {
		if dataTbl, ok := data.(*lua.LTable); ok {
			dataJSON = luaTableToJSON(dataTbl)
		}
	}
	return
}

// renderTraditionalTable renders the classic <table> control.
func renderTraditionalTable(ctrl *lua.LTable, formName, name, id, label, value, visible, enabled, attrs string) string {
	columns := ctrl.RawGetString("columns")
	rows := ctrl.RawGetString("rows")

	var thead string
	if columnsTbl, ok := columns.(*lua.LTable); ok {
		thead = "<thead><tr>"
		columnsTbl.ForEach(func(k, v lua.LValue) {
			thead += `<th data-k-col="` + escAttr(k.String()) + `">` + escText(v.String()) + `</th>`
		})
		thead += "</tr></thead>"
	} else {
		thead = `<thead><tr><th>` + label + `</th></tr></thead>`
	}

	var tbody string
	if rowsTbl, ok := rows.(*lua.LTable); ok {
		tbody = "<tbody>"
		rowsTbl.ForEach(func(k, v lua.LValue) {
			if rowTbl, ok := v.(*lua.LTable); ok {
				tbody += "<tr data-k-row=\"" + escAttr(k.String()) + "\">"
				rowTbl.ForEach(func(colK, colV lua.LValue) {
					tbody += `<td data-k-col="` + escAttr(colK.String()) + `">` + escText(colV.String()) + `</td>`
				})
				tbody += "</tr>"
			}
		})
		tbody += "</tbody>"
	} else {
		tbody = "<tbody></tbody>"
	}

	return `<div class="kalua-control"` + visible + `>
		<table class="kalua-table" id="` + escAttr(id) + `"` + attrs + enabled + `>` + thead + tbody + `</table>
	</div>`
}

// TableToJSON exports luaTableToJSON for cross-package use (e.g. the session
// actor serializing a tabulator_ajax_request handler's Lua return value).
func TableToJSON(tbl *lua.LTable) string {
	return luaTableToJSON(tbl)
}

// luaTableToJSON converts a Lua table to a JSON string. Sequential 1..N
// numeric keys produce an array; otherwise an object is emitted. Strings and
// numbers are emitted literally; nested tables are recursed.
func luaTableToJSON(tbl *lua.LTable) string {
	var parts []string
	isArray := true
	expected := 1
	tbl.ForEach(func(k, v lua.LValue) {
		if n, ok := k.(lua.LNumber); ok && int(n) == expected {
			expected++
		} else {
			isArray = false
		}
		parts = append(parts, luaValueJSON(v))
	})

	if isArray {
		return "[" + strings.Join(parts, ",") + "]"
	}

	var objParts []string
	tbl.ForEach(func(k, v lua.LValue) {
		objParts = append(objParts, `"`+jsonEscape(k.String())+`":`+luaValueJSON(v))
	})
	return "{" + strings.Join(objParts, ",") + "}"
}

// luaValueJSON renders a single Lua value as JSON.
func luaValueJSON(v lua.LValue) string {
	switch v.Type() {
	case lua.LTString:
		return `"` + jsonEscape(v.String()) + `"`
	case lua.LTNumber:
		return v.String()
	case lua.LTBool:
		return v.String()
	case lua.LTNil:
		return "null"
	case lua.LTTable:
		return luaTableToJSON(v.(*lua.LTable))
	default:
		return `"` + jsonEscape(v.String()) + `"`
	}
}

// jsonEscape escapes a string for embedding in a JSON double-quoted value.
func jsonEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	s = strings.ReplaceAll(s, "\t", `\t`)
	return s
}

// chartDataJSON renders the control's {labels, datasets} data as JSON.
func chartDataJSON(ctrl *lua.LTable) string {
	return `{"labels":` + chartJSONOrNil(ctrl, "labels") + `,"datasets":` + chartJSONOrNil(ctrl, "datasets") + `}`
}

// chartJSONOrNil renders a control field (labels/datasets) as a JSON table, or
// an empty array when absent.
func chartJSONOrNil(ctrl *lua.LTable, key string) string {
	if tbl, ok := ctrl.RawGetString(key).(*lua.LTable); ok {
		return luaTableToJSON(tbl)
	}
	return "[]"
}

// chartConfigJSON renders the full Chart.js config (type, data, options) that
// is embedded in the <canvas> as data-k-chart-config. The hbar/area aliases
// are normalized to bar/line with the appropriate indexAxis/fill options, and
// convenience opts (responsive, legend, stacked, ...) seed the defaults that
// the user's `options` table can override.
func chartConfigJSON(ctrl *lua.LTable) string {
	chartType := ctrl.RawGetString("chart_type").String()
	if chartType == "" {
		chartType = "line"
	}

	jsType := chartType
	options := chartBaseOptions(ctrl)
	switch chartType {
	case "hbar":
		jsType = "bar"
		if _, exists := options["indexAxis"]; !exists {
			options["indexAxis"] = "y"
		}
	case "area":
		jsType = "line"
		if _, exists := options["elements"]; !exists {
			options["elements"] = map[string]interface{}{"line": map[string]interface{}{"fill": true}}
		}
	}

	if user := ctrl.RawGetString("options"); user != lua.LNil {
		if ut, ok := user.(*lua.LTable); ok {
			var overlay map[string]interface{}
			if err := json.Unmarshal([]byte(luaTableToJSON(ut)), &overlay); err == nil && overlay != nil {
				deepMergeMap(options, overlay)
			}
		}
	}

	cfg := map[string]interface{}{
		"type": jsType,
		"data": map[string]interface{}{
			"labels":   json.RawMessage(chartJSONOrNil(ctrl, "labels")),
			"datasets": json.RawMessage(chartJSONOrNil(ctrl, "datasets")),
		},
		"options": options,
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		return `{"type":"line","data":{"labels":[],"datasets":[]},"options":{}}`
	}
	return string(b)
}

// chartBaseOptions seeds Chart.js option defaults from the Chart.js-specific
// convenience opts on the control (responsive, legend, stacked, ...).
func chartBaseOptions(ctrl *lua.LTable) map[string]interface{} {
	o := map[string]interface{}{
		"responsive":          true,
		"maintainAspectRatio": false,
		"animation":           true,
		"plugins": map[string]interface{}{
			"legend": map[string]interface{}{
				"display":  true,
				"position": "top",
			},
		},
	}

	o["responsive"] = optBool(ctrl, "responsive", true)
	o["maintainAspectRatio"] = optBool(ctrl, "maintainAspectRatio", false)
	o["animation"] = optBool(ctrl, "animation", true)

	legend := o["plugins"].(map[string]interface{})
	legendOpts := legend["legend"].(map[string]interface{})
	legendOpts["display"] = optBool(ctrl, "legend", true)
	if pos := ctrl.RawGetString("legendPosition"); pos != lua.LNil && pos.String() != "" {
		legendOpts["position"] = pos.String()
	}

	if optBool(ctrl, "stacked", false) {
		o["scales"] = map[string]interface{}{
			"x": map[string]interface{}{"stacked": true},
			"y": map[string]interface{}{"stacked": true},
		}
	}

	return o
}

// optBool reads a boolean option from the control, returning def when absent.
func optBool(ctrl *lua.LTable, key string, def bool) bool {
	v := ctrl.RawGetString(key)
	if v == lua.LNil {
		return def
	}
	switch v.Type() {
	case lua.LTBool:
		return bool(v.(lua.LBool))
	case lua.LTNumber:
		return v.(lua.LNumber) != 0
	case lua.LTString:
		return v.String() == "true" || v.String() == "1"
	}
	return def
}

// deepMergeMap overlays src into dst at the leaf level (nested maps merge
// recursively; scalars from src win). Used to let the user's `options` table
// override the seeded defaults.
func deepMergeMap(dst, src map[string]interface{}) {
	for k, v := range src {
		if srcMap, ok := v.(map[string]interface{}); ok {
			if dstMap, ok := dst[k].(map[string]interface{}); ok {
				deepMergeMap(dstMap, srcMap)
				continue
			}
		}
		dst[k] = v
	}
}

// renderChart renders a Chart.js control: a titled container holding a canvas
// whose data-k-chart-config attribute carries the full Chart.js config JSON.
func renderChart(ctrl *lua.LTable, formName, name, id, visible string) string {
	cfg := escAttr(chartConfigJSON(ctrl))
	title := escText(ctrl.RawGetString("label").String())

	w := int(lua.LVAsNumber(ctrl.RawGetString("width")))
	h := int(lua.LVAsNumber(ctrl.RawGetString("height")))
	style := ""
	if w > 0 && h > 0 {
		style = ` style="width:` + strconv.Itoa(w) + `px;height:` + strconv.Itoa(h) + `px;"`
	}

	return `<div class="kalua-control"` + visible + `>
		<label class="kalua-label">` + title + `</label>
		<div class="kalua-chart-container" id="` + escAttr(id) + `"` + style + `>
			<canvas class="kalua-chart-canvas" data-k-chart-config="` + cfg + `" data-k-form="` + escAttr(formName) + `" data-k-ctrl="` + escAttr(name) + `"></canvas>
		</div>
	</div>`
}

// renderGrid renders a grid control. The inner element is the standard
// `.kalua-tabulator-table` container the browser's initTabulators manages; the
// outer `.kalua-grid` wrapper carries the data-k-grid-* CRUD configuration the
// client uses to build the toolbar, action column and form modal.
func renderGrid(ctrl *lua.LTable, formName, name, id, visible string) string {
	optionsJSON, columnsJSON, dataJSON := tabulatorWidgetJSON(ctrl)

	// Apply default row_actions if not specified but grid has any row action capability
	rowActionsJSON := ""
	if v := ctrl.RawGetString("row_actions"); v != lua.LNil {
		if aTbl, ok := v.(*lua.LTable); ok {
			if j := gridConfigJSON(aTbl); j != "" {
				rowActionsJSON = j
			}
		}
	} else {
		// Default: view, edit, delete
		rowActionsJSON = `{"view":true,"edit":true,"delete":true}`
	}

	// Apply default global_actions if not specified
	globalActionsJSON := ""
	if v := ctrl.RawGetString("global_actions"); v != lua.LNil {
		if aTbl, ok := v.(*lua.LTable); ok {
			if j := gridConfigJSON(aTbl); j != "" {
				globalActionsJSON = j
			}
		}
	} else {
		// Default: new_record, batch_delete
		globalActionsJSON = `{"new_record":true,"batch_delete":true}`
	}

	gridAttrs := ` data-k-grid="1" data-k-grid-selection="multi"`
	if v := ctrl.RawGetString("selection_mode"); v != lua.LNil && v.String() != "" {
		gridAttrs = ` data-k-grid="1" data-k-grid-selection="` + escAttr(v.String()) + `"`
	}
	if v := ctrl.RawGetString("pk_field"); v != lua.LNil && v.String() != "" {
		gridAttrs += ` data-k-grid-pk="` + escAttr(v.String()) + `"`
	}
	if rowActionsJSON != "" {
		gridAttrs += ` data-k-grid-row-actions="` + escAttr(rowActionsJSON) + `"`
	}
	if globalActionsJSON != "" {
		gridAttrs += ` data-k-grid-global-actions="` + escAttr(globalActionsJSON) + `"`
	}
	if v := ctrl.RawGetString("row_click_action"); v != lua.LNil && v.String() != "" {
		gridAttrs += ` data-k-grid-row-click="` + escAttr(v.String()) + `"`
	}
	if v := ctrl.RawGetString("column_visibility"); v != lua.LNil && v.String() == "true" {
		gridAttrs += ` data-k-grid-column-visibility="true"`
	}
	if v := ctrl.RawGetString("form"); v != lua.LNil {
		if fTbl, ok := v.(*lua.LTable); ok {
			gridAttrs += ` data-k-grid-form="` + escAttr(luaTableToJSON(fTbl)) + `"`
		} else if v.String() != "" {
			gridAttrs += ` data-k-grid-formref="` + escAttr(v.String()) + `"`
		}
	}

	return `<div class="kalua-control"` + visible + `>
		<div class="kalua-grid" data-k-form="` + escAttr(formName) + `" data-k-ctrl="` + escAttr(name) + `"` + gridAttrs + `>
			<div class="kalua-tabulator-wrapper">
				<div id="` + escAttr(id) + `" class="kalua-tabulator-table"
				     data-k-form="` + escAttr(formName) + `" data-k-ctrl="` + escAttr(name) + `"
				     data-k-tabulator-options="` + escAttr(optionsJSON) + `"
				     data-k-tabulator-columns="` + escAttr(columnsJSON) + `"
				     data-k-tabulator-data="` + escAttr(dataJSON) + `"></div>
			</div>
		</div>
	</div>`
}

// gridConfigJSON serializes a row_actions/global_actions config table for the
// data-k-grid-* attribute. Function values (custom onclick handlers) cannot
// cross the WebSocket boundary and are dropped; the client reads those through
// k.form.on handlers instead.
func gridConfigJSON(tbl *lua.LTable) string {
	var parts []string
	isArray := true
	expected := 1
	tbl.ForEach(func(k, v lua.LValue) {
		if v.Type() == lua.LTFunction || v == lua.LNil {
			isArray = false
			return
		}
		if n, ok := k.(lua.LNumber); ok && int(n) == expected {
			expected++
		} else {
			isArray = false
		}
		parts = append(parts, luaValueJSON(v))
	})
	if len(parts) == 0 {
		return ""
	}
	if isArray {
		return "[" + strings.Join(parts, ",") + "]"
	}
	var keys []string
	keyMap := map[string]lua.LValue{}
	tbl.ForEach(func(k, v lua.LValue) {
		if v.Type() == lua.LTFunction || v == lua.LNil {
			return
		}
		keyMap[k.String()] = v
		keys = append(keys, k.String())
	})
	sort.Strings(keys)
	var objParts []string
	for _, k := range keys {
		objParts = append(objParts, `"`+jsonEscape(k)+`":`+luaValueJSON(keyMap[k]))
	}
	if len(objParts) == 0 {
		return ""
	}
	return "{" + strings.Join(objParts, ",") + "}"
}
