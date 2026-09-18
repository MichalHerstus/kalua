package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kalua/internal/host"
)

func TestRun_Hello(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "hello.lua")
	err := os.WriteFile(script, []byte(`function main() k.print("hi") k.quit() end`), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	code := Run([]string{"run", script, "--test"})
	if code != int(host.ExitOK) {
		t.Errorf("Run = %d, want %d", code, host.ExitOK)
	}
}

func TestRun_Check_OK(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "ok.lua")
	err := os.WriteFile(script, []byte(`function main() k.print("x") k.quit() end`), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	code := Run([]string{"check", script})
	if code != int(host.ExitOK) {
		t.Errorf("Run check = %d, want %d", code, host.ExitOK)
	}
}

func TestRun_Check_Bad(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "bad.lua")
	err := os.WriteFile(script, []byte(`function main() k.nope() end`), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	code := Run([]string{"check", script})
	if code != int(host.ExitError) {
		t.Errorf("Run check = %d, want %d", code, host.ExitError)
	}
}

func TestRun_New(t *testing.T) {
	tmp := t.TempDir()
	name := filepath.Join(tmp, "myapp")
	code := Run([]string{"new", name})
	if code != int(host.ExitOK) {
		t.Errorf("Run new = %d, want %d", code, host.ExitOK)
	}
	// verify file created
	if _, err := os.Stat(name + ".lua"); err != nil {
		t.Errorf("new did not create %s.lua: %v", name, err)
	}
}

func TestRun_Version(t *testing.T) {
	code := Run([]string{"version"})
	if code != int(host.ExitOK) {
		t.Errorf("Run version = %d, want %d", code, host.ExitOK)
	}
}

func TestRun_Ini_TestMode(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "app.lua")
	err := os.WriteFile(script, []byte(`function main() k.print("hi") k.quit() end`), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	ini := filepath.Join(tmp, "custom.ini")
	err = os.WriteFile(ini, []byte("[RUN]\ntest = 1\nverbose = 1\n"), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	code := Run([]string{"run", script, "--ini", ini})
	if code != int(host.ExitOK) {
		t.Errorf("Run = %d, want %d (INI should drive headless test mode)", code, host.ExitOK)
	}
}

func TestRun_Ini_ExplicitFlagWins(t *testing.T) {
	// A bad INI value for a flag that is explicitly set on the CLI must be
	// skipped (precedence: CLI flags > KALUA.INI), so no error occurs.
	tmp := t.TempDir()
	script := filepath.Join(tmp, "app.lua")
	err := os.WriteFile(script, []byte(`function main() k.print("hi") k.quit() end`), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	ini := filepath.Join(tmp, "bad.ini")
	err = os.WriteFile(ini, []byte("[RUN]\nport = not-a-port\ntest = 1\n"), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	code := Run([]string{"run", script, "--ini", ini, "--port", "1234"})
	if code != int(host.ExitOK) {
		t.Errorf("Run = %d, want %d (explicit --port must override bad INI value)", code, host.ExitOK)
	}
}

func TestRun_Ini_BadValueFromIni(t *testing.T) {
	// The bad INI value is not overridden on the CLI -> usage error.
	tmp := t.TempDir()
	script := filepath.Join(tmp, "app.lua")
	err := os.WriteFile(script, []byte(`function main() k.print("hi") k.quit() end`), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	ini := filepath.Join(tmp, "bad.ini")
	err = os.WriteFile(ini, []byte("[RUN]\nport = not-a-port\ntest = 1\n"), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	code := Run([]string{"run", script, "--ini", ini})
	if code != int(host.ExitUsage) {
		t.Errorf("Run = %d, want %d (unoverridden bad INI value must error)", code, host.ExitUsage)
	}
}

func TestRun_Ini_MissingExplicit(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "app.lua")
	err := os.WriteFile(script, []byte(`function main() k.quit() end`), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	code := Run([]string{"run", script, "--test", "--ini", filepath.Join(tmp, "nope.ini")})
	if code != int(host.ExitIOError) {
		t.Errorf("Run = %d, want %d (explicit missing --ini must error)", code, host.ExitIOError)
	}
}

func TestRun_Check_JSON(t *testing.T) {
	tmp := t.TempDir()
	okScript := filepath.Join(tmp, "ok.lua")
	_ = os.WriteFile(okScript, []byte(`function main() end`), 0o644)

	// Capture stdout + stderr for JSON parsing.
	var out bytes.Buffer
	origOut, origErr := os.Stdout, os.Stderr
	r, w, _ := os.Pipe()
	os.Stdout = w
	os.Stderr = w
	code := Run([]string{"check", okScript, "--json"})
	w.Close()
	os.Stdout, os.Stderr = origOut, origErr
	io.Copy(&out, r)
	r.Close()

	if code != int(host.ExitOK) {
		t.Fatalf("check --json ok: exit = %d, want %d", code, host.ExitOK)
	}
	var res struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("check --json output is not JSON: %v\nraw: %s", err, out.String())
	}
	if !res.OK {
		t.Error("json ok = false, want true")
	}
}

func TestRun_Check_JSON_Errors(t *testing.T) {
	tmp := t.TempDir()
	bad := filepath.Join(tmp, "bad.lua")
	os.WriteFile(bad, []byte(`function main() k.nope() end`), 0o644)
	var out bytes.Buffer
	origOut, origErr := os.Stdout, os.Stderr
	r, w, _ := os.Pipe()
	os.Stdout = w
	os.Stderr = w
	code := Run([]string{"check", bad, "--json"})
	w.Close()
	os.Stdout, os.Stderr = origOut, origErr
	io.Copy(&out, r)
	r.Close()

	if code != int(host.ExitError) {
		t.Fatalf("check --json bad: exit = %d, want %d", code, host.ExitError)
	}
	var res struct {
		OK     bool `json:"ok"`
		Issues []struct {
			Line int `json:"line"`
			Col  int `json:"col"`
		} `json:"issues"`
	}
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("not JSON: %v\nraw: %s", err, out.String())
	}
	if res.OK {
		t.Error("ok = true, want false")
	}
	if len(res.Issues) != 1 || res.Issues[0].Line != 1 {
		t.Errorf("issues = %+v, want 1 issue at line 1", res.Issues)
	}
}

func TestRun_Test_JSON(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "hello.lua")
	os.WriteFile(script, []byte(`function main() k.print("hi") k.quit() end`), 0o644)
	var out bytes.Buffer
	origOut, origErr := os.Stdout, os.Stderr
	r, w, _ := os.Pipe()
	os.Stdout = w
	os.Stderr = w
	code := Run([]string{"run", script, "--test", "--json"})
	w.Close()
	os.Stdout, os.Stderr = origOut, origErr
	io.Copy(&out, r)
	r.Close()

	if code != int(host.ExitOK) {
		t.Fatalf("run --test --json: exit = %d, want %d\noutput: %s", code, host.ExitOK, out.String())
	}
	var res struct {
		OK     bool   `json:"ok"`
		Phase  string `json:"phase"`
		Output string `json:"output"`
	}
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if !res.OK || res.Phase != "run" {
		t.Errorf("ok=%v phase=%q, want ok=true phase=run", res.OK, res.Phase)
	}
	if !strings.Contains(res.Output, "hi") {
		t.Errorf("output = %q, want 'hi'", res.Output)
	}
}

func TestRun_New_Template(t *testing.T) {
	tmp := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(tmp)
	defer os.Chdir(orig)

	// Unknown template → usage error.
	code := Run([]string{"new", "x.lua", "--template", "nope"})
	if code != int(host.ExitUsage) {
		t.Fatalf("new --template nope: exit = %d, want %d", code, host.ExitUsage)
	}

	// serve-http template → file starts with serve callback
	code = Run([]string{"new", "srv", "--template", "serve-http"})
	if code != int(host.ExitOK) {
		t.Fatalf("new serve-http: exit = %d", code)
	}
	data, _ := os.ReadFile("srv.lua")
	if !strings.Contains(string(data), "function handle_http") {
		t.Error("serve-http template should contain handle_http")
	}

	// Default template (no --template)
	code = Run([]string{"new", "def"})
	if code != int(host.ExitOK) {
		t.Fatalf("new def: exit = %d", code)
	}
	if _, err := os.Stat("def.lua"); err != nil {
		t.Errorf("default template should create def.lua: %v", err)
	}

	// .lua suffix detection
	code = Run([]string{"new", "suf.lua"})
	if code != int(host.ExitOK) {
		t.Fatalf("new suf.lua: exit = %d", code)
	}
	if _, err := os.Stat("suf.lua"); err != nil {
		t.Errorf("new suf.lua should create suf.lua (not suf.lua.lua): %v", err)
	}
}

func TestRun_New_Exists(t *testing.T) {
	tmp := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(tmp)
	defer os.Chdir(orig)
	os.WriteFile("exists.lua", []byte("x"), 0o644)
	code := Run([]string{"new", "exists"})
	if code != int(host.ExitError) {
		t.Fatalf("new exists: exit = %d, want ExitError", code)
	}
}

func TestRun_Serve_Test(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "app.lua")
	os.WriteFile(script, []byte(`
function main() end
function handle_http(req) return "ok" end
function handle_ws(msg)
  if msg.type == "text" then return "echo:" .. msg.data end
end
function handle_tcp(msg)
  if msg.type == "text" then return "tcp:" .. msg.data end
end
`), 0o644)
	code := Run([]string{"serve", script, "--test", "--mode", "http,ws,tcp", "--tcp-echo", "ping"})
	if code != int(host.ExitOK) {
		t.Fatalf("serve --test: exit = %d, want %d", code, host.ExitOK)
	}
}

func TestRun_Serve_Test_JSON(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "app.lua")
	os.WriteFile(script, []byte(`function main() end
function handle_http(req) return "hello" end
`), 0o644)
	var out bytes.Buffer
	origOut, origErr := os.Stdout, os.Stderr
	r, w, _ := os.Pipe()
	os.Stdout = w
	os.Stderr = w
	code := Run([]string{"serve", script, "--test", "--json"})
	w.Close()
	os.Stdout, os.Stderr = origOut, origErr
	io.Copy(&out, r)
	r.Close()

	if code != int(host.ExitOK) {
		t.Fatalf("serve --test --json: exit = %d\nraw: %s", code, out.String())
	}
	var res struct {
		OK     bool   `json:"ok"`
		Mode   string `json:"mode"`
		InitOK bool   `json:"init_ok"`
	}
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("not JSON: %v\nraw: %s", err, out.String())
	}
	if !res.OK || !res.InitOK {
		t.Errorf("ok=%v init_ok=%v, want both true", res.OK, res.InitOK)
	}
}
