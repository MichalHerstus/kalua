// Package bindings implements the §5.7 POP3 bindings (k.pop3_*). A minimal
// POP3 client over a raw TCP connection — no external dependency. Verbs map
// to the Kalipso surface: connect, stat, list, retrieve (retr), delete (dele),
// noop, and quit/disconnect.
package bindings

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yuin/gopher-lua"
)

// pop3Handle wraps a POP3 control connection.
type pop3Handle struct {
	conn net.Conn
	r    *bufio.Reader
	mu   sync.Mutex
}

// pop3Handles stores open POP3 connections by ID.
var pop3Handles = make(map[string]*pop3Handle)
var pop3HandlesMu sync.Mutex

// registerPop3 installs k.pop3_* bindings.
func registerPop3(e *Env) {
	// k.pop3_connect{host,port,user,pw,tls} -> handle
	e.register("pop3_connect", "email", func(L *lua.LState) int {
		opts := L.CheckTable(1)
		host := opts.RawGetString("host").String()
		port := opts.RawGetString("port")
		portInt := 110
		if port != lua.LNil {
			if n, ok := v(port).(float64); ok {
				portInt = int(n)
			}
		}
		user := opts.RawGetString("user").String()
		pw := opts.RawGetString("pw").String()
		tlsOn := lua.LVAsBool(opts.RawGetString("tls"))

		return runBlocking(e, L, func() (interface{}, error) {
			addr := net.JoinHostPort(host, strconv.Itoa(portInt))
			raw, err := net.DialTimeout("tcp", addr, 10*time.Second)
			if err != nil {
				return nil, fmt.Errorf("pop3 error: dial %s: %v", addr, err)
			}
			done := false
			defer func() {
				if !done {
					raw.Close()
				}
			}()
			if tlsOn {
				tc := tls.Client(raw, &tls.Config{ServerName: host})
				if err := tc.Handshake(); err != nil {
					raw.Close()
					return nil, fmt.Errorf("pop3 error: tls: %v", err)
				}
				raw = tc
			}

			h := &pop3Handle{conn: raw, r: bufio.NewReader(raw)}
			if _, err := pop3Cmd(h, ""); err != nil { // read greeting
				raw.Close()
				return nil, err
			}
			if user != "" {
				if _, err := pop3Cmd(h, "USER "+user); err != nil {
					raw.Close()
					return nil, err
				}
				if _, err := pop3Cmd(h, "PASS "+pw); err != nil {
					raw.Close()
					return nil, err
				}
			}
			id := fmt.Sprintf("pop3_%p", h)
			pop3HandlesMu.Lock()
			pop3Handles[id] = h
			pop3HandlesMu.Unlock()
			done = true
			return id, nil
		}, nil)
	})

	// k.pop3_stat(handle) -> {count, size}
	e.register("pop3_stat", "email", func(L *lua.LState) int {
		id := L.CheckString(1)
		return runBlocking(e, L, func() (interface{}, error) {
			h, ok := lookupPop3(id)
			if !ok {
				return nil, fmt.Errorf("pop3 error: handle not found: %s", id)
			}
			h.mu.Lock()
			defer h.mu.Unlock()
			line, err := pop3Cmd(h, "STAT")
			if err != nil {
				return nil, err
			}
			parts := strings.Fields(line)
			count, size := 0, 0
			if len(parts) >= 2 {
				fmt.Sscanf(parts[0], "%d", &count)
				fmt.Sscanf(parts[1], "%d", &size)
			}
			return map[string]interface{}{"count": count, "size": size}, nil
		}, nil)
	})

	// k.pop3_list(handle) -> 1-based table of {id,size}
	e.register("pop3_list", "email", func(L *lua.LState) int {
		id := L.CheckString(1)
		return runBlocking(e, L, func() (interface{}, error) {
			h, ok := lookupPop3(id)
			if !ok {
				return nil, fmt.Errorf("pop3 error: handle not found: %s", id)
			}
			h.mu.Lock()
			defer h.mu.Unlock()
			lines, err := pop3Multiline(h, "LIST")
			if err != nil {
				return nil, err
			}
			return parsePop3List(lines), nil
		}, func(L *lua.LState, v interface{}) lua.LValue {
			return stringMapSliceToLua(L, v)
		})
	})

	// k.pop3_retr(handle, index) -> message text
	e.register("pop3_retr", "email", func(L *lua.LState) int {
		id := L.CheckString(1)
		idx := L.CheckInt(2)
		return runBlocking(e, L, func() (interface{}, error) {
			h, ok := lookupPop3(id)
			if !ok {
				return nil, fmt.Errorf("pop3 error: handle not found: %s", id)
			}
			h.mu.Lock()
			defer h.mu.Unlock()
			lines, err := pop3Multiline(h, fmt.Sprintf("RETR %d", idx))
			if err != nil {
				return nil, err
			}
			return strings.Join(lines, "\r\n"), nil
		}, nil)
	})

	// k.pop3_dele(handle, index) — marks message for deletion.
	e.register("pop3_dele", "email", func(L *lua.LState) int {
		id := L.CheckString(1)
		idx := L.CheckInt(2)
		return runBlocking(e, L, func() (interface{}, error) {
			h, ok := lookupPop3(id)
			if !ok {
				return nil, fmt.Errorf("pop3 error: handle not found: %s", id)
			}
			h.mu.Lock()
			defer h.mu.Unlock()
			_, err := pop3Cmd(h, fmt.Sprintf("DELE %d", idx))
			return nil, err
		}, nil)
	})

	// k.pop3_noop(handle) — keeps the connection alive.
	e.register("pop3_noop", "email", func(L *lua.LState) int {
		id := L.CheckString(1)
		return runBlocking(e, L, func() (interface{}, error) {
			h, ok := lookupPop3(id)
			if !ok {
				return nil, fmt.Errorf("pop3 error: handle not found: %s", id)
			}
			h.mu.Lock()
			defer h.mu.Unlock()
			_, err := pop3Cmd(h, "NOOP")
			return nil, err
		}, nil)
	})

	// k.pop3_quit(handle) — sends QUIT and closes.
	e.register("pop3_quit", "email", func(L *lua.LState) int {
		id := L.CheckString(1)
		return runBlocking(e, L, func() (interface{}, error) {
			pop3HandlesMu.Lock()
			h, ok := pop3Handles[id]
			if ok {
				delete(pop3Handles, id)
			}
			pop3HandlesMu.Unlock()
			if !ok {
				return nil, fmt.Errorf("pop3 error: handle not found: %s", id)
			}
			h.mu.Lock()
			defer h.mu.Unlock()
			_, _ = pop3Cmd(h, "QUIT")
			return nil, h.conn.Close()
		}, nil)
	})
}

// pop3Cmd writes cmd (skipped when empty) and reads one response line,
// requiring a "+OK" prefix. Returns the tail after "+OK".
func pop3Cmd(h *pop3Handle, cmd string) (string, error) {
	if cmd != "" {
		if _, err := h.conn.Write([]byte(cmd + "\r\n")); err != nil {
			return "", fmt.Errorf("pop3 error: write %q: %v", cmd, err)
		}
	}
	h.conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	line, err := h.r.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("pop3 error: read: %v", err)
	}
	line = strings.TrimRight(line, "\r\n")
	if strings.HasPrefix(line, "+OK") {
		return strings.TrimSpace(strings.TrimPrefix(line, "+OK")), nil
	}
	return "", fmt.Errorf("pop3 error: %s", line)
}

// pop3Multiline issues a command expecting a "+OK" multiline response and
// returns the lines (terminated by a lone ".").
func pop3Multiline(h *pop3Handle, cmd string) ([]string, error) {
	if _, err := pop3Cmd(h, cmd); err != nil {
		return nil, err
	}
	var lines []string
	for {
		h.conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		line, err := h.r.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("pop3 error: read: %v", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "." {
			break
		}
		lines = append(lines, strings.TrimPrefix(line, "."))
	}
	return lines, nil
}

// parsePop3List interprets LIST output lines "id size" into [{id=..,size=..}].
func parsePop3List(lines []string) []map[string]interface{} {
	var out []map[string]interface{}
	for _, ln := range lines {
		parts := strings.Fields(ln)
		if len(parts) < 2 {
			continue
		}
		var id, size int
		fmt.Sscanf(parts[0], "%d", &id)
		fmt.Sscanf(parts[1], "%d", &size)
		out = append(out, map[string]interface{}{"id": id, "size": size})
	}
	return out
}

// stringMapSliceToLua converts []map[string]interface{} into a Lua table.
func stringMapSliceToLua(L *lua.LState, v interface{}) lua.LValue {
	items, ok := v.([]map[string]interface{})
	if !ok {
		return lua.LNil
	}
	out := L.NewTable()
	for i, m := range items {
		row := L.NewTable()
		for k, val := range m {
			row.RawSetString(k, lvalue(val))
		}
		out.RawSetInt(i+1, row)
	}
	return out
}

// lookupPop3 returns an open POP3 handle by id.
func lookupPop3(id string) (*pop3Handle, bool) {
	pop3HandlesMu.Lock()
	defer pop3HandlesMu.Unlock()
	h, ok := pop3Handles[id]
	return h, ok
}