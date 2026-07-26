/**
 * Spaxel Dashboard - Blob Identity Matcher Tests
 *
 * Bead bf-iner. Runtime (jsdom) proof for the data-only matcher in
 * blob-identity.js. Covers the acceptance criteria:
 *   - matching function exists and is callable
 *   - matched blobs get personName + assignedColor populated
 *   - unmatched blobs get identityResolved=false and no name/color
 *   - no crashes when BLE identity data is missing or malformed
 *
 * The matcher is a pure IIFE with no Three.js / WebGL dependency, so it loads
 * cleanly under jsdom (unlike viz3d.js).
 */

describe('BlobIdentity matcher (bf-iner)', function () {

    var BlobIdentity;

    function load() {
        if (!window.BlobIdentity) require('./blob-identity.js');
        return window.BlobIdentity;
    }

    beforeEach(function () {
        BlobIdentity = load();
    });

    // ------------------------------------------------------------------
    // Module surface
    // ------------------------------------------------------------------
    describe('module surface', function () {
        it('exposes the matcher on window.BlobIdentity', function () {
            expect(BlobIdentity).toBeDefined();
            expect(typeof BlobIdentity.resolve).toBe('function');
            expect(typeof BlobIdentity.resolveAll).toBe('function');
            expect(typeof BlobIdentity.findDeviceForBlob).toBe('function');
            expect(typeof BlobIdentity.colorForName).toBe('function');
            expect(typeof BlobIdentity.isRealName).toBe('function');
        });
    });

    // ------------------------------------------------------------------
    // Matching paths
    // ------------------------------------------------------------------
    describe('matching via BLE device', function () {
        it('matches by blob.ble_device address → registry label + color', function () {
            var blob = { id: 7, ble_device: 'AA:BB:CC:DD:EE:FF', x: 1, y: 2, z: 1 };
            var devices = [{
                addr: 'AA:BB:CC:DD:EE:FF', label: 'Alice', color: '#3b82f6', blob_id: 7
            }];
            var r = BlobIdentity.resolve(blob, devices);
            expect(r.identityResolved).toBe(true);
            expect(r.personName).toBe('Alice');
            expect(r.assignedColor).toBe('#3b82f6');
        });

        it('matches by device.blob_id when blob has no ble_device address', function () {
            var blob = { id: 12, x: 1, y: 2, z: 1 };
            var devices = [
                { mac: '11:22:33:44:55:66', blob_id: 99, label: 'Bob' },
                { mac: 'AA:BB:CC:DD:EE:FF', blob_id: 12, label: 'Bob', color: '#ef4444' }
            ];
            var r = BlobIdentity.resolve(blob, devices);
            expect(r.identityResolved).toBe(true);
            expect(r.personName).toBe('Bob');
            expect(r.assignedColor).toBe('#ef4444');
        });

        it('matches from a live-scan device with snake_case person_name', function () {
            // Shape produced by app.js handleBLEScanMessage / spaxelGetState().
            var blob = { id: 3 };
            var devices = [{ mac: 'AA:BB:CC:DD:EE:FF', blob_id: 3, person_name: 'Carol' }];
            var r = BlobIdentity.resolve(blob, devices);
            expect(r.identityResolved).toBe(true);
            expect(r.personName).toBe('Carol');
            // No explicit color on the scan device → derived from the name.
            expect(r.assignedColor).toMatch(/^hsl\(\d+, 70%, 60%\)$/);
        });

        it('matches from a Map-shaped device source', function () {
            var blob = { id: 5, ble_device: 'DE:AD:BE:EF:00:01' };
            var map = new Map();
            map.set('DE:AD:BE:EF:00:01', { addr: 'de:ad:be:ef:00:01', label: 'Dave' });
            var r = BlobIdentity.resolve(blob, map);
            expect(r.identityResolved).toBe(true);
            expect(r.personName).toBe('Dave');
        });

        it('matches from an object-keyed registry source', function () {
            var blob = { id: 2 };
            var registry = {
                'AA:BB:CC:DD:EE:FF': { addr: 'AA:BB:CC:DD:EE:FF', label: 'Erin', blob_id: 2 }
            };
            var r = BlobIdentity.resolve(blob, registry);
            expect(r.identityResolved).toBe(true);
            expect(r.personName).toBe('Erin');
        });

        it('prefers an address-matched device that carries a label', function () {
            var blob = { id: 4, ble_device: 'AA:BB:CC:DD:EE:FF' };
            var devices = [
                { addr: 'AA:BB:CC:DD:EE:FF', label: '' },        // same addr, no label
                { addr: 'AA:BB:CC:DD:EE:FF', label: 'Frank' }     // same addr, real label
            ];
            var r = BlobIdentity.resolve(blob, devices);
            expect(r.personName).toBe('Frank');
            expect(r.identityResolved).toBe(true);
        });
    });

    // ------------------------------------------------------------------
    // Server-resolved identity already on the blob
    // ------------------------------------------------------------------
    describe('server-resolved identity on the blob', function () {
        it('trusts personName/assignedColor already present', function () {
            var blob = { id: 1, personName: 'Grace', assignedColor: '#22c55e' };
            var r = BlobIdentity.resolve(blob, []);
            expect(r.identityResolved).toBe(true);
            expect(r.personName).toBe('Grace');
            expect(r.assignedColor).toBe('#22c55e');
        });

        it('accepts snake_case aliases and the bare `person` field', function () {
            var r1 = BlobIdentity.resolve({ id: 1, person_name: 'Heidi' }, []);
            expect(r1.personName).toBe('Heidi');
            expect(r1.identityResolved).toBe(true);

            var r2 = BlobIdentity.resolve({ id: 2, person: 'Ivan' }, []);
            expect(r2.personName).toBe('Ivan');
            expect(r2.identityResolved).toBe(true);
        });

        it('derives a color from the name when none is assigned', function () {
            var r = BlobIdentity.resolve({ id: 1, personName: 'Judy' }, []);
            expect(r.assignedColor).toMatch(/^hsl\(\d+, 70%, 60%\)$/);
            expect(r.identityResolved).toBe(true);
        });

        it('fills in a missing color from a matching device', function () {
            var blob = { id: 1, personName: 'Karl' }; // name, no color
            var devices = [{ ble_device: 'AA:BB:CC:DD:EE:FF', blob_id: 1, color: '#000000' }];
            var r = BlobIdentity.resolve(blob, devices);
            expect(r.personName).toBe('Karl');
            expect(r.assignedColor).toBe('#000000'); // taken from the device
            expect(r.identityResolved).toBe(true);
        });
    });

    // ------------------------------------------------------------------
    // Unmatched blobs
    // ------------------------------------------------------------------
    describe('unmatched blobs', function () {
        it('returns identityResolved=false with null name/color when no match', function () {
            var blob = { id: 9, x: 1, y: 2, z: 1 };
            var r = BlobIdentity.resolve(blob, [{ mac: 'AA', blob_id: 1, label: 'Alice' }]);
            expect(r).toEqual({ personName: null, assignedColor: null, identityResolved: false });
        });

        it('treats placeholder device names (Unknown) as non-identity', function () {
            var blob = { id: 1, ble_device: 'AA:BB:CC:DD:EE:FF' };
            var devices = [{ addr: 'AA:BB:CC:DD:EE:FF', name: 'Unknown', blob_id: 1 }];
            var r = BlobIdentity.resolve(blob, devices);
            expect(r.identityResolved).toBe(false);
            expect(r.personName).toBeNull();
            expect(r.assignedColor).toBeNull();
        });

        it('is identity-less when only a raw device name (no registry label) is present', function () {
            // 'iPhone' is the advertised device name, not a registered person. Only a
            // registry-assigned label/person_name promotes to an identity; a raw beacon
            // name must NOT. (Server-resolved blob.person is a different, authoritative path.)
            var blob = { id: 1 };
            var devices = [{ mac: 'AA:BB:CC:DD:EE:FF', blob_id: 1, name: 'iPhone' }];
            var r = BlobIdentity.resolve(blob, devices);
            expect(r.identityResolved).toBe(false);
            expect(r.personName).toBeNull();
            expect(r.assignedColor).toBeNull();
        });
    });

    // ------------------------------------------------------------------
    // Robustness: missing / malformed data must not crash
    // ------------------------------------------------------------------
    describe('robustness', function () {
        it('handles a null blob', function () {
            expect(function () { BlobIdentity.resolve(null, []); }).not.toThrow();
            expect(BlobIdentity.resolve(null, [])).toEqual({
                personName: null, assignedColor: null, identityResolved: false
            });
        });

        it('handles undefined / non-object blob', function () {
            expect(function () { BlobIdentity.resolve(undefined, []); }).not.toThrow();
            expect(function () { BlobIdentity.resolve('blob', []); }).not.toThrow();
            expect(function () { BlobIdentity.resolve(42, []); }).not.toThrow();
        });

        it('handles missing BLE source (null/undefined)', function () {
            var blob = { id: 1, ble_device: 'AA:BB:CC:DD:EE:FF' };
            expect(function () { BlobIdentity.resolve(blob, null); }).not.toThrow();
            expect(function () { BlobIdentity.resolve(blob, undefined); }).not.toThrow();
            expect(BlobIdentity.resolve(blob, null).identityResolved).toBe(false);
        });

        it('handles malformed device records without crashing', function () {
            var blob = { id: 1 };
            var devices = [null, undefined, 'string', 7, {}, { blob_id: 1, label: null },
                { blob_id: '1', label: NaN }];
            expect(function () { BlobIdentity.resolve(blob, devices); }).not.toThrow();
        });

        it('handles a non-iterable BLE source (number / string)', function () {
            var blob = { id: 1 };
            expect(function () { BlobIdentity.resolve(blob, 12345); }).not.toThrow();
            expect(function () { BlobIdentity.resolve(blob, 'not-a-source'); }).not.toThrow();
        });

        it('does not mutate the input blob', function () {
            var blob = { id: 1, person: 'Alice', x: 1 };
            var snapshot = JSON.parse(JSON.stringify(blob));
            BlobIdentity.resolve(blob, [{ blob_id: 1, label: 'Alice', color: '#fff' }]);
            expect(blob).toEqual(snapshot);
        });
    });

    // ------------------------------------------------------------------
    // colorForName / isRealName helpers
    // ------------------------------------------------------------------
    describe('helpers', function () {
        it('colorForName is stable for the same name', function () {
            expect(BlobIdentity.colorForName('Alice'))
                .toBe(BlobIdentity.colorForName('Alice'));
        });

        it('colorForName differs for different names', function () {
            expect(BlobIdentity.colorForName('Alice'))
                .not.toBe(BlobIdentity.colorForName('Bob'));
        });

        it('colorForName returns neutral gray for empty input', function () {
            expect(BlobIdentity.colorForName('')).toBe('#888888');
            expect(BlobIdentity.colorForName(null)).toBe('#888888');
        });

        it('isRealName rejects placeholders', function () {
            expect(BlobIdentity.isRealName('Alice')).toBe(true);
            expect(BlobIdentity.isRealName('Unknown')).toBe(false);
            expect(BlobIdentity.isRealName('')).toBe(false);
            expect(BlobIdentity.isRealName('  ')).toBe(false);
            expect(BlobIdentity.isRealName(null)).toBe(false);
        });
    });

    // ------------------------------------------------------------------
    // resolveAll
    // ------------------------------------------------------------------
    describe('resolveAll', function () {
        it('populates identity across a mixed blob fleet', function () {
            var blobs = [
                { id: 1, ble_device: 'AA:BB:CC:DD:EE:FF' },     // will match
                { id: 2, personName: 'Alice', assignedColor: '#abc' }, // server-resolved
                { id: 3 }                                         // unmatched
            ];
            var devices = [{ addr: 'AA:BB:CC:DD:EE:FF', blob_id: 1, label: 'Bob', color: '#def' }];
            var out = BlobIdentity.resolveAll(blobs, devices);

            expect(out[0].identityResolved).toBe(true);
            expect(out[0].personName).toBe('Bob');
            expect(out[0].assignedColor).toBe('#def');

            expect(out[1].identityResolved).toBe(true);
            expect(out[1].personName).toBe('Alice');
            expect(out[1].assignedColor).toBe('#abc');

            expect(out[2].identityResolved).toBe(false);
            expect(out[2].personName).toBeNull();
            expect(out[2].assignedColor).toBeNull();
        });

        it('returns shallow copies and does not mutate inputs', function () {
            var blobs = [{ id: 1, personName: 'Alice' }];
            var out = BlobIdentity.resolveAll(blobs, []);
            expect(out[0]).not.toBe(blobs[0]);            // new object
            expect(blobs[0].identityResolved).toBeUndefined(); // input untouched
            expect(out[0].identityResolved).toBe(true);
        });

        it('returns [] for a non-array input', function () {
            expect(BlobIdentity.resolveAll(null, [])).toEqual([]);
            expect(BlobIdentity.resolveAll(undefined, [])).toEqual([]);
        });
    });

    // ------------------------------------------------------------------
    // Integration: blob carries server-resolved identity AND a BLE link
    // (server field wins; this is the realistic mothership snapshot shape)
    // ------------------------------------------------------------------
    describe('integration: realistic snapshot blob', function () {
        it('resolves a blob with both person and ble_device', function () {
            var blob = {
                id: 1, x: 2.5, y: 1.0, z: 1.0, confidence: 0.85,
                vx: 0.1, vz: 0, posture: 'standing',
                person: 'Alice', ble_device: 'AA:BB:CC:DD:EE:FF'
            };
            var devices = [{ addr: 'AA:BB:CC:DD:EE:FF', blob_id: 1, label: 'Alice', color: '#3b82f6' }];
            var r = BlobIdentity.resolve(blob, devices);
            expect(r.identityResolved).toBe(true);
            expect(r.personName).toBe('Alice');
            expect(r.assignedColor).toBe('#3b82f6');
        });
    });

    // meshColor — per-person mesh color for humanoid figures (bf-3j3s).
    // A resolved person renders in its assigned color; an unresolved blob
    // renders in neutral gray. Pure and robust to malformed input.
    describe('meshColor (bf-3j3s)', function () {
        var cases = [
            // [label, resolvedResult, expected]
            ['resolved with explicit registry color',
                { identityResolved: true, assignedColor: '#3b82f6' }, '#3b82f6'],
            ['resolved with hash-derived hsl color',
                { identityResolved: true, assignedColor: 'hsl(123, 70%, 60%)' }, 'hsl(123, 70%, 60%)'],
            ['resolved but color missing → meshColor treats null assignedColor as unresolved, defensively (resolve() itself never returns this)',
                { identityResolved: true, assignedColor: null }, '#888888'],
            ['unresolved (identityResolved false)',
                { identityResolved: false, assignedColor: null }, '#888888'],
            ['empty resolve() result',
                { personName: null, assignedColor: null, identityResolved: false }, '#888888'],
            ['null input', null, '#888888'],
            ['undefined input', undefined, '#888888'],
            ['object with no identity fields', { personName: null }, '#888888'],
        ];
        cases.forEach(function (c) {
            it('returns the expected color: ' + c[0], function () {
                expect(BlobIdentity.meshColor(c[1])).toBe(c[2]);
            });
        });

        it('uses the default neutral gray (#888888) — matches viz3d marker material', function () {
            expect(BlobIdentity.UNIDENTIFIED_GRAY).toBe('#888888');
            expect(BlobIdentity.meshColor({ identityResolved: false })).toBe('#888888');
        });

        it('honors a caller-supplied default color override', function () {
            expect(BlobIdentity.meshColor(null, '#9e9e9e')).toBe('#9e9e9e');
            // A blank/invalid override falls back to the canonical gray.
            expect(BlobIdentity.meshColor(null, '')).toBe('#888888');
        });

        it('never lets an empty-string assignedColor through for a "resolved" blob', function () {
            expect(BlobIdentity.meshColor({ identityResolved: true, assignedColor: '' }))
                .toBe(BlobIdentity.UNIDENTIFIED_GRAY);
        });

        it('end-to-end: resolve() then meshColor() for an identified person', function () {
            var blob = { id: 2, person: 'Alice' };
            var r = BlobIdentity.resolve(blob, []);
            // resolve() backfills a stable hash color when none is registered.
            expect(r.identityResolved).toBe(true);
            expect(r.assignedColor).toBeTruthy();
            expect(BlobIdentity.meshColor(r)).toBe(r.assignedColor);
        });

        it('end-to-end: resolve() then meshColor() for an unidentified blob → gray', function () {
            var r = BlobIdentity.resolve({ id: 9 }, []);
            expect(r.identityResolved).toBe(false);
            expect(BlobIdentity.meshColor(r)).toBe('#888888');
        });

        it('is exposed as a function on window.BlobIdentity', function () {
            expect(typeof BlobIdentity.meshColor).toBe('function');
        });
    });
});
