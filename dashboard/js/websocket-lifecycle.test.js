/**
 * Isolated WebSocket lifecycle pattern tests.
 *
 * Reproduces the connection lifecycle patterns implemented by the dashboard's
 * WebSocket reconnection manager (dashboard/js/websocket.js) — the pattern set
 * docs/notes/leak-sources-catalog.md classifies as "properly managed" — one
 * pattern per test:
 *
 *   connect     WS-CONNECT-1/2      connect() :49-103, double-connect guard :50-52
 *   reconnect   WS-RECONNECT-1/2/3  _backoffMs :42-46, _scheduleReconnect :126-136,
 *                                   attempt reset on open :71
 *   message     WS-MSG-1/2/3        onmessage routing :100-102, unset-handler
 *                                   guard :347-349, send gating :397-401
 *   disconnect  WS-DISCONNECT-1/2/3 disconnect() :105-124, onclose detach :113,
 *                                   duration tracking :392-394
 *
 * Isolation contract:
 *   - Every test gets a fresh module registry (jest.resetModules) so the
 *     manager's closure state never carries between tests.
 *   - The transport is a FakeWebSocket installed on global.WebSocket — no
 *     network. It carries the real OPEN/CONNECTING constants the manager
 *     reads (:50, :398), which the shared onboard.test.setup.js mock lacks.
 *   - Tests that need timers use fake timers but NEVER await real time while
 *     they are installed — the hang mode documented for
 *     onboard.leak-isolation.test.js (ISOLATION-1/8/9). afterEach restores
 *     real timers unconditionally so an aborting test cannot leak fake timers
 *     into the next one.
 *
 * Run standalone:   cd dashboard && npx jest js/websocket-lifecycle.test.js
 * Run one pattern:  cd dashboard && npx jest js/websocket-lifecycle.test.js -t WS-RECONNECT-2
 */

class FakeWebSocket {
    constructor(url) {
        this.url = url;
        this.readyState = FakeWebSocket.CONNECTING;
        this.binaryType = '';
        this.sent = [];
        this.closeCalled = false;
        this.onopen = null;
        this.onclose = null;
        this.onerror = null;
        this.onmessage = null;
        FakeWebSocket.instances.push(this);
    }

    send(data) {
        this.sent.push(data);
    }

    close() {
        this.closeCalled = true;
        this.readyState = FakeWebSocket.CLOSED;
    }

    // ── Test-side transport drivers ─────────────────────────────────────────
    __serverOpen() {
        this.readyState = FakeWebSocket.OPEN;
        if (this.onopen) this.onopen({ type: 'open' });
    }

    __serverMessage(data) {
        if (this.onmessage) this.onmessage({ type: 'message', data: data });
    }

    __serverClose(code, reason) {
        this.readyState = FakeWebSocket.CLOSED;
        if (this.onclose) {
            this.onclose({ type: 'close', code: code || 1006, reason: reason || '' });
        }
    }
}

FakeWebSocket.CONNECTING = 0;
FakeWebSocket.OPEN = 1;
FakeWebSocket.CLOSING = 2;
FakeWebSocket.CLOSED = 3;
FakeWebSocket.instances = [];

const lastSocket = () => FakeWebSocket.instances[FakeWebSocket.instances.length - 1];

describe('WebSocket lifecycle patterns (websocket.js)', () => {
    let wsMgr;

    beforeEach(() => {
        jest.resetModules();
        FakeWebSocket.instances.length = 0;
        global.WebSocket = FakeWebSocket;
        // Zero jitter so backoff delays are exact (_backoffMs :42-46).
        jest.spyOn(Math, 'random').mockReturnValue(0.5);
        jest.spyOn(console, 'log').mockImplementation(() => {});
        jest.spyOn(console, 'error').mockImplementation(() => {});
        require('./websocket.js');
        wsMgr = window.SpaxelWebSocket;
    });

    afterEach(() => {
        // Stops the manager's reconnect timer, disconnect-state interval and
        // extrapolation loop before the timer mode flips back.
        if (wsMgr) wsMgr.disconnect();
        // Safety net against the ISOLATION-8 cascade: an aborting test must
        // not leave fake timers installed for the next one.
        jest.useRealTimers();
        jest.restoreAllMocks();
    });

    // ── Connect ─────────────────────────────────────────────────────────────

    test('WS-CONNECT-1: connect creates one CONNECTING socket that becomes connected on open', () => {
        const onOpen = jest.fn();
        wsMgr.init({ onOpen: onOpen });

        wsMgr.connect('ws://test.local/ws/dashboard');

        expect(FakeWebSocket.instances).toHaveLength(1);
        expect(lastSocket().readyState).toBe(FakeWebSocket.CONNECTING);
        expect(lastSocket().binaryType).toBe('arraybuffer');
        expect(wsMgr.isConnecting()).toBe(true);
        expect(wsMgr.isConnected()).toBe(false);

        lastSocket().__serverOpen();

        expect(onOpen).toHaveBeenCalledTimes(1);
        expect(onOpen.mock.calls[0][0]).toBe(lastSocket());
        expect(wsMgr.isConnected()).toBe(true);
        expect(wsMgr.isConnecting()).toBe(false);
    });

    test('WS-CONNECT-2: connect while CONNECTING or OPEN never opens a second socket', () => {
        wsMgr.connect('ws://test.local/ws/dashboard');
        wsMgr.connect('ws://test.local/ws/dashboard'); // still handshaking
        expect(FakeWebSocket.instances).toHaveLength(1);

        lastSocket().__serverOpen();
        wsMgr.connect('ws://test.local/ws/dashboard'); // already open
        expect(FakeWebSocket.instances).toHaveLength(1);
    });

    // ── Reconnect cycles ────────────────────────────────────────────────────

    test('WS-RECONNECT-1: an unexpected close schedules exactly one reconnect at base backoff', () => {
        jest.useFakeTimers();
        wsMgr.connect('ws://test.local/ws/dashboard');
        lastSocket().__serverOpen();
        lastSocket().__serverClose();

        expect(wsMgr.isConnected()).toBe(false);
        // Reconnect timeout (:130) + disconnect-state interval (:143).
        expect(jest.getTimerCount()).toBe(2);

        // Base backoff is BACKOFF_BASE_MS = 1000 (:11); jitter is mocked to zero.
        jest.advanceTimersByTime(999);
        expect(FakeWebSocket.instances).toHaveLength(1);

        jest.advanceTimersByTime(1);
        expect(FakeWebSocket.instances).toHaveLength(2);
        expect(lastSocket().readyState).toBe(FakeWebSocket.CONNECTING);
        expect(lastSocket().url).toBe(
            (window.location.protocol === 'https:' ? 'wss:' : 'ws:') +
                '//' + window.location.host + '/ws/dashboard'
        );
    });

    test('WS-RECONNECT-2: backoff doubles per failed attempt and caps at 10s', () => {
        jest.useFakeTimers();
        wsMgr.connect('ws://test.local/ws/dashboard');

        // Each cycle is a failed handshake: the reconnect socket is closed
        // before it ever opens, so the attempt counter keeps climbing (:132).
        // 1000 * 2^attempt, capped at BACKOFF_MAX_MS = 10000 (:12).
        const expectedDelays = [1000, 2000, 4000, 8000, 10000, 10000];
        expectedDelays.forEach((expectedDelay, attempt) => {
            lastSocket().__serverClose();

            jest.advanceTimersByTime(expectedDelay - 1);
            expect(FakeWebSocket.instances).toHaveLength(attempt + 1);

            jest.advanceTimersByTime(1);
            expect(FakeWebSocket.instances).toHaveLength(attempt + 2);
        });
    });

    test('WS-RECONNECT-3: a successful reconnect resets backoff to the base delay', () => {
        jest.useFakeTimers();
        wsMgr.connect('ws://test.local/ws/dashboard');
        lastSocket().__serverOpen();

        // First drop: base backoff.
        lastSocket().__serverClose();
        jest.advanceTimersByTime(999);
        expect(FakeWebSocket.instances).toHaveLength(1);
        jest.advanceTimersByTime(1);
        expect(FakeWebSocket.instances).toHaveLength(2);

        // Reconnect succeeds — attempt counter resets to 0 (:71).
        lastSocket().__serverOpen();

        // The next drop backs off from the base again instead of climbing on.
        lastSocket().__serverClose();
        jest.advanceTimersByTime(999);
        expect(FakeWebSocket.instances).toHaveLength(2);
        jest.advanceTimersByTime(1);
        expect(FakeWebSocket.instances).toHaveLength(3);
    });

    // ── Message handlers ────────────────────────────────────────────────────

    test('WS-MSG-1: every message reaches the registered handler as raw data', () => {
        const onMessage = jest.fn();
        wsMgr.init({ onMessage: onMessage });
        wsMgr.connect('ws://test.local/ws/dashboard');
        lastSocket().__serverOpen();

        lastSocket().__serverMessage('{"type":"snapshot","blobs":[]}');
        lastSocket().__serverMessage('{"type":"status","ok":true}');

        expect(onMessage).toHaveBeenCalledTimes(2);
        expect(onMessage.mock.calls[0][0]).toBe('{"type":"snapshot","blobs":[]}');
        expect(onMessage.mock.calls[1][0]).toBe('{"type":"status","ok":true}');
    });

    test('WS-MSG-2: a message with no handler registered is a no-op', () => {
        wsMgr.connect('ws://test.local/ws/dashboard');
        lastSocket().__serverOpen();

        // Unset-handler guard (:347-349).
        expect(() => lastSocket().__serverMessage('{"type":"snapshot"}')).not.toThrow();
    });

    test('WS-MSG-3: send only delivers while the socket is OPEN', () => {
        wsMgr.connect('ws://test.local/ws/dashboard');
        const socket = lastSocket();

        wsMgr.send('while-connecting'); // CONNECTING — dropped (:398)
        expect(socket.sent).toEqual([]);

        socket.__serverOpen();
        wsMgr.send('while-open');
        expect(socket.sent).toEqual(['while-open']);

        socket.__serverClose(); // manager nulls its socket in onclose (:82)
        wsMgr.send('after-close');
        expect(socket.sent).toEqual(['while-open']);
    });

    // ── Disconnect ──────────────────────────────────────────────────────────

    test('WS-DISCONNECT-1: an intentional disconnect closes the socket and suppresses reconnect', () => {
        jest.useFakeTimers();
        wsMgr.connect('ws://test.local/ws/dashboard');
        const socket = lastSocket();
        socket.__serverOpen();

        wsMgr.disconnect();

        // onclose is detached BEFORE close() so the local close cannot run the
        // reconnect path (:113).
        expect(socket.onclose).toBeNull();
        expect(socket.closeCalled).toBe(true);
        expect(wsMgr.isConnected()).toBe(false);
        expect(wsMgr.getDisconnectDurationMs()).toBe(0);

        // No reconnect is ever scheduled for an intentional close.
        jest.advanceTimersByTime(20000);
        expect(FakeWebSocket.instances).toHaveLength(1);
    });

    test('WS-DISCONNECT-2: disconnect after an unexpected drop cancels the pending reconnect', () => {
        jest.useFakeTimers();
        wsMgr.connect('ws://test.local/ws/dashboard');
        lastSocket().__serverOpen();
        lastSocket().__serverClose();

        // Reconnect timeout + disconnect-state interval are both pending.
        expect(jest.getTimerCount()).toBe(2);

        wsMgr.disconnect();

        expect(jest.getTimerCount()).toBe(0);
        jest.advanceTimersByTime(20000);
        expect(FakeWebSocket.instances).toHaveLength(1);
    });

    test('WS-DISCONNECT-3: disconnect duration tracks wall-clock time since the drop', async () => {
        // Real timers on purpose: this pattern is about wall-clock time, and
        // the sibling isolation suite showed that fake timers must never be
        // mixed with an awaited real sleep.
        expect(wsMgr.getDisconnectDurationMs()).toBe(0);

        wsMgr.connect('ws://test.local/ws/dashboard');
        lastSocket().__serverOpen();
        lastSocket().__serverClose();

        await new Promise(resolve => setTimeout(resolve, 40));

        const duration = wsMgr.getDisconnectDurationMs();
        expect(duration).toBeGreaterThanOrEqual(35);
        expect(duration).toBeLessThan(60000);

        // The snapshot handler reports the reconnection, clearing the clock.
        wsMgr.onReconnected();
        expect(wsMgr.getDisconnectDurationMs()).toBe(0);
    });
});
