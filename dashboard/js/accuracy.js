/**
 * Accuracy Panel for Detection Quality Metrics
 * Displays precision, recall, F1 scores, and improvement trends
 */
(function() {
    'use strict';

    var Accuracy = {
        // State
        panelVisible: false,
        currentData: null,
        historyData: null,
        improvementData: null,

        // Config
        config: {
            pollIntervalMs: 60000, // 1 minute
            historyWeeks: 8
        },

        /**
         * Initialize the accuracy panel
         */
        init: function() {
            this.createPanel();
            this.addStyles();
            this.startPolling();
            console.log('[Accuracy] Module initialized');
        },

        /**
         * Create the accuracy panel
         */
        createPanel: function() {
            var panel = document.createElement('div');
            panel.id = 'accuracy-panel';
            panel.className = 'accuracy-panel';
            panel.style.display = 'none';
            panel.innerHTML = '\
                <div class="accuracy-header">\
                    <h3>Detection Accuracy</h3>\
                    <button class="accuracy-close" onclick="Accuracy.togglePanel()">&times;</button>\
                </div>\
                <div class="accuracy-content">\
                    <div class="accuracy-gauge-section">\
                        <div class="accuracy-gauge-container">\
                            <svg class="accuracy-gauge" viewBox="0 0 100 100">\
                                <circle class="gauge-bg" cx="50" cy="50" r="40"/>\
                                <circle class="gauge-fill" cx="50" cy="50" r="40"/>\
                            </svg>\
                            <div class="gauge-value">\
                                <span class="gauge-number">--</span>\
                                <span class="gauge-label">F1 Score</span>\
                            </div>\
                        </div>\
                    </div>\
                    <div class="accuracy-metrics">\
                        <div class="metric-item">\
                            <span class="metric-label">Precision</span>\
                            <span class="metric-value" id="accuracy-precision">--</span>\
                        </div>\
                        <div class="metric-item">\
                            <span class="metric-label">Recall</span>\
                            <span class="metric-value" id="accuracy-recall">--</span>\
                        </div>\
                        <div class="metric-item">\
                            <span class="metric-label">F1 Score</span>\
                            <span class="metric-value" id="accuracy-f1">--</span>\
                        </div>\
                    </div>\
                    <div class="accuracy-motivation">\
                        <div class="motivation-text">\
                            You\'ve provided <span id="feedback-count">0</span> corrections.\
                        </div>\
                        <div class="motivation-improvement" id="improvement-text"></div>\
                    </div>\
                    <div class="accuracy-trend-section">\
                        <div class="trend-header">\
                            <span>F1 Score Trend (8 weeks)</span>\
                        </div>\
                        <canvas id="accuracy-sparkline" width="240" height="60"></canvas>\
                    </div>\
                    <div class="accuracy-breakdown">\
                        <div class="breakdown-header">Per-Zone Breakdown</div>\
                        <div id="zone-breakdown" class="zone-breakdown">\
                            <div class="loading-text">Loading...</div>\
                        </div>\
                    </div>\
                    <div class="accuracy-stats">\
                        <div class="stats-row">\
                            <span>Pending corrections</span>\
                            <span id="unprocessed-count">0</span>\
                        </div>\
                        <div class="stats-row">\
                            <span>Processed corrections</span>\
                            <span id="processed-count">0</span>\
                        </div>\
                    </div>\
                </div>';
            document.body.appendChild(panel);

            // Add toggle button to status bar
            this.addToggleButton();
        },

        /**
         * Add toggle button to status bar
         */
        addToggleButton: function() {
            var statusBar = document.getElementById('status-bar');
            if (!statusBar) return;

            var btn = document.createElement('div');
            btn.className = 'status-item accuracy-toggle';
            btn.id = 'accuracy-toggle';
            btn.innerHTML = '\
                <div class="accuracy-mini-gauge">\
                    <svg viewBox="0 0 32 32">\
                        <circle cx="16" cy="16" r="12" fill="none" stroke="rgba(255,255,255,0.1)" stroke-width="3"/>\
                        <circle class="mini-gauge-fill" cx="16" cy="16" r="12" fill="none" stroke="#66bb6a" stroke-width="3"\
                            stroke-dasharray="0 75.4" stroke-linecap="round" transform="rotate(-90 16 16)"/>\
                    </svg>\
                    <span class="mini-gauge-value">--</span>\
                </div>\
                <span class="accuracy-label">Accuracy</span>';
            btn.onclick = function() { Accuracy.togglePanel(); };
            btn.style.cursor = 'pointer';

            // Insert after detection-quality
            var qualityItem = document.getElementById('detection-quality');
            if (qualityItem && qualityItem.nextSibling) {
                statusBar.insertBefore(btn, qualityItem.nextSibling);
            } else {
                statusBar.appendChild(btn);
            }
        },

        /**
         * Toggle panel visibility
         */
        togglePanel: function() {
            var panel = document.getElementById('accuracy-panel');
            if (!panel) return;

            if (this.panelVisible) {
                panel.style.display = 'none';
                this.panelVisible = false;
            } else {
                panel.style.display = 'block';
                this.panelVisible = true;
                this.refresh();
            }
        },

        /**
         * Start polling for accuracy updates
         */
        startPolling: function() {
            var self = this;
            this.refresh();

            setInterval(function() {
                self.refresh();
            }, this.config.pollIntervalMs);
        },

        /**
         * Refresh all accuracy data
         */
        refresh: function() {
            this.fetchAccuracy();
            this.fetchHistory();
            this.fetchImprovement();
            this.fetchStats();
            this.fetchZoneBreakdown();
        },

        /**
         * Fetch current accuracy metrics
         */
        fetchAccuracy: function() {
            var self = this;
            fetch('/api/learning/accuracy')
                .then(function(res) { return res.json(); })
                .then(function(data) {
                    self.currentData = data;
                    self.updateDisplay();
                })
                .catch(function(err) {
                    console.error('[Accuracy] Failed to fetch accuracy:', err);
                });
        },

        /**
         * Fetch accuracy history for sparkline
         */
        fetchHistory: function() {
            var self = this;
            fetch('/api/learning/accuracy/history?weeks=' + this.config.historyWeeks)
                .then(function(res) { return res.json(); })
                .then(function(data) {
                    self.historyData = data;
                    self.drawSparkline();
                })
                .catch(function(err) {
                    console.error('[Accuracy] Failed to fetch history:', err);
                });
        },

        /**
         * Fetch improvement statistics
         */
        fetchImprovement: function() {
            var self = this;
            fetch('/api/learning/accuracy/improvement')
                .then(function(res) { return res.json(); })
                .then(function(data) {
                    self.improvementData = data;
                    self.updateMotivation();
                })
                .catch(function(err) {
                    console.error('[Accuracy] Failed to fetch improvement:', err);
                });
        },

        /**
         * Fetch feedback stats
         */
        fetchStats: function() {
            fetch('/api/learning/stats')
                .then(function(res) { return res.json(); })
                .then(function(data) {
                    document.getElementById('unprocessed-count').textContent = data.unprocessed_count || 0;
                    document.getElementById('processed-count').textContent = data.processed_count || 0;
                })
                .catch(function(err) {
                    console.error('[Accuracy] Failed to fetch stats:', err);
                });
        },

        /**
         * Fetch per-zone accuracy breakdown
         */
        fetchZoneBreakdown: function() {
            var self = this;
            var week = this.getCurrentWeek();

            fetch('/api/learning/accuracy/history?scope_type=zone&weeks=1')
                .then(function(res) { return res.json(); })
                .then(function(data) {
                    self.renderZoneBreakdown(data);
                })
                .catch(function(err) {
                    console.error('[Accuracy] Failed to fetch zone breakdown:', err);
                    self.renderZoneBreakdown([]);
                });
        },

        /**
         * Get current week string
         */
        getCurrentWeek: function() {
            var now = new Date();
            var start = new Date(now.getFullYear(), 0, 1);
            var diff = now - start;
            var oneWeek = 604800000; // ms in a week
            var weekNum = Math.ceil((diff + start.getDay() * 86400000) / oneWeek);
            return now.getFullYear() + '-W' + (weekNum < 10 ? '0' : '') + weekNum;
        },

        /**
         * Render zone breakdown
         */
        renderZoneBreakdown: function(zones) {
            var container = document.getElementById('zone-breakdown');
            if (!container) return;

            if (!zones || zones.length === 0) {
                container.innerHTML = '<div class="no-data-text">No zone data yet</div>';
                return;
            }

            var self = this;
            var html = '';
            zones.sort(function(a, b) { return (b.f1 || 0) - (a.f1 || 0); });

            zones.forEach(function(zone) {
                var f1 = zone.f1 !== null ? (zone.f1 * 100).toFixed(0) + '%' : '--';
                var color = zone.f1 >= 0.8 ? '#66bb6a' : (zone.f1 >= 0.6 ? '#ffa726' : '#ef5350');

                html += '<div class="zone-item" data-zone-id="' + zone.scope_id + '">' +
                    '<span class="zone-name">' + self.formatZoneName(zone.scope_id) + '</span>' +
                    '<span class="zone-score" style="color:' + color + '">' + f1 + '</span>' +
                    '</div>';
            });

            container.innerHTML = html;

            // Add click handlers to focus on zone
            container.querySelectorAll('.zone-item').forEach(function(item) {
                item.onclick = function() {
                    var zoneId = this.getAttribute('data-zone-id');
                    if (window.Viz3D && window.Viz3D.focusOnZone) {
                        window.Viz3D.focusOnZone(zoneId);
                    }
                };
            });
        },

        /**
         * Format zone name for display
         */
        formatZoneName: function(zoneId) {
            if (!zoneId) return 'Unknown';
            // Convert zone-xxx to Xxx
            return zoneId.replace(/^zone-/, '').replace(/-/g, ' ')
                .replace(/\b\w/g, function(c) { return c.toUpperCase(); });
        },

        /**
         * Update display with current data
         */
        updateDisplay: function() {
            if (!this.currentData) return;

            var precision = this.currentData.precision;
            var recall = this.currentData.recall;
            var f1 = this.currentData.f1;

            // Update metrics
            document.getElementById('accuracy-precision').textContent = this.formatPercent(precision);
            document.getElementById('accuracy-recall').textContent = this.formatPercent(recall);
            document.getElementById('accuracy-f1').textContent = this.formatPercent(f1);

            // Update gauge
            var gaugeValue = document.querySelector('.gauge-number');
            if (gaugeValue) {
                gaugeValue.textContent = this.formatPercent(f1);
            }

            // Update gauge fill
            var gaugeFill = document.querySelector('.gauge-fill');
            if (gaugeFill && f1 !== null) {
                var circumference = 2 * Math.PI * 40;
                var offset = circumference * (1 - f1);
                gaugeFill.style.strokeDasharray = (circumference - offset) + ' ' + circumference;
                gaugeFill.style.stroke = this.getColorForScore(f1);
            }

            // Update mini gauge in status bar
            this.updateMiniGauge(f1);
        },

        /**
         * Update mini gauge in status bar
         */
        updateMiniGauge: function(f1) {
            var miniFill = document.querySelector('.mini-gauge-fill');
            var miniValue = document.querySelector('.mini-gauge-value');

            if (miniFill && f1 !== null) {
                var circumference = 2 * Math.PI * 12;
                var offset = circumference * (1 - f1);
                miniFill.style.strokeDasharray = (circumference - offset) + ' ' + circumference;
                miniFill.style.stroke = this.getColorForScore(f1);
            }

            if (miniValue) {
                miniValue.textContent = this.formatPercent(f1);
            }
        },

        /**
         * Update motivation section
         */
        updateMotivation: function() {
            if (!this.improvementData) return;

            var feedbackCount = document.getElementById('feedback-count');
            var improvementText = document.getElementById('improvement-text');

            if (feedbackCount) {
                feedbackCount.textContent = this.improvementData.total_feedback || 0;
            }

            if (improvementText) {
                var improvement = this.improvementData.improvement_pct || 0;
                if (improvement > 0) {
                    improvementText.innerHTML = '<span class="improvement-positive">Accuracy improved ' +
                        improvement.toFixed(0) + '% this week!</span>';
                } else if (improvement < 0) {
                    improvementText.innerHTML = '<span class="improvement-negative">Accuracy decreased ' +
                        Math.abs(improvement).toFixed(0) + '% this week.</span>';
                } else {
                    improvementText.innerHTML = '<span class="improvement-neutral">Keep providing feedback to improve accuracy!</span>';
                }
            }
        },

        /**
         * Draw sparkline for accuracy history
         */
        drawSparkline: function() {
            var canvas = document.getElementById('accuracy-sparkline');
            if (!canvas || !this.historyData || this.historyData.length === 0) return;

            var ctx = canvas.getContext('2d');
            var width = canvas.width;
            var height = canvas.height;
            var padding = 4;

            ctx.clearRect(0, 0, width, height);

            // Sort by week
            var data = this.historyData.slice().sort(function(a, b) {
                return a.week.localeCompare(b.week);
            });

            if (data.length < 2) {
                ctx.fillStyle = '#666';
                ctx.font = '11px sans-serif';
                ctx.textAlign = 'center';
                ctx.fillText('Need more data...', width / 2, height / 2);
                return;
            }

            // Get F1 values
            var values = data.map(function(d) { return d.f1 || 0; });
            var min = Math.min.apply(null, values);
            var max = Math.max.apply(null, values);
            if (max === min) max = min + 0.1;

            // Draw line
            ctx.beginPath();
            ctx.strokeStyle = '#4fc3f7';
            ctx.lineWidth = 2;

            var stepX = (width - padding * 2) / (data.length - 1);

            for (var i = 0; i < data.length; i++) {
                var x = padding + i * stepX;
                var y = height - padding - ((values[i] - min) / (max - min)) * (height - padding * 2);

                if (i === 0) {
                    ctx.moveTo(x, y);
                } else {
                    ctx.lineTo(x, y);
                }
            }

            ctx.stroke();

            // Draw points
            ctx.fillStyle = '#4fc3f7';
            for (var i = 0; i < data.length; i++) {
                var x = padding + i * stepX;
                var y = height - padding - ((values[i] - min) / (max - min)) * (height - padding * 2);
                ctx.beginPath();
                ctx.arc(x, y, 3, 0, Math.PI * 2);
                ctx.fill();
            }
        },

        /**
         * Format a decimal as percentage
         */
        formatPercent: function(value) {
            if (value === null || value === undefined) return '--';
            return (value * 100).toFixed(0) + '%';
        },

        /**
         * Get color for a score (0-1)
         */
        getColorForScore: function(score) {
            if (score >= 0.8) return '#66bb6a';
            if (score >= 0.6) return '#ffa726';
            return '#ef5350';
        },

        /**
         * Add CSS styles
         */
        addStyles: function() {
            if (document.getElementById('accuracy-styles')) return;

            var style = document.createElement('style');
            style.id = 'accuracy-styles';
            style.textContent = '\
                .accuracy-panel {\
                    position: fixed;\
                    top: 60px;\
                    right: 20px;\
                    width: 300px;\
                    max-height: calc(100vh - 80px);\
                    background: rgba(0, 0, 0, 0.9);\
                    border-radius: 8px;\
                    box-shadow: 0 4px 20px rgba(0, 0, 0, 0.5);\
                    z-index: 150;\
                    overflow-y: auto;\
                }\
                .accuracy-header {\
                    display: flex;\
                    justify-content: space-between;\
                    align-items: center;\
                    padding: 12px 16px;\
                    border-bottom: 1px solid rgba(255, 255, 255, 0.1);\
                }\
                .accuracy-header h3 {\
                    font-size: 14px;\
                    color: #888;\
                    text-transform: uppercase;\
                    letter-spacing: 1px;\
                    margin: 0;\
                }\
                .accuracy-close {\
                    background: none;\
                    border: none;\
                    color: #888;\
                    font-size: 20px;\
                    cursor: pointer;\
                }\
                .accuracy-close:hover { color: #fff; }\
                .accuracy-content {\
                    padding: 16px;\
                }\
                .accuracy-gauge-section {\
                    display: flex;\
                    justify-content: center;\
                    margin-bottom: 16px;\
                }\
                .accuracy-gauge-container {\
                    position: relative;\
                    width: 120px;\
                    height: 120px;\
                }\
                .accuracy-gauge {\
                    width: 100%;\
                    height: 100%;\
                    transform: rotate(-90deg);\
                }\
                .gauge-bg {\
                    fill: none;\
                    stroke: rgba(255, 255, 255, 0.1);\
                    stroke-width: 8;\
                }\
                .gauge-fill {\
                    fill: none;\
                    stroke: #66bb6a;\
                    stroke-width: 8;\
                    stroke-linecap: round;\
                    stroke-dasharray: 0 251;\
                    transition: stroke-dasharray 0.5s, stroke 0.3s;\
                }\
                .gauge-value {\
                    position: absolute;\
                    top: 50%;\
                    left: 50%;\
                    transform: translate(-50%, -50%);\
                    text-align: center;\
                }\
                .gauge-number {\
                    display: block;\
                    font-size: 24px;\
                    font-weight: 600;\
                    color: #fff;\
                }\
                .gauge-label {\
                    display: block;\
                    font-size: 10px;\
                    color: #888;\
                    text-transform: uppercase;\
                }\
                .accuracy-metrics {\
                    display: flex;\
                    justify-content: space-around;\
                    padding: 12px 0;\
                    border-top: 1px solid rgba(255, 255, 255, 0.1);\
                    border-bottom: 1px solid rgba(255, 255, 255, 0.1);\
                    margin-bottom: 12px;\
                }\
                .metric-item {\
                    text-align: center;\
                }\
                .metric-label {\
                    display: block;\
                    font-size: 10px;\
                    color: #888;\
                    margin-bottom: 2px;\
                }\
                .metric-value {\
                    font-size: 16px;\
                    font-weight: 600;\
                    color: #eee;\
                }\
                .accuracy-motivation {\
                    background: rgba(79, 195, 247, 0.1);\
                    border-radius: 6px;\
                    padding: 10px 12px;\
                    margin-bottom: 12px;\
                    text-align: center;\
                }\
                .motivation-text {\
                    font-size: 12px;\
                    color: #bbb;\
                }\
                .motivation-text span {\
                    font-weight: 600;\
                    color: #4fc3f7;\
                }\
                .motivation-improvement {\
                    font-size: 11px;\
                    margin-top: 4px;\
                }\
                .improvement-positive { color: #66bb6a; }\
                .improvement-negative { color: #ef5350; }\
                .improvement-neutral { color: #888; }\
                .accuracy-trend-section {\
                    margin-bottom: 12px;\
                }\
                .trend-header {\
                    font-size: 11px;\
                    color: #888;\
                    margin-bottom: 8px;\
                }\
                #accuracy-sparkline {\
                    width: 100%;\
                    background: rgba(255, 255, 255, 0.03);\
                    border-radius: 4px;\
                }\
                .accuracy-breakdown {\
                    margin-bottom: 12px;\
                }\
                .breakdown-header {\
                    font-size: 11px;\
                    color: #888;\
                    margin-bottom: 8px;\
                }\
                .zone-breakdown {\
                    display: flex;\
                    flex-direction: column;\
                    gap: 4px;\
                }\
                .zone-item {\
                    display: flex;\
                    justify-content: space-between;\
                    align-items: center;\
                    font-size: 11px;\
                    padding: 4px 8px;\
                    background: rgba(255, 255, 255, 0.03);\
                    border-radius: 3px;\
                    cursor: pointer;\
                    transition: background 0.2s;\
                }\
                .zone-item:hover {\
                    background: rgba(255, 255, 255, 0.08);\
                }\
                .zone-name {\
                    color: #bbb;\
                }\
                .zone-score {\
                    font-weight: 500;\
                }\
                .no-data-text {\
                    color: #666;\
                    font-size: 11px;\
                    text-align: center;\
                    padding: 8px;\
                }\
                .accuracy-stats {\
                    font-size: 11px;\
                }\
                .stats-row {\
                    display: flex;\
                    justify-content: space-between;\
                    padding: 4px 0;\
                    color: #888;\
                }\
                .stats-row span:last-child {\
                    color: #ccc;\
                }\
                .accuracy-toggle {\
                    padding: 2px 10px;\
                    background: rgba(255, 255, 255, 0.05);\
                    border-radius: 4px;\
                }\
                .accuracy-toggle:hover {\
                    background: rgba(255, 255, 255, 0.1);\
                }\
                .accuracy-mini-gauge {\
                    position: relative;\
                    width: 32px;\
                    height: 32px;\
                }\
                .accuracy-mini-gauge svg {\
                    width: 32px;\
                    height: 32px;\
                }\
                .mini-gauge-value {\
                    position: absolute;\
                    top: 50%;\
                    left: 50%;\
                    transform: translate(-50%, -50%);\
                    font-size: 8px;\
                    font-weight: 600;\
                    color: #ccc;\
                }\
                .accuracy-label {\
                    font-size: 11px;\
                    color: #888;\
                }';
            document.head.appendChild(style);
        }
    };

    // Expose globally
    window.Accuracy = Accuracy;

    // Initialize when DOM is ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', function() { Accuracy.init(); });
    } else {
        Accuracy.init();
    }
})();
