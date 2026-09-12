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
	"sort"
	"strings"
	"time"

	"kalua/internal/ai"
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
	aiCfg    ai.ProviderConfig
}

// New binds the builder server to host:port immediately (port 0 → ephemeral),
// resolving the document format from the file extension (.lua vs .json).
func New(file, host string, port int) (*Server, error) {
	format := "json"
	if strings.HasSuffix(file, ".lua") {
		format = "lua"
	}
	aiCfg := ai.EnvConfig()
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	s := &Server{path: file, format: format, listener: ln, aiCfg: aiCfg}
	s.httpSrv = &http.Server{
		Handler:           s.mux(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		// Generous response deadlines: AI generation waits for a local model's
		// first token (cold starts can take minutes) and SSE streams for the
		// whole generation. The 30s WriteTimeout previously killed mid-flight
		// AI responses with an empty reply ("Failed to fetch" in the browser).
		WriteTimeout: 10 * time.Minute,
		IdleTimeout:  10 * time.Minute,
	}
	return s, nil
}

// SetAI overrides the AI provider config that New() read from the
// KALUA_AI_* env vars. Used by the CLI's --model/--base-url/--api-key-env.
func (s *Server) SetAI(cfg ai.ProviderConfig) {
	s.aiCfg = cfg
}

// GetAI returns the active AI provider config (for status/debug output).
func (s *Server) GetAI() ai.ProviderConfig {
	return s.aiCfg
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
	mux.HandleFunc("/api/ai/status", s.handleAIStatus)
	mux.HandleFunc("/api/ai/generate", s.handleAIGenerate)
	mux.HandleFunc("/api/ai/fix", s.handleAIFix)
	mux.HandleFunc("/api/ai/stream", s.handleAIStream)
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
		Lua  string     `json:"lua"`
		Mode string     `json:"mode"` // "replace" (default) | "merge"
		Base *Document  `json:"base"` // current open doc, required for merge
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, err)
		return
	}
	gen, err := Import(req.Lua, s.path)
	if err != nil {
		writeErr(w, err)
		return
	}
	if req.Mode == "merge" {
		if req.Base == nil {
			http.Error(w, "mode 'merge' requires the 'base' document", http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "merged": true, "doc": MergeDocument(req.Base, gen)})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "doc": gen})
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
	raw := map[string]any{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, "", err
	}
	raw = migrateLegacyCells(raw)
	b, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return nil, "", err
	}
	doc = &Document{}
	if err := json.Unmarshal(b, doc); err != nil {
		return nil, "", err
	}
	return doc, "Loaded JSON document.", nil
}

// migrateLegacyCells upgrades a v1 document (form.cells as an object map) to
// the v2 ordered-array form. Array-form cells (v2, or already migrated) pass
// through untouched. Map keys are sorted so the migration is deterministic.
func migrateLegacyCells(raw map[string]any) map[string]any {
	form, ok := raw["form"].(map[string]any)
	if !ok {
		return raw
	}
	if cells, isMap := form["cells"].(map[string]any); isMap {
		var list []any
		keys := make([]string, 0, len(cells))
		for k := range cells {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, id := range keys {
			var cell any = cells[id]
			m, ok := cell.(map[string]any)
			if !ok {
				m = map[string]any{}
			}
			m["id"] = id
			list = append(list, m)
		}
		form["cells"] = list
	}
	if ver, isNum := raw["version"].(float64); isNum && ver < 2 {
		raw["version"] = float64(DocVersion)
	}
	return raw
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

// AIStatusResponse is the JSON response for /api/ai/status.
type AIStatusResponse struct {
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	BaseURL   string `json:"baseUrl"`
	Reachable bool   `json:"reachable"`
	Error     string `json:"error,omitempty"`
}

func (s *Server) handleAIStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	status := AIStatusResponse{
		Provider:  providerOf(s.aiCfg.BaseURL),
		Model:     s.aiCfg.Model,
		BaseURL:   s.aiCfg.BaseURL,
		Reachable: false,
	}
	// Allow enough time for a remote provider's first-token latency (OpenRouter
	// free routes can be slow when cold; LM Studio may be loading the model).
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client := ai.NewClient(s.aiCfg)
	_, err := client.Completion(ctx, []ai.ChatMessage{{Role: "user", Content: "Reply with OK."}})
	if err == nil {
		status.Reachable = true
	} else {
		status.Error = err.Error()
	}
	writeJSON(w, status)
}

// AIGenerateRequest is the JSON body for /api/ai/generate.
type AIGenerateRequest struct {
	Request string           `json:"request"`
	Script  string           `json:"script,omitempty"`
	History []ai.ChatMessage `json:"history,omitempty"`
}

// AIGenerateResponse is the JSON response for /api/ai/generate.
type AIGenerateResponse struct {
	Script string   `json:"script"`
	Ok     bool     `json:"ok"`
	Errors []string `json:"errors,omitempty"`
	Logs   []string `json:"logs,omitempty"`
}

func (s *Server) handleAIGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req AIGenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, err)
		return
	}
	if req.Request == "" {
		writeErr(w, fmt.Errorf("request is required"))
		return
	}

	genReq := ai.GenerateRequest{Request: req.Request, Script: req.Script, History: req.History}
	result, err := ai.Generate(context.Background(), s.aiCfg, genReq)
	if err != nil {
		writeErr(w, fmt.Errorf("generation failed: %v", err))
		return
	}

	if err := os.WriteFile(s.path, []byte(result.Script), 0o644); err != nil {
		writeErr(w, fmt.Errorf("cannot write %s: %v", s.path, err))
		return
	}

	res := checker.Check(result.Script, s.path)
	writeJSON(w, AIGenerateResponse{
		Script: result.Script,
		Ok:     len(res.Errors) == 0,
		Errors: res.Errors,
		Logs:   result.Logs,
	})
}

// AIFixRequest is the JSON body for /api/ai/fix.
type AIFixRequest struct {
	Request string           `json:"request,omitempty"`
	Script  string           `json:"script"`
	History []ai.ChatMessage `json:"history,omitempty"`
}

// AIFixResponse is the JSON response for /api/ai/fix.
type AIFixResponse struct {
	Script string   `json:"script"`
	Ok     bool     `json:"ok"`
	Errors []string `json:"errors,omitempty"`
	Logs   []string `json:"logs,omitempty"`
}

func (s *Server) handleAIFix(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req AIFixRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, err)
		return
	}
	if req.Script == "" {
		writeErr(w, fmt.Errorf("script is required"))
		return
	}

	request := req.Request
	if request == "" {
		request = "Fix the validation errors in this KALUA script"
	}

	genReq := ai.GenerateRequest{Request: request, Script: req.Script, History: req.History}
	result, err := ai.Generate(context.Background(), s.aiCfg, genReq)
	if err != nil {
		writeErr(w, fmt.Errorf("fix failed: %v", err))
		return
	}

	if err := os.WriteFile(s.path, []byte(result.Script), 0o644); err != nil {
		writeErr(w, fmt.Errorf("cannot write %s: %v", s.path, err))
		return
	}

	res := checker.Check(result.Script, s.path)
	writeJSON(w, AIFixResponse{
		Script: result.Script,
		Ok:     len(res.Errors) == 0,
		Errors: res.Errors,
		Logs:   result.Logs,
	})
}

// handleAIStream runs ai.GenerateStream and relays progress as SSE events so
// the builder chat panel can show tokens as they arrive. A bounded channel
// decouples the LLM goroutine from the HTTP writer; the stream closes with a
// final "done" event (or "error").
func (s *Server) handleAIStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req AIGenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Request == "" {
		http.Error(w, "request is required", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	// Emit a status frame immediately so the browser receives headers and the
	// first data byte at t=0 (before the first LLM token, which can lag).
	if _, werr := w.Write([]byte("data: " + sseJSON(map[string]any{"type": "status", "text": "Requesting generation from the model…"}) + "\n\n")); werr != nil {
		return
	}

	genReq := ai.GenerateRequest{Request: req.Request, Script: req.Script, History: req.History}
	out := make(chan string, 8)
	go func() {
		defer close(out)
		_, err := ai.GenerateStream(context.Background(), s.aiCfg, genReq, func(ev ai.StreamEvent) {
			var payload map[string]any
			switch ev.Type {
			case "token":
				payload = map[string]any{"type": "token", "text": ev.Text}
			case "status":
				payload = map[string]any{"type": "status", "text": ev.Text}
			case "done":
				payload = map[string]any{"type": "done", "script": ev.Script, "ok": ev.Ok, "errors": ev.Errors, "logs": ev.Logs}
			}
			out <- "data: " + sseJSON(payload) + "\n\n"
		})
		if err != nil {
			out <- "data: " + sseJSON(map[string]any{"type": "error", "text": err.Error()}) + "\n\n"
		}
	}()

	for frame := range out {
		if _, werr := w.Write([]byte(frame)); werr != nil {
			break
		}
	}
}

// sseJSON renders v as a single-line JSON payload for an SSE data frame.
// json.Marshal escapes embedded newlines, so each event stays on one line.
func sseJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// providerOf guesses the provider label from a base URL for status display.
func providerOf(baseUrl string) string {
	if strings.Contains(baseUrl, "openrouter.ai") {
		return "openrouter"
	}
	if strings.HasPrefix(baseUrl, "http://localhost") || strings.HasPrefix(baseUrl, "http://127.0.0.1") {
		return "lmstudio"
	}
	return "custom"
}
