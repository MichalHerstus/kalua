package builder

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// MergeDocument combines an AI-generated document on top of the currently
// open document so "Apply to Builder" never destroys the builder's state.
// With multiple forms, generated forms are matched by name onto base forms;
// forms only in the base are kept, forms only in the generated script are
// appended.
//
// Per-form semantics (user decision):
//   - Form identity: the base form's Name is always kept (a generated form
//     matching by name inherits the base position and keeps its own name).
//   - Form scaffold: Title/Layout/Align/Gap are adopted from the generated
//     form when the generated form sets them (non-empty).
//   - Cells: merged by id — a generated cell replaces the same-id base cell,
//     new-generated cells are appended, base-only cells are kept (never
//     deleted).
//   - Controls: matched by unique name. A matched control keeps its position,
//     adopts the generated type, and its opts/inline handlers are overlaid so
//     generated values win per key while base options the model did not
//     mention survive. New names are appended. Base controls absent from the
//     generated script are kept — a merge never deletes.
//   - Handlers / handler bodies / verbatim handler statements: overlaid per
//     key (generated wins). Lines/Indent/HasShow follow the base.
//   - Notes: base notes are kept, generated notes appended.
func MergeDocument(base, gen *Document) *Document {
	if base == nil {
		return gen
	}
	if gen == nil {
		return base
	}
	merged := &Document{Version: DocVersion, ActiveForm: base.ActiveForm}
	byName := map[string]*Form{}
	order := make([]string, 0, len(base.Forms)+len(gen.Forms))
	for _, bf := range base.Forms {
		if bf == nil || bf.Name == "" {
			continue
		}
		byName[bf.Name] = bf
		order = append(order, bf.Name)
	}
	for _, gf := range gen.Forms {
		if gf == nil || gf.Name == "" {
			continue
		}
		if byName[gf.Name] != nil {
			byName[gf.Name] = mergeForm(byName[gf.Name], gf)
			continue
		}
		byName[gf.Name] = gf
		order = append(order, gf.Name)
	}
	for _, name := range order {
		merged.Forms = append(merged.Forms, byName[name])
	}
	// Notes: doc-level for appended forms, form-level on the merged form.
	for _, gf := range gen.Forms {
		if gf == nil || gf.Name == "" {
			continue
		}
		if findForm(base, gf.Name) == nil {
			merged.Notes = append(merged.Notes, fmt.Sprintf("Added form %q from the AI result.", gf.Name))
		}
	}
	if merged.ActiveForm == "" {
		if f := firstForm(merged); f != nil {
			merged.ActiveForm = f.Name
		}
	}
	return merged
}

func mergeForm(base, gen *Form) *Form {
	m := &Form{
		Name:          base.Name,
		Title:         base.Title,
		Layout:        base.Layout,
		Align:         base.Align,
		Gap:           base.Gap,
		Handlers:      base.Handlers,
		HandlerBodies: base.HandlerBodies,
		Notes:         base.Notes,
		Lines:         base.Lines,
		Indent:        base.Indent,
		HasShow:       base.HasShow,
		OrphanKeys:    base.OrphanKeys,
	}
	if gen.Title != "" {
		m.Title = gen.Title
	}
	if gen.Layout != "" {
		m.Layout = gen.Layout
	}
	if gen.Align != "" {
		m.Align = gen.Align
	}
	if gen.Gap != nil {
		m.Gap = gen.Gap
	}
	m.Cells = mergeCells(base.Cells, gen.Cells)
	m.Controls = mergeControls(base.Controls, gen.Controls)
	m.Handlers = mergeStringSlices(base.Handlers, gen.Handlers)
	m.HandlerBodies = mergeStrings(base.HandlerBodies, gen.HandlerBodies)
	m.Notes = append(m.Notes, "Merged AI changes into the existing form (no controls removed).")
	if len(gen.Notes) > 0 {
		m.Notes = append(m.Notes, gen.Notes...)
	}
	return m
}

func mergeNote(kept, added int) string {
	if added == 0 {
		return "Merged AI changes into the existing form (no controls removed)."
	}
	return "Merged AI changes into the existing form: kept all controls, added " +
		strconv.Itoa(added) + " new one(s)."
}

func cloneDoc(d *Document) *Document {
	b, _ := json.Marshal(d)
	var o Document
	_ = json.Unmarshal(b, &o)
	return &o
}

func mergeControls(base, gen []*Control) []*Control {
	out := make([]*Control, 0, len(base)+len(gen))
	index := make(map[string]int, len(base)+len(gen))
	for _, c := range base {
		if c == nil {
			out = append(out, nil)
			continue
		}
		index[c.Name] = len(out)
		out = append(out, c)
	}
	for _, c := range gen {
		if c == nil || c.Name == "" {
			continue
		}
		if i, ok := index[c.Name]; ok && out[i] != nil {
			out[i] = mergeControl(out[i], c)
			continue
		}
		index[c.Name] = len(out)
		out = append(out, c)
	}
	return out
}

func mergeControl(base, gen *Control) *Control {
	m := &Control{
		Name:   gen.Name,
		Type:   gen.Type,
		Opts:   make(map[string]any),
		Inline: make(map[string]string),
	}
	if gen.Type == "" {
		m.Type = base.Type
	}
	for k, v := range base.Opts {
		m.Opts[k] = v
	}
	for k, v := range gen.Opts {
		m.Opts[k] = v
	}
	for k, v := range base.Inline {
		m.Inline[k] = v
	}
	for k, v := range gen.Inline {
		m.Inline[k] = v
	}
	return m
}

func mergeCells(base, gen []*CellDef) []*CellDef {
	order := make([]string, 0, len(base)+len(gen))
	byID := make(map[string]*CellDef, len(base)+len(gen))
	for _, c := range base {
		if c == nil || c.Id == "" {
			continue
		}
		if _, ok := byID[c.Id]; !ok {
			order = append(order, c.Id)
		}
		byID[c.Id] = c
	}
	for _, c := range gen {
		if c == nil || c.Id == "" {
			continue
		}
		if _, ok := byID[c.Id]; ok {
			byID[c.Id] = c // replace def, keep position
			continue
		}
		order = append(order, c.Id)
		byID[c.Id] = c
	}
	out := make([]*CellDef, 0, len(order))
	for _, id := range order {
		out = append(out, byID[id])
	}
	return out
}

func mergeStrings(base, gen map[string]string) map[string]string {
	if len(gen) == 0 {
		return base
	}
	if len(base) == 0 {
		return gen
	}
	out := make(map[string]string, len(base)+len(gen))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range gen {
		out[k] = v
	}
	return out
}

func mergeStringSlices(base, gen map[string][]string) map[string][]string {
	if len(gen) == 0 {
		return base
	}
	if len(base) == 0 {
		return gen
	}
	out := make(map[string][]string, len(base)+len(gen))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range gen {
		out[k] = v
	}
	return out
}