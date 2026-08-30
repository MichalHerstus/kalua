package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"kalua/internal/bindings"
)

type discardLogger struct{}

func (discardLogger) Printf(string, ...interface{}) {}
func (discardLogger) Errorf(string, ...interface{}) {}

type captureLogger struct {
	mu  sync.Mutex
	err []string
}

func (c *captureLogger) Printf(string, ...interface{}) {}
func (c *captureLogger) Errorf(f string, a ...interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.err = append(c.err, fmt.Sprintf(f, a...))
}

func (c *captureLogger) errors() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, len(c.err))
	copy(out, c.err)
	return out
}

// newTestWorker builds a serve-mode worker from a Lua script string.
func newTestWorker(t *testing.T, src string) *Worker {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "app.lua")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	w, err := NewWorker(1, path, bindings.Options{}, NewSharedState(), NewWSHub(nil), NewTCPHub(nil), discardLogger{})
	if err != nil {
		t.Fatalf("NewWorker: %v", err)
	}
	t.Cleanup(w.Close)
	return w
}

func TestCallHTTP_ResponseForms(t *testing.T) {
	w := newTestWorker(t, `
function main() end

function handle_http(req)
  if req.path == "/text" then
    return "plain text"
  elseif req.path == "/json" then
    return {json = {a = 1, ok = true}}
  elseif req.path == "/status" then
    return {status = 404}
  elseif req.path == "/full" then
    return {status = 201, headers = {["content-type"] = "application/xml"}, body = "<x/>"}
  elseif req.path == "/nil" then
    return
  elseif req.path == "/echo" then
    return {json = {q = req.query, qr = req.query_raw, ra = req.remote_addr, tls = req.tls, body = req.body}}
  end
  return {status = 500}
end
`)

	ctx := context.Background()

	// String shortcut
	resp, err := w.CallHTTP(ctx, HTTPRequest{Path: "/text"})
	if err != nil {
		t.Fatalf("/text: %v", err)
	}
	if resp.Status != 200 || resp.Body != "plain text" || resp.Headers["content-type"] != "text/plain" {
		t.Errorf("/text = %+v", resp)
	}

	// JSON shortcut
	resp, err = w.CallHTTP(ctx, HTTPRequest{Path: "/json"})
	if err != nil {
		t.Fatalf("/json: %v", err)
	}
	if resp.Status != 200 || resp.Headers["content-type"] != "application/json" {
		t.Errorf("/json status/headers = %+v", resp)
	}
	var parsed interface{}
	if err := json.Unmarshal([]byte(resp.Body), &parsed); err != nil {
		t.Fatalf("/json body not valid JSON: %q (%v)", resp.Body, err)
	}

	// Status-only shortcut
	resp, err = w.CallHTTP(ctx, HTTPRequest{Path: "/status"})
	if err != nil {
		t.Fatalf("/status: %v", err)
	}
	if resp.Status != 404 || resp.Body != "" {
		t.Errorf("/status = %+v", resp)
	}

	// Full form: body wins over json
	resp, err = w.CallHTTP(ctx, HTTPRequest{Path: "/full"})
	if err != nil {
		t.Fatalf("/full: %v", err)
	}
	if resp.Status != 201 || resp.Body != "<x/>" || resp.Headers["content-type"] != "application/xml" {
		t.Errorf("/full = %+v", resp)
	}

	// No return value
	resp, err = w.CallHTTP(ctx, HTTPRequest{Path: "/nil"})
	if err != nil {
		t.Fatalf("/nil: %v", err)
	}
	if resp.Status != 200 || resp.Body != "" {
		t.Errorf("/nil = %+v", resp)
	}

	// Echo: verify the req table fields
	resp, err = w.CallHTTP(ctx, HTTPRequest{
		Path:        "/echo",
		Query:       "name=alice&tag=a&tag=b&x=",
		QueryValues: map[string][]string{"name": {"alice"}, "tag": {"a", "b"}, "x": {""}},
		RemoteAddr:  "1.2.3.4:5678",
		Body:        "hello",
	})
	if err != nil {
		t.Fatalf("/echo: %v", err)
	}
	var echo struct {
		Q  map[string]interface{} `json:"q"`
		QR string                 `json:"qr"`
		RA string                 `json:"ra"`
		TLS bool                  `json:"tls"`
		B  string                 `json:"body"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &echo); err != nil {
		t.Fatalf("/echo unmarshal: %v (body=%q)", err, resp.Body)
	}
	if echo.QR != "name=alice&tag=a&tag=b&x=" {
		t.Errorf("query_raw = %q", echo.QR)
	}
	if echo.RA != "1.2.3.4:5678" {
		t.Errorf("remote_addr = %q", echo.RA)
	}
	if echo.TLS {
		t.Error("tls should be false for plain HTTP")
	}
	if echo.B != "hello" {
		t.Errorf("body = %q", echo.B)
	}
	name, _ := echo.Q["name"].(string)
	if name != "alice" {
		t.Errorf("query.name = %v", echo.Q["name"])
	}
	tagArr, _ := echo.Q["tag"].([]interface{})
	if len(tagArr) != 2 || tagArr[0] != "a" || tagArr[1] != "b" {
		t.Errorf("query.tag = %v", echo.Q["tag"])
	}
}

func TestCallHTTP_NoHandler(t *testing.T) {
	w := newTestWorker(t, `
function main() end
`)
	resp, err := w.CallHTTP(context.Background(), HTTPRequest{Path: "/anything"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != 404 {
		t.Errorf("Status = %d, want 404", resp.Status)
	}
}

func TestCallHTTP_LuaError(t *testing.T) {
	w := newTestWorker(t, `
function main() end

function handle_http(req)
  error("boom")
end
`)
	resp, err := w.CallHTTP(context.Background(), HTTPRequest{Path: "/err"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error = %v", err)
	}
	if resp.Status != 500 {
		t.Errorf("Status = %d, want 500", resp.Status)
	}
}

func TestCallWS_TCP_PanicsFixed(t *testing.T) {
	w := newTestWorker(t, `
function main() end

function handle_ws(conn)
  return "pong"
end

function handle_tcp(msg)
  return "echo:" .. msg.data
end
`)
	// These previously panicked: NewThread returns a nil cancel when the state
	// has no context, and defer cancel() would nill-panic.
	ws := &WSConn{id: "ws-1", sendCh: make(chan []byte, 8)}
	w.CallWS(WSMessage{Type: "open", ClientID: "ws-1"}, ws)
	select {
	case got := <-ws.sendCh:
		if string(got) != "pong" {
			t.Errorf("ws echo = %q, want pong", got)
		}
	default:
		t.Error("expected handle_ws return value to be echoed")
	}

	tcp := &TCPConn{id: "tcp-1", sendCh: make(chan []byte, 8)}
	w.CallTCP(TCPMessage{Type: "text", Data: "hi", ClientID: "tcp-1"}, tcp)
	select {
	case got := <-tcp.sendCh:
		if string(got) != "echo:hi" {
			t.Errorf("tcp echo = %q, want echo:hi", got)
		}
	default:
		t.Error("expected handle_tcp return value to be echoed")
	}
}

func TestCallWS_TCP_ErrorLogged(t *testing.T) {
	log := &captureLogger{}
	dir := t.TempDir()
	path := filepath.Join(dir, "app.lua")
	src := `
function main() end

function handle_ws(conn)
  error("ws-boom")
end

function handle_tcp(conn)
  error("tcp-boom")
end
`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	w, err := NewWorker(1, path, bindings.Options{}, NewSharedState(), NewWSHub(nil), NewTCPHub(nil), log)
	if err != nil {
		t.Fatalf("NewWorker: %v", err)
	}
	defer w.Close()

	w.CallWS(WSMessage{Type: "text", ClientID: "ws-2"}, &WSConn{id: "ws-2"})
	w.CallTCP(TCPMessage{Type: "text", ClientID: "tcp-2"}, &TCPConn{id: "tcp-2"})

	got := strings.Join(log.errors(), "; ")
	if !strings.Contains(got, "ws-boom") || !strings.Contains(got, "tcp-boom") {
		t.Errorf("logged errors = %q, want ws-boom and tcp-boom", got)
	}
}