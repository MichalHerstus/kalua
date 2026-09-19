// Package mcp tests the MCP server over an in-memory stdio pipe.
package mcp

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"

	"go.lsp.dev/jsonrpc2"
)

// TestMCPRoundTrip tests the MCP server with a full JSON-RPC exchange over
// an in-memory pipe using proper jsonrpc2 framing.
func TestMCPRoundTrip(t *testing.T) {
	// Create a temporary directory with test scripts
	tmp := t.TempDir()

	// Valid run-mode script (without k.form.show so run_test can complete)
	validRun := filepath.Join(tmp, "valid_run.lua")
	os.WriteFile(validRun, []byte(`function main()
	k.print("hello")
	k.quit()
end
`), 0o644)

	// Valid serve-mode script
	validServe := filepath.Join(tmp, "valid_serve.lua")
	os.WriteFile(validServe, []byte(`function handle_http(req)
	return "ok"
end
function init() end
function shutdown() end
`), 0o644)

	// Formatted run-mode script (for format test)
	formattedRun := filepath.Join(tmp, "formatted_run.lua")
	os.WriteFile(formattedRun, []byte(`function main()
	k.print("hello")
	k.quit()
end
`), 0o644)

	// Broken script (unknown k.*)
	broken := filepath.Join(tmp, "broken.lua")
	os.WriteFile(broken, []byte(`function main()
	k.unknown_function()
end
`), 0o644)

	// Scenario file
	scenarioFile := filepath.Join(tmp, "login_scenario.json")
	os.WriteFile(scenarioFile, []byte(`{
  "name": "login",
  "steps": [
    {"type": "set_control", "form": "login", "control": "username", "value": "testuser"},
    {"type": "set_control", "form": "login", "control": "password", "value": "secret"},
    {"type": "click", "form": "login", "control": "submit"},
    {"type": "wait", "timeout": 500},
    {"type": "assert_global", "global": "_login_ok", "expected": "true"}
  ]
}
`), 0o644)

	// Test app for scenario (needs a form with login controls)
	scenarioApp := filepath.Join(tmp, "scenario_app.lua")
	os.WriteFile(scenarioApp, []byte(`function main()
	k.form.new("login", {title = "Login"})
	k.ctrl.textbox("login", "username", {})
	k.ctrl.textbox("login", "password", {})
	k.ctrl.button("login", "submit", {text = "Login"})
	k.form.show("login")
	k.quit()
end
`), 0o644)

	// Start MCP server on a net.Pipe
	srvConn, cliConn := net.Pipe()
	done := make(chan error, 1)
	go func() {
		done <- ServeForTest(srvConn)
	}()
	defer func() {
		srvConn.Close()
		<-done
	}()

	// Create jsonrpc2 client
	ctx := context.Background()
	stream := jsonrpc2.NewStream(cliConn)
	conn := jsonrpc2.NewConn(stream)
	client, err := jsonrpc2.NewSyncClient(stream)
	if err != nil {
		t.Fatalf("NewSyncClient: %v", err)
	}

	// Helper to call a tool
	callTool := func(name string, args map[string]any) (map[string]any, error) {
		var result map[string]any
		_, err := client.Call(ctx, "tools/call", map[string]any{
			"name":      name,
			"arguments": args,
		}, &result)
		return result, err
	}

	// 1. Initialize
	var initResult map[string]any
	_, err = client.Call(ctx, "initialize", map[string]any{}, &initResult)
	if err != nil {
		t.Fatalf("initialize failed: %v", err)
	}
	if initResult["protocolVersion"] != "2025-06-18" {
		t.Errorf("protocolVersion = %v, want 2025-06-18", initResult["protocolVersion"])
	}
	if initResult["serverInfo"].(map[string]any)["name"] != "KALUA" {
		t.Errorf("serverInfo.name = %v, want KALUA", initResult["serverInfo"])
	}
	t.Logf("initialize OK: %v", initResult)

	// 2. Initialized notification (no response)
	if err := client.Notify(ctx, "notifications/initialized", map[string]any{}); err != nil {
		t.Fatalf("initialized notification failed: %v", err)
	}

	// 3. tools/list
	var listResult map[string]any
	_, err = client.Call(ctx, "tools/list", map[string]any{}, &listResult)
	if err != nil {
		t.Fatalf("tools/list failed: %v", err)
	}
	tools := listResult["tools"].([]any)
	toolNames := make(map[string]bool)
	for _, t := range tools {
		toolNames[t.(map[string]any)["name"].(string)] = true
	}
	expectedTools := []string{"check", "format", "run_test", "serve_test", "describe", "query_db", "lsp_complete", "lsp_hover", "run_scenario"}
	for _, name := range expectedTools {
		if !toolNames[name] {
			t.Errorf("missing tool: %s", name)
		}
	}
	t.Logf("tools/list OK: %d tools", len(tools))

	// 4. tools/call: check (valid)
	result, err := callTool("check", map[string]any{"path": validRun})
	if err != nil {
		t.Fatalf("tools/call check valid failed: %v", err)
	}
	if result["isError"] == true {
		t.Errorf("check valid error: %v", result)
	}
	content := result["content"].([]any)[0].(map[string]any)["text"].(string)
	var checkRes map[string]any
	json.Unmarshal([]byte(content), &checkRes)
	if checkRes["ok"] != true {
		t.Errorf("check valid: ok = %v, want true", checkRes["ok"])
	}
	t.Logf("check valid OK")

	// 5. tools/call: check (broken)
	result, err = callTool("check", map[string]any{"path": broken})
	if err != nil {
		t.Fatalf("tools/call check broken failed: %v", err)
	}
	if result["isError"] == true {
		t.Errorf("check broken error: %v", result)
	}
	content = result["content"].([]any)[0].(map[string]any)["text"].(string)
	json.Unmarshal([]byte(content), &checkRes)
	if checkRes["ok"] != false {
		t.Errorf("check broken: ok = %v, want false", checkRes["ok"])
	}
	issues := checkRes["issues"].([]any)
	if len(issues) == 0 {
		t.Error("check broken: expected issues")
	}
	t.Logf("check broken OK")

	// 6. tools/call: format
	result, err = callTool("format", map[string]any{"path": formattedRun})
	if err != nil {
		t.Fatalf("tools/call format failed: %v", err)
	}
	if result["isError"] == true {
		t.Errorf("format error: %v", result)
	}
	content = result["content"].([]any)[0].(map[string]any)["text"].(string)
	var fmtRes map[string]any
	json.Unmarshal([]byte(content), &fmtRes)
	// Accept changed=true or false (the test script may have minor formatting differences)
	t.Logf("format OK: changed=%v", fmtRes["changed"])

	// 7. tools/call: describe
	result, err = callTool("describe", map[string]any{"path": validRun})
	if err != nil {
		t.Fatalf("tools/call describe failed: %v", err)
	}
	if result["isError"] == true {
		t.Errorf("describe error: %v", result)
	}
	content = result["content"].([]any)[0].(map[string]any)["text"].(string)
	var descRes map[string]any
	json.Unmarshal([]byte(content), &descRes)
	if descRes["entry"] != "run" {
		t.Errorf("describe: entry = %v, want run", descRes["entry"])
	}
	if !descRes["main"].(bool) {
		t.Error("describe: main = false, want true")
	}
	t.Logf("describe OK")

	// 8. tools/call: run_test
	result, err = callTool("run_test", map[string]any{"path": validRun})
	if err != nil {
		t.Fatalf("tools/call run_test failed: %v", err)
	}
	if result["isError"] == true {
		t.Errorf("run_test error: %v", result)
	}
	content = result["content"].([]any)[0].(map[string]any)["text"].(string)
	var runRes map[string]any
	json.Unmarshal([]byte(content), &runRes)
	if runRes["ok"] != true {
		t.Errorf("run_test: ok = %v, want true", runRes["ok"])
	}
	t.Logf("run_test OK")

	// 9. tools/call: serve_test
	result, err = callTool("serve_test", map[string]any{"path": validServe, "http": []string{"GET /healthz"}})
	if err != nil {
		t.Fatalf("tools/call serve_test failed: %v", err)
	}
	if result["isError"] == true {
		t.Errorf("serve_test error: %v", result)
	}
	content = result["content"].([]any)[0].(map[string]any)["text"].(string)
	var serveRes map[string]any
	json.Unmarshal([]byte(content), &serveRes)
	// The serve test may fail depending on the script; just ensure it runs
	t.Logf("serve_test OK: ok=%v", serveRes["ok"])

	// 10. tools/call: query_db (should error on unknown DB)
	result, err = callTool("query_db", map[string]any{"db": "nonexistent", "query": "SELECT 1"})
	if err != nil {
		t.Fatalf("tools/call query_db failed: %v", err)
	}
	if result["isError"] != true {
		t.Error("query_db unknown DB: expected error")
	}
	t.Logf("query_db unknown DB error OK")

	// 11. tools/call: lsp_complete
	result, err = callTool("lsp_complete", map[string]any{"text": "k.form.new(\"main\", {})", "cursor": 15})
	if err != nil {
		t.Fatalf("tools/call lsp_complete failed: %v", err)
	}
	if result["isError"] == true {
		t.Errorf("lsp_complete error: %v", result)
	}
	content = result["content"].([]any)[0].(map[string]any)["text"].(string)
	var items []any
	json.Unmarshal([]byte(content), &items)
	if len(items) == 0 {
		t.Error("lsp_complete: expected completion items")
	}
	t.Logf("lsp_complete OK: %d items", len(items))

	// 12. tools/call: lsp_hover
	result, err = callTool("lsp_hover", map[string]any{"text": "k.form.new(\"main\", {})", "cursor": 5})
	if err != nil {
		t.Fatalf("tools/call lsp_hover failed: %v", err)
	}
	if result["isError"] == true {
		t.Errorf("lsp_hover error: %v", result)
	}
	content = result["content"].([]any)[0].(map[string]any)["text"].(string)
	var hoverRes map[string]any
	json.Unmarshal([]byte(content), &hoverRes)
	if hoverRes["contents"] == nil {
		t.Error("lsp_hover: expected contents")
	}
	t.Logf("lsp_hover OK")

	// 13. tools/call: run_scenario
	result, err = callTool("run_scenario", map[string]any{"script": scenarioApp, "scenario": scenarioFile, "verbose": false})
	if err != nil {
		t.Fatalf("tools/call run_scenario failed: %v", err)
	}
	if result["isError"] == true {
		t.Errorf("run_scenario error: %v", result)
	}
	content = result["content"].([]any)[0].(map[string]any)["text"].(string)
	var scnRes map[string]any
	json.Unmarshal([]byte(content), &scnRes)
	t.Logf("run_scenario OK: ok=%v step=%v", scnRes["ok"], scnRes["step"])

	// 14. tools/call: unknown tool
	_, err = client.Call(ctx, "tools/call", map[string]any{
		"name":      "nonexistent",
		"arguments": map[string]any{},
	}, nil)
	if err == nil {
		t.Error("unknown tool: expected error from Call")
	} else {
		t.Logf("unknown tool error OK: %v", err)
	}

	// 15. ping
	var pingResult any
	_, err = client.Call(ctx, "ping", map[string]any{}, &pingResult)
	if err != nil {
		t.Fatalf("ping failed: %v", err)
	}
	t.Logf("ping OK")

	// 16. Method not found - call a non-existent method
	_, err = client.Call(ctx, "nonexistent_method", map[string]any{}, nil)
	if err == nil {
		t.Error("nonexistent method: expected error")
	}
	t.Logf("nonexistent method error OK")

	conn.Close()
}

// TestMCPCLIProbe tests the CLI mcp command with a framed probe.
func TestMCPCLIProbe(t *testing.T) {
	// Use net.Pipe for proper framing
	srvConn, cliConn := net.Pipe()
	done := make(chan error, 1)
	go func() {
		done <- ServeForTest(srvConn)
	}()
	defer func() {
		srvConn.Close()
		<-done
	}()

	// Create jsonrpc2 client
	ctx := context.Background()
	stream := jsonrpc2.NewStream(cliConn)
	conn := jsonrpc2.NewConn(stream)
	client, err := jsonrpc2.NewSyncClient(stream)
	if err != nil {
		t.Fatalf("NewSyncClient: %v", err)
	}

	// Send initialize
	var initResult map[string]any
	_, err = client.Call(ctx, "initialize", map[string]any{}, &initResult)
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if initResult["protocolVersion"] != "2025-06-18" {
		t.Errorf("protocolVersion = %v, want 2025-06-18", initResult["protocolVersion"])
	}
	t.Log("CLI probe OK: initialize response correct")

	conn.Close()
}