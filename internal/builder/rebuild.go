package builder

import (
	"encoding/json"
	"fmt"
	"strings"
)

// RebuildLua merges the (possibly edited) multi-form document back into the
// original Lua source using surgical line splicing: only the statement lines a
// changed/new form owns are touched, replaced, or removed; everything else —
// non-form code, comments, blank lines, untouched forms — survives
// byte-for-byte. Unchanged forms are detected by comparing the document's form
// against a fresh import of the source (formEqual), so opening a file and
// saving without edits rewrites nothing.
//
// The result is re-parsed before returning; a splice that would produce
// invalid Lua is rejected with an error so the caller can keep the old file.
func RebuildLua(source string, doc *Document) (string, error) {
	if source == "" {
		return ExportLua(doc), nil
	}
	cur, err := Import(source, "_rebuild.lua")
	if err != nil {
		if strings.Contains(err.Error(), "no k.form.new call found") {
			// A pure non-form script: append a main() with the document's forms.
			return appendMainToScript(source, doc), nil
		}
		// Unparsable source — regenerate the whole document. The caller already
		// warned the user at load time.
		return ExportLua(doc), nil
	}
	if len(cur.Forms) == 0 {
		return appendMainToScript(source, doc), nil
	}
	lines := strings.Split(source, "\n")

	// A single index-aligned rename (same form count, names equal except one
	// position) maps the document form onto the source block at the same
	// position, replacing it in place. Otherwise alignment is by name.
	renIdx := renameIndex(cur, doc)

	del := map[int]bool{}                  // 1-based lines to drop
	insertAt := map[int][]string{}         // 1-based anchor line → replacement block lines
	var newForms []*Form                   // appended after the last form block
	var consumedSrc []bool = make([]bool, len(cur.Forms))
	maxEnd := 0                            // last owned line across all source forms

	// iterate source forms; find their document counterpart
	for i, sf := range cur.Forms {
		df := docCounterpart(doc, sf, i, renIdx)
		if len(sf.Lines) == 0 {
			continue
		}
		for _, span := range sf.Lines {
			if span[1] > maxEnd {
				maxEnd = span[1]
			}
		}
		if df == nil {
			// source form not present in the document → deleted
			consumedSrc[i] = true
			markDeleted(del, sf)
			markBlankGaps(del, sf, lines)
			continue
		}
		consumedSrc[i] = true
		if formEqual(df, sf) {
			continue // untouched — leave its lines alone
		}
		block := GenerateFormLua(patchOrphans(df, sf), blockIndent(sf, df), sf.HasShow)
		if sigText(block) == ownedText(lines, sf) {
			// The source already carries exactly this form (a previous save's
			// echo — e.g. auto-generated stubs the document itself lacks). A
			// re-splice would be a no-op on the statements and would only add
			// stray blank lines, so leave it alone.
			continue
		}
		anchor := formAnchor(sf)
		if cur := insertAt[anchor]; cur != nil {
			insertAt[anchor] = append(cur, block)
		} else {
			insertAt[anchor] = []string{block}
		}
		markDeleted(del, sf)
		markBlankGaps(del, sf, lines)
	}

	// Document forms without a source counterpart that wasn't consumed as a
	// rename are new forms appended after the last form block.
	for _, df := range doc.Forms {
		found := false
		for i, sf := range cur.Forms {
			if df.Name == sf.Name && consumedSrc[i] {
				found = true
				break
			}
		}
		if found {
			continue
		}
		// renIdx counts as "found" for the renamed doc form
		if renIdx >= 0 && renIdx < len(doc.Forms) && doc.Forms[renIdx].Name == df.Name {
			continue
		}
		newForms = append(newForms, df)
	}

	// Anchor for appended new forms: just after the last source form block.
	insertPos := len(lines)+1
	if maxEnd > 0 {
		for ln := maxEnd+1; ln <= len(lines); ln++ {
			if !del[ln] {
				insertPos = ln
				break
			}
		}
		if insertPos == len(lines)+1 {
			for ln := maxEnd; ln >= 1; ln-- {
				if !del[ln] {
					insertPos = ln+1
					break
				}
			}
		}
	}

	// Reconstruct the file from the original lines (join semantics preserve the
	// exact trailing newline), with deleted lines dropped and new/replacement
	// blocks woven in at their anchors.
	var out []string
	for ln := 1; ln <= len(lines); ln++ {
		if blk := insertAt[ln]; blk != nil && len(blk) > 0 {
			out = append(out, strings.Join(blk, "\n"))
		}
		if ln == insertPos && len(newForms) > 0 {
			for _, df := range newForms {
				out = append(out, GenerateFormLua(df, "  ", true))
			}
		}
		if !del[ln] {
			out = append(out, lines[ln-1])
		}
	}
	if insertPos > len(lines) && len(newForms) > 0 {
		for _, df := range newForms {
			out = append(out, GenerateFormLua(df, "  ", true))
		}
	}
	final := strings.Join(out, "\n")

	// Safety net: a surgical text editor must fail safe — never write bytes
	// that do not parse.
	if _, rerr := Import(final, "_checked.lua"); rerr != nil {
		return "", fmt.Errorf("assembled Lua failed to re-import (%v); change not saved", rerr)
	}
	return final, nil
}

// appendMainToScript appends function main() with the document's forms to a
// script that contains no form at all, keeping its non-form code untouched.
func appendMainToScript(source string, doc *Document) string {
	var sb strings.Builder
	sb.WriteString(strings.TrimRight(source, "\n"))
	sb.WriteString("\n\nfunction main()\n")
	for _, f := range doc.Forms {
		for _, line := range strings.Split(GenerateFormLua(f, "  ", true), "\n") {
			sb.WriteString(line + "\n")
		}
	}
	sb.WriteString("end\n")
	return sb.String()
}

// docCounterpart maps a source form to its document counterpart: by name, or
// by position when this is the index-aligned rename.
func docCounterpart(doc *Document, sf *Form, i, renIdx int) *Form {
	if f := findForm(doc, sf.Name); f != nil {
		return f
	}
	if i == renIdx && renIdx >= 0 && renIdx < len(doc.Forms) {
		return doc.Forms[i]
	}
	return nil
}

// renameIndex returns the cur index of an index-aligned rename, or -1.
func renameIndex(cur, doc *Document) int {
	if len(cur.Forms) != len(doc.Forms) || len(cur.Forms) == 0 {
		return -1
	}
	diff := -1
	for i := 0; i < len(cur.Forms); i++ {
		if cur.Forms[i].Name != doc.Forms[i].Name {
			if diff >= 0 {
				return -1
			}
			diff = i
		}
	}
	return diff
}

// formAnchor is the first owned line of a form's block (its k.form.new).
func formAnchor(sf *Form) int {
	anchor := 0
	for _, span := range sf.Lines {
		if anchor == 0 || span[0] < anchor {
			anchor = span[0]
		}
	}
	if anchor == 0 {
		return 1
	}
	return anchor
}

// blockIndent chooses the generated block indentation: the source form's
// original indent when present, else two spaces.
func blockIndent(sf, df *Form) string {
	if sf != nil && sf.Indent != "" {
		return sf.Indent
	}
	if df != nil && df.Indent != "" {
		return df.Indent
	}
	return "  "
}

// markDeleted adds a form's owned lines to the deletion set.
func markDeleted(del map[int]bool, f *Form) {
	for _, span := range f.Lines {
		for ln := span[0]; ln <= span[1]; ln++ {
			del[ln] = true
		}
	}
}

// markBlankGaps extends a regenerate/delete to also drop blank lines that sit
// strictly between a form's owned statements, so re-spliced blocks do not pile
// up empty lines. Comments and interleaved non-form code are preserved.
func markBlankGaps(del map[int]bool, f *Form, lines []string) {
	n := len(f.Lines)
	for i := 0; i+1 < n; i++ {
		for ln := f.Lines[i][1]+1; ln < f.Lines[i+1][0]; ln++ {
			if ln >= 1 && ln <= len(lines) && strings.Trim(lines[ln-1], " \t") == "" {
				del[ln] = true
			}
		}
	}
}

// ownedText returns the non-blank source text of a form's owned statements, in
// file order, trimmed per line — a canonical signature for idempotence checks.
func ownedText(lines []string, f *Form) string {
	var out []string
	for _, span := range f.Lines {
		for ln := span[0]; ln <= span[1]; ln++ {
			if ln >= 1 && ln <= len(lines) && strings.Trim(lines[ln-1], " \t") != "" {
				out = append(out, strings.Trim(lines[ln-1], " \t"))
			}
		}
	}
	return strings.Join(out, "\n")
}

// sigText canonicalizes a generated block the same way as ownedText.
func sigText(block string) string {
	var out []string
	for _, l := range strings.Split(block, "\n") {
		if strings.Trim(l, " \t") != "" {
			out = append(out, strings.Trim(l, " \t"))
		}
	}
	return strings.Join(out, "\n")
}

// patchOrphans decides which stale (renamed-control) handler statements to
// preserve verbatim on a regenerated form. A rename is inferred when exactly
// one handler-bearing control disappeared and exactly one new control
// appeared; only then are the orphaned registrations kept (they reference the
// old name, per the "never rewrite a handler" rule). Fix-relative and return
// a copy-free view by mutating the document form directly.
func patchOrphans(df, sf *Form) *Form {
	if sf == nil {
		return df
	}
	var removed []string
	for _, sc := range sf.Controls {
		if !hasCtrl(df, sc.Name) {
			removed = append(removed, sc.Name)
		}
	}
	var added []string
	for _, dc := range df.Controls {
		if !hasCtrl(sf, dc.Name) {
			added = append(added, dc.Name)
		}
	}
	var orphans []string
	for name := range df.Handlers {
		if name == "@form" || hasCtrl(df, name) {
			continue
		}
		orphans = append(orphans, name)
	}
	if len(orphans) == 1 && len(removed) == 1 && len(added) == 1 && orphans[0] == removed[0] {
		df.OrphanKeys = orphans
	} else {
		df.OrphanKeys = nil
	}
	return df
}

// formEqual compares the document form against a freshly imported form,
// ignoring transient bookkeeping (Lines/Indent/OrphanKeys/Notes). Nil-vs-empty
// is normalized so the browser's round-tripped document matches the import.
func formEqual(a, b *Form) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Name != b.Name || a.Title != b.Title || a.Layout != b.Layout || a.Align != b.Align {
		return false
	}
	if gapEq(a.Gap, b.Gap) == false {
		return false
	}
	if jz(a.Cells) != jz(b.Cells) {
		return false
	}
	if jz(a.Controls) != jz(b.Controls) {
		return false
	}
	if jz(a.Handlers) != jz(b.Handlers) {
		return false
	}
	if jz(a.HandlerBodies) != jz(b.HandlerBodies) {
		return false
	}
	return true
}

func gapEq(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// jz canonicalizes nil and empty containers before marshalling so "no cells"
// on one side and an empty array on the other compare equal.
func jz(v any) string {
	switch n := v.(type) {
	case nil:
		return "Ø"
	case []any:
		if len(n) == 0 {
			return "Ø"
		}
	case map[string]any:
		if len(n) == 0 {
			return "Ø"
		}
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("ERR:%v", err)
	}
	return string(b)
}
