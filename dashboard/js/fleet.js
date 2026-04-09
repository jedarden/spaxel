/**
 * Spaxel Dashboard - Fleet Health Panel
 *
 * Displays fleet status, role assignments, and re-optimisation controls.
 * Shows coverage degradation warnings and before/after GDOP comparison.
 */

(function() {
    'use strict';

    // ============================================
    // State
    // ============================================
    const state = {
        nodes: new Map(),           // MAC -> node info
        roles: new Map(),           // MAC -> role
        history: [],                // Recent optimisation events
        coverageScore: 0,
        meanGDOP: 0,
        warningMessage: null,
        isDegraded: false,
        selectedCompareEvent: null,
        wsConnected: false
    };

    const CONFIG = {
        pollIntervalMs: 15000,      // Poll /api/fleet/health every 15 seconds
        historyLimit: 5
    };

    let pollTimer = null;

    // ============================================
    // Initialization
    // ============================================
    function init() {
        console.log('[Fleet] Initializing fleet health panel');

        // Create panel if not exists
        createPanel();

        // Start polling
        startPolling();

        // Register WebSocket message handler
        if (window.SpaxelApp && window.SpaxelApp.registerMessageHandler) {
            SpaxelApp.registerMessageHandler(handleWSMessage);
        }

        console.log('[Fleet] Fleet health panel ready');
    }

    function createPanel() {
        const sidebar = document.querySelector('.sidebar');
        if (!sidebar) {
            console.warn('[Fleet] Sidebar not found');
            return;
        }

        // Check if panel already exists
        if (document.getElementById('fleet-panel')) {
            return;
        }

        const panel = document.createElement('div');
        panel.id = 'fleet-panel';
        panel.className = 'panel';
        panel.innerHTML = `
            <div class="panel-header">
                <h3>Fleet Health</h3>
                <button id="fleet-optimise-btn" class="btn btn-sm" title="Re-optimise roles now">
                    <span class="icon">&#x21BB;</span> Optimise
                </button>
            </div>
            <div class="panel-content">
                <div id="fleet-warning" class="fleet-warning hidden">
                    <span class="warning-icon">&#x26A0;</span>
                    <span id="fleet-warning-text"></span>
                    <button id="fleet-warning-dismiss" class="btn-dismiss">&times;</button>
                </div>

                <div class="fleet-stats">
                    <div class="stat-row">
                        <span class="stat-label">Coverage</span>
                        <span id="fleet-coverage" class="stat-value">--</span>
                    </div>
                    <div class="stat-row">
                        <span class="stat-label">Mean GDOP</span>
                        <span id="fleet-mean-gdop" class="stat-value">--</span>
                    </div>
                    <div class="stat-row">
                        <span class="stat-label">Nodes</span>
                        <span id="fleet-node-count" class="stat-value">--</span>
                    </div>
                </div>

                <div class="fleet-roles">
                    <h4>Role Assignments</h4>
                    <div id="fleet-role-list" class="fleet-role-list">
                        <div class="empty-state">No nodes</div>
                    </div>
                </div>

                <div class="fleet-history">
                    <h4>Re-optimisation History</h4>
                    <div id="fleet-history-list" class="fleet-history-list">
                        <div class="empty-state">No events</div>
                    </div>
                </div>

                <div class="fleet-simulate">
                    <h4>Simulate Node Removal</h4>
                    <select id="fleet-simulate-select" class="select-sm">
                        <option value="">Select a node...</option>
                    </select>
                    <div id="fleet-simulate-result" class="simulate-result hidden"></div>
                </div>
            </div>
        `;

        sidebar.appendChild(panel);

        // Add event handlers
        document.getElementById('fleet-optimise-btn').addEventListener('click', onOptimiseClick);
        document.getElementById('fleet-warning-dismiss').addEventListener('click', dismissWarning);
        document.getElementById('fleet-simulate-select').addEventListener('change', onSimulateSelect);

        addStyles();
    }

    function addStyles() {
        if (document.getElementById('fleet-styles')) return;

        const style = document.createElement('style');
        style.id = 'fleet-styles';
        style.textContent = `
            #fleet-panel {
                margin-top: 1rem;
            }
            #fleet-panel .panel-header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                margin-bottom: 0.5rem;
            }
            #fleet-panel .panel-header h3 {
                margin: 0;
                font-size: 0.9rem;
                color: #aaa;
            }
            #fleet-optimise-btn {
                padding: 0.25rem 0.5rem;
                font-size: 0.75rem;
                background: #333;
                border: 1px solid #555;
                color: #ddd;
                cursor: pointer;
                border-radius: 3px;
            }
            #fleet-optimise-btn:hover {
                background: #444;
            }
            #fleet-optimise-btn:disabled {
                opacity: 0.5;
                cursor: not-allowed;
            }
            .fleet-warning {
                background: rgba(239, 68, 68, 0.15);
                border: 1px solid rgba(239, 68, 68, 0.4);
                border-radius: 4px;
                padding: 0.5rem;
                margin-bottom: 0.75rem;
                display: flex;
                align-items: flex-start;
                gap: 0.5rem;
            }
            .fleet-warning.hidden {
                display: none;
            }
            .fleet-warning .warning-icon {
                color: #ef4444;
                font-size: 1rem;
            }
            .fleet-warning #fleet-warning-text {
                flex: 1;
                font-size: 0.8rem;
                color: #fca5a5;
            }
            .fleet-warning .btn-dismiss {
                background: none;
                border: none;
                color: #888;
                cursor: pointer;
                font-size: 1rem;
                padding: 0;
                line-height: 1;
            }
            .fleet-stats {
                background: rgba(255,255,255,0.03);
                border-radius: 4px;
                padding: 0.5rem;
                margin-bottom: 0.75rem;
            }
            .stat-row {
                display: flex;
                justify-content: space-between;
                padding: 0.25rem 0;
                font-size: 0.8rem;
            }
            .stat-label {
                color: #888;
            }
            .stat-value {
                color: #ddd;
                font-weight: 500;
            }
            .stat-value.degraded {
                color: #ef4444;
            }
            .fleet-roles h4, .fleet-history h4, .fleet-simulate h4 {
                font-size: 0.75rem;
                color: #888;
                margin: 0.75rem 0 0.5rem 0;
                text-transform: uppercase;
                letter-spacing: 0.5px;
            }
            .fleet-role-list {
                max-height: 150px;
                overflow-y: auto;
                font-size: 0.75rem;
            }
            .fleet-role-item {
                display: flex;
                justify-content: space-between;
                align-items: center;
                padding: 0.35rem 0.5rem;
                background: rgba(255,255,255,0.02);
                border-radius: 3px;
                margin-bottom: 0.25rem;
            }
            .fleet-role-item .node-mac {
                color: #aaa;
                font-family: monospace;
                font-size: 0.7rem;
            }
            .fleet-role-item .node-role {
                font-size: 0.65rem;
                padding: 0.15rem 0.35rem;
                border-radius: 2px;
                text-transform: uppercase;
                font-weight: 600;
            }
            .node-role.tx { background: rgba(239, 68, 68, 0.3); color: #fca5a5; }
            .node-role.rx { background: rgba(59, 130, 246, 0.3); color: #93c5fd; }
            .node-role.tx_rx { background: rgba(168, 85, 247, 0.3); color: #d8b4fe; }
            .node-role.passive { background: rgba(107, 114, 128, 0.3); color: #d1d5db; }
            .node-role .health-score {
                font-size: 0.65rem;
                color: #666;
                margin-left: 0.5rem;
            }
            .fleet-identify-btn {
                background: rgba(255, 193, 7, 0.2);
                border: 1px solid rgba(255, 193, 7, 0.4);
                color: #ffc107;
                font-size: 0.7rem;
                padding: 0.15rem 0.4rem;
                border-radius: 3px;
                cursor: pointer;
                transition: all 0.2s;
                margin-left: 0.5rem;
            }
            .fleet-identify-btn:hover {
                background: rgba(255, 193, 7, 0.3);
                border-color: rgba(255, 193, 7, 0.6);
            }
            .fleet-history-list {
                max-height: 120px;
                overflow-y: auto;
                font-size: 0.7rem;
            }
            .fleet-history-item {
                padding: 0.35rem 0.5rem;
                background: rgba(255,255,255,0.02);
                border-radius: 3px;
                margin-bottom: 0.25rem;
                cursor: pointer;
            }
            .fleet-history-item:hover {
                background: rgba(255,255,255,0.05);
            }
            .history-time {
                color: #666;
                font-size: 0.65rem;
            }
            .history-trigger {
                color: #aaa;
                margin-left: 0.5rem;
            }
            .history-delta {
                float: right;
                font-weight: 500;
            }
            .history-delta.positive { color: #66bb6a; }
            .history-delta.negative { color: #ef4444; }
            .fleet-simulate select {
                width: 100%;
                padding: 0.35rem;
                background: #222;
                border: 1px solid #444;
                color: #ddd;
                border-radius: 3px;
                font-size: 0.75rem;
            }
            .simulate-result {
                margin-top: 0.5rem;
                padding: 0.5rem;
                background: rgba(255,255,255,0.03);
                border-radius: 3px;
                font-size: 0.75rem;
            }
            .simulate-result.hidden {
                display: none;
            }
            .simulate-result.degraded {
                border-left: 3px solid #ef4444;
            }
            .simulate-result.ok {
                border-left: 3px solid #66bb6a;
            }
            .gdop-compare-overlay {
                position: fixed;
                top: 0;
                left: 0;
                right: 0;
                bottom: 0;
                background: rgba(0,0,0,0.85);
                z-index: 1000;
                display: flex;
                align-items: center;
                justify-content: center;
            }
            .gdop-compare-overlay.hidden {
                display: none;
            }
            .gdop-compare-content {
                background: #1a1a2e;
                padding: 1.5rem;
                border-radius: 8px;
                max-width: 800px;
                width: 90%;
            }
            .gdop-compare-header {
                display: flex;
                justify-content: space-between;
                margin-bottom: 1rem;
            }
            .gdop-compare-header h3 {
                margin: 0;
                color: #ddd;
            }
            .gdop-compare-close {
                background: none;
                border: none;
                color: #888;
                font-size: 1.5rem;
                cursor: pointer;
            }
            .gdop-maps {
                display: flex;
                gap: 1rem;
            }
            .gdop-map-container {
                flex: 1;
            }
            .gdop-map-container h4 {
                margin: 0 0 0.5rem 0;
                font-size: 0.85rem;
                color: #aaa;
            }
            .gdop-map-canvas {
                width: 100%;
                height: 200px;
                background: #111;
                border-radius: 4px;
            }
            .gdop-slider-container {
                margin-top: 1rem;
                text-align: center;
            }
            .gdop-slider {
                width: 100%;
                max-width: 300px;
            }
            .empty-state {
                color: #555;
                font-size: 0.75rem;
                text-align: center;
                padding: 0.5rem;
            }
        `;
        document.head.appendChild(style);
    }

    // ============================================
    // Polling
    // ============================================
    function startPolling() {
        if (pollTimer) clearInterval(pollTimer);
        fetchFleetHealth();
        fetchFleetHistory();
        pollTimer = setInterval(function() {
            fetchFleetHealth();
            fetchFleetHistory();
        }, CONFIG.pollIntervalMs);
    }

    function fetchFleetHealth() {
        fetch('/api/fleet/health')
            .then(function(res) { return res.json(); })
            .then(function(data) {
                handleFleetHealth(data);
            })
            .catch(function(err) {
                console.error('[Fleet] Failed to fetch health:', err);
            });
    }

    function fetchFleetHistory() {
        fetch('/api/fleet/history?limit=' + CONFIG.historyLimit)
            .then(function(res) { return res.json(); })
            .then(function(data) {
                handleFleetHistory(data);
            })
            .catch(function(err) {
                console.error('[Fleet] Failed to fetch history:', err);
            });
    }

    // ============================================
    // Data Handlers
    // ============================================
    function handleFleetHealth(data) {
        state.coverageScore = data.coverage_score || 0;
        state.meanGDOP = data.mean_gdop || 0;
        state.isDegraded = data.is_degraded || false;

        // Update node list
        if (data.nodes) {
            state.nodes.clear();
            state.roles.clear();
            data.nodes.forEach(function(node) {
                state.nodes.set(node.mac, node);
                state.roles.set(node.mac, node.role);
            });
        }

        updateUI();
        updateSimulateSelect();
    }

    function handleFleetHistory(data) {
        state.history = data || [];
        updateHistoryUI();
    }

    function handleWSMessage(msg) {
        switch (msg.type) {
            case 'fleet_change':
                handleFleetChange(msg);
                break;
            case 'fleet_health':
                handleFleetHealth(msg);
                break;
            case 'fleet_history':
                handleFleetHistory(msg.history);
                break;
        }
    }

    function handleFleetChange(msg) {
        // Update state from real-time event
        if (msg.role_assignments) {
            state.roles.clear();
            Object.entries(msg.role_assignments).forEach(function(entry) {
                state.roles.set(entry[0], entry[1]);
            });
        }

        if (msg.coverage_after !== undefined) {
            state.coverageScore = msg.coverage_after;
        }
        if (msg.mean_gdop_after !== undefined) {
            state.meanGDOP = msg.mean_gdop_after;
        }
        state.isDegraded = msg.is_degradation || false;

        // Show warning if degraded
        if (msg.is_degradation && msg.warning_message) {
            showWarning(msg.warning_message);
        }

        // Show toast notification
        if (window.SpaxelApp && SpaxelApp.showToast) {
            var toastType = msg.is_degradation ? 'warning' : 'info';
            SpaxelApp.showToast('Fleet re-optimised: ' + msg.trigger_reason, toastType);
        }

        updateUI();

        // If we have GDOP comparison data, show comparison button
        if (msg.gdop_before && msg.gdop_after) {
            showComparisonButton(msg);
        }

        // Refresh history
        fetchFleetHistory();
    }

    // ============================================
    // UI Updates
    // ============================================
    function updateUI() {
        // Update coverage score
        var coverageEl = document.getElementById('fleet-coverage');
        if (coverageEl) {
            coverageEl.textContent = (state.coverageScore * 100).toFixed(0) + '%';
            coverageEl.classList.toggle('degraded', state.isDegraded);
        }

        // Update mean GDOP
        var gdopEl = document.getElementById('fleet-mean-gdop');
        if (gdopEl) {
            gdopEl.textContent = state.meanGDOP.toFixed(2);
        }

        // Update node count
        var countEl = document.getElementById('fleet-node-count');
        if (countEl) {
            var onlineCount = 0;
            state.nodes.forEach(function(node) {
                if (node.online) onlineCount++;
            });
            countEl.textContent = onlineCount + '/' + state.nodes.size;
        }

        // Update role list
        updateRoleList();
    }

    function updateRoleList() {
        var container = document.getElementById('fleet-role-list');
        if (!container) return;

        if (state.nodes.size === 0) {
            container.innerHTML = '<div class="empty-state">No nodes</div>';
            return;
        }

        var html = '';
        state.nodes.forEach(function(node, mac) {
            var role = state.roles.get(mac) || node.role || 'rx';
            var healthScore = node.health_score || 0;
            var healthDisplay = healthScore > 0 ? (healthScore * 100).toFixed(0) + '%' : '--';
            var isOnline = node.online || false;

            html += '<div class="fleet-role-item">' +
                '<span class="node-mac">' + formatMAC(mac) + '</span>' +
                '<span>' +
                '<span class="node-role ' + role + '">' + role + '</span>' +
                '<span class="health-score">' + healthDisplay + '</span>' +
                '</span>' +
                (isOnline ? '<button class="fleet-identify-btn" onclick="FleetPanel.identifyNode(\'' + mac + '\')" title="Identify (blink LED)">⚡</button>' : '') +
                '</div>';
        });
        container.innerHTML = html;
    }

    function updateHistoryUI() {
        var container = document.getElementById('fleet-history-list');
        if (!container) return;

        if (state.history.length === 0) {
            container.innerHTML = '<div class="empty-state">No events</div>';
            return;
        }

        var html = '';
        state.history.forEach(function(event) {
            var time = new Date(event.timestamp_ms);
            var timeStr = time.toLocaleTimeString();
            var delta = event.coverage_delta || 0;
            var deltaClass = delta >= 0 ? 'positive' : 'negative';
            var deltaSign = delta >= 0 ? '+' : '';

            html += '<div class="fleet-history-item" data-event-id="' + event.id + '">' +
                '<span class="history-time">' + timeStr + '</span>' +
                '<span class="history-trigger">' + (event.trigger_reason || 'unknown') + '</span>' +
                '<span class="history-delta ' + deltaClass + '">' + deltaSign + (delta * 100).toFixed(0) + '%</span>' +
                '</div>';
        });
        container.innerHTML = html;

        // Add click handlers for comparison view
        container.querySelectorAll('.fleet-history-item').forEach(function(el) {
            el.addEventListener('click', function() {
                var eventId = el.dataset.eventId;
                showEventComparison(eventId);
            });
        });
    }

    function updateSimulateSelect() {
        var select = document.getElementById('fleet-simulate-select');
        if (!select) return;

        var currentValue = select.value;
        var html = '<option value="">Select a node...</option>';

        state.nodes.forEach(function(node, mac) {
            if (node.online) {
                html += '<option value="' + mac + '">' + formatMAC(mac) + '</option>';
            }
        });
        select.innerHTML = html;
        select.value = currentValue;
    }

    function formatMAC(mac) {
        // Abbreviate MAC for display
        var parts = mac.split(':');
        if (parts.length >= 6) {
            return parts.slice(3, 6).join(':');
        }
        return mac;
    }

    // ============================================
    // Warning Display
    // ============================================
    function showWarning(message) {
        state.warningMessage = message;

        var warningEl = document.getElementById('fleet-warning');
        var textEl = document.getElementById('fleet-warning-text');
        if (warningEl && textEl) {
            textEl.textContent = message;
            warningEl.classList.remove('hidden');
        }
    }

    function dismissWarning() {
        state.warningMessage = null;
        var warningEl = document.getElementById('fleet-warning');
        if (warningEl) {
            warningEl.classList.add('hidden');
        }
    }

    function showComparisonButton(event) {
        // Add a "View Impact" button next to the warning
        var warningEl = document.getElementById('fleet-warning');
        if (!warningEl) return;

        var existingBtn = document.getElementById('fleet-compare-btn');
        if (existingBtn) existingBtn.remove();

        var btn = document.createElement('button');
        btn.id = 'fleet-compare-btn';
        btn.className = 'btn btn-sm';
        btn.textContent = 'View Impact';
        btn.style.cssText = 'margin-left: 0.5rem; padding: 0.15rem 0.4rem; font-size: 0.7rem; background: #444; border: 1px solid #666; color: #ddd; cursor: pointer; border-radius: 3px;';
        btn.addEventListener('click', function() {
            showGDOPComparison(event);
        });

        warningEl.appendChild(btn);
    }

    // ============================================
    // GDOP Comparison View
    // ============================================
    function showGDOPComparison(event) {
        // Remove existing overlay
        var existing = document.querySelector('.gdop-compare-overlay');
        if (existing) existing.remove();

        var overlay = document.createElement('div');
        overlay.className = 'gdop-compare-overlay';
        overlay.innerHTML = `
            <div class="gdop-compare-content">
                <div class="gdop-compare-header">
                    <h3>Coverage Impact: ${event.trigger_reason || 'Re-optimisation'}</h3>
                    <button class="gdop-compare-close">&times;</button>
                </div>
                <div class="gdop-maps">
                    <div class="gdop-map-container">
                        <h4>Before (${(event.coverage_before * 100).toFixed(0)}% coverage)</h4>
                        <canvas id="gdop-before-canvas" class="gdop-map-canvas"></canvas>
                    </div>
                    <div class="gdop-map-container">
                        <h4>After (${(event.coverage_after * 100).toFixed(0)}% coverage)</h4>
                        <canvas id="gdop-after-canvas" class="gdop-map-canvas"></canvas>
                    </div>
                </div>
                <div class="gdop-slider-container">
                    <label>Blend: <input type="range" id="gdop-blend-slider" class="gdop-slider" min="0" max="100" value="50"></label>
                </div>
            </div>
        `;

        document.body.appendChild(overlay);

        // Close handlers
        overlay.querySelector('.gdop-compare-close').addEventListener('click', function() {
            overlay.remove();
        });
        overlay.addEventListener('click', function(e) {
            if (e.target === overlay) overlay.remove();
        });

        // Draw GDOP maps
        if (event.gdop_before && event.gdop_after) {
            drawGDOPMap('gdop-before-canvas', event.gdop_before, event.gdop_cols, event.gdop_rows);
            drawGDOPMap('gdop-after-canvas', event.gdop_after, event.gdop_cols, event.gdop_rows);
        }

        // Blend slider
        var slider = document.getElementById('gdop-blend-slider');
        if (slider) {
            slider.addEventListener('input', function() {
                updateBlend(slider.value);
            });
        }
    }

    function drawGDOPMap(canvasId, data, cols, rows) {
        var canvas = document.getElementById(canvasId);
        if (!canvas || !data || !cols || !rows) return;

        var ctx = canvas.getContext('2d');
        var rect = canvas.getBoundingClientRect();
        canvas.width = rect.width * window.devicePixelRatio;
        canvas.height = rect.height * window.devicePixelRatio;
        ctx.scale(window.devicePixelRatio, window.devicePixelRatio);

        var cellWidth = rect.width / cols;
        var cellHeight = rect.height / rows;

        for (var row = 0; row < rows; row++) {
            for (var col = 0; col < cols; col++) {
                var idx = row * cols + col;
                var gdop = data[idx] || 10;
                var color = gdopToColor(gdop);
                ctx.fillStyle = color;
                ctx.fillRect(col * cellWidth, row * cellHeight, cellWidth, cellHeight);
            }
        }
    }

    function gdopToColor(gdop) {
        // GDOP < 2 = green, 2-5 = yellow, > 5 = red
        var t = Math.min(1, Math.max(0, (gdop - 1) / 5));
        if (gdop < 2) {
            // Green to yellow
            var g = 1;
            var r = t * 2;
            return 'rgb(' + Math.floor(r * 255) + ',' + Math.floor(g * 255) + ',0)';
        } else {
            // Yellow to red
            var r = 1;
            var g = Math.max(0, 1 - (t - 0.4) * 1.5);
            return 'rgb(' + Math.floor(r * 255) + ',' + Math.floor(g * 200) + ',0)';
        }
    }

    function updateBlend(value) {
        // Adjust opacity of the after map
        var afterCanvas = document.getElementById('gdop-after-canvas');
        if (afterCanvas) {
            afterCanvas.style.opacity = value / 100;
        }
    }

    function showEventComparison(eventId) {
        // Find the event in history
        var event = state.history.find(function(e) {
            return String(e.id) === String(eventId);
        });
        if (event && event.gdop_before && event.gdop_after) {
            showGDOPComparison(event);
        }
    }

    // ============================================
    // Actions
    // ============================================
    function onOptimiseClick() {
        var btn = document.getElementById('fleet-optimise-btn');
        if (btn) btn.disabled = true;

        fetch('/api/fleet/optimise', { method: 'POST' })
            .then(function(res) { return res.json(); })
            .then(function(data) {
                if (btn) btn.disabled = false;
                if (window.SpaxelApp && SpaxelApp.showToast) {
                    SpaxelApp.showToast('Fleet optimised: ' + (data.trigger_reason || 'manual'), 'success');
                }
                handleFleetHealth(data);
            })
            .catch(function(err) {
                if (btn) btn.disabled = false;
                console.error('[Fleet] Optimise failed:', err);
                if (window.SpaxelApp && SpaxelApp.showToast) {
                    SpaxelApp.showToast('Optimisation failed', 'error');
                }
            });
    }

    function onSimulateSelect(e) {
        var mac = e.target.value;
        var resultEl = document.getElementById('fleet-simulate-result');

        if (!mac) {
            if (resultEl) resultEl.classList.add('hidden');
            return;
        }

        fetch('/api/fleet/simulate?mac=' + encodeURIComponent(mac))
            .then(function(res) { return res.json(); })
            .then(function(data) {
                if (!resultEl) return;
                resultEl.classList.remove('hidden');

                var delta = data.coverage_delta || 0;
                var deltaAbs = Math.abs(delta * 100).toFixed(0);
                var direction = delta < 0 ? 'drop' : 'increase';
                var className = delta < -0.1 ? 'degraded' : 'ok';

                resultEl.className = 'simulate-result ' + className;
                resultEl.innerHTML =
                    '<strong>Coverage ' + direction + ':</strong> ' + deltaAbs + '%' +
                    '<br><small>New coverage: ' + ((data.coverage_after || 0) * 100).toFixed(0) + '%</small>';
            })
            .catch(function(err) {
                console.error('[Fleet] Simulate failed:', err);
                if (resultEl) {
                    resultEl.classList.remove('hidden');
                    resultEl.className = 'simulate-result';
                    resultEl.textContent = 'Simulation failed';
                }
            });
    }

    // ============================================
    // Public API
    // ============================================
    window.FleetPanel = {
        init: init,
        handleFleetChange: handleFleetChange,
        handleFleetHealth: handleFleetHealth,
        showWarning: showWarning,
        dismissWarning: dismissWarning,
        getState: function() { return state; },
        identifyNode: identifyNode
    };

    // ============================================
    // Identify Node Action
    // ============================================
    function identifyNode(mac, durationMs) {
        var payload = durationMs ? JSON.stringify({ duration_ms: durationMs }) : JSON.stringify({});

        fetch('/api/nodes/' + mac + '/identify', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: payload
        })
        .then(function(res) {
            if (!res.ok) {
                throw new Error('Identify failed: ' + res.status);
            }
            return res.json();
        })
        .then(function(data) {
            if (window.SpaxelApp && window.SpaxelApp.showToast) {
                window.SpaxelApp.showToast('Identify command sent to ' + mac, 'success');
            }
        })
        .catch(function(err) {
            console.error('[Fleet] Identify error:', err);
            if (window.SpaxelApp && window.SpaxelApp.showToast) {
                window.SpaxelApp.showToast('Failed to identify ' + mac + ': ' + err.message, 'error');
            }
        });
    }

    // Auto-init when DOM is ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }
})();
