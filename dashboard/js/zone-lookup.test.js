/**
 * Spaxel Dashboard - Zone Containment Lookup Tests
 *
 * Bead bf-3dip. Runtime (jsdom) proof for the pure lookup in zone-lookup.js.
 * Drives the per-person "{name} is in {zone}" name labels: nameAt() decides
 * which room's name follows a person's name above their humanoid mesh. Covers:
 *   - point clearly inside a zone -> that zone's name
 *   - point outside every zone -> ''
 *   - point on a zone edge -> inside (counts as contained)
 *   - overlapping zones -> smallest-area zone wins (most specific room)
 *   - empty / null / malformed zones -> '' (never throws)
 *   - zones supplied as Array, Map, or plain Object
 *
 * The module is a pure IIFE with no Three.js dependency, so it loads cleanly
 * under jsdom (unlike viz3d.js).
 */

describe('ZoneLookup (bf-3dip)', function () {

    var ZoneLookup;

    function load() {
        if (!window.ZoneLookup) require('./zone-lookup.js');
        return window.ZoneLookup;
    }

    beforeEach(function () {
        ZoneLookup = load();
    });

    // ------------------------------------------------------------------
    // Module surface
    // ------------------------------------------------------------------
    describe('module surface', function () {
        it('exposes the lookup on window.ZoneLookup', function () {
            expect(ZoneLookup).toBeDefined();
            expect(typeof ZoneLookup.nameAt).toBe('function');
            expect(typeof ZoneLookup.contains).toBe('function');
            expect(typeof ZoneLookup.eachZone).toBe('function');
        });
    });

    // ------------------------------------------------------------------
    // contains() — per-zone 2D floor-plane box test
    // ------------------------------------------------------------------
    describe('contains', function () {
        var zone = { name: 'Kitchen', x: 0, y: 0, z: 0, w: 4, d: 3, h: 2.5 };

        var cases = [
            { desc: 'center point',          x: 2,   z: 1.5, want: true  },
            { desc: 'origin corner',         x: 0,   z: 0,   want: true  },
            { desc: 'far corner',            x: 4,   z: 3,   want: true  },
            { desc: 'on +X edge',            x: 4,   z: 1,   want: true  },
            { desc: 'on +Z edge',            x: 2,   z: 3,   want: true  },
            { desc: 'just outside +X',       x: 4.01,z: 1,   want: false },
            { desc: 'just outside -X',       x: -0.01,z: 1,  want: false },
            { desc: 'just outside +Z',       x: 2,   z: 3.01,want: false },
            { desc: 'far away',              x: 100, z: 100, want: false }
        ];
        cases.forEach(function (c) {
            it('returns ' + c.want + ' for ' + c.desc, function () {
                expect(ZoneLookup.contains(zone, c.x, c.z)).toBe(c.want);
            });
        });

        it('rejects degenerate (non-positive extent) zones', function () {
            expect(ZoneLookup.contains({ x: 0, z: 0, w: 0, d: 3 }, 0, 0)).toBe(false);
            expect(ZoneLookup.contains({ x: 0, z: 0, w: 4, d: -1 }, 0, 0)).toBe(false);
        });

        it('defaults missing extents to 0 (treats as degenerate)', function () {
            expect(ZoneLookup.contains({ name: 'Z', x: 0, z: 0 }, 0, 0)).toBe(false);
        });
    });

    // ------------------------------------------------------------------
    // nameAt() — resolves a point to a zone name
    // ------------------------------------------------------------------
    describe('nameAt', function () {
        var zones = [
            { id: 1, name: 'Kitchen',  x: 0, z: 0, w: 4, d: 3 },   // [0,4] x [0,3]
            { id: 2, name: 'Hallway',  x: 4, z: 0, w: 2, d: 6 }    // [4,6] x [0,6]
        ];

        var cases = [
            { desc: 'inside Kitchen',        x: 2,   z: 1,   want: 'Kitchen' },
            { desc: 'inside Hallway',        x: 5,   z: 4,   want: 'Hallway' },
            { desc: 'just inside Hallway',   x: 4.5, z: 1,   want: 'Hallway' },
            { desc: 'outside everything',    x: 10,  z: 10,  want: '' },
            { desc: 'negative space',        x: -1,  z: -1,  want: '' }
        ];
        cases.forEach(function (c) {
            it('returns ' + JSON.stringify(c.want) + ' for ' + c.desc, function () {
                expect(ZoneLookup.nameAt(c.x, c.z, zones)).toBe(c.want);
            });
        });

        it('picks the smallest-area zone when zones overlap', function () {
            // Pantry (2x2) sits entirely inside Kitchen (4x3); a point in both
            // should resolve to the more specific room.
            var overlap = [
                { id: 1, name: 'Kitchen', x: 0, z: 0, w: 4, d: 3 },
                { id: 2, name: 'Pantry',  x: 0, z: 0, w: 2, d: 2 }
            ];
            expect(ZoneLookup.nameAt(1, 1, overlap)).toBe('Pantry');
        });

        it('on an exact area tie resolves to the first-iterated zone', function () {
            // Kitchen [0,4]x[0,3] and Hallway [4,6]x[0,6] both contain the
            // shared edge x=4 and have unequal area; equal-area ties keep the
            // first-encountered zone (strict < in the smallest-area rule).
            var equal = [
                { id: 1, name: 'A', x: 0, z: 0, w: 3, d: 3 }, // area 9
                { id: 2, name: 'B', x: 0, z: 0, w: 3, d: 3 }  // area 9 (tie)
            ];
            expect(ZoneLookup.nameAt(1, 1, equal)).toBe('A');
        });

        it('returns "" when the containing zone has no name', function () {
            var z = [{ id: 1, name: '', x: 0, z: 0, w: 4, d: 3 }];
            expect(ZoneLookup.nameAt(1, 1, z)).toBe('');
        });
    });

    // ------------------------------------------------------------------
    // Container shapes — Array, Map, plain Object, plus robustness
    // ------------------------------------------------------------------
    describe('container shapes', function () {
        var kitchen = { id: 1, name: 'Kitchen', x: 0, z: 0, w: 4, d: 3 };

        it('accepts an Array', function () {
            expect(ZoneLookup.nameAt(2, 1, [kitchen])).toBe('Kitchen');
        });

        it('accepts a Map (zoneID -> zone)', function () {
            var m = new Map();
            m.set(1, kitchen);
            expect(ZoneLookup.nameAt(2, 1, m)).toBe('Kitchen');
        });

        it('accepts a plain Object (id -> zone)', function () {
            expect(ZoneLookup.nameAt(2, 1, { 1: kitchen })).toBe('Kitchen');
        });
    });

    describe('robustness (never throws)', function () {
        var throwers = [
            { desc: 'null zones',            zones: null },
            { desc: 'undefined zones',       zones: undefined },
            { desc: 'empty array',           zones: [] },
            { desc: 'empty object',          zones: {} },
            { desc: 'array of nulls',        zones: [null, undefined, {}] },
            { desc: 'malformed zone fields', zones: [{ name: 'X', w: 'wide', d: NaN }] },
            { desc: 'non-numeric point',     zones: [{ name: 'X', x: 0, z: 0, w: 4, d: 3 }] }
        ];
        throwers.forEach(function (c) {
            it('does not throw for ' + c.desc, function () {
                var fn;
                if (c.desc === 'non-numeric point') {
                    fn = function () { ZoneLookup.nameAt('nope', undefined, c.zones); };
                } else {
                    fn = function () { ZoneLookup.nameAt(2, 1, c.zones); };
                }
                expect(fn).not.toThrow();
            });
        });

        it('contains() does not throw on null zone', function () {
            expect(function () { ZoneLookup.contains(null, 0, 0); }).not.toThrow();
            expect(ZoneLookup.contains(null, 0, 0)).toBe(false);
        });
    });
});
