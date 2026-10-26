package klog

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParseError describes a malformed size/retention/level value as given on the
// command line or in KALUA.INI.
type ParseError struct {
	Kind  string // "size" | "retention" | "level"
	Value string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("invalid %s %q", e.Kind, e.Value)
}

// ParseSize parses a size expression like "16k", "1.5M", "50", "2G" (also
// columnar "16KiB"/"1MB"). A bare integer means bytes. Returns the byte count.
func ParseSize(s string) (int64, error) {
	v := strings.TrimSpace(s)
	if v == "" {
		return 0, &ParseError{"size", s}
	}
	// Split the numeric prefix from the unit suffix.
	i := 0
	for i < len(v) && (v[i] >= '0' && v[i] <= '9' || v[i] == '.') {
		i++
	}
	numStr := v[:i]
	unit := strings.TrimSpace(v[i:])
	if numStr == "" {
		return 0, &ParseError{"size", s}
	}
	num, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0, &ParseError{"size", s}
	}
	mult := int64(1)
	switch strings.ToLower(unit) {
	case "", "b":
		mult = 1
	case "k", "kb", "kib":
		mult = 1 << 10
	case "m", "mb", "mib":
		mult = 1 << 20
	case "g", "gb", "gib":
		mult = 1 << 30
	case "t", "tb", "tib":
		mult = 1 << 40
	default:
		return 0, &ParseError{"size", s}
	}
	return int64(num * float64(mult)), nil
}

// ParseRetention parses a retention duration like "7d", "24h", "30m", "2d12h"
// or "0" (keep forever). Units are d/h/m/s; the token may combine several
// (e.g. "1d12h"). "0" and ""-equivalent bare zero mean zero (keep forever).
func ParseRetention(s string) (time.Duration, error) {
	v := strings.TrimSpace(s)
	if v == "" {
		return 0, &ParseError{"retention", s}
	}
	if v == "0" {
		return 0, nil
	}
	var total time.Duration
	rest := v
	for rest != "" {
		// Number prefix.
		i := 0
		for i < len(rest) && rest[i] >= '0' && rest[i] <= '9' {
			i++
		}
		if i == 0 {
			return 0, &ParseError{"retention", s}
		}
		n, err := strconv.ParseInt(rest[:i], 10, 64)
		if err != nil {
			return 0, &ParseError{"retention", s}
		}
		rest = rest[i:]
		// Unit suffix (may be absent at the end, meaning days — Kalipso style).
		j := 0
		for j < len(rest) && isAlpha(rest[j]) {
			j++
		}
		unit := strings.ToLower(rest[:j])
		rest = rest[j:]
		var mult time.Duration
		switch unit {
		case "":
			mult = 24 * time.Hour // bare number = days
		case "d":
			mult = 24 * time.Hour
		case "h":
			mult = time.Hour
		case "m":
			mult = time.Minute
		case "s":
			mult = time.Second
		default:
			return 0, &ParseError{"retention", s}
		}
		total += time.Duration(n) * mult
	}
	return total, nil
}

func isAlpha(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}
