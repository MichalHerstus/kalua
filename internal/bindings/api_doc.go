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
	"pick_file":     {Name: "pick_file", Group: "flow", Signature: "k.pick_file([opts])", Docs: "Opens a browser file picker dialog. opts (optional table): {accept=\"image/*,.pdf\", multiple=true}. Returns a table of files: {{name, size, type, data}, ...} where data is base64-encoded. Returns nil on cancel."},
	"bell":          {Name: "bell", Group: "flow", Signature: "k.bell()", Docs: "Plays a system beep sound via WebAudio."},
	"screen_size":   {Name: "screen_size", Group: "flow", Signature: "k.screen_size()", Docs: "Returns viewport dimensions as {width, height}."},
	"http_request":  {Name: "http_request", Group: "flow", Signature: "k.http_request(optsTable)", Docs: "Makes an HTTP request. opts: {method, url, headers, body, timeout}. Returns {status, headers, body}."},

	// debug
	"debug":        {Name: "debug", Group: "debug", Signature: "k.debug", Docs: "Runtime introspection helpers: stack/locals/trace."},
	"debug.stack":  {Name: "debug.stack", Group: "debug", Signature: "k.debug.stack()", Docs: "Returns a table of the current call frames, each with level, name, source, line and locals."},
	"debug.locals": {Name: "debug.locals", Group: "debug", Signature: "k.debug.locals([level])", Docs: "Returns a table of local name → value for the given frame level (default 1)."},
	"debug.trace":  {Name: "debug.trace", Group: "debug", Signature: "k.debug.trace([msg])", Docs: "Logs a script-side trace anchor when verbose tracing is enabled."},

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
	"table.set_data":            {Name: "table.set_data", Group: "controls", Signature: "k.table.set_data(form, name, dataTable)", Docs: "Bulk replaces all row data (Tabulator mode pushes tabulator_update)."},
	"table.get_data":            {Name: "table.get_data", Group: "controls", Signature: "k.table.get_data(form, name)", Docs: "Returns all current data of a table control."},
	"table.get_selected_rows":   {Name: "table.get_selected_rows", Group: "controls", Signature: "k.table.get_selected_rows(form, name)", Docs: "Returns the selected row indices (1-based)."},
"table.set_remote_data":   {Name: "table.set_remote_data", Group: "controls", Signature: "k.table.set_remote_data(form, name, {data,last_page,last_row})", Docs: "Pushes server-side pagination data to a tabulator table =  {data=rows, last_page=n} or {data=rows, last_row=n}."},
	"table.refresh":         {Name: "table.refresh", Group: "controls", Signature: "k.table.refresh(form, name)", Docs: "Re-runs a DB-linked tabulator table's query and shows page 1."},
	"table.set_db_source":   {Name: "table.set_db_source", Group: "controls", Signature: "k.table.set_db_source(form, name, opts)", Docs: "Swaps a DB-linked tabulator table's source {db,query,columns?,page_size?,count_query?,where?,order_by?} and refreshes."},

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

	// tier-2 flow
	"timer_start":  {Name: "timer_start", Group: "flow", Signature: "k.timer_start(id, ms[, repeats])", Docs: "Starts a session timer; fires a Lua function named id (repeats optional)."},
	"timer_stop":   {Name: "timer_stop", Group: "flow", Signature: "k.timer_stop(id)", Docs: "Stops a running session timer."},
	"status_show":  {Name: "status_show", Group: "flow", Signature: "k.status_show(text)", Docs: "Shows a busy/status bar with the given text."},
	"status_close": {Name: "status_close", Group: "flow", Signature: "k.status_close()", Docs: "Hides the status bar."},
	"param_set":    {Name: "param_set", Group: "flow", Signature: "k.param_set(key, value)", Docs: "Persists an app param (string) to an app-side file."},
	"param_get":    {Name: "param_get", Group: "flow", Signature: "k.param_get(key)", Docs: "Reads a persisted app param (string; \"\" if unset)."},
	"net_ok":       {Name: "net_ok", Group: "flow", Signature: "k.net_ok(timeout_ms)", Docs: "Reports internet reachability via a TCP dial."},
	"locale":       {Name: "locale", Group: "flow", Signature: "k.locale()", Docs: "Returns the session locale (\"en-US\" default)."},
	"ping":         {Name: "ping", Group: "flow", Signature: "k.ping(host, timeout_ms)", Docs: "TCP-based latency probe returning ms, or nil when unreachable."},

	// tier-2 database
	"connect_sqlite":    {Name: "connect_sqlite", Group: "database", Signature: "k.connect_sqlite(path)", Docs: "Opens a SQLite database file; returns a handle usable with k.sql/k.db_*."},
	"disconnect_sqlite": {Name: "disconnect_sqlite", Group: "database", Signature: "k.disconnect_sqlite([handle])", Docs: "Closes a SQLite connection (or all)."},
	"db_kill_table":     {Name: "db_kill_table", Group: "database", Signature: "k.db_kill_table(handle, table, where)", Docs: "Deletes rows matching the where table."},
	"db_proc":           {Name: "db_proc", Group: "database", Signature: "k.db_proc(handle, name, ...params)", Docs: "Executes a stored procedure."},

	// tier-2 comm
	"socket_open":      {Name: "socket_open", Group: "comm", Signature: "k.socket_open(host, port[, timeout_ms])", Docs: "Opens a TCP client connection; returns a handle."},
	"socket_write":     {Name: "socket_write", Group: "comm", Signature: "k.socket_write(handle, data)", Docs: "Writes data to an open socket; returns bytes written."},
	"socket_read":      {Name: "socket_read", Group: "comm", Signature: "k.socket_read(handle[, count])", Docs: "Reads count bytes (or all until close) from a socket."},
	"socket_read_line": {Name: "socket_read_line", Group: "comm", Signature: "k.socket_read_line(handle)", Docs: "Reads one line (trailing newline trimmed); nil at EOF."},
	"socket_close":     {Name: "socket_close", Group: "comm", Signature: "k.socket_close(handle)", Docs: "Closes an open socket."},

	// tier-2 FTP
	"ftp_connect":     {Name: "ftp_connect", Group: "comm", Signature: "k.ftp_connect(host[, port, user, pw])", Docs: "Connects to an FTP server; returns a handle."},
	"ftp_set_cwd":     {Name: "ftp_set_cwd", Group: "comm", Signature: "k.ftp_set_cwd(handle, path)", Docs: "Changes the remote working directory (CWD)."},
	"ftp_get_file":    {Name: "ftp_get_file", Group: "comm", Signature: "k.ftp_get_file(handle, remote, local)", Docs: "Downloads remote to a local path (RETR)."},
	"ftp_put_file":    {Name: "ftp_put_file", Group: "comm", Signature: "k.ftp_put_file(handle, local, remote)", Docs: "Uploads a local file to remote (STOR)."},
	"ftp_file_exists": {Name: "ftp_file_exists", Group: "comm", Signature: "k.ftp_file_exists(handle, path)", Docs: "Reports whether a remote file exists (SIZE)."},
	"ftp_create_dir":  {Name: "ftp_create_dir", Group: "comm", Signature: "k.ftp_create_dir(handle, path)", Docs: "Creates a remote directory (MKD)."},
	"ftp_delete":      {Name: "ftp_delete", Group: "comm", Signature: "k.ftp_delete(handle, path)", Docs: "Deletes a remote file (DELE)."},
	"ftp_rename":      {Name: "ftp_rename", Group: "comm", Signature: "k.ftp_rename(handle, from, to)", Docs: "Renames a remote file or folder (RNFR/RNTO)."},
	"ftp_list":        {Name: "ftp_list", Group: "comm", Signature: "k.ftp_list(handle[, path])", Docs: "Lists remote entry names (LIST)."},
	"ftp_disconnect":  {Name: "ftp_disconnect", Group: "comm", Signature: "k.ftp_disconnect(handle)", Docs: "Sends QUIT and closes the FTP connection."},

	// tier-2 email (SMTP)
	"smtp_connect":    {Name: "smtp_connect", Group: "email", Signature: "k.smtp_connect{host,port,user,pw,tls}", Docs: "Connects to an SMTP server; returns a handle."},
	"smtp_send":       {Name: "smtp_send", Group: "email", Signature: "k.smtp_send(handle, {from,to,subject,body,attachments})", Docs: "Sends an email through the connected SMTP server."},
	"smtp_disconnect": {Name: "smtp_disconnect", Group: "email", Signature: "k.smtp_disconnect([handle])", Docs: "Closes an SMTP connection (or all)."},

	// tier-2 email (POP3)
	"pop3_connect": {Name: "pop3_connect", Group: "email", Signature: "k.pop3_connect{host,port,user,pw,tls}", Docs: "Connects to a POP3 server; returns a handle."},
	"pop3_stat":    {Name: "pop3_stat", Group: "email", Signature: "k.pop3_stat(handle)", Docs: "Returns {count,size} of the mailbox."},
	"pop3_list":    {Name: "pop3_list", Group: "email", Signature: "k.pop3_list(handle)", Docs: "Returns a table of {id,size} message summaries."},
	"pop3_retr":    {Name: "pop3_retr", Group: "email", Signature: "k.pop3_retr(handle, index)", Docs: "Retrieves a message by index."},
	"pop3_dele":    {Name: "pop3_dele", Group: "email", Signature: "k.pop3_dele(handle, index)", Docs: "Marks a message for deletion."},
	"pop3_noop":    {Name: "pop3_noop", Group: "email", Signature: "k.pop3_noop(handle)", Docs: "Keeps the POP3 connection alive."},
	"pop3_quit":    {Name: "pop3_quit", Group: "email", Signature: "k.pop3_quit(handle)", Docs: "Sends QUIT and closes the POP3 connection."},

	// tier-2 web service (SOAP)
	"webservice_run": {Name: "webservice_run", Group: "comm", Signature: "k.webservice_run(profile, params)", Docs: "Calls a SOAP web service. profile: {url,action[,method,timeout_ms]}; params is the body table. Returns {status,headers,body}."},

	// tier-2 crypto
	"crypt_symmetric":  {Name: "crypt_symmetric", Group: "crypto", Signature: "k.crypt_symmetric(alg, key, data[, iv])", Docs: "AES-CBC symmetric encrypt/decrypt (alg aes-encrypt/aes-decrypt). Returns base64."},
	"crypt_asymmetric": {Name: "crypt_asymmetric", Group: "crypto", Signature: "k.crypt_asymmetric(alg, key, data)", Docs: "RSA PKCS#1 v1.5 encrypt/decrypt with a PEM key."},
	"sign":             {Name: "sign", Group: "crypto", Signature: "k.sign(data, key[, alg])", Docs: "RSA signature (default SHA-256); returns base64."},
	"verify":           {Name: "verify", Group: "crypto", Signature: "k.verify(data, signature, key[, alg])", Docs: "Verifies an RSA signature; returns true/false."},

	// tier-2 files (zip)
	"zip_list":    {Name: "zip_list", Group: "files", Signature: "k.zip_list(zipPath)", Docs: "Lists the member names of a zip archive."},
	"zip_add":     {Name: "zip_add", Group: "files", Signature: "k.zip_add(zipPath, entries)", Docs: "Writes a zip archive from {name=content} entries."},
	"zip_extract": {Name: "zip_extract", Group: "files", Signature: "k.zip_extract(zipPath, dir)", Docs: "Extracts a zip archive into dir; returns file count."},

	// tier-2 data formats
	"csv_parse":   {Name: "csv_parse", Group: "formats", Signature: "k.csv_parse(text[, opts])", Docs: "Parses CSV (opts {header, sep, quote})."},
	"csv_string":  {Name: "csv_string", Group: "formats", Signature: "k.csv_string(data[, opts])", Docs: "Serializes CSV from a table."},
	"csv_load":    {Name: "csv_load", Group: "formats", Signature: "k.csv_load(path[, opts])", Docs: "Reads and parses a CSV file."},
	"csv_save":    {Name: "csv_save", Group: "formats", Signature: "k.csv_save(path, data[, opts])", Docs: "Writes a CSV file atomically."},
	"ini_parse":   {Name: "ini_parse", Group: "formats", Signature: "k.ini_parse(text)", Docs: "Parses INI into {section={key=value}, _root={...}}."},
	"ini_string":  {Name: "ini_string", Group: "formats", Signature: "k.ini_string(data)", Docs: "Serializes INI from a table."},
	"ini_load":    {Name: "ini_load", Group: "formats", Signature: "k.ini_load(path)", Docs: "Reads and parses an INI file."},
	"ini_save":    {Name: "ini_save", Group: "formats", Signature: "k.ini_save(path, data)", Docs: "Writes an INI file atomically."},
	"ini_read":    {Name: "ini_read", Group: "formats", Signature: "k.ini_read(path, section, key)", Docs: "Reads a single INI key (Kalipso parity)."},
	"ini_write":   {Name: "ini_write", Group: "formats", Signature: "k.ini_write(path, section, key, value)", Docs: "Writes a single INI key (Kalipso parity)."},
	"yaml_parse":  {Name: "yaml_parse", Group: "formats", Signature: "k.yaml_parse(text)", Docs: "Parses YAML; multi-document input yields a list of tables."},
	"yaml_string": {Name: "yaml_string", Group: "formats", Signature: "k.yaml_string(data)", Docs: "Serializes a value as YAML."},
	"yaml_load":   {Name: "yaml_load", Group: "formats", Signature: "k.yaml_load(path)", Docs: "Reads and parses a YAML file."},
	"yaml_save":   {Name: "yaml_save", Group: "formats", Signature: "k.yaml_save(path, data)", Docs: "Writes a YAML file atomically."},
	"xml_load":    {Name: "xml_load", Group: "formats", Signature: "k.xml_load(path)", Docs: "Reads an XML file into the element-table shape {_name,_attrs,_children,_text}."},
	"xml_save":    {Name: "xml_save", Group: "formats", Signature: "k.xml_save(path, table)", Docs: "Writes an element-table as XML."},

	// tier-2 rows conversions
	"json_to_rows": {Name: "json_to_rows", Group: "rows", Signature: "k.json_to_rows(value)", Docs: "Converts a JSON array of row-maps into {columns, rows}."},
	"rows_to_json": {Name: "rows_to_json", Group: "rows", Signature: "k.rows_to_json(result)", Docs: "Extracts the rows array from a result set."},
	"csv_to_rows":  {Name: "csv_to_rows", Group: "rows", Signature: "k.csv_to_rows(csvTable)", Docs: "Converts a parsed CSV table into {columns, rows}."},
	"rows_to_csv":  {Name: "rows_to_csv", Group: "rows", Signature: "k.rows_to_csv(result[, opts])", Docs: "Serializes a result set as CSV."},
	"xml_to_rows":  {Name: "xml_to_rows", Group: "rows", Signature: "k.xml_to_rows(document)", Docs: "Converts an XML element-table into {columns, rows}."},
	"rows_to_xml":  {Name: "rows_to_xml", Group: "rows", Signature: "k.rows_to_xml(result[, rootName[, rowName]])", Docs: "Serializes a result set as XML."},
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

// ExprFuncs documents the §5.9 expression functions installed as flat globals
// (not under k.*) so expressions read like Kalipso. This is static tooling
// data; the globals are installed by registerExprFuncs in Setup.
var ExprFuncs = []Info{
	// string
	{Name: "left", Group: "string", Signature: "left(s, n)", Docs: "Returns the first n characters of s."},
	{Name: "right", Group: "string", Signature: "right(s, n)", Docs: "Returns the last n characters of s."},
	{Name: "middle", Group: "string", Signature: "middle(s, start, count)", Docs: "Returns count characters of s starting at 1-based start."},
	{Name: "length", Group: "string", Signature: "length(s)", Docs: "Returns the byte length of s (Lua # semantics)."},
	{Name: "replace", Group: "string", Signature: "replace(s, old, new)", Docs: "Replaces all occurrences of old in s with new."},
	{Name: "trim", Group: "string", Signature: "trim(s)", Docs: "Removes leading/trailing whitespace from s."},
	{Name: "upper", Group: "string", Signature: "upper(s)", Docs: "Converts s to uppercase."},
	{Name: "lower", Group: "string", Signature: "lower(s)", Docs: "Converts s to lowercase."},
	{Name: "find", Group: "string", Signature: "find(s, needle[, start])", Docs: "Returns the 1-based position of needle in s (0 if absent)."},
	{Name: "string_count", Group: "string", Signature: "string_count(s, needle)", Docs: "Counts non-overlapping occurrences of needle in s."},
	{Name: "complete", Group: "string", Signature: "complete(s, length[, pad])", Docs: "Pads s on the right to length with pad (default space)."},
	{Name: "ascii", Group: "string", Signature: "ascii(ch)", Docs: "Returns the numeric code of the first byte of ch."},
	{Name: "charact", Group: "string", Signature: "charact(code)", Docs: "Returns the byte corresponding to code."},
	{Name: "base64_encode", Group: "string", Signature: "base64_encode(s)", Docs: "Encodes s as base64."},
	{Name: "base64_decode", Group: "string", Signature: "base64_decode(s)", Docs: "Decodes base64 into a string."},
	{Name: "urlencode", Group: "string", Signature: "urlencode(s)", Docs: "URL-encodes s (query style, spaces become +)."},
	{Name: "urldecode", Group: "string", Signature: "urldecode(s)", Docs: "Decodes a URL-encoded string."},
	{Name: "encode", Group: "string", Signature: "encode(s)", Docs: "Alias of urlencode."},
	{Name: "decode", Group: "string", Signature: "decode(s)", Docs: "Alias of urldecode."},
	{Name: "full_encode", Group: "string", Signature: "full_encode(s)", Docs: "Percent-encodes every non-unreserved byte."},
	{Name: "jsonencode", Group: "string", Signature: "jsonencode(value)", Docs: "Encodes a value as compact JSON (same as k.json_string)."},
	{Name: "jsondecode", Group: "string", Signature: "jsondecode(text)", Docs: "Parses JSON text (same as k.json_parse)."},
	{Name: "xmlencode", Group: "string", Signature: "xmlencode(s)", Docs: "Escapes XML special characters."},
	{Name: "xmldecode", Group: "string", Signature: "xmldecode(s)", Docs: "Unescapes XML entities."},
	{Name: "guid", Group: "string", Signature: "guid()", Docs: "Generates a random RFC 4122 version-4 UUID."},
	{Name: "extract_string", Group: "string", Signature: "extract_string(s, start, end)", Docs: "Returns s from 1-based start to end inclusive."},
	{Name: "set_string", Group: "string", Signature: "set_string(s, start, count, new)", Docs: "Replaces count characters of s at start with new."},
	{Name: "file_extract_part", Group: "string", Signature: "file_extract_part(path, part)", Docs: "Extracts path/name/ext from a file path."},
	{Name: "mltext", Group: "string", Signature: "mltext(...)", Docs: "Joins arguments with newlines (multi-line text)."},

	// numeric
	{Name: "abs", Group: "numeric", Signature: "abs(x)", Docs: "Absolute value of x."},
	{Name: "round", Group: "numeric", Signature: "round(x[, decimals])", Docs: "Rounds x to decimals (default 0) with half-away-from-zero."},
	{Name: "floor", Group: "numeric", Signature: "floor(x)", Docs: "Largest integer <= x."},
	{Name: "ceiling", Group: "numeric", Signature: "ceiling(x)", Docs: "Smallest integer >= x."},
	{Name: "power", Group: "numeric", Signature: "power(x, y)", Docs: "x raised to the power y."},
	{Name: "nth_root", Group: "numeric", Signature: "nth_root(x, n)", Docs: "The n-th root of x."},
	{Name: "sqrt", Group: "numeric", Signature: "sqrt(x)", Docs: "Square root of x."},
	{Name: "exp", Group: "numeric", Signature: "exp(x)", Docs: "e raised to x."},
	{Name: "log", Group: "numeric", Signature: "log(x)", Docs: "Natural logarithm of x."},
	{Name: "log10", Group: "numeric", Signature: "log10(x)", Docs: "Base-10 logarithm of x."},
	{Name: "sin", Group: "numeric", Signature: "sin(x)", Docs: "Sine of x (radians)."},
	{Name: "cos", Group: "numeric", Signature: "cos(x)", Docs: "Cosine of x (radians)."},
	{Name: "tan", Group: "numeric", Signature: "tan(x)", Docs: "Tangent of x (radians)."},
	{Name: "asin", Group: "numeric", Signature: "asin(x)", Docs: "Arcsine in radians."},
	{Name: "acos", Group: "numeric", Signature: "acos(x)", Docs: "Arccosine in radians."},
	{Name: "atan", Group: "numeric", Signature: "atan(x)", Docs: "Arctangent in radians."},
	{Name: "deg2rad", Group: "numeric", Signature: "deg2rad(x)", Docs: "Converts degrees to radians."},
	{Name: "rad2deg", Group: "numeric", Signature: "rad2deg(x)", Docs: "Converts radians to degrees."},
	{Name: "bitwise_and", Group: "numeric", Signature: "bitwise_and(a, b)", Docs: "Bitwise AND of the integer parts of a and b."},
	{Name: "bitwise_or", Group: "numeric", Signature: "bitwise_or(a, b)", Docs: "Bitwise OR of the integer parts of a and b."},
	{Name: "bitwise_xor", Group: "numeric", Signature: "bitwise_xor(a, b)", Docs: "Bitwise XOR of the integer parts of a and b."},
	{Name: "random", Group: "numeric", Signature: "random() / random(max) / random(min, max)", Docs: "Random float in [0,1), integer in [1,max], or in [min,max)."},
	{Name: "int_part", Group: "numeric", Signature: "int_part(x)", Docs: "Integer part of x (truncated)."},
	{Name: "dec_part", Group: "numeric", Signature: "dec_part(x)", Docs: "Fractional part of x."},
	{Name: "mask_number", Group: "numeric", Signature: "mask_number(x, mask)", Docs: "Formats x using a mask (#, 0, comma grouping, decimal)."},
	{Name: "val", Group: "numeric", Signature: "val(x)", Docs: "Numeric value of x; 0 when not numeric."},
	{Name: "sum", Group: "numeric", Signature: "sum(table)", Docs: "Sum of the numeric values in a table."},
	{Name: "extractstringd", Group: "numeric", Signature: "extractstringd(s)", Docs: "Digits of s as a number."},

	// conditional
	{Name: "lookup", Group: "conditional", Signature: "lookup(key, k1, v1, ...)", Docs: "Returns the value whose key equals key; \"\" when absent."},
	{Name: "yesno", Group: "conditional", Signature: "yesno(cond, a, b)", Docs: "Returns a when cond is Kalipso-truthy, else b."},
	{Name: "iif", Group: "conditional", Signature: "iif(cond, a, b)", Docs: "Inline if; returns a when cond is truthy, else b."},

	// datetime
	{Name: "sys_date", Group: "datetime", Signature: "sys_date()", Docs: "Today's date as \"YYYY-MM-DD\"."},
	{Name: "sys_time", Group: "datetime", Signature: "sys_time()", Docs: "Current time as \"HH:MM:SS\"."},
	{Name: "day", Group: "datetime", Signature: "day(date)", Docs: "Day of month of a date string."},
	{Name: "month", Group: "datetime", Signature: "month(date)", Docs: "Month (1-12) of a date string."},
	{Name: "year", Group: "datetime", Signature: "year(date)", Docs: "Year of a date string."},
	{Name: "hour", Group: "datetime", Signature: "hour(time)", Docs: "Hour of a date/time string."},
	{Name: "minute", Group: "datetime", Signature: "minute(time)", Docs: "Minute of a date/time string."},
	{Name: "second", Group: "datetime", Signature: "second(time)", Docs: "Second of a date/time string."},
	{Name: "add_days", Group: "datetime", Signature: "add_days(date, n)", Docs: "Date n days later as \"YYYY-MM-DD\"."},
	{Name: "subtract_days", Group: "datetime", Signature: "subtract_days(date, n)", Docs: "Date n days earlier as \"YYYY-MM-DD\"."},
	{Name: "date_diff", Group: "datetime", Signature: "date_diff(d2, d1)", Docs: "Whole days between two date strings (d2 minus d1)."},
	{Name: "datetime_add", Group: "datetime", Signature: "datetime_add(dt, days[, hours[, minutes[, seconds]]])", Docs: "Adds a duration to a datetime string."},
	{Name: "datetime_sub", Group: "datetime", Signature: "datetime_sub(dt, days[, hours[, minutes[, seconds]]])", Docs: "Subtracts a duration from a datetime string."},
	{Name: "datetime_diff", Group: "datetime", Signature: "datetime_diff(dt2, dt1)", Docs: "Seconds between two datetime strings."},
	{Name: "date_to_string", Group: "datetime", Signature: "date_to_string(date[, format])", Docs: "Formats a date using %Y %m %d etc.; default \"YYYY-MM-DD\"."},
	{Name: "time_to_string", Group: "datetime", Signature: "time_to_string(time[, format])", Docs: "Formats a time using %H %M %S etc.; default \"HH:MM:SS\"."},
	{Name: "week_day", Group: "datetime", Signature: "week_day(date)", Docs: "Day of week as 1-7 (Sunday=1)."},
	{Name: "week_number", Group: "datetime", Signature: "week_number(date)", Docs: "ISO week number of a date."},
	{Name: "tick_count", Group: "datetime", Signature: "tick_count()", Docs: "Unix milliseconds since the epoch."},
	{Name: "julian", Group: "datetime", Signature: "julian(date)", Docs: "Julian day number of a date."},
	{Name: "utc_to_local", Group: "datetime", Signature: "utc_to_local(dt)", Docs: "Converts a UTC datetime string to local wall-clock."},
	{Name: "local_to_utc", Group: "datetime", Signature: "local_to_utc(dt)", Docs: "Converts a local datetime string to UTC."},

	// conversion
	{Name: "tostr", Group: "conversion", Signature: "tostr(x)", Docs: "Kalipso string form of x."},
	{Name: "tonum", Group: "conversion", Signature: "tonum(x)", Docs: "Kalipso number of x; 0 when not numeric."},
	{Name: "todate", Group: "conversion", Signature: "todate(s)", Docs: "Normalizes s to \"YYYY-MM-DD\"."},
	{Name: "strtodate", Group: "conversion", Signature: "strtodate(s[, format])", Docs: "Parses s (optionally with %-tokens) and emits \"YYYY-MM-DD\"."},
	{Name: "boolstr", Group: "conversion", Signature: "boolstr(v)", Docs: "{\"true\",\"false\"} for the Kalipso truthiness of v."},
}

// ExprInfo returns a copy of the expression-function documentation.
func ExprInfo() []Info {
	out := make([]Info, len(ExprFuncs))
	copy(out, ExprFuncs)
	return out
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
