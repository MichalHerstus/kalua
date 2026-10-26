//go:build js && wasm

package klog

import "time"

// DB is a no-op sink on WASM (the browser has no SQLite file). It satisfies
// the same interface as the native SQLite back end so Options.LogDB compiles
// identically; Emit and batch writes discard records.
type DB struct{}

// Open returns a no-op sink.
func Open(cfg Config) (*DB, error) { return &DB{}, nil }

// Emit discards the record.
func (d *DB) Emit(Record) {}

// Flush is a no-op.
func (d *DB) Flush() error { return nil }

// Close is a no-op.
func (d *DB) Close() error { return nil }

// Prune is a no-op.
func (d *DB) Prune(time.Time) int64 { return 0 }

// Stats returns an empty snapshot.
func (d *DB) Stats() Stats { return Stats{} }

// Path returns the configured path (kept as config value).
func (d *DB) Path() string { return DefaultPath }

// MaxRow returns the configured per-row cap.
func (d *DB) MaxRow() int64 { return DefaultMaxRow }

// Level returns the minimum accepted level.
func (d *DB) Level() Level { return LevelError }
