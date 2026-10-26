//go:build !wasm

package server

import (
	"context"
	"testing"
	"time"
)

// TestCallHTTPBoundedBusyWait verifies Phase 0.4: when a worker is busy, a
// CallHTTP waits a bounded time and then answers 503 + Retry-After instead of
// failing instantly. It also confirms the slot serialization still lets a
// waiting request proceed once the worker frees up.
func TestCallHTTPBoundedBusyWait(t *testing.T) {
	script := `
function handle_http(req)
  return {status=200, body="ok"}
end
function main() end
`
	w := newTestWorker(t, script)

	// Occupy the worker's slot.
	w.acquireSlot()
	defer w.freeSlot()

	start := time.Now()
	resp, err := w.CallHTTP(context.Background(), HTTPRequest{Method: "GET", Path: "/x"})
	elapsed := time.Since(start)

	if err == nil {
		t.Errorf("expected busy error, got status %d", resp.Status)
	}
	if resp.Status != 503 {
		t.Errorf("status = %d, want 503", resp.Status)
	}
	if ra := resp.Headers["Retry-After"]; ra != "1" {
		t.Errorf("Retry-After = %q, want 1", ra)
	}
	// busyTimeout is 500ms; allow scheduler slack.
	if elapsed < busyTimeout/2 {
		t.Errorf("returned after %v, want roughly >= %v (bounded wait)", elapsed, busyTimeout/2)
	}
	if elapsed > busyTimeout+2*time.Second {
		t.Errorf("wait too long: %v", elapsed)
	}
}

// TestCallHTTPProceedsWhenWorkerFrees verifies the wait unblocks as soon as the
// slot is released (no artificial latency beyond the wait goal).
func TestCallHTTPProceedsWhenWorkerFrees(t *testing.T) {
	script := `
function handle_http(req)
  return {status=200, body="ok"}
end
function main() end
`
	w := newTestWorker(t, script)
	w.acquireSlot()

	done := make(chan struct{})
	go func() {
		defer close(done)
		resp, err := w.CallHTTP(context.Background(), HTTPRequest{Method: "GET", Path: "/x"})
		if err != nil {
			t.Errorf("CallHTTP after free: %v", err)
			return
		}
		if resp.Status != 200 {
			t.Errorf("status = %d, want 200", resp.Status)
		}
	}()

	time.Sleep(120 * time.Millisecond) // request is now waiting
	select {
	case <-done:
		t.Fatal("request completed while worker was still busy")
	default:
	}
	w.freeSlot() // free the worker → the wait should unblock promptly
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("request did not unblock after worker freed")
	}
}

// TestWorkerSerialization ensures two concurrent CallHTTP requests serialize
// on the slot (each Lua invocation exclusive), even without a busy worker.
func TestWorkerSerialization(t *testing.T) {
	script := `
function handle_http(req)
  return {status=200, body=req.path}
end
function main() end
`
	w := newTestWorker(t, script)

	const n = 8
	results := make(chan string, n)
	for i := 0; i < n; i++ {
		go func(i int) {
			p := "/p" + string(rune('a'+i))
			resp, err := w.CallHTTP(context.Background(), HTTPRequest{Method: "GET", Path: p})
			if err != nil {
				results <- "err"
				return
			}
			results <- resp.Body
		}(i)
	}
	seen := map[string]bool{}
	for i := 0; i < n; i++ {
		select {
		case b := <-results:
			if b == "err" {
				t.Fatalf("one request errored (busy?)")
			}
			seen[b] = true
		case <-time.After(5 * time.Second):
			t.Fatalf("request hung; only %d/%d completed", i, n)
		}
	}
	for _, p := range []string{"/pa", "/pb", "/pc", "/pd", "/pe", "/pf", "/pg", "/ph"} {
		if !seen[p] {
			t.Errorf("missing response for %s (got %v)", p, seen)
		}
	}
}
