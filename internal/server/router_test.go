//go:build !wasm

package server

import (
	"context"
	"strings"
	"testing"
)

// newServeLuaWorker reuses the existing newTestWorker helper from worker_test.go
// (which builds a worker from a script string) — declared here for clarity.

func TestRouterMatchAndDispatch(t *testing.T) {
	script := `
k.http_routes = {
  {method="GET", path="/api/users/:id", handler="get_user"},
  {method="POST", path="/api/users", handler="create_user"},
}
function get_user(req)
  return k.http_response_json({id=tonumber(req.params.id), name="John"})
end
function create_user(req)
  return k.http_response_text("created", 201)
end
function main() end
`
	w := newTestWorker(t, script)
	if w.router == nil {
		t.Fatal("router not built")
	}

	// Match + dispatch via CallHTTP.
	resp, err := w.CallHTTP(context.Background(), HTTPRequest{Method: "GET", Path: "/api/users/42"})
	if err != nil {
		t.Fatalf("CallHTTP: %v", err)
	}
	if resp.Status != 200 {
		t.Errorf("status = %d, want 200", resp.Status)
	}
	if !strings.Contains(resp.Body, `"id":42`) {
		t.Errorf("body = %s, want id:42", resp.Body)
	}
	if ct := strings.ToLower(resp.Headers["content-type"]); ct != "application/json" {
		t.Errorf("content-type = %q", ct)
	}

	resp, err = w.CallHTTP(context.Background(), HTTPRequest{Method: "POST", Path: "/api/users", Body: `{"n":"x"}`})
	if err != nil {
		t.Fatalf("CallHTTP POST: %v", err)
	}
	if resp.Status != 201 || resp.Body != "created" {
		t.Errorf("POST = %d/%q, want 201/created", resp.Status, resp.Body)
	}

	// Unmatched path falls back to handle_http (absent → 404).
	resp, err = w.CallHTTP(context.Background(), HTTPRequest{Method: "GET", Path: "/nope"})
	if err != nil {
		t.Fatalf("CallHTTP unmatched: %v", err)
	}
	if resp.Status != 404 {
		t.Errorf("unmatched status = %d, want 404", resp.Status)
	}
}

func TestRouterMiddlewareChain(t *testing.T) {
	script := `
k.http_routes = {
  {method="GET", path="/secret", handler="handler"},
}
k.http_middleware = {"auth_mw"}
function auth_mw(req)
  if not req.headers["authorization"] then
    return k.http_response_json({error="unauthorized"}, 401)
  end
  req.user = "alice"
  return nil
end
function handler(req)
  return k.http_response_json({user=req.user})
end
function main() end
`
	w := newTestWorker(t, script)

	// No auth header → middleware short-circuits with 401.
	resp, _ := w.CallHTTP(context.Background(), HTTPRequest{Method: "GET", Path: "/secret"})
	if resp.Status != 401 {
		t.Errorf("unauthorized status = %d, want 401", resp.Status)
	}
	if !strings.Contains(resp.Body, "unauthorized") {
		t.Errorf("unauthorized body = %s", resp.Body)
	}

	// With auth header → middleware attaches req.user, handler sees it.
	resp, _ = w.CallHTTP(context.Background(), HTTPRequest{
		Method:  "GET",
		Path:    "/secret",
		Headers: map[string]string{"authorization": "tok"},
	})
	if resp.Status != 200 {
		t.Errorf("authorized status = %d, want 200", resp.Status)
	}
	if !strings.Contains(resp.Body, `"user":"alice"`) {
		t.Errorf("authorized body = %s, want user alice", resp.Body)
	}
}

func TestRouterFuncMiddleware(t *testing.T) {
	// Middleware registered via k.http_auth_middleware(fn) runs first.
	script := `
k.http_routes = { {method="GET", path="/x", handler="h"} }
k.http_auth_middleware(function(req)
  if req.headers["x-block"] then
    return k.http_response_text("blocked", 403)
  end
  return nil
end)
function h(req) return k.http_response_text("ok") end
function main() end
`
	w := newTestWorker(t, script)
	resp, _ := w.CallHTTP(context.Background(), HTTPRequest{Method: "GET", Path: "/x", Headers: map[string]string{"x-block": "1"}})
	if resp.Status != 403 {
		t.Errorf("blocked status = %d, want 403", resp.Status)
	}
	resp, _ = w.CallHTTP(context.Background(), HTTPRequest{Method: "GET", Path: "/x"})
	if resp.Status != 200 || resp.Body != "ok" {
		t.Errorf("open request = %d/%q, want 200/ok", resp.Status, resp.Body)
	}
}

func TestRouterNoRoutesFallsBackToHandleHTTP(t *testing.T) {
	script := `
function handle_http(req)
  return {status=200, body="legacy:" .. req.path}
end
function main() end
`
	w := newTestWorker(t, script)
	if w.router != nil {
		t.Fatal("router should be nil without k.http_routes")
	}
	resp, err := w.CallHTTP(context.Background(), HTTPRequest{Method: "GET", Path: "/legacy"})
	if err != nil {
		t.Fatalf("CallHTTP: %v", err)
	}
	if resp.Status != 200 || resp.Body != "legacy:/legacy" {
		t.Errorf("legacy fallback = %d/%q", resp.Status, resp.Body)
	}
}

func TestRouterHandlerError(t *testing.T) {
	script := `
k.http_routes = { {method="GET", path="/boom", handler="boom"} }
function boom() error("kaboom") end
function main() end
`
	w := newTestWorker(t, script)
	resp, err := w.CallHTTP(context.Background(), HTTPRequest{Method: "GET", Path: "/boom"})
	if err == nil {
		t.Errorf("expected handler error, got status %d", resp.Status)
	}
	if resp.Status != 500 {
		t.Errorf("error status = %d, want 500", resp.Status)
	}
}

func TestRouterQueryParamsPassed(t *testing.T) {
	script := `
k.http_routes = { {method="GET", path="/q", handler="h"} }
function h(req)
  local q = req.query
  return k.http_response_json({a=q.a, b=q.b})
end
function main() end
`
	w := newTestWorker(t, script)
	resp, _ := w.CallHTTP(context.Background(), HTTPRequest{Method: "GET", Path: "/q", Query: "a=1&b=x", QueryValues: map[string][]string{"a": {"1"}, "b": {"x"}}})
	if !strings.Contains(resp.Body, `"a":"1"`) || !strings.Contains(resp.Body, `"b":"x"`) {
		t.Errorf("query passed = %s", resp.Body)
	}
}
