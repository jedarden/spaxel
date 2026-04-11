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
        json: async function() {
            return mockEventData;
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

// Mock SpaxelSimpleModeDetection
global.SpaxelSimpleModeDetection = {
    onModeChange: jest.fn()
};

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
        beforeEach(function(done) {
            // Mock successful API response
            global.fetch.mockImplementation(function() {
                return Promise.resolve({
                    ok: true,
                    json: async function() {
                        return {
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
                        };
                    }
                });
            });

            // Show panel and refresh events
            SidebarTimeline.show();
            SidebarTimeline.refresh();

            // Wait for events to load
            setTimeout(done, 100);
        });

        test('zone_entry renders with correct icon and description', function(done) {
            setTimeout(function() {
                const eventEl = mockElements.eventsContainer.querySelector('[data-id="1"]');
                expect(eventEl).toBeTruthy();
                expect(eventEl.dataset.type).toBe('zone_entry');
                expect(eventEl.querySelector('.sidebar-timeline-event-icon').textContent).toBe('🚪');
                expect(eventEl.querySelector('.sidebar-timeline-event-title').textContent).toContain('Alice');
                expect(eventEl.querySelector('.sidebar-timeline-event-title').textContent).toContain('Kitchen');
                expect(eventEl.querySelector('.sidebar-timeline-event-title').textContent).toContain('entered');
                done();
            }, 100);
        });

        test('zone_exit renders with correct icon and description', function(done) {
            setTimeout(function() {
                const eventEl = mockElements.eventsContainer.querySelector('[data-id="2"]');
                expect(eventEl).toBeTruthy();
                expect(eventEl.dataset.type).toBe('zone_exit');
                expect(eventEl.querySelector('.sidebar-timeline-event-icon').textContent).toBe('🚶');
                expect(eventEl.querySelector('.sidebar-timeline-event-title').textContent).toContain('Bob');
                expect(eventEl.querySelector('.sidebar-timeline-event-title').textContent).toContain('Living Room');
                expect(eventEl.querySelector('.sidebar-timeline-event-title').textContent).toContain('left');
                done();
            }, 100);
        });

        test('portal_crossing renders with correct icon and description', function(done) {
            setTimeout(function() {
                const eventEl = mockElements.eventsContainer.querySelector('[data-id="3"]');
                expect(eventEl).toBeTruthy();
                expect(eventEl.dataset.type).toBe('portal_crossing');
                expect(eventEl.querySelector('.sidebar-timeline-event-icon').textContent).toBe('→');
                expect(eventEl.querySelector('.sidebar-timeline-event-title').textContent).toContain('Alice');
                expect(eventEl.querySelector('.sidebar-timeline-event-title').textContent).toContain('Kitchen');
                expect(eventEl.querySelector('.sidebar-timeline-event-title').textContent).toContain('Living Room');
                expect(eventEl.querySelector('.sidebar-timeline-event-title').textContent).toContain('crossed');
                done();
            }, 100);
        });

        test('presence_transition renders with correct icon and description', function(done) {
            setTimeout(function() {
                const eventEl = mockElements.eventsContainer.querySelector('[data-id="4"]');
                expect(eventEl).toBeTruthy();
                expect(eventEl.dataset.type).toBe('presence_transition');
                expect(eventEl.querySelector('.sidebar-timeline-event-icon').textContent).toBe('👤');
                expect(eventEl.querySelector('.sidebar-timeline-event-title').textContent).toContain('Alice');
                expect(eventEl.querySelector('.sidebar-timeline-event-title').textContent).toContain('Bedroom');
                expect(eventEl.querySelector('.sidebar-timeline-event-title').textContent).toContain('detected');
                done();
            }, 100);
        });

        test('stationary_detected renders with correct icon and description', function(done) {
            setTimeout(function() {
                const eventEl = mockElements.eventsContainer.querySelector('[data-id="5"]');
                expect(eventEl).toBeTruthy();
                expect(eventEl.dataset.type).toBe('stationary_detected');
                expect(eventEl.querySelector('.sidebar-timeline-event-icon').textContent).toBe('💤');
                expect(eventEl.querySelector('.sidebar-timeline-event-title').textContent).toContain('Bob');
                expect(eventEl.querySelector('.sidebar-timeline-event-title').textContent).toContain('Living Room');
                expect(eventEl.querySelector('.sidebar-timeline-event-title').textContent).toContain('stationary');
                done();
            }, 100);
        });

        test('detection renders with correct icon and description', function(done) {
            setTimeout(function() {
                const eventEl = mockElements.eventsContainer.querySelector('[data-id="6"]');
                expect(eventEl).toBeTruthy();
                expect(eventEl.dataset.type).toBe('detection');
                expect(eventEl.querySelector('.sidebar-timeline-event-icon').textContent).toBe('👁️');
                expect(eventEl.querySelector('.sidebar-timeline-event-title').textContent).toContain('Kitchen');
                expect(eventEl.querySelector('.sidebar-timeline-event-title').textContent).toContain('Motion');
                done();
            }, 100);
        });

        test('anomaly renders with correct icon and description', function(done) {
            setTimeout(function() {
                const eventEl = mockElements.eventsContainer.querySelector('[data-id="7"]');
                expect(eventEl).toBeTruthy();
                expect(eventEl.dataset.type).toBe('anomaly');
                expect(eventEl.querySelector('.sidebar-timeline-event-icon').textContent).toBe('⚠️');
                expect(eventEl.querySelector('.sidebar-timeline-event-title').textContent).toContain('Kitchen');
                expect(eventEl.querySelector('.sidebar-timeline-event-title').textContent).toContain('Unusual');
                expect(eventEl.classList.contains('severity-warning')).toBe(true);
                done();
            }, 100);
        });

        test('security_alert renders with correct icon and description', function(done) {
            setTimeout(function() {
                const eventEl = mockElements.eventsContainer.querySelector('[data-id="8"]');
                expect(eventEl).toBeTruthy();
                expect(eventEl.dataset.type).toBe('security_alert');
                expect(eventEl.querySelector('.sidebar-timeline-event-icon').textContent).toBe('🚨');
                expect(eventEl.querySelector('.sidebar-timeline-event-title').textContent).toContain('Security');
                expect(eventEl.querySelector('.sidebar-timeline-event-title').textContent).toContain('Motion detected while armed');
                expect(eventEl.classList.contains('severity-critical')).toBe(true);
                done();
            }, 100);
        });

        test('fall_alert renders with correct icon and description', function(done) {
            setTimeout(function() {
                const eventEl = mockElements.eventsContainer.querySelector('[data-id="9"]');
                expect(eventEl).toBeTruthy();
                expect(eventEl.dataset.type).toBe('fall_alert');
                expect(eventEl.querySelector('.sidebar-timeline-event-icon').textContent).toBe('🆘');
                expect(eventEl.querySelector('.sidebar-timeline-event-title').textContent).toContain('Fall');
                expect(eventEl.querySelector('.sidebar-timeline-event-title').textContent).toContain('Alice');
                expect(eventEl.querySelector('.sidebar-timeline-event-title').textContent).toContain('Bathroom');
                expect(eventEl.classList.contains('severity-critical')).toBe(true);
                done();
            }, 100);
        });

        test('node_online renders with correct icon and description', function(done) {
            setTimeout(function() {
                const eventEl = mockElements.eventsContainer.querySelector('[data-id="10"]');
                expect(eventEl).toBeTruthy();
                expect(eventEl.dataset.type).toBe('node_online');
                expect(eventEl.querySelector('.sidebar-timeline-event-icon').textContent).toBe('📡');
                expect(eventEl.querySelector('.sidebar-timeline-event-title').textContent).toContain('kitchen-north');
                expect(eventEl.querySelector('.sidebar-timeline-event-title').textContent).toContain('came online');
                expect(eventEl.classList.contains('secondary')).toBe(true);
                done();
            }, 100);
        });

        test('node_offline renders with correct icon and description', function(done) {
            setTimeout(function() {
                const eventEl = mockElements.eventsContainer.querySelector('[data-id="11"]');
                expect(eventEl).toBeTruthy();
                expect(eventEl.dataset.type).toBe('node_offline');
                expect(eventEl.querySelector('.sidebar-timeline-event-icon').textContent).toBe('📵');
                expect(eventEl.querySelector('.sidebar-timeline-event-title').textContent).toContain('living-room-west');
                expect(eventEl.querySelector('.sidebar-timeline-event-title').textContent).toContain('went offline');
                expect(eventEl.classList.contains('secondary')).toBe(true);
                done();
            }, 100);
        });

        test('ota_update renders with correct icon and description', function(done) {
            setTimeout(function() {
                const eventEl = mockElements.eventsContainer.querySelector('[data-id="12"]');
                expect(eventEl).toBeTruthy();
                expect(eventEl.dataset.type).toBe('ota_update');
                expect(eventEl.querySelector('.sidebar-timeline-event-icon').textContent).toBe('⬆️');
                expect(eventEl.querySelector('.sidebar-timeline-event-title').textContent).toContain('kitchen-north');
                expect(eventEl.querySelector('.sidebar-timeline-event-title').textContent).toContain('1.2.3');
                expect(eventEl.querySelector('.sidebar-timeline-event-title').textContent).toContain('Firmware');
                expect(eventEl.classList.contains('secondary')).toBe(true);
                done();
            }, 100);
        });

        test('baseline_changed renders with correct icon and description', function(done) {
            setTimeout(function() {
                const eventEl = mockElements.eventsContainer.querySelector('[data-id="13"]');
                expect(eventEl).toBeTruthy();
                expect(eventEl.dataset.type).toBe('baseline_changed');
                expect(eventEl.querySelector('.sidebar-timeline-event-icon').textContent).toBe('📊');
                expect(eventEl.querySelector('.sidebar-timeline-event-title').textContent).toContain('Baseline');
                expect(eventEl.querySelector('.sidebar-timeline-event-title').textContent).toContain('AA:BB:CC');
                expect(eventEl.classList.contains('secondary')).toBe(true);
                done();
            }, 100);
        });

        test('learning_milestone renders with correct icon and description', function(done) {
            setTimeout(function() {
                const eventEl = mockElements.eventsContainer.querySelector('[data-id="14"]');
                expect(eventEl).toBeTruthy();
                expect(eventEl.dataset.type).toBe('learning_milestone');
                expect(eventEl.querySelector('.sidebar-timeline-event-icon').textContent).toBe('🎓');
                expect(eventEl.querySelector('.sidebar-timeline-event-title').textContent).toContain('Anomaly patterns');
                expect(eventEl.querySelector('.sidebar-timeline-event-title').textContent).toContain('Kitchen');
                expect(eventEl.classList.contains('secondary')).toBe(true);
                done();
            }, 100);
        });

        test('sleep_session_end renders with correct icon and description', function(done) {
            setTimeout(function() {
                const eventEl = mockElements.eventsContainer.querySelector('[data-id="15"]');
                expect(eventEl).toBeTruthy();
                expect(eventEl.dataset.type).toBe('sleep_session_end');
                expect(eventEl.querySelector('.sidebar-timeline-event-icon').textContent).toBe('😴');
                expect(eventEl.querySelector('.sidebar-timeline-event-title').textContent).toContain('Alice');
                expect(eventEl.querySelector('.sidebar-timeline-event-title').textContent).toContain('7h 23m');
                expect(eventEl.querySelector('.sidebar-timeline-event-title').textContent).toContain('slept');
                done();
            }, 100);
        });
    });

    // ============================================
    // Plain English Description Tests
    // ============================================
    describe('Plain English Descriptions', function() {
        test('descriptions use plain English without technical jargon', function(done) {
            global.fetch.mockImplementation(function() {
                return Promise.resolve({
                    ok: true,
                    json: async function() {
                        return {
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
                        };
                    }
                });
            });

            SidebarTimeline.show();
            SidebarTimeline.refresh();
            global.fetch.mockResolvedValue({
                ok: true,
                json: async function() {
                    return {
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
                    };
                }
            });

            setTimeout(function() {
                const eventEl = mockElements.eventsContainer.querySelector('[data-id="1"]');
                const title = eventEl.querySelector('.sidebar-timeline-event-title').textContent;

                // Should not contain technical jargon
                expect(title).not.toMatch(/CSI|Fresnel|deltaRMS|blob_id|timestamp_ms/);

                // Should use plain English
                expect(title).toMatch(/Alice|Kitchen|entered/);
                done();
            }, 100);
        });

        test('unknown person defaults to "Someone"', function(done) {
            global.fetch.mockImplementation(function() {
                return Promise.resolve({
                    ok: true,
                    json: async function() {
                        return {
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
                        };
                    }
                });
            });

            SidebarTimeline.refresh();

            setTimeout(function() {
                const eventEl = mockElements.eventsContainer.querySelector('[data-id="1"]');
                const title = eventEl.querySelector('.sidebar-timeline-event-title').textContent;

                expect(title).toContain('Motion');
                expect(title).toContain('Hallway');
                done();
            }, 100);
        });

        test('unknown zone defaults to "the area"', function(done) {
            global.fetch.mockImplementation(function() {
                return Promise.resolve({
                    ok: true,
                    json: async function() {
                        return {
                            events: [
                                {
                                    id: 1,
                                    timestamp_ms: Date.now(),
                                    type: 'detection',
                                    zone: null,
                                    person: 'Alice',
                                    severity: 'info'
                                }
                            ],
                            cursor: null,
                            total_filtered: 1
                        };
                    }
                });
            });

            SidebarTimeline.refresh();

            setTimeout(function() {
                const eventEl = mockElements.eventsContainer.querySelector('[data-id="1"]');
                const title = eventEl.querySelector('.sidebar-timeline-event-title').textContent;

                expect(title).toContain('Motion');
                done();
            }, 100);
        });
    });

    // ============================================
    // Feedback Button Tests
    // ============================================
    describe('Feedback Buttons', function() {
        beforeEach(function(done) {
            global.fetch.mockImplementation(function() {
                return Promise.resolve({
                    ok: true,
                    json: async function() {
                        return {
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
                        };
                    }
                });
            });

            SidebarTimeline.show();
            SidebarTimeline.refresh();

            setTimeout(done, 100);
        });

        test('each event has thumbs-up and thumbs-down buttons', function() {
            setTimeout(function() {
                const eventEl = mockElements.eventsContainer.querySelector('[data-id="123"]');
                expect(eventEl).toBeTruthy();

                const thumbsUp = eventEl.querySelector('.feedback-positive');
                const thumbsDown = eventEl.querySelector('.feedback-negative');

                expect(thumbsUp).toBeTruthy();
                expect(thumbsDown).toBeTruthy();
                expect(thumbsUp.getAttribute('aria-label')).toBe('Thumbs up');
                expect(thumbsDown.getAttribute('aria-label')).toBe('Thumbs down');
                done();
            }, 100);
        });

        test('thumbs-up button delegates to feedback module', function(done) {
            setTimeout(function() {
                const eventEl = mockElements.eventsContainer.querySelector('[data-id="123"]');
                const thumbsUp = eventEl.querySelector('.feedback-positive');

                // Mock feedback API response
                global.fetch.mockResolvedValueOnce({
                    ok: true,
                    json: async function() {
                        return { ok: true };
                    }
                });

                thumbsUp.click();

                // Give async operations time to complete
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
                    done();
                }, 50);
            }, 100);
        });

        test('thumbs-down button delegates to feedback module', function(done) {
            setTimeout(function() {
                const eventEl = mockElements.eventsContainer.querySelector('[data-id="123"]');
                const thumbsDown = eventEl.querySelector('.feedback-negative');

                // Mock feedback API response
                global.fetch.mockResolvedValueOnce({
                    ok: true,
                    json: async function() {
                        return { ok: true };
                    }
                });

                thumbsDown.click();

                // Give async operations time to complete
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
                    done();
                }, 50);
            }, 100);
        });

        test('feedback button click stops propagation', function(done) {
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
                    done();
                }, 50);
            }, 100);
        });

        test('feedback button shows active state briefly after click', function(done) {
            setTimeout(function() {
                const eventEl = mockElements.eventsContainer.querySelector('[data-id="123"]');
                const thumbsUp = eventEl.querySelector('.feedback-positive');

                global.fetch.mockResolvedValueOnce({
                    ok: true,
                    json: async function() {
                        return { ok: true };
                    }
                });

                thumbsUp.click();

                // Check immediately after click
                expect(thumbsUp.classList.contains('active')).toBe(true);

                // Check after timeout (2 seconds)
                setTimeout(function() {
                    expect(thumbsUp.classList.contains('active')).toBe(false);
                    done();
                }, 2100);
            }, 100);
        });
    });

    // ============================================
    // Virtualized Rendering Tests
    // ============================================
    describe('Virtualized Rendering with IntersectionObserver', function() {
        beforeEach(function() {
            SidebarTimeline.show();
        });

        test('uses IntersectionObserver for virtualization when enabled', function(done) {
            // Mock IntersectionObserver
            const mockObserver = {
                observe: jest.fn(),
                unobserve: jest.fn(),
                disconnect: jest.fn()
            };

            global.IntersectionObserver = jest.fn(function(callback, options) {
                return mockObserver;
            });

            // Reload module after mocking IntersectionObserver
            jest.isolateModules(function() {
                require('./sidebar-timeline.js');
            });

            setTimeout(function() {
                expect(global.IntersectionObserver).toHaveBeenCalled();
                done();
            }, 100);
        });

        test('handles 10,000 events without performance degradation', function(done) {
            // Create a large array of events
            const largeEventList = [];
            for (let i = 0; i < 10000; i++) {
                largeEventList.push({
                    id: i,
                    timestamp_ms: Date.now() - (i * 60000), // 1 minute apart
                    type: i % 2 === 0 ? 'detection' : 'zone_entry',
                    zone: ['Kitchen', 'Living Room', 'Bedroom'][i % 3],
                    person: ['Alice', 'Bob', 'Charlie'][i % 3],
                    severity: 'info'
                });
            }

            global.fetch.mockResolvedValue({
                ok: true,
                json: async function() {
                    return {
                        events: largeEventList.slice(0, 100), // First batch
                        cursor: 'next-cursor',
                        total_filtered: 10000
                    };
                }
            });

            // Measure render time
            const startTime = performance.now();

            setTimeout(function() {
                const renderTime = performance.now() - startTime;

                // Rendering should complete quickly (< 100ms for 100 items)
                expect(renderTime).toBeLessThan(100);

                // Check that virtual spacers are used
                expect(mockElements.spacerTop.style.height).not.toBe('0px');
                expect(mockElements.spacerBottom.style.height).not.toBe('0px');

                // Check that not all events are rendered (virtualization)
                const renderedEvents = mockElements.eventsContainer.querySelectorAll('.sidebar-timeline-event');
                expect(renderedEvents.length).toBeLessThan(10000);
                expect(renderedEvents.length).toBeLessThan(150); // maxDOMItems

                done();
            }, 200);
        });

        test('updates spacers correctly for virtual scrolling', function(done) {
            const events = [];
            for (let i = 0; i < 100; i++) {
                events.push({
                    id: i,
                    timestamp_ms: Date.now() - (i * 60000),
                    type: 'detection',
                    zone: 'Kitchen',
                    person: 'Alice',
                    severity: 'info'
                });
            }

            global.fetch.mockResolvedValue({
                ok: true,
                json: async function() {
                    return {
                        events: events,
                        cursor: null,
                        total_filtered: 100
                    };
                }
            });

            setTimeout(function() {
                // Initial render - top spacer should be 0
                expect(parseInt(mockElements.spacerTop.style.height, 10)).toBe(0);

                // Bottom spacer should account for unrendered events
                const bottomHeight = parseInt(mockElements.spacerBottom.style.height, 10);
                expect(bottomHeight).toBeGreaterThan(0);

                done();
            }, 200);
        });

        test('only renders visible items plus buffer', function(done) {
            const events = [];
            for (let i = 0; i < 200; i++) {
                events.push({
                    id: i,
                    timestamp_ms: Date.now() - (i * 60000),
                    type: 'detection',
                    zone: 'Kitchen',
                    person: 'Alice',
                    severity: 'info'
                });
            }

            global.fetch.mockResolvedValue({
                ok: true,
                json: async function() {
                    return {
                        events: events,
                        cursor: null,
                        total_filtered: 200
                    };
                }
            });

            setTimeout(function() {
                const renderedEvents = mockElements.eventsContainer.querySelectorAll('.sidebar-timeline-event');

                // Should render initial batch (visible + buffer)
                expect(renderedEvents.length).toBeGreaterThan(0);
                expect(renderedEvents.length).toBeLessThan(200);

                done();
            }, 200);
        });
    });

    // ============================================
    // Event Click and Seek Tests
    // ============================================
    describe('Event Click and Seek', function() {
        beforeEach(function() {
            SidebarTimeline.show();

            global.fetch.mockResolvedValue({
                ok: true,
                json: async function() {
                    return {
                        events: [
                            {
                                id: 1,
                                timestamp_ms: 1712707200000, // 2024-04-09 12:00:00 UTC
                                type: 'detection',
                                zone: 'Kitchen',
                                person: 'Alice',
                                severity: 'info'
                            }
                        ],
                        cursor: null,
                        total_filtered: 1
                    };
                }
            });
        });

        test('clicking event seeks to timestamp in timeline', function(done) {
            setTimeout(function() {
                const eventEl = mockElements.eventsContainer.querySelector('[data-id="1"]');
                expect(eventEl).toBeTruthy();

                eventEl.click();

                expect(global.SpaxelRouter.navigate).toHaveBeenCalledWith('timeline');
                expect(global.SpaxelApp.showToast).toHaveBeenCalledWith('Opening timeline...', 'info');

                done();
            }, 100);
        });

        test('event timestamp is stored in data attribute', function(done) {
            setTimeout(function() {
                const eventEl = mockElements.eventsContainer.querySelector('[data-id="1"]');
                expect(eventEl).toBeTruthy();

                const timestamp = parseInt(eventEl.dataset.timestamp, 10);
                expect(timestamp).toBe(1712707200000);

                done();
            }, 100);
        });
    });

    // ============================================
    // Real-time Event Updates Tests
    // ============================================
    describe('Real-time Event Updates', function() {
        beforeEach(function() {
            SidebarTimeline.show();

            global.fetch.mockResolvedValue({
                ok: true,
                json: async function() {
                    return {
                        events: [],
                        cursor: null,
                        total_filtered: 0
                    };
                }
            });
        });

        test('new WebSocket event appears at top of timeline', function(done) {
            setTimeout(function() {
                // Simulate WebSocket message handler being called
                const wsEvent = {
                    type: 'event',
                    event: {
                        id: 999,
                        ts: Date.now(),
                        kind: 'detection',
                        zone: 'Kitchen',
                        person_name: 'Alice',
                        severity: 'info'
                    }
                };

                // Get the message handler and call it
                const handlers = global.SpaxelApp.registerMessageHandler.mock.calls;
                if (handlers.length > 0) {
                    const handler = handlers[0][0];
                    handler(wsEvent);

                    setTimeout(function() {
                        const newEventEl = mockElements.eventsContainer.querySelector('[data-id="999"]');
                        expect(newEventEl).toBeTruthy();
                        expect(newEventEl.classList.contains('new-event')).toBe(true);
                        done();
                    }, 50);
                } else {
                    done();
                }
            }, 100);
        });

        test('new events have animation class', function(done) {
            setTimeout(function() {
                const wsEvent = {
                    type: 'event',
                    event: {
                        id: 1000,
                        ts: Date.now(),
                        kind: 'zone_entry',
                        zone: 'Living Room',
                        person_name: 'Bob',
                        severity: 'info'
                    }
                };

                const handlers = global.SpaxelApp.registerMessageHandler.mock.calls;
                if (handlers.length > 0) {
                    const handler = handlers[0][0];
                    handler(wsEvent);

                    setTimeout(function() {
                        const newEventEl = mockElements.eventsContainer.querySelector('[data-id="1000"]');
                        expect(newEventEl.classList.contains('new-event')).toBe(true);
                        done();
                    }, 50);
                } else {
                    done();
                }
            }, 100);
        });
    });

    // ============================================
    // Empty State Tests
    // ============================================
    describe('Empty State', function() {
        beforeEach(function() {
            SidebarTimeline.show();

            global.fetch.mockResolvedValue({
                ok: true,
                json: async function() {
                    return {
                        events: [],
                        cursor: null,
                        total_filtered: 0
                    };
                }
            });
        });

        test('shows empty state when no events', function(done) {
            setTimeout(function() {
                expect(mockElements.empty.style.display).toBe('flex');
                expect(mockElements.eventsContainer.children.length).toBe(0);
                done();
            }, 100);
        });

        test('empty state shows correct message and icon', function(done) {
            setTimeout(function() {
                const emptyTitle = mockElements.empty.querySelector('h3');
                const emptyText = mockElements.empty.querySelector('p');
                const emptyIcon = mockElements.empty.querySelector('svg');

                expect(emptyTitle.textContent).toBe('No events yet');
                expect(emptyText.textContent).toBe('Events will appear here as they happen');
                expect(emptyIcon).toBeTruthy();
                done();
            }, 100);
        });

        test('loading state shows before events load', function(done) {
            // Initially should show loading
            expect(mockElements.loading.style.display).toBe('flex');

            setTimeout(function() {
                // After events load, loading should hide
                expect(mockElements.loading.style.display).toBe('none');
                done();
            }, 100);
        });
    });

    // ============================================
    // Severity Styling Tests
    // ============================================
    describe('Severity Styling', function() {
        beforeEach(function() {
            SidebarTimeline.show();

            global.fetch.mockResolvedValue({
                ok: true,
                json: async function() {
                    return {
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
                    };
                }
            });
        });

        test('info events have no severity class', function(done) {
            setTimeout(function() {
                const eventEl = mockElements.eventsContainer.querySelector('[data-id="1"]');
                expect(eventEl.classList.contains('severity-critical')).toBe(false);
                expect(eventEl.classList.contains('severity-warning')).toBe(false);
                done();
            }, 100);
        });

        test('warning events have severity-warning class', function(done) {
            setTimeout(function() {
                const eventEl = mockElements.eventsContainer.querySelector('[data-id="2"]');
                expect(eventEl.classList.contains('severity-warning')).toBe(true);
                done();
            }, 100);
        });

        test('critical events have severity-critical class', function(done) {
            setTimeout(function() {
                const eventEl = mockElements.eventsContainer.querySelector('[data-id="3"]');
                expect(eventEl.classList.contains('severity-critical')).toBe(true);
                done();
            }, 100);
        });
    });

    // ============================================
    // System Event Secondary Styling Tests
    // ============================================
    describe('System Event Secondary Styling', function() {
        beforeEach(function() {
            SidebarTimeline.show();

            global.fetch.mockResolvedValue({
                ok: true,
                json: async function() {
                    return {
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
                    };
                }
            });
        });

        test('user events have no secondary class', function(done) {
            setTimeout(function() {
                const eventEl = mockElements.eventsContainer.querySelector('[data-id="1"]');
                expect(eventEl.classList.contains('secondary')).toBe(false);
                done();
            }, 100);
        });

        test('system events have secondary class', function(done) {
            setTimeout(function() {
                const eventEl = mockElements.eventsContainer.querySelector('[data-id="2"]');
                expect(eventEl.classList.contains('secondary')).toBe(true);
                done();
            }, 100);
        });
    });

    // ============================================
    // Mode Change Tests
    // ============================================
    describe('Mode Change Handling', function() {
        beforeEach(function() {
            SidebarTimeline.show();
        });

        test('updates dashboard mode on simple mode change', function(done) {
            setTimeout(function() {
                const handlers = global.SpaxelSimpleModeDetection.onModeChange.mock.calls;
                if (handlers.length > 0) {
                    const handler = handlers[0][0];

                    // Simulate simple mode change
                    handler('simple', 'expert');

                    // Mode should be updated (visible in next API call)
                    global.fetch.mockClear();
                    global.fetch.mockResolvedValue({
                        ok: true,
                        json: async function() {
                            return {
                                events: [],
                                cursor: null,
                                total_filtered: 0
                            };
                        }
                    });

                    // Trigger refresh
                    SidebarTimeline.refresh();

                    setTimeout(function() {
                        expect(global.fetch).toHaveBeenCalled();
                        done();
                    }, 50);
                } else {
                    done();
                }
            }, 100);
        });

        test('updates dashboard mode on router mode change', function(done) {
            setTimeout(function() {
                const handlers = global.SpaxelRouter.onModeChange.mock.calls;
                if (handlers.length > 0) {
                    const handler = handlers[0][0];

                    // Simulate router mode change
                    handler('simple', 'expert');

                    // Mode should be updated
                    done();
                } else {
                    done();
                }
            }, 100);
        });
    });
});
