// Package web provides the HTTP server and WebSocket bridge for KALUA run mode.
package web

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/coder/websocket"

	"kalua/internal/bindings"
	"kalua/internal/common"
	"kalua/internal/host"
	"kalua/internal/session"
)

//go:embed assets/*
var assetsFS embed.FS

//go:embed templates/*
var templatesFS embed.FS

// Server is the KALUA web server for run mode.
type Server struct {
	host          string
	port          int
	sessionLimit  int
	logger        *host.Logger
	defaultScript string
	opts          bindings.Options

	sessionsMu sync.Mutex
	sessions   map[string]*session.Session
}

// Port returns the configured port. If 0 was given (ephemeral), the actual port
// is only available after the server starts listening.
func (s *Server) Port() int {
	return s.port
}

// NewServer creates a new web server. opts carries per-session binding
// options (AllowFS roots, MaxFileSize cap).
func NewServer(host string, port, sessionLimit int, opts bindings.Options, logger *host.Logger) *Server {
	return &Server{
		host:         host,
		port:         port,
		sessionLimit: sessionLimit,
		logger:       logger,
		opts:         opts,
		sessions:     make(map[string]*session.Session),
	}
}

// Run starts the HTTP server and blocks until context is cancelled.
func (s *Server) Run(ctx context.Context, defaultScript string) error {
	s.defaultScript = defaultScript
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/ws/ui", s.handleWS)

	// Serve static assets from embedded FS
	assetsSubFS, err := fs.Sub(assetsFS, "assets")
	if err != nil {
		return fmt.Errorf("failed to create assets sub-FS: %w", err)
	}
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(assetsSubFS))))

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	addr := fmt.Sprintf("%s:%d", s.host, s.port)
	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(shutdownCtx)
	}()

	s.logger.Printf("KALUA web server listening on %s", addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// handleIndex serves the main HTML shell.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	// Security headers
	w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

	tmpl, err := template.ParseFS(templatesFS, "templates/shell.html")
	if err != nil {
		s.logger.Errorf("template parse error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	data := struct {
		Title string
	}{
		Title: "KALUA",
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		s.logger.Errorf("template execute error: %v", err)
	}
}

// handleWS upgrades the connection to WebSocket and runs the session bridge.
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	// Check session limit
	s.sessionsMu.Lock()
	if len(s.sessions) >= s.sessionLimit {
		s.sessionsMu.Unlock()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>KALUA — Session Limit</title>
<style>
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
         background: #f5f5f5; color: #333; display: flex; align-items: center;
         justify-content: center; min-height: 100vh; }
  .card { background: #fff; border-radius: 8px; box-shadow: 0 2px 8px rgba(0,0,0,.12);
          padding: 40px 48px; max-width: 480px; text-align: center; }
  h1 { font-size: 20px; margin-bottom: 12px; }
  p  { font-size: 14px; color: #666; line-height: 1.6; margin-bottom: 16px; }
  code { background: #eee; padding: 2px 6px; border-radius: 4px; font-size: 13px; }
  .retry { display: inline-block; margin-top: 8px; padding: 8px 20px;
           background: #4a90d9; color: #fff; border: none; border-radius: 4px;
           font-size: 14px; cursor: pointer; text-decoration: none; }
  .retry:hover { background: #357abd; }
</style>
</head>
<body>
<div class="card">
  <h1>Application Already Running</h1>
  <p>This KALUA application has reached its session limit.
     Close one of the open tabs or increase the limit with <code>--session-limit N</code>.</p>
  <a class="retry" href="javascript:location.reload()">Retry</a>
</div>
</body>
</html>`)
		return
	}
	s.sessionsMu.Unlock()

	// Upgrade to WebSocket
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"http://127.0.0.1:*", "http://localhost:*", "http://[::1]:*"},
	})
	if err != nil {
		s.logger.Errorf("websocket accept: %v", err)
		return
	}
	defer c.CloseNow()

	// Create session ID
	sessionID := fmt.Sprintf("sess-%d", time.Now().UnixNano())

	// Get script path from query or use default
	scriptPath := r.URL.Query().Get("script")
	if scriptPath == "" {
		scriptPath = s.defaultScript
	}
	if scriptPath == "" {
		scriptPath = "app.lua"
	}
	// Security: restrict script path to the configured default or its basename
	// to prevent arbitrary file execution via ?script= parameter
	if s.defaultScript != "" {
		allowed := s.defaultScript
		if !filepath.IsAbs(allowed) {
			// If defaultScript is relative, also allow the basename
			allowed = filepath.Base(allowed)
		}
		if scriptPath != s.defaultScript && scriptPath != filepath.Base(s.defaultScript) {
			s.logger.Warnf("rejected arbitrary script path: %s", scriptPath)
			scriptPath = s.defaultScript
		}
	}

	// Create session
	sess, err := session.New(sessionID, scriptPath, s.opts, s.logger)
	if err != nil {
		s.logger.Errorf("session create: %v", err)
		c.Close(websocket.StatusInternalError, err.Error())
		return
	}

	// Register session
	s.sessionsMu.Lock()
	s.sessions[sessionID] = sess
	s.sessionsMu.Unlock()

	// Clean up on disconnect
	defer func() {
		s.sessionsMu.Lock()
		delete(s.sessions, sessionID)
		s.sessionsMu.Unlock()
		sess.Close()
	}()

	// Send init message
	initMsg := common.OutboxMsg{Type: "init", Form: sessionID}
	if err := s.sendWS(c, initMsg); err != nil {
		s.logger.Errorf("send init: %v", err)
		return
	}

	// Start outbox pump
	outboxDone := make(chan struct{})
	go func() {
		defer close(outboxDone)
		for {
			select {
			case msg, ok := <-sess.Outbox():
				if !ok {
					return // outbox closed
				}
				if err := s.sendWS(c, msg); err != nil {
					s.logger.Errorf("send outbox: %v", err)
					return
				}
			case <-sess.Done():
				return // session closing
			}
		}
	}()

	// Read inbox messages from WebSocket
	ctx := r.Context()
	for {
		_, data, err := c.Read(ctx)
		if err != nil {
			s.logger.Printf("ws read: %v", err)
			break
		}

		var msg map[string]interface{}
		if err := json.Unmarshal(data, &msg); err != nil {
			s.logger.Errorf("ws unmarshal: %v", err)
			continue
		}

		s.handleWSMessage(sess, msg)
	}

	select {
	case <-outboxDone:
	case <-ctx.Done():
	}
}

// sendWS sends a JSON message over WebSocket.
func (s *Server) sendWS(c *websocket.Conn, msg common.OutboxMsg) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return c.Write(ctx, websocket.MessageText, data)
}

// handleWSMessage processes a message from the browser.
func (s *Server) handleWSMessage(sess *session.Session, msg map[string]interface{}) {
	msgType, _ := msg["type"].(string)
	switch msgType {
	case "event":
		form := getString(msg, "form")
		ctrl := getString(msg, "ctrl")
		event := getString(msg, "event")
		value := msg["value"]
		sess.PostEventAny(form, ctrl, event, value)
	case "msgbox_choice":
		id := getString(msg, "id")
		choice := getString(msg, "choice")
		sess.HandleMsgboxChoice(id, choice)
	case "clipboard_resp":
		id := getString(msg, "id")
		value := getString(msg, "value")
		sess.PostClipboardResp(id, value)
	case "file_picker_resp":
		id := getString(msg, "id")
		value := getString(msg, "value")
		sess.PostFilePickerResp(id, value)
	case "client_info":
		w := getInt(msg, "w")
		h := getInt(msg, "h")
		locale := getString(msg, "locale")
		sess.SetClientInfo(w, h, locale)
	case "ping":
		// Keep-alive
	}
}

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getInt(m map[string]interface{}, key string) int {
	switch v := m[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return int(n)
		}
	}
	return 0
}
