/**
 * Spaxel Dashboard - State Management
 *
 * Central app state object with subscribe/notify pattern.
 * Separate from WebSocket message parsing.
 */

(function() {
    'use strict';

    // ============================================
    // Central Application State
    // ============================================
    const appState = {
        // Node data
        nodes: {}, // Map of MAC -> { mac, name, pos_x, pos_y, pos_z, role, firmware_version, status, rssi, uptime_s, last_seen, virtual }

        // Blobs (detected people)
        blobs: {}, // Map of blob_id -> { id, x, y, z, confidence, vx, vy, vz, posture, person, ble_device, trails }

        // Zones
        zones: {}, // Map of zone_id -> { id, name, x, y, z, w, d, h, zone_type, occupancy, people:[] }

        // Links (node-to-node connections)
        links: {}, // Map of link_id -> { id, node_mac, peer_mac, delta_rms, snr, phase_stability, quality, weight }

        // Alerts
        alerts: [], // Array of active alerts { id, type, severity, title, message, timestamp_ms, acknowledged }

        // Events (for timeline)
        events: [], // Array of recent events { id, timestamp_ms, type, zone, person, blob_id, detail_json, severity }

        // BLE devices
        ble_devices: {}, // Map of addr -> { addr, label, type, color, icon, auto_rotate, first_seen, last_seen, last_rssi }

        // Triggers (automation)
        triggers: {}, // Map of trigger_id -> { id, name, shape_json, condition, condition_params_json, time_constraint_json, actions_json, enabled, last_fired }

        // Portals
        portals: {}, // Map of portal_id -> { id, name, zone_a_id, zone_b_id, points_json }

        // System info
        system: {
            version: null,
            uptime_s: 0,
            detection_quality: 0,
            confidence: 0,
            security_mode: false,
            nodes_online: 0,
            nodes_total: 0
        },

        // Predictions
        predictions: [], // Array of { person, zone, probability, horizon_min }

        // Connection state
        connection: {
            connected: false,
            connecting: false,
            last_disconnect_time: null
        },

        // Settings
        settings: {
            delta_rms_threshold: 0.02,
            fusion_rate_hz: 10,
            grid_cell_m: 0.2,
            fresnel_decay: 2.0,
            n_subcarriers: 16,
            tau_s: 30,
            breathing_sensitivity: 0.5
        }
    };

    // ============================================
    // Subscriber Registry
    // ============================================
    const subscribers = new Map();

    /**
     * Subscribe to state changes
     * @param {string} key - State key to watch (or '*' for all changes)
     * @param {Function} callback - Callback(newValue, oldValue, key)
     * @returns {Function} Unsubscribe function
     */
    function subscribe(key, callback) {
        if (typeof callback !== 'function') {
            console.error('[State] Callback must be a function');
            return function() {};
        }

        if (!subscribers.has(key)) {
            subscribers.set(key, []);
        }

        const callbacks = subscribers.get(key);
        callbacks.push(callback);

        // Return unsubscribe function
        return function unsubscribe() {
            const callbacks = subscribers.get(key);
            if (callbacks) {
                const index = callbacks.indexOf(callback);
                if (index !== -1) {
                    callbacks.splice(index, 1);
                }
            }
        };
    }

    /**
     * Notify subscribers of a state change
     * @param {string} key - State key that changed
     * @param {*} newValue - New value
     * @param {*} oldValue - Previous value
     */
    function notify(key, newValue, oldValue) {
        // Notify key-specific subscribers
        if (subscribers.has(key)) {
            const callbacks = subscribers.get(key);
            callbacks.forEach(callback => {
                try {
                    callback(newValue, oldValue, key);
                } catch (e) {
                    console.error('[State] Subscriber error for key', key, ':', e);
                }
            });
        }

        // Notify wildcard subscribers
        if (subscribers.has('*')) {
            const callbacks = subscribers.get('*');
            callbacks.forEach(callback => {
                try {
                    callback(newValue, oldValue, key);
                } catch (e) {
                    console.error('[State] Wildcard subscriber error:', e);
                }
            });
        }
    }

    // ============================================
    // State Getters/Setters
    // ============================================

    /**
     * Get a value from state
     * @param {string} key - Dot-notation key (e.g., 'system.version')
     * @returns {*} Value or undefined if not found
     */
    function get(key) {
        const parts = key.split('.');
        let value = appState;

        for (let i = 0; i < parts.length; i++) {
            if (value && typeof value === 'object' && parts[i] in value) {
                value = value[parts[i]];
            } else {
                return undefined;
            }
        }

        return value;
    }

    /**
     * Set a value in state and notify subscribers
     * @param {string} key - Dot-notation key
     * @param {*} value - New value
     */
    function set(key, value) {
        const oldValue = get(key);

        const parts = key.split('.');
        let obj = appState;

        for (let i = 0; i < parts.length - 1; i++) {
            const part = parts[i];
            if (!(part in obj) || typeof obj[part] !== 'object') {
                obj[part] = {};
            }
            obj = obj[part];
        }

        const lastPart = parts[parts.length - 1];
        obj[lastPart] = value;

        notify(key, value, oldValue);
    }

    /**
     * Update a nested object in state
     * @param {string} key - Dot-notation key to the object
     * @param {Object} updates - Object with keys to update
     */
    function update(key, updates) {
        const obj = get(key);
        if (!obj || typeof obj !== 'object') {
            console.error('[State] Cannot update non-object at key:', key);
            return;
        }

        const oldObj = { ...obj };
        Object.assign(obj, updates);

        notify(key, obj, oldObj);
    }

    /**
     * Get the entire state (use carefully - for debugging)
     */
    function getState() {
        return appState;
    }

    /**
     * Reset state to initial values
     * @param {string} key - Optional dot-notation key to reset (resets all if omitted)
     */
    function reset(key) {
        if (key) {
            const parts = key.split('.');
            let obj = appState;

            for (let i = 0; i < parts.length - 1; i++) {
                obj = obj[parts[i]];
                if (!obj) return;
            }

            const lastPart = parts[parts.length - 1];
            const oldValue = obj[lastPart];

            // Reset to appropriate default based on type
            if (Array.isArray(oldValue)) {
                obj[lastPart] = [];
            } else if (typeof oldValue === 'object' && oldValue !== null) {
                obj[lastPart] = {};
            } else {
                obj[lastPart] = null;
            }

            notify(key, obj[lastPart], oldValue);
        } else {
            // Reset entire state (not commonly used)
            Object.keys(appState).forEach(k => {
                if (Array.isArray(appState[k])) {
                    appState[k] = [];
                } else if (typeof appState[k] === 'object' && appState[k] !== null) {
                    appState[k] = {};
                } else {
                    appState[k] = null;
                }
            });
            notify('*', null, null);
        }
    }

    // ============================================
    // Convenience Methods for Common State
    // ============================================

    /**
     * Update a node's state
     */
    function updateNode(mac, updates) {
        if (!appState.nodes[mac]) {
            appState.nodes[mac] = { mac: mac };
        }
        Object.assign(appState.nodes[mac], updates);
        notify('nodes.' + mac, appState.nodes[mac], null);
        notify('nodes', appState.nodes, null);
    }

    /**
     * Remove a node
     */
    function removeNode(mac) {
        const oldValue = appState.nodes[mac];
        delete appState.nodes[mac];
        notify('nodes.' + mac, null, oldValue);
        notify('nodes', appState.nodes, null);
    }

    /**
     * Update a blob's state
     */
    function updateBlob(id, updates) {
        if (!appState.blobs[id]) {
            appState.blobs[id] = { id: id };
        }
        Object.assign(appState.blobs[id], updates);
        notify('blobs.' + id, appState.blobs[id], null);
        notify('blobs', appState.blobs, null);
    }

    /**
     * Remove a blob
     */
    function removeBlob(id) {
        const oldValue = appState.blobs[id];
        delete appState.blobs[id];
        notify('blobs.' + id, null, oldValue);
        notify('blobs', appState.blobs, null);
    }

    /**
     * Add an event to the timeline
     */
    function addEvent(event) {
        if (!event.id) {
            event.id = 'evt_' + Date.now() + '_' + Math.random().toString(36).substr(2, 9);
        }
        if (!event.timestamp_ms) {
            event.timestamp_ms = Date.now();
        }
        appState.events.unshift(event);

        // Keep only last 1000 events in memory
        if (appState.events.length > 1000) {
            appState.events = appState.events.slice(0, 1000);
        }

        notify('events', appState.events, null);
    }

    /**
     * Add an alert
     */
    function addAlert(alert) {
        if (!alert.id) {
            alert.id = 'alert_' + Date.now() + '_' + Math.random().toString(36).substr(2, 9);
        }
        if (!alert.timestamp_ms) {
            alert.timestamp_ms = Date.now();
        }
        if (!alert.acknowledged) {
            alert.acknowledged = false;
        }

        appState.alerts.push(alert);
        notify('alerts', appState.alerts, null);

        return alert.id;
    }

    /**
     * Acknowledge an alert
     */
    function acknowledgeAlert(alertId) {
        const alert = appState.alerts.find(a => a.id === alertId);
        if (alert) {
            alert.acknowledged = true;
            notify('alerts', appState.alerts, null);
        }
    }

    /**
     * Remove an alert
     */
    function removeAlert(alertId) {
        const index = appState.alerts.findIndex(a => a.id === alertId);
        if (index !== -1) {
            const removed = appState.alerts.splice(index, 1)[0];
            notify('alerts', appState.alerts, null);
            return removed;
        }
    }

    /**
     * Update connection state
     */
    function setConnectionState(state) {
        const oldConnected = appState.connection.connected;

        if (state.connected !== undefined) {
            appState.connection.connected = state.connected;
        }
        if (state.connecting !== undefined) {
            appState.connection.connecting = state.connecting;
        }
        if (state.last_disconnect_time !== undefined) {
            appState.connection.last_disconnect_time = state.last_disconnect_time;
        }

        notify('connection', appState.connection, { connected: oldConnected });

        // Track disconnect time for stale detection
        if (state.connected === false && oldConnected === true) {
            appState.connection.last_disconnect_time = Date.now();
        }
    }

    /**
     * Update system info
     */
    function updateSystem(updates) {
        const oldSystem = { ...appState.system };
        Object.assign(appState.system, updates);
        notify('system', appState.system, oldSystem);
    }

    // ============================================
    // Public API
    // ============================================
    window.SpaxelState = {
        // Core methods
        get: get,
        set: set,
        update: update,
        getState: getState,
        reset: reset,

        // Subscription
        subscribe: subscribe,

        // Convenience methods
        updateNode: updateNode,
        removeNode: removeNode,
        updateBlob: updateBlob,
        removeBlob: removeBlob,
        addEvent: addEvent,
        addAlert: addAlert,
        acknowledgeAlert: acknowledgeAlert,
        removeAlert: removeAlert,
        setConnectionState: setConnectionState,
        updateSystem: updateSystem,

        // Direct access to state (read-only preferred)
        nodes: appState.nodes,
        blobs: appState.blobs,
        zones: appState.zones,
        links: appState.links,
        alerts: appState.alerts,
        events: appState.events,
        ble_devices: appState.ble_devices,
        triggers: appState.triggers,
        portals: appState.portals,
        system: appState.system,
        predictions: appState.predictions,
        connection: appState.connection,
        settings: appState.settings
    };

    console.log('[State] State management initialized');
})();
