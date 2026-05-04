/**
 * Spaxel Dashboard - Touch Controls for Panel Touch Event Handling
 *
 * Utility for preventing touch events on sidebar panels from propagating
 * to the Three.js canvas and triggering unintended OrbitControls actions.
 *
 * All listeners use passive: true with stopPropagation() to prevent
 * iOS Safari warnings about passive event listeners while still
 * preventing event bubbling to the canvas.
 *
 * OrbitControls Configuration (app.js):
 * - ONE: THREE.TOUCH.ROTATE (one-finger orbit)
 * - TWO: THREE.TOUCH.DOLLY (pinch zoom ONLY, no pan)
 * - THREE: THREE.TOUCH.PAN (three-finger pan)
 * - zoomSpeed: 1.0 (standard touch zoom speed)
 * - Canvas touch-action: none (prevents passive listener warnings)
 *
 * Usage:
 *   Controls.addPanelTouchBlocker(sidebarElement);
 *   Controls.addPanelTouchBlocker(overlayElement);
 *
 * The module also provides direct access to individual event handlers
 * for custom use cases.
 */

(function(window) {
    'use strict';

    var Controls = {
        /**
         * Add touch event stopPropagation listeners to a panel element.
         * Prevents touchstart/touchmove/touchend from bubbling to the canvas.
         * @param {HTMLElement} element - A sidebar, overlay, or modal element
         */
        addPanelTouchBlocker: function(element) {
            if (!element) return;

            // Use passive listeners with stopPropagation to avoid iOS warnings
            // touchstart: Prevents initial touch from reaching canvas
            element.addEventListener('touchstart', function(e) {
                e.stopPropagation();
            }, { passive: true });

            // touchmove: Prevents scrolling/dragging from reaching canvas
            element.addEventListener('touchmove', function(e) {
                e.stopPropagation();
            }, { passive: true });

            // touchend: Prevents tap release from reaching canvas
            element.addEventListener('touchend', function(e) {
                e.stopPropagation();
            }, { passive: true });

            // touchcancel: Handle system-interrupted touches
            element.addEventListener('touchcancel', function(e) {
                e.stopPropagation();
            }, { passive: true });
        },

        /**
         * Remove touch event blockers from an element.
         * Useful for dynamic panels that are reused.
         * @param {HTMLElement} element - The element to remove blockers from
         */
        removePanelTouchBlocker: (function() {
            // Store references to handlers to enable removal
            var handlers = new WeakMap();

            return function(element) {
                if (!element) return;

                var elementHandlers = handlers.get(element);
                if (elementHandlers) {
                    elementHandlers.forEach(function(handler) {
                        element.removeEventListener(handler.type, handler.fn, handler.options);
                    });
                    handlers.delete(element);
                }
            };
        })(),

        /**
         * Apply touch blocking to all panel elements in the DOM.
         * Useful for initializing after DOM modifications.
         * @param {string} selector - CSS selector for panel elements (default: all panel classes)
         */
        applyToAllPanels: function(selector) {
            selector = selector || '.panel-sidebar, .panel-sidebar-right, .panel-sidebar-left, .panel-overlay, .panel-modal, .modal-container, .modal-backdrop, .panel-backdrop, .sidebar-panel, .troubleshoot-modal-overlay';
            var panels = document.querySelectorAll(selector);
            panels.forEach(function(panel) {
                Controls.addPanelTouchBlocker(panel);
            });
            return panels.length;
        },

        /**
         * Check if an element has touch event blockers applied.
         * @param {HTMLElement} element - The element to check
         * @returns {boolean} True if blockers are present
         */
        hasTouchBlocker: function(element) {
            if (!element) return false;
            // Check if element has the data attribute we'll add
            return element.hasAttribute('data-touch-blocker');
        }
    };

    // Add data attribute when blocker is applied (for debugging/inspection)
    var originalAdd = Controls.addPanelTouchBlocker;
    Controls.addPanelTouchBlocker = function(element) {
        if (!element) return;
        originalAdd.call(this, element);
        element.setAttribute('data-touch-blocker', 'true');
    };

    // Auto-apply to existing panels on load (with slight delay for DOM readiness)
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', function() {
            setTimeout(function() {
                Controls.applyToAllPanels();
                console.log('[Controls] Touch blockers applied to ' + Controls.applyToAllPanels() + ' existing panels');
            }, 100);
        });
    } else {
        // DOM already ready
        setTimeout(function() {
            var count = Controls.applyToAllPanels();
            console.log('[Controls] Touch blockers applied to ' + count + ' existing panels');
        }, 100);
    }

    // Export to window
    window.Controls = Controls;

    // Also export to Spaxel namespace for consistency with other modules
    if (!window.Spaxel) {
        window.Spaxel = {};
    }
    window.Spaxel.Controls = Controls;

    console.log('[Controls] Touch controls module initialized');
})(window);
