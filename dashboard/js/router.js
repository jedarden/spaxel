/**
 * Spaxel Dashboard - Router
 *
 * Hash-based routing: #live (default), #timeline, #automations, #settings, #ambient, #replay
 */

(function() {
    'use strict';

    // ============================================
    // Configuration
    // ============================================
    const ROUTES = {
        live: {
            title: 'Live',
            icon: '&#x25A0;',
            description: 'Real-time 3D detection view'
        },
        timeline: {
            title: 'Timeline',
            icon: '&#x231A;',
            description: 'Activity history and events'
        },
        automations: {
            title: 'Automations',
            icon: '&#x2699;',
            description: 'Spatial triggers and automation rules'
        },
        settings: {
            title: 'Settings',
            icon: '&#x2699;',
            description: 'System configuration and preferences'
        },
        ambient: {
            title: 'Ambient',
            icon: '&#x25C9;',
            description: 'Always-on display mode'
        },
        replay: {
            title: 'Replay',
            icon: '&#x23F5;',
            description: 'Time-travel debugging mode'
        },
        simulate: {
            title: 'Simulate',
            icon: '&#x269B;',
            description: 'Pre-deployment simulator'
        }
    };

    // ============================================
    // State
    // ============================================
    let currentMode = 'live';
    let previousMode = null;
    let modeChangeHandlers = [];

    // ============================================
    // Router Core
    // ============================================

    /**
     * Initialize the router
     */
    function init() {
        // Clean up legacy localStorage key
        localStorage.removeItem('spaxel_active_mode');

        const hash = window.location.hash.slice(1) || 'live';

        // Validate hash against available routes
        if (ROUTES[hash]) {
            currentMode = hash;
        } else {
            currentMode = 'live';
        }

        // Set initial mode
        setMode(currentMode, false);

        // Listen for hash changes
        window.addEventListener('hashchange', onHashChange);

        // Listen for popstate (browser back/forward)
        window.addEventListener('popstate', onPopState);

        console.log('[Router] Initialized with mode:', currentMode);
    }

    /**
     * Handle hash change from browser navigation
     */
    function onHashChange() {
        const hash = window.location.hash.slice(1) || 'live';
        if (hash !== currentMode && ROUTES[hash]) {
            setMode(hash);
        }
    }

    /**
     * Handle popstate (browser back/forward)
     */
    function onPopState() {
        const hash = window.location.hash.slice(1) || 'live';
        if (hash !== currentMode && ROUTES[hash]) {
            setMode(hash);
        }
    }

    /**
     * Set the current mode
     * @param {string} mode - Mode identifier
     * @param {boolean} updateHash - Whether to update the URL hash (default: true)
     */
    function setMode(mode, updateHash = true) {
        if (!ROUTES[mode]) {
            console.error('[Router] Unknown mode:', mode);
            return;
        }

        previousMode = currentMode;
        currentMode = mode;

        // Update hash without triggering hashchange event
        if (updateHash) {
            history.replaceState({ mode: mode }, '', '#' + mode);
        }

        // Trigger mode change handlers
        notifyModeChange(mode, previousMode);

        console.log('[Router] Mode changed:', previousMode, '->', mode);
    }

    /**
     * Notify all registered mode change handlers
     */
    function notifyModeChange(newMode, oldMode) {
        modeChangeHandlers.forEach(handler => {
            try {
                handler(newMode, oldMode);
            } catch (e) {
                console.error('[Router] Mode change handler error:', e);
            }
        });
    }

    // ============================================
    // Public API
    // ============================================

    /**
     * Register a handler for mode changes
     * @param {Function} handler - Callback(newMode, oldMode)
     */
    function onModeChange(handler) {
        if (typeof handler === 'function') {
            modeChangeHandlers.push(handler);
        }
    }

    /**
     * Get the current mode
     */
    function getMode() {
        return currentMode;
    }

    /**
     * Get the previous mode
     */
    function getPreviousMode() {
        return previousMode;
    }

    /**
     * Navigate to a specific mode
     */
    function navigate(mode) {
        if (ROUTES[mode]) {
            setMode(mode);
        }
    }

    /**
     * Check if a specific mode is active
     */
    function isMode(mode) {
        return currentMode === mode;
    }

    /**
     * Get all available routes
     */
    function getRoutes() {
        return { ...ROUTES };
    }

    // ============================================
    // Export
    // ============================================
    window.SpaxelRouter = {
        init: init,
        onModeChange: onModeChange,
        getMode: getMode,
        getPreviousMode: getPreviousMode,
        navigate: navigate,
        isMode: isMode,
        getRoutes: getRoutes
    };

    // Auto-initialize when DOM is ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }

    console.log('[Router] Router module loaded');
})();
