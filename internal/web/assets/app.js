/**
 * KALUA vanilla JavaScript client
 * Handles WebSocket communication and DOM updates for forms/controls.
 */

(function() {
    'use strict';

    // Configuration
    const WS_PATH = '/ws/ui';
    const RECONNECT_DELAY = 2000;
    const PING_INTERVAL = 30000;

    // State
    let ws = null;
    let sessionId = null;
    let pingTimer = null;
    let reconnectTimer = null;

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

    // Connect to WebSocket
    function connect() {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const url = `${protocol}//${window.location.host}${WS_PATH}?script=${encodeURIComponent(getScriptParam())}`;

        ws = new WebSocket(url);

        ws.onopen = function() {
            console.log('[KALUA] WebSocket connected');
            clearTimeout(reconnectTimer);
            sendClientInfo();
            startPing();
        };

        ws.onmessage = function(event) {
            handleMessage(JSON.parse(event.data));
        };

        ws.onclose = function(event) {
            console.log('[KALUA] WebSocket closed:', event.code, event.reason);
            stopPing();
            if (event.code !== 1000) { // Not clean close
                scheduleReconnect();
            }
        };

        ws.onerror = function(error) {
            console.error('[KALUA] WebSocket error:', error);
        };
    }

    function getScriptParam() {
        const params = new URLSearchParams(window.location.search);
        return params.get('script') || 'app.lua';
    }

    function scheduleReconnect() {
        reconnectTimer = setTimeout(connect, RECONNECT_DELAY);
    }

    function startPing() {
        pingTimer = setInterval(function() {
            if (ws && ws.readyState === WebSocket.OPEN) {
                send({ type: 'ping' });
            }
        }, PING_INTERVAL);
    }

    function stopPing() {
        if (pingTimer) {
            clearInterval(pingTimer);
            pingTimer = null;
        }
    }

    function send(msg) {
        if (ws && ws.readyState === WebSocket.OPEN) {
            ws.send(JSON.stringify(msg));
        }
    }

    function sendClientInfo() {
        send({
            type: 'client_info',
            w: window.innerWidth,
            h: window.innerHeight,
            locale: navigator.language || 'en-US'
        });
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
function createTabulator(el) {
        var opts = {};
        try { opts = JSON.parse(el.dataset.kTabulatorOptions || '{}'); } catch (e) {}
        var cols = [];
        try { cols = JSON.parse(el.dataset.kTabulatorColumns || '[]'); } catch (e) {}
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
        opts.selectable = opts.selectable !== false;
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
        if (typeof opts.rowSelectionChanged !== 'function') {
            opts.rowSelectionChanged = function(selectedData, selectedRows) {
                var rows = [];
                if (selectedRows) {
                    selectedRows.forEach(function(r) {
                        var idx = r ? r.getPosition(true) : 0;
                        rows.push(idx + 1); // 1-based
                    });
                }
                sendEvent(form, ctrl, 'tabulator_selection_change', { rows: rows, data: selectedData || [] });
            };
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
        });
    }

    function tabulatorBySelector(selector) {
        return tabulatorInstances.get(selector);
    }

    // Message handling
    function handleMessage(msg) {
        switch (msg.type) {
            case 'init':
                sessionId = msg.form;
                break;
            case 'render_form':
                renderForm(msg.html);
                break;
            case 'update_control':
                updateControl(msg.selector, msg.html);
                break;
            case 'close_form':
                closeForm(msg.name, msg.top);
                break;
            case 'msgbox':
                showMsgbox(msg.id, msg.kind, msg.html);
                break;
            case 'close_msgbox':
                closeMsgbox(msg.id);
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
            case 'error':
                showError(msg.msg, msg.stack);
                break;
            case 'quit':
                handleQuit();
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
        const el = document.querySelector(selector);
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
        const el = typeof selector === 'string' && selector.charAt(0) === '#'
            ? document.querySelector(selector) : null;
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
    // payload: { rows: [{index, data:{control:value, "control.prop":value}}],
    //            has_more, last_page }.
    function handleLooperDBBatch(msg) {
        const el = looperById(msg.form, msg.ctrl, msg.selector);
        if (!el) return;
        const payload = {};
        try { Object.assign(payload, JSON.parse(msg.data || '{}')); } catch (e) {}
        const rows = payload.rows || [];
        const hasMore = !!payload.has_more;
        const lastPage = payload.last_page || 0;

        el.setAttribute('data-k-looper-loading', '0');
        const rowsEl = el.querySelector('.kalua-looper-rows');
        const template = rowsEl.querySelector('[data-k-looper-template="1"]');
        let templateCells = [];
        if (template) {
            template.querySelectorAll('.kalua-looper-cell').forEach(function(cell) {
                templateCells.push(cell.cloneNode(true));
            });
        }

        rows.forEach(function(row) {
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
            template.setAttribute('data-k-looper-template', '1');
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
        initTabulators(stage);
        initLoopers(stage);
        // Focus first focusable element
        const firstFocusable = stage.querySelector('input, select, button, textarea');
        if (firstFocusable) {
            firstFocusable.focus();
        }
    }

    function updateControl(selector, html) {
        const el = document.querySelector(selector);
        if (el) {
            // Destroy any Tabulator instance living inside the replaced element.
            destroyTabulators(el);
            el.outerHTML = html;
            const fresh = document.querySelector(selector);
            if (fresh) initTabulators(fresh.parentElement || stage);
            if (fresh) initLoopers(fresh.parentElement || stage);
        }
    }

    function closeForm(name, top) {
        if (top) {
            destroyTabulators(stage);
            stage.innerHTML = '';
        } else {
            const formEl = document.getElementById('f:' + name);
            if (formEl) {
                destroyTabulators(formEl);
                formEl.remove();
            }
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
        const el = document.querySelector(selector);
        if (el) {
            el.focus();
        }
    }

    // Event handlers (delegated)
    function handleClick(e) {
        // Msgbox buttons answer the modal (no form/ctrl context).
        const msgboxBtn = e.target.closest('[data-k-msgbox-id][data-k-choice]');
        if (msgboxBtn) {
            const id = msgboxBtn.dataset.kMsgboxId;
            const choice = msgboxBtn.dataset.kChoice;
            send({ type: 'msgbox_choice', id: id, choice: choice });
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

    // Initialize
    connect();

    // Handle page visibility for keep-alive
    document.addEventListener('visibilitychange', function() {
        if (document.hidden) {
            stopPing();
        } else {
            startPing();
        }
    });

    // Expose for debugging
    window.KALUA = {
        send: send,
        getSessionId: function() { return sessionId; }
    };
})();