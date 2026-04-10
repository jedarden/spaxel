/**
 * Spaxel Dashboard - Morning Briefing Module
 *
 * Displays morning briefing with sleep, anomaly, and system summaries.
 * Supports push notifications and configurable delivery time.
 */

(function() {
    'use strict';

    // Briefing state
    const briefingState = {
        currentBriefing: null,
        isVisible: false,
        isDismissed: false,
        settings: {
            enabled: true,
            time: '07:00',
            pushNotification: false,
            autoGenerate: true
        },
        lastCheckDate: null
    };

    // DOM elements
    const elements = {};

    // Initialize briefing module
    function init() {
        // Cache DOM elements
        elements.card = document.getElementById('briefing-card');
        elements.content = document.getElementById('briefing-content-text');
        elements.dateText = document.getElementById('briefing-date-text');
        elements.closeBtn = document.getElementById('briefing-close');
        elements.dismissBtn = document.getElementById('briefing-dismiss');
        elements.refreshBtn = document.getElementById('briefing-refresh');
        elements.actions = document.getElementById('briefing-actions');
        elements.indicator = document.getElementById('briefing-indicator');
        elements.settingsPanel = document.getElementById('briefing-settings');
        elements.briefingEnabledToggle = document.getElementById('briefing-enabled-toggle');
        elements.briefingTimeInput = document.getElementById('briefing-time-input');
        elements.briefingPushToggle = document.getElementById('briefing-push-toggle');
        elements.briefingAutoToggle = document.getElementById('briefing-auto-toggle');
        elements.settingsCancel = document.getElementById('briefing-settings-cancel');
        elements.settingsSave = document.getElementById('briefing-settings-save');

        // Bind event listeners
        if (elements.closeBtn) {
            elements.closeBtn.addEventListener('click', hideBriefing);
        }
        if (elements.dismissBtn) {
            elements.dismissBtn.addEventListener('click', dismissBriefing);
        }
        if (elements.refreshBtn) {
            elements.refreshBtn.addEventListener('click', refreshBriefing);
        }
        if (elements.indicator) {
            elements.indicator.addEventListener('click', showBriefing);
        }
        if (elements.settingsCancel) {
            elements.settingsCancel.addEventListener('click', hideSettings);
        }
        if (elements.settingsSave) {
            elements.settingsSave.addEventListener('click', saveSettings);
        }
        if (elements.briefingEnabledToggle) {
            elements.briefingEnabledToggle.addEventListener('click', () => {
                elements.briefingEnabledToggle.classList.toggle('active');
                briefingState.settings.enabled = elements.briefingEnabledToggle.classList.contains('active');
            });
        }
        if (elements.briefingPushToggle) {
            elements.briefingPushToggle.addEventListener('click', () => {
                elements.briefingPushToggle.classList.toggle('active');
                briefingState.settings.pushNotification = elements.briefingPushToggle.classList.contains('active');
            });
        }
        if (elements.briefingAutoToggle) {
            elements.briefingAutoToggle.addEventListener('click', () => {
                elements.briefingAutoToggle.classList.toggle('active');
                briefingState.settings.autoGenerate = elements.briefingAutoToggle.classList.contains('active');
            });
        }

        // Load settings from localStorage
        loadSettings();

        // Check for briefing on page load
        checkForBriefing();

        // Check every minute for new briefing
        setInterval(checkForBriefing, 60000);

        console.log('[Spaxel] Briefing module initialized');
    }

    // Load settings from localStorage
    function loadSettings() {
        try {
            const stored = localStorage.getItem('spaxel_briefing_settings');
            if (stored) {
                const parsed = JSON.parse(stored);
                Object.assign(briefingState.settings, parsed);
                updateSettingsUI();
            }
        } catch (e) {
            console.warn('[Spaxel] Failed to load briefing settings:', e);
        }
    }

    // Save settings to localStorage
    function saveSettings() {
        try {
            // Update settings from UI
            if (elements.briefingTimeInput) {
                briefingState.settings.time = elements.briefingTimeInput.value;
            }
            briefingState.settings.enabled = elements.briefingEnabledToggle.classList.contains('active');
            briefingState.settings.pushNotification = elements.briefingPushToggle.classList.contains('active');
            briefingState.settings.autoGenerate = elements.briefingAutoToggle.classList.contains('active');

            localStorage.setItem('spaxel_briefing_settings', JSON.stringify(briefingState.settings));

            // Send to server
            fetch('/api/briefing/settings', {
                method: 'PATCH',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(briefingState.settings)
            }).then(() => {
                console.log('[Spaxel] Briefing settings saved');
            }).catch(err => {
                console.warn('[Spaxel] Failed to save briefing settings:', err);
            });

            hideSettings();
        } catch (e) {
            console.warn('[Spaxel] Failed to save briefing settings:', e);
        }
    }

    // Update settings UI from state
    function updateSettingsUI() {
        if (elements.briefingTimeInput) {
            elements.briefingTimeInput.value = briefingState.settings.time;
        }
        if (elements.briefingEnabledToggle) {
            elements.briefingEnabledToggle.classList.toggle('active', briefingState.settings.enabled);
        }
        if (elements.briefingPushToggle) {
            elements.briefingPushToggle.classList.toggle('active', briefingState.settings.pushNotification);
        }
        if (elements.briefingAutoToggle) {
            elements.briefingAutoToggle.classList.toggle('active', briefingState.settings.autoGenerate);
        }
    }

    // Check if briefing should be shown
    function checkForBriefing() {
        if (!briefingState.settings.enabled) {
            return;
        }

        const today = new Date().toDateString();
        if (briefingState.lastCheckDate === today) {
            return;
        }
        briefingState.lastCheckDate = today;

        // Check if current time is past briefing time
        const now = new Date();
        const [hours, minutes] = briefingState.settings.time.split(':').map(Number);
        const briefingTime = new Date(now.getFullYear(), now.getMonth(), now.getDate(), hours, minutes);

        if (now < briefingTime) {
            // Not yet time
            return;
        }

        // Fetch briefing
        fetchBriefing();
    }

    // Fetch briefing from server
    function fetchBriefing() {
        const date = new Date().toISOString().split('T')[0];

        fetch(`/api/briefing?date=${date}`)
            .then(response => {
                if (!response.ok) {
                    throw new Error('Failed to fetch briefing');
                }
                return response.json();
            })
            .then(data => {
                briefingState.currentBriefing = data;
                displayBriefing(data);
            })
            .catch(err => {
                console.warn('[Spaxel] Failed to fetch briefing:', err);

                // Try generating it
                generateBriefing();
            });
    }

    // Generate new briefing
    function generateBriefing() {
        const date = new Date().toISOString().split('T')[0];

        fetch('/api/briefing/generate', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ date: date })
        })
        .then(response => {
            if (!response.ok) {
                throw new Error('Failed to generate briefing');
            }
            return response.json();
        })
        .then(data => {
            briefingState.currentBriefing = data;
            displayBriefing(data);
        })
        .catch(err => {
            console.error('[Spaxel] Failed to generate briefing:', err);
            showError();
        });
    }

    // Display briefing card
    function displayBriefing(data) {
        if (!elements.content) return;

        // Update date
        if (elements.dateText) {
            const date = new Date(data.date);
            elements.dateText.textContent = date.toLocaleDateString('en-US', {
                weekday: 'long',
                month: 'long',
                day: 'numeric'
            });
        }

        // Parse content into sections
        let html = '';
        if (data.sections && data.sections.length > 0) {
            data.sections.forEach(section => {
                html += `<div class="briefing-section ${section.type}">${escapeHtml(section.content)}</div>`;
            });
        } else {
            html = `<div class="briefing-section">${escapeHtml(data.content)}</div>`;
        }

        elements.content.innerHTML = html;
        elements.actions.style.display = 'flex';

        // Show card
        showBriefing();
    }

    // Show briefing card
    function showBriefing() {
        if (elements.card) {
            elements.card.classList.add('visible');
            briefingState.isVisible = true;
        }
        if (elements.indicator) {
            elements.indicator.classList.remove('visible');
        }
    }

    // Hide briefing card
    function hideBriefing() {
        if (elements.card) {
            elements.card.classList.remove('visible');
            briefingState.isVisible = false;
        }
        if (elements.indicator && briefingState.currentBriefing) {
            elements.indicator.classList.add('visible');
        }
    }

    // Dismiss briefing for today
    function dismissBriefing() {
        hideBriefing();
        briefingState.isDismissed = true;

        // Save dismissal to localStorage
        const today = new Date().toDateString();
        localStorage.setItem('spaxel_briefing_dismissed', today);
    }

    // Refresh briefing
    function refreshBriefing() {
        elements.content.innerHTML = `
            <div class="briefing-loading">
                <div class="briefing-spinner"></div>
                <span>Refreshing...</span>
            </div>
        `;
        generateBriefing();
    }

    // Show error state
    function showError() {
        if (elements.content) {
            elements.content.innerHTML = `
                <div class="briefing-section">
                    Unable to load morning briefing. Please try again later.
                </div>
            `;
        }
        elements.actions.style.display = 'none';
        showBriefing();
    }

    // Hide settings panel
    function hideSettings() {
        if (elements.settingsPanel) {
            elements.settingsPanel.classList.remove('visible');
        }
    }

    // Escape HTML to prevent XSS
    function escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    // Public API
    window.SpaxelBriefing = {
        init: init,
        show: showBriefing,
        hide: hideBriefing,
        refresh: refreshBriefing,
        getSettings: () => ({ ...briefingState.settings }),
        openSettings: () => {
            if (elements.settingsPanel) {
                elements.settingsPanel.classList.add('visible');
            }
        }
    };

    // Auto-initialize when DOM is ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }
})();
