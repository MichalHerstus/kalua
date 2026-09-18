package server

import (
	"os"
	"path/filepath"
	"testing"
)

func writeScript(t *testing.T, dir, name, src string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func httpProbe(path, want string) SmokeProbe {
	return SmokeProbe{Method: "GET", Path: path, WantContent: want}
}

func TestSmokeTest_HelloHTTP(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "app.lua", `
function main() end
function init(config)
  k.shared.set("started", config.mode)
end
function shutdown()
  k.shared.set("shutdown", "yes")
end
function handle_http(req)
  if req.path == "/hello" then
    return "world"
  end
  return {json = {mode = k.shared.get("started")}}
end
`)
	cfg := Config{Mode: "http", ScriptPath: script, Workers: 1}
	res := SmokeTest(cfg, SmokeOptions{HTTPProbes: []SmokeProbe{
		{Method: "GET", Path: "/hello", WantContent: "world"},
		{Method: "GET", Path: "/json", WantJSON: map[string]any{"mode": "http"}},
	}})
	if !res.OK {
		t.Fatalf("smoke test failed: %v", res.Errors)
	}
	if !res.InitOK {
		t.Error("InitOK = false, want true")
	}
	if !res.ShutdownOK {
		t.Error("ShutdownOK = false, want true")
	}
	if len(res.HTTP) != 3 {
		t.Fatalf("HTTP probes = %d, want 3 (healthz + 2 user)", len(res.HTTP))
	}
	for _, p := range res.HTTP[1:] {
		if !p.OK {
			t.Errorf("probe %s %s: got %d, want %d", p.Method, p.Path, p.Status, p.Expected)
		}
	}
}

func TestSmokeTest_FailingProbe(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "app.lua", `
function main() end
function handle_http(req)
  return {status = 404, body = "not found"}
end
`)
	cfg := Config{Mode: "http", ScriptPath: script, Workers: 1}
	res := SmokeTest(cfg, SmokeOptions{HTTPProbes: []SmokeProbe{
		{Method: "GET", Path: "/missing", WantStatus: 200},
	}})
	if res.OK {
		t.Error("smoke test should have failed on 404 vs want 200")
	}
}

func TestSmokeTest_BadScript(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "bad.lua", `function main( this is broken`)
	cfg := Config{Mode: "http", ScriptPath: script, Workers: 1}
	res := SmokeTest(cfg, SmokeOptions{})
	if res.OK {
		t.Error("smoke test should have failed for broken script")
	}
	if len(res.Errors) == 0 {
		t.Error("expected at least one error for broken script")
	}
}

func TestSmokeTest_WS_TCP(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "app.lua", `
function main() end
function handle_http(req) return "ok" end
function handle_ws(msg)
  if msg.type == "text" then
    return "ws-echo:" .. msg.data
  end
end
function handle_tcp(msg)
  if msg.type == "text" then
    return "tcp-echo:" .. msg.data
  end
end
`)
	cfg := Config{Mode: "http,ws,tcp", ScriptPath: script, Workers: 1}
	res := SmokeTest(cfg, SmokeOptions{
		TCPSend: "ping",
		WSSend:  "ping",
	})
	if !res.OK {
		t.Fatalf("smoke test failed: %v", res.Errors)
	}
	if !res.WSEcho {
		t.Error("WSEcho = false, want true (handler echoes)")
	}
	if !res.TCPEcho {
		t.Error("TCPEcho = false, want true (handler echoes)")
	}
}

func TestSmokeTest_TCPOnly(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "app.lua", `
function main() end
function handle_tcp(msg)
  if msg.type == "text" then return "got:" .. msg.data end
end
`)
	cfg := Config{Mode: "tcp", ScriptPath: script, Workers: 1}
	res := SmokeTest(cfg, SmokeOptions{TCPSend: "hi"})
	if !res.OK {
		t.Fatalf("smoke test failed: %v", res.Errors)
	}
	if res.Mode != "tcp" {
		t.Errorf("Mode = %q, want tcp", res.Mode)
	}
}

func TestSmokeTest_ShutdownRan(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "app.lua", `
function main() end
function shutdown()
  k.shared.set("done", "true")
end
function handle_http(req)
  return k.shared.get("done") or "nil"
end
`)
	cfg := Config{Mode: "http", ScriptPath: script, Workers: 1}
	res := SmokeTest(cfg, SmokeOptions{})
	if !res.ShutdownOK {
		t.Error("ShutdownOK = false, want true after SmokeTest")
	}
}
