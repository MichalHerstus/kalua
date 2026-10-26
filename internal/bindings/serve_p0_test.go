package bindings

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"kalua/internal/vm"
)

// TestServeJSONAndXMLGroups verifies Phase 0.1: k.json_* and k.xml_* are
// registered by SetupServe (previously run-mode only).
func TestServeJSONAndXMLGroups(t *testing.T) {
	store := newFakeStore()
	src := `
local function check(cond, msg)
  if not cond then error("FAIL: " .. msg) end
end
local o = k.json_parse('{"a":1,"b":[true,"x"]}')
check(o.a == 1 and o.b[1] == true and o.b[2] == "x", "json_parse")
check(k.json_string({x=1}) == '{"x":1}', "json_string")
check(k.json_count(k.json_parse('[1,2,3]')) == 3, "json_count")
local x = k.xml_parse("<doc><a>1</a></doc>")
check(k.xml_content(x, "a") == "1", "xml_parse/content")
`
	if err := runServeLua(t, store, src); err != nil {
		t.Fatalf("json/xml groups in serve mode failed: %v", err)
	}
}

// TestServeFlowSubset verifies Phase 0.6: yield/error/param/net/locale/ping
// are available in serve mode without a session.
func TestServeFlowSubset(t *testing.T) {
	store := newFakeStore()
	src := `
local function check(cond, msg)
  if not cond then error("FAIL: " .. msg) end
end
k.param_set("k", "v")
check(k.param_get("k") == "v", "param_set/get")
local loc = k.locale()
check(loc ~= nil and loc ~= "", "locale")
check(type(k.net_ok) == "function", "net_ok present")
check(type(k.ping) == "function", "ping present")
check(type(k.yield) == "function", "yield present")
check(type(k.error) == "function", "error present")
`
	if err := runServeLua(t, store, src); err != nil {
		t.Fatalf("flow subset in serve mode failed: %v", err)
	}
}

// TestServeQuitCallback verifies Phase 0.5: k.quit invokes opts.Quit
// (the serve command wires it to stop the whole server).
func TestServeQuitCallback(t *testing.T) {
	store := newFakeStore()
	L := vm.New()
	defer L.Close()
	app := vm.NewApp(L)
	quitCalled := false
	SetupServe(L, app, Options{Quit: func() { quitCalled = true }}, store, nil, nil, nil)
	if err := L.DoString(`k.quit()`); err != nil {
		t.Fatalf("k.quit failed: %v", err)
	}
	if !quitCalled {
		t.Error("k.quit did not call opts.Quit in serve mode")
	}
}

// TestServeSleepBlocks verifies Phase 0.2: k.sleep actually blocks in serve
// mode (previously a no-op stub).
func TestServeSleepBlocks(t *testing.T) {
	store := newFakeStore()
	L := vm.New()
	defer L.Close()
	app := vm.NewApp(L)
	SetupServe(L, app, Options{}, store, nil, nil, nil)
	start := time.Now()
	if err := L.DoString(`k.sleep(80)`); err != nil {
		t.Fatalf("k.sleep failed: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 60*time.Millisecond {
		t.Errorf("k.sleep returned after %v, want >= 60ms (blocking)", elapsed)
	}
}

// TestServeHTTPResponseHelpers verifies the Phase 1 k.http_response_* helpers.
func TestServeHTTPResponseHelpers(t *testing.T) {
	store := newFakeStore()
	src := `
local function check(cond, msg)
  if not cond then error("FAIL: " .. msg) end
end
local j = k.http_response_json({ok=true}, 201)
check(j.status == 201, "json status")
check(j.headers["content-type"] == "application/json", "json content-type")
local t2 = k.http_response_text("hello", 200)
check(t2.body == "hello" and t2.headers["content-type"] == "text/plain", "text")
local r = k.http_response_redirect("/login")
check(r.status == 302 and r.headers.location == "/login", "redirect")
`
	if err := runServeLua(t, store, src); err != nil {
		t.Fatalf("http response helpers failed: %v", err)
	}
}

// TestServeHTTPStats reports zeroed stats when no server stats are wired.
func TestServeHTTPStats(t *testing.T) {
	store := newFakeStore()
	src := `
local function check(cond, msg)
  if not cond then error("FAIL: " .. msg) end
end
local s = k.http_stats()
check(s.requests_total == 0 and s.avg_latency_ms == 0, "zeroed stats")
`
	if err := runServeLua(t, store, src); err != nil {
		t.Fatalf("http_stats failed: %v", err)
	}
}

// TestServeHTTPRequestBlocking verifies Phase 0.1: the serve-mode k.http_request
// performs a real blocking HTTP call and returns {status, headers, body}.
func TestServeHTTPRequestBlocking(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Test", "yes")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	store := newFakeStore()
	src := `
local function check(cond, msg)
  if not cond then error("FAIL: " .. msg) end
end
local resp = k.http_request{
  method = "GET",
  url = %q,
  timeout = 5000,
}
check(resp.status == 201, "status")
check(resp.headers["X-Test"] == "yes", "header")
check(resp.body:find("ok") ~= nil, "body")
`
	if err := runServeLua(t, store, fmt.Sprintf(src, srv.URL)); err != nil {
		t.Fatalf("http_request in serve mode failed: %v", err)
	}
}

// TestServeMultipart verifies k.http_request_parse_multipart parses a
// multipart body into {fields, files}.
func TestServeMultipart(t *testing.T) {
	store := newFakeStore()
	src := `
local function check(cond, msg)
  if not cond then error("FAIL: " .. msg) end
end
local body = "--B\r\nContent-Disposition: form-data; name=\"name\"\r\n\r\nJohn\r\n" ..
             "--B\r\nContent-Disposition: form-data; name=\"up\"; filename=\"a.txt\"\r\n" ..
             "Content-Type: text/plain\r\n\r\nhello\r\n--B--\r\n"
local req = {headers = {["content-type"]="multipart/form-data; boundary=B"}, body = body}
local mp = k.http_request_parse_multipart(req)
check(mp.fields.name == "John", "field parsed")
check(mp.files.up.filename == "a.txt", "file name")
check(mp.files.up.content_type == "text/plain", "file content type")
check(mp.files.up.data == "hello", "file data")
`
	if err := runServeLua(t, store, src); err != nil {
		t.Fatalf("multipart parse failed: %v", err)
	}
}

// TestServeAuthMiddleware verifies k.http_auth_middleware appends to the
// k.http_middleware chain.
func TestServeAuthMiddleware(t *testing.T) {
	store := newFakeStore()
	src := `
local function check(cond, msg)
  if not cond then error("FAIL: " .. msg) end
end
k.http_auth_middleware("auth_mw")
check(type(k.http_middleware) == "table", "chain created")
check(#k.http_middleware == 1, "one middleware appended")
check(k.http_middleware[1] == "auth_mw", "name stored")
`
	if err := runServeLua(t, store, src); err != nil {
		t.Fatalf("http_auth_middleware failed: %v", err)
	}
}
