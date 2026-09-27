package server

import (
	"context"
	"net"
	"testing"
	"time"
)

// newReadyServer boots a plain HTTP server on an ephemeral port and returns it
// with a stop func, mirroring how callers drive Server directly.
func newReadyServer(t *testing.T, mode string) (*Server, context.CancelFunc) {
	t.Helper()
	dir := t.TempDir()
	script := writeScript(t, dir, "app.lua", `
function main() end
function handle_http(req) return "ok" end
`)
	s := NewServer(Config{Mode: mode, ScriptPath: script, Workers: 1, Port: 0})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(8 * time.Second):
			t.Error("server did not stop")
		}
	})
	return s, cancel
}

// TestReadyPublishesBoundAddress pins the contract SmokeTest depends on: after
// Ready() fires, HTTPAddr() returns a real bound address. Reading the address
// before that point raced with Run's write (the -race failure this replaced).
func TestReadyPublishesBoundAddress(t *testing.T) {
	s, _ := newReadyServer(t, "http")

	readyCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := s.WaitReady(readyCtx); err != nil {
		t.Fatalf("WaitReady = %v, want nil (listeners should bind quickly)", err)
	}

	addr := s.HTTPAddr()
	if addr == "" {
		t.Fatal("HTTPAddr() is empty after Ready() fired")
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("HTTPAddr() = %q, want host:port: %v", addr, err)
	}
	if port == "0" {
		t.Errorf("HTTPAddr() port = %q, want the ephemeral port it bound to", port)
	}
	if host == "" {
		t.Errorf("HTTPAddr() host = %q, want a bound host", host)
	}

	// The published address must actually accept connections.
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		t.Fatalf("dial published address %s: %v", addr, err)
	}
	conn.Close()

	// WaitReady is idempotent once ready.
	if err := s.WaitReady(readyCtx); err != nil {
		t.Errorf("second WaitReady = %v, want nil", err)
	}
}

// TestReadyTCPPublishesAddress covers the tcp listener, which is a separate
// field from the HTTP one.
func TestReadyTCPPublishesAddress(t *testing.T) {
	s, _ := newReadyServer(t, "tcp")

	readyCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := s.WaitReady(readyCtx); err != nil {
		t.Fatalf("WaitReady = %v, want nil", err)
	}
	if addr := s.TCPAddr(); addr == "" {
		t.Error("TCPAddr() is empty after Ready() fired for tcp mode")
	}
}

// TestWaitReadyRespectsContext pins that a cancelled context aborts the wait
// rather than blocking forever, and that an aborted wait never blocks the
// server itself.
func TestWaitReadyRespectsContext(t *testing.T) {
	s := NewServer(Config{Mode: "http", ScriptPath: "unused.lua", Workers: 1, Port: 0})
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled
	if err := s.WaitReady(ctx); err == nil {
		t.Error("WaitReady = nil on a cancelled context, want ctx.Err()")
	}
}

// TestReadyNotSignalledOnBootFailure guards the other half of the contract: a
// failed boot must not claim readiness, so a caller watching only Ready() can
// tell "listeners bound" from "server died". This is the path that previously
// burned the full 8s poll deadline in SmokeTest.
func TestReadyNotSignalledOnBootFailure(t *testing.T) {
	dir := t.TempDir()
	// init() errors -> Run returns before any listener binds.
	script := writeScript(t, dir, "app.lua", `
function main() end
function init(config) error("boom") end
`)
	s := NewServer(Config{Mode: "http", ScriptPath: script, Workers: 1, Port: 0})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()

	select {
	case err := <-done:
		if err == nil {
			t.Error("Run = nil, want the init() error to abort startup")
		}
	case <-time.After(8 * time.Second):
		t.Fatal("Run did not return after an init() error")
	}

	select {
	case <-s.Ready():
		t.Error("Ready() fired despite the failed boot")
	default:
	}

	// SmokeTest must surface that failure promptly instead of polling.
	start := time.Now()
	res := SmokeTest(Config{Mode: "http", ScriptPath: script, Workers: 1}, SmokeOptions{})
	if res.OK {
		t.Error("SmokeResult.OK = true for a script whose init() errors, want false")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("SmokeTest took %v to report the boot failure, want well under the 8s poll budget", elapsed)
	}
}
