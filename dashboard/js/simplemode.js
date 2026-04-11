/**
 * Spaxel Dashboard - Simple Mode Detection
 *
 * Auto-detection of simple mode based on:
 * 1. Screen width < 768px (phones, small tablets in portrait)
 * 2. User-agent contains "Mobile" (additional phone detection signal)
 * 3. User has previously selected simple mode (localStorage "spaxel_mode" = "simple")
 *
 * Expert mode is the default for desktop browsers.
 */

(function() {
    'use strict';

    // ============================================
    // Configuration
    // ============================================
    const STORAGE_KEY = 'spaxel_mode';
    const MOBILE_BREAKPOINT = 768; // pixels
    const MOBILE_USER_AGENT_REGEX = /Mobile|Android|iPhone|iPad|iPod/i;

    // ============================================
    // State
    // ============================================
    let currentMode = null; // 'simple' or 'expert'
    let autoDetectionEnabled = true;

    // ============================================
    // Detection Functions
    // ============================================

    /**
     * Detect if the current device is a mobile device
     * @returns {boolean} true if mobile device detected
     */
    function isMobileDevice() {
        // Check user agent
        if (MOBILE_USER_AGENT_REGEX.test(navigator.userAgent)) {
            return true;
        }

        // Check screen width
        if (window.innerWidth < MOBILE_BREAKPOINT) {
            return true;
        }

        // Check touch capability (most mobile devices have touch)
        if ('ontouchstart' in window || navigator.maxTouchPoints > 0) {
            // But only consider it mobile if screen is also small
            // (this avoids flagging large touch-screen laptops as mobile)
            if (window.innerWidth < 1024) {
                return true;
            }
        }

        return false;
    }

    /**
     * Determine the default mode based on device detection
     * @returns {string} 'simple' for mobile, 'expert' for desktop
     */
    function getDetectedMode() {
        if (isMobileDevice()) {
            return 'simple';
        }
        return 'expert';
    }

    /**
     * Get the current mode, with auto-detection
     * @returns {string} 'simple' or 'expert'
     */
    function getMode() {
        // If user has explicitly set a preference, use it
        const savedMode = localStorage.getItem(STORAGE_KEY);
        if (savedMode === 'simple' || savedMode === 'expert') {
            return savedMode;
        }

        // Otherwise, use auto-detection
        if (autoDetectionEnabled) {
            return getDetectedMode();
        }

        // Default to expert mode
        return 'expert';
    }

    /**
     * Set the current mode
     * @param {string} mode - 'simple' or 'expert'
     * @param {boolean} savePreference - Whether to save to localStorage (default: true)
     */
    function setMode(mode, savePreference = true) {
        if (mode !== 'simple' && mode !== 'expert') {
            console.error('[SimpleMode] Invalid mode:', mode);
            return;
        }

        const previousMode = currentMode;
        currentMode = mode;

        // Save to localStorage if requested
        if (savePreference) {
            localStorage.setItem(STORAGE_KEY, mode);
        }

        // Apply mode to document
        applyMode(mode);

        // Notify listeners of mode change
        if (previousMode !== mode) {
            notifyModeChange(mode, previousMode);
        }

        console.log('[SimpleMode] Mode set to:', mode);
    }

    /**
     * Apply mode to document (CSS classes, visibility)
     * @param {string} mode - 'simple' or 'expert'
     */
    function applyMode(mode) {
        const isSimple = mode === 'simple';

        // Update body class
        if (isSimple) {
            document.body.classList.add('simple-mode');
            document.body.classList.remove('expert-mode');
        } else {
            document.body.classList.remove('simple-mode');
            document.body.classList.add('expert-mode');
        }

        // Update simple mode UI elements
        const header = document.getElementById('simple-mode-header');
        const content = document.getElementById('simple-mode-content');
        const quickActions = document.getElementById('simple-quick-actions');

        if (header) {
            header.style.display = isSimple ? 'flex' : 'none';
        }

        if (content) {
            content.style.display = isSimple ? 'block' : 'none';
        }

        if (quickActions) {
            quickActions.style.display = isSimple ? 'block' : 'none';
        }

        // Update toggle button states
        document.querySelectorAll('.mode-toggle-btn').forEach(btn => {
            btn.classList.toggle('active', btn.dataset.mode === mode);
        });

        // Apply night mode based on quiet hours
        updateNightMode();
    }

    /**
     * Enable or disable auto-detection
     * @param {boolean} enabled - Whether to enable auto-detection
     */
    function setAutoDetection(enabled) {
        autoDetectionEnabled = enabled;
        console.log('[SimpleMode] Auto-detection:', enabled ? 'enabled' : 'disabled');
    }

    // ============================================
    // Night Mode (OLED Dark)
    // ============================================

    /**
     * Night mode configuration (in hours)
     * Default: 10pm to 7am
     */
    const nightModeConfig = {
        startHour: 22, // 10pm
        endHour: 7    // 7am
    };

    /**
     * Check if we're currently in the night mode window
     * @returns {boolean} true if within night mode hours
     */
    function isNightTime() {
        const now = new Date();
        const hour = now.getHours();

        if (nightModeConfig.startHour < nightModeConfig.endHour) {
            // Night mode doesn't cross midnight (e.g., 2am-6am)
            return hour >= nightModeConfig.startHour && hour < nightModeConfig.endHour;
        } else {
            // Night mode crosses midnight (e.g., 10pm-7am)
            return hour >= nightModeConfig.startHour || hour < nightModeConfig.endHour;
        }
    }

    /**
     * Update night mode based on time of day and simple mode state
     */
    function updateNightMode() {
        const isSimple = document.body.classList.contains('simple-mode');
        const isNight = isNightTime();

        if (isSimple && isNight) {
            document.body.classList.add('night-mode');
            document.body.classList.add('oled-night');
        } else {
            document.body.classList.remove('night-mode');
            document.body.classList.remove('oled-night');
        }

        // Also check for prefers-color-scheme dark as fallback
        if (isSimple && !isNight && window.matchMedia('(prefers-color-scheme: dark)').matches) {
            document.body.classList.add('night-mode');
            document.body.classList.add('oled-night');
        }
    }

    /**
     * Set night mode configuration
     * @param {number} startHour - Start hour (0-23)
     * @param {number} endHour - End hour (0-23)
     */
    function setNightModeHours(startHour, endHour) {
        nightModeConfig.startHour = startHour;
        nightModeConfig.endHour = endHour;
        updateNightMode();
        console.log('[SimpleMode] Night mode hours updated:', startHour, '-', endHour);
    }

    // ============================================
    // Mode Change Listeners
    // ============================================
    const modeChangeListeners = [];

    function notifyModeChange(newMode, oldMode) {
        modeChangeListeners.forEach(listener => {
            try {
                listener(newMode, oldMode);
            } catch (e) {
                console.error('[SimpleMode] Mode change listener error:', e);
            }
        });
    }

    /**
     * Register a callback for mode changes
     * @param {Function} listener - Callback(newMode, oldMode)
     */
    function onModeChange(listener) {
        if (typeof listener === 'function') {
            modeChangeListeners.push(listener);
        }
    }

    // ============================================
    // Initialization
    // ============================================

    /**
     * Initialize simple mode detection
     */
    function init() {
        // Determine initial mode
        currentMode = getMode();
        console.log('[SimpleMode] Detected mode:', currentMode);

        // Apply the mode
        applyMode(currentMode);

        // Set up event listeners for mode toggle buttons
        document.querySelectorAll('.mode-toggle-btn').forEach(btn => {
            btn.addEventListener('click', onModeToggleClick);
        });

        // Set up quick action buttons
        document.querySelectorAll('.quick-action-btn').forEach(btn => {
            btn.addEventListener('click', onQuickActionClick);
        });

        // Update night mode every minute (in case time crosses threshold)
        setInterval(updateNightMode, 60000);

        // Listen for window resize to update mode if auto-detection is enabled
        let resizeTimer;
        window.addEventListener('resize', () => {
            clearTimeout(resizeTimer);
            resizeTimer = setTimeout(() => {
                if (autoDetectionEnabled && !localStorage.getItem(STORAGE_KEY)) {
                    const newMode = getDetectedMode();
                    if (newMode !== currentMode) {
                        setMode(newMode, false); // Don't save auto-detected changes
                    }
                }
            }, 250); // Debounce resize events
        });

        console.log('[SimpleMode] Initialized');
    }

    /**
     * Handle mode toggle button click
     */
    function onModeToggleClick(e) {
        const newMode = e.currentTarget.dataset.mode;
        setMode(newMode, true); // Save user preference
    }

    /**
     * Handle quick action button click
     */
    function onQuickActionClick(e) {
        const action = e.currentTarget.dataset.action;
        console.log('[SimpleMode] Quick action:', action);

        // Dispatch custom event for other modules to handle
        const event = new CustomEvent('simplemode-action', {
            detail: { action: action }
        });
        document.dispatchEvent(event);
    }

    // ============================================
    // Public API
    // ============================================

    window.SpaxelSimpleModeDetection = {
        init: init,
        getMode: getMode,
        setMode: setMode,
        isMobileDevice: isMobileDevice,
        setAutoDetection: setAutoDetection,
        setNightModeHours: setNightModeHours,
        onModeChange: onModeChange,
        isNightTime: isNightTime,
        updateNightMode: updateNightMode
    };

    // Auto-initialize when DOM is ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }

    console.log('[SimpleMode] Detection module loaded');
})();
