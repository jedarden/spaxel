/**
 * Spaxel Dashboard - Blob Identity Matching (data-only)
 *
 * Bead bf-iner. Matches BLE registry identities to blob IDs so each blob can
 * carry its assigned person's name and color. Pure data logic — NO rendering.
 * Rendering of identity labels / per-person colors is handled elsewhere
 * (viz3d.updateIdentities, ambient_renderer.getPersonColor, ble-panel). This
 * module only computes the identity and stores it on the blob data structure.
 *
 * BLE identity source (the "spaxel-2wg" work): the dashboard relays live BLE
 * scan results from the mothership. Each scan device carries a `blob_id` that
 * links it to a tracked blob plus a registry-assigned `label` (person name).
 * The blob itself may also carry a server-resolved `person` / `ble_device`.
 * `window.spaxelGetState().bleDevices` exposes the live scan as an array;
 * `window.SpaxelState.ble_devices` exposes the registered device registry as
 * an addr→device object. resolve() accepts either shape (Array | Map | Object).
 *
 * Identity resolution precedence (most authoritative first):
 *   1. Server-resolved identity already present on the blob
 *      (personName / assignedColor and their snake_case aliases, or `person`).
 *   2. BLE registry/scan device linked to this blob:
 *        a. blob.ble_device (address) → device with matching addr.
 *        b. device.blob_id === blob.id.
 *   3. Unmatched → identityResolved=false, no name/color.
 *
 * Robustness contract: resolve() must NEVER throw, regardless of missing or
 * malformed blob / BLE data (null blob, non-array sources, devices missing
 * fields, wrong types). See blob-identity.test.js for the full contract.
 */
(function () {
    'use strict';

    // Labels that are not real person identities (raw beacons / placeholders).
    var NON_IDENTITIES = {
        '': true,
        'unknown': true,
        'unknown device': true,
        'unnamed': true,
        'n/a': true,
        'null': true,
        'undefined': true
    };

    function isString(v) { return typeof v === 'string'; }

    // Coerce to a trimmed string; return '' for non-strings / nullish.
    function asStr(v) {
        if (v === null || v === undefined) return '';
        if (isString(v)) return v.trim();
        return '';
    }

    function isRealName(name) {
        var s = asStr(name).toLowerCase();
        return s.length > 0 && !NON_IDENTITIES[s];
    }

    // First non-empty trimmed-string value among args; '' if none.
    function firstNonEmpty(/* ...values */) {
        for (var i = 0; i < arguments.length; i++) {
            var s = asStr(arguments[i]);
            if (s.length > 0) return s;
        }
        return '';
    }

    // Normalize a color to a non-empty string or null.
    function asColor(v) {
        var s = asStr(v);
        return s.length > 0 ? s : null;
    }

    /**
     * Stable color derived from a person name. Mirrors the algorithm used by
     * ble-panel.getColorForPerson when no explicit color is assigned. Returns a
     * CSS hsl() string; '#888888' (device-default neutral gray) for empty names.
     */
    function colorForName(name) {
        var s = asStr(name);
        if (s.length === 0) return '#888888';
        var hash = 0;
        for (var i = 0; i < s.length; i++) {
            hash = s.charCodeAt(i) + ((hash << 5) - hash);
            hash |= 0; // keep int32 so charCodeAt never drifts to a float
        }
        var hue = Math.abs(hash) % 360;
        return 'hsl(' + hue + ', 70%, 60%)';
    }

    // ---- device-record field accessors (tolerate any field spelling) -----------

    function deviceAddr(d) {
        if (!d || typeof d !== 'object') return '';
        return firstNonEmpty(d.addr, d.mac, d.address, d.device_addr, d.peer_mac).toLowerCase();
    }

    // Registry-assigned person name on a device (NOT the raw beacon name).
    function deviceLabel(d) {
        if (!d || typeof d !== 'object') return '';
        // Only promote a user-assigned identity field. The raw advertised device
        // `name` (e.g. "iPhone", "Unknown") is a beacon label, not a person, so
        // it is deliberately excluded — an unregistered device must not resolve
        // to an identity. (Server-resolved `blob.person` is handled separately
        // in resolve() and IS authoritative.)
        return firstNonEmpty(d.label, d.person_name, d.personName);
    }

    function deviceColor(d) {
        if (!d || typeof d !== 'object') return null;
        return asColor(firstNonEmpty(
            d.color, d.assigned_color, d.assignedColor, d.person_color, d.personColor
        ));
    }

    function deviceBlobId(d) {
        if (!d || typeof d !== 'object') return null;
        var id = (d.blob_id != null) ? d.blob_id : d.blobId;
        return (id == null) ? null : id;
    }

    /**
     * Iterate BLE devices from any supported container shape (Array, Map, plain
     * object, or null). Invokes fn(device) per record. Never throws — a
     * malformed source simply yields nothing.
     */
    function eachDevice(bleDevices, fn) {
        if (!bleDevices) return;
        try {
            if (Array.isArray(bleDevices)) {
                for (var i = 0; i < bleDevices.length; i++) fn(bleDevices[i]);
            } else if (typeof bleDevices.forEach === 'function') {
                // Map.forEach yields (value, key); array-likes yield (value, idx).
                bleDevices.forEach(function (v) { fn(v); });
            } else if (typeof bleDevices === 'object') {
                var keys = Object.keys(bleDevices);
                for (var k = 0; k < keys.length; k++) fn(bleDevices[keys[k]]);
            }
        } catch (e) {
            // Swallow: malformed source must not crash identity resolution.
        }
    }

    /**
     * Find the best BLE device record for a blob.
     * Prefers an address match (blob.ble_device); falls back to a blob_id link.
     * Among ties, prefers a device that actually carries a person label.
     * @returns {Object|null}
     */
    function findDeviceForBlob(blob, bleDevices) {
        if (!blob || typeof blob !== 'object') return null;
        var wantAddr = firstNonEmpty(blob.ble_device, blob.ble_addr, blob.peer_mac).toLowerCase();
        var wantId = blob.id;

        var byAddr = null;
        var byId = null;
        var byIdHasLabel = false;

        eachDevice(bleDevices, function (d) {
            if (!d || typeof d !== 'object') return;

            var addr = deviceAddr(d);
            if (wantAddr && addr && addr === wantAddr) {
                // Prefer the device that carries a real label when several share an addr.
                if (!byAddr || (deviceLabel(byAddr) === '' && deviceLabel(d) !== '')) {
                    byAddr = d;
                }
            }

            var did = deviceBlobId(d);
            if (wantId != null && did != null && String(did) === String(wantId)) {
                var hasLabel = deviceLabel(d) !== '';
                if (!byId || (byIdHasLabel === false && hasLabel === true)) {
                    byId = d;
                    byIdHasLabel = hasLabel;
                }
            }
        });

        return byAddr || byId || null;
    }

    /**
     * Resolve a blob's identity from its own fields + BLE device sources.
     * Pure: does not mutate the blob. Never throws.
     *
     * @param {Object} blob - blob record (may carry person/personName/ble_device/...)
     * @param {*} bleDevices - BLE identity source: Array | Map | Object of device
     *                         records, or null/undefined when unavailable.
     * @returns {{personName:string|null, assignedColor:string|null, identityResolved:boolean}}
     */
    function resolve(blob, bleDevices) {
        var empty = { personName: null, assignedColor: null, identityResolved: false };
        if (!blob || typeof blob !== 'object') return empty;

        // 1. Server-resolved identity already on the blob (authoritative).
        var name = firstNonEmpty(
            blob.personName, blob.person_name, blob.personLabel, blob.person_label, blob.person
        );
        var color = asColor(firstNonEmpty(
            blob.assignedColor, blob.assigned_color, blob.personColor, blob.person_color
        ));

        // 2. Match via BLE registry/scan.
        var haveName = isRealName(name);
        if (!haveName || !color) {
            var dev = findDeviceForBlob(blob, bleDevices);
            if (dev) {
                if (!haveName) {
                    var devLabel = deviceLabel(dev);
                    if (isRealName(devLabel)) name = devLabel;
                }
                if (!color) color = deviceColor(dev);
            }
        }

        // 3. Finalize: only a real name counts as resolved.
        if (isRealName(name)) {
            if (!color) color = colorForName(name);
            return { personName: name, assignedColor: color, identityResolved: true };
        }
        return empty;
    }

    // Neutral gray applied to identity-unresolved (unidentified) blobs.
    // Matches the viz3d generic-marker material default (0x888888) so an
    // unresolved sphere reads as a consistent, intentionally-neutral marker
    // rather than one of the bright BLOB_COLORS. See bead bf-3j3s.
    var UNIDENTIFIED_GRAY = '#888888';

    /**
     * Pick the mesh color for a blob from an identity-resolve result.
     *
     * A resolved person renders in its assigned per-person color (the registry
     * color when set, else the stable hash-derived color from colorForName);
     * an unresolved blob renders in neutral gray. Pure: returns a CSS color
     * string, never throws, tolerates null/missing input. Used by viz3d to
     * color humanoid meshes (bf-3j3s).
     *
     * @param {{identityResolved?:boolean, assignedColor?:string|null}|null} resolved
     *        The result of resolve() (or any object carrying the same fields —
     *        viz3d stores personName/assignedColor/identityResolved directly on
     *        the blob obj, which satisfies this shape).
     * @param {string} [defaultColor] - override for the unresolved default.
     * @returns {string} CSS color string
     */
    function meshColor(resolved, defaultColor) {
        var gray = asColor(defaultColor) || UNIDENTIFIED_GRAY;
        if (resolved && resolved.identityResolved && asColor(resolved.assignedColor)) {
            return resolved.assignedColor;
        }
        return gray;
    }

    /**
     * Resolve identity for a list of blobs. Returns a NEW array of shallow
     * copies with personName/assignedColor/identityResolved populated. Does not
     * mutate inputs. Never throws.
     */
    function resolveAll(blobs, bleDevices) {
        if (!Array.isArray(blobs)) return [];
        return blobs.map(function (b) {
            var id = resolve(b, bleDevices);
            var copy = {};
            if (b && typeof b === 'object') {
                for (var k in b) {
                    if (Object.prototype.hasOwnProperty.call(b, k)) copy[k] = b[k];
                }
            }
            copy.personName = id.personName;
            copy.assignedColor = id.assignedColor;
            copy.identityResolved = id.identityResolved;
            return copy;
        });
    }

    window.BlobIdentity = {
        resolve: resolve,
        resolveAll: resolveAll,
        findDeviceForBlob: findDeviceForBlob,
        colorForName: colorForName,
        meshColor: meshColor,
        isRealName: isRealName,
        UNIDENTIFIED_GRAY: UNIDENTIFIED_GRAY
    };

    if (typeof console !== 'undefined') {
        console.log('[BlobIdentity] blob identity matcher initialized');
    }
})();
