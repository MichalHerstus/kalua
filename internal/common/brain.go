// Package common provides shared types and utilities used across KALUA packages to avoid import cycles.
// This file holds the M5 client-protocol brain: a pure, transport-agnostic
// router that translates session OutboxMsg commands into the compact "hands"
// command vocabulary understood by app.minimal.js. Keeping it in common makes
// it natively unit-testable while still available to the WASM build.
package common

// WasmCmd is one JavaScript "hands" command. It is JSON-marshalled by the
// caller (native tests or the WASM bridge) — fields are plain interface{}
// values so both sides see identical shapes.
type WasmCmd = map[string]interface{}

// RouteOutbox translates a session outbox message into one or more hand
// commands. Most messages map 1:1; form renders additionally request a
// component scan on the fresh scope.
func RouteOutbox(msg OutboxMsg) []WasmCmd {
	var cmds []WasmCmd
	switch msg.Type {
	case "init":
		cmds = append(cmds, map[string]interface{}{
			"cmd":  "init",
			"form": msg.Form,
		})

	case "render_form":
		if msg.Modal {
			cmds = append(cmds, map[string]interface{}{
				"cmd":       "modal_open",
				"name":      msg.Form,
				"html":      msg.HTML,
				"gap_x":     msg.GapX,
				"gap_y":     msg.GapY,
				"grid":      msg.Grid,
				"grid_mode": msg.GridMode,
				"grid_pk":   msg.GridPK,
				"grid_row":  msg.GridRow,
			})
		} else {
			cmds = append(cmds, map[string]interface{}{
				"cmd":  "stage",
				"html": msg.HTML,
			})
		}
		// After any render, ask the hands to (re)scan the live scope for
		// Tabulator/Chart/looper/flatpickr containers.
		scope := "#stage"
		if msg.Modal {
			scope = "#modals"
		}
		cmds = append(cmds, map[string]interface{}{
			"cmd":   "component_scan",
			"scope": scope,
		})

	case "update_control":
		cmds = append(cmds, map[string]interface{}{
			"cmd":      "update_control",
			"selector": msg.Selector,
			"html":     msg.HTML,
		})
		cmds = append(cmds, map[string]interface{}{
			"cmd":   "component_scan",
			"scope": "#stage",
		})

	case "close_form":
		if msg.Modal {
			cmds = append(cmds, map[string]interface{}{
				"cmd":  "modal_close",
				"name": msg.Form,
			})
		} else {
			cmds = append(cmds, map[string]interface{}{
				"cmd":     "form_close",
				"name":    msg.Form,
				"top":     msg.Modal,
				"destroy": true,
			})
		}

	case "close_msgbox":
		cmds = append(cmds, map[string]interface{}{
			"cmd": "msgbox_close",
			"id":  msg.ID,
		})

	case "close_popup":
		cmds = append(cmds, map[string]interface{}{
			"cmd": "popup_close",
			"id":  msg.ID,
		})

	case "status":
		cmds = append(cmds, map[string]interface{}{
			"cmd":  "status",
			"text": msg.Text,
		})

	case "status_close":
		cmds = append(cmds, map[string]interface{}{
			"cmd": "status_close",
		})

	case "focus":
		cmds = append(cmds, map[string]interface{}{
			"cmd":  "focus",
			"form": msg.Form,
			"ctrl": msg.Ctrl,
		})

	case "select_text":
		cmds = append(cmds, map[string]interface{}{
			"cmd":  "select_text",
			"form": msg.Form,
			"ctrl": msg.Ctrl,
		})

	case "select_range":
		cmds = append(cmds, map[string]interface{}{
			"cmd":  "select_range",
			"form": msg.Form,
			"ctrl": msg.Ctrl,
			"data": msg.Data,
		})

	case "get_selection":
		cmds = append(cmds, map[string]interface{}{
			"cmd":  "get_selection",
			"id":   msg.ID,
			"form": msg.Form,
			"ctrl": msg.Ctrl,
		})

	case "quit":
		cmds = append(cmds, map[string]interface{}{
			"cmd": "quit",
		})

	case "reload":
		cmds = append(cmds, map[string]interface{}{
			"cmd": "reload",
		})

	case "error":
		cmds = append(cmds, map[string]interface{}{
			"cmd":   "error",
			"msg":   msg.Msg,
			"stack": msg.Stack,
		})

	case "bell":
		cmds = append(cmds, map[string]interface{}{
			"cmd": "bell",
		})

	// ---- Browser-API requests (hands answers over the inbox) ----
	case "clipboard_set":
		cmds = append(cmds, map[string]interface{}{
			"cmd":  "clipboard_set",
			"text": msg.Text,
		})

	case "clipboard_get":
		cmds = append(cmds, map[string]interface{}{
			"cmd": "clipboard_get",
			"id":  msg.ID,
		})

	case "pick_file":
		cmds = append(cmds, map[string]interface{}{
			"cmd":      "pick_file",
			"id":       msg.ID,
			"accept":   msg.Accept,
			"multiple": msg.Multiple,
		})

	case "pick_file_save":
		cmds = append(cmds, map[string]interface{}{
			"cmd":  "pick_file_save",
			"id":   msg.ID,
			"data": msg.Data,
		})

	// ---- Component commands (Tabulator / Chart / Looper / Grid) ----
	case "tabulator_update":
		cmds = append(cmds, componentCmd("tabulator", "update", msg.Selector, msg.Data, "", msg.Form, msg.Ctrl))
	case "tabulator_remote_data":
		cmds = append(cmds, componentCmd("tabulator", "remote_data", msg.Selector, msg.Data, "", msg.Form, msg.Ctrl))
	case "tabulator_refresh":
		cmds = append(cmds, componentCmd("tabulator", "refresh", msg.Selector, "", "", msg.Form, msg.Ctrl))
	case "tabulator_destroy":
		cmds = append(cmds, componentCmd("tabulator", "destroy", msg.Selector, "", "", msg.Form, msg.Ctrl))
	case "tabulator_get_data":
		cmds = append(cmds, componentCmd("tabulator", "get_data", msg.Selector, "", msg.ID, msg.Form, msg.Ctrl))
	case "tabulator_get_selection":
		cmds = append(cmds, componentCmd("tabulator", "get_selection", msg.Selector, "", msg.ID, msg.Form, msg.Ctrl))
	case "looper_db_batch":
		cmds = append(cmds, componentCmd("looper", "batch", msg.Selector, msg.Data, "", msg.Form, msg.Ctrl))
	case "looper_refresh":
		cmds = append(cmds, componentCmd("looper", "refresh", msg.Selector, "", "", msg.Form, msg.Ctrl))
	case "chart_update":
		cmds = append(cmds, componentCmd("chart", "update", msg.Selector, msg.Data, "", msg.Form, msg.Ctrl))
	case "chart_options":
		cmds = append(cmds, componentCmd("chart", "options", msg.Selector, msg.Data, "", msg.Form, msg.Ctrl))
	case "chart_resize":
		cmds = append(cmds, componentCmd("chart", "resize", msg.Selector, msg.Data, "", msg.Form, msg.Ctrl))
	case "chart_destroy":
		cmds = append(cmds, componentCmd("chart", "destroy", msg.Selector, "", "", msg.Form, msg.Ctrl))
	case "chart_get_image":
		cmds = append(cmds, componentCmd("chart", "get_image", msg.Selector, "", msg.ID, msg.Form, msg.Ctrl))

	case "msgbox":
		cmds = append(cmds, map[string]interface{}{
			"cmd":  "msgbox",
			"id":   msg.ID,
			"kind": msg.Kind,
			"html": msg.HTML,
		})

	case "popup":
		cmds = append(cmds, map[string]interface{}{
			"cmd":  "popup",
			"id":   msg.ID,
			"html": msg.HTML,
		})

	default:
		// Unknown types are dropped so an unexpected server message never
		// reaches the page.
	}

	return cmds
}

// componentCmd builds a component hand command.
func componentCmd(kind, op, selector, data, id, form, ctrl string) WasmCmd {
	cmd := map[string]interface{}{
		"cmd":      "component",
		"kind":     kind,
		"op":       op,
		"selector": selector,
		"form":     form,
		"ctrl":     ctrl,
	}
	if data != "" {
		cmd["data"] = data
	}
	if id != "" {
		cmd["id"] = id
	}
	return cmd
}
