// Package common provides shared types and interfaces used across KALUA packages to avoid import cycles.
package common

// AIConfig carries LLM connection settings for the AI chat form. Defined
// here rather than in bindings because ai imports bindings, so bindings
// cannot import ai.
type AIConfig struct {
	BaseURL string
	APIKey  string
	Model   string
}

// ToolCall is a model-requested tool invocation emitted as a tagged JSON block:
//   function {"name":"get_customer","arguments":{"id":7}}
type ToolCall struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

// ToolSchema describes one tool available to the agent.
type ToolSchema struct {
	Name    string `json:"name"`
	Desc    string `json:"desc"`
	Params  []Param `json:"params,omitempty"`
}

// Param describes one tool parameter.
type Param struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
	Desc     string `json:"desc"`
}

// ChatMessage represents a single message in a conversation.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	Name    string `json:"name,omitempty"`
}
