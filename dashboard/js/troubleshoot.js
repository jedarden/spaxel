/**
 * Spaxel Troubleshooting Manager
 *
 * Handles node offline cards, detection quality banners, and
 * calibration completion reinforcement. Subscribes to WebSocket
 * events via TroubleshootManager.handleEvent() called from app.js.
 *
 * Issue state machine per issue key:
 *   DETECTED -> NOTIFIED -> RESOLVED | DISMISSED
 *
 * State is in-memory only — issues re-fire on next event after page refresh.
 */

(function () {
    'use strict';

    // ============================================
    // Constants
    // ============================================
    var STATES = {
        DETECTED: 'detected',
        NOTIFIED: 'notified',
        RESOLVED: 'resolved',
        DISMISSED: 'dismissed',
    };

    var PACKET_CHECK_MS = 5000;   // check link health every 5 s
    var NO_FRAME_MS = 60000;      // 60 s with no frames = quality issue
    var RECOVERY_RATIO = 0.8;     // frames resume → resolve if within 80 % of threshold

    // ============================================
    // Internal State
    // ============================================
    var state = {
        issues: {},       // issueKey -> { state, data, element }
        checkTimer: null,
        nodePanelSection: null,
    };

    // ============================================
    // Initialization
    // ============================================
    function init() {
        // Create troubleshoot section inside the node panel
        var panel = document.getElementById('node-panel');
        if (panel) {
            state.nodePanelSection = document.createElement('div');
            state.nodePanelSection.id = 'troubleshoot-section';
            panel.appendChild(state.nodePanelSection);
        }

        // Periodic client-side link health check
        state.checkTimer = setInterval(checkLinkHealth, PACKET_CHECK_MS);
    }

    // ============================================
    // Public Event Handler (called from app.js)
    // ============================================
    function handleEvent(type, data) {
        switch (type) {
            case 'node_disconnected':
                handleNodeOffline(data);
                break;
            case 'node_connected':
                handleNodeOnline(data);
                break;
            case 'low_packet_rate':
                handleLowPacketRate(data);
                break;
            case 'calibration_complete':
                handleCalibrationComplete(data);
                break;
        }
    }

    // ============================================
    // Node Offline
    // ============================================
    function handleNodeOffline(data) {
        var mac = data.mac;
        var key = 'offline_' + mac;

        if (state.issues[key]) return; // already tracking

        var issue = { state: STATES.DETECTED, data: data, element: null };
        state.issues[key] = issue;
        issue.state = STATES.NOTIFIED;
        issue.element = renderOfflineCard(mac);
    }

    function handleNodeOnline(data) {
        var mac = data.mac;
        var key = 'offline_' + mac;

        resolveIssue(key);

        // Also clear any quality banners for links involving this node
        clearQualityIssuesForNode(mac);
    }

    function renderOfflineCard(mac) {
        var last4 = mac.slice(-5).replace(':', '');
        var time = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });

        var card = document.createElement('div');
        card.className = 'troubleshoot-card troubleshoot-offline-card';
        card.innerHTML =
            '<div class="troubleshoot-card-header">' +
                '<span class="troubleshoot-card-icon">\u26A0</span>' +
                '<span>Node <strong>' + escapeAttr(mac) + '</strong> went offline at ' + escapeAttr(time) + '</span>' +
                '<button class="troubleshoot-dismiss" title="Dismiss">&times;</button>' +
            '</div>' +
            '<div class="troubleshoot-timeline">' +
                '<div class="troubleshoot-step">' +
                    '<div class="troubleshoot-step-num">1</div>' +
                    '<div class="troubleshoot-step-text">Check the node\'s power LED is on (solid green = connected, blinking = attempting WiFi)</div>' +
                '</div>' +
                '<div class="troubleshoot-step">' +
                    '<div class="troubleshoot-step-num">2</div>' +
                    '<div class="troubleshoot-step-text">If blinking: move the node closer to your WiFi router temporarily</div>' +
                '</div>' +
                '<details class="troubleshoot-more">' +
                    '<summary>More options</summary>' +
                    '<div class="troubleshoot-step">' +
                        '<div class="troubleshoot-step-num">3</div>' +
                        '<div class="troubleshoot-step-text">If the LED blinks rapidly after 5 minutes: the node has lost its WiFi configuration. Connect to <strong>spaxel-' + escapeAttr(last4) + '</strong> WiFi network to reconfigure.</div>' +
                    '</div>' +
                    '<div class="troubleshoot-step">' +
                        '<div class="troubleshoot-step-num">4</div>' +
                        '<div class="troubleshoot-step-text">If the LED is off: check the power supply and USB cable</div>' +
                    '</div>' +
                    '<div class="troubleshoot-step">' +
                        '<div class="troubleshoot-step-num">5</div>' +
                        '<div class="troubleshoot-step-text">Still stuck? <button class="troubleshoot-reset-btn" data-mac="' + escapeAttr(mac) + '">Reset to factory defaults</button></div>' +
                    '</div>' +
                '</details>' +
            '</div>';

        // Dismiss
        card.querySelector('.troubleshoot-dismiss').addEventListener('click', function () {
            resolveIssue('offline_' + mac);
        });

        // Factory reset instructions
        var resetBtn = card.querySelector('.troubleshoot-reset-btn');
        if (resetBtn) {
            resetBtn.addEventListener('click', function () {
                showResetInstructions(mac);
            });
        }

        if (state.nodePanelSection) {
            state.nodePanelSection.appendChild(card);
        }
        return card;
    }

    // ============================================
    // Factory Reset Instructions Modal
    // ============================================
    function showResetInstructions(mac) {
        var modal = document.createElement('div');
        modal.className = 'troubleshoot-modal-overlay';
        modal.innerHTML =
            '<div class="troubleshoot-modal">' +
                '<h3>Factory Reset Instructions</h3>' +
                '<ol class="troubleshoot-list">' +
                    '<li>Unplug the node from power</li>' +
                    '<li>Hold the <strong>BOOT</strong> button on the ESP32-S3</li>' +
                    '<li>While holding BOOT, plug in the USB cable</li>' +
                    '<li>Keep holding BOOT for 3 seconds, then release</li>' +
                    '<li>The node will enter setup mode (captive portal)</li>' +
                    '<li>Look for a WiFi network named <strong>Spaxel-Setup</strong> and connect to it</li>' +
                '</ol>' +
                '<button class="troubleshoot-modal-close wizard-btn wizard-btn-primary">Got it</button>' +
            '</div>';

        modal.querySelector('.troubleshoot-modal-close').addEventListener('click', function () {
            if (modal.parentNode) modal.parentNode.removeChild(modal);
        });
        modal.addEventListener('click', function (e) {
            if (e.target === modal && modal.parentNode) modal.parentNode.removeChild(modal);
        });

        document.body.appendChild(modal);
    }

    // ============================================
    // Detection Quality (Low Packet Rate)
    // ============================================
    function handleLowPacketRate(data) {
        var linkID = data.link_id;
        var key = 'quality_' + linkID;

        if (state.issues[key]) return;

        state.issues[key] = { state: STATES.NOTIFIED, data: data, element: null };
        state.issues[key].element = renderQualityBanner(data);
    }

    function renderQualityBanner(data) {
        var banner = document.createElement('div');
        banner.className = 'troubleshoot-quality-banner';
        var safeId = (data.link_id || '').replace(/[^a-zA-Z0-9]/g, '_');
        banner.id = 'quality-banner-' + safeId;
        banner.innerHTML =
            '<span class="troubleshoot-quality-icon">\u26A0</span>' +
            '<span>Node is having trouble communicating. Check that it is powered on and within WiFi range.</span>' +
            '<button class="troubleshoot-dismiss" title="Dismiss">&times;</button>';

        banner.querySelector('.troubleshoot-dismiss').addEventListener('click', function () {
            resolveIssue('quality_' + (data.link_id || ''));
        });

        document.body.appendChild(banner);
        return banner;
    }

    function clearQualityIssuesForNode(mac) {
        var keys = Object.keys(state.issues);
        for (var i = 0; i < keys.length; i++) {
            var key = keys[i];
            if (key.indexOf('quality_') !== 0) continue;
            var d = state.issues[key].data;
            if (d && (d.node_mac === mac || d.peer_mac === mac)) {
                resolveIssue(key);
            }
        }
    }

    // ============================================
    // Client-side Link Health Check
    // ============================================
    function checkLinkHealth() {
        if (!window.SpaxelApp || typeof window.SpaxelApp.getLinks !== 'function') return;

        var links = window.SpaxelApp.getLinks();
        var now = Date.now();

        links.forEach(function (link, linkID) {
            if (!link.lastFrame) return;
            var elapsed = now - link.lastFrame;

            if (elapsed > NO_FRAME_MS) {
                // No frames for over 60 s — flag quality issue
                var key = 'quality_' + linkID;
                if (!state.issues[key]) {
                    handleLowPacketRate({
                        link_id: linkID,
                        node_mac: link.nodeMAC,
                        peer_mac: link.peerMAC,
                    });
                }
            } else if (elapsed < NO_FRAME_MS * RECOVERY_RATIO) {
                // Frames resumed — auto-resolve
                var key = 'quality_' + linkID;
                if (state.issues[key]) {
                    resolveIssue(key);
                }
            }
        });
    }

    // ============================================
    // Calibration Complete
    // ============================================
    function handleCalibrationComplete(data) {
        // The post-calibration reinforcement card is rendered by the
        // onboarding wizard itself (onboard.js).  This handler is a
        // hook for future dashboard-level use (e.g. showing a
        // notification when calibration completes on a node that was
        // already on the dashboard).
    }

    // ============================================
    // Issue Helpers
    // ============================================
    function resolveIssue(key) {
        var issue = state.issues[key];
        if (!issue) return;
        issue.state = STATES.RESOLVED;
        if (issue.element && issue.element.parentNode) {
            issue.element.parentNode.removeChild(issue.element);
        }
        delete state.issues[key];
    }

    function escapeAttr(s) {
        return String(s || '').replace(/&/g, '&amp;').replace(/"/g, '&quot;')
            .replace(/</g, '&lt;').replace(/>/g, '&gt;');
    }

    // ============================================
    // Public API
    // ============================================
    window.SpaxelTroubleshoot = {
        init: init,
        handleEvent: handleEvent,
        // Exposed for testing
        _state: state,
        _STATES: STATES,
        _NO_FRAME_MS: NO_FRAME_MS,
    };

    // Auto-init when DOM is ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }
})();
