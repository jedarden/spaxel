#!/usr/bin/env node
/**
 * Run the onboard test suite with full resource profiling.
 *
 * Usage: node run-profiled-tests.js
 *
 * Output: onboard.test.profiling-output.json
 */

const { spawn } = require('child_process');
const fs = require('fs');
const path = require('path');

console.log('🧪 Starting profiled test run for onboard.test.js...\n');

// Track active handles before and after
function getActiveHandles() {
    return process._getActiveHandles().map(h => h.type);
}

function getActiveRequests() {
    return process._getActiveRequests().map(r => r.type);
}

// Capture initial state
const beforeHandles = getActiveHandles();
const beforeRequests = getActiveRequests();
const beforeMemory = process.memoryUsage();

console.log('📊 Initial state:');
console.log(`  Active handles: ${beforeHandles.length}`);
console.log(`  Active requests: ${beforeRequests.length}`);
console.log(`  Heap used: ${(beforeMemory.heapUsed / 1024 / 1024).toFixed(2)} MB`);
console.log(`  External: ${(beforeMemory.external / 1024 / 1024).toFixed(2)} MB\n`);

// Run Jest with the test file
const jestProcess = spawn('npm', ['test', '--', 'dashboard/js/onboard.test.js', '--verbose'], {
    cwd: path.join(__dirname, '..', '..'),
    stdio: 'inherit',
    env: { ...process.env, NODE_ENV: 'test' },
});

let testPassed = false;

jestProcess.on('close', (code) => {
    // Capture final state
    const afterHandles = getActiveHandles();
    const afterRequests = getActiveRequests();
    const afterMemory = process.memoryUsage();

    console.log('\n📊 Final state:');
    console.log(`  Active handles: ${afterHandles.length} (delta: ${afterHandles.length - beforeHandles.length})`);
    console.log(`  Active requests: ${afterRequests.length} (delta: ${afterRequests.length - beforeRequests.length})`);
    console.log(`  Heap used: ${(afterMemory.heapUsed / 1024 / 1024).toFixed(2)} MB (delta: ${((afterMemory.heapUsed - beforeMemory.heapUsed) / 1024 / 1024).toFixed(2)} MB)`);
    console.log(`  External: ${(afterMemory.external / 1024 / 1024).toFixed(2)} MB (delta: ${((afterMemory.external - beforeMemory.external) / 1024 / 1024).toFixed(2)} MB)`);

    // Analyze handle types
    console.log('\n🔍 Handle breakdown:');
    const handleTypes = {};
    afterHandles.forEach(type => {
        handleTypes[type] = (handleTypes[type] || 0) + 1;
    });
    Object.entries(handleTypes).sort((a, b) => b[1] - a[1]).forEach(([type, count]) => {
        console.log(`  ${type}: ${count}`);
    });

    // Build profiling report
    const report = {
        timestamp: new Date().toISOString(),
        exitCode: code,
        testPassed: code === 0,
        resourceDelta: {
            handles: afterHandles.length - beforeHandles.length,
            requests: afterRequests.length - beforeRequests.length,
            heapUsedMB: (afterMemory.heapUsed - beforeMemory.heapUsed) / 1024 / 1024,
            externalMB: (afterMemory.external - beforeMemory.external) / 1024 / 1024,
        },
        handleBreakdown: handleTypes,
        beforeState: {
            handles: beforeHandles.length,
            requests: beforeRequests.length,
            heapUsedMB: beforeMemory.heapUsed / 1024 / 1024,
            externalMB: beforeMemory.external / 1024 / 1024,
        },
        afterState: {
            handles: afterHandles.length,
            requests: afterRequests.length,
            heapUsedMB: afterMemory.heapUsed / 1024 / 1024,
            externalMB: afterMemory.external / 1024 / 1024,
        },
    };

    // Write report
    const outputPath = path.join(__dirname, 'onboard.test.profiling-output.json');
    fs.writeFileSync(outputPath, JSON.stringify(report, null, 2));
    console.log(`\n📈 Profiling report written to: ${outputPath}`);

    // Check for concerning leaks
    const concerning = report.resourceDelta.handles > 0 ||
                      report.resourceDelta.heapUsedMB > 1;

    if (concerning) {
        console.log('\n⚠️  WARNING: Possible resource leaks detected!');
        process.exit(1);
    } else if (code === 0) {
        console.log('\n✅ All tests passed with no concerning resource leaks');
        process.exit(0);
    } else {
        console.log(`\n❌ Tests failed with exit code ${code}`);
        process.exit(code);
    }
});

jestProcess.on('error', (err) => {
    console.error('Failed to start Jest process:', err);
    process.exit(1);
});
