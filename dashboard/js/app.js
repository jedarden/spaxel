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
        chartUpdateMs: 100   // update chart at 10 Hz max
    };

    // ============================================
    // State
    // ============================================
    const state = {
        ws: null,
        wsConnected: false,
        nodes: new Map(),        // MAC -> { mac, firmware, chip, lastSeen }
        links: new Map(),        // linkID -> { nodeMAC, peerMAC, lastFrame, lastCSI }
        selectedLinkID: null,
        lastChartUpdate: 0,
        frameCount: 0,
        lastFpsTime: performance.now()
    };

    // ============================================
    // Three.js Scene Setup
    // ============================================
    let scene, camera, renderer, controls, gridHelper, axesHelper;

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
                        state.links.set(link.id, {
                            nodeMAC: link.node_mac,
                            peerMAC: link.peer_mac,
                            lastFrame: Date.now(),
                            lastCSI: null
                        });
                    });
                }
                updateNodeList();
                updateLinkList();
                break;

            case 'node_connected':
                state.nodes.set(msg.mac, {
                    mac: msg.mac,
                    firmware: msg.firmware_version,
                    chip: msg.chip,
                    lastSeen: Date.now()
                });
                updateNodeList();
                break;

            case 'node_disconnected':
                state.nodes.delete(msg.mac);
                updateNodeList();
                break;

            case 'link_active':
                if (!state.links.has(msg.id)) {
                    state.links.set(msg.id, {
                        nodeMAC: msg.node_mac,
                        peerMAC: msg.peer_mac,
                        lastFrame: Date.now(),
                        lastCSI: null
                    });
                    updateLinkList();
                }
                break;

            default:
                // Ignore unknown types (forward-compatible)
        }
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
                lastCSI: null
            };
            state.links.set(linkID, link);
            updateLinkList();
        } else {
            link.lastFrame = Date.now();
        }

        // Store CSI for chart rendering
        link.lastCSI = frame;

        // Update chart if this is the selected link
        if (state.selectedLinkID === linkID) {
            const now = performance.now();
            if (now - state.lastChartUpdate >= CONFIG.chartUpdateMs) {
                drawAmplitudeChart(frame);
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
            const isOnline = Date.now() - node.lastSeen < 30000;
            html += `
                <div class="node-item" data-mac="${mac}">
                    <span class="node-mac">${mac}</span>
                    <span class="node-status ${isOnline ? 'online' : 'offline'}">
                        ${isOnline ? 'Online' : 'Offline'}
                    </span>
                </div>
            `;
        });
        container.innerHTML = html;
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
            html += `
                <div class="link-item ${selected}" data-link-id="${id}">
                    <span>${shortID}</span>
                    <span style="color:#666">${link.nSub || 64} sub</span>
                </div>
            `;
        });
        container.innerHTML = html;

        // Add click handlers
        container.querySelectorAll('.link-item').forEach(el => {
            el.addEventListener('click', () => selectLink(el.dataset.linkId));
        });
    }

    function selectLink(linkID) {
        state.selectedLinkID = linkID;
        updateLinkList();

        // Update chart title
        document.querySelector('#chart-title .link-id').textContent = linkID || 'no link selected';

        // Draw current data immediately if available
        const link = state.links.get(linkID);
        if (link && link.lastCSI) {
            drawAmplitudeChart(link.lastCSI);
        }
    }

    // ============================================
    // Amplitude Chart (Canvas 2D)
    // ============================================
    let chartCanvas, chartCtx;

    function initChart() {
        chartCanvas = document.getElementById('amplitude-chart');
        chartCtx = chartCanvas.getContext('2d');

        // Set canvas resolution
        const rect = chartCanvas.getBoundingClientRect();
        chartCanvas.width = rect.width * window.devicePixelRatio;
        chartCanvas.height = rect.height * window.devicePixelRatio;
        chartCtx.scale(window.devicePixelRatio, window.devicePixelRatio);

        // Draw empty state
        drawEmptyChart();
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
})();
