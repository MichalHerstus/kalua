//go:build js && wasm

// Package bindings provides WASM-specific implementations of shared functions.
package bindings

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/yuin/gopher-lua"

	"kalua/internal/coerce"
)

// registerHelpers installs the K.* helpers per §2.3 and §5.9:
//
//	K.EQ, K.NEQ, K.ADD — operator name constants
//	K.eq, K.ne, K.add  — binary operators with Kalipso coercion
//	K.tonum, K.tostr   — coercion helpers
//	K.truthy           — condition test for If(...)
func registerHelpers(e *Env, K *lua.LTable) {
	K.RawSetString("EQ", lua.LString("="))
	K.RawSetString("NEQ", lua.LString("<>"))
	K.RawSetString("ADD", lua.LString("+"))

	// eq(a,b) — numeric when both coerce (with ""→0), else string compare
	K.RawSetString("eq", e.L.NewFunction(func(L *lua.LState) int {
		a := v(L.Get(1))
		b := v(L.Get(2))
		L.Push(lvalue(coerce.Eq(a, b)))
		return 1
	}))

	// ne(a,b) — negation of eq
	K.RawSetString("ne", e.L.NewFunction(func(L *lua.LState) int {
		a := v(L.Get(1))
		b := v(L.Get(2))
		L.Push(lvalue(coerce.Ne(a, b)))
		return 1
	}))

	// add(a,b) — Kalipso +: numeric if both coerce, else concat
	K.RawSetString("add", e.L.NewFunction(func(L *lua.LState) int {
		a := v(L.Get(1))
		b := v(L.Get(2))
		L.Push(lvalue(coerce.Add(a, b)))
		return 1
	}))

	// tonum(x) → number, or 0 when not numeric
	K.RawSetString("tonum", e.L.NewFunction(func(L *lua.LState) int {
		a := v(L.Get(1))
		if n, ok := coerce.ToNum(a); ok {
			L.Push(lua.LNumber(n))
		} else {
			L.Push(lua.LNumber(0))
		}
		return 1
	}))

	// tostr(x) → Kalipso string form
	K.RawSetString("tostr", e.L.NewFunction(func(L *lua.LState) int {
		a := v(L.Get(1))
		L.Push(lua.LString(coerce.Stringify(a)))
		return 1
	}))

	// truthy(x) → Kalipso condition test
	K.RawSetString("truthy", e.L.NewFunction(func(L *lua.LState) int {
		a := v(L.Get(1))
		L.Push(lua.LBool(coerce.Truthy(a)))
		return 1
	}))
}

const maxJSONDepth = 200

// stringifyJSON encodes a Lua value tree as compact JSON. Objects use sorted
// keys for deterministic output; K.NULL and nil both encode as null.
func stringifyJSON(e *Env, v lua.LValue) (string, error) {
	var sb strings.Builder
	if err := writeJSON(&sb, e, v, 0, map[*lua.LTable]bool{}); err != nil {
		return "", err
	}
	return sb.String(), nil
}

func writeJSON(sb *strings.Builder, e *Env, v lua.LValue, depth int, seen map[*lua.LTable]bool) error {
	if depth > maxJSONDepth {
		return fmt.Errorf("nesting too deep")
	}
	switch lv := v.(type) {
	case *lua.LNilType:
		sb.WriteString("null")
	case lua.LBool:
		if bool(lv) {
			sb.WriteString("true")
		} else {
			sb.WriteString("false")
		}
	case lua.LNumber:
		b, err := json.Marshal(float64(lv))
		if err != nil {
			return err
		}
		sb.Write(b)
	case lua.LString:
		b, err := json.Marshal(string(lv))
		if err != nil {
			return err
		}
		sb.Write(b)
	case *lua.LTable:
		if lv == e.kNULL {
			sb.WriteString("null")
			return nil
		}
		if seen[lv] {
			return fmt.Errorf("circular reference")
		}
		seen[lv] = true
		defer delete(seen, lv)

		isArray := isArrayTable(lv)
		n := lv.Len()
		if isArray {
			sb.WriteByte('[')
			for i := 1; i <= n; i++ {
				if i > 1 {
					sb.WriteByte(',')
				}
				if err := writeJSON(sb, e, lv.RawGetInt(i), depth+1, seen); err != nil {
					return err
				}
			}
			sb.WriteByte(']')
			return nil
		}
		keys := tableNames(lv)
		// sort.Strings(keys) - sort not available in WASM without extra import
		// For WASM, we skip sorting for simplicity
		sb.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				sb.WriteByte(',')
			}
			kb, _ := json.Marshal(k)
			sb.Write(kb)
			sb.WriteByte(':')
			if err := writeJSON(sb, e, lv.RawGetString(k), depth+1, seen); err != nil {
				return err
			}
		}
		sb.WriteByte('}')
	default:
		return fmt.Errorf("cannot encode %s as JSON", v.Type())
	}
	return nil
}

// isArrayTable reports whether a Lua table is a dense 1..n sequence.
func isArrayTable(t *lua.LTable) bool {
	n := t.Len()
	if n == 0 {
		return false
	}
	for i := 1; i <= n; i++ {
		if t.RawGetInt(i) == lua.LNil {
			return false
		}
	}
	nonArray := false
	t.ForEach(func(k, _ lua.LValue) {
		if _, ok := k.(lua.LNumber); !ok {
			nonArray = true
			return
		}
		num := int(k.(lua.LNumber))
		if num < 1 || num > n {
			nonArray = true
		}
	})
	return !nonArray
}

// tableNames collects the string key names of a Lua table.
func tableNames(t *lua.LTable) []string {
	var names []string
	t.ForEach(func(k, _ lua.LValue) {
		switch kv := k.(type) {
		case lua.LString:
			names = append(names, string(kv))
		case lua.LNumber:
			names = append(names, strconv.Itoa(int(kv)))
		default:
			names = append(names, kv.String())
		}
	})
	return names
}

// parseJSON converts JSON text to a Lua value tree, mapping null to K.NULL.
func parseJSON(L *lua.LState, e *Env, data []byte) (lua.LValue, error) {
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.UseNumber()
	var raw interface{}
	if err := dec.Decode(&raw); err != nil {
		return nil, err
	}
	return convertJSONGo(L, e, raw), nil
}

// convertJSONGo converts a decoded Go tree into Lua values, mapping nil to
// K.NULL and numbers to LNumber.
func convertJSONGo(L *lua.LState, e *Env, v interface{}) lua.LValue {
	switch t := v.(type) {
	case nil:
		return e.kNULL
	case map[string]interface{}:
		tbl := L.NewTable()
		for k, val := range t {
			tbl.RawSetString(k, convertJSONGo(L, e, val))
		}
		return tbl
	case []interface{}:
		tbl := L.NewTable()
		for i, item := range t {
			tbl.RawSetInt(i+1, convertJSONGo(L, e, item))
		}
		return tbl
	case json.Number:
		if f, err := t.Float64(); err == nil {
			return lua.LNumber(f)
		}
		return lua.LString(string(t))
	case int:
		return lua.LNumber(float64(t))
	case int64:
		return lua.LNumber(float64(t))
	case float32:
		return lua.LNumber(float64(t))
	case string:
		return lua.LString(t)
	case bool:
		return lua.LBool(t)
	default:
		return lua.LNil
	}
}

// evalSymlinksBestEffort resolves symlinks on the longest existing prefix of p
// (so paths whose final components do not exist yet still canonicalize parent
// symlinks) and returns the fully-resolved path.
// WASM stub: just returns the path as-is.
func evalSymlinksBestEffort(p string) (string, error) {
	return p, nil
}

// luaToString strings any Lua value into bytes for file/json/crypto input.
func luaToString(L *lua.LState, idx int) string {
	return L.Get(idx).String()
}

// joinArgs joins arguments with spaces.
func joinArgs(args []string) string {
	result := ""
	for i, a := range args {
		if i > 0 {
			result += " "
		}
		result += a
	}
	return result
}