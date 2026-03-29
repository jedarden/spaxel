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
    let _scene, _camera, _controls, _clock;
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

    // Ghost node for repositioning advice
    let _ghostNode     = null;  // THREE.Mesh (translucent)
    let _ghostLine     = null;  // THREE.Line (dashed, from original to ghost)
    let _ghostNodeMAC  = null;  // MAC of the node being moved

    const BLOB_COLORS  = [0xef5350, 0x66bb6a, 0x42a5f5, 0xffa726, 0xab47bc, 0x26c6da];
    const TRAIL_COLORS = [0xff8a80, 0xa5d6a7, 0x90caf9, 0xffcc80, 0xce93d8, 0x80deea];

    // ── init / tick ───────────────────────────────────────────────────────────

    function init(scene, camera, controls) {
        _scene    = scene;
        _camera   = camera;
        _controls = controls;
        _clock    = new THREE.Clock();
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

    // ── node meshes ───────────────────────────────────────────────────────────

    function applyNodeRegistry(nodes) {
        const incoming = new Set(nodes.map(n => n.mac));
        _nodeMeshes.forEach((m, mac) => {
            if (!incoming.has(mac)) { _scene.remove(m); _nodeMeshes.delete(mac); }
        });
        nodes.forEach(n => {
            let m = _nodeMeshes.get(n.mac);
            if (!m) {
                const col = n.virtual ? 0x80cbc4 : 0x4fc3f7;
                m = new THREE.Mesh(
                    new THREE.OctahedronGeometry(0.12, 0),
                    new THREE.MeshPhongMaterial({ color: col, emissive: col, emissiveIntensity: 0.35, shininess: 60 })
                );
                _scene.add(m);
                _nodeMeshes.set(n.mac, m);
            }
            m.position.set(n.pos_x, n.pos_y, n.pos_z);
        });
        _rebuildLinkLines();
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

        return { group, humanoid, trail, pillar };
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
        blobs.forEach(b => {
            seen.add(b.id);
            let obj = _blobs3D.get(b.id);
            if (!obj) { obj = _createBlobObj(b.id); _blobs3D.set(b.id, obj); }

            obj.group.position.set(b.x, 0, b.z);

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

    // ── public API ────────────────────────────────────────────────────────────
    return {
        init,
        update,
        handleRegistryState,
        handleLocUpdate,
        handleLinkActive,
        handleLinkInactive,
        applyLinks,
        uploadFloorPlan,
        setViewPreset,
        getNodeMesh: function (mac) { return _nodeMeshes.get(mac); },
        rebuildLinkLines: _rebuildLinkLines,
        // Ghost node API
        setGhostNode: setGhostNode,
        clearGhostNode: clearGhostNode,
        // Link health API
        updateLinkHealth: updateLinkHealth,
        getLinkHealth: getLinkHealth,
        getAllLinkHealth: getAllLinkHealth,
    };
})();
