package bindings

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRegisterNamedDB(t *testing.T) {
	tmp := t.TempDir()
	dbFile := filepath.Join(tmp, "named.db")
	dsn := "sqlite://" + dbFile

	// Invalid name is rejected before any connection attempt.
	if err := RegisterNamedDB("bad name", dsn); err == nil {
		t.Error("RegisterNamedDB(bad name) succeeded, want error")
	}

	// Unknown driver scheme is rejected.
	if err := RegisterNamedDB("oracle", "oracle://x"); err == nil {
		t.Error("RegisterNamedDB(oracle) succeeded, want error")
	}

	if err := RegisterNamedDB("main", dsn); err != nil {
		t.Fatalf("RegisterNamedDB(main) = %v", err)
	}
	defer CloseNamedDBs()

	names := NamedDBs()
	if len(names) != 1 || names[0] != "main" {
		t.Errorf("NamedDBs() = %v, want [main]", names)
	}
	if h := getNamedDB("main"); h == nil {
		t.Error("getNamedDB(main) = nil")
	}
	// Runtime "db_0x…" ids never appear in NamedDBs, and named lookups resolve
	// through getDBHandle the same way connection ids do.
	dbHandlesMu.Lock()
	dbHandles["db_0xdeadbeef"] = &DBHandle{}
	dbHandlesMu.Unlock()
	defer func() {
		dbHandlesMu.Lock()
		delete(dbHandles, "db_0xdeadbeef")
		dbHandlesMu.Unlock()
	}()

	names = NamedDBs()
	for _, n := range names {
		if strings.HasPrefix(n, "db_") {
			t.Errorf("NamedDBs() leaked runtime handle %q", n)
		}
	}
	if getDBHandle(nil, "main") == nil {
		t.Error("getDBHandle(main) = nil")
	}

	// Re-registering replaces the previous handle without error.
	if err := RegisterNamedDB("main", dsn); err != nil {
		t.Fatalf("re-register RegisterNamedDB(main) = %v", err)
	}
	if h := getNamedDB("main"); h == nil {
		t.Error("getNamedDB(main) = nil after re-register")
	}
}

func TestQueryPreview(t *testing.T) {
	tmp := t.TempDir()
	name := "previewdb"
	if err := RegisterNamedDB(name, "sqlite://"+filepath.Join(tmp, "preview.db")); err != nil {
		t.Fatal(err)
	}
	defer CloseNamedDBs()

	h := getNamedDB(name)
	if h == nil {
		t.Fatal("named handle not registered")
	}
	h.mu.Lock()
	_, err := h.db.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT)")
	if err != nil {
		t.Fatal(err)
	}
	_, err = h.db.Exec("INSERT INTO t VALUES (1, 'a'), (2, 'b'), (3, 'c')")
	h.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}

	cols, rows, err := QueryPreview(name, "SELECT id, name FROM t", 0)
	if err != nil {
		t.Fatalf("QueryPreview = %v", err)
	}
	if want := []string{"id", "name"}; !reflect.DeepEqual(cols, want) {
		t.Errorf("cols = %v, want %v", cols, want)
	}
	if len(rows) != 3 {
		t.Errorf("rows = %d, want 3", len(rows))
	}

	// Limit caps the returned rows.
	_, rows, err = QueryPreview(name, "SELECT id FROM t", 2)
	if err != nil {
		t.Fatalf("QueryPreview(limit=2) = %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("rows(limit=2) = %d, want 2", len(rows))
	}

	// Query errors surface.
	if _, _, err := QueryPreview(name, "SELECT * FROM missing", 0); err == nil {
		t.Error("QueryPreview(missing table) succeeded, want error")
	}

	// Unknown DB name.
	if _, _, err := QueryPreview("nope", "SELECT 1", 0); err == nil || !strings.Contains(err.Error(), "unknown database") {
		t.Errorf("QueryPreview(unknown db) err = %v, want unknown database", err)
	}

	// Write statements are rejected even against a valid handle.
	for _, q := range []string{"DELETE FROM t", "DROP TABLE t", "INSERT INTO t VALUES (9,'z')", "UPDATE t SET name='x'"} {
		if _, _, err := QueryPreview(name, q, 0); err == nil {
			t.Errorf("QueryPreview(%q) succeeded, want read-only rejection", q)
		}
	}
}

func TestQueryPreview_WithCommentAndSemicolon(t *testing.T) {
	name := "commentdb"
	if err := RegisterNamedDB(name, "sqlite://"+filepath.Join(t.TempDir(), "c.db")); err != nil {
		t.Fatal(err)
	}
	defer CloseNamedDBs()
	if _, _, err := QueryPreview(name, "-- leading comment\nSELECT 1 AS x;", 0); err != nil {
		t.Fatalf("QueryPreview(commented select) = %v", err)
	}
}

func TestIsReadOnlyQuery(t *testing.T) {
	cases := []struct {
		q    string
		want bool
	}{
		{"SELECT * FROM t", true},
		{" select 1", true},
		{"WITH x AS (SELECT 1) SELECT * FROM x", true},
		{"PRAGMA table_info(t)", true},
		{"EXPLAIN SELECT * FROM t", true},
		{"SHOW TABLES", true},
		{"SELECT 1;", true},
		{"-- note\nSELECT 1", true},
		{"INSERT INTO t VALUES (1)", false},
		{"UPDATE t SET a=1", false},
		{"DELETE FROM t", false},
		{"DROP TABLE t", false},
		{"   ", false},
		{"-- just a comment", false},
		{"selectx FROM t", false},
		{"not secure SELECT", false},
	}
	for _, c := range cases {
		if got := isReadOnlyQuery(c.q); got != c.want {
			t.Errorf("isReadOnlyQuery(%q) = %v, want %v", c.q, got, c.want)
		}
	}
}

func TestRegisterNamedDBPair(t *testing.T) {
	if err := RegisterNamedDBPair("main=sqlite://:memory:"); err != nil {
		t.Fatalf("RegisterNamedDBPair valid = %v", err)
	}
	defer CloseNamedDBs()
	for _, bad := range []string{"novalue", "=x", ""} {
		if err := RegisterNamedDBPair(bad); err == nil {
			t.Errorf("RegisterNamedDBPair(%q) succeeded, want error", bad)
		}
	}
	if err := RegisterNamedDBPairs([]string{"a=sqlite://:memory:", "bad"}); err == nil {
		t.Error("RegisterNamedDBPairs with a bad spec succeeded, want error")
	}
}
