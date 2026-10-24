package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kalua/internal/bindings"
)

// TestGridLayoutSession exercises §6 in a real session: a grid-layout form
// renders cells in declaration order with controls inside their assigned cells.
func TestGridLayoutSession(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "app.lua")
	src := `
function main()
  k.form.new("dash", {
    title = "Dashboard",
    layout = "grid",
    gap = 16,
    cells = {
      {id = "header", width = 12, align = "center"},
      {id = "sidebar", width = 3},
      {id = "main", width = 9}
    }
  })
  k.ctrl.textbox("dash", "search", {label = "Search", cell = "header"})
  k.ctrl.button("dash", "nav", {label = "Nav", cell = "sidebar"})
  k.ctrl.button("dash", "go", {label = "Go", cell = "main"})
  k.form.show("dash")
end
`
	if err := os.WriteFile(script, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := New("t1", script, bindings.Options{}, tLogger{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	out := make(chan outboxWire, 16)
	go func() {
		for msg := range s.Outbox() {
			out <- outboxWire{Type: msg.Type, HTML: msg.HTML}
		}
	}()

	deadline := time.After(5 * time.Second)
	for {
		select {
		case w := <-out:
			if w.Type != "render_form" {
				continue
			}
			for _, want := range []string{
				`layout="grid"`,
				`data-k-cell="header"`,
				`data-k-cell="sidebar"`,
				`data-k-cell="main"`,
				"grid-column: span 12",
				"grid-column: span 3",
				"grid-column: span 9",
				`align="center"`,
			} {
				if !strings.Contains(w.HTML, want) {
					t.Errorf("form HTML missing %q", want)
				}
			}
			if !strings.Contains(w.HTML[strings.Index(w.HTML, `data-k-cell="header"`):strings.Index(w.HTML, `data-k-cell="sidebar"`)], `id="c:dash:search"`) {
				t.Errorf("search not in header cell")
			}
			if !strings.Contains(w.HTML[strings.Index(w.HTML, `data-k-cell="main"`):], `id="c:dash:go"`) {
				t.Errorf("go not in main cell")
			}
			return
		case <-deadline:
			t.Fatalf("timed out waiting for render_form")
		}
	}
}

// TestSetPropertyCellMovesControl verifies k.ctrl.set_property(form, ctrl,
// "cell", newCell) triggers a full form re-render that places the control in
// the destination cell.
func TestSetPropertyCellMovesControl(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "app.lua")
	src := `
function main()
  k.form.new("f", {
    layout = "grid",
    cells = {
      {id = "left", width = 6},
      {id = "right", width = 6}
    }
  })
  k.ctrl.button("f", "b1", {label = "MoveMe", cell = "left"})
  k.form.on("f", "b1", "click", function()
    k.ctrl.set_property("f", "b1", "cell", "right")
  end)
  k.form.show("f")
end
`
	if err := os.WriteFile(script, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := New("t1", script, bindings.Options{Verbose: true}, tLogger{t: t})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	out := make(chan outboxWire, 32)
	go func() {
		for msg := range s.Outbox() {
			out <- outboxWire{Type: msg.Type, HTML: msg.HTML}
		}
	}()

	waitFor := func(pred func(string) bool) {
		t.Helper()
		deadline := time.After(5 * time.Second)
		for {
			select {
			case w := <-out:
				if pred(w.HTML) {
					return
				}
			case <-deadline:
				t.Fatalf("timed out waiting for expected render_form")
			}
		}
	}

	// Initial render: control in "left" cell.
	waitFor(func(html string) bool {
		return strings.Contains(html, `data-k-cell="left"`)
	})

	// Clicking the button calls set_property("cell", "right").
	s.PostEventAny("f", "b1", "click", nil)

	// Expect a fresh render_form where the control is inside the right cell.
	waitFor(func(html string) bool {
		left := strings.Index(html, `data-k-cell="left"`)
		right := strings.Index(html, `data-k-cell="right"`)
		if left == -1 || right == -1 {
			return false
		}
		seg := html[right:]
		insideRight := strings.Contains(seg[0:strings.Index(seg, "</div>")], `id="c:f:b1"`)
		insideLeft := strings.Contains(html[left:right], `id="c:f:b1"`)
		return insideRight && !insideLeft
	})
}

// TestGridWithoutCellsRendersOnce verifies §6 backward compatibility: a
// grid-layout form without an explicit "cells" list auto-creates a single
// "main" cell and renders each control exactly once (no duplicated cell).
func TestGridWithoutCellsRendersOnce(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "app.lua")
	src := `
function main()
  k.form.new("main", {title = "t", layout = "grid", gap = 8})
  k.ctrl.label("main", "heading", { text = "Content area" })
  k.ctrl.textbox("main", "content", { multiline = true, rows = 8, value = "" })
  k.ctrl.label("main", "hint", { text = "hint" })
  k.form.show("main")
end
`
	if err := os.WriteFile(script, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := New("t1", script, bindings.Options{}, tLogger{t: t})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	out := make(chan outboxWire, 16)
	go func() {
		for msg := range s.Outbox() {
			out <- outboxWire{Type: msg.Type, HTML: msg.HTML}
		}
	}()

	deadline := time.After(5 * time.Second)
	for {
		select {
		case w := <-out:
			if w.Type != "render_form" {
				continue
			}
			// Exactly one auto "main" cell.
			mainCount := strings.Count(w.HTML, `data-k-cell="main"`)
			if mainCount != 1 {
				t.Fatalf("expected exactly one data-k-cell='main' cell, got %d", mainCount)
			}
			// Each control renders exactly once.
			for _, want := range []string{
				`id="c:main:heading"`,
				`id="c:main:content"`,
				`id="c:main:hint"`,
			} {
				if got := strings.Count(w.HTML, want); got != 1 {
					t.Errorf("expected %q exactly once, got %d", want, got)
				}
			}
			return
		case <-deadline:
			t.Fatalf("timed out waiting for render_form")
		}
	}
}
