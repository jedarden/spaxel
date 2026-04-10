/**
 * Spaxel Dashboard - Simple Mode
 *
 * Card-based mobile-first UI for non-technical users.
 * Progressive disclosure from simple to expert mode.
 */

(function() {
    'use strict';

    // ============================================
    // Configuration
    // ============================================
    const STORAGE_KEY_MODE = 'spaxel_ui_mode';
    const STORAGE_KEY_DISMISSED = 'spaxel_briefing_dismissed';
    const UPDATE_INTERVAL = 10000; // 10 seconds

    // ============================================
    // State
    // ============================================
    let isSimpleMode = false;
    let updateTimer = null;
    let currentState = {
        zones: [],
        events: [],
        alerts: [],
        securityMode: false,
        sleepSummary: null,
        briefing: null,
        blobs: [],
        nodes: [],
        triggers: [],
        lastUpdate: null
    };

    // ============================================
    // Initialization
    // ============================================

    /**
     * Initialize simple mode
     */
    function init() {
        console.log('[Simple Mode] Initializing...');

        // Check if simple mode is active
        const savedMode = localStorage.getItem(STORAGE_KEY_MODE);
        isSimpleMode = savedMode !== 'expert';

        // Set up mode toggle if not exists
        setupModeToggle();

        // Register WebSocket message handler for real-time updates
        registerWebSocketHandler();

        // Start periodic updates (for data that doesn't come via WebSocket)
        startUpdates();

        // Initial data fetch
        fetchAllData();

        console.log('[Simple Mode] Initialized');
    }

    /**
     * Register WebSocket message handler for real-time updates
     */
    function registerWebSocketHandler() {
        // Register with SpaxelApp to receive WebSocket messages
        if (window.SpaxelApp && window.SpaxelApp.registerMessageHandler) {
            window.SpaxelApp.registerMessageHandler(handleWebSocketMessage);
        }
    }

    /**
     * Handle WebSocket message from mothership
     */
    function handleWebSocketMessage(msg) {
        if (!isSimpleMode) return;

        // Handle different message types
        switch (msg.type) {
            case 'loc_update':
            case 'incremental':
                // Blob/localization updates
                if (msg.blobs) {
                    currentState.blobs = msg.blobs;
                    updateRoomCardsFromBlobs(msg.blobs);
                }
                if (msg.zones) {
                    currentState.zones = msg.zones;
                    updateRoomCards();
                }
                break;

            case 'event':
                // Real-time events
                if (msg.event) {
                    addEventToFeed(msg.event);
                }
                break;

            case 'alert':
                // Real-time alerts
                if (msg.alert) {
                    handleAlert(msg.alert);
                }
                break;

            case 'trigger_state':
                // Trigger state changes
                if (msg.trigger) {
                    updateTriggerState(msg.trigger);
                }
                break;

            case 'system_health':
                // System health updates
                if (msg.health) {
                    currentState.detectionQuality = msg.health.detection_quality || 0;
                }
                break;

            default:
                // Log unknown message types
                console.log('[Simple Mode] Unknown message type:', msg.type);
        }
    }

    /**
     * Set up the mode toggle between simple and expert
     */
    function setupModeToggle() {
        // Create header if not exists
        if (!document.getElementById('simple-mode-header')) {
            createHeader();
        }

        // Create content container
        if (!document.getElementById('simple-mode-content')) {
            createContentContainer();
        }

        // Create quick actions bar
        if (!document.getElementById('simple-quick-actions')) {
            createQuickActions();
        }

        // Apply initial mode
        if (isSimpleMode) {
            enableSimpleMode();
        }
    }

    /**
     * Create the simple mode header
     */
    function createHeader() {
        const header = document.createElement('div');
        header.id = 'simple-mode-header';
        header.className = 'simple-mode-header';
        header.innerHTML = `
            <h1>&#x1F3E0; Spaxel</h1>
            <div class="mode-toggle">
                <button class="mode-toggle-btn ${isSimpleMode ? 'active' : ''}" data-mode="simple">
                    Simple
                </button>
                <button class="mode-toggle-btn ${!isSimpleMode ? 'active' : ''}" data-mode="expert">
                    Expert
                </button>
            </div>
        `;

        // Insert at the beginning of body
        document.body.insertBefore(header, document.body.firstChild);

        // Add event listeners
        header.querySelectorAll('.mode-toggle-btn').forEach(btn => {
            btn.addEventListener('click', onModeToggle);
        });
    }

    /**
     * Create the content container
     */
    function createContentContainer() {
        const content = document.createElement('div');
        content.id = 'simple-mode-content';
        content.className = 'simple-mode-content';
        document.body.appendChild(content);
    }

    /**
     * Create the quick actions bar
     */
    function createQuickActions() {
        const actions = document.createElement('div');
        actions.id = 'simple-quick-actions';
        actions.className = 'simple-quick-actions';
        actions.innerHTML = `
            <div class="actions-container">
                <button class="quick-action-btn" data-action="home">
                    <span class="action-icon">&#x1F3E0;</span>
                    <span class="action-label">Home</span>
                </button>
                <button class="quick-action-btn" data-action="timeline">
                    <span class="action-icon">&#x23F0;</span>
                    <span class="action-label">Timeline</span>
                </button>
                <button class="quick-action-btn" data-action="security">
                    <span class="action-icon">&#x1F512;</span>
                    <span class="action-label">Security</span>
                </button>
                <button class="quick-action-btn" data-action="settings">
                    <span class="action-icon">&#x2699;</span>
                    <span class="action-label">Settings</span>
                </button>
            </div>
        `;

        document.body.appendChild(actions);

        // Add event listeners
        actions.querySelectorAll('.quick-action-btn').forEach(btn => {
            btn.addEventListener('click', onQuickAction);
        });
    }

    // ============================================
    // Mode Management
    // ============================================

    /**
     * Handle mode toggle
     */
    function onModeToggle(e) {
        const newMode = e.currentTarget.dataset.mode;
        const isSimple = newMode === 'simple';

        // Save preference
        localStorage.setItem(STORAGE_KEY_MODE, newMode);

        if (isSimple) {
            enableSimpleMode();
        } else {
            disableSimpleMode();
        }
    }

    /**
     * Enable simple mode
     */
    function enableSimpleMode() {
        isSimpleMode = true;
        document.body.classList.add('simple-mode');

        // Show simple mode UI
        const header = document.getElementById('simple-mode-header');
        const content = document.getElementById('simple-mode-content');
        const quickActions = document.getElementById('simple-quick-actions');

        if (header) header.style.display = 'flex';
        if (content) content.style.display = 'block';
        if (quickActions) quickActions.style.display = 'block';

        // Update toggle buttons in simple mode header
        document.querySelectorAll('.mode-toggle-btn').forEach(btn => {
            btn.classList.toggle('active', btn.dataset.mode === 'simple');
        });

        // Render content
        renderContent();

        console.log('[Simple Mode] Enabled');
    }

    /**
     * Disable simple mode (switch to expert)
     */
    function disableSimpleMode() {
        isSimpleMode = false;
        document.body.classList.remove('simple-mode');

        // Hide simple mode UI
        const header = document.getElementById('simple-mode-header');
        const content = document.getElementById('simple-mode-content');
        const quickActions = document.getElementById('simple-quick-actions');

        if (header) header.style.display = 'none';
        if (content) content.style.display = 'none';
        if (quickActions) quickActions.style.display = 'none';

        // Note: expert mode visibility is handled by router

        console.log('[Simple Mode] Disabled (expert mode active)');
    }

    // ============================================
    // Data Fetching
    // ============================================

    /**
     * Fetch all data needed for simple mode
     */
    async function fetchAllData() {
        if (!isSimpleMode) return;

        try {
            // Fetch zones with occupancy
            const zonesResponse = await fetch('/api/zones');
            if (zonesResponse.ok) {
                currentState.zones = await zonesResponse.json();
            }

            // Fetch recent events
            const eventsResponse = await fetch('/api/events?limit=20');
            if (eventsResponse.ok) {
                const eventsData = await eventsResponse.json();
                currentState.events = eventsData.events || [];
            }

            // Fetch system status
            const statusResponse = await fetch('/api/status');
            if (statusResponse.ok) {
                const statusData = await statusResponse.json();
                currentState.securityMode = statusData.security_mode || false;
                currentState.detectionQuality = statusData.detection_quality || 0;
            }

            // Fetch sleep summary (if available)
            const sleepResponse = await fetch('/api/sleep/summary?limit=1');
            if (sleepResponse.ok) {
                const sleepData = await sleepResponse.json();
                currentState.sleepSummary = sleepData[0] || null;
            }

            // Fetch morning briefing
            const today = new Date().toISOString().split('T')[0];
            const briefingResponse = await fetch(`/api/briefings/${today}`);
            if (briefingResponse.ok) {
                currentState.briefing = await briefingResponse.json();
            }

            currentState.lastUpdate = new Date();

            // Render the updated content
            renderContent();

        } catch (error) {
            console.error('[Simple Mode] Error fetching data:', error);
            showError('Unable to load data. Please check your connection.');
        }
    }

    /**
     * Start periodic updates
     */
    function startUpdates() {
        if (updateTimer) {
            clearInterval(updateTimer);
        }

        updateTimer = setInterval(fetchAllData, UPDATE_INTERVAL);
    }

    // ============================================
    // Rendering
    // ============================================

    /**
     * Render all simple mode content
     */
    function renderContent() {
        if (!isSimpleMode) return;

        const container = document.getElementById('simple-mode-content');
        if (!container) return;

        let html = '';

        // Alert banner (if any active alerts)
        if (currentState.alerts.length > 0) {
            html += renderAlertBanner(currentState.alerts[0]);
        }

        // Morning briefing (if not dismissed and available)
        const wasDismissed = localStorage.getItem(STORAGE_KEY_DISMISSED) === new Date().toISOString().split('T')[0];
        if (currentState.briefing && !wasDismissed) {
            html += renderMorningBriefing(currentState.briefing);
        }

        // Sleep summary (if available)
        if (currentState.sleepSummary) {
            html += renderSleepSummary(currentState.sleepSummary);
        }

        // Security toggle
        html += renderSecurityToggle();

        // Room cards
        html += renderRoomCards(currentState.zones);

        // Activity feed
        html += renderActivityFeed(currentState.events);

        // Loading state if no data
        if (!currentState.lastUpdate) {
            html = renderLoadingState();
        }

        container.innerHTML = html;

        // Attach event listeners
        attachEventListeners();
    }

    /**
     * Render alert banner
     */
    function renderAlertBanner(alert) {
        return `
            <div class="simple-alert-banner visible" data-alert-id="${alert.id}">
                <span class="alert-icon">&#x26A0;</span>
                <div class="alert-content">
                    <div class="alert-title">${alert.title || 'Alert'}</div>
                    <div class="alert-message">${alert.message}</div>
                </div>
                <button class="alert-dismiss" aria-label="Dismiss alert">&times;</button>
            </div>
        `;
    }

    /**
     * Render morning briefing
     */
    function renderMorningBriefing(briefing) {
        return `
            <div class="simple-morning-briefing" data-briefing-date="${briefing.date}">
                <div class="briefing-header">
                    <span class="briefing-greeting">${getGreeting()}</span>
                    <span class="briefing-date">${formatDate(briefing.generated_at)}</span>
                </div>
                <div class="briefing-content">
                    ${parseBriefingContent(briefing.content)}
                </div>
                <button class="briefing-dismiss" data-action="dismiss-briefing">Got it</button>
            </div>
        `;
    }

    /**
     * Render sleep summary card
     */
    function renderSleepSummary(sleep) {
        const quality = getSleepQualityLabel(sleep);
        const restlessness = getRestlessnessLabel(sleep.restlessness);

        return `
            <div class="simple-sleep-summary">
                <div class="sleep-header">
                    <span class="sleep-title">&#x1F634; Sleep Summary</span>
                    <span class="sleep-date">${formatDate(sleep.date)}</span>
                </div>
                <div class="sleep-metrics">
                    <div class="sleep-metric">
                        <div class="metric-label">Duration</div>
                        <div class="metric-value">${formatDuration(sleep.duration_min)}</div>
                    </div>
                    <div class="sleep-metric">
                        <div class="metric-label">Onset</div>
                        <div class="metric-value">${sleep.onset_latency_min || '--'}<span class="metric-unit">min</span></div>
                    </div>
                    <div class="sleep-quality">
                        <span class="quality-label">${restlessness}</span>
                        <span class="quality-value">${quality}</span>
                    </div>
                    ${sleep.breathing_rate_avg ? `
                        <div class="sleep-metric">
                            <div class="metric-label">Breathing</div>
                            <div class="metric-value">${sleep.breathing_rate_avg.toFixed(1)}<span class="metric-unit">bpm</span></div>
                        </div>
                    ` : ''}
                </div>
                <button class="sleep-details-btn" data-action="view-sleep-details">View Details</button>
            </div>
        `;
    }

    /**
     * Render security toggle
     */
    function renderSecurityToggle() {
        const isArmed = currentState.securityMode;

        return `
            <div class="simple-security-toggle">
                <div class="security-header">
                    <span class="security-title">&#x1F512; Security Mode</span>
                </div>
                <div class="security-description">
                    ${isArmed
                        ? 'Security mode is active. Any detected motion will trigger alerts.'
                        : 'Arm security mode to receive alerts when motion is detected.'}
                </div>
                <button class="security-toggle-btn ${isArmed ? 'armed' : 'disarmed'}"
                        data-action="${isArmed ? 'disarm' : 'arm'}-security">
                    ${isArmed ? 'Disarm' : 'Arm'} Security
                </button>
                <div class="security-status">
                    ${isArmed ? 'Armed and monitoring' : 'Disarmed - no alerts will be sent'}
                </div>
            </div>
        `;
    }

    /**
     * Render room cards
     */
    function renderRoomCards(zones) {
        if (!zones || zones.length === 0) {
            return `
                <div class="simple-room-cards">
                    <div class="simple-room-card empty">
                        <div class="room-header">
                            <span class="room-name">No Zones Defined</span>
                            <span class="room-status empty">Empty</span>
                        </div>
                        <div class="room-activity">
                            Set up zones in the expert 3D view to see room cards here.
                        </div>
                    </div>
                </div>
            `;
        }

        const cards = zones.map(zone => {
            const status = getZoneStatus(zone);
            const occupants = zone.people || [];
            const lastActivity = getLastActivityForZone(zone.name);

            return `
                <div class="simple-room-card ${status.class}" data-zone-id="${zone.id}">
                    <div class="room-header">
                        <span class="room-name">${zone.name}</span>
                        <span class="room-status ${status.class}">${status.label}</span>
                    </div>
                    ${occupants.length > 0 ? `
                        <div class="room-occupants">
                            ${occupants.map(person => `
                                <div class="occupant-avatar" style="background: ${getPersonColor(person)}">
                                    ${getPersonInitials(person)}
                                </div>
                            `).join('')}
                        </div>
                    ` : ''}
                    <div class="room-activity">
                        ${lastActivity ? lastActivity : 'No recent activity'}
                    </div>
                    <div class="room-timestamp">
                        ${zone.occupancy_updated_at ? formatTimestamp(zone.occupancy_updated_at) : ''}
                    </div>
                    <div class="room-expand-hint">
                        Tap for details &#x25BC;
                    </div>
                </div>
            `;
        }).join('');

        return `<div class="simple-room-cards">${cards}</div>`;
    }

    /**
     * Render activity feed
     */
    function renderActivityFeed(events) {
        if (!events || events.length === 0) {
            return `
                <div class="simple-activity-feed">
                    <div class="feed-header">
                        <span class="feed-title">Activity</span>
                        <div class="feed-filter">
                            <button class="filter-btn active" data-filter="all">All</button>
                            <button class="filter-btn" data-filter="recent">Recent</button>
                        </div>
                    </div>
                    <div class="feed-empty">
                        <div class="feed-empty-icon">&#x1F4C5;</div>
                        <div class="feed-empty-text">No activity yet</div>
                        <div class="feed-empty-subtext">Events will appear here as Spaxel detects activity</div>
                    </div>
                </div>
            `;
        }

        const activityItems = events.slice(0, 10).map(event => {
            const icon = getActivityIcon(event.type);
            const description = formatEventDescription(event);

            return `
                <div class="activity-item" data-event-id="${event.id}">
                    <div class="activity-icon ${icon.class}">${icon.icon}</div>
                    <div class="activity-content">
                        <div class="activity-title">${event.title || formatEventTitle(event)}</div>
                        <div class="activity-description">${description}</div>
                        <div class="activity-time">${formatTimestamp(event.timestamp_ms)}</div>
                    </div>
                </div>
            `;
        }).join('');

        return `
            <div class="simple-activity-feed">
                <div class="feed-header">
                    <span class="feed-title">Activity</span>
                    <div class="feed-filter">
                        <button class="filter-btn active" data-filter="all">All</button>
                        <button class="filter-btn" data-filter="recent">Recent</button>
                    </div>
                </div>
                <div class="activity-list">
                    ${activityItems}
                </div>
            </div>
        `;
    }

    /**
     * Render loading state
     */
    function renderLoadingState() {
        return `
            <div class="simple-loading">
                <div class="simple-loading-spinner"></div>
                <div class="simple-loading-text">Loading your home...</div>
            </div>
        `;
    }

    // ============================================
    // Event Handlers
    // ============================================

    /**
     * Attach event listeners to rendered elements
     */
    function attachEventListeners() {
        // Alert dismiss buttons
        document.querySelectorAll('.alert-dismiss').forEach(btn => {
            btn.addEventListener('click', dismissAlert);
        });

        // Briefing dismiss button
        document.querySelector('.briefing-dismiss')?.addEventListener('click', dismissBriefing);

        // Security toggle buttons
        document.querySelectorAll('[data-action="arm-security"], [data-action="disarm-security"]')
            .forEach(btn => btn.addEventListener('click', toggleSecurityMode));

        // Room card clicks
        document.querySelectorAll('.simple-room-card').forEach(card => {
            card.addEventListener('click', () => showRoomDetails(card.dataset.zoneId));
        });

        // Activity filter buttons
        document.querySelectorAll('.feed-filter-btn').forEach(btn => {
            btn.addEventListener('click', filterActivityFeed);
        });
    }

    /**
     * Handle quick action button clicks
     */
    function onQuickAction(e) {
        const action = e.currentTarget.dataset.action;

        switch (action) {
            case 'home':
                // Scroll to top
                window.scrollTo({ top: 0, behavior: 'smooth' });
                break;
            case 'timeline':
                // Switch to timeline view
                disableSimpleMode();
                if (window.SpaxelRouter) {
                    SpaxelRouter.navigate('timeline');
                }
                break;
            case 'security':
                // Scroll to security toggle or toggle it
                const securityToggle = document.querySelector('.simple-security-toggle');
                if (securityToggle) {
                    securityToggle.scrollIntoView({ behavior: 'smooth', block: 'center' });
                }
                break;
            case 'settings':
                // Switch to expert mode and open settings
                disableSimpleMode();
                if (window.openSettingsPanel) {
                    openSettingsPanel();
                }
                break;
        }

        // Update active state
        document.querySelectorAll('.quick-action-btn').forEach(btn => {
            btn.classList.toggle('active', btn.dataset.action === action);
        });
    }

    /**
     * Dismiss an alert
     */
    function dismissAlert(e) {
        const banner = e.target.closest('.simple-alert-banner');
        if (banner) {
            banner.style.animation = 'slideDown 0.3s ease-out reverse';
            setTimeout(() => banner.remove(), 300);
        }
    }

    /**
     * Dismiss the morning briefing
     */
    function dismissBriefing() {
        const today = new Date().toISOString().split('T')[0];
        localStorage.setItem(STORAGE_KEY_DISMISSED, today);

        const briefing = document.querySelector('.simple-morning-briefing');
        if (briefing) {
            briefing.style.animation = 'fadeIn 0.3s ease-out reverse';
            setTimeout(() => briefing.remove(), 300);
        }
    }

    /**
     * Toggle security mode
     */
    async function toggleSecurityMode(e) {
        const isArming = e.target.dataset.action === 'arm-security';
        const endpoint = isArming ? '/api/security/arm' : '/api/security/disarm';

        try {
            const response = await fetch(endpoint, { method: 'POST' });
            if (response.ok) {
                // Update state and re-render
                currentState.securityMode = isArming;
                renderContent();

                // Show toast confirmation
                showToast(isArming ? 'Security mode armed' : 'Security mode disarmed');
            } else {
                showError('Failed to toggle security mode');
            }
        } catch (error) {
            console.error('[Simple Mode] Error toggling security:', error);
            showError('Unable to toggle security mode');
        }
    }

    /**
     * Show room details modal
     */
    function showRoomDetails(zoneId) {
        const zone = currentState.zones.find(z => z.id == zoneId);
        if (!zone) return;

        // Create modal
        const modal = document.createElement('div');
        modal.className = 'simple-room-modal visible';
        modal.innerHTML = `
            <div class="modal-content">
                <div class="modal-header">
                    <span class="modal-title">${zone.name}</span>
                    <button class="modal-close">&times;</button>
                </div>
                <div class="modal-body">
                    <div class="room-stats">
                        <div class="room-stat">
                            <div class="stat-label">Occupancy</div>
                            <div class="stat-value">${zone.occupancy || 0}</div>
                        </div>
                        <div class="room-stat">
                            <div class="stat-label">People</div>
                            <div class="stat-value">${(zone.people || []).length}</div>
                        </div>
                    </div>
                    <div class="room-history">
                        <div class="history-title">Recent Activity</div>
                        <div class="history-list">
                            ${getZoneHistory(zone.name)}
                        </div>
                    </div>
                </div>
            </div>
        `;

        document.body.appendChild(modal);

        // Close on backdrop click or close button
        modal.addEventListener('click', (e) => {
            if (e.target === modal || e.target.classList.contains('modal-close')) {
                modal.remove();
            }
        });
    }

    /**
     * Filter activity feed
     */
    function filterActivityFeed(e) {
        const filter = e.target.dataset.filter;

        // Update active state
        document.querySelectorAll('.feed-filter-btn').forEach(btn => {
            btn.classList.toggle('active', btn.dataset.filter === filter);
        });

        // Re-render with filter applied
        // (In a full implementation, this would filter the events array)
        console.log('[Simple Mode] Filter activity feed:', filter);
    }

    // ============================================
    // Helper Functions
    // ============================================

    /**
     * Get greeting based on time of day
     */
    function getGreeting() {
        const hour = new Date().getHours();
        if (hour < 12) return 'Good morning';
        if (hour < 17) return 'Good afternoon';
        return 'Good evening';
    }

    /**
     * Format date for display
     */
    function formatDate(timestamp) {
        const date = new Date(timestamp);
        const today = new Date();
        const yesterday = new Date(today);
        yesterday.setDate(yesterday.getDate() - 1);

        if (date.toDateString() === today.toDateString()) {
            return 'Today';
        } else if (date.toDateString() === yesterday.toDateString()) {
            return 'Yesterday';
        } else {
            return date.toLocaleDateString('en-US', { weekday: 'long', month: 'short', day: 'numeric' });
        }
    }

    /**
     * Format timestamp for display
     */
    function formatTimestamp(ms) {
        const date = new Date(ms);
        const now = new Date();
        const diff = now - date;

        // Less than 1 minute
        if (diff < 60000) {
            return 'Just now';
        }

        // Less than 1 hour
        if (diff < 3600000) {
            const mins = Math.floor(diff / 60000);
            return `${mins}m ago`;
        }

        // Less than 1 day
        if (diff < 86400000) {
            const hours = Math.floor(diff / 3600000);
            return `${hours}h ago`;
        }

        // Otherwise show date
        return date.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
    }

    /**
     * Format duration in minutes to hours and minutes
     */
    function formatDuration(minutes) {
        if (!minutes) return '--';
        const hours = Math.floor(minutes / 60);
        const mins = minutes % 60;
        if (hours > 0) {
            return `${hours}h ${mins}m`;
        }
        return `${mins}m`;
    }

    /**
     * Parse briefing content into sections
     */
    function parseBriefingContent(content) {
        // Simple parsing - in production, this would be more sophisticated
        const lines = content.split('\n').filter(line => line.trim());
        return lines.map(line => `<div class="briefing-section">${line}</div>`).join('');
    }

    /**
     * Get zone status
     */
    function getZoneStatus(zone) {
        if (zone.occupancy > 0) {
            return { class: 'occupied', label: `Occupied (${zone.occupancy})` };
        }
        return { class: 'empty', label: 'Empty' };
    }

    /**
     * Get person color
     */
    function getPersonColor(person) {
        // Generate consistent color from name
        let hash = 0;
        for (let i = 0; i < person.length; i++) {
            hash = person.charCodeAt(i) + ((hash << 5) - hash);
        }
        const hue = Math.abs(hash) % 360;
        return `hsl(${hue}, 70%, 50%)`;
    }

    /**
     * Get person initials
     */
    function getPersonInitials(person) {
        const parts = person.trim().split(/\s+/);
        if (parts.length >= 2) {
            return (parts[0][0] + parts[1][0]).toUpperCase();
        }
        return person.substring(0, 2).toUpperCase();
    }

    /**
     * Get last activity for a zone
     */
    function getLastActivityForZone(zoneName) {
        const zoneEvents = currentState.events.filter(e => e.zone === zoneName);
        if (zoneEvents.length > 0) {
            const latest = zoneEvents[0];
            return formatEventDescription(latest);
        }
        return 'No recent activity';
    }

    /**
     * Get activity icon for event type
     */
    function getActivityIcon(type) {
        const icons = {
            'detection': { icon: '&#x1F464;', class: 'presence' },
            'zone_entry': { icon: '&#x1F6AA;', class: 'presence' },
            'zone_exit': { icon: '&#x1F6AA;', class: 'presence' },
            'portal_crossing': { icon: '&#x1F6AA;', class: 'presence' },
            'fall_alert': { icon: '&#x1FA78;', class: 'alert' },
            'anomaly': { icon: '&#x26A0;', class: 'alert' },
            'security_alert': { icon: '&#x1F512;', class: 'alert' },
            'node_online': { icon: '&#x1F4F1;', class: 'system' },
            'node_offline': { icon: '&#x1F4F1;', class: 'system' },
            'system': { icon: '&#x2699;', class: 'system' },
            'learning_milestone': { icon: '&#x1F4C5;', class: 'system' }
        };

        return icons[type] || { icon: '&#x2022;', class: 'presence' };
    }

    /**
     * Format event title
     */
    function formatEventTitle(event) {
        if (event.title) return event.title;

        const titles = {
            'detection': 'Motion detected',
            'zone_entry': `Entered ${event.zone}`,
            'zone_exit': `Left ${event.zone}`,
            'portal_crossing': 'Room transition',
            'fall_alert': 'Fall detected',
            'anomaly': 'Unusual activity',
            'security_alert': 'Security alert',
            'node_online': 'Node connected',
            'node_offline': 'Node disconnected',
            'system': 'System event',
            'learning_milestone': 'Learning progress'
        };

        return titles[event.type] || 'Event';
    }

    /**
     * Format event description
     */
    function formatEventDescription(event) {
        if (event.detail_json) {
            try {
                const detail = typeof event.detail_json === 'string'
                    ? JSON.parse(event.detail_json)
                    : event.detail_json;
                return detail.description || detail.message || '';
            } catch (e) {
                // Ignore parse errors
            }
        }

        // Default descriptions
        const descriptions = {
            'detection': 'Motion was detected in this area',
            'zone_entry': `Someone entered ${event.zone}`,
            'zone_exit': `Someone left ${event.zone}`,
            'portal_crossing': 'Movement between rooms detected',
            'fall_alert': 'A possible fall was detected',
            'anomaly': 'Activity outside normal patterns',
            'security_alert': 'Security mode was triggered',
            'node_online': 'A node came online',
            'node_offline': 'A node went offline',
            'system': 'System status changed',
            'learning_milestone': 'System learned something new'
        };

        return descriptions[event.type] || '';
    }

    /**
     * Get zone history HTML
     */
    function getZoneHistory(zoneName) {
        const zoneEvents = currentState.events
            .filter(e => e.zone === zoneName)
            .slice(0, 5);

        if (zoneEvents.length === 0) {
            return '<div class="history-item">No recent activity</div>';
        }

        return zoneEvents.map(event => `
            <div class="history-item">
                <span class="history-event">${formatEventTitle(event)}</span>
                <span class="history-time">${formatTimestamp(event.timestamp_ms)}</span>
            </div>
        `).join('');
    }

    /**
     * Get sleep quality label
     */
    function getSleepQualityLabel(sleep) {
        if (!sleep || !sleep.duration_min) return '--';

        const duration = sleep.duration_min;
        if (duration >= 420) return 'Great';
        if (duration >= 360) return 'Good';
        if (duration >= 300) return 'Fair';
        return 'Poor';
    }

    /**
     * Get restlessness label
     */
    function getRestlessnessLabel(restlessness) {
        if (!restlessness) return 'Unknown';
        if (restlessness < 1) return 'Calm';
        if (restlessness < 2) return 'Normal';
        if (restlessness < 3) return 'Restless';
        return 'Very restless';
    }

    /**
     * Show toast notification
     */
    function showToast(message) {
        // Use existing toast system if available
        if (window.showToast) {
            window.showToast(message, 'info');
            return;
        }

        // Otherwise create a simple toast
        const toast = document.createElement('div');
        toast.className = 'toast info';
        toast.textContent = message;
        toast.style.cssText = `
            position: fixed;
            bottom: 100px;
            left: 50%;
            transform: translateX(-50%);
            background: rgba(33, 150, 243, 0.95);
            color: white;
            padding: 12px 20px;
            border-radius: 8px;
            z-index: 300;
            animation: slideUp 0.3s ease-out;
        `;

        document.body.appendChild(toast);

        setTimeout(() => {
            toast.style.animation = 'fadeOut 0.3s ease-out forwards';
            setTimeout(() => toast.remove(), 300);
        }, 3000);
    }

    /**
     * Show error message
     */
    function showError(message) {
        if (window.showToast) {
            window.showToast(message, 'warning');
            return;
        }

        // Create error toast
        const toast = document.createElement('div');
        toast.className = 'toast warning';
        toast.textContent = message;
        toast.style.cssText = `
            position: fixed;
            bottom: 100px;
            left: 50%;
            transform: translateX(-50%);
            background: rgba(255, 152, 0, 0.95);
            color: white;
            padding: 12px 20px;
            border-radius: 8px;
            z-index: 300;
        `;

        document.body.appendChild(toast);

        setTimeout(() => toast.remove(), 5000);
    }

    /**
     * Update room cards from blob data
     */
    function updateRoomCardsFromBlobs(blobs) {
        if (!blobs || blobs.length === 0) return;

        // Update zone occupancy based on blob positions
        blobs.forEach(blob => {
            const zone = findZoneForPosition(blob.x, blob.y);
            if (zone) {
                // Update zone occupancy based on blob presence
                updateZoneOccupancy(zone.id, blob);
            }
        });

        // Re-render room cards with updated data
        renderRoomCards(currentState.zones);
    }

    /**
     * Find zone that contains a position
     */
    function findZoneForPosition(x, y) {
        return currentState.zones.find(zone => {
            return x >= zone.x && x < zone.x + zone.w &&
                   y >= zone.y && y < zone.y + zone.d;
        });
    }

    /**
     * Update zone occupancy based on blob
     */
    function updateZoneOccupancy(zoneId, blob) {
        const zone = currentState.zones.find(z => z.id === zoneId);
        if (!zone) return;

        // Check if this blob is already counted
        if (!zone.people) zone.people = [];
        if (!zone.people.includes(blob.person)) {
            zone.people.push(blob.person || 'Unknown');
        }
        zone.occupancy = zone.people.length;
        zone.occupancy_updated_at = Date.now();
    }

    /**
     * Add event to activity feed
     */
    function addEventToFeed(event) {
        if (!event) return;

        // Add to beginning of events array
        currentState.events.unshift(event);

        // Keep only last 50 events
        if (currentState.events.length > 50) {
            currentState.events.pop();
        }

        // Re-render activity feed
        renderActivityFeed(currentState.events);
    }

    /**
     * Handle alert
     */
    function handleAlert(alert) {
        currentState.alerts.push(alert);

        // Show alert banner
        const banner = document.querySelector('.simple-alert-banner');
        if (banner) {
            banner.remove();
        }

        const container = document.getElementById('simple-mode-content');
        if (!container) return;

        // Insert alert banner at the top
        const alertHtml = renderAlertBanner(alert);
        container.insertAdjacentHTML('afterbegin', alertHtml);

        // Attach dismiss handler
        const newBanner = container.querySelector('.simple-alert-banner');
        const dismissBtn = newBanner?.querySelector('.alert-dismiss');
        if (dismissBtn) {
            dismissBtn.addEventListener('click', () => {
                newBanner.remove();
                currentState.alerts = currentState.alerts.filter(a => a.id !== alert.id);
            });
        }
    }

    /**
     * Update trigger state
     */
    function updateTriggerState(trigger) {
        const index = currentState.triggers.findIndex(t => t.id === trigger.id);
        if (index >= 0) {
            currentState.triggers[index] = trigger;
        } else {
            currentState.triggers.push(trigger);
        }
    }

    // ============================================
    // Public API
    // ============================================
    window.SpaxelSimpleMode = {
        init: init,
        enable: enableSimpleMode,
        disable: disableSimpleMode,
        isEnabled: () => isSimpleMode,
        refresh: fetchAllData
    };

    // Auto-initialize
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }

    console.log('[Simple Mode] Module loaded');
})();
