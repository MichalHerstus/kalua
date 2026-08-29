package lsp

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// fakeClient receives notifications from the server so tests can assert on
// published diagnostics.
type fakeClient struct {
	protocol.Client
	diags chan protocol.PublishDiagnosticsParams
}

func (f *fakeClient) PublishDiagnostics(ctx context.Context, params *protocol.PublishDiagnosticsParams) error {
	f.diags <- *params
	return nil
}

// startLSP wires a real server over net.Pipe to a test-side client dispatcher
// and returns the server-facing dispatcher to drive requests from the test.
func startLSP(t *testing.T, fake *fakeClient) (protocol.Server, func()) {
	t.Helper()
	srvConn, cliConn := net.Pipe()
	go func() {
		_ = Serve(srvConn, "test")
	}()
	ctx := context.Background()
	_, conn, server := protocol.NewClient(ctx, fake, jsonrpc2.NewStream(cliConn))
	done := func() {
		conn.Close()
	}
	return server, done
}

func waitDiagnostics(t *testing.T, fake *fakeClient) []protocol.Diagnostic {
	t.Helper()
	select {
	case params := <-fake.diags:
		return params.Diagnostics
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for publishDiagnostics")
		return nil
	}
}

const (
	base = `-- demo
function main()
end
`
	goodScript = `-- demo
function main()
  k.print("hi")
end
`
	badScript = `-- demo
local x = 1
k.bogus("nope")
function main()
end
`
)

func TestInitialize(t *testing.T) {
	debounceDelay = 5 * time.Millisecond
	fake := &fakeClient{diags: make(chan protocol.PublishDiagnosticsParams, 8)}
	server, done := startLSP(t, fake)
	defer done()

	res, err := server.Initialize(context.Background(), &protocol.InitializeParams{})
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if res.ServerInfo.Name != "KALUA" {
		t.Errorf("serverInfo.Name = %q, want KALUA", res.ServerInfo.Name)
	}
	if res.Capabilities.CompletionProvider == nil {
		t.Error("completionProvider not advertised")
	}
	if res.Capabilities.HoverProvider == nil {
		t.Error("hoverProvider not advertised")
	}
	if res.Capabilities.DefinitionProvider == nil {
		t.Error("definitionProvider not advertised")
	}
}

func TestDiagnosticsLifecycle(t *testing.T) {
	debounceDelay = 5 * time.Millisecond
	fake := &fakeClient{diags: make(chan protocol.PublishDiagnosticsParams, 8)}
	server, done := startLSP(t, fake)
	defer done()

	ctx := context.Background()
	u := uri.URI("file:///tmp/app.lua")

	if _, err := server.Initialize(ctx, &protocol.InitializeParams{}); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	// a valid script publishes no diagnostics
	if err := server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{URI: u, LanguageID: protocol.LanguageKindLua, Version: 1, Text: goodScript},
	}); err != nil {
		t.Fatalf("didOpen: %v", err)
	}
	if ds := waitDiagnostics(t, fake); len(ds) != 0 {
		t.Errorf("valid script: got %d diagnostics, want 0", len(ds))
	}

	// a broken script reports the unknown k.* with a position
	if err := server.DidChange(ctx, &protocol.DidChangeTextDocumentParams{
		TextDocument: protocol.VersionedTextDocumentIdentifier{
			TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: u},
			Version:                2,
		},
		ContentChanges: []protocol.TextDocumentContentChangeEvent{
			&protocol.TextDocumentContentChangeWholeDocument{Text: badScript},
		},
	}); err != nil {
		t.Fatalf("didChange: %v", err)
	}
	ds := waitDiagnostics(t, fake)
	if len(ds) != 1 {
		t.Fatalf("bad script: got %d diagnostics, want 1 got %v", len(ds), ds)
	}
	d := ds[0]
	if !strings.Contains(diagMessage(d), "bogus") {
		t.Errorf("diagnostic message = %q, want mention of bogus", diagMessage(d))
	}
	if d.Range.Start.Line != 2 {
		t.Errorf("diagnostic line = %d, want 2 (0-based line 3)", d.Range.Start.Line)
	}
	if err := server.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}

func TestCompletion(t *testing.T) {
	debounceDelay = 5 * time.Millisecond
	fake := &fakeClient{diags: make(chan protocol.PublishDiagnosticsParams, 8)}
	server, done := startLSP(t, fake)
	defer done()

	ctx := context.Background()
	u := uri.URI("file:///tmp/app.lua")
	if _, err := server.Initialize(ctx, &protocol.InitializeParams{}); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	baseItems := `-- demo
local v = k.`
	if err := server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{URI: u, LanguageID: protocol.LanguageKindLua, Version: 1, Text: baseItems},
	}); err != nil {
		t.Fatalf("didOpen: %v", err)
	}
	waitDiagnostics(t, fake) // servers send didOpen; sync diagnostics so the doc is applied

	// top-level: after "k." we expect namespaces, functions and globals
	pos := protocol.Position{Line: 1, Character: 13} // just past "k."
	res, err := server.Completion(ctx, &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: u},
			Position:     pos,
		},
	})
	if err != nil {
		t.Fatalf("completion: %v", err)
	}
	roots := completionLabels(t, res)
	for _, want := range []string{"form", "ctrl", "table", "json_parse", "print", "ARGS", "main"} {
		if !contains(roots, want) {
			t.Errorf("top-level completion missing %q (have %v)", want, roots)
		}
	}

	// namespace member mode: "k.form." yields the form.* members
	memberText := `-- demo
local v = k.form.
`
	if err := server.DidChange(ctx, &protocol.DidChangeTextDocumentParams{
		TextDocument: protocol.VersionedTextDocumentIdentifier{
			TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: u},
			Version:                2,
		},
		ContentChanges: []protocol.TextDocumentContentChangeEvent{
			&protocol.TextDocumentContentChangeWholeDocument{Text: memberText},
		},
	}); err != nil {
		t.Fatalf("didChange: %v", err)
	}
	waitDiagnostics(t, fake)
	res, err = server.Completion(ctx, &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: u},
			Position:     protocol.Position{Line: 1, Character: 18},
		},
	})
	if err != nil {
		t.Fatalf("member completion: %v", err)
	}
	items := completionItems(t, res)
	for _, item := range items {
		te, ok := item.TextEdit.(*protocol.TextEdit)
		if !ok {
			t.Errorf("item %q has no textEdit", item.Label)
			continue
		}
		if te.NewText == "k.form.new" {
			// member labels are leaves: "new" inserts "k.form.new"
			if item.Label != "new" {
				t.Errorf("form.new label = %q, want leaf %q", item.Label, "new")
			}
			return
		}
	}
	t.Errorf("member completion missing k.form.new (labels: %v)", completionLabels(t, res))

	// K.* namespace
	kText := `-- demo
local v = K.`
	if err := server.DidChange(ctx, &protocol.DidChangeTextDocumentParams{
		TextDocument: protocol.VersionedTextDocumentIdentifier{
			TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: u},
			Version:                3,
		},
		ContentChanges: []protocol.TextDocumentContentChangeEvent{
			&protocol.TextDocumentContentChangeWholeDocument{Text: kText},
		},
	}); err != nil {
		t.Fatalf("didChange: %v", err)
	}
	waitDiagnostics(t, fake)
	res, err = server.Completion(ctx, &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: u},
			Position:     protocol.Position{Line: 1, Character: 12},
		},
	})
	if err != nil {
		t.Fatalf("K completion: %v", err)
	}
	klabels := completionLabels(t, res)
	for _, want := range []string{"K.eq", "K.NULL", "K.is_null"} {
		if !contains(klabels, want) {
			t.Errorf("K completion missing %q (have %v)", want, klabels)
		}
	}
}

// TestWireOrdering pins the LSP ordering guarantee: a completion sent back to
// back (no synchronization) after didOpen must already see the new document.
// The server dispatches messages serially in arrival order, so the didOpen
// notification is applied before the completion request that follows it.
func TestWireOrdering(t *testing.T) {
	debounceDelay = 5 * time.Millisecond
	fake := &fakeClient{diags: make(chan protocol.PublishDiagnosticsParams, 8)}
	server, done := startLSP(t, fake)
	defer done()

	ctx := context.Background()
	u := uri.URI("file:///tmp/app.lua")
	if _, err := server.Initialize(ctx, &protocol.InitializeParams{}); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	doc := `-- demo
local v = k.form.
`
	if err := server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{URI: u, LanguageID: protocol.LanguageKindLua, Version: 1, Text: doc},
	}); err != nil {
		t.Fatalf("didOpen: %v", err)
	}

	res, err := server.Completion(ctx, &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: u},
			Position:     protocol.Position{Line: 1, Character: 18},
		},
	})
	if err != nil {
		t.Fatalf("completion: %v", err)
	}
	for _, item := range completionItems(t, res) {
		if te, ok := item.TextEdit.(*protocol.TextEdit); ok && te.NewText == "k.form.new" {
			return
		}
	}
	t.Errorf("completion missed k.form.new (labels: %v)", completionLabels(t, res))
}

func TestHover(t *testing.T) {
	fake := &fakeClient{diags: make(chan protocol.PublishDiagnosticsParams, 8)}
	server, done := startLSP(t, fake)
	defer done()

	ctx := context.Background()
	u := uri.URI("file:///tmp/app.lua")
	if _, err := server.Initialize(ctx, &protocol.InitializeParams{}); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if err := server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{URI: u, LanguageID: protocol.LanguageKindLua, Version: 1, Text: base},
	}); err != nil {
		t.Fatalf("didOpen: %v", err)
	}
	waitDiagnostics(t, fake)

	hover, err := server.Hover(ctx, &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: u},
			Position:     protocol.Position{Line: 0, Character: 3}, // on "-- demo" → nothing
		},
	})
	if err != nil {
		t.Fatalf("hover: %v", err)
	}
	if hover != nil {
		t.Errorf("hover on comment should be nil, got %v", hover)
	}

	// contains the hover text
	if err := server.DidChange(ctx, &protocol.DidChangeTextDocumentParams{
		TextDocument: protocol.VersionedTextDocumentIdentifier{
			TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: u},
			Version:                2,
		},
		ContentChanges: []protocol.TextDocumentContentChangeEvent{
			&protocol.TextDocumentContentChangeWholeDocument{Text: "local a = k.print(\"x\")\n"},
		},
	}); err != nil {
		t.Fatalf("didChange: %v", err)
	}
	waitDiagnostics(t, fake)
	hover, err = server.Hover(ctx, &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: u},
			Position:     protocol.Position{Line: 0, Character: 13}, // inside "k.print"
		},
	})
	if err != nil {
		t.Fatalf("hover on k.print: %v", err)
	}
	if hover == nil {
		t.Fatal("hover on k.print returned nil")
	}
	md, ok := hover.Contents.(*protocol.MarkupContent)
	if !ok {
		t.Fatalf("hover contents type = %T, want *MarkupContent", hover.Contents)
	}
	if !strings.Contains(md.Value, "k.print") {
		t.Errorf("hover markdown = %q, want mention of k.print", md.Value)
	}
}

func TestDefinition(t *testing.T) {
	fake := &fakeClient{diags: make(chan protocol.PublishDiagnosticsParams, 8)}
	server, done := startLSP(t, fake)
	defer done()

	ctx := context.Background()
	u := uri.URI("file:///tmp/app.lua")
	if _, err := server.Initialize(ctx, &protocol.InitializeParams{}); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if err := server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{URI: u, LanguageID: protocol.LanguageKindLua, Version: 1, Text: "local a = k.json_parse(\"{}\")\n"},
	}); err != nil {
		t.Fatalf("didOpen: %v", err)
	}
	waitDiagnostics(t, fake)

	res, err := server.Definition(ctx, &protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: u},
			Position:     protocol.Position{Line: 0, Character: 13},
		},
	})
	if err != nil {
		t.Fatalf("definition: %v", err)
	}
	locs, ok := res.(protocol.LocationSlice)
	if !ok {
		t.Fatalf("definition result type = %T, want LocationSlice", res)
	}
	if len(locs) != 1 {
		t.Fatalf("definition locations = %d, want 1", len(locs))
	}
	if !strings.HasSuffix(string(locs[0].URI), refFileName) {
		t.Errorf("definition uri = %q, want suffix %q", string(locs[0].URI), refFileName)
	}
	if locs[0].Range.Start.Line == 0 && locs[0].Range.Start.Character == 0 && locs[0].Range.End == locs[0].Range.Start {
		t.Error("definition landed on an empty range")
	}
}

// diagMessage extracts the human-readable text from a diagnostic message
// (an InlayHintTooltip union of String or MarkupContent).
func diagMessage(d protocol.Diagnostic) string {
	switch m := d.Message.(type) {
	case protocol.String:
		return string(m)
	case *protocol.MarkupContent:
		return m.Value
	default:
		return ""
	}
}

// completionItems extracts completion items from a CompletionResult.
func completionItems(t *testing.T, res protocol.CompletionResult) []protocol.CompletionItem {
	t.Helper()
	switch r := res.(type) {
	case protocol.CompletionItemSlice:
		return r
	case *protocol.CompletionList:
		return r.Items
	default:
		t.Fatalf("completion result type = %T", res)
		return nil
	}
}

func completionLabels(t *testing.T, res protocol.CompletionResult) []string {
	t.Helper()
	items := completionItems(t, res)
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Label
	}
	return out
}

func contains(hay []string, needle string) bool {
	for _, s := range hay {
		if s == needle {
			return true
		}
	}
	return false
}
