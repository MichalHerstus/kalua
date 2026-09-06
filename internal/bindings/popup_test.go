package bindings

import (
	"strings"
	"testing"

	"github.com/yuin/gopher-lua"
)

// popupLState returns a throwaway LState for popup parse tests. layout builds
// the k.popup argument table (options form with title/items, or plain list).
func popupLState(t *testing.T, layout func(L *lua.LState) *lua.LTable) (*lua.LState, *lua.LTable) {
	t.Helper()
	L := lua.NewState(lua.Options{SkipOpenLibs: true})
	t.Cleanup(L.Close)
	return L, layout(L)
}

// TestPopupParseListForm verifies k.popup's list form: the argument itself is
// the menu. Leaves may be positional {label, value}, bare strings, or
// {label=, value=}; branches are {label=, items={...}}.
func TestPopupParseListForm(t *testing.T) {
	L, tbl := popupLState(t, func(L *lua.LState) *lua.LTable {
		item := func(vals ...lua.LValue) *lua.LTable {
			it := L.NewTable()
			it.RawSetInt(1, vals[0])
			if len(vals) > 1 {
				it.RawSetInt(2, vals[1])
			}
			return it
		}
		opts := L.NewTable()
		opts.RawSetInt(1, item(str("Open"), num(1))) // positional → label "Open", value 1
		opts.RawSetInt(2, str("Quit"))               // bare → label "Quit", value "Quit"
		branch := L.NewTable()
		branch.RawSetString("label", str("File"))
		sub := L.NewTable()
		deep := L.NewTable()
		deep.RawSetString("label", str("Recent"))
		deepSub := L.NewTable()
		d1 := L.NewTable()
		d1.RawSetString("label", str("a.lua"))
		d1.RawSetString("value", str("r1"))
		deepSub.RawSetInt(1, d1)
		deep.RawSetString("items", deepSub)
		sub.RawSetInt(1, deep)
		branch.RawSetString("items", sub)
		opts.RawSetInt(3, branch)
		return opts
	})

	opts, err := popupFromTable(L, &Env{}, tbl)
	if err != nil {
		t.Fatalf("popupFromTable: %v", err)
	}
	if opts.Title != "" {
		t.Fatalf("expected no title; got %q", opts.Title)
	}
	if len(opts.Items) != 3 {
		t.Fatalf("expected 3 top-level items; got %d", len(opts.Items))
	}

	if opts.Items[0].Label != "Open" || opts.Items[0].Value != "1" || len(opts.Items[0].Items) != 0 {
		t.Fatalf("bad positional leaf: %+v", opts.Items[0])
	}
	if opts.Items[1].Label != "Quit" || opts.Items[1].Value != "\"Quit\"" {
		t.Fatalf("bad bare leaf: %+v", opts.Items[1])
	}
	branch := opts.Items[2]
	if branch.Label != "File" || branch.Value != "" || len(branch.Items) != 1 {
		t.Fatalf("bad branch: %+v", branch)
	}
	if len(branch.Items[0].Items) != 1 || branch.Items[0].Items[0].Value != "\"r1\"" {
		t.Fatalf("bad nested leaf: %+v", branch.Items)
	}
}

// TestPopupParseOptionsForm verifies the options form {title=, items=...}.
func TestPopupParseOptionsForm(t *testing.T) {
	L, tbl := popupLState(t, func(L *lua.LState) *lua.LTable {
		opts := L.NewTable()
		opts.RawSetString("title", str("Maintenance"))
		items := L.NewTable()
		leaf := L.NewTable()
		leaf.RawSetString("label", str("Refresh"))
		leaf.RawSetString("value", str("refresh"))
		items.RawSetInt(1, leaf)
		opts.RawSetString("items", items)
		return opts
	})

	opts, err := popupFromTable(L, &Env{}, tbl)
	if err != nil {
		t.Fatalf("popupFromTable: %v", err)
	}
	if opts.Title != "Maintenance" {
		t.Fatalf("title = %q, want Maintenance", opts.Title)
	}
	if len(opts.Items) != 1 || opts.Items[0].Value != "\"refresh\"" {
		t.Fatalf("bad items: %+v", opts.Items)
	}
}

// TestPopupParseNonTable verifies a non-table argument is an error.
func TestPopupParseNonTable(t *testing.T) {
	L := lua.NewState(lua.Options{SkipOpenLibs: true})
	defer L.Close()
	_, err := popupFromTable(L, &Env{}, str("nope"))
	if err == nil {
		t.Fatal("expected error for non-table argument")
	}
	if !strings.Contains(err.Error(), "menu table") {
		t.Fatalf("unexpected error: %v", err)
	}
}
