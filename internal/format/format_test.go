package format

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustFormat(t *testing.T, src string) string {
	t.Helper()
	out, err := Format([]byte(src), "test.lua")
	if err != nil {
		t.Fatalf("Format error: %v\nsrc:\n%s", err, src)
	}
	return string(out)
}

func TestFormatGolden(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{
			name: "if else",
			in:   "if x then\ny=1\nelse\ny=2\nend",
			want: "if x then\n  y = 1\nelse\n  y = 2\nend\n",
		},
		{
			name: "single line if",
			in:   "if x then return 1 end",
			want: "if x then return 1\nend\n",
		},
		{
			name: "function",
			in:   "local function f( a ,b )\nreturn a..b\nend",
			want: "local function f(a, b)\n  return a .. b\nend\n",
		},
		{
			name: "method call and selector",
			in:   "t:a().b = a.b.c:d()",
			want: "t:a().b = a.b.c:d()\n",
		},
		{
			name: "call and index chains",
			in:   "(f())(x)\nt[1][2] = v",
			want: "(f())(x)\nt[1][2] = v\n",
		},
		{
			name: "table in one line",
			in:   "local t={\na=1,\nb={1,2},\n}",
			want: "local t = {\n  a = 1,\n  b = {1, 2},\n}\n",
		},
		{
			name: "arith spacing and unary",
			in:   "local x=1+2*3\nlocal y=-(a+b)\nlocal z=2^-3",
			want: "local x = 1 + 2 * 3\nlocal y = -(a + b)\nlocal z = 2 ^ -3\n",
		},
		{
			name: "repeat until",
			in:   "repeat\nx=x+1\nuntil x>10",
			want: "repeat\n  x = x + 1\nuntil x > 10\n",
		},
		{
			name: "nested for",
			in:   "for i=1,n do\nfor k,v in pairs(t) do\nend\nend",
			want: "for i = 1, n do\n  for k, v in pairs(t) do\n  end\nend\n",
		},
		{
			name: "comments",
			in:   "-- top\nfunction main() -- inline\n-- inside\nreturn 1 -- tail\nend",
			want: "-- top\nfunction main() -- inline\n  -- inside\n  return 1 -- tail\nend\n",
		},
		{
			name: "blank line kept",
			in:   "a=1\n\nb=2",
			want: "a = 1\n\nb = 2\n",
		},
		{
			name: "blank after opener dropped",
			in:   "if x then\n\nreturn 1\nend",
			want: "if x then\n  return 1\nend\n",
		},
		{
			name: "long strings and block comments",
			in:   "local s=[[\nline1\nline2\n]]\n--[[\nblock\ncomment\n]]\nx=1",
			want: "local s = [[\nline1\nline2\n]]\n--[[\nblock\ncomment\n]]\nx = 1\n",
		},
		{
			name: "semicolons",
			in:   "a=1;b=2",
			want: "a = 1; b = 2\n",
		},
		{
			name: "label and goto",
			in:   "::top::\ngoto top",
			want: "::top::\ngoto top\n",
		},
		{
			name: "constructor and calls tight",
			in:   "print({1,2})",
			want: "print({1, 2})\n",
		},
		{
			name: "kalipso form",
			in:   "local f=k.form.new('F1',{title='Demo',layout='vertical',cells={{id='main',width=12}}})\nf:show()",
			want: "local f = k.form.new('F1', {title = 'Demo', layout = 'vertical', cells = {{id = 'main', width = 12}}})\nf:show()\n",
		},
		{
			name: "elseif chain",
			in:   "if a then\nx=1\nelseif b then\nx=2\nelse\nx=3\nend\nx=4",
			want: "if a then\n  x = 1\nelseif b then\n  x = 2\nelse\n  x = 3\nend\nx = 4\n",
		},
		{
			name: "func expr body",
			in:   "local cb=function(self,x)\nreturn x*2\nend",
			want: "local cb = function(self, x)\n  return x * 2\nend\n",
		},
		{
			name: "concat and varargs",
			in:   "s=a..b..c\nlocal t={...}",
			want: "s = a .. b .. c\nlocal t = {...}\n",
		},
		{
			name: "comparison operators",
			in:   "if a==1 and b~=2 then\nlocal c=a<=3\nend",
			want: "if a == 1 and b ~= 2 then\n  local c = a <= 3\nend\n",
		},
		{
			name: "length operator",
			in:   "n=#t",
			want: "n = #t\n",
		},
		{
			name: "index expr and string key",
			in:   "v=t[1]\nw=t['k']",
			want: "v = t[1]\nw = t['k']\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, tc.in)
			if got != tc.want {
				t.Errorf("mismatch\n--- got ---\n%s--- want ---\n%s", got, tc.want)
			}
		})
	}
}

func TestFormatIdempotent(t *testing.T) {
	in := `function main()
local f=k.form.new('F1',{title='Demo'})
-- a comment
   f:button("ok", "Ok", function()   -- spaced
      k.form.close(f)
   end)
end`
	once := mustFormat(t, in)
	twice := mustFormat(t, once)
	if once != twice {
		t.Errorf("not idempotent\n--- once ---\n%s\n--- twice ---\n%s", once, twice)
	}
}

func TestFormatEmpty(t *testing.T) {
	for _, in := range []string{"", "\n", "   \n  \n"} {
		out, err := Format([]byte(in), "test.lua")
		if err != nil {
			t.Fatalf("empty input error: %v", err)
		}
		if string(out) != in {
			t.Errorf("empty input changed: %q -> %q", in, out)
		}
	}
}

func TestFormatInvalidInput(t *testing.T) {
	if _, err := Format([]byte("if x then"), "test.lua"); err == nil {
		t.Error("expected error for unparseable input")
	}
}

func TestDiff(t *testing.T) {
	before := "a = 1\nb = 2\nc = 3\n"
	after := "a = 1\nc = 3\n"
	got := Diff("file.lua", before, after)
	want := "--- file.lua\n+++ file.lua\n@@ -2,1 +0,0 @@\n-b = 2\n"
	if got != want {
		t.Errorf("diff mismatch\n--- got ---\n%s--- want ---\n%s", got, want)
	}
	if Diff("file.lua", before, before) != "" {
		t.Error("equal inputs should produce empty diff")
	}
}

func TestDiffManyRoot(t *testing.T) {
	// replacement with no common prefix content
	before := "x\n"
	after := "longer line\n"
	got := Diff("f", before, after)
	if !strings.Contains(got, "@@ -1,1 +1,1 @@") {
		t.Errorf("missing hunk header:\n%s", got)
	}
}

func TestFormatRealApps(t *testing.T) {
	apps, err := filepath.Glob("../../testdata/apps/*.lua")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range apps {
		base := filepath.Base(path)
		// broken.lua / kboom.lua / kbogus.lua / lboom.lua are intentionally
		// invalid scripts used by error-path tests; skip them here.
		if base == "broken.lua" || base == "kboom.lua" || base == "kbogus.lua" || base == "lboom.lua" {
			continue
		}
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		out, err := Format(src, path)
		if err != nil {
			t.Errorf("%s: Format error: %v", base, err)
			continue
		}
		out2, err := Format(out, path)
		if err != nil {
			t.Errorf("%s: second Format error: %v", base, err)
			continue
		}
		if string(out) != string(out2) {
			t.Errorf("%s: not idempotent", base)
		}
	}
}
