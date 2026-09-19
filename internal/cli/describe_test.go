package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kalua/internal/describe"
	"kalua/internal/host"
)

func TestDescribeCmd_RunFormApp(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "app.lua")
	os.WriteFile(script, []byte(`
function main()
	k.form.new("main", {title = "Hello"})
	k.ctrl.label("main", "greeting", {text = "hi"})
	k.ctrl.button("main", "go", {text = "Go"})
	k.form.show("main")
	k.quit()
end
`), 0o644)

	var out strings.Builder
	code := captureRun(t, &out, []string{"describe", script, "--json"})
	if code != int(host.ExitOK) {
		t.Fatalf("describe: exit = %d\n%s", code, out.String())
	}
	var res describe.Result
	if err := json.Unmarshal([]byte(out.String()), &res); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out.String())
	}
	if res.Entry != "run" || !res.Main {
		t.Errorf("entry=%q main=%v, want run/true", res.Entry, res.Main)
	}
	if len(res.Forms) != 2 || res.Forms[0].Form != "main" || res.Forms[0].Op != "new" {
		t.Errorf("forms = %+v, want [main(new) main(show)]", res.Forms)
	}
	if res.Forms[0].Title != "Hello" {
		t.Errorf("form title = %q, want Hello", res.Forms[0].Title)
	}
	for _, want := range []string{"k.form.new", "k.ctrl.label", "k.ctrl.button", "k.form.show", "k.quit"} {
		if res.KUsage[want] == 0 {
			t.Errorf("k_usage[%q] = 0, want >= 1 (full map: %+v)", want, res.KUsage)
		}
	}
	if res.KCallCount < 5 {
		t.Errorf("k_calls = %d, want >= 5", res.KCallCount)
	}
}

func TestDescribeCmd_ServeApp(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "app.lua")
	os.WriteFile(script, []byte(`
function init() end
function handle_http(req) return "ok" end
function handle_ws(msg) return "echo" end
function shutdown() end
`), 0o644)

	var out strings.Builder
	code := captureRun(t, &out, []string{"describe", script, "--json"})
	if code != int(host.ExitOK) {
		t.Fatalf("describe: exit = %d\n%s", code, out.String())
	}
	var res describe.Result
	if err := json.Unmarshal([]byte(out.String()), &res); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out.String())
	}
	if res.Entry != "serve" {
		t.Errorf("entry = %q, want serve", res.Entry)
	}
	joined := strings.Join(res.Handlers, ",")
	for _, want := range []string{"init", "handle_http", "handle_ws", "shutdown"} {
		if !strings.Contains(joined, want) {
			t.Errorf("handlers missing %s: %v", want, res.Handlers)
		}
	}
}

func TestDescribeCmd_HumanOutput(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "app.lua")
	os.WriteFile(script, []byte(`function main() k.print("x") k.quit() end`), 0o644)

	var out strings.Builder
	code := captureRun(t, &out, []string{"describe", script})
	if code != int(host.ExitOK) {
		t.Fatalf("describe: exit = %d\n%s", code, out.String())
	}
	s := out.String()
	if !strings.Contains(s, "run-mode app") || !strings.Contains(s, "main()") ||
		!strings.Contains(s, "k.print") || !strings.Contains(s, "k.quit") {
		t.Errorf("human output missing key parts:\n%s", s)
	}
}

func TestDescribeCmd_CheckError(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "bad.lua")
	os.WriteFile(script, []byte(`function main() k.unknown_one() end`), 0o644)

	var out strings.Builder
	code := captureRun(t, &out, []string{"describe", script, "--json"})
	if code != int(host.ExitError) {
		t.Fatalf("describe bad: exit = %d, want %d", code, host.ExitError)
	}
	var res describe.Result
	if err := json.Unmarshal([]byte(out.String()), &res); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out.String())
	}
	if res.OK || !strings.Contains(res.Error, "k.unknown_one") {
		t.Errorf("expected unknown-k error, got ok=%v error=%q", res.OK, res.Error)
	}
}