// Package coerce reproduces Kalipso's two data types (Numeric/String) and its
// weak-typing quirks for all KALUA bindings and the K.* helpers.
//
// Full Kalipso fidelity for the K.* expression surface lands in the
// expression-function milestone; see §2.3 of the spec.
package coerce

import (
	"strconv"
	"strings"
)

// ParseNum reports whether s is a Kalipso-style numeric string and its value.
// Leading/trailing whitespace is tolerated; hex and exponent forms are not
// part of the documented Kalipso numeric grammar.
func ParseNum(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// NumOrEmpty parses s numerically, treating the empty string as zero.
// Kalipso compares mixed numeric/string values numerically when both coerce,
// and "" coerces to 0 (documented example: `0 = ""` is true).
func NumOrEmpty(s string) (float64, bool) {
	if strings.TrimSpace(s) == "" {
		return 0, true
	}
	return ParseNum(s)
}

// Truthy implements Kalipso condition truthiness for If(...): numbers are
// false only when zero, the empty string is false, a numeric string is judged
// by its value, and any other string/table/function is true.
func Truthy(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case float64:
		return t != 0
	case int:
		return t != 0
	case int64:
		return t != 0
	case string:
		if t == "" {
			return false
		}
		if n, ok := ParseNum(t); ok {
			return n != 0
		}
		return true
	default:
		return true
	}
}

// Add implements the Kalipso `+`: numeric when both operands coerce to
// numbers, otherwise string concatenation of their string forms.
func Add(a, b any) any {
	an, aok := numeric(a)
	bn, bok := numeric(b)
	if aok && bok {
		return an + bn
	}
	return Stringify(a) + Stringify(b)
}

// Eq mirrors Kalipso `=`: numeric compare when both operands coerce (with ""
// as zero), string compare otherwise.
func Eq(a, b any) bool {
	an, aok := numeric(a)
	bn, bok := numeric(b)
	if aok && bok {
		return an == bn
	}
	return Stringify(a) == Stringify(b)
}

// Ne is the negation of Eq.
func Ne(a, b any) bool { return !Eq(a, b) }

// numeric extracts a float64 from a Kalipso Numeric or numeric string. Empty
// strings count as numeric zero (see NumOrEmpty).
func numeric(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case string:
		return NumOrEmpty(t)
	default:
		return 0, false
	}
}

// Stringify converts a Kalipso value to its string form.
func Stringify(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case bool:
		if t {
			return "true"
		}
		return "false"
	case string:
		return t
	default:
		return stringifyOther(t)
	}
}

// ToNum attempts to coerce v to a number per Kalipso rules (empty string → 0).
// Returns (value, true) on success, (0, false) if v cannot be interpreted as numeric.
func ToNum(v any) (float64, bool) { return numeric(v) }
