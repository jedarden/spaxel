/**
 * Spaxel Floor Plan Setup Module
 *
 * Handles floor plan image upload, calibration UI, and applying
 * pixel-to-meter scale and rotation to the 3D ground plane texture.
 */

(function() {
    'use strict';

    // Module state
    const state = {
        panelVisible: false,
        calibration: null,      // { ax, ay, bx, by, distance_m, rotation_deg, meters_per_pixel }
        imageLoaded: false,
        calibrating: false,
        pointA: null,           // { x, y } in image pixels
        pointB: null,           // { x, y } in image pixels
        imageURL: null
    };

    // DOM elements cache
    let elements = {};

    /**
     * Initialize the floor plan setup module.
     */
    function init() {
        console.log('[FloorPlan] Initializing');
        createPanel();
        loadExistingFloorplan();
    }

    /**
     * Create the floor plan setup panel DOM.
     */
    function createPanel() {
        // Check if panel already exists
        if (document.getElementById('floorplan-panel')) {
            cacheElements();
            return;
        }

        const panel = document.createElement('div');
        panel.id = 'floorplan-panel';
        panel.className = 'floorplan-panel';
        panel.style.display = 'none';
        panel.innerHTML = `
            <div class="floorplan-header">
                <h3>Floor Plan Setup</h3>
                <button class="floorplan-close" onclick="FloorPlanSetup.togglePanel()">&times;</button>
            </div>

            <div class="floorplan-content">
                <!-- Image Upload Section -->
                <div class="floorplan-section">
                    <h4>1. Upload Floor Plan</h4>
                    <p class="floorplan-hint">Upload an image of your floor plan (PNG or JPG, max 10 MB)</p>
                    <div class="floorplan-upload-area" id="floorplan-upload-area">
                        <input type="file" id="floorplan-file-input" accept="image/png,image/jpeg" style="display:none">
                        <button class="floorplan-btn" onclick="document.getElementById('floorplan-file-input').click()">
                            <span class="floorplan-icon">📁</span>
                            Choose Image
                        </button>
                        <span class="floorplan-file-name" id="floorplan-file-name">No file chosen</span>
                    </div>
                    <div class="floorplan-preview" id="floorplan-preview" style="display:none">
                        <img id="floorplan-preview-img" alt="Floor plan preview">
                    </div>
                </div>

                <!-- Calibration Section -->
                <div class="floorplan-section" id="calibration-section" style="display:none">
                    <h4>2. Calibrate Scale</h4>
                    <p class="floorplan-hint">Click two points on the image and enter the real-world distance between them</p>

                    <div class="floorplan-calibration-container">
                        <div class="floorplan-image-wrapper" id="floorplan-image-wrapper">
                            <canvas id="floorplan-canvas"></canvas>
                            <div class="floorplan-marker" id="marker-a" style="display:none">A</div>
                            <div class="floorplan-marker" id="marker-b" style="display:none">B</div>
                        </div>

                        <div class="floorplan-controls">
                            <div class="floorplan-instructions" id="floorplan-instructions">
                                Click on point <strong>A</strong> in the image
                            </div>

                            <div class="floorplan-points-info" id="floorplan-points-info" style="display:none">
                                <div class="floorplan-point-info">
                                    <span class="point-label">Point A:</span>
                                    <span id="point-a-coords">--</span>
                                </div>
                                <div class="floorplan-point-info">
                                    <span class="point-label">Point B:</span>
                                    <span id="point-b-coords">--</span>
                                </div>
                                <div class="floorplan-point-info">
                                    <span class="point-label">Pixel distance:</span>
                                    <span id="pixel-distance">--</span> px
                                </div>
                            </div>

                            <div class="floorplan-distance-input" id="floorplan-distance-input" style="display:none">
                                <label for="real-distance">Real-world distance (meters):</label>
                                <input type="number" id="real-distance" min="0.1" max="1000" step="0.01" placeholder="e.g., 5.0">
                            </div>

                            <div class="floorplan-actions">
                                <button class="floorplan-btn floorplan-btn-secondary" id="btn-reset" onclick="FloorPlanSetup.resetCalibration()" style="display:none">
                                    Reset
                                </button>
                                <button class="floorplan-btn floorplan-btn-primary" id="btn-save" onclick="FloorPlanSetup.saveCalibration()" style="display:none" disabled>
                                    Save Calibration
                                </button>
                            </div>
                        </div>
                    </div>
                </div>

                <!-- Calibration Status -->
                <div class="floorplan-section" id="calibration-status-section" style="display:none">
                    <h4>Calibration Status</h4>
                    <div class="floorplan-status-info" id="floorplan-status-info">
                        <div class="status-item">
                            <span class="status-label">Scale:</span>
                            <span id="status-scale">--</span>
                        </div>
                        <div class="status-item">
                            <span class="status-label">Rotation:</span>
                            <span id="status-rotation">--</span>
                        </div>
                    </div>
                    <button class="floorplan-btn floorplan-btn-secondary" onclick="FloorPlanSetup.resetCalibration()">
                        Recalibrate
                    </button>
                </div>
            </div>
        `;

        document.body.appendChild(panel);
        cacheElements();
        attachEventListeners();
    }

    /**
     * Cache DOM elements for faster access.
     */
    function cacheElements() {
        elements = {
            panel: document.getElementById('floorplan-panel'),
            fileInput: document.getElementById('floorplan-file-input'),
            fileName: document.getElementById('floorplan-file-name'),
            preview: document.getElementById('floorplan-preview'),
            previewImg: document.getElementById('floorplan-preview-img'),
            calibrationSection: document.getElementById('calibration-section'),
            calibrationStatusSection: document.getElementById('calibration-status-section'),
            imageWrapper: document.getElementById('floorplan-image-wrapper'),
            canvas: document.getElementById('floorplan-canvas'),
            markerA: document.getElementById('marker-a'),
            markerB: document.getElementById('marker-b'),
            instructions: document.getElementById('floorplan-instructions'),
            pointsInfo: document.getElementById('floorplan-points-info'),
            distanceInput: document.getElementById('floorplan-distance-input'),
            realDistanceInput: document.getElementById('real-distance'),
            pointACoords: document.getElementById('point-a-coords'),
            pointBCoords: document.getElementById('point-b-coords'),
            pixelDistance: document.getElementById('pixel-distance'),
            btnReset: document.getElementById('btn-reset'),
            btnSave: document.getElementById('btn-save'),
            statusScale: document.getElementById('status-scale'),
            statusRotation: document.getElementById('status-rotation')
        };
    }

    /**
     * Attach event listeners.
     */
    function attachEventListeners() {
        elements.fileInput.addEventListener('change', handleFileSelect);
        elements.canvas.addEventListener('click', handleCanvasClick);
        elements.realDistanceInput.addEventListener('input', handleDistanceInput);
    }

    /**
     * Toggle panel visibility.
     */
    function togglePanel() {
        state.panelVisible = !state.panelVisible;
        elements.panel.style.display = state.panelVisible ? 'block' : 'none';
        if (state.panelVisible && state.imageLoaded) {
            drawCanvas();
        }
    }

    /**
     * Load existing floor plan data from server.
     */
    function loadExistingFloorplan() {
        fetch('/api/floorplan')
            .then(res => res.json())
            .then(data => {
                if (data.image_url) {
                    state.imageURL = data.image_url;
                    state.imageLoaded = true;
                    elements.previewImg.src = data.image_url;
                    elements.preview.style.display = 'block';
                    elements.calibrationSection.style.display = 'block';

                    // Load image for canvas
                    const img = new Image();
                    img.crossOrigin = 'anonymous';
                    img.onload = function() {
                        state.imageElement = img;
                        if (state.panelVisible) drawCanvas();
                    };
                    img.src = data.image_url;
                }

                if (data.calibration) {
                    state.calibration = data.calibration;
                    updateCalibrationStatus();
                    elements.calibrationStatusSection.style.display = 'block';
                    elements.calibrationSection.style.display = 'none';

                    // Apply calibration to Viz3D
                    applyCalibrationTo3D();
                }
            })
            .catch(err => {
                console.error('[FloorPlan] Failed to load floor plan:', err);
            });
    }

    /**
     * Handle file selection.
     */
    function handleFileSelect(e) {
        const file = e.target.files[0];
        if (!file) return;

        elements.fileName.textContent = file.name;

        // Upload to server
        const formData = new FormData();
        formData.append('file', file);

        fetch('/api/floorplan/image', {
            method: 'POST',
            body: formData
        })
        .then(res => res.json())
        .then(data => {
            if (data.ok) {
                state.imageURL = data.image_url;
                state.imageLoaded = true;
                elements.previewImg.src = data.image_url;
                elements.preview.style.display = 'block';
                elements.calibrationSection.style.display = 'block';

                // Load image for canvas
                const img = new Image();
                img.onload = function() {
                    state.imageElement = img;
                    drawCanvas();
                };
                img.src = data.image_url;

                // Also update Viz3D texture
                if (window.Viz3D && window.Viz3D.uploadFloorPlan) {
                    window.Viz3D.uploadFloorPlan(file);
                }
            }
        })
        .catch(err => {
            console.error('[FloorPlan] Upload failed:', err);
            elements.fileName.textContent = 'Upload failed';
        });
    }

    /**
     * Draw the floor plan image on canvas.
     */
    function drawCanvas() {
        if (!state.imageElement || !elements.canvas) return;

        const img = state.imageElement;
        const canvas = elements.canvas;
        const ctx = canvas.getContext('2d');

        // Calculate dimensions to fit the wrapper
        const wrapper = elements.imageWrapper;
        const maxWidth = wrapper.clientWidth - 20;
        const maxHeight = 400;

        const scale = Math.min(maxWidth / img.width, maxHeight / img.height);
        canvas.width = img.width * scale;
        canvas.height = img.height * scale;

        state.canvasScale = scale;

        ctx.drawImage(img, 0, 0, canvas.width, canvas.height);

        // Draw existing calibration points if available
        if (state.pointA) drawMarker(state.pointA, 'A');
        if (state.pointB) drawMarker(state.pointB, 'B');

        // Draw line if both points exist
        if (state.pointA && state.pointB) {
            ctx.strokeStyle = 'rgba(79, 195, 247, 0.7)';
            ctx.lineWidth = 2;
            ctx.setLineDash([5, 5]);
            ctx.beginPath();
            ctx.moveTo(state.pointA.x, state.pointA.y);
            ctx.lineTo(state.pointB.x, state.pointB.y);
            ctx.stroke();
            ctx.setLineDash([]);
        }
    }

    /**
     * Draw a calibration marker on canvas.
     */
    function drawMarker(point, label) {
        const ctx = elements.canvas.getContext('2d');
        ctx.fillStyle = label === 'A' ? '#4fc3f7' : '#66bb6a';
        ctx.beginPath();
        ctx.arc(point.x, point.y, 8, 0, Math.PI * 2);
        ctx.fill();

        ctx.fillStyle = '#fff';
        ctx.font = 'bold 12px sans-serif';
        ctx.textAlign = 'center';
        ctx.textBaseline = 'middle';
        ctx.fillText(label, point.x, point.y);
    }

    /**
     * Handle canvas click for calibration point selection.
     */
    function handleCanvasClick(e) {
        if (!state.imageLoaded) return;

        const rect = elements.canvas.getBoundingClientRect();
        const x = e.clientX - rect.left;
        const y = e.clientY - rect.top;

        if (!state.pointA) {
            state.pointA = { x, y };
            elements.instructions.innerHTML = 'Click on point <strong>B</strong> in the image';
            elements.pointACoords.textContent = `${Math.round(x)}, ${Math.round(y)}`;
            elements.pointsInfo.style.display = 'block';
            elements.btnReset.style.display = 'inline-block';
        } else if (!state.pointB) {
            state.pointB = { x, y };
            elements.pointBCoords.textContent = `${Math.round(x)}, ${Math.round(y)}`;

            const pixelDist = calculatePixelDistance();
            elements.pixelDistance.textContent = pixelDist.toFixed(1);
            elements.distanceInput.style.display = 'block';
            elements.realDistanceInput.focus();

            // Update instructions
            elements.instructions.innerHTML = 'Enter the real-world distance and save';
        }

        drawCanvas();
    }

    /**
     * Calculate pixel distance between point A and B.
     */
    function calculatePixelDistance() {
        if (!state.pointA || !state.pointB) return 0;
        const dx = state.pointB.x - state.pointA.x;
        const dy = state.pointB.y - state.pointA.y;
        return Math.sqrt(dx * dx + dy * dy);
    }

    /**
     * Handle distance input change.
     */
    function handleDistanceInput(e) {
        const value = parseFloat(e.target.value);
        elements.btnSave.disabled = !value || value <= 0;
    }

    /**
     * Reset calibration state.
     */
    function resetCalibration() {
        state.pointA = null;
        state.pointB = null;
        state.calibrating = false;

        elements.instructions.innerHTML = 'Click on point <strong>A</strong> in the image';
        elements.pointsInfo.style.display = 'none';
        elements.distanceInput.style.display = 'none';
        elements.btnReset.style.display = 'none';
        elements.btnSave.style.display = 'none';
        elements.realDistanceInput.value = '';
        elements.calibrationStatusSection.style.display = 'none';
        elements.calibrationSection.style.display = 'block';

        drawCanvas();
    }

    /**
     * Save calibration to server.
     */
    function saveCalibration() {
        const distanceM = parseFloat(elements.realDistanceInput.value);
        if (!distanceM || distanceM <= 0) return;

        const pixelDist = calculatePixelDistance();
        const metersPerPixel = distanceM / pixelDist;

        // Calculate rotation angle from point A to B
        const dx = state.pointB.x - state.pointA.x;
        const dy = state.pointB.y - state.pointA.y;
        const rotationRad = Math.atan2(dy, dx);
        const rotationDeg = rotationRad * 180 / Math.PI;

        const calibrationData = {
            ax: state.pointA.x / state.canvasScale,
            ay: state.pointA.y / state.canvasScale,
            bx: state.pointB.x / state.canvasScale,
            by: state.pointB.y / state.canvasScale,
            distance_m: distanceM,
            rotation_deg: rotationDeg
        };

        fetch('/api/floorplan/calibrate', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(calibrationData)
        })
        .then(res => res.json())
        .then(data => {
            if (data.ok) {
                state.calibration = {
                    ...calibrationData,
                    meters_per_pixel: data.meters_per_pixel
                };

                updateCalibrationStatus();
                elements.calibrationStatusSection.style.display = 'block';
                elements.calibrationSection.style.display = 'none';

                // Apply calibration to 3D
                applyCalibrationTo3D();
            }
        })
        .catch(err => {
            console.error('[FloorPlan] Calibration save failed:', err);
        });
    }

    /**
     * Update calibration status display.
     */
    function updateCalibrationStatus() {
        if (!state.calibration) return;

        const mpp = state.calibration.meters_per_pixel;
        const scaleText = mpp ? `${(mpp * 100).toFixed(3)} cm/pixel` : '--';
        const rotationText = state.calibration.rotation_deg ?
            `${state.calibration.rotation_deg.toFixed(1)}°` : '--';

        elements.statusScale.textContent = scaleText;
        elements.statusRotation.textContent = rotationText;
    }

    /**
     * Apply calibration to the 3D floor texture in Viz3D.
     */
    function applyCalibrationTo3D() {
        if (!window.Viz3D || !state.calibration) return;

        // Store calibration for Viz3D to use
        if (window.Viz3D.setFloorPlanCalibration) {
            window.Viz3D.setFloorPlanCalibration(state.calibration);
        }
    }

    /**
     * Get current calibration data.
     */
    function getCalibration() {
        return state.calibration;
    }

    // Public API
    window.FloorPlanSetup = {
        init,
        togglePanel,
        resetCalibration,
        saveCalibration,
        getCalibration
    };

    // Auto-initialize
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }
})();
