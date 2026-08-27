#!/usr/bin/env node
/**
 * Standalone Leak Profiling Runner for onboard.test.js
 *
 * This script runs the full test suite with heap profiling, timer tracking,
 * and WebSocket monitoring to capture resource leak evidence.
 *
 * Usage:
 *   node run-leak-profiling.js [options]
 *
 * Options:
 *   --gc             Force garbage collection (requires --expose-gc flag)
 *   --verbose         Enable verbose output
 *   --output <path>  Custom output file (default: test-profiling-results.json)
 *
 * Example with GC:
 *   node --expose-gc run-leak-profiling.js --gc --verbose
 */

const { execSync } = require('child_process');
const fs = require('fs');
const path = require('path');

// Parse command line arguments
const args = process.argv.slice(2);
const options = {
    gc: args.includes('--gc'),
    verbose: args.includes('--verbose'),
    output: 'test-profiling-results.json'
};

// Parse --output flag
const outputIdx = args.indexOf('--output');
if (outputIdx !== -1 && args[outputIdx + 1]) {
    options.output = args[outputIdx + 1];
}

console.log('='.repeat(60));
console.log('Spaxel Onboard Test Suite - Leak Profiling Runner');
console.log('='.repeat(60));
console.log(`Options: ${JSON.stringify(options, null, 2)}`);
console.log('');

// Clean up any previous profiling results
const outputPath = path.join(__dirname, options.output);
if (fs.existsSync(outputPath)) {
    console.log(`Cleaning up previous results: ${outputPath}`);
    fs.unlinkSync(outputPath);
}

// Build Jest command (run from dashboard directory where jest.config.js lives)
let jestCmd = 'npx jest';
jestCmd += ' js/onboard.test.js';  // Relative path from dashboard directory
jestCmd += ' --verbose';

if (options.gc) {
    // Check if --expose-gc is enabled
    if (typeof global.gc === 'undefined') {
        console.warn('Warning: --gc flag passed but --expose-gc is not enabled.');
        console.warn('For GC support, run: node --expose-gc run-leak-profiling.js --gc');
    }
}

// Add Jest environment variables
const env = {
    ...process.env,
    NODE_ENV: 'test',
    FORCE_COLOR: '0'  // Disable colors for cleaner output parsing
};

console.log('');
console.log('Running test suite with profiling...');
console.log('-'.repeat(60));

try {
    const startTime = Date.now();

    // Run Jest with test environment from dashboard directory
    execSync(jestCmd, {
        stdio: 'inherit',
        cwd: __dirname,  // Run from dashboard directory where jest.config.js lives
        env: env
    });

    const duration = Date.now() - startTime;

    console.log('');
    console.log('-'.repeat(60));
    console.log(`Test suite completed in ${duration}ms`);
    console.log('');

    // Check if profiling results were generated
    const resultsPath = path.join(__dirname, '../test-profiling-results.json');
    if (fs.existsSync(resultsPath)) {
        console.log(`✓ Profiling results written to: ${resultsPath}`);
        console.log('');

        // Parse and display summary
        const content = fs.readFileSync(resultsPath, 'utf8');
        const reports = JSON.parse(content);

        if (Array.isArray(reports) && reports.length > 0) {
            const latestReport = reports[reports.length - 1];

            console.log('Latest Profiling Summary:');
            console.log('-'.repeat(60));

            if (latestReport.deltas) {
                const { heap, timers, websockets } = latestReport.deltas;

                console.log(`Heap Usage Delta:    ${heap.usedDeltaMB > 0 ? '+' : ''}${heap.usedDeltaMB} MB`);
                console.log(`Timer Delta:          ${timers.timeoutDelta > 0 ? '+' : ''}${timers.timeoutDelta} timeouts, ${timers.intervalDelta > 0 ? '+' : ''}${timers.intervalDelta} intervals`);
                console.log(`WebSocket Delta:      ${websockets.countDelta > 0 ? '+' : ''}${websockets.countDelta} instances`);
            }

            if (latestReport.analysis) {
                console.log('');
                console.log(`Analysis: ${latestReport.analysis.summary}`);

                if (latestReport.analysis.issuesFound) {
                    console.log('');
                    console.log('Issues Detected:');
                    latestReport.analysis.issues.forEach((issue, i) => {
                        console.log(`  ${i + 1}. [${issue.severity.toUpperCase()}] ${issue.type}`);
                        console.log(`     ${issue.message}`);
                    });
                }
            }

            console.log('');
            console.log('Full details available in the JSON file.');
        } else {
            console.log('⚠ Profiling file exists but contains no valid reports.');
        }
    } else {
        console.log('⚠ No profiling results found. Check test output for errors.');
    }

    console.log('');
    console.log('To view detailed profiling data:');
    console.log(`  cat ${resultsPath} | jq .`);
    console.log('');

} catch (error) {
    console.error('');
    console.error('❌ Test suite failed with exit code:', error.status);
    console.error('');
    process.exit(error.status || 1);
}
