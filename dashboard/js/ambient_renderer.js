/**
 * Spaxel Dashboard - Ambient Mode Canvas 2D Renderer
 *
 * Dedicated Canvas 2D rendering engine for ambient display mode.
 * Renders at 2 Hz (one frame every 500ms) for minimal CPU usage.
 * Uses lerp-interpolated positions for smooth person movement.
 */

(function() {
    'use strict';

    // ============================================
    // Configuration
    // ============================================
    const RENDER_INTERVAL_MS = 500;  // 2 Hz = one frame every 500ms
    const LERP_FACTOR = 0.2;          // 20% of remaining distance per frame
    const AUTO_DIM_TIMEOUT_MS = 60000; // 60 seconds of no presence in ambient zone
    const ALERT_PULSE_INTERVAL_MS = 1000; // 1 Hz pulse for alert mode

    // Time-of-day palette colors
    const TIME_COLORS = {
        morning: { bg: '#f0f4f8', text: '#1a365d', accent: '#4299e1' },   // 6-10am
        day:     { bg: '#ffffff', text: '#1d1d1f', accent: '#0066cc' },   // 10am-6pm
        evening: { bg: '#1c1507', text: '#fef3e7', accent: '#ed8936' },  // 6-10pm
        night:   { bg: '#040404', text: '#e0e0e0', accent: '#4fc3f7' }    // 10pm-6am
    };

    // ============================================
    // State
    // ============================================
    let canvas = null;
    let ctx = null;
    let renderTimer = null;
    let lastRenderTime = 0;
    let dimTimer = null;
    let isDimmed = false;
    let alertPulseTimer = null;
    let alertPulseState = false; // for pulsing animation
    let renderCallCount = 0; // Track number of renderFrame calls (for testing)

    // Current state
    let currentState = {
        zones: [],
        blobs: [],
        portals: [],
        nodes: [],
        systemHealth: 'unknown', // 'healthy', 'degraded', 'offline'
        securityMode: false,
        alerts: [],
        lastUpdate: null
    };

    // Target positions for lerp interpolation (blobId -> {x, y, z})
    let targetPositions = new Map();
    // Current interpolated positions (blobId -> {x, y, z})
    let currentPositions = new Map();

    // Expose internal state for testing
    function _getCurrentState() {
        return currentState;
    }

    function _getCurrentPositions() {
        return currentPositions;
    }

    function _getTargetPositions() {
        return targetPositions;
    }

    function _getAlertPulseState() {
        return alertPulseState;
    }

    function _getRenderCallCount() {
        return renderCallCount;
    }

    function _resetRenderCallCount() {
        renderCallCount = 0;
    }

    function _enterDimMode() {
        enterDimMode();
    }

    function _checkAmbientZonePresence() {
        checkAmbientZonePresence();
    }

    // Configuration
    let config = {
        ambientZone: null,  // Zone ID for auto-dim detection
        scale: 50,           // Pixels per meter
        margin: 40           // Canvas margin in pixels
    };

    // Callbacks
    let onAlertClick = null;
    let onUserActivity = null;

    // ============================================
    // Public API
    // ============================================
    const AmbientRenderer = {
        /**
         * Initialize the renderer
         * @param {HTMLCanvasElement} canvasElement - The canvas element to render to
         * @param {Object} rendererConfig - Configuration options
         */
        init(canvasElement, rendererConfig = {}) {
            canvas = canvasElement;
            ctx = canvas.getContext('2d');

            // Apply configuration
            Object.assign(config, rendererConfig);

            // Set up canvas
            resizeCanvas();
            window.addEventListener('resize', resizeCanvas);

            // Set up canvas interaction
            setupCanvasInteraction();

            // Start render loop
            startRenderLoop();

            // Start auto-dim timer
            resetDimTimer();

            console.log('[AmbientRenderer] Initialized');
        },

        /**
         * Update the current state
         * @param {Object} state - New state from WebSocket
         */
        updateState(state) {
            // Update target positions for lerp
            if (state.blobs) {
                state.blobs.forEach(blob => {
                    const target = {
                        x: blob.x,
                        y: blob.y,
                        z: blob.z || 0
                    };
                    targetPositions.set(blob.id, target);

                    // Initialize current position if this is a new blob
                    if (!currentPositions.has(blob.id)) {
                        currentPositions.set(blob.id, { ...target });
                    }
                });

                // Remove blobs that are no longer tracked
                const trackedIds = new Set(state.blobs.map(b => b.id));
                for (const id of currentPositions.keys()) {
                    if (!trackedIds.has(id)) {
                        currentPositions.delete(id);
                        targetPositions.delete(id);
                    }
                }
            }

            // Update state
            if (state.zones) currentState.zones = state.zones;
            if (state.portals) currentState.portals = state.portals;
            if (state.nodes) currentState.nodes = state.nodes;
            if (state.alerts) currentState.alerts = state.alerts;
            if (state.security_mode !== undefined) currentState.securityMode = state.security_mode;
            currentState.lastUpdate = new Date();

            // Check for alerts to update system health
            updateSystemHealth();

            // Check for presence in ambient zone (for auto-dim)
            checkAmbientZonePresence();
        },

        /**
         * Set the ambient zone for auto-dim detection
         * @param {string} zoneId - Zone ID to monitor for presence
         */
        setAmbientZone(zoneId) {
            config.ambientZone = zoneId;
            console.log('[AmbientRenderer] Ambient zone set to:', zoneId);
        },

        /**
         * Set alert click callback
         * @param {Function} callback - Function to call when alert is clicked
         */
        setAlertClickCallback(callback) {
            onAlertClick = callback;
        },

        /**
         * Set user activity callback
         * @param {Function} callback - Function to call on user activity
         */
        setUserActivityCallback(callback) {
            onUserActivity = callback;
        },

        /**
         * Manually trigger a render
         */
        render() {
            if (ctx && canvas) {
                renderFrame();
            }
        },

        /**
         * Enter alert mode
         * @param {Object} alert - Alert data
         */
        enterAlertMode(alert) {
            currentState.alerts = [alert];
            startAlertPulse();
        },

        /**
         * Exit alert mode
         */
        exitAlertMode() {
            currentState.alerts = [];
            stopAlertPulse();
        },

        /**
         * Wake from dim mode
         */
        wakeFromDim() {
            if (isDimmed) {
                isDimmed = false;
                canvas.style.filter = 'brightness(1)';
                canvas.style.transition = 'filter 0.3s ease';
                resetDimTimer();
            }
        },

        /**
         * Get current time period
         * @returns {string} - 'morning', 'day', 'evening', or 'night'
         */
        getTimePeriod() {
            const hour = new Date().getHours();
            if (hour >= 6 && hour < 10) return 'morning';
            if (hour >= 10 && hour < 18) return 'day';
            if (hour >= 18 && hour < 22) return 'evening';
            return 'night';
        },

        /**
         * Clean up resources
         */
        destroy() {
            stopRenderLoop();
            stopAlertPulse();
            if (dimTimer) {
                clearTimeout(dimTimer);
                dimTimer = null;
            }
            window.removeEventListener('resize', resizeCanvas);
            console.log('[AmbientRenderer] Destroyed');
        },

        /**
         * Stop the render loop (for testing)
         */
        stopRenderLoop() {
            stopRenderLoop();
        },

        /**
         * Start the render loop (for testing)
         */
        startRenderLoop() {
            startRenderLoop();
        },

        // Testing/internal methods
        _getCurrentState,
        _getCurrentPositions,
        _getTargetPositions,
        _getAlertPulseState,
        _getRenderCallCount,
        _resetRenderCallCount,
        _enterDimMode,
        _checkAmbientZonePresence
    };

    // ============================================
    // Internal Functions
    // ============================================

    function resizeCanvas() {
        if (!canvas) return;

        const container = canvas.parentElement;
        if (!container) return;

        const rect = container.getBoundingClientRect();
        const dpr = window.devicePixelRatio || 1;

        canvas.width = rect.width * dpr;
        canvas.height = rect.height * dpr;
        canvas.style.width = rect.width + 'px';
        canvas.style.height = rect.height + 'px';

        if (ctx) {
            ctx.scale(dpr, dpr);
        }
    }

    function startRenderLoop() {
        stopRenderLoop();

        function renderLoop(timestamp) {
            // Throttle to 2 Hz (500ms between frames)
            if (timestamp - lastRenderTime >= RENDER_INTERVAL_MS) {
                lastRenderTime = timestamp;
                renderFrame();
            }
            renderTimer = requestAnimationFrame(renderLoop);
        }

        renderTimer = requestAnimationFrame(renderLoop);
    }

    function stopRenderLoop() {
        if (renderTimer) {
            cancelAnimationFrame(renderTimer);
            renderTimer = null;
        }
    }

    function renderFrame() {
        if (!ctx || !canvas) return;

        renderCallCount++; // Track render calls for testing

        const width = canvas.width / (window.devicePixelRatio || 1);
        const height = canvas.height / (window.devicePixelRatio || 1);

        // Clear canvas
        ctx.clearRect(0, 0, width, height);

        // Get current time period and colors
        const period = AmbientRenderer.getTimePeriod();
        const colors = TIME_COLORS[period];

        // Draw background
        drawBackground(ctx, width, height, colors);

        // Check for alert mode
        const hasActiveAlert = currentState.alerts.length > 0;
        if (hasActiveAlert) {
            drawAlertMode(ctx, width, height);
            return; // Alert mode takes over the entire canvas
        }

        // Calculate floor plan bounds
        const bounds = calculateBounds(width, height);

        // Draw zones (room outlines)
        drawZones(ctx, bounds, colors);

        // Draw portals
        drawPortals(ctx, bounds, colors);

        // Draw nodes
        drawNodes(ctx, bounds, colors);

        // Draw people (with lerp-interpolated positions)
        drawPeople(ctx, bounds, colors);

        // Draw system status indicator (top-left)
        drawSystemStatus(ctx, colors);

        // Draw time display (top-right)
        drawTimeDisplay(ctx, width, colors);
    }

    function drawBackground(ctx, width, height, colors) {
        // Solid background color based on time of day
        ctx.fillStyle = colors.bg;
        ctx.fillRect(0, 0, width, height);
    }

    function drawAlertMode(ctx, width, height) {
        // Pulsing red background for alert mode
        const pulseColor = alertPulseState ? '#dc2626' : '#991b1b';
        ctx.fillStyle = pulseColor;
        ctx.fillRect(0, 0, width, height);

        // Draw alert text
        const alert = currentState.alerts[0];
        if (alert) {
            ctx.fillStyle = '#ffffff';
            ctx.font = 'bold 48px -apple-system, BlinkMacSystemFont, sans-serif';
            ctx.textAlign = 'center';
            ctx.textBaseline = 'middle';

            const title = alert.type === 'fall_alert' ? 'FALL DETECTED' : 'ALERT';
            const message = formatAlertMessage(alert);

            ctx.fillText(title, width / 2, height / 2 - 30);
            ctx.font = '24px -apple-system, BlinkMacSystemFont, sans-serif';
            ctx.fillText(message, width / 2, height / 2 + 30);

            // Draw acknowledge button
            const buttonWidth = 200;
            const buttonHeight = 60;
            const buttonX = (width - buttonWidth) / 2;
            const buttonY = height / 2 + 80;

            ctx.fillStyle = '#ffffff';
            ctx.beginPath();
            ctx.roundRect(buttonX, buttonY, buttonWidth, buttonHeight, 8);
            ctx.fill();

            ctx.fillStyle = '#dc2626';
            ctx.font = 'bold 20px -apple-system, BlinkMacSystemFont, sans-serif';
            ctx.fillText('Acknowledge', width / 2, buttonY + buttonHeight / 2);
        }
    }

    function formatAlertMessage(alert) {
        if (alert.type === 'fall_alert') {
            const person = alert.person || 'Someone';
            return `${person} has fallen`;
        } else if (alert.type === 'anomaly') {
            return 'Unusual activity detected';
        }
        return 'Alert detected';
    }

    function calculateBounds(canvasWidth, canvasHeight) {
        // Find bounds of all zones
        if (currentState.zones.length === 0) {
            // Default bounds - centered square
            const size = Math.min(canvasWidth, canvasHeight) - config.margin * 2;
            return {
                x: (canvasWidth - size) / 2,
                y: (canvasHeight - size) / 2,
                width: size,
                height: size,
                scale: size / 10, // 10 meters default
                minX: 0,
                minY: 0
            };
        }

        let minX = Infinity, minY = Infinity;
        let maxX = -Infinity, maxY = -Infinity;

        currentState.zones.forEach(zone => {
            const x = zone.x || zone.MinX || 0;
            const y = zone.y || zone.MinY || 0;
            const w = zone.w || zone.SizeX || zone.w || 1;
            const d = zone.d || zone.SizeY || zone.d || 1;

            minX = Math.min(minX, x);
            minY = Math.min(minY, y);
            maxX = Math.max(maxX, x + w);
            maxY = Math.max(maxY, y + d);
        });

        // Add margin
        const marginMeters = 1; // 1 meter margin
        minX -= marginMeters;
        minY -= marginMeters;
        maxX += marginMeters;
        maxY += marginMeters;

        const floorWidth = maxX - minX;
        const floorHeight = maxY - minY;

        // Calculate scale to fit canvas
        const scaleX = (canvasWidth - config.margin * 2) / floorWidth;
        const scaleY = (canvasHeight - config.margin * 2) / floorHeight;
        const scale = Math.min(scaleX, scaleY, config.scale);

        // Calculate centered bounds
        const boundsWidth = floorWidth * scale;
        const boundsHeight = floorHeight * scale;
        const boundsX = (canvasWidth - boundsWidth) / 2;
        const boundsY = (canvasHeight - boundsHeight) / 2;

        return {
            x: boundsX,
            y: boundsY,
            width: boundsWidth,
            height: boundsHeight,
            scale: scale,
            minX: minX,
            minY: minY
        };
    }

    function worldToScreen(wx, wy, bounds) {
        return {
            x: bounds.x + (wx - bounds.minX) * bounds.scale,
            y: bounds.y + (wy - bounds.minY) * bounds.scale
        };
    }

    function drawZones(ctx, bounds, colors) {
        currentState.zones.forEach(zone => {
            const x = zone.x || zone.MinX || 0;
            const y = zone.y || zone.MinY || 0;
            const w = zone.w || zone.SizeX || 1;
            const d = zone.d || zone.SizeY || 1;

            const topLeft = worldToScreen(x, y, bounds);
            const width = w * bounds.scale;
            const height = d * bounds.scale;

            // Zone outline (white, 1px stroke)
            ctx.strokeStyle = '#ffffff';
            ctx.lineWidth = 1;
            ctx.strokeRect(topLeft.x, topLeft.y, width, height);

            // Zone label at centroid
            const centerX = topLeft.x + width / 2;
            const centerY = topLeft.y + height / 2;

            const count = zone.count || zone.occupancy || 0;
            const zoneName = zone.name || zone.Name || 'Zone';

            ctx.fillStyle = '#ffffff';
            ctx.font = '14px -apple-system, BlinkMacSystemFont, sans-serif';
            ctx.textAlign = 'center';
            ctx.textBaseline = 'middle';
            ctx.fillText(zoneName, centerX, centerY);

            if (count > 0) {
                ctx.font = '12px -apple-system, BlinkMacSystemFont, sans-serif';
                ctx.fillText(`(${count})`, centerX, centerY + 16);
            }
        });
    }

    function drawPortals(ctx, bounds, colors) {
        if (!currentState.portals || currentState.portals.length === 0) {
            return;
        }

        ctx.strokeStyle = '#a855f7'; // Purple
        ctx.lineWidth = 0.5;

        currentState.portals.forEach(portal => {
            // Portal is defined by two points
            const p1 = worldToScreen(portal.p1_x || 0, portal.p1_y || 0, bounds);
            const p2 = worldToScreen(portal.p2_x || 0, portal.p2_y || 0, bounds);

            ctx.beginPath();
            ctx.moveTo(p1.x, p1.y);
            ctx.lineTo(p2.x, p2.y);
            ctx.stroke();
        });
    }

    function drawNodes(ctx, bounds, colors) {
        currentState.nodes.forEach(node => {
            const x = node.pos_x || node.PosX || 0;
            const y = node.pos_y || node.PosY || 0;

            const pos = worldToScreen(x, y, bounds);

            // Small filled circle (4px radius)
            ctx.fillStyle = '#6b7280'; // Grey
            ctx.beginPath();
            ctx.arc(pos.x, pos.y, 4, 0, Math.PI * 2);
            ctx.fill();
        });
    }

    function drawPeople(ctx, bounds, colors) {
        currentState.blobs.forEach(blob => {
            // Get current position (with lerp interpolation)
            let pos = currentPositions.get(blob.id);
            if (!pos) {
                pos = { x: blob.x, y: blob.y, z: blob.z || 0 };
                currentPositions.set(blob.id, pos);
            }

            // Get target position
            const target = targetPositions.get(blob.id);
            if (target) {
                // Lerp toward target (20% of remaining distance)
                pos.x = lerp(pos.x, target.x, LERP_FACTOR);
                pos.y = lerp(pos.y, target.y, LERP_FACTOR);
                pos.z = lerp(pos.z, target.z, LERP_FACTOR);

                // Update the currentPositions map with the lerped position
                currentPositions.set(blob.id, { ...pos });
            }

            const screenPos = worldToScreen(pos.x, pos.y, bounds);

            // Blob radius proportional to identity confidence
            const confidence = blob.confidence || 0.5;
            const radius = 10 + (confidence * 8); // 10-18px

            // Get person color
            let blobColor = '#6b7280'; // Grey for unknown
            if (blob.person) {
                blobColor = getPersonColor(blob.person);
            }

            // Draw person blob
            ctx.fillStyle = blobColor;
            ctx.beginPath();
            ctx.arc(screenPos.x, screenPos.y, radius, 0, Math.PI * 2);
            ctx.fill();

            // Draw name label above
            const name = blob.person ? getFirstName(blob.person) : '?';
            ctx.fillStyle = '#ffffff';
            ctx.font = '12px -apple-system, BlinkMacSystemFont, sans-serif';
            ctx.textAlign = 'center';
            ctx.fillText(name, screenPos.x, screenPos.y - radius - 4);
        });
    }

    function drawSystemStatus(ctx, colors) {
        const size = 16; // 8px radius = 16px diameter
        const margin = 16;
        const x = margin + size / 2;
        const y = margin + size / 2;

        // Determine status color
        let statusColor;
        if (currentState.alerts.length > 0) {
            statusColor = '#ef4444'; // Red - alert
        } else {
            // Check node health
            const onlineNodes = currentState.nodes.filter(n => n.status === 'online').length;
            const totalNodes = currentState.nodes.length;

            if (onlineNodes === 0 && totalNodes > 0) {
                statusColor = '#ef4444'; // Red - all offline
            } else if (onlineNodes < totalNodes) {
                statusColor = '#f59e0b'; // Amber - some degraded
            } else {
                statusColor = '#22c55e'; // Green - all healthy
            }
        }

        // Draw status dot
        ctx.fillStyle = statusColor;
        ctx.beginPath();
        ctx.arc(x, y, 8, 0, Math.PI * 2);
        ctx.fill();
    }

    function drawTimeDisplay(ctx, canvasWidth, colors) {
        const now = new Date();
        const timeStr = now.toLocaleTimeString('en-US', {
            hour: 'numeric',
            minute: '2-digit',
            hour12: true
        });

        ctx.fillStyle = colors.text;
        ctx.font = '28px -apple-system, BlinkMacSystemFont, monospace';
        ctx.textAlign = 'right';
        ctx.textBaseline = 'top';
        ctx.fillText(timeStr, canvasWidth - 16, 16);
    }

    // ============================================
    // Auto-Dim Logic
    // ============================================

    function resetDimTimer() {
        if (dimTimer) {
            clearTimeout(dimTimer);
        }

        dimTimer = setTimeout(() => {
            enterDimMode();
        }, AUTO_DIM_TIMEOUT_MS);
    }

    function enterDimMode() {
        isDimmed = true;
        // Reduce canvas brightness to 40%
        canvas.style.filter = 'brightness(0.4)';
        canvas.style.transition = 'filter 0.5s ease';
        console.log('[AmbientRenderer] Entered dim mode');
    }

    function checkAmbientZonePresence() {
        if (!config.ambientZone) {
            return; // No ambient zone configured
        }

        // Check if anyone is in the ambient zone
        const zone = currentState.zones.find(z => z.id === config.ambientZone || z.name === config.ambientZone);
        if (zone && (zone.count > 0 || zone.occupancy > 0)) {
            // Someone is present - wake from dim
            if (isDimmed) {
                AmbientRenderer.wakeFromDim();
            }
            // Reset the timer
            resetDimTimer();
        }
    }

    // ============================================
    // Alert Pulse Animation
    // ============================================

    function startAlertPulse() {
        if (alertPulseTimer) {
            return; // Already running
        }

        alertPulseTimer = setInterval(() => {
            alertPulseState = !alertPulseState;
            // Force immediate render
            renderFrame();
        }, ALERT_PULSE_INTERVAL_MS);
    }

    function stopAlertPulse() {
        if (alertPulseTimer) {
            clearInterval(alertPulseTimer);
            alertPulseTimer = null;
        }
        alertPulseState = false;
    }

    // ============================================
    // Helper Functions
    // ============================================

    function lerp(start, end, factor) {
        return start + (end - start) * factor;
    }

    function getPersonColor(personName) {
        // Generate consistent color from name
        let hash = 0;
        for (let i = 0; i < personName.length; i++) {
            hash = personName.charCodeAt(i) + ((hash << 5) - hash);
        }
        const hue = Math.abs(hash) % 360;
        return `hsl(${hue}, 70%, 50%)`;
    }

    function getFirstName(fullName) {
        if (!fullName) return '?';
        const parts = fullName.trim().split(/\s+/);
        return parts[0];
    }

    function updateSystemHealth() {
        if (currentState.alerts.length > 0) {
            currentState.systemHealth = 'alert';
            return;
        }

        const onlineNodes = currentState.nodes.filter(n => n.status === 'online').length;
        const totalNodes = currentState.nodes.length;

        if (totalNodes === 0) {
            currentState.systemHealth = 'unknown';
        } else if (onlineNodes === 0) {
            currentState.systemHealth = 'offline';
        } else if (onlineNodes < totalNodes) {
            currentState.systemHealth = 'degraded';
        } else {
            currentState.systemHealth = 'healthy';
        }
    }

    // ============================================
    // Canvas Interaction
    // ============================================

    function setupCanvasInteraction() {
        if (!canvas) return;

        // Handle clicks on canvas (for alert acknowledgment)
        canvas.addEventListener('click', (e) => {
            const rect = canvas.getBoundingClientRect();
            const x = e.clientX - rect.left;
            const y = e.clientY - rect.top;

            // Check if click is on acknowledge button in alert mode
            if (currentState.alerts.length > 0) {
                const width = canvas.width / (window.devicePixelRatio || 1);
                const height = canvas.height / (window.devicePixelRatio || 1);

                const buttonWidth = 200;
                const buttonHeight = 60;
                const buttonX = (width - buttonWidth) / 2;
                const buttonY = height / 2 + 80;

                if (x >= buttonX && x <= buttonX + buttonWidth &&
                    y >= buttonY && y <= buttonY + buttonHeight) {
                    if (onAlertClick) {
                        onAlertClick(currentState.alerts[0]);
                    }
                }
            }

            // Any click wakes from dim and resets timer
            wakeFromDim();
            if (onUserActivity) {
                onUserActivity();
            }
        });

        // Handle touch events
        canvas.addEventListener('touchstart', (e) => {
            wakeFromDim();
            if (onUserActivity) {
                onUserActivity();
            }
        }, { passive: true });
    }

    // ============================================
    // Export
    // ============================================
    window.SpaxelAmbientRenderer = AmbientRenderer;

    console.log('[AmbientRenderer] Module loaded');
})();
