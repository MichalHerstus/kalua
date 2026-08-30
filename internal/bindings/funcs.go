// Package bindings implements the §5.9 expression-function library: Kalipso
// expression functions exposed as flat globals (not under k.*) so expressions
// read like the original syntax. Types follow the spec's date convention:
// strings are "YYYY-MM-DD[ HH:MM[:SS]]".
//
// Names intentionally match Kalipso: left/right/middle, abs/round/ceiling,
// lookup/yesno/iif, sys_date/sys_time, ... Where the documented behavior is
// ambiguous, the implementation picks the Lua-flavoured reading and is pinned
// by funcs_test.go.
package bindings

import (
	crand "crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math"
	"math/rand"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/yuin/gopher-lua"

	"kalua/internal/coerce"
)

// registerExprFuncs installs the §5.9 expression functions as flat globals.
func registerExprFuncs(e *Env) {
	g := func(name string, fn lua.LGFunction) {
		e.L.SetGlobal(name, e.L.NewFunction(fn))
	}

	// -------- String --------
	g("left", func(L *lua.LState) int { // left(s, n) — first n chars
		s := L.CheckString(1)
		n := L.CheckInt(2)
		if n < 0 {
			n = 0
		}
		if n > len(s) {
			n = len(s)
		}
		L.Push(lua.LString(s[:n]))
		return 1
	})
	g("right", func(L *lua.LState) int { // right(s, n) — last n chars
		s := L.CheckString(1)
		n := L.CheckInt(2)
		if n < 0 {
			n = 0
		}
		if n > len(s) {
			n = len(s)
		}
		L.Push(lua.LString(s[len(s)-n:]))
		return 1
	})
	g("middle", func(L *lua.LState) int { // middle(s, start1, count) — 1-based
		s := L.CheckString(1)
		start := L.CheckInt(2)
		count := L.CheckInt(3)
		if start < 1 {
			start = 1
		}
		if count < 0 {
			count = 0
		}
		if start > len(s)+1 {
			start = len(s) + 1
		}
		end := start + count
		if end > len(s)+1 {
			end = len(s) + 1
		}
		L.Push(lua.LString(s[start-1 : end-1]))
		return 1
	})
	g("length", func(L *lua.LState) int { // length(s) — byte length (Lua # semantics)
		L.Push(lua.LNumber(len(L.CheckString(1))))
		return 1
	})
	g("replace", func(L *lua.LState) int { // replace(s, old, new) — all occurrences
		s := L.CheckString(1)
		old := L.CheckString(2)
		new := L.CheckString(3)
		if old == "" {
			L.Push(lua.LString(s))
			return 1
		}
		L.Push(lua.LString(strings.ReplaceAll(s, old, new)))
		return 1
	})
	g("trim", func(L *lua.LState) int { // trim(s) — strip surrounding whitespace
		L.Push(lua.LString(strings.TrimSpace(L.CheckString(1))))
		return 1
	})
	g("upper", func(L *lua.LState) int {
		L.Push(lua.LString(strings.ToUpper(L.CheckString(1))))
		return 1
	})
	g("lower", func(L *lua.LState) int {
		L.Push(lua.LString(strings.ToLower(L.CheckString(1))))
		return 1
	})
	g("find", func(L *lua.LState) int { // find(s, needle[, start1]) — 1-based pos or 0
		s := L.CheckString(1)
		needle := L.CheckString(2)
		from := L.OptInt(3, 1)
		if from < 1 {
			from = 1
		}
		if from > len(s) {
			L.Push(lua.LNumber(0))
			return 1
		}
		idx := strings.Index(s[from-1:], needle)
		if idx < 0 {
			L.Push(lua.LNumber(0))
			return 1
		}
		L.Push(lua.LNumber(idx + from))
		return 1
	})
	g("string_count", func(L *lua.LState) int { // string_count(s, needle) — non-overlapping
		s := L.CheckString(1)
		needle := L.CheckString(2)
		if needle == "" {
			L.Push(lua.LNumber(0))
			return 1
		}
		L.Push(lua.LNumber(strings.Count(s, needle)))
		return 1
	})
	g("complete", func(L *lua.LState) int { // complete(s, length[, pad]) — right-pad/truncate
		s := L.CheckString(1)
		n := L.CheckInt(2)
		pad := L.OptString(3, " ")
		if len(s) >= n {
			L.Push(lua.LString(s[:n]))
			return 1
		}
		need := n - len(s)
		if pad == "" {
			pad = " "
		}
		padStr := strings.Repeat(pad[:1], (need+len(pad)-1)/len(pad))[:need]
		L.Push(lua.LString(s + padStr))
		return 1
	})
	g("ascii", func(L *lua.LState) int { // ascii(ch) — code of first byte
		s := L.CheckString(1)
		if s == "" {
			L.Push(lua.LNumber(0))
			return 1
		}
		L.Push(lua.LNumber(s[0]))
		return 1
	})
	g("charact", func(L *lua.LState) int { // charact(code) — byte from code
		L.Push(lua.LString(string(rune(L.CheckInt(1)))))
		return 1
	})
	g("base64_encode", func(L *lua.LState) int {
		L.Push(lua.LString(base64.StdEncoding.EncodeToString([]byte(L.CheckString(1)))))
		return 1
	})
	g("base64_decode", func(L *lua.LState) int {
		b, err := base64.StdEncoding.DecodeString(L.CheckString(1))
		if err != nil {
			L.RaiseError("base64_decode error: %v", err)
			return 0
		}
		L.Push(lua.LString(b))
		return 1
	})
	g("urlencode", func(L *lua.LState) int { // query-style encoding (space -> +)
		L.Push(lua.LString(url.QueryEscape(L.CheckString(1))))
		return 1
	})
	g("urldecode", func(L *lua.LState) int {
		s, err := url.QueryUnescape(L.CheckString(1))
		if err != nil {
			L.RaiseError("urldecode error: %v", err)
			return 0
		}
		L.Push(lua.LString(s))
		return 1
	})
	// encode/decode are aliases of urlencode/urldecode in the Kalipso surface.
	g("encode", func(L *lua.LState) int {
		L.Push(lua.LString(url.QueryEscape(L.CheckString(1))))
		return 1
	})
	g("decode", func(L *lua.LState) int {
		s, err := url.QueryUnescape(L.CheckString(1))
		if err != nil {
			L.RaiseError("decode error: %v", err)
			return 0
		}
		L.Push(lua.LString(s))
		return 1
	})
	g("full_encode", func(L *lua.LState) int { // percent-encode every non-unreserved byte
		s := L.CheckString(1)
		var sb strings.Builder
		for i := 0; i < len(s); i++ {
			c := s[i]
			if isUnreservedByte(c) {
				sb.WriteByte(c)
			} else {
				fmt.Fprintf(&sb, "%%%02X", c)
			}
		}
		L.Push(lua.LString(sb.String()))
		return 1
	})
	g("jsonencode", func(L *lua.LState) int { // alias of k.json_string
		out, err := stringifyJSON(e, L.Get(1))
		if err != nil {
			L.RaiseError("jsonencode error: %v", err)
			return 0
		}
		L.Push(lua.LString(out))
		return 1
	})
	g("jsondecode", func(L *lua.LState) int { // alias of k.json_parse
		v, err := parseJSON(L, e, []byte(L.CheckString(1)))
		if err != nil {
			L.RaiseError("jsondecode error: %v", err)
			return 0
		}
		L.Push(v)
		return 1
	})
	g("xmlencode", func(L *lua.LState) int { // escape & < > " '
		L.Push(lua.LString(xmlEscape(L.CheckString(1))))
		return 1
	})
	g("xmldecode", func(L *lua.LState) int {
		L.Push(lua.LString(xmlUnescape(L.CheckString(1))))
		return 1
	})
	g("guid", func(L *lua.LState) int { // random UUID v4
		L.Push(lua.LString(newGUID()))
		return 1
	})
	g("extract_string", func(L *lua.LState) int { // extract_string(s, start1, end1) inclusive
		s := L.CheckString(1)
		start := L.CheckInt(2)
		end := L.CheckInt(3)
		if start < 1 {
			start = 1
		}
		if end < start {
			L.Push(lua.LString(""))
			return 1
		}
		if end > len(s) {
			end = len(s)
		}
		L.Push(lua.LString(s[start-1 : end]))
		return 1
	})
	g("set_string", func(L *lua.LState) int { // set_string(s, start1, count, new) — replace run
		s := L.CheckString(1)
		start := L.CheckInt(2)
		count := L.CheckInt(3)
		new := L.CheckString(4)
		if start < 1 {
			start = 1
		}
		if count < 0 {
			count = 0
		}
		end := start + count - 1
		if end > len(s) {
			end = len(s)
		}
		L.Push(lua.LString(s[:start-1] + new + s[end:]))
		return 1
	})
	g("file_extract_part", func(L *lua.LState) int { // file_extract_part(path, part)
		p := filepath.Clean(L.CheckString(1))
		part := strings.ToLower(L.OptString(2, "name"))
		switch part {
		case "path", "folder", "directory":
			L.Push(lua.LString(filepath.Dir(p)))
		case "ext", "extension":
			L.Push(lua.LString(strings.TrimPrefix(filepath.Ext(p), ".")))
		case "file", "filename":
			L.Push(lua.LString(filepath.Base(p)))
		default: // name = base without extension
			base := filepath.Base(p)
			ext := filepath.Ext(base)
			L.Push(lua.LString(strings.TrimSuffix(base, ext)))
		}
		return 1
	})
	g("mltext", func(L *lua.LState) int { // mltext(...) — join args with newlines
		parts := make([]string, 0, L.GetTop())
		for i := 1; i <= L.GetTop(); i++ {
			parts = append(parts, L.Get(i).String())
		}
		L.Push(lua.LString(strings.Join(parts, "\n")))
		return 1
	})

	// -------- Numeric --------
	g("abs", func(L *lua.LState) int {
		L.Push(lua.LNumber(math.Abs(float64(L.CheckNumber(1)))))
		return 1
	})
	g("round", func(L *lua.LState) int { // round(x[, decimals])
		x := float64(L.CheckNumber(1))
		dec := L.OptNumber(2, 0)
		f := math.Pow(10, float64(dec))
		L.Push(lua.LNumber(math.Round(x*f) / f))
		return 1
	})
	g("floor", func(L *lua.LState) int {
		L.Push(lua.LNumber(math.Floor(float64(L.CheckNumber(1)))))
		return 1
	})
	g("ceiling", func(L *lua.LState) int {
		L.Push(lua.LNumber(math.Ceil(float64(L.CheckNumber(1)))))
		return 1
	})
	g("power", func(L *lua.LState) int {
		L.Push(lua.LNumber(math.Pow(float64(L.CheckNumber(1)), float64(L.CheckNumber(2)))))
		return 1
	})
	g("nth_root", func(L *lua.LState) int { // nth_root(x, n) — x^(1/n)
		L.Push(lua.LNumber(math.Pow(float64(L.CheckNumber(1)), 1/float64(L.CheckNumber(2)))))
		return 1
	})
	g("sqrt", func(L *lua.LState) int {
		L.Push(lua.LNumber(math.Sqrt(float64(L.CheckNumber(1)))))
		return 1
	})
	g("exp", func(L *lua.LState) int {
		L.Push(lua.LNumber(math.Exp(float64(L.CheckNumber(1)))))
		return 1
	})
	g("log", func(L *lua.LState) int {
		L.Push(lua.LNumber(math.Log(float64(L.CheckNumber(1)))))
		return 1
	})
	g("log10", func(L *lua.LState) int {
		L.Push(lua.LNumber(math.Log10(float64(L.CheckNumber(1)))))
		return 1
	})
	g("sin", func(L *lua.LState) int {
		L.Push(lua.LNumber(math.Sin(float64(L.CheckNumber(1)))))
		return 1
	})
	g("cos", func(L *lua.LState) int {
		L.Push(lua.LNumber(math.Cos(float64(L.CheckNumber(1)))))
		return 1
	})
	g("tan", func(L *lua.LState) int {
		L.Push(lua.LNumber(math.Tan(float64(L.CheckNumber(1)))))
		return 1
	})
	g("asin", func(L *lua.LState) int {
		L.Push(lua.LNumber(math.Asin(float64(L.CheckNumber(1)))))
		return 1
	})
	g("acos", func(L *lua.LState) int {
		L.Push(lua.LNumber(math.Acos(float64(L.CheckNumber(1)))))
		return 1
	})
	g("atan", func(L *lua.LState) int {
		L.Push(lua.LNumber(math.Atan(float64(L.CheckNumber(1)))))
		return 1
	})
	g("deg2rad", func(L *lua.LState) int {
		L.Push(lua.LNumber(float64(L.CheckNumber(1)) * math.Pi / 180))
		return 1
	})
	g("rad2deg", func(L *lua.LState) int {
		L.Push(lua.LNumber(float64(L.CheckNumber(1)) * 180 / math.Pi))
		return 1
	})
	g("bitwise_and", func(L *lua.LState) int {
		L.Push(lua.LNumber(int64(L.CheckNumber(1)) & int64(L.CheckNumber(2))))
		return 1
	})
	g("bitwise_or", func(L *lua.LState) int {
		L.Push(lua.LNumber(int64(L.CheckNumber(1)) | int64(L.CheckNumber(2))))
		return 1
	})
	g("bitwise_xor", func(L *lua.LState) int {
		L.Push(lua.LNumber(int64(L.CheckNumber(1)) ^ int64(L.CheckNumber(2))))
		return 1
	})
	g("random", func(L *lua.LState) int { // random() / random(max) / random(min, max)
		switch L.GetTop() {
		case 0:
			L.Push(lua.LNumber(rand.Float64()))
		case 1:
			max := L.CheckInt(1)
			if max < 1 {
				max = 1
			}
			L.Push(lua.LNumber(rand.Intn(max) + 1))
		default:
			min := L.CheckInt(1)
			max := L.CheckInt(2)
			if max <= min {
				L.Push(lua.LNumber(min))
			} else {
				L.Push(lua.LNumber(rand.Intn(max-min) + min))
			}
		}
		return 1
	})
	g("int_part", func(L *lua.LState) int {
		L.Push(lua.LNumber(math.Trunc(float64(L.CheckNumber(1)))))
		return 1
	})
	g("dec_part", func(L *lua.LState) int {
		x := float64(L.CheckNumber(1))
		L.Push(lua.LNumber(x - math.Trunc(x)))
		return 1
	})
	g("mask_number", func(L *lua.LState) int { // mask_number(x, mask) — # 0 , . group/zero formatting
		mask := L.CheckString(2)
		out, err := maskNumber(float64(L.CheckNumber(1)), mask)
		if err != nil {
			L.RaiseError("mask_number error: %v", err)
			return 0
		}
		L.Push(lua.LString(out))
		return 1
	})
	g("val", func(L *lua.LState) int { // val(...) — numeric value of the argument (0 if not numeric)
		if n, ok := coerce.ToNum(v(L.Get(1))); ok {
			L.Push(lua.LNumber(n))
		} else {
			L.Push(lua.LNumber(0))
		}
		return 1
	})
	g("sum", func(L *lua.LState) int { // sum(table) — sum of array of numbers
		tbl := L.CheckTable(1)
		total := 0.0
		tbl.ForEach(func(_, lv lua.LValue) {
			if n, ok := coerce.ToNum(v(lv)); ok && lv.Type() != lua.LTBool {
				total += n
			}
		})
		L.Push(lua.LNumber(total))
		return 1
	})
	g("extractstringd", func(L *lua.LState) int { // extractstringd(s) — digits of the string as a number
		var digits []byte
		for _, c := range []byte(L.CheckString(1)) {
			if c >= '0' && c <= '9' {
				digits = append(digits, c)
			}
		}
		if len(digits) == 0 {
			L.Push(lua.LNumber(0))
			return 1
		}
		n, _ := strconv.Atoi(string(digits))
		L.Push(lua.LNumber(n))
		return 1
	})

	// -------- Conditional --------
	g("lookup", func(L *lua.LState) int { // lookup(key, k1, v1, k2, v2, ...) — "" if absent
		key := v(L.Get(1))
		top := L.GetTop()
		i := 2
		for i+1 <= top {
			if coerce.Eq(key, v(L.Get(i))) {
				L.Push(L.Get(i + 1))
				return 1
			}
			i += 2
		}
		L.Push(lua.LString(""))
		return 1
	})
	g("yesno", func(L *lua.LState) int { // yesno(cond, a, b) — a when true, b otherwise
		cond := coerce.Truthy(v(L.Get(1)))
		if cond {
			L.Push(L.Get(2))
		} else {
			L.Push(L.Get(3))
		}
		return 1
	})
	g("iif", func(L *lua.LState) int { // iif(cond, a, b) — inline if, value-preserving
		cond := coerce.Truthy(v(L.Get(1)))
		if cond {
			L.Push(L.Get(2))
		} else {
			L.Push(L.Get(3))
		}
		return 1
	})

	// -------- Date/Time --------
	g("sys_date", func(L *lua.LState) int { // sys_date() — "YYYY-MM-DD"
		L.Push(lua.LString(time.Now().Format("2006-01-02")))
		return 1
	})
	g("sys_time", func(L *lua.LState) int { // sys_time() — "HH:MM:SS"
		L.Push(lua.LString(time.Now().Format("15:04:05")))
		return 1
	})
	g("day", func(L *lua.LState) int {
		t, ok := parseDateStr(L.CheckString(1))
		if !ok {
			L.RaiseError("day: invalid date %q", L.CheckString(1))
			return 0
		}
		L.Push(lua.LNumber(t.Day()))
		return 1
	})
	g("month", func(L *lua.LState) int {
		t, ok := parseDateStr(L.CheckString(1))
		if !ok {
			L.RaiseError("month: invalid date %q", L.CheckString(1))
			return 0
		}
		L.Push(lua.LNumber(int(t.Month())))
		return 1
	})
	g("year", func(L *lua.LState) int {
		t, ok := parseDateStr(L.CheckString(1))
		if !ok {
			L.RaiseError("year: invalid date %q", L.CheckString(1))
			return 0
		}
		L.Push(lua.LNumber(t.Year()))
		return 1
	})
	g("hour", func(L *lua.LState) int {
		t, ok := parseDateStr(L.CheckString(1))
		if !ok {
			L.RaiseError("hour: invalid time %q", L.CheckString(1))
			return 0
		}
		L.Push(lua.LNumber(t.Hour()))
		return 1
	})
	g("minute", func(L *lua.LState) int {
		t, ok := parseDateStr(L.CheckString(1))
		if !ok {
			L.RaiseError("minute: invalid time %q", L.CheckString(1))
			return 0
		}
		L.Push(lua.LNumber(t.Minute()))
		return 1
	})
	g("second", func(L *lua.LState) int {
		t, ok := parseDateStr(L.CheckString(1))
		if !ok {
			L.RaiseError("second: invalid time %q", L.CheckString(1))
			return 0
		}
		L.Push(lua.LNumber(t.Second()))
		return 1
	})
	g("add_days", func(L *lua.LState) int { // add_days(date, n) — "YYYY-MM-DD"
		t, ok := parseDateStr(L.CheckString(1))
		if !ok {
			L.RaiseError("add_days: invalid date %q", L.CheckString(1))
			return 0
		}
		L.Push(lua.LString(t.AddDate(0, 0, L.CheckInt(2)).Format("2006-01-02")))
		return 1
	})
	g("subtract_days", func(L *lua.LState) int {
		t, ok := parseDateStr(L.CheckString(1))
		if !ok {
			L.RaiseError("subtract_days: invalid date %q", L.CheckString(1))
			return 0
		}
		L.Push(lua.LString(t.AddDate(0, 0, -L.CheckInt(2)).Format("2006-01-02")))
		return 1
	})
	g("date_diff", func(L *lua.LState) int { // date_diff(d2, d1) — days between (2 minus 1)
		t2, ok2 := parseDateStr(L.CheckString(1))
		t1, ok1 := parseDateStr(L.CheckString(2))
		if !ok2 || !ok1 {
			L.RaiseError("date_diff: invalid date argument")
			return 0
		}
		L.Push(lua.LNumber(math.Round(t2.Sub(t1).Hours() / 24)))
		return 1
	})
	g("datetime_add", func(L *lua.LState) int { // datetime_add(dt, days[, hours[, minutes[, seconds]]])
		t, ok := parseDateStr(L.CheckString(1))
		if !ok {
			L.RaiseError("datetime_add: invalid datetime %q", L.CheckString(1))
			return 0
		}
		days := L.OptInt(2, 0)
		hours := L.OptInt(3, 0)
		mins := L.OptInt(4, 0)
		secs := L.OptInt(5, 0)
		t = t.AddDate(0, 0, days).Add(time.Duration(hours)*time.Hour + time.Duration(mins)*time.Minute + time.Duration(secs)*time.Second)
		L.Push(lua.LString(t.Format("2006-01-02 15:04:05")))
		return 1
	})
	g("datetime_sub", func(L *lua.LState) int {
		t, ok := parseDateStr(L.CheckString(1))
		if !ok {
			L.RaiseError("datetime_sub: invalid datetime %q", L.CheckString(1))
			return 0
		}
		days := L.OptInt(2, 0)
		hours := L.OptInt(3, 0)
		mins := L.OptInt(4, 0)
		secs := L.OptInt(5, 0)
		t = t.AddDate(0, 0, -days).Add(-(time.Duration(hours)*time.Hour + time.Duration(mins)*time.Minute + time.Duration(secs)*time.Second))
		L.Push(lua.LString(t.Format("2006-01-02 15:04:05")))
		return 1
	})
	g("datetime_diff", func(L *lua.LState) int { // datetime_diff(dt2, dt1) — seconds between
		t2, ok2 := parseDateStr(L.CheckString(1))
		t1, ok1 := parseDateStr(L.CheckString(2))
		if !ok2 || !ok1 {
			L.RaiseError("datetime_diff: invalid datetime argument")
			return 0
		}
		L.Push(lua.LNumber(t2.Sub(t1).Seconds()))
		return 1
	})
	g("date_to_string", func(L *lua.LState) int { // date_to_string(date[, format]) — %Y %m %d tokens
		t, ok := parseDateStr(L.CheckString(1))
		if !ok {
			L.RaiseError("date_to_string: invalid date %q", L.CheckString(1))
			return 0
		}
		format := L.OptString(2, "%Y-%m-%d")
		L.Push(lua.LString(formatDate(t, format)))
		return 1
	})
	g("time_to_string", func(L *lua.LState) int { // time_to_string(time[, format]) — same tokens
		t, ok := parseDateStr(L.CheckString(1))
		if !ok {
			L.RaiseError("time_to_string: invalid time %q", L.CheckString(1))
			return 0
		}
		format := L.OptString(2, "%H:%M:%S")
		L.Push(lua.LString(formatDate(t, format)))
		return 1
	})
	g("week_day", func(L *lua.LState) int { // week_day(date) — 1..7 (Sunday=1)
		t, ok := parseDateStr(L.CheckString(1))
		if !ok {
			L.RaiseError("week_day: invalid date %q", L.CheckString(1))
			return 0
		}
		L.Push(lua.LNumber(int(t.Weekday()) + 1))
		return 1
	})
	g("week_number", func(L *lua.LState) int { // week_number(date) — ISO week
		t, ok := parseDateStr(L.CheckString(1))
		if !ok {
			L.RaiseError("week_number: invalid date %q", L.CheckString(1))
			return 0
		}
		_, wk := t.ISOWeek()
		L.Push(lua.LNumber(wk))
		return 1
	})
	g("tick_count", func(L *lua.LState) int { // tick_count() — unix milliseconds
		L.Push(lua.LNumber(time.Now().UnixNano() / int64(time.Millisecond)))
		return 1
	})
	g("julian", func(L *lua.LState) int { // julian(date) — Julian day number
		t, ok := parseDateStr(L.CheckString(1))
		if !ok {
			L.RaiseError("julian: invalid date %q", L.CheckString(1))
			return 0
		}
		L.Push(lua.LNumber(julianDay(t)))
		return 1
	})
	g("utc_to_local", func(L *lua.LState) int { // utc_to_local(dt) — "YYYY-MM-DD HH:MM:SS"
		t, ok := parseDateStr(L.CheckString(1))
		if !ok {
			L.RaiseError("utc_to_local: invalid datetime %q", L.CheckString(1))
			return 0
		}
		L.Push(lua.LString(t.Local().Format("2006-01-02 15:04:05")))
		return 1
	})
	g("local_to_utc", func(L *lua.LState) int {
		t, ok := parseLocalDateStr(L.CheckString(1))
		if !ok {
			L.RaiseError("local_to_utc: invalid datetime %q", L.CheckString(1))
			return 0
		}
		L.Push(lua.LString(t.UTC().Format("2006-01-02 15:04:05")))
		return 1
	})

	// -------- Conversion --------
	g("tostr", func(L *lua.LState) int { // Kalipso string form
		L.Push(lua.LString(coerce.Stringify(v(L.Get(1)))))
		return 1
	})
	g("tonum", func(L *lua.LState) int { // Kalipso number, 0 when not numeric
		if n, ok := coerce.ToNum(v(L.Get(1))); ok {
			L.Push(lua.LNumber(n))
		} else {
			L.Push(lua.LNumber(0))
		}
		return 1
	})
	g("todate", func(L *lua.LState) int { // todate(s) — normalized "YYYY-MM-DD"
		t, ok := parseDateStr(L.CheckString(1))
		if !ok {
			L.RaiseError("todate: invalid date %q", L.CheckString(1))
			return 0
		}
		L.Push(lua.LString(t.Format("2006-01-02")))
		return 1
	})
	g("strtodate", func(L *lua.LState) int { // strtodate(s[, format]) — parse with format, emit YYYY-MM-DD
		s := L.CheckString(1)
		if format := L.OptString(2, ""); format != "" {
			t, err := parseWithFormat(s, format)
			if err != nil {
				L.RaiseError("strtodate error: %v", err)
				return 0
			}
			L.Push(lua.LString(t.Format("2006-01-02")))
			return 1
		}
		t, ok := parseDateStr(s)
		if !ok {
			L.RaiseError("strtodate: invalid date %q", s)
			return 0
		}
		L.Push(lua.LString(t.Format("2006-01-02")))
		return 1
	})
	g("boolstr", func(L *lua.LState) int { // boolstr(v) — "true"/"false"
		L.Push(lua.LString(strconv.FormatBool(coerce.Truthy(v(L.Get(1))))))
		return 1
	})
}

// -------- helpers --------

// parseDateStr parses a Kalipso date/datetime string. Layouts tried cover
// "YYYY-MM-DD", "YYYY-MM-DD HH:MM" and "YYYY-MM-DD HH:MM:SS". Parsing is
// timezone-agnostic: the returned time keeps the literal wall-clock fields so
// day/month/hour/... and date arithmetic are deterministic regardless of the
// host zone. utc_to_local/local_to_utc perform the only explicit conversions.
func parseDateStr(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	layouts := []string{
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
	}
	for _, l := range layouts {
		t, err := time.Parse(l, s)
		if err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// parseLocalDateStr parses assuming the value is already wall-clock local time.
func parseLocalDateStr(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	layouts := []string{
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
	}
	for _, l := range layouts {
		t, err := time.ParseInLocation(l, s, time.Local)
		if err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// formatDate applies %-tokens from a Kalipso date format string. Supported
// tokens: %Y (4-digit year), %y (2-digit), %m (month 2-digit), %d (day),
// %H (24h), %M (minute), %S (second), %% (%). Everything else is literal.
func formatDate(t time.Time, format string) string {
	var sb strings.Builder
	for i := 0; i < len(format); i++ {
		if format[i] != '%' || i+1 >= len(format) {
			sb.WriteByte(format[i])
			continue
		}
		i++
		switch format[i] {
		case 'Y':
			sb.WriteString(strconv.Itoa(t.Year()))
		case 'y':
			sb.WriteString(strconv.Itoa(t.Year() % 100))
		case 'm':
			sb.WriteString(fmt.Sprintf("%02d", int(t.Month())))
		case 'd':
			sb.WriteString(fmt.Sprintf("%02d", t.Day()))
		case 'H':
			sb.WriteString(fmt.Sprintf("%02d", t.Hour()))
		case 'M':
			sb.WriteString(fmt.Sprintf("%02d", t.Minute()))
		case 'S':
			sb.WriteString(fmt.Sprintf("%02d", t.Second()))
		case '%':
			sb.WriteByte('%')
		default:
			sb.WriteByte('%')
			sb.WriteByte(format[i])
		}
	}
	return sb.String()
}

// parseWithFormat parses s using a strftime-like format token set, handling
// the inverse of formatDate. Unsupported tokens are treated literally.
func parseWithFormat(s, format string) (time.Time, error) {
	// Fast path: the common re-parseable form "YYYY-MM-DD".
	if t, ok := parseDateStr(s); ok {
		return t, nil
	}
	type token struct {
		isTok bool
		b     byte
		lit   byte
	}
	var toks []token
	for i := 0; i < len(format); i++ {
		if format[i] == '%' && i+1 < len(format) {
			i++
			switch format[i] {
			case 'Y', 'y', 'm', 'd', 'H', 'M', 'S':
				toks = append(toks, token{isTok: true, b: format[i]})
			default:
				toks = append(toks, token{lit: format[i]})
			}
			continue
		}
		toks = append(toks, token{lit: format[i]})
	}

	sIdx := 0
	var year, month, day, hour, min, sec int
	getDayN := func(n, max int) (int, bool) {
		if sIdx+n > len(s) {
			return 0, false
		}
		v, err := strconv.Atoi(s[sIdx : sIdx+n])
		if err != nil || v > max {
			return 0, false
		}
		sIdx += n
		return v, true
	}
	for _, tk := range toks {
		if tk.isTok {
			switch tk.b {
			case 'Y':
				v, ok := getDayN(4, 9999)
				if !ok {
					return time.Time{}, fmt.Errorf("expected 4-digit year")
				}
				year = v
			case 'y':
				v, ok := getDayN(2, 99)
				if !ok {
					return time.Time{}, fmt.Errorf("expected 2-digit year")
				}
				year = 2000 + v
			case 'm':
				v, ok := getDayN(2, 12)
				if !ok {
					return time.Time{}, fmt.Errorf("expected 2-digit month")
				}
				month = v
			case 'd':
				v, ok := getDayN(2, 31)
				if !ok {
					return time.Time{}, fmt.Errorf("expected 2-digit day")
				}
				day = v
			case 'H':
				v, ok := getDayN(2, 23)
				if !ok {
					return time.Time{}, fmt.Errorf("expected 2-digit hour")
				}
				hour = v
			case 'M':
				v, ok := getDayN(2, 59)
				if !ok {
					return time.Time{}, fmt.Errorf("expected 2-digit minute")
				}
				min = v
			case 'S':
				v, ok := getDayN(2, 59)
				if !ok {
					return time.Time{}, fmt.Errorf("expected 2-digit second")
				}
				sec = v
			}
			continue
		}
		if sIdx >= len(s) || s[sIdx] != tk.lit {
			return time.Time{}, fmt.Errorf("expected %q at offset %d", tk.lit, sIdx)
		}
		sIdx++
	}
	return time.Date(year, time.Month(month), day, hour, min, sec, 0, time.Local), nil
}

// julianDay computes the Julian day number for the date components of t.
// Fliegel–Van Flandern (Gregorian), avoiding negative mid-formula divisions.
func julianDay(t time.Time) int {
	y := t.Year()
	m := int(t.Month())
	d := t.Day()
	a := (14 - m) / 12
	yy := y + 4800 - a
	mm := m + 12*a - 3
	return d + (153*mm+2)/5 + 365*yy + yy/4 - yy/100 + yy/400 - 32045
}

// isUnreservedByte reports whether c may appear literally in a URL.
func isUnreservedByte(c byte) bool {
	switch {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		return true
	case c == '-', c == '_', c == '.', c == '~':
		return true
	}
	return false
}

// xmlEscape encodes the five XML predefined entities.
func xmlEscape(s string) string {
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '&':
			sb.WriteString("&amp;")
		case '<':
			sb.WriteString("&lt;")
		case '>':
			sb.WriteString("&gt;")
		case '"':
			sb.WriteString("&quot;")
		case '\'':
			sb.WriteString("&apos;")
		default:
			sb.WriteByte(s[i])
		}
	}
	return sb.String()
}

// xmlUnescape reverses xmlEscape.
func xmlUnescape(s string) string {
	if !strings.ContainsAny(s, "&") {
		return s
	}
	repl := strings.NewReplacer(
		"&lt;", "<", "&gt;", ">", "&quot;", "\"", "&apos;", "'", "&amp;", "&",
	)
	return repl.Replace(s)
}

// newGUID returns a random RFC 4122 version-4 UUID.
func newGUID() string {
	var b [16]byte
	if _, err := crand.Read(b[:]); err != nil {
		// crypto/rand must not fail on supported platforms; fall back to
		// math/rand so scripts never hard-crash.
		for i := range b {
			b[i] = byte(rand.Intn(256))
		}
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // RFC 4122 variant
	dst := b[:]
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(dst[0:4]),
		hex.EncodeToString(dst[4:6]),
		hex.EncodeToString(dst[6:8]),
		hex.EncodeToString(dst[8:10]),
		hex.EncodeToString(dst[10:16]))
}

// maskNumber formats x according to a Kalipso-style mask. Supported mask
// characters: 0 (mandatory zero), # (optional digit), , (thousands group),
// . (decimal separator, taken from the first '.', actual separator is the
// platform decimal separator), and literal characters elsewhere.
func maskNumber(x float64, mask string) (string, error) {
	neg := x < 0 || (x == 0 && math.Signbit(x))
	x = math.Abs(x)

	intMask, fracMask, hasDot := strings.Cut(mask, ".")
	if !hasDot {
		fracMask = ""
	}

	// Fractional side defines decimal places.
	decimals := 0
	for i := 0; i < len(fracMask); i++ {
		if fracMask[i] == '0' || fracMask[i] == '#' {
			decimals++
		}
	}

	// Round to the mask's precision.
	factor := math.Pow(10, float64(decimals))
	x = math.Round(x*factor) / factor

	// Split integer/fractional.
	intPart := int64(x)
	frac := x - float64(intPart)

	// Integer width: '0' forces zeros; '#' is optional.
	minDigits := 0
	for i := 0; i < len(intMask); i++ {
		if intMask[i] == '0' {
			minDigits++
		}
	}
	grouped := strings.Contains(intMask, ",")

	intStr := strconv.FormatInt(intPart, 10)
	if grouped {
		intStr = groupDigits(intStr)
	}
	if pad := minDigits - len(intStr); pad > 0 {
		intStr = strings.Repeat("0", pad) + intStr
	}

	var sb strings.Builder
	if neg {
		sb.WriteString("-")
	}
	sb.WriteString(intStr)
	if decimals > 0 {
		fracStr := strconv.FormatInt(int64(math.Round(frac*factor)), 10)
		fracStr = strings.Repeat("0", decimals-len(fracStr)) + fracStr
		sep := "."
		sb.WriteString(sep + fracStr)
	}
	return sb.String(), nil
}

// groupDigits inserts thousand separators into a non-negative integer string.
func groupDigits(s string) string {
	if len(s) <= 3 {
		return s
	}
	var sb strings.Builder
	first := len(s) % 3
	if first > 0 {
		sb.WriteString(s[:first])
	}
	for i := first; i < len(s); i += 3 {
		if sb.Len() > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(s[i : i+3])
	}
	return sb.String()
}