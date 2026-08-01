/**
 * Spaxel Dashboard - Crowd Flow Visualization Layer
 *
 * Manages the crowd flow visualization layers including:
 * - Movement flows (animated arrows)
 * - Dwell hotspots (heatmap)
 * - Corridors (detected pathways)
 *
 * Fetches data from the analytics API and manages layer state.
 */

(function() {
    'use strict';

    // ============================================
    // Layer State
    // ============================================
    const state = {
        flowVisible: false,
        dwellVisible: false,
        corridorVisible: false,
        personFilter: '',      // Empty string = all people
        timeFilter: 'all',      // 'all', '7d', '30d'
        lastRefresh: null,
        autoRefreshMinutes: 5   // Auto-refresh every 5 minutes
    };

    // ============================================
    // API Fetching
    // ============================================

    /**
     * Fetch flow map data from the API.
     * @returns {Promise<Object>} Flow map data
     */
    async function fetchFlowMap() {
        const params = new URLSearchParams();

        if (state.personFilter) {
            params.append('person_id', state.personFilter);
        }

        if (state.timeFilter !== 'all') {
            const now = new Date();
            let since;

            if (state.timeFilter === '7d') {
                since = new Date(now.getTime() - 7 * 24 * 60 * 60 * 1000);
            } else if (state.timeFilter === '30d') {
                since = new Date(now.getTime() - 30 * 24 * 60 * 60 * 1000);
            }

            if (since) {
                params.append('since', since.toISOString());
            }

            const until = now.toISOString();
            params.append('until', until);
        }

        const response = await fetch('/api/analytics/flow?' + params.toString());
        if (!response.ok) {
            throw new Error('Failed to fetch flow map: ' + response.statusText);
        }
        return await response.json();
    }

    /**
     * Fetch dwell heatmap data from the API.
     * @returns {Promise<Object>} Dwell heatmap data
     */
    async function fetchDwellHeatmap() {
        const params = new URLSearchParams();

        if (state.personFilter) {
            params.append('person_id', state.personFilter);
        }

        const response = await fetch('/api/analytics/dwell?' + params.toString());
        if (!response.ok) {
            throw new Error('Failed to fetch dwell heatmap: ' + response.statusText);
        }
        return await response.json();
    }

    /**
     * Fetch corridor data from the API.
     * @returns {Promise<Object>} Corridor data
     */
    async function fetchCorridors() {
        const response = await fetch('/api/analytics/corridors');
        if (!response.ok) {
            throw new Error('Failed to fetch corridors: ' + response.statusText);
        }
        return await response.json();
    }

    /**
     * Refresh all visible layers.
     */
    async function refreshLayers() {
        state.lastRefresh = Date.now();

        const promises = [];

        if (state.flowVisible) {
            promises.push(
                fetchFlowMap()
                    .then(data => Viz3D.setFlowData(data))
                    .catch(err => console.error('[CrowdFlow] Failed to refresh flow:', err))
            );
        }

        if (state.dwellVisible) {
            promises.push(
                fetchDwellHeatmap()
                    .then(data => Viz3D.setDwellData(data))
                    .catch(err => console.error('[CrowdFlow] Failed to refresh dwell:', err))
            );
        }

        if (state.corridorVisible) {
            promises.push(
                fetchCorridors()
                    .then(data => Viz3D.setCorridorData(data.corridors || []))
                    .catch(err => console.error('[CrowdFlow] Failed to refresh corridors:', err))
            );
        }

        await Promise.all(promises);
    }

    // ============================================
    // Layer Controls
    // ============================================

    /**
     * Toggle flow layer visibility.
     * @param {boolean} visible - Whether to show the layer
     */
    async function setFlowVisible(visible) {
        state.flowVisible = visible;
        Viz3D.setFlowLayerVisible(visible);

        if (visible) {
            await refreshLayers();
        }
    }

    /**
     * Toggle dwell layer visibility.
     * @param {boolean} visible - Whether to show the layer
     */
    async function setDwellVisible(visible) {
        state.dwellVisible = visible;
        Viz3D.setDwellLayerVisible(visible);

        if (visible) {
            await refreshLayers();
        }
    }

    /**
     * Toggle corridor layer visibility.
     * @param {boolean} visible - Whether to show the layer
     */
    async function setCorridorVisible(visible) {
        state.corridorVisible = visible;
        Viz3D.setCorridorLayerVisible(visible);

        if (visible) {
            await refreshLayers();
        }
    }

    /**
     * Set person filter for flow/dwell data.
     * @param {string} personId - Person ID or empty string for all
     */
    async function setPersonFilter(personId) {
        if (state.personFilter !== personId) {
            state.personFilter = personId;

            // Update Viz3D filter
            Viz3D.setFlowPersonFilter(personId);

            // Refresh visible layers
            await refreshLayers();
        }
    }

    /**
     * Set time filter for flow data.
     * @param {string} timeFilter - 'all', '7d', or '30d'
     */
    async function setTimeFilter(timeFilter) {
        if (state.timeFilter !== timeFilter) {
            state.timeFilter = timeFilter;

            // Update Viz3D filter
            Viz3D.setFlowTimeFilter(timeFilter);

            // Refresh flow layer if visible
            if (state.flowVisible) {
                await refreshLayers();
            }
        }
    }

    /**
     * Get available people for the person filter dropdown.
     * @returns {Array<{id: string, label: string}>} List of people
     */
    function getAvailablePeople() {
        const people = [];

        // Get people from BLE devices
        if (window.SpaxelState && window.SpaxelState.ble_devices) {
            Object.entries(window.SpaxelState.ble_devices).forEach(([addr, device]) => {
                if (device.label && device.type === 'person') {
                    people.push({
                        id: addr,
                        label: device.label
                    });
                }
            });
        }

        // Add "All people" option at the beginning
        people.unshift({ id: '', label: 'All people' });

        return people;
    }

    /**
     * Populate person filter dropdown.
     */
    function populatePersonFilter() {
        const select = document.getElementById('flow-person-filter');
        if (!select) return;

        // Clear existing options
        select.innerHTML = '';

        // Add people options
        const people = getAvailablePeople();
        people.forEach(person => {
            const option = document.createElement('option');
            option.value = person.id;
            option.textContent = person.label;
            select.appendChild(option);
        });

        // Set current selection
        select.value = state.personFilter;
    }

    // ============================================
    // Auto-Refresh
    // ============================================

    let autoRefreshTimer = null;

    /**
     * Start auto-refresh timer.
     */
    function startAutoRefresh() {
        stopAutoRefresh();

        autoRefreshTimer = setInterval(() => {
            if (state.flowVisible || state.dwellVisible || state.corridorVisible) {
                refreshLayers();
            }
        }, state.autoRefreshMinutes * 60 * 1000);

        console.log('[CrowdFlow] Auto-refresh started (' + state.autoRefreshMinutes + ' min interval)');
    }

    /**
     * Stop auto-refresh timer.
     */
    function stopAutoRefresh() {
        if (autoRefreshTimer) {
            clearInterval(autoRefreshTimer);
            autoRefreshTimer = null;
        }
    }

    // ============================================
    // Initialization
    // ============================================

    /**
     * Initialize the crowd flow module.
     */
    function init() {
        console.log('[CrowdFlow] Initializing crowd flow visualization');

        // Set up event listeners for filter controls
        const personFilter = document.getElementById('flow-person-filter');
        if (personFilter) {
            personFilter.addEventListener('change', (e) => {
                setPersonFilter(e.target.value);
            });
        }

        // Populate person filter dropdown
        populatePersonFilter();

        // Subscribe to BLE device changes to update person filter
        if (window.SpaxelState) {
            window.SpaxelState.subscribe('ble_devices', () => {
                populatePersonFilter();
            });
        }

        // Start auto-refresh
        startAutoRefresh();
    }

    // ============================================
    // Public API
    // ============================================
    window.CrowdFlow = {
        // Initialization
        init: init,

        // Layer controls
        setFlowVisible: setFlowVisible,
        setDwellVisible: setDwellVisible,
        setCorridorVisible: setCorridorVisible,

        // Filters
        setPersonFilter: setPersonFilter,
        setTimeFilter: setTimeFilter,

        // Data fetching
        refreshLayers: refreshLayers,

        // State
        getState: () => ({ ...state }),

        // People management
        getAvailablePeople: getAvailablePeople,
        populatePersonFilter: populatePersonFilter
    };

    // Auto-initialize when DOM is ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }

    console.log('[CrowdFlow] Crowd flow visualization module loaded');
})();
