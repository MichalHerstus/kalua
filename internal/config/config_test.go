package config

import (
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestNorm(t *testing.T) {
	cases := map[string]string{
		"no-browser":       "nobrowser",
		"NO_BROWSER":       "nobrowser",
		"no_browser":       "nobrowser",
		"KALUA_AI_MODEL":   "kaluaaimodel",
		"AI_OPEN_ROUTER":   "aiopenrouter",
		"  session-limit ": "sessionlimit",
	}
	for in, want := range cases {
		if got := Norm(in); got != want {
			t.Errorf("Norm(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParse(t *testing.T) {
	ini := []byte("\ufeff; comment\n# another\n" +
		"[RUN]\n" +
		"no-browser = 1\n" +
		"port = 9000\n" +
		"watch\n" + // bare key == true
		"db = main=sqlite://a.db\n" +
		"db = aux=sqlite://b.db\n" +
		"verbose=no\n" +
		"\n[AI_OPEN_ROUTER]\n" +
		"KALUA_AI_BASE_URL = https://openrouter.ai/api/v1\n" +
		"KALUA_AI_MODEL = nvidia/nemotron\n")
	f := Parse(ini)

	if got, _ := f.Get("run", "no-browser"); got != "1" {
		t.Errorf("run/no-browser = %q", got)
	}
	if got, _ := f.Get("RUN", "port"); got != "9000" {
		t.Errorf("RUN/port (case) = %q", got)
	}
	if got, _ := f.Get("run", "watch"); got != "true" {
		t.Errorf("run/watch (bare key) = %q", got)
	}
	vals := f.Values("run", "db")
	if !reflect.DeepEqual(vals, []string{"main=sqlite://a.db", "aux=sqlite://b.db"}) {
		t.Errorf("run/db repeated = %v", vals)
	}
	if v, _ := f.Get("run", "verbose"); v != "no" {
		t.Errorf("run/verbose = %q", v)
	}
	if v, ok := f.Get("ai_open_router", "KALUA_AI_BASE_URL"); !ok || v != "https://openrouter.ai/api/v1" {
		t.Errorf("ai_open_router/KALUA_AI_BASE_URL = %q, %v", v, ok)
	}
	if got, ok := f.Get("run", "missing"); ok || got != "" {
		t.Errorf("missing key = %q, %v", got, ok)
	}
}

func TestTruthy(t *testing.T) {
	for in, want := range map[string]bool{
		"1": true, "true": true, "TRUE": true, "yes": true, "on": true, "y": true, "t": true,
		"0": false, "false": false, "no": false, "off": false, "": false, "maybe": false,
	} {
		if got := Truthy(in); got != want {
			t.Errorf("Truthy(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestApplyFlags(t *testing.T) {
	f := Parse([]byte("[RUN]\nport=9090\nno-browser=1\nwatch=off\nsession-limit=0\ndb=main=sqlite://a.db\ndb=aux=sqlite://b.db\nverbose=no\n"))

	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	port := fs.Int("port", 9000, "")
	noBrowser := fs.Bool("no-browser", false, "")
	watch := fs.Bool("watch", false, "")
	sessionLimit := fs.Int("session-limit", 8, "")
	db := &multiFlag{}
	fs.Var(db, "db", "")
	verbose := fs.Bool("v", false, "")

	// Explicitly set on the CLI: must NOT be overridden by the INI.
	if err := fs.Set("port", "8500"); err != nil {
		t.Fatal(err)
	}

	if err := f.ApplyFlags(fs, "run", map[string][]string{"v": {"verbose"}}); err != nil {
		t.Fatalf("ApplyFlags: %v", err)
	}

	if *port != 8500 {
		t.Errorf("explicit flag port = %d, want 8500 (CLI wins)", *port)
	}
	if !*noBrowser {
		t.Error("no-browser not applied from INI")
	}
	if *watch {
		t.Error("watch=off should map to false")
	}
	if *sessionLimit != 0 {
		t.Errorf("session-limit = %d, want 0", *sessionLimit)
	}
	if len(db.values) != 2 || db.values[0] != "main=sqlite://a.db" || db.values[1] != "aux=sqlite://b.db" {
		t.Errorf("db multi-append = %v", db.values)
	}
	if *verbose {
		t.Error("verbose=no should map to false (v alias)")
	}
}

func TestApplyFlagsSkipsIniFlag(t *testing.T) {
	f := Parse([]byte("[RUN]\nini=/etc/x\nwatch=1\n"))
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	ini := fs.String("ini", "", "")
	watch := fs.Bool("watch", false, "")
	if err := f.ApplyFlags(fs, "run", nil); err != nil {
		t.Fatal(err)
	}
	if *ini != "" {
		t.Errorf("--ini must not be set from the INI file, got %q", *ini)
	}
	if !*watch {
		t.Error("watch not applied")
	}
}

func TestApplyFlagsBadValue(t *testing.T) {
	f := Parse([]byte("[RUN]\nport=not-a-number\n"))
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.Int("port", 9000, "")
	err := f.ApplyFlags(fs, "run", nil)
	if err == nil {
		t.Fatal("expected error for port=not-a-number")
	}
	if got := err.Error(); !strings.Contains(got, "port") {
		t.Errorf("error should mention the key, got %q", got)
	}
}

func TestApplyFlagsNoSection(t *testing.T) {
	f := Parse([]byte("[RUN]\nport=9090\n"))
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	port := fs.Int("port", 8080, "")
	if err := f.ApplyFlags(fs, "serve", nil); err != nil {
		t.Fatal(err)
	}
	if *port != 8080 {
		t.Errorf("unrelated section changed port to %d", *port)
	}
}

func TestFindPath(t *testing.T) {
	dir := t.TempDir()
	iniPath := filepath.Join(dir, "my.ini")
	if err := os.WriteFile(iniPath, []byte("[RUN]\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("KALUA_INI", iniPath)
	if got := FindPath(""); got != iniPath {
		t.Errorf("FindPath via env = %q, want %q", got, iniPath)
	}
	if got := FindPath("/explicit.ini"); got != "/explicit.ini" {
		t.Errorf("FindPath via flag = %q, want /explicit.ini", got)
	}

	t.Setenv("KALUA_INI", "")
	if got := FindPath(""); got != "" {
		t.Errorf("FindPath with nothing = %q, want empty", got)
	}

	// Default ./KALUA.INI in the working directory.
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWd) }()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("KALUA.INI", []byte("[RUN]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := FindPath(""); got != "KALUA.INI" {
		t.Errorf("FindPath via cwd = %q, want KALUA.INI", got)
	}
}

func TestSampleFileParses(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "KALUA.ini.sample"))
	if err != nil {
		t.Skipf("KALUA.ini.sample not present: %v", err)
	}
	f := Parse(data)
	if v, _ := f.Get("run", "port"); v != "9000" {
		t.Errorf("sample run/port = %q, want 9000", v)
	}
	if v, ok := f.Get("ai", "KALUA_AI_BASE_URL"); !ok || v == "" {
		t.Errorf("sample [AI] missing KALUA_AI_BASE_URL")
	}
}

type multiFlag struct{ values []string }

func (m *multiFlag) String() string {
	if len(m.values) == 0 {
		return ""
	}
	return m.values[0]
}
func (m *multiFlag) Set(s string) error {
	m.values = append(m.values, s)
	return nil
}
