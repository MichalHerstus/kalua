package web

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/coder/websocket"

	"kalua/internal/bindings"
	"kalua/internal/host"
)

// TestServer_OriginCheck verifies that WebSocket origin patterns are correctly
// enforced based on the bound host and --allow-origin.
func TestServer_OriginCheck(t *testing.T) {
	// Note: Tests that bind to specific non-loopback IPs (192.168.1.50) are
	// skipped because we can't bind to arbitrary IPs in tests. The server would
	// only listen on that specific IP, not on 127.0.0.1 where the test connects.
	// The logic is tested by the 0.0.0.0 tests with --allow-origin.
	tests := []struct {
		name           string
		host           string
		port           int
		allowOrigin    []string
		origin         string
		wantAccept     bool
	}{
		{
			name:        "loopback host allows localhost origins",
			host:        "127.0.0.1",
			port:        0,
			allowOrigin: nil,
			origin:      "http://127.0.0.1:1234",
			wantAccept:  true,
		},
		{
			name:        "loopback host rejects LAN origin",
			host:        "127.0.0.1",
			port:        0,
			allowOrigin: nil,
			origin:      "http://192.168.1.50",
			wantAccept:  false,
		},
		{
			name:        "0.0.0.0 defaults to localhost only",
			host:        "0.0.0.0",
			port:        9000,
			allowOrigin: nil,
			origin:      "http://127.0.0.1:9000",
			wantAccept:  true,
		},
		{
			name:        "0.0.0.0 rejects LAN origin by default",
			host:        "0.0.0.0",
			port:        9000,
			allowOrigin: nil,
			origin:      "http://192.168.1.50:9000",
			wantAccept:  false,
		},
		{
			name:        "0.0.0.0 with allow-origin accepts LAN pattern",
			host:        "0.0.0.0",
			port:        9000,
			allowOrigin: []string{"http://192.168.1.*"},
			origin:      "http://192.168.1.50:9000",
			wantAccept:  true,
		},
		{
			name:        "0.0.0.0 with allow-origin rejects non-matching",
			host:        "0.0.0.0",
			port:        9000,
			allowOrigin: []string{"http://192.168.1.*"},
			origin:      "http://10.0.0.50:9000",
			wantAccept:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			port := freePort(t)
			dir := t.TempDir()
			script := filepath.Join(dir, "app.lua")
			writeScript(t, script, `
function main()
  k.form.new("f", {})
  k.ctrl.label("f", "lbl", {text = "test"})
  k.form.show("f")
end
`)

			opts := bindings.Options{}
			if tc.allowOrigin != nil {
				opts.AllowOrigin = tc.allowOrigin
			}
			s := NewServer(tc.host, port, 8, opts, host.NewLogger(false))

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			runDone := make(chan error, 1)
			go func() { runDone <- s.Run(ctx, script) }()

			base := fmt.Sprintf("http://127.0.0.1:%d", port)
			waitUp(t, base)

			connCtx, connCancel := context.WithCancel(context.Background())
			defer connCancel()
			url := fmt.Sprintf("ws://127.0.0.1:%d/ws/ui?script=%s", port, script)
			ws, resp, err := websocket.Dial(connCtx, url, &websocket.DialOptions{
				HTTPHeader: http.Header{"Origin": []string{tc.origin}},
			})
			if tc.wantAccept {
				if err != nil {
					t.Fatalf("ws dial: %v (resp: %v)", err, resp)
				}
				ws.Close(websocket.StatusNormalClosure, "")
			} else {
				if err == nil {
					t.Fatalf("expected ws dial to fail for origin %s, but it succeeded", tc.origin)
					ws.Close(websocket.StatusNormalClosure, "")
				}
				if resp != nil && resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusBadRequest {
					t.Logf("unexpected status for rejected origin: %d", resp.StatusCode)
				}
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
		})
	}
}

// TestServer_AuthToken verifies that WebSocket authentication works correctly.
func TestServer_AuthToken(t *testing.T) {
	port := freePort(t)
	dir := t.TempDir()
	script := filepath.Join(dir, "app.lua")
	writeScript(t, script, `
function main()
  k.form.new("f", {})
  k.ctrl.label("f", "lbl", {text = "test"})
  k.form.show("f")
end
`)

	tests := []struct {
		name          string
		token         string
		queryToken    string
		headerToken   string
		wantAccept    bool
	}{
		{
			name:        "no auth token configured, connection accepted",
			token:       "",
			queryToken:  "",
			headerToken: "",
			wantAccept:  true,
		},
		{
			name:        "auth token via query param, correct token accepted",
			token:       "secret123",
			queryToken:  "secret123",
			headerToken: "",
			wantAccept:  true,
		},
		{
			name:        "auth token via header, correct token accepted",
			token:       "secret123",
			queryToken:  "",
			headerToken: "secret123",
			wantAccept:  true,
		},
		{
			name:        "auth token via query param, wrong token rejected",
			token:       "secret123",
			queryToken:  "wrong",
			headerToken: "",
			wantAccept:  false,
		},
		{
			name:        "auth token via header, wrong token rejected",
			token:       "secret123",
			queryToken:  "",
			headerToken: "wrong",
			wantAccept:  false,
		},
		{
			name:        "auth token required but none provided, rejected",
			token:       "secret123",
			queryToken:  "",
			headerToken: "",
			wantAccept:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			opts := bindings.Options{}
			if tc.token != "" {
				opts.AuthToken = tc.token
			}
			s := NewServer("127.0.0.1", port, 8, opts, host.NewLogger(false))

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			runDone := make(chan error, 1)
			go func() { runDone <- s.Run(ctx, script) }()

			base := fmt.Sprintf("http://127.0.0.1:%d", port)
			waitUp(t, base)

			connCtx, connCancel := context.WithCancel(context.Background())
			defer connCancel()

			url := fmt.Sprintf("ws://127.0.0.1:%d/ws/ui?script=%s", port, script)
			if tc.queryToken != "" {
				url += "&token=" + tc.queryToken
			}

			dialOpts := &websocket.DialOptions{
				HTTPHeader: http.Header{"Origin": []string{"http://127.0.0.1:" + fmt.Sprint(port)}},
			}
			if tc.headerToken != "" {
				dialOpts.HTTPHeader.Set("X-Kalua-Token", tc.headerToken)
			}

			ws, resp, err := websocket.Dial(connCtx, url, dialOpts)
			if tc.wantAccept {
				if err != nil {
					t.Fatalf("ws dial: %v (resp: %v)", err, resp)
				}
				ws.Close(websocket.StatusNormalClosure, "")
			} else {
				if err == nil {
					t.Fatalf("expected ws dial to fail, but it succeeded")
					ws.Close(websocket.StatusNormalClosure, "")
				}
				if resp != nil && resp.StatusCode != http.StatusUnauthorized {
					t.Logf("expected 401, got %d", resp.StatusCode)
				}
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
		})
	}
}

// TestServer_FetchAuthCheck verifies that the client fetch check works.
func TestServer_FetchAuthCheck(t *testing.T) {
	port := freePort(t)
	dir := t.TempDir()
	script := filepath.Join(dir, "app.lua")
	writeScript(t, script, `
function main()
  k.form.new("f", {})
  k.ctrl.label("f", "lbl", {text = "test"})
  k.form.show("f")
end
`)

	tests := []struct {
		name       string
		token      string
		wantStatus int
	}{
		{
			name:       "no auth token, fetch returns 200",
			token:      "",
			wantStatus: http.StatusOK,
		},
		{
			name:       "auth token configured, no token provided, fetch returns 401",
			token:      "secret123",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "auth token configured, correct token provided, fetch returns 200",
			token:      "secret123",
			wantStatus: http.StatusOK, // with query param
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			opts := bindings.Options{}
			if tc.token != "" {
				opts.AuthToken = tc.token
			}
			s := NewServer("127.0.0.1", port, 8, opts, host.NewLogger(false))

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			runDone := make(chan error, 1)
			go func() { runDone <- s.Run(ctx, script) }()

			base := fmt.Sprintf("http://127.0.0.1:%d", port)
			waitUp(t, base)

			url := fmt.Sprintf("%s/ws/ui?script=%s", base, script)
			if tc.token != "" && tc.wantStatus == http.StatusOK {
				url += "&token=" + tc.token
			}

			req, _ := http.NewRequest("GET", url, nil)
			req.Header.Set("Origin", "http://127.0.0.1")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("fetch failed: %v", err)
			}
			resp.Body.Close()

			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("expected status %d, got %d", tc.wantStatus, resp.StatusCode)
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
		})
	}
}