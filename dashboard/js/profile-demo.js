#!/usr/bin/env node
/**
 * Simple demonstration of profiling infrastructure
 *
 * This simulates the lifecycle of the onboarding wizard tests
 * to verify that profiling captures resource usage patterns.
 */

const { ResourceTracker, Instrumentation, ProfilingTestRunner } = require('./onboard.test.profiling.js');

console.log('🧪 Running profiling demonstration...\n');

// Create tracker and instrumentation
const tracker = new ResourceTracker();
const instrumentation = new Instrumentation(tracker);

// Install instrumentation to track resource creation
instrumentation.install();

// Simulate test phases
async function runDemo() {
    // Phase 1: Before any tests
    tracker.captureSnapshot('BEFORE_ALL_TESTS');

    // Simulate test 1: Browser check (creates timers)
    console.log('Running: Browser check tests...');
    await new Promise(resolve => setTimeout(resolve, 50));
    const t1 = setTimeout(() => {}, 1000);
    tracker.captureSnapshot('AFTER_BROWSER_CHECK_TESTS');

    // Simulate cleanup
    clearTimeout(t1);

    // Phase 2: Serial port tests (no new resources)
    console.log('Running: Serial port tests...');
    await new Promise(resolve => setTimeout(resolve, 50));
    tracker.captureSnapshot('AFTER_SERIAL_TESTS');

    // Simulate test 3: Node detection (creates intervals)
    console.log('Running: Node detection tests...');
    const i1 = setInterval(() => {}, 3000);
    await new Promise(resolve => setTimeout(resolve, 50));
    tracker.captureSnapshot('AFTER_NODE_DETECTION_TESTS');

    // Simulate partial cleanup - leak this interval
    // clearInterval(i1);  // DELIBERATELY NOT CLEARED TO DEMONSTRATE LEAK DETECTION

    // Phase 4: Final state
    tracker.captureSnapshot('AFTER_ALL_TESTS');

    // Generate and output report
    const report = tracker.generateReport();

    console.log('\n📊 PROFILING REPORT:');
    console.log('==================');
    console.log(`Timestamp: ${report.timestamp}`);
    console.log(`Total snapshots: ${report.summary.totalSnapshots}`);
    console.log(`Leaked timers: ${report.summary.totalLeakedTimers}`);
    console.log(`Leaked intervals: ${report.summary.totalLeakedIntervals}`);
    console.log(`Leaked WebSockets: ${report.summary.totalLeakedWebSockets}`);
    console.log(`Heap growth: ${(report.summary.totalHeapGrowth / 1024 / 1024).toFixed(2)} MB`);

    console.log('\n📈 Delta details:');
    report.deltas.forEach(delta => {
        console.log(`  "${delta.fromLabel}" → "${delta.toLabel}":`);
        console.log(`    Timers: ${delta.timers > 0 ? '+' : ''}${delta.timers}`);
        console.log(`    Intervals: ${delta.intervals > 0 ? '+' : ''}${delta.intervals}`);
        console.log(`    WebSockets: ${delta.websockets > 0 ? '+' : ''}${delta.websockets}`);
        console.log(`    Heap: ${(delta.heapGrowth / 1024).toFixed(2)} KB`);
    });

    // Write report to file
    const fs = require('fs');
    const path = require('path');
    const outputPath = path.join(__dirname, 'onboard.test.profiling-output.json');
    fs.writeFileSync(outputPath, JSON.stringify(report, null, 2));
    console.log(`\n📁 Report written to: ${outputPath}`);

    // Cleanup
    instrumentation.uninstall();

    // Return result
    return {
        leakedResources: report.summary.totalLeakedTimers +
                         report.summary.totalLeakedIntervals +
                         report.summary.totalLeakedWebSockets,
        heapGrowthMB: report.summary.totalHeapGrowth / 1024 / 1024,
    };
}

// Run the demo
runDemo()
    .then(result => {
        console.log('\n✅ Demo complete');
        console.log(`Total leaked resources: ${result.leakedResources}`);
        console.log(`Heap growth: ${result.heapGrowthMB.toFixed(2)} MB`);

        if (result.leakedResources > 0) {
            console.log('\n⚠️  Demonstrated leak detection working!');
            process.exit(0); // Exit 0 because we intentionally leaked to show it works
        } else {
            console.log('\n✅ No leaks detected');
            process.exit(0);
        }
    })
    .catch(err => {
        console.error('Demo failed:', err);
        process.exit(1);
    });
