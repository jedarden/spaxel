/**
 * Profiling wrapper for onboard.test.js
 *
 * Captures evidence of resource leaks across the test suite:
 * - Active timer/interval handles
 * - Heap snapshots
 * - WebSocket instance counts
 * - Event listener counts
 * - DOM node counts
 */

const fs = require('fs');
const path = require('path');

// Resource tracking utilities
class ResourceTracker {
    constructor() {
        this.snapshots = [];
    }

    captureSnapshot(label) {
        const snapshot = {
            label,
            timestamp: Date.now(),
            timers: this._countTimers(),
            intervals: this._countIntervals(),
            websockets: this._countWebSockets(),
            eventListeners: this._countEventListeners(),
            domNodes: this._countDOMNodes(),
            heapUsage: process.memoryUsage(),
            heapSnapshot: this._captureHeapSnapshot(),
        };
        this.snapshots.push(snapshot);
        return snapshot;
    }

    _countTimers() {
        // Count pending setTimeout handles
        let count = 0;
        if (global._timeoutHandlers) {
            count = Object.keys(global._timeoutHandlers).length;
        }
        return count;
    }

    _countIntervals() {
        // Count pending setInterval handles
        let count = 0;
        if (global._intervalHandlers) {
            count = Object.keys(global._intervalHandlers).length;
        }
        return count;
    }

    _countWebSockets() {
        // Count active WebSocket instances
        let count = 0;
        if (global._websocketInstances) {
            count = global._websocketInstances.length;
        }
        return count;
    }

    _countEventListeners() {
        // Estimate event listener count by tracking registered events
        let count = 0;
        if (typeof document !== 'undefined' && document.addEventListener) {
            // This is a rough estimate - actual tracking requires wrapping addEventListener
            count = this._estimateDocumentListeners();
        }
        return count;
    }

    _estimateDocumentListeners() {
        // Quick heuristic: count event handlers on known wizard elements
        let count = 0;
        const elements = [
            'wizard-overlay',
            'wizard-card',
            'wizard-next',
            'wizard-back',
            'wizard-close',
            'wizard-content',
        ];
        elements.forEach(id => {
            const el = document.getElementById(id);
            if (el) {
                // Each element might have click, submit, change listeners
                count += 3; // heuristic estimate
            }
        });
        return count;
    }

    _countDOMNodes() {
        if (typeof document !== 'undefined') {
            return document.getElementsByTagName('*').length;
        }
        return 0;
    }

    _captureHeapSnapshot() {
        // In a browser environment, this would use performance.memory
        // In Node.js with jsdom, we use process.memoryUsage()
        const mem = process.memoryUsage();
        return {
            heapUsed: mem.heapUsed,
            heapTotal: mem.heapTotal,
            external: mem.external,
            arrayBuffers: mem.arrayBuffers,
        };
    }

    compareSnapshots(before, after) {
        return {
            timers: after.timers - before.timers,
            intervals: after.intervals - before.intervals,
            websockets: after.websockets - before.websockets,
            eventListeners: after.eventListeners - before.eventListeners,
            domNodes: after.domNodes - before.domNodes,
            heapGrowth: after.heapUsage.heapUsed - before.heapUsage.heapUsed,
        };
    }

    generateReport() {
        const report = {
            timestamp: new Date().toISOString(),
            summary: {
                totalSnapshots: this.snapshots.length,
                totalLeakedTimers: 0,
                totalLeakedIntervals: 0,
                totalLeakedWebSockets: 0,
                totalHeapGrowth: 0,
            },
            snapshots: this.snapshots,
            deltas: [],
        };

        // Calculate deltas between consecutive snapshots
        for (let i = 1; i < this.snapshots.length; i++) {
            const delta = this.compareSnapshots(this.snapshots[i - 1], this.snapshots[i]);
            delta.fromLabel = this.snapshots[i - 1].label;
            delta.toLabel = this.snapshots[i].label;
            report.deltas.push(delta);

            // Track leaked resources
            if (delta.timers > 0) report.summary.totalLeakedTimers += delta.timers;
            if (delta.intervals > 0) report.summary.totalLeakedIntervals += delta.intervals;
            if (delta.websockets > 0) report.summary.totalLeakedWebSockets += delta.websockets;
            if (delta.heapGrowth > 0) report.summary.totalHeapGrowth += delta.heapGrowth;
        }

        return report;
    }
}

// Monkey-patching to track resource creation
class Instrumentation {
    constructor(tracker) {
        this.tracker = tracker;
        this.originalSetTimeout = global.setTimeout;
        this.originalClearTimeout = global.clearTimeout;
        this.originalSetInterval = global.setInterval;
        this.originalClearInterval = global.clearInterval;
        this.originalWebSocket = global.WebSocket;
        this.timeoutHandlers = {};
        this.intervalHandlers = {};
        this.websocketInstances = [];
    }

    install() {
        const self = this;

        // Track setTimeout
        global.setTimeout = function(callback, delay, ...args) {
            const id = self.originalSetTimeout(callback, delay, ...args);
            self.timeoutHandlers[id] = { callback, delay, args, created: Date.now() };
            global._timeoutHandlers = self.timeoutHandlers;
            return id;
        };

        global.clearTimeout = function(id) {
            delete self.timeoutHandlers[id];
            return self.originalClearTimeout(id);
        };

        // Track setInterval
        global.setInterval = function(callback, interval, ...args) {
            const id = self.originalSetInterval(callback, interval, ...args);
            self.intervalHandlers[id] = { callback, interval, args, created: Date.now() };
            global._intervalHandlers = self.intervalHandlers;
            return id;
        };

        global.clearInterval = function(id) {
            delete self.intervalHandlers[id];
            return self.originalClearInterval(id);
        };

        // Track WebSocket instances
        if (global.WebSocket) {
            global.WebSocket = function(...args) {
                const ws = new self.originalWebSocket(...args);
                self.websocketInstances.push({
                    ws,
                    url: args[0],
                    protocols: args[1],
                    created: Date.now(),
                    closed: false,
                });
                global._websocketInstances = self.websocketInstances;

                // Track close calls
                const originalClose = ws.close;
                ws.close = function(...closeArgs) {
                    const instance = self.websocketInstances.find(i => i.ws === ws);
                    if (instance) instance.closed = true;
                    return originalClose.apply(this, closeArgs);
                };

                return ws;
            };
        }
    }

    uninstall() {
        global.setTimeout = this.originalSetTimeout;
        global.clearTimeout = this.originalClearTimeout;
        global.setInterval = this.originalSetInterval;
        global.clearInterval = this.originalClearTimeout;
        if (this.originalWebSocket) {
            global.WebSocket = this.originalWebSocket;
        }
    }

    getActiveResources() {
        return {
            pendingTimeouts: Object.keys(this.timeoutHandlers).length,
            pendingIntervals: Object.keys(this.intervalHandlers).length,
            activeWebSockets: this.websocketInstances.filter(ws => !ws.closed).length,
        };
    }
}

// Main profiling runner
class ProfilingTestRunner {
    constructor() {
        this.tracker = new ResourceTracker();
        this.instrumentation = new Instrumentation(this.tracker);
        this.report = null;
    }

    async run(testModule) {
        console.log('🔍 Starting profiling test run...');

        // Install instrumentation
        this.instrumentation.install();

        // Capture initial state
        this.tracker.captureSnapshot('BEFORE_ALL');

        try {
            // Run all tests
            if (testModule && testModule.run) {
                await testModule.run();
            } else {
                throw new Error('Test module does not have a run() method');
            }
        } finally {
            // Capture final state
            this.tracker.captureSnapshot('AFTER_ALL');

            // Generate report
            this.report = this.tracker.generateReport();

            // Uninstall instrumentation
            this.instrumentation.uninstall();

            // Output report
            this.outputReport();
        }

        console.log('✅ Profiling test run complete');
        return this.report;
    }

    outputReport() {
        const outputPath = path.join(__dirname, 'onboard.test.profiling-output.json');

        // Write detailed JSON report
        fs.writeFileSync(outputPath, JSON.stringify(this.report, null, 2));
        console.log(`📊 Detailed report written to: ${outputPath}`);

        // Print summary to console
        console.log('\n📈 PROFILING SUMMARY:');
        console.log('==================');
        console.log(`Total test snapshots: ${this.report.summary.totalSnapshots}`);
        console.log(`Leaked timers: ${this.report.summary.totalLeakedTimers}`);
        console.log(`Leaked intervals: ${this.report.summary.totalLeakedIntervals}`);
        console.log(`Leaked WebSockets: ${this.report.summary.totalLeakedWebSockets}`);
        console.log(`Heap growth: ${this.formatBytes(this.report.summary.totalHeapGrowth)}`);

        // Show concerning deltas
        const concerningDeltas = this.report.deltas.filter(d =>
            d.timers > 0 || d.intervals > 0 || d.websockets > 0 || d.heapGrowth > 1024 * 1024
        );

        if (concerningDeltas.length > 0) {
            console.log('\n⚠️  CONCERNING RESOURCE LEAKS:');
            console.log('=====================================');
            concerningDeltas.forEach(delta => {
                console.log(`\n"${delta.fromLabel}" → "${delta.toLabel}":`);
                if (delta.timers > 0) console.log(`  ⏱️  +${delta.timers} timers`);
                if (delta.intervals > 0) console.log(`  🔄 +${delta.intervals} intervals`);
                if (delta.websockets > 0) console.log(`  🔌 +${delta.websockets} WebSockets`);
                if (delta.heapGrowth > 0) console.log(`  💾 +${this.formatBytes(delta.heapGrowth)} heap`);
            });
        } else {
            console.log('\n✅ No concerning resource leaks detected!');
        }
    }

    formatBytes(bytes) {
        if (bytes === 0) return '0 B';
        const k = 1024;
        const sizes = ['B', 'KB', 'MB', 'GB'];
        const i = Math.floor(Math.log(Math.abs(bytes)) / Math.log(k));
        return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
    }
}

// Export for use
module.exports = { ResourceTracker, Instrumentation, ProfilingTestRunner };
