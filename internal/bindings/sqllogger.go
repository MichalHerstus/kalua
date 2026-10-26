//go:build !wasm

package bindings

import (
	"context"
	"fmt"

	"kalua/internal/klog"
)

// SQLLogger adapts a klog.DB sink to the host Logger interface (Phase 8).
// It is used in serve mode: stdout logging still happens through the regular
// host logger, and identical records are also captured into klog.db when the
// sink is configured (--log-sqlite). It also implements the context-using
// request-scoped logging used by the HTTP/WS/TCP servers.
type SQLLogger struct {
	DB *klog.DB
	// MinLevel drops records below this severity at the boundary.
	MinLevel klog.Level
}

// NewSQLLogger wraps a klog sink. A nil db yields a no-op logger.
func NewSQLLogger(db *klog.DB) *SQLLogger {
	level := klog.LevelInfo
	if db != nil {
		level = db.Level()
	}
	return &SQLLogger{DB: db, MinLevel: level}
}

// Emit writes one structured record to the sink when the severity is at or
// above MinLevel (levels are 0..4, lower = more severe).
func (s *SQLLogger) Emit(level klog.Level, category, worker string, r klog.Record) {
	if s == nil || s.DB == nil {
		return
	}
	if level > s.MinLevel {
		return
	}
	s.DB.Emit(r)
}

// Logf records a formatted message at the given level with a data JSON object.
func (s *SQLLogger) Logf(ctx context.Context, level klog.Level, category string, r klog.Record, format string, args ...any) {
	if s == nil || s.DB == nil {
		return
	}
	if level > s.MinLevel {
		return
	}
	r.Level = level
	if r.Category == "" {
		r.Category = category
	}
	if len(args) == 0 {
		r.Message = format
	} else {
		r.Message = fmt.Sprintf(format, args...)
	}
	s.DB.Emit(r)
}

// Printf records at Info level (implements the host Logger style surface).
func (s *SQLLogger) Printf(format string, args ...any) {
	s.Logf(context.Background(), klog.LevelInfo, "app", klog.Record{}, format, args...)
}

// Errorf records at Error level.
func (s *SQLLogger) Errorf(format string, args ...any) {
	s.Logf(context.Background(), klog.LevelError, "app", klog.Record{}, format, args...)
}

// Warnf records at Warn level.
func (s *SQLLogger) Warnf(format string, args ...any) {
	s.Logf(context.Background(), klog.LevelWarn, "app", klog.Record{}, format, args...)
}

// Tracef records at Trace level.
func (s *SQLLogger) Tracef(format string, args ...any) {
	s.Logf(context.Background(), klog.LevelTrace, "app", klog.Record{}, format, args...)
}
