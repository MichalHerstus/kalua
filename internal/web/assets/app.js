/**
 * KALUA vanilla JavaScript client
 * Handles WebSocket communication and DOM updates for forms/controls.
 */

(function() {
    'use strict';

    // Transport abstraction - supports both WebSocket and WASM
    const Transport = {
        // Current transport instance
        current: null,

        // Initialize WebSocket transport
        initWebSocket: function() {
            const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
            const url = `${protocol}//${window.location.host}/ws/ui?script=${encodeURIComponent(getScriptParam())}`;

            const ws = new WebSocket(url);
            let pingTimer = null;
            let reconnectTimer = null;

            const transport = {
                send: function(msg) {
                    if (ws && ws.readyState === WebSocket.OPEN) {
                        ws.send(JSON.stringify(msg));
                    }
                },
                close: function() {
                    if (pingTimer) clearInterval(pingTimer);
                    if (reconnectTimer) clearTimeout(reconnectTimer);
                    ws.close(1000);
                }
            };

            ws.onopen = function() {
                console.log('[KALUA] WebSocket connected');
                if (reconnectTimer) clearTimeout(reconnectTimer);
                sendClientInfo();
                pingTimer = setInterval(function() {
                    if (ws && ws.readyState === WebSocket.OPEN) {
                        transport.send({ type: 'ping' });
                    }
                }, 30000);
            };

            transport.startPing = function() {
                if (!pingTimer) {
                    pingTimer = setInterval(function() {
                        if (ws && ws.readyState === WebSocket.OPEN) {
                            transport.send({ type: 'ping' });
                        }
                    }, 30000);
                }
            };

            transport.stopPing = function() {
                if (pingTimer) {
                    clearInterval(pingTimer);
                    pingTimer = null;
                }
            };

            ws.onmessage = function(event) {
                try {
                    handleMessage(JSON.parse(event.data));
                } catch (e) {
                    console.error('[KALUA] Failed to parse message:', e);
                }
            };

            ws.onclose = function(event) {
                console.log('[KALUA] WebSocket closed:', event.code, event.reason);
                if (pingTimer) clearInterval(pingTimer);
                if (event.code !== 1000) {
                    reconnectTimer = setTimeout(function() {
                        Transport.initWebSocket();
                    }, 2000);
                }
            };

            ws.onerror = function(error) {
                console.error('[KALUA] WebSocket error:', error);
            };

            Transport.current = transport;
        },

        // Initialize WASM transport
        initWasm: function() {
            if (!window.kaluaPostMessage || !window.kaluaRegisterCallbacks) {
                console.error('[KALUA] WASM exports not available');
                return;
            }

            const transport = {
                send: function(msg) {
                    window.kaluaPostMessage(JSON.stringify(msg));
                },
                close: function() {
                    // WASM doesn't need explicit close
                }
            };

            // Register callback for incoming messages
            window.kaluaRegisterCallbacks(function(msg) {
                handleMessage(msg);
            });

            Transport.current = transport;
            
            // Send client info after WASM is ready
            sendClientInfo();
        },

        // Send a message through the current transport
        send: function(msg) {
            if (Transport.current) {
                Transport.current.send(msg);
            }
        },

        // Close the current transport
        close: function() {
            if (Transport.current) {
                Transport.current.close();
                Transport.current = null;
            }
        }
    };

    // State
    let sessionId = null;

    // DOM elements
    const stage = document.getElementById('stage');
    const modals = document.getElementById('modals');
    const statusBar = document.getElementById('status-bar');

    // Event delegation for controls
    document.addEventListener('click', handleClick);
    document.addEventListener('input', handleInput);
    document.addEventListener('change', handleChange);
    document.addEventListener('focus', handleFocus, true);
    document.addEventListener('blur', handleBlur, true);
    document.addEventListener('keydown', handleKeydown);

    // Initialize transport based on environment
    function initTransport() {
        // Check if we're in WASM mode (kaluaStartApp is available)
        if (window.kaluaStartApp) {
            console.log('[KALUA] Initializing WASM transport');
            Transport.initWasm();
        } else {
            console.log('[KALUA] Initializing WebSocket transport');
            Transport.initWebSocket();
        }
    }

    function getScriptParam() {
        const params = new URLSearchParams(window.location.search);
        return params.get('script') || 'app.lua';
    }

    function sendClientInfo() {
        Transport.send({
            type: 'client_info',
            w: window.innerWidth,
            h: window.innerHeight,
            locale: navigator.language || 'en-US'
        });
    }

    // Generic send function - uses current transport
    function send(msg) {
        Transport.send(msg);
    }

    // Clipboard
    function setClipboard(text) {
        if (navigator.clipboard && navigator.clipboard.writeText) {
            navigator.clipboard.writeText(text).catch(function(){});
        }
    }

    function getClipboard(id) {
        if (!navigator.clipboard || !navigator.clipboard.readText) {
            send({type: 'clipboard_resp', id: id, value: ''});
            return;
        }
        navigator.clipboard.readText()
            .then(function(text) {
                send({type: 'clipboard_resp', id: id, value: text || ''});
            })
            .catch(function() {
                send({type: 'clipboard_resp', id: id, value: ''});
            });
    }

    // File picker
    function pickFile(id, accept, multiple) {
        var input = document.createElement('input');
        input.type = 'file';
        input.style.display = 'none';
        if (accept) input.accept = accept;
        if (multiple) input.multiple = true;

        input.addEventListener('change', function() {
            var files = input.files;
            if (!files || files.length === 0) {
                send({type: 'file_picker_resp', id: id, value: '[]'});
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
                        results.push({
                            name: file.name,
                            size: file.size,
                            type: file.type,
                            data: base64
                        });
                        remaining--;
                        if (remaining === 0) {
                            results.sort(function(a, b) {
                                return a.name.localeCompare(b.name);
                            });
                            send({type: 'file_picker_resp', id: id, value: JSON.stringify(results)});
                            document.body.removeChild(input);
                        }
                    };
                    reader.onerror = function() {
                        remaining--;
                        if (remaining === 0) {
                            send({type: 'file_picker_resp', id: id, value: JSON.stringify(results)});
                            document.body.removeChild(input);
                        }
                    };
                    reader.readAsDataURL(file);
                })(files[i]);
            }
        });

        input.addEventListener('cancel', function() {
            send({type: 'file_picker_resp', id: id, value: '[]'});
            document.body.removeChild(input);
        });

        document.body.appendChild(input);
        input.click();
    }

    function pickFileSave(id, data) {
        // data: {mode: "save"|"download", filename: "name.txt", data: "base64"}
        // For save mode: show save dialog
        // For download mode: trigger download
        var mode = data.mode || 'save';
        var filename = data.filename || 'file';
        var fileData = data.data || '';

        if (mode === 'download') {
            // Trigger download directly
            var link = document.createElement('a');
            link.href = 'data:application/octet-stream;base64,' + fileData;
            link.download = filename;
            link.style.display = 'none';
            document.body.appendChild(link);
            link.click();
            document.body.removeChild(link);
            send({type: 'file_picker_save_resp', id: id, value: JSON.stringify({path: filename, name: filename})});
            return;
        }

        // Save mode: show save dialog
        // Use <a download> for save-as, but we need user to pick location
        // Fallback: use a file input with type="file" and webkitdirectory for directory picking
        // Since we can't truly show a save dialog, we'll use a file input and let user pick a file to overwrite
        var input = document.createElement('input');
        input.type = 'file';
        input.style.display = 'none';
        input.accept = '*/*';

        input.addEventListener('change', function() {
            var file = input.files[0];
            if (!file) {
                send({type: 'file_picker_save_resp', id: id, value: JSON.stringify({path: '', name: ''})});
                document.body.removeChild(input);
                return;
            }
            // For save mode, we return the path of the selected file
            // In a real implementation, this would show a save-as dialog
            send({type: 'file_picker_save_resp', id: id, value: JSON.stringify({path: file.name, name: file.name})});
            document.body.removeChild(input);
        });

        input.addEventListener('cancel', function() {
            send({type: 'file_picker_save_resp', id: id, value: JSON.stringify({path: '', name: ''})});
            document.body.removeChild(input);
        });

        document.body.appendChild(input);
        input.click();
    }

    // ---- Tabulator table support ----
    // Instances are keyed by the element id (selector "#c:form:ctrl").
    const tabulatorInstances = new Map();
    // Pending remote-pagination requests: selector -> {resolve, reject, timer}.
    // Tabulator fires one dataLoader request at a time per table, so a single
    // slot per selector is sufficient.
    const ajaxResolvers = new Map();

    // initTabulators scans a DOM scope for table containers with
    // data-k-tabulator-options and initializes a Tabulator instance for each
    // that is not already managed. When the Tabulator library is not loaded
    // (missing asset), a plain <table> fallback is rendered so data is never
    // silently invisible.
    function initTabulators(scope) {
        const els = scope.querySelectorAll('.kalua-tabulator-table:not([data-k-tabulator-ready])');
        if (typeof Tabulator === 'undefined') {
            els.forEach(function(el) {
                renderFallbackTable(el);
            });
            return;
        }
        els.forEach(function(el) {
            createTabulator(el);
        });
    }

    // createTabulator reads the data-k-tabulator-* attributes on a container
    // and instantiates Tabulator. Columns are taken from data-k-tabulator-columns
    // (JSON array of {field,title,...}) or inferred from the first row of data.
    // An optional seed object supplies call-backs and overrides the grid layers
    // need (functions cannot travel as data attributes): seed = {
    //   columns, selectable, selectableRows, rowClick, rowSelectionChanged }.
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

        // Remote mode must be known before we decide whether to pass local data:
        // Tabulator v6 treats a `data` array (even []) as the LOCAL data source
        // and then never triggers remote loading, so DB/remote tables would
        // stay empty. Omit `data` so ajaxRequestFunc (below) drives the load.
        var remoteMode = opts.paginationMode === 'remote' || opts.pagination === 'remote';

        if (!cols || cols.length === 0) {
            cols = inferColumns(data);
        }
        opts.columns = cols;
        if (cols.length === 0) {
            // Let Tabulator build columns from the first (possibly remote) row.
            opts.autoColumns = true;
        }
        opts.layout = opts.layout || 'fitColumns';
        if (seed.selectable !== undefined) {
            opts.selectable = seed.selectable;
        } else {
            opts.selectable = opts.selectable !== false;
        }
        if (seed.selectableRows !== undefined) {
            opts.selectableRows = seed.selectableRows;
        }
        opts.selectableRangeMode = opts.selectableRangeMode || 'click';

        var form = el.dataset.kForm;
        var ctrl = el.dataset.kCtrl;

        if (remoteMode && (!data || data.length === 0)) {
            // Omit `data` so the remote ajax loader drives the initial load.
            delete opts.data;
        } else {
            opts.data = data || [];
        }

        // Bridge selection changes to the host as tabulator_selection_change.
        opts.rowSelectionChanged = seed.rowSelectionChanged || function(selectedData, selectedRows) {
            var rows = [];
            if (selectedRows) {
                selectedRows.forEach(function(r) {
                    var idx = r ? r.getPosition(true) : 0;
                    rows.push(idx + 1); // 1-based
                });
            }
            sendEvent(form, ctrl, 'tabulator_selection_change', { rows: rows, data: selectedData || [] });
        };

        if (seed.rowClick) {
            opts.rowClick = seed.rowClick;
        }

        // Remote pagination: route every data request (initial load, page change,
        // sort, filter) through a WebSocket round-trip using Tabulator's
        // documented ajaxRequestFunc override. ajaxURL is a dummy so Tabulator
        // enters ajax-loading mode; the request is never actually made over HTTP.
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
                        q.sort = (q.sort || []).map(function(s) {
                            return { field: s.field, dir: s.dir || 'asc' };
                        });
                        q.filter = (q.filter || []).map(function(f) {
                            return { field: f.field, type: f.type, value: f.value };
                        });
                        // Key the resolver slot by the table's selector so the
                        // tabulator_remote_data response can find it.
                        var selector = '#c:' + form + ':' + ctrl;
                        var timer = setTimeout(function() {
                            ajaxResolvers.delete(selector);
                            reject(new Error('tabulator_ajax_request timeout'));
                        }, 15000);
                        ajaxResolvers.set(selector, { resolve: resolve, reject: reject, timer: timer });
                        send({ type: 'tabulator_ajax_request', form: form, ctrl: ctrl, value: q });
                    });
                };
            }
        }

        var inst = new Tabulator(el, opts);
        tabulatorInstances.set('#' + el.id, inst);
        el.setAttribute('data-k-tabulator-ready', 'true');
        return inst;
    }

    // inferColumns derives Tabulator column definitions from the first row.
    function inferColumns(data) {
        var cols = [];
        if (!data || data.length === 0) return cols;
        var first = data[0];
        Object.keys(first).forEach(function(key) {
            var val = first[key];
            var col = { field: key, title: key };
            if (typeof val === 'number') {
                col.sorter = 'number';
                col.editor = 'number';
                col.hozAlign = 'right';
            } else if (typeof val === 'boolean') {
                col.formatter = 'tickCross';
                col.editor = 'tickCross';
                col.hozAlign = 'center';
            } else {
                col.sorter = 'string';
                col.editor = 'input';
            }
            cols.push(col);
        });
        return cols;
    }

    // renderFallbackTable renders a plain HTML <table> from the
    // data-k-tabulator-* attributes when the Tabulator library itself is not
    // available, so tabulator=true controls degrade to the classic grid.
    function renderFallbackTable(el) {
        var cols = [];
        try { cols = JSON.parse(el.dataset.kTabulatorColumns || '[]'); } catch (e) {}
        var data = [];
        try { data = JSON.parse(el.dataset.kTabulatorData || '[]'); } catch (e) {}

        if (!cols || cols.length === 0) {
            cols = inferColumns(data);
        }
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
                if (typeof v === 'boolean') {
                    v = v ? '\u2713' : '';
                }
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

    // destroyTabulators destroys all Tabulator instances in a DOM scope
    // (used on form close / update replacement).
    function destroyTabulators(scope) {
        const els = scope.querySelectorAll('.kalua-tabulator-table');
        els.forEach(function(el) {
            const key = '#' + el.id;
            const inst = tabulatorInstances.get(key);
            if (inst) {
                inst.destroy();
                tabulatorInstances.delete(key);
            }
            el.removeAttribute('data-k-tabulator-ready');
            gridInstances.delete(key);
        });
    }

    function tabulatorBySelector(selector) {
        return tabulatorInstances.get(selector);
    }

    // ---- CRUD Grid support (k.ctrl.grid, §7) ----
    // Config lives on the outer .kalua-grid wrapper (data-k-grid-*); the inner
    // .kalua-tabulator-table is a standard Tabulator container keyed the same
    // way as plain tables. gridInstances is keyed by the inner table's selector
    // and holds {wrapper, tableEl, inst, pkField, rowActions, globalActions,
    // rowClick, columnVisibility, selectionMode}.
    const gridInstances = new Map();

    // initGrids scans a DOM scope for grid wrappers and builds each one that is
    // not already managed. Must run BEFORE initTabulators so the inner table is
    // seeded (action column, selection, row click) and marked ready.
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
        if (!cols || cols.length === 0) {
            cols = inferColumns(data);
        }

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
                    if (row.isSelected()) {
                        row.deselect();
                    } else {
                        row.select();
                    }
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
            seed.rowSelectionChanged = function() {
                updateGridToolbar(cfg);
            };
        }

        const inst = createTabulator(tableEl, seed);
        cfg.inst = inst;
        gridInstances.set('#' + tableEl.id, cfg);
        renderGridToolbar(cfg);
        return cfg;
    }

    function gridActionsColumn(cfg) {
        return {
            field: '_actions',
            title: '',
            width: 120,
            hozAlign: 'center',
            headerSort: false,
            resizable: false,
            frozen: true,
            formatter: function(cell) {
                const d = cell.getData() || {};
                const pkv = d[cfg.pkField] === undefined || d[cfg.pkField] === null
                    ? '' : JSON.stringify(d[cfg.pkField]);
                const rowv = escapeHtml(JSON.stringify(d));
                let html = '<div class="kalua-grid-row-actions">';
                if (cfg.rowActions.view) {
                    html += gridActionButton('view', 'View', pkv, rowv);
                }
                if (cfg.rowActions.edit) {
                    html += gridActionButton('edit', 'Edit', pkv, rowv);
                }
                if (cfg.rowActions.delete) {
                    html += gridActionButton('delete', 'Delete', pkv, rowv);
                }
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

    function renderGridToolbar(cfg) {
        if (cfg.wrapper.querySelector('.kalua-grid-toolbar')) return;
        const tb = document.createElement('div');
        tb.className = 'kalua-grid-toolbar';
        tb.setAttribute('data-k-grid-form', cfg.wrapper.dataset.kForm);
        tb.setAttribute('data-k-grid-ctrl', cfg.wrapper.dataset.kCtrl);
        const ga = cfg.globalActions || {};
        if (ga.new_record) {
            tb.appendChild(gridToolbarButton('new_record', 'New'));
        }
        if (ga.batch_delete) {
            const delBtn = gridToolbarButton('batch_delete', 'Delete Selected');
            delBtn.disabled = true;
            delBtn.classList.add('disabled');
            tb.appendChild(delBtn);
        }
        if (cfg.columnVisibility) {
            const wrap = document.createElement('div');
            wrap.className = 'kalua-grid-columns';
            const toggle = document.createElement('button');
            toggle.type = 'button';
            toggle.className = 'kalua-grid-btn kalua-grid-btn-columns';
            toggle.setAttribute('data-k-grid-columns', '1');
            toggle.textContent = 'Columns';
            wrap.appendChild(toggle);
            const list = document.createElement('div');
            list.className = 'kalua-grid-columns-list hidden';
            wrap.appendChild(list);
            tb.appendChild(wrap);
        }
        cfg.wrapper.insertBefore(tb, cfg.wrapper.firstChild);
    }

    function gridToolbarButton(action, label) {
        const b = document.createElement('button');
        b.type = 'button';
        b.className = 'kalua-grid-btn kalua-grid-btn-' + action;
        b.setAttribute('data-k-grid-toolbar', action);
        b.textContent = label;
        return b;
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
        }).filter(function(v) {
            return v !== undefined && v !== null;
        });
    }

    function rebuildGridColumnsMenu(cfg) {
        const list = cfg.wrapper.querySelector('.kalua-grid-columns-list');
        if (!list) return;
        list.innerHTML = '';
        const inst = cfg.inst;
        if (!inst || typeof inst.getColumns !== 'function') return;
        const cols = inst.getColumns();
        cols.forEach(function(col) {
            const field = col.getField();
            if (field === '_actions') return;
            const def = col.getDefinition();
            const label = def.title !== undefined && def.title !== '' ? def.title : field;
            const item = document.createElement('label');
            item.className = 'kalua-grid-column-item';
            const cb = document.createElement('input');
            cb.type = 'checkbox';
            cb.checked = col.isVisible();
            cb.setAttribute('data-k-grid-column-toggle', field);
            cb.addEventListener('change', function() {
                if (cb.checked) {
                    inst.showColumn(field);
                } else {
                    inst.hideColumn(field);
                }
            });
            const span = document.createElement('span');
            span.textContent = label;
            item.appendChild(cb);
            item.appendChild(span);
            list.appendChild(item);
        });
    }

    // gridOpenForm asks the host to render the grid's detail/edit/new modal.
    function gridOpenForm(cfg, mode, pk, row) {
        send({
            type: 'grid_form_open',
            form: cfg.wrapper.dataset.kForm,
            ctrl: cfg.wrapper.dataset.kCtrl,
            value: { mode: mode, pk: pk, row: row || {} }
        });
    }

    // setupGridModal populates a grid's modal form from the server-sent row and
    // makes it read-only in view mode (the Save button is removed).
    function setupGridModal(msg) {
        const overlay = modalForms.get(msg.form);
        if (!overlay) return;
        const formEl = overlay.querySelector('#f:' + msg.form);
        let row = {};
        try { row = JSON.parse(msg.grid_row || '{}'); } catch (e) {}
        const mode = msg.grid_mode || 'edit';

        const controls = formEl ? formEl.querySelectorAll('[data-k-form][data-k-ctrl]') : [];
        controls.forEach(function(el) {
            if (el.tagName === 'IMG') {
                if (row[el.dataset.kCtrl]) {
                    el.setAttribute('src', String(row[el.dataset.kCtrl]));
                }
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

        // Apply form_width if specified
        const footer = overlay.querySelector('[data-k-grid-width]');
        if (footer) {
            const width = footer.dataset.kGridWidth;
            const modal = overlay.querySelector('.form-modal');
            if (modal) {
                modal.style.maxWidth = width;
            }
        }
    }

    function gridFormValues(formEl) {
        const values = {};
        if (!formEl) return values;
        formEl.querySelectorAll('[data-k-form][data-k-ctrl]').forEach(function(el) {
            values[el.dataset.kCtrl] = getControlValue(el);
        });
        return values;
    }

    // ---- Chart.js control support ----
    // Instances are keyed by the selector "#c:form:ctrl" (the canvas's form/ctrl
    // data attributes). The config (type/data/options) is stored server-side in
    // data-k-chart-config.
    const chartInstances = new Map();

    // initCharts scans a DOM scope for chart canvases and creates a Chart
    // instance for each that is not already managed.
    function initCharts(scope) {
        if (typeof Chart === 'undefined') return;
        const canvases = scope.querySelectorAll('.kalua-chart-canvas:not([data-k-chart-ready])');
        canvases.forEach(function(canvas) {
            const form = canvas.dataset.kForm;
            const ctrl = canvas.dataset.kCtrl;
            const key = '#c:' + form + ':' + ctrl;
            if (chartInstances.has(key)) return;

            let config = null;
            try {
                config = JSON.parse(canvas.getAttribute('data-k-chart-config') || '{}');
            } catch (e) { return; }
            if (!config.options) config.options = {};

            // Click / hover → report element + value to the host.
            const makePointHandler = function(eventName) {
                return function(event, elements, chart) {
                    if (!elements || !elements.length) return;
                    const el = elements[0];
                    sendEvent(form, ctrl, eventName, {
                        dataset_index: el.datasetIndex + 1,
                        index: el.index + 1,
                        value: chartElementValue(chart, el)
                    });
                };
            };
            config.options.onClick = makePointHandler('chart_click');
            config.options.onHover = makePointHandler('chart_hover');

            // Legend click → report dataset index, then keep the default
            // show/hide-dataset toggle behavior.
            config.options.plugins = config.options.plugins || {};
            config.options.plugins.legend = config.options.plugins.legend || {};
            if (!config.options.plugins.legend.onClick) {
                config.options.plugins.legend.onClick = function(e, legendItem, legend) {
                    const chart = legend.chart;
                    const di = legendItem.datasetIndex;
                    sendEvent(form, ctrl, 'chart_legend_click', { dataset_index: di + 1 });
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

    // chartElementValue resolves the data value under a clicked/hovered element.
    function chartElementValue(chart, el) {
        const ds = chart.data.datasets[el.datasetIndex];
        if (!ds) return null;
        const v = ds.data[el.index];
        if (v && typeof v === 'object' && !Array.isArray(v)) {
            return v.x !== undefined ? v.x : null;
        }
        return v === undefined ? null : v;
    }

    function chartBySelector(selector) {
        return chartInstances.get(selector);
    }

    // destroyCharts destroys all Chart instances whose canvas lives in scope
    // (used on form close / update replacement).
    function destroyCharts(scope) {
        if (typeof Chart === 'undefined') return;
        const canvases = scope.querySelectorAll('.kalua-chart-canvas[data-k-chart-ready]');
        canvases.forEach(function(canvas) {
            const key = '#c:' + (canvas.dataset.kForm || '') + ':' + (canvas.dataset.kCtrl || '');
            const inst = chartInstances.get(key);
            if (inst) {
                inst.destroy();
                chartInstances.delete(key);
            }
            canvas.removeAttribute('data-k-chart-ready');
        });
    }

    function destroyChartBySelector(selector) {
        const inst = chartInstances.get(selector);
        if (inst) {
            inst.destroy();
            chartInstances.delete(selector);
        }
    }

    // chartUpdate replaces the chart's data (and optionally options) and
    // re-renders without animation.
    function chartUpdate(selector, dataJson, optionsJson) {
        const chart = chartBySelector(selector);
        if (!chart) return;
        try {
            if (dataJson) chart.data = JSON.parse(dataJson);
            if (optionsJson) mergeChartOptions(chart.options, JSON.parse(optionsJson));
            chart.update('none');
        } catch (e) {}
    }

    // chartOptions deep-merges new Chart.js options into the live chart.
    function chartOptions(selector, optionsJson) {
        const chart = chartBySelector(selector);
        if (!chart) return;
        try {
            mergeChartOptions(chart.options, JSON.parse(optionsJson));
            chart.update();
        } catch (e) {}
    }

    // mergeChartOptions overlays source onto target at the leaf level so
    // partial option updates (e.g. {scales:{y:{beginAtZero:true}}}) keep the
    // rest of the chart options intact.
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
        const el = byId(selector);
        if (el && sizeJson) {
            const size = {};
            try { Object.assign(size, JSON.parse(sizeJson)); } catch (e) {}
            if (size.width) el.style.width = size.width + 'px';
            if (size.height) el.style.height = size.height + 'px';
        }
        const chart = chartBySelector(selector);
        if (chart) chart.resize();
    }

    // chartGetImage renders the canvas to a base64 PNG data URL and answers
    // the suspended k.chart.get_image coroutine.
    function chartGetImage(id, selector) {
        const chart = chartBySelector(selector);
        let value = '';
        if (chart) {
            try { value = chart.toBase64Image('image/png'); } catch (e) {}
        }
        send({ type: 'chart_image_resp', id: id, value: value });
    }

    // Date/time pickers (flatpickr, §4.1). Instances are keyed by selector so
    // re-rendered controls destroy the old picker before the element is swapped.
    const datePickerInstances = new Map();

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

    // Message handling
    function handleMessage(msg) {
        switch (msg.type) {
            case 'init':
                sessionId = msg.form;
                break;
            case 'render_form':
                if (msg.modal) {
                    renderModalForm(msg.form, msg.html, msg.gap_x, msg.gap_y);
                    if (msg.grid) {
                        setupGridModal(msg);
                    }
                    setupLoginModal(document.getElementById('mf:' + msg.form));
                } else {
                    renderForm(msg.html);
                }
                break;
            case 'update_control':
                updateControl(msg.selector, msg.html);
                break;
            case 'close_form':
                if (msg.modal) {
                    closeModalForm(msg.name || msg.form);
                } else {
                    closeForm(msg.name, msg.top);
                }
                break;
            case 'msgbox':
                showMsgbox(msg.id, msg.kind, msg.html);
                break;
            case 'close_msgbox':
                closeMsgbox(msg.id);
                break;
            case 'popup':
                showPopup(msg.id, msg.html);
                break;
            case 'close_popup':
                closePopup(msg.id);
                break;
            case 'status':
                showStatus(msg.text);
                break;
            case 'status_close':
                hideStatus();
                break;
            case 'clipboard_set':
                setClipboard(msg.text);
                break;
            case 'clipboard_get':
                getClipboard(msg.id);
                break;
            case 'pick_file':
                pickFile(msg.id, msg.accept, msg.multiple);
                break;
            case 'pick_file_save':
                pickFileSave(msg.id, msg.data);
                break;
            case 'error':
                showError(msg.msg, msg.stack);
                break;
            case 'quit':
                handleQuit();
                break;
            case 'reload':
                // Hot reload (--watch): the server asks all tabs to reload so
                // a fresh session is created from the current script on disk.
                location.reload();
                break;
            case 'focus':
                handleFocusControl(msg.form, msg.ctrl);
                break;
            case 'tabulator_update':
                tabulatorUpdate(msg.selector, msg.data);
                break;
            case 'tabulator_destroy':
                tabulatorDestroy(msg.selector || ('#f:' + msg.form));
                break;
            case 'tabulator_get_data':
                tabulatorGetData(msg.id, msg.selector, msg.form, msg.ctrl);
                break;
            case 'tabulator_get_selection':
                tabulatorGetSelection(msg.id, msg.selector, msg.form, msg.ctrl);
                break;
            case 'tabulator_remote_data':
                tabulatorRemoteData(msg);
                break;
            case 'tabulator_refresh':
                tabulatorRefresh(msg.selector);
                break;
            case 'looper_db_batch':
                handleLooperDBBatch(msg);
                break;
            case 'looper_refresh':
                looperRefresh(msg.selector || ('#c:' + msg.form + ':' + msg.ctrl));
                break;
            case 'tree_data':
                handleTreeData(msg);
                break;
            case 'chart_update':
                chartUpdate(msg.selector, msg.data);
                break;
            case 'chart_options':
                chartOptions(msg.selector, msg.data);
                break;
            case 'chart_resize':
                chartResize(msg.selector, msg.data);
                break;
            case 'chart_destroy':
                destroyChartBySelector(msg.selector);
                break;
            case 'chart_get_image':
                chartGetImage(msg.id, msg.selector);
                break;
            case 'select_text':
                handleSelectText(msg.form, msg.ctrl);
                break;
            case 'select_range':
                handleSelectRange(msg.form, msg.ctrl, msg.data);
                break;
            case 'get_selection':
                handleGetSelection(msg.id, msg.form, msg.ctrl);
                break;
        }
    }

    // tabulatorRefresh re-triggers a DB-linked table's remote loader so page 1
    // is re-fetched from the Go host (used by k.table.refresh / set_db_source).
    function tabulatorRefresh(selector) {
        const inst = tabulatorInstances.get(selector);
        if (!inst) return;
        // Clear local sort/filter and jump back to page 1; the dataLoader
        // re-requests page 1 from the host.
        inst.setSort(false);
        inst.setFilter(false);
        inst.setPage(1);
    }

    // tabulatorRemoteData resolves the pending remote-pagination ajax promise for
    // a table with the page the Go host returned. payload layout:
    // { data: [...], last_page?: N, last_row?: N }.
    // Tabulator's ajax module wants ajaxRequestFunc to resolve with a plain
    // ARRAY, so we resolve `data`; last_page/last_row are pushed via setMaxPage.
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
            // No pending request (e.g. an app-initiated push via
            // k.table.set_remote_data): replace data directly.
            const inst = tabulatorInstances.get(selector);
            if (inst) {
                inst.setData(data);
                applyMaxPage(selector, lastPage, lastRow);
            }
        }
    }

    // applyMaxPage sets the total page count from last_page/last_row.
    function applyMaxPage(selector, lastPage, lastRow) {
        const inst = tabulatorInstances.get(selector);
        if (!inst) return;
        if (lastPage > 0) {
            inst.setMaxPage(lastPage);
        } else if (lastRow > 0) {
            var size = inst.getPageSize() || 10;
            inst.setMaxPage(Math.ceil(lastRow / size));
        }
    }

    // tabulatorUpdate replaces the data of an existing instance (or re-creates
    // the instance from scratch when the container was replaced).
    function tabulatorUpdate(selector, dataJSON) {
        const el = byId(selector);
        if (!el) return;
        const inst = tabulatorInstances.get(selector);
        let data = [];
        try { data = JSON.parse(dataJSON || '[]'); } catch (e) {}
        if (inst) {
            inst.setData(data);
        } else {
            el.dataset.kTabulatorData = dataJSON || '[]';
            if (typeof Tabulator !== 'undefined') {
                createTabulator(el);
            } else {
                // Fallback mode: rebuild the plain table so data updates show.
                el.removeAttribute('data-k-tabulator-ready');
                renderFallbackTable(el);
            }
        }
    }

    // tabulatorDestroy destroys the instance(s) matching a selector or a form
    // scope (called on close_form / form stack pop).
    function tabulatorDestroy(selector) {
        const el = byId(selector);
        if (el) {
            const inst = tabulatorInstances.get('#' + el.id);
            if (inst) {
                inst.destroy();
                tabulatorInstances.delete('#' + el.id);
            }
            el.removeAttribute('data-k-tabulator-ready');
        } else if (!selector || selector.charAt(0) === '#') {
            // Whole-form destroy: iterate managed instances and remove those
            // inside the form element.
            const formEl = selector ? document.getElementById(selector.substring(2)) : null;
            tabulatorInstances.forEach(function(inst, key) {
                const owner = document.querySelector(key);
                if (owner && (!formEl || formEl.contains(owner))) {
                    inst.destroy();
                    tabulatorInstances.delete(key);
                    owner.removeAttribute('data-k-tabulator-ready');
                }
            });
        }
    }

    // Loopers — DB-linked virtual-scroll row lists (k.ctrl.looper + k.looper.*).
    // The container carries the paging contract as data-k-looper-* attributes and
    // a hidden template row describing one row's cells. Rows arrive as
    // looper_db_batch messages; scrolling near the bottom requests the next batch.

    // initLoopers scans a DOM scope for .kalua-looper containers and primes each
    // that is not already managed. DB-linked loopers request page 1 immediately.
    function initLoopers(scope) {
        const els = scope.querySelectorAll('.kalua-looper:not([data-k-looper-ready])');
        els.forEach(function(el) {
            el.setAttribute('data-k-looper-ready', 'true');
            el.setAttribute('data-k-looper-next', '1');
            el.setAttribute('data-k-looper-has-more', el.hasAttribute('data-k-looper-links') ? '1' : '0');
            el.addEventListener('scroll', function() {
                looperMaybeFetch(el);
            });
            if (el.getAttribute('data-k-looper-has-more') === '1') {
                looperMaybeFetch(el);
            }
        });
    }

    // looperNextStart returns the next start_idx to request (1-based).
    function looperNextStart(el) {
        return parseInt(el.getAttribute('data-k-looper-next') || '1', 10);
    }

    // looperMaybeFetch issues the next batch when the scroll position is near
    // the bottom and no request is in flight and more rows remain.
    function looperMaybeFetch(el) {
        if (el.getAttribute('data-k-looper-has-more') !== '1') return;
        if (el.getAttribute('data-k-looper-loading') === '1') return;
        if (el.scrollHeight - (el.scrollTop + el.clientHeight) > 80) return;
        const form = el.dataset.kForm || '';
        const ctrl = el.dataset.kCtrl || '';
        const count = parseInt(el.dataset.kLooperPageSize || '50', 10) || 50;
        const startIdx = looperNextStart(el);
        el.setAttribute('data-k-looper-loading', '1');
        send({
            type: 'looper_scroll_request',
            form: form,
            ctrl: ctrl,
            value: { start_idx: startIdx, count: count }
        });
    }

    // looperCellValue formats a batch value for display.
    function looperCellValue(v) {
        if (v === null || v === undefined) return '';
        if (typeof v === 'boolean') return v ? '\u2713' : '';
        return String(v);
    }

    // looperById resolves a looper element. The looper id is "c:form:ctrl", which
    // contains colons and therefore must be looked up by id, never querySelector.
    function looperById(form, ctrl, selector) {
        const id = (selector || ('#c:' + form + ':' + ctrl)).replace(/^#/, '');
        return document.getElementById(id);
    }

    // handleLooperDBBatch appends rows from a looper_db_batch payload.
    // payload: { rows: [{index, data:{control:value, "control.prop":value}} |
    //                    {index, html}], has_more, last_page }.
    // Row-template loopers (data-k-looper-html="1") insert server-rendered
    // html rows directly; legacy loopers clone the value-cell template.
    function handleLooperDBBatch(msg) {
        const el = looperById(msg.form, msg.ctrl, msg.selector);
        if (!el) return;
        const payload = {};
        try { Object.assign(payload, JSON.parse(msg.data || '{}')); } catch (e) {}
        const rows = payload.rows || [];
        const hasMore = !!payload.has_more;
        const lastPage = payload.last_page || 0;
        const htmlMode = el.hasAttribute('data-k-looper-html');

        el.setAttribute('data-k-looper-loading', '0');
        const rowsEl = el.querySelector('.kalua-looper-rows');
        if (!rowsEl) return;
        const template = rowsEl.querySelector('[data-k-looper-template="1"]');
        let templateCells = [];
        if (template) {
            template.querySelectorAll('.kalua-looper-cell').forEach(function(cell) {
                templateCells.push(cell.cloneNode(true));
            });
        }

        rows.forEach(function(row) {
            if (htmlMode && row.html) {
                const wrap = document.createElement('div');
                wrap.innerHTML = row.html;
                const htmlRow = wrap.firstElementChild;
                if (htmlRow) {
                    htmlRow.setAttribute('data-k-looper-index', String(row.index || ''));
                    rowsEl.appendChild(htmlRow);
                }
                return;
            }
            const rowEl = document.createElement('div');
            rowEl.className = 'kalua-looper-row';
            rowEl.setAttribute('data-k-looper-index', String(row.index || ''));
            if (templateCells.length === 0) {
                const cell = document.createElement('div');
                cell.className = 'kalua-looper-cell';
                cell.innerHTML = '<span class="kalua-looper-cell-value"></span>';
                rowEl.appendChild(cell);
            } else {
                templateCells.forEach(function(cell) {
                    rowEl.appendChild(cell.cloneNode(true));
                });
            }
            const data = row.data || {};
            rowEl.querySelectorAll('.kalua-looper-cell').forEach(function(cell) {
                const key = cell.getAttribute('data-k-looper-control') || '';
                const valueEl = cell.querySelector('.kalua-looper-cell-value');
                if (valueEl && key) {
                    valueEl.textContent = looperCellValue(data[key]);
                }
            });
            rowsEl.appendChild(rowEl);
        });

        const next = looperNextStart(el) + rows.length;
        el.setAttribute('data-k-looper-next', String(next));
        el.setAttribute('data-k-looper-has-more', hasMore ? '1' : '0');
        if (lastPage > 0) el.setAttribute('data-k-looper-last-page', String(lastPage));

        // Keep fetching while the viewport can still be filled by short first pages.
        if (hasMore && el.scrollHeight <= el.clientHeight) {
            looperMaybeFetch(el);
        }
    }

    // looperRefresh resets a looper's rows and re-requests page 1 (driven by
    // k.looper.refresh / k.looper.set_db_source / k.looper.link_db). A refresh
    // rebuilds the row set, so the current row selection is dropped.
    function looperRefresh(selector) {
        const el = document.getElementById(String(selector || '').replace(/^#/, ''));
        if (!el) return;
        el.querySelectorAll('.kalua-looper-row.selected').forEach(function(r) {
            r.classList.remove('selected');
        });
        const rowsEl = el.querySelector('.kalua-looper-rows');
        if (rowsEl) {
            const template = rowsEl.querySelector('[data-k-looper-template="1"]');
            rowsEl.querySelectorAll('.kalua-looper-row:not([data-k-looper-template="1"])').forEach(function(r) {
                r.remove();
            });
            if (template) {
                template.setAttribute('data-k-looper-template', '1');
            }
        }
        el.setAttribute('data-k-looper-next', '1');
        el.setAttribute('data-k-looper-has-more', '1');
        el.setAttribute('data-k-looper-loading', '0');
        looperMaybeFetch(el);
    }

    // selectLooperRow implements the looper cursor: clicking a row highlights it
    // (single selection) and reports the selection to the host as onselect(line_idx)
    // plus onclick(ctrl_name, line_idx). The highlight is a class on the row
    // element, so it stays put as further batches append below it.
    function selectLooperRow(el, rowEl, target) {
        if (!rowEl || rowEl.hasAttribute('data-k-looper-template')) return;
        el.querySelectorAll('.kalua-looper-row.selected').forEach(function(r) {
            r.classList.remove('selected');
        });
        rowEl.classList.add('selected');

        const lineIdx = parseInt(rowEl.getAttribute('data-k-looper-index') || '0', 10);
        let ctrlName = '';
        if (target) {
            const cellEl = target.closest('.kalua-looper-cell');
            if (cellEl) ctrlName = cellEl.getAttribute('data-k-looper-control') || '';
        }
        const form = el.dataset.kForm || '';
        const ctrl = el.dataset.kCtrl || '';
        sendEvent(form, ctrl, 'onselect', { line_idx: lineIdx, ctrl_name: ctrlName });
        sendEvent(form, ctrl, 'onclick', { ctrl_name: ctrlName, line_idx: lineIdx });
    }

    // Trees — DB-linked expandable tree lists (k.ctrl.tree §9).
    // The container carries the DB-link config as data-k-tree-* attributes; the
    // client issues a single tree_data_request and renders the nested JSON
    // payload locally (full load — expand/collapse never round-trips).

    // initTrees scans a DOM scope for .kalua-tree containers and primes each
    // that is not already managed, requesting the full tree data once.
    function initTrees(scope) {
        scope.querySelectorAll('.kalua-tree:not([data-k-tree-ready])').forEach(function(el) {
            el.setAttribute('data-k-tree-ready', 'true');
            const form = el.dataset.kForm || '';
            const ctrl = el.dataset.kCtrl || '';
            send({ type: 'tree_data_request', form: form, ctrl: ctrl });
        });
    }

    // treeById resolves a tree element. The tree id is "c:form:ctrl", which
    // contains colons and therefore must be looked up by id, never querySelector.
    function treeById(form, ctrl, selector) {
        const id = (selector || ('#c:' + form + ':' + ctrl)).replace(/^#/, '');
        return document.getElementById(id);
    }

    // byId resolves a DOM element by a "#c:form:ctrl" selector. The id contains
    // colons and therefore must be looked up by id, never querySelector.
    function byId(selector) {
        if (typeof selector !== 'string' || selector.charAt(0) !== '#') return null;
        return document.getElementById(selector.slice(1));
    }

    // handleTreeData renders the nested tree payload from a tree_data message.
    // payload: { nodes: [{id, parent_id, label, _children:[...], ...extra}],
    //            count, error? }.
    function handleTreeData(msg) {
        const el = treeById(msg.form, msg.ctrl, msg.selector);
        if (!el) return;
        const payload = {};
        try { Object.assign(payload, JSON.parse(msg.data || '{}')); } catch (e) {}
        const nodes = payload.nodes || [];
        const count = payload.count || 0;

        if (payload.error) {
            el.innerHTML = '<div class="kalua-tree-empty">' + escapeHtml(String(payload.error)) + '</div>';
            return;
        }
        if (count === 0) {
            el.innerHTML = '<div class="kalua-tree-empty">No data</div>';
            return;
        }

        const expandAll = el.getAttribute('data-k-tree-expand-all') === 'true';
        el.innerHTML = '<ul class="kalua-tree-root">' + renderTreeNodes(nodes, 0, expandAll) + '</ul>';
    }

    // renderTreeNodes renders a list of nodes recursively. Each node embeds its
    // full row map as data-k-node JSON so click events echo the whole record
    // back to the host (on_click/on_select receive the row map).
    function renderTreeNodes(list, depth, expandAll) {
        return list.map(function(n) {
            const hasKids = n._children && n._children.length > 0;
            const toggle = hasKids
                ? '<button type="button" class="kalua-tree-toggle" data-k-tree-toggle="1" aria-expanded="' + (expandAll ? 'true' : 'false') + '">' + (expandAll ? '\u25be' : '\u25b8') + '</button>'
                : '';
            const kids = hasKids
                ? '<ul class="kalua-tree-children"' + (expandAll ? '' : ' style="display:none"') + '>' + renderTreeNodes(n._children, depth + 1, expandAll) + '</ul>'
                : '';
            return '<li class="kalua-tree-node" data-k-node-id="' + escapeAttr(String(n.id)) + '"' +
                ' data-k-node="' + escapeAttr(JSON.stringify(n)) + '" style="padding-left:' + (depth * 14) + 'px">' +
                toggle +
                '<span class="kalua-tree-label" data-k-tree-label="1">' + escapeHtml(String(n.label)) + '</span>' +
                kids + '</li>';
        }).join('');
    }

    // treeChildrenOf returns the direct child <ul> of a tree node <li>.
    function treeChildrenOf(li) {
        for (let i = 0; i < li.children.length; i++) {
            if (li.children[i].classList.contains('kalua-tree-children')) return li.children[i];
        }
        return null;
    }

    // handleTreeToggle expands/collapses a node locally and reports on_expand
    // to the host with {node_id, expanded}.
    function handleTreeToggle(e) {
        const toggle = e.target.closest('[data-k-tree-toggle]');
        if (!toggle) return;
        e.preventDefault();
        e.stopPropagation();
        const li = toggle.closest('.kalua-tree-node');
        if (!li) return;
        const kids = treeChildrenOf(li);
        const expanded = !kids || kids.style.display === 'none';
        if (kids) {
            kids.style.display = expanded ? '' : 'none';
        }
        toggle.textContent = expanded ? '\u25be' : '\u25b8';
        toggle.setAttribute('aria-expanded', expanded ? 'true' : 'false');
        const tree = toggle.closest('.kalua-tree');
        sendEvent(tree.dataset.kForm || '', tree.dataset.kCtrl || '', 'on_expand', {
            node_id: li.getAttribute('data-k-node-id'),
            expanded: expanded
        });
    }

    // handleTreeLabelClick reports on_click (and on_select when selection is
    // enabled) with the node's full row map.
    function handleTreeLabelClick(e) {
        const label = e.target.closest('[data-k-tree-label]');
        if (!label) return;
        e.preventDefault();
        e.stopPropagation();
        const li = label.closest('.kalua-tree-node');
        if (!li) return;
        const tree = label.closest('.kalua-tree');
        let node = null;
        try { node = JSON.parse(li.getAttribute('data-k-node') || 'null'); } catch (err) {}
        if (!node) return;
        const form = tree.dataset.kForm || '';
        const ctrl = tree.dataset.kCtrl || '';
        sendEvent(form, ctrl, 'on_click', { node: node });
        const selection = tree.getAttribute('data-k-tree-selection') || 'single';
        if (selection !== 'none') {
            tree.querySelectorAll('.kalua-tree-node.selected').forEach(function(n) {
                n.classList.remove('selected');
            });
            li.classList.add('selected');
            sendEvent(form, ctrl, 'on_select', { node: node });
        }
    }

    // tabulatorGetData answers k.table.get_data requests with all current row data.
    function tabulatorGetData(id, selector, form, ctrl) {
        const inst = selector && tabulatorInstances.get(selector);
        const data = inst ? inst.getData() : [];
        // Convert to 1-based rows of string-keyed objects for the Lua side.
        send({ type: 'tabulator_data_resp', id: id, value: JSON.stringify(data) });
    }

    // tabulatorGetSelection answers k.table.get_selected_rows requests.
    function tabulatorGetSelection(id, selector, form, ctrl) {
        const inst = selector && tabulatorInstances.get(selector);
        let rows = [];
        if (inst) {
            const sel = inst.getSelectedRows();
            if (sel) {
                sel.forEach(function(r) {
                    rows.push(r.getPosition(true) + 1); // 1-based
                });
            }
        }
        send({ type: 'tabulator_selection_resp', id: id, rows: rows });
    }

    // Form rendering
    function renderForm(html) {
        stage.innerHTML = html;
        initGrids(stage);
        initTabulators(stage);
        initLoopers(stage);
        initTrees(stage);
        initCharts(stage);
        initDatePickers(stage);
        // Focus first focusable element
        const firstFocusable = stage.querySelector('input, select, button, textarea');
        if (firstFocusable) {
            firstFocusable.focus();
        }
    }

    function updateControl(selector, html) {
        const el = byId(selector);
        if (el) {
            // The id selector resolves to the inner field (input/select/img)
            // that carries the id attribute. The renderable unit is the outer
            // .kalua-control wrapper, so replace that — otherwise repeated
            // k.ctrl.set_value / update_control calls nest the control inside
            // itself and duplicate its label/field on every trigger.
            const wrapper = el.closest('.kalua-control') || el;
            destroyTabulators(wrapper);
            destroyCharts(wrapper);
            destroyDatePickers(wrapper);
            wrapper.outerHTML = html;
            const fresh = byId(selector);
            if (fresh) initGrids(fresh.parentElement || stage);
            if (fresh) initTabulators(fresh.parentElement || stage);
            if (fresh) initLoopers(fresh.parentElement || stage);
            if (fresh) initTrees(fresh.parentElement || stage);
            if (fresh) initCharts(fresh.parentElement || stage);
            if (fresh) initDatePickers(fresh.parentElement || stage);
        }
    }

    function closeForm(name, top) {
        if (top) {
            destroyTabulators(stage);
            destroyCharts(stage);
            destroyDatePickers(stage);
            stage.innerHTML = '';
        } else {
            const formEl = document.getElementById('f:' + name);
            if (formEl) {
                destroyTabulators(formEl);
                destroyCharts(formEl);
                destroyDatePickers(formEl);
                formEl.remove();
            }
        }
    }

    // Modal form rendering (for k.form.show with modal=true)
    const modalForms = new Map(); // form name -> overlay element

    // setupLoginModal focuses the login input and binds Enter-to-submit for
    // login controls inside a modal form overlay.
    function setupLoginModal(overlay) {
        const loginCtrl = overlay.querySelector('[data-k-login="1"]');
        if (!loginCtrl) return;

        const loginInput = loginCtrl.querySelector('input[name="login"]');
        if (loginInput) {
            loginInput.focus();
        }

        // Enter key in either input submits the form
        overlay.addEventListener('keydown', function(e) {
            if (e.key === 'Enter') {
                const submitBtn = loginCtrl.querySelector('[data-k-login-submit]');
                if (submitBtn) {
                    e.preventDefault();
                    submitBtn.click();
                }
            }
        });
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

        // Focus first focusable element
        const firstFocusable = overlay.querySelector('input, select, button, textarea');
        if (firstFocusable) {
            firstFocusable.focus();
        }

        // Initialize components inside the modal
        initGrids(overlay);
        initTabulators(overlay);
        initLoopers(overlay);
        initTrees(overlay);
        initCharts(overlay);
        initDatePickers(overlay);

        // Trap focus within modal (same as msgbox)
        overlay.addEventListener('keydown', function(e) {
            if (e.key === 'Tab') {
                trapFocus(e, overlay);
            }
        });
    }

    function closeModalForm(formName) {
        const overlay = modalForms.get(formName);
        if (overlay) {
            // Destroy components inside before removing
            destroyTabulators(overlay);
            destroyCharts(overlay);
            destroyDatePickers(overlay);
            overlay.remove();
            modalForms.delete(formName);
        }
    }

    // Msgbox (modal)
    function showMsgbox(id, kind, html) {
        const overlay = document.createElement('div');
        overlay.id = 'mb:' + id;
        overlay.className = 'msgbox-overlay';
        overlay.innerHTML = `
            <div class="msgbox msgbox-${kind}" role="dialog" aria-modal="true">
                ${html}
            </div>
        `;
        modals.appendChild(overlay);

        // Focus first button
        const firstButton = overlay.querySelector('button');
        if (firstButton) {
            firstButton.focus();
        }

        // Trap focus
        overlay.addEventListener('keydown', function(e) {
            if (e.key === 'Tab') {
                trapFocus(e, overlay);
            }
        });
    }

    function closeMsgbox(id) {
        const overlay = document.getElementById('mb:' + id);
        if (overlay) {
            overlay.remove();
        }
    }

    // Popup (multilevel menu modal)
    function showPopup(id, html) {
        const overlay = document.createElement('div');
        overlay.id = 'pop:' + id;
        overlay.className = 'popup-overlay';
        overlay.innerHTML = `
            <div class="popup" role="menu" aria-label="menu">
                ${html}
            </div>
        `;
        modals.appendChild(overlay);

        const firstItem = overlay.querySelector('.popup-item');
        if (firstItem) {
            firstItem.focus();
        }

        // Esc / outside click dismisses the popup (the Lua app gets nil).
        overlay.addEventListener('keydown', function(e) {
            if (e.key === 'Escape') {
                e.preventDefault();
                send({ type: 'popup_dismiss', id: id });
                closePopup(id);
            } else if (e.key === 'Tab') {
                trapFocus(e, overlay);
            }
        });
        overlay.addEventListener('mousedown', function(e) {
            if (e.target === overlay) {
                send({ type: 'popup_dismiss', id: id });
                closePopup(id);
            }
        });
    }

    function closePopup(id) {
        const overlay = document.getElementById('pop:' + id);
        if (overlay) {
            overlay.remove();
        }
    }

    // Expand/collapse a popup submenu (touch/keyboard; hover handled by CSS).
    function togglePopupSubmenu(item) {
        const submenu = item.querySelector('.kalua-popup-submenu');
        if (!submenu) {
            return;
        }
        item.classList.toggle('open');
    }

    function trapFocus(e, container) {
        const focusable = container.querySelectorAll(
            'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])'
        );
        const first = focusable[0];
        const last = focusable[focusable.length - 1];

        if (e.shiftKey && document.activeElement === first) {
            e.preventDefault();
            last.focus();
        } else if (!e.shiftKey && document.activeElement === last) {
            e.preventDefault();
            first.focus();
        }
    }

    // Status bar
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
        if (stack) {
            banner.innerHTML += `<pre>${escapeHtml(stack)}</pre>`;
        }
        document.body.appendChild(banner);

        // Auto-dismiss after 10 seconds
        setTimeout(function() {
            banner.remove();
        }, 10000);
    }

    function handleQuit() {
        // Show terminal state
        stage.innerHTML = '<div class="quit-message">Application ended.</div>';
        modals.innerHTML = '';
        statusBar.classList.add('hidden');
    }

    // Focus control
    function handleFocusControl(form, ctrl) {
        const selector = '#c:' + form + ':' + ctrl;
        const el = byId(selector);
        if (el) {
            el.focus();
        }
    }

    // Event handlers (delegated)
    function handleClick(e) {
        // Popup menu: a branch opens its submenu, a leaf answers popup_choice.
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
            try {
                value = JSON.parse(json);
            } catch (err) {
                value = choice;
            }
            send({ type: 'popup_choice', id: id, value: value });
            closePopup(id);
            return;
        }

        // Msgbox buttons answer the modal (no form/ctrl context).
        const msgboxBtn = e.target.closest('[data-k-msgbox-id][data-k-value]');
        if (msgboxBtn) {
            const id = msgboxBtn.dataset.kMsgboxId;
            const choice = msgboxBtn.dataset.kChoice || '';
            const json = msgboxBtn.dataset.kValue;
            let value;
            try {
                value = JSON.parse(json);
            } catch (err) {
                value = choice;
            }
            send({ type: 'msgbox_choice', id: id, value: value, choice: choice });
            closeMsgbox(id);
            return;
        }

        // Looper row clicks move the cursor: highlight the row and report
        // onselect/onclick to the host.
        const loopEl = e.target.closest('.kalua-looper');
        if (loopEl) {
            selectLooperRow(loopEl, e.target.closest('.kalua-looper-row'), e.target);
            return;
        }

        // Tree node toggle (expand/collapse) and label click (on_click/on_select).
        const treeToggle = e.target.closest('[data-k-tree-toggle]');
        if (treeToggle) {
            handleTreeToggle(e);
            return;
        }
        const treeLabel = e.target.closest('[data-k-tree-label]');
        if (treeLabel) {
            handleTreeLabelClick(e);
            return;
        }

        // Grid row action buttons (view/edit/delete).
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
                send({
                    type: 'grid_row_delete',
                    form: wrapper.dataset.kForm,
                    ctrl: wrapper.dataset.kCtrl,
                    value: { pk: pk }
                });
            } else if (action === 'view' || action === 'edit') {
                gridOpenForm(cfg, action, pk, row);
            }
            return;
        }

        // Grid toolbar buttons (New, Delete Selected, Columns).
        const gridTool = e.target.closest('[data-k-grid-toolbar]');
        if (gridTool) {
            e.preventDefault();
            const wrapper = gridTool.closest('.kalua-grid');
            if (!wrapper) return;
            const cfg = gridCfgOf(wrapper);
            const action = gridTool.dataset.kGridToolbar;
            if (action === 'new_record') {
                gridOpenForm(cfg, 'new', null, {});
            } else if (action === 'batch_delete') {
                const pks = collectSelectedGridPks(cfg);
                if (!pks.length) return;
                if (!window.confirm('Delete ' + pks.length + ' selected row(s)?')) return;
                send({
                    type: 'grid_batch_delete',
                    form: wrapper.dataset.kForm,
                    ctrl: wrapper.dataset.kCtrl,
                    value: { pks: pks }
                });
            }
            return;
        }

        // Grid column-visibility dropdown toggle.
        const gridCols = e.target.closest('[data-k-grid-columns]');
        if (gridCols) {
            e.preventDefault();
            const wrapper = gridCols.closest('.kalua-grid');
            if (!wrapper) return;
            const cfg = gridCfgOf(wrapper);
            if (!cfg) return;
            const list = wrapper.querySelector('.kalua-grid-columns-list');
            if (list.classList.contains('hidden')) {
                rebuildGridColumnsMenu(cfg);
                list.classList.remove('hidden');
            } else {
                list.classList.add('hidden');
            }
            return;
        }

        // Grid modal Save/Cancel buttons.
        const gridSave = e.target.closest('[data-k-grid-save]');
        if (gridSave) {
            e.preventDefault();
            const overlay = gridSave.closest('.form-modal-overlay');
            let pk = null;
            if (gridSave.dataset.kGridPk !== '') {
                try { pk = JSON.parse(gridSave.dataset.kGridPk); } catch (err) { pk = gridSave.dataset.kGridPk; }
            }
            send({
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
            send({
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

        // Login control Submit button.
        const loginSubmit = e.target.closest('[data-k-login-submit]');
        if (loginSubmit) {
            e.preventDefault();
            const overlay = loginSubmit.closest('.form-modal-overlay');
            const v = overlay ? gridFormValues(overlay) : {};
            send({ type: 'event', form: loginSubmit.dataset.kForm,
                   ctrl: loginSubmit.dataset.kCtrl, event: 'on_login_submit',
                   value: { login: v.login || '', password: v.password || '' } });
            return;
        }

        // Login control Cancel button.
        const loginCancel = e.target.closest('[data-k-login-cancel]');
        if (loginCancel) {
            e.preventDefault();
            send({ type: 'event', form: loginCancel.dataset.kForm,
                   ctrl: loginCancel.dataset.kCtrl, event: 'on_login_cancel', value: null });
            return;
        }

        const target = e.target.closest('[data-k-form][data-k-ctrl]');
        if (!target) return;

        const form = target.dataset.kForm;
        const ctrl = target.dataset.kCtrl;

        // For button clicks, also collect all form control values
        let value = getControlValue(target);
        if (target.tagName === 'BUTTON') {
            value = collectFormValues(form);
        }

        sendEvent(form, ctrl, 'click', value);
    }

    function collectFormValues(form) {
        const formEl = document.getElementById('f:' + form);
        if (!formEl) return null;

        const values = {};
        const inputs = formEl.querySelectorAll('input[data-k-form][data-k-ctrl], select[data-k-form][data-k-ctrl], textarea[data-k-form][data-k-ctrl]');
        inputs.forEach(function(input) {
            const ctrlName = input.dataset.kCtrl;
            values[ctrlName] = getControlValue(input);
        });
        return values;
    }

    function handleInput(e) {
        const target = e.target.closest('[data-k-form][data-k-ctrl]');
        if (!target) return;

        const form = target.dataset.kForm;
        const ctrl = target.dataset.kCtrl;
        sendEvent(form, ctrl, 'whenever_modified', getControlValue(target));
    }

    function handleChange(e) {
        const target = e.target.closest('[data-k-form][data-k-ctrl]');
        if (!target) return;

        const form = target.dataset.kForm;
        const ctrl = target.dataset.kCtrl;
        sendEvent(form, ctrl, 'selection_change', getControlValue(target));
    }

    function handleFocus(e) {
        const target = e.target.closest('[data-k-form][data-k-ctrl]');
        if (!target) return;

        const form = target.dataset.kForm;
        const ctrl = target.dataset.kCtrl;
        sendEvent(form, ctrl, 'get_focus', getControlValue(target));
    }

    function handleBlur(e) {
        const target = e.target.closest('[data-k-form][data-k-ctrl]');
        if (!target) return;

        const form = target.dataset.kForm;
        const ctrl = target.dataset.kCtrl;
        sendEvent(form, ctrl, 'lose_focus', getControlValue(target));
    }

    function handleKeydown(e) {
        const target = e.target.closest('[data-k-form][data-k-ctrl]');
        if (!target) return;

        const form = target.dataset.kForm;
        const ctrl = target.dataset.kCtrl;
        sendEvent(form, ctrl, 'key_pressed', {
            key: e.key,
            code: e.code
        });
    }

    function sendEvent(form, ctrl, event, value) {
        send({
            type: 'event',
            form: form,
            ctrl: ctrl,
            event: event,
            value: value
        });
    }

    function getControlValue(el) {
        if (el.tagName === 'IMG') {
            return el.getAttribute('src') || '';
        }
        if (el.tagName === 'INPUT') {
            if (el.type === 'checkbox') {
                return el.checked;
            }
            if (el.type === 'radio') {
                return el.checked ? el.value : null;
            }
            return el.value;
        }
        if (el.tagName === 'SELECT') {
            return el.value;
        }
        if (el.tagName === 'TEXTAREA') {
            return el.value;
        }
        return el.textContent;
    }

    // Utility
    function escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    // escapeAttr escapes a string for safe use inside a double-quoted HTML
    // attribute (including single quotes, which escapeHtml leaves alone).
    function escapeAttr(text) {
        return String(text)
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;')
            .replace(/'/g, '&#39;');
    }

    // Selection handlers
    function handleSelectText(form, ctrl) {
        const selector = '#c:' + form + ':' + ctrl;
        const el = byId(selector);
        if (!el) return;
        el.focus();
        if (typeof el.setSelectionRange === 'function') {
            const len = el.value ? el.value.length : 0;
            el.setSelectionRange(0, len);
        }
    }

    function handleSelectRange(form, ctrl, data) {
        const selector = '#c:' + form + ':' + ctrl;
        const el = byId(selector);
        if (!el) return;
        let range = { from: 0, to: 0 };
        try {
            if (data) range = JSON.parse(data);
        } catch (e) {}
        el.focus();
        if (typeof el.setSelectionRange === 'function') {
            el.setSelectionRange(range.from || 0, range.to || 0);
        }
    }

    function handleGetSelection(id, form, ctrl) {
        const selector = '#c:' + form + ':' + ctrl;
        const el = byId(selector);
        let start = 0, end = 0, text = '';
        if (el) {
            if (typeof el.selectionStart === 'number') {
                start = el.selectionStart;
                end = el.selectionEnd;
                const v = el.value != null ? el.value : '';
                text = v.substring(start, end);
            }
        }
        send({
            type: 'selection_resp',
            id: id,
            start: start,
            end: end,
            text: text
        });
    }

    // Utility
    function escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    // Initialize
    initTransport();

    // Handle page visibility for keep-alive (WebSocket only)
    document.addEventListener('visibilitychange', function() {
        if (document.hidden && Transport.current && Transport.current.stopPing) {
            Transport.current.stopPing();
        } else if (!document.hidden && Transport.current && Transport.current.startPing) {
            Transport.current.startPing();
        }
    });

    // Expose for debugging
    window.KALUA = {
        send: send,
        getSessionId: function() { return sessionId; }
    };
})();