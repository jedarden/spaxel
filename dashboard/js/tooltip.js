/**
 * Spaxel First-Time Feature Discovery Toololtips
 *
 * Shows brief, non-intrusive tooltips when users access features for the first time.
 * Tooltips are shown once per feature and remembered in localStorage.
 */

(function () {
    'use strict';

    // Feature IDs that trigger tooltips
    var FEATURES = {
        TRIGGER_VOLUMES: 'trigger_volumes',
        COVERAGE_PAINTING: 'coverage_painting',
        TIME_TRAVEL: 'time_travel',
        FRESNEL_ZONES: 'fresnel_zones',
        PERSON_IDENTITY: 'person_identity',
        AUTOMATION_BUILDER: 'automation_builder',
    };

    // Cooldown duration (24 hours)
    var COOLDOWN_MS = 24 * 60 * 60 * 1000;

    /**
     * Check if a tooltip should be shown for a feature.
     * Returns a promise that resolves to {show: boolean, tooltip?: object}.
     */
    function shouldShow(featureId) {
        // Check localStorage cooldown
        var cooldownKey = 'spaxel_tooltip_shown_' + featureId;
        var lastShown = localStorage.getItem(cooldownKey);
        if (lastShown) {
            var elapsed = Date.now() - parseInt(lastShown, 10);
            if (elapsed < COOLDOWN_MS) {
                return Promise.resolve({ show: false });
            }
        }

        // Check with server
        return fetch('/api/guided/tooltip/' + featureId)
            .then(function(res) {
                if (!res.ok) {
                    if (res.status === 404) {
                        // No tooltip configured for this feature
                        return { show: false };
                    }
                    throw new Error('Failed to check tooltip: ' + res.status);
                }
                return res.json();
            })
            .then(function(data) {
                return data.show ? { show: true, tooltip: data } : { show: false };
            })
            .catch(function(err) {
                console.error('[Tooltip] Failed to check tooltip:', err);
                return { show: false };
            });
    }

    /**
     * Mark a tooltip as shown (dismissed).
     */
    function markShown(featureId) {
        var cooldownKey = 'spaxel_tooltip_shown_' + featureId;
        localStorage.setItem(cooldownKey, Date.now().toString());

        // Also notify server
        return fetch('/api/guided/tooltip/' + featureId + '/dismiss', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
        }).catch(function(err) {
            console.error('[Tooltip] Failed to dismiss tooltip:', err);
        });
    }

    /**
     * Show a tooltip for a feature.
     * @param {string} featureId - The feature ID
     * @param {HTMLElement} target - The target element to anchor the tooltip to
     * @param {string} position - Position: 'top', 'bottom', 'left', or 'right'
     */
    function show(featureId, target, position) {
        if (!target) return;

        shouldShow(featureId).then(function(result) {
            if (!result.show) return;

            var tooltip = result.tooltip;
            var el = createTooltipElement(tooltip, position);
            positionTooltip(el, target, position || 'bottom');

            // Auto-dismiss after 10 seconds or on click
            var dismissTimer = setTimeout(function() {
                dismiss(el, featureId);
            }, 10000);

            el.addEventListener('click', function() {
                clearTimeout(dismissTimer);
                dismiss(el, featureId);
            });
        });
    }

    /**
     * Create the tooltip DOM element.
     */
    function createTooltipElement(tooltip, position) {
        var el = document.createElement('div');
        el.className = 'spaxel-tooltip spaxel-tooltip-' + (position || 'bottom');
        el.innerHTML =
            '<div class="spaxel-tooltip-content">' +
                '<div class="spaxel-tooltip-title">' + escapeHtml(tooltip.title || 'Tip') + '</div>' +
                '<div class="spaxel-tooltip-text">' + escapeHtml(tooltip.description || '') + '</div>' +
                '<div class="spaxel-tooltip-close"></div>' +
            '</div>' +
            '<div class="spaxel-tooltip-arrow spaxel-tooltip-arrow-' + (position || 'bottom') + '"></div>';
        document.body.appendChild(el);
        return el;
    }

    /**
     * Position the tooltip relative to the target element.
     */
    function positionTooltip(tooltip, target, position) {
        var targetRect = target.getBoundingClientRect();
        var tooltipRect = tooltip.getBoundingClientRect();

        var top, left;

        switch (position) {
            case 'top':
                top = targetRect.top - tooltipRect.height - 10;
                left = targetRect.left + (targetRect.width - tooltipRect.width) / 2;
                break;
            case 'bottom':
                top = targetRect.bottom + 10;
                left = targetRect.left + (targetRect.width - tooltipRect.width) / 2;
                break;
            case 'left':
                top = targetRect.top + (targetRect.height - tooltipRect.height) / 2;
                left = targetRect.left - tooltipRect.width - 10;
                break;
            case 'right':
                top = targetRect.top + (targetRect.height - tooltipRect.height) / 2;
                left = targetRect.right + 10;
                break;
            default:
                top = targetRect.bottom + 10;
                left = targetRect.left + (targetRect.width - tooltipRect.width) / 2;
        }

        // Constrain to viewport
        var viewportWidth = window.innerWidth;
        var viewportHeight = window.innerHeight;

        if (left < 10) left = 10;
        if (left + tooltipRect.width > viewportWidth - 10) {
            left = viewportWidth - tooltipRect.width - 10;
        }
        if (top < 10) top = 10;
        if (top + tooltipRect.height > viewportHeight - 10) {
            top = viewportHeight - tooltipRect.height - 10;
        }

        tooltip.style.left = left + 'px';
        tooltip.style.top = top + 'px';
    }

    /**
     * Dismiss and remove the tooltip.
     */
    function dismiss(el, featureId) {
        el.classList.add('spaxel-tooltip-hiding');
        setTimeout(function() {
            if (el.parentNode) {
                el.parentNode.removeChild(el);
            }
            if (featureId) {
                markShown(featureId);
            }
        }, 300);
    }

    /**
     * Escape HTML to prevent XSS.
     */
    function escapeHtml(str) {
        var div = document.createElement('div');
        div.textContent = str;
        return div.innerHTML;
    }

    // Public API
    window.SpaxelTooltip = {
        show: show,
        shouldShow: shouldShow,
        markShown: markShown,
        FEATURES: FEATURES,
    };

    console.log('[Spaxel] Tooltip manager loaded');
})();
