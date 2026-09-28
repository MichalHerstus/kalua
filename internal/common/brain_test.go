// Package common provides shared types and utilities used across KALUA packages.
// Tests for the M5 client-protocol brain (RouteOutbox).
package common

import (
	"testing"
)

// cmdString reads a string field out of a command map ("" when absent/mismatched).
func cmdString(cmd map[string]interface{}, key string) string {
	v := cmd[key]
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// cmdBool reads a bool field out of a command map.
func cmdBool(cmd map[string]interface{}, key string) bool {
	v := cmd[key]
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}

// TestRouteRenderFormNonModal verifies render_form (non-modal) maps to a stage
// command followed by a scope component scan.
func TestRouteRenderFormNonModal(t *testing.T) {
	cmds := RouteOutbox(OutboxMsg{
		Type: "render_form",
		Form: "main",
		HTML: `<div id="f:main">hi</div>`,
	})
	if len(cmds) != 2 {
		t.Fatalf("expected stage + scan, got %d cmds", len(cmds))
	}
	if cmdString(cmds[0], "cmd") != "stage" {
		t.Fatalf("first cmd = %q, want stage", cmdString(cmds[0], "cmd"))
	}
	if cmdString(cmds[0], "html") != `<div id="f:main">hi</div>` {
		t.Fatalf("html did not pass through: %q", cmdString(cmds[0], "html"))
	}
	if cmdString(cmds[1], "cmd") != "component_scan" {
		t.Fatalf("second cmd = %q, want component_scan", cmdString(cmds[1], "cmd"))
	}
	if cmdString(cmds[1], "scope") != "#stage" {
		t.Fatalf("non-modal scans %q, want #stage", cmdString(cmds[1], "scope"))
	}
}

// TestRouteRenderFormModal verifies modal renders emit modal_open + scan #modals.
func TestRouteRenderFormModal(t *testing.T) {
	cmds := RouteOutbox(OutboxMsg{Type: "render_form", Form: "dlg", HTML: "<p>x</p>", Modal: true, GapX: 5, GapY: 5})
	if len(cmds) != 2 {
		t.Fatalf("modal + scan, got %d", len(cmds))
	}
	if cmdString(cmds[0], "cmd") != "modal_open" || cmdString(cmds[0], "name") != "dlg" {
		t.Fatalf("modal_open not emitted: %q", cmdString(cmds[0], "cmd"))
	}
	if cmdString(cmds[1], "scope") != "#modals" {
		t.Fatalf("modal scans %q, want #modals", cmdString(cmds[1], "scope"))
	}
}

// TestRouteComponentCommands verifies tabulator/chart/looper outbox messages map
// to component commands with the right kind/op/selector.
func TestRouteComponentCommands(t *testing.T) {
	tabulator := RouteOutbox(OutboxMsg{
		Type:     "tabulator_remote_data",
		Form:     "f",
		Ctrl:     "t",
		Selector: "#c:f:t",
		Data:     `{"data":[{"a":1}]}`,
	})
	if len(tabulator) != 1 {
		t.Fatalf("single component cmd, got %d", len(tabulator))
	}
	c := tabulator[0]
	if cmdString(c, "cmd") != "component" || cmdString(c, "kind") != "tabulator" ||
		cmdString(c, "op") != "remote_data" || cmdString(c, "selector") != "#c:f:t" {
		t.Fatalf("bad tabulator cmd: %v", c)
	}

	chart := RouteOutbox(OutboxMsg{Type: "chart_update", Selector: "#c:f:c", Data: "{}"})
	if cmdString(chart[0], "kind") != "chart" {
		t.Fatalf("chart kind = %q", cmdString(chart[0], "kind"))
	}
	looper := RouteOutbox(OutboxMsg{Type: "looper_db_batch", Selector: "#c:f:l", Data: "{}"})
	if cmdString(looper[0], "kind") != "looper" {
		t.Fatalf("looper kind = %q", cmdString(looper[0], "kind"))
	}
}

// TestRouteComponentRoundTrip verifies request commands carry the resp id so the
// hands can answer browser round-trips (data/selection/image).
func TestRouteComponentRoundTrip(t *testing.T) {
	cmds := RouteOutbox(OutboxMsg{Type: "chart_get_image", ID: "req-1", Selector: "#c:f:c", Form: "f", Ctrl: "c"})
	if cmdString(cmds[0], "id") != "req-1" || cmdString(cmds[0], "op") != "get_image" {
		t.Fatalf("round-trip cmd: %v", cmds[0])
	}
	tabulator := RouteOutbox(OutboxMsg{Type: "tabulator_get_data", ID: "r2", Selector: "#c:f:t"})
	if cmdString(tabulator[0], "id") != "r2" {
		t.Fatalf("tabulator get_data id = %q", cmdString(tabulator[0], "id"))
	}
}

// TestRouteMisc verifies status/error/quit/browser-API requests map cleanly.
func TestRouteMisc(t *testing.T) {
	status := RouteOutbox(OutboxMsg{Type: "status", Text: "loading..."})
	if cmdString(status[0], "text") != "loading..." {
		t.Fatalf("status text = %q", cmdString(status[0], "text"))
	}
	err := RouteOutbox(OutboxMsg{Type: "error", Msg: "boom", Stack: "at x"})
	if cmdString(err[0], "cmd") != "error" || cmdString(err[0], "msg") != "boom" {
		t.Fatalf("error cmd: %v", err[0])
	}
	quit := RouteOutbox(OutboxMsg{Type: "quit"})
	if cmdString(quit[0], "cmd") != "quit" {
		t.Fatalf("quit cmd = %q", cmdString(quit[0], "cmd"))
	}
	clip := RouteOutbox(OutboxMsg{Type: "clipboard_get", ID: "c1"})
	if cmdString(clip[0], "cmd") != "clipboard_get" || cmdString(clip[0], "id") != "c1" {
		t.Fatalf("clipboard_get: %v", clip[0])
	}
	pick := RouteOutbox(OutboxMsg{Type: "pick_file", ID: "p1", Accept: ".lua", Multiple: true})
	if cmdString(pick[0], "cmd") != "pick_file" || !cmdBool(pick[0], "multiple") {
		t.Fatalf("pick_file: %v", pick[0])
	}
}

// TestRouteUpdateControl verifies update_control maps to an update + scan.
func TestRouteUpdateControl(t *testing.T) {
	cmds := RouteOutbox(OutboxMsg{Type: "update_control", Selector: "#c:f:tb", HTML: "<input/>"})
	if len(cmds) != 2 {
		t.Fatalf("update + scan, got %d", len(cmds))
	}
	if cmdString(cmds[0], "selector") != "#c:f:tb" || cmdString(cmds[1], "cmd") != "component_scan" {
		t.Fatalf("update_control cmds: %v", cmds)
	}
}

// TestRouteUnknownDropped verifies unknown message types are silently dropped.
func TestRouteUnknownDropped(t *testing.T) {
	if cmds := RouteOutbox(OutboxMsg{Type: "nonsense_message", Form: "x"}); len(cmds) != 0 {
		t.Fatalf("unknown dropped, got %d", len(cmds))
	}
}

// TestRouteCloseForm verifies modal vs non-modal close differ.
func TestRouteCloseForm(t *testing.T) {
	modal := RouteOutbox(OutboxMsg{Type: "close_form", Form: "dlg", Modal: true})
	if cmdString(modal[0], "cmd") != "modal_close" {
		t.Fatalf("modal close = %q", cmdString(modal[0], "cmd"))
	}
	plain := RouteOutbox(OutboxMsg{Type: "close_form", Form: "main"})
	if cmdString(plain[0], "cmd") != "form_close" || !cmdBool(plain[0], "destroy") {
		t.Fatalf("form close: %v", plain[0])
	}
}
