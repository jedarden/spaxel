/**
 * Tests for auth.js demo-mode behavior.
 *
 * In demo mode the server admits every dashboard request without a PIN, so the
 * client must never render the PIN setup or login overlay, and must report the
 * dashboard as usable. Without demo mode the existing gating is unchanged.
 *
 * auth.js is an IIFE that auto-initializes on load, so each case re-requires
 * the module against a stubbed fetch and asserts on the resulting DOM.
 */

function jsonResponse(body) {
    return Promise.resolve({
        ok: true,
        json: function () { return Promise.resolve(body); }
    });
}

// fetch stub covering the two requests auth.js can make during init:
// /api/auth/status (always) and /api/settings HEAD (session probe).
function stubFetch(statusBody, sessionValid) {
    return jest.fn(function (url, opts) {
        if (url === '/api/auth/status') return jsonResponse(statusBody);
        if (url === '/api/settings') {
            return Promise.resolve({ ok: !!sessionValid, status: sessionValid ? 200 : 401 });
        }
        throw new Error('unexpected fetch: ' + url);
    });
}

function loadAuth(statusBody, sessionValid) {
    global.fetch = stubFetch(statusBody, sessionValid);
    jest.resetModules();
    require('../js/auth.js');
    return window.SpaxelAuth;
}

function cleanupDom() {
    document.body.innerHTML = '';
}

beforeEach(cleanupDom);
afterEach(cleanupDom);

describe('auth demo-mode gating', function () {
    test('demo mode with a configured PIN renders no auth overlay', async function () {
        const auth = loadAuth({ pin_configured: true, demo_mode: true }, true);
        const usable = await auth.checkStatus();

        expect(usable).toBe(true);
        expect(auth.isDemoMode()).toBe(true);
        expect(document.getElementById('auth-overlay')).toBeNull();
    });

    test('demo mode with no PIN configured renders no setup overlay', async function () {
        const auth = loadAuth({ pin_configured: false, demo_mode: true }, false);
        const usable = await auth.checkStatus();

        expect(usable).toBe(true);
        expect(document.getElementById('auth-overlay')).toBeNull();
    });

    test('without demo mode a configured PIN still shows the login overlay', async function () {
        const auth = loadAuth({ pin_configured: true, demo_mode: false }, false);
        const usable = await auth.checkStatus();

        expect(usable).toBe(false);
        expect(auth.isDemoMode()).toBe(false);

        const overlay = document.getElementById('auth-overlay');
        expect(overlay).not.toBeNull();
        expect(overlay.textContent).toContain('Enter your PIN to continue');
    });

    test('without demo mode an unconfigured PIN still shows first-run setup', async function () {
        const auth = loadAuth({ pin_configured: false, demo_mode: false }, false);
        const usable = await auth.checkStatus();

        expect(usable).toBe(true);

        const overlay = document.getElementById('auth-overlay');
        expect(overlay).not.toBeNull();
        expect(overlay.textContent).toContain("Let's secure your dashboard with a PIN");
    });

    test('without demo mode a valid session renders no overlay', async function () {
        const auth = loadAuth({ pin_configured: true, demo_mode: false }, true);
        const usable = await auth.checkStatus();

        expect(usable).toBe(true);
        expect(document.getElementById('auth-overlay')).toBeNull();
    });
});
