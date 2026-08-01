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
            case 'quality_drop':
                handleQualityDrop(data);
                break;
            case 'repeated_edit':
                handleRepeatedEdit(data);
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
        if (!data) return;

        // Show positive reinforcement message
        showCalibrationReinforcement(data);
    }

    function showCalibrationReinforcement(data) {
        var improvement = Math.round((data.quality_after || 0) - (data.quality_before || 0));
        var improvementText = improvement > 0 ? '+' + improvement : Math.round(improvement);
        var encouragement = '';

        if (improvement > 20) {
            encouragement = 'Excellent! That\'s a significant improvement.';
        } else if (improvement > 10) {
            encouragement = 'Great progress! Detection is much more reliable now.';
        } else if (improvement > 0) {
            encouragement = 'Getting better. The system will continue to refine baseline over time.';
        } else {
            encouragement = 'Baseline has been updated. The system needs more data to adapt to this environment.';
        }

        var card = document.createElement('div');
        card.className = 'troubleshoot-card troubleshoot-success-card';
        card.innerHTML =
            '<div class="troubleshoot-card-header">' +
                '<span class="troubleshoot-card-icon">\u2714</span>' +
                '<span><strong>Re-baseline complete</strong></span>' +
                '<button class="troubleshoot-dismiss" title="Dismiss">&times;</button>' +
            '</div>' +
            '<div class="troubleshoot-content">' +
                '<p class="troubleshoot-success-message">' + encouragement + '</p>' +
                '<div class="troubleshoot-metrics">' +
                    '<span>Quality: ' + Math.round(data.quality_after || 0) + '%</span>' +
                    '<span class="troubleshoot-improvement">' + improvementText + '%</span>' +
                    '<span>' + (data.links || 0) + ' links calibrated</span>' +
                '</div>' +
            '</div>';

        card.querySelector('.troubleshoot-dismiss').addEventListener('click', function() {
            if (card.parentNode) card.parentNode.removeChild(card);
        });

        if (state.nodePanelSection) {
            state.nodePanelSection.appendChild(card);
        }

        // Auto-dismiss after 10 seconds
        setTimeout(function() {
            if (card.parentNode) {
                card.classList.add('troubleshoot-card-fadeout');
                setTimeout(function() {
                    if (card.parentNode) card.parentNode.removeChild(card);
                }, 500);
            }
        }, 10000);
    }

    // ============================================
    // Quality Drop Detection
    // ============================================
    function handleQualityDrop(data) {
        if (!data || !data.zone_id) return;

        var key = 'quality_' + data.zone_id;
        if (state.issues[key]) return;

        state.issues[key] = { state: STATES.NOTIFIED, data: data, element: null };
        state.issues[key].element = renderQualityDropBanner(data);
    }

    function renderQualityDropBanner(data) {
        var banner = document.createElement('div');
        banner.className = 'troubleshoot-quality-banner';
        banner.innerHTML =
            '<div class="troubleshoot-quality-content">' +
                '<span class="troubleshoot-quality-icon">\u26A0</span>' +
                '<div class="troubleshoot-quality-text">' +
                    '<strong>Detection quality has degraded in ' + escapeAttr(data.zone_name || 'this zone') + '</strong><br>' +
                    '<span class="troubleshoot-quality-detail">Quality has been below 60% for over 24 hours. This may indicate node placement issues or environmental changes.</span>' +
                '</div>' +
                '<button class="troubleshoot-action-btn" data-action="diagnose">Diagnose</button>' +
                '<button class="troubleshoot-dismiss" title="Dismiss">&times;</button>' +
            '</div>';

        // Diagnose button
        banner.querySelector('.troubleshoot-action-btn').addEventListener('click', function() {
            showQualityDiagnostics(data);
        });

        // Dismiss button
        banner.querySelector('.troubleshoot-dismiss').addEventListener('click', function() {
            resolveIssue('quality_' + (data.zone_id || ''));
            // Also dismiss on server
            fetch('/api/guided/issues/quality/' + (data.zone_id || '') + '/dismiss', {
                method: 'POST'
            }).catch(function(err) {
                console.error('[Troubleshoot] Failed to dismiss quality issue:', err);
            });
        });

        document.body.appendChild(banner);
        return banner;
    }

    function showQualityDiagnostics(data) {
        // Fetch diagnostic steps from the API
        fetch('/api/guided/issues')
            .then(function(res) { return res.json(); })
            .then(function(result) {
                var issues = result.issues || [];
                var qualityIssue = issues.find(function(i) { return i.type === 'quality_drop' && i.zone_id === data.zone_id; });

                if (qualityIssue) {
                    showGuidedDiagnosticsFlow(qualityIssue);
                }
            })
            .catch(function(err) {
                console.error('[Troubleshoot] Failed to fetch diagnostics:', err);
            });
    }

    function showGuidedDiagnosticsFlow(issue) {
        var modal = document.createElement('div');
        modal.className = 'troubleshoot-modal-overlay';
        modal.innerHTML =
            '<div class="troubleshoot-modal troubleshoot-diagnostics-modal">' +
                '<h3>Detection Quality Diagnostics</h3>' +
                '<p class="troubleshoot-diagnostics-intro">Let\'s diagnose the detection quality issue in <strong>' + escapeAttr(issue.zone_name || 'this zone') + '</strong>.</p>' +
                '<div class="troubleshoot-steps-flow">' +
                    '<div class="troubleshoot-flow-step" data-step="1">' +
                        '<div class="troubleshoot-step-number">1</div>' +
                        '<div class="troubleshoot-step-content">' +
                            '<h4>Check Node Connectivity</h4>' +
                            '<p>Verify all nodes in this zone are online and communicating properly.</p>' +
                            '<div class="troubleshoot-step-actions">' +
                                '<button class="troubleshoot-step-btn" data-action="connectivity">Check Connectivity</button>' +
                            '</div>' +
                        '</div>' +
                    '</div>' +
                    '<div class="troubleshoot-flow-step" data-step="2">' +
                        '<div class="troubleshoot-step-number">2</div>' +
                        '<div class="troubleshoot-step-content">' +
                            '<h4>View Link Health</h4>' +
                            '<p>Examine the health of sensing links in this zone to identify problematic links.</p>' +
                            '<div class="troubleshoot-step-actions">' +
                                '<button class="troubleshoot-step-btn" data-action="link_health">View Link Health</button>' +
                            '</div>' +
                        '</div>' +
                    '</div>' +
                    '<div class="troubleshoot-flow-step" data-step="3">' +
                        '<div class="troubleshoot-step-number">3</div>' +
                        '<div class="troubleshoot-step-content">' +
                            '<h4>Re-baseline Links</h4>' +
                            '<p>If the environment has changed, re-baselining the links may improve detection quality.</p>' +
                            '<div class="troubleshoot-step-actions">' +
                                '<button class="troubleshoot-step-btn" data-action="rebaseline">Re-baseline This Zone</button>' +
                            '</div>' +
                        '</div>' +
                    '</div>' +
                    '<div class="troubleshoot-flow-step" data-step="4">' +
                        '<div class="troubleshoot-step-number">4</div>' +
                        '<div class="troubleshoot-step-content">' +
                            '<h4>Consider Node Repositioning</h4>' +
                            '<p>Sometimes moving nodes slightly can dramatically improve coverage.</p>' +
                            '<div class="troubleshoot-step-actions">' +
                                '<button class="troubleshoot-step-btn" data-action="reposition">Open 3D Placement View</button>' +
                            '</div>' +
                        '</div>' +
                    '</div>' +
                '</div>' +
                '<button class="troubleshoot-modal-close wizard-btn wizard-btn-secondary">Close</button>' +
            '</div>';

        modal.querySelector('.troubleshoot-modal-close').addEventListener('click', function() {
            if (modal.parentNode) modal.parentNode.removeChild(modal);
        });

        modal.addEventListener('click', function(e) {
            if (e.target === modal && modal.parentNode) modal.parentNode.removeChild(modal);
        });

        // Handle step buttons
        modal.querySelectorAll('.troubleshoot-step-btn').forEach(function(btn) {
            btn.addEventListener('click', function() {
                var action = this.dataset.action;
                handleDiagnosticsAction(action, issue.zone_id);
            });
        });

        document.body.appendChild(modal);
    }

    function handleDiagnosticsAction(action, zoneID) {
        switch(action) {
            case 'connectivity':
                // Navigate to fleet status page
                if (window.SpaxelApp && window.SpaxelApp.navigateTo) {
                    window.SpaxelApp.navigateTo('fleet');
                }
                break;
            case 'link_health':
                // Open link health panel
                if (window.SpaxelApp && window.SpaxelApp.openLinkHealth) {
                    window.SpaxelApp.openLinkHealth();
                }
                break;
            case 'rebaseline':
                // Trigger re-baseline for zone
                fetch('/api/baseline/capture', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ zone_id: zoneID })
                })
                .then(function(res) { return res.json(); })
                .then(function(result) {
                    if (window.SpaxelApp && window.SpaxelApp.showToast) {
                        window.SpaxelApp.showToast('Re-baseline started for zone. Please keep the room clear for 60 seconds.', 'info');
                    }
                })
                .catch(function(err) {
                    console.error('[Troubleshoot] Failed to start re-baseline:', err);
                });
                break;
            case 'reposition':
                // Open 3D placement view
                if (window.SpaxelApp && window.SpaxelApp.navigateTo) {
                    window.SpaxelApp.navigateTo('placement');
                }
                break;
        }
    }

    // ============================================
    // Repeated Settings Edit
    // ============================================
    function handleRepeatedEdit(data) {
        if (!data || !data.key) return;

        showRepeatedEditHint(data);
    }

    function showRepeatedEditHint(data) {
        // Check if we've already shown this hint recently
        var hintKey = 'repeated_edit_hint_' + data.key;
        var lastShown = localStorage.getItem(hintKey);
        if (lastShown) {
            var elapsed = Date.now() - parseInt(lastShown, 10);
            if (elapsed < 24 * 60 * 60 * 1000) { // 24 hours
                return; // Already shown within cooldown
            }
        }

        var banner = document.createElement('div');
        banner.className = 'troubleshoot-hint-banner';
        banner.innerHTML =
            '<div class="troubleshoot-hint-content">' +
                '<span class="troubleshoot-hint-icon">\u2139</span>' +
                '<div class="troubleshoot-hint-text">' +
                    '<strong>Frequent adjustments detected</strong><br>' +
                    '<span>You\'ve adjusted the detection threshold several times. Would you like me to show you what the system is seeing?</span>' +
                '</div>' +
                '<button class="troubleshoot-hint-action" data-action="show">Show me</button>' +
                '<button class="troubleshoot-dismiss" title="Dismiss">&times;</button>' +
            '</div>';

        banner.querySelector('.troubleshoot-hint-action').addEventListener('click', function() {
            // Open time-travel replay with explainability
            if (window.SpaxelApp && window.SpaxelApp.openTimeTravel) {
                window.SpaxelApp.openTimeTravel({ with_explainability: true });
            }
            // Mark hint as shown
            localStorage.setItem(hintKey, Date.now().toString());
            if (banner.parentNode) banner.parentNode.removeChild(banner);
        });

        banner.querySelector('.troubleshoot-dismiss').addEventListener('click', function() {
            // Mark hint as shown
            localStorage.setItem(hintKey, Date.now().toString());
            if (banner.parentNode) banner.parentNode.removeChild(banner);
        });

        document.body.appendChild(banner);
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

    // ============================================
    // CSS Styles
    // ============================================
    function addStyles() {
        if (document.getElementById('troubleshoot-styles')) return;

        var style = document.createElement('style');
        style.id = 'troubleshoot-styles';
        style.textContent =
            '.troubleshoot-card {' +
                'background: rgba(30, 30, 58, 0.95);' +
                'border-radius: 8px;' +
                'box-shadow: 0 4px 20px rgba(0, 0, 0, 0.5);' +
                'margin-bottom: 16px;' +
                'overflow: hidden;' +
                'font-size: 13px;' +
            '}' +
            '.troubleshoot-success-card {' +
                'border-left: 3px solid #4caf50;' +
            '}' +
            '.troubleshoot-card-fadeout {' +
                'animation: troubleshootFadeOut 0.5s ease-out forwards;' +
            '}' +
            '@keyframes troubleshootFadeOut {' +
                'to { opacity: 0; max-height: 0; margin: 0; }' +
            '}' +
            '.troubleshoot-success-message {' +
                'color: #81c784;' +
                'font-weight: 500;' +
                'margin-bottom: 8px;' +
            '}' +
            '.troubleshoot-metrics {' +
                'display: flex;' +
                'gap: 16px;' +
                'font-size: 12px;' +
                'color: #888;' +
            '}' +
            '.troubleshoot-improvement {' +
                'color: #81c784;' +
                'font-weight: 500;' +
            '}' +
            '.troubleshoot-quality-banner {' +
                'position: fixed;' +
                'bottom: 0;' +
                'left: 0;' +
                'right: 0;' +
                'background: rgba(255, 167, 38, 0.15);' +
                'border-top: 2px solid #ffa726;' +
                'padding: 12px 20px;' +
                'display: flex;' +
                'align-items: center;' +
                'justify-content: center;' +
                'gap: 16px;' +
                'z-index: 150;' +
                'animation: troubleshootSlideUp 0.3s ease-out;' +
            '}' +
            '@keyframes troubleshootSlideUp {' +
                'from { transform: translateY(100%); }' +
                'to { transform: translateY(0); }' +
            '}' +
            '.troubleshoot-quality-content {' +
                'display: flex;' +
                'align-items: center;' +
                'gap: 12px;' +
            '}' +
            '.troubleshoot-quality-icon {' +
                'font-size: 20px;' +
            '}' +
            '.troubleshoot-quality-text {' +
                'flex: 1;' +
            '}' +
            '.troubleshoot-quality-detail {' +
                'font-size: 12px;' +
                'color: #aaa;' +
                'display: block;' +
                'margin-top: 2px;' +
            '}' +
            '.troubleshoot-action-btn {' +
                'background: #4fc3f7;' +
                'color: #1a1a2e;' +
                'border: none;' +
                'padding: 6px 14px;' +
                'border-radius: 4px;' +
                'font-size: 12px;' +
                'font-weight: 500;' +
                'cursor: pointer;' +
            '}' +
            '.troubleshoot-hint-banner {' +
                'position: fixed;' +
                'bottom: 80px;' +
                'left: 50%;' +
                'transform: translateX(-50%);' +
                'background: rgba(33, 150, 243, 0.15);' +
                'border: 1px solid rgba(33, 150, 243, 0.5);' +
                'border-radius: 8px;' +
                'padding: 12px 16px;' +
                'display: flex;' +
                'align-items: center;' +
                'gap: 12px;' +
                'z-index: 150;' +
                'max-width: 500px;' +
                'animation: troubleshootHintSlideUp 0.3s ease-out;' +
            '}' +
            '@keyframes troubleshootHintSlideUp {' +
                'from { transform: translateX(-50%) translateY(100px); opacity: 0; }' +
                'to { transform: translateX(-50%) translateY(0); opacity: 1; }' +
            '}' +
            '.troubleshoot-hint-icon {' +
                'font-size: 18px;' +
            '}' +
            '.troubleshoot-hint-text {' +
                'flex: 1;' +
                'font-size: 12px;' +
            '}' +
            '.troubleshoot-hint-text strong {' +
                'display: block;' +
                'color: #64b5f6;' +
                'margin-bottom: 2px;' +
            '}' +
            '.troubleshoot-hint-action {' +
                'background: #64b5f6;' +
                'color: #1a1a2e;' +
                'border: none;' +
                'padding: 4px 12px;' +
                'border-radius: 4px;' +
                'font-size: 11px;' +
                'cursor: pointer;' +
            '}' +
            '.troubleshoot-diagnostics-modal {' +
                'max-width: 600px;' +
                'width: 90%;' +
            '}' +
            '.troubleshoot-diagnostics-intro {' +
                'color: #aaa;' +
                'font-size: 13px;' +
                'margin-bottom: 20px;' +
            '}' +
            '.troubleshoot-steps-flow {' +
                'display: flex;' +
                'flex-direction: column;' +
                'gap: 16px;' +
                'margin-bottom: 20px;' +
            '}' +
            '.troubleshoot-flow-step {' +
                'display: flex;' +
                'gap: 12px;' +
                'align-items: flex-start;' +
            '}' +
            '.troubleshoot-step-number {' +
                'width: 28px;' +
                'height: 28px;' +
                'border-radius: 50%;' +
                'background: #4fc3f7;' +
                'color: #1a1a2e;' +
                'display: flex;' +
                'align-items: center;' +
                'justify-content: center;' +
                'font-weight: 600;' +
                'flex-shrink: 0;' +
            '}' +
            '.troubleshoot-step-content {' +
                'flex: 1;' +
            '}' +
            '.troubleshoot-step-content h4 {' +
                'margin: 0 0 4px 0;' +
                'font-size: 14px;' +
                'color: #eee;' +
            '}' +
            '.troubleshoot-step-content p {' +
                'margin: 0 0 8px 0;' +
                'font-size: 12px;' +
                'color: #aaa;' +
            '}' +
            '.troubleshoot-step-actions {' +
                'display: flex;' +
                'gap: 8px;' +
            '}' +
            '.troubleshoot-step-btn {' +
                'background: rgba(79, 195, 247, 0.2);' +
                'border: 1px solid rgba(79, 195, 247, 0.5);' +
                'color: #4fc3f7;' +
                'padding: 6px 12px;' +
                'border-radius: 4px;' +
                'font-size: 11px;' +
                'cursor: pointer;' +
                'transition: background 0.2s;' +
            '}' +
            '.troubleshoot-step-btn:hover {' +
                'background: rgba(79, 195, 247, 0.3);' +
            '}';

        document.head.appendChild(style);
    }

    // Add styles on init
    addStyles();

    // Auto-init when DOM is ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }
})();
