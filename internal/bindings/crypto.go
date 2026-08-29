// Package bindings implements the crypto bindings (Phase 5 - Data groups).
package bindings

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"hash"
	"hash/crc32"
	"strings"

	"github.com/yuin/gopher-lua"
	"golang.org/x/crypto/pbkdf2"
)

// registerCrypto installs the k.checksum / k.encrypt / k.decrypt bindings.
func registerCrypto(e *Env) {
	// k.checksum(alg, data [, key] [, salt] [, iterations [, keylen]])
	// algs: crc32, md5, sha1, sha256, hmac-sha256, pbkdf2 (hex output, lowercase).
	e.register("checksum", "crypto", func(L *lua.LState) int {
		alg := strings.ToLower(L.CheckString(1))
		data := []byte(luaToString(L, 2))

		var out []byte
		switch alg {
		case "crc32":
			out = []byte(fmt.Sprintf("%08x", crc32.ChecksumIEEE(data)))
		case "md5":
			out = []byte(hex.EncodeToString(sumHash(md5.New(), data)))
		case "sha1":
			out = []byte(hex.EncodeToString(sumHash(sha1.New(), data)))
		case "sha256":
			out = []byte(hex.EncodeToString(sumHash(sha256.New(), data)))
		case "hmac-sha256":
			key := []byte(L.OptString(3, ""))
			if len(key) == 0 {
				L.RaiseError("checksum error: hmac-sha256 requires a key")
				return 0
			}
			mac := hmac.New(sha256.New, key)
			mac.Write(data)
			out = []byte(hex.EncodeToString(mac.Sum(nil)))
		case "pbkdf2":
			salt := []byte(L.OptString(3, ""))
			iterations := L.OptInt(4, 10000)
			keylen := L.OptInt(5, 32)
			if iterations < 1 || keylen < 1 {
				L.RaiseError("checksum error: pbkdf2 requires positive iterations and keylen")
				return 0
			}
			if len(salt) == 0 {
				L.RaiseError("checksum error: pbkdf2 requires a salt")
				return 0
			}
			out = []byte(hex.EncodeToString(pbkdf2.Key(data, salt, iterations, keylen, sha256.New)))
		default:
			L.RaiseError("checksum error: unknown algorithm %q (use crc32, md5, sha1, sha256, hmac-sha256, pbkdf2)", alg)
			return 0
		}
		L.Push(lua.LString(out))
		return 1
	})

	// k.encrypt(plaintext, key) -> base64(nonce || AES-GCM ciphertext)
	e.register("encrypt", "crypto", func(L *lua.LState) int {
		plain := []byte(luaToString(L, 1))
		key := []byte(L.CheckString(2))
		enc, err := aesGCMSeal(plain, key)
		if err != nil {
			L.RaiseError("encrypt error: %v", err)
			return 0
		}
		L.Push(lua.LString(base64.StdEncoding.EncodeToString(enc)))
		return 1
	})

	// k.decrypt(base64(nonce || ciphertext), key) -> plaintext
	e.register("decrypt", "crypto", func(L *lua.LState) int {
		b64 := L.CheckString(1)
		key := []byte(L.CheckString(2))
		raw, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			L.RaiseError("decrypt error: invalid base64: %v", err)
			return 0
		}
		plain, err := aesGCMOpen(raw, key)
		if err != nil {
			L.RaiseError("decrypt error: %v", err)
			return 0
		}
		L.Push(lua.LString(plain))
		return 1
	})
}

func sumHash(h hash.Hash, data []byte) []byte {
	h.Write(data)
	return h.Sum(nil)
}

// aesGCMSeal encrypts plain under an AES-GCM key derived from key via SHA-256.
// The 12-byte random nonce is prepended to the ciphertext.
func aesGCMSeal(plain, key []byte) ([]byte, error) {
	keySum := sha256.Sum256(key)
	block, err := aes.NewCipher(keySum[:])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	sealed := gcm.Seal(nil, nonce, plain, nil)
	out := make([]byte, 0, len(nonce)+len(sealed))
	out = append(out, nonce...)
	out = append(out, sealed...)
	return out, nil
}

// aesGCMOpen reverses aesGCMSeal.
func aesGCMOpen(raw, key []byte) ([]byte, error) {
	keySum := sha256.Sum256(key)
	block, err := aes.NewCipher(keySum[:])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	ns := gcm.NonceSize()
	if len(raw) < ns {
		return nil, fmt.Errorf("ciphertext too short")
	}
	return gcm.Open(nil, raw[:ns], raw[ns:], nil)
}
