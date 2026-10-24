//go:build !wasm

// Tab control support (see kforms_enhancements.md §10).
//
// k.ctrl.tab renders a <div.kalua-tab> with headers and panels whose
// data-k-tab-* attributes drive the client (app.js). The k.tab.* operations
// in this file manipulate the tabs array and current_tab stored on the Lua
// control table, pushing tab_set_tab messages through the session outbox so
// the browser can activate tabs programmatically.
package bindings

import (
	"strconv"

	"github.com/yuin/gopher-lua"

	"kalua/internal/common"
)

// registerTabOps installs the k.tab.* Tab control operations. Called from
// registerControls so the operations share the same API namespace.
func registerTabOps(e *Env) {
	// k.tab.set_tab(form, name, index) - programmatically switch to tab (1-based)
	e.register("tab.set_tab", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)
		index := L.CheckInt(3)

		ctrl := tabControl(L, formName, name)
		if ctrl == nil {
			return 0
		}
		if index < 1 {
			L.RaiseError("tab.set_tab: index must be >= 1")
			return 0
		}

		tabs := ctrl.RawGetString("tabs")
		if tabs == lua.LNil {
			return 0
		}
		if tabsTbl, ok := tabs.(*lua.LTable); ok {
			if index > tabsTbl.Len() {
				L.RaiseError("tab.set_tab: index %d out of range (tabs=%d)", index, tabsTbl.Len())
				return 0
			}
		}

		oldTab := 1
		if v := ctrl.RawGetString("current_tab"); v != lua.LNil {
			oldTab = int(lua.LVAsNumber(v))
		}
		ctrl.RawSetString("current_tab", lua.LNumber(index))

		sendOutbox(e, common.OutboxMsg{
			Type:     "tab_set_tab",
			Form:     formName,
			Ctrl:     name,
			Selector: "#c:" + formName + ":" + name,
			Data:     `{"index":` + strconv.Itoa(index) + `,"old_index":` + strconv.Itoa(oldTab) + `}`,
		})
		return 0
	})

	// k.tab.get_tab(form, name) -> number (1-based current tab index)
	e.register("tab.get_tab", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)

		ctrl := tabControl(L, formName, name)
		if ctrl == nil {
			L.Push(lua.LNil)
			return 1
		}
		if v := ctrl.RawGetString("current_tab"); v != lua.LNil {
			L.Push(lua.LNumber(lua.LVAsNumber(v)))
			return 1
		}
		L.Push(lua.LNumber(1))
		return 1
	})

	// k.tab.get_tab_count(form, name) -> number
	e.register("tab.get_tab_count", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)

		ctrl := tabControl(L, formName, name)
		if ctrl == nil {
			L.Push(lua.LNil)
			return 1
		}
		tabs := ctrl.RawGetString("tabs")
		if tabs == lua.LNil {
			L.Push(lua.LNumber(0))
			return 1
		}
		if tabsTbl, ok := tabs.(*lua.LTable); ok {
			L.Push(lua.LNumber(tabsTbl.Len()))
			return 1
		}
		L.Push(lua.LNumber(0))
		return 1
	})

	// k.tab.add_tab(form, name, {title, icon?}) - add tab at end
	e.register("tab.add_tab", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)
		tab := L.CheckTable(3)

		ctrl := tabControl(L, formName, name)
		if ctrl == nil {
			return 0
		}

		tabs := ctrl.RawGetString("tabs")
		if tabs == lua.LNil {
			tabs = L.NewTable()
			ctrl.RawSetString("tabs", tabs)
		}
		if tabsTbl, ok := tabs.(*lua.LTable); ok {
			idx := tabsTbl.Len() + 1
			tabsTbl.RawSetInt(idx, tab)
		}
		tabUpdate(e, formName, name, ctrl)
		return 0
	})

	// k.tab.insert_tab(form, name, index, {title, icon?}) - insert tab at position
	e.register("tab.insert_tab", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)
		index := L.CheckInt(3)
		tab := L.CheckTable(4)

		ctrl := tabControl(L, formName, name)
		if ctrl == nil {
			return 0
		}
		if index < 1 {
			L.RaiseError("tab.insert_tab: index must be >= 1")
			return 0
		}

		tabs := ctrl.RawGetString("tabs")
		if tabs == lua.LNil {
			tabs = L.NewTable()
			ctrl.RawSetString("tabs", tabs)
		}
		if tabsTbl, ok := tabs.(*lua.LTable); ok {
			if index > tabsTbl.Len()+1 {
				L.RaiseError("tab.insert_tab: index %d out of range (tabs=%d)", index, tabsTbl.Len()+1)
				return 0
			}
			// Shift existing tabs down to make room.
			for i := tabsTbl.Len(); i >= index; i-- {
				tabsTbl.RawSetInt(i+1, tabsTbl.RawGetInt(i))
			}
			tabsTbl.RawSetInt(index, tab)
		}
		tabUpdate(e, formName, name, ctrl)
		return 0
	})

	// k.tab.remove_tab(form, name, index) - remove tab by index
	e.register("tab.remove_tab", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)
		index := L.CheckInt(3)

		ctrl := tabControl(L, formName, name)
		if ctrl == nil {
			return 0
		}
		if index < 1 {
			L.RaiseError("tab.remove_tab: index must be >= 1")
			return 0
		}

		tabs := ctrl.RawGetString("tabs")
		if tabs == lua.LNil {
			return 0
		}
		if tabsTbl, ok := tabs.(*lua.LTable); ok {
			if index > tabsTbl.Len() {
				return 0
			}
			tabsTbl.RawSetInt(index, lua.LNil)
			// Compact: shift remaining tabs up.
			for i := index + 1; i <= tabsTbl.Len(); i++ {
				v := tabsTbl.RawGetInt(i)
				if v != lua.LNil {
					tabsTbl.RawSetInt(i-1, v)
					tabsTbl.RawSetInt(i, lua.LNil)
				}
			}
		}
		tabUpdate(e, formName, name, ctrl)
		return 0
	})

	// k.tab.set_tab_title(form, name, index, title) - update tab title
	e.register("tab.set_tab_title", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)
		index := L.CheckInt(3)
		title := L.CheckString(4)

		ctrl := tabControl(L, formName, name)
		if ctrl == nil {
			return 0
		}
		if index < 1 {
			L.RaiseError("tab.set_tab_title: index must be >= 1")
			return 0
		}

		tabs := ctrl.RawGetString("tabs")
		if tabs == lua.LNil {
			return 0
		}
		if tabsTbl, ok := tabs.(*lua.LTable); ok {
			if index > tabsTbl.Len() {
				L.RaiseError("tab.set_tab_title: index %d out of range (tabs=%d)", index, tabsTbl.Len())
				return 0
			}
			tab := tabsTbl.RawGetInt(index)
			if tabTbl, ok := tab.(*lua.LTable); ok {
				tabTbl.RawSetString("title", lua.LString(title))
			}
		}
		tabUpdate(e, formName, name, ctrl)
		return 0
	})

	// k.tab.set_tab_icon(form, name, index, icon) - update tab icon
	e.register("tab.set_tab_icon", "controls", func(L *lua.LState) int {
		formName := L.CheckString(1)
		name := L.CheckString(2)
		index := L.CheckInt(3)
		icon := L.CheckString(4)

		ctrl := tabControl(L, formName, name)
		if ctrl == nil {
			return 0
		}
		if index < 1 {
			L.RaiseError("tab.set_tab_icon: index must be >= 1")
			return 0
		}

		tabs := ctrl.RawGetString("tabs")
		if tabs == lua.LNil {
			return 0
		}
		if tabsTbl, ok := tabs.(*lua.LTable); ok {
			if index > tabsTbl.Len() {
				L.RaiseError("tab.set_tab_icon: index %d out of range (tabs=%d)", index, tabsTbl.Len())
				return 0
			}
			tab := tabsTbl.RawGetInt(index)
			if tabTbl, ok := tab.(*lua.LTable); ok {
				tabTbl.RawSetString("icon", lua.LString(icon))
			}
		}
		tabUpdate(e, formName, name, ctrl)
		return 0
	})
}

// tabControl returns the tab control table for (form, name), or nil when the
// control does not exist or is not a tab.
func tabControl(L *lua.LState, formName, name string) *lua.LTable {
	ctrl := getControl(L, formName, name)
	if ctrl == nil || ctrl.RawGetString("type").String() != "tab" {
		return nil
	}
	return ctrl
}

// tabUpdate pushes a tab_update message carrying the full tabs array and
// current_tab so the browser can re-render headers/panels.
func tabUpdate(e *Env, formName, name string, ctrl *lua.LTable) {
	sendOutbox(e, common.OutboxMsg{
		Type:     "tab_update",
		Form:     formName,
		Ctrl:     name,
		Selector: "#c:" + formName + ":" + name,
		Data:     tabControlJSON(ctrl),
	})
}

// tabControlJSON serializes a tab control's tabs + current_tab to JSON for the
// tab_update message.
func tabControlJSON(ctrl *lua.LTable) string {
	tabs := ctrl.RawGetString("tabs")
	tabsJSON := "[]"
	if tabsTbl, ok := tabs.(*lua.LTable); ok && tabsTbl.Len() > 0 {
		tabsJSON = luaTableToJSON(tabsTbl)
	}
	currentTab := 1
	if v := ctrl.RawGetString("current_tab"); v != lua.LNil {
		currentTab = int(lua.LVAsNumber(v))
	}
	return `{"tabs":` + tabsJSON + `,"current_tab":` + strconv.Itoa(currentTab) + `}`
}
