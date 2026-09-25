# KALUA Quick Reference

> Top-usage subset. Full API: `api.md`. Auto-generated — do not edit.

## K.* coercion & expression idioms

- `K.tonum(x)` — number of x, else 0
- `K.tostr(x)` — Kalipso string form of x
- `K.truthy(x)` — condition test (0, empty string and nil are false)
- `K.eq(a,b) / K.ne(a,b)` — numeric compare when both coerce, else string
- `K.add(a,b)` — sum if both numeric, else concatenate
- `tostr(x) / tonum(x)` — string / number coercion (0 if not numeric)
- `todate(s) / strtodate(s[,fmt])` — normalize to YYYY-MM-DD
- `boolstr(v)` — true/false (Kalipso truthiness)
- `left/right/middle(s,..) length(s)` — substring + length
- `upper(s) lower(s) trim(s)` — case + whitespace
- `replace(s,old,new) find(s,needle[,start])` — replace all / 1-based position
- `string_count(s,needle) complete(s,len[,pad])` — count / right-pad
- `base64_encode/decode(s) urlencode/decode(s)` — encoding helpers
- `jsonencode(v) jsondecode(s)` — compact JSON (== k.json_string/parse)
- `round(x[,d]) floor(x) ceiling(x) abs(x)` — numeric helpers
- `power(x,y) sqrt(16) random([a[,b]])` — math / random float or int range
- `iif(cond,a,b) yesno(cond,a,b)` — inline conditional
- `lookup(key, k1,v1,k2,v2,..)` — key → value, "" when absent
- `sys_date() sys_time()` — today YYYY-MM-DD / now HH:MM:SS
- `day/mon/year/hour/min/sec(dt) date_diff(d2,d1)` — date fields / whole days
- `add_days(d,n) subtract_days(d,n)` — date arithmetic
- `datetime_add/sub(dt, days[,h[,m[,s]]])` — datetime arithmetic
- `val(x) sum(tbl) mask_number(x, #,##0.00)` — numeric idioms

## k.* bindings (run + serve)

- `k.form.new(name, opts)` — declare form + title/align/layout
- `k.form.show(name, [options]) k.form.close([name])` — visibility (options: `{modal=true|false, gap=num|{x,y}}`)
- `k.form.on(form, ctrl, event, fn)` — control event handler (onclick, changed...)
- `k.form.on(name, event, fn) | (name, on_idle, ms, fn)` — form-level / idle timer
- `k.form.set_property(form, prop, v) / get_property` — form props
- `k.ctrl.label/textbox/button(form, name, opts)` — common controls
- `k.ctrl.combo/list/table/checkbox/radio(form, name, opts)` — selection + data controls
- `k.ctrl.looper(form, name, opts)` — DB row-template list (opts.db/row/links)
- `k.ctrl.chart(form, name, opts)` — Chart.js canvas control
- `k.ctrl.image(form, name, opts)` — image control (src/alt/width/height/fit)
- `k.ctrl.set_value(form, name, v) / get_value` — control value
- `k.ctrl.set_property(form, name, prop, v) / get_property` — control props (incl. cell/align)
- `k.msgbox(opts) / k.msgbox(text[,kind])` — modal message box, returns choice
- `k.popup(items)` — multi-level menu, returns picked value
- `k.print(...) k.sleep(ms) k.yield()` — log / pause / cooperative yield
- `k.quit() k.error(msg) k.on_error(fn)` — lifecycle + error handling
- `k.http_request({method,url,headers,body,timeout})` — async HTTP client → {status,headers,body}
- `k.pick_file([opts])` — open/save/download file picker (base64 data)
- `k.clipboard_set(s) k.clipboard_get() k.bell()` — browser helpers
- `k.json_parse(s) k.json_string(v) k.json_load/save(path)` — JSON
- `k.csv_parse(s[..parse opts]) k.csv_string(rows)` — CSV
- `k.xml_parse(s) k.xml_root/child/attr(s..)` — XML
- `k.file_load(path) k.file_save(path, data)` — whole-file read/write
- `k.file_exists/dir/delete/list/copy/move(path..)` — file ops
- `k.zip_add/extract/list(zip, ...)` — zip helper
- `k.connect_db(dsn) k.disconnect_db([h])` — open/close (sqlite,mysql,pg,mssql)
- `k.sql(h, select ..., ...) k.rows(res)` — raw query + row list
- `k.db_select/insert/update/delete(h, tbl, ...)` — CRUD by table+maps
- `k.connect_sqlite(path) k.db_kill_table(h, tbl, where)` — sqlite quick connect / truncate
- `k.shared.set/get/del/keys/incr(key[,..])` — cross-worker state
- `k.ws.broadcast(s) k.ws.send(id, s) k.ws.close(id)` — WebSocket out
- `k.tcp.send(id, data) k.tcp.close(id) k.tcp.accept()` — TCP out / accept connection

## Golden loop

`KALUA check app.lua → run --test → serve --test` (append `--json`) · exit: 0 ok / 1 error / 2 usage / 3 io.
Run-mode apps: `function main()` with `k.form.new/show` + `k.form.on` handlers. Serve-mode apps: `handle_http(req)`, `handle_ws(msg)`, `handle_tcp(msg)`, optional `init`/`shutdown`.
Prefer `k.*`/`K.*`/expression functions over generic Lua whenever an equivalent exists.
