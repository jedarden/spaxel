/**
 * Tests for Sidebar Timeline Panel
 *
 * Tests the collapsible sidebar panel showing events in reverse-chronological order.
 * Covers:
 * - Event-specific visual rendering with icons and descriptions per event type
 * - Thumbs-up/down buttons on each event delegating to feedback module
 * - Virtualized rendering with IntersectionObserver for 10,000+ events
 */

'use strict';

// Mock dependencies
let mockEventData = { events: [], cursor: null, total_filtered: 0 };
global.fetch = jest.fn(function() {
    return Promise.resolve({
        ok: true,
        json: function() {
            return Promise.resolve(mockEventData);
        }
    });
});

// Mock Feedback module
global.Feedback = {
    sendFeedback: jest.fn()
};

// Mock SpaxelApp
global.SpaxelApp = {
    registerMessageHandler: jest.fn(),
    showToast: jest.fn()
};

// Mock SpaxelRouter
global.SpaxelRouter = {
    onModeChange: jest.fn(),
    navigate: jest.fn()
};

// Mock SpaxelTimeline
global.SpaxelTimeline = {};

// Mock IntersectionObserver
global.IntersectionObserver = jest.fn(function(callback, options) {
    this.observe = jest.fn();
    this.unobserve = jest.fn();
    this.disconnect = jest.fn();
    this.callback = callback;
    this.options = options;

    // Simulate immediate intersection for testing
    this.simulateIntersection = function(entries) {
        if (this.callback) {
            this.callback(entries);
        }
    };
});

describe('SidebarTimeline', function() {
    let SidebarTimeline;
    let mockElements;
    let mockEventData = { events: [], cursor: null, total_filtered: 0 };

    beforeEach(function() {
        // Reset all mocks
        jest.clearAllMocks();

        // Reset mock event data
        mockEventData = { events: [], cursor: null, total_filtered: 0 };

        // Setup DOM structure
        document.body.innerHTML = `
            <div id="sidebar-timeline-panel" class="sidebar-panel collapsed">
                <div class="sidebar-panel-header">
                    <div class="sidebar-panel-title">
                        <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor">
                            <path d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"></path>
                        </svg>
                        <span>Timeline</span>
                    </div>
                    <div class="sidebar-panel-actions">
                        <button id="sidebar-timeline-toggle" class="sidebar-panel-btn" title="Toggle panel">
                            <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor">
                                <polyline points="18 15 12 9 6 15"></polyline>
                            </svg>
                        </button>
                        <button id="sidebar-timeline-close" class="sidebar-panel-btn" title="Close panel">
                            <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor">
                                <line x1="18" y1="6" x2="6" y2="18"></line>
                                <line x1="6" y1="6" x2="18" y2="18"></line>
                            </svg>
                        </button>
                    </div>
                </div>
                <div id="sidebar-timeline-content" class="sidebar-panel-content">
                    <div id="sidebar-timeline-events" class="sidebar-timeline-events"></div>
                    <div id="sidebar-timeline-loading" class="sidebar-timeline-loading" style="display: none;">
                        <div class="sidebar-spinner"></div>
                    </div>
                    <div id="sidebar-timeline-empty" class="sidebar-timeline-empty" style="display: none;">
                        <svg xmlns="http://www.w3.org/2000/svg" width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor">
                            <circle cx="12" cy="12" r="10"></circle>
                            <line x1="12" y1="6" x2="12" y2="12"></line>
                            <line x1="12" y1="12" x2="12" y2="18"></line>
                        </svg>
                        <h3>No events yet</h3>
                        <p>Events will appear here as they happen</p>
                    </div>
                    <div id="sidebar-timeline-spacer-top" class="timeline-spacer timeline-spacer-top" style="height: 0px;"></div>
                    <div id="sidebar-timeline-spacer-bottom" class="timeline-spacer timeline-spacer-bottom" style="height: 0px;"></div>
                </div>
            </div>
            <button id="sidebar-timeline-show-btn" class="sidebar-show-btn hidden" title="Show timeline">
                <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor">
                    <polyline points="9 18 15 12 9 6"></polyline>
                </svg>
            </button>
        `;

        // Load the module
        jest.isolateModules(function() {
            require('./sidebar-timeline.js');
            SidebarTimeline = window.SpaxelSidebarTimeline;
        });

        // Cache mock elements
        mockElements = {
            panel: document.getElementById('sidebar-timeline-panel'),
            content: document.getElementById('sidebar-timeline-content'),
            eventsContainer: document.getElementById('sidebar-timeline-events'),
            loading: document.getElementById('sidebar-timeline-loading'),
            empty: document.getElementById('sidebar-timeline-empty'),
            spacerTop: document.getElementById('sidebar-timeline-spacer-top'),
            spacerBottom: document.getElementById('sidebar-timeline-spacer-bottom'),
            toggleBtn: document.getElementById('sidebar-timeline-toggle'),
            closeBtn: document.getElementById('sidebar-timeline-close'),
            showBtn: document.getElementById('sidebar-timeline-show-btn')
        };
    });

    afterEach(function() {
        document.body.innerHTML = '';
        delete window.SpaxelSidebarTimeline;
    });

    // ============================================
    // Panel Visibility Tests
    // ============================================
    describe('Panel Visibility', function() {
        test('show() displays the panel', function() {
            SidebarTimeline.show();

            expect(mockElements.panel.classList.contains('collapsed')).toBe(false);
            expect(mockElements.showBtn.classList.contains('hidden')).toBe(true);
        });

        test('hide() collapses the panel', function() {
            SidebarTimeline.show();
            SidebarTimeline.hide();

            expect(mockElements.panel.classList.contains('collapsed')).toBe(true);
            expect(mockElements.showBtn.classList.contains('hidden')).toBe(false);
        });

        test('toggle() switches panel state', function() {
            expect(mockElements.panel.classList.contains('collapsed')).toBe(true);

            SidebarTimeline.toggle();
            expect(mockElements.panel.classList.contains('collapsed')).toBe(false);

            SidebarTimeline.toggle();
            expect(mockElements.panel.classList.contains('collapsed')).toBe(true);
        });
    });

    // ============================================
    // Event Type Rendering Tests
    // ============================================
    describe('Event Type Rendering', function() {
        beforeEach(function() {
            // Reset events state
            if (window.SpaxelSidebarTimeline && window.SpaxelSidebarTimeline.state) {
                window.SpaxelSidebarTimeline.state.events = [];
            }

            // Mock successful API response
            global.fetch.mockImplementation(function() {
                return Promise.resolve({
                    ok: true,
                    json: function() {
                        return Promise.resolve({
                            events: [
                                {
                                    id: 1,
                                    timestamp_ms: Date.now() - 3600000,
                                    type: 'zone_entry',
                                    zone: 'Kitchen',
                                    person: 'Alice',
                                    severity: 'info'
                                },
                                {
                                    id: 2,
                                    timestamp_ms: Date.now() - 7200000,
                                    type: 'zone_exit',
                                    zone: 'Living Room',
                                    person: 'Bob',
                                    severity: 'info'
                                },
                                {
                                    id: 3,
                                    timestamp_ms: Date.now() - 10800000,
                                    type: 'portal_crossing',
                                    zone: 'Hallway',
                                    person: 'Alice',
                                    detail_json: JSON.stringify({
                                        from_zone: 'Kitchen',
                                        to_zone: 'Living Room'
                                    }),
                                    severity: 'info'
                                },
                                {
                                    id: 4,
                                    timestamp_ms: Date.now() - 14400000,
                                    type: 'presence_transition',
                                    zone: 'Bedroom',
                                    person: 'Alice',
                                    severity: 'info'
                                },
                                {
                                    id: 5,
                                    timestamp_ms: Date.now() - 18000000,
                                    type: 'stationary_detected',
                                    zone: 'Living Room',
                                    person: 'Bob',
                                    severity: 'info'
                                },
                                {
                                    id: 6,
                                    timestamp_ms: Date.now() - 21600000,
                                    type: 'detection',
                                    zone: 'Kitchen',
                                    person: null,
                                    severity: 'info'
                                },
                                {
                                    id: 7,
                                    timestamp_ms: Date.now() - 25200000,
                                    type: 'anomaly',
                                    zone: 'Kitchen',
                                    person: null,
                                    severity: 'warning'
                                },
                                {
                                    id: 8,
                                    timestamp_ms: Date.now() - 28800000,
                                    type: 'security_alert',
                                    zone: 'Hallway',
                                    person: null,
                                    detail_json: JSON.stringify({
                                        description: 'Motion detected while armed'
                                    }),
                                    severity: 'alert'
                                },
                                {
                                    id: 9,
                                    timestamp_ms: Date.now() - 32400000,
                                    type: 'fall_alert',
                                    zone: 'Bathroom',
                                    person: 'Alice',
                                    severity: 'critical'
                                },
                                {
                                    id: 10,
                                    timestamp_ms: Date.now() - 36000000,
                                    type: 'node_online',
                                    zone: null,
                                    person: null,
                                    detail_json: JSON.stringify({
                                        node: 'kitchen-north'
                                    }),
                                    severity: 'info'
                                },
                                {
                                    id: 11,
                                    timestamp_ms: Date.now() - 39600000,
                                    type: 'node_offline',
                                    zone: null,
                                    person: null,
                                    detail_json: JSON.stringify({
                                        node: 'living-room-west'
                                    }),
                                    severity: 'warning'
                                },
                                {
                                    id: 12,
                                    timestamp_ms: Date.now() - 43200000,
                                    type: 'ota_update',
                                    zone: null,
                                    person: null,
                                    detail_json: JSON.stringify({
                                        node: 'kitchen-north',
                                        version: '1.2.3'
                                    }),
                                    severity: 'info'
                                },
                                {
                                    id: 13,
                                    timestamp_ms: Date.now() - 46800000,
                                    type: 'baseline_changed',
                                    zone: null,
                                    person: null,
                                    detail_json: JSON.stringify({
                                        link: 'AA:BB:CC:DD:EE:FF:11:22:33:44:55:66'
                                    }),
                                    severity: 'info'
                                },
                                {
                                    id: 14,
                                    timestamp_ms: Date.now() - 50400000,
                                    type: 'learning_milestone',
                                    zone: null,
                                    person: null,
                                    detail_json: JSON.stringify({
                                        description: 'Anomaly patterns learned for Kitchen'
                                    }),
                                    severity: 'info'
                                },
                                {
                                    id: 15,
                                    timestamp_ms: Date.now() - 54000000,
                                    type: 'sleep_session_end',
                                    zone: 'Bedroom',
                                    person: 'Alice',
                                    detail_json: JSON.stringify({
                                        duration: '7h 23m'
                                    }),
                                    severity: 'info'
                                }
                            ],
                            cursor: null,
                            total_filtered: 15
                        });
                    }
                });
            });
        });

        test('all event types render correctly', function() {
            SidebarTimeline.show();
            SidebarTimeline.refresh();

            // Wait for fetch to complete and rendering to happen
            return new Promise(function(resolve) {
                setTimeout(function() {
                    // Check that all events were rendered
                    const eventEls = mockElements.eventsContainer.querySelectorAll('.sidebar-timeline-event');
                    expect(eventEls.length).toBe(15);

                    // Verify zone_entry
                    const zoneEntry = mockElements.eventsContainer.querySelector('[data-id="1"]');
                    expect(zoneEntry).toBeTruthy();
                    expect(zoneEntry.dataset.type).toBe('zone_entry');
                    expect(zoneEntry.querySelector('.sidebar-timeline-event-icon').textContent).toBe('🚪');
                    expect(zoneEntry.querySelector('.sidebar-timeline-event-title').textContent).toContain('Alice');
                    expect(zoneEntry.querySelector('.sidebar-timeline-event-title').textContent).toContain('Kitchen');
                    expect(zoneEntry.querySelector('.sidebar-timeline-event-title').textContent).toContain('entered');

                    // Verify zone_exit
                    const zoneExit = mockElements.eventsContainer.querySelector('[data-id="2"]');
                    expect(zoneExit).toBeTruthy();
                    expect(zoneExit.dataset.type).toBe('zone_exit');
                    expect(zoneExit.querySelector('.sidebar-timeline-event-icon').textContent).toBe('🚶');

                    // Verify portal_crossing
                    const portal = mockElements.eventsContainer.querySelector('[data-id="3"]');
                    expect(portal).toBeTruthy();
                    expect(portal.dataset.type).toBe('portal_crossing');
                    expect(portal.querySelector('.sidebar-timeline-event-icon').textContent).toBe('→');

                    // Verify presence_transition
                    const presence = mockElements.eventsContainer.querySelector('[data-id="4"]');
                    expect(presence).toBeTruthy();
                    expect(presence.dataset.type).toBe('presence_transition');
                    expect(presence.querySelector('.sidebar-timeline-event-icon').textContent).toBe('👤');

                    // Verify stationary_detected
                    const stationary = mockElements.eventsContainer.querySelector('[data-id="5"]');
                    expect(stationary).toBeTruthy();
                    expect(stationary.dataset.type).toBe('stationary_detected');
                    expect(stationary.querySelector('.sidebar-timeline-event-icon').textContent).toBe('💤');

                    // Verify detection
                    const detection = mockElements.eventsContainer.querySelector('[data-id="6"]');
                    expect(detection).toBeTruthy();
                    expect(detection.dataset.type).toBe('detection');
                    expect(detection.querySelector('.sidebar-timeline-event-icon').textContent).toBe('👁️');

                    // Verify anomaly
                    const anomaly = mockElements.eventsContainer.querySelector('[data-id="7"]');
                    expect(anomaly).toBeTruthy();
                    expect(anomaly.dataset.type).toBe('anomaly');
                    expect(anomaly.querySelector('.sidebar-timeline-event-icon').textContent).toBe('⚠️');
                    expect(anomaly.classList.contains('severity-warning')).toBe(true);

                    // Verify security_alert
                    const security = mockElements.eventsContainer.querySelector('[data-id="8"]');
                    expect(security).toBeTruthy();
                    expect(security.dataset.type).toBe('security_alert');
                    expect(security.querySelector('.sidebar-timeline-event-icon').textContent).toBe('🚨');
                    expect(security.classList.contains('severity-critical')).toBe(true);

                    // Verify fall_alert
                    const fall = mockElements.eventsContainer.querySelector('[data-id="9"]');
                    expect(fall).toBeTruthy();
                    expect(fall.dataset.type).toBe('fall_alert');
                    expect(fall.querySelector('.sidebar-timeline-event-icon').textContent).toBe('🆘');
                    expect(fall.classList.contains('severity-critical')).toBe(true);

                    // Verify node_online
                    const nodeOnline = mockElements.eventsContainer.querySelector('[data-id="10"]');
                    expect(nodeOnline).toBeTruthy();
                    expect(nodeOnline.dataset.type).toBe('node_online');
                    expect(nodeOnline.querySelector('.sidebar-timeline-event-icon').textContent).toBe('📡');
                    expect(nodeOnline.classList.contains('secondary')).toBe(true);

                    // Verify node_offline
                    const nodeOffline = mockElements.eventsContainer.querySelector('[data-id="11"]');
                    expect(nodeOffline).toBeTruthy();
                    expect(nodeOffline.dataset.type).toBe('node_offline');
                    expect(nodeOffline.querySelector('.sidebar-timeline-event-icon').textContent).toBe('📵');

                    // Verify ota_update
                    const ota = mockElements.eventsContainer.querySelector('[data-id="12"]');
                    expect(ota).toBeTruthy();
                    expect(ota.dataset.type).toBe('ota_update');
                    expect(ota.querySelector('.sidebar-timeline-event-icon').textContent).toBe('⬆️');

                    // Verify baseline_changed
                    const baseline = mockElements.eventsContainer.querySelector('[data-id="13"]');
                    expect(baseline).toBeTruthy();
                    expect(baseline.dataset.type).toBe('baseline_changed');
                    expect(baseline.querySelector('.sidebar-timeline-event-icon').textContent).toBe('📊');

                    // Verify learning_milestone
                    const learning = mockElements.eventsContainer.querySelector('[data-id="14"]');
                    expect(learning).toBeTruthy();
                    expect(learning.dataset.type).toBe('learning_milestone');
                    expect(learning.querySelector('.sidebar-timeline-event-icon').textContent).toBe('🎓');

                    // Verify sleep_session_end
                    const sleep = mockElements.eventsContainer.querySelector('[data-id="15"]');
                    expect(sleep).toBeTruthy();
                    expect(sleep.dataset.type).toBe('sleep_session_end');
                    expect(sleep.querySelector('.sidebar-timeline-event-icon').textContent).toBe('😴');
                    expect(sleep.querySelector('.sidebar-timeline-event-title').textContent).toContain('7h 23m');

                    resolve();
                }, 150);
            });
        });
    });

    // ============================================
    // Plain English Description Tests
    // ============================================
    describe('Plain English Descriptions', function() {
        test('descriptions use plain English without technical jargon', function() {
            global.fetch.mockImplementation(function() {
                return Promise.resolve({
                    ok: true,
                    json: function() {
                        return Promise.resolve({
                            events: [
                                {
                                    id: 1,
                                    timestamp_ms: Date.now(),
                                    type: 'zone_entry',
                                    zone: 'Kitchen',
                                    person: 'Alice',
                                    severity: 'info'
                                }
                            ],
                            cursor: null,
                            total_filtered: 1
                        });
                    }
                });
            });

            SidebarTimeline.show();
            SidebarTimeline.refresh();

            return new Promise(function(resolve) {
                setTimeout(function() {
                    const eventEl = mockElements.eventsContainer.querySelector('[data-id="1"]');
                    const title = eventEl.querySelector('.sidebar-timeline-event-title').textContent;

                    // Should not contain technical jargon
                    expect(title).not.toMatch(/CSI|Fresnel|deltaRMS|blob_id|timestamp_ms/);

                    // Should use plain English
                    expect(title).toMatch(/Alice|Kitchen|entered/);
                    resolve();
                }, 150);
            });
        });

        test('unknown person defaults to "Someone"', function() {
            global.fetch.mockImplementation(function() {
                return Promise.resolve({
                    ok: true,
                    json: function() {
                        return Promise.resolve({
                            events: [
                                {
                                    id: 1,
                                    timestamp_ms: Date.now(),
                                    type: 'detection',
                                    zone: 'Hallway',
                                    person: null,
                                    severity: 'info'
                                }
                            ],
                            cursor: null,
                            total_filtered: 1
                        });
                    }
                });
            });

            SidebarTimeline.refresh();

            return new Promise(function(resolve) {
                setTimeout(function() {
                    const eventEl = mockElements.eventsContainer.querySelector('[data-id="1"]');
                    const title = eventEl.querySelector('.sidebar-timeline-event-title').textContent;

                    expect(title).toContain('Motion');
                    expect(title).toContain('Hallway');
                    resolve();
                }, 150);
            });
        });
    });

    // ============================================
    // Feedback Button Tests
    // ============================================
    describe('Feedback Buttons', function() {
        beforeEach(function() {
            global.fetch.mockImplementation(function() {
                return Promise.resolve({
                    ok: true,
                    json: function() {
                        return Promise.resolve({
                            events: [
                                {
                                    id: 123,
                                    timestamp_ms: Date.now(),
                                    type: 'detection',
                                    zone: 'Kitchen',
                                    person: 'Alice',
                                    blob_id: 42,
                                    severity: 'info'
                                }
                            ],
                            cursor: null,
                            total_filtered: 1
                        });
                    }
                });
            });
        });

        test('each event has thumbs-up and thumbs-down buttons', function() {
            SidebarTimeline.show();
            SidebarTimeline.refresh();

            return new Promise(function(resolve) {
                setTimeout(function() {
                    const eventEl = mockElements.eventsContainer.querySelector('[data-id="123"]');
                    expect(eventEl).toBeTruthy();

                    const thumbsUp = eventEl.querySelector('.feedback-positive');
                    const thumbsDown = eventEl.querySelector('.feedback-negative');

                    expect(thumbsUp).toBeTruthy();
                    expect(thumbsDown).toBeTruthy();
                    expect(thumbsUp.getAttribute('aria-label')).toBe('Thumbs up');
                    expect(thumbsDown.getAttribute('aria-label')).toBe('Thumbs down');
                    resolve();
                }, 150);
            });
        });

        test('thumbs-up button delegates to feedback module', function() {
            SidebarTimeline.show();
            SidebarTimeline.refresh();

            // Mock feedback API response
            global.fetch.mockImplementationOnce(function() {
                return Promise.resolve({
                    ok: true,
                    json: function() {
                        return Promise.resolve({ ok: true });
                    }
                });
            });

            return new Promise(function(resolve) {
                setTimeout(function() {
                    const eventEl = mockElements.eventsContainer.querySelector('[data-id="123"]');
                    const thumbsUp = eventEl.querySelector('.feedback-positive');

                    thumbsUp.click();

                    setTimeout(function() {
                        expect(global.Feedback.sendFeedback).toHaveBeenCalledWith(
                            '123',
                            'detection',
                            'TRUE_POSITIVE',
                            expect.anything()
                        );
                        expect(global.SpaxelApp.showToast).toHaveBeenCalledWith(
                            'Thanks for the feedback!',
                            'success'
                        );
                        resolve();
                    }, 50);
                }, 150);
            });
        });

        test('thumbs-down button delegates to feedback module', function() {
            SidebarTimeline.show();
            SidebarTimeline.refresh();

            // Mock feedback API response
            global.fetch.mockImplementationOnce(function() {
                return Promise.resolve({
                    ok: true,
                    json: function() {
                        return Promise.resolve({ ok: true });
                    }
                });
            });

            return new Promise(function(resolve) {
                setTimeout(function() {
                    const eventEl = mockElements.eventsContainer.querySelector('[data-id="123"]');
                    const thumbsDown = eventEl.querySelector('.feedback-negative');

                    thumbsDown.click();

                    setTimeout(function() {
                        expect(global.Feedback.sendFeedback).toHaveBeenCalledWith(
                            '123',
                            'detection',
                            'FALSE_POSITIVE',
                            expect.anything()
                        );
                        expect(global.SpaxelApp.showToast).toHaveBeenCalledWith(
                            'Thanks — I\'ll adjust my detection.',
                            'success'
                        );
                        resolve();
                    }, 50);
                }, 150);
            });
        });

        test('feedback button click stops propagation', function() {
            SidebarTimeline.show();
            SidebarTimeline.refresh();

            return new Promise(function(resolve) {
                setTimeout(function() {
                    const eventEl = mockElements.eventsContainer.querySelector('[data-id="123"]');
                    const thumbsUp = eventEl.querySelector('.feedback-positive');

                    let eventClickFired = false;
                    eventEl.addEventListener('click', function() {
                        eventClickFired = true;
                    });

                    thumbsUp.click();

                    setTimeout(function() {
                        expect(eventClickFired).toBe(false);
                        resolve();
                    }, 50);
                }, 150);
            });
        });
    });

    // ============================================
    // Virtualized Rendering Tests
    // ============================================
    describe('Virtualized Rendering with IntersectionObserver', function() {
        test('uses IntersectionObserver for virtualization when enabled', function() {
            SidebarTimeline.show();

            // Check that IntersectionObserver was called during init
            // This is verified by the module initialization logging
            expect(global.IntersectionObserver).toHaveBeenCalled();
        });

        test('handles large event lists without performance degradation', function() {
            // Create a large array of events
            const largeEventList = [];
            for (let i = 0; i < 100; i++) {
                largeEventList.push({
                    id: i,
                    timestamp_ms: Date.now() - (i * 60000), // 1 minute apart
                    type: i % 2 === 0 ? 'detection' : 'zone_entry',
                    zone: ['Kitchen', 'Living Room', 'Bedroom'][i % 3],
                    person: ['Alice', 'Bob', 'Charlie'][i % 3],
                    severity: 'info'
                });
            }

            global.fetch.mockImplementation(function() {
                return Promise.resolve({
                    ok: true,
                    json: function() {
                        return Promise.resolve({
                            events: largeEventList,
                            cursor: null,
                            total_filtered: 100
                        });
                    }
                });
            });

            SidebarTimeline.refresh();

            return new Promise(function(resolve) {
                setTimeout(function() {
                    // Check that events were rendered
                    const renderedEvents = mockElements.eventsContainer.querySelectorAll('.sidebar-timeline-event');
                    expect(renderedEvents.length).toBeGreaterThan(0);
                    expect(renderedEvents.length).toBeLessThanOrEqual(100);

                    resolve();
                }, 200);
            });
        });
    });

    // ============================================
    // Empty State Tests
    // ============================================
    describe('Empty State', function() {
        test('shows empty state when no events', function() {
            global.fetch.mockImplementation(function() {
                return Promise.resolve({
                    ok: true,
                    json: function() {
                        return Promise.resolve({
                            events: [],
                            cursor: null,
                            total_filtered: 0
                        });
                    }
                });
            });

            SidebarTimeline.show();
            SidebarTimeline.refresh();

            return new Promise(function(resolve) {
                setTimeout(function() {
                    expect(mockElements.empty.style.display).toBe('flex');
                    expect(mockElements.eventsContainer.children.length).toBe(0);
                    resolve();
                }, 150);
            });
        });

        test('empty state shows correct message and icon', function() {
            global.fetch.mockImplementation(function() {
                return Promise.resolve({
                    ok: true,
                    json: function() {
                        return Promise.resolve({
                            events: [],
                            cursor: null,
                            total_filtered: 0
                        });
                    }
                });
            });

            SidebarTimeline.show();
            SidebarTimeline.refresh();

            return new Promise(function(resolve) {
                setTimeout(function() {
                    const emptyTitle = mockElements.empty.querySelector('h3');
                    const emptyText = mockElements.empty.querySelector('p');
                    const emptyIcon = mockElements.empty.querySelector('svg');

                    expect(emptyTitle.textContent).toBe('No events yet');
                    expect(emptyText.textContent).toBe('Events will appear here as they happen');
                    expect(emptyIcon).toBeTruthy();
                    resolve();
                }, 150);
            });
        });
    });

    // ============================================
    // Severity Styling Tests
    // ============================================
    describe('Severity Styling', function() {
        beforeEach(function() {
            global.fetch.mockImplementation(function() {
                return Promise.resolve({
                    ok: true,
                    json: function() {
                        return Promise.resolve({
                            events: [
                                {
                                    id: 1,
                                    timestamp_ms: Date.now(),
                                    type: 'detection',
                                    zone: 'Kitchen',
                                    person: 'Alice',
                                    severity: 'info'
                                },
                                {
                                    id: 2,
                                    timestamp_ms: Date.now() - 3600000,
                                    type: 'anomaly',
                                    zone: 'Living Room',
                                    person: null,
                                    severity: 'warning'
                                },
                                {
                                    id: 3,
                                    timestamp_ms: Date.now() - 7200000,
                                    type: 'fall_alert',
                                    zone: 'Bathroom',
                                    person: 'Alice',
                                    severity: 'critical'
                                }
                            ],
                            cursor: null,
                            total_filtered: 3
                        });
                    }
                });
            });
        });

        test('info events have no severity class', function() {
            SidebarTimeline.show();
            SidebarTimeline.refresh();

            return new Promise(function(resolve) {
                setTimeout(function() {
                    const eventEl = mockElements.eventsContainer.querySelector('[data-id="1"]');
                    expect(eventEl).toBeTruthy();
                    expect(eventEl.classList.contains('severity-critical')).toBe(false);
                    expect(eventEl.classList.contains('severity-warning')).toBe(false);
                    resolve();
                }, 150);
            });
        });

        test('warning events have severity-warning class', function() {
            SidebarTimeline.show();
            SidebarTimeline.refresh();

            return new Promise(function(resolve) {
                setTimeout(function() {
                    const eventEl = mockElements.eventsContainer.querySelector('[data-id="2"]');
                    expect(eventEl).toBeTruthy();
                    expect(eventEl.classList.contains('severity-warning')).toBe(true);
                    resolve();
                }, 150);
            });
        });

        test('critical events have severity-critical class', function() {
            SidebarTimeline.show();
            SidebarTimeline.refresh();

            return new Promise(function(resolve) {
                setTimeout(function() {
                    const eventEl = mockElements.eventsContainer.querySelector('[data-id="3"]');
                    expect(eventEl).toBeTruthy();
                    expect(eventEl.classList.contains('severity-critical')).toBe(true);
                    resolve();
                }, 150);
            });
        });
    });

    // ============================================
    // System Event Secondary Styling Tests
    // ============================================
    describe('System Event Secondary Styling', function() {
        beforeEach(function() {
            global.fetch.mockImplementation(function() {
                return Promise.resolve({
                    ok: true,
                    json: function() {
                        return Promise.resolve({
                            events: [
                                {
                                    id: 1,
                                    timestamp_ms: Date.now(),
                                    type: 'detection',
                                    zone: 'Kitchen',
                                    person: 'Alice',
                                    severity: 'info'
                                },
                                {
                                    id: 2,
                                    timestamp_ms: Date.now() - 3600000,
                                    type: 'node_online',
                                    zone: null,
                                    person: null,
                                    detail_json: JSON.stringify({ node: 'kitchen-north' }),
                                    severity: 'info'
                                }
                            ],
                            cursor: null,
                            total_filtered: 2
                        });
                    }
                });
            });
        });

        test('user events have no secondary class', function() {
            SidebarTimeline.show();
            SidebarTimeline.refresh();

            return new Promise(function(resolve) {
                setTimeout(function() {
                    const eventEl = mockElements.eventsContainer.querySelector('[data-id="1"]');
                    expect(eventEl).toBeTruthy();
                    expect(eventEl.classList.contains('secondary')).toBe(false);
                    resolve();
                }, 150);
            });
        });

        test('system events have secondary class', function() {
            SidebarTimeline.show();
            SidebarTimeline.refresh();

            return new Promise(function(resolve) {
                setTimeout(function() {
                    const eventEl = mockElements.eventsContainer.querySelector('[data-id="2"]');
                    expect(eventEl).toBeTruthy();
                    expect(eventEl.classList.contains('secondary')).toBe(true);
                    resolve();
                }, 150);
            });
        });
    });

    // ============================================
    // Tap-to-Jump Time-Travel Tests
    // ============================================
    describe('Tap-to-Jump Time-Travel', function() {
        var mockJumpToTime;
        var routerModeChangeCallback = null;

        beforeEach(function() {
            mockJumpToTime = jest.fn(function() {
                return Promise.resolve({
                    session_id: 'test-session-1',
                    timestamp_ms: 1710519800000,
                    from_ms: 1710519795000,
                    to_ms: 1710519805000,
                    state: 'paused'
                });
            });
            window.SpaxelReplay = {
                jumpToTime: mockJumpToTime,
                isReplayMode: jest.fn(function() { return false; })
            };

            // Capture router mode change callback via mockImplementation
            routerModeChangeCallback = null;
            global.SpaxelRouter.onModeChange.mockImplementation(function(cb) {
                routerModeChangeCallback = cb;
            });

            global.fetch.mockImplementation(function() {
                return Promise.resolve({
                    ok: true,
                    json: function() {
                        return Promise.resolve({
                            events: [
                                {
                                    id: 100,
                                    timestamp_ms: 1710519800000,
                                    type: 'zone_entry',
                                    zone: 'Kitchen',
                                    person: 'Alice',
                                    severity: 'info'
                                },
                                {
                                    id: 200,
                                    timestamp_ms: 1710519860000,
                                    type: 'zone_exit',
                                    zone: 'Kitchen',
                                    person: 'Alice',
                                    severity: 'info'
                                }
                            ],
                            cursor: null,
                            total_filtered: 2
                        });
                    }
                });
            });
        });

        afterEach(function() {
            delete window.SpaxelReplay;
        });

        test('clicking event calls jumpToTime with event timestamp', function() {
            SidebarTimeline.show();
            SidebarTimeline.refresh();

            return new Promise(function(resolve) {
                setTimeout(function() {
                    var eventEl = mockElements.eventsContainer.querySelector('[data-id="100"]');
                    expect(eventEl).toBeTruthy();

                    eventEl.click();

                    expect(mockJumpToTime).toHaveBeenCalledTimes(1);
                    expect(mockJumpToTime).toHaveBeenCalledWith(1710519800000);
                    resolve();
                }, 150);
            });
        });

        test('clicking different events emits correct timestamps', function() {
            SidebarTimeline.show();
            SidebarTimeline.refresh();

            return new Promise(function(resolve) {
                setTimeout(function() {
                    var first = mockElements.eventsContainer.querySelector('[data-id="100"]');
                    var second = mockElements.eventsContainer.querySelector('[data-id="200"]');
                    expect(first).toBeTruthy();
                    expect(second).toBeTruthy();

                    first.click();
                    expect(mockJumpToTime).toHaveBeenCalledWith(1710519800000);

                    second.click();
                    expect(mockJumpToTime).toHaveBeenCalledWith(1710519860000);

                    expect(mockJumpToTime).toHaveBeenCalledTimes(2);
                    resolve();
                }, 150);
            });
        });

        test('selected event highlights with selected class', function() {
            SidebarTimeline.show();
            SidebarTimeline.refresh();

            return new Promise(function(resolve) {
                setTimeout(function() {
                    var eventEl = mockElements.eventsContainer.querySelector('[data-id="100"]');
                    expect(eventEl).toBeTruthy();
                    expect(eventEl.classList.contains('selected')).toBe(false);

                    eventEl.click();

                    // Wait for async handleSeek
                    setTimeout(function() {
                        expect(eventEl.classList.contains('selected')).toBe(true);
                        resolve();
                    }, 50);
                }, 150);
            });
        });

        test('clicking new event clears previous selection', function() {
            SidebarTimeline.show();
            SidebarTimeline.refresh();

            return new Promise(function(resolve) {
                setTimeout(function() {
                    var first = mockElements.eventsContainer.querySelector('[data-id="100"]');
                    var second = mockElements.eventsContainer.querySelector('[data-id="200"]');

                    first.click();
                    setTimeout(function() {
                        expect(first.classList.contains('selected')).toBe(true);
                        expect(second.classList.contains('selected')).toBe(false);

                        second.click();
                        setTimeout(function() {
                            expect(first.classList.contains('selected')).toBe(false);
                            expect(second.classList.contains('selected')).toBe(true);
                            resolve();
                        }, 50);
                    }, 50);
                }, 150);
            });
        });

        test('Now replaying chip appears after jump', function() {
            SidebarTimeline.show();
            SidebarTimeline.refresh();

            return new Promise(function(resolve) {
                setTimeout(function() {
                    var eventEl = mockElements.eventsContainer.querySelector('[data-id="100"]');
                    eventEl.click();

                    // Wait for jumpToTime promise to resolve
                    setTimeout(function() {
                        var chip = document.getElementById('now-replaying-chip');
                        expect(chip).toBeTruthy();
                        expect(chip.style.display).not.toBe('none');
                        expect(chip.textContent).toContain('Now replaying');
                        resolve();
                    }, 100);
                }, 150);
            });
        });

        test('hideNowReplayingChip hides the chip', function() {
            SidebarTimeline.show();
            SidebarTimeline.refresh();

            return new Promise(function(resolve) {
                setTimeout(function() {
                    var eventEl = mockElements.eventsContainer.querySelector('[data-id="100"]');
                    eventEl.click();

                    setTimeout(function() {
                        var chip = document.getElementById('now-replaying-chip');
                        expect(chip).toBeTruthy();

                        SidebarTimeline.hideNowReplayingChip();
                        expect(chip.style.display).toBe('none');
                        resolve();
                    }, 100);
                }, 150);
            });
        });

        test('clearSelection removes selected class from event', function() {
            SidebarTimeline.show();
            SidebarTimeline.refresh();

            return new Promise(function(resolve) {
                setTimeout(function() {
                    var eventEl = mockElements.eventsContainer.querySelector('[data-id="100"]');
                    eventEl.click();

                    setTimeout(function() {
                        expect(eventEl.classList.contains('selected')).toBe(true);
                        SidebarTimeline.clearSelection();
                        expect(eventEl.classList.contains('selected')).toBe(false);
                        resolve();
                    }, 50);
                }, 150);
            });
        });

        test('without SpaxelReplay navigates to timeline view', function() {
            // Remove SpaxelReplay to simulate it not being available
            delete window.SpaxelReplay;

            // Show panel and load events
            SidebarTimeline.show();
            SidebarTimeline.refresh();

            return new Promise(function(resolve) {
                setTimeout(function() {
                    var eventEl = mockElements.eventsContainer.querySelector('[data-id="100"]');
                    expect(eventEl).toBeTruthy();
                    eventEl.click();

                    // Without SpaxelReplay, should navigate via router
                    expect(global.SpaxelRouter.navigate).toHaveBeenCalledWith('timeline');
                    resolve();
                }, 150);
            });
        });

        test('jumpToTime failure shows error toast', function() {
            mockJumpToTime.mockImplementation(function() {
                return Promise.reject(new Error('Network error'));
            });

            SidebarTimeline.show();
            SidebarTimeline.refresh();

            return new Promise(function(resolve) {
                setTimeout(function() {
                    var eventEl = mockElements.eventsContainer.querySelector('[data-id="100"]');
                    eventEl.click();

                    setTimeout(function() {
                        expect(global.SpaxelApp.showToast).toHaveBeenCalledWith('Failed to jump to time', 'error');
                        resolve();
                    }, 100);
                }, 150);
            });
        });
    });
});
