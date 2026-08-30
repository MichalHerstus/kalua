package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"kalua/internal/host"
)

// freePort returns an available TCP port on the loopback interface.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// waitUp polls GET /healthz until the server responds or the timeout passes.
func waitUp(t *testing.T, base string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(base + "/healthz")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("server did not become ready")
}

func TestServer_E2E_HTTP_WS_TCP(t *testing.T) {
	port := freePort(t)
	dir := t.TempDir()
	script := filepath.Join(dir, "app.lua")
	src := `
function main() end

function handle_http(req)
  if req.path == "/json" then
    return {json = {path = req.path, remote = req.remote_addr}}
  end
  return {status = 200, body = "ok:" .. req.path}
end

function handle_ws(msg)
  if msg.type == "text" then
    return "echo:" .. msg.data
  end
end

function handle_tcp(msg)
  if msg.type == "text" then
    return "tcp-echo:" .. msg.data
  end
end
`
	if err := os.WriteFile(script, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		Host:       "127.0.0.1",
		Port:       port,
		Workers:    2,
		Mode:       "http,ws,tcp",
		ScriptPath: script,
		Logger:     host.NewLogger(false),
	}
	s := NewServer(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan error, 1)
	go func() { runDone <- s.Run(ctx) }()

	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	waitUp(t, base)

	// --- HTTP ---
	resp, err := http.Get(base + "/hello")
	if err != nil {
		t.Fatalf("http get: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || string(body) != "ok:/hello" {
		t.Errorf("http /hello = %d %q", resp.StatusCode, body)
	}

	if resp2, err := http.Get(base + "/json"); err != nil {
		t.Fatalf("http get /json: %v", err)
	} else {
		b, _ := io.ReadAll(resp2.Body)
		resp2.Body.Close()
		if ct := resp2.Header.Get("content-type"); !strings.Contains(ct, "application/json") {
			t.Errorf("/json content-type = %q", ct)
		}
		if !strings.Contains(string(b), `"path":"/json"`) {
			t.Errorf("/json body = %s", b)
		}
	}

	// --- WebSocket ---
	wsCtx, wsCancel := context.WithTimeout(context.Background(), 5*time.Second)
	wsURL := fmt.Sprintf("ws://127.0.0.1:%d/ws", port)
	ws, _, err := websocket.Dial(wsCtx, wsURL, nil)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	if err := ws.Write(wsCtx, websocket.MessageText, []byte("hi")); err != nil {
		t.Fatalf("ws write: %v", err)
	}
	_, data, err := ws.Read(wsCtx)
	wsCancel()
	if err != nil {
		t.Fatalf("ws read: %v", err)
	}
	if string(data) != "echo:hi" {
		t.Errorf("ws echo = %q, want echo:hi", data)
	}
	ws.Close(websocket.StatusNormalClosure, "")

	// --- TCP ---
	tcpAddr := fmt.Sprintf("127.0.0.1:%d", port+1)
	tcpConn, err := net.DialTimeout("tcp", tcpAddr, 5*time.Second)
	if err != nil {
		t.Fatalf("tcp dial: %v", err)
	}
	tcpConn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := tcpConn.Write([]byte("ping")); err != nil {
		t.Fatalf("tcp write: %v", err)
	}
	buf := make([]byte, 64)
	n, err := tcpConn.Read(buf)
	if err != nil {
		t.Fatalf("tcp read: %v", err)
	}
	if got := string(buf[:n]); got != "tcp-echo:ping" {
		t.Errorf("tcp echo = %q, want tcp-echo:ping", got)
	}
	tcpConn.Close()

	// Shut down and verify Run returns cleanly.
	cancel()
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
}

func TestServer_InitShutdown(t *testing.T) {
	port := freePort(t)
	dir := t.TempDir()
	script := filepath.Join(dir, "app.lua")
	src := `
function main() end

function init(config)
  k.shared.set("init_called", config.mode)
end

function shutdown()
  k.shared.set("shutdown_called", "yes")
end

function handle_http(req)
  return {json = {init = k.shared.get("init_called"), shutdown = k.shared.get("shutdown_called")}}
end
`
	if err := os.WriteFile(script, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		Host:       "127.0.0.1",
		Port:       port,
		Workers:    2,
		Mode:       "http,ws,tcp",
		ScriptPath: script,
		Logger:     host.NewLogger(false),
	}
	s := NewServer(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() { runDone <- s.Run(ctx) }()

	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	waitUp(t, base)

	resp, err := http.Get(base + "/")
	if err != nil {
		t.Fatalf("http get: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if v := string(body); !strings.Contains(v, `"init":"http,ws,tcp"`) {
		t.Errorf("before shutdown: body = %s", v)
	}

	// Cancel: Run must invoke shutdown() once on a worker before teardown.
	cancel()
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancel")
	}

	// Shared store holds the raw JSON-encoded string ("yes" serialized as a
	// JSON string), so assert on the decoded value.
	raw := s.shared.Get("shutdown_called")
	var got string
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("shutdown_called raw = %q, unmarshal: %v", raw, err)
	}
	if got != "yes" {
		t.Errorf("shutdown_called = %q, want yes", got)
	}
}