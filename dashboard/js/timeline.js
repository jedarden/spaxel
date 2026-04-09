/**
 * Spaxel Dashboard - Activity Timeline
 *
 * Scrollable chronological event list with filtering and event interaction.
 * Click event → jump to that moment in replay mode.
 * Inline feedback (thumbs up/down) on presence detection events.
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
		replaySeekWindowSec: 5 // seconds before/after event timestamp
	};

	// ============================================
	// State
	// ============================================
	const state = {
		events: [],
		cursor: null,
		total: 0,
		filters: {
			type: null,
			zone: null,
			person: null,
			after: null, // ISO8601 string
			q: null
		},
		loading: false,
		error: null,
		// Filter options populated from available events
		availableTypes: new Set(),
		availableZones: new Set(),
		availablePersons: new Set()
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
			description: 'Person entered a zone'
		},
		zone_exit: {
			icon: '🚶',
			color: '#ffa726',
			label: 'Left',
			description: 'Person exited a zone'
		},
		portal_crossing: {
			icon: '→',
			color: '#42a5f5',
			label: 'Crossed',
			description: 'Person crossed a portal'
		},
		presence_transition: {
			icon: '👤',
			color: '#ab47bc',
			label: 'Detected',
			description: 'Presence detected'
		},
		stationary_detected: {
			icon: '💤',
			color: '#7e57c2',
			label: 'Stationary',
			description: 'Stationary person detected'
		},
		anomaly: {
			icon: '⚠️',
			color: '#ef5350',
			label: 'Anomaly',
			description: 'Unusual activity detected'
		},
		security_alert: {
			icon: '🚨',
			color: '#d32f2f',
			label: 'Security',
			description: 'Security alert'
		},
		fall_alert: {
			icon: '🆘',
			color: '#f44336',
			label: 'Fall',
			description: 'Fall detected'
		},
		node_online: {
			icon: '📡',
			color: '#4caf50',
			label: 'Online',
			description: 'Node came online'
		},
		node_offline: {
			icon: '📵',
			color: '#9e9e9e',
			label: 'Offline',
			description: 'Node went offline'
		},
		ota_update: {
			icon: '⬆️',
			color: '#2196f3',
			label: 'Updated',
			description: 'Firmware updated'
		},
		baseline_changed: {
			icon: '📊',
			color: '#00bcd4',
			label: 'Baseline',
			description: 'Baseline updated'
		},
		learning_milestone: {
			icon: '🎓',
			color: '#9c27b0',
			label: 'Learned',
			description: 'System learned patterns'
		},
		system: {
			icon: '⚙️',
			color: '#607d8b',
			label: 'System',
			description: 'System event'
		}
	};

	// ============================================
	// Initialization
	// ============================================
	function init() {
		console.log('[Timeline] Initializing');

		cacheElements();
		bindEvents();

		// Listen for route changes to show/hide timeline
		if (window.SpaxelRouter) {
			SpaxelRouter.onModeChange(onModeChange);
		}

		// Listen for WebSocket event messages
		if (window.SpaxelApp) {
			SpaxelApp.registerMessageHandler(handleWebSocketMessage);
		}
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
			loadMoreBtn: document.getElementById('timeline-load-more-btn')
		};
	}

	function bindEvents() {
		// Filter changes
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
			elements.filterTime.addEventListener('change', onFilterChange);
		}
		if (elements.filterSearch) {
			let searchTimeout;
			elements.filterSearch.addEventListener('input', function() {
				clearTimeout(searchTimeout);
				searchTimeout = setTimeout(onFilterChange, CONFIG.debounceMs);
			});
		}

		// Load more button
		if (elements.loadMoreBtn) {
			elements.loadMoreBtn.addEventListener('click', loadMoreEvents);
		}
	}

	// ============================================
	// Mode Change Handler
	// ============================================
	function onModeChange(newMode, oldMode) {
		const container = elements.container;
		if (!container) return;

		if (newMode === 'timeline') {
			// Container is shown by inline script, just load events if needed
			if (state.events.length === 0) {
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
					state.events = [];
					updateFilterOptions(data.events);
				}

				state.events = state.events.concat(data.events || []);
				state.cursor = data.cursor || null;
				state.total = data.total || 0;

				renderEvents();
				updateLoadMoreButton();
				updateFilterOptions(data.events);
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

	function applyFiltersToParams(params) {
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
			params.set('after', state.filters.after);
		}
		if (state.filters.q) {
			params.set('q', state.filters.q);
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

		// Add to beginning of events array
		state.events.unshift(normalizedEvent);
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

		// Prepend to DOM if timeline is visible (no layout shift)
		if (elements.container && elements.container.style.display !== 'none' && elements.eventsList) {
			elements.empty.style.display = 'none';

			const tempDiv = document.createElement('div');
			tempDiv.innerHTML = renderEvent(normalizedEvent, true);
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
				elements.eventsList.removeChild(elements.eventsList.lastChild);
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

		if (state.events.length === 0) {
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

		// Build HTML for events
		let html = '';
		state.events.forEach(function(event) {
			html += renderEvent(event);
		});

		elements.eventsList.innerHTML = html;

		// Bind click handlers
		bindEventHandlers();
	}

	function renderEvent(event, isNew) {
		const info = eventTypeInfo[event.type] || eventTypeInfo.system;
		const timeStr = formatTimestamp(event.timestamp_ms);
		const personStr = event.person ? escapeHtml(event.person) : '';
		const zoneStr = event.zone ? escapeHtml(event.zone) : '';
		const description = buildEventDescription(event);

		// Severity indicator for alerts
		const severityClass = event.severity === 'alert' || event.severity === 'critical' ? ' severity-critical' : '';
		const newClass = isNew ? ' new-event' : '';

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

		return `
			<div class="timeline-event timeline-${event.type}${severityClass}${newClass}" data-type="${event.type}" data-id="${event.id}" data-timestamp="${event.timestamp_ms}" data-blob-id="${event.blob_id || ''}">
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

	// ============================================
	// Filter Handling
	// ============================================
	function onFilterChange() {
		// Update filter state
		if (elements.filterType) {
			state.filters.type = elements.filterType.value || null;
		}
		if (elements.filterZone) {
			state.filters.zone = elements.filterZone.value || null;
		}
		if (elements.filterPerson) {
			state.filters.person = elements.filterPerson.value || null;
		}
		if (elements.filterTime) {
			const value = elements.filterTime.value;
			if (value === 'today') {
				const today = new Date();
				today.setHours(0, 0, 0, 0);
				state.filters.after = today.toISOString();
			} else if (value === '7d') {
				const weekAgo = new Date(Date.now() - 7 * 24 * 60 * 60 * 1000);
				state.filters.after = weekAgo.toISOString();
			} else if (value === '30d') {
				const monthAgo = new Date(Date.now() - 30 * 24 * 60 * 60 * 1000);
				state.filters.after = monthAgo.toISOString();
			} else {
				state.filters.after = null;
			}
		}
		if (elements.filterSearch) {
			state.filters.q = elements.filterSearch.value.trim() || null;
		}

		// Reload events with new filters
		loadInitialEvents();
	}

	function updateFilterOptions(events) {
		// Extract unique values from events
		const types = new Set();
		const zones = new Set();
		const persons = new Set();

		(events || state.events).forEach(function(event) {
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
			countEl.textContent = state.events.length + ' events';
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
		}
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
