// Package format implements a gofmt-style formatter for Kalua (Lua) scripts.
//
// It re-indents blocks, normalizes spacing and line breaks without reflowing
// statements (line breaks in the source are preserved), moves every comment to
// a canonical position, and is idempotent: formatting already-formatted source
// is a no-op. Strings, numbers, long-bracket strings and comment interiors are
// re-emitted verbatim from the source tokens.
package format

import (
	"fmt"
	"strings"

	"github.com/yuin/gopher-lua/parse"
)

// ID is a stable marker exposed for tooling; the exact value is insignificant.
const ID = "kalua-format"

// Format canonicalizes the Lua source src and returns the formatted bytes.
//
// The algorithm is token-based (see Tokenize in the vendored gopher-lua
// lexer): tokens carry exact source byte spans and the printer re-emits them
// with normalized whitespace. Syntax errors surface as an error and the
// original source is never modified.
func Format(src []byte, name string) ([]byte, error) {
	if len(strings.TrimSpace(string(src))) == 0 {
		return src, nil
	}
	toks, err := parse.Tokenize(src, name)
	if err != nil {
		return nil, err
	}
	var p printer
	out := p.print(toks)
	if _, err := parse.Parse(strings.NewReader(out), name); err != nil {
		return nil, fmt.Errorf("formatter produced invalid Lua: %v", err)
	}
	return []byte(out), nil
}

// Diff returns a unified-style diff (zero context lines) between before and
// after, prefixed with --- path / +++ path hunks, or "" when equal. It is used
// by `KALUA check -d` to preview formatting changes.
func Diff(path, before, after string) string {
	if before == after {
		return ""
	}
	a := strings.Split(before, "\n")
	b := strings.Split(after, "\n")
	// trailing empty element from the final newline
	if n := len(a); n > 0 && a[n-1] == "" {
		a = a[:n-1]
	}
	if n := len(b); n > 0 && b[n-1] == "" {
		b = b[:n-1]
	}

	// LCS length table (backward so the walk can tie-break consistently).
	n, m := len(a), len(b)
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		r := dp[i]
		rn := dp[i+1]
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				r[j] = rn[j+1] + 1
			} else if rn[j] >= r[j+1] {
				r[j] = rn[j]
			} else {
				r[j] = r[j+1]
			}
		}
	}

	type dline struct {
		kind byte // ' ', '+', '-'
		text string
		a, b int // 1-based source line numbers for '-'/'+' (0 when the side has none)
	}
	var dl []dline
	i, j, aNo, bNo := 0, 0, 0, 0
	for i < n || j < m {
		if i < n && j < m && a[i] == b[j] {
			aNo++
			bNo++
			dl = append(dl, dline{' ', a[i], aNo, bNo})
			i++
			j++
		} else if j < m && (i >= n || dp[i][j+1] >= dp[i+1][j]) {
			bNo++
			dl = append(dl, dline{'+', b[j], 0, bNo})
			j++
		} else {
			aNo++
			dl = append(dl, dline{'-', a[i], aNo, 0})
			i++
		}
	}

	var out strings.Builder
	fmt.Fprintf(&out, "--- %s\n+++ %s\n", path, path)
	k := 0
	for k < len(dl) {
		if dl[k].kind == ' ' {
			k++
			continue
		}
		runStart := k
		aS, aE, bS, bE := 0, 0, 0, 0
		for k < len(dl) && dl[k].kind != ' ' {
			d := dl[k]
			if d.a > 0 {
				if aS == 0 {
					aS, aE = d.a, d.a
				} else {
					if d.a < aS {
						aS = d.a
					}
					if d.a > aE {
						aE = d.a
					}
				}
			}
			if d.b > 0 {
				if bS == 0 {
					bS, bE = d.b, d.b
				} else {
					if d.b < bS {
						bS = d.b
					}
					if d.b > bE {
						bE = d.b
					}
				}
			}
			k++
		}
		aCount, bCount := 0, 0
		if aS > 0 {
			aCount = aE - aS + 1
		}
		if bS > 0 {
			bCount = bE - bS + 1
		}
		fmt.Fprintf(&out, "@@ -%d,%d +%d,%d @@\n", aS, aCount, bS, bCount)
		for _, d := range dl[runStart:k] {
			out.WriteByte(d.kind)
			out.WriteString(d.text)
			out.WriteByte('\n')
		}
	}
	return out.String()
}
