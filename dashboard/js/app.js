/**
 * Spaxel Dashboard - Main Application
 *
 * Phase 1 skeleton: 3D scene with ground grid, OrbitControls,
 * WebSocket connection, and amplitude bar chart visualization.
 */

(function() {
    'use strict';

    // ============================================
    // Configuration
    // ============================================
    const CONFIG = {
        gridWidth: 10,       // meters
        gridDepth: 10,       // meters
        gridDivisions: 20,
        cameraFov: 60,
        cameraNear: 0.1,
        cameraFar: 1000,
        cameraInitial: { x: 8, y: 8, z: 8 },
        chartBars: 64,       // number of subcarriers
        chartUpdateMs: 100,  // update chart at 10 Hz max
        tsMaxPoints: 360,    // max time series samples per link (~60s at 6Hz)
        tsMinIntervalMs: 100, // min ms between time series samples
        drTsWindowMs: 10000, // deltaRMS time series window: 10 seconds
        drTsMaxPoints: 100,  // max deltaRMS samples per link
        drThreshold: 0.02,   // DefaultDeltaRMSThreshold
        healthPollIntervalMs: 10000,  // poll /api/links every 10 seconds
        diurnalPollIntervalMs: 30000  // poll /api/diurnal/status every 30 seconds
    };

    // ============================================
    // State
    // ============================================
    const state = {
        ws: null,
        wsConnected: false,
        awaitingSnapshot: false,   // true between WS open and first snapshot message
        nodes: new Map(),        // MAC -> { mac, firmware, chip, lastSeen }
        links: new Map(),        // linkID -> { nodeMAC, peerMAC, lastFrame, lastCSI, motionDetected, deltaRMS, ampHistory, lastAmpSample }
        selectedLinkID: null,
        presenceSelectedLinkID: null,
        drHistory: new Map(),    // linkID -> [{ t: number, rms: number }]
        lastChartUpdate: 0,
        frameCount: 0,
        lastFpsTime: performance.now(),
        // System health tracking
        systemHealth: 0,
        worstLinkID: null,
        worstLinkScore: 1.0,
        // Diurnal learning tracking
        diurnalStatus: new Map(),  // linkID -> { is_learning, progress, is_ready, days_remaining }
        diurnalPollTimer: null,
        healthPollTimer: null,
        // BLE device tracking
        bleDevices: new Map(),     // MAC -> { mac, name, rssi, last_seen, label, blob_id }
        // Alert tracking
        alerts: new Map(),         // id -> { id, ts, severity, description, acknowledged }
        unacknowledgedCount: 0,
        // Event dedup: set of recently processed event IDs to avoid double-processing
        // from immediate broadcast + delta buffering
        recentEventIDs: new Set(),
        recentEventIDsPruneAt: 0
    };

    // ============================================
    // Three.js Scene Setup
    // ============================================
    let scene, camera, renderer, controls, gridHelper, axesHelper, clock;

    function initScene() {
        const container = document.getElementById('scene-container');

        // Scene
        scene = new THREE.Scene();
        scene.background = new THREE.Color(0x1a1a2e);

        // Camera
        camera = new THREE.PerspectiveCamera(
            CONFIG.cameraFov,
            window.innerWidth / window.innerHeight,
            CONFIG.cameraNear,
            CONFIG.cameraFar
        );
        camera.position.set(
            CONFIG.cameraInitial.x,
            CONFIG.cameraInitial.y,
            CONFIG.cameraInitial.z
        );

        // Renderer
        renderer = new THREE.WebGLRenderer({ antialias: true });
        renderer.setSize(window.innerWidth, window.innerHeight);
        renderer.setPixelRatio(window.devicePixelRatio);
        container.appendChild(renderer.domElement);

        // OrbitControls
        controls = new THREE.OrbitControls(camera, renderer.domElement);
        controls.enableDamping = true;
        controls.dampingFactor = 0.05;
        controls.screenSpacePanning = true;
        controls.minDistance = 2;
        controls.maxDistance = 50;
        controls.maxPolarAngle = Math.PI / 2 + 0.3; // Allow slight below-ground view

        // Grid helper (XZ plane, Y-up)
        gridHelper = new THREE.GridHelper(
            CONFIG.gridWidth,
            CONFIG.gridDivisions,
            0x444466,  // center line color
            0x333344   // grid line color
        );
        scene.add(gridHelper);

        // Axes helper for orientation
        axesHelper = new THREE.AxesHelper(2);
        axesHelper.position.set(-CONFIG.gridWidth / 2, 0.01, -CONFIG.gridDepth / 2);
        scene.add(axesHelper);

        // Ambient light
        const ambientLight = new THREE.AmbientLight(0xffffff, 0.6);
        scene.add(ambientLight);

        // Directional light
        const directionalLight = new THREE.DirectionalLight(0xffffff, 0.4);
        directionalLight.position.set(5, 10, 5);
        scene.add(directionalLight);

        // Handle window resize
        window.addEventListener('resize', onWindowResize);

        // Clock for animation mixer delta
        clock = new THREE.Clock();

        // Initialise 3-D spatial visualisation layer
        Viz3D.init(scene, camera, controls);

        // Initialise placement (TransformControls, GDOP, room editor)
        if (window.Placement) {
            Placement.init(scene, camera, renderer, controls);
        }

        console.log('[Spaxel] Scene initialized');
    }

    function onWindowResize() {
        camera.aspect = window.innerWidth / window.innerHeight;
        camera.updateProjectionMatrix();
        renderer.setSize(window.innerWidth, window.innerHeight);
    }

    function animate() {
        requestAnimationFrame(animate);
        controls.update();
        Viz3D.update();
        if (window.Placement) Placement.update();
        renderer.render(scene, camera);
        updateFPS();
    }

    // ============================================
    // System Quality Gauge
    // ============================================
    function updateQualityGauge(score, linkCount, worstLinkID, worstScore) {
        const valueEl = document.getElementById('quality-value');
        const fillEl = document.getElementById('quality-gauge-fill');
        const linkCountEl = document.getElementById('quality-link-count');
        const worstLinkEl = document.getElementById('quality-worst-link');
        const worstScoreEl = document.getElementById('quality-worst-score');

        if (!valueEl || !fillEl) return;

        // Update percentage display
        const pct = Math.round(score * 100);
        valueEl.textContent = pct + '%';

        // Update circular gauge (stroke-dasharray: circumference = 2 * PI * r = ~81.7 for r=13)
        const circumference = 2 * Math.PI * 13;
        const dashLength = (score * circumference).toFixed(1);
        fillEl.setAttribute('stroke-dasharray', dashLength + ' ' + circumference);

        // Update color based on score
        let color;
        if (score >= 0.7) {
            color = '#66bb6a'; // green
        } else if (score >= 0.4) {
            color = '#eab308'; // yellow
        } else {
            color = '#ef4444'; // red
        }
        fillEl.setAttribute('stroke', color);

        // Update tooltip
        if (linkCountEl) linkCountEl.textContent = linkCount;
        if (worstLinkEl) worstLinkEl.textContent = worstLinkID ? abbreviateLinkID(worstLinkID) : '--';
        if (worstScoreEl) worstScoreEl.textContent = worstScore !== null ? Math.round(worstScore * 100) + '%' : '--';
    }

    function startHealthPolling() {
        if (state.healthPollTimer) {
            clearInterval(state.healthPollTimer);
        }

        fetchLinkHealth();

        state.healthPollTimer = setInterval(fetchLinkHealth, CONFIG.healthPollIntervalMs);
    }

    function fetchLinkHealth() {
        fetch('/api/links')
            .then(function(res) { return res.json(); })
            .then(function(links) {
                handleLinkHealthUpdate(links);
            })
            .catch(function(err) {
                console.error('[Spaxel] Failed to fetch link health:', err);
            });
    }

    function handleLinkHealthUpdate(links) {
        if (!links || links.length === 0) {
            state.systemHealth = 0;
            state.worstLinkID = null;
            state.worstLinkScore = 1.0;
            updateQualityGauge(0, 0, null, null);
            return;
        }

        // Calculate system health (weighted average of all links)
        var totalScore = 0;
        var worstScore = 1.0;
        var worstID = null;

        links.forEach(function(link) {
            var score = link.health_score !== undefined ? link.health_score : 0.5;
            totalScore += score;

            if (score < worstScore) {
                worstScore = score;
                worstID = link.link_id;
            }
        });

        state.systemHealth = totalScore / links.length;
        state.worstLinkID = worstID;
        state.worstLinkScore = worstScore;

        updateQualityGauge(state.systemHealth, links.length, worstID, worstScore);

        // Also update 3D visualization
        if (window.Viz3D && window.Viz3D.updateLinkHealth) {
            Viz3D.updateLinkHealth(links);
        }

        // Also update LinkHealth panel
        if (window.LinkHealth && window.LinkHealth.updateLinkHealth) {
            LinkHealth.updateLinkHealth(links);
        }
    }

    // ============================================
    // Diurnal Learning Status
    // ============================================
    function startDiurnalPolling() {
        if (state.diurnalPollTimer) {
            clearInterval(state.diurnalPollTimer);
        }

        fetchDiurnalStatus();

        state.diurnalPollTimer = setInterval(fetchDiurnalStatus, CONFIG.diurnalPollIntervalMs);
    }

    function fetchDiurnalStatus() {
        fetch('/api/diurnal/status')
            .then(function(res) { return res.json(); })
            .then(function(statuses) {
                handleDiurnalStatusUpdate(statuses);
            })
            .catch(function(err) {
                console.error('[Spaxel] Failed to fetch diurnal status:', err);
            });
    }

    function handleDiurnalStatusUpdate(statuses) {
        if (!statuses || statuses.length === 0) {
            updateDiurnalBanner(null);
            return;
        }

        // Find the link with the longest remaining learning time
        var worstStatus = null;
        statuses.forEach(function(status) {
            state.diurnalStatus.set(status.link_id, status);
            if (!worstStatus || status.days_remaining > worstStatus.days_remaining) {
                if (status.is_learning) {
                    worstStatus = status;
                }
            }
        });

        updateDiurnalBanner(worstStatus);
    }

    function updateDiurnalBanner(status) {
        var banner = document.getElementById('diurnal-banner');
        var message = document.getElementById('diurnal-message');
        var progress = document.getElementById('diurnal-progress');
        var daysLeft = document.getElementById('diurnal-days-left');

        if (!banner) return;

        if (!status || !status.is_learning) {
            banner.classList.remove('visible');
            return;
        }

        banner.classList.add('visible');

        if (message) {
            message.textContent = 'Learning your home\'s daily patterns...';
        }

        if (progress) {
            var pct = Math.min(100, Math.max(0, status.progress || 0));
            progress.style.width = pct + '%';
        }

        if (daysLeft) {
            var days = Math.ceil(status.days_remaining || 0);
            if (days > 0) {
                daysLeft.textContent = days + (days === 1 ? ' day left' : ' days left');
            } else {
                daysLeft.textContent = 'Almost ready...';
            }
        }
    }

    function showToast(message, type) {
        var container = document.getElementById('toast-container');
        if (!container) return;

        var toast = document.createElement('div');
        toast.className = 'toast toast-' + (type || 'info');
        toast.textContent = message;

        container.appendChild(toast);

        setTimeout(function() {
            toast.classList.add('fade-out');
            setTimeout(function() {
                if (toast.parentNode) {
                    toast.parentNode.removeChild(toast);
                }
            }, 300);
        }, 5000);
    }

    function updateFPS() {
        state.frameCount++;
        const now = performance.now();
        const elapsed = now - state.lastFpsTime;
        if (elapsed >= 1000) {
            const fps = Math.round(state.frameCount * 1000 / elapsed);
            document.getElementById('fps-counter').textContent = fps;
            state.frameCount = 0;
            state.lastFpsTime = now;
        }
    }

    // ============================================
    // WebSocket Connection (via SpaxelWebSocket)
    // ============================================
    function connectWebSocket() {
        // Initialize the WebSocket manager with callbacks
        SpaxelWebSocket.init({
            onOpen: function(ws) {
                console.log('[Spaxel] WebSocket connected');
                state.wsConnected = true;
                state.awaitingSnapshot = true;
            },
            onMessage: function(data) {
                handleMessage(data);
            },
            onClose: function(event) {
                console.log('[Spaxel] WebSocket closed:', event.code, event.reason);
                state.wsConnected = false;

                // Start blob extrapolation using captured blob states
                SpaxelWebSocket.startExtrapolation();
            },
            onError: function(error) {
                console.error('[Spaxel] WebSocket error:', error);
            }
        });

        SpaxelWebSocket.connect();
    }

    // ============================================
    // Message Handling
    // ============================================

    // Event handlers for new message types

    function handleEventMessage(msg) {
        if (!msg.event) return;

        const event = msg.event;

        // Dedup: skip if we already processed this event (from immediate broadcast or prior delta)
        const eid = String(event.id);
        if (state.recentEventIDs.has(eid)) return;
        state.recentEventIDs.add(eid);

        // Prune old IDs every 30 seconds to prevent unbounded growth
        const now = Date.now();
        if (now > state.recentEventIDsPruneAt) {
            state.recentEventIDs.clear();
            state.recentEventIDsPruneAt = now + 30000;
        }

        console.log('[Spaxel] Event:', event.kind, 'in', event.zone, 'by', event.person_name || 'blob #' + event.blob_id);

        // Log to timeline
        const timeStr = new Date(event.ts).toLocaleTimeString();
        let description = '';

        if (event.kind === 'zone_entry') {
            description = (event.person_name || 'Someone') + ' entered ' + event.zone;
        } else if (event.kind === 'zone_exit') {
            description = (event.person_name || 'Someone') + ' left ' + event.zone;
        } else if (event.kind === 'portal_crossing') {
            description = (event.person_name || 'Someone') + ' crossed portal in ' + event.zone;
        } else if (event.kind === 'presence_transition') {
            description = (event.person_name || 'Someone') + ' presence detected in ' + event.zone;
        } else {
            description = event.kind + ' in ' + event.zone;
        }

        logTimelineEvent(event.kind, null, description + ' (' + timeStr + ')');

        // Show toast for security-relevant events
        if (event.kind === 'zone_entry' || event.kind === 'portal_crossing') {
            showToast(description, 'info');
        }
    }

    function handleAlertMessage(msg) {
        if (!msg.alert) return;

        const alert = msg.alert;
        console.log('[Spaxel] Alert:', alert.severity, alert.description);

        // Show toast notification
        const toastType = alert.severity === 'critical' ? 'error' : 'warning';
        showToast(alert.description, toastType);

        // Log to timeline
        const timeStr = new Date(alert.ts).toLocaleTimeString();
        logTimelineEvent('alert', null, '[' + alert.severity.toUpperCase() + '] ' + alert.description + ' (' + timeStr + ')');

        // Could trigger UI alert state here (e.g., show alert banner)
        if (window.showAlertBanner) {
            window.showAlertBanner(alert);
        }
    }

    function handleBLEScanMessage(msg) {
        if (!msg.devices || !Array.isArray(msg.devices)) return;

        console.log('[Spaxel] BLE scan: ' + msg.devices.length + ' devices');

        // Update BLE device list state
        if (!state.bleDevices) {
            state.bleDevices = new Map();
        }

        // Clear previous entries and add current devices
        state.bleDevices.clear();
        msg.devices.forEach(function (device) {
            state.bleDevices.set(device.mac || device.addr, {
                mac: device.mac || device.addr,
                name: device.name || device.device_name || 'Unknown',
                rssi: device.rssi || device.rssi_dbm || 0,
                last_seen: device.last_seen || Date.now(),
                label: device.label || '',
                blob_id: device.blob_id || null
            });
        });

        // Update UI if BLE panel exists
        if (window.BLEPanel && window.BLEPanel.updateDevices) {
            window.BLEPanel.updateDevices(msg.devices);
        }
    }

    function handleTriggerStateMessage(msg) {
        if (!msg.trigger) return;

        const trigger = msg.trigger;
        console.log('[Spaxel] Trigger state:', trigger.name, 'enabled=' + trigger.enabled);

        // Update trigger state in UI if automation panel exists
        if (window.Automations && window.Automations.updateTriggerState) {
            window.Automations.updateTriggerState(trigger);
        }
    }

    function handleSystemHealthMessage(msg) {
        if (!msg.health) return;

        const health = msg.health;

        // Update system health display in UI
        const healthEl = document.getElementById('system-uptime');
        if (healthEl) {
            const uptimeSec = health.uptime_s || 0;
            const days = Math.floor(uptimeSec / 86400);
            const hours = Math.floor((uptimeSec % 86400) / 3600);
            const mins = Math.floor((uptimeSec % 3600) / 60);
            if (days > 0) {
                healthEl.textContent = days + 'd ' + hours + 'h ' + mins + 'm';
            } else {
                healthEl.textContent = hours + 'h ' + mins + 'm';
            }
        }

        const memEl = document.getElementById('system-memory');
        if (memEl) {
            memEl.textContent = (health.mem_mb || 0).toFixed(1) + ' MB';
        }

        const goroutinesEl = document.getElementById('system-goroutines');
        if (goroutinesEl) {
            goroutinesEl.textContent = health.go_routines || 0;
        }
    }

    function handleMessage(data) {
        if (typeof data === 'string') {
            // JSON message
            try {
                const msg = JSON.parse(data);
                handleJSONMessage(msg);
            } catch (e) {
                console.error('[Spaxel] Failed to parse JSON:', e);
            }
        } else if (data instanceof ArrayBuffer) {
            // Binary CSI frame
            handleBinaryFrame(data);
        }
    }

    function handleJSONMessage(msg) {
        // Snapshot: first message on every connect/reconnect.  Contains full state.
        if (msg.type === 'snapshot') {
            handleSnapshot(msg);
            return;
        }

        // Incremental update: 10 Hz delta with no type field.
        // Only fields that changed since last tick are present.
        if (!msg.type) {
            handleIncrementalUpdate(msg);
            return;
        }

        switch (msg.type) {
            case 'state':
                // Initial state dump
                if (msg.nodes) {
                    msg.nodes.forEach(node => {
                        state.nodes.set(node.mac, {
                            mac: node.mac,
                            firmware: node.firmware_version,
                            chip: node.chip,
                            lastSeen: Date.now()
                        });
                    });
                }
                if (msg.links) {
                    msg.links.forEach(link => {
                        const existing = state.links.get(link.id) || {};
                        state.links.set(link.id, {
                            nodeMAC: link.node_mac,
                            peerMAC: link.peer_mac,
                            lastFrame: Date.now(),
                            lastCSI: existing.lastCSI || null,
                            motionDetected: existing.motionDetected || false,
                            deltaRMS: existing.deltaRMS || 0,
                            ampHistory: existing.ampHistory || [],
                            lastAmpSample: existing.lastAmpSample || 0
                        });
                    });
                }
                if (msg.motion_states) {
                    msg.motion_states.forEach(ms => applyMotionState(ms));
                }
                updateNodeList();
                updateLinkList();
                Viz3D.applyLinks(msg.links || []);
                break;

            case 'node_connected':
                state.nodes.set(msg.mac, {
                    mac: msg.mac,
                    firmware: msg.firmware_version,
                    chip: msg.chip,
                    lastSeen: Date.now()
                });
                updateNodeList();
                if (window.SpaxelTroubleshoot) {
                    window.SpaxelTroubleshoot.handleEvent('node_connected', msg);
                }
                // Show first-time tooltips on first node connection
                if (window.SpaxelTooltips && state.nodes.size === 1) {
                    setTimeout(function () { window.SpaxelTooltips.showSequence(); }, 2000);
                }
                break;

            case 'node_disconnected':
                state.nodes.delete(msg.mac);
                updateNodeList();
                if (window.SpaxelTroubleshoot) {
                    window.SpaxelTroubleshoot.handleEvent('node_disconnected', msg);
                }
                break;

            case 'link_active':
                if (!state.links.has(msg.id)) {
                    state.links.set(msg.id, {
                        nodeMAC: msg.node_mac,
                        peerMAC: msg.peer_mac,
                        lastFrame: Date.now(),
                        lastCSI: null,
                        motionDetected: false,
                        deltaRMS: 0,
                        ampHistory: [],
                        lastAmpSample: 0
                    });
                    updateLinkList();
                }
                Viz3D.handleLinkActive(msg);
                break;

            case 'link_inactive':
                state.links.delete(msg.id);
                updateLinkList();
                Viz3D.handleLinkInactive(msg);
                break;

            case 'motion_state':
                // Targeted broadcast on state change
                if (msg.links) {
                    let changed = false;
                    msg.links.forEach(ms => {
                        if (applyMotionState(ms)) changed = true;
                    });
                    if (changed) updateLinkList();
                }
                break;

            case 'presence_update':
                handlePresenceUpdate(msg);
                break;

            case 'registry_state':
                Viz3D.handleRegistryState(msg);
                if (window.Placement) Placement.handleRegistryState(msg);
                // Merge virtual nodes into local state and refresh node list
                if (msg.nodes) {
                    var registryMACs = new Set(msg.nodes.map(function (n) { return n.mac; }));
                    msg.nodes.forEach(function (node) {
                        if (!state.nodes.has(node.mac)) {
                            state.nodes.set(node.mac, {
                                mac: node.mac,
                                firmware: '',
                                chip: '',
                                lastSeen: 0,
                                virtual: !!node.virtual
                            });
                        } else {
                            var existing = state.nodes.get(node.mac);
                            if (node.virtual) existing.virtual = true;
                        }
                    });
                    // Remove virtual nodes no longer in registry
                    state.nodes.forEach(function (node, mac) {
                        if (!registryMACs.has(mac) && node.virtual) {
                            state.nodes.delete(mac);
                        }
                    });
                    updateNodeList();
                }
                break;

            case 'loc_update':
                Viz3D.handleLocUpdate(msg);
                break;

            case 'link_health':
                // Health score update from server
                if (msg.links) {
                    handleLinkHealthUpdate(msg.links);
                }
                break;

            case 'diurnal_ready':
                // Diurnal patterns learned notification
                showToast(msg.message || 'Daily patterns learned! Detection accuracy improved.', 'success');
                // Refresh diurnal status
                fetchDiurnalStatus();
                break;

            case 'event':
                // Event: presence transition, zone entry/exit, portal crossing
                handleEventMessage(msg);
                break;

            case 'alert':
                // Alert: anomaly detection, security mode trigger
                handleAlertMessage(msg);
                break;

            case 'ble_scan':
                // BLE device list update (5s interval)
                handleBLEScanMessage(msg);
                break;

            case 'trigger_state':
                // Automation trigger state change
                handleTriggerStateMessage(msg);
                break;

            case 'system_health':
                // System health stats (60s interval)
                handleSystemHealthMessage(msg);
                break;

            default:
                // Log unhandled types for future debugging
                console.log('[Spaxel] Unknown message type:', msg.type, msg);
        }
    }

    // ─── Snapshot + Incremental Update Protocol ─────────────────────────────

    function handleSnapshot(msg) {
        state.awaitingSnapshot = false;

        // On reconnect: clear trails, restore scene, log duration
        if (SpaxelWebSocket.isConnected()) {
            SpaxelWebSocket.onReconnected();
        }

        console.log('[Spaxel] Received snapshot, rebuilding state');

        // Store snapshot for blob extrapolation on future disconnects
        SpaxelWebSocket.setLastSnapshot(msg);

        // Nodes
        if (msg.nodes) {
            state.nodes.clear();
            msg.nodes.forEach(function (node) {
                state.nodes.set(node.mac, {
                    mac: node.mac,
                    firmware: node.firmware_version,
                    chip: node.chip,
                    lastSeen: Date.now()
                });
            });
        }

        // Links
        if (msg.links) {
            state.links.clear();
            msg.links.forEach(function (link) {
                state.links.set(link.id, {
                    nodeMAC: link.node_mac,
                    peerMAC: link.peer_mac,
                    lastFrame: Date.now(),
                    lastCSI: null,
                    motionDetected: false,
                    deltaRMS: 0,
                    ampHistory: [],
                    lastAmpSample: 0
                });
            });
        }

        // Motion states
        if (msg.motion_states) {
            msg.motion_states.forEach(function (ms) { applyMotionState(ms); });
        }

        // Blobs (localisation)
        if (msg.blobs) {
            Viz3D.handleLocUpdate({ type: 'loc_update', blobs: msg.blobs });
        }

        // BLE devices
        if (msg.ble_devices) {
            handleBLEScanMessage({ type: 'ble_scan', devices: msg.ble_devices });
        }

        // Triggers
        if (msg.triggers) {
            msg.triggers.forEach(function (trigger) {
                if (window.Automations && window.Automations.updateTriggerState) {
                    window.Automations.updateTriggerState(trigger);
                }
            });
        }

        // Zones
        if (msg.zones) {
            if (window.Viz3D && window.Viz3D.handleZoneUpdate) {
                Viz3D.handleZoneUpdate(msg.zones);
            }
        }

        updateNodeList();
        updateLinkList();
        Viz3D.applyLinks(msg.links || []);
    }

    function handleIncrementalUpdate(msg) {
        // Drop incremental updates until the snapshot has been received.
        if (state.awaitingSnapshot) return;

        // Blobs (always present when localisation is running)
        if (msg.blobs) {
            Viz3D.handleLocUpdate({ type: 'loc_update', blobs: msg.blobs });
        }

        // Nodes (only present when node list changed)
        if (msg.nodes !== undefined) {
            if (msg.nodes.length > 0) {
                msg.nodes.forEach(function (node) {
                    state.nodes.set(node.mac, {
                        mac: node.mac,
                        firmware: node.firmware_version,
                        chip: node.chip,
                        lastSeen: Date.now()
                    });
                });
            }
            updateNodeList();
        }

        // Links (only present when link list changed)
        if (msg.links !== undefined) {
            if (msg.links.length > 0) {
                msg.links.forEach(function (link) {
                    state.links.set(link.id, {
                        nodeMAC: link.node_mac,
                        peerMAC: link.peer_mac,
                        lastFrame: Date.now(),
                        lastCSI: null,
                        motionDetected: false,
                        deltaRMS: 0,
                        ampHistory: [],
                        lastAmpSample: 0
                    });
                });
            }
            updateLinkList();
            Viz3D.applyLinks(msg.links);
        }

        // Motion states (only present when motion state changed)
        if (msg.motion_states) {
            var changed = false;
            msg.motion_states.forEach(function (ms) {
                if (applyMotionState(ms)) changed = true;
            });
            if (changed) updateLinkList();
        }

        // BLE devices
        if (msg.ble_devices) {
            handleBLEScanMessage({ type: 'ble_scan', devices: msg.ble_devices });
        }

        // Triggers
        if (msg.triggers) {
            msg.triggers.forEach(function (trigger) {
                if (window.Automations && window.Automations.updateTriggerState) {
                    window.Automations.updateTriggerState(trigger);
                }
            });
        }

        // Zones
        if (msg.zones) {
            if (window.Viz3D && window.Viz3D.handleZoneUpdate) {
                Viz3D.handleZoneUpdate(msg.zones);
            }
        }

        // Events buffered since last tick (presence transitions, zone entries/exits, portal crossings)
        if (msg.events && Array.isArray(msg.events)) {
            msg.events.forEach(function (evt) {
                handleEventMessage({ type: 'event', event: evt });
            });
        }
    }

    // applyMotionState updates a link's motion fields; returns true if it changed.
    function applyMotionState(ms) {
        const link = state.links.get(ms.link_id);
        if (!link) return false;
        const prev = link.motionDetected;
        link.motionDetected = ms.motion_detected;
        link.deltaRMS = ms.delta_rms || 0;
        // Phase 6: Breathing/dwell state
        link.breathingState = ms.breathing_state || 'CLEAR';
        link.breathingBPM = ms.breathing_bpm || 0;
        return prev !== ms.motion_detected;
    }

    function handleBinaryFrame(buffer) {
        const frame = parseCSIFrame(buffer);
        if (!frame) return;

        const linkID = frame.linkID;

        // Update link state
        let link = state.links.get(linkID);
        if (!link) {
            link = {
                nodeMAC: frame.nodeMAC,
                peerMAC: frame.peerMAC,
                lastFrame: Date.now(),
                lastCSI: null,
                motionDetected: false,
                deltaRMS: 0,
                ampHistory: [],
                lastAmpSample: 0
            };
            state.links.set(linkID, link);
            updateLinkList();
        } else {
            link.lastFrame = Date.now();
            // Ensure time-series fields exist on links pre-created from JSON events
            if (!link.ampHistory) {
                link.ampHistory = [];
                link.lastAmpSample = 0;
            }
        }

        // Store CSI for chart rendering
        link.lastCSI = frame;

        // Push amplitude sample to time series (rate-limited)
        const now = performance.now();
        if (now - link.lastAmpSample >= CONFIG.tsMinIntervalMs) {
            link.lastAmpSample = now;
            let sum = 0;
            for (let i = 0; i < frame.subcarriers.length; i++) {
                sum += frame.subcarriers[i].amplitude;
            }
            const meanAmp = frame.subcarriers.length > 0 ? sum / frame.subcarriers.length : 0;
            link.ampHistory.push({ t: now, amp: meanAmp, motion: link.motionDetected });
            if (link.ampHistory.length > CONFIG.tsMaxPoints) {
                link.ampHistory.shift();
            }
        }

        // Update charts if this is the selected link
        if (state.selectedLinkID === linkID) {
            if (now - state.lastChartUpdate >= CONFIG.chartUpdateMs) {
                drawAmplitudeChart(frame);
                drawTimeSeries(link.ampHistory);
                state.lastChartUpdate = now;
            }
        }
    }

    // ============================================
    // CSI Frame Parsing (matches Go binary format)
    // ============================================
    function parseCSIFrame(buffer) {
        const view = new DataView(buffer);
        const bytes = new Uint8Array(buffer);

        if (bytes.length < 24) {
            return null; // Header too short
        }

        const nodeMAC = formatMAC(bytes, 0);
        const peerMAC = formatMAC(bytes, 6);
        const timestampUS = view.getBigUint64(12, true); // little-endian
        const rssi = view.getInt8(20);
        const noiseFloor = view.getInt8(21);
        const channel = bytes[22];
        const nSub = bytes[23];

        if (channel === 0 || channel > 14) {
            return null; // Invalid channel
        }

        const expectedLen = 24 + nSub * 2;
        if (bytes.length !== expectedLen) {
            return null; // Payload length mismatch
        }

        // Extract I/Q pairs and compute amplitude
        const subcarriers = [];
        for (let i = 0; i < nSub; i++) {
            const offset = 24 + i * 2;
            const iVal = bytes[offset];
            const qVal = bytes[offset + 1];
            // Convert from unsigned to signed (JavaScript quirk)
            const I = iVal > 127 ? iVal - 256 : iVal;
            const Q = qVal > 127 ? qVal - 256 : qVal;
            const amplitude = Math.sqrt(I * I + Q * Q);
            subcarriers.push({ I, Q, amplitude });
        }

        return {
            nodeMAC,
            peerMAC,
            linkID: `${nodeMAC}:${peerMAC}`,
            timestampUS: Number(timestampUS),
            rssi,
            noiseFloor,
            channel,
            nSub,
            subcarriers
        };
    }

    function formatMAC(bytes, offset) {
        const parts = [];
        for (let i = 0; i < 6; i++) {
            parts.push(bytes[offset + i].toString(16).padStart(2, '0').toUpperCase());
        }
        return parts.join(':');
    }

    // ============================================
    // UI Updates
    // ============================================
    function updateNodeList() {
        const container = document.getElementById('node-list');
        document.getElementById('node-count').textContent = state.nodes.size;

        if (state.nodes.size === 0) {
            container.innerHTML = '<div class="empty-state">No nodes connected</div>';
            return;
        }

        let html = '';
        state.nodes.forEach((node, mac) => {
            const isVirtual = !!node.virtual;
            const isOnline = isVirtual || Date.now() - node.lastSeen < 30000;
            const statusClass = isVirtual ? 'virtual' : (isOnline ? 'online' : 'offline');
            const statusLabel = isVirtual ? 'Virtual' : (isOnline ? 'Online' : 'Offline');

            // Check for OTA rollback state
            let rollbackBadge = '';
            let otaBadge = '';
            if (window.SpaxelOTA) {
                const otaProgress = SpaxelOTA.getProgress();
                if (otaProgress && otaProgress[mac]) {
                    const p = otaProgress[mac];
                    if (p.state === 'rollback') {
                        rollbackBadge = '<span class="node-rollback-badge">ROLLBACK</span>';
                    } else if (p.state === 'downloading' || p.state === 'rebooting') {
                        otaBadge = '<span class="node-ota-badge">OTA ' + (p.progress_pct || 0) + '%</span>';
                    } else if (p.state === 'verified') {
                        otaBadge = '<span class="node-verified-badge">UPDATED</span>';
                    }
                }
            }

            // Firmware version display (shortened)
            const fwDisplay = node.firmware ? '<span class="node-fw">' + escapeHtml(node.firmware) + '</span>' : '';

            html += `
                <div class="node-item" data-mac="${mac}">
                    <span class="node-mac">${mac}</span>
                    ${fwDisplay}
                    ${rollbackBadge}
                    ${otaBadge}
                    <span class="node-status ${statusClass}">
                        ${statusLabel}
                    </span>
                </div>
            `;
        });
        container.innerHTML = html;

        // Click-to-select for placement
        container.querySelectorAll('.node-item').forEach(function (el) {
            el.addEventListener('click', function () {
                if (window.Placement) Placement.selectNode(el.dataset.mac);
            });
        });
    }

    function escapeHtml(s) {
        if (!s) return '';
        return String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
    }

    function updatePresenceIndicator() {
        let anyMotion = false;
        state.links.forEach(link => {
            if (link.motionDetected) anyMotion = true;
        });
        const el = document.getElementById('presence-indicator');
        if (anyMotion) {
            el.className = 'motion';
            el.textContent = 'MOTION';
        } else {
            el.className = 'clear';
            el.textContent = 'CLEAR';
        }
    }

    // ============================================
    // Presence Panel
    // ============================================
    function handlePresenceUpdate(msg) {
        if (!msg.links) return;

        const now = performance.now();
        let anyMotion = false;
        let anyStationary = false;

        for (const [linkID, info] of Object.entries(msg.links)) {
            // Update link state if link exists
            const link = state.links.get(linkID);
            const prevBreathingState = link ? link.breathingState : 'CLEAR';
            if (link) {
                link.motionDetected = info.is_motion || info.motion_detected || false;
                link.deltaRMS = info.delta_rms || 0;
                // Phase 6: Breathing/dwell state
                link.breathingState = info.breathing_state || 'CLEAR';
                link.breathingBPM = info.breathing_bpm || 0;
            }

            if (info.is_motion || info.motion_detected) anyMotion = true;
            if (info.breathing_state === 'STATIONARY_DETECTED') anyStationary = true;

            // Log timeline event on transition to STATIONARY_DETECTED
            const newBreathingState = info.breathing_state || 'CLEAR';
            if (prevBreathingState !== 'STATIONARY_DETECTED' && newBreathingState === 'STATIONARY_DETECTED') {
                const bpm = info.breathing_bpm || 0;
                const shortID = abbreviateLinkID(linkID);
                const timeStr = new Date().toLocaleTimeString();
                logTimelineEvent('stationary_detected', linkID, 'Stationary person detected on ' + shortID + ' at ' + timeStr + ' - breathing at ' + bpm.toFixed(1) + ' BPM');
            }

            // Append to deltaRMS history
            let history = state.drHistory.get(linkID);
            if (!history) {
                history = [];
                state.drHistory.set(linkID, history);
            }
            history.push({ t: now, rms: info.delta_rms || 0 });

            // Trim to window
            const cutoff = now - CONFIG.drTsWindowMs;
            while (history.length > 0 && history[0].t < cutoff) {
                history.shift();
            }
            if (history.length > CONFIG.drTsMaxPoints) {
                history.splice(0, history.length - CONFIG.drTsMaxPoints);
            }
        }

        updatePresencePanel(msg.links, anyMotion, anyStationary);
        updateLinkList();
        drawDeltaRMSTimeSeries();
    }

    // Timeline event logging
    function logTimelineEvent(eventType, linkID, message) {
        // Log to console for debugging
        console.log('[Timeline]', eventType, linkID, message);

        // Show toast notification for stationary detection
        if (eventType === 'stationary_detected') {
            showToast(message, 'info');
        }

        // Could also append to a timeline panel in the UI if one exists
        const timelineEl = document.getElementById('timeline-events');
        if (timelineEl) {
            const entry = document.createElement('div');
            entry.className = 'timeline-entry timeline-' + eventType;
            entry.innerHTML = '<span class="timeline-time">' + new Date().toLocaleTimeString() + '</span> ' + message;
            timelineEl.insertBefore(entry, timelineEl.firstChild);

            // Keep only last 50 entries
            while (timelineEl.children.length > 50) {
                timelineEl.removeChild(timelineEl.lastChild);
            }
        }
    }

    function updatePresencePanel(links, anyMotion, anyStationary) {
        const container = document.getElementById('presence-list');
        const statusEl = document.getElementById('presence-status');

        // Update status indicator with priority: motion > stationary > clear
        if (anyMotion) {
            statusEl.className = 'motion';
            statusEl.textContent = 'MOTION';
        } else if (anyStationary) {
            statusEl.className = 'stationary';
            statusEl.textContent = 'STATIONARY';
        } else {
            statusEl.className = 'clear';
            statusEl.textContent = 'CLEAR';
        }

        const entries = Object.entries(links);
        if (entries.length === 0) {
            container.innerHTML = '<div class="empty-state">No links active</div>';
            return;
        }

        let html = '';
        for (const [linkID, info] of entries) {
            const isMotion = info.is_motion || info.motion_detected || false;
            const confidence = info.confidence || 0;
            const rms = info.delta_rms || 0;
            const breathingState = info.breathing_state || 'CLEAR';
            const breathingBPM = info.breathing_bpm || 0;
            const isStationary = breathingState === 'STATIONARY_DETECTED';
            const shortID = abbreviateLinkID(linkID);
            const selected = state.presenceSelectedLinkID === linkID ? 'selected' : '';

            // Determine dot class based on state priority
            let dotClass = 'clear';
            let dotTitle = 'No motion detected';
            if (isStationary) {
                dotClass = 'stationary';
                dotTitle = 'Stationary person detected - breathing at ' + breathingBPM.toFixed(1) + ' BPM';
            } else if (isMotion && confidence > 0.7) {
                dotClass = 'high-confidence';
                dotTitle = 'High confidence motion detected';
            } else if (isMotion) {
                dotClass = 'motion';
                dotTitle = 'Motion detected';
            } else if (breathingState === 'POSSIBLY_PRESENT') {
                dotClass = 'possibly';
                dotTitle = 'Possibly present (waiting for confirmation)';
            }

            // Breathing info for tooltip/status
            let breathingInfo = '';
            if (isStationary) {
                breathingInfo = '<span class="breathing-bpm">' + breathingBPM.toFixed(1) + ' BPM</span>';
            }

            html += `
                <div class="presence-row ${selected}" data-link-id="${linkID}" title="${dotTitle}">
                    <span class="presence-dot ${dotClass}"></span>
                    <span class="presence-link-id">${shortID}</span>
                    <span class="presence-rms">${rms.toFixed(4)}</span>
                    ${breathingInfo}
                </div>
            `;
        }
        container.innerHTML = html;

        container.querySelectorAll('.presence-row').forEach(el => {
            el.addEventListener('click', () => {
                state.presenceSelectedLinkID = el.dataset.linkId;
                updatePresencePanel(links, anyMotion, anyStationary);
            });
        });
    }

    function abbreviateLinkID(linkID) {
        const parts = linkID.split(':');
        if (parts.length >= 12) {
            // Full MAC:MAC format: AA:BB:CC:DD:EE:FF:AA:BB:CC:DD:EE:FF
            const nodeShort = parts.slice(3, 6).join(':');
            const peerShort = parts.slice(9, 12).join(':');
            return nodeShort + '\u2192' + peerShort;
        }
        // Fallback: last segment of each side
        const halves = linkID.split(':').reduce((acc, p, i, arr) => {
            if (i < 6) acc[0].push(p);
            else acc[1].push(p);
            return acc;
        }, [[], []]);
        return halves[0].slice(-2).join(':') + '\u2192' + halves[1].slice(-2).join(':');
    }

    function updateLinkList() {
        const container = document.getElementById('link-list');
        document.getElementById('link-count').textContent = state.links.size;

        if (state.links.size === 0) {
            container.innerHTML = '<div class="empty-state">No links active</div>';
            return;
        }

        let html = '';
        state.links.forEach((link, id) => {
            const selected = state.selectedLinkID === id ? 'selected' : '';
            const shortID = id.split(':').map(p => p.split(':').slice(-1)[0]).join('→');
            const motionClass = link.motionDetected ? 'motion' : 'clear';
            const motionLabel = link.motionDetected ? 'MOTION' : 'CLEAR';
            html += `
                <div class="link-item ${selected}" data-link-id="${id}">
                    <span>${shortID}</span>
                    <span class="presence-badge ${motionClass}">${motionLabel}</span>
                </div>
            `;
        });
        container.innerHTML = html;

        // Add click handlers
        container.querySelectorAll('.link-item').forEach(el => {
            el.addEventListener('click', () => selectLink(el.dataset.linkId));
        });

        updatePresenceIndicator();
    }

    function selectLink(linkID) {
        state.selectedLinkID = linkID;
        updateLinkList();

        // Update chart title
        document.querySelector('#chart-title .link-id').textContent = linkID || 'no link selected';

        // Draw current data immediately if available
        const link = state.links.get(linkID);
        if (link) {
            if (link.lastCSI) drawAmplitudeChart(link.lastCSI);
            drawTimeSeries(link.ampHistory || []);
        }
    }

    // ============================================
    // Amplitude Chart (Canvas 2D) + Time Series
    // ============================================
    let chartCanvas, chartCtx;
    let tsCanvas, tsCtx;
    let drCanvas, drCtx;

    function initChart() {
        chartCanvas = document.getElementById('amplitude-chart');
        chartCtx = chartCanvas.getContext('2d');

        const rect = chartCanvas.getBoundingClientRect();
        chartCanvas.width = rect.width * window.devicePixelRatio;
        chartCanvas.height = rect.height * window.devicePixelRatio;
        chartCtx.scale(window.devicePixelRatio, window.devicePixelRatio);

        drawEmptyChart();

        tsCanvas = document.getElementById('timeseries-chart');
        tsCtx = tsCanvas.getContext('2d');

        const tsRect = tsCanvas.getBoundingClientRect();
        tsCanvas.width = tsRect.width * window.devicePixelRatio;
        tsCanvas.height = tsRect.height * window.devicePixelRatio;
        tsCtx.scale(window.devicePixelRatio, window.devicePixelRatio);

        drawTimeSeries([]);

        drCanvas = document.getElementById('deltarms-chart');
        drCtx = drCanvas.getContext('2d');

        const drRect = drCanvas.getBoundingClientRect();
        drCanvas.width = drRect.width * window.devicePixelRatio;
        drCanvas.height = drRect.height * window.devicePixelRatio;
        drCtx.scale(window.devicePixelRatio, window.devicePixelRatio);

        drawDeltaRMSTimeSeries();
    }

    function drawEmptyChart() {
        const width = chartCanvas.width / window.devicePixelRatio;
        const height = chartCanvas.height / window.devicePixelRatio;

        chartCtx.fillStyle = '#1a1a2e';
        chartCtx.fillRect(0, 0, width, height);

        chartCtx.fillStyle = '#444';
        chartCtx.font = '12px sans-serif';
        chartCtx.textAlign = 'center';
        chartCtx.fillText('No data', width / 2, height / 2);
    }

    function drawAmplitudeChart(frame) {
        const width = chartCanvas.width / window.devicePixelRatio;
        const height = chartCanvas.height / window.devicePixelRatio;

        // Clear
        chartCtx.fillStyle = '#1a1a2e';
        chartCtx.fillRect(0, 0, width, height);

        const subcarriers = frame.subcarriers;
        const nSub = subcarriers.length;
        if (nSub === 0) return;

        const barWidth = width / nSub;
        const padding = 1;

        // Find max amplitude for scaling
        let maxAmp = 0;
        subcarriers.forEach(s => {
            if (s.amplitude > maxAmp) maxAmp = s.amplitude;
        });
        if (maxAmp === 0) maxAmp = 1;

        // Draw bars
        for (let i = 0; i < nSub; i++) {
            const amp = subcarriers[i].amplitude;
            const barHeight = (amp / maxAmp) * (height - 10);
            const x = i * barWidth + padding;
            const y = height - barHeight;

            // Gradient color based on amplitude
            const intensity = amp / maxAmp;
            const r = Math.floor(79 + intensity * (255 - 79));
            const g = Math.floor(195 - intensity * 100);
            const b = Math.floor(247 - intensity * 150);
            chartCtx.fillStyle = `rgb(${r}, ${g}, ${b})`;

            chartCtx.fillRect(x, y, barWidth - padding * 2, barHeight);
        }

        // Draw channel/rssi info
        chartCtx.fillStyle = '#666';
        chartCtx.font = '10px monospace';
        chartCtx.textAlign = 'left';
        chartCtx.fillText(`CH${frame.channel} RSSI:${frame.rssi}dBm`, 4, height - 4);
    }

    function drawTimeSeries(history) {
        if (!tsCanvas) return;
        const width = tsCanvas.width / window.devicePixelRatio;
        const height = tsCanvas.height / window.devicePixelRatio;

        tsCtx.fillStyle = '#1a1a2e';
        tsCtx.fillRect(0, 0, width, height);

        if (history.length < 2) {
            tsCtx.fillStyle = '#444';
            tsCtx.font = '10px sans-serif';
            tsCtx.textAlign = 'center';
            tsCtx.fillText('Waiting for data…', width / 2, height / 2 + 4);
            return;
        }

        // Find max amplitude for y-scale (with a minimum floor)
        let maxAmp = 1;
        for (let i = 0; i < history.length; i++) {
            if (history[i].amp > maxAmp) maxAmp = history[i].amp;
        }

        const padTop = 4;
        const padBottom = 14; // room for time label
        const plotH = height - padTop - padBottom;
        const xStep = width / (CONFIG.tsMaxPoints - 1);

        // Draw zero line
        tsCtx.strokeStyle = 'rgba(255,255,255,0.05)';
        tsCtx.lineWidth = 1;
        tsCtx.beginPath();
        tsCtx.moveTo(0, padTop + plotH);
        tsCtx.lineTo(width, padTop + plotH);
        tsCtx.stroke();

        // Draw amplitude line, colored by motion state
        const startIdx = Math.max(0, CONFIG.tsMaxPoints - history.length);
        tsCtx.lineWidth = 1.5;

        for (let i = 0; i < history.length - 1; i++) {
            const x0 = (startIdx + i) * xStep;
            const x1 = (startIdx + i + 1) * xStep;
            const y0 = padTop + plotH - (history[i].amp / maxAmp) * plotH;
            const y1 = padTop + plotH - (history[i + 1].amp / maxAmp) * plotH;

            tsCtx.strokeStyle = history[i].motion ? 'rgba(239,83,80,0.8)' : 'rgba(102,187,106,0.7)';
            tsCtx.beginPath();
            tsCtx.moveTo(x0, y0);
            tsCtx.lineTo(x1, y1);
            tsCtx.stroke();
        }

        // Time label: oldest → newest
        const oldest = history[0].t;
        const newest = history[history.length - 1].t;
        const spanS = ((newest - oldest) / 1000).toFixed(0);
        tsCtx.fillStyle = '#555';
        tsCtx.font = '9px monospace';
        tsCtx.textAlign = 'left';
        tsCtx.fillText(`-${spanS}s`, 2, height - 2);
        tsCtx.textAlign = 'right';
        tsCtx.fillText('now', width - 2, height - 2);
    }

    function drawDeltaRMSTimeSeries() {
        if (!drCanvas) return;
        const width = drCanvas.width / window.devicePixelRatio;
        const height = drCanvas.height / window.devicePixelRatio;

        drCtx.fillStyle = '#1a1a2e';
        drCtx.fillRect(0, 0, width, height);

        const linkID = state.presenceSelectedLinkID;
        const history = linkID ? state.drHistory.get(linkID) : null;

        if (!history || history.length < 2) {
            drCtx.fillStyle = '#444';
            drCtx.font = '10px sans-serif';
            drCtx.textAlign = 'center';
            drCtx.fillText('Select a link', width / 2, height / 2 + 4);
            return;
        }

        const padTop = 4;
        const padBottom = 14;
        const plotH = height - padTop - padBottom;

        // Y-axis range: 0 to 0.1 (typical deltaRMS range)
        const yMax = 0.1;

        // Draw threshold line at 0.02
        const threshY = padTop + plotH - (CONFIG.drThreshold / yMax) * plotH;
        drCtx.strokeStyle = 'rgba(255, 167, 38, 0.5)';
        drCtx.lineWidth = 1;
        drCtx.setLineDash([4, 3]);
        drCtx.beginPath();
        drCtx.moveTo(0, threshY);
        drCtx.lineTo(width, threshY);
        drCtx.stroke();
        drCtx.setLineDash([]);

        // Threshold label
        drCtx.fillStyle = 'rgba(255, 167, 38, 0.7)';
        drCtx.font = '9px monospace';
        drCtx.textAlign = 'left';
        drCtx.fillText('0.02', 2, threshY - 2);

        // Time range
        const tEnd = history[history.length - 1].t;
        const tStart = tEnd - CONFIG.drTsWindowMs;

        // Draw deltaRMS line
        drCtx.lineWidth = 1.5;
        drCtx.strokeStyle = 'rgba(79, 195, 247, 0.8)';
        drCtx.beginPath();
        let started = false;

        for (let i = 0; i < history.length; i++) {
            const x = ((history[i].t - tStart) / CONFIG.drTsWindowMs) * width;
            const y = padTop + plotH - (Math.min(history[i].rms, yMax) / yMax) * plotH;

            if (!started) {
                drCtx.moveTo(x, y);
                started = true;
            } else {
                drCtx.lineTo(x, y);
            }
        }
        drCtx.stroke();

        // Fill area under curve
        if (history.length >= 2) {
            const lastX = ((history[history.length - 1].t - tStart) / CONFIG.drTsWindowMs) * width;
            const firstX = ((history[0].t - tStart) / CONFIG.drTsWindowMs) * width;
            drCtx.lineTo(lastX, padTop + plotH);
            drCtx.lineTo(firstX, padTop + plotH);
            drCtx.closePath();
            drCtx.fillStyle = 'rgba(79, 195, 247, 0.08)';
            drCtx.fill();
        }

        // Time labels
        drCtx.fillStyle = '#555';
        drCtx.font = '9px monospace';
        drCtx.textAlign = 'left';
        drCtx.fillText('-10s', 2, height - 2);
        drCtx.textAlign = 'right';
        drCtx.fillText('now', width - 2, height - 2);

        // Y-axis labels
        drCtx.textAlign = 'right';
        drCtx.fillText('0.1', width - 2, padTop + 10);
    }

    // ============================================
    // Initialization
    // ============================================
    function init() {
        console.log('[Spaxel] Dashboard initializing...');

        initScene();
        initChart();
        connectWebSocket();
        startHealthPolling();
        startDiurnalPolling();
        animate();

        console.log('[Spaxel] Dashboard ready');
    }

    // Start when DOM is ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }

    // ============================================
    // Public API
    // ============================================

    // Message handlers registered by other modules (e.g., FleetPanel)
    const messageHandlers = [];

    function registerMessageHandler(handler) {
        if (typeof handler === 'function') {
            messageHandlers.push(handler);
        }
    }

    window.SpaxelApp = {
        getLinks: function () { return state.links; },
        getNodes: function () { return state.nodes; },
        refreshNodeList: updateNodeList,
        refreshLinkList: updateLinkList,
        showToast: showToast,
        registerMessageHandler: registerMessageHandler
    };

    // ============================================
    // Crowd Flow Visualization Controls
    // Global wrappers for HTML onchange handlers -> Viz3D module
    // ============================================
    window.toggleFlowLayer = function(visible) {
        Viz3D.setFlowLayerVisible(visible);
    };

    window.toggleDwellLayer = function(visible) {
        Viz3D.setDwellLayerVisible(visible);
    };

    window.toggleCorridorLayer = function(visible) {
        Viz3D.setCorridorLayerVisible(visible);
    };

    window.setFlowTimeFilter = function(value) {
        Viz3D.setFlowTimeFilter(value);
    };
})();
