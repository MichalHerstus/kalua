package klog

import (
	"testing"
	"time"
)

func TestParseLevel(t *testing.T) {
	cases := map[string]Level{
		"error": LevelError,
		"warn":  LevelWarn,
		"info":  LevelInfo,
		"debug": LevelDebug,
		"trace": LevelTrace,
		"ERROR": LevelError,
		"INFO":  LevelInfo,
	}
	for in, want := range cases {
		got, err := ParseLevel(in)
		if err != nil {
			t.Errorf("ParseLevel(%q) error: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("ParseLevel(%q) = %d, want %d", in, got, want)
		}
	}
	if _, err := ParseLevel("verbose"); err == nil {
		t.Error("ParseLevel(verbose) should error")
	}
}

func TestTruncateMessage(t *testing.T) {
	s := "abcdefghij"
	if got := TruncateMessage(s, 100); got != s {
		t.Errorf("no-trunc case: got %q", got)
	}
	got := TruncateMessage(s, 5)
	// "abcde" + "…[truncated]" (7 bytes) would be 12 > 5: max smaller than
	// suffix+1 yields just the suffix, limited to max bytes.
	if len(got) > 5 {
		t.Errorf("truncated to %d bytes, want <= 5: %q", len(got), got)
	}
	got10 := TruncateMessage(s, 10)
	if got10 != s {
		t.Errorf("exact fit should be untouched, got %q", got10)
	}
	// Rune boundary: multi-byte rune should not be cut.
	uni := "héllo wörld"
	tr := TruncateMessage(uni, 6)
	for _, r := range tr {
		if r == 0xFFFD {
			t.Errorf("truncated mid-rune: %q", tr)
		}
	}
}

func TestTruncateData(t *testing.T) {
	// Under cap → untouched.
	if got := TruncateData(`{"a":1}`, 100); got != `{"a":1}` {
		t.Errorf("no-trunc: got %q", got)
	}
	// Over cap → valid envelope with _truncated flag.
	got := TruncateData(`{"pad":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","long":"a very long value"}`, 40)
	if len(got) > 40 {
		t.Errorf("data exceeded cap: %d bytes %q", len(got), got)
	}
	if got == `{"pad":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","long":"a very long value"}` {
		t.Errorf("expected truncation, got %q", got)
	}
	if got[0] != '{' || got[len(got)-1] != '}' {
		t.Errorf("truncated data is not a valid envelope: %q", got)
	}
	// Heavily capped → minimal valid envelope (cannot compress below itself).
	got = TruncateData(`{"a":"a very long value"}`, 2)
	if got != `{"_truncated":true}` {
		t.Errorf("minimal cap: got %q", got)
	}
}

func TestParseSize(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"", 0},
		{"0", 0},
		{"50", 50},
		{"16k", 16 << 10},
		{"1.5M", int64(1.5 * (1 << 20))},
		{"2G", 2 << 30},
		{"10KB", 10 << 10},
	}
	for _, c := range cases {
		if c.in == "" {
			if _, err := ParseSize(c.in); err == nil {
				t.Errorf("ParseSize(%q) should error", c.in)
			}
			continue
		}
		got, err := ParseSize(c.in)
		if err != nil {
			t.Errorf("ParseSize(%q) error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseSize(%q) = %d, want %d", c.in, got, c.want)
		}
	}
	if _, err := ParseSize("abc"); err == nil {
		t.Error("ParseSize(abc) should error")
	}
}

func TestParseRetention(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
		err  bool
	}{
		{"0", 0, false},
		{"7d", 7 * 24 * time.Hour, false},
		{"24h", 24 * time.Hour, false},
		{"30m", 30 * time.Minute, false},
		{"2d12h", 2*24*time.Hour + 12*time.Hour, false},
		{"10s", 10 * time.Second, false},
		{"abc", 0, true},
	}
	for _, c := range cases {
		got, err := ParseRetention(c.in)
		if c.err {
			if err == nil {
				t.Errorf("ParseRetention(%q) should error", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseRetention(%q) error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseRetention(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
