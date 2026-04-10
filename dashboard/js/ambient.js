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

    const UPDATE_INTERVAL = 5000;  // 5 seconds
    const DIM_TIMEOUT = 30 * 60 * 1000;  // 30 minutes of inactivity
    const BRIEFING_DURATION = 30 * 1000;  // 30 seconds

    // ============================================
    // State
    // ============================================
    let isActive = false;
    let canvas = null;
    let ctx = null;
    let currentState = {
        zones: [],
        blobs: [],
        alerts: [],
        securityMode: false,
        nodesOnline: 0,
        nodesTotal: 0,
        lastUpdate: null
    };
    let ws = null;
    let wsReconnectTimer = null;
    let updateTimer = null;
    let dimTimer = null;
    let briefingTimer = null;
    let isDimmed = false;

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

        // Set up activity monitoring
        startActivityMonitoring();

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
                    <div class="ambient-status-dot" id="ambient-status-dot"></div>
                    <span id="ambient-status-text">Loading...</span>
                </div>
                <div class="ambient-status-item">
                    <span id="ambient-time"></span>
                </div>
                <div class="ambient-status-item">
                    <span id="ambient-nodes">0 nodes</span>
                </div>
            </div>

            <!-- Alert overlay -->
            <div id="ambient-alert" class="ambient-alert hidden">
                <div class="ambient-alert-icon">&#x26A0;</div>
                <div class="ambient-alert-title" id="alert-title">Alert</div>
                <div class="ambient-alert-message" id="alert-message"></div>
                <div class="ambient-alert-actions">
                    <button class="ambient-alert-btn primary" id="alert-action-primary">I'm Fine</button>
                    <button class="ambient-alert-btn secondary" id="alert-action-secondary">Dismiss</button>
                </div>
            </div>

            <!-- Morning briefing overlay -->
            <div id="ambient-briefing" class="ambient-briefing hidden">
                <div class="ambient-briefing-content">
                    <div class="ambient-briefing-greeting" id="briefing-greeting">Good morning!</div>
                    <div id="briefing-content"></div>
                    <button class="ambient-briefing-dismiss" id="briefing-dismiss">Got it</button>
                </div>
            </div>

            <!-- "All Secure" message -->
            <div id="ambient-secure" class="ambient-secure" style="display: none;">
                <div class="ambient-secure-icon">&#x1F512;</div>
                <div class="ambient-secure-text">All secure</div>
            </div>
        `;

        document.body.appendChild(container);

        // Set up canvas
        canvas = document.getElementById('ambient-canvas');
        ctx = canvas.getContext('2d');

        // Handle resize
        window.addEventListener('resize', resizeCanvas);
        resizeCanvas();

        // Set up event listeners
        setupEventListeners();
    }

    /**
     * Set up event listeners
     */
    function setupEventListeners() {
        // Alert action buttons
        document.getElementById('alert-action-primary')?.addEventListener('click', handleAlertAction);
        document.getElementById('alert-action-secondary')?.addEventListener('click', dismissAlert);

        // Briefing dismiss
        document.getElementById('briefing-dismiss')?.addEventListener('click', dismissBriefing);

        // Touch/click to wake from dim
        document.getElementById('ambient-container')?.addEventListener('click', wakeFromDim);

        // Monitor for route changes
        window.addEventListener('hashchange', checkAmbientMode);
    }

    /**
     * Resize canvas to fit container
     */
    function resizeCanvas() {
        if (!canvas || !ctx) return;

        const container = document.querySelector('.ambient-floorplan');
        if (!container) return;

        const rect = container.getBoundingClientRect();
        const dpr = window.devicePixelRatio || 1;

        canvas.width = rect.width * dpr;
        canvas.height = rect.height * dpr;
        canvas.style.width = rect.width + 'px';
        canvas.style.height = rect.height + 'px';

        ctx.scale(dpr, dpr);

        // Re-render
        render();
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

        console.log('[Ambient Mode] Time period:', period);
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
        if (statusDot) {
            if (connected) {
                statusDot.className = 'ambient-status-dot online';
            } else {
                statusDot.className = 'ambient-status-dot';
                statusDot.style.background = '#ff3b30';
            }
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
            if (data.events) currentState.alerts = data.events.filter(e => e.type === 'alert' || e.type === 'fall_alert' || e.type === 'anomaly');
            if (data.security_mode !== undefined) currentState.securityMode = data.security_mode;

            currentState.lastUpdate = new Date();

            // Update UI
            updateStatus();
            checkAlerts();
            render();

            return;
        }

        // Handle incremental updates
        if (data.blobs) {
            currentState.blobs = data.blobs;
        }
        if (data.zones) {
            currentState.zones = data.zones;
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

        // Update UI
        updateStatus();
        checkAlerts();
        render();
    }

    // ============================================
    // Data Updates (polling fallback)
    // ============================================

    /**
     * Start periodic updates (fallback if WebSocket fails)
     */
    function startUpdates() {
        if (updateTimer) {
            clearInterval(updateTimer);
        }

        // Note: Updates primarily come via WebSocket
        // This timer is just for periodic status refresh
        updateTimer = setInterval(() => {
            updateStatus();
        }, 10000); // Every 10 seconds
    }

    /**
     * Stop updates
     */
    function stopUpdates() {
        if (updateTimer) {
            clearInterval(updateTimer);
            updateTimer = null;
        }
    }

    /**
     * Fetch data for ambient display (fallback polling)
     */
    async function fetchAmbientData() {
        if (!isActive) return;

        try {
            // Fetch zones
            const zonesResponse = await fetch('/api/zones');
            if (zonesResponse.ok) {
                currentState.zones = await zonesResponse.json();
            }

            // Fetch blobs
            const blobsResponse = await fetch('/api/blobs');
            if (blobsResponse.ok) {
                currentState.blobs = await blobsResponse.json();
            }

            // Fetch system status
            const statusResponse = await fetch('/api/status');
            if (statusResponse.ok) {
                const statusData = await statusResponse.json();
                currentState.securityMode = statusData.security_mode || false;
                currentState.nodesOnline = statusData.nodes_online || 0;
                currentState.nodesTotal = statusData.nodes || 0;
            }

            // Fetch recent alerts
            const alertsResponse = await fetch('/api/events?limit=5&type=alert');
            if (alertsResponse.ok) {
                const alertsData = await alertsResponse.json();
                currentState.alerts = alertsData.events || [];
            }

            currentState.lastUpdate = new Date();

            // Update UI
            updateStatus();
            checkAlerts();
            render();

        } catch (error) {
            console.error('[Ambient Mode] Error fetching data:', error);
        }
    }

    /**
     * Update status bar
     */
    function updateStatus() {
        const statusDot = document.getElementById('ambient-status-dot');
        const statusText = document.getElementById('ambient-status-text');
        const timeDisplay = document.getElementById('ambient-time');
        const nodesDisplay = document.getElementById('ambient-nodes');

        if (statusDot && statusText) {
            // Determine status based on alerts and security mode
            if (currentState.alerts.length > 0) {
                statusDot.className = 'ambient-status-dot alert';
                statusText.textContent = 'Alert active';
            } else if (currentState.securityMode) {
                statusDot.className = 'ambient-status-dot';
                statusDot.style.background = '#ff9500';
                statusText.textContent = 'Security armed';
            } else {
                statusDot.className = 'ambient-status-dot online';
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
    }

    /**
     * Check for alerts and show overlay
     */
    function checkAlerts() {
        const alertOverlay = document.getElementById('ambient-alert');
        const secureMessage = document.getElementById('ambient-secure');

        if (!alertOverlay) return;

        if (currentState.alerts.length > 0) {
            // Show alert
            const latestAlert = currentState.alerts[0];
            showAlert(latestAlert);
            secureMessage.style.display = 'none';
        } else {
            // Hide alert, show secure if no people
            alertOverlay.classList.add('hidden');

            const hasPeople = currentState.blobs.length > 0;
            if (!hasPeople && !isDimmed) {
                secureMessage.style.display = 'block';
            } else {
                secureMessage.style.display = 'none';
            }
        }
    }

    /**
     * Show alert overlay
     */
    function showAlert(alert) {
        const alertOverlay = document.getElementById('ambient-alert');
        const titleEl = document.getElementById('alert-title');
        const messageEl = document.getElementById('alert-message');

        if (!alertOverlay) return;

        titleEl.textContent = alert.title || 'Alert';
        messageEl.textContent = formatAlertMessage(alert);

        alertOverlay.classList.remove('hidden');
        document.getElementById('ambient-container').classList.add('alert-active');

        // Wake from dim
        wakeFromDim();
    }

    /**
     * Format alert message
     */
    function formatAlertMessage(alert) {
        if (alert.detail_json) {
            try {
                const detail = typeof alert.detail_json === 'string'
                    ? JSON.parse(alert.detail_json)
                    : alert.detail_json;
                return detail.message || detail.description || 'Alert triggered';
            } catch (e) {
                // Ignore parse errors
            }
        }

        return 'Alert detected in your home';
    }

    /**
     * Handle alert action button
     */
    function handleAlertAction() {
        // Dismiss the alert and mark as handled
        dismissAlert();

        // In a real implementation, this would call an API to acknowledge the alert
        showToast('Alert acknowledged', 'info');
    }

    /**
     * Dismiss alert overlay
     */
    function dismissAlert() {
        const alertOverlay = document.getElementById('ambient-alert');
        if (alertOverlay) {
            alertOverlay.classList.add('hidden');
            document.getElementById('ambient-container').classList.remove('alert-active');
        }
    }

    // ============================================
    // Rendering
    // ============================================

    /**
     * Render the ambient display
     */
    function render() {
        if (!canvas || !ctx) return;

        const width = canvas.width / (window.devicePixelRatio || 1);
        const height = canvas.height / (window.devicePixelRatio || 1);

        // Clear canvas
        ctx.clearRect(0, 0, width, height);

        // Get floor plan bounds (default to centered square)
        const bounds = getFloorPlanBounds(width, height);

        // Draw floor plan background
        drawFloorPlan(ctx, bounds);

        // Draw zones
        drawZones(ctx, bounds);

        // Draw people
        drawPeople(ctx, bounds);
    }

    /**
     * Get floor plan bounds
     */
    function getFloorPlanBounds(canvasWidth, canvasHeight) {
        // Default bounds - centered square with margin
        const margin = 40;
        const size = Math.min(canvasWidth, canvasHeight) - margin * 2;

        return {
            x: (canvasWidth - size) / 2,
            y: (canvasHeight - size) / 2,
            width: size,
            height: size
        };
    }

    /**
     * Draw floor plan background
     */
    function drawFloorPlan(ctx, bounds) {
        // Draw floor rectangle
        ctx.fillStyle = '#f5f5f7';
        ctx.fillRect(bounds.x, bounds.y, bounds.width, bounds.height);

        // Draw grid lines
        ctx.strokeStyle = '#e0e0e0';
        ctx.lineWidth = 1;

        const gridSize = 50;

        for (let x = bounds.x; x <= bounds.x + bounds.width; x += gridSize) {
            ctx.beginPath();
            ctx.moveTo(x, bounds.y);
            ctx.lineTo(x, bounds.y + bounds.height);
            ctx.stroke();
        }

        for (let y = bounds.y; y <= bounds.y + bounds.height; y += gridSize) {
            ctx.beginPath();
            ctx.moveTo(bounds.x, y);
            ctx.lineTo(bounds.x + bounds.width, y);
            ctx.stroke();
        }

        // Draw border
        ctx.strokeStyle = '#ccc';
        ctx.lineWidth = 2;
        ctx.strokeRect(bounds.x, bounds.y, bounds.width, bounds.height);
    }

    /**
     * Draw zones
     */
    function drawZones(ctx, bounds) {
        if (!currentState.zones || currentState.zones.length === 0) {
            return;
        }

        // Find the bounds of all zones (using x and y for 2D floor plan)
        const allZones = currentState.zones;

        // Zones have x, y, z (3D position) and w, d, h (size)
        // For 2D top-down view, we use x (horizontal) and y (depth)
        // Note: Zone JSON uses field names: x, y, z for position and w, d, h for sizes
        // But in the actual ZoneSnapshot, these map to: MinX, MinY, MinZ and SizeX, SizeY, SizeZ
        // For 2D rendering: x = MinX, y = MinY, width = SizeX, height = SizeY (depth)

        const minX = Math.min(...allZones.map(z => z.x || z.MinX || 0));
        const minY = Math.min(...allZones.map(z => z.y || z.MinY || 0));
        const maxX = Math.max(...allZones.map(z => (z.x || z.MinX || 0) + (z.w || z.SizeX || z.w || 0)));
        const maxY = Math.max(...allZones.map(z => (z.y || z.MinY || 0) + (z.d || z.SizeY || z.d || 0)));

        const zoneScale = Math.min(
            bounds.width / (maxX - minX || 1),
            bounds.height / (maxY - minY || 1)
        );

        // Draw each zone
        allZones.forEach(zone => {
            const zoneX = zone.x || zone.MinX || 0;
            const zoneY = zone.y || zone.MinY || 0;
            const zoneW = zone.w || zone.SizeX || zone.w || 1;
            const zoneD = zone.d || zone.SizeY || zone.d || 1;

            const zx = bounds.x + (zoneX - minX) * zoneScale;
            const zy = bounds.y + (zoneY - minY) * zoneScale;
            const zw = zoneW * zoneScale;
            const zh = zoneD * zoneScale;

            // Zone background
            const count = zone.count || zone.occupancy || zone.Count || 0;
            const isOccupied = count > 0;
            ctx.fillStyle = isOccupied ? 'rgba(52, 199, 89, 0.15)' : 'rgba(200, 200, 200, 0.1)';
            ctx.fillRect(zx, zy, zw, zh);

            // Zone border
            ctx.strokeStyle = isOccupied ? 'rgba(52, 199, 89, 0.3)' : 'rgba(200, 200, 200, 0.3)';
            ctx.lineWidth = 2;
            ctx.strokeRect(zx, zy, zw, zh);

            // Zone label
            drawZoneLabel(ctx, zone, zx, zy, zw, count);
        });
    }

    /**
     * Draw zone label
     */
    function drawZoneLabel(ctx, zone, x, y, width, count) {
        const zoneName = zone.name || zone.Name || 'Zone';
        const labelText = `${zoneName}${count > 0 ? ` (${count})` : ''}`;

        ctx.font = '13px -apple-system, BlinkMacSystemFont, "SF Pro Display", sans-serif';
        ctx.textAlign = 'center';
        ctx.textBaseline = 'middle';

        // Background for label
        const metrics = ctx.measureText(labelText);
        const padding = 8;
        const labelWidth = metrics.width + padding * 2;
        const labelHeight = 24;
        const labelX = x + width / 2 - labelWidth / 2;
        const labelY = y - labelHeight / 2;

        ctx.fillStyle = 'rgba(255, 255, 255, 0.9)';
        ctx.beginPath();
        ctx.roundRect(labelX, labelY, labelWidth, labelHeight, 6);
        ctx.fill();

        // Text
        ctx.fillStyle = count > 0 ? '#333' : '#666';
        ctx.fillText(labelText, x + width / 2, y + labelHeight / 2);
    }

    /**
     * Draw people
     */
    function drawPeople(ctx, bounds) {
        if (!currentState.blobs || currentState.blobs.length === 0) {
            return;
        }

        // Find the bounds of all blobs
        const minX = Math.min(...currentState.blobs.map(b => b.x));
        const minY = Math.min(...currentState.blobs.map(b => b.y));
        const maxX = Math.max(...currentState.blobs.map(b => b.x));
        const maxY = Math.max(...currentState.blobs.map(b => b.y));

        const blobScale = Math.min(
            bounds.width / (maxX - minX || 1),
            bounds.height / (maxY - minY || 1)
        );

        // Assign person indices for consistent coloring
        const personIndices = new Map();
        let nextIndex = 0;

        currentState.blobs.forEach((blob, index) => {
            const bx = bounds.x + (blob.x - minX) * blobScale;
            const by = bounds.y + (blob.y - minY) * blobScale;

            // Get person index
            let personIndex = personIndices.get(blob.person);
            if (personIndex === undefined) {
                personIndex = nextIndex++;
                personIndices.set(blob.person, personIndex);
            }

            // Draw person circle
            drawPerson(ctx, bx, by, blob.person, personIndex);
        });
    }

    /**
     * Draw a person indicator
     */
    function drawPerson(ctx, x, y, person, index) {
        const radius = 20;

        // Person circle
        ctx.beginPath();
        ctx.arc(x, y, radius, 0, Math.PI * 2);

        if (person) {
            // Known person - use their color
            const color = getPersonColor(person);
            ctx.fillStyle = color;
        } else {
            // Unknown person - use index for color
            ctx.fillStyle = '#95a5a6';
        }

        ctx.fill();

        // Add glow effect
        ctx.shadowColor = ctx.fillStyle;
        ctx.shadowBlur = 10;
        ctx.fill();
        ctx.shadowBlur = 0;

        // Person initial or icon
        ctx.fillStyle = 'white';
        ctx.font = 'bold 14px -apple-system, sans-serif';
        ctx.textAlign = 'center';
        ctx.textBaseline = 'middle';

        if (person) {
            const initials = getPersonInitials(person);
            ctx.fillText(initials, x, y);
        } else {
            ctx.fillText('?', x, y);
        }

        // Position indicator (pillar)
        ctx.fillStyle = 'rgba(255, 255, 255, 0.5)';
        ctx.fillRect(x - 2, y, 4, radius + 8);
    }

    /**
     * Get person color
     */
    function getPersonColor(person) {
        // Generate consistent color from name
        let hash = 0;
        for (let i = 0; i < person.length; i++) {
            hash = person.charCodeAt(i) + ((hash << 5) - hash);
        }
        const hue = Math.abs(hash) % 360;
        return `hsl(${hue}, 70%, 50%)`;
    }

    /**
     * Get person initials
     */
    function getPersonInitials(person) {
        const parts = person.trim().split(/\s+/);
        if (parts.length >= 2) {
            return (parts[0][0] + parts[1][0]).toUpperCase();
        }
        return person.substring(0, 2).toUpperCase();
    }

    // ============================================
    // Activity Monitoring & Dimming
    // ============================================

    /**
     * Start activity monitoring
     */
    function startActivityMonitoring() {
        // Reset dim timer on any user interaction
        const events = ['click', 'touchstart', 'keydown', 'mousemove'];
        events.forEach(event => {
            document.addEventListener(event, resetDimTimer, { passive: true });
        });

        // Start dim timer
        resetDimTimer();
    }

    /**
     * Reset the dim timer
     */
    function resetDimTimer() {
        if (!isActive) return;

        // Clear existing timer
        if (dimTimer) {
            clearTimeout(dimTimer);
        }

        // Wake up if dimmed
        if (isDimmed) {
            wakeFromDim();
        }

        // Set new timer
        dimTimer = setTimeout(() => {
            if (!isAlertActive()) {
                enterDimMode();
            }
        }, DIM_TIMEOUT);
    }

    /**
     * Check if alert is active
     */
    function isAlertActive() {
        const alertOverlay = document.getElementById('ambient-alert');
        return alertOverlay && !alertOverlay.classList.contains('hidden');
    }

    /**
     * Enter dim mode
     */
    function enterDimMode() {
        if (isDimmed) return;

        isDimmed = true;
        document.getElementById('ambient-container')?.classList.add('dimmed');

        // Hide "All Secure" message
        document.getElementById('ambient-secure').style.display = 'none';

        console.log('[Ambient Mode] Entered dim mode');
    }

    /**
     * Wake from dim mode
     */
    function wakeFromDim() {
        if (!isDimmed) return;

        isDimmed = false;
        document.getElementById('ambient-container')?.classList.remove('dimmed');

        console.log('[Ambient Mode] Woke from dim mode');
    }

    // ============================================
    // Morning Briefing
    // ============================================

    /**
     * Check and show morning briefing
     */
    async function checkAndShowBriefing() {
        // Check if briefing was already shown today
        const today = new Date().toISOString().split('T')[0];
        const lastShown = localStorage.getItem('ambient_briefing_last_shown');

        if (lastShown === today) {
            return; // Already shown today
        }

        // Check if this is morning and first detection
        const hour = new Date().getHours();
        if (hour < 6 || hour >= 12) {
            return; // Not morning hours
        }

        // Fetch briefing
        try {
            const response = await fetch(`/api/briefings/${today}`);
            if (response.ok) {
                const briefing = await response.json();

                // Show briefing
                showBriefing(briefing);

                // Mark as shown
                localStorage.setItem('ambient_briefing_last_shown', today);
            }
        } catch (error) {
            console.error('[Ambient Mode] Error fetching briefing:', error);
        }
    }

    /**
     * Show morning briefing
     */
    function showBriefing(briefing) {
        const briefingEl = document.getElementById('ambient-briefing');
        const greetingEl = document.getElementById('briefing-greeting');
        const contentEl = document.getElementById('briefing-content');

        if (!briefingEl) return;

        greetingEl.textContent = getGreeting();

        // Parse and display content
        contentEl.innerHTML = parseBriefingContent(briefing.content);

        briefingEl.classList.remove('hidden');

        // Auto-dismiss after duration
        briefingTimer = setTimeout(() => {
            dismissBriefing();
        }, BRIEFING_DURATION);

        // Wake from dim
        wakeFromDim();
    }

    /**
     * Dismiss morning briefing
     */
    function dismissBriefing() {
        const briefingEl = document.getElementById('ambient-briefing');
        if (briefingEl) {
            briefingEl.classList.add('hidden');
        }

        if (briefingTimer) {
            clearTimeout(briefingTimer);
            briefingTimer = null;
        }
    }

    /**
     * Get greeting based on time of day
     */
    function getGreeting() {
        const hour = new Date().getHours();
        if (hour < 12) return 'Good morning';
        if (hour < 17) return 'Good afternoon';
        return 'Good evening';
    }

    /**
     * Parse briefing content for display
     */
    function parseBriefingContent(content) {
        // Convert plain text to HTML
        const lines = content.split('\n');
        return lines.map(line => {
            if (line.trim() === '') {
                return '<br>';
            }
            return `<div class="ambient-briefing-section-value">${line}</div>`;
        }).join('');
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
        refresh: fetchAmbientData
    };

    // Auto-initialize
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }

    // Monitor hash changes
    window.addEventListener('hashchange', checkAmbientMode);

    console.log('[Ambient Mode] Module loaded');
})();
