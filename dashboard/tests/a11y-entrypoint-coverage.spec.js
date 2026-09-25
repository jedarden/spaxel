const fs = require('fs');
const path = require('path');
const { test, expect } = require('@playwright/test');
const { corePages, dashboardPages } = require('./accessibility/pages');

/**
 * Entry-point coverage guard for the a11y gate.
 *
 * The structure docs enumerate the dashboard's HTML entry points
 * (docs/codebase-structure-and-test-patterns.md — "8 HTML entry points";
 * docs/repo-structure.md section 8). This test fails the gate whenever an
 * entry point exists on disk without WCAG 2.1 AA coverage in one of the page
 * specs, so the enumeration cannot silently drift out of date.
 *
 * Only top-level dashboard/*.html counts as an entry point. Dev-only
 * harnesses live under dashboard/_dev/ — go:embed excludes `_`-prefixed
 * path segments, so they never ship in the production image and stay
 * outside this gate (see dashboard/_dev/README.md). Moving a harness back
 * to the top level therefore fails here, forcing the deliberate choice of
 * either gating it or re-homing it under _dev/.
 *
 * The specs run against the static file server rooted at dashboard/, so the
 * on-disk set and the served set are the same set.
 */
test('every dashboard HTML entry point has a11y coverage', () => {
  const dashboardRoot = path.join(__dirname, '..');
  const onDisk = fs
    .readdirSync(dashboardRoot)
    .filter((f) => f.endsWith('.html'))
    .sort();

  const covered = [...corePages, ...dashboardPages].map((p) => p.path.replace(/^\//, ''));
  const uncovered = onDisk.filter((f) => !covered.includes(f));
  const phantom = [...new Set(covered)].filter((f) => !onDisk.includes(f));

  expect(
    uncovered,
    `entry point(s) without a11y coverage: ${uncovered.join(', ')} — add them to ` +
      `tests/accessibility/pages.js and scan them in a11y.spec.js or ` +
      `a11y-dashboard.spec.js (see docs/ci-accessibility-integration.md)`
  ).toEqual([]);

  expect(
    phantom,
    `a11y specs reference non-existent entry point(s): ${phantom.join(', ')}`
  ).toEqual([]);
});
