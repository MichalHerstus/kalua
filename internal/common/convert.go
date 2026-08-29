// Package common provides shared types and utilities used across KALUA packages to avoid import cycles.
package common

import (
	"database/sql"
	"fmt"
	"reflect"

	"github.com/yuin/gopher-lua"
)

// GoValueToLua converts a Go value to a Lua value
func GoValueToLua(L *lua.LState, v interface{}) lua.LValue {
	switch val := v.(type) {
	case nil:
		return lua.LNil
	case bool:
		return lua.LBool(val)
	case int:
		return lua.LNumber(val)
	case int64:
		return lua.LNumber(val)
	case float64:
		return lua.LNumber(val)
	case string:
		return lua.LString(val)
	case []interface{}:
		tbl := L.NewTable()
		for i, item := range val {
			tbl.RawSetInt(i+1, GoValueToLua(L, item))
		}
		return tbl
	case map[string]interface{}:
		tbl := L.NewTable()
		for k, item := range val {
			tbl.RawSetString(k, GoValueToLua(L, item))
		}
		return tbl
	case sql.NullString:
		if val.Valid {
			return lua.LString(val.String)
		}
		return lua.LNil
	case sql.NullInt64:
		if val.Valid {
			return lua.LNumber(val.Int64)
		}
		return lua.LNil
	case sql.NullFloat64:
		if val.Valid {
			return lua.LNumber(val.Float64)
		}
		return lua.LNil
	case sql.NullBool:
		if val.Valid {
			return lua.LBool(val.Bool)
		}
		return lua.LNil
	default:
		// Handle any slice/array type via reflection
		rv := reflect.ValueOf(v)
		if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
			tbl := L.NewTable()
			for i := 0; i < rv.Len(); i++ {
				tbl.RawSetInt(i+1, GoValueToLua(L, rv.Index(i).Interface()))
			}
			return tbl
		}
		return lua.LString(fmt.Sprintf("%v", v))
	}
}
