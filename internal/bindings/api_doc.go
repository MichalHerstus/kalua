// api_doc.go is the single source of truth for tooling (LSP completion,
// hover, definition). Every entry here must correspond to one in
// registerKnown; TestApiDocSync enforces that so the two never drift.
package bindings

// Info documents a single k.* binding for editors and generated API
// reference stubs.
type Info struct {
	// Name is the canonical dotted name, e.g. "form.new".
	Name string

	// Group is the registry group, e.g. "forms", "controls" or "json".
	Group string

	// Signature is a Lua-flavoured call signature, e.g.
	// "k.form.new(name, optsTable)".
	Signature string

	// Docs is a short human description shown on hover.
	Docs string

	// Source is the bindings source file that implements the binding
	// (e.g. "forms.go"); empty for namespace entries. Used to locate a
	// definition when the module sources are available.
	Source string
}

// apiDocs carries the tooling documentation for every k.* binding. Keep in
// sync with registerKnown; ApiDocSyncTest verifies the correspondence.
var apiDocs = map[string]Info{
	// flow
	"print":         {Name: "print", Group: "flow", Signature: "k.print(...)", Docs: "Prints values to the app log (tab-separated, like Lua print)."},
	"sleep":         {Name: "sleep", Group: "flow", Signature: "k.sleep(ms)", Docs: "Suspends the script for ms milliseconds."},
	"quit":          {Name: "quit", Group: "flow", Signature: "k.quit()", Docs: "Requests a clean termination of the app."},
	"error":         {Name: "error", Group: "flow", Signature: "k.error(msg)", Docs: "Raises a deliberate Lua error."},
	"msgbox":        {Name: "msgbox", Group: "flow", Signature: "k.msgbox(text[, kind])", Docs: "Shows a message box; kind defaults to \"info\". Returns user's choice."},
	"clipboard_set": {Name: "clipboard_set", Group: "flow", Signature: "k.clipboard_set(text)", Docs: "Writes text to the browser clipboard."},
	"clipboard_get": {Name: "clipboard_get", Group: "flow", Signature: "k.clipboard_get()", Docs: "Reads text from the browser clipboard."},
	"bell":          {Name: "bell", Group: "flow", Signature: "k.bell()", Docs: "Plays a system beep sound via WebAudio."},
	"screen_size":   {Name: "screen_size", Group: "flow", Signature: "k.screen_size()", Docs: "Returns viewport dimensions as {width, height}."},
	"http_request":  {Name: "http_request", Group: "flow", Signature: "k.http_request(optsTable)", Docs: "Makes an HTTP request. opts: {method, url, headers, body, timeout}. Returns {status, headers, body}."},

	// forms
	"form":           {Name: "form", Group: "forms", Signature: "k.form", Docs: "Form declarations: k.form.new/show/close/..."},
	"form.new":       {Name: "form.new", Group: "forms", Signature: "k.form.new(name, optsTable)", Docs: "Declares a form. optsTable may set title and layout."},
	"form.show":      {Name: "form.show", Group: "forms", Signature: "k.form.show(name)", Docs: "Shows a form (modal) and suspends the script until it closes."},
	"form.close":     {Name: "form.close", Group: "forms", Signature: "k.form.close([name])", Docs: "Closes the top form, or the named form."},
	"form.return_to": {Name: "form.return_to", Group: "forms", Signature: "k.form.return_to(name)", Docs: "Closes all forms above name."},
	"form.clear":     {Name: "form.clear", Group: "forms", Signature: "k.form.clear(name)", Docs: "Clears a form's control values."},
	"form.refresh":   {Name: "form.refresh", Group: "forms", Signature: "k.form.refresh(name)", Docs: "Re-renders and pushes the form to the browser."},
	"form.on":        {Name: "form.on", Group: "forms", Signature: "k.form.on(form, ctrl, event, fn)", Docs: "Registers an event handler (e.g. event \"onclick\") for a control."},

	// controls
	"ctrl":              {Name: "ctrl", Group: "controls", Signature: "k.ctrl", Docs: "Control constructors: k.ctrl.label/textbox/button/..."},
	"ctrl.label":        {Name: "ctrl.label", Group: "controls", Signature: "k.ctrl.label(form, name, optsTable)", Docs: "Adds a label control to a form."},
	"ctrl.textbox":      {Name: "ctrl.textbox", Group: "controls", Signature: "k.ctrl.textbox(form, name, optsTable)", Docs: "Adds a textbox control. opts may set label, value, enabled, visible."},
	"ctrl.button":       {Name: "ctrl.button", Group: "controls", Signature: "k.ctrl.button(form, name, optsTable)", Docs: "Adds a button control. opts may set label, class, onclick, enabled."},
	"ctrl.combo":        {Name: "ctrl.combo", Group: "controls", Signature: "k.ctrl.combo(form, name, optsTable)", Docs: "Adds a combo (dropdown) control. opts.items is a table of choices."},
	"ctrl.list":         {Name: "ctrl.list", Group: "controls", Signature: "k.ctrl.list(form, name, optsTable)", Docs: "Adds a multi-row select list. opts.items is a table of choices."},
	"ctrl.table":        {Name: "ctrl.table", Group: "controls", Signature: "k.ctrl.table(form, name, optsTable)", Docs: "Adds a table control; rows manipulated via k.table.*."},
	"ctrl.checkbox":     {Name: "ctrl.checkbox", Group: "controls", Signature: "k.ctrl.checkbox(form, name, optsTable)", Docs: "Adds a checkbox control."},
	"ctrl.radio":        {Name: "ctrl.radio", Group: "controls", Signature: "k.ctrl.radio(form, name, optsTable)", Docs: "Adds a radio button control."},
	"ctrl.set_value":    {Name: "ctrl.set_value", Group: "controls", Signature: "k.ctrl.set_value(form, name, value)", Docs: "Sets a control's value and re-renders it."},
	"ctrl.get_value":    {Name: "ctrl.get_value", Group: "controls", Signature: "k.ctrl.get_value(form, name)", Docs: "Returns a control's current value."},
	"ctrl.set_property": {Name: "ctrl.set_property", Group: "controls", Signature: "k.ctrl.set_property(form, name, prop, value)", Docs: "Sets an arbitrary control property."},
	"ctrl.get_property": {Name: "ctrl.get_property", Group: "controls", Signature: "k.ctrl.get_property(form, name, prop)", Docs: "Gets an arbitrary control property."},
	"ctrl.set_focus":    {Name: "ctrl.set_focus", Group: "controls", Signature: "k.ctrl.set_focus(form, name)", Docs: "Moves focus to a control in the browser."},
	"ctrl.refresh":      {Name: "ctrl.refresh", Group: "controls", Signature: "k.ctrl.refresh(form, name)", Docs: "Re-renders a single control and pushes the update."},

	// table operations
	"table":                     {Name: "table", Group: "controls", Signature: "k.table", Docs: "Table control operations: k.table.add_line/delete_line/..."},
	"table.add_line":            {Name: "table.add_line", Group: "controls", Signature: "k.table.add_line(form, name, valuesTable)", Docs: "Appends a row to a table control."},
	"table.delete_line":         {Name: "table.delete_line", Group: "controls", Signature: "k.table.delete_line(form, name, index)", Docs: "Removes the row at index."},
	"table.set_column_value":    {Name: "table.set_column_value", Group: "controls", Signature: "k.table.set_column_value(form, name, row, column, value)", Docs: "Sets a cell value in a table control."},
	"table.get_column_value":    {Name: "table.get_column_value", Group: "controls", Signature: "k.table.get_column_value(form, name, row, column)", Docs: "Gets a cell value from a table control."},
	"table.get_selected_column": {Name: "table.get_selected_column", Group: "controls", Signature: "k.table.get_selected_column(form, name)", Docs: "Gets the currently selected column."},
	"table.set_selected_column": {Name: "table.set_selected_column", Group: "controls", Signature: "k.table.set_selected_column(form, name, column)", Docs: "Sets the selected column."},

	// database
	"connect_db":    {Name: "connect_db", Group: "database", Signature: "k.connect_db(dsn)", Docs: "Opens a database connection (DSN scheme: sqlite://, mysql://, postgres://, sqlserver://) and returns a handle."},
	"disconnect_db": {Name: "disconnect_db", Group: "database", Signature: "k.disconnect_db([handle])", Docs: "Closes a connection, or all connections when no handle is given."},
	"sql":           {Name: "sql", Group: "database", Signature: "k.sql(handle, query, ...params)", Docs: "Executes arbitrary SQL; returns rows or {rows_affected}."},
	"db_select":     {Name: "db_select", Group: "database", Signature: "k.db_select(handle, table, fieldsTable, whereTable, order)", Docs: "Query builder returning {columns, rows}."},
	"db_insert":     {Name: "db_insert", Group: "database", Signature: "k.db_insert(handle, table, keyvalsTable)", Docs: "Inserts a row; returns {last_insert_id, rows_affected}."},
	"db_update":     {Name: "db_update", Group: "database", Signature: "k.db_update(handle, table, keyvalsTable, whereTable)", Docs: "Updates rows matching the where table."},
	"db_delete":     {Name: "db_delete", Group: "database", Signature: "k.db_delete(handle, table, whereTable)", Docs: "Deletes rows matching the where table."},
	"tx_begin":      {Name: "tx_begin", Group: "database", Signature: "k.tx_begin(handle)", Docs: "Starts a transaction on a connection."},
	"tx_commit":     {Name: "tx_commit", Group: "database", Signature: "k.tx_commit(handle)", Docs: "Commits the active transaction."},
	"tx_rollback":   {Name: "tx_rollback", Group: "database", Signature: "k.tx_rollback(handle)", Docs: "Rolls back the active transaction."},
	"rows":          {Name: "rows", Group: "database", Signature: "k.rows(result)", Docs: "Returns an iterator over a query result's rows."},

	// files
	"file_open":      {Name: "file_open", Group: "files", Signature: "k.file_open(path[, mode])", Docs: "Opens a file; mode is r, r+, w, w+, a or a+ (default r). Returns a handle."},
	"file_read":      {Name: "file_read", Group: "files", Signature: "k.file_read(handle[, count])", Docs: "Reads the whole file or count bytes; empty string at EOF."},
	"file_read_line": {Name: "file_read_line", Group: "files", Signature: "k.file_read_line(handle)", Docs: "Reads one line (trailing newline trimmed); nil at EOF."},
	"file_write":     {Name: "file_write", Group: "files", Signature: "k.file_write(handle, data)", Docs: "Writes data to an open file."},
	"file_close":     {Name: "file_close", Group: "files", Signature: "k.file_close(handle)", Docs: "Closes an open file handle."},
	"file_load":      {Name: "file_load", Group: "files", Signature: "k.file_load(path)", Docs: "Reads an entire file as a string (async; max 16 MiB)."},
	"file_save":      {Name: "file_save", Group: "files", Signature: "k.file_save(path, data)", Docs: "Writes a file atomically (async; temp file + rename)."},
	"file_copy":      {Name: "file_copy", Group: "files", Signature: "k.file_copy(src, dst)", Docs: "Copies a file, preserving permissions."},
	"file_move":      {Name: "file_move", Group: "files", Signature: "k.file_move(src, dst)", Docs: "Moves/renames a file."},
	"file_delete":    {Name: "file_delete", Group: "files", Signature: "k.file_delete(path)", Docs: "Deletes a file."},
	"file_exists":    {Name: "file_exists", Group: "files", Signature: "k.file_exists(path)", Docs: "Reports whether a path exists."},
	"file_mkdir":     {Name: "file_mkdir", Group: "files", Signature: "k.file_mkdir(path[, parents])", Docs: "Creates a directory; parents=true creates intermediate dirs."},
	"file_list":      {Name: "file_list", Group: "files", Signature: "k.file_list(dir)", Docs: "Lists a directory as a 1-based, sorted table of names."},
	"file_info":      {Name: "file_info", Group: "files", Signature: "k.file_info(path)", Docs: "Returns {name, size, is_dir, modified} for a path."},

	// json
	"json_parse":      {Name: "json_parse", Group: "json", Signature: "k.json_parse(text)", Docs: "Parses JSON text; null maps to K.NULL."},
	"json_string":     {Name: "json_string", Group: "json", Signature: "k.json_string(value)", Docs: "Encodes a value as compact JSON (sorted keys)."},
	"json_load":       {Name: "json_load", Group: "json", Signature: "k.json_load(path)", Docs: "Reads and parses a JSON file (async; max 16 MiB)."},
	"json_save":       {Name: "json_save", Group: "json", Signature: "k.json_save(path, value)", Docs: "Encodes a value and writes it atomically (async)."},
	"json_get":        {Name: "json_get", Group: "json", Signature: "k.json_get(root, path)", Docs: "Walks a dot/bracket path over a parsed value, e.g. \"a.b[0].c\"."},
	"json_array_item": {Name: "json_array_item", Group: "json", Signature: "k.json_array_item(root, path, index)", Docs: "Returns the element at a 0-based array index."},
	"json_count":      {Name: "json_count", Group: "json", Signature: "k.json_count(root, path)", Docs: "Returns the element count (array length or object size)."},
	"json_names":      {Name: "json_names", Group: "json", Signature: "k.json_names(root, path)", Docs: "Returns a 1-based table of keys or indices."},
	"is_null":         {Name: "is_null", Group: "json", Signature: "k.is_null(value)", Docs: "Reports whether value is the K.NULL sentinel."},

	// crypto
	"checksum": {Name: "checksum", Group: "crypto", Signature: "k.checksum(alg, data[, key[, salt[, iterations[, keylen]]]])", Docs: "Hex hash for alg: crc32, md5, sha1, sha256, hmac-sha256 (requires key), pbkdf2 (requires salt)."},
	"encrypt":  {Name: "encrypt", Group: "crypto", Signature: "k.encrypt(plaintext, key)", Docs: "AES-GCM encryption; returns base64(nonce || ciphertext)."},
	"decrypt":  {Name: "decrypt", Group: "crypto", Signature: "k.decrypt(b64, key)", Docs: "Reverse of k.encrypt."},

	// xml
	"xml_parse":      {Name: "xml_parse", Group: "xml", Signature: "k.xml_parse(text)", Docs: "Parses XML text and returns a document handle."},
	"xml_root":       {Name: "xml_root", Group: "xml", Signature: "k.xml_root(doc)", Docs: "Returns the root element name of a parsed document."},
	"xml_child":      {Name: "xml_child", Group: "xml", Signature: "k.xml_child(doc, path)", Docs: "Returns child element at path (e.g., \"book/author\")."},
	"xml_child_list": {Name: "xml_child_list", Group: "xml", Signature: "k.xml_child_list(doc, path)", Docs: "Returns a table of child elements at path."},
	"xml_attr":       {Name: "xml_attr", Group: "xml", Signature: "k.xml_attr(doc, path, name)", Docs: "Returns attribute value at path."},
	"xml_content":    {Name: "xml_content", Group: "xml", Signature: "k.xml_content(doc, path)", Docs: "Returns text content of element at path."},
	"xml_attrs":      {Name: "xml_attrs", Group: "xml", Signature: "k.xml_attrs(doc, path)", Docs: "Returns all attributes of element at path as a table."},
	"xml_name":       {Name: "xml_name", Group: "xml", Signature: "k.xml_name(doc, path)", Docs: "Returns the name of the element at path."},

	// server (serve mode)
	"shared":       {Name: "shared", Group: "server", Signature: "k.shared", Docs: "Shared state across workers: k.shared.set/get/del/keys/incr."},
	"shared.set":   {Name: "shared.set", Group: "server", Signature: "k.shared.set(key, value)", Docs: "Stores a string value in shared state."},
	"shared.get":   {Name: "shared.get", Group: "server", Signature: "k.shared.get(key)", Docs: "Retrieves a value from shared state; empty string if missing."},
	"shared.del":   {Name: "shared.del", Group: "server", Signature: "k.shared.del(key)", Docs: "Deletes a key from shared state."},
	"shared.keys":  {Name: "shared.keys", Group: "server", Signature: "k.shared.keys([pattern])", Docs: "Returns all keys matching pattern (prefix, * = all)."},
	"shared.incr":  {Name: "shared.incr", Group: "server", Signature: "k.shared.incr(key[, delta])", Docs: "Increments a numeric key by delta (default 1); returns new value."},
	"ws":           {Name: "ws", Group: "server", Signature: "k.ws", Docs: "WebSocket operations: k.ws.broadcast/send/close."},
	"ws.broadcast": {Name: "ws.broadcast", Group: "server", Signature: "k.ws.broadcast(message)", Docs: "Broadcasts a text message to all connected WebSocket clients."},
	"ws.send":      {Name: "ws.send", Group: "server", Signature: "k.ws.send(client_id, message)", Docs: "Sends a text message to a specific WebSocket client."},
	"ws.close":     {Name: "ws.close", Group: "server", Signature: "k.ws.close(client_id)", Docs: "Closes a WebSocket connection."},
	"tcp":          {Name: "tcp", Group: "server", Signature: "k.tcp", Docs: "TCP operations: k.tcp.send/close."},
	"tcp.send":     {Name: "tcp.send", Group: "server", Signature: "k.tcp.send(client_id, data)", Docs: "Sends data to a specific TCP client."},
	"tcp.close":    {Name: "tcp.close", Group: "server", Signature: "k.tcp.close(client_id)", Docs: "Closes a TCP connection."},
}

// KSets documents the K.* helpers and constants. This is static tooling data;
// the corresponding globals are installed by registerHelpers and Setup.
var KSets = []Info{
	{Name: "K.EQ", Signature: "K.EQ", Docs: "Operator constant \"=\".", Group: "helpers"},
	{Name: "K.NEQ", Signature: "K.NEQ", Docs: "Operator constant \"<>\".", Group: "helpers"},
	{Name: "K.ADD", Signature: "K.ADD", Docs: "Operator constant \"+\".", Group: "helpers"},
	{Name: "K.eq", Signature: "K.eq(a, b)", Docs: "Kalipso equality (numeric when both coerce, else string compare).", Group: "helpers"},
	{Name: "K.ne", Signature: "K.ne(a, b)", Docs: "Negation of K.eq.", Group: "helpers"},
	{Name: "K.add", Signature: "K.add(a, b)", Docs: "Kalipso addition: numeric if both coerce, else concatenation.", Group: "helpers"},
	{Name: "K.tonum", Signature: "K.tonum(x)", Docs: "Coerces to a number, else 0.", Group: "helpers"},
	{Name: "K.tostr", Signature: "K.tostr(x)", Docs: "Coerces to the Kalipso string form.", Group: "helpers"},
	{Name: "K.truthy", Signature: "K.truthy(x)", Docs: "Kalipso condition test (0/\"0\"/\"\"/nil are false).", Group: "helpers"},
	{Name: "K.NULL", Signature: "K.NULL", Docs: "Sentinel representing a JSON null.", Group: "helpers"},
	{Name: "K.is_null", Signature: "K.is_null(value)", Docs: "Reports whether value is K.NULL.", Group: "helpers"},
}

// Globals lists the script-visible globals beyond the k/K namespaces.
var Globals = []string{"ARGS", "CTRL", "main"}

// namespaceNames are registry entries that are pure namespaces with no
// implementation of their own; the sync test exempts them from Info.
var namespaceNames = map[string]bool{"form": true, "ctrl": true, "table": true, "shared": true, "ws": true, "tcp": true, "xml": true}

// Docs returns a copy of the k.* documentation map (name → Info).
func Docs() map[string]Info {
	out := make(map[string]Info, len(apiDocs))
	for k, v := range apiDocs {
		out[k] = v
	}
	return out
}

// KInfo returns a copy of the K.* helper documentation slice.
func KInfo() []Info {
	out := make([]Info, len(KSets))
	copy(out, KSets)
	return out
}

// GlobalsList returns the script-visible globals copy.
func GlobalsList() []string {
	out := make([]string, len(Globals))
	copy(out, Globals)
	return out
}

// Namespace reports whether name is a pure namespace entry (form, ctrl, table).
func Namespace(name string) bool { return namespaceNames[name] }
