//go:build !wasm

package klog

import (
	"database/sql"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite"
)

// Writer batching / retention tuning (Phase 8 architecture).
const (
	batchMax      = 64
	flushInterval = 200 * time.Millisecond
	pruneInterval = time.Hour
	lowWaterPct   = 80
)

// DB is the structured-log sink backed by SQLite. All Emit calls are
// non-blocking; a single writer goroutine batches rows and a pruning pass runs
// hourly. Safe for concurrent use from N workers.
type DB struct {
	path     string
	maxRow   int64
	minLevel Level
	retain   time.Duration
	maxMB    int64

	db *sql.DB

	ch      chan Record
	flushes chan chan error
	stopCh  chan struct{}
	done    chan struct{}

	mu      sync.Mutex // guards rows/oldest/newest
	dropped atomic.Int64
	rows    int64
	oldest  time.Time
	newest  time.Time
}

// Open creates/opens the log database and starts the writer goroutine.
func Open(cfg Config) (*DB, error) {
	if cfg.Path == "" {
		cfg.Path = DefaultPath
	}
	if cfg.MaxRow <= 0 {
		cfg.MaxRow = DefaultMaxRow
	}
	db, err := sql.Open("sqlite", filepath.ToSlash(cfg.Path))
	if err != nil {
		return nil, err
	}
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA synchronous=NORMAL",
	} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, err
		}
	}
	if _, err := db.Exec(schemaSQL); err != nil {
		db.Close()
		return nil, err
	}

	d := &DB{
		path:     cfg.Path,
		maxRow:   cfg.MaxRow,
		minLevel: cfg.Level,
		retain:   cfg.Retain,
		maxMB:    cfg.MaxMB,
		db:       db,
		ch:       make(chan Record, 1024),
		flushes:  make(chan chan error, 1),
		stopCh:   make(chan struct{}),
		done:     make(chan struct{}),
	}
	go d.writer()
	return d, nil
}
const schemaSQL = `
CREATE TABLE IF NOT EXISTS klog (
  id          INTEGER PRIMARY KEY,
  ts          TEXT    NOT NULL,
  ts_unix_ms  INTEGER NOT NULL,
  level       INTEGER NOT NULL,
  category    TEXT    NOT NULL,
  worker      INTEGER,
  session     TEXT,
  remote_addr TEXT,
  message     TEXT    NOT NULL,
  data        TEXT
);
CREATE INDEX IF NOT EXISTS klog_ts        ON klog(ts_unix_ms);
CREATE INDEX IF NOT EXISTS klog_level_cat ON klog(level, category);
CREATE TABLE IF NOT EXISTS klog_meta (
  key   TEXT PRIMARY KEY,
  value TEXT
);
`

// Emit queues a record. The call never blocks: a full queue increments the
// dropped counter instead (records are truncated to MaxRow first, so the queue
// holds at most 1024 * MaxRow bytes). Time/level gating happens here: levels
// are 0..4 (error/warn/info/debug/trace), so a record is kept when its level
// is at or above the configured minimum (a "warn" threshold keeps error+warn).
func (d *DB) Emit(r Record) {
	if d == nil {
		return
	}
	if r.Level > d.minLevel {
		return
	}
	if r.Time.IsZero() {
		r.Time = time.Now().UTC()
	}
	// Per-row cap: message + data combined ≤ maxRow.
	if d.maxRow > 0 {
		r.Message = TruncateMessage(r.Message, int(d.maxRow))
		left := int(d.maxRow) - len(r.Message)
		if left < 0 {
			left = 0
		}
		r.Data = TruncateData(r.Data, left)
	}
	select {
	case d.ch <- r:
	default:
		d.dropped.Add(1)
	}
}

// Flush blocks until all queued records are written. It returns an aggregate
// error of the failed inserts, or nil.
func (d *DB) Flush() error {
	done := make(chan error, 1)
	select {
	case d.flushes <- done:
	case <-d.done:
		return nil
	}
	return <-done
}

// Close stops the writer, flushes any pending rows, and closes the database.
func (d *DB) Close() error {
	select {
	case <-d.done:
		return d.db.Close()
	default:
	}
	close(d.stopCh)
	<-d.done
	// The writer drains and flushes remaining rows before exiting, so a
	// separate Flush here would deadlock (nobody reads d.flushes anymore).
	return d.db.Close()
}

// writer owns the database: it batches ch records into transactions, services
// flush requests, and runs the hourly retention prune.
func (d *DB) writer() {
	defer close(d.done)

	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()
	pruneTicker := time.NewTicker(pruneInterval)
	defer pruneTicker.Stop()

	var batch []Record
	insert := func() {
		if len(batch) == 0 {
			return
		}
		if err := d.insertBatch(batch); err == nil {
			d.mu.Lock()
			for _, r := range batch {
				d.rows++
				if d.oldest.IsZero() || r.Time.Before(d.oldest) {
					d.oldest = r.Time
				}
				if r.Time.After(d.newest) {
					d.newest = r.Time
				}
			}
			d.mu.Unlock()
		} else {
			d.dropped.Add(int64(len(batch)))
		}
		batch = batch[:0]
	}
	// drain pulls every record already queued into batch so a Flush reflects
	// them deterministically (no reliance on the ticker).
	drain := func() {
		for {
			select {
			case r := <-d.ch:
				batch = append(batch, r)
				if len(batch) >= batchMax {
					insert()
				}
			default:
				return
			}
		}
	}

	for {
		select {
		case r := <-d.ch:
			batch = append(batch, r)
			if len(batch) >= batchMax {
				insert()
			}
		case <-ticker.C:
			insert()
		case done := <-d.flushes:
			drain()
			insert()
			done <- nil
		case <-pruneTicker.C:
			drain()
			insert()
			d.prune(time.Now())
		case <-d.stopCh:
			// Drain remaining records without blocking forever.
			for {
				select {
				case r := <-d.ch:
					batch = append(batch, r)
					if len(batch) >= batchMax {
						insert()
					}
				default:
					insert()
					return
				}
			}
		}
	}
}

func (d *DB) insertBatch(batch []Record) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO klog
		(ts, ts_unix_ms, level, category, worker, session, remote_addr, message, data)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		tx.Rollback()
		return err
	}
	for _, r := range batch {
		var worker any
		if r.Worker != 0 {
			worker = r.Worker
		}
		var session any
		if r.Session != "" {
			session = r.Session
		}
		var remote any
		if r.RemoteAddr != "" {
			remote = r.RemoteAddr
		}
		var data any
		if r.Data != "" {
			data = r.Data
		}
		if _, err := stmt.Exec(
			r.Time.UTC().Format(time.RFC3339Nano),
			r.Time.UnixMilli(),
			int(r.Level),
			r.Category,
			worker, session, remote,
			r.Message, data,
		); err != nil {
			stmt.Close()
			tx.Rollback()
			return err
		}
	}
	stmt.Close()
	return tx.Commit()
}

// Prune removes records older than retain and, when a size cap is set, shrinks
// the file to 80% low-water. Returns the number of rows removed.
func (d *DB) Prune(now time.Time) int64 {
	return d.prune(now)
}

func (d *DB) prune(now time.Time) int64 {
	var removed int64
	if d.retain > 0 {
		cutoff := now.Add(-d.retain).UnixMilli()
		res, err := d.db.Exec(`DELETE FROM klog WHERE ts_unix_ms < ?`, cutoff)
		if err == nil {
			if n, _ := res.RowsAffected(); n > 0 {
				removed += n
			}
		}
	}
	if d.maxMB > 0 {
		bytes := int64(d.maxMB) * (1 << 20)
		low := bytes * lowWaterPct / 100
		// Loop delete oldest rows until the file is back to low-water.
		for {
			size := dbFileSize(d.path)
			if size <= low {
				break
			}
			res, err := d.db.Exec(`DELETE FROM klog WHERE id IN (SELECT id FROM klog ORDER BY ts_unix_ms, id LIMIT 1000)`)
			if err != nil {
				break
			}
			n, _ := res.RowsAffected()
			if n == 0 || n < 0 {
				break
			}
			removed += n
		}
		if dbFileSize(d.path) > 0 {
			// Checkpoint so the WAL collapses into the main file (the size cap
			// measures the on-disk file). Best-effort.
			d.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`)
		}
	}
	return removed
}

func dbFileSize(path string) int64 {
	fi, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return fi.Size()
}

// Stats returns a snapshot of rows/dropped/oldest/newest/path.
func (d *DB) Stats() Stats {
	d.mu.Lock()
	defer d.mu.Unlock()
	return Stats{
		Rows:    d.rows,
		Dropped: d.dropped.Load(),
		Oldest:  d.oldest,
		Newest:  d.newest,
		Path:    d.path,
	}
}

// Path returns the database file path.
func (d *DB) Path() string { return d.path }

// MaxRow returns the configured per-row cap.
func (d *DB) MaxRow() int64 { return d.maxRow }

// Level returns the minimum accepted level.
func (d *DB) Level() Level { return d.minLevel }
