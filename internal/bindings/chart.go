// Chart.js control support (see kforms_enhancements.md §3).
//
// k.ctrl.chart renders a <canvas> inside a .kalua-chart-container whose
// data-k-chart-config attribute carries the full Chart.js config JSON (type,
// data, options). The browser (app.js) instantiates and manages the Chart
// instance keyed by selector. The k.chart.* operations in this file push
// chart_update / chart_options / chart_resize messages through the session
// outbox and resume coroutines suspended by k.chart.get_image.
package bindings

import (
	"encoding/json"
	"strconv"

	"github.com/yuin/gopher-lua"

	"kalua/internal/common"
)

// registerChartOps installs the k.chart.* Chart.js data operations. Called
// from registerControls so the operations share the same API namespace.
func registerChartOps(e *Env) {
	// k.chart.set_data(form, name, {labels, datasets}) - bulk replace all data
	e.register("chart.set_data", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)
		opts := L.OptTable(3, L.NewTable())

		ctrl := chartControl(L, formName, name)
		if ctrl == nil {
			return 0
		}

		if labels := opts.RawGetString("labels"); labels != lua.LNil {
			ctrl.RawSetString("labels", labels)
		}
		if datasets := opts.RawGetString("datasets"); datasets != lua.LNil {
			ctrl.RawSetString("datasets", datasets)
		}
		chartUpdate(e, formName, name, ctrl)
		return 0
	})

	// k.chart.add_dataset(form, name, dataset) - append a dataset
	e.register("chart.add_dataset", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)
		dataset := L.CheckTable(3)

		ctrl := chartControl(L, formName, name)
		if ctrl == nil {
			return 0
		}

		ds := ctrl.RawGetString("datasets")
		if ds == lua.LNil {
			ds = L.NewTable()
			ctrl.RawSetString("datasets", ds)
		}
		if dsTbl, ok := ds.(*lua.LTable); ok {
			dsTbl.RawSetInt(dsTbl.Len()+1, dataset)
		}
		chartUpdate(e, formName, name, ctrl)
		return 0
	})

	// k.chart.remove_dataset(form, name, index) - remove dataset by 1-based index
	e.register("chart.remove_dataset", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)
		index := L.CheckInt(3)

		ctrl := chartControl(L, formName, name)
		if ctrl == nil {
			return 0
		}

		if dsTbl, ok := ctrl.RawGetString("datasets").(*lua.LTable); ok {
			dsTbl.RawSetInt(index, lua.LNil)
		}
		chartUpdate(e, formName, name, ctrl)
		return 0
	})

	// k.chart.update_dataset(form, name, index, dataset) - replace a dataset
	e.register("chart.update_dataset", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)
		index := L.CheckInt(3)
		dataset := L.CheckTable(4)

		ctrl := chartControl(L, formName, name)
		if ctrl == nil {
			return 0
		}

		if dsTbl, ok := ctrl.RawGetString("datasets").(*lua.LTable); ok {
			dsTbl.RawSetInt(index, dataset)
		}
		chartUpdate(e, formName, name, ctrl)
		return 0
	})

	// k.chart.set_labels(form, name, labels) - replace X-axis labels
	e.register("chart.set_labels", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)
		labels := L.CheckTable(3)

		ctrl := chartControl(L, formName, name)
		if ctrl == nil {
			return 0
		}

		ctrl.RawSetString("labels", labels)
		chartUpdate(e, formName, name, ctrl)
		return 0
	})

	// k.chart.set_options(form, name, options) - update Chart.js options
	e.register("chart.set_options", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)
		options := L.OptTable(3, L.NewTable())

		ctrl := chartControl(L, formName, name)
		if ctrl == nil {
			return 0
		}

		ctrl.RawSetString("options", options)
		sendOutbox(e, common.OutboxMsg{
			Type:     "chart_options",
			Form:     formName,
			Ctrl:     name,
			Selector: "#c:" + formName + ":" + name,
			Data:     luaTableToJSON(options),
		})
		return 0
	})

	// k.chart.resize(form, name, width, height) - resize the canvas
	e.register("chart.resize", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)
		width := L.CheckInt(3)
		height := L.CheckInt(4)

		ctrl := chartControl(L, formName, name)
		if ctrl == nil {
			return 0
		}

		ctrl.RawSetString("width", lua.LNumber(width))
		ctrl.RawSetString("height", lua.LNumber(height))
		sendOutbox(e, common.OutboxMsg{
			Type:     "chart_resize",
			Form:     formName,
			Ctrl:     name,
			Selector: "#c:" + formName + ":" + name,
			Data:     `{"width":` + strconv.Itoa(width) + `,"height":` + strconv.Itoa(height) + `}`,
		})
		return 0
	})

	// k.chart.get_image(form, name) - base64 PNG data URL rendered by the
	// browser's canvas. Suspends the coroutine until chart_image_resp arrives.
	e.register("chart.get_image", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)

		ctrl := chartControl(L, formName, name)
		if ctrl == nil {
			L.Push(lua.LNil)
			return 1
		}

		if e.Sess == nil {
			L.RaiseError("chart.get_image: no session available")
			return 0
		}
		e.Sess.RequestChartGetImage(L, func() {}, formName, name)
		return L.Yield(lua.LNil)
	})
}

// chartControl returns the chart control table for (form, name), or nil when
// the control does not exist or is not a chart.
func chartControl(L *lua.LState, formName, name string) *lua.LTable {
	ctrl := getControl(L, formName, name)
	if ctrl == nil || ctrl.RawGetString("type").String() != "chart" {
		return nil
	}
	return ctrl
}

// chartUpdate pushes a chart_update message carrying the full {labels,
// datasets} data for the control.
func chartUpdate(e *Env, formName, name string, ctrl *lua.LTable) {
	sendOutbox(e, common.OutboxMsg{
		Type:     "chart_update",
		Form:     formName,
		Ctrl:     name,
		Selector: "#c:" + formName + ":" + name,
		Data:     chartDataJSON(ctrl),
	})
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

// emitChartDestroys pushes a chart_destroy message for every chart control on
// the given form so the browser can tear down its Chart.js instance (called
// from form close / return_to).
func emitChartDestroys(e *Env, L *lua.LState, formName string) {
	formTbl := L.GetGlobal(formName)
	if formTbl == lua.LNil {
		return
	}
	tbl, ok := formTbl.(*lua.LTable)
	if !ok {
		return
	}
	controlsTbl, ok := tbl.RawGetString("controls").(*lua.LTable)
	if !ok {
		return
	}

	var chartNames []string
	controlsTbl.ForEach(func(k, v lua.LValue) {
		if ctrl, ok := v.(*lua.LTable); ok && ctrl.RawGetString("type").String() == "chart" {
			chartNames = append(chartNames, k.String())
		}
	})
	for _, name := range chartNames {
		sendOutbox(e, common.OutboxMsg{
			Type:     "chart_destroy",
			Form:     formName,
			Ctrl:     name,
			Selector: "#c:" + formName + ":" + name,
		})
	}
}
