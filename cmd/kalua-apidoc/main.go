package main

import (
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
	)
	flag.Parse()

	generated := apiref.Render()

	if *check {
		repoRoot := findRepoRoot()
		apiPath := filepath.Join(repoRoot, ".opencode", "skills", "kalua-api", "api.md")

		committed, err := os.ReadFile(apiPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading committed api.md: %v\n", err)
			os.Exit(1)
		}

		if string(committed) != generated {
			fmt.Fprintf(os.Stderr, "api.md drift detected! Run 'make gen-api' to update.\n")
			os.Exit(1)
		}
		fmt.Println("api.md is in sync")
		return
	}

	if *outputPath != "" {
		if err := os.WriteFile(*outputPath, []byte(generated), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing output: %v\n", err)
			os.Exit(1)
		}
		return
	}

	fmt.Print(generated)
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