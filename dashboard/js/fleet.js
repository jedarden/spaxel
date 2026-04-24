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

    // Full table view state
    let isFullTableView = false;
    let selectedNodes = new Set();
    let sortColumn = null;
    let sortDirection = 'asc';

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
        // Check if panel already exists
        if (document.getElementById('fleet-panel')) {
            return;
        }

        // Create the fleet panel container (fixed position panel)
        const panel = document.createElement('div');
        panel.id = 'fleet-panel';
        panel.className = 'fleet-health-panel';
        panel.style.cssText = 'position: fixed; top: 60px; left: 20px; width: 280px; max-height: calc(100vh - 80px); background: rgba(0, 0, 0, 0.8); border-radius: 8px; padding: 12px; z-index: 100; overflow-y: auto; border: 1px solid rgba(255, 255, 255, 0.1);';
        panel.innerHTML = `
            <div class="panel-header">
                <h3>Fleet Health</h3>
                <div style="display: flex; gap: 8px;">
                    <button id="fleet-full-view-btn" class="btn btn-sm" title="Open full table view">
                        <span class="icon">&#x26F6;</span> Full View
                    </button>
                    <button id="fleet-optimise-btn" class="btn btn-sm" title="Re-optimise roles now">
                        <span class="icon">&#x21BB;</span> Optimise
                    </button>
                </div>
            </div>
            <div class="panel-content">
                <div id="fleet-warning" class="fleet-warning hidden">
                    <span class="warning-icon">&#x26A0;</span>
                    <span id="fleet-warning-text"></span>
                    <button id="fleet-warning-dismiss" class="btn-dismiss">&times;</button>
                </div>

                <div id="fleet-unpaired-banner" class="fleet-unpaired-banner" style="display:none"></div>

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
                    <select id="fleet-simulate-select" class="select-sm" aria-label="Select a node to simulate removal">
                        <option value="">Select a node...</option>
                    </select>
                    <div id="fleet-simulate-result" class="simulate-result hidden"></div>
                </div>
            </div>
        `;

        document.body.appendChild(panel);

        // Add event handlers
        document.getElementById('fleet-optimise-btn').addEventListener('click', onOptimiseClick);
        document.getElementById('fleet-full-view-btn').addEventListener('click', toggleFullTableView);
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

            /* Full Table View Styles */
            .fleet-table-overlay {
                position: fixed;
                top: 0;
                left: 0;
                right: 0;
                bottom: 0;
                background: rgba(0, 0, 0, 0.85);
                z-index: 1000;
                display: flex;
                align-items: center;
                justify-content: center;
                backdrop-filter: blur(4px);
            }

            .fleet-table-container {
                background: #1a1a2e;
                border-radius: 12px;
                box-shadow: 0 8px 32px rgba(0, 0, 0, 0.5);
                width: 90vw;
                max-width: 1200px;
                max-height: 85vh;
                display: flex;
                flex-direction: column;
                border: 1px solid rgba(255, 255, 255, 0.1);
            }

            .fleet-table-header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                padding: 20px 24px;
                border-bottom: 1px solid rgba(255, 255, 255, 0.1);
            }

            .fleet-table-header h2 {
                margin: 0;
                font-size: 20px;
                color: #eee;
            }

            .fleet-table-actions {
                display: flex;
                gap: 8px;
            }

            .fleet-btn {
                padding: 8px 16px;
                border-radius: 6px;
                font-size: 13px;
                font-weight: 500;
                cursor: pointer;
                border: none;
                transition: background 0.2s;
                display: flex;
                align-items: center;
                gap: 6px;
            }

            .fleet-btn .icon {
                font-size: 14px;
            }

            .fleet-btn-primary {
                background: #4fc3f7;
                color: #1a1a2e;
            }

            .fleet-btn-primary:hover {
                background: #29b6f6;
            }

            .fleet-btn-secondary {
                background: rgba(255, 255, 255, 0.1);
                color: #ccc;
                border: 1px solid rgba(255, 255, 255, 0.15);
            }

            .fleet-btn-secondary:hover {
                background: rgba(255, 255, 255, 0.15);
            }

            .fleet-btn-action {
                background: rgba(76, 175, 80, 0.2);
                color: #66bb6a;
                border: 1px solid rgba(76, 175, 80, 0.4);
            }

            .fleet-btn-action:hover:not(:disabled) {
                background: rgba(76, 175, 80, 0.3);
            }

            .fleet-btn:disabled {
                opacity: 0.5;
                cursor: not-allowed;
            }

            .fleet-actions-divider {
                width: 1px;
                height: 24px;
                background: rgba(255, 255, 255, 0.15);
                margin: 0 4px;
            }

            .fleet-table-toolbar {
                display: flex;
                justify-content: space-between;
                align-items: center;
                padding: 16px 24px;
                background: rgba(0, 0, 0, 0.2);
                border-bottom: 1px solid rgba(255, 255, 255, 0.1);
                flex-wrap: wrap;
                gap: 12px;
            }

            .fleet-selection-info {
                display: flex;
                align-items: center;
                gap: 8px;
                font-size: 13px;
                color: #ccc;
            }

            .fleet-checkbox {
                width: 16px;
                height: 16px;
                accent-color: #4fc3f7;
                cursor: pointer;
            }

            .fleet-filters {
                display: flex;
                gap: 8px;
                flex-wrap: wrap;
            }

            .fleet-select {
                padding: 6px 10px;
                background: rgba(255, 255, 255, 0.08);
                border: 1px solid rgba(255, 255, 255, 0.15);
                border-radius: 4px;
                color: #ddd;
                font-size: 12px;
                cursor: pointer;
            }

            .fleet-select:focus {
                outline: none;
                border-color: #4fc3f7;
            }

            .fleet-search-input {
                padding: 6px 10px;
                background: rgba(255, 255, 255, 0.08);
                border: 1px solid rgba(255, 255, 255, 0.15);
                border-radius: 4px;
                color: #ddd;
                font-size: 12px;
                width: 180px;
            }

            .fleet-search-input:focus {
                outline: none;
                border-color: #4fc3f7;
            }

            .fleet-table-wrapper {
                flex: 1;
                overflow: auto;
                padding: 0;
            }

            .fleet-table {
                width: 100%;
                border-collapse: collapse;
                font-size: 13px;
            }

            .fleet-table thead {
                position: sticky;
                top: 0;
                background: #1e1e3a;
                z-index: 1;
            }

            .fleet-table th {
                padding: 12px 16px;
                text-align: left;
                font-weight: 600;
                color: #888;
                text-transform: uppercase;
                font-size: 11px;
                letter-spacing: 0.5px;
                border-bottom: 1px solid rgba(255, 255, 255, 0.1);
                white-space: nowrap;
            }

            .fleet-table th.sortable {
                cursor: pointer;
                user-select: none;
            }

            .fleet-table th.sortable:hover {
                background: rgba(255, 255, 255, 0.05);
            }

            .fleet-table th.sort-asc::after {
                content: ' ▲';
                color: #4fc3f7;
            }

            .fleet-table th.sort-desc::after {
                content: ' ▼';
                color: #4fc3f7;
            }

            .fleet-table td {
                padding: 12px 16px;
                border-bottom: 1px solid rgba(255, 255, 255, 0.05);
            }

            .fleet-table tbody tr {
                transition: background 0.2s;
            }

            .fleet-table tbody tr:hover {
                background: rgba(255, 255, 255, 0.05);
            }

            .fleet-table tbody tr.selected {
                background: rgba(79, 195, 247, 0.15);
            }

            .fleet-select-col {
                width: 40px;
                text-align: center;
            }

            .fleet-mac-col {
                font-family: monospace;
                color: #4fc3f7;
            }

            .node-mac-full {
                font-size: 13px;
            }

            .node-name {
                font-size: 11px;
                color: #888;
                font-family: inherit;
            }

            .node-role-badge {
                padding: 3px 8px;
                border-radius: 3px;
                font-size: 10px;
                font-weight: 600;
                text-transform: uppercase;
            }

            .node-role-badge.tx { background: rgba(239, 68, 68, 0.3); color: #fca5a5; }
            .node-role-badge.rx { background: rgba(59, 130, 246, 0.3); color: #93c5fd; }
            .node-role-badge.tx_rx { background: rgba(168, 85, 247, 0.3); color: #d8b4fe; }
            .node-role-badge.passive { background: rgba(107, 114, 128, 0.3); color: #d1d5db; }

            .node-status-badge {
                padding: 3px 8px;
                border-radius: 3px;
                font-size: 11px;
                font-weight: 500;
            }

            .node-status-badge.online {
                background: rgba(76, 175, 80, 0.2);
                color: #66bb6a;
            }

            .node-status-badge.offline {
                background: rgba(244, 67, 54, 0.2);
                color: #e57373;
            }

            .node-status-badge.unpaired {
                background: rgba(251, 191, 36, 0.2);
                color: #fbbf24;
            }

            .node-unpaired-badge {
                display: inline-block;
                padding: 1px 6px;
                border-radius: 3px;
                font-size: 10px;
                font-weight: 600;
                background: rgba(251, 191, 36, 0.2);
                color: #fbbf24;
                border: 1px solid rgba(251, 191, 36, 0.4);
                margin-left: 6px;
                vertical-align: middle;
            }

            .fleet-action-btn.reprovision {
                color: #fbbf24;
                border-color: rgba(251, 191, 36, 0.4);
            }

            .fleet-unpaired-banner {
                background: rgba(251, 191, 36, 0.1);
                border: 1px solid rgba(251, 191, 36, 0.3);
                border-radius: 6px;
                padding: 8px 12px;
                margin-bottom: 8px;
                font-size: 12px;
                color: #fbbf24;
            }

            .health-bar-wrapper {
                display: flex;
                align-items: center;
                gap: 8px;
                min-width: 100px;
            }

            .health-bar {
                height: 6px;
                border-radius: 3px;
                background: rgba(255, 255, 255, 0.1);
                overflow: hidden;
                flex: 1;
            }

            .health-bar-good {
                background: linear-gradient(90deg, #22c55e, #66bb6a);
            }

            .health-bar-fair {
                background: linear-gradient(90deg, #eab308, #f59e0b);
            }

            .health-bar-poor {
                background: linear-gradient(90deg, #ef4444, #dc2626);
            }

            .health-text {
                font-size: 11px;
                color: #ccc;
                min-width: 35px;
                text-align: right;
            }

            .fleet-uptime-col {
                font-family: monospace;
                color: #aaa;
                font-size: 12px;
            }

            .fleet-fw-col {
                font-family: monospace;
                color: #888;
                font-size: 11px;
            }

            .fleet-actions-col {
                white-space: nowrap;
            }

            .fleet-position-col {
                font-family: monospace;
                font-size: 12px;
                color: #aaa;
            }

            .position-link {
                color: #4fc3f7;
                cursor: pointer;
                text-decoration: none;
                transition: color 0.2s;
            }

            .position-link:hover {
                color: #29b6f6;
                text-decoration: underline;
            }

            .fleet-action-btn {
                background: none;
                border: none;
                color: #888;
                cursor: pointer;
                padding: 4px;
                font-size: 14px;
                transition: color 0.2s, background 0.2s;
                border-radius: 3px;
            }

            .fleet-action-btn:hover {
                color: #4fc3f7;
                background: rgba(79, 195, 247, 0.1);
            }

            .fleet-empty-state {
                text-align: center;
                padding: 40px 20px;
                color: #666;
                font-size: 14px;
            }

            .fleet-table-footer {
                padding: 16px 24px;
                background: rgba(0, 0, 0, 0.2);
                border-top: 1px solid rgba(255, 255, 255, 0.1);
            }

            .fleet-stats-summary {
                display: flex;
                gap: 24px;
                font-size: 13px;
                color: #888;
            }

            .fleet-stats-summary strong {
                color: #ddd;
            }

            /* Diagnostics Modal */
            .fleet-diagnostics-modal {
                position: fixed;
                top: 0;
                left: 0;
                right: 0;
                bottom: 0;
                background: rgba(0, 0, 0, 0.8);
                z-index: 1100;
                display: flex;
                align-items: center;
                justify-content: center;
                backdrop-filter: blur(4px);
            }

            .fleet-diagnostics-content {
                background: #1a1a2e;
                border-radius: 12px;
                width: 90%;
                max-width: 500px;
                border: 1px solid rgba(255, 255, 255, 0.1);
                box-shadow: 0 8px 32px rgba(0, 0, 0, 0.5);
            }

            .fleet-diagnostics-header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                padding: 16px 20px;
                border-bottom: 1px solid rgba(255, 255, 255, 0.1);
            }

            .fleet-diagnostics-header h3 {
                margin: 0;
                font-size: 16px;
                color: #eee;
            }

            .fleet-close-modal {
                background: none;
                border: none;
                color: #888;
                font-size: 24px;
                cursor: pointer;
                padding: 0;
                width: 28px;
                height: 28px;
                display: flex;
                align-items: center;
                justify-content: center;
                border-radius: 4px;
            }

            .fleet-close-modal:hover {
                background: rgba(255, 255, 255, 0.1);
                color: #ccc;
            }

            .fleet-diagnostics-body {
                padding: 20px;
                max-height: 60vh;
                overflow-y: auto;
            }

            .diagnostics-section {
                margin-bottom: 20px;
            }

            .diagnostics-section:last-child {
                margin-bottom: 0;
            }

            .diagnostics-section h4 {
                margin: 0 0 12px 0;
                font-size: 13px;
                color: #888;
                text-transform: uppercase;
                letter-spacing: 0.5px;
            }

            .diagnostics-table {
                width: 100%;
                font-size: 13px;
            }

            .diagnostics-table td {
                padding: 6px 0;
                color: #ccc;
            }

            .diagnostics-table td:first-child {
                color: #888;
                font-weight: 500;
                padding-right: 16px;
            }

            .fleet-diagnostics-actions {
                display: flex;
                gap: 8px;
                padding: 16px 20px;
                border-top: 1px solid rgba(255, 255, 255, 0.1);
                background: rgba(0, 0, 0, 0.2);
            }

            .fleet-diagnostics-actions .fleet-btn {
                flex: 1;
                justify-content: center;
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

        // Update unpaired banner
        var unpairedCount = 0;
        state.nodes.forEach(function(node) { if (node.unpaired) unpairedCount++; });
        var bannerEl = document.getElementById('fleet-unpaired-banner');
        if (bannerEl) {
            if (unpairedCount > 0) {
                bannerEl.textContent = '⚠ ' + unpairedCount + ' node' + (unpairedCount > 1 ? 's' : '') + ' connected without credentials — re-provision to pair.';
                bannerEl.style.display = '';
            } else {
                bannerEl.style.display = 'none';
            }
        }
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
                '<span class="node-mac">' + formatMAC(mac) + (node.unpaired ? ' <span title="Unpaired — needs re-provisioning">⚠</span>' : '') + '</span>' +
                '<span>' +
                '<span class="node-role ' + role + '">' + role + '</span>' +
                '<span class="health-score">' + healthDisplay + '</span>' +
                '</span>' +
                (isOnline && !node.unpaired ? '<button class="fleet-identify-btn" onclick="FleetPanel.identifyNode(\'' + mac + '\')" title="Identify (blink LED)">⚡</button>' : '') +
                (node.unpaired ? '<button class="fleet-identify-btn" onclick="FleetPanel.reproveNode(\'' + mac + '\')" title="Re-provision credentials" style="color:#fbbf24">↺</button>' : '') +
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
    // Full Table View
    // ============================================
    function toggleFullTableView() {
        isFullTableView = !isFullTableView;

        if (isFullTableView) {
            showFullTableView();
        } else {
            hideFullTableView();
        }
    }

    function showFullTableView() {
        // Create overlay
        var overlay = document.createElement('div');
        overlay.id = 'fleet-table-overlay';
        overlay.className = 'fleet-table-overlay';
        overlay.innerHTML = `
            <div class="fleet-table-container">
                <div class="fleet-table-header">
                    <h2>Fleet Management</h2>
                    <div class="fleet-table-actions">
                        <button id="fleet-refresh-btn" class="fleet-btn fleet-btn-secondary">
                            <span class="icon">&#x21BB;</span> Refresh
                        </button>
                        <button id="fleet-bulk-identify-btn" class="fleet-btn fleet-btn-action" disabled>
                            <span class="icon">&#x26A1;</span> Identify Selected
                        </button>
                        <button id="fleet-bulk-restart-btn" class="fleet-btn fleet-btn-action" disabled>
                            <span class="icon">&#x21BB;</span> Restart Selected
                        </button>
                        <button id="fleet-update-all-btn" class="fleet-btn fleet-btn-action">
                            <span class="icon">&#x2191;</span> Update All
                        </button>
                        <button id="fleet-rebaseline-all-btn" class="fleet-btn fleet-btn-action">
                            <span class="icon">&#x267B;</span> Re-baseline All
                        </button>
                        <div class="fleet-actions-divider"></div>
                        <button id="fleet-export-btn" class="fleet-btn fleet-btn-secondary">
                            <span class="icon">&#x2193;</span> Export
                        </button>
                        <button id="fleet-import-btn" class="fleet-btn fleet-btn-secondary">
                            <span class="icon">&#x2191;</span> Import
                        </button>
                        <button id="fleet-close-table-btn" class="fleet-btn fleet-btn-secondary">
                            <span class="icon">&times;</span> Close
                        </button>
                    </div>
                </div>

                <div class="fleet-table-toolbar">
                    <div class="fleet-selection-info">
                        <input type="checkbox" id="fleet-select-all" class="fleet-checkbox">
                        <label for="fleet-select-all">
                            <span id="fleet-selected-count">0</span> of <span id="fleet-total-count">0</span> nodes selected
                        </label>
                    </div>
                    <div class="fleet-filters">
                        <select id="fleet-filter-role" class="fleet-select">
                            <option value="">All Roles</option>
                            <option value="tx">TX Only</option>
                            <option value="rx">RX Only</option>
                            <option value="tx_rx">TX/RX</option>
                            <option value="passive">Passive</option>
                        </select>
                        <select id="fleet-filter-status" class="fleet-select">
                            <option value="">All Status</option>
                            <option value="online">Online</option>
                            <option value="offline">Offline</option>
                        </select>
                        <input type="text" id="fleet-search" class="fleet-search-input" placeholder="Search MAC or name...">
                    </div>
                </div>

                <div class="fleet-table-wrapper">
                    <table class="fleet-table">
                        <thead>
                            <tr>
                                <th class="fleet-select-col">
                                    <input type="checkbox" id="fleet-header-select-all" class="fleet-checkbox">
                                </th>
                                <th class="sortable" data-sort="mac">MAC Address</th>
                                <th class="sortable" data-sort="role">Role</th>
                                <th class="sortable" data-sort="status">Status</th>
                                <th class="sortable" data-sort="position">Position</th>
                                <th class="sortable" data-sort="health">Health</th>
                                <th class="sortable" data-sort="uptime">Uptime</th>
                                <th class="sortable" data-sort="fw">Firmware</th>
                                <th>Actions</th>
                            </tr>
                        </thead>
                        <tbody id="fleet-table-body">
                            <tr>
                                <td colspan="9" class="fleet-empty-state">Loading...</td>
                            </tr>
                        </tbody>
                    </table>
                </div>

                <div class="fleet-table-footer">
                    <div class="fleet-stats-summary">
                        <span>Total: <strong id="fleet-stat-total">0</strong></span>
                        <span>Online: <strong id="fleet-stat-online">0</strong></span>
                        <span>Coverage: <strong id="fleet-stat-coverage">--%</strong></span>
                        <span>Mean GDOP: <strong id="fleet-stat-gdop">--</strong></span>
                    </div>
                </div>
            </div>
        `;

        document.body.appendChild(overlay);

        // Add event listeners
        document.getElementById('fleet-close-table-btn').addEventListener('click', hideFullTableView);
        document.getElementById('fleet-refresh-btn').addEventListener('click', refreshFullTable);
        document.getElementById('fleet-bulk-identify-btn').addEventListener('click', bulkIdentify);
        document.getElementById('fleet-bulk-restart-btn').addEventListener('click', bulkRestart);
        document.getElementById('fleet-update-all-btn').addEventListener('click', updateAllNodes);
        document.getElementById('fleet-rebaseline-all-btn').addEventListener('click', rebaselineAllNodes);
        document.getElementById('fleet-export-btn').addEventListener('click', exportConfig);
        document.getElementById('fleet-import-btn').addEventListener('click', importConfig);
        document.getElementById('fleet-select-all').addEventListener('change', toggleSelectAll);
        document.getElementById('fleet-header-select-all').addEventListener('change', toggleSelectAll);
        document.getElementById('fleet-filter-role').addEventListener('change', renderFullTable);
        document.getElementById('fleet-filter-status').addEventListener('change', renderFullTable);
        document.getElementById('fleet-search').addEventListener('input', renderFullTable);

        // Add sort handlers
        overlay.querySelectorAll('th.sortable').forEach(function(th) {
            th.addEventListener('click', function() {
                var column = th.dataset.sort;
                handleSort(column);
            });
        });

        // Populate and render
        renderFullTable();
    }

    function hideFullTableView() {
        isFullTableView = false;
        var overlay = document.getElementById('fleet-table-overlay');
        if (overlay) {
            overlay.remove();
        }
        selectedNodes.clear();
    }

    function renderFullTable() {
        var tbody = document.getElementById('fleet-table-body');
        if (!tbody) return;

        var filterRole = document.getElementById('fleet-filter-role').value;
        var filterStatus = document.getElementById('fleet-filter-status').value;
        var searchTerm = document.getElementById('fleet-search').value.toLowerCase();

        var nodes = Array.from(state.nodes.values());

        // Apply filters
        nodes = nodes.filter(function(node) {
            if (filterRole && node.role !== filterRole) return false;
            if (filterStatus === 'online' && !node.online) return false;
            if (filterStatus === 'offline' && node.online) return false;
            if (searchTerm && !node.mac.toLowerCase().includes(searchTerm) &&
                !(node.name && node.name.toLowerCase().includes(searchTerm))) {
                return false;
            }
            return true;
        });

        // Apply sort
        if (sortColumn) {
            nodes.sort(function(a, b) {
                var aVal = getNodeSortValue(a, sortColumn);
                var bVal = getNodeSortValue(b, sortColumn);
                if (sortDirection === 'asc') {
                    return aVal > bVal ? 1 : aVal < bVal ? -1 : 0;
                } else {
                    return aVal < bVal ? 1 : aVal > bVal ? -1 : 0;
                }
            });
        }

        // Update stats
        var totalNodes = state.nodes.size;
        var onlineNodes = Array.from(state.nodes.values()).filter(function(n) { return n.online; }).length;

        document.getElementById('fleet-total-count').textContent = totalNodes;
        document.getElementById('fleet-selected-count').textContent = selectedNodes.size;
        document.getElementById('fleet-stat-total').textContent = totalNodes;
        document.getElementById('fleet-stat-online').textContent = onlineNodes;
        document.getElementById('fleet-stat-coverage').textContent = (state.coverageScore * 100).toFixed(0) + '%';
        document.getElementById('fleet-stat-gdop').textContent = state.meanGDOP.toFixed(2);

        // Update bulk button state
        var bulkBtn = document.getElementById('fleet-bulk-identify-btn');
        if (bulkBtn) {
            bulkBtn.disabled = selectedNodes.size === 0;
        }

        // Render rows
        if (nodes.length === 0) {
            tbody.innerHTML = '<tr><td colspan="9" class="fleet-empty-state">No nodes match the current filters</td></tr>';
            return;
        }

        var html = '';
        nodes.forEach(function(node) {
            var isSelected = selectedNodes.has(node.mac);
            var healthScore = node.health_score || 0;
            var healthPercent = (healthScore * 100).toFixed(0);
            var healthClass = healthScore > 0.7 ? 'good' : healthScore > 0.4 ? 'fair' : 'poor';
            var uptime = node.uptime_seconds || 0;
            var uptimeStr = formatUptime(uptime);
            var firmware = node.firmware_version || '--';
            var statusClass = node.unpaired ? 'unpaired' : (node.online ? 'online' : 'offline');
            var statusText = node.unpaired ? 'Unpaired' : (node.online ? 'Online' : 'Offline');

            html += '<tr class="fleet-row' + (isSelected ? ' selected' : '') + '" data-mac="' + node.mac + '">' +
                '<td class="fleet-select-col">' +
                '<input type="checkbox" class="fleet-checkbox fleet-row-checkbox" ' +
                (isSelected ? 'checked ' : '') + 'data-mac="' + node.mac + '">' +
                '</td>' +
                '<td class="fleet-mac-col">' +
                '<span class="node-mac-full">' + node.mac + '</span>' +
                (node.unpaired ? '<span class="node-unpaired-badge">UNPAIRED</span>' : '') +
                (node.name ? '<br><span class="node-name">' + node.name + '</span>' : '') +
                '</td>' +
                '<td><span class="node-role-badge ' + node.role + '">' + node.role + '</span></td>' +
                '<td><span class="node-status-badge ' + statusClass + '">' + statusText + '</span></td>' +
                '<td class="fleet-position-col">' +
                '<span class="position-link" data-mac="' + node.mac + '" title="Click to fly to node">' +
                formatPosition(node.pos_x, node.pos_y, node.pos_z) +
                '</span>' +
                '</td>' +
                '<td>' +
                '<div class="health-bar-wrapper">' +
                '<div class="health-bar health-bar-' + healthClass + '" style="width: ' + healthPercent + '%"></div>' +
                '<span class="health-text">' + healthPercent + '%</span>' +
                '</div>' +
                '</td>' +
                '<td class="fleet-uptime-col">' + uptimeStr + '</td>' +
                '<td class="fleet-fw-col">' + firmware + '</td>' +
                '<td class="fleet-actions-col">' +
                '<button class="fleet-action-btn" data-action="flyto" data-mac="' + node.mac + '" title="Fly camera to node">&#x26F6;</button>' +
                '<button class="fleet-action-btn" data-action="identify" data-mac="' + node.mac + '" title="Identify (blink LED)">&#x26A1;</button>' +
                '<button class="fleet-action-btn" data-action="diagnostics" data-mac="' + node.mac + '" title="View diagnostics">&#x2699;</button>' +
                (node.unpaired ? '<button class="fleet-action-btn reprovision" data-action="reprovision" data-mac="' + node.mac + '" title="Re-provision credentials">&#x21BA;</button>' : '') +
                '</td>' +
                '</tr>';
        });

        tbody.innerHTML = html;

        // Add row checkbox handlers
        tbody.querySelectorAll('.fleet-row-checkbox').forEach(function(checkbox) {
            checkbox.addEventListener('change', function() {
                var mac = this.dataset.mac;
                if (this.checked) {
                    selectedNodes.add(mac);
                } else {
                    selectedNodes.delete(mac);
                }
                renderFullTable(); // Re-render to update selection state
            });
        });

        // Add action button handlers
        tbody.querySelectorAll('.fleet-action-btn').forEach(function(btn) {
            btn.addEventListener('click', function() {
                var action = this.dataset.action;
                var mac = this.dataset.mac;
                handleNodeAction(action, mac);
            });
        });

        // Add position link click handlers
        tbody.querySelectorAll('.position-link').forEach(function(link) {
            link.addEventListener('click', function(e) {
                e.preventDefault();
                e.stopPropagation();
                var mac = this.dataset.mac;
                flyToNode(mac);
            });
        });

        // Add row click handler for selection
        tbody.querySelectorAll('.fleet-row').forEach(function(row) {
            row.addEventListener('dblclick', function() {
                var mac = row.dataset.mac;
                handleNodeAction('flyto', mac);
            });
        });
    }

    function getNodeSortValue(node, column) {
        switch (column) {
            case 'mac': return node.mac || '';
            case 'role': return node.role || '';
            case 'status': return node.online ? 1 : 0;
            case 'position': return (node.pos_x || 0) + (node.pos_y || 0) + (node.pos_z || 0);
            case 'health': return node.health_score || 0;
            case 'uptime': return node.uptime_seconds || 0;
            case 'fw': return node.firmware_version || '';
            default: return '';
        }
    }

    function handleSort(column) {
        if (sortColumn === column) {
            sortDirection = sortDirection === 'asc' ? 'desc' : 'asc';
        } else {
            sortColumn = column;
            sortDirection = 'asc';
        }

        // Update sort indicators
        document.querySelectorAll('th.sortable').forEach(function(th) {
            th.classList.remove('sort-asc', 'sort-desc');
            if (th.dataset.sort === column) {
                th.classList.add('sort-' + sortDirection);
            }
        });

        renderFullTable();
    }

    function toggleSelectAll(e) {
        var isChecked = e.target.checked;
        var filterRole = document.getElementById('fleet-filter-role').value;
        var filterStatus = document.getElementById('fleet-filter-status').value;
        var searchTerm = document.getElementById('fleet-search').value.toLowerCase();

        Array.from(state.nodes.values()).forEach(function(node) {
            // Check if node matches current filters
            if (filterRole && node.role !== filterRole) return;
            if (filterStatus === 'online' && !node.online) return;
            if (filterStatus === 'offline' && node.online) return;
            if (searchTerm && !node.mac.toLowerCase().includes(searchTerm) &&
                !(node.name && node.name.toLowerCase().includes(searchTerm))) {
                return;
            }

            if (isChecked) {
                selectedNodes.add(node.mac);
            } else {
                selectedNodes.delete(node.mac);
            }
        });

        renderFullTable();
    }

    function handleNodeAction(action, mac) {
        switch (action) {
            case 'flyto':
                flyToNode(mac);
                break;
            case 'identify':
                identifyNode(mac);
                break;
            case 'diagnostics':
                showNodeDiagnostics(mac);
                break;
            case 'reprovision':
                if (window.SpaxelOnboard && SpaxelOnboard.reprove) {
                    hideFullTableView();
                    SpaxelOnboard.reprove(mac);
                }
                break;
        }
    }

    function flyToNode(mac) {
        // Close the table view first
        hideFullTableView();

        // Use Viz3D to fly camera to the node
        if (window.Viz3D && Viz3D.flyToNode) {
            Viz3D.flyToNode(mac);
        } else if (window.Viz3D && Viz3D.focusOnNode) {
            Viz3D.focusOnNode(mac);
        } else {
            console.warn('[Fleet] No flyToNode method available on Viz3D');
        }
    }

    function showNodeDiagnostics(mac) {
        var node = state.nodes.get(mac);
        if (!node) return;

        // Create diagnostics modal
        var modal = document.createElement('div');
        modal.className = 'fleet-diagnostics-modal';
        modal.innerHTML = `
            <div class="fleet-diagnostics-content">
                <div class="fleet-diagnostics-header">
                    <h3>Node Diagnostics</h3>
                    <button class="fleet-close-modal">&times;</button>
                </div>
                <div class="fleet-diagnostics-body">
                    <div class="diagnostics-section">
                        <h4>Node Information</h4>
                        <table class="diagnostics-table">
                            <tr><td>MAC Address:</td><td>${node.mac}</td></tr>
                            <tr><td>Role:</td><td>${node.role}</td></tr>
                            <tr><td>Status:</td><td>${node.online ? 'Online' : 'Offline'}</td></tr>
                            <tr><td>Firmware:</td><td>${node.firmware_version || '--'}</td></tr>
                            <tr><td>Uptime:</td><td>${formatUptime(node.uptime_seconds || 0)}</td></tr>
                        </table>
                    </div>
                    <div class="diagnostics-section">
                        <h4>Health Metrics</h4>
                        <table class="diagnostics-table">
                            <tr><td>Health Score:</td><td>${((node.health_score || 0) * 100).toFixed(0)}%</td></tr>
                            <tr><td>Last Seen:</td><td>${new Date((node.last_seen_ms || 0)).toLocaleString()}</td></tr>
                        </table>
                    </div>
                </div>
                <div class="fleet-diagnostics-actions">
                    <button class="fleet-btn fleet-btn-primary" onclick="FleetPanel.identifyNode('${mac}')">Identify Node</button>
                    <button class="fleet-btn fleet-btn-secondary" onclick="FleetPanel.flyToNode('${mac}')">Fly to Node</button>
                </div>
            </div>
        `;

        document.body.appendChild(modal);

        modal.querySelector('.fleet-close-modal').addEventListener('click', function() {
            modal.remove();
        });

        modal.addEventListener('click', function(e) {
            if (e.target === modal) {
                modal.remove();
            }
        });
    }

    function bulkIdentify() {
        if (selectedNodes.size === 0) return;

        selectedNodes.forEach(function(mac) {
            identifyNode(mac, 3000); // 3 second blink
        });

        if (window.SpaxelApp && SpaxelApp.showToast) {
            SpaxelApp.showToast('Identifying ' + selectedNodes.size + ' nodes', 'info');
        }
    }

    function refreshFullTable() {
        fetchFleetHealth();
        fetchFleetHistory();
        renderFullTable();
    }

    function formatUptime(seconds) {
        if (!seconds) return '--';

        var days = Math.floor(seconds / 86400);
        var hours = Math.floor((seconds % 86400) / 3600);
        var minutes = Math.floor((seconds % 3600) / 60);

        if (days > 0) {
            return days + 'd ' + hours + 'h';
        } else if (hours > 0) {
            return hours + 'h ' + minutes + 'm';
        } else {
            return minutes + 'm';
        }
    }

    function formatPosition(x, y, z) {
        var px = (x || 0).toFixed(1);
        var py = (y || 0).toFixed(1);
        var pz = (z || 0).toFixed(1);
        return '(' + px + ', ' + py + ', ' + pz + ')';
    }

    // ============================================
    // Bulk Actions
    // ============================================
    function bulkRestart() {
        if (selectedNodes.size === 0) return;

        selectedNodes.forEach(function(mac) {
            restartNode(mac);
        });

        if (window.SpaxelApp && SpaxelApp.showToast) {
            SpaxelApp.showToast('Restarting ' + selectedNodes.size + ' nodes', 'info');
        }
    }

    function restartNode(mac) {
        fetch('/api/nodes/' + mac + '/reboot', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' }
        })
        .then(function(res) {
            if (!res.ok) {
                throw new Error('Restart failed: ' + res.status);
            }
            return res.json();
        })
        .then(function(data) {
            if (window.SpaxelApp && window.SpaxelApp.showToast) {
                window.SpaxelApp.showToast('Restart command sent to ' + mac, 'success');
            }
        })
        .catch(function(err) {
            console.error('[Fleet] Restart error:', err);
            if (window.SpaxelApp && window.SpaxelApp.showToast) {
                window.SpaxelApp.showToast('Failed to restart ' + mac + ': ' + err.message, 'error');
            }
        });
    }

    function updateAllNodes() {
        if (!confirm('Start OTA update for all nodes? This may take several minutes.')) {
            return;
        }

        fetch('/api/nodes/update-all', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' }
        })
        .then(function(res) {
            if (!res.ok) {
                throw new Error('Update all failed: ' + res.status);
            }
            return res.json();
        })
        .then(function(data) {
            if (window.SpaxelApp && SpaxelApp.showToast) {
                SpaxelApp.showToast('OTA update started for ' + (data.count || 0) + ' nodes', 'success');
            }
        })
        .catch(function(err) {
            console.error('[Fleet] Update all error:', err);
            if (window.SpaxelApp && SpaxelApp.showToast) {
                SpaxelApp.showToast('Failed to start OTA update: ' + err.message, 'error');
            }
        });
    }

    function rebaselineAllNodes() {
        if (!confirm('Re-baseline all links? This requires an empty room for accurate results.')) {
            return;
        }

        fetch('/api/nodes/rebaseline-all', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' }
        })
        .then(function(res) {
            if (!res.ok) {
                throw new Error('Re-baseline failed: ' + res.status);
            }
            return res.json();
        })
        .then(function(data) {
            if (window.SpaxelApp && SpaxelApp.showToast) {
                SpaxelApp.showToast('Re-baseline started for ' + (data.count || 0) + ' nodes', 'success');
            }
        })
        .catch(function(err) {
            console.error('[Fleet] Re-baseline error:', err);
            if (window.SpaxelApp && SpaxelApp.showToast) {
                SpaxelApp.showToast('Failed to start re-baseline: ' + err.message, 'error');
            }
        });
    }

    function exportConfig() {
        fetch('/api/export')
            .then(function(res) {
                if (!res.ok) {
                    throw new Error('Export failed: ' + res.status);
                }
                return res.json();
            })
            .then(function(data) {
                var blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' });
                var url = URL.createObjectURL(blob);
                var a = document.createElement('a');
                a.href = url;
                a.download = 'spaxel-config-' + new Date().toISOString().slice(0, 10) + '.json';
                document.body.appendChild(a);
                a.click();
                document.body.removeChild(a);
                URL.revokeObjectURL(url);

                if (window.SpaxelApp && SpaxelApp.showToast) {
                    SpaxelApp.showToast('Configuration exported successfully', 'success');
                }
            })
            .catch(function(err) {
                console.error('[Fleet] Export error:', err);
                if (window.SpaxelApp && SpaxelApp.showToast) {
                    SpaxelApp.showToast('Failed to export configuration: ' + err.message, 'error');
                }
            });
    }

    function importConfig() {
        var input = document.createElement('input');
        input.type = 'file';
        input.accept = 'application/json';

        input.onchange = function(e) {
            var file = e.target.files[0];
            if (!file) return;

            var reader = new FileReader();
            reader.onload = function(event) {
                try {
                    var config = JSON.parse(event.target.result);

                    if (!confirm('Import configuration? This will replace all existing nodes, zones, and settings.')) {
                        return;
                    }

                    fetch('/api/import', {
                        method: 'POST',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify(config)
                    })
                    .then(function(res) {
                        if (!res.ok) {
                            throw new Error('Import failed: ' + res.status);
                        }
                        return res.json();
                    })
                    .then(function(data) {
                        if (window.SpaxelApp && SpaxelApp.showToast) {
                            SpaxelApp.showToast('Configuration imported successfully. Reloading...', 'success');
                        }
                        setTimeout(function() {
                            location.reload();
                        }, 2000);
                    })
                    .catch(function(err) {
                        console.error('[Fleet] Import error:', err);
                        if (window.SpaxelApp && SpaxelApp.showToast) {
                            SpaxelApp.showToast('Failed to import configuration: ' + err.message, 'error');
                        }
                    });
                } catch (err) {
                    if (window.SpaxelApp && SpaxelApp.showToast) {
                        SpaxelApp.showToast('Invalid JSON file: ' + err.message, 'error');
                    }
                }
            };
            reader.readAsText(file);
        };

        input.click();
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
        identifyNode: identifyNode,
        flyToNode: flyToNode,
        reproveNode: function(mac) { handleNodeAction('reprovision', mac); },
        toggleFullTableView: toggleFullTableView,
        showFullTableView: showFullTableView,
        hideFullTableView: hideFullTableView
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
