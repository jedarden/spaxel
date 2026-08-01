/**
 * Spaxel Dashboard - Settings Panel
 *
 * Motion threshold slider, sensing rate override, notification channel config,
 * and system info (version, uptime, node count)
 */

(function() {
    'use strict';

    // ============================================
    // Settings State
    // ============================================
    const settingsState = {
        loading: false,
        saving: false,
        currentSettings: null,
        notificationSettings: null
    };

    // ============================================
    // Settings API
    // ============================================

    /**
     * Fetch settings from server
     */
    function fetchSettings() {
        settingsState.loading = true;
        renderContent();

        // Fetch both main settings and notification settings in parallel
        return Promise.all([
            fetch('/api/settings').then(function(res) {
                if (!res.ok) {
                    throw new Error('Failed to fetch settings: ' + res.status);
                }
                return res.json();
            }),
            fetch('/api/settings/notifications').then(function(res) {
                if (!res.ok) {
                    // If notification settings endpoint fails, return null
                    console.warn('[SettingsPanel] Failed to fetch notification settings: ' + res.status);
                    return null;
                }
                return res.json();
            }).catch(function(err) {
                // Notification settings are optional, log warning but don't fail
                console.warn('[SettingsPanel] Notification settings not available:', err);
                return null;
            })
        ])
        .then(function(results) {
            settingsState.currentSettings = results[0];
            settingsState.notificationSettings = results[1];
            settingsState.loading = false;
            renderContent();
            return results[0];
        })
        .catch(function(err) {
            settingsState.loading = false;
            console.error('[SettingsPanel] Error fetching settings:', err);
            renderContent();
            throw err;
        });
    }

    /**
     * Save settings to server
     * @param {Object} updates - Settings to update (partial)
     */
    function saveSettings(updates) {
        if (!settingsState.currentSettings) {
            return Promise.reject(new Error('No settings loaded'));
        }

        settingsState.saving = true;
        renderContent();

        return fetch('/api/settings', {
            method: 'PATCH',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(updates)
        })
        .then(function(res) {
            if (!res.ok) {
                throw new Error('Failed to save settings: ' + res.status);
            }
            return res.json();
        })
        .then(function(data) {
            settingsState.currentSettings = data;
            settingsState.saving = false;
            renderContent();
            SpaxelPanels.showSuccess('Settings saved successfully');
            return data;
        })
        .catch(function(err) {
            settingsState.saving = false;
            console.error('[SettingsPanel] Error saving settings:', err);
            renderContent();
            SpaxelPanels.showError('Failed to save settings: ' + err.message);
            throw err;
        });
    }

    // ============================================
    // Panel Content Rendering
    // ============================================

    /**
     * Render the settings panel content
     */
    function renderContent() {
        const content = document.getElementById('settings-panel-content');
        if (!content) return;

        if (settingsState.loading) {
            content.innerHTML = renderLoading();
            return;
        }

        const settings = settingsState.currentSettings || {};

        content.innerHTML = `
            ${renderDetectionSettings(settings)}
            ${renderSecuritySettings(settings)}
            ${renderNotificationSettings(settings)}
            ${renderSystemInfo()}
        `;

        // Attach event listeners
        attachEventListeners();
    }

    function renderLoading() {
        return `
            <div class="panel-loading">
                <div class="panel-loading-spinner"></div>
                <span>Loading settings...</span>
            </div>
        `;
    }

    function renderDetectionSettings(settings) {
        const threshold = settings.delta_rms_threshold !== undefined ? settings.delta_rms_threshold : 0.02;
        const fusionRate = settings.fusion_rate_hz !== undefined ? settings.fusion_rate_hz : 10;
        const gridCell = settings.grid_cell_m !== undefined ? settings.grid_cell_m : 0.2;
        const fresnelDecay = settings.fresnel_decay !== undefined ? settings.fresnel_decay : 2.0;
        const nSubcarriers = settings.n_subcarriers !== undefined ? settings.n_subcarriers : 16;
        const tau = settings.tau_s !== undefined ? settings.tau_s : 30;

        return `
            <div class="panel-section">
                <div class="panel-section-header">Detection Settings</div>

                <div class="panel-form-group">
                    <label for="setting-threshold">Motion Threshold</label>
                    <input type="range" id="setting-threshold" min="0.005" max="0.1" step="0.005" value="${threshold}">
                    <div class="panel-form-range-value" id="setting-threshold-value">${(threshold * 1000).toFixed(0)}</div>
                    <div style="font-size: 11px; color: #888; margin-top: -4px;">
                        Lower = more sensitive, Higher = less sensitive
                    </div>
                </div>

                <div class="panel-form-group">
                    <label for="setting-fusion-rate">Fusion Rate (Hz)</label>
                    <select id="setting-fusion-rate">
                        <option value="5" ${fusionRate === 5 ? 'selected' : ''}>5 Hz (low CPU)</option>
                        <option value="10" ${fusionRate === 10 ? 'selected' : ''}>10 Hz (default)</option>
                        <option value="20" ${fusionRate === 20 ? 'selected' : ''}>20 Hz (smooth)</option>
                    </select>
                </div>

                <div class="panel-form-group">
                    <label for="setting-grid-cell">Grid Cell Size (meters)</label>
                    <input type="range" id="setting-grid-cell" min="0.1" max="0.5" step="0.05" value="${gridCell}">
                    <div class="panel-form-range-value" id="setting-grid-cell-value">${gridCell.toFixed(2)} m</div>
                </div>

                <div class="panel-form-group">
                    <label for="setting-fresnel-decay">Fresnel Weight Decay Rate</label>
                    <input type="range" id="setting-fresnel-decay" min="1" max="4" step="0.5" value="${fresnelDecay}">
                    <div class="panel-form-range-value" id="setting-fresnel-decay-value">${fresnelDecay.toFixed(1)}</div>
                    <div style="font-size: 11px; color: #888; margin-top: -4px;">
                        1.0 = flat, 2.0 = inverse-square, 4.0 = strong decay
                    </div>
                </div>

                <div class="panel-form-group">
                    <label for="setting-subcarriers">Subcarriers for Detection</label>
                    <select id="setting-subcarriers">
                        <option value="8" ${nSubcarriers === 8 ? 'selected' : ''}>8 (fast)</option>
                        <option value="16" ${nSubcarriers === 16 ? 'selected' : ''}>16 (default)</option>
                        <option value="32" ${nSubcarriers === 32 ? 'selected' : ''}>32 (accurate)</option>
                        <option value="64" ${nSubcarriers === 64 ? 'selected' : ''}>64 (all)</option>
                    </select>
                </div>

                <div class="panel-form-group">
                    <label for="setting-tau">Baseline Time Constant (seconds)</label>
                    <input type="range" id="setting-tau" min="10" max="120" step="5" value="${tau}">
                    <div class="panel-form-range-value" id="setting-tau-value">${tau} s</div>
                    <div style="font-size: 11px; color: #888; margin-top: -4px;">
                        How quickly the baseline adapts to environmental changes
                    </div>
                </div>

                <button class="panel-btn panel-btn-primary panel-btn-full" id="save-detection-btn"
                        ${settingsState.saving ? 'disabled' : ''}>
                    ${settingsState.saving ? 'Saving...' : 'Save Detection Settings'}
                </button>
            </div>
        `;
    }

    function renderSecuritySettings(settings) {
        return `
            <div class="panel-section">
                <div class="panel-section-header">Security</div>

                <div class="panel-info-card">
                    <div class="panel-info-card-title">Dashboard PIN</div>
                    <div class="panel-info-card-value">Configured</div>
                    <div class="panel-info-card-subtitle">Protects access to your dashboard</div>
                </div>

                <button class="panel-btn panel-btn-secondary panel-btn-full" id="change-pin-btn">
                    Change PIN
                </button>
            </div>
        `;
    }

    function renderNotificationSettings(settings) {
        // Get notification settings from separately fetched state
        const notificationSettings = settingsState.notificationSettings || {};

        // Channel configuration
        const channelType = notificationSettings.channel_type || 'none';
        const channelConfig = notificationSettings.channel_config || {};

        // Quiet hours
        const quietHoursEnabled = notificationSettings.quiet_hours_enabled || false;
        const quietHoursStart = notificationSettings.quiet_hours_start || '22:00';
        const quietHoursEnd = notificationSettings.quiet_hours_end || '07:00';
        const quietHoursDays = notificationSettings.quiet_hours_days !== undefined ? notificationSettings.quiet_hours_days : 0x7F;

        // Morning digest
        const morningDigestEnabled = notificationSettings.morning_digest_enabled !== undefined ? notificationSettings.morning_digest_enabled : true;
        const morningDigestTime = notificationSettings.morning_digest_time || '07:00';

        // Smart batching
        const smartBatchingEnabled = notificationSettings.smart_batching_enabled !== undefined ? notificationSettings.smart_batching_enabled : true;
        const smartBatchingWindow = notificationSettings.smart_batching_window || 30;

        // Event types
        const eventTypes = notificationSettings.event_types || {
            zone_enter: true,
            zone_leave: true,
            zone_vacant: true,
            fall_detected: true,
            fall_escalation: true,
            anomaly_alert: true,
            node_offline: true,
            sleep_summary: true
        };

        return `
            <div class="panel-section">
                <div class="panel-section-header">Notifications</div>

                <!-- Delivery Channel Selector -->
                <div class="panel-form-group">
                    <label for="notification-channel-type">Delivery Channel</label>
                    <select id="notification-channel-type" class="panel-select">
                        <option value="none" ${channelType === 'none' ? 'selected' : ''}>None</option>
                        <option value="ntfy" ${channelType === 'ntfy' ? 'selected' : ''}>Ntfy</option>
                        <option value="pushover" ${channelType === 'pushover' ? 'selected' : ''}>Pushover</option>
                        <option value="webhook" ${channelType === 'webhook' ? 'selected' : ''}>Webhook</option>
                    </select>
                </div>

                <!-- Ntfy Configuration -->
                <div id="ntfy-config" class="channel-config-group" style="${channelType === 'ntfy' ? '' : 'display: none;'}">
                    <div class="panel-form-group">
                        <label for="ntfy-server-url">Server URL</label>
                        <input type="url" id="ntfy-server-url" class="panel-input"
                               placeholder="https://ntfy.sh" value="${escapeHtml(channelConfig.url || '')}">
                    </div>
                    <div class="panel-form-group">
                        <label for="ntfy-topic">Topic</label>
                        <input type="text" id="ntfy-topic" class="panel-input"
                               placeholder="my-topic" value="${escapeHtml(channelConfig.topic || '')}">
                    </div>
                    <div class="panel-form-group">
                        <label for="ntfy-token">Access Token (optional)</label>
                        <input type="password" id="ntfy-token" class="panel-input"
                               placeholder="tk_..." value="${escapeHtml(channelConfig.token || '')}">
                    </div>
                </div>

                <!-- Pushover Configuration -->
                <div id="pushover-config" class="channel-config-group" style="${channelType === 'pushover' ? '' : 'display: none;'}">
                    <div class="panel-form-group">
                        <label for="pushover-api-key">API Key</label>
                        <input type="text" id="pushover-api-key" class="panel-input"
                               placeholder="a..." value="${escapeHtml(channelConfig.api_key || '')}">
                    </div>
                </div>

                <!-- Webhook Configuration -->
                <div id="webhook-config" class="channel-config-group" style="${channelType === 'webhook' ? '' : 'display: none;'}">
                    <div class="panel-form-group">
                        <label for="webhook-url">Webhook URL</label>
                        <input type="url" id="webhook-url" class="panel-input"
                               placeholder="https://example.com/webhook" value="${escapeHtml(channelConfig.url || '')}">
                    </div>
                </div>

                <hr class="panel-divider">

                <!-- Event Type Toggles -->
                <div class="panel-form-group">
                    <label>Event Types</label>
                    <div class="event-type-toggles">
                        ${renderEventTypeToggle('zone_enter', 'Zone Entry', 'When someone enters a zone', eventTypes.zone_enter)}
                        ${renderEventTypeToggle('zone_leave', 'Zone Leave', 'When someone leaves a zone', eventTypes.zone_leave)}
                        ${renderEventTypeToggle('zone_vacant', 'Zone Vacant', 'When a zone becomes empty', eventTypes.zone_vacant)}
                        ${renderEventTypeToggle('fall_detected', 'Fall Detected', 'When a possible fall is detected', eventTypes.fall_detected)}
                        ${renderEventTypeToggle('fall_escalation', 'Fall Escalation', 'When fall is unacknowledged', eventTypes.fall_escalation)}
                        ${renderEventTypeToggle('anomaly_alert', 'Anomaly Alert', 'Unusual activity detected', eventTypes.anomaly_alert)}
                        ${renderEventTypeToggle('node_offline', 'Node Offline', 'When a node goes offline', eventTypes.node_offline)}
                        ${renderEventTypeToggle('sleep_summary', 'Sleep Summary', 'Daily sleep quality report', eventTypes.sleep_summary)}
                    </div>
                </div>

                <hr class="panel-divider">

                <!-- Quiet Hours -->
                <div class="panel-form-group">
                    <label class="panel-form-checkbox">
                        <input type="checkbox" id="quiet-hours-enabled" ${quietHoursEnabled ? 'checked' : ''}>
                        <span>Enable Quiet Hours</span>
                    </label>
                </div>

                <div id="quiet-hours-fields" style="${quietHoursEnabled ? '' : 'display: none;'}">
                    <div class="panel-time-range">
                        <div class="panel-time-field">
                            <label for="quiet-hours-start">From</label>
                            <input type="time" id="quiet-hours-start" class="panel-input" value="${quietHoursStart}">
                        </div>
                        <div class="panel-time-field">
                            <label for="quiet-hours-end">To</label>
                            <input type="time" id="quiet-hours-end" class="panel-input" value="${quietHoursEnd}">
                        </div>
                    </div>
                    <div class="panel-days-selector">
                        <label>Active Days</label>
                        ${renderDayCheckboxes(quietHoursDays)}
                    </div>
                </div>

                <hr class="panel-divider">

                <!-- Morning Digest -->
                <div class="panel-form-group">
                    <label class="panel-form-checkbox">
                        <input type="checkbox" id="morning-digest-enabled" ${morningDigestEnabled ? 'checked' : ''}>
                        <span>Morning Digest</span>
                    </label>
                    <div class="panel-form-hint">Deliver queued events at wake time</div>
                </div>

                <div id="morning-digest-fields" style="${morningDigestEnabled ? '' : 'display: none;'}">
                    <div class="panel-form-group">
                        <label for="morning-digest-time">Digest Time</label>
                        <input type="time" id="morning-digest-time" class="panel-input" value="${morningDigestTime}">
                    </div>
                </div>

                <hr class="panel-divider">

                <!-- Smart Batching -->
                <div class="panel-form-group">
                    <label class="panel-form-checkbox">
                        <input type="checkbox" id="smart-batching-enabled" ${smartBatchingEnabled ? 'checked' : ''}>
                        <span>Smart Batching</span>
                    </label>
                    <div class="panel-form-hint">Combine multiple events into single notification</div>
                </div>

                <div id="smart-batching-fields" style="${smartBatchingEnabled ? '' : 'display: none;'}">
                    <div class="panel-form-group">
                        <label for="smart-batching-window">Batch Window (seconds)</label>
                        <input type="number" id="smart-batching-window" class="panel-input"
                               min="5" max="300" step="5" value="${smartBatchingWindow}">
                    </div>
                </div>

                <hr class="panel-divider">

                <!-- Test Button -->
                <button class="panel-btn panel-btn-secondary panel-btn-full" id="test-notification-btn">
                    Test Notification
                </button>

                <!-- Save Button -->
                <button class="panel-btn panel-btn-primary panel-btn-full" id="save-notification-btn"
                        ${settingsState.saving ? 'disabled' : ''}>
                    ${settingsState.saving ? 'Saving...' : 'Save Notification Settings'}
                </button>
            </div>
        `;
    }

    function renderEventTypeToggle(id, label, description, checked) {
        return `
            <div class="panel-event-type-toggle">
                <label class="panel-toggle-switch">
                    <input type="checkbox" class="event-type-checkbox" data-event="${id}" ${checked ? 'checked' : ''}>
                    <span class="panel-toggle-slider"></span>
                </label>
                <div class="panel-event-type-info">
                    <span class="panel-event-type-label">${label}</span>
                    <span class="panel-event-type-desc">${description}</span>
                </div>
            </div>
        `;
    }

    function renderDayCheckboxes(mask) {
        const days = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];
        return days.map((day, index) => {
            const isChecked = (mask & (1 << index)) !== 0;
            return `
                <label class="panel-day-checkbox">
                    <input type="checkbox" class="day-checkbox-input" data-day="${index}" ${isChecked ? 'checked' : ''}>
                    ${day}
                </label>
            `;
        }).join('');
    }

    function renderSystemInfo() {
        const system = window.SpaxelState ? window.SpaxelState.system : {};
        const version = system.version || 'unknown';
        const uptime = system.uptime_s || 0;
        const nodesOnline = system.nodes_online || 0;
        const nodesTotal = system.nodes_total || 0;
        const quality = system.detection_quality || 0;

        // Format uptime
        const uptimeHours = Math.floor(uptime / 3600);
        const uptimeMins = Math.floor((uptime % 3600) / 60);
        const uptimeText = uptimeHours > 0
            ? `${uptimeHours}h ${uptimeMins}m`
            : `${uptimeMins}m`;

        return `
            <div class="panel-section">
                <div class="panel-section-header">System Information</div>

                <div class="panel-info-card">
                    <div class="panel-info-card-title">Version</div>
                    <div class="panel-info-card-value">${escapeHtml(version)}</div>
                </div>

                <div class="panel-info-card">
                    <div class="panel-info-card-title">Uptime</div>
                    <div class="panel-info-card-value">${uptimeText}</div>
                </div>

                <div class="panel-info-card">
                    <div class="panel-info-card-title">Detection Quality</div>
                    <div class="panel-info-card-value">${Math.round(quality)}%</div>
                    <div class="panel-info-card-subtitle">System-wide confidence</div>
                </div>

                <div class="panel-info-card">
                    <div class="panel-info-card-title">Nodes</div>
                    <div class="panel-info-card-value">${nodesOnline} / ${nodesTotal}</div>
                    <div class="panel-info-card-subtitle">Online / Total</div>
                </div>

                <hr class="panel-divider">

                <button class="panel-btn panel-btn-danger panel-btn-full" id="logout-btn">
                    Logout
                </button>
            </div>
        `;
    }

    /**
     * Attach event listeners to the rendered content
     */
    function attachEventListeners() {
        // Range slider value updates
        const thresholdInput = document.getElementById('setting-threshold');
        const thresholdValue = document.getElementById('setting-threshold-value');
        if (thresholdInput && thresholdValue) {
            thresholdInput.addEventListener('input', function() {
                thresholdValue.textContent = (parseFloat(this.value) * 1000).toFixed(0);
            });
        }

        const gridCellInput = document.getElementById('setting-grid-cell');
        const gridCellValue = document.getElementById('setting-grid-cell-value');
        if (gridCellInput && gridCellValue) {
            gridCellInput.addEventListener('input', function() {
                gridCellValue.textContent = parseFloat(this.value).toFixed(2) + ' m';
            });
        }

        const fresnelDecayInput = document.getElementById('setting-fresnel-decay');
        const fresnelDecayValue = document.getElementById('setting-fresnel-decay-value');
        if (fresnelDecayInput && fresnelDecayValue) {
            fresnelDecayInput.addEventListener('input', function() {
                fresnelDecayValue.textContent = parseFloat(this.value).toFixed(1);
            });
        }

        const tauInput = document.getElementById('setting-tau');
        const tauValue = document.getElementById('setting-tau-value');
        if (tauInput && tauValue) {
            tauInput.addEventListener('input', function() {
                tauValue.textContent = parseInt(this.value) + ' s';
            });
        }

        // Channel type selector - show/hide relevant config fields
        const channelTypeSelect = document.getElementById('notification-channel-type');
        if (channelTypeSelect) {
            channelTypeSelect.addEventListener('change', function() {
                updateChannelConfigVisibility(this.value);
            });
        }

        // Quiet hours toggle
        const quietHoursEnabled = document.getElementById('quiet-hours-enabled');
        const quietHoursFields = document.getElementById('quiet-hours-fields');
        if (quietHoursEnabled && quietHoursFields) {
            quietHoursEnabled.addEventListener('change', function() {
                quietHoursFields.style.display = this.checked ? '' : 'none';
            });
        }

        // Morning digest toggle
        const morningDigestEnabled = document.getElementById('morning-digest-enabled');
        const morningDigestFields = document.getElementById('morning-digest-fields');
        if (morningDigestEnabled && morningDigestFields) {
            morningDigestEnabled.addEventListener('change', function() {
                morningDigestFields.style.display = this.checked ? '' : 'none';
            });
        }

        // Smart batching toggle
        const smartBatchingEnabled = document.getElementById('smart-batching-enabled');
        const smartBatchingFields = document.getElementById('smart-batching-fields');
        if (smartBatchingEnabled && smartBatchingFields) {
            smartBatchingEnabled.addEventListener('change', function() {
                smartBatchingFields.style.display = this.checked ? '' : 'none';
            });
        }

        // Save detection settings
        const saveDetectionBtn = document.getElementById('save-detection-btn');
        if (saveDetectionBtn) {
            saveDetectionBtn.addEventListener('click', saveDetectionSettings);
        }

        // Save notification settings
        const saveNotificationBtn = document.getElementById('save-notification-btn');
        if (saveNotificationBtn) {
            saveNotificationBtn.addEventListener('click', saveNotificationSettings);
        }

        // Test notification
        const testNotificationBtn = document.getElementById('test-notification-btn');
        if (testNotificationBtn) {
            testNotificationBtn.addEventListener('click', sendTestNotification);
        }

        // Logout
        const logoutBtn = document.getElementById('logout-btn');
        if (logoutBtn) {
            logoutBtn.addEventListener('click', handleLogout);
        }

        // Change PIN
        const changePinBtn = document.getElementById('change-pin-btn');
        if (changePinBtn) {
            changePinBtn.addEventListener('click', openChangePINModal);
        }
    }

    /**
     * Update channel config visibility based on selected channel type
     */
    function updateChannelConfigVisibility(channelType) {
        const ntfyConfig = document.getElementById('ntfy-config');
        const pushoverConfig = document.getElementById('pushover-config');
        const webhookConfig = document.getElementById('webhook-config');

        // Hide all first
        if (ntfyConfig) ntfyConfig.style.display = 'none';
        if (pushoverConfig) pushoverConfig.style.display = 'none';
        if (webhookConfig) webhookConfig.style.display = 'none';

        // Show selected
        switch (channelType) {
            case 'ntfy':
                if (ntfyConfig) ntfyConfig.style.display = '';
                break;
            case 'pushover':
                if (pushoverConfig) pushoverConfig.style.display = '';
                break;
            case 'webhook':
                if (webhookConfig) webhookConfig.style.display = '';
                break;
        }
    }

    /**
     * Save detection settings
     */
    function saveDetectionSettings() {
        const threshold = parseFloat(document.getElementById('setting-threshold').value);
        const fusionRate = parseInt(document.getElementById('setting-fusion-rate').value);
        const gridCell = parseFloat(document.getElementById('setting-grid-cell').value);
        const fresnelDecay = parseFloat(document.getElementById('setting-fresnel-decay').value);
        const nSubcarriers = parseInt(document.getElementById('setting-subcarriers').value);
        const tau = parseInt(document.getElementById('setting-tau').value);

        const updates = {
            delta_rms_threshold: threshold,
            fusion_rate_hz: fusionRate,
            grid_cell_m: gridCell,
            fresnel_decay: fresnelDecay,
            n_subcarriers: nSubcarriers,
            tau_s: tau
        };

        saveSettings(updates).then(function() {
            // Update local state settings
            if (window.SpaxelState) {
                Object.assign(window.SpaxelState.settings, updates);
            }

            // Track setting changes for proactive assistance
            if (window.Proactive) {
                // Track each qualifying setting that changed
                const qualifyingSettings = ['delta_rms_threshold', 'fresnel_decay', 'n_subcarriers', 'tau_s', 'breathing_sensitivity'];
                for (const key in updates) {
                    if (qualifyingSettings.includes(key)) {
                        window.Proactive.trackSettingChange(key, updates[key]);
                    }
                }
            }
        });
    }

    /**
     * Save notification settings
     */
    function saveNotificationSettings() {
        const channelType = document.getElementById('notification-channel-type').value;
        const channelConfig = {};

        // Get channel-specific config
        switch (channelType) {
            case 'ntfy':
                channelConfig.url = document.getElementById('ntfy-server-url').value || null;
                channelConfig.topic = document.getElementById('ntfy-topic').value || null;
                channelConfig.token = document.getElementById('ntfy-token').value || null;
                break;
            case 'pushover':
                channelConfig.api_key = document.getElementById('pushover-api-key').value || null;
                break;
            case 'webhook':
                channelConfig.url = document.getElementById('webhook-url').value || null;
                break;
        }

        // Get quiet hours settings
        const quietHoursEnabled = document.getElementById('quiet-hours-enabled').checked;
        const quietHoursStart = document.getElementById('quiet-hours-start').value;
        const quietHoursEnd = document.getElementById('quiet-hours-end').value;

        // Get quiet hours days mask
        let quietHoursDays = 0;
        document.querySelectorAll('.day-checkbox-input:checked').forEach(function(cb) {
            quietHoursDays |= (1 << parseInt(cb.dataset.day));
        });

        // Get morning digest settings
        const morningDigestEnabled = document.getElementById('morning-digest-enabled').checked;
        const morningDigestTime = document.getElementById('morning-digest-time').value;

        // Get smart batching settings
        const smartBatchingEnabled = document.getElementById('smart-batching-enabled').checked;
        const smartBatchingWindow = parseInt(document.getElementById('smart-batching-window').value);

        // Get event type preferences
        const eventTypes = {};
        document.querySelectorAll('.event-type-checkbox:checked').forEach(function(cb) {
            eventTypes[cb.dataset.event] = true;
        });
        document.querySelectorAll('.event-type-checkbox:not(:checked)').forEach(function(cb) {
            eventTypes[cb.dataset.event] = false;
        });

        // Build notification settings object
        const notificationSettings = {
            channel_type: channelType,
            channel_config: Object.keys(channelConfig).length > 0 ? channelConfig : null,
            quiet_hours_enabled: quietHoursEnabled,
            quiet_hours_start: quietHoursStart,
            quiet_hours_end: quietHoursEnd,
            quiet_hours_days: quietHoursDays,
            morning_digest_enabled: morningDigestEnabled,
            morning_digest_time: morningDigestTime,
            smart_batching_enabled: smartBatchingEnabled,
            smart_batching_window: smartBatchingWindow,
            event_types: eventTypes
        };

        // Send to API
        fetch('/api/settings/notifications', {
            method: 'PUT',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(notificationSettings)
        })
        .then(function(res) {
            if (!res.ok) {
                return res.json().then(function(err) {
                    throw new Error(err.error || 'Failed to save notification settings');
                });
            }
            return res.json();
        })
        .then(function(data) {
            // Update local state
            settingsState.notificationSettings = data;
            SpaxelPanels.showSuccess('Notification settings saved successfully');
            renderContent();
        })
        .catch(function(err) {
            console.error('[SettingsPanel] Error saving notification settings:', err);
            SpaxelPanels.showError('Failed to save notification settings: ' + err.message);
        });
    }

    /**
     * Send a test notification
     */
    function sendTestNotification() {
        SpaxelPanels.showInfo('Sending test notification...');

        // Get current channel type from settings
        const channelType = document.getElementById('notification-channel-type').value;

        if (channelType === 'none') {
            SpaxelPanels.showError('Please configure a notification channel first');
            return;
        }

        fetch('/api/notifications/test', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({
                channel_type: channelType,
                title: 'Spaxel Test Notification',
                body: 'This is a test notification from Spaxel.',
                data: { test: true }
            })
        })
        .then(function(res) {
            if (!res.ok) {
                return res.json().then(function(err) {
                    throw new Error(err.error || 'Failed to send test notification');
                });
            }
            return res.json();
        })
        .then(function(data) {
            if (data.status === 'sent') {
                SpaxelPanels.showSuccess('Test notification sent successfully!');
            } else if (data.status === 'simulated') {
                SpaxelPanels.showInfo('Test notification simulated (notification sender not attached)');
            } else {
                SpaxelPanels.showSuccess(data.message || 'Test notification processed');
            }
        })
        .catch(function(err) {
            console.error('[SettingsPanel] Error sending test notification:', err);
            SpaxelPanels.showError('Failed to send test notification: ' + err.message);
        });
    }

    /**
     * Handle logout
     */
    function handleLogout() {
        // Auth is handled by Traefik/Google OAuth — no in-app logout needed
    }

    function escapeHtml(text) {
        if (!text) return '';
        return String(text)
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;');
    }

    // ============================================
    // Change PIN Modal
    // ============================================

    const changePINState = {
        oldPin: '',
        newPin: '',
        confirmPin: '',
        error: '',
        isSubmitting: false
    };

    function openChangePINModal() {
        changePINState.oldPin = '';
        changePINState.newPin = '';
        changePINState.confirmPin = '';
        changePINState.error = '';
        changePINState.isSubmitting = false;

        const modal = document.createElement('div');
        modal.id = 'change-pin-modal';
        modal.className = 'panel-modal-overlay';
        modal.innerHTML = renderChangePINModal();
        document.body.appendChild(modal);

        attachChangePINEvents(modal);

        // Focus first input
        setTimeout(function() {
            const firstInput = modal.querySelector('#change-pin-old');
            if (firstInput) firstInput.focus();
        }, 10);
    }

    function closeChangePINModal() {
        const modal = document.getElementById('change-pin-modal');
        if (modal) {
            modal.remove();
        }
    }

    function renderChangePINModal() {
        return `
            <div class="panel-modal">
                <div class="panel-modal-header">
                    <h2>Change PIN</h2>
                    <button class="panel-modal-close" id="change-pin-close">&times;</button>
                </div>
                <div class="panel-modal-body">
                    ${changePINState.error ? `
                        <div class="panel-error" id="change-pin-error">${escapeHtml(changePINState.error)}</div>
                    ` : ''}

                    <div class="panel-form-group">
                        <label for="change-pin-old">Current PIN</label>
                        <input type="password" id="change-pin-old" class="panel-input"
                               maxlength="8" placeholder="Enter current PIN" autocomplete="current-password">
                    </div>

                    <div class="panel-form-group">
                        <label for="change-pin-new">New PIN</label>
                        <input type="password" id="change-pin-new" class="panel-input"
                               maxlength="8" placeholder="Enter new PIN (4-8 digits)" autocomplete="new-password">
                    </div>

                    <div class="panel-form-group">
                        <label for="change-pin-confirm">Confirm New PIN</label>
                        <input type="password" id="change-pin-confirm" class="panel-input"
                               maxlength="8" placeholder="Confirm new PIN" autocomplete="new-password">
                    </div>

                    <div class="panel-modal-actions">
                        <button class="panel-btn panel-btn-secondary" id="change-pin-cancel" ${changePINState.isSubmitting ? 'disabled' : ''}>
                            Cancel
                        </button>
                        <button class="panel-btn panel-btn-primary" id="change-pin-submit" ${changePINState.isSubmitting ? 'disabled' : ''}>
                            ${changePINState.isSubmitting ? 'Changing...' : 'Change PIN'}
                        </button>
                    </div>
                </div>
            </div>
        `;
    }

    function attachChangePINEvents(modal) {
        // Close button
        const closeBtn = modal.querySelector('#change-pin-close');
        if (closeBtn) {
            closeBtn.addEventListener('click', closeChangePINModal);
        }

        // Cancel button
        const cancelBtn = modal.querySelector('#change-pin-cancel');
        if (cancelBtn) {
            cancelBtn.addEventListener('click', closeChangePINModal);
        }

        // Submit button
        const submitBtn = modal.querySelector('#change-pin-submit');
        if (submitBtn) {
            submitBtn.addEventListener('click', submitChangePIN);
        }

        // Close on overlay click
        modal.addEventListener('click', function(e) {
            if (e.target === modal) {
                closeChangePINModal();
            }
        });

        // Handle Enter key
        const inputs = modal.querySelectorAll('.panel-input');
        inputs.forEach(function(input) {
            input.addEventListener('keydown', function(e) {
                if (e.key === 'Enter') {
                    submitChangePIN();
                }
            });

            // Only allow digits
            input.addEventListener('input', function(e) {
                const value = e.target.value;
                if (!/^\d*$/.test(value)) {
                    e.target.value = value.replace(/\D/g, '');
                }
            });
        });
    }

    function submitChangePIN() {
        const oldPinInput = document.getElementById('change-pin-old');
        const newPinInput = document.getElementById('change-pin-new');
        const confirmPinInput = document.getElementById('change-pin-confirm');

        if (!oldPinInput || !newPinInput || !confirmPinInput) {
            return;
        }

        const oldPin = oldPinInput.value.trim();
        const newPin = newPinInput.value.trim();
        const confirmPin = confirmPinInput.value.trim();

        // Validation
        if (!oldPin || oldPin.length < 4) {
            changePINState.error = 'Please enter your current PIN (4-8 digits)';
            updateChangePINModal();
            return;
        }

        if (!newPin || newPin.length < 4 || newPin.length > 8) {
            changePINState.error = 'New PIN must be 4-8 digits';
            updateChangePINModal();
            return;
        }

        if (newPin !== confirmPin) {
            changePINState.error = 'New PINs do not match';
            updateChangePINModal();
            return;
        }

        if (oldPin === newPin) {
            changePINState.error = 'New PIN must be different from current PIN';
            updateChangePINModal();
            return;
        }

        changePINState.oldPin = oldPin;
        changePINState.newPin = newPin;
        changePINState.confirmPin = confirmPin;
        changePINState.error = '';
        changePINState.isSubmitting = true;
        updateChangePINModal();

        // Send change PIN request
        fetch('/api/auth/change-pin', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({
                old_pin: oldPin,
                new_pin: newPin
            })
        })
        .then(function(res) {
            if (res.status === 403) {
                throw new Error('Incorrect current PIN');
            }
            if (!res.ok) {
                return res.text().then(function(text) {
                    throw new Error(text || 'Failed to change PIN');
                });
            }
            return res.json();
        })
        .then(function(data) {
            closeChangePINModal();
            SpaxelPanels.showSuccess('PIN changed successfully');
        })
        .catch(function(err) {
            changePINState.error = err.message || 'Failed to change PIN';
            changePINState.isSubmitting = false;
            updateChangePINModal();
        });
    }

    function updateChangePINModal() {
        const modal = document.getElementById('change-pin-modal');
        if (!modal) return;

        const modalContent = modal.querySelector('.panel-modal');
        if (modalContent) {
            modalContent.innerHTML = renderChangePINModal().match(/<div class="panel-modal">([\s\S]*)<\/div>/)[1];
            attachChangePINEvents(modal);
        }
    }

    // ============================================
    // Panel Registration
    // ============================================

    /**
     * Open the settings panel
     */
    function openSettingsPanel() {
        // Fetch settings first, then open panel
        fetchSettings().then(function() {
            SpaxelPanels.openSidebar({
                title: 'Settings',
                content: '<div id="settings-panel-content"></div>',
                width: '400px',
                onOpen: function() {
                    renderContent();
                }
            });
        }).catch(function() {
            // Open panel anyway with error state
            SpaxelPanels.openSidebar({
                title: 'Settings',
                content: '<div id="settings-panel-content">' + renderLoading() + '</div>',
                width: '400px'
            });
        });
    }

    // Register the settings panel
    if (window.SpaxelPanels) {
        SpaxelPanels.register('settings', openSettingsPanel);
    }

    // Also register as a global function for direct access
    window.openSettingsPanel = openSettingsPanel;

    // ============================================
    // Router Integration
    // ============================================

    // Auto-open settings panel when navigating to #settings
    if (window.SpaxelRouter) {
        SpaxelRouter.onModeChange(function(newMode) {
            if (newMode === 'settings') {
                openSettingsPanel();
            }
        });
    }

    console.log('[SettingsPanel] Settings panel module loaded');
})();
