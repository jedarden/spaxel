const fs = require('fs');
const path = require('path');
const { test, expect } = require('@playwright/test');
const { corePages, dashboardPages } = require('./accessibility/pages');

/**
 * Agentation mount smoke test (spaxel-ba77654a).
 *
 * Workspace guidance requires Agentation on every UI page, verified by the
 * `#agentation-root` mount — not by grepping for a script tag: without its
 * import map the module script dies on unresolvable bare specifiers while
 * the page itself keeps rendering, so the tag alone is NOT evidence.
 *
 * Each entry point therefore gets four checks:
 *
 *   1. an import map whose JSON parses and resolves `react/jsx-runtime` to
 *      the self-hosted vendor chunk (live.html already had an import map for
 *      Three.js — a document allows only one, so the entry is merged there;
 *      this test reads whatever maps exist). react itself is inlined into
 *      agentation.js by the build (react-dom's lazy CommonJS factories
 *      cannot be ESM-externalized), so the map carries exactly the one
 *      specifier the bundle imports,
 *   2. `/agentation.js` and the vendor chunk fetched with status 200 and a
 *      JavaScript Content-Type — the mothership answers unknown paths with
 *      index.html at 200 text/html (the SPA fallback), so status alone would
 *      not catch a mis-routed module,
 *   3. no page error from module resolution or the agentation/react stack
 *      (unrelated page errors — Three.js CDN, absent backend sockets — are
 *      out of scope here),
 *   4. `#agentation-root` attached, and agentation's toolbar actually
 *      rendered: v3 portals the toolbar to document.body and renders its UI
 *      inside a shadow root, so the light DOM shows only the portal host —
 *      the `[data-agentation-root]` wrapper the toolbar renders is reachable
 *      only through `agentation-toolbar`'s shadowRoot, which is where this
 *      test looks. (An earlier draft queried it from the light DOM and could
 *      never match — querySelector does not pierce shadow boundaries.)
 *
 * The wiring lives in each HTML entry point (import map + module script);
 * the bundles are built by `npm run build:agentation`.
 */

const MODULE_SCRIPT = '/agentation.js';
const VENDOR_CHUNKS = {
  'react/jsx-runtime': '/static/vendor/react-jsx-runtime.js',
};

// The agentation mount test fails only on errors from the module stack it
// exists to verify. The dashboard pages talk to Three.js CDNs and expect a
// live backend (`/ws/*`) that the static test server cannot provide; failing
// on those would measure the fixture, not the wiring.
const agentationErrorPattern =
  /agentation|react|importmap|vendor|Failed to resolve module specifier|error loading module/i;

function isJavaScriptContentType(contentType) {
  const type = (contentType || '').toLowerCase();
  return (
    type.startsWith('text/javascript') ||
    type.startsWith('application/javascript')
  );
}

// De-duplicated union: pages.js deliberately lists live.html in both scan
// groups; the mount wiring per document is singular.
const entryPoints = [
  ...new Map([...corePages, ...dashboardPages].map((p) => [p.path, p])).values(),
];

for (const entry of entryPoints) {
  test(`agentation mounts on ${entry.name} (${entry.path})`, async ({ page }) => {
    // ambient.html (and any auth-aware page) bounces to / when the PIN is
    // configured; pin_configured:false keeps every entry point on its own URL.
    await page.route('**/api/auth/status', (route) =>
      route.fulfill({ json: { pin_configured: false } })
    );

    const pageErrors = [];
    page.on('pageerror', (err) => pageErrors.push(String(err)));

    // waitForResponse registered before goto, rather than a response event
    // listener, so a module that is never fetched fails fast with a null
    // instead of silently missing an event.
    const modulePaths = [MODULE_SCRIPT, ...Object.values(VENDOR_CHUNKS)];
    const responsesPromise = Promise.all(
      modulePaths.map((pathname) =>
        page
          .waitForResponse((res) => new URL(res.url()).pathname === pathname, {
            timeout: 10_000,
          })
          .then((res) => ({
            pathname,
            status: res.status(),
            contentType: res.headers()['content-type'] || '',
          }))
          .catch(() => null)
      )
    );

    await page.goto(entry.path, { waitUntil: 'load' });
    const moduleResponses = (await responsesPromise).filter(Boolean);

    // 1. Import map present, valid JSON, and resolving the bare specifiers
    //    agentation.js imports. All maps in the document are merged (there
    //    should be exactly one per document, but the union is what the
    //    browser applies — and reading them this way keeps live.html's
    //    merged Three.js map and the standalone maps on one code path).
    const importMaps = await page.evaluate(() =>
      [...document.querySelectorAll('script[type="importmap"]')].map((s) => {
        try {
          return { ok: true, imports: JSON.parse(s.textContent).imports ?? {} };
        } catch {
          return { ok: false, imports: {} };
        }
      })
    );
    expect(
      importMaps.length,
      `${entry.path}: no <script type="importmap"> found — agentation.js imports bare ` +
        `specifiers that cannot resolve without it`
    ).toBeGreaterThan(0);
    for (const [index, map] of importMaps.entries()) {
      expect(
        map.ok,
        `${entry.path}: import map #${index + 1} is not valid JSON`
      ).toBe(true);
    }
    const mergedImports = Object.assign({}, ...importMaps.map((m) => m.imports));
    for (const [specifier, expectedPath] of Object.entries(VENDOR_CHUNKS)) {
      expect(
        mergedImports[specifier],
        `${entry.path}: import map does not resolve "${specifier}" to ${expectedPath}`
      ).toBe(expectedPath);
    }

    // The module script itself must be wired as a module (classic scripts
    // cannot import).
    const hasModuleScript = await page.evaluate(
      (src) => !!document.querySelector(`script[type="module"][src="${src}"]`),
      MODULE_SCRIPT
    );
    expect(
      hasModuleScript,
      `${entry.path}: no <script type="module" src="${MODULE_SCRIPT}"> in the document`
    ).toBe(true);

    // 2. Every module of the agentation graph fetched OK, as JavaScript.
    //    A 200 with text/html here is the SPA fallback serving index.html in
    //    place of the file — the module still fails, just unidiagnostically.
    const expected = [MODULE_SCRIPT, ...Object.values(VENDOR_CHUNKS)];
    for (const pathname of expected) {
      const res = moduleResponses.find((r) => r.pathname === pathname);
      expect(
        res,
        `${entry.path}: ${pathname} was never fetched (import map or script tag miswired)`
      ).toBeTruthy();
      expect(
        res.status,
        `${entry.path}: GET ${pathname} returned ${res.status}`
      ).toBe(200);
      expect(
        isJavaScriptContentType(res.contentType),
        `${entry.path}: GET ${pathname} served Content-Type "${res.contentType}" — ` +
          `modules require text/javascript (text/html means the SPA fallback answered)`
      ).toBe(true);
    }

    // 3. No errors from the agentation module stack.
    const relevant = pageErrors.filter((e) => agentationErrorPattern.test(e));
    expect(
      relevant,
      `${entry.path}: module/agentation errors during load:\n${relevant.join('\n')}`
    ).toEqual([]);

    // 4. The mount: #agentation-root attached to the document (the workspace
    //    recipe's verification signal) AND agentation's toolbar actually
    //    rendered. The toolbar React-commits through a portal on document.body
    //    into a shadow root, so the rendered evidence is
    //    agentation-toolbar.shadowRoot [data-agentation-root] — light-DOM
    //    querySelector cannot cross the shadow boundary. React commits
    //    asynchronously after render(), so poll rather than checking once.
    await page.waitForFunction(
      () => {
        const root = document.getElementById('agentation-root');
        if (!root || !root.isConnected) return false;
        const toolbar = document.querySelector('agentation-toolbar');
        return (
          !!toolbar &&
          !!toolbar.shadowRoot &&
          !!toolbar.shadowRoot.querySelector('[data-agentation-root]')
        );
      },
      undefined,
      { timeout: 15_000 }
    );
    const mount = await page.evaluate(() => {
      const root = document.getElementById('agentation-root');
      const toolbar = document.querySelector('agentation-toolbar');
      return {
        attached: !!root && root.isConnected,
        toolbarRendered:
          !!toolbar &&
          !!toolbar.shadowRoot &&
          !!toolbar.shadowRoot.querySelector('[data-agentation-root]'),
      };
    });
    expect(
      mount.attached,
      `${entry.path}: #agentation-root exists but is not attached to the document`
    ).toBe(true);
    expect(
      mount.toolbarRendered,
      `${entry.path}: #agentation-root is mounted but agentation-toolbar has no ` +
        `rendered [data-agentation-root] in its shadow root — React never committed`
    ).toBe(true);
  });
}

/**
 * Coverage guard, mirroring tests/a11y-entrypoint-coverage.spec.js: the specs
 * run against the static file server rooted at dashboard/, so the on-disk
 * set and the served set are the same set. An entry point added without
 * agentation wiring must fail here deliberately rather than ship silently
 * bare.
 */
test('every dashboard HTML entry point has agentation coverage', () => {
  const dashboardRoot = path.join(__dirname, '..');
  const onDisk = fs
    .readdirSync(dashboardRoot)
    .filter((f) => f.endsWith('.html'))
    .sort();

  const covered = entryPoints.map((p) => p.path.replace(/^\//, ''));
  const uncovered = onDisk.filter((f) => !covered.includes(f));
  const phantom = [...new Set(covered)].filter((f) => !onDisk.includes(f));

  expect(
    uncovered,
    `entry point(s) without agentation coverage: ${uncovered.join(', ')} — add them to ` +
      `tests/accessibility/pages.js and wire the agentation import map + ` +
      `module script (see tests/agentation-mount.spec.js)`
  ).toEqual([]);

  expect(
    phantom,
    `agentation spec references non-existent entry point(s): ${phantom.join(', ')}`
  ).toEqual([]);
});
