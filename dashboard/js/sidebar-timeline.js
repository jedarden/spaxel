/**
 * Spaxel Dashboard - Sidebar Timeline Panel
 *
 * A collapsible sidebar panel showing events in reverse-chronological order.
 * Features:
 * - Event-specific visual rendering with icons and descriptions per event type
 * - Thumbs-up/down buttons on each event delegating to feedback module
 * - Virtualized rendering with IntersectionObserver for 1000+ events
 * - Real-time event updates via WebSocket
 */
(function() {
    'use strict';

    // ============================================
    // Configuration
    // ============================================
    const CONFIG = {
        initialLoadLimit: 100,
        maxEventsInMemory: 10000,
        virtualization: {
            enabled: true,
            bufferSize: 20, // number of extra items to render above/below viewport
            rootMargin: '200px', // load items 200px before they enter viewport
            threshold: 0.01
        },
        itemHeight: 70, // estimated height of a sidebar event item in pixels
        maxDOMItems: 50 // maximum number of items to keep in DOM at once
    };

    // ============================================
    // Event Type Information
    // ============================================
    const EVENT_TYPES = {
        zone_entry: {
            icon: '🚪',
            label: 'Entered',
            description: '{person} entered {zone}',
            category: 'zones'
        },
        zone_exit: {
            icon: '🚶',
            label: 'Left',
            description: '{person} left {zone}',
            category: 'zones'
        },
        portal_crossing: {
            icon: '→',
            label: 'Crossed',
            description: '{person} crossed from {from_zone} to {to_zone}',
            category: 'zones'
        },
        presence_transition: {
            icon: '👤',
            label: 'Detected',
            description: '{person} detected in {zone}',
            category: 'presence'
        },
        stationary_detected: {
            icon: '💤',
            label: 'Stationary',
            description: '{person} is stationary in {zone}',
            category: 'presence'
        },
        detection: {
            icon: '👁️',
            label: 'Motion',
            description: 'Motion detected in {zone}',
            category: 'presence'
        },
        anomaly: {
            icon: '⚠️',
            label: 'Unusual Activity',
            description: 'Unusual activity detected in {zone}',
            category: 'alerts'
        },
        anomaly_detected: {
            icon: '⚠️',
            label: 'Anomaly',
            description: 'Anomaly detected in {zone}',
            category: 'alerts'
        },
        security_alert: {
            icon: '🚨',
            label: 'Security Alert',
            description: 'Security alert: {description}',
            category: 'alerts'
        },
        fall_alert: {
            icon: '🆘',
            label: 'Fall Detected',
            description: 'Fall detected: {person} in {zone}',
            category: 'alerts'
        },
        node_online: {
            icon: '📡',
            label: 'Node Online',
            description: 'Node {node} came online',
            category: 'system'
        },
        node_offline: {
            icon: '📵',
            label: 'Node Offline',
            description: 'Node {node} went offline',
            category: 'system'
        },
        ota_update: {
            icon: '⬆️',
            label: 'Firmware Updated',
            description: 'Node {node} firmware updated to {version}',
            category: 'system'
        },
        baseline_changed: {
            icon: '📊',
            label: 'Baseline Updated',
            description: 'Baseline updated for {link}',
            category: 'system'
        },
        system: {
            icon: '⚙️',
            label: 'System',
            description: '{description}',
            category: 'system'
        },
        learning_milestone: {
            icon: '🎓',
            label: 'Learning Complete',
            description: '{description}',
            category: 'learning'
        },
        anomaly_learned: {
            icon: '🧠',
            label: 'Pattern Learned',
            description: '{description}',
            category: 'learning'
        },
        sleep_session_end: {
            icon: '😴',
            label: 'Sleep Session',
            description: '{person} slept for {duration}',
            category: 'presence'
        }
    };

    // ============================================
    // State
    // ============================================
    const state = {
        events: [],
        cursor: null,
        total: 0,
        loading: false,
        panelVisible: false,
        dashboardMode: 'expert', // 'expert' or 'simple' - determines timeline mode
        selectedEventId: null,   // ID of the currently selected (jumped-to) event
        // Virtualization state
        virtualization: {
            observer: null,
            visibleIndices: new Set(),
            renderedIndices: new Set(),
            firstVisibleIndex: 0,
            lastVisibleIndex: 0,
            scrollTop: 0
        }
    };

    // DOM elements cache
    const elements = {};

    // ============================================
    // Initialization
    // ============================================
    function init() {
        console.log('[SidebarTimeline] Initializing');

        cacheElements();
        bindEvents();
        setupVirtualization();

        // Listen for simple mode changes
        if (window.SpaxelSimpleModeDetection) {
            SpaxelSimpleModeDetection.onModeChange(onSimpleModeChange);
        }

        // Listen for router mode changes
        if (window.SpaxelRouter) {
            SpaxelRouter.onModeChange(onRouterModeChange);
        }

        loadInitialEvents();

        // Register for WebSocket messages
        if (window.SpaxelApp) {
            SpaxelApp.registerMessageHandler(handleWebSocketMessage);
        }

        console.log('[SidebarTimeline] Initialized');
    }

    function cacheElements() {
        elements.panel = document.getElementById('sidebar-timeline-panel');
        elements.content = document.getElementById('sidebar-timeline-content');
        elements.eventsContainer = document.getElementById('sidebar-timeline-events');
        elements.loading = document.getElementById('sidebar-timeline-loading');
        elements.empty = document.getElementById('sidebar-timeline-empty');
        elements.spacerTop = document.getElementById('sidebar-timeline-spacer-top');
        elements.spacerBottom = document.getElementById('sidebar-timeline-spacer-bottom');
        elements.toggleBtn = document.getElementById('sidebar-timeline-toggle');
        elements.closeBtn = document.getElementById('sidebar-timeline-close');
        elements.showBtn = document.getElementById('sidebar-timeline-show-btn');
    }

    function bindEvents() {
        // Panel toggle buttons
        if (elements.toggleBtn) {
            elements.toggleBtn.addEventListener('click', togglePanel);
        }
        if (elements.closeBtn) {
            elements.closeBtn.addEventListener('click', hidePanel);
        }
        if (elements.showBtn) {
            elements.showBtn.addEventListener('click', showPanel);
        }

        // Scroll events for virtualization
        if (elements.content) {
            elements.content.addEventListener('scroll', onScroll, { passive: true });
        }
    }

    // ============================================
    // Mode Change Handlers
    // ============================================
    function onSimpleModeChange(newMode, oldMode) {
        console.log('[SidebarTimeline] Simple mode changed from', oldMode, 'to', newMode);

        // Update dashboard mode based on simple mode
        if (newMode === 'simple') {
            state.dashboardMode = 'simple';
        } else {
            state.dashboardMode = 'expert';
        }

        // Reload events with new mode
        if (state.panelVisible) {
            loadInitialEvents();
        }
    }

    function onRouterModeChange(newMode, oldMode) {
        // Determine dashboard mode: expert mode shows all events, simple mode shows person-relevant only
        if (newMode === 'live' || newMode === 'replay' || newMode === 'timeline') {
            state.dashboardMode = 'expert';
        } else if (newMode === 'simple' || newMode === 'ambient') {
            state.dashboardMode = 'simple';
        } else {
            state.dashboardMode = 'expert'; // Default to expert
        }

        // Reload events if panel is visible
        if (state.panelVisible) {
            loadInitialEvents();
        }
    }

    // ============================================
    // Panel Visibility
    // ============================================
    function showPanel() {
        if (elements.panel) {
            elements.panel.classList.remove('collapsed');
            state.panelVisible = true;
        }
        if (elements.showBtn) {
            elements.showBtn.classList.add('hidden');
        }
    }

    function hidePanel() {
        if (elements.panel) {
            elements.panel.classList.add('collapsed');
            state.panelVisible = false;
        }
        if (elements.showBtn) {
            elements.showBtn.classList.remove('hidden');
        }
    }

    function togglePanel() {
        if (state.panelVisible) {
            hidePanel();
        } else {
            showPanel();
        }
    }

    // ============================================
    // Event Loading
    // ============================================
    function loadInitialEvents() {
        const params = new URLSearchParams();
        params.set('limit', CONFIG.initialLoadLimit);
        params.set('mode', state.dashboardMode);

        fetch('/api/events?' + params.toString())
            .then(function(res) {
                if (!res.ok) {
                    throw new Error('Failed to fetch events: ' + res.statusText);
                }
                return res.json();
            })
            .then(function(data) {
                state.events = data.events || [];
                state.cursor = data.cursor || null;
                state.total = data.total_filtered || 0;
                renderEvents();
            })
            .catch(function(err) {
                console.error('[SidebarTimeline] Failed to load events:', err);
                showError(err.message);
            })
            .finally(function() {
                state.loading = false;
                updateLoadingState();
            });
    }

    // ============================================
    // Virtualization Setup
    // ============================================
    function setupVirtualization() {
        if (!CONFIG.virtualization.enabled || !elements.content) {
            return;
        }

        // Create IntersectionObserver for lazy rendering
        const observerOptions = {
            root: elements.content,
            rootMargin: CONFIG.virtualization.rootMargin,
            threshold: CONFIG.virtualization.threshold
        };

        state.virtualization.observer = new IntersectionObserver(function(entries) {
            handleIntersection(entries);
        }, observerOptions);

        console.log('[SidebarTimeline] Virtualization enabled');
    }

    function handleIntersection(entries) {
        entries.forEach(function(entry) {
            const index = parseInt(entry.target.dataset.index, 10);
            if (isNaN(index)) return;

            if (entry.isIntersecting) {
                state.virtualization.visibleIndices.add(index);
            } else {
                state.virtualization.visibleIndices.delete(index);
            }
        });

        // Update rendered range based on visibility
        updateRenderedRange();
    }

    function onScroll() {
        if (!elements.content) return;

        state.virtualization.scrollTop = elements.content.scrollTop;

        // Update visible range based on scroll position
        const firstIndex = Math.floor(state.virtualization.scrollTop / CONFIG.itemHeight);
        const visibleCount = Math.ceil(elements.content.clientHeight / CONFIG.itemHeight);
        const lastIndex = firstIndex + visibleCount;

        state.virtualization.firstVisibleIndex = Math.max(0, firstIndex - CONFIG.virtualization.bufferSize);
        state.virtualization.lastVisibleIndex = Math.min(state.events.length - 1, lastIndex + CONFIG.virtualization.bufferSize);

        updateRenderedRange();
    }

    function updateRenderedRange() {
        if (!state.events.length) return;

        const firstIdx = Math.max(0, state.virtualization.firstVisibleIndex);
        const lastIdx = Math.min(state.events.length - 1, state.virtualization.lastVisibleIndex);

        // Create new set of rendered indices
        const newRenderedIndices = new Set();
        for (let i = firstIdx; i <= lastIdx; i++) {
            newRenderedIndices.add(i);
        }

        // Render new items
        const fragment = document.createDocumentFragment();
        newRenderedIndices.forEach(function(index) {
            if (!state.virtualization.renderedIndices.has(index)) {
                const event = state.events[index];
                if (event) {
                    const tempDiv = document.createElement('div');
                    tempDiv.innerHTML = renderEvent(event, false, index);
                    const newEventEl = tempDiv.firstElementChild;
                    if (newEventEl) {
                        newEventEl.dataset.index = index;
                        fragment.appendChild(newEventEl);
                    }
                }
            }
        });

        if (fragment.children.length > 0) {
            elements.eventsContainer.appendChild(fragment);

            // Bind handlers for new items
            Array.from(fragment.children).forEach(function(item) {
                bindEventHandlers(item);
                if (state.virtualization.observer) {
                    state.virtualization.observer.observe(item);
                }
            });
        }

        // Remove items that are no longer in rendered range
        state.virtualization.renderedIndices.forEach(function(index) {
            if (!newRenderedIndices.has(index)) {
                const item = elements.eventsContainer.querySelector('[data-index="' + index + '"]');
                if (item) {
                    if (state.virtualization.observer) {
                        state.virtualization.observer.unobserve(item);
                    }
                    item.remove();
                }
            }
        });

        state.virtualization.renderedIndices = newRenderedIndices;

        // Update spacers
        updateSpacers();
    }

    function updateSpacers() {
        if (!elements.spacerTop || !elements.spacerBottom) return;

        const topHeight = state.virtualization.firstVisibleIndex * CONFIG.itemHeight;
        const bottomHeight = (state.events.length - state.virtualization.lastVisibleIndex - 1) * CONFIG.itemHeight;

        elements.spacerTop.style.height = Math.max(0, topHeight) + 'px';
        elements.spacerBottom.style.height = Math.max(0, bottomHeight) + 'px';
    }

    // ============================================
    // Rendering
    // ============================================
    function renderEvents() {
        if (!elements.eventsContainer) return;

        if (state.events.length === 0) {
            elements.eventsContainer.innerHTML = '';
            if (elements.empty) {
                elements.empty.style.display = 'flex';
            }
            if (elements.loading) {
                elements.loading.style.display = 'none';
            }
            return;
        }

        elements.empty.style.display = 'none';
        elements.loading.style.display = 'none';

        // Use virtualized rendering for large datasets
        if (CONFIG.virtualization.enabled && state.events.length > CONFIG.maxDOMItems) {
            renderVirtualized();
        } else {
            renderAll();
        }
    }

    function renderAll() {
        let html = '';
        state.events.forEach(function(event, index) {
            html += renderEvent(event, false, index);
        });
        elements.eventsContainer.innerHTML = html;

        // Bind handlers
        Array.from(elements.eventsContainer.children).forEach(function(item) {
            bindEventHandlers(item);
        });
    }

    function renderVirtualized() {
        // Clear existing content
        elements.eventsContainer.innerHTML = '';

        // Calculate initial visible range
        const containerHeight = elements.content.clientHeight || 400;
        const visibleCount = Math.ceil(containerHeight / CONFIG.itemHeight);
        const bufferCount = CONFIG.virtualization.bufferSize;

        state.virtualization.firstVisibleIndex = 0;
        state.virtualization.lastVisibleIndex = Math.min(state.events.length - 1, visibleCount + bufferCount * 2);

        // Create spacers
        updateSpacers();

        // Render initial batch
        updateRenderedRange();
    }

    function renderEvent(event, isNew, index) {
        const typeInfo = EVENT_TYPES[event.type] || EVENT_TYPES.system;
        const timeStr = formatTimestamp(event.timestamp_ms);
        const description = buildEventDescription(event, typeInfo);

        // Severity indicator
        const severityClass = event.severity === 'alert' || event.severity === 'critical' ? ' severity-critical' :
                            event.severity === 'warning' ? ' severity-warning' : '';
        const newClass = isNew ? ' new-event' : '';

        // Determine if this is a system event (for secondary styling in expert mode only)
        const systemEventTypes = ['node_online', 'node_offline', 'ota_update', 'baseline_changed', 'system', 'learning_milestone', 'anomaly_learned'];
        const isSystemEvent = systemEventTypes.indexOf(event.type) !== -1;
        // Only apply secondary class in expert mode for system events
        const secondaryClass = (state.dashboardMode === 'expert' && isSystemEvent) ? ' secondary' : '';

        const dataIndex = index !== undefined ? ` data-index="${index}"` : '';

        return `
            <div class="sidebar-timeline-event${severityClass}${newClass}${secondaryClass}"
                 data-type="${event.type}"
                 data-id="${event.id}"
                 data-timestamp="${event.timestamp_ms}"${dataIndex}>
                <div class="sidebar-timeline-event-icon">${typeInfo.icon}</div>
                <div class="sidebar-timeline-event-content">
                    <div class="sidebar-timeline-event-title">${escapeHtml(description)}</div>
                    <div class="sidebar-timeline-event-meta">
                        <span class="sidebar-timeline-event-time">${timeStr}</span>
                    </div>
                </div>
                <div class="sidebar-timeline-event-actions">
                    <button class="sidebar-timeline-action-btn feedback-positive"
                            data-action="correct"
                            title="Correct detection"
                            aria-label="Thumbs up">👍</button>
                    <button class="sidebar-timeline-action-btn feedback-negative"
                            data-action="incorrect"
                            title="Incorrect detection"
                            aria-label="Thumbs down">👎</button>
                </div>
            </div>
        `;
    }

    function buildEventDescription(event, typeInfo) {
        // Parse detail_json for additional context
        let detail = {};
        if (event.detail_json) {
            try {
                detail = JSON.parse(event.detail_json);
            } catch (e) {
                // Ignore parse errors
            }
        }

        // Build description using template
        let description = typeInfo.description;

        // Replace placeholders with actual values
        const replacements = {
            '{person}': event.person || detail.person || 'Someone',
            '{zone}': event.zone || detail.zone || 'the area',
            '{from_zone}': detail.from_zone || 'previous zone',
            '{to_zone}': detail.to_zone || 'next zone',
            '{node}': detail.node || 'a node',
            '{version}': detail.version || 'latest',
            '{link}': detail.link || 'a link',
            '{description}': detail.description || 'activity detected',
            '{duration}': detail.duration || 'a while'
        };

        // Apply replacements
        for (const [placeholder, value] of Object.entries(replacements)) {
            description = description.replace(placeholder, value);
        }

        return description;
    }

    function formatTimestamp(ms) {
        const date = new Date(ms);
        const now = new Date();
        const isToday = date.toDateString() === now.toDateString();

        const time = date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });

        if (isToday) {
            return time;
        } else {
            return date.toLocaleDateString() + ' ' + time;
        }
    }

    function escapeHtml(str) {
        if (!str) return '';
        return String(str)
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;');
    }

    // ============================================
    // Event Handlers
    // ============================================
    function bindEventHandlers(eventEl) {
        // Feedback buttons
        eventEl.querySelectorAll('.sidebar-timeline-action-btn').forEach(function(btn) {
            btn.addEventListener('click', function(e) {
                e.stopPropagation();
                const action = btn.dataset.action;
                const eventId = eventEl.dataset.id;
                handleFeedback(eventId, action, eventEl);
            });
        });

        // Event click (seeks to timestamp in replay)
        eventEl.addEventListener('click', function() {
            const timestamp = parseInt(this.dataset.timestamp, 10);
            handleSeek(timestamp, this);
        });
    }

    function handleFeedback(eventId, action, eventElement) {
        const correct = action === 'correct';

        // Delegate to feedback module if available
        if (window.Feedback) {
            const event = state.events.find(function(e) { return e.id == eventId; });
            if (event) {
                let detail = {};
                try {
                    detail = event.detail_json ? JSON.parse(event.detail_json) : {};
                } catch (e) {}

                // Call feedback module's sendFeedback
                Feedback.sendFeedback(eventId, event.type, correct ? 'TRUE_POSITIVE' : 'FALSE_POSITIVE', detail);

                // Show visual feedback immediately
                const feedbackBtns = eventElement.querySelectorAll('.sidebar-timeline-action-btn');
                feedbackBtns.forEach(function(btn) {
                    if (btn.dataset.action === action) {
                        btn.classList.add('active');
                        setTimeout(function() {
                            btn.classList.remove('active');
                        }, 2000);
                    }
                });

                // Show toast notification
                if (window.SpaxelApp && SpaxelApp.showToast) {
                    const message = correct ? 'Thanks for the feedback!' : 'Thanks — I\'ll adjust my detection.';
                    SpaxelApp.showToast(message, 'success');
                }

                // Dismiss entry for incorrect feedback
                if (!correct) {
                    eventElement.classList.add('feedback-dismissed');
                }
            }
        } else {
            // Fallback: direct API call
            const payload = {
                type: correct ? 'correct' : 'incorrect',
                event_id: parseInt(eventId, 10)
            };

            fetch('/api/feedback', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(payload)
            })
                .then(function(res) {
                    if (res.ok) {
                        return res.json();
                    }
                    throw new Error('Failed to submit feedback');
                })
                .then(function(data) {
                    // Show visual feedback
                    const feedbackBtns = eventElement.querySelectorAll('.sidebar-timeline-action-btn');
                    feedbackBtns.forEach(function(btn) {
                        if (btn.dataset.action === action) {
                            btn.classList.add('active');
                            setTimeout(function() {
                                btn.classList.remove('active');
                            }, 2000);
                        }
                    });

                    // Show toast notification
                    if (window.SpaxelApp && SpaxelApp.showToast) {
                        const message = correct ? 'Thanks for the feedback!' : 'Thanks — I\'ll adjust my detection.';
                        SpaxelApp.showToast(message, 'success');
                    }

                    // Dismiss entry for incorrect feedback
                    if (!correct) {
                        eventElement.classList.add('feedback-dismissed');
                    }
                })
                .catch(function(err) {
                    console.error('[SidebarTimeline] Feedback failed:', err);
                });
        }
    }

    function handleSeek(timestamp, eventEl) {
        if (!timestamp || timestamp <= 0) return;

        // Highlight selected event
        clearSelectedEvent();
        if (eventEl) {
            state.selectedEventId = eventEl.dataset.id;
            eventEl.classList.add('selected');
        }

        // In expert mode, use jump-to-time replay
        if (state.dashboardMode === 'expert' && window.SpaxelReplay) {
            SpaxelReplay.jumpToTime(timestamp).then(function() {
                updateNowReplayingChip(true, timestamp);
            }).catch(function(err) {
                console.error('[SidebarTimeline] Jump-to-time failed:', err);
                if (window.SpaxelApp && SpaxelApp.showToast) {
                    SpaxelApp.showToast('Failed to jump to time', 'error');
                }
            });
        } else {
            // Simple mode: navigate to timeline view
            if (window.SpaxelRouter) {
                SpaxelRouter.navigate('timeline');
            }
        }
    }

    function clearSelectedEvent() {
        if (state.selectedEventId) {
            const prev = elements.eventsContainer
                ? elements.eventsContainer.querySelector('.sidebar-timeline-event.selected')
                : null;
            if (prev) {
                prev.classList.remove('selected');
            }
            state.selectedEventId = null;
        }
    }

    function updateNowReplayingChip(visible, timestampMs) {
        let chip = document.getElementById('now-replaying-chip');
        if (!chip) {
            // Create chip in the sidebar panel header
            const header = document.querySelector('.sidebar-panel-header');
            if (!header) return;
            chip = document.createElement('span');
            chip.id = 'now-replaying-chip';
            chip.className = 'now-replaying-chip';
            header.appendChild(chip);
        }

        if (visible && timestampMs) {
            const date = new Date(timestampMs);
            const time = date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
            chip.textContent = 'Now replaying ' + time;
            chip.style.display = 'inline-flex';
        } else {
            chip.style.display = 'none';
        }
    }

    // ============================================
    // WebSocket Message Handler
    // ============================================
    function handleWebSocketMessage(msg) {
        if (msg.type === 'event') {
            handleNewEvent(msg.event);
        }
    }

    function handleNewEvent(event) {
        // Normalize event format
        const normalizedEvent = {
            id: event.id || event.timestamp_ms || Date.now(),
            timestamp_ms: event.timestamp_ms || event.ts || Date.now(),
            type: event.type || event.kind || 'system',
            zone: event.zone || '',
            person: event.person || event.person_name || '',
            blob_id: event.blob_id || event.blobID || 0,
            detail_json: event.detail_json || '',
            severity: event.severity || 'info'
        };

        // Add to beginning of events
        state.events.unshift(normalizedEvent);
        state.total++;

        // Limit events in memory
        if (state.events.length > CONFIG.maxEventsInMemory) {
            state.events = state.events.slice(0, CONFIG.maxEventsInMemory);
        }

        // Re-render if panel is visible
        if (state.panelVisible) {
            renderEvents();
        }
    }

    // ============================================
    // UI Updates
    // ============================================
    function updateLoadingState() {
        if (!elements.loading) return;
        elements.loading.style.display = state.loading ? 'flex' : 'none';
    }

    function showError(message) {
        console.error('[SidebarTimeline]', message);
    }

    // ============================================
    // Public API
    // ============================================
    const SidebarTimeline = {
        init: init,
        show: showPanel,
        hide: hidePanel,
        toggle: togglePanel,
        refresh: loadInitialEvents,
        isVisible: function() { return state.panelVisible; },
        clearSelection: clearSelectedEvent,
        hideNowReplayingChip: function() { updateNowReplayingChip(false); }
    };

    // Auto-initialize when DOM is ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }

    // Export for use by other modules
    window.SpaxelSidebarTimeline = SidebarTimeline;

    console.log('[SidebarTimeline] Module loaded');
})();
