/**
 * demo-indicator.js — Shows a "Demo Mode" badge when the server runs in demo mode.
 *
 * Fetches /api/auth/status once on load. When the server reports demo_mode the
 * badge is appended to <body> so every user knows the dashboard is read-only
 * and PIN-gated. When demo mode is off nothing is rendered at all.
 */
(function () {
  'use strict';

  var isDemoMode = false;

  function buildBadge() {
    var badge = document.createElement('div');
    badge.id = 'demo-mode-banner';
    badge.className = 'demo-mode-banner';
    badge.setAttribute('role', 'status');
    badge.textContent = 'Demo Mode';
    return badge;
  }

  // ── Styles ──
  // Injected once so the badge ships with its module instead of requiring
  // every page's stylesheet to know about it.
  function injectStyles() {
    if (document.getElementById('demo-mode-banner-style')) return;
    var style = document.createElement('style');
    style.id = 'demo-mode-banner-style';
    style.textContent =
      '.demo-mode-banner{position:fixed;bottom:12px;right:12px;z-index:9999;' +
      'padding:6px 14px;border-radius:999px;background:#b45309;color:#fff;' +
      'font:600 13px/1.4 system-ui,-apple-system,sans-serif;letter-spacing:.03em;' +
      'box-shadow:0 2px 8px rgba(0,0,0,.25);pointer-events:none;}';
    document.head.appendChild(style);
  }

  function check() {
    return fetch('/api/auth/status')
      .then(function (res) { return res.json(); })
      .then(function (data) {
        isDemoMode = !!data.demo_mode;
        render();
        return isDemoMode;
      })
      .catch(function () {
        // Fail closed: an unreachable status endpoint never shows the badge.
        isDemoMode = false;
        render();
        return isDemoMode;
      });
  }

  function render() {
    var existing = document.getElementById('demo-mode-banner');
    if (isDemoMode) {
      if (!existing) document.body.appendChild(buildBadge());
      injectStyles();
    } else if (existing) {
      existing.remove();
    }
  }

  // ── Public API ──
  window.SpaxelDemoIndicator = {
    check: check,
    render: render,
    isDemoMode: function () { return isDemoMode; }
  };

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', check);
  } else {
    check();
  }
})();
