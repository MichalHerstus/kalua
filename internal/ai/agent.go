package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"kalua/internal/common"
)

// ToolExecutor is invoked by AgentRun for each tool call the model requests.
// The name identifies the tool; args are the decoded arguments. It returns a
// result value that is fed back to the model as a role:"tool" message.
type ToolExecutor func(name string, args map[string]interface{}) (interface{}, error)

// AgentRun runs the agent tool-calling loop:
//   1. Stream an assistant reply via CompletionStream
//   2. Extract tool calls from the accumulated text
//   3. If any, execute each via executor, append the results as role:"tool",
//      re-prompt
//   4. Repeat until the model answers in prose or maxTurns is reached
//
// deltas receives each assistant token delta as it arrives.
// status receives status messages (e.g. "Calling get_customer(7)…").
// result receives the final assistant prose (or error).
//
// The agent is VM-free: it takes a ToolExecutor callback, so the Lua-side
// tool execution can be implemented by the caller (session package).
func AgentRun(ctx context.Context, client *Client, system string, messages []common.ChatMessage, maxTurns int, executor ToolExecutor,
	deltas func(string), status func(string)) (string, error) {

	// Build the initial conversation with tool schema instructions.
	toolInstruction := `You are an AI assistant with access to tools. When you need to call a tool,
emit exactly one tagged block per assistant turn:

  function {"name":"<tool_name>","arguments":{"<param>": <value>, ...}}

Do not wrap the block in markdown fences. After each tool result is returned,
continue your reasoning or give your final answer in plain text.`

	if system != "" {
		toolInstruction = system + "\n\n" + toolInstruction
	}

	conv := make([]common.ChatMessage, 0, len(messages)+2)
	conv = append(conv, common.ChatMessage{Role: "system", Content: toolInstruction})
	conv = append(conv, messages...)

	var turnErr error
	for turn := 0; turn < maxTurns; turn++ {
		status("Thinking…")

		stream, err := client.CompletionStream(ctx, conv)
		if err != nil {
			return "", fmt.Errorf("completion stream: %w", err)
		}

		var assistantText strings.Builder
		for delta := range stream {
			assistantText.WriteString(delta)
			if deltas != nil {
				deltas(delta)
			}
		}

		assistantReply := assistantText.String()
		if assistantReply == "" {
			// Empty assistant turn; stop rather than looping forever.
			return "", nil
		}

		// Append assistant turn.
		conv = append(conv, common.ChatMessage{Role: "assistant", Content: assistantReply})

		// Extract tool calls.
		calls, err := ExtractToolCalls(assistantReply)
		if err != nil {
			// Malformed tool call — report as tool error and let the model retry.
			status("Tool error")
			toolResult := fmt.Sprintf(`{"error":"%s"}`, err.Error())
			conv = append(conv, common.ChatMessage{Role: "tool", Name: "unknown", Content: toolResult})
			turnErr = err
			continue
		}

		if len(calls) == 0 {
			// No tool calls: final prose answer.
			return assistantReply, nil
		}

		// Execute each tool call.
		for _, call := range calls {
			status(fmt.Sprintf("Calling %s(%v)…", call.Name, formatArgs(call.Arguments)))
			result, execErr := executor(call.Name, call.Arguments)
			if execErr != nil {
				result = fmt.Sprintf(`{"error":"%s"}`, execErr.Error())
			}
			resultJSON, _ := jsonMarshal(result)
			conv = append(conv, common.ChatMessage{Role: "tool", Name: call.Name, Content: resultJSON})
		}
		// Loop again for the next assistant turn after tool results.
	}

	// Turn bound reached: return whatever the last assistant said (or error).
	if turnErr != nil {
		return "", turnErr
	}
	return "", fmt.Errorf("agent turn bound (%d) reached without a final answer", maxTurns)
}

// jsonMarshal best-effort serializes a tool result; falls back to a string.
func jsonMarshal(v interface{}) (string, error) {
	if s, ok := v.(string); ok {
		return s, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// formatArgs renders tool args for status messages (truncated for readability).
func formatArgs(args map[string]interface{}) string {
	if len(args) == 0 {
		return ""
	}
	parts := make([]string, 0, len(args))
	for k, v := range args {
		parts = append(parts, fmt.Sprintf("%s=%v", k, v))
	}
	return strings.Join(parts, ", ")
}
