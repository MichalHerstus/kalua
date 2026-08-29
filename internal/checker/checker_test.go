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
			wantErrs: []string{"test.lua: missing required function main()"},
		},
		{
			name:     "unknown k.bogus",
			src:      `function main() k.bogus() end`,
			wantErrs: []string{"test.lua: unknown k.bogus (not implemented)"},
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
