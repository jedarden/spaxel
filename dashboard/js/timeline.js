/**
 * Spaxel Dashboard - Activity Timeline
 *
 * Scrollable chronological event list with filtering and event interaction.
 * Click event → jump to that moment in replay mode.
 * Inline feedback (thumbs up/down) on presence detection events.
 * Virtualized rendering with IntersectionObserver for 1000+ events.
 */
(function() {
	'use strict';

	// ============================================
	// Configuration
	// ============================================
	const CONFIG = {
		initialLoadLimit: 200,
		fetchSinceHours: 24,
		debounceMs: 300,
		replaySeekWindowSec: 5, // seconds before/after event timestamp
		virtualization: {
			enabled: true,
			bufferSize: 50, // number of extra items to render above/below viewport
			rootMargin: '400px', // load items 400px before they enter viewport
			threshold: 0.01 // trigger when 1% of item is visible
		},
		itemHeight: 80, // estimated height of a timeline event item in pixels
		maxDOMItems: 150 // maximum number of items to keep in DOM at once
	};

	// ============================================
	// Event Type Categories
	// ============================================
	const EVENT_CATEGORIES = {
		presence: ['presence_transition', 'stationary_detected', 'detection'],
		zones: ['zone_entry', 'zone_exit', 'ZoneTransition', 'portal_crossing', 'sleep_session_end'],
		alerts: ['fall_alert', 'FallDetected', 'anomaly', 'AnomalyDetected', 'security_alert'],
		system: ['node_online', 'node_offline', 'ota_update', 'baseline_changed', 'system'],
		learning: ['learning_milestone', 'anomaly_learned']
	};

	// ============================================
	// State
	// ============================================
	const state = {
		events: [],
		cursor: null,
		total: 0,
		dashboardMode: 'expert', // 'expert' or 'simple' - determines timeline mode
		filters: {
			categories: {
				presence: true,
				zones: true,
				alerts: true,
				system: true,
				learning: true
			},
			type: null, // specific type filter (overrides categories)
			zone: null,
			person: null,
			after: null, // ISO8601 string
			until: null, // ISO8601 string
			q: null // text search query
		},
		loading: false,
		error: null,
		// Filter options populated from available events
		availableTypes: new Set(),
		availableZones: new Set(),
		availablePersons: new Set(),
		// Virtualization state
		virtualization: {
			observer: null,
			visibleIndices: new Set(),
			renderedIndices: new Set(),
			firstVisibleIndex: 0,
			lastVisibleIndex: 0,
			containerHeight: 0,
			scrollTop: 0,
			totalHeight: 0
		},
		// Client-side filtered events
		filteredEvents: [],
		// All loaded events (for client-side filtering)
		allLoadedEvents: []
	};

	// DOM elements
	let elements = {};

	// ============================================
	// Event Type Icons and Colors
	// ============================================
	const eventTypeInfo = {
		zone_entry: {
			icon: '🚪',
			color: '#66bb6a',
			label: 'Entered',
			description: 'Person entered a zone',
			category: 'zones'
		},
		zone_exit: {
			icon: '🚶',
			color: '#ffa726',
			label: 'Left',
			description: 'Person exited a zone',
			category: 'zones'
		},
		ZoneTransition: {
			icon: '🚶',
			color: '#ffa726',
			label: 'Moved',
			description: 'Person moved between zones',
			category: 'zones'
		},
		portal_crossing: {
			icon: '→',
			color: '#42a5f5',
			label: 'Crossed',
			description: 'Person crossed a portal',
			category: 'zones'
		},
		sleep_session_end: {
			icon: '🌅',
			color: '#4fc3f7',
			label: 'Woke Up',
			description: 'Sleep session ended',
			category: 'zones'
		},
		presence_transition: {
			icon: '👤',
			color: '#ab47bc',
			label: 'Detected',
			description: 'Presence detected',
			category: 'presence'
		},
		stationary_detected: {
			icon: '💤',
			color: '#7e57c2',
			label: 'Stationary',
			description: 'Stationary person detected',
			category: 'presence'
		},
		detection: {
			icon: '👁️',
			color: '#ab47bc',
			label: 'Detected',
			description: 'Motion detected',
			category: 'presence'
		},
		fall_alert: {
			icon: '🆘',
			color: '#f44336',
			label: 'Fall',
			description: 'Fall detected',
			category: 'alerts'
		},
		FallDetected: {
			icon: '🆘',
			color: '#f44336',
			label: 'Fall',
			description: 'Fall detected',
			category: 'alerts'
		},
		anomaly: {
			icon: '⚠️',
			color: '#ef5350',
			label: 'Anomaly',
			description: 'Unusual activity detected',
			category: 'alerts'
		},
		AnomalyDetected: {
			icon: '⚠️',
			color: '#ef5350',
			label: 'Anomaly',
			description: 'Unusual activity detected',
			category: 'alerts'
		},
		security_alert: {
			icon: '🚨',
			color: '#d32f2f',
			label: 'Security',
			description: 'Security alert',
			category: 'alerts'
		},
		node_online: {
			icon: '📡',
			color: '#4caf50',
			label: 'Online',
			description: 'Node came online',
			category: 'system'
		},
		node_offline: {
			icon: '📵',
			color: '#9e9e9e',
			label: 'Offline',
			description: 'Node went offline',
			category: 'system'
		},
		ota_update: {
			icon: '⬆️',
			color: '#2196f3',
			label: 'Updated',
			description: 'Firmware updated',
			category: 'system'
		},
		baseline_changed: {
			icon: '📊',
			color: '#00bcd4',
			label: 'Baseline',
			description: 'Baseline updated',
			category: 'system'
		},
		system: {
			icon: '⚙️',
			color: '#607d8b',
			label: 'System',
			description: 'System event',
			category: 'system'
		},
		learning_milestone: {
			icon: '🎓',
			color: '#9c27b0',
			label: 'Learned',
			description: 'System learned patterns',
			category: 'learning'
		},
		anomaly_learned: {
			icon: '🧠',
			color: '#9c27b0',
			label: 'Learned',
			description: 'Anomaly pattern learned',
			category: 'learning'
		}
	};

	// ============================================
	// Initialization
	// ============================================
	function init() {
		console.log('[Timeline] Initializing');

		cacheElements();
		bindEvents();
		setupVirtualization();

		// Listen for route changes to show/hide timeline
		if (window.SpaxelRouter) {
			SpaxelRouter.onModeChange(onModeChange);
		}

		// Listen for simple mode changes
		if (window.SpaxelSimpleModeDetection) {
			SpaxelSimpleModeDetection.onModeChange(onSimpleModeChange);
		}

		// Listen for WebSocket event messages
		if (window.SpaxelApp) {
			SpaxelApp.registerMessageHandler(handleWebSocketMessage);
		}
	}

	// ============================================
	// Simple Mode Change Handler
	// ============================================
	function onSimpleModeChange(newMode, oldMode) {
		console.log('[Timeline] Simple mode changed from', oldMode, 'to', newMode);

		// Update dashboard mode based on simple mode
		if (newMode === 'simple') {
			state.dashboardMode = 'simple';
		} else {
			state.dashboardMode = 'expert';
		}

		// Reload events if timeline is visible
		if (elements.container && elements.container.style.display !== 'none') {
			loadInitialEvents();
		}
	}

	// ============================================
	// Virtualization Setup
	// ============================================
	function setupVirtualization() {
		if (!CONFIG.virtualization.enabled || !elements.eventsList) {
			return;
		}

		// Create IntersectionObserver for lazy rendering
		const observerOptions = {
			root: elements.eventsList,
			rootMargin: CONFIG.virtualization.rootMargin,
			threshold: CONFIG.virtualization.threshold
		};

		state.virtualization.observer = new IntersectionObserver(function(entries) {
			handleIntersection(entries);
		}, observerOptions);

		// Set up scroll listener for virtualization
		if (elements.eventsList) {
			elements.eventsList.addEventListener('scroll', onScroll, { passive: true });
		}

		console.log('[Timeline] Virtualization enabled with IntersectionObserver');
	}

	// ============================================
	// Intersection Observer Handler
	// ============================================
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

	// ============================================
	// Scroll Handler for Virtualization
	// ============================================
	function onScroll() {
		if (!elements.eventsList) return;

		state.virtualization.scrollTop = elements.eventsList.scrollTop;

		// Update visible range based on scroll position
		const firstIndex = Math.floor(state.virtualization.scrollTop / CONFIG.itemHeight);
		const visibleCount = Math.ceil(elements.eventsList.clientHeight / CONFIG.itemHeight);
		const lastIndex = firstIndex + visibleCount;

		state.virtualization.firstVisibleIndex = Math.max(0, firstIndex - CONFIG.virtualization.bufferSize);
		state.virtualization.lastVisibleIndex = Math.min(state.filteredEvents.length - 1, lastIndex + CONFIG.virtualization.bufferSize);

		updateRenderedRange();
	}

	// ============================================
	// Update Rendered Range
	// ============================================
	function updateRenderedRange() {
		if (!state.filteredEvents.length) return;

		const firstIdx = Math.max(0, state.virtualization.firstVisibleIndex);
		const lastIdx = Math.min(state.filteredEvents.length - 1, state.virtualization.lastVisibleIndex);

		// Unobserve items that are no longer in range
		state.virtualization.renderedIndices.forEach(function(index) {
			if (index < firstIdx || index > lastIdx) {
				const item = elements.eventsList.querySelector('[data-index="' + index + '"]');
				if (item && state.virtualization.observer) {
					state.virtualization.observer.unobserve(item);
				}
			}
		});

		// Create new set of rendered indices
		const newRenderedIndices = new Set();
		for (let i = firstIdx; i <= lastIdx; i++) {
			newRenderedIndices.add(i);
		}

		// Render new items and observe them
		const fragment = document.createDocumentFragment();
		newRenderedIndices.forEach(function(index) {
			if (!state.virtualization.renderedIndices.has(index)) {
				const event = state.filteredEvents[index];
				if (event) {
					const tempDiv = document.createElement('div');
					tempDiv.innerHTML = renderEvent(event, false, index);
					const newEventEl = tempDiv.firstElementChild;
					if (newEventEl) {
						newEventEl.dataset.index = index;
						fragment.appendChild(newEventEl);
					}
				}
			} else {
				// Keep existing item in rendered set
				newRenderedIndices.add(index);
			}
		});

		if (fragment.children.length > 0) {
			elements.eventsList.appendChild(fragment);

			// Bind handlers for new items
			Array.from(fragment.children).forEach(function(item) {
				bindEventHandlersForElement(item);
				if (state.virtualization.observer) {
					state.virtualization.observer.observe(item);
				}
			});
		}

		// Remove items that are no longer in rendered range
		state.virtualization.renderedIndices.forEach(function(index) {
			if (!newRenderedIndices.has(index)) {
				const item = elements.eventsList.querySelector('[data-index="' + index + '"]');
				if (item) {
					item.remove();
				}
			}
		});

		state.virtualization.renderedIndices = newRenderedIndices;

		// Update total height for spacer
		updateVirtualSpacers();
	}

	// ============================================
	// Update Virtual Spacers
	// ============================================
	function updateVirtualSpacers() {
		if (!elements.eventsList) return;

		const totalHeight = state.filteredEvents.length * CONFIG.itemHeight;
		state.virtualization.totalHeight = totalHeight;

		// Add top spacer if needed
		let topSpacer = elements.eventsList.querySelector('.timeline-spacer-top');
		if (!topSpacer) {
			topSpacer = document.createElement('div');
			topSpacer.className = 'timeline-spacer timeline-spacer-top';
			elements.eventsList.insertBefore(topSpacer, elements.eventsList.firstChild);
		}
		topSpacer.style.height = (state.virtualization.firstVisibleIndex * CONFIG.itemHeight) + 'px';

		// Add bottom spacer if needed
		let bottomSpacer = elements.eventsList.querySelector('.timeline-spacer-bottom');
		if (!bottomSpacer) {
			bottomSpacer = document.createElement('div');
			bottomSpacer.className = 'timeline-spacer timeline-spacer-bottom';
			elements.eventsList.appendChild(bottomSpacer);
		}
		const remainingHeight = (state.filteredEvents.length - state.virtualization.lastVisibleIndex - 1) * CONFIG.itemHeight;
		bottomSpacer.style.height = Math.max(0, remainingHeight) + 'px';
	}

	function cacheElements() {
		elements = {
			container: document.getElementById('timeline-view'),
			eventsList: document.getElementById('timeline-events'),
			filterType: document.getElementById('timeline-filter-type'),
			filterZone: document.getElementById('timeline-filter-zone'),
			filterPerson: document.getElementById('timeline-filter-person'),
			filterTime: document.getElementById('timeline-filter-time'),
			filterSearch: document.getElementById('timeline-filter-search'),
			loading: document.getElementById('timeline-loading'),
			empty: document.getElementById('timeline-empty'),
			error: document.getElementById('timeline-error'),
			loadMore: document.getElementById('timeline-load-more'),
			loadMoreBtn: document.getElementById('timeline-load-more-btn'),
			// Category checkboxes
			categoryPresence: document.getElementById('timeline-category-presence'),
			categoryZones: document.getElementById('timeline-category-zones'),
			categoryAlerts: document.getElementById('timeline-category-alerts'),
			categorySystem: document.getElementById('timeline-category-system'),
			categoryLearning: document.getElementById('timeline-category-learning'),
			// Custom date range inputs
			customDateContainer: document.getElementById('timeline-custom-date-container'),
			dateFrom: document.getElementById('timeline-date-from'),
			dateTo: document.getElementById('timeline-date-to'),
			dateApply: document.getElementById('timeline-date-apply'),
			// Filter bar toggle
			filterToggle: document.getElementById('timeline-filter-toggle'),
			filterBar: document.getElementById('timeline-filter-bar')
		};
	}

	function bindEvents() {
		// Category checkboxes
		const categoryInputs = [
			{ el: elements.categoryPresence, key: 'presence' },
			{ el: elements.categoryZones, key: 'zones' },
			{ el: elements.categoryAlerts, key: 'alerts' },
			{ el: elements.categorySystem, key: 'system' },
			{ el: elements.categoryLearning, key: 'learning' }
		];
		categoryInputs.forEach(function(item) {
			if (item.el) {
				item.el.addEventListener('change', function() {
					state.filters.categories[item.key] = item.el.checked;
					applyClientSideFilters();
				});
			}
		});

		// Filter dropdowns
		if (elements.filterType) {
			elements.filterType.addEventListener('change', onFilterChange);
		}
		if (elements.filterZone) {
			elements.filterZone.addEventListener('change', onFilterChange);
		}
		if (elements.filterPerson) {
			elements.filterPerson.addEventListener('change', onFilterChange);
		}
		if (elements.filterTime) {
			elements.filterTime.addEventListener('change', onTimeFilterChange);
		}
		if (elements.filterSearch) {
			let searchTimeout;
			elements.filterSearch.addEventListener('input', function() {
				clearTimeout(searchTimeout);
				searchTimeout = setTimeout(onSearchChange, CONFIG.debounceMs);
			});
		}

		// Custom date range
		if (elements.dateApply) {
			elements.dateApply.addEventListener('click', applyCustomDateRange);
		}

		// Load more button
		if (elements.loadMoreBtn) {
			elements.loadMoreBtn.addEventListener('click', loadMoreEvents);
		}

		// Filter bar toggle
		if (elements.filterToggle) {
			elements.filterToggle.addEventListener('click', function() {
				elements.filterBar.classList.toggle('collapsed');
			});
		}
	}

	// ============================================
	// Mode Change Handler
	// ============================================
	function onModeChange(newMode, oldMode) {
		const container = elements.container;
		if (!container) return;

		// Determine dashboard mode: expert mode shows all events, simple mode shows person-relevant only
		// Expert mode is the default for live view, simple mode is for simplified dashboard
		if (newMode === 'live' || newMode === 'replay' || newMode === 'timeline') {
			state.dashboardMode = 'expert';
		} else if (newMode === 'simple' || newMode === 'ambient') {
			state.dashboardMode = 'simple';
		} else {
			state.dashboardMode = 'expert'; // Default to expert
		}

		if (newMode === 'timeline') {
			// Container is shown by inline script, just load events if needed
			if (state.allLoadedEvents.length === 0) {
				loadInitialEvents();
			} else {
				// Reload events with new mode
				loadInitialEvents();
			}
		}
	}

	// ============================================
	// Event Loading
	// ============================================
	function loadInitialEvents() {
		const params = new URLSearchParams();
		params.set('limit', CONFIG.initialLoadLimit);

		applyFiltersToParams(params);

		const url = '/api/events?' + params.toString();
		fetchEvents(url, true);
	}

	function loadMoreEvents() {
		if (state.loading || !state.cursor) {
			return;
		}

		const params = new URLSearchParams();
		params.set('limit', CONFIG.initialLoadLimit);
		if (state.cursor) {
			params.set('before', state.cursor);
		}

		applyFiltersToParams(params);

		const url = '/api/events?' + params.toString();
		fetchEvents(url, false);
	}

	function fetchEvents(url, isInitial) {
		state.loading = true;
		updateLoadingState();

		fetch(url)
			.then(function(res) {
				if (!res.ok) {
					throw new Error('Failed to fetch events: ' + res.statusText);
				}
				return res.json();
			})
			.then(function(data) {
				if (isInitial) {
					state.allLoadedEvents = [];
					state.filteredEvents = [];
				}

				// Append to all loaded events
				state.allLoadedEvents = state.allLoadedEvents.concat(data.events || []);
				state.cursor = data.cursor || null;
				state.total = data.total_filtered || 0;

				// Update filter options with new data
				updateFilterOptions(data.events);

				// Apply client-side filters
				applyClientSideFilters();

				updateLoadMoreButton();
			})
			.catch(function(err) {
				console.error('[Timeline] Failed to load events:', err);
				state.error = err.message;
				showError(err.message);
			})
			.finally(function() {
				state.loading = false;
				updateLoadingState();
			});
	}

	// ============================================
	// Filter Handling
	// ============================================
	function onFilterChange() {
		// Update filter state
		if (elements.filterType) {
			const value = elements.filterType.value;
			if (value) {
				// If a specific type is selected, clear category filters
				state.filters.type = value;
				// Disable category checkboxes when specific type selected
				disableCategoryCheckboxes(true);
			} else {
				state.filters.type = null;
				disableCategoryCheckboxes(false);
			}
		}
		if (elements.filterZone) {
			state.filters.zone = elements.filterZone.value || null;
		}
		if (elements.filterPerson) {
			state.filters.person = elements.filterPerson.value || null;
		}

		// Reload events with new server-side filters
		loadInitialEvents();
	}

	function onTimeFilterChange() {
		if (!elements.filterTime) return;

		const value = elements.filterTime.value;

		// Hide/show custom date container
		if (elements.customDateContainer) {
			if (value === 'custom') {
				elements.customDateContainer.style.display = 'flex';
			} else {
				elements.customDateContainer.style.display = 'none';
			}
		}

		if (value === 'today') {
			const today = new Date();
			today.setHours(0, 0, 0, 0);
			state.filters.after = today.toISOString();
			state.filters.until = null;
		} else if (value === '7d') {
			const weekAgo = new Date(Date.now() - 7 * 24 * 60 * 60 * 1000);
			state.filters.after = weekAgo.toISOString();
			state.filters.until = null;
		} else if (value === '30d') {
			const monthAgo = new Date(Date.now() - 30 * 24 * 60 * 60 * 1000);
			state.filters.after = monthAgo.toISOString();
			state.filters.until = null;
		} else if (value === 'custom') {
			// Wait for user to apply custom range
			return;
		} else {
			state.filters.after = null;
			state.filters.until = null;
		}

		// Reload events with new date range
		loadInitialEvents();
	}

	function applyCustomDateRange() {
		if (!elements.dateFrom || !elements.dateTo) return;

		const fromDate = new Date(elements.dateFrom.value);
		const toDate = new Date(elements.dateTo.value);

		if (isNaN(fromDate.getTime()) || isNaN(toDate.getTime())) {
			if (window.SpaxelApp && SpaxelApp.showToast) {
				SpaxelApp.showToast('Invalid date range', 'warning');
			}
			return;
		}

		// Set to start of from day and end of to day
		fromDate.setHours(0, 0, 0, 0);
		toDate.setHours(23, 59, 59, 999);

		state.filters.after = fromDate.toISOString();
		state.filters.until = toDate.toISOString();

		// Reload events with custom date range
		loadInitialEvents();
	}

	function onSearchChange() {
		if (elements.filterSearch) {
			state.filters.q = elements.filterSearch.value.trim() || null;
			applyClientSideFilters();
		}
	}

	function disableCategoryCheckboxes(disabled) {
		const checkboxes = [
			elements.categoryPresence,
			elements.categoryZones,
			elements.categoryAlerts,
			elements.categorySystem,
			elements.categoryLearning
		];
		checkboxes.forEach(function(cb) {
			if (cb) {
				cb.disabled = disabled;
				if (disabled) {
					cb.parentElement.style.opacity = '0.5';
				} else {
					cb.parentElement.style.opacity = '1';
				}
			}
		});
	}

	// ============================================
	// Client-Side Filtering
	// ============================================
	function applyClientSideFilters() {
		// Start with all loaded events
		let filtered = state.allLoadedEvents.slice();

		// Apply category filters
		if (!state.filters.type) {
			const enabledCategories = Object.keys(state.filters.categories).filter(
				function(cat) { return state.filters.categories[cat]; }
			);

			const allowedTypes = new Set();
			enabledCategories.forEach(function(cat) {
				const types = EVENT_CATEGORIES[cat];
				if (types) {
					types.forEach(function(t) { allowedTypes.add(t); });
				}
			});

			if (allowedTypes.size > 0) {
				filtered = filtered.filter(function(event) {
					return allowedTypes.has(event.type);
				});
			}
		} else {
			// Specific type filter
			filtered = filtered.filter(function(event) {
				return event.type === state.filters.type;
			});
		}

		// Apply zone filter
		if (state.filters.zone) {
			filtered = filtered.filter(function(event) {
				return event.zone === state.filters.zone;
			});
		}

		// Apply person filter
		if (state.filters.person) {
			filtered = filtered.filter(function(event) {
				return event.person === state.filters.person;
			});
		}

		// Apply text search with fuzzy matching
		if (state.filters.q) {
			const searchLower = state.filters.q.toLowerCase();
			filtered = filtered.filter(function(event) {
				// Search in type, zone, person, and detail_json
				if (event.type && event.type.toLowerCase().indexOf(searchLower) !== -1) return true;
				if (event.zone && event.zone.toLowerCase().indexOf(searchLower) !== -1) return true;
				if (event.person && event.person.toLowerCase().indexOf(searchLower) !== -1) return true;

				// Parse detail_json for additional search
				if (event.detail_json) {
					try {
						const detail = JSON.parse(event.detail_json);
						const detailStr = JSON.stringify(detail).toLowerCase();
						if (detailStr.indexOf(searchLower) !== -1) return true;

						// Check description field specifically
						if (detail.description && detail.description.toLowerCase().indexOf(searchLower) !== -1) {
							return true;
						}
					} catch (e) {
						// If not JSON, search as string
						if (event.detail_json.toLowerCase().indexOf(searchLower) !== -1) return true;
					}
				}

				return false;
			});
		}

		// Sort by timestamp descending
		filtered.sort(function(a, b) {
			return b.timestamp_ms - a.timestamp_ms;
		});

		state.filteredEvents = filtered;
		renderEvents();
	}

	function applyFiltersToParams(params) {
		// Server-side filters
		if (state.filters.type) {
			params.set('type', state.filters.type);
		}
		if (state.filters.zone) {
			params.set('zone', state.filters.zone);
		}
		if (state.filters.person) {
			params.set('person', state.filters.person);
		}
		if (state.filters.after) {
			params.set('since', state.filters.after);
		}
		if (state.filters.until) {
			params.set('until', state.filters.until);
		}
		if (state.filters.q) {
			params.set('q', state.filters.q);
		}
		// Add mode parameter based on dashboard mode
		params.set('mode', state.dashboardMode);
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
		console.log('[Timeline] New event:', event);

		// Normalize event format (live events use ts/kind/person_name, DB events use timestamp_ms/type/person)
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

		// Add to beginning of all loaded events
		state.allLoadedEvents.unshift(normalizedEvent);
		state.total++;

		// Update filter options
		if (normalizedEvent.type) {
			state.availableTypes.add(normalizedEvent.type);
		}
		if (normalizedEvent.zone) {
			state.availableZones.add(normalizedEvent.zone);
		}
		if (normalizedEvent.person) {
			state.availablePersons.add(normalizedEvent.person);
		}

		// Apply client-side filters to update displayed events
		applyClientSideFilters();

		// Prepend to DOM if timeline is visible and event passes filters
		if (elements.container && elements.container.style.display !== 'none' && elements.eventsList) {
			// Check if event passes current filters
			const passesFilters = state.filteredEvents.length > 0 &&
			                      state.filteredEvents[0].id === normalizedEvent.id;

			if (!passesFilters) return; // Don't show if filtered out

			elements.empty.style.display = 'none';

			const tempDiv = document.createElement('div');
			tempDiv.innerHTML = renderEvent(normalizedEvent, true, 0);
			const newEventEl = tempDiv.firstElementChild;

			elements.eventsList.insertBefore(newEventEl, elements.eventsList.firstChild);

			// Bind handlers for the new event
			bindEventHandlersForElement(newEventEl);

			// Remove animation class after it completes
			setTimeout(function() {
				newEventEl.classList.remove('new-event');
			}, 300);

			// Limit DOM elements (keep only most recent 100 in DOM)
			while (elements.eventsList.children.length > 100) {
				const lastChild = elements.eventsList.lastElementChild;
				if (lastChild && !lastChild.classList.contains('timeline-spacer')) {
					elements.eventsList.removeChild(lastChild);
				} else {
					break;
				}
			}

			updateFilterOptions();
		}
	}

	// Bind handlers for a single event element
	function bindEventHandlersForElement(eventEl) {
		// Feedback buttons
		eventEl.querySelectorAll('.timeline-feedback-btn').forEach(function(btn) {
			btn.addEventListener('click', function(e) {
				e.stopPropagation();
				const action = btn.dataset.action;
				const eventId = eventEl.dataset.id;
				handleFeedback(eventId, action, eventEl);
			});
		});

		// Explainability button
		eventEl.querySelectorAll('.timeline-explain-btn').forEach(function(btn) {
			btn.addEventListener('click', function(e) {
				e.stopPropagation();
				const blobId = btn.dataset.blobId;
				handleExplainability(blobId, eventEl);
			});
		});

		// Seek button
		eventEl.querySelectorAll('.timeline-seek-btn').forEach(function(btn) {
			btn.addEventListener('click', function(e) {
				e.stopPropagation();
				const timestamp = parseInt(eventEl.dataset.timestamp, 10);
				handleSeek(timestamp, eventEl);
			});
		});

		// Entry click (also seeks)
		eventEl.addEventListener('click', function() {
			const timestamp = parseInt(this.dataset.timestamp, 10);
			handleSeek(timestamp, this);
		});
	}

	// ============================================
	// Rendering
	// ============================================
	function renderEvents() {
		if (!elements.eventsList) return;

		if (state.filteredEvents.length === 0) {
			elements.eventsList.innerHTML = '';
			if (elements.empty) {
				elements.empty.style.display = 'block';
			}
			if (elements.error) {
				elements.error.style.display = 'none';
			}
			return;
		}

		elements.empty.style.display = 'none';
		if (elements.error) {
			elements.error.style.display = 'none';
		}

		// Use virtualized rendering if enabled
		if (CONFIG.virtualization.enabled && state.filteredEvents.length > CONFIG.maxDOMItems) {
			renderVirtualizedEvents();
		} else {
			renderAllEvents();
		}
	}

	// ============================================
	// Render All Events (for small datasets)
	// ============================================
	function renderAllEvents() {
		// Build HTML for events
		let html = '';
		state.filteredEvents.forEach(function(event, index) {
			html += renderEvent(event, false, index);
		});

		elements.eventsList.innerHTML = html;

		// Bind click handlers
		bindEventHandlers();
	}

	// ============================================
	// Render Virtualized Events (for large datasets)
	// ============================================
	function renderVirtualizedEvents() {
		// Clear existing content
		elements.eventsList.innerHTML = '';

		// Calculate initial visible range
		const containerHeight = elements.eventsList.clientHeight || 400;
		const visibleCount = Math.ceil(containerHeight / CONFIG.itemHeight);
		const bufferCount = CONFIG.virtualization.bufferSize;

		state.virtualization.firstVisibleIndex = 0;
		state.virtualization.lastVisibleIndex = Math.min(state.filteredEvents.length - 1, visibleCount + bufferCount * 2);

		// Create spacers
		updateVirtualSpacers();

		// Render initial batch
		updateRenderedRange();
	}

	function renderEvent(event, isNew, index) {
		const info = eventTypeInfo[event.type] || eventTypeInfo.system;
		const timeStr = formatTimestamp(event.timestamp_ms);
		const personStr = event.person ? escapeHtml(event.person) : '';
		const zoneStr = event.zone ? escapeHtml(event.zone) : '';
		const description = buildEventDescription(event);

		// Severity indicator for alerts
		const severityClass = event.severity === 'alert' || event.severity === 'critical' ? ' severity-critical' : '';
		const newClass = isNew ? ' new-event' : '';

		// Determine if this is a system event (for secondary styling in expert mode)
		// System events: node_online, node_offline, ota_update, baseline_changed, system, learning_milestone, anomaly_learned
		const systemEventTypes = ['node_online', 'node_offline', 'ota_update', 'baseline_changed', 'system', 'learning_milestone', 'anomaly_learned'];
		const isSystemEvent = systemEventTypes.indexOf(event.type) !== -1;
		const secondaryClass = (state.dashboardMode === 'expert' && isSystemEvent) ? ' secondary' : '';

		// Check if this event has a blob_id for explainability
		const hasBlobId = event.blob_id !== undefined && event.blob_id !== null && event.blob_id !== 0;
		const explainabilityBtn = hasBlobId ? `
			<button class="timeline-explain-btn" data-blob-id="${event.blob_id}" title="Why is this here?">
				<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
					<circle cx="12" cy="12" r="10"></circle>
					<path d="M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3"></path>
					<line x1="12" y1="17" x2="12.01" y2="17"></line>
				</svg>
			</button>
		` : '';

		const dataIndex = index !== undefined ? ` data-index="${index}"` : '';

		return `
			<div class="timeline-event timeline-${event.type}${severityClass}${newClass}${secondaryClass}" data-type="${event.type}" data-id="${event.id}" data-timestamp="${event.timestamp_ms}" data-blob-id="${event.blob_id || ''}"${dataIndex}>
				<div class="timeline-event-icon">${info.icon}</div>
				<div class="timeline-event-content">
					<div class="timeline-event-header">
						<div class="timeline-event-title">${description}</div>
					</div>
					<div class="timeline-event-meta">
						<span class="timeline-event-time">${timeStr}</span>
						${zoneStr ? `<span class="timeline-event-zone">${zoneStr}</span>` : ''}
						${personStr ? `<span class="timeline-event-person">${personStr}</span>` : ''}
					</div>
				</div>
				<div class="timeline-event-actions">
					<button class="timeline-feedback-btn positive" data-action="correct" title="Correct">👍</button>
					<button class="timeline-feedback-btn negative" data-action="incorrect" title="Incorrect">👎</button>
					${explainabilityBtn}
					<button class="timeline-seek-btn" title="Jump to this moment">
						<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
							<polygon points="5 3 19 12 5 21 5 3"></polygon>
						</svg>
					</button>
				</div>
			</div>
		`;
	}

	function buildEventDescription(event) {
		const info = eventTypeInfo[event.type] || eventTypeInfo.system;
		const base = info.description;

		// Parse detail_json for additional context
		let detail = '';
		if (event.detail_json) {
			try {
				const detailObj = JSON.parse(event.detail_json);
				if (detailObj.description) {
					detail = detailObj.description;
				}
			} catch (e) {
				// Ignore parse errors
			}
		}

		return detail || base;
	}

	function formatTimestamp(ms) {
		const date = new Date(ms);
		const now = new Date();
		const isToday = date.toDateString() === now.toDateString();
		const isYesterday = new Date(now);
		isYesterday.setDate(now.getDate() - 1);
		const isYesterdayDate = date.toDateString() === isYesterday.toDateString();

		const time = date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });

		if (isToday) {
			return time + ' today';
		} else if (isYesterdayDate) {
			return time + ' yesterday';
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
	function bindEventHandlers() {
		// Feedback buttons
		elements.eventsList.querySelectorAll('.timeline-feedback-btn').forEach(function(btn) {
			btn.addEventListener('click', function(e) {
				e.stopPropagation();
				const action = btn.dataset.action;
				const entry = btn.closest('.timeline-event');
				const eventId = entry.dataset.id;
				handleFeedback(eventId, action, entry);
			});
		});

		// Explainability buttons
		elements.eventsList.querySelectorAll('.timeline-explain-btn').forEach(function(btn) {
			btn.addEventListener('click', function(e) {
				e.stopPropagation();
				const blobId = btn.dataset.blobId;
				const entry = btn.closest('.timeline-event');
				handleExplainability(blobId, entry);
			});
		});

		// Seek button
		elements.eventsList.querySelectorAll('.timeline-seek-btn').forEach(function(btn) {
			btn.addEventListener('click', function(e) {
				e.stopPropagation();
				const entry = btn.closest('.timeline-event');
				const timestamp = parseInt(entry.dataset.timestamp, 10);
				handleSeek(timestamp, entry);
			});
		});

		// Entry click (also seeks)
		elements.eventsList.querySelectorAll('.timeline-event').forEach(function(entry) {
			entry.addEventListener('click', function() {
				const timestamp = parseInt(this.dataset.timestamp, 10);
				handleSeek(timestamp, this);
			});
		});
	}

	// ============================================
	// Explainability Handler
	// ============================================
	function handleExplainability(blobId, entryElement) {
		console.log('[Timeline] Explainability requested for blob:', blobId);

		// Open explainability overlay
		if (window.Explainability) {
			window.Explainability.explain(blobId);
		} else if (window.Viz3D && window.Viz3D.explainBlob) {
			// Fallback to Viz3D's explainBlob if Explainability module not loaded
			window.Viz3D.explainBlob(blobId);
		} else {
			console.error('[Timeline] Explainability module not available');
			if (window.SpaxelApp && SpaxelApp.showToast) {
				SpaxelApp.showToast('Explainability not available', 'warning');
			}
		}
	}

	// ============================================
	// Feedback Handler
	// ============================================
	function handleFeedback(eventId, action, entryElement) {
		const correct = action === 'correct';

		// POST /api/feedback
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
				const feedbackBtns = entryElement.querySelectorAll('.timeline-feedback-btn');
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

				// Remove entry from display for incorrect feedback
				if (!correct) {
					entryElement.classList.add('feedback-dismissed');
				}
			})
			.catch(function(err) {
				console.error('[Timeline] Feedback failed:', err);
				if (window.SpaxelApp && SpaxelApp.showToast) {
					SpaxelApp.showToast('Failed to submit feedback', 'warning');
				}
			});
	}

	// ============================================
	// Seek Handler (Time-Travel)
	// ============================================
	function handleSeek(timestamp, entryElement) {
		// Convert timestamp to ISO8601
		const targetDate = new Date(timestamp);
		const iso8601 = targetDate.toISOString();

		// Create a replay window around the event timestamp
		const windowMs = CONFIG.replaySeekWindowSec * 1000;
		const fromDate = new Date(timestamp - windowMs);
		const toDate = new Date(timestamp + windowMs);

		// Create replay session
		const startPayload = {
			from_iso8601: fromDate.toISOString(),
			to_iso8601: toDate.toISOString(),
			speed: 1
		};

		fetch('/api/replay/start', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify(startPayload)
		})
			.then(function(res) {
				if (!res.ok) {
					throw new Error('Failed to start replay session');
				}
				return res.json();
			})
			.then(function(data) {
				const sessionId = data.session_id;

				// Seek to the specific timestamp
				return fetch('/api/replay/seek', {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify({
						session_id: sessionId,
						timestamp_iso8601: iso8601
					})
				});
			})
			.then(function(res) {
				if (!res.ok) {
					throw new Error('Failed to seek in replay');
				}
				return res.json();
			})
			.then(function(data) {
				// Navigate to replay mode
				if (window.SpaxelRouter) {
					SpaxelRouter.navigate('replay');
				}

				if (window.SpaxelApp && SpaxelApp.showToast) {
					SpaxelApp.showToast('Replay mode: viewing ' + formatTimestamp(timestamp), 'info');
				}
			})
			.catch(function(err) {
				console.error('[Timeline] Replay seek failed:', err);
				if (window.SpaxelApp && SpaxelApp.showToast) {
					SpaxelApp.showToast('Failed to jump to replay: ' + err.message, 'warning');
				}
			});
	}

	function updateFilterOptions(events) {
		// Extract unique values from events
		const types = new Set();
		const zones = new Set();
		const persons = new Set();

		(events || state.allLoadedEvents).forEach(function(event) {
			if (event.type) types.add(event.type);
			if (event.zone) zones.add(event.zone);
			if (event.person) persons.add(event.person);
		});

		// Update dropdowns
		populateFilterDropdown(elements.filterType, types, 'All Types');
		populateFilterDropdown(elements.filterZone, zones, 'All Zones');
		populateFilterDropdown(elements.filterPerson, persons, 'All People');

		// Store available options
		state.availableTypes = types;
		state.availableZones = zones;
		state.availablePersons = persons;
	}

	function populateFilterDropdown(select, values, placeholder) {
		if (!select) return;

		// Save current selection
		const currentValue = select.value;

		// Clear existing options (except placeholder)
		while (select.options.length > 1) {
			select.remove(1);
		}

		// Add new options
		Array.from(values).sort().forEach(function(value) {
			const option = document.createElement('option');
			option.value = value;
			option.textContent = value.charAt(0).toUpperCase() + value.slice(1).replace(/_/g, ' ');
			select.appendChild(option);
		});

		// Restore selection if it still exists
		if (currentValue && values.has(currentValue)) {
			select.value = currentValue;
		}
	}

	// ============================================
	// UI Updates
	// ============================================
	function updateLoadingState() {
		if (!elements.loading) return;
		elements.loading.style.display = state.loading ? 'flex' : 'none';
	}

	function updateLoadMoreButton() {
		if (!elements.loadMore || !elements.loadMoreBtn) return;

		if (state.cursor) {
			elements.loadMore.style.display = 'flex';
			elements.loadMoreBtn.disabled = false;
		} else {
			elements.loadMore.style.display = 'none';
		}

		// Update count display
		const countEl = document.getElementById('timeline-count');
		if (countEl) {
			countEl.textContent = state.filteredEvents.length + ' of ' + state.total + ' events';
		}
	}

	function showError(message) {
		if (elements.error) {
			elements.error.textContent = message;
			elements.error.style.display = 'flex';
		}
	}

	// ============================================
	// Public API
	// ============================================
	const Timeline = {
		init: init,
		logEvent: function(eventType, zone, person, blobID, detail) {
			// Allow other modules to log events programmatically
			const event = {
				id: Date.now(),
				type: eventType,
				timestamp_ms: Date.now(),
				zone: zone || '',
				person: person || '',
				blob_id: blobID || 0,
				detail_json: detail ? JSON.stringify(detail) : '',
				severity: 'info'
			};

			handleNewEvent(event);
		},
		refresh: loadInitialEvents
	};

	// Auto-initialize when DOM is ready
	if (document.readyState === 'loading') {
		document.addEventListener('DOMContentLoaded', init);
	} else {
		init();
	}

	// Export for use by other modules
	window.SpaxelTimeline = Timeline;

	console.log('[Timeline] Timeline module loaded');
})();
