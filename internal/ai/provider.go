package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// ProviderConfig holds the LLM connection settings.
type ProviderConfig struct {
	BaseURL string // e.g. http://localhost:1234/v1 or https://openrouter.ai/api/v1
	APIKey  string
	Model   string
}

// defaultConfig returns the provider config from env vars and defaults.
// Values are trimmed so a trailing newline/space in an export cannot break the
// URL, key or model.
func EnvConfig() ProviderConfig {
	baseURL := strings.TrimSpace(os.Getenv("KALUA_AI_BASE_URL"))
	if baseURL == "" {
		baseURL = "http://localhost:1234/v1"
	}
	model := strings.TrimSpace(os.Getenv("KALUA_AI_MODEL"))
	if model == "" {
		model = "local-model"
	}
	return ProviderConfig{
		BaseURL: baseURL,
		APIKey:  strings.TrimSpace(os.Getenv("KALUA_AI_API_KEY")),
		Model:   model,
	}
}

// ChatRequest represents an OpenAI-compatible chat completion request.
type ChatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Stream      bool          `json:"stream,omitempty"`
	Temperature float64       `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
}

// ChatMessage represents a single message in the conversation.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatResponse represents an OpenAI-compatible chat completion response.
type ChatResponse struct {
	Choices []ChatChoice `json:"choices"`
}

// ChatChoice represents a single completion choice.
type ChatChoice struct {
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

// StreamChoice represents a single streaming choice.
type StreamChoice struct {
	Delta struct {
		Content string `json:"content"`
	} `json:"delta"`
	FinishReason string `json:"finish_reason"`
}

// StreamResponse represents a single SSE event from a streaming response.
type StreamResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Choices []StreamChoice `json:"choices"`
}

// Client is an OpenAI-compatible LLM client (LM Studio / OpenRouter).
type Client struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

// NewClient creates a new LLM client from a ProviderConfig.
func NewClient(cfg ProviderConfig) *Client {
	return &Client{
		baseURL: cfg.BaseURL,
		apiKey:  cfg.APIKey,
		model:   cfg.Model,
		client:  &http.Client{Timeout: 5 * time.Minute},
	}
}

// NewClientFromEnv creates a client using env vars (KALUA_AI_BASE_URL, KALUA_AI_API_KEY, KALUA_AI_MODEL).
func NewClientFromEnv() *Client {
	return NewClient(EnvConfig())
}

// Completion sends a non-streaming chat completion request and returns the assistant's message content.
func (c *Client) Completion(ctx context.Context, messages []ChatMessage) (string, error) {
	req := ChatRequest{
		Model:    c.model,
		Messages: messages,
	}
	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	url := c.baseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("LLM error: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	var result ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("LLM returned no choices")
	}
	return result.Choices[0].Message.Content, nil
}

// CompletionStream sends a streaming chat completion request and returns a channel
// of content deltas. The channel is closed when the stream ends. Chromium-style
// OpenAI servers (LM Studio, OpenAI, OpenRouter) only SSE-stream when the
// request body carries "stream": true — the Accept header alone is not enough.
func (c *Client) CompletionStream(ctx context.Context, messages []ChatMessage) (<-chan string, error) {
	req := ChatRequest{
		Model:    c.model,
		Messages: messages,
		Stream:   true,
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := c.baseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer func() {
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
		}
	}()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("LLM streaming error: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	ch := make(chan string)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		// Server-Sent Events: each object arrives as a "data: {...}" line
		// (followed by a blank line); the stream ends with "data: [DONE]".
		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			payload := strings.TrimSpace(line[len("data:"):])
			if payload == "" || payload == "[DONE]" {
				if payload == "[DONE]" {
					return
				}
				continue
			}
			var sr StreamResponse
			if err := json.Unmarshal([]byte(payload), &sr); err != nil {
				return
			}
			for _, sc := range sr.Choices {
				if sc.Delta.Content != "" {
					ch <- sc.Delta.Content
				}
				if sc.FinishReason == "stop" {
					return
				}
			}
		}
	}()
	return ch, nil
}
