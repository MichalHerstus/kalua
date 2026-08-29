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
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/yuin/gopher-lua"

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
		Addr:    addr,
		Handler: mux,
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
		http.Error(w, "session limit reached", http.StatusServiceUnavailable)
		return
	}
	s.sessionsMu.Unlock()

	// Upgrade to WebSocket
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
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
		for msg := range sess.Outbox() {
			if err := s.sendWS(c, msg); err != nil {
				s.logger.Errorf("send outbox: %v", err)
				return
			}
		}
		close(outboxDone)
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

	<-outboxDone
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
		sess.PostEvent(form, ctrl, event, toLValue(value))
	case "msgbox_choice":
		id := getString(msg, "id")
		choice := getString(msg, "choice")
		sess.HandleMsgboxChoice(id, choice)
	case "client_info":
		// TODO: handle client info (screen size, locale)
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

func toLValue(v interface{}) lua.LValue {
	switch val := v.(type) {
	case string:
		return lua.LString(val)
	case float64:
		return lua.LNumber(val)
	case bool:
		return lua.LBool(val)
	case nil:
		return lua.LNil
	default:
		return lua.LString(fmt.Sprintf("%v", v))
	}
}
