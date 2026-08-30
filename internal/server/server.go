package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	"kalua/internal/bindings"
	"kalua/internal/host"
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
	Verbose     bool
	Logger      *host.Logger
}

// Server is the KALUA serve mode server with worker pool.
type Server struct {
	cfg           Config
	shared        *SharedState
	wsHub         *WSHub
	tcpHub        *TCPHub
	workers       []*Worker
	workerMu      sync.RWMutex // guards s.workers and s.retired
	retired       []*Worker    // superseded by hot reload, drained on shutdown
	workerCh      chan *Worker
	nextWorkerIdx atomic.Uint64 // round-robin cursor
	httpServer    *http.Server
	tcpListener   net.Listener
	wg            sync.WaitGroup
	stopCh        chan struct{}
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

	shared := NewSharedState()
	workerCh := make(chan *Worker, cfg.Workers)
	wsHub := NewWSHub(workerCh)
	tcpHub := NewTCPHub(workerCh)

	return &Server{
		cfg:      cfg,
		shared:   shared,
		wsHub:    wsHub,
		tcpHub:   tcpHub,
		workerCh: workerCh,
		stopCh:   make(chan struct{}),
	}
}

// Run starts the server and blocks until context is cancelled.
func (s *Server) Run(ctx context.Context) error {
	// Start workers
	if err := s.startWorkers(ctx); err != nil {
		return err
	}

	// Start HTTP server if mode includes http
	if s.modeHas("http") {
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

	s.cfg.Logger.Printf("KALUA serve mode listening on %s:%d (workers=%d, mode=%s)",
		s.cfg.Host, s.cfg.Port, s.cfg.Workers, s.cfg.Mode)

	// Wait for context cancellation
	<-ctx.Done()
	s.runShutdownHandlers()
	s.shutdown()
	return nil
}

// runShutdownHandlers invokes the optional shutdown() callback once, on the
// first worker, before workers are torn down (spec §2.2 shutdown).
func (s *Server) runShutdownHandlers() {
	if len(s.workers) > 0 {
		s.workers[0].CallShutdown()
	}
}

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
	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.cfg.Logger.Errorf("HTTP server error: %v", err)
		}
	}()

	// Shutdown on context cancel
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.httpServer.Shutdown(shutdownCtx)
	}()

	return nil
}

func (s *Server) handleHTTP(w http.ResponseWriter, r *http.Request) {
	// Get next worker (round-robin), leased for this request
	worker, release := s.leaseWorker()
	if worker == nil {
		http.Error(w, "no available workers", http.StatusServiceUnavailable)
		return
	}
	defer release()

	// Read request body
	body := ""
	if r.Body != nil {
		buf := make([]byte, r.ContentLength+1)
		n, _ := r.Body.Read(buf)
		body = string(buf[:n])
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
		s.cfg.Logger.Errorf("handle_http error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for k, v := range resp.Headers {
		w.Header().Set(k, v)
	}
	w.WriteHeader(resp.Status)
	w.Write([]byte(resp.Body))
}

func (s *Server) handleWSUpgrade(w http.ResponseWriter, r *http.Request) {
	// Upgrade to WebSocket
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		s.cfg.Logger.Errorf("websocket accept: %v", err)
		return
	}

	connID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
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
	s.cfg.Logger.Printf("WebSocket endpoint available at ws://%s:%d/ws", s.cfg.Host, s.cfg.Port)
}

func (s *Server) startTCP(ctx context.Context) error {
	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port+1) // TCP on port+1
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	s.tcpListener = listener

	s.cfg.Logger.Printf("TCP server listening on %s", addr)

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
	close(s.stopCh)
	if s.httpServer != nil {
		s.httpServer.Close()
	}
	if s.tcpListener != nil {
		s.tcpListener.Close()
	}
	// Close every current and retired worker. Retired workers may already
	// have been closed by their last lease release; Close is idempotent.
	s.workerMu.Lock()
	all := make([]*Worker, 0, len(s.workers)+len(s.retired))
	all = append(all, s.workers...)
	all = append(all, s.retired...)
	s.workers = nil
	s.retired = nil
	s.workerMu.Unlock()
	for _, w := range all {
		w.Close()
	}
	s.wg.Wait()
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
	if s.httpServer != nil {
		s.httpServer.TLSConfig = &tls.Config{Certificates: []tls.Certificate{cert}}
	}
	return nil
}
