package builder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kalua/internal/ai"
)

// mockAIProvider creates an httptest server that acts as an OpenAI-compatible
// LLM. responseFn generates responses based on request content.
func mockAIProvider(t *testing.T, responseFn func(body string) string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("expected /chat/completions, got %s", r.URL.Path)
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		var req ai.ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		resp := ai.ChatResponse{
			Choices: []ai.ChatChoice{{
				Message: ai.ChatMessage{Role: "assistant", Content: responseFn(fmt.Sprintf("%+v", req))},
			}},
		}
		w.Header().Set("Content-Type", "application/json")
		b, _ := json.Marshal(resp)
		w.Write(b)
	}))
}

func TestAIStatus(t *testing.T) {
	srv := mockAIProvider(t, func(body string) string {
		return "ok"
	})
	defer srv.Close()

	// Create a builder with the mock provider
	s := &Server{
		aiCfg: ai.ProviderConfig{
			BaseURL: srv.URL,
			Model:   "test",
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/ai/status", nil)
	w := httptest.NewRecorder()
	s.handleAIStatus(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", w.Code, http.StatusOK)
	}
	var resp AIStatusResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if !resp.Reachable {
		t.Error("expected reachable=true")
	}
	if resp.Model != "test" {
		t.Errorf("model=%q want test", resp.Model)
	}
}

func TestAIGenerate(t *testing.T) {
	srv := mockAIProvider(t, func(body string) string {
		return "function main()\n  k.print(\"hello\")\n  k.quit()\nend"
	})
	defer srv.Close()

	tmp := t.TempDir()
	path := tmp + "/gen.lua"
	s := &Server{
		path:   path,
		format: "lua",
		aiCfg: ai.ProviderConfig{
			BaseURL: srv.URL,
			Model:   "test",
		},
	}

	reqBody, _ := json.Marshal(map[string]string{
		"request": "generate a hello app",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/ai/generate", bytes.NewReader(reqBody))
	w := httptest.NewRecorder()
	s.handleAIGenerate(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	resp := AIGenerateResponse{}
	json.NewDecoder(w.Body).Decode(&resp)
	if !resp.Ok {
		t.Errorf("expected ok=true, got errors=%v", resp.Errors)
	}
	if !bytes.Contains([]byte(resp.Script), []byte("function main()")) {
		t.Errorf("script missing main(): %s", resp.Script)
	}

	// Verify file was written
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read generated file: %v", err)
	}
	if string(data) != resp.Script {
		t.Error("file content mismatch")
	}
}

func TestAIGenerateInvalid(t *testing.T) {
	srv := mockAIProvider(t, func(body string) string {
		return "this is not lua"
	})
	defer srv.Close()

	tmp := t.TempDir()
	path := tmp + "/bad.lua"
	s := &Server{
		path:   path,
		format: "lua",
		aiCfg: ai.ProviderConfig{
			BaseURL: srv.URL,
			Model:   "test",
		},
	}

	reqBody, _ := json.Marshal(map[string]string{
		"request": "generate something",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/ai/generate", bytes.NewReader(reqBody))
	w := httptest.NewRecorder()
	s.handleAIGenerate(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", w.Code, http.StatusOK)
	}
	resp := AIGenerateResponse{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Ok {
		t.Error("expected ok=false for invalid Lua")
	}
}

func TestAIFix(t *testing.T) {
	srv := mockAIProvider(t, func(body string) string {
		if hasSubstring(body, "errors") {
			return "function main()\n  k.print(\"fixed\")\n  k.quit()\nend"
		}
		return "broken code"
	})
	defer srv.Close()

	tmp := t.TempDir()
	path := tmp + "/fix.lua"
	s := &Server{
		path:   path,
		format: "lua",
		aiCfg: ai.ProviderConfig{
			BaseURL: srv.URL,
			Model:   "test",
		},
	}
	// Write initial broken content
	os.WriteFile(path, []byte("bad code"), 0o644)

	reqBody, _ := json.Marshal(map[string]string{
		"script": "bad code",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/ai/fix", bytes.NewReader(reqBody))
	w := httptest.NewRecorder()
	s.handleAIFix(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	resp := AIFixResponse{}
	json.NewDecoder(w.Body).Decode(&resp)
	if !resp.Ok {
		t.Errorf("expected ok=true after fix, errors=%v", resp.Errors)
	}
	if !bytes.Contains([]byte(resp.Script), []byte("main()")) {
		t.Errorf("fixed script missing main(): %s", resp.Script)
	}
}

func hasSubstring(s, substr string) bool {
	return bytes.Contains([]byte(s), []byte(substr))
}

func TestAIEndpointsMethods(t *testing.T) {
	srv := mockAIProvider(t, func(body string) string { return "ok" })
	defer srv.Close()

	tmp := t.TempDir()
	path := tmp + "/test.lua"
	s := &Server{
		path:   path,
		format: "lua",
		aiCfg:  ai.ProviderConfig{BaseURL: srv.URL, Model: "test"},
	}

	// Test wrong methods
	for _, ep := range []string{"/api/ai/status", "/api/ai/generate", "/api/ai/fix"} {
		req := httptest.NewRequest(http.MethodPut, ep, nil)
		w := httptest.NewRecorder()
		switch ep {
		case "/api/ai/status":
			s.handleAIStatus(w, req)
		case "/api/ai/generate":
			s.handleAIGenerate(w, req)
		case "/api/ai/fix":
			s.handleAIFix(w, req)
		}
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s PUT: status=%d want %d", ep, w.Code, http.StatusMethodNotAllowed)
		}
	}
}

// mockSSEProvider returns an httptest LLM that responds to streaming requests
// with OpenAI-style SSE chunks (one "data:" frame per token).
func mockSSEProvider(t *testing.T, response string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		for _, tok := range strings.Split(response, " ") {
			b, _ := json.Marshal(map[string]any{"choices": []any{
				map[string]any{"delta": map[string]any{"content": tok + " "}},
			}})
			_, _ = w.Write([]byte("data: " + string(b) + "\n\n"))
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
}

// startStreamingServer boots a real builder Server (port 0) with a custom
// LLM config so /api/ai/* can be exercised over real HTTP.
func startStreamingServer(t *testing.T, cfg ai.ProviderConfig) (*Server, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "app.lua")
	srv, err := New(path, "127.0.0.1", 0)
	if err != nil {
		t.Fatal(err)
	}
	srv.aiCfg = cfg
	ctx, cancel := context.WithCancel(context.Background())
	go srv.Run(ctx)
	t.Cleanup(func() { cancel(); srv.Close() })
	return srv, path
}

func TestAIStreamE2E(t *testing.T) {
	llm := mockSSEProvider(t, "function main()\n  k.print(\"hi\")\n  k.quit()\nend")
	defer llm.Close()
	srv, _ := startStreamingServer(t, ai.ProviderConfig{BaseURL: llm.URL, Model: "m"})
	base := "http://" + srv.Addr()

	b, _ := json.Marshal(map[string]string{"request": "build hello app"})
	req, _ := http.NewRequest("POST", base+"/api/ai/stream", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	defer res.Body.Close()
	txt, _ := io.ReadAll(res.Body)
	body := string(txt)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.StatusCode, body)
	}
	if !strings.Contains(body, "data:") {
		t.Errorf("no SSE frames: %q", body)
	}
	if !strings.Contains(body, "\"type\":\"done\"") {
		t.Errorf("missing done event: %q", body)
	}
}
