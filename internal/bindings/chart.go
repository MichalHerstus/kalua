//go:build !wasm

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
