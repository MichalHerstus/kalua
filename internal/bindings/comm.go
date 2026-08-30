// Package bindings implements the §5.4 communications tier-2 bindings:
// k.socket_* (TCP client sockets), plus k.ftp_* / k.webservice_run which live
// in ftp.go and soap.go. Ping helpers sit beside the flow bindings.
package bindings

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/yuin/gopher-lua"
)

// socketHandle wraps an open TCP connection.
type socketHandle struct {
	conn net.Conn
	r    *bufio.Reader
	mode string // reserved for TLS later
}

// socketHandles stores open sockets by ID (Go-side only, like fileHandles).
var socketHandles = make(map[string]*socketHandle)
var socketHandlesMu sync.Mutex

// registerComm installs k.socket_* bindings.
func registerComm(e *Env) {
	// k.socket_open(host, port[, timeout_ms]) -> handle
	e.register("socket_open", "comm", func(L *lua.LState) int {
		host := L.CheckString(1)
		port := L.CheckInt(2)
		timeout := time.Duration(L.OptInt(3, 5000)) * time.Millisecond
		return runBlocking(e, L, func() (interface{}, error) {
			addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
			conn, err := net.DialTimeout("tcp", addr, timeout)
			if err != nil {
				return nil, fmt.Errorf("socket error: %v", err)
			}
			h := &socketHandle{conn: conn, r: bufio.NewReader(conn)}
			id := fmt.Sprintf("sock_%p", h)
			socketHandlesMu.Lock()
			socketHandles[id] = h
			socketHandlesMu.Unlock()
			return id, nil
		}, nil)
	})

	// k.socket_write(handle, data) -> bytes written
	e.register("socket_write", "comm", func(L *lua.LState) int {
		id := L.CheckString(1)
		data := []byte(luaToString(L, 2))
		return runBlocking(e, L, func() (interface{}, error) {
			h, ok := lookupSocket(id)
			if !ok {
				return nil, fmt.Errorf("socket error: handle not found: %s", id)
			}
			n, err := h.conn.Write(data)
			if err != nil {
				return nil, fmt.Errorf("socket error: %v", err)
			}
			return n, nil
		}, nil)
	})

	// k.socket_read(handle[, count]) -> string (nil at EOF/close)
	e.register("socket_read", "comm", func(L *lua.LState) int {
		id := L.CheckString(1)
		count := L.OptInt(2, -1)
		return runBlocking(e, L, func() (interface{}, error) {
			h, ok := lookupSocket(id)
			if !ok {
				return nil, fmt.Errorf("socket error: handle not found: %s", id)
			}
			var buf []byte
			if count < 0 {
				var sb strings.Builder
				tmp := make([]byte, 4096)
				for {
					n, err := h.r.Read(tmp)
					if n > 0 {
						sb.Write(tmp[:n])
					}
					if err != nil {
						if err == io.EOF {
							break
						}
						return nil, fmt.Errorf("socket error: %v", err)
					}
				}
				buf = []byte(sb.String())
			} else {
				buf = make([]byte, count)
				n, err := io.ReadFull(h.r, buf)
				buf = buf[:n]
				if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
					return nil, fmt.Errorf("socket error: %v", err)
				}
			}
			return string(buf), nil
		}, nil)
	})

	// k.socket_read_line(handle) -> string (nil at EOF)
	e.register("socket_read_line", "comm", func(L *lua.LState) int {
		id := L.CheckString(1)
		return runBlocking(e, L, func() (interface{}, error) {
			h, ok := lookupSocket(id)
			if !ok {
				return nil, fmt.Errorf("socket error: handle not found: %s", id)
			}
			line, err := h.r.ReadString('\n')
			if err != nil && err != io.EOF {
				return nil, fmt.Errorf("socket error: %v", err)
			}
			if err == io.EOF && len(line) == 0 {
				return "", io.EOF
			}
			line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
			return line, nil
		}, func(L *lua.LState, v interface{}) lua.LValue {
			if err, ok := v.(error); ok && err == io.EOF {
				return lua.LNil
			}
			return lua.LString(v.(string))
		})
	})

	// k.socket_close(handle)
	e.register("socket_close", "comm", func(L *lua.LState) int {
		id := L.CheckString(1)
		return runBlocking(e, L, func() (interface{}, error) {
			socketHandlesMu.Lock()
			h, ok := socketHandles[id]
			if ok {
				delete(socketHandles, id)
			}
			socketHandlesMu.Unlock()
			if !ok {
				return nil, fmt.Errorf("socket error: handle not found: %s", id)
			}
			return nil, h.conn.Close()
		}, nil)
	})
}

// lookupSocket returns an open socket by id.
func lookupSocket(id string) (*socketHandle, bool) {
	socketHandlesMu.Lock()
	defer socketHandlesMu.Unlock()
	h, ok := socketHandles[id]
	return h, ok
}