/**
 * Spaxel Dashboard - Zone Containment Lookup (pure data logic)
 *
 * Bead bf-3dip. Resolves which floor-plan zone contains a given (x, z) point,
 * so a person's floating name label can read "Alice is in Kitchen" instead of
 * just "Alice". Pure: no Three.js / WebGL / DOM dependency, so it loads cleanly
 * under jsdom (unlike viz3d.js) and is unit-tested in zone-lookup.test.js.
 * Consumed by viz3d to render per-person "{name} is in {zone}" labels above
 * humanoid meshes.
 *
 * Zone coordinate convention (matches viz3d._createZoneMesh and the server's
 * `zones` table): a zone is an axis-aligned floor-plan box described by an
 * origin corner (x, y, z) and extents w (width / X), d (depth / Z), h
 * (height / Y, vertical). Room membership is a 2D floor-plane containment
 * test on X and Z only — a tracked person lives on the floor plane (the
 * figure's Y is a rendering offset, not a room coordinate), so Z-height is
 * intentionally ignored when deciding which room someone is "in".
 *
 * Robustness contract: nameAt() / contains() must NEVER throw, regardless of
 * missing or malformed zone data (null zones, non-numeric extents, weird
 * container shapes). See zone-lookup.test.js for the full contract.
 */
(function () {
    'use strict';

    // A finite number, else the supplied default (0). Guards against NaN /
    // string / undefined extents silently producing a bogus containment result.
    function num(v, dflt) {
        return (typeof v === 'number' && isFinite(v)) ? v : dflt;
    }

    /**
     * True if the floor-plane point (x, z) lies inside `zone` (its X/Z box).
     * A point exactly on an edge counts as inside. Degenerate zones
     * (non-positive width or depth) contain nothing. Never throws.
     *
     * @param {Object|null} zone - zone record with x,z,w,d (origin corner + extents)
     * @param {number} x - floor-plan X (meters)
     * @param {number} z - floor-plan Z (meters)
     * @returns {boolean}
     */
    function contains(zone, x, z) {
        if (!zone || typeof zone !== 'object') return false;
        var zx = num(zone.x, 0);
        var zz = num(zone.z, 0);
        var zw = num(zone.w, 0);
        var zd = num(zone.d, 0);
        if (zw <= 0 || zd <= 0) return false;
        return x >= zx && x <= zx + zw && z >= zz && z <= zz + zd;
    }

    // Iterate zones from any supported container shape (Array, Map, plain
    // object, or null). Invokes fn(zone) per record. Never throws — a
    // malformed source simply yields nothing. Mirrors BlobIdentity.eachDevice.
    function eachZone(zones, fn) {
        if (!zones) return;
        try {
            if (Array.isArray(zones)) {
                for (var i = 0; i < zones.length; i++) fn(zones[i]);
            } else if (typeof zones.forEach === 'function') {
                // Map.forEach yields (value, key); array-likes yield (value, idx).
                zones.forEach(function (v) { fn(v); });
            } else if (typeof zones === 'object') {
                var keys = Object.keys(zones);
                for (var k = 0; k < keys.length; k++) fn(zones[keys[k]]);
            }
        } catch (e) {
            // Swallow: malformed source must not crash the lookup.
        }
    }

    /**
     * Name of the zone whose X/Z box contains the point (x, z), or '' when the
     * point is in no zone (or zones is empty/malformed). Never throws.
     *
     * When several zones overlap the point, the SMALLEST-area zone wins — the
     * most specific room (e.g. a "Pantry" nook inside a "Kitchen") takes
     * precedence over the larger enclosing room.
     *
     * @param {number} x - floor-plan X (meters)
     * @param {number} z - floor-plan Z (meters)
     * @param {Array|Map|Object|null} zones - container of zone records
     * @returns {string} zone name, or '' if none / nameless
     */
    function nameAt(x, z, zones) {
        var best = null;
        var bestArea = Infinity;
        eachZone(zones, function (zone) {
            if (contains(zone, x, z)) {
                var area = num(zone.w, 0) * num(zone.d, 0);
                if (area > 0 && area < bestArea) {
                    bestArea = area;
                    best = zone;
                }
            }
        });
        return best ? (best.name || '') : '';
    }

    window.ZoneLookup = {
        contains: contains,
        nameAt: nameAt,
        eachZone: eachZone
    };

    if (typeof console !== 'undefined') {
        console.log('[ZoneLookup] zone containment lookup initialized');
    }
})();
