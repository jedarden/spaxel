/**
 * Spaxel Dashboard - Time-Travel Replay Mode
 *
 * Provides pause-live, timeline scrubbing, and replay 3D visualization
 * from recorded CSI data.
 */

(function() {
    'use strict';

    // ============================================
    // Configuration
    // ============================================
    const CONFIG = {
        // Default replay window when pausing live (60 seconds before now)
        defaultReplayWindowSec: 60,
        // Timeline scrubber update interval (ms)
        timelineUpdateInterval: 100,
        // Playback speeds
        speeds: [1, 2, 5],
        // Timestamp range padding when creating replay sessions
        sessionPaddingMs: 5000,
        // Event fetch configuration
        eventFetchBatchSize: 100, // events per batch
        eventMarkerTypes: ['anomaly', 'anomaly_detected', 'security_alert', 'portal_crossing', 'zone_entry', 'zone_exit'],
    };

    // ============================================
    // State
    // ============================================
    const state = {
        // Replay session state
        activeSessionId: null,
        sessionFromMs: null,
        sessionToMs: null,
        sessionCurrentMs: null,
        sessionSpeed: 1,
        sessionState: 'stopped', // stopped, paused, playing

        // Recording store info
        storeOldestMs: null,
        storeNewestMs: null,
        storeHasData: false,

        // UI state
        isPaused: false,
        isReplayMode: false,

        // Callbacks
        onReplayBlob: null,
    };

    // ============================================
    // DOM Elements
    // ============================================
    let elements = {};

    // ============================================
    // Initialization
    // ============================================
    function init() {
        console.log('[Replay] Initializing time-travel replay');

        // Create replay controls
        createReplayControls();

        // Fetch recording store info
        fetchStoreInfo();

        // Start timeline update loop
        startTimelineLoop();

        console.log('[Replay] Ready');
    }

    // ============================================
    // Replay Controls UI
    // ============================================
    function createReplayControls() {
        const statusBar = document.getElementById('status-bar');
        if (!statusBar) {
            console.warn('[Replay] Status bar not found');
            return;
        }

        // Create replay control bar (hidden by default)
        const replayBar = document.createElement('div');
        replayBar.id = 'replay-control-bar';
        replayBar.className = 'replay-control-bar';
        replayBar.style.display = 'none';

        replayBar.innerHTML = `
            <div class="replay-controls">
                <button id="replay-back-btn" class="replay-btn" title="Back to live">
                    <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <path d="M1 4v6h6"/>
                        <path d="M3.51 15a9 9 0 1 0 2.13-9.36L23 10M10 19l-7-7"/>
                    </svg>
                </button>

                <div class="replay-info">
                    <span id="replay-timestamp" class="replay-timestamp">--:--:--</span>
                    <span id="replay-range" class="replay-range">0:00 / 0:00</span>
                </div>

                <div class="replay-playback">
                    <button id="replay-play-btn" class="replay-btn" title="Play/Pause">
                        <svg id="play-icon" xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="currentColor">
                            <path d="M8 5v14l11-7z"/>
                        </svg>
                        <svg id="pause-icon" style="display:none" xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="currentColor">
                            <path d="M6 19h4V5H6v14zm8-14v14h4V5h-4z"/>
                        </svg>
                    </button>

                    <select id="replay-speed" class="replay-speed">
                        <option value="1">1×</option>
                        <option value="2">2×</option>
                        <option value="5">5×</option>
                    </select>
                </div>

                <div class="replay-timeline">
                    <input type="range" id="replay-scrubber" class="replay-scrubber"
                           min="0" max="100" step="0.1" value="0"
                           title="Scrub through timeline">
                </div>

                <button id="replay-close-btn" class="replay-btn" title="Exit replay mode">
                    <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <line x1="18" y1="6" x2="6" y2="18"/>
                        <line x1="6" y1="6" x2="18" y2="18"/>
                    </svg>
                </button>
            </div>

            <div class="replay-tuning">
                <button id="replay-tune-btn" class="replay-tune-btn" title="Tune detection parameters">
                    <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <circle cx="12" cy="12" r="3"/>
                        <path d="M12 1v6m0 6v6"/>
                        <path d="m19.07 4.93-1.41 1.41M17 16l-5-5"/>
                        <path d="M4.93 19.07l1.41-1.41M16 17l5-5"/>
                    </svg>
                    Tune
                </button>
            </div>
        `;

        // Insert after status bar
        statusBar.parentNode.insertBefore(replayBar, statusBar.nextSibling);

        // Store element references
        elements = {
            bar: replayBar,
            backBtn: document.getElementById('replay-back-btn'),
            timestamp: document.getElementById('replay-timestamp'),
            range: document.getElementById('replay-range'),
            playBtn: document.getElementById('replay-play-btn'),
            playIcon: document.getElementById('play-icon'),
            pauseIcon: document.getElementById('pause-icon'),
            speed: document.getElementById('replay-speed'),
            scrubber: document.getElementById('replay-scrubber'),
            closeBtn: document.getElementById('replay-close-btn'),
            tuneBtn: document.getElementById('replay-tune-btn'),
        };

        // Attach event listeners
        elements.backBtn.addEventListener('click', onBackToLive);
        elements.playBtn.addEventListener('click', onPlayPause);
        elements.speed.addEventListener('change', onSpeedChange);
        elements.scrubber.addEventListener('input', onScrub);
        elements.closeBtn.addEventListener('click', onExitReplay);
        elements.tuneBtn.addEventListener('click', onTuneParams);
    }

    // ============================================
    // API Communication
    // ============================================
    function fetchStoreInfo() {
        fetch('/api/replay/sessions')
            .then(res => res.json())
            .then(data => {
                state.storeOldestMs = data.oldest_timestamp_ms;
                state.storeNewestMs = data.newest_timestamp_ms;
                state.storeHasData = data.has_data;
                console.log('[Replay] Store info:', data);
            })
            .catch(err => {
                console.error('[Replay] Failed to fetch store info:', err);
            });
    }

    function startReplaySession(fromMs, toMs) {
        const fromISO = new Date(fromMs).toISOString();
        const toISO = toMs ? new Date(toMs).toISOString() : '';

        return fetch('/api/replay/start', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                from_iso8601: fromISO,
                to_iso8601: toISO,
                speed: 1
            })
        })
        .then(res => {
            if (!res.ok) {
                throw new Error('Failed to start replay: ' + res.statusText);
            }
            return res.json();
        })
        .then(data => {
            state.activeSessionId = data.session_id;
            state.sessionFromMs = data.from_ms;
            state.sessionToMs = data.to_ms;
            state.sessionCurrentMs = data.from_ms;
            state.sessionState = 'paused';
            state.sessionSpeed = data.speed;

            console.log('[Replay] Session started:', data);
            updateUI();
            return data.session_id;
        });
    }

    function stopReplaySession() {
        if (!state.activeSessionId) return Promise.resolve();

        return fetch('/api/replay/stop', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                session_id: state.activeSessionId
            })
        })
        .then(res => {
            if (!res.ok) {
                throw new Error('Failed to stop replay: ' + res.statusText);
            }
            return res.json();
        })
        .then(() => {
            console.log('[Replay] Session stopped:', state.activeSessionId);
            state.activeSessionId = null;
            state.isPaused = false;
            updateUI();
        });
    }

    function seekReplay(targetMs) {
        if (!state.activeSessionId) return Promise.resolve();

        const targetISO = new Date(targetMs).toISOString();

        return fetch('/api/replay/seek', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                session_id: state.activeSessionId,
                timestamp_iso8601: targetISO
            })
        })
        .then(res => {
            if (!res.ok) {
                throw new Error('Failed to seek: ' + res.statusText);
            }
            return res.json();
        })
        .then(data => {
            state.sessionCurrentMs = data.current_ms;
            updateUI();
        });
    }

    function setPlaybackSpeed(speed) {
        if (!state.activeSessionId) return;

        fetch('/api/replay/set-speed', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                session_id: state.activeSessionId,
                speed: speed
            })
        })
        .then(res => {
            if (!res.ok) {
                throw new Error('Failed to set speed: ' + res.statusText);
            }
            return res.json();
        })
        .then(() => {
            state.sessionSpeed = speed;
            console.log('[Replay] Speed set to', speed, 'x');
        });
    }

    function setPlaybackState(playbackState) {
        if (!state.activeSessionId) return;

        fetch('/api/replay/set-state', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                session_id: state.activeSessionId,
                state: playbackState
            })
        })
        .then(res => {
            if (!res.ok) {
                throw new Error('Failed to set state: ' + res.statusText);
            }
            return res.json();
        })
        .then(() => {
            state.sessionState = playbackState;
            updatePlayPauseButton();
        });
    }

    function tuneParams(params) {
        if (!state.activeSessionId) return Promise.resolve();

        return fetch('/api/replay/tune', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                session_id: state.activeSessionId,
                ...params
            })
        })
        .then(res => {
            if (!res.ok) {
                throw new Error('Failed to tune params: ' + res.statusText);
            }
            return res.json();
        })
        .then(data => {
            console.log('[Replay] Parameters tuned:', data.params);
            return data;
        });
    }

    // ============================================
    // Event Handlers
    // ============================================
    function onBackToLive() {
        exitReplayMode();
    }

    function onPlayPause() {
        if (!state.activeSessionId) return;

        const newState = state.sessionState === 'playing' ? 'paused' : 'playing';
        setPlaybackState(newState);
    }

    function onSpeedChange(e) {
        const speed = parseInt(e.target.value, 10);
        setPlaybackSpeed(speed);
    }

    function onScrub(e) {
        const percent = parseFloat(e.target.value);
        const rangeMs = state.sessionToMs - state.sessionFromMs;
        const targetMs = state.sessionFromMs + Math.round(rangeMs * percent / 100);

        seekReplay(targetMs);
    }

    function onExitReplay() {
        exitReplayMode();
    }

    function onTuneParams() {
        showTuningPanel();
    }

    // ============================================
    // UI Updates
    // ============================================
    function updateUI() {
        updateTimestampDisplay();
        updateRangeDisplay();
        updateScrubber();
        updatePlayPauseButton();
    }

    function updateTimestampDisplay() {
        if (!elements.timestamp) return;

        if (state.sessionCurrentMs !== null) {
            elements.timestamp.textContent = formatTimestamp(state.sessionCurrentMs);
        } else {
            elements.timestamp.textContent = '--:--:--';
        }
    }

    function updateRangeDisplay() {
        if (!elements.range) return;

        if (state.sessionFromMs !== null && state.sessionToMs !== null) {
            elements.range.textContent = formatTimestamp(state.sessionFromMs) +
                ' / ' + formatTimestamp(state.sessionToMs);
        } else {
            elements.range.textContent = '0:00 / 0:00';
        }
    }

    function updateScrubber() {
        if (!elements.scrubber) return;

        if (state.sessionFromMs !== null && state.sessionToMs !== null) {
            const rangeMs = state.sessionToMs - state.sessionFromMs;
            if (state.sessionCurrentMs !== null) {
                const offset = state.sessionCurrentMs - state.sessionFromMs;
                const percent = Math.max(0, Math.min(100, (offset / rangeMs) * 100));
                elements.scrubber.value = percent;
            }
        }
    }

    function updatePlayPauseButton() {
        if (!elements.playIcon || !elements.pauseIcon) return;

        if (state.sessionState === 'playing') {
            elements.playIcon.style.display = 'none';
            elements.pauseIcon.style.display = 'block';
        } else {
            elements.playIcon.style.display = 'block';
            elements.pauseIcon.style.display = 'none';
        }
    }

    // ============================================
    // Mode Transitions
    // ============================================
    function enterReplayMode(fromMs, toMs) {
        console.log('[Replay] Entering replay mode:', { fromMs, toMs });

        state.isReplayMode = true;

        // Start replay session
        startReplaySession(fromMs, toMs).then(sessionId => {
            // Show replay control bar
            if (elements.bar) {
                elements.bar.style.display = 'block';
            }

            // Notify 3D visualization to enter replay mode
            if (window.Viz3D && Viz3D.enterReplayMode) {
                Viz3D.enterReplayMode();
            }

            return sessionId;
        }).catch(err => {
            console.error('[Replay] Failed to enter replay mode:', err);
            if (window.SpaxelApp) {
                SpaxelApp.showToast('Failed to enter replay mode: ' + err.message, 'error');
            }
        });
    }

    function exitReplayMode() {
        console.log('[Replay] Exiting replay mode');

        // Stop replay session
        stopReplaySession().then(() => {
            state.isReplayMode = false;
            state.isPaused = false;

            // Hide replay control bar
            if (elements.bar) {
                elements.bar.style.display = 'none';
            }

            // Notify 3D visualization to exit replay mode
            if (window.Viz3D && Viz3D.exitReplayMode) {
                Viz3D.exitReplayMode();
            }

            // Navigate back to live mode
            if (window.SpaxelRouter) {
                SpaxelRouter.navigate('live');
            }
        });
    }

    function pauseLiveMode() {
        if (state.isPaused) {
            // Already paused, exit replay mode
            exitReplayMode();
            return;
        }

        console.log('[Replay] Pausing live mode');

        state.isPaused = true;

        // Calculate replay window (default: 60 seconds before now)
        const now = Date.now();
        const fromMs = now - (CONFIG.defaultReplayWindowSec * 1000);
        const toMs = now;

        enterReplayMode(fromMs, toMs);
    }

    // ============================================
    // Timeline Loop
    // ============================================
    let timelineInterval = null;

    function startTimelineLoop() {
        if (timelineInterval) return;

        timelineInterval = setInterval(() => {
            if (state.sessionState === 'playing' && state.activeSessionId) {
                // Fetch current session state
                fetch(`/api/replay/session/${state.activeSessionId}`)
                    .then(res => res.json())
                    .then(data => {
                        state.sessionCurrentMs = data.current_ms;
                        updateUI();

                        // Trigger 3D visualization update with replay blobs
                        if (data.blobs && window.Viz3D && Viz3D.updateReplayBlobs) {
                            Viz3D.updateReplayBlobs(data.blobs, data.timestamp_ms);
                        }
                    })
                    .catch(err => {
                        console.error('[Replay] Failed to fetch session state:', err);
                    });
            }
        }, CONFIG.timelineUpdateInterval);
    }

    // ============================================
    // Tuning Panel
    // ============================================
    function showTuningPanel() {
        // Create or show tuning panel overlay
        let panel = document.getElementById('replay-tuning-panel');
        if (!panel) {
            panel = createTuningPanel();
            document.body.appendChild(panel);
        }

        panel.style.display = 'flex';

        // Fetch current session params
        if (state.activeSessionId) {
            fetch(`/api/replay/session/${state.activeSessionId}`)
                .then(res => res.json())
                .then(data => {
                    populateTuningPanel(data.params || {});
                });
        }
    }

    function createTuningPanel() {
        const panel = document.createElement('div');
        panel.id = 'replay-tuning-panel';
        panel.className = 'replay-tuning-panel';

        panel.innerHTML = `
            <div class="replay-tuning-content">
                <div class="replay-tuning-header">
                    <h2>Detection Parameters</h2>
                    <button class="replay-tuning-close" onclick="this.parentElement.parentElement.parentElement.style.display='none'">
                        <svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                            <line x1="18" y1="6" x2="6" y2="18"/>
                            <line x1="6" y1="6" x2="18" y2="18"/>
                        </svg>
                    </button>
                </div>

                <div class="replay-tuning-body">
                    <div class="tuning-param">
                        <label>Detection Threshold (deltaRMS)</label>
                        <input type="range" id="tune-threshold" min="0.001" max="0.1" step="0.001" value="0.02">
                        <span class="tuning-value">0.02</span>
                    </div>

                    <div class="tuning-param">
                        <label>Baseline Time Constant (tau) [seconds]</label>
                        <input type="range" id="tune-tau" min="1" max="600" step="1" value="30">
                        <span class="tuning-value">30</span>
                    </div>

                    <div class="tuning-param">
                        <label>Fresnel Weight Decay Rate</label>
                        <input type="range" id="tune-fresnel" min="1.0" max="4.0" step="0.1" value="2.0">
                        <span class="tuning-value">2.0</span>
                    </div>

                    <div class="tuning-param">
                        <label>Subcarrier Count (NBVI)</label>
                        <input type="range" id="tune-subcarriers" min="8" max="47" step="1" value="16">
                        <span class="tuning-value">16</span>
                    </div>

                    <div class="tuning-param">
                        <label>Breathing Sensitivity</label>
                        <input type="range" id="tune-breathing" min="0.001" max="0.1" step="0.001" value="0.005">
                        <span class="tuning-value">0.005</span>
                    </div>

                    <div class="tuning-actions">
                        <button id="tune-apply-btn" class="tuning-btn">Apply Parameters</button>
                        <button id="tune-reset-btn" class="tuning-btn tuning-btn-secondary">Reset to Live</button>
                    </div>
                </div>
            </div>
        `;

        // Attach event listeners
        panel.querySelector('#tune-apply-btn').addEventListener('click', applyTuningParams);
        panel.querySelector('#tune-reset-btn').addEventListener('click', resetTuningParams);

        // Update value displays on slider change
        panel.querySelectorAll('input[type="range"]').forEach(input => {
            input.addEventListener('input', (e) => {
                const valueSpan = e.target.parentElement.querySelector('.tuning-value');
                if (valueSpan) {
                    valueSpan.textContent = e.target.value;
                }
            });
        });

        return panel;
    }

    function populateTuningPanel(params) {
        const panel = document.getElementById('replay-tuning-panel');
        if (!panel) return;

        // Update sliders with current params
        if (params.delta_rms_threshold !== undefined) {
            const input = panel.querySelector('#tune-threshold');
            if (input) {
                input.value = params.delta_rms_threshold;
                input.parentElement.querySelector('.tuning-value').textContent = params.delta_rms_threshold;
            }
        }
        if (params.tau_s !== undefined) {
            const input = panel.querySelector('#tune-tau');
            if (input) {
                input.value = params.tau_s;
                input.parentElement.querySelector('.tuning-value').textContent = params.tau_s;
            }
        }
        if (params.fresnel_decay !== undefined) {
            const input = panel.querySelector('#tune-fresnel');
            if (input) {
                input.value = params.fresnel_decay;
                input.parentElement.querySelector('.tuning-value').textContent = params.fresnel_decay;
            }
        }
        if (params.n_subcarriers !== undefined) {
            const input = panel.querySelector('#tune-subcarriers');
            if (input) {
                input.value = params.n_subcarriers;
                input.parentElement.querySelector('.tuning-value').textContent = params.n_subcarriers;
            }
        }
        if (params.breathing_sensitivity !== undefined) {
            const input = panel.querySelector('#tune-breathing');
            if (input) {
                input.value = params.breathing_sensitivity;
                input.parentElement.querySelector('.tuning-value').textContent = params.breathing_sensitivity;
            }
        }
    }

    function applyTuningParams() {
        const panel = document.getElementById('replay-tuning-panel');
        if (!panel) return;

        const params = {
            delta_rms_threshold: parseFloat(panel.querySelector('#tune-threshold').value),
            tau_s: parseFloat(panel.querySelector('#tune-tau').value),
            fresnel_decay: parseFloat(panel.querySelector('#tune-fresnel').value),
            n_subcarriers: parseInt(panel.querySelector('#tune-subcarriers').value, 10),
            breathing_sensitivity: parseFloat(panel.querySelector('#tune-breathing').value),
        };

        tuneParams(params).then(() => {
            if (window.SpaxelApp) {
                SpaxelApp.showToast('Parameters updated. Processing replay with new settings...', 'info');
            }

            // Hide panel after applying
            panel.style.display = 'none';
        }).catch(err => {
            console.error('[Replay] Failed to tune parameters:', err);
            if (window.SpaxelApp) {
                SpaxelApp.showToast('Failed to update parameters: ' + err.message, 'error');
            }
        });
    }

    function resetTuningParams() {
        // Reset to live default values
        const params = {
            delta_rms_threshold: 0.02,
            tau_s: 30,
            fresnel_decay: 2.0,
            n_subcarriers: 16,
            breathing_sensitivity: 0.005,
        };

        tuneParams(params).then(() => {
            populateTuningPanel(params);
            if (window.SpaxelApp) {
                SpaxelApp.showToast('Parameters reset to live defaults', 'info');
            }
        }).catch(err => {
            console.error('[Replay] Failed to reset parameters:', err);
        });
    }

    // ============================================
    // Utilities
    // ============================================
    function formatTimestamp(ms) {
        const date = new Date(ms);
        const hours = String(date.getHours()).padStart(2, '0');
        const minutes = String(date.getMinutes()).padStart(2, '0');
        const seconds = String(date.getSeconds()).padStart(2, '0');
        return `${hours}:${minutes}:${seconds}`;
    }

    // ============================================
    // Public API
    // ============================================
    window.SpaxelReplay = {
        init: init,

        // Pause live mode and enter replay
        pauseLive: pauseLiveMode,

        // Exit replay mode and return to live
        exitReplay: exitReplayMode,

        // Check if currently in replay mode
        isReplayMode: () => state.isReplayMode,

        // Check if currently paused
        isPaused: () => state.isPaused,

        // Get current replay session info
        getSession: () => ({
            id: state.activeSessionId,
            fromMs: state.sessionFromMs,
            toMs: state.sessionToMs,
            currentMs: state.sessionCurrentMs,
            speed: state.sessionSpeed,
            state: state.sessionState,
        }),
    };

    // Auto-initialize when DOM is ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }
})();
