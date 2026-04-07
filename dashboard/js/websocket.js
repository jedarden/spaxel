/**
 * Spaxel Dashboard - WebSocket Reconnection Manager
 *
 * Handles robust reconnection with exponential backoff, jitter,
 * disconnect state tracking, blob extrapolation, and visual state transitions.
 */
(function () {
    'use strict';

    // ── Configuration ───────────────────────────────────────────────────────
    var BACKOFF_BASE_MS = 1000;       // 1s initial
    var BACKOFF_MAX_MS  = 10000;      // 10s cap
    var JITTER_MS       = 500;        // ±500ms random jitter
    var SILENT_MS       = 5000;       // <5s: no UI change, extrapolate blobs
    var DIMMING_MS      = 30000;      // 30s: show modal
    var EXTRAP_MAX_S    = 2.0;        // max extrapolation duration in seconds

    // ── Internal state ───────────────────────────────────────────────────────
    var _ws              = null;
    var _connected       = false;
    var _connecting      = false;
    var _reconnectTimer  = null;
    var _reconnectAttempt = 0;
    var _disconnectStart = null;       // Date.now() when disconnect began
    var _lastSnapshot    = null;       // last received snapshot (for blob data)
    var _blobStates      = new Map();  // blobId -> { x, z, vx, vz, ts }
    var _extrapolRAF     = null;       // requestAnimationFrame id for extrapolation
    var _dimOverlay      = null;       // THREE.js overlay plane for dimming
    var _modalShown      = false;

    // ── Callbacks (set by app.js) ───────────────────────────────────────────
    var _onOpen    = null;  // function(ws) — called after successful connect
    var _onMessage = null;  // function(data) — called for each message
    var _onClose   = null;  // function(event) — called after ws close
    var _onError   = null;  // function(error) — called on ws error

    // ── Viz3D references (set by app.js during init) ─────────────────────────
    var _scene    = null;
    var _renderer = null;

    // ── Exponential backoff with jitter ──────────────────────────────────────
    function _backoffMs() {
        var delay = Math.min(BACKOFF_BASE_MS * Math.pow(2, _reconnectAttempt), BACKOFF_MAX_MS);
        var jitter = (Math.random() * 2 - 1) * JITTER_MS; // -500 to +500
        return Math.max(100, delay + jitter);
    }

    // ── Connection lifecycle ─────────────────────────────────────────────────
    function connect(url) {
        if (_ws && (_ws.readyState === WebSocket.OPEN || _ws.readyState === WebSocket.CONNECTING)) {
            return;
        }

        _connecting = true;
        console.log('[WS] Connecting (attempt ' + (_reconnectAttempt + 1) + ')...');

        try {
            _ws = new WebSocket(url);
        } catch (e) {
            console.error('[WS] Failed to create WebSocket:', e);
            _connecting = false;
            _scheduleReconnect();
            return;
        }

        _ws.binaryType = 'arraybuffer';

        _ws.onopen = function () {
            _connected = true;
            _connecting = false;
            _reconnectAttempt = 0;
            _stopExtrapolation();
            console.log('[WS] Connected');
            _updateStatusUI('connected');
            _fireOnOpen(_ws);
        };

        _ws.onclose = function (event) {
            console.log('[WS] Closed:', event.code, event.reason);
            _connected = false;
            _connecting = false;
            _ws = null;

            if (!_disconnectStart) {
                _disconnectStart = Date.now();
                _captureBlobStates();
            }

            _updateStatusUI('disconnected');
            _startDisconnectTimer();
            _scheduleReconnect();
            _fireOnClose(event);
        };

        _ws.onerror = function (error) {
            console.error('[WS] Error:', error);
            _fireOnError(error);
        };

        _ws.onmessage = function (event) {
            _fireOnMessage(event.data);
        };
    }

    function disconnect() {
        _stopDisconnectTimer();
        _stopExtrapolation();
        if (_reconnectTimer) {
            clearTimeout(_reconnectTimer);
            _reconnectTimer = null;
        }
        if (_ws) {
            _ws.onclose = null; // prevent reconnect
            _ws.close();
            _ws = null;
        }
        _connected = false;
        _connecting = false;
        _disconnectStart = null;
        _reconnectAttempt = 0;
        _restoreScene();
        _dismissModal();
        _updateStatusUI('disconnected');
    }

    function _scheduleReconnect() {
        if (_reconnectTimer) clearTimeout(_reconnectTimer);
        var delay = _backoffMs();
        console.log('[WS] Reconnecting in', Math.round(delay), 'ms');
        _reconnectTimer = setTimeout(function () {
            _reconnectTimer = null;
            _reconnectAttempt++;
            var wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
            connect(wsProtocol + '//' + window.location.host + '/ws/dashboard');
        }, delay);
    }

    // ── Disconnect state timer ──────────────────────────────────────────────
    var _disconnectTimer = null;

    function _startDisconnectTimer() {
        _stopDisconnectTimer();
        _disconnectTimer = setInterval(function () {
            if (_connected || !_disconnectStart) return;
            var elapsed = Date.now() - _disconnectStart;
            if (elapsed >= SILENT_MS && elapsed < DIMMING_MS) {
                _applyDimming();
            } else if (elapsed >= DIMMING_MS) {
                _showModal();
            }
        }, 500);
    }

    function _stopDisconnectTimer() {
        if (_disconnectTimer) {
            clearInterval(_disconnectTimer);
            _disconnectTimer = null;
        }
    }

    // ── Blob position extrapolation (<5s) ────────────────────────────────────
    function _captureBlobStates() {
        // Capture current blob positions and velocities from last known state
        if (!_lastSnapshot || !_lastSnapshot.blobs) return;

        _lastSnapshot.blobs.forEach(function (b) {
            _blobStates.set(b.id, {
                x: b.x,
                z: b.z,
                vx: b.vx || 0,
                vz: b.vz || 0,
                ts: Date.now()
            });
        });
    }

    function _startExtrapolation() {
        if (_blobStates.size === 0) return;
        if (_extrapolRAF) return; // already running

        function tick() {
            if (_connected) return;
            var now = Date.now();

            _blobStates.forEach(function (state, blobId) {
                var elapsed = (now - state.ts) / 1000; // seconds
                if (elapsed > EXTRAP_MAX_S) return; // cap extrapolation

                // position = last_position + last_velocity * elapsed
                var newX = state.x + state.vx * elapsed;
                var newZ = state.z + state.vz * elapsed;

                // Update Viz3D blob position
                if (window.Viz3D && Viz3D.extrapolateBlobPosition) {
                    Viz3D.extrapolateBlobPosition(blobId, newX, newZ);
                }
            });

            _extrapolRAF = requestAnimationFrame(tick);
        }

        _extrapolRAF = requestAnimationFrame(tick);
    }

    function _stopExtrapolation() {
        if (_extrapolRAF) {
            cancelAnimationFrame(_extrapolRAF);
            _extrapolRAF = null;
        }
        _blobStates.clear();
    }

    // ── Visual state transitions ─────────────────────────────────────────────

    // Dim overlay: semi-transparent plane in front of camera
    function _applyDimming() {
        if (_dimOverlay) return; // already dimmed
        if (!_scene) return;

        // Use a screen-space overlay via CSS instead of 3D plane for simplicity
        var overlay = document.getElementById('ws-dim-overlay');
        if (!overlay) {
            overlay = document.createElement('div');
            overlay.id = 'ws-dim-overlay';
            overlay.style.cssText = 'position:fixed;top:0;left:0;right:0;bottom:0;' +
                'background:rgba(0,0,0,0.5);z-index:50;pointer-events:auto;' +
                'transition:opacity 0.5s ease;opacity:0;';
            document.body.appendChild(overlay);
        }
        // Trigger fade in
        requestAnimationFrame(function () {
            overlay.style.opacity = '1';
        });
        _dimOverlay = overlay;

        // Show reconnecting spinner in status bar
        var spinner = document.getElementById('ws-reconnect-spinner');
        if (spinner) spinner.classList.add('visible');
        var reconnectText = document.getElementById('ws-status-text');
        if (reconnectText) reconnectText.textContent = 'Reconnecting...';
    }

    function _restoreScene() {
        if (_dimOverlay) {
            _dimOverlay.style.opacity = '0';
            setTimeout(function () {
                if (_dimOverlay && _dimOverlay.parentNode) {
                    _dimOverlay.parentNode.removeChild(_dimOverlay);
                }
                _dimOverlay = null;
            }, 500);
        }

        var spinner = document.getElementById('ws-reconnect-spinner');
        if (spinner) spinner.classList.remove('visible');

        if (_connected) {
            var reconnectText = document.getElementById('ws-status-text');
            if (reconnectText) reconnectText.textContent = 'Connected';
        }
    }

    // ── Connection lost modal (>30s) ────────────────────────────────────────
    function _showModal() {
        if (_modalShown) return;
        _modalShown = true;

        // Stop extrapolation when showing modal (scene is stale anyway)
        _stopExtrapolation();

        var existing = document.getElementById('ws-lost-modal');
        if (existing) return;

        var modal = document.createElement('div');
        modal.id = 'ws-lost-modal';
        modal.className = 'ws-lost-modal';
        modal.innerHTML =
            '<div class="ws-lost-modal-content">' +
            '<h3>Connection lost</h3>' +
            '<p>The dashboard lost connection to the mothership. ' +
            'The scene below shows the last known state.</p>' +
            '<button class="ws-lost-reload-btn" onclick="location.reload()">Reload Page</button>' +
            '<button class="ws-lost-dismiss-btn" onclick="this.closest(\'.ws-lost-modal\').style.display=\'none\'">Dismiss</button>' +
            '</div>';
        document.body.appendChild(modal);
    }

    function _dismissModal() {
        _modalShown = false;
        var modal = document.getElementById('ws-lost-modal');
        if (modal && modal.parentNode) {
            modal.parentNode.removeChild(modal);
        }
    }

    // ── Status indicator ────────────────────────────────────────────────────
    function _updateStatusUI(status) {
        var dot = document.getElementById('ws-status');
        var text = document.getElementById('ws-status-text');
        if (!dot || !text) return;

        dot.classList.remove('connected', 'disconnected', 'reconnecting');

        if (status === 'connected') {
            dot.classList.add('connected');
            text.textContent = 'Connected';
        } else if (status === 'reconnecting') {
            dot.classList.add('reconnecting');
            text.textContent = 'Reconnecting...';
        } else {
            dot.classList.add('disconnected');
            // Don't override text if already set to "Reconnecting..."
            if (_disconnectStart && Date.now() - _disconnectStart >= SILENT_MS) {
                text.textContent = 'Reconnecting...';
            } else {
                text.textContent = 'Disconnected';
            }
        }
    }

    // ── Snapshot tracking (for blob extrapolation on disconnect) ────────────
    function setLastSnapshot(snapshot) {
        _lastSnapshot = snapshot;
    }

    // ── Reconnect handler: clear trails, restore scene ──────────────────────
    function onReconnected() {
        var elapsed = _disconnectStart ? ((Date.now() - _disconnectStart) / 1000).toFixed(1) : '?';
        console.log('[WS] Reconnected after ' + elapsed + 's');

        // Clear disconnect state
        _disconnectStart = null;
        _restoreScene();
        _dismissModal();
        _stopDisconnectTimer();

        // Clear blob trails in Viz3D
        if (window.Viz3D && Viz3D.clearAllTrails) {
            Viz3D.clearAllTrails();
        }
    }

    // ── Callbacks ───────────────────────────────────────────────────────────
    function _fireOnOpen(ws) {
        if (typeof _onOpen === 'function') _onOpen(ws);
    }
    function _fireOnMessage(data) {
        if (typeof _onMessage === 'function') _onMessage(data);
    }
    function _fireOnClose(event) {
        if (typeof _onClose === 'function') _onClose(event);
    }
    function _fireOnError(error) {
        if (typeof _onError === 'function') _onError(error);
    }

    // ── Public API ──────────────────────────────────────────────────────────
    window.SpaxelWebSocket = {
        /**
         * Initialize the WebSocket manager.
         * @param {Object} opts
         * @param {Function} opts.onOpen    — called with ws after connect
         * @param {Function} opts.onMessage — called with raw message data
         * @param {Function} opts.onClose   — called with CloseEvent
         * @param {Function} opts.onError   — called with ErrorEvent
         */
        init: function (opts) {
            if (opts.onOpen)    _onOpen = opts.onOpen;
            if (opts.onMessage) _onMessage = opts.onMessage;
            if (opts.onClose)   _onClose = opts.onClose;
            if (opts.onError)   _onError = opts.onError;
        },

        /** Connect (or reconnect) to the dashboard WebSocket. */
        connect: connect,

        /** Disconnect and stop all reconnection attempts. */
        disconnect: disconnect,

        /** Record the last snapshot for blob extrapolation. */
        setLastSnapshot: setLastSnapshot,

        /** Called when the connection is re-established (from snapshot handler). */
        onReconnected: onReconnected,

        /** Start blob extrapolation (called on disconnect if <5s). */
        startExtrapolation: _startExtrapolation,

        /** Get current connection state. */
        isConnected: function () { return _connected; },
        isConnecting: function () { return _connecting; },
        getDisconnectDurationMs: function () {
            return _disconnectStart ? (Date.now() - _disconnectStart) : 0;
        },

        /** Send raw data over the WebSocket. */
        send: function (data) {
            if (_ws && _ws.readyState === WebSocket.OPEN) {
                _ws.send(data);
            }
        }
    };

    console.log('[WS] Reconnection manager initialized');
})();
