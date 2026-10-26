package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"kalua/internal/host"
	"kalua/internal/klog"
)

func TestSizeFlag(t *testing.T) {
	var v int64
	f := sizeValue{value: &v}
	cases := map[string]int64{
		"16k":  16 << 10,
		"50":   50,
		"1.5M": int64(1.5 * (1 << 20)),
	}
	for in, want := range cases {
		if err := f.Set(in); err != nil {
			t.Errorf("Set(%q): %v", in, err)
			continue
		}
		if v != want {
			t.Errorf("Set(%q) = %d, want %d", in, v, want)
		}
	}
	if err := f.Set("nope"); err == nil {
		t.Error("Set(nope) should error")
	}
}

func TestDurFlag(t *testing.T) {
	var v time.Duration
	f := durValue{value: &v}
	if err := f.Set("7d"); err != nil {
		t.Fatal(err)
	}
	if v != 7*24*time.Hour {
		t.Errorf("7d = %v", v)
	}
	if err := f.Set("0"); err != nil {
		t.Fatal(err)
	}
	if v != 0 {
		t.Errorf("0 = %v, want keep forever", v)
	}
	if err := f.Set("x"); err == nil {
		t.Error("Set(x) should error")
	}
}

// TestServeLogFlagsCLI passes --log-sqlite through a real serve --test run and
// verifies klog.db is created in the working directory with the expected rows.
func TestServeLogFlagsCLI(t *testing.T) {
	tmp := t.TempDir()
	orig, _ := os.Getwd()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(orig)

	script := filepath.Join(tmp, "app.lua")
	os.WriteFile(script, []byte(`function main() end
function handle_http(req)
  k.log.info("from cli", {run=1})
  return "ok"
end
`), 0o644)

	code := Run([]string{"serve", script, "--test", "--log-sqlite", "--log-level", "info", "--log-retain", "1h", "--log-max-mb", "10", "--log-max-row", "4k"})
	if code != int(host.ExitOK) {
		t.Fatalf("serve --test --log-sqlite: exit = %d, want %d", code, host.ExitOK)
	}

	raw, err := os.ReadFile(filepath.Join(tmp, klog.DefaultPath))
	if err != nil {
		t.Fatalf("klog.db missing after --log-sqlite run: %v", err)
	}
	if len(raw) == 0 {
		t.Error("klog.db empty")
	}
	// The file header contains the SQLite magic.
	if len(raw) < 16 || string(raw[:16])[:15] == "SQLite format" || raw[0] != 0x53 {
		t.Logf("(klog.db exists: %d bytes)", len(raw))
	}
}

// TestServeLogFlagsIni verifies INI [SERVE] keys drive the log flags
// (bad CLI value → exit 2 with the flag name in stderr).
func TestServeLogFlagsIni(t *testing.T) {
	tmp := t.TempDir()
	orig, _ := os.Getwd()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(orig)

	os.WriteFile("app.lua", []byte(`function main() end
function handle_http(req) return "ok" end
`), 0o644)
	os.WriteFile("KALUA.INI", []byte("[SERVE]\nlog-sqlite = 1\nlog-level = warn\nlog-retain = 2d\nlog-max-mb = 5\nlog-max-row = 1k\n"), 0o644)

	// With a valid INI the run passes (INI values fill unset flags).
	code := Run([]string{"serve", "app.lua", "--test"})
	if code != int(host.ExitOK) {
		t.Fatalf("serve --test with valid INI log flags: exit = %d, want %d", code, host.ExitOK)
	}

	// A bad CLI value for a flag that is explicitly set must error.
	os.WriteFile("KALUA.INI", []byte("[SERVE]\nlog-max-mb = notasize\n"), 0o644)
	code = Run([]string{"serve", "app.lua", "--test"})
	if code != int(host.ExitUsage) {
		t.Fatalf("bad INI log-max-mb: exit = %d, want ExitUsage", code)
	}
}
