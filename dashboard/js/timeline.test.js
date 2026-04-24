/**
 * Tests for Activity Timeline tap-to-jump time-travel
 *
 * Covers:
 * - Clicking event emits jumpToTime with correct timestamp
 * - Selected event highlights with timeline-event-selected class
 * - Clicking new event clears previous selection
 * - Now replaying chip appears in timeline header
 * - hideNowReplayingChip hides the chip and clears replay state
 * - clearSelection removes selected class from event
 * - Simple mode does not trigger replay jump
 * - Cross-module coordination: sidebar selection cleared on timeline jump
 */

'use strict';

// Mock dependencies
var mockJumpToTime;
global.fetch = jest.fn(function() {
    return Promise.resolve({
        ok: true,
        json: function() {
            return Promise.resolve({ events: [], cursor: null, total_filtered: 0 });
        }
    });
});

// Mock Feedback module (unused but referenced in module)
global.Feedback = { sendFeedback: jest.fn() };

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

// Mock SpaxelSimpleModeDetection
global.SpaxelSimpleModeDetection = {
    onModeChange: jest.fn()
};

// Mock SpaxelSidebarTimeline
global.SpaxelSidebarTimeline = {
    clearSelection: jest.fn(),
    hideNowReplayingChip: jest.fn()
};

// Mock IntersectionObserver
global.IntersectionObserver = jest.fn(function(callback, options) {
    this.observe = jest.fn();
    this.unobserve = jest.fn();
    this.disconnect = jest.fn();
    this.callback = callback;
    this.options = options;
});

describe('Timeline tap-to-jump', function() {
    var Timeline;
    var mockElements;
    var testEvents;

    beforeEach(function() {
        jest.clearAllMocks();

        mockJumpToTime = jest.fn(function() {
            return Promise.resolve({
                session_id: 'test-session-1',
                timestamp_ms: 1710519800000,
                from_ms: 1710519795000,
                to_ms: 1710519805000,
                state: 'paused'
            });
        });

        global.SpaxelReplay = {
            jumpToTime: mockJumpToTime,
            onJumpToTime: jest.fn(),
            isReplayMode: jest.fn(function() { return false; }),
            getSession: jest.fn(function() {
                return { id: 'test-session-1', fromMs: 1710519795000, toMs: 1710519805000, currentMs: 1710519800000, speed: 1, state: 'paused' };
            })
        };

        // Setup DOM structure matching live.html
        document.body.innerHTML = `
            <div id="timeline-view" style="display: none;">
                <div class="timeline-header">
                    <h2 class="timeline-title">Activity Timeline</h2>
                    <div id="timeline-now-replaying" class="timeline-now-replaying" style="display: none;">
                        <span class="now-replaying-dot"></span>
                        <span class="now-replaying-text"></span>
                    </div>
                </div>
                <div class="timeline-filter-toggle">Filter</div>
                <div id="timeline-filter-bar" class="timeline-filter-bar">
                    <select id="timeline-filter-type"><option value="">All Types</option></select>
                    <select id="timeline-filter-zone"><option value="">All Zones</option></select>
                    <select id="timeline-filter-person"><option value="">All People</option></select>
                    <select id="timeline-filter-time"><option value="">All Time</option></select>
                    <input id="timeline-filter-search" type="text" placeholder="Search events...">
                    <div id="timeline-custom-date-container" style="display: none;">
                        <input id="timeline-date-from" type="date">
                        <input id="timeline-date-to" type="date">
                        <button id="timeline-date-apply">Apply</button>
                    </div>
                </div>
                <div class="timeline-category-filters">
                    <label><input type="checkbox" id="timeline-category-presence" checked> Presence</label>
                    <label><input type="checkbox" id="timeline-category-zones" checked> Zones</label>
                    <label><input type="checkbox" id="timeline-category-alerts" checked> Alerts</label>
                    <label><input type="checkbox" id="timeline-category-system" checked> System</label>
                    <label><input type="checkbox" id="timeline-category-learning" checked> Learning</label>
                </div>
                <div id="timeline-events" class="timeline-events-list"></div>
                <div id="timeline-loading" style="display: none;">Loading...</div>
                <div id="timeline-empty" style="display: none;">No events</div>
                <div id="timeline-error" style="display: none;">Error</div>
                <div id="timeline-load-more" style="display: none;">
                    <button id="timeline-load-more-btn">Load More</button>
                </div>
                <div id="timeline-count"></div>
            </div>
        `;

        // Load the module fresh
        jest.isolateModules(function() {
            require('./timeline.js');
            Timeline = window.SpaxelTimeline;
        });

        testEvents = [
            {
                id: 100,
                timestamp_ms: 1710519800000,
                type: 'zone_entry',
                zone: 'Kitchen',
                person: 'Alice',
                blob_id: 0,
                detail_json: '',
                severity: 'info'
            },
            {
                id: 200,
                timestamp_ms: 1710519860000,
                type: 'zone_exit',
                zone: 'Kitchen',
                person: 'Alice',
                blob_id: 0,
                detail_json: '',
                severity: 'info'
            },
            {
                id: 300,
                timestamp_ms: 1710519920000,
                type: 'fall_alert',
                zone: 'Bathroom',
                person: 'Bob',
                blob_id: 42,
                detail_json: JSON.stringify({ description: 'Fall detected in Bathroom' }),
                severity: 'critical'
            }
        ];
    });

    afterEach(function() {
        document.body.innerHTML = '';
        delete window.SpaxelTimeline;
        delete window.SpaxelReplay;
    });

    // Helper: load events and return a promise that resolves when rendered
    function loadEvents(events) {
        global.fetch.mockImplementation(function() {
            return Promise.resolve({
                ok: true,
                json: function() {
                    return Promise.resolve({
                        events: events,
                        cursor: null,
                        total_filtered: events.length
                    });
                }
            });
        });

        Timeline.refresh();
        return new Promise(function(resolve) {
            setTimeout(resolve, 150);
        });
    }

    // ============================================
    // Tap-to-Jump: correct timestamp emission
    // ============================================
    describe('timestamp emission', function() {
        test('clicking event calls SpaxelReplay.jumpToTime with event timestamp', function() {
            return loadEvents(testEvents).then(function() {
                var eventEl = document.querySelector('[data-id="100"]');
                expect(eventEl).toBeTruthy();

                eventEl.click();

                expect(mockJumpToTime).toHaveBeenCalledTimes(1);
                expect(mockJumpToTime).toHaveBeenCalledWith(1710519800000, 5000);
            });
        });

        test('clicking different events emits correct timestamps', function() {
            return loadEvents(testEvents).then(function() {
                var first = document.querySelector('[data-id="100"]');
                var second = document.querySelector('[data-id="200"]');

                first.click();
                expect(mockJumpToTime).toHaveBeenCalledWith(1710519800000, 5000);

                second.click();
                expect(mockJumpToTime).toHaveBeenCalledWith(1710519860000, 5000);

                expect(mockJumpToTime).toHaveBeenCalledTimes(2);
            });
        });

        test('seek button on event card calls jumpToTime', function() {
            return loadEvents(testEvents).then(function() {
                var seekBtn = document.querySelector('[data-id="100"] .timeline-seek-btn');
                expect(seekBtn).toBeTruthy();

                seekBtn.click();

                expect(mockJumpToTime).toHaveBeenCalledWith(1710519800000, 5000);
            });
        });
    });

    // ============================================
    // Selected event highlighting
    // ============================================
    describe('event highlighting', function() {
        test('clicked event gets timeline-event-selected class', function() {
            return loadEvents(testEvents).then(function() {
                var eventEl = document.querySelector('[data-id="100"]');
                expect(eventEl.classList.contains('timeline-event-selected')).toBe(false);

                eventEl.click();

                return new Promise(function(resolve) {
                    setTimeout(function() {
                        expect(eventEl.classList.contains('timeline-event-selected')).toBe(true);
                        resolve();
                    }, 50);
                });
            });
        });

        test('clicking new event clears previous selection', function() {
            return loadEvents(testEvents).then(function() {
                var first = document.querySelector('[data-id="100"]');
                var second = document.querySelector('[data-id="200"]');

                first.click();

                return new Promise(function(resolve) {
                    setTimeout(function() {
                        expect(first.classList.contains('timeline-event-selected')).toBe(true);

                        second.click();

                        setTimeout(function() {
                            expect(first.classList.contains('timeline-event-selected')).toBe(false);
                            expect(second.classList.contains('timeline-event-selected')).toBe(true);
                            resolve();
                        }, 50);
                    }, 50);
                });
            });
        });

        test('clearSelection removes selected class', function() {
            return loadEvents(testEvents).then(function() {
                var eventEl = document.querySelector('[data-id="100"]');
                eventEl.click();

                return new Promise(function(resolve) {
                    setTimeout(function() {
                        expect(eventEl.classList.contains('timeline-event-selected')).toBe(true);
                        Timeline.clearSelection();
                        expect(eventEl.classList.contains('timeline-event-selected')).toBe(false);
                        resolve();
                    }, 50);
                });
            });
        });
    });

    // ============================================
    // Now replaying chip
    // ============================================
    describe('Now replaying chip', function() {
        test('chip appears in timeline header after jump', function() {
            return loadEvents(testEvents).then(function() {
                var eventEl = document.querySelector('[data-id="100"]');
                eventEl.click();

                return new Promise(function(resolve) {
                    setTimeout(function() {
                        var chip = document.getElementById('timeline-now-replaying');
                        expect(chip).toBeTruthy();
                        expect(chip.style.display).not.toBe('none');
                        expect(chip.textContent).toContain('Now replaying');
                        resolve();
                    }, 100);
                });
            });
        });

        test('hideNowReplayingChip hides the chip', function() {
            return loadEvents(testEvents).then(function() {
                var eventEl = document.querySelector('[data-id="100"]');
                eventEl.click();

                return new Promise(function(resolve) {
                    setTimeout(function() {
                        var chip = document.getElementById('timeline-now-replaying');
                        expect(chip.style.display).not.toBe('none');

                        Timeline.hideNowReplayingChip();
                        expect(chip.style.display).toBe('none');
                        resolve();
                    }, 100);
                });
            });
        });

        test('hideNowReplayingChip clears replay state and selection', function() {
            return loadEvents(testEvents).then(function() {
                var eventEl = document.querySelector('[data-id="100"]');
                eventEl.click();

                return new Promise(function(resolve) {
                    setTimeout(function() {
                        expect(eventEl.classList.contains('timeline-event-selected')).toBe(true);

                        Timeline.hideNowReplayingChip();
                        expect(eventEl.classList.contains('timeline-event-selected')).toBe(false);
                        resolve();
                    }, 100);
                });
            });
        });
    });

    // ============================================
    // Cross-module coordination
    // ============================================
    describe('cross-module coordination', function() {
        test('jump clears sidebar timeline selection', function() {
            return loadEvents(testEvents).then(function() {
                var eventEl = document.querySelector('[data-id="100"]');
                eventEl.click();

                return new Promise(function(resolve) {
                    setTimeout(function() {
                        expect(global.SpaxelSidebarTimeline.clearSelection).toHaveBeenCalled();
                        resolve();
                    }, 100);
                });
            });
        });

        test('navigates to replay mode via router', function() {
            return loadEvents(testEvents).then(function() {
                var eventEl = document.querySelector('[data-id="100"]');
                eventEl.click();

                return new Promise(function(resolve) {
                    setTimeout(function() {
                        expect(global.SpaxelRouter.navigate).toHaveBeenCalledWith('replay');
                        resolve();
                    }, 100);
                });
            });
        });
    });

    // ============================================
    // Simple mode gating
    // ============================================
    describe('simple mode gating', function() {
        test('clicking event does not trigger jump in simple mode', function() {
            // Simulate simple mode by triggering the registered router callback
            var modeCallback = null;
            var calls = global.SpaxelRouter.onModeChange.mock.calls;
            if (calls.length > 0) {
                modeCallback = calls[0][0];
            }
            if (modeCallback) {
                modeCallback('simple');
            }

            return loadEvents(testEvents).then(function() {
                var eventEl = document.querySelector('[data-id="100"]');
                if (eventEl) {
                    eventEl.click();
                    // In simple mode, jumpToTime should NOT be called
                    expect(mockJumpToTime).not.toHaveBeenCalled();
                }
            });
        });
    });

    // ============================================
    // Error handling
    // ============================================
    describe('error handling', function() {
        test('jumpToTime failure shows error toast', function() {
            mockJumpToTime.mockImplementation(function() {
                return Promise.reject(new Error('Network error'));
            });

            return loadEvents(testEvents).then(function() {
                var eventEl = document.querySelector('[data-id="100"]');
                eventEl.click();

                return new Promise(function(resolve) {
                    setTimeout(function() {
                        expect(global.SpaxelApp.showToast).toHaveBeenCalledWith(
                            expect.stringContaining('Failed'),
                            'warning'
                        );
                        resolve();
                    }, 100);
                });
            });
        });

        test('missing SpaxelReplay shows warning toast', function() {
            delete window.SpaxelReplay;

            return loadEvents(testEvents).then(function() {
                var eventEl = document.querySelector('[data-id="100"]');
                eventEl.click();

                expect(global.SpaxelApp.showToast).toHaveBeenCalledWith(
                    'Replay module not available',
                    'warning'
                );
            });
        });
    });
});
