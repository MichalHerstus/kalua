package host

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/yuin/gopher-lua"

	"kalua/internal/bindings"
	"kalua/internal/checker"
	"kalua/internal/vm"
)

// RunConfig holds the parameters for a run invocation.
type RunConfig struct {
	ScriptPath   string
	Args         []string // seeds ARGS global
	DBs          []string // --db NAME=DSN (parsed elsewhere)
	AllowFS      []string // --allow-fs paths
	MaxFileSize  int64    // cap for k.file_load/k.json_load (0 = default 16 MiB)
	Verbose      bool
	ReplOnError  bool     // --repl-on-error: drop into REPL on runtime error
	Logger       *Logger
	Out          io.Writer // for k.print output
}

// ExitCode maps errors to process exit codes per §4.
type ExitCode int

const (
	ExitOK      ExitCode = 0
	ExitError   ExitCode = 1 // script / runtime error
	ExitUsage   ExitCode = 2 // CLI usage error
	ExitIOError ExitCode = 3 // I/O error (file not found, permission)
)

// Run loads, validates, and executes a KALUA script. Returns the exit code.
func Run(cfg RunConfig) ExitCode {
	log := cfg.Logger
	if log == nil {
		log = NewLogger(cfg.Verbose)
	}
	if cfg.Out == nil {
		cfg.Out = os.Stdout
	}

	// 1. Read source
	src, err := os.ReadFile(cfg.ScriptPath)
	if err != nil {
		log.Errorf("cannot read %s: %v", cfg.ScriptPath, err)
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, os.ErrPermission) {
			return ExitIOError
		}
		return ExitError
	}

	// 2. Static check (syntax + unknown k.* + main)
	if res := checker.Check(string(src), cfg.ScriptPath); len(res.Errors) > 0 {
		for _, e := range res.Errors {
			log.Errorf("%s", e)
		}
		return ExitError
	}

	// 3. Sandbox VM
	L := vm.New()
	defer L.Close()

	// 4. Load and compile the chunk (which defines main)
	chunkFn, err := vm.LoadSource(L, cfg.ScriptPath, string(src))
	if err != nil {
		log.Errorf("%v", err)
		return ExitError
	}

	// 5. Bindings + ARGS (must be before running chunk so k.* exists)
	app := vm.NewApp(L)
	bindings.Setup(L, app, bindings.Options{
		Args:        cfg.Args,
		AllowFS:     cfg.AllowFS,
		MaxFileSize: cfg.MaxFileSize,
		Verbose:     cfg.Verbose,
	}, nil, log)

	// 6. Execute the chunk to define main() in globals
	if err := L.CallByParam(lua.P{Fn: chunkFn, NRet: 0, Protect: true}); err != nil {
		log.Errorf("%v", err)
		return ExitError
	}

	// 7. Get main function and run it
	mainFn := L.GetGlobal("main")
	if mainFn == lua.LNil {
		log.Errorf("main function not found after chunk execution")
		return ExitError
	}
	mainLFn, ok := mainFn.(*lua.LFunction)
	if !ok {
		log.Errorf("main is not a function")
		return ExitError
	}

	// 8. Run main
	var runErr error
	if cfg.ReplOnError {
		runErr = runMainWithXPCall(L, mainLFn)
	} else {
		runErr = app.Run(mainLFn)
	}
	if runErr != nil {
		// Classification: path errors → ExitIOError, else ExitError
		var perr *os.PathError
		if errors.As(runErr, &perr) {
			log.Errorf("%v", runErr)
			return ExitIOError
		}
		log.Errorf("%v", runErr)
		if cfg.Verbose {
			log.Errorf("%s", postMortemDump(L))
		}
		if cfg.ReplOnError {
			startRepl(cfg, L, runErr.Error())
		}
		return ExitError
	}
	return ExitOK
}

// postMortemDump builds a backtrace for a runtime error: every frame with
// source, line, function name and locals, plus upvalues. Used for post-mortem
// inspection (Tier 1 debugging) under --verbose.
func postMortemDump(L *lua.LState) string {
	var sb strings.Builder
	sb.WriteString("Lua stack trace:\n")
	level := 0
	for {
		dbg, ok := L.GetStack(level)
		if !ok {
			break
		}
		_, _ = L.GetInfo("nSlu", dbg, lua.LNil)
		fmt.Fprintf(&sb, "  #%d %s in %q (line %d)\n",
			level, dbg.Source, dbg.Name, dbg.CurrentLine)
		li := 1
		for {
			name, val := L.GetLocal(dbg, li)
			if name == "" {
				break
			}
			if !strings.HasPrefix(name, "(*temporary)") {
				fmt.Fprintf(&sb, "      local %s = %s\n", name, val.String())
			}
			li++
		}
		level++
	}
	return sb.String()
}

// runMainWithXPCall runs the main function wrapped in xpcall with debug.traceback.
// Returns the error (with full traceback) if any.
func runMainWithXPCall(L *lua.LState, mainLFn *lua.LFunction) error {
	// Create a wrapper that calls xpcall(main, debug.traceback)
	wrapper := L.NewFunction(func(L *lua.LState) int {
		// xpcall(mainLFn, debug.traceback)
		xpcall := L.GetGlobal("xpcall")
		traceback := L.GetGlobal("debug").(*lua.LTable).RawGetString("traceback")
		L.Push(xpcall)
		L.Push(mainLFn)
		L.Push(traceback)
		L.Call(2, 2)
		// xpcall returns (ok, result_or_error)
		ok := L.ToBool(-2)
		if !ok {
			// Re-raise the error with traceback
			L.Error(lua.LString(L.Get(-1).String()), 0)
			return 0
		}
		L.Push(L.Get(-1))
		return 1
	})
	// Run the wrapper via app
	app := vm.NewApp(L)
	return app.Run(wrapper)
}

// startRepl runs an interactive Lua REPL in the given LState.
func startRepl(cfg RunConfig, L *lua.LState, errMsg string) {
	fmt.Fprintf(cfg.Out, "\n=== Runtime error: %v ===\n", errMsg)
	fmt.Fprintf(cfg.Out, "Entering REPL. Type 'exit' or Ctrl+D to quit.\n")
	fmt.Fprintf(cfg.Out, "Available: k.*, K.*, debug.*, and all script globals.\n\n")

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Fprintf(cfg.Out, "kalua> ")
		line, err := reader.ReadString('\n')
		if err != nil {
			fmt.Fprintf(cfg.Out, "\n")
			break
		}
		line = strings.TrimSpace(line)
		if line == "exit" || line == "quit" {
			break
		}
		if line == "" {
			continue
		}
		// Handle expression lines: prefix with "return " for Lua REPL semantics
		// If line starts with "=", treat as expression (Lua convention)
		if strings.HasPrefix(line, "=") {
			line = "return " + strings.TrimSpace(line[1:])
		} else if !isStatement(line) {
			// Heuristic: if it looks like an expression, wrap in return
			line = "return " + line
		}
		// Compile and run the line
		if err := L.DoString(line); err != nil {
			fmt.Fprintf(cfg.Out, "Error: %v\n", err)
		} else {
			// Print any returned values
			top := L.GetTop()
			if top > 0 {
				for i := 1; i <= top; i++ {
					fmt.Fprintf(cfg.Out, "%v\t", L.Get(i))
				}
				fmt.Fprintf(cfg.Out, "\n")
				L.SetTop(0)
			}
		}
	}
}

// isStatement heuristically checks if a line is a Lua statement (assignment, call, control flow)
// vs an expression. Used to decide whether to wrap in "return ".
func isStatement(line string) bool {
	trimmed := strings.TrimSpace(line)
	// Empty or comment
	if trimmed == "" || strings.HasPrefix(trimmed, "--") {
		return true
	}
	// Keywords that start statements
	stmtPrefixes := []string{
		"local ", "function ", "if ", "for ", "while ", "repeat ",
		"return ", "break ", "goto ", "do ", "end ",
	}
	for _, p := range stmtPrefixes {
		if strings.HasPrefix(trimmed, p) {
			return true
		}
	}
	// Assignment pattern: name = or name, name = 
	if strings.Contains(trimmed, "=") && !strings.Contains(trimmed, "==") && !strings.Contains(trimmed, "~=") && !strings.Contains(trimmed, "<=") && !strings.Contains(trimmed, ">=") {
		parts := strings.SplitN(trimmed, "=", 2)
		left := strings.TrimSpace(parts[0])
		// Left side looks like variable(s): no function calls, no brackets
		if !strings.Contains(left, "(") && !strings.Contains(left, "[") && !strings.Contains(left, ".") {
			return true
		}
	}
	// Function call statement: name(...)
	if strings.Contains(trimmed, "(") && strings.HasSuffix(strings.TrimSpace(trimmed), ")") {
		return true
	}
	return false
}