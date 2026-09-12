package ai

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"kalua/internal/checker"
	"kalua/internal/host"
)

// LuaBlockRE matches ```lua ... ``` code blocks.
var LuaBlockRE = regexp.MustCompile("(?s)" + "```" + "(?:lua)?\\s*\n(.*?)" + "```")

// GenerateRequest holds the input to Generate.
type GenerateRequest struct {
	Request string // natural language description
	Script  string // existing script to edit (empty = new)
	// FullDoc controls whether BuildSystemPrompt includes the full API or the run-mode subset.
	FullDoc bool
	// History carries earlier user/assistant turns (oldest first) so a chat
	// conversation can refine a request across multiple generations. Each
	// ChatMessage must be Role "user" or "assistant"; the current Request is
	// appended as the final user turn.
	History []ChatMessage
}

// GenerateResult holds the output of Generate.
type GenerateResult struct {
	Script    string   // generated or edited Lua script
	Generated bool     // true if a new script was generated, false if edited
	Logs      []string // validation/fixup log entries
}

// Generate produces a KALUA Lua script from a natural language request.
// It runs the validation→fix loop (up to maxFixRetries) and returns
// the final script plus a log of what happened.
func Generate(ctx context.Context, cfg ProviderConfig, req GenerateRequest) (*GenerateResult, error) {
	client := NewClient(cfg)
	return generate(ctx, client, buildMessages(req), req)
}

// generate runs the single LLM generation then the validation→fix loop.
// messages are pre-assembled (system + user) so streaming and non-streaming
// share the same pipeline.
func generate(ctx context.Context, client *Client, messages []ChatMessage, req GenerateRequest) (*GenerateResult, error) {
	script, err := client.Completion(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("initial generation: %w", err)
	}
	// Models typically wrap output in a ```lua fence (the prompt asks for it);
	// strip it so the checker sees plain Lua, not markdown.
	script = extractLuaBlock(script)

	result := &GenerateResult{
		Script:    script,
		Generated: req.Script == "",
		Logs:      []string{},
	}
	result.Logs = append(result.Logs, "Generated script from natural language request.")

	for attempt := 1; attempt <= maxFixRetries; attempt++ {
		lintResult := lint(script)
		if len(lintResult.Errors) == 0 {
			result.Logs = append(result.Logs, fmt.Sprintf("Validation passed on attempt %d.", attempt))
			return result, nil
		}
		result.Logs = append(result.Logs, fmt.Sprintf("Validation errors on attempt %d: %s", attempt, strings.Join(lintResult.Errors, "; ")))
		if attempt == maxFixRetries {
			break
		}
		fixScript, fixErr := fix(client, script, lintResult.Errors)
		if fixErr != nil {
			result.Logs = append(result.Logs, fmt.Sprintf("Fix attempt %d failed: %v", attempt, fixErr))
			break
		}
		// fix() already unwraps any fence; assign the cleaned script.
		script = fixScript
		result.Script = fixScript
	}
	return result, nil
}

const maxFixRetries = 3

// StreamEvent is a single progress event emitted by GenerateStream.
type StreamEvent struct {
	Type   string   // "token" | "status" | "done"
	Text   string   // token delta (token) or status message (status)
	Script string   // final script (done)
	Ok     bool     // validation passed (done)
	Errors []string // validation errors (done)
	Logs   []string // pipeline log (done)
}

// GenerateStream is the streaming variant of Generate. It streams the first
// LLM response token-by-token via emit (Type "token"), reports pipeline
// milestones (Type "status"), and finishes with a single Type "done" event
// carrying the validated result. The fix-loop runs non-streaming.
func GenerateStream(ctx context.Context, cfg ProviderConfig, req GenerateRequest, emit func(StreamEvent)) (*GenerateResult, error) {
	client := NewClient(cfg)
	messages := buildMessages(req)

	var script strings.Builder
	stream, err := client.CompletionStream(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("initial generation: %w", err)
	}
	for tok := range stream {
		script.WriteString(tok)
		emit(StreamEvent{Type: "token", Text: tok})
	}
	// Strip a ```lua fence the model may have emitted around the code.
	scriptText := extractLuaBlock(script.String())

	result := &GenerateResult{
		Script:    scriptText,
		Generated: req.Script == "",
		Logs:      []string{"Generated script from natural language request."},
	}

	for attempt := 1; attempt <= maxFixRetries; attempt++ {
		lintResult := lint(scriptText)
		if len(lintResult.Errors) == 0 {
			result.Logs = append(result.Logs, fmt.Sprintf("Validation passed on attempt %d.", attempt))
			emit(StreamEvent{Type: "done", Script: scriptText, Ok: true, Logs: result.Logs})
			return result, nil
		}
		msg := fmt.Sprintf("Validation errors on attempt %d: %s", attempt, strings.Join(lintResult.Errors, "; "))
		result.Logs = append(result.Logs, msg)
		emit(StreamEvent{Type: "status", Text: msg})
		if attempt == maxFixRetries {
			break
		}
		emit(StreamEvent{Type: "status", Text: fmt.Sprintf("Fixing errors (attempt %d)...", attempt)})
		fixScript, fixErr := fix(client, scriptText, lintResult.Errors)
		if fixErr != nil {
			result.Logs = append(result.Logs, fmt.Sprintf("Fix attempt %d failed: %v", attempt, fixErr))
			emit(StreamEvent{Type: "status", Text: "Fix failed: " + fixErr.Error()})
			break
		}
		// fix() already unwraps any fence; assign the cleaned script.
		scriptText = fixScript
		result.Script = fixScript
	}
	emit(StreamEvent{Type: "done", Script: result.Script, Ok: false, Errors: lint(result.Script).Errors, Logs: result.Logs})
	return result, nil
}

func buildPrompt(req GenerateRequest) string {
	if req.Script != "" {
		return BuildUserPrompt(req.Request, req.Script)
	}
	return BuildUserPrompt(req.Request, "")
}

func fix(client *Client, script string, errors []string) (string, error) {
	errorText := strings.Join(errors, "\n")
	prompt := fmt.Sprintf("Fix the following KALUA Lua script errors:\n\n%s\n\nScript:\n```lua\n%s\n```\n\nReturn the corrected script in a ```lua ... ``` block.", errorText, script)
	messages := []ChatMessage{
		{Role: "system", Content: BuildSystemPrompt(false) + KaluaComponentPrompt()},
		{Role: "user", Content: prompt},
	}
	out, err := client.Completion(context.Background(), messages)
	if err != nil {
		return "", err
	}
	return extractLuaBlock(out), nil
}

func lint(script string) checker.Result {
	return checker.Check(script, "ai-generated")
}

// extractLuaBlock extracts Lua code from a ```lua ... ``` fence,
// returning the original text if no fence is found.
func extractLuaBlock(text string) string {
	m := LuaBlockRE.FindStringSubmatch(text)
	if len(m) < 2 {
		return text
	}
	return strings.TrimSpace(m[1])
}

// ValidateAndRun validates a script via checker and then executes it headless.
// Returns the exit code and any output.
func ValidateAndRun(script string, scriptPath string) (host.ExitCode, string, error) {
	res := checker.Check(script, scriptPath)
	if len(res.Errors) > 0 {
		return host.ExitError, strings.Join(res.Errors, "\n"), nil
	}
	if scriptPath == "" {
		return host.ExitError, "script path required for run", nil
	}
	cfg := host.RunConfig{ScriptPath: scriptPath}
	code := host.Run(cfg)
	return code, "", nil
}
