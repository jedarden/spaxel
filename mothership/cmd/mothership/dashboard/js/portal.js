/**
 * Spaxel Portal Editor – Interactive portal placement with TransformControls
 *
 * Provides: TransformControls for dragging/rotating portals in 3D, portal
 * creation/editing workflow, zone assignment, and REST API integration for
 * persisting portal definitions.
 */

const PortalEditor = (function () {
    'use strict';

    // ── module state ──────────────────────────────────────────────────────────
    var _scene, _camera, _renderer, _orbitControls;
    var _transformControls = null;
    var _selectedPortalID = null;
    var _editMode = false;           // true when editing an existing portal
    var _newPortalMesh = null;       // temporary mesh for new portal
    var _portals = [];               // [{id, name, zoneA, zoneB, p1, p2, p3, width, height}]
    var _zones = [];                 // [{id, name, minX, minY, minZ, maxX, maxY, maxZ}]
    var _mouseDown = { x: 0, y: 0 };
    var _isDragging = false;

    var DEFAULT_WIDTH = 0.9;         // standard door width (m)
    var DEFAULT_HEIGHT = 2.1;        // standard door height (m)
    var PORTAL_COLOR = 0xffa726;     // orange color for portal planes

    // ── Portal mesh creation ───────────────────────────────────────────────────

    function createPortalMesh(width, height, position, quaternion) {
        var geometry = new THREE.PlaneGeometry(width, height);
        var material = new THREE.MeshBasicMaterial({
            color: PORTAL_COLOR,
            transparent: true,
            opacity: 0.3,
            side: THREE.DoubleSide,
            depthWrite: false
        });

        var mesh = new THREE.Mesh(geometry, material);
        mesh.position.copy(position);
        mesh.quaternion.copy(quaternion);

        // Add edge helper for better visibility
        var edges = new THREE.EdgesGeometry(geometry);
        var line = new THREE.LineSegments(edges, new THREE.LineBasicMaterial({ color: 0xffcc80 }));
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

            // Save portal position on drag end
            if (!event.value && _selectedPortalID) {
                savePortalPosition();
            }
        });

        _transformControls.addEventListener('objectChange', function () {
            if (!_selectedPortalID) return;

            var obj = _transformControls.object;

            // Clamp to reasonable bounds (above floor, below ceiling)
            obj.position.y = Math.max(0.1, Math.min(4.0, obj.position.y));

            // Update portal panel fields
            updatePortalPanelFromMesh(obj);
        });
    }

    // ── Portal selection ───────────────────────────────────────────────────────

    function selectPortal(portalID) {
        if (_selectedPortalID === portalID) return;

        deselectPortal();

        var portal = _portals.find(function (p) { return p.id === portalID; });
        if (!portal) return;

        _selectedPortalID = portalID;
        _editMode = true;

        // Create or update mesh for this portal
        var mesh = getPortalMesh(portalID);
        if (!mesh) {
            mesh = createPortalMeshFromData(portal);
            mesh.userData.portalID = portalID;
            _scene.add(mesh);
        }

        _transformControls.attach(mesh);

        // Show portal editor panel
        showPortalEditorPanel(portal);

        // Highlight in portal list
        document.querySelectorAll('.portal-item').forEach(function (el) {
            el.classList.toggle('selected', el.dataset.portalId === portalID);
        });
    }

    function deselectPortal() {
        if (_transformControls) _transformControls.detach();
        _selectedPortalID = null;
        _editMode = false;

        // Remove new portal mesh if exists
        if (_newPortalMesh) {
            _scene.remove(_newPortalMesh);
            _newPortalMesh.geometry.dispose();
            _newPortalMesh.material.dispose();
            _newPortalMesh = null;
        }

        document.querySelectorAll('.portal-item').forEach(function (el) {
            el.classList.remove('selected');
        });

        var panel = document.getElementById('portal-editor-panel');
        if (panel) panel.style.display = 'none';
    }

    // ── Portal creation workflow ───────────────────────────────────────────────

    function startNewPortal() {
        deselectPortal();

        // Position portal at camera focal point, 2m away
        var direction = new THREE.Vector3();
        _camera.getWorldDirection(direction);
        var position = new THREE.Vector3().copy(_camera.position).add(direction.multiplyScalar(2));
        position.y = Math.max(1.0, Math.min(2.5, position.y)); // Default height

        // Create default portal facing camera
        var quaternion = new THREE.Quaternion();
        quaternion.setFromUnitVectors(new THREE.Vector3(0, 0, 1), direction.normalize());

        _newPortalMesh = createPortalMesh(DEFAULT_WIDTH, DEFAULT_HEIGHT, position, quaternion);
        _newPortalMesh.userData.isNewPortal = true;
        _scene.add(_newPortalMesh);

        _transformControls.attach(_newPortalMesh);

        // Show portal editor panel for new portal
        showNewPortalPanel();

        console.log('[PortalEditor] Creating new portal at camera focus');
    }

    function saveNewPortal() {
        if (!_newPortalMesh) return;

        var name = document.getElementById('portal-name').value || 'New Portal';
        var zoneA = document.getElementById('portal-zone-a').value;
        var zoneB = document.getElementById('portal-zone-b').value;
        var width = parseFloat(document.getElementById('portal-width').value) || DEFAULT_WIDTH;
        var height = parseFloat(document.getElementById('portal-height').value) || DEFAULT_HEIGHT;

        // Calculate portal plane from mesh transform
        var portalData = calculatePortalDataFromMesh(_newPortalMesh, name, zoneA, zoneB, width, height);

        // Create portal via REST API
        fetch('/api/portals', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(portalData)
        }).then(function (resp) {
            if (resp.ok) {
                return resp.json();
            }
            throw new Error('Failed to create portal');
        }).then(function (created) {
            console.log('[PortalEditor] Portal created:', created.id);
            deselectPortal();
            // The dashboard will receive a portal_change WebSocket message
            // and Viz3D will create the permanent mesh
        }).catch(function (e) {
            console.error('[PortalEditor] Create portal failed:', e);
            alert('Failed to create portal: ' + e.message);
        });
    }

    // ── Portal update workflow ─────────────────────────────────────────────────

    function savePortalPosition() {
        if (!_selectedPortalID || !_editMode) return;

        var portal = _portals.find(function (p) { return p.id === _selectedPortalID; });
        if (!portal) return;

        var mesh = _transformControls.object;
        if (!mesh) return;

        // Calculate updated portal data from mesh
        var updatedData = calculatePortalDataFromMesh(mesh, portal.name, portal.zoneA, portal.zoneB,
            parseFloat(document.getElementById('portal-width').value) || portal.width,
            parseFloat(document.getElementById('portal-height').value) || portal.height);

        // Update portal via REST API
        fetch('/api/portals/' + encodeURIComponent(_selectedPortalID), {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(updatedData)
        }).then(function (resp) {
            if (resp.ok) {
                console.log('[PortalEditor] Portal updated:', _selectedPortalID);
            } else {
                throw new Error('Failed to update portal');
            }
        }).catch(function (e) {
            console.error('[PortalEditor] Update portal failed:', e);
        });
    }

    function deleteSelectedPortal() {
        if (!_selectedPortalID || !_editMode) return;

        if (!confirm('Delete this portal?')) return;

        fetch('/api/portals/' + encodeURIComponent(_selectedPortalID), {
            method: 'DELETE'
        }).then(function (resp) {
            if (resp.ok) {
                console.log('[PortalEditor] Portal deleted:', _selectedPortalID);
                deselectPortal();
            } else {
                throw new Error('Failed to delete portal');
            }
        }).catch(function (e) {
            console.error('[PortalEditor] Delete portal failed:', e);
            alert('Failed to delete portal: ' + e.message);
        });
    }

    // ── Portal data calculation ────────────────────────────────────────────────

    function calculatePortalDataFromMesh(mesh, name, zoneA, zoneB, width, height) {
        // Get portal position and orientation from mesh
        var position = mesh.position;
        var normal = new THREE.Vector3(0, 0, 1);
        normal.applyQuaternion(mesh.quaternion);

        // Calculate three points defining the portal plane
        // P1: center of bottom edge
        var p1 = new THREE.Vector3(0, -height / 2, 0);
        // P2: center of top edge
        var p2 = new THREE.Vector3(0, height / 2, 0);
        // P3: one corner of bottom edge (defines width direction)
        var p3 = new THREE.Vector3(-width / 2, -height / 2, 0);

        p1.applyQuaternion(mesh.quaternion).add(mesh.position);
        p2.applyQuaternion(mesh.quaternion).add(mesh.position);
        p3.applyQuaternion(mesh.quaternion).add(mesh.position);

        return {
            id: _editMode ? _selectedPortalID : undefined,
            name: name,
            zone_a: zoneA || '',
            zone_b: zoneB || '',
            p1_x: p1.x, p1_y: p1.y, p1_z: p1.z,
            p2_x: p2.x, p2_y: p2.y, p2_z: p2.z,
            p3_x: p3.x, p3_y: p3.y, p3_z: p3.z,
            width: width,
            height: height,
            enabled: true
        };
    }

    function createPortalMeshFromData(portal) {
        // Calculate position and quaternion from three points
        var p1 = new THREE.Vector3(portal.p1_x || 0, portal.p1_y || 0, portal.p1_z || 0);
        var p2 = new THREE.Vector3(portal.p2_x || 0, portal.p2_y || 0, portal.p2_z || 0);
        var p3 = new THREE.Vector3(portal.p3_x || 0, portal.p3_y || 0, portal.p3_z || 0);

        // Calculate center (midpoint of p1 and p2)
        var position = new THREE.Vector3().addVectors(p1, p2).multiplyScalar(0.5);

        // Calculate normal from the plane defined by p1, p2, p3
        var v1 = new THREE.Vector3().subVectors(p2, p1);
        var v2 = new THREE.Vector3().subVectors(p3, p1);
        var normal = new THREE.Vector3().crossVectors(v1, v2).normalize();

        // Calculate quaternion from normal (default +Z facing)
        var quaternion = new THREE.Quaternion();
        var up = new THREE.Vector3(0, 0, 1);
        quaternion.setFromUnitVectors(up, normal);

        var width = portal.width || DEFAULT_WIDTH;
        var height = portal.height || DEFAULT_HEIGHT;

        return createPortalMesh(width, height, position, quaternion);
    }

    function getPortalMesh(portalID) {
        for (var i = 0; i < _scene.children.length; i++) {
            var obj = _scene.children[i];
            if (obj.userData && obj.userData.portalID === portalID) {
                return obj;
            }
        }
        return null;
    }

    // ── Portal editor panel ────────────────────────────────────────────────────

    function showNewPortalPanel() {
        var panel = document.getElementById('portal-editor-panel');
        if (!panel) return;

        panel.style.display = 'block';

        // Set default values
        document.getElementById('portal-name').value = 'New Portal';
        document.getElementById('portal-width').value = DEFAULT_WIDTH;
        document.getElementById('portal-height').value = DEFAULT_HEIGHT;

        // Populate zone dropdowns
        populateZoneDropdowns();

        // Show save button, hide update button
        document.getElementById('portal-save-btn').style.display = 'inline-block';
        document.getElementById('portal-update-btn').style.display = 'none';
        document.getElementById('portal-delete-btn').style.display = 'none';

        // Set panel title
        document.querySelector('#portal-editor-panel h3').textContent = 'New Portal';
    }

    function showPortalEditorPanel(portal) {
        var panel = document.getElementById('portal-editor-panel');
        if (!panel) return;

        panel.style.display = 'block';

        // Populate fields with portal data
        document.getElementById('portal-name').value = portal.name || '';
        document.getElementById('portal-width').value = portal.width || DEFAULT_WIDTH;
        document.getElementById('portal-height').value = portal.height || DEFAULT_HEIGHT;

        // Populate zone dropdowns
        populateZoneDropdowns();
        document.getElementById('portal-zone-a').value = portal.zoneA || '';
        document.getElementById('portal-zone-b').value = portal.zoneB || '';

        // Show update/delete buttons, hide save button
        document.getElementById('portal-save-btn').style.display = 'none';
        document.getElementById('portal-update-btn').style.display = 'inline-block';
        document.getElementById('portal-delete-btn').style.display = 'inline-block';

        // Set panel title
        document.querySelector('#portal-editor-panel h3').textContent = 'Edit Portal';
    }

    function populateZoneDropdowns() {
        var zoneASelect = document.getElementById('portal-zone-a');
        var zoneBSelect = document.getElementById('portal-zone-b');

        if (!zoneASelect || !zoneBSelect) return;

        // Clear existing options
        zoneASelect.innerHTML = '<option value="">-- Select Zone --</option>';
        zoneBSelect.innerHTML = '<option value="">-- Select Zone --</option>';

        // Add zone options
        _zones.forEach(function (zone) {
            var optionA = new Option(zone.name || zone.id, zone.id);
            var optionB = new Option(zone.name || zone.id, zone.id);
            zoneASelect.add(optionA);
            zoneBSelect.add(optionB);
        });
    }

    function updatePortalPanelFromMesh(mesh) {
        // Update position display
        var posDisplay = document.getElementById('portal-position-display');
        if (posDisplay) {
            posDisplay.textContent = 'X: ' + mesh.position.x.toFixed(2) +
                ' Y: ' + mesh.position.y.toFixed(2) +
                ' Z: ' + mesh.position.z.toFixed(2);
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

        // Check for portal mesh intersections
        var portalMeshes = [];
        for (var i = 0; i < _portals.length; i++) {
            var mesh = getPortalMesh(_portals[i].id);
            if (mesh) portalMeshes.push(mesh);
        }

        var intersects = raycaster.intersectObjects(portalMeshes);
        if (intersects.length > 0) {
            var mesh = intersects[0].object;
            if (mesh.userData && mesh.userData.portalID) {
                selectPortal(mesh.userData.portalID);
                return;
            }
        }

        // Deselect if clicked elsewhere
        deselectPortal();
    }

    // ── Keyboard shortcuts ─────────────────────────────────────────────────────

    function onKeyDown(event) {
        if (event.target.tagName === 'INPUT') return;

        if (event.key === 'Escape') {
            deselectPortal();
        }
        if (event.key === 'Delete' || event.key === 'Backspace') {
            if (_selectedPortalID && _editMode) {
                deleteSelectedPortal();
            }
        }
    }

    // ── Data from WebSocket/Viz3D ───────────────────────────────────────────────

    function handlePortalUpdate(portals) {
        _portals = portals.map(function (p) {
            return {
                id: p.id,
                name: p.name,
                zoneA: p.zone_a,
                zoneB: p.zone_b,
                p1_x: p.p1_x, p1_y: p.p1_y, p1_z: p.p1_z,
                p2_x: p.p2_x, p2_y: p.p2_y, p2_z: p.p2_z,
                p3_x: p.p3_x, p3_y: p.p3_y, p3_z: p.p3_z,
                width: p.width,
                height: p.height,
                enabled: p.enabled
            };
        });
    }

    function handleZoneUpdate(zones) {
        _zones = zones.map(function (z) {
            return {
                id: z.id,
                name: z.name,
                minX: z.x, minY: z.y, minZ: z.z,
                maxX: z.x + z.w, maxY: z.y + z.h, maxZ: z.z + z.d
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

        console.log('[PortalEditor] initialized');
    }

    // ── Tick (called from animation loop) ───────────────────────────────────────

    function update() {
        // Deselect if selected portal was deleted
        if (_selectedPortalID && _editMode) {
            var portal = _portals.find(function (p) { return p.id === _selectedPortalID; });
            if (!portal) {
                deselectPortal();
            }
        }
    }

    // ── Public API ────────────────────────────────────────────────────────────

    return {
        init: init,
        update: update,
        handlePortalUpdate: handlePortalUpdate,
        handleZoneUpdate: handleZoneUpdate,
        startNewPortal: startNewPortal,
        saveNewPortal: saveNewPortal,
        deleteSelectedPortal: deleteSelectedPortal,
        selectPortal: selectPortal,
        deselectPortal: deselectPortal,
        togglePanel: function () {
            var panel = document.getElementById('portal-editor-panel');
            if (!panel) return;
            var visible = panel.style.display !== 'none';
            panel.style.display = visible ? 'none' : 'block';
            var btn = document.getElementById('portal-editor-btn');
            if (btn) btn.classList.toggle('active', !visible);
        }
    };
})();
