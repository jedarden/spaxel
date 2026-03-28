/**
 * Jest setup for onboard tests.
 * Mocks Web Serial API, fetch, WebSocket, and sessionStorage.
 */

// Mock TextEncoderStream (not available in jsdom)
global.TextEncoderStream = class TextEncoderStream {
    readable = {};
    writable = {};
};

// Mock ReadableStream/WritableStream (not available in jsdom)
global.ReadableStream = class ReadableStream {};
global.WritableStream = class WritableStream {};

// Mock navigator.serial
const mockPort = {
    open: jest.fn().mockResolvedValue(undefined),
    close: jest.fn().mockResolvedValue(undefined),
    readable: {},
    writable: {
        getWriter: jest.fn().mockReturnValue({
            write: jest.fn().mockResolvedValue(undefined),
            close: jest.fn().mockResolvedValue(undefined),
            releaseLock: jest.fn(),
        }),
    },
};

Object.defineProperty(navigator, 'serial', {
    value: {
        requestPort: jest.fn().mockResolvedValue(mockPort),
        getPorts: jest.fn().mockResolvedValue([mockPort]),
    },
    writable: true,
    configurable: true,
});

// Mock fetch
global.fetch = jest.fn().mockResolvedValue({
    ok: true,
    json: jest.fn().mockResolvedValue([]),
});

// Mock WebSocket
global.WebSocket = jest.fn().mockImplementation(function () {
    this.binaryType = 'arraybuffer';
    this.close = jest.fn();
    this.send = jest.fn();
    this.readyState = 1;
    this.onopen = null;
    this.onclose = null;
    this.onerror = null;
    this.onmessage = null;
});

// Mock crypto.randomUUID
Object.defineProperty(global, 'crypto', {
    value: {
        randomUUID: jest.fn().mockReturnValue('test-uuid-1234'),
    },
});

// Mock customElements
global.customElements = {
    get: jest.fn().mockReturnValue(null),
    define: jest.fn(),
};

// Mock sessionStorage
var storage = {};
global.sessionStorage = {
    getItem: jest.fn(function (key) { return storage[key] || null; }),
    setItem: jest.fn(function (key, val) { storage[key] = val; }),
    removeItem: jest.fn(function (key) { delete storage[key]; }),
    clear: jest.fn(function () { storage = {}; }),
};

// Export mock port for tests
global.__mockPort = mockPort;
