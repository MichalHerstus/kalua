// Command kalua-userguide regenerates the auto-generated API reference
// section embedded in docs/USER_GUIDE.md between the KALUA:gen-api markers.
//
// The generated block is produced from internal/bindings/api_doc.go (the
// single source of truth) via internal/apiref.Render, with headings shifted
// one level so it nests under the guide's chapter heading. Everything else in
// the file is preserved verbatim.
//
// Usage:
//
//	go run ./cmd/kalua-userguide            # regenerate docs/USER_GUIDE.md in place
//	go run ./cmd/kalua-userguide -check     # fail on drift (CI)
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"kalua/internal/apiref"
)

const (
	startMarker = "<!-- KALUA:gen-api -->"
	endMarker   = "<!-- KALUA:gen-api-end -->"
)

func main() {
	check := flag.Bool("check", false, "Check that the committed guide matches generated output")
	flag.Parse()

	guidePath := filepath.Join(findRepoRoot(), "docs", "USER_GUIDE.md")

	guide, err := os.ReadFile(guidePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading %s: %v\n", guidePath, err)
		os.Exit(1)
	}

	regenerated := splice(guide)

	if *check {
		if !bytes.Equal(guide, regenerated) {
			fmt.Fprintf(os.Stderr, "docs/USER_GUIDE.md drift detected! Run 'go run ./cmd/kalua-userguide' to regenerate.\n")
			os.Exit(1)
		}
		fmt.Println("docs/USER_GUIDE.md is in sync")
		return
	}

	if err := os.WriteFile(guidePath, regenerated, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing %s: %v\n", guidePath, err)
		os.Exit(1)
	}
	fmt.Printf("regenerated %s\n", guidePath)
}

// splice replaces the content between the gen-api markers with the freshly
// generated API reference.
func splice(guide []byte) []byte {
	start := bytes.Index(guide, []byte(startMarker))
	end := bytes.Index(guide, []byte(endMarker))
	if start < 0 || end < 0 || end <= start {
		fmt.Fprintln(os.Stderr, "markers not found (expected in docs/USER_GUIDE.md)")
		os.Exit(1)
	}

	head := append([]byte(nil), guide[:start+len(startMarker)]...)
	tail := append([]byte(nil), guide[end:]...)

	var buf bytes.Buffer
	buf.Write(head)
	buf.WriteString("\n\n")
	buf.WriteString(renderReference())
	buf.WriteString("\n")
	buf.Write(tail)
	return buf.Bytes()
}

// renderReference produces the api-reference block for embedding: apiref.Render
// with its H1 title and auto-gen note dropped, and every heading demoted one
// level so the sections nest under the guide's chapter heading.
func renderReference() string {
	ref := apiref.Render()

	// Strip everything up to and including the leading title + note.
	lines := strings.Split(ref, "\n")
	firstSection := 0
	for i, line := range lines {
		if strings.HasPrefix(line, "## ") {
			firstSection = i
			break
		}
	}
	lines = lines[firstSection:]

	for i, line := range lines {
		if strings.HasPrefix(line, "### ") {
			lines[i] = "#### " + strings.TrimPrefix(line, "### ")
		} else if strings.HasPrefix(line, "## ") {
			lines[i] = "### " + strings.TrimPrefix(line, "## ")
		}
	}

	return strings.Join(lines, "\n")
}

func findRepoRoot() string {
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	fmt.Fprintln(os.Stderr, "could not locate repo root (go.mod not found above cwd)")
	os.Exit(1)
	return ""
}