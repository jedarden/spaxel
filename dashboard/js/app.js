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
        wsReconnectDelay: 3000,
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
        drThreshold: 0.02    // DefaultDeltaRMSThreshold
    };

    // ============================================
    // State
    // ============================================
    const state = {
        ws: null,
        wsConnected: false,
        nodes: new Map(),        // MAC -> { mac, firmware, chip, lastSeen }
        links: new Map(),        // linkID -> { nodeMAC, peerMAC, lastFrame, lastCSI, motionDetected, deltaRMS, ampHistory, lastAmpSample }
        selectedLinkID: null,
        presenceSelectedLinkID: null,
        drHistory: new Map(),    // linkID -> [{ t: number, rms: number }]
        lastChartUpdate: 0,
        frameCount: 0,
        lastFpsTime: performance.now()
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
    // WebSocket Connection
    // ============================================
    function connectWebSocket() {
        const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsURL = `${wsProtocol}//${window.location.host}/ws/dashboard`;

        console.log('[Spaxel] Connecting to', wsURL);

        state.ws = new WebSocket(wsURL);
        state.ws.binaryType = 'arraybuffer';

        state.ws.onopen = function() {
            console.log('[Spaxel] WebSocket connected');
            state.wsConnected = true;
            updateConnectionStatus(true);
        };

        state.ws.onclose = function(event) {
            console.log('[Spaxel] WebSocket closed:', event.code, event.reason);
            state.wsConnected = false;
            updateConnectionStatus(false);
            scheduleReconnect();
        };

        state.ws.onerror = function(error) {
            console.error('[Spaxel] WebSocket error:', error);
        };

        state.ws.onmessage = function(event) {
            handleMessage(event.data);
        };
    }

    function scheduleReconnect() {
        console.log('[Spaxel] Reconnecting in', CONFIG.wsReconnectDelay, 'ms');
        setTimeout(connectWebSocket, CONFIG.wsReconnectDelay);
    }

    function updateConnectionStatus(connected) {
        const dot = document.getElementById('ws-status');
        const text = document.getElementById('ws-status-text');

        if (connected) {
            dot.classList.remove('disconnected');
            dot.classList.add('connected');
            text.textContent = 'Connected';
        } else {
            dot.classList.remove('connected');
            dot.classList.add('disconnected');
            text.textContent = 'Disconnected';
        }
    }

    // ============================================
    // Message Handling
    // ============================================
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

            default:
                // Ignore unknown types (forward-compatible)
        }
    }

    // applyMotionState updates a link's motion fields; returns true if it changed.
    function applyMotionState(ms) {
        const link = state.links.get(ms.link_id);
        if (!link) return false;
        const prev = link.motionDetected;
        link.motionDetected = ms.motion_detected;
        link.deltaRMS = ms.delta_rms || 0;
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

        for (const [linkID, info] of Object.entries(msg.links)) {
            // Update link state if link exists
            const link = state.links.get(linkID);
            if (link) {
                link.motionDetected = info.is_motion || info.motion_detected || false;
                link.deltaRMS = info.delta_rms || 0;
            }

            if (info.is_motion || info.motion_detected) anyMotion = true;

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

        updatePresencePanel(msg.links, anyMotion);
        updateLinkList();
        drawDeltaRMSTimeSeries();
    }

    function updatePresencePanel(links, anyMotion) {
        const container = document.getElementById('presence-list');
        const statusEl = document.getElementById('presence-status');

        if (anyMotion) {
            statusEl.className = 'motion';
            statusEl.textContent = 'MOTION';
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
            const shortID = abbreviateLinkID(linkID);
            const selected = state.presenceSelectedLinkID === linkID ? 'selected' : '';

            let dotClass = 'clear';
            if (isMotion && confidence > 0.7) {
                dotClass = 'high-confidence';
            } else if (isMotion) {
                dotClass = 'motion';
            }

            html += `
                <div class="presence-row ${selected}" data-link-id="${linkID}">
                    <span class="presence-dot ${dotClass}"></span>
                    <span class="presence-link-id">${shortID}</span>
                    <span class="presence-rms">${rms.toFixed(4)}</span>
                </div>
            `;
        }
        container.innerHTML = html;

        container.querySelectorAll('.presence-row').forEach(el => {
            el.addEventListener('click', () => {
                state.presenceSelectedLinkID = el.dataset.linkId;
                updatePresencePanel(links, anyMotion);
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
    window.SpaxelApp = {
        getLinks: function () { return state.links; },
        getNodes: function () { return state.nodes; },
        refreshNodeList: updateNodeList,
        refreshLinkList: updateLinkList
    };
})();
