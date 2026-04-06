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

    // API endpoints
    const API = {
        status: '/api/security/status',
        arm: '/api/mode',  // Uses POST with {mode: "away", reason: "manual"}
        disarm: '/api/mode',  // Uses POST with {mode: "home", reason: "manual"}
        anomalies: '/api/anomalies',
        activeAnomalies: '/api/anomalies/active',
        anomalyHistory: '/api/anomalies/history',
        learning: '/api/anomalies/learning'
    };

    // DOM elements (lazy-initialized)
    let _statusIndicator = null;
    let _securityCard = null;
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

        console.log('[SecurityPanel] Module initialized');
    }

    function ensureStatusIndicator() {
        // Check if indicator already exists
        _statusIndicator = document.getElementById('security-status-indicator');
        if (_statusIndicator) return;

        // Create indicator in status bar
        const statusBar = document.getElementById('status-bar');
        if (!statusBar) return;

        const indicator = document.createElement('div');
        indicator.id = 'security-status-indicator';
        indicator.className = 'security-status-indicator';
        indicator.innerHTML = `
            <span class="security-icon">🛡️</span>
            <span class="security-text">Disarmed</span>
            <button class="security-toggle-btn" onclick="SecurityPanel.openSecurityDialog()">
                <svg viewBox="0 0 24 24" width="16" height="16" fill="currentColor">
                    <path d="M12 15.5a1.5 1.5 0 1 1-3 0 1.5 1.5 0 0 1 3 0zm.5 0a1 1 0 1 0-2 0 1 1 0 0 0 2 0zM12 2a10 10 0 1 0 10 10 10 10 0 0 0-10-10zm0 18a8 8 0 1 1 8-8 8 8 0 0 1-8-8zm0-14a6 6 0 1 0-6 6 6 6 0 0 0 6 6zm0 10a4 4 0 1 1-4 4 4 4 0 0 1 4-4zm0-6a2 2 0 1 0-2 2 2 2 0 0 0 2 2z"/>
                </svg>
            </button>
        `;

        // Insert before FPS counter
        const fpsCounter = document.getElementById('fps-counter');
        if (fpsCounter && fpsCounter.parentElement) {
            fpsCounter.parentElement.insertBefore(indicator, fpsCounter);
        } else {
            statusBar.appendChild(indicator);
        }

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
        fetch(API.anomalyHistory + '?limit=100&since=' + (Date.now() - 24*60*60*1000))
            .then(res => res.json())
            .then(history => {
                if (Array.isArray(history)) {
                    _anomalyCount24h = history.length;
                    if (history.length > 0) {
                        _lastAnomaly = history[0];
                    }
                    updateSecurityCard();
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
                _learningUntil = data.days_remaining || null;
                updateLearningProgress();
            })
            .catch(err => {
                console.error('[SecurityPanel] Failed to fetch learning progress:', err);
            });
    }

    // ── status updates ────────────────────────────────────────────────────────

    function updateSecurityStatus(data) {
        const prevMode = _securityMode;

        if (data.security_mode) {
            _securityMode = 'armed';
        } else if (data.model_ready && data.learning_progress >= 1.0) {
            _securityMode = 'ready';
        } else if (data.learning_progress > 0) {
            _securityMode = 'learning';
        } else {
            _securityMode = 'disarmed';
        }

        _armedAt = data.armed_at || null;

        updateStatusIndicator();
        updateSecurityCard();

        // Fire mode change callback
        if (prevMode !== _securityMode && _onModeChange) {
            _onModeChange(_securityMode, prevMode);
        }

        // Update learning progress if available
        if (data.learning_progress !== undefined) {
            _learningProgress = data.learning_progress;
            updateLearningProgress();
        }
    }

    function updateStatusIndicator() {
        if (!_statusIndicator) return;

        const textEl = _statusIndicator.querySelector('.security-text');
        const iconEl = _statusIndicator.querySelector('.security-icon');

        let text, icon;
        switch (_securityMode) {
            case 'armed':
                text = 'ARMED';
                icon = '🔴';
                break;
            case 'alert':
                text = 'ALERT';
                icon = '🚨';
                break;
            case 'ready':
                text = 'READY';
                icon = '🛡️';
                break;
            case 'learning':
                text = 'LEARNING';
                icon = '📚';
                break;
            default:
                text = 'DISARMED';
                icon = '🛡️';
                break;
        }

        if (textEl) textEl.textContent = text;
        if (iconEl) iconEl.textContent = icon;

        _statusIndicator.dataset.mode = _securityMode;
    }

    function updateSecurityCard() {
        // If security card is visible, update it
        if (_securityCard) {
            renderSecurityCard();
        }
    }

    function updateLearningProgress() {
        // Update learning banner if visible
        const banner = document.getElementById('anomaly-learning-banner');
        if (!banner) return;

        const progressEl = banner.querySelector('.learning-progress');
        const daysEl = banner.querySelector('.days-remaining');

        if (_securityMode === 'learning') {
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
                    <button class="security-dialog-close" onclick="this.closest('.security-dialog-overlay').remove()">×</button>
                </div>
                <div class="security-dialog-content">
                    <p class="security-dialog-prompt">
                        ${isArmed
                            ? 'Disarming security mode will disable automatic intrusion detection.'
                            : 'Arming security mode will enable automatic intrusion detection. Any motion detected will trigger an alert.'
                        }
                    </p>
                    ${_securityMode === 'learning' ? `
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
                    <button class="security-dialog-btn cancel" onclick="SecurityPanel.closeSecurityDialog()">Cancel</button>
                    <button class="security-dialog-btn ${actionClass}" onclick="SecurityPanel.${isArmed ? 'disarm' : 'arm'}()">
                        ${action}
                    </button>
                </div>
            </div>
        `;

        document.body.appendChild(dialog);

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
                mode: 'away',
                reason: 'manual'
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
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({
                mode: 'home',
                reason: 'manual'
            })
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

        if (title) title.textContent = anomaly.type === 'unknown_ble' ? 'Unknown Device Detected' : 'Motion Detected';
        if (desc) desc.textContent = anomaly.description || 'Anomaly detected';
        if (zone) zone.textContent = anomaly.zone_name || 'Unknown zone';
        if (time) time.textContent = formatTimeAgo(anomaly.timestamp);

        if (acknowledgeBtn) {
            acknowledgeBtn.onclick = function() {
                acknowledgeAnomaly(anomaly.id);
            };
        }

        _alertBanner.classList.remove('hidden');
        _alertBanner.dataset.anomalyId = anomaly.id;

        // Play alert sound
        playAlertSound();
    }

    function hideAlertBanner() {
        if (_alertBanner) {
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
            <button class="alert-banner-acknowledge" onclick="">Acknowledge</button>
        `;

        document.body.appendChild(_alertBanner);
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

    // ── security card (sidebar) ─────────────────────────────────────────────────────

    function openSecurityCard() {
        // If panels framework is available, open as a sidebar panel
        if (window.SpaxelPanels) {
            SpaxelPanels.openSidebar({
                title: 'Security Mode',
                content: '<div id="security-card-content"></div>',
                width: '350px',
                onOpen: function() {
                    renderSecurityCard();
                }
            });
        } else {
            // Fallback: create as standalone modal
            createSecurityCardModal();
        }
    }

    function renderSecurityCard() {
        const container = document.getElementById('security-card-content');
        if (!container) return;

        const isArmed = _securityMode === 'armed' || _securityMode === 'alert';
        const isLearning = _securityMode === 'learning';

        container.innerHTML = `
            <div class="security-card-status ${_securityMode}">
                <div class="security-card-status-icon">
                    ${getModeIcon()}
                </div>
                <div class="security-card-status-text">
                    ${getModeText()}
                </div>
            </div>

            ${renderLearningProgress()}

            <div class="security-card-stats">
                <div class="security-stat">
                    <span class="security-stat-label">Last 24h</span>
                    <span class="security-stat-value">${_anomalyCount24h}</span>
                </div>
                <div class="security-stat">
                    <span class="security-stat-label">Last Event</span>
                    <span class="security-stat-value">${_lastAnomaly ? formatTimeAgo(_lastAnomaly.timestamp) : 'None'}</span>
                </div>
            </div>

            <div class="security-card-actions">
                ${isArmed ? `
                    <button class="security-card-btn disarm" onclick="SecurityPanel.disarm()">Disarm</button>
                ` : `
                    <button class="security-card-btn arm" onclick="SecurityPanel.arm()" ${isLearning ? 'disabled' : ''}>
                        Arm Security
                    </button>
                `}
                <button class="security-card-btn timeline" onclick="SecurityPanel.openAnomalyTimeline()">View Timeline</button>
            </div>

            ${renderRecentAnomalies()}
        `;
    }

    function getModeIcon() {
        switch (_securityMode) {
            case 'armed': return '🔴';
            case 'alert': return '🚨';
            case 'ready': return '🛡️';
            case 'learning': return '📚';
            default: return '🛡️';
        }
    }

    function getModeText() {
        switch (_securityMode) {
            case 'armed': return 'ARMED';
            case 'alert': return 'ALERT';
            case 'ready': return 'READY';
            case 'learning': return 'LEARNING';
            default: return 'DISARMED';
        }
    }

    function renderLearningProgress() {
        if (_securityMode !== 'learning') return '';

        const progress = Math.round(_learningProgress * 100);
        const days = Math.ceil((1 - _learningProgress) * 7);

        return `
            <div class="security-learning-progress">
                <div class="learning-progress-header">
                    <span>Learning Progress</span>
                    <span>${progress}%</span>
                </div>
                <div class="learning-progress-bar">
                    <div class="learning-progress-fill" style="width: ${progress}%"></div>
                </div>
                <div class="learning-progress-footer">
                    ${days} day${days !== 1 ? 's' : ''} remaining
                </div>
            </div>
        `;
    }

    function renderRecentAnomalies() {
        // This would fetch recent anomalies from the API
        return `
            <div class="security-recent-anomalies">
                <div class="security-section-header">Recent Anomalies</div>
                <div class="security-anomalies-list" id="security-anomalies-list">
                    <div class="security-empty-state">No recent anomalies</div>
                </div>
            </div>
        `;
    }

    // ── anomaly timeline ─────────────────────────────────────────────────────────

    function openAnomalyTimeline() {
        // If panels framework is available, open as panel
        if (window.SpaxelPanels) {
            SpaxelPanels.openSidebar({
                title: 'Anomaly Timeline',
                content: '<div id="anomaly-timeline-content"></div>',
                width: '450px',
                onOpen: function() {
                    renderAnomalyTimeline();
                }
            });
        }
    }

    function renderAnomalyTimeline() {
        const container = document.getElementById('anomaly-timeline-content');
        if (!container) return;

        container.innerHTML = `
            <div class="anomaly-timeline-filters">
                <select id="anomaly-time-filter" class="timeline-filter-select">
                    <option value="24h">Last 24 hours</option>
                    <option value="7d">Last 7 days</option>
                    <option value="30d">Last 30 days</option>
                    <option value="all">All time</option>
                </select>
            </div>
            <div id="anomaly-timeline-list" class="anomaly-timeline-list">
                <div class="timeline-loading">Loading anomalies...</div>
            </div>
        `;

        // Load anomalies
        loadAnomalies('24h');

        // Attach filter listener
        const filter = container.querySelector('#anomaly-time-filter');
        if (filter) {
            filter.addEventListener('change', function() {
                loadAnomalies(this.value);
            });
        }
    }

    function loadAnomalies(timeFilter) {
        const listContainer = document.getElementById('anomaly-timeline-list');
        if (!listContainer) return;

        let url = API.anomalyHistory + '?limit=50';
        if (timeFilter && timeFilter !== 'all') {
            const since = Date.now() - parseTimeFilter(timeFilter);
            url += '&since=' + Math.floor(since / 1000);
        }

        fetch(url)
            .then(res => res.json())
            .then(anomalies => {
                renderAnomaliesList(anomalies);
            })
            .catch(err => {
                console.error('[SecurityPanel] Failed to load anomalies:', err);
                listContainer.innerHTML = `
                    <div class="timeline-error">Failed to load anomalies</div>
                `;
            });
    }

    function renderAnomaliesList(anomalies) {
        const listContainer = document.getElementById('anomaly-timeline-list');
        if (!listContainer) return;

        if (!anomalies || anomalies.length === 0) {
            listContainer.innerHTML = `
                <div class="timeline-empty">
                    <div class="timeline-empty-icon">📭</div>
                    <h3>No Anomalies</h3>
                    <p>When anomalies are detected, they will appear here.</p>
                </div>
            `;
            return;
        }

        let html = '';
        anomalies.forEach(function(anomaly) {
            const severityClass = getSeverityClass(anomaly);
            const icon = getAnomalyIcon(anomaly);
            const time = formatTimeAgo(anomaly.timestamp);

            html += `
                <div class="anomaly-timeline-item ${severityClass}">
                    <div class="anomaly-timeline-icon">${icon}</div>
                    <div class="anomaly-timeline-content">
                        <div class="anomaly-timeline-title">${anomaly.description}</div>
                        <div class="anomaly-timeline-meta">
                            ${anomaly.zone_name ? `<span>${anomaly.zone_name}</span> • ` : ''}
                            <span>${time}</span>
                            ${anomaly.score ? `<span class="anomaly-score">Score: ${Math.round(anomaly.score * 100)}%</span>` : ''}
                        </div>
                    </div>
                    ${!anomaly.acknowledged ? `
                        <button class="anomaly-timeline-ack" onclick="SecurityPanel.acknowledgeFromTimeline('${anomaly.id}')">
                            Acknowledge
                        </button>
                    ` : `
                        <div class="anomaly-timeline-acknowledged">Acknowledged</div>
                    `}
                </div>
            `;
        });

        listContainer.innerHTML = html;
    }

    function getSeverityClass(anomaly) {
        const score = anomaly.score || 0;
        if (score >= 0.85) return 'severity-critical';
        if (score >= 0.6) return 'severity-warning';
        return 'severity-info';
    }

    function getAnomalyIcon(anomaly) {
        switch (anomaly.type) {
            case 'unknown_ble': return '📱';
            case 'motion_during_away': return '🏃';
            case 'unusual_hour': return '🕐';
            case 'unusual_dwell': return '⏱️';
            default: return '⚠️';
        }
    }

    function acknowledgeFromTimeline(anomalyId) {
        acknowledgeAnomaly(anomalyId).then(() => {
            // Refresh the list
            const filter = document.getElementById('anomaly-time-filter');
            loadAnomalies(filter ? filter.value : '24h');
        });
    }

    // ── WebSocket message handling ───────────────────────────────────────────────

    function handleWebSocketMessage(msg) {
        switch (msg.type) {
            case 'system_mode_change':
                if (msg.data) {
                    updateSecurityStatus({
                        security_mode: msg.data.new_mode === 'away'
                    });
                }
                break;

            case 'anomaly_detected':
                if (msg.data && _securityMode === 'armed') {
                    showAlertBanner(msg.data);
                }
                break;
        }
    }

    // ── helpers ─────────────────────────────────────────────────────────────────

    function formatTimeAgo(timestamp) {
        const now = Date.now();
        const diff = now - timestamp;

        if (diff < 60000) {
            const mins = Math.floor(diff / 60000);
            return mins + ' min' + (mins !== 1 ? 's' : '') + ' ago';
        } else if (diff < 3600000) {
            const hours = Math.floor(diff / 3600000);
            return hours + ' hour' + (hours !== 1 ? 's' : '') + ' ago';
        } else {
            const days = Math.floor(diff / 86400000);
            return days + ' day' + (days !== 1 ? 's' : '') + ' ago';
        }
    }

    function parseTimeFilter(filter) {
        // Convert "24h", "7d", "30d" to milliseconds
        const match = filter.match(/^(\d+)([hd])$/);
        if (match) {
            const value = parseInt(match[1]);
            const unit = match[2];
            if (unit === 'h') return value * 3600000;
            if (unit === 'd') return value * 86400000;
        }
        return 86400000; // Default to 24 hours
    }

    // ── public API ──────────────────────────────────────────────────────────────

    window.SecurityPanel = {
        init: init,
        arm: arm,
        disarm: disarm,
        openSecurityDialog: openSecurityDialog,
        closeSecurityDialog: closeSecurityDialog,
        openSecurityCard: openSecurityCard,
        openAnomalyTimeline: openAnomalyTimeline,
        acknowledgeFromTimeline: acknowledgeFromTimeline,
        acknowledgeAnomaly: acknowledgeAnomaly,
        getSecurityMode: function() { return _securityMode; },
        isLearning: function() { return _securityMode === 'learning'; },
        isReady: function() { return _securityMode === 'ready'; },
        setOnModeChange: function(cb) { _onModeChange = cb; }
    };

    // Auto-initialize when DOM is ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }

})();
