/**
 * Test Profiler for Leak Detection
 *
 * Captures heap, timer, and WebSocket state before/after test runs
 * to identify resource leaks in the test suite.
 */

const fs = require('fs');
const path = require('path');

// Track timer and WebSocket instances across the test run
const timerTracker = new Map();
const wsTracker = new Map();
let snapshotCount = 0;

/**
 * Get all active timers/intervals in the process
 * Note: This is a best-effort approximation in Node.js/Jest environment
 */
function getActiveTimers() {
    // Node.js doesn't expose a direct API to list active timers
    // We track what we can through monkey-patching
    const active = {
        intervals: timerTracker.get('intervals')?.size || 0,
        timeouts: timerTracker.get('timeouts')?.size || 0,
        total: (timerTracker.get('intervals')?.size || 0) + (timerTracker.get('timeouts')?.size || 0)
    };
    return active;
}

/**
 * Get WebSocket instance count from our tracker
 */
function getWebSocketCount() {
    return wsTracker.get('instances')?.size || 0;
}

/**
 * Get heap usage information
 */
function getHeapInfo() {
    const mem = process.memoryUsage();
    return {
        heapUsed: Math.round(mem.heapUsed / 1024 / 1024), // MB
        heapTotal: Math.round(mem.heapTotal / 1024 / 1024), // MB
        external: Math.round(mem.external / 1024 / 1024), // MB
        rss: Math.round(mem.rss / 1024 / 1024), // MB
        heapUsedBytes: mem.heapUsed,
        heapTotalBytes: mem.heapTotal
    };
}

/**
 * Capture a comprehensive state snapshot
 */
function captureSnapshot(label) {
    snapshotCount++;
    const snapshot = {
        id: snapshotCount,
        timestamp: new Date().toISOString(),
        label: label,
        heap: getHeapInfo(),
        timers: getActiveTimers(),
        websockets: {
            count: getWebSocketCount(),
            instances: Array.from(wsTracker.get('instances')?.keys() || [])
        },
        process: {
            pid: process.pid,
            uptime: Math.round(process.uptime()),
            memoryUsage: process.memoryUsage()
        }
    };
    return snapshot;
}

/**
 * Monkey-patch global setTimeout/clearTimeout/setInterval/clearInterval
 * to track timer creation and cleanup
 */
function instrumentTimers() {
    const originalSetTimeout = global.setTimeout;
    const originalClearTimeout = global.clearTimeout;
    const originalSetInterval = global.setInterval;
    const originalClearInterval = global.clearInterval;

    timerTracker.set('timeouts', new Map());
    timerTracker.set('intervals', new Map());

    global.setTimeout = function(...args) {
        const timeoutId = originalSetTimeout.apply(this, args);
        timerTracker.get('timeouts').set(timeoutId, {
            created: Date.now(),
            stack: new Error().stack
        });
        return timeoutId;
    };

    global.clearTimeout = function(timeoutId) {
        timerTracker.get('timeouts').delete(timeoutId);
        return originalClearTimeout.apply(this, arguments);
    };

    global.setInterval = function(...args) {
        const intervalId = originalSetInterval.apply(this, args);
        timerTracker.get('intervals').set(intervalId, {
            created: Date.now(),
            stack: new Error().stack
        });
        return intervalId;
    };

    global.clearInterval = function(intervalId) {
        timerTracker.get('intervals').delete(intervalId);
        return originalClearInterval.apply(this, arguments);
    };
}

/**
 * Monkey-patch WebSocket to track instances
 */
function instrumentWebSockets() {
    // WebSocket is mocked in tests, but we track the mock
    wsTracker.set('instances', new Map());

    // In tests, WebSocket is a Jest mock
    if (typeof WebSocket !== 'undefined' && WebSocket._isMock) {
        const OriginalWebSocket = WebSocket;
        let wsId = 0;

        global.WebSocket = function(...args) {
            const ws = new OriginalWebSocket(...args);
            const id = ++wsId;
            wsTracker.get('instances').set(id, {
                instance: ws,
                created: Date.now(),
                url: args[0]
            });
            ws._id = id;
            return ws;
        };
    }
}

/**
 * Write profiling results to file
 */
function writeProfilingData(before, after, filePath) {
    const report = {
        meta: {
            testFile: 'onboard.test.js',
            timestamp: new Date().toISOString(),
            duration: after.timestamp - new Date(before.timestamp).toISOString()
        },
        snapshots: {
            beforeTest: before,
            afterTest: after
        },
        deltas: {
            heap: {
                usedDelta: after.heap.heapUsedBytes - before.heap.heapUsedBytes,
                usedDeltaMB: Math.round((after.heap.heapUsedBytes - before.heap.heapUsedBytes) / 1024 / 1024)
            },
            timers: {
                timeoutDelta: after.timers.timeouts - before.timers.timeouts,
                intervalDelta: after.timers.intervals - before.timers.intervals,
                totalDelta: after.timers.total - before.timers.total
            },
            websockets: {
                countDelta: after.websockets.count - before.websockets.count
            }
        },
        analysis: analyzeLeaks(before, after)
    };

    // Append to file or create new
    const data = JSON.stringify(report, null, 2);
    const dir = path.dirname(filePath);
    if (!fs.existsSync(dir)) {
        fs.mkdirSync(dir, { recursive: true });
    }

    if (fs.existsSync(filePath)) {
        fs.appendFileSync(filePath, ',\n' + data);
    } else {
        fs.writeFileSync(filePath, '[' + data);
    }
}

/**
 * Analyze snapshots for potential leaks
 */
function analyzeLeaks(before, after) {
    const issues = [];
    const heapDelta = after.heap.heapUsedBytes - before.heap.heapUsedBytes;
    const heapDeltaMB = heapDelta / 1024 / 1024;

    // Check for heap growth > 5MB
    if (heapDeltaMB > 5) {
        issues.push({
            severity: 'high',
            type: 'heap-growth',
            message: `Heap grew by ${heapDeltaMB.toFixed(2)} MB during test run`,
            before: before.heap.heapUsed,
            after: after.heap.heapUsed
        });
    }

    // Check for uncleared timers
    if (after.timers.total > before.timers.total) {
        issues.push({
            severity: 'medium',
            type: 'timer-leak',
            message: `${after.timers.total - before.timers.total} timers created but not cleared`,
            before: before.timers,
            after: after.timers
        });
    }

    // Check for uncleared WebSockets
    if (after.websockets.count > before.websockets.count) {
        issues.push({
            severity: 'medium',
            type: 'websocket-leak',
            message: `${after.websockets.count - before.websockets.count} WebSocket(s) not closed`,
            before: before.websockets,
            after: after.websockets
        });
    }

    // Check if intervals remain (common leak source)
    if (after.timers.intervals > 0) {
        issues.push({
            severity: 'medium',
            type: 'interval-leak',
            message: `${after.timers.intervals} interval(s) still active after test completion`,
            intervals: after.timers.intervals
        });
    }

    return {
        issuesFound: issues.length > 0,
        issues: issues,
        summary: issues.length === 0 ? 'No leaks detected' : `${issues.length} potential leak(s) found`
    };
}

/**
 * Force garbage collection (if available)
 */
function forceGC() {
    if (global.gc) {
        global.gc();
    }
}

module.exports = {
    captureSnapshot,
    writeProfilingData,
    analyzeLeaks,
    instrumentTimers,
    instrumentWebSockets,
    forceGC,
    getActiveTimers,
    getWebSocketCount,
    getHeapInfo
};
