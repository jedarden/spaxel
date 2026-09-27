/**
 * Spaxel Dashboard - End-to-End 3D Localization Render Test
 *
 * Bead spaxel-457ca83b. Closes the gap between the documented 2D/3D
 * localization pipeline and the accessibility-only dashboard coverage: no test
 * fed known localized coordinates through the mothership's dashboard feed and
 * verified the Three.js floor-plan renders the expected 3D person marker.
 *
 * The chain under test, top to bottom:
 *
 *   mothership  BroadcastLocUpdate -> blobJSON wire frame
 *               (shape pinned by TestDashboardWSLocUpdateShape in
 *                mothership/tests/contract/dashboard_ws_test.go: lowercase
 *                id/x/z/vx/vz/weight/trail, optional personName/assignedColor/
 *                identityResolved, no height field)
 *   -> app.js   `if (msg.blobs) Viz3D.handleLocUpdate({type:'loc_update',
 *               blobs: msg.blobs})` (app.js handleSnapshotMessage /
 *               handleIncrementalUpdate) — this test calls those same Viz3D
 *               entry points with the same frame objects
 *   -> viz3d.js applyLocUpdate: group placed at (x, 0, z), floor-anchored
 *               pillar spanning the room height, floor-pinned trail, and the
 *               identity-driven representation swap (gray generic marker ->
 *               per-person colored humanoid + floating name label).
 *
 * Coordinate convention (asserted here, not assumed): the wire carries
 * floor-plane (x, z) only; the Three.js scene is Y-up with the floor in the
 * XZ plane. The person marker therefore rests ON the floor at y = 0 while the
 * pillar rises to the registry room height, and node meshes mount at their
 * real pos_y height. Known coordinates are fed in a non-square room
 * (8 m x 6 m x 2.7 m) so a swapped x/z or a floor-plane collapse would fail.
 *
 * Determinism: jsdom + a behavioral THREE stub, not Playwright — the
 * Playwright-managed browsers do not launch on this NixOS host
 * (see playwright.nix.config.js) and the jest suite is the landed CI path.
 * The stub is behavioral where the render logic is numeric: a real
 * add/remove scene graph and Float32Array-backed buffer attributes, so
 * assertions read the exact arrays the render loop writes.
 */

// The shared ambient canvas mock (js/ambient.test.setup.js) predates the
// bf-3dip name-label path and has no measureText, which _drawBlobLabel's
// auto-fit loop requires. Wrap the prototype locally (after the shared mock
// is installed) rather than changing the mock every suite depends on.
(function () {
    var protoGetContext = HTMLCanvasElement.prototype.getContext;
    HTMLCanvasElement.prototype.getContext = function (contextType) {
        var ctx = protoGetContext.call(this, contextType);
        if (ctx && typeof ctx.measureText !== 'function') {
            ctx.measureText = function (text) {
                return { width: (text || '').length * 18 };
            };
        }
        return ctx;
    };
})();

/**
 * Minimal behavioral THREE stub. Only the surface viz3d.js touches is
 * implemented; geometry primitives carry a small dummy position attribute so
 * the humanoid body-merge loop can count vertices without real tessellation.
 */
function makeTHREE() {
    function Color(input) {
        this.css = typeof input === 'number'
            ? '#' + input.toString(16).padStart(6, '0')
            : (input || '#000000');
        this.r = 0; this.g = 0; this.b = 0;
    }
    Color.prototype.copy = function (c) {
        this.css = c.css; this.r = c.r; this.g = c.g; this.b = c.b;
        return this;
    };

    function Vector2(x, y) { this.x = x || 0; this.y = y || 0; }

    function Vector3(x, y, z) { this.x = x || 0; this.y = y || 0; this.z = z || 0; }
    Vector3.prototype.set = function (x, y, z) {
        this.x = x; this.y = y; this.z = z; return this;
    };
    Vector3.prototype.copy = function (v) { return this.set(v.x, v.y, v.z); };
    Vector3.prototype.clone = function () { return new Vector3(this.x, this.y, this.z); };
    Vector3.prototype.lerp = function (v, a) {
        return this.set(
            this.x + (v.x - this.x) * a,
            this.y + (v.y - this.y) * a,
            this.z + (v.z - this.z) * a
        );
    };
    Vector3.prototype.distanceTo = function (v) {
        var dx = this.x - v.x, dy = this.y - v.y, dz = this.z - v.z;
        return Math.sqrt(dx * dx + dy * dy + dz * dz);
    };

    function Euler(x, y, z) { this.x = x || 0; this.y = y || 0; this.z = z || 0; }

    function Quaternion() { this.x = 0; this.y = 0; this.z = 0; this.w = 1; }
    Quaternion.prototype.setFromEuler = function () { return this; };

    function Matrix4() {}
    Matrix4.prototype.compose = function () { return this; };
    Matrix4.prototype.setUvTransform = function () { return this; };

    function Clock() {}
    Clock.prototype.getDelta = function () { return 1 / 60; };

    function Raycaster() {}

    function BufferAttribute(array, itemSize) {
        this.array = array;
        this.itemSize = itemSize;
        this.count = array.length / itemSize;
        this.needsUpdate = false;
    }

    function BufferGeometry() {
        this.attributes = {};
        this.index = null;
        this.drawRange = { start: 0, count: Infinity };
    }
    BufferGeometry.prototype.setAttribute = function (name, attr) {
        this.attributes[name] = attr; return this;
    };
    BufferGeometry.prototype.setIndex = function (idx) {
        this.index = Array.isArray(idx) ? { array: idx } : idx; return this;
    };
    BufferGeometry.prototype.setDrawRange = function (start, count) {
        this.drawRange = { start: start, count: count }; return this;
    };
    BufferGeometry.prototype.setFromPoints = function (pts) {
        var arr = new Float32Array(pts.length * 3);
        for (var i = 0; i < pts.length; i++) {
            arr[i * 3] = pts[i].x; arr[i * 3 + 1] = pts[i].y; arr[i * 3 + 2] = pts[i].z;
        }
        return this.setAttribute('position', new BufferAttribute(arr, 3));
    };
    BufferGeometry.prototype.toNonIndexed = function () { return this; };
    BufferGeometry.prototype.applyMatrix4 = function () { return this; };
    BufferGeometry.prototype.translate = function () { return this; };
    BufferGeometry.prototype.dispose = function () {};

    // Primitive geometries: same base, plus a dummy position/normal pair so
    // _mergeWithSkin can read .count/.array for the humanoid body parts.
    function PrimitiveGeometry() {
        BufferGeometry.call(this);
        var dummy = new Float32Array(8 * 3);
        this.setAttribute('position', new BufferAttribute(dummy, 3));
        this.setAttribute('normal', new BufferAttribute(new Float32Array(8 * 3), 3));
    }
    PrimitiveGeometry.prototype = Object.create(BufferGeometry.prototype);

    function Object3D() {
        this.position = new Vector3();
        this.rotation = new Euler();
        this.scale = new Vector3(1, 1, 1);
        this.children = [];
        this.parent = null;
        this.userData = {};
        this.visible = true;
        this.renderOrder = 0;
        this.frustumCulled = true;
        this.name = '';
        this.type = 'Object3D';
    }
    Object3D.prototype.add = function () {
        for (var i = 0; i < arguments.length; i++) {
            var o = arguments[i];
            if (o.parent) o.parent.remove(o);
            o.parent = this;
            this.children.push(o);
        }
        return this;
    };
    Object3D.prototype.remove = function (o) {
        var i = this.children.indexOf(o);
        if (i !== -1) this.children.splice(i, 1);
        if (o && o.parent === this) o.parent = null;
        return this;
    };
    Object3D.prototype.traverse = function (fn) {
        fn(this);
        this.children.forEach(function (c) { c.traverse(fn); });
    };

    function materialCtor(params) {
        params = params || {};
        var wrap = function (v) {
            return (v instanceof Color) ? v : new Color(v);
        };
        this.color = params.color !== undefined ? wrap(params.color) : new Color(0xffffff);
        this.emissive = params.emissive !== undefined ? wrap(params.emissive) : new Color(0x000000);
        for (var k in params) {
            if (k !== 'color' && k !== 'emissive') this[k] = params[k];
        }
        this.transparent = !!params.transparent;
        this.opacity = params.opacity !== undefined ? params.opacity : 1;
        this.dispose = function () {};
    }
    function MeshPhongMaterial(p) { materialCtor.call(this, p); }
    function MeshLambertMaterial(p) { materialCtor.call(this, p); }
    function MeshBasicMaterial(p) { materialCtor.call(this, p); }
    function LineBasicMaterial(p) { materialCtor.call(this, p); }
    function LineDashedMaterial(p) { materialCtor.call(this, p); }
    function SpriteMaterial(p) { materialCtor.call(this, p); }

    function Mesh(geometry, material) {
        Object3D.call(this);
        this.geometry = geometry;
        this.material = material;
        this.type = 'Mesh';
    }
    Mesh.prototype = Object.create(Object3D.prototype);

    function SkinnedMesh(geometry, material) {
        Mesh.call(this, geometry, material);
        this.type = 'SkinnedMesh';
    }
    SkinnedMesh.prototype = Object.create(Mesh.prototype);
    SkinnedMesh.prototype.bind = function () { return this; };

    function Line(geometry, material) {
        Object3D.call(this);
        this.geometry = geometry;
        this.material = material;
        this.type = 'Line';
    }
    Line.prototype = Object.create(Object3D.prototype);

    function LineSegments(geometry, material) {
        Line.call(this, geometry, material);
        this.type = 'LineSegments';
    }
    LineSegments.prototype = Object.create(Line.prototype);

    function Group() { Object3D.call(this); this.type = 'Group'; }
    Group.prototype = Object.create(Object3D.prototype);

    function Sprite(material) {
        Object3D.call(this);
        this.material = material;
        this.type = 'Sprite';
    }
    Sprite.prototype = Object.create(Object3D.prototype);

    function Bone() { Object3D.call(this); this.type = 'Bone'; }
    Bone.prototype = Object.create(Object3D.prototype);

    function Skeleton(bones) { this.bones = bones; }

    function AnimationClip(name, duration, tracks) {
        this.name = name; this.duration = duration; this.tracks = tracks || [];
    }
    function QuaternionKeyframeTrack(name, times, values) {
        this.name = name; this.times = times; this.values = values;
    }

    function clipAction() {
        return {
            timeScale: 1,
            setLoop: function () { return this; },
            reset: function () { return this; },
            fadeIn: function () { return this; },
            fadeOut: function () { return this; },
            play: function () { return this; },
            stop: function () { return this; }
        };
    }
    function AnimationMixer() {
        this.clipAction = function () { return clipAction(); };
        this.update = function () {};
    }

    function CanvasTexture(canvas) {
        this.image = canvas;
        this.needsUpdate = false;
        this.matrix = new Matrix4();
        this.dispose = function () {};
    }
    function TextureLoader() {
        this.load = function () {};
    }

    function Scene() { Object3D.call(this); this.type = 'Scene'; }
    Scene.prototype = Object.create(Object3D.prototype);

    return {
        Color: Color,
        Vector2: Vector2,
        Vector3: Vector3,
        Euler: Euler,
        Quaternion: Quaternion,
        Matrix4: Matrix4,
        Clock: Clock,
        Raycaster: Raycaster,
        BufferAttribute: BufferAttribute,
        Float32BufferAttribute: function (array, itemSize) {
            return new BufferAttribute(
                array instanceof Float32Array ? array : new Float32Array(array), itemSize);
        },
        BufferGeometry: BufferGeometry,
        PlaneGeometry: PrimitiveGeometry,
        BoxGeometry: PrimitiveGeometry,
        SphereGeometry: PrimitiveGeometry,
        CylinderGeometry: PrimitiveGeometry,
        OctahedronGeometry: PrimitiveGeometry,
        TorusGeometry: PrimitiveGeometry,
        RingGeometry: PrimitiveGeometry,
        WireframeGeometry: PrimitiveGeometry,
        Object3D: Object3D,
        Mesh: Mesh,
        SkinnedMesh: SkinnedMesh,
        Line: Line,
        LineSegments: LineSegments,
        Group: Group,
        Sprite: Sprite,
        Bone: Bone,
        Skeleton: Skeleton,
        MeshPhongMaterial: MeshPhongMaterial,
        MeshLambertMaterial: MeshLambertMaterial,
        MeshBasicMaterial: MeshBasicMaterial,
        LineBasicMaterial: LineBasicMaterial,
        LineDashedMaterial: LineDashedMaterial,
        SpriteMaterial: SpriteMaterial,
        AnimationClip: AnimationClip,
        QuaternionKeyframeTrack: QuaternionKeyframeTrack,
        AnimationMixer: AnimationMixer,
        CanvasTexture: CanvasTexture,
        TextureLoader: TextureLoader,
        Scene: Scene,
        LoopRepeat: 'LoopRepeat',
        FrontSide: 0,
        BackSide: 1,
        DoubleSide: 2
    };
}

describe('3D localization render pipeline (spaxel-457ca83b)', function () {
    var THREE, Viz3D, scene;

    // Non-square room so a swapped x/z axis cannot pass by accident.
    var ROOM = { width: 8, depth: 6, height: 2.7, origin_x: 0, origin_z: 0 };
    var NODES = [{ mac: 'aa:aa:aa:aa:aa:01', pos_x: 1, pos_y: 2.4, pos_z: 0.5 }];

    // Known localized coordinates (metres, room frame). z = 3.75 is half the
    // room depth; a floor-plane collapse or an x/z swap moves the marker.
    var WALKER = {
        id: 7, x: 2.5, z: 3.75, vx: 0, vz: 0, weight: 0.9,
        trail: [[2.5, 3.75], [2.2, 3.5], [1.9, 3.25]],
        posture: 'standing'
    };

    function loadModules() {
        global.THREE = makeTHREE();
        THREE = global.THREE;
        require('./blob-identity.js');   // window.BlobIdentity (pure, jsdom-safe)
        require('./viz3d.js');           // window.Viz3D — fresh closure per test
        Viz3D = window.Viz3D;
    }

    function arrClose(actual, expected) {
        var a = Array.from(actual);
        expect(a.length).toBe(expected.length);
        for (var i = 0; i < expected.length; i++) {
            expect(a[i]).toBeCloseTo(expected[i], 6);
        }
    }

    // Render-object probe: Viz3D.forEachBlob is the exported window surface
    // over the internal _blobs3D map (getBlobStates is the flattened probe
    // the live-console capture harness uses).
    function blobObj(id) {
        var found = null;
        Viz3D.forEachBlob(function (obj, blobID) {
            if (blobID === id) found = obj;
        });
        return found;
    }

    function blobCount() {
        var n = 0;
        Viz3D.forEachBlob(function () { n++; });
        return n;
    }

    beforeEach(function () {
        jest.resetModules();
        loadModules();
        window.spaxelGetState = function () { return { bleDevices: [] }; };
        scene = new THREE.Scene();
        Viz3D.init(scene, null, null, null); // renderer omitted: no interaction wiring
        Viz3D.handleRegistryState({ room: ROOM, nodes: NODES });
    });

    // ------------------------------------------------------------------
    // Room / registry stage
    // ------------------------------------------------------------------
    describe('registry_state staging', function () {
        it('builds the Y-up room: floor on the XZ plane, ceiling at room height', function () {
            expect(Viz3D.getNodeMesh(NODES[0].mac)).not.toBeNull();

            var floor = scene.children.filter(function (c) { return c.type === 'Mesh'; })[0];
            expect(floor.rotation.x).toBeCloseTo(-Math.PI / 2, 6); // floor lies in XZ
            expect(floor.position.x).toBeCloseTo(ROOM.width / 2, 6);
            expect(floor.position.y).toBeCloseTo(0.001, 6); // z-fighting offset above y=0
            expect(floor.position.z).toBeCloseTo(ROOM.depth / 2, 6);

            var ceiling = scene.children.filter(function (c) { return c.type === 'Mesh'; })[1];
            expect(ceiling.position.y).toBeCloseTo(ROOM.height, 6);
        });

        it('mounts node meshes at their full 3D registry position, height included', function () {
            var node = Viz3D.getNodeMesh(NODES[0].mac);
            expect(node.position.x).toBeCloseTo(1, 6);
            expect(node.position.y).toBeCloseTo(2.4, 6); // sensor mount height
            expect(node.position.z).toBeCloseTo(0.5, 6);
        });
    });

    // ------------------------------------------------------------------
    // The 3D person marker from a mothership loc_update frame
    // ------------------------------------------------------------------
    describe('unresolved walker: generic marker', function () {
        it('renders the marker at (x, 0, z) — on the floor, at the fed coordinates', function () {
            Viz3D.handleLocUpdate({ type: 'loc_update', blobs: [WALKER] });

            var obj = blobObj(7);
            expect(obj).toBeDefined();
            expect(obj.group.position.x).toBeCloseTo(2.5, 6);
            expect(obj.group.position.y).toBeCloseTo(0, 6);   // floor plane
            expect(obj.group.position.z).toBeCloseTo(3.75, 6);

            // Generic gray marker until identity resolves (bf-1h7h / bf-3j3s).
            expect(obj.rep).toBe('marker');
            expect(obj.humanoid).toBeNull();
            expect(obj.marker).not.toBeNull();
            expect(obj.group.children).toContain(obj.marker);
            expect(obj.marker.material.color.css).toBe('#888888');
            expect(obj.marker.position.y).toBeCloseTo(0.28, 6); // resting on the floor
        });

        it('anchors the pillar to the floor at (x, z) and raises it to room height', function () {
            Viz3D.handleLocUpdate({ type: 'loc_update', blobs: [WALKER] });

            var obj = blobObj(7);
            // [x, 0.05, z, x, height-0.05, z] — a floor-to-ceiling height probe
            // at the walker's fed coordinates (2.5, 3.75) in the 2.7 m room.
            arrClose(obj.pillar.geometry.attributes.position.array,
                [2.5, 0.05, 3.75, 2.5, ROOM.height - 0.05, 3.75]);
            expect(scene.children).toContain(obj.pillar);
        });

        it('pins the fed trail to the floor plane (y = 0.02) in fed (x, z) order', function () {
            Viz3D.handleLocUpdate({ type: 'loc_update', blobs: [WALKER] });

            var obj = blobObj(7);
            arrClose(obj.trail.geometry.attributes.position.array.slice(0, 9),
                [2.5, 0.02, 3.75, 2.2, 0.02, 3.5, 1.9, 0.02, 3.25]);
            expect(obj.trail.geometry.drawRange.count).toBe(3);
        });

        it('reports the fed state through the getBlobStates render probe', function () {
            Viz3D.handleLocUpdate({ type: 'loc_update', blobs: [WALKER] });
            expect(Viz3D.getBlobStates()).toEqual([
                { id: 7, x: 2.5, z: 3.75, vx: 0, vz: 0 }
            ]);
        });

        it('carries no name label while identity is unresolved', function () {
            Viz3D.handleLocUpdate({ type: 'loc_update', blobs: [WALKER] });
            var obj = blobObj(7);
            expect(obj.identityResolved).toBe(false);
            expect(obj.nameLabel).toBeFalsy();
            var sprites = obj.group.children.filter(function (c) { return c.type === 'Sprite'; });
            expect(sprites).toHaveLength(0);
        });
    });

    // ------------------------------------------------------------------
    // Identity-resolved person: humanoid + name label
    // ------------------------------------------------------------------
    describe('resolved person: humanoid marker', function () {
        var PERSON = {
            id: 7, x: 2.5, z: 3.75, vx: 0, vz: 0, weight: 0.9,
            trail: [[2.5, 3.75]],
            posture: 'standing',
            personName: 'Alice', assignedColor: '#3b82f6', identityResolved: true
        };

        it('upgrades the marker to a per-person colored humanoid when identity resolves', function () {
            Viz3D.handleLocUpdate({ type: 'loc_update', blobs: [WALKER] });
            Viz3D.handleLocUpdate({ type: 'loc_update', blobs: [PERSON] });

            var obj = blobObj(7);
            expect(obj.identityResolved).toBe(true);
            expect(obj.personName).toBe('Alice');
            expect(obj.rep).toBe('humanoid');
            expect(obj.marker).toBeNull();                 // gray marker torn down
            expect(obj.humanoid).not.toBeNull();
            expect(obj.group.children).toContain(obj.humanoid.mesh);
            expect(obj.humanoid.mesh.material.color.css).toBe('#3b82f6');
        });

        it('floats the person name label above the figure, parented to the group', function () {
            Viz3D.handleLocUpdate({ type: 'loc_update', blobs: [PERSON] });

            var obj = blobObj(7);
            var sprites = obj.group.children.filter(function (c) { return c.type === 'Sprite'; });
            expect(sprites).toHaveLength(1);
            expect(sprites[0].userData.labelText).toBe('Alice');
            expect(sprites[0].position.y).toBeCloseTo(2.0, 6); // above ~1.8 m head
        });

        it('drives the walking posture and facing from fed velocity, standing otherwise', function () {
            var strolling = Object.assign({}, PERSON, { vx: 0.6, vz: 0.8 }); // |v| = 1.0 m/s
            Viz3D.handleLocUpdate({ type: 'loc_update', blobs: [strolling] });

            var obj = blobObj(7);
            expect(obj.humanoid.posture).toBe('walking');
            expect(obj.humanoid.actions.walking.timeScale).toBeCloseTo(1.8, 6); // min(1.0*1.8, 2.5)
            expect(obj.group.rotation.y).toBeCloseTo(Math.atan2(0.6, 0.8), 6);

            Viz3D.handleLocUpdate({ type: 'loc_update', blobs: [PERSON] }); // back to still
            expect(obj.humanoid.posture).toBe('standing');
        });
    });

    // ------------------------------------------------------------------
    // Frame-scoped tracking across a snapshot -> delta sequence
    // ------------------------------------------------------------------
    describe('multi-blob frames and eviction', function () {
        it('renders every blob in the frame and evicts ids the next frame omits', function () {
            var other = {
                id: 8, x: 6.25, z: 1.25, vx: 0, vz: 0, weight: 0.8, trail: [[6.25, 1.25]]
            };
            Viz3D.handleLocUpdate({ type: 'loc_update', blobs: [WALKER, other] });
            expect(Viz3D.getBlobStates().map(function (s) { return s.id; }).sort())
                .toEqual([7, 8]);
            var gone = blobObj(7).group;

            Viz3D.handleLocUpdate({ type: 'loc_update', blobs: [other] });
            expect(Viz3D.getBlobStates()).toEqual([
                { id: 8, x: 6.25, z: 1.25, vx: 0, vz: 0 }
            ]);
            // Evicted marker removed from the scene graph entirely.
            expect(scene.children).not.toContain(gone);
            expect(scene.children).toContain(blobObj(8).group);
        });

        it('clears the scene when a frame carries no blobs (walker out of range)', function () {
            Viz3D.handleLocUpdate({ type: 'loc_update', blobs: [WALKER] });
            Viz3D.handleLocUpdate({ type: 'loc_update', blobs: [] });
            expect(Viz3D.getBlobStates()).toEqual([]);
            expect(blobCount()).toBe(0);
        });
    });
});
