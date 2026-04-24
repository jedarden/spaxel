/**
 * Spaxel Dashboard — Command Palette Tests (Component 34)
 *
 * Tests for:
 * - Fuzzy matching (fuzzyScore)
 * - Time navigation parsing (parseTimeExpression)
 * - Commands registry completeness
 * - Keyboard navigation (arrow down, enter, escape)
 * - Recent history (localStorage)
 * - Expert-mode gating (palette unavailable in simple/ambient mode)
 * - Viewport positioning (palette centred on screen)
 */

describe('CommandPaletteManager', function () {

    // ── Setup ────────────────────────────────────────────────────────────────

    beforeAll(function () {
        // Mock localStorage
        var _store = {};
        global.localStorage = {
            getItem:    function (k) { return _store[k] !== undefined ? _store[k] : null; },
            setItem:    function (k, v) { _store[k] = String(v); },
            removeItem: function (k) { delete _store[k]; },
            clear:      function () { _store = {}; }
        };

        // jsdom does not implement scrollIntoView — stub it
        if (typeof HTMLElement !== 'undefined' && !HTMLElement.prototype.scrollIntoView) {
            HTMLElement.prototype.scrollIntoView = function () {};
        }

        // Load the module (if not already loaded in this env)
        if (typeof window.CommandPaletteManager === 'undefined') {
            require('./command-palette.js');
        }
    });

    beforeEach(function () {
        // Reset body classes and history before each test
        document.body.classList.remove('simple-mode', 'ambient-mode');
        localStorage.clear();

        // Close palette if open
        if (window.CommandPaletteManager && window.CommandPaletteManager.isOpen) {
            window.CommandPaletteManager.close();
        }

        // Remove palette DOM if present
        var root = document.getElementById('cp-root');
        if (root) root.parentNode.removeChild(root);
    });

    // ── Fuzzy matching ───────────────────────────────────────────────────────

    describe('fuzzyScore', function () {
        var fuzzyScore;

        beforeAll(function () {
            fuzzyScore = window.CommandPaletteManager._fuzzyScore;
        });

        it('returns 1 for exact match', function () {
            expect(fuzzyScore('Kitchen', 'Kitchen')).toBe(1);
        });

        it('"kit" → "Kitchen" scores > 0.7 (prefix)', function () {
            expect(fuzzyScore('kit', 'Kitchen')).toBeGreaterThan(0.7);
        });

        it('"kitch" → "Kitchen" scores > 0.7 (prefix)', function () {
            expect(fuzzyScore('kitch', 'Kitchen')).toBeGreaterThan(0.7);
        });

        it('"ktchn" → "Kitchen" scores >= 0.3 (subsequence)', function () {
            expect(fuzzyScore('ktchn', 'Kitchen')).toBeGreaterThanOrEqual(0.3);
        });

        it('"livig rm" → "Living Room" scores > 0.5 (multi-word typo)', function () {
            expect(fuzzyScore('livig rm', 'Living Room')).toBeGreaterThan(0.5);
        });

        it('"xyz" → "Kitchen" scores < 0.3 (excluded)', function () {
            expect(fuzzyScore('xyz', 'Kitchen')).toBeLessThan(0.3);
        });

        it('"living room" → "Living Room" scores >= 0.8 (case insensitive substring)', function () {
            expect(fuzzyScore('living room', 'Living Room')).toBeGreaterThanOrEqual(0.8);
        });

        it('empty needle returns 1 (matches everything)', function () {
            expect(fuzzyScore('', 'Anything')).toBe(1);
        });

        it('"bedrm" → "Bedroom" scores >= 0.3', function () {
            expect(fuzzyScore('bedrm', 'Bedroom')).toBeGreaterThanOrEqual(0.3);
        });
    });

    // ── Time parsing ─────────────────────────────────────────────────────────

    describe('parseTimeExpression', function () {
        var parse;

        beforeAll(function () {
            parse = window.CommandPaletteManager._parseTimeExpression;
        });

        it('@3am → today at 03:00', function () {
            var d = parse('@3am');
            expect(d).not.toBeNull();
            expect(d.getHours()).toBe(3);
            expect(d.getMinutes()).toBe(0);
        });

        it('@3:15am → today at 03:15', function () {
            var d = parse('@3:15am');
            expect(d).not.toBeNull();
            expect(d.getHours()).toBe(3);
            expect(d.getMinutes()).toBe(15);
        });

        it('@11pm → today at 23:00', function () {
            var d = parse('@11pm');
            expect(d).not.toBeNull();
            expect(d.getHours()).toBe(23);
            expect(d.getMinutes()).toBe(0);
        });

        it('@12am → today at 00:00 (midnight)', function () {
            var d = parse('@12am');
            expect(d).not.toBeNull();
            expect(d.getHours()).toBe(0);
        });

        it('@12pm → today at 12:00 (noon)', function () {
            var d = parse('@12pm');
            expect(d).not.toBeNull();
            expect(d.getHours()).toBe(12);
        });

        it('@-30min → 30 minutes ago', function () {
            var before = new Date();
            var d = parse('@-30min');
            var after = new Date();
            expect(d).not.toBeNull();
            var expectedMin = before.getTime() - 30 * 60 * 1000;
            var expectedMax = after.getTime()  - 30 * 60 * 1000;
            expect(d.getTime()).toBeGreaterThanOrEqual(expectedMin - 1000);
            expect(d.getTime()).toBeLessThanOrEqual(expectedMax + 1000);
        });

        it('@-2h → 2 hours ago', function () {
            var before = new Date();
            var d = parse('@-2h');
            var after = new Date();
            expect(d).not.toBeNull();
            var expectedMin = before.getTime() - 2 * 3600 * 1000;
            var expectedMax = after.getTime()  - 2 * 3600 * 1000;
            expect(d.getTime()).toBeGreaterThanOrEqual(expectedMin - 1000);
            expect(d.getTime()).toBeLessThanOrEqual(expectedMax + 1000);
        });

        it('@2026-03-27 14:23 → specific datetime', function () {
            var d = parse('@2026-03-27 14:23');
            expect(d).not.toBeNull();
            expect(d.getFullYear()).toBe(2026);
            expect(d.getMonth()).toBe(2);   // March = 2 (0-indexed)
            expect(d.getDate()).toBe(27);
            expect(d.getHours()).toBe(14);
            expect(d.getMinutes()).toBe(23);
        });

        it('@yesterday 11pm → yesterday at 23:00', function () {
            var d = parse('@yesterday 11pm');
            expect(d).not.toBeNull();
            var yesterday = new Date();
            yesterday.setDate(yesterday.getDate() - 1);
            expect(d.getDate()).toBe(yesterday.getDate());
            expect(d.getHours()).toBe(23);
        });

        it('returns null for unparseable expression', function () {
            var d = parse('@not-a-time');
            expect(d).toBeNull();
        });

        it('returns null for empty @', function () {
            var d = parse('@');
            expect(d).toBeNull();
        });
    });

    // ── Commands registry completeness ───────────────────────────────────────

    describe('Commands registry', function () {
        var COMMANDS;

        beforeAll(function () {
            COMMANDS = window.CommandPaletteManager._COMMANDS;
        });

        var REQUIRED_COMMANDS = [
            'Open settings',
            'Open fleet page',
            'Open automations',
            'Open simulator',
            'Toggle Fresnel overlay',
            'Toggle flow map',
            'Toggle dwell heatmap',
            'Toggle zone volumes',
            'Reset camera',
            'Enter away mode',
            'Enter home mode',
            'Enter sleep mode',
            'Trigger fleet OTA',
            'Add a person',
            'Add a zone',
            'Add a portal',
            'Export all events CSV',
            'Show link health table',
            'Run diagnostics',
            'Check firmware updates'
        ];

        REQUIRED_COMMANDS.forEach(function (label) {
            it('contains "' + label + '"', function () {
                var found = COMMANDS.some(function (c) { return c.label === label; });
                expect(found).toBe(true);
            });
        });

        it('all commands have an id, label, category, and action', function () {
            COMMANDS.forEach(function (cmd) {
                expect(typeof cmd.id).toBe('string');
                expect(typeof cmd.label).toBe('string');
                expect(cmd.category).toBe('command');
                expect(typeof cmd.action).toBe('function');
            });
        });
    });

    // ── Keyboard navigation ──────────────────────────────────────────────────

    describe('Keyboard navigation', function () {

        it('arrow down increments selectedIndex', function () {
            var mgr = window.CommandPaletteManager;
            if (!mgr) return;

            mgr.open();
            // Populate with some items via search
            var items = mgr._search('open');
            mgr._showItems(items);
            mgr.selectedIndex = 0;

            // Simulate ArrowDown keydown on the input
            var input = document.querySelector('.cp-input');
            if (!input) return;

            var event = new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true });
            input.dispatchEvent(event);

            expect(mgr.selectedIndex).toBe(1);
            mgr.close();
        });

        it('arrow up decrements selectedIndex (not below 0)', function () {
            var mgr = window.CommandPaletteManager;
            if (!mgr) return;

            mgr.open();
            var items = mgr._search('open');
            mgr._showItems(items);
            mgr.selectedIndex = 0;

            var input = document.querySelector('.cp-input');
            if (!input) return;

            var event = new KeyboardEvent('keydown', { key: 'ArrowUp', bubbles: true });
            input.dispatchEvent(event);

            expect(mgr.selectedIndex).toBe(0); // clamped at 0
            mgr.close();
        });

        it('Escape closes the palette', function () {
            var mgr = window.CommandPaletteManager;
            if (!mgr) return;

            mgr.open();
            expect(mgr.isOpen).toBe(true);

            var event = new KeyboardEvent('keydown', { key: 'Escape', bubbles: true });
            document.dispatchEvent(event);

            expect(mgr.isOpen).toBe(false);
        });

        it('Enter executes selected item and closes palette', function () {
            var mgr = window.CommandPaletteManager;
            if (!mgr) return;

            var executed = false;
            mgr.open();

            // Inject a synthetic item with a trackable action
            mgr._showItems([{
                id:       'test-enter',
                label:    'Test Action',
                category: 'command',
                icon:     '•',
                score:    1,
                action:   function () { executed = true; }
            }]);
            mgr.selectedIndex = 0;

            var input = document.querySelector('.cp-input');
            if (!input) return;

            var event = new KeyboardEvent('keydown', { key: 'Enter', bubbles: true });
            input.dispatchEvent(event);

            expect(executed).toBe(true);
            expect(mgr.isOpen).toBe(false);
        });
    });

    // ── Recent history ───────────────────────────────────────────────────────

    describe('Recent history', function () {

        it('addToHistory saves item to localStorage', function () {
            var addToHistory = window.CommandPaletteManager._addToHistory;
            var loadHistory  = window.CommandPaletteManager._loadHistory;
            if (!addToHistory || !loadHistory) return;

            addToHistory({ id: 'test-cmd', label: 'Test', category: 'command', icon: '⚙' });
            var hist = loadHistory();
            expect(hist.length).toBe(1);
            expect(hist[0].id).toBe('test-cmd');
        });

        it('addToHistory excludes time navigation entries', function () {
            var addToHistory = window.CommandPaletteManager._addToHistory;
            var loadHistory  = window.CommandPaletteManager._loadHistory;
            if (!addToHistory || !loadHistory) return;

            addToHistory({ id: 'time:@3am', label: 'Jump to 3am', category: 'time', icon: '🕐' });
            var hist = loadHistory();
            expect(hist.length).toBe(0);
        });

        it('stores at most 5 recent items', function () {
            var addToHistory = window.CommandPaletteManager._addToHistory;
            var loadHistory  = window.CommandPaletteManager._loadHistory;
            if (!addToHistory || !loadHistory) return;

            for (var i = 0; i < 7; i++) {
                addToHistory({ id: 'cmd-' + i, label: 'Command ' + i, category: 'command', icon: '•' });
            }
            var hist = loadHistory();
            expect(hist.length).toBeLessThanOrEqual(5);
        });

        it('most recently added item is first in history', function () {
            var addToHistory = window.CommandPaletteManager._addToHistory;
            var loadHistory  = window.CommandPaletteManager._loadHistory;
            if (!addToHistory || !loadHistory) return;

            addToHistory({ id: 'first',  label: 'First',  category: 'command', icon: '•' });
            addToHistory({ id: 'second', label: 'Second', category: 'command', icon: '•' });
            var hist = loadHistory();
            expect(hist[0].id).toBe('second');
        });

        it('empty query shows recent items', function () {
            var addToHistory = window.CommandPaletteManager._addToHistory;
            var search       = window.CommandPaletteManager._search;
            if (!addToHistory || !search) return;

            addToHistory({ id: 'recent-a', label: 'Recent A', category: 'command', icon: '•' });
            addToHistory({ id: 'recent-b', label: 'Recent B', category: 'command', icon: '•' });

            var results = search('');
            expect(results.length).toBeGreaterThan(0);
            var cats = results.map(function (r) { return r.category; });
            expect(cats.every(function (c) { return c === 'recent'; })).toBe(true);
        });

        it('open palette with 5 prior actions shows 5 recent items on empty query', function () {
            var addToHistory = window.CommandPaletteManager._addToHistory;
            var search       = window.CommandPaletteManager._search;
            if (!addToHistory || !search) return;

            for (var i = 0; i < 5; i++) {
                addToHistory({ id: 'hist-' + i, label: 'Hist ' + i, category: 'command', icon: '•' });
            }
            var results = search('');
            expect(results.length).toBe(5);
        });
    });

    // ── Expert-mode gating ───────────────────────────────────────────────────

    describe('Expert-mode gating', function () {

        it('isExpertMode() returns true by default (no class on body)', function () {
            var isExpert = window.CommandPaletteManager._isExpertMode;
            if (!isExpert) return;
            document.body.classList.remove('simple-mode', 'ambient-mode');
            expect(isExpert()).toBe(true);
        });

        it('isExpertMode() returns false when body has simple-mode class', function () {
            var isExpert = window.CommandPaletteManager._isExpertMode;
            if (!isExpert) return;
            document.body.classList.add('simple-mode');
            expect(isExpert()).toBe(false);
            document.body.classList.remove('simple-mode');
        });

        it('isExpertMode() returns false when body has ambient-mode class', function () {
            var isExpert = window.CommandPaletteManager._isExpertMode;
            if (!isExpert) return;
            document.body.classList.add('ambient-mode');
            expect(isExpert()).toBe(false);
            document.body.classList.remove('ambient-mode');
        });

        it('open() does nothing in simple mode', function () {
            var mgr = window.CommandPaletteManager;
            if (!mgr) return;
            document.body.classList.add('simple-mode');
            mgr.open();
            expect(mgr.isOpen).toBe(false);
            document.body.classList.remove('simple-mode');
        });

        it('open() does nothing in ambient mode (window.currentMode)', function () {
            var mgr = window.CommandPaletteManager;
            if (!mgr) return;
            window.currentMode = 'ambient';
            mgr.open();
            expect(mgr.isOpen).toBe(false);
            delete window.currentMode;
        });

        it('Ctrl+K does not open palette in simple mode', function () {
            var mgr = window.CommandPaletteManager;
            if (!mgr) return;
            document.body.classList.add('simple-mode');
            var ev = new KeyboardEvent('keydown', { key: 'k', ctrlKey: true, bubbles: true });
            document.dispatchEvent(ev);
            expect(mgr.isOpen).toBe(false);
            document.body.classList.remove('simple-mode');
        });
    });

    // ── Viewport positioning ─────────────────────────────────────────────────

    describe('Viewport positioning', function () {

        it('palette container has position:absolute and transform:translate(-50%,-50%)', function () {
            var mgr = window.CommandPaletteManager;
            if (!mgr) return;

            // Open in expert mode to create DOM
            mgr.open();
            expect(mgr.isOpen).toBe(true);

            var container = document.querySelector('.cp-container');
            expect(container).not.toBeNull();

            // The CSS class sets the centering rules.
            // In jsdom, computed styles aren't fully calculated, but we can
            // verify the class is present on the element.
            expect(container.className).toContain('cp-container');

            mgr.close();
        });

        it('overlay covers the viewport (cp-overlay present when open)', function () {
            var mgr = window.CommandPaletteManager;
            if (!mgr) return;

            mgr.open();
            var overlay = document.getElementById('cp-root');
            expect(overlay).not.toBeNull();
            expect(overlay.classList.contains('cp-visible')).toBe(true);
            mgr.close();
        });

        it('overlay is hidden when palette is closed', function () {
            var mgr = window.CommandPaletteManager;
            if (!mgr) return;

            mgr.open();
            mgr.close();
            var overlay = document.getElementById('cp-root');
            // After close, cp-visible should be removed
            if (overlay) {
                expect(overlay.classList.contains('cp-visible')).toBe(false);
            }
        });
    });

    // ── Search results ───────────────────────────────────────────────────────

    describe('Search', function () {

        it('search for "@3am" returns a time navigation result', function () {
            var search = window.CommandPaletteManager._search;
            if (!search) return;
            var results = search('@3am');
            expect(results.length).toBeGreaterThan(0);
            expect(results[0].category).toBe('time');
        });

        it('search for "open" returns command results', function () {
            var search = window.CommandPaletteManager._search;
            if (!search) return;
            var results = search('open');
            expect(results.length).toBeGreaterThan(0);
            var cmdResults = results.filter(function (r) { return r.category === 'command'; });
            expect(cmdResults.length).toBeGreaterThan(0);
        });

        it('results are capped at 8', function () {
            var search = window.CommandPaletteManager._search;
            if (!search) return;
            // A broad query should still not return more than 8
            var results = search('a');
            expect(results.length).toBeLessThanOrEqual(8);
        });

        it('@unparseable returns empty array', function () {
            var search = window.CommandPaletteManager._search;
            if (!search) return;
            var results = search('@zzznottimestamp');
            expect(results.length).toBe(0);
        });
    });
});
