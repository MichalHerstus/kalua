//go:build !wasm

package klog

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// openTest mirrors how the serve server constructs a sink: a parsed level is
// always supplied. The zero value (LevelError) means "only errors"; tests that
// want everything pass LevelTrace here. Config.Level is 0 (the natural zero),
// so the helper treats an absent level as "keep everything" only when the test
// explicitly opts in through keepAll. Real callers (the server / klog.Config
// users) always pass a concrete level.
func openTest(t *testing.T, cfg Config, keepAll ...bool) *DB {
	t.Helper()
	if cfg.Path == "" {
		cfg.Path = filepath.Join(t.TempDir(), "test.db")
	}
	if len(keepAll) > 0 && keepAll[0] && cfg.Level == 0 {
		cfg.Level = LevelTrace
	}
	db, err := Open(cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestOpenSchemaAndPragmas(t *testing.T) {
	db := openTest(t, Config{Level: LevelTrace})

	// journal_mode=WAL is persistent in the file and visible to new opens.
	sqlDB, err := sql.Open("sqlite", db.Path())
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer sqlDB.Close()

	var jm string
	if err := sqlDB.QueryRow("PRAGMA journal_mode").Scan(&jm); err != nil {
		t.Fatalf("journal_mode: %v", err)
	}
	if jm != "wal" {
		t.Errorf("journal_mode = %q, want wal", jm)
	}
	// busy_timeout is per-connection; verify it was applied to the sink's own
	// connection by querying through the same handle.
	var bt int
	if err := db.db.QueryRow("PRAGMA busy_timeout").Scan(&bt); err != nil {
		t.Fatalf("busy_timeout: %v", err)
	}
	if bt != 5000 {
		t.Errorf("busy_timeout = %d, want 5000", bt)
	}
	// Tables exist.
	var n int
	if err := sqlDB.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name IN ('klog','klog_meta')").Scan(&n); err != nil {
		t.Fatalf("schema query: %v", err)
	}
	if n != 2 {
		t.Errorf("expected klog + klog_meta tables, got %d", n)
	}
	// Indexes exist.
	if err := sqlDB.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='index' AND name IN ('klog_ts','klog_level_cat')").Scan(&n); err != nil {
		t.Fatalf("index query: %v", err)
	}
	if n != 2 {
		t.Errorf("expected klog_ts + klog_level_cat indexes, got %d", n)
	}
}

func TestEmitAndFlush(t *testing.T) {
	db := openTest(t, Config{Level: LevelTrace})
	db.Emit(Record{Level: LevelInfo, Category: "app", Message: "hello"})
	db.Emit(Record{Level: LevelWarn, Category: "http", Message: "slow", Data: `{"ms":200}`})
	if err := db.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	st := db.Stats()
	if st.Rows != 2 {
		t.Fatalf("rows = %d, want 2", st.Rows)
	}
	if got := st.Path; got == "" {
		t.Error("Stats.Path empty")
	}
	if st.Oldest.IsZero() || st.Newest.IsZero() {
		t.Error("oldest/newest not set")
	}

	sqlDB, err := sql.Open("sqlite", db.Path())
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer sqlDB.Close()
	var msg, cat, data string
	var level int
	var tsMS int64
	err = sqlDB.QueryRow("SELECT message, category, data, level, ts_unix_ms FROM klog WHERE category='http'").Scan(&msg, &cat, &data, &level, &tsMS)
	if err != nil {
		t.Fatalf("select http row: %v", err)
	}
	if msg != "slow" || cat != "http" || data != `{"ms":200}` || level != int(LevelWarn) {
		t.Errorf("row = %q/%q/%q/%d", msg, cat, data, level)
	}
	if tsMS <= 0 {
		t.Errorf("ts_unix_ms = %d", tsMS)
	}
}

func TestLevelGating(t *testing.T) {
	db := openTest(t, Config{Level: LevelWarn})
	db.Emit(Record{Level: LevelError, Category: "app", Message: "e"})
	db.Emit(Record{Level: LevelInfo, Category: "app", Message: "info-dropped"})
	db.Emit(Record{Level: LevelTrace, Category: "app", Message: "trace-dropped"})
	db.Flush()
	if st := db.Stats(); st.Rows != 1 {
		t.Fatalf("rows = %d, want 1 (only error above warn threshold)", st.Rows)
	}
}

func TestRowCapTruncation(t *testing.T) {
	db := openTest(t, Config{MaxRow: 20, Level: LevelTrace})
	long := "x"
	for len(long) < 100 {
		long += "x"
	}
	db.Emit(Record{Level: LevelInfo, Category: "app", Message: long})
	db.Flush()
	sqlDB, err := sql.Open("sqlite", db.Path())
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer sqlDB.Close()
	var msg string
	if err := sqlDB.QueryRow("SELECT message FROM klog").Scan(&msg); err != nil {
		t.Fatalf("select: %v", err)
	}
	if len(msg) > 20 {
		t.Errorf("message %d bytes exceeds cap 20: %q", len(msg), msg)
	}
}

func TestPruneAge(t *testing.T) {
	db := openTest(t, Config{Retain: time.Hour, Level: LevelTrace})
	now := time.Now().UTC()
	// Old row inserted via direct SQL (bypasses the async channel so it can
	// carry a backdated timestamp).
	sqlDB, _ := sql.Open("sqlite", db.Path())
	defer sqlDB.Close()
	old := now.Add(-48 * time.Hour)
	sqlDB.Exec(`INSERT INTO klog(ts, ts_unix_ms, level, category, message) VALUES (?,?,?,?,?)`,
		old.Format(time.RFC3339Nano), old.UnixMilli(), 2, "app", "old")
	newer := now.Add(-30 * time.Minute)
	sqlDB.Exec(`INSERT INTO klog(ts, ts_unix_ms, level, category, message) VALUES (?,?,?,?,?)`,
		newer.Format(time.RFC3339Nano), newer.UnixMilli(), 2, "app", "new")
	db.Emit(Record{Level: LevelInfo, Category: "app", Message: "live"})
	db.Flush()

	removed := db.Prune(now)
	if removed != 1 {
		t.Fatalf("prune removed %d rows, want 1 (the 48h-old one)", removed)
	}
	var n int
	sqlDB.QueryRow("SELECT count(*) FROM klog").Scan(&n)
	if n != 2 {
		t.Errorf("rows after prune = %d, want 2", n)
	}
}

func TestSizeCapPrune(t *testing.T) {
	db := openTest(t, Config{MaxMB: 0, Level: LevelTrace}) // 0 = unlimited; use tiny MaxRow to keep file small
	db.Emit(Record{Level: LevelInfo, Category: "app", Message: "a"})
	db.Emit(Record{Level: LevelInfo, Category: "app", Message: "b"})
	db.Flush()

	// Force a prune pass; rows should remain (below any cap).
	if removed := db.Prune(time.Now()); removed != 0 {
		t.Errorf("prune removed %d rows for unlimited size", removed)
	}
}

func TestCloseFlushes(t *testing.T) {
	db := openTest(t, Config{Level: LevelTrace})
	db.Emit(Record{Level: LevelInfo, Category: "app", Message: "bye"})
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	sqlDB, err := sql.Open("sqlite", db.Path())
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer sqlDB.Close()
	var n int
	sqlDB.QueryRow("SELECT count(*) FROM klog").Scan(&n)
	if n != 1 {
		t.Errorf("rows after close = %d, want 1", n)
	}
}

func TestDroppedCounterOnFullQueue(t *testing.T) {
	db := openTest(t, Config{Level: LevelTrace})
	// Fill far beyond the channel cap (1024). The writer drains concurrently
	// but cannot keep up with 16k synchronous emits, so the dropped counter
	// must trip.
	const n = 16384
	for i := 0; i < n; i++ {
		db.Emit(Record{Level: LevelInfo, Category: "app", Message: "x"})
	}
	db.Flush()
	if st := db.Stats(); st.Dropped <= 0 {
		t.Errorf("dropped = %d, want > 0", st.Dropped)
	}
}

func TestNewSinkPathAndLevel(t *testing.T) {
	db := openTest(t, Config{Level: LevelDebug})
	if db.Path() == "" {
		t.Error("Path empty")
	}
	if db.Level() != LevelDebug {
		t.Errorf("Level = %v, want debug", db.Level())
	}
	if db.MaxRow() != DefaultMaxRow {
		t.Errorf("MaxRow = %d, want %d", db.MaxRow(), DefaultMaxRow)
	}
}
