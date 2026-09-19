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
	"time"

	"kalua/internal/bindings"
	"kalua/internal/builder"
	"kalua/internal/checker"
	"kalua/internal/config"
	"kalua/internal/host"
	"kalua/internal/lsp"
	"kalua/internal/mcp"
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
	case "test":
		return testCmd(args[1:])
	case "describe":
		return describeCmd(args[1:])
	case "new":
		return newCmd(args[1:])
	case "lsp":
		return lspCmd()
	case "serve":
		return serveCmd(args[1:])
	case "builder":
		return builderCmd(args[1:])
	case "ai":
		return aiCmd(args[1:])
	case "scenario":
		return scenarioCmd(args[1:])
	case "mcp":
		return mcpCmd()
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
  run       <app.lua> [flags]   Run app as web app (--watch hot-reloads on change)
  serve     <app.lua> [flags]   Run app as headless API server
  check     <app.lua> [flags]   Validate script; --format/-w/-l/-d format it gofmt-style
  test      <app.lua> [flags]   Headless test: check + format + run/serve smoke (auto-detected)
  scenario  <app.lua> [flags]   Run UI scenario test (--scenario file.json)
  describe  <app.lua> [flags]   Structural overview: entry, forms, k.* API usage (AST-derived)
  builder  <app.lua|form.json> Visual form builder (opens browser)
  ai       AI builder (generate, fix, validate scripts)
  mcp       Model Context Protocol server (stdio)
  version                     Print version

Run 'KALUA <command> -h' for command-specific flags.
`)
}

// loadConfig resolves and loads the KALUA.INI configuration. iniFlag comes from
// the command's --ini flag. An absent default file (./KALUA.INI / $KALUA_INI is
// not set) yields an empty config; a missing file that was explicitly requested
// (via --ini or $KALUA_INI) is an error.
func loadConfig(iniFlag string) (*config.File, error) {
	path := config.FindPath(iniFlag)
	if path == "" {
		return config.New(), nil
	}
	return config.Load(path)
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
		jsonOutput   = fs.Bool("json", false, "Emit the headless result as JSON (with --test)")
		replOnError  = fs.Bool("repl-on-error", false, "Drop into REPL on runtime error")
		debugMode    = fs.Bool("debug", false, "Enable EmmyLua debugger (Tier 2, not yet implemented)")
		watch        = fs.Bool("watch", false, "Reload the app automatically when the script changes on disk")
		iniFlag      = fs.String("ini", "", "Path to KALUA.INI (default ./KALUA.INI or $KALUA_INI)")
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

	// KALUA.INI values fill in flags that were not set on the command line
	// (precedence: CLI flags > KALUA.INI > env vars > defaults).
	ini, err := loadConfig(*iniFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "run: %v\n", err)
		return int(host.ExitIOError)
	}
	if err := ini.ApplyFlags(fs, "run", map[string][]string{"v": {"verbose"}}); err != nil {
		fmt.Fprintln(os.Stderr, "run:", err)
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
		if *jsonOutput {
			return runTestJSON(cfg)
		}
		return int(host.Run(cfg))
	}

	// INT/TERM stop the server cleanly; SIGHUP triggers a manual hot reload.
	// The cancelable context also stops the --watch file watcher on shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Pre-register named --db handles (db="NAME" in scripts, table/looper rows).
	if code := registerNamedDBs(dbFlag.values); code != int(host.ExitOK) {
		return code
	}

	server := web.NewServer("127.0.0.1", *port, *sessionLimit,
		bindings.Options{AllowFS: allowFSFlag.values, Verbose: *verbose}, host.NewLogger(*verbose))

	// SIGHUP triggers a hot reload (same path as the --watch file watcher).
	hup := make(chan os.Signal, 1)
	signal.Notify(hup, syscall.SIGHUP)
	defer signal.Stop(hup)

	// Open browser with script parameter
	if !*noBrowser {
		url := fmt.Sprintf("http://127.0.0.1:%d/?script=%s", server.Port(), script)
		_ = openBrowser(url)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- server.Run(ctx, script) }()

	// Poll the app script and hot-reload on real changes (--watch). The
	// server's Reload() dedupes by content hash, so a 400ms poll is both cheap
	// and exact; reload errors are logged to stderr only when they change.
	watchErrSeen := ""
	if *watch {
		go func() {
			t := time.NewTicker(400 * time.Millisecond)
			defer t.Stop()
			for {
				select {
				case <-t.C:
					if err := server.Reload(); err != nil {
						msg := err.Error()
						if msg != watchErrSeen {
							fmt.Fprintf(os.Stderr, "%v\n", err)
							watchErrSeen = msg
						}
					} else {
						watchErrSeen = ""
					}
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	for {
		select {
		case err := <-runDone:
			if err != nil {
				fmt.Fprintf(os.Stderr, "server error: %v\n", err)
				return int(host.ExitError)
			}
			return int(host.ExitOK)
		case <-hup:
			if err := server.Reload(); err != nil {
				fmt.Fprintf(os.Stderr, "reload error: %v\n", err)
			}
		}
	}
}

func checkCmd(args []string) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var (
		verbose  = fs.Bool("v", false, "Verbose logging")
		iniFlag  = fs.String("ini", "", "Path to KALUA.INI (default ./KALUA.INI or $KALUA_INI)")
		formatTo = fs.Bool("format", false, "Print formatted source to stdout (exit 0 even when changed)")
		writeOut = fs.Bool("w", false, "Write formatted source back in place (permissions preserved)")
		listOnly = fs.Bool("l", false, "List files whose formatting differs (exit 1 if any)")
		diffOut  = fs.Bool("d", false, "Print a 0-context unified diff of the formatting changes (exit 1 if any)")
		jsonOut  = fs.Bool("json", false, "Emit the check result as JSON (issues carry line/col)")
	)
	if err := fs.Parse(args); err != nil {
		return int(host.ExitUsage)
	}
	rest := fs.Args()
	if len(rest) == 0 {
		fmt.Fprintln(os.Stderr, "check: requires a script argument")
		return int(host.ExitUsage)
	}
	// Two-pass parse so trailing flags after the first script work
	// (e.g. `check app.lua -w`); multi-file runs are gofmt-style flags-first.
	if err := fs.Parse(rest[1:]); err != nil {
		return int(host.ExitUsage)
	}
	scripts := append([]string{rest[0]}, fs.Args()...)

	ini, err := loadConfig(*iniFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "check: %v\n", err)
		return int(host.ExitIOError)
	}
	if err := ini.ApplyFlags(fs, "check", map[string][]string{"v": {"verbose"}}); err != nil {
		fmt.Fprintln(os.Stderr, "check:", err)
		return int(host.ExitUsage)
	}

	// Format modes (gofmt-style, -w wins over -d over -l over --format).
	var fm formatMode
	switch {
	case *writeOut:
		fm = formatWrite
	case *diffOut:
		fm = formatDiff
	case *listOnly:
		fm = formatList
	case *formatTo:
		fm = formatStdout
	}
	if fm != formatNone {
		return runFormat(fm, scripts, *jsonOut)
	}

	if len(scripts) != 1 {
		fmt.Fprintln(os.Stderr, "check: requires exactly one script argument")
		return int(host.ExitUsage)
	}
	cfg := host.RunConfig{
		ScriptPath: scripts[0],
		Verbose:    *verbose,
	}
	// check reuses RunConfig but only does static check; we just call checker directly
	return runCheck(cfg, *jsonOut)
}

func runCheck(cfg host.RunConfig, jsonOut bool) int {
	src, err := os.ReadFile(cfg.ScriptPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot read %s: %v\n", cfg.ScriptPath, err)
		if os.IsNotExist(err) || os.IsPermission(err) {
			return int(host.ExitIOError)
		}
		return int(host.ExitError)
	}
	res := checker.Check(string(src), cfg.ScriptPath)
	if jsonOut {
		writeJSON(checkResult{
			OK:     len(res.Errors) == 0,
			Files:  1,
			Issues: issuesJSON(cfg.ScriptPath, res.Issues),
		})
		if len(res.Errors) > 0 {
			return int(host.ExitError)
		}
		return int(host.ExitOK)
	}
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
	var (
		templateName = fs.String("template", "", "Scaffold template: run-form (default), run-crud, serve-http, serve-ws, serve-tcp, serve-all")
		jsonOut      = fs.Bool("json", false, "Emit the creation result as JSON")
	)
	// Two-pass parse so flags may appear before or after the name
	// (e.g. `new app.lua --template serve-all`).
	name, err := parseArgsScript(fs, args)
	if err != nil {
		return int(host.ExitUsage)
	}
	if name == "" {
		fmt.Fprintln(os.Stderr, "new: requires exactly one name argument")
		return int(host.ExitUsage)
	}
	path, err := resolveScriptName(name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "new: %v\n", err)
		return int(host.ExitError)
	}
	body, err := selectTemplate(*templateName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "new: %v\n", err)
		return int(host.ExitUsage)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "cannot write %s: %v\n", path, err)
		return int(host.ExitIOError)
	}
	if *jsonOut {
		writeJSON(checkResult{OK: true, Files: 1, Message: "Created " + path})
	} else {
		fmt.Printf("Created %s\n", path)
	}
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

func mcpCmd() int {
	if err := mcp.ServeForTest(stdioConn{in: os.Stdin, out: os.Stdout}); err != nil {
		fmt.Fprintf(os.Stderr, "mcp error: %v\n", err)
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
		fs.Bool("test", false, "Run once as a headless smoke test (ephemeral port, PASS/FAIL)")
		fs.Bool("json", false, "Emit the smoke-test result as JSON (with --test)")
		fs.String("ini", "", "Path to KALUA.INI (default ./KALUA.INI or $KALUA_INI)")
		fs.Var(&multiFlag{}, "http", "HTTP probe 'METHOD /path' against handle_http (repeatable, with --test)")
		fs.Int("expect-status", 0, "Expected HTTP status for --http probes (default 200)")
		fs.String("expect-body-json", "", "Object of key/value assertions on --http probe bodies")
		fs.String("expect-contains", "", "Substring assertion on --http probe bodies")
		fs.String("ws-echo", "", "Payload sent to handle_ws; any reply passes the ws probe")
		fs.String("tcp-echo", "", "Payload sent to handle_tcp; any reply passes the tcp probe")
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
		iniFlag     = fs.String("ini", "", "Path to KALUA.INI (default ./KALUA.INI or $KALUA_INI)")
		testMode    = fs.Bool("test", false, "Run once as a headless smoke test (ephemeral port, PASS/FAIL)")
		jsonOutput  = fs.Bool("json", false, "Emit the smoke-test result as JSON (with --test)")
		expectStatus = fs.Int("expect-status", 0, "Expected HTTP status for --http probes (default 200)")
		expectBodyJSON = fs.String("expect-body-json", "", "Object of key/value assertions on --http probe bodies")
		expectContain  = fs.String("expect-contains", "", "Substring assertion on --http probe bodies")
		wsEcho     = fs.String("ws-echo", "", "Payload sent to handle_ws; any reply passes the ws probe")
		tcpEcho    = fs.String("tcp-echo", "", "Payload sent to handle_tcp; any reply passes the tcp probe")
		httpProbes = multiFlag{}
		dbFlag      = multiFlag{}
		argFlag     = multiFlag{}
		allowFSFlag = multiFlag{}
	)
	fs.Var(&httpProbes, "http", "HTTP probe 'METHOD /path' against handle_http (repeatable, with --test)")
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

	ini, err := loadConfig(*iniFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "serve: %v\n", err)
		return int(host.ExitIOError)
	}
	if err := ini.ApplyFlags(fs, "serve", map[string][]string{"v": {"verbose"}}); err != nil {
		fmt.Fprintln(os.Stderr, "serve:", err)
		return int(host.ExitUsage)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Pre-register named --db handles for the workers.
	if code := registerNamedDBs(dbFlag.values); code != int(host.ExitOK) {
		return code
	}

	if *testMode {
		if *debugMode || *debugWorker {
			fmt.Fprintln(os.Stderr, "warning: --debug/--debug-worker (EmmyLua debugger) not yet implemented")
		}
		return runServeTest(serveTestOptions{
			ScriptPath:    script,
			Host:          *hostFlag,
			Workers:       *workers,
			Mode:          *mode,
			DBs:           dbFlag.values,
			Args:          argFlag.values,
			AllowFS:       allowFSFlag.values,
			Verbose:       *verbose,
			JSON:          *jsonOutput,
			HTTP:          httpProbes.values,
			ExpectStatus:  *expectStatus,
			ExpectJSON:    *expectBodyJSON,
			ExpectContain: *expectContain,
			WSEcho:        *wsEcho,
			TCPEcho:       *tcpEcho,
		})
	}

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

// builderCmd starts the visual form builder server for a .lua or .json form
// file. The file need not exist yet: an empty form is served and Save creates
// it (Lua files are written as generated source, JSON files as documents).
func builderCmd(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "builder: requires a form file argument (app.lua or form.json)")
		return int(host.ExitUsage)
	}
	if args[0] == "-h" || args[0] == "--help" {
		fmt.Fprintln(os.Stderr, "Usage: KALUA builder <app.lua|form.json> [--host 127.0.0.1] [--port 9001] [-n] [--model M] [--base-url U] [--api-key-env V] [--ini PATH]")
		return int(host.ExitOK)
	}

	fs := flag.NewFlagSet("builder", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var (
		hostFlag  = fs.String("host", "127.0.0.1", "Host to bind to")
		port      = fs.Int("port", 9001, "HTTP port")
		noBrowser = fs.Bool("no-browser", false, "Do not open browser")
		aiModel   = fs.String("model", "", "AI model (overrides KALUA_AI_MODEL)")
		aiBaseURL = fs.String("base-url", "", "AI base URL (overrides KALUA_AI_BASE_URL)")
		aiKeyEnv  = fs.String("api-key-env", "KALUA_AI_API_KEY", "Env var holding the AI API key")
		iniFlag   = fs.String("ini", "", "Path to KALUA.INI (default ./KALUA.INI or $KALUA_INI)")
		dbFlag    = multiFlag{}
	)
	fs.Var(&dbFlag, "db", "Pre-register DB connection for live previews: NAME=DSN (repeatable)")
	fs.Var(&dbFlag, "d", "Shorthand for --db")
	fs.IntVar(port, "p", 9001, "Shorthand for --port")
	fs.BoolVar(noBrowser, "n", false, "Shorthand for --no-browser")

	// Parse flags before AND after the positional file.
	file, err := parseArgsScript(fs, args)
	if err != nil {
		return int(host.ExitUsage)
	}
	if file == "" {
		fmt.Fprintln(os.Stderr, "builder: requires a form file argument")
		return int(host.ExitUsage)
	}

	ini, err := loadConfig(*iniFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "builder: %v\n", err)
		return int(host.ExitIOError)
	}
	if err := ini.ApplyFlags(fs, "builder", nil); err != nil {
		fmt.Fprintln(os.Stderr, "builder:", err)
		return int(host.ExitUsage)
	}

	// Pre-register named --db handles so the builder can run live preview
	// queries against them (/api/db, /api/db/query).
	if code := registerNamedDBs(dbFlag.values); code != int(host.ExitOK) {
		return code
	}

	srv, err := builder.New(file, *hostFlag, *port)
	if err != nil {
		fmt.Fprintf(os.Stderr, "builder error: %v\n", err)
		return int(host.ExitError)
	}
	// CLI flags (then KALUA.INI, then the KALUA_AI_* env vars) decide the LLM
	// connection. --api-key-env applies only when passed explicitly, so an
	// INI/OPENAI fallback key is not clobbered.
	var keyOverride string
	fs.Visit(func(fl *flag.Flag) {
		if fl.Name == "api-key-env" {
			keyOverride = os.Getenv(*aiKeyEnv)
		}
	})
	cfg := resolveAI(ini, aiOptions{BaseURL: *aiBaseURL, Model: *aiModel, APIKey: keyOverride})
	srv.SetAI(cfg)
	fmt.Fprintf(os.Stderr, "KALUA AI: %s (model %s)\n", cfg.BaseURL, cfg.Model)
	defer srv.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if !*noBrowser {
		_ = openBrowser(srv.URL())
	}
	fmt.Fprintf(os.Stderr, "KALUA Form Builder: %s\n", srv.URL())
	srv.Run(ctx)
	return int(host.ExitOK)
}

// registerNamedDBs pre-registers each --db NAME=DSN spec (the dbFlag values
// are parsed into configs elsewhere). Returns the exit code on failure.
func registerNamedDBs(dbs []string) int {
	if err := bindings.RegisterNamedDBPairs(dbs); err != nil {
		fmt.Fprintf(os.Stderr, "--db: %v\n", err)
		return int(host.ExitError)
	}
	return int(host.ExitOK)
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
	fs.Bool("watch", false, "Reload the app automatically when the script changes on disk")
	fs.String("ini", "", "Path to KALUA.INI (default ./KALUA.INI or $KALUA_INI)")
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
