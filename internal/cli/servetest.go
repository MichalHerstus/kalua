package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"kalua/internal/host"
	kaluaserver "kalua/internal/server"
)

// serveTestOptions carries the serve --test flags into the smoke driver.
type serveTestOptions struct {
	ScriptPath    string
	Host          string
	Workers       int
	Mode          string
	DBs           []string
	Args          []string
	AllowFS       []string
	Verbose       bool
	JSON          bool
	HTTP          []string   // "METHOD /path" entries
	ExpectStatus  int        // applied to each --http probe (0 → 200)
	ExpectJSON    string     // JSON object with key/value assertions
	ExpectContain string     // substring assertion on probe bodies
	WSEcho        string     // payload; any reply proves the ws text round-trip
	TCPEcho       string     // payload; any reply proves the tcp text round-trip
}

// runServeTest boots the app with serve semantics against ephemeral sockets,
// runs the HTTP/WS/TCP probes, and returns the exit code (0 = all pass).
func runServeTest(opts serveTestOptions) int {
	cfg := kaluaserver.Config{
		Host:        opts.Host,
		Port:        0, // ephemeral; addresses discovered by the smoke test
		Workers:     opts.Workers,
		Mode:        opts.Mode,
		ScriptPath:  opts.ScriptPath,
		DBs:         opts.DBs,
		Args:        opts.Args,
		AllowFS:     opts.AllowFS,
		MaxFileSize: 0,
		Verbose:     opts.Verbose,
	}

	probes := buildHTTPProbes(opts)
	smokeOpts := kaluaserver.SmokeOptions{
		HTTPProbes: probes,
		WSSend:     opts.WSEcho,
		TCPSend:    opts.TCPEcho,
	}
	res := kaluaserver.SmokeTest(cfg, smokeOpts)

	if opts.JSON {
		writeJSON(res)
	} else {
		printSmokeResult(res)
	}
	if res.OK {
		return int(host.ExitOK)
	}
	return int(host.ExitError)
}

// buildHTTPProbes turns the --http / --expect-* flags into SmokeProbes.
func buildHTTPProbes(opts serveTestOptions) []kaluaserver.SmokeProbe {
	var probes []kaluaserver.SmokeProbe
	var wantJSON map[string]any
	if opts.ExpectJSON != "" {
		_ = json.Unmarshal([]byte(opts.ExpectJSON), &wantJSON)
	}
	for _, spec := range opts.HTTP {
		method, path := parseMethodPath(spec)
		probes = append(probes, kaluaserver.SmokeProbe{
			Method:      method,
			Path:        path,
			WantStatus:  opts.ExpectStatus,
			WantJSON:    wantJSON,
			WantContent: opts.ExpectContain,
		})
	}
	return probes
}

// parseMethodPath splits a "METHOD /path" spec, defaulting the method to GET
// when only "/path" is given.
func parseMethodPath(spec string) (method, path string) {
	spec = strings.TrimSpace(spec)
	fields := strings.Fields(spec)
	if len(fields) == 0 {
		return "GET", "/"
	}
	m := strings.ToUpper(fields[0])
	switch m {
	case "GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS":
		if len(fields) > 1 {
			return m, fields[1]
		}
		return m, "/"
	}
	return "GET", fields[0]
}

// printSmokeResult renders a human-friendly pass/fail summary.
func printSmokeResult(res kaluaserver.SmokeResult) {
	fail := func(lines ...string) {
		for _, l := range lines {
			fmt.Fprintln(os.Stderr, l)
		}
	}
	if res.OK {
		fmt.Printf("PASS (mode=%s)\n", res.Mode)
		return
	}
	fail("FAIL (mode=" + res.Mode + ")")
	for _, e := range res.Errors {
		fail("  - " + e)
	}
}