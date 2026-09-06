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
             page_size: { t: 'number' }, count_query: { t: 'string' } },
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
  previewTimer: null, jsonErrors: {},
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

function form() { return state.doc.form; }
function ctrl() { return state.doc.form.controls[state.selected]; }

function normalizeDoc(doc) {
  if (!doc.form) doc.form = { name: 'main', layout: 'vertical', align: 'left', controls: [] };
  const f = doc.form;
  if (!f.name) f.name = 'main';
  if (!f.layout) f.layout = 'vertical';
  if (!f.align) f.align = 'left';
  if (!f.controls) f.controls = [];
  if (!f.cells) f.cells = {};
  if (!f.handlers) f.handlers = {};
  if (!f.notes) f.notes = [];
  for (const c of f.controls) {
    if (!c.opts) c.opts = {};
    if (c.type === 'label' && c.opts.text !== undefined && c.opts.label === undefined) {
      c.opts.label = c.opts.value; /* migrated on import */
    }
  }
  return doc;
}

/* ---------- API endpoints (document-driven) ---------- */
async function loadInitial() {
  const data = await api('GET', '/api/form');
  state.path = data.path;
  state.format = data.format;
  state.doc = normalizeDoc(data.doc);
  render();
  setStatus(data.message || (state.format === 'lua' ? 'Imported from Lua source' : 'Loaded JSON document'));
}

async function save() {
  try {
    const r = await api('PUT', '/api/form', { doc: state.doc });
    state.dirty = false;
    setStatus(r.message);
    $('#btn-save').textContent = state.format === 'lua' ? 'Save' : 'Save';
  } catch (e) {
    setStatus('Save failed: ' + e.message, true);
  }
}

async function exportLua() {
  try {
    const r = await api('POST', '/api/export', { doc: state.doc });
    showModal('Generated Lua (' + r.lua.split('\n').length + ' lines)', r.lua);
    return r.lua;
  } catch (e) { setStatus('Export failed: ' + e.message, true); return null; }
}

async function validate() {
  let lua = null;
  if (state.format === 'lua') {
    /* ask the server for the source as first imported from the file */
    try { lua = (await api('GET', '/api/source')).source; } catch (e) { /* fall through */ }
  }
  if (lua === null) lua = await exportLua();
  if (lua === null) return;
  try {
    const r = await api('POST', '/api/validate', { lua });
    const lines = [];
    for (const e of r.errors) lines.push('ERROR  ' + e);
    for (const e of r.issues) lines.push('WARN   ' + e);
    if (!r.errors && r.ok === false) lines.push('ERROR  ' + r.error);
    if (!lines.length) lines.push('No issues found — ' + lua.split('\n').length + ' lines OK.');
    showModal('Validation', lines.join('\n'));
  } catch (e) { setStatus('Validate failed: ' + e.message, true); }
}

function schedulePreview() {
  clearTimeout(state.previewTimer);
  state.previewTimer = setTimeout(preview, 150);
}

async function preview() {
  if (!state.doc) return;
  try {
    const r = await api('POST', '/api/preview', { doc: state.doc });
    $('#preview').innerHTML = r.html;
    if (state.selected >= 0) flashSelected(form().controls[state.selected]?.name);
  } catch (e) {
    $('#preview').innerHTML = '<div class="error">' + escapeHtml(e.message) + '</div>';
  }
}

/* ---------- rendering ---------- */
function render() {
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
  $('#form-title').textContent = state.doc.form.title || state.doc.form.name;
}

function renderFormProps() {
  const f = state.doc.form;
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
    head.textContent = 'Control';
    $('#ctrl-type').textContent = '';
    editor.innerHTML = '<span class="hint">Select a control in the preview or the list.</span>';
    return;
  }
  const c = ctrl();
  head.textContent = c.name;
  $('#ctrl-type').textContent = TYPE_NAMES[c.type] || c.type;
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
      const dots = Object.keys(form().cells || {}).map(id =>
        `<option value="${id}" ${v === id ? 'selected' : ''}>${id}</option>`).join('');
      return `<label>${label}<select data-k="${k}" data-sel=""><option value="">—</option>${dots}</select></label>`;
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

function renderCells() {
  const f = form();
  const sel = $('#f_cellsel');
  const ids = Object.keys(f.cells || {});
  sel.innerHTML = `<option value="">(none)</option>` +
    ids.map(id => `<option value="${id}" ${id === currentCell ? 'selected' : ''}>${id}</option>`).join('');
  const cell = f.cells[currentCell];
  $('#c_width').value = cell ? cell.width || '' : '';
  $('#c_bg').value = cell ? cell.bg || '' : '';
  $('#c_align').value = cell ? cell.align || '' : '';
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
  const f = form();
  f.controls.splice(i, 1);
  delete f.handlers[f.controls[i]?.name || ''];
  if (state.selected > i) state.selected--;
  else if (state.selected === i) state.selected = -1;
  render();
}

function moveControl(i, dir) {
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
  const id = 'c' + (Object.keys(f.cells || {}).length + 1);
  if (!f.cells) f.cells = {};
  f.cells[id] = { width: 12 };
  currentCell = id;
  renderCells();
  schedulePreview();
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
  bind('#f_name', e => { form().name = e.target.value; state.dirty = true; });
  bind('#f_layout', e => { form().layout = e.target.value; state.dirty = true; });
  bind('#f_align', e => { form().align = e.target.value; state.dirty = true; });
  bind('#f_title', e => {
    const v = e.target.value;
    form().title = v;
    if (!v) delete form().title;
    state.dirty = true;
  });
  bind('#f_gap', e => {
    const v = e.target.value;
    if (v === '') { state.dirty = true; return; }
    form().gap = +v;
    state.dirty = true;
  });

  $('#f_cellsel').addEventListener('change', e => {
    currentCell = e.target.value;
    renderCells();
  });
  $('#btn-add-cell').addEventListener('click', addCell);
  const cellInput = (id, fn) => $(id).addEventListener('change', e => {
    if (!currentCell) { setStatus('Select a cell first', true); return; }
    fn(e);
    schedulePreview();
  });
  cellInput('#c_width', e => {
    const v = +e.target.value;
    if (v >= 1 && v <= 12) form().cells[currentCell].width = v;
  });
  cellInput('#c_bg', e => {
    const v = e.target.value;
    if (v) form().cells[currentCell].bg = v; else delete form().cells[currentCell].bg;
  });
  cellInput('#c_align', e => {
    const v = e.target.value;
    if (v) form().cells[currentCell].align = v; else delete form().cells[currentCell].align;
  });
}

function wireEditor() {
  $('#ctrl-editor').addEventListener('input', onEditorInput);
  $('#ctrl-editor').addEventListener('change', onEditorInput);
}

function onEditorInput(e) {
  const t = e.target;
  const k = t.dataset.k;
  if (!k || state.selected < 0) return;
  const c = ctrl();
  if (t.dataset.bool !== undefined) {
    if (t.checked) setOpt(c, k, true); else delete c.opts[k];
  } else if (t.dataset.num !== undefined) {
    if (t.value === '') delete c.opts[k]; else setOpt(c, k, +t.value);
  } else if (t.dataset.str !== undefined) {
    setOpt(c, k, t.value);
  } else if (t.dataset.sel !== undefined) {
    if (t.value) setOpt(c, k, t.value); else delete c.opts[k];
  } else if (t.dataset.json !== undefined) {
    const raw = t.value.trim();
    if (!raw) { delete c.opts[k]; delete state.jsonErrors[k]; t.classList.remove('baderr'); }
    else {
      try { c.opts[k] = JSON.parse(raw); delete state.jsonErrors[k]; t.classList.remove('baderr'); }
      catch (err) { state.jsonErrors[k] = 'Invalid JSON: ' + err.message; t.classList.add('baderr'); }
    }
  } else if (t.dataset.items !== undefined) {
    c.opts.items = linesToItems(t.value);
  } else if (t.dataset.dton !== undefined) {
    if (t.checked) { c.opts[k] = { mode: 'datetime' }; renderEditor(); }
    else delete c.opts[k];
  } else if (t.dataset.dtm !== undefined) {
    const cur = c.opts[k] && typeof c.opts[k] === 'object' ? c.opts[k] : { mode: 'datetime' };
    cur.mode = t.value; c.opts[k] = cur;
  } else if (t.dataset.dtf !== undefined) {
    const cur = c.opts[k] && typeof c.opts[k] === 'object' ? c.opts[k] : { mode: 'datetime' };
    if (t.value) cur.format = t.value; else delete cur.format;
    c.opts[k] = cur;
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
    const el = e.target.closest('[data-k-ctrl]');
    if (!el) return;
    const name = el.getAttribute('data-k-ctrl');
    const i = form().controls.findIndex(c => c.name === name);
    if (i >= 0) selectControl(i);
  });
}

function wireTopbar() {
  $('#btn-new').addEventListener('click', async () => {
    state.doc = { version: 1, form: { name: 'main', layout: 'vertical', align: 'left', controls: [] } };
    state.selected = -1;
    state.dirty = true;
    render();
    setStatus('New empty form');
  });
  $('#btn-save').addEventListener('click', save);
  $('#btn-export').addEventListener('click', exportLua);
  $('#btn-validate').addEventListener('click', validate);
  document.addEventListener('keydown', e => {
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 's') { e.preventDefault(); save(); }
  });
  $('#modal-close').addEventListener('click', () => $('#overlay').classList.add('hidden'));
  $('#overlay').addEventListener('click', e => {
    if (e.target.id === 'overlay') $('#overlay').classList.add('hidden');
  });
}

/* ---------- utilities ---------- */
function setStatus(msg, isErr) {
  const el = $('#status');
  el.textContent = msg;
  el.classList.toggle('err', !!isErr);
  const notes = state.doc && state.doc.form && state.doc.form.notes;
  $('#notes').textContent = notes && notes.length ? '⚠ ' + notes.join(' · ') : '';
}

function showModal(title, text) {
  $('#modal-title').textContent = title;
  $('#modal-body').textContent = text;
  $('#overlay').classList.remove('hidden');
}

function escapeHtml(s) {
  return String(s).replace(/[&<>"']/g, c => (
    { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
}
function escapeAttr(s) { return escapeHtml(s); }

/* ---------- init ---------- */
wirePalette();
wireControlList();
wireFormProps();
wireEditor();
wirePreview();
wireTopbar();
loadInitial().catch(e => {
  $('#preview').innerHTML = '<div class="error">Failed to load: ' + escapeHtml(e.message) + '</div>';
  setStatus(e.message, true);
});