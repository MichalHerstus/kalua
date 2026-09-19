package cli

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"kalua/internal/checker"
	"kalua/internal/format"
	"kalua/internal/host"
	"kalua/internal/scenario"
	kaluaserver "kalua/internal/server"
)

// testResult is the unified machine-readable output of `KALUA test app.lua`.
type testResult struct {
	OK        bool                     `json:"ok"`
	Mode      string                   `json:"mode"`   // "run" | "serve"
	Formatted bool                     `json:"formatted"`
	Issues    []checkIssue             `json:"issues,omitempty"`
	Run       *runTestResult           `json:"run,omitempty"`
	Serve     *kaluaserver.SmokeResult `json:"serve,omitempty"`
	Error     string                   `json:"error,omitempty"`
}

func testCmd(args []string) int {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		jsonOutput    = fs.Bool("json", false, "Emit the test result as JSON")
		verbose       = fs.Bool("v", false, "Verbose logging")
		iniFlag       = fs.String("ini", "", "Path to KALUA.INI (default ./KALUA.INI or $KALUA_INI)")
		scenarioPath  = fs.String("scenario", "", "Path to scenario JSON file (run-mode only)")
		// Serve-mode probe flags (used when auto-detected mode is serve).
		httpProbes = multiFlag{}
		expectJSON = fs.String("expect-body-json", "", "JSON object with key/value assertions on HTTP probe bodies")
		expectStat = fs.Int("expect-status", 0, "Expected HTTP status for --http probes (default 200)")
		expectCont = fs.String("expect-contains", "", "Substring assertion on HTTP probe bodies")
		wsEcho     = fs.String("ws-echo", "", "Payload sent to handle_ws; any reply passes the ws probe")
		tcpEcho    = fs.String("tcp-echo", "", "Payload sent to handle_tcp; any reply passes the tcp probe")
		// Common flags.
		dbFlag      = multiFlag{}
		argFlag     = multiFlag{}
		allowFSFlag = multiFlag{}
	)
	fs.Var(&httpProbes, "http", "HTTP probe 'METHOD /path' against handle_http (repeatable)")
	fs.Var(&dbFlag, "db", "Pre-register DB connection: NAME=DSN (repeatable)")
	fs.Var(&dbFlag, "d", "Shorthand for --db")
	fs.Var(&argFlag, "arg", "Seed ARGS table: K=V (repeatable)")
	fs.Var(&argFlag, "a", "Shorthand for --arg")
	fs.Var(&allowFSFlag, "allow-fs", "Allow filesystem access outside cwd (repeatable)")
	fs.Var(&allowFSFlag, "f", "Shorthand for --allow-fs")

	// Parse flags before AND after the positional script.
	if err := fs.Parse(args); err != nil {
		return int(host.ExitUsage)
	}
	rest := fs.Args()
	if len(rest) == 0 {
		fmt.Fprintln(os.Stderr, "test: requires a script argument")
		return int(host.ExitUsage)
	}
	if err := fs.Parse(rest[1:]); err != nil {
		return int(host.ExitUsage)
	}
	script := rest[0]

	ini, err := loadConfig(*iniFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "test: %v\n", err)
		return int(host.ExitIOError)
	}
	if err := ini.ApplyFlags(fs, "test", map[string][]string{"v": {"verbose"}}); err != nil {
		fmt.Fprintln(os.Stderr, "test:", err)
		return int(host.ExitUsage)
	}

	// If --scenario is provided, run the scenario test (run-mode only)
	if *scenarioPath != "" {
		return runScenarioTest(script, *scenarioPath, *jsonOutput, *verbose)
	}

	src, err := os.ReadFile(script)
	if err != nil {
		fail(script, false, testResult{Error: err.Error()})
		if os.IsNotExist(err) || os.IsPermission(err) {
			return int(host.ExitIOError)
		}
		return int(host.ExitError)
	}

	// --- Phase 1: Static check ---
	cr := checker.Check(string(src), script)
	if len(cr.Errors) > 0 {
		fail(script, *jsonOutput, testResult{Issues: issuesJSON(script, cr.Issues)})
		return int(host.ExitError)
	}

	// --- Phase 2: Format check ---
	formatted, fmtErr := format.Format(src, script)
	isFormatted := fmtErr == nil && string(formatted) == string(src)

	// --- Phase 3: Headless test ---
	mode := checker.EntryMode(string(src), script)

	switch mode {
	case "serve":
		return runTestServe(script, mode, isFormatted, *jsonOutput, *verbose, dbFlag.values, argFlag.values, allowFSFlag.values, httpProbes.values, *expectStat, *expectJSON, *expectCont, *wsEcho, *tcpEcho)
	case "run":
		return runTestRun(script, mode, isFormatted, *jsonOutput, *verbose, dbFlag.values, argFlag.values, allowFSFlag.values)
	default:
		fail(script, *jsonOutput, testResult{Error: "no entry point found (main or handle_http/handle_ws/handle_tcp)"})
		return int(host.ExitError)
	}
}

// runTestServe executes the serve-mode smoke test and writes the aggregated result.
func runTestServe(script, mode string, formatted, jsonOut, verbose bool, dbs, args, allowFS, http []string, expectStat int, expectJSON, expectContain, wsEcho, tcpEcho string) int {
	cfg := kaluaserver.Config{
		Host:       "127.0.0.1",
		Port:       0,
		Workers:    1,
		Mode:       mode,
		ScriptPath: script,
		DBs:        dbs,
		Args:       args,
		AllowFS:    allowFS,
		Verbose:    verbose,
	}
	probes := buildHTTPProbes(serveTestOptions{
		HTTP:          http,
		ExpectStatus:  expectStat,
		ExpectJSON:    expectJSON,
		ExpectContain: expectContain,
	})
	smoke := kaluaserver.SmokeTest(cfg, kaluaserver.SmokeOptions{
		HTTPProbes: probes,
		WSSend:     wsEcho,
		TCPSend:    tcpEcho,
	})

	tr := testResult{OK: smoke.OK, Mode: mode, Formatted: formatted, Serve: &smoke}
	if !smoke.OK {
		fail(script, jsonOut, tr)
		return int(host.ExitError)
	}
	if jsonOut {
		writeJSON(tr)
	} else {
		fmt.Printf("PASS (mode=%s, formatted=%v)\n", mode, formatted)
	}
	return int(host.ExitOK)
}

// runTestRun executes the run-mode headless test and writes the aggregated result.
func runTestRun(script, mode string, formatted, jsonOut, verbose bool, dbs, args, allowFS []string) int {
	cfg := host.RunConfig{
		ScriptPath: script,
		DBs:        dbs,
		Args:       args,
		AllowFS:    allowFS,
		Verbose:    verbose,
	}
	var out, errOut bytes.Buffer
	cfg.Out = &out
	cfg.Logger = host.NewLoggerWriter(&out, &errOut, verbose)
	code := host.Run(cfg)
	ok := code == host.ExitOK
	rtr := runTestResult{
		OK:     ok,
		Phase:  "run",
		Output: out.String(),
		Error:  errOut.String(),
	}
	tr := testResult{OK: ok, Mode: mode, Formatted: formatted, Run: &rtr}
	if !ok {
		fail(script, jsonOut, tr)
		return int(host.ExitError)
	}
	if jsonOut {
		writeJSON(tr)
	} else {
		fmt.Printf("PASS (mode=%s, formatted=%v)\n", mode, formatted)
	}
	return int(host.ExitOK)
}

func fail(script string, jsonOut bool, tr testResult) {
	if jsonOut {
		b, _ := json.MarshalIndent(tr, "", "  ")
		os.Stdout.Write(b)
		os.Stdout.Write([]byte("\n"))
	} else {
		fmt.Fprintf(os.Stderr, "FAIL (mode=%s)\n", tr.Mode)
		if tr.Error != "" {
			fmt.Fprintf(os.Stderr, "  - %s\n", tr.Error)
		}
		for _, iss := range tr.Issues {
			fmt.Fprintf(os.Stderr, "  - %s:%d:%d: %s\n", iss.File, iss.Line, iss.Col, iss.Message)
		}
		if tr.Run != nil && tr.Run.Error != "" {
			fmt.Fprintf(os.Stderr, "  - %s\n", tr.Run.Error)
		}
		if tr.Serve != nil {
			for _, e := range tr.Serve.Errors {
				fmt.Fprintf(os.Stderr, "  - %s\n", e)
			}
		}
	}
}

// runScenarioTest runs a UI scenario test and returns the result in testResult format.
func runScenarioTest(script, scenarioPath string, jsonOut, verbose bool) int {
	scn, err := scenario.LoadScenario(scenarioPath)
	if err != nil {
		tr := testResult{OK: false, Mode: "run", Error: fmt.Sprintf("cannot load scenario: %v", err)}
		if jsonOut {
			writeJSON(tr)
		} else {
			fmt.Fprintf(os.Stderr, "FAIL: %v\n", err)
		}
		return int(host.ExitError)
	}

	runner, err := scenario.NewRunner(script, verbose)
	if err != nil {
		tr := testResult{OK: false, Mode: "run", Error: err.Error()}
		if jsonOut {
			writeJSON(tr)
		} else {
			fmt.Fprintf(os.Stderr, "FAIL: %v\n", err)
		}
		return int(host.ExitError)
	}

	result := runner.Run(scn)

	tr := testResult{
		OK:    result.OK,
		Mode:  "run",
		Error: result.Error,
		Run: &runTestResult{
			OK:     result.OK,
			Phase:  "scenario",
			Output: result.Output,
			Error:  result.Error,
		},
	}

	if !result.OK {
		fail(script, jsonOut, tr)
		return int(host.ExitError)
	}
	if jsonOut {
		writeJSON(tr)
	} else {
		fmt.Printf("PASS (mode=run, scenario=%s, formatted=false)\n", result.Name)
	}
	return int(host.ExitOK)
}
