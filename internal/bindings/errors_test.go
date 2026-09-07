package bindings

import (
	"strings"
	"testing"
)

// failPath is outside every file root, so k.file_open always fails with
// KErrorPermission (-20) through the catch+continue path.
const failPath = "/non/existent/x.txt"

func TestOnError_HookFiresWithCodeAndMessage(t *testing.T) {
	src := `
local calls = 0
k.on_error(function(code, msg)
  calls = calls + 1
  if code ~= -20 then error("wrong code " .. tostring(code)) end
  if msg == "" then error("empty msg") end
end)

local f = k.file_open("` + failPath + `", "r")
if f ~= nil then error("expected nil handle") end
if calls ~= 1 then error("hook not called once: " .. tostring(calls)) end
if ERRORCODE ~= -20 then error("ERRORCODE not set: " .. tostring(ERRORCODE)) end
if ERRORMSG == "" then error("ERRORMSG not set") end
`
	if err := runLua(t, src); err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestOnError_ClearWithNil(t *testing.T) {
	src := `
local calls = 0
k.on_error(function(code, msg) calls = calls + 1 end)
local f = k.file_open("` + failPath + `", "r")
if f ~= nil then error("expected nil handle") end
if calls ~= 1 then error("hook not called: " .. tostring(calls)) end

k.on_error(nil)
local f2 = k.file_open("` + failPath + `", "r")
if f2 ~= nil then error("expected nil handle") end
if calls ~= 1 then error("hook ran after clear") end
if ERRORCODE ~= -20 then error("globals lost after clear") end
`
	if err := runLua(t, src); err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestOnError_RecursionGuard(t *testing.T) {
	src := `
local calls = 0
k.on_error(function(code, msg)
  calls = calls + 1
  local x = k.file_open("` + failPath + `", "r")
  if x ~= nil then error("inner fail should return nil") end
end)

local f = k.file_open("` + failPath + `", "r")
if f ~= nil then error("expected nil handle") end
if calls ~= 1 then error("hook recursed " .. tostring(calls) .. " times") end
if ERRORMSG == "" then error("inner fail should still set ERRORMSG") end
`
	if err := runLua(t, src); err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestOnError_HookErrorIsSwallowed(t *testing.T) {
	src := `
local calls = 0
k.on_error(function(code, msg)
  calls = calls + 1
  error("hook blew up")
end)

local f = k.file_open("` + failPath + `", "r")
if f ~= nil then error("expected nil handle") end
if calls ~= 1 then error("hook not called once: " .. tostring(calls)) end
if ERRORCODE ~= -20 then error("ERRORCODE not set despite hook error") end
`
	if err := runLua(t, src); err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestOnError_InvalidArgRejected(t *testing.T) {
	err := runLua(t, `k.on_error("not a function")`)
	if err == nil || !strings.Contains(err.Error(), "expected a function or nil") {
		t.Fatalf("expected function-or-nil error, got %v", err)
	}
}
