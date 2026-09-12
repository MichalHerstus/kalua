package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"kalua/internal/ai"
	"kalua/internal/checker"
	"kalua/internal/host"
)

// aiCmd handles the `KALUA ai` subcommand: generate, fix, or validate
// KALUA Lua scripts from natural language.
func aiCmd(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "ai: requires a subcommand (generate, fix, validate)")
		printAIHelp()
		return int(host.ExitUsage)
	}
	sub := args[0]
	switch sub {
	case "generate":
		return aiGenerate(args[1:])
	case "fix":
		return aiFix(args[1:])
	case "validate":
		return aiValidate(args[1:])
	case "help":
		printAIHelp()
		return int(host.ExitOK)
	default:
		fmt.Fprintf(os.Stderr, "ai: unknown subcommand %q\n\n", sub)
		printAIHelp()
		return int(host.ExitUsage)
	}
}

func printAIHelp() {
	fmt.Fprint(os.Stderr, `KALUA AI — natural language KALUA app builder

Usage: KALUA ai <subcommand> [args...]

Subcommands:
  generate "<request>" [-o app.lua] [flags]  Generate a KALUA app from natural language
  fix <app.lua> [flags]                         Fix an existing KALUA script
  validate <app.lua>                          Validate a KALUA script (syntax + unknown k.*)
  help                                        Show this help message

Flags (all subcommands):
  --provider string   LLM provider: lmstudio, openrouter (default: lmstudio)
  --model string      Model name (default from env)
  --base-url string   LLM base URL (default: http://localhost:1234/v1)
  --api-key-env string Env var for API key (default: KALUA_AI_API_KEY)
  --full-doc          Include full API doc in prompt (default: run-mode subset)

Environment:
  KALUA_AI_BASE_URL   LLM base URL (default: http://localhost:1234/v1)
  KALUA_AI_API_KEY    API key (optional for local models)
  KALUA_AI_MODEL      Model name (default: local-model)

Examples:
  KALUA ai generate "a form with name and email fields" -o myapp.lua
  KALUA ai fix myapp.lua
  KALUA ai validate myapp.lua
`)
}

func providerFromFlags(f *aiFlags) ai.ProviderConfig {
	// Base config comes from the KALUA_AI_* env vars (via ai.defaultConfig);
	// explicit --base-url/--model/--api-key-env flags override them.
	base := ai.EnvConfig()
	if f.BaseURL != "" {
		base.BaseURL = f.BaseURL
	}
	if f.Model != "" {
		base.Model = f.Model
	}
	if f.APIKeyEnv != "" {
		base.APIKey = os.Getenv(f.APIKeyEnv)
	}
	if f.Provider == "openrouter" && base.APIKey == "" {
		base.APIKey = os.Getenv("OPENAI_API_KEY")
	}
	return base
}

func setAIFlags(fs *flag.FlagSet, f *aiFlags) {
	fs.StringVar(&f.Provider, "provider", "lmstudio", "LLM provider: lmstudio, openrouter")
	fs.StringVar(&f.Model, "model", "", "Model name")
	fs.StringVar(&f.BaseURL, "base-url", "", "LLM base URL")
	fs.StringVar(&f.APIKeyEnv, "api-key-env", "KALUA_AI_API_KEY", "Env var for API key")
	fs.BoolVar(&f.FullDoc, "full-doc", false, "Include full API doc in prompt")
}

type aiFlags struct {
	Provider   string
	Model      string
	BaseURL    string
	APIKeyEnv  string
	FullDoc    bool
	Output     string
	ScriptPath string
}

func aiGenerate(args []string) int {
	f := &aiFlags{Provider: "lmstudio"}
	fs := flag.NewFlagSet("ai generate", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	setAIFlags(fs, f)
	fs.StringVar(&f.Output, "o", "app.lua", "Output file")
	fs.StringVar(&f.Output, "output", "app.lua", "Output file")
	if err := fs.Parse(args); err != nil {
		return int(host.ExitUsage)
	}
	request := fs.Arg(0)
	if request == "" {
		fmt.Fprintln(os.Stderr, "ai generate: requires a natural language request")
		return int(host.ExitUsage)
	}

	cfg := providerFromFlags(f)
	fmt.Fprintf(os.Stderr, "KALUA AI: generating from \"%s\" (provider: %s, model: %s)\n", request, cfg.BaseURL, cfg.Model)
	req := ai.GenerateRequest{Request: request, FullDoc: f.FullDoc}
	ctx, cancel := aiCtx()
	defer cancel()
	result, err := ai.Generate(ctx, cfg, req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ai: generation failed: %v\n", err)
		return int(host.ExitError)
	}
	if err := os.WriteFile(f.Output, []byte(result.Script), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "ai: cannot write %s: %v\n", f.Output, err)
		return int(host.ExitIOError)
	}
	fmt.Fprintf(os.Stderr, "KALUA AI: generated %s\n", f.Output)
	for _, l := range result.Logs {
		fmt.Fprintf(os.Stderr, "  • %s\n", l)
	}
	if errs := staticValidate(f.Output); len(errs) > 0 {
		fmt.Fprintf(os.Stderr, "ai: validation failed:\n")
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "  %s\n", e)
		}
		fmt.Fprintf(os.Stderr, "Use 'KALUA ai fix %s' to auto-fix.\n", f.Output)
		return int(host.ExitError)
	}
	fmt.Fprintln(os.Stderr, "KALUA AI: validation passed ✓")
	fmt.Fprintf(os.Stderr, "Run with: KALUA run %s\n", f.Output)
	return int(host.ExitOK)
}

func aiFix(args []string) int {
	f := &aiFlags{}
	fs := flag.NewFlagSet("ai fix", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	setAIFlags(fs, f)
	if err := fs.Parse(args); err != nil {
		return int(host.ExitUsage)
	}
	scriptPath := fs.Arg(0)
	if scriptPath == "" {
		fmt.Fprintln(os.Stderr, "ai fix: requires a script file")
		return int(host.ExitUsage)
	}

	cfg := providerFromFlags(f)
	src, err := os.ReadFile(scriptPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ai: cannot read %s: %v\n", scriptPath, err)
		if os.IsNotExist(err) {
			return int(host.ExitIOError)
		}
		return int(host.ExitError)
	}
	req := ai.GenerateRequest{Request: "Fix the validation errors in this KALUA script", Script: string(src), FullDoc: f.FullDoc}
	fmt.Fprintf(os.Stderr, "KALUA AI: fixing %s...\n", scriptPath)
	ctx, cancel := aiCtx()
	defer cancel()
	result, err := ai.Generate(ctx, cfg, req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ai: fix failed: %v\n", err)
		return int(host.ExitError)
	}
	if err := os.WriteFile(scriptPath, []byte(result.Script), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "ai: cannot write %s: %v\n", scriptPath, err)
		return int(host.ExitIOError)
	}
	fmt.Fprintf(os.Stderr, "KALUA AI: fixed %s\n", scriptPath)
	for _, l := range result.Logs {
		fmt.Fprintf(os.Stderr, "  • %s\n", l)
	}
	if errs := staticValidate(scriptPath); len(errs) > 0 {
		fmt.Fprintf(os.Stderr, "ai: validation still failing:\n")
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "  %s\n", e)
		}
		return int(host.ExitError)
	}
	fmt.Fprintln(os.Stderr, "KALUA AI: validation passed ✓")
	return int(host.ExitOK)
}

func aiValidate(args []string) int {
	fs := flag.NewFlagSet("ai validate", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return int(host.ExitUsage)
	}
	scriptPath := fs.Arg(0)
	if scriptPath == "" {
		fmt.Fprintln(os.Stderr, "ai validate: requires a script file")
		return int(host.ExitUsage)
	}
	errs := staticValidate(scriptPath)
	if len(errs) > 0 {
		fmt.Fprintf(os.Stderr, "Validation errors:\n")
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "  %s\n", e)
		}
		return int(host.ExitError)
	}
	fmt.Fprintln(os.Stderr, "Validation passed ✓ (static; use KALUA run for execution)")
	return int(host.ExitOK)
}

// staticValidate runs checker.Check on a file and returns errors.
func staticValidate(scriptPath string) []string {
	src, err := os.ReadFile(scriptPath)
	if err != nil {
		return []string{fmt.Sprintf("cannot read %s: %v", scriptPath, err)}
	}
	res := checker.Check(string(src), scriptPath)
	if len(res.Errors) == 0 {
		return nil
	}
	cleaned := extractLuaBlock(string(src))
	if cleaned != string(src) {
		res = checker.Check(cleaned, scriptPath)
		if len(res.Errors) > 0 {
			return res.Errors
		}
	}
	return res.Errors
}

func extractLuaBlock(text string) string {
	m := ai.LuaBlockRE.FindStringSubmatch(text)
	if len(m) < 2 {
		return text
	}
	return strings.TrimSpace(m[1])
}

func aiCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 10*time.Minute)

}
