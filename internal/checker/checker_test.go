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
