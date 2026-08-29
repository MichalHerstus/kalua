// Package lsp implements a Language Server Protocol server for the KALUA
// language. It gives editors (via `KALUA lsp`) completion, hover and go-to
// definition for the k.* API plus live diagnostics from the static checker.
//
// The server advertises UTF-8 position encoding, so LSP character offsets are
// byte offsets within a line. Because every token the server reasons about
// (k.* / K.* names) is ASCII this is exact; positions handed back to the
// client for unknown-* diagnostics are best-effort.
package lsp

import (
	"context"
	"io"
	"sync"
	"time"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"kalua/internal/checker"
)

// debounceDelay batches rapid keystrokes into a single diagnostics pass.
// Kept as a package var so tests can shrink it.
var debounceDelay = 150 * time.Millisecond

const refFileName = "kalua-api-reference.lua"

// Server implements protocol.Server for the KALUA language.
type Server struct {
	protocol.Server // embedded empty interface; only the methods below are served

	ctx    context.Context
	conn   jsonrpc2.Conn
	client protocol.Client
	vers   string

	mu   sync.Mutex
	docs map[uri.URI]document

	diagMu    sync.Mutex
	diagTimer *time.Timer
	diagDirty bool

	refOnce sync.Once
	refPath string
	refLine map[string]int
	refErr  error
}

type document struct {
	text    string
	version int32
}

// NewServer returns a fresh KALUA LSP server.
func NewServer() *Server {
	return &Server{
		docs: make(map[uri.URI]document),
		vers: "dev",
	}
}

// Serve runs the server over a connection (stdin/stdout) and returns when the
// peer closes it.
//
// The connection is wired like protocol.NewServer (union-aware codec, client
// embedded in the context) except that messages are dispatched serially on the
// read goroutine instead of via protocol.Handlers' asynchronous wrapper, so the
// server processes requests and notifications strictly in arrival order as the
// LSP spec requires. Without this a didOpen notification can be handled after
// a completion request that depends on it.
func Serve(l io.ReadWriteCloser, version string) error {
	s := NewServer()
	s.vers = version

	stream := jsonrpc2.NewStream(l)
	conn := jsonrpc2.NewConn(stream, jsonrpc2.WithCodec(lspCodec{}))
	client := protocol.ClientDispatcher(conn)
	ctx := protocol.WithClient(context.Background(), client)

	h := protocol.ServerHandler(s, jsonrpc2.MethodNotFoundHandler)
	conn.Go(ctx, h)

	s.ctx = ctx
	s.conn = conn
	s.mu.Lock()
	s.client = client
	s.mu.Unlock()
	<-conn.Done()
	return nil
}

// lspCodec mirrors protocol's internal codec: it routes message payloads
// through the protocol package's union-aware Marshal/Unmarshal (and passes
// jsonrpc2.RawMessage values through verbatim). Defined here so Serve can
// install the codec on a connection of its own wiring.
type lspCodec struct{}

// Marshal implements jsonrpc2.Codec.
func (lspCodec) Marshal(v any) ([]byte, error) {
	switch m := v.(type) {
	case jsonrpc2.RawMessage:
		if m == nil {
			return []byte("null"), nil
		}
		return m, nil
	case *jsonrpc2.RawMessage:
		if m == nil || *m == nil {
			return []byte("null"), nil
		}
		return *m, nil
	}
	return protocol.Marshal(v)
}

// Unmarshal implements jsonrpc2.Codec.
func (lspCodec) Unmarshal(data []byte, v any) error {
	if p, ok := v.(*jsonrpc2.RawMessage); ok {
		b := make(jsonrpc2.RawMessage, len(data))
		copy(b, data)
		*p = b
		return nil
	}
	return protocol.Unmarshal(data, v)
}

// /////// Lifecycle /////////

func (s *Server) Initialize(ctx context.Context, params *protocol.InitializeParams) (*protocol.InitializeResult, error) {
	if c, ok := protocol.ClientFromContext(ctx); ok {
		s.mu.Lock()
		s.client = c
		s.mu.Unlock()
	}
	resolve := true
	return &protocol.InitializeResult{
		Capabilities: protocol.ServerCapabilities{
			PositionEncoding: protocol.PositionEncodingKindUTF8,
			TextDocumentSync: protocol.TextDocumentSyncKindFull,
			CompletionProvider: &protocol.CompletionOptions{
				TriggerCharacters: []string{".", "'", "\""},
				ResolveProvider:   &resolve,
			},
			HoverProvider:      protocol.Boolean(true),
			DefinitionProvider: protocol.Boolean(true),
		},
		ServerInfo: protocol.ServerInfo{Name: "KALUA", Version: protocol.NewOptional(s.vers)},
	}, nil
}

func (s *Server) Initialized(ctx context.Context, params *protocol.InitializedParams) error {
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	return nil
}

func (s *Server) Exit(ctx context.Context) error {
	if s.conn != nil {
		s.conn.Close()
	}
	return nil
}

// /////// Text features /////////

func (s *Server) Completion(ctx context.Context, params *protocol.CompletionParams) (protocol.CompletionResult, error) {
	s.mu.Lock()
	d, ok := s.docs[params.TextDocument.URI]
	s.mu.Unlock()
	if !ok {
		return nil, nil
	}
	items := Completion(d.text, byteOffset(d.text, params.Position))
	if len(items) == 0 {
		return nil, nil
	}
	return protocol.CompletionItemSlice(items), nil
}

func (s *Server) Hover(ctx context.Context, params *protocol.HoverParams) (*protocol.Hover, error) {
	s.mu.Lock()
	d, ok := s.docs[params.TextDocument.URI]
	s.mu.Unlock()
	if !ok {
		return nil, nil
	}
	info, start, end := match(d.text, byteOffset(d.text, params.Position))
	if info == nil {
		return nil, nil
	}
	return &protocol.Hover{
		Contents: &protocol.MarkupContent{Kind: protocol.MarkupKindMarkdown, Value: markdown(*info)},
		Range:    &protocol.Range{Start: posAt(d.text, start), End: posAt(d.text, end)},
	}, nil
}

func (s *Server) Definition(ctx context.Context, params *protocol.DefinitionParams) (protocol.DefinitionResult, error) {
	s.mu.Lock()
	d, ok := s.docs[params.TextDocument.URI]
	s.mu.Unlock()
	if !ok {
		return nil, nil
	}
	loc := s.definition(d.text, byteOffset(d.text, params.Position))
	if loc == nil {
		return nil, nil
	}
	return protocol.LocationSlice{*loc}, nil
}

// /////// Text sync /////////

func (s *Server) DidOpen(ctx context.Context, params *protocol.DidOpenTextDocumentParams) error {
	td := params.TextDocument
	s.mu.Lock()
	s.docs[td.URI] = document{text: td.Text, version: td.Version}
	s.mu.Unlock()
	s.scheduleDiagnostics(td.URI)
	return nil
}

func (s *Server) DidChange(ctx context.Context, params *protocol.DidChangeTextDocumentParams) error {
	s.mu.Lock()
	d := s.docs[params.TextDocument.URI]
	for _, cc := range params.ContentChanges {
		switch c := cc.(type) {
		case *protocol.TextDocumentContentChangeWholeDocument:
			d.text = c.Text // full document sync
		case *protocol.TextDocumentContentChangePartial:
			d.text = applyEdit(d.text, c.Range, c.Text)
		}
	}
	d.version = params.TextDocument.Version
	s.docs[params.TextDocument.URI] = d
	s.mu.Unlock()
	s.scheduleDiagnostics(params.TextDocument.URI)
	return nil
}

func (s *Server) DidClose(ctx context.Context, params *protocol.DidCloseTextDocumentParams) error {
	s.mu.Lock()
	delete(s.docs, params.TextDocument.URI)
	s.mu.Unlock()
	s.publishEmpty(params.TextDocument.URI)
	return nil
}

// applyEdit applies a single incremental edit to text (defensive; the server
// advertises full sync but tolerates ranged changes).
func applyEdit(text string, r protocol.Range, replace string) string {
	start := byteOffset(text, r.Start)
	end := byteOffset(text, r.End)
	if end < start {
		start, end = end, start
	}
	return text[:start] + replace + text[end:]
}

// /////// Diagnostics /////////

// scheduleDiagnostics marks a document dirty and (re)arms the debounce timer
// so a burst of keystrokes produces one re-parse.
func (s *Server) scheduleDiagnostics(u uri.URI) {
	s.diagMu.Lock()
	s.diagDirty = true
	if s.diagTimer == nil {
		s.diagTimer = time.AfterFunc(debounceDelay, s.flushDiagnostics)
	} else {
		s.diagTimer.Reset(debounceDelay)
	}
	s.diagMu.Unlock()
}

func (s *Server) flushDiagnostics() {
	s.diagMu.Lock()
	if !s.diagDirty {
		s.diagMu.Unlock()
		return
	}
	s.diagDirty = false
	s.diagMu.Unlock()

	s.mu.Lock()
	uris := make([]uri.URI, 0, len(s.docs))
	for u := range s.docs {
		uris = append(uris, u)
	}
	s.mu.Unlock()

	for _, u := range uris {
		s.publishDiags(u)
	}
}

func (s *Server) publishDiags(u uri.URI) {
	s.mu.Lock()
	client := s.client
	d, ok := s.docs[u]
	s.mu.Unlock()
	if client == nil {
		return
	}
	var diags []protocol.Diagnostic
	if ok {
		name := u.Path()
		if name == "" {
			name = u.String()
		}
		res := checker.Check(d.text, name)
		for _, iss := range res.Issues {
			diags = append(diags, issueToDiag(iss, d.text))
		}
	}
	client.PublishDiagnostics(s.ctx, &protocol.PublishDiagnosticsParams{URI: u, Diagnostics: diags})
}

func (s *Server) publishEmpty(u uri.URI) {
	s.mu.Lock()
	client := s.client
	s.mu.Unlock()
	if client != nil {
		client.PublishDiagnostics(s.ctx, &protocol.PublishDiagnosticsParams{URI: u})
	}
}

// issueToDiag converts a checker.Issue (1-based line/column; 0 = unknown)
// into an LSP diagnostic (0-based, UTF-8 columns).
func issueToDiag(iss checker.Issue, text string) protocol.Diagnostic {
	line := uint32(0)
	if iss.Line > 0 {
		line = uint32(iss.Line - 1)
	}
	startCol := uint32(0)
	if iss.Col > 0 {
		startCol = uint32(iss.Col - 1)
	}
	start := protocol.Position{Line: line, Character: startCol}
	end := tokenEnd(text, start)
	return protocol.Diagnostic{
		Range:    protocol.Range{Start: start, End: end},
		Severity: protocol.DiagnosticSeverityError,
		Source:   protocol.NewOptional("kalua"),
		Message:  protocol.String(iss.Message),
	}
}

// tokenEnd returns the position just past an identifier-ish token starting at
// pos, clamped to the line so the squiggle never overruns the document.
func tokenEnd(text string, start protocol.Position) protocol.Position {
	startOff := byteOffset(text, start)
	endOff := startOff
	for endOff < len(text) && !isDelim(text[endOff]) {
		endOff++
	}
	// never emit a zero-width range; point diagnostics need a non-empty range
	if endOff == startOff {
		endOff++
	}
	return posAt(text, endOff)
}

func isDelim(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\r', ',', ';', ')', '(', '[', ']', '{', '}', '=', '"', '\'', '+', '-', '*', '/', '&', '|', '<', '>', '#', ':':
		return true
	}
	return false
}
