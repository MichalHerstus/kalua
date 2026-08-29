package host

import (
	"errors"
	"io"
	"os"

	"github.com/yuin/gopher-lua"

	"kalua/internal/bindings"
	"kalua/internal/checker"
	"kalua/internal/vm"
)

// RunConfig holds the parameters for a run invocation.
type RunConfig struct {
	ScriptPath  string
	Args        []string // seeds ARGS global
	DBs         []string // --db NAME=DSN (parsed elsewhere)
	AllowFS     []string // --allow-fs paths
	MaxFileSize int64    // cap for k.file_load/k.json_load (0 = default 16 MiB)
	Verbose     bool
	Logger      *Logger
	Out         io.Writer // for k.print output
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
	})

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
	if err := app.Run(mainLFn); err != nil {
		// Classification: path errors → ExitIOError, else ExitError
		var perr *os.PathError
		if errors.As(err, &perr) {
			log.Errorf("%v", err)
			return ExitIOError
		}
		log.Errorf("%v", err)
		return ExitError
	}
	return ExitOK
}
