package bindings

import (
	"encoding/json"
	"html"
	"strings"
	"testing"

	"github.com/yuin/gopher-lua"
)

// TestChartConfigJSONNormalization verifies the Chart.js config assembly:
// hbar/area aliases normalize to bar/line with the right options, convenience
// opts seed defaults, and a user `options` table overrides them.
func TestChartConfigJSONNormalization(t *testing.T) {
	L := setupTestState(t)

	build := func(chartType string) *lua.LTable {
		ctrl := L.NewTable()
		ctrl.RawSetString("chart_type", lua.LString(chartType))

		labels := L.NewTable()
		labels.RawSetInt(1, lua.LString("Jan"))
		labels.RawSetInt(2, lua.LString("Feb"))
		ctrl.RawSetString("labels", labels)

		d := L.NewTable()
		d.RawSetString("label", lua.LString("Revenue"))
		data := L.NewTable()
		data.RawSetInt(1, lua.LNumber(10))
		data.RawSetInt(2, lua.LNumber(20))
		d.RawSetString("data", data)
		d.RawSetString("fill", lua.LTrue)
		ds := L.NewTable()
		ds.RawSetInt(1, d)
		ctrl.RawSetString("datasets", ds)
		return ctrl
	}

	t.Run("hbar", func(t *testing.T) {
		got := chartConfigJSON(build("hbar"))
		var cfg map[string]interface{}
		if err := json.Unmarshal([]byte(got), &cfg); err != nil {
			t.Fatalf("chartConfigJSON not valid JSON: %v", err)
		}
		if cfg["type"] != "bar" {
			t.Errorf("hbar type = %v, want bar", cfg["type"])
		}
		opts := cfg["options"].(map[string]interface{})
		if opts["indexAxis"] != "y" {
			t.Errorf("hbar indexAxis = %v, want y", opts["indexAxis"])
		}
		if opts["responsive"] != true {
			t.Errorf("responsive default = %v, want true", opts["responsive"])
		}
		plugins := opts["plugins"].(map[string]interface{})
		legend := plugins["legend"].(map[string]interface{})
		if legend["display"] != true || legend["position"] != "top" {
			t.Errorf("legend defaults = %+v, want display true / top", legend)
		}
	})

	t.Run("area", func(t *testing.T) {
		got := chartConfigJSON(build("area"))
		var cfg map[string]interface{}
		if err := json.Unmarshal([]byte(got), &cfg); err != nil {
			t.Fatalf("chartConfigJSON not valid JSON: %v", err)
		}
		if cfg["type"] != "line" {
			t.Errorf("area type = %v, want line", cfg["type"])
		}
		opts := cfg["options"].(map[string]interface{})
		elements := opts["elements"].(map[string]interface{})
		line := elements["line"].(map[string]interface{})
		if line["fill"] != true {
			t.Errorf("area line fill = %v, want true", line["fill"])
		}
	})

	t.Run("user options override", func(t *testing.T) {
		ctrl := build("bar")
		ctrl.RawSetString("stacked", lua.LTrue)
		optsTbl := L.NewTable()
		scales := L.NewTable()
		y := L.NewTable()
		y.RawSetString("beginAtZero", lua.LTrue)
		scales.RawSetString("y", y)
		optsTbl.RawSetString("scales", scales)
		ctrl.RawSetString("options", optsTbl)

		got := chartConfigJSON(ctrl)
		var cfg map[string]interface{}
		if err := json.Unmarshal([]byte(got), &cfg); err != nil {
			t.Fatalf("chartConfigJSON not valid JSON: %v", err)
		}
		opts := cfg["options"].(map[string]interface{})
		scalesM := opts["scales"].(map[string]interface{})
		x := scalesM["x"].(map[string]interface{})
		if x["stacked"] != true {
			t.Errorf("stacked x = %v, want true (default from stacked opt)", x["stacked"])
		}
		yM := scalesM["y"].(map[string]interface{})
		if yM["beginAtZero"] != true {
			t.Errorf("user y.beginAtZero lost: %+v", yM)
		}
	})

	t.Run("data round trip", func(t *testing.T) {
		got := chartConfigJSON(build("line"))
		var cfg struct {
			Data struct {
				Labels   []string                 `json:"labels"`
				Datasets []map[string]interface{} `json:"datasets"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(got), &cfg); err != nil {
			t.Fatalf("chartConfigJSON not valid JSON: %v", err)
		}
		if len(cfg.Data.Labels) != 2 || cfg.Data.Labels[1] != "Feb" {
			t.Errorf("labels = %v", cfg.Data.Labels)
		}
		if len(cfg.Data.Datasets) != 1 || cfg.Data.Datasets[0]["fill"] != true {
			t.Errorf("datasets = %+v", cfg.Data.Datasets)
		}
	})
}

// TestRenderChart verifies the chart control HTML carries the config JSON and
// sizing attributes.
func TestRenderChart(t *testing.T) {
	L := setupTestState(t)
	ctrl := L.NewTable()
	ctrl.RawSetString("type", lua.LString("chart"))
	ctrl.RawSetString("chart_type", lua.LString("line"))
	ctrl.RawSetString("label", lua.LString("Sales"))
	ctrl.RawSetString("width", lua.LNumber(600))
	ctrl.RawSetString("height", lua.LNumber(300))
	labels := L.NewTable()
	labels.RawSetInt(1, lua.LString("Q1"))
	ctrl.RawSetString("labels", labels)

	got := renderChart(ctrl, "f", "c1", "c:f:c1", "")
	if !strings.Contains(got, `class="kalua-chart-canvas"`) {
		t.Errorf("renderChart missing canvas: %s", got)
	}
	if !strings.Contains(got, `data-k-chart-config="`) {
		t.Errorf("renderChart missing config attribute: %s", got)
	}
	if !strings.Contains(got, `data-k-form="f"`) || !strings.Contains(got, `data-k-ctrl="c1"`) {
		t.Errorf("renderChart missing form/ctrl attrs: %s", got)
	}
	if !strings.Contains(got, `id="c:f:c1"`) {
		t.Errorf("renderChart missing container id: %s", got)
	}
	if !strings.Contains(got, `width:600px;height:300px;`) {
		t.Errorf("renderChart missing size style: %s", got)
	}
	if !strings.Contains(got, `<label class="kalua-label">Sales</label>`) {
		t.Errorf("renderChart missing title label: %s", got)
	}

	// The embedded config must parse back as JSON after unescaping.
	start := strings.Index(got, `data-k-chart-config="`) + len(`data-k-chart-config="`)
	end := strings.Index(got[start:], `"`)
	rawCfg := got[start : start+end]
	cfg, err := decodeHTMLAttr(rawCfg)
	if err != nil {
		t.Fatalf("embedded config not valid JSON: %v", err)
	}
	var cfgMap map[string]interface{}
	if err := json.Unmarshal(cfg, &cfgMap); err != nil {
		t.Fatalf("embedded config decode: %v", err)
	}
	if cfgMap["type"] != "line" {
		t.Errorf("embedded type = %v, want line", cfgMap["type"])
	}
}

// decodeHTMLAttr un-escapes an HTML attribute value (the escAttr wrapper) back
// to the raw JSON string.
func decodeHTMLAttr(s string) ([]byte, error) {
	return []byte(html.UnescapeString(s)), nil
}
