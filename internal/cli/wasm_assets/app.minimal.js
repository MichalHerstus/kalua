/**
 * KALUA WASM "hands" client (M5).
 *
 * The page's JavaScript is deliberately dumb. The Go "brain" (internal/wasm/
 * brain.go + common.RouteOutbox) owns message routing, protocol construction,
 * UI state and component-lifecycle decisions; this file only:
 *   1. executes compact "hands" commands (stage / modal_open / update_control /
 *      component / clipboard / pick_file / ...) on the DOM, and
 *   2. reports browser events and browser-API answers back over a tiny bridge.
 *
 * No WebSocket. No transport selection. No ~30-case message switch.
 */

(function() {
    'use strict';

    // ---- bridge: Go <-> JS ------------------------------------------------

    // response emits a protocol response the WASM session understands (same
    // inbox types as the native client: msgbox_choice, file_picker_resp, ...).
    function response(msg) {
        if (window.kaluaPostMessage) {
            window.kaluaPostMessage(JSON.stringify(msg));
        }
    }

    // reportDOMEvent sends a DOM event with its current value to the brain.
    function reportDOMEvent(form, ctrl, event, value) {
        if (window.kaluaOnDOMEvent) {
            window.kaluaOnDOMEvent(form, ctrl, event, JSON.stringify(value));
        }
    }

    // ---- DOM roots --------------------------------------------------------

    const stage = document.getElementById('stage');
    const modals = document.getElementById('modals');
    const statusBar = document.getElementById('status-bar');

    function el(selector) { return document.querySelector(selector); }

    // ---- browser APIs -----------------------------------------------------

    function setClipboard(text) {
        if (navigator.clipboard && navigator.clipboard.writeText) {
            navigator.clipboard.writeText(text).catch(function(){});
        }
    }

    function getClipboard(id) {
        if (!navigator.clipboard || !navigator.clipboard.readText) {
            response({type: 'clipboard_resp', id: id, choice: ''});
            return;
        }
        navigator.clipboard.readText()
            .then(function(text) { response({type: 'clipboard_resp', id: id, choice: text || ''}); })
            .catch(function() { response({type: 'clipboard_resp', id: id, choice: ''}); });
    }

    function pickFile(id, accept, multiple) {
        var input = document.createElement('input');
        input.type = 'file';
        input.style.display = 'none';
        if (accept) input.accept = accept;
        if (multiple) input.multiple = true;

        input.addEventListener('change', function() {
            var files = input.files;
            if (!files || files.length === 0) {
                response({type: 'file_picker_resp', id: id, choice: '[]'});
                document.body.removeChild(input);
                return;
            }
            var results = [];
            var remaining = files.length;
            for (var i = 0; i < files.length; i++) {
                (function(file) {
                    var reader = new FileReader();
                    reader.onload = function(e) {
                        var base64 = e.target.result.split(',')[1] || '';
                        results.push({name: file.name, size: file.size, type: file.type, data: base64});
                        remaining--;
                        if (remaining === 0) {
                            results.sort(function(a, b) { return a.name.localeCompare(b.name); });
                            response({type: 'file_picker_resp', id: id, choice: JSON.stringify(results)});
                            document.body.removeChild(input);
                        }
                    };
                    reader.onerror = function() {
                        remaining--;
                        if (remaining === 0) {
                            response({type: 'file_picker_resp', id: id, choice: JSON.stringify(results)});
                            document.body.removeChild(input);
                        }
                    };
                    reader.readAsDataURL(file);
                })(files[i]);
            }
        });
        input.addEventListener('cancel', function() {
            response({type: 'file_picker_resp', id: id, choice: '[]'});
            document.body.removeChild(input);
        });
        document.body.appendChild(input);
        input.click();
    }

    function pickFileSave(id, data) {
        var mode = data.mode || 'save';
        var filename = data.filename || 'file';
        var fileData = data.data || '';

        if (mode === 'download') {
            var link = document.createElement('a');
            link.href = 'data:application/octet-stream;base64,' + fileData;
            link.download = filename;
            link.style.display = 'none';
            document.body.appendChild(link);
            link.click();
            document.body.removeChild(link);
            response({type: 'file_picker_save_resp', id: id, choice: JSON.stringify({path: filename, name: filename})});
            return;
        }
        var input = document.createElement('input');
        input.type = 'file';
        input.style.display = 'none';
        input.accept = '*/*';
        input.addEventListener('change', function() {
            var file = input.files[0];
            if (!file) {
                response({type: 'file_picker_save_resp', id: id, choice: JSON.stringify({path: '', name: ''})});
            } else {
                response({type: 'file_picker_save_resp', id: id, choice: JSON.stringify({path: file.name, name: file.name})});
            }
            document.body.removeChild(input);
        });
        input.addEventListener('cancel', function() {
            response({type: 'file_picker_save_resp', id: id, choice: JSON.stringify({path: '', name: ''})});
            document.body.removeChild(input);
        });
        document.body.appendChild(input);
        input.click();
    }

    // Msgbox / popup
    function showMsgbox(id, kind, html) {
        const overlay = document.createElement('div');
        overlay.id = 'mb:' + id;
        overlay.className = 'msgbox-overlay';
        overlay.innerHTML = `<div class="msgbox msgbox-${kind}" role="dialog" aria-modal="true">${html}</div>`;
        modals.appendChild(overlay);
        const firstButton = overlay.querySelector('button');
        if (firstButton) firstButton.focus();
        overlay.addEventListener('keydown', function(e) {
            if (e.key === 'Tab') trapFocus(e, overlay);
        });
    }

    function closeMsgbox(id) {
        const overlay = document.getElementById('mb:' + id);
        if (overlay) overlay.remove();
    }

    function showPopup(id, html) {
        const overlay = document.createElement('div');
        overlay.id = 'pop:' + id;
        overlay.className = 'popup-overlay';
        overlay.innerHTML = `<div class="popup" role="menu" aria-label="menu">${html}</div>`;
        modals.appendChild(overlay);
        const firstItem = overlay.querySelector('.popup-item');
        if (firstItem) firstItem.focus();
        overlay.addEventListener('keydown', function(e) {
            if (e.key === 'Escape') {
                e.preventDefault();
                response({type: 'popup_dismiss', id: id});
                closePopup(id);
            } else if (e.key === 'Tab') {
                trapFocus(e, overlay);
            }
        });
        overlay.addEventListener('mousedown', function(e) {
            if (e.target === overlay) {
                response({type: 'popup_dismiss', id: id});
                closePopup(id);
            }
        });
    }

    function closePopup(id) {
        const overlay = document.getElementById('pop:' + id);
        if (overlay) overlay.remove();
    }

    function togglePopupSubmenu(item) {
        const submenu = item.querySelector('.kalua-popup-submenu');
        if (submenu) item.classList.toggle('open');
    }

    function trapFocus(e, container) {
        const focusable = container.querySelectorAll(
            'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])');
        const first = focusable[0];
        const last = focusable[focusable.length - 1];
        if (e.shiftKey && document.activeElement === first) {
            e.preventDefault(); last.focus();
        } else if (!e.shiftKey && document.activeElement === last) {
            e.preventDefault(); first.focus();
        }
    }

    function showStatus(text) {
        statusBar.textContent = text;
        statusBar.classList.remove('hidden');
    }

    function hideStatus() {
        statusBar.classList.add('hidden');
        statusBar.textContent = '';
    }

    function showError(msg, stack) {
        const banner = document.createElement('div');
        banner.className = 'error-banner';
        banner.innerHTML = `<strong>Error:</strong> ${escapeHtml(msg)}`;
        if (stack) banner.innerHTML += `<pre>${escapeHtml(stack)}</pre>`;
        document.body.appendChild(banner);
        setTimeout(function() { banner.remove(); }, 10000);
    }

    function handleQuit() {
        stage.innerHTML = '<div class="quit-message">Application ended.</div>';
        modals.innerHTML = '';
        statusBar.classList.add('hidden');
    }

    function playBell() {
        if (typeof Audio === 'undefined') return;
        try {
            const ctx = new Audio.AudioContext;
            const osc = ctx.createOscillator();
            const gain = ctx.createGainNode();
            gain.setGain(0.15);
            osc.connect(gain.connect(ctx.tone));
            osc.frequency.setValue(880);
            osc.envelope.setTarget(0.001);
            osc.envelope.setTimeConstant(0.3);
            osc.start();
            setTimeout(function() { osc.stop(); }, 300);
        } catch (e) {}
    }

    // ---- Component lifecycle ----------------------------------------------

    // All live component instances keyed by selector ("#c:form:ctrl").
    const tabulatorInstances = new Map();
    const gridInstances = new Map();
    const chartInstances = new Map();
    const datePickerInstances = new Map();
    // Pending Tabulator remote-pagination resolvers (selector -> {resolve, reject, timer}).
    const ajaxResolvers = new Map();

    function scanComponents(scope) {
        if (!scope) return;
        initGrids(scope);
        initTabulators(scope);
        initLoopers(scope);
        initCharts(scope);
        initDatePickers(scope);
    }

    function destroyComponents(scope) {
        if (!scope) return;
        destroyTabulators(scope);
        destroyCharts(scope);
        destroyDatePickers(scope);
    }

    // ---- Tabulator (k.ctrl.table) -----------------------------------------

    function initTabulators(scope) {
        const els = scope.querySelectorAll('.kalua-tabulator-table:not([data-k-tabulator-ready])');
        if (typeof Tabulator === 'undefined') {
            els.forEach(function(el) { renderFallbackTable(el); });
            return;
        }
        els.forEach(function(el) { createTabulator(el); });
    }

    function createTabulator(el, seed) {
        seed = seed || {};
        var opts = {};
        try { opts = JSON.parse(el.dataset.kTabulatorOptions || '{}'); } catch (e) {}
        var cols = seed.columns || [];
        if (!cols || cols.length === 0) {
            try { cols = JSON.parse(el.dataset.kTabulatorColumns || '[]'); } catch (e) {}
        }
        var data = [];
        try { data = JSON.parse(el.dataset.kTabulatorData || '[]'); } catch (e) {}

        var remoteMode = opts.paginationMode === 'remote' || opts.pagination === 'remote';
        if (!cols || cols.length === 0) cols = inferColumns(data);
        opts.columns = cols;
        if (cols.length === 0) opts.autoColumns = true;
        opts.layout = opts.layout || 'fitColumns';
        if (seed.selectable !== undefined) {
            opts.selectable = seed.selectable;
        } else {
            opts.selectable = opts.selectable !== false;
        }
        if (seed.selectableRows !== undefined) opts.selectableRows = seed.selectableRows;
        opts.selectableRangeMode = opts.selectableRangeMode || 'click';

        var form = el.dataset.kForm;
        var ctrl = el.dataset.kCtrl;

        if (remoteMode && (!data || data.length === 0)) {
            delete opts.data;
        } else {
            opts.data = data || [];
        }

        opts.rowSelectionChanged = seed.rowSelectionChanged || function(selectedData, selectedRows) {
            var rows = [];
            if (selectedRows) {
                selectedRows.forEach(function(r) {
                    var idx = r ? r.getPosition(true) : 0;
                    rows.push(idx + 1);
                });
            }
            reportDOMEvent(form, ctrl, 'tabulator_selection_change', { rows: rows, data: selectedData || [] });
        };
        if (seed.rowClick) opts.rowClick = seed.rowClick;

        if (remoteMode) {
            opts.ajaxURL = opts.ajaxURL || '#kalua-ws';
            opts.sortMode = 'remote';
            opts.filterMode = 'remote';
            if (typeof opts.ajaxRequestFunc !== 'function') {
                opts.ajaxRequestFunc = function(url, config, params) {
                    return new Promise(function(resolve, reject) {
                        params = params || {};
                        var q = {
                            page: params.page || 1,
                            size: params.size || opts.paginationSize || 10,
                            sort: params.sorters || params.sort || [],
                            filter: params.filters || params.filter || []
                        };
                        q.sort = (q.sort || []).map(function(s) { return { field: s.field, dir: s.dir || 'asc' }; });
                        q.filter = (q.filter || []).map(function(f) { return { field: f.field, type: f.type, value: f.value }; });
                        var selector = '#c:' + form + ':' + ctrl;
                        var timer = setTimeout(function() {
                            ajaxResolvers.delete(selector);
                            reject(new Error('tabulator_ajax_request timeout'));
                        }, 15000);
                        ajaxResolvers.set(selector, { resolve: resolve, reject: reject, timer: timer });
                        response({ type: 'tabulator_ajax_request', form: form, ctrl: ctrl, value: q });
                    });
                };
            }
        }

        var inst = new Tabulator(el, opts);
        tabulatorInstances.set('#' + el.id, inst);
        el.setAttribute('data-k-tabulator-ready', 'true');
        return inst;
    }

    function inferColumns(data) {
        var cols = [];
        if (!data || data.length === 0) return cols;
        var first = data[0];
        Object.keys(first).forEach(function(key) {
            var val = first[key];
            var col = { field: key, title: key };
            if (typeof val === 'number') {
                col.sorter = 'number'; col.editor = 'number'; col.hozAlign = 'right';
            } else if (typeof val === 'boolean') {
                col.formatter = 'tickCross'; col.editor = 'tickCross'; col.hozAlign = 'center';
            } else {
                col.sorter = 'string'; col.editor = 'input';
            }
            cols.push(col);
        });
        return cols;
    }

    function renderFallbackTable(el) {
        var cols = [];
        try { cols = JSON.parse(el.dataset.kTabulatorColumns || '[]'); } catch (e) {}
        var data = [];
        try { data = JSON.parse(el.dataset.kTabulatorData || '[]'); } catch (e) {}
        if (!cols || cols.length === 0) cols = inferColumns(data);
        if (cols.length === 0 && (!data || data.length === 0)) {
            el.innerHTML = '<div class="kalua-tabulator-empty">No data</div>';
            el.setAttribute('data-k-tabulator-ready', 'true');
            return;
        }
        var table = document.createElement('table');
        table.className = 'kalua-table';
        var thead = document.createElement('thead');
        var headRow = document.createElement('tr');
        cols.forEach(function(col) {
            var th = document.createElement('th');
            th.innerHTML = escapeHtml(col.title !== undefined ? col.title : col.field);
            headRow.appendChild(th);
        });
        thead.appendChild(headRow);
        table.appendChild(thead);
        var tbody = document.createElement('tbody');
        (data || []).forEach(function(row) {
            var tr = document.createElement('tr');
            cols.forEach(function(col) {
                var td = document.createElement('td');
                var v = row[col.field];
                if (typeof v === 'boolean') v = v ? '\u2713' : '';
                td.innerHTML = escapeHtml(v === null || v === undefined ? '' : String(v));
                tr.appendChild(td);
            });
            tbody.appendChild(tr);
        });
        table.appendChild(tbody);
        el.innerHTML = '';
        el.appendChild(table);
        el.setAttribute('data-k-tabulator-ready', 'true');
    }

    function destroyTabulators(scope) {
        const els = scope.querySelectorAll('.kalua-tabulator-table');
        els.forEach(function(el) {
            const key = '#' + el.id;
            const inst = tabulatorInstances.get(key);
            if (inst) { inst.destroy(); tabulatorInstances.delete(key); }
            el.removeAttribute('data-k-tabulator-ready');
            gridInstances.delete(key);
        });
    }

    function tabulatorUpdate(selector, dataJSON) {
        const inst = tabulatorInstances.get(selector);
        if (!inst) return;
        try {
            var data = JSON.parse(dataJSON || '[]');
            if (Array.isArray(data)) {
                inst.setData(data);
            } else {
                inst.setData(data.data || []);
                applyMaxPage(selector, data.last_page || data.last_row || 0, data.last_row || 0);
            }
        } catch (e) {}
    }

    function tabulatorRemoteData(msg) {
        var selector = msg.selector || ('#c:' + msg.form + ':' + msg.ctrl);
        var payload = {};
        try { payload = JSON.parse(msg.data || '{}'); } catch (e) {}
        var data = payload.data || [];
        var lastPage = payload.last_page || 0;
        var lastRow = payload.last_row || 0;
        var slot = ajaxResolvers.get(selector);
        if (slot) {
            clearTimeout(slot.timer);
            ajaxResolvers.delete(selector);
            slot.resolve(data);
            applyMaxPage(selector, lastPage, lastRow);
        } else {
            const inst = tabulatorInstances.get(selector);
            if (inst) {
                inst.setData(data);
                applyMaxPage(selector, lastPage, lastRow);
            }
        }
    }

    function applyMaxPage(selector, lastPage, lastRow) {
        const inst = tabulatorInstances.get(selector);
        if (!inst) return;
        if (lastPage > 0 && typeof inst.setMaxPage === 'function') inst.setMaxPage(lastPage);
        if (lastRow > 0 && typeof inst.setRowCount === 'function') inst.setRowCount(lastRow);
    }

    function tabulatorRefresh(selector) {
        const inst = tabulatorInstances.get(selector);
        if (!inst) return;
        inst.setSort(false);
        inst.setFilter(false);
        inst.setPage(1);
    }

    function tabulatorGetData(id, selector, form, ctrl) {
        const inst = tabulatorInstances.get(selector);
        let data = [];
        if (inst) { try { data = inst.getData(); } catch (e) {} }
        response({ type: 'tabulator_data_resp', id: id, choice: JSON.stringify(data) });
    }

    function tabulatorGetSelection(id, selector) {
        const inst = tabulatorInstances.get(selector);
        const rows = [];
        if (inst && typeof inst.getSelectedRows === 'function') {
            inst.getSelectedRows().forEach(function(r) {
                rows.push((r.getPosition(true)) + 1);
            });
        }
        response({ type: 'tabulator_selection_resp', id: id, select_rows: rows });
    }

    function tabulatorDestroy(selector) {
        const inst = tabulatorInstances.get(selector);
        if (inst) { inst.destroy(); tabulatorInstances.delete(selector); }
        gridInstances.delete(selector);
    }

    // ---- CRUD Grid (k.ctrl.grid) ------------------------------------------

    function initGrids(scope) {
        scope.querySelectorAll('.kalua-grid:not([data-k-grid-ready])').forEach(function(wrapper) {
            createGrid(wrapper);
            wrapper.setAttribute('data-k-grid-ready', 'true');
        });
    }

    function gridCfgOf(wrapper) {
        const tableEl = wrapper.querySelector('.kalua-tabulator-table');
        if (!tableEl) return null;
        return gridInstances.get('#' + tableEl.id) || null;
    }

    function createGrid(wrapper) {
        const tableEl = wrapper.querySelector('.kalua-tabulator-table');
        if (!tableEl) return;

        let rowActions = {};
        try { rowActions = JSON.parse(wrapper.dataset.kGridRowActions || '{}'); } catch (e) {}
        let globalActions = {};
        try { globalActions = JSON.parse(wrapper.dataset.kGridGlobalActions || '{}'); } catch (e) {}

        const cfg = {
            wrapper: wrapper,
            tableEl: tableEl,
            pkField: wrapper.dataset.kGridPk || '',
            rowActions: rowActions,
            globalActions: globalActions,
            rowClick: wrapper.dataset.kGridRowClick || '',
            columnVisibility: wrapper.dataset.kGridColumnVisibility === 'true',
            selectionMode: wrapper.dataset.kGridSelection || 'multi',
            inst: null
        };

        var cols = [];
        try { cols = JSON.parse(tableEl.dataset.kTabulatorColumns || '[]'); } catch (e) {}
        var data = [];
        try { data = JSON.parse(tableEl.dataset.kTabulatorData || '[]'); } catch (e) {}
        if (!cols || cols.length === 0) cols = inferColumns(data);

        const hasActions = !!(rowActions && (rowActions.view || rowActions.edit || rowActions.delete));
        if (hasActions && cfg.pkField && cols.length > 0) {
            cols = cols.slice();
            cols.push(gridActionsColumn(cfg));
        }

        const selection = cfg.selectionMode || 'multi';
        const seed = { columns: cols };
        if (selection === 'none') {
            seed.selectable = false;
        } else if (selection === 'single') {
            seed.selectable = true;
            seed.selectableRows = 1;
        } else if (selection !== 'multi' && Number(selection) > 0) {
            seed.selectable = true;
            seed.selectableRows = Number(selection);
        }

        if (cfg.rowClick === 'select') {
            seed.rowClick = function(e, row) {
                if (e && e.target && e.target.closest && e.target.closest('button')) return;
                if (row && typeof row.select === 'function') {
                    if (row.isSelected()) row.deselect(); else row.select();
                }
            };
        } else if (cfg.rowClick === 'view' || cfg.rowClick === 'edit') {
            seed.rowClick = function(e, row) {
                if (e && e.target && e.target.closest && e.target.closest('button')) return;
                if (!cfg.pkField || !row) return;
                const d = row.getData();
                if (!d || d[cfg.pkField] === undefined) return;
                gridOpenForm(cfg, cfg.rowClick, d[cfg.pkField], d);
            };
        }

        if (hasActions || cfg.columnVisibility) {
            seed.rowSelectionChanged = function() { updateGridToolbar(cfg); };
        }

        const inst = createTabulator(tableEl, seed);
        cfg.inst = inst;
        gridInstances.set('#' + tableEl.id, cfg);
        return cfg;
    }

    function gridActionsColumn(cfg) {
        return {
            field: '_actions', title: '', width: 120, hozAlign: 'center',
            headerSort: false, resizable: false, frozen: true,
            formatter: function(cell) {
                const d = cell.getData() || {};
                const pkv = d[cfg.pkField] === undefined || d[cfg.pkField] === null ? '' : JSON.stringify(d[cfg.pkField]);
                const rowv = escapeHtml(JSON.stringify(d));
                let html = '<div class="kalua-grid-row-actions">';
                if (cfg.rowActions.view) html += gridActionButton('view', 'View', pkv, rowv);
                if (cfg.rowActions.edit) html += gridActionButton('edit', 'Edit', pkv, rowv);
                if (cfg.rowActions.delete) html += gridActionButton('delete', 'Delete', pkv, rowv);
                html += '</div>';
                return html;
            }
        };
    }

    function gridActionButton(action, label, pkv, rowv) {
        return '<button type="button" class="kalua-grid-btn kalua-grid-btn-' + action + '"'
            + ' data-k-grid-action="' + action + '"'
            + ' data-k-grid-pk="' + escapeHtml(pkv) + '"'
            + ' data-k-grid-row="' + escapeHtml(rowv) + '">' + label + '</button>';
    }

    function gridOpenForm(cfg, mode, pk, row) {
        response({
            type: 'grid_form_open',
            form: cfg.wrapper.dataset.kForm,
            ctrl: cfg.wrapper.dataset.kCtrl,
            value: { mode: mode, pk: pk, row: row || {} }
        });
    }

    function updateGridToolbar(cfg) {
        const btn = cfg.wrapper.querySelector('[data-k-grid-toolbar="batch_delete"]');
        if (!btn) return;
        const inst = cfg.inst;
        const n = inst && typeof inst.getSelectedRows === 'function' ? inst.getSelectedRows().length : 0;
        btn.disabled = n === 0;
        btn.classList.toggle('disabled', n === 0);
    }

    function collectSelectedGridPks(cfg) {
        const inst = cfg.inst;
        if (!inst || typeof inst.getSelectedRows !== 'function' || !cfg.pkField) return [];
        return inst.getSelectedRows().map(function(row) {
            const d = row.getData();
            return d ? d[cfg.pkField] : undefined;
        }).filter(function(v) { return v !== undefined && v !== null; });
    }

    function gridFormValues(formEl) {
        const values = {};
        if (!formEl) return values;
        formEl.querySelectorAll('[data-k-form][data-k-ctrl]').forEach(function(el) {
            values[el.dataset.kCtrl] = getControlValue(el);
        });
        return values;
    }

    function setupGridModal(msg) {
        const overlay = modalForms.get(msg.name);
        if (!overlay) return;
        const formEl = overlay.querySelector('#f:' + msg.name);
        let row = {};
        try { row = JSON.parse(msg.grid_row || '{}'); } catch (e) {}
        const mode = msg.grid_mode || 'edit';
        const controls = formEl ? formEl.querySelectorAll('[data-k-form][data-k-ctrl]') : [];
        controls.forEach(function(el) {
            if (el.tagName === 'IMG') {
                if (row[el.dataset.kCtrl]) el.setAttribute('src', String(row[el.dataset.kCtrl]));
                return;
            }
            let value = row[el.dataset.kCtrl];
            if (el.tagName === 'INPUT' && el.type === 'checkbox') {
                el.checked = !!value;
            } else if (el.tagName === 'INPUT' && el.type === 'radio') {
                el.checked = String(value) === el.value;
            } else {
                el.value = value === null || value === undefined ? '' : String(value);
            }
        });
        if (mode === 'view') {
            controls.forEach(function(el) {
                if (el.disabled !== undefined) el.disabled = true;
                if (el.readOnly !== undefined) el.readOnly = true;
            });
            const saveBtn = overlay.querySelector('[data-k-grid-save]');
            if (saveBtn) saveBtn.remove();
        }
        const footer = overlay.querySelector('[data-k-grid-width]');
        if (footer) {
            const width = footer.dataset.kGridWidth;
            const modal = overlay.querySelector('.form-modal');
            if (modal) modal.style.maxWidth = width;
        }
    }

    // ---- Chart.js (k.ctrl.chart) ------------------------------------------

    function initCharts(scope) {
        if (typeof Chart === 'undefined') return;
        const canvases = scope.querySelectorAll('.kalua-chart-canvas:not([data-k-chart-ready])');
        canvases.forEach(function(canvas) {
            const form = canvas.dataset.kForm;
            const ctrl = canvas.dataset.kCtrl;
            const key = '#c:' + form + ':' + ctrl;
            if (chartInstances.has(key)) return;
            let config = null;
            try { config = JSON.parse(canvas.getAttribute('data-k-chart-config') || '{}'); } catch (e) { return; }
            if (!config.options) config.options = {};
            const makePointHandler = function(eventName) {
                return function(event, elements, chart) {
                    if (!elements || !elements.length) return;
                    const el = elements[0];
                    reportDOMEvent(form, ctrl, eventName, {
                        dataset_index: el.datasetIndex + 1,
                        index: el.index + 1,
                        value: chartElementValue(chart, el)
                    });
                };
            };
            config.options.onClick = makePointHandler('chart_click');
            config.options.onHover = makePointHandler('chart_hover');
            config.options.plugins = config.options.plugins || {};
            config.options.plugins.legend = config.options.plugins.legend || {};
            if (!config.options.plugins.legend.onClick) {
                config.options.plugins.legend.onClick = function(e, legendItem, legend) {
                    const chart = legend.chart;
                    const di = legendItem.datasetIndex;
                    reportDOMEvent(form, ctrl, 'chart_legend_click', { dataset_index: di + 1 });
                    const meta = chart.getDatasetMeta(di);
                    meta.hidden = meta.hidden === null ? !chart.data.datasets[di].hidden : null;
                    chart.update();
                };
            }
            const chart = new Chart(canvas.getContext('2d'), config);
            chartInstances.set(key, chart);
            canvas.setAttribute('data-k-chart-ready', 'true');
        });
    }

    function chartElementValue(chart, el) {
        const ds = chart.data.datasets[el.datasetIndex];
        if (!ds) return null;
        const v = ds.data[el.index];
        if (v && typeof v === 'object' && !Array.isArray(v)) return v.x !== undefined ? v.x : null;
        return v === undefined ? null : v;
    }

    function destroyCharts(scope) {
        if (typeof Chart === 'undefined') return;
        const canvases = scope.querySelectorAll('.kalua-chart-canvas[data-k-chart-ready]');
        canvases.forEach(function(canvas) {
            const key = '#c:' + (canvas.dataset.kForm || '') + ':' + (canvas.dataset.kCtrl || '');
            const inst = chartInstances.get(key);
            if (inst) { inst.destroy(); chartInstances.delete(key); }
            canvas.removeAttribute('data-k-chart-ready');
        });
    }

    function chartUpdate(selector, dataJson) {
        const chart = chartInstances.get(selector);
        if (!chart) return;
        try { chart.data = JSON.parse(dataJson); chart.update('none'); } catch (e) {}
    }

    function chartOptions(selector, optionsJson) {
        const chart = chartInstances.get(selector);
        if (!chart) return;
        try { mergeChartOptions(chart.options, JSON.parse(optionsJson)); chart.update(); } catch (e) {}
    }

    function mergeChartOptions(target, source) {
        if (!source) return target;
        Object.keys(source).forEach(function(key) {
            const sv = source[key];
            const tv = target[key];
            if (tv && typeof tv === 'object' && sv && typeof sv === 'object' &&
                !Array.isArray(tv) && !Array.isArray(sv)) {
                mergeChartOptions(tv, sv);
            } else {
                target[key] = sv;
            }
        });
        return target;
    }

    function chartResize(selector, sizeJson) {
        const el = document.querySelector(selector);
        if (el && sizeJson) {
            const size = {};
            try { Object.assign(size, JSON.parse(sizeJson)); } catch (e) {}
            if (size.width) el.style.width = size.width + 'px';
            if (size.height) el.style.height = size.height + 'px';
        }
        const chart = chartInstances.get(selector);
        if (chart) chart.resize();
    }

    function chartGetImage(id, selector) {
        const chart = chartInstances.get(selector);
        let value = '';
        if (chart) { try { value = chart.toBase64Image('image/png'); } catch (e) {} }
        response({ type: 'chart_image_resp', id: id, choice: value });
    }

    // ---- Looper (k.ctrl.looper) -------------------------------------------

    function initLoopers(scope) {
        scope.querySelectorAll('.kalua-looper:not([data-k-looper-ready])').forEach(function(el) {
            el.setAttribute('data-k-looper-ready', 'true');
            el.setAttribute('data-k-looper-next', '0');
            const rowsEl = el.querySelector('.kalua-looper-rows');
            if (rowsEl) rowsEl.innerHTML = '';
            if (el.dataset.kLooperHtml !== undefined) {
                // Row-template loopers: server-rendered {index, html} batches.
                scheduleLooperFetch(el);
            } else {
                fetchLooperPage(el, 0);
            }
        });
    }

    function looperCellValue(v) {
        if (v === null || v === undefined) return '';
        return String(v);
    }

    function handleLooperDBBatch(msg) {
        var wrapper = el(msg.selector || ('#c:' + msg.form + ':' + msg.ctrl));
        if (!wrapper) return;
        var payload = {};
        try { payload = JSON.parse(msg.data || '{}'); } catch (e) {}
        var rows = payload.rows || payload.data || [];
        var rowsEl = wrapper.querySelector('.kalua-looper-rows');
        if (!rowsEl) return;
        var htmlMode = wrapper.dataset.kLooperHtml !== undefined;

        rows.forEach(function(row) {
            if (htmlMode) {
                // {index, html} pair from the server renderer.
                if (row.html) rowsEl.innerHTML += row.html;
            } else {
                var line = document.createElement('div');
                line.className = 'kalua-looper-row';
                var data = row.data || row;
                Object.keys(data).forEach(function(ctrlName) {
                    var cell = document.createElement('span');
                    cell.className = 'kalua-looper-cell-value';
                    cell.dataset.kLooperControl = ctrlName;
                    cell.textContent = looperCellValue(data[ctrlName]);
                    line.appendChild(cell);
                });
                rowsEl.appendChild(line);
            }
        });
        wrapper.setAttribute('data-k-looper-next', String(rows.length));
        scheduleLooperFetch(wrapper);
    }

    function looperRefresh(selector) {
        const wrapper = el(selector);
        if (!wrapper) return;
        const rowsEl = wrapper.querySelector('.kalua-looper-rows');
        if (rowsEl) rowsEl.innerHTML = '';
        wrapper.setAttribute('data-k-looper-next', '0');
        if (wrapper.dataset.kLooperHtml !== undefined) {
            scheduleLooperFetch(wrapper);
        } else {
            fetchLooperPage(wrapper, 0);
        }
    }

    function scheduleLooperFetch(wrapper) {
        const sentinel = wrapper.querySelector('.kalua-looper-sentinel');
        if (!sentinel) return;
        if (typeof IntersectionObserver === 'undefined') return;
        if (!wrapper._looperObs) {
            wrapper._looperObs = new IntersectionObserver(function(entries) {
                entries.forEach(function(entry) {
                    if (entry.isIntersecting) fetchLooperPage(wrapper, Number(wrapper.dataset.kLooperNext || '0'));
                });
            }, { root: wrapper, rootMargin: '200px' });
        }
        wrapper._looperObs.observe(sentinel);
    }

    function fetchLooperPage(wrapper, startIdx) {
        const form = wrapper.dataset.kForm;
        const ctrl = wrapper.dataset.kCtrl;
        const size = Number(wrapper.dataset.kLooperPageSize || '50');
        const requested = Number(wrapper.dataset.kLooperNext || '0');
        if (startIdx < requested) return;
        response({
            type: 'looper_scroll_request',
            form: form,
            ctrl: ctrl,
            value: { start_idx: requested + 1, count: size }
        });
    }

    function selectLooperRow(el, rowEl, target) {
        const form = el.dataset.kForm;
        const ctrl = el.dataset.kCtrl;
        let lineIdx = 0;
        if (rowEl && rowEl.dataset && rowEl.dataset.kLineIndex) lineIdx = Number(rowEl.dataset.kLineIndex);
        if (rowEl) {
            el.querySelectorAll('.kalua-looper-row').forEach(function(r) { r.classList.remove('selected'); });
            rowEl.classList.add('selected');
        }
        const ctrlName = target && target.dataset ? target.dataset.kLooperControl : '';
        reportDOMEvent(form, ctrl, 'looper_click', { line_idx: lineIdx, ctrl_name: ctrlName });
    }

    // ---- flatpickr (textbox datetime) -------------------------------------

    function initDatePickers(scope) {
        scope.querySelectorAll('input.kalua-datetime[data-k-form][data-k-ctrl]').forEach(function(el) {
            const selector = '#' + el.id;
            if (datePickerInstances.has(selector)) return;
            if (typeof flatpickr === 'undefined') return;
            const options = { allowInput: true };
            if (el.dataset.kDatetimeOptions) {
                try { Object.assign(options, JSON.parse(el.dataset.kDatetimeOptions)); } catch (e) {}
            }
            let fp = null;
            try { fp = flatpickr(el, options); } catch (e) {}
            if (fp) datePickerInstances.set(selector, fp);
        });
    }

    function destroyDatePickers(scope) {
        scope.querySelectorAll('input.kalua-datetime[data-k-form][data-k-ctrl]').forEach(function(el) {
            const fp = datePickerInstances.get('#' + el.id);
            if (fp) {
                try { fp.destroy(); } catch (e) {}
                datePickerInstances.delete('#' + el.id);
            }
        });
    }

    // ---- Form/control DOM ops (the "hands") -------------------------------

    const modalForms = new Map(); // form name -> overlay element

    function renderForm(html) {
        stage.innerHTML = html;
        scanComponents(stage);
        const firstFocusable = stage.querySelector('input, select, button, textarea');
        if (firstFocusable) firstFocusable.focus();
    }

    function updateControl(selector, html) {
        const el = document.querySelector(selector);
        if (!el) return;
        destroyComponents(el);
        el.outerHTML = html;
        const fresh = document.querySelector(selector);
        const scope = fresh ? (fresh.parentElement || stage) : stage;
        scanComponents(scope);
    }

    function closeForm(name, top) {
        if (top) {
            destroyComponents(stage);
            stage.innerHTML = '';
        } else {
            const formEl = document.getElementById('f:' + name);
            if (formEl) {
                destroyComponents(formEl);
                formEl.remove();
            }
        }
    }

    function renderModalForm(formName, html, gapX, gapY) {
        const overlay = document.createElement('div');
        overlay.id = 'mf:' + formName;
        overlay.className = 'form-modal-overlay';
        overlay.innerHTML = `
            <div class="form-modal" role="dialog" aria-modal="true" style="--gap-x: ${gapX}%; --gap-y: ${gapY}%;">
                ${html}
            </div>
        `;
        modals.appendChild(overlay);
        modalForms.set(formName, overlay);
        const firstFocusable = overlay.querySelector('input, select, button, textarea');
        if (firstFocusable) firstFocusable.focus();
        scanComponents(overlay);
        overlay.addEventListener('keydown', function(e) {
            if (e.key === 'Tab') trapFocus(e, overlay);
        });
    }

    function closeModalForm(formName) {
        const overlay = modalForms.get(formName);
        if (overlay) {
            destroyComponents(overlay);
            overlay.remove();
            modalForms.delete(formName);
        }
    }

    // Selection ops
    function handleFocusControl(form, ctrl) {
        const el = document.querySelector('#c:' + form + ':' + ctrl);
        if (el) el.focus();
    }

    function handleSelectText(form, ctrl) {
        const el = document.querySelector('#c:' + form + ':' + ctrl);
        if (!el) return;
        el.focus();
        if (typeof el.setSelectionRange === 'function') {
            const len = el.value ? el.value.length : 0;
            el.setSelectionRange(0, len);
        }
    }

    function handleSelectRange(form, ctrl, data) {
        const el = document.querySelector('#c:' + form + ':' + ctrl);
        if (!el) return;
        let range = { from: 0, to: 0 };
        try { if (data) range = JSON.parse(data); } catch (e) {}
        el.focus();
        if (typeof el.setSelectionRange === 'function') {
            el.setSelectionRange(range.from || 0, range.to || 0);
        }
    }

    function handleGetSelection(id, form, ctrl) {
        const el = document.querySelector('#c:' + form + ':' + ctrl);
        let start = 0, end = 0, text = '';
        if (el && typeof el.selectionStart === 'number') {
            start = el.selectionStart;
            end = el.selectionEnd;
            const v = el.value != null ? el.value : '';
            text = v.substring(start, end);
        }
        response({ type: 'selection_resp', id: id, value: { start: start, end: end, text: text } });
    }

    // ---- Event delegation --------------------------------------------------

    function getControlValue(el) {
        if (el.tagName === 'IMG') return el.getAttribute('src') || '';
        if (el.tagName === 'INPUT') {
            if (el.type === 'checkbox') return el.checked;
            if (el.type === 'radio') return el.checked ? el.value : null;
            return el.value;
        }
        if (el.tagName === 'SELECT') return el.value;
        if (el.tagName === 'TEXTAREA') return el.value;
        return el.textContent;
    }

    function collectFormValues(form) {
        const formEl = document.getElementById('f:' + form);
        if (!formEl) return null;
        const values = {};
        const inputs = formEl.querySelectorAll('input[data-k-form][data-k-ctrl], select[data-k-form][data-k-ctrl], textarea[data-k-form][data-k-ctrl]');
        inputs.forEach(function(input) {
            values[input.dataset.kCtrl] = getControlValue(input);
        });
        return values;
    }

    function handleClick(e) {
        const popupItem = e.target.closest('[data-k-popup-id]');
        if (popupItem) {
            const id = popupItem.dataset.kPopupId;
            if (popupItem.dataset.kSubmenu !== undefined) {
                e.preventDefault();
                togglePopupSubmenu(popupItem);
                return;
            }
            const choice = popupItem.dataset.kChoice || '';
            const json = popupItem.dataset.kValue;
            let value;
            try { value = JSON.parse(json); } catch (err) { value = choice; }
            response({ type: 'popup_choice', id: id, value: value });
            closePopup(id);
            return;
        }

        const msgboxBtn = e.target.closest('[data-k-msgbox-id][data-k-value]');
        if (msgboxBtn) {
            const id = msgboxBtn.dataset.kMsgboxId;
            const choice = msgboxBtn.dataset.kChoice || '';
            const json = msgboxBtn.dataset.kValue;
            let value;
            try { value = JSON.parse(json); } catch (err) { value = choice; }
            response({ type: 'msgbox_choice', id: id, value: value, choice: choice });
            closeMsgbox(id);
            return;
        }

        const loopEl = e.target.closest('.kalua-looper');
        if (loopEl) {
            selectLooperRow(loopEl, e.target.closest('.kalua-looper-row'), e.target);
            return;
        }

        const gridAction = e.target.closest('[data-k-grid-action]');
        if (gridAction) {
            e.preventDefault();
            const action = gridAction.dataset.kGridAction;
            let pk = null;
            if (gridAction.dataset.kGridPk !== '') {
                try { pk = JSON.parse(gridAction.dataset.kGridPk); } catch (err) { pk = gridAction.dataset.kGridPk; }
            }
            let row = {};
            try { row = JSON.parse(gridAction.dataset.kGridRow || '{}'); } catch (err) {}
            const wrapper = gridAction.closest('.kalua-grid');
            if (!wrapper) return;
            const cfg = gridCfgOf(wrapper);
            if (action === 'delete') {
                if (!window.confirm('Delete this row?')) return;
                response({ type: 'grid_row_delete', form: wrapper.dataset.kForm, ctrl: wrapper.dataset.kCtrl, value: { pk: pk } });
            } else if (action === 'view' || action === 'edit') {
                gridOpenForm(cfg, action, pk, row);
            }
            return;
        }

        const gridSave = e.target.closest('[data-k-grid-save]');
        if (gridSave) {
            e.preventDefault();
            const overlay = gridSave.closest('.form-modal-overlay');
            let pk = null;
            if (gridSave.dataset.kGridPk !== '') {
                try { pk = JSON.parse(gridSave.dataset.kGridPk); } catch (err) { pk = gridSave.dataset.kGridPk; }
            }
            response({
                type: 'grid_form_save',
                form: gridSave.dataset.kGridForm,
                ctrl: gridSave.dataset.kGridCtrl,
                value: {
                    mode: gridSave.dataset.kGridMode || 'edit',
                    pk: pk,
                    values: overlay ? gridFormValues(overlay) : {}
                }
            });
            return;
        }

        const gridCancel = e.target.closest('[data-k-grid-cancel]');
        if (gridCancel) {
            e.preventDefault();
            response({
                type: 'grid_form_cancel',
                form: gridCancel.dataset.kGridForm,
                ctrl: gridCancel.dataset.kGridCtrl
            });
            const overlay = gridCancel.closest('.form-modal-overlay');
            if (overlay) overlay.remove();
            const modalName = overlay && overlay.id.indexOf('mf:') === 0 ? overlay.id.substring(3) : null;
            if (modalName) modalForms.delete(modalName);
            return;
        }

        const target = e.target.closest('[data-k-form][data-k-ctrl]');
        if (!target) return;
        const form = target.dataset.kForm;
        const ctrl = target.dataset.kCtrl;
        let value = getControlValue(target);
        if (target.tagName === 'BUTTON') value = collectFormValues(form);
        reportDOMEvent(form, ctrl, 'click', value);
    }

    function handleInput(e) {
        const target = e.target.closest('[data-k-form][data-k-ctrl]');
        if (!target) return;
        reportDOMEvent(target.dataset.kForm, target.dataset.kCtrl, 'whenever_modified', getControlValue(target));
    }

    function handleChange(e) {
        const target = e.target.closest('[data-k-form][data-k-ctrl]');
        if (!target) return;
        reportDOMEvent(target.dataset.kForm, target.dataset.kCtrl, 'selection_change', getControlValue(target));
    }

    function handleFocus(e) {
        const target = e.target.closest('[data-k-form][data-k-ctrl]');
        if (!target) return;
        reportDOMEvent(target.dataset.kForm, target.dataset.kCtrl, 'get_focus', getControlValue(target));
    }

    function handleBlur(e) {
        const target = e.target.closest('[data-k-form][data-k-ctrl]');
        if (!target) return;
        reportDOMEvent(target.dataset.kForm, target.dataset.kCtrl, 'lose_focus', getControlValue(target));
    }

    function handleKeydown(e) {
        const target = e.target.closest('[data-k-form][data-k-ctrl]');
        if (!target) return;
        reportDOMEvent(target.dataset.kForm, target.dataset.kCtrl, 'key_pressed', { key: e.key, code: e.code });
    }

    // ---- Command dispatcher (driven by the Go brain) -----------------------

    function handleComponent(msg) {
        const kind = msg.kind;
        const op = msg.op;
        const selector = msg.selector;

        if (kind === 'tabulator') {
            switch (op) {
                case 'update': tabulatorUpdate(selector, msg.data); break;
                case 'remote_data': tabulatorRemoteData(msg); break;
                case 'refresh': tabulatorRefresh(selector); break;
                case 'get_data': tabulatorGetData(msg.id, selector, msg.form, msg.ctrl); break;
                case 'get_selection': tabulatorGetSelection(msg.id, selector); break;
                case 'destroy': tabulatorDestroy(selector); break;
            }
            return;
        }
        if (kind === 'chart') {
            switch (op) {
                case 'update': chartUpdate(selector, msg.data); break;
                case 'options': chartOptions(selector, msg.data); break;
                case 'resize': chartResize(selector, msg.data); break;
                case 'destroy': destroyChartBySelector(selector); break;
                case 'get_image': chartGetImage(msg.id, selector); break;
            }
            return;
        }
        if (kind === 'looper') {
            switch (op) {
                case 'batch': handleLooperDBBatch(msg); break;
                case 'refresh': looperRefresh(selector); break;
            }
        }
    }

    function destroyChartBySelector(selector) {
        const inst = chartInstances.get(selector);
        if (inst) { inst.destroy(); chartInstances.delete(selector); }
    }

    function handleCommand(cmd) {
        switch (cmd.cmd) {
            case 'init':
                break;
            case 'stage':
                renderForm(cmd.html);
                break;
            case 'modal_open':
                renderModalForm(cmd.name, cmd.html, cmd.gap_x, cmd.gap_y);
                if (cmd.grid) setupGridModal(cmd);
                break;
            case 'modal_close':
                closeModalForm(cmd.name);
                break;
            case 'form_close':
                closeForm(cmd.name, cmd.destroy && !!cmd.top);
                break;
            case 'update_control':
                updateControl(cmd.selector, cmd.html);
                break;
            case 'msgbox':
                showMsgbox(cmd.id, cmd.kind, cmd.html);
                break;
            case 'msgbox_close':
                closeMsgbox(cmd.id);
                break;
            case 'popup':
                showPopup(cmd.id, cmd.html);
                break;
            case 'popup_close':
                closePopup(cmd.id);
                break;
            case 'status':
                showStatus(cmd.text);
                break;
            case 'status_close':
                hideStatus();
                break;
            case 'error':
                showError(cmd.msg, cmd.stack);
                break;
            case 'quit':
                handleQuit();
                break;
            case 'reload':
                if (location.reload) location.reload();
                break;
            case 'focus':
                handleFocusControl(cmd.form, cmd.ctrl);
                break;
            case 'bell':
                playBell();
                break;
            case 'clipboard_set':
                setClipboard(cmd.text);
                break;
            case 'clipboard_get':
                getClipboard(cmd.id);
                break;
            case 'pick_file':
                pickFile(cmd.id, cmd.accept, cmd.multiple);
                break;
            case 'pick_file_save':
                pickFileSave(cmd.id, cmd.data);
                break;
            case 'select_text':
                handleSelectText(cmd.form, cmd.ctrl);
                break;
            case 'select_range':
                handleSelectRange(cmd.form, cmd.ctrl, cmd.data);
                break;
            case 'get_selection':
                handleGetSelection(cmd.id, cmd.form, cmd.ctrl);
                break;
            case 'component_scan': {
                const scope = el(cmd.scope || '#stage');
                if (scope) scanComponents(scope);
                break;
            }
            case 'component':
                handleComponent(cmd);
                break;
        }
    }

    // ---- utility ----------------------------------------------------------

    function escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    // ---- init -------------------------------------------------------------

    if (!window.kaluaPostMessage || !window.kaluaRegisterCallbacks) {
        console.error('[KALUA] WASM exports not available');
        return;
    }

    // Event delegation for controls.
    document.addEventListener('click', handleClick);
    document.addEventListener('input', handleInput);
    document.addEventListener('change', handleChange);
    document.addEventListener('focus', handleFocus, true);
    document.addEventListener('blur', handleBlur, true);
    document.addEventListener('keydown', handleKeydown);

    // The brain drives everything from here.
    window.kaluaRegisterCallbacks(function(msg) {
        handleCommand(msg);
    });

    // Report client info so k.screen_size/k.locale work.
    response({
        type: 'client_info',
        value: {
            w: window.innerWidth,
            h: window.innerHeight,
            locale: navigator.language || 'en-US'
        }
    });

    // Expose a small debug surface.
    window.KALUA = {
        scanComponents: scanComponents,
        destroyComponents: destroyComponents,
        send: response
    };
})();