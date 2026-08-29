package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"kalua/internal/checker"
	"kalua/internal/host"
	"kalua/internal/web"
)

// Run is the CLI entry point. Returns exit code for os.Exit.
func Run(args []string) int {
	if len(args) == 0 {
		printUsage()
		return int(host.ExitUsage)
	}

	cmd := args[0]
	switch cmd {
	case "run":
		return runCmd(args[1:])
	case "check":
		return checkCmd(args[1:])
	case "new":
		return newCmd(args[1:])
	case "version":
		fmt.Println("KALUA dev (phase 2)")
		return int(host.ExitOK)
	default:
		fmt.Fprintf(os.Stderr, "KALUA: unknown command %q\n\n", cmd)
		printUsage()
		return int(host.ExitUsage)
	}
}

func printUsage() {
	fmt.Fprint(os.Stderr, `KALUA — Kalipso-style web apps in embedded Lua

Usage: KALUA <command> [args...]

Commands:
  run    <app.lua> [flags]   Run app as web app (opens browser)
  check  <app.lua>           Validate script (syntax, unknown k.*, main)
  new    <name>              Scaffold a minimal app.lua
  version                    Print version

Run 'KALUA <command> -h' for command-specific flags.
`)
}

func runCmd(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "run: requires a script argument")
		return int(host.ExitUsage)
	}
	script := args[0]
	flagArgs := args[1:]

	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		port         = fs.Int("port", 0, "HTTP port (0 = ephemeral)")
		_            = fs.Bool("no-browser", false, "Do not open browser")
		sessionLimit = fs.Int("session-limit", 8, "Max concurrent browser tabs")
		verbose      = fs.Bool("v", false, "Verbose logging")
		testMode     = fs.Bool("test", false, "Run in test mode (headless, no server)")
		dbFlag       = multiFlag{}
		argFlag      = multiFlag{}
		allowFSFlag  = multiFlag{}
	)
	fs.Var(&dbFlag, "db", "Pre-register DB connection: NAME=DSN (repeatable)")
	fs.Var(&dbFlag, "d", "Shorthand for --db")
	fs.Var(&argFlag, "arg", "Seed ARGS table: K=V (repeatable)")
	fs.Var(&argFlag, "a", "Shorthand for --arg")
	fs.Var(&allowFSFlag, "allow-fs", "Allow filesystem access outside cwd (repeatable)")
	fs.Var(&allowFSFlag, "f", "Shorthand for --allow-fs")

	if err := fs.Parse(flagArgs); err != nil {
		return int(host.ExitUsage)
	}

	if *testMode {
		// Run in headless mode for tests
		cfg := host.RunConfig{
			ScriptPath: script,
			Args:       argFlag.values,
			DBs:        dbFlag.values,
			AllowFS:    allowFSFlag.values,
			Verbose:    *verbose,
		}
		return int(host.Run(cfg))
	}

	ctx := context.Background()
	server := web.NewServer("127.0.0.1", *port, *sessionLimit, host.NewLogger(*verbose))

	// TODO: open browser if not --no-browser

	if err := server.Run(ctx, script); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		return int(host.ExitError)
	}
	return int(host.ExitOK)
}

func checkCmd(args []string) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var verbose = fs.Bool("v", false, "Verbose logging")
	if err := fs.Parse(args); err != nil {
		return int(host.ExitUsage)
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "check: requires exactly one script argument")
		return int(host.ExitUsage)
	}
	script := fs.Arg(0)
	cfg := host.RunConfig{
		ScriptPath: script,
		Verbose:    *verbose,
	}
	// check reuses RunConfig but only does static check; we just call checker directly
	return runCheck(cfg)
}

func runCheck(cfg host.RunConfig) int {
	src, err := os.ReadFile(cfg.ScriptPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot read %s: %v\n", cfg.ScriptPath, err)
		if os.IsNotExist(err) || os.IsPermission(err) {
			return int(host.ExitIOError)
		}
		return int(host.ExitError)
	}
	res := checker.Check(string(src), cfg.ScriptPath)
	if len(res.Errors) > 0 {
		for _, e := range res.Errors {
			fmt.Fprintln(os.Stderr, e)
		}
		return int(host.ExitError)
	}
	fmt.Println("OK")
	return int(host.ExitOK)
}

func newCmd(args []string) int {
	fs := flag.NewFlagSet("new", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return int(host.ExitUsage)
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "new: requires exactly one name argument")
		return int(host.ExitUsage)
	}
	name := fs.Arg(0)
	path := name + ".lua"
	if _, err := os.Stat(path); err == nil {
		fmt.Fprintf(os.Stderr, "%s already exists\n", path)
		return int(host.ExitError)
	}
	if err := os.WriteFile(path, []byte(template), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "cannot write %s: %v\n", path, err)
		return int(host.ExitIOError)
	}
	fmt.Printf("Created %s\n", path)
	return int(host.ExitOK)
}

type multiFlag struct{ values []string }

func (m *multiFlag) String() string { return strings.Join(m.values, ",") }
func (m *multiFlag) Set(s string) error {
	m.values = append(m.values, s)
	return nil
}

const template = `-- minimal KALUA app
function main()
  local arg = ARGS[1]
  if arg == nil then arg = "KALUA" end
  k.print("Hello from " .. arg)
  k.sleep(100)
  k.quit()
end
`
