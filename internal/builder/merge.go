package builder

import (
	"encoding/json"
	"strconv"
)

// MergeDocument combines an AI-generated document on top of the currently
// open form so "Apply to Builder" never destroys the builder's state.
//
// Semantics (user decision):
//   - Form identity: the base form's Name is always kept.
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
//   - Handlers / handler bodies: overlaid per key (generated wins).
//   - Notes: base notes are kept, generated notes appended.
func MergeDocument(base, gen *Document) *Document {
	if base == nil || base.Form == nil {
		return gen
	}
	if gen == nil || gen.Form == nil {
		return base
	}

	merged := cloneDoc(base)
	bf := merged.Form
	gf := gen.Form

	if gf.Title != "" {
		bf.Title = gf.Title
	}
	if gf.Layout != "" {
		bf.Layout = gf.Layout
	}
	if gf.Align != "" {
		bf.Align = gf.Align
	}
	if gf.Gap != nil {
		bf.Gap = gf.Gap
	}
	bf.Cells = mergeCells(bf.Cells, gf.Cells)

	before := len(bf.Controls)
	bf.Controls = mergeControls(bf.Controls, gf.Controls)
	bf.Handlers = mergeStringSlices(bf.Handlers, gf.Handlers)
	bf.HandlerBodies = mergeStrings(bf.HandlerBodies, gf.HandlerBodies)
	if len(gf.Notes) > 0 {
		bf.Notes = append(bf.Notes, gf.Notes...)
	}

	added := len(bf.Controls) - before
	if before > 0 || added > 0 {
		bf.Notes = append(bf.Notes, mergeNote(before, added))
	}
	return merged
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