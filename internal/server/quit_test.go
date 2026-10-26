//go:build !wasm

package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"kalua/internal/host"
)

// TestKQuitStopsServer verifies Phase 0.5: a script calling k.quit() stops the
// whole serve server cleanly (Run returns nil) rather than merely flagging the
// single worker.
func TestKQuitStopsServer(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "app.lua")
	os.WriteFile(script, []byte(`function main() end
function handle_http(req) return "ok" end
`), 0o644)

	cfg := Config{
		Host:       "127.0.0.1",
		Port:       0,
		Workers:    2,
		Mode:       "http",
		ScriptPath: script,
		Logger:     host.NewLogger(false),
	}
	s := NewServer(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan error, 1)
	go func() { runDone <- s.Run(ctx) }()

	select {
	case <-s.Ready():
	case <-time.After(8 * time.Second):
		t.Fatal("server not ready")
	}

	// Find a worker and trigger k.quit through its callbacks: the quit func is
	// wired into Options as s.Stop, so invoking it must end Run. The test holds
	// the worker slot (as a real in-flight handler would) so shutdown's waitIdle
	// blocks until the Lua frame returns — mirroring production timing.
	s.workerMu.RLock()
	workers := append([]*Worker{}, s.workers...)
	s.workerMu.RUnlock()
	if len(workers) == 0 {
		t.Fatal("no workers")
	}
	w := workers[0]
	w.acquireSlot()
	if err := w.env.L.DoString(`k.quit()`); err != nil {
		t.Fatalf("k.quit() failed: %v", err)
	}
	w.freeSlot()

	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("server did not stop after k.quit()")
	}
	if !s.ShutdownRan() {
		t.Error("shutdown() callback did not run")
	}
}
