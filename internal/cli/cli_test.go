package cli

import (
	"os"
	"path/filepath"
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
