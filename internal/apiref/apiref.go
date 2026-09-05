package apiref

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"kalua/internal/bindings"
)

// Render generates the full API reference markdown from the single source of truth.
func Render() string {
	var buf bytes.Buffer

	buf.WriteString("# KALUA API Reference\n\n")
	buf.WriteString("> Auto-generated from `internal/bindings/api_doc.go`. Do not edit manually.\n")
	buf.WriteString("> Run `make gen-api` to regenerate.\n\n")

	renderKBindings(&buf)
	renderKHelpers(&buf)
	renderExprFuncs(&buf)
	renderGlobals(&buf)

	return buf.String()
}

func renderKBindings(buf *bytes.Buffer) {
	docs := bindings.Docs()
	namespaces := map[string]bool{
		"form": true, "ctrl": true, "table": true, "shared": true,
		"ws": true, "tcp": true, "xml": true,
	}

	groups := map[string][]string{}
	for name, info := range docs {
		if namespaces[name] {
			continue
		}
		groups[info.Group] = append(groups[info.Group], name)
	}

	groupOrder := []string{
		"flow", "debug", "forms", "controls", "database", "files",
		"json", "crypto", "xml", "server", "comm", "email", "formats", "rows",
	}

	buf.WriteString("## k.* Bindings\n\n")

	for _, group := range groupOrder {
		names, ok := groups[group]
		if !ok {
			continue
		}
		sort.Strings(names)

		buf.WriteString(fmt.Sprintf("### %s\n\n", strings.Title(group)))

		for _, name := range names {
			info := docs[name]
			buf.WriteString(fmt.Sprintf("**`%s`**  \n", info.Signature))
			buf.WriteString(fmt.Sprintf("%s\n\n", info.Docs))
		}
	}
}

func renderKHelpers(buf *bytes.Buffer) {
	helpers := bindings.KInfo()

	buf.WriteString("## K.* Helpers & Constants\n\n")

	for _, h := range helpers {
		buf.WriteString(fmt.Sprintf("**`%s`**  \n", h.Signature))
		buf.WriteString(fmt.Sprintf("%s\n\n", h.Docs))
	}
}

func renderExprFuncs(buf *bytes.Buffer) {
	funcs := bindings.ExprInfo()

	groups := map[string][]*bindings.Info{}
	for i := range funcs {
		f := &funcs[i]
		groups[f.Group] = append(groups[f.Group], f)
	}

	groupOrder := []string{
		"string", "numeric", "conditional", "datetime", "conversion",
	}

	buf.WriteString("## Expression Functions (§5.9)\n\n")
	buf.WriteString("> Flat globals (not under `k.*`) — Kalipso-style expressions.\n\n")

	for _, group := range groupOrder {
		list, ok := groups[group]
		if !ok {
			continue
		}
		sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })

		buf.WriteString(fmt.Sprintf("### %s\n\n", strings.Title(group)))

		for _, f := range list {
			buf.WriteString(fmt.Sprintf("**`%s`**  \n", f.Signature))
			buf.WriteString(fmt.Sprintf("%s\n\n", f.Docs))
		}
	}
}

func renderGlobals(buf *bytes.Buffer) {
	globals := bindings.GlobalsList()

	buf.WriteString("## Script Globals\n\n")

	for _, g := range globals {
		var desc string
		switch g {
		case "ARGS":
			desc = "Table seeded from `--arg K=V` flags (string keys)."
		case "CTRL":
			desc = "Accessor: `CTRL(name)` returns a control handle for `k.ctrl.*` operations."
		case "main":
			desc = "Entry point function (required in run mode)."
		}
		buf.WriteString(fmt.Sprintf("- **`%s`** — %s\n", g, desc))
	}
	buf.WriteString("\n")
}