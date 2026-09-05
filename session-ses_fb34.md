# Phase 3 Controls Full Set

**Session ID:** ses_fb34f80faffem1vNZWC33kLyjV
**Created:** 8/29/2026, 10:43:50 AM
**Updated:** 8/29/2026, 6:42:07 PM

---

## Assistant





**Tool: write**

---

## Assistant

**Tool: write**

---

## Assistant

**Tool: write**

---

## User

---

## Assistant

## Objective
- Complete the approved full VSCode-support plan for the KALUA language: a Go LSP server subcommand (`KALUA lsp`) plus a `extensions/vscode-kalua` VSCode extension (completion, hover, diagnostics, go-to-definition, run/check/new commands for `*.lua` files).
- User said "implement"; LSP package is now written but its tests have not yet been run. Extension scaffold, AGENTS.md update, and final verification/commit remain.

## Important Details
- Repo: `github.com/MichalHerstus/kalua`; root `/Users/michalherstus/dev/kalua`; module `kalua`; Go 1.26.3; branch `main`. Commit `c033148` already pushed.
- Git identity for commits: `Michal Herstus <MichalHerstus@users.noreply.github.com>` via `git -c user.name=... -c user.email=...`; commit/push only at user's request.
- Import cycle constraint: `session` imports `bindings`; `bindings` must not import `session`. New `k.*` names must exist in `registerKnown`; `K.*` helpers are not checker-validated.
- gofmt: leave pre-existing unformatted `internal/common/outbox.go`, `internal/vm/app.go` alone.
- LSP stack: `go.lsp.dev/protocol v1.0.1` + `go.lsp.dev/jsonrpc2 v1.0.1` (+ indirect `go.lsp.dev/uri v1.0.1`, `github.com/go-json-experiment/json`). Protocol v1.0.1 API facts confirmed:
  - `protocol.NewServer(ctx, server, stream)` → `(ctx, conn, Client)`; `protocol.NewClient(ctx, client, stream)` → `(ctx, conn, Server)`; `ClientFromContext(ctx)`; `jsonrpc2.NewStream(io.ReadWriteCloser)`; Conn has `Go`, `Close() error`, `Done() <-chan struct{}`; `Handlers = CancelHandler(AsyncHandler(...))`.
  - Server interface uses short names: `Initialize`, `Initialized`, `Shutdown`, `Exit`, `DidOpen`, `DidChange`, `DidClose`, `Completion` → `(CompletionResult, error)`, `Hover` → `(*Hover, error)`, `Definition` → `(DefinitionResult, error)`.
  - Union types: `HoverProvider`/`DefinitionProvider` = `Boolean(true)` (types_unions.gen.go:20 `type Boolean bool`); `TextDocumentSync` = `TextDocumentSyncKindFull` (value 1); `CompletionResult` = `CompletionItemSlice` (named `[]CompletionItem`) | `*CompletionList`; `DefinitionResult` = `*Location` | `LocationSlice` | `DefinitionLinkSlice`; `HoverContents` = `*MarkupContent` | `String` | ...; `InlayHintTooltip` = `String` | `*MarkupContent`; `CompletionItem.TextEdit` = `CompletionItemTextEdit` union satisfied by `*TextEdit`.
  - `Optional[T]` + `protocol.NewOptional(v)`; `MarkupContent{Kind MarkupKind, Value string}` with `MarkupKindMarkdown = "markdown"`; `Position{Line, Character uint32}`; `TextEdit{Range Range, NewText string}`; `Diagnostic{..., Severity DiagnosticSeverity, Source Optional[string], Message InlayHintTooltip}`; `DiagnosticSeverityError = 1`; `PublishDiagnosticsParams{URI uri.URI, Version Optional[int32], Diagnostics []Diagnostic}`; `ServerCapabilities.PositionEncoding` + `PositionEncodingKindUTF8 = "utf-8"` exist; `ServerInfo.Version` is `Optional[string]`; kinds incl. `CompletionItemKindFunction=3`, `Variable=6`, `Class=7`, `Constant=21` (no Namespace kind).
  - **`TextDocumentContentChangeEvent` is a sealed union** of `*TextDocumentContentChangePartial{Range Range, RangeLength Optional[uint32], Text string}` and `*TextDocumentContentChangeWholeDocument{Text string}` (whole-document variant = full sync). `DidChange` must type-switch on these.
- Checker: `checker.Check(src, name string) Result`; `Issue{Message, Line, Col}` 1-based, 0=unknown; messages `unknown k.%s (not implemented)` and `missing required function main()`.
- bindings: `apiDocs` map keys are WITHOUT the `k.` prefix (e.g. `"form.new"`, `"print"`); `Info{Name, Group, Signature, Docs, Source}`; `Docs()`, `KInfo()` (names like `K.eq`), `GlobalsList()` (ARGS/CTRL/main), `Namespace()`.
- URI helper: `uri.URI` has `Path()`, `String()` (no `Filename()`).
- LSP server design decisions (implemented): advertise UTF-8 position encoding → LSP character offsets == byte offsets within a line (fine because all matched tokens are ASCII); Full text sync; ~150 ms debounced publishDiagnostics (severity Error, source "kalua"); completion prefix regex `` `\b([kK])(?:\.([a-zA-Z0-9_.]*))?$` `` with textEdit replacing token-start→cursor; hover/definition via longest `[kK]\.[a-zA-Z_][a-zA-Z0-9_]*(\.[a-zA-Z_][a-zA-Z0-9_]*)*` token overlapping cursor; definition jumps to generated `kalua-api-reference.lua` in os.TempDir() (name→1-based line of `-- signature` comment; `name = function() end` per entry; graceful failure via refErr). No snippet placeholders in v1.
- Extension plan (pending): language id `kalua` for `*.lua`, vendored MIT Lua TextMate grammar, thin TS client via `vscode-languageclient` spawning `kalua.binaryPath` (setting, default `KALUA`) args `["lsp"]`, commands `KALUA: Check file / Run app / New app` via integrated

## Objective
- Give editors full support for the KALUA language: completion, hover, diagnostics, go-to-definition, and editor commands, implemented as a Go LSP server subcommand (`KALUA lsp`) plus a `extensions/vscode-kalua` VSCode extension (language id `kalua` for `*.lua`). User approved the full plan and said "implement"; implementation is in progress.

## Important Details
- Repo: `https://github.com/MichalHerstus/kalua.git`, root `/Users/michalherstus/dev/kalua`, module `kalua`, Go 1.26.3, branch `main` tracks `origin/main`.
- Git identity for commits: `Michal Herstus <MichalHerstus@users.noreply.github.com>`; commit `c033148` (Phase 5a) already pushed. Commit/push only at user's request.
- Import cycle constraint: `session` imports `bindings`, so `bindings` must not import `session`.
- Any new `k.*` name must exist in `registerKnown` (internal/bindings/bindings.go). `K.*` helpers are not checker-validated.
- gofmt: only pre-existing unformatted `internal/common/outbox.go`, `internal/vm/app.go` — leave alone. Agent's own new/edited files (internal/lsp/*, internal/cli/cli.go) were gofmt'd.
- LSP stack: `go.lsp.dev/protocol v1.0.1` + `go.lsp.dev/jsonrpc2 v1.0.1` (+ indirect `go.lsp.dev/uri v1.0.1`, `github.com/go-json-experiment/json`).
- v1.0.1 API idioms confirmed:
  - `jsonrpc2.NewStream(io.ReadWriteCloser)`; `Conn` = `{Call, Notify, Go(ctx, handler), Close() error, Done() <-chan struct{}}`.
  - Server: `protocol.NewServer(ctx, server, stream) (context.Context, jsonrpc2.Conn, Client)`; Client: `protocol.NewClient(ctx, client, stream) (context.Context, jsonrpc2.Conn, Server)`. `Handlers` = `CancelHandler(jsonrpc2.AsyncHandler(handler))`.
  - `protocol.ClientFromContext(ctx)`/`WithClient` exist; `Client.PublishDiagnostics(ctx, *PublishDiagnosticsParams) error`.
  - `Server` interface uses short method names: `Initialize`, `Initialized`, `Shutdown`, `Exit`, `DidOpen`, `DidChange`, `DidClose`, `Definition(ctx, *DefinitionParams) (DefinitionResult, error)`, `Hover(ctx, *HoverParams) (*Hover, error)`, `Completion(ctx, *CompletionParams) (CompletionResult, error)`. Embed nil `protocol.Server` to implement subset.
  - Sealed unions everywhere: `Boolean bool` (Hover/DefinitionProvider), `TextDocumentSync` = `TextDocumentSyncKind` | `*TextDocumentSyncOptions`; `TextDocumentSyncKindFull = 1`; `CompletionResult` = `CompletionItemSlice` | `*CompletionList`; `HoverContents` = `*MarkupContent` | `String` | `*MarkedStringWithLanguage` | `MarkedStringSlice`; `DefinitionResult` = `*Location` | `LocationSlice` | `DefinitionLinkSlice`; `CompletionItem.TextEdit` = `*TextEdit` | `*InsertReplaceEdit`; `InlayHintTooltip` = `String` | `*MarkupContent`; named `type String string`.
  - **`TextDocumentContentChangeEvent` is a union**: `*TextDocumentContentChangePartial{Range Range (value type), RangeLength Optional[uint32], Text string}` or `*TextDocumentContentChangeWholeDocument{Text string}` (Since 3.18.0) — DidChange must type-switch.
  - `Optional[T]`: construct via `protocol.NewOptional(v)`. `Position{Line,Character uint32}`; `MarkupContent{Kind MarkupKind, Value string}`, `MarkupKindMarkdown="markdown"`. `Diagnostic{Message InlayHintTooltip, Source Optional[string], Severity DiagnosticSeverity (Error=1)}`. `ServerCapabilities.PositionEncoding` exists (`PositionEncodingKindUTF8 = "utf-8"`). `CompletionItemKind`: Variable=6, Class=7, Function=3, Constant=21.
- Server decisions: advertises UTF-8 position encoding, so LSP Character == byte offset in line (all tokens ASCII — exact); Full text sync; ~150 ms debounced publishDiagnostics (severity Error, source "kalua"); completion prefix regex `\b([kK])(?:\.([a-zA-Z0-9_.]*))?$`, textEdit replaces token-start→cursor with `"k."+fullName`; member-mode labels are leaf names; hover matches longest `k.*`/`K.*` token overlapping cursor; definition via generated `os.TempDir()/kalua-api-reference.lua` stub (name→1-based line; graceful failure → no definition).
- Checker facts: `checker.Check(src, name string) Result{Errors []string, Issues []Issue{Message, Line, Col}}` (1-based; 0=unknown); message format `unknown k.%s (not implemented)`.
- bindings api_doc: `apiDocs` map keys are WITHOUT `k.` prefix (e.g. `"form.new"`); exports `Docs()`, `KInfo()`, `GlobalsList()`, `Namespace()`; `Info{Name, Group, Signature, Docs, Source}`.
- Extension plan (still pending): language id `kalua` for `*.lua`, vendored MIT Lua TextMate grammar, thin TS client via `vscode-languageclient` spawning `kalua.binaryPath` (default `KALUA`) with `["lsp"]`; commands `KALUA: Check file / Run app / New app`; `vsce` → `.vsix`.

## Work State
### Completed
- Phase-5a groundwork (pushed as c033148): `internal/bindings/api_doc.go`, `api_doc_test.go` (TestApiDocSync), checker refactor to `Issue`/`Result` with `columnOf`, `TestIssuePositions`; `go build ./...` and `go test ./internal/...` were green; `go.lsp.dev/*` deps in go.mod.
- Protocol v1.0.1 API exploration complete (union members, signatures, marshaling; see Important Details).
- `internal/lsp/lsp.go` written: package doc (UTF-8 positions), `Server` struct (embeds nil `protocol.Server`; fields ctx/conn/client/vers/mu/docs/diagMu/diagTimer/diagDirty/refOnce/refPath/refLine/refErr), `NewServer()`, `Serve(l io.ReadWriteCloser, version string)` via `protocol.NewServer(...)` blocking on `<-conn.Done()`; `Initialize` (sets client via `ClientFromContext`, advertises Full sync, CompletionProvider TriggerCharacters [".","'","\""], Boolean hover/definition, ServerInfo "KALUA"); no-op `Initialized`/`Shutdown`; `Exit` closes conn; `DidOpen`/`DidChange` (union type-switch) /`DidClose`; `applyEdit`; debounced diagnostics (`scheduleDiagnostics`/`flushDiagnostics`/`publishDiags`/`publishEmpty`, `issueToDiag` 1-based→0-based + `tokenEnd` squiggle clamp); `Completion`/`Hover`/`Definition` request handlers.
- `internal/lsp/feature.go` written: `byteOffset`/`posAt` helpers, `sortedDocs`/`sortedKSets`, `Completion(text, cursor)` with K-branch, member mode (`memberScope`), top-level (namespaces, k.* functions, globals), `apiItem`/`memberItem`/`namespaceItem`/`globalItem`/`textEditAt`; `nameToken` regex, `match()`, `lookup()`, `markdown()`; `ensureReference()` (stub file builder + name→line map) and `definition()`.
- `internal/cli/cli.go`: `case "lsp"` → `lspCmd()`; `stdioConn{in io.Reader; out io.Writer}` (named fields after `invalid field name io.Reader` compile error); usage line `lsp    Language server over stdio (completion, hover, definitions)`; calls `lsp.Serve(stdioConn{in: os.Stdin, out: os.Stdout}, "dev")`.
- `go build ./...`, `go vet ./internal/lsp/ ./internal/cli/` pass ("BUILD_OK"); gofmt applied to lsp.go/feature.go/cli.go.
- `internal/lsp/lsp_test.go` written: `fakeClient` (PublishDiagnostics → buffered chan), `startLSP(t, fake)` (net.Pipe + `protocol.NewClient` + `go Serve(srvConn,"test")`, returns server dispatcher + done), `waitDiagnostics` (3s timeout), consts `base`/`goodScript`/`badScript`, tests `TestInitialize`, `TestDiagnosticsLifecycle` (no diags on good; 1 diagnostic containing "bogus" at Range.Start.Line==2 on bad), `TestCompletion` (top-level k. → form/ctrl/table/json_parse/print/ARGS/main; `k.form.` member → Label "new"/NewText "k.form.new"; `K.` → K.eq/K.NULL/K.is_null), `TestHover` (nil on comment; `*MarkupContent` containing "k.print"), `TestDefinition` (LocationSlice len 1, URI suffix `kalua-api-reference.lua`), helpers `completionItems`/`completionLabels`/`contains`.

### Active
- `internal/lsp/lsp_test.go` written but **not yet run**; known likely compile/runtime issues to resolve:
  - `strings.Contains(string(d.Message), "bogus")` won't compile — `Diagnostic.Message` is interface `InlayHintTooltip`, not `string` (use type switch or `fmt.Sprint`).
  - TestCompletion: the member assertion loop `return`s on success, which would skip the subsequent K.* completion section.
  - Unverified: `protocol.LanguageKindLua` constant exists (used in DidOpen TextDocumentItem).
- Verifying `go test ./internal/lsp/` next.

### Blocked
- None.

## Next Move
1. Run `go test -count=1 ./internal/lsp/` and fix compile/test issues: `string(d.Message)` conversion, TestCompletion early-return structure (make each completion scenario independent), and `LanguageKindLua` if undefined.
2. Re-run `go vet ./internal/...`, `go test -count=1 ./...`, `go build -o KALUA ./cmd/KALUA`; gofmt any edited files.
3. Scaffold `extensions/vscode-kalua/` per plan (package.json, vendored Lua TextMate grammar, TS client with `kalua.lsp`/`KALUA lsp` spawn, Check/Run/New commands, .vsix packaging).
4. Update AGENTS.md (`lsp` command + package layout), final verification, commit & push at user's request with the git identity above.

## Relevant Files
- `/Users/michalherstus/dev/kalua/internal/lsp/lsp.go` (new): server lifecycle, text sync, diagnostic debounce/publish, Completion/Hover/Definition handlers.
- `/Users/michalherstus/dev/kalua/internal/lsp/feature.go` (new): position helpers, completion engine, hover/definition token matching, generated API reference stub (`ensureReference`), `definition`.
- `/Users/michalherstus/dev/kalua/internal/lsp/lsp_test.go` (new, unrun): end-to-end tests over net.Pipe via `protocol.NewClient`.
- `/Users/michalherstus/dev/kalua/internal/cli/cli.go` (modified): `lsp` subcommand, `stdioConn`, usage text.
- `/Users/michalherstus/dev/kalua/internal/bindings/api_doc.go` (+ `api_doc_test.go`): tooling source of truth `Docs()`/`KInfo()`/`GlobalsList()`/`Namespace()`; keys lack `k.` prefix.
- `/Users/michalherstus/dev/kalua/internal/checker/checker.go`, `checker_test.go`: `Issue`/`Result`, `columnOf`; `Check(src, name string) Result`.
- `/Users/michalherstus/dev/kalua/go.mod`: requires `go.lsp.dev/protocol v1.0.1`, `go.lsp.dev/jsonrpc2 v1.0.1`, `go.lsp.dev/uri v1.0.1`.
- `$(go env GOMODCACHE)/go.lsp.dev/protocol@v1.0.1/`: `protocol.go`, `server.go`, `lifecycle.gen.go`, `types_unions.gen.go` (incl. `TextDocumentContentChange*` unions), `workspace_features.gen.go`, `completion.gen.go`, `context.go`; `go.lsp.dev/jsonrpc2@v1.0.1/`: `framer.go`, `conn.go`.
- `/Users/michalherstus/dev/kalua/AGENTS.md`: to be updated (lsp command + `internal/lsp/`, `extensions/vscode-kalua/`).
- `/Users/michalherstus/dev/kalua/extensions/vscode-kalua/`: to be scaffolded (next).

---

## User

---

## Assistant





**Tool: bash**

---

## Assistant

**Tool: write**

---

## Assistant

**Tool: write**

---

## Assistant

**Tool: write**

---

## Assistant

**Tool: write**

---

## Assistant

**Tool: write**

---

## Assistant

**Tool: write**

---

## Assistant

**Tool: write**

---

## Assistant

**Tool: write**

---

## Assistant

Scaffold complete. Verifying the TS client compiles:

**Tool: bash**

---

## Assistant





**Tool: bash**

---

## Assistant

**Tool: bash**

---

## Assistant

TS compiles cleanly. Now package to validate `vsce` finds everything:

**Tool: bash**

---

## Assistant





**Tool: edit**

---

## Assistant

**Tool: bash**

---

## Assistant

**Tool: bash**

---

## Assistant

Extension installs cleanly as `michalherstus.vscode-kalua`. Now updating AGENTS.md:

**Tool: read**

---

## Assistant

**Tool: edit**

---

## Assistant

**Tool: read**

---

## Assistant

**Tool: edit**

---

## Assistant

**Tool: edit**

---

## Assistant

**Tool: read**

---

## Assistant

**Tool: edit**

---

## Assistant

Now the final verification pass (build, vet, tests, race, gofmt):

**Tool: bash**

---

## Assistant

`api_doc.go`/`api_doc_test.go` show as unformatted (from the earlier phase). Checking the diff before fixing:

**Tool: bash**

---

## Assistant

These are our files from the prior phase — formatting them now:

**Tool: bash**

---

## Assistant

Single binary smoke test — full initialize → didOpen → completion round trip, lengths computed precisely:

**Tool: bash**

---

## Assistant

Server exits on stdin EOF before flushing — need to keep stdin open briefly so responses drain:

**Tool: bash**

---

## Assistant

Found a real bug: completion returns `null` when the cursor is at end-of-line before a trailing newline (`$` anchor fails). Let me inspect the completion code:

**Tool: read**

---

## Assistant

**Tool: grep**

---

## Assistant

**Tool: read**

---

## Assistant





**Tool: edit**

---

## Assistant

**Tool: edit**

---

## Assistant

Now add regression cases for EOL-before-newline to the unit tests:

**Tool: read**

---

## Assistant

**Tool: read**

---

## Assistant

**Tool: read**

---

## Assistant

**Tool: edit**

---

## Assistant

**Tool: bash**

---

## Assistant

Re-running the binary smoke test to confirm the regression fix end-to-end:

**Tool: bash**

---

## Assistant

Stale binary — rebuild before the smoke test:

**Tool: bash**

---

## Assistant

The real server returns `null` even after the fix while unit tests pass — that smells like message reordering (AsyncHandler runs each message in a goroutine; the completion can be processed before didOpen). Let me verify and inspect how the protocol lib wires the client into context:

**Tool: bash**

---

## Assistant

**Tool: bash**

---

## Assistant

**Tool: read**

---

## Assistant





**Tool: bash**

---

## Assistant

Confirmed: with a short delay after `didOpen`, completion works (7 form members). The `null` was a dispatch reordering race — `protocol.NewServer` wraps the handler in `AsyncHandler`, so notifications/requests can run out of order (breaking the LSP ordering guarantee). Fix: hand-wire the connection with a synchronous handler. Checking `NewServer`'s internals to replicate its client wiring:

**Tool: read**

---

## Assistant

**Tool: read**

---

## Assistant

**Tool: bash**

---

## Assistant

**Tool: read**

---

## Assistant

**Tool: bash**

---

## Assistant

The protocol lib wraps its handler in `AsyncHandler`, and jsonrpc2 otherwise runs handlers inline (strict ordering). To fix ordering I need the connection but not the async wrapper. Let me check the codec — if local, I can replicate the wiring:

**Tool: bash**

---

## Assistant

**Tool: read**

---

## Assistant

The codec delegates to exported `protocol.Marshal`/`Unmarshal`, so I can wire the connection myself with a synchronous handler. Checking `Handlers` and `ServerHandler`:

**Tool: bash**

---

## Assistant

`AsyncHandler` explicitly releases the read loop (`Async(ctx)`), breaking ordering; without it handlers run inline. I'll wire the connection with a serial `CancelHandler(ServerHandler(...))` and a local codec that routes through exported `protocol.Marshal`/`Unmarshal`. Verifying exports first:

**Tool: bash**

---

## Assistant

**Tool: bash**

---

## Assistant

Now rewiring `Serve`:

**Tool: edit**

---

## Assistant

**Tool: bash**

---

## Assistant

Now add a wire-ordering regression test that sends didOpen → completion back-to-back with no synchronization:



**Tool: read**

---

## Assistant

**Tool: edit**

---

## Assistant

**Tool: edit**

---

## Assistant

**Tool: bash**

---

## Assistant

Rebuild and re-run the original no-delay smoke script — completion/hover must now resolve without a sleep:



**Tool: bash**

---

## Assistant

Completion now returns members immediately after didOpen (ordering fixed). Hover at char-18 was past-EOL so nil is correct. Full verification:



**Tool: bash**

---

## Assistant

**Tool: bash**

---

## Assistant

**Tool: read**

---

## Assistant

**Tool: bash**

---

## Assistant

**Tool: read**

---

## Assistant

**Tool: bash**

---

## Assistant

**Tool: read**

---

## Assistant

**Tool: bash**

---

## Assistant

**Tool: read**

---

## Assistant

**Tool: bash**

---

## Assistant

**Tool: read**

---

## Assistant

**Tool: bash**

---

## Assistant

**Tool: bash**

---

## Assistant

**Tool: read**

---

## Assistant

The panic is inside the jsonrpc2 library teardown. Let me check whether it's pre-existing (latent) by temporarily reverting to `protocol.NewServer` and stress-running:



**Tool: bash**

---

## Assistant

**Tool: edit**

---

## Assistant

**Tool: bash**

---

## Assistant

**Tool: bash**

---

## Assistant

**Tool: bash**

---

## Assistant





**Tool: bash**

---

## Assistant

With the async wiring, `TestWireOrdering` fails from the race — confirming my serial dispatch fixed the ordering issue. The panic only appears with my wiring, so it's real. Restoring the serial wiring and reproducing the panic:

**Tool: bash**

---

## Assistant

**Tool: bash**

---

## Assistant

A real (pre-existing) teardown race in the jsonrpc2 library — repeated in-process server connections trigger a pooled-`incomingRequest` + context propagation crash. Checking for a newer module version:

**Tool: bash**

---

## Assistant





**Tool: bash**

---

## Assistant

Found it: `CancelHandler` wraps our handler with `context.WithCancel(ctx)` (handler.go:108), and that registers a `propagateCancel` goroutine watching the pooled `incomingRequest` — the source of the race. Dropping the cancellation wrapper removes it:

**Tool: edit**

---

## Assistant

**Tool: bash**

---

## Assistant

**Tool: bash**

---

## Assistant

0/15 failures. Full suite + race + smoke:

**Tool: bash**

---

## Assistant

All green. Adding a note about serial dispatch to AGENTS.md for future agents:

**Tool: edit**

---

## Assistant

**Tool: todowrite**

---

## Assistant

**Tool: bash**

---

## Assistant

All final checks pass — gofmt clean (only pre-existing unformatted files remain), vet OK, all tests green, binary builds, and the extension packages/installs.

**Tool: todowrite**

---

## Assistant

All done. Summary of the completed LSP + VSCode extension work:

**Go LSP server** (`internal/lsp/`)
- `lsp.go` — full lifecycle (initialize/initialized/shutdown/exit), full text sync (whole/partial change union), debounced `publishDiagnostics` from the checker, and completion/hover/definition handlers. `Serve` hand-wires the connection with a union-aware codec and **serial dispatch**: `protocol.NewServer`'s `AsyncHandler` broke LSP ordering (a completion could be served before a preceding `didOpen`) and `CancelHandler`'s context propagation raced the pooled request (rare teardown panic).
- `feature.go` — UTF-8 position helpers, completion (namespaces, members, `K.*`, globals with text edits), hover markdown from `bindings` api_doc, definition via a generated `~/kalua-api-reference.lua` stub. Fixed EOL-before-newline clamping so `k.form.` at line end completes correctly.
- `cli.go` — `KALUA lsp` subcommand over stdio.
- Tests cover lifecycle, diagnostics, completion member/top-level/`K.*`, hover, definition, and a dedicated wire-ordering regression test; `-race` and repeated runs are green.

**VSCode extension** (`extensions/vscode-kalua/`)
- Language id `kalua` for `*.lua`, Lua TextMate grammar, language-config, TS client via `vscode-languageclient` spawning `KALUA lsp`, commands `KALUA: Check file / Run app / New app`, `kalua.binaryPath` setting. Compiled, packaged as `.vsix` (6.9 KB), and the packaged extension installs into VS Code.

**AGENTS.md** updated with the `lsp` command, `internal/lsp/` + `extensions/vscode-kalua/` in the layout, and LSP wiring notes.

Anything not yet done: no commit. Want me to commit and push this phase (identity `Michal Herstus`)?

---

