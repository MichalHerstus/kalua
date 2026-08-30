// Package bindings implements the crypto bindings (Phase 5 - Data groups).
package bindings

import (
	"bytes"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
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

	// k.crypt_symmetric(alg, key, data[, iv]) — AES-CBC symmetric encrypt/decrypt.
	// alg "aes-encrypt"/"encrypt"/"aes-cbc-encrypt" encrypts; the "-decrypt"
	// variants reverse it. key is 16/24/32 bytes. With an explicit iv the
	// ciphertext is iv || block; without one a random iv is prepended. Returns
	// base64.
	e.register("crypt_symmetric", "crypto", func(L *lua.LState) int {
		alg := strings.ToLower(L.CheckString(1))
		key := []byte(L.CheckString(2))
		data := []byte(luaToString(L, 3))

		decryptMode := strings.Contains(alg, "decrypt")
		if decryptMode {
			raw, err := base64.StdEncoding.DecodeString(luaToString(L, 3))
			if err != nil {
				L.RaiseError("crypt_symmetric error: invalid base64: %v", err)
				return 0
			}
			var iv []byte
			if L.GetTop() >= 4 && L.Get(4) != lua.LNil {
				iv = []byte(L.CheckString(4))
			} else {
				iv, raw = raw[:aes.BlockSize], raw[aes.BlockSize:]
			}
			out, err := aesCBCCrypt(key, iv, raw, true)
			if err != nil {
				L.RaiseError("crypt_symmetric error: %v", err)
				return 0
			}
			L.Push(lua.LString(out))
			return 1
		}

		var iv []byte
		if L.GetTop() >= 4 && L.Get(4) != lua.LNil {
			iv = []byte(L.CheckString(4))
			if len(iv) != aes.BlockSize {
				L.RaiseError("crypt_symmetric error: iv must be %d bytes", aes.BlockSize)
				return 0
			}
		} else {
			iv = make([]byte, aes.BlockSize)
			if _, err := rand.Read(iv); err != nil {
				L.RaiseError("crypt_symmetric error: %v", err)
				return 0
			}
		}
		ct, err := aesCBCCrypt(key, iv, data, false)
		if err != nil {
			L.RaiseError("crypt_symmetric error: %v", err)
			return 0
		}
		out := make([]byte, 0, len(iv)+len(ct))
		out = append(out, iv...)
		out = append(out, ct...)
		L.Push(lua.LString(base64.StdEncoding.EncodeToString(out)))
		return 1
	})

	// k.crypt_asymmetric(alg, key, data) — RSA encrypt/decrypt with a PEM key.
	// alg "rsa-encrypt" / "rsa-pkcs1-encrypt" → PKCS#1 v1.5 encrypt; "rsa-decrypt"
	// → decrypt. key is a PEM public key for encrypt / private key for decrypt.
	e.register("crypt_asymmetric", "crypto", func(L *lua.LState) int {
		alg := strings.ToLower(L.CheckString(1))
		key := []byte(L.CheckString(2))
		data := []byte(luaToString(L, 3))

		if strings.Contains(alg, "decrypt") {
			raw, err := base64.StdEncoding.DecodeString(luaToString(L, 3))
			if err != nil {
				L.RaiseError("crypt_asymmetric error: invalid base64: %v", err)
				return 0
			}
			priv, err := parseRSAPrivateKey(key)
			if err != nil {
				L.RaiseError("crypt_asymmetric error: %v", err)
				return 0
			}
			out, err := rsa.DecryptPKCS1v15(nil, priv, raw)
			if err != nil {
				L.RaiseError("crypt_asymmetric error: %v", err)
				return 0
			}
			L.Push(lua.LString(out))
			return 1
		}

		pub, err := parseRSAPublicKey(key)
		if err != nil {
			L.RaiseError("crypt_asymmetric error: %v", err)
			return 0
		}
		out, err := rsa.EncryptPKCS1v15(nil, pub, data)
		if err != nil {
			L.RaiseError("crypt_asymmetric error: %v", err)
			return 0
		}
		L.Push(lua.LString(base64.StdEncoding.EncodeToString(out)))
		return 1
	})

	// k.sign(data, key[, alg]) — RSA PKCS#1 v1.5 signature (alg default sha256;
	// also accepts sha1). Returns base64.
	e.register("sign", "crypto", func(L *lua.LState) int {
		data := []byte(luaToString(L, 1))
		key := []byte(L.CheckString(2))
		hashAlg := strings.ToLower(L.OptString(3, "sha256"))

		priv, err := parseRSAPrivateKey(key)
		if err != nil {
			L.RaiseError("sign error: %v", err)
			return 0
		}
		hashType, digest := digestFor(data, hashAlg)
		out, err := rsa.SignPKCS1v15(nil, priv, hashType, digest[:])
		if err != nil {
			L.RaiseError("sign error: %v", err)
			return 0
		}
		L.Push(lua.LString(base64.StdEncoding.EncodeToString(out)))
		return 1
	})

	// k.verify(data, signature, key[, alg]) — RSA signature verify; returns bool.
	e.register("verify", "crypto", func(L *lua.LState) int {
		data := []byte(luaToString(L, 1))
		sig, err := base64.StdEncoding.DecodeString(L.CheckString(2))
		if err != nil {
			L.RaiseError("verify error: invalid base64 signature: %v", err)
			return 0
		}
		key := []byte(L.CheckString(3))
		hashAlg := strings.ToLower(L.OptString(4, "sha256"))

		pub, err := parseRSAPublicKey(key)
		if err != nil {
			L.RaiseError("verify error: %v", err)
			return 0
		}
		hashType, digest := digestFor(data, hashAlg)
		err = rsa.VerifyPKCS1v15(pub, hashType, digest[:], sig)
		L.Push(lua.LBool(err == nil))
		return 1
	})
}

// aesCBCCrypt performs AES-CBC with PKCS#7 padding. decrypt=true reverses.
func aesCBCCrypt(key, iv, data []byte, decrypt bool) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	mode := cipher.NewCBCEncrypter(block, iv)
	if decrypt {
		modeDec := cipher.NewCBCDecrypter(block, iv)
		if len(data)%aes.BlockSize != 0 {
			return nil, fmt.Errorf("ciphertext length is not a multiple of block size")
		}
		out := make([]byte, len(data))
		modeDec.CryptBlocks(out, data)
		return pkcs7Unpad(out)
	}
	padded := pkcs7Pad(data, aes.BlockSize)
	out := make([]byte, len(padded))
	mode.CryptBlocks(out, padded)
	return out, nil
}

// pkcs7Pad pads data to a multiple of blockSize.
func pkcs7Pad(data []byte, blockSize int) []byte {
	padLen := blockSize - len(data)%blockSize
	pad := bytes.Repeat([]byte{byte(padLen)}, padLen)
	return append(data, pad...)
}

// pkcs7Unpad removes PKCS#7 padding.
func pkcs7Unpad(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty ciphertext after padding")
	}
	n := int(data[len(data)-1])
	if n == 0 || n > aes.BlockSize || n > len(data) {
		return nil, fmt.Errorf("invalid padding")
	}
	for _, b := range data[len(data)-n:] {
		if int(b) != n {
			return nil, fmt.Errorf("invalid padding")
		}
	}
	return data[:len(data)-n], nil
}

// parseRSAPublicKey parses a PEM RSA public key (PKCS#1 or SPKI).
func parseRSAPublicKey(pemBytes []byte) (*rsa.PublicKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found in public key")
	}
	if pub, err := x509.ParsePKCS1PublicKey(block.Bytes); err == nil {
		return pub, nil
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("cannot parse public key: %w", err)
	}
	pub, ok := parsed.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("public key is not RSA")
	}
	return pub, nil
}

// parseRSAPrivateKey parses a PEM RSA private key (PKCS#1 or PKCS#8).
func parseRSAPrivateKey(pemBytes []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found in private key")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("cannot parse private key: %w", err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key is not RSA")
	}
	return key, nil
}

// digestFor returns the hash type and digest for a named algorithm.
func digestFor(data []byte, alg string) (crypto.Hash, []byte) {
	switch strings.ToLower(alg) {
	case "sha1":
		s := sha1.Sum(data)
		return crypto.SHA1, s[:]
	default: // sha256
		s := sha256.Sum256(data)
		return crypto.SHA256, s[:]
	}
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
