package bindings

import (
	"fmt"
	"strings"
	"testing"

	"github.com/yuin/gopher-lua"
)

func TestParseDSN(t *testing.T) {
	cases := []struct {
		in       string
		driver   string
		cleanDSN string
	}{
		{"postgres://postgres:pw@host:5433/db", "pgx", "postgres://postgres:pw@host:5433/db"},
		{"postgresql://postgres:pw@host:5433/db", "pgx", "postgresql://postgres:pw@host:5433/db"},
		{"postgres:postgres:pw@host:5433/db", "pgx", "postgres://postgres:pw@host:5433/db"},
		{"sqlserver://sa:pw@host:1433?database=x", "sqlserver", "sqlserver://sa:pw@host:1433?database=x"},
		{"sqlserver:sa:pw@host:1433?database=x", "sqlserver", "sqlserver://sa:pw@host:1433?database=x"},
		{"mysql://user:pw@host/db", "mysql", "user:pw@host/db"},
		{"mysql:user:pw@host/db", "mysql", "user:pw@host/db"},
		{"sqlite:///abs/path.db", "sqlite", "/abs/path.db"},
		{"sqlite:rel/path.db", "sqlite", "rel/path.db"},
		{"", "", ""},
		{"oracle://x", "", ""},
	}
	for _, c := range cases {
		d, clean := parseDSN(c.in)
		if d != c.driver || clean != c.cleanDSN {
			t.Errorf("parseDSN(%q) = (%q, %q); want (%q, %q)", c.in, d, clean, c.driver, c.cleanDSN)
		}
	}
}

func TestPlaceholder(t *testing.T) {
	cases := []struct {
		driver string
		n      int
		want   string
	}{
		{"pgx", 1, "$1"},
		{"pgx", 2, "$2"},
		{"sqlserver", 1, "@p1"},
		{"sqlserver", 3, "@p3"},
		{"mysql", 1, "?"},
		{"sqlite", 2, "?"},
	}
	for _, c := range cases {
		h := &DBHandle{driver: c.driver}
		if got := h.Placeholder(c.n); got != c.want {
			t.Errorf("Placeholder driver=%q n=%d = %q; want %q", c.driver, c.n, got, c.want)
		}
	}
}

func TestBuildWhereClause(t *testing.T) {
	L := lua.NewState(lua.Options{SkipOpenLibs: true})
	defer L.Close()

	where := L.NewTable()
	where.RawSetString("id", lua.LNumber(42))
	where.RawSetString("active", lua.LBool(true))

	cases := []struct {
		driver   string
		startIdx int
	}{
		{"pgx", 1},
		{"pgx", 3},
		{"sqlserver", 1},
		{"mysql", 1},
	}
	for _, c := range cases {
		handle := &DBHandle{driver: c.driver}
		clause, params := buildWhereClause(handle, where, c.startIdx)
		if !strings.HasPrefix(clause, " WHERE ") {
			t.Errorf("buildWhereClause(driver=%q) missing WHERE prefix: %q", c.driver, clause)
		}
		// id and active should both be present with sequential placeholder indices
		for _, col := range []string{"id", "active"} {
			exp := fmt.Sprintf("%s = %s", col, handle.Placeholder(c.startIdx))
			exp2 := fmt.Sprintf("%s = %s", col, handle.Placeholder(c.startIdx+1))
			if !strings.Contains(clause, exp) && !strings.Contains(clause, exp2) {
				t.Errorf("buildWhereClause(driver=%q, startIdx=%d) missing %q (both %q and %q): %q", c.driver, c.startIdx, col, exp, exp2, clause)
			}
		}
		// params should carry the two values in insertion order
		if len(params) != 2 {
			t.Errorf("buildWhereClause(driver=%q) params len = %d; want 2", c.driver, len(params))
		}
	}

	// Empty where table → no clause, no params
	empty := L.NewTable()
	if clause, params := buildWhereClause(&DBHandle{driver: "pgx"}, empty, 1); clause != "" || len(params) != 0 {
		t.Errorf("empty where: got (%q, %v); want (\"\", [])", clause, params)
	}
}
