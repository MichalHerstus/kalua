package cli

import (
	"bytes"
	"strings"
	"testing"
)

// TestProgressWriterPlainLines verifies that on a non-terminal writer (pipes,
// files, CI logs, test buffers) stage messages are written as plain lines with
// no spinner escape codes — deterministic and diff-friendly.
func TestProgressWriterPlainLines(t *testing.T) {
	var buf bytes.Buffer
	pw := newProgressWriter(&buf)
	pw.start() // non-TTY: no-op
	pw.msg("KALUA AI: generating…")
	pw.msg("KALUA AI: validation passed (attempt 1)")
	pw.stop()
	got := buf.String()
	want := "KALUA AI: generating…\nKALUA AI: validation passed (attempt 1)\n"
	if got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
	if strings.Contains(got, "\x1b[") || strings.Contains(got, "\r") {
		t.Errorf("non-TTY output must not contain escape/control sequences: %q", got)
	}
}

// TestProgressWriterMsgAfterStop verifies a message written after stop() still
// lands as a plain line (no spinner state is left dangling).
func TestProgressWriterMsgAfterStop(t *testing.T) {
	var buf bytes.Buffer
	pw := newProgressWriter(&buf)
	pw.start()
	pw.stop()
	pw.msg("late")
	if got := buf.String(); got != "late\n" {
		t.Errorf("output = %q, want %q", got, "late\n")
	}
}

// TestProgressWriterStopIdempotent verifies stop() is safe to call twice.
func TestProgressWriterStopIdempotent(t *testing.T) {
	var buf bytes.Buffer
	pw := newProgressWriter(&buf)
	pw.start()
	pw.stop()
	pw.stop()
	if got := buf.String(); got != "" {
		t.Errorf("output = %q, want empty", got)
	}
}
