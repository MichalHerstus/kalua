//go:build !wasm

// Package bindings implements the login control verification logic.
package bindings

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/yuin/gopher-lua"
	"golang.org/x/crypto/pbkdf2"

	"kalua/internal/coerce"
)

// LoginOptions holds the parsed and validated options for a login control.
type LoginOptions struct {
	DB               string // Named DB handle or runtime id
	Table            string // Users table name
	LoginColumn      string // Column matched against login input
	PasswordColumn   string // Column holding the stored hash/secret
	SaltColumn       string // Per-user salt column (required for pbkdf2 unless static salt given)
	Hash             string // "pbkdf2" | "sha256" | "plain"
	StaticSalt       string // Static salt; overrides SaltColumn
	Iterations       int    // pbkdf2 rounds
	KeyLen           int    // pbkdf2 derived key length in bytes
	ErrorMessage     string // Inline error shown on failed attempt
	MaxAttempts      int    // Wrong-password retries before control stops retrying
}

// LoginOptsFromTable parses and validates login control options from a Lua table.
// Applies defaults and validates identifiers. Returns error if required db is missing
// or any identifier contains invalid characters.
func LoginOptsFromTable(L *lua.LState, ctrl *lua.LTable) (LoginOptions, error) {
	var o LoginOptions

	dbVal := ctrl.RawGetString("db")
	if dbVal == lua.LNil || dbVal.String() == "" {
		return o, fmt.Errorf("login control requires 'db' option (named handle or k.connect_db id)")
	}
	o.DB = dbVal.String()

	// Table name (default "users")
	tableVal := ctrl.RawGetString("table")
	if tableVal != lua.LNil && tableVal.String() != "" {
		o.Table = tableVal.String()
	} else {
		o.Table = "users"
	}

	// Login column (default "login")
	loginColVal := ctrl.RawGetString("login_column")
	if loginColVal != lua.LNil && loginColVal.String() != "" {
		o.LoginColumn = loginColVal.String()
	} else {
		o.LoginColumn = "login"
	}

	// Password column (default "password")
	passwordColVal := ctrl.RawGetString("password_column")
	if passwordColVal != lua.LNil && passwordColVal.String() != "" {
		o.PasswordColumn = passwordColVal.String()
	} else {
		o.PasswordColumn = "password"
	}

	// Salt column (optional, required for pbkdf2)
	saltColVal := ctrl.RawGetString("salt_column")
	if saltColVal != lua.LNil && saltColVal.String() != "" {
		o.SaltColumn = saltColVal.String()
	}

	// Hash algorithm (default "pbkdf2")
	hashVal := ctrl.RawGetString("hash")
	if hashVal != lua.LNil && hashVal.String() != "" {
		o.Hash = strings.ToLower(hashVal.String())
	} else {
		o.Hash = "pbkdf2"
	}

	// Static salt (overrides salt_column)
	saltVal := ctrl.RawGetString("salt")
	if saltVal != lua.LNil && saltVal.String() != "" {
		o.StaticSalt = saltVal.String()
	}

	// Iterations (default 100000)
	iterVal := ctrl.RawGetString("iterations")
	if iterVal != lua.LNil {
		o.Iterations = int(lua.LVAsNumber(iterVal))
	} else {
		o.Iterations = 100000
	}

	// Key length (default 32)
	keyLenVal := ctrl.RawGetString("keylen")
	if keyLenVal != lua.LNil {
		o.KeyLen = int(lua.LVAsNumber(keyLenVal))
	} else {
		o.KeyLen = 32
	}

	// Error message (default "Invalid login or password")
	errMsgVal := ctrl.RawGetString("error_message")
	if errMsgVal != lua.LNil && errMsgVal.String() != "" {
		o.ErrorMessage = errMsgVal.String()
	} else {
		o.ErrorMessage = "Invalid login or password"
	}

	// Max attempts (default 3)
	attemptsVal := ctrl.RawGetString("attempts")
	if attemptsVal != lua.LNil {
		o.MaxAttempts = int(lua.LVAsNumber(attemptsVal))
	} else {
		o.MaxAttempts = 3
	}

	// Validate identifiers
	if !isValidIdentifier(o.Table) {
		return o, fmt.Errorf("invalid table name: %s", o.Table)
	}
	if !isValidIdentifier(o.LoginColumn) {
		return o, fmt.Errorf("invalid login_column: %s", o.LoginColumn)
	}
	if !isValidIdentifier(o.PasswordColumn) {
		return o, fmt.Errorf("invalid password_column: %s", o.PasswordColumn)
	}
	if o.SaltColumn != "" && !isValidIdentifier(o.SaltColumn) {
		return o, fmt.Errorf("invalid salt_column: %s", o.SaltColumn)
	}

	// Validate hash algorithm
	switch o.Hash {
	case "pbkdf2", "sha256", "plain":
	default:
		return o, fmt.Errorf("unsupported hash algorithm: %s (must be pbkdf2, sha256, or plain)", o.Hash)
	}

	// For pbkdf2, require a salt source
	if o.Hash == "pbkdf2" && o.StaticSalt == "" && o.SaltColumn == "" {
		return o, fmt.Errorf("hash=pbkdf2 requires either 'salt' or 'salt_column' option")
	}

	// Validate iterations and keylen for pbkdf2
	if o.Hash == "pbkdf2" {
		if o.Iterations <= 0 {
			o.Iterations = 100000
		}
		if o.KeyLen <= 0 {
			o.KeyLen = 32
		}
	}

	return o, nil
}

// LookupLoginUser looks up a user by login in the users table.
// Returns the first matching row as a map, or nil if not found.
func LookupLoginUser(L *lua.LState, ctrl *lua.LTable, login string) (map[string]interface{}, error) {
	opts, err := LoginOptsFromTable(L, ctrl)
	if err != nil {
		return nil, err
	}

	h := getDBHandle(L, opts.DB)
	if h == nil {
		return nil, fmt.Errorf("db handle not found: %s", opts.DB)
	}

	// Build query: SELECT * FROM table WHERE login_column = ?
	query := fmt.Sprintf("SELECT * FROM %s WHERE %s = %s", opts.Table, opts.LoginColumn, h.Placeholder(1))

	rows, err := h.Query(query, login)
	if err != nil {
		return nil, err
	}

	if len(rows) == 0 {
		return nil, nil // Not found
	}

	return rows[0], nil
}

// VerifyLoginPassword verifies the provided password against the stored hash
// in the user row according to the configured hash algorithm.
// Returns true if verification succeeds, false otherwise.
func VerifyLoginPassword(L *lua.LState, ctrl *lua.LTable, row map[string]interface{}, password string) bool {
	opts, err := LoginOptsFromTable(L, ctrl)
	if err != nil {
		return false
	}

	storedHash, ok := row[opts.PasswordColumn].(string)
	if !ok {
		return false
	}

	var salt string
	if opts.StaticSalt != "" {
		salt = opts.StaticSalt
	} else if opts.SaltColumn != "" {
		if s, ok := row[opts.SaltColumn].(string); ok {
			salt = s
		}
	}

	switch opts.Hash {
	case "pbkdf2":
		if salt == "" {
			return false
		}
		derived := pbkdf2.Key([]byte(password), []byte(salt), opts.Iterations, opts.KeyLen, sha256.New)
		computedHash := hex.EncodeToString(derived)
		return subtle.ConstantTimeCompare([]byte(storedHash), []byte(computedHash)) == 1

	case "sha256":
		var computedHash string
		if salt != "" {
			h := sha256.New()
			h.Write([]byte(password + salt))
			computedHash = hex.EncodeToString(h.Sum(nil))
		} else {
			h := sha256.New()
			h.Write([]byte(password))
			computedHash = hex.EncodeToString(h.Sum(nil))
		}
		return subtle.ConstantTimeCompare([]byte(storedHash), []byte(computedHash)) == 1

	case "plain":
		return subtle.ConstantTimeCompare([]byte(storedHash), []byte(password)) == 1
	}

	return false
}

// LoginUserFromRow converts a database row map into a Lua table.
// Uses the same conversion logic as k.grid.get_row (GoValueToLua).
func LoginUserFromRow(L *lua.LState, row map[string]interface{}) *lua.LTable {
	tbl := L.NewTable()
	for k, v := range row {
		tbl.RawSetString(k, GoValueToLua(L, v))
	}
	return tbl
}

// GoValueToLua converts a Go value to a Lua value.
// Mirrors the conversion used in k.grid.get_row and k.db_select.
func GoValueToLua(L *lua.LState, v interface{}) lua.LValue {
	if v == nil {
		return lua.LNil
	}
	switch t := v.(type) {
	case bool:
		return lua.LBool(t)
	case int:
		return lua.LNumber(t)
	case int64:
		return lua.LNumber(t)
	case float64:
		return lua.LNumber(t)
	case string:
		return lua.LString(t)
	case []byte:
		return lua.LString(string(t))
	default:
		// Fallback: use coerce.Stringify for unknown types
		return lua.LString(coerce.Stringify(v))
	}
}