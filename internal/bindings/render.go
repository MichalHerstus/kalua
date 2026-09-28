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

	var html string
	html += `<div id="f:` + formNameEsc + `" class="kalua-form"`
	if layout != "vertical" {
		html += ` layout="` + escAttr(layout) + `"`
	}
	if align != "" && align != "left" {
		html += ` align="` + escAttr(align) + `"`
	}
	style := "--kalua-gap:" + strconv.Itoa(gap) + "px"
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
	} else if controlsTbl, ok := controls.(*lua.LTable); ok {
		html += renderVerticalControls(controlsTbl, tbl)
	}

	html += `</div>`
	return html
}

// renderVerticalControls renders a vertical-layout form's controls in creation
// order (the form's "order" table), falling back to unordered iteration. Each
// control (including buttons) renders on its own full-width row — matching
// `kalua run` exactly (the preview loads the same kalua.css).
func renderVerticalControls(controlsTbl, tbl *lua.LTable) string {
	order := tbl.RawGetString("order")
	var html string
	if orderTbl, ok := order.(*lua.LTable); ok {
		orderTbl.ForEach(func(k, v lua.LValue) {
			if ctrl, ok := controlsTbl.RawGetString(v.String()).(*lua.LTable); ok {
				html += renderControl(ctrl)
			}
		})
	} else {
		// Fallback to unordered iteration
		controlsTbl.ForEach(func(k, v lua.LValue) {
			if ctrl, ok := v.(*lua.LTable); ok {
				html += renderControl(ctrl)
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
	attrs := renderAttrs(formName, name)

	switch ctrlType {
	case "label":
		// Multiline labels render as a pre-wrap div so \n is preserved.
		if ctrl.RawGetString("multiline").String() == "true" {
			return `<div class="kalua-label kalua-label-multiline" id="` + escAttr(id) + `"` + visible + `>` + label + `</div>`
		}
		return `<label class="kalua-label" id="` + escAttr(id) + `">` + label + `</label>`
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
			<input type="text" class="kalua-input" id="` + escAttr(id) + `" name="` + name + `" value="` + value + `"` + attrs + enabled + `>
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
	}
	return `<div class="kalua-control">Unknown control: ` + escText(ctrlType) + `</div>`
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
