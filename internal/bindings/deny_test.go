package bindings

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yuin/gopher-lua"

	"kalua/internal/vm"
)

// newDenyEnv builds a bindings Env whose sandbox home is a private temp dir
// (with AllowFS covering it), the host-artefact DenyFS defaults, plus one extra
// DenyFS entry — mirroring production wiring (setup_native.go + bindings.go).
func newDenyEnv(t *testing.T) (*Env, *lua.LState) {
	t.Helper()
	tmp := t.TempDir()
	L := vm.New()
	t.Cleanup(L.Close)
	app := vm.NewApp(L)
	opts := Options{AllowFS: []string{tmp}, DenyFS: []string{filepath.Join(tmp, "protected.db")}}
	e := Setup(L, app, opts, nil, nil)
	// Setup pins the sandbox home to the process CWD; override it so the test
	// is hermetic and any file writes land in t.TempDir().
	e.workdir = tmp
	e.denyFS = denyFSFor(opts, tmp)
	return e, L
}

// TestResolvePathDeny pins the DenyFS semantics (P11/L16): host artefacts and
// DenyFS entries are denied even inside allowed roots, siblings of a denied
// file are not, and path traversal still fails.
func TestResolvePathDeny(t *testing.T) {
	e, _ := newDenyEnv(t)
	tmp := e.workdir

	allowed := []string{
		"app.db",
		filepath.Join(tmp, "app.db"),
		"sub/app.db",
		"protected.db.bak", // sibling of a denied file is not denied
	}
	for _, p := range allowed {
		if _, err := e.resolvePath(p); err != nil {
			t.Errorf("resolvePath(%q) denied: %v", p, err)
		}
	}

	denied := []string{
		".kalua.params.json",        // host artefact (default DenyFS)
		filepath.Join(tmp, ".kalua.params.json"),
		"sub/../.kalua.params.json", // cleans to the host artefact
		"protected.db",              // operator DenyFS entry
		filepath.Join(tmp, "protected.db"),
		"../outside.txt", // outside allowed roots
	}
	for _, p := range denied {
		_, err := e.resolvePath(p)
		if err == nil {
			t.Errorf("resolvePath(%q) allowed, want denial", p)
			continue
		}
		if !strings.Contains(err.Error(), "denied") {
			t.Errorf("resolvePath(%q) err = %q, want a denial message", p, err)
		}
	}
}

// TestFileLoadSaveDenied verifies the deny list applies to both reads and
// writes of host artefacts and DenyFS entries through the file bindings.
func TestFileLoadSaveDenied(t *testing.T) {
	e, L := newDenyEnv(t)
	tmp := e.workdir
	if err := os.WriteFile(filepath.Join(tmp, "app.db"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	src := `
local ok = k.file_load("app.db")
assert(ok == "hello", "file_load(app.db) = " .. tostring(ok))

local got = k.file_load(".kalua.params.json")
assert(got == nil, "file_load read host artefact: " .. tostring(got))

local got2 = k.file_load("protected.db")
assert(got2 == nil, "file_load read DenyFS entry: " .. tostring(got2))

-- The deny list also blocks writes.
local wr = k.file_save(".kalua.params.json", "x")
assert(wr == nil, "file_save wrote host artefact")
local wr2 = k.file_save("protected.db", "x")
assert(wr2 == nil, "file_save wrote DenyFS entry")
`
	if err := L.DoString(src); err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, ".kalua.params.json")); !os.IsNotExist(err) {
		t.Error("host artefact was created on disk")
	}
	if _, err := os.Stat(filepath.Join(tmp, "protected.db")); !os.IsNotExist(err) {
		t.Error("DenyFS entry was created on disk")
	}
}

// TestConnectDBSandbox pins the C7 fix: k.connect_db routes file-backed
// sqlite DSNs through the filesystem sandbox, so an outside path is denied, a
// host artefact is denied even inside the sandbox, while in-workdir files and
// :memory: still work.
func TestConnectDBSandbox(t *testing.T) {
	_, L := newDenyEnv(t)

	// C7 regression: a sqlite DSN pointing outside the sandbox is denied.
	outside := filepath.Join(t.TempDir(), "secret.db")
	denySrc := fmt.Sprintf(`
local db = k.connect_db(%q)
assert(db == nil, "connect_db escaped the sandbox: " .. tostring(db))
assert(ERRORMSG ~= nil, "connect_db failed without ERRORMSG")
`, "sqlite://"+outside)
	if err := L.DoString(denySrc); err != nil {
		t.Fatalf("deny script failed: %v", err)
	}

	// A host artefact is denied even when the path is sandbox-relative.
	artefactSrc := `
local db2 = k.connect_db("sqlite://.kalua.params.json")
assert(db2 == nil, "connect_db opened host artefact: " .. tostring(db2))
`
	if err := L.DoString(artefactSrc); err != nil {
		t.Fatalf("artefact script failed: %v", err)
	}

	// In-workdir file-backed connect_db still works; :memory: is unrestricted.
	okSrc := `
local db3 = k.connect_db("sqlite://app.db")
assert(db3 ~= nil, "in-workdir connect_db denied: " .. tostring(ERRORMSG))
k.sql(db3, "CREATE TABLE IF NOT EXISTS t (id INTEGER PRIMARY KEY, name TEXT)")
k.db_insert(db3, "t", {name = "x"})
local res = k.db_select(db3, "t", {"name"}, {})
if #res.rows ~= 1 or res.rows[1].name ~= "x" then error("db round-trip failed") end
k.disconnect_db(db3)

local mem = k.connect_db("sqlite://:memory:")
assert(mem ~= nil, ":memory: connect_db denied: " .. tostring(ERRORMSG))
k.sql(mem, "CREATE TABLE m (id INTEGER PRIMARY KEY)")
k.disconnect_db(mem)
`
	if err := L.DoString(okSrc); err != nil {
		t.Fatalf("ok script failed: %v", err)
	}
}

// TestZipExtractMemberDenied verifies zip_extract refuses to write a member
// named after a host artefact (L16/P11) without touching the disk, while a
// benign archive extracts normally.
func TestZipExtractMemberDenied(t *testing.T) {
	e, L := newDenyEnv(t)
	tmp := e.workdir

	writeZip := func(name string, entries map[string]string) {
		t.Helper()
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		for n, content := range entries {
			w, err := zw.Create(n)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := w.Write([]byte(content)); err != nil {
				t.Fatal(err)
			}
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(tmp, name), buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	writeZip("bad.zip", map[string]string{".kalua.params.json": "x", "ok.txt": "y"})
	badSrc := `
local n = k.zip_extract("bad.zip", ".")
assert(n == nil, "zip_extract wrote denied member: " .. tostring(n))
`
	if err := L.DoString(badSrc); err != nil {
		t.Fatalf("bad zip script failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, ".kalua.params.json")); !os.IsNotExist(err) {
		t.Error("denied zip member was written to disk")
	}

	writeZip("safe.zip", map[string]string{"ok.txt": "hi"})
	okSrc := `
local n2 = k.zip_extract("safe.zip", ".")
if n2 ~= 1 then error("zip_extract(safe) = " .. tostring(n2)) end
`
	if err := L.DoString(okSrc); err != nil {
		t.Fatalf("safe zip script failed: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(tmp, "ok.txt")); err != nil || string(data) != "hi" {
		t.Fatalf("safe member = %q, err = %v", data, err)
	}
}
