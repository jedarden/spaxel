/**
 * Spaxel Dashboard - Explain Mode Entry Point
 *
 * Bridges right-click / long-press (300ms) blob interactions to the
 * Explainability module. Delegates all heavy lifting (sidebar panel,
 * X-ray overlay, Fresnel zones, sparklines) to Explainability.
 *
 * Also handles Escape key exit and click-outside-to-close.
 *
 * @module Explain
 */

(function () {
    'use strict';

    var LONG_PRESS_MS = 300;
    var _longPressTimer = null;
    var _longPressBlobID = null;

    // ── Public API ─────────────────────────────────────────────────────────────

    window.Explain = {
        /**
         * Open explain mode for the given blob.
         * Called from Viz3D context menu ("Why is this here?").
         *
         * @param {number} blobID
         */
        open: function (blobID) {
            if (window.Explainability) {
                Explainability.explain(blobID);
            }
        },

        /**
         * Close explain mode and restore the scene.
         */
        close: function () {
            if (window.Explainability) {
                Explainability.close();
            }
        },

        /**
         * Check if explain mode is active.
         * @returns {boolean}
         */
        isActive: function () {
            return window.Explainability ? Explainability.isActive() : false;
        },

        /**
         * Start a long-press timer for mobile explain activation.
         * If the press is held for 300ms without moving, explain mode opens.
         *
         * @param {number} blobID
         * @param {Event} event - touchstart event (for cancelling)
         */
        startLongPress: function (blobID, event) {
            _longPressBlobID = blobID;
            _longPressTimer = setTimeout(function () {
                _longPressTimer = null;
                window.Explain.open(blobID);
            }, LONG_PRESS_MS);
        },

        /**
         * Cancel a pending long-press timer (on touchmove or touchend).
         */
        cancelLongPress: function () {
            if (_longPressTimer !== null) {
                clearTimeout(_longPressTimer);
                _longPressTimer = null;
            }
            _longPressBlobID = null;
        },

        /**
         * Handle a blob_explain WebSocket message.
         * Routes to Explainability.handleExplainSnapshot.
         *
         * @param {number} blobID
         * @param {Object} snapshot
         */
        handleSnapshot: function (blobID, snapshot) {
            if (window.Explainability) {
                Explainability.handleExplainSnapshot(blobID, snapshot);
            }
        }
    };

    console.log('[Explain] Module loaded');
})();
