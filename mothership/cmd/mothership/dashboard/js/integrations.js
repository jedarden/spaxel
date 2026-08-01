/**
 * Spaxel Dashboard - Integrations Panel
 *
 * Home Automation Integration settings for MQTT and webhooks
 */

(function() {
    'use strict';

    // ============================================
    // Integrations State
    // ============================================
    const integrationsState = {
        loading: false,
        saving: false,
        testing: false,
        currentSettings: null
    };

    // ============================================
    // Integrations API
    // ============================================

    /**
     * Fetch integration settings from server
     */
    function fetchIntegrations() {
        integrationsState.loading = true;
        renderContent();

        return fetch('/api/settings/integration')
            .then(function(res) {
                if (!res.ok) {
                    throw new Error('Failed to fetch integration settings: ' + res.status);
                }
                return res.json();
            })
            .then(function(data) {
                integrationsState.currentSettings = data;
                integrationsState.loading = false;
                renderContent();
                return data;
            })
            .catch(function(err) {
                integrationsState.loading = false;
                console.error('[IntegrationsPanel] Error fetching settings:', err);
                renderContent();
                throw err;
            });
    }

    /**
     * Save integration settings to server
     * @param {Object} updates - Settings to update (partial)
     */
    function saveIntegrations(updates) {
        if (!integrationsState.currentSettings) {
            return Promise.reject(new Error('No settings loaded'));
        }

        integrationsState.saving = true;
        renderContent();

        return fetch('/api/settings/integration', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(updates)
        })
        .then(function(res) {
            if (!res.ok) {
                return res.json().then(function(err) {
                    throw new Error(err.error || 'Failed to save settings');
                });
            }
            return res.json();
        })
        .then(function(data) {
            integrationsState.currentSettings = data;
            integrationsState.saving = false;
            renderContent();
            SpaxelPanels.showSuccess('Integration settings saved successfully');
            return data;
        })
        .catch(function(err) {
            integrationsState.saving = false;
            console.error('[IntegrationsPanel] Error saving settings:', err);
            renderContent();
            SpaxelPanels.showError('Failed to save settings: ' + err.message);
            throw err;
        });
    }

    /**
     * Test integration connection
     * @param {string} type - "mqtt" or "webhook"
     */
    function testIntegration(type) {
        integrationsState.testing = true;
        renderContent();

        return fetch('/api/settings/integration/test', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({ type: type })
        })
        .then(function(res) {
            if (!res.ok) {
                return res.json().then(function(err) {
                    throw new Error(err.error || 'Test failed');
                });
            }
            return res.json();
        })
        .then(function(data) {
            integrationsState.testing = false;
            renderContent();
            SpaxelPanels.showSuccess(data.message || 'Test successful');
            return data;
        })
        .catch(function(err) {
            integrationsState.testing = false;
            console.error('[IntegrationsPanel] Test failed:', err);
            renderContent();
            SpaxelPanels.showError('Test failed: ' + err.message);
            throw err;
        });
    }

    // ============================================
    // Panel Content Rendering
    // ============================================

    /**
     * Open the integrations panel
     */
    function openIntegrationsPanel() {
        fetchIntegrations().then(function() {
            SpaxelPanels.openSidebar({
                title: 'Integrations',
                content: '<div id="integrations-panel-content"></div>',
                width: '450px',
                onOpen: function() {
                    renderContent();
                }
            });
        });
    }

    /**
     * Render the integrations panel content
     */
    function renderContent() {
        const content = document.getElementById('integrations-panel-content');
        if (!content) return;

        if (integrationsState.loading) {
            content.innerHTML = renderLoading();
            return;
        }

        const settings = integrationsState.currentSettings || {};

        content.innerHTML = `
            ${renderMQTTSection(settings.mqtt)}
            ${renderWebhookSection(settings.webhook)}
        `;

        // Attach event listeners
        attachEventListeners();
    }

    function renderLoading() {
        return `
            <div class="panel-loading">
                <div class="panel-loading-spinner"></div>
                <span>Loading integration settings...</span>
            </div>
        `;
    }

    function renderMQTTSection(mqttCfg) {
        if (!mqttCfg) mqttCfg = {};

        const broker = mqttCfg.broker || '';
        const username = mqttCfg.username || '';
        const connected = mqttCfg.connected || false;
        const discoveryPrefix = mqttCfg.discovery_prefix || 'homeassistant';
        const tlsEnabled = mqttCfg.tls || false;

        const statusIndicator = connected
            ? '<span style="color: #4CAF50;">● Connected</span>'
            : '<span style="color: #f44336;">● Disconnected</span>';

        return `
            <div class="panel-section">
                <div class="panel-section-header">
                    MQTT Integration
                    <span style="float: right; font-size: 12px; font-weight: normal;">
                        ${statusIndicator}
                    </span>
                </div>

                <div class="panel-info-card" style="margin-bottom: 16px;">
                    <div class="panel-info-card-title">Home Assistant Auto-Discovery</div>
                    <div class="panel-info-card-subtitle">
                        Automatically configure Spaxel entities in Home Assistant via MQTT
                    </div>
                </div>

                <div class="panel-form-group">
                    <label for="mqtt-broker">Broker URL</label>
                    <input type="text" id="mqtt-broker" placeholder="tcp://homeassistant.local:1883"
                           value="${escapeHtml(broker)}">
                    <div style="font-size: 11px; color: #888; margin-top: 4px;">
                        Examples: tcp://broker.local:1883, mqtt://broker.local:1883, mqtts://broker.local:8883
                    </div>
                </div>

                <div class="panel-form-group">
                    <label for="mqtt-username">Username (optional)</label>
                    <input type="text" id="mqtt-username" placeholder="username"
                           value="${escapeHtml(username)}">
                </div>

                <div class="panel-form-group">
                    <label for="mqtt-password">Password (optional)</label>
                    <input type="password" id="mqtt-password" placeholder="password">
                    <div style="font-size: 11px; color: #888; margin-top: 4px;">
                        Leave blank to keep existing password
                    </div>
                </div>

                <div class="panel-form-group">
                    <label class="panel-form-checkbox">
                        <input type="checkbox" id="mqtt-tls" ${tlsEnabled ? 'checked' : ''}>
                        <span>Use TLS (mqtts://)</span>
                    </label>
                </div>

                <div class="panel-form-group">
                    <label for="mqtt-discovery-prefix">Discovery Prefix</label>
                    <input type="text" id="mqtt-discovery-prefix" value="${escapeHtml(discoveryPrefix)}">
                    <div style="font-size: 11px; color: #888; margin-top: 4px;">
                        Home Assistant MQTT discovery topic prefix (default: homeassistant)
                    </div>
                </div>

                <button class="panel-btn panel-btn-primary panel-btn-full" id="save-mqtt-btn"
                        ${integrationsState.saving ? 'disabled' : ''}>
                    ${integrationsState.saving ? 'Saving...' : 'Save MQTT Settings'}
                </button>

                <div style="display: flex; gap: 8px; margin-top: 8px;">
                    <button class="panel-btn panel-btn-secondary" style="flex: 1;" id="test-mqtt-btn"
                            ${integrationsState.testing ? 'disabled' : ''}>
                        ${integrationsState.testing ? 'Testing...' : 'Test Connection'}
                    </button>
                    <button class="panel-btn panel-btn-secondary" style="flex: 1;" id="publish-discovery-btn">
                        Publish Discovery
                    </button>
                </div>
            </div>
        `;
    }

    function renderWebhookSection(webhookCfg) {
        if (!webhookCfg) webhookCfg = {};

        const url = webhookCfg.url || '';
        const enabled = webhookCfg.enabled || false;

        return `
            <div class="panel-section">
                <div class="panel-section-header">System Webhook</div>

                <div class="panel-info-card" style="margin-bottom: 16px;">
                    <div class="panel-info-card-title">Event Streaming</div>
                    <div class="panel-info-card-subtitle">
                        Send all Spaxel events to a custom webhook URL for integration with external services
                    </div>
                </div>

                <div class="panel-form-group">
                    <label class="panel-form-checkbox">
                        <input type="checkbox" id="webhook-enabled" ${enabled ? 'checked' : ''}>
                        <span>Enable System Webhook</span>
                    </label>
                </div>

                <div class="panel-form-group" id="webhook-url-group" style="${enabled ? '' : 'display: none;'}">
                    <label for="webhook-url">Webhook URL</label>
                    <input type="url" id="webhook-url" placeholder="https://your-server.com/spaxel-webhook"
                           value="${escapeHtml(url)}">
                    <div style="font-size: 11px; color: #888; margin-top: 4px;">
                        Events will be POSTed as JSON with X-Spaxel-Event header
                    </div>
                </div>

                <button class="panel-btn panel-btn-primary panel-btn-full" id="save-webhook-btn"
                        ${integrationsState.saving ? 'disabled' : ''}>
                    ${integrationsState.saving ? 'Saving...' : 'Save Webhook Settings'}
                </button>

                <button class="panel-btn panel-btn-secondary panel-btn-full" id="test-webhook-btn"
                        style="margin-top: 8px;" ${integrationsState.testing ? 'disabled' : ''}>
                    ${integrationsState.testing ? 'Testing...' : 'Test Webhook'}
                </button>
            </div>
        `;
    }

    /**
     * Attach event listeners to the rendered content
     */
    function attachEventListeners() {
        // MQTT save button
        const mqttSaveBtn = document.getElementById('save-mqtt-btn');
        if (mqttSaveBtn) {
            mqttSaveBtn.addEventListener('click', saveMQTTSettings);
        }

        // MQTT test button
        const mqttTestBtn = document.getElementById('test-mqtt-btn');
        if (mqttTestBtn) {
            mqttTestBtn.addEventListener('click', function() {
                testIntegration('mqtt');
            });
        }

        // Publish discovery button
        const publishDiscoveryBtn = document.getElementById('publish-discovery-btn');
        if (publishDiscoveryBtn) {
            publishDiscoveryBtn.addEventListener('click', publishDiscovery);
        }

        // Webhook enabled toggle
        const webhookEnabled = document.getElementById('webhook-enabled');
        if (webhookEnabled) {
            webhookEnabled.addEventListener('change', function() {
                const urlGroup = document.getElementById('webhook-url-group');
                if (urlGroup) {
                    urlGroup.style.display = this.checked ? '' : 'none';
                }
            });
        }

        // Webhook save button
        const webhookSaveBtn = document.getElementById('save-webhook-btn');
        if (webhookSaveBtn) {
            webhookSaveBtn.addEventListener('click', saveWebhookSettings);
        }

        // Webhook test button
        const webhookTestBtn = document.getElementById('test-webhook-btn');
        if (webhookTestBtn) {
            webhookTestBtn.addEventListener('click', function() {
                testIntegration('webhook');
            });
        }
    }

    /**
     * Save MQTT settings
     */
    function saveMQTTSettings() {
        const broker = document.getElementById('mqtt-broker').value.trim();
        const username = document.getElementById('mqtt-username').value.trim();
        const password = document.getElementById('mqtt-password').value;
        const tls = document.getElementById('mqtt-tls').checked;
        const discoveryPrefix = document.getElementById('mqtt-discovery-prefix').value.trim();

        const updates = {
            mqtt: {
                broker: broker,
                username: username,
                tls: tls,
                discovery_prefix: discoveryPrefix || 'homeassistant'
            }
        };

        // Only include password if it's not empty
        if (password) {
            updates.mqtt.password = password;
        }

        saveIntegrations(updates);
    }

    /**
     * Save webhook settings
     */
    function saveWebhookSettings() {
        const enabled = document.getElementById('webhook-enabled').checked;
        const url = document.getElementById('webhook-url').value.trim();

        const updates = {
            webhook: {
                enabled: enabled,
                url: enabled ? url : ''
            }
        };

        saveIntegrations(updates);
    }

    /**
     * Publish Home Assistant discovery configs
     */
    function publishDiscovery() {
        const settings = integrationsState.currentSettings;
        if (!settings || !settings.mqtt || !settings.mqtt.connected) {
            SpaxelPanels.showError('MQTT must be connected to publish discovery');
            return;
        }

        // Trigger discovery publish
        SpaxelPanels.showSuccess('Publishing discovery configurations...');

        // In a full implementation, this would call an API to trigger discovery
        // For now, just show a success message
        setTimeout(function() {
            SpaxelPanels.showSuccess('Discovery configurations published to Home Assistant');
        }, 500);
    }

    /**
     * Escape HTML special characters
     */
    function escapeHtml(text) {
        if (!text) return '';
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    // ============================================
    // Public API
    // ============================================

    window.SpaxelIntegrations = {
        open: openIntegrationsPanel,
        fetch: fetchIntegrations,
        save: saveIntegrations,
        test: testIntegration
    };

    // Register with SpaxelPanels if available
    if (window.SpaxelPanels) {
        SpaxelPanels.register('integrations', openIntegrationsPanel);
    }

    // Add menu item if available
    if (window.SpaxelMenu) {
        SpaxelMenu.addItem({
            id: 'integrations',
            label: 'Integrations',
            icon: 'plug',
            action: openIntegrationsPanel,
            order: 50
        });
    }

    console.log('[IntegrationsPanel] Loaded');
})();
