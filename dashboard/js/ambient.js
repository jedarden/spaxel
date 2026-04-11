/**
 * Spaxel Dashboard - Ambient Mode
 *
 * Simplified, always-on display mode for wall-mounted tablets.
 * Time-of-day aware palettes, auto-dim, and calm visualization.
 */

(function() {
    'use strict';

    // ============================================
    // Configuration
    // ============================================
    const TIME_PERIODS = {
        morning: { start: 6, end: 10 },   // 6am - 10am
        day: { start: 10, end: 18 },     // 10am - 6pm
        evening: { start: 18, end: 22 },  // 6pm - 10pm
        night: { start: 22, end: 6 }      // 10pm - 6am
    };

    const UPDATE_INTERVAL = 5000;  // 5 seconds for polling fallback
    const AUTO_DIM_TIMEOUT = 60 * 1000;  // 60 seconds of no presence in ambient zone

    // ============================================
    // State
    // ============================================
    let isActive = false;
    let canvas = null;
    let renderer = null;
    let ws = null;
    let wsReconnectTimer = null;
    let updateTimer = null;
    let currentState = {
        zones: [],
        blobs: [],
        portals: [],
        alerts: [],
        nodes: [],
        securityMode: false,
        nodesOnline: 0,
        nodesTotal: 0,
        lastUpdate: null
    };

    // Configuration
    let config = {
        ambientZone: null  // Zone ID for auto-dim detection
    };

    // ============================================
    // Initialization
    // ============================================

    /**
     * Initialize ambient mode
     */
    function init() {
        console.log('[Ambient Mode] Initializing...');

        // Check if we should be in ambient mode
        checkAmbientMode();

        // Set up time-of-day updates
        startTimeOfDayUpdater();

        console.log('[Ambient Mode] Initialized');
    }

    /**
     * Check if ambient mode should be active
     */
    function checkAmbientMode() {
        // For standalone ambient.html, always enable
        // For main dashboard, check hash
        if (window.location.pathname.endsWith('/ambient.html') ||
            window.location.pathname === '/ambient') {
            enableAmbientMode();
            return;
        }

        const hash = window.location.hash.slice(1);
        if (hash === 'ambient') {
            enableAmbientMode();
        } else if (isActive) {
            disableAmbientMode();
        }
    }

    /**
     * Enable ambient mode
     */
    function enableAmbientMode() {
        if (isActive) return;

        isActive = true;
        document.body.classList.add('ambient-mode');

        // Create ambient UI
        createAmbientUI();

        // Set initial time period
        updateTimeOfDay();

        // Initialize renderer
        const canvasEl = document.getElementById('ambient-canvas');
        if (canvasEl && window.SpaxelAmbientRenderer) {
            renderer = window.SpaxelAmbientRenderer;
            renderer.init(canvasEl, {
                ambientZone: config.ambientZone
            });

            // Set up callbacks
            renderer.setAlertClickCallback(handleAlertClick);
            renderer.setUserActivityCallback(resetDimTimer);
        }

        // Initialize briefing
        if (window.SpaxelAmbientBriefing) {
            window.SpaxelAmbientBriefing.init();
            window.SpaxelAmbientBriefing.setOnDismiss(() => {
                // After briefing dismissed, check for alerts
                checkAlerts();
            });
        }

        // Connect WebSocket
        connectWebSocket();

        // Show briefing if this is first detection today
        checkAndShowBriefing();

        console.log('[Ambient Mode] Enabled');
    }

    /**
     * Disable ambient mode
     */
    function disableAmbientMode() {
        if (!isActive) return;

        isActive = false;
        document.body.classList.remove('ambient-mode');

        // Disconnect WebSocket
        disconnectWebSocket();

        // Destroy renderer
        if (renderer) {
            renderer.destroy();
            renderer = null;
        }

        // Remove ambient UI
        const ambientContainer = document.getElementById('ambient-container');
        if (ambientContainer) {
            ambientContainer.remove();
        }

        // Stop timers
        stopUpdates();

        console.log('[Ambient Mode] Disabled');
    }

    // ============================================
    // UI Creation
    // ============================================

    /**
     * Create ambient mode UI
     */
    function createAmbientUI() {
        // Check if already exists
        if (document.getElementById('ambient-container')) {
            return;
        }

        const container = document.createElement('div');
        container.id = 'ambient-container';
        container.innerHTML = `
            <div class="ambient-floorplan">
                <canvas id="ambient-canvas" class="ambient-canvas"></canvas>
            </div>
            <div class="ambient-status">
                <div class="ambient-status-item">
                    <div class="ambient-status-dot online" id="ambient-status-dot"></div>
                    <span id="ambient-status-text">Loading...</span>
                </div>
                <div class="ambient-status-item">
                    <span id="ambient-time"></span>
                </div>
                <div class="ambient-status-item">
                    <span id="ambient-nodes">0 nodes</span>
                </div>
            </div>

            <!-- "All Secure" message -->
            <div id="ambient-secure" class="ambient-secure" style="display: none;">
                <div class="ambient-secure-icon">&#128274;</div>
                <div class="ambient-secure-text">All secure</div>
            </div>
        `;

        document.body.appendChild(container);

        canvas = document.getElementById('ambient-canvas');

        // Set up event listeners for route changes
        window.addEventListener('hashchange', checkAmbientMode);
    }

    // ============================================
    // Time of Day
    // ============================================

    /**
     * Start time-of-day updater
     */
    function startTimeOfDayUpdater() {
        updateTimeOfDay();
        setInterval(updateTimeOfDay, 60000); // Check every minute
    }

    /**
     * Update time-of-day theme
     */
    function updateTimeOfDay() {
        if (!isActive) return;

        const hour = new Date().getHours();
        let period = 'night';

        for (const [key, range] of Object.entries(TIME_PERIODS)) {
            if (range.start <= range.end) {
                // Normal period (e.g., morning: 6-10)
                if (hour >= range.start && hour < range.end) {
                    period = key;
                    break;
                }
            } else {
                // Overnight period (e.g., night: 22-6)
                if (hour >= range.start || hour < range.end) {
                    period = key;
                    break;
                }
            }
        }

        // Remove all time periods
        document.body.classList.remove('time-morning', 'time-day', 'time-evening', 'time-night');

        // Add current period
        document.body.classList.add('time-' + period);

        // Update renderer time period
        if (renderer) {
            // Force a render to update background color
            renderer.render();
        }
    }

    // ============================================
    // WebSocket Connection
    // ============================================

    /**
     * Connect to WebSocket for real-time updates
     */
    function connectWebSocket() {
        // Disconnect existing connection
        if (ws) {
            ws.close();
        }

        // Determine WebSocket protocol
        const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsHost = window.location.host;
        const wsUrl = `${wsProtocol}//${wsHost}/ws/dashboard`;

        console.log('[Ambient Mode] Connecting WebSocket:', wsUrl);

        ws = new WebSocket(wsUrl);

        ws.onopen = function() {
            console.log('[Ambient Mode] WebSocket connected');
            updateConnectionStatus(true);

            // Clear reconnect timer
            if (wsReconnectTimer) {
                clearTimeout(wsReconnectTimer);
                wsReconnectTimer = null;
            }
        };

        ws.onmessage = function(event) {
            try {
                const data = JSON.parse(event.data);
                handleWebSocketMessage(data);
            } catch (e) {
                console.error('[Ambient Mode] Error parsing WebSocket message:', e);
            }
        };

        ws.onclose = function(event) {
            console.log('[Ambient Mode] WebSocket disconnected:', event.code, event.reason);
            updateConnectionStatus(false);

            // Attempt to reconnect
            if (isActive) {
                const delay = 5000; // 5 seconds
                wsReconnectTimer = setTimeout(connectWebSocket, delay);
            }
        };

        ws.onerror = function(error) {
            console.error('[Ambient Mode] WebSocket error:', error);
            updateConnectionStatus(false);
        };
    }

    /**
     * Disconnect WebSocket
     */
    function disconnectWebSocket() {
        if (ws) {
            ws.close();
            ws = null;
        }

        if (wsReconnectTimer) {
            clearTimeout(wsReconnectTimer);
            wsReconnectTimer = null;
        }
    }

    /**
     * Update connection status indicator
     */
    function updateConnectionStatus(connected) {
        const statusDot = document.getElementById('ambient-status-dot');
        const statusText = document.getElementById('ambient-status-text');

        if (statusDot) {
            if (connected) {
                statusDot.className = 'ambient-status-dot online';
            } else {
                statusDot.className = 'ambient-status-dot';
                statusDot.style.background = '#ff3b30';
            }
        }

        if (statusText) {
            statusText.textContent = connected ? 'Connected' : 'Reconnecting...';
        }
    }

    /**
     * Handle WebSocket message
     */
    function handleWebSocketMessage(data) {
        // Handle snapshot message (first message on connect)
        if (data.type === 'snapshot' || (!data.type && data.blobs !== undefined)) {
            // Full snapshot
            if (data.zones) currentState.zones = data.zones;
            if (data.blobs) currentState.blobs = data.blobs;
            if (data.portals) currentState.portals = data.portals;
            if (data.nodes) currentState.nodes = data.nodes;
            if (data.events) currentState.alerts = data.events.filter(e =>
                e.type === 'alert' || e.type === 'fall_alert' || e.type === 'anomaly'
            );
            if (data.security_mode !== undefined) currentState.securityMode = data.security_mode;

            // Update node counts
            currentState.nodesOnline = currentState.nodes.filter(n => n.status === 'online').length;
            currentState.nodesTotal = currentState.nodes.length;

            currentState.lastUpdate = new Date();

            // Update renderer state
            if (renderer) {
                renderer.updateState(currentState);
            }

            // Update UI
            updateStatus();
            checkAlerts();

            // Trigger first detection for briefing
            if (currentState.blobs.length > 0 && window.SpaxelAmbientBriefing) {
                window.SpaxelAmbientBriefing.onFirstDetection();
            }

            return;
        }

        // Handle incremental updates
        if (data.blobs) {
            currentState.blobs = data.blobs;
        }
        if (data.zones) {
            currentState.zones = data.zones;
        }
        if (data.portals) {
            currentState.portals = data.portals;
        }
        if (data.nodes) {
            currentState.nodes = data.nodes;
            currentState.nodesOnline = currentState.nodes.filter(n => n.status === 'online').length;
            currentState.nodesTotal = currentState.nodes.length;
        }
        if (data.events && data.events.length > 0) {
            // Add new alerts
            data.events.forEach(event => {
                if (event.type === 'alert' || event.type === 'fall_alert' || event.type === 'anomaly') {
                    // Check if alert already exists
                    const exists = currentState.alerts.some(a => a.id === event.id);
                    if (!exists) {
                        currentState.alerts.push(event);
                    }
                }
            });
        }

        currentState.lastUpdate = new Date();

        // Update renderer state
        if (renderer) {
            renderer.updateState(currentState);
        }

        // Update UI
        updateStatus();
        checkAlerts();
    }

    // ============================================
    // Status Updates
    // ============================================

    /**
     * Update status bar
     */
    function updateStatus() {
        const statusText = document.getElementById('ambient-status-text');
        const timeDisplay = document.getElementById('ambient-time');
        const nodesDisplay = document.getElementById('ambient-nodes');
        const secureMessage = document.getElementById('ambient-secure');

        if (statusText) {
            // Determine status based on alerts and security mode
            if (currentState.alerts.length > 0) {
                statusText.textContent = 'Alert active';
            } else if (currentState.securityMode) {
                statusText.textContent = 'Security armed';
            } else {
                statusText.textContent = 'All secure';
            }
        }

        if (timeDisplay) {
            const now = new Date();
            timeDisplay.textContent = now.toLocaleTimeString('en-US', {
                hour: 'numeric',
                minute: '2-digit',
                hour12: true
            });
        }

        if (nodesDisplay) {
            nodesDisplay.textContent = `${currentState.nodesOnline}/${currentState.nodesTotal} nodes`;
        }

        // Show/hide "All Secure" message
        if (secureMessage) {
            const hasPeople = currentState.blobs.length > 0;
            const hasAlerts = currentState.alerts.length > 0;

            if (!hasPeople && !hasAlerts && !isDimmed()) {
                secureMessage.style.display = 'block';
            } else {
                secureMessage.style.display = 'none';
            }
        }
    }

    function isDimmed() {
        return renderer && document.getElementById('ambient-canvas')?.style.filter.includes('brightness(0.4)');
    }

    /**
     * Check for alerts and show/hide alert mode
     */
    function checkAlerts() {
        if (currentState.alerts.length > 0) {
            // Show alert mode in renderer
            if (renderer) {
                const latestAlert = currentState.alerts[0];
                renderer.enterAlertMode(latestAlert);
            }
        } else {
            // Hide alert mode
            if (renderer) {
                renderer.exitAlertMode();
            }
        }
    }

    /**
     * Handle alert click/acknowledge
     */
    function handleAlertClick(alert) {
        // Dismiss the alert
        currentState.alerts = [];
        checkAlerts();

        // In a real implementation, this would call an API to acknowledge the alert
        // For now, just show a toast
        showToast('Alert acknowledged', 'info');

        // Check for fall alerts - POST to acknowledge endpoint
        if (alert.type === 'fall_alert' && alert.id) {
            acknowledgeAlert(alert.id, 'fall');
        } else if (alert.type === 'anomaly' && alert.id) {
            acknowledgeAlert(alert.id, 'anomaly');
        }
    }

    /**
     * Acknowledge an alert via API
     */
    async function acknowledgeAlert(alertId, alertType) {
        try {
            let endpoint = `/api/anomalies/${alertId}/acknowledge`;
            if (alertType === 'fall') {
                endpoint = `/api/fall/${alertId}/acknowledge`;
            }

            const response = await fetch(endpoint, {
                method: 'POST'
            });

            if (response.ok) {
                console.log('[Ambient Mode] Alert acknowledged:', alertId);
            } else {
                console.warn('[Ambient Mode] Failed to acknowledge alert:', alertId);
            }
        } catch (error) {
            console.error('[Ambient Mode] Error acknowledging alert:', error);
        }
    }

    /**
     * Reset the dim timer (called on user activity)
     */
    function resetDimTimer() {
        if (renderer) {
            renderer.wakeFromDim();
        }
    }

    // ============================================
    // Morning Briefing
    // ============================================

    /**
     * Check and show morning briefing
     */
    async function checkAndShowBriefing() {
        if (!window.SpaxelAmbientBriefing) {
            return;
        }

        // Check if briefing should be shown
        if (await window.SpaxelAmbientBriefing.shouldShowToday()) {
            window.SpaxelAmbientBriefing.fetchAndShow();
        }
    }

    // ============================================
    // Helper Functions
    // ============================================

    /**
     * Show toast notification
     */
    function showToast(message, type = 'info') {
        if (window.showToast) {
            window.showToast(message, type);
            return;
        }

        // Fallback toast
        const toast = document.createElement('div');
        toast.className = `toast ${type}`;
        toast.textContent = message;
        toast.style.cssText = `
            position: fixed;
            top: 20px;
            left: 50%;
            transform: translateX(-50%);
            background: rgba(0, 0, 0, 0.9);
            color: white;
            padding: 12px 20px;
            border-radius: 8px;
            z-index: 200;
        `;

        document.body.appendChild(toast);

        setTimeout(() => {
            toast.style.animation = 'fadeOut 0.3s ease-out forwards';
            setTimeout(() => toast.remove(), 300);
        }, 3000);
    }

    // ============================================
    // Public API
    // ============================================
    window.SpaxelAmbientMode = {
        init: init,
        enable: enableAmbientMode,
        disable: disableAmbientMode,
        isActive: () => isActive,
        setAmbientZone: (zoneId) => {
            config.ambientZone = zoneId;
            if (renderer) {
                renderer.setAmbientZone(zoneId);
            }
        },
        refresh: () => {
            // Trigger a render
            if (renderer) {
                renderer.render();
            }
        }
    };

    // Auto-initialize
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }

    console.log('[Ambient Mode] Module loaded');
})();
