package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kalua/internal/format"
	"kalua/internal/host"
)

func TestTestCmd_RunMode(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "app.lua")
	os.WriteFile(script, []byte(`function main() k.print("hi") k.quit() end`), 0o644)

	var out strings.Builder
	code := captureRun(t, &out, []string{"test", script, "--json"})
	if code != int(host.ExitOK) {
		t.Fatalf("test run-mode: exit = %d, want %d\n%s", code, host.ExitOK, out.String())
	}
	var res struct {
		OK        bool `json:"ok"`
		Formatted bool `json:"formatted"`
		Mode      string
		Run       struct {
			Output string `json:"output"`
			OK     bool   `json:"ok"`
		} `json:"run"`
	}
	if err := json.Unmarshal([]byte(out.String()), &res); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out.String())
	}
	if !res.OK || res.Mode != "run" {
		t.Errorf("ok=%v mode=%q, want ok=true mode=run", res.OK, res.Mode)
	}
	if !strings.Contains(res.Run.Output, "hi") {
		t.Errorf("run output = %q, want 'hi'", res.Run.Output)
	}
}

func TestTestCmd_ServeMode_Detected(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "app.lua")
	os.WriteFile(script, []byte(`function main() end
function handle_http(req) return "hello" end
function handle_ws(msg) return "echo" end
`), 0o644)

	var out strings.Builder
	code := captureRun(t, &out, []string{"test", script, "--json"})
	if code != int(host.ExitOK) {
		t.Fatalf("test serve-mode: exit = %d, want %d\n%s", code, host.ExitOK, out.String())
	}
	var res struct {
		OK    bool   `json:"ok"`
		Mode  string `json:"mode"`
		Serve struct {
			InitOK bool `json:"init_ok"`
		} `json:"serve"`
	}
	if err := json.Unmarshal([]byte(out.String()), &res); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out.String())
	}
	if !res.OK || res.Mode != "serve" || !res.Serve.InitOK {
		t.Errorf("ok=%v mode=%q init_ok=%v, want serve-mode all-true", res.OK, res.Mode, res.Serve.InitOK)
	}
}

func TestTestCmd_Check_Errors(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "bad.lua")
	os.WriteFile(script, []byte(`function main() k.nope() end`), 0o644)

	var out strings.Builder
	code := captureRun(t, &out, []string{"test", script, "--json"})
	if code != int(host.ExitError) {
		t.Fatalf("test bad script: exit = %d, want %d", code, host.ExitError)
	}
	var res struct {
		OK     bool `json:"ok"`
		Issues []struct {
			Message string `json:"message"`
		} `json:"issues"`
	}
	if err := json.Unmarshal([]byte(out.String()), &res); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out.String())
	}
	if res.OK || len(res.Issues) == 0 || !strings.Contains(res.Issues[0].Message, "k.nope") {
		t.Errorf("expected unknown-k.* issue, got %+v", res)
	}
}

func TestTestCmd_Mainless(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "mainless.lua")
	os.WriteFile(script, []byte(`local x = 1`), 0o644)

	var out strings.Builder
	code := captureRun(t, &out, []string{"test", script, "--json"})
	if code != int(host.ExitError) {
		t.Fatalf("test mainless: exit = %d, want %d", code, host.ExitError)
	}
	var res struct {
		OK     bool `json:"ok"`
		Issues []struct {
			Message string `json:"message"`
		} `json:"issues"`
	}
	if err := json.Unmarshal([]byte(out.String()), &res); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out.String())
	}
	if res.OK || len(res.Issues) == 0 || !strings.Contains(res.Issues[0].Message, "entry point") {
		t.Errorf("expected missing-entry-point issue, got %+v", res)
	}
}

func TestTestCmd_FormattedFlag(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "app.lua")
	os.WriteFile(script, []byte(`function main() k.print("hi") k.quit() end`), 0o644)

	// Unformatted → formatted=false (but still runs).
	var out strings.Builder
	code := captureRun(t, &out, []string{"test", script, "--json"})
	if code != int(host.ExitOK) {
		t.Fatalf("test unformatted: exit = %d", code)
	}
	var res struct {
		Formatted bool `json:"formatted"`
	}
	json.Unmarshal([]byte(out.String()), &res)
	if res.Formatted {
		t.Fatal("expected formatted=false for unformatted script")
	}

	// Formatted → formatted=true.
	src, _ := os.ReadFile(script)
	fmtd, err := format.Format(src, script)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(script, fmtd, 0o644)
	out.Reset()
	code = captureRun(t, &out, []string{"test", script, "--json"})
	if code != int(host.ExitOK) {
		t.Fatalf("test formatted: exit = %d\n%s", code, out.String())
	}
	json.Unmarshal([]byte(out.String()), &res)
	if !res.Formatted {
		t.Fatal("expected formatted=true for formatted script")
	}
}

func TestTestCmd_HumanOutput(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "app.lua")
	os.WriteFile(script, []byte(`function main() k.quit() end`), 0o644)

	var out strings.Builder
	captureRun(t, &out, []string{"test", script})
	if !strings.Contains(out.String(), "PASS (mode=run") {
		t.Errorf("human output = %q, want PASS line", out.String())
	}
}

// captureRun runs fn with stdout/stderr captured into out (for --json
// assertions) or a throwaway buffer.
func captureRun(t *testing.T, out *strings.Builder, args []string) int {
	t.Helper()
	origOut, origErr := os.Stdout, os.Stderr
	r, w, _ := os.Pipe()
	os.Stdout = w
	os.Stderr = w
	code := Run(args)
	w.Close()
	os.Stdout, os.Stderr = origOut, origErr
	var buf bytes.Buffer
	io.Copy(&buf, r)
	r.Close()
	if out != nil {
		out.WriteString(buf.String())
	}
	return code
}