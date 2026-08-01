/**
 * Spaxel Anomaly Detection UI
 *
 * Handles: alarm overlay, acknowledgement flow, feedback form,
 * zone pulsing indication, and anomaly history display.
 */
(function() {
    'use strict';

    // ── module state ──────────────────────────────────────────────────────────
    let _activeAnomalies = [];  // Unacknowledged anomalies
    let _anomalyHistory = [];   // Recent anomaly history
    let _learningProgress = 0;  // 0.0 - 1.0
    let _modelReady = false;
    let _securityMode = false;

    // DOM elements (lazy-initialized)
    let _overlayEl = null;
    let _bannerEl = null;
    let _feedbackModalEl = null;

    // Callbacks
    let _onAcknowledge = null;
    let _onViewIn3D = null;

    // ── initialization ────────────────────────────────────────────────────────

    function init() {
        // Create overlay elements if not present
        ensureOverlayElements();
        ensureFeedbackModal();

        // Start polling for anomaly status
        startPolling();

        console.log('[Anomaly] Module initialized');
    }

    function ensureOverlayElements() {
        // Check if overlay already exists
        _overlayEl = document.getElementById('anomaly-overlay');
        if (_overlayEl) {
            _bannerEl = document.getElementById('anomaly-banner');
            return;
        }

        // Create overlay structure
        const overlay = document.createElement('div');
        overlay.id = 'anomaly-overlay';
        overlay.className = 'anomaly-overlay hidden';
        overlay.innerHTML = `
            <div id="anomaly-banner" class="anomaly-banner">
                <div class="anomaly-icon">
                    <svg viewBox="0 0 24 24" width="48" height="48">
                        <path fill="currentColor" d="M12,2L1,21H23M12,6L19.53,19H4.47M11,10V14H13V10M11,16V18H13V16"/>
                    </svg>
                </div>
                <div class="anomaly-content">
                    <div class="anomaly-title">Anomaly Detected</div>
                    <div id="anomaly-description" class="anomaly-description"></div>
                    <div id="anomaly-meta" class="anomaly-meta"></div>
                </div>
                <div class="anomaly-actions">
                    <button id="anomaly-ack-btn" class="anomaly-btn ack">Acknowledge</button>
                    <button id="anomaly-view-btn" class="anomaly-btn view">View in 3D</button>
                    <button id="anomaly-dismiss-btn" class="anomaly-btn dismiss">Dismiss</button>
                </div>
            </div>
        `;
        document.body.appendChild(overlay);
        _overlayEl = overlay;
        _bannerEl = document.getElementById('anomaly-banner');

        // Add event listeners
        document.getElementById('anomaly-ack-btn').addEventListener('click', handleAcknowledge);
        document.getElementById('anomaly-view-btn').addEventListener('click', handleViewIn3D);
        document.getElementById('anomaly-dismiss-btn').addEventListener('click', handleDismiss);
    }

    function ensureFeedbackModal() {
        _feedbackModalEl = document.getElementById('anomaly-feedback-modal');
        if (_feedbackModalEl) return;

        const modal = document.createElement('div');
        modal.id = 'anomaly-feedback-modal';
        modal.className = 'anomaly-feedback-modal hidden';
        modal.innerHTML = `
            <div class="modal-backdrop"></div>
            <div class="modal-content">
                <h3>What was this?</h3>
                <p id="feedback-anomaly-desc" class="feedback-anomaly-desc"></p>
                <div class="feedback-options">
                    <button class="feedback-btn expected" data-feedback="expected">
                        <span class="icon">✓</span>
                        <span class="label">Expected / Known Event</span>
                        <span class="desc">I was expecting this activity</span>
                    </button>
                    <button class="feedback-btn intrusion" data-feedback="intrusion">
                        <span class="icon">⚠</span>
                        <span class="label">Genuine Intrusion</span>
                        <span class="desc">This was unauthorized access</span>
                    </button>
                    <button class="feedback-btn false-alarm" data-feedback="false_alarm">
                        <span class="icon">✕</span>
                        <span class="label">False Alarm</span>
                        <span class="desc">This detection was incorrect</span>
                    </button>
                </div>
                <div class="feedback-notes">
                    <textarea id="feedback-notes-input" placeholder="Additional notes (optional)"></textarea>
                </div>
                <div class="modal-actions">
                    <button id="feedback-cancel-btn" class="modal-btn cancel">Cancel</button>
                    <button id="feedback-submit-btn" class="modal-btn submit" disabled>Submit Feedback</button>
                </div>
            </div>
        `;
        document.body.appendChild(modal);
        _feedbackModalEl = modal;

        // Event listeners
        modal.querySelector('.modal-backdrop').addEventListener('click', hideFeedbackModal);
        document.getElementById('feedback-cancel-btn').addEventListener('click', hideFeedbackModal);
        document.getElementById('feedback-submit-btn').addEventListener('click', submitFeedback);

        // Option selection
        modal.querySelectorAll('.feedback-btn').forEach(btn => {
            btn.addEventListener('click', function() {
                modal.querySelectorAll('.feedback-btn').forEach(b => b.classList.remove('selected'));
                this.classList.add('selected');
                document.getElementById('feedback-submit-btn').disabled = false;
            });
        });
    }

    // ── polling ────────────────────────────────────────────────────────────────

    function startPolling() {
        fetchAnomalyStatus();
        setInterval(fetchAnomalyStatus, 5000);  // Poll every 5 seconds
    }

    function fetchAnomalyStatus() {
        fetch('/api/anomalies/active')
            .then(res => res.json())
            .then(anomalies => {
                handleAnomalyUpdate(anomalies || []);
            })
            .catch(err => console.error('[Anomaly] Failed to fetch status:', err));

        fetch('/api/anomalies/learning')
            .then(res => res.json())
            .then(data => {
                _learningProgress = data.progress || 0;
                _modelReady = data.model_ready || false;
                updateLearningBanner();
            })
            .catch(err => console.error('[Anomaly] Failed to fetch learning status:', err));
    }

    // ── anomaly handling ────────────────────────────────────────────────────────

    function handleAnomalyUpdate(anomalies) {
        const prevCount = _activeAnomalies.length;
        _activeAnomalies = anomalies;

        if (anomalies.length > 0) {
            // Show overlay with first unacknowledged anomaly
            showAnomalyOverlay(anomalies[0]);
        } else if (prevCount > 0) {
            // All anomalies acknowledged/cleared
            hideAnomalyOverlay();
        }

        // Update zone pulsing in 3D view
        if (window.Viz3D && Viz3D.setAnomalyZones) {
            const zoneIDs = anomalies.map(a => a.zone_id).filter(z => z);
            Viz3D.setAnomalyZones(zoneIDs);
        }
    }

    function showAnomalyOverlay(anomaly) {
        if (!_overlayEl) ensureOverlayElements();

        const descEl = document.getElementById('anomaly-description');
        const metaEl = document.getElementById('anomaly-meta');

        descEl.textContent = anomaly.description || 'Unknown anomaly detected';

        // Format metadata
        let meta = '';
        if (anomaly.zone_name) meta += `Zone: ${anomaly.zone_name} `;
        if (anomaly.person_name) meta += `Person: ${anomaly.person_name} `;
        if (anomaly.score) meta += `Score: ${(anomaly.score * 100).toFixed(0)}% `;
        if (anomaly.timestamp) {
            const ts = new Date(anomaly.timestamp);
            meta += `Time: ${ts.toLocaleTimeString()}`;
        }
        metaEl.textContent = meta;

        _overlayEl.classList.remove('hidden');
        _overlayEl.dataset.anomalyId = anomaly.id;
    }

    function hideAnomalyOverlay() {
        if (_overlayEl) {
            _overlayEl.classList.add('hidden');
        }
    }

    function handleAcknowledge() {
        if (!_overlayEl || !_overlayEl.dataset.anomalyId) return;

        const anomalyId = _overlayEl.dataset.anomalyId;
        const anomaly = _activeAnomalies.find(a => a.id === anomalyId);

        if (!anomaly) return;

        // Show feedback modal
        showFeedbackModal(anomaly);
    }

    function handleViewIn3D() {
        if (!_overlayEl || !_overlayEl.dataset.anomalyId) return;

        const anomalyId = _overlayEl.dataset.anomalyId;
        const anomaly = _activeAnomalies.find(a => a.id === anomalyId);

        if (!anomaly) return;

        if (_onViewIn3D) {
            _onViewIn3D(anomaly);
        } else if (window.Viz3D && Viz3D.focusOnZone && anomaly.zone_id) {
            Viz3D.focusOnZone(anomaly.zone_id);
        } else if (window.Viz3D && Viz3D.focusOnPosition && anomaly.position) {
            Viz3D.focusOnPosition(anomaly.position.x, anomaly.position.y, anomaly.position.z);
        }
    }

    function handleDismiss() {
        // Dismiss just hides the overlay, doesn't acknowledge
        hideAnomalyOverlay();

        // Show next anomaly if any
        if (_activeAnomalies.length > 1) {
            setTimeout(() => {
                if (_activeAnomalies.length > 0) {
                    showAnomalyOverlay(_activeAnomalies[0]);
                }
            }, 1000);
        }
    }

    // ── feedback modal ──────────────────────────────────────────────────────────

    let _currentFeedbackAnomaly = null;

    function showFeedbackModal(anomaly) {
        _currentFeedbackAnomaly = anomaly;

        const descEl = document.getElementById('feedback-anomaly-desc');
        descEl.textContent = anomaly.description || 'Unknown anomaly';

        // Reset selection
        _feedbackModalEl.querySelectorAll('.feedback-btn').forEach(b => b.classList.remove('selected'));
        document.getElementById('feedback-notes-input').value = '';
        document.getElementById('feedback-submit-btn').disabled = true;

        _feedbackModalEl.classList.remove('hidden');
    }

    function hideFeedbackModal() {
        if (_feedbackModalEl) {
            _feedbackModalEl.classList.add('hidden');
        }
        _currentFeedbackAnomaly = null;
    }

    function submitFeedback() {
        if (!_currentFeedbackAnomaly) return;

        const selectedBtn = _feedbackModalEl.querySelector('.feedback-btn.selected');
        if (!selectedBtn) return;

        const feedback = selectedBtn.dataset.feedback;
        const notes = document.getElementById('feedback-notes-input').value;

        const anomalyId = _currentFeedbackAnomaly.id;

        fetch(`/api/anomalies/${anomalyId}/acknowledge`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                feedback: feedback,
                notes: notes,
                acknowledged_by: 'dashboard_user'
            })
        })
        .then(res => res.json())
        .then(data => {
            console.log('[Anomaly] Feedback submitted:', data);
            hideFeedbackModal();
            hideAnomalyOverlay();

            // Show toast
            if (window.SpaxelApp && SpaxelApp.showToast) {
                SpaxelApp.showToast('Feedback recorded. Thank you!', 'success');
            }

            // Refresh anomaly list
            fetchAnomalyStatus();
        })
        .catch(err => {
            console.error('[Anomaly] Failed to submit feedback:', err);
            if (window.SpaxelApp && SpaxelApp.showToast) {
                SpaxelApp.showToast('Failed to submit feedback', 'error');
            }
        });
    }

    // ── learning banner ─────────────────────────────────────────────────────────

    function updateLearningBanner() {
        const banner = document.getElementById('anomaly-learning-banner');
        if (!banner) return;

        if (_modelReady) {
            banner.classList.add('hidden');
            return;
        }

        banner.classList.remove('hidden');
        const progressEl = banner.querySelector('.learning-progress');
        const daysEl = banner.querySelector('.days-remaining');

        if (progressEl) {
            progressEl.style.width = (_learningProgress * 100) + '%';
        }
        if (daysEl) {
            const daysLeft = Math.ceil((1 - _learningProgress) * 7);
            daysEl.textContent = daysLeft > 0 ? `${daysLeft} days remaining` : 'Almost ready...';
        }
    }

    // ── WebSocket message handling ───────────────────────────────────────────────

    function handleWebSocketMessage(msg) {
        switch (msg.type) {
            case 'anomaly_detected':
                // New anomaly detected
                if (msg.data) {
                    _activeAnomalies.unshift(msg.data);
                    showAnomalyOverlay(msg.data);

                    // Play alert sound in security mode
                    if (_securityMode) {
                        playAlertSound();
                    }
                }
                break;

            case 'system_mode_change':
                if (msg.data && msg.data.new_mode) {
                    _securityMode = msg.data.new_mode === 'away';
                    updateSecurityModeIndicator();
                }
                break;

            case 'anomaly_cleared':
                // Anomaly was acknowledged/cleared
                if (msg.data && msg.data.id) {
                    _activeAnomalies = _activeAnomalies.filter(a => a.id !== msg.data.id);
                    if (_activeAnomalies.length > 0) {
                        showAnomalyOverlay(_activeAnomalies[0]);
                    } else {
                        hideAnomalyOverlay();
                    }
                }
                break;
        }
    }

    function updateSecurityModeIndicator() {
        const indicator = document.getElementById('security-mode-indicator');
        if (!indicator) return;

        if (_securityMode) {
            indicator.classList.add('active');
            indicator.textContent = 'SECURITY MODE';
        } else {
            indicator.classList.remove('active');
            indicator.textContent = '';
        }
    }

    function playAlertSound() {
        // Create a simple alert tone
        try {
            const audioCtx = new (window.AudioContext || window.webkitAudioContext)();
            const oscillator = audioCtx.createOscillator();
            const gainNode = audioCtx.createGain();

            oscillator.connect(gainNode);
            gainNode.connect(audioCtx.destination);

            oscillator.type = 'sine';
            oscillator.frequency.setValueAtTime(880, audioCtx.currentTime);
            oscillator.frequency.setValueAtTime(660, audioCtx.currentTime + 0.1);
            oscillator.frequency.setValueAtTime(880, audioCtx.currentTime + 0.2);

            gainNode.gain.setValueAtTime(0.3, audioCtx.currentTime);
            gainNode.gain.exponentialRampToValueAtTime(0.01, audioCtx.currentTime + 0.5);

            oscillator.start(audioCtx.currentTime);
            oscillator.stop(audioCtx.currentTime + 0.5);
        } catch (e) {
            console.warn('[Anomaly] Could not play alert sound:', e);
        }
    }

    // ── public API ──────────────────────────────────────────────────────────────

    window.AnomalyUI = {
        init: init,
        handleWebSocketMessage: handleWebSocketMessage,
        getActiveAnomalies: function() { return _activeAnomalies; },
        isSecurityMode: function() { return _securityMode; },
        isModelReady: function() { return _modelReady; },
        getLearningProgress: function() { return _learningProgress; },
        setOnAcknowledge: function(cb) { _onAcknowledge = cb; },
        setOnViewIn3D: function(cb) { _onViewIn3D = cb; },
        showAnomalyOverlay: showAnomalyOverlay,
        hideAnomalyOverlay: hideAnomalyOverlay,
        refresh: fetchAnomalyStatus
    };

})();
