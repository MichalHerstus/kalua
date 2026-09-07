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
  undoStack: [], redoStack: [], lastSnapAt: 0,
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
  f.cells = normalizeCells(f.cells);
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
    const wrap = $('#preview');
    wrap.innerHTML = r.html;
    wrap.dataset.layout = (form().layout || 'vertical');
    if (state.selected >= 0) flashSelected(form().controls[state.selected]?.name);
    flashCell();
  } catch (e) {
    $('#preview').innerHTML = '<div class="error">' + escapeHtml(e.message) + '</div>';
  }
}

function flashCell() {
  $$('#preview .kalua-cell').forEach(el => {
    el.classList.toggle('selected-cell', currentCell && el.getAttribute('data-k-cell') === currentCell);
  });
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
    const del = $('#ctrl-del');
    del.hidden = true;
    editor.innerHTML = '<span class="hint">Select a control in the preview or the list.</span>';
    return;
  }
  const c = ctrl();
  head.textContent = c.name;
  $('#ctrl-type').textContent = TYPE_NAMES[c.type] || c.type;
  const del = $('#ctrl-del');
  del.hidden = false;
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
    setStatus('Cell id already exists.', true);
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
    if (!currentCell) { setStatus('Select a cell first', true); return; }
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
  $('#btn-new').addEventListener('click', async () => {
    resetUndo();
    state.doc = { version: 2, form: { name: 'main', layout: 'vertical', align: 'left', controls: [], cells: [] } };
    state.selected = -1;
    currentCell = '';
    state.dirty = true;
    render();
    setStatus('New empty form');
  });
  $('#btn-save').addEventListener('click', save);
  $('#btn-export').addEventListener('click', exportLua);
  $('#btn-validate').addEventListener('click', validate);
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

/* ---------- init ---------- */
wirePalette();
wireControlList();
wireFormProps();
wireEditor();
wirePreview();
wireTopbar();
wireTheme();
loadInitial().catch(e => {
  $('#preview').innerHTML = '<div class="error">Failed to load: ' + escapeHtml(e.message) + '</div>';
  setStatus(e.message, true);
});