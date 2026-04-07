/**
 * Spaxel Dashboard - Security Mode Panel
 *
 * Security mode controls, status display, learning progress,
 * and anomaly timeline UI.
 */
(function() {
    'use strict';

    // ── module state ──────────────────────────────────────────────────────────
    let _securityMode = 'disarmed';  // 'disarmed', 'learning', 'armed', 'alert'
    let _armedAt = null;
    let _learningUntil = null;
    let _learningProgress = 0;  // 0.0 - 1.0
    let _anomalyCount24h = 0;
    let _lastAnomaly = null;
    let _modelReady = false;

    // API endpoints
    const API = {
        status: '/api/security/status',
        arm: '/api/security/arm',
        disarm: '/api/security/disarm',
        anomalies: '/api/anomalies',
        activeAnomalies: '/api/anomalies/active',
        anomalyHistory: '/api/anomalies/history',
        learning: '/api/anomalies/learning'
    };

    // DOM elements (lazy-initialized)
    let _statusIndicator = null;
    let _alertBanner = null;

    // Callbacks
    let _onModeChange = null;

    // ── initialization ────────────────────────────────────────────────────────

    function init() {
        // Create security status indicator in status bar
        ensureStatusIndicator();

        // Start polling for security status
        startPolling();

        // Listen for WebSocket messages
        if (window.SpaxelApp) {
            SpaxelApp.registerMessageHandler(handleWebSocketMessage);
        }

        // Subscribe to state changes
        if (window.SpaxelState) {
            SpaxelState.subscribe('system.security_mode', handleSecurityModeChange);
            SpaxelState.subscribe('alerts', handleAlertsChange);
        }

        console.log('[SecurityPanel] Module initialized');
    }

    function ensureStatusIndicator() {
        // Check if indicator already exists
        _statusIndicator = document.getElementById('security-status-indicator');
        if (_statusIndicator) return;

        // Find the security status container
        const container = document.getElementById('security-status-container');
        if (!container) return;

        const indicator = document.createElement('div');
        indicator.id = 'security-status-indicator';
        indicator.className = 'security-status-indicator mode-disarmed';
        indicator.innerHTML = `
            <span class="security-icon">🛡️</span>
            <span class="security-text">Disarmed</span>
            <button class="security-toggle-btn" aria-label="Toggle security mode">
                <svg viewBox="0 0 24 24" width="16" height="16" fill="currentColor">
                    <path d="M12 15.5a1.5 1.5 0 1 1-3 0 1.5 1.5 0 0 1 3 0zm.5 0a1 1 0 1 0-2 0 1 1 0 0 0 2 0zM12 2a10 10 0 1 0 10 10 10 10 0 0 0-10-10zm0 18a8 8 0 1 1 8-8 8 8 0 0 1-8-8zm0-14a6 6 0 1 0-6 6 6 6 0 0 0 6 6zm0 10a4 4 0 1 1-4 4 4 4 0 0 1 4-4zm0-6a2 2 0 1 0-2 2 2 2 0 0 0 2 2z"/>
                </svg>
            </button>
        `;

        container.appendChild(indicator);
        _statusIndicator = indicator;

        // Add click handler for mode toggle
        const toggleBtn = indicator.querySelector('.security-toggle-btn');
        if (toggleBtn) {
            toggleBtn.addEventListener('click', openSecurityDialog);
        }
    }

    // ── polling ────────────────────────────────────────────────────────────────

    function startPolling() {
        fetchSecurityStatus();
        setInterval(fetchSecurityStatus, 10000);  // Poll every 10 seconds
        setInterval(fetchAnomalyCount, 30000);   // Update anomaly count every 30s
        setInterval(fetchLearningProgress, 60000);  // Update learning progress every minute
    }

    function fetchSecurityStatus() {
        fetch(API.status)
            .then(res => res.json())
            .then(data => {
                updateSecurityStatus(data);
            })
            .catch(err => {
                console.error('[SecurityPanel] Failed to fetch security status:', err);
            });
    }

    function fetchAnomalyCount() {
        fetch(API.anomalyHistory + '?since=24h')
            .then(res => res.json())
            .then(data => {
                if (data.history && Array.isArray(data.history)) {
                    _anomalyCount24h = data.history.length;
                    if (data.history.length > 0) {
                        _lastAnomaly = data.history[0];
                    }
                    updateStatusIndicator();
                }
            })
            .catch(err => {
                console.error('[SecurityPanel] Failed to fetch anomaly count:', err);
            });
    }

    function fetchLearningProgress() {
        fetch(API.learning)
            .then(res => res.json())
            .then(data => {
                _learningProgress = data.progress || 0;
                _modelReady = data.ready || false;
                if (data.learning_until) {
                    _learningUntil = data.learning_until;
                }
                updateLearningProgress();
                updateStatusIndicator();
            })
            .catch(err => {
                console.error('[SecurityPanel] Failed to fetch learning progress:', err);
            });
    }

    // ── status updates ────────────────────────────────────────────────────────

    function updateSecurityStatus(data) {
        const prevMode = _securityMode;

        if (data.armed) {
            _securityMode = 'armed';
        } else if (data.model_ready) {
            _securityMode = 'ready';
        } else if (data.learning_until) {
            _securityMode = 'learning';
            _learningUntil = data.learning_until;
        } else {
            _securityMode = 'disarmed';
        }

        _modelReady = data.model_ready || false;
        _anomalyCount24h = data.anomaly_count_24h || 0;

        updateStatusIndicator();

        // Update global state
        if (window.SpaxelState) {
            SpaxelState.set('system.security_mode', data.armed);
        }

        // Fire mode change callback
        if (prevMode !== _securityMode && _onModeChange) {
            _onModeChange(_securityMode, prevMode);
        }
    }

    function updateStatusIndicator() {
        if (!_statusIndicator) return;

        const textEl = _statusIndicator.querySelector('.security-text');
        const iconEl = _statusIndicator.querySelector('.security-icon');

        let text, icon, modeClass;
        switch (_securityMode) {
            case 'armed':
                text = 'ARMED';
                icon = '🔴';
                modeClass = 'mode-armed';
                break;
            case 'alert':
                text = 'ALERT';
                icon = '🚨';
                modeClass = 'mode-alert';
                break;
            case 'ready':
                text = 'READY';
                icon = '🛡️';
                modeClass = 'mode-ready';
                break;
            case 'learning':
                text = 'LEARNING';
                icon = '📚';
                modeClass = 'mode-learning';
                break;
            default:
                text = 'DISARMED';
                icon = '🛡️';
                modeClass = 'mode-disarmed';
                break;
        }

        if (textEl) textEl.textContent = text;
        if (iconEl) iconEl.textContent = icon;

        // Update mode class
        _statusIndicator.classList.remove('mode-disarmed', 'mode-learning', 'mode-armed', 'mode-alert', 'mode-ready');
        _statusIndicator.classList.add(modeClass);
    }

    function updateLearningProgress() {
        // Update learning banner if visible
        const banner = document.getElementById('anomaly-learning-banner');
        if (!banner) return;

        const progressEl = banner.querySelector('.learning-progress');
        const daysEl = banner.querySelector('.days-remaining');

        if (_securityMode === 'learning' || !_modelReady) {
            banner.classList.add('visible');
            if (progressEl) {
                progressEl.style.width = (_learningProgress * 100) + '%';
            }
            if (daysEl) {
                const daysLeft = Math.ceil((1 - _learningProgress) * 7);
                daysEl.textContent = daysLeft > 0 ? daysLeft + ' days remaining' : 'Almost ready...';
            }
        } else {
            banner.classList.remove('visible');
        }
    }

    // ── state change handlers ─────────────────────────────────────────────────────

    function handleSecurityModeChange(armed) {
        // Update from global state changes
        if (armed) {
            _securityMode = 'armed';
        } else {
            _securityMode = _modelReady ? 'ready' : 'disarmed';
        }
        updateStatusIndicator();
    }

    function handleAlertsChange(alerts) {
        // Check for active alerts
        const activeAlerts = alerts.filter(a => !a.acknowledged);
        if (activeAlerts.length > 0 && _securityMode === 'armed') {
            showAlertBanner(activeAlerts[0]);
        }
    }

    // ── security dialog ───────────────────────────────────────────────────────

    function openSecurityDialog() {
        const isArmed = _securityMode === 'armed' || _securityMode === 'alert';
        const action = isArmed ? 'Disarm' : 'Arm';
        const actionClass = isArmed ? 'disarm' : 'arm';

        const dialog = document.createElement('div');
        dialog.className = 'security-dialog-overlay';
        dialog.innerHTML = `
            <div class="security-dialog-card ${actionClass}">
                <div class="security-dialog-header">
                    <h2>${action} Security Mode</h2>
                    <button class="security-dialog-close" aria-label="Close">&times;</button>
                </div>
                <div class="security-dialog-content">
                    <p class="security-dialog-prompt">
                        ${isArmed
                            ? 'Disarming security mode will disable automatic intrusion detection.'
                            : 'Arming security mode will enable automatic intrusion detection. Any motion detected will trigger an alert.'
                        }
                    </p>
                    ${!_modelReady ? `
                        <div class="security-dialog-warning">
                            <p>⚠️ Warning: The system is still learning normal patterns.</p>
                            <p>Accuracy will improve over the next ${Math.ceil((1 - _learningProgress) * 7)} days.</p>
                        </div>
                    ` : ''}
                    <div class="security-dialog-stats">
                        <div class="stat-item">
                            <span class="stat-label">Last 24h</span>
                            <span class="stat-value">${_anomalyCount24h} anomaly${_anomalyCount24h !== 1 ? 'ies' : ''}</span>
                        </div>
                        ${_lastAnomaly ? `
                        <div class="stat-item">
                            <span class="stat-label">Last Event</span>
                            <span class="stat-value">${formatTimeAgo(_lastAnomaly.timestamp)}</span>
                        </div>
                        ` : ''}
                    </div>
                </div>
                <div class="security-dialog-actions">
                    <button class="security-dialog-btn cancel" data-action="cancel">Cancel</button>
                    <button class="security-dialog-btn ${actionClass}" data-action="${isArmed ? 'disarm' : 'arm'}">
                        ${action}
                    </button>
                </div>
            </div>
        `;

        document.body.appendChild(dialog);

        // Add event listeners
        const closeBtn = dialog.querySelector('.security-dialog-close');
        const cancelBtn = dialog.querySelector('[data-action="cancel"]');
        const actionBtn = dialog.querySelector('[data-action="arm"], [data-action="disarm"]');

        if (closeBtn) closeBtn.addEventListener('click', closeSecurityDialog);
        if (cancelBtn) cancelBtn.addEventListener('click', closeSecurityDialog);
        if (actionBtn) {
            actionBtn.addEventListener('click', function() {
                const action = this.dataset.action;
                if (action === 'arm') arm();
                else if (action === 'disarm') disarm();
            });
        }

        // Auto-close on backdrop click
        dialog.addEventListener('click', function(e) {
            if (e.target === dialog) {
                closeSecurityDialog();
            }
        });
    }

    function closeSecurityDialog() {
        const dialog = document.querySelector('.security-dialog-overlay');
        if (dialog) {
            dialog.remove();
        }
    }

    // ── arm/disarm actions ────────────────────────────────────────────────────────

    function arm() {
        closeSecurityDialog();

        fetch(API.arm, {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({
                mode: 'armed'
            })
        })
        .then(res => {
            if (!res.ok) {
                throw new Error('Failed to arm: ' + res.status);
            }
            return res.json();
        })
        .then(data => {
            console.log('[SecurityPanel] Armed:', data);
            fetchSecurityStatus();  // Refresh status

            if (window.SpaxelApp && SpaxelApp.showToast) {
                SpaxelApp.showToast('Security mode armed', 'warning');
            }
        })
        .catch(err => {
            console.error('[SecurityPanel] Arm failed:', err);
            if (window.SpaxelApp && SpaxelApp.showToast) {
                SpaxelApp.showToast('Failed to arm security mode', 'error');
            }
        });
    }

    function disarm() {
        closeSecurityDialog();

        fetch(API.disarm, {
            method: 'POST',
            headers: {'Content-Type': 'application/json'}
        })
        .then(res => {
            if (!res.ok) {
                throw new Error('Failed to disarm: ' + res.status);
            }
            return res.json();
        })
        .then(data => {
            console.log('[SecurityPanel] Disarmed:', data);
            fetchSecurityStatus();  // Refresh status

            if (window.SpaxelApp && SpaxelApp.showToast) {
                SpaxelApp.showToast('Security mode disarmed', 'info');
            }
        })
        .catch(err => {
            console.error('[SecurityPanel] Disarm failed:', err);
            if (window.SpaxelApp && SpaxelApp.showToast) {
                SpaxelApp.showToast('Failed to disarm security mode', 'error');
            }
        });
    }

    // ── alert banner ────────────────────────────────────────────────────────────────

    function showAlertBanner(anomaly) {
        ensureAlertBanner();

        const title = _alertBanner.querySelector('.alert-banner-title');
        const desc = _alertBanner.querySelector('.alert-banner-description');
        const zone = _alertBanner.querySelector('.alert-banner-zone');
        const time = _alertBanner.querySelector('.alert-banner-time');
        const acknowledgeBtn = _alertBanner.querySelector('.alert-banner-acknowledge');

        if (title) title.textContent = getAnomalyTitle(anomaly);
        if (desc) desc.textContent = anomaly.description || 'Anomaly detected';
        if (zone) zone.textContent = anomaly.zone_name || 'Unknown zone';
        if (time) time.textContent = formatTimeAgo(anomaly.timestamp);

        if (acknowledgeBtn) {
            acknowledgeBtn.onclick = function() {
                acknowledgeAnomaly(anomaly.id);
            };
        }

        _alertBanner.classList.remove('hidden');
        _alertBanner.classList.add('visible');
        _alertBanner.dataset.anomalyId = anomaly.id;

        // Play alert sound
        playAlertSound();

        // Add to global state
        if (window.SpaxelState) {
            SpaxelState.addAlert({
                id: anomaly.id,
                type: 'anomaly',
                severity: anomaly.severity || 'critical',
                title: getAnomalyTitle(anomaly),
                message: anomaly.description,
                timestamp_ms: anomaly.timestamp
            });
        }
    }

    function hideAlertBanner() {
        if (_alertBanner) {
            _alertBanner.classList.remove('visible');
            _alertBanner.classList.add('hidden');
        }
    }

    function ensureAlertBanner() {
        if (_alertBanner) return;

        _alertBanner = document.createElement('div');
        _alertBanner.id = 'alert-banner';
        _alertBanner.className = 'alert-banner hidden';
        _alertBanner.innerHTML = `
            <div class="alert-banner-icon">⚠️</div>
            <div class="alert-banner-content">
                <div class="alert-banner-title">Anomaly Detected</div>
                <div class="alert-banner-description">Motion detected in unusual location</div>
                <div class="alert-banner-meta">
                    <span class="alert-banner-zone">Kitchen</span>
                    <span class="alert-banner-time">2 minutes ago</span>
                </div>
            </div>
            <div class="alert-banner-actions">
                <button class="alert-banner-btn acknowledge">Acknowledge</button>
                <button class="alert-banner-btn view">View Timeline</button>
            </div>
        `;

        document.body.appendChild(_alertBanner);

        // Add event listener for view button
        const viewBtn = _alertBanner.querySelector('.alert-banner-btn.view');
        if (viewBtn) {
            viewBtn.addEventListener('click', function() {
                hideAlertBanner();
                if (window.TimelineView) {
                    TimelineView.show();
                } else if (window.SpaxelRouter) {
                    SpaxelRouter.setMode('timeline');
                }
            });
        }
    }

    function acknowledgeAnomaly(anomalyId) {
        fetch(`/api/anomalies/${anomalyId}/acknowledge`, {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({
                feedback: 'false_alarm',
                acknowledged_by: 'dashboard_user'
            })
        })
        .then(res => {
            if (!res.ok) {
                throw new Error('Failed to acknowledge: ' + res.status);
            }
            return res.json();
        })
        .then(() => {
            hideAlertBanner();
            if (window.SpaxelApp && SpaxelApp.showToast) {
                SpaxelApp.showToast('Anomaly acknowledged', 'success');
            }

            // Update global state
            if (window.SpaxelState) {
                SpaxelState.acknowledgeAlert(anomalyId);
            }
        })
        .catch(err => {
            console.error('[SecurityPanel] Acknowledge failed:', err);
            if (window.SpaxelApp && SpaxelApp.showToast) {
                SpaxelApp.showToast('Failed to acknowledge anomaly', 'error');
            }
        });
    }

    function playAlertSound() {
        try {
            const audioCtx = new (window.AudioContext || window.webkitAudioContext)();
            const oscillator = audioCtx.createOscillator();
            const gainNode = audioCtx.createGain();

            oscillator.connect(gainNode);
            gainNode.connect(audioCtx.destination);

            oscillator.type = 'square';
            oscillator.frequency.setValueAtTime(880, audioCtx.currentTime);
            oscillator.frequency.setValueAtTime(660, audioCtx.currentTime + 0.1);
            oscillator.frequency.setValueAtTime(880, audioCtx.currentTime + 0.2);

            gainNode.gain.setValueAtTime(0.2, audioCtx.currentTime);
            gainNode.gain.exponentialRampToValueAtTime(0.01, audioCtx.currentTime + 0.5);

            oscillator.start(audioCtx.currentTime);
            oscillator.stop(audioCtx.currentTime + 0.5);
        } catch (e) {
            console.warn('[SecurityPanel] Could not play alert sound:', e);
        }
    }

    // ── helpers ─────────────────────────────────────────────────────────────────

    function formatTimeAgo(timestamp) {
        const now = Date.now();
        const diff = now - timestamp;

        if (diff < 60000) {
            const secs = Math.floor(diff / 1000);
            return secs + ' sec' + (secs !== 1 ? 's' : '') + ' ago';
        } else if (diff < 3600000) {
            const mins = Math.floor(diff / 60000);
            return mins + ' min' + (mins !== 1 ? 's' : '') + ' ago';
        } else if (diff < 86400000) {
            const hours = Math.floor(diff / 3600000);
            return hours + ' hour' + (hours !== 1 ? 's' : '') + ' ago';
        } else {
            const days = Math.floor(diff / 86400000);
            return days + ' day' + (days !== 1 ? 's' : '') + ' ago';
        }
    }

    function getAnomalyTitle(anomaly) {
        switch (anomaly.type) {
            case 'unknown_ble': return 'Unknown Device Detected';
            case 'motion_during_away': return 'Motion Detected';
            case 'unusual_hour': return 'Unusual Activity';
            case 'unusual_dwell': return 'Unusual Dwell Time';
            default: return 'Anomaly Detected';
        }
    }

    // ── WebSocket message handling ───────────────────────────────────────────────

    function handleWebSocketMessage(msg) {
        switch (msg.type) {
            case 'system_mode_change':
                if (msg.data) {
                    updateSecurityStatus({
                        armed: msg.data.armed || msg.data.mode === 'away'
                    });
                }
                break;

            case 'anomaly_detected':
                if (msg.data && _securityMode === 'armed') {
                    showAlertBanner(msg.data);
                }
                break;

            case 'security_mode':
                if (msg.data) {
                    updateSecurityStatus(msg.data);
                }
                break;
        }
    }

    // ── public API ──────────────────────────────────────────────────────────────

    window.SecurityPanel = {
        init: init,
        arm: arm,
        disarm: disarm,
        openSecurityDialog: openSecurityDialog,
        closeSecurityDialog: closeSecurityDialog,
        acknowledgeAnomaly: acknowledgeAnomaly,
        getSecurityMode: function() { return _securityMode; },
        isLearning: function() { return _securityMode === 'learning' || !_modelReady; },
        isReady: function() { return _modelReady; },
        isArmed: function() { return _securityMode === 'armed' || _securityMode === 'alert'; },
        setOnModeChange: function(cb) { _onModeChange = cb; },
        showAlertBanner: showAlertBanner,
        hideAlertBanner: hideAlertBanner
    };

    // Auto-initialize when DOM is ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }

})();
