# KALUA API Reference

> Auto-generated from `internal/bindings/api_doc.go`. Do not edit manually.
> Run `make gen-api` to regenerate.

## k.* Bindings

### Flow

**`k.assign(target, kind, value)`**  
Sets a global variable or control value with type coercion. target: string (global name) or table {form=, ctrl=}. kind: "numeric"|"string"|"boolean"|"date". Returns the coerced value.

**`k.bell()`**  
Plays a system beep sound via WebAudio.

**`k.clipboard_get()`**  
Reads text from the browser clipboard.

**`k.clipboard_set(text)`**  
Writes text to the browser clipboard.

**`k.error(msg)`**  
Raises a deliberate Lua error.

| Parameter | Type | Description |
|-----------|------|-------------|
| `msg` | string | Error message raised to the caller. |

**Example:**

```lua
k.error("download failed")
```

**`k.exec(name, ...)`**  
Executes a previously stored function (via k.set) asynchronously with the given arguments. Returns the function's result(s).

**`k.http_request(optsTable)`**  
Makes an HTTP request asynchronously; returns {status, headers, body}. Suspends the script until the response arrives.

| Parameter | Type | Description |
|-----------|------|-------------|
| `optsTable.method` | string | HTTP method; default "GET". |
| `optsTable.url` | string | Full request URL. |
| `optsTable.headers` | table | Request headers as {name = value, ...}. |
| `optsTable.body` | string | Request body for POST/PUT. |
| `optsTable.timeout` | number | Timeout in milliseconds. |

**Example:**

```lua
local res = k.http_request{ method = "GET", url = "https://api.example.com/x" }
k.print(res.status, res.body)
```

**`k.locale()`**  
Returns the session locale ("en-US" default).

**`k.msgbox(opts)`**  
Shows a message box and returns the clicked button's value. Legacy form: `k.msgbox(text[, kind])` where kind is `info`/`warn`/`error`/`ok-cancel`/`yes-no` (returns `"ok"`, `"cancel"`, `"yes"`, `"no"`). Rich form takes a single options table: `type` sets the left color strip (`info` blue, `warning` amber, `danger` red); `buttons` is a list of `{label, value}` pairs (values keep their type: number, boolean or string), `{label=…, value=…}` tables, or bare strings (label = value). When omitted, a single `OK` button returning `"ok"` is added.

| Parameter | Type | Description |
|-----------|------|-------------|
| `title` | string | Optional dialog title; empty hides the header. |
| `message` | string | Body text; empty hides the text. |
| `type` | string | "info" (default), "warning", or "danger" — sets the left color strip. |
| `buttons` | list | {label, value} entries (or bare strings); default single OK. |

**Example:**

```lua
local choice = k.msgbox{
  title   = "Confirm delete",
  message = "Delete row 42?",
  type    = "warning",                    -- "info" | "warning" | "danger"
  buttons = { {"Delete", 1}, {"Keep", 0}, {"Cancel", false} },
}
-- choice == 1, 0, false, or nil
```

**`k.net_ok(timeout_ms)`**  
Reports internet reachability via a TCP dial.

**`k.on_error(fn)`**  
Registers (or clears, with nil) the Kalipso error hook. When ERRORCODE is set — by a failing binding or a genuine Lua error — fn is called with (ERRORCODE, ERRORMSG) so the script can show the error and continue. Erroring bindings return nil; branch on ERRORCODE.

| Parameter | Type | Description |
|-----------|------|-------------|
| `fn` | function | Error handler invoked as fn(ERRORCODE, ERRORMSG); pass nil to clear. |

**Example:**

```lua
k.on_error(function(code, msg)
  k.msgbox{title="Error", message=msg, type="danger"}
end)
```

**`k.param_get(key)`**  
Reads a persisted app param (string; "" if unset).

**`k.param_set(key, value)`**  
Persists an app param (string) to an app-side file.

**`k.pick_file([opts])`**  
Opens a browser file picker dialog. mode="open" (default): picks existing files and returns a table of {{name, size, type, data}, ...} with base64-encoded data. mode="save": shows a save dialog with a filename and returns {path, name}. mode="download": triggers a download of base64 data and returns {path, name}. Returns nil on cancel. Suspends the script until the dialog completes.

| Parameter | Type | Description |
|-----------|------|-------------|
| `opts.accept` | string | Comma-separated accepted types, e.g. "image/*,.pdf". |
| `opts.multiple` | boolean | Allow choosing several files at once. |
| `opts.mode` | string | "open" (default), "save" or "download". |
| `opts.filename` | string | Suggested name for save/download modes. |
| `opts.data` | string | Base64 content to save when mode = "download". |

**Example:**

```lua
local files = k.pick_file{ accept = "image/*,.pdf", multiple = true }  -- nil on cancel
```

**`k.ping(host, timeout_ms)`**  
TCP-based latency probe returning ms, or nil when unreachable.

**`k.popup(items[, opts])`**  
Shows a multilevel menu-style popup (centered modal, fly-out submenus) and returns the picked item's value with its type preserved, or nil when dismissed (Esc / click outside). Items are leaves (label, value) or branches ({label, items={...}} which only open a fly-out submenu, up to 8 levels). Leaves accept {label, value} pairs, {label=…, value=…} tables, or bare strings (label = value); values round-trip typed (number, boolean, string, table). Alternative list form (no title): k.popup{ {"Open", "open"}, {"Quit"} }.

| Parameter | Type | Description |
|-----------|------|-------------|
| `title` | string | Optional menu header (options-table form). |
| `items` | list | Menu items: leaves {label, value} / bare strings, or branches {label, items={...}}. |

**Example:**

```lua
local pick = k.popup{
  title = "Maintenance",
  items = {
    { label = "File", items = {                       -- branch -> hover/click to open
        { label = "Open",   value = "open" },
        { label = "Recent", items = {
            { label = "a.lua", value = "r1" },
            { label = "b.lua", value = "r2" },
        }},
    }},
    { label = "Refresh", value = 1 },                  -- leaf -> returns 1
    { "Quit" },                                        -- bare string -> returns "Quit"
  },
}
if pick == nil then ... end                            -- dismissed
```

**`k.print(...)`**  
Prints values to the app log (tab-separated, like Lua print).

| Parameter | Type | Description |
|-----------|------|-------------|
| `...` | any | Values to print; joined with tabs into the app log. |

**Example:**

```lua
k.print("avg =", 3.5)
```

**`k.quit()`**  
Requests a clean termination of the app.

**Example:**

```lua
k.quit()
```

**`k.screen_size()`**  
Returns viewport dimensions as {width, height}.

**`k.set(name, fn)`**  
Stores a function in the action registry for later execution via k.exec.

**`k.sleep(ms)`**  
Suspends the script for ms milliseconds.

| Parameter | Type | Description |
|-----------|------|-------------|
| `ms` | number | Milliseconds to pause before resuming. |

**Example:**

```lua
k.sleep(2000)  -- wait 2 seconds
```

**`k.status_close()`**  
Hides the status bar.

**`k.status_show(text)`**  
Shows a busy/status bar with the given text.

**`k.timer_start(id, ms[, repeats])`**  
Starts a session timer; fires a Lua function named id every ms (repeats times when given, else forever).

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string | Timer id; a Lua function with this global name is invoked on each tick. |
| `ms` | number | Interval in milliseconds. |
| `repeats` | number | Optional tick count; omitted runs forever. |

**Example:**

```lua
k.timer_start("refresh", 1000)  -- calls refresh() every second
function refresh() k.form.refresh("main") end
```

**`k.timer_stop(id)`**  
Stops a running session timer.

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string | Timer id passed to k.timer_start. |

**Example:**

```lua
k.timer_stop("refresh")
```

**`k.yield()`**  
Yields the current coroutine, allowing other coroutines to run.

**Example:**

```lua
k.yield()
```

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

| Parameter | Type | Description |
|-----------|------|-------------|
| `name` | string | Form name to clear. |

**Example:**

```lua
k.form.clear("main")
```

**`k.form.close([name])`**  
Closes the top form, or the named form.

| Parameter | Type | Description |
|-----------|------|-------------|
| `name` | string | Optional form name; omitted closes the top form. |

**Example:**

```lua
k.form.close()
```

**`k.form.new(name, optsTable)`**  
Declares a form. opts: {title, layout=vertical|grid, align=left|center|right, gap=n px, cells}. grid cells: {id={width 1-12, bg, border={width,color}, align}} or ordered array of {id,...}; assign controls via control opt cell="id" and override alignment via align (kforms_enhancements §6).

| Parameter | Type | Description |
|-----------|------|-------------|
| `name` | string | Unique form name. |
| `optsTable.title` | string | Title shown in the form header. |
| `optsTable.layout` | string | "vertical" (default) or "grid". |
| `optsTable.align` | string | "left" (default), "center" or "right" — control alignment in vertical layout. |
| `optsTable.gap` | number | Gap between controls/cells in px. |
| `optsTable.cells` | table | Grid cells: ordered {id, width, bg, border, align} array or {id = {width, bg, border, align}} map. |

**Example:**

```lua
k.form.new("main", {
  title  = "Hello",
  layout = "vertical",            -- or "grid" with a cells table
  align  = "center",
})
```

**`k.form.on(form, ctrl, event, fn) | k.form.on(name, event, fn) | k.form.on(name, "on_idle", ms, fn)`**  
Registers an event handler. 4-arg form: control handler (form, ctrl, event, fn) for control events (e.g. "onclick"). 3-arg form: form-level handler (name, event, fn) for events like open_form, after_open_form, close_form, key_pressed (used as fallback when no control handler exists). 4-arg form with a number: k.form.on(name, "on_idle", ms, fn) sets a periodic idle callback (ms interval, default 1000) — only the topmost form receives idle events.

| Parameter | Type | Description |
|-----------|------|-------------|
| `form` | string | Form name (4-arg control handler). |
| `ctrl` | string | Control name (4-arg control handler). |
| `name` | string | Form name (3-arg form-level handler). |
| `event` | string | Event name: "onclick", "open_form", "after_open_form", "close_form", "key_pressed", "on_idle", chart events, ... |
| `ms` | number | Idle callback interval in ms (on_idle form). |
| `fn` | function | Handler function invoked with the event payload. |

**Example:**

```lua
k.form.on("main", "btn_ok", "onclick", function()
  k.form.show("details")
end)
k.form.on("main", "on_idle", 500, function() ... end)
```

**`k.form.refresh(name)`**  
Re-renders and pushes the form to the browser.

| Parameter | Type | Description |
|-----------|------|-------------|
| `name` | string | Form name to re-render. |

**Example:**

```lua
k.form.refresh("main")
```

**`k.form.return_to(name)`**  
Closes all forms above name (returns to it).

| Parameter | Type | Description |
|-----------|------|-------------|
| `name` | string | Form name to return to. |

**Example:**

```lua
k.form.return_to("main")
```

**`k.form.show(name)`**  
Shows a form (modal) and suspends the script until it closes.

| Parameter | Type | Description |
|-----------|------|-------------|
| `name` | string | Form name declared with k.form.new. |

**Example:**

```lua
k.form.show("main")
```

**`k.get_property(form, prop)`**  
Reads a form-level property; returns nil when the property or form is not set.

| Parameter | Type | Description |
|-----------|------|-------------|
| `form` | string | Form name declared with k.form.new. |
| `prop` | string | Property name to read. |

**Example:**

```lua
local bg = k.get_property("main", "bg")
```

**`k.set_property(form, prop, value)`**  
Sets a form-level property and re-renders the form. Supports title, align, gap and dynamic styling props: bg (background color), color (text color), font (CSS font-family or an h1–h6/p text preset), font_size (numeric px, overrides preset), style (h1–h6/p text preset). The reserved keys name, controls, handlers and order cannot be set.

| Parameter | Type | Description |
|-----------|------|-------------|
| `form` | string | Form name declared with k.form.new. |
| `prop` | string | Property name: "title", "align", "gap", "bg", "color", "font", "font_size", "style", ... |
| `value` | any | Property value (string or number). |

**Example:**

```lua
k.set_property("main", "bg", "#1e3a5f")
k.set_property("main", "font", "h2")
k.set_property("main", "title", "Welcome")
```

### Controls

**`k.chart`**  
Chart control operations: k.chart.set_data/add_dataset/...

**`k.chart.add_dataset(form, name, dataset)`**  
Appends a dataset {label, data, backgroundColor?, borderColor?, fill?, tension?, ...} to a chart.

| Parameter | Type | Description |
|-----------|------|-------------|
| `form` | string | Parent form name. |
| `name` | string | Chart control name. |
| `dataset` | table | Dataset: {label, data, backgroundColor, borderColor, fill, tension, ...}. |

**Example:**

```lua
k.chart.add_dataset("dash", "trend", { label = "Clicks", data = {5, 8, 12} })
```

**`k.chart.get_image(form, name)`**  
Renders the chart canvas to a base64 PNG data URL.

| Parameter | Type | Description |
|-----------|------|-------------|
| `form` | string | Parent form name. |
| `name` | string | Chart control name. |

**Example:**

```lua
local png = k.chart.get_image("dash", "trend")  -- "data:image/png;base64,..."
```

**`k.chart.remove_dataset(form, name, index)`**  
Removes a dataset by 1-based index.

| Parameter | Type | Description |
|-----------|------|-------------|
| `form` | string | Parent form name. |
| `name` | string | Chart control name. |
| `index` | number | 1-based dataset index. |

**Example:**

```lua
k.chart.remove_dataset("dash", "trend", 1)
```

**`k.chart.resize(form, name, width, height)`**  
Resizes the chart canvas to the given pixel dimensions.

| Parameter | Type | Description |
|-----------|------|-------------|
| `form` | string | Parent form name. |
| `name` | string | Chart control name. |
| `width` | number | New width in px. |
| `height` | number | New height in px. |

**Example:**

```lua
k.chart.resize("dash", "trend", 800, 400)
```

**`k.chart.set_data(form, name, {labels, datasets})`**  
Bulk replaces a chart's labels and datasets.

| Parameter | Type | Description |
|-----------|------|-------------|
| `form` | string | Parent form name. |
| `name` | string | Chart control name. |
| `opts.labels` | list | New X-axis labels. |
| `opts.datasets` | list | New dataset tables. |

**Example:**

```lua
k.chart.set_data("dash", "trend", { labels = {"A","B"}, datasets = { {label = "S", data = {1,2}} } })
```

**`k.chart.set_labels(form, name, labels)`**  
Replaces the chart's X-axis labels (array).

| Parameter | Type | Description |
|-----------|------|-------------|
| `form` | string | Parent form name. |
| `name` | string | Chart control name. |
| `labels` | list | New label array. |

**Example:**

```lua
k.chart.set_labels("dash", "trend", {"Jan", "Feb", "Mar"})
```

**`k.chart.set_options(form, name, options)`**  
Merges Chart.js options (scales, plugins, ...) into the chart.

| Parameter | Type | Description |
|-----------|------|-------------|
| `form` | string | Parent form name. |
| `name` | string | Chart control name. |
| `options` | table | Chart.js options to deep-merge. |

**Example:**

```lua
k.chart.set_options("dash", "trend", { scales = { y = { beginAtZero = true } } })
```

**`k.chart.update_dataset(form, name, index, dataset)`**  
Replaces the dataset at 1-based index.

| Parameter | Type | Description |
|-----------|------|-------------|
| `form` | string | Parent form name. |
| `name` | string | Chart control name. |
| `index` | number | 1-based dataset index. |
| `dataset` | table | New dataset table. |

**Example:**

```lua
k.chart.update_dataset("dash", "trend", 2, { label = "B", data = {3, 4, 1} })
```

**`k.ctrl.button(form, name, optsTable)`**  
Adds a button control. opts may set label, class, onclick, enabled.

| Parameter | Type | Description |
|-----------|------|-------------|
| `form` | string | Parent form name. |
| `name` | string | Unique control name. |
| `optsTable.label` | string | Button text. |
| `optsTable.class` | string | CSS class applied to the button. |
| `optsTable.onclick` | string | Event name fired on click. |
| `optsTable.enabled` | boolean | Whether the button is clickable. |

**Example:**

```lua
k.ctrl.button("main", "btn_ok", { label = "OK", onclick = "ok_clicked" })
```

**`k.ctrl.chart(form, name, optsTable)`**  
Adds a Chart.js control. opts: {type=line|bar|hbar|pie|doughnut|scatter|radar|area, title, width=400, height=300, labels, datasets, options, responsive=true, maintainAspectRatio=false, legend=true, legendPosition=top, animation=true, stacked=false}. Events chart_click/chart_hover/chart_legend_click via k.form.on.

| Parameter | Type | Description |
|-----------|------|-------------|
| `form` | string | Parent form name. |
| `name` | string | Unique control name. |
| `optsTable.type` | string | "line", "bar", "hbar", "pie", "doughnut", "scatter", "radar" or "area". |
| `optsTable.title` | string | Chart title. |
| `optsTable.width` | number | Canvas width in px (default 400). |
| `optsTable.height` | number | Canvas height in px (default 300). |
| `optsTable.labels` | list | X-axis category labels. |
| `optsTable.datasets` | list | Dataset tables {label, data, ...}. |
| `optsTable.options` | table | Extra Chart.js options deep-merged over the defaults. |
| `optsTable.responsive` | boolean | Scale canvas to its container (default true). |
| `optsTable.legend` | boolean | Show the legend (default true). |
| `optsTable.legendPosition` | string | Legend position: "top" (default), "bottom", "left", "right". |
| `optsTable.animation` | boolean | Animate updates (default true). |
| `optsTable.stacked` | boolean | Stack datasets on the value axis (default false). |

**Example:**

```lua
k.ctrl.chart("dash", "trend", {
  type     = "line",
  labels   = {"Mon", "Tue", "Wed"},
  datasets = { { label = "Sales", data = {10, 20, 15} } },
})
```

**`k.ctrl.checkbox(form, name, optsTable)`**  
Adds a checkbox control.

| Parameter | Type | Description |
|-----------|------|-------------|
| `form` | string | Parent form name. |
| `name` | string | Unique control name. |
| `optsTable.label` | string | Label text next to the box. |
| `optsTable.value` | boolean | Initial checked state. |

**Example:**

```lua
k.ctrl.checkbox("main", "agreed", { label = "I agree", value = false })
```

**`k.ctrl.combo(form, name, optsTable)`**  
Adds a combo (dropdown) control. opts.items is a table of choices.

| Parameter | Type | Description |
|-----------|------|-------------|
| `form` | string | Parent form name. |
| `name` | string | Unique control name. |
| `optsTable.items` | list | Drop-down choices (list of strings or {label, value} pairs). |
| `optsTable.label` | string | Label text above the control. |
| `optsTable.value` | any | Initial selected value. |

**Example:**

```lua
k.ctrl.combo("main", "color", { items = {"red", "green", "blue"}, value = "green" })
```

**`k.ctrl.execute_event(form, name, event)`**  
Fires a control's event handler as if the user triggered it (e.g., "onclick"). Runs asynchronously via the session actor.

| Parameter | Type | Description |
|-----------|------|-------------|
| `form` | string | Parent form name. |
| `name` | string | Control name. |
| `event` | string | Event name to fire, e.g. "onclick". |

**Example:**

```lua
k.ctrl.execute_event("main", "btn_ok", "onclick")
```

**`k.ctrl.get_item_count(form, name)`**  
Returns the number of items/rows in a combo, list, radio, or table control.

| Parameter | Type | Description |
|-----------|------|-------------|
| `form` | string | Parent form name. |
| `name` | string | Control name. |

**Example:**

```lua
local n = k.ctrl.get_item_count("main", "sel")
```

**`k.ctrl.get_property(form, name, prop)`**  
Gets an arbitrary control property.

| Parameter | Type | Description |
|-----------|------|-------------|
| `form` | string | Parent form name. |
| `name` | string | Control name. |
| `prop` | string | Property name. |

**Example:**

```lua
local label = k.ctrl.get_property("main", "btn", "label")
```

**`k.ctrl.get_selection(form, name)`**  
Returns the current selection as {start, end, text} from a textbox or textarea.

| Parameter | Type | Description |
|-----------|------|-------------|
| `form` | string | Parent form name. |
| `name` | string | Control name. |

**Example:**

```lua
local sel = k.ctrl.get_selection("main", "notes")  -- {start, end, text}
```

**`k.ctrl.get_value(form, name)`**  
Returns a control's current value.

| Parameter | Type | Description |
|-----------|------|-------------|
| `form` | string | Parent form name. |
| `name` | string | Control name. |

**Example:**

```lua
local age = k.ctrl.get_value("main", "age")
```

**`k.ctrl.image(form, name, optsTable)`**  
Adds an image control (<img>). opts: {src (required), alt, width, height (px or %), fit="cover|contain|fill|scale-down|none" (default contain), clickable?, onclick?}. k.ctrl.set_value(form, name, new_src) updates the image (kforms_enhancements.md §4.3).

| Parameter | Type | Description |
|-----------|------|-------------|
| `form` | string | Parent form name. |
| `name` | string | Unique control name. |
| `optsTable.src` | string | Image URL or data URI (required). |
| `optsTable.alt` | string | Accessibility/fallback text. |
| `optsTable.width` | string | Width (px or %). |
| `optsTable.height` | string | Height (px or %). |
| `optsTable.fit` | string | Object-fit: "cover", "contain" (default), "fill", "scale-down", "none". |
| `optsTable.clickable` | boolean | Enable click events on the image. |
| `optsTable.onclick` | string | Event name fired when the image is clicked. |

**Example:**

```lua
k.ctrl.image("main", "logo", { src = "/img/logo.png", width = "200px", fit = "contain", clickable = true, onclick = "logo_clicked" })
```

**`k.ctrl.label(form, name, optsTable)`**  
Adds a label control. opts: {text, multiline?:boolean, cell?, align?}. multiline renders a pre-wrap div preserving \n (kforms_enhancements.md §4.2). cell/align: grid layout assignment + alignment (kforms_enhancements.md §6).

| Parameter | Type | Description |
|-----------|------|-------------|
| `form` | string | Parent form name. |
| `name` | string | Unique control name within the form. |
| `optsTable.text` | string | Label text. |
| `optsTable.multiline` | boolean | Preserve newlines with white-space: pre-wrap. |
| `optsTable.cell` | string | Grid cell id to place the control in. |
| `optsTable.align` | string | Alignment override: "left", "center" or "right". |

**Example:**

```lua
k.ctrl.label("main", "lbl", { text = "Hello\nWorld", multiline = true })
```

**`k.ctrl.list(form, name, optsTable)`**  
Adds a multi-row select list. opts.items is a table of choices.

| Parameter | Type | Description |
|-----------|------|-------------|
| `form` | string | Parent form name. |
| `name` | string | Unique control name. |
| `optsTable.items` | list | List choices. |
| `optsTable.label` | string | Label text above the control. |

**Example:**

```lua
k.ctrl.list("main", "sel", { items = {"a", "b", "c"} })
```

**`k.ctrl.looper(form, name, optsTable)`**  
Adds a looper control (repeating row layout). DB-linked when opts carry {db,query,links,page_size?,count_query?,where?,order_by?}.

| Parameter | Type | Description |
|-----------|------|-------------|
| `form` | string | Parent form name. |
| `name` | string | Unique control name. |
| `optsTable.db` | string | DB handle for a DB-linked looper. |
| `optsTable.query` | string | SQL query for a DB-linked looper. |
| `optsTable.links` | list | Maps result columns to template controls. |
| `optsTable.page_size` | number | Rows per page for pagination. |

**Example:**

```lua
k.ctrl.looper("main", "rows", { db = h, query = "SELECT * FROM items", links = { {field = "name", control = "tpl_name", property = "text"} } })
```

**`k.ctrl.radio(form, name, optsTable)`**  
Adds a radio button control.

| Parameter | Type | Description |
|-----------|------|-------------|
| `form` | string | Parent form name. |
| `name` | string | Unique control name. |
| `optsTable.items` | list | List of radio options. |
| `optsTable.value` | any | Initially selected option value. |

**Example:**

```lua
k.ctrl.radio("main", "plan", { items = {"basic", "pro"}, value = "pro" })
```

**`k.ctrl.refresh(form, name)`**  
Re-renders a single control and pushes the update.

| Parameter | Type | Description |
|-----------|------|-------------|
| `form` | string | Parent form name. |
| `name` | string | Control name. |

**Example:**

```lua
k.ctrl.refresh("main", "grid")
```

**`k.ctrl.select_text(form, name)`**  
Selects all text in a textbox or textarea control.

| Parameter | Type | Description |
|-----------|------|-------------|
| `form` | string | Parent form name. |
| `name` | string | Control name. |

**Example:**

```lua
k.ctrl.select_text("main", "notes")
```

**`k.ctrl.set_focus(form, name)`**  
Moves focus to a control in the browser.

| Parameter | Type | Description |
|-----------|------|-------------|
| `form` | string | Parent form name. |
| `name` | string | Control name. |

**Example:**

```lua
k.ctrl.set_focus("main", "age")
```

**`k.ctrl.set_property(form, name, prop, value)`**  
Sets an arbitrary control property.

| Parameter | Type | Description |
|-----------|------|-------------|
| `form` | string | Parent form name. |
| `name` | string | Control name. |
| `prop` | string | Property name, e.g. "label", "enabled", "cell", "align". |
| `value` | any | New property value. |

**Example:**

```lua
k.ctrl.set_property("main", "btn", "enabled", false)
```

**`k.ctrl.set_selection(form, name, from, to)`**  
Sets the selection range in a textbox or textarea (0-based character offsets).

| Parameter | Type | Description |
|-----------|------|-------------|
| `form` | string | Parent form name. |
| `name` | string | Control name. |
| `from` | number | 0-based start offset. |
| `to` | number | 0-based end offset. |

**Example:**

```lua
k.ctrl.set_selection("main", "notes", 0, 3)
```

**`k.ctrl.set_value(form, name, value)`**  
Sets a control's value and re-renders it.

| Parameter | Type | Description |
|-----------|------|-------------|
| `form` | string | Parent form name. |
| `name` | string | Control name. |
| `value` | any | New value; for images the src (kforms_enhancements.md §4.3). |

**Example:**

```lua
k.ctrl.set_value("main", "age", 31)
```

**`k.ctrl.table(form, name, optsTable)`**  
Adds a table control; rows manipulated via k.table.*. With opts {db, query, ...} the table is DB-linked (Tabulator mode).

| Parameter | Type | Description |
|-----------|------|-------------|
| `form` | string | Parent form name. |
| `name` | string | Unique control name. |
| `optsTable.columns` | list | Column definitions (Tabulator mode). |
| `optsTable.db` | string | DB handle for a DB-linked table. |
| `optsTable.query` | string | SQL query for a DB-linked table. |

**Example:**

```lua
k.ctrl.table("main", "grid", { columns = { {field = "id", title = "ID"} } })
```

**`k.ctrl.textbox(form, name, optsTable)`**  
Adds a textbox control. opts: {label, value, enabled, visible, multiline?:boolean, rows?:number, cols?:number, datetime?:boolean|table, cell?, align?}. multiline renders a <textarea>. datetime enables a flatpickr picker: mode="date"|"time"|"datetime", format, min, max, step (kforms_enhancements.md §4.1). cell/align: grid layout assignment + alignment (kforms_enhancements.md §6).

| Parameter | Type | Description |
|-----------|------|-------------|
| `form` | string | Parent form name. |
| `name` | string | Unique control name within the form. |
| `optsTable.label` | string | Label text above the input. |
| `optsTable.value` | string | Initial value. |
| `optsTable.enabled` | boolean | Whether the control is editable. |
| `optsTable.visible` | boolean | Whether the control is shown. |
| `optsTable.multiline` | boolean | Render a <textarea>. |
| `optsTable.rows` | number | Textarea rows (multiline); default 4. |
| `optsTable.cols` | number | Textarea columns (multiline); default 50. |
| `optsTable.datetime` | table | Enable a flatpickr picker: {mode, format, min, max, step}. |
| `optsTable.cell` | string | Grid cell id to place the control in. |
| `optsTable.align` | string | Alignment override: "left", "center" or "right". |

**Example:**

```lua
k.ctrl.textbox("main", "age", { label = "Age", datetime = { mode = "date", format = "Y-m-d" } })
```

**`k.looper`**  
Looper control operations: k.looper.link_db/set_db_source/refresh/...

**`k.looper.add_line(form, name, valuesTable)`**  
Raises a runtime error on DB-linked loopers (rows come from the linked query).

| Parameter | Type | Description |
|-----------|------|-------------|
| `form` | string | Parent form name. |
| `name` | string | Looper control name. |
| `valuesTable` | table | Row values (unused on DB-linked loopers). |

**Example:**

```lua
k.looper.add_line("main", "rows", { name = "new" })
```

**`k.looper.delete_line(form, name, index)`**  
Raises a runtime error on DB-linked loopers (rows come from the linked query).

| Parameter | Type | Description |
|-----------|------|-------------|
| `form` | string | Parent form name. |
| `name` | string | Looper control name. |
| `index` | number | 1-based row index (ignored on DB-linked loopers). |

**Example:**

```lua
k.looper.delete_line("main", "rows", 2)
```

**`k.looper.link_db(form, name, opts)`**  
Attaches a DB source to a looper: {db,query,links,page_size?,count_query?,where?,order_by?}. links list maps result columns to template controls: {column=N,control,property} by 1-based index or {field,col,control,property} by name.

| Parameter | Type | Description |
|-----------|------|-------------|
| `form` | string | Parent form name. |
| `name` | string | Looper control name. |
| `opts.db` | string | DB handle. |
| `opts.query` | string | SQL query producing the rows. |
| `opts.links` | list | Column→control mappings: {field/column, control, property}. |
| `opts.page_size` | number | Rows per page for pagination. |
| `opts.count_query` | string | Optional query returning total row count. |
| `opts.where` | string | Optional WHERE clause appended to query. |
| `opts.order_by` | string | Optional ORDER BY clause. |

**Example:**

```lua
k.looper.link_db("main", "rows", {
  db = h, query = "SELECT * FROM items",
  links = { {field = "name", control = "tpl_name", property = "text"} },
})
```

**`k.looper.refresh(form, name)`**  
Re-runs a DB-linked looper's query and shows page 1.

| Parameter | Type | Description |
|-----------|------|-------------|
| `form` | string | Parent form name. |
| `name` | string | Looper control name. |

**Example:**

```lua
k.looper.refresh("main", "rows")
```

**`k.looper.set_db_source(form, name, opts)`**  
Swaps a DB-linked looper's source {db,query,links?,page_size?,count_query?,where?,order_by?} and refreshes.

| Parameter | Type | Description |
|-----------|------|-------------|
| `form` | string | Parent form name. |
| `name` | string | Looper control name. |
| `opts` | table | New source: {db, query, links, page_size, count_query, where, order_by}. |

**Example:**

```lua
k.looper.set_db_source("main", "rows", { db = h, query = "SELECT * FROM archived" })
```

**`k.looper.set_line(form, name, index, valuesTable)`**  
Raises a runtime error on DB-linked loopers (rows come from the linked query).

| Parameter | Type | Description |
|-----------|------|-------------|
| `form` | string | Parent form name. |
| `name` | string | Looper control name. |
| `index` | number | 1-based row index. |
| `valuesTable` | table | Row values (unused on DB-linked loopers). |

**Example:**

```lua
k.looper.set_line("main", "rows", 1, { name = "x" })
```

**`k.table.add_line(form, name, valuesTable)`**  
Appends a row to a table control.

| Parameter | Type | Description |
|-----------|------|-------------|
| `form` | string | Parent form name. |
| `name` | string | Table control name. |
| `valuesTable` | list | Cell values in column order. |

**Example:**

```lua
k.table.add_line("main", "grid", {"Alice", 32})
```

**`k.table.delete_line(form, name, index)`**  
Removes the row at index.

| Parameter | Type | Description |
|-----------|------|-------------|
| `form` | string | Parent form name. |
| `name` | string | Table control name. |
| `index` | number | 1-based row index. |

**Example:**

```lua
k.table.delete_line("main", "grid", 1)
```

**`k.table.find(form, name, value)`**  
Searches for a row where any cell equals value. Returns 1-based row index or nil. Works for both traditional tables and tabulator tables.

| Parameter | Type | Description |
|-----------|------|-------------|
| `form` | string | Parent form name. |
| `name` | string | Table control name. |
| `value` | any | Value to search for. |

**Example:**

```lua
local idx = k.table.find("main", "grid", "Alice")
```

**`k.table.get_column_value(form, name, row, column)`**  
Gets a cell value from a table control.

| Parameter | Type | Description |
|-----------|------|-------------|
| `form` | string | Parent form name. |
| `name` | string | Table control name. |
| `row` | number | 1-based row index. |
| `column` | number | 1-based column index. |

**Example:**

```lua
local v = k.table.get_column_value("main", "grid", 1, 2)
```

**`k.table.get_data(form, name)`**  
Returns all current data of a table control.

| Parameter | Type | Description |
|-----------|------|-------------|
| `form` | string | Parent form name. |
| `name` | string | Table control name. |

**Example:**

```lua
local rows = k.table.get_data("main", "grid")
```

**`k.table.get_selected_column(form, name)`**  
Gets the currently selected column.

| Parameter | Type | Description |
|-----------|------|-------------|
| `form` | string | Parent form name. |
| `name` | string | Table control name. |

**Example:**

```lua
local col = k.table.get_selected_column("main", "grid")
```

**`k.table.get_selected_rows(form, name)`**  
Returns the selected row indices (1-based).

| Parameter | Type | Description |
|-----------|------|-------------|
| `form` | string | Parent form name. |
| `name` | string | Table control name. |

**Example:**

```lua
local sel = k.table.get_selected_rows("main", "grid")  -- {1, 3}
```

**`k.table.refresh(form, name)`**  
Re-runs a DB-linked tabulator table's query and shows page 1.

| Parameter | Type | Description |
|-----------|------|-------------|
| `form` | string | Parent form name. |
| `name` | string | Table control name. |

**Example:**

```lua
k.table.refresh("main", "grid")
```

**`k.table.set_column_value(form, name, row, column, value)`**  
Sets a cell value in a table control.

| Parameter | Type | Description |
|-----------|------|-------------|
| `form` | string | Parent form name. |
| `name` | string | Table control name. |
| `row` | number | 1-based row index. |
| `column` | number | 1-based column index. |
| `value` | any | New cell value. |

**Example:**

```lua
k.table.set_column_value("main", "grid", 1, 2, 33)
```

**`k.table.set_data(form, name, dataTable)`**  
Bulk replaces all row data (Tabulator mode pushes tabulator_update).

| Parameter | Type | Description |
|-----------|------|-------------|
| `form` | string | Parent form name. |
| `name` | string | Table control name. |
| `dataTable` | list | Rows: list of value lists, or of row-maps (Tabulator mode). |

**Example:**

```lua
k.table.set_data("main", "grid", { {"Alice", 32}, {"Bob", 41} })
```

**`k.table.set_db_source(form, name, opts)`**  
Swaps a DB-linked tabulator table's source {db,query,columns?,page_size?,count_query?,where?,order_by?} and refreshes.

| Parameter | Type | Description |
|-----------|------|-------------|
| `form` | string | Parent form name. |
| `name` | string | Table control name. |
| `opts` | table | New source: {db, query, columns, page_size, count_query, where, order_by}. |

**Example:**

```lua
k.table.set_db_source("main", "grid", { db = h, query = "SELECT * FROM archived" })
```

**`k.table.set_remote_data(form, name, {data,last_page,last_row})`**  
Pushes server-side pagination data to a tabulator table =  {data=rows, last_page=n} or {data=rows, last_row=n}.

| Parameter | Type | Description |
|-----------|------|-------------|
| `form` | string | Parent form name. |
| `name` | string | Table control name. |
| `opts.data` | list | Page rows. |
| `opts.last_page` | number | Total page count (page-based pagination). |
| `opts.last_row` | number | Total row count (row-based pagination). |

**Example:**

```lua
k.table.set_remote_data("main", "grid", { data = rows, last_page = 5 })
```

**`k.table.set_selected_column(form, name, column)`**  
Sets the selected column.

| Parameter | Type | Description |
|-----------|------|-------------|
| `form` | string | Parent form name. |
| `name` | string | Table control name. |
| `column` | number | 1-based column index. |

**Example:**

```lua
k.table.set_selected_column("main", "grid", 2)
```

### Database

**`k.connect_db(dsn)`**  
Opens a database connection (DSN scheme: sqlite://, mysql://, postgres://, sqlserver://) and returns a handle. Supported drivers: SQLite (built-in), MySQL (github.com/go-sql-driver/mysql), PostgreSQL (github.com/jackc/pgx/v5/stdlib), SQL Server (github.com/microsoft/go-mssqldb).

| Parameter | Type | Description |
|-----------|------|-------------|
| `dsn` | string | Connection string, e.g. "sqlite://data.db" or "postgres://user:pw@host/db". |

**Example:**

```lua
local h = k.connect_db("sqlite:///tmp/app.db")
```

**`k.connect_sqlite(path)`**  
Opens a SQLite database file; returns a handle usable with k.sql/k.db_*.

| Parameter | Type | Description |
|-----------|------|-------------|
| `path` | string | SQLite database file path. |

**Example:**

```lua
local h = k.connect_sqlite("/tmp/app.db")
```

**`k.db_delete(handle, table, whereTable)`**  
Deletes rows matching the where table.

| Parameter | Type | Description |
|-----------|------|-------------|
| `handle` | string | Connection handle. |
| `table` | string | Table name. |
| `whereTable` | table | Equality filters {column = value}. |

**Example:**

```lua
k.db_delete(h, "items", { name = "widget" })
```

**`k.db_insert(handle, table, keyvalsTable)`**  
Inserts a row; returns {last_insert_id, rows_affected}.

| Parameter | Type | Description |
|-----------|------|-------------|
| `handle` | string | Connection handle. |
| `table` | string | Table name. |
| `keyvalsTable` | table | {column = value} map to insert. |

**Example:**

```lua
local r = k.db_insert(h, "items", { name = "widget", price = 9.9 })
```

**`k.db_kill_table(handle, table, where)`**  
Deletes rows matching the where table.

| Parameter | Type | Description |
|-----------|------|-------------|
| `handle` | string | Connection handle. |
| `table` | string | Table name. |
| `where` | table | Equality filters {column = value}. |

**Example:**

```lua
k.db_kill_table(h, "items", { expired = 1 })
```

**`k.db_proc(handle, name, ...params)`**  
Executes a stored procedure.

| Parameter | Type | Description |
|-----------|------|-------------|
| `handle` | string | Connection handle. |
| `name` | string | Stored procedure name. |
| `...params` | any | Procedure arguments. |

**Example:**

```lua
local res = k.db_proc(h, "sp_reprice", 1.1)
```

**`k.db_select(handle, table, fieldsTable, whereTable, order)`**  
Query builder returning {columns, rows}.

| Parameter | Type | Description |
|-----------|------|-------------|
| `handle` | string | Connection handle. |
| `table` | string | Table name. |
| `fieldsTable` | list | Columns to select (or "*" as a string). |
| `whereTable` | table | Equality filters {column = value}. |
| `order` | string | Optional ORDER BY clause. |

**Example:**

```lua
local res = k.db_select(h, "items", {"name", "price"}, {category = "x"}, "price DESC")
```

**`k.db_update(handle, table, keyvalsTable, whereTable)`**  
Updates rows matching the where table.

| Parameter | Type | Description |
|-----------|------|-------------|
| `handle` | string | Connection handle. |
| `table` | string | Table name. |
| `keyvalsTable` | table | {column = value} map to set. |
| `whereTable` | table | Equality filters {column = value}. |

**Example:**

```lua
k.db_update(h, "items", { price = 11.5 }, { name = "widget" })
```

**`k.disconnect_db([handle])`**  
Closes a connection, or all connections when no handle is given.

| Parameter | Type | Description |
|-----------|------|-------------|
| `handle` | string | Optional handle from k.connect_db; omitted closes all. |

**Example:**

```lua
k.disconnect_db(h)
```

**`k.disconnect_sqlite([handle])`**  
Closes a SQLite connection (or all).

| Parameter | Type | Description |
|-----------|------|-------------|
| `handle` | string | Optional handle from k.connect_sqlite; omitted closes all. |

**Example:**

```lua
k.disconnect_sqlite(h)
```

**`k.rows(result)`**  
Returns an iterator over a query result's rows.

| Parameter | Type | Description |
|-----------|------|-------------|
| `result` | table | Result set from k.sql/k.db_select. |

**Example:**

```lua
for row in k.rows(res) do k.print(row.name) end
```

**`k.sql(handle, query, ...params)`**  
Executes arbitrary SQL; returns rows or {rows_affected}.

| Parameter | Type | Description |
|-----------|------|-------------|
| `handle` | string | Connection handle. |
| `query` | string | SQL statement (may contain ? placeholders). |
| `...params` | any | Bind values for placeholders. |

**Example:**

```lua
local res = k.sql(h, "SELECT * FROM items WHERE stock < ?", 5)
```

**`k.tx_begin(handle)`**  
Starts a transaction on a connection.

| Parameter | Type | Description |
|-----------|------|-------------|
| `handle` | string | Connection handle. |

**Example:**

```lua
k.tx_begin(h)
```

**`k.tx_commit(handle)`**  
Commits the active transaction.

| Parameter | Type | Description |
|-----------|------|-------------|
| `handle` | string | Connection handle. |

**Example:**

```lua
k.tx_commit(h)
```

**`k.tx_rollback(handle)`**  
Rolls back the active transaction.

| Parameter | Type | Description |
|-----------|------|-------------|
| `handle` | string | Connection handle. |

**Example:**

```lua
k.tx_rollback(h)
```

### Files

**`k.file_close(handle)`**  
Closes an open file handle.

| Parameter | Type | Description |
|-----------|------|-------------|
| `handle` | string | Open file handle. |

**Example:**

```lua
k.file_close(f)
```

**`k.file_copy(src, dst)`**  
Copies a file, preserving permissions.

| Parameter | Type | Description |
|-----------|------|-------------|
| `src` | string | Source path. |
| `dst` | string | Destination path. |

**Example:**

```lua
k.file_copy("/tmp/a.txt", "/tmp/b.txt")
```

**`k.file_delete(path)`**  
Deletes a file.

| Parameter | Type | Description |
|-----------|------|-------------|
| `path` | string | Path to delete. |

**Example:**

```lua
k.file_delete("/tmp/junk.txt")
```

**`k.file_exists(path)`**  
Reports whether a path exists.

| Parameter | Type | Description |
|-----------|------|-------------|
| `path` | string | Path to test. |

**Example:**

```lua
if k.file_exists("/tmp/data.txt") then ... end
```

**`k.file_info(path)`**  
Returns {name, size, is_dir, modified} for a path.

| Parameter | Type | Description |
|-----------|------|-------------|
| `path` | string | Path to inspect. |

**Example:**

```lua
local info = k.file_info("/tmp/data.txt")
```

**`k.file_list(dir)`**  
Lists a directory as a 1-based, sorted table of names.

| Parameter | Type | Description |
|-----------|------|-------------|
| `dir` | string | Directory to list. |

**Example:**

```lua
for _, name in ipairs(k.file_list("/tmp")) do k.print(name) end
```

**`k.file_load(path)`**  
Reads an entire file as a string (async; max 16 MiB).

| Parameter | Type | Description |
|-----------|------|-------------|
| `path` | string | File path to read. |

**Example:**

```lua
local text = k.file_load("/tmp/data.txt")
```

**`k.file_mkdir(path[, parents])`**  
Creates a directory; parents=true creates intermediate dirs.

| Parameter | Type | Description |
|-----------|------|-------------|
| `path` | string | Directory to create. |
| `parents` | boolean | Create intermediate directories (default false). |

**Example:**

```lua
k.file_mkdir("/tmp/a/b/c", true)
```

**`k.file_move(src, dst)`**  
Moves/renames a file.

| Parameter | Type | Description |
|-----------|------|-------------|
| `src` | string | Source path. |
| `dst` | string | Destination path. |

**Example:**

```lua
k.file_move("/tmp/a.txt", "/tmp/b.txt")
```

**`k.file_open(path[, mode])`**  
Opens a file; mode is r, r+, w, w+, a or a+ (default r). Returns a handle.

| Parameter | Type | Description |
|-----------|------|-------------|
| `path` | string | File path to open. |
| `mode` | string | Access mode: r, r+, w, w+, a, a+ (default r). |

**Example:**

```lua
local f = k.file_open("/tmp/data.txt", "w")
```

**`k.file_read(handle[, count])`**  
Reads the whole file or count bytes; empty string at EOF.

| Parameter | Type | Description |
|-----------|------|-------------|
| `handle` | string | Open file handle. |
| `count` | number | Optional byte count to read. |

**Example:**

```lua
local all = k.file_read(f)
```

**`k.file_read_line(handle)`**  
Reads one line (trailing newline trimmed); nil at EOF.

| Parameter | Type | Description |
|-----------|------|-------------|
| `handle` | string | Open file handle. |

**Example:**

```lua
local line = k.file_read_line(f)
```

**`k.file_save(path, data)`**  
Writes a file atomically (async; temp file + rename).

| Parameter | Type | Description |
|-----------|------|-------------|
| `path` | string | Destination path. |
| `data` | string | Content to write. |

**Example:**

```lua
k.file_save("/tmp/out.txt", "content")
```

**`k.file_write(handle, data)`**  
Writes data to an open file.

| Parameter | Type | Description |
|-----------|------|-------------|
| `handle` | string | Open file handle. |
| `data` | string | Text to write. |

**Example:**

```lua
k.file_write(f, "hello\n")
```

**`k.zip_add(zipPath, entries)`**  
Writes a zip archive from {name=content} entries.

| Parameter | Type | Description |
|-----------|------|-------------|
| `zipPath` | string | Destination zip path. |
| `entries` | table | {memberName = content, ...} map. |

**Example:**

```lua
k.zip_add("bundle.zip", { "readme.txt" = "hello" })
```

**`k.zip_extract(zipPath, dir)`**  
Extracts a zip archive into dir; returns file count.

| Parameter | Type | Description |
|-----------|------|-------------|
| `zipPath` | string | Path of the zip archive. |
| `dir` | string | Destination directory. |

**Example:**

```lua
local n = k.zip_extract("bundle.zip", "/tmp/out")
```

**`k.zip_list(zipPath)`**  
Lists the member names of a zip archive.

| Parameter | Type | Description |
|-----------|------|-------------|
| `zipPath` | string | Path of the zip archive. |

**Example:**

```lua
for _, name in ipairs(k.zip_list("bundle.zip")) do k.print(name) end
```

### Json

**`k.is_null(value)`**  
Reports whether value is the K.NULL sentinel.

| Parameter | Type | Description |
|-----------|------|-------------|
| `value` | any | Value to test. |

**Example:**

```lua
if k.is_null(v) then ... end
```

**`k.json_array_item(root, path, index)`**  
Returns the element at a 0-based array index.

| Parameter | Type | Description |
|-----------|------|-------------|
| `root` | any | Parsed document. |
| `path` | string | Path to an array (or "" for root). |
| `index` | number | 0-based element index. |

**Example:**

```lua
local first = k.json_array_item(root, "tags", 0)
```

**`k.json_count(root, path)`**  
Returns the element count (array length or object size).

| Parameter | Type | Description |
|-----------|------|-------------|
| `root` | any | Parsed document. |
| `path` | string | Path to an array or object. |

**Example:**

```lua
local n = k.json_count(root, "items")
```

**`k.json_get(root, path)`**  
Walks a dot/bracket path over a parsed value, e.g. "a.b[0].c".

| Parameter | Type | Description |
|-----------|------|-------------|
| `root` | any | Parsed document (from k.json_parse/k.json_load). |
| `path` | string | Dot/bracket path, e.g. "a.b[0].c". |

**Example:**

```lua
local v = k.json_get(root, "a.b[0].c")
```

**`k.json_load(path)`**  
Reads and parses a JSON file (async; max 16 MiB).

| Parameter | Type | Description |
|-----------|------|-------------|
| `path` | string | Path of the JSON file. |

**Example:**

```lua
local root = k.json_load("config.json")
```

**`k.json_names(root, path)`**  
Returns a 1-based table of keys or indices.

| Parameter | Type | Description |
|-----------|------|-------------|
| `root` | any | Parsed document. |
| `path` | string | Path to an array or object. |

**Example:**

```lua
for _, key in ipairs(k.json_names(root, "items")) do ... end
```

**`k.json_parse(text)`**  
Parses JSON text; null maps to K.NULL.

| Parameter | Type | Description |
|-----------|------|-------------|
| `text` | string | JSON document to parse. |

**Example:**

```lua
local root = k.json_parse('{"name": "Alice", "age": 32}')
```

**`k.json_save(path, value)`**  
Encodes a value and writes it atomically (async).

| Parameter | Type | Description |
|-----------|------|-------------|
| `path` | string | Destination path. |
| `value` | any | Value to serialize. |

**Example:**

```lua
k.json_save("out.json", { ok = true })
```

**`k.json_string(value)`**  
Encodes a value as compact JSON (sorted keys).

| Parameter | Type | Description |
|-----------|------|-------------|
| `value` | any | Value to serialize (tables, strings, numbers, booleans, nil). |

**Example:**

```lua
local text = k.json_string({ name = "Alice", age = 32 })
```

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

| Parameter | Type | Description |
|-----------|------|-------------|
| `doc` | any | Document handle. |
| `path` | string | Element path. |
| `name` | string | Attribute name. |

**Example:**

```lua
local id = k.xml_attr(doc, "catalog/cd", "id")
```

**`k.xml_attrs(doc, path)`**  
Returns all attributes of element at path as a table.

| Parameter | Type | Description |
|-----------|------|-------------|
| `doc` | any | Document handle. |
| `path` | string | Element path. |

**Example:**

```lua
local attrs = k.xml_attrs(doc, "catalog/cd")
```

**`k.xml_child(doc, path)`**  
Returns child element at path (e.g., "book/author").

| Parameter | Type | Description |
|-----------|------|-------------|
| `doc` | any | Document handle. |
| `path` | string | Element path, e.g. "book/author". |

**Example:**

```lua
local el = k.xml_child(doc, "book/author")
```

**`k.xml_child_list(doc, path)`**  
Returns a table of child elements at path.

| Parameter | Type | Description |
|-----------|------|-------------|
| `doc` | any | Document handle. |
| `path` | string | Element path. |

**Example:**

```lua
local els = k.xml_child_list(doc, "catalog/cd")
```

**`k.xml_content(doc, path)`**  
Returns text content of element at path.

| Parameter | Type | Description |
|-----------|------|-------------|
| `doc` | any | Document handle. |
| `path` | string | Element path. |

**Example:**

```lua
local title = k.xml_content(doc, "catalog/cd/title")
```

**`k.xml_name(doc, path)`**  
Returns the name of the element at path.

| Parameter | Type | Description |
|-----------|------|-------------|
| `doc` | any | Document handle. |
| `path` | string | Element path. |

**Example:**

```lua
local name = k.xml_name(doc, "catalog/cd")
```

**`k.xml_parse(text)`**  
Parses XML text and returns a document handle.

| Parameter | Type | Description |
|-----------|------|-------------|
| `text` | string | XML document to parse. |

**Example:**

```lua
local doc = k.xml_parse("<book><author>A</author></book>")
```

**`k.xml_root(doc)`**  
Returns the root element name of a parsed document.

| Parameter | Type | Description |
|-----------|------|-------------|
| `doc` | any | Document handle from k.xml_parse. |

**Example:**

```lua
local name = k.xml_root(doc)  -- "book"
```

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

**`k.tcp.accept()`**  
Waits for an incoming TCP connection and returns {id}. The connection can then be used with k.tcp.send and k.tcp.close. (Serve mode only.)

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

**Example:**

```lua
ascii("A")  -- 65
```

**`base64_decode(s)`**  
Decodes base64 into a string.

**Example:**

```lua
base64_decode("aGk=")  -- "hi"
```

**`base64_encode(s)`**  
Encodes s as base64.

**Example:**

```lua
base64_encode("hi")  -- "aGk="
```

**`charact(code)`**  
Returns the byte corresponding to code.

**Example:**

```lua
charact(65)  -- "A"
```

**`complete(s, length[, pad])`**  
Pads s on the right to length with pad (default space).

**Example:**

```lua
complete("ab", 4, ".")  -- "ab.."
```

**`decode(s)`**  
Alias of urldecode.

**Example:**

```lua
decode("a+b")  -- "a b"
```

**`encode(s)`**  
Alias of urlencode.

**Example:**

```lua
encode("a b")  -- "a+b"
```

**`extract_string(s, start, end)`**  
Returns s from 1-based start to end inclusive.

**Example:**

```lua
extract_string("hello", 2, 4)  -- "ell"
```

**`file_extract_part(path, part)`**  
Extracts path/name/ext from a file path.

**Example:**

```lua
file_extract_part("/a/b.txt", "name")  -- "b"
```

**`find(s, needle[, start])`**  
Returns the 1-based position of needle in s (0 if absent).

**Example:**

```lua
find("hello", "ll")  -- 3
```

**`full_encode(s)`**  
Percent-encodes every non-unreserved byte.

**Example:**

```lua
full_encode("a b")  -- "a%20b"
```

**`guid()`**  
Generates a random RFC 4122 version-4 UUID.

**Example:**

```lua
guid()  -- "4f3c..."
```

**`jsondecode(text)`**  
Parses JSON text (same as k.json_parse).

**Example:**

```lua
jsondecode('{"a":1}').a  -- 1
```

**`jsonencode(value)`**  
Encodes a value as compact JSON (same as k.json_string).

**Example:**

```lua
jsonencode({ a = 1 })  -- '{"a":1}'
```

**`left(s, n)`**  
Returns the first n characters of s.

**Example:**

```lua
left("hello", 2)  -- "he"
```

**`length(s)`**  
Returns the byte length of s (Lua # semantics).

**Example:**

```lua
length("hello")  -- 5
```

**`lower(s)`**  
Converts s to lowercase.

**Example:**

```lua
lower("HI")  -- "hi"
```

**`middle(s, start, count)`**  
Returns count characters of s starting at 1-based start.

**Example:**

```lua
middle("hello", 2, 3)  -- "ell"
```

**`mltext(...)`**  
Joins arguments with newlines (multi-line text).

**Example:**

```lua
mltext("a", "b")  -- "a\nb"
```

**`replace(s, old, new)`**  
Replaces all occurrences of old in s with new.

**Example:**

```lua
replace("a-b-c", "-", ".")  -- "a.b.c"
```

**`right(s, n)`**  
Returns the last n characters of s.

**Example:**

```lua
right("hello", 2)  -- "lo"
```

**`set_string(s, start, count, new)`**  
Replaces count characters of s at start with new.

**Example:**

```lua
set_string("hello", 2, 3, "i")  -- "hi"
```

**`string_count(s, needle)`**  
Counts non-overlapping occurrences of needle in s.

**Example:**

```lua
string_count("bobobo", "bo")  -- 2
```

**`trim(s)`**  
Removes leading/trailing whitespace from s.

**Example:**

```lua
trim("  hi  ")  -- "hi"
```

**`upper(s)`**  
Converts s to uppercase.

**Example:**

```lua
upper("hi")  -- "HI"
```

**`urldecode(s)`**  
Decodes a URL-encoded string.

**Example:**

```lua
urldecode("a+b")  -- "a b"
```

**`urlencode(s)`**  
URL-encodes s (query style, spaces become +).

**Example:**

```lua
urlencode("a b")  -- "a+b"
```

**`xmldecode(s)`**  
Unescapes XML entities.

**Example:**

```lua
xmldecode("&lt;a&gt;")  -- "<a>"
```

**`xmlencode(s)`**  
Escapes XML special characters.

**Example:**

```lua
xmlencode("<a>")  -- "&lt;a&gt;"
```

### Numeric

**`abs(x)`**  
Absolute value of x.

**Example:**

```lua
abs(-3)  -- 3
```

**`acos(x)`**  
Arccosine in radians.

**Example:**

```lua
acos(1)  -- 0
```

**`asin(x)`**  
Arcsine in radians.

**Example:**

```lua
asin(1)  -- 1.570...
```

**`atan(x)`**  
Arctangent in radians.

**Example:**

```lua
atan(1)  -- 0.785...
```

**`bitwise_and(a, b)`**  
Bitwise AND of the integer parts of a and b.

**Example:**

```lua
bitwise_and(6, 3)  -- 2
```

**`bitwise_or(a, b)`**  
Bitwise OR of the integer parts of a and b.

**Example:**

```lua
bitwise_or(4, 1)  -- 5
```

**`bitwise_xor(a, b)`**  
Bitwise XOR of the integer parts of a and b.

**Example:**

```lua
bitwise_xor(6, 3)  -- 5
```

**`ceiling(x)`**  
Smallest integer >= x.

**Example:**

```lua
ceiling(2.1)  -- 3
```

**`cos(x)`**  
Cosine of x (radians).

**Example:**

```lua
cos(0)  -- 1
```

**`dec_part(x)`**  
Fractional part of x.

**Example:**

```lua
dec_part(3.7)  -- 0.7
```

**`deg2rad(x)`**  
Converts degrees to radians.

**Example:**

```lua
deg2rad(180)  -- 3.141...
```

**`exp(x)`**  
e raised to x.

**Example:**

```lua
exp(1)  -- 2.718...
```

**`extractstringd(s)`**  
Digits of s as a number.

**Example:**

```lua
extractstringd("ab12cd")  -- 12
```

**`floor(x)`**  
Largest integer <= x.

**Example:**

```lua
floor(2.7)  -- 2
```

**`int_part(x)`**  
Integer part of x (truncated).

**Example:**

```lua
int_part(3.7)  -- 3
```

**`log(x)`**  
Natural logarithm of x.

**Example:**

```lua
log(1)  -- 0
```

**`log10(x)`**  
Base-10 logarithm of x.

**Example:**

```lua
log10(100)  -- 2
```

**`mask_number(x, mask)`**  
Formats x using a mask (#, 0, comma grouping, decimal).

**Example:**

```lua
mask_number(1234.5, "#,##0.00")  -- "1,234.50"
```

**`nth_root(x, n)`**  
The n-th root of x.

**Example:**

```lua
nth_root(27, 3)  -- 3
```

**`power(x, y)`**  
x raised to the power y.

**Example:**

```lua
power(2, 3)  -- 8
```

**`rad2deg(x)`**  
Converts radians to degrees.

**Example:**

```lua
rad2deg(math.pi)  -- 180
```

**`random() / random(max) / random(min, max)`**  
Random float in [0,1), integer in [1,max], or in [min,max).

**Example:**

```lua
random(1, 6)  -- die roll
```

**`round(x[, decimals])`**  
Rounds x to decimals (default 0) with half-away-from-zero.

**Example:**

```lua
round(2.567, 2)  -- 2.57
```

**`sin(x)`**  
Sine of x (radians).

**Example:**

```lua
sin(0)  -- 0
```

**`sqrt(x)`**  
Square root of x.

**Example:**

```lua
sqrt(16)  -- 4
```

**`sum(table)`**  
Sum of the numeric values in a table.

**Example:**

```lua
sum({1, 2, 3})  -- 6
```

**`tan(x)`**  
Tangent of x (radians).

**Example:**

```lua
tan(0)  -- 0
```

**`val(x)`**  
Numeric value of x; 0 when not numeric.

**Example:**

```lua
val("42")  -- 42
```

### Conditional

**`iif(cond, a, b)`**  
Inline if; returns a when cond is truthy, else b.

**Example:**

```lua
iif(5 > 2, "yes", "no")  -- "yes"
```

**`lookup(key, k1, v1, ...)`**  
Returns the value whose key equals key; "" when absent.

**Example:**

```lua
lookup("b", "a", 1, "b", 2)  -- 2
```

**`yesno(cond, a, b)`**  
Returns a when cond is Kalipso-truthy, else b.

**Example:**

```lua
yesno(1, "y", "n")  -- "y"
```

### Datetime

**`add_days(date, n)`**  
Date n days later as "YYYY-MM-DD".

**Example:**

```lua
add_days("2026-09-07", 3)  -- "2026-09-10"
```

**`date_diff(d2, d1)`**  
Whole days between two date strings (d2 minus d1).

**Example:**

```lua
date_diff("2026-09-10", "2026-09-07")  -- 3
```

**`date_to_string(date[, format])`**  
Formats a date using %Y %m %d etc.; default "YYYY-MM-DD".

**Example:**

```lua
date_to_string("2026-09-07", "%d/%m/%Y")  -- "07/09/2026"
```

**`datetime_add(dt, days[, hours[, minutes[, seconds]]])`**  
Adds a duration to a datetime string.

**Example:**

```lua
datetime_add("2026-09-07 10:00", 1, 2)  -- "2026-09-08 12:00"
```

**`datetime_diff(dt2, dt1)`**  
Seconds between two datetime strings.

**Example:**

```lua
datetime_diff("2026-09-07 11:00", "2026-09-07 10:30")  -- 1800
```

**`datetime_sub(dt, days[, hours[, minutes[, seconds]]])`**  
Subtracts a duration from a datetime string.

**Example:**

```lua
datetime_sub("2026-09-07 10:00", 1, 2)  -- "2026-09-06 08:00"
```

**`day(date)`**  
Day of month of a date string.

**Example:**

```lua
day("2026-09-07")  -- 7
```

**`hour(time)`**  
Hour of a date/time string.

**Example:**

```lua
hour("12:34:56")  -- 12
```

**`julian(date)`**  
Julian day number of a date.

**Example:**

```lua
julian("2026-09-07")
```

**`local_to_utc(dt)`**  
Converts a local datetime string to UTC.

**Example:**

```lua
local_to_utc("2026-09-07 14:00:00")
```

**`minute(time)`**  
Minute of a date/time string.

**Example:**

```lua
minute("12:34:56")  -- 34
```

**`month(date)`**  
Month (1-12) of a date string.

**Example:**

```lua
month("2026-09-07")  -- 9
```

**`second(time)`**  
Second of a date/time string.

**Example:**

```lua
second("12:34:56")  -- 56
```

**`subtract_days(date, n)`**  
Date n days earlier as "YYYY-MM-DD".

**Example:**

```lua
subtract_days("2026-09-07", 3)  -- "2026-09-04"
```

**`sys_date()`**  
Today's date as "YYYY-MM-DD".

**Example:**

```lua
sys_date()  -- "2026-09-07"
```

**`sys_time()`**  
Current time as "HH:MM:SS".

**Example:**

```lua
sys_time()  -- "14:30:00"
```

**`tick_count()`**  
Unix milliseconds since the epoch.

**Example:**

```lua
tick_count()
```

**`time_to_string(time[, format])`**  
Formats a time using %H %M %S etc.; default "HH:MM:SS".

**Example:**

```lua
time_to_string("14:05:00", "%H:%M")  -- "14:05"
```

**`utc_to_local(dt)`**  
Converts a UTC datetime string to local wall-clock.

**Example:**

```lua
utc_to_local("2026-09-07 12:00:00")
```

**`week_day(date)`**  
Day of week as 1-7 (Sunday=1).

**Example:**

```lua
week_day("2026-09-07")  -- 1
```

**`week_number(date)`**  
ISO week number of a date.

**Example:**

```lua
week_number("2026-09-07")  -- 37
```

**`year(date)`**  
Year of a date string.

**Example:**

```lua
year("2026-09-07")  -- 2026
```

### Conversion

**`boolstr(v)`**  
{"true","false"} for the Kalipso truthiness of v.

**Example:**

```lua
boolstr(1)  -- "true"
```

**`strtodate(s[, format])`**  
Parses s (optionally with %-tokens) and emits "YYYY-MM-DD".

**Example:**

```lua
strtodate("07/09/2026", "%d/%m/%Y")  -- "2026-09-07"
```

**`todate(s)`**  
Normalizes s to "YYYY-MM-DD".

**Example:**

```lua
todate("07/09/2026")  -- "2026-09-07"
```

**`tonum(x)`**  
Kalipso number of x; 0 when not numeric.

**Example:**

```lua
tonum("3.5")  -- 3.5
```

**`tostr(x)`**  
Kalipso string form of x.

**Example:**

```lua
tostr(42)  -- "42"
```

## Script Globals

- **`ARGS`** — Table seeded from `--arg K=V` flags (string keys).
- **`CTRL`** — Accessor: `CTRL(name)` returns a control handle for `k.ctrl.*` operations.
- **`ERRORCODE`** — Last Kalipso error code (number; nil until an error). Negative `K_ERROR_*` values, default `-1`.
- **`ERRORMSG`** — Text of the last error (string; "" until an error).
- **`main`** — Entry point function (required in run mode).

