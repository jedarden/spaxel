/**
 * home-cards.js — Populates the Spaxel home page from /ws/dashboard snapshot.
 *
 * Connects to the dashboard WebSocket, reads the initial snapshot to fill all
 * three cards, then applies incremental updates to refresh counts only.
 * Caches the full snapshot so incremental merges always have complete state.
 */
(function () {
  'use strict';

  // ── DOM refs ──
  var $banner      = document.getElementById('status-banner');
  var $peopleCnt   = document.getElementById('people-count');
  var $peopleDtl   = document.getElementById('people-detail');
  var $devicesCnt  = document.getElementById('devices-count');
  var $devicesDtl  = document.getElementById('devices-detail');
  var $devicesMeta = document.getElementById('devices-meta');
  var $eventsDtl   = document.getElementById('events-detail');
  var $briefing    = document.getElementById('briefing-card');
  var $anomaly     = document.getElementById('anomaly-banner');
  var $security    = document.getElementById('security-toggle');

  // ── State ──
  var ws = null;
  var reconnectDelay = 1000;
  var recentEvents = [];
  // Cached snapshot — always holds the latest full state
  var cached = { blobs: [], nodes: [], zones: [], events: [], triggers: [] };

  // ── Helpers ──

  function relativeTime(isoOrMs) {
    var ts = typeof isoOrMs === 'number' ? isoOrMs : new Date(isoOrMs).getTime();
    var diff = Date.now() - ts;
    if (diff < 60000)    return 'just now';
    if (diff < 3600000)  return Math.floor(diff / 60000) + 'm ago';
    if (diff < 86400000) return Math.floor(diff / 3600000) + 'h ago';
    return Math.floor(diff / 86400000) + 'd ago';
  }

  function setBanner(level, html) {
    $banner.className = 'home-status__banner home-status__banner--' + level;
    $banner.innerHTML = '<span class="home-status__dot home-status__dot--connected" id="ws-dot"></span>' + html;
  }

  // Merge an incremental message's partial arrays into the cached snapshot.
  // Incremental messages only contain items that changed — replace by id/key.
  function mergeIncremental(msg) {
    if (msg.blobs) {
      if (msg.type !== 'snapshot') {
        // Replace existing blobs with same id, add new ones
        var ids = {};
        for (var i = 0; i < msg.blobs.length; i++) ids[msg.blobs[i].id] = msg.blobs[i];
        cached.blobs = cached.blobs.filter(function (b) { return !(b.id in ids); }).concat(msg.blobs);
      } else {
        cached.blobs = msg.blobs;
      }
    }
    if (msg.nodes) {
      if (msg.type !== 'snapshot') {
        var macs = {};
        for (var i = 0; i < msg.nodes.length; i++) macs[msg.nodes[i].mac] = msg.nodes[i];
        cached.nodes = cached.nodes.filter(function (n) { return !(n.mac in macs); }).concat(msg.nodes);
      } else {
        cached.nodes = msg.nodes;
      }
    }
    if (msg.zones) {
      if (msg.type !== 'snapshot') {
        var zids = {};
        for (var i = 0; i < msg.zones.length; i++) zids[msg.zones[i].id] = msg.zones[i];
        cached.zones = cached.zones.filter(function (z) { return !(z.id in zids); }).concat(msg.zones);
      } else {
        cached.zones = msg.zones;
      }
    }
    if (msg.triggers) cached.triggers = msg.triggers;
    if (typeof msg.confidence === 'number') cached.confidence = msg.confidence;
    if (msg.security_mode !== undefined) cached.security_mode = msg.security_mode;
  }

  // ── Card updaters ──

  function updatePeopleCard() {
    var blobs = cached.blobs || [];
    var zones = cached.zones || [];
    var peopleNames = [];
    for (var i = 0; i < blobs.length; i++) {
      var p = blobs[i].person;
      if (p && p !== 'Unknown' && peopleNames.indexOf(p) === -1) peopleNames.push(p);
    }

    $peopleCnt.textContent = peopleNames.length + (peopleNames.length === 1 ? ' person' : ' people');
    if (peopleNames.length > 0) {
      $peopleDtl.textContent = peopleNames.join(', ');
    } else {
      var occupied = zones.filter(function (z) { return z.count > 0; });
      $peopleDtl.textContent = occupied.length > 0
        ? occupied.map(function (z) { return z.name; }).join(', ')
        : 'No one detected';
    }
  }

  function updateDevicesCard() {
    var nodes = cached.nodes || [];
    var online = nodes.filter(function (n) { return n.status === 'online'; });
    var stale  = nodes.filter(function (n) { return n.status === 'stale'; });
    var offline = nodes.filter(function (n) { return n.status === 'offline'; });

    $devicesCnt.textContent = online.length + '/' + nodes.length + ' online';
    $devicesMeta.innerHTML = '';

    if (offline.length > 0) {
      $devicesDtl.textContent = offline.length + ' device' + (offline.length > 1 ? 's' : '') + ' offline';
      addTag($devicesMeta, 'alert', offline.length + ' offline');
    } else if (stale.length > 0) {
      $devicesDtl.textContent = stale.length + ' device' + (stale.length > 1 ? 's' : '') + ' stale';
      addTag($devicesMeta, 'warn', stale.length + ' stale');
    } else {
      $devicesDtl.textContent = nodes.length === 0
        ? 'No devices registered'
        : 'All devices healthy';
    }

    var quality = cached.confidence;
    if (typeof quality === 'number') {
      var qLevel = quality >= 80 ? 'ok' : quality >= 60 ? 'warn' : 'alert';
      addTag($devicesMeta, qLevel, quality + '% quality');
    }
  }

  function addTag(container, level, text) {
    var span = document.createElement('span');
    span.className = 'home-card__tag home-card__tag--' + level;
    span.textContent = text;
    container.appendChild(span);
  }

  function updateEventsCard() {
    if (recentEvents.length === 0) {
      $eventsDtl.textContent = 'No recent events';
      return;
    }
    var lines = recentEvents.slice(0, 5).map(function (e) {
      var label = e.person || e.zone || e.type || 'Event';
      var time  = e.timestamp_ms ? relativeTime(e.timestamp_ms) : '';
      return label + (time ? ' — ' + time : '');
    });
    $eventsDtl.innerHTML = lines.join('<br>');
  }

  function updateExtras() {
    // Morning briefing
    if (cached.briefing) {
      $briefing.textContent = cached.briefing;
      $briefing.classList.add('home-extras__item--visible');
    }
    // Security mode
    if (cached.security_mode) {
      $security.innerHTML = '<strong>Security mode: ARMED</strong>';
      $security.classList.add('home-extras__item--visible', 'home-extras__item--armed');
    } else {
      $security.classList.remove('home-extras__item--visible', 'home-extras__item--armed');
    }
    // Anomaly
    if (cached.anomaly_active) {
      $anomaly.textContent = 'Anomaly detected';
      $anomaly.classList.add('home-extras__item--visible');
    }
  }

  function updateBanner() {
    var nodes   = cached.nodes || [];
    var blobs   = cached.blobs || [];
    var offline = nodes.filter(function (n) { return n.status === 'offline'; });
    var online  = nodes.filter(function (n) { return n.status === 'online'; });

    // Check for active alerts (fall, security) in recent events
    var now = Date.now();
    var recentAlert = null;
    for (var i = 0; i < recentEvents.length; i++) {
      var e = recentEvents[i];
      var age = now - (e.timestamp_ms || 0);
      if (age > 300000) break; // only consider last 5 min
      if (e.type === 'fall_alert') {
        recentAlert = 'Fall alert' + (e.zone ? ' in ' + e.zone : '') +
                      (e.person ? ': ' + e.person : '');
        break;
      }
      if (e.type === 'security_alert') {
        recentAlert = 'Security alert' + (e.zone ? ' in ' + e.zone : '');
        break;
      }
    }

    if (recentAlert) {
      setBanner('alert', recentAlert);
    } else if (offline.length > 0) {
      setBanner('warn', offline.length + ' device' + (offline.length > 1 ? 's' : '') + ' offline');
    } else {
      var names = [];
      for (var i = 0; i < blobs.length; i++) {
        var p = blobs[i].person;
        if (p && names.indexOf(p) === -1) names.push(p);
      }
      if (names.length > 0) {
        setBanner('ok', 'All clear — ' + names.length + ' people home, ' +
                   online.length + ' devices online');
      } else {
        setBanner('ok', 'All clear — No one detected, ' + online.length + ' devices online');
      }
    }
  }

  // ── Snapshot processing ──

  function handleSnapshot(snapshot) {
    cached = {
      blobs: snapshot.blobs || [],
      nodes: snapshot.nodes || [],
      zones: snapshot.zones || [],
      triggers: snapshot.triggers || [],
      events: snapshot.events || [],
      confidence: snapshot.confidence,
      security_mode: snapshot.security_mode,
      briefing: snapshot.briefing,
      anomaly_active: snapshot.anomaly_active,
    };

    if (snapshot.events && snapshot.events.length > 0) {
      recentEvents = snapshot.events.slice(0, 5);
    }

    updateBanner();
    updatePeopleCard();
    updateDevicesCard();
    updateEventsCard();
    updateExtras();
  }

  function handleIncremental(msg) {
    mergeIncremental(msg);

    if (msg.events && msg.events.length > 0) {
      recentEvents = msg.events.concat(recentEvents).slice(0, 5);
    }

    // Always refresh all cards from cached state
    updateBanner();
    updatePeopleCard();
    updateDevicesCard();
    updateEventsCard();
    updateExtras();
  }

  // ── WebSocket ──

  function connect() {
    var proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
    var url   = proto + '//' + location.host + '/ws/dashboard';
    ws = new WebSocket(url);

    ws.onopen = function () {
      reconnectDelay = 1000;
    };

    ws.onmessage = function (evt) {
      var msg;
      try { msg = JSON.parse(evt.data); } catch (e) { return; }

      if (msg.type === 'snapshot') {
        handleSnapshot(msg);
      } else {
        handleIncremental(msg);
      }
    };

    ws.onclose = function () {
      setBanner('warn', 'Reconnecting&hellip;');
      setTimeout(function () {
        reconnectDelay = Math.min(reconnectDelay * 2, 10000);
        connect();
      }, reconnectDelay);
    };

    ws.onerror = function () {
      ws.close();
    };
  }

  // ── Boot ──
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', connect);
  } else {
    connect();
  }

  // Refresh relative timestamps every 30s
  setInterval(updateEventsCard, 30000);
})();
