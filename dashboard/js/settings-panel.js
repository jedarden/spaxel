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
        currentSettings: null
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

        return fetch('/api/settings')
            .then(function(res) {
                if (!res.ok) {
                    throw new Error('Failed to fetch settings: ' + res.status);
                }
                return res.json();
            })
            .then(function(data) {
                settingsState.currentSettings = data;
                settingsState.loading = false;
                renderContent();
                return data;
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

    function renderNotificationSettings(settings) {
        const notificationChannels = settings.notification_channels || {};
        const ntfyEnabled = notificationChannels.ntfy && notificationChannels.ntfy.enabled;
        const ntfyUrl = notificationChannels.ntfy && notificationChannels.ntfy.config ? (notificationChannels.ntfy.config.url || '') : '';
        const ntfyToken = notificationChannels.ntfy && notificationChannels.ntfy.config ? (notificationChannels.ntfy.config.token || '') : '';

        const pushoverEnabled = notificationChannels.pushover && notificationChannels.pushover.enabled;
        const pushoverToken = notificationChannels.pushover && notificationChannels.pushover.config ? (notificationChannels.pushover.config.user_key || '') : '';
        const pushoverApp = notificationChannels.pushover && notificationChannels.pushover.config ? (notificationChannels.pushover.config.app_token || '') : '';

        return `
            <div class="panel-section">
                <div class="panel-section-header">Notification Channels</div>

                <div class="panel-form-group">
                    <label class="panel-form-checkbox">
                        <input type="checkbox" id="setting-ntfy-enabled" ${ntfyEnabled ? 'checked' : ''}>
                        <span>Enable Ntfy Notifications</span>
                    </label>
                </div>

                <div class="panel-form-group" id="ntfy-settings" style="${ntfyEnabled ? '' : 'display: none;'}">
                    <label for="setting-ntfy-url">Ntfy Topic URL</label>
                    <input type="url" id="setting-ntfy-url" placeholder="https://ntfy.sh/my-topic" value="${escapeHtml(ntfyUrl)}">
                </div>

                <div class="panel-form-group" id="ntfy-token-setting" style="${ntfyEnabled ? '' : 'display: none;'}">
                    <label for="setting-ntfy-token">Access Token (optional)</label>
                    <input type="password" id="setting-ntfy-token" placeholder="tk_..." value="${escapeHtml(ntfyToken)}">
                </div>

                <hr class="panel-divider">

                <div class="panel-form-group">
                    <label class="panel-form-checkbox">
                        <input type="checkbox" id="setting-pushover-enabled" ${pushoverEnabled ? 'checked' : ''}>
                        <span>Enable Pushover Notifications</span>
                    </label>
                </div>

                <div class="panel-form-group" id="pushover-settings" style="${pushoverEnabled ? '' : 'display: none;'}">
                    <label for="setting-pushover-token">Pushover User Key</label>
                    <input type="text" id="setting-pushover-token" placeholder="u..." value="${escapeHtml(pushoverToken)}">
                </div>

                <div class="panel-form-group" id="pushover-app-setting" style="${pushoverEnabled ? '' : 'display: none;'}">
                    <label for="setting-pushover-app">Application Token</label>
                    <input type="text" id="setting-pushover-app" placeholder="a..." value="${escapeHtml(pushoverApp)}">
                </div>

                <button class="panel-btn panel-btn-primary panel-btn-full" id="save-notification-btn"
                        ${settingsState.saving ? 'disabled' : ''}>
                    ${settingsState.saving ? 'Saving...' : 'Save Notification Settings'}
                </button>

                <button class="panel-btn panel-btn-secondary panel-btn-full" id="test-notification-btn">
                    Test Notification
                </button>
            </div>
        `;
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

        // Ntfy toggle
        const ntfyEnabled = document.getElementById('setting-ntfy-enabled');
        const ntfySettings = document.getElementById('ntfy-settings');
        const ntfyTokenSetting = document.getElementById('ntfy-token-setting');
        if (ntfyEnabled && ntfySettings && ntfyTokenSetting) {
            ntfyEnabled.addEventListener('change', function() {
                const visible = this.checked;
                ntfySettings.style.display = visible ? '' : 'none';
                ntfyTokenSetting.style.display = visible ? '' : 'none';
            });
        }

        // Pushover toggle
        const pushoverEnabled = document.getElementById('setting-pushover-enabled');
        const pushoverSettings = document.getElementById('pushover-settings');
        const pushoverAppSetting = document.getElementById('pushover-app-setting');
        if (pushoverEnabled && pushoverSettings && pushoverAppSetting) {
            pushoverEnabled.addEventListener('change', function() {
                const visible = this.checked;
                pushoverSettings.style.display = visible ? '' : 'none';
                pushoverAppSetting.style.display = visible ? '' : 'none';
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
        });
    }

    /**
     * Save notification settings
     */
    function saveNotificationSettings() {
        const ntfyEnabled = document.getElementById('setting-ntfy-enabled').checked;
        const ntfyUrl = document.getElementById('setting-ntfy-url').value;
        const ntfyToken = document.getElementById('setting-ntfy-token').value;

        const pushoverEnabled = document.getElementById('setting-pushover-enabled').checked;
        const pushoverToken = document.getElementById('setting-pushover-token').value;
        const pushoverApp = document.getElementById('setting-pushover-app').value;

        const updates = {
            notification_channels: {
                ntfy: {
                    enabled: ntfyEnabled,
                    config: {
                        url: ntfyUrl || null,
                        token: ntfyToken || null
                    }
                },
                pushover: {
                    enabled: pushoverEnabled,
                    config: {
                        user_key: pushoverToken || null,
                        app_token: pushoverApp || null
                    }
                }
            }
        };

        saveSettings(updates);
    }

    /**
     * Send a test notification
     */
    function sendTestNotification() {
        SpaxelPanels.showInfo('Sending test notification...');

        fetch('/api/notifications/test', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({
                channel_type: 'all'
            })
        })
        .then(function(res) {
            if (!res.ok) {
                throw new Error('Failed to send test notification');
            }
            return res.json();
        })
        .then(function(data) {
            SpaxelPanels.showSuccess('Test notification sent!');
        })
        .catch(function(err) {
            console.error('[SettingsPanel] Error sending test notification:', err);
            SpaxelPanels.showError('Failed to send test notification');
        });
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
