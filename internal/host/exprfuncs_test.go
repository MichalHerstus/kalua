package host

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestRun_ExprFuncsFlow runs the §5.9 expression-function globals from a real
// app in headless test mode, proving they are installed in the run-mode VM.
func TestRun_ExprFuncsFlow(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "expr.lua")
	src := `function main()
  if upper("abc") ~= "ABC" then k.error("upper") end
  if left("hello", 2) ~= "he" then k.error("left") end
  if round(2.5678, 2) ~= 2.57 then k.error("round") end
  if abs(-7) ~= 7 then k.error("abs") end
  if iif(true, "y", "n") ~= "y" then k.error("iif") end
  if lookup("k", "a", 1, "k", 9) ~= 9 then k.error("lookup") end
  local today = sys_date()
  if #today ~= 10 then k.error("sys_date") end
  if add_days("2024-01-31", 1) ~= "2024-02-01" then k.error("add_days") end
  if tostr(42) ~= "42" then k.error("tostr") end
  if tonum("3.5") ~= 3.5 then k.error("tonum") end
  if boolstr(0) ~= "false" then k.error("boolstr") end
  if base64_decode(base64_encode("kalua")) ~= "kalua" then k.error("base64") end
  if jsonencode({n=1}) ~= '{"n":1}' then k.error("jsonencode") end
  k.quit()
end
`
	if err := os.WriteFile(script, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	log := NewLogger(false)
	var buf bytes.Buffer
	cfg := RunConfig{ScriptPath: script, Logger: log, Out: &buf}
	code := Run(cfg)
	if code != ExitOK {
		t.Errorf("Run = %d, want ExitOK\noutput:\n%s", code, buf.String())
	}
}