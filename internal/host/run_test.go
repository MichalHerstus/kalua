package host

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestRun_OK(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "hello.lua")
	err := os.WriteFile(script, []byte(`function main() k.print("hi") k.quit() end`), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	log := NewLogger(false)
	var buf bytes.Buffer
	cfg := RunConfig{ScriptPath: script, Logger: log, Out: &buf}
	code := Run(cfg)
	if code != ExitOK {
		t.Errorf("Run = %d, want %d", code, ExitOK)
	}
}

func TestRun_MissingMain(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "nomain.lua")
	err := os.WriteFile(script, []byte(`function foo() end`), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	log := NewLogger(false)
	cfg := RunConfig{ScriptPath: script, Logger: log}
	code := Run(cfg)
	if code != ExitError {
		t.Errorf("Run = %d, want %d (ExitError)", code, ExitError)
	}
}

func TestRun_SyntaxError(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "broken.lua")
	err := os.WriteFile(script, []byte(`function main(`), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	log := NewLogger(false)
	cfg := RunConfig{ScriptPath: script, Logger: log}
	code := Run(cfg)
	if code != ExitError {
		t.Errorf("Run = %d, want %d (ExitError)", code, ExitError)
	}
}

func TestRun_UnknownBinding(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "bogus.lua")
	err := os.WriteFile(script, []byte(`function main() k.nope() end`), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	log := NewLogger(false)
	cfg := RunConfig{ScriptPath: script, Logger: log}
	code := Run(cfg)
	if code != ExitError {
		t.Errorf("Run = %d, want %d (ExitError)", code, ExitError)
	}
}

func TestRun_KError(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "kboom.lua")
	err := os.WriteFile(script, []byte(`function main() k.error("boom") end`), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	log := NewLogger(false)
	cfg := RunConfig{ScriptPath: script, Logger: log}
	code := Run(cfg)
	if code != ExitError {
		t.Errorf("Run = %d, want %d (ExitError)", code, ExitError)
	}
}

func TestRun_IOError(t *testing.T) {
	log := NewLogger(false)
	cfg := RunConfig{ScriptPath: "/nonexistent/path.lua", Logger: log}
	code := Run(cfg)
	if code != ExitIOError {
		t.Errorf("Run = %d, want %d (ExitIOError)", code, ExitIOError)
	}
}
