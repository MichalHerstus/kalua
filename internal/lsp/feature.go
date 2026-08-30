package lsp

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"kalua/internal/bindings"
)

// Position helpers. The server advertises UTF-8 position encoding, so an LSP
// character offset equals a byte offset within its line.

// byteOffset converts an LSP position to a byte offset into text. The offset is
// clamped into the cursor's line (clients may report a character just past the
// end of line, typically before the trailing newline).
func byteOffset(text string, pos protocol.Position) int {
	line := int(pos.Line)
	idx := 0
	for n := 0; n < line && idx < len(text); n++ {
		o := strings.IndexByte(text[idx:], '\n')
		if o < 0 {
			return len(text)
		}
		idx += o + 1
	}
	lineEnd := len(text)
	if o := strings.IndexByte(text[idx:], '\n'); o >= 0 {
		lineEnd = idx + o
	}
	off := idx + int(pos.Character)
	if off > lineEnd {
		off = lineEnd
	}
	return off
}

// posAt converts a byte offset back into an LSP position.
func posAt(text string, off int) protocol.Position {
	if off > len(text) {
		off = len(text)
	}
	line := 0
	lineStart := 0
	for i := 0; i < off; i++ {
		if text[i] == '\n' {
			line++
			lineStart = i + 1
		}
	}
	return protocol.Position{Line: uint32(line), Character: uint32(off - lineStart)}
}

// sortedDocs is the k.* documentation sorted by name for deterministic output.
var sortedDocs = func() []bindings.Info {
	docs := bindings.Docs()
	names := make([]string, 0, len(docs))
	for n := range docs {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]bindings.Info, 0, len(names))
	for _, n := range names {
		out = append(out, docs[n])
	}
	return out
}()

// sortedKSets is the K.* helper documentation sorted by name.
var sortedKSets = func() []bindings.Info {
	out := bindings.KInfo()
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}()

// sortedExprs is the §5.9 expression-function documentation sorted by name.
var sortedExprs = func() []bindings.Info {
	out := bindings.ExprInfo()
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}()

// completionPrefix matches the trailing "k.<rest>" or "K.<rest>" token before
// the cursor. Group 1 is the namespace letter; group 2 is what follows the dot.
var completionPrefix = regexp.MustCompile(`\b([kK])(?:\.([a-zA-Z0-9_.]*))?$`)

// bareIdentPrefix matches a trailing bare Lua identifier (name only), used to
// offer the §5.9 expression-function globals when the cursor is inside one.
var bareIdentPrefix = regexp.MustCompile(`\b([a-zA-Z_][a-zA-Z0-9_]*)$`)

// Completion computes completion items at an absolute byte offset.
func Completion(text string, cursor int) []protocol.CompletionItem {
	// Trim trailing horizontal whitespace so the prefix regex (anchored at the
	// document/line end via $) still matches when the cursor sits past a final
	// newline. Offsets remain valid in the original text; only trailing spaces,
	// tabs, and newlines are removed.
	prefix := strings.TrimRight(text[:cursor], " \t\r\n")
	loc := completionPrefix.FindStringSubmatchIndex(prefix)
	if loc == nil || loc[0] < 0 {
		return completeBareIdent(text, prefix, cursor)
	}
	start := loc[0]
	kind := "k"
	if loc[2] >= 0 {
		kind = prefix[loc[2]:loc[3]]
	}
	after := ""
	if loc[4] >= 0 {
		after = prefix[loc[4]:loc[5]]
	}

	if kind == "K" {
		return completeK(text, after, start, cursor)
	}
	return completeKapi(text, after, start, cursor)
}

// completeBareIdent offers expression-function globals and the script globals
// (ARGS/CTRL/main) when the typed prefix is a plain identifier, e.g. "up" →
// upper/upcase-related only; "wig" → nothing. Bare identifiers that also name
// a k.* group (form/ctrl/...) are resolved through the member scope instead.
func completeBareIdent(text, prefix string, cursor int) []protocol.CompletionItem {
	loc := bareIdentPrefix.FindStringSubmatchIndex(prefix)
	if loc == nil || loc[0] < 0 {
		return nil
	}
	start := loc[0]
	after := prefix[loc[2]:loc[3]]
	if len(after) < 1 {
		return nil
	}
	// The user is typing inside a dotted access (obj.<name>) which is a k./K.
	// reference handled above (they require the k/K prefix); suppress bare
	// expression completion so "x." doesn't suggest expression functions.
	if start > 0 && prefix[start-1] == '.' {
		return nil
	}

	var items []protocol.CompletionItem
	for _, info := range sortedExprs {
		if !strings.HasPrefix(info.Name, after) {
			continue
		}
		item := protocol.CompletionItem{
			Label:            info.Name,
			Kind:             protocol.CompletionItemKindFunction,
			Detail:           protocol.NewOptional(info.Signature),
			Documentation:    &protocol.MarkupContent{Kind: protocol.MarkupKindMarkdown, Value: markdown(info)},
			SortText:         protocol.NewOptional(info.Name),
			InsertTextFormat: protocol.InsertTextFormatPlainText,
			TextEdit:         textEditAt(text, start, cursor, info.Name),
		}
		items = append(items, item)
	}
	// script globals
	for _, g := range bindings.GlobalsList() {
		if !strings.HasPrefix(g, after) {
			continue
		}
		items = append(items, globalItem(text, g, start, cursor))
	}
	return items
}

func completeK(text, after string, start, cursor int) []protocol.CompletionItem {
	var items []protocol.CompletionItem
	for _, info := range sortedKSets {
		base := strings.TrimPrefix(info.Name, "K.")
		if !strings.HasPrefix(base, after) {
			continue
		}
		item := apiItem(text, info, info.Name, "K", start, cursor)
		if strings.HasPrefix(base, "is_") || base == "NULL" {
			item.Kind = protocol.CompletionItemKindConstant
		} else {
			item.Kind = protocol.CompletionItemKindFunction
		}
		items = append(items, item)
	}
	return items
}

func completeKapi(text, after string, start, cursor int) []protocol.CompletionItem {
	// Namespace member mode: "k.form.<partial>" / "k.ctrl." / "k.table."
	if ns, partial, ok := memberScope(after); ok {
		var items []protocol.CompletionItem
		for _, info := range sortedDocs {
			if !strings.HasPrefix(info.Name, ns+".") {
				continue
			}
			rest := strings.TrimPrefix(info.Name, ns+".")
			if !strings.HasPrefix(rest, partial) {
				continue
			}
			items = append(items, memberItem(text, info, info.Name, "k", rest, start, cursor))
		}
		return items
	}

	var items []protocol.CompletionItem
	// namespace roots
	for _, ns := range []string{"form", "ctrl", "table"} {
		if !strings.HasPrefix(ns, after) {
			continue
		}
		items = append(items, namespaceItem(text, ns, start, cursor))
	}
	// top-level k.* functions
	for _, info := range sortedDocs {
		if bindings.Namespace(info.Name) { // form/ctrl/table handled above
			continue
		}
		if !strings.HasPrefix(info.Name, after) {
			continue
		}
		items = append(items, apiItem(text, info, info.Name, "k", start, cursor))
	}
	// script globals
	for _, g := range bindings.GlobalsList() {
		if !strings.HasPrefix(g, after) {
			continue
		}
		items = append(items, globalItem(text, g, start, cursor))
	}
	return items
}

// memberScope reports whether after names a namespace member scope and returns
// the namespace and the remaining partial. e.g. "form.ad" → "form", "ad".
func memberScope(after string) (ns, partial string, ok bool) {
	first, rest, hasDot := strings.Cut(after, ".")
	if bindings.Namespace(first) {
		return first, rest, true
	}
	if !hasDot && bindings.Namespace(after) {
		return after, "", true
	}
	return "", "", false
}

func apiItem(text string, info bindings.Info, name, prefixKind string, start, cursor int) protocol.CompletionItem {
	return protocol.CompletionItem{
		Label:            name,
		Kind:             protocol.CompletionItemKindFunction,
		Detail:           protocol.NewOptional(info.Signature),
		Documentation:    &protocol.MarkupContent{Kind: protocol.MarkupKindMarkdown, Value: markdown(info)},
		SortText:         protocol.NewOptional(name),
		InsertTextFormat: protocol.InsertTextFormatPlainText,
		TextEdit:         textEditAt(text, start, cursor, prefixKind+"."+name),
	}
}

// memberItem labels a namespace member by its leaf name for tighter matching
// ("ad" → "add_line") while insert text remains the fully qualified name.
func memberItem(text string, info bindings.Info, name, prefixKind, label string, start, cursor int) protocol.CompletionItem {
	item := apiItem(text, info, name, prefixKind, start, cursor)
	item.Label = label
	return item
}

func namespaceItem(text, ns string, start, cursor int) protocol.CompletionItem {
	return protocol.CompletionItem{
		Label:            ns,
		Kind:             protocol.CompletionItemKindClass,
		Detail:           protocol.NewOptional("k." + ns),
		Documentation:    &protocol.MarkupContent{Kind: protocol.MarkupKindMarkdown, Value: "Namespace: `k." + ns + ".*`"},
		InsertTextFormat: protocol.InsertTextFormatPlainText,
		TextEdit:         textEditAt(text, start, cursor, "k."+ns),
	}
}

func globalItem(text, name string, start, cursor int) protocol.CompletionItem {
	return protocol.CompletionItem{
		Label:            name,
		Kind:             protocol.CompletionItemKindVariable,
		Detail:           protocol.NewOptional("global"),
		InsertTextFormat: protocol.InsertTextFormatPlainText,
		TextEdit:         textEditAt(text, start, cursor, name),
	}
}

func textEditAt(text string, start, cursor int, newText string) *protocol.TextEdit {
	return &protocol.TextEdit{
		Range:   protocol.Range{Start: posAt(text, start), End: posAt(text, cursor)},
		NewText: newText,
	}
}

// nameToken matches the longest k.* or K.* identifier at the cursor, e.g.
// "k.form.new", "k.print", "K.eq".
var nameToken = regexp.MustCompile(`[kK]\.[a-zA-Z_][a-zA-Z0-9_]*(\.[a-zA-Z_][a-zA-Z0-9_]*)*`)

// bareToken matches a plain Lua identifier at the cursor (used for §5.9
// expression-function globals).
var bareToken = regexp.MustCompile(`[a-zA-Z_][a-zA-Z0-9_]*`)

// match returns the tooling Info and byte range of the k.* / K.* token whose
// span contains cursor (longest wins), falling back to a bare expression
// function identifier (upper, round, iif, ...). Returns nil when the cursor is
// not on a known token.
func match(text string, cursor int) (*bindings.Info, int, int) {
	best := ""
	bs, be := 0, 0
	for _, loc := range nameToken.FindAllStringIndex(text, -1) {
		start, end := loc[0], loc[1]
		if cursor >= start && cursor <= end && end-start > be-bs {
			best, bs, be = text[start:end], start, end
		}
	}
	if best == "" {
		return matchBare(text, cursor)
	}
	info := lookup(best)
	if info == nil {
		return nil, 0, 0
	}
	return info, bs, be
}

// matchBare locates a bare expression-function identifier at the cursor.
func matchBare(text string, cursor int) (*bindings.Info, int, int) {
	best := ""
	bs, be := 0, 0
	for _, loc := range bareToken.FindAllStringIndex(text, -1) {
		start, end := loc[0], loc[1]
		if cursor >= start && cursor <= end && end-start > be-bs {
			best, bs, be = text[start:end], start, end
		}
	}
	if best == "" {
		return nil, 0, 0
	}
	info := lookupExpr(best)
	if info == nil {
		return nil, 0, 0
	}
	return info, bs, be
}

// lookup resolves a matched token to its tooling documentation.
func lookup(token string) *bindings.Info {
	if strings.HasPrefix(token, "K.") {
		for i := range sortedKSets {
			if sortedKSets[i].Name == token {
				return &sortedKSets[i]
			}
		}
		return nil
	}
	if info, ok := bindings.Docs()[strings.TrimPrefix(token, "k.")]; ok {
		return &info
	}
	return nil
}

// lookupExpr resolves a bare identifier to an expression-function doc.
func lookupExpr(name string) *bindings.Info {
	for i := range sortedExprs {
		if sortedExprs[i].Name == name {
			return &sortedExprs[i]
		}
	}
	return nil
}

// markdown renders an Info entry as hover/completion documentation.
func markdown(info bindings.Info) string {
	sig := info.Signature
	if info.Name != "" && !strings.HasPrefix(sig, info.Name) {
		sig = info.Signature
	}
	var b strings.Builder
	b.WriteString("```lua\n" + sig + "\n```\n\n")
	b.WriteString(info.Docs)
	if info.Group != "" {
		b.WriteString("\n\n*Group: " + info.Group + "*")
	}
	return b.String()
}

// /////// Go-to-definition via a generated API reference stub /////////

// ensureReference materializes a synthetic Lua source that declares every k.*
// and K.* binding, one per line, plus a name → line index. Go-to-definition
// jumps into this file. Generation failure is surfaced to the caller through
// refErr and degrades definition to a no-op.
func (s *Server) ensureReference() {
	s.refOnce.Do(func() {
		var b strings.Builder
		b.WriteString("-- KALUA generated API reference (do not edit).\n")
		line := 1
		m := make(map[string]int)

		add := func(decl string) {
			b.WriteString(decl + "\n")
			line++
		}
		emit := func(name, signature string) {
			m[name] = line
			add("-- " + signature)
			add(name + " = function() end")
			add("")
		}

		for _, info := range sortedDocs {
			emit(info.Name, info.Signature)
		}
		for _, info := range sortedKSets {
			emit(info.Name, info.Signature)
		}
		for _, info := range sortedExprs {
			emit(info.Name, info.Signature)
		}

		path := filepath.Join(os.TempDir(), refFileName)
		if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
			s.refErr = err
			return
		}
		s.refPath = path
		s.refLine = m
	})
}

// definition returns a single-file Location into the API reference for the
// token at the cursor, or nil when there is nothing useful to jump to.
func (s *Server) definition(text string, cursor int) *protocol.Location {
	info, _, _ := match(text, cursor)
	if info == nil {
		return nil
	}
	s.ensureReference()
	if s.refErr != nil || s.refPath == "" {
		return nil
	}
	lineNo, ok := s.refLine[info.Name]
	if !ok {
		return nil
	}
	l := uint32(lineNo - 1)
	return &protocol.Location{
		URI: uri.URI("file://" + s.refPath),
		Range: protocol.Range{
			Start: protocol.Position{Line: l, Character: 0},
			End:   protocol.Position{Line: l, Character: 1},
		},
	}
}
