// Package bindings implements the §5.4 FTP bindings (k.ftp_*). A minimal FTP
// client is implemented over net — control connection plus passive (EPSV/PASV)
// data connections — with no external dependency. Verbs: connect, set_cwd,
// get_file (RETR), put_file (STOR), file_exists (SIZE), create_dir (MKD),
// delete (DELE), rename (RNFR/RNTO), list (LIST), disconnect/quit.
package bindings

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yuin/gopher-lua"
)

// ftpHandle wraps an FTP control connection and its current directory.
type ftpHandle struct {
	conn net.Conn
	r    *bufio.Reader
	mu   sync.Mutex
}

// ftpHandles stores open FTP connections by ID.
var ftpHandles = make(map[string]*ftpHandle)
var ftpHandlesMu sync.Mutex

// registerFTP installs k.ftp_* bindings.
func registerFTP(e *Env) {
	// k.ftp_connect(host[, port, user, pw]) -> handle
	e.register("ftp_connect", "comm", func(L *lua.LState) int {
		host := L.CheckString(1)
		port := L.OptInt(2, 21)
		user := L.OptString(3, "anonymous")
		pw := L.OptString(4, "guest@")

		return runBlocking(e, L, func() (interface{}, error) {
			addr := net.JoinHostPort(host, strconv.Itoa(port))
			conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
			if err != nil {
				return nil, fmt.Errorf("ftp error: dial %s: %v", addr, err)
			}
			h := &ftpHandle{conn: conn, r: bufio.NewReader(conn)}
			done := false
			defer func() {
				if !done {
					conn.Close()
				}
			}()
			if _, err := ftpCmd(h, ""); err != nil {
				conn.Close()
				return nil, err
			}
			// USER typically replies 331 (need password), not 2xx.
			if _, err := ftpCmdAny(h, "USER "+user, 2, 3); err != nil {
				conn.Close()
				return nil, err
			}
			if _, err := ftpCmd(h, "PASS "+pw); err != nil {
				conn.Close()
				return nil, err
			}
			if _, err := ftpCmd(h, "TYPE I"); err != nil {
				conn.Close()
				return nil, err
			}
			id := fmt.Sprintf("ftp_%p", h)
			ftpHandlesMu.Lock()
			ftpHandles[id] = h
			ftpHandlesMu.Unlock()
			done = true
			return id, nil
		}, nil)
	})

	// k.ftp_set_cwd(handle, path)
	e.register("ftp_set_cwd", "comm", func(L *lua.LState) int {
		return ftpSimple(e, L, "CWD", 2)
	})

	// k.ftp_get_file(handle, remote, local) — RETR to local path.
	e.register("ftp_get_file", "comm", func(L *lua.LState) int {
		id := L.CheckString(1)
		remote := L.CheckString(2)
		local := L.CheckString(3)
		return runBlocking(e, L, func() (interface{}, error) {
			h, ok := lookupFTP(id)
			if !ok {
				return nil, fmt.Errorf("ftp error: handle not found: %s", id)
			}
			resolved, err := e.resolvePath(local)
			if err != nil {
				return nil, err
			}
			h.mu.Lock()
			defer h.mu.Unlock()
			return ftpTransfer(h, "RETR", remote, resolved, false)
		}, nil)
	})

	// k.ftp_put_file(handle, local, remote) — STOR from local path.
	e.register("ftp_put_file", "comm", func(L *lua.LState) int {
		id := L.CheckString(1)
		local := L.CheckString(2)
		remote := L.CheckString(3)
		return runBlocking(e, L, func() (interface{}, error) {
			h, ok := lookupFTP(id)
			if !ok {
				return nil, fmt.Errorf("ftp error: handle not found: %s", id)
			}
			resolved, err := e.resolvePath(local)
			if err != nil {
				return nil, err
			}
			h.mu.Lock()
			defer h.mu.Unlock()
			return ftpTransfer(h, "STOR", remote, resolved, true)
		}, nil)
	})

	// k.ftp_file_exists(handle, path) -> bool
	e.register("ftp_file_exists", "comm", func(L *lua.LState) int {
		id := L.CheckString(1)
		path := L.CheckString(2)
		return runBlocking(e, L, func() (interface{}, error) {
			h, ok := lookupFTP(id)
			if !ok {
				return nil, fmt.Errorf("ftp error: handle not found: %s", id)
			}
			h.mu.Lock()
			defer h.mu.Unlock()
			// SIZE returns 213 for files, 550 for missing/dirs.
			line, err := ftpRaw(h, "SIZE "+path)
			if err != nil {
				return false, nil
			}
			return strings.HasPrefix(line, "213"), nil
		}, nil)
	})

	// k.ftp_create_dir(handle, path) — MKD
	e.register("ftp_create_dir", "comm", func(L *lua.LState) int {
		return ftpSimple(e, L, "MKD", 2)
	})

	// k.ftp_delete(handle, path) — DELE (file) ; for a directory use delete_dir? spec says "Delete File-Folder"
	e.register("ftp_delete", "comm", func(L *lua.LState) int {
		return ftpSimple(e, L, "DELE", 2)
	})

	// k.ftp_rename(handle, from, to) — RNFR + RNTO
	e.register("ftp_rename", "comm", func(L *lua.LState) int {
		id := L.CheckString(1)
		from := L.CheckString(2)
		to := L.CheckString(3)
		return runBlocking(e, L, func() (interface{}, error) {
			h, ok := lookupFTP(id)
			if !ok {
				return nil, fmt.Errorf("ftp error: handle not found: %s", id)
			}
			h.mu.Lock()
			defer h.mu.Unlock()
			// RNFR replies 350 (intermediate) then RNTO completes with 2xx.
			if _, err := ftpCmdAny(h, "RNFR "+from, 3); err != nil {
				return nil, err
			}
			_, err := ftpCmd(h, "RNTO "+to)
			return nil, err
		}, nil)
	})

	// k.ftp_list(handle[, path]) -> 1-based table of entry names (LIST)
	e.register("ftp_list", "comm", func(L *lua.LState) int {
		id := L.CheckString(1)
		path := L.OptString(2, "")
		return runBlocking(e, L, func() (interface{}, error) {
			h, ok := lookupFTP(id)
			if !ok {
				return nil, fmt.Errorf("ftp error: handle not found: %s", id)
			}
			h.mu.Lock()
			defer h.mu.Unlock()
			data, err := ftpDataConn(h)
			if err != nil {
				return nil, err
			}
			if data != nil {
				defer data.Close()
			}
			cmd := "LIST"
			if path != "" {
				cmd += " " + path
			}
			if _, err := ftpCmdAny(h, cmd, 1, 2); err != nil {
				return nil, err
			}
			var entries []string
			if data != nil {
				sc := bufio.NewScanner(data)
				for sc.Scan() {
					line := sc.Text()
					entries = append(entries, ftpNameFromList(line))
				}
			}
			if _, err := ftpCmd(h, ""); err != nil {
				return nil, err
			}
			return entries, nil
		}, func(L *lua.LState, v interface{}) lua.LValue {
			entries, ok := v.([]string)
			if !ok {
				return lua.LNil
			}
			out := L.NewTable()
			for i, s := range entries {
				out.RawSetInt(i+1, lua.LString(s))
			}
			return out
		})
	})

	// k.ftp_disconnect(handle) / k.ftp_quit(handle) — QUIT + close.
	e.register("ftp_disconnect", "comm", func(L *lua.LState) int {
		return ftpQuit(e, L)
	})
}

// ftpSimple runs a no-data FTP command (CWD/MKD/DELE) with signature
// k.ftp_* (handle, path).
func ftpSimple(e *Env, L *lua.LState, verb string, argIdx int) int {
	id := L.CheckString(1)
	arg := L.CheckString(argIdx)
	return runBlocking(e, L, func() (interface{}, error) {
		h, ok := lookupFTP(id)
		if !ok {
			return nil, fmt.Errorf("ftp error: handle not found: %s", id)
		}
		h.mu.Lock()
		defer h.mu.Unlock()
		_, err := ftpCmd(h, verb+" "+arg)
		return nil, err
	}, nil)
}

// ftpQuit sends QUIT and closes the handle.
func ftpQuit(e *Env, L *lua.LState) int {
	id := L.CheckString(1)
	return runBlocking(e, L, func() (interface{}, error) {
		ftpHandlesMu.Lock()
		h, ok := ftpHandles[id]
		if ok {
			delete(ftpHandles, id)
		}
		ftpHandlesMu.Unlock()
		if !ok {
			return nil, fmt.Errorf("ftp error: handle not found: %s", id)
		}
		h.mu.Lock()
		defer h.mu.Unlock()
		_, _ = ftpCmd(h, "QUIT")
		return nil, h.conn.Close()
	}, nil)
}

// ftpCmd writes cmd (empty reads a response only) and asserts a 2xx code.
func ftpCmd(h *ftpHandle, cmd string) (string, error) {
	return ftpCmdAny(h, cmd, 2)
}

// ftpCmdAny is ftpCmd but accepts the given hundred-level status class
// (e.g. 2 for 2xx) or any of the listed classes.
func ftpCmdAny(h *ftpHandle, cmd string, classes ...int) (string, error) {
	line, err := ftpRaw(h, cmd)
	if err != nil {
		return "", err
	}
	hundred, _ := strconv.Atoi(first3(line))
	hundred /= 100
	for _, c := range classes {
		if hundred == c {
			return line, nil
		}
	}
	return "", fmt.Errorf("ftp error: %s", line)
}

// ftpRaw writes cmd and reads one control line (handling multi-line replies of
// the form "123-..." by draining until "123 ...").
func ftpRaw(h *ftpHandle, cmd string) (string, error) {
	if cmd != "" {
		if _, err := h.conn.Write([]byte(cmd + "\r\n")); err != nil {
			return "", fmt.Errorf("ftp error: write %q: %v", cmd, err)
		}
	}
	h.conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	line, err := h.r.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("ftp error: read: %v", err)
	}
	line = strings.TrimRight(line, "\r\n")
	// Multi-line reply: "123-..."
	if len(line) >= 4 && line[3] == '-' {
		prefix := line[:3]
		for {
			next, err := h.r.ReadString('\n')
			if err != nil {
				return "", fmt.Errorf("ftp error: read (multiline): %v", err)
			}
			next = strings.TrimRight(next, "\r\n")
			if strings.HasPrefix(next, prefix+" ") || next == prefix {
				break
			}
		}
	}
	return line, nil
}

// ftpDataConn opens a passive-mode data connection (EPSV first, fallback PASV)
// for the next transfer command. Returns a nil conn when the server offers no
// usable passive mode (the caller then reports an error).
func ftpDataConn(h *ftpHandle) (net.Conn, error) {
	peer := "127.0.0.1"
	if ta, ok := h.conn.RemoteAddr().(*net.TCPAddr); ok {
		peer = ta.IP.String()
	}
	// Try EPSV.
	if line, err := ftpRaw(h, "EPSV"); err == nil && strings.Contains(line, "|||") {
		if port, ok := parseEPSV(line); ok {
			return net.DialTimeout("tcp", fmt.Sprintf("%s:%d", peer, port), 10*time.Second)
		}
	}
	// Fallback PASV.
	line, err := ftpRaw(h, "PASV")
	if err != nil {
		return nil, err
	}
	host, port, ok := parsePASV(line)
	if !ok {
		return nil, fmt.Errorf("ftp error: cannot parse PASV: %s", line)
	}
	return net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), 10*time.Second)
}

// parseEPSV extracts the port from an EPSV "229 (|||port|)" response.
func parseEPSV(line string) (int, bool) {
	idx := strings.Index(line, "|||")
	if idx < 0 {
		return 0, false
	}
	rest := line[idx+3:]
	end := strings.IndexByte(rest, '|')
	if end < 0 {
		return 0, false
	}
	port, err := strconv.Atoi(rest[:end])
	if err != nil || port <= 0 {
		return 0, false
	}
	return port, true
}

// parsePASV extracts host:port from a PASV "227 (h1,h2,h3,h4,p1,p2)" response.
func parsePASV(line string) (string, int, bool) {
	start := strings.IndexByte(line, '(')
	end := strings.IndexByte(line, ')')
	if start < 0 || end < 0 || end <= start+1 {
		return "", 0, false
	}
	parts := strings.Split(line[start+1:end], ",")
	if len(parts) != 6 {
		return "", 0, false
	}
	nums := make([]int, 6)
	for i, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			return "", 0, false
		}
		nums[i] = n
	}
	host := fmt.Sprintf("%d.%d.%d.%d", nums[0], nums[1], nums[2], nums[3])
	port := nums[4]*256 + nums[5]
	return host, port, true
}

// ftpTransfer runs a STOR/RETR command over a passive data connection.
func ftpTransfer(h *ftpHandle, verb, remote, local string, upload bool) (interface{}, error) {
	data, err := ftpDataConn(h)
	if err != nil {
		return nil, err
	}
	if data != nil {
		defer data.Close()
	}
	// RETR/STOR reply 150 (preliminary) then 226 after transfer.
	if _, err := ftpCmdAny(h, verb+" "+remote, 1, 2); err != nil {
		return nil, err
	}
	if data == nil {
		return nil, fmt.Errorf("ftp error: no data connection")
	}
	data.SetDeadline(time.Now().Add(60 * time.Second))
	if upload {
		f, err := os.Open(local)
		if err != nil {
			return nil, fmt.Errorf("ftp error: open %s: %v", local, err)
		}
		defer f.Close()
		if _, err := io.Copy(data, f); err != nil {
			return nil, fmt.Errorf("ftp error: stor: %v", err)
		}
	} else {
		f, err := os.Create(local)
		if err != nil {
			return nil, fmt.Errorf("ftp error: create %s: %v", local, err)
		}
		defer f.Close()
		if _, err := io.Copy(f, data); err != nil {
			return nil, fmt.Errorf("ftp error: retr: %v", err)
		}
	}
	data.Close()
	if _, err := ftpCmd(h, ""); err != nil {
		return nil, err
	}
	return nil, nil
}

// ftpNameFromList pulls the trailing filename from a LIST line. Both the
// common "drwxr-xr-x 1 user group 123 Jan 1 12:00 name" format and the
// raw MS "MM-DD-YY  hh:mm  name" format are handled.
func ftpNameFromList(line string) string {
	// Best effort: last field is the name.
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return ""
	}
	return fields[len(fields)-1]
}

// first3 returns the first up-to-3 characters of a control line.
func first3(line string) string {
	if len(line) > 3 {
		return line[:3]
	}
	return line
}

// lookupFTP returns an open FTP handle by id.
func lookupFTP(id string) (*ftpHandle, bool) {
	ftpHandlesMu.Lock()
	defer ftpHandlesMu.Unlock()
	h, ok := ftpHandles[id]
	return h, ok
}