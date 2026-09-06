package builder

import (
	"fmt"
	"strings"

	"github.com/yuin/gopher-lua"
	"kalua/internal/bindings"
	"kalua/internal/vm"
)

// previewState builds a fresh, sandboxed LState populated from the document's
// form definition, then renders the form with the runtime renderer.
func previewState(d *Document) (*lua.LState, error) {
	if msgs := d.Validate(); len(msgs) > 0 {
		return nil, fmt.Errorf("invalid document: %s", strings.Join(msgs, "; "))
	}
	L := vm.New()
	f := d.Form

	formTbl := L.NewTable()
	formTbl.RawSetString("name", lua.LString(f.Name))
	formTbl.RawSetString("title", lua.LString(f.Title))
	layout := f.Layout
	if layout == "" {
		layout = "vertical"
	}
	formTbl.RawSetString("layout", lua.LString(layout))
	align := f.Align
	if align == "" {
		align = "left"
	}
	formTbl.RawSetString("align", lua.LString(align))
	if f.Gap != nil {
		formTbl.RawSetString("gap", lua.LNumber(*f.Gap))
	}
	formTbl.RawSetString("controls", L.NewTable())
	formTbl.RawSetString("order", L.NewTable())
	formTbl.RawSetString("handlers", L.NewTable())
	if len(f.Cells) > 0 {
		cells := L.NewTable()
		for id, c := range f.Cells {
			cell := L.NewTable()
			if c.Width > 0 {
				cell.RawSetString("width", lua.LNumber(c.Width))
			}
			if c.Bg != "" {
				cell.RawSetString("bg", lua.LString(c.Bg))
			}
			if c.Border != nil {
				b := L.NewTable()
				if c.Border.Width > 0 {
					b.RawSetString("width", lua.LNumber(c.Border.Width))
				}
				if c.Border.Color != "" {
					b.RawSetString("color", lua.LString(c.Border.Color))
				}
				cell.RawSetString("border", b)
			}
			if c.Align != "" {
				cell.RawSetString("align", lua.LString(c.Align))
			}
			cells.RawSetString(id, cell)
		}
		formTbl.RawSetString("cells", cells)
	}

	L.SetGlobal(f.Name, formTbl)

	for _, c := range d.Form.Controls {
		opts := L.NewTable()
		for k, v := range c.Opts {
			if k == "items" {
				opts.RawSetString("items", itemsToLuaMap(L, v))
				continue
			}
			opts.RawSetString(k, jsonToLua(L, v))
		}
		bindings.AddControl(L, f.Name, c.Name, c.Type, opts)
	}
	return L, nil
}

// jsonToLua converts a decoded JSON value into a gopher-lua value.
func jsonToLua(L *lua.LState, v any) lua.LValue {
	switch t := v.(type) {
	case nil:
		return lua.LNil
	case string:
		return lua.LString(t)
	case float64:
		return lua.LNumber(t)
	case bool:
		return lua.LBool(t)
	case []any:
		tbl := L.NewTable()
		for i, item := range t {
			tbl.RawSetInt(i+1, jsonToLua(L, item))
		}
		return tbl
	case map[string]any:
		tbl := L.NewTable()
		for k2, item := range t {
			tbl.RawSetString(k2, jsonToLua(L, item))
		}
		return tbl
	default:
		return lua.LNil
	}
}

// itemsToLuaMap converts the ordered JSON items representation
// ([{key,display},...]) into the runtime map {key="display"}.
func itemsToLuaMap(L *lua.LState, v any) *lua.LTable {
	tbl := L.NewTable()
	arr, ok := v.([]any)
	if !ok {
		return tbl
	}
	for _, item := range arr {
		if m, ok := item.(map[string]any); ok {
			key, _ := m["key"].(string)
			display, _ := m["display"].(string)
			if display == "" {
				display = key
			}
			tbl.RawSetString(key, lua.LString(display))
		}
	}
	return tbl
}

// Preview renders the form HTML for the document. When no Lua source file is
// loaded yet (empty document), returns a placeholder.
func Preview(d *Document) (string, error) {
	if d == nil || d.Form == nil {
		return `<div class="kalua-form"><p class="kalua-hint">No form loaded — open a .lua file or start from an empty form.</p></div>`, nil
	}
	L, err := previewState(d)
	if err != nil {
		return "", err
	}
	defer L.Close()
	return bindings.RenderForm(L, d.Form.Name), nil
}
