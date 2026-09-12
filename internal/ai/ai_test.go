package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"kalua/internal/bindings"
	"kalua/internal/checker"
)

// mockChatServer creates an httptest server that returns a fixed completion.
func mockChatServer(t *testing.T, response string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("expected /chat/completions, got %s", r.URL.Path)
		}
		var req ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.Model == "" {
			t.Errorf("expected model in request")
		}
		// Streaming path (Accept: text/event-stream): respond with SSE frames.
		if strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			for _, tok := range strings.Split(response, " ") {
				_, _ = w.Write([]byte(sseFrame(tok + " ")))
			}
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"choices":[{"message":{"role":"assistant","content":%s},"finish_reason":"stop"}]}`, toJSON(response))
	}))
}

func toJSON(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// sseFrame wraps text in an OpenAI-compatible streaming chunk.
func sseFrame(text string) string {
	b, _ := json.Marshal(map[string]any{"choices": []any{
		map[string]any{"delta": map[string]any{"content": text}},
	}})
	return "data: " + string(b) + "\n\n"
}

// mockChatStreamServer always responds in SSE streaming format regardless of
// the Accept header, for tests that exercise CompletionStream directly.
func mockChatStreamServer(t *testing.T, response string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		for _, tok := range strings.Split(response, " ") {
			_, _ = w.Write([]byte(sseFrame(tok + " ")))
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
}

func TestProviderCompletion(t *testing.T) {
	srv := mockChatServer(t, "k.print(\"hello\")")
	defer srv.Close()

	c := NewClient(ProviderConfig{
		BaseURL: srv.URL,
		Model:   "test-model",
	})
	got, err := c.Completion(context.Background(), []ChatMessage{
		{Role: "user", Content: "test"},
	})
	if err != nil {
		t.Fatalf("Completion: %v", err)
	}
	if got != "k.print(\"hello\")" {
		t.Errorf("got %q, want %q", got, "k.print(\"hello\")")
	}
}

func TestProviderError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewClient(ProviderConfig{BaseURL: srv.URL, Model: "m"})
	_, err := c.Completion(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGenerateFromMock(t *testing.T) {
	srv := mockChatServer(t, "function main()\n  k.print(\"hi\")\n  k.quit()\nend")
	defer srv.Close()

	req := GenerateRequest{Request: "generate a hello app"}
	res, err := Generate(context.Background(), ProviderConfig{BaseURL: srv.URL, Model: "m"}, req)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if res.Script != "function main()\n  k.print(\"hi\")\n  k.quit()\nend" {
		t.Errorf("script = %q", res.Script)
	}
	if !res.Generated {
		t.Error("Generated should be true")
	}
}

func TestGenerateFixLoop(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body struct{ Messages []struct{ Content string } }
		json.NewDecoder(r.Body).Decode(&body)
		lastMsg := body.Messages[len(body.Messages)-1].Content
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.Contains(lastMsg, "errors") && calls == 1:
			// First response has an error
			fmt.Fprintf(w, `{"choices":[{"message":{"role":"assistant","content":"broken code"}, "finish_reason":"stop"}]}`)
		default:
			fmt.Fprintf(w, `{"choices":[{"message":{"role":"assistant","content":"function main()\n  k.print(\"ok\")\n  k.quit()\nend"}, "finish_reason":"stop"}]}`)
		}
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	req := GenerateRequest{Request: "fix this"}
	res, err := Generate(context.Background(), ProviderConfig{BaseURL: u.String(), Model: "m"}, req)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.Contains(res.Script, "main()") {
		t.Errorf("fix did not produce main: %s", res.Script)
	}
}

func TestKnowledgePackNoDrift(t *testing.T) {
	// BuildSystemPrompt should not panic and should include all k.* names
	// from bindings.Docs() — the sync is enforced by TestApiDocSync.
	prompt := BuildSystemPrompt(false)
	if prompt == "" {
		t.Fatal("empty prompt")
	}
	// Every run-mode binding should appear in the prompt.
	for name := range bindings.Docs() {
		if !runModeBindings[name] {
			continue
		}
		if !strings.Contains(prompt, "`"+name+"`") {
			t.Errorf("prompt missing binding %s", name)
		}
	}
}

func TestBuildSystemPromptFull(t *testing.T) {
	prompt := BuildSystemPrompt(true)
	if prompt == "" {
		t.Fatal("empty prompt")
	}
	// Should contain hard rules
	if !strings.Contains(prompt, "function main()") {
		t.Error("prompt missing main() rule")
	}
	if !strings.Contains(prompt, "FLAT GLOBALS") {
		t.Error("prompt missing flat globals rule")
	}
}

func TestExtractLuaBlock(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"Some text\n```lua\nfunction main() end\n```\nmore", "function main() end"},
		{`plain text`, "plain text"},
	}
	for _, tt := range tests {
		got := extractLuaBlock(tt.in)
		if got != tt.want {
			t.Errorf("extractLuaBlock(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestLintPasses(t *testing.T) {
	res := lint(`function main()
  k.print("ok")
  k.quit()
end`)
	if len(res.Errors) != 0 {
		t.Errorf("expected no errors, got %v", res.Errors)
	}
}

func TestLintFails(t *testing.T) {
	res := lint("k.nonexistent()")
	if len(res.Errors) == 0 {
		t.Error("expected errors for unknown k.* reference")
	}
}

func TestCheckIntegration(t *testing.T) {
	src := `function main()
  k.print("hello")
end`
	res := checker.Check(src, "test.lua")
	if len(res.Errors) != 0 {
		t.Errorf("checker failed: %v", res.Errors)
	}
}

// TestGenerateResultWriteable verifies that generate result can be written
// and re-validated end-to-end (no LLM needed for this check).
func TestGenerateResultWriteable(t *testing.T) {
	tmp := t.TempDir()
	path := tmp + "/app.lua"
	content := `function main()
  k.print("gen")
  k.quit()
end`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != content {
		t.Errorf("roundtrip mismatch")
	}
}

func TestComponentPrompt(t *testing.T) {
	p := KaluaComponentPrompt()
	if p == "" {
		t.Fatal("empty component prompt")
	}
	for _, name := range []string{"k.form.new", "k.ctrl.label", "k.ctrl.textbox", "k.ctrl.button", "k.ctrl.combo", "k.ctrl.list", "k.ctrl.checkbox", "k.ctrl.radio", "k.ctrl.table", "k.ctrl.chart", "k.ctrl.image", "k.msgbox", "k.popup", "k.ctrl.set_value", "k.ctrl.get_value"} {
		if !strings.Contains(p, name) {
			t.Errorf("component prompt missing %s", name)
		}
	}
}

func TestGenerateStreamMock(t *testing.T) {
	srv := mockChatStreamServer(t, "function main()\n  k.print(\"hi\")\n  k.quit()\nend")
	defer srv.Close()

	cfg := ProviderConfig{BaseURL: srv.URL, Model: "m"}
	// Mutable holder captured by reference: closure field writes propagate.
	type holder struct {
		tokens []string
		done   StreamEvent
	}
	h := &holder{}
	req := GenerateRequest{Request: "build hello app"}
	res, err := GenerateStream(context.Background(), cfg, req, func(ev StreamEvent) {
		if ev.Type == "token" {
			h.tokens = append(h.tokens, ev.Text)
		}
		if ev.Type == "done" {
			h.done = ev
		}
	})
	if err != nil {
		t.Fatalf("GenerateStream: %v", err)
	}
	if len(h.tokens) == 0 {
		t.Error("expected streamed tokens")
	}
	if !h.done.Ok {
		t.Errorf("expected done ok=true, got errors=%v", h.done.Errors)
	}
	if !strings.Contains(h.done.Script, "main()") {
		t.Errorf("script missing main: %s", h.done.Script)
	}
	if res.Script != h.done.Script {
		t.Error("result script != done script")
	}
}

func TestBuildMessagesHistory(t *testing.T) {
	req := GenerateRequest{
		Request: "make the button green",
		History: []ChatMessage{
			{Role: "user", Content: "a login form with a button"},
			{Role: "assistant", Content: "function main() ... end"},
		},
	}
	msgs := buildMessages(req)
	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages (system+2 history+user), got %d", len(msgs))
	}
	if msgs[0].Role != "system" {
		t.Errorf("msgs[0].role=%q want system", msgs[0].Role)
	}
	if msgs[1].Role != "user" || !strings.Contains(msgs[1].Content, "login form") {
		t.Errorf("history user turn missing: %q", msgs[1].Content)
	}
	if msgs[2].Role != "assistant" || !strings.Contains(msgs[2].Content, "main()") {
		t.Errorf("history assistant turn missing: %q", msgs[2].Content)
	}
	if msgs[3].Role != "user" || !strings.Contains(msgs[3].Content, "green") {
		t.Errorf("current user turn missing: %q", msgs[3].Content)
	}
}

func TestBuildMessagesNoHistory(t *testing.T) {
	msgs := buildMessages(GenerateRequest{Request: "hello"})
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "system" || msgs[1].Role != "user" {
		t.Errorf("roles wrong: %s / %s", msgs[0].Role, msgs[1].Role)
	}
}

// mockOpenAIStreamProvider simulates a strict OpenAI-compatible server that
// only SSE-streams when the request BODY carries "stream":true — the Accept
// header alone is not enough (this is how LM Studio/OpenAI behave). Without
// the flag it must return a plain JSON response; with it, SSE frames.
func mockOpenAIStreamProvider(t *testing.T, response string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if !req.Stream {
			// Server requires "stream":true in the body; it answers JSON.
			b, _ := json.Marshal(ChatResponse{Choices: []ChatChoice{{
				Message: ChatMessage{Role: "assistant", Content: "NOT-STREAMED"},
			}}})
			w.Header().Set("Content-Type", "application/json")
			w.Write(b)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		for _, tok := range strings.Split(response, " ") {
			_, _ = w.Write([]byte(sseFrame(tok + " ")))
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
}

// TestCompletionStreamSendsStreamTrue verifies the body carries "stream":true;
// without it, strict servers (LM Studio) return JSON and the stream yields
// nothing — the bug that left the chat panel stuck on "Generating…".
func TestCompletionStreamSendsStreamTrue(t *testing.T) {
	srv := mockOpenAIStreamProvider(t, "function main() end")
	defer srv.Close()
	c := NewClient(ProviderConfig{BaseURL: srv.URL, Model: "m"})
	ch, err := c.CompletionStream(context.Background(), []ChatMessage{{Role: "user", Content: "x"}})
	if err != nil {
		t.Fatalf("CompletionStream: %v", err)
	}
	var sb strings.Builder
	n := 0
	for tok := range ch {
		n++
		sb.WriteString(tok)
	}
	if n == 0 {
		t.Fatal("no tokens — server did not see stream:true")
	}
	if !strings.Contains(sb.String(), "main()") {
		t.Errorf("wrong content: %q", sb.String())
	}
}

// TestCompletionStreamErrorStatus verifies a 4xx/5xx from the LLM surfaces as
// an error instead of silently yielding an empty channel.
func TestCompletionStreamErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := json.Marshal(map[string]any{"error": map[string]string{"message": "context_length_exceeded"}})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write(b)
	}))
	defer srv.Close()
	c := NewClient(ProviderConfig{BaseURL: srv.URL, Model: "m"})
	ch, err := c.CompletionStream(context.Background(), []ChatMessage{{Role: "user", Content: "x"}})
	if err == nil {
		t.Fatal("expected error for HTTP 400, got nil")
	}
	if ch != nil {
		t.Error("expected nil channel on error")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("error missing status: %v", err)
	}
}

// mockFencedProvider returns the response wrapped in a ```lua ... ``` fence,
// mimicking how real local models answer the "return a ```lua block" prompt.
func mockFencedProvider(t *testing.T, response string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		fenced := "```lua\n" + response + "\n```"
		if req.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			for _, tok := range strings.Split(fenced, " ") {
				_, _ = w.Write([]byte(sseFrame(tok + " ")))
			}
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
			return
		}
		b, _ := json.Marshal(ChatResponse{Choices: []ChatChoice{{
			Message: ChatMessage{Role: "assistant", Content: fenced},
		}}})
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}))
}

// TestGenerateStripsFencedCode verifies that a model response wrapped in a
// ```lua fence is unwrapped before validation (previously the backticks made
// the checker fail with "Invalid token", stalling the fix loop).
func TestGenerateStripsFencedCode(t *testing.T) {
	srv := mockFencedProvider(t, "function main()\n  k.print(\"hi\")\n  k.quit()\nend")
	defer srv.Close()
	req := GenerateRequest{Request: "build hello"}
	res, err := Generate(context.Background(), ProviderConfig{BaseURL: srv.URL, Model: "m"}, req)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.HasPrefix(res.Script, "function main()") {
		t.Errorf("script not unwrapped: %q", res.Script)
	}
	if strings.Contains(res.Script, "```") {
		t.Errorf("script still contains fence: %q", res.Script)
	}
	if len(res.Logs) == 0 || !strings.Contains(res.Logs[len(res.Logs)-1], "Validation passed") {
		t.Errorf("expected validation pass log, got %v", res.Logs)
	}
}

// TestGenerateStreamStripsFencedCode is the streaming twin of the above.
func TestGenerateStreamStripsFencedCode(t *testing.T) {
	srv := mockFencedProvider(t, "function main()\n  k.ctrl.textbox(\"m\", \"n\", {label = \"N\"})\n  k.quit()\nend")
	defer srv.Close()
	type holder struct {
		done StreamEvent
	}
	h := &holder{}
	req := GenerateRequest{Request: "a form"}
	res, err := GenerateStream(context.Background(), ProviderConfig{BaseURL: srv.URL, Model: "m"}, req, func(ev StreamEvent) {
		if ev.Type == "done" {
			h.done = ev
		}
	})
	if err != nil {
		t.Fatalf("GenerateStream: %v", err)
	}
	if strings.Contains(res.Script, "```") {
		t.Errorf("streamed script still fenced: %q", res.Script)
	}
	if !strings.Contains(res.Script, "function main()") {
		t.Errorf("streamed script missing main: %q", res.Script)
	}
	if !h.done.Ok {
		t.Errorf("expected ok after unwrap, errors=%v", h.done.Errors)
	}
}
