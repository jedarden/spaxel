/**
 * Canonical enumeration of the dashboard's HTML entry points for the a11y gate.
 *
 * The set must match the nine entry points documented in
 * docs/codebase-structure-and-test-patterns.md ("9 HTML entry points") and
 * docs/repo-structure.md §8; docs/ci-accessibility-integration.md maps each
 * one to the spec that scans it. tests/a11y-entrypoint-coverage.spec.js
 * fails the gate if a file in dashboard/*.html is missing from this module.
 *
 * This is a plain module (no test() calls) so that spec files and the guard
 * can require it — requiring one spec file from another would re-register
 * its tests and trip Playwright's duplicate-title check.
 */

// Core pages, scanned in tests/a11y.spec.js.
const corePages = [
  { name: 'index', path: '/index.html' },
  { name: 'live', path: '/live.html' },
  { name: 'fleet', path: '/fleet.html' },
  { name: 'setup', path: '/setup.html' },
  { name: 'integrations', path: '/integrations.html' },
];

// Remaining entry points, scanned in tests/a11y-dashboard.spec.js.
const dashboardPages = [
  { name: 'ambient', path: '/ambient.html' },
  { name: 'live', path: '/live.html' },
  { name: 'simple', path: '/simple.html' },
  { name: 'simulator', path: '/simulator.html' },
  { name: 'test-transformcontrols', path: '/test-transformcontrols.html' },
];

module.exports = { corePages, dashboardPages };
