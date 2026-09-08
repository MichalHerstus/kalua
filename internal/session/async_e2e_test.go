package session

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	glua "github.com/yuin/gopher-lua"
	"kalua/internal/bindings"
)

// waitAsyncGlobal polls globals[name] until it becomes true, matching the
// inline polling helpers used by the other session e2e tests.
func waitAsyncGlobal(t *testing.T, s *Session, name string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if s.GetGlobal(name) == glua.LBool(true) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s never became true within deadline", name)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestAsyncFileLoadResult guards against the runBlocking async resume bug: a
// successful k.file_load in a live session must hand the file contents back
// to the script, not resume with nil/"".
func TestAsyncFileLoadResult(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "pgs.txt")
	want := "Postgres test instance\nIP: 127.0.0.1\n"
	if err := os.WriteFile(cfgPath, []byte(want), 0o644); err != nil {
		t.Fatal(err)
	}

	script := filepath.Join(tmp, "app.lua")
	src := fmt.Sprintf(`
function main()
  local text = k.file_load(%q)
  _result = text
  _done = true
end
`, cfgPath)
	if err := os.WriteFile(script, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	s, err := New("t1", script, bindings.Options{AllowFS: []string{tmp}}, tLogger{t: t})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	waitAsyncGlobal(t, s, "_done")
	if got := s.GetGlobal("_result"); got != glua.LString(want) {
		t.Fatalf("file_load returned %q; want %q", got.String(), want)
	}
}

// TestAsyncFileLoadEmpty guards the "" vs nil distinction: reading an empty
// file must resume with an empty string, which is still a valid result.
func TestAsyncFileLoadEmpty(t *testing.T) {
	tmp := t.TempDir()
	empty := filepath.Join(tmp, "empty.txt")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	script := filepath.Join(tmp, "app.lua")
	src := fmt.Sprintf(`
function main()
  local text = k.file_load(%q)
  _is_nil = (text == nil)
  _done = true
end
`, empty)
	if err := os.WriteFile(script, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	s, err := New("t2", script, bindings.Options{AllowFS: []string{tmp}}, tLogger{t: t})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	waitAsyncGlobal(t, s, "_done")
	if s.GetGlobal("_is_nil") == glua.LBool(true) {
		t.Fatal("file_load of an empty file resumed nil; want empty string")
	}
}

// TestAsyncFileLoadError guards the catch+continue path: on failure the
// coroutine resumes nil, ERRORMSG is set and the k.on_error hook fires with
// the real message.
func TestAsyncFileLoadError(t *testing.T) {
	tmp := t.TempDir()
	missing := filepath.Join(tmp, "nope.txt")

	script := filepath.Join(tmp, "app.lua")
	src := fmt.Sprintf(`
k.on_error(function(code, msg)
  _on_err_code = code
  _on_err_msg = msg
end)
function main()
  local text = k.file_load(%q)
  _got_nil = (text == nil)
  _errmsg = tostring(ERRORMSG)
  _done = true
end
`, missing)
	if err := os.WriteFile(script, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	s, err := New("t3", script, bindings.Options{AllowFS: []string{tmp}}, tLogger{t: t})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	waitAsyncGlobal(t, s, "_done")
	if s.GetGlobal("_got_nil") != glua.LBool(true) {
		t.Fatalf("file_load on missing file did not resume nil; _got_nil=%v", s.GetGlobal("_got_nil"))
	}
	if msg := s.GetGlobal("_errmsg"); msg == glua.LNil || msg.String() == "" {
		t.Fatalf("ERRORMSG not populated on failure: %v", msg)
	}
	if code := s.GetGlobal("_on_err_code"); code == glua.LNil {
		t.Fatalf("k.on_error hook not fired on file_load failure")
	}
	if msg := s.GetGlobal("_on_err_msg"); msg == glua.LNil || msg.String() == "" {
		t.Fatalf("k.on_error got empty message: %v", msg)
	}
}