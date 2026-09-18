package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"kalua/internal/checker"
	"kalua/internal/host"
)

// checkIssue mirrors checker.Issue for JSON output.
type checkIssue struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Col     int    `json:"col"`
	Message string `json:"message"`
}

func issuesJSON(name string, issues []checker.Issue) []checkIssue {
	out := make([]checkIssue, 0, len(issues))
	for _, iss := range issues {
		out = append(out, checkIssue{File: name, Line: iss.Line, Col: iss.Col, Message: iss.Message})
	}
	return out
}

// checkResult is the machine-readable `check` output (--json).
type checkResult struct {
	OK      bool         `json:"ok"`
	Files   int          `json:"files"`
	Issues  []checkIssue `json:"issues,omitempty"`
	Message string       `json:"message,omitempty"`
}

// formatResult is the machine-readable `check -l/-w/-d` output (--json).
type formatResult struct {
	OK      bool   `json:"ok"`
	Files   int    `json:"files"`
	Changed []string `json:"changed,omitempty"` // names (with diffs appended for -d)
	Error   string `json:"error,omitempty"`
}

// runTestResult is the machine-readable `run --test --json` output.
type runTestResult struct {
	OK     bool         `json:"ok"`
	Phase  string       `json:"phase"` // "check" | "run"
	Issues []checkIssue `json:"issues,omitempty"`
	Output string       `json:"output,omitempty"`
	Error  string       `json:"error,omitempty"`
}

// writeJSON marshals v to stdout (indented) and returns the exit code. A
// marshaling error is genuinely internal and reported to stderr.
func writeJSON(v any) int {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "json output error: %v\n", err)
		return int(host.ExitError)
	}
	os.Stdout.Write(b)
	os.Stdout.Write([]byte("\n"))
	return int(host.ExitOK)
}

// runTestJSON runs `run --test --json`: static check first (structured
// issues), then a headless run whose stdout/k.print output and logger errors
// are captured into the JSON result instead of the terminal.
func runTestJSON(cfg host.RunConfig) int {
	src, err := os.ReadFile(cfg.ScriptPath)
	if err != nil {
		writeJSON(runTestResult{OK: false, Phase: "read", Error: err.Error()})
		return int(host.ExitIOError)
	}
	res := checker.Check(string(src), cfg.ScriptPath)
	if len(res.Errors) > 0 {
		writeJSON(runTestResult{
			OK:     false,
			Phase:  "check",
			Issues: issuesJSON(cfg.ScriptPath, res.Issues),
		})
		return int(host.ExitError)
	}

	var out, errOut bytes.Buffer
	cfg.Out = &out
	cfg.Logger = host.NewLoggerWriter(&out, &errOut, cfg.Verbose)
	code := host.Run(cfg)
	writeJSON(runTestResult{
		OK:     code == host.ExitOK,
		Phase:  "run",
		Output: out.String(),
		Error:  errOut.String(),
	})
	return int(code)
}