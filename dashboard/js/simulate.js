/**
 * Spaxel Dashboard - Pre-Deployment Simulator
 *
 * Allows users to model their space, place virtual nodes, and run synthetic walkers
 * to estimate expected accuracy before purchasing hardware.
 *
 * Features:
 * - Space configuration (width, depth, height)
 * - Virtual node placement with 3D editor reuse (TransformControls)
 * - Ghost wireframe nodes for virtual nodes
 * - Dashed links between virtual nodes
 * - GDOP coverage overlay
 * - Simulation with synthetic walkers
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
        editingNode: null,

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
    let _transformControls = null;
    let _nodeMeshes = new Map();
    let _walkerMeshes = new Map();
    let _gdopMesh = null;
    let _roomBoundsMesh = null;
    let _linkLineMeshes = [];

    // Node MAC address generator for virtual nodes
    let _virtualNodeCounter = 0;

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
        if (!container) {
            console.warn('[Simulate] No scene-container found');
            return;
        }

        // Initialize UI elements from existing HTML
        initSimulatorUI();

        // Initialize TransformControls for 3D editing
        initTransformControls();

        // Apply default space
        applySpace();

        console.log('[Simulate] Ready');
    }

    // ============================================
    // Simulator UI
    // ============================================
    function initSimulatorUI() {
        // Store element references from existing HTML structure in simulator.html
        elements = {
            // Space configuration
            spaceWidth: document.getElementById('space-width'),
            spaceDepth: document.getElementById('space-depth'),
            spaceHeight: document.getElementById('space-height'),
            applySpace: document.getElementById('btn-apply-space'),

            // Node controls
            addNode: document.getElementById('btn-add-node'),
            nodeList: document.getElementById('node-list'),
            nodeSuggestions: document.getElementById('node-suggestions'),

            // Simulation controls
            simulateBtn: document.getElementById('btn-simulate'),
            stopBtn: document.getElementById('btn-stop'),
            resetBtn: document.getElementById('btn-reset'),
            duration: document.getElementById('sim-duration'),
            walkers: document.getElementById('sim-walkers'),
            showPaths: document.getElementById('sim-show-paths'),

            // Results display
            metricCoverage: document.getElementById('metric-coverage'),
            metricGdop: document.getElementById('metric-gdop'),
            metricBlobs: document.getElementById('metric-blobs'),
            simStatus: document.getElementById('sim-status'),

            // GDOP canvas
            gdopCanvas: document.getElementById('gdop-canvas'),
        };

        // Attach event listeners
        if (elements.applySpace) {
            elements.applySpace.addEventListener('click', applySpace);
        }
        if (elements.addNode) {
            elements.addNode.addEventListener('click', addNode);
        }
        if (elements.simulateBtn) {
            elements.simulateBtn.addEventListener('click', startSimulation);
        }
        if (elements.stopBtn) {
            elements.stopBtn.addEventListener('click', stopSimulation);
        }
        if (elements.resetBtn) {
            elements.resetBtn.addEventListener('click', resetSimulation);
        }

        console.log('[Simulate] Simulator UI initialized');
    }

    // ============================================
    // TransformControls (3D Editor Reuse)
    // ============================================
    function initTransformControls() {
        if (!window.Viz3D) return;

        _scene = window.Viz3D.getScene?.();
        _camera = window.Viz3D.getCamera?.();
        _renderer = window.Viz3D.getRenderer?.();
        _controls = window.Viz3D.getControls?.();

        if (!_scene || !_camera || !_renderer) {
            console.warn('[Simulate] Viz3D not ready yet');
            return;
        }

        // Initialize TransformControls for dragging nodes
        if (typeof THREE !== 'undefined' && THREE.TransformControls) {
            _transformControls = new THREE.TransformControls(_camera, _renderer.domElement);
            _transformControls.setMode('translate');
            _transformControls.setSize(0.75);
            _scene.add(_transformControls.getHelper());

            // Disable orbit controls while dragging
            _transformControls.addEventListener('dragging-changed', function (event) {
                if (_controls) {
                    _controls.enabled = !event.value;
                }

                // Save position on drag end
                if (!event.value && state.editingNode) {
                    const mesh = _nodeMeshes.get(state.editingNode.id);
                    if (mesh) {
                        updateNodePosition(state.editingNode.id, {
                            x: mesh.position.x,
                            y: mesh.position.y,
                            z: mesh.position.z
                        });
                        // Update GDOP when node position changes
                        if (state.showGDOP) {
                            updateGDOP();
                        }
                    }
                    state.editingNode = null;
                }
            });

            // Constrain to room bounds during drag
            _transformControls.addEventListener('objectChange', function () {
                if (!state.editingNode || !state.space) return;

                const obj = _transformControls.object;
                const { width, depth, height } = state.space;

                // Clamp to room bounds
                obj.position.x = Math.max(0, Math.min(width, obj.position.x));
                obj.position.y = Math.max(0.1, Math.min(height, obj.position.y));
                obj.position.z = Math.max(0, Math.min(depth, obj.position.z));

                // Update link lines during drag
                rebuildLinkLines();
            });
        }

        // Add pointer event listener for node selection
        _renderer.domElement.addEventListener('pointerdown', onPointerDown);
        _renderer.domElement.addEventListener('pointerup', onPointerUp);

        console.log('[Simulate] TransformControls initialized');
    }

    // Pointer event handlers for click-to-select
    let _mouseDown = { x: 0, y: 0 };
    const CLICK_THRESHOLD = 5;

    function onPointerDown(event) {
        _mouseDown.x = event.clientX;
        _mouseDown.y = event.clientY;
    }

    function onPointerUp(event) {
        const dx = event.clientX - _mouseDown.x;
        const dy = event.clientY - _mouseDown.y;
        if (Math.sqrt(dx * dx + dy * dy) > CLICK_THRESHOLD) return;
        if (event.target !== _renderer?.domElement) return;

        const rect = _renderer.domElement.getBoundingClientRect();
        const mouse = new THREE.Vector2(
            ((event.clientX - rect.left) / rect.width) * 2 - 1,
            -((event.clientY - rect.top) / rect.height) * 2 + 1
        );

        const raycaster = new THREE.Raycaster();
        raycaster.setFromCamera(mouse, _camera);

        // Collect all node meshes
        const meshes = Array.from(_nodeMeshes.values());
        const intersects = raycaster.intersectObjects(meshes);

        if (intersects.length > 0) {
            // Find which node owns this mesh
            for (const [nodeId, mesh] of _nodeMeshes.entries()) {
                if (mesh === intersects[0].object || mesh === intersects[0].object.parent) {
                    selectNode(nodeId);
                    return;
                }
            }
        }

        deselectNode();
    }

    // ============================================
    // Node Selection (3D Editor)
    // ============================================
    function selectNode(nodeId) {
        if (state.editingNode?.id === nodeId) return;

        deselectNode();

        const node = state.nodes.find(n => n.id === nodeId);
        if (!node) return;

        state.editingNode = node;

        const mesh = _nodeMeshes.get(nodeId);
        if (mesh && _transformControls) {
            _transformControls.attach(mesh);
        }

        // Update UI
        renderNodes();

        console.log('[Simulate] Node selected:', nodeId);
    }

    function deselectNode() {
        if (_transformControls) {
            _transformControls.detach();
        }
        state.editingNode = null;

        // Update UI
        renderNodes();
    }

    // ============================================
    // Space Management
    // ============================================
    function applySpace() {
        const width = elements.spaceWidth ? parseFloat(elements.spaceWidth.value) : 10;
        const depth = elements.spaceDepth ? parseFloat(elements.spaceDepth.value) : 8;
        const height = elements.spaceHeight ? parseFloat(elements.spaceHeight.value) : 2.5;

        state.space = { width, depth, height, walls: [] };

        // Update 3D visualization with Viz3D
        if (window.Viz3D && window.Viz3D.applyRoom) {
            window.Viz3D.applyRoom({
                width: width,
                depth: depth,
                height: height,
                origin_x: 0,
                origin_z: 0,
            });
        }

        // Update room bounds wireframe visualization
        updateRoomBoundsVisualization();

        console.log('[Simulate] Space applied:', state.space);
    }

    // ============================================
    // Room Bounds Visualization
    // ============================================
    function updateRoomBoundsVisualization() {
        const scene = window.Viz3D?.getScene?.();
        if (!scene || !state.space) return;

        // Remove existing room bounds mesh
        if (_roomBoundsMesh) {
            scene.remove(_roomBoundsMesh);
            _roomBoundsMesh.geometry.dispose();
            _roomBoundsMesh.material.dispose();
            _roomBoundsMesh = null;
        }

        const { width, depth, height } = state.space;

        // Create wireframe box geometry
        const geometry = new THREE.BoxGeometry(width, height, depth);
        const edges = new THREE.EdgesGeometry(geometry);

        const material = new THREE.LineBasicMaterial({
            color: 0x4fc3f7,
            opacity: 0.4,
            transparent: true,
        });

        _roomBoundsMesh = new THREE.LineSegments(edges, material);
        _roomBoundsMesh.position.set(width / 2, height / 2, depth / 2);

        scene.add(_roomBoundsMesh);

        console.log('[Simulate] Room bounds visualization updated:', { width, depth, height });
    }

    // ============================================
    // Node Management
    // ============================================
    function addNode() {
        // Generate virtual MAC address
        _virtualNodeCounter++;
        const mac = 'AA:BB:CC:' +
            (_virtualNodeCounter & 0xFF).toString(16).padStart(2, '0').toUpperCase() + ':' +
            ((_virtualNodeCounter >> 8) & 0xFF).toString(16).padStart(2, '0').toUpperCase() + ':' +
            ((_virtualNodeCounter >> 16) & 0xFF).toString(16).padStart(2, '0').toUpperCase();

        const id = 'node_' + Date.now() + '_' + _virtualNodeCounter;
        const node = {
            id: id,
            mac: mac,
            name: 'Virtual Node ' + (state.nodes.length + 1),
            type: 'virtual',
            node_type: 'esp32',
            virtual: true,
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
        rebuildLinkLines();

        // Update status
        updateStatus(`Added ${node.name} at (${node.position.x.toFixed(1)}, ${node.position.y.toFixed(1)}, ${node.position.z.toFixed(1)})`);

        // Auto-show GDOP if this is the second node
        if (state.nodes.length >= 2 && !state.showGDOP) {
            toggleGDOP();
        }

        console.log('[Simulate] Node added:', node);
    }

    function removeNode(nodeId) {
        const node = state.nodes.find(n => n.id === nodeId);
        state.nodes = state.nodes.filter(n => n.id !== nodeId);
        renderNodes();
        removeNodeVisualization(nodeId);

        updateStatus(`Removed ${node ? node.name : 'node'}`);
        console.log('[Simulate] Node removed:', nodeId);
    }

    function updateNodePosition(nodeId, position) {
        const node = state.nodes.find(n => n.id === nodeId);
        if (node) {
            node.position = position;
            updateNodeVisualization(node);
        }
    }

    function renderNodes() {
        if (!elements.nodeList) return;

        elements.nodeList.innerHTML = '';
        state.nodes.forEach(node => {
            const div = document.createElement('div');
            div.className = 'sim-item' + (state.editingNode?.id === node.id ? ' selected' : '');
            div.style.cssText = 'display:flex;justify-content:space-between;align-items:center;padding:8px;background:var(--bg-hover);border-radius:4px;margin-bottom:4px;cursor:pointer;';
            div.innerHTML = `
                <span style="font-size:var(--text-sm);">${node.name}</span>
                <span style="font-size:var(--text-2xs);color:var(--text-muted);">
                    (${node.position.x.toFixed(1)}, ${node.position.y.toFixed(1)}, ${node.position.z.toFixed(1)})
                </span>
                <button class="sim-item-delete" data-id="${node.id}" style="background:none;border:none;color:var(--alert);cursor:pointer;padding:4px 8px;">&times;</button>
            `;
            elements.nodeList.appendChild(div);

            // Add click handler for selection
            div.addEventListener('click', (e) => {
                if (!e.target.classList.contains('sim-item-delete')) {
                    selectNode(node.id);
                }
            });
        });

        // Attach delete handlers
        elements.nodeList.querySelectorAll('.sim-item-delete').forEach(btn => {
            btn.addEventListener('click', () => removeNode(btn.dataset.id));
        });
    }

    function updateStatus(message) {
        if (elements.simStatus) {
            elements.simStatus.textContent = message;
        }
    }

    // ============================================
    // GDOP Visualization
    // ============================================
    function toggleGDOP() {
        state.showGDOP = !state.showGDOP;
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

        // Update metrics
        if (data.coverage_score !== undefined && elements.metricCoverage) {
            const coveragePercent = (data.coverage_score * 100).toFixed(1);
            elements.metricCoverage.textContent = coveragePercent + '%';
        }

        if (data.mean_gdop !== undefined && elements.metricGdop) {
            const meanGDOP = data.mean_gdop < 9999 ? data.mean_gdop.toFixed(2) : '∞';
            elements.metricGdop.textContent = meanGDOP;
        }

        console.log('[Simulate] GDOP rendered');
    }

    function clearGDOPMesh() {
        if (_gdopMesh) {
            const scene = window.Viz3D?.getScene?.();
            if (scene) scene.remove(_gdopMesh);
            _gdopMesh.geometry.dispose();
            _gdopMesh.material.dispose();
            _gdopMesh = null;
        }
    }

    // ============================================
    // Simulation Control
    // ============================================
    async function startSimulation() {
        if (state.nodes.length < 2) {
            updateStatus('Error: Add at least 2 nodes first');
            return;
        }

        updateStatus('Starting simulation...');

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
            if (elements.simulateBtn) elements.simulateBtn.disabled = true;
            if (elements.stopBtn) elements.stopBtn.disabled = false;

            // Start progress update
            startProgressLoop();

            // Poll for results
            pollSimulationResults();

            console.log('[Simulate] Simulation started');
        } catch (err) {
            console.error('[Simulate] Failed to start simulation:', err);
            updateStatus('Error: ' + err.message);
        }
    }

    function stopSimulation() {
        state.simulationRunning = false;
        state.simulationPaused = false;
        state.simulationTime = 0;

        // Update UI
        if (elements.simulateBtn) elements.simulateBtn.disabled = false;
        if (elements.stopBtn) elements.stopBtn.disabled = true;

        updateStatus('Simulation stopped');

        console.log('[Simulate] Simulation stopped');
    }

    function resetSimulation() {
        stopSimulation();
        state.nodes = [];
        state.walkers = [];
        renderNodes();
        clearSimulationMeshes();
        clearGDOPMesh();

        if (elements.metricCoverage) elements.metricCoverage.textContent = '--';
        if (elements.metricGdop) elements.metricGdop.textContent = '--';
        if (elements.metricBlobs) elements.metricBlobs.textContent = '--';

        updateStatus('Simulation reset');

        console.log('[Simulate] Simulation reset');
    }

    function startProgressLoop() {
        const interval = setInterval(() => {
            if (!state.simulationRunning) {
                clearInterval(interval);
                return;
            }

            if (!state.simulationPaused) {
                state.simulationTime += 0.1;
            }
        }, 100);
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
        if (elements.metricCoverage && data.coverage_score) {
            elements.metricCoverage.textContent = (data.coverage_score * 100).toFixed(0) + '%';
        }

        if (elements.metricGdop && data.mean_gdop) {
            elements.metricGdop.textContent = data.mean_gdop < 9999 ? data.mean_gdop.toFixed(2) : '∞';
        }

        if (elements.metricBlobs && data.blob_count !== undefined) {
            elements.metricBlobs.textContent = data.blob_count;
        }

        updateStatus('Simulation complete');
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

        let mesh;

        if (node.virtual) {
            // Ghost wireframe node for virtual nodes
            // Use wireframe octahedron (matching viz3d.js style)
            const geometry = new THREE.OctahedronGeometry(0.15, 0);
            const material = new THREE.MeshPhongMaterial({
                color: 0x80cbc4,  // Teal for virtual nodes
                wireframe: true,
                transparent: true,
                opacity: 0.6,
            });

            mesh = new THREE.Mesh(geometry, material);
            mesh.userData = {
                nodeId: node.id,
                mac: node.mac,
                virtual: true,
                node_type: node.node_type || 'esp32',
            };
        } else {
            // Solid sphere for real nodes
            const geometry = new THREE.SphereGeometry(0.15, 16, 16);
            const material = new THREE.MeshLambertMaterial({
                color: node.role === 'tx' ? 0xff6b6b : node.role === 'rx' ? 0x4ecdc4 : 0x45b7d1,
                emissive: node.role === 'tx' ? 0xff6b6b : node.role === 'rx' ? 0x4ecdc4 : 0x45b7d1,
                emissiveIntensity: 0.3,
            });

            mesh = new THREE.Mesh(geometry, material);
            mesh.userData = {
                nodeId: node.id,
                mac: node.mac || '',
                virtual: false,
            };
        }

        mesh.position.set(node.position.x, node.position.y, node.position.z);

        scene.add(mesh);
        _nodeMeshes.set(node.id, mesh);
    }

    // ============================================
    // Link Lines (with dashed lines for virtual nodes)
    // ============================================
    function rebuildLinkLines() {
        const scene = window.Viz3D?.getScene?.();
        if (!scene) return;

        // Clear existing link lines
        clearLinkLines();

        if (state.nodes.length < 2) return;

        // Create links between all node pairs
        for (let i = 0; i < state.nodes.length; i++) {
            for (let j = i + 1; j < state.nodes.length; j++) {
                const nodeA = state.nodes[i];
                const nodeB = state.nodes[j];
                const meshA = _nodeMeshes.get(nodeA.id);
                const meshB = _nodeMeshes.get(nodeB.id);

                if (!meshA || !meshB) continue;

                // Check if either node is virtual
                const aIsVirtual = nodeA.virtual || false;
                const bIsVirtual = nodeB.virtual || false;
                const isVirtualLink = aIsVirtual || bIsVirtual;

                // Create line geometry
                const points = [
                    meshA.position.clone(),
                    meshB.position.clone()
                ];
                const geometry = new THREE.BufferGeometry().setFromPoints(points);

                let material;
                if (isVirtualLink) {
                    // Dashed line for virtual links (teal color)
                    material = new THREE.LineDashedMaterial({
                        color: 0x80cbc4,
                        dashSize: 0.1,
                        gapSize: 0.05,
                        opacity: 0.5,
                        transparent: true,
                    });
                } else {
                    // Solid line for real links
                    material = new THREE.LineBasicMaterial({
                        color: 0x4fc3f7,
                        opacity: 0.3,
                        transparent: true,
                    });
                }

                const line = new THREE.Line(geometry, material);
                line.userData = {
                    nodeA: nodeA.id,
                    nodeB: nodeB.id,
                    virtual: isVirtualLink
                };

                if (isVirtualLink) {
                    line.computeLineDistances(); // Required for dashed lines
                }

                scene.add(line);
                _linkLineMeshes.push(line);
            }
        }

        console.log('[Simulate] Rebuilt', _linkLineMeshes.length, 'link lines');
    }

    function clearLinkLines() {
        const scene = window.Viz3D?.getScene?.();
        if (!scene) return;

        for (const line of _linkLineMeshes) {
            scene.remove(line);
            line.geometry.dispose();
            line.material.dispose();
        }
        _linkLineMeshes = [];
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

        // Rebuild link lines after removing a node
        rebuildLinkLines();
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
        clearLinkLines();
        clearGDOPMesh();
    }

    // ============================================
    // Public API
    // ============================================
    window.SpaxelSimulator = {
        init: init,
        getState: () => state,
        addNode: addNode,
        removeNode: removeNode,
        applySpace: applySpace,
        toggleGDOP: toggleGDOP,
        updateGDOP: updateGDOP,
    };

    // Auto-initialize when DOM is ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }

    console.log('[Simulate] Simulator module loaded');
})();
