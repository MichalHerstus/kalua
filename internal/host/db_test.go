package host

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestRun_DBFlow(t *testing.T) {
	tmp := t.TempDir()
	dbFile := filepath.Join(tmp, "test.db")
	script := filepath.Join(tmp, "db.lua")
	src := `
function main()
    local db = k.connect_db("sqlite:` + dbFile + `")
    k.sql(db, "CREATE TABLE IF NOT EXISTS items (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL)")
    k.db_insert(db, "items", {name = "apple"})
    k.db_insert(db, "items", {name = "banana"})

    local res = k.db_select(db, "items", {"id", "name"}, {})
    local n = 0
    local iter = k.rows(res)
    while true do
        local row = iter()
        if row == nil then break end
        n = n + 1
    end
    if n ~= 2 then k.error("expected 2 rows, got " .. n) end

    k.db_update(db, "items", {name = "apple2"}, {name = "apple"})
    k.db_delete(db, "items", {name = "banana"})

    local final = k.db_select(db, "items", {"COUNT(*) as cnt"}, {})
    if final.rows[1].cnt ~= 1 then k.error("expected 1 row after update/delete") end

    k.tx_begin(db)
    k.db_insert(db, "items", {name = "cherry"})
    k.tx_rollback(db)

    local after = k.db_select(db, "items", {"COUNT(*) as cnt"}, {})
    if after.rows[1].cnt ~= 1 then k.error("rollback failed") end

    k.disconnect_db(db)
    k.quit()
end
`
	if err := os.WriteFile(script, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	log := NewLogger(false)
	var buf bytes.Buffer
	cfg := RunConfig{ScriptPath: script, Logger: log, Out: &buf}
	code := Run(cfg)
	if code != ExitOK {
		t.Errorf("Run = %d, want %d\noutput:\n%s", code, ExitOK, buf.String())
	}
}

// TestRun_NamedDB verifies that a --db NAME=DSN handle is usable as db="NAME"
// throughout the DB bindings (k.sql, k.db_insert, k.db_select) without a
// Lua-side k.connect_db().
func TestRun_NamedDB(t *testing.T) {
	tmp := t.TempDir()
	dbFile := filepath.Join(tmp, "named.db")
	script := filepath.Join(tmp, "named.lua")
	src := `
function main()
    k.sql("main", "CREATE TABLE IF NOT EXISTS items (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT)")
    k.db_insert("main", "items", {name = "apple"})
    k.db_insert("main", "items", {name = "banana"})

    local res = k.db_select("main", "items", {"id", "name"}, {})
    local n = 0
    local iter = k.rows(res)
    while true do
        local row = iter()
        if row == nil then break end
        n = n + 1
    end
    if n ~= 2 then k.error("expected 2 rows, got " .. n) end

    k.disconnect_db()
    k.quit()
end
`
	if err := os.WriteFile(script, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	log := NewLogger(false)
	var buf bytes.Buffer
	cfg := RunConfig{
		ScriptPath: script,
		DBs:        []string{"main=sqlite://" + dbFile},
		Logger:     log,
		Out:        &buf,
	}
	code := Run(cfg)
	if code != ExitOK {
		t.Errorf("Run = %d, want %d\noutput:\n%s", code, ExitOK, buf.String())
	}
}

// TestRun_NamedDB_BadDSN verifies that an unreachable --db spec fails fast with
// a startup error instead of failing silently later.
func TestRun_NamedDB_BadDSN(t *testing.T) {
	script := filepath.Join(t.TempDir(), "bad.lua")
	if err := os.WriteFile(script, []byte("function main() k.quit() end\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	log := NewLogger(false)
	var buf bytes.Buffer
	cfg := RunConfig{
		ScriptPath: script,
		DBs:        []string{"broken=sqlite:///nonexistent-dir-xyz/nope.db"},
		Logger:     log,
		Out:        &buf,
	}
	if code := Run(cfg); code != ExitError {
		t.Errorf("Run with bad --db = %d, want ExitError\noutput:\n%s", code, buf.String())
	}
}
