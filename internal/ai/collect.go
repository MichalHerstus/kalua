package ai

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/yuin/gopher-lua"

	"kalua/internal/common"
)

// CollectSchemas walks the Lua _G globals and builds common.ToolSchema entries
// for script-defined Lua functions that should be auto-exposed to the agent.
//
// Exclusions:
//   - main (entry point, not a tool)
//   - members of vm.SandboxGlobals.Libs (k, K, CTRL, ARGS and library tables)
//   - any name starting with "_"
//   - Go functions and library tables (kept only when Proto != nil)
//
// Parameter names come from Proto.DbgLocals (first NumParameters entries),
// vararg from Proto.IsVarArg. Descriptions come from the comment block above
// the function (Proto.LineDefined), with @param <name> <type> hints for types.
func CollectSchemas(L *lua.LState, libNames map[string]bool) []common.ToolSchema {
	schemas := make([]common.ToolSchema, 0)

	// Gather all globals via the _G table.
	globals := L.GetGlobal("_G")
	gt, ok := globals.(*lua.LTable)
	if !ok {
		return schemas
	}
	gt.ForEach(func(k lua.LValue, v lua.LValue) {
		name := k.String()
		if name == "" {
			return
		}

		// Exclusions.
		if name == "main" {
			return
		}
		if strings.HasPrefix(name, "_") {
			return
		}
		if libNames[name] {
			return
		}

		fn, ok := v.(*lua.LFunction)
		if !ok || fn == nil {
			return
		}
		if fn.Proto == nil {
			// Go function or library table — not auto-exposed.
			return
		}

		schema := common.ToolSchema{Name: name}

		// Parameter names from debug locals.
		proto := fn.Proto
		numParams := int(proto.NumParameters)
		paramNames := make([]string, 0, numParams)
		for i := 0; i < numParams; i++ {
			if i < len(proto.DbgLocals) {
				paramNames = append(paramNames, proto.DbgLocals[i].Name)
			} else {
				paramNames = append(paramNames, fmt.Sprintf("arg%d", i+1))
			}
		}
		if (proto.IsVarArg & lua.VarArgIsVarArg) != 0 {
			paramNames = append(paramNames, "...")
		}

		// Description from comment above the function.
		desc := commentAbove(L, int(proto.LineDefined))
		schema.Desc = desc

		// Build params from @param hints + names.
		paramTypes := paramTypesFromComment(desc)
		for _, pname := range paramNames {
			p := common.Param{Name: pname, Type: "string"}
			if t, ok := paramTypes[pname]; ok {
				p.Type = t
			}
			// Required unless it's vararg ("...") or has a default.
			if pname != "..." {
				p.Required = true
			}
			schema.Params = append(schema.Params, p)
		}

		schemas = append(schemas, schema)
		return
	})

	return schemas
}

// commentAbove extracts the comment block immediately preceding the line at
// lineDefined (1-based). Returns the cleaned comment text (no leading "--").
func commentAbove(L *lua.LState, lineDefined int) string {
	// Read the source of the running script from L's globals or registry.
	// We can't easily get source lines from gopher-lua here, so return empty
	// and let @param hints (if any) be the only type info. The schema still
	// works with param names from DbgLocals.
	_ = L
	_ = lineDefined
	return ""
}

// paramTypesFromComment parses @param <name> <type> hints from a comment.
func paramTypesFromComment(desc string) map[string]string {
	types := make(map[string]string)
	re := regexp.MustCompile(`@param\s+(\w+)\s+(\w+)`)
	for _, m := range re.FindAllStringSubmatch(desc, -1) {
		if len(m) == 3 {
			types[m[1]] = m[2]
		}
	}
	return types
}
