package host

import (
	"bytes"
	"os"
	"testing"
)

// Integration tests for PostgreSQL and MSSQL connections (D10). These are
// gated behind env vars so the test suite passes without live databases.
//
//	KALUA_TEST_PG_DSN=postgres://user:pw@host:port/db
//	KALUA_TEST_MSSQL_DSN=sqlserver://user:pw@host:port?database=db
func runDBFlow(t *testing.T, dsn, driver string) {
	t.Helper()
	if dsn == "" {
		t.Skip("no DSN provided, skipping DB integration test")
	}

	src := `
function main()
    local db = k.connect_db("` + dsn + `")
    k.sql(db, "DROP TABLE IF EXISTS kalua_ext_test")
    k.sql(db, "CREATE TABLE kalua_ext_test (id INTEGER PRIMARY KEY, name VARCHAR(100) NOT NULL)")
    k.db_insert(db, "kalua_ext_test", {id = 1, name = "apple"})
    k.db_insert(db, "kalua_ext_test", {id = 2, name = "banana"})

    -- WHERE clause exercises driver-specific placeholders ($1 / @p1)
    local res = k.db_select(db, "kalua_ext_test", {"id", "name"}, {name = "apple"})
    local n = 0
    local iter = k.rows(res)
    while true do
        local row = iter()
        if row == nil then break end
        n = n + 1
    end
    if n ~= 1 then k.error("expected 1 row, got " .. n) end

    -- UPDATE with SET + WHERE mixes placeholders sequentially
    k.db_update(db, "kalua_ext_test", {name = "apple2"}, {name = "apple"})
    local check = k.db_select(db, "kalua_ext_test", {"COUNT(*) as cnt"}, {name = "apple2"})
    if check.rows[1].cnt ~= 1 then k.error("update failed") end

    k.db_delete(db, "kalua_ext_test", {name = "banana"})
    local after = k.db_select(db, "kalua_ext_test", {"COUNT(*) as cnt"}, {})
    if after.rows[1].cnt ~= 1 then k.error("delete failed") end

    -- transaction rollback
    k.tx_begin(db)
    k.db_insert(db, "kalua_ext_test", {id = 3, name = "cherry"})
    k.tx_rollback(db)
    local after2 = k.db_select(db, "kalua_ext_test", {"COUNT(*) as cnt"}, {})
    if after2.rows[1].cnt ~= 1 then k.error("rollback failed") end

    k.sql(db, "DROP TABLE kalua_ext_test")
    k.disconnect_db(db)
    k.quit()
end
`
	script := t.TempDir() + "/db_ext.lua"
	if err := os.WriteFile(script, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	log := NewLogger(false)
	var buf bytes.Buffer
	cfg := RunConfig{ScriptPath: script, Logger: log, Out: &buf}
	code := Run(cfg)
	if code != ExitOK {
		t.Errorf("%s Run = %d, want %d\noutput:\n%s", driver, code, ExitOK, buf.String())
	}
}

func TestRun_DBFlowPostgres(t *testing.T) {
	runDBFlow(t, os.Getenv("KALUA_TEST_PG_DSN"), "postgres")
}

func TestRun_DBFlowMSSQL(t *testing.T) {
	runDBFlow(t, os.Getenv("KALUA_TEST_MSSQL_DSN"), "mssql")
}
