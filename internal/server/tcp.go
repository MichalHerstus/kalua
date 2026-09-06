package server

import (
	"context"
	"net"
	"sync"
)

// TCPHub manages TCP connections across workers.
type TCPHub struct {
	mu       sync.RWMutex
	conns    map[string]*TCPConn
	workerCh chan *Worker
	acceptCh chan *TCPConn // channel for k.tcp_accept to wait for new connections
}

// TCPConn wraps a TCP connection for use in Lua callbacks.
type TCPConn struct {
	ID     string
	conn   net.Conn
	sendCh chan []byte
	close  func()
}

// NewTCPHub creates a new TCP hub.
func NewTCPHub(workerCh chan *Worker) *TCPHub {
	return &TCPHub{
		conns:    make(map[string]*TCPConn),
		workerCh: workerCh,
		acceptCh: make(chan *TCPConn, 16),
	}
}

// Register adds a new TCP connection.
func (h *TCPHub) Register(id string, c net.Conn, close func()) *TCPConn {
	tcp := &TCPConn{
		ID:     id,
		conn:   c,
		sendCh: make(chan []byte, 256),
		close:  close,
	}
	h.mu.Lock()
	h.conns[id] = tcp
	h.mu.Unlock()

	// Also push to accept channel for k.tcp_accept
	select {
	case h.acceptCh <- tcp:
	default:
		// Channel full, connection will be handled by auto-dispatch
	}

	return tcp
}

// Accept waits for and returns the next incoming TCP connection ID.
// Returns empty string if the hub is closed.
func (h *TCPHub) Accept(ctx context.Context) string {
	select {
	case tcp := <-h.acceptCh:
		return tcp.ID
	case <-ctx.Done():
		return ""
	}
}

// Unregister removes a TCP connection.
func (h *TCPHub) Unregister(id string) {
	h.mu.Lock()
	delete(h.conns, id)
	h.mu.Unlock()
}

// Get returns a connection by ID.
func (h *TCPHub) Get(id string) (*TCPConn, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	tcp, ok := h.conns[id]
	return tcp, ok
}

// Broadcast sends a message to all connected clients.
func (h *TCPHub) Broadcast(msg []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, tcp := range h.conns {
		select {
		case tcp.sendCh <- msg:
		default:
			// Channel full, drop
		}
	}
}

// Send sends a message to a specific client.
func (h *TCPHub) Send(id string, msg []byte) bool {
	h.mu.RLock()
	tcp, ok := h.conns[id]
	h.mu.RUnlock()
	if !ok {
		return false
	}
	select {
	case tcp.sendCh <- msg:
		return true
	default:
		return false
	}
}

// Close closes a specific connection.
func (h *TCPHub) Close(id string) {
	h.mu.RLock()
	tcp, ok := h.conns[id]
	h.mu.RUnlock()
	if ok {
		tcp.close()
	}
}

// Read reads from the TCP connection.
func (tcp *TCPConn) Read(ctx context.Context) ([]byte, error) {
	buf := make([]byte, 4096)
	n, err := tcp.conn.Read(buf)
	return buf[:n], err
}

// Write writes to the TCP connection.
func (tcp *TCPConn) Write(ctx context.Context, data []byte) error {
	_, err := tcp.conn.Write(data)
	return err
}

// Close closes the connection.
func (tcp *TCPConn) Close() error {
	return tcp.conn.Close()
}
