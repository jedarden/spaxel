/**
 * Jest setup for ambient tests.
 * Mocks Canvas 2D context which is not implemented in jsdom.
 */

// Storage for canvas draw operations (for pixel-based tests)
const canvasDrawData = new Map();

// Helper to reset canvas draw data
global.resetCanvasDrawData = function() {
    canvasDrawData.clear();
};

// Helper to get canvas key
function getCanvasKey(canvas) {
    return canvas.id || canvas.toString();
}

// Mock Canvas 2D context before modules are loaded
HTMLCanvasElement.prototype.getContext = function(contextType) {
    if (contextType === '2d' && !this._mockContext) {
        const canvasKey = getCanvasKey(this);

        // Initialize draw data for this canvas
        if (!canvasDrawData.has(canvasKey)) {
            const width = this.width || 800;
            const height = this.height || 600;
            const data = new Uint8ClampedArray(width * height * 4);
            // Fill with white background
            for (let i = 0; i < data.length; i += 4) {
                data[i] = 255;     // R
                data[i + 1] = 255; // G
                data[i + 2] = 255; // B
                data[i + 3] = 255; // A
            }
            canvasDrawData.set(canvasKey, { data, width, height });
        }

        // Create a mock 2D context that actually tracks draw operations
        const mockContext = {
            canvas: this,
            fillStyle: '#000000',
            strokeStyle: '#000000',
            lineWidth: 1,
            font: '12px sans-serif',
            textAlign: 'left',
            textBaseline: 'alphabetic',

            // Mock methods that track drawing
            clearRect: jest.fn(function(x, y, w, h) {
                const drawData = canvasDrawData.get(canvasKey);
                if (!drawData) return;

                const { data, width } = drawData;
                for (let py = y; py < y + h && py < drawData.height; py++) {
                    for (let px = x; px < x + w && px < width; px++) {
                        const i = (py * width + px) * 4;
                        data[i] = 255;
                        data[i + 1] = 255;
                        data[i + 2] = 255;
                        data[i + 3] = 255;
                    }
                }
            }),

            fillRect: jest.fn(function(x, y, w, h) {
                const drawData = canvasDrawData.get(canvasKey);
                if (!drawData) return;

                const { data, width, height } = drawData;
                // Parse fillStyle
                const color = parseColor(mockContext.fillStyle);

                for (let py = y; py < y + h && py < height; py++) {
                    for (let px = x; px < x + w && px < width; px++) {
                        const i = (py * width + px) * 4;
                        data[i] = color.r;
                        data[i + 1] = color.g;
                        data[i + 2] = color.b;
                        data[i + 3] = 255;
                    }
                }
            }),

            strokeRect: jest.fn(function(x, y, w, h) {
                const drawData = canvasDrawData.get(canvasKey);
                if (!drawData) return;

                const { data, width, height } = drawData;
                const color = parseColor(mockContext.strokeStyle);

                // Draw outline (1px thick)
                const lineWidth = mockContext.lineWidth || 1;
                for (let i = 0; i < lineWidth; i++) {
                    // Top edge
                    for (let px = x; px < x + w && px < width; px++) {
                        setPixel(data, width, px, y + i, color);
                    }
                    // Bottom edge
                    for (let px = x; px < x + w && px < width; px++) {
                        setPixel(data, width, px, y + h - i - 1, color);
                    }
                    // Left edge
                    for (let py = y; py < y + h && py < height; py++) {
                        setPixel(data, width, x + i, py, color);
                    }
                    // Right edge
                    for (let py = y; py < y + h && py < height; py++) {
                        setPixel(data, width, x + w - i - 1, py, color);
                    }
                }
            }),

            fillText: jest.fn(function(text, x, y) {
                const drawData = canvasDrawData.get(canvasKey);
                if (!drawData) return;

                const { data, width, height } = drawData;
                const color = parseColor(mockContext.fillStyle);

                // Draw a simple "text" as colored pixels at the position
                for (let py = y - 6; py < y + 6 && py < height; py++) {
                    for (let px = x - 20; px < x + 20 && px < width; px++) {
                        setPixel(data, width, px, py, color);
                    }
                }
            }),

            beginPath: jest.fn(),
            arc: jest.fn(function(x, y, radius, startAngle, endAngle) {
                const drawData = canvasDrawData.get(canvasKey);
                if (!drawData) return;

                const { data, width, height } = drawData;
                const color = parseColor(mockContext.fillStyle);

                // Draw a filled circle
                for (let py = Math.floor(y - radius); py <= Math.ceil(y + radius) && py < height; py++) {
                    for (let px = Math.floor(x - radius); px <= Math.ceil(x + radius) && px < width; px++) {
                        const dx = px - x;
                        const dy = py - y;
                        if (dx * dx + dy * dy <= radius * radius) {
                            setPixel(data, width, px, py, color);
                        }
                    }
                }
            }),
            moveTo: jest.fn(),
            lineTo: jest.fn(),
            closePath: jest.fn(),
            fill: jest.fn(),
            stroke: jest.fn(),
            scale: jest.fn(),
            roundRect: jest.fn(function(x, y, w, h, radius) {
                // Just draw a filled rectangle for simplicity
                mockContext.fillRect(x, y, w, h);
            }),

            // Mock getImageData to return actual pixel data
            getImageData: function(x, y, width, height) {
                const drawData = canvasDrawData.get(canvasKey);
                if (!drawData) {
                    // Return empty data
                    const data = new Uint8ClampedArray(width * height * 4);
                    return { data, width, height };
                }

                const { data: srcData } = drawData;
                const result = new Uint8ClampedArray(width * height * 4);

                // Copy the requested region
                for (let py = 0; py < height; py++) {
                    for (let px = 0; px < width; px++) {
                        const srcX = x + px;
                        const srcY = y + py;

                        if (srcX >= 0 && srcX < drawData.width && srcY >= 0 && srcY < drawData.height) {
                            const srcIdx = (srcY * drawData.width + srcX) * 4;
                            const dstIdx = (py * width + px) * 4;
                            result[dstIdx] = srcData[srcIdx];
                            result[dstIdx + 1] = srcData[srcIdx + 1];
                            result[dstIdx + 2] = srcData[srcIdx + 2];
                            result[dstIdx + 3] = srcData[srcIdx + 3];
                        }
                    }
                }

                return {
                    data: result,
                    width: width,
                    height: height
                };
            }
        };

        this._mockContext = mockContext;
    }

    return this._mockContext;
};

// Helper to parse color strings
function parseColor(colorStr) {
    if (typeof colorStr !== 'string') {
        return { r: 0, g: 0, b: 0 };
    }

    // Handle hex colors
    if (colorStr.startsWith('#')) {
        let hex = colorStr.slice(1);
        if (hex.length === 3) {
            hex = hex[0] + hex[0] + hex[1] + hex[1] + hex[2] + hex[2];
        }
        const r = parseInt(hex.slice(0, 2), 16);
        const g = parseInt(hex.slice(2, 4), 16);
        const b = parseInt(hex.slice(4, 6), 16);
        return { r, g, b };
    }

    // Handle rgb/hsl colors - simplified
    if (colorStr.startsWith('rgb')) {
        const match = colorStr.match(/\d+/g);
        if (match && match.length >= 3) {
            return { r: parseInt(match[0]), g: parseInt(match[1]), b: parseInt(match[2]) };
        }
    }

    if (colorStr.startsWith('hsl')) {
        // Simplified HSL to RGB - just return a default color
        return { r: 100, g: 100, b: 100 };
    }

    // Default colors
    const namedColors = {
        'white': { r: 255, g: 255, b: 255 },
        'black': { r: 0, g: 0, b: 0 },
        'red': { r: 255, g: 0, b: 0 },
        'green': { r: 0, g: 255, b: 0 },
        'blue': { r: 0, g: 0, b: 255 },
        'grey': { r: 128, g: 128, b: 128 },
        'gray': { r: 128, g: 128, b: 128 }
    };

    const lower = colorStr.toLowerCase();
    if (namedColors[lower]) {
        return namedColors[lower];
    }

    return { r: 0, g: 0, b: 0 };
}

// Helper to set a pixel
function setPixel(data, width, x, y, color) {
    if (x < 0 || y < 0 || x >= width) return;
    const i = (y * width + x) * 4;
    data[i] = color.r;
    data[i + 1] = color.g;
    data[i + 2] = color.b;
    data[i + 3] = 255;
}

// Mock getBoundingClientRect for proper hit testing
HTMLElement.prototype.getBoundingClientRect = function() {
    const rect = {
        x: 0,
        y: 0,
        width: this.offsetWidth || 800,
        height: this.offsetHeight || 600,
        top: 0,
        left: 0,
        bottom: (this.offsetHeight || 600),
        right: (this.offsetWidth || 800),
        toJSON: function() {
            return {
                x: this.x,
                y: this.y,
                width: this.width,
                height: this.height,
                top: this.top,
                left: this.left,
                bottom: this.bottom,
                right: this.right
            };
        }
    };
    return rect;
};

// Mock devicePixelRatio
Object.defineProperty(window, 'devicePixelRatio', {
    value: 1,
    writable: true
});

// Mock requestAnimationFrame with increasing timestamps
let rafTimestamp = 0;
let rafCallbacks = new Map();
let rafIdCounter = 0;
let rafLoopRunning = false;

// Start a mock RAF loop that runs at ~60fps (16ms per frame)
function startRafLoop() {
    if (rafLoopRunning) return;
    rafLoopRunning = true;

    function loop() {
        // Process all pending callbacks
        const callbacksToRun = Array.from(rafCallbacks.entries())
            .filter(([id]) => typeof id === 'number')
            .map(([id, callback]) => callback);

        // Clear processed callbacks
        for (const id of rafCallbacks.keys()) {
            if (typeof id === 'number') {
                rafCallbacks.delete(id);
            }
        }

        // Run all callbacks with current timestamp
        rafTimestamp += 16; // Advance time
        callbacksToRun.forEach(callback => {
            try {
                callback(rafTimestamp);
            } catch (e) {
                // Ignore errors in callbacks
            }
        });

        // Schedule next iteration
        if (rafLoopRunning) {
            const timerId = setTimeout(loop, 0);
            if (global._activeRafTimers) {
                global._activeRafTimers.add(timerId);
            }
        }
    }

    const timerId = setTimeout(loop, 0);
    if (global._activeRafTimers) {
        global._activeRafTimers.add(timerId);
    }
}

// Start the RAF loop immediately
startRafLoop();

global.requestAnimationFrame = function(callback) {
    const id = ++rafIdCounter;
    rafCallbacks.set(id, callback);
    return id;
};

global.cancelAnimationFrame = function(id) {
    rafCallbacks.delete(id);
};

// Helper to stop the RAF loop (for testing)
global.stopRafLoop = function() {
    rafLoopRunning = false;
};

// Helper to restart the RAF loop
global.restartRafLoop = function() {
    rafLoopRunning = false; // Stop first
    rafTimestamp = 0; // Reset timestamp
    startRafLoop(); // Restart
};

// Mock localStorage
var storage = {};
Object.defineProperty(global, 'localStorage', {
    value: {
        getItem: function(key) { return storage[key] || null; },
        setItem: function(key, val) { storage[key] = String(val); },
        removeItem: function(key) { delete storage[key]; },
        clear: function() { storage = {}; },
        get length() { return Object.keys(storage).length; },
        key: function(index) { return Object.keys(storage)[index] || null; }
    },
    writable: true,
    configurable: true
});

// Make storage accessible for test cleanup
global._localStorage = storage;

// Track active requestAnimationFrame timers for cleanup
global._activeRafTimers = new Set();
global._stopAllRafTimers = function() {
    _activeRafTimers.forEach(timerId => clearTimeout(timerId));
    _activeRafTimers.clear();
};

// Mock addEventListener and removeEventListener on EventTarget prototype
// to ensure they work properly in tests
const originalAddEventListener = EventTarget.prototype.addEventListener;
const originalRemoveEventListener = EventTarget.prototype.removeEventListener;
