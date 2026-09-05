# KALUA API Reference

> Auto-generated from `internal/bindings/api_doc.go`. Do not edit manually.
> Run `make gen-api` to regenerate.

## k.* Bindings

### Flow

**`k.bell()`**  
Plays a system beep sound via WebAudio.

**`k.clipboard_get()`**  
Reads text from the browser clipboard.

**`k.clipboard_set(text)`**  
Writes text to the browser clipboard.

**`k.error(msg)`**  
Raises a deliberate Lua error.

**`k.http_request(optsTable)`**  
Makes an HTTP request. opts: {method, url, headers, body, timeout}. Returns {status, headers, body}.

**`k.locale()`**  
Returns the session locale ("en-US" default).

**`k.msgbox(text[, kind])`**  
Shows a message box; kind defaults to "info". Returns user's choice.

**`k.net_ok(timeout_ms)`**  
Reports internet reachability via a TCP dial.

**`k.param_get(key)`**  
Reads a persisted app param (string; "" if unset).

**`k.param_set(key, value)`**  
Persists an app param (string) to an app-side file.

**`k.pick_file([opts])`**  
Opens a browser file picker dialog. opts (optional table): {accept="image/*,.pdf", multiple=true}. Returns a table of files: {{name, size, type, data}, ...} where data is base64-encoded. Returns nil on cancel.

**`k.ping(host, timeout_ms)`**  
TCP-based latency probe returning ms, or nil when unreachable.

**`k.print(...)`**  
Prints values to the app log (tab-separated, like Lua print).

**`k.quit()`**  
Requests a clean termination of the app.

**`k.screen_size()`**  
Returns viewport dimensions as {width, height}.

**`k.sleep(ms)`**  
Suspends the script for ms milliseconds.

**`k.status_close()`**  
Hides the status bar.

**`k.status_show(text)`**  
Shows a busy/status bar with the given text.

**`k.timer_start(id, ms[, repeats])`**  
Starts a session timer; fires a Lua function named id (repeats optional).

**`k.timer_stop(id)`**  
Stops a running session timer.

### Debug

**`k.debug`**  
Runtime introspection helpers: stack/locals/trace.

**`k.debug.locals([level])`**  
Returns a table of local name → value for the given frame level (default 1).

**`k.debug.stack()`**  
Returns a table of the current call frames, each with level, name, source, line and locals.

**`k.debug.trace([msg])`**  
Logs a script-side trace anchor when verbose tracing is enabled.

### Forms

**`k.form.clear(name)`**  
Clears a form's control values.

**`k.form.close([name])`**  
Closes the top form, or the named form.

**`k.form.new(name, optsTable)`**  
Declares a form. optsTable may set title and layout.

**`k.form.on(form, ctrl, event, fn)`**  
Registers an event handler (e.g. event "onclick") for a control.

**`k.form.refresh(name)`**  
Re-renders and pushes the form to the browser.

**`k.form.return_to(name)`**  
Closes all forms above name.

**`k.form.show(name)`**  
Shows a form (modal) and suspends the script until it closes.

### Controls

**`k.ctrl.button(form, name, optsTable)`**  
Adds a button control. opts may set label, class, onclick, enabled.

**`k.ctrl.checkbox(form, name, optsTable)`**  
Adds a checkbox control.

**`k.ctrl.combo(form, name, optsTable)`**  
Adds a combo (dropdown) control. opts.items is a table of choices.

**`k.ctrl.get_property(form, name, prop)`**  
Gets an arbitrary control property.

**`k.ctrl.get_value(form, name)`**  
Returns a control's current value.

**`k.ctrl.label(form, name, optsTable)`**  
Adds a label control to a form.

**`k.ctrl.list(form, name, optsTable)`**  
Adds a multi-row select list. opts.items is a table of choices.

**`k.ctrl.radio(form, name, optsTable)`**  
Adds a radio button control.

**`k.ctrl.refresh(form, name)`**  
Re-renders a single control and pushes the update.

**`k.ctrl.set_focus(form, name)`**  
Moves focus to a control in the browser.

**`k.ctrl.set_property(form, name, prop, value)`**  
Sets an arbitrary control property.

**`k.ctrl.set_value(form, name, value)`**  
Sets a control's value and re-renders it.

**`k.ctrl.table(form, name, optsTable)`**  
Adds a table control; rows manipulated via k.table.*.

**`k.ctrl.textbox(form, name, optsTable)`**  
Adds a textbox control. opts may set label, value, enabled, visible.

**`k.table.add_line(form, name, valuesTable)`**  
Appends a row to a table control.

**`k.table.delete_line(form, name, index)`**  
Removes the row at index.

**`k.table.get_column_value(form, name, row, column)`**  
Gets a cell value from a table control.

**`k.table.get_data(form, name)`**  
Returns all current data of a table control.

**`k.table.get_selected_column(form, name)`**  
Gets the currently selected column.

**`k.table.get_selected_rows(form, name)`**  
Returns the selected row indices (1-based).

**`k.table.set_column_value(form, name, row, column, value)`**  
Sets a cell value in a table control.

**`k.table.set_data(form, name, dataTable)`**  
Bulk replaces all row data (Tabulator mode pushes tabulator_update).

**`k.table.set_selected_column(form, name, column)`**  
Sets the selected column.

### Database

**`k.connect_db(dsn)`**  
Opens a database connection (DSN scheme: sqlite://, mysql://, postgres://, sqlserver://) and returns a handle.

**`k.connect_sqlite(path)`**  
Opens a SQLite database file; returns a handle usable with k.sql/k.db_*.

**`k.db_delete(handle, table, whereTable)`**  
Deletes rows matching the where table.

**`k.db_insert(handle, table, keyvalsTable)`**  
Inserts a row; returns {last_insert_id, rows_affected}.

**`k.db_kill_table(handle, table, where)`**  
Deletes rows matching the where table.

**`k.db_proc(handle, name, ...params)`**  
Executes a stored procedure.

**`k.db_select(handle, table, fieldsTable, whereTable, order)`**  
Query builder returning {columns, rows}.

**`k.db_update(handle, table, keyvalsTable, whereTable)`**  
Updates rows matching the where table.

**`k.disconnect_db([handle])`**  
Closes a connection, or all connections when no handle is given.

**`k.disconnect_sqlite([handle])`**  
Closes a SQLite connection (or all).

**`k.rows(result)`**  
Returns an iterator over a query result's rows.

**`k.sql(handle, query, ...params)`**  
Executes arbitrary SQL; returns rows or {rows_affected}.

**`k.tx_begin(handle)`**  
Starts a transaction on a connection.

**`k.tx_commit(handle)`**  
Commits the active transaction.

**`k.tx_rollback(handle)`**  
Rolls back the active transaction.

### Files

**`k.file_close(handle)`**  
Closes an open file handle.

**`k.file_copy(src, dst)`**  
Copies a file, preserving permissions.

**`k.file_delete(path)`**  
Deletes a file.

**`k.file_exists(path)`**  
Reports whether a path exists.

**`k.file_info(path)`**  
Returns {name, size, is_dir, modified} for a path.

**`k.file_list(dir)`**  
Lists a directory as a 1-based, sorted table of names.

**`k.file_load(path)`**  
Reads an entire file as a string (async; max 16 MiB).

**`k.file_mkdir(path[, parents])`**  
Creates a directory; parents=true creates intermediate dirs.

**`k.file_move(src, dst)`**  
Moves/renames a file.

**`k.file_open(path[, mode])`**  
Opens a file; mode is r, r+, w, w+, a or a+ (default r). Returns a handle.

**`k.file_read(handle[, count])`**  
Reads the whole file or count bytes; empty string at EOF.

**`k.file_read_line(handle)`**  
Reads one line (trailing newline trimmed); nil at EOF.

**`k.file_save(path, data)`**  
Writes a file atomically (async; temp file + rename).

**`k.file_write(handle, data)`**  
Writes data to an open file.

**`k.zip_add(zipPath, entries)`**  
Writes a zip archive from {name=content} entries.

**`k.zip_extract(zipPath, dir)`**  
Extracts a zip archive into dir; returns file count.

**`k.zip_list(zipPath)`**  
Lists the member names of a zip archive.

### Json

**`k.is_null(value)`**  
Reports whether value is the K.NULL sentinel.

**`k.json_array_item(root, path, index)`**  
Returns the element at a 0-based array index.

**`k.json_count(root, path)`**  
Returns the element count (array length or object size).

**`k.json_get(root, path)`**  
Walks a dot/bracket path over a parsed value, e.g. "a.b[0].c".

**`k.json_load(path)`**  
Reads and parses a JSON file (async; max 16 MiB).

**`k.json_names(root, path)`**  
Returns a 1-based table of keys or indices.

**`k.json_parse(text)`**  
Parses JSON text; null maps to K.NULL.

**`k.json_save(path, value)`**  
Encodes a value and writes it atomically (async).

**`k.json_string(value)`**  
Encodes a value as compact JSON (sorted keys).

### Crypto

**`k.checksum(alg, data[, key[, salt[, iterations[, keylen]]]])`**  
Hex hash for alg: crc32, md5, sha1, sha256, hmac-sha256 (requires key), pbkdf2 (requires salt).

**`k.crypt_asymmetric(alg, key, data)`**  
RSA PKCS#1 v1.5 encrypt/decrypt with a PEM key.

**`k.crypt_symmetric(alg, key, data[, iv])`**  
AES-CBC symmetric encrypt/decrypt (alg aes-encrypt/aes-decrypt). Returns base64.

**`k.decrypt(b64, key)`**  
Reverse of k.encrypt.

**`k.encrypt(plaintext, key)`**  
AES-GCM encryption; returns base64(nonce || ciphertext).

**`k.sign(data, key[, alg])`**  
RSA signature (default SHA-256); returns base64.

**`k.verify(data, signature, key[, alg])`**  
Verifies an RSA signature; returns true/false.

### Xml

**`k.xml_attr(doc, path, name)`**  
Returns attribute value at path.

**`k.xml_attrs(doc, path)`**  
Returns all attributes of element at path as a table.

**`k.xml_child(doc, path)`**  
Returns child element at path (e.g., "book/author").

**`k.xml_child_list(doc, path)`**  
Returns a table of child elements at path.

**`k.xml_content(doc, path)`**  
Returns text content of element at path.

**`k.xml_name(doc, path)`**  
Returns the name of the element at path.

**`k.xml_parse(text)`**  
Parses XML text and returns a document handle.

**`k.xml_root(doc)`**  
Returns the root element name of a parsed document.

### Server

**`k.shared.del(key)`**  
Deletes a key from shared state.

**`k.shared.get(key)`**  
Retrieves a value from shared state; empty string if missing.

**`k.shared.incr(key[, delta])`**  
Increments a numeric key by delta (default 1); returns new value.

**`k.shared.keys([pattern])`**  
Returns all keys matching pattern (prefix, * = all).

**`k.shared.set(key, value)`**  
Stores a string value in shared state.

**`k.tcp.close(client_id)`**  
Closes a TCP connection.

**`k.tcp.send(client_id, data)`**  
Sends data to a specific TCP client.

**`k.ws.broadcast(message)`**  
Broadcasts a text message to all connected WebSocket clients.

**`k.ws.close(client_id)`**  
Closes a WebSocket connection.

**`k.ws.send(client_id, message)`**  
Sends a text message to a specific WebSocket client.

### Comm

**`k.ftp_connect(host[, port, user, pw])`**  
Connects to an FTP server; returns a handle.

**`k.ftp_create_dir(handle, path)`**  
Creates a remote directory (MKD).

**`k.ftp_delete(handle, path)`**  
Deletes a remote file (DELE).

**`k.ftp_disconnect(handle)`**  
Sends QUIT and closes the FTP connection.

**`k.ftp_file_exists(handle, path)`**  
Reports whether a remote file exists (SIZE).

**`k.ftp_get_file(handle, remote, local)`**  
Downloads remote to a local path (RETR).

**`k.ftp_list(handle[, path])`**  
Lists remote entry names (LIST).

**`k.ftp_put_file(handle, local, remote)`**  
Uploads a local file to remote (STOR).

**`k.ftp_rename(handle, from, to)`**  
Renames a remote file or folder (RNFR/RNTO).

**`k.ftp_set_cwd(handle, path)`**  
Changes the remote working directory (CWD).

**`k.socket_close(handle)`**  
Closes an open socket.

**`k.socket_open(host, port[, timeout_ms])`**  
Opens a TCP client connection; returns a handle.

**`k.socket_read(handle[, count])`**  
Reads count bytes (or all until close) from a socket.

**`k.socket_read_line(handle)`**  
Reads one line (trailing newline trimmed); nil at EOF.

**`k.socket_write(handle, data)`**  
Writes data to an open socket; returns bytes written.

**`k.webservice_run(profile, params)`**  
Calls a SOAP web service. profile: {url,action[,method,timeout_ms]}; params is the body table. Returns {status,headers,body}.

### Email

**`k.pop3_connect{host,port,user,pw,tls}`**  
Connects to a POP3 server; returns a handle.

**`k.pop3_dele(handle, index)`**  
Marks a message for deletion.

**`k.pop3_list(handle)`**  
Returns a table of {id,size} message summaries.

**`k.pop3_noop(handle)`**  
Keeps the POP3 connection alive.

**`k.pop3_quit(handle)`**  
Sends QUIT and closes the POP3 connection.

**`k.pop3_retr(handle, index)`**  
Retrieves a message by index.

**`k.pop3_stat(handle)`**  
Returns {count,size} of the mailbox.

**`k.smtp_connect{host,port,user,pw,tls}`**  
Connects to an SMTP server; returns a handle.

**`k.smtp_disconnect([handle])`**  
Closes an SMTP connection (or all).

**`k.smtp_send(handle, {from,to,subject,body,attachments})`**  
Sends an email through the connected SMTP server.

### Formats

**`k.csv_load(path[, opts])`**  
Reads and parses a CSV file.

**`k.csv_parse(text[, opts])`**  
Parses CSV (opts {header, sep, quote}).

**`k.csv_save(path, data[, opts])`**  
Writes a CSV file atomically.

**`k.csv_string(data[, opts])`**  
Serializes CSV from a table.

**`k.ini_load(path)`**  
Reads and parses an INI file.

**`k.ini_parse(text)`**  
Parses INI into {section={key=value}, _root={...}}.

**`k.ini_read(path, section, key)`**  
Reads a single INI key (Kalipso parity).

**`k.ini_save(path, data)`**  
Writes an INI file atomically.

**`k.ini_string(data)`**  
Serializes INI from a table.

**`k.ini_write(path, section, key, value)`**  
Writes a single INI key (Kalipso parity).

**`k.xml_load(path)`**  
Reads an XML file into the element-table shape {_name,_attrs,_children,_text}.

**`k.xml_save(path, table)`**  
Writes an element-table as XML.

**`k.yaml_load(path)`**  
Reads and parses a YAML file.

**`k.yaml_parse(text)`**  
Parses YAML; multi-document input yields a list of tables.

**`k.yaml_save(path, data)`**  
Writes a YAML file atomically.

**`k.yaml_string(data)`**  
Serializes a value as YAML.

### Rows

**`k.csv_to_rows(csvTable)`**  
Converts a parsed CSV table into {columns, rows}.

**`k.json_to_rows(value)`**  
Converts a JSON array of row-maps into {columns, rows}.

**`k.rows_to_csv(result[, opts])`**  
Serializes a result set as CSV.

**`k.rows_to_json(result)`**  
Extracts the rows array from a result set.

**`k.rows_to_xml(result[, rootName[, rowName]])`**  
Serializes a result set as XML.

**`k.xml_to_rows(document)`**  
Converts an XML element-table into {columns, rows}.

## K.* Helpers & Constants

**`K.EQ`**  
Operator constant "=".

**`K.NEQ`**  
Operator constant "<>".

**`K.ADD`**  
Operator constant "+".

**`K.eq(a, b)`**  
Kalipso equality (numeric when both coerce, else string compare).

**`K.ne(a, b)`**  
Negation of K.eq.

**`K.add(a, b)`**  
Kalipso addition: numeric if both coerce, else concatenation.

**`K.tonum(x)`**  
Coerces to a number, else 0.

**`K.tostr(x)`**  
Coerces to the Kalipso string form.

**`K.truthy(x)`**  
Kalipso condition test (0/"0"/""/nil are false).

**`K.NULL`**  
Sentinel representing a JSON null.

**`K.is_null(value)`**  
Reports whether value is K.NULL.

## Expression Functions (§5.9)

> Flat globals (not under `k.*`) — Kalipso-style expressions.

### String

**`ascii(ch)`**  
Returns the numeric code of the first byte of ch.

**`base64_decode(s)`**  
Decodes base64 into a string.

**`base64_encode(s)`**  
Encodes s as base64.

**`charact(code)`**  
Returns the byte corresponding to code.

**`complete(s, length[, pad])`**  
Pads s on the right to length with pad (default space).

**`decode(s)`**  
Alias of urldecode.

**`encode(s)`**  
Alias of urlencode.

**`extract_string(s, start, end)`**  
Returns s from 1-based start to end inclusive.

**`file_extract_part(path, part)`**  
Extracts path/name/ext from a file path.

**`find(s, needle[, start])`**  
Returns the 1-based position of needle in s (0 if absent).

**`full_encode(s)`**  
Percent-encodes every non-unreserved byte.

**`guid()`**  
Generates a random RFC 4122 version-4 UUID.

**`jsondecode(text)`**  
Parses JSON text (same as k.json_parse).

**`jsonencode(value)`**  
Encodes a value as compact JSON (same as k.json_string).

**`left(s, n)`**  
Returns the first n characters of s.

**`length(s)`**  
Returns the byte length of s (Lua # semantics).

**`lower(s)`**  
Converts s to lowercase.

**`middle(s, start, count)`**  
Returns count characters of s starting at 1-based start.

**`mltext(...)`**  
Joins arguments with newlines (multi-line text).

**`replace(s, old, new)`**  
Replaces all occurrences of old in s with new.

**`right(s, n)`**  
Returns the last n characters of s.

**`set_string(s, start, count, new)`**  
Replaces count characters of s at start with new.

**`string_count(s, needle)`**  
Counts non-overlapping occurrences of needle in s.

**`trim(s)`**  
Removes leading/trailing whitespace from s.

**`upper(s)`**  
Converts s to uppercase.

**`urldecode(s)`**  
Decodes a URL-encoded string.

**`urlencode(s)`**  
URL-encodes s (query style, spaces become +).

**`xmldecode(s)`**  
Unescapes XML entities.

**`xmlencode(s)`**  
Escapes XML special characters.

### Numeric

**`abs(x)`**  
Absolute value of x.

**`acos(x)`**  
Arccosine in radians.

**`asin(x)`**  
Arcsine in radians.

**`atan(x)`**  
Arctangent in radians.

**`bitwise_and(a, b)`**  
Bitwise AND of the integer parts of a and b.

**`bitwise_or(a, b)`**  
Bitwise OR of the integer parts of a and b.

**`bitwise_xor(a, b)`**  
Bitwise XOR of the integer parts of a and b.

**`ceiling(x)`**  
Smallest integer >= x.

**`cos(x)`**  
Cosine of x (radians).

**`dec_part(x)`**  
Fractional part of x.

**`deg2rad(x)`**  
Converts degrees to radians.

**`exp(x)`**  
e raised to x.

**`extractstringd(s)`**  
Digits of s as a number.

**`floor(x)`**  
Largest integer <= x.

**`int_part(x)`**  
Integer part of x (truncated).

**`log(x)`**  
Natural logarithm of x.

**`log10(x)`**  
Base-10 logarithm of x.

**`mask_number(x, mask)`**  
Formats x using a mask (#, 0, comma grouping, decimal).

**`nth_root(x, n)`**  
The n-th root of x.

**`power(x, y)`**  
x raised to the power y.

**`rad2deg(x)`**  
Converts radians to degrees.

**`random() / random(max) / random(min, max)`**  
Random float in [0,1), integer in [1,max], or in [min,max).

**`round(x[, decimals])`**  
Rounds x to decimals (default 0) with half-away-from-zero.

**`sin(x)`**  
Sine of x (radians).

**`sqrt(x)`**  
Square root of x.

**`sum(table)`**  
Sum of the numeric values in a table.

**`tan(x)`**  
Tangent of x (radians).

**`val(x)`**  
Numeric value of x; 0 when not numeric.

### Conditional

**`iif(cond, a, b)`**  
Inline if; returns a when cond is truthy, else b.

**`lookup(key, k1, v1, ...)`**  
Returns the value whose key equals key; "" when absent.

**`yesno(cond, a, b)`**  
Returns a when cond is Kalipso-truthy, else b.

### Datetime

**`add_days(date, n)`**  
Date n days later as "YYYY-MM-DD".

**`date_diff(d2, d1)`**  
Whole days between two date strings (d2 minus d1).

**`date_to_string(date[, format])`**  
Formats a date using %Y %m %d etc.; default "YYYY-MM-DD".

**`datetime_add(dt, days[, hours[, minutes[, seconds]]])`**  
Adds a duration to a datetime string.

**`datetime_diff(dt2, dt1)`**  
Seconds between two datetime strings.

**`datetime_sub(dt, days[, hours[, minutes[, seconds]]])`**  
Subtracts a duration from a datetime string.

**`day(date)`**  
Day of month of a date string.

**`hour(time)`**  
Hour of a date/time string.

**`julian(date)`**  
Julian day number of a date.

**`local_to_utc(dt)`**  
Converts a local datetime string to UTC.

**`minute(time)`**  
Minute of a date/time string.

**`month(date)`**  
Month (1-12) of a date string.

**`second(time)`**  
Second of a date/time string.

**`subtract_days(date, n)`**  
Date n days earlier as "YYYY-MM-DD".

**`sys_date()`**  
Today's date as "YYYY-MM-DD".

**`sys_time()`**  
Current time as "HH:MM:SS".

**`tick_count()`**  
Unix milliseconds since the epoch.

**`time_to_string(time[, format])`**  
Formats a time using %H %M %S etc.; default "HH:MM:SS".

**`utc_to_local(dt)`**  
Converts a UTC datetime string to local wall-clock.

**`week_day(date)`**  
Day of week as 1-7 (Sunday=1).

**`week_number(date)`**  
ISO week number of a date.

**`year(date)`**  
Year of a date string.

### Conversion

**`boolstr(v)`**  
{"true","false"} for the Kalipso truthiness of v.

**`strtodate(s[, format])`**  
Parses s (optionally with %-tokens) and emits "YYYY-MM-DD".

**`todate(s)`**  
Normalizes s to "YYYY-MM-DD".

**`tonum(x)`**  
Kalipso number of x; 0 when not numeric.

**`tostr(x)`**  
Kalipso string form of x.

## Script Globals

- **`ARGS`** — Table seeded from `--arg K=V` flags (string keys).
- **`CTRL`** — Accessor: `CTRL(name)` returns a control handle for `k.ctrl.*` operations.
- **`main`** — Entry point function (required in run mode).

