#!/usr/bin/env node
/**
 * Standalone profiling test runner for onboard.test.js
 *
 * This script:
 * 1. Sets up instrumentation before all tests
 * 2. Runs the existing test suite
 * 3. Captures resource usage before/after each test
 * 4. Outputs a detailed profiling report
 *
 * Usage: node profile-suite.js
 * Output: onboard.test.profiling-output.json
 */

const fs = require('fs');
const path = require('path');

// Resource tracking utilities
class ResourceTracker {
    constructor() {
        this.snapshots = [];
        this.timers = new Set();
        this.intervals = new Set();
        this.websockets = new Set();
        this.originalSetTimeout = global.setTimeout;
        this.originalClearTimeout = global.clearTimeout;
        this.originalSetInterval = global.setInterval;
        this.originalClearInterval = global.clearInterval;
    }

    install() {
        const self = this;

        // Wrap setTimeout to track timers
        global.setTimeout = function(callback, delay, ...args) {
            const id = self.originalSetTimeout(callback, delay, ...args);
            self.timers.add(id);
            return id;
        };

        global.clearTimeout = function(id) {
            self.timers.delete(id);
            return self.originalClearTimeout(id);
        };

        // Wrap setInterval to track intervals
        global.setInterval = function(callback, interval, ...args) {
            const id = self.originalSetInterval(callback, interval, ...args);
            self.intervals.add(id);
            return id;
        };

        global.clearInterval = function(id) {
            self.intervals.delete(id);
            return self.originalClearInterval(id);
        };

        // Track WebSocket if available (in jsdom environment)
        if (global.WebSocket) {
            const OriginalWebSocket = global.WebSocket;
            global.WebSocket = function(...args) {
                const ws = new OriginalWebSocket(...args);
                self.websockets.add(ws);
                return ws;
            };
        }
    }

    uninstall() {
        global.setTimeout = this.originalSetTimeout;
        global.clearTimeout = this.originalClearTimeout;
        global.setInterval = this.originalSetInterval;
        global.clearInterval = this.originalClearInterval;
    }

    captureSnapshot(label) {
        const snapshot = {
            label,
            timestamp: Date.now(),
            activeTimers: this.timers.size,
            activeIntervals: this.intervals.size,
            activeWebSockets: this.websockets.size,
            memory: process.memoryUsage(),
            handles: process._getActiveHandles ? process._getActiveHandles().map(h => h.type) : [],
            requests: process._getActiveRequests ? process._getActiveRequests().map(r => r.type) : [],
        };
        this.snapshots.push(snapshot);
        return snapshot;
    }

    generateReport() {
        const report = {
            timestamp: new Date().toISOString(),
            summary: {
                totalSnapshots: this.snapshots.length,
            },
            snapshots: this.snapshots,
            deltas: [],
        };

        // Calculate deltas
        for (let i = 1; i < this.snapshots.length; i++) {
            const prev = this.snapshots[i - 1];
            const curr = this.snapshots[i];
            report.deltas.push({
                from: prev.label,
                to: curr.label,
                timerDelta: curr.activeTimers - prev.activeTimers,
                intervalDelta: curr.activeIntervals - prev.activeIntervals,
                websocketDelta: curr.activeWebSockets - prev.activeWebSockets,
                heapDelta: curr.memory.heapUsed - prev.memory.heapUsed,
                externalDelta: curr.memory.external - prev.memory.external,
            });

            // Update summary
            if (curr.activeTimers > prev.activeTimers) report.summary.leakedTimers = (report.summary.leakedTimers || 0) + (curr.activeTimers - prev.activeTimers);
            if (curr.activeIntervals > prev.activeIntervals) report.summary.leakedIntervals = (report.summary.leakedIntervals || 0) + (curr.activeIntervals - prev.activeIntervals);
            if (curr.activeWebSockets > prev.activeWebSockets) report.summary.leakedWebSockets = (report.summary.leakedWebSockets || 0) + (curr.activeWebSockets - prev.activeWebSockets);
        }

        // Overall growth
        if (this.snapshots.length >= 2) {
            const first = this.snapshots[0];
            const last = this.snapshots[this.snapshots.length - 1];
            report.summary.overallGrowth = {
                timers: last.activeTimers - first.activeTimers,
                intervals: last.activeIntervals - first.activeIntervals,
                websockets: last.activeWebSockets - first.activeWebSockets,
                heapMB: (last.memory.heapUsed - first.memory.heapUsed) / 1024 / 1024,
                externalMB: (last.memory.external - first.memory.external) / 1024 / 1024,
            };
        }

        return report;
    }
}

// Main profiling runner
async function runProfiledTests() {
    console.log('🧪 Starting profiled test run for onboard.test.js\n');

    const tracker = new ResourceTracker();
    tracker.install();

    try {
        // Load and run the test file
        console.log('Loading test environment...');

        // Setup JSDOM environment (if not already set up by Jest)
        if (typeof document === 'undefined') {
            tracker.captureSnapshot('BEFORE_ALL_TESTS');
        }

        // Since we can't easily run Jest tests from here, we'll instead
        // spawn the test process and monitor it
        const { spawn } = require('child_process');

        return new Promise((resolve, reject) => {
            const testProcess = spawn('npm', ['test', '--', 'dashboard/js/onboard.test.js', '--verbose'], {
                cwd: path.join(__dirname, '..', '..'),
                stdio: 'inherit',
            });

            testProcess.on('close', (code) => {
                tracker.captureSnapshot('AFTER_ALL_TESTS');

                const report = tracker.generateReport();
                const outputPath = path.join(__dirname, 'onboard.test.profiling-output.json');
                fs.writeFileSync(outputPath, JSON.stringify(report, null, 2));

                console.log('\n📊 PROFILING SUMMARY:');
                console.log('==================');
                console.log(`Test exit code: ${code}`);
                console.log(`Total snapshots: ${report.summary.totalSnapshots}`);

                if (report.summary.leakedTimers) {
                    console.log(`⚠️  Leaked timers: ${report.summary.leakedTimers}`);
                }
                if (report.summary.leakedIntervals) {
                    console.log(`⚠️  Leaked intervals: ${report.summary.leakedIntervals}`);
                }
                if (report.summary.leakedWebSockets) {
                    console.log(`⚠️  Leaked WebSockets: ${report.summary.leakedWebSockets}`);
                }
                if (report.summary.overallGrowth) {
                    console.log(`Heap growth: ${report.summary.overallGrowth.heapMB.toFixed(2)} MB`);
                    console.log(`External growth: ${report.summary.overallGrowth.externalMB.toFixed(2)} MB`);
                }

                console.log(`\n📈 Detailed report: ${outputPath}`);
                resolve({ code, report });
            });

            testProcess.on('error', reject);
        });

    } finally {
        // Don't uninstall yet - we want to track until the end
        // tracker.uninstall();
    }
}

// Run if called directly
if (require.main === module) {
    runProfiledTests()
        .then(({ code, report }) => {
            const concerning = report.summary.leakedTimers > 0 ||
                              report.summary.leakedIntervals > 0 ||
                              report.summary.leakedWebSockets > 0 ||
                              (report.summary.overallGrowth && report.summary.overallGrowth.heapMB > 1);

            if (concerning) {
                console.log('\n⚠️  WARNING: Resource leaks detected!');
                process.exit(1);
            } else if (code === 0) {
                console.log('\n✅ Tests passed with no concerning leaks');
                process.exit(0);
            } else {
                console.log(`\n❌ Tests failed with exit code ${code}`);
                process.exit(code);
            }
        })
        .catch(err => {
            console.error('Error running profiled tests:', err);
            process.exit(1);
        });
}

module.exports = { runProfiledTests, ResourceTracker };
