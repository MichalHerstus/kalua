package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kalua/internal/bindings"
)

// TestTabDemoRendersHeader checks that a real session running the committed
// tab_demo.lua emits a render_form outbox message whose HTML contains the tab
// headers and buttons — end-to-end coverage of k.ctrl.tab + renderTab.
func TestTabDemoRendersHeader(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "tab_demo.lua")
	src, err := os.ReadFile("../../testdata/apps/tab_demo.lua")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(script, src, 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := New("t1", script, bindings.Options{}, tLogger{t: t})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	var html string
	done := make(chan struct{})
	go func() {
		for msg := range s.Outbox() {
			if msg.Type == "render_form" && msg.Form == "main" {
				html = msg.HTML
				close(done)
				return
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(4 * time.Second):
		t.Fatalf("timed out waiting for render_form")
	}

	for _, want := range []string{"kalua-tab", "kalua-tab-btn", "Dashboard", "Reports", "Settings", "data-k-tab-position=\"top\""} {
		if !strings.Contains(html, want) {
			t.Errorf("render_form HTML missing %q in:\n%s", want, html)
		}
	}
}
