// Package common provides shared types used across KALUA packages to avoid import cycles.
package common

// OutboxMsg is a UI command sent to the browser via WebSocket.
type OutboxMsg struct {
	Type     string  `json:"type"`
	Form     string  `json:"form,omitempty"`
	Ctrl     string  `json:"ctrl,omitempty"`
	HTML     string  `json:"html,omitempty"`
	Selector string  `json:"selector,omitempty"`
	ID       string  `json:"id,omitempty"`
	Kind     string  `json:"kind,omitempty"`
	Text     string  `json:"text,omitempty"`
	Msg      string  `json:"msg,omitempty"`
	Stack    string  `json:"stack,omitempty"`
	Accept   string  `json:"accept,omitempty"`
	Multiple bool    `json:"multiple,omitempty"`
	Data     string  `json:"data,omitempty"`
	Modal    bool    `json:"modal,omitempty"`
	GapX     float64 `json:"gap_x,omitempty"`
	GapY     float64 `json:"gap_y,omitempty"`
	Grid     bool    `json:"grid,omitempty"`
	GridMode string  `json:"grid_mode,omitempty"`
	GridPK   string  `json:"grid_pk,omitempty"`
	GridRow  string  `json:"grid_row,omitempty"`
}

// InboxMsg represents an incoming message from the client.
// This mirrors the session's inboxMsg but is transport-agnostic.
type InboxMsg struct {
	Type       string                 `json:"type"`
	Form       string                 `json:"form,omitempty"`
	Ctrl       string                 `json:"ctrl,omitempty"`
	Event      string                 `json:"event,omitempty"`
	Value      interface{}            `json:"value,omitempty"`
	Timer      string                 `json:"timer,omitempty"`
	ID         string                 `json:"id,omitempty"`
	Choice     string                 `json:"choice,omitempty"`
	SelectRows []int                  `json:"select_rows,omitempty"`
	Data       map[string]interface{} `json:"data,omitempty"`
}

// Transport defines the interface for session message transport.
// Both WebSocket (native) and WASM bridge implement this.
type Transport interface {
	// Send sends an outbox message to the client.
	Send(msg OutboxMsg) error

	// Recv returns a channel that receives inbox messages from the client.
	// The session reads from this channel to process incoming events.
	Recv() <-chan InboxMsg

	// Close closes the transport.
	Close() error
}