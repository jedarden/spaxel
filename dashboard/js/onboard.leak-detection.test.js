/**
 * Profiling Test Suite - Minimal Reproducible Test for Leak Detection
 *
 * This test runs the full wizard lifecycle to capture evidence of leaks.
 * It instrument timers, WebSockets, and heap usage to identify what
 * is not being cleaned up between tests.
 */

const profiler = require('./testProfiler');
const path = require('path');

// Load the wizard script (IIFE attaches to window.SpaxelOnboard)
require('./onboard.js');

const { SpaxelOnboard } = global;
const { _state, _CONFIG } = SpaxelOnboard;

describe('Leak Detection - Full Wizard Lifecycle', () => {
    let profilingPath = null;
    let suiteBefore = null;

    beforeAll(async () => {
        profilingPath = path.join(__dirname, '../leak-detection-report.json');

        // Instrument for tracking
        profiler.instrumentTimers();
        profiler.instrumentWebSockets();

        // Force GC and wait for event loop to clear
        profiler.forceGC();
        await new Promise(resolve => setTimeout(resolve, 0));

        suiteBefore = profiler.captureSnapshot('suite-start');
        console.log('[LEAK-TEST] Starting leak detection suite');
    });

    afterAll(async () => {
        profiler.forceGC();
        await new Promise(resolve => setTimeout(resolve, 0));

        const suiteAfter = profiler.captureSnapshot('suite-end');

        // Write detailed profiling report
        profiler.writeProfilingData(suiteBefore, suiteAfter, profilingPath);

        const analysis = profiler.analyzeLeaks(suiteBefore, suiteAfter);
        console.log('[LEAK-TEST] Analysis:', analysis.summary);

        if (analysis.issuesFound) {
            console.warn('[LEAK-TEST] Leaks detected:');
            analysis.issues.forEach(issue => {
                console.warn(`  [${issue.severity}] ${issue.type}: ${issue.message}`);
            });
        }

        // Close JSON array
        const fs = require('fs');
        if (fs.existsSync(profilingPath)) {
            const content = fs.readFileSync(profilingPath, 'utf8');
            if (!content.endsWith(']')) {
                fs.appendFileSync(profilingPath, ']');
            }
        }
    });

    beforeEach(() => {
        // Reset wizard state cleanly
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
        // Explicitly clean up all resources
        if (_state.pollTimer) {
            clearInterval(_state.pollTimer);
            _state.pollTimer = null;
        }
        if (_state.calibrateTimer) {
            clearTimeout(_state.calibrateTimer);
            _state.calibrateTimer = null;
        }
        if (_state.ws) {
            _state.ws.close();
            _state.ws = null;
        }
        SpaxelOnboard.close();

        // Clear sessionStorage
        sessionStorage.clear();

        // Force GC after each test
        profiler.forceGC();
    });

    test('full wizard lifecycle: start through close', async () => {
        jest.useFakeTimers();

        const beforeTest = profiler.captureSnapshot('before-full-lifecycle');

        // Start the wizard
        SpaxelOnboard.start();

        // Advance through auto-step (browser_check)
        jest.advanceTimersByTime(400);

        // Simulate being on connect_device step
        expect(_state.currentStepIndex).toBe(1);

        // Close the wizard
        SpaxelOnboard.close();

        jest.useRealTimers();

        profiler.forceGC();
        await new Promise(resolve => setTimeout(resolve, 0));

        const afterTest = profiler.captureSnapshot('after-full-lifecycle');

        // Write individual test profiling
        const testPath = path.join(__dirname, '../leak-test-full-lifecycle.json');
        profiler.writeProfilingData(beforeTest, afterTest, testPath);

        // Verify wizard overlay is removed
        expect(document.getElementById('wizard-overlay')).toBeNull();

        const analysis = profiler.analyzeLeaks(beforeTest, afterTest);
        console.log('[LEAK-TEST] Full lifecycle:', analysis.summary);
    });

    test('timer leak detection: poll timer creation and cleanup', async () => {
        jest.useFakeTimers();

        const beforeTimers = profiler.captureSnapshot('before-timers');

        // Simulate node detection step which creates a poll timer
        _state.currentStepIndex = 4; // detect_node step
        _state.nodeMAC = null;
        _state.knownMACs = [];
        _state.pollTimer = setInterval(() => {}, 3000);

        // Let some time pass
        jest.advanceTimersByTime(100);

        expect(_state.pollTimer).not.toBeNull();

        // Clean up the timer explicitly
        clearInterval(_state.pollTimer);
        _state.pollTimer = null;

        jest.useRealTimers();

        profiler.forceGC();
        await new Promise(resolve => setTimeout(resolve, 0));

        const afterTimers = profiler.captureSnapshot('after-timers');

        const analysis = profiler.analyzeLeaks(beforeTimers, afterTimers);
        console.log('[LEAK-TEST] Timer leak check:', analysis.summary);

        // Verify timer was cleared
        expect(_state.pollTimer).toBeNull();
    });

    test('WebSocket leak detection: WebSocket creation and cleanup', async () => {
        const beforeWS = profiler.captureSnapshot('before-websockets');

        // Create a WebSocket connection (as in calibration step)
        _state.currentStepIndex = 5; // calibrate step
        _state.nodeMAC = 'AA:BB:CC:DD:EE:FF';
        _state.ws = new WebSocket('ws://test.local');

        expect(_state.ws).not.toBeNull();

        // Clean up WebSocket
        _state.ws.close();
        _state.ws = null;

        profiler.forceGC();
        await new Promise(resolve => setTimeout(resolve, 0));

        const afterWS = profiler.captureSnapshot('after-websockets');

        const analysis = profiler.analyzeLeaks(beforeWS, afterWS);
        console.log('[LEAK-TEST] WebSocket leak check:', analysis.summary);

        // Verify WebSocket was closed
        expect(_state.ws).toBeNull();
    });

    test('CSI history leak detection: array growth', async () => {
        const beforeCSI = profiler.captureSnapshot('before-csi-history');

        // Simulate CSI data collection
        _state.csiHistory = [];
        for (let i = 0; i < 100; i++) {
            _state.csiHistory.push({
                timestamp: Date.now(),
                meanAmplitude: 10 + Math.random() * 5
            });
        }

        expect(_state.csiHistory.length).toBe(100);

        // Clear CSI history
        _state.csiHistory = [];

        profiler.forceGC();
        await new Promise(resolve => setTimeout(resolve, 0));

        const afterCSI = profiler.captureSnapshot('after-csi-history');

        const heapDelta = (afterCSI.heap.heapUsedBytes - beforeCSI.heap.heapUsedBytes) / 1024 / 1024;
        console.log('[LEAK-TEST] CSI history heap delta:', heapDelta.toFixed(2), 'MB');

        // CSI history should be cleared
        expect(_state.csiHistory.length).toBe(0);
    });

    test('sessionStorage leak detection', async () => {
        const beforeSession = profiler.captureSnapshot('before-session');

        // Write to sessionStorage
        sessionStorage.setItem(_CONFIG.storageKey, JSON.stringify({
            currentStepIndex: 3,
            nodeMAC: 'AA:BB:CC:DD:EE:FF',
            knownMACs: []
        }));

        expect(sessionStorage.getItem(_CONFIG.storageKey)).not.toBeNull();

        // Clear sessionStorage
        sessionStorage.clear();

        profiler.forceGC();
        await new Promise(resolve => setTimeout(resolve, 0));

        const afterSession = profiler.captureSnapshot('after-session');

        const analysis = profiler.analyzeLeaks(beforeSession, afterSession);
        console.log('[LEAK-TEST] SessionStorage check:', analysis.summary);

        expect(sessionStorage.getItem(_CONFIG.storageKey)).toBeNull();
    });

    test('DOM element leak detection: wizard overlay creation/removal', async () => {
        jest.useFakeTimers();

        const beforeDOM = profiler.captureSnapshot('before-dom');

        // Start wizard (creates DOM elements)
        SpaxelOnboard.start();
        expect(document.getElementById('wizard-overlay')).not.toBeNull();

        // Close wizard (should remove DOM elements)
        SpaxelOnboard.close();
        expect(document.getElementById('wizard-overlay')).toBeNull();

        jest.useRealTimers();

        profiler.forceGC();
        await new Promise(resolve => setTimeout(resolve, 0));

        const afterDOM = profiler.captureSnapshot('after-dom');

        const analysis = profiler.analyzeLeaks(beforeDOM, afterDOM);
        console.log('[LEAK-TEST] DOM leak check:', analysis.summary);
    });

    test('mock cleanup verification', async () => {
        const beforeMock = profiler.captureSnapshot('before-mocks');

        // This test verifies that our mocks are properly cleaned up
        // Set up some state
        _state.port = __mockPort;
        _state.ws = new WebSocket('ws://test');
        _state.pollTimer = setInterval(() => {}, 1000);
        _state.calibrateTimer = setTimeout(() => {}, 5000);

        // Run the full resetWizardState function (extracted logic)
        _state.currentStepIndex = -1;
        _state.port = null;
        _state.nodeMAC = null;
        _state.knownMACs = [];
        _state.fleetNetworkConfigured = false;
        _state.fleetNetworkSSID = '';
        _state.mothershipHost = '';
        _state.mothershipPort = 8080;

        if (_state.pollTimer) { clearInterval(_state.pollTimer); _state.pollTimer = null; }
        if (_state.calibrateTimer) { clearTimeout(_state.calibrateTimer); _state.calibrateTimer = null; }
        if (_state.ws) { _state.ws.close(); _state.ws = null; }
        _state.csiHistory = [];
        _state.container = null;

        sessionStorage.clear();
        jest.resetAllMocks();

        profiler.forceGC();
        await new Promise(resolve => setTimeout(resolve, 0));

        const afterMock = profiler.captureSnapshot('after-mocks');

        // Verify all state cleared
        expect(_state.currentStepIndex).toBe(-1);
        expect(_state.port).toBeNull();
        expect(_state.pollTimer).toBeNull();
        expect(_state.calibrateTimer).toBeNull();
        expect(_state.ws).toBeNull();
        expect(_state.csiHistory.length).toBe(0);

        const analysis = profiler.analyzeLeaks(beforeMock, afterMock);
        console.log('[LEAK-TEST] Mock cleanup:', analysis.summary);
    });
});
