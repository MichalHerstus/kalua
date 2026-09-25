/* KALUA Form Builder client.
 * The Go server is the source of truth for the render; this script keeps the
 * JSON document model and pushes it to /api/preview for the live canvas.
 */
const $ = (s, el = document) => el.querySelector(s);
const $$ = (s, el = document) => Array.from(el.querySelectorAll(s));

const PALETTE = [
  ['label', 'Label'], ['textbox', 'Text Box'], ['button', 'Button'],
  ['combo', 'Combo Box'], ['list', 'List'], ['table', 'Table'],
  ['checkbox', 'Check Box'], ['radio', 'Radio'], ['looper', 'Looper'],
  ['chart', 'Chart'], ['image', 'Image'],
];

/* Option schemas: type = string|number|bool|select|json|items|dt|cell|align */
const TYPE_OPTS = {
  label:   { text: { t: 'string' }, multiline: { t: 'bool' } },
  textbox: { multiline: { t: 'bool' }, rows: { t: 'number' }, cols: { t: 'number' },
             datetime: { t: 'dt' } },
  button:  {},
  combo:   { items: { t: 'items' }, size: { t: 'number' } },
  list:    { items: { t: 'items' }, size: { t: 'number' } },
  table:   { columns: { t: 'json' }, data: { t: 'json' }, db: { t: 'string' },
             query: { t: 'string' }, page_size: { t: 'number' }, count_query: { t: 'string' },
             where: { t: 'string' }, order_by: { t: 'string' },
             tabulator: { t: 'bool' }, tabulatorOptions: { t: 'json' } },
  looper:  { db: { t: 'string' }, query: { t: 'string' }, links: { t: 'json' },
             page_size: { t: 'number' }, count_query: { t: 'string' },
             where: { t: 'string' }, order_by: { t: 'string' }, row: { t: 'json' } },
  chart:   { type: { t: 'select', opts: ['line', 'bar', 'hbar', 'pie', 'doughnut', 'scatter', 'radar', 'area'] },
             labels: { t: 'json' }, datasets: { t: 'json' }, options: { t: 'json' },
             width: { t: 'string' }, height: { t: 'string' }, responsive: { t: 'bool' },
             maintainAspectRatio: { t: 'bool' }, legend: { t: 'bool' },
             legendPosition: { t: 'select', opts: ['top', 'bottom', 'left', 'right'] },
             animation: { t: 'bool' }, stacked: { t: 'bool' } },
  checkbox: { hidden_value: { t: 'string' }, checked: { t: 'bool' } },
  radio:   { hidden_value: { t: 'string' }, selected: { t: 'bool' } },
  image:   { src: { t: 'string' }, alt: { t: 'string' }, width: { t: 'string' },
             height: { t: 'string' },
             fit: { t: 'select', opts: ['contain', 'cover', 'fill', 'scale-down', 'none'] },
             clickable: { t: 'bool' } },
};
const COMMON_OPTS = {
  label: { t: 'string' }, value: { t: 'string' }, enabled: { t: 'bool' },
  visible: { t: 'bool' }, class: { t: 'string' }, hidden_value: { t: 'string' },
  cell: { t: 'cell' }, align: { t: 'align' },
};
const TYPE_NAMES = Object.fromEntries(PALETTE);
const LABELS = {
  label: 'Label', text: 'Text', value: 'Value', type: 'Type', items: 'Items',
  size: 'Size', multiline: 'Multiline', rows: 'Rows', cols: 'Cols',
  datetime: 'Datetime', columns: 'Columns', data: 'Data', db: 'DB handle',
  query: 'Query', links: 'Links', page_size: 'Page size', count_query: 'Count query',
  where: 'Where', order_by: 'Order by', tabulator: 'Tabulator', tabulatorOptions: 'Tabulator options',
  labels: 'Labels', datasets: 'Datasets', options: 'Options', width: 'Width', height: 'Height',
  responsive: 'Responsive', maintainAspectRatio: 'Aspect ratio', legend: 'Legend',
  legendPosition: 'Legend position', animation: 'Animation', stacked: 'Stacked',
  hidden_value: 'Hidden value', src: 'Src', alt: 'Alt', fit: 'Fit', clickable: 'Clickable',
  enabled: 'Enabled', visible: 'Visible', class: 'CSS class', cell: 'Grid cell',
  align: 'Align', checked: 'Checked', selected: 'Selected',
};

const state = {
  path: null, format: 'lua', doc: null, selected: -1, dirty: false,
  previewTimer: null, jsonErrors: {}, validateIssues: null,
  undoStack: [], redoStack: [], lastSnapAt: 0,
  view: 'preview',
};

let currentCell = '';

/* ---------- HTTP ---------- */
async function api(method, url, body) {
  const opt = { method, headers: {} };
  if (body !== undefined) {
    opt.headers['Content-Type'] = 'application/json';
    opt.body = JSON.stringify(body);
  }
  const res = await fetch(url, opt);
  const ct = res.headers.get('content-type') || '';
  const data = ct.includes('json') ? await res.json() : await res.text();
  if (!res.ok) throw new Error(data && data.error ? data.error : 'HTTP ' + res.status);
  return data;
}

/* ---------- model helpers ---------- */
function setOpt(ctrl, key, value) {
  if (value === undefined || value === null || value === '') delete ctrl.opts[key];
  else ctrl.opts[key] = value;
  state.dirty = true;
}

function form() { return activeFormOf(state.doc); }
function ctrl() { return form().controls[state.selected]; }

/* activeFormOf returns the UI-selected form (doc.activeForm), falling back to
 * the first. Mirrors the server's activeForm() helper. */
function activeFormOf(doc) {
  const fs = doc && doc.forms;
  if (!fs || !fs.length) return { name: 'main', layout: 'vertical', align: 'left', controls: [] };
  const i = fs.findIndex(f => f.name === doc.activeForm);
  return fs[i >= 0 ? i : 0];
}

/* replaceDoc swaps the whole document (after Code-Apply / AI-import), keeping
 * the previously active form selected when it still exists. */
function replaceDoc(doc) {
  const prev = form().name;
  state.doc = normalizeDoc(doc);
  if (!state.doc.forms.some(f => f.name === prev)) state.doc.activeForm = state.doc.forms[0].name;
  return state.doc;
}

function normalizeDoc(doc) {
  /* v2 single-form documents ({form}) migrate to v3 ({forms:[...]}). */
  if (!doc.forms) {
    doc.forms = doc.form ? [doc.form] : [];
    delete doc.form;
  }
  if (!Array.isArray(doc.forms)) doc.forms = [];
  if (!doc.forms.length) doc.forms = [{ name: 'main', layout: 'vertical', align: 'left', controls: [] }];
  for (const f of doc.forms) {
    if (!f.name) f.name = 'main';
    if (!f.layout) f.layout = 'vertical';
    if (!f.align) f.align = 'left';
    if (!f.controls) f.controls = [];
    f.cells = normalizeCells(f.cells);
    if (!f.handlers) f.handlers = {};
    if (!f.notes) f.notes = [];
    for (const c of f.controls) {
      if (!c.opts) c.opts = {};
      /* The runtime renders label-control text from "text"; the code editor and
       * some imports emit "label". Canonicalize so the doc always carries "text"
       * for label controls (matching the GUI field and the export path). */
      if (c.type === 'label' && c.opts.label !== undefined && c.opts.text === undefined) {
        c.opts.text = c.opts.label;
      }
      if (c.type === 'label') delete c.opts.label;
    }
  }
  if (!doc.activeForm || !doc.forms.some(f => f.name === doc.activeForm)) {
    doc.activeForm = doc.forms[0].name;
  }
  if (!doc.notes) doc.notes = [];
  return doc;
}

/* Cells are an ordered array of {id,width,bg,border,align} (v2). Defensively
 * accept the v1 object form too and normalize to a sorted array. */
function normalizeCells(cells) {
  if (cells === undefined || cells === null) return [];
  if (Array.isArray(cells)) return cells.filter(c => c && c.id).map(c => ({
    id: c.id, width: c.width, bg: c.bg, align: c.align,
    border: c.border && (c.border.width || c.border.color) ? c.border : undefined,
  }));
  const ids = Object.keys(cells).sort();
  return ids.map(id => {
    const c = cells[id] || {};
    return { id, width: c.width, bg: c.bg, align: c.align,
      border: c.border && (c.border.width || c.border.color) ? c.border : undefined };
  });
}

/* ---------- API endpoints (document-driven) ---------- */
async function loadInitial() {
  const data = await api('GET', '/api/form');
  state.path = data.path;
  state.format = data.format;
  state.doc = normalizeDoc(data.doc);
  resetUndo();
  render();
  setStatus(data.message || (state.format === 'lua' ? 'Imported from Lua source' : 'Loaded JSON document'));
}

async function save() {
  try {
    const r = await api('PUT', '/api/form', { doc: state.doc });
    state.dirty = false;
    setStatus(r && r.message ? r.message : 'Form successfully saved', 'success');
    $('#btn-save').textContent = state.format === 'lua' ? 'Save' : 'Save';
  } catch (e) {
    setStatus('Save failed: ' + e.message, 'error');
  }
}

async function exportLua() {
  try {
    const r = await api('POST', '/api/export', { doc: state.doc });
    showModal('Generated Lua (' + r.lua.split('\n').length + ' lines)', r.lua);
    return r.lua;
  } catch (e) { setStatus('Export failed: ' + e.message, 'error'); return null; }
}

/* Assembled Lua: for .lua workspaces the server splices the edited document
 * into the original source (non-form code preserved); for JSON docs it
 * regenerates the whole file from all forms. This is what Save writes, so
 * Validate/Fix/AI-context operate on the real output. */
async function assembledLua() {
  try {
    const r = await api('POST', '/api/export', { doc: state.doc });
    return r.lua;
  } catch (e) { return null; }
}

async function validate() {
  const lua = await assembledLua();
  if (lua === null) { setStatus('Validate failed: export failed', 'error'); return; }
  try {
    const r = await api('POST', '/api/validate', { lua });
    const lines = [];
    for (const e of (r.errors || [])) lines.push('ERROR  ' + e);
    for (const e of (r.issues || [])) lines.push('WARN   ' + e);
    if (!r.errors && r.ok === false) lines.push('ERROR  ' + r.error);
    if (!lines.length) lines.push('No issues found — ' + lua.split('\n').length + ' lines OK.');
    showModal('Validation', lines.join('\n'));
    /* concise statusbar summary; the modal above has the full detail */
    const eCount = (r.errors || []).length;
    const wCount = (r.issues || []).length;
    if (r.ok && eCount === 0 && wCount === 0) setStatus('Validated. No issues.', 'success');
    else setStatus('Validated: ' + eCount + ' error(s), ' + wCount + ' warning(s)', eCount > 0 ? 'error' : 'warning');
    state.validateIssues = { hasIssues: eCount > 0 || wCount > 0, eCount, wCount };
    refreshFixButton();
  } catch (e) {
    setStatus('Validate failed: ' + e.message, 'error');
    state.validateIssues = null;
    refreshFixButton();
  }
}

/* The sidebar Fix button repairs the open form with the AI after Validate has
 * found issues. Enabled only when the last validation reported problems AND
 * the LLM is reachable. */
function refreshFixButton() {
  const btn = $('#btn-fix');
  const issues = state.validateIssues;
  const ready = AI.reachable && !!issues && issues.hasIssues;
  btn.disabled = !ready;
  btn.title = ready
    ? 'Fix validation issues with AI'
    : (!AI.reachable
        ? 'AI unreachable — Fix needs the LLM'
        : (!issues || !issues.hasIssues ? 'Validate first — enabled when issues are found' : 'Fix validation issues with AI'));
}

/* Fix the open form via the AI: ask it to repair the source, import the fixed
 * script into the builder document and persist it immediately. */
async function fixForm() {
  const issues = state.validateIssues;
  if (!issues || !issues.hasIssues) { setStatus('Nothing to fix'); return; }
  if (!AI.reachable) { setStatus('Fix failed: AI unreachable', 'error'); return; }
  const lua = await assembledLua();
  if (lua === null) return;
  const btn = $('#btn-fix');
  const prev = btn.textContent;
  btn.disabled = true;
  btn.textContent = 'Fixing…';
  try {
    const r = await api('POST', '/api/ai/fix', { script: lua });
    const imported = await api('POST', '/api/import', { lua: r.script, mode: 'replace' });
    if (!imported || !imported.doc) throw new Error('fixed script contains no form (no k.form.new call)');
    await api('PUT', '/api/form', { doc: imported.doc });
    snapshot('ai-fix');
    replaceDoc(imported.doc);
    state.selected = -1;
    currentCell = '';
    state.dirty = false;
    state.validateIssues = null;
    render();
    if (state.view === 'code') refreshCode();
    setStatus(r.ok ? 'Form successfully fixed' : 'Form fixed, but still has issues', r.ok ? 'success' : 'warning');
  } catch (e) {
    setStatus('Fix failed: ' + e.message, 'error');
  } finally {
    btn.textContent = prev;
    refreshFixButton();
  }
}

function schedulePreview() {
  clearTimeout(state.previewTimer);
  state.previewTimer = setTimeout(preview, 150);
}

async function preview() {
  if (!state.doc) return;
  try {
    const r = await api('POST', '/api/preview', { doc: state.doc });
    const wrap = $('#preview');
    wrap.innerHTML = r.html;
    wrap.dataset.layout = (form().layout || 'vertical');
    renderPreviewTabulators(wrap);
    if (state.selected >= 0) flashSelected(form().controls[state.selected]?.name);
    flashCell();
  } catch (e) {
    $('#preview').innerHTML = '<div class="error">' + escapeHtml(e.message) + '</div>';
  }
}

/* The builder preview is server-rendered HTML — app.js (which instantiates
 * Tabulator at runtime) is not loaded here. Convert every data-k-tabulator-*
 * container into a plain <table> so configured tables are visible in the
 * canvas preview, mirroring app.js's renderFallbackTable. */
function renderPreviewTabulators(scope) {
  const roots = scope ? [scope] : $$('#preview');
  roots.forEach(w => {
    w.querySelectorAll('.kalua-tabulator-table:not([data-k-tabulator-ready])').forEach(el => {
      let cols = [];
      try { cols = JSON.parse(el.dataset.kTabulatorColumns || '[]'); } catch (e) {}
      let data = [];
      try { data = JSON.parse(el.dataset.kTabulatorData || '[]'); } catch (e) {}
      if (Array.isArray(data) && data.length && (!cols || !cols.length)) {
        cols = Object.keys(data[0] || {}).map(f => ({ field: f, title: f }));
      }
      const rows = Array.isArray(data) ? data : [];
      if (!cols.length && !rows.length) {
        el.innerHTML = '<div class="kalua-tabulator-empty">No data</div>';
        el.setAttribute('data-k-tabulator-ready', 'true');
        return;
      }
      let html = '<table class="kalua-table"><thead><tr>';
      cols.forEach(c => {
        html += '<th>' + escapeHtml(c.title !== undefined ? c.title : c.field) + '</th>';
      });
      html += '</tr></thead><tbody>';
      rows.forEach(row => {
        html += '<tr>';
        cols.forEach(c => {
          let v = row && row[c.field];
          if (typeof v === 'boolean') v = v ? '\u2713' : '';
          html += '<td>' + escapeHtml(v === null || v === undefined ? '' : String(v)) + '</td>';
        });
        html += '</tr>';
      });
      html += '</tbody></table>';
      el.innerHTML = html;
      el.setAttribute('data-k-tabulator-ready', 'true');
    });
  });
}

function flashCell() {
  $$('#preview .kalua-cell').forEach(el => {
    el.classList.toggle('selected-cell', currentCell && el.getAttribute('data-k-cell') === currentCell);
  });
}

/* ---------- rendering ---------- */
function render() {
  renderForms();
  renderFormProps();
  renderPalette();
  renderControlList();
  renderEditor();
  renderCells();
  renderPath();
  schedulePreview();
}

function renderPath() {
  $('#path').textContent = state.path || 'untitled';
  $('#format').textContent = state.format === 'lua' ? 'LUA' : 'JSON';
  $('#form-title').textContent = form().title || form().name;
}

/* Form switcher: one line — "Select form:" + dropdown, then New/Delete. The
 * Delete button stays enabled so clicking it reports "cannot delete the last
 * form" in the status bar when only one form remains. */
function renderForms() {
  const fs = state.doc.forms;
  $('#form-select').innerHTML = fs.map(f =>
    `<option value="${escapeAttr(f.name)}" ${f.name === state.doc.activeForm ? 'selected' : ''}>${escapeHtml(f.name)}</option>`).join('');
}

function switchForm(name) {
  if (name === state.doc.activeForm) return;
  snapshot('switch-form');
  state.doc.activeForm = name;
  state.selected = -1;
  state.jsonErrors = {};
  currentCell = '';
  state.dirty = true;
  resetValidate();
  render();
  if (state.view === 'code') refreshCode();
  setStatus('Editing form "' + name + '".');
}

function addForm() {
  snapshot('add-form');
  const used = new Set(state.doc.forms.map(f => f.name));
  let n = 1;
  while (used.has('form_' + n)) n++;
  const name = 'form_' + n;
  state.doc.forms.push({
    name, layout: 'vertical', align: 'left', controls: [], cells: [],
    handlers: {}, handlerBodies: {},
  });
  state.doc.activeForm = name;
  state.selected = -1;
  currentCell = '';
  state.dirty = true;
  render();
  setStatus('Added form "' + name + '". Save to append it to the file.');
}

function deleteForm() {
  const cur = state.doc.activeForm;
  const fs = state.doc.forms;
  if (fs.length <= 1) { setStatus('Cannot delete the last form.', 'warning'); return; }
  confirmDialog(`Delete form “${cur}”?`, 'The form and ALL its controls and k.form.on handlers will be removed from the file on Save. This can be undone with Undo.', 'Delete', doDeleteForm);
}

function doDeleteForm() {
  if (state.doc.forms.length <= 1) return;
  snapshot('delete-form');
  const cur = state.doc.activeForm;
  state.doc.forms = state.doc.forms.filter(f => f.name !== cur);
  state.doc.activeForm = state.doc.forms[0].name;
  state.selected = -1;
  currentCell = '';
  state.dirty = true;
  render();
  setStatus('Form "' + cur + '" removed — its block will be deleted on Save.', 'warning');
}

function resetValidate() {
  state.validateIssues = null;
  refreshFixButton();
}

function renderFormProps() {
  const f = form();
  $('#f_name').value = f.name || '';
  $('#f_title').value = f.title || '';
  $('#f_layout').value = f.layout || 'vertical';
  $('#f_align').value = f.align || 'left';
  $('#f_gap').value = f.gap === undefined || f.gap === null ? '' : f.gap;
}

function renderPalette() {
  $('#palette-list').innerHTML = PALETTE.map(([t, n], i) =>
    `<li data-palette="${t}" class="${i === 0 ? 'sel' : ''}"><span class="dot"></span>${escapeHtml(n)}</li>`).join('');
}

function renderControlList() {
  const list = $('#control-list');
  const f = form();
  list.innerHTML = f.controls.map((c, i) =>
    `<li data-i="${i}" class="${i === state.selected ? 'sel' : ''}">
       <button class="mv up" data-act="up" title="Move up">▲</button>
       <button class="mv dn" data-act="down" title="Move down">▼</button>
       <span class="cname">${escapeHtml(c.name)}</span>
       <span class="badge">${TYPE_NAMES[c.type] || c.type}</span>
       ${f.handlers[c.name] ? `<span class="branch">⚡${f.handlers[c.name].length}</span>` : ''}
       <button class="del" data-act="del" title="Delete control">×</button>
     </li>`).join('');
}

function renderEditor() {
  const head = $('#ctrl-head');
  const editor = $('#ctrl-editor');
  if (state.selected < 0 || !form().controls[state.selected]) {
    head.textContent = 'Selected Control';
    $('#ctrl-type').textContent = '';
    const del = $('#ctrl-del');
    del.hidden = true;
    const edit = $('#ctrl-edit');
    edit.hidden = true;
    editor.innerHTML = '<span class="hint">Select a control in the preview or the list.</span>';
    return;
  }
  const c = ctrl();
  head.textContent = c.name;
  $('#ctrl-type').textContent = TYPE_NAMES[c.type] || c.type;
  const del = $('#ctrl-del');
  del.hidden = false;
  const edit = $('#ctrl-edit');
  edit.hidden = !(c.type === 'table' || c.type === 'looper');
  const fields = [];
  for (const [k, s] of Object.entries({ ...TYPE_OPTS[c.type], ...COMMON_OPTS })) {
    fields.push(fieldHTML(k, s, c));
  }
  editor.innerHTML = fields.join('');
  for (const [k, s] of Object.entries({ ...TYPE_OPTS[c.type], ...COMMON_OPTS })) {
    const input = $('[data-k="' + k + '"]', editor);
    if (input && input._wire) input._wire();
  }
}

function fieldHTML(k, s, c) {
  const v = c.opts[k];
  const label = LABELS[k] || k;
  switch (s.t) {
    case 'bool':
      return `<label class="row"><span>${label}</span><input type="checkbox" data-k="${k}" data-bool="" ${v ? 'checked' : ''}></label>`;
    case 'number':
      return `<label>${label}<input type="number" step="any" data-k="${k}" data-num="" value="${v === undefined ? '' : v}"></label>`;
    case 'select': {
      const dots = s.opts.map(o => `<option value="${o}" ${v === o ? 'selected' : ''}>${o}</option>`).join('');
      return `<label>${label}<select data-k="${k}" data-sel=""><option value="">—</option>${dots}</select></label>`;
    }
    case 'json':
      return `<label class="wide">${label}
        <textarea data-k="${k}" data-json="" rows="3" ${state.jsonErrors[k] ? 'class="baderr"' : ''}>${v === undefined ? '' : escapeHtml(JSON.stringify(v, null, 1))}</textarea>
        ${state.jsonErrors[k] ? '<span class="err">' + state.jsonErrors[k] + '</span>' : ''}</label>`;
    case 'items':
      return `<label class="wide">${label} <span class="hint">key=display, one per line</span>
        <textarea data-k="${k}" data-items="" rows="4">${itemsToLines(v)}</textarea></label>`;
    case 'dt':
      return datetimeFieldHTML(k, v);
    case 'cell': {
      const opts = cellsArr().map(c => `<option value="${escapeAttr(c.id)}" ${v === c.id ? 'selected' : ''}>${escapeAttr(c.id)}</option>`).join('');
      return `<label>${label}<select data-k="${k}" data-sel=""><option value="">—</option>${opts}</select></label>`;
    }
    case 'align': {
      const dots = ['left', 'center', 'right'].map(o =>
        `<option value="${o}" ${v === o ? 'selected' : ''}>${o}</option>`).join('');
      return `<label>${label}<select data-k="${k}" data-sel=""><option value="">default</option>${dots}</select></label>`;
    }
    case 'string':
    default:
      return `<label>${label}<input type="text" data-k="${k}" data-str="" value="${v === undefined ? '' : escapeAttr(String(v))}"></label>`;
  }
}

function datetimeFieldHTML(k, v) {
  const on = !!v && typeof v === 'object';
  const mode = on ? (v.mode || 'datetime') : 'datetime';
  const fmt = on ? (v.format || '') : '';
  return `<label class="row"><span>Datetime</span>
      <input type="checkbox" data-k="${k}" data-dton="" ${on ? 'checked' : ''}></label>
    ${on ? `
      <label>Mode<select data-k="${k}" data-dtm="">
        <option value="date" ${mode === 'date' ? 'selected' : ''}>date</option>
        <option value="time" ${mode === 'time' ? 'selected' : ''}>time</option>
        <option value="datetime" ${mode === 'datetime' ? 'selected' : ''}>datetime</option>
      </select></label>
      <label>Format<textarea data-k="${k}" data-dtf="" rows="2">${escapeHtml(fmt)}</textarea></label>` : ''}`;
}

function itemsToLines(v) {
  if (!Array.isArray(v)) return '';
  return v.map(it => `${it.key}=${it.display}`).join('\n');
}

function cellsArr() {
  return Array.isArray(form().cells) ? form().cells : [];
}
function currentCellDef() {
  return cellsArr().find(c => c.id === currentCell);
}

function renderCells() {
  const f = form();
  const sel = $('#f_cellsel');
  const cells = f.cells || [];
  sel.innerHTML = `<option value="">(none)</option>` +
    cells.map(c => `<option value="${escapeAttr(c.id)}" ${c.id === currentCell ? 'selected' : ''}>${escapeAttr(c.id)}</option>`).join('');
  const i = cells.findIndex(c => c.id === currentCell);
  const cell = i >= 0 ? cells[i] : null;
  $('#btn-cell-up').disabled = !(i > 0);
  $('#btn-cell-dn').disabled = !(i >= 0 && i < cells.length - 1);
  $('#btn-del-cell').disabled = cell === null;
  $('#c_id').value = cell ? cell.id : '';
  $('#c_width').value = cell && cell.width ? cell.width : '';
  $('#c_bg').value = cell && cell.bg ? cell.bg : '';
  $('#c_align').value = cell && cell.align ? cell.align : '';
  $('#c_bw').value = cell && cell.border && cell.border.width ? cell.border.width : '';
  $('#c_bc').value = cell && cell.border && cell.border.color ? cell.border.color : '';
}

function flashSelected(name) {
  $$('#preview [data-k-ctrl]').forEach(el => {
    el.classList.toggle('selected-in-canvas', el.getAttribute('data-k-ctrl') === name);
  });
}

/* ---------- actions ---------- */
function selectControl(i) {
  state.selected = i;
  state.jsonErrors = {};
  renderControlList();
  renderEditor();
  flashSelected(i >= 0 ? form().controls[i]?.name : null);
}

function addControl(type) {
  snapshot('add');
  const f = form();
  const n = f.controls.filter(c => c.type === type).length + 1;
  const name = type + '_' + n;
  const c = { name, type, opts: {} };
  switch (type) {
    case 'label': c.opts.text = 'Label text'; break;
    case 'button': c.opts.label = 'Button'; break;
    case 'textbox': c.opts.label = 'Text box'; break;
    case 'combo': case 'list': c.opts.items = [{ key: '1', display: 'Option 1' }]; break;
    case 'table': case 'looper': c.opts.query = 'SELECT * FROM t'; break;
    case 'chart': c.opts.type = 'line'; c.opts.labels = ['A', 'B', 'C']; c.opts.datasets = [{ label: 'Series', data: [1, 2, 3] }]; break;
    case 'image': c.opts.src = 'https://placehold.co/320x120'; break;
  }
  f.controls.push(c);
  selectControl(f.controls.length - 1);
  schedulePreview();
}

function deleteControl(i) {
  snapshot('delete');
  const f = form();
  f.controls.splice(i, 1);
  delete f.handlers[f.controls[i]?.name || ''];
  if (state.selected > i) state.selected--;
  else if (state.selected === i) state.selected = -1;
  render();
}

function moveControl(i, dir) {
  snapshot('move');
  const f = form();
  const j = i + dir;
  if (j < 0 || j >= f.controls.length) return;
  [f.controls[i], f.controls[j]] = [f.controls[j], f.controls[i]];
  state.selected = j;
  renderControlList();
  schedulePreview();
}

function addCell() {
  const f = form();
  if (!Array.isArray(f.cells)) f.cells = [];
  const used = new Set(f.cells.map(c => c.id));
  let n = 1;
  while (used.has('c' + n)) n++;
  const id = 'c' + n;
  f.cells.push({ id, width: 12 });
  currentCell = id;
  renderCells();
  schedulePreview();
}

function deleteCell(id) {
  const f = form();
  const cells = Array.isArray(f.cells) ? f.cells : [];
  const i = cells.findIndex(c => c.id === id);
  if (i < 0) return;
  cells.splice(i, 1);
  let moved = 0;
  for (const c of f.controls) {
    if (c.opts.cell === id) { c.opts.cell = 'main'; moved++; }
  }
  if (currentCell === id) currentCell = cells.length ? cells[0].id : '';
  if (moved) setStatus(`Deleted cell “${id}” — ${moved} control(s) reassigned to “main”.`);
  render();
}

function moveCell(i, dir) {
  const cells = cellsArr();
  const j = i + dir;
  if (j < 0 || j >= cells.length) return;
  snapshot('cell');
  [cells[i], cells[j]] = [cells[j], cells[i]];
  renderCells();
  schedulePreview();
}

function renameCell(newId) {
  const cells = cellsArr();
  const i = cells.findIndex(c => c.id === currentCell);
  if (i < 0) return;
  const id = String(newId || '').trim();
  if (!id) { renderCells(); return; }
  if (id !== cells[i].id && cells.some(c => c.id === id)) {
    setStatus('Cell id already exists.', 'warning');
    renderCells();
    return;
  }
  snapshot('cell');
  const old = cells[i].id;
  cells[i].id = id;
  for (const c of form().controls) if (c.opts.cell === old) c.opts.cell = id;
  currentCell = id;
  render();
}

/* ---------- event wiring ---------- */
function wirePalette() {
  const list = $('#palette-list');
  list.addEventListener('click', e => {
    const li = e.target.closest('li');
    if (!li) return;
    $$('#palette-list li').forEach(x => x.classList.remove('sel'));
    li.classList.add('sel');
  });
  $('#btn-add-control').addEventListener('click', () => {
    const sel = $('#palette-list li.sel');
    const type = sel ? sel.dataset.palette : 'label';
    addControl(type);
  });

  $('#palette-list').addEventListener('dblclick', e => {
    const li = e.target.closest('li');
    if (li) addControl(li.dataset.palette);
  });
}

function wireControlList() {
  $('#control-list').addEventListener('click', e => {
    const btn = e.target.closest('button');
    const li = e.target.closest('li');
    if (!li) return;
    const i = +li.dataset.i;
    if (btn) {
      const act = btn.dataset.act;
      if (act === 'del') deleteControl(i);
      else if (act === 'up') moveControl(i, -1);
      else if (act === 'down') moveControl(i, 1);
      return;
    }
    selectControl(i);
  });
}

function wireFormProps() {
  const bind = (id, fn) => $(id).addEventListener('change', e => {
    fn(e);
    renderPath();
    renderCells();
    schedulePreview();
  });
  bind('#f_name', e => { snapshot('form'); form().name = e.target.value; state.dirty = true; });
  bind('#f_layout', e => { snapshot('form'); form().layout = e.target.value; state.dirty = true; });
  bind('#f_align', e => { snapshot('form'); form().align = e.target.value; state.dirty = true; });
  bind('#f_title', e => {
    snapshot('form');
    const v = e.target.value;
    form().title = v;
    if (!v) delete form().title;
    state.dirty = true;
  });
  bind('#f_gap', e => {
    snapshot('form');
    const v = e.target.value;
    if (v === '') { delete form().gap; state.dirty = true; return; }
    form().gap = +v;
    state.dirty = true;
  });

  $('#f_cellsel').addEventListener('change', e => {
    currentCell = e.target.value;
    renderCells();
    flashCell();
  });
  $('#btn-add-cell').addEventListener('click', () => { snapshot('cell'); addCell(); });
  $('#btn-cell-up').addEventListener('click', () => {
    const i = cellsArr().findIndex(c => c.id === currentCell);
    if (i > 0) moveCell(i, -1);
  });
  $('#btn-cell-dn').addEventListener('click', () => {
    const i = cellsArr().findIndex(c => c.id === currentCell);
    if (i >= 0) moveCell(i, 1);
  });
  $('#btn-del-cell').addEventListener('click', () => {
    if (!currentCell) return;
    snapshot('cell');
    deleteCell(currentCell);
  });
  $('#c_id').addEventListener('change', e => {
    if (!currentCell) return;
    renameCell(e.target.value);
    flashCell();
  });
  const cellInput = (id, fn) => $(id).addEventListener('change', e => {
    if (!currentCell) { setStatus('Select a cell first'); return; }
    fn(e);
    schedulePreview();
  });
  cellInput('#c_width', e => {
    const v = +e.target.value;
    const cell = currentCellDef();
    if (!cell) return;
    if (v >= 1 && v <= 12) { snapshot('cell'); cell.width = v; }
    else if (v === 0) { snapshot('cell'); delete cell.width; }
  });
  cellInput('#c_bg', e => {
    const cell = currentCellDef();
    if (!cell) return;
    const v = e.target.value;
    snapshot('cell');
    if (v) cell.bg = v; else delete cell.bg;
  });
  cellInput('#c_align', e => {
    const cell = currentCellDef();
    if (!cell) return;
    const v = e.target.value;
    snapshot('cell');
    if (v) cell.align = v; else delete cell.align;
  });
  cellInput('#c_bw', e => {
    const cell = currentCellDef();
    if (!cell) return;
    const v = +e.target.value;
    snapshot('cell');
    if (v >= 1) {
      if (!cell.border) cell.border = {};
      cell.border.width = v;
    } else if (cell.border) {
      delete cell.border.width;
      if (!Object.keys(cell.border).length) delete cell.border;
    }
  });
  cellInput('#c_bc', e => {
    const cell = currentCellDef();
    if (!cell) return;
    const v = e.target.value;
    snapshot('cell');
    if (v) {
      if (!cell.border) cell.border = {};
      cell.border.color = v;
    } else if (cell.border) {
      delete cell.border.color;
      if (!Object.keys(cell.border).length) delete cell.border;
    }
  });
}

function deleteControlSelected() {
  if (state.selected < 0) return;
  deleteControl(state.selected);
}

function wireEditor() {
  $('#ctrl-editor').addEventListener('input', onEditorInput);
  $('#ctrl-editor').addEventListener('change', onEditorInput);
  $('#ctrl-del').addEventListener('click', deleteControlSelected);
}

function onEditorInput(e) {
  const t = e.target;
  const k = t.dataset.k;
  if (!k || state.selected < 0) return;
  const c = ctrl();
  if (t.dataset.bool !== undefined) {
    snapshot('edit');
    if (t.checked) setOpt(c, k, true); else delete c.opts[k];
  } else if (t.dataset.num !== undefined) {
    snapshot('edit');
    if (t.value === '') delete c.opts[k]; else setOpt(c, k, +t.value);
  } else if (t.dataset.str !== undefined) {
    snapshot('edit');
    setOpt(c, k, t.value);
  } else if (t.dataset.sel !== undefined) {
    snapshot('edit');
    if (t.value) setOpt(c, k, t.value); else delete c.opts[k];
  } else if (t.dataset.json !== undefined) {
    snapshot('edit');
    const raw = t.value.trim();
    if (!raw) { delete c.opts[k]; delete state.jsonErrors[k]; t.classList.remove('baderr'); }
    else {
      try { c.opts[k] = JSON.parse(raw); delete state.jsonErrors[k]; t.classList.remove('baderr'); }
      catch (err) { state.jsonErrors[k] = 'Invalid JSON: ' + err.message; t.classList.add('baderr'); }
    }
  } else if (t.dataset.items !== undefined) {
    snapshot('edit');
    c.opts.items = linesToItems(t.value);
  } else if (t.dataset.dton !== undefined) {
    snapshot('edit');
    if (t.checked) { c.opts[k] = { mode: 'datetime' }; renderEditor(); }
    else delete c.opts[k];
  } else if (t.dataset.dtm !== undefined) {
    snapshot('edit');
    const dst = c.opts[k] && typeof c.opts[k] === 'object' ? c.opts[k] : { mode: 'datetime' };
    dst.mode = t.value; c.opts[k] = dst;
  } else if (t.dataset.dtf !== undefined) {
    snapshot('edit');
    const dt = c.opts[k] && typeof c.opts[k] === 'object' ? c.opts[k] : { mode: 'datetime' };
    if (t.value) dt.format = t.value; else delete dt.format;
    c.opts[k] = dt;
  }
  schedulePreview();
}

function linesToItems(text) {
  const out = [];
  for (const raw of text.split('\n')) {
    const line = raw.trim();
    if (!line) continue;
    const eq = line.indexOf('=');
    const key = eq >= 0 ? line.slice(0, eq).trim() : line;
    const display = eq >= 0 ? line.slice(eq + 1).trim() : key;
    if (key) out.push({ key, display });
  }
  return out;
}

function wirePreview() {
  $('#preview').addEventListener('click', e => {
    const ctrlEl = e.target.closest('[data-k-ctrl]');
    if (ctrlEl) {
      const name = ctrlEl.getAttribute('data-k-ctrl');
      const i = form().controls.findIndex(c => c.name === name);
      if (i >= 0) selectControl(i);
      return;
    }
    const cellEl = e.target.closest('.kalua-cell');
    if (cellEl) {
      currentCell = cellEl.getAttribute('data-k-cell') || '';
      selectCell(currentCell);
    }
  });
}

/* ---------- canvas view switch: Preview | Code ---------- */
function wireCanvasSwitch() {
  $$('#canvas-switch button').forEach(btn => {
    btn.addEventListener('click', () => switchView(btn.dataset.view));
  });
  $('#btn-code-refresh').addEventListener('click', refreshCode);
  $('#btn-code-apply').addEventListener('click', applyCode);
  $('#btn-code-validate').addEventListener('click', validateCode);
  const ta = $('#code-editor');
  /* The overlay pre is the only visible text, so the highlight must follow
   * each keystroke immediately — a debounce made typing look laggy (text stayed
   * invisible until the timer fired). The tokenizer is linear; docs are small
   * enough that a synchronous re-highlight per input is fine. */
  ta.addEventListener('input', syncCodeHighlight);
  ta.addEventListener('scroll', syncCodeScroll);
  ta.addEventListener('keydown', e => {
    if (e.key !== 'Tab') return;
    e.preventDefault();
    const s = ta.selectionStart, x = ta.selectionEnd;
    ta.value = ta.value.slice(0, s) + '  ' + ta.value.slice(x);
    ta.selectionStart = ta.selectionEnd = s + 2;
    ta.dispatchEvent(new Event('input'));
  });
}

function switchView(view) {
  if (state.view === view) return;
  state.view = view;
  $$('#canvas-switch button').forEach(b => b.classList.toggle('sel', b.dataset.view === view));
  const codeMode = view === 'code';
  $('#preview').classList.toggle('hidden', codeMode);
  $('#code').classList.toggle('hidden', !codeMode);
  $('#canvas-hint').classList.toggle('hidden', codeMode);
  if (codeMode) refreshCode();
}

/* Load the current form's generated Lua into the editor. This resets any local
 * edits to what the builder document holds (use ↻ after changing the form). */
async function refreshCode() {
  if (!state.doc) return;
  try {
    const r = await api('POST', '/api/export', { doc: state.doc });
    $('#code-editor').value = r.lua;
    syncCodeHighlight();
  } catch (e) {
    $('#code-editor').value = '-- Failed to load the generated source: ' + e.message;
    syncCodeHighlight();
  }
}

/* Parse the edited Lua and replace the builder document with it. */
async function applyCode() {
  const ta = $('#code-editor');
  const lua = ta.value;
  if (lua.trim() === '') { setStatus('Nothing to apply', 'info'); return; }
  try {
    const imported = await api('POST', '/api/import', { lua, mode: 'replace' });
    if (!imported || !imported.doc) throw new Error('no k.form.new call found in the edited code');
    await api('PUT', '/api/form', { doc: imported.doc });
    snapshot('code-apply');
    replaceDoc(imported.doc);
    state.selected = -1;
    currentCell = '';
    state.dirty = false;
    state.validateIssues = null;
    refreshFixButton();
    render();
    refreshCode();
    setStatus('Code applied — parsed into the form', 'success');
  } catch (e) {
    setStatus('Code apply failed: ' + e.message, 'error');
  }
}

async function validateCode() {
  const lua = $('#code-editor').value;
  if (lua.trim() === '') { setStatus('Nothing to validate', 'info'); return; }
  try {
    const r = await api('POST', '/api/validate', { lua });
    const eCount = (r.errors || []).length;
    const wCount = (r.issues || []).length;
    if (r.ok && eCount === 0 && wCount === 0) setStatus('Code validates: no issues', 'success');
    else setStatus('Code: ' + eCount + ' error(s), ' + wCount + ' warning(s)', eCount > 0 ? 'error' : 'warning');
  } catch (e) {
    setStatus('Code validate failed: ' + e.message, 'error');
  }
}

/* Keep the syntax-highlighted <pre> behind the transparent editor in sync. */
function syncCodeScroll() {
  const pre = $('#code-highlight'), ta = $('#code-editor');
  pre.scrollTop = ta.scrollTop;
  pre.scrollLeft = ta.scrollLeft;
}

function syncCodeHighlight() {
  const ta = $('#code-editor');
  /* trailing newline keeps the overlay height equal to the editor's */
  $('#code-highlight').innerHTML = highlight(ta.value + '\n');
  syncCodeScroll();
}

const LUA_KEYWORDS = new Set([
  'and', 'break', 'do', 'else', 'elseif', 'end', 'false', 'for',
  'function', 'goto', 'if', 'in', 'local', 'nil', 'not', 'or',
  'repeat', 'return', 'then', 'true', 'until', 'while',
]);

/* Minimal Lua tokenizer: comments, strings (incl. long brackets), numbers,
 * keywords, and dotted call targets (k.form.new) get classed spans. */
function highlight(src) {
  const out = [];
  let i = 0;
  const n = src.length;
  const span = (cls, raw) => {
    if (raw) out.push('<span class="' + cls + '">' + escapeHtml(raw) + '</span>');
  };
  while (i < n) {
    const ch = src[i];
    /* -- line comment / --[[ long comment ]] */
    if (ch === '-' && src[i + 1] === '-') {
      if (src[i + 2] === '[') {
        let eq = i + 2;
        while (src[eq] === '=') eq++;
        if (src[eq] === '[') {
          const close = ']' + '='.repeat(eq - i - 2) + ']';
          const j = src.indexOf(close, eq + 1);
          const end = j === -1 ? n : j + close.length;
          span('lua-cm', src.slice(i, end));
          i = end;
          continue;
        }
      }
      let j = src.indexOf('\n', i + 2);
      j = j === -1 ? n : j;
      span('lua-cm', src.slice(i, j));
      i = j;
      continue;
    }
    /* string: '...' / "..." (backslash escapes) */
    if (ch === "'" || ch === '"') {
      let j = i + 1, esc = false;
      while (j < n) {
        const c = src[j];
        if (esc) { esc = false; j++; continue; }
        if (c === '\\') { esc = true; j++; continue; }
        if (c === ch) { j++; break; }
        j++;
      }
      span('lua-str', src.slice(i, j));
      i = j;
      continue;
    }
    /* long bracket string [[...]] */
    if (ch === '[') {
      let eq = i + 1;
      while (src[eq] === '=') eq++;
      if (src[eq] === '[') {
        const close = ']' + '='.repeat(eq - i - 1) + ']';
        const j = src.indexOf(close, eq + 1);
        const end = j === -1 ? n : j + close.length;
        span('lua-str', src.slice(i, end));
        i = end;
        continue;
      }
      /* bare '[' that is not a long-bracket opener (e.g. the ["key"] = value
       * map-key form): emit it verbatim so the loop always advances. Falling
       * through would make the plain-text scan break on '[' without consuming
       * it, leaving the tokenizer stuck on the same character forever. */
      out.push('[');
      i++;
      continue;
    }
    /* number */
    if (/[0-9]/.test(ch) || (ch === '.' && /[0-9]/.test(src[i + 1] || ''))) {
      let j = i;
      while (j < n) {
        const c = src[j];
        if (!/[0-9a-fA-FxX_.eE+-]/.test(c)) break;
        if ((c === '+' || c === '-') && !/[eE]/.test(src[j - 1] || '')) break;
        j++;
      }
      span('lua-num', src.slice(i, j));
      i = j;
      continue;
    }
    /* identifier / keyword / dotted call target */
    if (/[A-Za-z_]/.test(ch)) {
      let j = i;
      while (j < n && /[A-Za-z0-9_]/.test(src[j])) j++;
      let name = src.slice(i, j);
      while (src[j] === '.' && /[A-Za-z_]/.test(src[j + 1] || '')) {
        const k = j + 1;
        j = k;
        while (j < n && /[A-Za-z0-9_]/.test(src[j])) j++;
        name += src.slice(k - 1, j);
      }
      if (LUA_KEYWORDS.has(name)) {
        out.push('<span class="lua-kw">' + name + '</span>');
      } else {
        let k = j;
        while (k < n && /\s/.test(src[k])) k++;
        if (src[k] === '(') span('lua-fn', name);
        else out.push(escapeHtml(name));
      }
      i = j;
      continue;
    }
    /* plain text until the next token start */
    const start = i;
    while (i < n) {
      const c = src[i];
      if (c === '-' && src[i + 1] === '-') break;
      if (c === "'" || c === '"' || c === '[') break;
      if (/[0-9]/.test(c) || (c === '.' && /[0-9]/.test(src[i + 1] || ''))) break;
      if (/[A-Za-z_]/.test(c)) break;
      i++;
    }
    out.push(escapeHtml(src.slice(start, i)));
  }
  return out.join('');
}

function selectCell(id) {
  currentCell = id;
  renderCells();
  flashCell();
  state.selected = -1;
  renderControlList();
  renderEditor();
  flashSelected(null);
}

function wireTopbar() {
  $('#form-select').addEventListener('change', e => switchForm(e.target.value));
  $('#btn-new-form').addEventListener('click', addForm);
  $('#btn-del-form').addEventListener('click', deleteForm);
  $('#btn-new').addEventListener('click', async () => {
    resetUndo();
    state.doc = { version: 3, forms: [{ name: 'main', layout: 'vertical', align: 'left', controls: [], cells: [] }], activeForm: 'main', notes: [] };
    state.selected = -1;
    currentCell = '';
    state.dirty = true;
    resetValidate();
    render();
    setStatus('New empty form — Save appends a fresh main() to the file.');
  });
  $('#btn-save').addEventListener('click', save);
  $('#btn-export').addEventListener('click', exportLua);
  $('#btn-validate').addEventListener('click', validate);
  $('#btn-fix').addEventListener('click', fixForm);
  $('#btn-undo').addEventListener('click', undo);
  $('#btn-redo').addEventListener('click', redo);
  document.addEventListener('keydown', e => {
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 's') { e.preventDefault(); save(); return; }
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'z') {
      if (e.shiftKey) { e.preventDefault(); redo(); }
      else { e.preventDefault(); undo(); }
      return;
    }
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'y') { e.preventDefault(); redo(); return; }
    if ((e.key === 'Delete' || e.key === 'Backspace') &&
        !(e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA' || e.target.tagName === 'SELECT')) {
      deleteControlSelected();
    }
  });
  $('#modal-close').addEventListener('click', hideModal);
  $('#overlay').addEventListener('click', e => {
    if (e.target.id === 'overlay') hideModal();
  });
}

/* ---------- utilities ---------- */
function setStatus(msg, kind) {
  const el = $('#status');
  el.textContent = msg;
  el.className = 'status-' + (kind || 'info');
  const notes = state.doc && state.doc.notes && state.doc.notes.length
    ? state.doc.notes
    : (state.doc.forms ? (form().notes || []) : []);
  $('#notes').textContent = notes && notes.length ? '⚠ ' + notes.join(' · ') : '';
}

function hideModal() {
  $('#overlay').classList.add('hidden');
  $('#modal-confirm').classList.add('hidden');
}

function showModal(title, text) {
  $('#modal-title').textContent = title;
  $('#modal-body').textContent = text;
  $('#modal-confirm').classList.add('hidden');
  $('#overlay').classList.remove('hidden');
}

/* confirmDialog shows the shared modal with explicit yes/no buttons and runs
 * `onYes` only when the user confirms. Uses the app's own overlay instead of
 * window.confirm, which some browsers/embeddings block silently (making
 * buttons appear to "do nothing"). The yes/no buttons are wired once; each
 * call only replaces the pending action. */
let pendingConfirm = null;
let confirmWired = false;
function confirmDialog(title, message, yesLabel, onYes) {
  pendingConfirm = onYes;
  $('#modal-title').textContent = title;
  $('#modal-body').textContent = message;
  $('#modal-yes').textContent = yesLabel;
  $('#modal-confirm').classList.remove('hidden');
  $('#overlay').classList.remove('hidden');
  if (!confirmWired) {
    confirmWired = true;
    $('#modal-yes').addEventListener('click', () => {
      hideModal();
      const fn = pendingConfirm;
      pendingConfirm = null;
      if (fn) fn();
    });
    $('#modal-no').addEventListener('click', hideModal);
  }
}

function escapeHtml(s) {
  return String(s).replace(/[&<>"']/g, c => (
    { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
}
function escapeAttr(s) { return escapeHtml(s); }

/* ---------- undo / redo (one step) ---------- */
function snapDoc() {
  return JSON.parse(JSON.stringify(state.doc));
}

/* Snapshot the pre-mutation document. Consecutive snapshots of the same kind
 * within 600 ms coalesce (replace the top entry) so a typing burst in a field
 * is a single undo step. */
function snapshot(kind) {
  const now = Date.now();
  const candidate = snapDoc();
  const top = state.undoStack[state.undoStack.length - 1];
  if (top && top.kind === kind && now - state.lastSnapAt < 600) {
    top.doc = candidate;
    top.selected = state.selected;
    top.cell = currentCell;
  } else if (top && JSON.stringify(top.doc) === JSON.stringify(candidate)) {
    top.selected = state.selected; /* no-op edit (e.g. blur after typing) — don't push */
    state.redoStack = [];
    state.lastSnapAt = now;
    state.dirty = true;
    updateUndoButtons();
    return;
  } else {
    state.undoStack.push({ kind, doc: candidate, selected: state.selected, cell: currentCell });
    if (state.undoStack.length > 100) state.undoStack.shift();
  }
  state.redoStack = [];
  state.lastSnapAt = now;
  state.dirty = true;
  updateUndoButtons();
}

function undo() {
  if (!state.undoStack.length) return;
  state.redoStack.push({ doc: snapDoc(), selected: state.selected, cell: currentCell });
  restore(state.undoStack.pop());
}

function redo() {
  if (!state.redoStack.length) return;
  state.undoStack.push({ doc: snapDoc(), selected: state.selected, cell: currentCell });
  restore(state.redoStack.pop());
}

function restore(snap) {
  state.doc = snap.doc;
  state.selected = snap.selected;
  currentCell = snap.cell;
  state.jsonErrors = {};
  state.dirty = true;
  render();
  updateUndoButtons();
}

function updateUndoButtons() {
  $('#btn-undo').disabled = state.undoStack.length === 0;
  $('#btn-redo').disabled = state.redoStack.length === 0;
}

function resetUndo() {
  state.undoStack = [];
  state.redoStack = [];
  state.lastSnapAt = 0;
  updateUndoButtons();
}

/* ---------- theme ---------- */
function currentTheme() {
  return document.documentElement.getAttribute('data-theme') === 'dark' ? 'dark' : 'light';
}
function applyTheme(theme) {
  document.documentElement.setAttribute('data-theme', theme);
  try { localStorage.setItem('kalua-builder-theme', theme); } catch (e) {}
  /* Switch semantics: the label names the theme you'll switch TO. */
  const next = theme === 'dark' ? 'Light' : 'Dark';
  $('#theme-label').textContent = next;
  $('#btn-theme').title = 'Switch to ' + next.toLowerCase() + ' theme';
}
function wireTheme() {
  var theme = null;
  try { theme = localStorage.getItem('kalua-builder-theme'); } catch (e) {}
  if (theme !== 'light' && theme !== 'dark') {
    theme = (window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches)
      ? 'dark' : 'light';
  }
  applyTheme(theme);
  $('#btn-theme').addEventListener('click', () => {
    applyTheme(currentTheme() === 'dark' ? 'light' : 'dark');
  });
}

/* ---------- AI builder (Phases 3–4) ---------- */
const AI = {
  provider: null, model: null, reachable: false,
  generated: null,   // last generated script
  errors: [],
  logs: [],
  history: [],      // [{role:'user'|'assistant', content}] multi-turn context
};

/* Stop-button plumbing: aiStop() is installed while a generation runs and
 * cancels the SSE reader (the server halts writes when the client disconnects,
 * so the LLM goroutine drains harmlessly in the background). */
let aiStop = null;
let aiReader = null;
let aiStopped = false;

function aiAddMsg(cls, text) {
  const chat = $('#ai-chat');
  const el = document.createElement('div');
  el.className = 'ai-msg ' + cls;
  el.textContent = text;
  chat.appendChild(el);
  chat.scrollTop = chat.scrollHeight;
  return el;
}

async function aiStatus() {
  try {
    const r = await api('GET', '/api/ai/status');
    const dot = $('#ai-dot');
    dot.dataset.state = r.reachable ? 'ok' : 'error';
    let why = r.error ? r.error.trim() : '';
    if (why.length > 140) why = why.slice(0, 140) + '…';
    dot.title = r.reachable ? 'LLM reachable' : 'LLM unreachable' + (why ? '\n' + why : '');
    AI.provider = r.provider; AI.model = r.model; AI.reachable = r.reachable;
    $('#ai-provider').textContent = r.reachable
      ? r.baseUrl
      : r.baseUrl + ' (offline' + (why ? ': ' + why : '') + ')';
    $('#ai-model').textContent = r.model;
    $('#ai-model').title = why;
    refreshFixButton();
  } catch (e) {
    const dot = $('#ai-dot');
    dot.dataset.state = 'error';
    dot.title = 'LLM unreachable: ' + e.message;
    $('#ai-provider').textContent = 'status check failed';
    AI.reachable = false;
    refreshFixButton();
  }
}

function aiPushPrompt() {
  const v = $('#ai-prompt').value.trim();
  if (!v) return null;
  aiAddMsg('user', v);
  AI.history.push({ role: 'user', content: v });
  $('#ai-prompt').value = '';
  return v;
}

/* aiContext returns the current form source to attach as edit-existing
 * context, or empty when the checkbox is off or no source is available. */
async function aiContext() {
  if (!$('#ai-context').checked) return '';
  const lua = await assembledLua();
  return lua === null ? '' : lua;
}

async function aiGenerate() {
  const prompt = aiPushPrompt();
  if (!prompt) return;
  const ctx = await aiContext();
  const code = $('#ai-code');
  code.classList.remove('hidden');
  code.textContent = '';
  $('#ai-tab-code').disabled = false;
  $('#ai-tab-chat').className = 'sel';
  $('#ai-tab-code').className = '';
  $('#ai-generate').disabled = true;
  $('#ai-fix').disabled = true;
  $('#ai-apply').disabled = true;
  aiStopped = false;

  const statusEl = aiAddMsg('assistant', 'Generating…');
  statusEl.classList.add('streaming');
  const snapMsgs = AI.history.length;   // roll history back on failure
  let raw = '';          // raw streamed LLM response (markdown, fence included)
  let script = '';       // code-pane view (same text while streaming)
  let streamDone = false;
  let started = false;   // first token received
  let lastActivity = Date.now();
  const aiStarted = Date.now();

  /* Liveness nudge: if the LLM produces nothing for a while (local model
   * loading, remote queueing), show live progress so it never looks frozen.
   * The wording adapts to the active provider. */
  const nudge = () => {
    if (streamDone || aiStopped || started) return;
    const idle = Math.floor((Date.now() - lastActivity) / 1000);
    if (idle < 6) return;
    const kind = (AI.provider === 'openrouter' || AI.provider === 'custom')
      ? 'the remote provider may be queueing or slow to respond'
      : 'slow local models can take a while to load';
    const total = Math.floor((Date.now() - aiStarted) / 1000);
    statusEl.textContent = 'Still working… ' + total + 's (waiting for the first token — ' + kind + ')';
  };
  const nudgeTimer = setInterval(nudge, 1000);

  /* Render the accumulating response into the live assistant bubble. Before
   * the first token it shows the plain status line; afterwards it streams the
   * LLM's raw markdown (including the ```lua fence) exactly like a chat agent. */
  const renderBox = () => {
    if (!started) { statusEl.textContent = 'Generating…'; return; }
    statusEl.classList.add('has-md');
    statusEl.innerHTML = mdRender(raw) + (streaming ? '<span class="md-cursor"></span>' : '');
    code.textContent = raw;
    code.scrollTop = code.scrollHeight;
    const chat = $('#ai-chat');
    chat.scrollTop = chat.scrollHeight;
  };
  let streaming = true;

  aiStop = () => { aiStopped = true; if (aiReader) aiReader.cancel().catch(() => {}); };
  $('#ai-stop').classList.add('active');
  $('#ai-stop').disabled = false;

  try {
    const body = { request: prompt, history: AI.history.slice(0, snapMsgs) };
    if (ctx) body.script = ctx;
    const res = await fetch('/api/ai/stream', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    if (!res.ok) {
      const err = await res.text();
      throw new Error(err && err.includes('error') ? err : 'HTTP ' + res.status);
    }
    const reader = res.body.getReader();
    aiReader = reader;
    const decoder = new TextDecoder();
    let buf = '';
    let final = null;
    for (;;) {
      const chunk = await reader.read();
      if (chunk.done) break;
      if (!chunk.value) continue;
      buf += decoder.decode(chunk.value, { stream: true });
      let idx;
      while ((idx = buf.indexOf('\n\n')) >= 0) {
        const frame = buf.slice(0, idx).trim();
        buf = buf.slice(idx + 2);
        if (!frame.startsWith('data:')) continue;
        const payload = frame.slice(5).trim();
        if (!payload) continue;
        const ev = JSON.parse(payload);
        if (ev.type === 'token') {
          raw += ev.text; script += ev.text;
          started = true; lastActivity = Date.now();
          renderBox();
        }
        else if (ev.type === 'status') { lastActivity = Date.now(); aiAddMsg('status', ev.text); }
        else if (ev.type === 'done') { final = ev; streamDone = true; clearInterval(nudgeTimer); }
        else if (ev.type === 'error') { throw new Error(ev.text); }
      }
    }
    if (aiStopped) {
      clearInterval(nudgeTimer);
      streaming = false;
      renderBox();
      statusEl.classList.remove('streaming');
      aiAddMsg('status', 'Generation stopped — partial output kept; not applied.');
      return;
    }
    if (!final) throw new Error('stream closed before done');
    streaming = false;
    renderBox();
    statusEl.classList.remove('streaming');
    AI.generated = final.script || script;
    AI.errors = final.errors || [];
    AI.logs = final.logs || [];
    /* Store the raw response (what the user saw) as the assistant turn so the
     * conversation stays coherent on follow-ups. */
    AI.history.push({ role: 'assistant', content: raw || AI.generated });
    aiAddMsg('status', 'Done (' + (final.ok ? 'validation passed ✓' : 'has errors — Fix or edit') + ')');
    code.textContent = AI.generated;
    $('#ai-fix').disabled = !final.ok;
    $('#ai-apply').disabled = false;
    if (!final.ok && AI.errors.length) {
      aiAddMsg('error', AI.errors.join('\n'));
    }
  } catch (e) {
    streamDone = true;
    clearInterval(nudgeTimer);
    if (!aiStopped) AI.history.length = snapMsgs;   // drop the failed turn
    statusEl.classList.remove('streaming');
    statusEl.textContent = aiStopped ? 'Generation stopped' : 'Generation failed';
    if (!aiStopped) {
      const msg = e && e.message ? e.message : String(e);
      /* Browser network failures on SSE (empty reply / connection reset) surface
       * as "Failed to fetch" — turn that into actionable text. */
      const friendly = /Failed to fetch/.test(msg)
        ? 'Connection lost while talking to the model (empty reply / timeout). The model may be loading or unresponsive — check LM Studio and retry.'
        : msg;
      aiAddMsg('error', friendly);
    }
  } finally {
    aiReader = null;
    aiStop = null;
    $('#ai-stop').classList.remove('active');
    $('#ai-stop').disabled = true;
    $('#ai-generate').disabled = false;
  }
}

async function aiFix() {
  let script = AI.generated;
  if (!script) {
    script = await assembledLua();
    if (script === null) script = '';
  }
  if (!script) { setStatus('Script was empty; generate first.', 'warning'); return; }
  const prompt = 'Fix these errors: ' + (AI.errors.join('; ') || 'validation issues');
  aiAddMsg('user', prompt);
  const statusEl = aiAddMsg('assistant', 'Fixing…');
  $('#ai-generate').disabled = true;
  $('#ai-fix').disabled = true;
  const snapMsgs = AI.history.length;
  AI.history.push({ role: 'user', content: prompt });
  try {
    const r = await api('POST', '/api/ai/fix', { script, request: prompt, history: AI.history });
    AI.generated = r.script;
    AI.errors = r.errors || [];
    AI.history.push({ role: 'assistant', content: r.script });
    statusEl.textContent = r.ok ? 'Fixed ✓' : 'Fix still has errors';
    const code = $('#ai-code');
    code.textContent = AI.generated;
    code.classList.remove('hidden');
    $('#ai-apply').disabled = false;
    $('#ai-fix').disabled = false;
    if (!r.ok && AI.errors.length) aiAddMsg('error', AI.errors.join('\n'));
  } catch (e) {
    AI.history.length = snapMsgs;
    statusEl.textContent = 'Fix failed';
    aiAddMsg('error', e.message);
  } finally {
    $('#ai-generate').disabled = false;
    $('#ai-fix').disabled = false;
  }
}

async function aiApply() {
  if (!AI.generated) return;
  /* "Edit current form" is the merge signal: the generated script is overlaid
   * onto the open form (controls matched by name, base work preserved). */
  const merge = $('#ai-context').checked;
  try {
    const body = { lua: AI.generated, mode: merge ? 'merge' : 'replace' };
    if (merge) {
      if (!state.doc) throw new Error('no open form to merge into');
      body.base = state.doc;
    }
    const r = await api('POST', '/api/import', body);
    if (!r || !r.doc) {
      const why = r && r.error ? r.error : 'the generated script contains no form (no k.form.new call)';
      throw new Error('Import failed: ' + why);
    }
    snapshot('ai-import');
    replaceDoc(r.doc);
    state.selected = -1;
    currentCell = '';
    state.dirty = true;
    state.format = 'lua';
    resetValidate();
    render();
    if (state.view === 'code') refreshCode();
    setStatus((merge
      ? 'Merged AI-generated changes into the current form.'
      : 'Imported AI-generated script into the builder (replaced form).'), 'success');
    $('#ai-close').click();
  } catch (e) {
    aiAddMsg('error', 'Apply failed: ' + e.message);
  }
}

function wireAI() {
  $('#btn-ai').addEventListener('click', () => {
    const panel = $('#ai-panel');
    panel.classList.toggle('hidden');
    if (!panel.classList.contains('hidden')) aiStatus();
  });
  $('#ai-close').addEventListener('click', () => $('#ai-panel').classList.add('hidden'));
  $('#ai-generate').addEventListener('click', aiGenerate);
  $('#ai-stop').addEventListener('click', () => { if (aiStop) aiStop(); });
  $('#ai-fix').addEventListener('click', aiFix);
  $('#ai-apply').addEventListener('click', aiApply);
  $('#ai-clear').addEventListener('click', () => {
    if (aiStop) aiStop();   /* don't leave a half-open stream running */
    $('#ai-chat').innerHTML = '';
    $('#ai-code').textContent = '';
    $('#ai-code').classList.add('hidden');
    AI.generated = null; AI.errors = []; AI.logs = []; AI.history = [];
    $('#ai-fix').disabled = true; $('#ai-apply').disabled = true; $('#ai-tab-code').disabled = true;
  });
  $('#ai-prompt').addEventListener('keydown', e => {
    if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') { e.preventDefault(); aiGenerate(); }
  });
  $('#ai-tab-chat').addEventListener('click', () => {
    $('#ai-tab-chat').className = 'sel'; $('#ai-tab-code').className = '';
    $('#ai-chat').classList.remove('hidden'); $('#ai-code').classList.add('hidden');
  });
  $('#ai-tab-code').addEventListener('click', () => {
    $('#ai-tab-code').className = 'sel'; $('#ai-tab-chat').className = '';
    $('#ai-code').classList.remove('hidden'); $('#ai-chat').classList.add('hidden');
  });
  aiStatus();
}

/* ---------- table / looper editor modal ---------- */
/* The visual editor for table and looper controls (Datasource / Setup /
 * Preview tabs). Tabulator is instantiated only inside this modal so the main
 * canvas stays inert for click-to-select. */
const CM = {
  mode: null,             // 'table' | 'looper'
  tab: 'datasource',
  dbs: [],
  db: '', query: '', pageSize: undefined, countQuery: '', where: '', orderBy: '',
  tabulator: false,
  data: null,             // static rows (table mode, DB-less apps)
  columns: [],            // table workspace: {field,title,sortable,headerFilter,editor,width,align,frozen}
  links: [],              // looper legacy: {field,control,property}
  row: [],                // looper new model: array of {type,name,property,field,opts}
  rowOpts: -1,            // index of the row-template cell whose opts editor is open
  lastResult: null,       // {columns, rows} from the last Run query
  tbl: null,              // live Tabulator instance inside Preview
};

function cmModeLabel() { return CM.mode === 'looper' ? 'Looper' : 'Table'; }

function openControlModal() {
  const c = ctrl();
  if (!c || (c.type !== 'table' && c.type !== 'looper')) return;
  CM.mode = c.type;
  CM.tab = 'datasource';
  CM.tabulator = !!c.opts.tabulator;
  CM.db = c.opts.db || '';
  CM.query = c.opts.query || '';
  CM.pageSize = c.opts.page_size;
  CM.countQuery = c.opts.count_query || '';
  CM.where = c.opts.where || '';
  CM.orderBy = c.opts.order_by || '';
  CM.data = c.opts.data !== undefined ? c.opts.data : null;
  CM.columns = c.type === 'table' ? parseColumns(c.opts.columns) : [];
  CM.row = [];
  if (c.type === 'looper') {
    if (Array.isArray(c.opts.row)) {
      CM.row = c.opts.row.map(r => ({
        type: r.type || 'label', name: r.name || '', property: r.property || 'value',
        field: r.field || r.column || '', opts: r.opts || {},
      }));
    } else if (Array.isArray(c.opts.links)) {
      // Legacy: derive a label-based row template from links
      CM.row = c.opts.links.map(l => ({
        type: 'label', name: l.control || l.ctrl || l.field || '',
        field: l.field || l.column_name || '', property: l.property || l.prop || 'value',
        opts: {},
      }));
    }
  }
  // Derive legacy links from row (kept for preview/reference)
  CM.links = CM.row.map(r => ({ field: r.field, control: r.name, property: r.property }));
  CM.rowOpts = -1;
  CM.lastResult = null;
  renderControlModal();
  $('#control-modal').classList.remove('hidden');
}

function closeControlModal() {
  destroyTableTabulator();
  CM.lastResult = null;
  $('#control-modal').classList.add('hidden');
}

async function renderControlModal() {
  $('#cm-title').textContent = cmModeLabel() + ' Editor';
  $('#cm-tab-setup').textContent = CM.mode === 'looper' ? 'Row Template' : 'Table Setup';
  $('#cm-tabulator-row').style.display = CM.mode === 'table' ? '' : 'none';
  $('#cm-query').value = CM.query;
  $('#cm-page-size').value = CM.pageSize === undefined ? '' : CM.pageSize;
  $('#cm-count-query').value = CM.countQuery;
  $('#cm-where').value = CM.where;
  $('#cm-order-by').value = CM.orderBy;
  $('#cm-tabulator').checked = CM.tabulator;
  renderStaticData();
  $('#cm-datasource-out').innerHTML = '';
  $('#cm-preview-box').innerHTML = '';
  cmShowTab('datasource');
  try {
    CM.dbs = (await api('GET', '/api/db')).dbs || [];
  } catch (e) {
    CM.dbs = [];
  }
  $('#cm-db-hint').textContent = CM.dbs.length
    ? ''
    : 'No database handles — start the builder with --db NAME=DSN for live data (static data above still works).';
  const sel = $('#cm-db');
  sel.innerHTML = '<option value="">(none — static data)</option>' +
    CM.dbs.map(n => `<option value="${escapeAttr(n)}" ${n === CM.db ? 'selected' : ''}>${escapeHtml(n)}</option>`).join('');
}

function renderStaticData() {
  const wrap = $('#cm-data-row');
  wrap.classList.toggle('hidden', !!CM.db);
  $('#cm-data').value = Array.isArray(CM.data) ? JSON.stringify(CM.data, null, 1) : '';
  $('#cm-data').classList.remove('baderr');
}

/* parseColumns converts a stored columns value (Tabulator array or basic
 * {field: title} map) into the workspace array. */
function parseColumns(raw) {
  if (Array.isArray(raw)) {
    return raw.map(c => ({
      field: c.field || '', title: c.title !== undefined ? c.title : '',
      sortable: c.sortable !== false, headerFilter: c.headerFilter || 'none',
      editor: c.editor || '', width: c.width, align: c.align || '', frozen: !!c.frozen,
    }));
  }
  if (raw && typeof raw === 'object') {
    return Object.entries(raw).map(([f, t]) => ({
      field: f, title: t !== undefined && t !== null && String(t) !== f ? String(t) : titleCase(f),
      sortable: true, headerFilter: 'none', editor: '', width: undefined, align: '', frozen: false,
    }));
  }
  return [];
}

function titleCase(s) {
  return String(s).replace(/_/g, ' ').replace(/\b\w/g, c => c.toUpperCase());
}

function cmShowTab(tab) {
  CM.tab = tab;
  $$('#control-modal .cm-tabs [data-cm-tab]').forEach(b =>
    b.classList.toggle('sel', b.dataset.cmTab === tab));
  $('#cm-datasource').classList.toggle('hidden', tab !== 'datasource');
  $('#cm-setup').classList.toggle('hidden', tab !== 'setup');
  $('#cm-crud').classList.toggle('hidden', tab !== 'crud');
  $('#cm-preview').classList.toggle('hidden', tab !== 'preview');
  if (tab === 'setup') cmRenderSetup();
  if (tab === 'crud') cmRenderCrud();
  if (tab === 'preview') cmRenderPreview();
}

/* ---------- Datasource ---------- */
async function cmRunQuery() {
  const out = $('#cm-datasource-out');
  if (!CM.db) { out.innerHTML = '<div class="hint">Select a database first (or paste static data).</div>'; return; }
  if (!CM.query.trim()) { out.innerHTML = '<div class="hint">Enter a query first.</div>'; return; }
  try {
    const r = await api('POST', '/api/db/query', { db: CM.db, query: CM.query, limit: CM.pageSize || 50 });
    CM.lastResult = r;
    const cols = r.columns || [];
    out.innerHTML = '<div class="cm-hint">' + (r.rows || []).length + ' sample row(s)</div>' +
      cmMiniGrid(cmColDefsFor(cols), r.rows || []);
  } catch (e) {
    out.innerHTML = '<div class="error">' + escapeHtml(e.message) + '</div>';
  }
}

function cmGenerateColumns() {
  const dbCols = CM.lastResult && CM.lastResult.columns ? CM.lastResult.columns : [];
  let fields = dbCols.slice();
  if (!fields.length && Array.isArray(CM.data) && CM.data[0] && typeof CM.data[0] === 'object') {
    fields = Object.keys(CM.data[0]);
  }
  if (!fields.length) {
    setStatus('No columns yet — run the query or paste static data first.', 'warning');
    return;
  }
  if (CM.mode === 'table') {
    CM.columns = fields.map(f => ({
      field: f, title: titleCase(f), sortable: true, headerFilter: 'none',
      editor: '', width: undefined, align: '', frozen: false,
    }));
    cmShowTab('setup');
    cmRenderSetup();
  } else {
    // Row template: one label cell per column (field → text display)
    CM.row = fields.map(f => ({ type: 'label', name: f, field: f, property: 'text', opts: {} }));
    cmRenderSetup();
  }
}

function cmMiniGrid(colDefs, rows) {
  if (!colDefs || !colDefs.length) return '<div class="hint">No rows to show.</div>';
  const head = '<tr>' + colDefs.map(c => '<th>' + escapeHtml(String(c.name)) + '</th>').join('') + '</tr>';
  const body = rows.slice(0, 100).map(r => {
    const tds = colDefs.map(c => {
      let v = '';
      if (Array.isArray(r)) v = c.idx >= 0 ? (r[c.idx] ?? '') : '';
      else if (r && typeof r === 'object') v = r[c.field] !== undefined ? r[c.field] : '';
      return '<td>' + escapeHtml(v === null || v === undefined ? '' : String(v)) + '</td>';
    }).join('');
    return '<tr>' + tds + '</tr>';
  }).join('');
  return '<table class="cm-table"><thead>' + head + '</thead><tbody>' + body + '</tbody></table>';
}

function cmColDefsFor(columns) {
  return (columns || []).map((c, i) => ({ name: c, field: c, idx: i }));
}

/* ---------- CRUD Grid ---------- */
function cmRenderCrud() {
  // PK Field
  $('#cm-pk-field').value = CM.pkField || '';

  // Selection Mode
  $('#cm-selection-mode').value = CM.selectionMode || 'multi';

  // Column Visibility
  $('#cm-column-visibility').checked = !!CM.columnVisibility;

  // Row Click Action (mutually exclusive)
  $('#cm-row-click-select').checked = CM.rowClickAction === 'select';
  $('#cm-row-click-view').checked = CM.rowClickAction === 'view';
  $('#cm-row-click-edit').checked = CM.rowClickAction === 'edit';

  // Default Visible Columns
  const dv = Array.isArray(CM.defaultVisible) ? CM.defaultVisible.join(',') : '';
  $('#cm-default-visible').checked = !!CM.defaultVisible && CM.defaultVisible.length > 0;
  $('#cm-default-visible-row').classList.toggle('hidden', !$('#cm-default-visible').checked);
  $('#cm-default-visible-input').hidden = !$('#cm-default-visible').checked;
  $('#cm-default-visible-cols').value = dv;

  // Row Actions
  $('#cm-row-view').checked = !!(CM.rowActions && CM.rowActions.view);
  $('#cm-row-edit').checked = !!(CM.rowActions && CM.rowActions.edit);
  $('#cm-row-delete').checked = !!(CM.rowActions && CM.rowActions.delete);

  // Global Actions
  $('#cm-global-new').checked = !!(CM.globalActions && CM.globalActions.new_record);
  $('#cm-global-batch-del').checked = !!(CM.globalActions && CM.globalActions.batch_delete);

  // Form
  $('#cm-form-ref').value = CM.formRef || '';
  $('#cm-form-width').value = CM.formWidth || '80%';
  const inline = !!CM.inlineForm;
  $('#cm-inline-form').checked = inline;
  $('#cm-inline-form-editor').hidden = !inline;
  if (inline) {
    $('#cm-inline-title').value = CM.inlineTitle || '';
    $('#cm-inline-gap').value = CM.inlineGap || 10;
    renderInlineFormControls();
  }
}

function renderInlineFormControls() {
  const box = $('#cm-inline-controls');
  const ctrls = CM.inlineControls || [];
  box.innerHTML = (ctrls.length ? '' : '<div class="hint">No controls yet — Add a control below.</div>') +
    ctrls.map((ctrl, i) => `
      <div class="cm-colrow" data-i="${i}" data-ctype="${escapeAttr(ctrl.type)}">
        <button class="mv up" data-cmcol="up" title="Move up">▲</button>
        <button class="mv dn" data-cmcol="down" title="Move down">▼</button>
        <button class="del" data-cmcol="del" title="Delete control">×</button>
        <select data-cminlinef="type">
          ${CM_CTRL_TYPES.map(t => `<option value="${t}" ${t === ctrl.type ? 'selected' : ''}>${t}</option>`).join('')}
        </select>
        <input data-cminlinef="name" value="${escapeAttr(ctrl.name)}" placeholder="name">
        <input data-cminlinef="label" value="${escapeAttr(ctrl.label || '')}" placeholder="label">
        <button class="opt" data-cminlinef="opts" data-i="${i}" title="Options">⚙</button>
      </div>`).join('') +
    `<div class="cm-add-row">
      <label>Add control: </label>
      <select id="cm-inline-add-type">
        ${CM_CTRL_TYPES.map(t => `<option value="${t}">${t}</option>`).join('')}
      </select>
      <button id="cm-inline-add-ctrl" data-cm-inline-add="1">Add</button>
    </div>`;
}

const CM_CTRL_TYPES = [
  'label', 'textbox', 'button', 'combo', 'list', 'table', 'checkbox', 'radio', 'image', 'chart'
];
  if (CM.mode === 'looper') return renderRowTemplate();
  $('#cm-col-hint').textContent = 'Tabulator → columns array; basic table → {field: title} map.';
  const box = $('#cm-columns');
  box.innerHTML = (CM.columns.length ? '' : '<div class="hint">No columns yet — Generate columns from the query result or add below.</div>') +
    CM.columns.map((col, i) => `
      <div class="cm-colrow" data-i="${i}">
        <button class="mv up" data-cmcol="up" title="Move up">▲</button>
        <button class="mv dn" data-cmcol="down" title="Move down">▼</button>
        <button class="del" data-cmcol="del" title="Delete column">×</button>
        <input data-cmcolf="field" value="${escapeAttr(col.field)}" placeholder="field">
        <input data-cmcolf="title" value="${escapeAttr(col.title)}" placeholder="title">
        <label class="chk">sort<input type="checkbox" data-cmcolf="sortable" ${col.sortable ? 'checked' : ''}></label>
        <select data-cmcolf="headerFilter">
          <option value="none" ${col.headerFilter === 'none' ? 'selected' : ''}>filter —</option>
          <option value="text" ${col.headerFilter === 'text' ? 'selected' : ''}>filter text</option>
          <option value="number" ${col.headerFilter === 'number' ? 'selected' : ''}>filter number</option>
        </select>
        <input data-cmcolf="editor" value="${escapeAttr(col.editor)}" placeholder="editor">
        <input data-cmcolf="width" type="number" value="${col.width === undefined ? '' : col.width}" placeholder="width">
        <select data-cmcolf="align">
          <option value="" ${col.align === '' ? 'selected' : ''}>align —</option>
          <option value="left" ${col.align === 'left' ? 'selected' : ''}>left</option>
          <option value="center" ${col.align === 'center' ? 'selected' : ''}>center</option>
          <option value="right" ${col.align === 'right' ? 'selected' : ''}>right</option>
        </select>
        <label class="chk">frozen<input type="checkbox" data-cmcolf="frozen" ${col.frozen ? 'checked' : ''}></label>
      </div>`).join('');
}

const LOOPER_CELL_TYPES = [
  {type: 'label', label: 'Label', defaultProp: 'text'},
  {type: 'textbox', label: 'Textbox (display)', defaultProp: 'value'},
  {type: 'checkbox', label: 'Checkbox (display)', defaultProp: 'value'},
  {type: 'image', label: 'Image', defaultProp: 'src'},
];

function renderRowTemplate() {
  $('#cm-col-hint').textContent = 'Row template: each control maps a data field to a display property. Drag to reorder.';
  const box = $('#cm-columns');
  box.innerHTML = (CM.row.length ? '' : '<div class="hint">No cells yet — Add a cell type below.</div>') +
    CM.row.map((rc, i) => {
      const propOpts = getCellPropOptions(rc.type);
      return `
      <div class="cm-colrow" data-i="${i}" data-ctype="${escapeAttr(rc.type)}">
        <button class="mv up" data-cmcol="up" title="Move up">▲</button>
        <button class="mv dn" data-cmcol="down" title="Move down">▼</button>
        <button class="del" data-cmcol="del" title="Delete cell">×</button>
        <select data-cmrowf="type">
          ${LOOPER_CELL_TYPES.map(t => `<option value="${t.type}" ${t.type === rc.type ? 'selected' : ''}>${t.label}</option>`).join('')}
        </select>
        <input data-cmrowf="name" value="${escapeAttr(rc.name)}" placeholder="cell key (control name)" title="Unique key for this cell">
        <input data-cmrowf="field" value="${escapeAttr(rc.field)}" placeholder="data field" title="Column name from query result">
        <select data-cmrowf="property">
          ${propOpts.map(p => `<option value="${p}" ${p === rc.property ? 'selected' : ''}>${p}</option>`).join('')}
        </select>
        <button class="opt" data-cmrowf="opts" data-i="${i}" title="Cell options">⚙</button>
      </div>`;
    }).join('') +
    `<div class="cm-add-row">
      <label>Add cell: </label>
      <select id="cm-add-cell-type">
        ${LOOPER_CELL_TYPES.map(t => `<option value="${t.type}">${t.label}</option>`).join('')}
      </select>
      <button id="cm-add-cell" data-cm-addcell="1">Add</button>
    </div>` +
    (CM.rowOpts >= 0 && CM.rowOpts < CM.row.length ? renderRowCellOpts(CM.row[CM.rowOpts], CM.rowOpts) : '');
}

const LOOPER_CELL_OPTS = {
  label:   { multiline: { t: 'bool' } },
  textbox: { multiline: { t: 'bool' }, rows: { t: 'number' }, cols: { t: 'number' }, placeholder: { t: 'string' } },
  checkbox:{ label: { t: 'string' } },
  image:   { alt: { t: 'string' }, width: { t: 'string' }, height: { t: 'string' },
             fit: { t: 'select', opts: ['contain', 'cover', 'fill', 'scale-down', 'none'] } },
};

function renderRowCellOpts(rc, i) {
  const fields = LOOPER_CELL_OPTS[rc.type] || {};
  const entries = Object.entries(fields);
  if (!entries.length) return '<div class="cm-hint">No extra options for this cell type.</div>';
  return `
    <div class="cm-cell-opts" data-i="${i}">
      <div class="cm-hint">Options for cell “${escapeHtml(rc.name || rc.type + ' ' + (i + 1))}”:</div>
      <div class="cm-grid">
      ${entries.map(([k, s]) => {
        const v = rc.opts[k];
        if (s.t === 'bool') {
          return `<label class="chk">${k}<input type="checkbox" data-cmopt="bool" data-optkey="${k}" ${v ? 'checked' : ''}></label>`;
        }
        if (s.t === 'select') {
          return `<label>${k}<select data-cmopt="raw" data-optkey="${k}">${s.opts.map(o => `<option value="${o}" ${String(v) === o ? 'selected' : ''}>${o}</option>`).join('')}</select></label>`;
        }
        if (s.t === 'number') {
          return `<label>${k}<input type="number" data-cmopt="num" data-optkey="${k}" value="${v === undefined ? '' : escapeAttr(String(v))}"></label>`;
        }
        return `<label>${k}<input type="text" data-cmopt="raw" data-optkey="${k}" value="${v === undefined ? '' : escapeAttr(String(v))}"></label>`;
      }).join('')}
      </div>
      <button class="opt close" data-cmoptclose="1">Done</button>
    </div>`;
}

function removeEmptyOpts(rc) {
  const o = {};
  Object.entries(rc.opts || {}).forEach(([k, v]) => {
    if (v === undefined || v === null || v === '') return;
    o[k] = v;
  });
  rc.opts = o;
}

function getCellPropOptions(type) {
  switch (type) {
    case 'label': return ['text'];
    case 'textbox': return ['value'];
    case 'checkbox': return ['value', 'hidden_value'];
    case 'image': return ['src', 'alt', 'width', 'height'];
    default: return ['value'];
  }
}

function getDefaultCell(type) {
  const t = LOOPER_CELL_TYPES.find(x => x.type === type) || LOOPER_CELL_TYPES[0];
  return {type, name: '', field: '', property: t.defaultProp, opts: {}};
}

function wireCMSetup() {
  $('#cm-columns').addEventListener('input', onCMSetupInput);
  $('#cm-columns').addEventListener('change', onCMSetupInput);
  $('#cm-columns').addEventListener('click', e => {
    // Add-cell button (rendered dynamically with the Row Template editor, so
    // it is handled here via delegation rather than a startup binding).
    if (e.target.closest('[data-cm-addcell]')) {
      const type = $('#cm-add-cell-type').value;
      CM.row.push(getDefaultCell(type));
      cmRenderSetup();
      return;
    }
    // Cell options open/close
    const optBtn = e.target.closest('[data-cmrowf="opts"]');
    if (optBtn) {
      const i = +optBtn.dataset.i;
      CM.rowOpts = CM.rowOpts === i ? -1 : i;
      cmRenderSetup();
      return;
    }
    const optClose = e.target.closest('[data-cmoptclose]');
    if (optClose) {
      CM.rowOpts = -1;
      cmRenderSetup();
      return;
    }
    const btn = e.target.closest('[data-cmcol]');
    if (!btn) return;
    const row = btn.closest('.cm-colrow');
    if (!row) return;
    const i = +row.dataset.i;
    let arr;
    if (CM.mode === 'table') arr = CM.columns;
    else if (CM.mode === 'looper') arr = CM.row;
    else return;
    const act = btn.dataset.cmcol;
    if (act === 'up' && i > 0) { const t = arr[i - 1]; arr[i - 1] = arr[i]; arr[i] = t; }
    else if (act === 'down' && i < arr.length - 1) { const t = arr[i + 1]; arr[i + 1] = arr[i]; arr[i] = t; }
    else if (act === 'del') { arr.splice(i, 1); CM.rowOpts = -1; }
    else return;
    cmRenderSetup();
  });
  $('#cm-add-col').addEventListener('click', () => {
    if (CM.mode === 'table') CM.columns.push({ field: '', title: '', sortable: true, headerFilter: 'none', editor: '', width: undefined, align: '', frozen: false });
    else if (CM.mode === 'looper') { CM.row.push(getDefaultCell('label')); CM.rowOpts = -1; }
    cmRenderSetup();
  });
}

function onCMSetupInput(e) {
  const t = e.target;
  // Cell options editor (not inside a .cm-colrow)
  if (t.dataset.cmopt) {
    const wrap = t.closest('.cm-cell-opts');
    if (!wrap) return;
    const i = +wrap.dataset.i;
    const rc = CM.row[i];
    if (!rc) return;
    const key = t.dataset.optkey;
    if (t.dataset.cmopt === 'bool') rc.opts[key] = t.checked;
    else if (t.dataset.cmopt === 'num') rc.opts[key] = t.value === '' ? '' : +t.value;
    else rc.opts[key] = t.value;
    return;
  }
  const row = t.closest('.cm-colrow');
  if (!row) return;
  const i = +row.dataset.i;
  if (CM.mode === 'looper') {
    const k = t.dataset.cmrowf;
    const rc = CM.row[i];
    if (!rc) return;
    if (k === 'type') {
      rc.type = t.value;
      // Update property options when type changes
      const propOpts = getCellPropOptions(rc.type);
      if (!propOpts.includes(rc.property)) rc.property = propOpts[0];
      cmRenderSetup();
    } else {
      rc[k] = t.value;
    }
    return;
  }
  const k = t.dataset.cmcolf;
  const c = CM.columns[i];
  if (!c) return;
  if (k === 'sortable' || k === 'frozen') c[k] = t.checked;
  else if (k === 'width') c[k] = t.value === '' ? undefined : +t.value;
  else c[k] = t.value;
if (CM.mode === 'table') CM.columns.push({ field: '', title: '', sortable: true, headerFilter: 'none', editor: '', width: undefined, align: '', frozen: false });
    else if (CM.mode === 'looper') { CM.row.push(getDefaultCell('label')); CM.rowOpts = -1; }
    cmRenderSetup();
  });
}

/* ---------- CRUD Grid wiring ---------- */
function wireCMCrud() {
  // PK Field
  $('#cm-pk-field').addEventListener('input', e => { CM.pkField = e.target.value; });
  // Selection Mode
  $('#cm-selection-mode').addEventListener('change', e => { CM.selectionMode = e.target.value; });
  // Column Visibility
  $('#cm-column-visibility').addEventListener('change', e => { CM.columnVisibility = e.target.checked; });
  // Row Click Action (mutually exclusive)
  $('#cm-row-click-select').addEventListener('change', e => { if (e.target.checked) CM.rowClickAction = 'select'; });
  $('#cm-row-click-view').addEventListener('change', e => { if (e.target.checked) CM.rowClickAction = 'view'; });
  $('#cm-row-click-edit').addEventListener('change', e => { if (e.target.checked) CM.rowClickAction = 'edit'; });
  // Default Visible Columns
  $('#cm-default-visible').addEventListener('change', e => {
    const checked = e.target.checked;
    $('#cm-default-visible-row').classList.toggle('hidden', !checked);
    $('#cm-default-visible-input').hidden = !checked;
    if (!checked) CM.defaultVisible = null;
    else if (!CM.defaultVisible) CM.defaultVisible = [];
  });
  $('#cm-default-visible-cols').addEventListener('input', e => {
    CM.defaultVisible = e.target.value.split(',').map(s => s.trim()).filter(Boolean);
  });
  // Row Actions
  $('#cm-row-view').addEventListener('change', e => { CM.rowActions = CM.rowActions || {}; CM.rowActions.view = e.target.checked; });
  $('#cm-row-edit').addEventListener('change', e => { CM.rowActions = CM.rowActions || {}; CM.rowActions.edit = e.target.checked; });
  $('#cm-row-delete').addEventListener('change', e => { CM.rowActions = CM.rowActions || {}; CM.rowActions.delete = e.target.checked; });
  // Global Actions
  $('#cm-global-new').addEventListener('change', e => { CM.globalActions = CM.globalActions || {}; CM.globalActions.new_record = e.target.checked; });
  $('#cm-global-batch-del').addEventListener('change', e => { CM.globalActions = CM.globalActions || {}; CM.globalActions.batch_delete = e.target.checked; });
  // Form
  $('#cm-form-ref').addEventListener('input', e => { CM.formRef = e.target.value; });
  $('#cm-form-width').addEventListener('input', e => { CM.formWidth = e.target.value; });
  $('#cm-inline-form').addEventListener('change', e => {
    const checked = e.target.checked;
    CM.inlineForm = checked;
    $('#cm-inline-form-editor').hidden = !checked;
    if (checked) renderInlineFormControls();
  });
  // Inline Form
  $('#cm-inline-title').addEventListener('input', e => { CM.inlineTitle = e.target.value; });
  $('#cm-inline-gap').addEventListener('input', e => { CM.inlineGap = e.target.value === '' ? undefined : +e.target.value; });
  // Inline Form Controls
  $('#cm-inline-controls').addEventListener('click', e => {
    const delBtn = e.target.closest('[data-cmcol="del"]');
    if (delBtn) {
      const row = delBtn.closest('.cm-colrow');
      if (!row) return;
      const i = +row.dataset.i;
      CM.inlineControls.splice(i, 1);
      renderInlineFormControls();
      return;
    }
    const optBtn = e.target.closest('[data-cminlinef="opts"]');
    if (optBtn) {
      // TODO: inline control options
      return;
    }
  });
  $('#cm-inline-controls').addEventListener('input', e => {
    const row = e.target.closest('.cm-colrow');
    if (!row) return;
    const i = +row.dataset.i;
    const ctrl = CM.inlineControls[i];
    if (!ctrl) return;
    const k = e.target.dataset.cminlinef;
    if (k === 'type') {
      ctrl.type = e.target.value;
      ctrl.opts = ctrl.opts || {};
    } else {
      ctrl[k] = e.target.value;
    }
  });
  $('#cm-inline-add-ctrl').addEventListener('click', () => {
    const type = $('#cm-inline-add-type').value;
    CM.inlineControls = CM.inlineControls || [];
    CM.inlineControls.push({ type, name: '', label: '', opts: {} });
    renderInlineFormControls();
  });
}

/* ---------- Preview ---------- */
async function cmFetchRows(limit) {
  if (CM.db && CM.query.trim()) {
    const r = await api('POST', '/api/db/query', { db: CM.db, query: CM.query, limit });
    return r.rows || [];
  }
  return Array.isArray(CM.data) ? CM.data : [];
}

async function cmFetchLooperRows(limit) {
  if (!CM.db || !CM.query.trim() || !CM.row.length) return null;
  try {
    const rowDefs = CM.row.filter(rc => rc.name && rc.field).map(rc => ({
      type: rc.type, name: rc.name, property: rc.property, field: rc.field, opts: rc.opts || {},
    }));
    const r = await api('POST', '/api/looper/rows', { db: CM.db, query: CM.query, row: rowDefs, limit });
    return { columns: r.columns, rows: r.html_rows || [] };
  } catch (e) {
    return null;
  }
}

async function cmRenderPreview() {
  const hint = $('#cm-preview-hint');
  const box = $('#cm-preview-box');
  destroyTableTabulator();
  box.innerHTML = '<div class="hint">Loading…</div>';

  if (CM.mode === 'looper') {
    // Try pixel-faithful preview via /api/looper/rows when row template exists
    let usedLooperEndpoint = false;
    if (CM.row.length) {
      const lr = await cmFetchLooperRows(20);
      if (lr) {
        usedLooperEndpoint = true;
        hint.textContent = 'Live looper row-template preview (server-rendered).';
        // html_rows are {index, html} - render as a list of rendered rows
        box.innerHTML = '<div class="looper-preview">' +
          lr.rows.map(r => `<div class="looper-row">${r.html}</div>`).join('') +
          '</div>';
        return;
      }
    }
    // Fallback: basic grid preview from /api/db/query
    hint.textContent = CM.db ? 'Live sample rows (basic grid — row template not configured).' : 'Static sample rows.';
    let rows = [];
    try { rows = await cmFetchRows(50); } catch (e) { box.innerHTML = '<div class="error">' + escapeHtml(e.message) + '</div>'; return; }
    const cols = CM.lastResult && CM.lastResult.columns ? CM.lastResult.columns : (rows[0] && typeof rows[0] === 'object' ? Object.keys(rows[0]) : []);
    box.innerHTML = cmMiniGrid(cmColDefsFor(cols), rows);
    return;
  }

  const defs = CM.columns.filter(c => c.field);
  if (CM.tabulator) {
    if (!defs.length) { box.innerHTML = '<div class="hint">Add columns in the Table Setup tab first.</div>'; return; }
    let rows = [];
    try { rows = await cmFetchRows(CM.pageSize || 50); } catch (e) { box.innerHTML = '<div class="error">' + escapeHtml(e.message) + '</div>'; return; }
    hint.textContent = CM.db ? 'Live Tabulator preview (read-only).' : 'Static data Tabulator preview.';
    initTableTabulator(defs.map(cmTabulatorColumn), rows);
    return;
  }

  hint.textContent = 'Basic table (no Tabulator).';
  let rows = [];
  try { rows = await cmFetchRows(50); } catch (e) { box.innerHTML = '<div class="error">' + escapeHtml(e.message) + '</div>'; return; }
  if (!defs.length) { box.innerHTML = '<div class="hint">Add columns in the Table Setup tab first.</div>'; return; }
  const dbCols = CM.lastResult && CM.lastResult.columns ? CM.lastResult.columns : [];
  const colDefs = defs.map(c => ({ name: c.title || c.field, field: c.field, idx: dbCols.indexOf(c.field) }));
  box.innerHTML = cmMiniGrid(colDefs, rows);
}

function cmTabulatorColumn(col) {
  const c = { title: col.title || col.field, field: col.field };
  if (col.sortable !== false) c.sortable = true;
  if (col.headerFilter === 'text') c.headerFilter = 'input';
  else if (col.headerFilter === 'number') c.headerFilter = 'number';
  if (col.editor) c.editor = col.editor;
  if (col.width) c.width = col.width;
  if (col.align) c.hozAlign = col.align;
  if (col.frozen) c.frozen = true;
  return c;
}

function initTableTabulator(columns, rows) {
  if (typeof Tabulator === 'undefined') {
    $('#cm-preview-box').innerHTML = '<div class="hint">Tabulator bundle not loaded.</div>';
    return;
  }
  destroyTableTabulator();
  CM.tbl = new Tabulator('#cm-preview-box', {
    layout: 'fitColumns',
    columns,
    data: rows,
    pagination: false,
  });
}

function destroyTableTabulator() {
  if (CM.tbl) { try { CM.tbl.destroy(); } catch (e) { /* ignore */ } CM.tbl = null; }
}

/* ---------- Apply / Cancel ---------- */
function cmApply() {
  const c = ctrl();
  if (!c || CM.mode !== c.type) return;
  snapshot('edit-cm');
  setOpt(c, 'db', CM.db);
  setOpt(c, 'query', CM.query);
  setOpt(c, 'page_size', CM.pageSize);
  setOpt(c, 'count_query', CM.countQuery);
  setOpt(c, 'where', CM.where);
  setOpt(c, 'order_by', CM.orderBy);
  if (CM.mode === 'table') {
    if (CM.tabulator) setOpt(c, 'tabulator', true); else delete c.opts.tabulator;
    if (CM.tabulator) {
      const cols = CM.columns.filter(x => x.field).map(x => {
        const o = { field: x.field };
        if (x.title) o.title = x.title;
        o.sortable = x.sortable !== false;
        if (x.headerFilter === 'text' || x.headerFilter === 'number') o.headerFilter = x.headerFilter;
        if (x.editor) o.editor = x.editor;
        if (x.width) o.width = x.width;
        if (x.align) o.align = x.align;
        if (x.frozen) o.frozen = true;
        return o;
      });
      setOpt(c, 'columns', cols.length ? cols : null);
    } else {
      const m = {};
      CM.columns.forEach(x => { if (x.field) m[x.field] = x.title || x.field; });
      setOpt(c, 'columns', Object.keys(m).length ? m : null);
    }
    if (!CM.db) setOpt(c, 'data', CM.data); else delete c.opts.data;
  } else {
    // Save new row model
    const row = CM.row.filter(rc => rc.name && rc.field).map(rc => {
      const o = { type: rc.type, name: rc.name, field: rc.field, property: rc.property };
      removeEmptyOpts(rc);
      if (rc.opts && Object.keys(rc.opts).length) o.opts = rc.opts;
      return o;
    });
    setOpt(c, 'row', row.length ? row : null);
    // Derive legacy links from row for backward compatibility
    const links = row.map(rc => ({
      field: rc.field, control: rc.name, property: rc.property,
    }));
    setOpt(c, 'links', links.length ? links : null);
  }
  schedulePreview();
  closeControlModal();
  setStatus('Applied ' + cmModeLabel().toLowerCase() + ' settings.');
}

function wireControlModal() {
  $('#ctrl-edit').addEventListener('click', openControlModal);
  $('#cm-close').addEventListener('click', closeControlModal);
  $('#cm-cancel').addEventListener('click', closeControlModal);
  $$('#control-modal .cm-tabs [data-cm-tab]').forEach(b =>
    b.addEventListener('click', () => cmShowTab(b.dataset.cmTab)));
  $('#cm-db').addEventListener('change', e => { CM.db = e.target.value; renderStaticData(); });
  $('#cm-query').addEventListener('input', e => { CM.query = e.target.value; });
  $('#cm-page-size').addEventListener('input', e => { CM.pageSize = e.target.value === '' ? undefined : +e.target.value; });
  $('#cm-count-query').addEventListener('input', e => { CM.countQuery = e.target.value; });
  $('#cm-where').addEventListener('input', e => { CM.where = e.target.value; });
  $('#cm-order-by').addEventListener('input', e => { CM.orderBy = e.target.value; });
  $('#cm-tabulator').addEventListener('change', e => { CM.tabulator = e.target.checked; });
  $('#cm-run-query').addEventListener('click', cmRunQuery);
  $('#cm-generate-cols').addEventListener('click', cmGenerateColumns);
  $('#cm-static-data').addEventListener('click', () => $('#cm-data-row').classList.toggle('hidden'));
  $('#cm-data').addEventListener('input', e => {
    const raw = e.target.value.trim();
    if (!raw) { CM.data = null; e.target.classList.remove('baderr'); return; }
    try { CM.data = JSON.parse(raw); e.target.classList.remove('baderr'); }
    catch (err) { e.target.classList.add('baderr'); }
  });
  wireCMSetup();
  wireCMCrud();
  $('#cm-apply').addEventListener('click', cmApply);
}

/* ---------- init ---------- */
wirePalette();
wireControlList();
wireFormProps();
wireEditor();
wirePreview();
wireCanvasSwitch();
wireTopbar();
wireTheme();
wireAI();
wireControlModal();
loadInitial().catch(e => {
  $('#preview').innerHTML = '<div class="error">Failed to load: ' + escapeHtml(e.message) + '</div>';
  setStatus(e.message, 'error');
});