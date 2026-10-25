// Package ai provides LLM client and agent tool-calling support for KALUA.
package ai

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"kalua/internal/common"
)

// ToolCallBlockRE matches `function {"name":...,"arguments":{...}}` blocks.
//
// Brace-flat by design: the args object must not contain nested objects (use
// scalar params), matching the documented §12 format.
var ToolCallBlockRE = regexp.MustCompile(`function\s*\{[^{}]*"name"\s*:\s*"[^"]*"[^{}]*"arguments"\s*:\s*\{[^{}]*\}\}`)

// ExtractToolCalls scans text for tagged tool-call blocks and returns them.
// Returns nil when none are found. Malformed JSON inside a block yields an
// error so the agent loop can report it to the model as a tool error result.
func ExtractToolCalls(text string) ([]common.ToolCall, error) {
	var calls []common.ToolCall
	matches := ToolCallBlockRE.FindAllString(text, -1)
	for _, m := range matches {
		// Strip the leading `function ` prefix, leaving just the JSON object.
		jsonPart := strings.TrimPrefix(m, "function ")
		var tc common.ToolCall
		if err := json.Unmarshal([]byte(jsonPart), &tc); err != nil {
			return nil, fmt.Errorf("parse tool call %q: %w", jsonPart, err)
		}
		calls = append(calls, tc)
	}
	return calls, nil
}

// SchemaJSON renders a common.ToolSchema as a compact JSON fragment suitable
// for inclusion in a system prompt. Parameters without an explicit type
// default to "string"; required defaults to false.
func SchemaJSON(schema common.ToolSchema) string {
	params := make([]map[string]interface{}, len(schema.Params))
	for i, p := range schema.Params {
		typ := p.Type
		if typ == "" {
			typ = "string"
		}
		params[i] = map[string]interface{}{
			"name":     p.Name,
			"type":     typ,
			"required": p.Required,
		}
		if p.Desc != "" {
			params[i]["desc"] = p.Desc
		}
	}
	obj := map[string]interface{}{
		"name":   schema.Name,
		"desc":   schema.Desc,
		"params": params,
	}
	b, _ := json.Marshal(obj)
	return string(b)
}

// ToolSchemasJSON renders a list of schemas as a JSON array.
func ToolSchemasJSON(schemas []common.ToolSchema) string {
	arr := make([]map[string]interface{}, len(schemas))
	for i, s := range schemas {
		params := make([]map[string]interface{}, len(s.Params))
		for j, p := range s.Params {
			typ := p.Type
			if typ == "" {
				typ = "string"
			}
			params[j] = map[string]interface{}{
				"name":     p.Name,
				"type":     typ,
				"required": p.Required,
			}
			if p.Desc != "" {
				params[j]["desc"] = p.Desc
			}
		}
		arr[i] = map[string]interface{}{
			"name":   s.Name,
			"desc":   s.Desc,
			"params": params,
		}
	}
	b, _ := json.Marshal(arr)
	return string(b)
}
