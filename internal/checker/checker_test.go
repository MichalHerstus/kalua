package checker

import (
	"strings"
	"testing"
)

func TestCheck(t *testing.T) {
	cases := []struct {
		name     string
		src      string
		wantErrs []string
	}{
		{
			name:     "valid hello",
			src:      `function main() k.print("hi") end`,
			wantErrs: nil,
		},
		{
			name:     "missing main",
			src:      `function foo() end`,
			wantErrs: []string{"test.lua: missing required entry point: main() (run mode) or handle_http/handle_ws/handle_tcp/init/shutdown (serve mode)"},
		},
		{
			name:     "unknown k.bogus",
			src:      `function main() k.bogus() end`,
			wantErrs: []string{"test.lua:1:"},
		},
		{
			name:     "syntax error",
			src:      `function main(`,
			wantErrs: []string{"syntax error"},
		},
	}
	for _, tc := range cases {
		res := Check(tc.src, "test.lua")
		if len(res.Errors) != len(tc.wantErrs) {
			t.Errorf("%s: got %d errors, want %d: %v", tc.name, len(res.Errors), len(tc.wantErrs), res.Errors)
			continue
		}
		for i, err := range res.Errors {
			if tc.wantErrs[i] != "" && !strings.Contains(err, tc.wantErrs[i]) {
				t.Errorf("%s: error %d = %q, want contains %q", tc.name, i, err, tc.wantErrs[i])
			}
		}
	}
}

func TestIssuePositions(t *testing.T) {
	// Unknown k.* must carry the offending line and a good-effort column so
	// the LSP can draw a squiggle under the token.
	res := Check("function main()\n  k.bogus()\n  k.form.new(\"f\")\n  k.no_such_api()\nend", "test.lua")
	var gotLine, gotCol int
	for _, iss := range res.Issues {
		if iss.Message == "unknown k.bogus (not implemented)" {
			gotLine, gotCol = iss.Line, iss.Col
		}
	}
	if gotLine != 2 {
		t.Errorf("bogus issue line = %d, want 2", gotLine)
	}
	if gotCol != 3 {
		t.Errorf("bogus issue col = %d, want 3", gotCol)
	}

	// Syntax errors expose the parser's line/column.
	s := Check("function main()\n  local x = & 5\nend", "test.lua")
	if len(s.Issues) == 0 || s.Issues[0].Line != 2 {
		t.Fatalf("syntax issue has wrong position: %+v", s.Issues)
	}
}

func TestEntryMode(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"run", "function main() end", "run"},
		{"serve-http", "function handle_http(req) return \"ok\" end", "serve"},
		{"serve-ws", "function handle_ws(msg) end", "serve"},
		{"serve-tcp", "function handle_tcp(msg) end", "serve"},
		{"serve-priority", "function main() end\nfunction handle_http(req) return \"ok\" end", "serve"},
		{"both-empty", "local x = 1", ""},
		{"syntax-error", "function ", ""},
		{"nested-do", "do\n  function main() end\nend", "run"},
		{"nested-if", "if true then\n  function handle_http(req) return \"ok\" end\nend", "serve"},
		{"method-only", "function t.handle_http() end", ""},
	}
	for _, c := range cases {
		if got := EntryMode(c.src, c.name+".lua"); got != c.want {
			t.Errorf("%s: EntryMode = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestServeMode(t *testing.T) {
	if !ServeMode("function handle_ws(msg) end", "a.lua") {
		t.Error("handle_ws should report serve mode")
	}
	if ServeMode("function main() end", "a.lua") {
		t.Error("main should NOT report serve mode")
	}
	if ServeMode("function handle_wsx(msg) end", "a.lua") {
		t.Error("handle_wsx must not match handle_ws")
	}
	if ServeMode("function alib.handle_tcp(msg) end", "a.lua") {
		t.Error("method handle_tcp must not count as serve mode")
	}
}
