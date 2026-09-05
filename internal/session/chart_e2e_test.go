package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	glua "github.com/yuin/gopher-lua"
	"kalua/internal/bindings"
)

// TestRealChartDOM exercises the Chart.js control end-to-end inside a real
// session: k.ctrl.chart renders, chart_click is unpacked into the Kalipso
// handler signature chart_click(dataset_index, index, value), k.chart.set_data
// pushes a chart_update message, and k.chart.get_image suspends the handler
// coroutine until the browser answers chart_image_resp with a PNG data URL.
func TestRealChartDOM(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "app.lua")
	src := `
function main()
  k.form.new("f", {title = "t"})
  k.ctrl.chart("f", "c1", {
    type = "bar",
    width = 600,
    height = 300,
    labels = {"A", "B"},
    datasets = {{label = "S", data = {1, 2}}},
  })
  k.form.on("f", "c1", "chart_click", function(di, idx, val)
    k.chart.set_data("f", "c1", {
      labels = {"A", "B", "C"},
      datasets = {{label = "S", data = {1, 2, 3}}}
    })
    _img = k.chart.get_image("f", "c1")
    _di = di
    _idx = idx
    _val = val
    _done = true
  end)
  k.form.show("f")
end
`
	if err := os.WriteFile(script, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := New("t1", script, bindings.Options{}, tLogger{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	out := make(chan outboxWire, 32)
	go func() {
		for msg := range s.Outbox() {
			out <- outboxWire{Type: msg.Type, Selector: msg.Selector, Data: msg.Data, Form: msg.Form, Ctrl: msg.Ctrl}
			if msg.Type == "chart_get_image" {
				s.PostChartImageResp(msg.ID, "data:image/png;base64,AAAA")
			}
		}
	}()

	time.Sleep(150 * time.Millisecond)

	// Browser reports a click on dataset 1, point 2 with value 2 (JSON-decoded
	// numbers arrive as float64, matching the real WebSocket path).
	s.PostEventAny("f", "c1", "chart_click", map[string]interface{}{
		"dataset_index": float64(1), "index": float64(2), "value": 2.0,
	})

	// The chart_click handler calls k.chart.set_data which pushes chart_update.
	deadline := time.After(4 * time.Second)
	for {
		select {
		case w := <-out:
			if w.Type != "chart_update" {
				continue
			}
			if w.Ctrl != "c1" {
				t.Fatalf("chart_update ctrl = %q, want c1", w.Ctrl)
			}
			if w.Selector != "#c:f:c1" {
				t.Fatalf("chart_update selector = %q, want #c:f:c1", w.Selector)
			}
			if !strings.Contains(w.Data, `"labels"`) || !strings.Contains(w.Data, `"C"`) {
				t.Fatalf("chart_update data = %s, want labels with C", w.Data)
			}
			goto updated
		case <-deadline:
			t.Fatalf("timed out waiting for chart_update")
		}
	}
updated:

	// k.chart.get_image suspended the handler; the browser's PNG data URL
	// resumes it and the handler records the unpacked args.
	deadline2 := time.After(4 * time.Second)
	for {
		select {
		case <-deadline2:
			t.Fatalf("timed out waiting for chart get_image round trip")
		default:
		}
		if v := s.GetGlobal("_done"); v == glua.LTrue {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if img := s.GetGlobal("_img"); img.String() != "data:image/png;base64,AAAA" {
		t.Errorf("_img = %q, want data:image/png;base64,AAAA", img.String())
	}
	if di := s.GetGlobal("_di"); di != glua.LNumber(1) {
		t.Errorf("_di = %v, want 1", di)
	}
	if idx := s.GetGlobal("_idx"); idx != glua.LNumber(2) {
		t.Errorf("_idx = %v, want 2", idx)
	}
	if val := s.GetGlobal("_val"); val != glua.LNumber(2) {
		t.Errorf("_val = %v, want 2", val)
	}
}

// TestChartDataOps exercises the pure data-structure operations on a chart
// control through the session: add/remove/update dataset, set_labels, resize.
func TestChartDataOps(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "app.lua")
	src := `
function main()
  k.form.new("f", {title = "t"})
  k.ctrl.chart("f", "c1", {
    type = "line",
    labels = {"A", "B"},
    datasets = {{label = "S", data = {1, 2}}},
  })
  k.chart.add_dataset("f", "c1", {label = "S2", data = {3, 4}})
  k.chart.update_dataset("f", "c1", 1, {label = "S1", data = {10, 20}})
  k.chart.set_labels("f", "c1", {"A", "B", "C"})
  k.chart.remove_dataset("f", "c1", 2)
  k.chart.set_options("f", "c1", {scales = {y = {beginAtZero = true}}})
  k.chart.resize("f", "c1", 800, 400)
  k.form.show("f")
end
`
	if err := os.WriteFile(script, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := New("t1", script, bindings.Options{}, tLogger{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	out := make(chan outboxWire, 32)
	go func() {
		for msg := range s.Outbox() {
			if msg.Type == "chart_update" || msg.Type == "chart_options" || msg.Type == "chart_resize" {
				out <- outboxWire{Type: msg.Type, Selector: msg.Selector, Data: msg.Data, Form: msg.Form, Ctrl: msg.Ctrl}
			}
		}
	}()

	// Collect the ops until every expected message type has been seen.
	want := map[string]bool{"chart_update": false, "chart_options": false, "chart_resize": false}
	deadline := time.After(4 * time.Second)
	var lastUpdate string
	for !doneAll(want) {
		select {
		case w := <-out:
			if w.Type == "chart_options" {
				want["chart_options"] = true
				if !strings.Contains(w.Data, `"beginAtZero":true`) {
					t.Fatalf("chart_options data = %s", w.Data)
				}
			} else if w.Type == "chart_resize" {
				want["chart_resize"] = true
				if !strings.Contains(w.Data, `"width":800`) || !strings.Contains(w.Data, `"height":400`) {
					t.Fatalf("chart_resize data = %s", w.Data)
				}
			} else {
				want["chart_update"] = true
				lastUpdate = w.Data
			}
		case <-deadline:
			t.Fatalf("timed out; saw = %+v lastUpdate=%s", want, lastUpdate)
		}
	}

	// Final state: one dataset ("S1" with {10,20}) and labels A,B,C.
	if !strings.Contains(lastUpdate, `"label":"S1"`) || !strings.Contains(lastUpdate, "10") {
		t.Errorf("final chart_update = %s, want S1/10", lastUpdate)
	}
	if !strings.Contains(lastUpdate, `"B"`) || !strings.Contains(lastUpdate, `"C"`) {
		t.Errorf("final chart_update labels = %s", lastUpdate)
	}
	if strings.Contains(lastUpdate, `"S2"`) {
		t.Errorf("removed dataset S2 still present: %s", lastUpdate)
	}
}

func doneAll(want map[string]bool) bool {
	for _, seen := range want {
		if !seen {
			return false
		}
	}
	return true
}
