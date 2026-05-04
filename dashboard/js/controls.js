/**
 * Panel Touch Controls
 *
 * Utility for preventing touch events on sidebar panels from propagating
 * to the Three.js canvas and triggering unintended OrbitControls actions.
 *
 * Usage:
 *   Controls.addPanelTouchBlocker(sidebarElement);
 *   Controls.addPanelTouchBlocker(overlayElement);
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

            element.addEventListener('touchstart', function(e) {
                e.stopPropagation();
            }, { passive: true });

            element.addEventListener('touchmove', function(e) {
                e.stopPropagation();
            }, { passive: false }); // non-passive to allow preventDefault if needed

            element.addEventListener('touchend', function(e) {
                e.stopPropagation();
            }, { passive: true });
        }
    };

    window.Controls = Controls;
})(window);
