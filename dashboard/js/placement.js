/**
 * Spaxel Placement – Interactive node placement with GDOP coverage overlay
 *
 * Provides: TransformControls for dragging nodes in 3D, real-time GDOP
 * (Geometric Dilution of Precision) coverage overlay on the ground plane,
 * space dimension editor, virtual node support for planning, and REST API
 * integration for persisting positions.
 */

const Placement = (function () {
    'use strict';

    // ── module state ──────────────────────────────────────────────────────────
    var _scene, _camera, _renderer, _orbitControls;
    var _transformControls = null;
    var _selectedMAC = null;
    var _gdopEnabled = false;
    var _gdopMesh = null;
    var _gdopTexture = null;
    var _gdopDirty = false;
    var _roomConfig = null;
    var _nodeMACs = [];       // [{mac, virtual}]
    var _mouseDown = { x: 0, y: 0 };
    var _isDragging = false;

    var GDOP_RES = 128;
    var CLICK_THRESHOLD = 5;

    // ── GDOP colour mapping ──────────────────────────────────────────────────
    //
    // Multi-stop gradient: green → yellow-green → yellow → orange → red
    function gdopToColor(hdop) {
        if (!isFinite(hdop) || hdop > 20) return [180, 30, 30, 150];

        var t = Math.min(hdop / 8.0, 1.0);

        var stops = [
            { t: 0.00, r: 30,  g: 200, b: 80  },
            { t: 0.25, r: 140, g: 220, b: 50  },
            { t: 0.50, r: 240, g: 210, b: 40  },
            { t: 0.75, r: 240, g: 130, b: 30  },
            { t: 1.00, r: 210, g: 40,  b: 30  },
        ];

        var i = 0;
        for (; i < stops.length - 2; i++) {
            if (t <= stops[i + 1].t) break;
        }
        var s0 = stops[i], s1 = stops[i + 1];
        var f = (t - s0.t) / (s1.t - s0.t);

        return [
            Math.round(s0.r + (s1.r - s0.r) * f),
            Math.round(s0.g + (s1.g - s0.g) * f),
            Math.round(s0.b + (s1.b - s0.b) * f),
            150
        ];
    }

    // ── GDOP computation ─────────────────────────────────────────────────────
    //
    // Computes 2-D horizontal DOP (HDOP) for a point (px, pz) on the ground
    // plane, given anchor nodes at (x, y, z).  The observation model assumes
    // range-based localisation; the 2×2 normal matrix G = HᵀH is inverted to
    // obtain HDOP = √ trace(G⁻¹).

    function computeHDOP(px, pz, nodes) {
        if (nodes.length < 2) return Infinity;

        var g00 = 0, g01 = 0, g11 = 0;

        for (var i = 0; i < nodes.length; i++) {
            var n = nodes[i];
            var dx = px - n.x;
            var dz = pz - n.z;
            var dy = n.y;
            var d2 = dx * dx + dy * dy + dz * dz;
            if (d2 < 1e-6) continue;
            var d = Math.sqrt(d2);
            var ux = dx / d;
            var uz = dz / d;
            g00 += ux * ux;
            g01 += ux * uz;
            g11 += uz * uz;
        }

        var det = g00 * g11 - g01 * g01;
        if (det < 1e-8) return Infinity;

        return Math.sqrt(Math.max(0, (g00 + g11) / det));
    }

    // ── Read live mesh positions for GDOP ────────────────────────────────────

    function getNodePositions() {
        var positions = [];
        for (var i = 0; i < _nodeMACs.length; i++) {
            var entry = _nodeMACs[i];
            var mesh = Viz3D.getNodeMesh(entry.mac);
            if (mesh) {
                positions.push({
                    x: mesh.position.x,
                    y: mesh.position.y,
                    z: mesh.position.z
                });
            }
        }
        return positions;
    }

    // ── GDOP overlay ─────────────────────────────────────────────────────────

    function updateGDOPOverlay() {
        var nodes = getNodePositions();

        if (!_gdopEnabled || !_roomConfig || nodes.length < 2) {
            if (_gdopMesh) _gdopMesh.visible = false;
            return;
        }

        var w = _roomConfig.width;
        var d = _roomConfig.depth;
        var ox = _roomConfig.origin_x || 0;
        var oz = _roomConfig.origin_z || 0;

        // Create or reuse DataTexture
        if (!_gdopTexture) {
            var data = new Uint8Array(GDOP_RES * GDOP_RES * 4);
            _gdopTexture = new THREE.DataTexture(data, GDOP_RES, GDOP_RES, THREE.RGBAFormat);
            _gdopTexture.magFilter = THREE.LinearFilter;
            _gdopTexture.minFilter = THREE.LinearFilter;
        }

        var data = _gdopTexture.image.data;

        for (var iz = 0; iz < GDOP_RES; iz++) {
            var pz = oz + (iz / (GDOP_RES - 1)) * d;
            for (var ix = 0; ix < GDOP_RES; ix++) {
                var px = ox + (ix / (GDOP_RES - 1)) * w;
                var hdop = computeHDOP(px, pz, nodes);
                var c = gdopToColor(hdop);
                var idx = (iz * GDOP_RES + ix) * 4;
                data[idx]     = c[0];
                data[idx + 1] = c[1];
                data[idx + 2] = c[2];
                data[idx + 3] = c[3];
            }
        }

        _gdopTexture.needsUpdate = true;

        if (!_gdopMesh) {
            var geo = new THREE.PlaneGeometry(w, d);
            var mat = new THREE.MeshBasicMaterial({
                map: _gdopTexture,
                transparent: true,
                depthWrite: false,
                side: THREE.DoubleSide
            });
            _gdopMesh = new THREE.Mesh(geo, mat);
            _gdopMesh.rotation.x = -Math.PI / 2;
            _gdopMesh.position.set(ox + w / 2, 0.006, oz + d / 2);
            _scene.add(_gdopMesh);
        } else {
            _gdopMesh.geometry.dispose();
            _gdopMesh.geometry = new THREE.PlaneGeometry(w, d);
            _gdopMesh.position.set(ox + w / 2, 0.006, oz + d / 2);
        }

        _gdopMesh.visible = true;
    }

    function rebuildGDOPIfDirty() {
        if (_gdopDirty && _gdopEnabled) {
            updateGDOPOverlay();
            _gdopDirty = false;
        }
    }

    // ── TransformControls ────────────────────────────────────────────────────

    function initTransformControls() {
        _transformControls = new THREE.TransformControls(_camera, _renderer.domElement);
        _transformControls.setMode('translate');
        _transformControls.setSize(0.75);
        _scene.add(_transformControls.getHelper());

        _transformControls.addEventListener('dragging-changed', function (event) {
            _orbitControls.enabled = !event.value;
            _isDragging = event.value;

            // Save position on drag end
            if (!event.value && _selectedMAC) {
                var mesh = Viz3D.getNodeMesh(_selectedMAC);
                if (mesh) {
                    saveNodePosition(_selectedMAC, mesh.position.x, mesh.position.y, mesh.position.z);
                }
            }
        });

        _transformControls.addEventListener('objectChange', function () {
            if (!_selectedMAC || !_roomConfig) return;

            var obj = _transformControls.object;
            var ox = _roomConfig.origin_x || 0;
            var oz = _roomConfig.origin_z || 0;

            // Clamp to room bounds
            obj.position.x = Math.max(ox, Math.min(ox + _roomConfig.width, obj.position.x));
            obj.position.y = Math.max(0.05, Math.min(_roomConfig.height, obj.position.y));
            obj.position.z = Math.max(oz, Math.min(oz + _roomConfig.depth, obj.position.z));

            _gdopDirty = true;

            // Update link lines to follow dragged node
            if (Viz3D.rebuildLinkLines) Viz3D.rebuildLinkLines();
        });
    }

    // ── Node selection ───────────────────────────────────────────────────────

    function selectNode(mac) {
        if (_selectedMAC === mac) return;

        deselectNode();

        var mesh = Viz3D.getNodeMesh(mac);
        if (!mesh) return;

        _selectedMAC = mac;
        _transformControls.attach(mesh);

        // Highlight in node panel
        document.querySelectorAll('.node-item').forEach(function (el) {
            el.classList.toggle('selected', el.dataset.mac === mac);
        });

        // Show delete button for virtual nodes
        var isVirtual = _nodeMACs.some(function (n) { return n.mac === mac && n.virtual; });
        var delBtn = document.getElementById('delete-node-btn');
        if (delBtn) delBtn.style.display = isVirtual ? 'inline-block' : 'none';
    }

    function deselectNode() {
        if (_transformControls) _transformControls.detach();
        _selectedMAC = null;

        document.querySelectorAll('.node-item').forEach(function (el) {
            el.classList.remove('selected');
        });

        var delBtn = document.getElementById('delete-node-btn');
        if (delBtn) delBtn.style.display = 'none';
    }

    // ── Pointer handling (click-to-select) ───────────────────────────────────

    function onPointerDown(event) {
        _mouseDown.x = event.clientX;
        _mouseDown.y = event.clientY;
    }

    function onPointerUp(event) {
        var dx = event.clientX - _mouseDown.x;
        var dy = event.clientY - _mouseDown.y;
        if (Math.sqrt(dx * dx + dy * dy) > CLICK_THRESHOLD) return;
        if (_isDragging) return;
        if (event.target !== _renderer.domElement) return;

        var rect = _renderer.domElement.getBoundingClientRect();
        var mouse = new THREE.Vector2(
            ((event.clientX - rect.left) / rect.width) * 2 - 1,
            -((event.clientY - rect.top) / rect.height) * 2 + 1
        );

        var raycaster = new THREE.Raycaster();
        raycaster.setFromCamera(mouse, _camera);

        var meshes = [];
        for (var i = 0; i < _nodeMACs.length; i++) {
            var m = Viz3D.getNodeMesh(_nodeMACs[i].mac);
            if (m) meshes.push(m);
        }

        var intersects = raycaster.intersectObjects(meshes);
        if (intersects.length > 0) {
            for (var j = 0; j < _nodeMACs.length; j++) {
                var mesh = Viz3D.getNodeMesh(_nodeMACs[j].mac);
                if (mesh === intersects[0].object) {
                    selectNode(_nodeMACs[j].mac);
                    return;
                }
            }
        }

        deselectNode();
    }

    // ── Keyboard shortcuts ───────────────────────────────────────────────────

    function onKeyDown(event) {
        if (event.target.tagName === 'INPUT') return;

        if (event.key === 'Escape') {
            deselectNode();
            var panel = document.getElementById('room-editor-panel');
            if (panel) panel.style.display = 'none';
        }
        if (event.key === 'g' || event.key === 'G') {
            toggleGDOP();
        }
    }

    // ── REST API calls ───────────────────────────────────────────────────────

    function saveNodePosition(mac, x, y, z) {
        fetch('/api/nodes/' + encodeURIComponent(mac) + '/position', {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ x: x, y: y, z: z })
        }).catch(function (e) {
            console.error('[Placement] save position failed:', e);
        });
    }

    function addVirtualNode(name, x, y, z) {
        var mac = 'AA:BB:CC:' +
            Math.floor(Math.random() * 256).toString(16).padStart(2, '0').toUpperCase() + ':' +
            Math.floor(Math.random() * 256).toString(16).padStart(2, '0').toUpperCase() + ':' +
            Math.floor(Math.random() * 256).toString(16).padStart(2, '0').toUpperCase();

        fetch('/api/nodes/virtual', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ mac: mac, name: name || 'Virtual', x: x, y: y, z: z })
        }).then(function (resp) {
            if (resp.ok) console.log('[Placement] virtual node added:', mac);
        }).catch(function (e) {
            console.error('[Placement] add virtual node failed:', e);
        });
    }

    function deleteNodeFromServer(mac) {
        fetch('/api/nodes/' + encodeURIComponent(mac), {
            method: 'DELETE'
        }).then(function () {
            console.log('[Placement] node deleted:', mac);
        }).catch(function (e) {
            console.error('[Placement] delete node failed:', e);
        });
    }

    function saveRoomConfig(width, depth, height) {
        fetch('/api/room', {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ width: width, depth: depth, height: height, origin_x: 0, origin_z: 0 })
        }).catch(function (e) {
            console.error('[Placement] save room failed:', e);
        });
    }

    // ── Room editor ──────────────────────────────────────────────────────────

    function toggleRoomEditor() {
        var panel = document.getElementById('room-editor-panel');
        if (!panel) return;
        var visible = panel.style.display !== 'none';
        panel.style.display = visible ? 'none' : 'block';
        var btn = document.getElementById('room-editor-btn');
        if (btn) btn.classList.toggle('active', !visible);
    }

    function applyRoomFromEditor() {
        var w = parseFloat(document.getElementById('room-width').value) || 6;
        var d = parseFloat(document.getElementById('room-depth').value) || 5;
        var h = parseFloat(document.getElementById('room-height').value) || 2.5;
        saveRoomConfig(w, d, h);
    }

    // ── Init ─────────────────────────────────────────────────────────────────

    function init(scene, camera, renderer, controls) {
        _scene = scene;
        _camera = camera;
        _renderer = renderer;
        _orbitControls = controls;

        initTransformControls();

        renderer.domElement.addEventListener('pointerdown', onPointerDown);
        renderer.domElement.addEventListener('pointerup', onPointerUp);
        window.addEventListener('keydown', onKeyDown);

        console.log('[Placement] initialized');
    }

    // ── Tick (called from animation loop) ────────────────────────────────────

    function update() {
        // Deselect if selected node was removed
        if (_selectedMAC && !Viz3D.getNodeMesh(_selectedMAC)) {
            deselectNode();
        }
        rebuildGDOPIfDirty();
    }

    // ── Data from WebSocket registry_state ───────────────────────────────────

    function handleRegistryState(msg) {
        if (msg.room) {
            _roomConfig = msg.room;
            var wEl = document.getElementById('room-width');
            var dEl = document.getElementById('room-depth');
            var hEl = document.getElementById('room-height');
            if (wEl) wEl.value = msg.room.width;
            if (dEl) dEl.value = msg.room.depth;
            if (hEl) hEl.value = msg.room.height;
        }

        if (msg.nodes) {
            _nodeMACs = msg.nodes.map(function (n) {
                return { mac: n.mac, virtual: !!n.virtual };
            });
        }

        _gdopDirty = true;
    }

    // ── GDOP toggle ──────────────────────────────────────────────────────────

    function toggleGDOP() {
        _gdopEnabled = !_gdopEnabled;
        var btn = document.getElementById('gdop-toggle-btn');
        if (btn) btn.classList.toggle('active', _gdopEnabled);

        var legend = document.getElementById('gdop-legend');
        if (legend) legend.classList.toggle('visible', _gdopEnabled);

        if (_gdopEnabled) {
            updateGDOPOverlay();
        } else if (_gdopMesh) {
            _gdopMesh.visible = false;
        }
    }

    // ── Public API ────────────────────────────────────────────────────────────

    return {
        init: init,
        update: update,
        handleRegistryState: handleRegistryState,
        selectNode: selectNode,
        deselectNode: deselectNode,
        toggleGDOP: toggleGDOP,
        toggleRoomEditor: toggleRoomEditor,
        applyRoomFromEditor: applyRoomFromEditor,
        addVirtualNode: function () {
            var cx = _roomConfig ? (_roomConfig.origin_x || 0) + _roomConfig.width / 2 : 3;
            var cz = _roomConfig ? (_roomConfig.origin_z || 0) + _roomConfig.depth / 2 : 2.5;
            var h = _roomConfig ? _roomConfig.height * 0.8 : 2.0;
            addVirtualNode('Virtual ' + _nodeMACs.length, cx, h, cz);
        },
        deleteSelectedNode: function () {
            if (!_selectedMAC) return;
            var isVirtual = _nodeMACs.some(function (n) { return n.mac === _selectedMAC && n.virtual; });
            if (!isVirtual) return;
            var mac = _selectedMAC;
            deselectNode();
            deleteNodeFromServer(mac);
        }
    };
})();
