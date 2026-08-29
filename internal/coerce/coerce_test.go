package coerce

import "testing"

func TestTruthy(t *testing.T) {
	cases := []struct {
		name string
		v    any
		want bool
	}{
		{"nil", nil, false},
		{"false", false, false},
		{"true", true, true},
		{"zero", float64(0), false},
		{"nonzero", float64(42), true},
		{"empty string", "", false},
		{"zero string", "0", false},
		{"nonzero string", "3.5", true},
		{"text", "hello", true},
		{"table", map[string]any{}, true},
	}
	for _, tc := range cases {
		if got := Truthy(tc.v); got != tc.want {
			t.Errorf("Truthy(%v) = %v, want %v", tc.v, got, tc.want)
		}
	}
}

func TestParseNum(t *testing.T) {
	cases := []struct {
		s    string
		want float64
		ok   bool
	}{
		{"10", 10, true},
		{" 3.5 ", 3.5, true},
		{"-2", -2, true},
		{"", 0, false},
		{"abc", 0, false},
		{"0x1F", 0, false}, // hex not part of Kalipso numeric strings
	}
	for _, tc := range cases {
		got, ok := ParseNum(tc.s)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("ParseNum(%q) = (%v, %v), want (%v, %v)", tc.s, got, ok, tc.want, tc.ok)
		}
	}
}

func TestEq(t *testing.T) {
	cases := []struct {
		name string
		a, b any
		want bool
	}{
		{"documented 0 = empty string", float64(0), "", true},
		{"documented 0 = empty string reversed", "", float64(0), true},
		{"number matches numeric string", float64(1), "1", true},
		{"number vs text", float64(1), "banana", false},
		{"text equals text", "x", "x", true},
		{"string compare not numeric", "10", "09", false},
	}
	for _, tc := range cases {
		if got := Eq(tc.a, tc.b); got != tc.want {
			t.Errorf("%s: Eq(%v, %v) = %v, want %v", tc.name, tc.a, tc.b, got, tc.want)
		}
	}
}

func TestAdd(t *testing.T) {
	cases := []struct {
		name string
		a, b any
		want any
	}{
		{"numeric", float64(1), "2", 3.0},
		{"concatenation", "a", float64(1), "a1"},
		{"text concat", "x", "y", "xy"},
	}
	for _, tc := range cases {
		if got := Add(tc.a, tc.b); got != tc.want {
			t.Errorf("%s: Add(%v, %v) = %v, want %v", tc.name, tc.a, tc.b, got, tc.want)
		}
	}
}
