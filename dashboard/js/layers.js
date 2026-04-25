/**
 * Spaxel Dashboard - Layer Management Module
 *
 * Centralized management for 3D scene overlay layers.
 * Provides toggle state tracking and event dispatch for layer changes.
 *
 * Layers:
 *   - Links: TX→RX link lines
 *   - Fresnel Zones: Wireframe ellipsoids for active links (debug)
 *   - Trails: Blob footprint trails
 *   - Zones: Zone box meshes and labels
 *   - Portals: Portal connection meshes
 *   - Coverage: GDOP quality overlay
 *   - Crowd Flow: Trajectory flow arrows
 */

(function() {
    'use strict';

    var LAYERS = {
        LINKS:        'links',
        FRESNEL:      'fresnel',
        TRAILS:       'trails',
        ZONES:        'zones',
        PORTALS:      'portals',
        COVERAGE:     'coverage',
        CROWD_FLOW:   'crowd_flow'
    };

    // Default visibility state
    var _state = {};
    _state[LAYERS.LINKS]      = true;
    _state[LAYERS.FRESNEL]    = false;  // off by default
    _state[LAYERS.TRAILS]     = true;
    _state[LAYERS.ZONES]      = true;
    _state[LAYERS.PORTALS]    = true;
    _state[LAYERS.COVERAGE]   = false;
    _state[LAYERS.CROWD_FLOW] = false;

    // Listener registry: layerName -> [callback, ...]
    var _listeners = {};

    /**
     * Register a callback for layer visibility changes.
     * @param {string} layer - Layer name (use LAYERS constants)
     * @param {function(visible: boolean)} callback
     */
    function onLayerChange(layer, callback) {
        if (!_listeners[layer]) _listeners[layer] = [];
        _listeners[layer].push(callback);
    }

    /**
     * Remove a previously registered callback.
     * @param {string} layer
     * @param {function} callback
     */
    function offLayerChange(layer, callback) {
        if (!_listeners[layer]) return;
        _listeners[layer] = _listeners[layer].filter(function(fn) {
            return fn !== callback;
        });
    }

    /**
     * Set layer visibility and notify listeners.
     * @param {string} layer - Layer name
     * @param {boolean} visible
     */
    function setLayerVisible(layer, visible) {
        var prev = _state[layer];
        _state[layer] = visible;

        if (prev !== visible && _listeners[layer]) {
            _listeners[layer].forEach(function(cb) {
                try { cb(visible); } catch (e) {
                    console.error('[Layers] Listener error for ' + layer + ':', e);
                }
            });
        }
    }

    /**
     * Toggle layer visibility.
     * @param {string} layer
     */
    function toggleLayer(layer) {
        setLayerVisible(layer, !_state[layer]);
    }

    /**
     * Get current visibility of a layer.
     * @param {string} layer
     * @returns {boolean}
     */
    function isVisible(layer) {
        return !!_state[layer];
    }

    /**
     * Sync a checkbox DOM element with layer state.
     * Sets up bidirectional binding: checkbox changes -> layer state -> checkbox updates.
     * @param {string} layer
     * @param {HTMLInputElement} checkbox
     */
    function bindCheckbox(layer, checkbox) {
        if (!checkbox) return;

        // Sync initial state
        checkbox.checked = _state[layer];

        // Checkbox -> layer
        checkbox.addEventListener('change', function() {
            setLayerVisible(layer, checkbox.checked);
        });

        // Layer -> checkbox
        onLayerChange(layer, function(visible) {
            checkbox.checked = visible;
        });
    }

    /**
     * Sync a button DOM element with layer state (active class toggling).
     * @param {string} layer
     * @param {HTMLElement} button
     */
    function bindButton(layer, button) {
        if (!button) return;

        // Sync initial state
        if (_state[layer]) {
            button.classList.add('active');
        } else {
            button.classList.remove('active');
        }

        // Button -> layer
        button.addEventListener('click', function() {
            toggleLayer(layer);
        });

        // Layer -> button
        onLayerChange(layer, function(visible) {
            if (visible) {
                button.classList.add('active');
            } else {
                button.classList.remove('active');
            }
        });
    }

    // Public API
    window.Layers = {
        LAYERS: LAYERS,
        onLayerChange: onLayerChange,
        offLayerChange: offLayerChange,
        setLayerVisible: setLayerVisible,
        toggleLayer: toggleLayer,
        isVisible: isVisible,
        bindCheckbox: bindCheckbox,
        bindButton: bindButton
    };

    console.log('[Layers] Module loaded');
})();
