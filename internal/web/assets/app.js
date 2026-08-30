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
            case 'error':
                showError(msg.msg, msg.stack);
                break;
            case 'quit':
                handleQuit();
                break;
            case 'focus':
                handleFocusControl(msg.form, msg.ctrl);
                break;
        }
    }

    // Form rendering
    function renderForm(html) {
        stage.innerHTML = html;
        // Focus first focusable element
        const firstFocusable = stage.querySelector('input, select, button, textarea');
        if (firstFocusable) {
            firstFocusable.focus();
        }
    }

    function updateControl(selector, html) {
        const el = document.querySelector(selector);
        if (el) {
            el.outerHTML = html;
        }
    }

    function closeForm(name, top) {
        if (top) {
            stage.innerHTML = '';
        } else {
            const formEl = document.getElementById('f:' + name);
            if (formEl) {
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
        const target = e.target.closest('[data-k-form][data-k-ctrl]');
        if (!target) return;

        const form = target.dataset.kForm;
        const ctrl = target.dataset.kCtrl;
        sendEvent(form, ctrl, 'click', getControlValue(target));
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