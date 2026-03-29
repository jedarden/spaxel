/**
 * Spaxel Link Health Panel
 *
 * Displays link diagnostics, weekly health trends (sparkline),
 * and repositioning advice with ghost node rendering.
 *
 * Integrates with Viz3D for 3D ghost node visualization.
 */
(function () {
    'use strict';

    // ============================================
    // Configuration
    // ============================================
    var CONFIG = {
        pollIntervalMs: 30000,      // Poll diagnostics every 30s
        historyWindowHours: 24,     // Default history window
        sparklineWidth: 120,
        sparklineHeight: 30,
        sparklinePoints: 7,         // 7 days
    };

    // ============================================
    // Internal State
    // ============================================
    var state = {
        selectedLinkID: null,
        diagnostics: {},            // linkID -> [diagnosis]
        weeklyTrends: {},           // linkID -> [dailySummary]
        healthHistory: {},          // linkID -> [healthLogEntry]
        linkHealthData: {},         // linkID -> { score, details }
        panel: null,
        pollTimer: null,
    };

    // ============================================
    // Initialization
    // ============================================
    function init() {
        var container = document.getElementById('link-health-panel');
        if (!container) {
            // Create panel if it doesn't exist
            var sidebar = document.querySelector('.sidebar') || document.body;
            container = document.createElement('div');
            container.id = 'link-health-panel';
            container.className = 'link-health-panel';
            sidebar.appendChild(container);
        }
        state.panel = container;

        // Start polling for diagnostics
        state.pollTimer = setInterval(fetchAllDiagnostics, CONFIG.pollIntervalMs);

        // Initial fetch
        fetchAllDiagnostics();
    }

    // ============================================
    // API Fetching
    // ============================================
    function fetchAllDiagnostics() {
        fetch('/api/diagnostics')
            .then(function (res) { return res.json(); })
            .then(function (data) {
                state.diagnostics = data || {};
                renderPanel();
            })
            .catch(function (err) {
                console.error('[LinkHealth] Failed to fetch diagnostics:', err);
            });
    }

    function fetchWeeklyTrend(linkID) {
        fetch('/api/weather/' + encodeURIComponent(linkID) + '/weekly')
            .then(function (res) { return res.json(); })
            .then(function (data) {
                state.weeklyTrends[linkID] = data || [];
                renderPanel();
            })
            .catch(function (err) {
                console.error('[LinkHealth] Failed to fetch weekly trend:', err);
            });
    }

    function fetchHealthHistory(linkID) {
        var url = '/api/links/' + encodeURIComponent(linkID) + '/health-history?window=' + CONFIG.historyWindowHours;
        fetch(url)
            .then(function (res) { return res.json(); })
            .then(function (data) {
                state.healthHistory[linkID] = data || [];
                renderPanel();
            })
            .catch(function (err) {
                console.error('[LinkHealth] Failed to fetch health history:', err);
            });
    }

    // ============================================
    // Metric Interpretation
    // ============================================
    function interpretMetric(metric, value) {
        if (value >= 0.8) return { label: 'Excellent', class: 'metric-excellent' };
        if (value >= 0.6) return { label: 'Good', class: 'metric-good' };
        if (value >= 0.4) return { label: 'Fair', class: 'metric-fair' };
        if (value >= 0.2) return { label: 'Poor', class: 'metric-poor' };
        return { label: 'Critical', class: 'metric-critical' };
    }

    function getWhyLowHint(details, compositeScore) {
        if (compositeScore >= 0.7) return null; // No hint needed

        // Find the lowest sub-metric
        var metrics = [
            { key: 'snr', value: details.snr || 0, label: 'Signal quality', hint: 'Check for interference from other WiFi networks or physical obstructions in the Fresnel zone.' },
            { key: 'phase_stability', value: details.phase_stability || 0, label: 'Phase stability', hint: 'This may indicate temperature fluctuations or clock drift between TX and RX nodes.' },
            { key: 'packet_rate', value: details.packet_rate || 0, label: 'Packet rate', hint: 'Packets may be dropping due to congestion. Check for other devices on the same WiFi channel.' },
            { key: 'baseline_drift', value: details.baseline_drift || 0, label: 'Baseline stability', hint: 'The environment is changing. This can be caused by moving furniture, doors opening/closing, or temperature changes.' }
        ];

        // Sort by value ascending to find lowest
        metrics.sort(function(a, b) { return a.value - b.value; });
        var lowest = metrics[0];

        return {
            metric: lowest.label,
            value: lowest.value,
            hint: lowest.hint
        };
    }

    // ============================================
    // Panel Rendering
    // ============================================
    function renderPanel() {
        if (!state.panel) return;

        var linkID = state.selectedLinkID;
        if (!linkID) {
            state.panel.innerHTML = '<div class="link-health-empty">Select a link to view diagnostics</div>';
            return;
        }

        var diagnoses = state.diagnostics[linkID] || [];
        var weeklyTrend = state.weeklyTrends[linkID] || [];
        var healthData = state.linkHealthData[linkID] || {};
        var healthScore = healthData.score !== undefined ? healthData.score : 0.5;
        var healthDetails = healthData.details || { snr: 0.5, phase_stability: 0.5, packet_rate: 0.5, baseline_drift: 0.5 };

        var html = '<div class="link-health-content">' +
            '<div class="link-health-header">' +
                '<h3>Link Health</h3>' +
                '<span class="link-health-id">' + escapeHtml(abbreviateLinkID(linkID)) + '</span>' +
            '</div>';

        // Composite score gauge
        var scoreClass = healthScore >= 0.7 ? 'score-good' : (healthScore >= 0.4 ? 'score-fair' : 'score-poor');
        html += '<div class="link-health-composite">' +
            '<div class="composite-gauge">' +
                '<div class="gauge-fill ' + scoreClass + '" style="width: ' + (healthScore * 100) + '%"></div>' +
            '</div>' +
            '<span class="composite-score">' + Math.round(healthScore * 100) + '%</span>' +
            '<span class="composite-label">Overall Health</span>' +
        '</div>';

        // Per-metric breakdown
        html += '<div class="link-health-metrics">';
        html += renderMetricGauge('SNR', healthDetails.snr || 0, 'Signal-to-noise ratio');
        html += renderMetricGauge('Phase', healthDetails.phase_stability || 0, 'Phase stability');
        html += renderMetricGauge('Rate', healthDetails.packet_rate || 0, 'Packet rate health');
        html += renderMetricGauge('Drift', healthDetails.baseline_drift || 0, 'Baseline stability');
        html += '</div>';

        // "Why is this low?" contextual hint
        var hint = getWhyLowHint(healthDetails, healthScore);
        if (hint) {
            html += '<div class="link-health-hint">' +
                '<div class="hint-header">Why is this low?</div>' +
                '<div class="hint-metric">' + escapeHtml(hint.metric) + ' at ' + Math.round(hint.value * 100) + '%</div>' +
                '<div class="hint-text">' + escapeHtml(hint.hint) + '</div>' +
            '</div>';
        }

        // 24-hour health history sparkline
        html += '<div class="link-health-sparkline-section">' +
            '<span class="sparkline-label">24-Hour Trend</span>' +
            renderHealthSparkline(linkID) +
        '</div>';

        // Weekly sparkline
        html += '<div class="link-health-sparkline-section">' +
            '<span class="sparkline-label">7-Day Trend</span>' +
            renderSparkline(weeklyTrend) +
            '<span class="sparkline-annotations">' + renderSparklineAnnotations(weeklyTrend) + '</span>' +
        '</div>';

        // Diagnoses list
        if (diagnoses.length === 0) {
            html += '<div class="link-health-no-issues">' +
                '<span class="no-issues-icon">&#10003;</span>' +
                '<span>No issues detected</span>' +
            '</div>';
        } else {
            html += '<div class="link-health-diagnoses">';
            diagnoses.forEach(function (d) {
                html += renderDiagnosisCard(d);
            });
            html += '</div>';
        }

        html += '</div>';

        state.panel.innerHTML = html;

        // Add click handlers for repositioning advice
        state.panel.querySelectorAll('.reposition-apply-btn').forEach(function (btn) {
            btn.addEventListener('click', function () {
                var x = parseFloat(btn.dataset.x);
                var z = parseFloat(btn.dataset.z);
                var nodeMac = btn.dataset.nodeMac;
                applyRepositioning(nodeMac, x, z);
            });
        });

        // Show ghost node in 3D view if there's a repositioning target
        showGhostNodeForDiagnosis(diagnoses);

        // Trigger sparkline drawing after render
        setTimeout(drawSparklines, 0);
    }

    function renderMetricGauge(name, value, tooltip) {
        var interp = interpretMetric(name, value);
        return '<div class="metric-gauge" title="' + escapeHtml(tooltip) + '">' +
            '<div class="metric-bar">' +
                '<div class="metric-fill ' + interp.class + '" style="width: ' + (value * 100) + '%"></div>' +
            '</div>' +
            '<div class="metric-info">' +
                '<span class="metric-name">' + escapeHtml(name) + '</span>' +
                '<span class="metric-value">' + Math.round(value * 100) + '%</span>' +
            '</div>' +
        '</div>';
    }

    function renderHealthSparkline(linkID) {
        var history = state.healthHistory[linkID] || [];
        if (history.length === 0) {
            return '<canvas class="sparkline-canvas" width="' + CONFIG.sparklineWidth + '" height="' + CONFIG.sparklineHeight + '" data-empty="true"></canvas>';
        }

        // Use composite_score from history
        var points = history.map(function (entry) {
            return entry.composite_score || entry.CompositeScore || 0.5;
        });

        return '<canvas class="sparkline-canvas" width="' + CONFIG.sparklineWidth + '" height="' + CONFIG.sparklineHeight + '" data-points="' + points.join(',') + '"></canvas>';
    }

    function renderSparkline(weeklyTrend) {
        if (!weeklyTrend || weeklyTrend.length === 0) {
            return '<canvas class="sparkline-canvas" width="' + CONFIG.sparklineWidth + '" height="' + CONFIG.sparklineHeight + '" data-empty="true"></canvas>';
        }

        var canvas = '<canvas class="sparkline-canvas" width="' + CONFIG.sparklineWidth + '" height="' + CONFIG.sparklineHeight + '" data-points="';
        var points = weeklyTrend.map(function (d) {
            return d.avg_health || d.mean_health || 0.5;
        });
        canvas += points.join(',') + '"></canvas>';
        return canvas;
    }

    function renderSparklineAnnotations(weeklyTrend) {
        if (!weeklyTrend || weeklyTrend.length === 0) {
            return '<span class="sparkline-empty">No data</span>';
        }

        var scores = weeklyTrend.map(function (d) {
            return d.avg_health || d.mean_health || 0.5;
        });
        var max = Math.max.apply(null, scores);
        var min = Math.min.apply(null, scores);
        var maxIdx = scores.indexOf(max);
        var minIdx = scores.indexOf(min);

        var annotations = [];
        if (weeklyTrend[maxIdx]) {
            var bestDate = weeklyTrend[maxIdx].date || '';
            annotations.push('<span class="sparkline-best" title="Best day">Best: ' + (bestDate.toString().substring(0, 10) || 'N/A') + '</span>');
        }
        if (weeklyTrend[minIdx] && minIdx !== maxIdx) {
            var worstDate = weeklyTrend[minIdx].date || '';
            annotations.push('<span class="sparkline-worst" title="Worst day">Worst: ' + (worstDate.toString().substring(0, 10) || 'N/A') + '</span>');
        }

        return annotations.join(' ');
    }

    function renderDiagnosisCard(d) {
        var severityClass = 'severity-' + d.severity.toLowerCase();
        var severityIcon = getSeverityIcon(d.severity);

        var html = '<div class="diagnosis-card ' + severityClass + '">' +
            '<div class="diagnosis-header">' +
                '<span class="diagnosis-icon">' + severityIcon + '</span>' +
                '<span class="diagnosis-title">' + escapeHtml(d.title || '') + '</span>' +
                '<span class="diagnosis-confidence">' + Math.round((d.confidence_score || 0) * 100) + '%</span>' +
            '</div>' +
            '<div class="diagnosis-detail">' + escapeHtml(d.detail || '') + '</div>' +
            '<div class="diagnosis-advice">' + escapeHtml(d.advice || '') + '</div>';

        // Repositioning target button
        if (d.repositioning_target && d.repositioning_node_mac) {
            var target = d.repositioning_target;
            html += '<div class="diagnosis-reposition">' +
                '<div class="reposition-target">' +
                    '<span>Move to: X=' + target.x.toFixed(2) + 'm, Z=' + target.z.toFixed(2) + 'm</span>' +
                    (d.gdop_improvement ? '<span class="gdop-improvement">GDOP improvement: +' + (d.gdop_improvement * 100).toFixed(0) + '%</span>' : '') +
                '</div>' +
                '<button class="reposition-apply-btn" data-node-mac="' + escapeHtml(d.repositioning_node_mac) + '" data-x="' + target.x + '" data-z="' + target.z + '">' +
                    'Show in 3D' +
                '</button>' +
            '</div>';
        }

        html += '</div>';
        return html;
    }

    function getSeverityIcon(severity) {
        switch (severity) {
            case 'INFO': return '&#9432;';
            case 'WARNING': return '&#9888;';
            case 'ACTIONABLE': return '&#9888;&#65039;';
            default: return '&#9432;';
        }
    }

    // ============================================
    // Sparkline Drawing
    // ============================================
    function drawSparklines() {
        var canvases = document.querySelectorAll('.sparkline-canvas[data-points]');
        canvases.forEach(function (canvas) {
            var pointsStr = canvas.dataset.points;
            if (!pointsStr) return;

            var points = pointsStr.split(',').map(parseFloat);
            if (points.length === 0) return;

            var ctx = canvas.getContext('2d');
            var w = canvas.width;
            var h = canvas.height;
            var pad = 2;

            // Clear
            ctx.fillStyle = '#1a1a2e';
            ctx.fillRect(0, 0, w, h);

            // Normalize points
            var min = Math.min.apply(null, points);
            var max = Math.max.apply(null, points);
            var range = max - min || 1;

            // Draw line
            ctx.strokeStyle = '#4fc3f7';
            ctx.lineWidth = 1.5;
            ctx.beginPath();

            var step = (w - pad * 2) / Math.max(points.length - 1, 1);
            for (var i = 0; i < points.length; i++) {
                var x = pad + i * step;
                var y = h - pad - ((points[i] - min) / range) * (h - pad * 2);
                if (i === 0) ctx.moveTo(x, y);
                else ctx.lineTo(x, y);
            }
            ctx.stroke();

            // Fill area under curve
            ctx.lineTo(pad + (points.length - 1) * step, h - pad);
            ctx.lineTo(pad, h - pad);
            ctx.closePath();
            ctx.fillStyle = 'rgba(79, 195, 247, 0.15)';
            ctx.fill();

            // Mark best and worst points
            var bestIdx = points.indexOf(max);
            var worstIdx = points.indexOf(min);
            if (bestIdx === worstIdx) worstIdx = -1;

            // Best point (green)
            ctx.fillStyle = '#66bb6a';
            ctx.beginPath();
            ctx.arc(pad + bestIdx * step, h - pad - ((points[bestIdx] - min) / range) * (h - pad * 2), 3, 0, Math.PI * 2);
            ctx.fill();

            // Worst point (red)
            if (worstIdx >= 0) {
                ctx.fillStyle = '#ef5350';
                ctx.beginPath();
                ctx.arc(pad + worstIdx * step, h - pad - ((points[worstIdx] - min) / range) * (h - pad * 2), 3, 0, Math.PI * 2);
                ctx.fill();
            }
        });

        // Handle empty sparklines
        var emptyCanvases = document.querySelectorAll('.sparkline-canvas[data-empty="true"]');
        emptyCanvases.forEach(function (canvas) {
            var ctx = canvas.getContext('2d');
            ctx.fillStyle = '#1a1a2e';
            ctx.fillRect(0, 0, canvas.width, canvas.height);
            ctx.fillStyle = '#444';
            ctx.font = '10px sans-serif';
            ctx.textAlign = 'center';
            ctx.fillText('No data', canvas.width / 2, canvas.height / 2 + 3);
        });
    }

    // ============================================
    // Ghost Node Visualization
    // ============================================
    function showGhostNodeForDiagnosis(diagnoses) {
        // Find first diagnosis with a repositioning target
        var targetDiagnosis = null;
        for (var i = 0; i < diagnoses.length; i++) {
            if (diagnoses[i].repositioning_target && diagnoses[i].repositioning_node_mac) {
                targetDiagnosis = diagnoses[i];
                break;
            }
        }

        if (window.Viz3D && window.Viz3D.setGhostNode) {
            if (targetDiagnosis) {
                var target = targetDiagnosis.repositioning_target;
                window.Viz3D.setGhostNode(
                    targetDiagnosis.repositioning_node_mac,
                    target.x,
                    target.y || 1.5,
                    target.z
                );
            } else {
                window.Viz3D.clearGhostNode();
            }
        }
    }

    function applyRepositioning(nodeMac, x, z) {
        // Update the node position via API
        fetch('/api/nodes/' + encodeURIComponent(nodeMac) + '/position', {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ x: x, y: 1.5, z: z })
        })
            .then(function (res) {
                if (res.ok) {
                    console.log('[LinkHealth] Node position updated');
                    if (window.Viz3D) window.Viz3D.clearGhostNode();
                } else {
                    return res.text().then(function (text) {
                        throw new Error(text);
                    });
                }
            })
            .catch(function (err) {
                console.error('[LinkHealth] Failed to update node position:', err);
                alert('Failed to update node position: ' + err.message);
            });
    }

    // ============================================
    // Selection
    // ============================================
    function selectLink(linkID) {
        state.selectedLinkID = linkID;
        if (linkID) {
            fetchWeeklyTrend(linkID);
            fetchHealthHistory(linkID);
        }
        renderPanel();
        // Trigger sparkline drawing after render
        setTimeout(drawSparklines, 0);
    }

    // ============================================
    // Health Data Updates
    // ============================================
    function updateLinkHealth(links) {
        if (!links) return;
        links.forEach(function (link) {
            var id = link.link_id || (link.tx_mac && link.rx_mac ? link.tx_mac + ':' + link.rx_mac : null);
            if (!id) return;
            state.linkHealthData[id] = {
                score: link.health_score !== undefined ? link.health_score : 0.5,
                details: link.health_details || {},
                last_updated: link.last_updated
            };
        });
        // Re-render if the selected link was updated
        if (state.selectedLinkID && state.linkHealthData[state.selectedLinkID]) {
            renderPanel();
        }
        // Also update 3D visualization
        if (window.Viz3D && window.Viz3D.updateLinkHealth) {
            window.Viz3D.updateLinkHealth(links);
        }
    }

    function setLinkHealth(linkID, score, details) {
        state.linkHealthData[linkID] = {
            score: score,
            details: details || {},
            last_updated: new Date().toISOString()
        };
        if (state.selectedLinkID === linkID) {
            renderPanel();
        }
    }

    // ============================================
    // Utilities
    // ============================================
    function escapeHtml(s) {
        if (!s) return '';
        return String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
    }

    function abbreviateLinkID(linkID) {
        if (!linkID) return '';
        var parts = linkID.split(':');
        if (parts.length >= 12) {
            var nodeShort = parts.slice(3, 6).join(':');
            var peerShort = parts.slice(9, 12).join(':');
            return nodeShort + '\u2192' + peerShort;
        }
        return linkID.substring(0, 17) + '...';
    }

    // ============================================
    // Public API
    // ============================================
    window.LinkHealth = {
        init: init,
        selectLink: selectLink,
        refresh: fetchAllDiagnostics,
        drawSparklines: drawSparklines,
        updateLinkHealth: updateLinkHealth,
        setLinkHealth: setLinkHealth,
        getLinkHealth: function (linkID) { return state.linkHealthData[linkID]; },
    };

    // Auto-init when DOM is ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', function () {
            init();
            // Draw sparklines after initial render
            setTimeout(drawSparklines, 100);
        });
    } else {
        init();
        setTimeout(drawSparklines, 100);
    }
})();
