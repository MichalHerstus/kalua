package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"kalua/internal/bindings"
	glua "github.com/yuin/gopher-lua"
)

type tLogger struct{}

func (tLogger) Printf(string, ...interface{}) {}
func (tLogger) Errorf(string, ...interface{}) {}
func (tLogger) Warnf(string, ...interface{})  {}

func TestRealClipboard(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "app.lua")
	src := `
function main()
  local form = k.form.new("f", {title="t"})
  k.form.on("f", "btn", "click", function(ctx)
    local text = k.clipboard_get()
    _result = text
    _done = true
  end)
  k.form.show("f")
end
`
	if err := os.WriteFile(script, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := New("t1", script, bindings.Options{}, tLogger{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	// Answer any clipboard_get request the app makes.
	go func() {
		for {
			select {
			case out, ok := <-s.Outbox():
				if !ok {
					return
				}
				if out.Type == "clipboard_get" {
					s.PostClipboardResp(out.ID, "hello")
				}
			case <-time.After(3 * time.Second):
				return
			}
		}
	}()

	time.Sleep(100 * time.Millisecond)
	s.PostEvent("f", "btn", "click", glua.LString(""))

	deadline := time.Now().Add(2 * time.Second)
	for {
		flag := s.GetGlobal("_done")
		if flag == glua.LBool(true) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("handler did not complete within deadline; _done=%v", flag)
		}
		time.Sleep(10 * time.Millisecond)
	}

	res := s.GetGlobal("_result")
	if res != glua.LString("hello") {
		t.Fatalf("clipboard value not returned; _result=%v", res)
	}
}

func TestRealMsgbox(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "app.lua")
	src := `
function main()
  local form = k.form.new("f", {title="t"})
  k.form.on("f", "btn", "click", function(ctx)
    local choice = k.msgbox("are you sure?")
    _result = tostring(choice)
    _done = true
  end)
  k.form.show("f")
end
`
	if err := os.WriteFile(script, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := New("t1", script, bindings.Options{}, tLogger{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	// Answer any msgbox the app shows.
	go func() {
		for {
			select {
			case out, ok := <-s.Outbox():
				if !ok {
					return
				}
				if out.Type == "msgbox" {
					s.HandleMsgboxChoice(out.ID, "ok")
				}
			case <-time.After(3 * time.Second):
				return
			}
		}
	}()

	time.Sleep(100 * time.Millisecond)
	s.PostEvent("f", "btn", "click", glua.LString(""))

	// Wait for the handler to complete (poll; the resume is async).
	deadline := time.Now().Add(2 * time.Second)
	for {
		flag := s.GetGlobal("_done")
		if flag == glua.LBool(true) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("handler did not complete within deadline; _done=%v", flag)
		}
		time.Sleep(10 * time.Millisecond)
	}

	res := s.GetGlobal("_result")
	if res != glua.LString("ok") {
		t.Fatalf("msgbox choice not returned; _result=%v", res)
	}
}
