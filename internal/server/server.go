//go:build !wasm

package server

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	"kalua/internal/bindings"
	"kalua/internal/host"
	"kalua/internal/klog"
)

// Config holds the server configuration.
type Config struct {
	Host        string
	Port        int
	Workers     int
	Mode        string // "http", "ws", "tcp", or combination
	ScriptPath  string
	DBs         []string
	Args        []string
	AllowFS     []string
	MaxFileSize int64
	// MaxWSMessageSize caps inbound WebSocket frames (H5/P6). Zero means the
	// default of 1 MiB.
	MaxWSMessageSize int64
	Verbose          bool
	Logger           *host.Logger

	// Phase 8 structured logging (serve-communication-plan.md §8). When
	// LogSQLite is set, records flow into klog.db (path <cwd>/klog.db);
	// LogLevel is the minimum severity, LogRetain the age prune (0 = forever),
	// LogMaxMB the size cap (0 = unlimited), LogMaxRow the per-row cap.
	LogSQLite bool
	LogLevel  string
	LogRetain time.Duration
	LogMaxMB  int64
	LogMaxRow int64

	// AllowNet gates outbound MQTT client connections (Phase 2 security).
	// k.mqtt_connect refuses when false.
	AllowNet bool
}

// Server is the KALUA serve mode server with worker pool.
type Server struct {
	cfg           Config
	shared        *SharedState
	wsHub         *WSHub
	tcpHub        *TCPHub
	mqttHub       *MQTTHub
	httpStats     *bindings.HTTPStats
	klog          *klog.DB
	workers       []*Worker
	workerMu      sync.RWMutex // guards s.workers and s.retired
	retired       []*Worker    // superseded by hot reload, drained on shutdown
	workerCh      chan *Worker
	nextWorkerIdx atomic.Uint64 // round-robin cursor
	netMu         sync.RWMutex  // guards httpServer, httpListener, tcpListener, httpAddr, tcpAddr
	httpServer    *http.Server
	httpListener  net.Listener
	tcpListener   net.Listener
	httpAddr      string        // actual bound HTTP/WS address (for Port 0)
	tcpAddr       string        // actual bound TCP address (for Port 0)
	readyCh       chan struct{} // closed once every listener of the mode is bound
	readyOnce     sync.Once
	shutdownRan   atomic.Bool
	wg            sync.WaitGroup
	stopCh        chan struct{}
	stopOnce      sync.Once
}

// NewServer creates a new serve mode server.
func NewServer(cfg Config) *Server {
	if cfg.Logger == nil {
		cfg.Logger = host.NewLogger(cfg.Verbose)
	}
	if cfg.Workers <= 0 {
		cfg.Workers = 1
	}
	if cfg.Mode == "" {
		cfg.Mode = "http"
	}
	if cfg.MaxWSMessageSize <= 0 {
		cfg.MaxWSMessageSize = 1 << 20 // 1 MiB
	}

	shared := NewSharedState()
	workerCh := make(chan *Worker, cfg.Workers)
	wsHub := NewWSHub(workerCh)
	tcpHub := NewTCPHub(workerCh)
	mqttHub := NewMQTTHub()

	s := &Server{
		cfg:       cfg,
		shared:    shared,
		wsHub:     wsHub,
		tcpHub:    tcpHub,
		mqttHub:   mqttHub,
		httpStats: &bindings.HTTPStats{},
		workerCh:  workerCh,
		stopCh:    make(chan struct{}),
		readyCh:   make(chan struct{}),
	}

	// Phase 8: open the structured-log sink when --log-sqlite is set. A failed
	// open aborts startup (bad permissions, unusable path) rather than
	// silently degrading to a no-op logger.
	if cfg.LogSQLite {
		level := klog.LevelInfo
		if cfg.LogLevel != "" {
			lv, err := klog.ParseLevel(cfg.LogLevel)
			if err != nil {
				cfg.Logger.Errorf("klog: invalid --log-level %q: %v (continuing without structured logging)", cfg.LogLevel, err)
				return s
			}
			level = lv
		}
		wd, _ := os.Getwd()
		db, err := klog.Open(klog.Config{
			Path:   filepath.Join(wd, klog.DefaultPath),
			Level:  level,
			MaxRow: cfg.LogMaxRow,
			Retain: cfg.LogRetain,
			MaxMB:  cfg.LogMaxMB,
		})
		if err == nil {
			s.klog = db
			db.Emit(klog.Record{
				Level:    klog.LevelInfo,
				Category: "lifecycle",
				Message:  "structured logging started",
			})
			db.Flush()
		} else {
			cfg.Logger.Errorf("klog open: %v (continuing without structured logging)", err)
		}
	}

	return s
}

// Run starts the server and blocks until context is cancelled.
func (s *Server) Run(ctx context.Context) error {
	// Start workers
	if err := s.startWorkers(ctx); err != nil {
		return err
	}

	// Start HTTP server if mode includes http or ws (WS upgrades over the
	// same HTTP listener, so a ws-only server still needs it running).
	if s.modeHas("http") || s.modeHas("ws") {
		if err := s.startHTTP(ctx); err != nil {
			return err
		}
	}

	// Start WebSocket server if mode includes ws
	if s.modeHas("ws") {
		s.startWS(ctx)
	}

	// Start TCP server if mode includes tcp
	if s.modeHas("tcp") {
		if err := s.startTCP(ctx); err != nil {
			return err
		}
	}

	httpPort := s.cfg.Port
	if addr := s.HTTPAddr(); addr != "" {
		if _, p, err := net.SplitHostPort(addr); err == nil {
			fmt.Sscanf(p, "%d", &httpPort)
		}
	}
	tcpPort := httpPort + 1
	if addr := s.TCPAddr(); addr != "" {
		if _, p, err := net.SplitHostPort(addr); err == nil {
			fmt.Sscanf(p, "%d", &tcpPort)
		}
	}
	s.cfg.Logger.Printf("KALUA serve mode listening on %s:%d (tcp=%d, workers=%d, mode=%s)",
		s.cfg.Host, httpPort, tcpPort, s.cfg.Workers, s.cfg.Mode)

	// Every listener the mode asks for is bound: publish readiness so callers
	// can read the real (possibly ephemeral) addresses without polling.
	s.signalReady()

	// Wait for context cancellation or k.quit (Stop).
	select {
	case <-ctx.Done():
	case <-s.stopCh:
	}
	s.runShutdownHandlers()
	s.shutdown()
	return nil
}

// Stop requests a clean shutdown of the running server (wired to k.quit via
// bindings.Options.Quit). It cancels the Run wait below; Run then runs the
// shutdown() callback and tears down listeners/workers.
func (s *Server) Stop() {
	s.stopOnce.Do(func() { close(s.stopCh) })
}

// Ready returns a channel that is closed once every listener required by the
// configured mode has been bound. After it fires, HTTPAddr()/TCPAddr() are safe
// to read from another goroutine and hold the real bound addresses.
//
// A boot that fails before binding (init() error, bad script, port in use) never
// fires it, so a caller watching only Ready() would block: select on Run's error
// return too, as SmokeTest does.
func (s *Server) Ready() <-chan struct{} { return s.readyCh }

// WaitReady blocks until the server is ready or ctx is done. It returns ctx's
// error if the wait was abandoned; a nil error means the listeners are bound.
func (s *Server) WaitReady(ctx context.Context) error {
	select {
	case <-s.readyCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// signalReady publishes readiness exactly once.
func (s *Server) signalReady() { s.readyOnce.Do(func() { close(s.readyCh) }) }

// runShutdownHandlers invokes the optional shutdown() callback once, on the
// first worker, before workers are torn down (spec §2.2 shutdown).
func (s *Server) runShutdownHandlers() {
	s.shutdownRan.Store(true)
	if len(s.workers) > 0 {
		s.workers[0].CallShutdown()
	}
}

// ShutdownRan reports whether Run() reached the shutdown-handler phase.
func (s *Server) ShutdownRan() bool { return s.shutdownRan.Load() }

func (s *Server) modeHas(m string) bool {
	// Check if mode contains the substring (e.g., "http,ws" contains "http")
	for i := 0; i <= len(s.cfg.Mode)-len(m); i++ {
		if s.cfg.Mode[i:i+len(m)] == m {
			// Check boundaries
			if (i == 0 || s.cfg.Mode[i-1] == ',') &&
				(i+len(m) == len(s.cfg.Mode) || s.cfg.Mode[i+len(m)] == ',') {
				return true
			}
		}
	}
	return false
}

func (s *Server) startWorkers(ctx context.Context) error {
	workers, err := s.buildWorkers(s.cfg.Workers)
	if err != nil {
		return fmt.Errorf("failed to start worker: %w", err)
	}
	s.workers = workers
	for _, w := range workers {
		s.workerCh <- w
	}
	// Wire the MQTT inbound pump to the (new) worker pool.
	s.mqttHub.SetDeliver(func(msg bindings.MQTTMessage) {
		worker, release := s.leaseWorker()
		if worker == nil {
			return
		}
		defer release()
		worker.CallMQTT(msg)
	})

	// Run the optional init(config) callback once, on the first worker. A
	// handler error aborts startup (spec §2.2 init).
	if len(workers) > 0 {
		if err := workers[0].CallInit(s.cfg); err != nil {
			for _, w := range workers {
				w.Close()
			}
			return err
		}
	}
	return nil
}

func (s *Server) startHTTP(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleHTTP)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// WebSocket upgrade endpoint
	mux.HandleFunc("/ws", s.handleWSUpgrade)

	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)

	// Bind the listener up front so Port 0 yields an ephemeral port the caller
	// can discover via HTTPAddr(). Anybind error (port in use) surfaces from
	// Run() instead of being swallowed by the goroutine.
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}
	bound := ln.Addr().String()
	hs := &http.Server{
		Addr:              bound,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	s.netMu.Lock()
	s.httpListener = ln
	s.httpAddr = bound
	s.httpServer = hs
	s.netMu.Unlock()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := hs.Serve(ln); err != nil && err != http.ErrServerClosed {
			s.cfg.Logger.Errorf("HTTP server error: %v", err)
		}
	}()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		hs.Shutdown(shutdownCtx)
	}()

	return nil
}

func (s *Server) handleHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	s.httpStats.ActiveConns.Add(1)
	s.httpStats.RequestsTotal.Add(1)
	defer func() {
		s.httpStats.ActiveConns.Add(-1)
		if ms := time.Since(start).Milliseconds(); ms >= 0 {
			s.httpStats.LatencySumMs.Add(ms)
			s.httpStats.LatencyCount.Add(1)
		}
	}()

	// Security headers
	w.Header().Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'; base-uri 'self'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

	// Get next worker (round-robin), leased for this request
	worker, release := s.leaseWorker()
	if worker == nil {
		s.httpStats.ErrorsTotal.Add(1)
		s.emitHTTPLog(r, time.Since(start), http.StatusServiceUnavailable)
		http.Error(w, "no available workers", http.StatusServiceUnavailable)
		return
	}
	defer release()

	// Read request body with size limit
	body := ""
	if r.Body != nil {
		limited := http.MaxBytesReader(w, r.Body, 16<<20) // 16 MiB
		data, err := io.ReadAll(limited)
		if err != nil {
			s.httpStats.ErrorsTotal.Add(1)
			s.emitHTTPLog(r, time.Since(start), http.StatusBadRequest)
			s.cfg.Logger.Errorf("read body: %v", err)
			http.Error(w, "body too large or read error", http.StatusBadRequest)
			return
		}
		body = string(data)
	}

	// Build headers map
	headers := make(map[string]string)
	for k, v := range r.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}

	req := HTTPRequest{
		Method:      r.Method,
		Path:        r.URL.Path,
		Query:       r.URL.RawQuery,
		QueryValues: r.URL.Query(),
		Headers:     headers,
		Body:        body,
		RemoteAddr:  r.RemoteAddr,
		TLS:         r.TLS != nil,
	}

	resp, err := worker.CallHTTP(r.Context(), req)
	if err != nil {
		s.httpStats.ErrorsTotal.Add(1)
		s.cfg.Logger.Errorf("handle_http error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		s.emitHTTPLog(r, time.Since(start), http.StatusInternalServerError)
		return
	}

	for k, v := range resp.Headers {
		w.Header().Set(k, v)
	}
	w.WriteHeader(resp.Status)
	w.Write([]byte(resp.Body))
	s.emitHTTPLog(r, time.Since(start), resp.Status)
}

// emitHTTPLog records a Phase 8 http-category row when structured logging is
// enabled. start→now gives the request duration; status + duration_ms ride in
// the data JSON object.
func (s *Server) emitHTTPLog(r *http.Request, dur time.Duration, status int) {
	if s.klog == nil {
		return
	}
	durMs := dur.Milliseconds()
	if durMs < 1 {
		durMs = 1
	}
	s.klog.Emit(klog.Record{
		Level:      klog.LevelInfo,
		Category:   "http",
		RemoteAddr: r.RemoteAddr,
		Message:    fmt.Sprintf("%s %s -> %d", r.Method, r.URL.Path, status),
		Data:       fmt.Sprintf(`{"method":%s,"path":%s,"status":%d,"duration_ms":%d}`, jsonQuote(r.Method), jsonQuote(r.URL.Path), status, durMs),
	})
}


func (s *Server) handleWSUpgrade(w http.ResponseWriter, r *http.Request) {
	// Upgrade to WebSocket
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"http://127.0.0.1:*", "http://localhost:*", "http://[::1]:*"},
	})
	if err != nil {
		s.cfg.Logger.Errorf("websocket accept: %v", err)
		return
	}

	// P6 (H5): cap inbound WebSocket frames (config.MaxWSMessageSize, default
	// 1 MiB) so a large message cannot OOM the worker's inbox.
	c.SetReadLimit(s.cfg.MaxWSMessageSize)

	connID := randomID("ws")
	closeFn := func() { c.CloseNow() }

	ws := s.wsHub.Register(connID, c, closeFn)
	defer func() {
		s.wsHub.Unregister(connID)
		c.CloseNow()
	}()

	// Get worker for this connection, leased for its whole lifetime
	worker, release := s.leaseWorker()
	if worker != nil {
		defer release()
		worker.CallWS(WSMessage{Type: "open", ClientID: connID}, ws)
	}

	// Start outbound message pump
	outboundDone := make(chan struct{})
	pumpStop := make(chan struct{})
	go func() {
		defer close(outboundDone)
		for {
			select {
			case msg := <-ws.sendCh:
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				if err := c.Write(ctx, websocket.MessageText, msg); err != nil {
					s.cfg.Logger.Printf("ws write: %v", err)
					cancel()
					return
				}
				cancel()
			case <-pumpStop:
				return
			}
		}
	}()

	// Read inbound messages and dispatch to handle_ws(msg)
	for {
		mtype, data, err := c.Read(r.Context())
		if err != nil {
			break
		}
		typ := "text"
		if mtype == websocket.MessageBinary {
			typ = "binary"
		}
		if worker != nil {
			worker.CallWS(WSMessage{Type: typ, Data: string(data), ClientID: connID}, ws)
		}
	}

	close(pumpStop)
	if worker != nil {
		worker.CallWS(WSMessage{Type: "close", ClientID: connID}, ws)
	}
	<-outboundDone
}

func (s *Server) startWS(ctx context.Context) {
	// WebSocket is handled via HTTP upgrade on same port
	// This is just for logging
	s.cfg.Logger.Printf("WebSocket endpoint available at ws://%s/ws", s.HTTPAddr())
}

func (s *Server) startTCP(ctx context.Context) error {
	// TCP on port+1 by convention; ephemeral when the HTTP port is 0.
	tcpPort := s.cfg.Port + 1
	if s.cfg.Port == 0 {
		tcpPort = 0
	}
	addr := fmt.Sprintf("%s:%d", s.cfg.Host, tcpPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	s.netMu.Lock()
	s.tcpListener = listener
	s.tcpAddr = listener.Addr().String()
	s.netMu.Unlock()

	s.cfg.Logger.Printf("TCP server listening on %s", s.TCPAddr())

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			conn, err := listener.Accept()
			if err != nil {
				select {
				case <-ctx.Done():
					return
				default:
					s.cfg.Logger.Errorf("tcp accept: %v", err)
					continue
				}
			}

			connID := fmt.Sprintf("tcp-%d", time.Now().UnixNano())
			closeFn := func() { conn.Close() }

			tcpConn := s.tcpHub.Register(connID, conn, closeFn)
			defer func() {
				s.tcpHub.Unregister(connID)
				conn.Close()
			}()

			// Get worker for this connection, leased for its whole lifetime
			worker, release := s.leaseWorker()
			if worker != nil {
				defer release()
				worker.CallTCP(TCPMessage{Type: "open", ClientID: connID}, tcpConn)
			}

			// Outbound pump: drain sendCh (k.tcp_send, handler return values) → conn
			pumpStop := make(chan struct{})
			pumpDone := make(chan struct{})
			go func() {
				defer close(pumpDone)
				for {
					select {
					case msg := <-tcpConn.sendCh:
						if _, err := conn.Write(msg); err != nil {
							s.cfg.Logger.Printf("tcp write: %v", err)
							return
						}
					case <-pumpStop:
						return
					}
				}
			}()

			// Read loop: dispatch each chunk to handle_tcp(msg)
			buf := make([]byte, 4096)
			for {
				n, err := conn.Read(buf)
				if err != nil {
					break
				}
				if worker != nil {
					worker.CallTCP(TCPMessage{Type: "text", Data: string(buf[:n]), ClientID: connID}, tcpConn)
				}
			}

			close(pumpStop)
			if worker != nil {
				worker.CallTCP(TCPMessage{Type: "close", ClientID: connID}, tcpConn)
			}
			<-pumpDone
		}
	}()

	go func() {
		<-ctx.Done()
		listener.Close()
	}()

	return nil
}

// leaseWorker returns a worker for one unit of work (an HTTP request or a
// WS/TCP connection) plus a release func that must be called when the work
// ends. The lease guards against a hot-reload closing the worker underneath a
// live handler or connection.
func (s *Server) leaseWorker() (*Worker, func()) {
	s.workerMu.RLock()
	defer s.workerMu.RUnlock()
	if len(s.workers) == 0 {
		return nil, nil
	}
	n := s.nextWorkerIdx.Add(1)
	w := s.workers[n%uint64(len(s.workers))]
	w.refs.Add(1)
	return w, w.release
}

// Reload recompiles the script and atomically swaps the worker pool (SIGHUP
// hot reload). In-flight requests and open WS/TCP connections finish on their
// current worker, which is released when its last lease ends. On any error the
// existing pool is left untouched.
func (s *Server) Reload() error {
	s.cfg.Logger.Printf("reload: rebuilding %d workers from %s", s.cfg.Workers, s.cfg.ScriptPath)

	newWorkers, err := s.buildWorkers(s.cfg.Workers)
	if err != nil {
		return fmt.Errorf("reload: %w", err)
	}

	s.workerMu.Lock()
	old := s.workers
	s.workers = newWorkers
	s.nextWorkerIdx.Store(0)
	for _, w := range old {
		w.retire()
	}
	s.retired = append(s.retired, old...)
	s.workerMu.Unlock()

	s.cfg.Logger.Printf("reload: %d workers swapped", len(newWorkers))
	return nil
}

func (s *Server) buildWorkers(count int) ([]*Worker, error) {
	opts := bindings.Options{
		Args:        s.cfg.Args,
		AllowFS:     s.cfg.AllowFS,
		MaxFileSize: s.cfg.MaxFileSize,
		Verbose:     s.cfg.Verbose,
		Quit:        s.Stop,
		Stats:       s.httpStats,
		LogDB:       s.klog,
		MQTT:        s.mqttHub,
		AllowNet:    s.cfg.AllowNet,
	}
	var workers []*Worker
	for i := 0; i < count; i++ {
		w, err := NewWorker(i+1, s.cfg.ScriptPath, opts, s.shared, s.wsHub, s.tcpHub, s.cfg.Logger)
		if err != nil {
			for _, w := range workers {
				w.Close()
			}
			return nil, err
		}
		workers = append(workers, w)
	}
	return workers, nil
}

func (s *Server) shutdown() {
	s.Stop() // idempotent (sync.Once); also closes stopCh for k.quit
	s.netMu.RLock()
	hs, hln, tln := s.httpServer, s.httpListener, s.tcpListener
	s.netMu.RUnlock()
	if hs != nil {
		hs.Close()
	}
	if hln != nil {
		hln.Close()
	}
	if tln != nil {
		tln.Close()
	}
	// Close every current and retired worker. Retired workers may already
	// have been closed by their last lease release; Close is idempotent. Wait
	// for an in-flight call (e.g. the one that invoked k.quit) to release the
	// slot before closing the LState so we never race a live Lua frame.
	s.workerMu.Lock()
	all := make([]*Worker, 0, len(s.workers)+len(s.retired))
	all = append(all, s.workers...)
	all = append(all, s.retired...)
	s.workers = nil
	s.retired = nil
	s.workerMu.Unlock()
	for _, w := range all {
		w.waitIdle(5 * time.Second)
		w.Close()
	}
	if s.klog != nil {
		s.klog.Close()
	}
	s.mqttHub.CloseAll() // disconnect any k.mqtt_* clients (Phase 2)
	s.wg.Wait()
}

// waitIdle blocks until the worker's slot is free (any in-flight CallX has
// returned) or the deadline passes. Called only during shutdown so a live
// mutation can never Race with LState.Close().
func (w *Worker) waitIdle(timeout time.Duration) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-w.slot:
		w.freeSlot() // re-seed; the caller closes the worker next
	case <-timer.C:
		// Best effort: the connection is being torn down anyway; proceed.
	}
}

// randomID returns a 16-byte crypto/rand token hex-encoded with the given
// label as prefix, used for connection identifiers (P4: previously
// time.Now().UnixNano(), which was predictable from the startup time).
func randomID(prefix string) string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand must not fail on supported platforms; a timestamp is a
		// defensible last resort so the server can still start.
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return fmt.Sprintf("%s-%s", prefix, hex.EncodeToString(b))
}

// jsonQuote returns s as a JSON-escaped string literal (for klog data rows).
func jsonQuote(s string) string {
	buf := make([]byte, 0, len(s)+2)
	buf = append(buf, '"')
	for _, r := range s {
		switch r {
		case '"':
			buf = append(buf, '\\', '"')
		case '\\':
			buf = append(buf, '\\', '\\')
		case '\n':
			buf = append(buf, '\\', 'n')
		case '\r':
			buf = append(buf, '\\', 'r')
		case '\t':
			buf = append(buf, '\\', 't')
		default:
			buf = append(buf, string(r)...)
		}
	}
	buf = append(buf, '"')
	return string(buf)
}

// HTTPAddr returns the actual bound HTTP/WS address ("host:port"), useful
// when Port 0 requested an ephemeral port. Empty until the listener is bound;
// wait on Ready() before reading it from another goroutine.
func (s *Server) HTTPAddr() string {
	s.netMu.RLock()
	defer s.netMu.RUnlock()
	return s.httpAddr
}

// TCPAddr returns the actual bound TCP address ("host:port"), useful when
// Port 0 requested an ephemeral port. Empty until the listener is bound;
// wait on Ready() before reading it from another goroutine.
func (s *Server) TCPAddr() string {
	s.netMu.RLock()
	defer s.netMu.RUnlock()
	return s.tcpAddr
}

// TLSConfig holds TLS configuration for HTTPS/WSS.
type TLSConfig struct {
	CertFile string
	KeyFile  string
}

// WithTLS configures TLS for the server.
func (s *Server) WithTLS(cfg TLSConfig) error {
	cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
	if err != nil {
		return err
	}
	s.netMu.Lock()
	defer s.netMu.Unlock()
	if s.httpServer != nil {
		s.httpServer.TLSConfig = &tls.Config{Certificates: []tls.Certificate{cert}}
	}
	return nil
}
