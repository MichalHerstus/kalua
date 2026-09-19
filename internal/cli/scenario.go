package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"kalua/internal/host"
	"kalua/internal/scenario"
)

// scenarioCmd runs a UI scenario test against a run-mode app.
func scenarioCmd(args []string) int {
	fs := flag.NewFlagSet("scenario", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		scenarioPath = fs.String("scenario", "", "Path to scenario JSON file (required)")
		jsonOutput   = fs.Bool("json", false, "Emit the scenario result as JSON")
		verbose      = fs.Bool("v", false, "Verbose logging")
		iniFlag      = fs.String("ini", "", "Path to KALUA.INI (default ./KALUA.INI or $KALUA_INI)")
	)

	if err := fs.Parse(args); err != nil {
		return int(host.ExitUsage)
	}

	rest := fs.Args()
	if len(rest) == 0 {
		fmt.Fprintln(os.Stderr, "scenario: requires a script argument")
		return int(host.ExitUsage)
	}
	if err := fs.Parse(rest[1:]); err != nil {
		return int(host.ExitUsage)
	}
	script := rest[0]

	if *scenarioPath == "" {
		fmt.Fprintln(os.Stderr, "scenario: requires --scenario flag")
		return int(host.ExitUsage)
	}

	// Load INI config
	ini, err := loadConfig(*iniFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "scenario: %v\n", err)
		return int(host.ExitIOError)
	}
	if err := ini.ApplyFlags(fs, "scenario", map[string][]string{"v": {"verbose"}}); err != nil {
		fmt.Fprintln(os.Stderr, "scenario:", err)
		return int(host.ExitUsage)
	}

	// Load scenario
	scn, err := scenario.LoadScenario(*scenarioPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "scenario: cannot load %s: %v\n", *scenarioPath, err)
		return int(host.ExitIOError)
	}

	// Run scenario
	runner, err := scenario.NewRunner(script, *verbose)
	if err != nil {
		fmt.Fprintf(os.Stderr, "scenario: %v\n", err)
		return int(host.ExitError)
	}

	fmt.Fprintf(os.Stderr, "Running scenario: %s\n", scn.Name)
	start := time.Now()
	result := runner.Run(scn)
	result.Duration = time.Since(start)

	if *jsonOutput {
		b, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "json output error: %v\n", err)
			return int(host.ExitError)
		}
		os.Stdout.Write(b)
		os.Stdout.Write([]byte("\n"))
	} else {
		if result.OK {
			fmt.Printf("PASS scenario %s (%.2fs)\n", result.Name, result.Duration.Seconds())
		} else {
			fmt.Fprintf(os.Stderr, "FAIL scenario %s at step %d: %s\n", result.Name, result.Step, result.Error)
		}
	}

	if result.OK {
		return int(host.ExitOK)
	}
	return int(host.ExitError)
}