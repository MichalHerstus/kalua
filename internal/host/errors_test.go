package host

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestRun_OnErrorHook verifies the Kalipso catch+continue error path in run
// (--test) mode: a failing binding returns nil, sets ERRORCODE/ERRORMSG, and
// fires the k.on_error hook.
func TestRun_OnErrorHook(t *testing.T) {
	tmp := t.TempDir()
	outside := t.TempDir()
	script := filepath.Join(tmp, "onerr.lua")
	src := fmt.Sprintf(`
function main()
  local calls = 0
  local hookCode = nil
  local hookMsg = nil
  k.on_error(function(code, msg)
    calls = calls + 1
    hookCode = code
    hookMsg = msg
  end)

  local d = k.file_load(%q .. "/secret.txt")
  if d ~= nil then error("file_load should return nil on error, got " .. tostring(d)) end
  if calls ~= 1 then error("hook called " .. tostring(calls) .. " times") end
  if ERRORCODE ~= -20 then error("expected code -20, got " .. tostring(ERRORCODE)) end
  if hookCode ~= -20 or hookMsg == "" then error("bad hook args") end

  k.quit()
end
`, outside)
	if err := os.WriteFile(script, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	log := NewLogger(false)
	var buf bytes.Buffer
	cfg := RunConfig{ScriptPath: script, AllowFS: []string{tmp}, Logger: log, Out: &buf}
	if code := Run(cfg); code != ExitOK {
		t.Errorf("Run = %d, want %d\noutput:\n%s", code, ExitOK, buf.String())
	}
}
