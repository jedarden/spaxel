/**
 * Spaxel Simple Mode - Mobile-first card-based UI
 *
 * Progressive disclosure interface for household members.
 * No 3D scene, just room cards, activity feed, and quick actions.
 */

(function() {
    'use strict';

    // ============================================
    // Configuration
    // ============================================
    const CONFIG = {
        WS_URL: (window.location.protocol === 'https:' ? 'wss:' : 'ws:') +
                '//' + window.location.host + '/ws/dashboard',
        POLL_INTERVAL: 30000, // 30 seconds for zone updates
        MAX_EVENTS: 50, // Maximum events to keep in feed
        STORAGE_KEY_MODE: 'spaxel_ui_mode', // localStorage key for mode preference
        STORAGE_KEY_PIN: 'spaxel_expert_pin_set' // Whether PIN has been set
    };

    // ============================================
    // Application State
    // ============================================
    const state = {
        // WebSocket connection
        ws: null,
        connected: false,
        reconnectTimer: null,
        reconnectAttempts: 0,

        // Cached data from WebSocket
        zones: [],
        blobs: [],
        nodes: [],
        events: [],
        alerts: [],
        system: {
            detection_quality: 0,
            security_mode: false,
            nodes_online: 0,
            nodes_total: 0
        },

        // UI state
        filters: {
            eventTypes: ['presence', 'zone_entry', 'zone_exit', 'alert', 'system'],
            person: '',
            zone: ''
        },

        // Sleep data
        sleepData: null,

        // Alert dismissal state
        silencedUntil: null
    };

    // ============================================
    // DOM Elements
    // ============================================
    const dom = {};

    function initDOM() {
        dom.connectionStatus = document.getElementById('connection-status');
        dom.alertBanner = document.getElementById('alert-banner');
        dom.alertTitle = document.getElementById('alert-title');
        dom.alertMessage = document.getElementById('alert-message');
        dom.alertDismiss = document.getElementById('alert-dismiss');
        dom.zonesGrid = document.getElementById('zones-grid');
        dom.activityFeed = document.getElementById('activity-feed');
        dom.modeToggle = document.getElementById('mode-toggle');
        dom.securityBtn = document.getElementById('action-security');
        dom.securityLabel = document.getElementById('security-label');
        dom.rebaselineBtn = document.getElementById('action-rebaseline');
        dom.silenceBtn = document.getElementById('action-silence');
        dom.sleepCard = document.getElementById('sleep-card');
        dom.sleepContent = document.getElementById('sleep-content');
        dom.systemStatus = document.getElementById('system-status');
    }

    // ============================================
    // WebSocket Connection
    // ============================================
    function connectWebSocket() {
        if (state.ws && state.ws.readyState === WebSocket.OPEN) {
            return;
        }

        updateConnectionStatus('connecting');

        try {
            state.ws = new WebSocket(CONFIG.WS_URL);
        } catch (e) {
            console.error('[Simple Mode] Failed to create WebSocket:', e);
            scheduleReconnect();
            return;
        }

        state.ws.onopen = function() {
            state.connected = true;
            state.reconnectAttempts = 0;
            updateConnectionStatus('connected');
            console.log('[Simple Mode] WebSocket connected');
        };

        state.ws.onclose = function(event) {
            state.connected = false;
            updateConnectionStatus('disconnected');
            console.log('[Simple Mode] WebSocket closed:', event.code, event.reason);
            scheduleReconnect();
        };

        state.ws.onerror = function(error) {
            console.error('[Simple Mode] WebSocket error:', error);
        };

        state.ws.onmessage = function(event) {
            handleMessage(event.data);
        };
    }

    function scheduleReconnect() {
        if (state.reconnectTimer) {
            clearTimeout(state.reconnectTimer);
        }

        const delay = Math.min(1000 * Math.pow(2, state.reconnectAttempts), 10000);
        state.reconnectAttempts++;

        console.log('[Simple Mode] Reconnecting in', Math.round(delay / 1000), 'seconds');

        state.reconnectTimer = setTimeout(function() {
            state.reconnectTimer = null;
            connectWebSocket();
        }, delay);
    }

    function handleMessage(data) {
        try {
            const msg = JSON.parse(data);

            // Handle snapshot (first message)
            if (msg.type === 'snapshot') {
                handleSnapshot(msg);
                return;
            }

            // Handle incremental updates
            if (msg.blobs) updateBlobs(msg.blobs);
            if (msg.zones) updateZones(msg.zones);
            if (msg.nodes) updateNodes(msg.nodes);
            if (msg.events) updateEvents(msg.events);
            if (msg.alerts) updateAlerts(msg.alerts);
            if (msg.confidence !== undefined) state.system.detection_quality = msg.confidence;
            if (msg.security_mode !== undefined) {
                state.system.security_mode = msg.security_mode;
                updateSecurityButton();
            }

            updateSystemStatus();
        } catch (e) {
            console.error('[Simple Mode] Error handling message:', e);
        }
    }

    function handleSnapshot(msg) {
        state.blobs = msg.blobs || [];
        state.zones = msg.zones || [];
        state.nodes = msg.nodes || [];
        state.events = msg.events || [];
        state.alerts = msg.alerts || [];
        state.system.detection_quality = msg.confidence || 0;
        state.system.security_mode = msg.security_mode || false;
        state.system.nodes_online = (msg.nodes || []).filter(n => n.status === 'online').length;
        state.system.nodes_total = (msg.nodes || []).length;

        renderAll();
        loadSleepSummary();
    }

    // ============================================
    // Data Updates
    // ============================================
    function updateBlobs(blobs) {
        // Replace or add blobs by ID
        const ids = {};
        for (let i = 0; i < blobs.length; i++) {
            ids[blobs[i].id] = blobs[i];
        }
        state.blobs = state.blobs.filter(function(b) { return !(b.id in ids); });
        state.blobs = state.blobs.concat(blobs);
        renderZoneCards();
    }

    function updateZones(zones) {
        const ids = {};
        for (let i = 0; i < zones.length; i++) {
            ids[zones[i].id] = zones[i];
        }
        state.zones = state.zones.filter(function(z) { return !(z.id in ids); });
        state.zones = state.zones.concat(zones);
        renderZoneCards();
    }

    function updateNodes(nodes) {
        state.nodes = nodes;
        state.system.nodes_online = nodes.filter(n => n.status === 'online').length;
        state.system.nodes_total = nodes.length;
        updateSystemStatus();
    }

    function updateEvents(events) {
        // Add new events to the beginning
        state.events = events.concat(state.events);
        // Keep only the most recent events
        if (state.events.length > CONFIG.MAX_EVENTS) {
            state.events = state.events.slice(0, CONFIG.MAX_EVENTS);
        }
        renderActivityFeed();
    }

    function updateAlerts(alerts) {
        state.alerts = alerts || [];
        renderAlertBanner();
    }

    // ============================================
    // Rendering
    // ============================================
    function renderAll() {
        renderZoneCards();
        renderActivityFeed();
        renderAlertBanner();
        updateSecurityButton();
        updateSystemStatus();
    }

    function renderZoneCards() {
        if (!dom.zonesGrid) return;

        if (state.zones.length === 0) {
            dom.zonesGrid.innerHTML = '<p class="simple-zone-card simple-zone-card--loading">No rooms configured yet.</p>';
            return;
        }

        const html = state.zones.map(function(zone) {
            const occupancy = zone.count || 0;
            const people = zone.people || [];
            const isOccupied = occupancy > 0;
            const hasAlert = zoneHasAlert(zone);

            let statusClass = 'simple-zone-card--empty';
            if (hasAlert) {
                statusClass = 'simple-zone-card--alert';
            } else if (isOccupied) {
                statusClass = 'simple-zone-card--occupied';
            }

            let statusText = isOccupied ?
                (occupancy === 1 ? '1 person' : occupancy + ' people') :
                'Empty';

            let peopleHtml = '';
            if (people.length > 0) {
                peopleHtml = '<div class="simple-zone-card__people">' +
                    people.map(function(p) {
                        return '<span class="simple-zone-card__person">' + escapeHtml(p) + '</span>';
                    }).join('') +
                    '</div>';
            }

            return '<div class="simple-zone-card ' + statusClass + '" data-zone-id="' + zone.id + '">' +
                '<div class="simple-zone-card__name">' + escapeHtml(zone.name) + '</div>' +
                '<div class="simple-zone-card__status">' + statusText + '</div>' +
                peopleHtml +
                '</div>';
        }).join('');

        dom.zonesGrid.innerHTML = html;

        // Add click handlers for zone cards
        dom.zonesGrid.querySelectorAll('.simple-zone-card').forEach(function(card) {
            card.addEventListener('click', function() {
                const zoneId = this.getAttribute('data-zone-id');
                showZoneActivity(zoneId);
            });
        });
    }

    function zoneHasAlert(zone) {
        // Check if any active alerts are for this zone
        return state.alerts.some(function(alert) {
            return alert.zone === zone.name && !alert.acknowledged;
        });
    }

    function renderActivityFeed() {
        if (!dom.activityFeed) return;

        if (state.events.length === 0) {
            dom.activityFeed.innerHTML = '<p class="simple-feed__empty">No recent activity</p>';
            return;
        }

        const html = state.events.slice(0, 20).map(function(event) {
            return renderEventItem(event);
        }).join('');

        dom.activityFeed.innerHTML = html;
    }

    function renderEventItem(event) {
        const icon = getEventIcon(event.type);
        const title = getEventTitle(event);
        const time = formatTime(event.timestamp_ms);
        const zone = event.zone ? '<span class="simple-feed-item__zone">' + escapeHtml(event.zone) + '</span>' : '';

        return '<div class="simple-feed-item" data-event-id="' + event.id + '">' +
            '<div class="simple-feed-item__icon">' + icon + '</div>' +
            '<div class="simple-feed-item__content">' +
                '<div class="simple-feed-item__title">' + escapeHtml(title) + '</div>' +
                '<div class="simple-feed-item__meta">' +
                    '<span class="simple-feed-item__time">' + time + '</span>' +
                    zone +
                '</div>' +
            '</div>' +
            '</div>';
    }

    function getEventIcon(type) {
        const icons = {
            'detection': '&#x1F464;', // person
            'zone_entry': '&#x27A1;', // arrow right
            'zone_exit': '&#x2B05;', // arrow left
            'portal_crossing': '&#x2744;', // snowflake
            'trigger_fired': '&#x26A1;', // high voltage
            'fall_alert': '&#x26A0;', // warning
            'anomaly': '&#x1F515;', // radio
            'security_alert': '&#x1F512;', // lock
            'node_online': '&#x1F4E1;', // antenna
            'node_offline': '&#x1F4E5;', // crossed antenna
            'system': '&#x2699;', // gear
            'learning': '&#x1F4D6;' // book
        };
        return icons[type] || '&#x1F4AC;'; // speech bubble
    }

    function getEventTitle(event) {
        if (event.person) {
            return event.person + ' — ' + (event.title || event.type);
        }
        return event.title || formatEventType(event.type);
    }

    function formatEventType(type) {
        return type.replace(/_/g, ' ').replace(/\b\w/g, function(l) {
            return l.toUpperCase();
        });
    }

    function formatTime(timestampMs) {
        const diff = Date.now() - timestampMs;
        if (diff < 60000) return 'just now';
        if (diff < 3600000) return Math.floor(diff / 60000) + 'm ago';
        if (diff < 86400000) return Math.floor(diff / 3600000) + 'h ago';
        return new Date(timestampMs).toLocaleDateString();
    }

    function renderAlertBanner() {
        if (!dom.alertBanner) return;

        // Check if alerts are silenced
        if (state.silencedUntil && Date.now() < state.silencedUntil) {
            dom.alertBanner.hidden = true;
            return;
        }

        // Find the highest priority unacknowledged alert
        const alert = state.alerts.find(function(a) { return !a.acknowledged; });

        if (!alert) {
            dom.alertBanner.hidden = true;
            return;
        }

        dom.alertTitle.textContent = alert.title || 'Alert';
        dom.alertMessage.textContent = alert.message || '';
        dom.alertBanner.hidden = false;
        dom.alertBanner.className = 'simple-alerts simple-alerts--' + (alert.severity || 'warning');

        // Show silence button if there are multiple alerts
        dom.silenceBtn.hidden = state.alerts.filter(a => !a.acknowledged).length <= 1;
    }

    function updateSecurityButton() {
        if (!dom.securityBtn || !dom.securityLabel) return;

        if (state.system.security_mode) {
            dom.securityLabel.textContent = 'Disarm Security';
            dom.securityBtn.classList.add('simple-action-btn--active');
        } else {
            dom.securityLabel.textContent = 'Arm Security';
            dom.securityBtn.classList.remove('simple-action-btn--active');
        }
    }

    function updateSystemStatus() {
        if (!dom.systemStatus) return;

        const quality = state.system.detection_quality;
        const nodes = state.system.nodes_online + '/' + state.system.nodes_total;

        let status = 'System: ' + nodes + ' nodes online';
        if (quality > 0) {
            status += ' • ' + quality + '% quality';
        }

        dom.systemStatus.textContent = status;
    }

    function updateConnectionStatus(status) {
        if (!dom.connectionStatus) return;

        const dot = dom.connectionStatus.querySelector('.simple-status__dot');
        const text = dom.connectionStatus.querySelector('.simple-status__text');

        if (status === 'connected') {
            dot.className = 'simple-status__dot simple-status__dot--connected';
            text.textContent = 'Connected';
        } else if (status === 'connecting') {
            dot.className = 'simple-status__dot simple-status__dot--connecting';
            text.textContent = 'Connecting...';
        } else {
            dot.className = 'simple-status__dot simple-status__dot--disconnected';
            text.textContent = 'Disconnected — Reconnecting...';
        }
    }

    // ============================================
    // Actions
    // ============================================
    function handleSecurityToggle() {
        const newState = !state.system.security_mode;

        fetch('/api/security/' + (newState ? 'arm' : 'disarm'), {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            }
        }).then(function(response) {
            if (!response.ok) throw new Error('Failed to toggle security mode');
            return response.json();
        }).then(function(data) {
            state.system.security_mode = data.security_mode;
            updateSecurityButton();
            showToast(newState ? 'Security mode armed' : 'Security mode disarmed', 'success');
        }).catch(function(error) {
            console.error('[Simple Mode] Error toggling security:', error);
            showToast('Failed to toggle security mode', 'error');
        });
    }

    function handleRebaseline() {
        fetch('/api/nodes/rebaseline-all', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            }
        }).then(function(response) {
            if (!response.ok) throw new Error('Failed to trigger re-baseline');
            return response.json();
        }).then(function(data) {
            showToast('Re-baseline started — this takes about 60 seconds', 'info');
        }).catch(function(error) {
            console.error('[Simple Mode] Error triggering re-baseline:', error);
            showToast('Failed to trigger re-baseline', 'error');
        });
    }

    function handleSilenceAlerts() {
        // Silence for 1 hour
        state.silencedUntil = Date.now() + 3600000;
        renderAlertBanner();
        showToast('Alerts silenced for 1 hour', 'info');
    }

    function handleModeToggle() {
        // Save preference to localStorage
        localStorage.setItem(CONFIG.STORAGE_KEY_MODE, 'expert');
        // Navigate to expert mode
        window.location.href = '/live';
    }

    function showZoneActivity(zoneId) {
        // Filter activity feed to show only events for this zone
        const zone = state.zones.find(function(z) { return z.id == zoneId; });
        if (!zone) return;

        state.filters.zone = zone.name;

        const filteredEvents = state.events.filter(function(e) {
            return e.zone === zone.name;
        });

        if (filteredEvents.length === 0) {
            dom.activityFeed.innerHTML = '<p class="simple-feed__empty">No recent activity in ' + escapeHtml(zone.name) + '</p>';
            return;
        }

        const html = filteredEvents.slice(0, 20).map(function(event) {
            return renderEventItem(event);
        }).join('');

        dom.activityFeed.innerHTML = html;
    }

    // ============================================
    // Morning Briefing (includes sleep summary)
    // ============================================
    function loadSleepSummary() {
        // First try to load the full morning briefing
        const today = new Date().toISOString().split('T')[0];

        fetch('/api/briefing/today')
            .then(function(response) {
                if (!response.ok) {
                    // Fall back to sleep summary if briefing not available
                    return fetch('/api/sleep/summary').then(r => r.json()).then(data => ({ type: 'sleep', data: data }));
                }
                return response.json().then(briefing => ({ type: 'briefing', data: briefing }));
            })
            .then(function(result) {
                if (!result || !result.data) {
                    if (dom.sleepCard) dom.sleepCard.hidden = true;
                    return;
                }

                if (result.type === 'briefing') {
                    renderMorningBriefing(result.data);
                } else {
                    state.sleepData = result.data;
                    renderSleepSummary();
                }
            })
            .catch(function(error) {
                console.error('[Simple Mode] Error loading morning briefing:', error);
                if (dom.sleepCard) dom.sleepCard.hidden = true;
            });
    }

    function renderMorningBriefing(briefing) {
        if (!dom.sleepCard || !dom.sleepContent) return;

        // Update date header
        const dateEl = document.getElementById('sleep-date');
        if (dateEl) {
            const date = new Date(briefing.date || Date.now());
            dateEl.textContent = date.toLocaleDateString('en-US', {
                weekday: 'long',
                month: 'long',
                day: 'numeric'
            });
        }

        // Build HTML from briefing sections or content
        let html = '';
        if (briefing.sections && briefing.sections.length > 0) {
            briefing.sections.forEach(function(section) {
                html += '<div class="simple-briefing-section simple-briefing-section--' + section.type + '">' +
                    escapeHtml(section.content) +
                    '</div>';
            });
        } else if (briefing.content) {
            // Parse content into paragraphs
            const paragraphs = briefing.content.split('\n\n').filter(p => p.trim());
            paragraphs.forEach(function(p) {
                html += '<div class="simple-briefing-section">' + escapeHtml(p) + '</div>';
            });
        } else {
            html = '<p>No briefing data available.</p>';
        }

        dom.sleepContent.innerHTML = html;
        dom.sleepCard.hidden = false;
        dom.sleepCard.classList.add('morning-briefing-card');
    }

    function renderSleepSummary() {
        if (!dom.sleepCard || !dom.sleepContent || !state.sleepData) return;

        const data = state.sleepData;
        const html =
            '<p><strong>' + (data.duration_min ? Math.floor(data.duration_min / 60) + 'h ' + (data.duration_min % 60) + 'm' : 'N/A') +
            ' in bed</strong></p>' +
            '<p>Restlessness: ' + getRestlessnessLabel(data.restlessness) + '</p>' +
            '<p>Breathing: ' + getBreathingRegularityLabel(data.breathing_regularity) + '</p>';

        dom.sleepContent.innerHTML = html;
        dom.sleepCard.hidden = false;
    }

    function getRestlessnessLabel(value) {
        if (!value) return 'N/A';
        if (value < 1) return '<span class="simple-sleep__good">Low</span>';
        if (value < 3) return '<span class="simple-sleep__ok">Moderate</span>';
        return '<span class="simple-sleep__poor">High</span>';
    }

    function getBreathingRegularityLabel(value) {
        if (!value) return 'N/A';
        if (value < 0.15) return '<span class="simple-sleep__good">Regular</span>';
        if (value < 0.25) return '<span class="simple-sleep__ok">Fair</span>';
        return '<span class="simple-sleep__poor">Irregular</span>';
    }

    // ============================================
    // Toast Notifications
    // ============================================
    function showToast(message, type) {
        const container = document.querySelector('.toast-container');
        if (!container) {
            // Create container if it doesn't exist
            const tc = document.createElement('div');
            tc.className = 'toast-container';
            document.body.appendChild(tc);
        }

        const toast = document.createElement('div');
        toast.className = 'toast toast--' + (type || 'info');
        toast.textContent = message;

        document.querySelector('.toast-container').appendChild(toast);

        // Remove after 3 seconds
        setTimeout(function() {
            toast.style.animation = 'toast-out 0.3s ease-out forwards';
            setTimeout(function() {
                if (toast.parentNode) {
                    toast.parentNode.removeChild(toast);
                }
            }, 300);
        }, 3000);
    }

    // ============================================
    // Utility Functions
    // ============================================
    function escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    // ============================================
    // Event Bindings
    // ============================================
    function bindEvents() {
        if (dom.modeToggle) {
            dom.modeToggle.addEventListener('click', handleModeToggle);
        }

        if (dom.securityBtn) {
            dom.securityBtn.addEventListener('click', handleSecurityToggle);
        }

        if (dom.rebaselineBtn) {
            dom.rebaselineBtn.addEventListener('click', handleRebaseline);
        }

        if (dom.silenceBtn) {
            dom.silenceBtn.addEventListener('click', handleSilenceAlerts);
        }

        if (dom.alertDismiss) {
            dom.alertDismiss.addEventListener('click', function() {
                // Acknowledge all current alerts
                state.alerts.forEach(function(alert) {
                    alert.acknowledged = true;
                });
                renderAlertBanner();
            });
        }
    }

    // ============================================
    // Initialization
    // ============================================
    function init() {
        initDOM();
        bindEvents();
        connectWebSocket();

        // Set up polling for zone updates
        setInterval(loadSleepSummary, 60000); // Check for sleep summary every minute
    }

    // Auto-initialize when DOM is ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }

})();
