/**
 * home-cards.js — Populates the Spaxel home page from /ws/dashboard snapshot.
 *
 * Connects to the dashboard WebSocket, reads the initial snapshot to fill all
 * three cards, then applies incremental updates to refresh counts only.
 */
(function () {
  'use strict';

  // ── DOM refs ──
  const $banner     = document.getElementById('status-banner');
  const $dot        = document.getElementById('ws-dot');
  const $peopleCnt  = document.getElementById('people-count');
  const $peopleDtl  = document.getElementById('people-detail');
  const $devicesCnt = document.getElementById('devices-count');
  const $devicesDtl = document.getElementById('devices-detail');
  const $devicesMeta= document.getElementById('devices-meta');
  const $eventsDtl  = document.getElementById('events-detail');
  const $briefing   = document.getElementById('briefing-card');
  const $anomaly    = document.getElementById('anomaly-banner');
  const $security   = document.getElementById('security-toggle');

  // ── State ──
  let ws = null;
  let reconnectDelay = 1000;
  let recentEvents = [];

  // ── Helpers ──
  function $(id) { return document.getElementById(id); }

  function relativeTime(isoOrMs) {
    const ts = typeof isoOrMs === 'number' ? isoOrMs : new Date(isoOrMs).getTime();
    const diff = Date.now() - ts;
    if (diff < 60000)    return 'just now';
    if (diff < 3600000)  return Math.floor(diff / 60000) + 'm ago';
    if (diff < 86400000) return Math.floor(diff / 3600000) + 'h ago';
    return Math.floor(diff / 86400000) + 'd ago';
  }

  function setBanner(level, html) {
    $banner.className = 'home-status__banner home-status__banner--' + level;
    $banner.innerHTML = '<span class="home-status__dot home-status__dot--connected" id="ws-dot"></span>' + html;
  }

  // ── Card updaters ──

  function updatePeopleCard(snapshot) {
    const blobs = snapshot.blobs || [];
    const zones = snapshot.zones || [];
    const peopleNames = blobs
      .map(function (b) { return b.person; })
      .filter(function (p) { return p && p !== 'Unknown'; });

    const uniquePeople = [];
    peopleNames.forEach(function (n) {
      if (uniquePeople.indexOf(n) === -1) uniquePeople.push(n);
    });

    $peopleCnt.textContent = uniquePeople.length + ' people';
    if (uniquePeople.length > 0) {
      $peopleDtl.textContent = uniquePeople.join(', ');
    } else {
      var occupied = zones.filter(function (z) { return z.count > 0; });
      $peopleDtl.textContent = occupied.length > 0
        ? occupied.map(function (z) { return z.name; }).join(', ')
        : 'No one detected';
    }
  }

  function updateDevicesCard(snapshot) {
    var nodes = snapshot.nodes || [];
    var online = nodes.filter(function (n) { return n.status === 'online'; });
    var stale  = nodes.filter(function (n) { return n.status === 'stale'; });
    var offline= nodes.filter(function (n) { return n.status === 'offline'; });

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

    var quality = snapshot.confidence;
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

  function updateEventsCard(snapshot) {
    var evts = snapshot.events || [];
    if (evts.length > 0) {
      recentEvents = evts.slice(0, 5);
    }
    renderEvents();
  }

  function renderEvents() {
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

  function updateExtras(snapshot) {
    // Morning briefing
    if (snapshot.briefing) {
      $briefing.textContent = snapshot.briefing;
      $briefing.classList.add('home-extras__item--visible');
    }
    // Security mode
    if (snapshot.security_mode) {
      $security.textContent = 'Security mode: ARMED';
      $security.classList.add('home-extras__item--visible');
    }
    // Anomaly
    if (snapshot.anomaly_active) {
      $anomaly.textContent = 'Anomaly detected';
      $anomaly.classList.add('home-extras__item--visible');
    }
  }

  function updateBanner(snapshot) {
    var nodes    = snapshot.nodes || [];
    var blobs    = snapshot.blobs || [];
    var offline  = nodes.filter(function (n) { return n.status === 'offline'; });
    var people   = blobs.filter(function (b) { return b.person; });

    if (offline.length > 0) {
      setBanner('warn', offline.length + ' device' + (offline.length > 1 ? 's' : '') + ' offline');
    } else if (people.length > 0) {
      var names = [];
      people.forEach(function (b) {
        if (b.person && names.indexOf(b.person) === -1) names.push(b.person);
      });
      var online = nodes.filter(function (n) { return n.status === 'online'; });
      setBanner('ok', 'All clear — ' + names.length + ' people home, ' +
                 online.length + ' devices online');
    } else {
      var onlineN = nodes.filter(function (n) { return n.status === 'online'; });
      setBanner('ok', 'All clear — No one detected, ' + onlineN.length + ' devices online');
    }
  }

  // ── Snapshot processing ──

  function handleSnapshot(snapshot) {
    updateBanner(snapshot);
    updatePeopleCard(snapshot);
    updateDevicesCard(snapshot);
    updateEventsCard(snapshot);
    updateExtras(snapshot);
  }

  function handleIncremental(msg) {
    // Lightweight refresh on incremental updates
    if (msg.blobs)    updatePeopleCard(msg);
    if (msg.nodes)    { updateDevicesCard(msg); updateBanner(msg); }
    if (msg.events && msg.events.length > 0) {
      // Prepend new events, keep last 5
      recentEvents = msg.events.concat(recentEvents).slice(0, 5);
      renderEvents();
    }
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
  setInterval(renderEvents, 30000);
})();
