package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kalua/internal/format"
	"kalua/internal/host"
)

// captureStdout runs fn with stdout redirected to a pipe and returns the
// exit code plus everything written to stdout.
func captureStdout(t *testing.T, fn func() int) (int, string) {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	code := fn()
	_ = w.Close()
	os.Stdout = orig
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return code, string(b)
}

func writeScript(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCheck_FormatStdout(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "a.lua", "local x=1\n")
	code, out := captureStdout(t, func() int { return Run([]string{"check", script, "--format"}) })
	if code != int(host.ExitOK) {
		t.Fatalf("exit = %d, want OK", code)
	}
	if out != "local x = 1\n" {
		t.Errorf("stdout = %q, want %q", out, "local x = 1\n")
	}
	// --format must not touch the source file
	src, _ := os.ReadFile(script)
	if string(src) != "local x=1\n" {
		t.Errorf("--format modified the source: %q", src)
	}
}

func TestCheck_FormatWrite(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "a.lua", "local x=1\n")
	code := Run([]string{"check", script, "-w"})
	if code != int(host.ExitOK) {
		t.Fatalf("exit = %d, want OK", code)
	}
	src, err := os.ReadFile(script)
	if err != nil {
		t.Fatal(err)
	}
	if string(src) != "local x = 1\n" {
		t.Errorf("wrote %q, want %q", src, "local x = 1\n")
	}
	// permissions preserved (0o600)
	if info, err := os.Stat(script); err == nil && info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", info.Mode().Perm())
	}
	// idempotent second run: exit OK, no further changes
	code = Run([]string{"check", script, "-w"})
	if code != int(host.ExitOK) {
		t.Errorf("second -w exit = %d, want OK", code)
	}
}

func TestCheck_FormatWriteError(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "bad.lua", "if x then\n")
	code := Run([]string{"check", script, "-w"})
	if code != int(host.ExitError) {
		t.Fatalf("exit = %d, want ExitError", code)
	}
	// unparseable input must not be written
	src, _ := os.ReadFile(script)
	if string(src) != "if x then\n" {
		t.Errorf("-w modified an unparseable file: %q", src)
	}
}

func TestCheck_FormatList(t *testing.T) {
	dir := t.TempDir()
	clean := writeScript(t, dir, "clean.lua", "local x = 1\n")
	dirty := writeScript(t, dir, "dirty.lua", "local x=1\n")
	code, out := captureStdout(t, func() int { return Run([]string{"check", "-l", clean, dirty}) })
	if code != int(host.ExitError) {
		t.Errorf("exit = %d, want ExitError (1) when any file differs", code)
	}
	if !strings.Contains(out, "dirty.lua") || strings.Contains(out, "clean.lua") {
		t.Errorf("-l output = %q, want only the dirty file listed", out)
	}
	// all clean
	code, out = captureStdout(t, func() int { return Run([]string{"check", clean, "-l"}) })
	if code != int(host.ExitOK) || out != "" {
		t.Errorf("clean -l: exit=%d out=%q, want OK and empty", code, out)
	}
}

func TestCheck_FormatDiff(t *testing.T) {
	dir := t.TempDir()
	dirty := writeScript(t, dir, "dirty.lua", "local x=1\n")
	code, out := captureStdout(t, func() int { return Run([]string{"check", dirty, "-d"}) })
	if code != int(host.ExitError) {
		t.Errorf("exit = %d, want ExitError (1)", code)
	}
	if !strings.Contains(out, "--- "+dirty) || !strings.Contains(out, "+++ "+dirty) {
		t.Errorf("-d output missing file headers:\n%s", out)
	}
	if !strings.Contains(out, "-local x=1") || !strings.Contains(out, "+local x = 1") {
		t.Errorf("-d output missing change lines:\n%s", out)
	}
	// clean file: no output, exit 0
	clean := writeScript(t, dir, "clean.lua", "local x = 1\n")
	code, out = captureStdout(t, func() int { return Run([]string{"check", clean, "-d"}) })
	if code != int(host.ExitOK) || out != "" {
		t.Errorf("clean -d: exit=%d out=%q, want OK and empty", code, out)
	}
}

func TestCheck_FormatRoundTripRealApp(t *testing.T) {
	// Formatting any valid app and running it with check must still pass.
	src, err := os.ReadFile("../../testdata/apps/hello.lua")
	if err != nil {
		t.Fatal(err)
	}
	out, err := format.Format(src, "hello.lua")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	formatted := filepath.Join(dir, "hello_fmt.lua")
	if err := os.WriteFile(formatted, out, 0o600); err != nil {
		t.Fatal(err)
	}
	if code := Run([]string{"check", formatted}); code != int(host.ExitOK) {
		t.Errorf("check of formatted app = %d, want OK", code)
	}
}
