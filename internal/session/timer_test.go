package session

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"kalua/internal/bindings"
)

// discardLogger satisfies the session Logger interface silently.
type discardLogger struct{}

func (discardLogger) Printf(string, ...interface{}) {}
func (discardLogger) Errorf(string, ...interface{}) {}
func (discardLogger) Warnf(string, ...interface{})  {}
func (discardLogger) Tracef(string, ...interface{}) {}

// captureLogger collects error messages so tests can surface failures.
type captureLogger struct{ errs []string }

func newCaptureLogger() *captureLogger { return &captureLogger{} }
func (c *captureLogger) Printf(string, ...interface{}) {}
func (c *captureLogger) Errorf(f string, a ...interface{}) {
	c.errs = append(c.errs, fmt.Sprintf(f, a...))
}
func (c *captureLogger) Warnf(string, ...interface{}) {}
func (c *captureLogger) Tracef(string, ...interface{}) {}

// TestTimerFiresGlobal verifies k.timer_start fires a Lua global named after
// the timer id through the actor loop.
func TestTimerFiresGlobal(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "timer.lua")
	// The timer handler mutates an outbox-visible status bar so the test can
	// observe the fire without blocking.
	src := `function fired()
  k.status_show("fired")
end
function main()
  k.timer_start("fired", 20, false)
end
`
	if err := os.WriteFile(script, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	log := newCaptureLogger()
	s, err := New("sess1", script, bindings.Options{}, log)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	deadline := time.After(3 * time.Second)
	for {
		select {
		case msg := <-s.Outbox():
			if msg.Type == "status" && msg.Text == "fired" {
				return // timer fired and reached the outbox
			}
			if msg.Type == "error" {
				t.Fatalf("session error: %s (logger: %v)", msg.Msg, log.errs)
			}
			if msg.Type == "quit" {
				// main() returned; still wait for the timer to fire
			}
		case <-deadline:
			t.Fatalf("timed out waiting for timer to fire (logger errs: %v)", log.errs)
		}
	}
}

// TestTimerDoesNotFireAfterStop verifies k.timer_stop prevents delivery.
func TestTimerDoesNotFireAfterStop(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "timerstop.lua")
	src := `function bogus()
  k.status_show("should-not-fire")
end
function main()
  k.timer_start("bogus", 20, false)
  k.timer_stop("bogus")
end
`
	if err := os.WriteFile(script, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := New("sess2", script, bindings.Options{}, discardLogger{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	deadline := time.After(300 * time.Millisecond)
	select {
	case msg := <-s.Outbox():
		if msg.Type == "status" && msg.Text == "should-not-fire" {
			t.Fatal("timer fired despite timer_stop")
		}
	case <-deadline:
		// No status outbox within the window: timer correctly stopped.
	}
}