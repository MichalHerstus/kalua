package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"

	"kalua/internal/host"
)

// SmokeProbe describes an explicit HTTP probe run through the app's
// handle_http callback during a serve-mode smoke test. WantStatus 0 means
// expect 200; WantJSON asserts key/value pairs appear in the decoded body;
// WantContent asserts a substring of the raw body.
type SmokeProbe struct {
	Method      string         `json:"method"`
	Path        string         `json:"path"`
	WantStatus  int            `json:"want_status"`
	WantJSON    map[string]any `json:"want_json,omitempty"`
	WantContent string         `json:"want_contains,omitempty"`
}

// HTTPProbeResult reports the outcome of one HTTP probe.
type HTTPProbeResult struct {
	Method   string            `json:"method"`
	Path     string            `json:"path"`
	Status   int               `json:"status"`
	Expected int               `json:"expected"`
	OK       bool              `json:"ok"`
	Error    string            `json:"error,omitempty"`
	BodyHead string            `json:"body_head,omitempty"`
}

// SmokeResult is the full structured result of a serve-mode smoke test. It is
// JSON-serializable for the CLI --json output.
type SmokeResult struct {
	OK         bool              `json:"ok"`
	Mode       string            `json:"mode"`
	InitOK     bool              `json:"init_ok"`
	ShutdownOK bool              `json:"shutdown_ok"`
	HTTP       []HTTPProbeResult `json:"http,omitempty"`
	WS         string            `json:"ws,omitempty"` // "ok" or failure detail
	WSEcho     bool              `json:"ws_echo,omitempty"`
	TCP        string            `json:"tcp,omitempty"` // "ok" or failure detail
	TCPEcho    bool              `json:"tcp_echo,omitempty"`
	Errors     []string          `json:"errors,omitempty"`
}

// SmokeOptions tunes the probe set. WS/TCP probes are always added when the
// mode includes those transports; the HTTP probes come from the caller.
type SmokeOptions struct {
	HTTPProbes []SmokeProbe // explicit handle_http probes (healthz is implicit)
	WSSend     string       // if set, send this and require a reply (round-trip probe)
	TCPSend    string       // if set, send this and require a reply (round-trip probe)
}

// SmokeTest boots the app in serve mode on an ephemeral port, exercises
// init()/handle_http()/handle_ws()/handle_tcp() against real sockets, runs
// shutdown(), and reports a structured pass/fail. cfg.Port may be 0 (the
// bound addresses are discovered via HTTPAddr()/TCPAddr()).
func SmokeTest(cfg Config, opts SmokeOptions) SmokeResult {
	res := SmokeResult{Mode: cfg.Mode}
	if cfg.Host == "" {
		cfg.Host = "127.0.0.1"
	}
	// Quiet boot logs; errors still surface via cfg.Logger (level error).
	quiet := host.NewLogger(false)
	quiet.SetLevel(host.LevelError)
	cfg.Logger = quiet
	if cfg.Workers <= 0 {
		cfg.Workers = 1
	}

	s := NewServer(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan error, 1)
	go func() { runDone <- s.Run(ctx) }()

	waiter := waiter{deadline: time.Now().Add(8 * time.Second)}

	// Wait for the listeners to be bound.
	if httpRunning(cfg.Mode) {
		waiter.wait(func() bool { return s.HTTPAddr() != "" })
	}
	if modeHas(cfg.Mode, "tcp") {
		waiter.wait(func() bool { return s.TCPAddr() != "" })
	}

	// Boot failure (init() error, bind error, bad script) surfaces via Run.
	select {
	case err := <-runDone:
		res.Errors = append(res.Errors, err.Error())
		res.OK = false
		return res
	default:
	}
	res.InitOK = true

	if httpRunning(cfg.Mode) {
		base := "http://" + s.HTTPAddr()
		if !waitHealthy(base+"/healthz", 8*time.Second) {
			res.Errors = append(res.Errors, "server did not become ready on "+s.HTTPAddr())
		} else {
			res.HTTP = runHTTPProbes(base, opts.HTTPProbes)
			for _, p := range res.HTTP {
				if !p.OK {
					res.Errors = append(res.Errors,
						fmt.Sprintf("http %s %s: got status %d, want %d%s", p.Method, p.Path, p.Status, p.Expected, suffix(p.Error)))
				}
			}
		}
	}

	if modeHas(cfg.Mode, "ws") {
		if s.HTTPAddr() == "" {
			res.WS = "no http listener for ws upgrade"
			res.Errors = append(res.Errors, res.WS)
		} else {
			ok, echo, detail := probeWS("ws://"+s.HTTPAddr()+"/ws", opts.WSSend)
			res.WSEcho = echo
			if ok {
				res.WS = "ok"
			} else {
				res.WS = detail
				res.Errors = append(res.Errors, "ws probe failed: "+detail)
			}
		}
	}

	if modeHas(cfg.Mode, "tcp") {
		if s.TCPAddr() == "" {
			res.TCP = "no tcp listener"
			res.Errors = append(res.Errors, res.TCP)
		} else {
			ok, echo, detail := probeTCP(s.TCPAddr(), opts.TCPSend)
			res.TCPEcho = echo
			if ok {
				res.TCP = "ok"
			} else {
				res.TCP = detail
				res.Errors = append(res.Errors, "tcp probe failed: "+detail)
			}
		}
	}

	// Shut down: Run must return cleanly so shutdown() had the chance to run.
	cancel()
	select {
	case err := <-runDone:
		if err != nil {
			res.Errors = append(res.Errors, "serve run error: "+err.Error())
		}
	case <-time.After(8 * time.Second):
		res.Errors = append(res.Errors, "server did not stop within 8s")
	}
	res.ShutdownOK = s.ShutdownRan()

	res.OK = len(res.Errors) == 0
	return res
}

type waiter struct{ deadline time.Time }

func (w *waiter) wait(ready func() bool) {
	for !ready() && time.Now().Before(w.deadline) {
		time.Sleep(25 * time.Millisecond)
	}
}

func suffix(e string) string {
	if e == "" {
		return ""
	}
	return " (" + e + ")"
}

// httpRunning reports whether the mode starts an HTTP listener (http or ws;
// ws upgrades over the same listener).
func httpRunning(mode string) bool { return modeHas(mode, "http") || modeHas(mode, "ws") }

func modeHas(mode, m string) bool {
	for i := 0; i <= len(mode)-len(m); i++ {
		if mode[i:i+len(m)] == m && (i == 0 || mode[i-1] == ',') &&
			(i+len(m) == len(mode) || mode[i+len(m)] == ',') {
			return true
		}
	}
	return false
}

// waitHealthy polls an HTTP endpoint until it returns 200 or the timeout
// passes.
func waitHealthy(url string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 1200 * time.Millisecond}
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return true
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// runHTTPProbes executes the caller's probes plus an implicit healthz
// connectivity check.
func runHTTPProbes(base string, probes []SmokeProbe) []HTTPProbeResult {
	health := SmokeProbe{Method: "GET", Path: "/healthz", WantStatus: http.StatusOK}
	all := append([]SmokeProbe{health}, probes...)
	out := make([]HTTPProbeResult, 0, len(all))
	for _, p := range all {
		out = append(out, doHTTPProbe(base, p))
	}
	return out
}

func doHTTPProbe(base string, p SmokeProbe) HTTPProbeResult {
	method := p.Method
	if method == "" {
		method = "GET"
	}
	path := p.Path
	if path == "" {
		path = "/"
	}
	want := p.WantStatus
	if want == 0 {
		want = http.StatusOK
	}
	res := HTTPProbeResult{Method: method, Path: path, Expected: want}

	req, err := http.NewRequest(method, base+path, nil)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	res.Status = resp.StatusCode
	res.BodyHead = head(body)
	res.OK = res.Status == want
	if !res.OK {
		return res
	}

	if p.WantContent != "" && !strings.Contains(string(body), p.WantContent) {
		res.Error = fmt.Sprintf("body does not contain %q", p.WantContent)
		res.OK = false
		return res
	}
	if len(p.WantJSON) > 0 {
		var decoded map[string]any
		if err := json.Unmarshal(body, &decoded); err != nil {
			res.Error = "response is not JSON: " + err.Error()
			res.OK = false
			return res
		}
		for k, wantV := range p.WantJSON {
			gotV := decoded[k]
			if !jsonEqual(gotV, wantV) {
				res.Error = fmt.Sprintf("json[%q] = %v, want %v", k, gotV, wantV)
				res.OK = false
				break
			}
		}
	}
	return res
}

func jsonEqual(a, b any) bool {
	as, _ := json.Marshal(a)
	bs, _ := json.Marshal(b)
	return string(as) == string(bs)
}

func head(b []byte) string {
	h := string(b)
	if len(h) > 120 {
		h = h[:120]
	}
	return h
}

// probeWS opens a WebSocket and waits for the open event to round-trip through
// the handler. Passing = the connection survives a short read timeout (no
// handler crash on open). When sendText is non-empty, a text message is sent
// and any reply proves the text handler round-trip.
func probeWS(url, sendText string) (ok, reply bool, detail string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ws, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		return false, false, "dial: " + err.Error()
	}
	defer ws.Close(websocket.StatusNormalClosure, "")

	if sendText != "" {
		if err := ws.Write(ctx, websocket.MessageText, []byte(sendText)); err != nil {
			return false, false, "write: " + err.Error()
		}
		_, data, err := ws.Read(ctx)
		if err != nil {
			return false, false, "no reply to " + strconv.Quote(sendText) + ": " + err.Error()
		}
		return true, true, "replied (" + string(head(data)) + ")"
	}

	readCtx, readCancel := context.WithTimeout(context.Background(), 1200*time.Millisecond)
	defer readCancel()
	if _, data, err := ws.Read(readCtx); err == nil {
		return true, true, "connected (" + string(head(data)) + ")"
	}
	return true, false, "connected"
}

// probeTCP dials the TCP endpoint and waits for the open event to round-trip
// through the handler. Passing = the connection stays alive briefly (no
// handler crash on open). When sendText is non-empty, a text message is sent
// and any reply proves the text handler round-trip.
func probeTCP(addr, sendText string) (ok, reply bool, detail string) {
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return false, false, "dial: " + err.Error()
	}
	defer conn.Close()

	if sendText != "" {
		conn.SetDeadline(time.Now().Add(5 * time.Second))
		if _, err := conn.Write([]byte(sendText)); err != nil {
			return false, false, "write: " + err.Error()
		}
		buf := make([]byte, 512)
		n, err := conn.Read(buf)
		if err != nil {
			return false, false, "no reply to " + strconv.Quote(sendText) + ": " + err.Error()
		}
		return true, true, "replied (" + string(head(buf[:n])) + ")"
	}

	conn.SetDeadline(time.Now().Add(1200 * time.Millisecond))
	buf := make([]byte, 256)
	if n, err := conn.Read(buf); err == nil {
		return true, true, "connected (" + string(head(buf[:n])) + ")"
	}
	return true, false, "connected"
}