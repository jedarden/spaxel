/**
 * Spaxel Viz3D – Phase 3 spatial visualization
 *
 * Handles: room bounds, floor-plan texture, humanoid SkinnedMesh + AnimationMixer,
 * node meshes, link lines, blob trails, vertical pillar anchors, view presets.
 *
 * Depends on Three.js r128 being loaded before this script.
 */
const Viz3D = (function () {
    'use strict';

    // ── module state ──────────────────────────────────────────────────────────
    let _scene, _camera, _controls, _clock, _renderer;
    let _room    = null;
    let _roomObjs = { floor: null, ceiling: null, walls: [], edges: null };
    let _nodeMeshes  = new Map();  // mac   → THREE.Mesh
    let _linkLines   = new Map();  // id    → THREE.Line
    let _activeLinks = new Map();  // id    → { node_mac, peer_mac, health_score }
    let _blobs3D     = new Map();  // blobId → blobObj
    let _linkHealth  = new Map();  // id    → { score, details, last_updated }
    let _mixers      = [];
    let _floorTex    = null;
    let _followId    = null;

    // ── blob interaction state ────────────────────────────────────────────────
    let _raycaster   = new THREE.Raycaster();
    let _mouse       = new THREE.Vector2();
    let _hoveredBlob = null;
    let _feedbackTooltip = null;
    let _renderer    = null;

    // Ghost node for repositioning advice
    let _ghostNode     = null;  // THREE.Mesh (translucent)
    let _ghostLine     = null;  // THREE.Line (dashed, from original to ghost)
    let _ghostNodeMAC  = null;  // MAC of the node being moved

    // Zone and portal rendering state
    let _zoneMeshes    = new Map();  // zoneID -> { mesh, label, occupantsLabel }
    let _portalMeshes  = new Map();  // portalID -> { mesh, label, flashEndTime }
    let _zonesVisible  = true;       // Toggle state for zones layer
    let _portalsVisible = true;      // Toggle state for portals layer
    let _currentZones  = new Map();  // zoneID -> zone data
    let _currentPortals = new Map(); // portalID -> portal data

    const BLOB_COLORS  = [0xef5350, 0x66bb6a, 0x42a5f5, 0xffa726, 0xab47bc, 0x26c6da];
    const TRAIL_COLORS = [0xff8a80, 0xa5d6a7, 0x90caf9, 0xffcc80, 0xce93d8, 0x80deea];

    // ── init / tick ───────────────────────────────────────────────────────────

    function init(scene, camera, controls, renderer) {
        _scene    = scene;
        _camera   = camera;
        _controls = controls;
        _clock    = new THREE.Clock();

        // Initialize blob interaction if renderer provided
        if (renderer) {
            initBlobInteraction(renderer);
            _addBlobFeedbackStyles();
        }
    }

    function update() {
        const dt = _clock.getDelta();
        for (let i = 0; i < _mixers.length; i++) _mixers[i].update(dt);

        if (_followId !== null) {
            const b = _blobs3D.get(_followId);
            if (b) {
                const p = b.group.position;
                _camera.position.lerp(new THREE.Vector3(p.x + 1.5, 1.8, p.z + 3.5), 0.07);
                _controls.target.lerp(new THREE.Vector3(p.x, 1.3, p.z), 0.07);
                _controls.update();
            }
        }

        // Update ghost line if node moved
        _updateGhostLine();

        // Update flow arrow animation
        updateFlowAnimation(dt);

        // Update anomaly zone pulse
        updateAnomalyPulse(dt);

        // Update portal flash animations
        updatePortalFlashes(dt);
    }

    // ── room bounds ───────────────────────────────────────────────────────────

    function clearRoom() {
        const r = _roomObjs;
        if (r.floor)   _scene.remove(r.floor);
        if (r.ceiling) _scene.remove(r.ceiling);
        if (r.edges)   _scene.remove(r.edges);
        r.walls.forEach(w => _scene.remove(w));
        _roomObjs = { floor: null, ceiling: null, walls: [], edges: null };
    }

    function applyRoom(cfg) {
        clearRoom();
        _room = cfg;
        const w = cfg.width, d = cfg.depth, h = cfg.height;
        const ox = cfg.origin_x || 0, oz = cfg.origin_z || 0;
        const cx = ox + w / 2, cz = oz + d / 2;

        // floor
        const floor = new THREE.Mesh(
            new THREE.PlaneGeometry(w, d),
            new THREE.MeshLambertMaterial({ color: 0x1e2a3a, map: _floorTex, side: THREE.FrontSide })
        );
        floor.rotation.x = -Math.PI / 2;
        floor.position.set(cx, 0.001, cz);
        _scene.add(floor);
        _roomObjs.floor = floor;

        // Apply floor plan calibration if texture and calibration data exist
        if (_floorTex && _floorCalibration.metersPerPixel !== 1) {
            _applyCalibrationToFloor();
        }

        // ceiling (dim, transparent)
        const ceil = new THREE.Mesh(
            new THREE.PlaneGeometry(w, d),
            new THREE.MeshLambertMaterial({ color: 0x1a2030, transparent: true, opacity: 0.25, side: THREE.BackSide })
        );
        ceil.rotation.x = Math.PI / 2;
        ceil.position.set(cx, h, cz);
        _scene.add(ceil);
        _roomObjs.ceiling = ceil;

        // walls (semi-transparent, double-sided)
        const wallMat = new THREE.MeshLambertMaterial({ color: 0x243040, transparent: true, opacity: 0.13, side: THREE.DoubleSide });
        [
            { pw: w, ry: 0,           px: cx,    py: h / 2, pz: oz     },
            { pw: w, ry: Math.PI,     px: cx,    py: h / 2, pz: oz + d },
            { pw: d, ry: Math.PI / 2, px: ox,    py: h / 2, pz: cz     },
            { pw: d, ry:-Math.PI / 2, px: ox + w, py: h / 2, pz: cz    },
        ].forEach(({ pw, ry, px, py, pz }) => {
            const m = new THREE.Mesh(new THREE.PlaneGeometry(pw, h), wallMat);
            m.rotation.y = ry;
            m.position.set(px, py, pz);
            _scene.add(m);
            _roomObjs.walls.push(m);
        });

        // wireframe edges (floor rect + ceiling rect + 4 verticals)
        const e = [ox,oz, ox+w,oz, ox+w,oz+d, ox,oz+d, ox,oz]; // perimeter loop
        const verts = [];
        for (let i = 0; i < e.length - 1; i += 2) {
            verts.push(e[i],0,e[i+1],  e[i+2],0,e[i+3]);
            verts.push(e[i],h,e[i+1],  e[i+2],h,e[i+3]);
        }
        [[ox,oz],[ox+w,oz],[ox+w,oz+d],[ox,oz+d]].forEach(([ex, ez]) => {
            verts.push(ex,0,ez, ex,h,ez);
        });
        const edgeGeo = new THREE.BufferGeometry();
        edgeGeo.setAttribute('position', new THREE.Float32BufferAttribute(verts, 3));
        const edges = new THREE.LineSegments(edgeGeo, new THREE.LineBasicMaterial({ color: 0x556677, transparent: true, opacity: 0.55 }));
        _scene.add(edges);
        _roomObjs.edges = edges;
    }

    // ── floor plan texture ────────────────────────────────────────────────────

    function uploadFloorPlan(file) {
        const url = URL.createObjectURL(file);
        new THREE.TextureLoader().load(url, function (tex) {
            _floorTex = tex;
            if (_roomObjs.floor) {
                _roomObjs.floor.material.map = tex;
                _roomObjs.floor.material.needsUpdate = true;
            }
            URL.revokeObjectURL(url);
        });
    }

    // Calibration state for floor plan
    let _floorCalibration = {
        metersPerPixel: 1,
        rotationDeg: 0,
        ax: 0, ay: 0, bx: 0, by: 0,
        distanceM: 1
    };

    /**
     * Apply floor plan calibration to the ground plane texture.
     * @param {Object} calibration - Calibration data from API
     * @param {number} calibration.meters_per_pixel - Scale factor
     * @param {number} calibration.rotation_deg - Rotation angle in degrees
     * @param {number} calibration.cal_ax - Point A X coordinate
     * @param {number} calibration.cal_ay - Point A Y coordinate
     * @param {number} calibration.cal_bx - Point B X coordinate
     * @param {number} calibration.cal_by - Point B Y coordinate
     * @param {number} calibration.distance_m - Real-world distance in meters
     */
    function setFloorPlanCalibration(calibration) {
        if (!calibration) return;

        _floorCalibration = {
            metersPerPixel: calibration.meters_per_pixel || 1,
            rotationDeg: calibration.rotation_deg || 0,
            ax: calibration.cal_ax || 0,
            ay: calibration.cal_ay || 0,
            bx: calibration.cal_bx || 0,
            by: calibration.cal_by || 0,
            distanceM: calibration.distance_m || 1
        };

        // Apply calibration to floor texture if floor exists
        if (_roomObjs.floor && _floorTex) {
            _applyCalibrationToFloor();
        }
    }

    /**
     * Apply stored calibration to the floor mesh.
     * Uses texture transformation matrix to scale and rotate.
     */
    function _applyCalibrationToFloor() {
        if (!_roomObjs.floor || !_floorTex) return;

        const floor = _roomObjs.floor;
        const tex = _floorTex;

        // Enable texture transformation
        tex.matrixAutoUpdate = false;

        // Calculate texture scale based on room dimensions vs image dimensions
        // Default room size if not set
        const roomWidth = _room ? _room.width : 10;
        const roomDepth = _room ? _room.depth : 10;

        // Calculate how many meters the image covers at current scale
        const imageWidthMeters = tex.image.width * _floorCalibration.metersPerPixel;
        const imageHeightMeters = tex.image.height * _floorCalibration.metersPerPixel;

        // Scale texture to fit room
        const scaleX = roomWidth / imageWidthMeters;
        const scaleY = roomDepth / imageHeightMeters;

        // Build transformation matrix: center -> rotate -> scale -> center back
        tex.matrix.setUvTransform(
            0.5, 0.5,           // center
            scaleX, scaleY,     // scale
            _floorCalibration.rotationDeg * Math.PI / 180,  // rotation
            0, 0                // translation
        );

        floor.material.needsUpdate = true;
    }

    /**
     * Get current floor plan calibration state.
     * @returns {Object} Current calibration data
     */
    function getFloorPlanCalibration() {
        return _floorCalibration;
    }

    // ── node meshes ───────────────────────────────────────────────────────────

    function applyNodeRegistry(nodes) {
        const incoming = new Set(nodes.map(n => n.mac));
        _nodeMeshes.forEach((m, mac) => {
            if (!incoming.has(mac)) { _scene.remove(m); _nodeMeshes.delete(mac); }
        });
        nodes.forEach(n => {
            let m = _nodeMeshes.get(n.mac);
            if (!m) {
                // Check if this is a virtual router AP node
                const isRouterAP = n.virtual && n.node_type === 'ap';

                if (isRouterAP) {
                    // Create a router icon: box with 4 antennas
                    m = _createRouterMesh();
                } else {
                    // Standard node: Octahedron
                    const col = n.virtual ? 0x80cbc4 : 0x4fc3f7;
                    m = new THREE.Mesh(
                        new THREE.OctahedronGeometry(0.12, 0),
                        new THREE.MeshPhongMaterial({ color: col, emissive: col, emissiveIntensity: 0.35, shininess: 60 })
                    );
                }
                // Store MAC in userData for spatial quick actions raycasting
                m.userData = m.userData || {};
                m.userData.mac = n.mac;
                _scene.add(m);
                _nodeMeshes.set(n.mac, m);
            }
            m.position.set(n.pos_x, n.pos_y, n.pos_z);
        });
        _rebuildLinkLines();
    }

    /**
     * Creates a router icon mesh (box with antennas)
     * @returns {THREE.Group} Group containing router geometry
     */
    function _createRouterMesh() {
        const routerGroup = new THREE.Group();

        // Router body (horizontal box)
        const bodyGeo = new THREE.BoxGeometry(0.16, 0.04, 0.1);
        const routerMat = new THREE.MeshPhongMaterial({
            color: 0x80cbc4,  // Teal for virtual AP
            emissive: 0x80cbc4,
            emissiveIntensity: 0.3,
            shininess: 80
        });
        const body = new THREE.Mesh(bodyGeo, routerMat);
        routerGroup.add(body);

        // 4 antennas (vertical cylinders at corners)
        const antennaGeo = new THREE.CylinderGeometry(0.008, 0.008, 0.12, 8);
        const antennaMat = new THREE.MeshPhongMaterial({
            color: 0x4dd0e1,
            emissive: 0x4dd0e1,
            emissiveIntensity: 0.2
        });

        // Antenna positions (relative to body center)
        const antennaPositions = [
            [-0.06, 0.06, 0.03],
            [0.06, 0.06, 0.03],
            [-0.06, 0.06, -0.03],
            [0.06, 0.06, -0.03]
        ];

        antennaPositions.forEach(pos => {
            const antenna = new THREE.Mesh(antennaGeo, antennaMat);
            antenna.position.set(pos[0], pos[1], pos[2]);
            routerGroup.add(antenna);
        });

        // Add LED indicator (small glowing sphere on top)
        const ledGeo = new THREE.SphereGeometry(0.012, 8, 8);
        const ledMat = new THREE.MeshBasicMaterial({ color: 0x00ff00 }); // Green LED
        const led = new THREE.Mesh(ledGeo, ledMat);
        led.position.set(0, 0.03, 0);
        routerGroup.add(led);

        return routerGroup;
    }

    function applyLinks(links) {
        _activeLinks.clear();
        (links || []).forEach(l => {
            const id = l.id || `${l.node_mac}:${l.peer_mac}`;
            _activeLinks.set(id, l);
        });
        _rebuildLinkLines();
    }

    /**
     * Get health color for a link based on health score.
     * Green (#22c55e at health=1.0) → Yellow (#eab308 at health=0.5) → Red (#ef4444 at health=0)
     * @param {number} health - Health score in [0, 1]
     * @returns {number} Three.js color hex value
     */
    function _getHealthColor(health) {
        // Interpolate from red (0) through yellow (0.5) to green (1)
        var r, g, b;
        if (health < 0.5) {
            // Red to yellow
            var t = health * 2; // 0-0.5 maps to 0-1
            r = 0.94; // 0xef/255
            g = 0.31 + t * 0.61; // 0x4f → 0xeab3 (approx)
            b = 0.14 + t * 0.06;
        } else {
            // Yellow to green
            var t = (health - 0.5) * 2; // 0.5-1 maps to 0-1
            r = 0.92 - t * 0.78; // 0xeab3 → 0x22
            g = 0.69 + t * 0.09; // stays mostly yellow-green
            b = 0.08 + t * 0.28; // 0x08 → 0x5e
        }
        return (Math.round(r * 255) << 16) | (Math.round(g * 255) << 8) | Math.round(b * 255);
    }

    /**
     * Get link line thickness based on health score.
     * health > 0.7 → 2px, health 0.4-0.7 → 1px, health < 0.4 → 0.5px
     * @param {number} health - Health score in [0, 1]
     * @returns {number} Line thickness
     */
    function _getHealthThickness(health) {
        if (health > 0.7) return 2;
        if (health >= 0.4) return 1;
        return 0.5;
    }

    function _rebuildLinkLines() {
        _linkLines.forEach(l => _scene.remove(l));
        _linkLines.clear();
        _activeLinks.forEach((link, id) => {
            const a = _nodeMeshes.get(link.node_mac);
            const b = _nodeMeshes.get(link.peer_mac);
            if (!a || !b) return;

            // Get health score from stored health data or link object
            var healthData = _linkHealth.get(id);
            var healthScore = healthData ? healthData.score : (link.health_score !== undefined ? link.health_score : 0.5);
            var healthColor = _getHealthColor(healthScore);
            var thickness = _getHealthThickness(healthScore);

            // Scale opacity by health for lower health links
            var opacity = 0.3 + healthScore * 0.5;

            const geo = new THREE.BufferGeometry().setFromPoints([a.position.clone(), b.position.clone()]);
            const line = new THREE.Line(geo, new THREE.LineBasicMaterial({
                color: healthColor,
                transparent: true,
                opacity: opacity,
                linewidth: thickness // Note: linewidth > 1 only works on some platforms
            }));
            _scene.add(line);
            _linkLines.set(id, line);
        });

        // Update Fresnel zones if visible
        if (_fresnelZonesVisible) {
            rebuildActiveFresnelZones();
        }
    }

    /**
     * Update link health scores from API response.
     * @param {Array} links - Array of link objects with health_score and health_details
     */
    function updateLinkHealth(links) {
        if (!links) return;
        links.forEach(function(link) {
            var id = link.link_id || (link.node_mac + ':' + link.peer_mac);
            _linkHealth.set(id, {
                score: link.health_score !== undefined ? link.health_score : 0.5,
                details: link.health_details || {},
                last_updated: link.last_updated
            });
            // Also update _activeLinks with health score
            if (_activeLinks.has(id)) {
                var existing = _activeLinks.get(id);
                existing.health_score = link.health_score;
                existing.health_details = link.health_details;
            }
        });
        _rebuildLinkLines();
    }

    /**
     * Get health data for a specific link.
     * @param {string} linkID - Link identifier
     * @returns {Object|null} Health data object or null
     */
    function getLinkHealth(linkID) {
        return _linkHealth.get(linkID) || null;
    }

    /**
     * Get all current health scores.
     * @returns {Map} Map of linkID → health data
     */
    function getAllLinkHealth() {
        return new Map(_linkHealth);
    }

    // ── humanoid SkinnedMesh factory ──────────────────────────────────────────
    //
    // Bone index constants
    const BI = { ROOT:0, PELVIS:1, SPINE:2, CHEST:3, HEAD:4,
                 LS:5, LE:6, RS:7, RE:8,
                 LH:9, LK:10, RH:11, RK:12 };

    function _buildBones() {
        const bones = [];
        function b(name, x, y, z) {
            const bn = new THREE.Bone();
            bn.name = name;
            bn.position.set(x, y, z);
            bones.push(bn);
            return bn;
        }
        // local positions relative to parent
        const root   = b('root',       0,     0,     0);
        const pelvis = b('pelvis',      0,     0.9,   0);
        const spine  = b('spine',       0,     0.25,  0);  // world y ≈ 1.15
        const chest  = b('chest',       0,     0.25,  0);  // world y ≈ 1.4
        const head   = b('head',        0,     0.22,  0);  // world y ≈ 1.62
        const ls     = b('l_shoulder', -0.18,  0,     0);  // world (-0.18, 1.4, 0)
        const le     = b('l_elbow',    -0.25,  0,     0);  // world (-0.43, 1.4, 0)
        const rs     = b('r_shoulder',  0.18,  0,     0);
        const re     = b('r_elbow',     0.25,  0,     0);
        const lh     = b('l_hip',      -0.1,   0,     0);  // world (-0.1, 0.9, 0)
        const lk     = b('l_knee',      0,    -0.44,  0);  // world (-0.1, 0.46, 0)
        const rh     = b('r_hip',       0.1,   0,     0);
        const rk     = b('r_knee',      0,    -0.44,  0);

        root.add(pelvis);
        pelvis.add(spine);
        spine.add(chest);
        chest.add(head);
        chest.add(ls);  ls.add(le);
        chest.add(rs);  rs.add(re);
        pelvis.add(lh); lh.add(lk);
        pelvis.add(rh); rh.add(rk);

        return bones;
    }

    // Merge an array of {geo, boneIdx} into one BufferGeometry with skinning attrs.
    function _mergeWithSkin(parts) {
        let totalVerts = 0;
        const indexArrays = [];
        parts.forEach(({ geo }) => {
            if (!geo.index) geo = geo.toNonIndexed();
            totalVerts += geo.attributes.position.count;
        });

        const pos  = new Float32Array(totalVerts * 3);
        const nrm  = new Float32Array(totalVerts * 3);
        const si   = new Float32Array(totalVerts * 4); // skinIndex
        const sw   = new Float32Array(totalVerts * 4); // skinWeight
        const idxArr = [];
        let vOff = 0;

        parts.forEach(({ geo: g, boneIdx }) => {
            if (!g.index) g = g.toNonIndexed();
            const p = g.attributes.position.array;
            const n = g.attributes.normal ? g.attributes.normal.array : null;
            const cnt = g.attributes.position.count;

            for (let i = 0; i < cnt; i++) {
                pos[(vOff+i)*3+0] = p[i*3+0];
                pos[(vOff+i)*3+1] = p[i*3+1];
                pos[(vOff+i)*3+2] = p[i*3+2];
                if (n) { nrm[(vOff+i)*3+0]=n[i*3+0]; nrm[(vOff+i)*3+1]=n[i*3+1]; nrm[(vOff+i)*3+2]=n[i*3+2]; }
                si[(vOff+i)*4] = boneIdx;
                sw[(vOff+i)*4] = 1.0;
            }
            if (g.index) {
                const ia = g.index.array;
                for (let i = 0; i < ia.length; i++) idxArr.push(ia[i] + vOff);
            } else {
                for (let i = 0; i < cnt; i++) idxArr.push(vOff + i);
            }
            vOff += cnt;
        });

        const merged = new THREE.BufferGeometry();
        merged.setAttribute('position',  new THREE.BufferAttribute(pos, 3));
        merged.setAttribute('normal',    new THREE.BufferAttribute(nrm, 3));
        merged.setAttribute('skinIndex', new THREE.BufferAttribute(new Uint16Array(si),  4));
        merged.setAttribute('skinWeight',new THREE.BufferAttribute(sw, 4));
        merged.setIndex(idxArr);
        return merged;
    }

    function _buildBodyGeometry() {
        const parts = [];

        function cyl(rT, rB, h, segs, boneIdx, tx, ty, tz, rx, ry, rz) {
            const g = new THREE.CylinderGeometry(rT, rB, h, segs);
            const m = new THREE.Matrix4().compose(
                new THREE.Vector3(tx, ty, tz),
                new THREE.Quaternion().setFromEuler(new THREE.Euler(rx||0, ry||0, rz||0)),
                new THREE.Vector3(1,1,1)
            );
            g.applyMatrix4(m);
            parts.push({ geo: g, boneIdx });
        }
        function sph(r, ws, hs, boneIdx, tx, ty, tz) {
            const g = new THREE.SphereGeometry(r, ws, hs);
            g.translate(tx, ty, tz);
            parts.push({ geo: g, boneIdx });
        }

        // torso (spine)
        cyl(0.13, 0.10, 0.48, 8, BI.SPINE,  0,    1.16, 0);
        // shoulder bar (chest)
        cyl(0.05, 0.05, 0.34, 6, BI.CHEST,  0,    1.40, 0,  0, 0, Math.PI/2);
        // neck + head
        cyl(0.05, 0.055,0.12, 6, BI.HEAD,   0,    1.58, 0);
        sph(0.11, 8, 6,            BI.HEAD,   0,    1.72, 0);
        // left upper arm – cylinder along –X
        cyl(0.04, 0.04, 0.22, 6, BI.LS,    -0.30, 1.40, 0,  0, 0, Math.PI/2);
        // left forearm
        cyl(0.035,0.03, 0.20, 6, BI.LE,    -0.54, 1.40, 0,  0, 0, Math.PI/2);
        // right upper arm – cylinder along +X
        cyl(0.04, 0.04, 0.22, 6, BI.RS,     0.30, 1.40, 0,  0, 0,-Math.PI/2);
        // right forearm
        cyl(0.035,0.03, 0.20, 6, BI.RE,     0.54, 1.40, 0,  0, 0,-Math.PI/2);
        // left upper leg
        cyl(0.065,0.055,0.42, 7, BI.LH,    -0.10, 0.68, 0);
        // left lower leg
        cyl(0.05, 0.04, 0.42, 7, BI.LK,    -0.10, 0.25, 0);
        // right upper leg
        cyl(0.065,0.055,0.42, 7, BI.RH,     0.10, 0.68, 0);
        // right lower leg
        cyl(0.05, 0.04, 0.42, 7, BI.RK,     0.10, 0.25, 0);

        return _mergeWithSkin(parts);
    }

    function _qFlat(euler) {
        const q = new THREE.Quaternion().setFromEuler(
            new THREE.Euler(euler[0], euler[1], euler[2])
        );
        return [q.x, q.y, q.z, q.w];
    }

    function _buildAnimClips() {
        function qt(name, times, eulerFrames) {
            const vals = [];
            eulerFrames.forEach(e => vals.push(..._qFlat(e)));
            return new THREE.QuaternionKeyframeTrack(`${name}.quaternion`, times, vals);
        }
        function staticTrack(name, euler) {
            const q = _qFlat(euler);
            return new THREE.QuaternionKeyframeTrack(`${name}.quaternion`, [0, 1], [...q, ...q]);
        }
        function identTrack(name) {
            return staticTrack(name, [0,0,0]);
        }

        // ── standing: identity pose ──
        const standTracks = ['l_hip','r_hip','l_knee','r_knee','l_shoulder','r_shoulder']
            .map(identTrack);
        const standing = new THREE.AnimationClip('standing', 1, standTracks);

        // ── walking: 1.2 s loop, 5 keyframes ──
        const wt = [0, 0.3, 0.6, 0.9, 1.2];
        function walkSwing(name, a0, a1) {
            return qt(name, wt, [
                [a0,0,0], [a1,0,0], [a0,0,0], [a1,0,0], [a0,0,0]
            ]);
        }
        const walking = new THREE.AnimationClip('walking', 1.2, [
            walkSwing('l_hip',     -0.50,  0.50),
            walkSwing('r_hip',      0.50, -0.50),
            walkSwing('l_knee',     0.00,  0.45),
            walkSwing('r_knee',     0.45,  0.00),
            walkSwing('l_shoulder', 0.28, -0.28),
            walkSwing('r_shoulder',-0.28,  0.28),
        ]);

        // ── seated: hips flexed, knees bent ──
        const seated = new THREE.AnimationClip('seated', 1, [
            staticTrack('pelvis',     [-Math.PI/2, 0, 0]),
            staticTrack('l_hip',      [ Math.PI/2, 0, 0]),
            staticTrack('r_hip',      [ Math.PI/2, 0, 0]),
            staticTrack('l_knee',     [-Math.PI/2, 0, 0]),
            staticTrack('r_knee',     [-Math.PI/2, 0, 0]),
        ]);

        // ── lying: whole figure horizontal ──
        const lying = new THREE.AnimationClip('lying', 1, [
            staticTrack('root', [-Math.PI/2, 0, 0]),
        ]);

        return { standing, walking, seated, lying };
    }

    function _buildHumanoid(color) {
        const bones = _buildBones();
        const geo   = _buildBodyGeometry();
        const mat   = new THREE.MeshPhongMaterial({
            color: color || 0x4fc3f7,
            skinning: true,
            shininess: 40,
        });

        const mesh = new THREE.SkinnedMesh(geo, mat);
        mesh.add(bones[0]);
        mesh.bind(new THREE.Skeleton(bones));

        const mixer  = new THREE.AnimationMixer(mesh);
        const clips  = _buildAnimClips();
        const actions = {};
        Object.entries(clips).forEach(([name, clip]) => {
            const a = mixer.clipAction(clip);
            a.setLoop(THREE.LoopRepeat, Infinity);
            actions[name] = a;
        });
        actions.standing.play();

        return { mesh, mixer, actions, posture: 'standing' };
    }

    function _setPosture(h, posture) {
        if (h.posture === posture) return;
        const from = h.actions[h.posture];
        const to   = h.actions[posture];
        if (from) from.fadeOut(0.35);
        if (to)   to.reset().fadeIn(0.35).play();
        h.posture = posture;
    }

    // ── blob management ───────────────────────────────────────────────────────

    function _createBlobObj(id) {
        const ci    = id % BLOB_COLORS.length;
        const color = BLOB_COLORS[ci];

        const group = new THREE.Group();
        group.userData.blobId = id;  // Store blob ID for interaction
        _scene.add(group);

        const humanoid = _buildHumanoid(color);
        group.add(humanoid.mesh);
        _mixers.push(humanoid.mixer);

        // footprint trail (max 60 pts, Y=floor)
        const trailPos = new Float32Array(60 * 3);
        const trailGeo = new THREE.BufferGeometry();
        trailGeo.setAttribute('position', new THREE.BufferAttribute(trailPos, 3));
        trailGeo.setDrawRange(0, 0);
        const trail = new THREE.Line(
            trailGeo,
            new THREE.LineBasicMaterial({ color: TRAIL_COLORS[ci % TRAIL_COLORS.length], transparent: true, opacity: 0.5 })
        );
        trail.frustumCulled = false;
        _scene.add(trail);

        // vertical pillar anchor
        const pillarGeo = new THREE.BufferGeometry();
        pillarGeo.setAttribute('position', new THREE.BufferAttribute(new Float32Array([0,0,0, 0,2.5,0]), 3));
        const pillar = new THREE.Line(
            pillarGeo,
            new THREE.LineBasicMaterial({ color: 0x445566, transparent: true, opacity: 0.3 })
        );
        _scene.add(pillar);

        return { group, humanoid, trail, pillar, blobId: id };
    }

    function _removeBlobObj(id, obj) {
        _scene.remove(obj.group);
        _scene.remove(obj.trail);
        _scene.remove(obj.pillar);
        const idx = _mixers.indexOf(obj.humanoid.mixer);
        if (idx !== -1) _mixers.splice(idx, 1);
        _blobs3D.delete(id);
        if (_followId === id) _followId = null;
    }

    function _updateTrail(obj, trailData) {
        if (!trailData || trailData.length === 0) return;
        const arr = obj.trail.geometry.attributes.position.array;
        const cnt = Math.min(trailData.length, 60);
        for (let i = 0; i < cnt; i++) {
            arr[i*3+0] = trailData[i][0];
            arr[i*3+1] = 0.02;
            arr[i*3+2] = trailData[i][1];
        }
        obj.trail.geometry.attributes.position.needsUpdate = true;
        obj.trail.geometry.setDrawRange(0, cnt);
    }

    function _updatePillar(obj, x, z, height) {
        const a = obj.pillar.geometry.attributes.position.array;
        a[0]=x; a[1]=0.05; a[2]=z;
        a[3]=x; a[4]=height-0.05; a[5]=z;
        obj.pillar.geometry.attributes.position.needsUpdate = true;
    }

    function applyLocUpdate(blobs) {
        const seen = new Set();
        const now = Date.now();
        blobs.forEach(b => {
            seen.add(b.id);
            let obj = _blobs3D.get(b.id);
            if (!obj) {
                obj = _createBlobObj(b.id);
                obj.createdAt = now;
                _blobs3D.set(b.id, obj);
            }

            obj.group.position.set(b.x, 0, b.z);
            obj.lastPosition = { x: b.x, z: b.z };
            obj.lastVelocity = { vx: b.vx || 0, vz: b.vz || 0 };

            const speed = Math.sqrt(b.vx*b.vx + b.vz*b.vz);
            _setPosture(obj.humanoid, speed > 0.25 ? 'walking' : 'standing');
            if (speed > 0.25) {
                obj.humanoid.actions.walking.timeScale = Math.min(speed * 1.8, 2.5);
                obj.group.rotation.y = Math.atan2(b.vx, b.vz);
            }

            _updateTrail(obj, b.trail);
            if (_room) _updatePillar(obj, b.x, b.z, _room.height);
        });

        _blobs3D.forEach((obj, id) => {
            if (!seen.has(id)) _removeBlobObj(id, obj);
        });
    }

    // ── identity label rendering ────────────────────────────────────────────────

    let _identityLabels = new Map();  // blobId → THREE.Sprite (text label)
    let _bleOnlyTracks  = new Map();  // personID → { group, pillar, circle }

    /**
     * Create a text sprite with the given text and color.
     * @param {string} text - Label text
     * @param {string} color - CSS color string (e.g., '#3b82f6')
     * @returns {THREE.Sprite}
     */
    function _createTextSprite(text, color) {
        var canvas = document.createElement('canvas');
        var ctx = canvas.getContext('2d');
        canvas.width = 256;
        canvas.height = 64;

        // Draw background with rounded corners
        ctx.fillStyle = 'rgba(0, 0, 0, 0.7)';
        ctx.beginPath();
        ctx.roundRect(4, 4, canvas.width - 8, canvas.height - 8, 8);
        ctx.fill();

        // Draw border in person color
        ctx.strokeStyle = color || '#4fc3f7';
        ctx.lineWidth = 3;
        ctx.beginPath();
        ctx.roundRect(4, 4, canvas.width - 8, canvas.height - 8, 8);
        ctx.stroke();

        // Draw text
        ctx.fillStyle = color || '#ffffff';
        ctx.font = 'bold 28px Arial, sans-serif';
        ctx.textAlign = 'center';
        ctx.textBaseline = 'middle';
        ctx.fillText(text, canvas.width / 2, canvas.height / 2);

        var texture = new THREE.CanvasTexture(canvas);
        texture.needsUpdate = true;

        var material = new THREE.SpriteMaterial({
            map: texture,
            transparent: true,
            depthTest: false
        });

        var sprite = new THREE.Sprite(material);
        sprite.scale.set(1.2, 0.3, 1);
        sprite.position.set(0, 2.0, 0);  // Above humanoid head

        return sprite;
    }

    /**
     * Create a BLE-only placeholder track visualization.
     * These are shown when a BLE device is heard but no CSI blob is nearby.
     * @param {Object} match - IdentityMatch with triangulation position
     * @returns {Object} Three.js objects { group, pillar, circle }
     */
    function _createBLEOnlyTrack(match) {
        var group = new THREE.Group();
        group.userData.personId = match.person_id;
        group.userData.isBLEOnly = true;

        // Dashed circle on floor to indicate BLE-only position
        var circleGeo = new THREE.RingGeometry(0.25, 0.35, 32);
        var circleMat = new THREE.MeshBasicMaterial({
            color: match.person_color ? parseInt(match.person_color.replace('#', '0x')) : 0x4fc3f7,
            transparent: true,
            opacity: 0.5,
            side: THREE.DoubleSide
        });
        var circle = new THREE.Mesh(circleGeo, circleMat);
        circle.rotation.x = -Math.PI / 2;
        circle.position.y = 0.02;
        group.add(circle);

        // Vertical dashed pillar
        var pillarGeo = new THREE.BufferGeometry();
        pillarGeo.setAttribute('position', new THREE.BufferAttribute(new Float32Array([0, 0, 0, 0, 2.0, 0]), 3));
        var pillarMat = new THREE.LineDashedMaterial({
            color: 0x888888,
            dashSize: 0.1,
            gapSize: 0.05,
            transparent: true,
            opacity: 0.4
        });
        var pillar = new THREE.Line(pillarGeo, pillarMat);
        pillar.computeLineDistances();
        group.add(pillar);

        // Position from triangulation
        var pos = match.triangulation_pos || { x: 0, y: 0, z: 0 };
        group.position.set(pos.x, 0, pos.z);

        // Add identity label
        if (match.person_name) {
            var label = _createTextSprite(match.person_name, match.person_color);
            label.position.set(0, 1.2, 0);
            group.add(label);
            group.userData.label = label;
        }

        _scene.add(group);

        return { group: group, pillar: pillar, circle: circle };
    }

    /**
     * Update identity labels on tracked blobs.
     * Called from BLEPanel when matches are updated.
     * @param {Array} matches - Array of IdentityMatch objects
     */
    function updateIdentities(matches) {
        if (!matches) matches = [];

        var matchesByBlobId = new Map();
        matches.forEach(function(m) {
            if (m.blob_id > 0) {
                matchesByBlobId.set(m.blob_id, m);
            }
        });

        // Update or create identity labels on existing blobs
        _blobs3D.forEach(function(obj, blobId) {
            var match = matchesByBlobId.get(blobId);

            // Remove existing label if any
            if (obj.identityLabel) {
                obj.group.remove(obj.identityLabel);
                obj.identityLabel = null;
            }

            if (match && match.person_name && match.confidence >= 0.6) {
                // Create new label
                var label = _createTextSprite(match.person_name, match.person_color);
                label.position.set(0, 2.0, 0);
                obj.group.add(label);
                obj.identityLabel = label;

                // Update humanoid color if available
                if (match.person_color && obj.humanoid && obj.humanoid.mesh) {
                    var color = parseInt(match.person_color.replace('#', '0x'));
                    obj.humanoid.mesh.material.color.setHex(color);
                    obj.humanoid.mesh.material.emissive.setHex(color);
                    obj.humanoid.mesh.material.emissiveIntensity = 0.15;
                }

                // Store identity info
                obj.identity = match;
            } else {
                // Reset to default color
                var ci = blobId % BLOB_COLORS.length;
                if (obj.humanoid && obj.humanoid.mesh) {
                    obj.humanoid.mesh.material.color.setHex(BLOB_COLORS[ci]);
                    obj.humanoid.mesh.material.emissive = new THREE.Color(BLOB_COLORS[ci]);
                    obj.humanoid.mesh.material.emissiveIntensity = 0;
                }
                obj.identity = null;
            }
        });

        // Handle BLE-only tracks (devices heard but no CSI blob nearby)
        var seenBLEOnly = new Set();

        matches.forEach(function(match) {
            if (match.is_ble_only && match.person_id) {
                seenBLEOnly.add(match.person_id);

                var existing = _bleOnlyTracks.get(match.person_id);
                var pos = match.triangulation_pos || { x: 0, y: 0, z: 0 };

                if (existing) {
                    // Update position
                    existing.group.position.set(pos.x, 0, pos.z);
                    existing.group.visible = true;
                } else {
                    // Create new BLE-only track
                    var track = _createBLEOnlyTrack(match);
                    _bleOnlyTracks.set(match.person_id, track);
                }
            }
        });

        // Hide BLE-only tracks not in current matches
        _bleOnlyTracks.forEach(function(track, personId) {
            if (!seenBLEOnly.has(personId)) {
                track.group.visible = false;
            }
        });
    }

    /**
     * Get identity info for a blob.
     * @param {number} blobId
     * @returns {Object|null} Identity match or null
     */
    function getBlobIdentity(blobId) {
        var obj = _blobs3D.get(blobId);
        return obj ? obj.identity : null;
    }

    /**
     * Clear all identity labels.
     */
    function clearIdentities() {
        _identityLabels.forEach(function(label) {
            if (label.parent) label.parent.remove(label);
        });
        _identityLabels.clear();

        _bleOnlyTracks.forEach(function(track) {
            _scene.remove(track.group);
        });
        _bleOnlyTracks.clear();

        _blobs3D.forEach(function(obj) {
            if (obj.identityLabel) {
                obj.group.remove(obj.identityLabel);
                obj.identityLabel = null;
            }
            obj.identity = null;
        });
    }

    // ── blob interaction (feedback buttons) ────────────────────────────────────

    /**
     * Initialize blob interaction system.
     * @param {THREE.WebGLRenderer} renderer - The Three.js renderer
     */
    function initBlobInteraction(renderer) {
        _renderer = renderer;

        // Create feedback tooltip element
        _feedbackTooltip = document.createElement('div');
        _feedbackTooltip.className = 'blob-feedback-tooltip';
        _feedbackTooltip.style.display = 'none';
        document.body.appendChild(_feedbackTooltip);

        // Add mouse move listener
        var canvas = renderer.domElement;
        canvas.addEventListener('mousemove', _onBlobMouseMove);
        canvas.addEventListener('mouseleave', _hideBlobFeedbackTooltip);
        canvas.addEventListener('contextmenu', _onBlobContextMenu);
        canvas.addEventListener('click', _onBlobClick);

        // Close context menus on click elsewhere
        document.addEventListener('click', function() {
            _hideBlobContextMenu();
            _hideNodeContextMenu();
        });
    }

    /**
     * Handle mouse move for blob hover detection.
     */
    function _onBlobMouseMove(event) {
        if (!_camera || !_scene || _blobs3D.size === 0) return;

        // Calculate mouse position in normalized device coordinates
        var rect = event.target.getBoundingClientRect();
        _mouse.x = ((event.clientX - rect.left) / rect.width) * 2 - 1;
        _mouse.y = -((event.clientY - rect.top) / rect.height) * 2 + 1;

        // Raycast to find hovered blob
        _raycaster.setFromCamera(_mouse, _camera);

        var blobMeshes = [];
        _blobs3D.forEach(function(obj) {
            if (obj.group) {
                blobMeshes.push(obj.group);
            }
        });

        var intersects = _raycaster.intersectObjects(blobMeshes, true);

        if (intersects.length > 0) {
            // Find the blob object from the intersected mesh
            var intersected = intersects[0].object;
            var blobObj = null;

            // Walk up the parent chain to find the group
            var current = intersected;
            while (current) {
                _blobs3D.forEach(function(obj, id) {
                    if (obj.group === current) {
                        blobObj = obj;
                    }
                });
                if (blobObj) break;
                current = current.parent;
            }

            if (blobObj && blobObj !== _hoveredBlob) {
                _hoveredBlob = blobObj;
                _showBlobFeedbackTooltip(event, blobObj);
            } else if (blobObj) {
                // Update tooltip position
                _updateTooltipPosition(event);
            }
        } else {
            if (_hoveredBlob) {
                _hideBlobFeedbackTooltip();
            }
        }
    }

    /**
     * Show feedback tooltip for a blob.
     */
    function _showBlobFeedbackTooltip(event, blobObj) {
        if (!_feedbackTooltip) return;

        var blobId = blobObj.blobId;
        var eventType = 'blob_detection';
        var eventTime = blobObj.createdAt || Date.now();
        var position = blobObj.lastPosition || { x: 0, z: 0 };

        _feedbackTooltip.innerHTML =
            '<div class="feedback-tooltip-content">' +
            '  <div class="feedback-tooltip-label">Track #' + blobId + '</div>' +
            '  <div class="feedback-tooltip-actions">' +
            '    <button class="feedback-btn-icon feedback-why" title="Why is this here?" ' +
            '            onclick="Viz3D.explainBlob(' + blobId + ')">&#128526;</button>' +
            '    <button class="feedback-btn-icon feedback-thumbs-up" title="Correct detection" ' +
            '            onclick="Viz3D.submitBlobFeedback(' + blobId + ', \'TRUE_POSITIVE\')">&#x1F44D;</button>' +
            '    <button class="feedback-btn-icon feedback-thumbs-down" title="Incorrect detection" ' +
            '            onclick="Viz3D.showBlobFeedbackForm(' + blobId + ')">&#x1F44E;</button>' +
            '  </div>' +
            '</div>';

        _feedbackTooltip.style.display = 'block';
        _updateTooltipPosition(event);

        // Store blob data for feedback submission
        _feedbackTooltip.dataset.blobId = blobId;
        _feedbackTooltip.dataset.eventType = eventType;
        _feedbackTooltip.dataset.eventTime = eventTime;
        _feedbackTooltip.dataset.posX = position.x;
        _feedbackTooltip.dataset.posZ = position.z;
    }

    /**
     * Update tooltip position to follow cursor.
     */
    function _updateTooltipPosition(event) {
        if (!_feedbackTooltip) return;

        var offsetX = 15;
        var offsetY = 15;

        _feedbackTooltip.style.left = (event.clientX + offsetX) + 'px';
        _feedbackTooltip.style.top = (event.clientY + offsetY) + 'px';
    }

    /**
     * Hide the blob feedback tooltip.
     */
    function _hideBlobFeedbackTooltip() {
        if (_feedbackTooltip) {
            _feedbackTooltip.style.display = 'none';
        }
        _hoveredBlob = null;
    }

    /**
     * Submit feedback for a blob detection.
     * @param {number} blobId - The blob ID
     * @param {string} feedbackType - Feedback type (TRUE_POSITIVE, FALSE_POSITIVE, etc.)
     */
    function submitBlobFeedback(blobId, feedbackType) {
        var blobObj = _blobs3D.get(blobId);
        if (!blobObj) return;

        var details = {
            position_x: blobObj.lastPosition ? blobObj.lastPosition.x : 0,
            position_z: blobObj.lastPosition ? blobObj.lastPosition.z : 0
        };

        // Use Feedback module if available
        if (window.Feedback) {
            window.Feedback.sendFeedback(
                'blob-' + blobId + '-' + (blobObj.createdAt || Date.now()),
                window.Feedback.EventTypes.BLOB_DETECTION,
                feedbackType,
                details
            );
        } else {
            // Direct API call
            fetch('/api/learning/feedback', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    event_id: 'blob-' + blobId + '-' + (blobObj.createdAt || Date.now()),
                    event_type: 'blob_detection',
                    feedback_type: feedbackType,
                    details: details
                })
            }).then(function(res) { return res.json(); })
              .then(function(result) {
                  console.log('[Viz3D] Feedback submitted:', feedbackType);
              })
              .catch(function(err) {
                  console.error('[Viz3D] Failed to submit feedback:', err);
              });
        }

        _hideBlobFeedbackTooltip();
    }

    /**
     * Show the feedback form for a blob (thumbs-down flow).
     * @param {number} blobId - The blob ID
     */
    function showBlobFeedbackForm(blobId) {
        var blobObj = _blobs3D.get(blobId);
        if (!blobObj) return;

        var details = {
            position_x: blobObj.lastPosition ? blobObj.lastPosition.x : 0,
            position_z: blobObj.lastPosition ? blobObj.lastPosition.z : 0
        };

        if (window.Feedback) {
            window.Feedback.showFeedbackPanel(
                'blob-' + blobId + '-' + (blobObj.createdAt || Date.now()),
                window.Feedback.EventTypes.BLOB_DETECTION,
                blobObj.createdAt || Date.now(),
                details
            );
        }

        _hideBlobFeedbackTooltip();
    }

    // ── Context Menu (Blobs & Nodes) ────────────────────────────────────────────

    let _blobContextMenu = null;
    let _nodeContextMenu = null;

    /**
     * Handle context menu (right-click) on blobs and nodes.
     */
    function _onBlobContextMenu(event) {
        event.preventDefault();

        if (!_camera || !_scene) return;

        // Calculate mouse position in normalized device coordinates
        var rect = event.target.getBoundingClientRect();
        _mouse.x = ((event.clientX - rect.left) / rect.width) * 2 - 1;
        _mouse.y = -((event.clientY - rect.top) / rect.height) * 2 + 1;

        _raycaster.setFromCamera(_mouse, _camera);

        // Check for blob intersection first
        if (_blobs3D.size > 0) {
            var blobMeshes = [];
            _blobs3D.forEach(function(obj) {
                if (obj.group) {
                    blobMeshes.push(obj.group);
                }
            });

            var blobIntersects = _raycaster.intersectObjects(blobMeshes, true);

            if (blobIntersects.length > 0) {
                // Find the blob object from the intersected mesh
                var intersected = blobIntersects[0].object;
                var blobObj = null;

                // Walk up the parent chain to find the group
                var current = intersected;
                while (current) {
                    _blobs3D.forEach(function(obj, id) {
                        if (obj.group === current) {
                            blobObj = obj;
                        }
                    });
                    if (blobObj) break;
                    current = current.parent;
                }

                if (blobObj) {
                    _showBlobContextMenu(event, blobObj);
                    return;
                }
            }
        }

        // Check for node intersection
        if (_nodeMeshes.size > 0) {
            var nodeMeshes = Array.from(_nodeMeshes.values());
            var nodeIntersects = _raycaster.intersectObjects(nodeMeshes, true);

            if (nodeIntersects.length > 0) {
                var intersectedNode = nodeIntersects[0].object;
                var nodeMAC = null;

                // Walk up the parent chain to find the node mesh/group
                var current = intersectedNode;
                while (current && !nodeMAC) {
                    _nodeMeshes.forEach(function(mesh, mac) {
                        if (mesh === current) {
                            nodeMAC = mac;
                        }
                    });
                    if (nodeMAC) break;
                    current = current.parent;
                }

                if (nodeMAC) {
                    _showNodeContextMenu(event, nodeMAC);
                    return;
                }
            }
        }
    }

    /**
     * Show context menu for a blob.
     */
    function _showBlobContextMenu(event, blobObj) {
        // Remove existing context menu
        _hideBlobContextMenu();

        var blobId = blobObj.blobId;

        // Create context menu element
        var menu = document.createElement('div');
        menu.className = 'blob-context-menu';
        menu.innerHTML =
            '<div class="blob-context-menu-item" onclick="Viz3D.explainBlob(' + blobId + ')">' +
            '  <span class="blob-context-menu-icon">&#128526;</span>' +
            '  Why is this here?' +
            '</div>' +
            '<div class="blob-context-menu-divider"></div>' +
            '<div class="blob-context-menu-item" onclick="Viz3D.submitBlobFeedback(' + blobId + ', \'TRUE_POSITIVE\')">' +
            '  <span class="blob-context-menu-icon">&#x1F44D;</span>' +
            '  Correct detection' +
            '</div>' +
            '<div class="blob-context-menu-item" onclick="Viz3D.showBlobFeedbackForm(' + blobId + ')">' +
            '  <span class="blob-context-menu-icon">&#x1F44E;</span>' +
            '  Incorrect detection' +
            '</div>';

        // Position menu at cursor
        menu.style.left = event.clientX + 'px';
        menu.style.top = event.clientY + 'px';

        document.body.appendChild(menu);
        _blobContextMenu = menu;

        // Prevent menu from going off screen
        var rect = menu.getBoundingClientRect();
        if (rect.right > window.innerWidth) {
            menu.style.left = (event.clientX - rect.width) + 'px';
        }
        if (rect.bottom > window.innerHeight) {
            menu.style.top = (event.clientY - rect.height) + 'px';
        }
    }

    /**
     * Hide the blob context menu.
     */
    function _hideBlobContextMenu() {
        if (_blobContextMenu) {
            document.body.removeChild(_blobContextMenu);
            _blobContextMenu = null;
        }
    }

    /**
     * Show context menu for a node.
     */
    function _showNodeContextMenu(event, nodeMAC) {
        // Remove existing context menus
        _hideBlobContextMenu();
        _hideNodeContextMenu();

        // Create context menu element
        var menu = document.createElement('div');
        menu.className = 'blob-context-menu';
        menu.innerHTML =
            '<div class="blob-context-menu-item" onclick="Viz3D.identifyNode(\'' + nodeMAC + '\')">' +
            '  <span class="blob-context-menu-icon">&#9889;</span>' +
            '  Identify (blink LED)' +
            '</div>';

        // Position menu at cursor
        menu.style.left = event.clientX + 'px';
        menu.style.top = event.clientY + 'px';

        document.body.appendChild(menu);
        _nodeContextMenu = menu;

        // Prevent menu from going off screen
        var rect = menu.getBoundingClientRect();
        if (rect.right > window.innerWidth) {
            menu.style.left = (event.clientX - rect.width) + 'px';
        }
        if (rect.bottom > window.innerHeight) {
            menu.style.top = (event.clientY - rect.height) + 'px';
        }
    }

    /**
     * Hide the node context menu.
     */
    function _hideNodeContextMenu() {
        if (_nodeContextMenu) {
            document.body.removeChild(_nodeContextMenu);
            _nodeContextMenu = null;
        }
    }

    /**
     * Handle click on blobs for explainability.
     * Opens the explainability view when a blob figure is clicked.
     */
    function _onBlobClick(event) {
        if (!_camera || !_scene) return;

        // Don't trigger if right-click (context menu)
        if (event.button === 2) return;

        // Calculate mouse position in normalized device coordinates
        var rect = event.target.getBoundingClientRect();
        _mouse.x = ((event.clientX - rect.left) / rect.width) * 2 - 1;
        _mouse.y = -((event.clientY - rect.top) / rect.height) * 2 + 1;

        // Raycast to find clicked blob
        _raycaster.setFromCamera(_mouse, _camera);

        var blobMeshes = [];
        _blobs3D.forEach(function(obj) {
            if (obj.group) {
                blobMeshes.push(obj.group);
            }
        });

        var intersects = _raycaster.intersectObjects(blobMeshes, true);

        if (intersects.length > 0) {
            // Find the blob object from the intersected mesh
            var intersected = intersects[0].object;
            var blobObj = null;

            // Walk up the parent chain to find the group
            var current = intersected;
            while (current) {
                _blobs3D.forEach(function(obj, id) {
                    if (obj.group === current) {
                        blobObj = obj;
                    }
                });
                if (blobObj) break;
                current = current.parent;
            }

            if (blobObj) {
                // Open explainability view
                explainBlob(blobObj.blobId);
            }
        }
    }

    /**
     * Identify a node by blinking its LED.
     * @param {string} mac - The MAC address of the node to identify
     */
    function identifyNode(mac) {
        _hideNodeContextMenu();

        if (window.identifyNode) {
            window.identifyNode(mac);
        } else {
            console.error('[Viz3D] identifyNode function not available');
        }
    }

    /**
     * Open explainability view for a blob.
     * @param {number} blobId - The blob ID to explain
     */
    function explainBlob(blobId) {
        _hideBlobContextMenu();

        if (window.Explainability) {
            window.Explainability.explain(blobId);
        } else {
            console.error('[Viz3D] Explainability module not loaded');
        }
    }

    /**
     * Add blob feedback tooltip styles.
     */
    function _addBlobFeedbackStyles() {
        if (document.getElementById('blob-feedback-styles')) return;

        var style = document.createElement('style');
        style.id = 'blob-feedback-styles';
        style.textContent =
            '.blob-feedback-tooltip {' +
            '  position: fixed;' +
            '  background: rgba(0, 0, 0, 0.9);' +
            '  border-radius: 6px;' +
            '  padding: 8px 12px;' +
            '  z-index: 1000;' +
            '  pointer-events: auto;' +
            '  box-shadow: 0 2px 10px rgba(0, 0, 0, 0.5);' +
            '}' +
            '.feedback-tooltip-content {' +
            '  display: flex;' +
            '  flex-direction: column;' +
            '  gap: 6px;' +
            '}' +
            '.feedback-tooltip-label {' +
            '  font-size: 11px;' +
            '  color: #888;' +
            '  text-align: center;' +
            '}' +
            '.feedback-tooltip-actions {' +
            '  display: flex;' +
            '  gap: 6px;' +
            '  justify-content: center;' +
            '}' +
            '.feedback-btn-icon {' +
            '  background: rgba(255, 255, 255, 0.1);' +
            '  border: none;' +
            '  width: 32px;' +
            '  height: 32px;' +
            '  border-radius: 50%;' +
            '  cursor: pointer;' +
            '  font-size: 16px;' +
            '  display: flex;' +
            '  align-items: center;' +
            '  justify-content: center;' +
            '  transition: background 0.2s;' +
            '}' +
            '.feedback-btn-icon:hover {' +
            '  background: rgba(255, 255, 255, 0.2);' +
            '}' +
            '.feedback-thumbs-up:hover {' +
            '  background: rgba(76, 175, 80, 0.4);' +
            '}' +
            '.feedback-thumbs-down:hover {' +
            '  background: rgba(244, 67, 54, 0.4);' +
            '}' +
            '.feedback-why {' +
            '  background: rgba(76, 195, 247, 0.3);' +
            '}' +
            '.feedback-why:hover {' +
            '  background: rgba(76, 195, 247, 0.5);' +
            '}';
        document.head.appendChild(style);
    }

    // ── message handlers ──────────────────────────────────────────────────────

    function handleRegistryState(msg) {
        applyRoom(msg.room);
        applyNodeRegistry(msg.nodes || []);
    }

    function handleLocUpdate(msg) {
        applyLocUpdate(msg.blobs || []);
    }

    function handleLinkActive(msg) {
        const id = msg.id || `${msg.node_mac}:${msg.peer_mac}`;
        _activeLinks.set(id, msg);
        // Also store health if provided
        if (msg.health_score !== undefined) {
            _linkHealth.set(id, {
                score: msg.health_score,
                details: msg.health_details || {},
                last_updated: msg.last_updated
            });
        }
        _rebuildLinkLines();
    }

    function handleLinkInactive(msg) {
        _activeLinks.delete(msg.id);
        _linkHealth.delete(msg.id);
        const line = _linkLines.get(msg.id);
        if (line) { _scene.remove(line); _linkLines.delete(msg.id); }
    }

    function handleZoneChange(msg) {
        var zone = msg.zone;
        if (!zone) return;

        if (msg.action === 'deleted') {
            var existing = _zoneMeshes.get(zone.id);
            if (existing) {
                _scene.remove(existing.mesh);
                _scene.remove(existing.label);
                _scene.remove(existing.occupantsLabel);
                existing.mesh.geometry.dispose();
                existing.mesh.material.dispose();
                _zoneMeshes.delete(zone.id);
            }
            _currentZones.delete(zone.id);
        } else {
            _currentZones.set(zone.id, zone);
            var existing = _zoneMeshes.get(zone.id);
            if (!existing) {
                var zoneMesh = _createZoneMesh(zone);
                _zoneMeshes.set(zone.id, zoneMesh);
            } else {
                // Update existing zone
                existing.occupantsLabel.visible = zone.count > 0;
                if (zone.count > 0) {
                    var peopleText = zone.people && zone.people.length > 0 ? zone.people.join(', ') : zone.count;
                    _updateTextSprite(existing.occupantsLabel, zone.name + ': ' + peopleText);
                }
            }
        }
    }

    function handlePortalChange(msg) {
        var portal = msg.portal;
        if (!portal) return;

        if (msg.action === 'deleted') {
            var existing = _portalMeshes.get(portal.id);
            if (existing) {
                _scene.remove(existing.mesh);
                _scene.remove(existing.label);
                existing.mesh.geometry.dispose();
                existing.mesh.material.dispose();
                _portalMeshes.delete(portal.id);
            }
            _currentPortals.delete(portal.id);
        } else {
            _currentPortals.set(portal.id, portal);
            var existing = _portalMeshes.get(portal.id);
            if (!existing) {
                var portalMesh = _createPortalMesh(portal);
                _portalMeshes.set(portal.id, portalMesh);
            }
        }
    }

    function handleZoneOccupancy(msg) {
        var zones = msg.zones || [];
        zones.forEach(function(zoneOcc) {
            var zoneMesh = _zoneMeshes.get(zoneOcc.id);
            if (zoneMesh && zoneOcc.count > 0) {
                zoneMesh.occupantsLabel.visible = true;
                var zone = _currentZones.get(zoneOcc.id);
                var zoneName = zone ? zone.name : zoneOcc.id;
                _updateTextSprite(zoneMesh.occupantsLabel, zoneName + ': ' + zoneOcc.count);
            }
        });
    }

    function handleZoneTransition(msg) {
        // Flash the portal to indicate crossing
        if (msg.portal_id) {
            flashPortal(msg.portal_id);
        }
    }

    // ── view presets ──────────────────────────────────────────────────────────

    function setViewPreset(preset, blobId) {
        _followId = null;
        _controls.enabled = true;

        const cx = _room ? (_room.origin_x||0) + _room.width  / 2 : 5;
        const cz = _room ? (_room.origin_z||0) + _room.depth  / 2 : 5;
        const h  = _room ? _room.height : 2.5;

        if (preset === 'topdown') {
            _camera.up.set(0, 0, -1);
            _camera.position.set(cx, Math.max(h * 4, 12), cz);
            _controls.target.set(cx, 0, cz);
            _controls.update();
        } else if (preset === 'perspective') {
            _camera.up.set(0, 1, 0);
            _camera.position.set(cx + 8, h * 2.5, cz + 8);
            _controls.target.set(cx, 0, cz);
            _controls.update();
        } else if (preset === 'follow') {
            _camera.up.set(0, 1, 0);
            _followId = (blobId !== undefined) ? blobId
                      : (_blobs3D.size > 0 ? _blobs3D.keys().next().value : null);
        }
    }

    // ── ghost node for repositioning advice ───────────────────────────────────

    /**
     * Set a ghost node at the target position, connected by a dashed line
     * to the original node's current position.
     * @param {string} nodeMAC - The MAC address of the node to move
     * @param {number} x - Target X position in meters
     * @param {number} y - Target Y position (height) in meters
     * @param {number} z - Target Z position in meters
     */
    function setGhostNode(nodeMAC, x, y, z) {
        // Clear any existing ghost
        clearGhostNode();

        var originalMesh = _nodeMeshes.get(nodeMAC);
        if (!originalMesh) {
            console.warn('[Viz3D] Cannot set ghost node: original node not found:', nodeMAC);
            return;
        }

        _ghostNodeMAC = nodeMAC;

        // Create translucent ghost node mesh
        var ghostGeo = new THREE.OctahedronGeometry(0.14, 0); // Slightly larger
        var ghostMat = new THREE.MeshPhongMaterial({
            color: 0x66bb6a,           // Green for "go here"
            emissive: 0x66bb6a,
            emissiveIntensity: 0.4,
            transparent: true,
            opacity: 0.5,
            shininess: 40
        });
        _ghostNode = new THREE.Mesh(ghostGeo, ghostMat);
        _ghostNode.position.set(x, y !== undefined ? y : 1.5, z);
        _scene.add(_ghostNode);

        // Create dashed line from original to ghost
        var origPos = originalMesh.position;
        var ghostPos = new THREE.Vector3(x, y !== undefined ? y : 1.5, z);

        var lineGeo = new THREE.BufferGeometry().setFromPoints([origPos.clone(), ghostPos]);
        var lineMat = new THREE.LineDashedMaterial({
            color: 0x66bb6a,
            dashSize: 0.1,
            gapSize: 0.05,
            transparent: true,
            opacity: 0.7
        });
        _ghostLine = new THREE.Line(lineGeo, lineMat);
        _ghostLine.computeLineDistances();
        _scene.add(_ghostLine);

        console.log('[Viz3D] Ghost node set at', x, y, z, 'for', nodeMAC);
    }

    /**
     * Clear the ghost node and dashed line.
     */
    function clearGhostNode() {
        if (_ghostNode) {
            _scene.remove(_ghostNode);
            _ghostNode.geometry.dispose();
            _ghostNode.material.dispose();
            _ghostNode = null;
        }
        if (_ghostLine) {
            _scene.remove(_ghostLine);
            _ghostLine.geometry.dispose();
            _ghostLine.material.dispose();
            _ghostLine = null;
        }
        _ghostNodeMAC = null;
    }

    /**
     * Update the ghost line if the original node has moved.
     * Called from the main update loop.
     */
    function _updateGhostLine() {
        if (!_ghostLine || !_ghostNodeMAC) return;

        var originalMesh = _nodeMeshes.get(_ghostNodeMAC);
        if (!originalMesh) return;

        var origPos = originalMesh.position;
        var ghostPos = _ghostNode.position;

        _ghostLine.geometry.setFromPoints([origPos.clone(), ghostPos]);
        _ghostLine.computeLineDistances();
    }

    // ── Flow Analytics Layers ────────────────────────────────────────────────────

    // State for analytics layers
    let _flowLayerVisible = false;
    let _dwellLayerVisible = false;
    let _corridorLayerVisible = false;
    let _flowArrows = [];          // Array of THREE.ArrowHelper
    let _dwellPlanes = [];         // Array of THREE.Mesh (heatmap cells)
    let _corridorMeshes = [];      // Array of THREE.Mesh (corridor regions)
    let _flowAnimTime = 0;
    let _flowData = null;
    let _dwellData = null;
    let _corridorData = null;
    let _flowPersonFilter = '';
    let _flowTimeFilter = '30d';  // '7d', '30d', 'all'

    /**
     * Set visibility of flow arrows layer.
     * @param {boolean} visible
     */
    function setFlowLayerVisible(visible) {
        _flowLayerVisible = visible;
        _flowArrows.forEach(function(arrow) {
            arrow.visible = visible;
        });
        if (visible && !_flowData) {
            fetchFlowData();
        }
    }

    /**
     * Set visibility of dwell heatmap layer.
     * @param {boolean} visible
     */
    function setDwellLayerVisible(visible) {
        _dwellLayerVisible = visible;
        _dwellPlanes.forEach(function(plane) {
            plane.visible = visible;
        });
        if (visible && !_dwellData) {
            fetchDwellData();
        }
    }

    /**
     * Set visibility of corridor overlay layer.
     * @param {boolean} visible
     */
    function setCorridorLayerVisible(visible) {
        _corridorLayerVisible = visible;
        _corridorMeshes.forEach(function(mesh) {
            mesh.visible = visible;
        });
        if (visible && !_corridorData) {
            fetchCorridorData();
        }
    }

    /**
     * Set person filter for flow/dwell data.
     * @param {string} personId - Empty string for all people
     */
    function setFlowPersonFilter(personId) {
        if (_flowPersonFilter !== personId) {
            _flowPersonFilter = personId;
            _flowData = null;
            _dwellData = null;
            if (_flowLayerVisible) fetchFlowData();
            if (_dwellLayerVisible) fetchDwellData();
        }
    }

    /**
     * Set time filter for flow data.
     * @param {string} timeFilter - '7d', '30d', or 'all'
     */
    function setFlowTimeFilter(timeFilter) {
        if (_flowTimeFilter !== timeFilter) {
            _flowTimeFilter = timeFilter;
            _flowData = null;
            if (_flowLayerVisible) fetchFlowData();
        }
    }

    /**
     * Fetch flow data from API and update visualization.
     */
    function fetchFlowData() {
        var since = 0;
        var now = Date.now() / 1000;
        if (_flowTimeFilter === '7d') {
            since = now - 7 * 24 * 3600;
        } else if (_flowTimeFilter === '30d') {
            since = now - 30 * 24 * 3600;
        }

        var url = '/api/analytics/flow?since=' + since + '&until=' + now;
        if (_flowPersonFilter) {
            url += '&person_id=' + encodeURIComponent(_flowPersonFilter);
        }

        fetch(url)
            .then(function(response) { return response.json(); })
            .then(function(data) {
                _flowData = data;
                rebuildFlowArrows();
            })
            .catch(function(err) {
                console.error('[Viz3D] Failed to fetch flow data:', err);
            });
    }

    /**
     * Fetch dwell heatmap data from API and update visualization.
     */
    function fetchDwellData() {
        var url = '/api/analytics/dwell';
        if (_flowPersonFilter) {
            url += '?person_id=' + encodeURIComponent(_flowPersonFilter);
        }

        fetch(url)
            .then(function(response) { return response.json(); })
            .then(function(data) {
                _dwellData = data;
                rebuildDwellPlanes();
            })
            .catch(function(err) {
                console.error('[Viz3D] Failed to fetch dwell data:', err);
            });
    }

    /**
     * Fetch corridor data from API and update visualization.
     */
    function fetchCorridorData() {
        fetch('/api/analytics/corridors')
            .then(function(response) { return response.json(); })
            .then(function(data) {
                _corridorData = data;
                rebuildCorridorMeshes();
            })
            .catch(function(err) {
                console.error('[Viz3D] Failed to fetch corridor data:', err);
            });
    }

    /**
     * Rebuild flow arrow meshes from _flowData.
     */
    function rebuildFlowArrows() {
        // Clear existing arrows
        _flowArrows.forEach(function(arrow) {
            _scene.remove(arrow);
        });
        _flowArrows = [];

        if (!_flowData || !_flowData.cells) return;

        var gridSize = _flowData.grid_size || 0.25;

        _flowData.cells.forEach(function(cell) {
            var cx = (cell.grid_x + 0.5) * gridSize;
            var cz = (cell.grid_z + 0.5) * gridSize;

            // Direction vector
            var dir = new THREE.Vector3(cell.vector_x, 0, cell.vector_z).normalize();
            var length = Math.min(Math.sqrt(cell.vector_x * cell.vector_x + cell.vector_z * cell.vector_z) * 0.5 + 0.1, 0.4);

            // Color based on segment count (blue to red)
            var intensity = Math.min(cell.segment_count / 50, 1);
            var color = new THREE.Color();
            color.setHSL(0.6 - intensity * 0.6, 0.8, 0.5); // Blue (0.6) to Red (0)

            var arrow = new THREE.ArrowHelper(
                dir,
                new THREE.Vector3(cx, 0.02, cz),
                length,
                color.getHex(),
                length * 0.3,  // headLength
                length * 0.15  // headWidth
            );
            arrow.visible = _flowLayerVisible;
            arrow.userData = { segmentCount: cell.segment_count, baseOpacity: 0.7 };

            _scene.add(arrow);
            _flowArrows.push(arrow);
        });

        console.log('[Viz3D] Built', _flowArrows.length, 'flow arrows');
    }

    /**
     * Rebuild dwell heatmap planes from _dwellData.
     */
    function rebuildDwellPlanes() {
        // Clear existing planes
        _dwellPlanes.forEach(function(plane) {
            _scene.remove(plane);
            plane.geometry.dispose();
            plane.material.dispose();
        });
        _dwellPlanes = [];

        if (!_dwellData || !_dwellData.cells) return;

        var gridSize = 0.25; // GridCellSize

        _dwellData.cells.forEach(function(cell) {
            var cx = (cell.grid_x + 0.5) * gridSize;
            var cz = (cell.grid_z + 0.5) * gridSize;

            // Color: blue (low) -> green (mid) -> red (high)
            var normalized = cell.normalized;
            var color = new THREE.Color();
            if (normalized < 0.5) {
                // Blue to green
                color.setHSL(0.55 + normalized * 0.1, 0.8, 0.4);
            } else {
                // Green to red
                color.setHSL(0.35 - (normalized - 0.5) * 0.7, 0.8, 0.45);
            }

            var geo = new THREE.PlaneGeometry(gridSize * 0.95, gridSize * 0.95);
            var mat = new THREE.MeshBasicMaterial({
                color: color.getHex(),
                transparent: true,
                opacity: 0.4,
                side: THREE.DoubleSide
            });

            var plane = new THREE.Mesh(geo, mat);
            plane.rotation.x = -Math.PI / 2;
            plane.position.set(cx, 0.015, cz);
            plane.visible = _dwellLayerVisible;
            plane.userData = { count: cell.count, normalized: normalized };

            _scene.add(plane);
            _dwellPlanes.push(plane);
        });

        console.log('[Viz3D] Built', _dwellPlanes.length, 'dwell heatmap cells');
    }

    /**
     * Rebuild corridor region meshes from _corridorData.
     */
    function rebuildCorridorMeshes() {
        // Clear existing meshes
        _corridorMeshes.forEach(function(mesh) {
            _scene.remove(mesh);
            mesh.geometry.dispose();
            mesh.material.dispose();
        });
        _corridorMeshes = [];

        if (!_corridorData || !Array.isArray(_corridorData)) return;

        _corridorData.forEach(function(corridor) {
            // Create an extruded rectangle for the corridor region
            var length = corridor.length_m;
            var width = corridor.width_m;
            var cx = corridor.centroid_x;
            var cz = corridor.centroid_z;

            // Compute rotation from dominant direction
            var angle = Math.atan2(corridor.dominant_dir_x, corridor.dominant_dir_z);

            var geo = new THREE.PlaneGeometry(length, width);
            var mat = new THREE.MeshBasicMaterial({
                color: 0x8899aa,  // Warm grey
                transparent: true,
                opacity: 0.3,
                side: THREE.DoubleSide
            });

            var mesh = new THREE.Mesh(geo, mat);
            mesh.rotation.x = -Math.PI / 2;
            mesh.rotation.z = angle;
            mesh.position.set(cx, 0.025, cz);  // Slightly raised
            mesh.visible = _corridorLayerVisible;
            mesh.userData = { corridor: corridor };

            _scene.add(mesh);
            _corridorMeshes.push(mesh);
        });

        console.log('[Viz3D] Built', _corridorMeshes.length, 'corridor regions');
    }

    /**
     * Update flow arrow animation (called from main update loop).
     * @param {number} dt - Delta time in seconds
     */
    function updateFlowAnimation(dt) {
        if (!_flowLayerVisible) return;

        _flowAnimTime += dt;
        // 2-second loop for flowing effect
        var phase = (_flowAnimTime % 2.0) / 2.0;

        _flowArrows.forEach(function(arrow, index) {
            // Stagger animation based on arrow position
            var stagger = (arrow.position.x * 0.5 + arrow.position.z * 0.3) % 1.0;
            var localPhase = (phase + stagger) % 1.0;

            // Animate opacity: 0.3 -> 1.0 -> 0.3
            var opacity = 0.3 + 0.7 * (1 - Math.abs(localPhase - 0.5) * 2);

            if (arrow.line && arrow.line.material) {
                arrow.line.material.opacity = opacity;
                arrow.line.material.transparent = true;
            }
            if (arrow.cone && arrow.cone.material) {
                arrow.cone.material.opacity = opacity;
                arrow.cone.material.transparent = true;
            }
        });
    }

    /**
     * Refresh all analytics data.
     */
    function refreshAnalyticsData() {
        if (_flowLayerVisible) fetchFlowData();
        if (_dwellLayerVisible) fetchDwellData();
        if (_corridorLayerVisible) fetchCorridorData();
    }

    /**
     * Get current analytics layer visibility state.
     */
    function getAnalyticsLayerState() {
        return {
            flow: _flowLayerVisible,
            dwell: _dwellLayerVisible,
            corridor: _corridorLayerVisible,
            personFilter: _flowPersonFilter,
            timeFilter: _flowTimeFilter
        };
    }

    // ── Anomaly Zone Pulsing ─────────────────────────────────────────────────────

    let _anomalyZones = [];          // Array of zone IDs with active anomalies
    let _anomalyMeshes = new Map();  // zoneID -> THREE.Mesh (pulsing overlay)
    let _anomalyPulseTime = 0;

    /**
     * Set which zones have active anomalies (will pulse red).
     * @param {Array} zoneIDs - Array of zone ID strings
     */
    function setAnomalyZones(zoneIDs) {
        _anomalyZones = zoneIDs || [];

        // Remove meshes for zones no longer anomalous
        _anomalyMeshes.forEach(function(mesh, zoneID) {
            if (_anomalyZones.indexOf(zoneID) === -1) {
                _scene.remove(mesh);
                mesh.geometry.dispose();
                mesh.material.dispose();
                _anomalyMeshes.delete(zoneID);
            }
        });

        // Add meshes for new anomalous zones
        _anomalyZones.forEach(function(zoneID) {
            if (!_anomalyMeshes.has(zoneID)) {
                // Create a pulsing red overlay for this zone
                // Default to center of room if zone position unknown
                var cx = _room ? (_room.origin_x || 0) + _room.width / 2 : 3;
                var cz = _room ? (_room.origin_z || 0) + _room.depth / 2 : 2.5;

                // Try to get zone-specific position from zone provider
                // For now, use a 1x1m red overlay at the zone center
                var geo = new THREE.PlaneGeometry(1.5, 1.5);
                var mat = new THREE.MeshBasicMaterial({
                    color: 0xef4444,
                    transparent: true,
                    opacity: 0.4,
                    side: THREE.DoubleSide,
                    depthWrite: false
                });

                var mesh = new THREE.Mesh(geo, mat);
                mesh.rotation.x = -Math.PI / 2;
                mesh.position.set(cx, 0.03, cz);
                mesh.userData.zoneID = zoneID;

                _scene.add(mesh);
                _anomalyMeshes.set(zoneID, mesh);
            }
        });

        console.log('[Viz3D] Anomaly zones updated:', _anomalyZones);
    }

    /**
     * Update anomaly pulse animation (called from main update loop).
     * @param {number} dt - Delta time in seconds
     */
    function updateAnomalyPulse(dt) {
        if (_anomalyMeshes.size === 0) return;

        _anomalyPulseTime += dt;
        // 1.5 second pulse cycle
        var phase = (_anomalyPulseTime % 1.5) / 1.5;
        // Opacity oscillates: 0.2 -> 0.6 -> 0.2
        var opacity = 0.2 + 0.4 * (1 - Math.abs(phase - 0.5) * 2);

        _anomalyMeshes.forEach(function(mesh) {
            mesh.material.opacity = opacity;
        });
    }

    // ── GDOP Overlay Visualization ───────────────────────────────────────────────────

    // GDOP overlay state
    let _gdopOverlayVisible = false;
    let _gdopMesh = null;          // THREE.Mesh with GDOP texture
    let _gdopTexture = null;       // THREE.DataTexture with GDOP data
    let _gdopData = null;         // Cached GDOP heatmap data
    let _gdopLegendVisible = false;
    let _gdopLegendSprites = [];  // Array of THREE.Sprite for legend

    /**
     * Set visibility of GDOP overlay layer.
     * @param {boolean} visible - Whether to show GDOP overlay
     */
    function setGDOPOverlayVisible(visible) {
        _gdopOverlayVisible = visible;

        if (_gdopMesh) {
            _gdopMesh.visible = visible;
        }
        if (_gdopLegendVisible) {
            _gdopLegendSprites.forEach(function(sprite) {
                sprite.visible = visible;
            });
        }

        if (visible && !_gdopData) {
            fetchGDOPData();
        }
    }

    /**
     * Fetch GDOP heatmap data from API.
     */
    function fetchGDOPData() {
        fetch('/api/simulator/gdop/heatmap')
            .then(function(response) { return response.json(); })
            .then(function(data) {
                _gdopData = data;
                updateGDOPOverlay(data);
            })
            .catch(function(err) {
                console.error('[Viz3D] Failed to fetch GDOP data:', err);
            });
    }

    /**
     * Update the GDOP overlay with new data.
     * @param {Object} data - GDOP computation results
     */
    function updateGDOPOverlay(data) {
        if (!data || !data.gdop_map) {
            console.warn('[Viz3D] No GDOP heatmap data in response');
            return;
        }

        var gridDimensions = data.grid_dimensions || [0, 0, 0];
        var width = gridDimensions[0];
        var depth = gridDimensions[1];
        var cellSize = 0.2; // Default cell size (not provided by this endpoint)

        // Create texture from GDOP data
        var gdopValues = new Float32Array(data.gdop_map);

        // Generate colors from GDOP values using GDOPColorMap
        var colors = new Uint8Array(width * depth * 3);
        for (var i = 0; i < gdopValues.length; i++) {
            var gdop = gdopValues[i];
            var color;
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
            colors[i * 3] = color.r;
            colors[i * 3 + 1] = color.g;
            colors[i * 3 + 2] = color.b;
        }

        // Create data texture
        if (_gdopTexture) {
            _gdopTexture.dispose();
        }

        _gdopTexture = new THREE.DataTexture(colors, width, depth, THREE.RGBFormat);
        _gdopTexture.needsUpdate = true;

        // Create or update mesh
        if (!_gdopMesh) {
            var geo = new THREE.PlaneGeometry(
                width * cellSize,
                depth * cellSize
            );
            var mat = new THREE.MeshBasicMaterial({
                map: _gdopTexture,
                transparent: true,
                opacity: 0.6,
                side: THREE.DoubleSide,
                depthWrite: false
            });
            _gdopMesh = new THREE.Mesh(geo, mat);
            _gdopMesh.rotation.x = -Math.PI / 2;
            _gdopMesh.position.set(
                (width * cellSize) / 2,
                0.01, // Slightly above floor
                (depth * cellSize) / 2
            );
            _scene.add(_gdopMesh);
            _gdopMesh.visible = _gdopOverlayVisible;
        } else {
            // Update existing mesh dimensions
            _gdopMesh.geometry.dispose();
            _gdopMesh.geometry = new THREE.PlaneGeometry(
                width * cellSize,
                depth * cellSize
            );
            _gdopMesh.position.set(
                (width * cellSize) / 2,
                0.01,
                (depth * cellSize) / 2
            );
            _gdopMesh.material.map = _gdopTexture;
        }

        // Update or create legend
        updateGDOPLegend(data.coverage_percent || data.coverage_score);

        console.log('[Viz3D] GDOP overlay updated:', (data.coverage_percent || data.coverage_score || 0).toFixed(1) + '% coverage');
    }

    /**
     * Update or create the GDOP legend.
     * @param {number} coverageScore - Coverage percentage (0-100)
     */
    function updateGDOPLegend(coverageScore) {
        // Clear existing legend sprites
        _gdopLegendSprites.forEach(function(sprite) {
            _scene.remove(sprite);
        });
        _gdopLegendSprites = [];

        if (!_gdopOverlayVisible) {
            return;
        }

        // Create legend sprites
        var legendItems = [
            { color: [34, 197, 94], label: 'Excellent', gdop: '< 2' },
            { color: [255, 193, 7], label: 'Good', gdop: '2-4' },
            { color: [255, 146, 0], label: 'Fair', gdop: '4-8' },
            { color: [220, 53, 69], label: 'Poor', gdop: '> 8' },
            { color: [80, 80, 80], label: 'None', gdop: '∞' }
        ];

        var startY = 1.5;
        var spacing = 0.15;

        legendItems.forEach(function(item, index) {
            var canvas = document.createElement('canvas');
            canvas.width = 256;
            canvas.height = 64;

            var ctx = canvas.getContext('2d');

            // Draw color box
            ctx.fillStyle = 'rgb(' + item.color.join(',') + ')';
            ctx.fillRect(10, 16, 32, 32);

            // Draw border
            ctx.strokeStyle = 'rgba(255, 255, 255, 0.5)';
            ctx.lineWidth = 2;
            ctx.strokeRect(10, 16, 32, 32);

            // Draw label
            ctx.fillStyle = '#ffffff';
            ctx.font = 'bold 24px Arial, sans-serif';
            ctx.textAlign = 'left';
            ctx.textBaseline = 'middle';
            ctx.fillText(item.label + ' (GDOP ' + item.gdop + ')', 50, 32);

            // Create texture
            var texture = new THREE.CanvasTexture(canvas);
            texture.needsUpdate = true;

            // Create sprite
            var material = new THREE.SpriteMaterial({
                map: texture,
                transparent: true,
                depthTest: false
            });

            var sprite = new THREE.Sprite(material);
            sprite.scale.set(1.5, 0.4, 1);
            sprite.position.set(
                (_room ? (_room.origin_x || 0) + _room.width + 0.5 : 6),
                startY - index * spacing,
                (_room ? (_room.origin_z || 0) + _room.depth / 2 : 2.5)
            );

            _scene.add(sprite);
            _gdopLegendSprites.push(sprite);
        });

        // Add coverage score sprite
        var scoreCanvas = document.createElement('canvas');
        scoreCanvas.width = 256;
        scoreCanvas.height = 64;

        var scoreCtx = scoreCanvas.getContext('2d');
        scoreCtx.fillStyle = '#ffffff';
        scoreCtx.font = 'bold 28px Arial, sans-serif';
        scoreCtx.textAlign = 'center';
        scoreCtx.textBaseline = 'middle';
        scoreCtx.fillText('Coverage: ' + coverageScore.toFixed(1) + '%', 128, 32);

        var scoreTexture = new THREE.CanvasTexture(scoreCanvas);
        scoreTexture.needsUpdate = true;

        var scoreSprite = new THREE.Sprite(
            new THREE.SpriteMaterial({
                map: scoreTexture,
                transparent: true,
                depthTest: false
            })
        );
        scoreSprite.scale.set(2, 0.5, 1);
        scoreSprite.position.set(
            (_room ? (_room.origin_x || 0) + _room.width + 0.5 : 6),
            startY - legendItems.length * spacing - 0.2,
            (_room ? (_room.origin_z || 0) + _room.depth / 2 : 2.5)
        );

        _scene.add(scoreSprite);
        _gdopLegendSprites.push(scoreSprite);

        _gdopLegendVisible = true;
    }

    /**
     * Clear the GDOP overlay.
     */
    function clearGDOPOverlay() {
        if (_gdopMesh) {
            _scene.remove(_gdopMesh);
            _gdopMesh.geometry.dispose();
            _gdopMesh.material.dispose();
            _gdopMesh = null;
        }
        if (_gdopTexture) {
            _gdopTexture.dispose();
            _gdopTexture = null;
        }

        _gdopData = null;
    }

    /**
     * Get current GDOP overlay state.
     * @returns {Object} State object
     */
    function getGDOPState() {
        return {
            visible: _gdopOverlayVisible,
            hasData: _gdopData !== null,
            coverageScore: _gdopData ? _gdopData.coverage_score : null
        };
    }

    /**
     * Get the Three.js scene (for external modules like simulator).
     * @returns {THREE.Scene} The scene object
     */
    function getScene() {
        return _scene;
    }

    /**
     * Focus the camera on a specific zone.
     * @param {string} zoneID - The zone ID to focus on
     */
    function focusOnZone(zoneID) {
        if (!_camera || !_controls) return;

        // Get zone position from anomaly mesh if available
        var mesh = _anomalyMeshes.get(zoneID);
        if (mesh) {
            var pos = mesh.position;
            _camera.position.set(pos.x + 2, 2.0, pos.z + 3);
            _controls.target.set(pos.x, 0.5, pos.z);
            _controls.update();
            return;
        }

        // Fallback: focus on room center
        var cx = _room ? (_room.origin_x || 0) + _room.width / 2 : 3;
        var cz = _room ? (_room.origin_z || 0) + _room.depth / 2 : 2.5;
        _camera.position.set(cx + 2, 2.0, cz + 3);
        _controls.target.set(cx, 0.5, cz);
        _controls.update();
    }

    /**
     * Focus the camera on a specific position.
     * @param {number} x - X coordinate
     * @param {number} y - Y coordinate (height)
     * @param {number} z - Z coordinate
     */
    function focusOnPosition(x, y, z) {
        if (!_camera || !_controls) return;

        _camera.position.set(x + 2, Math.max(y + 1, 2.0), z + 3);
        _controls.target.set(x, y, z);
        _controls.update();
    }

    /**
     * Fly the camera to focus on a specific node.
     * @param {string} mac - Node MAC address
     */
    function flyToNode(mac) {
        var nodeMesh = _nodeMeshes.get(mac);
        if (!nodeMesh) {
            console.warn('[Viz3D] Node mesh not found for MAC:', mac);
            return;
        }

        var pos = nodeMesh.position;
        focusOnPosition(pos.x, pos.y, pos.z);
    }

    /**
     * Clear all anomaly zone overlays.
     */
    function clearAnomalyZones() {
        _anomalyMeshes.forEach(function(mesh) {
            _scene.remove(mesh);
            mesh.geometry.dispose();
            mesh.material.dispose();
        });
        _anomalyMeshes.clear();
        _anomalyZones = [];
    }

    // ── Zone and Portal Rendering ───────────────────────────────────────────────

    /**
     * Create a zone mesh as a semi-transparent colored cuboid.
     * @param {Object} zone - Zone data with id, name, x, y, z, w, d, h, color
     * @returns {Object} { mesh, label, occupantsLabel }
     */
    function _createZoneMesh(zone) {
        var geometry = new THREE.BoxGeometry(zone.w || 4, zone.h || 2.5, zone.d || 3);
        var color = zone.color ? parseInt(zone.color.replace('#', '0x')) : 0x3b82f6;
        var material = new THREE.MeshLambertMaterial({
            color: color,
            transparent: true,
            opacity: 0.1,
            side: THREE.DoubleSide,
            depthWrite: false
        });
        var mesh = new THREE.Mesh(geometry, material);
        mesh.position.set(
            (zone.x || 0) + (zone.w || 4) / 2,
            (zone.y || 0) + (zone.h || 2.5) / 2,
            (zone.z || 0) + (zone.d || 3) / 2
        );
        mesh.userData.zoneId = zone.id;
        mesh.renderOrder = -1;  // Render before other objects
        _scene.add(mesh);

        // Create zone label (floating text at zone centroid)
        var label = _createTextSprite(zone.name || zone.id, color);
        var cx = (zone.x || 0) + (zone.w || 4) / 2;
        var cy = (zone.y || 0) + (zone.h || 2.5) / 2;
        var cz = (zone.z || 0) + (zone.d || 3) / 2;
        label.position.set(cx, cy + 0.5, cz);
        _scene.add(label);

        // Create occupants label (initially empty)
        var occupantsLabel = _createTextSprite('', color);
        occupantsLabel.position.set(cx, cy + 0.2, cz);
        occupantsLabel.visible = false;
        _scene.add(occupantsLabel);

        return { mesh: mesh, label: label, occupantsLabel: occupantsLabel };
    }

    /**
     * Create a portal mesh as a thin vertical plane.
     * @param {Object} portal - Portal data with id, name, p1_x, p1_y, p1_z, p2_x, p2_y, p2_z, width, height
     * @returns {Object} { mesh, label, flashEndTime }
     */
    function _createPortalMesh(portal) {
        var p1 = new THREE.Vector3(portal.p1_x || 0, portal.p1_y || 0, portal.p1_z || 0);
        var p2 = new THREE.Vector3(portal.p2_x || 0, portal.p2_y || 0, portal.p2_z || 0);
        var width = portal.width || 1.0;
        var height = portal.height || 2.1;

        // Calculate portal center
        var center = new THREE.Vector3().addVectors(p1, p2).multiplyScalar(0.5);

        // Create plane geometry (width x height)
        var geometry = new THREE.PlaneGeometry(width, height);
        var material = new THREE.MeshLambertMaterial({
            color: 0xa855f7,  // Purple
            transparent: true,
            opacity: 0.3,
            side: THREE.DoubleSide,
            depthWrite: false
        });
        var mesh = new THREE.Mesh(geometry, material);
        mesh.position.copy(center);

        // Calculate orientation (perpendicular to floor, facing along portal normal)
        // For a vertical plane defined by two floor points, we need the horizontal direction
        var dx = p2.x - p1.x;
        var dz = p2.z - p1.z;
        var angle = Math.atan2(dz, dx);
        mesh.rotation.y = angle + Math.PI / 2;

        mesh.userData.portalId = portal.id;
        mesh.renderOrder = -1;
        _scene.add(mesh);

        // Create portal label at top edge
        var label = _createTextSprite(portal.name || portal.id, '#a855f7');
        label.position.set(center.x, center.y + height / 2 + 0.3, center.z);
        _scene.add(label);

        return { mesh: mesh, label: label, flashEndTime: 0 };
    }

    /**
     * Update zones from the snapshot data.
     * @param {Array} zones - Array of zone objects from snapshot
     */
    function updateZones(zones) {
        if (!zones) return;

        var zoneIDs = new Set();
        zones.forEach(function(zone) {
            zoneIDs.add(zone.id);
            _currentZones.set(zone.id, zone);

            var existing = _zoneMeshes.get(zone.id);
            if (!existing) {
                // Create new zone mesh
                var zoneMesh = _createZoneMesh(zone);
                _zoneMeshes.set(zone.id, zoneMesh);
            } else {
                // Update existing zone
                existing.occupantsLabel.visible = zone.count > 0;
                if (zone.count > 0) {
                    var peopleText = zone.people && zone.people.length > 0 ? zone.people.join(', ') : zone.count;
                    _updateTextSprite(existing.occupantsLabel, zone.name + ': ' + peopleText);
                }
            }
        });

        // Remove zones that no longer exist
        _zoneMeshes.forEach(function(zoneMesh, zoneID) {
            if (!zoneIDs.has(zoneID)) {
                _scene.remove(zoneMesh.mesh);
                _scene.remove(zoneMesh.label);
                _scene.remove(zoneMesh.occupantsLabel);
                zoneMesh.mesh.geometry.dispose();
                zoneMesh.mesh.material.dispose();
                _zoneMeshes.delete(zoneID);
            }
        });
    }

    /**
     * Update portals from the snapshot data.
     * @param {Array} portals - Array of portal objects from snapshot
     */
    function updatePortals(portals) {
        if (!portals) return;

        var portalIDs = new Set();
        portals.forEach(function(portal) {
            portalIDs.add(portal.id);
            _currentPortals.set(portal.id, portal);

            var existing = _portalMeshes.get(portal.id);
            if (!existing) {
                // Create new portal mesh
                var portalMesh = _createPortalMesh(portal);
                _portalMeshes.set(portal.id, portalMesh);
            }
        });

        // Remove portals that no longer exist
        _portalMeshes.forEach(function(portalMesh, portalID) {
            if (!portalIDs.has(portalID)) {
                _scene.remove(portalMesh.mesh);
                _scene.remove(portalMesh.label);
                portalMesh.mesh.geometry.dispose();
                portalMesh.mesh.material.dispose();
                _portalMeshes.delete(portalID);
            }
        });
    }

    /**
     * Flash a portal to indicate a crossing event.
     * @param {string} portalId - The portal ID to flash
     */
    function flashPortal(portalId) {
        var portalMesh = _portalMeshes.get(portalId);
        if (!portalMesh) return;

        // Set flash end time (1 second from now)
        portalMesh.flashEndTime = Date.now() + 1000;
    }

    /**
     * Update portal flash animations.
     * Called from the main update loop.
     * @param {number} dt - Delta time in seconds
     */
    function updatePortalFlashes(dt) {
        var now = Date.now();
        _portalMeshes.forEach(function(portalMesh, portalId) {
            if (portalMesh.flashEndTime > now) {
                // Flash animation: increase opacity
                var progress = (portalMesh.flashEndTime - now) / 1000;  // 1 to 0
                portalMesh.mesh.material.opacity = 0.3 + progress * 0.7;
            } else if (portalMesh.mesh.material.opacity !== 0.3) {
                portalMesh.mesh.material.opacity = 0.3;
            }
        });
    }

    /**
     * Update the text content of a sprite label.
     * @param {THREE.Sprite} sprite - The sprite to update
     * @param {string} text - New text content
     */
    function _updateTextSprite(sprite, text) {
        if (!sprite || !sprite.material || !sprite.material.map) return;
        var canvas = sprite.material.map.image;
        var ctx = canvas.getContext('2d');
        var color = '#4fc3f7';  // Default color

        ctx.clearRect(0, 0, canvas.width, canvas.height);

        // Draw background
        ctx.fillStyle = 'rgba(0, 0, 0, 0.7)';
        ctx.beginPath();
        ctx.roundRect(4, 4, canvas.width - 8, canvas.height - 8, 8);
        ctx.fill();

        // Draw border
        ctx.strokeStyle = color;
        ctx.lineWidth = 3;
        ctx.beginPath();
        ctx.roundRect(4, 4, canvas.width - 8, canvas.height - 8, 8);
        ctx.stroke();

        // Draw text
        ctx.fillStyle = color;
        ctx.font = 'bold 28px Arial, sans-serif';
        ctx.textAlign = 'center';
        ctx.textBaseline = 'middle';
        ctx.fillText(text, canvas.width / 2, canvas.height / 2);

        sprite.material.map.needsUpdate = true;
    }

    /**
     * Toggle zones visibility.
     * @param {boolean} visible - Whether to show zones
     */
    function toggleZonesVisible(visible) {
        _zonesVisible = visible !== undefined ? visible : !_zonesVisible;
        _zoneMeshes.forEach(function(zoneMesh) {
            zoneMesh.mesh.visible = _zonesVisible;
            zoneMesh.label.visible = _zonesVisible;
            zoneMesh.occupantsLabel.visible = _zonesVisible && zoneMesh.occupantsLabel.visible;
        });
    }

    /**
     * Toggle portals visibility.
     * @param {boolean} visible - Whether to show portals
     */
    function togglePortalsVisible(visible) {
        _portalsVisible = visible !== undefined ? visible : !_portalsVisible;
        _portalMeshes.forEach(function(portalMesh) {
            portalMesh.mesh.visible = _portalsVisible;
            portalMesh.label.visible = _portalsVisible;
        });
    }

    // ── Fresnel zone ellipsoid rendering for explainability ───────────────────────

    // Configuration for Fresnel zone visualization
    const FRESNEL_CONFIG = {
        color: 0x4FC3F7,      // Blue for Fresnel zones
        opacity: 0.25,        // Opacity for Fresnel zones
        wavelength: 0.123,    // WiFi wavelength at 2.437 GHz (c/f in meters)
        maxZones: 5           // Maximum number of Fresnel zones to visualize
    };

    let _fresnelZones = [];  // Array of THREE.Mesh for explainability Fresnel zones
    let _fresnelActiveZones = [];  // Array of THREE.Line for active link Fresnel zones (wireframe)
    let _fresnelZonesVisible = false;  // Toggle state for active link Fresnel zones

    /**
     * Calculate Fresnel zone ellipsoid geometry for a link.
     * @param {THREE.Vector3} tx - Transmitter position
     * @param {THREE.Vector3} rx - Receiver position
     * @param {number} zoneNumber - Fresnel zone number (1-based)
     * @returns {Object} Ellipsoid parameters: { center, semiAxes, rotation }
     */
    function _calculateFresnelZone(tx, rx, zoneNumber) {
        // WiFi wavelength and Fresnel zone constants
        const lambda = FRESNEL_CONFIG.wavelength;  // ~0.123 m for 2.4 GHz
        const n = zoneNumber;

        // Direct distance between TX and RX
        const d = tx.distanceTo(rx);

        // Fresnel zone path difference: n * lambda / 2
        const deltaL = n * lambda / 2;

        // Ellipsoid semi-axes calculation
        // For a prolate spheroid with foci at tx and rx:
        // Semi-major axis (a) = (d + deltaL) / 2
        // Semi-minor axis (b) = sqrt(deltaL * (d + deltaL)) / 2
        const a = (d + deltaL) / 2;
        const b = Math.sqrt(deltaL * (d + deltaL)) / 2;

        // Center of ellipsoid (midpoint between TX and RX)
        const center = new THREE.Vector3().addVectors(tx, rx).multiplyScalar(0.5);

        // Rotation: align with TX-RX axis
        const direction = new THREE.Vector3().subVectors(rx, tx).normalize();
        const up = new THREE.Vector3(0, 1, 0);
        const quaternion = new THREE.Quaternion().setFromUnitVectors(up, direction);

        return {
            center: center,
            semiAxes: new THREE.Vector3(b, b, a),  // X, Y, Z semi-axes (Z is along link axis)
            rotation: quaternion,
            zoneNumber: n
        };
    }

    /**
     * Add a Fresnel zone ellipsoid to the scene.
     * Used by the explainability module to visualize contributing links.
     * @param {number} cx, cy, cz - Center position
     * @param {number} sx, sy, sz - Semi-axes
     * @param {number} color - Color hex value
     * @param {number} opacity - Material opacity
     * @returns {THREE.Mesh|null} The created mesh
     */
    function addFresnelZone(cx, cy, cz, sx, sy, sz, color, opacity) {
        if (!_scene) return null;

        // Create ellipsoid using SphereGeometry scaled to semi-axes
        // THREE.SphereGeometry(radius, widthSegments, heightSegments)
        var geometry = new THREE.SphereGeometry(1, 32, 32);
        geometry.scale(sx, sy, sz);

        var material = new THREE.MeshBasicMaterial({
            color: color || FRESNEL_CONFIG.color,
            transparent: true,
            opacity: opacity || FRESNEL_CONFIG.opacity,
            side: THREE.DoubleSide,
            depthWrite: false,
            wireframe: false
        });

        var mesh = new THREE.Mesh(geometry, material);
        mesh.position.set(cx, cy, cz);

        _scene.add(mesh);
        _fresnelZones.push(mesh);

        return mesh;
    }

    /**
     * Remove a Fresnel zone mesh from the scene.
     * @param {THREE.Mesh} mesh - The mesh to remove
     */
    function removeFresnelZone(mesh) {
        if (!mesh || !_scene) return;

        _scene.remove(mesh);
        mesh.geometry.dispose();
        mesh.material.dispose();

        var idx = _fresnelZones.indexOf(mesh);
        if (idx !== -1) {
            _fresnelZones.splice(idx, 1);
        }
    }

    /**
     * Clear all Fresnel zone meshes.
     */
    function clearFresnelZones() {
        _fresnelZones.forEach(function(mesh) {
            if (_scene) {
                _scene.remove(mesh);
            }
            mesh.geometry.dispose();
            mesh.material.dispose();
        });
        _fresnelZones = [];
    }

    // ── Active Link Fresnel Zone Visualization (Wireframe) ────────────────────────

    /**
     * Create a wireframe Fresnel zone ellipsoid for an active link.
     * @param {THREE.Vector3} tx - Transmitter position
     * @param {THREE.Vector3} rx - Receiver position
     * @param {number} zoneNumber - Fresnel zone number (1-5)
     * @param {number} color - Color hex value
     * @returns {THREE.LineSegments|null} The created wireframe mesh
     */
    function _createWireframeFresnelZone(tx, rx, zoneNumber, color) {
        if (!_scene) return null;

        // Calculate Fresnel zone geometry
        var zone = _calculateFresnelZone(tx, rx, zoneNumber);
        if (!zone) return null;

        // Create wireframe ellipsoid using TorusGeometry (thin tube)
        // Torus with tube radius ~0.005m, following the ellipsoid path
        var tubeRadius = 0.008;  // 8mm tube thickness for visibility
        var tubularSegments = 64;
        var radialSegments = 8;
        var geometry = new THREE.TorusGeometry(
            zone.semiAxes.z,  // major radius (distance from center to ellipsoid surface along Z axis)
            tubeRadius,
            tubularSegments,
            radialSegments
        );

        // Apply scaling to create ellipsoid instead of torus
        // Scale X and Y by semi-minor / semi-major ratio
        var scaleRatio = zone.semiAxes.x / zone.semiAxes.z;
        geometry.scale(scaleRatio, scaleRatio, 1.0);

        // Position and rotate
        var mesh = new THREE.Mesh(geometry);

        // Rotate to align with link direction
        mesh.position.copy(zone.center);
        mesh.quaternion.copy(zone.rotation);

        // Orient the torus: rotate 90 degrees so tube lies in correct plane
        var orientQuat = new THREE.Quaternion().setFromAxisAngle(
            new THREE.Vector3(1, 0, 0),
            Math.PI / 2
        );
        mesh.quaternion.multiply(orientQuat);

        // Create wireframe material
        var material = new THREE.LineBasicMaterial({
            color: color || FRESNEL_CONFIG.color,
            transparent: true,
            opacity: 0.4,
            depthTest: false
        });

        // Convert mesh to wireframe
        var wireframe = new THREE.LineSegments(
            new THREE.WireframeGeometry(geometry),
            material
        );
        wireframe.position.copy(mesh.position);
        wireframe.quaternion.copy(mesh.quaternion);

        // Clean up temporary mesh
        mesh.geometry.dispose();

        _scene.add(wireframe);
        return wireframe;
    }

    /**
     * Rebuild Fresnel zone visualization for all active links.
     * Creates wireframe ellipsoids for the first 3 Fresnel zones of each active link.
     */
    function rebuildActiveFresnelZones() {
        // Clear existing Fresnel zones
        clearActiveFresnelZones();

        if (!_fresnelZonesVisible) return;

        // Get active links
        _activeLinks.forEach(function(link, linkID) {
            var txMesh = _nodeMeshes.get(link.node_mac);
            var rxMesh = _nodeMeshes.get(link.peer_mac);
            if (!txMesh || !rxMesh) return;

            var tx = txMesh.position;
            var rx = rxMesh.position;

            // Determine color based on link health
            var healthData = _linkHealth.get(linkID);
            var healthScore = healthData ? healthData.score : 0.5;
            var zoneColor = _getHealthColor(healthScore);

            // Create Fresnel zones for first 3 zones
            for (var n = 1; n <= 3; n++) {
                var wireframe = _createWireframeFresnelZone(tx, rx, n, zoneColor);
                if (wireframe) {
                    _fresnelActiveZones.push(wireframe);
                    wireframe.userData = {
                        linkID: linkID,
                        zoneNumber: n
                    };
                }
            }
        });
    }

    /**
     * Clear all active Fresnel zone wireframes.
     */
    function clearActiveFresnelZones() {
        _fresnelActiveZones.forEach(function(wireframe) {
            if (_scene) {
                _scene.remove(wireframe);
            }
            wireframe.geometry.dispose();
            wireframe.material.dispose();
        });
        _fresnelActiveZones = [];
    }

    /**
     * Toggle visibility of Fresnel zone overlays for active links.
     * @param {boolean} visible - Whether to show Fresnel zones
     */
    function toggleFresnelZones(visible) {
        _fresnelZonesVisible = visible;

        if (visible) {
            rebuildActiveFresnelZones();
        } else {
            clearActiveFresnelZones();
        }
    }

    // ── WebSocket reconnect helpers ─────────────────────────────────────────

    /**
     * Clear all blob trails (called on reconnect).
     */
    function clearAllTrails() {
        _blobs3D.forEach(function (obj) {
            var arr = obj.trail.geometry.attributes.position.array;
            arr.fill(0);
            obj.trail.geometry.attributes.position.needsUpdate = true;
            obj.trail.geometry.setDrawRange(0, 0);
        });
    }

    /**
     * Extrapolate a single blob's position during disconnect.
     * @param {number} blobId
     * @param {number} x - new X position
     * @param {number} z - new Z position
     */
    function extrapolateBlobPosition(blobId, x, z) {
        var obj = _blobs3D.get(blobId);
        if (!obj) return;
        obj.group.position.set(x, 0, z);
    }

    /**
     * Get current blob states for extrapolation on disconnect.
     * Returns array of { id, x, z, vx, vz } for each tracked blob.
     * @returns {Array}
     */
    function getBlobStates() {
        var states = [];
        _blobs3D.forEach(function (obj, blobId) {
            states.push({
                id: blobId,
                x: obj.lastPosition ? obj.lastPosition.x : 0,
                z: obj.lastPosition ? obj.lastPosition.z : 0,
                vx: obj.lastVelocity ? obj.lastVelocity.vx : 0,
                vz: obj.lastVelocity ? obj.lastVelocity.vz : 0
            });
        });
        return states;
    }

    // ── Follow Camera ───────────────────────────────────────────────────────────

    /**
     * Set camera to follow a specific blob.
     * @param {number} blobId - The blob ID to follow, or null to stop following
     */
    function setFollowTarget(blobId) {
        if (blobId === null || blobId === undefined) {
            _followId = null;
            return;
        }

        // Check if blob exists
        if (!_blobs3D.has(blobId)) {
            console.warn('[Viz3D] Cannot follow blob', blobId, '- not found');
            return;
        }

        _followId = blobId;
        console.log('[Viz3D] Now following blob', blobId);

        // Show indicator
        _showFollowIndicator(blobId);
    }

    /**
     * Get the current follow target.
     * @returns {number|null} The blob ID being followed, or null
     */
    function getFollowTarget() {
        return _followId;
    }

    /**
     * Show follow mode indicator in UI.
     */
    function _showFollowIndicator(blobId) {
        // Remove existing indicator
        _removeFollowIndicator();

        // Create indicator
        const indicator = document.createElement('div');
        indicator.className = 'follow-mode-indicator';
        indicator.id = 'follow-indicator';

        // Get blob info
        const blob = _blobs3D.get(blobId);
        const personName = blob && blob.personLabel ? blob.personLabel : 'Blob #' + blobId;
        indicator.textContent = 'Following ' + personName;
        indicator.style.cursor = 'pointer';
        indicator.style.pointerEvents = 'auto';

        // Click to stop following
        indicator.addEventListener('click', function() {
            setFollowTarget(null);
            _removeFollowIndicator();
        });

        document.body.appendChild(indicator);

        // Auto-hide after 5 seconds
        setTimeout(function() {
            _removeFollowIndicator();
        }, 5000);
    }

    /**
     * Remove follow mode indicator.
     */
    function _removeFollowIndicator() {
        const indicator = document.getElementById('follow-indicator');
        if (indicator) {
            indicator.remove();
        }
    }

    // ── Node Link Highlighting ─────────────────────────────────────────────────────

    /**
     * Highlight all links connected to a specific node.
     * @param {string} mac - The node MAC address
     * @param {boolean} highlight - Whether to highlight (true) or restore (false)
     * @param {number} color - Optional color hex value (default: 0x4fc3f7)
     */
    function highlightNodeLinks(mac, highlight, color) {
        if (!_linkLines || _linkLines.size === 0) return;

        const highlightColor = color || 0x4fc3f7;

        _linkLines.forEach(function(line, linkID) {
            // Check if this link involves the specified node
            if (linkID.includes(mac)) {
                if (highlight) {
                    // Store original material state
                    if (!line.userData.originalState) {
                        line.userData.originalState = {
                            opacity: line.material.opacity,
                            transparent: line.material.transparent,
                            color: line.material.color ? line.material.color.getHex() : null
                        };

                    // Apply highlight
                    line.material.opacity = 1.0;
                    line.material.transparent = false;
                    if (line.material.color) {
                        line.material.color.setHex(highlightColor);
                    }
                    if (line.material.emissive) {
                        line.material.emissive.setHex(highlightColor);
                        line.material.emissiveIntensity = 0.5;
                    }
                    line.material.needsUpdate = true;
                } else {
                    // Restore original state
                    if (line.userData.originalState) {
                        const orig = line.userData.originalState;
                        line.material.opacity = orig.opacity;
                        line.material.transparent = orig.transparent;
                        if (line.material.color && orig.color !== null) {
                            line.material.color.setHex(orig.color);
                        }
                        if (line.material.emissive) {
                            line.material.emissiveIntensity = 0;
                        }
                        line.material.needsUpdate = true;
                        delete line.userData.originalState;
                    }
                }
            }
        });
    }

    /**
     * Clear all link highlights.
     */
    function clearLinkHighlights() {
        if (!_linkLines) return;
        _linkLines.forEach(function(line) {
            if (line.userData.originalState) {
                const orig = line.userData.originalState;
                line.material.opacity = orig.opacity;
                line.material.transparent = orig.transparent;
                if (line.material.color && orig.color !== null) {
                    line.material.color.setHex(orig.color);
                }
                if (line.material.emissive) {
                    line.material.emissiveIntensity = 0;
                }
                line.material.needsUpdate = true;
                delete line.userData.originalState;
            }
        });
    }

    // ── Public API ────────────────────────────────────────────────────────────
    return {
        init,
        update,
        handleRegistryState,
        handleLocUpdate,
        handleLinkActive,
        handleLinkInactive,
        applyLinks,
        uploadFloorPlan,
        setFloorPlanCalibration,
        getFloorPlanCalibration,
        setViewPreset,
        // WebSocket reconnect helpers
        clearAllTrails: clearAllTrails,
        extrapolateBlobPosition: extrapolateBlobPosition,
        getBlobStates: getBlobStates,
        getNodeMesh: function (mac) { return _nodeMeshes.get(mac); },
        rebuildLinkLines: _rebuildLinkLines,
        // Ghost node API
        setGhostNode: setGhostNode,
        clearGhostNode: clearGhostNode,
        // Link health API
        updateLinkHealth: updateLinkHealth,
        getLinkHealth: getLinkHealth,
        getAllLinkHealth: getAllLinkHealth,
        // Analytics layers API
        setFlowLayerVisible: setFlowLayerVisible,
        setDwellLayerVisible: setDwellLayerVisible,
        setCorridorLayerVisible: setCorridorLayerVisible,
        setFlowPersonFilter: setFlowPersonFilter,
        setFlowTimeFilter: setFlowTimeFilter,
        refreshAnalyticsData: refreshAnalyticsData,
        getAnalyticsLayerState: getAnalyticsLayerState,
        // Blob feedback API
        initBlobInteraction: initBlobInteraction,
        submitBlobFeedback: submitBlobFeedback,
        showBlobFeedbackForm: showBlobFeedbackForm,
        // Identity API
        updateIdentities: updateIdentities,
        getBlobIdentity: getBlobIdentity,
        clearIdentities: clearIdentities,
        // Anomaly zone API
        setAnomalyZones: setAnomalyZones,
        focusOnZone: focusOnZone,
        focusOnPosition: focusOnPosition,
        flyToNode: flyToNode,
        clearAnomalyZones: clearAnomalyZones,
        // Explainability support API
        forEachRoomObject: function(callback) {
            if (!_roomObjs) return;
            var room = _roomObjs;
            if (room.floor) callback(room.floor);
            if (room.ceiling) callback(room.ceiling);
            room.walls.forEach(function(w) { callback(w); });
            if (room.edges) callback(room.edges);
        },
        forEachLink: function(callback) {
            _linkLines.forEach(function(line, linkID) {
                callback(line, linkID);
            });
        },
        forEachBlob: function(callback) {
            _blobs3D.forEach(function(obj, blobID) {
                callback(obj, blobID);
            });
        },
        highlightLink: function(linkID, color, emissiveColor, emissiveIntensity) {
            var line = _linkLines.get(linkID);
            if (!line) return;
            line.material.opacity = 1.0;
            line.material.transparent = false;
            if (line.material.color) {
                line.material.color.setHex(color);
            }
            if (line.material.emissive) {
                line.material.emissive.setHex(emissiveColor);
                line.material.emissiveIntensity = emissiveIntensity;
            }
            line.material.needsUpdate = true;
        },
        restoreObjectMaterial: function(uuid, state) {
            // Search for object by UUID in room, links, and blobs
            var found = false;
            if (_roomObjs) {
                [_roomObjs.floor, _roomObjs.ceiling, _roomObjs.edges].concat(_roomObjs.walls).forEach(function(obj) {
                    if (obj && obj.uuid === uuid) {
                        if (state.opacity !== undefined) obj.material.opacity = state.opacity;
                        if (state.transparent !== undefined) obj.material.transparent = state.transparent;
                        if (obj.material.emissive && state.emissiveIntensity !== undefined) {
                            obj.material.emissiveIntensity = state.emissiveIntensity;
                        }
                        if (obj.material.emissive && state.emissiveColor) {
                            obj.material.emissive.setHex(state.emissiveColor);
                        }
                        if (obj.material.color && state.color) {
                            obj.material.color.setHex(state.color);
                        }
                        obj.material.needsUpdate = true;
                        found = true;
                    }
                });
            }
            _linkLines.forEach(function(line) {
                if (line.uuid === uuid) {
                    if (state.opacity !== undefined) line.material.opacity = state.opacity;
                    if (state.transparent !== undefined) line.material.transparent = state.transparent;
                    if (line.material.emissive && state.emissiveIntensity !== undefined) {
                        line.material.emissiveIntensity = state.emissiveIntensity;
                    }
                    if (line.material.emissive && state.emissiveColor) {
                        line.material.emissive.setHex(state.emissiveColor);
                    }
                    if (line.material.color && state.color) {
                        line.material.color.setHex(state.color);
                    }
                    line.material.needsUpdate = true;
                    found = true;
                }
            });
            _blobs3D.forEach(function(obj) {
                if (obj.group && obj.group.uuid === uuid) {
                    if (state.opacity !== undefined) obj.group.material.opacity = state.opacity;
                    if (state.transparent !== undefined) obj.group.material.transparent = state.transparent;
                    if (obj.group.material.emissive && state.emissiveIntensity !== undefined) {
                        obj.group.material.emissiveIntensity = state.emissiveIntensity;
                    }
                    if (obj.group.material.emissive && state.emissiveColor) {
                        obj.group.material.emissive.setHex(state.emissiveColor);
                    }
                    if (obj.group.material.color && state.color) {
                        obj.group.material.color.setHex(state.color);
                    }
                    obj.group.material.needsUpdate = true;
                    found = true;
                }
            });
        },
        addFresnelZone: addFresnelZone,
        removeFresnelZone: removeFresnelZone,
        clearFresnelZones: clearFresnelZones,
        toggleFresnelZones: toggleFresnelZones,
        // Blob explainability
        explainBlob: explainBlob,
        // Node identification
        identifyNode: identifyNode,
        // Replay mode support
        enterReplayMode: enterReplayMode,
        exitReplayMode: exitReplayMode,
        updateReplayBlobs: updateReplayBlobs,
        // GDOP overlay support
        setGDOPOverlayVisible: setGDOPOverlayVisible,
        clearGDOPOverlay: clearGDOPOverlay,
        getGDOPState: getGDOPState,
        // Follow camera API
        setFollowTarget: setFollowTarget,
        getFollowTarget: getFollowTarget,
        // Node link highlighting API
        highlightNodeLinks: highlightNodeLinks,
        clearLinkHighlights: clearLinkHighlights,
        // Scene and controls access
        scene: function() { return _scene; },
        camera: function() { return _camera; },
        controls: function() { return _controls; },
        renderer: function() { return _renderer; },
        blobMeshes: function() {
            const meshes = [];
            _blobs3D.forEach(function(obj) {
                meshes.push(obj.group);
            });
            return meshes;
        },
        nodeMeshes: function() { return Array.from(_nodeMeshes.values()); },
        // Zone and portal update handlers for WebSocket messages
        handleZoneUpdate: function(zones) {
            updateZones(zones);
        },
        handlePortalUpdate: function(portals) {
            updatePortals(portals);
        },
        // Zone and portal change handlers for REST API changes
        handleZoneChange: handleZoneChange,
        handlePortalChange: handlePortalChange,
        handleZoneOccupancy: handleZoneOccupancy,
        handleZoneTransition: handleZoneTransition,
        flashPortal: flashPortal,
        toggleZonesVisible: toggleZonesVisible,
        togglePortalsVisible: togglePortalsVisible,
    };
    // ── Replay Mode Support ─────────────────────────────────────────────────────
    // Store live blob states for replay mode restoration
    let _liveBlobStates = new Map();
    let _isReplayMode = false;
    /**
     * Enter replay mode: store current blob states and prepare for replay visualization
     */
    function enterReplayMode() {
        if (_isReplayMode) return;
        _isReplayMode = true;
        // Store current blob states for restoration
        _liveBlobStates.clear();
        _blobs3D.forEach(function(obj, blobId) {
            _liveBlobStates.set(blobId, {
                id: blobId,
                x: obj.lastPosition ? obj.lastPosition.x : 0,
                y: obj.lastPosition ? obj.lastPosition.y : 1.3,
                z: obj.lastPosition ? obj.lastPosition.z : 0,
                vx: obj.lastVelocity ? obj.lastVelocity.vx : 0,
                vy: obj.lastVelocity ? obj.lastVelocity.vy : 0,
                vz: obj.lastVelocity ? obj.lastVelocity.vz : 0,
                weight: obj.weight || 0.5,
                posture: obj.posture || 'unknown',
                personId: obj.personId || null,
                personLabel: obj.personLabel || null,
                personColor: obj.personColor || null,
                trail: obj.trail ? obj.trail.slice() : []
            });
        });
        console.log('[Viz3D] Replay mode entered, stored', _liveBlobStates.size, 'blob states');
    }
    /**
     * Exit replay mode: restore live blob states
     */
    function exitReplayMode() {
        if (!_isReplayMode) return;
        _isReplayMode = false;
        // Clear all replay blobs
        _blobs3D.forEach(function(obj, blobId) {
            _removeBlobObj(blobId, obj);
        });
        _blobs3D.clear();
        // Restore live blob states
        var liveBlobs = [];
        _liveBlobStates.forEach(function(state) {
            liveBlobs.push({
                id: state.id,
                x: state.x,
                y: state.y,
                z: state.z,
                vx: state.vx,
                vy: state.vy,
                vz: state.vz,
                weight: state.weight,
                posture: state.posture,
                person_id: state.personId,
                person_label: state.personLabel,
                person_color: state.personColor
            });
        });
        if (liveBlobs.length > 0) {
            applyLocUpdate(liveBlobs);
        }
        _liveBlobStates.clear();
        console.log('[Viz3D] Replay mode exited, restored', liveBlobs.length, 'blob states');
    }
    /**
     * Update blobs during replay mode
     * @param {Array} blobs - Array of blob updates from replay worker
     * @param {number} timestampMS - Replay timestamp in milliseconds
     */
    function updateReplayBlobs(blobs, timestampMS) {
        if (!_isReplayMode) {
            console.warn('[Viz3D] updateReplayBlobs called but not in replay mode');
            return;
        }
        // Clear current blobs
        _blobs3D.forEach(function(obj, blobId) {
            _removeBlobObj(blobId, obj);
        });
        _blobs3D.clear();
        // Add replay blobs
        if (blobs && blobs.length > 0) {
            var blobUpdates = blobs.map(function(b) {
                return {
                    id: b.id,
                    x: b.x,
                    y: b.y,
                    z: b.z,
                    vx: b.vx,
                    vy: b.vy,
                    vz: b.vz,
                    weight: b.weight,
                    posture: b.posture,
                    person_id: b.person_id,
                    person_label: b.person_label,
                    person_color: b.person_color,
                    trail: b.trail
                };
            });
            applyLocUpdate(blobUpdates);
        }
    }

    // ── Public API ───────────────────────────────────────────────────────────────
    return {
        // Core
        init: init,
        update: update,

        // Room
        applyRoom: applyRoom,
        clearRoom: clearRoom,

        // Nodes
        applyNodeList: applyNodeList,
        updateNodePositions: updateNodePositions,
        getNodeMeshes: function() { return _nodeMeshes; },
        nodeMeshes: function() { return _nodeMeshes; },  // alias for quick-actions

        // Links
        applyLinkList: applyLinkList,
        updateLinkHealth: updateLinkHealth,
        highlightNodeLinks: highlightNodeLinks,
        clearLinkHighlights: clearLinkHighlights,

        // Blobs
        applyLocUpdate: applyLocUpdate,
        getBlobs3D: function() { return _blobs3D; },

        // View presets
        setViewPreset: setViewPreset,
        resetView: resetView,

        // Ghost node
        setGhostNode: setGhostNode,
        clearGhostNode: clearGhostNode,

        // Replay
        loadReplaySnapshot: loadReplaySnapshot,

        // Follow camera mode
        setFollowTarget: setFollowTarget,
        getFollowTarget: getFollowTarget,

        // Spatial quick actions support
        portalMeshes: function() {
            const meshes = [];
            _portalMeshes.forEach(function(portalMesh) {
                meshes.push(portalMesh.mesh);
            });
            return meshes;
        },
        triggerMeshes: function() {
            // Get trigger volume meshes from VolumeEditor if available
            if (window.VolumeEditor && window.VolumeEditor.getVolumeMeshes) {
                return window.VolumeEditor.getVolumeMeshes();
            }
            return [];
        },

        // Direct access (for advanced integrations)
        scene: function() { return _scene; },
        camera: function() { return _camera; },
        controls: function() { return _controls; },
        followId: function() { return _followId; }
    };
})();
