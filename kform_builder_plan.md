# KALUA Form Builder - Evaluation & Implementation Plan

## Executive Summary

A visual form builder for KALUA that allows drag-and-drop form design with live preview, property editing, and export to Lua script. This is a **design-time tool** — no Lua execution, purely form structure editing.

---

## Current Form Structure Analysis

### Lua → JSON Mapping

```lua
-- Lua script
k.form.new("main", {title="Test Form", layout="vertical"})
k.ctrl.label("main", "lbl1", {text="Hello KALUA!"})
k.ctrl.textbox("main", "txt1", {label="Name", value="World"})
k.ctrl.button("main", "btn1", {label="Click Me", onclick=function() ... end})
```

```json
// Equivalent JSON representation
{
  "name": "main",
  "title": "Test Form",
  "layout": "vertical",
  "controls": [
    {
      "name": "lbl1",
      "type": "label",
      "text": "Hello KALUA!",
      "enabled": true,
      "visible": true
    },
    {
      "name": "txt1",
      "type": "textbox",
      "label": "Name",
      "value": "World",
      "enabled": true,
      "visible": true
    },
    {
      "name": "btn1",
      "type": "button",
      "label": "Click Me",
      "class": "kalua-button-primary",
      "enabled": true,
      "visible": true
      // onclick: not serializable to JSON - handled separately
    }
  ]
}
```

### Form Properties

| Property | Type | Default | Description |
|----------|------|---------|-------------|
| `name` | string | required | Form identifier |
| `title` | string | "" | Form title display |
| `layout` | "vertical" \| "grid" | "vertical" | Layout mode |

### Control Properties by Type

| Control | Required Props | Optional Props |
|---------|----------------|----------------|
| **label** | `name`, `type`, `text` | `enabled`, `visible` |
| **textbox** | `name`, `type`, `label` | `value`, `enabled`, `visible`, `multiline`, `rows`, `cols`, `datetime` |
| **button** | `name`, `type`, `label` | `class`, `enabled`, `visible`, `onclick` (handler ref) |
| **combo** | `name`, `type`, `label`, `items` | `enabled`, `visible` |
| **list** | `name`, `type`, `label`, `items` | `enabled`, `visible` |
| **table** | `name`, `type` | `label`, `columns`, `rows`, `enabled`, `visible` |
| **checkbox** | `name`, `type`, `label` | `value`, `enabled`, `visible`, `hidden_value` |
| **radio** | `name`, `type`, `label` | `value`, `enabled`, `visible`, `hidden_value` |

### Items Format (combo/list)
```json
{
  "items": {
    "key1": "Display 1",
    "key2": "Display 2"
  }
}
```

---

## Architecture Options

### Option A: VS Code Webview (Recommended)
- **Pros**: Integrated into existing extension, access to workspace, file system, LSP
- **Cons**: Webview API limitations, TypeScript only
- **Implementation**: Add "Open Form Builder" command → opens webview panel

### Option B: Standalone Web App (Served by KALUA)
- **Pros**: Full browser capabilities, can be used independently
- **Cons**: Separate deployment, needs auth/access control
- **Implementation**: New `KALUA builder` command serves builder UI

### Option C: Electron/Tauri Desktop App
- **Pros**: Native feel, file system access
- **Cons**: Additional maintenance, separate distribution
- **Not recommended** for Phase 1

### Recommendation: **Option A (VS Code Webview)**
- Leverages existing extension infrastructure
- Natural fit for "edit .lua file → open builder → save back"
- Can use VS Code's file watcher for live sync
- Access to LSP for validation

---

## JSON Schema Definition

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "title": "KALUA Form Definition",
  "type": "object",
  "required": ["name", "controls"],
  "properties": {
    "name": { "type": "string", "pattern": "^[a-zA-Z_][a-zA-Z0-9_]*$" },
    "title": { "type": "string" },
    "layout": { "type": "string", "enum": ["vertical", "grid"] },
    "controls": {
      "type": "array",
      "items": { "$ref": "#/definitions/control" }
    }
  },
  "definitions": {
    "control": {
      "type": "object",
      "required": ["name", "type"],
      "properties": {
        "name": { "type": "string", "pattern": "^[a-zA-Z_][a-zA-Z0-9_]*$" },
        "type": { "type": "string", "enum": ["label", "textbox", "button", "combo", "list", "table", "checkbox", "radio"] },
        "label": { "type": "string" },
        "text": { "type": "string" },
        "value": { "type": ["string", "number", "boolean"] },
        "enabled": { "type": "boolean", "default": true },
        "visible": { "type": "boolean", "default": true },
        "class": { "type": "string" },
        "items": { "type": "object", "additionalProperties": { "type": "string" } },
        "columns": { "type": "array", "items": { "type": "string" } },
        "rows": { "type": "array", "items": { "type": "object" } },
        "hidden_value": { "type": "string" },
        "multiline": { "type": "boolean" },
        "rows": { "type": "number" },
        "cols": { "type": "number" },
        "datetime": { "type": "object" }
      }
    }
  }
}
```

---

## Builder UI Layout

```
+-----------------------------------------------------------------+
| Toolbar: [Save] [Export Lua] [Import Lua] [Undo] [Redo] [Zoom]  |
+----------+--------------------------------------+----------------+
|          |                                      |                |
| Controls |         Preview Canvas             | Property Editor|
| Palette  |                                      |                |
|          |  +--------------------------------+  | +------------+ |
|  +------+ | |  Form Title                  |  | | Control:   | |
|  |label | | +--------------------------------+  | | | btn1      | |
|  +------+ | |  [lbl1] Hello KALUA!         |  | +------------+ |
|  |textbox| | |  [txt1] Name: [World______] |  | | Type:      | |
|  +------+ | |  [btn1] [Click Me]           |  | | button     | |
|  |button | | |                              |  | | Label:     | |
|  +------+ | |  Drag controls from palette  |  | | Click Me   | |
|  |combo  | | |  to canvas. Click to select. |  | | Class:     | |
|  +------+ | +--------------------------------+  | | primary    | |
|  |list   | |                                      | | Enabled:  | |
|  +------+ |                                      | | [x]        | |
|  |table  | |                                      | | Visible:  | |
|  +------+ |                                      | | [x]        | |
|  |checkbox| |                                      | | OnClick:  | |
|  +------+ |                                      | | [  fx  ]   | |
|  |radio  | |                                      | +------------+ |
|  +------+ |                                      |                |
|  |image  | |                                      |                |
|  +------+ |                                      |                |
|          |                                      |                |
+----------+--------------------------------------+----------------+
```

### Components

1. **Controls Palette** (left) - Draggable control types
2. **Preview Canvas** (center) - Live form rendering using actual KALUA CSS
3. **Property Editor** (right) - Dynamic form based on selected control type

---

## Preview Rendering Strategy

### Approach: Reuse KALUA Rendering Engine

**Option 1: Embed KALUA CSS + Simulated Rendering**
- Include `kalua.css` in webview
- JavaScript renders controls using same HTML structure as Go `renderControl()`
- Fast, no Go dependency in builder

**Option 2: Call KALUA Binary for Preview**
- Send JSON to `KALUA check` or custom endpoint
- Get back HTML
- Slower, but 100% accurate

**Recommendation: Option 1** with periodic sync validation via Option 2

### Preview HTML Generation (TypeScript)
```typescript
function renderControl(control: Control): string {
  switch (control.type) {
    case 'label':
      return '<label class="kalua-label" id="c:' + formName + ':' + control.name + '">' + escapeHtml(control.text) + '</label>';
    case 'textbox':
      return '<div class="kalua-control">\n' +
        '  <label class="kalua-label" for="c:' + formName + ':' + control.name + '">' + escapeHtml(control.label) + '</label>\n' +
        '  <input type="text" class="kalua-input" id="c:' + formName + ':' + control.name + '" value="' + escapeHtml(control.value) + '"' + (control.enabled ? '' : ' disabled') + '>\n' +
        '</div>';
    // ... etc
  }
}
```

---

## Lua Export Generation

### Strategy: Template-based Code Generation

```typescript
function exportToLua(form: FormDefinition): string {
  const lines: string[] = [];
  
  lines.push('function main()');
  lines.push('  k.form.new("' + form.name + '", {title="' + escapeLua(form.title) + '", layout="' + form.layout + '"})');
  
  for (const ctrl of form.controls) {
    const opts = buildControlOptions(ctrl);
    lines.push('  k.ctrl.' + ctrl.type + '("' + form.name + '", "' + ctrl.name + '", ' + opts + ')');
  }
  
  lines.push('  k.form.show("' + form.name + '")');
  lines.push('end');
  
  return lines.join('\n');
}

function buildControlOptions(ctrl: Control): string {
  const opts: string[] = [];
  
  if (ctrl.type === 'label') {
    if (ctrl.text) opts.push('text="' + escapeLua(ctrl.text) + '"');
  } else {
    if (ctrl.label) opts.push('label="' + escapeLua(ctrl.label) + '"');
  }
  
  if (ctrl.value !== undefined) opts.push('value="' + escapeLua(String(ctrl.value)) + '"');
  if (ctrl.enabled === false) opts.push('enabled=false');
  if (ctrl.visible === false) opts.push('visible=false');
  if (ctrl.class) opts.push('class="' + escapeLua(ctrl.class) + '"');
  if (ctrl.items) opts.push('items=' + jsonToLuaTable(ctrl.items));
  if (ctrl.columns) opts.push('columns=' + jsonToLuaArray(ctrl.columns));
  // ... etc
  
  return '{' + opts.join(', ') + '}';
}
```

### Event Handlers (onclick, etc.)
- **Not exported** — builder only handles form structure
- User adds logic manually in Lua after export
- Builder can insert placeholder comments:
  ```lua
  k.ctrl.button("main", "btn1", {label="Click Me"})
  -- TODO: Add onclick handler: k.form.on("main", "btn1", "click", function() ... end)
  ```

---

## Implementation Plan

### Phase 1: Foundation (3 days)
- [ ] Create JSON schema for form definition
- [ ] Build TypeScript types matching schema
- [ ] Set up VS Code webview infrastructure in extension
- [ ] Add "KALUA: Open Form Builder" command

### Phase 2: Preview Engine (3 days)
- [ ] Port `renderControl` logic to TypeScript
- [ ] Include `kalua.css` in webview
- [ ] Implement live preview canvas
- [ ] Handle form/control selection highlighting

### Phase 3: Controls Palette (2 days)
- [ ] Draggable control list
- [ ] Drag-and-drop to canvas
- [ ] Insert at position (before/after/into)
- [ ] Visual drop zones

### Phase 4: Property Editor (3 days)
- [ ] Dynamic form generation per control type
- [ ] Real-time preview updates
- [ ] Validation (required fields, unique names)
- [ ] Special editors: items (key-value grid), columns, datetime config

### Phase 5: Layout & Ordering (2 days)
- [ ] Reorder controls (drag handles in preview)
- [ ] Delete controls
- [ ] Form-level properties (title, layout)
- [ ] Copy/paste/duplicate controls

### Phase 6: Import/Export (2 days)
- [ ] Export to Lua script (as described above)
- [ ] Import from existing Lua file (parse `k.form.new` + `k.ctrl.*` calls)
- [ ] Save/load `.kalua-form.json` files

### Phase 7: Polish & Integration (2 days)
- [ ] Undo/redo stack
- [ ] Keyboard shortcuts
- [ ] Responsive preview (mobile/desktop toggle)
- [ ] Error handling & validation feedback
- [ ] Documentation

---

## Technical Details

### Webview Communication

```typescript
// Extension side (extension.ts)
const panel = vscode.window.createWebviewPanel(
  'kaluaFormBuilder',
  'KALUA Form Builder',
  vscode.ViewColumn.One,
  { enableScripts: true, retainContextWhenHidden: true }
);

panel.webview.html = getWebviewContent();

// Message handling
panel.webview.onDidReceiveMessage(msg => {
  switch (msg.type) {
    case 'exportLua':
      // Write to .lua file
      break;
    case 'saveJson':
      // Write to .kalua-form.json
      break;
    case 'loadFile':
      // Read .lua or .json file, send back
      break;
  }
});
```

### File Association

- `.kalua-form.json` — Builder native format
- Double-click → opens in builder
- Right-click `.lua` → "Open in Form Builder" (parses and loads)

---

## Open Questions & Decisions Needed

| Question | Options | Recommendation |
|----------|---------|----------------|
| **Builder hosting** | VS Code webview vs standalone web app | VS Code webview (Option A) |
| **Layout system** | Vertical only vs Grid vs Absolute | Start vertical only; grid later |
| **Event handlers** | Include in JSON? | No — design-time only, export as TODO comments |
| **Table/Looper support** | Include complex controls? | Phase 2 — start with basic controls |
| **Multi-form support** | Single form vs multiple | Single form per file (per spec) |
| **Live sync** | Auto-save to Lua on change? | Manual save/export; auto-save JSON |
| **CSS framework** | Plain CSS vs Tailwind vs other | Plain CSS (match KALUA style) |

---

## Dependencies

### New Dependencies (Webview)
- No heavy frameworks — vanilla TypeScript + CSS
- Optional: `sortablejs` for drag-and-drop reordering (~20KB)
- Optional: `uuid` for control IDs

### Existing Assets Reused
- `kalua.css` — embedded in webview
- Control rendering logic — ported to TypeScript

---

## Estimated Timeline: 17 days

| Phase | Days | Deliverable |
|-------|------|-------------|
| 1: Foundation | 3 | Webview + schema + types |
| 2: Preview Engine | 3 | Live rendering matching KALUA |
| 3: Controls Palette | 2 | Drag-drop from palette |
| 4: Property Editor | 3 | Dynamic per-control-type editor |
| 5: Layout & Ordering | 2 | Reorder, delete, form props |
| 6: Import/Export | 2 | Lua <-> JSON round-trip |
| 7: Polish | 2 | UX, validation, docs |
| **Total** | **17** | **MVP Form Builder** |

---

## Future Enhancements (Post-MVP)

1. **Grid Layout** — CSS Grid-based positioning
2. **Looper Support** — Template editor for looper controls
3. **Tabulator Table Config** — Visual column editor
4. **Theme Preview** — Light/dark mode toggle
5. **Responsive Preview** — Device toolbar (mobile/tablet/desktop)
6. **Collaboration** — Real-time co-editing via VS Code Live Share
7. **Code Generation** — Full app skeleton with handlers

---

## Risk Assessment

| Risk | Impact | Mitigation |
|------|--------|------------|
| Preview mismatch vs runtime | High | Periodic validation via `KALUA check` |
| Complex control properties (table, looper) | Medium | Defer to Phase 2 |
| Lua parsing for import | Medium | Use regex + simple AST for known patterns |
| Webview performance with many controls | Low | Virtualize palette, memoize preview |

---

## Next Steps

1. **Confirm architecture**: VS Code webview vs standalone?
2. **Prioritize controls**: All 8 basic controls + image, or subset first?
3. **Layout system**: Vertical-only MVP sufficient?
4. **Event handler strategy**: Placeholder comments acceptable?
5. **File format**: `.kalua-form.json` as primary, Lua as export-only?

Please confirm decisions on open questions, and I'll create detailed technical specs for Phase 1.
