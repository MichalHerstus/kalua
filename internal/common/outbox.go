// Package common provides shared types used across KALUA packages to avoid import cycles.
package common

// OutboxMsg is a UI command sent to the browser via WebSocket.
type OutboxMsg struct {
	Type     string `json:"type"`
	Form     string `json:"form,omitempty"`
	Ctrl     string `json:"ctrl,omitempty"`
	HTML     string `json:"html,omitempty"`
	Selector string `json:"selector,omitempty"`
	ID       string `json:"id,omitempty"`
	Kind     string `json:"kind,omitempty"`
	Text     string `json:"text,omitempty"`
	Msg      string `json:"msg,omitempty"`
	Stack    string `json:"stack,omitempty"`
	Accept   string `json:"accept,omitempty"`
	Multiple bool   `json:"multiple,omitempty"`
}