package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"kalua/internal/apiref"
)

func main() {
	var (
		outputPath = flag.String("o", "", "Output file path (default: stdout)")
		check      = flag.Bool("check", false, "Check if committed api.md matches generated output")
		quickPath  = flag.String("quickref", "", "Quickref output path (written alongside api.md)")
	)
	flag.Parse()

	generated := apiref.Render()

	if *check {
		root := findRepoRoot()
		pairs := map[string]string{
			"api.md":      filepath.Join(root, "_opencode", "skills", "kalua-api", "api.md"),
			"quickref.md": filepath.Join(root, "docs", "agentic", "quickref.md"),
		}
		generatedQuick := apiref.RenderQuick()
		var fails bytes.Buffer
		for label, path := range pairs {
			var want string
			if label == "quickref.md" {
				want = generatedQuick
			} else {
				want = generated
			}
			committed, err := os.ReadFile(path)
			if err != nil {
				fmt.Fprintf(&fails, "%s: read error: %v\n", label, err)
				continue
			}
			if string(committed) != want {
				fmt.Fprintf(&fails, "%s drift detected! Run 'make gen-api' to update.\n", label)
			}
		}
		if fails.Len() > 0 {
			os.Stderr.Write(fails.Bytes())
			os.Exit(1)
		}
		fmt.Println("api.md and quickref.md are in sync")
		return
	}

	if *outputPath != "" {
		if err := os.WriteFile(*outputPath, []byte(generated), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing output: %v\n", err)
			os.Exit(1)
		}
		if *quickPath != "" {
			if err := os.WriteFile(*quickPath, []byte(apiref.RenderQuick()), 0644); err != nil {
				fmt.Fprintf(os.Stderr, "Error writing quickref: %v\n", err)
				os.Exit(1)
			}
		}
		return
	}

	fmt.Print(generated)
	if *quickPath != "" {
		fmt.Println("(-quickref requires -o)")
	}
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
	return "."
}
