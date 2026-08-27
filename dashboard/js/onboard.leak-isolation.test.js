/**
 * Isolated Leak Tests - Target Individual Components
 *
 * Each test focuses on ONE specific potential leak source to isolate
 * the exact component causing the timer leak.
 */

const profiler = require('./testProfiler');
const path = require('path');

// Load the wizard script
require('./onboard.js');

const { SpaxelOnboard } = global;
const { _state, _CONFIG } = SpaxelOnboard;

describe('Leak Isolation - Targeted Component Tests', () => {
    let profilingResults = [];

    beforeAll(async () => {
        profiler.instrumentTimers();
        profiler.instrumentWebSockets();
        profiler.forceGC();
        await new Promise(resolve => setTimeout(resolve, 0));
        console.log('[LEAK-ISOLATION] Starting isolated leak tests');
    });

    afterAll(async () => {
        // Write combined results
        const resultPath = path.join(__dirname, '../leak-isolation-results.json');
        const fs = require('fs');

        if (profilingResults.length > 0) {
            fs.writeFileSync(
                resultPath,
                JSON.stringify(profilingResults, null, 2)
            );
            console.log(`[LEAK-ISOLATION] Results written to ${resultPath}`);

            // Summarize findings
            console.log('\n[LEAK-ISOLATION] SUMMARY:');
            profilingResults.forEach(result => {
                const heapDeltaMB = (
                    (result.after.heap.heapUsedBytes - result.before.heap.heapUsedBytes) /
                    1024 / 1024
                ).toFixed(2);
                const timerDelta = result.after.timers.total - result.before.timers.total;

                console.log(`  ${result.testName}:`);
                console.log(`    Heap delta: ${heapDeltaMB > 0 ? '+' : ''}${heapDeltaMB} MB`);
                console.log(`    Timer delta: ${timerDelta > 0 ? '+' : ''}${timerDelta}`);
                console.log(`    Verdict: ${result.verdict}`);
            });
        }
    });

    beforeEach(() => {
        // Clean reset
        _state.currentStepIndex = -1;
        _state.port = null;
        _state.nodeMAC = null;
        _state.knownMACs = [];
        _state.pollTimer = null;
        _state.calibrateTimer = null;
        _state.calibratePhase = 'idle';
        _state.ws = null;
        _state.csiHistory = [];
        _state.container = null;
        sessionStorage.clear();
        jest.resetAllMocks();

        // Re-apply mocks
        fetch.mockResolvedValue({
            ok: true,
            json: jest.fn().mockResolvedValue([]),
        });
        navigator.serial.requestPort.mockResolvedValue(__mockPort);
        navigator.serial.getPorts.mockResolvedValue([__mockPort]);
        crypto.randomUUID.mockReturnValue('test-uuid-1234');
        __mockPort.open.mockResolvedValue(undefined);
        __mockPort.close.mockResolvedValue(undefined);
        __mockPort.readable.pipeTo.mockResolvedValue(undefined);
        WebSocket.mockImplementation(function () {
            return {
                binaryType: 'arraybuffer',
                close: jest.fn(),
                send: jest.fn(),
                readyState: 1,
                onopen: null,
                onclose: null,
                onerror: null,
                onmessage: null,
            };
        });
    });

    afterEach(() => {
        profiler.forceGC();
    });

    // ============================================================================
    // TEST 1: Fake Timers Without Proper Cleanup
    // ============================================================================
    test('ISOLATION-1: jest.useFakeTimers() without cleanup', async () => {
        const testName = 'fake-timers-no-cleanup';
        const before = profiler.captureSnapshot(`${testName}-before`);

        // Simulate test that uses fake timers but forgets to restore
        jest.useFakeTimers();
        jest.advanceTimersByTime(1000);

        // NO jest.useRealTimers() call here - this simulates the leak

        profiler.forceGC();
        await new Promise(resolve => setTimeout(resolve, 0));

        const after = profiler.captureSnapshot(`${testName}-after`);

        // NOW clean up (too late)
        jest.useRealTimers();

        const result = {
            testName,
            before,
            after,
            verdict: after.timers.total > before.timers.total ? 'LEAKS' : 'CLEAN'
        };
        profilingResults.push(result);
    });

    // ============================================================================
    // TEST 2: Fake Timers WITH Proper Cleanup
    // ============================================================================
    test('ISOLATION-2: jest.useFakeTimers() WITH cleanup', async () => {
        const testName = 'fake-timers-with-cleanup';
        const before = profiler.captureSnapshot(`${testName}-before`);

        // Proper pattern: use fake timers then restore
        jest.useFakeTimers();
        jest.advanceTimersByTime(1000);
        jest.useRealTimers(); // <-- Proper cleanup

        profiler.forceGC();
        await new Promise(resolve => setTimeout(resolve, 0));

        const after = profiler.captureSnapshot(`${testName}-after`);

        const result = {
            testName,
            before,
            after,
            verdict: after.timers.total > before.timers.total ? 'LEAKS' : 'CLEAN'
        };
        profilingResults.push(result);
    });

    // ============================================================================
    // TEST 3: Wizard Lifecycle (Most Suspect - lines 529-600)
    // ============================================================================
    test('ISOLATION-3: Wizard lifecycle without afterEach cleanup', async () => {
        const testName = 'wizard-lifecycle-no-aftereach';
        const before = profiler.captureSnapshot(`${testName}-before`);

        // Simulate the 'Wizard lifecycle' test pattern
        jest.useFakeTimers();
        SpaxelOnboard.start();
        jest.advanceTimersByTime(400);

        // Simulate test that calls close() but doesn't have afterEach cleanup
        SpaxelOnboard.close();

        // NO afterEach cleanup - no timer/WebSocket cleanup

        profiler.forceGC();
        await new Promise(resolve => setTimeout(resolve, 0));

        const after = profiler.captureSnapshot(`${testName}-after`);

        // Clean up now (too late)
        jest.useRealTimers();
        if (_state.pollTimer) { clearInterval(_state.pollTimer); _state.pollTimer = null; }
        if (_state.calibrateTimer) { clearTimeout(_state.calibrateTimer); _state.calibrateTimer = null; }
        if (_state.ws) { _state.ws.close(); _state.ws = null; }

        const result = {
            testName,
            before,
            after,
            verdict: after.timers.total > before.timers.total ? 'LEAKS' : 'CLEAN'
        };
        profilingResults.push(result);
    });

    // ============================================================================
    // TEST 4: Wizard Lifecycle WITH afterEach Cleanup
    // ============================================================================
    test('ISOLATION-4: Wizard lifecycle WITH afterEach cleanup', async () => {
        const testName = 'wizard-lifecycle-with-aftereach';
        const before = profiler.captureSnapshot(`${testName}-before`);

        // Same pattern but WITH proper afterEach cleanup
        jest.useFakeTimers();
        SpaxelOnboard.start();
        jest.advanceTimersByTime(400);
        SpaxelOnboard.close();

        // Proper afterEach cleanup
        if (_state.pollTimer) { clearInterval(_state.pollTimer); _state.pollTimer = null; }
        if (_state.calibrateTimer) { clearTimeout(_state.calibrateTimer); _state.calibrateTimer = null; }
        if (_state.ws) { _state.ws.close(); _state.ws = null; }
        jest.useRealTimers();

        profiler.forceGC();
        await new Promise(resolve => setTimeout(resolve, 0));

        const after = profiler.captureSnapshot(`${testName}-after`);

        const result = {
            testName,
            before,
            after,
            verdict: after.timers.total > before.timers.total ? 'LEAKS' : 'CLEAN'
        };
        profilingResults.push(result);
    });

    // ============================================================================
    // TEST 5: setTimeout in beforeAll/afterAll (Suspect from catalog)
    // ============================================================================
    test('ISOLATION-5: setTimeout in beforeAll hook', async () => {
        const testName = 'settimeout-beforeall-hook';
        const before = profiler.captureSnapshot(`${testName}-before`);

        // Simulate pattern from leak catalog line 80
        await new Promise(resolve => setTimeout(resolve, 0));

        profiler.forceGC();
        await new Promise(resolve => setTimeout(resolve, 0));

        const after = profiler.captureSnapshot(`${testName}-after`);

        const result = {
            testName,
            before,
            after,
            verdict: after.timers.total > before.timers.total ? 'LEAKS' : 'CLEAN'
        };
        profilingResults.push(result);
    });

    // ============================================================================
    // TEST 6: Node Detection Step (Creates pollTimer)
    // ============================================================================
    test('ISOLATION-6: Node detection pollTimer without cleanup', async () => {
        const testName = 'node-detection-polltimer-no-cleanup';
        const before = profiler.captureSnapshot(`${testName}-before`);

        // Simulate node detection step creating pollTimer
        _state.currentStepIndex = 4; // detect_node
        _state.nodeMAC = null;
        _state.knownMACs = [];

        jest.useFakeTimers();
        _state.pollTimer = setInterval(() => {}, 3000);
        jest.advanceTimersByTime(100);

        // NO cleanup

        profiler.forceGC();
        await new Promise(resolve => setTimeout(resolve, 0));

        const after = profiler.captureSnapshot(`${testName}-after`);

        // Cleanup now (too late)
        jest.useRealTimers();
        if (_state.pollTimer) { clearInterval(_state.pollTimer); _state.pollTimer = null; }

        const result = {
            testName,
            before,
            after,
            verdict: after.timers.total > before.timers.total ? 'LEAKS' : 'CLEAN'
        };
        profilingResults.push(result);
    });

    // ============================================================================
    // TEST 7: Calibration Step (Creates calibrateTimer)
    // ============================================================================
    test('ISOLATION-7: Calibration calibrateTimer without cleanup', async () => {
        const testName = 'calibration-calibratetimer-no-cleanup';
        const before = profiler.captureSnapshot(`${testName}-before`);

        // Simulate calibration step creating calibrateTimer
        _state.currentStepIndex = 5; // calibrate
        _state.nodeMAC = 'AA:BB:CC:DD:EE:FF';

        jest.useFakeTimers();
        _state.calibrateTimer = setTimeout(() => {}, 5000);
        jest.advanceTimersByTime(100);

        // NO cleanup

        profiler.forceGC();
        await new Promise(resolve => setTimeout(resolve, 0));

        const after = profiler.captureSnapshot(`${testName}-after`);

        // Cleanup now (too late)
        jest.useRealTimers();
        if (_state.calibrateTimer) { clearTimeout(_state.calibrateTimer); _state.calibrateTimer = null; }

        const result = {
            testName,
            before,
            after,
            verdict: after.timers.total > before.timers.total ? 'LEAKS' : 'CLEAN'
        };
        profilingResults.push(result);
    });

    // ============================================================================
    // TEST 8: Multiple Sequential Tests with Fake Timers
    // ============================================================================
    test('ISOLATION-8: Multiple sequential fake timer uses', async () => {
        const testName = 'multiple-sequential-fake-timers';
        const before = profiler.captureSnapshot(`${testName}-before`);

        // Simulate multiple tests using fake timers sequentially
        for (let i = 0; i < 5; i++) {
            jest.useFakeTimers();
            jest.advanceTimersByTime(100);
            // FORGET to call jest.useRealTimers()
        }

        profiler.forceGC();
        await new Promise(resolve => setTimeout(resolve, 0));

        const after = profiler.captureSnapshot(`${testName}-after`);

        // Cleanup now
        jest.useRealTimers();

        const result = {
            testName,
            before,
            after,
            verdict: after.timers.total > before.timers.total ? 'LEAKS' : 'CLEAN'
        };
        profilingResults.push(result);
    });

    // ============================================================================
    // TEST 9: WebSocket Creation Without Cleanup
    // ============================================================================
    test('ISOLATION-9: WebSocket creation without cleanup', async () => {
        const testName = 'websocket-no-cleanup';
        const before = profiler.captureSnapshot(`${testName}-before`);

        // Create WebSocket but don't close it
        _state.ws = new WebSocket('ws://test.local');

        profiler.forceGC();
        await new Promise(resolve => setTimeout(resolve, 0));

        const after = profiler.captureSnapshot(`${testName}-after`);

        // Cleanup now
        if (_state.ws) { _state.ws.close(); _state.ws = null; }

        const result = {
            testName,
            before,
            after,
            verdict: after.timers.total > before.timers.total ? 'LEAKS' : 'CLEAN'
        };
        profilingResults.push(result);
    });
});
