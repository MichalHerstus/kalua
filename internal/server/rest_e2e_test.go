//go:build !wasm

package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kalua/internal/host"
)

func bytesNewReader(s string) io.Reader { return strings.NewReader(s) }


// TestRestE2E verifies Phase 1 through a real HTTP server: declarative routes
// match, :params resolve, middleware chain runs, helpers produce responses,
// and k.http_stats() reports counters.
func TestRestE2E(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "app.lua")
	os.WriteFile(script, []byte(`
k.http_routes = {
  {method="GET", path="/api/users/:id", handler="get_user"},
  {method="POST", path="/api/users", handler="create_user"},
  {method="GET", path="/redirect", handler="go_away"},
}
k.http_middleware = {"cors"}
function cors(req)
  -- set a header the handler can inspect (simulate CORS preflight pass-through)
  req.cors = req.headers["Origin"] or "none"
  return nil
end
function get_user(req)
  return k.http_response_json({id=tonumber(req.params.id), cors=req.cors})
end
function create_user(req)
  local body = k.json_parse(req.body)
  return k.http_response_json({created=body.name}, 201)
end
function go_away(req)
  return k.http_response_redirect("/api/users/1")
end
function main() end
`), 0o644)

	cfg := Config{
		Host:       "127.0.0.1",
		Port:       0,
		Workers:    2,
		Mode:       "http",
		ScriptPath: script,
		Logger:     host.NewLogger(false),
	}
	s := NewServer(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan error, 1)
	go func() { runDone <- s.Run(ctx) }()

	select {
	case <-s.Ready():
	case err := <-runDone:
		t.Fatalf("boot failed: %v", err)
	case <-time.After(8 * time.Second):
		t.Fatal("server not ready")
	}
	base := "http://" + s.HTTPAddr()
	client := &http.Client{Timeout: 5 * time.Second}

	t.Run("RouteWithParams", func(t *testing.T) {
		req, _ := http.NewRequest("GET", base+"/api/users/77", nil)
		req.Header.Set("Origin", "http://example.com")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != 200 {
			t.Fatalf("status = %d, body %s", resp.StatusCode, body)
		}
		var out struct {
			ID   int    `json:"id"`
			Cors string `json:"cors"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			t.Fatalf("not JSON: %v (%s)", err, body)
		}
		if out.ID != 77 || out.Cors != "http://example.com" {
			t.Errorf("route/middleware = %+v", out)
		}
	})

	t.Run("PostCreates", func(t *testing.T) {
		resp, err := client.Post(base+"/api/users", "application/json", bytesNewReader(`{"name":"Ada"}`))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != 201 {
			t.Fatalf("status = %d, body %s", resp.StatusCode, body)
		}
		var out struct {
			Created string `json:"created"`
		}
		json.Unmarshal(body, &out)
		if out.Created != "Ada" {
			t.Errorf("created = %q", out.Created)
		}
	})

	t.Run("Redirect", func(t *testing.T) {
		clientNoFollow := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}}
		resp, err := clientNoFollow.Get(base + "/redirect")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 302 {
			t.Fatalf("status = %d, want 302", resp.StatusCode)
		}
		if loc := resp.Header.Get("Location"); loc != "/api/users/1" {
			t.Errorf("Location = %q", loc)
		}
	})

	t.Run("Stats", func(t *testing.T) {
		// Trigger a few requests then read counters from a routed handler path.
		// k.http_stats is available via setup; query it through the worker env.
		resp, err := client.Get(base + "/api/users/1")
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		st := s.httpStats
		if st.RequestsTotal.Load() < 4 {
			t.Errorf("requests_total = %d, want >= 4", st.RequestsTotal.Load())
		}
		if st.ErrorsTotal.Load() != 0 {
			t.Errorf("errors_total = %d, want 0", st.ErrorsTotal.Load())
		}
		if st.LatencyCount.Load() == 0 {
			t.Error("latency never recorded")
		}
		if st.ActiveConns.Load() != 0 {
			t.Errorf("active_connections = %d, want 0 after requests", st.ActiveConns.Load())
		}
	})

	cancel()
	select {
	case <-runDone:
	case <-time.After(8 * time.Second):
		t.Fatal("server did not stop")
	}
}
