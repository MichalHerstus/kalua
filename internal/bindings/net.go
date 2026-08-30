// Package bindings implements the §5.2 net/locale/param helpers used by the
// flow bindings (k.net_ok, k.ping, k.locale, k.param_set/get).
package bindings

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// paramsMu guards the params file so param_set/get are safe across the worker
// goroutine and re-entries (run mode serializes them, but serve mode may not).
var paramsMu sync.Mutex

// paramsPath is the file that stores k.param_set/get values. It lives in the
// sandbox home (working directory), matching the "app-side file" wording in
// spec §5.2.
func paramsPath(e *Env) string {
	dir := e.workdir
	if dir == "" {
		dir, _ = os.Getwd()
	}
	return filepath.Join(dir, ".kalua.params.json")
}

// loadParams reads the persisted param map.
func (e *Env) loadParams() map[string]string {
	paramsMu.Lock()
	defer paramsMu.Unlock()
	out := map[string]string{}
	data, err := os.ReadFile(paramsPath(e))
	if err != nil {
		return out
	}
	_ = json.Unmarshal(data, &out)
	return out
}

// setParam persists a single key/value atomically.
func (e *Env) setParam(key, value string) {
	paramsMu.Lock()
	defer paramsMu.Unlock()
	params := map[string]string{}
	if data, err := os.ReadFile(paramsPath(e)); err == nil {
		_ = json.Unmarshal(data, &params)
	}
	params[key] = value
	if data, err := json.Marshal(params); err == nil {
		_ = writeFileAtomic(paramsPath(e), data)
	}
}

// getParam reads a single persisted param ("" when unset).
func (e *Env) getParam(key string) string {
	return e.loadParams()[key]
}

// locale returns the session/browser locale, defaulting to "en-US" until the
// web session seeds it from client_info.
func (e *Env) locale() string {
	return "en-US"
}

// netOKProbe is the host k.net_ok dials to detect internet reachability.
// Overridable in tests.
var netOKProbe = "1.1.1.1:80"

func netOK(timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", netOKProbe, timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// pingHost measures the TCP connect latency to a host in milliseconds.
// Without ICMP privileges this is a TCP-based reachability check; k.ping is a
// best-effort latency probe (spec §5.4).
func pingHost(host string, timeout time.Duration) (float64, bool) {
	if !strings.Contains(host, ":") {
		host = host + ":80"
	}
	start := time.Now()
	conn, err := net.DialTimeout("tcp", host, timeout)
	if err != nil {
		return 0, false
	}
	conn.Close()
	return float64(time.Since(start).Milliseconds()), true
}