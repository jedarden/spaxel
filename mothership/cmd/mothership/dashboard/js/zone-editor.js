/**
 * Spaxel Zone Editor – Interactive zone placement with TransformControls
 *
 * Provides: TransformControls for dragging/resizing zones in 3D, zone
 * creation/editing workflow, and REST API integration for persisting
 * zone definitions.
 */

const ZoneEditor = (function () {
    'use strict';

    // ── module state ──────────────────────────────────────────────────────────
    var _scene, _camera, _renderer, _orbitControls;
    var _transformControls = null;
    var _selectedZoneID = null;
    var _editMode = false;           // true when editing an existing zone
    var _newZoneMesh = null;        // temporary mesh for new zone
    var _zones = [];                 // [{id, name, x, y, z, w, d, h, color, zoneType}]
    var _mouseDown = { x: 0, y: 0 };
    var _isDragging = false;

    var DEFAULT_WIDTH = 4.0;         // default room width (m)
    var DEFAULT_DEPTH = 3.0;         // default room depth (m)
    var DEFAULT_HEIGHT = 2.5;        // default room height (m)
    var DEFAULT_COLOR = '#3b82f6';    // blue color for zones

    // ── Zone mesh creation ────────────────────────────────────────────────────

    function createZoneMesh(width, depth, height, position, color) {
        var geometry = new THREE.BoxGeometry(width, height, depth);
        var material = new THREE.MeshBasicMaterial({
            color: parseInt(color.replace('#', '0x'), 16),
            transparent: true,
            opacity: 0.1,
            side: THREE.DoubleSide,
            depthWrite: false
        });

        var mesh = new THREE.Mesh(geometry, material);
        mesh.position.set(position.x, position.y, position.z);

        // Add edge helper for better visibility
        var edges = new THREE.EdgesGeometry(geometry);
        var line = new THREE.LineSegments(edges, new THREE.LineBasicMaterial({
            color: parseInt(color.replace('#', '0x'), 16),
            transparent: true,
            opacity: 0.3
        }));
        mesh.add(line);

        return mesh;
    }

    // ── TransformControls setup ─────────────────────────────────────────────────

    function initTransformControls() {
        _transformControls = new THREE.TransformControls(_camera, _renderer.domElement);
        _transformControls.setMode('translate');
        _transformControls.setSize(0.75);
        _scene.add(_transformControls.getHelper());

        _transformControls.addEventListener('dragging-changed', function (event) {
            _orbitControls.enabled = !event.value;
            _isDragging = event.value;

            // Save zone position on drag end
            if (!event.value && _selectedZoneID) {
                saveZonePosition();
            }
        });

        _transformControls.addEventListener('objectChange', function () {
            if (!_selectedZoneID) return;

            var obj = _transformControls.object;

            // Clamp to reasonable bounds (above floor, below ceiling)
            obj.position.y = Math.max(0.1, Math.min(10.0, obj.position.y));

            // Update zone panel fields
            updateZonePanelFromMesh(obj);
        });
    }

    // ── Zone selection ───────────────────────────────────────────────────────

    function selectZone(zoneID) {
        if (_selectedZoneID === zoneID) return;

        deselectZone();

        var zone = _zones.find(function (z) { return z.id === zoneID; });
        if (!zone) return;

        _selectedZoneID = zoneID;
        _editMode = true;

        // Create or update mesh for this zone
        var mesh = getZoneMesh(zoneID);
        if (!mesh) {
            mesh = createZoneMeshFromData(zone);
            mesh.userData.zoneID = zoneID;
            _scene.add(mesh);
        }

        _transformControls.attach(mesh);

        // Show zone editor panel
        showZoneEditorPanel(zone);

        // Highlight in zone list
        document.querySelectorAll('.zone-item').forEach(function (el) {
            el.classList.toggle('selected', el.dataset.zoneId === zoneID);
        });
    }

    function deselectZone() {
        if (_transformControls) _transformControls.detach();
        _selectedZoneID = null;
        _editMode = false;

        // Remove new zone mesh if exists
        if (_newZoneMesh) {
            _scene.remove(_newZoneMesh);
            _newZoneMesh.geometry.dispose();
            _newZoneMesh.material.dispose();
            _newZoneMesh = null;
        }

        document.querySelectorAll('.zone-item').forEach(function (el) {
            el.classList.remove('selected');
        });

        var panel = document.getElementById('zone-editor-panel');
        if (panel) panel.style.display = 'none';
    }

    // ── Zone creation workflow ───────────────────────────────────────────────

    function startNewZone() {
        deselectZone();

        // Position zone at camera focal point, 2m away
        var direction = new THREE.Vector3();
        _camera.getWorldDirection(direction);
        var position = new THREE.Vector3().copy(_camera.position).add(direction.multiplyScalar(2));
        position.y = Math.max(0.5, Math.min(2.0, position.y)); // Default height

        _newZoneMesh = createZoneMesh(DEFAULT_WIDTH, DEFAULT_DEPTH, DEFAULT_HEIGHT, position, DEFAULT_COLOR);
        _newZoneMesh.userData.isNewZone = true;
        _scene.add(_newZoneMesh);

        _transformControls.attach(_newZoneMesh);

        // Show zone editor panel for new zone
        showNewZonePanel();

        console.log('[ZoneEditor] Creating new zone at camera focus');
    }

    function saveNewZone() {
        if (!_newZoneMesh) return;

        var name = document.getElementById('zone-name').value || 'New Zone';
        var x = parseFloat(document.getElementById('zone-x').value) || 0;
        var y = parseFloat(document.getElementById('zone-y').value) || 0;
        var z = parseFloat(document.getElementById('zone-z').value) || 0;
        var w = parseFloat(document.getElementById('zone-w').value) || DEFAULT_WIDTH;
        var d = parseFloat(document.getElementById('zone-d').value) || DEFAULT_DEPTH;
        var h = parseFloat(document.getElementById('zone-h').value) || DEFAULT_HEIGHT;
        var color = document.getElementById('zone-color').value || DEFAULT_COLOR;
        var zoneType = document.getElementById('zone-type').value || 'general';

        // Calculate zone data from mesh position
        var mesh = _newZoneMesh;
        var zoneData = {
            id: undefined, // will be auto-generated
            name: name,
            x: mesh.position.x - w / 2,
            y: mesh.position.y - h / 2,
            z: mesh.position.z - d / 2,
            w: w,
            d: d,
            h: h,
            color: color,
            zone_type: zoneType,
            enabled: true
        };

        // Create zone via REST API
        fetch('/api/zones', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(zoneData)
        }).then(function (resp) {
            if (resp.ok) {
                return resp.json();
            }
            throw new Error('Failed to create zone');
        }).then(function (created) {
            console.log('[ZoneEditor] Zone created:', created.id);
            deselectZone();
            // The dashboard will receive a zone_change WebSocket message
            // and Viz3D will create the permanent mesh
        }).catch(function (e) {
            console.error('[ZoneEditor] Create zone failed:', e);
            alert('Failed to create zone: ' + e.message);
        });
    }

    // ── Zone update workflow ─────────────────────────────────────────────────

    function saveZonePosition() {
        if (!_selectedZoneID || !_editMode) return;

        var zone = _zones.find(function (z) { return z.id === _selectedZoneID; });
        if (!zone) return;

        var mesh = _transformControls.object;
        if (!mesh) return;

        // Calculate updated zone data from mesh
        var updatedData = calculateZoneDataFromMesh(mesh, zone.name, zone.color, zone.zone_type);

        // Update zone via REST API
        fetch('/api/zones/' + encodeURIComponent(_selectedZoneID), {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(updatedData)
        }).then(function (resp) {
            if (resp.ok) {
                console.log('[ZoneEditor] Zone updated:', _selectedZoneID);
            } else {
                throw new Error('Failed to update zone');
            }
        }).catch(function (e) {
            console.error('[ZoneEditor] Update zone failed:', e);
        });
    }

    function deleteSelectedZone() {
        if (!_selectedZoneID || !_editMode) return;

        if (!confirm('Delete this zone?')) return;

        fetch('/api/zones/' + encodeURIComponent(_selectedZoneID), {
            method: 'DELETE'
        }).then(function (resp) {
            if (resp.ok) {
                console.log('[ZoneEditor] Zone deleted:', _selectedZoneID);
                deselectZone();
            } else {
                throw new Error('Failed to delete zone');
            }
        }).catch(function (e) {
            console.error('[ZoneEditor] Delete zone failed:', e);
            alert('Failed to delete zone: ' + e.message);
        });
    }

    // ── Zone data calculation ────────────────────────────────────────────────

    function calculateZoneDataFromMesh(mesh, name, color, zoneType) {
        // Get zone position from mesh
        var position = mesh.position;

        // Get zone dimensions (stored in userData if created by us)
        var w, h, d;
        if (mesh.userData.zoneWidth) {
            w = mesh.userData.zoneWidth;
            h = mesh.userData.zoneHeight;
            d = mesh.userData.zoneDepth;
        } else {
            // Extract from geometry scale or use defaults
            w = DEFAULT_WIDTH;
            h = DEFAULT_HEIGHT;
            d = DEFAULT_DEPTH;
        }

        return {
            id: _editMode ? _selectedZoneID : undefined,
            name: name,
            x: position.x - w / 2,
            y: position.y - h / 2,
            z: position.z - d / 2,
            w: w,
            d: d,
            h: h,
            color: color,
            zone_type: zoneType || 'general',
            enabled: true
        };
    }

    function createZoneMeshFromData(zone) {
        var width = zone.w || DEFAULT_WIDTH;
        var height = zone.h || DEFAULT_HEIGHT;
        var depth = zone.d || DEFAULT_DEPTH;
        var color = zone.color || DEFAULT_COLOR;

        // Calculate center position
        var position = new THREE.Vector3(
            (zone.x || 0) + width / 2,
            (zone.y || 0) + height / 2,
            (zone.z || 0) + depth / 2
        );

        var mesh = createZoneMesh(width, depth, height, position, color);
        mesh.userData.zoneID = zone.id;
        mesh.userData.zoneWidth = width;
        mesh.userData.zoneHeight = height;
        mesh.userData.zoneDepth = depth;

        return mesh;
    }

    function getZoneMesh(zoneID) {
        for (var i = 0; i < _scene.children.length; i++) {
            var obj = _scene.children[i];
            if (obj.userData && obj.userData.zoneID === zoneID) {
                return obj;
            }
        }
        return null;
    }

    // ── Zone editor panel ────────────────────────────────────────────────────

    function showNewZonePanel() {
        var panel = document.getElementById('zone-editor-panel');
        if (!panel) return;

        panel.style.display = 'block';

        // Set default values
        document.getElementById('zone-name').value = 'New Zone';
        document.getElementById('zone-w').value = DEFAULT_WIDTH;
        document.getElementById('zone-d').value = DEFAULT_DEPTH;
        document.getElementById('zone-h').value = DEFAULT_HEIGHT;
        document.getElementById('zone-color').value = DEFAULT_COLOR;
        document.getElementById('zone-type').value = 'general';

        // Show save button, hide update/delete buttons
        document.getElementById('zone-save-btn').style.display = 'inline-block';
        document.getElementById('zone-update-btn').style.display = 'none';
        document.getElementById('zone-delete-btn').style.display = 'none';

        // Set panel title
        panel.querySelector('h3').textContent = 'New Zone';
    }

    function showZoneEditorPanel(zone) {
        var panel = document.getElementById('zone-editor-panel');
        if (!panel) return;

        panel.style.display = 'block';

        // Populate fields with zone data
        document.getElementById('zone-name').value = zone.name || '';
        document.getElementById('zone-x').value = (zone.x || 0).toFixed(2);
        document.getElementById('zone-y').value = (zone.y || 0).toFixed(2);
        document.getElementById('zone-z').value = (zone.z || 0).toFixed(2);
        document.getElementById('zone-w').value = (zone.w || DEFAULT_WIDTH).toFixed(2);
        document.getElementById('zone-d').value = (zone.d || DEFAULT_DEPTH).toFixed(2);
        document.getElementById('zone-h').value = (zone.h || DEFAULT_HEIGHT).toFixed(2);
        document.getElementById('zone-color').value = zone.color || DEFAULT_COLOR;
        document.getElementById('zone-type').value = zone.zone_type || 'general';

        // Show update/delete buttons, hide save button
        document.getElementById('zone-save-btn').style.display = 'none';
        document.getElementById('zone-update-btn').style.display = 'inline-block';
        document.getElementById('zone-delete-btn').style.display = 'inline-block';

        // Set panel title
        panel.querySelector('h3').textContent = 'Edit Zone';
    }

    function updateZonePanelFromMesh(mesh) {
        // Update position display
        var posDisplay = document.getElementById('zone-position-display');
        if (posDisplay) {
            posDisplay.textContent = 'X: ' + mesh.position.x.toFixed(2) +
                ' Y: ' + mesh.position.y.toFixed(2) +
                ' Z: ' + mesh.position.z.toFixed(2);
        }

        // Update dimension fields from mesh userData
        if (mesh.userData.zoneWidth) {
            document.getElementById('zone-w').value = mesh.userData.zoneWidth.toFixed(2);
            document.getElementById('zone-d').value = mesh.userData.zoneDepth.toFixed(2);
            document.getElementById('zone-h').value = mesh.userData.zoneHeight.toFixed(2);
        }
    }

    // ── Pointer handling (click-to-select) ─────────────────────────────────────

    function onPointerDown(event) {
        _mouseDown.x = event.clientX;
        _mouseDown.y = event.clientY;
    }

    function onPointerUp(event) {
        var dx = event.clientX - _mouseDown.x;
        var dy = event.clientY - _mouseDown.y;
        if (Math.sqrt(dx * dx + dy * dy) > 5) return; // Not a click
        if (_isDragging) return;
        if (event.target !== _renderer.domElement) return;

        var rect = _renderer.domElement.getBoundingClientRect();
        var mouse = new THREE.Vector2(
            ((event.clientX - rect.left) / rect.width) * 2 - 1,
            -((event.clientY - rect.top) / rect.height) * 2 + 1
        );

        var raycaster = new THREE.Raycaster();
        raycaster.setFromCamera(mouse, _camera);

        // Check for zone mesh intersections
        var zoneMeshes = [];
        for (var i = 0; i < _zones.length; i++) {
            var mesh = getZoneMesh(_zones[i].id);
            if (mesh) zoneMeshes.push(mesh);
        }

        var intersects = raycaster.intersectObjects(zoneMeshes);
        if (intersects.length > 0) {
            var mesh = intersects[0].object;
            if (mesh.userData && mesh.userData.zoneID) {
                selectZone(mesh.userData.zoneID);
                return;
            }
        }

        // Deselect if clicked elsewhere
        deselectZone();
    }

    // ── Keyboard shortcuts ─────────────────────────────────────────────────────

    function onKeyDown(event) {
        if (event.target.tagName === 'INPUT') return;

        if (event.key === 'Escape') {
            deselectZone();
        }
        if (event.key === 'Delete' || event.key === 'Backspace') {
            if (_selectedZoneID && _editMode) {
                deleteSelectedZone();
            }
        }
    }

    // ── Data from WebSocket/Viz3D ───────────────────────────────────────────────

    function handleZoneUpdate(zones) {
        _zones = zones.map(function (z) {
            return {
                id: z.id,
                name: z.name,
                x: z.x, y: z.y, z: z.z,
                w: z.w, d: z.d, h: z.h,
                color: z.color || '#3b82f6',
                zoneType: z.zone_type || 'general',
                enabled: z.enabled
            };
        });
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

        console.log('[ZoneEditor] initialized');
    }

    // ── Tick (called from animation loop) ───────────────────────────────────────

    function update() {
        // Deselect if selected zone was deleted
        if (_selectedZoneID && _editMode) {
            var zone = _zones.find(function (z) { return z.id === _selectedZoneID; });
            if (!zone) {
                deselectZone();
            }
        }
    }

    // ── Public API ────────────────────────────────────────────────────────────

    return {
        init: init,
        update: update,
        handleZoneUpdate: handleZoneUpdate,
        startNewZone: startNewZone,
        saveNewZone: saveNewZone,
        deleteSelectedZone: deleteSelectedZone,
        selectZone: selectZone,
        deselectZone: deselectZone,
        togglePanel: function () {
            var panel = document.getElementById('zone-editor-panel');
            if (!panel) return;
            var visible = panel.style.display !== 'none';
            panel.style.display = visible ? 'none' : 'block';
            var btn = document.getElementById('add-zone-btn');
            if (btn) btn.classList.toggle('active', !visible);
        }
    };
})();
