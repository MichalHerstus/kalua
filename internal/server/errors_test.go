package server

import (
	"context"
	"strings"
	"testing"

	"github.com/yuin/gopher-lua"
)

// TestServe_OnError_HookFiresOnGenuineError verifies that a Lua error aborting
// a handle_http handler still invokes the k.on_error hook (via Worker.notifyError).
func TestServe_OnError_HookFiresOnGenuineError(t *testing.T) {
	w := newTestWorker(t, `
calls = 0
hookCode = nil
hookMsg = ""
function main() end

k.on_error(function(code, msg)
  calls = calls + 1
  hookCode = code
  hookMsg = msg
end)

function handle_http(req)
  error("boom in handler")
end
`)

	ctx := context.Background()
	resp, err := w.CallHTTP(ctx, HTTPRequest{Path: "/"})
	if err == nil {
		t.Fatalf("expected error, got resp=%+v", resp)
	}
	if got := w.L.GetGlobal("calls"); got != lua.LNumber(1) {
		t.Fatalf("hook calls = %v, want 1", got)
	}
	if got := w.L.GetGlobal("hookCode"); got != lua.LNumber(-1) {
		t.Fatalf("hook code = %v, want -1 (KErrorGeneric)", got)
	}
	if got := w.L.GetGlobal("hookMsg"); !strings.Contains(got.String(), "boom in handler") {
		t.Fatalf("hook msg = %v, want boom in handler", got)
	}
}

// TestServe_OnError_CatchContinueInBinding verifies the catch+continue path in
// serve mode: a failing binding returns nil, sets ERRORCODE/ERRORMSG, and the
// handler can branch on the globals and complete normally.
func TestServe_OnError_CatchContinueInBinding(t *testing.T) {
	w := newTestWorker(t, `
function main() end

function handle_http(req)
  local d = k.file_load("/untrusted/secret.txt")
  if d ~= nil then return {status = 500, body = "nil expected, got " .. tostring(d)} end
  if ERRORCODE ~= -20 then return {status = 500, body = "code " .. tostring(ERRORCODE)} end
  if ERRORMSG == "" then return {status = 500, body = "no message"} end
  return {status = 200, body = "handled"}
end
`)

	ctx := context.Background()
	resp, err := w.CallHTTP(ctx, HTTPRequest{Path: "/"})
	if err != nil {
		t.Fatalf("CallHTTP: %v", err)
	}
	if resp.Status != 200 || resp.Body != "handled" {
		t.Fatalf("resp = %+v, want 200 handled", resp)
	}
}
