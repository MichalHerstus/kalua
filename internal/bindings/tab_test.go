package bindings

import (
	"strings"
	"testing"

	glua "github.com/yuin/gopher-lua"
)

// TestTabRender verifies renderTab emits headers and panels with the correct
// aria-selected/aria-hidden (only the current tab active) and honors bottom
// tab_position.
func TestTabRender(t *testing.T) {
	L := glua.NewState()
	defer L.Close()
	e := &Env{L: L, known: map[string]string{}}

	L.SetGlobal("k", L.NewTable())
	formTbl := L.NewTable()
	controls := L.NewTable()
	tabs := L.NewTable()
	tab1 := L.NewTable()
	tab1.RawSetString("title", glua.LString("Dashboard"))
	tab1.RawSetString("icon", glua.LString("📊"))
	tab2 := L.NewTable()
	tab2.RawSetString("title", glua.LString("Reports"))
	tab2.RawSetString("icon", glua.LString("📈"))
	tabs.RawSetInt(1, tab1)
	tabs.RawSetInt(2, tab2)

	ctrl := L.NewTable()
	ctrl.RawSetString("name", glua.LString("tabs"))
	ctrl.RawSetString("type", glua.LString("tab"))
	ctrl.RawSetString("form", glua.LString("f"))
	ctrl.RawSetString("tabs", tabs)
	ctrl.RawSetString("tab_position", glua.LString("top"))
	ctrl.RawSetString("swipable", glua.LTrue)
	ctrl.RawSetString("current_tab", glua.LNumber(1))
	controls.RawSetString("tabs", ctrl)
	formTbl.RawSetString("controls", controls)
	L.SetGlobal("f", formTbl)

	// bottom position
	ctrl.RawSetString("tab_position", glua.LString("bottom"))

	var r strings.Builder
	e.known = map[string]string{"ctrl.tab": "controls"}
	_ = r
	got := renderTab(ctrl, "f", "tabs", "c:f:tabs", "")
	if !strings.Contains(got, `data-k-tab-position="bottom"`) {
		t.Errorf("renderTab bottom: missing data-k-tab-position=\"bottom\": %s", got)
	}
	if !strings.Contains(got, `data-k-tab-swipable="true"`) {
		t.Errorf("renderTab: missing data-k-tab-swipable=\"true\": %s", got)
	}

	// tab 1 active: first button aria-selected true, panel aria-hidden false.
	ctrl.RawSetString("tab_position", glua.LString("top"))
	got = renderTab(ctrl, "f", "tabs", "c:f:tabs", "")
	if !strings.Contains(got, `data-k-tab-btn="1"`+"\n\t\t\t\taria-selected=\"true\"") {
		t.Errorf("renderTab: tab 1 not selected: %s", got)
	}
	if !strings.Contains(got, `data-k-tab-panel="1"`+"\n\t\t\t\taria-hidden=\"false\"") {
		t.Errorf("renderTab: tab 1 panel not visible: %s", got)
	}
	if !strings.Contains(got, `data-k-tab-btn="2"`+"\n\t\t\t\taria-selected=\"false\"") {
		t.Errorf("renderTab: tab 2 should not be selected: %s", got)
	}
	if !strings.Contains(got, "Dashboard") || !strings.Contains(got, "Reports") {
		t.Errorf("renderTab: missing titles: %s", got)
	}
}

// TestTabCurrentTabSecond verifies that when current_tab is 2, the second tab
// renders active.
func TestTabCurrentTabSecond(t *testing.T) {
	L := glua.NewState()
	defer L.Close()

	formTbl := L.NewTable()
	controls := L.NewTable()
	tabs := L.NewTable()
	t1 := L.NewTable()
	t1.RawSetString("title", glua.LString("A"))
	t2 := L.NewTable()
	t2.RawSetString("title", glua.LString("B"))
	tabs.RawSetInt(1, t1)
	tabs.RawSetInt(2, t2)
	ctrl := L.NewTable()
	ctrl.RawSetString("name", glua.LString("tabs"))
	ctrl.RawSetString("type", glua.LString("tab"))
	ctrl.RawSetString("form", glua.LString("f"))
	ctrl.RawSetString("tabs", tabs)
	ctrl.RawSetString("current_tab", glua.LNumber(2))
	controls.RawSetString("tabs", ctrl)
	formTbl.RawSetString("controls", controls)
	L.SetGlobal("f", formTbl)

	got := renderTab(ctrl, "f", "tabs", "c:f:tabs", "")
	if !strings.Contains(got, `data-k-tab-btn="2"`+"\n\t\t\t\taria-selected=\"true\"") {
		t.Errorf("renderTab: tab 2 not selected when current_tab=2: %s", got)
	}
	if !strings.Contains(got, `data-k-tab-panel="2"`+"\n\t\t\t\taria-hidden=\"false\"") {
		t.Errorf("renderTab: tab 2 panel not visible: %s", got)
	}
}

// TestTabControlJSON verifies tabControlJSON produces the {tabs, current_tab}
// payload used by tab_update.
func TestTabControlJSON(t *testing.T) {
	L := glua.NewState()
	defer L.Close()

	tabs := L.NewTable()
	t1 := L.NewTable()
	t1.RawSetString("title", glua.LString("A"))
	t1.RawSetString("icon", glua.LString("i1"))
	t2 := L.NewTable()
	t2.RawSetString("title", glua.LString("B"))
	tabs.RawSetInt(1, t1)
	tabs.RawSetInt(2, t2)
	ctrl := L.NewTable()
	ctrl.RawSetString("tabs", tabs)
	ctrl.RawSetString("current_tab", glua.LNumber(2))

	got := tabControlJSON(ctrl)
	if !strings.Contains(got, `"title":"A"`) || !strings.Contains(got, `"title":"B"`) {
		t.Errorf("tabControlJSON missing titles: %s", got)
	}
	if !strings.Contains(got, `"icon":"i1"`) {
		t.Errorf("tabControlJSON missing icon: %s", got)
	}
	if !strings.Contains(got, `"current_tab":2`) {
		t.Errorf("tabControlJSON missing current_tab: %s", got)
	}
}

// TestTabOpsScript exercises the k.tab.* operations end-to-end through the full
// installed binding surface: set_tab/get_tab/get_tab_count/add_tab/insert_tab/
// remove_tab/set_tab_title/set_tab_icon.
func TestTabOpsScript(t *testing.T) {
	L := setupFullState(t)

	src := `
k.form.new("main", { title = "Tabs" })
k.ctrl.tab("main", "tabs", {
  tabs = {
    { title = "Dashboard", icon = "a" },
    { title = "Reports", icon = "b" },
    { title = "Settings", icon = "c" },
  },
})

local function assert(c, m) if not c then error(m) end end

-- Constructor defaults
assert(type(k.tab.get_tab("main", "tabs")) == "number", "get_tab returns number")
assert(k.tab.get_tab("main", "tabs") == 1, "initial tab is 1")
assert(k.tab.get_tab_count("main", "tabs") == 3, "count == 3")

-- set_tab
k.tab.set_tab("main", "tabs", 3)
assert(k.tab.get_tab("main", "tabs") == 3, "set_tab(3)")

-- add
k.tab.add_tab("main", "tabs", { title = "New", icon = "n" })
assert(k.tab.get_tab_count("main", "tabs") == 4, "count == 4 after add")

-- insert at 1 shifts others
k.tab.insert_tab("main", "tabs", 1, { title = "First", icon = "f" })
assert(k.tab.get_tab_count("main", "tabs") == 5, "count == 5 after insert")
k.tab.set_tab("main", "tabs", 1)
assert(k.tab.get_tab("main", "tabs") == 1, "set_tab(1)")

-- remove
k.tab.remove_tab("main", "tabs", 1)
assert(k.tab.get_tab_count("main", "tabs") == 4, "count == 4 after remove")

-- title/icon
k.tab.set_tab_title("main", "tabs", 2, "Renamed")
k.tab.set_tab_icon("main", "tabs", 2, "x")

-- Check the control table reflects current_tab.
local ctrl = CTRL("main", "tabs")
assert(ctrl.current_tab == 1, "control current_tab tracked")
assert(k.tab.get_tab_count("main", "tabs") == 4, "final count")

-- Errors: bad index
local ok, err = pcall(function() k.tab.set_tab("main", "tabs", 99) end)
assert(not ok, "set_tab out of range raises")
`
	if err := L.DoString(src); err != nil {
		t.Fatalf("tab ops script failed: %v", err)
	}
}
