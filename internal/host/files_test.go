package host

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestRun_FilesFlow(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "files.lua")
	src := fmt.Sprintf(`
function main()
  local base = %q

  local f = k.file_open(base .. "/a.txt", "w")
  k.file_write(f, "hello ")
  k.file_write(f, "world")
  k.file_close(f)

  if not k.file_exists(base .. "/a.txt") then k.error("file_exists failed") end

  local f2 = k.file_open(base .. "/a.txt", "r")
  local data = k.file_read(f2)
  if data ~= "hello world" then k.error("read mismatch: " .. data) end
  k.file_close(f2)

  k.file_save(base .. "/b.txt", "line1\nline2\n")
  local f3 = k.file_open(base .. "/b.txt", "r")
  local l1 = k.file_read_line(f3)
  local l2 = k.file_read_line(f3)
  local l3 = k.file_read_line(f3)
  if l1 ~= "line1" or l2 ~= "line2" or l3 ~= nil then
    k.error("read_line failed")
  end
  k.file_close(f3)

  local loaded = k.file_load(base .. "/b.txt")
  if loaded ~= "line1\nline2\n" then k.error("file_load mismatch") end

  k.file_copy(base .. "/b.txt", base .. "/c.txt")
  if not k.file_exists(base .. "/c.txt") then k.error("copy failed") end
  k.file_move(base .. "/c.txt", base .. "/d.txt")
  if k.file_exists(base .. "/c.txt") or not k.file_exists(base .. "/d.txt") then
    k.error("move failed")
  end

  k.file_mkdir(base .. "/sub")
  local list = k.file_list(base)
  local found = false
  for _, name in ipairs(list) do
    if name == "d.txt" then found = true end
  end
  if not found then k.error("list missing d.txt") end

  local info = k.file_info(base .. "/a.txt")
  if info.size ~= 11 or info.is_dir then k.error("file_info wrong") end

  k.file_delete(base .. "/d.txt")
  if k.file_exists(base .. "/d.txt") then k.error("delete failed") end

  local ok, err = pcall(function() k.file_save("/tmp/escapeme.txt", "x") end)
  if ok then k.error("sandbox escape not denied") end
  _ = err

  k.quit()
end
`, tmp)
	if err := os.WriteFile(script, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	log := NewLogger(false)
	var buf bytes.Buffer
	cfg := RunConfig{ScriptPath: script, AllowFS: []string{tmp}, Logger: log, Out: &buf}
	code := Run(cfg)
	if code != ExitOK {
		t.Errorf("Run = %d, want %d\noutput:\n%s", code, ExitOK, buf.String())
	}
}

func TestRun_FilesSandboxDenied(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "deny.lua")
	// The outside dir is deliberately NOT in AllowFS.
	outside := t.TempDir()
	src := fmt.Sprintf(`
function main()
  local ok = pcall(function()
    k.file_load(%q .. "/secret.txt")
  end)
  if ok then k.error("load outside root should be denied") end
  k.quit()
end
`, outside)
	if err := os.WriteFile(script, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	log := NewLogger(false)
	var buf bytes.Buffer
	cfg := RunConfig{ScriptPath: script, AllowFS: []string{tmp}, Logger: log, Out: &buf}
	code := Run(cfg)
	if code != ExitOK {
		t.Errorf("Run = %d, want %d\noutput:\n%s", code, ExitOK, buf.String())
	}
}
