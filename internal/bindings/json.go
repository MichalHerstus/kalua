// Package bindings implements the JSON bindings (Phase 5 - Data groups).
package bindings

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/yuin/gopher-lua"
)

const maxJSONDepth = 200

// registerJSON installs the k.json_* bindings and the K.NULL sentinel behavior.
func registerJSON(e *Env) {
	// k.json_parse(text) -> value (objects/arrays/strings/numbers/booleans; null -> K.NULL)
	e.register("json_parse", "json", func(L *lua.LState) int {
		text := L.CheckString(1)
		v, err := parseJSON(L, e, []byte(text))
		if err != nil {
			L.RaiseError("json error: %v", err)
			return 0
		}
		L.Push(v)
		return 1
	})

	// k.json_string(value) -> JSON text
	e.register("json_string", "json", func(L *lua.LState) int {
		out, err := stringifyJSON(e, L.Get(1))
		if err != nil {
			L.RaiseError("json error: %v", err)
			return 0
		}
		L.Push(lua.LString(out))
		return 1
	})

	// k.json_load(path) -> parsed value (async; reads and parses off-thread)
	e.register("json_load", "json", func(L *lua.LState) int {
		path := L.CheckString(1)
		return runBlocking(e, L, func() (interface{}, error) {
			resolved, err := e.resolvePath(path)
			if err != nil {
				return nil, err
			}
			data, err := os.ReadFile(resolved)
			if err != nil {
				return nil, fmt.Errorf("file error: cannot read %s: %w", path, err)
			}
			if int64(len(data)) > e.maxFileSize {
				return nil, fmt.Errorf("file error: %s exceeds max file size (%d bytes)", path, e.maxFileSize)
			}
			return unmarshalJSONGo(data)
		}, func(L *lua.LState, v interface{}) lua.LValue {
			return convertJSONGo(L, e, v)
		})
	})

	// k.json_save(path, value) (async write; stringify happens on the caller so
	// the worker goroutine never touches the Lua state)
	e.register("json_save", "json", func(L *lua.LState) int {
		path := L.CheckString(1)
		text, err := stringifyJSON(e, L.Get(2))
		if err != nil {
			L.RaiseError("json error: %v", err)
			return 0
		}
		data := []byte(text)
		return runBlocking(e, L, func() (interface{}, error) {
			resolved, err := e.resolvePath(path)
			if err != nil {
				return nil, err
			}
			if err := writeFileAtomic(resolved, data); err != nil {
				return nil, fmt.Errorf("file error: cannot write %s: %w", path, err)
			}
			return nil, nil
		}, nil)
	})

	// k.json_get(root, path) -> value at dot/bracket path ("" returns root)
	e.register("json_get", "json", func(L *lua.LState) int {
		path := L.OptString(2, "")
		v, err := navigateJSON(L, e, L.Get(1), path)
		if err != nil {
			L.RaiseError("json error: %v", err)
			return 0
		}
		L.Push(v)
		return 1
	})

	// k.json_array_item(root, path, index) -> value at [index] (0-based)
	e.register("json_array_item", "json", func(L *lua.LState) int {
		path := L.OptString(2, "")
		idx := L.CheckInt(3)
		v, err := navigateJSON(L, e, L.Get(1), path)
		if err != nil {
			L.RaiseError("json error: %v", err)
			return 0
		}
		tbl, ok := v.(*lua.LTable)
		if !ok {
			L.RaiseError("json error: %s does not resolve to an array", path)
			return 0
		}
		item := tbl.RawGetInt(idx + 1)
		if item == lua.LNil {
			L.RaiseError("json error: array index %d out of range", idx)
			return 0
		}
		L.Push(item)
		return 1
	})

	// k.json_count(root, path) -> number of elements (array length or object size)
	e.register("json_count", "json", func(L *lua.LState) int {
		path := L.OptString(2, "")
		v, err := navigateJSON(L, e, L.Get(1), path)
		if err != nil {
			L.RaiseError("json error: %v", err)
			return 0
		}
		tbl, ok := v.(*lua.LTable)
		if !ok {
			L.RaiseError("json error: %s does not resolve to a table", path)
			return 0
		}
		L.Push(lua.LNumber(tbl.Len()))
		return 1
	})

	// k.json_names(root, path) -> 1-based table of keys (object) or indices (array)
	e.register("json_names", "json", func(L *lua.LState) int {
		path := L.OptString(2, "")
		v, err := navigateJSON(L, e, L.Get(1), path)
		if err != nil {
			L.RaiseError("json error: %v", err)
			return 0
		}
		tbl, ok := v.(*lua.LTable)
		if !ok {
			L.RaiseError("json error: %s does not resolve to a table", path)
			return 0
		}
		names := tableNames(tbl)
		sort.Strings(names)
		out := L.NewTable()
		for i, n := range names {
			out.RawSetInt(i+1, lua.LString(n))
		}
		L.Push(out)
		return 1
	})

	// k.is_null(v)
	e.register("is_null", "json", func(L *lua.LState) int {
		L.Push(lua.LBool(L.Get(1) == e.kNULL))
		return 1
	})
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
		sort.Strings(keys)
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
	// Any key outside 1..n makes it an object.
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

// unmarshalJSONGo parses JSON text into a plain Go tree (null -> nil).
func unmarshalJSONGo(data []byte) (interface{}, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var raw interface{}
	if err := dec.Decode(&raw); err != nil {
		return nil, err
	}
	return raw, nil
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
	case string:
		return lua.LString(t)
	case bool:
		return lua.LBool(t)
	default:
		return lua.LNil
	}
}

// navigateJSON walks a dot/bracket path ("a.b[0].c") over a parsed Lua tree.
// Array indices are 0-based. An empty path returns the root.
func navigateJSON(L *lua.LState, e *Env, root lua.LValue, path string) (lua.LValue, error) {
	if path == "" {
		return root, nil
	}
	p := root
	i := 0
	fail := func() (lua.LValue, error) {
		return nil, fmt.Errorf("cannot navigate %q from %s", path, luaTypeName(p))
	}
	for i < len(path) {
		if path[i] == '[' {
			end := strings.IndexByte(path[i:], ']')
			if end < 0 {
				return fail()
			}
			numStr := strings.TrimSpace(path[i+1 : i+end])
			idx, err := strconv.Atoi(numStr)
			if err != nil {
				return fail()
			}
			tbl, ok := p.(*lua.LTable)
			if !ok {
				return fail()
			}
			p = tbl.RawGetInt(idx + 1)
			if p == lua.LNil {
				return nil, fmt.Errorf("index %d missing at %q", idx, path)
			}
			i += end + 1
			if i < len(path) && path[i] == '.' {
				i++
			}
			continue
		}
		start := i
		for i < len(path) && path[i] != '.' && path[i] != '[' {
			i++
		}
		seg := path[start:i]
		if seg == "" {
			return fail()
		}
		tbl, ok := p.(*lua.LTable)
		if !ok {
			return fail()
		}
		p = tbl.RawGetString(seg)
		if p == lua.LNil {
			return nil, fmt.Errorf("key %q missing at %q", seg, path)
		}
		if i < len(path) && path[i] == '.' {
			i++
		}
	}
	return p, nil
}

func luaTypeName(lv lua.LValue) string {
	if lv == nil || lv == lua.LNil {
		return "nil"
	}
	return lv.Type().String()
}
