package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"

	"kalua/internal/bindings"
	"kalua/internal/checker"
	"kalua/internal/host"
	"kalua/internal/lsp"
	"kalua/internal/server"
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
	case "lsp":
		return lspCmd()
	case "serve":
		return serveCmd(args[1:])
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
  serve  <app.lua> [flags]   Run app as headless API server
  check  <app.lua>           Validate script (syntax, unknown k.*, main)
  new    <name>              Scaffold a minimal app.lua
  lsp    Language server over stdio (completion, hover, definitions)
  version                    Print version

Run 'KALUA <command> -h' for command-specific flags.
`)
}

func runCmd(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "run: requires a script argument")
		return int(host.ExitUsage)
	}
	// Handle -h/--help (also when it appears as a flag anywhere).
	if hasHelpFlag(args) {
		fs := flag.NewFlagSet("run", flag.ContinueOnError)
		fs.SetOutput(os.Stdout)
		addRunFlags(fs)
		fs.PrintDefaults()
		return int(host.ExitOK)
	}

	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		port         = fs.Int("port", 9000, "HTTP port (default 9000)")
		noBrowser    = fs.Bool("no-browser", false, "Do not open browser")
		sessionLimit = fs.Int("session-limit", 8, "Max concurrent browser tabs")
		verbose      = fs.Bool("v", false, "Verbose logging")
		testMode     = fs.Bool("test", false, "Run in test mode (headless, no server)")
		replOnError  = fs.Bool("repl-on-error", false, "Drop into REPL on runtime error")
		debugMode    = fs.Bool("debug", false, "Enable EmmyLua debugger (Tier 2, not yet implemented)")
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
	fs.IntVar(port, "p", 9000, "Shorthand for --port")
	fs.BoolVar(noBrowser, "n", false, "Shorthand for --no-browser")
	fs.IntVar(sessionLimit, "l", 8, "Shorthand for --session-limit")

	// Parse flags before AND after the positional script.
	script, err := parseArgsScript(fs, args)
	if err != nil {
		return int(host.ExitUsage)
	}
	if script == "" {
		fmt.Fprintln(os.Stderr, "run: requires a script argument")
		return int(host.ExitUsage)
	}

	if *testMode {
		// Run in headless mode for tests
		cfg := host.RunConfig{
			ScriptPath:  script,
			Args:        argFlag.values,
			DBs:         dbFlag.values,
			AllowFS:     allowFSFlag.values,
			Verbose:     *verbose,
			ReplOnError: *replOnError,
		}
		if *debugMode {
			fmt.Fprintln(os.Stderr, "warning: --debug (EmmyLua debugger) not yet implemented")
		}
		return int(host.Run(cfg))
	}

	ctx := context.Background()
	server := web.NewServer("127.0.0.1", *port, *sessionLimit,
		bindings.Options{AllowFS: allowFSFlag.values, Verbose: *verbose}, host.NewLogger(*verbose))

	// Open browser with script parameter
	if !*noBrowser {
		url := fmt.Sprintf("http://127.0.0.1:%d/?script=%s", server.Port(), script)
		_ = openBrowser(url)
	}

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

// stdioConn adapts stdin/stdout into a single ReadWriteCloser for the LSP
// stream (requests arrive on stdin, responses go out on stdout).
type stdioConn struct {
	in  io.Reader
	out io.Writer
}

func (s stdioConn) Read(p []byte) (int, error)  { return s.in.Read(p) }
func (s stdioConn) Write(p []byte) (int, error) { return s.out.Write(p) }
func (stdioConn) Close() error                  { return nil }

func lspCmd() int {
	if err := lsp.Serve(stdioConn{in: os.Stdin, out: os.Stdout}, "dev"); err != nil {
		fmt.Fprintf(os.Stderr, "lsp error: %v\n", err)
		return int(host.ExitError)
	}
	return int(host.ExitOK)
}

func serveCmd(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "serve: requires a script argument")
		return int(host.ExitUsage)
	}
	// Handle -h/--help for serve command
	if hasHelpFlag(args) {
		fs := flag.NewFlagSet("serve", flag.ContinueOnError)
		fs.SetOutput(os.Stdout)
		fs.String("host", "127.0.0.1", "Host to bind to")
		fs.Int("port", 8080, "HTTP port")
		fs.Int("workers", 4, "Number of worker processes")
		fs.String("mode", "http", "Server mode: http, ws, tcp, or comma-separated combination")
		fs.Bool("v", false, "Verbose logging")
		fs.Var(&multiFlag{}, "db", "Pre-register DB connection: NAME=DSN (repeatable)")
		fs.Var(&multiFlag{}, "d", "Shorthand for --db")
		fs.Var(&multiFlag{}, "arg", "Seed ARGS table: K=V (repeatable)")
		fs.Var(&multiFlag{}, "a", "Shorthand for --arg")
		fs.Var(&multiFlag{}, "allow-fs", "Allow filesystem access outside cwd (repeatable)")
		fs.Var(&multiFlag{}, "f", "Shorthand for --allow-fs")
		fs.PrintDefaults()
		return int(host.ExitOK)
	}

	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		hostFlag    = fs.String("host", "127.0.0.1", "Host to bind to")
		port        = fs.Int("port", 8080, "HTTP port")
		workers     = fs.Int("workers", 4, "Number of worker processes")
		mode        = fs.String("mode", "http", "Server mode: http, ws, tcp, or comma-separated combination")
		verbose     = fs.Bool("v", false, "Verbose logging")
		debugMode   = fs.Bool("debug", false, "Enable EmmyLua debugger per worker (Tier 2, not yet implemented)")
		debugWorker = fs.Bool("debug-worker", false, "Attach debugger to each worker (Tier 2, not yet implemented)")
		dbFlag      = multiFlag{}
		argFlag     = multiFlag{}
		allowFSFlag = multiFlag{}
	)
	fs.Var(&dbFlag, "db", "Pre-register DB connection: NAME=DSN (repeatable)")
	fs.Var(&dbFlag, "d", "Shorthand for --db")
	fs.Var(&argFlag, "arg", "Seed ARGS table: K=V (repeatable)")
	fs.Var(&argFlag, "a", "Shorthand for --arg")
	fs.Var(&allowFSFlag, "allow-fs", "Allow filesystem access outside cwd (repeatable)")
	fs.Var(&allowFSFlag, "f", "Shorthand for --allow-fs")
	fs.IntVar(port, "p", 8080, "Shorthand for --port")
	fs.IntVar(workers, "w", 4, "Shorthand for --workers")
	fs.StringVar(mode, "m", "http", "Shorthand for --mode")

	// Parse flags before AND after the positional script.
	script, perr := parseArgsScript(fs, args)
	if perr != nil {
		return int(host.ExitUsage)
	}
	if script == "" {
		fmt.Fprintln(os.Stderr, "serve: requires a script argument")
		return int(host.ExitUsage)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := server.Config{
		Host:        *hostFlag,
		Port:        *port,
		Workers:     *workers,
		Mode:        *mode,
		ScriptPath:  script,
		DBs:         dbFlag.values,
		Args:        argFlag.values,
		AllowFS:     allowFSFlag.values,
		MaxFileSize: 0,
		Verbose:     *verbose,
	}
	if *debugMode || *debugWorker {
		fmt.Fprintln(os.Stderr, "warning: --debug/--debug-worker (EmmyLua debugger) not yet implemented")
	}

	srv := server.NewServer(cfg)

	// SIGHUP triggers a hot reload (recompile + atomic worker swap). The
	// INT/TERM context above stops the server; HUP is consumed here.
	hup := make(chan os.Signal, 1)
	signal.Notify(hup, syscall.SIGHUP)
	defer signal.Stop(hup)

	runDone := make(chan error, 1)
	go func() { runDone <- srv.Run(ctx) }()

	for {
		select {
		case err := <-runDone:
			if err != nil {
				fmt.Fprintf(os.Stderr, "server error: %v\n", err)
				return int(host.ExitError)
			}
			return int(host.ExitOK)
		case <-hup:
			if err := srv.Reload(); err != nil {
				fmt.Fprintf(os.Stderr, "reload error: %v\n", err)
			}
		}
	}
}

func openBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start"}
	case "darwin":
		cmd = "open"
	default: // "linux", "freebsd", "openbsd", "netbsd"
		cmd = "xdg-open"
	}
	args = append(args, url)
	return exec.Command(cmd, args...).Start()
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

// addRunFlags registers the run-mode flags (used for --help output). The
// values are discarded; only the definitions/usage text matter.
func addRunFlags(fs *flag.FlagSet) {
	fs.Int("port", 9000, "HTTP port (default 9000)")
	fs.Bool("no-browser", false, "Do not open browser")
	fs.Bool("n", false, "Shorthand for --no-browser")
	fs.Int("session-limit", 8, "Max concurrent browser tabs")
	fs.Int("l", 8, "Shorthand for --session-limit")
	fs.Bool("v", false, "Verbose logging")
	fs.Bool("test", false, "Run in test mode (headless, no server)")
	fs.Bool("repl-on-error", false, "Drop into REPL on runtime error")
	fs.Bool("debug", false, "Enable EmmyLua debugger (Tier 2, not yet implemented)")
	fs.Var(&multiFlag{}, "db", "Pre-register DB connection: NAME=DSN (repeatable)")
	fs.Var(&multiFlag{}, "d", "Shorthand for --db")
	fs.Var(&multiFlag{}, "arg", "Seed ARGS table: K=V (repeatable)")
	fs.Var(&multiFlag{}, "a", "Shorthand for --arg")
	fs.Var(&multiFlag{}, "allow-fs", "Allow filesystem access outside cwd (repeatable)")
	fs.Var(&multiFlag{}, "f", "Shorthand for --allow-fs")
}

// hasHelpFlag reports whether -h or --help appears anywhere in args.
func hasHelpFlag(args []string) bool {
	for _, a := range args {
		if a == "-h" || a == "--help" {
			return true
		}
	}
	return false
}

// parseArgsScript parses args in two passes so flags may appear before or after
// the positional script. It returns the script (the first non-flag argument)
// and nil on success. The script-free case distinguishes "no script at all"
// from "script given" via the empty-string result.
func parseArgsScript(fs *flag.FlagSet, args []string) (string, error) {
	// Pass 1: consume any leading flags and find the first positional.
	if err := fs.Parse(args); err != nil {
		return "", err
	}
	rest := fs.Args()
	if len(rest) == 0 {
		return "", nil
	}
	script := rest[0]

	// Pass 2: any flags appearing after the script.
	if len(rest) > 1 {
		if err := fs.Parse(rest[1:]); err != nil {
			return "", err
		}
	}
	return script, nil
}
