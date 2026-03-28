/**
 * Spaxel First-Time Feature Tooltips
 *
 * Shows contextual tooltips on first dashboard open after a node is added.
 * Each tooltip is shown once and tracked via localStorage flags.
 *
 * TooltipManager.show(tooltipId, targetSelector, text, direction)
 * TooltipManager.showSequence() — walks through the manifest automatically
 */

(function () {
    'use strict';

    // ============================================
    // Configuration
    // ============================================
    var STORAGE_PREFIX = 'spaxel_tooltip_';
    var DISMISS_MS = 8000;        // auto-dismiss after 8 s
    var SEQUENCE_GAP_MS = 2000;   // pause between sequential tooltips

    var TOOLTIP_MANIFEST = [
        {
            id: 'csi-chart',
            target: '#chart-panel',
            text: 'This is your live signal. Motion causes the waves to change.',
            direction: 'left',
        },
        {
            id: '3d-view',
            target: '#scene-container',
            text: 'This 3D space updates as people move around.',
            direction: 'top',
        },
        {
            id: 'presence-indicator',
            target: '#presence-indicator',
            text: 'Green = no one detected. Red = motion detected.',
            direction: 'bottom',
        },
        {
            id: 'link-list',
            target: '.link-section',
            text: 'Each line between two nodes is a sensing link.',
            direction: 'left',
        },
    ];

    // ============================================
    // Internal State
    // ============================================
    var state = {
        activeTooltip: null,
        dismissTimer: null,
        sequenceTimer: null,
        sequenceIndex: 0,
    };

    // ============================================
    // localStorage Helpers
    // ============================================
    function hasShown(id) {
        try {
            return localStorage.getItem(STORAGE_PREFIX + id + '_shown') === 'true';
        } catch (e) {
            return false;
        }
    }

    function markShown(id) {
        try {
            localStorage.setItem(STORAGE_PREFIX + id + '_shown', 'true');
        } catch (e) { /* ignore */ }
    }

    function shouldShowSequence() {
        try {
            return localStorage.getItem('spaxel_tooltips_shown') !== 'true';
        } catch (e) {
            return true;
        }
    }

    function markAllShown() {
        try {
            localStorage.setItem('spaxel_tooltips_shown', 'true');
        } catch (e) { /* ignore */ }
    }

    // ============================================
    // Tooltip Rendering
    // ============================================
    function show(tooltipId, targetSelector, text, direction) {
        if (hasShown(tooltipId)) return false;

        dismiss();

        var target = document.querySelector(targetSelector);
        if (!target) return false;

        var tooltip = document.createElement('div');
        tooltip.className = 'spaxel-tooltip';
        tooltip.id = 'spaxel-tooltip-' + tooltipId;
        tooltip.innerHTML =
            '<div class="spaxel-tooltip-text">' + escapeHTML(text) + '</div>' +
            '<div class="spaxel-tooltip-arrow spaxel-tooltip-arrow-' + (direction || 'top') + '"></div>';

        // Append first so we can measure dimensions
        tooltip.style.visibility = 'hidden';
        document.body.appendChild(tooltip);

        var rect = target.getBoundingClientRect();
        var tipRect = tooltip.getBoundingClientRect();
        var top, left, transform;

        switch (direction) {
            case 'bottom':
                top = rect.bottom + 10;
                left = rect.left + rect.width / 2 - tipRect.width / 2;
                transform = '';
                break;
            case 'left':
                top = rect.top + rect.height / 2 - tipRect.height / 2;
                left = rect.left - tipRect.width - 10;
                transform = '';
                break;
            case 'right':
                top = rect.top + rect.height / 2 - tipRect.height / 2;
                left = rect.right + 10;
                transform = '';
                break;
            default: // top
                top = rect.top - tipRect.height - 10;
                left = rect.left + rect.width / 2 - tipRect.width / 2;
                transform = '';
                break;
        }

        // Clamp to viewport
        var vpW = window.innerWidth;
        var vpH = window.innerHeight;
        left = Math.max(8, Math.min(left, vpW - tipRect.width - 8));
        top = Math.max(8, Math.min(top, vpH - tipRect.height - 8));

        tooltip.style.top = top + 'px';
        tooltip.style.left = left + 'px';
        tooltip.style.transform = transform;
        tooltip.style.visibility = 'visible';

        state.activeTooltip = tooltip;

        // Auto-dismiss
        state.dismissTimer = setTimeout(function () {
            markShown(tooltipId);
            dismiss();
        }, DISMISS_MS);

        return true;
    }

    function dismiss() {
        if (state.dismissTimer) {
            clearTimeout(state.dismissTimer);
            state.dismissTimer = null;
        }
        if (state.activeTooltip) {
            if (state.activeTooltip.parentNode) {
                state.activeTooltip.parentNode.removeChild(state.activeTooltip);
            }
            state.activeTooltip = null;
        }
    }

    function dismissAll() {
        dismiss();
        if (state.sequenceTimer) {
            clearTimeout(state.sequenceTimer);
            state.sequenceTimer = null;
        }
        markAllShown();
        hideDismissAllButton();
    }

    // ============================================
    // Sequential Tooltip Tour
    // ============================================
    function showSequence() {
        if (!shouldShowSequence()) return;

        // Collect tooltips whose targets exist in the DOM
        var visible = [];
        for (var i = 0; i < TOOLTIP_MANIFEST.length; i++) {
            var t = TOOLTIP_MANIFEST[i];
            if (!hasShown(t.id) && document.querySelector(t.target)) {
                visible.push(t);
            }
        }

        if (visible.length === 0) {
            markAllShown();
            return;
        }

        showDismissAllButton();

        // Show first tooltip
        if (show(visible[0].id, visible[0].target, visible[0].text, visible[0].direction)) {
            state.sequenceIndex = 1;
            scheduleNext(visible);
        } else {
            markAllShown();
            hideDismissAllButton();
        }
    }

    function scheduleNext(visible) {
        if (state.sequenceIndex >= visible.length) {
            markAllShown();
            hideDismissAllButton();
            return;
        }

        state.sequenceTimer = setTimeout(function () {
            var next = visible[state.sequenceIndex];
            if (show(next.id, next.target, next.text, next.direction)) {
                state.sequenceIndex++;
                scheduleNext(visible);
            } else {
                // Target no longer visible — skip
                state.sequenceIndex++;
                scheduleNext(visible);
            }
        }, DISMISS_MS + SEQUENCE_GAP_MS);
    }

    // ============================================
    // Dismiss-All Button
    // ============================================
    function showDismissAllButton() {
        if (document.getElementById('spaxel-dismiss-all-tooltips')) return;

        var btn = document.createElement('button');
        btn.id = 'spaxel-dismiss-all-tooltips';
        btn.className = 'spaxel-dismiss-all';
        btn.textContent = 'Dismiss all tips';
        btn.addEventListener('click', dismissAll);
        document.body.appendChild(btn);
    }

    function hideDismissAllButton() {
        var btn = document.getElementById('spaxel-dismiss-all-tooltips');
        if (btn && btn.parentNode) btn.parentNode.removeChild(btn);
    }

    // ============================================
    // Helpers
    // ============================================
    function escapeHTML(s) {
        var div = document.createElement('div');
        div.textContent = s;
        return div.innerHTML;
    }

    // ============================================
    // Public API
    // ============================================
    window.SpaxelTooltips = {
        show: show,
        dismiss: dismiss,
        dismissAll: dismissAll,
        showSequence: showSequence,
        // Exposed for testing
        _TOOLTIP_MANIFEST: TOOLTIP_MANIFEST,
        _STORAGE_PREFIX: STORAGE_PREFIX,
        _DISMISS_MS: DISMISS_MS,
        _state: state,
    };
})();
