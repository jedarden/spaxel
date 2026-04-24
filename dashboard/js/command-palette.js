/**
 * Spaxel Dashboard — Command Palette (Component 34)
 *
 * Ctrl+K / Cmd+K: universal keyboard-driven interface for expert mode.
 * Fuzzy search across zones, people, nodes, events, and commands.
 * Time navigation via "@" prefix.
 *
 * Exposes: window.CommandPaletteManager
 */

(function () {
    'use strict';

    // =========================================================
    // Constants
    // =========================================================
    var STORAGE_KEY = 'spaxel_palette_history';
    var MAX_RECENT  = 5;
    var MAX_RESULTS = 8;

    // Category priority (lower = higher in results)
    var CAT_PRIORITY = {
        command:   0,
        time:      1,
        person:    2,
        zone:      3,
        node:      4,
        event:     5,
        recent:   -1   // shown only on empty query
    };

    // =========================================================
    // Levenshtein distance (compact)
    // =========================================================
    function levenshteinDist(a, b) {
        var m = a.length, n = b.length;
        if (!m) return n;
        if (!n) return m;
        var prev = [], curr = [];
        for (var j = 0; j <= n; j++) prev[j] = j;
        for (var i = 1; i <= m; i++) {
            curr[0] = i;
            for (var k = 1; k <= n; k++) {
                curr[k] = a[i - 1] === b[k - 1]
                    ? prev[k - 1]
                    : 1 + Math.min(prev[k], curr[k - 1], prev[k - 1]);
            }
            var tmp = prev; prev = curr; curr = tmp;
        }
        return prev[n];
    }

    // =========================================================
    // Fuzzy scorer  [0, 1]
    // =========================================================
    /**
     * Returns a score in [0, 1] indicating how well `needle` matches `haystack`.
     * Scores below 0.3 are considered non-matches and are excluded from results.
     *
     * Matching strategy (in priority order):
     *   1. Exact prefix of full haystack              → 0.90–1.00
     *   2. Exact substring of full haystack           → 0.80
     *   3. Word-level matching (prefix / typo / subseq per word)
     *   4. Character subsequence across full string   → 0.30–0.40
     */
    function fuzzyScore(needle, haystack) {
        if (!needle) return 1;
        needle   = needle.toLowerCase().trim();
        haystack = haystack.toLowerCase().trim();
        if (!needle) return 1;
        if (needle === haystack) return 1;

        // 1. Full prefix
        if (haystack.startsWith(needle)) {
            return 0.90 + 0.10 * (needle.length / haystack.length);
        }

        // 2. Exact substring
        if (haystack.includes(needle)) {
            return 0.80;
        }

        // 3. Word-level matching
        var needleWords   = needle.split(/\s+/).filter(function (w) { return w.length > 0; });
        var haystackWords = haystack.split(/\s+/).filter(function (w) { return w.length > 0; });

        if (needleWords.length > 0) {
            var allMatch   = true;
            var totalScore = 0;

            for (var ni = 0; ni < needleWords.length; ni++) {
                var nw = needleWords[ni];
                var bestWord = 0;

                for (var hi = 0; hi < haystackWords.length; hi++) {
                    var hw = haystackWords[hi];
                    var ws = 0;

                    if (hw.startsWith(nw)) {
                        ws = 0.90;
                    } else if (hw.includes(nw)) {
                        ws = 0.75;
                    } else if (nw.length > 2 && hw.length > 2) {
                        var dist = levenshteinDist(nw, hw);
                        if (dist === 1) {
                            ws = 0.70;
                        } else if (dist === 2 && nw.length > 4) {
                            ws = 0.50;
                        }
                        // Per-word subsequence (e.g. "rm" in "room")
                        if (ws === 0) {
                            var si = 0;
                            for (var ci = 0; ci < hw.length && si < nw.length; ci++) {
                                if (nw[si] === hw[ci]) si++;
                            }
                            if (si === nw.length) ws = 0.50;
                        }
                    } else if (nw.length <= 2) {
                        // Short needle: prefix or subsequence within each haystack word
                        if (hw.startsWith(nw)) {
                            ws = 0.75;
                        } else {
                            var si2 = 0;
                            for (var ci2 = 0; ci2 < hw.length && si2 < nw.length; ci2++) {
                                if (nw[si2] === hw[ci2]) si2++;
                            }
                            if (si2 === nw.length) ws = 0.50;
                        }
                    }

                    if (ws > bestWord) bestWord = ws;
                }

                if (bestWord === 0) {
                    allMatch = false;
                    break;
                }
                totalScore += bestWord;
            }

            if (allMatch) {
                return 0.40 + (totalScore / needleWords.length) * 0.30;
            }
        }

        // 4. Character subsequence across full string
        var si3 = 0;
        for (var ci3 = 0; ci3 < haystack.length && si3 < needle.length; ci3++) {
            if (needle[si3] === haystack[ci3]) si3++;
        }
        if (si3 === needle.length) {
            return 0.30 + 0.10 * (needle.length / haystack.length);
        }

        return 0;
    }

    // =========================================================
    // Time expression parser
    // =========================================================
    /**
     * Parse a "@..." time expression.
     * @param {string} query - full query string starting with "@"
     * @returns {Date|null}
     */
    function parseTimeExpression(query) {
        var s = query.replace(/^@/, '').trim();
        if (!s) return null;
        var now = new Date();

        // @-30min  @-2h
        var rel = s.match(/^-(\d+)(min|h)$/i);
        if (rel) {
            var amount = parseInt(rel[1], 10);
            var unit   = rel[2].toLowerCase();
            var d = new Date(now);
            if (unit === 'min') d.setMinutes(d.getMinutes() - amount);
            else                d.setHours(d.getHours() - amount);
            return d;
        }

        // @2026-03-27 14:23
        var abs = s.match(/^(\d{4}-\d{2}-\d{2})\s+(\d{1,2}:\d{2})$/);
        if (abs) {
            var dt = new Date(abs[1] + 'T' + abs[2] + ':00');
            if (!isNaN(dt.getTime())) return dt;
        }

        // @yesterday ...
        var yest = s.match(/^yesterday\s+(.+)$/i);
        if (yest) {
            var base = new Date(now);
            base.setDate(base.getDate() - 1);
            return parseTimeOfDay(yest[1], base);
        }

        // @3am  @3:15pm  @14:23
        return parseTimeOfDay(s, new Date(now));
    }

    function parseTimeOfDay(s, baseDate) {
        // 12-hour: 3am, 3:15am, 11:30pm
        var m12 = s.match(/^(\d{1,2})(?::(\d{2}))?\s*(am|pm)$/i);
        if (m12) {
            var h = parseInt(m12[1], 10);
            var min = m12[2] ? parseInt(m12[2], 10) : 0;
            var ampm = m12[3].toLowerCase();
            if (ampm === 'pm' && h !== 12) h += 12;
            if (ampm === 'am' && h === 12) h = 0;
            var r = new Date(baseDate);
            r.setHours(h, min, 0, 0);
            return r;
        }
        // 24-hour: 14:23
        var m24 = s.match(/^(\d{1,2}):(\d{2})$/);
        if (m24) {
            var r2 = new Date(baseDate);
            r2.setHours(parseInt(m24[1], 10), parseInt(m24[2], 10), 0, 0);
            return r2;
        }
        return null;
    }

    // =========================================================
    // Command registry
    // =========================================================
    var COMMANDS = [
        // ---- Navigation ----
        {
            id: 'nav-settings',
            label: 'Open settings',
            category: 'command',
            group: 'Navigation',
            icon: '⚙',
            hint: '',
            action: function () { window.location.href = '/settings'; }
        },
        {
            id: 'nav-fleet',
            label: 'Open fleet page',
            category: 'command',
            group: 'Navigation',
            icon: '📡',
            hint: '',
            action: function () { window.location.href = '/fleet'; }
        },
        {
            id: 'nav-automations',
            label: 'Open automations',
            category: 'command',
            group: 'Navigation',
            icon: '⚡',
            hint: '',
            action: function () { window.location.href = '/automations'; }
        },
        {
            id: 'nav-simulator',
            label: 'Open simulator',
            category: 'command',
            group: 'Navigation',
            icon: '🔬',
            hint: '',
            action: function () { window.location.href = '/simulate'; }
        },
        // ---- View ----
        {
            id: 'view-fresnel',
            label: 'Toggle Fresnel overlay',
            category: 'command',
            group: 'View',
            icon: '◈',
            hint: '',
            action: function () {
                if (window.toggleFresnelZones) window.toggleFresnelZones();
            }
        },
        {
            id: 'view-flowmap',
            label: 'Toggle flow map',
            category: 'command',
            group: 'View',
            icon: '🌊',
            hint: '',
            action: function () {
                if (window.Viz3D && window.Viz3D.toggleFlowLayer) window.Viz3D.toggleFlowLayer();
            }
        },
        {
            id: 'view-heatmap',
            label: 'Toggle dwell heatmap',
            category: 'command',
            group: 'View',
            icon: '🔥',
            hint: '',
            action: function () {
                if (window.Viz3D && window.Viz3D.toggleDwellLayer) window.Viz3D.toggleDwellLayer();
            }
        },
        {
            id: 'view-zones',
            label: 'Toggle zone volumes',
            category: 'command',
            group: 'View',
            icon: '📦',
            hint: '',
            action: function () {
                if (window.ZoneEditor && window.ZoneEditor.toggleVolumes) window.ZoneEditor.toggleVolumes();
                else if (window.Viz3D && window.Viz3D.toggleZoneVolumes) window.Viz3D.toggleZoneVolumes();
            }
        },
        {
            id: 'view-reset-camera',
            label: 'Reset camera',
            category: 'command',
            group: 'View',
            icon: '🎥',
            hint: '',
            action: function () {
                if (window.Viz3D && window.Viz3D.setViewPreset) window.Viz3D.setViewPreset('topdown');
            }
        },
        // ---- System ----
        {
            id: 'mode-away',
            label: 'Enter away mode',
            category: 'command',
            group: 'System',
            icon: '🏠',
            hint: '',
            action: function () {
                fetch('/api/mode', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ mode: 'away' })
                });
            }
        },
        {
            id: 'mode-home',
            label: 'Enter home mode',
            category: 'command',
            group: 'System',
            icon: '🏡',
            hint: '',
            action: function () {
                fetch('/api/mode', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ mode: 'home' })
                });
            }
        },
        {
            id: 'mode-sleep',
            label: 'Enter sleep mode',
            category: 'command',
            group: 'System',
            icon: '🌙',
            hint: '',
            action: function () {
                fetch('/api/mode', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ mode: 'sleep' })
                });
            }
        },
        {
            id: 'ota-fleet',
            label: 'Trigger fleet OTA',
            category: 'command',
            group: 'System',
            icon: '⬆',
            hint: '',
            action: function () {
                if (window.SpaxelOTA && window.SpaxelOTA.openDialog) {
                    window.SpaxelOTA.openDialog();
                } else {
                    fetch('/api/nodes/update-all', { method: 'POST' });
                }
            }
        },
        {
            id: 'add-person',
            label: 'Add a person',
            category: 'command',
            group: 'System',
            icon: '👤',
            hint: '',
            action: function () {
                if (window.BLEPanel && window.BLEPanel.openAddPerson) window.BLEPanel.openAddPerson();
            }
        },
        {
            id: 'add-zone',
            label: 'Add a zone',
            category: 'command',
            group: 'System',
            icon: '📍',
            hint: '',
            action: function () {
                if (window.ZoneEditor && window.ZoneEditor.startCreate) window.ZoneEditor.startCreate();
            }
        },
        {
            id: 'add-portal',
            label: 'Add a portal',
            category: 'command',
            group: 'System',
            icon: '🚪',
            hint: '',
            action: function () {
                if (window.PortalEditor && window.PortalEditor.startCreate) window.PortalEditor.startCreate();
            }
        },
        // ---- Debug ----
        {
            id: 'debug-export-csv',
            label: 'Export all events CSV',
            category: 'command',
            group: 'Debug',
            icon: '📥',
            hint: '',
            action: function () {
                var a = document.createElement('a');
                a.href = '/api/events?format=csv';
                a.download = 'spaxel-events.csv';
                a.click();
            }
        },
        {
            id: 'debug-link-health',
            label: 'Show link health table',
            category: 'command',
            group: 'Debug',
            icon: '📊',
            hint: '',
            action: function () {
                if (window.LinkHealth && window.LinkHealth.openPanel) window.LinkHealth.openPanel();
            }
        },
        {
            id: 'debug-diagnostics',
            label: 'Run diagnostics',
            category: 'command',
            group: 'Debug',
            icon: '🔧',
            hint: '',
            action: function () {
                fetch('/api/diagnostics', { method: 'POST' }).then(function (r) {
                    return r.json();
                }).then(function (data) {
                    if (window.showToast) window.showToast('Diagnostics: ' + (data.summary || 'done'), 'info');
                }).catch(function () {
                    if (window.showToast) window.showToast('Diagnostics triggered', 'info');
                });
            }
        },
        {
            id: 'debug-firmware-check',
            label: 'Check firmware updates',
            category: 'command',
            group: 'Debug',
            icon: '🔄',
            hint: '',
            action: function () {
                fetch('/api/firmware').then(function (r) { return r.json(); }).then(function (data) {
                    var latest = data && data[0] ? data[0].version : '?';
                    if (window.showToast) window.showToast('Latest firmware: v' + latest, 'info');
                }).catch(function () {
                    if (window.showToast) window.showToast('Could not fetch firmware info', 'warning');
                });
            }
        }
    ];

    // =========================================================
    // Recent history  (localStorage)
    // =========================================================
    function loadHistory() {
        try {
            return JSON.parse(localStorage.getItem(STORAGE_KEY) || '[]');
        } catch (e) {
            return [];
        }
    }

    function saveHistory(items) {
        try {
            localStorage.setItem(STORAGE_KEY, JSON.stringify(items.slice(0, MAX_RECENT)));
        } catch (e) {
            // quota error — ignore
        }
    }

    function addToHistory(item) {
        // Exclude time navigation entries
        if (item.category === 'time') return;
        var hist = loadHistory().filter(function (h) { return h.id !== item.id; });
        hist.unshift({ id: item.id, label: item.label, category: item.category, icon: item.icon });
        saveHistory(hist);
    }

    // =========================================================
    // Entity data source
    // =========================================================
    /**
     * Returns a snapshot of searchable entities from app state or cached API data.
     * @returns {{ nodes: Array, zones: Array, people: Array, events: Array }}
     */
    function getEntityData() {
        var data = { nodes: [], zones: [], people: [], events: [] };

        // Nodes: from app.js state exposure
        if (window.spaxelGetState) {
            var st = window.spaxelGetState();
            data.nodes = st.nodes || [];
        }

        // Zones / people / events: use cached API snapshot if available
        var cache = Manager._entityCache;
        if (cache) {
            data.zones   = cache.zones   || data.zones;
            data.people  = cache.people  || data.people;
            data.events  = cache.events  || data.events;
        }

        return data;
    }

    // =========================================================
    // Search
    // =========================================================
    /**
     * Search all categories with the given query.
     * @param {string} query
     * @returns {Array} sorted result items
     */
    function search(query) {
        var results = [];
        var q = query.trim();

        // Empty query: show recent history
        if (!q) {
            var hist = loadHistory();
            return hist.slice(0, MAX_RECENT).map(function (h) {
                return {
                    id:       h.id,
                    label:    h.label,
                    category: 'recent',
                    icon:     h.icon || '🕐',
                    secondary: 'Recent',
                    score:    1,
                    action:   findCommandAction(h.id)
                };
            });
        }

        // Time navigation
        if (q.startsWith('@')) {
            var dt = parseTimeExpression(q);
            if (dt) {
                var label = 'Jump to ' + dt.toLocaleString();
                results.push({
                    id:       'time:' + q,
                    label:    label,
                    category: 'time',
                    icon:     '🕐',
                    secondary: dt.toISOString(),
                    score:    1,
                    action: function () {
                        if (window.SpaxelReplay && window.SpaxelReplay.seekTo) {
                            window.SpaxelReplay.seekTo(dt.getTime());
                        }
                    }
                });
            }
            return results;
        }

        var entities = getEntityData();

        // Commands
        COMMANDS.forEach(function (cmd) {
            var s = Math.max(
                fuzzyScore(q, cmd.label),
                fuzzyScore(q, cmd.group || '')
            );
            if (s >= 0.3) {
                results.push({
                    id:       cmd.id,
                    label:    cmd.label,
                    category: 'command',
                    icon:     cmd.icon,
                    secondary: cmd.group || '',
                    score:    s,
                    action:   cmd.action
                });
            }
        });

        // People
        entities.people.forEach(function (p) {
            var name = p.name || p.label || p.addr || '';
            var s = fuzzyScore(q, name);
            if (s >= 0.3) {
                results.push({
                    id:       'person:' + name,
                    label:    name,
                    category: 'person',
                    icon:     '👤',
                    secondary: p.zone || '',
                    score:    s,
                    action: function () {
                        if (window.Viz3D && window.Viz3D.flyToPerson) window.Viz3D.flyToPerson(name);
                    }
                });
            }
        });

        // Zones
        entities.zones.forEach(function (z) {
            var name = z.name || '';
            var s = fuzzyScore(q, name);
            if (s >= 0.3) {
                var count = z.count != null ? z.count : (z.occupancy || 0);
                results.push({
                    id:       'zone:' + name,
                    label:    name,
                    category: 'zone',
                    icon:     '📍',
                    secondary: count + ' people currently',
                    score:    s,
                    action: function () {
                        if (window.Viz3D && window.Viz3D.flyToZone) window.Viz3D.flyToZone(name);
                    }
                });
            }
        });

        // Nodes
        entities.nodes.forEach(function (n) {
            var label = n.name || n.mac || '';
            var s = Math.max(
                fuzzyScore(q, label),
                n.mac ? fuzzyScore(q, n.mac) : 0
            );
            if (s >= 0.3) {
                results.push({
                    id:       'node:' + (n.mac || label),
                    label:    label,
                    category: 'node',
                    icon:     '📡',
                    secondary: n.status || '',
                    score:    s,
                    action: function () {
                        if (window.Viz3D && window.Viz3D.flyToNode && n.mac) window.Viz3D.flyToNode(n.mac);
                    }
                });
            }
        });

        // Recent events (last 20)
        entities.events.forEach(function (evt) {
            var title = evt.title || evt.type || '';
            var s = fuzzyScore(q, title);
            if (s >= 0.3) {
                results.push({
                    id:       'event:' + (evt.id || title),
                    label:    title,
                    category: 'event',
                    icon:     '🕐',
                    secondary: evt.zone || '',
                    score:    s,
                    action: function () {
                        if (window.SpaxelTimeline && window.SpaxelTimeline.openEvent) {
                            window.SpaxelTimeline.openEvent(evt.id);
                        }
                    }
                });
            }
        });

        // Sort: commands first (if query starts with "/"), then by category priority, then score desc
        results.sort(function (a, b) {
            var pa = CAT_PRIORITY[a.category] != null ? CAT_PRIORITY[a.category] : 99;
            var pb = CAT_PRIORITY[b.category] != null ? CAT_PRIORITY[b.category] : 99;
            if (pa !== pb) return pa - pb;
            return b.score - a.score;
        });

        return results.slice(0, MAX_RESULTS);
    }

    function findCommandAction(id) {
        for (var i = 0; i < COMMANDS.length; i++) {
            if (COMMANDS[i].id === id) return COMMANDS[i].action;
        }
        return function () {};
    }

    // =========================================================
    // Mode detection
    // =========================================================
    function isExpertMode() {
        // Palette is unavailable in simple mode or ambient mode
        if (document.body.classList.contains('simple-mode')) return false;
        if (document.body.classList.contains('ambient-mode')) return false;
        if (window.currentMode === 'simple' || window.currentMode === 'ambient') return false;
        return true;
    }

    // =========================================================
    // DOM creation
    // =========================================================
    function createDOM() {
        if (document.getElementById('cp-root')) return;

        var root = document.createElement('div');
        root.id        = 'cp-root';
        root.className = 'cp-overlay';
        root.setAttribute('role', 'dialog');
        root.setAttribute('aria-modal', 'true');
        root.setAttribute('aria-label', 'Command palette');
        root.innerHTML =
            '<div class="cp-backdrop"></div>' +
            '<div class="cp-container" role="combobox" aria-haspopup="listbox" aria-expanded="true">' +
            '  <div class="cp-search-row">' +
            '    <span class="cp-search-icon">🔍</span>' +
            '    <input class="cp-input" type="text" autocomplete="off" spellcheck="false"' +
            '      placeholder="Search people, zones, nodes, commands..." />' +
            '    <span class="cp-esc-hint">ESC</span>' +
            '  </div>' +
            '  <ul class="cp-results" role="listbox" id="cp-listbox"></ul>' +
            '</div>';

        document.body.appendChild(root);
        Manager.el = root;
    }

    // =========================================================
    // Rendering
    // =========================================================
    function renderResults(items) {
        var list = document.getElementById('cp-listbox');
        if (!list) return;

        if (!items.length) {
            list.innerHTML = '<li class="cp-empty">No results</li>';
            return;
        }

        var html = '';
        var lastCat = null;

        for (var i = 0; i < items.length; i++) {
            var item = items[i];

            // Group header for "Recent"
            if (item.category === 'recent' && lastCat !== 'recent') {
                html += '<li class="cp-group-header">Recent</li>';
            }

            var selectedClass = (i === Manager.selectedIndex) ? ' cp-item-selected' : '';
            html +=
                '<li class="cp-item' + selectedClass + '" data-index="' + i + '" role="option"' +
                '  aria-selected="' + (i === Manager.selectedIndex) + '">' +
                '  <span class="cp-item-icon">' + (item.icon || '•') + '</span>' +
                '  <span class="cp-item-body">' +
                '    <span class="cp-item-label">' + escapeHtml(item.label) + '</span>' +
                '    <span class="cp-item-secondary">' + escapeHtml(item.secondary || '') + '</span>' +
                '  </span>' +
                '  <span class="cp-item-arrow">›</span>' +
                '</li>';

            lastCat = item.category;
        }

        list.innerHTML = html;

        // Click handlers
        list.querySelectorAll('.cp-item').forEach(function (el) {
            el.addEventListener('mousedown', function (e) {
                e.preventDefault(); // prevent input blur
                var idx = parseInt(el.getAttribute('data-index'), 10);
                Manager.selectedIndex = idx;
                Manager.execute();
            });
        });
    }

    function escapeHtml(s) {
        return String(s)
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;');
    }

    // =========================================================
    // Entity cache loader  (one fetch per palette open)
    // =========================================================
    function loadEntityCache() {
        Manager._entityCache = Manager._entityCache || { zones: [], people: [], events: [] };

        // Fetch zones
        fetch('/api/zones').then(function (r) { return r.json(); }).then(function (data) {
            Manager._entityCache.zones = Array.isArray(data) ? data : [];
        }).catch(function () {});

        // Fetch people (BLE devices of type "person")
        fetch('/api/ble/devices?registered=true').then(function (r) { return r.json(); }).then(function (data) {
            Manager._entityCache.people = (Array.isArray(data) ? data : [])
                .filter(function (d) { return d.type === 'person'; });
        }).catch(function () {});

        // Fetch recent events
        fetch('/api/events?limit=20').then(function (r) { return r.json(); }).then(function (data) {
            var arr = data && Array.isArray(data.events) ? data.events : (Array.isArray(data) ? data : []);
            Manager._entityCache.events = arr.slice(0, 20).map(function (e) {
                return { id: e.id, title: e.type || '', zone: e.zone || '', ts: e.timestamp_ms };
            });
        }).catch(function () {});
    }

    // =========================================================
    // Manager
    // =========================================================
    var Manager = {
        el:            null,
        isOpen:        false,
        selectedIndex: 0,
        _items:        [],
        _entityCache:  null,

        init: function () {
            // Register Ctrl+K / Cmd+K globally
            document.addEventListener('keydown', this._onKeydown.bind(this));
        },

        open: function () {
            if (!isExpertMode()) return;

            createDOM();

            // Refresh entity cache (async, non-blocking)
            loadEntityCache();

            this.isOpen = true;
            this.selectedIndex = 0;
            this.el.classList.add('cp-visible');

            var input = this.el.querySelector('.cp-input');
            if (input) {
                input.value = '';
                setTimeout(function () { input.focus(); }, 10);
                input.addEventListener('input',   this._onInput.bind(this));
                input.addEventListener('keydown', this._onInputKeydown.bind(this));
            }

            var backdrop = this.el.querySelector('.cp-backdrop');
            if (backdrop) {
                backdrop.addEventListener('click', this.close.bind(this));
            }

            this._showItems([]);
        },

        close: function () {
            if (!this.isOpen) return;
            this.isOpen = false;

            if (this.el) {
                this.el.classList.remove('cp-visible');
                // Detach listeners by replacing input (simple)
                var input = this.el.querySelector('.cp-input');
                if (input) {
                    var newInput = input.cloneNode(true);
                    input.parentNode.replaceChild(newInput, input);
                }
            }
        },

        toggle: function () {
            if (this.isOpen) this.close();
            else             this.open();
        },

        execute: function () {
            var item = this._items[this.selectedIndex];
            if (!item) return;
            if (item.action) {
                addToHistory(item);
                item.action();
            }
            this.close();
        },

        _onKeydown: function (e) {
            if ((e.ctrlKey || e.metaKey) && e.key === 'k') {
                e.preventDefault();
                if (!isExpertMode()) return;
                this.toggle();
            } else if (e.key === 'Escape' && this.isOpen) {
                e.preventDefault();
                this.close();
            }
        },

        _onInput: function (e) {
            var q = e.target.value;
            this.selectedIndex = 0;
            var items = search(q);
            this._showItems(items);
        },

        _onInputKeydown: function (e) {
            switch (e.key) {
            case 'ArrowDown':
                e.preventDefault();
                this.selectedIndex = Math.min(this.selectedIndex + 1, this._items.length - 1);
                renderResults(this._items);
                this._scrollToSelected();
                break;
            case 'ArrowUp':
                e.preventDefault();
                this.selectedIndex = Math.max(this.selectedIndex - 1, 0);
                renderResults(this._items);
                this._scrollToSelected();
                break;
            case 'Enter':
            case 'Tab':
                e.preventDefault();
                this.execute();
                break;
            case 'Escape':
                this.close();
                break;
            }
        },

        _showItems: function (items) {
            this._items = items;
            renderResults(items);
        },

        _scrollToSelected: function () {
            var list = document.getElementById('cp-listbox');
            if (!list) return;
            var sel = list.querySelector('.cp-item-selected');
            if (sel) sel.scrollIntoView({ block: 'nearest' });
        }
    };

    // =========================================================
    // Public API
    // =========================================================
    window.CommandPaletteManager = Manager;

    // Expose internals for testing
    Manager._fuzzyScore          = fuzzyScore;
    Manager._parseTimeExpression = parseTimeExpression;
    Manager._parseTimeOfDay      = parseTimeOfDay;
    Manager._COMMANDS            = COMMANDS;
    Manager._loadHistory         = loadHistory;
    Manager._saveHistory         = saveHistory;
    Manager._addToHistory        = addToHistory;
    Manager._search              = search;
    Manager._isExpertMode        = isExpertMode;

    // Auto-init when DOM is ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', function () { Manager.init(); });
    } else {
        Manager.init();
    }

})();
