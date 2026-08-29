package vm

import (
	"fmt"
	"os"
	"strings"

	"github.com/yuin/gopher-lua"
)

// LoadSource compiles Lua source named name without executing it. Compilation
// errors are wrapped with the source name for readable diagnostics.
func LoadSource(L *lua.LState, name, src string) (*lua.LFunction, error) {
	fn, err := L.Load(strings.NewReader(src), name)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return fn, nil
}

// LoadFile reads and compiles a .lua file. Reader errors (missing file,
// permissions) surface unchanged so the CLI can classify them as I/O errors.
func LoadFile(L *lua.LState, path string) (*lua.LFunction, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return LoadSource(L, path, string(src))
}
