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
