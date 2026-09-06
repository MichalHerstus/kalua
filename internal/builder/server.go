// Package builder is declared in model.go; this file implements the HTTP
// editor (KALUA builder) that round-trips a single form between its source
// file and the browser UI.

package builder

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"kalua/internal/checker"
)

//go:embed assets/*
var editorFS embed.FS

// Server is the KALUA Form Builder HTTP server. It serves the embedded editor
// UI and the /api/* JSON endpoints backing it.
type Server struct {
	path     string
	listener net.Listener
	httpSrv  *http.Server
	format   string // "lua" or "json"
}

// New binds the builder server to host:port immediately (port 0 → ephemeral),
// resolving the document format from the file extension (.lua vs .json).
func New(file, host string, port int) (*Server, error) {
	format := "json"
	if strings.HasSuffix(file, ".lua") {
		format = "lua"
	}
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	s := &Server{path: file, format: format, listener: ln}
	s.httpSrv = &http.Server{
		Handler:           s.mux(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	return s, nil
}

// Addr returns the bound listener address (resolves ephemeral ports).
func (s *Server) Addr() string { return s.listener.Addr().String() }

// URL returns the http:// URL reachable by a browser.
func (s *Server) URL() string { return "http://" + s.Addr() + "/" }

// Close stops the HTTP server.
func (s *Server) Close() error { return s.httpSrv.Close() }

// Run serves requests until ctx is cancelled.
func (s *Server) Run(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		s.httpSrv.Shutdown(shutdownCtx)
	}()
	err := s.httpSrv.Serve(s.listener)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func (s *Server) mux() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.Handle("/static/", noCache(http.StripPrefix("/static/", http.FileServer(http.FS(mustSub(editorFS, "assets"))))))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/api/form", s.handleForm)
	mux.HandleFunc("/api/source", s.handleSource)
	mux.HandleFunc("/api/export", s.handleExport)
	mux.HandleFunc("/api/import", s.handleImport)
	mux.HandleFunc("/api/validate", s.handleValidate)
	mux.HandleFunc("/api/preview", s.handlePreview)
	return security(noCache(mux))
}

func mustSub(e embed.FS, dir string) fs.FS {
	sub, err := fs.Sub(e, dir)
	if err != nil {
		panic(err)
	}
	return sub
}

func security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

func noCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data, err := editorFS.ReadFile("assets/index.html")
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

func (s *Server) handleForm(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.getForm(w, r)
	case http.MethodPut:
		s.putForm(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) getForm(w http.ResponseWriter, r *http.Request) {
	doc, message, err := s.load()
	if err != nil {
		writeErr(w, err)
		return
	}
	if doc == nil {
		doc = &Document{Version: DocVersion, Form: &Form{Name: "main", Layout: "vertical", Align: "left"}}
	}
	writeJSON(w, map[string]any{
		"path":    s.path,
		"format":  s.format,
		"doc":     doc,
		"message": message,
	})
}

func (s *Server) putForm(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Doc *Document `json:"doc"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, err)
		return
	}
	if req.Doc == nil {
		http.Error(w, `{"error":"missing doc"}`, http.StatusBadRequest)
		return
	}
	if msgs := req.Doc.Validate(); len(msgs) > 0 {
		writeErr(w, fmt.Errorf("invalid document: %s", strings.Join(msgs, "; ")))
		return
	}
	var data []byte
	var message string
	if s.format == "lua" {
		data = []byte(ExportLua(req.Doc))
		message = "Saved Lua source to " + s.path
	} else {
		var err error
		data, err = RawJSON(req.Doc)
		if err != nil {
			writeErr(w, err)
			return
		}
		message = "Saved JSON document to " + s.path
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		writeErr(w, err)
		return
	}
	if err := os.Rename(tmp, s.path); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "message": message})
}

func (s *Server) handleSource(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, map[string]any{"source": string(data)})
}

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Doc *Document `json:"doc"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, map[string]any{"lua": ExportLua(req.Doc)})
}

func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Lua string `json:"lua"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, err)
		return
	}
	doc, err := Import(req.Lua, s.path)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "doc": doc})
}

func (s *Server) handleValidate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Lua string `json:"lua"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, err)
		return
	}
	res := checker.Check(req.Lua, s.path)
	writeJSON(w, map[string]any{
		"ok":     len(res.Errors) == 0,
		"errors": res.Errors,
		"issues": res.Issues,
	})
}

func (s *Server) handlePreview(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Doc *Document `json:"doc"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, err)
		return
	}
	html, err := Preview(req.Doc)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, map[string]any{"html": html})
}

// load reads the document from disk: Lua sources are imported via the AST
// walker (converted on the fly), JSON documents are parsed directly. A
// missing file yields a nil doc (start-from-empty) with an explanatory note.
func (s *Server) load() (doc *Document, message string, err error) {
	data, rerr := os.ReadFile(s.path)
	if rerr != nil {
		if os.IsNotExist(rerr) {
			return nil, "File not found — starting with an empty form. Save to create it.", nil
		}
		return nil, "", rerr
	}
	if s.format == "lua" {
		doc, err = Import(string(data), s.path)
		if err != nil {
			return nil, "", err
		}
		return doc, "Imported from Lua source (structure extraction). Non-form code is kept only in the source file.", nil
	}
	doc = &Document{}
	if err := json.Unmarshal(data, doc); err != nil {
		return nil, "", err
	}
	return doc, "Loaded JSON document.", nil
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	writeJSON(w, map[string]any{"error": err.Error()})
}

// FileName returns the absolute form file path the server edits.
func (s *Server) FileName() string { return s.path }

// AbsPath is a small helper for callers that need the resolved path.
func AbsPath(p string) (string, error) { return filepath.Abs(p) }
