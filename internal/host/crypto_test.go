package host

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestRun_CryptoFlow(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "crypto.lua")
	src := `function main()
  if k.checksum("md5", "abc") ~= "900150983cd24fb0d6963f7d28e17f72" then k.error("md5") end
  if k.checksum("sha1", "abc") ~= "a9993e364706816aba3e25717850c26c9cd0d89d" then k.error("sha1") end
  if k.checksum("sha256", "abc") ~= "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" then k.error("sha256") end
  if k.checksum("crc32", "123456789") ~= "cbf43926" then k.error("crc32") end
  if k.checksum("hmac-sha256", "The quick brown fox jumps over the lazy dog", "key") ~= "f7bc83f430538424b13298e6aa6fb143ef4d59a14946175997479dbc2d1a3cd8" then k.error("hmac") end

  local pb = k.checksum("pbkdf2", "password", "salt", 1000, 32)
  if #pb ~= 64 then k.error("pbkdf2 length") end
  if pb ~= "632c2812e46d4604102ba7618e9d6d7d2f8128f6266b4a03264d2a0460b7dcb3" then k.error("pbkdf2 vector") end

  local enc1 = k.encrypt("secret data", "password123")
  local enc2 = k.encrypt("secret data", "password123")
  if enc1 == enc2 then k.error("encrypt must be randomized (nonce)") end
  if k.decrypt(enc1, "password123") ~= "secret data" then k.error("decrypt") end

  local ok, err = pcall(function() k.decrypt(enc1, "wrongkey") end)
  if ok then k.error("decrypt with wrong key should fail") end
  _ = err

  local ok2, err2 = pcall(function() k.decrypt("!!!not-base64!!!", "password123") end)
  if ok2 then k.error("decrypt invalid base64 should fail") end
  _ = err2

  k.quit()
end
`
	if err := os.WriteFile(script, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	log := NewLogger(false)
	var buf bytes.Buffer
	cfg := RunConfig{ScriptPath: script, Logger: log, Out: &buf}
	code := Run(cfg)
	if code != ExitOK {
		t.Errorf("Run = %d, want %d\noutput:\n%s", code, ExitOK, buf.String())
	}
}
