/**
 * Spaxel Dashboard - Router
 *
 * Hash-based routing: #live (default), #timeline, #automations, #settings, #ambient, #replay
 * Mode toggle bar in header with active state preserved in localStorage
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
        simple: {
            title: 'Simple',
            icon: '&#x1F3E0;',
            description: 'Card-based mobile-first UI'
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

    const STORAGE_KEY = 'spaxel_active_mode';

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
        // Read saved mode from localStorage
        const savedMode = localStorage.getItem(STORAGE_KEY);
        const hash = window.location.hash.slice(1) || savedMode || 'live';

        // Validate hash against available routes
        if (ROUTES[hash]) {
            currentMode = hash;
        } else {
            currentMode = 'live';
        }

        // Create mode toggle bar
        createModeToggleBar();

        // Set initial mode
        setMode(currentMode, false);

        // Listen for hash changes
        window.addEventListener('hashchange', onHashChange);

        // Listen for popstate (browser back/forward)
        window.addEventListener('popstate', onPopState);

        console.log('[Router] Initialized with mode:', currentMode);
    }

    /**
     * Create the mode toggle bar in the header
     */
    function createModeToggleBar() {
        // Check if bar already exists
        if (document.getElementById('mode-toggle-bar')) {
            return;
        }

        const statusBar = document.getElementById('status-bar');
        if (!statusBar) {
            console.error('[Router] Status bar not found');
            return;
        }

        const bar = document.createElement('div');
        bar.id = 'mode-toggle-bar';
        bar.className = 'mode-toggle-bar';

        // Create buttons for each mode
        Object.keys(ROUTES).forEach(mode => {
            const route = ROUTES[mode];
            const btn = document.createElement('button');
            btn.className = 'mode-toggle-btn';
            btn.dataset.mode = mode;
            btn.innerHTML = route.icon + ' ' + route.title;
            btn.href = '#' + mode;
            btn.addEventListener('click', onModeClick);
            bar.appendChild(btn);
        });

        // Insert bar after status bar
        statusBar.parentNode.insertBefore(bar, statusBar.nextSibling);

        // Adjust scene-container top position to account for mode bar
        const sceneContainer = document.getElementById('scene-container');
        if (sceneContainer) {
            sceneContainer.style.marginTop = '44px';
        }
    }

    /**
     * Handle mode button click
     */
    function onModeClick(e) {
        const mode = e.currentTarget.dataset.mode;
        if (mode !== currentMode) {
            setMode(mode);
        }
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

        // Update active button state
        updateActiveButton();

        // Save to localStorage
        localStorage.setItem(STORAGE_KEY, mode);

        // Handle simple mode visibility
        handleSimpleModeVisibility(mode);

        // Trigger mode change handlers
        notifyModeChange(mode, previousMode);

        console.log('[Router] Mode changed:', previousMode, '->', mode);
    }

    /**
     * Handle simple mode visibility
     * Hides expert mode UI elements when in simple mode
     */
    function handleSimpleModeVisibility(mode) {
        const isSimple = mode === 'simple';
        const statusBar = document.getElementById('status-bar');
        const modeToggleBar = document.getElementById('mode-toggle-bar');
        const sceneContainer = document.getElementById('scene-container');
        const simpleHeader = document.getElementById('simple-mode-header');
        const simpleContent = document.getElementById('simple-mode-content');
        const simpleQuickActions = document.getElementById('simple-quick-actions');

        if (isSimple) {
            // Hide expert mode elements
            if (statusBar) statusBar.style.display = 'none';
            if (modeToggleBar) modeToggleBar.style.display = 'none';
            if (sceneContainer) sceneContainer.style.display = 'none';

            // Show simple mode elements
            if (simpleHeader) simpleHeader.style.display = 'flex';
            if (simpleContent) simpleContent.style.display = 'block';
            if (simpleQuickActions) simpleQuickActions.style.display = 'block';

            // Enable simple mode
            document.body.classList.add('simple-mode');

            // Initialize simple mode if available
            if (window.SpaxelSimpleMode) {
                window.SpaxelSimpleMode.enable();
            }
        } else {
            // Show expert mode elements
            if (statusBar) statusBar.style.display = 'flex';
            if (modeToggleBar) modeToggleBar.style.display = 'flex';
            if (sceneContainer) sceneContainer.style.display = 'block';

            // Hide simple mode elements
            if (simpleHeader) simpleHeader.style.display = 'none';
            if (simpleContent) simpleContent.style.display = 'none';
            if (simpleQuickActions) simpleQuickActions.style.display = 'none';

            // Disable simple mode
            document.body.classList.remove('simple-mode');

            // Disable simple mode module if available
            if (window.SpaxelSimpleMode) {
                window.SpaxelSimpleMode.disable();
            }
        }
    }

    /**
     * Update the active state of mode buttons
     */
    function updateActiveButton() {
        const buttons = document.querySelectorAll('.mode-toggle-btn');
        buttons.forEach(btn => {
            if (btn.dataset.mode === currentMode) {
                btn.classList.add('active');
            } else {
                btn.classList.remove('active');
            }
        });
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
