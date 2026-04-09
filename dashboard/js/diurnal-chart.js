// Diurnal Baseline Visualization - 24-hour polar chart
// Shows baseline amplitude variance by hour with confidence coloring

var DiurnalChart = (function() {
    'use strict';

    var canvas = null;
    var ctx = null;
    var currentLinkID = null;
    var slotData = null;

    // Confidence colors: green (ready), amber (partial), red (no data)
    var CONFIDENCE_COLORS = {
        high: { fill: 'rgba(76, 175, 80, 0.8)', stroke: 'rgba(76, 175, 80, 1)' },    // green
        medium: { fill: 'rgba(255, 152, 0, 0.8)', stroke: 'rgba(255, 152, 0, 1)' },  // orange
        low: { fill: 'rgba(244, 67, 54, 0.8)', stroke: 'rgba(244, 67, 54, 1)' }     // red
    };

    function init() {
        canvas = document.getElementById('diurnal-chart');
        if (!canvas) {
            // Create canvas if it doesn't exist
            var panel = document.getElementById('chart-panel');
            if (panel) {
                var container = document.createElement('div');
                container.id = 'diurnal-chart-container';
                container.style.cssText = 'margin-top: 20px; padding: 10px; background: rgba(0,0,0,0.2); border-radius: 8px;';
                container.innerHTML = '<h4 style="margin: 0 0 10px 0; color: #888;">24-Hour Diurnal Baseline</h4>' +
                    '<div id="diurnal-chart-legend">' +
                    '<div class="legend-item"><div class="legend-color" style="background: rgba(76, 175, 80, 0.8)"></div>Ready</div>' +
                    '<div class="legend-item"><div class="legend-color" style="background: rgba(255, 152, 0, 0.8)"></div>Learning</div>' +
                    '<div class="legend-item"><div class="legend-color" style="background: rgba(244, 67, 54, 0.8)"></div>No Data</div>' +
                    '</div>' +
                    '<canvas id="diurnal-chart" width="300" height="300"></canvas>';
                panel.appendChild(container);
                canvas = document.getElementById('diurnal-chart');
            }
        }

        if (canvas) {
            ctx = canvas.getContext('2d');
            // Handle high DPI displays
            var dpr = window.devicePixelRatio || 1;
            var rect = canvas.getBoundingClientRect();
            canvas.width = rect.width * dpr;
            canvas.height = rect.height * dpr;
            ctx.scale(dpr, dpr);
        }
    }

    function getConfidenceColor(confidence) {
        if (confidence >= 1.0) {
            return CONFIDENCE_COLORS.high;
        } else if (confidence >= 0.5) {
            return CONFIDENCE_COLORS.medium;
        } else {
            return CONFIDENCE_COLORS.low;
        }
    }

    function render(data) {
        if (!ctx || !canvas) {
            init();
            if (!ctx) return;
        }

        slotData = data;

        var width = canvas.width / (window.devicePixelRatio || 1);
        var height = canvas.height / (window.devicePixelRatio || 1);
        var centerX = width / 2;
        var centerY = height / 2;
        var radius = Math.min(width, height) / 2 - 20;

        // Clear canvas
        ctx.clearRect(0, 0, width, height);

        // Draw background circle
        ctx.beginPath();
        ctx.arc(centerX, centerY, radius, 0, 2 * Math.PI);
        ctx.fillStyle = 'rgba(60, 60, 60, 0.3)';
        ctx.fill();
        ctx.strokeStyle = 'rgba(255, 255, 255, 0.1)';
        ctx.stroke();

        // Draw hour labels and radial lines
        ctx.font = '10px sans-serif';
        ctx.fillStyle = '#888';
        ctx.textAlign = 'center';
        ctx.textBaseline = 'middle';

        for (var h = 0; h < 24; h++) {
            var angle = (h / 24) * 2 * Math.PI - Math.PI / 2; // Start at 12 o'clock

            // Draw radial line
            ctx.beginPath();
            ctx.moveTo(centerX, centerY);
            var lineEndX = centerX + Math.cos(angle) * radius;
            var lineEndY = centerY + Math.sin(angle) * radius;
            ctx.lineTo(lineEndX, lineEndY);
            ctx.strokeStyle = 'rgba(255, 255, 255, 0.1)';
            ctx.stroke();

            // Draw hour label
            var labelRadius = radius + 15;
            var labelX = centerX + Math.cos(angle) * labelRadius;
            var labelY = centerY + Math.sin(angle) * labelRadius;
            ctx.fillText(h.toString(), labelX, labelY);
        }

        // Draw current hour indicator
        if (data.current_hour !== undefined) {
            var currentAngle = (data.current_hour / 24) * 2 * Math.PI - Math.PI / 2;
            ctx.beginPath();
            ctx.moveTo(centerX, centerY);
            var currentEndX = centerX + Math.cos(currentAngle) * (radius + 5);
            var currentEndY = centerY + Math.sin(currentAngle) * (radius + 5);
            ctx.lineTo(currentEndX, currentEndY);
            ctx.strokeStyle = '#4fc3f7';
            ctx.lineWidth = 2;
            ctx.stroke();
            ctx.lineWidth = 1;
        }

        // Draw data bars for each hour
        if (data.slot_amplitudes && data.slot_confidences) {
            var maxAmplitude = 0;
            for (var i = 0; i < 24; i++) {
                if (data.slot_amplitudes[i] > maxAmplitude) {
                    maxAmplitude = data.slot_amplitudes[i];
                }
            }

            // Avoid division by zero
            if (maxAmplitude === 0) maxAmplitude = 1;

            for (var h = 0; h < 24; h++) {
                var amplitude = data.slot_amplitudes[h] || 0;
                var confidence = data.slot_confidences[h] || 0;

                if (amplitude === 0 && confidence === 0) {
                    continue; // Skip empty slots
                }

                var angle = (h / 24) * 2 * Math.PI - Math.PI / 2;
                var barLength = (amplitude / maxAmplitude) * radius;
                var barWidth = (2 * Math.PI * radius) / 24 * 0.8;

                // Draw bar segment
                ctx.beginPath();
                ctx.arc(centerX, centerY, barLength, angle - barWidth / radius / 2, angle + barWidth / radius / 2);
                ctx.arc(centerX, centerY, 0, angle + barWidth / radius / 2, angle - barWidth / radius / 2, true);
                ctx.closePath();

                var colors = getConfidenceColor(confidence);
                ctx.fillStyle = colors.fill;
                ctx.fill();
                ctx.strokeStyle = colors.stroke;
                ctx.stroke();
            }
        }

        // Draw center info
        ctx.fillStyle = '#fff';
        ctx.font = 'bold 12px sans-serif';
        var infoText = '';
        if (data.is_ready) {
            infoText = 'READY';
        } else if (data.is_learning) {
            infoText = Math.round(data.learning_progress || 0) + '%';
        } else {
            infoText = 'N/A';
        }
        ctx.fillText(infoText, centerX, centerY);
    }

    function showForLink(linkID) {
        currentLinkID = linkID;

        fetch('/api/diurnal/slots/' + encodeURIComponent(linkID))
            .then(function(res) { return res.json(); })
            .then(function(data) {
                render(data);
            })
            .catch(function(err) {
                console.error('[DiurnalChart] Failed to load slot data:', err);
            });
    }

    function clear() {
        if (!ctx || !canvas) return;
        var width = canvas.width / (window.devicePixelRatio || 1);
        var height = canvas.height / (window.devicePixelRatio || 1);
        ctx.clearRect(0, 0, width, height);
        currentLinkID = null;
        slotData = null;
    }

    // Public API
    return {
        init: init,
        render: render,
        showForLink: showForLink,
        clear: clear,
        getCurrentData: function() { return slotData; }
    };
})();
