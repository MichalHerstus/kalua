package host

import (
	"testing"
)

func TestExitCode_IO(t *testing.T) {
	cfg := RunConfig{
		ScriptPath: "/nonexistent/path.lua",
		Verbose:    false,
	}
	code := Run(cfg)
	if code != ExitIOError {
		t.Errorf("Run = %d, want %d", code, ExitIOError)
	}
}
