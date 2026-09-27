// NixOS-local variant of playwright.config.js for the a11y/agentation suite.
//
// Playwright-managed chromium builds do not launch on NixOS: they are built for
// glibc/FHS distributions and are dynamically linked against system libraries
// NixOS does not expose at /usr/lib and /lib64, so every browser under
// ~/.cache/ms-playwright/ dies at launch with exit code 127. The one browser
// that works is the Nix-wrapped chromium in /nix/store/, whose Nix RPATH
// carries its whole library closure. This config points the suite at it via
// use.launchOptions.executablePath — the nesting matters, Playwright silently
// ignores a top-level use.executablePath.
//
// Usage (from dashboard/):
//   npx playwright test --config=playwright.nix.config.js
//
// See docs/ci-accessibility-integration.md, "Running locally on NixOS".
// The store path is resolved fresh at load (the hash changes on every chromium
// upgrade; never copy an old one). Set SPAXEL_CHROMIUM to force a specific
// browser when several builds coexist in the store. On non-NixOS hosts use the
// default playwright.config.js — this config throws when no Nix chromium exists.

const fs = require('fs');
const path = require('path');
const { defineConfig } = require('@playwright/test');
const base = require('./playwright.config.js');

function nixChromiumPath() {
  if (process.env.SPAXEL_CHROMIUM) return process.env.SPAXEL_CHROMIUM;
  const store = '/nix/store';
  if (!fs.existsSync(store)) return null;
  const hits = fs
    .readdirSync(store)
    .filter(
      (name) =>
        name.includes('chromium') &&
        fs.existsSync(path.join(store, name, 'bin', 'chromium'))
    )
    .sort();
  return hits.length > 0
    ? path.join(store, hits[hits.length - 1], 'bin', 'chromium')
    : null;
}

const executablePath = nixChromiumPath();
if (!executablePath) {
  throw new Error(
    'playwright.nix.config.js: no chromium found under /nix/store ' +
      '(set SPAXEL_CHROMIUM to the browser path). On non-NixOS hosts use the ' +
      'default playwright.config.js instead.'
  );
}

module.exports = defineConfig({
  ...base,
  // Keep test artifacts out of the shared checkout entirely.
  outputDir: '/tmp/pw-nix-results',
  use: {
    ...base.use,
    launchOptions: { executablePath },
  },
  webServer: {
    ...base.webServer,
    // Pin the static server to this directory so the config works from any cwd.
    cwd: __dirname,
  },
});
