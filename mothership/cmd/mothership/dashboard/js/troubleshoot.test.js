/**
 * Tests for troubleshoot.js and tooltips.js
 *
 * Covers:
 * - Node offline card rendering and dismissal
 * - Node online event dismissing offline card
 * - Low packet rate quality banner
 * - Tooltip localStorage persistence
 * - Tooltip auto-dismiss and sequential tour
 * - Issue state machine
 */

// Set up minimal DOM before IIFEs execute
document.body.innerHTML =
    '<div id="node-panel">' +
        '<div id="node-list"></div>' +
        '<div class="link-section"></div>' +
    '</div>' +
    '<div id="chart-panel"></div>' +
    '<div id="scene-container"></div>' +
    '<div id="presence-indicator" class="clear">CLEAR</div>';

// Load modules (IIFEs execute immediately, setting window.SpaxelTroubleshoot / window.SpaxelTooltips)
require('./troubleshoot.js');
require('./tooltips.js');

var TS = window.SpaxelTroubleshoot;
var TT = window.SpaxelTooltips;

function setupDOM() {
    document.body.innerHTML =
        '<div id="node-panel">' +
            '<div id="node-list"></div>' +
            '<div class="link-section"></div>' +
        '</div>' +
        '<div id="chart-panel"></div>' +
        '<div id="scene-container"></div>' +
        '<div id="presence-indicator" class="clear">CLEAR</div>';
}

beforeEach(function () {
    // Clear any real timers from IIFE auto-init
    if (TS._state.checkTimer) {
        clearInterval(TS._state.checkTimer);
        TS._state.checkTimer = null;
    }
    if (TT._state.dismissTimer) {
        clearTimeout(TT._state.dismissTimer);
        TT._state.dismissTimer = null;
    }
    if (TT._state.sequenceTimer) {
        clearTimeout(TT._state.sequenceTimer);
        TT._state.sequenceTimer = null;
    }
    TT._state.activeTooltip = null;
    TT._state.sequenceIndex = 0;

    jest.useFakeTimers();
    localStorage.clear();
    setupDOM();

    // Reset troubleshoot issues
    TS._state.issues = {};

    // Re-init troubleshoot with fresh DOM (uses fake setInterval)
    TS.init();
});

afterEach(function () {
    jest.useRealTimers();
});

// ============================================
// Troubleshoot Tests
// ============================================

describe('SpaxelTroubleshoot', function () {

    describe('node offline card', function () {
        test('renders offline card with correct node label when node_disconnected fires', function () {
            TS.handleEvent('node_disconnected', { mac: 'AA:BB:CC:DD:EE:FF' });

            var section = document.getElementById('troubleshoot-section');
            expect(section).toBeTruthy();

            var card = section.querySelector('.troubleshoot-offline-card');
            expect(card).toBeTruthy();
            expect(card.textContent).toContain('AA:BB:CC:DD:EE:FF');
            expect(card.textContent).toContain('went offline');
        });

        test('offline card includes actionable troubleshooting steps', function () {
            TS.handleEvent('node_disconnected', { mac: 'AA:BB:CC:DD:EE:FF' });

            var card = document.querySelector('.troubleshoot-offline-card');
            expect(card.textContent).toContain('power LED');
            expect(card.textContent).toContain('WiFi router');
        });

        test('offline card shows captive portal AP SSID with last 4 MAC chars', function () {
            TS.handleEvent('node_disconnected', { mac: 'AA:BB:CC:DD:EE:FF' });

            var card = document.querySelector('.troubleshoot-offline-card');
            // mac.slice(-5) = 'EE:FF', .replace(':','') = 'EEFF'
            expect(card.innerHTML).toContain('spaxel-EEFF');
        });

        test('offline card can be dismissed via X button', function () {
            TS.handleEvent('node_disconnected', { mac: 'AA:BB:CC:DD:EE:FF' });

            var card = document.querySelector('.troubleshoot-offline-card');
            expect(card).toBeTruthy();

            card.querySelector('.troubleshoot-dismiss').click();

            expect(document.querySelector('.troubleshoot-offline-card')).toBeNull();
            expect(TS._state.issues['offline_AA:BB:CC:DD:EE:FF']).toBeUndefined();
        });

        test('node_online event dismisses the offline card', function () {
            TS.handleEvent('node_disconnected', { mac: 'AA:BB:CC:DD:EE:FF' });
            expect(document.querySelector('.troubleshoot-offline-card')).toBeTruthy();

            TS.handleEvent('node_connected', { mac: 'AA:BB:CC:DD:EE:FF' });

            expect(document.querySelector('.troubleshoot-offline-card')).toBeNull();
            expect(TS._state.issues['offline_AA:BB:CC:DD:EE:FF']).toBeUndefined();
        });

        test('does not create duplicate offline cards for the same node', function () {
            TS.handleEvent('node_disconnected', { mac: 'AA:BB:CC:DD:EE:FF' });
            TS.handleEvent('node_disconnected', { mac: 'AA:BB:CC:DD:EE:FF' });

            var cards = document.querySelectorAll('.troubleshoot-offline-card');
            expect(cards.length).toBe(1);
        });

        test('"More options" expander reveals additional steps', function () {
            TS.handleEvent('node_disconnected', { mac: 'AA:BB:CC:DD:EE:FF' });

            var card = document.querySelector('.troubleshoot-offline-card');
            var more = card.querySelector('.troubleshoot-more');
            var steps = more.querySelectorAll('.troubleshoot-step');
            expect(steps.length).toBe(3);
            expect(more.textContent).toContain('factory defaults');
        });
    });

    describe('factory reset modal', function () {
        test('shows modal with reset instructions when button clicked', function () {
            TS.handleEvent('node_disconnected', { mac: 'AA:BB:CC:DD:EE:FF' });

            document.querySelector('.troubleshoot-reset-btn').click();

            var modal = document.querySelector('.troubleshoot-modal-overlay');
            expect(modal).toBeTruthy();
            expect(modal.textContent).toContain('BOOT');
            expect(modal.textContent).toContain('Spaxel-Setup');
        });

        test('modal can be closed with Got it button', function () {
            TS.handleEvent('node_disconnected', { mac: 'AA:BB:CC:DD:EE:FF' });
            document.querySelector('.troubleshoot-reset-btn').click();
            expect(document.querySelector('.troubleshoot-modal-overlay')).toBeTruthy();

            document.querySelector('.troubleshoot-modal-close').click();
            expect(document.querySelector('.troubleshoot-modal-overlay')).toBeNull();
        });

        test('modal can be closed by clicking overlay background', function () {
            TS.handleEvent('node_disconnected', { mac: 'AA:BB:CC:DD:EE:FF' });
            document.querySelector('.troubleshoot-reset-btn').click();

            document.querySelector('.troubleshoot-modal-overlay').click();
            expect(document.querySelector('.troubleshoot-modal-overlay')).toBeNull();
        });
    });

    describe('detection quality banner', function () {
        test('renders quality banner when low_packet_rate event fires', function () {
            TS.handleEvent('low_packet_rate', { link_id: 'AA:BB:CC:DD:EE:FF:11:22:33:44:55:66' });

            var banner = document.querySelector('.troubleshoot-quality-banner');
            expect(banner).toBeTruthy();
            expect(banner.textContent).toContain('having trouble communicating');
        });

        test('quality banner can be dismissed', function () {
            TS.handleEvent('low_packet_rate', { link_id: 'test-link' });

            var banner = document.querySelector('.troubleshoot-quality-banner');
            banner.querySelector('.troubleshoot-dismiss').click();

            expect(document.querySelector('.troubleshoot-quality-banner')).toBeNull();
        });

        test('does not create duplicate quality banners for the same link', function () {
            TS.handleEvent('low_packet_rate', { link_id: 'test-link' });
            TS.handleEvent('low_packet_rate', { link_id: 'test-link' });

            var banners = document.querySelectorAll('.troubleshoot-quality-banner');
            expect(banners.length).toBe(1);
        });

        test('node_online clears quality issues for links involving that node', function () {
            TS.handleEvent('low_packet_rate', {
                link_id: 'AA:BB:CC:DD:EE:FF:11:22:33:44:55:66',
                node_mac: 'AA:BB:CC:DD:EE:FF',
                peer_mac: '11:22:33:44:55:66',
            });
            expect(document.querySelector('.troubleshoot-quality-banner')).toBeTruthy();

            TS.handleEvent('node_connected', { mac: 'AA:BB:CC:DD:EE:FF' });

            expect(document.querySelector('.troubleshoot-quality-banner')).toBeNull();
        });
    });

    describe('client-side link health check', function () {
        test('flags quality issue when no frames received for 60+ seconds', function () {
            window.SpaxelApp = {
                getLinks: function () {
                    var links = new Map();
                    links.set('test-link', {
                        nodeMAC: 'AA:BB:CC:DD:EE:FF',
                        peerMAC: '11:22:33:44:55:66',
                        lastFrame: Date.now() - 65000,
                    });
                    return links;
                },
            };

            jest.advanceTimersByTime(5000);

            var banner = document.querySelector('.troubleshoot-quality-banner');
            expect(banner).toBeTruthy();

            delete window.SpaxelApp;
        });

        test('auto-resolves quality issue when frames resume', function () {
            window.SpaxelApp = {
                getLinks: function () {
                    var links = new Map();
                    links.set('test-link', {
                        nodeMAC: 'AA:BB:CC:DD:EE:FF',
                        peerMAC: '11:22:33:44:55:66',
                        lastFrame: Date.now() - 65000,
                    });
                    return links;
                },
            };

            jest.advanceTimersByTime(5000);
            expect(document.querySelector('.troubleshoot-quality-banner')).toBeTruthy();

            // Frames resume
            window.SpaxelApp = {
                getLinks: function () {
                    var links = new Map();
                    links.set('test-link', {
                        nodeMAC: 'AA:BB:CC:DD:EE:FF',
                        peerMAC: '11:22:33:44:55:66',
                        lastFrame: Date.now(),
                    });
                    return links;
                },
            };

            jest.advanceTimersByTime(5000);
            expect(document.querySelector('.troubleshoot-quality-banner')).toBeNull();

            delete window.SpaxelApp;
        });
    });

    describe('calibration_complete handler', function () {
        test('does not throw on calibration_complete event', function () {
            expect(function () {
                TS.handleEvent('calibration_complete', {
                    node_mac: 'AA:BB:CC:DD:EE:FF',
                    link_count: 2,
                });
            }).not.toThrow();
        });
    });

    describe('issue state machine', function () {
        test('issue transitions to NOTIFIED when created', function () {
            TS.handleEvent('node_disconnected', { mac: 'AA:BB:CC:DD:EE:FF' });

            var issue = TS._state.issues['offline_AA:BB:CC:DD:EE:FF'];
            expect(issue).toBeTruthy();
            expect(issue.state).toBe(TS._STATES.NOTIFIED);
        });

        test('issue is removed from state when resolved', function () {
            TS.handleEvent('node_disconnected', { mac: 'AA:BB:CC:DD:EE:FF' });
            expect(TS._state.issues['offline_AA:BB:CC:DD:EE:FF']).toBeTruthy();

            document.querySelector('.troubleshoot-offline-card .troubleshoot-dismiss').click();

            expect(TS._state.issues['offline_AA:BB:CC:DD:EE:FF']).toBeUndefined();
        });
    });
});

// ============================================
// Tooltip Tests
// ============================================

describe('SpaxelTooltips', function () {

    describe('localStorage persistence', function () {
        test('show() returns false if tooltip was previously shown', function () {
            localStorage.setItem('spaxel_tooltip_csi-chart_shown', 'true');

            var result = TT.show('csi-chart', '#chart-panel', 'Test tooltip', 'left');
            expect(result).toBe(false);
        });

        test('show() returns true and renders tooltip on first display', function () {
            var result = TT.show('csi-chart', '#chart-panel', 'Test tooltip text', 'left');
            expect(result).toBe(true);

            var tooltip = document.getElementById('spaxel-tooltip-csi-chart');
            expect(tooltip).toBeTruthy();
            expect(tooltip.textContent).toContain('Test tooltip text');
        });

        test('tooltip sets localStorage flag on auto-dismiss after 8 seconds', function () {
            TT.show('csi-chart', '#chart-panel', 'Test', 'left');

            expect(localStorage.getItem('spaxel_tooltip_csi-chart_shown')).toBeNull();

            jest.advanceTimersByTime(8000);

            expect(localStorage.getItem('spaxel_tooltip_csi-chart_shown')).toBe('true');
            expect(document.getElementById('spaxel-tooltip-csi-chart')).toBeNull();
        });

        test('tooltip does not re-appear after localStorage flag is set', function () {
            TT.show('csi-chart', '#chart-panel', 'First show', 'left');
            jest.advanceTimersByTime(8000);

            var result = TT.show('csi-chart', '#chart-panel', 'Second show', 'left');
            expect(result).toBe(false);
            expect(document.getElementById('spaxel-tooltip-csi-chart')).toBeNull();
        });
    });

    describe('dismiss and dismissAll', function () {
        test('dismiss() removes the active tooltip from DOM', function () {
            TT.show('csi-chart', '#chart-panel', 'Test', 'left');
            expect(document.getElementById('spaxel-tooltip-csi-chart')).toBeTruthy();

            TT.dismiss();

            expect(document.getElementById('spaxel-tooltip-csi-chart')).toBeNull();
        });

        test('dismissAll() sets spaxel_tooltips_shown flag', function () {
            expect(localStorage.getItem('spaxel_tooltips_shown')).toBeNull();

            TT.dismissAll();

            expect(localStorage.getItem('spaxel_tooltips_shown')).toBe('true');
        });

        test('dismissAll() removes dismiss-all button', function () {
            TT.showSequence();
            expect(document.getElementById('spaxel-dismiss-all-tooltips')).toBeTruthy();

            document.getElementById('spaxel-dismiss-all-tooltips').click();

            expect(document.getElementById('spaxel-dismiss-all-tooltips')).toBeNull();
        });
    });

    describe('sequential tour (showSequence)', function () {
        test('showSequence() does nothing if spaxel_tooltips_shown is already set', function () {
            localStorage.setItem('spaxel_tooltips_shown', 'true');

            TT.showSequence();

            expect(document.querySelector('.spaxel-tooltip')).toBeNull();
        });

        test('showSequence() shows first tooltip and dismiss-all button', function () {
            TT.showSequence();

            var tooltip = document.querySelector('.spaxel-tooltip');
            expect(tooltip).toBeTruthy();

            var dismissAllBtn = document.getElementById('spaxel-dismiss-all-tooltips');
            expect(dismissAllBtn).toBeTruthy();
            expect(dismissAllBtn.textContent).toBe('Dismiss all tips');
        });

        test('showSequence() advances through tooltips after auto-dismiss + gap', function () {
            TT.showSequence();

            var firstId = document.querySelector('.spaxel-tooltip').id;

            // Advance past auto-dismiss (8s) + gap (2s) = 10s
            jest.advanceTimersByTime(10000);

            var secondTooltip = document.querySelector('.spaxel-tooltip');
            expect(secondTooltip).toBeTruthy();
            expect(secondTooltip.id).not.toBe(firstId);
        });

        test('dismiss-all button stops the tour and sets localStorage', function () {
            TT.showSequence();
            expect(document.querySelector('.spaxel-tooltip')).toBeTruthy();

            document.getElementById('spaxel-dismiss-all-tooltips').click();

            expect(document.querySelector('.spaxel-tooltip')).toBeNull();
            expect(localStorage.getItem('spaxel_tooltips_shown')).toBe('true');
        });

        test('marks all tooltips shown after completing the full sequence', function () {
            // 4 tooltips × (8s dismiss + 2s gap) = 32s total
            // Last scheduleNext fires at 30s and immediately calls markAllShown
            TT.showSequence();

            jest.advanceTimersByTime(31000);

            expect(localStorage.getItem('spaxel_tooltips_shown')).toBe('true');
            expect(document.getElementById('spaxel-dismiss-all-tooltips')).toBeNull();
        });
    });

    describe('tooltip manifest', function () {
        test('manifest has 4 tooltips with required properties', function () {
            var manifest = TT._TOOLTIP_MANIFEST;
            expect(manifest.length).toBe(4);

            manifest.forEach(function (t) {
                expect(t.id).toBeTruthy();
                expect(t.target).toBeTruthy();
                expect(t.text).toBeTruthy();
                expect(t.direction).toBeTruthy();
            });
        });

        test('manifest covers CSI chart, 3D view, presence indicator, and link list', function () {
            var ids = TT._TOOLTIP_MANIFEST.map(function (t) { return t.id; });
            expect(ids).toContain('csi-chart');
            expect(ids).toContain('3d-view');
            expect(ids).toContain('presence-indicator');
            expect(ids).toContain('link-list');
        });
    });

    describe('tooltip positioning', function () {
        test('tooltip is positioned with fixed positioning via CSS class', function () {
            TT.show('csi-chart', '#chart-panel', 'Test', 'left');

            var tooltip = document.querySelector('.spaxel-tooltip');
            expect(tooltip.className).toContain('spaxel-tooltip');
            expect(tooltip.style.top).toBeTruthy();
            expect(tooltip.style.left).toBeTruthy();
        });

        test('tooltip has arrow element matching direction', function () {
            TT.show('csi-chart', '#chart-panel', 'Test', 'left');

            var tooltip = document.querySelector('.spaxel-tooltip');
            expect(tooltip.querySelector('.spaxel-tooltip-arrow-left')).toBeTruthy();
        });

        test('show() returns false if target element does not exist', function () {
            var result = TT.show('nonexistent', '#nonexistent-element', 'Test', 'top');
            expect(result).toBe(false);
        });
    });
});
