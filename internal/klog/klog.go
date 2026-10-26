// Package klog implements KALUA's self-maintained structured log stored in a
// SQLite file (klog.db). It is the queryable complement to k.print (which stays
// stdout-only): records carry a level, category, message and an optional JSON
// data object, are written in batches by a single writer goroutine, and are
// pruned by age and total size. The design is documented in
// docs/spec/serve-communication-plan.md (Phase 8).
package klog

import "time"

// Config controls the SQLite log sink. Zero values fall back to the defaults
// documented in serve-communication-plan.md Phase 8.
type Config struct {
	// Path is the database file ("klog.db" by default; fixed to the process
	// working directory so --ini and script location never move it).
	Path string
	// Level is the minimum severity accepted (records below level drop).
	Level Level
	// MaxRow caps the combined message+data size of one record (16 KiB).
	MaxRow int64
	// Retain is the maximum record age; zero keeps everything.
	Retain time.Duration
	// MaxMB caps the log file size; zero means unlimited. Prune removes
	// oldest rows until 80% low-water.
	MaxMB int64
}

// Defaults mirror the serve-communication-plan.md flag table.
const (
	DefaultPath   = "klog.db"
	DefaultMaxRow = 16 * 1024
)

// Level ranks a log record. Lower values are more severe.
type Level int

const (
	LevelError Level = iota // 0
	LevelWarn               // 1
	LevelInfo               // 2
	LevelDebug              // 3
	LevelTrace              // 4
)

// Number of severity levels (0..LevelTrace, inclusive).
const NumLevels = 5

// ValidLevel reports whether l is a defined severity level.
func ValidLevel(l Level) bool { return l >= LevelError && l <= LevelTrace }

// ParseLevel converts a level name to a Level. Accepts "error", "warn",
// "info", "debug", "trace" (case-insensitive). An unknown name is an error.
func ParseLevel(s string) (Level, error) {
	switch s {
	case "error", "ERROR":
		return LevelError, nil
	case "warn", "warning", "WARN", "WARNING":
		return LevelWarn, nil
	case "info", "INFO":
		return LevelInfo, nil
	case "debug", "DEBUG":
		return LevelDebug, nil
	case "trace", "TRACE":
		return LevelTrace, nil
	}
	return 0, &ParseError{"level", s}
}

func (l Level) String() string {
	switch l {
	case LevelError:
		return "error"
	case LevelWarn:
		return "warn"
	case LevelInfo:
		return "info"
	case LevelDebug:
		return "debug"
	case LevelTrace:
		return "trace"
	}
	return "unknown"
}

// Record is one structured log entry.
type Record struct {
	Time       time.Time // set by Emit when zero
	Level      Level
	Category   string // http | ws | tcp | worker | app | db | lifecycle ...
	Worker     int
	Session    string
	RemoteAddr string
	Message    string
	Data       string // valid JSON object of structured fields ("" when none)
}

// Stats is a snapshot of sink health for k.log.stats().
type Stats struct {
	Rows    int64
	Dropped int64
	Oldest  time.Time
	Newest  time.Time
	Path    string
}

// TruncatedSuffix is appended to a message that exceeded the per-row cap.
const TruncatedSuffix = "…[truncated]"

// TruncateMessage shortens s so len(s) <= max, cutting at a rune boundary and
// appending TruncatedSuffix. A max smaller than the suffix yields just the
// suffix.
func TruncateMessage(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	suffixLen := len(TruncatedSuffix)
	if max <= suffixLen {
		return TruncatedSuffix[:max]
	}
	// Keep max-suffixLen bytes, trimmed to a rune boundary.
	keep := max - suffixLen
	if keep > len(s) {
		keep = len(s)
	}
	// Walk back to a rune boundary.
	for keep > 0 && s[keep-1] >= 0x80 {
		// Find the start of the rune containing keep-1.
		start := keep - 1
		for start > 0 && s[start]&0xC0 == 0x80 {
			start--
		}
		// If a complete rune starts at start and ends at keep, keep it.
		if start >= 0 && start+1 == keep {
			// 1-byte rune that was misclassified? impossible after the loop.
		}
		keep = start
		if keep <= 0 {
			break
		}
	}
	return s[:keep] + TruncatedSuffix
}

// DataEnvelope describes a truncated data payload.
func dataEnvelope(origBytes int, preview string) string {
	return `{"_truncated":true,"_orig_bytes":` + itoa(origBytes) + `,"_preview":` + quoteJSON(preview) + `}`
}

// TruncateData caps data so that the combined Message+Data fits inside cap
// (message bytes already counted). When data cannot fit (even a 1-byte
// preview would overflow), it is replaced with a minimal valid envelope.
func TruncateData(data string, max int) string {
	if max <= 0 || len(data) <= max {
		return data
	}
	minEnvelope := dataEnvelope(0, "")
	if max < len(minEnvelope) {
		return `{"_truncated":true}`
	}
	// Target half the remaining budget for the preview.
	previewLen := (max - len(dataEnvelope(0, ""))) / 2
	if previewLen < 0 {
		previewLen = 0
	}
	preview := TruncateMessage(data, previewLen)
	return dataEnvelope(len(data), preview)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func quoteJSON(s string) string {
	out := make([]byte, 0, len(s)+2)
	out = append(out, '"')
	for _, r := range s {
		switch r {
		case '"':
			out = append(out, '\\', '"')
		case '\\':
			out = append(out, '\\', '\\')
		case '\n':
			out = append(out, '\\', 'n')
		case '\r':
			out = append(out, '\\', 'r')
		case '\t':
			out = append(out, '\\', 't')
		default:
			out = append(out, string(r)...)
		}
	}
	out = append(out, '"')
	return string(out)
}
