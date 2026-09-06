// Package builder implements the KALUA visual Form Builder (§5
// kforms_enhancements.md). It exposes an HTTP editor that round-trips a form
// between a JSON document, an in-memory Lua model (preview), and generated
// Kalipso-style Lua source.
//
// The JSON document stores controls at *source level*: the `opts` map holds
// exactly the keys that appear in a k.ctrl.<type> constructor call. The
// preview reuses the runtime renderer (internal/bindings) by reconstructing a
// fresh LState with the sandbox factory and calling the exported
// AddControl/RenderForm wrappers, so what the builder shows matches what the
// app renders.
package builder

import (
	"encoding/json"
	"fmt"
	"regexp"
)

// DocVersion is the schema version emitted by the builder.
const DocVersion = 1

// Not a file extension but the canonical document file suffix used to detect
// builder documents vs raw Lua sources.
const FormFileExt = ".kalua-form.json"

// Types are the 11 runtime control types understood by the builder.
var Types = []string{
	"label", "textbox", "button", "combo", "list", "table",
	"checkbox", "radio", "looper", "chart", "image",
}

var identRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Document is a single-form builder document.
type Document struct {
	Version int   `json:"version"`
	Form    *Form `json:"form"`
}

// Form is the source-level form definition.
type Form struct {
	Name     string              `json:"name"`
	Title    string              `json:"title,omitempty"`
	Layout   string              `json:"layout,omitempty"`
	Align    string              `json:"align,omitempty"`
	Gap      *int                `json:"gap,omitempty"`
	Cells    map[string]*Cell    `json:"cells,omitempty"`
	Controls []*Control          `json:"controls"`
	Handlers map[string][]string `json:"handlers,omitempty"`
	Notes    []string            `json:"notes,omitempty"` // import/export notices
}

// Cell describes one grid cell in a grid layout.
type Cell struct {
	Width  int     `json:"width,omitempty"`
	Bg     string  `json:"bg,omitempty"`
	Border *Border `json:"border,omitempty"`
	Align  string  `json:"align,omitempty"`
}

// Border is a grid cell border.
type Border struct {
	Width int    `json:"width,omitempty"`
	Color string `json:"color,omitempty"`
}

// Control is a source-level control declaration.
type Control struct {
	Name string         `json:"name"`
	Type string         `json:"type"`
	Opts map[string]any `json:"opts,omitempty"`
}

// Validate checks the document for structural errors and returns human
// readable messages (empty when the document is valid). Structural validity
// is checked by the caller; here we enforce builder invariants.
func (d *Document) Validate() []string {
	var msgs []string
	if d == nil || d.Form == nil {
		return []string{"missing 'form'"}
	}
	if d.Version != DocVersion {
		msgs = append(msgs, fmt.Sprintf("unsupported version %d (expected %d)", d.Version, DocVersion))
	}
	f := d.Form
	if f.Name == "" {
		msgs = append(msgs, "form.name is required")
	}
	if !identRe.MatchString(f.Name) {
		msgs = append(msgs, fmt.Sprintf("form.name %q is not a valid Lua identifier", f.Name))
	}
	if f.Layout != "" && f.Layout != "vertical" && f.Layout != "grid" {
		msgs = append(msgs, fmt.Sprintf("form.layout %q must be 'vertical' or 'grid'", f.Layout))
	}
	if f.Align != "" && f.Align != "left" && f.Align != "center" && f.Align != "right" {
		msgs = append(msgs, fmt.Sprintf("form.align %q must be left|center|right", f.Align))
	}
	seen := map[string]bool{}
	for _, c := range f.Controls {
		if c == nil {
			msgs = append(msgs, "null control in form.controls")
			continue
		}
		if c.Name == "" {
			msgs = append(msgs, "control with empty name")
			continue
		}
		if pending := c.Validate(); len(pending) > 0 {
			msgs = append(msgs, pending...)
		}
		if seen[c.Name] {
			msgs = append(msgs, fmt.Sprintf("duplicate control name %q", c.Name))
		}
		seen[c.Name] = true
	}
	return msgs
}

// Validate checks a single control.
func (c *Control) Validate() []string {
	var msgs []string
	if !identRe.MatchString(c.Name) {
		msgs = append(msgs, fmt.Sprintf("control %q: name is not a valid Lua identifier", c.Name))
	}
	if !contains(Types, c.Type) {
		msgs = append(msgs, fmt.Sprintf("control %q: unknown type %q", c.Name, c.Type))
	}
	return msgs
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// RawJSON is a helper that serializes v to indented JSON.
func RawJSON(v any) ([]byte, error) {
	return json.MarshalIndent(v, "", "  ")
}
