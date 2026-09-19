// Package mcp implements a Model Context Protocol (MCP) server for KALUA.
// It speaks JSON-RPC 2.0 over stdio (Content-Length framing), identical to the
// LSP transport, exposing 9 tools that wrap the CLI's static checks and run-time
// probes so any MCP-capable agent can drive KALUA without shelling out.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"go.lsp.dev/jsonrpc2"

	"kalua/internal/checker"
	"kalua/internal/describe"
	"kalua/internal/format"
	"kalua/internal/host"
	"kalua/internal/lsp"
	"kalua/internal/scenario"
	"kalua/internal/server"
	"kalua/internal/bindings"
)

// mcpCodec is a pass-through codec: it routes jsonrpc2.RawMessage values
// verbatim (like the LSP codec) and uses encoding/json for structured types.
// MCP has no union-typed schema, so this is simpler than lspCodec.
type mcpCodec struct{}

func (mcpCodec) Marshal(v any) ([]byte, error) {
	switch m := v.(type) {
	case jsonrpc2.RawMessage:
		if m == nil {
			return []byte("null"), nil
		}
		return m, nil
	case *jsonrpc2.RawMessage:
		if m == nil || *m == nil {
			return []byte("null"), nil
		}
		return *m, nil
	}
	return json.Marshal(v)
}

func (mcpCodec) Unmarshal(data []byte, v any) error {
	if p, ok := v.(*jsonrpc2.RawMessage); ok {
		b := make(jsonrpc2.RawMessage, len(data))
		copy(b, data)
		*p = b
		return nil
	}
	return json.Unmarshal(data, v)
}

// Serve runs the MCP server over a stdio connection (stdin/stdout).
func Serve(l io.ReadWriteCloser) error {
	stream := jsonrpc2.NewStream(l)
	conn := jsonrpc2.NewConn(stream, jsonrpc2.WithCodec(mcpCodec{}))

	h := &mcpHandler{tools: buildToolRegistry()}
	// Wrap handler in a function that matches jsonrpc2.Handler signature
	handler := func(ctx context.Context, req *jsonrpc2.Request) (any, error) {
		return h.Handle(ctx, req)
	}
	conn.Go(context.Background(), handler)
	<-conn.Done()
	return nil
}

// mcpHandler handles the JSON-RPC methods for MCP.
type mcpHandler struct {
	tools map[string]mcpTool
}

type mcpTool struct {
	name        string
	description string
	inputSchema map[string]any
	call        func(ctx context.Context, args map[string]any) (string, error)
}

func (h *mcpHandler) Handle(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	method := req.Method()
	switch method {
	case "initialize":
		return h.handleInitialize(req.Params())
	case "notifications/initialized":
		return nil, nil // no-op, notification
	case "ping":
		return struct{}{}, nil
	case "tools/list":
		return h.handleToolsList()
	case "tools/call":
		return h.handleToolsCall(ctx, req.Params())
	default:
		return nil, jsonrpc2.ErrMethodNotFound
	}
}

func (h *mcpHandler) handleInitialize(params jsonrpc2.RawMessage) (map[string]any, error) {
	// Optional: read client protocolVersion
	return map[string]any{
		"protocolVersion": "2025-06-18",
		"capabilities": map[string]any{
			"tools": map[string]any{
				"listChanged": false,
			},
		},
		"serverInfo": map[string]any{
			"name":    "KALUA",
			"version": "dev",
		},
	}, nil
}

func (h *mcpHandler) handleToolsList() (map[string]any, error) {
	tools := make([]map[string]any, 0, len(h.tools))
	for _, t := range h.tools {
		tools = append(tools, map[string]any{
			"name":        t.name,
			"description": t.description,
			"inputSchema": t.inputSchema,
		})
	}
	return map[string]any{"tools": tools}, nil
}

func (h *mcpHandler) handleToolsCall(ctx context.Context, params jsonrpc2.RawMessage) (map[string]any, error) {
	var callParams struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments,omitempty"`
	}
	if err := json.Unmarshal(params, &callParams); err != nil {
		return nil, jsonrpc2.Errorf(jsonrpc2.InvalidParams, "invalid tools/call params: %v", err)
	}
	tool, ok := h.tools[callParams.Name]
	if !ok {
		return nil, jsonrpc2.Errorf(jsonrpc2.MethodNotFound, "unknown tool %s", callParams.Name)
	}
	text, err := tool.call(ctx, callParams.Arguments)
	if err != nil {
		return map[string]any{
			"content": []map[string]any{{"type": "text", "text": err.Error()}},
			"isError": true,
		}, nil
	}
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}},
		"isError": false,
	}, nil
}

// buildToolRegistry creates the 9 tool definitions and their call functions.
func buildToolRegistry() map[string]mcpTool {
	return map[string]mcpTool{
		"check": {
			name:        "check",
			description: "Static validation of a KALUA script (syntax, unknown k.*, main presence). Returns issues with line/col.",
			inputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Path to the .lua script to validate",
					},
				},
				"required": []string{"path"},
			},
			call: func(ctx context.Context, args map[string]any) (string, error) {
				path, ok := args["path"].(string)
				if !ok || path == "" {
					return "", fmt.Errorf("check: 'path' argument required (string)")
				}
				src, err := os.ReadFile(path)
				if err != nil {
					return "", fmt.Errorf("check: cannot read %s: %v", path, err)
				}
				res := checker.Check(string(src), path)
				type issueJSON struct {
					Message string `json:"message"`
					Line    int    `json:"line"`
					Col     int    `json:"col"`
				}
				var issues []issueJSON
				for _, iss := range res.Issues {
					issues = append(issues, issueJSON{Message: iss.Message, Line: iss.Line, Col: iss.Col})
				}
				out := map[string]any{
					"ok":     len(res.Errors) == 0,
					"issues": issues,
				}
				b, _ := json.Marshal(out)
				return string(b), nil
			},
		},
		"format": {
			name:        "format",
			description: "Format a KALUA script (gofmt-style). Returns changed flag, formatted source, and optional diff.",
			inputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Path to the .lua script to format",
					},
				},
				"required": []string{"path"},
			},
			call: func(ctx context.Context, args map[string]any) (string, error) {
				path, ok := args["path"].(string)
				if !ok || path == "" {
					return "", fmt.Errorf("format: 'path' argument required (string)")
				}
				src, err := os.ReadFile(path)
				if err != nil {
					return "", fmt.Errorf("format: cannot read %s: %v", path, err)
				}
				formatted, err := format.Format(src, path)
				if err != nil {
					return "", fmt.Errorf("format: %v", err)
				}
				before := string(src)
				after := string(formatted)
				changed := before != after
				diff := ""
				if changed {
					diff = format.Diff(path, before, after)
				}
				out := map[string]any{
					"changed":   changed,
					"formatted": after,
					"diff":      diff,
				}
				b, _ := json.Marshal(out)
				return string(b), nil
			},
		},
		"run_test": {
			name:        "run_test",
			description: "Headless test of a run-mode KALUA app (static check + execute main()). Returns exit code and output.",
			inputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Path to the .lua script to test",
					},
					"dbs": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "Named DB connections: NAME=DSN",
					},
					"args": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "ARGS table seeds: K=V",
					},
				},
				"required": []string{"path"},
			},
			call: func(ctx context.Context, args map[string]any) (string, error) {
				path, ok := args["path"].(string)
				if !ok || path == "" {
					return "", fmt.Errorf("run_test: 'path' argument required (string)")
				}
				dbs := []string{}
				if v, ok := args["dbs"].([]any); ok {
					for _, d := range v {
						if s, ok := d.(string); ok {
							dbs = append(dbs, s)
						}
					}
				}
				argVals := []string{}
				if v, ok := args["args"].([]any); ok {
					for _, a := range v {
						if s, ok := a.(string); ok {
							argVals = append(argVals, s)
						}
					}
				}
				cfg := host.RunConfig{
					ScriptPath: path,
					Args:       argVals,
					DBs:        dbs,
					Verbose:    false,
				}
				code := host.Run(cfg)
				out := map[string]any{
					"exit_code": int(code),
					"ok":        code == host.ExitOK,
				}
				b, _ := json.Marshal(out)
				return string(b), nil
			},
		},
		"serve_test": {
			name:        "serve_test",
			description: "Headless smoke test of a serve-mode KALUA app (init, handle_http, handle_ws, handle_tcp, shutdown). Returns structured SmokeResult.",
			inputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Path to the .lua script to test",
					},
					"http": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "HTTP probes: 'METHOD /path' (e.g. 'GET /healthz')",
					},
					"expect_status": map[string]any{
						"type":        "integer",
						"description": "Expected HTTP status for probes (default 200)",
					},
					"expect_json": map[string]any{
						"type":        "object",
						"description": "JSON key/value assertions on probe bodies",
					},
					"expect_contains": map[string]any{
						"type":        "string",
						"description": "Substring assertion on probe bodies",
					},
					"ws_echo": map[string]any{
						"type":        "string",
						"description": "Payload to send to handle_ws; any reply passes",
					},
					"tcp_echo": map[string]any{
						"type":        "string",
						"description": "Payload to send to handle_tcp; any reply passes",
					},
				},
				"required": []string{"path"},
			},
			call: func(ctx context.Context, args map[string]any) (string, error) {
				path, ok := args["path"].(string)
				if !ok || path == "" {
					return "", fmt.Errorf("serve_test: 'path' argument required (string)")
				}
				httpProbes := []string{}
				if v, ok := args["http"].([]any); ok {
					for _, p := range v {
						if s, ok := p.(string); ok {
							httpProbes = append(httpProbes, s)
						}
					}
				}
				expectStatus := 200
				if v, ok := args["expect_status"].(float64); ok {
					expectStatus = int(v)
				}
				var expectJSON map[string]any
				if v, ok := args["expect_json"].(map[string]any); ok {
					expectJSON = v
				}
				expectContain, _ := args["expect_contains"].(string)
				wsEcho, _ := args["ws_echo"].(string)
				tcpEcho, _ := args["tcp_echo"].(string)

				// Build SmokeProbes from http probes
				var probes []server.SmokeProbe
				for _, hp := range httpProbes {
					parts := strings.SplitN(hp, " ", 2)
					method := "GET"
					path := "/"
					if len(parts) >= 1 && parts[0] != "" {
						method = parts[0]
					}
					if len(parts) == 2 {
						path = parts[1]
					}
					probe := server.SmokeProbe{
						Method:      method,
						Path:        path,
						WantStatus:  expectStatus,
						WantContent: expectContain,
					}
					if len(expectJSON) > 0 {
						probe.WantJSON = expectJSON
					}
					probes = append(probes, probe)
				}
				opts := server.SmokeOptions{
					HTTPProbes: probes,
					WSSend:     wsEcho,
					TCPSend:    tcpEcho,
				}
				cfg := server.Config{
					ScriptPath: path,
					Mode:       "http",
				}
				if strings.Contains(strings.ToLower(path), "ws") {
					cfg.Mode = "http,ws"
				}
				if strings.Contains(strings.ToLower(path), "tcp") {
					cfg.Mode = "http,tcp"
				}
				result := server.SmokeTest(cfg, opts)
				b, _ := json.Marshal(result)
				return string(b), nil
			},
		},
		"describe": {
			name:        "describe",
			description: "Structural overview of a KALUA script from its AST: entry points, forms, k.* API usage.",
			inputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Path to the .lua script to describe",
					},
				},
				"required": []string{"path"},
			},
			call: func(ctx context.Context, args map[string]any) (string, error) {
				path, ok := args["path"].(string)
				if !ok || path == "" {
					return "", fmt.Errorf("describe: 'path' argument required (string)")
				}
				src, err := os.ReadFile(path)
				if err != nil {
					return "", fmt.Errorf("describe: cannot read %s: %v", path, err)
				}
				chk := checker.Check(string(src), path)
				res := describe.Result{
					OK:       len(chk.Errors) == 0,
					Entry:    checker.EntryMode(string(src), path),
					Lines:    len(strings.Split(string(src), "\n")),
					KUsage:   map[string]int{},
					Handlers: []string{},
				}
				if !res.OK {
					res.Error = strings.Join(chk.Errors[:1], "; ")
				} else {
					describe.Scan(string(src), path, &res)
				}
				b, _ := json.Marshal(res)
				return string(b), nil
			},
		},
		"query_db": {
			name:        "query_db",
			description: "Read-only preview query against a named DB handle (SELECT/WITH/PRAGMA/EXPLAIN/SHOW only, capped at 200 rows). Returns columns and rows.",
			inputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"db": map[string]any{
						"type":        "string",
						"description": "Named DB handle (registered via --db NAME=DSN)",
					},
					"query": map[string]any{
						"type":        "string",
						"description": "SQL query (read-only, leading comments ok)",
					},
					"limit": map[string]any{
						"type":        "integer",
						"description": "Max rows (default 200, max 200)",
					},
				},
				"required": []string{"db", "query"},
			},
			call: func(ctx context.Context, args map[string]any) (string, error) {
				dbName, ok := args["db"].(string)
				if !ok || dbName == "" {
					return "", fmt.Errorf("query_db: 'db' argument required (string)")
				}
				query, ok := args["query"].(string)
				if !ok || query == "" {
					return "", fmt.Errorf("query_db: 'query' argument required (string)")
				}
				limit := 200
				if v, ok := args["limit"].(float64); ok {
					limit = int(v)
				}
				if limit > 200 {
					limit = 200
				}
				cols, rows, err := bindings.QueryPreview(dbName, query, limit)
				if err != nil {
					return "", fmt.Errorf("query_db: %v", err)
				}
				out := map[string]any{
					"columns": cols,
					"rows":    rows,
				}
				b, _ := json.Marshal(out)
				return string(b), nil
			},
		},
		"lsp_complete": {
			name:        "lsp_complete",
			description: "LSP completion items at a cursor position (k.*, K.*, expression functions, globals).",
			inputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"text": map[string]any{
						"type":        "string",
						"description": "Full source text of the .lua file",
					},
					"cursor": map[string]any{
						"type":        "integer",
						"description": "Absolute byte offset cursor position",
					},
				},
				"required": []string{"text", "cursor"},
			},
			call: func(ctx context.Context, args map[string]any) (string, error) {
				text, ok := args["text"].(string)
				if !ok || text == "" {
					return "", fmt.Errorf("lsp_complete: 'text' argument required (string)")
				}
				cursor, ok := args["cursor"].(float64)
				if !ok {
					return "", fmt.Errorf("lsp_complete: 'cursor' argument required (integer)")
				}
				items := lsp.Completion(text, int(cursor))
				b, _ := json.Marshal(items)
				return string(b), nil
			},
		},
		"lsp_hover": {
			name:        "lsp_hover",
			description: "LSP hover markdown at a cursor position (k.*, K.*, expression functions).",
			inputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"text": map[string]any{
						"type":        "string",
						"description": "Full source text of the .lua file",
					},
					"cursor": map[string]any{
						"type":        "integer",
						"description": "Absolute byte offset cursor position",
					},
				},
				"required": []string{"text", "cursor"},
			},
			call: func(ctx context.Context, args map[string]any) (string, error) {
				text, ok := args["text"].(string)
				if !ok || text == "" {
					return "", fmt.Errorf("lsp_hover: 'text' argument required (string)")
				}
				cursor, ok := args["cursor"].(float64)
				if !ok {
					return "", fmt.Errorf("lsp_hover: 'cursor' argument required (integer)")
				}
				info, _, _ := lsp.Match(text, int(cursor))
				if info == nil {
					return "{}", nil
				}
				md := lsp.Markdown(*info)
				out := map[string]any{
					"contents": map[string]any{
						"kind":  "markdown",
						"value": md,
					},
				}
				b, _ := json.Marshal(out)
				return string(b), nil
			},
		},
		"run_scenario": {
			name:        "run_scenario",
			description: "Run a UI scenario test against a run-mode KALUA app. Returns pass/fail with step index and error.",
			inputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"script": map[string]any{
						"type":        "string",
						"description": "Path to the .lua script to test",
					},
					"scenario": map[string]any{
						"type":        "string",
						"description": "Path to the scenario JSON file",
					},
					"verbose": map[string]any{
						"type":        "boolean",
						"description": "Enable verbose logging",
					},
				},
				"required": []string{"script", "scenario"},
			},
			call: func(ctx context.Context, args map[string]any) (string, error) {
				script, ok := args["script"].(string)
				if !ok || script == "" {
					return "", fmt.Errorf("run_scenario: 'script' argument required (string)")
				}
				scenarioPath, ok := args["scenario"].(string)
				if !ok || scenarioPath == "" {
					return "", fmt.Errorf("run_scenario: 'scenario' argument required (string, file path)")
				}
				verbose := false
				if v, ok := args["verbose"].(bool); ok {
					verbose = v
				}
				// Load scenario from file
				scn, err := scenario.LoadScenario(scenarioPath)
				if err != nil {
					return "", fmt.Errorf("run_scenario: load scenario %s: %v", scenarioPath, err)
				}
				runner, err := scenario.NewRunner(script, verbose)
				if err != nil {
					return "", fmt.Errorf("run_scenario: %v", err)
				}
				result := runner.Run(scn)
				b, _ := json.Marshal(result)
				return string(b), nil
			},
		},
	}
}

// ServeForTest is exposed for the CLI probe test.
func ServeForTest(l io.ReadWriteCloser) error {
	return Serve(l)
}