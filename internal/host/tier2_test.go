package host

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// rsaKeyPair returns a PEM private/public key pair for asymmetric crypto tests.
func rsaKeyPair(t *testing.T) (privPEM, pubPEM string) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	privBytes := x509.MarshalPKCS1PrivateKey(priv)
	privPEM = string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: privBytes}))
	pubBytes, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	pubPEM = string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubBytes}))
	return privPEM, pubPEM
}

// runTestScript writes a script to a temp file and runs it headless.
// extraFS lists additional temp roots to grant filesystem access to (the
// script temp dir is always allowed).
func runTestScript(t *testing.T, src string, extraFS ...string) (string, ExitCode) {
	t.Helper()
	tmp := t.TempDir()
	script := filepath.Join(tmp, "app.lua")
	if err := os.WriteFile(script, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	allow := append([]string{tmp}, extraFS...)
	log := NewLogger(false)
	var buf bytes.Buffer
	cfg := RunConfig{ScriptPath: script, Logger: log, Out: &buf, AllowFS: allow}
	return buf.String(), Run(cfg)
}

func TestRun_Tier2Formats(t *testing.T) {
	datadir := t.TempDir()
	out, code := runTestScript(t, fmt.Sprintf(`function main()
  local tmp = %q

  -- CSV parse/string
  local rows = k.csv_parse("a,b\n1,2\n3,4", {header=true})
  if rows[1].a ~= "1" or rows[2].b ~= "4" then k.error("csv_parse header") end
  local arr = k.csv_parse("1,2\n3,4")
  if arr[1][2] ~= "2" then k.error("csv_parse array") end
  local csv2 = k.csv_string({{"a","b"},{"5","6"}})
  if not csv2:find("5,6") then k.error("csv_string") end

  -- csv file round-trip
  local f = tmp .. "/out.csv"
  k.csv_save(f, {{"x","y"},{"9","8"}})
  local loaded = k.csv_load(f, {header=true})
  if loaded[1].x ~= "9" or loaded[1].y ~= "8" then k.error("csv_save/load") end

  -- INI
  local ini = k.ini_parse("[db]\nhost=localhost\nport=5432\n")
  if ini.db.host ~= "localhost" or ini.db.port ~= "5432" then k.error("ini_parse") end
  local ini2 = k.ini_string({_root={app="demo"}, db={host="h"}})
  if not ini2:find("[db]") or not ini2:find("app=demo") then k.error("ini_string") end
  local ipath = tmp .. "/c.ini"
  k.ini_save(ipath, {_root={a="1"}, s={b="2"}})
  if k.ini_read(ipath, "s", "b") ~= "2" then k.error("ini_read") end
  k.ini_write(ipath, "s", "c", "3")
  if k.ini_read(ipath, "s", "c") ~= "3" then k.error("ini_write") end
  local ini3 = k.ini_load(ipath)
  if ini3._root.a ~= "1" or ini3.s.c ~= "3" then k.error("ini_load roundtrip") end

  -- YAML parse/string/load/save
  local y = k.yaml_parse("name: kalua\nnums:\n  - 1\n  - 2\n")
  if y.name ~= "kalua" or y.nums[2] ~= 2 then k.error("yaml_parse") end
  local ystr = k.yaml_string({a=1, b={c="x"}})
  if not ystr:find("a: 1") then k.error("yaml_string") end
  local ypath = tmp .. "/d.yaml"
  k.yaml_save(ypath, {q=1})
  local y2 = k.yaml_load(ypath)
  if y2.q ~= 1 then k.error("yaml_save/load") end

  -- XML load/save
  local xpath = tmp .. "/e.xml"
  k.file_save(xpath, '<root><item id="1">one</item></root>')
  local doc = k.xml_load(xpath)
  if doc._name ~= "root" then k.error("xml_load name") end
  if doc._children[1]._name ~= "item" then k.error("xml_load child") end
  if doc._children[1]._attrs.id ~= "1" then k.error("xml_load attrs") end
  if doc._children[1]._text ~= "one" then k.error("xml_load text") end
  local xp2 = tmp .. "/f.xml"
  k.xml_save(xp2, {_name="r", _children={{_name="c", _text="t"}}})
  local doc2 = k.xml_load(xp2)
  if doc2._children[1]._text ~= "t" then k.error("xml_save roundtrip") end

  k.quit()
end
`, datadir), datadir)
	if code != ExitOK {
		t.Errorf("Run = %d, want ExitOK\noutput:\n%s", code, out)
	}
}

func TestRun_Tier2Rows(t *testing.T) {
	out, code := runTestScript(t, `function main()
  -- json_to_rows / rows_to_json
  local rs = k.json_to_rows({{name="a", age=1}, {name="b", age=2}})
  if rs.columns[1] ~= "age" or rs.rows[2].name ~= "b" then k.error("json_to_rows") end
  local rows = k.rows_to_json(rs)
  if #rows ~= 2 or rows[1].age ~= 1 then k.error("rows_to_json") end

  -- csv_to_rows / rows_to_csv
  local crs = k.csv_to_rows({{"h","w"},{"a","b"}})
  if crs.columns[1] ~= "h" or crs.rows[1].w ~= "b" then k.error("csv_to_rows") end
  local csvtext = k.rows_to_csv(crs)
  if not csvtext:find("h,w") then k.error("rows_to_csv header") end

  -- xml_to_rows / rows_to_xml
  local doc = k.xml_parse('<rows><row><id>1</id></row><row><id>2</id></row></rows>')
  -- convert parse handle to element table shape via _document? use xml_load style table:
  local xtbl = {_name="rows", _children={
    {_name="row", _children={{_name="id", _text="1"}}},
    {_name="row", _children={{_name="id", _text="2"}}},
  }}
  local xrs = k.xml_to_rows(xtbl)
  if xrs.rows[2].id ~= "2" then k.error("xml_to_rows") end
  local xtext = k.rows_to_xml(xrs, "rows", "row")
  if not xtext:find("<rows>") or not xtext:find("<id>1</id>") then k.error("rows_to_xml") end

  k.quit()
end
`)
	if code != ExitOK {
		t.Errorf("Run = %d, want ExitOK\noutput:\n%s", code, out)
	}
}

func TestRun_Tier2SQLite(t *testing.T) {
	dbDir := t.TempDir()
	out, code := runTestScript(t, fmt.Sprintf(`function main()
  local db = k.connect_sqlite(%q)
  k.sql(db, "CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT)")
  k.db_insert(db, "t", {name="hi"})
  local res = k.db_select(db, "t", {"id","name"})
  if #res.rows ~= 1 or res.rows[1].name ~= "hi" then k.error("sqlite select") end
  k.db_kill_table(db, "t", {id=1})
  if #k.db_select(db, "t", {"id"}).rows ~= 0 then k.error("db_kill_table") end
  k.disconnect_sqlite(db)
  k.quit()
end
`, filepath.Join(dbDir, "t.db")), dbDir)
	if code != ExitOK {
		t.Errorf("Run = %d, want ExitOK\noutput:\n%s", code, out)
	}
}

func TestRun_Tier2CryptoAsymmetric(t *testing.T) {
	priv, pub := rsaKeyPair(t)
	out, code := runTestScript(t, fmt.Sprintf(`function main()
  local priv = %q
  local pub = %q

  -- sign/verify
  local sig = k.sign("hello", priv)
  if k.verify("hello", sig, pub) ~= true then k.error("verify ok") end
  if k.verify("tampered", sig, pub) ~= false then k.error("verify tamper") end

  -- encrypt/decrypt
  local ct = k.crypt_asymmetric("rsa-encrypt", pub, "secret")
  if k.crypt_asymmetric("rsa-decrypt", priv, ct) ~= "secret" then k.error("asym roundtrip") end

  k.quit()
end
`, priv, pub))
	if code != ExitOK {
		t.Errorf("Run = %d, want ExitOK\noutput:\n%s", code, out)
	}
}

func TestRun_Tier2SymmetricCrypto(t *testing.T) {
	out, code := runTestScript(t, `function main()
  local key = "0123456789abcdef0123456789abcdef"
  local ct = k.crypt_symmetric("aes-encrypt", key, "data to seal")
  if k.crypt_symmetric("aes-decrypt", key, ct) ~= "data to seal" then k.error("sym roundtrip") end
  local iv = "0123456789abcdef"
  local ct2 = k.crypt_symmetric("aes-encrypt", key, "x", iv)
  local ct3 = k.crypt_symmetric("aes-encrypt", key, "x", iv)
  if ct2 ~= ct3 then k.error("sym deterministic with iv") end
  k.quit()
end
`)
	if code != ExitOK {
		t.Errorf("Run = %d, want ExitOK\noutput:\n%s", code, out)
	}
}

func TestRun_Tier2Zip(t *testing.T) {
	dir := t.TempDir()
	out, code := runTestScript(t, fmt.Sprintf(`function main()
  local zip = %q
  k.zip_add(zip, {a="hello", b="world"})
  local names = k.zip_list(zip)
  if #names ~= 2 then k.error("zip_list") end
  local dest = %q
  local n = k.zip_extract(zip, dest)
  if n ~= 2 then k.error("zip_extract count " .. n) end
  if k.file_load(dest .. "/a") ~= "hello" then k.error("zip_extract content") end
  k.quit()
end
`, filepath.Join(dir, "x.zip"), filepath.Join(dir, "out")), dir)
	if code != ExitOK {
		t.Errorf("Run = %d, want ExitOK\noutput:\n%s", code, out)
	}
}

func TestRun_Tier2ParamsAndFlow(t *testing.T) {
	out, code := runTestScript(t, `function main()
  k.param_set("greeting", "hola")
  if k.param_get("greeting") ~= "hola" then k.error("param") end
  if k.locale() ~= "en-US" then k.error("locale") end
  -- net_ok/ping are best-effort; assume ring-when-reachable but never crash
  local ok, err = pcall(function() return k.net_ok(200) end)
  if not ok then k.error("net_ok must not raise") end
  _ = err
  local ping = k.ping("127.0.0.1", 200)
  if ping ~= nil and ping < 0 then k.error("ping negative") end
  k.quit()
end
`)
	if code != ExitOK {
		t.Errorf("Run = %d, want ExitOK\noutput:\n%s", code, out)
	}
}

func TestRun_Tier2Socket(t *testing.T) {
	// Start a tiny echo server in-process.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	host, portStr, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(portStr)

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 128)
		n, _ := conn.Read(buf)
		conn.Write([]byte("echo:"))
		conn.Write(buf[:n])
	}()

	out, code := runTestScript(t, fmt.Sprintf(`function main()
  local s = k.socket_open(%q, %d, 3000)
  k.socket_write(s, "ping")
  local echo = k.socket_read(s, 9)  -- "echo:ping"
  if echo ~= "echo:ping" then k.error("socket roundtrip got [" .. echo .. "]") end
  k.socket_close(s)
  k.quit()
end
`, host, port))
	<-done
	if code != ExitOK {
		t.Errorf("Run = %d, want ExitOK\noutput:\n%s", code, out)
	}
}