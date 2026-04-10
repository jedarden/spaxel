/**
 * Spaxel Dashboard - Pre-Deployment Simulator
 *
 * Allows users to model their space, place virtual nodes, and run synthetic walkers
 * to estimate expected accuracy before purchasing hardware.
 */

(function() {
    'use strict';

    // ============================================
    // Configuration
    // ============================================
    const CONFIG = {
        // Simulation tick rate (Hz)
        tickRateHz: 10,
        // Default simulation duration (seconds)
        defaultDurationSec: 30,
        // Walker speed (m/s)
        walkerSpeed: 1.0,
        // Grid resolution for GDOP calculation (meters)
        gridResolutionM: 0.2,
        // Fresnel zone parameters
        fresnelSigma: 0.3,
        signalAmplitude: 0.05,
    };

    // ============================================
    // State
    // ============================================
    const state = {
        // Space definition
        space: {
            width: 10,
            depth: 10,
            height: 2.5,
            walls: [],
        },

        // Virtual nodes
        nodes: [],

        // Walkers
        walkers: [],

        // Simulation state
        simulationRunning: false,
        simulationPaused: false,
        simulationTime: 0,
        simulationResults: null,

        // UI state
        currentTool: 'select', // select, wall, node, walker
        editingWall: null,
        editingNode: null,
        editingWalker: null,

        // GDOP overlay
        showGDOP: false,
        gdopData: null,

        // Session ID
        sessionId: null,
    };

    // ============================================
    // DOM Elements
    // ============================================
    let elements = {};

    // ============================================
    // Three.js references
    // ============================================
    let _scene = null;
    let _camera = null;
    let _renderer = null;
    let _controls = null;
    let _wallMeshes = [];
    let _nodeMeshes = new Map();
    let _walkerMeshes = new Map();
    let _gdopMesh = null;

    // ============================================
    // Initialization
    // ============================================
    function init() {
        console.log('[Simulate] Initializing pre-deployment simulator');

        // Wait for Three.js scene to be ready
        if (window.Viz3D) {
            initAfterViz3D();
        } else {
            document.addEventListener('viz3d-ready', initAfterViz3D);
        }
    }

    function initAfterViz3D() {
        // Get Three.js references from Viz3D
        const container = document.getElementById('scene-container');
        if (!container) return;

        // Create simulator UI
        createSimulatorUI();

        // Listen for router mode changes
        if (window.SpaxelRouter) {
            window.SpaxelRouter.onModeChange(onModeChange);
        }

        console.log('[Simulate] Ready');
    }

    // ============================================
    // Simulator UI
    // ============================================
    function createSimulatorUI() {
        // Create simulator panel (hidden by default)
        const panel = document.createElement('div');
        panel.id = 'simulator-panel';
        panel.className = 'simulator-panel';
        panel.style.display = 'none';

        panel.innerHTML = `
            <div class="simulator-header">
                <h2>Pre-Deployment Simulator</h2>
                <button id="sim-close-btn" class="sim-close-btn" title="Exit simulator">
                    <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <line x1="18" y1="6" x2="6" y2="18"/>
                        <line x1="6" y1="6" x2="18" y2="18"/>
                    </svg>
                </button>
            </div>

            <div class="simulator-content">
                <!-- Space Editor -->
                <div class="sim-section">
                    <h3>Space Configuration</h3>
                    <div class="sim-space-controls">
                        <label>
                            Width (m): <input type="number" id="sim-space-width" value="10" min="2" max="50" step="0.5">
                        </label>
                        <label>
                            Depth (m): <input type="number" id="sim-space-depth" value="10" min="2" max="50" step="0.5">
                        </label>
                        <label>
                            Height (m): <input type="number" id="sim-space-height" value="2.5" min="2" max="10" step="0.1">
                        </label>
                        <button id="sim-apply-space" class="sim-btn">Apply Space</button>
                    </div>

                    <div class="sim-tools">
                        <button class="sim-tool-btn" data-tool="select" title="Select">
                            <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                                <path d="M3 3l7.07 16.97 2.51-7.39 7.39-2.51L3 3z"/>
                            </svg>
                        </button>
                        <button class="sim-tool-btn" data-tool="wall" title="Add Wall">
                            <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                                <rect x="3" y="3" width="18" height="18" rx="2"/>
                            </svg>
                        </button>
                        <button class="sim-tool-btn" data-tool="node" title="Add Node">
                            <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                                <circle cx="12" cy="12" r="3"/>
                                <path d="M12 1v6m0 6v6"/>
                            </svg>
                        </button>
                        <button class="sim-tool-btn" data-tool="walker" title="Add Walker">
                            <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                                <circle cx="12" cy="8" r="3"/>
                                <path d="M12 11v10m-4-6h8"/>
                            </svg>
                        </button>
                    </div>
                </div>

                <!-- Node List -->
                <div class="sim-section">
                    <h3>Virtual Nodes</h3>
                    <div class="sim-node-list">
                        <button id="sim-add-node" class="sim-btn">Add Node</button>
                        <button id="sim-clear-nodes" class="sim-btn sim-btn-danger">Clear All</button>
                    </div>
                    <div id="sim-nodes-container" class="sim-items-list"></div>
                </div>

                <!-- Walker List -->
                <div class="sim-section">
                    <h3>Synthetic Walkers</h3>
                    <div class="sim-walker-controls">
                        <select id="sim-walker-type">
                            <option value="random">Random Walk</option>
                            <option value="path">Path Walk</option>
                            <option value="zone">Zone Walk</option>
                        </select>
                        <button id="sim-add-walker" class="sim-btn">Add Walker</button>
                        <button id="sim-clear-walkers" class="sim-btn sim-btn-danger">Clear All</button>
                    </div>
                    <div id="sim-walkers-container" class="sim-items-list"></div>
                </div>

                <!-- GDOP Overlay -->
                <div class="sim-section">
                    <h3>Coverage Analysis</h3>
                    <div class="sim-gdop-controls">
                        <label>
                            <input type="checkbox" id="sim-show-gdop"> Show GDOP Overlay
                        </label>
                        <button id="sim-update-gdop" class="sim-btn">Update GDOP</button>
                    </div>
                    <div id="sim-gdop-legend" class="sim-gdop-legend" style="display: none;">
                        <div class="gdop-legend-item">
                            <span class="gdop-legend-color" style="background-color: #22c65e;"></span>
                            <span class="gdop-legend-label">Excellent (GDOP &lt; 2)</span>
                        </div>
                        <div class="gdop-legend-item">
                            <span class="gdop-legend-color" style="background-color: #ffc107;"></span>
                            <span class="gdop-legend-label">Good (GDOP 2-4)</span>
                        </div>
                        <div class="gdop-legend-item">
                            <span class="gdop-legend-color" style="background-color: #ff9200;"></span>
                            <span class="gdop-legend-label">Fair (GDOP 4-8)</span>
                        </div>
                        <div class="gdop-legend-item">
                            <span class="gdop-legend-color" style="background-color: #dc3545;"></span>
                            <span class="gdop-legend-label">Poor (GDOP &gt; 8)</span>
                        </div>
                        <div class="gdop-legend-item">
                            <span class="gdop-legend-color" style="background-color: #505050;"></span>
                            <span class="gdop-legend-label">No Coverage</span>
                        </div>
                        <div class="gdop-stats" id="sim-gdop-stats">
                            <span class="gdop-stat-item">Coverage: <strong id="sim-gdop-coverage">--</strong></span>
                            <span class="gdop-stat-item">Mean GDOP: <strong id="sim-gdop-mean">--</strong></span>
                        </div>
                    </div>
                </div>

                <!-- Simulation Controls -->
                <div class="sim-section">
                    <h3>Simulation</h3>
                    <div class="sim-controls">
                        <button id="sim-start-btn" class="sim-btn sim-btn-primary">Start Simulation</button>
                        <button id="sim-pause-btn" class="sim-btn" disabled>Pause</button>
                        <button id="sim-stop-btn" class="sim-btn sim-btn-danger" disabled>Stop</button>
                    </div>
                    <div class="sim-progress">
                        <span id="sim-time">0:00 / 0:30</span>
                        <div class="sim-progress-bar">
                            <div id="sim-progress-fill" class="sim-progress-fill" style="width: 0%"></div>
                        </div>
                    </div>
                </div>

                <!-- Results -->
                <div class="sim-section" id="sim-results-section" style="display: none;">
                    <h3>Results</h3>
                    <div class="sim-results">
                        <div class="sim-result-item">
                            <span class="sim-result-label">Expected Accuracy:</span>
                            <span id="sim-result-accuracy" class="sim-result-value">--</span>
                        </div>
                        <div class="sim-result-item">
                            <span class="sim-result-label">Coverage Score:</span>
                            <span id="sim-result-coverage" class="sim-result-value">--</span>
                        </div>
                    </div>
                </div>

                <!-- Recommendations -->
                <div class="sim-section" id="sim-recommendations-section" style="display: none;">
                    <h3>Recommendations</h3>
                    <div id="sim-recommendations" class="sim-recommendations"></div>
                </div>

                <!-- Shopping List -->
                <div class="sim-section" id="sim-shopping-section" style="display: none;">
                    <h3>Shopping List</h3>
                    <div id="sim-shopping-list" class="sim-shopping-list"></div>
                </div>
            </div>
        `;

        document.body.appendChild(panel);

        // Store element references
        elements = {
            panel: panel,
            closeBtn: document.getElementById('sim-close-btn'),
            spaceWidth: document.getElementById('sim-space-width'),
            spaceDepth: document.getElementById('sim-space-depth'),
            spaceHeight: document.getElementById('sim-space-height'),
            applySpace: document.getElementById('sim-apply-space'),
            toolBtns: document.querySelectorAll('.sim-tool-btn'),
            addNode: document.getElementById('sim-add-node'),
            clearNodes: document.getElementById('sim-clear-nodes'),
            nodesContainer: document.getElementById('sim-nodes-container'),
            walkerType: document.getElementById('sim-walker-type'),
            addWalker: document.getElementById('sim-add-walker'),
            clearWalkers: document.getElementById('sim-clear-walkers'),
            walkersContainer: document.getElementById('sim-walkers-container'),
            showGDOP: document.getElementById('sim-show-gdop'),
            updateGDOP: document.getElementById('sim-update-gdop'),
            startBtn: document.getElementById('sim-start-btn'),
            pauseBtn: document.getElementById('sim-pause-btn'),
            stopBtn: document.getElementById('sim-stop-btn'),
            time: document.getElementById('sim-time'),
            progressFill: document.getElementById('sim-progress-fill'),
            resultsSection: document.getElementById('sim-results-section'),
            resultAccuracy: document.getElementById('sim-result-accuracy'),
            resultCoverage: document.getElementById('sim-result-coverage'),
            recommendationsSection: document.getElementById('sim-recommendations-section'),
            recommendations: document.getElementById('sim-recommendations'),
            shoppingSection: document.getElementById('sim-shopping-section'),
            shoppingList: document.getElementById('sim-shopping-list'),
        };

        // Attach event listeners
        elements.closeBtn.addEventListener('click', exitSimulator);
        elements.applySpace.addEventListener('click', applySpace);
        elements.toolBtns.forEach(btn => {
            btn.addEventListener('click', () => selectTool(btn.dataset.tool));
        });
        elements.addNode.addEventListener('click', addNode);
        elements.clearNodes.addEventListener('click', clearNodes);
        elements.addWalker.addEventListener('click', addWalker);
        elements.clearWalkers.addEventListener('click', clearWalkers);
        elements.showGDOP.addEventListener('change', toggleGDOP);
        elements.updateGDOP.addEventListener('click', updateGDOP);
        elements.startBtn.addEventListener('click', startSimulation);
        elements.pauseBtn.addEventListener('click', pauseSimulation);
        elements.stopBtn.addEventListener('click', stopSimulation);

        // Set default tool
        selectTool('select');
    }

    // ============================================
    // Router Integration
    // ============================================
    function onModeChange(newMode, oldMode) {
        if (newMode === 'simulate') {
            enterSimulator();
        } else if (oldMode === 'simulate') {
            exitSimulator();
        }
    }

    function enterSimulator() {
        console.log('[Simulate] Entering simulator mode');
        elements.panel.style.display = 'block';

        // Apply default space
        applySpace();

        // Create session
        createSession();
    }

    function exitSimulator() {
        console.log('[Simulate] Exiting simulator mode');
        elements.panel.style.display = 'none';

        // Stop simulation if running
        if (state.simulationRunning) {
            stopSimulation();
        }

        // Clear visualization
        clearSimulationMeshes();
    }

    // ============================================
    // Session Management
    // ============================================
    async function createSession() {
        try {
            const response = await fetch('/api/simulator/session', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    space: state.space,
                }),
            });

            if (!response.ok) throw new Error('Failed to create session');

            const data = await response.json();
            state.sessionId = data.session_id;
            console.log('[Simulate] Session created:', state.sessionId);
        } catch (err) {
            console.error('[Simulate] Failed to create session:', err);
        }
    }

    // ============================================
    // Space Management
    // ============================================
    function applySpace() {
        const width = parseFloat(elements.spaceWidth.value);
        const depth = parseFloat(elements.spaceDepth.value);
        const height = parseFloat(elements.spaceHeight.value);

        state.space = { width, depth, height, walls: [] };

        // Update 3D visualization
        if (window.Viz3D) {
            window.Viz3D.applyRoom({
                width: width,
                depth: depth,
                height: height,
                origin_x: 0,
                origin_z: 0,
            });
        }

        console.log('[Simulate] Space applied:', state.space);
    }

    // ============================================
    // Tool Selection
    // ============================================
    function selectTool(tool) {
        state.currentTool = tool;

        // Update button states
        elements.toolBtns.forEach(btn => {
            if (btn.dataset.tool === tool) {
                btn.classList.add('active');
            } else {
                btn.classList.remove('active');
            }
        });

        console.log('[Simulate] Tool selected:', tool);
    }

    // ============================================
    // Node Management
    // ============================================
    function addNode() {
        const id = 'node_' + Date.now();
        const node = {
            id: id,
            name: 'Node ' + (state.nodes.length + 1),
            type: 'virtual',
            position: {
                x: state.space.width / 2,
                y: 1.0,
                z: state.space.depth / 2,
            },
            role: 'tx_rx',
            enabled: true,
        };

        state.nodes.push(node);
        renderNodes();
        updateNodeVisualization(node);

        // Sync with backend
        syncNode(node);

        console.log('[Simulate] Node added:', node);
    }

    function removeNode(nodeId) {
        state.nodes = state.nodes.filter(n => n.id !== nodeId);
        renderNodes();
        removeNodeVisualization(nodeId);

        // Sync with backend
        deleteNode(nodeId);

        console.log('[Simulate] Node removed:', nodeId);
    }

    function updateNodePosition(nodeId, position) {
        const node = state.nodes.find(n => n.id === nodeId);
        if (node) {
            node.position = position;
            updateNodeVisualization(node);
            syncNode(node);
        }
    }

    function clearNodes() {
        state.nodes.forEach(n => removeNodeVisualization(n.id));
        state.nodes = [];
        renderNodes();
        console.log('[Simulate] All nodes cleared');
    }

    function renderNodes() {
        elements.nodesContainer.innerHTML = '';
        state.nodes.forEach(node => {
            const div = document.createElement('div');
            div.className = 'sim-item';
            div.innerHTML = `
                <span class="sim-item-name">${node.name}</span>
                <span class="sim-item-position">
                    (${node.position.x.toFixed(1)}, ${node.position.y.toFixed(1)}, ${node.position.z.toFixed(1)})
                </span>
                <button class="sim-item-delete" data-id="${node.id}">Delete</button>
            `;
            elements.nodesContainer.appendChild(div);
        });

        // Attach delete handlers
        elements.nodesContainer.querySelectorAll('.sim-item-delete').forEach(btn => {
            btn.addEventListener('click', () => removeNode(btn.dataset.id));
        });
    }

    async function syncNode(node) {
        try {
            await fetch('/api/simulator/nodes', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(node),
            });
        } catch (err) {
            console.error('[Simulate] Failed to sync node:', err);
        }
    }

    async function deleteNode(nodeId) {
        try {
            await fetch(`/api/simulator/nodes/${nodeId}`, {
                method: 'DELETE',
            });
        } catch (err) {
            console.error('[Simulate] Failed to delete node:', err);
        }
    }

    // ============================================
    // Walker Management
    // ============================================
    function addWalker() {
        const type = elements.walkerType.value;
        const id = 'walker_' + Date.now();

        // Map frontend type to backend WalkerType
        const backendType = type === 'path' ? 'path_follow' :
                            type === 'zone' ? 'node_to_node' : 'random_walk';

        const walker = {
            id: id,
            type: backendType,
            position: {
                x: state.space.width / 2,
                y: 1.0,
                z: state.space.depth / 2,
            },
            velocity: {
                x: (Math.random() - 0.5) * CONFIG.walkerSpeed,
                y: 0,
                z: (Math.random() - 0.5) * CONFIG.walkerSpeed,
            },
            speed: CONFIG.walkerSpeed,
            height: 1.7,
        };

        if (type === 'path') {
            // Create default path
            walker.path = [
                { x: 2, y: 1.0, z: 2 },
                { x: state.space.width - 2, y: 1.0, z: 2 },
                { x: state.space.width - 2, y: 1.0, z: state.space.depth - 2 },
                { x: 2, y: 1.0, z: state.space.depth - 2 },
            ];
            walker.path_index = 0;
        }

        state.walkers.push(walker);
        renderWalkers();
        updateWalkerVisualization(walker);

        // Sync with backend
        syncWalker(walker);

        console.log('[Simulate] Walker added:', walker);
    }

    function removeWalker(walkerId) {
        state.walkers = state.walkers.filter(w => w.id !== walkerId);
        renderWalkers();
        removeWalkerVisualization(walkerId);

        // Sync with backend
        deleteWalker(walkerId);

        console.log('[Simulate] Walker removed:', walkerId);
    }

    function clearWalkers() {
        state.walkers.forEach(w => removeWalkerVisualization(w.id));
        state.walkers = [];
        renderWalkers();
        console.log('[Simulate] All walkers cleared');
    }

    function renderWalkers() {
        elements.walkersContainer.innerHTML = '';
        state.walkers.forEach(walker => {
            const div = document.createElement('div');
            div.className = 'sim-item';
            div.innerHTML = `
                <span class="sim-item-name">${walker.type} walker</span>
                <span class="sim-item-position">
                    (${walker.position.x.toFixed(1)}, ${walker.position.z.toFixed(1)})
                </span>
                <button class="sim-item-delete" data-id="${walker.id}">Delete</button>
            `;
            elements.walkersContainer.appendChild(div);
        });

        // Attach delete handlers
        elements.walkersContainer.querySelectorAll('.sim-item-delete').forEach(btn => {
            btn.addEventListener('click', () => removeWalker(btn.dataset.id));
        });
    }

    async function syncWalker(walker) {
        try {
            await fetch('/api/simulator/walkers', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(walker),
            });
        } catch (err) {
            console.error('[Simulate] Failed to sync walker:', err);
        }
    }

    async function deleteWalker(walkerId) {
        try {
            await fetch(`/api/simulator/walkers/${walkerId}`, {
                method: 'DELETE',
            });
        } catch (err) {
            console.error('[Simulate] Failed to delete walker:', err);
        }
    }

    // ============================================
    // Panel Toggle
    // ============================================
    function togglePanel() {
        const panel = document.getElementById('simulator-panel');
        const btn = document.getElementById('simulator-btn');
        if (!panel || !btn) return;

        const isVisible = panel.style.display !== 'none';
        panel.style.display = isVisible ? 'none' : 'block';
        btn.classList.toggle('active', !isVisible);

        console.log('[Simulate] Panel', isVisible ? 'hidden' : 'shown');
    }

    // ============================================
    // GDOP Visualization
    // ============================================
    function toggleGDOP() {
        state.showGDOP = elements.showGDOP.checked;
        if (state.showGDOP) {
            updateGDOP();
        } else {
            clearGDOPMesh();
        }
    }

    async function updateGDOP() {
        if (state.nodes.length < 2) {
            console.warn('[Simulate] Need at least 2 nodes for GDOP');
            return;
        }

        try {
            const response = await fetch('/api/simulator/gdop/heatmap', {
                method: 'GET',
                headers: { 'Content-Type': 'application/json' },
            });

            if (!response.ok) throw new Error('Failed to compute GDOP');

            const data = await response.json();
            state.gdopData = data;
            renderGDOP(data);

            console.log('[Simulate] GDOP updated:', data);
        } catch (err) {
            console.error('[Simulate] Failed to update GDOP:', err);
        }
    }

    function renderGDOP(data) {
        clearGDOPMesh();

        if (!data.gdop_map || !data.grid_dimensions) {
            console.warn('[Simulate] Invalid GDOP data');
            return;
        }

        // Create texture from GDOP data
        const canvas = document.createElement('canvas');
        const size = 256;
        canvas.width = size;
        canvas.height = size;
        const ctx = canvas.getContext('2d');

        const imageData = ctx.createImageData(size, size);

        // Grid dimensions from backend: [width_cells, depth_cells]
        const gridWidth = data.grid_dimensions[0];
        const gridDepth = data.grid_dimensions[1];

        // gdop_map is a 1D flattened array: [x + y * width]
        // We render the 2D floor plane

        for (let y = 0; y < size; y++) {
            for (let x = 0; x < size; x++) {
                // Map pixel to grid cell
                const gridX = Math.floor((x / size) * gridWidth);
                const gridY = Math.floor((y / size) * gridDepth);

                // Calculate index in flattened array
                const idx = gridY * gridWidth + gridX;

                // Get GDOP value (9999 = infinity)
                const gdop = data.gdop_map[idx] !== undefined ? data.gdop_map[idx] : 9999;

                // Color based on GDOP quality (matching Go GDOPColorMap)
                // GDOP < 2: excellent (#22c65e = 34, 197, 94)
                // GDOP 2-4: good (#ffc107 = 255, 193, 7)
                // GDOP 4-8: fair (#ff9200 = 255, 146, 0)
                // GDOP > 8: poor (#dc3545 = 220, 53, 69)
                // Infinity: none (#505050 = 80, 80, 80)
                let color;
                if (gdop >= 9999) {
                    color = { r: 80, g: 80, b: 80 }; // None - gray
                } else if (gdop < 2.0) {
                    color = { r: 34, g: 197, b: 94 }; // Excellent - green
                } else if (gdop < 4.0) {
                    color = { r: 255, g: 193, b: 7 }; // Good - yellow
                } else if (gdop < 8.0) {
                    color = { r: 255, g: 146, b: 0 }; // Fair - orange
                } else {
                    color = { r: 220, g: 53, b: 69 }; // Poor - red
                }

                const i = (y * size + x) * 4;
                imageData.data[i] = color.r;
                imageData.data[i + 1] = color.g;
                imageData.data[i + 2] = color.b;
                imageData.data[i + 3] = 180; // Alpha
            }
        }

        ctx.putImageData(imageData, 0, 0);

        const texture = new THREE.CanvasTexture(canvas);
        texture.needsUpdate = true;
        const material = new THREE.MeshBasicMaterial({
            map: texture,
            transparent: true,
            opacity: 0.7,
            side: THREE.DoubleSide,
            depthWrite: false,
        });

        const geometry = new THREE.PlaneGeometry(state.space.width, state.space.depth);
        _gdopMesh = new THREE.Mesh(geometry, material);
        _gdopMesh.rotation.x = -Math.PI / 2;
        _gdopMesh.position.set(state.space.width / 2, 0.01, state.space.depth / 2);

        // Get scene from Viz3D
        if (window.Viz3D) {
            const scene = window.Viz3D.getScene?.();
            if (scene) scene.add(_gdopMesh);
        }

        // Update GDOP stats in legend
        const legendEl = document.getElementById('sim-gdop-legend');
        if (legendEl) {
            legendEl.style.display = 'block';
        }

        const coverageEl = document.getElementById('sim-gdop-coverage');
        const meanEl = document.getElementById('sim-gdop-mean');

        if (data.coverage_score !== undefined) {
            const coveragePercent = (data.coverage_score * 100).toFixed(1);
            if (coverageEl) {
                coverageEl.textContent = coveragePercent + '%';
            }
            console.log('[Simulate] Coverage score:', coveragePercent + '%');
        }

        if (data.mean_gdop !== undefined) {
            const meanGDOP = data.mean_gdop < 9999 ? data.mean_gdop.toFixed(2) : '∞';
            if (meanEl) {
                meanEl.textContent = meanGDOP;
            }
            console.log('[Simulate] Mean GDOP:', meanGDOP);
        }

        // Update quality counts if available
        if (data.quality_counts) {
            console.log('[Simulate] Quality distribution:', data.quality_counts);

            // Update legend items with counts
            const qualityLabels = {
                'excellent': 'Excellent (GDOP < 2)',
                'good': 'Good (GDOP 2-4)',
                'fair': 'Fair (GDOP 4-8)',
                'poor': 'Poor (GDOP > 8)',
                'none': 'No Coverage'
            };

            const legendItems = document.querySelectorAll('.gdop-legend-item');
            legendItems.forEach(item => {
                const label = item.querySelector('.gdop-legend-label');
                if (label) {
                    const labelText = label.textContent.split(':')[0];
                    for (const [quality, fullName] of Object.entries(qualityLabels)) {
                        if (fullName.includes(labelText) || labelText.includes(quality.charAt(0).toUpperCase() + quality.slice(1))) {
                            const count = data.quality_counts[quality] || 0;
                            label.textContent = `${fullName}: ${count} cells`;
                            break;
                        }
                    }
                }
            });
        }
    }

    function clearGDOPMesh() {
        if (_gdopMesh) {
            const scene = window.Viz3D?.getScene?.();
            if (scene) scene.remove(_gdopMesh);
            _gdopMesh.geometry.dispose();
            _gdopMesh.material.dispose();
            _gdopMesh = null;
        }

        // Hide the legend when GDOP is cleared
        const legendEl = document.getElementById('sim-gdop-legend');
        if (legendEl) {
            legendEl.style.display = 'none';
        }
    }

    // ============================================
    // Simulation Control
    // ============================================
    async function startSimulation() {
        if (state.nodes.length < 2) {
            alert('Please add at least 2 nodes before starting simulation');
            return;
        }

        if (state.walkers.length === 0) {
            alert('Please add at least 1 walker before starting simulation');
            return;
        }

        try {
            const response = await fetch('/api/simulator/simulate', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    duration_sec: CONFIG.defaultDurationSec,
                    tick_rate_hz: CONFIG.tickRateHz,
                }),
            });

            if (!response.ok) throw new Error('Failed to start simulation');

            const data = await response.json();
            state.simulationRunning = true;
            state.simulationPaused = false;
            state.simulationTime = 0;

            // Update UI
            elements.startBtn.disabled = true;
            elements.pauseBtn.disabled = false;
            elements.stopBtn.disabled = false;
            elements.resultsSection.style.display = 'none';
            elements.recommendationsSection.style.display = 'none';
            elements.shoppingSection.style.display = 'none';

            // Start progress update
            startProgressLoop();

            // Poll for results
            pollSimulationResults();

            console.log('[Simulate] Simulation started');
        } catch (err) {
            console.error('[Simulate] Failed to start simulation:', err);
            alert('Failed to start simulation: ' + err.message);
        }
    }

    function pauseSimulation() {
        if (!state.simulationRunning) return;

        state.simulationPaused = !state.simulationPaused;
        elements.pauseBtn.textContent = state.simulationPaused ? 'Resume' : 'Pause';

        console.log('[Simulate] Simulation', state.simulationPaused ? 'paused' : 'resumed');
    }

    async function stopSimulation() {
        state.simulationRunning = false;
        state.simulationPaused = false;
        state.simulationTime = 0;

        // Update UI
        elements.startBtn.disabled = false;
        elements.pauseBtn.disabled = true;
        elements.pauseBtn.textContent = 'Pause';
        elements.stopBtn.disabled = true;

        // Reset progress
        elements.time.textContent = '0:00 / 0:30';
        elements.progressFill.style.width = '0%';

        console.log('[Simulate] Simulation stopped');
    }

    function startProgressLoop() {
        const interval = setInterval(() => {
            if (!state.simulationRunning) {
                clearInterval(interval);
                return;
            }

            if (!state.simulationPaused) {
                state.simulationTime += 0.1;
                updateProgress();
            }
        }, 100);
    }

    function updateProgress() {
        const duration = CONFIG.defaultDurationSec;
        const progress = Math.min(state.simulationTime / duration, 1);

        const elapsed = Math.floor(state.simulationTime);
        const total = Math.floor(duration);
        elements.time.textContent = `${Math.floor(elapsed / 60)}:${(elapsed % 60).toString().padStart(2, '0')} / ${Math.floor(total / 60)}:${(total % 60).toString().padStart(2, '0')}`;
        elements.progressFill.style.width = (progress * 100) + '%';
    }

    async function pollSimulationResults() {
        const pollInterval = setInterval(async () => {
            if (!state.simulationRunning) {
                clearInterval(pollInterval);
                return;
            }

            try {
                const response = await fetch('/api/simulator/status');
                if (!response.ok) return;

                const data = await response.json();

                // Update walker positions
                if (data.walker_positions) {
                    data.walker_positions.forEach(pos => {
                        const walker = state.walkers.find(w => w.id === pos.id);
                        if (walker) {
                            walker.position = pos.position;
                            updateWalkerVisualization(walker);
                        }
                    });
                }

                // Check if simulation complete
                if (data.state === 'complete' || data.state === 'stopped') {
                    clearInterval(pollInterval);
                    await fetchSimulationResults();
                }
            } catch (err) {
                console.error('[Simulate] Failed to poll results:', err);
            }
        }, 200);
    }

    async function fetchSimulationResults() {
        try {
            const response = await fetch('/api/simulator/results');
            if (!response.ok) throw new Error('Failed to fetch results');

            const data = await response.json();
            state.simulationResults = data;

            displayResults(data);
            stopSimulation();

            console.log('[Simulate] Simulation results:', data);
        } catch (err) {
            console.error('[Simulate] Failed to fetch results:', err);
        }
    }

    function displayResults(data) {
        // Show results section
        elements.resultsSection.style.display = 'block';

        // Display accuracy
        const accuracy = data.expected_accuracy_m || 0;
        elements.resultAccuracy.textContent = accuracy < 0.5 ? '< 0.5m (Excellent)' :
                                               accuracy < 1.0 ? '< 1.0m (Good)' :
                                               accuracy < 1.5 ? '< 1.5m (Fair)' :
                                               '> 1.5m (Poor)';

        // Display coverage
        const coverage = data.coverage_score || 0;
        elements.resultCoverage.textContent = (coverage * 100).toFixed(0) + '%';

        // Display recommendations
        if (data.recommendations && data.recommendations.length > 0) {
            elements.recommendationsSection.style.display = 'block';
            elements.recommendations.innerHTML = data.recommendations.map(rec => `
                <div class="sim-recommendation">
                    <span class="sim-rec-priority ${rec.priority}">${rec.priority}</span>
                    <span class="sim-rec-text">${rec.message}</span>
                </div>
            `).join('');
        }

        // Display shopping list
        if (data.shopping_list) {
            elements.shoppingSection.style.display = 'block';
            elements.shoppingList.innerHTML = `
                <div class="sim-shopping-item">
                    <span>Minimum nodes:</span>
                    <strong>${data.shopping_list.min_nodes || state.nodes.length}</strong>
                </div>
                <div class="sim-shopping-item">
                    <span>Recommended nodes:</span>
                    <strong>${data.shopping_list.recommended_nodes || state.nodes.length}</strong>
                </div>
            `;
        }
    }

    // ============================================
    // 3D Visualization
    // ============================================
    function updateNodeVisualization(node) {
        // Get scene from Viz3D
        const scene = window.Viz3D?.getScene?.();
        if (!scene) return;

        // Remove existing mesh
        removeNodeVisualization(node.id);

        // Create node mesh
        const geometry = new THREE.SphereGeometry(0.15, 16, 16);
        const material = new THREE.MeshLambertMaterial({
            color: node.role === 'tx' ? 0xff6b6b : node.role === 'rx' ? 0x4ecdc4 : 0x45b7d1,
            emissive: node.role === 'tx' ? 0xff6b6b : node.role === 'rx' ? 0x4ecdc4 : 0x45b7d1,
            emissiveIntensity: 0.3,
        });

        const mesh = new THREE.Mesh(geometry, material);
        mesh.position.set(node.position.x, node.position.y, node.position.z);
        mesh.userData.nodeId = node.id;

        scene.add(mesh);
        _nodeMeshes.set(node.id, mesh);
    }

    function removeNodeVisualization(nodeId) {
        const mesh = _nodeMeshes.get(nodeId);
        if (mesh) {
            const scene = window.Viz3D?.getScene?.();
            if (scene) scene.remove(mesh);
            mesh.geometry.dispose();
            mesh.material.dispose();
            _nodeMeshes.delete(nodeId);
        }
    }

    function updateWalkerVisualization(walker) {
        // Get scene from Viz3D
        const scene = window.Viz3D?.getScene?.();
        if (!scene) return;

        // Remove existing mesh
        removeWalkerVisualization(walker.id);

        // Create walker mesh (capsule for person)
        const geometry = new THREE.CapsuleGeometry(0.1, 0.5, 4, 8);
        const material = new THREE.MeshLambertMaterial({
            color: 0xffa726,
            transparent: true,
            opacity: 0.8,
        });

        const mesh = new THREE.Mesh(geometry, material);
        mesh.position.set(walker.position.x, walker.position.y, walker.position.z);
        mesh.userData.walkerId = walker.id;

        scene.add(mesh);
        _walkerMeshes.set(walker.id, mesh);
    }

    function removeWalkerVisualization(walkerId) {
        const mesh = _walkerMeshes.get(walkerId);
        if (mesh) {
            const scene = window.Viz3D?.getScene?.();
            if (scene) scene.remove(mesh);
            mesh.geometry.dispose();
            mesh.material.dispose();
            _walkerMeshes.delete(walkerId);
        }
    }

    function clearSimulationMeshes() {
        _nodeMeshes.forEach((mesh, id) => removeNodeVisualization(id));
        _walkerMeshes.forEach((mesh, id) => removeWalkerVisualization(id));
        clearGDOPMesh();
    }

    // ============================================
    // Public API
    // ============================================
    window.SpaxelSimulator = {
        init: init,
        getState: () => state,
    };

    // Auto-initialize
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }

    console.log('[Simulate] Simulator module loaded');
})();
