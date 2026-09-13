package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/coder/websocket"

	"kalua/internal/bindings"
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

func writeScript(t *testing.T, path, src string) {
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
}

// wsPump continuously reads a WebSocket connection on a background goroutine.
// A context expiring on websocket.Conn.Read closes the connection (see the
// websocket docs), so the test never reads with a timeout; it collects
// messages here and drains them with select.
type wsPump struct {
	conn *websocket.Conn
	msgs chan string
	end  chan struct{}
}

func pumpStart(conn *websocket.Conn) *wsPump {
	p := &wsPump{
		conn: conn,
		msgs: make(chan string, 64),
		end:  make(chan struct{}),
	}
	go func() {
		defer close(p.end)
		ctx, _ := context.WithCancel(context.Background())
		for {
			_, data, err := p.conn.Read(ctx)
			if err != nil {
				return
			}
			select {
			case p.msgs <- string(data):
			default:
				return
			}
		}
	}()
	return p
}

func has(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// pumpTypes drains pump messages until the window elapses or every type in
// want has been seen. It returns the distinct types observed. With an empty
// want slice it always waits the full window, so callers can assert absence
// (e.g. no "reload" on an unchanged file).
func pumpTypes(p *wsPump, want []string, window time.Duration) []string {
	var seen []string
	deadline := time.Now().Add(window)
	for time.Now().Before(deadline) {
		rem := deadline.Sub(time.Now())
		if rem <= 0 * time.Second {
			break
		}
		select {
		case data, ok := <-p.msgs:
			if !ok {
				return seen
			}
			var msg map[string]interface{}
			if err := json.Unmarshal([]byte(data), &msg); err != nil {
				continue
			}
			typ, _ := msg["type"].(string)
			if typ != "" && !has(seen, typ) {
				seen = append(seen, typ)
			}
			if len(want) > 0 {
				done := true
				for _, w := range want {
					if !has(seen, w) {
						done = false
					}
				}
				if done {
					return seen
				}
			}
		case <-time.After(rem):
			return seen
		}
	}
	return seen
}

// TestServer_Reload broadcasts a "reload" message when the app script changes,
// broadcasts a "status" message (keeping the old app) on a broken script, and
// does nothing when the content is unchanged.
func TestServer_Reload(t *testing.T) {
	port := freePort(t)
	dir := t.TempDir()
	script := filepath.Join(dir, "apps.lua")
	writeScript(t, script, `
function main()
  local form = k.form.new("f", {})
  k.ctrl.label("f", "lbl", {text = "v1"})
  k.form.show("f")
end
`)

	s := NewServer("127.0.0.1", port, 8, bindings.Options{}, host.NewLogger(false))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan error, 1)
	go func() { runDone <- s.Run(ctx, script) }()

	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	waitUp(t, base)

	connCtx, connCancel := context.WithCancel(context.Background())
	defer connCancel()
	url := fmt.Sprintf("ws://127.0.0.1:%d/ws/ui?script=%s", port, script)
	ws, _, err := websocket.Dial(connCtx, url, nil)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	defer ws.Close(websocket.StatusNormalClosure, "")
	p := pumpStart(ws)

	// The initial session sends init (+ render). Swallow it.
	initial := pumpTypes(p, []string{"init"}, 3 * time.Second)
	if !has(initial, "init") {
		t.Fatalf("no init message on connect: %v", initial)
	}

	// 1. Unchanged content -> Reload is a no-op (no reload message).
	if err := s.Reload(); err != nil {
		t.Fatalf("Reload (unchanged): %v", err)
	}
	if types := pumpTypes(p, []string{}, 600 * time.Millisecond); has(types, "reload") {
		t.Fatalf("unchanged reload sent a reload message: %v", types)
	}

	// 2. Changed content -> a reload message is broadcast.
	writeScript(t, script, `
function main()
  local form = k.form.new("f", {})
  k.ctrl.label("f", "lbl", {text = "v2"})
  k.form.show("f")
end
`)
	if err := s.Reload(); err != nil {
		t.Fatalf("Reload (changed): %v", err)
	}
	if types := pumpTypes(p, []string{"reload"}, 2 * time.Second); !has(types, "reload") {
		t.Fatalf("changed reload did not send a reload message: %v", types)
	}

	// 3. Broken script -> Reload returns an error and only a status message
	//    is broadcast; the old app keeps running (no reload).
	writeScript(t, script, `
function main(
`)
	if err := s.Reload(); err == nil {
		t.Fatal("Reload on broken script succeeded, want error")
	}
	if types := pumpTypes(p, []string{"status"}, 2 * time.Second); !has(types, "status") {
		t.Fatalf("broken script did not send a status message: %v", types)
	}
	if types := pumpTypes(p, []string{}, 500 * time.Millisecond); has(types, "reload") {
		t.Fatalf("broken script also sent a reload message: %v", types)
	}

	cancel()
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not stop after cancel")
	}
}