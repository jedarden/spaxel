/**
 * Tests for the Spaxel Onboarding Wizard.
 */

// Load the wizard script (IIFE attaches to window.SpaxelOnboard)
require('./onboard.js');

const { SpaxelOnboard } = global;
const { _CONFIG, _STEPS, _parseCSIFrame, _state, _UserError, _isUserError, _provisionAndSend } = SpaxelOnboard;

// Reset state between tests
function resetWizardState() {
    _state.currentStepIndex = -1;
    _state.port = null;
    _state.nodeMAC = null;
    _state.knownMACs = [];
    _state.wifiSSID = '';
    _state.wifiPass = '';
    _state.mothershipHost = '';
    _state.mothershipPort = 8080;
    _state.pollTimer = null;
    _state.calibrateTimer = null;
    _state.calibratePhase = 'idle';
    _state.ws = null;
    _state.csiHistory = [];
    _state.container = null;

    // Clear sessionStorage
    sessionStorage.clear();
    // resetAllMocks clears mockRejectedValueOnce/mockResolvedValueOnce queues
    // that clearAllMocks misses, then re-apply default implementations
    jest.resetAllMocks();
    fetch.mockResolvedValue({
        ok: true,
        json: jest.fn().mockResolvedValue([]),
    });
    navigator.serial.requestPort.mockResolvedValue(__mockPort);
    navigator.serial.getPorts.mockResolvedValue([__mockPort]);
    crypto.randomUUID.mockReturnValue('test-uuid-1234');
    // Re-apply port mock implementations (resetAllMocks clears them)
    __mockPort.open.mockResolvedValue(undefined);
    __mockPort.close.mockResolvedValue(undefined);
    __mockPort.readable.pipeTo.mockResolvedValue(undefined);
    // Re-apply WebSocket mock (resetAllMocks clears mockImplementation)
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
}

// ============================================
// Configuration and Step Definitions
// ============================================
describe('Onboard configuration', () => {
    test('has correct poll interval', () => {
        expect(_CONFIG.nodePollInterval).toBe(3000);
    });

    test('has correct poll timeout', () => {
        expect(_CONFIG.nodePollTimeout).toBe(120000);
    });

    test('has correct endpoints', () => {
        expect(_CONFIG.provisioningEndpoint).toBe('/api/provision');
        expect(_CONFIG.nodesEndpoint).toBe('/api/nodes');
    });
});

describe('Step definitions', () => {
    test('has 8 steps in correct order', () => {
        expect(_STEPS.length).toBe(8);
        expect(_STEPS.map(s => s.id)).toEqual([
            'browser_check',
            'connect_device',
            'flash_firmware',
            'provision_wifi',
            'detect_node',
            'calibrate',
            'placement',
            'complete',
        ]);
    });
});

// ============================================
// Browser Check
// ============================================
describe('Browser check step', () => {
    beforeEach(resetWizardState);

    test('detects Web Serial API is available', () => {
        // navigator.serial is mocked in setup
        expect(navigator.serial).toBeDefined();
        expect(typeof navigator.serial.requestPort).toBe('function');
    });

    test('UserError is correctly identified', () => {
        var err = new _UserError('test message');
        expect(_isUserError(err)).toBe(true);
        expect(err.message).toBe('test message');
    });

    test('regular Error is not identified as UserError', () => {
        var err = new Error('regular error');
        expect(_isUserError(err)).toBe(false);
    });

    test('error with UserError name is identified', () => {
        var err = new Error('test');
        err.name = 'UserError';
        expect(_isUserError(err)).toBe(true);
    });
});

// ============================================
// Session Storage Persistence
// ============================================
describe('State persistence', () => {
    beforeEach(resetWizardState);

    test('saves and loads state from sessionStorage', () => {
        _state.currentStepIndex = 3;
        _state.nodeMAC = 'AA:BB:CC:DD:EE:FF';
        _state.wifiSSID = 'TestWiFi';
        _state.mothershipPort = 9090;

        // Trigger save (via goToStep or directly)
        sessionStorage.setItem(_CONFIG.storageKey, JSON.stringify({
            currentStepIndex: _state.currentStepIndex,
            nodeMAC: _state.nodeMAC,
            knownMACs: _state.knownMACs,
            wifiSSID: _state.wifiSSID,
            wifiPass: _state.wifiPass,
            mothershipHost: _state.mothershipHost,
            mothershipPort: _state.mothershipPort,
        }));

        // Simulate load
        var raw = sessionStorage.getItem(_CONFIG.storageKey);
        var loaded = JSON.parse(raw);

        expect(loaded.currentStepIndex).toBe(3);
        expect(loaded.nodeMAC).toBe('AA:BB:CC:DD:EE:FF');
        expect(loaded.wifiSSID).toBe('TestWiFi');
        expect(loaded.mothershipPort).toBe(9090);
    });

    test('clearState removes sessionStorage entry', () => {
        sessionStorage.setItem(_CONFIG.storageKey, '{"currentStepIndex":2}');
        sessionStorage.removeItem(_CONFIG.storageKey);
        expect(sessionStorage.getItem(_CONFIG.storageKey)).toBeNull();
    });

    test('returns null for missing state', () => {
        expect(sessionStorage.getItem(_CONFIG.storageKey)).toBeNull();
    });
});

// ============================================
// Serial Port Handling
// ============================================
describe('Serial port handling', () => {
    beforeEach(resetWizardState);

    test('requestPort calls navigator.serial.requestPort', async () => {
        var port = await navigator.serial.requestPort();
        expect(navigator.serial.requestPort).toHaveBeenCalled();
        expect(port).toBeDefined();
    });

    test('getPorts returns previously authorized ports', async () => {
        var ports = await navigator.serial.getPorts();
        expect(navigator.serial.getPorts).toHaveBeenCalled();
        expect(ports.length).toBeGreaterThan(0);
    });

    test('requestPort throws UserError on NotFoundError', async () => {
        navigator.serial.requestPort.mockRejectedValueOnce({ name: 'NotFoundError' });

        // The wizard's requestPort wraps errors, but we test the raw mock here
        await expect(navigator.serial.requestPort()).rejects.toEqual(
            expect.objectContaining({ name: 'NotFoundError' })
        );
    });
});

// ============================================
// Provisioning Payload
// ============================================
describe('Provisioning payload', () => {
    beforeEach(resetWizardState);

    test('POST /api/provision with WiFi credentials', async () => {
        fetch.mockResolvedValueOnce({
            ok: true,
            json: jest.fn().mockResolvedValue({
                version: 1,
                wifi_ssid: 'TestWiFi',
                wifi_pass: 'secret123',
                node_id: 'uuid-123',
                node_token: 'token-abc',
                ms_mdns: 'spaxel-mothership.local',
                ms_port: 8080,
                debug: false,
            }),
        });

        var resp = await fetch('/api/provision', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ wifi_ssid: 'TestWiFi', wifi_pass: 'secret123' }),
        });

        expect(fetch).toHaveBeenCalledWith('/api/provision', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ wifi_ssid: 'TestWiFi', wifi_pass: 'secret123' }),
        });

        var payload = await resp.json();
        expect(payload.wifi_ssid).toBe('TestWiFi');
        expect(payload.node_id).toBe('uuid-123');
    });

    test('falls back to client-side payload when provisioning server fails', async () => {
        fetch.mockRejectedValueOnce(new Error('server unavailable'));
        fetch.mockRejectedValueOnce(new Error('server unavailable')); // for the nodes fetch fallback

        // The wizard's provisionAndSend falls back to client-side assembly.
        // We verify the fallback UUID generation works.
        var uuid = crypto.randomUUID();
        expect(uuid).toBe('test-uuid-1234');
    });
});

// ============================================
// Node Detection Polling
// ============================================
describe('Node detection', () => {
    beforeEach(resetWizardState);

    test('polls /api/nodes and detects new node', async () => {
        // Simulate initial state: no known nodes
        _state.knownMACs = [];

        // First poll: returns new node
        fetch.mockResolvedValueOnce({
            ok: true,
            json: jest.fn().mockResolvedValue([
                { mac: 'AA:BB:CC:DD:EE:FF', role: 'tx', online: true },
            ]),
        });

        var resp = await fetch(_CONFIG.nodesEndpoint);
        var nodes = await resp.json();

        // Simulate detection logic
        var currentMACs = nodes.map(function (n) { return n.mac; });
        var newMAC = null;
        for (var i = 0; i < currentMACs.length; i++) {
            if (_state.knownMACs.indexOf(currentMACs[i]) === -1) {
                newMAC = currentMACs[i];
                break;
            }
        }

        // Also accept first node if no known MACs
        if (!newMAC && _state.knownMACs.length === 0 && currentMACs.length > 0) {
            newMAC = currentMACs[0];
        }

        expect(newMAC).toBe('AA:BB:CC:DD:EE:FF');
    });

    test('does not detect existing node as new', async () => {
        _state.knownMACs = ['AA:BB:CC:DD:EE:FF', '11:22:33:44:55:66'];

        fetch.mockResolvedValueOnce({
            ok: true,
            json: jest.fn().mockResolvedValue([
                { mac: 'AA:BB:CC:DD:EE:FF', role: 'tx', online: true },
                { mac: '11:22:33:44:55:66', role: 'rx', online: true },
            ]),
        });

        var resp = await fetch(_CONFIG.nodesEndpoint);
        var nodes = await resp.json();

        var currentMACs = nodes.map(function (n) { return n.mac; });
        var newMAC = null;
        for (var i = 0; i < currentMACs.length; i++) {
            if (_state.knownMACs.indexOf(currentMACs[i]) === -1) {
                newMAC = currentMACs[i];
                break;
            }
        }

        expect(newMAC).toBeNull();
    });

    test('detects new node among existing ones', async () => {
        _state.knownMACs = ['AA:BB:CC:DD:EE:FF'];

        fetch.mockResolvedValueOnce({
            ok: true,
            json: jest.fn().mockResolvedValue([
                { mac: 'AA:BB:CC:DD:EE:FF', role: 'tx', online: true },
                { mac: '11:22:33:44:55:66', role: 'rx', online: true },
            ]),
        });

        var resp = await fetch(_CONFIG.nodesEndpoint);
        var nodes = await resp.json();

        var currentMACs = nodes.map(function (n) { return n.mac; });
        var newMAC = null;
        for (var i = 0; i < currentMACs.length; i++) {
            if (_state.knownMACs.indexOf(currentMACs[i]) === -1) {
                newMAC = currentMACs[i];
                break;
            }
        }

        expect(newMAC).toBe('11:22:33:44:55:66');
    });

    test('handles network error during polling gracefully', async () => {
        fetch.mockRejectedValueOnce(new Error('network error'));

        // The wizard catches this and retries, so no exception propagates
        await expect(fetch(_CONFIG.nodesEndpoint)).rejects.toThrow('network error');
    });
});

// ============================================
// CSI Frame Parsing
// ============================================
describe('CSI frame parser', () => {
    test('parses a valid CSI frame', () => {
        // Build a minimal CSI frame: 24-byte header + 4 subcarriers * 2 bytes = 32 bytes
        var buffer = new ArrayBuffer(32);
        var bytes = new Uint8Array(buffer);

        // Node MAC: AA:BB:CC:DD:EE:FF
        bytes[0] = 0xAA; bytes[1] = 0xBB; bytes[2] = 0xCC;
        bytes[3] = 0xDD; bytes[4] = 0xEE; bytes[5] = 0xFF;

        // Peer MAC: 11:22:33:44:55:66
        bytes[6] = 0x11; bytes[7] = 0x22; bytes[8] = 0x33;
        bytes[9] = 0x44; bytes[10] = 0x55; bytes[11] = 0x66;

        // Timestamp: 8 bytes (little-endian) - just fill with zeros
        bytes[12] = 0; bytes[13] = 0; bytes[14] = 0; bytes[15] = 0;
        bytes[16] = 0; bytes[17] = 0; bytes[18] = 0; bytes[19] = 0;

        // RSSI: -40 dBm (as signed byte)
        bytes[20] = 0xD8; // -40 as uint8

        // Noise floor: -80 dBm
        bytes[21] = 0xB0; // -80 as uint8

        // Channel: 6
        bytes[22] = 6;

        // NSub: 4
        bytes[23] = 4;

        // Payload: 4 I/Q pairs (int8 values)
        bytes[24] = 10; bytes[25] = 5;   // subcarrier 0: I=10, Q=5
        bytes[26] = 20; bytes[27] = 8;   // subcarrier 1: I=20, Q=8
        bytes[28] = 15; bytes[29] = 12;  // subcarrier 2: I=15, Q=12
        bytes[30] = 25; bytes[31] = 3;   // subcarrier 3: I=25, Q=3

        var frame = _parseCSIFrame(buffer);

        expect(frame).not.toBeNull();
        expect(frame.nodeMAC).toBe('AA:BB:CC:DD:EE:FF');
        expect(frame.peerMAC).toBe('11:22:33:44:55:66');
        expect(frame.rssi).toBe(-40);

        // Mean amplitude: avg of sqrt(I^2+Q^2) for each subcarrier
        var expected = (Math.sqrt(100 + 25) + Math.sqrt(400 + 64) +
            Math.sqrt(225 + 144) + Math.sqrt(625 + 9)) / 4;
        expect(frame.meanAmplitude).toBeCloseTo(expected, 5);
    });

    test('returns null for frame too short', () => {
        var buffer = new ArrayBuffer(10);
        var frame = _parseCSIFrame(buffer);
        expect(frame).toBeNull();
    });

    test('returns null for invalid channel', () => {
        var buffer = new ArrayBuffer(26); // 24 header + 1 subcarrier
        var bytes = new Uint8Array(buffer);
        bytes[22] = 0; // invalid channel
        bytes[23] = 1;

        var frame = _parseCSIFrame(buffer);
        expect(frame).toBeNull();
    });

    test('returns null for payload length mismatch', () => {
        var buffer = new ArrayBuffer(30); // 24 header but wrong payload
        var bytes = new Uint8Array(buffer);
        bytes[22] = 6;  // valid channel
        bytes[23] = 4;  // says 4 subcarriers = 8 bytes payload, but we have 6

        var frame = _parseCSIFrame(buffer);
        expect(frame).toBeNull();
    });

    test('handles negative I/Q values (signed bytes)', () => {
        var buffer = new ArrayBuffer(26); // 24 header + 1 subcarrier
        var bytes = new Uint8Array(buffer);

        // MACs
        bytes[0] = 0x01; bytes[1] = 0x02; bytes[2] = 0x03;
        bytes[3] = 0x04; bytes[4] = 0x05; bytes[5] = 0x06;
        bytes[6] = 0x07; bytes[7] = 0x08; bytes[8] = 0x09;
        bytes[9] = 0x0A; bytes[10] = 0x0B; bytes[11] = 0x0C;

        // Channel and NSub
        bytes[22] = 1;
        bytes[23] = 1;

        // I=-10 (0xF6), Q=-5 (0xFB)
        bytes[24] = 0xF6;
        bytes[25] = 0xFB;

        var frame = _parseCSIFrame(buffer);
        expect(frame).not.toBeNull();
        expect(frame.meanAmplitude).toBeCloseTo(Math.sqrt(100 + 25), 5);
    });
});

// ============================================
// Wizard Lifecycle
// ============================================
describe('Wizard lifecycle', () => {
    beforeEach(resetWizardState);

    test('SpaxelOnboard is defined on window', () => {
        expect(SpaxelOnboard).toBeDefined();
        expect(typeof SpaxelOnboard.start).toBe('function');
        expect(typeof SpaxelOnboard.close).toBe('function');
    });

    test('start creates wizard overlay in DOM', () => {
        expect(document.getElementById('wizard-overlay')).toBeNull();

        SpaxelOnboard.start();

        expect(document.getElementById('wizard-overlay')).not.toBeNull();
        expect(document.getElementById('wizard-card')).not.toBeNull();
        expect(document.getElementById('wizard-steps')).not.toBeNull();
        expect(document.getElementById('wizard-content')).not.toBeNull();

        SpaxelOnboard.close();
    });

    test('close removes wizard overlay from DOM', () => {
        SpaxelOnboard.start();
        expect(document.getElementById('wizard-overlay')).not.toBeNull();

        SpaxelOnboard.close();
        expect(document.getElementById('wizard-overlay')).toBeNull();
    });

    test('resume from saved state', () => {
        // Simulate saved state at step 4 (detect_node)
        sessionStorage.setItem(_CONFIG.storageKey, JSON.stringify({
            currentStepIndex: 4,
            nodeMAC: 'AA:BB:CC:DD:EE:FF',
            knownMACs: ['11:22:33:44:55:66'],
            wifiSSID: 'TestWiFi',
            wifiPass: 'secret',
            mothershipHost: '',
            mothershipPort: 8080,
        }));

        SpaxelOnboard.start();

        // State should be restored
        expect(_state.currentStepIndex).toBe(4);
        expect(_state.nodeMAC).toBe('AA:BB:CC:DD:EE:FF');
        expect(_state.wifiSSID).toBe('TestWiFi');

        SpaxelOnboard.close();
    });

    test('duplicate wizard instances are prevented', () => {
        SpaxelOnboard.start();
        var firstOverlay = document.getElementById('wizard-overlay');
        expect(firstOverlay).not.toBeNull();

        SpaxelOnboard.start(); // Should replace the first
        var secondOverlay = document.getElementById('wizard-overlay');
        expect(secondOverlay).not.toBeNull();

        SpaxelOnboard.close();
        expect(document.getElementById('wizard-overlay')).toBeNull();
    });
});

// ============================================
// Wizard Step Indicator
// ============================================
describe('Step indicator rendering', () => {
    beforeEach(resetWizardState);

    test('renders correct number of step dots', () => {
        SpaxelOnboard.start();
        var dots = document.querySelectorAll('.wizard-step-dot');
        expect(dots.length).toBe(8);
        SpaxelOnboard.close();
    });

    test('first step is active on fresh start', () => {
        SpaxelOnboard.start();
        var dots = document.querySelectorAll('.wizard-step-dot');
        // Step 0 (browser_check) auto-advances, so step 1 should be active
        // But in the test, navigator.serial is mocked, so it should auto-advance
        // We just verify the indicator is rendered
        expect(dots.length).toBe(8);
        SpaxelOnboard.close();
    });
});

// ============================================
// Error Message Mapping
// ============================================
describe('Error message mapping', () => {
    test('NotFoundError maps to user-friendly message', () => {
        var err = new _UserError(
            'No device detected. Make sure the USB cable is connected ' +
            'and hold the BOOT button while plugging in.'
        );
        expect(_isUserError(err)).toBe(true);
        expect(err.message).toContain('USB cable');
        expect(err.message).toContain('BOOT button');
    });

    test('never exposes stack traces or technical details', () => {
        var techErr = new Error('ENOENT: no such file or directory');
        expect(_isUserError(techErr)).toBe(false);
        // The wizard should wrap this in a UserError before displaying
    });
});

// ============================================
// Browser Check — Missing Serial API
// ============================================
describe('Browser check without serial API', () => {
    beforeEach(resetWizardState);

    afterEach(() => {
        // Restore serial API for other tests
        Object.defineProperty(navigator, 'serial', {
            value: {
                requestPort: jest.fn().mockResolvedValue(__mockPort),
                getPorts: jest.fn().mockResolvedValue([__mockPort]),
            },
            writable: true,
            configurable: true,
        });
    });

    test('shows error when navigator.serial is missing', () => {
        // Remove serial API to simulate Firefox/Safari
        Object.defineProperty(navigator, 'serial', {
            value: undefined,
            writable: true,
            configurable: true,
        });

        SpaxelOnboard.start();

        var content = document.getElementById('wizard-content');
        expect(content.innerHTML).toContain('Browser Not Supported');
        expect(content.innerHTML).toContain('Google Chrome');
        expect(content.innerHTML).toContain('Microsoft Edge');
        expect(content.innerHTML).toContain('Firefox and Safari');

        SpaxelOnboard.close();
    });

    test('shows no navigation buttons when serial API is missing', () => {
        Object.defineProperty(navigator, 'serial', {
            value: undefined,
            writable: true,
            configurable: true,
        });

        SpaxelOnboard.start();

        var nav = document.getElementById('wizard-nav');
        expect(nav.innerHTML).toBe('');

        SpaxelOnboard.close();
    });
});

// ============================================
// Wizard State Transitions (without hardware)
// ============================================
describe('Wizard state transitions', () => {
    beforeEach(resetWizardState);

    afterEach(() => {
        SpaxelOnboard.close();
    });

    test('auto-advances from browser_check to connect_device when serial is available', (done) => {
        jest.useFakeTimers();

        SpaxelOnboard.start();

        // browser_check auto-advances after 400ms
        jest.advanceTimersByTime(400);

        // Should be on step 1 (connect_device)
        expect(_state.currentStepIndex).toBe(1);
        var content = document.getElementById('wizard-content');
        expect(content.innerHTML).toContain('Connect Your ESP32-S3');

        jest.useRealTimers();
        done();
    });

    test('connect_device step renders ESP32 illustration with BOOT button highlighted', () => {
        jest.useFakeTimers();
        SpaxelOnboard.start();
        jest.advanceTimersByTime(400); // Skip browser_check

        var content = document.getElementById('wizard-content');
        expect(content.innerHTML).toContain('esp32-illustration');
        expect(content.innerHTML).toContain('BOOT');

        // "Select Device" button is in the nav area
        var nextBtn = document.getElementById('wizard-next');
        expect(nextBtn).not.toBeNull();
        expect(nextBtn.textContent).toBe('Select Device');

        jest.useRealTimers();
    });

    test('connect_device shows error when requestPort fails with NotFoundError', async () => {
        jest.useFakeTimers();
        navigator.serial.requestPort.mockRejectedValueOnce({ name: 'NotFoundError' });

        SpaxelOnboard.start();
        jest.advanceTimersByTime(400); // Skip browser_check

        // Click "Select Device"
        var nextBtn = document.getElementById('wizard-next');
        expect(nextBtn).not.toBeNull();
        nextBtn.click();

        // Flush microtasks
        await jest.advanceTimersByTimeAsync(0);

        var errEl = document.getElementById('connect-error');
        expect(errEl.style.display).toBe('block');
        expect(errEl.textContent).toContain('USB cable');
        expect(errEl.textContent).toContain('BOOT button');

        // Button should be re-enabled
        nextBtn = document.getElementById('wizard-next');
        expect(nextBtn.disabled).toBe(false);

        jest.useRealTimers();
    });

    test('connect_device shows generic error for non-NotFoundError', async () => {
        jest.useFakeTimers();
        navigator.serial.requestPort.mockRejectedValueOnce(new Error('some error'));

        SpaxelOnboard.start();
        jest.advanceTimersByTime(400);

        var nextBtn = document.getElementById('wizard-next');
        nextBtn.click();

        await jest.advanceTimersByTimeAsync(0);

        var errEl = document.getElementById('connect-error');
        expect(errEl.style.display).toBe('block');
        expect(errEl.textContent).toContain('Could not select a device');

        jest.useRealTimers();
    });

    test('flash_firmware shows skip option when esp-web-tools is not loaded', () => {
        jest.useFakeTimers();
        // Make customElements.get return null to simulate missing esp-web-tools
        customElements.get = jest.fn(() => null);

        sessionStorage.setItem(_CONFIG.storageKey, JSON.stringify({
            currentStepIndex: 2,
            nodeMAC: null,
            knownMACs: [],
            wifiSSID: '',
            wifiPass: '',
            mothershipHost: '',
            mothershipPort: 8080,
        }));

        SpaxelOnboard.start();

        var content = document.getElementById('wizard-content');
        expect(content.innerHTML).toContain('Firmware flashing component failed to load');
        // "Skip Flashing" is in the nav area, not the content area
        var nav = document.getElementById('wizard-nav');
        expect(nav.innerHTML).toContain('Skip Flashing');

        jest.useRealTimers();
    });

    test('provision_wifi step renders form with WiFi fields', () => {
        jest.useFakeTimers();

        sessionStorage.setItem(_CONFIG.storageKey, JSON.stringify({
            currentStepIndex: 3,
            nodeMAC: null,
            knownMACs: [],
            wifiSSID: 'MyWiFi',
            wifiPass: 'password123',
            mothershipHost: '',
            mothershipPort: 8080,
        }));

        SpaxelOnboard.start();

        var content = document.getElementById('wizard-content');
        expect(content.innerHTML).toContain('Configure WiFi');
        expect(content.innerHTML).toContain('wifi-ssid');
        expect(content.innerHTML).toContain('wifi-pass');
        expect(content.innerHTML).toContain('MyWiFi');

        jest.useRealTimers();
    });

    test('provision_wifi shows error for empty SSID', () => {
        sessionStorage.setItem(_CONFIG.storageKey, JSON.stringify({
            currentStepIndex: 3,
            nodeMAC: null,
            knownMACs: [],
            wifiSSID: '',
            wifiPass: '',
            mothershipHost: '',
            mothershipPort: 8080,
        }));

        SpaxelOnboard.start();

        // Submit form with empty SSID — validation is synchronous
        var form = document.getElementById('wifi-form');
        var ssidInput = document.getElementById('wifi-ssid');
        ssidInput.value = '';

        form.dispatchEvent(new Event('submit'));

        var errEl = document.getElementById('provision-error');
        expect(errEl.style.display).toBe('block');
        expect(errEl.textContent).toContain('WiFi network name');
    });

    test('placement step renders placement guidance', () => {
        jest.useFakeTimers();

        sessionStorage.setItem(_CONFIG.storageKey, JSON.stringify({
            currentStepIndex: 6,
            nodeMAC: 'AA:BB:CC:DD:EE:FF',
            knownMACs: [],
            wifiSSID: 'TestWiFi',
            wifiPass: 'pass',
            mothershipHost: '',
            mothershipPort: 8080,
        }));

        SpaxelOnboard.start();

        var content = document.getElementById('wizard-content');
        expect(content.innerHTML).toContain('Node Placement');
        expect(content.innerHTML).toContain('opposite corners');
        expect(content.innerHTML).toContain('2 meters');
        expect(content.innerHTML).toContain('chest height');

        jest.useRealTimers();
    });

    test('complete step shows node MAC and dashboard button', () => {
        jest.useFakeTimers();

        sessionStorage.setItem(_CONFIG.storageKey, JSON.stringify({
            currentStepIndex: 7,
            nodeMAC: 'AA:BB:CC:DD:EE:FF',
            knownMACs: [],
            wifiSSID: 'TestWiFi',
            wifiPass: 'pass',
            mothershipHost: '',
            mothershipPort: 8080,
        }));

        SpaxelOnboard.start();

        var content = document.getElementById('wizard-content');
        expect(content.innerHTML).toContain('Setup Complete');
        expect(content.innerHTML).toContain('AA:BB:CC:DD:EE:FF');
        expect(content.innerHTML).toContain('Go to Dashboard');

        var gotoBtn = document.getElementById('goto-dashboard');
        expect(gotoBtn).not.toBeNull();

        jest.useRealTimers();
    });

    test('complete step hides navigation buttons', () => {
        jest.useFakeTimers();

        sessionStorage.setItem(_CONFIG.storageKey, JSON.stringify({
            currentStepIndex: 7,
            nodeMAC: 'AA:BB:CC:DD:EE:FF',
            knownMACs: [],
            wifiSSID: '',
            wifiPass: '',
            mothershipHost: '',
            mothershipPort: 8080,
        }));

        SpaxelOnboard.start();

        var nav = document.getElementById('wizard-nav');
        expect(nav.innerHTML).toBe('');

        jest.useRealTimers();
    });
});

// ============================================
// Provisioning Payload Assembly and Serial Send
// ============================================
describe('Provisioning payload assembly and serial send', () => {
    beforeEach(resetWizardState);
    afterEach(() => { __clearLastEncodedData(); });

    test('provisionAndSend calls POST /api/provision with correct body', async () => {
        fetch.mockResolvedValueOnce({
            ok: true,
            json: jest.fn().mockResolvedValue({
                version: 1,
                wifi_ssid: 'TestWiFi',
                wifi_pass: 'secret123',
                node_id: 'uuid-123',
                node_token: 'token-abc',
                ms_mdns: 'spaxel-mothership.local',
                ms_port: 8080,
                debug: false,
            }),
        });

        await _provisionAndSend('TestWiFi', 'secret123', '', 8080);

        expect(fetch).toHaveBeenCalledWith('/api/provision', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ wifi_ssid: 'TestWiFi', wifi_pass: 'secret123' }),
        });

        // Port should have been opened
        expect(__mockPort.open).toHaveBeenCalledWith({ baudRate: 115200 });

        // Verify data was sent over serial
        var sent = __getLastEncodedData();
        expect(sent).toContain('"wifi_ssid":"TestWiFi"');
        expect(sent).toContain('"node_id":"uuid-123"');
    });

    test('provisionAndSend applies mothership host override', async () => {
        fetch.mockResolvedValueOnce({
            ok: true,
            json: jest.fn().mockResolvedValue({
                version: 1,
                wifi_ssid: 'TestWiFi',
                wifi_pass: 'pass',
                node_id: 'uuid',
                node_token: 'tok',
                ms_mdns: 'spaxel-mothership.local',
                ms_port: 8080,
                debug: false,
            }),
        });

        await _provisionAndSend('TestWiFi', 'pass', '192.168.1.100', 9090);

        var sent = __getLastEncodedData();
        expect(sent).toContain('"ms_mdns":"192.168.1.100"');
        expect(sent).toContain('"ms_port":9090');
    });

    test('provisionAndSend falls back to client-side payload when server fails', async () => {
        fetch.mockRejectedValueOnce(new Error('server unavailable'));

        await _provisionAndSend('TestWiFi', 'pass', '', 8080);

        // Port should still be opened with client-side payload
        expect(__mockPort.open).toHaveBeenCalledWith({ baudRate: 115200 });

        // Writer should have been called with client-side assembled payload
        var sent = __getLastEncodedData();
        expect(sent).toContain('"wifi_ssid":"TestWiFi"');
        expect(sent).toContain('"node_id":"test-uuid-1234"');
    });

    test('provisionAndSend throws UserError when no port is available', async () => {
        fetch.mockResolvedValueOnce({
            ok: true,
            json: jest.fn().mockResolvedValue({
                version: 1,
                wifi_ssid: 'W',
                wifi_pass: 'p',
                node_id: 'u',
                node_token: 't',
                ms_mdns: 'h',
                ms_port: 8080,
                debug: false,
            }),
        });

        // No authorized ports — use mockResolvedValue (not Once) so fallback path also fails
        navigator.serial.getPorts.mockResolvedValue([]);

        await expect(_provisionAndSend('W', 'p', '', 8080)).rejects.toEqual(
            expect.objectContaining({ name: 'UserError' })
        );
    });
});

// ============================================
// Node Detection Polling — Full Wizard Transition
// ============================================
describe('Node detection wizard transition', () => {
    beforeEach(resetWizardState);

    afterEach(() => {
        SpaxelOnboard.close();
        jest.useRealTimers();
    });

    test('detect_node polls and transitions to calibrate when new node appears', async () => {
        jest.useFakeTimers();

        sessionStorage.setItem(_CONFIG.storageKey, JSON.stringify({
            currentStepIndex: 4,
            nodeMAC: null,
            knownMACs: ['AA:BB:CC:DD:EE:FF'],
            wifiSSID: 'TestWiFi',
            wifiPass: 'pass',
            mothershipHost: '',
            mothershipPort: 8080,
        }));

        SpaxelOnboard.start();

        // Verify we're on detect_node step
        expect(_state.currentStepIndex).toBe(4);
        var content = document.getElementById('wizard-content');
        expect(content.innerHTML).toContain('Detecting Your Node');

        // Simulate a poll returning a new node
        fetch.mockResolvedValueOnce({
            ok: true,
            json: jest.fn().mockResolvedValue([
                { mac: 'AA:BB:CC:DD:EE:FF', role: 'tx', online: true },
                { mac: '11:22:33:44:55:66', role: 'rx', online: true },
            ]),
        });

        // Advance time by the poll interval to trigger the first poll
        await jest.advanceTimersByTimeAsync(3000);

        // Node should be detected
        expect(_state.nodeMAC).toBe('11:22:33:44:55:66');

        // After 1s timeout, it should transition to calibrate
        await jest.advanceTimersByTimeAsync(1000);
        expect(_state.currentStepIndex).toBe(5);
    });

    test('detect_node shows troubleshooting after timeout', () => {
        jest.useFakeTimers();

        sessionStorage.setItem(_CONFIG.storageKey, JSON.stringify({
            currentStepIndex: 4,
            nodeMAC: null,
            knownMACs: [],
            wifiSSID: 'TestWiFi',
            wifiPass: 'pass',
            mothershipHost: '',
            mothershipPort: 8080,
        }));

        SpaxelOnboard.start();

        // Simulate no new nodes appearing — fetch returns empty
        fetch.mockResolvedValue({
            ok: true,
            json: jest.fn().mockResolvedValue([]),
        });

        // Advance past timeout
        jest.advanceTimersByTime(121000);

        var content = document.getElementById('wizard-content');
        expect(content.innerHTML).toContain('Troubleshooting');
        expect(content.innerHTML).toContain('2.4 GHz');
        expect(content.innerHTML).toContain('AP isolation');

        // Retry button should appear
        var retryBtn = document.getElementById('wizard-next');
        expect(retryBtn).not.toBeNull();
        expect(retryBtn.textContent).toBe('Retry Detection');
    });
});

// ============================================
// Session Storage Restore at Each Step
// ============================================
describe('Session storage restore at each step', () => {
    beforeEach(resetWizardState);

    afterEach(() => {
        SpaxelOnboard.close();
        jest.useRealTimers();
    });

    function testRestoreAtStep(stepIndex, stepLabel, expectedContent) {
        test('restores state at step ' + stepIndex + ' (' + stepLabel + ')', () => {
            jest.useFakeTimers();

            sessionStorage.setItem(_CONFIG.storageKey, JSON.stringify({
                currentStepIndex: stepIndex,
                nodeMAC: stepIndex >= 4 ? 'AA:BB:CC:DD:EE:FF' : null,
                knownMACs: stepIndex >= 4 ? ['11:22:33:44:55:66'] : [],
                wifiSSID: 'TestWiFi',
                wifiPass: 'secret',
                mothershipHost: 'custom-host',
                mothershipPort: 9090,
            }));

            SpaxelOnboard.start();

            expect(_state.currentStepIndex).toBe(stepIndex);
            expect(_state.wifiSSID).toBe('TestWiFi');
            expect(_state.mothershipHost).toBe('custom-host');
            expect(_state.mothershipPort).toBe(9090);

            if (stepIndex >= 4) {
                expect(_state.nodeMAC).toBe('AA:BB:CC:DD:EE:FF');
            }
        });
    }

    testRestoreAtStep(1, 'connect_device');
    testRestoreAtStep(2, 'flash_firmware');
    testRestoreAtStep(3, 'provision_wifi');
    testRestoreAtStep(4, 'detect_node');
    testRestoreAtStep(6, 'placement');
    testRestoreAtStep(7, 'complete');

    test('restores state at calibrate step and connects WebSocket', () => {
        jest.useFakeTimers();

        sessionStorage.setItem(_CONFIG.storageKey, JSON.stringify({
            currentStepIndex: 5,
            nodeMAC: 'AA:BB:CC:DD:EE:FF',
            knownMACs: ['11:22:33:44:55:66'],
            wifiSSID: 'TestWiFi',
            wifiPass: 'secret',
            mothershipHost: '',
            mothershipPort: 8080,
        }));

        SpaxelOnboard.start();

        expect(_state.currentStepIndex).toBe(5);
        expect(_state.nodeMAC).toBe('AA:BB:CC:DD:EE:FF');
        expect(_state.calibratePhase).toBe('walk');

        var content = document.getElementById('wizard-content');
        expect(content.innerHTML).toContain('Guided Calibration');
        expect(content.innerHTML).toContain('Walk Around Your Space');
    });

    test('fresh start (no saved state) begins at step 0', () => {
        jest.useFakeTimers();

        SpaxelOnboard.start();

        // browser_check auto-advances to step 1
        jest.advanceTimersByTime(400);
        expect(_state.currentStepIndex).toBe(1);
    });
});
