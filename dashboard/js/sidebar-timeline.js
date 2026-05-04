/**
 * Spaxel Dashboard - Sidebar Timeline Panel
 *
 * A collapsible sidebar panel showing events in reverse-chronological order.
 * Features:
 * - Category checkboxes (Presence, Zones, Alerts, System, Learning)
 * - Person + zone dropdowns for subset filtering
 * - Date range selector with server-side re-fetch for today/7d/30d/custom
 * - Text search with fuzzy client-side matching and FTS5 server-side
 * - Client-side filtering on loaded events; server-side for date-range queries
 * - Load more cursor pagination for 500+ results
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
        searchDebounceMs: 300,
        virtualization: {
            enabled: true,
            bufferSize: 20,
            rootMargin: '200px',
            threshold: 0.01
        },
        itemHeight: 70,
        maxDOMItems: 50
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

    // Map category names to the event type keys they contain.
    const CATEGORY_TYPES = {
        presence: ['presence_transition', 'stationary_detected', 'detection', 'sleep_session_end'],
        zones: ['zone_entry', 'zone_exit', 'portal_crossing'],
        alerts: ['anomaly', 'anomaly_detected', 'security_alert', 'fall_alert'],
        system: ['node_online', 'node_offline', 'ota_update', 'baseline_changed', 'system'],
        learning: ['learning_milestone', 'anomaly_learned']
    };

    // ============================================
    // State
    // ============================================
    const state = {
        allLoadedEvents: [],    // all events fetched from server (before client filter)
        events: [],             // display events (after client-side filtering)
        cursor: null,
        hasMore: false,
        total: 0,
        loading: false,
        panelVisible: false,
        selectedEventId: null,
        filters: {
            categories: { presence: true, zones: true, alerts: true, system: false, learning: false },
            person: '',
            zone: '',
            dateRange: 'all',
            customFrom: '',
            customTo: '',
            searchQuery: ''
        },
        // Server-side date bounds (ISO8601 strings, empty = no bound)
        serverSince: '',
        serverUntil: '',
        filterControlsVisible: false,
        searchDebounceTimer: null,
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
        cacheElements();
        bindEvents();
        setupVirtualization();
        populateDropdowns();

        if (window.SpaxelRouter) {
            SpaxelRouter.onModeChange(onRouterModeChange);
        }

        loadInitialEvents();

        if (window.SpaxelApp) {
            SpaxelApp.registerMessageHandler(handleWebSocketMessage);
        }
    }

    function cacheElements() {
        elements.panel = document.getElementById('sidebar-timeline-panel');
        elements.content = document.getElementById('sidebar-timeline-content');
        elements.eventsContainer = document.getElementById('sidebar-timeline-events');
        elements.loading = document.getElementById('sidebar-timeline-loading');
        elements.empty = document.getElementById('sidebar-timeline-empty');
        elements.count = document.getElementById('sidebar-timeline-count');
        elements.spacerTop = document.getElementById('sidebar-timeline-spacer-top');
        elements.spacerBottom = document.getElementById('sidebar-timeline-spacer-bottom');
        elements.toggleBtn = document.getElementById('sidebar-timeline-toggle');
        elements.closeBtn = document.getElementById('sidebar-timeline-close');
        elements.showBtn = document.getElementById('sidebar-timeline-show-btn');
        elements.loadMoreContainer = document.getElementById('sidebar-load-more');
        elements.loadMoreBtn = document.getElementById('sidebar-load-more-btn');

        // Filter controls
        elements.filterBar = document.getElementById('sidebar-timeline-filter-bar');
        elements.filterToggleBtn = document.getElementById('sidebar-filter-toggle-btn');
        elements.filterControls = document.getElementById('sidebar-timeline-filter-controls');
        elements.searchInput = document.getElementById('sidebar-timeline-search');
        elements.categoryPresence = document.getElementById('filter-category-presence');
        elements.categoryZones = document.getElementById('filter-category-zones');
        elements.categoryAlerts = document.getElementById('filter-category-alerts');
        elements.categorySystem = document.getElementById('filter-category-system');
        elements.categoryLearning = document.getElementById('filter-category-learning');
        elements.personSelect = document.getElementById('sidebar-filter-person');
        elements.zoneSelect = document.getElementById('sidebar-filter-zone');
        elements.dateRangeSelect = document.getElementById('sidebar-filter-date-range');
        elements.customDateContainer = document.getElementById('sidebar-custom-date-container');
        elements.dateFrom = document.getElementById('sidebar-date-from');
        elements.dateTo = document.getElementById('sidebar-date-to');
        elements.dateApplyBtn = document.getElementById('sidebar-date-apply-btn');
        elements.activeFilters = document.getElementById('sidebar-active-filters');
        elements.activeFilterTags = document.getElementById('sidebar-active-filter-tags');
        elements.clearFiltersBtn = document.getElementById('sidebar-clear-filters-btn');
    }

    function bindEvents() {
        if (elements.toggleBtn) elements.toggleBtn.addEventListener('click', togglePanel);
        if (elements.closeBtn) elements.closeBtn.addEventListener('click', hidePanel);
        if (elements.showBtn) elements.showBtn.addEventListener('click', showPanel);

        if (elements.content) {
            elements.content.addEventListener('scroll', onScroll, { passive: true });
        }

        // Filter toggle
        if (elements.filterToggleBtn) {
            elements.filterToggleBtn.addEventListener('click', toggleFilterControls);
        }

        // Search input with debounce
        if (elements.searchInput) {
            elements.searchInput.addEventListener('input', function() {
                clearTimeout(state.searchDebounceTimer);
                state.searchDebounceTimer = setTimeout(function() {
                    state.filters.searchQuery = elements.searchInput.value.trim();
                    applyClientFilters();
                    updateActiveFilters();
                }, CONFIG.searchDebounceMs);
            });
        }

        // Category checkboxes
        var categoryMap = {
            'filter-category-presence': 'presence',
            'filter-category-zones': 'zones',
            'filter-category-alerts': 'alerts',
            'filter-category-system': 'system',
            'filter-category-learning': 'learning'
        };
        Object.keys(categoryMap).forEach(function(id) {
            var el = document.getElementById(id);
            if (el) {
                el.addEventListener('change', function() {
                    state.filters.categories[categoryMap[id]] = el.checked;
                    applyClientFilters();
                    updateActiveFilters();
                });
            }
        });

        // Person dropdown
        if (elements.personSelect) {
            elements.personSelect.addEventListener('change', function() {
                state.filters.person = elements.personSelect.value;
                applyClientFilters();
                updateActiveFilters();
            });
        }

        // Zone dropdown
        if (elements.zoneSelect) {
            elements.zoneSelect.addEventListener('change', function() {
                state.filters.zone = elements.zoneSelect.value;
                applyClientFilters();
                updateActiveFilters();
            });
        }

        // Date range selector
        if (elements.dateRangeSelect) {
            elements.dateRangeSelect.addEventListener('change', function() {
                handleDateRangeChange(elements.dateRangeSelect.value);
            });
        }

        // Custom date apply
        if (elements.dateApplyBtn) {
            elements.dateApplyBtn.addEventListener('click', applyCustomDateRange);
        }

        // Clear all filters
        if (elements.clearFiltersBtn) {
            elements.clearFiltersBtn.addEventListener('click', clearAllFilters);
        }

        // Load more
        if (elements.loadMoreBtn) {
            elements.loadMoreBtn.addEventListener('click', loadMoreEvents);
        }
    }

    // ============================================
    // Dropdown Population
    // ============================================
    function populateDropdowns() {
        // People
        fetch('/api/people')
            .then(function(res) { return res.ok ? res.json() : null; })
            .then(function(data) {
                if (!data || !elements.personSelect) return;
                var people = data.people || data || [];
                people.forEach(function(p) {
                    var name = p.name || p.label || p.id || '';
                    if (!name) return;
                    var opt = document.createElement('option');
                    opt.value = name;
                    opt.textContent = name;
                    elements.personSelect.appendChild(opt);
                });
            })
            .catch(function() {
                // People endpoint may not be available yet — silently skip
            });

        // Zones
        fetch('/api/zones')
            .then(function(res) { return res.ok ? res.json() : null; })
            .then(function(data) {
                if (!data || !elements.zoneSelect) return;
                var zones = Array.isArray(data) ? data : (data.zones || []);
                zones.forEach(function(z) {
                    var name = z.name || '';
                    if (!name) return;
                    var opt = document.createElement('option');
                    opt.value = name;
                    opt.textContent = name;
                    elements.zoneSelect.appendChild(opt);
                });
            })
            .catch(function() {
                // Zones endpoint may not be available yet — silently skip
            });
    }

    // ============================================
    // Mode Change Handlers
    // ============================================
    function onRouterModeChange() {
        if (state.panelVisible) {
            resetAndReload();
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
    // Filter Controls Visibility
    // ============================================
    function toggleFilterControls() {
        state.filterControlsVisible = !state.filterControlsVisible;
        if (elements.filterControls) {
            if (state.filterControlsVisible) {
                elements.filterControls.classList.remove('collapsed');
            } else {
                elements.filterControls.classList.add('collapsed');
            }
        }
        if (elements.filterToggleBtn) {
            elements.filterToggleBtn.classList.toggle('active', state.filterControlsVisible);
        }
    }

    // ============================================
    // Date Range Handling (server-side)
    // ============================================
    function handleDateRangeChange(value) {
        state.filters.dateRange = value;

        if (elements.customDateContainer) {
            elements.customDateContainer.style.display = value === 'custom' ? 'flex' : 'none';
        }

        if (value === 'custom') {
            // Wait for the Apply button
            return;
        }

        var now = new Date();
        state.serverSince = '';
        state.serverUntil = '';

        switch (value) {
            case 'today': {
                var start = new Date(now.getFullYear(), now.getMonth(), now.getDate());
                state.serverSince = start.toISOString();
                break;
            }
            case '7days':
                state.serverSince = new Date(now.getTime() - 7 * 24 * 60 * 60 * 1000).toISOString();
                break;
            case '30days':
                state.serverSince = new Date(now.getTime() - 30 * 24 * 60 * 60 * 1000).toISOString();
                break;
            default:
                // 'all' — no bounds
                break;
        }

        updateActiveFilters();
        resetAndReload();
    }

    function applyCustomDateRange() {
        var fromVal = elements.dateFrom ? elements.dateFrom.value : '';
        var toVal = elements.dateTo ? elements.dateTo.value : '';

        state.filters.customFrom = fromVal;
        state.filters.customTo = toVal;

        // Convert date strings (YYYY-MM-DD) to ISO8601
        if (fromVal) {
            state.serverSince = new Date(fromVal).toISOString();
        } else {
            state.serverSince = '';
        }
        if (toVal) {
            // Include the full day by setting to end-of-day
            var endOfDay = new Date(toVal);
            endOfDay.setHours(23, 59, 59, 999);
            state.serverUntil = endOfDay.toISOString();
        } else {
            state.serverUntil = '';
        }

        updateActiveFilters();
        resetAndReload();
    }

    // ============================================
    // Event Loading
    // ============================================

    function buildServerParams(cursor) {
        var params = new URLSearchParams();
        params.set('limit', CONFIG.initialLoadLimit);

        if (state.serverSince) params.set('since', state.serverSince);
        if (state.serverUntil) params.set('until', state.serverUntil);

        // Pass the search query to FTS5 when doing a fresh fetch
        if (state.filters.searchQuery) {
            params.set('q', state.filters.searchQuery);
        }

        if (cursor) params.set('before', cursor);

        return params;
    }

    function resetAndReload() {
        state.allLoadedEvents = [];
        state.events = [];
        state.cursor = null;
        state.hasMore = false;

        if (elements.eventsContainer) elements.eventsContainer.innerHTML = '';
        renderLoadMore();
        updateCountDisplay();
        loadInitialEvents();
    }

    function loadInitialEvents() {
        if (state.loading) return;
        state.loading = true;
        updateLoadingState();

        var params = buildServerParams(null);

        fetch('/api/events?' + params.toString())
            .then(function(res) {
                if (!res.ok) throw new Error('Failed to fetch events: ' + res.statusText);
                return res.json();
            })
            .then(function(data) {
                state.allLoadedEvents = data.events || [];
                state.cursor = data.cursor || null;
                state.hasMore = data.has_more || false;
                state.total = data.total_filtered || 0;
                applyClientFilters();
                renderLoadMore();
                updateCountDisplay();
            })
            .catch(function(err) {
                console.error('[SidebarTimeline] Failed to load events:', err);
            })
            .finally(function() {
                state.loading = false;
                updateLoadingState();
            });
    }

    function loadMoreEvents() {
        if (state.loading || !state.hasMore || !state.cursor) return;
        state.loading = true;
        updateLoadingState();

        if (elements.loadMoreBtn) {
            elements.loadMoreBtn.disabled = true;
            elements.loadMoreBtn.textContent = 'Loading...';
        }

        var params = buildServerParams(state.cursor);

        fetch('/api/events?' + params.toString())
            .then(function(res) {
                if (!res.ok) throw new Error('Failed to fetch events: ' + res.statusText);
                return res.json();
            })
            .then(function(data) {
                var newEvents = data.events || [];
                state.allLoadedEvents = state.allLoadedEvents.concat(newEvents);

                // Trim to memory limit
                if (state.allLoadedEvents.length > CONFIG.maxEventsInMemory) {
                    state.allLoadedEvents = state.allLoadedEvents.slice(0, CONFIG.maxEventsInMemory);
                }

                state.cursor = data.cursor || null;
                state.hasMore = data.has_more || false;
                state.total = data.total_filtered || 0;

                applyClientFilters();
                renderLoadMore();
                updateCountDisplay();
            })
            .catch(function(err) {
                console.error('[SidebarTimeline] Failed to load more events:', err);
            })
            .finally(function() {
                state.loading = false;
                updateLoadingState();
                if (elements.loadMoreBtn) {
                    elements.loadMoreBtn.disabled = false;
                    elements.loadMoreBtn.textContent = 'Load more';
                }
            });
    }

    // ============================================
    // Client-Side Filtering
    // ============================================

    function eventMatchesFilters(event) {
        var typeInfo = EVENT_TYPES[event.type] || EVENT_TYPES.system;
        var category = typeInfo.category || 'system';

        // Category filter
        if (!state.filters.categories[category]) return false;

        // Person filter
        if (state.filters.person && event.person !== state.filters.person) return false;

        // Zone filter
        if (state.filters.zone && event.zone !== state.filters.zone) return false;

        // Text search — fuzzy match on computed description
        if (state.filters.searchQuery) {
            var desc = buildEventDescription(event, typeInfo).toLowerCase();
            var query = state.filters.searchQuery.toLowerCase();
            if (!fuzzyMatch(desc, query)) return false;
        }

        return true;
    }

    /**
     * Simple fuzzy match: returns true if all characters in query appear
     * in order within text, or text contains query as a substring.
     * Uses substring match for short queries (< 4 chars) and sequential
     * character matching for longer queries.
     */
    function fuzzyMatch(text, query) {
        if (!query) return true;
        if (text.indexOf(query) !== -1) return true;
        if (query.length < 4) return false;

        // Sequential character fuzzy match
        var qi = 0;
        for (var ti = 0; ti < text.length && qi < query.length; ti++) {
            if (text[ti] === query[qi]) qi++;
        }
        return qi === query.length;
    }

    function applyClientFilters() {
        state.events = state.allLoadedEvents.filter(eventMatchesFilters);
        renderEvents();
    }

    // ============================================
    // Active Filters Display
    // ============================================
    function updateActiveFilters() {
        if (!elements.activeFilters || !elements.activeFilterTags) return;

        var tags = [];

        // Disabled categories
        var allCats = ['presence', 'zones', 'alerts', 'system', 'learning'];
        var catLabels = { presence: 'Presence', zones: 'Zones', alerts: 'Alerts', system: 'System', learning: 'Learning' };
        allCats.forEach(function(cat) {
            if (!state.filters.categories[cat]) {
                tags.push({ label: catLabels[cat] + ' hidden', key: 'cat:' + cat });
            }
        });

        // Person
        if (state.filters.person) {
            tags.push({ label: 'Person: ' + state.filters.person, key: 'person' });
        }

        // Zone
        if (state.filters.zone) {
            tags.push({ label: 'Zone: ' + state.filters.zone, key: 'zone' });
        }

        // Date range
        var dateLabels = { today: 'Today', '7days': 'Last 7 days', '30days': 'Last 30 days', custom: 'Custom range' };
        if (state.filters.dateRange !== 'all') {
            var label = dateLabels[state.filters.dateRange] || state.filters.dateRange;
            if (state.filters.dateRange === 'custom' && state.filters.customFrom) {
                label = state.filters.customFrom + ' to ' + (state.filters.customTo || 'now');
            }
            tags.push({ label: label, key: 'date' });
        }

        // Search
        if (state.filters.searchQuery) {
            tags.push({ label: 'Search: ' + state.filters.searchQuery, key: 'search' });
        }

        if (tags.length === 0) {
            elements.activeFilters.style.display = 'none';
            return;
        }

        elements.activeFilters.style.display = 'flex';
        elements.activeFilterTags.innerHTML = tags.map(function(tag) {
            return '<span class="sidebar-filter-tag" data-key="' + escapeHtml(tag.key) + '">' +
                escapeHtml(tag.label) +
                '<button class="sidebar-filter-tag-remove" title="Remove filter">×</button>' +
                '</span>';
        }).join('');

        // Bind remove buttons
        elements.activeFilterTags.querySelectorAll('.sidebar-filter-tag-remove').forEach(function(btn) {
            btn.addEventListener('click', function(e) {
                e.stopPropagation();
                var key = btn.closest('.sidebar-filter-tag').dataset.key;
                removeFilter(key);
            });
        });
    }

    function removeFilter(key) {
        if (key === 'person') {
            state.filters.person = '';
            if (elements.personSelect) elements.personSelect.value = '';
            applyClientFilters();
        } else if (key === 'zone') {
            state.filters.zone = '';
            if (elements.zoneSelect) elements.zoneSelect.value = '';
            applyClientFilters();
        } else if (key === 'search') {
            state.filters.searchQuery = '';
            if (elements.searchInput) elements.searchInput.value = '';
            applyClientFilters();
        } else if (key === 'date') {
            state.filters.dateRange = 'all';
            state.filters.customFrom = '';
            state.filters.customTo = '';
            state.serverSince = '';
            state.serverUntil = '';
            if (elements.dateRangeSelect) elements.dateRangeSelect.value = 'all';
            if (elements.customDateContainer) elements.customDateContainer.style.display = 'none';
            resetAndReload();
        } else if (key.indexOf('cat:') === 0) {
            var cat = key.substring(4);
            state.filters.categories[cat] = true;
            var catEl = document.getElementById('filter-category-' + cat);
            if (catEl) catEl.checked = true;
            applyClientFilters();
        }
        updateActiveFilters();
    }

    function clearAllFilters() {
        state.filters.categories = { presence: true, zones: true, alerts: true, system: false, learning: false };
        state.filters.person = '';
        state.filters.zone = '';
        state.filters.dateRange = 'all';
        state.filters.customFrom = '';
        state.filters.customTo = '';
        state.filters.searchQuery = '';
        state.serverSince = '';
        state.serverUntil = '';

        // Reset UI
        if (elements.categoryPresence) elements.categoryPresence.checked = true;
        if (elements.categoryZones) elements.categoryZones.checked = true;
        if (elements.categoryAlerts) elements.categoryAlerts.checked = true;
        if (elements.categorySystem) elements.categorySystem.checked = false;
        if (elements.categoryLearning) elements.categoryLearning.checked = false;
        if (elements.personSelect) elements.personSelect.value = '';
        if (elements.zoneSelect) elements.zoneSelect.value = '';
        if (elements.dateRangeSelect) elements.dateRangeSelect.value = 'all';
        if (elements.searchInput) elements.searchInput.value = '';
        if (elements.customDateContainer) elements.customDateContainer.style.display = 'none';

        updateActiveFilters();
        resetAndReload();
    }

    // ============================================
    // Virtualization Setup
    // ============================================
    function setupVirtualization() {
        if (!CONFIG.virtualization.enabled || !elements.content) return;

        var observerOptions = {
            root: elements.content,
            rootMargin: CONFIG.virtualization.rootMargin,
            threshold: CONFIG.virtualization.threshold
        };

        state.virtualization.observer = new IntersectionObserver(function(entries) {
            handleIntersection(entries);
        }, observerOptions);
    }

    function handleIntersection(entries) {
        entries.forEach(function(entry) {
            var index = parseInt(entry.target.dataset.index, 10);
            if (isNaN(index)) return;
            if (entry.isIntersecting) {
                state.virtualization.visibleIndices.add(index);
            } else {
                state.virtualization.visibleIndices.delete(index);
            }
        });
        updateRenderedRange();
    }

    function onScroll() {
        if (!elements.content) return;

        state.virtualization.scrollTop = elements.content.scrollTop;

        var firstIndex = Math.floor(state.virtualization.scrollTop / CONFIG.itemHeight);
        var visibleCount = Math.ceil(elements.content.clientHeight / CONFIG.itemHeight);
        var lastIndex = firstIndex + visibleCount;

        state.virtualization.firstVisibleIndex = Math.max(0, firstIndex - CONFIG.virtualization.bufferSize);
        state.virtualization.lastVisibleIndex = Math.min(state.events.length - 1, lastIndex + CONFIG.virtualization.bufferSize);

        updateRenderedRange();
    }

    function updateRenderedRange() {
        if (!state.events.length) return;

        var firstIdx = Math.max(0, state.virtualization.firstVisibleIndex);
        var lastIdx = Math.min(state.events.length - 1, state.virtualization.lastVisibleIndex);

        var newRenderedIndices = new Set();
        for (var i = firstIdx; i <= lastIdx; i++) {
            newRenderedIndices.add(i);
        }

        var fragment = document.createDocumentFragment();
        newRenderedIndices.forEach(function(index) {
            if (!state.virtualization.renderedIndices.has(index)) {
                var event = state.events[index];
                if (event) {
                    var tempDiv = document.createElement('div');
                    tempDiv.innerHTML = renderEvent(event, false, index);
                    var newEventEl = tempDiv.firstElementChild;
                    if (newEventEl) {
                        newEventEl.dataset.index = index;
                        fragment.appendChild(newEventEl);
                    }
                }
            }
        });

        if (fragment.children.length > 0) {
            elements.eventsContainer.appendChild(fragment);
            Array.from(fragment.children).forEach(function(item) {
                bindEventHandlers(item);
                if (state.virtualization.observer) {
                    state.virtualization.observer.observe(item);
                }
            });
        }

        state.virtualization.renderedIndices.forEach(function(index) {
            if (!newRenderedIndices.has(index)) {
                var item = elements.eventsContainer.querySelector('[data-index="' + index + '"]');
                if (item) {
                    if (state.virtualization.observer) {
                        state.virtualization.observer.unobserve(item);
                    }
                    item.remove();
                }
            }
        });

        state.virtualization.renderedIndices = newRenderedIndices;
        updateSpacers();
    }

    function updateSpacers() {
        if (!elements.spacerTop || !elements.spacerBottom) return;
        var topHeight = state.virtualization.firstVisibleIndex * CONFIG.itemHeight;
        var bottomHeight = (state.events.length - state.virtualization.lastVisibleIndex - 1) * CONFIG.itemHeight;
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
            if (elements.empty) elements.empty.style.display = 'flex';
            if (elements.loading) elements.loading.style.display = 'none';
            updateCountDisplay();
            return;
        }

        if (elements.empty) elements.empty.style.display = 'none';
        if (elements.loading) elements.loading.style.display = 'none';

        // Reset virtualization state on re-render
        state.virtualization.firstVisibleIndex = 0;
        state.virtualization.lastVisibleIndex = 0;
        state.virtualization.renderedIndices = new Set();

        if (CONFIG.virtualization.enabled && state.events.length > CONFIG.maxDOMItems) {
            renderVirtualized();
        } else {
            renderAll();
        }

        updateCountDisplay();
    }

    function renderAll() {
        var html = '';
        state.events.forEach(function(event, index) {
            html += renderEvent(event, false, index);
        });
        elements.eventsContainer.innerHTML = html;

        Array.from(elements.eventsContainer.children).forEach(function(item) {
            bindEventHandlers(item);
        });
    }

    function renderVirtualized() {
        elements.eventsContainer.innerHTML = '';

        var containerHeight = elements.content.clientHeight || 400;
        var visibleCount = Math.ceil(containerHeight / CONFIG.itemHeight);
        var bufferCount = CONFIG.virtualization.bufferSize;

        state.virtualization.firstVisibleIndex = 0;
        state.virtualization.lastVisibleIndex = Math.min(state.events.length - 1, visibleCount + bufferCount * 2);

        updateSpacers();
        updateRenderedRange();
    }

    function renderEvent(event, isNew, index) {
        var typeInfo = EVENT_TYPES[event.type] || EVENT_TYPES.system;
        var timeStr = formatTimestamp(event.timestamp_ms);
        var description = buildEventDescription(event, typeInfo);

        var severityClass = (event.severity === 'alert' || event.severity === 'critical') ? ' severity-critical' :
                            (event.severity === 'warning' ? ' severity-warning' : '');
        var newClass = isNew ? ' new-event' : '';

        var systemEventTypes = ['node_online', 'node_offline', 'ota_update', 'baseline_changed', 'system', 'learning_milestone', 'anomaly_learned'];
        var isSystemEvent = systemEventTypes.indexOf(event.type) !== -1;
        var secondaryClass = isSystemEvent ? ' secondary' : '';

        var dataIndex = index !== undefined ? (' data-index="' + index + '"') : '';

        return '<div class="sidebar-timeline-event' + severityClass + newClass + secondaryClass + '"' +
            ' data-type="' + escapeHtml(event.type) + '"' +
            ' data-id="' + event.id + '"' +
            ' data-timestamp="' + event.timestamp_ms + '"' + dataIndex + '>' +
            '<div class="sidebar-timeline-event-icon">' + typeInfo.icon + '</div>' +
            '<div class="sidebar-timeline-event-content">' +
            '<div class="sidebar-timeline-event-title">' + escapeHtml(description) + '</div>' +
            '<div class="sidebar-timeline-event-meta">' +
            '<span class="sidebar-timeline-event-time">' + timeStr + '</span>' +
            '</div>' +
            '</div>' +
            '<div class="sidebar-timeline-event-actions">' +
            '<button class="sidebar-timeline-action-btn feedback-positive"' +
            ' data-action="correct" title="Correct detection" aria-label="Thumbs up">👍</button>' +
            '<button class="sidebar-timeline-action-btn feedback-negative"' +
            ' data-action="incorrect" title="Incorrect detection" aria-label="Thumbs down">👎</button>' +
            '</div>' +
            '</div>';
    }

    function renderLoadMore() {
        if (!elements.loadMoreContainer) return;
        elements.loadMoreContainer.style.display = state.hasMore ? 'flex' : 'none';
        if (elements.loadMoreBtn) {
            elements.loadMoreBtn.disabled = state.loading;
            elements.loadMoreBtn.textContent = state.loading ? 'Loading...' : 'Load more';
        }
    }

    function updateCountDisplay() {
        if (!elements.count) return;
        var total = state.allLoadedEvents.length;
        var showing = state.events.length;

        if (total === 0) {
            elements.count.style.display = 'none';
            return;
        }

        elements.count.style.display = 'block';
        if (showing < total) {
            elements.count.textContent = 'Showing ' + showing + ' of ' + total + ' loaded events';
        } else {
            elements.count.textContent = total + (state.hasMore ? '+' : '') + ' events';
        }
    }

    function buildEventDescription(event, typeInfo) {
        var detail = {};
        if (event.detail_json) {
            try { detail = JSON.parse(event.detail_json); } catch (e) {}
        }

        var description = typeInfo.description;
        var replacements = {
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

        for (var placeholder in replacements) {
            description = description.replace(placeholder, replacements[placeholder]);
        }

        return description;
    }

    function formatTimestamp(ms) {
        var date = new Date(ms);
        var now = new Date();
        var isToday = date.toDateString() === now.toDateString();
        var time = date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
        return isToday ? time : (date.toLocaleDateString() + ' ' + time);
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
        eventEl.querySelectorAll('.sidebar-timeline-action-btn').forEach(function(btn) {
            btn.addEventListener('click', function(e) {
                e.stopPropagation();
                var action = btn.dataset.action;
                var eventId = eventEl.dataset.id;
                handleFeedback(eventId, action, eventEl);
            });
        });

        eventEl.addEventListener('click', function() {
            var timestamp = parseInt(this.dataset.timestamp, 10);
            handleSeek(timestamp, this);
        });
    }

    function handleFeedback(eventId, action, eventElement) {
        var correct = action === 'correct';

        if (window.Feedback) {
            var event = state.allLoadedEvents.find(function(e) { return String(e.id) === String(eventId); });
            if (event) {
                var detail = {};
                try { detail = event.detail_json ? JSON.parse(event.detail_json) : {}; } catch (e) {}

                Feedback.sendFeedback(eventId, event.type, correct ? 'TRUE_POSITIVE' : 'FALSE_POSITIVE', detail);

                eventElement.querySelectorAll('.sidebar-timeline-action-btn').forEach(function(btn) {
                    if (btn.dataset.action === action) {
                        btn.classList.add('active');
                        setTimeout(function() { btn.classList.remove('active'); }, 2000);
                    }
                });

                if (window.SpaxelApp && SpaxelApp.showToast) {
                    SpaxelApp.showToast(correct ? 'Thanks for the feedback!' : 'Thanks — I\'ll adjust my detection.', 'success');
                }

                if (!correct) {
                    eventElement.classList.add('feedback-dismissed');
                }
            }
        } else {
            fetch('/api/feedback', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ type: correct ? 'correct' : 'incorrect', event_id: parseInt(eventId, 10) })
            })
                .then(function(res) { if (!res.ok) throw new Error('Feedback failed'); })
                .then(function() {
                    eventElement.querySelectorAll('.sidebar-timeline-action-btn').forEach(function(btn) {
                        if (btn.dataset.action === action) {
                            btn.classList.add('active');
                            setTimeout(function() { btn.classList.remove('active'); }, 2000);
                        }
                    });
                    if (window.SpaxelApp && SpaxelApp.showToast) {
                        SpaxelApp.showToast(correct ? 'Thanks for the feedback!' : 'Thanks — I\'ll adjust my detection.', 'success');
                    }
                    if (!correct) eventElement.classList.add('feedback-dismissed');
                })
                .catch(function(err) { console.error('[SidebarTimeline] Feedback failed:', err); });
        }
    }

    function handleSeek(timestamp, eventEl) {
        if (!timestamp || timestamp <= 0) return;

        clearSelectedEvent();
        if (eventEl) {
            state.selectedEventId = eventEl.dataset.id;
            eventEl.classList.add('selected');
        }

        if (window.SpaxelReplay) {
            SpaxelReplay.jumpToTime(timestamp).then(function() {
                updateNowReplayingChip(true, timestamp);
                if (window.SpaxelTimeline && SpaxelTimeline.clearSelection) {
                    SpaxelTimeline.clearSelection();
                }
            }).catch(function(err) {
                console.error('[SidebarTimeline] Jump-to-time failed:', err);
                if (window.SpaxelApp && SpaxelApp.showToast) {
                    SpaxelApp.showToast('Failed to jump to time', 'error');
                }
            });
        } else if (window.SpaxelRouter) {
            SpaxelRouter.navigate('timeline');
        }
    }

    function clearSelectedEvent() {
        if (state.selectedEventId) {
            var prev = elements.eventsContainer
                ? elements.eventsContainer.querySelector('.sidebar-timeline-event.selected')
                : null;
            if (prev) prev.classList.remove('selected');
            state.selectedEventId = null;
        }
    }

    function updateNowReplayingChip(visible, timestampMs) {
        var chip = document.getElementById('now-replaying-chip');
        if (!chip) {
            var header = document.querySelector('.sidebar-panel-header');
            if (!header) return;
            chip = document.createElement('span');
            chip.id = 'now-replaying-chip';
            chip.className = 'now-replaying-chip';
            header.appendChild(chip);
        }
        if (visible && timestampMs) {
            var date = new Date(timestampMs);
            var time = date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
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
        var normalizedEvent = {
            id: event.id || event.timestamp_ms || Date.now(),
            timestamp_ms: event.timestamp_ms || event.ts || Date.now(),
            type: event.type || event.kind || 'system',
            zone: event.zone || '',
            person: event.person || event.person_name || '',
            blob_id: event.blob_id || event.blobID || 0,
            detail_json: event.detail_json || '',
            severity: event.severity || 'info'
        };

        // Only add if within the current server-side date range
        if (state.serverSince) {
            var sinceMs = new Date(state.serverSince).getTime();
            if (normalizedEvent.timestamp_ms < sinceMs) return;
        }
        if (state.serverUntil) {
            var untilMs = new Date(state.serverUntil).getTime();
            if (normalizedEvent.timestamp_ms > untilMs) return;
        }

        state.allLoadedEvents.unshift(normalizedEvent);

        if (state.allLoadedEvents.length > CONFIG.maxEventsInMemory) {
            state.allLoadedEvents = state.allLoadedEvents.slice(0, CONFIG.maxEventsInMemory);
        }

        if (state.panelVisible && eventMatchesFilters(normalizedEvent)) {
            state.events.unshift(normalizedEvent);
            if (state.events.length > CONFIG.maxEventsInMemory) {
                state.events = state.events.slice(0, CONFIG.maxEventsInMemory);
            }
            renderEvents();
            updateCountDisplay();
        }
    }

    // ============================================
    // UI Updates
    // ============================================
    function updateLoadingState() {
        if (!elements.loading) return;
        elements.loading.style.display = state.loading ? 'flex' : 'none';
    }

    // ============================================
    // Public API
    // ============================================
    var SidebarTimeline = {
        init: init,
        show: showPanel,
        hide: hidePanel,
        toggle: togglePanel,
        refresh: resetAndReload,
        isVisible: function() { return state.panelVisible; },
        clearSelection: clearSelectedEvent,
        hideNowReplayingChip: function() { updateNowReplayingChip(false); }
    };

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }

    window.SpaxelSidebarTimeline = SidebarTimeline;
})();
