/**
 * Spaxel Sleep Quality Monitoring UI
 *
 * Handles: morning summary card, sleep panel with weekly trends,
 * and live sleep session display.
 */
(function() {
    'use strict';

    // ── module state ──────────────────────────────────────────────────────────
    let _currentSummary = null;   // Most recent morning summary
    let _weeklyTrends = null;     // Weekly trends data
    let _sleepRecords = [];       // Historical sleep records
    let _summaryDismissed = false; // Whether morning summary was dismissed this session
    let _panelVisible = false;    // Whether sleep panel is showing

    // DOM element cache
    let _summaryCardEl = null;
    let _sleepPanelEl = null;

    // ── initialization ────────────────────────────────────────────────────────

    function init() {
        ensureSummaryCard();
        ensureSleepPanel();
        console.log('[Sleep] Module initialized');
    }

    // ── Morning Summary Card ────────────────────────────────────────────────

    function ensureSummaryCard() {
        if (document.getElementById('sleep-summary-card')) return;

        const card = document.createElement('div');
        card.id = 'sleep-summary-card';
        card.className = 'sleep-summary-card hidden';
        card.innerHTML = `
            <div class="sleep-summary-header">
                <span class="sleep-summary-icon">
                    <svg viewBox="0 0 24 24" width="20" height="20">
                        <path fill="currentColor" d="M17.75,4.09L15.22,6.03L16.13,9.09L13.5,7.28L10.87,9.09L11.78,6.03L9.25,4.09L12.44,4L13.5,1L14.56,4L17.75,4.09M21.25,11L19.61,12.25L20.2,14.23L18.5,13.06L16.8,14.23L17.39,12.25L15.75,11L17.81,10.95L18.5,9L19.19,10.95L21.25,11M18.97,15.95C19.8,15.87 20.69,17.05 20.16,17.8C19.84,18.25 19.17,18.7 18.46,18.7H14.5L18.27,21.63C18.5,21.8 18.5,22.12 18.27,22.29C18.1,22.5 17.77,22.5 17.56,22.29L12.5,18.25L7.44,22.29C7.23,22.5 6.9,22.5 6.73,22.29C6.5,22.12 6.5,21.8 6.73,21.63L10.5,18.7H6.54C5.83,18.7 5.16,18.25 4.84,17.8C4.31,17.05 5.2,15.87 6.03,15.95L8.25,16.13L9.37,14.5L7.5,13.87L4.5,14.57L3.86,12.24L7.5,11.37L11.25,10.5L12.5,8.16L13.75,10.5L17.5,11.37L21.14,12.24L20.5,14.57L17.5,13.87L18.63,15.5L18.97,15.95Z"/>
                    </svg>
                </span>
                <span class="sleep-summary-title">Last Night's Sleep</span>
                <button class="sleep-summary-dismiss" title="Dismiss">&times;</button>
            </div>
            <div class="sleep-summary-body">
                <div class="sleep-summary-duration"></div>
                <div class="sleep-summary-efficiency"></div>
                <div class="sleep-summary-wake-episodes"></div>
                <div class="sleep-summary-breathing"></div>
                <div class="sleep-summary-anomaly hidden"></div>
                <button class="sleep-summary-details-btn hidden">View full sleep report</button>
            </div>
        `;
        document.body.appendChild(card);
        _summaryCardEl = card;

        // Dismiss button
        card.querySelector('.sleep-summary-dismiss').addEventListener('click', function() {
            dismissSummary();
        });

        // View details button
        card.querySelector('.sleep-summary-details-btn').addEventListener('click', function() {
            showSleepPanel();
        });
    }

    /**
     * Show morning summary card with data from the backend.
     * @param {Object} report - Sleep report data from /api/sleep/summary or WebSocket morning_summary message
     */
    function showMorningSummary(report) {
        if (!report) return;
        _currentSummary = report;
        _summaryDismissed = false;

        ensureSummaryCard();
        const card = _summaryCardEl;
        const metrics = report.metrics || {};

        // Duration
        const durationEl = card.querySelector('.sleep-summary-duration');
        if (metrics.total_duration_hours) {
            const hours = Math.floor(metrics.total_duration_hours);
            const mins = Math.round((metrics.total_duration_hours - hours) * 60);
            durationEl.textContent = 'Last night: ' + hours + 'h ' + mins + 'm';
        } else if (report.time_in_bed_hours) {
            const hours = Math.floor(report.time_in_bed_hours);
            const mins = Math.round((report.time_in_bed_hours - hours) * 60);
            durationEl.textContent = 'Last night: ' + hours + 'h ' + mins + 'm in bed';
        }

        // Efficiency with color indicator
        const effEl = card.querySelector('.sleep-summary-efficiency');
        const efficiency = metrics.sleep_efficiency || metrics.overall_score || 0;
        let effColor = 'red';
        let effLabel = 'Poor';
        if (efficiency >= 85) { effColor = 'green'; effLabel = 'Good'; }
        else if (efficiency >= 70) { effColor = 'amber'; effLabel = 'Fair'; }
        effEl.innerHTML = '<span class="sleep-efficiency-dot ' + effColor + '"></span> Sleep efficiency: ' + efficiency.toFixed(0) + '% (' + effLabel + ')';

        // Wake episodes
        const wakeEl = card.querySelector('.sleep-summary-wake-episodes');
        const wakeCount = metrics.wake_episode_count || 0;
        const waso = metrics.waso_minutes || 0;
        if (wakeCount > 0) {
            wakeEl.textContent = wakeCount + ' wake episode' + (wakeCount !== 1 ? 's' : '') + ', ' + Math.round(waso) + ' min awake after onset';
        } else {
            wakeEl.textContent = 'No wake episodes detected';
        }

        // Breathing
        const breathEl = card.querySelector('.sleep-summary-breathing');
        const avgBPM = metrics.avg_breathing_rate || 0;
        if (avgBPM > 0) {
            breathEl.textContent = 'Average breathing: ' + avgBPM.toFixed(1) + ' breaths/min';
        } else {
            breathEl.textContent = 'No breathing data available';
        }

        // Anomaly note
        const anomalyEl = card.querySelector('.sleep-summary-anomaly');
        if (metrics.breathing_anomaly || (metrics.breathing_anomaly_count > 0)) {
            anomalyEl.classList.remove('hidden');
            anomalyEl.innerHTML = '<span class="sleep-anomaly-warning">Unusual breathing detected</span>' +
                (metrics.personal_avg_bpm ? ' (' + avgBPM.toFixed(0) + ' bpm vs. ' + metrics.personal_avg_bpm.toFixed(0) + ' bpm average)' : '');
        } else {
            anomalyEl.classList.add('hidden');
        }

        // Show details button in expert mode
        const detailsBtn = card.querySelector('.sleep-summary-details-btn');
        if (window.SpaxelApp && window.SpaxelApp.isExpertMode && window.SpaxelApp.isExpertMode()) {
            detailsBtn.classList.remove('hidden');
        } else {
            detailsBtn.classList.add('hidden');
        }

        // Show the card
        card.classList.remove('hidden');
    }

    function dismissSummary() {
        _summaryDismissed = true;
        if (_summaryCardEl) {
            _summaryCardEl.classList.add('hidden');
        }
    }

    // ── Sleep Panel ──────────────────────────────────────────────────────────

    function ensureSleepPanel() {
        if (document.getElementById('sleep-panel')) return;

        const panel = document.createElement('div');
        panel.id = 'sleep-panel';
        panel.className = 'sleep-panel hidden';
        panel.innerHTML = `
            <div class="sleep-panel-header">
                <h3>Sleep Monitoring</h3>
                <button class="sleep-panel-close" title="Close">&times;</button>
            </div>
            <div class="sleep-panel-content">
                <div class="sleep-panel-section">
                    <h4>Weekly Trends</h4>
                    <div class="sleep-trends-container">
                        <div class="sleep-trend-row">
                            <span class="sleep-trend-label">Sleep Duration</span>
                            <div class="sleep-sparkline" id="sleep-duration-sparkline"></div>
                            <span class="sleep-trend-value" id="sleep-duration-avg"></span>
                        </div>
                        <div class="sleep-trend-row">
                            <span class="sleep-trend-label">Sleep Efficiency</span>
                            <div class="sleep-sparkline" id="sleep-efficiency-sparkline"></div>
                            <span class="sleep-trend-value" id="sleep-efficiency-avg"></span>
                        </div>
                    </div>
                    <div class="sleep-week-comparison" id="sleep-week-comparison"></div>
                </div>

                <div class="sleep-panel-section">
                    <h4>Breathing</h4>
                    <div class="sleep-breathing-stats">
                        <div class="sleep-stat">
                            <span class="sleep-stat-label">Average Rate</span>
                            <span class="sleep-stat-value" id="sleep-avg-breathing"></span>
                        </div>
                        <div class="sleep-stat">
                            <span class="sleep-stat-label">Variability</span>
                            <span class="sleep-stat-value" id="sleep-breathing-variability"></span>
                        </div>
                    </div>
                </div>

                <div class="sleep-panel-section">
                    <h4>Recent Nights</h4>
                    <div class="sleep-history" id="sleep-history-list"></div>
                </div>
            </div>
        `;
        document.body.appendChild(panel);
        _sleepPanelEl = panel;

        // Close button
        panel.querySelector('.sleep-panel-close').addEventListener('click', function() {
            hideSleepPanel();
        });
    }

    function showSleepPanel() {
        ensureSleepPanel();
        _sleepPanelEl.classList.remove('hidden');
        _panelVisible = true;
        fetchSleepData();
    }

    function hideSleepPanel() {
        if (_sleepPanelEl) {
            _sleepPanelEl.classList.add('hidden');
        }
        _panelVisible = false;
    }

    function toggleSleepPanel() {
        if (_panelVisible) {
            hideSleepPanel();
        } else {
            showSleepPanel();
        }
    }

    // ── Data Fetching ────────────────────────────────────────────────────────

    function fetchSleepData() {
        fetchSleepRecords();
        fetchWeeklyTrends();
    }

    function fetchSleepRecords() {
        fetch('/api/sleep?limit=14')
            .then(function(r) { return r.json(); })
            .then(function(records) {
                _sleepRecords = records || [];
                renderHistory();
            })
            .catch(function(e) {
                console.warn('[Sleep] Failed to fetch sleep records:', e);
            });
    }

    function fetchWeeklyTrends() {
        fetch('/api/sleep/summary')
            .then(function(r) { return r.json(); })
            .then(function(summary) {
                if (summary) {
                    _currentSummary = summary;
                    renderBreathingStats(summary);
                }
            })
            .catch(function(e) {
                console.warn('[Sleep] Failed to fetch sleep summary:', e);
            });

        // Fetch weekly trends from storage
        fetch('/api/sleep/reports')
            .then(function(r) { return r.json(); })
            .then(function(reports) {
                if (reports) {
                    renderWeeklyTrends(reports);
                }
            })
            .catch(function(e) {
                console.warn('[Sleep] Failed to fetch weekly trends:', e);
            });
    }

    // ── Rendering ────────────────────────────────────────────────────────────

    function renderWeeklyTrends(reports) {
        if (!reports || typeof reports !== 'object') return;

        // Extract per-link reports into arrays sorted by date
        const entries = [];
        for (var linkID in reports) {
            var r = reports[linkID];
            if (r && r.metrics) {
                entries.push({
                    date: r.session_date || linkID,
                    duration: (r.metrics.total_duration_hours || r.metrics.time_in_bed_hours || 0) * 60,
                    efficiency: r.metrics.sleep_efficiency || r.metrics.overall_score || 0,
                    breathing: r.metrics.avg_breathing_rate || 0
                });
            }
        }

        if (entries.length === 0) return;

        // Sort by date
        entries.sort(function(a, b) { return (a.date > b.date) - (a.date < b.date); });

        // Duration sparkline
        renderSparkline('sleep-duration-sparkline', entries.map(function(e) { return e.duration; }), 'min');
        var avgDuration = entries.reduce(function(s, e) { return s + e.duration; }, 0) / entries.length;
        var durH = Math.floor(avgDuration / 60);
        var durM = Math.round(avgDuration % 60);
        var durAvgEl = document.getElementById('sleep-duration-avg');
        if (durAvgEl) durAvgEl.textContent = durH + 'h ' + durM + 'm avg';

        // Efficiency sparkline
        renderSparkline('sleep-efficiency-sparkline', entries.map(function(e) { return e.efficiency; }), '%');
        var avgEff = entries.reduce(function(s, e) { return s + e.efficiency; }, 0) / entries.length;
        var effAvgEl = document.getElementById('sleep-efficiency-avg');
        if (effAvgEl) effAvgEl.textContent = avgEff.toFixed(0) + '% avg';

        // Average breathing rate
        var breathEntries = entries.filter(function(e) { return e.breathing > 0; });
        if (breathEntries.length > 0) {
            var avgBreath = breathEntries.reduce(function(s, e) { return s + e.breathing; }, 0) / breathEntries.length;
            var breathAvgEl = document.getElementById('sleep-avg-breathing');
            if (breathAvgEl) breathAvgEl.textContent = avgBreath.toFixed(1) + ' bpm';
        }

        // Week comparison
        var compEl = document.getElementById('sleep-week-comparison');
        if (compEl && entries.length >= 7) {
            var thisWeek = entries.slice(-7);
            var lastWeek = entries.slice(-14, -7);
            if (lastWeek.length > 0) {
                var thisAvg = thisWeek.reduce(function(s, e) { return s + e.duration; }, 0) / thisWeek.length;
                var lastAvg = lastWeek.reduce(function(s, e) { return s + e.duration; }, 0) / lastWeek.length;
                var diff = thisAvg - lastAvg;
                var sign = diff >= 0 ? '+' : '';
                var diffH = Math.floor(Math.abs(diff) / 60);
                var diffM = Math.round(Math.abs(diff) % 60);
                compEl.textContent = 'This week you slept ' + Math.floor(thisAvg / 60) + 'h ' + Math.round(thisAvg % 60) + 'm on average (vs. ' + Math.floor(lastAvg / 60) + 'h ' + Math.round(lastAvg % 60) + 'm last week, ' + sign + diffH + 'h ' + diffM + 'm)';
            }
        }
    }

    function renderSparkline(containerId, values, unit) {
        var container = document.getElementById(containerId);
        if (!container || values.length === 0) return;

        // Clear previous sparkline
        container.innerHTML = '';

        var width = 120;
        var height = 30;
        var max = Math.max.apply(null, values);
        var min = Math.min.apply(null, values);
        var range = max - min || 1;

        var svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
        svg.setAttribute('viewBox', '0 0 ' + width + ' ' + height);
        svg.setAttribute('class', 'sleep-sparkline-svg');

        // Build polyline points
        var points = values.map(function(v, i) {
            var x = (i / Math.max(1, values.length - 1)) * (width - 4) + 2;
            var y = height - 2 - ((v - min) / range) * (height - 4);
            return x.toFixed(1) + ',' + y.toFixed(1);
        }).join(' ');

        var polyline = document.createElementNS('http://www.w3.org/2000/svg', 'polyline');
        polyline.setAttribute('points', points);
        polyline.setAttribute('fill', 'none');
        polyline.setAttribute('stroke', '#4a9eff');
        polyline.setAttribute('stroke-width', '1.5');
        polyline.setAttribute('stroke-linecap', 'round');
        polyline.setAttribute('stroke-linejoin', 'round');
        svg.appendChild(polyline);

        // Latest value dot
        if (values.length > 0) {
            var lastVal = values[values.length - 1];
            var lx = width - 2;
            var ly = height - 2 - ((lastVal - min) / range) * (height - 4);
            var dot = document.createElementNS('http://www.w3.org/2000/svg', 'circle');
            dot.setAttribute('cx', lx);
            dot.setAttribute('cy', ly);
            dot.setAttribute('r', '2');
            dot.setAttribute('fill', '#4a9eff');
            svg.appendChild(dot);
        }

        container.appendChild(svg);
    }

    function renderBreathingStats(summary) {
        if (!summary) return;

        var metrics = summary.metrics || summary;
        var avgEl = document.getElementById('sleep-avg-breathing');
        if (avgEl && (metrics.avg_breathing_rate || metrics.breathing_rate_avg)) {
            var rate = metrics.avg_breathing_rate || metrics.breathing_rate_avg;
            avgEl.textContent = rate.toFixed(1) + ' bpm';
        }

        var varEl = document.getElementById('sleep-breathing-variability');
        if (varEl && metrics.breathing_regularity !== undefined) {
            var reg = metrics.breathing_regularity;
            var label = 'Regular';
            if (reg > 0.25) label = 'Irregular';
            else if (reg > 0.10) label = 'Moderate';
            varEl.textContent = reg.toFixed(2) + ' CV (' + label + ')';
        }
    }

    function renderHistory() {
        var listEl = document.getElementById('sleep-history-list');
        if (!listEl) return;

        listEl.innerHTML = '';

        if (_sleepRecords.length === 0) {
            listEl.innerHTML = '<div class="sleep-history-empty">No sleep data yet. Sleep monitoring requires a bedroom zone with stationary detection.</div>';
            return;
        }

        _sleepRecords.forEach(function(rec) {
            var row = document.createElement('div');
            row.className = 'sleep-history-row';

            var date = rec.date || '';
            var duration = '';
            if (rec.duration_min) {
                var h = Math.floor(rec.duration_min / 60);
                var m = rec.duration_min % 60;
                duration = h + 'h ' + m + 'm';
            }

            var effColor = 'red';
            if (rec.breathing_regularity !== undefined) {
                // Use regularity as a rough proxy if efficiency not available
                effColor = 'amber';
            }

            var breathing = '';
            if (rec.breathing_rate_avg) {
                breathing = rec.breathing_rate_avg.toFixed(1) + ' bpm';
            }

            row.innerHTML =
                '<span class="sleep-history-date">' + date + '</span>' +
                '<span class="sleep-history-duration">' + duration + '</span>' +
                '<span class="sleep-history-breathing">' + breathing + '</span>';

            listEl.appendChild(row);
        });
    }

    // ── WebSocket Message Handler ────────────────────────────────────────────

    /**
     * Handle a morning_summary WebSocket message.
     * Called from app.js handleJSONMessage when msg.type === 'morning_summary'.
     * @param {Object} msg - { type: 'morning_summary', report: { ... } }
     */
    function handleMorningSummary(msg) {
        if (msg.report) {
            showMorningSummary(msg.report);
        }
    }

    /**
     * Handle a sleep_status WebSocket message.
     * @param {Object} msg - { type: 'sleep_status', data: { ... } }
     */
    function handleSleepStatus(msg) {
        if (msg.data && _panelVisible) {
            // Update live breathing rate in panel if visible
            var states = msg.data.link_states || {};
            for (var linkID in states) {
                var ls = states[linkID];
                if (ls.current_breathing_rate > 0) {
                    var avgEl = document.getElementById('sleep-avg-breathing');
                    if (avgEl) avgEl.textContent = ls.current_breathing_rate.toFixed(1) + ' bpm (live)';
                }
            }
        }
    }

    // ── Public API ───────────────────────────────────────────────────────────

    window.SpaxelSleep = {
        init: init,
        showMorningSummary: showMorningSummary,
        dismissSummary: dismissSummary,
        showPanel: showSleepPanel,
        hidePanel: hideSleepPanel,
        togglePanel: toggleSleepPanel,
        handleMorningSummary: handleMorningSummary,
        handleSleepStatus: handleSleepStatus,
        fetchSleepData: fetchSleepData
    };
})();
