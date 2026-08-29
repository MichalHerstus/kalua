package server

import (
	"context"
	"sync"

	"github.com/coder/websocket"
)

// WSHub manages WebSocket connections across workers.
type WSHub struct {
	mu       sync.RWMutex
	conns    map[string]*WSConn
	workerCh chan *Worker
}

// WSConn wraps a WebSocket connection for use in Lua callbacks.
type WSConn struct {
	id     string
	conn   *websocket.Conn
	sendCh chan []byte
	close  func()
}

// NewWSHub creates a new WebSocket hub.
func NewWSHub(workerCh chan *Worker) *WSHub {
	return &WSHub{
		conns:    make(map[string]*WSConn),
		workerCh: workerCh,
	}
}

// Register adds a new WebSocket connection.
func (h *WSHub) Register(id string, c *websocket.Conn, close func()) *WSConn {
	ws := &WSConn{
		id:     id,
		conn:   c,
		sendCh: make(chan []byte, 256),
		close:  close,
	}
	h.mu.Lock()
	h.conns[id] = ws
	h.mu.Unlock()
	return ws
}

// Unregister removes a WebSocket connection.
func (h *WSHub) Unregister(id string) {
	h.mu.Lock()
	delete(h.conns, id)
	h.mu.Unlock()
}

// Get returns a connection by ID.
func (h *WSHub) Get(id string) (*WSConn, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	ws, ok := h.conns[id]
	return ws, ok
}

// Broadcast sends a message to all connected clients.
func (h *WSHub) Broadcast(msg []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, ws := range h.conns {
		select {
		case ws.sendCh <- msg:
		default:
			// Channel full, drop
		}
	}
}

// Send sends a message to a specific client.
func (h *WSHub) Send(id string, msg []byte) bool {
	h.mu.RLock()
	ws, ok := h.conns[id]
	h.mu.RUnlock()
	if !ok {
		return false
	}
	select {
	case ws.sendCh <- msg:
		return true
	default:
		return false
	}
}

// Close closes a specific connection.
func (h *WSHub) Close(id string) {
	h.mu.RLock()
	ws, ok := h.conns[id]
	h.mu.RUnlock()
	if ok {
		ws.close()
	}
}

// Writer returns a writer for the connection (for http.ResponseWriter compatibility).
func (ws *WSConn) Writer() *WSWriter {
	return &WSWriter{ws: ws}
}

// WSWriter wraps a WSConn for writing.
type WSWriter struct {
	ws *WSConn
}

func (w *WSWriter) Write(p []byte) (int, error) {
	select {
	case w.ws.sendCh <- p:
		return len(p), nil
	default:
		return 0, nil
	}
}

// Reader reads from the WebSocket connection.
func (ws *WSConn) Read(ctx context.Context) ([]byte, error) {
	_, data, err := ws.conn.Read(ctx)
	return data, err
}

// Write writes to the WebSocket connection.
func (ws *WSConn) Write(ctx context.Context, msgType websocket.MessageType, data []byte) error {
	return ws.conn.Write(ctx, msgType, data)
}

// Close closes the connection.
func (ws *WSConn) Close() error {
	return ws.conn.Close(websocket.StatusNormalClosure, "")
}
