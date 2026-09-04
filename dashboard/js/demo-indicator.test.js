/**
 * Tests for demo-indicator.js
 *
 * The indicator must render a "Demo Mode" badge only while the server reports
 * demo_mode, and must leave the DOM untouched otherwise (including on a
 * failed status fetch).
 */

function statusResponse(demoMode) {
    return Promise.resolve({
        ok: true,
        json: function () { return Promise.resolve({ pin_configured: true, demo_mode: demoMode }); }
    });
}

function freshModule() {
    jest.resetModules();
    require('../js/demo-indicator.js');
    return window.SpaxelDemoIndicator;
}

// The module auto-checks on load; let that pass settle before driving check().
function flush() {
    return new Promise(function (resolve) { setTimeout(resolve, 0); });
}

beforeEach(function () {
    document.body.innerHTML = '';
    delete document.getElementById('demo-mode-banner-style');
    var head = document.head;
    while (head.firstChild) head.removeChild(head.firstChild);
});

afterEach(function () {
    document.body.innerHTML = '';
});

describe('demo mode indicator', function () {
    test('renders the badge when demo_mode is true', async function () {
        global.fetch = jest.fn().mockReturnValue(statusResponse(true));

        const indicator = freshModule();
        const result = await indicator.check();

        expect(result).toBe(true);
        expect(indicator.isDemoMode()).toBe(true);
        const badge = document.getElementById('demo-mode-banner');
        expect(badge).not.toBeNull();
        expect(badge.textContent).toBe('Demo Mode');
        expect(badge.className).toBe('demo-mode-banner');
        expect(global.fetch).toHaveBeenCalledWith('/api/auth/status');
    });

    test('renders nothing when demo_mode is false', async function () {
        global.fetch = jest.fn().mockReturnValue(statusResponse(false));

        const indicator = freshModule();
        const result = await indicator.check();

        expect(result).toBe(false);
        expect(document.getElementById('demo-mode-banner')).toBeNull();
    });

    test('renders nothing when the status endpoint is unreachable', async function () {
        global.fetch = jest.fn().mockRejectedValue(new Error('down'));

        const indicator = freshModule();
        const result = await indicator.check();

        expect(result).toBe(false);
        expect(indicator.isDemoMode()).toBe(false);
        expect(document.getElementById('demo-mode-banner')).toBeNull();
    });

    test('removes a previously rendered badge when demo mode turns off', async function () {
        global.fetch = jest.fn()
            .mockReturnValueOnce(statusResponse(true))
            .mockReturnValueOnce(statusResponse(false));

        const indicator = freshModule();
        await flush();
        expect(document.getElementById('demo-mode-banner')).not.toBeNull();

        await indicator.check();
        expect(document.getElementById('demo-mode-banner')).toBeNull();
    });
});
