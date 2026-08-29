// Package bindings implements the filesystem bindings (Phase 5 - Data groups).
package bindings

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/yuin/gopher-lua"

	"kalua/internal/common"
)

// fileHandle wraps an open file for use in Lua.
type fileHandle struct {
	f *os.File
	r *bufio.Reader
}

// fileHandles stores open file handles by ID (Go-side only, like dbHandles).
var fileHandles = make(map[string]*fileHandle)
var fileHandlesMu sync.Mutex

// registerFiles installs the k.file_* bindings.
func registerFiles(e *Env) {
	// k.file_open(path, mode) -> handle id
	e.register("file_open", "files", func(L *lua.LState) int {
		path, err := e.resolvePath(L.CheckString(1))
		if err != nil {
			L.RaiseError("%v", err)
			return 0
		}
		mode := L.OptString(2, "r")

		flags, ok := openFlags(mode)
		if !ok {
			L.RaiseError("file error: invalid mode %q", mode)
			return 0
		}
		f, err := os.OpenFile(path, flags, 0o644)
		if err != nil {
			L.RaiseError("file error: cannot open %s: %v", path, err)
			return 0
		}

		h := &fileHandle{f: f}
		if isReadMode(mode) {
			h.r = bufio.NewReader(f)
		}
		id := fmt.Sprintf("file_%p", h)
		fileHandlesMu.Lock()
		fileHandles[id] = h
		fileHandlesMu.Unlock()

		L.Push(lua.LString(id))
		return 1
	})

	// k.file_read(handle [, count]) -> string ("" at EOF)
	e.register("file_read", "files", func(L *lua.LState) int {
		h, ok := lookupFileHandle(L, 1)
		if !ok {
			return 0
		}
		count := L.OptInt(2, -1)
		var data []byte
		var err error
		if count < 0 {
			data, err = io.ReadAll(h.f)
		} else {
			buf := make([]byte, count)
			n := 0
			n, err = h.f.Read(buf)
			data = buf[:n]
		}
		if err != nil && err != io.EOF {
			L.RaiseError("file error: read failed: %v", err)
			return 0
		}
		L.Push(lua.LString(data))
		return 1
	})

	// k.file_read_line(handle) -> string (nil at EOF)
	e.register("file_read_line", "files", func(L *lua.LState) int {
		h, ok := lookupFileHandle(L, 1)
		if !ok {
			return 0
		}
		var line string
		line, err := h.r.ReadString('\n')
		if err != nil && err != io.EOF {
			L.RaiseError("file error: read failed: %v", err)
			return 0
		}
		if err == io.EOF && len(line) == 0 {
			L.Push(lua.LNil)
			return 1
		}
		line = strings.TrimSuffix(line, "\n")
		line = strings.TrimSuffix(line, "\r")
		L.Push(lua.LString(line))
		return 1
	})

	// k.file_write(handle, data)
	e.register("file_write", "files", func(L *lua.LState) int {
		h, ok := lookupFileHandle(L, 1)
		if !ok {
			return 0
		}
		data := []byte(luaToString(L, 2))
		if _, err := h.f.Write(data); err != nil {
			L.RaiseError("file error: write failed: %v", err)
			return 0
		}
		return 0
	})

	// k.file_close(handle)
	e.register("file_close", "files", func(L *lua.LState) int {
		id := L.CheckString(1)
		fileHandlesMu.Lock()
		h, ok := fileHandles[id]
		if ok {
			delete(fileHandles, id)
		}
		fileHandlesMu.Unlock()
		if !ok {
			L.RaiseError("file error: handle not found: %s", id)
			return 0
		}
		h.f.Close()
		return 0
	})

	// k.file_load(path) -> contents (async; bounded by MaxFileSize)
	e.register("file_load", "files", func(L *lua.LState) int {
		path := L.CheckString(1)
		return runBlocking(e, L, func() (interface{}, error) {
			resolved, err := e.resolvePath(path)
			if err != nil {
				return nil, err
			}
			fi, err := os.Stat(resolved)
			if err != nil {
				return nil, fmt.Errorf("file error: cannot stat %s: %w", path, err)
			}
			if fi.Size() > e.maxFileSize {
				return nil, fmt.Errorf("file error: %s exceeds max file size (%d bytes)", path, e.maxFileSize)
			}
			data, err := os.ReadFile(resolved)
			if err != nil {
				return nil, fmt.Errorf("file error: cannot read %s: %w", path, err)
			}
			return string(data), nil
		}, nil)
	})

	// k.file_save(path, data) (async; atomic temp+rename)
	e.register("file_save", "files", func(L *lua.LState) int {
		path := L.CheckString(1)
		data := []byte(luaToString(L, 2))
		return runBlocking(e, L, func() (interface{}, error) {
			resolved, err := e.resolvePath(path)
			if err != nil {
				return nil, err
			}
			if err := writeFileAtomic(resolved, data); err != nil {
				return nil, fmt.Errorf("file error: cannot write %s: %w", path, err)
			}
			return nil, nil
		}, nil)
	})

	e.register("file_copy", "files", func(L *lua.LState) int {
		return fdOp(L, e, L.CheckString(1), L.CheckString(2), func(src, dst string) error {
			return copyFile(src, dst)
		})
	})

	e.register("file_move", "files", func(L *lua.LState) int {
		return fdOp(L, e, L.CheckString(1), L.CheckString(2), func(src, dst string) error {
			return os.Rename(src, dst)
		})
	})

	e.register("file_delete", "files", func(L *lua.LState) int {
		path, err := e.resolvePath(L.CheckString(1))
		if err != nil {
			L.RaiseError("%v", err)
			return 0
		}
		if err := os.Remove(path); err != nil {
			L.RaiseError("file error: cannot delete %s: %v", path, err)
			return 0
		}
		return 0
	})

	e.register("file_exists", "files", func(L *lua.LState) int {
		path, err := e.resolvePath(L.CheckString(1))
		if err != nil {
			L.RaiseError("%v", err)
			return 0
		}
		_, statErr := os.Stat(path)
		L.Push(lua.LBool(statErr == nil))
		return 1
	})

	// k.file_mkdir(path [, parents])
	e.register("file_mkdir", "files", func(L *lua.LState) int {
		path, err := e.resolvePath(L.CheckString(1))
		if err != nil {
			L.RaiseError("%v", err)
			return 0
		}
		recursive := L.OptBool(2, false)
		perm := os.FileMode(0o755)
		if recursive {
			_, err = os.Stat(path)
			if err == nil {
				return 0 // already exists
			}
			err = os.MkdirAll(path, perm)
		} else {
			err = os.Mkdir(path, perm)
		}
		if err != nil {
			L.RaiseError("file error: cannot create directory %s: %v", path, err)
			return 0
		}
		return 0
	})

	// k.file_list(dir) -> 1-based table of entry names (sorted)
	e.register("file_list", "files", func(L *lua.LState) int {
		path, err := e.resolvePath(L.CheckString(1))
		if err != nil {
			L.RaiseError("%v", err)
			return 0
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			L.RaiseError("file error: cannot list %s: %v", path, err)
			return 0
		}
		names := make([]string, 0, len(entries))
		for _, en := range entries {
			names = append(names, en.Name())
		}
		sort.Strings(names)
		tbl := L.NewTable()
		for i, n := range names {
			tbl.RawSetInt(i+1, lua.LString(n))
		}
		L.Push(tbl)
		return 1
	})

	// k.file_info(path) -> table {name, size, is_dir, modified}
	e.register("file_info", "files", func(L *lua.LState) int {
		path, err := e.resolvePath(L.CheckString(1))
		if err != nil {
			L.RaiseError("%v", err)
			return 0
		}
		fi, err := os.Stat(path)
		if err != nil {
			L.RaiseError("file error: cannot stat %s: %v", path, err)
			return 0
		}
		tbl := L.NewTable()
		tbl.RawSetString("name", lua.LString(filepath.Base(path)))
		tbl.RawSetString("size", lua.LNumber(fi.Size()))
		tbl.RawSetString("is_dir", lua.LBool(fi.IsDir()))
		tbl.RawSetString("modified", lua.LNumber(fi.ModTime().Unix()))
		L.Push(tbl)
		return 1
	})
}

// lookupFileHandle resolves the handle id at stack index 1, raising on failure.
func lookupFileHandle(L *lua.LState, idx int) (*fileHandle, bool) {
	id := L.CheckString(idx)
	fileHandlesMu.Lock()
	h, ok := fileHandles[id]
	fileHandlesMu.Unlock()
	if !ok {
		L.RaiseError("file error: handle not found: %s", id)
		return nil, false
	}
	return h, true
}

// fdOp resolves src/dst, runs the op, and reports errors.
func fdOp(L *lua.LState, e *Env, srcArg, dstArg string, op func(src, dst string) error) int {
	src, err := e.resolvePath(srcArg)
	if err != nil {
		L.RaiseError("%v", err)
		return 0
	}
	dst, err := e.resolvePath(dstArg)
	if err != nil {
		L.RaiseError("%v", err)
		return 0
	}
	if err := op(src, dst); err != nil {
		L.RaiseError("file error: %v", err)
		return 0
	}
	return 0
}

func openFlags(mode string) (int, bool) {
	switch mode {
	case "r":
		return os.O_RDONLY, true
	case "r+":
		return os.O_RDWR, true
	case "w":
		return os.O_WRONLY | os.O_CREATE | os.O_TRUNC, true
	case "w+":
		return os.O_RDWR | os.O_CREATE | os.O_TRUNC, true
	case "a":
		return os.O_WRONLY | os.O_CREATE | os.O_APPEND, true
	case "a+":
		return os.O_RDWR | os.O_CREATE | os.O_APPEND, true
	}
	return 0, false
}

func isReadMode(mode string) bool {
	return mode == "r" || mode == "r+" || mode == "a+"
}

// runBlocking runs fn on the session worker in web mode (yielding the caller),
// or synchronously in test mode. conv converts the result to a Lua value on
// the caller's state; nil means the default Go→Lua conversion.
func runBlocking(e *Env, L *lua.LState, fn func() (interface{}, error), conv func(*lua.LState, interface{}) lua.LValue) int {
	sess := e.App.Session()
	if sess != nil {
		sess.RunAsync(L, func() {}, fn, conv)
		return L.Yield(lua.LNil)
	}

	result, err := fn()
	if err != nil {
		L.RaiseError("%v", err)
		return 0
	}
	if conv != nil {
		L.Push(conv(L, result))
	} else {
		L.Push(common.DefaultConv(L, result))
	}
	return 1
}

// copyFile copies src to dst preserving permissions.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	fi, err := os.Stat(src)
	if err == nil {
		os.Chmod(dst, fi.Mode())
	}
	return nil
}

// writeFileAtomic writes data to a temp file in the same directory then
// renames it over dst, so a crash never leaves a partially-written file.
func writeFileAtomic(dst string, data []byte) error {
	dir := filepath.Dir(dst)
	tmp, err := os.CreateTemp(dir, ".kalua-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		if tmpName != "" {
			os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmpName, dst); err != nil {
		return err
	}
	tmpName = ""
	return nil
}

// resolvePath resolves path against the sandbox root and verifies the result
// stays inside an allowed root (working directory + configured AllowFS roots).
func (e *Env) resolvePath(path string) (string, error) {
	p := path
	if !filepath.IsAbs(p) {
		p = filepath.Join(e.workdir, p)
	}
	p = filepath.Clean(p)
	resolved, err := evalSymlinksBestEffort(p)
	if err != nil {
		return "", fmt.Errorf("file error: cannot resolve %s: %w", path, err)
	}

	roots := []string{e.workdir}
	roots = append(roots, e.allowFS...)
	for _, root := range roots {
		r, err := evalSymlinksBestEffort(filepath.Clean(root))
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(r, resolved)
		if err != nil {
			continue
		}
		if rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..") {
			return resolved, nil
		}
	}
	return "", fmt.Errorf("file error: filesystem access denied: %s is outside the allowed roots", path)
}

// evalSymlinksBestEffort resolves symlinks on the longest existing prefix of p
// (so paths whose final components do not exist yet still canonicalize parent
// symlinks) and returns the fully-resolved path.
func evalSymlinksBestEffort(p string) (string, error) {
	resolved, err := filepath.EvalSymlinks(p)
	if err == nil {
		return resolved, nil
	}
	dir := filepath.Dir(p)
	if dir == p {
		return p, nil
	}
	resolvedDir, err := evalSymlinksBestEffort(dir)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolvedDir, filepath.Base(p)), nil
}

// luaToString strings any Lua value into bytes for file/json/crypto input.
func luaToString(L *lua.LState, idx int) string {
	return L.Get(idx).String()
}
