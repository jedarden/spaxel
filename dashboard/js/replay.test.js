/**
 * Tests for SpaxelReplay time-travel module
 *
 * Covers:
 * - jumpToTime creates replay session and fetches blobs
 * - exitReplayMode cleans up state and notifies Viz3D
 * - pauseLiveMode enters replay with correct time window
 * - Timeline loop polls for session state during playback
 * - Scrubber updates position based on session state
 */

'use strict';

// Mock DOM elements
function setupDOM() {
    document.body.innerHTML = `
        <div id="status-bar"></div>
        <div id="scene-container"></div>
    `;
}

// Mock Viz3D on both global and window
var viz3dMock = {
    enterReplayMode: jest.fn(),
    exitReplayMode: jest.fn(),
    updateReplayBlobs: jest.fn()
};
global.Viz3D = viz3dMock;

// Mock SpaxelApp on both global and window
var appMock = {
    showToast: jest.fn()
};
global.SpaxelApp = appMock;

// Mock SpaxelSidebarTimeline on both global and window
var sidebarMock = {
    clearSelection: jest.fn(),
    hideNowReplayingChip: jest.fn()
};
global.SpaxelSidebarTimeline = sidebarMock;

// Mock SpaxelRouter on both global and window
var routerMock = {
    navigate: jest.fn()
};
global.SpaxelRouter = routerMock;

describe('SpaxelReplay', function() {
    let Replay;
    let fetchMock;

    beforeEach(function() {
        jest.clearAllMocks();
        setupDOM();

        // Default fetch mock returns store info
        fetchMock = jest.fn(function(url, options) {
            if (url === '/api/replay/sessions') {
                return Promise.resolve({
                    ok: true,
                    json: function() {
                        return Promise.resolve({
                            has_data: true,
                            oldest_timestamp_ms: Date.now() - 3600000,
                            newest_timestamp_ms: Date.now(),
                            sessions: []
                        });
                    }
                });
            }

            if (url === '/api/replay/jump-to-time') {
                return Promise.resolve({
                    ok: true,
                    json: function() {
                        return Promise.resolve({
                            session_id: 'jump-session-1',
                            timestamp_ms: 1710519800000,
                            from_ms: 1710519795000,
                            to_ms: 1710519805000,
                            state: 'paused'
                        });
                    }
                });
            }

            // Session state endpoint (with blobs)
            var sessionMatch = url.match(/\/api\/replay\/session\/(.+)$/);
            if (sessionMatch) {
                return Promise.resolve({
                    ok: true,
                    json: function() {
                        return Promise.resolve({
                            session_id: sessionMatch[1],
                            current_ms: 1710519800000,
                            from_ms: 1710519795000,
                            to_ms: 1710519805000,
                            state: 'paused',
                            speed: 1,
                            blobs: [
                                {
                                    id: 1,
                                    x: 2.5,
                                    y: 1.3,
                                    z: 0.8,
                                    vx: 0.1,
                                    vy: 0.0,
                                    vz: 0.0,
                                    weight: 0.85,
                                    posture: 'standing'
                                }
                            ],
                            timestamp_ms: 1710519800000
                        });
                    }
                });
            }

            // Stop session (match by method+url since body isn't in URL)
            if (options && options.method === 'POST' && url === '/api/replay/stop') {
                return Promise.resolve({
                    ok: true,
                    json: function() {
                        return Promise.resolve({ status: 'stopped' });
                    }
                });
            }

            // Start session
            if (url === '/api/replay/start') {
                return Promise.resolve({
                    ok: true,
                    json: function() {
                        return Promise.resolve({
                            session_id: 'start-session-1',
                            from_ms: 1710519740000,
                            to_ms: 1710519800000,
                            speed: 1,
                            state: 'paused'
                        });
                    }
                });
            }

            return Promise.resolve({
                ok: false,
                statusText: 'Not Found'
            });
        });

        global.fetch = fetchMock;

        // Load the module
        jest.isolateModules(function() {
            require('./replay.js');
            Replay = window.SpaxelReplay;
        });
    });

    afterEach(function() {
        document.body.innerHTML = '';
    });

    // ============================================
    // Initialization Tests
    // ============================================
    describe('Initialization', function() {
        test('initializes and fetches store info', function() {
            expect(Replay).toBeTruthy();
            expect(Replay.isReplayMode()).toBe(false);
            expect(Replay.isPaused()).toBe(false);
            // Should have called fetch for store info
            expect(fetchMock).toHaveBeenCalledWith('/api/replay/sessions');
        });

        test('getSession returns initial empty state', function() {
            var session = Replay.getSession();
            expect(session.id).toBeNull();
            expect(session.fromMs).toBeNull();
            expect(session.state).toBe('stopped');
        });
    });

    // ============================================
    // Jump-to-Time Tests
    // ============================================
    describe('jumpToTime', function() {
        test('creates replay session centered on timestamp', function() {
            return Replay.jumpToTime(1710519800000).then(function(data) {
                expect(fetchMock).toHaveBeenCalledWith('/api/replay/jump-to-time', expect.objectContaining({
                    method: 'POST',
                    headers: expect.objectContaining({ 'Content-Type': 'application/json' })
                }));

                // Check the request body
                var call = fetchMock.mock.calls.find(function(c) { return c[0] === '/api/replay/jump-to-time'; });
                var body = JSON.parse(call[1].body);
                expect(body.timestamp_ms).toBe(1710519800000);
                expect(body.window_ms).toBe(5000);
            });
        });

        test('uses custom window_ms when provided', function() {
            fetchMock.mockImplementation(function(url) {
                if (url === '/api/replay/jump-to-time') {
                    return Promise.resolve({
                        ok: true,
                        json: function() {
                            return Promise.resolve({
                                session_id: 'custom-window',
                                timestamp_ms: 1710519800000,
                                from_ms: 1710519790000,
                                to_ms: 1710519810000,
                                state: 'paused'
                            });
                        }
                    });
                }
                if (url.match(/\/api\/replay\/session\//)) {
                    return Promise.resolve({
                        ok: true,
                        json: function() {
                            return Promise.resolve({
                                session_id: 'custom-window',
                                current_ms: 1710519800000,
                                from_ms: 1710519790000,
                                to_ms: 1710519810000,
                                state: 'paused',
                                blobs: [],
                                timestamp_ms: 1710519800000
                            });
                        }
                    });
                }
                if (url === '/api/replay/sessions') {
                    return Promise.resolve({
                        ok: true,
                        json: function() { return Promise.resolve({ has_data: true, sessions: [] }); }
                    });
                }
                return Promise.resolve({ ok: false, statusText: 'Not Found' });
            });

            // Need to re-init with new fetch mock
            jest.isolateModules(function() {
                require('./replay.js');
                var R = window.SpaxelReplay;
                R.jumpToTime(1710519800000, 10000).then(function() {
                    var call = fetchMock.mock.calls.find(function(c) { return c[0] === '/api/replay/jump-to-time'; });
                    var body = JSON.parse(call[1].body);
                    expect(body.window_ms).toBe(10000);
                });
            });

            return Promise.resolve();
        });

        test('enters replay mode after successful jump', function() {
            return Replay.jumpToTime(1710519800000).then(function() {
                expect(Replay.isReplayMode()).toBe(true);
            });
        });

        test('shows replay control bar after jump', function() {
            return Replay.jumpToTime(1710519800000).then(function() {
                var bar = document.getElementById('replay-control-bar');
                expect(bar).toBeTruthy();
                expect(bar.style.display).toBe('block');
            });
        });

        test('calls Viz3D.enterReplayMode', function() {
            return Replay.jumpToTime(1710519800000).then(function() {
                expect(global.Viz3D.enterReplayMode).toHaveBeenCalled();
            });
        });

        test('fetches session blobs and feeds them to Viz3D', function() {
            return Replay.jumpToTime(1710519800000).then(function() {
                // Should have fetched session state for blobs
                var sessionCall = fetchMock.mock.calls.find(function(c) {
                    return c[0] && c[0].match(/\/api\/replay\/session\//);
                });
                expect(sessionCall).toBeTruthy();

                // Should have fed blobs to Viz3D
                expect(global.Viz3D.updateReplayBlobs).toHaveBeenCalledWith(
                    expect.arrayContaining([
                        expect.objectContaining({
                            id: 1,
                            x: 2.5,
                            y: 1.3,
                            z: 0.8
                        })
                    ]),
                    1710519800000
                );
            });
        });

        test('updates session state from jump response', function() {
            return Replay.jumpToTime(1710519800000).then(function() {
                var session = Replay.getSession();
                expect(session.id).toBe('jump-session-1');
                expect(session.currentMs).toBe(1710519800000);
                expect(session.state).toBe('paused');
            });
        });

        test('handles API error gracefully', function() {
            fetchMock.mockImplementation(function(url) {
                if (url === '/api/replay/sessions') {
                    return Promise.resolve({
                        ok: true,
                        json: function() { return Promise.resolve({ has_data: false, sessions: [] }); }
                    });
                }
                if (url === '/api/replay/jump-to-time') {
                    return Promise.resolve({
                        ok: false,
                        statusText: 'Internal Server Error'
                    });
                }
                return Promise.resolve({ ok: false, statusText: 'Not Found' });
            });

            jest.isolateModules(function() {
                require('./replay.js');
                return window.SpaxelReplay.jumpToTime(1710519800000).catch(function(err) {
                    expect(err.message).toContain('Failed to jump to time');
                });
            });
        });
    });

    // ============================================
    // Exit Replay Mode Tests
    // ============================================
    describe('exitReplayMode', function() {
        test('exits replay mode and clears state', function() {
            return Replay.jumpToTime(1710519800000).then(function() {
                expect(Replay.isReplayMode()).toBe(true);

                return Replay.exitReplay();
            }).then(function() {
                expect(Replay.isReplayMode()).toBe(false);
                expect(Replay.isPaused()).toBe(false);
            });
        });

        test('hides replay control bar', function() {
            return Replay.jumpToTime(1710519800000).then(function() {
                return Replay.exitReplay();
            }).then(function() {
                var bar = document.getElementById('replay-control-bar');
                expect(bar.style.display).toBe('none');
            });
        });

        test('calls Viz3D.exitReplayMode', function() {
            return Replay.jumpToTime(1710519800000).then(function() {
                return Replay.exitReplay();
            }).then(function() {
                expect(global.Viz3D.exitReplayMode).toHaveBeenCalled();
            });
        });

        test('clears sidebar timeline selection and chip', function() {
            return Replay.jumpToTime(1710519800000).then(function() {
                return Replay.exitReplay();
            }).then(function() {
                expect(global.SpaxelSidebarTimeline.clearSelection).toHaveBeenCalled();
                expect(global.SpaxelSidebarTimeline.hideNowReplayingChip).toHaveBeenCalled();
            });
        });

        test('navigates back to live mode', function() {
            return Replay.jumpToTime(1710519800000).then(function() {
                return Replay.exitReplay();
            }).then(function() {
                expect(global.SpaxelRouter.navigate).toHaveBeenCalledWith('live');
            });
        });

        test('stops the replay session via API', function() {
            return Replay.jumpToTime(1710519800000).then(function() {
                return Replay.exitReplay();
            }).then(function() {
                var stopCall = fetchMock.mock.calls.find(function(c) { return c[0] === '/api/replay/stop'; });
                expect(stopCall).toBeTruthy();
                var body = JSON.parse(stopCall[1].body);
                expect(body.session_id).toBe('jump-session-1');
            });
        });
    });

    // ============================================
    // Pause Live Mode Tests
    // ============================================
    describe('pauseLiveMode', function() {
        test('creates replay window 60 seconds before now', function() {
            var beforeMs = Date.now();

            Replay.pauseLive();

            var startCall = fetchMock.mock.calls.find(function(c) { return c[0] === '/api/replay/start'; });
            expect(startCall).toBeTruthy();
            var body = JSON.parse(startCall[1].body);
            var fromMs = new Date(body.from_iso8601).getTime();
            var toMs = new Date(body.to_iso8601).getTime();

            // Should be approximately 60s window
            var windowMs = toMs - fromMs;
            expect(windowMs).toBeGreaterThanOrEqual(59000);
            expect(windowMs).toBeLessThanOrEqual(61000);
            expect(fromMs).toBeGreaterThanOrEqual(beforeMs - 61000);
        });
    });

    // ============================================
    // Replay Control Bar UI Tests
    // ============================================
    describe('Replay Control Bar', function() {
        test('control bar is created in the DOM', function() {
            var bar = document.getElementById('replay-control-bar');
            expect(bar).toBeTruthy();
        });

        test('control bar has all required buttons', function() {
            expect(document.getElementById('replay-back-btn')).toBeTruthy();
            expect(document.getElementById('replay-play-btn')).toBeTruthy();
            expect(document.getElementById('replay-speed')).toBeTruthy();
            expect(document.getElementById('replay-scrubber')).toBeTruthy();
            expect(document.getElementById('replay-close-btn')).toBeTruthy();
            expect(document.getElementById('replay-tune-btn')).toBeTruthy();
        });

        test('control bar is hidden by default', function() {
            var bar = document.getElementById('replay-control-bar');
            expect(bar.style.display).toBe('none');
        });
    });
});
