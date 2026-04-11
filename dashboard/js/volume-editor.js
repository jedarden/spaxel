/**
 * Spaxel Volume Editor - 3D trigger volume builder
 *
 * Provides interactive 3D volume creation and editing for automation triggers.
 * Supports box and cylinder volumes with click-drag drawing and TransformControls.
 */

(function() {
    'use strict';

    // ── Module state ─────────────────────────────────────────────────────────────
    let _scene = null;
    let _camera = null;
    let _controls = null;
    let _renderer = null;
    let _volumes = new Map(); // volume_id -> { mesh, shape, trigger }
    let _editingVolume = null;
    let _transformControls = null;
    let _raycaster = new THREE.Raycaster();
    let _mouse = new THREE.Vector2();
    let _groundPlane = null;
    let _drawMode = null; // 'box' | 'cylinder' | null
    let _drawStart = null; // {x, z} for box start or cylinder center
    let _drawPreview = null; // Preview mesh during drawing
    let _heightSlider = null;
    let _onVolumeCreated = null;

    // Volume visualization materials
    const VOLUME_MATERIALS = {
        idle: new THREE.MeshBasicMaterial({
            color: 0x4fc3f7,
            transparent: true,
            opacity: 0.15,
            side: THREE.DoubleSide,
            depthWrite: false
        }),
        active: new THREE.MeshBasicMaterial({
            color: 0x4fc3f7,
            transparent: true,
            opacity: 0.25,
            side: THREE.DoubleSide,
            depthWrite: false
        }),
        edge: new THREE.LineBasicMaterial({
            color: 0x4fc3f7,
            transparent: true,
            opacity: 0.8
        }),
        triggered: new THREE.MeshBasicMaterial({
            color: 0xff9800, // Orange for triggered state
            transparent: true,
            opacity: 0.3,
            side: THREE.DoubleSide,
            depthWrite: false
        })
    };

    // ── Initialization ─────────────────────────────────────────────────────────────
    function init(scene, camera, controls, renderer) {
        _scene = scene;
        _camera = camera;
        _controls = controls;
        _renderer = renderer;

        // Create ground plane for raycasting
        _createGroundPlane();

        // Load TransformControls
        _loadTransformControls();

        // Setup event listeners
        _setupEventListeners();

        // Load existing volumes from state
        _loadExistingVolumes();

        // Subscribe to state changes
        SpaxelState.subscribe('triggers', _onTriggersChanged);

        console.log('[VolumeEditor] Initialized');
    }

    function _createGroundPlane() {
        const groundGeo = new THREE.PlaneGeometry(100, 100);
        const groundMat = new THREE.MeshBasicMaterial({ visible: false });
        _groundPlane = new THREE.Mesh(groundGeo, groundMat);
        _groundPlane.rotation.x = -Math.PI / 2;
        _groundPlane.position.y = 0;
        _scene.add(_groundPlane);
    }

    function _loadTransformControls() {
        // Dynamically load TransformControls from the same CDN as Three.js
        const script = document.createElement('script');
        script.src = 'https://cdn.jsdelivr.net/npm/three@0.128.0/examples/js/controls/TransformControls.js';
        script.onload = () => {
            if (typeof THREE.TransformControls !== 'undefined') {
                _transformControls = new THREE.TransformControls(_camera, _renderer.domElement);
                _transformControls.addEventListener('change', () => {
                    // Update volume shape based on transform
                    if (_editingVolume) {
                        _updateVolumeShapeFromTransform();
                    }
                });
                _transformControls.addEventListener('dragging-changed', (event) => {
                    _controls.enabled = !event.value;
                });
                _scene.add(_transformControls);
            }
        };
        document.head.appendChild(script);
    }

    function _setupEventListeners() {
        const canvas = _renderer.domElement;

        canvas.addEventListener('pointerdown', _onPointerDown);
        canvas.addEventListener('pointermove', _onPointerMove);
        canvas.addEventListener('pointerup', _onPointerUp);
        canvas.addEventListener('keydown', _onKeyDown);
    }

    function _loadExistingVolumes() {
        const triggers = SpaxelState.triggers;
        if (!triggers) return;

        for (const id in triggers) {
            const trigger = triggers[id];
            if (trigger.shape_json) {
                _createVolumeMesh(id, trigger.shape_json, trigger);
            }
        }
    }

    function _onTriggersChanged(triggers) {
        // Reload volumes when triggers state changes
        _clearAllVolumes();
        _loadExistingVolumes();
    }

    // ── Volume creation ────────────────────────────────────────────────────────────────
    function _createVolumeMesh(id, shape, trigger) {
        let geometry, mesh, edges;

        if (shape.type === 'box') {
            geometry = new THREE.BoxGeometry(shape.w || 1, shape.h || 1, shape.d || 1);
            mesh = new THREE.Mesh(geometry, VOLUME_MATERIALS.idle.clone());
            mesh.position.set(
                (shape.x || 0) + (shape.w || 1) / 2,
                (shape.y || 0) + (shape.h || 1) / 2,
                (shape.z || 0) + (shape.d || 1) / 2
            );
        } else if (shape.type === 'cylinder') {
            geometry = new THREE.CylinderGeometry(
                shape.r || 0.5,
                shape.r || 0.5,
                shape.h || 1,
                32
            );
            mesh = new THREE.Mesh(geometry, VOLUME_MATERIALS.idle.clone());
            mesh.position.set(
                shape.cx || 0,
                (shape.z || 0) + (shape.h || 1) / 2,
                shape.cy || 0
            );
        } else {
            console.warn('[VolumeEditor] Unknown shape type:', shape.type);
            return;
        }

        mesh.userData.volumeId = id;
        mesh.userData.shape = shape;
        mesh.userData.trigger = trigger;

        // Add edges
        edges = new THREE.EdgesGeometry(geometry);
        const line = new THREE.LineSegments(edges, VOLUME_MATERIALS.edge.clone());
        mesh.add(line);

        _scene.add(mesh);
        _volumes.set(id, { mesh, shape, trigger, edges: line });

        return mesh;
    }

    function _clearAllVolumes() {
        _volumes.forEach((vol) => {
            _scene.remove(vol.mesh);
            vol.mesh.geometry.dispose();
        });
        _volumes.clear();
    }

    // ── Drawing interaction ────────────────────────────────────────────────────────────
    function _onPointerDown(event) {
        if (_drawMode === null || event.button !== 0) return;

        // Calculate mouse position in normalized device coordinates
        const rect = _renderer.domElement.getBoundingClientRect();
        _mouse.x = ((event.clientX - rect.left) / rect.width) * 2 - 1;
        _mouse.y = -((event.clientY - rect.top) / rect.height) * 2 + 1;

        _raycaster.setFromCamera(_mouse, _camera);

        const intersects = _raycaster.intersectObject(_groundPlane);
        if (intersects.length > 0) {
            const point = intersects[0].point;
            _drawStart = { x: point.x, z: point.z };

            if (_drawMode === 'box') {
                // Create box preview
                const boxGeo = new THREE.BoxGeometry(0.1, 0.1, 0.1);
                _drawPreview = new THREE.Mesh(boxGeo, VOLUME_MATERIALS.active.clone());
                _drawPreview.position.set(point.x, 0.05, point.z);
                _scene.add(_drawPreview);
            } else if (_drawMode === 'cylinder') {
                // Create cylinder preview
                const cylGeo = new THREE.CylinderGeometry(0.1, 0.1, 0.1, 32);
                _drawPreview = new THREE.Mesh(cylGeo, VOLUME_MATERIALS.active.clone());
                _drawPreview.position.set(point.x, 0.05, point.z);
                _scene.add(_drawPreview);
            }
        }
    }

    function _onPointerMove(event) {
        if (!_drawStart || !_drawPreview) return;

        const rect = _renderer.domElement.getBoundingClientRect();
        _mouse.x = ((event.clientX - rect.left) / rect.width) * 2 - 1;
        _mouse.y = -((event.clientY - rect.top) / rect.height) * 2 + 1;

        _raycaster.setFromCamera(_mouse, _camera);
        const intersects = _raycaster.intersectObject(_groundPlane);

        if (intersects.length > 0) {
            const point = intersects[0].point;

            if (_drawMode === 'box') {
                // Update box dimensions
                const width = Math.abs(point.x - _drawStart.x);
                const depth = Math.abs(point.z - _drawStart.z);
                const height = 1.0; // Default height

                _drawPreview.scale.set(width * 10, height * 10, depth * 10);
                _drawPreview.position.set(
                    Math.min(point.x, _drawStart.x) + width / 2,
                    height / 2,
                    Math.min(point.z, _drawStart.z) + depth / 2
                );
            } else if (_drawMode === 'cylinder') {
                // Update cylinder dimensions
                const radius = Math.sqrt(
                    Math.pow(point.x - _drawStart.x, 2) +
                    Math.pow(point.z - _drawStart.z, 2)
                );
                const height = 1.0;

                _drawPreview.scale.set(radius * 10, height * 10, radius * 10);
                _drawPreview.position.set(
                    _drawStart.x,
                    height / 2,
                    _drawStart.z
                );
            }
        }
    }

    function _onPointerUp(event) {
        if (!_drawStart || !_drawPreview) return;

        // Get final dimensions
        const scale = _drawPreview.scale;
        const pos = _drawPreview.position;

        let shape;
        if (_drawMode === 'box') {
            shape = {
                type: 'box',
                x: pos.x - scale.x / 10,
                y: pos.y - scale.y / 10,
                z: pos.z - scale.z / 10,
                w: scale.x / 5,
                h: scale.y / 5,
                d: scale.z / 5
            };
        } else if (_drawMode === 'cylinder') {
            shape = {
                type: 'cylinder',
                cx: pos.x,
                cy: pos.z,
                z: pos.z - scale.y / 10,
                r: scale.x / 10,
                h: scale.y / 5
            };
        }

        // Remove preview
        _scene.remove(_drawPreview);
        _drawPreview.geometry.dispose();
        _drawPreview = null;

        // Show height dialog
        _showHeightDialog(shape);

        // Reset draw state
        _drawStart = null;
    }

    function _showHeightDialog(shape) {
        const height = shape.h || 1.0;

        // Create a simple dialog
        const dialog = document.createElement('div');
        dialog.className = 'volume-height-dialog';
        dialog.innerHTML = `
            <div class="dialog-content">
                <h3>Set Volume Height</h3>
                <label>Height (meters):</label>
                <input type="range" id="volume-height-slider" min="0.1" max="5" step="0.1" value="${height}">
                <span id="volume-height-value">${height.toFixed(1)}</span>
                <div class="dialog-buttons">
                    <button id="volume-height-cancel" class="btn btn-secondary">Cancel</button>
                    <button id="volume-height-confirm" class="btn btn-primary">Create</button>
                </div>
            </div>
        `;

        document.body.appendChild(dialog);

        const slider = dialog.querySelector('#volume-height-slider');
        const value = dialog.querySelector('#volume-height-value');
        const cancel = dialog.querySelector('#volume-height-cancel');
        const confirm = dialog.querySelector('#volume-height-confirm');

        slider.oninput = () => {
            value.textContent = parseFloat(slider.value).toFixed(1);
        };

        cancel.onclick = () => {
            document.body.removeChild(dialog);
        };

        confirm.onclick = () => {
            const newHeight = parseFloat(slider.value);
            shape.h = newHeight;

            document.body.removeChild(dialog);

            // Create the volume
            if (_onVolumeCreated) {
                _onVolumeCreated(shape);
            }
        };
    }

    // ── Volume editing ────────────────────────────────────────────────────────────────
    function startEditing(volumeId) {
        const vol = _volumes.get(volumeId);
        if (!vol) return;

        _editingVolume = volumeId;
        _transformControls.attach(vol.mesh);
        _controls.enabled = false;
    }

    function stopEditing() {
        if (_transformControls) {
            _transformControls.detach();
        }
        _editingVolume = null;
        _controls.enabled = true;
    }

    function _updateVolumeShapeFromTransform() {
        if (!_editingVolume) return;

        const vol = _volumes.get(_editingVolume);
        if (!vol) return;

        const mesh = vol.mesh;
        const shape = vol.shape;

        // Update shape based on mesh position and scale
        if (shape.type === 'box') {
            const scale = mesh.scale;
            const pos = mesh.position;
            shape.w = scale.x;
            shape.h = scale.y;
            shape.d = scale.z;
            shape.x = pos.x - scale.x / 2;
            shape.y = pos.y - scale.y / 2;
            shape.z = pos.z - scale.z / 2;
        } else if (shape.type === 'cylinder') {
            const scale = mesh.scale;
            const pos = mesh.position;
            shape.r = scale.x;
            shape.h = scale.y;
            shape.cx = pos.x;
            shape.cy = pos.z;
            shape.z = pos.y - scale.y / 2;
        }

        // Update edges
        if (vol.edges) {
            mesh.remove(vol.edges);
            vol.edges.geometry.dispose();
            const edges = new THREE.EdgesGeometry(mesh.geometry);
            vol.edges = new THREE.LineSegments(edges, VOLUME_MATERIALS.edge.clone());
            mesh.add(vol.edges);
        }

        // Notify callback of shape change
        if (_onVolumeChanged) {
            _onVolumeChanged(_editingVolume, shape);
        }
    }

    // ── Volume deletion ────────────────────────────────────────────────────────────────
    function deleteVolume(volumeId) {
        const vol = _volumes.get(volumeId);
        if (!vol) return;

        // Detach from transform controls if attached
        if (_transformControls && _transformControls.object === vol.mesh) {
            _transformControls.detach();
        }

        _scene.remove(vol.mesh);
        vol.mesh.geometry.dispose();
        _volumes.delete(volumeId);

        // Notify callback
        if (_onVolumeDeleted) {
            _onVolumeDeleted(volumeId);
        }
    }

    // ── Volume visualization ────────────────────────────────────────────────────────────
    function setTriggerState(triggerId, state) {
        const vol = _volumes.get(triggerId);
        if (!vol) return;

        const mesh = vol.mesh;

        if (state === 'triggered') {
            mesh.material = VOLUME_MATERIALS.triggered.clone();
            // Pulse animation
            _animatePulse(triggerId);
        } else if (state === 'active') {
            mesh.material = VOLUME_MATERIALS.active.clone();
        } else {
            mesh.material = VOLUME_MATERIALS.idle.clone();
        }
    }

    function _animatePulse(triggerId) {
        const vol = _volumes.get(triggerId);
        if (!vol) return;

        const mesh = vol.mesh;
        const baseOpacity = 0.3;
        const pulseDuration = 500; // ms
        const startTime = Date.now();

        function pulse() {
            if (!_volumes.has(triggerId)) return;

            const elapsed = Date.now() - startTime;
            if (elapsed > pulseDuration * 2) {
                // Reset to base state
                mesh.material.opacity = baseOpacity;
                setTriggerState(triggerId, 'idle');
                return;
            }

            // Sine wave pulse
            const progress = (elapsed % pulseDuration) / pulseDuration;
            mesh.material.opacity = baseOpacity + Math.sin(progress * Math.PI) * 0.2;

            requestAnimationFrame(pulse);
        }

        requestAnimationFrame(pulse);
    }

    // ── Keyboard shortcuts ────────────────────────────────────────────────────────────
    function _onKeyDown(event) {
        if (event.key === 'Escape') {
            if (_drawMode !== null) {
                cancelDrawMode();
            } else if (_editingVolume !== null) {
                stopEditing();
            }
        } else if (event.key === 'Delete' || event.key === 'Backspace') {
            if (_editingVolume !== null && document.activeElement.tagName !== 'INPUT') {
                // Confirm deletion
                if (confirm('Delete this volume?')) {
                    deleteVolume(_editingVolume);
                    stopEditing();
                }
            }
        }
    }

    // ── Public API ────────────────────────────────────────────────────────────────────
    function startDrawMode(mode) {
        _drawMode = mode; // 'box' | 'cylinder'
        _controls.enabled = false;
        _renderer.domElement.style.cursor = 'crosshair';
    }

    function cancelDrawMode() {
        _drawMode = null;
        _drawStart = null;
        if (_drawPreview) {
            _scene.remove(_drawPreview);
            _drawPreview = null;
        }
        _controls.enabled = true;
        _renderer.domElement.style.cursor = 'default';
    }

    // ── Callbacks for integration ───────────────────────────────────────────────────────
    function onVolumeCreated(callback) {
        _onVolumeCreated = callback;
    }

    function onVolumeChanged(callback) {
        _onVolumeChanged = callback;
    }

    function onVolumeDeleted(callback) {
        _onVolumeDeleted = callback;
    }

    // Export public API
    window.VolumeEditor = {
        init,
        startDrawMode,
        cancelDrawMode,
        startEditing,
        stopEditing,
        deleteVolume,
        setTriggerState,
        getVolumeMeshes: function() {
            // Return array of trigger volume meshes for raycasting
            const meshes = [];
            _volumes.forEach(function(vol) {
                meshes.push(vol.mesh);
            });
            return meshes;
        },
        onVolumeCreated,
        onVolumeChanged,
        onVolumeDeleted
    };

    console.log('[VolumeEditor] Module loaded');
})();
